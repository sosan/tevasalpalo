package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fiber "github.com/gofiber/fiber/v3"

	"tevasalpalo/internal/ace"
	"tevasalpalo/internal/telemetry"
	"tevasalpalo/internal/transport"
)

// montarApp levanta la app de Fiber sin escuchar (nuevoServidor) para poder
// atacarla con app.Test. Los settings van a un fichero temporal para que el
// POST de /api/warp no escriba en el arbol del proyecto.
func clienteAppTest(t *testing.T) *fiber.App {
	t.Helper()
	sobrescribirSettings(t)
	ace.SetProxyMedia(false)
	t.Cleanup(func() { ace.SetProxyMedia(false) })
	return nuevoServidor()
}

func sobrescribirSettings(t *testing.T) {
	t.Helper()
	settingsPathOverride = t.TempDir() + "/settings.json"
	settingsMu.Lock()
	settingsLoaded = false
	settingsMu.Unlock()
	t.Cleanup(func() {
		settingsPathOverride = ""
		settingsMu.Lock()
		settingsLoaded = false
		settingsMu.Unlock()
	})
}

func pide(t *testing.T, aceApp *fiber.App, metodo, url string, cuerpo ...string) *http.Response {
	t.Helper()
	var req *http.Request
	var err error
	if len(cuerpo) > 0 {
		req, err = http.NewRequest(metodo, url, strings.NewReader(cuerpo[0]))
	} else {
		req, err = http.NewRequest(metodo, url, nil)
	}
	if err != nil {
		t.Fatalf("petición %s %s: %v", metodo, url, err)
	}
	// El CORS de Fiber solo refleja cabeceras si la petición trae Origin
	// (un navegador siempre lo trae); app.Test no lo pone por defecto.
	req.Header.Set("Origin", "http://web.test")
	resp, err := aceApp.Test(req, fiber.TestConfig{Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("app.Test %s %s: %v", metodo, url, err)
	}
	return resp
}

func jsonDe(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var salida map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&salida); err != nil {
		t.Fatalf("JSON no parseable (%d): %v", resp.StatusCode, err)
	}
	return salida
}

