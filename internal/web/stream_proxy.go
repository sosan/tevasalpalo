// Pipeline del proxy de streaming: helpers de respuesta, reescritura de
// manifest HLS y el handler de rutas de AceStream que reúne gateway, WARP y
// reintentos.

package web

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"net/http"
	"net/url"

	"tevasalpalo/internal/ace"
	"tevasalpalo/internal/store"
	"tevasalpalo/internal/transport"
)

func fromBase64Url(str string) (string, error) {
	// Añadir padding si es necesario
	padding := 4 - (len(str) % 4)
	if padding != 4 {
		str += strings.Repeat("=", padding)
	}

	// Reemplazar caracteres de URL-safe base64
	str = strings.ReplaceAll(str, "-", "+")
	str = strings.ReplaceAll(str, "_", "/")

	// Decodificar base64
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

// Copiar headers importantes de la respuesta
func copyHeaders(c fiber.Ctx, resp *http.Response) {
	// Headers comunes
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Set("Content-Type", contentType)
	}
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		c.Set("Content-Length", contentLength)
	}
	if acceptRanges := resp.Header.Get("Accept-Ranges"); acceptRanges != "" {
		c.Set("Accept-Ranges", acceptRanges)
	}
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		c.Set("Content-Range", contentRange)
	}
	if cacheControl := resp.Header.Get("Cache-Control"); cacheControl != "" {
		c.Set("Cache-Control", cacheControl)
	}

	// Headers CORS esenciales para streaming
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	c.Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept, Authorization")
	c.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
}

func fetchAndProxy(c fiber.Ctx, targetURL string) error {
	// WARP on-demand: equivalencia de http_stream_start en el cliente Android.
	transport.TraceLog("fetchAndProxy: %s (WARP=%v)", targetURL, transport.WarpEnabled())
	transport.WarpStreamStart()
	resp, finalURL, err := ace.OpenUpstream(targetURL, c.Get("Range"), ace.MediaTransports())
	if err != nil {
		transport.WarpStreamStop()
		if ace.ProxyMediaEnabled() {
			return c.Status(502).SendString("Sin transporte multimedia (ni proxies ni directa): " + err.Error())
		}
		return c.Status(500).SendString("Failed to connect to stream: " + err.Error())
	}
	defer func() {
		resp.Body.Close()
		// http_stream_stop del cliente Android: sin reproducción, WARP para
		// a los 60 s (ver WarpStreamStop).
		transport.WarpStreamStop()
	}()

	return serveUpstream(c, resp, finalURL, targetURL)
}

func serveUpstream(c fiber.Ctx, resp *http.Response, finalURL, targetURL string) error {
	// Verificar si la respuesta es un manifiesto M3U8
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "mpegurl") || strings.Contains(contentType, "m3u") || strings.HasSuffix(strings.ToLower(finalURL), ".m3u8") {
		return handleManifest(c, resp, finalURL) // Pasar la URL final después de redirecciones
	}

	// Buffer de lectura/copia para segmentos: el tamaño del buffer no limita
	// el tamaño del vídeo, solo es el trozo de tránsito por cada copia, así
	// que el vídeo pasa entero sea cual sea su duración. 256 KB es margen
	// cómodo frente al buffer de 60 MB de antes, que se asignaba por petición.
	// Copiar headers importantes
	copyHeaders(c, resp)

	// Establecer el código de estado
	c.Status(resp.StatusCode)

	// Obtener el writer de Fiber
	fiberWriter := c.Response().BodyWriter()

	const bufferSize = 256 * 1024
	copyBuf := make([]byte, bufferSize)
	bufferedReader := bufio.NewReaderSize(resp.Body, bufferSize)

	// fmt.Printf("📡 Iniciando transmisión buffered de %s\n", targetURL)
	_, err := io.CopyBuffer(fiberWriter, bufferedReader, copyBuf)
	if err != nil {
		fmt.Printf("⚠️ Error en transmisión buffered de %s: %v\n", targetURL, err)
		// No devolvemos error HTTP porque la escritura ya pudo haber comenzado
		return nil
	}
	// fmt.Printf("✅ Transmisión buffered finalizada para %s\n", targetURL)

	return nil
}

func handleManifest(c fiber.Ctx, resp *http.Response, finalManifestURL string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).SendString("Failed to read manifest: " + err.Error())
	}

	manifestContent := string(body)

	// Construir la URL base para los segmentos. Debe resolver las rutas
	// relativas contra el directorio del manifiesto: contra la raíz ("/"), el
	// segmento "seg-1.ts" de "http://host/ace/hash/segs/lista.m3u8" terminaba
	// pedido como "http://host/seg-1.ts" y no se cargaba nada.
	manifestBaseURL, err := baseDelManifiesto(finalManifestURL)
	if err != nil {
		// Si no se puede parsear, enviar el manifiesto sin modificar (menos ideal)
		fmt.Printf("Warning: Could not parse manifest URL '%s': %v\n", finalManifestURL, err)
		copyHeaders(c, resp)
		c.Status(resp.StatusCode)
		return c.SendString(manifestContent)
	}

	// fmt.Printf("Manifest base URL for segments: %s\n", manifestBaseURL.String())

	// Modificar el manifiesto para hacer las URLs de segmentos absolutas
	modifiedContent := modifySegmentURLs(manifestContent, manifestBaseURL)

	// Establecer los headers, asegurando el tipo MIME correcto
	c.Set("Content-Type", "application/vnd.apple.mpegurl") // o resp.Header.Get("Content-Type")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Cache-Control", "no-cache") // Los manifiestos HLS suelen ser dinámicos
	// Copiar otros headers relevantes si es necesario
	if acceptRanges := resp.Header.Get("Accept-Ranges"); acceptRanges != "" {
		c.Set("Accept-Ranges", acceptRanges)
	}

	c.Status(resp.StatusCode)
	return c.SendString(modifiedContent)
}

