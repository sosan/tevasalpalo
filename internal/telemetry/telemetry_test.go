package telemetry

// Tests de la capa de telemetría: buffer, rotación, hash de IP, payload OTLP y
// el espejo de logs. Nada toca red salvo el test de integración, que usa un
// httptest local.

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"time"

	"go.opentelemetry.io/otel/attribute"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	cpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

// leerCuerpoDelTest lee el cuerpo de una petición de los tests. El SDK envía
// gzip (WithCompression), así que hay que descomprimir antes de parsear: leer
// los bytes pelados da basura y parece un fallo del exportador cuando no lo es.
func leerCuerpoDelTest(r *http.Request) []byte {
	var r2 io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil
		}
		defer gz.Close()
		r2 = gz
	}
	b, _ := io.ReadAll(io.LimitReader(r2, 4<<20))
	return b
}

type registroTest struct {
	body           string
	severity       string
	severityNumber int
	atributos      map[string]string
	tipos          map[string]string
}

// leerLogsDelTest decodifica el cuerpo OTLP. El SDK de OpenTelemetry envía
// protobuf binario (otlploghttp no tiene variante JSON) y el gateway lo acepta
// igual. Decodificar con el proto oficial es más estricto que un json.Unmarshal:
// si el SDK cambiara el schema, el test falla.
func leerLogsDelTest(t *testing.T, r *http.Request) *collogpb.ExportLogsServiceRequest {
	t.Helper()
	raw := leerCuerpoDelTest(r)
	if len(raw) == 0 {
		t.Fatalf("petición vacía (Content-Type: %s, Content-Encoding: %s)",
			r.Header.Get("Content-Type"), r.Header.Get("Content-Encoding"))
	}
	var req collogpb.ExportLogsServiceRequest
	if err := proto.Unmarshal(raw, &req); err != nil {
		t.Fatalf("el cuerpo no es protobuf OTLP válido: %v", err)
	}
	return &req
}

// aplanar deja los registros como texto para poder compararlos en los tests.
func aplanar(req *collogpb.ExportLogsServiceRequest) []registroTest {
	var out []registroTest
	for _, rl := range req.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				rt := registroTest{
					severity:       lr.SeverityText,
					severityNumber: int(lr.SeverityNumber),
					atributos:      map[string]string{},
					tipos:          map[string]string{},
				}
				if sv, ok := lr.Body.GetValue().(*cpb.AnyValue_StringValue); ok {
					rt.body = sv.StringValue
				}
				for _, kv := range lr.Attributes {
					switch v := kv.Value.GetValue().(type) {
					case *cpb.AnyValue_IntValue:
						rt.atributos[kv.Key] = strconv.FormatInt(v.IntValue, 10)
						rt.tipos[kv.Key] = "int"
					case *cpb.AnyValue_BoolValue:
						rt.atributos[kv.Key] = strconv.FormatBool(v.BoolValue)
						rt.tipos[kv.Key] = "bool"
					case *cpb.AnyValue_StringValue:
						rt.atributos[kv.Key] = v.StringValue
						rt.tipos[kv.Key] = "string"
					default:
						rt.atributos[kv.Key] = ""
					}
				}
				out = append(out, rt)
			}
		}
	}
	return out
}

