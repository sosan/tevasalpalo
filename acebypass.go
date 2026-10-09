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
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
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

// deadPaths se cortan con un documento XML vacío y válido.
var deadPaths = []string{
	"vast.php",
	"vast/wrapper",
	"/api/v1/notification",
}

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

// isDeadPath dice si la ruta es de VAST o de notificaciones.
func isDeadPath(path string) bool {
	p := strings.ToLower(path)
	for _, d := range deadPaths {
		if strings.Contains(p, d) {
			return true
		}
	}
	return false
}

// bypassHandler implementa el proxy: CONNECT para túneles y respuesta directa
// para peticiones en claro.
type bypassHandler struct {
	dial func(network, addr string) (net.Conn, error)
}

// serveBypass arranca el proxy de bloqueo en loopback y devuelve la dirección
// para inyectarla en el entorno del motor.
func serveBypass() (string, error) {
	addr := strings.TrimSpace(os.Getenv(bypassAddrEnv))
	if addr == "" {
		addr = bypassListenAddr
	}
	dial := (&net.Dialer{Timeout: 20 * time.Second}).Dial

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           &bypassHandler{dial: dial},
		ReadHeaderTimeout: 20 * time.Second,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("proxy de bypass del motor: %v", err)
		}
	}()
	return ln.Addr().String(), nil
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
	if isDeadHost(hostOnly(host)) {
		// DNS muerto: el motor no espera el timeout de resolución.
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if isDeadPath(r.URL.Path) {
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
	if isDeadHost(host) {
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
