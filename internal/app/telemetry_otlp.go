package app

// Envío asíncrono de eventos a Grafana Cloud por OTLP.
//
// El payload, la compresión, los reintentos y el transporte los pone el SDK
// oficial de OpenTelemetry (otlplog/otlploghttp). Antes esto se serializaba a
// mano y salía mal: en OTLP el body de un logRecord es un AnyValue pelado
// ({"stringValue":"..."}) mientras que los atributos son {key, value}. Esa
// confusión hacía que Loki devolviera 400 y, peor, que un body mal interpreted
// se descartara en silencio. Delegar el protocolo es lo sensato en cuanto hay
// un SDK que lo hace bien.
//
// Lo que sigue siendo nuestro:
//   - La cola en disco (spool), que sobrevive a los reinicios. El SDK no la
//     tiene en Go: su cola es en memoria.
//   - El backoff entre lotes y el volcado final al apagar.
//   - El mapeo de evento -> cuerpo legible + atributos tipados.
//
// La app nunca espera a la red: Record solo encola.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func contextWithDeadline(t time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(context.Background(), t)
}

// forceFlush empuja al provider lo que tenga pendiente sin cerrarlo. Mantener el
// provider vivo entre lotes es lo que permite seguir enviando tras un fallo.
func (e *otlpExporter) forceFlush(ctx context.Context) error {
	if e.provider == nil {
		return nil
	}
	return e.provider.ForceFlush(ctx)
}

type otlpExporter struct {
	cfg      telemetryConfig
	mu       sync.Mutex
	queue    []map[string]any
	spool    *diskSpool // cola en disco: sobrevive a los reinicios
	logger   otellog.Logger
	provider *sdklog.LoggerProvider
	// flushTimeout acota el ForceFlush. Al cerrar se baja para no pasarse del
	// deadline de apagado: si no, esperar a la red podría alargar el cierre.
	flushTimeout time.Duration
	done         chan struct{}
	closed       bool
	lastErr      string
	drops        int64
	sent         int64
	// failCount controla el backoff entre lotes.
	failCount   int
	successes   int
	lastAttempt time.Time
}

func newOTLPExporter(cfg telemetryConfig) (*otlpExporter, error) {
	e := &otlpExporter{
		cfg:          cfg,
		done:         make(chan struct{}),
		flushTimeout: 3 * time.Second,
	}

	opts, err := exporterOptions(cfg)
	if err != nil {
		return nil, err
	}
	exp, err := otlploghttp.New(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el exportador OTLP: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		"", // sin schema URL: si no, el merge falla
		attribute.String("service.name", "streaming"),
		attribute.String("service.version", telVersion),
		attribute.String("client_id", cfg.clientID),
	))
	if err != nil {
		res = resource.Default()
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
	)
	e.logger = provider.Logger("streaming")
	e.provider = provider

	// El spool es opcional: sin TELEMETRY_LOG_DIR se queda solo en memoria.
	if cfg.spoolPath != "" {
		sp, err := newDiskSpool(cfg.spoolPath, cfg.spoolMaxSize)
		if err != nil {
			log.Printf("⚠️  telemetría: sin cola en disco (%v), los eventos pendientes se perderán al reiniciar", err)
		} else {
			e.spool = sp
			if p := sp.pending(); p > 0 {
				log.Printf("📦 %d eventos pendientes de un arranque anterior, se reintentarán", p)
			}
		}
	}
	return e, nil
}

// exporterOptions arma las opciones del exporter de OpenTelemetry.
//
// Trampa importante: WithEndpointURL parte la URL en host y path, y NO añade
// la ruta de la señal. Si se le pasa el endpoint base del gateway de Grafana
// (https://otlp-gateway-prod-eu-west-0.grafana.net/otlp, que es lo que da la
// consola) el exporter iría a /otlp y Loki respondería 404. Hay que pasarle
// siempre la ruta completa /v1/logs.
//
// Además, si no hay URL configurada no se pasa ninguna opción de endpoint: así
// el SDK lee las variables estándar OTEL_EXPORTER_OTLP_* del entorno, que es lo
// que espera cualquiera que ya use OpenTelemetry.
func exporterOptions(cfg telemetryConfig) ([]otlploghttp.Option, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
		otlploghttp.WithTimeout(10 * time.Second),
	}

	if url := strings.TrimSpace(cfg.otlpURL); url != "" {
		completa, err := normalizarEndpointOTLP(url)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlploghttp.WithEndpointURL(completa))
	}

	if h := authHeaders(cfg.user, cfg.password); h != nil {
		opts = append(opts, otlploghttp.WithHeaders(h))
	}
	return opts, nil
}