func TestSaludEndpoint(t *testing.T) {
	resp := pide(t, clienteAppTest(t), "GET", "http://web.test/healthz")
	if resp.StatusCode != 200 {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	salida := jsonDe(t, resp)
	if _, ok := salida["ok"]; !ok {
		t.Errorf("falta 'ok' en %v", salida)
	}

	resp = pide(t, clienteAppTest(t), "GET", "http://web.test/updateavailable")
	if resp.StatusCode != 200 {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	salida = jsonDe(t, resp)
	if _, ok := salida["needUpdate"]; !ok {
		t.Errorf("falta 'needUpdate' en %v", salida)
	}
}

func TestProxyMediaInterruptor(t *testing.T) {
	aceApp := clienteAppTest(t)

	resp := pide(t, aceApp, "GET", "http://web.test/api/proxy-media")
	if resp.StatusCode != 200 {
		t.Fatalf("GET estado %d", resp.StatusCode)
	}
	if habilitado := jsonDe(t, resp)["enabled"]; habilitado != false {
		t.Errorf("por defecto debe estar OFF, hay %v", habilitado)
	}

	resp = pide(t, aceApp, "POST", "http://web.test/api/proxy-media", `{"enabled":true}`)
	if resp.StatusCode != 200 {
		t.Fatalf("POST estado %d", resp.StatusCode)
	}
	if habilitado := jsonDe(t, resp)["enabled"]; habilitado != true {
		t.Errorf("tras POST no está ON: %v", habilitado)
	}
	if !ace.ProxyMediaEnabled() {
		t.Error("el interruptor runtime no cambió con el POST")
	}

	// Cuerpo corrupto y sin 'enabled': 400 siempre
	for _, cuerpo := range []string{"{mal json", `{}`} {
		resp = pide(t, aceApp, "POST", "http://web.test/api/proxy-media", cuerpo)
		if resp.StatusCode != 400 {
			t.Errorf("estado %d para cuerpo %q", resp.StatusCode, cuerpo)
		}
	}
}

func TestPlayerParametros(t *testing.T) {
	aceApp := clienteAppTest(t)

	// Faltan parámetros
	resp := pide(t, aceApp, "GET", "http://web.test/player/index.html")
	if resp.StatusCode != 400 {
		t.Fatalf("sin link: estado %d", resp.StatusCode)
	}

	// Contenido no base64
	resp = pide(t, aceApp, "GET", "http://web.test/player/index.html?link=x&content=%%%no-base64")
	if resp.StatusCode != 400 {
		t.Errorf("content corrupto: estado %d", resp.StatusCode)
	}

	// Contenido con las tres partes que espera player.html
	content := base64.StdEncoding.EncodeToString([]byte("Broad;Evento;Competición"))
	resp = pide(t, aceApp, "GET", "http://web.test/player/index.html?link=URL&content="+content)
	if resp.StatusCode != 200 {
		t.Errorf("content válido: estado %d", resp.StatusCode)
	}

	// Content vacío: ruta de la plantilla con error nil
	resp = pide(t, aceApp, "GET", "http://web.test/player/index.html?link=URL")
	if resp.StatusCode != 200 {
		t.Errorf("sin content: estado %d", resp.StatusCode)
	}
}

func TestIptvBase64Invalido(t *testing.T) {
	aceApp := clienteAppTest(t)
	// "no-es-base64" sería base64 VALIDO (a-zson letras del alfabeto). Un
	// canal corrupto de verdad lleva caracteres fuera del alfabeto, y ahi
	// el handler debe cortar con 400 y NO llegar a hacer la petición.
	resp := pide(t, aceApp, "GET", "http://web.test/api/iptv/p;***bad***")
	if resp.StatusCode != 400 {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	if err := jsonDe(t, resp)["error"]; err != "Invalid base64 URL" {
		t.Errorf("error: %v", err)
	}
}

func TestTelemetryPlayerLote(t *testing.T) {
	aceApp := clienteAppTest(t)

	// Lote válido: un evento
	resp := pide(t, aceApp, "POST", "http://web.test/api/telemetry",
		`{"events":[{"event":"player_hola","fields":{"v":1}}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	if salida := jsonDe(t, resp); salida["ok"] != true || salida["aceptados"] != float64(1) {
		t.Errorf("lote de 1: %v", salida)
	}

	// Formato suelto (cliente viejo)
	resp = pide(t, aceApp, "POST", "http://web.test/api/telemetry",
		`{"event":"player_hola","fields":{"x":"y"}}`)
	if salida := jsonDe(t, resp); salida["aceptados"] != float64(1) {
		t.Errorf("evento suelto: %v", salida)
	}

	// Solo eventos vacíos: 400
	resp = pide(t, aceApp, "POST", "http://web.test/api/telemetry",
		`{"events":[{"event":"  "}]}`)
	if resp.StatusCode != 400 {
		t.Errorf("vacíos: estado %d", resp.StatusCode)
	}

	// JSON roto: 400
	resp = pide(t, aceApp, "POST", "http://web.test/api/telemetry", `{"events":`)
	if resp.StatusCode != 400 {
		t.Errorf("json roto: estado %d", resp.StatusCode)
	}

	// Cuerpo demasiado grande: 413
	grande := strings.Repeat("x", telemetry.TelMaxBody+1024)
	resp = pide(t, aceApp, "POST", "http://web.test/api/telemetry",
		fmt.Sprintf(`{"event":"%s"}`, grande))
	if resp.StatusCode != 413 {
		t.Errorf("cuerpo gigante: estado %d", resp.StatusCode)
	}

	// Tope de eventos por lote (TelMaxEvents)
	lote := make([]string, telemetry.TelMaxEvents+10)
	for i := range lote {
		lote[i] = fmt.Sprintf(`{"event":"e%d"}`, i)
	}
	resp = pide(t, aceApp, "POST", "http://web.test/api/telemetry",
		fmt.Sprintf(`{"events":[%s]}`, strings.Join(lote, ",")))
	if salida := jsonDe(t, resp); salida["aceptados"] != float64(telemetry.TelMaxEvents) {
		t.Errorf("tope de lote: esperaba %d, hay %v", telemetry.TelMaxEvents, salida["aceptados"])
	}
}

func TestAceSegmentoSinMotor(t *testing.T) {
	t.Setenv("ACESTREAM_API", "http://127.0.0.1:1") // puerto sin nadie escuchando
	aceApp := clienteAppTest(t)

	// Segmento .ts: el engine no responde -> 502 con mensaje del proxy
	resp := pide(t, aceApp, "GET", "http://web.test/ace/c/abcdef123456/0.ts")
	if resp.StatusCode != 502 {
		t.Fatalf("estado %d, queríamos 502 (engine no disponible)", resp.StatusCode)
	}

	// Manifests anidados: NI 502 (tumbaría HLS.js), sino 204
	resp = pide(t, aceApp, "GET", "http://web.test/ace/m/abcdef123456/lista.m3u8")
	if resp.StatusCode != 204 {
		t.Errorf("manifest anidado: estado %d, queríamos 204", resp.StatusCode)
	}
}

func TestAceManifestConMotorFalso(t *testing.T) {
	// Un motor falso que devuelve un manifest con URLs de su propio host:
	// el handler las reescribe al origen de la web (el host que pide el
	// player) para que los segmentos entren por el proxy /ace/*.
	motor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\nhttp://127.0.0.1:6878/ace/c/abcdef/0.ts\n")
	}))
	defer motor.Close()
	t.Setenv("ACESTREAM_API", motor.URL)
	aceApp := clienteAppTest(t)

	resp := pide(t, aceApp, "GET", "http://web.test/ace/proxy.m3u8")
	if resp.StatusCode != 200 {
		t.Fatalf("estado %d", resp.StatusCode)
	}
	tipo := resp.Header.Get("Content-Type")
	if !strings.Contains(tipo, "mpegurl") {
		t.Errorf("Content-Type %q", tipo)
	}
	cuerpo := strDe(t, resp)
	if !strings.Contains(cuerpo, "http://web.test/ace/c/abcdef/0.ts") {
		t.Errorf("URL de segmento no reescrita al origen: %q", cuerpo)
	}
	if strings.Contains(cuerpo, "6878") {
		t.Errorf("queda una URL del motor sin reescribir: %q", cuerpo)
	}
}

// strDe lee todo el cuerpo de una respuesta de app.Test.
func strDe(t *testing.T, resp *http.Response) string {
	t.Helper()
	cuerpo, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("lectura del cuerpo: %v", err)
	}
	return string(cuerpo)
}

func TestApiLogsConToken(t *testing.T) {
	aceApp := clienteAppTest(t)

	// Sin token y sin loopback dentro de app.Test: 403
	if resp := pide(t, aceApp, "GET", "http://web.test/api/logs"); resp.StatusCode != 403 {
		t.Fatalf("sin credencial: estado %d", resp.StatusCode)
	}

	t.Setenv("TELEMETRY_API_TOKEN", "secreto-test")
	resp := pide(t, aceApp, "GET", "http://web.test/api/logs?token=secreto-test")
	if resp.StatusCode != 200 {
		t.Fatalf("con token por query: estado %d", resp.StatusCode)
	}
	salida := jsonDe(t, resp)
	if _, ok := salida["config"]; !ok {
		t.Errorf("falta 'config' en %v", salida)
	}
	resp = pide(t, aceApp, "GET", "http://web.test/api/db?que=fuentes&token=secreto-test")
	if resp.StatusCode != 200 {
		t.Fatalf("db con token: estado %d", resp.StatusCode)
	}
	if salida := jsonDe(t, resp); salida["estado"] == nil {
		t.Errorf("falta 'estado' en %v", salida)
	}
}

func TestWarpApi(t *testing.T) {
	aceApp := clienteAppTest(t)

	// Estado: json con flags
	resp := pide(t, aceApp, "GET", "http://web.test/api/warp")
	if resp.StatusCode != 200 {
		t.Fatalf("estado GET %d", resp.StatusCode)
	}

	// Guardar OFF y ver si queda persistido: settingsPathOverride apunta
	// a un fichero temporal
	resp = pide(t, aceApp, "POST", "http://web.test/api/warp", `{"enabled":false}`)
	if resp.StatusCode != 200 {
		t.Fatalf("estado POST %d", resp.StatusCode)
	}
	if !transport.WarpEnabled() {
		t.Log("nota: el entorno ya tenía warp OFF")
	}

	// Cuerpo corrupto: 400
	resp = pide(t, aceApp, "POST", "http://web.test/api/warp", `{}`)
	if resp.StatusCode != 400 {
		t.Errorf("cuerpo vacío: estado %d", resp.StatusCode)
	}
}

func TestCorsYStaticos(t *testing.T) {
	aceApp := clienteAppTest(t)

	resp := pide(t, aceApp, "GET", "http://web.test/healthz")
	if origen := resp.Header.Get("Access-Control-Allow-Origin"); origen != "*" {
		t.Errorf("CORS de /healthz: %q", origen)
	}

	// El stub de .ts responde 200
	resp = pide(t, aceApp, "GET", "http://web.test/hls/seg-1.ts")
	if resp.StatusCode != 200 {
		t.Errorf("stub /hls/: estado %d", resp.StatusCode)
	}
}
