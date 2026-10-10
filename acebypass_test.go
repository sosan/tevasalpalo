package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
		"torrentstream.org/x",
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

// TestIsNotifURL separa notificaciones (JSON) de VAST (XML): el APK responde
// {"notifications":[]} a unas y un VAST vacío a las otras, y mezclarlas hace
// que el bypass no sirva.
func TestIsNotifURL(t *testing.T) {
	yes := [][2]string{
		{"android.acestream.net", "/api/v1/notification"},
		{"ANDROID.ACESTREAM.NET", "/API/V1/Notification"},
	}
	for _, c := range yes {
		if !isNotifURL(c[0], c[1]) {
			t.Errorf("isNotifURL(%q, %q) = false, se esperaba true", c[0], c[1])
		}
	}
	no := [][2]string{
		{"ads.acestream.net", "/vast.php"},
		{"example.com", "/api/v1/notification"},
	}
	for _, c := range no {
		if isNotifURL(c[0], c[1]) {
			t.Errorf("isNotifURL(%q, %q) = true, se esperaba false", c[0], c[1])
		}
	}
}

// TestBypassNotifDevuelveJSON comprueba end-to-end que el endpoint de
// notificaciones recibe JSON válido, no el VAST.
func TestBypassNotifDevuelveJSON(t *testing.T) {
	_, addr := startTestBypass(t)
	client := &http.Client{Transport: &http.Transport{Proxy: proxyURL(t, addr)}}
	resp, err := client.Get("http://android.acestream.net/api/v1/notification")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, se esperaba json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var doc struct {
		Notifications []any `json:"notifications"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("la respuesta no es JSON válido: %v (cuerpo %q)", err, body)
	}
	if len(doc.Notifications) != 0 {
		t.Errorf("notifications = %v, se esperaba lista vacía", doc.Notifications)
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
	h := &bypassHandler{dial: (&net.Dialer{}).Dial, stats: &bypassStats{}}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln, ln.Addr().String()
}

func TestBypassStatsCount(t *testing.T) {
	// Los contadores son la única forma de saber en runtime si el motor pasa
	// tráfico por el proxy: si no se incrementan, HTTP_PROXY no lo está cogiendo.
	// TLS para forzar CONNECT: sobre http:// el cliente usa petición en claro.
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	stats := &bypassStats{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: &bypassHandler{dial: (&net.Dialer{}).Dial, stats: stats}}
	go srv.Serve(ln)
	defer srv.Close()
	addr := ln.Addr().String()

	client := &http.Client{Transport: &http.Transport{Proxy: proxyURL(t, addr)}}

	// 1) CONNECT a un host vivo.
	tr := upstream.Client().Transport.(*http.Transport).Clone()
	tr.Proxy = proxyURL(t, addr)
	resp, err := (&http.Client{Transport: tr}).Get(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// 2) VAST: debe sumar plain y empty_vast.
	resp2, err := client.Get("http://ads.acestream.net/vast.php")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// 3) Host muerto en claro: debe sumar dead_dns.
	resp3, err := client.Get("http://router.acestream.me/announce")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()

	got := stats.snapshot()
	if got["connect"] < 1 {
		t.Errorf("connect = %d, se esperaba >= 1", got["connect"])
	}
	if got["plain"] < 2 {
		t.Errorf("plain = %d, se esperaban >= 2 (vast y host muerto)", got["plain"])
	}
	if got["empty_vast"] < 1 {
		t.Errorf("empty_vast = %d, se esperaba >= 1", got["empty_vast"])
	}
	if got["dead_dns"] < 1 {
		t.Errorf("dead_dns = %d, se esperaba >= 1", got["dead_dns"])
	}
}

// TestBypassHandlerSinStats comprueba que un handler sin contadores no peta: los
// tests y cualquier uso futuro pueden montarlo sin stats.
func TestBypassHandlerSinStats(t *testing.T) {
	h := &bypassHandler{dial: (&net.Dialer{}).Dial}
	h.count("connect") // no debe entrar en pánico
}

// TestDohResolveOverWARP comprueba el parseo de la respuesta DoH de Cloudflare
// y que devuelve la IP A. Es la pieza que saca la resolución de DNS del ISP.
func TestDohResolveOverWARP(t *testing.T) {
	var gotName, gotAccept string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.URL.Query().Get("name")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/dns-json")
		io.WriteString(w, `{"Status":0,"Answer":[{"name":"ads.acestream.net","type":1,"TTL":60,"data":"104.21.5.7"}]}`)
	}))
	defer srv.Close()
	// El resolutor real es https://1.1.1.1/dns-query, con el certificado de
	// cloudflare-dns.com; aquí se apunta al servidor de test.
	prev, prevInsecure := warpDNSURL, warpDNSInsecure
	warpDNSURL, warpDNSInsecure = srv.URL, true
	t.Cleanup(func() { warpDNSURL, warpDNSInsecure = prev, prevInsecure })

	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	ip, err := dohResolveOverWARP(dialCtx, "ads.acestream.net")
	if err != nil {
		t.Fatalf("dohResolveOverWARP: %v", err)
	}
	if ip != "104.21.5.7" {
		t.Errorf("IP = %q, se esperaba 104.21.5.7", ip)
	}
	if gotName != "ads.acestream.net" {
		t.Errorf("name consultado = %q", gotName)
	}
	if !strings.Contains(gotAccept, "dns-json") {
		t.Errorf("Accept = %q, se esperaba dns-json", gotAccept)
	}
}

// TestDohResolveSinRespuestaA cubre el fallo: el host no resuelve por DoH y hay
// que devolver error, no una IP inventada.
func TestDohResolveSinRespuestaA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Status":3}`)
	}))
	defer srv.Close()
	prev, prevInsecure := warpDNSURL, warpDNSInsecure
	warpDNSURL, warpDNSInsecure = srv.URL, true
	t.Cleanup(func() { warpDNSURL, warpDNSInsecure = prev, prevInsecure })

	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	if _, err := dohResolveOverWARP(dialCtx, "no-existe.example"); err == nil {
		t.Fatal("se esperaba error sin respuesta A")
	}
}

