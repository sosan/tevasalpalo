package main

// import (
// 	"fmt"
// 	"net/http"
// 	"regexp"
// 	"sort"
// 	"strings"
// 	"time"

// 	"github.com/PuerkitoBio/goquery"
// 	"golang.org/x/net/html/charset"
// 	"golang.org/x/text/encoding/htmlindex"
// 	"golang.org/x/text/transform"
// )

// type MatchView struct {
// 	Match
// 	Icon  string
// 	Sport string
// }

// // type DayView struct {
// // 	FormattedDate string
// // 	DateKey       string
// // 	Matches       []MatchView
// // }
// type DayView struct {
// 	FormattedDate string
// 	DateKey       string
// 	Competitions  map[string][]MatchView
// }
// // Mapeo de meses en español a número
// var monthMap = map[string]string{
// 	"enero":      "01",
// 	"febrero":    "02",
// 	"marzo":      "03",
// 	"abril":      "04",
// 	"mayo":       "05",
// 	"junio":      "06",
// 	"julio":      "07",
// 	"agosto":     "08",
// 	"septiembre": "09",
// 	"octubre":    "10",
// 	"noviembre":  "11",
// 	"diciembre":  "12",
// }

// func fetchScheduleMatchesMarca() ([]Match, error) {
// 	client := &http.Client{}

// 	// URL corregida (sin espacios)
// 	req, err := http.NewRequest("GET", "https://www.marca.com/programacion-tv.html", nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Añadir headers de navegador real
// 	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
// 	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
// 	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
// 	req.Header.Set("Referer", "https://www.google.com/")
// 	req.Header.Set("Connection", "keep-alive")
// 	req.Header.Set("Upgrade-Insecure-Requests", "1")

// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != 200 {
// 		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
// 	}

// 	// Manejar la codificación de caracteres
// 	var reader transform.Reader
// 	if resp.Header.Get("Content-Type") != "" {
// 		encoding, name, certain := charset.DetermineEncoding(nil, resp.Header.Get("Content-Type"))
// 		if !certain && name != "" {
// 			encoding, _ = htmlindex.Get(name)
// 		}
// 		if encoding != nil {
// 			reader = *transform.NewReader(resp.Body, encoding.NewDecoder())
// 		}
// 	}

// 	doc, err := goquery.NewDocumentFromReader(&reader)
// 	if err != nil {
// 		// Si falla, intentar con el body original
// 		doc, err = goquery.NewDocumentFromReader(resp.Body)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}

// 	var matches []Match

// 	doc.Find("div.schedule ol.daylist > li.content-item").Each(func(i int, s *goquery.Selection) {
// 		title := s.Find(".title-section-widget").Text()
// 		currentDay := cleanDate(title)

// 		s.Find("li.dailyevent").Each(func(j int, event *goquery.Selection) {
// 			competition := cleanTextSpace(event.Find("span.dailycompetition").Text())
// 			eventName := cleanTextSpace(event.Find("h4.dailyteams").Text())
// 			timeStr := cleanTextSpace(event.Find("strong.dailyhour").Text())
// 			channel := cleanTextSpace(event.Find("span.dailychannel").Text())

// 			// Limpiar el canal (remover el ícono "M+")
// 			// channel = strings.Replace(channel, "M+", "", 1)
// 			channel = strings.TrimSpace(channel)
// 			// channel = strings.TrimPrefix(channel, "M+ ")
// 			links := findLinkForBroadcaster(channel)

// 			newBroadcaster := BroadcasterInfo{
// 				Name:  channel,
// 				Links: links,
// 			}

// 			if currentDay != "" && competition != "" && eventName != "" {
// 				matches = append(matches, Match{
// 					Date:         currentDay,
// 					Competition:  normalizeCompetition(competition),
// 					Time:         timeStr,
// 					Event:        eventName,
// 					Broadcasters: []BroadcasterInfo{newBroadcaster}, // Inicializar con estructura BroadcastInfo
// 				})
// 			}
// 		})
// 	})

