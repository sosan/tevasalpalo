package main

import (
	"bytes"
	"fmt"

	// "io"
	// "net/http"
	"regexp"
	// "sort"
	"strings"
	"time"
	"unicode"

	// "time"

	"github.com/PuerkitoBio/goquery"
	// "golang.org/x/net/html/charset"
	// "golang.org/x/text/encoding/htmlindex"
	// "golang.org/x/text/transform"
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

// var defaultSportsInterest = map[string]string{
// "Fútbol": "Fútbol",
// "Automovilismo": "Automovilismo",
// "Tenis": "Tenis",
// "MMA": "MMA",

// }

// var defaultSpainCompetitionInterest = map[string]string{
// 	"La Liga EA Sports":      "La Liga EA Sports",
// 	"LaLiga Hypermotion":     "LaLiga Hypermotion",
// 	"Copa del Rey":           "Copa del Rey",
// 	"Supercopa de España":    "Supercopa de España",
// 	"Champions League":       "Champions League",
// 	"Europa League":          "Europa League",
// 	"Premier League":         "Premier League",
// 	"Bundesliga":             "Bundesliga",
// 	"Ligue 1":                "Ligue 1",
// 	"Serie A Italiana":       "Serie A Italiana",
// 	"Nations League":         "Nations League",
// 	"FIFA Copa Mundial 2026": "FIFA Copa Mundial 2026",
// 	"Mundial de Clubes":      "Mundial de Clubes",
// }

type CompetitionDetail struct {
	Titulo string `json:"titulo"`
	Top    bool   `json:"top"`
	Icon   string `json:"icon,omitempty"`
	Slug   string `json:"slug,omitempty"`
}

type CountryCompetitions map[string]CompetitionDetail
type AllCompetitions map[string]CountryCompetitions

var topCompetitions map[string]CompetitionDetail

var allCompetitions = AllCompetitions{
	"Sports": CountryCompetitions{
		"Tenis":                  {Titulo: "Tenis", Top: true, Icon: "tenis.png"},
		"FIFA Copa Mundial 2026": {Titulo: "Munidal", Top: true, Icon: "mundial.png"},
		"Mundial Clubes":         {Titulo: "Munidal", Top: true, Icon: "mundialclubes.png"},
		"FIA Fórmula 2":          {Titulo: "FIA Fórmula 2", Top: true, Icon: "formula2.png"},
		"FIA Fórmula 3":          {Titulo: "FIA Fórmula 3", Top: true, Icon: "formula3.png"},
		"Fórmula 1":              {Titulo: "Fórmula 1", Top: true, Icon: "formula1.png"},
		"Moto2":                  {Titulo: "Moto2", Top: true, Icon: "moto2.png"},
		"Moto3":                  {Titulo: "Moto3", Top: true, Icon: "moto3.png"},
		"MotoGP":                 {Titulo: "MotoGP", Top: true, Icon: "motogp.png"},
		"Boxeo":                  {Titulo: "Boxeo", Top: true, Icon: "boxeo.png"},
	},
	"España": CountryCompetitions{
		"LaLiga":                        {Titulo: "LaLiga", Top: true, Icon: "liga.png"},
		"LaLiga Hypermotion":            {Titulo: "LaLiga 2", Top: true, Icon: "liga2.png"},
		"Primera Federación":            {Titulo: "Primera Federación", Top: false, Icon: "uefa.png"},
		"Segunda Federación":            {Titulo: "Segunda Federación", Top: false, Icon: "uefa.png"},
		"Copa del Rey":                  {Titulo: "Copa del Rey", Top: false, Icon: "uefa.png"},
		"Supercopa de España":           {Titulo: "Supercopa de España", Top: false, Icon: "uefa.png"},
		"Liga F Moeve":                  {Titulo: "Liga F Moeve", Top: false, Icon: "uefa.png"},
		"Copa Federación":               {Titulo: "Copa Federación", Top: false, Icon: "uefa.png"},
		"Supercopa Femenina":            {Titulo: "Supercopa Femenina", Top: false, Icon: "uefa.png"},
		"Copa de SM La Reina":           {Titulo: "Copa de SM La Reina", Top: false, Icon: "uefa.png"},
		"División de Honor Juvenil":     {Titulo: "División de Honor Juvenil", Top: false, Icon: "uefa.png"},
		"Primera Federacion Women":      {Titulo: "Primera Federacion Women", Top: false, Icon: "uefa.png"},
		"Segunda Federación Femenina":   {Titulo: "Segunda Federación Femenina", Top: false, Icon: "uefa.png"},
		"Spain U19 Cup":                 {Titulo: "Spain U19 Cup", Top: false, Icon: "uefa.png"},
		"U19 Division de Honor Juvenil": {Titulo: "U19 Division de Honor Juvenil", Top: false, Icon: "uefa.png"},
	},
	"Inglaterra": CountryCompetitions{
		"Premier League":              {Titulo: "Premier League", Top: true, Icon: "premiere.png"},
		"Championship":                {Titulo: "Championship", Top: false, Icon: "uefa.png"},
		"League One":                  {Titulo: "League One", Top: false, Icon: "uefa.png"},
		"League Two":                  {Titulo: "League Two", Top: false, Icon: "uefa.png"},
		"National League":             {Titulo: "National League", Top: false, Icon: "uefa.png"},
		"FA Cup":                      {Titulo: "FA Cup", Top: false, Icon: "uefa.png"},
		"FA Cup, Qualification":       {Titulo: "FA Cup, Qualification", Top: false, Icon: "uefa.png"},
		"EFL Cup":                     {Titulo: "EFL Cup", Top: false, Icon: "uefa.png"},
		"Football League Trophy":      {Titulo: "Football League Trophy", Top: false, Icon: "uefa.png"},
		"FA Women's Championship":     {Titulo: "FA Women's Championship", Top: false, Icon: "uefa.png"},
		"Community Shield":            {Titulo: "Community Shield", Top: false, Icon: "uefa.png"},
		"Women's Super League":        {Titulo: "Women's Super League", Top: false, Icon: "uefa.png"},
		"Women's FA Cup":              {Titulo: "Women's FA Cup", Top: false, Icon: "uefa.png"},
		"FA Women's League Cup":       {Titulo: "FA Women's League Cup", Top: false, Icon: "uefa.png"},
		"England National League Cup": {Titulo: "England National League Cup", Top: false, Icon: "uefa.png"},
		"Baller League UK":            {Titulo: "Baller League UK", Top: false, Icon: "uefa.png"},
		"FA Youth Cup":                {Titulo: "FA Youth Cup", Top: false, Icon: "uefa.png"},
	},
	"Alemania": CountryCompetitions{
		"Bundesliga":    {Titulo: "Bundesliga", Top: true, Icon: "bundesliga.png"},
		"2. Bundesliga": {Titulo: "2. Bundesliga", Top: false, Icon: "uefa.png"},
		"DFB Pokal":     {Titulo: "DFB Pokal", Top: false, Icon: "uefa.png"},
		"DFL Supercup":  {Titulo: "DFL Supercup", Top: false, Icon: "uefa.png"},
	},
	"Italia": CountryCompetitions{
		"Serie A":                {Titulo: "Serie A", Top: true, Icon: "seriea.png"},
		"Serie B":                {Titulo: "Serie B", Top: false, Icon: "uefa.png"},
		"Campionato Primavera 1": {Titulo: "Campionato Primavera 1", Top: false, Icon: "uefa.png"},
		"Campionato Primavera 2": {Titulo: "Campionato Primavera 2", Top: false, Icon: "uefa.png"},
		"Serie C, Playoffs":      {Titulo: "Serie C, Playoffs", Top: false, Icon: "uefa.png"},
		"Supercoppa Serie C":     {Titulo: "Supercoppa Serie C", Top: false, Icon: "uefa.png"},
		"Serie D Poule Scudetto": {Titulo: "Serie D Poule Scudetto", Top: false, Icon: "uefa.png"},
		"Serie A Women":          {Titulo: "Serie A Women", Top: false, Icon: "uefa.png"},
		"Coppa Italia Femminile": {Titulo: "Coppa Italia Femminile", Top: false, Icon: "uefa.png"},
		"Supercoppa Primavera":   {Titulo: "Supercoppa Primavera", Top: false, Icon: "uefa.png"},
		"Trofeo Dossena":         {Titulo: "Trofeo Dossena", Top: false, Icon: "uefa.png"},
		"Serie D, Girone H":      {Titulo: "Serie D, Girone H", Top: false, Icon: "uefa.png"},
		"Serie B Femminile":      {Titulo: "Serie B Femminile", Top: false, Icon: "uefa.png"},
	},
	"Francia": CountryCompetitions{
		"Ligue 1":                  {Titulo: "Ligue 1", Top: true, Icon: "ligue1.png"},
		"Ligue 2":                  {Titulo: "Ligue 2", Top: false, Icon: "uefa.png"},
		"National 1":               {Titulo: "National 1", Top: false, Icon: "uefa.png"},
		"National 2":               {Titulo: "National 2", Top: false, Icon: "uefa.png"},
		"Coupe de France":          {Titulo: "Coupe de France", Top: false, Icon: "uefa.png"},
		"Trophée des Champions":    {Titulo: "Trophée des Champions", Top: false, Icon: "uefa.png"},
		"Première Ligue, Féminine": {Titulo: "Première Ligue, Féminine", Top: false, Icon: "uefa.png"},
		"Coupe de France, Women":   {Titulo: "Coupe de France, Women", Top: false, Icon: "uefa.png"},
		"Championnat National U19": {Titulo: "Championnat National U19", Top: false, Icon: "uefa.png"},
		"Seconde Ligue Women":      {Titulo: "Seconde Ligue Women", Top: false, Icon: "uefa.png"},
	},
	"Europa": CountryCompetitions{
		"UEFA Champions League":                     {Titulo: "UEFA Champions League", Top: true, Icon: "champions.png"},
		"UEFA Europa League":                        {Titulo: "UEFA Europa League", Top: true, Icon: "uefa.png"},
		"UEFA Conference League":                    {Titulo: "UEFA Conference League", Top: true, Icon: "conference.png"},
		"UEFA Super Cup":                            {Titulo: "UEFA Super Cup", Top: false, Icon: "uefa.png"},
		"UEFA Nations League":                       {Titulo: "UEFA Nations League", Top: false, Icon: "uefa.png"},
		"UEFA Women's Nations League":               {Titulo: "UEFA Women's Nations League", Top: false, Icon: "uefa.png"},
		"Women's Euro":                              {Titulo: "Women's Euro", Top: false, Icon: "uefa.png"},
		"Women's Euro, Qualification":               {Titulo: "Women's Euro, Qualification", Top: false, Icon: "uefa.png"},
		"U21 European Championship":                 {Titulo: "U21 European Championship", Top: false, Icon: "uefa.png"},
		"U21 Euro Qualification":                    {Titulo: "U21 Euro Qualification", Top: false, Icon: "uefa.png"},
		"U19 European Championship Qualif.":         {Titulo: "U19 European Championship Qualif.", Top: false, Icon: "uefa.png"},
		"U17 European Championship":                 {Titulo: "U17 European Championship", Top: false, Icon: "uefa.png"},
		"U17 European Championship, Qual.":          {Titulo: "U17 European Championship, Qual.", Top: false, Icon: "uefa.png"},
		"U19 European Women's Championship Qualif.": {Titulo: "U19 European Women's Championship Qualif.", Top: false, Icon: "uefa.png"},
		"U17 European Women's Championship":         {Titulo: "U17 European Women's Championship", Top: false, Icon: "uefa.png"},
		"UEFA Youth League":                         {Titulo: "UEFA Youth League", Top: false, Icon: "uefa.png"},
	},
	"América del Sur": CountryCompetitions{
		"CONMEBOL Libertadores":             {Titulo: "CONMEBOL Libertadores", Top: false, Icon: "uefa.png"},
		"CONMEBOL Sudamericana":             {Titulo: "CONMEBOL Sudamericana", Top: false, Icon: "uefa.png"},
		"CONMEBOL Recopa":                   {Titulo: "CONMEBOL Recopa", Top: false, Icon: "uefa.png"},
		"Copa América":                      {Titulo: "Copa América", Top: false, Icon: "uefa.png"},
		"World Cup Qual. CONMEBOL":          {Titulo: "World Cup Qual. CONMEBOL", Top: false, Icon: "uefa.png"},
		"U17 CONMEBOL Championship":         {Titulo: "U17 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Libertadores":         {Titulo: "U20 CONMEBOL Libertadores", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Championship":         {Titulo: "U20 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Women's Championship": {Titulo: "U20 CONMEBOL Women's Championship", Top: false, Icon: "uefa.png"},
		"Copa Libertadores Femenina":        {Titulo: "Copa Libertadores Femenina", Top: false, Icon: "uefa.png"},
		"Copa América Femenina":             {Titulo: "Copa América Femenina", Top: false, Icon: "uefa.png"},
		"U17 CONMEBOL Women's Championship": {Titulo: "U17 CONMEBOL Women's Championship", Top: false, Icon: "uefa.png"},
		"U13 Liga Evolución":                {Titulo: "U13 Liga Evolución", Top: false, Icon: "uefa.png"},
		"U16 Liga Evolución, Women":         {Titulo: "U16 Liga Evolución, Women", Top: false, Icon: "uefa.png"},
		"U14 Liga Evolución, Women":         {Titulo: "U14 Liga Evolución, Women", Top: false, Icon: "uefa.png"},
		"U15 CONMEBOL Championship":         {Titulo: "U15 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"CONMEBOL Pre-Olympic":              {Titulo: "CONMEBOL Pre-Olympic", Top: false, Icon: "uefa.png"},
	},
	"Brasil": CountryCompetitions{
		"Serie A":          {Titulo: "Serie A", Top: false, Icon: "uefa.png"},
		"Copa do Brasil":   {Titulo: "Copa do Brasil", Top: false, Icon: "uefa.png"},
		"Série B":          {Titulo: "Série B", Top: false, Icon: "uefa.png"},
		"Internacional":    {Titulo: "Internacional", Top: false, Icon: "uefa.png"},
		"Fortaleza SC":     {Titulo: "Fortaleza SC", Top: false, Icon: "uefa.png"},
		"Sport Recife":     {Titulo: "Sport Recife", Top: false, Icon: "uefa.png"},
		"Vasco da Gama":    {Titulo: "Vasco da Gama", Top: false, Icon: "uefa.png"},
		"Grêmio":           {Titulo: "Grêmio", Top: false, Icon: "uefa.png"},
		"Ceará":            {Titulo: "Ceará", Top: false, Icon: "uefa.png"},
		"São Paulo":        {Titulo: "São Paulo", Top: false, Icon: "uefa.png"},
		"Atlético Mineiro": {Titulo: "Atlético Mineiro", Top: false, Icon: "uefa.png"},
		"Palmeiras":        {Titulo: "Palmeiras", Top: false, Icon: "uefa.png"},
	},
	"Argentina": CountryCompetitions{
		"Primera División": {Titulo: "Primera División", Top: false, Icon: "uefa.png"},
		"Copa Argentina":   {Titulo: "Copa Argentina", Top: false, Icon: "uefa.png"},

		// "River Plate":       {Titulo: "River Plate", Top: false, Icon: "uefa.png" },
		// "San Martín SJ":     {Titulo: "San Martín SJ", Top: false, Icon: "uefa.png" },
		// "Racing Avellaneda": {Titulo: "Racing Avellaneda", Top: false, Icon: "uefa.png" },
		// "Unión Santa Fe":    {Titulo: "Unión Santa Fe", Top: false, Icon: "uefa.png" },
		// "Gimnasia LP":       {Titulo: "Gimnasia LP", Top: false, Icon: "uefa.png" },
		// "Atlético Tucumán":  {Titulo: "Atlético Tucumán", Top: false, Icon: "uefa.png" },
		// "Platense":          {Titulo: "Platense", Top: false, Icon: "uefa.png" },
		// "Godoy Cruz":        {Titulo: "Godoy Cruz", Top: false, Icon: "uefa.png" },
		// "Estudiantes LP":    {Titulo: "Estudiantes LP", Top: false, Icon: "uefa.png" },
		// "Aldosivi":          {Titulo: "Aldosivi", Top: false},
		// "Independiente":     {Titulo: "Independiente", Top: false},
		// "Miramar Misiones":  {Titulo: "Miramar Misiones", Top: false},
		// "Cerro Largo":       {Titulo: "Cerro Largo", Top: false},
	},

	"Colombia": CountryCompetitions{
		"Primera A":                {Titulo: "Primera A", Top: false, Icon: "uefa.png"},
		"Copa Colombia":            {Titulo: "Copa Colombia", Top: false, Icon: "uefa.png"},
		"Santa Fe":                 {Titulo: "Santa Fe", Top: false, Icon: "uefa.png"},
		"Once Caldas":              {Titulo: "Once Caldas", Top: false, Icon: "uefa.png"},
		"Deportes Tolima":          {Titulo: "Deportes Tolima", Top: false, Icon: "uefa.png"},
		"Bucaramanga":              {Titulo: "Bucaramanga", Top: false, Icon: "uefa.png"},
		"Águilas Doradas Rionegro": {Titulo: "Águilas Doradas Rionegro", Top: false, Icon: "uefa.png"},
		"Boyacá Chicó":             {Titulo: "Boyacá Chicó", Top: false, Icon: "uefa.png"},
		"LDU Quito":                {Titulo: "LDU Quito", Top: false, Icon: "uefa.png"},
		"El Nacional":              {Titulo: "El Nacional", Top: false, Icon: "uefa.png"},
	},
	"Venezuela": CountryCompetitions{
		"Primera División": {Titulo: "Primera División", Top: false, Icon: "uefa.png"},
		"Trujillanos":      {Titulo: "Trujillanos", Top: false, Icon: "uefa.png"},
		"Héroes de Falcón": {Titulo: "Héroes de Falcón", Top: false, Icon: "uefa.png"},
	},
	"Ecuador": CountryCompetitions{
		"Serie A":      {Titulo: "Serie A", Top: false, Icon: "uefa.png"},
		"Barcelona SC": {Titulo: "Barcelona SC", Top: false, Icon: "uefa.png"},
		"U. Católica":  {Titulo: "U. Católica", Top: false, Icon: "uefa.png"},
		"Delfín SC":    {Titulo: "Delfín SC", Top: false, Icon: "uefa.png"},
		"Libertad FC":  {Titulo: "Libertad FC", Top: false, Icon: "uefa.png"},
		"LDU Quito":    {Titulo: "LDU Quito", Top: false, Icon: "uefa.png"},
		"El Nacional":  {Titulo: "El Nacional", Top: false, Icon: "uefa.png"},
	},
	"Estados Unidos": CountryCompetitions{
		"Major League Soccer (MLS)": {Titulo: "Major League Soccer (MLS)", Top: false, Icon: "uefa.png"},
		"US Open Cup":               {Titulo: "US Open Cup", Top: false, Icon: "uefa.png"},
		"Seattle Sounders":          {Titulo: "Seattle Sounders", Top: false, Icon: "uefa.png"},
		"Inter Miami CF":            {Titulo: "Inter Miami CF", Top: false, Icon: "uefa.png"},
		"Los Angeles FC":            {Titulo: "Los Angeles FC", Top: false, Icon: "uefa.png"},
		"San Diego FC":              {Titulo: "San Diego FC", Top: false, Icon: "uefa.png"},
		"Columbus Crew":             {Titulo: "Columbus Crew", Top: false, Icon: "uefa.png"},
		"New England Revolution":    {Titulo: "New England Revolution", Top: false, Icon: "uefa.png"},
		"FC Cincinnati":             {Titulo: "FC Cincinnati", Top: false, Icon: "uefa.png"},
		"New York City":             {Titulo: "New York City", Top: false, Icon: "uefa.png"},
		"Sporting KC":               {Titulo: "Sporting KC", Top: false, Icon: "uefa.png"},
	},
	"México": CountryCompetitions{
		"Liga MX": {Titulo: "Liga MX", Top: false, Icon: "uefa.png"},
		"Copa MX": {Titulo: "Copa MX", Top: false, Icon: "uefa.png"},

		"Chivas Guadalajara": {Titulo: "Chivas Guadalajara", Top: false, Icon: "uefa.png"},
		"Club América":       {Titulo: "Club América", Top: false, Icon: "uefa.png"},
	},
	"Arabia Saudita": CountryCompetitions{
		"Saudi Professional League": {Titulo: "Saudi Professional League", Top: false, Icon: "uefa.png"},

		"Al Nassr": {Titulo: "Al Nassr", Top: false, Icon: "uefa.png"},
		"Al Hilal": {Titulo: "Al Hilal", Top: false, Icon: "uefa.png"},
	},
}

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
	// Intenta obtener el título del label
	competitionLabel := details.Find("label").First()
	if title, exists := competitionLabel.Attr("title"); exists && title != "" {
		return cleanTextForTabsNewlines(title)
	}
	// Si no hay título, usa el texto del label
	if text := cleanTextForTabsNewlines(competitionLabel.Text()); text != "" {
		return text
	}
	// Si no hay label, busca en span (como en "Torneo Clausura")
	competitionSpan := details.Find("span").First()
	if title, exists := competitionSpan.Attr("title"); exists && title != "" {
		return cleanTextForTabsNewlines(title)
	}
	// Si no hay título, usa el texto del span
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
	body, err := FetchWebData("https://www.futbolenlatv.es/deporte")
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
