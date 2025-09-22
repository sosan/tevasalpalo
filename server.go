package main

import (
	"bufio"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"

	"strings"
	"sync"

	"log"
	"main/update"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html/v2"
)

//go:embed views/*
//go:embed views/*/*
//go:embed views/css/*.*
//go:embed views/*.html
var viewsFS embed.FS

var (
	cachedData     CachedData
	dataMutex      sync.RWMutex
	lastDataUpdate time.Time
)

type CachedData struct {
	Days                []DayView
	DaysJSON            template.JS
	TopCompetitionsJSON template.JS
	AllCompetitions     AllCompetitions
	TopCompetitions     map[string]CompetitionDetail
	Broadcasters        map[string]BroadcasterInfo
}

// Pre-cargar datos al iniciar
func preloadData() error {
	days, daysJSON, topCompetitionsJSON, err := fetchEvents()
	if err != nil {
		return fmt.Errorf("error preloading data: %v", err)
	}

	dataMutex.Lock()
	cachedData = CachedData{
		Days:                days,
		DaysJSON:            *daysJSON,
		TopCompetitionsJSON: *topCompetitionsJSON,
		AllCompetitions:     allCompetitions,
		TopCompetitions:     topCompetitions,
		Broadcasters:        broadcasterToAcestream,
	}
	lastDataUpdate = time.Now()
	dataMutex.Unlock()

	log.Println("✅ Datos pre-cargados en memoria")
	return nil
}

// Refrescar datos periódicamente
func startDataRefresh() {
	ticker := time.NewTicker(6 * time.Hour)
	go func() {
		for range ticker.C {
			if err := preloadData(); err != nil {
				log.Printf("⚠️  Error refrescando datos: %v", err)
			}
		}
	}()
}

func StartWebServer() (*fiber.App, error) {
	topCompetitions = transformCompetitionsToTop(allCompetitions)

	if err := preloadData(); err != nil {
		log.Printf("⚠️  Advertencia: No se pudieron pre-cargar datos: %v", err)
	}

	startDataRefresh()

	engine := html.NewFileSystem(http.FS(viewsFS), ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "http://localhost:3000",
		AllowMethods: "GET,POST,PUT,DELETE",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	app.Use("/css", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/css",
		Browse:     false,
	}))

	app.Use("/images", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/images",
		Browse:     false,
	}))

	app.Use("/player/js", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/js",
		Browse:     false,
	}))

	app.Use("/player/css", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/css",
		Browse:     false,
	}))

	app.Use("/js", filesystem.New(filesystem.Config{
		Root:       http.FS(viewsFS),
		PathPrefix: "/views/js",
		Browse:     false,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()
		return c.Render("index", fiber.Map{
			"Days":                data.Days,
			"allCompetitions":     data.AllCompetitions,
			"topCompetitions":     data.TopCompetitions,
			"DaysJSON":            data.DaysJSON,
			"topCompetitionsJSON": data.TopCompetitionsJSON,
		})
	})

	app.Get("/broadcasters", func(c *fiber.Ctx) error {
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()

		return c.Render("views/broadcasters", fiber.Map{
			"Broadcasters":        data.Broadcasters,
			"DaysJSON":            data.DaysJSON,
			"Days":                data.Days,
			"allCompetitions":     data.AllCompetitions,
			"topCompetitions":     data.TopCompetitions,
			"topCompetitionsJSON": data.TopCompetitionsJSON,
		})
	})

	app.Get("/refresh-data", func(c *fiber.Ctx) error {
		if err := preloadData(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Datos actualizados correctamente",
		})
	})

	app.Get("/player/:id", func(c *fiber.Ctx) error {
		acestreamId := c.Params("id")
		if acestreamId == "" {
			fmt.Printf("Error obteniendo content_id para stream_id %s\n", acestreamId)
			return c.Status(500).SendString("No se pudo obtener el content ID")
		}
		// var err error
		// var uri string
		// if strings.Contains(acestreamId, ";") {
		// 	channel := strings.Split(acestreamId, ";")[1]
		// 	uri, err = fromBase64Url(channel)
		// 	if strings.Contains(uri, "p;") {
		// 		uri = strings.Split(uri, "p;")[1]
		// 	}
		// 	log.Print(uri)
		// }

		return c.Render("views/player", fiber.Map{
			"AcestreamId": acestreamId,
			"Error":       nil,
		})
	})

	app.Get("/update", func(c *fiber.Ctx) error {
		go func() {
			err := update.ForceUpdate()
			if err != nil {
				log.Print("ERROR | No es posible actualizarse")
				return
			}
			time.Sleep(3 * time.Second)
			shutdownChan <- struct{}{}
		}()

		return c.JSON(fiber.Map{
			"sendedupdate": true,
		})
	})

	app.Get("/healthz", func(c *fiber.Ctx) error {
		if update.Updated {
			return c.JSON(fiber.Map{
				"ok": true,
			})
		}
		return c.JSON(fiber.Map{
			"ok": false,
		})
	})

	app.Get("/updateavailable", func(c *fiber.Ctx) error {
		needUpdate := update.GetNeedUpdate()
		return c.JSON(fiber.Map{
			"needUpdate": needUpdate,
		})
	})

	// Endpoint de fallback para cualquier .ts
	app.Get("/hls/:segment", func(c *fiber.Ctx) error {
		segmentName := c.Params("segment")
		log.Print(segmentName)
		return c.SendStatus(200)
	})

	app.Get("/api/iptv/:channel", func(c *fiber.Ctx) error {
		channel := c.Params("channel")
		var targetURL string
		var err error
		if strings.Contains(channel, ";") {
			channel = strings.Split(channel, ";")[1]
			targetURL, err = fromBase64Url(channel)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{
					"error": "Invalid base64 URL",
				})
			}
			if targetURL == "" {
				return c.Status(400).JSON(fiber.Map{
					"error": "Channel not found",
				})
			}
			if strings.Contains(targetURL, "p;") {
				targetURL = strings.Replace(targetURL, "p;", "", 1)
			}
		}
		return fetchAndProxy(c, targetURL)
	})

	go func() {
		if err := app.Listen("0.0.0.0:3000"); err != nil {
			log.Printf("❌ Error en servidor web: %v", err)
		}
	}()

	return app, nil
}

