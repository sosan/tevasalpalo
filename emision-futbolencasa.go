package main

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

type Match struct {
	Competition  string            `json:"competition"`
	Date         string            `json:"date"`
	Time         string            `json:"time"`
	Event        string            `json:"event"`
	Broadcasters []BroadcasterInfo `json:"channels"`
	Country      string            `json:"-"`
	Sport        string            `json:"-"`
}

type BroadcasterInfo struct {
	Name             string   `json:"name"`
	Logo             string   `json:"logo"`
	Links            []string `json:"link,omitempty"`
	ShowListChannels bool     `json:"showListChannels,omitempty"`
	Order            int      `json:"order,omitempty"`
}

type MatchView struct {
	Match
	Icon  string
	Sport string
}

type DayView struct {
	FormattedDate string
	DateKey       string
	Competitions  map[string][]MatchView
}

type CompetitionDetail struct {
	Titulo string `json:"titulo"`
	Top    bool   `json:"top"`
	Icon   string `json:"icon,omitempty"`
	Slug   string `json:"slug,omitempty"`
	Order  int    `json:"order,omitempty"`
}

type CountryCompetitions map[string]CompetitionDetail
type AllCompetitions map[string]CountryCompetitions

var topCompetitions map[string]CompetitionDetail

// Función auxiliar para obtener texto plano sin espacios extra
func cleanTextForTabsNewlines(s string) string {
	if strings.Contains(s, "(") {
		splitted := strings.Split(s, "(")
		s = splitted[0]
	}
	return strings.Join(strings.Fields(s), " ")
}

// Función auxiliar para obtener el título de la competición
func getCompetitionTitle(details *goquery.Selection) string {
	competitionLabel := details.Find("label").First()
	if title, exists := competitionLabel.Attr("title"); exists && title != "" {
		if title == "La Liga EA Sports" {
			title = "LaLiga"
		}
		return cleanTextForTabsNewlines(title)
	}

	if text := cleanTextForTabsNewlines(competitionLabel.Text()); text != "" {
		if text == "La Liga EA Sports" {
			text = "LaLiga"
		}
		return text
	}

	competitionSpan := details.Find("span").First()
	if title, exists := competitionSpan.Attr("title"); exists && title != "" {
		if title == "La Liga EA Sports" {
			title = "LaLiga"
		}
		return cleanTextForTabsNewlines(title)
	}

	return cleanTextForTabsNewlines(competitionSpan.Text())
}

// Función auxiliar para obtener el nombre de un equipo (local o visitante)
func getTeamName(cell *goquery.Selection) string {
	// Busca el span con el atributo title
	teamSpan := cell.Find("span[title]")
	if title, exists := teamSpan.Attr("title"); exists && title != "" {
		return cleanTextForTabsNewlines(title)
	}
	// Si no encuentra el span con title, usa el texto del span
	if text := cleanTextForTabsNewlines(teamSpan.Text()); text != "" {
		return text
	}
	// Como último recurso, usa el texto de la celda completa (limpiando imágenes)
	// cellClone := goquery.Clone(cell)
	cell.Find("img").Remove()
	return cleanTextForTabsNewlines(cell.Text())
}

