package main

// xray-core embebido como librería Go: sin binario externo, sin proceso
// separado y sin credenciales en disco (el enlace solo vive en env y en
// memoria). Expone el inbound SOCKS en SOCKS5_ADDR para la cadena de
// transportes de fetch.go (primario xray, reserva Tor).
//
// - El usuario aporta el SERVIDOR con XRAY_LINK (vless/trojan/ss/wireguard), nunca al repo.
// - MaybeRunXray() nunca es fatal: sin link no hace nada; con error lo
//   devuelve y la cadena de transportes de fetch.go cae a Tor.
// - Soporta redes tcp/ws/grpc/xhttp/httpupgrade y seguridad none/tls/reality.
//   NO soporta: vmess, hysteria/hysteria2, tuic, wireguard, kcp, quic ni
//   plugins ss (obfs) — dan error claro antes de arrancar.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	// distro/all registra todas las apps, proxies y transportes (inbounds,
	// outbounds vless/trojan/ss/wireguard, tls/reality/ws/grpc/xhttp). Sin estos
	// imports ciegos, core.New falla con "... is not registered".
	_ "github.com/xtls/xray-core/main/distro/all"
	"golang.org/x/net/proxy"
)

const xrayDefaultSocks = "127.0.0.1:10808"

const (
	// Límites anti-cuelgue al usar subscripciones grandes o caídas.
	xrayMaxSubEndpoints   = 25
	xrayMaxProbeEndpoints = 5
	xrayProbeTimeout      = 6 * time.Second
	xraySubFetchTimeout   = 20 * time.Second
	xrayProbeURL          = "https://connectivitycheck.gstatic.com/generate_204"
)

// ---------------------------------------------------------------------------
// Parseo de enlaces de subscripción (solo lectura, sin red)
// ---------------------------------------------------------------------------

// xrayEndpoint es un enlace vless/trojan/ss/wireguard normalizado.
type xrayEndpoint struct {
	Protocol  string // vless | trojan | shadowsocks | wireguard
	Address   string
	Port      int
	ID        string // vless uuid
	Password  string // trojan password / ss password
	Method    string // ss method
	Flow      string // vless flow (puede ir vacío)
	WgPrivate string // wireguard private key (base64)
	WgPeer    string // wireguard peer public key (base64)
	WgPsk     string // wireguard pre-shared key (base64, opcional)
	WgAddr    string // wireguard local address (opcional, "ip1,ip2")
	WgKeep    int    // wireguard keepalive segundos (opcional)
	WgMTU     int    // wireguard mtu (opcional)
	WgRes     []byte // wireguard reserved bytes (opcional)
	Security  string // none | tls | reality
	Network   string // tcp | ws | grpc | xhttp | httpupgrade
	Path      string
	Host      string
	SNI       string
	FP        string // fingerprint uTLS
	PBK       string // reality public key
	SID       string // reality short id
	ALPN      []string
	Service   string // grpc serviceName
	Mode      string // grpc/xhttp mode (gun/multi/auto/...)
	Authority string // grpc authority
}

func parseXrayLink(raw string) (*xrayEndpoint, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("enlace vacío")
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("enlace inválido: %w", err)
	}
	ep := &xrayEndpoint{}
	switch strings.ToLower(u.Scheme) {
	case "vless":
		ep.Protocol = "vless"
		ep.ID = u.User.Username()
		if ep.ID == "" {
			return nil, fmt.Errorf("vless sin UUID")
		}
		ep.Flow = u.Query().Get("flow")
		if err := xrayHostPort(ep, u, 443); err != nil {
			return nil, err
		}
		if err := xrayStream(ep, u); err != nil {
			return nil, err
		}
	case "trojan":
		ep.Protocol = "trojan"
		if u.User == nil {
			return nil, fmt.Errorf("trojan sin password")
		}
		ep.Password = u.User.Username()
		if ep.Password == "" {
			return nil, fmt.Errorf("trojan sin password")
		}
		if err := xrayHostPort(ep, u, 443); err != nil {
			return nil, err
		}
		if err := xrayStream(ep, u); err != nil {
			return nil, err
		}
	case "ss", "shadowsocks":
		if err := xrayParseSS(ep, u); err != nil {
			return nil, err
		}
	case "wireguard":
		if err := xrayParseWireguard(ep, s); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("esquema %q no soportado (vless/trojan/ss/wireguard)", u.Scheme)
	}
	return ep, nil
}

