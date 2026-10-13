package main

// Acceso al motor AceStream local o remoto a través de la cadena proxy.
//
// - ACESTREAM_API (defecto http://127.0.0.1:6878): base del engine. Con un
//   motor remoto (p. ej. un VPS sin throttling) todo el tráfico HTTP entre
//   esta app y el engine viaja por la cadena de transportes (xray/Tor), así
//   los streams se ven a través de los clientes proxy. El P2P ocurre en el
//   remoto; aquí solo viaja HTTP.
// - Loopback nunca va por proxy: un SOCKS resolvería 127.0.0.1 en SU lado.
//   Con el engine local el comportamiento es idéntico al anterior.

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

// acestreamAPI devuelve la base del engine sin barra final.
func acestreamAPI() string {
	base := strings.TrimSpace(os.Getenv("ACESTREAM_API"))
	if base == "" {
		return "http://127.0.0.1:6878"
	}
	return strings.TrimSuffix(base, "/")
}

// acestreamEngineHosts devuelve los hosts a reescribir en manifests:
// el clásico local más el del ACESTREAM_API si es otro.
func acestreamEngineHosts() []string {
	hosts := []string{"127.0.0.1:6878"}
	u, err := url.Parse(acestreamAPI())
	if err == nil && u.Host != "" && u.Host != hosts[0] {
		hosts = append(hosts, u.Host)
	}
	return hosts
}

// isLoopbackURL dice si la URL apunta a esta máquina.
func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ---------------------------------------------------------------------------
// Interruptor proxy multimedia (UI web + PROXY_MEDIA).
// OFF (defecto, comportamiento actual): streams directos.
// ON: /api/iptv/* y /ace/* remoto van por la cadena (Tor/xray) con directa
// como último recurso; loopback siempre directo (un SOCKS resolvería
// 127.0.0.1 en su lado).
// ---------------------------------------------------------------------------

var proxyMediaFlag atomic.Bool
var proxyMediaOnce sync.Once

func parseBoolEnv(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func proxyMediaEnabled() bool {
	proxyMediaOnce.Do(func() {
		proxyMediaFlag.Store(parseBoolEnv("PROXY_MEDIA", false))
	})
	return proxyMediaFlag.Load()
}

func setProxyMedia(v bool) {
	proxyMediaOnce.Do(func() {})
	proxyMediaFlag.Store(v)
}

// mediaTransports: directa si el interruptor está OFF; cadena + directa
// final como último recurso si está ON (ver el stream manda).
func mediaTransports() []string {
	if !proxyMediaEnabled() {
		return []string{"direct"}
	}
	return proxyChainWithDirectFallback()
}

// proxyChainWithDirectFallback añade "direct" al final si no está.
// ON = proxies primero, directa última (nunca duplicada).
//
// Con WARP habilitado (warpEnabled) se antepone "warp": es el transporte que
// corresponde al tráfico de reproducción, y como el túnel es on-demand puede
// no estar levantado todavía. Si no lo está, el dial da "connection refused" y
// la rotación sigue con el resto de la cadena sin coste apreciable.
func proxyChainWithDirectFallback() []string {
	chain := proxyChain()
	if warpEnabled() {
		chain = append([]string{"warp"}, chain...)
	}
	for _, tr := range chain {
		if tr == "direct" {
			return chain
		}
	}
	return append(chain, "direct")
}

// socksDialContext construye un DialContext vía SOCKS (sin marcar: no abre
// conexiones, el dial ocurre al usarlo).
func socksDialContext(socksAddr string) (func(context.Context, string, string) (net.Conn, error), error) {
	sockURL, err := url.Parse("socks5://" + socksAddr)
	if err != nil {
		return nil, err
	}
	dialer, err := proxy.FromURL(sockURL, &net.Dialer{Timeout: 15 * time.Second})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}, nil
}