func TestPortOf(t *testing.T) {
	cases := map[string]string{
		"example.com:8080": "8080",
		"example.com":      "443",
		"1.2.3.4:443":      "443",
	}
	for in, want := range cases {
		if got := portOf(in); got != want {
			t.Errorf("portOf(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

// TestBypassDialSinWarpVaDirecto comprueba que, con WARP apagado, el proxy del
// motor sale directo. Es el caso por defecto: el bypass debe funcionar sin WARP.
func TestBypassDialSinWarpVaDirecto(t *testing.T) {
	t.Setenv("WARP", "0")
	// Un servidor local al que el dial sí puede llegar.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	engineBypassStats = &bypassStats{}
	conn, err := bypassDial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("bypassDial: %v", err)
	}
	conn.Close()

	got := engineBypassStats.snapshot()
	if got["via_direct"] != 1 {
		t.Errorf("via_direct = %d, se esperaba 1", got["via_direct"])
	}
	if got["dns_doh"] != 0 {
		t.Errorf("dns_doh = %d, se esperaba 0 con WARP apagado", got["dns_doh"])
	}
	if got["dns_fail"] != 0 {
		t.Errorf("dns_fail = %d, se esperaba 0 con WARP apagado (no se intenta DoH)", got["dns_fail"])
	}
}

// TestBypassDialWarpCaidoCaeADirecto comprueba el caso que importa para no
// romper la reproducción: WARP habilitado pero el túnel aún no levantado. El
// motor debe salir directo, no quedarse sin conexión. Con un nombre de host
// agota los reintentos (dns_fail) y cae a la IP directa.
func TestBypassDialWarpCaidoCaeADirecto(t *testing.T) {
	t.Setenv("WARP", "1")
	// WARP_SOCKS_ADDR apunta a un puerto sin nada escuchando.
	t.Setenv("WARP_SOCKS_ADDR", "127.0.0.1:1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	engineBypassStats = &bypassStats{}
	// "localhost" obliga a pasar por la rama de resolución: es un nombre, no
	// una IP, así que se intenta DoH y luego se cae a directo.
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := bypassDial("tcp", net.JoinHostPort("localhost", port))
	if err != nil {
		t.Fatalf("con WARP caído el motor debe caer a directo: %v", err)
	}
	conn.Close()

	got := engineBypassStats.snapshot()
	if got["via_direct"] != 1 {
		t.Errorf("via_direct = %d, se esperaba 1 (reserva)", got["via_direct"])
	}
	if got["dns_fail"] != 1 {
		t.Errorf("dns_fail = %d, se esperaba 1 (DoH reintentado y caído)", got["dns_fail"])
	}
	if got["dns_doh"] != 0 {
		t.Errorf("dns_doh = %d, se esperaba 0 con el túnel caído", got["dns_doh"])
	}
}

// TestBypassDialConIPNoResuelve comprueba que una IP literal no dispara la
// resolución: no hay nada que preguntar y debe ir directa sin esperar.
func TestBypassDialConIPNoResuelve(t *testing.T) {
	t.Setenv("WARP", "1")
	t.Setenv("WARP_SOCKS_ADDR", "127.0.0.1:1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	engineBypassStats = &bypassStats{}
	start := time.Now()
	conn, err := bypassDial("tcp", ln.Addr().String()) // ya es host:IP
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	// Con una IP no debe agotar los dnsWaitAttempts.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("tardó %s en dialar una IP: no debería esperar a WARP", elapsed)
	}
	got := engineBypassStats.snapshot()
	if got["dns_doh"] != 0 || got["dns_fail"] != 0 {
		t.Errorf("con IP literal no debe haber intentos de DoH: %v", got)
	}
}

func proxyURL(t *testing.T, addr string) func(*http.Request) (*url.URL, error) {
	t.Helper()
	u, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	return http.ProxyURL(u)
}