// normalizarEndpointOTLP acepta el endpoint como lo da la consola de Grafana
// (base, sin la ruta de la señal) o ya completo, y devuelve siempre la URL
// final correcta. Aceptar las dos formas evita el 404 silencioso cuando alguien
// cambia de una a otra.
func normalizarEndpointOTLP(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("TELEMETRY_OTLP_URL no es una URL válida: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("TELEMETRY_OTLP_URL necesita esquema http o https: %q", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("TELEMETRY_OTLP_URL no tiene host: %q", raw)
	}

	// Sin query ni fragmento: no tiene sentido en un endpoint de ingesta y
	// el SDK los rechazaría igualmente.
	u.RawQuery = ""
	u.Fragment = ""

	const rutaLogs = "/v1/logs"
	p := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(p, rutaLogs) {
		p += rutaLogs
	}
	u.Path = p
	return u.String(), nil
}

// authHeaders construye el Basic auth que espera el gateway. Vacío si no hay
// credenciales (por ejemplo cuando el destino es un collector local).
func authHeaders(user, pass string) map[string]string {
	if user == "" && pass == "" {
		return nil
	}
	par := user + ":" + pass
	return map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(par)),
	}
}

// enqueue mete un evento. No bloquea nunca.
func (e *otlpExporter) enqueue(rec map[string]any) {
	e.mu.Lock()
	closed := e.closed
	e.mu.Unlock()
	if closed {
		return
	}

	// Si hay spool, el evento se escribe a disco y el envío posterior lo
	// recoge de ahí. Es más lento que la RAM, pero sobrevive al reinicio.
	if e.spool != nil {
		if _, err := e.spool.append(rec); err != nil {
			e.mu.Lock()
			e.lastErr = "cola en disco: " + err.Error()
			e.mu.Unlock()
		}
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	if len(e.queue) >= e.cfg.bufferCap {
		// Descarta el más antiguo para hacer hueco.
		copy(e.queue, e.queue[1:])
		e.queue = e.queue[:len(e.queue)-1]
		e.drops++
	}
	e.queue = append(e.queue, rec)
}

// run es la goroutine de envío: cada flushEvery intenta vaciar la cola.
func (e *otlpExporter) run() {
	ticker := time.NewTicker(e.cfg.flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			e.flush()
		case <-e.done:
			return
		}
	}
}

// closeWithDeadline vacía lo pendiente y para la goroutine. Se usa al apagar:
// sin esto, el evento que explica el cierre se queda en la cola.
func (e *otlpExporter) closeWithDeadline(deadline time.Time) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	for time.Now().Before(deadline) {
		e.mu.Lock()
		pendientes := e.pendientesLocked()
		e.mu.Unlock()
		if pendientes == 0 {
			break
		}
		e.mu.Lock()
		e.failCount = 0 // último intento: no respetar backoff al cerrar
		restante := time.Until(deadline)
		if restante > 0 && restante < e.flushTimeout {
			// Un ForceFlush que espera más de lo que queda prolongaría el
			// apagado; con spool los eventos no se pierden igual.
			e.flushTimeout = restante
		}
		e.mu.Unlock()
		e.flush()
		if e.pendientes() == pendientes {
			// No progresa: el central no responde. Un reintento más con
			// más espera no va a arreglarlo.
			break
		}
	}

	e.mu.Lock()
	e.closed = true
	restantes := len(e.queue)
	pendientesDisco := 0
	if e.spool != nil {
		pendientesDisco = e.spool.pending()
	}
	e.mu.Unlock()
	close(e.done)

	if e.provider != nil {
		// ForceFlush antes de cerrar: sin esto el batch processor puede
		// quedarse con los últimos eventos al apagar.
		ctx, cancel := contextWithDeadline(deadline)
		_ = e.forceFlush(ctx)
		_ = e.provider.Shutdown(ctx)
		cancel()
	}

	if restantes > 0 {
		// Solo a fichero/buffer; no es un error de la app.
		log.Printf("📊 Telemetría: %d eventos sin enviar al apagar", restantes)
	}
	if pendientesDisco > 0 {
		// Con spool no se pierde nada: quedan en disco y se mandan al
		// arrancar. Es lo contrario de un problema.
		log.Printf("📦 Telemetría: %d eventos quedan en la cola en disco, se enviarán al arrancar", pendientesDisco)
	}
}

func (e *otlpExporter) flush() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	if e.spool != nil {
		e.flushSpool()
		return
	}
	e.flushCola()
}