// baseDelManifiesto localiza el directorio del manifest como base para
// resolver URLs de segmento relativas: con Path en la raíz cualquier segmento
// relativo acababa pedido contra la raíz del host y no contra su directorio.
func baseDelManifiesto(manifestURL string) (*url.URL, error) {
	u, err := url.Parse(manifestURL)
	if err != nil {
		return nil, err
	}
	directorio := path.Dir(u.Path)
	if !strings.HasSuffix(directorio, "/") {
		directorio += "/"
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: directorio}, nil
}

func modifySegmentURLs(manifestContent string, baseURL *url.URL) string {
	lines := strings.Split(manifestContent, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Saltar líneas de comentarios y directivas
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// Si la línea parece una URL de segmento (no es una directiva). El
		// sufijo se comprueba sin la query: "seg-1.ts?token=x" terminaba en
		// ".ts?token=x" y no se reescribía. .m4s cubre los manifests que usan
		// fragmentación fMP4 en lugar de TS.
		ruta := line
		if j := strings.IndexByte(ruta, '?'); j >= 0 {
			ruta = ruta[:j]
		}
		ruta = strings.ToLower(ruta)
		if strings.HasSuffix(ruta, ".ts") || strings.HasSuffix(ruta, ".m4s") || strings.HasSuffix(ruta, ".m3u8") {
			// Parsear la URL del segmento. Si es relativa, url.Parse la resolverá
			// contra la baseURL que le demos.
			segmentURL, err := url.Parse(line)
			if err != nil {
				fmt.Printf("Warning: Could not parse segment URL '%s': %v\n", line, err)
				continue // Dejar la línea original si no se puede parsear
			}

			// Resolver la URL del segmento contra la baseURL del manifiesto
			absoluteSegmentURL := baseURL.ResolveReference(segmentURL)

			// fmt.Printf("Modified segment URL: '%s' -> '%s'\n", line, absoluteSegmentURL.String())
			lines[i] = absoluteSegmentURL.String()
		}
	}
	return strings.Join(lines, "\n")
}

func encodeContent(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

func handleAceManifest(c fiber.Ctx, resp *http.Response, engineHosts []string, hash string) error {
	// Leer el contenido del manifest
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Error leyendo manifest: %v", err)
		return c.Status(500).SendString("Error leyendo manifest: " + err.Error())
	}

	manifestContent := string(body)

	// Obtener el origen del servidor (http://127.0.0.1:3000 o el que sea)
	scheme := "http"
	if c.Protocol() == "https" {
		scheme = "https"
	}
	host := string(c.Request().Host())
	if host == "" {
		host = "127.0.0.1:3000" // Fallback
	}

	serverOrigin := fmt.Sprintf("%s://%s", scheme, host)

	// Reescribir las URLs en el manifest (hosts del engine -> origen local).
	// Se cuenta cuántas cambian en vez de loguear una línea por segmento: con
	// 25 segmentos y un refresco cada 30s eran 25 líneas cada medio minuto, y
	// el total de telemetría era eso y nada más. El detalle por URL queda
	// detrás de la traza.
	lines := strings.Split(manifestContent, "\n")
	reescritas := 0
	for i, line := range lines {
		nueva := ace.RewriteAceManifestLine(line, serverOrigin, host, engineHosts)
		if nueva != line {
			reescritas++
		}
		lines[i] = nueva
	}

	log.Printf("📝 Manifest reescrito: %d URLs de %d -> origen %s",
		reescritas, len(lines), serverOrigin)

	// Lo que dice el manifest es lo único que hay para saber si el stream
	// sirve algo: uno con segmentos dentro demuestra que hay peers y uno vacío
	// que no hay nada que reproducir. Es una señal débil a propósito, así que
	// pesa poco y no hunde un enlace por un refresco en unlucky.
	if hash != "" && resp.StatusCode == 200 {
		if reescritas > 0 {
			store.Global.RegistrarEvidencia(hash, store.EvPlaylistOK)
		} else {
			store.Global.RegistrarEvidencia(hash, store.EvPlaylistVacia)
		}
	}

	modifiedManifest := strings.Join(lines, "\n")

	// Establecer headers
	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Set("Access-Control-Allow-Headers", "*")
	c.Set("Cache-Control", "no-cache")

	c.Status(resp.StatusCode)
	return c.SendString(modifiedManifest)
}