func xrayHostPort(ep *xrayEndpoint, u *url.URL, defPort int) error {
	ep.Address = u.Hostname()
	if ep.Address == "" {
		return fmt.Errorf("enlace sin servidor")
	}
	p := u.Port()
	if p == "" {
		ep.Port = defPort
		return nil
	}
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("puerto inválido %q", p)
	}
	ep.Port = n
	return nil
}

// xrayStream lee seguridad/red comunes a vless y trojan.
func xrayStream(ep *xrayEndpoint, u *url.URL) error {
	q := u.Query()
	switch sec := strings.ToLower(q.Get("security")); sec {
	case "", "none":
		ep.Security = "none"
	case "tls":
		ep.Security = "tls"
	case "reality":
		ep.Security = "reality"
	default:
		return fmt.Errorf("seguridad %q no soportada (none/tls/reality)", q.Get("security"))
	}
	netw := strings.ToLower(q.Get("type"))
	if netw == "" {
		netw = "tcp"
	}
	switch netw {
	case "tcp", "ws", "grpc", "xhttp", "httpupgrade":
		ep.Network = netw
	default:
		return fmt.Errorf("red %q no soportada (tcp/ws/grpc/xhttp/httpupgrade)", q.Get("type"))
	}
	if ht := strings.ToLower(q.Get("headerType")); ht != "" && ht != "none" {
		return fmt.Errorf("headerType %q (obfs) no soportado", q.Get("headerType"))
	}
	ep.Path = q.Get("path")
	if ep.Network == "ws" && ep.Path == "" {
		ep.Path = "/"
	}
	ep.Host = q.Get("host")
	ep.SNI = q.Get("sni")
	if ep.SNI == "" && ep.Security != "none" {
		if ep.Host != "" {
			ep.SNI = ep.Host
		} else {
			ep.SNI = ep.Address
		}
	}
	ep.FP = q.Get("fp")
	if ep.Security == "reality" {
		ep.PBK = q.Get("pbk")
		if ep.PBK == "" {
			return fmt.Errorf("reality sin public key (pbk)")
		}
		ep.SID = q.Get("sid")
		if ep.FP == "" {
			ep.FP = "chrome" // defecto de xray/uTLS
		}
	}
	if alpn := q.Get("alpn"); alpn != "" {
		for _, a := range strings.Split(alpn, ",") {
			if a = strings.TrimSpace(a); a != "" {
				ep.ALPN = append(ep.ALPN, a)
			}
		}
	}
	ep.Service = q.Get("serviceName")
	ep.Mode = q.Get("mode")
	ep.Authority = q.Get("authority")
	return nil
}

// xrayParseSS acepta SIP002 (ss://base64(método:pass)@host:port) y legacy
// (ss://base64(método:pass@host:port)). Los plugins (obfs) dan error claro.
func xrayParseSS(ep *xrayEndpoint, u *url.URL) error {
	ep.Protocol = "shadowsocks"
	if u.Query().Get("plugin") != "" {
		return fmt.Errorf("ss con plugin obfs no soportado")
	}
	var methodPass, hostport string
	if u.User == nil && !strings.Contains(u.Hostname(), ".") && !strings.Contains(u.Hostname(), ":") {
		// Legacy: todo el host es el blob base64.
		raw, err := b64decode(u.Hostname())
		if err != nil {
			return fmt.Errorf("ss legacy inválido: %w", err)
		}
		at := strings.LastIndex(string(raw), "@")
		if at == -1 {
			return fmt.Errorf("ss legacy sin @host")
		}
		methodPass, hostport = string(raw[:at]), string(raw[at+1:])
	} else {
		if u.User == nil {
			return fmt.Errorf("ss sin credenciales")
		}
		raw, err := b64decode(u.User.Username())
		if err != nil {
			return fmt.Errorf("ss SIP002 inválido: %w", err)
		}
		methodPass = string(raw)
		hostport = u.Host
	}
	colon := strings.Index(methodPass, ":")
	if colon == -1 {
		return fmt.Errorf("ss sin método:password")
	}
	ep.Method, ep.Password = methodPass[:colon], methodPass[colon+1:]
	if ep.Method == "" || ep.Password == "" {
		return fmt.Errorf("ss con método o password vacíos")
	}
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		host, portStr = hostport, "8388"
	}
	ep.Address = strings.Trim(host, "[]")
	if ep.Address == "" {
		return fmt.Errorf("ss sin servidor")
	}
	n, err := strconv.Atoi(portStr)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("ss con puerto inválido %q", portStr)
	}
	ep.Port = n
	return nil
}