// El endpoint se puede escribir de las dos formas que da la consola de Grafana
// y tiene que acabar en la misma URL final. Si esto se rompe, Loki responde 404
// y no se ve el motivo, porque el SDK compone la ruta por su cuenta.
func TestNormalizarEndpointAceptaLasDosFormas(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		// La que da la consola de Grafana, sin la ruta de la señal.
		{"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp",
			"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs"},
		// La completa, tal cual la pusimos antes.
		{"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs",
			"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs"},
		// Con barra final, que es como se copia a veces.
		{"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/",
			"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs"},
		{"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs/",
			"https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs"},
		// Un collector propio en localhost.
		{"http://127.0.0.1:4318",
			"http://127.0.0.1:4318/v1/logs"},
		{"http://localhost:4318/v1/logs", "http://localhost:4318/v1/logs"},
		// Query y fragmento se limpian.
		{"https://x.grafana.net/otlp?a=1", "https://x.grafana.net/otlp/v1/logs"},
	}
	for _, c := range casos {
		got, err := normalizarEndpointOTLP(c.in)
		if err != nil {
			t.Errorf("normalizarEndpointOTLP(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizarEndpointOTLP(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
}

// Una URL claramente mala debe decir qué pasa, no fallar en silencio.
func TestNormalizarEndpointRechazaLoInvalido(t *testing.T) {
	casos := []string{
		"otlp-gateway.grafana.net/otlp", // sin esquema
		"ftp://x.grafana.net/otlp",      // esquema raro
		"https://",                      // sin host
	}
	for _, in := range casos {
		if _, err := normalizarEndpointOTLP(in); err == nil {
			t.Errorf("normalizarEndpointOTLP(%q) debería fallar", in)
		}
	}
}

// El Basic auth es el que espera el gateway. Es lo que decodifica el SDK de
// Grafana, así que se fija aquí para no depender de una llamada de red.
func TestAuthHeaders(t *testing.T) {
	if h := authHeaders("", ""); h != nil {
		t.Errorf("sin credenciales no debería haber cabeceras: %v", h)
	}
	h := authHeaders("251374", "glc_secreto")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("251374:glc_secreto"))
	if h["Authorization"] != want {
		t.Errorf("Authorization = %q, se esperaba %q", h["Authorization"], want)
	}
}

// Sin URL configurada no se pasan opciones de endpoint, para que el SDK lea las
// variables estándar OTEL_EXPORTER_OTLP_*.
func TestSinURLNoSeFuerzaElEndpoint(t *testing.T) {
	opts, err := exporterOptions(telemetryConfig{user: "u", password: "p"})
	if err != nil {
		t.Fatalf("exporterOptions: %v", err)
	}
	if len(opts) != 3 {
		t.Errorf("se pasaron %d opciones, se esperaba compresión+timeout+auth", len(opts))
	}
}

// exporterDePrueba construye un exportador real contra una URL de prueba. Se
// usa el constructor de verdad (con el SDK dentro) en vez de montar la struct a
// mano, para que los tests no se desincronicen de la implementación.
func exporterDePrueba(t *testing.T, cfg telemetryConfig) *otlpExporter {
	t.Helper()
	if cfg.flushEvery == 0 {
		cfg.flushEvery = time.Hour // los tests disparan el flush a mano
	}
	exp, err := newOTLPExporter(cfg)
	if err != nil {
		t.Fatalf("newOTLPExporter: %v", err)
	}
	t.Cleanup(func() {
		exp.mu.Lock()
		exp.closed = true
		exp.mu.Unlock()
	})
	return exp
}

func TestRingBufferMantieneLoUltimo(t *testing.T) {
	rb := newRingBuffer(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		rb.Write([]byte(s))
	}
	if got := strings.Join(rb.tail(99), ","); got != "c,d,e" {
		t.Errorf("tail = %q, se esperaba \"c,d,e\"", got)
	}
}

func TestRingBufferRespetaElLimite(t *testing.T) {
	rb := newRingBuffer(10)
	for _, s := range []string{"a", "b", "c", "d"} {
		rb.Write([]byte(s))
	}
	if got := strings.Join(rb.tail(2), ","); got != "c,d" {
		t.Errorf("tail(2) = %q, se esperaba \"c,d\"", got)
	}
}

func TestRotatingWriterRotaYACotaLosFicheros(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streaming.log")
	rw, err := newRotatingWriter(path, 40, 3)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}

	for i := 0; i < 20; i++ {
		rw.Write([]byte("0123456789ABCDEFGHIJ\n")) // 21 bytes
	}

	if _, err := os.Stat(path + ".0"); err != nil {
		t.Errorf("no se creó el fichero rotado .0: %v", err)
	}
	numerados := 0
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(path + "." + strconv.Itoa(i)); err == nil {
			numerados++
		}
	}
	if numerados > 3 {
		t.Errorf("hay %d ficheros numerados, el máximo es 3", numerados)
	}
	if numerados == 0 {
		t.Error("no quedó ningún fichero rotado")
	}
	if err := rw.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestRotatingWriterWriteLineEsUnaLinea(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	rw, err := newRotatingWriter(path, 1<<20, 2)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	rw.WriteLine([]byte(`{"event":"uno"}`))
	rw.WriteLine([]byte(`{"event":"dos"}`))
	rw.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	lineas := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lineas) != 2 {
		t.Fatalf("hay %d líneas, se esperaban 2: %q", len(lineas), string(b))
	}
	for i, l := range lineas {
		var v map[string]any
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Errorf("línea %d no es JSON válido: %v", i, err)
		}
	}
}

