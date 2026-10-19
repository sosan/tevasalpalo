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

	// // Para cualquier otro contenido (segmentos .ts, etc.)
	// copyHeaders(c, resp)
	// c.Status(resp.StatusCode)
	// _, err = io.Copy(c.Response().BodyWriter(), resp.Body)
	// return err
	// Configurar buffer
	const bufferSize = 60 * 1024 * 1024 // 60MB buffer
	bufferedReader := bufio.NewReaderSize(resp.Body, bufferSize)

	// Copiar headers importantes
	copyHeaders(c, resp)

	// Establecer el código de estado
	c.Status(resp.StatusCode)

	// Obtener el writer de Fiber
	fiberWriter := c.Response().BodyWriter()

	// Usar io.CopyBuffer para una copia potencialmente más eficiente con nuestro buffer
	// Creamos un buffer para io.CopyBuffer. Su tamaño puede ser el mismo o diferente.
	copyBuf := make([]byte, bufferSize)

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

	// Parsear la URL final del manifiesto para obtener la base
	manifestURL, err := url.Parse(finalManifestURL)
	if err != nil {
		// Si no se puede parsear, enviar el manifiesto sin modificar (menos ideal)
		fmt.Printf("Warning: Could not parse manifest URL '%s': %v\n", finalManifestURL, err)
		copyHeaders(c, resp)
		c.Status(resp.StatusCode)
		return c.SendString(manifestContent)
	}

	// Construir la URL base para los segmentos
	// Debe incluir el esquema, host y el directorio del manifiesto
	manifestBaseURL := &url.URL{
		Scheme: manifestURL.Scheme,
		Host:   manifestURL.Host,
		Path:   "/", // Empezamos desde la raíz, ajustaremos las rutas relativas
	}
	// Alternativamente, si las rutas son relativas al directorio del manifiesto:
	// manifestBaseURL.Path = strings.TrimSuffix(manifestURL.Path, "/") + "/"

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