// streamClientFor construye el cliente de streaming de un transporte con los
// mismos parámetros que el directo histórico (Timeout 0, sin redirects auto).
func streamClientFor(name string) (*http.Client, error) {
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	if name != "direct" {
		addr := socks5Addr()
		if name == "tor" {
			addr = "localhost:" + portTor
		}
		dialCtx, err := socksDialContext(addr)
		if err != nil {
			return nil, err
		}
		tr.DialContext = dialCtx
	}
	return &http.Client{
		Timeout:   0,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// openUpstream obtiene la respuesta final (siguiendo redirects) rotando por
// transportes ante errores de transporte. Los HTTP del origen se propagan.
// El llamador cierra resp.Body. Mensajes idénticos a los históricos.
func openUpstream(targetURL, rangeHeader string, transports []string) (*http.Response, string, error) {
	lastErr := fmt.Errorf("sin transportes disponibles")
	for _, tr := range transports {
		client, err := streamClientFor(tr)
		if err != nil {
			lastErr = err
			continue
		}
		resp, finalURL, err := doUpstream(client, targetURL, rangeHeader)
		if err != nil {
			lastErr = err
			log.Printf("⚠️  media vía %s falló, rotando: %v", tr, err)
			continue
		}
		return resp, finalURL, nil
	}
	return nil, "", lastErr
}

func doUpstream(client *http.Client, targetURL, rangeHeader string) (*http.Response, string, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to create request: %s", err.Error())
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to connect to stream: %s", err.Error())
	}

	for resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location == "" {
			break // No hay dónde redirigir
		}
		resp.Body.Close() // Cerrar el cuerpo de la respuesta de redirección

		// Resolver la nueva URL con base en la anterior (por si la Location es relativa)
		baseURL, err := url.Parse(targetURL) // targetURL es la URL de la solicitud original o la última redirección
		if err != nil {
			return nil, "", fmt.Errorf("Failed to parse base URL for redirect: %s", err.Error())
		}
		newURL, err := baseURL.Parse(location)
		if err != nil {
			return nil, "", fmt.Errorf("Failed to parse redirect location: %s", err.Error())
		}

		// fmt.Printf("Following redirect from '%s' to '%s'\n", targetURL, newURL.String())
		targetURL = newURL.String()

		// Crear una nueva solicitud para la URL redirigida
		req, err = http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return nil, "", fmt.Errorf("Failed to create redirect request: %s", err.Error())
		}
		// Copiar headers importantes nuevamente
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}

		// Hacer la nueva petición
		resp, err = client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("Failed to connect to redirected stream: %s", err.Error())
		}
	}

	return resp, req.URL.String(), nil
}

// aceTimeoutSegmento es el timeout para datos de medio (/ace/c/*.ts). Corto a
// propósito: un segmento que no llega en 30s no va a llegar, y colgarse aquí
// retiene la conexión del player.
const aceTimeoutSegmento = 30 * time.Second

// aceTimeoutManifest es el timeout para el manifest principal
// (/ace/manifest.m3u8). Es una long-poll del engine: se retiene esperando a
// que aparezcan pares P2P, así que tarda bastante más que un segmento. Con el
// timeout de 30s moría siempre y el player se quedaba sin nivel de medio.
const aceTimeoutManifest = 120 * time.Second

// aceTimeoutAnidado es el timeout para los manifests anidados /ace/m/*.m3u8.
// A diferencia del principal, estos no llegan nunca: el engine los retiene
// indefinidamente aunque el flujo principal sí advanced (los .ts llegan bien).
// Esperar 120s solo servía para retener la conexión y, al expirar, HLS.js lo
// escalaba a error fatal y tumbaba toda la carga. Retenerlos 8s y responder
// 204 (no hay actualización) mantiene vivo el player con el último playlist
// bueno mientras el manifest principal sigue fluyendo.
const aceTimeoutAnidado = 8 * time.Second

// aceTimeoutFor elige el timeout según la ruta de /ace/: los anidados
// /ace/m/ fallan rápido a propósito, el manifest principal tolera largo
// porque es long-poll, y los segmentos van cortos.
func aceTimeoutFor(acePath string) time.Duration {
	p := strings.ToLower(acePath)
	if isAceNestedManifest(p) {
		return aceTimeoutAnidado
	}
	if strings.HasSuffix(p, ".m3u8") {
		return aceTimeoutManifest
	}
	return aceTimeoutSegmento
}

// isAceNestedManifest indica si la ruta es un manifest anidado /ace/m/*.m3u8.
// El path que entrega Fiber no lleva barra inicial ("m/aa42/711b.m3u8"), así
// que se compara por segmento y no con un Contains con barras, que nunca
// casaría. Cuando estos fallan hay que responder 204 en lugar de 502: HLS.js
// trata el 502 como error fatal de red y detiene la carga.
func isAceNestedManifest(acePath string) bool {
	p := strings.ToLower(acePath)
	if !strings.HasSuffix(p, ".m3u8") {
		return false
	}
	rest := strings.TrimPrefix(p, "/")
	return rest == "m" || strings.HasPrefix(rest, "m/")
}

