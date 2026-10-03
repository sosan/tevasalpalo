package main

// Tests del sidecar xray. Todo offline con enlaces sintéticos: ninguna
// credencial real (ni las probadas manualmente) entra al repo.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

const (
	xrayTestVlessReality = "vless://11111111-2222-3333-4444-555555555555@example.com:443?security=reality&encryption=none&pbk=0123456789abcdef0123456789abcdef0123456789AB&fp=chrome&type=tcp&flow=xtls-rprx-vision&sni=example.org&sid=1a2b3c#test-reality"
	xrayTestTrojanWS     = "trojan://s3cr3t@example.net:443?path=%2Fws-path&security=tls&host=example.net&type=ws&sni=example.net#test-trojan"
	xrayTestVlessWS      = "vless://11111111-2222-3333-4444-555555555555@example.com:443?security=tls&encryption=none&host=cdn.example.com&path=%2Fsome%2Fpath&sni=cdn.example.com&fp=firefox&alpn=h2%2Chttp%2F1.1&type=ws#test-ws"
)

func TestParseXrayLinkVlessReality(t *testing.T) {
	ep, err := parseXrayLink(xrayTestVlessReality)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ep.Protocol != "vless" || ep.Address != "example.com" || ep.Port != 443 {
		t.Fatalf("endpoint = %+v", ep)
	}
	if ep.ID != "11111111-2222-3333-4444-555555555555" || ep.Flow != "xtls-rprx-vision" {
		t.Fatalf("credenciales = %+v", ep)
	}
	if ep.Security != "reality" || ep.Network != "tcp" || ep.PBK == "" || ep.SID != "1a2b3c" || ep.SNI != "example.org" || ep.FP != "chrome" {
		t.Fatalf("stream = %+v", ep)
	}
}

func TestParseXrayLinkTrojanWS(t *testing.T) {
	ep, err := parseXrayLink(xrayTestTrojanWS)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ep.Protocol != "trojan" || ep.Password != "s3cr3t" || ep.Address != "example.net" || ep.Port != 443 {
		t.Fatalf("endpoint = %+v", ep)
	}
	if ep.Security != "tls" || ep.Network != "ws" || ep.Path != "/ws-path" || ep.Host != "example.net" || ep.SNI != "example.net" {
		t.Fatalf("stream = %+v", ep)
	}
}

func TestParseXrayLinkVlessWSAlpn(t *testing.T) {
	ep, err := parseXrayLink(xrayTestVlessWS)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(ep.ALPN) != 2 || ep.ALPN[0] != "h2" || ep.ALPN[1] != "http/1.1" {
		t.Fatalf("alpn = %v", ep.ALPN)
	}
	if ep.Path != "/some/path" || ep.FP != "firefox" {
		t.Fatalf("stream = %+v", ep)
	}
}

func TestParseXrayLinkSS(t *testing.T) {
	sip002 := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:password123")) + "@example.org:8388#test-ss"
	ep, err := parseXrayLink(sip002)
	if err != nil {
		t.Fatalf("SIP002 error: %v", err)
	}
	if ep.Protocol != "shadowsocks" || ep.Method != "aes-256-gcm" || ep.Password != "password123" || ep.Address != "example.org" || ep.Port != 8388 {
		t.Fatalf("endpoint = %+v", ep)
	}

	legacy := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:secret@mail.example:9999")) + "#test-legacy"
	ep, err = parseXrayLink(legacy)
	if err != nil {
		t.Fatalf("legacy error: %v", err)
	}
	if ep.Method != "chacha20-ietf-poly1305" || ep.Password != "secret" || ep.Address != "mail.example" || ep.Port != 9999 {
		t.Fatalf("endpoint = %+v", ep)
	}
}

func TestParseXrayLinkErrors(t *testing.T) {
	badSIP002 := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:password123")) + "@example.org:8388?plugin=obfs-local%3Bobfs%3Dhttp#x"
	cases := map[string]string{
		"vacío":           "",
		"basura":          "not-a-link",
		"hysteria2":       "hysteria2://pass@host.example:443?sni=x#y",
		"vmess":           "vmess://eyJhZGQiOiIxLjIuMy40In0=",
		"vless sin uuid":  "vless://@example.com:443",
		"reality sin pbk": "vless://11111111-2222-3333-4444-555555555555@example.com:443?security=reality&type=tcp#x",
		"red desconocida": "vless://11111111-2222-3333-4444-555555555555@example.com:443?type=kcp#x",
		"ss con plugin":   badSIP002,
		"trojan sin pass": "trojan://@example.net:443",
		"ss sin servidor": "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pw")) + "#x",
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseXrayLink(link); err == nil {
				t.Fatalf("parseXrayLink(%q) debería dar error", link)
			}
		})
	}
}

