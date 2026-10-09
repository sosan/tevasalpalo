package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grafov/m3u8"
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
	// Resolve opcional: si no es nil, se llama antes de cada fetch para obtener
	// la URL real. Necesario en fuentes cuyo path cambia en cada publicación
	// (el subdirectorio del worker de elcano rota al republicar).
	Resolve func(string) (string, error)
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
	{Name: "listaplana", URL: listaplana, Type: SourceTxtRaw, Proxied: false},
	{Name: "peticiones", URL: peticiones, Type: SourceM3U, Proxied: false},
	{Name: "platinsport", URL: platinsport, Type: SourceM3U, Proxied: false},
	{Name: "platinsport_canales", URL: platinsportCanales, Type: SourceM3U, Proxied: false},
	{Name: "tokyo_elcano", URL: tokyoElcanoIndex, Type: SourceTxtRaw, Proxied: false, Resolve: resolveElcanoFileURL("hashes.txt")},
	// Espejo de git.gay: mismo operador que elcano, pero en otro host. Aporta
	// 6 hashes que ninguna otra fuente da y cubre una caída de raw.githubusercontent.
	{Name: "gitgay_hashes", URL: gitgayHashes, Type: SourceM3U, Proxied: false},
	// Otro IPNS de hashes por filebase (ver filebaseHashes). 58 hashes, 25
	// nuevos respecto a las fuentes m3u de arriba.
	{Name: "ipfs_hashes", URL: filebaseHashes, Type: SourceM3U, Proxied: false},
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

