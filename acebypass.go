package main

// Proxy de bloqueo para el tráfico saliente del motor AceStream.
//
// El bypass DNS/VAST del APK (engine/main.py) parchea el motor en Python: DNS
// rápido para hosts muertos y respuestas vacías para VAST/notificaciones. Aquí
// no se puede tocar el motor (Windows lo ejecuta con Python 3.8 embebido sin
// punto de entrada importable), pero sí se puede poner delante.
//
// Se levanta un proxy HTTP en loopback y se le pasa al motor por las variables
// de entorno HTTP_PROXY/http_proxy, que respetan urllib, lxml y avformat sin
// tocar acestream.conf: el motor 3.2.8 no expone claves de proxy (escaneados
// los 701 ficheros del runtime: no existen http-proxy, socks5-proxy ni
// socks-proxy; el único SOCKS real es rtmpdump.exe --socks, que el motor no
// lanza). Las variables de entorno funcionan igual en Windows y en Linux, sin
// permisos de administrador y sin parar el servicio.
//
// Reglas:
//   - hosts muertos -> 502 inmediato, sin esperar el timeout de DNS.
//   - rutas de VAST y notificaciones -> documento XML vacío y válido, para que
//     el reproductor avance en vez de quedarse 14 s bloqueado.
//   - todo lo demás -> túnel CONNECT transparente hacia el destino real.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// bypassListenAddr es el puerto loopback del proxy de bloqueo. 6878 es el
// propio motor y 10808/10809 son los SOCKS de xray y WARP.
const bypassListenAddr = "127.0.0.1:18080"

// bypassAddrEnv permite cambiar el puerto sin recompilar.
const bypassAddrEnv = "ACE_BYPASS_ADDR"

// deadHosts son los hosts que no resuelven en la práctica. Bloquearlos aquí
// evita el timeout de resolución, tanto si el motor llega por CONNECT como si
// resuelve el nombre antes de abrir el túnel.
var deadHosts = []string{
	"torrentstream.org",
	"router.acestream.me",
	"54.36.163.2",
}

// deadVASTPaths se cortan con un documento XML vacío y válido, replicando
// assets/engine/main.py del APK ("torrentstream.org", "vast.php", "vast/wrapper").
var deadVASTPaths = []string{
	"torrentstream.org",
	"vast.php",
	"vast/wrapper",
}

// notifHost y notifPathLocal son el endpoint de notificaciones del APK, que
// responde JSON y no VAST: sustituirlo por un <VAST> no haría el bypass.
const (
	notifHost = "android.acestream.net"
	notifPath = "/api/v1/notification"
)

// emptyNotif es la respuesta que devuelve el APK para notificaciones.
const emptyNotif = `{"notifications":[]}`

// emptyVAST es un VAST válido y sin creatividades: el reproductor lo acepta
// como "no hay anuncios" y sigue al siguiente paso de la decisión
// publicitaria en vez de agotar el tiempo de espera.
const emptyVAST = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
	`<VAST version="3.0"><Ad></Ad></VAST>`

// isDeadHost dice si el host está en la lista de resolución dead. Acepta
// "host" o "host:puerto" (el destino CONNECT viene con puerto) y compara
// sufijos para cubrir los subdominios (*.torrentstream.org).
func isDeadHost(host string) bool {
	h := strings.ToLower(hostOnly(host))
	if h == "" {
		return false
	}
	for _, d := range deadHosts {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// isDeadPath dice si la URL es de VAST (publicidad) y debe responderse con un
// XML vacío. Las notificaciones tienen su propia respuesta JSON: ver
// isNotifURL.
func isDeadPath(path string) bool {
	p := strings.ToLower(path)
	for _, d := range deadVASTPaths {
		if strings.Contains(p, d) {
			return true
		}
	}
	return false
}

// isNotifURL dice si es el endpoint de notificaciones, que espera JSON. Se
// comparan host y ruta por separado: host y path llegan desglosados, no como
// una URL entera.
func isNotifURL(host, path string) bool {
	h := strings.ToLower(hostOnly(host))
	p := strings.ToLower(path)
	return (h == notifHost || strings.HasSuffix(h, "."+notifHost)) && strings.Contains(p, notifPath)
}

// bypassHandler implementa el proxy: CONNECT para túneles y respuesta directa
// para peticiones en claro.
type bypassHandler struct {
	dial func(network, addr string) (net.Conn, error)
	// El motor puede ignorar HTTP_PROXY (no está documentado que lo respete),
	// así que se cuenta el tráfico real que pasa por aquí. Con los contadores a
	// cero tras ver un stream se sabe que la vía no está sirviendo y hay que
	// recurrir al parche de Py_NoSiteFlag.
	stats *bypassStats
}

// bypassStats lleva la cuenta de lo que ha pasado por el proxy, para poder
// verificar en runtime si el motor respeta HTTP_PROXY.
type bypassStats struct {
	mu         sync.Mutex
	connect    int
	plain      int
	deadDNS    int
	emptyVAST  int
	viaWARP    int
	viaDirect  int
	dnsDoH     int
	dnsFail    int
	emptyNotif int
}

// snapshot devuelve una copia de los contadores para leerlos sin bloquear.
func (s *bypassStats) snapshot() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]int{
		"connect":     s.connect,
		"plain":       s.plain,
		"dead_dns":    s.deadDNS,
		"empty_vast":  s.emptyVAST,
		"via_warp":    s.viaWARP,
		"via_direct":  s.viaDirect,
		"dns_doh":     s.dnsDoH,
		"dns_fail":    s.dnsFail,
		"empty_notif": s.emptyNotif,
	}
}

