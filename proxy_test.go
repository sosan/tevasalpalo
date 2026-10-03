package main

// Tests del transporte proxy configurable y la rotación automática.
// Sin internet: solo conexiones a localhost con puerto cerrado (rechazo
// inmediato) para probar el agotamiento de la cadena.

import (
	"fmt"
	"testing"
)

func TestProxyModeDefault(t *testing.T) {
	t.Setenv("PROXY_MODE", "")
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "")
	if got := proxyMode(); got != "tor" {
		t.Fatalf("proxyMode por defecto = %q, want tor", got)
	}
}

func TestProxyModeValues(t *testing.T) {
	cases := []struct{ in, want string }{
		{"tor", "tor"},
		{"SOCKS5", "socks5"},
		{" Xray ", "xray"},
		{"OFF", "off"},
		{"direct", "direct"},
	}
	for _, tc := range cases {
		t.Setenv("PROXY_MODE", tc.in)
		if got := proxyMode(); got != tc.want {
			t.Errorf("proxyMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSocks5Addr(t *testing.T) {
	t.Setenv("SOCKS5_ADDR", "")
	if got := socks5Addr(); got != "127.0.0.1:10808" {
		t.Fatalf("socks5Addr por defecto = %q", got)
	}
	t.Setenv("SOCKS5_ADDR", " 10.0.0.2:1080 ")
	if got := socks5Addr(); got != "10.0.0.2:1080" {
		t.Fatalf("socks5Addr custom = %q", got)
	}
}

func TestNewSOCKS5Client(t *testing.T) {
	c, err := newSOCKS5Client("127.0.0.1:1")
	if err != nil {
		t.Fatalf("newSOCKS5Client válido error: %v", err)
	}
	if c == nil || c.Timeout != timeTimeout {
		t.Fatal("cliente mal construido")
	}
	if _, err := newSOCKS5Client("ho st:1080"); err == nil {
		t.Fatal("dirección inválida debería dar error")
	}
	// Compat: el helper de Tor sigue existiendo y delega.
	if _, err := createSOCKS5Client(); err != nil {
		t.Fatalf("createSOCKS5Client error: %v", err)
	}
}

func TestIsFatalFetchErr(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		fatal bool
	}{
		{"nil no fatal", nil, false},
		{"404 fatal", fmt.Errorf("status code error: 404 404 Not Found"), true},
		{"403 fatal", fmt.Errorf("status code error: 403 403 Forbidden"), true},
		{"429 reintentable", fmt.Errorf("status code error: 429 429 Too Many Requests"), false},
		{"500 reintentable", fmt.Errorf("status code error: 500 500 Internal Server Error"), false},
		{"timeout reintentable", fmt.Errorf("error al realizar la solicitud HTTP: timeout"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFatalFetchErr(tc.err); got != tc.fatal {
				t.Fatalf("isFatalFetchErr(%v) = %v, want %v", tc.err, got, tc.fatal)
			}
		})
	}
}

func TestShouldProxyFallback(t *testing.T) {
	src := Source{Name: "x", URL: "http://example.com/", Type: SourceM3U, Proxied: false}
	proxied := Source{Name: "y", URL: "http://example.com/", Type: SourceM3U, Proxied: true}
	if !shouldProxyFallback(src, false, false) {
		t.Fatal("directa agotada sin fatal debe caer a proxy")
	}
	if shouldProxyFallback(src, true, false) {
		t.Fatal("éxito no necesita fallback")
	}
	if shouldProxyFallback(src, false, true) {
		t.Fatal("error fatal (404) no debe ir a proxy")
	}
	if shouldProxyFallback(proxied, false, false) {
		t.Fatal("fuente ya proxied no necesita fallback")
	}
}

func TestProxyChain(t *testing.T) {
	// Por defecto la cadena cierra en "direct" para que un xray/Tor caído
	// degrade en vez de romper; PROXY_FALLBACK_DIRECT=0 lo quita.
	cases := []struct {
		name, mode, link, sub, fallback string
		want                            []string
	}{
		{"sin nada (sub hardcodeada) -> socks5", "", "", "", "", []string{"socks5", "tor", "direct"}},
		{"todo vacío -> tor", "", "", " ", "", []string{"tor", "socks5", "direct"}},
		{"primario socks5", "socks5", "", "", "", []string{"socks5", "tor", "direct"}},
		{"alias xray", "xray", "", "", "", []string{"socks5", "tor", "direct"}},
		{"off solo directa", "off", "", "", "", []string{"direct"}},
		{"fallback directa opt-out", "", "", "", "0", []string{"socks5", "tor"}},
		{"off no duplica directa", "off", "", "", "1", []string{"direct"}},
		{"off no duplica directa (opt-out)", "off", "", "", "0", []string{"direct"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PROXY_MODE", tc.mode)
			t.Setenv("XRAY_LINK", tc.link)
			if tc.sub == " " {
				t.Setenv("XRAY_SUB", "")
			}
			t.Setenv("PROXY_FALLBACK_DIRECT", tc.fallback)
			got := proxyChain()
			if len(got) != len(tc.want) {
				t.Fatalf("proxyChain = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("proxyChain = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestClientForTransport(t *testing.T) {
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:1")
	if c, err := clientForTransport("direct"); err != nil || c.Transport != nil {
		t.Fatalf("direct = %v, %v", c, err)
	}
	if _, err := clientForTransport("tor"); err != nil {
		t.Fatalf("tor error: %v", err)
	}
	if _, err := clientForTransport("socks5"); err != nil {
		t.Fatalf("socks5 error: %v", err)
	}
}

// TestFetchWebDataRotatesAndFails: con Tor y SOCKS caídos (puertos cerrados
// en localhost, sin red), la cadena tor->socks5 debe agotarse y dar error
// sin colgarse y sin caer a directa (por defecto).
func TestFetchWebDataRotatesAndFails(t *testing.T) {
	t.Setenv("PROXY_MODE", "")
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:1")
	t.Setenv("PROXY_FALLBACK_DIRECT", "")
	if _, err := FetchWebData("http://127.0.0.1:1/", true); err == nil {
		t.Fatal("con todos los transportes caídos debe dar error")
	}
}

// TestFetchWebDataDirect: sin proxy va directa (puerto cerrado = error rápido).
func TestFetchWebDataDirect(t *testing.T) {
	if _, err := FetchWebData("http://127.0.0.1:1/", false); err == nil {
		t.Fatal("directa a puerto cerrado debe dar error")
	}
}