func fetchScheduleMatchesFutbolEnCasa() ([]DayView, error) {
	// generalEvents, err := getCompetition("https://www.futbolenlatv.es/deporte", false)
	// if err != nil {
	// 	return nil, err
	// }

	// log.Println("Obteniendo programacion de la liga")
	// eventsFromLigaToMx, err := getCompetition("https://www.futbolenvivomexico.com/competicion/la-liga", false)
	// eventsFromBundesligaToPt, err := getCompetition("https://www.futebolnatv.pt/campeonato/bundesliga", true)
	// if err != nil {
	// 	eventsFromBundesligaToPt, err = getCompetition("https://www.futebolnatv.pt/campeonato/bundesliga", true)
	// }

	// // eventsFromBundesligaToPt = changeCompetitionName(eventsFromBundesligaToPt, "Bundesliga", "Bundesligass")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN", "DAZN 1 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 1", "DAZN 1 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 2", "DAZN 2 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 3", "DAZN 3 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 4", "DAZN 4 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 5", "DAZN 5 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 6", "DAZN 6 PT", "Bundesliga")

	// generalEvents = overrideCompetition(generalEvents, eventsFromBundesligaToPt)

	// // https://www.futbolenvivomexico.com/competicion/calcio-serie-a ESPN
	// // eventsFromCalcio, err := getCompetition("https://www.futebolnatv.pt/campeonato/calcio-serie-a")
	// // eventsFromCalcio, err := getCompetition("https://www.futbolenvivomexico.com/competicion/calcio-serie-a")
	// // if err != nil {
	// // 	return nil, err
	// // }
	// log.Println("Obteniendo programacion de ligue 1")
	// eventsFromLigue1ToPt, err := getCompetition("https://www.futebolnatv.pt/campeonato/ligue-1", false)

	// log.Println("Obteniendo programacion de calcio")
	// eventsFromCalcioToPt, err := getCompetition("https://www.futebolnatv.pt/campeonato/calcio-serie-a", false)
	// // if err != nil {
	// // 	return nil, err
	// // }
	// log.Println("Obteniendo programacion de mma")
	// eventsmma, err := getCompetition("https://www.futbolenlatv.es/deporte/mma", false)
	// generalEvents = mixCompetitions(generalEvents, eventsFromLigaToMx, eventsFromLigue1ToPt, eventsFromCalcioToPt, eventsmma, eventsFromBundesligaToPt)
	// return generalEvents, err

	// ---------------

	// Proxied=true usa la cadena xray/Tor como RESERVA (getCompetition va
	// directo primero). Timeout por source: futbolenvivoargentina tarda ~25s+.
	requests := []CompetitionRequest{
		{"https://www.futbolenlatv.es/deporte", true, "general", 0},
		{"https://www.futbolenvivomexico.com/competicion/la-liga", true, "laligaMX", 0},
		{"https://www.futebolnatv.pt/campeonato/bundesliga", true, "bundesligaPT", 0},
		{"https://www.footballtv.pl/rozgrywki/bundesliga", true, "bundesligaPL", 0},
		{"https://www.futebolnatv.pt/campeonato/ligue-1", true, "ligue1PT", 0},
		{"https://www.futebolnatv.pt/campeonato/calcio-serie-a", true, "calcioPT", 0},
		{"https://www.futbolenlatv.es/deporte/mma", true, "mma", 0},
		{"https://www.footballtv.pl/rozgrywki/calcio-serie-a", true, "calcioPL", 0},
		{"https://www.futbolenvivoargentina.com/deporte", true, "argentina", 45 * time.Second},
		// {"https://www.futbolenlatv.es/deporte/baloncesto", true, "baloncesto", 0},
	}

	results := FetchCompetitionsParallel(requests, getCompetition)

	// Recoger los resultados
	generalEvents := results["general"]
	eventsFromLigaToMx := results["laligaMX"]
	eventsFromBundesligaToPt := results["bundesligaPT"]
	eventsFromLigue1ToPt := results["ligue1PT"]
	eventsFromCalcioToPt := results["calcioPT"]
	eventsFromCalcioToPl := results["calcioPL"]
	eventsFromBundesligaToPl := results["bundesligaPL"]
	eventsmma := results["mma"]
	eventsArgentina := results["argentina"]
	// eventsbaloncesto := results["baloncesto"]

	// Ajustes de nombres de canales de Bundesliga
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN", "DAZN 1 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 1", "DAZN 1 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 2", "DAZN 2 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 3", "DAZN 3 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 4", "DAZN 4 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 5", "DAZN 5 PT", "Bundesliga")
	// eventsFromBundesligaToPt = changeBroadcasterName(eventsFromBundesligaToPt, "DAZN 6", "DAZN 6 PT", "Bundesliga")
	eventsmma = changeBroadcasterName(eventsmma, "", "UFC", "UFC")
	// eventsbaloncesto = changeBroadcasterName(eventsbaloncesto, "DAZN", "UFC", "UFC")

	// Sobrescribir y mezclar resultados
	// generalEvents = overrideCompetition(generalEvents, eventsFromBundesligaToPt)
	generalEvents = mixCompetitions(generalEvents, eventsFromLigaToMx, eventsFromLigue1ToPt, eventsFromCalcioToPt, eventsFromCalcioToPl, eventsmma, eventsFromBundesligaToPt, eventsFromBundesligaToPl)

	// Argentina (DSports / ESPN Argentina / TNT Sports) — https://www.futbolenvivoargentina.com/deporte
	if len(eventsArgentina) > 0 {
		for i := range generalEvents {
			generalEvents[i] = addCompetition(generalEvents[i], eventsArgentina)
		}
		// si general aún vacío (primer arranque) pero argentina tiene días, usarlos directamente
		if len(generalEvents) == 0 {
			generalEvents = eventsArgentina
		} else {
			// añadir días que solo existen en argentina (fechas nuevas)
			for _, argDay := range eventsArgentina {
				found := false
				for _, gDay := range generalEvents {
					if gDay.DateKey == argDay.DateKey {
						found = true
						break
					}
				}
				if !found {
					generalEvents = append(generalEvents, argDay)
				}
			}
		}
	}

	return generalEvents, nil

}

