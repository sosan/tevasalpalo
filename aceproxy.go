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
func proxyChainWithDirectFallback() []string {
	chain := proxyChain()
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

// aceDo ejecuta la petición contra el engine: directa a loopback siempre;
// a remoto, cadena proxy (xray/Tor) si allowProxy o directa si no.
// Devuelve el transporte usado. Solo rota ante errores de transporte, no
// ante errores HTTP del engine (se propagan tal cual).
func aceDo(targetURL string, allowProxy bool, do func(*http.Client) (*http.Response, error)) (*http.Response, string, error) {
	transports := []string{"direct"}
	loopback := isLoopbackURL(targetURL)
	if !loopback && allowProxy {
		transports = proxyChainWithDirectFallback()
	}
	lastErr := fmt.Errorf("sin transportes disponibles")
	for _, tr := range transports {
		client := &http.Client{Timeout: 30 * time.Second}
		if tr != "direct" {
			var err error
			client, err = clientForTransport(tr)
			if err != nil {
				lastErr = err
				continue
			}
		}
		resp, err := do(client)
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
			log.Printf("🔄 URL reescrita: %s -> %s", trimmed, newLine)
			return newLine
		}
	}
	if strings.HasSuffix(trimmed, ".ts") || strings.HasSuffix(trimmed, ".m3u8") {
		if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
			newURL := fmt.Sprintf("%s/ace/%s", serverOrigin, strings.TrimPrefix(trimmed, "/"))
			log.Printf("🔄 URL relativa convertida: %s -> %s", trimmed, newURL)
			return newURL
		}
	}
	return line
}
