package source

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"tevasalpalo/internal/store"
	"tevasalpalo/internal/transport"
	"time"
)

var filterList = []string{}

type SourceType string

const (
	SourceTxtRaw SourceType = "txtRaw"
	SourceM3U    SourceType = "m3u"
)

type Source struct {
	Name    string
	URL     string
	Type    SourceType
	Proxied bool
	// ProxyFirst intenta descargar vía proxy antes que en directo. Para hosts
	// que el ISP bloquea o estrangula en la salida directa (ipfs/filebase),
	// que el proxy resuelva por su cuenta es más fiable que esperar a la
	// política directa-con-reintentos.
	ProxyFirst bool
	// Resolve opcional: si no es nil, se llama antes de cada fetch para obtener
	// la URL real. Necesario en fuentes cuyo path cambia en cada publicación
	// (el subdirectorio del worker de elcano rota al republicar).
	Resolve func(string) (string, error)

	// Obligatoria fuerza a consultar la fuente en cada refresco, saltándose
	// la pausa adaptativa. Para las que nunca se deben dejar de mirar.
	Obligatoria bool
}

const (
	shickatWeb         = "https://shickat.online/"
	elcanoWeb          = "https://tokyo.elcano-ipfs.workers.dev/k51qzi5uqu5dh5qej4b9wlcr5i6vhc7rcfkekhrxqek5c9lk6gdaiik820fecs/"
	peticiones         = "https://raw.githubusercontent.com/Icastresana/lista1/refs/heads/main/peticiones"
	platinsport        = "https://raw.githubusercontent.com/tutw/platinsport-m3u-updater/refs/heads/main/lista_scraper_acestream_api.m3u"
	platinsportCanales = "https://raw.githubusercontent.com/tutw/platinsport-m3u-updater/refs/heads/main/canales_acestream.m3u"
	// Renombre de la fuente "unificada": el repo a1975morales/ACESTREAM ya no
	// tiene lista_acestream_unificada.m3u (404 desde 2026-10), pero publica estos tres:
	//   - hashes_acestream.m3u: enlaces acestream://<hash>  <- se usa esta
	//   - hashes.m3u:            mismos 126 hashes con ?id= (contenido idéntico)
	//   - canales_acestream.m3u: byte-idéntico a platinsportCanales (md5 igual)
	gitgayHashes = "https://git.gay/a1975morales/ACESTREAM/raw/branch/main/hashes_acestream.m3u"
	// Segundo IPNS de hashes, servido por filebase. La URL que nos pasaron
	// apuntaba a este IPNS vía ipfs.io, pero ese gateway ya es
	// service-worker-only (429 con "switching to a service worker gateway
	// only", y 403 con challenge de Cloudflare si se finge un navegador), igual
	// que dweb.link y w3s.link. Filebase es el único de los probados que
	// resuelve este IPNS por HTTP plano: 200 y 13445 bytes. Aporta 58 hashes de
	// los que 25 no salen de ninguna otra fuente.
	filebaseHashesIPNS = "https://ipfs.filebase.io/ipns/k51qzi5uqu5di462t7j4vu4akwfhvtjhy88qbupktvoacqfqe9uforjvhyi4wr"
	filebaseHashes     = filebaseHashesIPNS + "/hashes_acestream.m3u"
	// Gateway IPFS que SIGUE hablando HTTP plano. Las gateways grandes
	// (dweb.link, w3s.link, ipfs.io, inbrowser.link) son ya service-worker-only
	// y devuelven 403 o un bootstrap de 11684 bytes. Filebase resuelve el IPNS
	// completo y devuelve el fichero. Aporta 93 hashes que ninguna otra fuente da.
	ipfsGateway = "https://ipfs.filebase.io/ipns/k2k4r8lm8tkmuxbc8lkmq1in3v0oya1p6pe9o5bu0hu30br5ko08k2gb"
	listaplana  = ipfsGateway + "/data/listas/listaplana.txt"
	// lista_fuera_iptv.m3u (mismo repo) tiene EXACTAMENTE los mismos 400 hashes
	// y los mismos nombres: no añadirlo, es un fetch de 83 KB sin ganancia.
	// Si se cayera el IPNS, el mismo contenido está fijado en el CID bloque
	// bafybeihgs5jmggjt7i6vul2sg37lkfs66ko5bmpod7o54zhimywcohttdm.
	// elcano publica tras un IPNS cuyo <base href> apunta a un subdirectorio
	// con CID que ROTA en cada republicación. Hay que resolverlo en cada fetch:
	// con el subdirectorio viejo el worker responde 403 "Forbidden", y sin
	// subdirectorio también 403. Solo la raíz del IPNS responde.
	tokyoElcanoIndex = "https://tokyo.elcano-ipfs.workers.dev/k51qzi5uqu5dh5qej4b9wlcr5i6vhc7rcfkekhrxqek5c9lk6gdaiik820fecs/"
)