func getCompetition(req CompetitionRequest) ([]DayView, error) {
	// Se empieza por el proxy y no por directo: con el ISP cortando por DNS,
	// el directo se come los 10 intentos (y los timeouts) antes de llegar al
	// proxy que sí funciona. Con el proxy primero, un origen que responde va
	// al primer intento y uno bloqueado en directo tarda la mitad que antes.
	//
	// Dentro de cada transporte solo se reintenta si el error puede resolverlo un
	// reintento (transporte/DNS): un 404 o un 403 no se arregla repetidos 10
	// veces, y así se evita el minuto y medio de reintentos inútiles.
	type attempt struct {
		proxied bool
		label   string
	}
	attempts := []attempt{{true, "proxy"}, {false, "direct"}}
	if !req.Proxied {
		attempts = attempts[1:]
	}

	const intentosPorTransporte = 5

	var body []byte
	for _, a := range attempts {
		var err error
		for i := 1; i <= intentosPorTransporte; i++ {
			body, err = FetchWebDataTimeout(req.URL, a.proxied, req.Timeout)
			if err == nil && len(body) != 0 {
				if a.label == "proxy" {
					log.Printf("🔄 [%s] recuperado vía proxy (intento %d/%d)", req.Name, i, intentosPorTransporte)
				}
				break
			}
			if !retryWorthIt(err) {
				log.Printf("❌ [%s] fallo no recuperable en %s: %v", req.Name, a.label, err)
				break
			}
			log.Printf("Intento FALLIDO %d [%s/%s]: Obteniendo datos de %s (proxied=%v) error=%v", i, req.Name, a.label, req.URL, a.proxied, err)
		}
		if len(body) != 0 {
			break
		}
	}

	dayviews, err := prepareMatchDay(body)
	return dayviews, err
}

// retryWorthIt decide si merece la pena repetir. Los fallos de red, DNS y
// timeout se reintentan siempre. Entre los estados HTTP solo se reintenta lo que
// puede cambiar por sí solo: 429 (rate limit) y 5xx (error del servidor). Un 4xx
// —404, 403, 410— es una respuesta definitiva y repetirlo solo multiplica la
// espera.
func retryWorthIt(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	// Errores que no son de estado HTTP: red, DNS, timeouts. Se reintenta.
	if !strings.Contains(msg, "status code error") {
		return true
	}
	// 429 (rate limit) y 5xx pueden mejorar solos.
	return strings.Contains(msg, "429") || strings.Contains(msg, " 5")
}

func prepareMatchDay(body []byte) ([]DayView, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error al parsear el HTML: %w", err)
	}

	var days []DayView
	var currentDayView *DayView

	doc.Find("tr").Each(func(i int, row *goquery.Selection) {
		if row.HasClass("cabeceraTabla") {
			titleCompetitions := row.Text()
			dateKeyRaw := cleanDate(titleCompetitions)
			dateKeyRaw = strings.ReplaceAll(dateKeyRaw, "/", "-")
			dateKeyRaw = strings.ReplaceAll(dateKeyRaw, ".", "-")

			formattedDate, err := FormatDateDMYToSpanish(dateKeyRaw)
			if err != nil {
				fmt.Printf("Error formateando fecha %s: %v\n", dateKeyRaw, err)
				formattedDate = dateKeyRaw
			}
			// Normalizar DateKey a formato canónico con ceros para que general y argentina coincidan (6-9-2026 vs 06-09-2026)
			dateKey := dateKeyRaw
			var t time.Time
			for _, layout := range []string{"02-01-2006", "2-01-2006", "02-1-2006", "2-1-2006"} {
				t, err = time.Parse(layout, dateKeyRaw)
				if err == nil {
					dateKey = t.Format("02-01-2006")
					break
				}
			}

			newDayView := DayView{
				FormattedDate: formattedDate,
				DateKey:       dateKey,
				Competitions:  make(map[string][]MatchView), // Inicializamos el mapa
			}

			days = append(days, newDayView)
			currentDayView = &days[len(days)-1]
			return
		}

		if currentDayView == nil {
			return
		}

		timeStr := cleanTextForTabsNewlines(row.Find("td.hora").Text())
		if timeStr == "" {
			return
		}

		details := row.Find("td.detalles")
		competitionName := getCompetitionTitle(details)

		var sport string = "Desconocido"
		sportImg := details.Find("ul > li > div.contenedorImgCompeticion img")
		if sportImg.Length() > 0 {
			if title, exists := sportImg.Attr("title"); exists && strings.TrimSpace(title) != "" {
				sport = strings.TrimSpace(title)
			} else if alt, exists := sportImg.Attr("alt"); exists && strings.TrimSpace(alt) != "" {
				sport = strings.TrimSpace(alt)
			}
		}

		localTeamCell := row.Find("td.local")
		visitorTeamCell := row.Find("td.visitante")
		uniqueEventCell := row.Find("td.eventoUnaColumna span.eventoUnico")

		eventName := ""
		if uniqueEventCell.Length() > 0 {
			eventName = cleanTextForTabsNewlines(uniqueEventCell.Text())
		} else {
			localTeam := getTeamName(localTeamCell)
			visitorTeam := getTeamName(visitorTeamCell)

			if localTeam != "" && visitorTeam != "" {
				eventName = fmt.Sprintf("%s - %s", localTeam, visitorTeam)
			} else if localTeam != "" {
				eventName = localTeam
			} else {
				eventName = "Partido desconocido"
			}
		}

		broadcasters := []BroadcasterInfo{}
		row.Find("ul.listaCanales li").Each(func(j int, channel *goquery.Selection) {
			channelName := ""
			if title, exists := channel.Attr("title"); exists && title != "" {
				channelName = cleanTextForTabsNewlines(title)
			} else {
				channelName = cleanTextForTabsNewlines(channel.Text())
			}

			if channelName != "" {
				channelName = cleanChannelNameFutbolEnCasa(channelName)
				if channelName == "" {
					return
				}
				if sport == "Baloncesto" && strings.ToUpper(competitionName) == "COPA DEL REY" {
					competitionName = "Copa del Rey Baloncesto"
				}
				if sport == "Hockey Patines" && strings.ToUpper(competitionName) == "COPA DEL REY" {
					competitionName = "Copa del Rey Hockey Patines"
				}
				broadcaster := findBroadcaster(channelName, competitionName, sport)
				// filtrar broadcasters vacíos/desconocidos (name == "" -> no se conoce mapeo)
				if broadcaster.Name == "" {
					return
				}
				broadcasters = append(broadcasters, broadcaster)
			}
		})

		// dedup por nombre (case-insensitive): misma cadena Hypermotion aparece 2x en el HTML
		if len(broadcasters) > 1 {
			deduped := make(map[string]BroadcasterInfo)
			order := make([]string, 0, len(broadcasters))
			for _, b := range broadcasters {
				key := strings.ToUpper(strings.TrimSpace(b.Name))
				if existing, ok := deduped[key]; ok {
					existing.Links = removeDuplicates(append(existing.Links, b.Links...))
					deduped[key] = existing
				} else {
					deduped[key] = b
					order = append(order, key)
				}
			}
			tmp := make([]BroadcasterInfo, 0, len(deduped))
			for _, k := range order {
				tmp = append(tmp, deduped[k])
			}
			broadcasters = tmp
		}

		if timeStr != "" && competitionName != "" && eventName != "" {
			match := Match{
				Date:         currentDayView.DateKey,
				Competition:  competitionName,
				Time:         timeStr,
				Event:        eventName,
				Broadcasters: broadcasters,
				Sport:        sport,
			}

			icon, sportName := GetSportInfo(sport)

			matchView := MatchView{
				Match: match,
				Icon:  icon,
				Sport: sportName,
			}

			// Agrupar por competición
			currentDayView.Competitions[competitionName] = append(currentDayView.Competitions[competitionName], matchView)
		}
	})

	return days, nil
}

