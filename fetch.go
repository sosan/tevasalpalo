package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type CompetitionRequest struct {
	URL     string
	Proxied bool
	Name    string
	// Timeout por source. 0 = timeTimeout. Alguna web de horarios tarda >20s
	// en responder y con el timeout global se caía siempre.
	Timeout time.Duration
}

const (
	timeTimeout = 20 * time.Second
)

// IinitializeRedirectClients devuelve el cliente para resolver URLs "p;".
// El timeout es corto a propósito: aquí solo se busca la URL final tras las
// redirecciones, no el manifiesto. Con timeTimeout (20 s) cada servidor
// directo caído costaba 20 s de arranque.
func IinitializeRedirectClients() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return http.ErrUseLastResponse
		},
		Timeout: pLinkTimeout,
	}
}

func StopRedirectClient(client *http.Client) {
	if client.Transport != nil {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

func FetchWebData(url string, proxied bool) ([]byte, error) {
	return FetchWebDataTimeout(url, proxied, timeTimeout)
}

// FetchWebDataTimeout es FetchWebData con timeout por llamada. Recorre la cadena
// de transportes (direct o la de proxy) girando al siguiente si uno falla.
func FetchWebDataTimeout(url string, proxied bool, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = timeTimeout
	}
	transports := []string{"direct"}
	if proxied {
		transports = proxyChain()
	}

	var lastErr error
	for idx, tr := range transports {
		client, err := clientForTransportTimeout(tr, timeout)
		if err != nil {
			lastErr = fmt.Errorf("transporte %s: %w", tr, err)
			continue
		}
		body, err := doFetchWithClient(client, url)
		if err != nil {
			lastErr = err
			if idx < len(transports)-1 {
				log.Printf("⚠️  %s vía %s falló, rotando transporte: %v", url, tr, err)
			}
			continue
		}
		if idx > 0 {
			log.Printf("🔄 %s OK vía transporte de reserva %s", url, tr)
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("sin transportes disponibles")
	}
	return nil, lastErr
}

func doFetchWithClient(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error al crear la solicitud: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://www.google.com/")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error al realizar la solicitud HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error al leer el cuerpo de la respuesta: %w", err)
	}

	return body, nil
}

func createSOCKS5Client() (*http.Client, error) {
	return newSOCKS5Client("localhost:" + portTor)
}

// ---------------------------------------------------------------------------
// Transporte proxy configurable (sin credenciales en el repo).
//
// PROXY_MODE:
//   - "tor"    (defecto sin XRAY_LINK/XRAY_SUB): SOCKS5 de Tor en localhost:9050.
//   - "socks5": SOCKS5 genérico en SOCKS5_ADDR (defecto si hay XRAY_LINK o
//     XRAY_SUB: xray en-proceso gestionado por la app, ver xray.go).
//     Primario xray, reserva Tor por la rotación automática.
// XRAY_SUB: URL de subscripción (lista vless/trojan/ss/wireguard, texto o base64).
//   Se prueba con tráfico real y se usa el primer endpoint vivo; XRAY_LINK
//   queda como reserva. Los enlaces nunca se guardan en el repo.
//   - "socks5": SOCKS5 genérico en SOCKS5_ADDR (p. ej. xray-core local con
//     cualquier outbound vless/trojan/shadowsocks). El usuario lanza xray
//     por su cuenta como sidecar:
//       ./xray run -c xray-client.json   # inbound socks 127.0.0.1:10808
//     y exporta PROXY_MODE=socks5 SOCKS5_ADDR=127.0.0.1:10808
//     (verificado con un trojan+ws público: 200 en <1s).
//   - "off"/"direct": ignora el flag proxied, conexión directa.
//   - Rotación automática: si el primario falla, FetchWebData prueba el otro
//     SOCKS (tor<->socks5) y luego "direct" antes de dar error. "direct" entra
//     por defecto (PROXY_FALLBACK_DIRECT=0 lo desactiva, ver proxyFallbackDirect).
//
// Los enlaces/UUIDs van en el config local de xray, nunca en este repo.
// ---------------------------------------------------------------------------

func proxyMode() string {
	m := strings.ToLower(strings.TrimSpace(os.Getenv("PROXY_MODE")))
	if m == "" {
		if xrayLink() != "" || xraySubURL() != "" {
			return "socks5" // hay xray gestionado (link o sub): primario xray, reserva Tor
		}
		return "tor"
	}
	return m
}

func socks5Addr() string {
	if addr := strings.TrimSpace(os.Getenv("SOCKS5_ADDR")); addr != "" {
		return addr
	}
	return "127.0.0.1:10808"
}

// isFatalFetchErr dice si el error no merece reintento: 4xx salvo 429
// (rate limit, que sí reintenta con backoff).
func isFatalFetchErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status code error: 4") &&
		!strings.Contains(msg, "status code error: 429")
}

