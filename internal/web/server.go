package web

import (
	"embed"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"log"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
	htmlgo "github.com/gofiber/template/html/v3"
	"net/http"
	"net/url"

	"tevasalpalo/internal/ace"
	"tevasalpalo/internal/source"
	"tevasalpalo/internal/store"
	"tevasalpalo/internal/telemetry"
	"tevasalpalo/internal/transport"
	"tevasalpalo/update"
)

//go:embed views
var viewsFS embed.FS

func StartWebServer() (*fiber.App, error) {
	// El ajuste guardado en settings.json alimenta el flag de WARP del
	// runtime: warpEnabled ya no lee el fichero por su cuenta (ver warp.go).
	transport.SetWarpAuto(loadSettings().WarpEnabled)

	// El refresco de fuentes avista a la web cuando hay listado nuevo
	// (source no puede importar server: ciclo).
	source.OnListaRefrescada = func() error {
		if err := preloadProgramationTVData(); err != nil {
			return err
		}
		go startTVProgramationDataRefresh()
		return nil
	}

	sub, err := fs.Sub(viewsFS, "views")
	if err != nil {
		log.Fatalf("failed to create sub FS: %v", err)
	}
	if sub == nil {
		log.Fatal("sub FS is nil")
	}
	engine := htmlgo.NewFileSystem(http.FS(sub), ".html")
	engine.AddFunc("b64", encodeContent)
	engine.AddFunc("add", func(a, b int) int { return a + b })
	// "join" y "urlencode" para pasar la lista de enlaces del canal al player
	// (views/broadcasters.html): el player pinta un botón por enlace.
	engine.AddFunc("join", func(sep string, items []string) string {
		return strings.Join(items, sep)
	})
	engine.AddFunc("urlencode", func(s string) string {
		return url.QueryEscape(s)
	})
	if err := engine.Load(); err != nil {
		log.Fatalf("failed to load views: %v", err)
	}

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	// app.Use(cors.New(cors.Config{
	// 	AllowOrigins: "http://localhost:3000",
	// 	AllowMethods: "GET,POST,PUT,DELETE",
	// 	AllowHeaders: "Origin, Content-Type, Accept",
	// }))

	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Range"},
		AllowCredentials: false,
	}))

	app.Get("/ace/*", func(c fiber.Ctx) error {
		// Obtener la ruta completa después de /ace/
		acePath := c.Params("*")

		// Motor AceStream: local por defecto, remoto vía ACESTREAM_API.
		// En remoto el tráfico va por la cadena proxy (xray/Tor); en local,
		// directo como siempre (ver aceproxy.go).
		targetURL := ace.AcestreamAPI() + "/ace/" + acePath

		// 🔥 SOLO añadir query string si realmente existe y no está vacío
		queryString := string(c.Request().URI().QueryString())
		if queryString != "" {
			targetURL += "?" + queryString
		}

		// WARP on-demand: /ace/* con datos de stream es justo el caso que
		// activa el túnel en el cliente Android (http_stream_start). Los
		// manifests y comandos del webui no lo activan.
		// Anotación del resultado del hash. Se usa ace.HashDeRutaAce y no solo
		// ace.HashDePeticionAce porque el camino que el player recorre de verdad
		// (manifest.m3u8 -> /ace/m/<hash>/... -> /ace/c/<hash>/N.ts) no pasa
		// por getstream ni por /h/, así que antes no se anotaba nada y el
		// bucket hashes se quedaba vacío. Los segmentos son además la señal
		// más fiable: si llegaron, el stream estaba vivo.
		//
		// La ruta final solo se conoce después de la petición, así que aquí
		// se resuelve con lo que hay y se completa más abajo.
		hashProbado := ace.HashDeRutaAce(acePath, queryString, false, "")

		isAceStream := transport.IsAceStreamPath(acePath)
		transport.TraceLog("/ace/* ruta=%q -> isAceStream=%v (WARP=%v)", acePath, isAceStream, transport.WarpEnabled())
		if isAceStream {
			transport.WarpStreamStart()
		}

		// También es una línea por segmento. Con traza sale el detalle; sin
		// ella, solo el resumen que ya escribe handleAceManifest.
		transport.TraceLog("🔄 Proxy ACE: %s -> %s", c.Path(), targetURL)

		// Crear la petición
		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			log.Printf("❌ Error creando petición proxy: %v", err)
			return c.Status(500).SendString("Error en proxy: " + err.Error())
		}

		// Copiar headers importantes
		if userAgent := c.Get("User-Agent"); userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}
		if acceptLang := c.Get("Accept-Language"); acceptLang != "" {
			req.Header.Set("Accept-Language", acceptLang)
		}
		if rangeHeader := c.Get("Range"); rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}

		// Hacer la petición (directa a loopback; a remoto, cadena proxy
		// solo con el interruptor proxy-media ON)
		//
		// Los manifests (incluidos los anidados /ace/m/*.m3u8) se retienen
		// esperando pares P2P, así que van con timeout largo; los segmentos,
		// con uno corto. Se traza el timeout efectivo porque si un manifest
		// muere a los ~30s el síntoma es idéntico al de un binario viejo
		// compilado antes de que existiera el timeout largo.
		aceTimeout := ace.AceTimeoutFor(acePath)
		transport.TraceLog("/ace/* ruta=%q timeout=%v", acePath, aceTimeout)
		inicio := time.Now()
		// Los redirects los sigue ace.AceDoTimeout con timeout propio por salto,
		// así que aquí hay que poder reconstruir la petición para una URL que
		// no es la original. El primer salto reutiliza req ya construida (y
		// ya validada) para no perder el 500 con "Error creando petición".
		resp, via, finalURL, err := ace.AceDoTimeout(targetURL, ace.ProxyMediaEnabled(), aceTimeout, func(u string, client *http.Client) (*http.Response, error) {
			if u == targetURL {
				return client.Do(req)
			}
			saltado, err := http.NewRequest("GET", u, nil)
			if err != nil {
				return nil, err
			}
			for key, values := range req.Header {
				for _, value := range values {
					saltado.Header.Add(key, value)
				}
			}
			return client.Do(saltado)
		})
		// Un manifest principal que redirige a /ace/m/... sigue siendo, a
		// efectos del player, un manifest anidado: si expira hay que
		// responder 204, no 502, aunque la ruta de entrada no lo parezca.
		rutaFinal := ace.AcePathFromURL(finalURL)
		esAnidado := ace.IsAceNestedManifest(acePath) || ace.IsAceNestedManifest(rutaFinal)
		// Con la ruta final ya se puede completar el hash: el manifest
		// principal llega sin hash en la ruta y lo trae en la que redirige.
		if hashProbado == "" {
			hashProbado = ace.HashDeRutaAce(acePath, queryString, true, rutaFinal)
		}
		esSegmento := strings.HasSuffix(acePath, ".ts")
		if err != nil {
			if isAceStream {
				transport.WarpStreamStop()
			}
			// Un manifest anidado /ace/m/*.m3u8 que no responde no es un
			// fallo de reproducción: el engine lo retiene indefinidamente
			// aunque los .ts sí llegan. Devolver 502 aquí lo convierte en
			// error fatal de red en HLS.js y tumba el stream entero, así que
			// se responde 204 ("sin actualización") para que mantenga el
			// último playlist bueno.
			//
			// Aquí NO se anota nada contra el hash: que el engine retenga un
			// manifest no dice si el stream está muerto, solo que todavía no ha
			// llegado. Anotarlo era el error que dejó esta capa muerta.
			if esAnidado {
				esperado := time.Since(inicio).Round(time.Second)
				// No es traceLog: el 204 es el resultado esperado cada vez que
				// el engine retiene un manifest anidado, pero si falla el
				// timeout (por ejemplo si colara por la ruta del principal)
				// el tiempo delataría el problema. Por eso el aviso va siempre.
				log.Printf("↩️  Manifest anidado retenido por el engine (%v), respondo 204 para no tumbar el player: %q -> %q",
					esperado, acePath, rutaFinal)
				c.Set("Cache-Control", "no-store")
				return c.SendStatus(fiber.StatusNoContent)
			}
			// Un segmento que el engine no puede servir sí es evidencia en
			// contra: ahí no hay ambigüedad, el medio no llegó.
			if hashProbado != "" && esSegmento {
				store.Global.RegistrarEvidencia(hashProbado, store.EvSegmentoErr)
			}
			// La ruta y el timeout van en el mensaje a propósito. Con solo
			// "(2m0s)" no se distingue un manifest principal de uno
			// anidado, y ambos dan 120s si la clasificación falla: el mismo
			// síntoma para dos causas distintas, imposible de diagnosticar
			// desde fuera.
			log.Printf("❌ Error en petición a Ace Stream ruta=%q (final=%q) timeout=%v tras %v: %v",
				acePath, rutaFinal, aceTimeout, time.Since(inicio).Round(time.Second), err)
			return c.Status(502).SendString("Error conectando con Ace Stream: " + err.Error())
		}
		transport.TraceLog("/ace/* ruta=%q respondida en %v vía %s", acePath, time.Since(inicio).Round(time.Millisecond), via)
		defer func() {
			resp.Body.Close()
			if isAceStream {
				transport.WarpStreamStop()
			}
		}()
		if via != "direct" {
			log.Printf("🔄 ACE %s vía %s", c.Path(), via)
		}

		// Evidencia a favor. Un segmento que llegó es la señal más fuerte que
		// tenemos de que el enlace está vivo, y no depende de ninguna API
		// externa. Los manifests los anota handleAceManifest, que sí sabe si
		// traerían algo que reproducir.
		if hashProbado != "" && esSegmento && resp.StatusCode == 200 {
			store.Global.RegistrarEvidencia(hashProbado, store.EvSegmentoOK)
		}

		// 🔥 SI ES UN MANIFEST M3U8, MODIFICAR LAS URLs
		if strings.HasSuffix(acePath, ".m3u8") {
			return handleAceManifest(c, resp, ace.AcestreamEngineHosts(), hashProbado)
		}

		// Para archivos .ts y otros
		// Copiar headers de respuesta
		for key, values := range resp.Header {
			for _, value := range values {
				c.Set(key, value)
			}
		}

		// 🔥 CORREGIR Content-Type para archivos .ts
		if strings.HasSuffix(acePath, ".ts") {
			c.Set("Content-Type", "video/mp2t")
			// Una por segmento, igual que las anteriores.
			transport.TraceLog("✅ Content-Type corregido para .ts: %s", acePath)
		}

		// Asegurar CORS
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "*")
		c.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

		// Manejar OPTIONS
		if c.Method() == "OPTIONS" {
			return c.SendStatus(200)
		}

		// Copiar status code y body
		c.Status(resp.StatusCode)

		// 🔥 Si Ace Stream devuelve error, loggear y devolver
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			log.Printf("❌ Ace Stream devolvió error %d para %s: %s", resp.StatusCode, targetURL, string(bodyBytes))
			return c.SendString(string(bodyBytes))
		}

		_, err = io.Copy(c.Response().BodyWriter(), resp.Body)
		if err != nil {
			log.Printf("⚠️ Error copiando respuesta: %v", err)
		}

		return nil
	})

	cssFS, err := fs.Sub(viewsFS, "views/css")
	if err != nil {
		log.Fatalf("failed to create css sub FS: %v", err)
	}
	imagesFS, err := fs.Sub(viewsFS, "views/images")
	if err != nil {
		log.Fatalf("failed to create images sub FS: %v", err)
	}
	jsFS, err := fs.Sub(viewsFS, "views/js")
	if err != nil {
		log.Fatalf("failed to create js sub FS: %v", err)
	}

	app.Use("/", static.New("", static.Config{
		FS:         viewsFS,
		Browse:     false,
		IndexNames: []string{"index.html"},
	}))

	app.Use("/css", static.New("", static.Config{
		FS:     cssFS,
		Browse: false,
		// IndexNames: []string{"index.html"},
	}))

	app.Use("/images", static.New("", static.Config{
		FS:     imagesFS,
		Browse: false,
	}))

	app.Use("/player/js", static.New("", static.Config{
		FS:     jsFS,
		Browse: false,
	}))

	app.Use("/player/css", static.New("", static.Config{
		FS:     cssFS,
		Browse: false,
	}))

	app.Use("/js", static.New("", static.Config{
		FS:     jsFS,
		Browse: false,
	}))

	app.Get("/", func(c fiber.Ctx) error {
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()
		return c.Render("index", fiber.Map{
			"Days":                   data.Days,
			"allCompetitions":        data.AllCompetitions,
			"topCompetitions":        data.TopCompetitions,
			"topCompetitionsOrdered": data.TopCompetitionsOrdered,
			"DaysJSON":               data.DaysJSON,
			"topCompetitionsJSON":    data.TopCompetitionsJSON,
		})
	})

	app.Get("/broadcasters", func(c fiber.Ctx) error {
		dataMutex.RLock()
		data := cachedData
		dataMutex.RUnlock()

		return c.Render("broadcasters", fiber.Map{
			//
			"Broadcasters":           data.BroadcastersOrdered,
			"DaysJSON":               data.DaysJSON,
			"Days":                   data.Days,
			"allCompetitions":        data.AllCompetitions,
			"topCompetitions":        data.TopCompetitions,
			"topCompetitionsOrdered": data.TopCompetitionsOrdered,
			"topCompetitionsJSON":    data.TopCompetitionsJSON,
		})
	})

	app.Get("/refresh-data", func(c fiber.Ctx) error {
		if err := preloadProgramationTVData(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Datos actualizados correctamente",
		})
	})

	// Interruptor proxy multimedia (ver aceproxy.go): OFF = streams directos
	// (comportamiento actual); ON = /api/iptv/* y /ace/* remoto por Tor/xray.
	app.Get("/api/proxy-media", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"enabled": ace.ProxyMediaEnabled(),
		})
	})

	app.Post("/api/proxy-media", func(c fiber.Ctx) error {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(c.Body(), &body); err != nil || body.Enabled == nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "JSON {enabled: bool} requerido",
			})
		}
		ace.SetProxyMedia(*body.Enabled)
		log.Printf("🎚️  proxy multimedia: %v", *body.Enabled)
		return c.JSON(fiber.Map{
			"enabled": ace.ProxyMediaEnabled(),
		})
	})

	// Estado de WARP. "active" es si el túnel está realmente levantado ahora,
	// que no es lo mismo que "enabled": WARP es on-demand y solo levanta el
	// túnel cuando entra un stream.
	app.Get("/api/warp", func(c fiber.Ctx) error {
		return c.JSON(transport.WarpStatus())
	})

	// Diagnóstico del bypass DNS/VAST: contadores del proxy local que se pone
	// delante del motor. Si tras ver un canal siguen a cero, el motor no está
	// usando HTTP_PROXY y hay que recurrir al parche de Py_NoSiteFlag.
	// via_warp/via_direct dicen por dónde salió el tráfico del motor.
	app.Get("/api/bypass", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"addr":       ace.BypassEngineAddr,
			"warp_on":    transport.WarpEnabled(),
			"warp_socks": transport.WarpSocksAddr(),
			"stats":      ace.EngineBypassStats.Stats(),
		})
	})

	// Telemetría del navegador. El player no habla nunca con el central
	// directamente: postea aquí y el servidor reenvía, para que las
	// credenciales de Loki no salgan del servidor. La IP se hashea con el
	// salt local antes de enviar nada.
	// El player manda lotes: {"events":[{"event":..,"fields":{..}}]}. Se
	// acepta también el formato de un solo evento para no romper clientes
	// viejos. Límites duros porque esto está abierto a cualquiera de la red
	// local: sin tope, un bucle de reintentos de HLS.js podría meter miles de
	// líneas en la cola central.
	app.Post("/api/telemetry", func(c fiber.Ctx) error {
		if n := len(c.Body()); n > telemetry.TelMaxBody {
			return c.Status(413).JSON(fiber.Map{"error": "cuerpo demasiado grande"})
		}
		var body struct {
			Event  string    `json:"event"`
			Fields fiber.Map `json:"fields"`
			Events []struct {
				Event  string    `json:"event"`
				Fields fiber.Map `json:"fields"`
			} `json:"events"`
		}
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "JSON inválido"})
		}

		ip := c.IP()
		aceptados := 0
		registrar := func(event string, fields fiber.Map) {
			event = strings.TrimSpace(event)
			if event == "" || len(event) > telemetry.TelMaxEventLen {
				return
			}
			f := map[string]any{}
			for k, v := range fields {
				if len(k) > telemetry.TelMaxFieldLen {
					continue
				}
				f[k] = v
			}
			telemetry.RecordPlayer(event, f, ip)
			aceptados++
		}

		if len(body.Events) > 0 {
			for _, ev := range body.Events {
				if aceptados >= telemetry.TelMaxEvents {
					break
				}
				registrar(ev.Event, ev.Fields)
			}
		} else {
			registrar(body.Event, body.Fields)
		}
		if aceptados == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "ningún evento válido"})
		}
		return c.JSON(fiber.Map{"ok": true, "aceptados": aceptados})
	})

	// Últimas líneas del log en memoria, para pegar en un bug report sin
	// tener que buscar el fichero. ?lines=N (por defecto 200, tope 2000).
	//
	// El servidor escucha en 0.0.0.0, así que esto solo se sirve a loopback o
	// con token: el log lleva URLs y hashes literales y no cualquiera de la
	// red debería poder leerlos.
	app.Get("/api/logs", func(c fiber.Ctx) error {
		if !telemetry.TelAutorizado(c) {
			return c.Status(403).JSON(fiber.Map{
				"error": "requiere TELEMETRY_API_TOKEN o conexión desde localhost",
			})
		}
		lines := telemetry.EnvInt("API_LOG_LINES", 200)
		if v := c.Query("lines"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				lines = n
			}
		}
		if lines > 2000 {
			lines = 2000
		}
		out := fiber.Map{
			"config": telemetry.ConfigSnapshot(),
			"lines":  telemetry.TailRecent(lines),
		}
		if st := telemetry.ExporterStats(); st != nil {
			out["exporter"] = st
		}
		return c.JSON(out)
	})

	// Base de datos de AceStream: estado, aporte por fuente, búsqueda e
	// historial. Todo de solo lectura y con las mismas protecciones que
	// /api/logs, porque el historial contiene nombres de canal y hashes.
	app.Get("/api/db", func(c fiber.Ctx) error {
		if !telemetry.TelAutorizado(c) {
			return c.Status(403).JSON(fiber.Map{
				"error": "requiere TELEMETRY_API_TOKEN o conexión desde localhost",
			})
		}
		out := fiber.Map{"estado": store.Global.Resumen()}

		switch c.Query("que") {
		case "fuentes":
			out["fuentes"] = store.Global.ListarFuentes()
		case "buscar":
			out["resultados"] = store.Global.BuscarCaneles(c.Query("q"), 50)
		case "historial":
			n := 50
			if v := c.Query("n"); v != "" {
				if i, err := strconv.Atoi(v); err == nil && i > 0 && i <= 500 {
					n = i
				}
			}
			out["eventos"] = store.Global.EventosRecientes(n)
		case "hashes":
			// Estado de la capa de calidad de los enlaces y por qué un canal
			// pone un hash delante de otro. Con ?canal= se ve el detalle.
			out["hashes"] = store.Global.ResumenHashes()
			out["muertos"] = store.Global.HashesMuertos()
			if q := c.Query("canal"); q != "" {
				out["detalle"] = store.Global.HashesOrdenadosPara(store.Global.HashesDeCanal(q))
			}
		default:
			// Sin ?que, un resumen de las fuentes es lo más útil: dice de
			// dónde viene cada canal y cuál se podría quitar.
			out["fuentes"] = store.Global.ListarFuentes()
		}
		return c.JSON(out)
	})

	app.Post("/api/warp", func(c fiber.Ctx) error {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(c.Body(), &body); err != nil || body.Enabled == nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "JSON {enabled: bool} requerido",
			})
		}
		if err := setWarpEnabled(*body.Enabled); err != nil {
			log.Printf("❌ No se pudo guardar el ajuste de WARP: %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error":   "no se pudo guardar settings.json",
				"details": err.Error(),
			})
		}
		// El flag va al mismo runtime que warpEnabled consulta; sin esto
		// el ajuste solo valdría tras reiniciar.
		transport.SetWarpAuto(*body.Enabled)
		log.Printf("🛡️  WARP: %v (guardado)", *body.Enabled)
		// Al desactivar y no haber stream, el túnel se para ya: si no se
		// quedaría abierto hasta que saltara el temporizador de 60 s.
		if !*body.Enabled {
			if transport.WarpStreamsIdle() {
				if err := transport.StopWarpIfRunning(); err != nil {
					log.Printf("⚠️  Error al cerrar WARP: %v", err)
				}
			}
		}
		return c.JSON(transport.WarpStatus())
	})

	app.Get("/player/index.html", func(c fiber.Ctx) error {
		link := c.Query("link")
		content := c.Query("content")
		pid := c.Query("pid")

		if link == "" {
			return c.Status(400).SendString("Faltan parámetros")
		}
		if pid == "" {
			pid = fmt.Sprintf("%d", time.Now().Unix())
		}

		// Lista de enlaces del mismo canal/recurrencia, para poder saltar de
		// uno a otro sin volver a la portada. Viene separada por comas en
		// "all"; el enlace actual se marca para saber cuál está sonando.
		all := c.Query("all")
		var altLinks []string
		if all != "" {
			altLinks = strings.Split(all, ",")
		}
		currentLink := 0
		for i, l := range altLinks {
			if l == link {
				currentLink = i
				break
			}
		}

		if content == "" {
			btn := c.Query("btn")
			return c.Render("player", fiber.Map{
				"linkid":      link,
				"btntext":     btn,
				"pid":         pid,
				"error":       nil,
				"altlinks":    altLinks,
				"currentlink": currentLink,
			})
		}

		decodedContent, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return c.Status(400).SendString("Error al decodificar el contenido")
		}
		splitted := strings.Split(string(decodedContent), ";")
		btn := c.Query("btn")
		return c.Render("player", fiber.Map{
			"linkid":          link,
			"broadcastername": splitted[0],
			"eventname":       splitted[1],
			"competitionname": splitted[2],
			"btntext":         btn,
			"pid":             pid,
			"error":           nil,
			"altlinks":        altLinks,
			"currentlink":     currentLink,
		})
	})

	app.Get("/update", func(c fiber.Ctx) error {
		go func() {
			err := update.ForceUpdate()
			if err != nil {
				log.Print("ERROR | No es posible actualizarse")
				return
			}
			time.Sleep(3 * time.Second)
			OnShutdownApp()
		}()

		return c.JSON(fiber.Map{
			"sendedupdate": true,
		})
	})

	app.Get("/healthz", func(c fiber.Ctx) error {
		if update.Updated {
			return c.JSON(fiber.Map{
				"ok": true,
			})
		}
		return c.JSON(fiber.Map{
			"ok": false,
		})
	})

	app.Get("/updateavailable", func(c fiber.Ctx) error {
		needUpdate := update.GetNeedUpdate()
		return c.JSON(fiber.Map{
			"needUpdate": needUpdate,
		})
	})

	// Endpoint de fallback para cualquier .ts
	app.Get("/hls/:segment", func(c fiber.Ctx) error {
		segmentName := c.Params("segment")
		log.Print(segmentName)
		return c.SendStatus(200)
	})

	app.Get("/api/iptv/:channel", func(c fiber.Ctx) error {
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
			// if strings.Contains(targetURL, "p;") {
			targetURL = strings.Replace(targetURL, "p;", "", 1)
			// }
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

// OnShutdownApp lo fija el arranque (app.go): con este callback la web pide
// el cierre ordenado del proceso (p. ej. tras una autoupdate que cambia el
// binario). Sin fijarlo, no hay canal y no pasa nada.
var OnShutdownApp func()