func cleanDate(input string) string {
	// Limpiar el texto
	input = cleanTextSpace(input)
	if strings.Contains(input, ", ") {
		dateRow := strings.Split(input, ", ")[1] // format 26/08/2025
		splittedDate := strings.ReplaceAll(dateRow, "/", "-")
		return splittedDate

	}
	log.Printf("⚠️  cleanDate: cabecera sin fecha %q, usando hoy (revisar selector)", input)
	return time.Now().Format("02-01-2006")
}

func cleanTextSpace(text string) string {
	// Remover espacios extra y limpiar texto
	text = strings.TrimSpace(text)
	text = reCleanSpaces.ReplaceAllString(text, " ")
	return text
}

var (
	reVerPartido     = regexp.MustCompile(`(?i)\s*:?\s*VER PARTIDO\s*$`)
	reMovistarParen  = regexp.MustCompile(`\s*\(M\d+ O\d+\)\s*$`)
	reDaznVerDirecto = regexp.MustCompile(`(?i)\s*\(Ver en directo\)\s*$`)
	reOrangeNumber   = regexp.MustCompile(`\s*\(\d+\)\s*$`)
	reCleanSpaces    = regexp.MustCompile(`\s+`)
)

func cleanChannelNameFutbolEnCasa(s string) string {
	s = strings.TrimSpace(s)
	s = reVerPartido.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reMovistarParen.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reDaznVerDirecto.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reOrangeNumber.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// Nota: el mapeo de LALIGA TV M1-M4 a Hypermotion se hace en findBroadcaster()
	// con contexto de competitionName, para no contaminar Serie A Italiana
	// con enlaces de Hypermotion (ej: Genoa-Como con "LaLiga TV M3" -> no es Hypermotion)
	return strings.TrimSpace(s)
}

func isRelevant(competition string) bool {
	// --- Define your relevant competitions ---
	// Use names that match the structure found in the HTML dump.
	// Be as specific as needed for the top tier.
	relevantLeagues := []string{
		"LaLigaEASports",
		"LaLigaHypermotion",
		"PremierLeague",
		"SerieA",
		"Ligue1",
		"Bundesliga",

		// Major Cups (adjust normalization as needed based on actual HTML text)
		"ChampionsLeague",
		"EuropaLeague",
		"CopadelRey",
		"SuperCopadeEspaña",
		"USOpen",
		"VueltaaEspaña",
	}

	// Normalize the input competition name
	normalizedComp := normalizeForComparisonWithoutSpaces(competition)

	// Check if the normalized competition name matches any normalized relevant league
	for _, league := range relevantLeagues {
		if normalizedComp == league {
			return true
		}
	}
	return false
}