// resolveElcanoFileURL devuelve un resolver que traduce la raíz del IPNS de
// elcano en la URL real de un fichero publicado. El index es HTML con
// <base href=".../<cid-actual>/"> y los enlaces son relativos a ese base, así
// que hay que leer el index primero: el subdirectorio cambia en cada
// publicación y fijarlo en el código lo deja muerto (403) en la siguiente.
func resolveElcanoFileURL(file string) func(string) (string, error) {
	return func(index string) (string, error) {
		body, err := FetchWebDataTimeout(index, false, timeTimeout)
		if err != nil {
			return "", fmt.Errorf("no se pudo leer el índice %s: %w", index, err)
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
	return FetchWebData(fetchURL, s.Proxied)
}

// fetchSourceBody descarga una fuente con reintentos y, como último recurso, vía
// proxy. La política de reintento NO depende de si el fallo vino del resolver o
// del fetch: un 429 del worker de elcano es tan reintentable como un 429 al
// descargar la lista, y un error de red lo arregla cambiar de transporte. Antes
// el resolver marcaba fatal4xx siempre, así que un 429 en el índice de elcano
// mataba la fuente en el intento 1 sin reintentar ni probar el proxy.
func fetchSourceBody(s Source, isDev bool) fetchResult {
	return fetchSourceBodyConPolitica(s, isDev, maxFetchAttempts, defaultFetchBackoff)
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
		if isNetworkErr(err) {
			netErrs++
			if netErrs >= maxNetworkRetries {
				if isDev {
					log.Printf("🔄 [%s] %d fallos de red seguidos, paso al proxy: %v", s.Name, netErrs, err)
				}
				break
			}
		} else {
			netErrs = 0
		}

		// 429 (rate limit) SÍ reintenta con backoff; el resto de 4xx no.
		if isFatalFetchErr(err) {
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

	if shouldProxyFallback(s, res.success(), res.fatal4xx) {
		// Último recurso: la IP directa puede estar limitada (429
		// persistente); otra salida vía proxy (Tor/xray) lo salva.
		log.Printf("🔄 [%s] directa agotada, último recurso vía proxy...", s.Name)
		for pAttempt := 1; pAttempt <= 3; pAttempt++ {
			if s.Resolve != nil {
				if resolved, rErr := s.Resolve(s.URL); rErr == nil {
					fetchURL = resolved
				}
			}
			body, err := FetchWebData(fetchURL, true)
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

	for _, src := range sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			res := fetchSourceBody(s, isDev)

			if !res.success() {
				log.Printf("❌ [%s] no se pudo obtener tras %d intentos", s.Name, res.attempts)
				if res.err != nil {
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
			broadcasterToAcestream = updateBroadcasterMapWithGatewayTolerant(broadcasterToAcestream, extracted, s.Name)
			fetchedCount++
			mu.Unlock()
		}(src)
	}
	wg.Wait()

	// Transformar una sola vez al final
	broadcasterToAcestream = transformUriSafeBroadcasters(broadcasterToAcestream)
	log.Printf("✅ Fuentes procesadas: %d/%d", fetchedCount, len(sources))
	if fetchedCount == 0 && firstErr != nil {
		// si todas fallaron, retornar error
		return firstErr
	}

	log.Print("Filtrando canales TV....")
	// transform uri links to base64 uri safe

	topCompetitions = transformCompetitionsToTop(allCompetitions)

	if err := preloadProgramationTVData(); err != nil {
		log.Printf("⚠️  Advertencia: No se pudieron pre-cargar datos: %v", err)
	}

	startTVProgramationDataRefresh()
	return nil
}

func extractDataFromWebElCano(body []byte) (map[string][]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error al parsear el documento HTML: %w", err)
	}

	// Buscar el bloque <script> que contiene el JSON
	var scriptContent string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		scriptText := s.Text()
		if strings.Contains(scriptText, "const linksData") {
			scriptContent = scriptText
			return
		}
	})

	if scriptContent == "" {
		return nil, fmt.Errorf("no se encontró el bloque <script> con el JSON")
	}

	splitted := strings.Split(scriptContent, "\n        const linksData =")
	faseA := strings.Split(splitted[1], "const linksList = document.getElementById('linksList');")
	faseB := strings.Split(faseA[0], ";")
	faseC := strings.ReplaceAll(faseB[0], "acestream://", "")
	jsonStr := faseC

	// Parsear el JSON extraído
	var linksData struct {
		Links []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
	}
	err = json.Unmarshal([]byte(jsonStr), &linksData)
	if err != nil {
		return nil, fmt.Errorf("error al parsear el JSON: %w", err)
	}

	replacer := strings.NewReplacer(
		"720", "",
		"1080P", "",
		"1080", "",
		"(Fórmula 1)", "",
		" ", "",
	)

	// Construir el mapa resultante
	extractedData := make(map[string][]string)
	for _, link := range linksData.Links {
		if link.URL != "" {
			if strings.Contains(strings.ToUpper(link.Name), "UHD") || strings.Contains(strings.ToUpper(link.Name), "MULTIAUDIO") {
				continue
			}
			name := strings.TrimSpace(replacer.Replace(link.Name))

			// name := link.Name
			// name = strings.ReplaceAll(name, "720", "")
			// name = strings.ReplaceAll(name, "1080P", "")
			// name = strings.ReplaceAll(name, "1080", "")
			// name = strings.ReplaceAll(name, "(Fórmula 1)", "")
			// name = strings.ReplaceAll(name, " ", "")

			if name == "Dedporte2" {
				name = "Deporte2"
			}
			extractedData[name] = append(extractedData[name], link.URL)
		}
	}

	return extractedData, nil
}

func extractDataFromWebShitkat(body []byte) map[string][]string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	extractedData := make(map[string][]string)
	doc.Find(".canal-card").Each(func(i int, card *goquery.Selection) {
		nombre := card.Find(".canal-nombre").Text()
		acestreamLink := strings.TrimSpace(card.Find(".acestream-link").Text()) //.AttrOr("href", "")
		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
	})
	return extractedData
}

// reHashCorrupto: un token que parece un hash de AceStream pero no lo es:
// 40 caracteres de los cuales 39 son hex y el último es cualquier letra o
// dígito. Es lo que publica listaplana cuando un hash se trunca o se le
// pega un carácter ("d4ff041287a43e3114d411d671c4b4e92e21f33y"). Se distingue
// de un nombre de canal porque no contiene espacios ni más de 42 caracteres.
var reHashCorrupto = regexp.MustCompile(`^[a-f0-9]{30,41}[^a-f0-9\s]$|^[a-f0-9]{42,}$`)

// esHashCorrupto dice si la línea es un hash mal formado y no un nombre de
// canal ni un enlace válido.
func esHashCorrupto(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	// Si extractHashFromLink lo reconoce como enlace, no es basura.
	if extractHashFromLink(t) != "" || strings.Contains(t, "://") || strings.HasPrefix(t, "p;") {
		return false
	}
	return reHashCorrupto.MatchString(t)
}

// normalizarNombreACB renombra los eventos ACB a los nombres de DAZN que usan
// el resto del mapa (vars.go).
func normalizarNombreACB(nombre string) string {
	switch nombre {
	case "ACB EVENTO 01":
		return "DAZN BALONCESTO 1"
	case "ACB EVENTO 02":
		return "DAZN BALONCESTO 2"
	case "ACB EVENTO 03":
		return "DAZN BALONCESTO 3"
	}
	return nombre
}

func extractDataFromWebTxtRaw(body []byte) map[string][]string {
	extractedData := make(map[string][]string)
	rawLines := strings.Split(string(body), "\n")
	// Una pasada de limpieza: cabeceras/ruido de tokyo_elcano y similares (el
	// fichero mezcla español e inglés y algunas cabeceras no empiezan por
	// "===", así que se compara en minúsculas y por prefijo) más los hashes
	// corruptos que el publicador dejó sueltos.
	//
	// Los hashes corruptos ("d4ff...33y": 39 hex + una letra) se descartan
	// ANTES del emparejado. Si no, la resincronización los toma como nombre y
	// el nombre real de al lado como enlace, y el log emite dos avisos que
	// parecen una pérdida ("M+ LALIGA" -> hash, hash -> "M+ LALIGA FHD") cuando
	// el par bueno se recupera igual. El aviso que queda es el que importa: el
	// canal al que le faltaba su hash.
	var lines []string
	for _, l := range rawLines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		if strings.HasPrefix(t, "===") ||
			strings.HasPrefix(lower, "identificadores") ||
			strings.HasPrefix(lower, "acestream ids") ||
			strings.HasPrefix(lower, "generated:") ||
			strings.HasPrefix(lower, "generado:") ||
			strings.HasPrefix(lower, "total:") {
			continue
		}
		if esHashCorrupto(t) {
			if os.Getenv("ENV") == "dev" {
				log.Printf("⚠️  txtRaw hash corrupto descartado: %q", t)
			}
			continue
		}
		lines = append(lines, l)
	}

	// Emparejado nombre/enlace con resincronización: si la línea de enlace no
	// aporta nada usable se avanza de uno en uno, para que un desajuste no se
	// arrastre al resto del fichero (cabeceras filtradas, pares incompletos...).
	for i := 0; i+1 < len(lines); i++ {
		nombre := normalizeChannelName(lines[i])
		if nombre == "" {
			continue
		}
		nombre = normalizarNombreACB(nombre)
		acestreamLink := strings.TrimSpace(lines[i+1])
		// tokyo_elcano entrega "acestream://<hash>"; el player solo acepta hash
		// 40 hex puro, así que se normaliza igual que en las fuentes M3U.
		if hash := extractHashFromLink(acestreamLink); hash != "" {
			acestreamLink = hash
		} else if !strings.Contains(acestreamLink, "://") && !strings.HasPrefix(acestreamLink, "p;") {
			if os.Getenv("ENV") == "dev" {
				log.Printf("⚠️  txtRaw skip línea sin enlace: name %q -> %q", nombre, acestreamLink)
			}
			continue
		}

		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
		i++ // par consumido: saltar la línea del enlace
	}

	return extractedData
}