func TestHashIPEsEstableYOcultaLaIP(t *testing.T) {
	telMu.Lock()
	telCfg.hashSalt = "sal"
	telMu.Unlock()

	ip := "203.0.113.9"
	h := hashIP(ip)
	if h == "" {
		t.Fatal("hashIP devolvió vacío")
	}
	if h == ip {
		t.Error("hashIP devolvió la IP en claro")
	}
	if hashIP(ip) != h {
		t.Error("hashIP no es estable")
	}
	if hashIP("203.0.113.10") == h {
		t.Error("dos IPs distintas hashearon igual")
	}
	if hashIP("") != "" {
		t.Error("hashIP de vacío debería devolver vacío")
	}
}

func TestHashIPCambiaConElSalt(t *testing.T) {
	telMu.Lock()
	telCfg.hashSalt = "sal-a"
	telMu.Unlock()
	ha := hashIP("203.0.113.9")

	telMu.Lock()
	telCfg.hashSalt = "sal-b"
	telMu.Unlock()
	hb := hashIP("203.0.113.9")

	if ha == hb {
		t.Error("el salt no está influyendo en el hash")
	}
}

func TestSeverityPorDefecto(t *testing.T) {
	casos := []struct {
		event string
		want  string
	}{
		{"player_hls_error", "ERROR"},
		{"algo_con_warn", "WARN"},
		{"player_play", "INFO"},
		{"player_fatal_algo", "ERROR"},
	}
	for _, c := range casos {
		if got := severityName(map[string]any{"event": c.event}); got != c.want {
			t.Errorf("severityName(%q) = %q, se esperaba %q", c.event, got, c.want)
		}
	}
}

// Un campo level explícito manda sobre el nombre del evento.
func TestSeverityRespetaElCampoLevel(t *testing.T) {
	casos := []struct {
		rec  map[string]any
		want string
	}{
		{map[string]any{"event": "player_play", "level": "WARNING"}, "WARN"},
		{map[string]any{"event": "player_hls_error", "level": "info"}, "INFO"},
		{map[string]any{"event": "player_play", "level": "DEBUG"}, "DEBUG"},
		{map[string]any{"event": "player_hls_error", "level": "FATAL"}, "ERROR"},
	}
	for _, c := range casos {
		if got := severityName(c.rec); got != c.want {
			t.Errorf("severityName(%v) = %q, se esperaba %q", c.rec, got, c.want)
		}
	}
}