func normalizeForComparisonWithoutSpaces(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// normalizeSportName limpia y normaliza el nombre del deporte para facilitar la comparación.
// Elimina espacios extra, convierte a minúsculas y elimina tildes/acentos.
func normalizeSportName(sport string) string {
	lowerSport := strings.ToLower(sport)

	// Eliminar acentos y tildes (simplificación básica)
	// Para una solución más robusta, considera usar la librería "golang.org/x/text/unicode/norm"
	// y "golang.org/x/text/transform" con "golang.org/x/text/runes".
	mapAccents := map[rune]rune{
		'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a',
		'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
		'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
		'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o',
		'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
		'ñ': 'n',
	}

	var normalized []rune
	for _, r := range lowerSport {
		if replacement, ok := mapAccents[r]; ok {
			normalized = append(normalized, replacement)
		} else if unicode.IsLetter(r) || unicode.IsSpace(r) { // Mantener letras y espacios
			normalized = append(normalized, r)
		}
		// Otros caracteres (como puntuación) se eliminan
	}

	// Eliminar espacios múltiples y trim
	return strings.Join(strings.Fields(string(normalized)), " ")
}

// Para obtener el icono y tipo de deporte según la competición
func GetSportInfo(typeSport string) (icon string, sport string) {
	normalizedType := normalizeSportName(typeSport)
	if normalizedType == "desconocido" {
		return "icon-deporte", "Deporte"
	}

	return fmt.Sprintf("icon-%s", normalizedType), typeSport
}

// FormatDateDMYToSpanish convierte una fecha DD-MM-YYYY a texto español
func FormatDateDMYToSpanish(dateStr string) (string, error) {
	// Soporta tanto "06-09-2026" como "6-9-2026" (futbolenlatv / futbolenvivoargentina devuelven sin ceros a la izquierda)
	layouts := []string{"02-01-2006", "2-01-2006", "02-1-2006", "2-1-2006"}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, strings.TrimSpace(dateStr))
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("error parseando fecha %s: %w", dateStr, err)
	}

	weekdays := map[time.Weekday]string{
		time.Sunday:    "Domingo",
		time.Monday:    "Lunes",
		time.Tuesday:   "Martes",
		time.Wednesday: "Miércoles",
		time.Thursday:  "Jueves",
		time.Friday:    "Viernes",
		time.Saturday:  "Sábado",
	}

	months := map[time.Month]string{
		time.January:   "Enero",
		time.February:  "Febrero",
		time.March:     "Marzo",
		time.April:     "Abril",
		time.May:       "Mayo",
		time.June:      "Junio",
		time.July:      "Julio",
		time.August:    "Agosto",
		time.September: "Septiembre",
		time.October:   "Octubre",
		time.November:  "Noviembre",
		time.December:  "Diciembre",
	}

	weekday := weekdays[t.Weekday()]
	day := t.Day()
	month := months[t.Month()]
	year := t.Year()

	return fmt.Sprintf("%s, %d de %s de %d", weekday, day, month, year), nil
}

func mixCompetitions(general, mxliga, ligue1, calcio, calcioPL, eventsmma, bundesligaPT, bundesligaPL []DayView) []DayView {
	general = changeBroadcasterName(general, "dazn", "DAZN CALCIO", "Serie A Italiana")
	general = changeBroadcasterName(general, "dazn", "", "LaLiga Hypermotion")
	eventsmma = changeBroadcasterName(eventsmma, "HBO MAX", "UFC", "UFC")
	if len(mxliga) == 0 {
		general = addNewBroadcaster(general, "SKY SPORTS", "LaLiga")
	}

	// general = addNewBroadcaster(general, "LIGA ENDESA 7", "Liga Endesa")
	// general = addNewBroadcaster(general, "DAZN 2", "Liga Endesa")
	// general = addNewBroadcaster(general, "DAZN 3", "Liga Endesa")
	// general = addNewBroadcaster(general, "DAZN 4", "Liga Endesa")

	if len(calcio) > 0 {
		calcio = changeCompetitionName(calcio, "Liga italiana", "Serie A Italiana")
	}
	if len(calcioPL) > 0 {
		calcioPL = changeCompetitionName(calcioPL, "Italian Serie A", "Serie A Italiana")
	}
	for i := range general {
		// general[i] = addCompetition(general[i], ligue1)
		general[i] = addCompetition(general[i], calcio)
		general[i] = addCompetition(general[i], calcioPL)
		general[i] = addCompetition(general[i], mxliga)
		general[i] = addCompetition(general[i], eventsmma)
		// general[i] = addCompetition(general[i], bundesligaPL)
		// general[i] = addCompetition(general[i], eventsbaloncesto)
	}
	return general
}

