package app

// Tests del acceso al motor AceStream (local/remoto). Sin red real: solo
// puertos cerrados en localhost (rechazo inmediato) y reescritura pura.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcestreamAPI(t *testing.T) {
	// El camino de env es determinista y no toca red; el caso de env vacía
	// (descubrimiento) se prueba en TestBaseEngine*, porque cachea con
	// sync.Once y sondea puertos reales.
	t.Setenv("ACESTREAM_API", "http://vps.example:6878/")
	if got := acestreamAPI(); got != "http://vps.example:6878" {
		t.Fatalf("override con barra = %q", got)
	}
	t.Setenv("ACESTREAM_API", "http://vps.example:6878")
	if got := acestreamAPI(); got != "http://vps.example:6878" {
		t.Fatalf("override limpio = %q", got)
	}
	t.Setenv("ACESTREAM_API", "   ")
	if got := acestreamAPI(); got != fmt.Sprintf("http://127.0.0.1:%d", httpPort) || strings.Contains(got, "vps") {
		// Solo espacios = vacía: cae en discovery/cache, que nunca devuelve
		// una URL heredada de otra variable. Basta con que empiece por
		// http:// y no arrastre el valor anterior.
		t.Fatalf("espacios deben tratarse como vacío, got %q", got)
	}
}

func TestAcestreamEngineHosts(t *testing.T) {
	t.Setenv("ACESTREAM_API", "http://127.0.0.1:6878")
	hosts := acestreamEngineHosts()
	if len(hosts) != 1 || hosts[0] != "127.0.0.1:6878" {
		t.Fatalf("mismo host no se duplica = %v", hosts)
	}
	t.Setenv("ACESTREAM_API", "http://vps.example:6878")
	hosts = acestreamEngineHosts()
	if len(hosts) != 2 || hosts[1] != "vps.example:6878" {
		t.Fatalf("remoto = %v", hosts)
	}
}

// Prioridad del descubrimiento con ACESTREAM_API vacía. Sondeos con fakes:
// nada de red ni /proc real.

func TestBaseEnginePriorizaLoopback(t *testing.T) {
	sondeo := func(host string, _ int) bool { return true } // todo responde
	gateway := func() (string, bool) { return "172.17.0.1", true }
	if got := baseEngineConSondeo(httpPort, sondeo, gateway); got != "http://127.0.0.1:6878" {
		t.Fatalf("con todo disponible debe quedar en loopback, got %q", got)
	}
}

func TestBaseEngineRecaeEnDockerHost(t *testing.T) {
	sondeo := func(host string, _ int) bool { return host == "host.docker.internal" }
	gateway := func() (string, bool) { return "172.17.0.1", true }
	if got := baseEngineConSondeo(httpPort, sondeo, gateway); got != "http://host.docker.internal:6878" {
		t.Fatalf("host.docker.internal vivo debe ganar al gateway, got %q", got)
	}
}

func TestBaseEngineUsaGateway(t *testing.T) {
	// Caso dockerizado real: loopback y host.docker.internal no responden,
	// y en el gateway (el host) está publicado el 6878 del engine.
	sondeo := func(host string, _ int) bool { return host == "172.17.0.1" }
	gateway := func() (string, bool) { return "172.17.0.1", true }
	if got := baseEngineConSondeo(httpPort, sondeo, gateway); got != "http://172.17.0.1:6878" {
		t.Fatalf("gateway vivo debe usarse, got %q", got)
	}
}

func TestBaseEngineFallbackLoopback(t *testing.T) {
	// Nada responde: loopback, como antes del descubrimiento.
	sondeo := func(string, int) bool { return false }
	gateway := func() (string, bool) { return "172.17.0.1", true }
	if got := baseEngineConSondeo(httpPort, sondeo, gateway); got != "http://127.0.0.1:6878" {
		t.Fatalf("nada vivo debe volver a loopback, got %q", got)
	}
	// Sin /proc (Windows/macOS nativo): mismo resultado.
	sinProc := func() (string, bool) { return "", false }
	if got := baseEngineConSondeo(httpPort, sondeo, sinProc); got != "http://127.0.0.1:6878" {
		t.Fatalf("sin fichero de rutas debe volver a loopback, got %q", got)
	}
}

func TestBaseEngineSinGatewayValido(t *testing.T) {
	// gateway() devolviendo vacío añade solo los dos primeros candidatos.
	sondeo := func(host string, _ int) bool { return host == "172.17.0.1" }
	gateway := func() (string, bool) { return "", false }
	if got := baseEngineConSondeo(httpPort, sondeo, gateway); got != "http://127.0.0.1:6878" {
		t.Fatalf("el gateway muerto no debe inventarse candidatos, got %q", got)
	}
}

