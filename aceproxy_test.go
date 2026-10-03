package main

// Tests del acceso al motor AceStream (local/remoto). Sin red real: solo
// puertos cerrados en localhost (rechazo inmediato) y reescritura pura.

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAcestreamAPI(t *testing.T) {
	t.Setenv("ACESTREAM_API", "")
	if got := acestreamAPI(); got != "http://127.0.0.1:6878" {
		t.Fatalf("defecto = %q", got)
	}
	t.Setenv("ACESTREAM_API", "http://vps.example:6878/")
	if got := acestreamAPI(); got != "http://vps.example:6878" {
		t.Fatalf("override con barra = %q", got)
	}
}

func TestAcestreamEngineHosts(t *testing.T) {
	t.Setenv("ACESTREAM_API", "")
	hosts := acestreamEngineHosts()
	if len(hosts) != 1 || hosts[0] != "127.0.0.1:6878" {
		t.Fatalf("defecto = %v", hosts)
	}
	t.Setenv("ACESTREAM_API", "http://vps.example:6878")
	hosts = acestreamEngineHosts()
	if len(hosts) != 2 || hosts[1] != "vps.example:6878" {
		t.Fatalf("remoto = %v", hosts)
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

func aceTestDo(target string) func(*http.Client) (*http.Response, error) {
	return func(c *http.Client) (*http.Response, error) {
		req, err := http.NewRequest("GET", target, nil)
		if err != nil {
			return nil, err
		}
		return c.Do(req)
	}
}

func aceFakeDo(calls *int, failFirst bool) func(*http.Client) (*http.Response, error) {
	return func(*http.Client) (*http.Response, error) {
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
	_, _, err := aceDo("http://203.0.113.10/ace/x", true, func(*http.Client) (*http.Response, error) {
		calls++
		return nil, errAceTestBoom
	})
	if err == nil || calls != 3 {
		t.Fatalf("debió agotar la cadena + directa, calls=%d err=%v", calls, err)
	}
}