func addNewBroadcaster(days []DayView, broadcasterNameDestination, competitionName string) []DayView {
	for i := range days {
		for o := range days[i].Competitions {
			if o != competitionName {
				continue
			}
			for l := range days[i].Competitions[o] {
				newBroadcaster := broadcasterToAcestream[broadcasterNameDestination]
				days[i].Competitions[o][l].Broadcasters = append(days[i].Competitions[o][l].Broadcasters, newBroadcaster)
			}
		}
	}
	return days
}

// Afijos habituales en nombres de equipos que no cambian la identidad del club.
// Se comparan con espacio de borde para no recortar palabras ("CA " no toca "CÁDIZ").
// NOTA: no se recortan UNITED/CITY a propósito: "Manchester City" y
// "Manchester United" deben seguir siendo distintos.
var matchTeamPrefixes = []string{
	"REAL ", "FC ", "CF ", "RCD ", "RC ", "CD ", "UD ", "SD ", "AD ",
	"AC ", "AS ", "SC ", "CA ", "CLUB ",
}

var matchTeamSuffixes = []string{
	" CF", " FC", " CD", " SD", " UD", " RCD", " RC", " SC", " AC", " AS", " CA", " CLUB",
}

var matchAccentReplacer = strings.NewReplacer(
	"Á", "A", "À", "A", "Ä", "A", "Â", "A",
	"É", "E", "È", "E", "Ë", "E", "Ê", "E",
	"Í", "I", "Ì", "I", "Ï", "I", "Î", "I",
	"Ó", "O", "Ò", "O", "Ö", "O", "Ô", "O",
	"Ú", "U", "Ù", "U", "Ü", "U", "Û", "U",
	"Ñ", "N", "Ç", "C",
	"á", "a", "à", "a", "ä", "a", "â", "a",
	"é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i",
	"ó", "o", "ò", "o", "ö", "o", "ô", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c",
)

// normTeamName normaliza un nombre de equipo para comparar: mayúsculas,
// sin tildes, espacios colapsados y sin afijos de club (CF/FC/Real/...).
func normTeamName(s string) string {
	s = matchAccentReplacer.Replace(strings.ToUpper(strings.TrimSpace(s)))
	s = strings.Join(strings.Fields(s), " ")
	for i := 0; i < 3; i++ {
		prev := s
		for _, p := range matchTeamPrefixes {
			s = strings.TrimPrefix(s, p)
		}
		for _, suf := range matchTeamSuffixes {
			s = strings.TrimSuffix(s, suf)
		}
		s = strings.TrimSpace(s)
		if s == prev {
			break
		}
	}
	return s
}

// splitMatchTeams divide "Local - Visitante" (separador con espacios para no
// romper equipos con guion como Paris Saint-Germain). ok=false si no hay par.
func splitMatchTeams(event string) (teams [2]string, ok bool) {
	parts := strings.Split(event, " - ")
	if len(parts) != 2 {
		return teams, false
	}
	teams[0] = strings.TrimSpace(parts[0])
	teams[1] = strings.TrimSpace(parts[1])
	if teams[0] == "" || teams[1] == "" {
		return teams, false
	}
	return teams, true
}

// sameFootballMatch decide si dos cadenas describen el mismo partido.
// Compara los dos equipos como conjunto (insensible al orden local/visitante,
// que varía entre fuentes) tras normalizar cada equipo. Sin formato "A - B"
// recurre a igualdad exacta normalizada.
//
// Sustituye a la comparación por Contains, que fallaba su propio caso
// publicitado ("Valencia - Barcelona" vs "Valencia CF - FC Barcelona" no
// mergeaba) y solo se usa dentro del mismo DateKey, así que el orden no
// distingue partidos distintos. Validado offline contra 13 juicios TypeSafe
// (ver event_match_test.go). Limitación conocida: "Leeds" vs "Leeds United"
// no mergea (conservador frente a City/United).
func sameFootballMatch(a, b string) bool {
	ta, oka := splitMatchTeams(a)
	tb, okb := splitMatchTeams(b)
	if !oka || !okb {
		na := matchAccentReplacer.Replace(strings.ToLower(strings.Join(strings.Fields(a), "")))
		nb := matchAccentReplacer.Replace(strings.ToLower(strings.Join(strings.Fields(b), "")))
		return na != "" && na == nb
	}
	na := []string{normTeamName(ta[0]), normTeamName(ta[1])}
	nb := []string{normTeamName(tb[0]), normTeamName(tb[1])}
	sort.Strings(na)
	sort.Strings(nb)
	return na[0] == nb[0] && na[1] == nb[1] && na[0] != ""
}