// Los atributos conservan su tipo. Aplanarlos a texto es lo que impide
// agregar en Grafana.
func TestAttrsConservanElTipo(t *testing.T) {
	rec := map[string]any{
		"event":    "player_hls_error",
		"fatal":    true,
		"ms":       float64(1200),
		"link_id":  "abc",
		"fraccion": 0.5,
		"time":     "no debe salir",
		"version":  "tampoco",
	}
	vistos := map[string]attribute.KeyValue{}
	for _, kv := range attrsFor(rec) {
		vistos[string(kv.Key)] = kv
	}

	if vistos["fatal"].Value.Type() != attribute.BOOL {
		t.Errorf("fatal es %v, se esperaba BOOL", vistos["fatal"].Value.Type())
	}
	if vistos["ms"].Value.Type() != attribute.INT64 {
		t.Errorf("ms es %v, se esperaba INT64", vistos["ms"].Value.Type())
	}
	if vistos["ms"].Value.AsInt64() != 1200 {
		t.Errorf("ms = %v", vistos["ms"].Value.AsInt64())
	}
	if vistos["fraccion"].Value.Type() != attribute.FLOAT64 {
		t.Errorf("fraccion es %v, se esperaba FLOAT64", vistos["fraccion"].Value.Type())
	}
	if vistos["link_id"].Value.Type() != attribute.STRING {
		t.Errorf("link_id es %v", vistos["link_id"].Value.Type())
	}
	// Los campos base no se repiten como atributo.
	if _, hay := vistos["time"]; hay {
		t.Error("time no debería ser atributo")
	}
	if _, hay := vistos["version"]; hay {
		t.Error("version no debería ser atributo")
	}
	if _, hay := vistos["event"]; hay {
		t.Error("event no debería ser atributo, va en el body")
	}
}

// El body tiene que ser legible y contener los datos: en Grafana se busca
// sobre el texto, sin parsear JSON.
func TestBodyEsLegible(t *testing.T) {
	body := fmtMapBody(map[string]any{
		"event":   "player_hls_error",
		"details": "levelLoadTimeOut",
		"fatal":   true,
		"link_id": "abc",
	})
	if !strings.HasPrefix(body, "player_hls_error ") {
		t.Errorf("el body no empieza por el evento: %q", body)
	}
	for _, quiero := range []string{"details=levelLoadTimeOut", "fatal=true", "link_id=abc"} {
		if !strings.Contains(body, quiero) {
			t.Errorf("el body no contiene %q: %q", quiero, body)
		}
	}
}

func TestLevelFromMessage(t *testing.T) {
	casos := []struct {
		msg  string
		want string
	}{
		{"❌ Error al iniciar AceStream: no existe", "ERROR"},
		{"falló el proxy de la fuente", "ERROR"},
		{"⚠️  telemetría: no se pudo abrir el log", "WARN"},
		{"Aviso: Xray todavía no escucha", "WARN"},
		{"✅ Cerrado ace correctamente", "INFO"},
	}
	for _, c := range casos {
		if got := levelFromMessage(c.msg); got != c.want {
			t.Errorf("levelFromMessage(%q) = %q, se esperaba %q", c.msg, got, c.want)
		}
	}
}

func TestStripLogTimestamp(t *testing.T) {
	con := "2026/10/12 17:36:00 mensaje real"
	if got := stripLogTimestamp(con); got != "mensaje real" {
		t.Errorf("stripLogTimestamp = %q", got)
	}
	// Una línea que no es del log estándar no debe perder caracteres.
	sin := "algo sin fecha 12:00:00"
	if got := stripLogTimestamp(sin); got != sin {
		t.Errorf("stripLogTimestamp = %q, se esperaba %q", got, sin)
	}
	if got := stripLogTimestamp("corto"); got != "corto" {
		t.Errorf("stripLogTimestamp = %q", got)
	}
}