func TestGatewayFromRouteFile(t *testing.T) {
	dir := t.TempDir()
	casos := []struct {
		nombre string
		texto  string
		want   string
		ok     bool
	}{
		{
			nombre: "docker bridge por defecto",
			texto: "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT   \n" +
				"eth0\t00000000\t010011AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
				"eth0\t000011AC\t00000000\t0003\t0\t0\t0\t000F0000\t0\t0\t0\n",
			want: "172.17.0.1",
			ok:   true,
		},
		{
			nombre: "primera ruta por defecto gana",
			texto: "Iface\tDestination\tGateway\tFlags\n" +
				"eth0\t00000000\t010011AC\t0003\n" +
				"eth0\t00000000\t01AA11AC\t0003\n",
			want: "172.17.0.1",
			ok:   true,
		},
		{
			nombre: "sin ruta por defecto",
			texto: "Iface\tDestination\tGateway\n" +
				"eth0\t000011AC\t010011AC\n",
			ok: false,
		},
		{
			nombre: "gateway no hex",
			texto: "Iface\tDestination\tGateway\n" +
				"eth0\t00000000\tZZZZZZZZ\n",
			ok: false,
		},
	}
	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			ruta := filepath.Join(dir, tc.nombre)
			if err := os.WriteFile(ruta, []byte(tc.texto), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok := gatewayFromRouteFile(ruta)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.ok, got)
			}
			if tc.ok && got != tc.want {
				t.Fatalf("gateway = %q, want %q", got, tc.want)
			}
		})
	}
	// Fichero inexistente (Windows/macOS): sin gateway, sin error fatal.
	if _, ok := gatewayFromRouteFile(filepath.Join(dir, "no-existe")); ok {
		t.Fatal("fichero inexistente debe dar false")
	}
}

// TestEngineLocalDirecto: loopback, host.docker.internal y la IP del gateway
// del contenedor van siempre directas al engine; un remoto configurado a mano
// sigue por la cadena proxy.
func TestEsEngineLocalDirecto(t *testing.T) {
	ponGatewayCache(escFixtureRoute(t, "Iface\tDestination\tGateway\neth0\t00000000\t010011AC\t0003\n"))

	casos := map[string]bool{
		"http://127.0.0.1:6878/ace/x":                    true,
		"http://localhost:6878/ace/x":                    true,
		"http://host.docker.internal:6878/ace/x":         true,
		"http://172.17.0.1:6878/ace/x":                   true,
		"http://HOST.DOCKER.INTERNAL:6878/ace/x":         true,
		"http://vps.example:6878/ace/x":                  false,
		"http://192.168.1.50:6878/ace/x":                 false, // LAN pero no el gateway
		"http://10.0.0.5:6878/ace/x":                     false,
		"http://172.17.0.1:6878/ace/manifest.m3u8?id=ab": true,
	}
	for raw, want := range casos {
		if got := esEngineLocalDirecto(raw); got != want {
			t.Errorf("esEngineLocalDirecto(%q) = %v, want %v", raw, got, want)
		}
	}
}

// TestGatewayIPCacheada: la decisión direct/proxy lee el gateway en cada
// salto, así que el valor se resuelve una sola vez.
func TestGatewayIPCacheada(t *testing.T) {
	ponGatewayCache(escFixtureRoute(t, "Iface\tDestination\tGateway\neth0\t00000000\t010011AC\t0003\n"))
	if gw, ok := gatewayIPPorDefecto(); !ok || gw != "172.17.0.1" {
		t.Fatalf("gateway cacheado = %q ok=%v, want 172.17.0.1 con el fixture", gw, ok)
	}
}