func addCompetition(generalCompetition DayView, newCompetition []DayView) DayView {
	for j := range newCompetition {
		if newCompetition[j].DateKey == generalCompetition.DateKey {
			for compKey, matches := range newCompetition[j].Competitions {
				if generalCompetition.Competitions[compKey] != nil {
					for _, match := range matches {
						found := false
						for o := range generalCompetition.Competitions[compKey] {
							// match insensible a orden/afijos ("Valencia - Barcelona" == "Valencia CF - FC Barcelona")
							if sameFootballMatch(generalCompetition.Competitions[compKey][o].Match.Event, match.Event) {
								found = true
								merged := append(generalCompetition.Competitions[compKey][o].Broadcasters, match.Broadcasters...)
								// dedup por nombre para no duplicar LALIGA HYPERMOTION desde distintas fuentes
								deduped := make(map[string]BroadcasterInfo)
								order := make([]string, 0, len(merged))
								for _, b := range merged {
									if b.Name == "" {
										continue
									}
									key := strings.ToUpper(strings.TrimSpace(b.Name))
									if existing, ok := deduped[key]; ok {
										existing.Links = removeDuplicates(append(existing.Links, b.Links...))
										deduped[key] = existing
									} else {
										deduped[key] = b
										order = append(order, key)
									}
								}
								tmp := make([]BroadcasterInfo, 0, len(deduped))
								for _, k := range order {
									tmp = append(tmp, deduped[k])
								}
								generalCompetition.Competitions[compKey][o].Broadcasters = tmp
								break
							}
						}
						if !found {
							// evento nuevo que no existía en general (ej: Valencia CF - FC Barcelona solo en argentina) → añadirlo
							generalCompetition.Competitions[compKey] = append(generalCompetition.Competitions[compKey], MatchView{
								Match: match.Match,
								Icon:  match.Icon,
								Sport: match.Sport,
							})
						}
					}
				} else {
					generalCompetition.Competitions[compKey] = matches
				}
			}
		}
	}
	return generalCompetition
}

func changeBroadcasterName(days []DayView, broadcasterNameOrigin, broadcasterNameDestination, competitionName string) []DayView {
	for i := range days {
		for o := range days[i].Competitions {
			if o != competitionName {
				continue
			}
			for l := range days[i].Competitions[o] {
				for m := range days[i].Competitions[o][l].Broadcasters {
					if strings.EqualFold(days[i].Competitions[o][l].Broadcasters[m].Name, broadcasterNameOrigin) {
						// days[i].Competitions[o][l].Broadcasters[m].Name = broadcasterNameDestination // "sky sports" "SKY SPORTS BUNDESLIGA"
						currentBroadcaster := broadcasterToAcestream[broadcasterNameDestination]
						days[i].Competitions[o][l].Broadcasters[m] = currentBroadcaster

					}
				}
			}
		}
	}

	return days
}

func changeCompetitionName(days []DayView, competitionNameOrigin, competitionNameDestination string) []DayView {
	for i := range days {
		if matches, ok := days[i].Competitions[competitionNameOrigin]; ok {
			for l := range matches {
				matches[l].Match.Competition = competitionNameDestination
				matches[l].Match.Sport = competitionNameDestination
				matches[l].Sport = competitionNameDestination
			}
			days[i].Competitions[competitionNameDestination] = matches
			delete(days[i].Competitions, competitionNameOrigin)
		}
	}
	return days
}

// func getPremierLeagueMatchesFromDAZNChannels(generalEvents []DayView) ([]DayView, error) {
// 	var dayviews []DayView
// 	for i := range generalEvents {
// 		if _, ok := generalEvents[i].Competitions["Premier League"]; !ok {
// 			continue
// 		}
// 		parsedDate, _ := time.Parse("02-01-2006", generalEvents[i].DateKey)
// 		usaParsedDate := parsedDate.Format("2006-01-02")
// 		uri := fmt.Sprintf("https://tvepg.eu/es/spain/epg/sports/%s", usaParsedDate)
// 		body, err := FetchWebData(uri, false)
// 		if err != nil {
// 			return nil, err
// 		}

// 		tempdays, err := extractPremierLeagueMatchesFromDAZNChannels(body, generalEvents[i].DateKey, generalEvents[i].FormattedDate, usaParsedDate)
// 		if err != nil {
// 			return nil, err
// 		}
// 		if len(tempdays.Competitions) == 0 {
// 			continue
// 		}
// 		dayviews = append(dayviews, tempdays)
// 	}
// 	generalEvents = mixDaznChannelsforGeneralEvents(generalEvents, dayviews)

// 	return generalEvents, nil
// }

// func mixDaznChannelsforGeneralEvents(generalEvents []DayView, dayviews []DayView) []DayView {
// 	panic("unimplemented")
// }

// var channelNameMap = map[string]string{
// 	"dazn_1": "DAZN 1",
// 	"dazn_2": "DAZN 2",
// 	"dazn_3": "DAZN 3",
// 	"dazn_4": "DAZN 4",
// }

// func extractPremierLeagueMatchesFromDAZNChannels(body []byte, datekey, formatedDate, usaParsedDate string) (DayView, error) {
// 	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
// 	if err != nil {
// 		return DayView{}, fmt.Errorf("error al parsear el HTML: %w", err)
// 	}

// 	targetChannels := []string{"dazn_1", "dazn_2", "dazn_3", "dazn_4"}