var (
	reArrow      = regexp.MustCompile(`\s*-->.*$`)
	reParens     = regexp.MustCompile(`\([^)]*\)`)
	reBrackets   = regexp.MustCompile(`\[[^]]*\]`)
	reStars      = regexp.MustCompile(`\*+`)
	reQuality    = regexp.MustCompile(`(?i)\b(4K|UHD|FHDp|FHD|HDp|HD|SDp|SD|720p|1080p|2160p)\b`)
	reMultiSpace = regexp.MustCompile(`\s+`)
	reDotsuffix  = regexp.MustCompile(`\s*\.\.\.[a-f0-9]{2,}$`)
	reHash40     = regexp.MustCompile(`^[a-f0-9]{40}$`)
	// reHash40Anywhere: búsqueda no anclada para hashes embebidos (evita
	// compilar la regex en cada llamada a extractHashFromLink).
	reHash40Anywhere = regexp.MustCompile(`[a-f0-9]{40}`)
)

// NormalizeChannelName limpia y normaliza el nombre del canal
func normalizeChannelName(input string) string {

	s := input

	// 1. eliminar todo después de -->
	s = reArrow.ReplaceAllString(s, "")

	// 2. eliminar (...) y [...]
	s = reParens.ReplaceAllString(s, "")
	s = reBrackets.ReplaceAllString(s, "")

	// 3. eliminar *
	s = reStars.ReplaceAllString(s, "")

	// 4. eliminar etiquetas de calidad
	s = reQuality.ReplaceAllString(s, "")

	// 5. trim
	s = strings.TrimSpace(s)

	// 5b. eliminar cola ...hex (ej. "Canal 1 ...37d")
	s = reDotsuffix.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// 6. normalizar espacios
	s = reMultiSpace.ReplaceAllString(s, " ")

	return s
}