var sources = []Source{
	{Name: "listaplana", URL: listaplana, Type: SourceTxtRaw, Proxied: true, ProxyFirst: true},
	{Name: "peticiones", URL: peticiones, Type: SourceM3U, Proxied: false},
	{Name: "platinsport", URL: platinsport, Type: SourceM3U, Proxied: false},
	{Name: "platinsport_canales", URL: platinsportCanales, Type: SourceM3U, Proxied: false},
	{Name: "tokyo_elcano", URL: tokyoElcanoIndex, Type: SourceTxtRaw, Proxied: false, Resolve: resolveElcanoFileURL("hashes.txt")},
	// Espejo de git.gay: mismo operador que elcano, pero en otro host. Aporta
	// 6 hashes que ninguna otra fuente da y cubre una caída de raw.githubusercontent.
	{Name: "gitgay_hashes", URL: gitgayHashes, Type: SourceM3U, Proxied: false},
	// Otro IPNS de hashes por filebase (ver filebaseHashes). 58 hashes, 25
	// nuevos respecto a las fuentes m3u de arriba.
	{Name: "ipfs_hashes", URL: filebaseHashes, Type: SourceM3U, Proxied: true, ProxyFirst: true},
	// Fuentes que NO deben volver:
	//
	// - fuera_iptv: mismo repo y mismo contenido que listaplana (400 hashes
	//   idénticos, mismos nombres). Duplicaría el fetch sin ganancia.
	// - unificada (git.gay/…/lista_acestream_unificada.m3u): 404, el fichero
	//   se borró del repo. La sustituye gitgay_hashes.
	// - tokyo_hashes (git.gay/TokyoGhoulles/…): 404. La sustituye tokyo_elcano.
	// - hashes.m3u: mismos 126 hashes que hashes_acestream.m3u.
	// - canales_acestream.m3u: byte-idéntico a platinsportCanales.
	// - cualquier URL de gateway IPFS que no sea ipfs.filebase.io: dweb.link,
	//   w3s.link, ipfs.io e inbrowser.link ya no sirven contenido por HTTP
	//   (429 service-worker-only, o 403 con challenge de Cloudflare). Probado
	//   también el IPNS k51qzi5uqu5di462: solo filebase lo resuelve.
}

var reElcanoBaseHref = regexp.MustCompile(`(?i)<base\s[^>]*href\s*=\s*["']([^"']+)["']`)

// fetchParaTest permite sustituir el transporte en los tests del resolver.
// En producción es FetchWebDataTimeout directamente.
var fetchParaTest = transport.FetchWebDataTimeout