func xrayTestOutbound(t *testing.T, link string) map[string]any {
	t.Helper()
	ep, err := parseXrayLink(link)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	raw, err := xrayClientConfigJSON(ep, "127.0.0.1", 10808)
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	inbounds := cfg["inbounds"].([]any)
	if inbounds[0].(map[string]any)["port"].(float64) != 10808 {
		t.Fatal("inbound SOCKS mal generado")
	}
	return cfg["outbounds"].([]any)[0].(map[string]any)
}

func TestXrayClientConfigTrojan(t *testing.T) {
	out := xrayTestOutbound(t, xrayTestTrojanWS)
	if out["protocol"] != "trojan" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	srv := out["settings"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if srv["password"] != "s3cr3t" || srv["address"] != "example.net" {
		t.Fatalf("servers = %v", srv)
	}
	ss := out["streamSettings"].(map[string]any)
	if ss["network"] != "ws" || ss["security"] != "tls" {
		t.Fatalf("streamSettings = %v", ss)
	}
	ws := ss["wsSettings"].(map[string]any)
	if ws["path"] != "/ws-path" {
		t.Fatalf("wsSettings = %v", ws)
	}
}

func TestXrayClientConfigVlessReality(t *testing.T) {
	out := xrayTestOutbound(t, xrayTestVlessReality)
	users := out["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)["users"].([]any)
	if users[0].(map[string]any)["flow"] != "xtls-rprx-vision" {
		t.Fatalf("users = %v", users)
	}
	rs := out["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)
	if rs["publicKey"] != "0123456789abcdef0123456789abcdef0123456789AB" || rs["serverName"] != "example.org" {
		t.Fatalf("realitySettings = %v", rs)
	}
}

func TestProxyModeWithSub(t *testing.T) {
	t.Setenv("PROXY_MODE", "")
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "https://example.com/sub.txt")
	if got := proxyMode(); got != "socks5" {
		t.Fatalf("con XRAY_SUB el defecto debe ser socks5, got %q", got)
	}
}

func TestFetchSubscriptionPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() == "" {
			t.Error("la sub debería llevar User-Agent para negociar formato")
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("// comentario\n\n" + xrayTestTrojanWS + "\n"))
	}))
	defer srv.Close()
	lines, err := fetchSubscription(srv.URL)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	// El fetch no filtra comentarios (lo hace parseSubscriptionLinks).
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "trojan://") {
		t.Fatalf("líneas = %v", lines)
	}
}

func TestFetchSubscriptionBase64(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte(xrayTestVlessWS + "\n" + xrayTestTrojanWS + "\n"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(blob))
	}))
	defer srv.Close()
	lines, err := fetchSubscription(srv.URL)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("líneas = %d, want 2", len(lines))
	}
}

