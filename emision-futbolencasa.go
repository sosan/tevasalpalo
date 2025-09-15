package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

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
	generalEvents, err := getCompetition("https://www.futbolenlatv.es/deporte")
	if err != nil {
		return nil, err
	}

	eventsFromLigaToMx, err := getCompetition("https://www.futbolenvivomexico.com/competicion/la-liga")
	if err != nil {
		return nil, err
	}

	eventsFromPremierToMx, err := getCompetition("https://www.futbolenvivomexico.com/competicion/premier-league")
	if err != nil {
		return nil, err
	}

	eventsFromCalcioToMx, err := getCompetition("https://www.futbolenvivomexico.com/competicion/calcio-serie-a")
	if err != nil {
		return nil, err
	}

	generalEvents = mixCompetitions(generalEvents, eventsFromLigaToMx, eventsFromPremierToMx, eventsFromCalcioToMx)
	return generalEvents, err
}

func getCompetition(uri string) ([]DayView, error) {
	body, err := FetchWebData(uri)
	if err != nil {
		return nil, err
	}

	dayviews, err := prepareMatchDay(body)
	// fmt.Printf("%+v\n", dayviews)
	return dayviews, err
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
			dateKey := cleanDate(titleCompetitions)
			dateKey = strings.ReplaceAll(dateKey, "/", "-")

			formattedDate, err := FormatDateDMYToSpanish(dateKey)
			if err != nil {
				fmt.Printf("Error formateando fecha %s: %v\n", dateKey, err)
				formattedDate = dateKey
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

		var broadcasters []BroadcasterInfo
		row.Find("ul.listaCanales li").Each(func(j int, channel *goquery.Selection) {
			channelName := ""
			if title, exists := channel.Attr("title"); exists && title != "" {
				channelName = cleanTextForTabsNewlines(title)
			} else {
				channelName = cleanTextForTabsNewlines(channel.Text())
			}

			if channelName != "" {
				links := findLinkForBroadcaster(channelName)
				broadcasters = append(broadcasters, BroadcasterInfo{Name: channelName, Links: links})
			}
		})

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
	return time.Now().Format("02-01-2006")
}

func cleanTextSpace(text string) string {
	// Remover espacios extra y limpiar texto
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return text
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
	// AQUÍ: dateStr es "DD-MM-YYYY"
	layout := "02-01-2006"
	t, err := time.Parse(layout, dateStr)
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

func mixCompetitions(general, liga, premier, calcio []DayView) []DayView {
	for i := range general {
		general[i] = addCompetition(general[i], liga)
		general[i] = addCompetition(general[i], premier)
		general[i] = addCompetition(general[i], calcio)
	}
	return general
}

func addCompetition(generalCompetition DayView, newCompetition []DayView) DayView {
	for j := range newCompetition {
		if newCompetition[j].DateKey == generalCompetition.DateKey {
			for compKey, matches := range newCompetition[j].Competitions {
				if generalCompetition.Competitions[compKey] != nil {
					for _, match := range matches {
						for o := range generalCompetition.Competitions[compKey] {
							if generalCompetition.Competitions[compKey][o].Match.Event == match.Event {
								generalCompetition.Competitions[compKey][o].Broadcasters = append(generalCompetition.Competitions[compKey][o].Broadcasters, match.Broadcasters...)
							}
						}
					}
				}
			}
		}
	}
	return generalCompetition
}