// flushCola es la vía rápida en memoria, para cuando no hay spool.
func (e *otlpExporter) flushCola() {
	e.mu.Lock()
	if len(e.queue) == 0 {
		e.mu.Unlock()
		return
	}
	if !e.puedeIntentarLocked() {
		e.mu.Unlock()
		return
	}
	n := len(e.queue)
	if n > e.cfg.maxBatch {
		n = e.cfg.maxBatch
	}
	batch := make([]map[string]any, n)
	copy(batch, e.queue[:n])
	e.lastAttempt = time.Now()
	e.mu.Unlock()

	if err := e.send(batch); err != nil {
		e.mu.Lock()
		e.lastErr = err.Error()
		e.failCount++
		e.mu.Unlock()
		return
	}

	e.mu.Lock()
	// Solo quita del frente lo que se envió con éxito.
	e.queue = e.queue[n:]
	e.sent += int64(n)
	e.failCount = 0
	e.successes++
	e.lastErr = ""
	e.mu.Unlock()
}

// flushSpool saca los eventos del fichero, intenta mandarlos y, si falla, los
// devuelve al principio. Así un corte de red no pierde nada.
func (e *otlpExporter) flushSpool() {
	e.mu.Lock()
	if !e.puedeIntentarLocked() {
		e.mu.Unlock()
		return
	}
	e.lastAttempt = time.Now()
	e.mu.Unlock()

	batch := e.spool.take(e.cfg.maxBatch)
	if len(batch) == 0 {
		e.mu.Lock()
		e.failCount = 0
		e.lastErr = ""
		e.mu.Unlock()
		return
	}

	if err := e.send(batch); err != nil {
		e.spool.requeue(batch)
		e.mu.Lock()
		e.lastErr = err.Error()
		e.failCount++
		e.mu.Unlock()
		return
	}

	e.mu.Lock()
	e.sent += int64(len(batch))
	e.failCount = 0
	e.successes++
	e.lastErr = ""
	e.mu.Unlock()
}

// send entrega el lote al SDK. Emitir es no bloqueante (el BatchProcessor
// encola), así que el error solo puede venir de un mal uso, no de la red: el
// SDK reintenta por su cuenta y su error se vería en los logs del provider.
func (e *otlpExporter) send(batch []map[string]any) error {
	ctx := context.Background()
	for _, rec := range batch {
		e.emit(ctx, rec)
	}
	// ForceFlush: sin él, el batch processor podría retener el lote y no se
	// contaría como enviado.
	e.mu.Lock()
	timeout := e.flushTimeout
	e.mu.Unlock()
	ctx, cancel := contextWithTimeout(timeout)
	defer cancel()
	_ = e.forceFlush(ctx)
	return nil
}

// emit traduce un registro al SDK: cuerpo legible + atributos con su tipo.
//
// El severity va dos veces a propósito. El SDK solo rellena severity_number
// (la API de Record no tiene campo de texto), así que sin el atributo extra no
// habría forma de filtrar por nombre en Grafana.
func (e *otlpExporter) emit(ctx context.Context, rec map[string]any) {
	var r otellog.Record
	r.SetTimestamp(time.Now())
	r.SetSeverity(severityFor(rec))
	r.SetBody(attribute.StringValue(fmtMapBody(rec)))
	r.AddAttributes(attribute.String("severity", severityName(rec)))
	for _, kv := range attrsFor(rec) {
		r.AddAttributes(kv)
	}
	e.logger.Emit(ctx, r)
}

// puedeIntentarLocked consulta el backoff. El llamante debe tener el lock.
func (e *otlpExporter) puedeIntentarLocked() bool {
	if e.failCount > 0 && !e.lastAttempt.IsZero() {
		if time.Since(e.lastAttempt) < backoffDuration(e.failCount) {
			return false
		}
	}
	return true
}

func backoffDuration(fails int) time.Duration {
	d := 2 * time.Second
	for i := 0; i < fails && d < 2*time.Minute; i++ {
		d *= 2
	}
	if d > 2*time.Minute {
		d = 2 * time.Minute
	}
	return d
}

// stats devuelve un resumen del exportador para /api/logs.
func (e *otlpExporter) stats() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return map[string]any{
		"queued":     e.pendientesLocked(),
		"sent":       e.sent,
		"drops":      e.drops,
		"fail_count": e.failCount,
		"successes":  e.successes,
		"last_error": e.lastErr,
	}
}

// pendientes cuenta lo que hay esperando, venga de donde venga.
func (e *otlpExporter) pendientes() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pendientesLocked()
}

// pendientesLocked cuenta la cola en memoria o el spool. El llamante tiene el
// lock de e.mu, pero el spool tiene el suyo propio.
func (e *otlpExporter) pendientesLocked() int {
	if e.spool != nil {
		return e.spool.pending()
	}
	return len(e.queue)
}

// eventBaseFields son los que el body ya repite; no tiene sentido mandarlos
// dos veces como atributos.
var eventBaseFields = map[string]bool{"time": true, "event": true, "version": true, "os": true, "arch": true, "client_id": true}