// 	// // Supongamos que la fecha del EPG es 2025-09-20 (como se ve en los hrefs)
// 	// // Puedes extraerla del HTML si es variable
// 	// epgDateStr := "20250920" // Formato YYYYMMDD
// 	// // Convertir a FormattedDate (DD/MM/YYYY)
// 	// parsedDate, err := time.Parse("20060102", epgDateStr)
// 	// if err != nil {
// 	// 	log.Printf("Error parsing date %s: %v", epgDateStr, err)
// 	// 	// Fallback si no se puede parsear
// 	// 	epgDateStr = time.Now().Format("20060102")
// 	// 	parsedDate, _ = time.Parse("20060102", epgDateStr)
// 	// }
// 	// formattedDate := parsedDate.Format("02/01/2006")

// 	dayView := DayView{
// 		FormattedDate: formatedDate,
// 		DateKey:       datekey,
// 		Competitions:  make(map[string][]MatchView),
// 	}

// 	// Regex para extraer hora y evento del title/span text
// 	// Ejemplo: "15:50 Premier League (T25/26): Wolverhampton - Leeds"
// 	// Captura grupos: 1=Hora, 2=Competición completa, 3=Evento
// 	eventRegex := regexp.MustCompile(`^(\d{2}:\d{2})\s+(.+?):\s*(.+)$`)

// 	// var days []DayView
// 	// var currentDayView *DayView

// 	// Iterar por cada canal objetivo
// 	channelLinkSelector := "div.mr-tvgrid-row" //fmt.Sprintf("a[href='/es/spain/channel/%s/%s']", channelKey, usaParsedDate)
// 	// fmt.Println("Buscando enlace de canal:", channelLinkSelector) // Debug

// 	doc.Find(channelLinkSelector).Each(func(rowIndex int, row *goquery.Selection) {
// 		for _, channelKey := range targetChannels {
// 			programSelector := fmt.Sprintf("a[href*='/channel/%s/'][title*='Premier League' i]", channelKey)
// 			row.Find(programSelector).Each(func(programIndex int, program *goquery.Selection) {
// 				titleAttr, titleExists := program.Attr("title")
// 				if !titleExists {
// 					return
// 				}

// 				// Extraer datos del title o del span
// 				displayText := strings.TrimSpace(program.Find("span").Text())
// 				if displayText == "" {
// 					displayText = titleAttr
// 				}

// 				// Parsear displayText con regex
// 				matches := eventRegex.FindStringSubmatch(displayText)
// 				if len(matches) < 4 {
// 					log.Printf("Warning: Could not parse event format for: %s", displayText)
// 					return // Saltar si no coincide el formato esperado
// 				}

// 				timeStr := matches[1]
// 				// competitionFull := strings.TrimSpace(matches[2]) // "Premier League (T25/26)"
// 				eventStr := strings.TrimSpace(matches[3]) // "Wolverhampton - Leeds"

// 				// Extraer solo "Premier League" como clave de competición
// 				// Puedes usar strings.Split o regexp para esto también si es más complejo
// 				competitionKey := "Premier League" // Simplificación, asumiendo siempre es "Premier League ..."
// 				// Opción más robusta si hay variaciones:
// 				// parts := strings.SplitN(competitionFull, " (", 2)
// 				// if len(parts) > 0 { competitionKey = parts[0] }

// 				var broadcaster BroadcasterInfo
// 				channelName := channelNameMap[channelKey]
// 				if channelName != "" {
// 					broadcaster = broadcasterToAcestream[channelName]
// 					// links := findLinkForBroadcaster(channelName, "premiere league")
// 					// // broadcaster = append(broadcasters, BroadcasterInfo{Name: channelName, Links: links})
// 					// broadcaster := BroadcasterInfo{
// 					// 	Name: channelName,
// 					// 	Logo: channelLogo,
// 					// 	Links: links,
// 					// }
// 				}

// 				// Crear BroadcasterInfo

// 				// Crear Match
// 				match := Match{
// 					Competition:  competitionKey,
// 					Date:         datekey,
// 					Time:         timeStr,
// 					Event:        eventStr,
// 					Broadcasters: []BroadcasterInfo{broadcaster},
// 					Country:      "",       // Asumido
// 					Sport:        "Fútbol", // Asumido
// 				}

// 				// Crear MatchView
// 				matchView := MatchView{
// 					Match: match,
// 					Icon:  "icon-futbol",
// 					Sport: match.Sport,
// 				}

// 				// Añadir al DayView.Competitions map
// 				dayView.Competitions[competitionKey] = append(dayView.Competitions[competitionKey], matchView)
// 			})
// 		}
// 	})

// 	return dayView, nil
// }

func overrideCompetition(original, override []DayView) []DayView {
	for i := range original {
		for o := range override {
			if original[i].DateKey != override[o].DateKey {
				continue
			}
			original[i].Competitions["Bundesliga"] = override[o].Competitions["Bundesliga"]

		}
	}
	return original
}