func TestFetchSubscriptionErrors(t *testing.T) {
	if _, err := fetchSubscription("http://127.0.0.1:1/sub"); err == nil {
		t.Fatal("URL caída debe dar error")
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer empty.Close()
	if _, err := fetchSubscription(empty.URL); err == nil {
		t.Fatal("sub vacía debe dar error")
	}
}

func TestParseSubscriptionLinks(t *testing.T) {
	lines := []string{
		"//profile-title: x",
		"",
		"# comentario",
		xrayTestVlessWS,
		"hysteria2://pass@host.example:443?sni=x#y",
		"basura-sin-esquema",
		xrayTestTrojanWS,
	}
	supported, skipped := parseSubscriptionLinks(lines, 10)
	if len(supported) != 2 || skipped != 2 {
		t.Fatalf("supported=%d skipped=%d, want 2/2", len(supported), skipped)
	}
	// Tope max: con 3 válidos y max=2, el tercero ni se mira.
	multi := []string{xrayTestVlessWS, xrayTestTrojanWS, xrayTestVlessReality, "otro-mal"}
	supported, skipped = parseSubscriptionLinks(multi, 2)
	if len(supported) != 2 || skipped != 0 {
		t.Fatalf("con tope: supported=%d skipped=%d, want 2/0", len(supported), skipped)
	}
}

func TestProxyModeWithLink(t *testing.T) {
	t.Setenv("PROXY_MODE", "")
	t.Setenv("XRAY_SUB", "")
	t.Setenv("XRAY_LINK", xrayTestTrojanWS)
	if got := proxyMode(); got != "socks5" {
		t.Fatalf("con XRAY_LINK el defecto debe ser socks5, got %q", got)
	}
	chain := proxyChain()
	// "direct" cierra la cadena por defecto: si xray o Tor no están
	// levantados, la petición degrada en vez de romperse.
	if len(chain) != 3 || chain[0] != "socks5" || chain[1] != "tor" || chain[2] != "direct" {
		t.Fatalf("chain = %v", chain)
	}
}

func TestProxyChainNoFallbackDirect(t *testing.T) {
	t.Setenv("XRAY_SUB", "")
	t.Setenv("XRAY_LINK", xrayTestTrojanWS)
	t.Setenv("PROXY_FALLBACK_DIRECT", "0")
	chain := proxyChain()
	if len(chain) != 2 || chain[0] != "socks5" || chain[1] != "tor" {
		t.Fatalf("con PROXY_FALLBACK_DIRECT=0 no debe entrar direct, chain = %v", chain)
	}
}

func TestMaybeRunXrayNoLink(t *testing.T) {
	t.Setenv("XRAY_LINK", "")
	t.Setenv("XRAY_SUB", "")
	cmd, err := MaybeRunXray()
	if err != nil || cmd != nil {
		t.Fatalf("sin link debe ser no-op, got %v, %v", cmd, err)
	}
}

func TestMaybeRunXrayBadLink(t *testing.T) {
	t.Setenv("XRAY_SUB", "")
	t.Setenv("XRAY_LINK", "hysteria2://pass@host.example:443")
	if _, err := MaybeRunXray(); err == nil {
		t.Fatal("link no soportado debe dar error (sin arrancar nada)")
	}
}

// TestXrayInProcessStart arranca una instancia real en-proceso con un enlace
// sintético: solo bindea el SOCKS en localhost, sin tráfico saliente (offline).
func TestXrayInProcessStart(t *testing.T) {
	t.Setenv("SOCKS5_ADDR", "127.0.0.1:18080")
	ep, err := parseXrayLink(xrayTestTrojanWS)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	cfgJSON, err := xrayClientConfigJSON(ep, "127.0.0.1", 18080)
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	cfg, err := serial.LoadJSONConfig(bytes.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("LoadJSONConfig error: %v", err)
	}
	inst, err := core.New(cfg)
	if err != nil {
		t.Fatalf("core.New error: %v", err)
	}
	if err := inst.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer inst.Close()
	if err := waitSocksReady("127.0.0.1:18080", 10*time.Second); err != nil {
		t.Fatalf("SOCKS en-proceso no responde: %v", err)
	}
	if !CheckXray() {
		t.Fatal("CheckXray debería ver el SOCKS en-proceso")
	}
	if err := StopXray(&xrayHandle{instance: inst}); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestXraySocksListen(t *testing.T) {
	t.Setenv("SOCKS5_ADDR", "")
	host, port, err := xraySocksListen()
	if err != nil || host != "127.0.0.1" || port != 10808 {
		t.Fatalf("defecto = %s:%d, %v", host, port, err)
	}
	t.Setenv("SOCKS5_ADDR", "garbage-sin-puerto")
	if _, _, err := xraySocksListen(); err == nil {
		t.Fatal("SOCKS5_ADDR inválido debe dar error")
	}
}

const (
	xrayTestWGPriv = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	xrayTestWGPub  = "ZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5/gIGCg4Q="
	xrayTestWGPsk  = "ycrLzM3Oz9DR0tPU1dbX2Nna29zd3t/g4eLj5OXm5+g="
)

func TestParseXrayLinkWireguard(t *testing.T) {
	// userinfo con +/= crudos: solo el parseo manual lo aguanta.
	link := "wireguard://AB+C/DEFghijkLMNOpqrsTUVwxyz0123456789AB@vpn.example:51820?publickey=" +
		"ZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5%2FgIGCg4Q%3D&presharedkey=abc&address=10.0.0.2%2F32" +
		"&keep_alive=25&mtu=1280&reserved=1%2C2%2C3#test"
	ep, err := parseXrayLink(link)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ep.Protocol != "wireguard" || ep.Address != "vpn.example" || ep.Port != 51820 {
		t.Fatalf("endpoint = %+v", ep)
	}
	if ep.WgPrivate != "AB+C/DEFghijkLMNOpqrsTUVwxyz0123456789AB" {
		t.Fatalf("private = %q", ep.WgPrivate)
	}
	if ep.WgPeer != "ZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5/gIGCg4Q=" || ep.WgPsk != "abc" {
		t.Fatalf("peer/psk = %q / %q", ep.WgPeer, ep.WgPsk)
	}
	if ep.WgAddr != "10.0.0.2/32" || ep.WgKeep != 25 || ep.WgMTU != 1280 {
		t.Fatalf("addr/keep/mtu = %q / %d / %d", ep.WgAddr, ep.WgKeep, ep.WgMTU)
	}
	if len(ep.WgRes) != 3 || ep.WgRes[0] != 1 || ep.WgRes[2] != 3 {
		t.Fatalf("reserved = %v", ep.WgRes)
	}
	// Alias de parámetros y puerto por defecto.
	ep2, err := parseXrayLink("wireguard://" + xrayTestWGPriv + "@vpn.example?public_key=" + xrayTestWGPub)
	if err != nil {
		t.Fatalf("parse alias error: %v", err)
	}
	if ep2.Port != 51820 || ep2.WgPeer != xrayTestWGPub {
		t.Fatalf("alias = %+v", ep2)
	}
}

func TestParseXrayLinkWireguardErrors(t *testing.T) {
	cases := map[string]string{
		"sin arroba":        "wireguard://solo-privada",
		"puerto malo":       "wireguard://cHJpdkBzaG9ydA@vpn.example:99999?publickey=eA",
		"sin publickey":     "wireguard://cHJpdkBzaG9ydA@vpn.example?mtu=1280",
		"keepalive malo":    "wireguard://cHJpdkBzaG9ydA@vpn.example?publickey=eA&keepalive=-1",
		"mtu malo":          "wireguard://cHJpdkBzaG9ydA@vpn.example?publickey=eA&mtu=abc",
		"reserved malo":     "wireguard://cHJpdkBzaG9ydA@vpn.example?publickey=eA&reserved=1,999",
		"reserved ilegible": "wireguard://cHJpdkBzaG9ydA@vpn.example?publickey=eA&reserved=!!!",
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseXrayLink(link); err == nil {
				t.Fatalf("parseXrayLink(%q) debería dar error", link)
			}
		})
	}
}