// escFixtureRoute escribe un /proc/net/route de juguete en un temp dir.
func escFixtureRoute(t *testing.T, texto string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "route")
	if err := os.WriteFile(ruta, []byte(texto), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// ponGatewayCache fuerza el caché del gateway con el fichero dado, para que
// los tests no dependan ni del /proc real ni del orden de ejecución. Las
// variables del caché son del paquete y las escrituras aquí son las que
// lee gatewayIPPorDefecto a partir de entonces.
func ponGatewayCache(rutaFichero string) {
	gatewayIPOnce.Do(func() {}) // consume el Once; la sobrescritura manda
	gatewayIPCache, gatewayIPOK = gatewayFromRouteFile(rutaFichero)
}

// Detalle del formato little-endian de /proc/net/route: el gateway es un
// dword hex con los bytes de la IP en orden inverso.
func TestGatewayLittleEndianConocido(t *testing.T) {
	// 192.168.65.7 (Docker Desktop en macOS) = hex big-endian C0A84107 ->
	// little-endian 0741A8C0.
	got, ok := gatewayFromRouteFile(escFixtureRoute(t, "Iface\tDestination\tGateway\neth0\t00000000\t0741A8C0\t0003\n"))
	if !ok || got != "192.168.65.7" {
		t.Fatalf("gateway = %q ok=%v, want 192.168.65.7", got, ok)
	}
}

func TestIsLoopbackURL(t *testing.T) {
	cases := map[string]bool{
		"http://127.0.0.1:6878/ace/x":    true,
		"http://localhost:6878/ace/x":    true,
		"http://[::1]:6878/ace/x":        true,
		"http://192.168.1.10:6878/ace/x": false,
		"http://vps.example:6878/ace/x":  false,
		"://rota":                        false,
	}
	for raw, want := range cases {
		if got := isLoopbackURL(raw); got != want {
			t.Errorf("isLoopbackURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestRewriteAceManifestLine(t *testing.T) {
	local, origin := "127.0.0.1:3000", "http://127.0.0.1:3000"
	defHosts := []string{"127.0.0.1:6878"}

	if got := rewriteAceManifestLine("http://127.0.0.1:6878/ace/c/xxx/0.ts", origin, local, defHosts); got != "http://127.0.0.1:3000/ace/c/xxx/0.ts" {
		t.Fatalf("engine local = %q", got)
	}
	remHosts := []string{"127.0.0.1:6878", "vps.example:6878"}
	if got := rewriteAceManifestLine("http://vps.example:6878/ace/c/xxx/0.ts", origin, local, remHosts); got != "http://127.0.0.1:3000/ace/c/xxx/0.ts" {
		t.Fatalf("engine remoto = %q", got)
	}
	if got := rewriteAceManifestLine("http://vps.example:6878/ace/getstream?id=abc", origin, local, remHosts); got != "http://127.0.0.1:3000/ace/getstream?id=abc" {
		t.Fatalf("getstream remoto = %q", got)
	}
	if got := rewriteAceManifestLine("seg-12.ts", origin, local, defHosts); got != "http://127.0.0.1:3000/ace/seg-12.ts" {
		t.Fatalf("relativo = %q", got)
	}
	if got := rewriteAceManifestLine("#EXTM3U", origin, local, defHosts); got != "#EXTM3U" {
		t.Fatalf("comentario = %q", got)
	}
	if got := rewriteAceManifestLine("   ", origin, local, defHosts); got != "   " {
		t.Fatalf("vacía = %q", got)
	}
	if got := rewriteAceManifestLine("https://cdn.example.com/a.ts", origin, local, defHosts); got != "https://cdn.example.com/a.ts" {
		t.Fatalf("absoluta ajena = %q", got)
	}
}

func aceTestDo(target string) aceReqFunc {
	return func(u string, c *http.Client) (*http.Response, error) {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		return c.Do(req)
	}
}

func aceFakeDo(calls *int, failFirst bool) aceReqFunc {
	return func(string, *http.Client) (*http.Response, error) {
		*calls++
		if failFirst && *calls == 1 {
			return nil, errAceTestBoom
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}
}

var errAceTestBoom = errors.New("boom")

func TestAceDoRespectsAllowProxy(t *testing.T) {
	// Remoto con allowProxy=false: directa aunque haya cadena (do simulado).
	calls := 0
	resp, via, err := aceDo("http://203.0.113.10/ace/x", false, aceFakeDo(&calls, false))
	if err != nil || via != "direct" || calls != 1 {
		t.Fatalf("sin proxy debe ir directo, got via=%q calls=%d err=%v", via, calls, err)
	}
	resp.Body.Close()
}

func TestProxyMediaFlag(t *testing.T) {
	t.Setenv("PROXY_MEDIA", "")
	setProxyMedia(false)
	if proxyMediaEnabled() {
		t.Fatal("defecto OFF")
	}
	setProxyMedia(true)
	if !proxyMediaEnabled() {
		t.Fatal("tras set(true) debe estar ON")
	}
	setProxyMedia(false)
}

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("PROXY_MEDIA", "1")
	setProxyMedia(false)
	// Nota: el Once ya corrió en este proceso; parseBoolEnv se prueba directo.
	for in, want := range map[string]bool{"1": true, "true": true, "YES": true, "on": true, "0": false, "off": false, "banana": false} {
		t.Setenv("T_FOO_X", in)
		if got := parseBoolEnv("T_FOO_X", false); got != want {
			t.Errorf("parseBoolEnv(%q) = %v, want %v", in, got, want)
		}
	}
	t.Setenv("T_FOO_X", "")
	if got := parseBoolEnv("T_FOO_X", true); !got {
		t.Error("vacío debe dar el defecto")
	}
}

func TestMediaTransports(t *testing.T) {
	setProxyMedia(false)
	if got := mediaTransports(); len(got) != 1 || got[0] != "direct" {
		t.Fatalf("OFF = %v", got)
	}
	setProxyMedia(true)
	t.Setenv("PROXY_MODE", "off")
	if got := mediaTransports(); len(got) != 1 || got[0] != "direct" {
		t.Fatalf("ON pero PROXY_MODE=off = %v", got)
	}
	t.Setenv("PROXY_MODE", "")
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "")
	if got := mediaTransports(); len(got) != 3 || got[0] != "tor" || got[2] != "direct" {
		t.Fatalf("ON por defecto = %v", got)
	}
	setProxyMedia(false)
}

func TestProxyChainWithDirectFallback(t *testing.T) {
	t.Setenv("PROXY_MODE", "")
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "")
	if got := proxyChainWithDirectFallback(); len(got) != 3 || got[2] != "direct" {
		t.Fatalf("añade directa = %v", got)
	}
	t.Setenv("PROXY_FALLBACK_DIRECT", "1")
	if got := proxyChainWithDirectFallback(); len(got) != 3 {
		t.Fatalf("no debe duplicar directa = %v", got)
	}
}

func TestSocksDialContext(t *testing.T) {
	if _, err := socksDialContext("ho st:1080"); err == nil {
		t.Fatal("dirección inválida debe dar error")
	}
	fn, err := socksDialContext("127.0.0.1:1")
	if err != nil || fn == nil {
		t.Fatalf("construcción válida: %v", err)
	}
}

func TestOpenUpstreamNoTransports(t *testing.T) {
	if _, _, err := openUpstream("http://127.0.0.1:1/x", "", nil); err == nil {
		t.Fatal("sin transportes debe dar error")
	}
}

func TestOpenUpstreamDirectFailsFast(t *testing.T) {
	// Directa a puerto cerrado: error inmediato sin red real.
	if _, _, err := openUpstream("http://127.0.0.1:1/x", "", []string{"direct"}); err == nil {
		t.Fatal("directa caída debe dar error")
	}
}

func TestStreamClientFor(t *testing.T) {
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:1")
	c, err := streamClientFor("direct")
	if err != nil || c.Transport == nil {
		t.Fatalf("direct: %v", err)
	}
	for _, tr := range []string{"tor", "socks5"} {
		c, err := streamClientFor(tr)
		if err != nil || c.Transport == nil {
			t.Fatalf("%s: %v", tr, err)
		}
	}
}

func TestAceDoLoopbackFailsFast(t *testing.T) {
	// Loopback con puerto cerrado: directa y error inmediato.
	if _, via, err := aceDo("http://127.0.0.1:1/ace/x", true, aceTestDo("http://127.0.0.1:1/ace/x")); err == nil || via != "" {
		t.Fatalf("loopback caído debe dar error, got via=%q err=%v", via, err)
	}
}

func TestAceDoLoopbackDirect(t *testing.T) {
	// Loopback usa directa sin tocar la cadena (do simulado, sin red).
	calls := 0
	resp, via, err := aceDo("http://127.0.0.1:6878/ace/x", true, aceFakeDo(&calls, false))
	if err != nil || via != "direct" || calls != 1 {
		t.Fatalf("loopback debe ir directo en 1 llamada, got via=%q calls=%d err=%v", via, calls, err)
	}
	resp.Body.Close()
}

func TestAceDoRotatesOnTransportError(t *testing.T) {
	// Remoto: si el primario falla, usa el segundo (do simulado, sin red).
	t.Setenv("PROXY_MODE", "socks5")
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:1")
	t.Setenv("PROXY_FALLBACK_DIRECT", "")
	calls := 0
	resp, via, err := aceDo("http://203.0.113.10/ace/x", true, aceFakeDo(&calls, true))
	if err != nil || via != "tor" || calls != 2 {
		t.Fatalf("debió rotar a tor, got via=%q calls=%d err=%v", via, calls, err)
	}
	resp.Body.Close()
}

func TestAceDoRemoteAllFail(t *testing.T) {
	// Remoto: si todo falla (proxies + directa final), error tras probar todo.
	t.Setenv("PROXY_MODE", "socks5")
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "")
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:1")
	t.Setenv("PROXY_FALLBACK_DIRECT", "")
	calls := 0
	_, _, err := aceDo("http://203.0.113.10/ace/x", true, func(string, *http.Client) (*http.Response, error) {
		calls++
		return nil, errAceTestBoom
	})
	if err == nil || calls != 3 {
		t.Fatalf("debió agotar la cadena + directa, calls=%d err=%v", calls, err)
	}
}