// resolveElcanoFileURL devuelve un resolver que traduce la raíz del IPNS de
// elcano en la URL real de un fichero publicado. El index es HTML con
// <base href=".../<cid-actual>/"> y los enlaces son relativos a ese base, así
// que hay que leer el index primero: el subdirectorio cambia en cada
// publicación y fijarlo en el código lo deja muerto (403) en la siguiente.
func resolveElcanoFileURL(file string) func(string) (string, error) {
	return func(index string) (string, error) {
		// El índice se lee por el mismo transporte que después bajará el
		// fichero. Antes era siempre en directo, así que cuando el ISP corta
		// la conexión directa (connection reset contra este worker, que pasa
		// con frecuencia) el resolver fallaba siempre, aunque la descarga
		// principal fuera a salir por proxy. Resultado: la fuente se quedaba
		// sin resolver y el fallback acababa bajando el índice en HTML.
		body, err := fetchParaTest(index, false, transport.TimeTimeout)
		if err != nil || len(body) == 0 {
			// Reserva por proxy: es la misma salida que usará la descarga.
			var errDirecto error
			if err != nil {
				errDirecto = err
			} else {
				errDirecto = errors.New("el índice vino vacío")
			}
			proxy, errProxy := fetchParaTest(index, true, transport.TimeTimeout)
			if errProxy != nil || len(proxy) == 0 {
				if errProxy != nil {
					return "", fmt.Errorf("no se pudo leer el índice %s (directo: %v; proxy: %v)",
						index, errDirecto, errProxy)
				}
				return "", fmt.Errorf("no se pudo leer el índice %s (directo: %v; proxy: índice vacío)",
					index, errDirecto)
			}
			body = proxy
		}
		m := reElcanoBaseHref.FindSubmatch(body)
		if m == nil {
			return "", fmt.Errorf("no se encontró <base href> en el índice de %s", index)
		}
		base := strings.TrimSpace(string(m[1]))
		if base == "" {
			return "", fmt.Errorf("<base href> vacío en el índice de %s", index)
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		log.Printf("🔗 elcano: subdirectorio actual %s", base)
		return base + file, nil
	}
}

// fetchResult es el desenlace de descargar una fuente: cuerpo, error y si el
// fallo fue un 4xx hopeless (no reintentable ni proxiable).
type fetchResult struct {
	body     []byte
	err      error
	attempts int
	fatal4xx bool
}

func (r fetchResult) success() bool { return r.err == nil && len(r.body) != 0 }

// resolveAndFetch resuelve la URL efectiva de la fuente (si tiene resolver) y
// descarga el cuerpo en un solo intento. El resolver se llama en cada intento
// porque el path puede rotar entre reintentos.
func resolveAndFetch(s Source) ([]byte, error) {
	fetchURL := s.URL
	if s.Resolve != nil {
		resolved, err := s.Resolve(s.URL)
		if err != nil {
			// Se propaga como error normal (no como fatal) para que la política
			// de reintentos sea la misma que en el fetch.
			return nil, fmt.Errorf("no se pudo resolver la URL: %w", err)
		}
		fetchURL = resolved
	}
	return fetchWebData(fetchURL, s.Proxied)
}

// fetchWebData permite a los tests sustituir el transporte para verificar la
// política de reintentos sin tocar la red. En producción siempre vale
// FetchWebData.
var fetchWebData = transport.FetchWebData

// fetchSourceBody descarga una fuente con reintentos y, como último recurso, vía
// proxy. La política de reintento NO depende de si el fallo vino del resolver o
// del fetch: un 429 del worker de elcano es tan reintentable como un 429 al
// descargar la lista, y un error de red lo arregla cambiar de transporte. Antes
// el resolver marcaba fatal4xx siempre, así que un 429 en el índice de elcano
// mataba la fuente en el intento 1 sin reintentar ni probar el proxy.
func fetchSourceBody(s Source, isDev bool) fetchResult {
	// Fuentes marcadas como ProxyFirst (ipfs/filebase)': el ISP bloquea o
	// ralentiza la IP directa contra estos hosts (timeouts de client),
	// así que se intenta por proxy (xray/Tor) primero y, si tras varios
	// intentos no sale, se cae a directo como reserva.
	if s.ProxyFirst {
		return fetchSourceBodyProxyFirst(s, isDev)
	}
	return fetchSourceBodyConPolitica(s, isDev, maxFetchAttempts, defaultFetchBackoff)
}

// fetchSourceBodyProxyFirst invierte el orden respecto a
// fetchSourceBodyConPolitica: primero descarga vía proxy, y solo si la fuente
// se queda sin respuesta pasa a intentarlo en directo.
func fetchSourceBodyProxyFirst(s Source, isDev bool) fetchResult {
	var res fetchResult
	fetchURL := s.URL

	// Fase 1: proxy (máx 3 intentos, igual que el fallback directo->proxy
	// que ya existía pero en orden contrario).
	for pAttempt := 1; pAttempt <= 3; pAttempt++ {
		res.attempts = pAttempt
		if isDev {
			log.Printf("📡 [%s] proxy intento %d/3 %s (proxied=true)", s.Name, pAttempt, fetchURL)
		}
		body, err := fetchWebData(fetchURL, true)
		if err == nil && len(body) != 0 {
			log.Printf("✅ [%s] ok vía proxy (intento %d/3)", s.Name, pAttempt)
			return fetchResult{body: body, attempts: res.attempts}
		}
		res.err = err
		if transport.IsFatalFetchErr(err) {
			res.fatal4xx = true
			break
		}
		if isDev {
			log.Printf("⚠️  [%s] proxy intento %d/3 fallo: %v", s.Name, pAttempt, err)
		}
		if pAttempt < 3 {
			time.Sleep(time.Duration(pAttempt) * 3 * time.Second)
		}
	}

	// Fase 2: reserva directa (una pasada corta con la política estándar).
	log.Printf("🔄 [%s] proxy agotado, último recurso directo...", s.Name)
	direct := s
	direct.Proxied = false
	return fetchSourceBodyPolitica(direct, isDev, transport.MaxNetworkRetries, defaultFetchBackoff, false)
}

// maxFetchAttempts: intentos contra la misma salida antes de rendirse.
const maxFetchAttempts = 10

// fetchBackoff: espera tras un intento fallido. Inyectable para no dormir en
// los tests.
type fetchBackoff func(attempt int) time.Duration

func defaultFetchBackoff(attempt int) time.Duration {
	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	if backoff > 4*time.Second {
		backoff = 4 * time.Second
	}
	return backoff
}

func fetchSourceBodyConPolitica(s Source, isDev bool, maxAttempts int, backoff fetchBackoff) fetchResult {
	return fetchSourceBodyPolitica(s, isDev, maxAttempts, backoff, true)
}

// fetchSourceBodyPolitica es fetchSourceBodyConPolitica con la opción de no
// volver a caer al proxy al final. La fase de reserva directa de una fuente
// ProxyFirst la usa con permitirFallbackProxy=false: el proxy ya se agotó antes
// y reintentar por ahí duplicaba el peor caso hasta ~70s por fuente.
func fetchSourceBodyPolitica(s Source, isDev bool, maxAttempts int, backoff fetchBackoff, permitirFallbackProxy bool) fetchResult {
	var res fetchResult
	netErrs := 0
	fetchURL := s.URL

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res.attempts = attempt

		if isDev {
			log.Printf("📡 [%s] intento %d/%d %s (proxied=%v)", s.Name, attempt, maxAttempts, fetchURL, s.Proxied)
		} else if attempt == 1 {
			log.Printf("📡 Obteniendo [%s] %s", s.Name, fetchURL)
		}

		body, err := resolveAndFetch(s)
		if err == nil && len(body) != 0 {
			return fetchResult{body: body, attempts: attempt}
		}
		res.err = err

		// Fallo de DNS/conexión: reintentar contra la MISMA salida no lo
		// arregla (el resolver local sigue roto). Tras unos pocos intentos se
		// deja pasar al fallback por proxy, que resuelve el DNS por su cuenta
		// (no se marca fatal: el proxy sí puede resolverlo).
		if transport.IsNetworkErr(err) {
			netErrs++
			if netErrs >= transport.MaxNetworkRetries {
				if isDev {
					log.Printf("🔄 [%s] %d fallos de red seguidos, paso al proxy: %v", s.Name, netErrs, err)
				}
				break
			}
		} else {
			netErrs = 0
		}

		// 429 (rate limit) SÍ reintenta con backoff; el resto de 4xx no.
		if transport.IsFatalFetchErr(err) {
			res.fatal4xx = true
			if isDev {
				log.Printf("❌ [%s] 4xx no reintenta: %v", s.Name, err)
			} else {
				log.Printf("❌ [%s] error 4xx: %v", s.Name, err)
			}
			break
		}

		if isDev {
			switch {
			case err != nil:
				log.Printf("⚠️  [%s] intento %d fallo (reintentable): %v", s.Name, attempt, err)
			default:
				log.Printf("⚠️  [%s] intento %d body vacío", s.Name, attempt)
			}
		}

		if attempt < maxAttempts {
			time.Sleep(backoff(attempt))
		}
	}

	if permitirFallbackProxy && shouldProxyFallback(s, res.success(), res.fatal4xx) {
		// Último recurso: la IP directa puede estar limitada (429
		// persistente); otra salida vía proxy (Tor/xray) lo salva.
		log.Printf("🔄 [%s] directa agotada, último recurso vía proxy...", s.Name)
		for pAttempt := 1; pAttempt <= 3; pAttempt++ {
			// Si la fuente tiene resolver, hay que resolverla ANTES de
			// descargar. Antes, si el resolver fallaba se dejaba fetchURL en
			// s.URL y se bajaba el índice: para elcano eso es una página HTML,
			// que devuelve 200 y se aceptaba como si fuera la lista. El
			// resultado eran 16 entradas basura y cientos de líneas de aviso.
			// Aquí, si no se resuelve, se pasa al siguiente intento en vez de
			// descargar algo que no es la lista.
			if s.Resolve != nil {
				resolved, rErr := s.Resolve(s.URL)
				if rErr != nil {
					res.err = rErr
					if isDev {
						log.Printf("⚠️  [%s] proxy intento %d/3 sin URL resuelta: %v", s.Name, pAttempt, rErr)
					}
					if pAttempt < 3 {
						time.Sleep(time.Duration(pAttempt) * 5 * time.Second)
					}
					continue
				}
				fetchURL = resolved
			}
			body, err := fetchWebData(fetchURL, true)
			if err == nil && len(body) != 0 {
				log.Printf("✅ [%s] recuperada vía proxy (intento %d/3)", s.Name, pAttempt)
				return fetchResult{body: body, attempts: res.attempts}
			}
			res.err = err
			if isDev {
				log.Printf("⚠️  [%s] proxy intento %d/3 fallo: %v", s.Name, pAttempt, err)
			}
			time.Sleep(time.Duration(pAttempt) * 5 * time.Second)
		}
	}

	return res
}