func (s *bypassStats) add(field string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch field {
	case "connect":
		s.connect++
	case "plain":
		s.plain++
	case "dead_dns":
		s.deadDNS++
	case "empty_vast":
		s.emptyVAST++
	case "via_warp":
		s.viaWARP++
	case "via_direct":
		s.viaDirect++
	case "dns_doh":
		s.dnsDoH++
	case "dns_fail":
		s.dnsFail++
	case "empty_notif":
		s.emptyNotif++
	}
}

// engineBypassStats son los contadores del proxy en marcha.
var engineBypassStats = &bypassStats{}

// bypassDial abre los túneles del motor. Si WARP está habilitado pero su SOCKS
// no está levantado (es on-demand), cae a conexión directa: mejor que dejar al
// motor sin salida. Los contadores viaWARP/viaDirect dicen por dónde salió cada
// petición, que es como se comprueba en runtime que el tráfico del motor está
// saliendo por el túnel.
func bypassDial(network, addr string) (net.Conn, error) {
	if warpEnabled() {
		if dialCtx, err := socksDialContext(warpSocksAddr()); err == nil {
			// El motor suele pedir CONNECT host:puerto. Si el nombre llega al
			// SOCKS sin resolver, xray lo resuelve DENTRO del túnel y no hay
			// nada que hacer. Pero para no depender de eso, y para tapar el
			// caso de que xray resuelva con el DNS del sistema, se resuelve por
			// DoH dentro del túnel y se conecta a la IP.
			if host, _, err2 := net.SplitHostPort(addr); err2 == nil && net.ParseIP(host) == nil {
				if ip, err3 := dohResolveOverWARP(dialCtx, host); err3 == nil {
					if c, err4 := dialCtx(context.Background(), network, net.JoinHostPort(ip, portOf(addr))); err4 == nil {
						engineBypassStats.add("via_warp")
						engineBypassStats.add("dns_doh")
						return c, nil
					}
				}
				engineBypassStats.add("dns_fail")
			}
			if c, err2 := dialCtx(context.Background(), network, addr); err2 == nil {
				engineBypassStats.add("via_warp")
				return c, nil
			}
		}
	}
	engineBypassStats.add("via_direct")
	return (&net.Dialer{Timeout: 20 * time.Second}).Dial(network, addr)
}

// portOf devuelve el puerto de un destino "host:puerto", o 443 si no lo trae.
func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return "443"
}

// warpDNSURL es el resolutor DoH de Cloudflare. La consulta va cifrada dentro
// del túnel WARP, así que el ISP no ve ni puede cortar la resolución: es lo que
// arregla el bloqueo de los servidores de propaganda por DNS.
var warpDNSURL = "https://1.1.1.1/dns-query"

// warpDNSInsecure solo lo activa el test, que usa un certificado propio.
var warpDNSInsecure bool

