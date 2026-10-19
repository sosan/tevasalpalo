package source

import (
	"strings"
	"testing"
)

func TestCleanChannelNameFutbolEnCasa(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"DAZN 2 : Ver Partido", "DAZN 2"},           // sufijo VER PARTIDO
		{" Movistar Plus  (M1 O2)", "Movistar Plus"}, // sufijo (M1 O2)
		{"DAZN LaLiga (Ver en Directo)", "DAZN LaLiga"},
		{"Orange TV 2 (2)", "Orange TV 2"},
		{"  Canal limpio  ", "Canal limpio"},
		{"", ""},
		// combinados: cadenas reales que traen admin(simultáneos sufijos)
		{"DAZN 3 : VER PARTIDO (3)", "DAZN 3"},
	}
	for _, caso := range casos {
		if got := cleanChannelNameFutbolEnCasa(caso.entrada); got != caso.esperado {
			t.Errorf("clean(%q) = %q, se esperaba %q", caso.entrada, got, caso.esperado)
		}
	}
}

func TestIsRelevant(t *testing.T) {
	// Con espacios desiguales y mayúsculas/acentos: normaliza sin espacios
	relevantes := []string{"LaligaEASports", "laliga ea sports", "Champions League", "COPA DEL REY", "SuperCopa de España"}
	for _, r := range relevantes {
		if !isRelevant(r) {
			t.Errorf("isRelevant(%q) = false", r)
		}
	}
	descartes := []string{"Liga Endesa", "NBA", "Copa Sudamericana"}
	for _, d := range descartes {
		if isRelevant(d) {
			t.Errorf("isRelevant(%q) = true y no debería", d)
		}
	}
}

func TestNormalizeSportName(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"Baloncesto", "baloncesto"},
		{"FÚTB OL", "futb ol"}, // el mapa de tildes va rune a rune; los espacios internos se conservan
		{"  TenIs  ", "tenis"},
		{"Balonmáscon espacios!!", "balonmascon espacios"}, // puntuación fuera
		{"", ""},
	}
	for _, caso := range casos {
		if got := normalizeSportName(caso.entrada); got != caso.esperado {
			t.Errorf("normaliza(%q) = %q, se esperaba %q", caso.entrada, got, caso.esperado)
		}
	}
}

func TestGetSportInfo(t *testing.T) {
	if icon, sport := GetSportInfo("desconocido"); icon != "icon-deporte" || sport != "Deporte" {
		t.Errorf("desconocido: %q, %q", icon, sport)
	}
	if icon, sport := GetSportInfo("Baloncesto"); icon != "icon-baloncesto" || sport != "Baloncesto" {
		t.Errorf("baloncesto: %q, %q", icon, sport)
	}
}

func TestFormatDateDMYToSpanish(t *testing.T) {
	// 1-1-2026 es jueves
	got, err := FormatDateDMYToSpanish("01-01-2026")
	if err != nil || got != "Jueves, 1 de Enero de 2026" {
		t.Errorf("01-01-2026 = %q, %v", got, err)
	}
	// 6-9-2026 es domingo, sin ceros a la izquierda
	got, err = FormatDateDMYToSpanish("6-9-2026")
	if err != nil || got != "Domingo, 6 de Septiembre de 2026" {
		t.Errorf("6-9-2026 = %q, %v", got, err)
	}
	// Con espacios alrededor
	if _, err := FormatDateDMYToSpanish(" 6-9-2026 "); err != nil {
		t.Errorf("con espacios: %v", err)
	}
	// Mal formada
	if _, err := FormatDateDMYToSpanish("32-13-2026"); err == nil {
		t.Error("32-13-2026 no debería parsear")
	}
	if _, err := FormatDateDMYToSpanish(""); err == nil {
		t.Error("vacia no debería parsear")
	}
}

func TestNormTeamName(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"Real Madrid CF", "MADRID"},
		{"CÁDIZ CF", "CADIZ"},
		{"Club Atlético Osasuna", "ATLETICO OSASUNA"},
		{"  FC   Barcelona  ", "BARCELONA"},
		{"Manchester City", "MANCHESTER CITY"}, // NO se recorta City: distingue de United
		{"Manchester United", "MANCHESTER UNITED"},
	}

	for _, caso := range casos {
		if got := normTeamName(caso.entrada); got != caso.esperado {
			t.Errorf("normTeamName(%q) = %q, se esperaba %q", caso.entrada, got, caso.esperado)
		}
	}
}

func TestSplitMatchTeams(t *testing.T) {
	if teams, ok := splitMatchTeams("Valencia CF - FC Barcelona"); !ok || teams[0] != "Valencia CF" || teams[1] != "FC Barcelona" {
		t.Errorf("par normal: %v, ok=%v", teams, ok)
	}
	if _, ok := splitMatchTeams("Valencia-FC Barcelona"); ok {
		t.Error("sin espacios alrededor del guion no debe partir")
	}
	if _, ok := splitMatchTeams("Solo equipo"); ok {
		t.Error("sin guion no debe partir")
	}
	if _, ok := splitMatchTeams("A - B - C"); ok {
		t.Error("tres partes no es un par")
	}
	if _, ok := splitMatchTeams(" - B"); ok {
		t.Error("equipo vacío no debe partir")
	}
}