// normalizeTolerant normaliza para gateway tolerante: normalize + +/. -> espacio + UPPER
func normalizeTolerant(input string) string {
	s := normalizeChannelName(input)
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "+", " ")
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)
	return s
}

var (
	normGateway     map[string][]string
	normGatewayOnce sync.Once
)

func ensureNormGateway() {
	normGatewayOnce.Do(func() {
		normGateway = make(map[string][]string)
		for k, v := range broadcasterGatewayMap {
			nk := normalizeTolerant(k)
			if existing, ok := normGateway[nk]; ok {
				normGateway[nk] = removeDuplicates(append(existing, v...))
			} else {
				normGateway[nk] = append([]string(nil), v...)
			}
		}
		if os.Getenv("ENV") == "dev" {
			log.Printf("🔧 normGateway inicializado: %d claves normalizadas (de %d originales)", len(normGateway), len(broadcasterGatewayMap))
		}
	})
}

// extractHashFromLink extrae hash 40 hex lower desde acestream://, ?id=, o hash puro
func extractHashFromLink(link string) string {
	s := strings.TrimSpace(link)
	if s == "" {
		return ""
	}
	// caso acestream://<hash>
	if strings.HasPrefix(strings.ToLower(s), "acestream://") {
		h := strings.TrimSpace(s[len("acestream://"):])
		// puede venir con params? tomar hasta ? o &
		if idx := strings.IndexAny(h, "?&"); idx != -1 {
			h = h[:idx]
		}
		h = strings.ToLower(strings.TrimSpace(h))
		if reHash40.MatchString(h) {
			return h
		}
		// fallback: buscar 40 hex embebido (tolerante a typos como 39+y)
		if m := reHash40Anywhere.FindString(h); m != "" {
			return m
		}
		return ""
	}
	// caso URL con ?id= || ?infohash= || ?hash= (backend agnóstico, player hace dual-try ?id ↔ ?infohash)
	if strings.Contains(s, "://") {
		parsed, err := url.Parse(s)
		if err == nil {
			// buscar case-insensitive en query keys
			var candidate string
			for k, vals := range parsed.Query() {
				lk := strings.ToLower(k)
				if lk == "id" || lk == "infohash" || lk == "hash" || lk == "info_hash" {
					if len(vals) > 0 && vals[0] != "" {
						candidate = vals[0]
						break
					}
				}
			}
			if candidate != "" {
				h := strings.ToLower(strings.TrimSpace(candidate))
				if reHash40.MatchString(h) {
					return h
				}
				if m := reHash40Anywhere.FindString(strings.ToLower(candidate)); m != "" {
					return m
				}
				return ""
			}
		}
		// fallback: buscar 40 hex en toda la URL (cubre variantes no contempladas)
		if m := reHash40Anywhere.FindString(strings.ToLower(s)); m != "" {
			return m
		}
		return ""
	}
	// hash puro
	h := strings.ToLower(strings.TrimSpace(s))
	if reHash40.MatchString(h) {
		return h
	}
	// buscar 40 hex embebido
	if m := reHash40Anywhere.FindString(h); m != "" {
		return m
	}
	return ""
}