func fetchEvents() ([]DayView, *template.JS, *template.JS, error) {
	// TODO: retry si da err 520, retry
	var err error
	var days []DayView
	for i := 1; i < 10; i++ {
		days, err = fetchScheduleMatchesFutbolEnCasa()
		if err != nil {
			// Loggear el error completo para depuración
			log.Printf("Error fetching schedule: %v", err)
			time.Sleep(3 * time.Second)
			continue
			// Devolver un error al cliente
			// return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
		}
		break
	}

	if err != nil {
		// Loggear el error completo para depuración
		log.Printf("Error fetching schedule: %v", err)
		// Devolver un error al cliente
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}

	daysJSONBytes, err := json.Marshal(days)
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}
	daysJSON := template.JS(daysJSONBytes)

	topCompetitionsBytes, err := json.Marshal(topCompetitions)
	if err != nil {
		log.Printf("Error marshaling days to JSON: %v", err)
		return nil, nil, nil, fmt.Errorf("Error al obtener la programación")
	}
	topCompetitionsJSON := template.JS(topCompetitionsBytes)
	return days, &daysJSON, &topCompetitionsJSON, nil
}

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
func copyHeaders(c *fiber.Ctx, resp *http.Response) {
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

func fetchAndProxy(c *fiber.Ctx, targetURL string) error {
	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return c.Status(500).SendString("Failed to create request: " + err.Error())
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	if rangeHeader := c.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		return c.Status(500).SendString("Failed to connect to stream: " + err.Error())
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
			return c.Status(500).SendString("Failed to parse base URL for redirect: " + err.Error())
		}
		newURL, err := baseURL.Parse(location)
		if err != nil {
			return c.Status(500).SendString("Failed to parse redirect location: " + err.Error())
		}

		// fmt.Printf("Following redirect from '%s' to '%s'\n", targetURL, newURL.String())
		targetURL = newURL.String()

		// Crear una nueva solicitud para la URL redirigida
		req, err = http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return c.Status(500).SendString("Failed to create redirect request: " + err.Error())
		}
		// Copiar headers importantes nuevamente
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")
		if rangeHeader := c.Get("Range"); rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}

		// Hacer la nueva petición
		resp, err = client.Do(req)
		if err != nil {
			return c.Status(500).SendString("Failed to connect to redirected stream: " + err.Error())
		}
	}

	defer resp.Body.Close()

	// Verificar si la respuesta es un manifiesto M3U8
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "mpegurl") || strings.Contains(contentType, "m3u") || strings.HasSuffix(strings.ToLower(targetURL), ".m3u8") {
		return handleManifest(c, resp, req.URL.String()) // Pasar la URL final después de redirecciones
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
	_, err = io.CopyBuffer(fiberWriter, bufferedReader, copyBuf)
	if err != nil {
		fmt.Printf("⚠️ Error en transmisión buffered de %s: %v\n", targetURL, err)
		// No devolvemos error HTTP porque la escritura ya pudo haber comenzado
		return nil
	}
	// fmt.Printf("✅ Transmisión buffered finalizada para %s\n", targetURL)

	return nil
}

func handleManifest(c *fiber.Ctx, resp *http.Response, finalManifestURL string) error {
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