// attrsFor convierte el resto de campos en atributos de Loki para poder filtrar
// y agregar en el visor. Se conserva el tipo original (int, bool, string) en
// vez de aplanarlo todo a texto, que es donde se pierde la información al
// agregar en Grafana.
func attrsFor(rec map[string]any) []attribute.KeyValue {
	// Para server_log los campos van solo en el body. Motivo: es el flujo de
	// mayor volumen (cientos de líneas por arranque y decenas por refresco de
	// manifest) y no se agrega por ellos, se lee. Duplicarlos como structured
	// metadata solo costs la mitad del volumen de Loki sin aportar nada: los
	// campos同样 salen del body y Grafana los detecta igual.
	//
	// Los eventos del player (pocos, y sí se filtran por fatal/details/link_id)
	// sí conservan los atributos tipados.
	if fmt.Sprint(rec["event"]) == eventServerLog {
		return nil
	}

	keys := make([]string, 0, len(rec))
	for k := range rec {
		if !eventBaseFields[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys) // estable: evita ruido por orden de mapa
	out := make([]attribute.KeyValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, attributeFor(k, rec[k]))
	}
	return out
}

func attributeFor(k string, v any) attribute.KeyValue {
	switch t := v.(type) {
	case string:
		return attribute.String(k, t)
	case bool:
		return attribute.Bool(k, t)
	case float64:
		if t == float64(int64(t)) {
			return attribute.Int64(k, int64(t))
		}
		return attribute.Float64(k, t)
	case int:
		return attribute.Int64(k, int64(t))
	case int64:
		return attribute.Int64(k, t)
	default:
		return attribute.String(k, scalarString(v))
	}
}

// severityFor mapea el evento a un nivel de Loki para poder filtrar por
// ERROR/WARN en el visor.
func severityFor(rec map[string]any) otellog.Severity {
	nombre := severityName(rec)
	switch nombre {
	case "TRACE":
		return otellog.SeverityTrace
	case "DEBUG":
		return otellog.SeverityDebug
	case "WARN":
		return otellog.SeverityWarn
	case "ERROR":
		return otellog.SeverityError
	case "FATAL":
		return otellog.SeverityFatal
	default:
		return otellog.SeverityInfo
	}
}

// severityName es el nombre de la severidad, que es lo que se ve en Grafana.
func severityName(rec map[string]any) string {
	// Ojo con el nombre del campo: el espejo de logs escribe "nivel" y el
	// player puede mandar "level". Antes solo se miraba "level", así que
	// todos los server_log de error acababan como INFO: el nombre del evento
	// ("server_log") no contiene "error" y caían en el valor por defecto.
	// El resultado era que filtrar por ERROR en Grafana no sacaba ni un
	// timeout del motor.
	for _, clave := range []string{"level", "nivel", "severity"} {
		lvl, ok := rec[clave].(string)
		if !ok {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(lvl)) {
		case "ERROR", "FATAL", "CRITICAL":
			return "ERROR"
		case "WARN", "WARNING":
			return "WARN"
		case "DEBUG", "TRACE":
			return "DEBUG"
		case "INFO", "INFORMATIONAL", "NOTICE":
			// Un nivel explícito gana siempre sobre el nombre del evento: si
			// alguien dice que es INFO, es INFO aunque el evento se llame
			// player_hls_error.
			return "INFO"
		}
	}
	// Sin campo explícito, heurística por el nombre del evento.
	e := strings.ToLower(fmt.Sprint(rec["event"]))
	switch {
	case strings.Contains(e, "error"), strings.Contains(e, "fatal"):
		return "ERROR"
	case strings.Contains(e, "warn"):
		return "WARN"
	}
	return "INFO"
}

// scalarString convierte un valor a string plano para cuerpos de texto.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64, int, int64, bool:
		return fmt.Sprint(t)
	default:
		if b, err := jsonMarshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

// fmtMapBody crea el body legible del evento: "evento clave=valor clave=valor".
func fmtMapBody(rec map[string]any) string {
	var b strings.Builder
	b.WriteString(fmt.Sprint(rec["event"]))
	keys := make([]string, 0, len(rec))
	for k := range rec {
		if k != "event" && !eventBaseFields[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(" ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(quotearValor(scalarString(rec[k])))
	}
	return b.String()
}

// quotearValor entrecomilla los valores que romperían el formato clave=valor.
//
// Sin esto, un mensaje de error normal como "Error en petición a Ace Stream
// (2m0s): context deadline exceeded" se parte en nueve trozos y Grafana los
// interpreta como otros tantos campos: en vez de mensaje=... aparecía
// mensaje=❌ y luego un montón de campos basura con el resto de la frase. Los
// mensajes de error son casi siempre frases, así que el caso era el habitual y
// no la excepción.
func quotearValor(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t\r\n\"=") {
		return strconv.Quote(v)
	}
	return v
}