func b64decode(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	padded := s + strings.Repeat("=", (4-len(s)%4)%4)
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("base64 inválido")
}

// xrayQuery normaliza query a mapa minúsculas para aceptar alias.
func xrayQuery(qs string) map[string]string {
	out := make(map[string]string)
	vals, err := url.ParseQuery(qs)
	if err != nil {
		return out
	}
	for k, v := range vals {
		if len(v) > 0 {
			out[strings.ToLower(k)] = v[0]
		}
	}
	return out
}

func xrayQGet(q map[string]string, names ...string) string {
	for _, n := range names {
		if v, ok := q[n]; ok && v != "" {
			return v
		}
	}
	return ""
}

// xrayParseWireguard acepta wireguard://PRIV@host:port?publickey=..&...
// Parseo manual del userinfo: las claves base64 traen +/= que url.Parse
// rechaza sin escapar. NO es el warp:// de Hiddify (ese requiere registro
// de cuenta Cloudflare vía API; ver comentario en la cabecera).
func xrayParseWireguard(ep *xrayEndpoint, raw string) error {
	ep.Protocol = "wireguard"
	rest := raw
	if i := strings.Index(rest, "://"); i != -1 {
		rest = rest[i+3:]
	}
	if i := strings.Index(rest, "#"); i != -1 {
		rest = rest[:i]
	}
	query := ""
	if i := strings.Index(rest, "?"); i != -1 {
		query = rest[i+1:]
		rest = rest[:i]
	}
	at := strings.LastIndex(rest, "@")
	if at == -1 {
		return fmt.Errorf("wireguard sin privatekey@host")
	}
	priv, err := url.PathUnescape(rest[:at])
	if err != nil || strings.TrimSpace(priv) == "" {
		return fmt.Errorf("wireguard sin private key")
	}
	ep.WgPrivate = priv
	hostport := rest[at+1:]
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		host, portStr = hostport, "51820"
	}
	ep.Address = strings.Trim(host, "[]")
	if ep.Address == "" {
		return fmt.Errorf("wireguard sin servidor")
	}
	ep.Port = 51820
	if portStr != "" {
		n, err := strconv.Atoi(portStr)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("wireguard con puerto inválido %q", portStr)
		}
		ep.Port = n
	}
	q := xrayQuery(query)
	ep.WgPeer = xrayQGet(q, "publickey", "public_key", "peer", "peerpublickey")
	if ep.WgPeer == "" {
		return fmt.Errorf("wireguard sin publickey del peer")
	}
	ep.WgPsk = xrayQGet(q, "presharedkey", "pre-shared-key", "pre_shared_key", "psk")
	ep.WgAddr = xrayQGet(q, "address", "local_address", "localaddress")
	if ka := xrayQGet(q, "keepalive", "keep-alive", "keep_alive", "persistentkeepalive", "persistent_keepalive"); ka != "" {
		n, err := strconv.Atoi(ka)
		if err != nil || n < 0 {
			return fmt.Errorf("wireguard keepalive inválido %q", ka)
		}
		ep.WgKeep = n
	}
	if mtu := xrayQGet(q, "mtu"); mtu != "" {
		n, err := strconv.Atoi(mtu)
		if err != nil || n <= 0 {
			return fmt.Errorf("wireguard mtu inválido %q", mtu)
		}
		ep.WgMTU = n
	}
	if res := xrayQGet(q, "reserved"); res != "" {
		b, err := parseWGReserved(res)
		if err != nil {
			return err
		}
		ep.WgRes = b
	}
	return nil
}

// parseWGReserved acepta "1,2,3" o base64.
func parseWGReserved(s string) ([]byte, error) {
	if strings.Contains(s, ",") {
		var out []byte
		for _, p := range strings.Split(s, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 || n > 255 {
				return nil, fmt.Errorf("reserved inválido %q", s)
			}
			out = append(out, byte(n))
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("reserved vacío")
		}
		return out, nil
	}
	b, err := b64decode(strings.TrimSpace(s))
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("reserved inválido %q", s)
	}
	return b, nil
}