func modifySegmentURLs(manifestContent string, baseURL *url.URL) string {
	lines := strings.Split(manifestContent, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Saltar líneas de comentarios y directivas
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// Si la línea parece una URL de segmento (no es una directiva)
		if strings.HasSuffix(line, ".ts") {
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

// var client = &http.Client{
// 	Timeout: 30 * time.Second,
// 	CheckRedirect: func(req *http.Request, via []*http.Request) error {
// 		// No seguir redirecciones automáticamente
// 		return http.ErrUseLastResponse
// 	},
// 	Transport: &http.Transport{
// 		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
// 	},
// }

// func fetchAndProxy(c *fiber.Ctx, targetURL string) error {
// 	client := &http.Client{
//             Timeout: 0, // Sin timeout para streams continuos
//             Transport: &http.Transport{
//                 MaxIdleConns:        100,
//                 MaxIdleConnsPerHost: 10,
//                 IdleConnTimeout:     90 * time.Second,
//             },
//         }

//         // Crear request
//         req, err := http.NewRequest("GET", targetURL, nil)
//         if err != nil {
//             return c.Status(500).SendString("Failed to create request")
//         }

//         // Headers para streaming
//         req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
//         req.Header.Set("Accept", "*/*")
//         req.Header.Set("Connection", "keep-alive")

//         // Copiar headers del cliente original
//         if rangeHeader := c.Get("Range"); rangeHeader != "" {
//             req.Header.Set("Range", rangeHeader)
//         }

//         // Hacer la petición
//         resp, err := client.Do(req)
//         if err != nil {
//             return c.Status(500).SendString("Failed to connect to stream: " + err.Error())
//         }
//         defer resp.Body.Close()

//         // Copiar headers importantes
//         if contentType := resp.Header.Get("Content-Type"); contentType != "" {
//             c.Set("Content-Type", contentType)
//         }
//         if acceptRanges := resp.Header.Get("Accept-Ranges"); acceptRanges != "" {
//             c.Set("Accept-Ranges", acceptRanges)
//         }
//         if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
//             c.Set("Content-Range", contentRange)
//         }

//         // Headers CORS esenciales para streaming
//         c.Set("Access-Control-Allow-Origin", "*")
//         c.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
//         c.Set("Access-Control-Allow-Headers", "Range, Content-Type")
//         c.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
//         c.Set("Cache-Control", "no-cache")

//         // Establecer status y comenzar streaming
//         c.Status(resp.StatusCode)

//         // Streaming continuo - copiar datos en tiempo real
//         _, err = io.Copy(c.Response().BodyWriter(), resp.Body)
//         return err
// }

// func proxyRequest(c *fiber.Ctx, targetURL string, isManifest bool) error {
//     fmt.Printf("Proxying request to: %s (manifest: %t)\n", targetURL, isManifest)

//     // Cliente HTTP configurado
//     client := &http.Client{
//         Timeout: 60 * time.Second,
//         Transport: &http.Transport{
//             MaxIdleConns:        100,
//             MaxIdleConnsPerHost: 10,
//             IdleConnTimeout:     90 * time.Second,
//         },
//     }

//     // Crear request
//     req, err := http.NewRequest("GET", targetURL, nil)
//     if err != nil {
//         return c.Status(500).SendString("Failed to create request: " + err.Error())
//     }

//     // Headers para streaming
//     req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
//     req.Header.Set("Accept", "*/*")
//     req.Header.Set("Connection", "keep-alive")

//     // Copiar headers del cliente original
//     if rangeHeader := c.Get("Range"); rangeHeader != "" {
//         req.Header.Set("Range", rangeHeader)
//     }

//     // Hacer la petición
//     resp, err := client.Do(req)
//     if err != nil {
//         return c.Status(500).SendString("Failed to fetch: " + err.Error())
//     }
//     defer resp.Body.Close()

//     // Para manifiestos HLS, necesitamos modificar las URLs
//     if isManifest && strings.Contains(resp.Header.Get("Content-Type"), "mpegurl") {
//         return handleManifest(c, resp, targetURL)
//     }

//     // Copiar headers normales
//     copyHeaders(c, resp)

//     // Enviar respuesta
//     c.Status(resp.StatusCode)
//     _, err = io.Copy(c.Response().BodyWriter(), resp.Body)
//     return err
// }

// func handleManifest(c *fiber.Ctx, resp *http.Response, manifestURL string) error {
//     // Leer el contenido del manifiesto
//     body, err := io.ReadAll(resp.Body)
//     if err != nil {
//         return c.Status(500).SendString("Failed to read manifest")
//     }

//     // Codificar la URL del manifiesto para pasarla a los segmentos
//     encodedManifestURL := base64.URLEncoding.EncodeToString([]byte(manifestURL))

//     // Modificar las URLs en el manifiesto para que apunten a nuestro proxy
//     manifestContent := string(body)

//     // Reemplazar URLs relativas y absolutas
//     lines := strings.Split(manifestContent, "\n")
//     for i, line := range lines {
//         line = strings.TrimSpace(line)
//         if strings.HasPrefix(line, "#") || line == "" {
//             continue
//         }

//         // Si es una URL de segmento, reemplazarla
//         if strings.HasSuffix(line, ".ts") {
//             // Construir URL proxy para el segmento
//             segmentProxyURL := fmt.Sprintf("/hls/segment/%s?manifest=%s",
//                 url.QueryEscape(path.Base(line)),
//                 encodedManifestURL)
//             lines[i] = segmentProxyURL
//         }
//     }

//     // Copiar headers importantes
//     copyHeaders(c, resp)
//     c.Set("Content-Type", "application/vnd.apple.mpegurl")

//     // Enviar el manifiesto modificado
//     c.Status(resp.StatusCode)
//     return c.SendString(strings.Join(lines, "\n"))
// }

// func copyHeaders(c *fiber.Ctx, resp *http.Response) {
//     // Headers importantes
//     importantHeaders := []string{
//         "Content-Type", "Content-Length", "Accept-Ranges",
//         "Content-Range", "Cache-Control", "Expires", "Last-Modified",
//     }

//     for _, header := range importantHeaders {
//         if value := resp.Header.Get(header); value != "" {
//             c.Set(header, value)
//         }
//     }

//     // Headers CORS
//     c.Set("Access-Control-Allow-Origin", "*")
//     c.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
//     c.Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept")
//     c.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
// }

// func proxySegment(c *fiber.Ctx, segmentName string) error {
//     // Parsear la URL del manifiesto para obtener la base
//     // manifestParsed, err := url.Parse(manifestURL)
//     // if err != nil {
//     //     return c.Status(400).SendString("Invalid manifest URL")
//     // }

//     // Construir la URL completa del segmento
//     // segmentPath := path.Join(path.Dir(manifestParsed.Path), segmentName)
//     // manifestParsed.Path = segmentPath
//     // segmentURL := manifestParsed.String()

//     // fmt.Printf("Proxying segment: %s\n", segmentURL)

//     // Cliente HTTP para segmentos
//     client := &http.Client{
//         Timeout: 30 * time.Second,
//         Transport: &http.Transport{
//             MaxIdleConns:        100,
//             MaxIdleConnsPerHost: 10,
//         },
//     }

//     req, err := http.NewRequest("GET", segmentURL, nil)
//     if err != nil {
//         return c.Status(500).SendString("Failed to create segment request")
//     }

//     req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
//     req.Header.Set("Accept", "*/*")
//     req.Header.Set("Connection", "keep-alive")

//     // Copiar Range header si existe
//     if rangeHeader := c.Get("Range"); rangeHeader != "" {
//         req.Header.Set("Range", rangeHeader)
//     }

//     resp, err := client.Do(req)
//     if err != nil {
//         return c.Status(500).SendString("Failed to fetch segment: " + err.Error())
//     }
//     defer resp.Body.Close()

//     // Copiar headers importantes
//     if contentType := resp.Header.Get("Content-Type"); contentType != "" {
//         c.Set("Content-Type", contentType)
//     }
//     if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
//         c.Set("Content-Length", contentLength)
//     }
//     if acceptRanges := resp.Header.Get("Accept-Ranges"); acceptRanges != "" {
//         c.Set("Accept-Ranges", acceptRanges)
//     }
//     if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
//         c.Set("Content-Range", contentRange)
//     }

//     // Headers CORS
//     c.Set("Access-Control-Allow-Origin", "*")
//     c.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
//     c.Set("Access-Control-Allow-Headers", "Range, Content-Type")

//     // Enviar segmento
//     c.Status(resp.StatusCode)
//     _, err = io.Copy(c.Response().BodyWriter(), resp.Body)
//     return err
// }

// // Función genérica para hacer proxy de cualquier petición
// func fetchAndProxy(c *fiber.Ctx, targetURL string) error {
//     // Cliente HTTP configurado para streams
//     client := &http.Client{
//         Timeout: 0, // Sin timeout para streams continuos
//         Transport: &http.Transport{
//             MaxIdleConns:        100,
//             MaxIdleConnsPerHost: 10,
//             IdleConnTimeout:     90 * time.Second,
//         },
//     }

//     // Crear request
//     req, err := http.NewRequest("GET", targetURL, nil)
//     if err != nil {
//         return c.Status(500).SendString("Failed to create request")
//     }

//     // Headers para streaming (simulando un navegador)
//     req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
//     req.Header.Set("Accept", "*/*")
//     req.Header.Set("Connection", "keep-alive")

//     // Copiar headers del cliente original si existen
//     if rangeHeader := c.Get("Range"); rangeHeader != "" {
//         req.Header.Set("Range", rangeHeader)
//     }

//     // Hacer la petición
//     resp, err := client.Do(req)
//     if err != nil {
//         return c.Status(500).SendString("Failed to connect to stream: " + err.Error())
//     }
//     defer resp.Body.Close()

//     // Copiar headers importantes
//     copyHeaders(c, resp)

//     // Establecer status y comenzar streaming
//     c.Status(resp.StatusCode)

//     // Streaming continuo - copiar datos en tiempo real
//     _, err = io.Copy(c.Response().BodyWriter(), resp.Body)
//     return err
// }

//  app.Get("/hls/manifest", func(c *fiber.Ctx) error {
//         targetURL := c.Query("url")
//         if targetURL == "" {
//             return c.Status(400).SendString("URL parameter required")
//         }

//         // Decodificar si está en base64
//         if decoded, err := base64.URLEncoding.DecodeString(targetURL); err == nil {
//             targetURL = string(decoded)
//         }

//         return proxyRequest(c, targetURL, true) // true = es manifiesto
//     })

// // Endpoint para segmentos HLS (.ts files)
// app.Get("/hls/segment/*", func(c *fiber.Ctx) error {
//     // Obtener la URL base del manifiesto de query params o headers
//     manifestURL := c.Query("manifest")
//     if manifestURL == "" {
//         return c.Status(400).SendString("manifest parameter required")
//     }

//     // Decodificar la URL del manifiesto
//     if decoded, err := base64.URLEncoding.DecodeString(manifestURL); err == nil {
//         manifestURL = string(decoded)
//     }

//     // Obtener el nombre del segmento
//     segmentName := c.Params("*")

//     // Construir la URL completa del segmento
//     baseURL, err := url.Parse(manifestURL)
//     if err != nil {
//         return c.Status(400).SendString("Invalid manifest URL")
//     }

//     // Resolver la URL relativa del segmento
//     segmentPath := path.Join(path.Dir(baseURL.Path), segmentName)
//     baseURL.Path = segmentPath
//     segmentURL := baseURL.String()

//     fmt.Printf("Proxying segment: %s\n", segmentURL)
//     return proxyRequest(c, segmentURL, false) // false = es segmento
// })

//   app.Get("/hls/:segment", func(c *fiber.Ctx) error {
//     segmentName := c.Params("segment")
//     manifestURL := c.Query("manifest")

//     if manifestURL == "" {
//         return c.Status(400).SendString("manifest parameter required")
//     }

//     fmt.Printf("Loading segment: %s from manifest: %s\n", segmentName, manifestURL)
//     return proxySegment(c, segmentName, manifestURL)
// })

// app.Get("/api/transcode", func(c *fiber.Ctx) error {
// 	originalStreamURL := c.Query("url")
// 	if originalStreamURL == "" {
// 		return c.Status(400).SendString("Missing 'url' query parameter")
// 	}

// 	// Opcional: Añadir autenticación o límites de concurrencia aquí
// 	// para evitar abusos de recursos.

// 	fmt.Printf("🎬 Iniciando transcodificación para: %s\n", originalStreamURL)

// 	// Comando de FFmpeg para transcodificar de HEVC a H.264 y servir como MPEG-TS
// 	// Ajusta los parámetros según tus necesidades de calidad/latencia/rendimiento.
// 	// ultrafast y zerolatency son buenos para baja latencia, pero consumen más CPU.
// 	// baseline profile para máxima compatibilidad.
// 	cmd := exec.Command(
// 		"ffmpeg",                // Asegúrate de que 'ffmpeg' esté en el PATH del sistema
// 		"-i", originalStreamURL, // URL del stream HEVC original
// 		"-c:v", "libx264", // Codec de video de salida (H.264)
// 		"-preset", "ultrafast", // Velocidad de codificación (puede ser ultrafast, superfast, veryfast, etc.)
// 		"-tune", "zerolatency", // Ajuste para latencia cero
// 		"-profile:v", "baseline", // Perfil H.264 compatible con más dispositivos/navegadores
// 		"-level", "3.0", // Nivel de compatibilidad
// 		"-c:a", "aac", // Codec de audio de salida (puedes usar "copy" si el audio ya es AAC/compatible)
// 		// "-crf", "28",         // Control de tasa constante (calidad vs. tamaño) - opcional, menor valor = mejor calidad
// 		"-f", "mpegts", // Formato de salida MPEG-TS (flujo continuo)
// 		"pipe:1", // Escribir la salida al stdout (descriptor de archivo 1)
// 	)

// 	// Obtener el stdout del proceso FFmpeg
// 	ffmpegStdout, err := cmd.StdoutPipe()
// 	if err != nil {
// 		fmt.Printf("❌ Error creando pipe para stdout de FFmpeg: %v\n", err)
// 		return c.Status(500).SendString("Failed to create stdout pipe for FFmpeg")
// 	}

// 	// Iniciar el proceso FFmpeg
// 	err = cmd.Start()
// 	if err != nil {
// 		fmt.Printf("❌ Error iniciando FFmpeg: %v\n", err)
// 		return c.Status(500).SendString("Failed to start FFmpeg process")
// 	}

// 	// Asegurarse de matar el proceso FFmpeg cuando la solicitud termine o falle
// 	defer func() {
// 		fmt.Println("⏹️  Finalizando proceso FFmpeg...")
// 		if cmd.Process != nil {
// 			// Intentar terminación suave primero
// 			cmd.Process.Signal(os.Interrupt)
// 			done := make(chan error, 1)
// 			go func() { done <- cmd.Wait() }()

// 			select {
// 			case <-time.After(5 * time.Second):
// 				fmt.Println("⏰ Timeout esperando a FFmpeg, forzando kill...")
// 				cmd.Process.Kill()
// 			case err := <-done:
// 				if err != nil {
// 					fmt.Printf("🎥 FFmpeg terminó con error: %v\n", err)
// 				} else {
// 					fmt.Println("✅ FFmpeg terminó correctamente.")
// 				}
// 			}
// 		}
// 	}()

// 	// Establecer headers para el stream MPEG-TS
// 	// Nota: Esto ya no es HLS, es un flujo MPEG-TS continuo.
// 	c.Set("Content-Type", "video/MP2T")
// 	c.Set("Access-Control-Allow-Origin", "*")
// 	// Chunked es importante para streaming
// 	c.Set("Transfer-Encoding", "chunked")

// 	// Enviar el código de estado 200
// 	c.Status(200)

// 	// Copiar la salida de FFmpeg directamente al cuerpo de la respuesta HTTP
// 	// Esto transmite los datos a medida que FFmpeg los produce.
// 	fmt.Println("📡 Comenzando a transmitir datos desde FFmpeg...")
// 	_, copyErr := io.Copy(c.Response().BodyWriter(), ffmpegStdout)

// 	// cmd.Wait() se maneja en el defer, pero podemos verificar errores de io.Copy
// 	if copyErr != nil && copyErr != io.EOF {
// 		fmt.Printf("⚠️ Error transmitiendo datos desde FFmpeg: %v\n", copyErr)
// 		// No podemos devolver un error HTTP aquí porque ya empezamos a escribir la respuesta
// 		// Fiber manejará la desconexión del cliente.
// 		return nil
// 	}

// 	fmt.Println("🏁 Transmisión desde FFmpeg finalizada.")
// 	return nil // Fiber manejará la desconexión del cliente
// })