func extractDataFromM3U_Manual(body []byte, filterList []string) map[string][]string {
	extractedData := make(map[string][]string)
	lines := strings.Split(string(body), "\n")
	var pendingName string
	hasPending := false
	filterUpper := make([]string, 0, len(filterList))
	for _, f := range filterList {
		f = strings.TrimSpace(f)
		if f != "" {
			filterUpper = append(filterUpper, strings.ToUpper(f))
		}
	}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF") {
			// extraer nombre tras última coma
			idx := strings.LastIndex(line, ",")
			var raw string
			if idx != -1 && idx+1 < len(line) {
				raw = line[idx+1:]
			} else {
				// sin coma, intentar tvg-name? skip
				hasPending = false
				pendingName = ""
				continue
			}
			nameNorm := normalizeChannelName(raw)
			if nameNorm == "" {
				hasPending = false
				pendingName = ""
				continue
			}
			// filtro allowlist
			if len(filterUpper) > 0 {
				upper := strings.ToUpper(nameNorm)
				matched := false
				for _, f := range filterUpper {
					if strings.Contains(upper, f) {
						matched = true
						break
					}
				}
				if !matched {
					hasPending = false
					pendingName = ""
					continue
				}
			}
			pendingName = nameNorm
			hasPending = true
			continue
		}
		if hasPending {
			if strings.HasPrefix(line, "#") {
				// comentario, no es URI, mantener pending para siguiente línea
				continue
			}
			hash := extractHashFromLink(line)
			if hash == "" {
				if os.Getenv("ENV") == "dev" {
					log.Printf("⚠️  M3U skip link sin hash: %q (name %q)", line, pendingName)
				}
				hasPending = false
				pendingName = ""
				continue
			}
			extractedData[pendingName] = append(extractedData[pendingName], hash)
			hasPending = false
			pendingName = ""
		}
	}
	return extractedData
}

// hostMuerto recuerda los hosts cuyos enlaces "p;" no resuelven, para que N
// enlaces del mismo servidor muerto costen un solo timeout en vez de N. Log de
// arranque: dos servidores directos muertos (181.78.106.127:9000 con
// "connection refused" y 190.92.10.66:4000 sin headers) costaban 20 s cada uno
// porque se recorrían en serie.
type hostMuerto struct {
	mu    sync.Mutex
	hosts map[string]bool
}

func newHostMuerto() *hostMuerto {
	return &hostMuerto{hosts: make(map[string]bool)}
}

func (h *hostMuerto) markDead(host string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hosts[host] = true
}

func (h *hostMuerto) isDead(host string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hosts[host]
}

// pLink es un enlace "p;" pendiente de resolver: el broadcaster al que
// pertenece y la URL inicial.
type pLink struct {
	broacasterKey string
	linkIndex     int
	initialURI    string
}