// ---------------------------------------------------------------------------
// Generación de config xray (JSON) — sin secretos fuera de runtime/
// ---------------------------------------------------------------------------

func putNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func xrayOutboundProtocol(ep *xrayEndpoint) string {
	if ep.Protocol == "shadowsocks" {
		return "shadowsocks"
	}
	return ep.Protocol // vless | trojan
}

func xrayClientConfigJSON(ep *xrayEndpoint, listen string, port int) ([]byte, error) {
	inbound := map[string]any{
		"protocol": "socks",
		"listen":   listen,
		"port":     port,
		"settings": map[string]any{"auth": "noauth", "udp": true},
	}
	var outbound map[string]any
	switch ep.Protocol {
	case "vless":
		user := map[string]any{"id": ep.ID, "encryption": "none"}
		putNonEmpty(user, "flow", ep.Flow)
		outbound = map[string]any{
			"protocol": "vless",
			"settings": map[string]any{
				"vnext": []any{map[string]any{
					"address": ep.Address,
					"port":    ep.Port,
					"users":   []any{user},
				}},
			},
			"streamSettings": xrayStreamSettings(ep),
		}
	case "trojan":
		outbound = map[string]any{
			"protocol": "trojan",
			"settings": map[string]any{
				"servers": []any{map[string]any{
					"address":  ep.Address,
					"port":     ep.Port,
					"password": ep.Password,
				}},
			},
			"streamSettings": xrayStreamSettings(ep),
		}
	case "shadowsocks":
		outbound = map[string]any{
			"protocol": "shadowsocks",
			"settings": map[string]any{
				"servers": []any{map[string]any{
					"address":  ep.Address,
					"port":     ep.Port,
					"method":   ep.Method,
					"password": ep.Password,
				}},
			},
		}
	case "wireguard":
		peer := map[string]any{
			"publicKey": ep.WgPeer,
			"endpoint":  net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port)),
		}
		putNonEmpty(peer, "preSharedKey", ep.WgPsk)
		if ep.WgKeep > 0 {
			peer["keepAlive"] = ep.WgKeep
		}
		dev := map[string]any{
			"secretKey": ep.WgPrivate,
			"peers":     []any{peer},
		}
		if ep.WgAddr != "" {
			var addrs []string
			for _, a := range strings.Split(ep.WgAddr, ",") {
				if a = strings.TrimSpace(a); a != "" {
					addrs = append(addrs, a)
				}
			}
			if len(addrs) > 0 {
				dev["address"] = addrs
			}
		}
		if ep.WgMTU > 0 {
			dev["mtu"] = ep.WgMTU
		}
		if len(ep.WgRes) > 0 {
			dev["reserved"] = base64.StdEncoding.EncodeToString(ep.WgRes)
		}
		outbound = map[string]any{
			"protocol": "wireguard",
			"settings": dev,
		}
	default:
		return nil, fmt.Errorf("protocolo %q no soportado", ep.Protocol)
	}
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  []any{inbound},
		"outbounds": []any{outbound},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func xrayStreamSettings(ep *xrayEndpoint) map[string]any {
	ss := map[string]any{"network": ep.Network, "security": ep.Security}
	switch ep.Security {
	case "tls":
		tls := map[string]any{"serverName": ep.SNI}
		putNonEmpty(tls, "fingerprint", ep.FP)
		if len(ep.ALPN) > 0 {
			tls["alpn"] = ep.ALPN
		}
		ss["tlsSettings"] = tls
	case "reality":
		ss["realitySettings"] = map[string]any{
			"serverName":  ep.SNI,
			"publicKey":   ep.PBK,
			"shortId":     ep.SID,
			"fingerprint": ep.FP,
		}
	}
	switch ep.Network {
	case "ws":
		wsHost := ep.Host
		if wsHost == "" {
			wsHost = ep.SNI
		}
		ws := map[string]any{"path": ep.Path}
		if wsHost != "" && wsHost != ep.Address {
			ws["headers"] = map[string]any{"Host": wsHost}
		}
		ss["wsSettings"] = ws
	case "grpc":
		grpc := map[string]any{}
		putNonEmpty(grpc, "serviceName", ep.Service)
		putNonEmpty(grpc, "authority", ep.Authority)
		if ep.Mode == "gun" || ep.Mode == "multi" {
			grpc["multiMode"] = true
		}
		ss["grpcSettings"] = grpc
	case "xhttp":
		xhttp := map[string]any{}
		putNonEmpty(xhttp, "path", ep.Path)
		putNonEmpty(xhttp, "host", ep.Host)
		putNonEmpty(xhttp, "mode", ep.Mode)
		ss["xhttpSettings"] = xhttp
	case "httpupgrade":
		hu := map[string]any{}
		putNonEmpty(hu, "path", ep.Path)
		putNonEmpty(hu, "host", ep.Host)
		ss["httpupgradeSettings"] = hu
	}
	return ss
}

