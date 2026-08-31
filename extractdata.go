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
}

const (
	shickatWeb         = "https://shickat.me/"
	elcanoWeb          = "https://ipfs.io/ipns/elcano.top"
	listaplana         = "https://k2k4r8lm8tkmuxbc8lkmq1in3v0oya1p6pe9o5bu0hu30br5ko08k2gb.ipns.dweb.link/data/listas/listaplana.txt"
	peticiones         = "https://raw.githubusercontent.com/Icastresana/lista1/refs/heads/main/peticiones"
	platinsport        = "https://raw.githubusercontent.com/tutw/platinsport-m3u-updater/refs/heads/main/lista_scraper_acestream_api.m3u"
	platinsportCanales = "https://raw.githubusercontent.com/tutw/platinsport-m3u-updater/refs/heads/main/canales_acestream.m3u"
	unificada          = "https://git.gay/a1975morales/ACESTREAM/raw/branch/main/lista_acestream_unificada.m3u"
	tokyoHashes        = "https://git.gay/TokyoGhoulles/AceStream_IDs/raw/branch/main/hashes.txt"
)

var sources = []Source{
	{Name: "listaplana", URL: listaplana, Type: SourceTxtRaw, Proxied: false},
	{Name: "peticiones", URL: peticiones, Type: SourceM3U, Proxied: false},
	{Name: "platinsport", URL: platinsport, Type: SourceM3U, Proxied: false},
	{Name: "platinsport_canales", URL: platinsportCanales, Type: SourceM3U, Proxied: false},
	{Name: "unificada", URL: unificada, Type: SourceM3U, Proxied: false},
	{Name: "tokyo_hashes", URL: tokyoHashes, Type: SourceTxtRaw, Proxied: false},
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
			var body []byte
			var err error
			var success bool
			for attempt := 1; attempt <= 10; attempt++ {
				if isDev {
					log.Printf("📡 [%s] intento %d/10 %s (proxied=%v)", s.Name, attempt, s.URL, s.Proxied)
				} else if attempt == 1 {
					log.Printf("📡 Obteniendo [%s] %s", s.Name, s.URL)
				}
				body, err = FetchWebData(s.URL, s.Proxied)
				if err == nil && len(body) != 0 {
					success = true
					break
				}
				if err != nil {
					is4xx := strings.Contains(err.Error(), "status code error: 4")
					if is4xx {
						if isDev {
							log.Printf("❌ [%s] 4xx no reintenta: %v", s.Name, err)
						} else {
							log.Printf("❌ [%s] error 4xx: %v", s.Name, err)
						}
						break
					}
					if isDev {
						log.Printf("⚠️  [%s] intento %d fallo: %v", s.Name, attempt, err)
					}
				} else if len(body) == 0 {
					if isDev {
						log.Printf("⚠️  [%s] intento %d body vacío", s.Name, attempt)
					}
				}
				if attempt < 10 {
					backoff := time.Duration(1<<uint(attempt-1)) * time.Second
					if backoff > 4*time.Second {
						backoff = 4 * time.Second
					}
					time.Sleep(backoff)
				}
			}
			if !success {
				log.Printf("❌ [%s] no se pudo obtener tras 10 intentos", s.Name)
				if err != nil {
					firstErrMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					firstErrMu.Unlock()
				}
				return
			}
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
			broadcasterToAcestream = updateBroadcasterMapWithGatewayTolerant(broadcasterToAcestream, extracted)
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

func extractDataFromWebTxtRaw(body []byte) map[string][]string {
	extractedData := make(map[string][]string)
	rawLines := strings.Split(string(body), "\n")
	// Filtrar cabeceras/ruido de tokyo_hashes y similares (===, AceStream IDs, Generated, Total, ====)
	var lines []string
	for _, l := range rawLines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "AceStream") || strings.HasPrefix(t, "Generated:") || strings.HasPrefix(t, "Total:") || strings.HasPrefix(t, "===") || strings.HasPrefix(t, "===") || t == "========================================" {
			continue
		}
		if t == "========================================" {
			continue
		}
		lines = append(lines, l)
	}
	for i := 0; i < len(lines); i += 2 {
		if i+1 >= len(lines) {
			break
		}
		
		nombre := normalizeChannelName(lines[i])
		if nombre == "ACB EVENTO 01" {
			nombre = "DAZN BALONCESTO 1"
		}
		if nombre == "ACB EVENTO 02" {
			nombre = "DAZN BALONCESTO 2"
		}
		if nombre == "ACB EVENTO 03" {
			nombre = "DAZN BALONCESTO 3"
		}
		acestreamLink := strings.TrimSpace(lines[i+1])

		if nombre == "" || acestreamLink == "" {
			continue
		}

		extractedData[nombre] = append(extractedData[nombre], acestreamLink)
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
				if m := regexp.MustCompile(`[a-f0-9]{40}`).FindString(strings.ToLower(candidate)); m != "" {
					return m
				}
				return ""
			}
		}
		// fallback: buscar 40 hex en toda la URL (cubre variantes no contempladas)
		if m := regexp.MustCompile(`[a-f0-9]{40}`).FindString(strings.ToLower(s)); m != "" {
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
	if m := regexp.MustCompile(`[a-f0-9]{40}`).FindString(h); m != "" {
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

func transformUriSafeBroadcasters(broadcasterToAcestream map[string]BroadcasterInfo) map[string]BroadcasterInfo {
	redirectClient := IinitializeRedirectClients()
	for key := range broadcasterToAcestream {
		for i := 0; i < len(broadcasterToAcestream[key].Links); i++ {
			if strings.Contains(broadcasterToAcestream[key].Links[i], "p;") {
				initialUri := strings.Split(broadcasterToAcestream[key].Links[i], "p;")[1]
				finalURL, _, _, _ := resolveFinalManifestURL(initialUri, redirectClient)
				// fmt.Println(string(manifestContent) + "...")
				// fmt.Printf("%v", headers)
				// fmt.Println(string(finalURL) + "...")
				broadcasterToAcestream[key].Links[i] = finalURL
			}
			if strings.Contains(broadcasterToAcestream[key].Links[i], ":") {
				encoded := changeLinkToUriSafe(broadcasterToAcestream[key].Links[i])
				broadcasterToAcestream[key].Links[i] = fmt.Sprintf(";%s", encoded)
			}
		}
	}
	StopRedirectClient(redirectClient)
	return broadcasterToAcestream
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