// transformUriSafeBroadcasters resuelve los enlaces "p;" y codifica todos los
// enlaces en base64 URI-safe.
//
// La resolución va en paralelo (pLinkWorkers) y con un timeout menor que el
// global, porque aquí no hace falta el manifiesto entero: solo la URL final
// tras las redirecciones. Además, cuando un host falla se marca como muerto
// para el resto de la pasada, así que N enlaces del mismo servidor muerto
// cuestan un solo timeout en vez de N.
func transformUriSafeBroadcasters(broadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	isDev := os.Getenv("ENV") == "dev"

	// Fase 1: recoger los enlaces "p;" sin tocar la red.
	type broadcasterJob struct {
		key      string
		original []string
		indices  []int
		initial  []string
	}
	var jobs []broadcasterJob
	for key, info := range broadcasterToAcestream {
		job := broadcasterJob{key: key}
		for i, link := range info.Links {
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, "p;") {
				job.indices = append(job.indices, i)
				job.initial = append(job.initial, strings.Split(link, "p;")[1])
			}
		}
		job.original = info.Links
		jobs = append(jobs, job)
	}

	// Fase 2: resolver en paralelo.
	resolved := map[pLink]string{}
	var pending []pLink
	for _, job := range jobs {
		for idx, initial := range job.initial {
			pending = append(pending, pLink{broacasterKey: job.key, linkIndex: job.indices[idx], initialURI: initial})
		}
	}
	if len(pending) > 0 {
		muertos := newHostMuerto()
		var muResolved sync.Mutex
		sem := make(chan struct{}, pLinkWorkers)
		var wg sync.WaitGroup
		for _, p := range pending {
			wg.Add(1)
			sem <- struct{}{}
			go func(p pLink) {
				defer wg.Done()
				defer func() { <-sem }()

				host := hostDeURL(p.initialURI)
				if host != "" && muertos.isDead(host) {
					if isDev {
						log.Printf("⏭️  [%s] host ya caído, se mantiene p; original %s", p.broacasterKey, p.initialURI)
					}
					return
				}

				client := IinitializeRedirectClients()
				finalURL, _, _, err := resolveFinalManifestURL(p.initialURI, client)
				StopRedirectClient(client)

				if err != nil || strings.TrimSpace(finalURL) == "" {
					if host != "" {
						muertos.markDead(host)
					}
					log.Printf("⚠️  [%s] no se pudo resolver p; link %s (err=%v) - manteniendo original", p.broacasterKey, p.initialURI, err)
					return
				}
				muResolved.Lock()
				resolved[p] = finalURL
				muResolved.Unlock()
			}(p)
		}
		wg.Wait()
	}

	// Fase 3: ensamblar los enlaces finales por broadcaster.
	for _, job := range jobs {
		newLinks := make([]string, 0, len(job.original))
		for i, link := range job.original {
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, "p;") {
				initialURI := strings.Split(link, "p;")[1]
				if finalURL, ok := resolved[pLink{broacasterKey: job.key, linkIndex: i, initialURI: initialURI}]; ok && strings.TrimSpace(finalURL) != "" {
					link = finalURL
				} else {
					link = initialURI
				}
			}
			if strings.TrimSpace(link) == "" {
				continue
			}
			if strings.Contains(link, ":") {
				encoded := changeLinkToUriSafe(link)
				link = fmt.Sprintf(";%s", encoded)
			}
			if strings.TrimSpace(link) != "" {
				newLinks = append(newLinks, link)
			}
		}
		info := broadcasterToAcestream[job.key]
		info.Links = removeDuplicates(newLinks)
		broadcasterToAcestream[job.key] = info
		// Solo avisar si tenía links y se quedó sin ninguno tras transformar (evita ruido de broadcasters que ya nacen vacíos como HYPERMOTION alias o DAZN LALIGA 3 recién creado)
		if isDev && len(job.original) > 0 && len(newLinks) == 0 {
			log.Printf("⚠️  [%s] quedó sin links válidos tras transformUriSafe (tenía %d, ahora 0)", job.key, len(job.original))
		}
	}
	return broadcasterToAcestream
}