// 	return matches, nil
// }

// // cleanTextSpace limpia el texto de caracteres especiales
// func cleanTextSpace(text string) string {
// 	// Remover espacios extra y limpiar texto
// 	text = strings.TrimSpace(text)
// 	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
// 	return text
// }

// // cleanDate extrae y convierte "Sábado 23 de Agosto de 2025" → "2025-08-23"


// // normalizeCompetition normaliza el nombre de la competición
// func normalizeCompetition(comp string) string {
// 	// Remover espacios extra y normalizar
// 	comp = strings.TrimSpace(comp)
// 	comp = regexp.MustCompile(`\s+`).ReplaceAllString(comp, " ")

// 	// Correcciones específicas para caracteres problemáticos
// 	// comp = strings.ReplaceAll(comp, "G.P. HUNGRA", "G.P. HUNGRÍA")
// 	// comp = strings.ReplaceAll(comp, "G.P. HUNGRIA", "G.P. HUNGRÍA")

// 	return comp
// }

// func FormatDate(dateStr string) (string, error) {
// 	layout := "02-01-2006"
// 	t, err := time.Parse(layout, dateStr)
// 	if err != nil {
// 		return "", err
// 	}

// 	weekdays := map[time.Weekday]string{
// 		time.Sunday:    "Domingo",
// 		time.Monday:    "Lunes",
// 		time.Tuesday:   "Martes",
// 		time.Wednesday: "Miércoles",
// 		time.Thursday:  "Jueves",
// 		time.Friday:    "Viernes",
// 		time.Saturday:  "Sábado",
// 	}

// 	months := map[time.Month]string{
// 		time.January:   "Enero",
// 		time.February:  "Febrero",
// 		time.March:     "Marzo",
// 		time.April:     "Abril",
// 		time.May:       "Mayo",
// 		time.June:      "Junio",
// 		time.July:      "Julio",
// 		time.August:    "Agosto",
// 		time.September: "Septiembre",
// 		time.October:   "Octubre",
// 		time.November:  "Noviembre",
// 		time.December:  "Diciembre",
// 	}

// 	weekday := weekdays[t.Weekday()]
// 	day := t.Day()
// 	month := months[t.Month()]
// 	year := t.Year()

// 	return fmt.Sprintf("%s %d de %s de %d", weekday, day, month, year), nil
// }

