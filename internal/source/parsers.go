// Parseo de los cuerpos descargados de las fuentes: extractores por formato
// (elcano/shitkat/txtRaw/m3u), normalización de nombres, gateway tolerante y
// extracción de hashes.
package source

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"encoding/json"
	"tevasalpalo/internal/ace"
)

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
	// Guarda contra el spam: si lo que llega es una página HTML en vez de la
	// lista, avisar por cada línea genera cientos de mensajes que entierran
	// el log y no explican nada. Se detecta una vez y se dice una vez.
	if ace.SeemsHTML(body) {
		log.Printf("⚠️  txtRaw: la respuesta es HTML, no la lista (%.1f KB). "+
			"Lo normal es haber bajado el índice en vez del fichero.",
			float64(len(body))/1024)
		return extractedData
	}
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