func FetchUpdatedList() error {
	ensureNormGateway()
	isDev := os.Getenv("ENV") == "dev"
	var mu sync.Mutex
	var wg sync.WaitGroup
	fetchedCount := 0
	var firstErr error
	var firstErrMu sync.Mutex

	log.Print("📡 Obteniendo listado de Canales TV (multi-fuente)")

	// Lo que aporta cada fuente por separado. Se usa para el registro de
	// fuentes de la base de datos y para medir el solapamiento entre ellas.
	porFuente := map[string]map[string][]string{}
	fusionadoBruto := map[string][]string{}
	estadoFuente := map[string]string{}
	intentosFuente := map[string]int{}
	duracionFuente := map[string]int64{}

	// Qué fuentes hay en pausa y hasta cuándo. Una fuente dormida no se
	// descarga, pero su estado anterior se conserva para que el cálculo de
	// exclusividad siga teniendo en cuenta lo que aportó la última vez.
	ahora := time.Now()
	enPausa := map[string]store.FuenteRegistro{}
	if store.Global != nil {
		for _, f := range store.Global.ListarFuentes() {
			if !f.DebeConsultarse(ahora) {
				enPausa[f.Nombre] = f
			}
		}
	}
	for nombre, f := range enPausa {
		log.Printf("💤 [%s] en pausa: no aporta ningún canal exclusivo desde hace %d ciclos. Siguiente consulta en %v",
			nombre, f.CiclosSinAportar, f.ProximaConsulta.Format("15:04"))
	}

	pausadas := map[string]bool{}
	// El refresco se construye sobre un mapa privado y se publica al final
	// con una sola asignación (invariante copy-on-write): los lectores
	// concurrentes de la web nunca ven un mapa a medio escribir.
	vivo := SnapshotBroadcasters()

	for _, src := range sources {
		if _, dormida := enPausa[src.Name]; dormida && !src.Obligatoria {
			// No se descarga, pero se comunica a la base para que su cadencia
			// no se recalcule contra un cero que no es real: si no, cada
			// refresco en pausa lo hundiría un nivel más y nunca podría
			// volver a entrar por su cuenta.
			estadoFuente[src.Name] = ""
			porFuente[src.Name] = map[string][]string{}
			pausadas[src.Name] = true
			continue
		}
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			inicio := time.Now()
			res := fetchSourceBody(s, isDev)
			duracionFuente[s.Name] = time.Since(inicio).Milliseconds()
			intentosFuente[s.Name] = res.attempts

			if !res.success() {
				estadoFuente[s.Name] = "sin respuesta"
				log.Printf("❌ [%s] no se pudo obtener tras %d intentos", s.Name, res.attempts)
				if res.err != nil {
					estadoFuente[s.Name] = res.err.Error()
					firstErrMu.Lock()
					if firstErr == nil {
						firstErr = res.err
					}
					firstErrMu.Unlock()
				}
				return
			}
			body := res.body
			var extracted map[string][]string
			switch s.Type {
			case SourceTxtRaw:
				extracted = extractDataFromWebTxtRaw(body)
				if isDev {
					log.Printf("🔧 [%s] txtRaw parseadas %d entradas únicas", s.Name, len(extracted))
				}
			case SourceM3U:
				extracted = extractDataFromM3U_Manual(body, filterList)
				if isDev {
					totalExtInf := strings.Count(string(body), "#EXTINF")
					log.Printf("🔧 [%s] m3u parseadas %d entradas únicas de %d EXTINF (filterList %d)", s.Name, len(extracted), totalExtInf, len(filterList))
				}
			default:
				log.Printf("❌ [%s] tipo desconocido %q", s.Name, s.Type)
				return
			}
			mu.Lock()
			// Lo que se guarda en la base es lo fusionado ANTES del gateway
			// estricto. Ese filtro es para decidir qué se ofrece en la web, y
			// se come casi todo: con las fuentes reales, 1651 entradas de
			// platinsport se quedan en 121 tras filtrar. Para una base de
			// datos de AceStream interesa el listado completo, no el curado
			// para la interfaz.
			fusionadoBruto = fusionarListados(fusionadoBruto, extracted)
			vivo = updateBroadcasterMapWithGatewayTolerant(vivo, extracted, s.Name)
			porFuente[s.Name] = extracted
			fetchedCount++
			mu.Unlock()
		}(src)
	}
	wg.Wait()

	// Transformar una sola vez al final
	vivo = transformUriSafeBroadcasters(vivo)

	// Poner el hash más probable primero en cada canal, para que el player no
	// se coma un timeout probando uno que ya sabemos que está muerto.
	ordenarListadoEnVivo(vivo)

	// Publicar: una asignación, mapa nuevo; los lectores con vistas viejas
	// siguen con el mapa anterior sin ver escrituras a medias.
	publicarBroadcaster(vivo)
	log.Printf("✅ Fuentes procesadas: %d/%d", fetchedCount, len(sources))

	// Guardar el resultado en la base de datos: sirve para el arranque
	// siguiente, para el historial de altas y bajas, y para saber qué fuente
	// aporta qué. Es best effort: si falla, la app sigue igual.
	if store.Global != nil {
		// Se guarda el listado fusionado completo, no el que ha pasado el
		// gateway estricto. Ese filtro existe para decidir qué se ofrece en la
		// web y se come casi todo: con las fuentes reales, 1651 entradas de
		// platinsport se quedan en 121. Para una base de datos de AceStream
		// interesa el listado completo, no el curado para la interfaz.
		paraGuardar := fusionadoBruto
		altas, bajas, err := store.Global.GuardarRefresco(
			paraGuardar, porFuente, estadoFuente, intentosFuente, duracionFuente, pausadas)
		switch {
		case err != nil:
			log.Printf("⚠️  base de datos: no se pudo guardar (%v)", err)
		case altas > 0 || bajas > 0:
			log.Printf("💾 Guardados %d canales: %d nuevos, %d desaparecidos",
				len(paraGuardar), altas, bajas)
		default:
			log.Printf("💾 Guardados %d canales (sin cambios)", len(paraGuardar))
		}
	}
	if fetchedCount == 0 && firstErr != nil {
		// si todas fallaron, retornar error
		return firstErr
	}

	log.Print("Filtrando canales TV....")
	// transform uri links to base64 uri safe

	publicarTop(transformCompetitionsToTop(AllCompetitionsData))

	// El callback lo fija la capa web (server.go): source no puede importar
	// a la web sin ciclo, así que tras cada refresco avisa "listado listo".
	if OnListaRefrescada != nil {
		if err := OnListaRefrescada(); err != nil {
			log.Printf("⚠️  Advertencia: No se pudieron pre-cargar datos: %v", err)
		}
	}
	return nil
}