// ---------------------------------------------------------------------------
// Ciclo de vida en-proceso (sin binario externo ni credenciales en disco)
// ---------------------------------------------------------------------------

// xrayHandle es una instancia xray-core corriendo en este proceso.
type xrayHandle struct {
	instance *core.Instance
	desc     string
}

func xrayLink() string {
	return strings.TrimSpace(os.Getenv("XRAY_LINK"))
}

// xrayDefaultSub es la subscripción pública por defecto (igual que las
// fuentes de listas en extractdata.go: URL pública, no secreto).
// XRAY_SUB la sustituye; XRAY_SUB vacía la desactiva.
const xrayDefaultSub = "https://raw.githubusercontent.com/4n0nymou3/multi-proxy-config-fetcher/refs/heads/main/configs/proxy_configs.txt"

// xraySubURL devuelve la URL de subscripción (lista de enlaces), si hay.
func xraySubURL() string {
	if v, ok := os.LookupEnv("XRAY_SUB"); ok {
		return strings.TrimSpace(v)
	}
	return xrayDefaultSub
}

// fetchSubscription descarga una subscripción estándar (texto plano o blob
// base64 estilo v2rayNG) y devuelve sus líneas no vacías.
func fetchSubscription(rawURL string) ([]string, error) {
	client := &http.Client{Timeout: xraySubFetchTimeout}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	// Muchas subscripciones (incl. paneles Hiddify) negocian el formato
	// lista-URI según el User-Agent.
	req.Header.Set("User-Agent", "v2rayNG")
	req.Header.Set("Accept", "text/plain,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscripción status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil, fmt.Errorf("subscripción vacía")
	}
	if !strings.Contains(text, "://") {
		compact := strings.Join(strings.Fields(text), "")
		if raw, err := b64decode(compact); err == nil && strings.Contains(string(raw), "://") {
			text = string(raw)
		}
	}
	var lines []string
	for _, ln := range strings.Split(text, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	return lines, nil
}

// parseSubscriptionLinks filtra líneas a endpoints soportados (vless/trojan/ss/wireguard).
// Lo demás (hysteria2, vmess, comentarios...) cuenta como omitido, no como error.
func parseSubscriptionLinks(lines []string, max int) (supported []*xrayEndpoint, skipped int) {
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "//") {
			continue
		}
		ep, err := parseXrayLink(ln)
		if err != nil {
			skipped++
			continue
		}
		supported = append(supported, ep)
		if len(supported) >= max {
			break
		}
	}
	return supported, skipped
}

// xrayCandidates ordena las fuentes: subscripción primero (pool fresco),
// enlace único como reserva.
func xrayCandidates(link, sub string) ([]*xrayEndpoint, error) {
	if sub != "" {
		lines, err := fetchSubscription(sub)
		if err != nil {
			log.Printf("⚠️  XRAY_SUB no se pudo obtener (%v); probando XRAY_LINK", err)
		} else {
			supported, skipped := parseSubscriptionLinks(lines, xrayMaxSubEndpoints)
			log.Printf("📡 XRAY_SUB: %d endpoints utilizables (%d omitidos: no vless/trojan/ss/wireguard)", len(supported), skipped)
			if len(supported) > 0 {
				return supported, nil
			}
			log.Println("⚠️  XRAY_SUB sin endpoints utilizables; probando XRAY_LINK")
		}
	}
	if link == "" {
		return nil, fmt.Errorf("sin XRAY_LINK de reserva")
	}
	ep, err := parseXrayLink(link)
	if err != nil {
		return nil, fmt.Errorf("XRAY_LINK inválido: %w", err)
	}
	return []*xrayEndpoint{ep}, nil
}