// pLinkWorkers: nº de resoluciones "p;" simultáneas. Acota los conexiones a
// los servidores directos sin arrastrar una lluvia de peticiones.
const pLinkWorkers = 8

// pLinkTimeout: timeout para resolver una URL "p;". Solo interesa la URL final
// tras redirecciones, así que no hace falta esperar el manifiesto entero ni
// pagar los 20 s de timeTimeout por cada servidor caído.
const pLinkTimeout = 6 * time.Second

// hostDeURL extrae host:port de una URL para la cache de hosts caídos.
func hostDeURL(u string) string {
	parsed, err := url.Parse(strings.TrimSpace(u))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func changeLinkToUriSafe(url string) string {
	encodedRaw := base64.RawURLEncoding.EncodeToString([]byte(url))
	return encodedRaw
}

func extractDataFromM3U8(body []byte, filterList []string) (map[string][]string, error) {
	p, listType, err := m3u8.Decode(*bytes.NewBuffer(body), false)
	if err != nil {
		return nil, err
	}
	var mediapl *m3u8.MediaPlaylist
	// var masterpl *m3u8.MasterPlaylist
	switch listType {
	case m3u8.MEDIA:
		mediapl = p.(*m3u8.MediaPlaylist)
		fmt.Printf("%+v\n", mediapl)
	case m3u8.MASTER:
		return nil, fmt.Errorf("Not playlist")
		// masterpl = p.(*m3u8.MasterPlaylist)
		// fmt.Printf("%+v\n", masterpl)
	}
	extractedData := make(map[string][]string)
	for i := 0; i < len(mediapl.Segments); i++ {
		if mediapl.Segments[i] == nil {
			continue
		}
		name := mediapl.Segments[i].Title
		link := mediapl.Segments[i].URI
		extractedData[name] = append(extractedData[name], link)
	}
	return extractedData, nil
}

func resolveFinalManifestURL(initialURL string, redirectClient *http.Client) (finalURL string, finalHeaders http.Header, manifestBody []byte, err error) {
	return fetchWithRedirects(initialURL, redirectClient)
}

func checkActiveLinks(broadcasters map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	log.Printf(" 🔍 Comprobando enlaces activos...")
	for key := range broadcasters {
		log.Printf(" 🔍 Comprobando %s...", key)
		for i := len(broadcasters[key].Links) - 1; i >= 0; i-- {
			if strings.Contains(broadcasters[key].Links[i], ";") {
				// es un enlace codificado, no se puede comprobar
				log.Printf("Link codificado, no se puede comprobar: %s - %s", key, broadcasters[key].Links[i])
				continue
			}

			boolean, err := checkActiveLink(broadcasters[key].Links[i])
			if err != nil || !boolean {
				log.Printf("Link no activo: %s - %s", key, broadcasters[key].Links[i])
				currentBroadcaster := broadcasters[key]
				currentBroadcaster.Links = append(currentBroadcaster.Links[:i], currentBroadcaster.Links[i+1:]...)
				broadcasters[key] = currentBroadcaster
			} else {
				log.Printf("Link activo: %s - %s", key, broadcasters[key].Links[i])
			}
		}
	}
	return broadcasters
}

func checkActiveLink(initialURL string) (bool, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return http.ErrUseLastResponse
		},
		Timeout: timeTimeout,
	}

	currentURL := initialURL
	redirectCount := 0

	for {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return false, err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Connection", "keep-alive")

		resp, err := client.Do(req)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			redirectCount++
			if redirectCount > 10 {
				return false, err
			}

			location := resp.Header.Get("Location")
			if location == "" {
				return false, err
			}
			currentURL = location
			continue
		}

		if resp.StatusCode >= 400 {
			return false, nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		log.Printf("Manifest body: %s", string(body))
		return true, err
	}

}