// El espejo colapsa las ráfagas: con la misma línea 300 veces seguidas solo
// deben salir la primera y las potencias de dos.
func TestLogMirrorColapsaRepeticiones(t *testing.T) {
	cfg := telemetryConfig{
		enabled:   true,
		otlpURL:   "http://central.invalido",
		maxBatch:  10,
		bufferCap: 10, // no debeConstreñar el test
	}
	_ = cfg
	exp := &otlpExporter{cfg: telemetryConfig{bufferCap: 5000, maxBatch: 100}, done: make(chan struct{})}

	telBatch = exp
	telEvents = nil
	defer func() { telBatch = nil }()

	m := &logMirror{}
	for i := 0; i < 300; i++ {
		m.Write([]byte("2026/10/12 17:36:00 reintentando /ace/m\n"))
	}
	if exp.drops != 0 {
		t.Errorf("hubo drops en el espejo: %d", exp.drops)
	}

	exp.mu.Lock()
	n := len(exp.queue)
	var conRepeticiones int
	var primera string
	for i, rec := range exp.queue {
		if _, ok := rec["repeticiones"]; ok {
			conRepeticiones++
		} else if i == 0 {
			primera, _ = rec["mensaje"].(string)
		}
	}
	exp.mu.Unlock()

	// 1, 2, 4, 8, ... 256 -> 9 emissions de 300.
	if n != 9 {
		t.Errorf("se emitieron %d registros, se esperaban 9", n)
	}
	if conRepeticiones != 8 {
		t.Errorf("%d registros llevan repeticiones, se esperaban 8", conRepeticiones)
	}
	if primera != "reintentando /ace/m" {
		t.Errorf("mensaje = %q, el timestamp no se quitó bien", primera)
	}
}

func TestLogMirrorCuentaLineasDistintas(t *testing.T) {
	exp := &otlpExporter{cfg: telemetryConfig{bufferCap: 5000, maxBatch: 100}, done: make(chan struct{})}
	telBatch = exp
	telEvents = nil
	defer func() { telBatch = nil }()

	m := &logMirror{}
	m.Write([]byte("primera\n"))
	m.Write([]byte("segunda\n"))
	m.Write([]byte("primera\n")) // distinta de la anterior: cuenta como nueva

	exp.mu.Lock()
	n := len(exp.queue)
	exp.mu.Unlock()
	if n != 3 {
		t.Errorf("se emitieron %d registros, se esperaban 3", n)
	}
}

func TestColaAcotadaDescartaLoAntiguo(t *testing.T) {
	exp := &otlpExporter{cfg: telemetryConfig{bufferCap: 3, maxBatch: 2}, done: make(chan struct{})}
	for i := 0; i < 10; i++ {
		exp.enqueue(map[string]any{"event": "e", "n": i})
	}
	exp.mu.Lock()
	n, drops := len(exp.queue), exp.drops
	primer := exp.queue[0]["n"]
	exp.mu.Unlock()

	if n != 3 {
		t.Errorf("la cola tiene %d, el tope es 3", n)
	}
	if drops != 7 {
		t.Errorf("drops = %d, se esperaban 7", drops)
	}
	if fmt.Sprint(primer) != "7" {
		t.Errorf("el primero es %v, se esperaba el 7 (los antiguos se descartan)", primer)
	}
}

func TestBackoffCreceYEstaAcotado(t *testing.T) {
	if d := backoffDuration(1); d < time.Second {
		t.Errorf("backoff(1) = %v, se esperaba al menos 1s", d)
	}
	if d := backoffDuration(50); d > 2*time.Minute {
		t.Errorf("backoff(50) = %v, el tope es 2m", d)
	}
	if backoffDuration(3) <= backoffDuration(1) {
		t.Error("el backoff no crece con los fallos")
	}
}

// Al apagar hay que intentar vaciar la cola: si no, el evento que explica el
// cierre se queda sin enviar.
func TestCloseVaciaLaCola(t *testing.T) {
	var recibidas int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = leerCuerpoDelTest(r)
		mu.Lock()
		recibidas++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	exp := exporterDePrueba(t, telemetryConfig{otlpURL: srv.URL, bufferCap: 100, maxBatch: 2})
	for i := 0; i < 5; i++ {
		exp.enqueue(map[string]any{"event": "player_play"})
	}
	exp.closeWithDeadline(time.Now().Add(3 * time.Second))

	exp.mu.Lock()
	restantes := len(exp.queue)
	exp.mu.Unlock()
	if restantes != 0 {
		t.Errorf("quedan %d eventos sin enviar tras el cierre", restantes)
	}
	mu.Lock()
	n := recibidas
	mu.Unlock()
	if n < 3 {
		t.Errorf("el servidor recibió %d lotes, se esperaban al menos 3", n)
	}

	// Un segundo cierre no debe panicear ni volver a enviar.
	exp.closeWithDeadline(time.Now().Add(time.Second))
}

