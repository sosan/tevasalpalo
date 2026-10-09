package main

import (
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIsDeadHost(t *testing.T) {
	dead := []string{
		"torrentstream.org",
		"TORRENTSTREAM.ORG",
		"router.acestream.me",
		"cdn.torrentstream.org",
		"54.36.163.2",
		"tracker.TorrentStream.org:8080",
	}
	for _, h := range dead {
		if !isDeadHost(h) {
			t.Errorf("isDeadHost(%q) = false, se esperaba true", h)
		}
	}
	alive := []string{
		"",
		"example.com",
		"nottorrentstream.org.example.com",
		"acestream.me",
		"retrouter.acestream.me.example.org",
	}
	for _, h := range alive {
		if isDeadHost(h) {
			t.Errorf("isDeadHost(%q) = true, se esperaba false", h)
		}
	}
}

func TestIsDeadPath(t *testing.T) {
	dead := []string{
		"/vast.php",
		"/adserver/vast/wrapper?token=1",
		"/api/v1/notification",
		"/VAST.PHP?id=9",
	}
	for _, p := range dead {
		if !isDeadPath(p) {
			t.Errorf("isDeadPath(%q) = false, se esperaba true", p)
		}
	}
	for _, p := range []string{"/playlist.m3u8", "/webui/api/service", ""} {
		if isDeadPath(p) {
			t.Errorf("isDeadPath(%q) = true, se esperaba false", p)
		}
	}
}

func TestHostOnly(t *testing.T) {
	cases := map[string]string{
		"example.com:8080": "example.com",
		"example.com":      "example.com",
		"1.2.3.4:443":      "1.2.3.4",
		"":                 "",
	}
	for in, want := range cases {
		if got := hostOnly(in); got != want {
			t.Errorf("hostOnly(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

// TestBypassProxyDeadHostCONNECT comprueba que un CONNECT a un host muerto
// devuelve 502 al instante, sin intentar ni un DNS.
func TestBypassProxyDeadHostCONNECT(t *testing.T) {
	ln, addr := startTestBypass(t)
	defer ln.Close()

	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	_ = client
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "CONNECT torrentstream.org:80 HTTP/1.1\r\nHost: torrentstream.org:80\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	n, _ := conn.Read(buf)
	resp := string(buf[:n])
	if !strings.Contains(resp, "502") {
		t.Errorf("CONNECT a host muerto devolvió %q, se esperaba 502", resp)
	}
}

// TestBypassProxyDeadHostPlainHTTP comprueba el bloqueo en peticiones en claro.
func TestBypassProxyDeadHostPlainHTTP(t *testing.T) {
	srv, addr := startTestBypass(t)
	_ = srv

	proxy := &http.Client{Transport: &http.Transport{Proxy: proxyURL(t, addr)}}
	resp, err := proxy.Get("http://router.acestream.me/announce")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, se esperaba 502", resp.StatusCode)
	}
}

// TestBypassProxyVASTEmpty verifica que una ruta de VAST devuelve un documento
// XML válido y sin creatividades, que es lo que evita el bloqueo de 14 s.
func TestBypassProxyVASTEmpty(t *testing.T) {
	_, addr := startTestBypass(t)

	proxy := &http.Client{Transport: &http.Transport{Proxy: proxyURL(t, addr)}}
	resp, err := proxy.Get("http://ads.acestream.net/vast.php?id=7")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("Content-Type = %q, se esperaba xml", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("cuerpo VAST vacío")
	}
	var doc struct {
		XMLName xml.Name `xml:"VAST"`
		Version string   `xml:"version,attr"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("el VAST de respuesta no es XML válido: %v (cuerpo %q)", err, body)
	}
	if doc.XMLName.Local != "VAST" {
		t.Errorf("raíz XML = %q, se esperaba VAST", doc.XMLName.Local)
	}
	if strings.Contains(strings.ToLower(string(body)), "<ad") && !strings.Contains(string(body), "<Ad></Ad>") {
		t.Errorf("el VAST de respuesta contiene creatividades: %q", body)
	}
}

// TestBypassProxyPassthrough comprueba que un host vivo llega a su upstream y
// que el tráfico no permitido no se corta.
func TestBypassProxyPassthrough(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		io.WriteString(w, "contenido real")
	}))
	defer upstream.Close()

	_, addr := startTestBypass(t)
	proxy := &http.Client{Transport: &http.Transport{Proxy: proxyURL(t, addr)}}
	resp, err := proxy.Get(upstream.URL + "/playlist.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "contenido real" {
		t.Errorf("cuerpo = %q, se esperaba el del upstream", body)
	}
	if gotPath != "/playlist.m3u8" {
		t.Errorf("upstream recibió %q", gotPath)
	}
}

// TestBypassProxyConnectTunnel comprueba el túnel CONNECT de extremo a extremo:
// el destino real recibe la petición y la respuesta vuelve intacta.
func TestBypassProxyConnectTunnel(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "tunel ok")
	}))
	defer upstream.Close()

	_, addr := startTestBypass(t)
	// upstream.Client() trae el TLSClientConfig del cert de test; hay que
	// conservar ese config y solo cambiar el proxy.
	tr := upstream.Client().Transport.(*http.Transport).Clone()
	tr.Proxy = proxyURL(t, addr)
	resp, err := (&http.Client{Transport: tr}).Get(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tunel ok" {
		t.Errorf("cuerpo por CONNECT = %q", body)
	}
}

func TestEngineProxyEnv(t *testing.T) {
	env := engineProxyEnv("127.0.0.1:18080")
	want := map[string]string{
		"HTTP_PROXY":  "127.0.0.1:18080",
		"http_proxy":  "127.0.0.1:18080",
		"HTTPS_PROXY": "127.0.0.1:18080",
		"https_proxy": "127.0.0.1:18080",
	}
	got := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			got[k] = v
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, se esperaba %q", k, got[k], v)
		}
	}
	if len(env) != len(want) {
		t.Errorf("engineProxyEnv devolvió %d vars, se esperaban %d: %v", len(env), len(want), env)
	}
}

// startTestBypass levanta el proxy en un puerto libre y devuelve listener y
// dirección para los tests.
func startTestBypass(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := &bypassHandler{dial: (&net.Dialer{}).Dial}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln, ln.Addr().String()
}

func proxyURL(t *testing.T, addr string) func(*http.Request) (*url.URL, error) {
	t.Helper()
	u, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	return http.ProxyURL(u)
}