// // // Para obtener el icono y tipo de deporte según la competición
// // func GetSportInfo(typeSport string) (icon string, sport string) {
// // 	typeSport = strings.ToUpper(typeSport)
// // 	switch {
// // 	case strings.Contains(typeSport, "FUTBOL") || strings.Contains(typeSport, "FÜTBOL"):
// // 		return "icon-futbol", "Fútbol"
// // 	case strings.Contains(typeSport, "MOTO") || strings.Contains(typeSport, "MOTOGP") || strings.Contains(typeSport, "MOTO3") || strings.Contains(typeSport, "MOTO2"):
// // 		return "icon-motociclismo", "Motos"
// // 	case strings.Contains(typeSport, "RUGBY") || strings.Contains(typeSport, "MUNDIAL FEMENINO") || strings.Contains(typeSport, "CHAMPIONSHIP"):
// // 		return "icon-rugby", "Rugby"
// // 	case strings.Contains(typeSport, "CICLISMO") || strings.Contains(typeSport, "VUELTA") || strings.Contains(typeSport, "ETAPA") || strings.Contains(typeSport, "CYCLASSICS"):
// // 		return "icon-ciclismo", "Ciclismo"
// // 	case strings.Contains(typeSport, "BALONCESTO") || strings.Contains(typeSport, "GIRA SELECCIÓN") || strings.Contains(typeSport, "LIVERPOOL") || strings.Contains(typeSport, "MILAN") || strings.Contains(typeSport, "CHELSEA") || strings.Contains(typeSport, "PSG") || strings.Contains(typeSport, "AJAX") || strings.Contains(typeSport, "PSV"):
// // 		return "icon-baloncesto", "Baloncesto"
// // 	case strings.Contains(typeSport, "GIMNASIA") || strings.Contains(typeSport, "CAMPEONATO DEL MUNDO") || strings.Contains(typeSport, "MEETING") || strings.Contains(typeSport, "SILESIA"):
// // 		return "icon-gimnasia", "Rítmica"
// // 	case strings.Contains(typeSport, "GOLF") || strings.Contains(typeSport, "LIV MICHIGAN") || strings.Contains(typeSport, "JORNADA"):
// // 		return "icon-golf", "Golf"
// // 	case strings.Contains(typeSport, "BOXEO") || strings.Contains(typeSport, "PESO SUPERLIGERO"):
// // 		return "icon-boxeo", "Boxeo"
// // 	case strings.Contains(typeSport, "MOTOR") || strings.Contains(typeSport, "INDYCAR") || strings.Contains(typeSport, "GRAND PRIX"):
// // 		return "icon-motor", "Motor"
// // 	case strings.Contains(typeSport, "PLAYA") || strings.Contains(typeSport, "SUPERCOPA MASCULINA"):
// // 		return "icon-futbol", "F. Playa"
// // 	case strings.Contains(typeSport, "ATLETISMO") || strings.Contains(typeSport, "SILESIA") || strings.Contains(typeSport, "MEETING"):
// // 		return "icon-atletismo", "Atletismo"
// // 	case strings.Contains(typeSport, "TENNIS") || strings.Contains(typeSport, "SINNER") || strings.Contains(typeSport, "ALCARAZ") || strings.Contains(typeSport, "DIALLO") || strings.Contains(typeSport, "MEDJE"):
// // 		return "icon-tenis", "Tenis"
// // 	default:
// // 		return "icon-deporte", "Deporte"
// // 	}
// // }

// func GroupMatchesByDate(matches []Match) map[string][]Match {
// 	grouped := make(map[string][]Match)
// 	for _, match := range matches {
// 		grouped[match.Date] = append(grouped[match.Date], match)
// 	}
// 	return grouped
// }

// // func PrepareView(matches []Match) ([]DayView, error) {
// // 	grouped := GroupMatchesByDate(matches)
// // 	var days []DayView

// // 	for date, matchList := range grouped {
// // 		formattedDate, err := FormatDate(date)
// // 		if err != nil {
// // 			return nil, err
// // 		}

// // 		var matchViews []MatchView
// // 		for _, m := range matchList {
// // 			icon, sport := GetSportInfo(m.Sport)
// // 			matchViews = append(matchViews, MatchView{
// // 				Match: m,
// // 				Icon:  icon,
// // 				Sport: sport,
// // 			})
// // 		}

// // 		days = append(days, DayView{
// // 			FormattedDate: formattedDate,
// // 			Matches:       matchViews,
// // 		})
// // 	}

// // 	// Ordenar por fecha
// // 	sort.Slice(days, func(i, j int) bool {
// // 		parts1 := strings.Split(days[i].FormattedDate, " ")
// // 		parts2 := strings.Split(days[j].FormattedDate, " ")

// // 		if len(parts1) >= 4 && len(parts2) >= 4 {
// // 			date1 := fmt.Sprintf("%s %s %s", parts1[len(parts1)-3], parts1[len(parts1)-2], parts1[len(parts1)-1])
// // 			date2 := fmt.Sprintf("%s %s %s", parts2[len(parts2)-3], parts2[len(parts2)-2], parts2[len(parts2)-1])

// // 			layout := "2 de January de 2006"
// // 			t1, _ := time.Parse(layout, date1)
// // 			t2, _ := time.Parse(layout, date2)

// // 			return t1.Before(t2)
// // 		}
// // 		return false
// // 	})

// // 	return days, nil
// // }