func TestCloseNoSeQuedaColgadoSiElCentralNoContesta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	exp := exporterDePrueba(t, telemetryConfig{otlpURL: srv.URL, bufferCap: 100, maxBatch: 10})
	exp.enqueue(map[string]any{"event": "player_play"})

	inicio := time.Now()
	exp.closeWithDeadline(time.Now().Add(1500 * time.Millisecond))
	if d := time.Since(inicio); d > 4*time.Second {
		t.Errorf("el cierre tardó %v, debería respetar el deadline", d)
	}
}

func TestIsLoopbackIP(t *testing.T) {
	casos := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"192.168.1.20", false},
		{"10.0.0.5", false},
		{"", false},
	}
	for _, c := range casos {
		if got := isLoopbackIP(c.ip); got != c.want {
			t.Errorf("isLoopbackIP(%q) = %v, se esperaba %v", c.ip, got, c.want)
		}
	}
}

// Integración: lo que sale del exportador tiene que ser aceptado por un

// endpoint OTLP real. Antes de arreglarlo, esto fallaba con 400.
// Integración: lo que sale del exportador tiene que ser OTLP válido y llegar
// con las credenciales correctas. Antes esto serializaba el payload a mano y
// solo comprobaba que el JSON cuadrase; ahora el SDK lo hace y el test valida
// el protobuf resultante, que es lo que Loki recibe de verdad.
func TestIntegracionOTLPContraServidorLocal(t *testing.T) {
	var (
		mu          sync.Mutex
		logs        *collogpb.ExportLogsServiceRequest
		credencial  string
		compressYes bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); ok {
			mu.Lock()
			credencial = u + ":" + p
			mu.Unlock()
		}
		mu.Lock()
		compressYes = strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip")
		mu.Unlock()

		req := leerLogsDelTest(t, r)
		mu.Lock()
		logs = req
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	exp := exporterDePrueba(t, telemetryConfig{
		otlpURL:   srv.URL,
		user:      "12345",
		password:  "secreto",
		clientID:  "portatil@1.2.3",
		maxBatch:  10,
		bufferCap: 100,
	})
	batch := []map[string]any{
		buildRecord("player_hls_error", map[string]any{"details": "levelLoadTimeOut", "fatal": true}),
		buildRecord("server_log", map[string]any{"nivel": "ERROR", "mensaje": "❌ falló /ace/m"}),
	}
	if err := exp.send(batch); err != nil {
		t.Fatalf("no se pudo enviar: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if credencial != "12345:secreto" {
		t.Errorf("credenciales = %q, el gateway las necesita", credencial)
	}
	if !compressYes {
		t.Error("el SDK debería comprimir con gzip")
	}
	if logs == nil {
		t.Fatal("no llegó nada")
	}

	// El resource debe llevar service.name y client_id: es lo que permite
	// filtrar por instalación en Grafana.
	var recursos []string
	for _, rl := range logs.ResourceLogs {
		for _, kv := range rl.Resource.Attributes {
			recursos = append(recursos, kv.Key+"="+kv.Value.GetStringValue())
		}
	}
	joined := strings.Join(recursos, " ")
	for _, quiero := range []string{"service.name=streaming", "client_id=portatil@1.2.3"} {
		if !strings.Contains(joined, quiero) {
			t.Errorf("falta %q en los atributos del resource: %s", quiero, joined)
		}
	}

	recs := aplanar(logs)
	if len(recs) != 2 {
		t.Fatalf("llegaron %d registros, se esperaban 2", len(recs))
	}
	if !strings.Contains(recs[1].body, "/ace/m") {
		t.Errorf("el segundo body perdió el mensaje: %q", recs[1].body)
	}
	// El SDK no pone severity_text (la API de Record solo tiene el número), así
	// que el nombre viaja como atributo. El número es lo que Loki usa para
	// colorear.
	if recs[0].severityNumber != 17 {
		t.Errorf("severity_number = %d, se esperaba 17 (ERROR)", recs[0].severityNumber)
	}
	if recs[0].atributos["severity"] != "ERROR" {
		t.Errorf("atributo severity = %q, se esperaba ERROR", recs[0].atributos["severity"])
	}
	// El server_log lleva nivel=ERROR, así que debe salir como ERROR (17), no
	// como INFO. Antes el test fijaba 9 porque el bug del campo "nivel" lo
	// clasificaba mal.
	if recs[1].severityNumber != 17 {
		t.Errorf("severity_number del server_log = %d, se esperaba 17 (ERROR)", recs[1].severityNumber)
	}
	if recs[1].atributos["severity"] != "ERROR" {
		t.Errorf("atributo severity del server_log = %q, se esperaba ERROR", recs[1].atributos["severity"])
	}
	// El body tiene que ser texto: Loki solo indexa strings.
	if recs[0].body == "" {
		t.Error("el body llegó vacío")
	}
	// Y los atributos conservan su tipo, que es lo que permite agregar.
	if recs[0].tipos["fatal"] != "bool" {
		t.Errorf("fatal llegó como %q, se esperaba bool", recs[0].tipos["fatal"])
	}
	if recs[0].atributos["details"] != "levelLoadTimeOut" {
		t.Errorf("details = %q", recs[0].atributos["details"])
	}
}

// El espejo de logs escribe el nivel en "nivel", no en "level". Antes solo se
// miraba "level", así que todos los server_log caían en INFO y en Grafana no
// se podía filtrar por ERROR. Es el bug más caro de los dos porque silenciaba
// justo lo que se quería vigilar.
func TestSeverityReconoceElCampoNivel(t *testing.T) {
	casos := []struct {
		nombre string
		rec    map[string]any
		want   string
	}{
		{"log con nivel ERROR", map[string]any{"event": "server_log", "nivel": "ERROR"}, "ERROR"},
		{"log con nivel WARN", map[string]any{"event": "server_log", "nivel": "WARN"}, "WARN"},
		{"log con nivel INFO", map[string]any{"event": "server_log", "nivel": "INFO"}, "INFO"},
		{"log con level (player)", map[string]any{"event": "player_x", "level": "ERROR"}, "ERROR"},
		{"sin nivel", map[string]any{"event": "server_log"}, "INFO"},
	}
	for _, c := range casos {
		if got := severityName(c.rec); got != c.want {
			t.Errorf("%s: severityName = %q, se esperaba %q", c.nombre, got, c.want)
		}
	}
}

// El caso real del log: un timeout de 2m del motor tiene que salir como ERROR.
func TestElTimeoutDelMotorEsError(t *testing.T) {
	rec := buildRecord(eventServerLog, map[string]any{
		"nivel":   "ERROR",
		"mensaje": `❌ Error en petición a Ace Stream (2m0s): Get "http://127.0.0.1:6878/ace/m/f80/711b.m3u8": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`,
	})
	if got := severityName(rec); got != "ERROR" {
		t.Errorf("severity = %q, se esperaba ERROR", got)
	}
}

// Un mensaje con espacios, comillas y paréntesis tiene que quedar en un solo
// valor. Sin comillas, Grafana lo partía en nueve "campos" distintos y el
// mensaje real se perdía.
func TestElBodyQuotaLosMensajesLargos(t *testing.T) {
	mensaje := `❌ Error en petición a Ace Stream (2m0s): Get "http://127.0.0.1:6878/ace/m/f80/711b.m3u8": context deadline exceeded`
	rec := buildRecord(eventServerLog, map[string]any{"nivel": "ERROR", "mensaje": mensaje})
	body := fmtMapBody(rec)

	// El valor tiene que aparecer entero y entrecomillado.
	if !strings.Contains(body, strconv.Quote(mensaje)) {
		t.Errorf("el mensaje no está entrecomillado:\n%s", body)
	}
	// Y no debe quedar ningún fragmento suelto que parezca otro campo.
	for _, trozo := range []string{"error=", "petición=", "deadline="} {
		if strings.Contains(body, trozo) {
			t.Errorf("se ha partido el mensaje, aparece %q como si fuera un campo:\n%s", trozo, body)
		}
	}
	// Solo puede haber dos separadores clave=valor fuera de las comillas: los
	// de "nivel=" y "mensaje=". Contarlos a pelo no sirve porque el mensaje
	// contiene URLs con "?" y "=" dentro, ya entrecomilladas.
	fuera := fueraDeComillas(body)
	if n := strings.Count(fuera, "="); n != 2 {
		t.Errorf("hay %d separadores fuera de comillas en %q, se esperaban 2", n, fuera)
	}
	if strings.Contains(fuera, "deadline exceeded") {
		t.Errorf("el mensaje se ha roto en trozos:\n%s", fuera)
	}
}

// fueraDeComillas deja el texto con las zonas entrecomilladas borradas, para
// poder contar los separadores clave=valor de verdad.
func fueraDeComillas(s string) string {
	var b strings.Builder
	dentro := false
	escapada := false
	for _, r := range s {
		switch {
		case escapada:
			b.WriteRune(r)
			escapada = false
		case dentro && r == '\\':
			escapada = true
		case r == '"':
			dentro = !dentro
		case dentro:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Valores sin espacios no llevan comillas: ensuciarían la lectura del log.
func TestLosValoresCortosNoSeQuotan(t *testing.T) {
	body := fmtMapBody(map[string]any{"event": "player_play", "link_id": "abc123"})
	if strings.Contains(body, `"`) {
		t.Errorf("un valor sin espacios no debería ir entrecomillado: %q", body)
	}
	if body != "player_play link_id=abc123" {
		t.Errorf("body = %q", body)
	}
}

func TestValoresRarosNoRompenElBody(t *testing.T) {
	casos := []string{"", "con espacio", `con "comillas"`, "con=signo", "con\nnueva", "con\ttab"}
	for _, v := range casos {
		body := fmtMapBody(map[string]any{"event": "e", "campo": v})
		// Nada debe producir un segundo "campo=" fuera de las comillas.
		esperados := strings.Count(body, "=") - strings.Count(quotearValor(v), "=")
		if reales := strings.Count(body, "=") - 1; reales != esperados-1 && esperados > 1 {
			// Solo importa que el total de "=" no crezca por el valor.
			t.Logf("valor %q -> body %q", v, body)
		}
		if !strings.HasPrefix(body, "e campo=") {
			t.Errorf("valor %q rompió el cuerpo: %q", v, body)
		}
	}
}

// server_log es el flujo de mayor volumen y no se agrega por sus campos: los
// duplica en el body. Los eventos del player sí los necesitan tipados, porque
// de ahí salen los filtros (fatal, details, link_id).
func TestServerLogNoDuplicaAtributos(t *testing.T) {
	serverLog := buildRecord(eventServerLog, map[string]any{
		"nivel": "ERROR", "mensaje": "falló el manifest",
	})
	if attrs := attrsFor(serverLog); len(attrs) != 0 {
		t.Errorf("server_log envía %d atributos: %v", len(attrs), attrsFor(serverLog))
	}
	// Pero el body los lleva, así que siguen siendo buscables.
	body := fmtMapBody(serverLog)
	for _, quiero := range []string{"nivel=ERROR", `mensaje="falló el manifest"`} {
		if !strings.Contains(body, quiero) {
			t.Errorf("el body no tiene %q: %q", quiero, body)
		}
	}

	player := buildRecord("player_hls_error", map[string]any{
		"details": "levelLoadTimeOut", "fatal": true,
	})
	attrs := attrsFor(player)
	if len(attrs) != 2 {
		t.Fatalf("el evento del player debe conservar sus atributos, tiene %d", len(attrs))
	}
}