// dohResolveOverWARP pregunta a Cloudflare por la IP de host usando un cliente
// HTTP cuyo Proxy es el SOCKS de WARP. Devolver la IP (y no el nombre) obliga a
// que la conexión vaya después por el mismo túnel.
//
// Se usa cuando WARP está activo porque xray-core no tiene DNS propio
// configurado: resuelve con el resolver del sistema, que es el del ISP.
func dohResolveOverWARP(dialCtx func(context.Context, string, string) (net.Conn, error), host string) (string, error) {
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, a string) (net.Conn, error) {
				return dialCtx(ctx, network, a)
			},
			// El certificado de 1.1.1.1 es el de cloudflare-dns.com: sin esto
			// el handshake falla por no coincidir el nombre.
			TLSClientConfig: &tls.Config{
				ServerName: "cloudflare-dns.com",
				// Solo los tests inyectan un cert propio; en producción el
				// certificado de 1.1.1.1 sí es el de cloudflare-dns.com.
				InsecureSkipVerify: warpDNSInsecure,
			},
		},
	}
	req, err := http.NewRequest(http.MethodGet, warpDNSURL+"?name="+host+"&type=A", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", err
	}
	var out struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	for _, a := range out.Answer {
		if a.Type == 1 && a.Data != "" { // A
			return a.Data, nil
		}
		if a.Type == 28 { // AAAA sin IPv4 detrás: no sirve para el dial
			break
		}
	}
	return "", fmt.Errorf("sin respuesta A para %s", host)
}

// engineBypassAddr es la dirección en la que quedó escuchando el proxy, para
// poder consultarla desde la API.
var engineBypassAddr string

// serveBypass arranca el proxy de bloqueo en loopback y devuelve la dirección
// para inyectarla en el entorno del motor.
func serveBypass() (string, error) {
	addr := strings.TrimSpace(os.Getenv(bypassAddrEnv))
	if addr == "" {
		addr = bypassListenAddr
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           &bypassHandler{dial: bypassDial, stats: engineBypassStats},
		ReadHeaderTimeout: 20 * time.Second,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("proxy de bypass del motor: %v", err)
		}
	}()
	engineBypassAddr = ln.Addr().String()
	return engineBypassAddr, nil
}

// ServeHTTP enruta CONNECT y peticiones en claro.
func (h *bypassHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		h.serveConnect(w, r)
		return
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	h.count("plain")
	if isDeadHost(hostOnly(host)) {
		// DNS muerto: el motor no espera el timeout de resolución.
		h.count("dead_dns")
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if isNotifURL(host, r.URL.Path) {
		// Notificaciones: el APK devuelve {"notifications":[]} (JSON), no un
		// VAST. Servirle XML a un endpoint que espera JSON no hace el bypass.
		h.count("empty_notif")
		writeEmptyNotif(w)
		return
	}
	if isDeadPath(r.URL.Path) {
		h.count("empty_vast")
		writeEmptyVAST(w)
		return
	}
	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(outReq)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// count suma al contador indicado, tolerando un handler sin stats (tests).
func (h *bypassHandler) count(field string) {
	if h.stats == nil {
		return
	}
	h.stats.add(field)
}

// serveConnect hace de túnel CONNECT: bloqueo inmediato para hosts muertos y
// túnel transparente para el resto.
func (h *bypassHandler) serveConnect(w http.ResponseWriter, r *http.Request) {
	target := r.Host
	if target == "" {
		target = r.URL.Host
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host, port = target, "443"
	}
	h.count("connect")
	if isDeadHost(host) {
		h.count("dead_dns")
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	upstream, err := h.dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	client, _, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	// Tunnel bidireccional; al cerrar una dirección se cae la otra.
	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, client); done <- struct{}{} }()
	go func() { io.Copy(client, upstream); done <- struct{}{} }()
	<-done
	client.Close()
	upstream.Close()
}

// writeEmptyNotif responde el JSON vacío de notificaciones que usa el APK.
func writeEmptyNotif(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, emptyNotif)
}

// writeEmptyVAST responde el documento VAST vacío y válido.
func writeEmptyVAST(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, emptyVAST)
}

// engineProxyEnv son las variables que hacen que el motor use el proxy de
// bloqueo. Se cubren mayúsculas y minúsculas porque cada biblioteca Python lee
// una u otra: urllib en minúsculas, lxml/avformat en mayúsculas.
func engineProxyEnv(addr string) []string {
	return []string{
		"HTTP_PROXY=" + addr,
		"http_proxy=" + addr,
		"HTTPS_PROXY=" + addr,
		"https_proxy=" + addr,
	}
}