// startXrayInstance levanta una instancia en-proceso con su SOCKS listo.
func startXrayInstance(ep *xrayEndpoint, listen string, port int) (*core.Instance, error) {
	cfgJSON, err := xrayClientConfigJSON(ep, listen, port)
	if err != nil {
		return nil, err
	}
	cfg, err := serial.LoadJSONConfig(bytes.NewReader(cfgJSON))
	if err != nil {
		return nil, fmt.Errorf("config xray inválida: %w", err)
	}
	inst, err := core.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear xray: %w", err)
	}
	if err := inst.Start(); err != nil {
		return nil, fmt.Errorf("no se pudo iniciar xray: %w", err)
	}
	addr := net.JoinHostPort(listen, strconv.Itoa(port))
	if err := waitSocksReady(addr, 15*time.Second); err != nil {
		_ = inst.Close()
		return nil, fmt.Errorf("xray arrancó pero su SOCKS %s no responde: %w", addr, err)
	}
	return inst, nil
}

// probeEndpoint comprueba tráfico REAL por el túnel (cualquier respuesta HTTP
// vale, incluso un error del host de prueba: demuestra que el túnel pasa).
func probeEndpoint(socksAddr string) bool {
	sockURL, err := url.Parse("socks5://" + socksAddr)
	if err != nil {
		return false
	}
	dialer, err := proxy.FromURL(sockURL, proxy.Direct)
	if err != nil {
		return false
	}
	client := &http.Client{
		Timeout: xrayProbeTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		},
	}
	resp, err := client.Get(xrayProbeURL)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	return true
}
func xraySocksListen() (string, int, error) {
	host, portStr, err := net.SplitHostPort(socks5Addr())
	if err != nil {
		return "", 0, fmt.Errorf("SOCKS5_ADDR inválido %q (formato host:port)", socks5Addr())
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("SOCKS5_ADDR con puerto inválido %q", socks5Addr())
	}
	return host, port, nil
}

// MaybeRunXray arranca xray-core en-proceso si hay XRAY_SUB o XRAY_LINK.
// Nunca es fatal: sin fuentes devuelve (nil,nil); con error lo devuelve y el
// llamador cae a Tor vía la cadena de transportes.
func MaybeRunXray() (*xrayHandle, error) {
	link, sub := xrayLink(), xraySubURL()
	if link == "" && sub == "" {
		return nil, nil
	}
	if proxyMode() == "off" {
		log.Println("⏭️  PROXY_MODE=off: xray no se inicia aunque haya XRAY_SUB/XRAY_LINK")
		return nil, nil
	}
	candidates, err := xrayCandidates(link, sub)
	if err != nil {
		return nil, err
	}
	listen, port, err := xraySocksListen()
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(listen, strconv.Itoa(port))
	tried := 0
	for _, ep := range candidates {
		if tried >= xrayMaxProbeEndpoints {
			break
		}
		tried++
		desc := fmt.Sprintf("%s %s:%d", ep.Protocol, ep.Address, ep.Port)
		inst, err := startXrayInstance(ep, listen, port)
		if err != nil {
			log.Printf("⚠️  xray [%s] no arranca: %v", desc, err)
			continue
		}
		if !probeEndpoint(addr) {
			log.Printf("⚠️  xray [%s] sin tráfico, probando siguiente...", desc)
			_ = inst.Close()
			continue
		}
		log.Printf("✅ xray listo (en-proceso): %s vía SOCKS %s (reserva: Tor)", desc, addr)
		return &xrayHandle{instance: inst, desc: desc}, nil
	}
	return nil, fmt.Errorf("ningún endpoint responde tras probar %d", tried)
}

// CheckXray dice si el SOCKS en-proceso responde (para estado/diagnóstico).
func CheckXray() bool {
	conn, err := net.DialTimeout("tcp", socks5Addr(), 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitSocksReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout esperando SOCKS %s", addr)
}

// StopXray detiene la instancia en-proceso (nil-safe).
func StopXray(h *xrayHandle) error {
	if h == nil || h.instance == nil {
		return nil
	}
	return h.instance.Close()
}