// OnListaRefrescada la fija la web (preloadProgramationTVData + goroutine de
// refresco programado): se ejecuta tras cada FetchUpdatedList con éxito.
var OnListaRefrescada func() error

// shouldProxyFallback decide el último recurso para fuentes directas
// agotadas sin error fatal (p. ej. 429 persistente = IP limitada).
func shouldProxyFallback(s Source, success, fatal bool) bool {
	return !success && !fatal && !s.Proxied
}

// IniciarRefrescoCiclico lanza el refresco de las fuentes inmediatamente y
// luego cada `cada`. Los reintentos se solapan nunca (si un ciclo sigue en
// marcha, el siguiente tick se salta): dos FetchUpdatedList a la vez
// descargarían las mismas fuentes doble.
//
// La cadencia por fuente (pauses por exclusividad, ver store.DebeConsultarse)
// funciona dentro de cada ciclo; este intervalo solo manda cuántas veces al
// día se vuelve a preguntar. 6h alinea el listado con el ticker de datos de
// programación de la web.
var refrescoEnMarcha atomic.Bool

func IniciarRefrescoCiclico(cada time.Duration) {
	go func() {
		refresco := time.NewTicker(cada)
		defer refresco.Stop()
		for {
			if refrescoEnMarcha.CompareAndSwap(false, true) {
				if err := FetchUpdatedList(); err != nil {
					log.Printf("⚠️ refresco de fuentes fallido (se reintenta en %v): %v", cada, err)
				}
				refrescoEnMarcha.Store(false)
			}
			<-refresco.C
		}
	}()
}