// aceReqFunc construye y ejecuta la petición de un salto. Recibe la URL del
// salto y no solo el cliente porque los redirects se siguen aquí y no dentro
// de http.Client: el timeout se decide por ruta, y el cliente arrastraría el
// timeout del manifest principal (2m) al anidado al que este redirige.
type aceReqFunc func(url string, client *http.Client) (*http.Response, error)

// aceRedirectsMax acota los saltos de redirección. El engine usa uno
// (manifest.m3u8 -> /ace/m/<hash>/<hash>.m3u8); el tope está para que un
// bucle no se convierta en una espera infinita.
const aceRedirectsMax = 5

// aceDo ejecuta la petición contra el engine: directa a loopback siempre;
// a remoto, cadena proxy (xray/Tor) si allowProxy o directa si no.
// Devuelve el transporte usado. Solo rota ante errores de transporte, no
// ante errores HTTP del engine (se propagan tal cual).
func aceDo(targetURL string, allowProxy bool, do aceReqFunc) (*http.Response, string, error) {
	resp, via, _, err := aceDoTimeout(targetURL, allowProxy, aceTimeoutSegmento, do)
	return resp, via, err
}

// aceDoTimeout es aceDo con timeout explícito por el primer salto. Sigue los
// redirects recalculando el timeout de cada destino, y devuelve la URL final
// (la alcanzada, también si el salto falla) para que el llamador pueda
// clasificar el fallo por la ruta por la que realmente se acabó pidiendo.
//
// Sin esto, /ace/manifest.m3u8?id=<id> responde 302 hacia /ace/m/<hash>/*.m3u8
// en ~200ms y el cliente directo lo seguía con el timeout del principal (2m),
// contra un manifest que el engine retiene indefinidamente esperando pares:
// 120s y un 502 que HLS.js trata como error fatal de red. Es la diferencia
// entre un arranque de stream y ninguno.
func aceDoTimeout(targetURL string, allowProxy bool, timeout time.Duration, do aceReqFunc) (*http.Response, string, string, error) {
	actual := targetURL
	via := ""
	for salto := 0; ; salto++ {
		resp, tr, err := aceDoSalto(actual, timeout, allowProxy, do)
		if err != nil {
			return nil, "", actual, err
		}
		via = tr
		siguiente := aceRedirectTarget(resp)
		if siguiente == "" || salto >= aceRedirectsMax {
			if siguiente != "" {
				log.Printf("⚠️  ACE: demasiados redirects, sigo en %s", actual)
			}
			return resp, via, actual, nil
		}
		resp.Body.Close()
		timeout = aceTimeoutFor(acePathFromURL(siguiente))
		actual = siguiente
		traceLog("↪️  ACE redirect -> %s (timeout=%v)", actual, timeout)
	}
}

// aceDoSalto ejecuta un único salto con rotación de transportes. El cliente
// directo no sigue redirects (los lleva aceDoTimeout, que recalcula el
// timeout por ruta); si los siguiera, el timeout del primer salto manda
// sobre todos los siguientes.
func aceDoSalto(targetURL string, timeout time.Duration, allowProxy bool, do aceReqFunc) (*http.Response, string, error) {
	transports := []string{"direct"}
	loopback := isLoopbackURL(targetURL)
	if !loopback && allowProxy {
		transports = proxyChainWithDirectFallback()
	}
	lastErr := fmt.Errorf("sin transportes disponibles")
	for _, tr := range transports {
		client := &http.Client{
			Timeout: timeout,
			// Los redirects los lleva aceDoTimeout, que recalcula el timeout
			// por ruta. Si los siguiera el cliente, el timeout del primer
			// salto mandaría sobre el anidado al que redirige.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		if tr != "direct" {
			var err error
			client, err = clientForTransport(tr)
			if err != nil {
				lastErr = err
				continue
			}
		}
		resp, err := do(targetURL, client)
		if err != nil {
			lastErr = err
			if len(transports) > 1 {
				log.Printf("⚠️  ACE vía %s falló, rotando: %v", tr, err)
			}
			continue
		}
		return resp, tr, nil
	}
	return nil, "", lastErr
}