func TestAddCompetitionFusionaYDeduplica(t *testing.T) {
	general := DayView{
		DateKey: "d1",
		Competitions: map[string][]MatchView{
			"LaLiga": {
				{Match: Match{Event: "Valencia - Barcelona", Broadcasters: []BroadcasterInfo{{Name: "DAZN", Links: []string{"a"}}}}},
			},
		},
	}
	otro := []DayView{
		{
			DateKey: "d1",
			Competitions: map[string][]MatchView{
				"LaLiga": {
					// mismo partido con afijos distintos: se fusiona, broadcaster se dedup
					{Match: Match{Event: "Valencia CF - FC Barcelona", Broadcasters: []BroadcasterInfo{{Name: "dazn", Links: []string{"a", "b"}}, {Name: "MOVISTAR+", Links: []string{"m"}}}}},
					// evento nuevo solo en esta fuente: se añade
					{Match: Match{Event: "Betis - Sevilla", Broadcasters: []BroadcasterInfo{{Name: "M+ LIGA", Links: []string{"x"}}}}},
				},
			},
		},
	}

	fusionado := addCompetition(general, otro)
	comp := fusionado.Competitions["LaLiga"]
	if len(comp) != 2 {
		t.Fatalf("eventos tras merge = %d, se esperaban 2 (merged + nuevo)", len(comp))
	}
	for _, mv := range comp {
		switch mv.Match.Event {
		case "Valencia - Barcelona":
			if len(mv.Match.Broadcasters) != 2 { // DAZN dedup + MOVISTAR+
				t.Errorf("broadcasters tras dedup = %v, se esperaban 2", mv.Match.Broadcasters)
			}
			for _, b := range mv.Match.Broadcasters {
				if strings.EqualFold(b.Name, "DAZN") && len(b.Links) != 2 {
					t.Errorf("links dedup de DAZN = %v, se esperaban {a,b}", b.Links)
				}
			}
		case "Betis - Sevilla":
			if len(mv.Match.Broadcasters) != 1 || mv.Match.Broadcasters[0].Name != "M+ LIGA" {
				t.Errorf("evento nuevo debe conservar sus broadcasters: %v", mv.Match.Broadcasters)
			}
		default:
			t.Errorf("evento inesperado %q", mv.Match.Event)
		}
	}
}

func TestMixCompetitionsCambiaYCae(t *testing.T) {
	// Con listado vivo sembrado (BroadcasterVivo usa el COW)
	conListado(t, map[string]BroadcasterInfo{
		"DAZN CALCIO": {Name: "DAZN CALCIO", Links: []string{"c1"}},
		"SKY SPORTS":  {Name: "SKY SPORTS", Links: []string{"s1"}},
	})

	dias := []DayView{
		{
			DateKey: "d1",
			Competitions: map[string][]MatchView{
				"Serie A Italiana": {{Match: Match{Event: "A - B", Broadcasters: []BroadcasterInfo{{Name: "dazn"}}}}},
				"LaLiga":           {{Match: Match{Event: "C - D", Broadcasters: []BroadcasterInfo{{Name: "otro"}}}}},
			},
		},
	}

	calcio := []DayView{{DateKey: "d1", Competitions: map[string][]MatchView{
		"Liga italiana": {{Match: Match{Event: "A - B", Broadcasters: []BroadcasterInfo{{Name: "dazn"}}}}},
	}}}

	mx := []DayView{{DateKey: "d1", Competitions: map[string][]MatchView{
		"LaLiga Hypermotion": {{Match: Match{Event: "C - D", Broadcasters: []BroadcasterInfo{{Name: "laliga tv hypermotion"}}}}},
	}}}

	out := mixCompetitions(dias, mx, calcio, nil, nil, nil, nil, nil)

	compSerieA := out[0].Competitions["Serie A Italiana"]
	if compSerieA[0].Match.Broadcasters[0].Name != "DAZN CALCIO" {
		t.Errorf("broadcaster dazn en Serie A debería haberse cambiado a DAZN CALCIO, hay %q", compSerieA[0].Match.Broadcasters[0].Name)
	}
	// La competición italiana llega renombrada de "Liga italiana" y sus match renombrados
	if _, ok := out[0].Competitions["Serie A Italiana"]; len(out[0].Competitions) < 2 || !ok {
		t.Errorf("competencias tras mix: %v", keysDe(out[0].Competitions))
	}
}

func keysDe(m map[string][]MatchView) []string {
	claves := make([]string, 0, len(m))
	for k := range m {
		claves = append(claves, k)
	}
	return claves
}

func TestAddNewBroadcasterSoloEnCompetencia(t *testing.T) {
	conListado(t, map[string]BroadcasterInfo{
		"SKY SPORTS": {Name: "SKY SPORTS", Links: []string{"s1"}},
	})
	dias := []DayView{
		{DateKey: "d1", Competitions: map[string][]MatchView{
			"LaLiga": {{Match: Match{Event: "A - B"}}},
			"Bundes": {{Match: Match{Event: "D - E"}}},
		}},
	}
	salida := addNewBroadcaster(dias, "SKY SPORTS", "LaLiga")
	if got := salida[0].Competitions["LaLiga"][0].Match.Broadcasters; len(got) != 1 || got[0].Name != "SKY SPORTS" {
		t.Errorf("broadcaster añadido: %v", got)
	}
	if got := salida[0].Competitions["Bundes"][0].Match.Broadcasters; len(got) != 0 {
		t.Errorf("Bundes no debía tocar: %v", got)
	}
}