// maxNetworkRetries: intentos contra la misma salida cuando el fallo es de
// red (DNS/conexión) antes de rotating de transporte.
const maxNetworkRetries = 3

// isNetworkErr detecta fallos de transporte: el resolver local no encuentra el
// host, o no hay quien acepte la conexión. Reintentar 10 veces contra la MISMA
// salida no lo arregla (el resolver local sigue roto), pero cambiar de
// transporte sí: el proxy resuelve el DNS por su cuenta. Medido con
// ipfs.filebase.io: 32 s de reintentos por directa y 1 s por proxy.
func isNetworkErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range []string{
		"no such host",
		"server misbehaving",
		"connection refused",
		"connection reset",
		"no route to host",
		"network is unreachable",
		"i/o timeout",
		"Client.Timeout exceeded",
		// Solo "unexpected EOF", no un "EOF" a secas: casa con errores de
		// parseo propios ("error al parsear el HTML: EOF inesperado") que no
		// son de red.
		"unexpected EOF",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// shouldProxyFallback decide el último recurso para fuentes directas
// agotadas sin error fatal (p. ej. 429 persistente = IP limitada).
func shouldProxyFallback(s Source, success, fatal bool) bool {
	return !success && !fatal && !s.Proxied
}

// proxyChain orden de transportes a probar para peticiones proxied.
// El secundario es el otro SOCKS disponible. "direct" entra al final salvo que
// PROXY_FALLBACK_DIRECT esté desactivado (ver proxyFallbackDirect).
func proxyChain() []string {
	var chain []string
	switch proxyMode() {
	case "off", "direct", "none":
		return []string{"direct"}
	case "socks5", "xray":
		chain = []string{"socks5", "tor"}
	default:
		chain = []string{"tor", "socks5"}
	}
	if proxyFallbackDirect() {
		chain = append(chain, "direct")
	}
	return chain
}

// proxyFallbackDirect decide si "direct" se añade al final de la cadena de
// proxy. Por defecto SÍ: si xray o Tor no están levantados, la petición se
// degrada a conexión directa en vez de romperse (antes cero fuentes proxeadas
// funcionaban en una instalación limpia, porque socks5 y Tor dan "connection
// refused" y no había red de seguridad).
//
// PROXY_FALLBACK_DIRECT=0/false/no/off lo desactiva, para cuando la IP real no
// debe salir nunca y preferimos un fallo explícito a salir sin anonimizar.
func proxyFallbackDirect() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROXY_FALLBACK_DIRECT"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// clientForTransport construye el cliente HTTP de un transporte de la cadena.
func clientForTransport(name string) (*http.Client, error) {
	return clientForTransportTimeout(name, timeTimeout)
}

func clientForTransportTimeout(name string, timeout time.Duration) (*http.Client, error) {
	switch name {
	case "direct":
		return &http.Client{Timeout: timeout}, nil
	case "socks5":
		return newSOCKS5ClientTimeout(socks5Addr(), timeout)
	default: // "tor" y desconocidos: comportamiento anterior
		return newSOCKS5ClientTimeout("localhost:"+portTor, timeout)
	}
}

func newSOCKS5Client(addr string) (*http.Client, error) {
	return newSOCKS5ClientTimeout(addr, timeTimeout)
}

func newSOCKS5ClientTimeout(addr string, timeout time.Duration) (*http.Client, error) {
	sockURL, err := url.Parse("socks5://" + addr)
	if err != nil {
		return nil, err
	}
	dialer, err := proxy.FromURL(sockURL, proxy.Direct)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
		Timeout: timeout,
	}, nil
}

// func getSockList() ([]string, error) {
// 	proxyListURL := "https://raw.githubusercontent.com/proxifly/free-proxy-list/main/proxies/protocols/socks5/data.txt"
// 	body, err := FetchWebData(proxyListURL, false)
// 	strBody := strings.ReplaceAll(string(body), "socks5://", "")
// 	proxiesNotChecked := strings.Split(strBody, "\n")

// 	if len(proxiesNotChecked) == 0 {
// 		fmt.Println("No se encontraron proxies en la lista.")
// 		return nil, fmt.Errorf("error al obtener la lista de proxies: %w", err)
// 	}

// 	fmt.Printf("Se encontraron %d proxies. Probando...\n", len(proxiesNotChecked))
// 	return proxiesNotChecked, nil
// 	// var proxies []string
// 	// for _, proxyAddr := range proxiesNotChecked {
// 	// 	proxies = append(proxies, proxyAddr)
// 	// 	// if testSOCKS5Proxy(proxyAddr) {
// 	// 	// 	// return proxyAddr, nil
// 	// 	// }
// 	// }

// 	// return proxies, nil
// }

// func testSOCKS5Proxy(proxyAddr string) bool {
// 	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
// 	if err != nil {
// 		return false
// 	}