// aceRedirectTarget devuelve la URL absoluta a la que redirige resp, o "" si
// no es una redirección con Location. Resuelve Locations relativos contra la
// URL del propio salto.
func aceRedirectTarget(resp *http.Response) string {
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return ""
	}
	loc := resp.Header.Get("Location")
	if loc == "" || resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	siguiente, err := resp.Request.URL.Parse(loc)
	if err != nil {
		return ""
	}
	return siguiente.String()
}

// acePathFromURL devuelve la ruta de una URL del engine sin el prefijo /ace,
// con el formato que espera aceTimeoutFor e isAceNestedManifest.
func acePathFromURL(rawURL string) string {
	p, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	rest := strings.TrimPrefix(p.Path, "/")
	rest = strings.TrimPrefix(rest, "ace/")
	return rest
}

// rewriteAceManifestLine reescribe una línea de manifest al origen local:
// hosts del engine -> localHost; segmentos relativos -> absolutos.
// Las líneas de comentario/vacías y URLs absolutas ajenas se dejan igual.
func rewriteAceManifestLine(line, serverOrigin, localHost string, engineHosts []string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}
	for _, eh := range engineHosts {
		if eh != "" && strings.Contains(trimmed, eh) {
			newLine := strings.Replace(trimmed, eh, localHost, 1)
			// Una línea por segmento del manifest, y los manifests se
			// refrescan cada ~30s: son el grueso del volumen de telemetría y
			// no dicen nada que el resumen no diga. El detalle queda para la
			// traza.
			traceLog("🔄 URL reescrita: %s -> %s", trimmed, newLine)
			return newLine
		}
	}
	if strings.HasSuffix(trimmed, ".ts") || strings.HasSuffix(trimmed, ".m3u8") {
		// Solo lo que es una ruta relativa de verdad. Antes se comprobaba
		// únicamente que no empezara por http, así que cualquier otro esquema
		// ("otro://x.ts") se reescribía a una URL absurda del propio proxy.
		if !strings.Contains(trimmed, "://") {
			newURL := fmt.Sprintf("%s/ace/%s", serverOrigin, strings.TrimPrefix(trimmed, "/"))
			traceLog("🔄 URL relativa convertida: %s -> %s", trimmed, newURL)
			return newURL
		}
	}
	return line
}

// hashDePeticionAce saca el infohash de una ruta /ace/* cuando la petición es
// de datos de stream, que es donde se sabe si el hash está vivo.
//
// Solo cuenta getstream y las rutas de datos con el hash en la query. Los
// manifests (principal y anidado) no dicen nada sobre el hash: el motor los
// retiene aunque el flujo esté bien, y anotarlos como fallo mataría hashes que
// funcionan a la perfection.
func hashDePeticionAce(acePath, queryString string) string {
	p := strings.ToLower(acePath)

	// /ace/getstream?id=<hash> y /ace/h/<hash>/...
	if strings.Contains(p, "getstream") || strings.Contains(p, "server/stream") {
		return hashDeQuery(queryString)
	}
	if i := strings.Index(p, "/h/"); i >= 0 {
		resto := p[i+3:]
		if j := strings.IndexAny(resto, "/?"); j >= 0 {
			resto = resto[:j]
		}
		return normalizarHash(resto)
	}
	if strings.Contains(p, "content/") || strings.Contains(p, "hls/") {
		return hashDeQuery(queryString)
	}
	return ""
}

// hashDeQuery busca un infohash de 40 hex en la query string. Se busca por
// forma y no por nombre de parámetro porque el motor usa id, infohash o hash
// según la versión.
func hashDeQuery(queryString string) string {
	for _, par := range strings.Split(queryString, "&") {
		k, v, ok := strings.Cut(par, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "id" && k != "infohash" && k != "hash" && k != "stream" {
			continue
		}
		if h := normalizarHash(v); esHashAceStream(h) {
			return h
		}
	}
	return ""
}

// esHashAceStream comprueba la forma: 40 hex. Filtra el ruido de la query sin
// tener que mantener una lista de campos.
func esHashAceStream(h string) bool {
	if len(h) != 40 {
		return false
	}
	for _, r := range h {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
