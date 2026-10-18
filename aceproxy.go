package main

// Acceso al motor AceStream local o remoto a través de la cadena proxy.
//
// - ACESTREAM_API (defecto auto-detectado): base del engine. Con un
//   motor remoto (p. ej. un VPS sin throttling) todo el tráfico HTTP entre
//   esta app y el engine viaja por la cadena de transportes (xray/Tor), así
//   los streams se ven a través de los clientes proxy. El P2P ocurre en el
//   remoto; aquí solo viaja HTTP.
//   Sin la variable, la base se descubre al primer uso sondeando el 6878:
//   127.0.0.1 (la app lanza el motor), luego host.docker.internal y por
//   último el gateway de la ruta por defecto del contenedor (en Docker es el
//   host, donde el compose publica el puerto del servicio acestream).
//   En compose con engine como servicio hermano basta ACESTREAM_API=
//   http://acestream:6878 para no depender del descubrimiento.
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"
)

// acestreamAPI devuelve la base del engine sin barra final.
//
// ACESTREAM_API manda. Si está vacía, la base se descubre una sola vez al
// primer uso (sync.Once, no depende de que llamen); es lo que hace que la app
// arranque contra el engine sin configurar nada:
//
//   - 127.0.0.1: la app misma lanza el motor (RunAceStream) o corre en el host.
//     Se comprueba primero y si el puerto responde, nada más toca.
//   - host.docker.internal: Docker Desktop / Linux con extra_hosts.
//   - gateway de la ruta por defecto del contenedor: en Docker es el host, y
//     el compose publica el 6878 del servicio acestream (6878:6878). Este es
//     el caso que se rompía: dentro de un contenedor 127.0.0.1 no tiene salida
//     hacia el host, y el engine vive en el host o en el servicio hermano.
//
// Si nada responde, se queda en loopback (comportamiento anterior). El
// descubrimiento toca red solo una vez: los candidatos se sondean con un dial
// corto, no peticiones HTTP.
func acestreamAPI() string {
	if base := strings.TrimSpace(os.Getenv("ACESTREAM_API")); base != "" {
		return strings.TrimSuffix(base, "/")
	}
	acestreamBaseOnce.Do(func() {
		acestreamBaseFall = baseEngineDescubierta()
	})
	return acestreamBaseFall
}

var (
	acestreamBaseOnce sync.Once
	acestreamBaseFall string
)

// baseEngineDescubierta elige la base del engine sondeando el puerto una vez.
// Si nada responde, loopback, como siempre.
func baseEngineDescubierta() string {
	return baseEngineConSondeo(httpPort, engineDialHost, gatewayIPPorDefecto)
}

// baseEngineConSondeo es la lógica pura del descubrimiento: sondea el puerto
// del engine en 127.0.0.1, host.docker.internal y (si existe) el gateway de
// la ruta por defecto, y devuelve la primera base que responde. El gateway
// llega como función para que las pruebas no lean /proc net real.
func baseEngineConSondeo(puerto int, sondear func(string, int) bool, gateway func() (string, bool)) string {
	candidatos := []struct {
		host   string
		origen string
	}{
		{"127.0.0.1", "loopback"},
		{"host.docker.internal", "Docker extra_hosts"},
	}
	if gw, ok := gateway(); ok {
		candidatos = append(candidatos, struct {
			host   string
			origen string
		}{gw, "gateway de la ruta por defecto"})
	}
	for _, c := range candidatos {
		if sondear(c.host, puerto) {
			log.Printf("🔧 ACESTREAM_API vacía: engine en http://%s:%d (%s)", c.host, puerto, c.origen)
			return fmt.Sprintf("http://%s:%d", c.host, puerto)
		}
	}
	log.Printf("⚠️  ACESTREAM_API vacía y ningún candidato responde en el puerto %d (127.0.0.1, host.docker.internal, gateway); base por defecto: 127.0.0.1", puerto)
	return fmt.Sprintf("http://127.0.0.1:%d", puerto)
}