// 	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	// defer cancel()

// 	conn, err := dialer.Dial("tcp", "httpbin.org:80")
// 	if err != nil {
// 		return false
// 	}
// 	conn.Close()
// 	return true
// }

func testSOCKS5Proxy(proxyAddr string) bool {
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return false
	}

	result := make(chan bool, 1)
	go func() {
		conn, err := dialer.Dial("tcp", "httpbin.org:80")
		if err != nil {
			result <- false
			return
		}
		defer conn.Close()

		request := "GET / HTTP/1.0\r\nHost: httpbin.org\r\n\r\n"
		_, err = conn.Write([]byte(request))
		if err != nil {
			result <- false
			return
		}

		response := make([]byte, 100) // Leer solo los primeros 100 bytes
		_, err = conn.Read(response)
		if err != nil {
			result <- false
			return
		}
		// fmt.Printf("Proxy %s responded with: %s\n", proxyAddr, string(response[:n])) // Opcional: para depuración

		result <- true
	}()

	select {
	case success := <-result:
		return success
	case <-time.After(timeTimeout): // Usar el timeout aquí también
		return false
	}
}

func FetchCompetitionsParallel(requests []CompetitionRequest, getFunc func(req CompetitionRequest) ([]DayView, error)) map[string][]DayView {
	results := make(map[string][]DayView)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, req := range requests {
		wg.Add(1)
		go func(req CompetitionRequest) {
			defer wg.Done()
			events, err := getFunc(req)
			if err != nil {
				log.Printf("❌ Error en %s: %v", req.Name, err)
				return
			}
			mu.Lock()
			results[req.Name] = events
			mu.Unlock()
		}(req)
	}

	wg.Wait()
	return results
}


// func fetchWithRedirects(initialURL string, proxified bool) (finalURL string, finalHeaders http.Header, manifestBody []byte, err error) {
// 	client := &http.Client{
// 		CheckRedirect: func(req *http.Request, via []*http.Request) error {
// 			if len(via) >= 10 {
// 				return fmt.Errorf("stopped after 10 redirects")
// 			}
			
// 			return http.ErrUseLastResponse
// 		},
// 		Timeout: timeTimeout,
// 	}

// 	if proxified {
// 		client, err = createSOCKS5Client()
// 	}

// 	// client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
// 	// 	if len(via) >= 10 {
// 	// 		return fmt.Errorf("stopped after 10 redirects")
// 	// 	}
// 	// 	return http.ErrUseLastResponse
// 	// }

// 	currentURL := initialURL
// 	redirectCount := 0

// 	for {
// 		req, err := http.NewRequest("GET", currentURL, nil)
// 		if err != nil {
// 			return "", nil, nil, fmt.Errorf("failed to create request for %s: %w", currentURL, err)
// 		}

// 		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
// 		req.Header.Set("Accept", "*/*")
// 		req.Header.Set("Connection", "keep-alive")

// 		// Hacer la solicitud
// 		resp, err := client.Do(req)
// 		if err != nil {
// 			return "", nil, nil, fmt.Errorf("failed to fetch %s: %w", currentURL, err)
// 		}

// 		defer resp.Body.Close() 

// 		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
// 			redirectCount++
// 			if redirectCount > 10 {
// 				return "", nil, nil, fmt.Errorf("too many redirects (>10)")
// 			}

// 			location := resp.Header.Get("Location")
// 			if location == "" {
// 				return "", nil, nil, fmt.Errorf("redirect status %d received but no Location header found for URL %s", resp.StatusCode, currentURL)
// 			}
// 			currentURL = location
// 			continue
// 		}

// 		body, err := io.ReadAll(resp.Body)
// 		if err != nil {
// 			return "", nil, nil, fmt.Errorf("failed to read manifest body from %s: %w", currentURL, err)
// 		}

// 		return resp.Request.URL.String(), resp.Header, body, nil
// 	}
// }

func fetchWithRedirects(initialURL string, client *http.Client) (finalURL string, finalHeaders http.Header, manifestBody []byte, err error) {
	currentURL := initialURL
	redirectCount := 0

	for {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to create request for %s: %w", currentURL, err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")

		resp, err := client.Do(req) // <--- Usar el cliente pasado como parámetro
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to fetch %s: %w", currentURL, err)
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			redirectCount++
			if redirectCount > 10 {
				return "", nil, nil, fmt.Errorf("too many redirects (>10)")
			}

			location := resp.Header.Get("Location")
			if location == "" {
				return "", nil, nil, fmt.Errorf("redirect status %d received but no Location header found for URL %s", resp.StatusCode, currentURL)
			}
			currentURL = location
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to read manifest body from %s: %w", currentURL, err)
		}

		return resp.Request.URL.String(), resp.Header, body, nil
	}
}