func TestXrayClientConfigWireguard(t *testing.T) {
	link := "wireguard://" + xrayTestWGPriv + "@vpn.example:51820?publickey=" + url.QueryEscape(xrayTestWGPub) +
		"&presharedkey=" + url.QueryEscape(xrayTestWGPsk) + "&keepalive=25&mtu=1280&reserved=AQID#test"
	ep, err := parseXrayLink(link)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	raw, err := xrayClientConfigJSON(ep, "127.0.0.1", 10808)
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	out := cfg["outbounds"].([]any)[0].(map[string]any)
	if out["protocol"] != "wireguard" {
		t.Fatalf("protocol = %v", out["protocol"])
	}
	if _, hasStream := out["streamSettings"]; hasStream {
		t.Fatal("wireguard no lleva streamSettings")
	}
	dev := out["settings"].(map[string]any)
	if dev["secretKey"] != xrayTestWGPriv {
		t.Fatalf("secretKey = %v", dev["secretKey"])
	}
	if dev["mtu"] != float64(1280) || dev["reserved"] != "AQID" {
		t.Fatalf("settings = %v", dev)
	}
	peer := dev["peers"].([]any)[0].(map[string]any)
	if peer["publicKey"] != xrayTestWGPub || peer["endpoint"] != "vpn.example:51820" {
		t.Fatalf("peer = %v", peer)
	}
	if peer["preSharedKey"] != xrayTestWGPsk || peer["keepAlive"] != float64(25) {
		t.Fatalf("peer = %v", peer)
	}
}

func TestParseSubscriptionLinksWireguard(t *testing.T) {
	wg := "wireguard://" + xrayTestWGPriv + "@vpn.example?publickey=" + xrayTestWGPub
	lines := []string{wg, "hysteria2://pass@host.example:443", "basura"}
	supported, skipped := parseSubscriptionLinks(lines, 10)
	if len(supported) != 1 || skipped != 2 {
		t.Fatalf("supported=%d skipped=%d, want 1/2", len(supported), skipped)
	}
	if supported[0].Protocol != "wireguard" {
		t.Fatalf("protocol = %q", supported[0].Protocol)
	}
}