// engineDialHost sondea el puerto TCP de un host (resuelve DNS también),
// para ver si ahí vive el engine. Dial corto: si no responde ya, no va a
// responder al pedirle el manifest tampoco.
func engineDialHost(host string, puerto int) bool {
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	conn, err := (&net.Dialer{Timeout: timeDialEngine}).Dial("tcp", net.JoinHostPort(addrs[0], strconv.Itoa(puerto)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// timeDialEngine: timeout del dial de sondeo. El engine es local/red local:
// más largo es solo retraso del usuario esperando el arranque.
const timeDialEngine = 1 * time.Second

// gatewayIPPorDefecto devuelve la IP del gateway de la ruta por defecto, que
// en Docker es el host (donde el engine suele estar publicado en 6878).
// Se cachea: la tabla de rutas del contenedor no cambia en ejecución y
// decidir direct/proxy lee esto en cada salto del engine.
func gatewayIPPorDefecto() (string, bool) {
	gatewayIPOnce.Do(func() {
		gatewayIPCache, gatewayIPOK = gatewayFromRouteFile("/proc/net/route")
	})
	return gatewayIPCache, gatewayIPOK
}

var (
	gatewayIPOnce  sync.Once
	gatewayIPCache string
	gatewayIPOK    bool
)

// esEngineLocalDirecto dice si una URL de /ace/* apunta a infraestructura del
// propio entorno y debe ir SIEMPRE directa (nunca por Tor/xray): loopback,
// host.docker.internal o la IP del gateway del contenedor (la dirección del
// host en Docker). El host configurado con ACESTREAM_API no entra aquí a
// propósito: si es un VPS remoto, ese tráfico sí va por la cadena.
func esEngineLocalDirecto(raw string) bool {
	if isLoopbackURL(raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	switch h {
	case "", "host.docker.internal":
		return h != ""
	}
	if gw, ok := gatewayIPPorDefecto(); ok && h == gw {
		return true
	}
	// El host de la base descubierta (env vacía) ya solo puede ser loopback
	// (arriba), host.docker.internal o el propio gateway: cubierto.
	return false
}

// gatewayFromRouteFile lee un fichero estilo /proc/net/route y devuelve la IP
// del gateway de la entrada de destino 00000000 (la ruta por defecto), o false
// si no existe (Windows/macOS nativo, donde 127.0.0.1 sigue siendo correcto)
// o no hay ruta por defecto con gateway.
//
// Formato de cada línea (Iface la columna 0, Destination la 1, Gateway la 2):
//
//	Iface Destination Gateway    Flags ...
//	eth0  00000000    010011AC   ...
//
// El gateway va como dword little-endian en hex: "010011AC" -> bytes
// 01 00 11 AC en orden inverso -> AC.11.00.01 -> 172.17.0.1.
func gatewayFromRouteFile(rutaFichero string) (string, bool) {
	data, err := os.ReadFile(rutaFichero)
	if err != nil {
		return "", false
	}
	for _, línea := range strings.Split(string(data), "\n") {
		campos := strings.Fields(línea)
		if len(campos) < 3 {
			continue
		}
		if campos[0] == "Iface" || campos[1] != "00000000" {
			continue
		}
		if len(campos[2]) != 8 {
			continue
		}
		var b [4]byte
		for i := 0; i < 4; i++ {
			byteVal, err := strconv.ParseUint(campos[2][i*2:i*2+2], 16, 8)
			if err != nil {
				return "", false
			}
			b[3-i] = byte(byteVal)
		}
		return net.IPv4(b[0], b[1], b[2], b[3]).String(), true
	}
	return "", false
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
	local := esEngineLocalDirecto(targetURL)
	if !local && allowProxy {
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

// hashDeRutaAce saca el infohash de cualquier ruta /ace/* por la que pasen
// datos de un stream, incluidos los manifests.
//
// Es lo que completa a hashDePeticionAce para poder APRENDER de los enlaces:
// hasta ahora solo se anotaban getstream y /h/, que HLS.js no pide nunca, así
// que el bucket hashes no recibía nada. Ahora sí se puede saber de un enlace
// por el camino que el player usa de verdad:
//
//	m/<hash>/<hash>.m3u8    manifest anidado (la URL a la que redirige el principal)
//	c/<hash>/0.ts           segmento, que es la señal más fuerte
//	manifest.m3u8?id=<hash> manifest principal cuando el engine no redirige
//	getstream?id=<hash>     datos del stream
//	h/<hash>/...            datos del stream
//
// Con redirect=true se mira también la ruta final, que es la que lleva el hash
// en el caso normal del manifest principal.
func hashDeRutaAce(acePath, queryString string, conRedirect bool, rutaFinal string) string {
	p := strings.ToLower(acePath)
	// m/<hash>/... y c/<hash>/...
	for _, pre := range []string{"m/", "c/", "h/"} {
		if strings.HasPrefix(p, pre) {
			resto := p[len(pre):]
			if j := strings.IndexAny(resto, "/?"); j >= 0 {
				resto = resto[:j]
			}
			if h := normalizarHash(resto); esHashAceStream(h) {
				return h
			}
		}
	}
	// El manifest principal lleva el hash solo en la query, pero la ruta a la
	// que redirige (m/<hash>/...) es la que de verdad lo dice.
	if h := hashDeQuery(queryString); h != "" && strings.Contains(p, "manifest") {
		return h
	}
	if conRedirect && rutaFinal != "" {
		f := strings.ToLower(rutaFinal)
		for _, pre := range []string{"m/", "c/", "h/"} {
			if strings.HasPrefix(f, pre) {
				resto := f[len(pre):]
				if j := strings.IndexAny(resto, "/?"); j >= 0 {
					resto = resto[:j]
				}
				if h := normalizarHash(resto); esHashAceStream(h) {
					return h
				}
			}
		}
	}
	return hashDePeticionAce(acePath, queryString)
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
