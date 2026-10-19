package source

import (
	"fmt"
	"reflect"
	"testing"

	"tevasalpalo/internal/store"
)

// conListado instala un listado COW nuevo para el test y lo restaura al salir,
// para no contaminar a los demás (el original puede ser el de la semilla).
func conListado(t *testing.T, m map[string]BroadcasterInfo) {
	t.Helper()
	listadoMu.RLock()
	original := BroadcasterToAcestream
	listadoMu.RUnlock()
	t.Cleanup(func() { publicarBroadcaster(original) })
	publicarBroadcaster(m)
}

func listadoDePrueba() map[string]BroadcasterInfo {
	return map[string]BroadcasterInfo{
		"DAZN":                  {Name: "DAZN", Links: []string{"a1"}},
		"DAZN 2":                {Name: "DAZN 2", Links: []string{"a2"}},
		"DAZN BALONCESTO 1":     {Name: "DAZN BALONCESTO 1", Links: []string{"dxb1"}},
		"DAZN BALONCESTO 2":     {Name: "DAZN BALONCESTO 2", Links: []string{"dxb2"}},
		"M+ LIGA":               {Name: "M+ LIGA", Links: []string{"l1"}},
		"DS SPORT":              {Name: "DS SPORT", Links: []string{"ds1"}},
		"ESPN ARGENTINA":        {Name: "ESPN ARGENTINA", Links: []string{"ea"}},
		"ESPN ARGENTINA 2":      {Name: "ESPN ARGENTINA 2", Links: []string{"ea2"}},
		"ESPN ARGENTINA 3":      {Name: "ESPN ARGENTINA 3", Links: []string{"ea3"}},
		"ESPN ARGENTINA 4":      {Name: "ESPN ARGENTINA 4", Links: []string{"ea4"}},
		"TNT SPORTS":            {Name: "TNT SPORTS", Links: []string{"ts"}},
		"SKY SPORTS BUNDESLIGA": {Name: "SKY SPORTS BUNDESLIGA", Links: []string{"sb"}},
		"SKY SPORTS LALIGA":     {Name: "SKY SPORTS LALIGA", Links: []string{"sl"}},
		"M+ BALONCESTO":         {Name: "M+ BALONCESTO", Links: []string{"mxb"}},
		"M+ BALONCESTO 2":       {Name: "M+ BALONCESTO 2", Links: []string{"mxb2"}},
		"PARAMOUNT+ UFC":        {Name: "PARAMOUNT+ UFC", Links: []string{"pufc"}},
		"UFC":                   {Name: "UFC", Links: []string{"ufc"}},
		"LALIGA HYPERMOTION":    {Name: "LALIGA HYPERMOTION", Links: []string{"lh1", "lh2"}},
		"LALIGA HYPERMOTION 2":  {Name: "LALIGA HYPERMOTION 2", Links: []string{"lh21", "lh22"}},
		"LALIGA HYPERMOTION 3":  {Name: "LALIGA HYPERMOTION 3", Links: []string{"lh31", "lh32"}},
		"LALIGA HYPERMOTION 4":  {Name: "LALIGA HYPERMOTION 4", Links: []string{"lh41", "lh42"}},
		"LALIGA HYPERMOTION 5":  {Name: "LALIGA HYPERMOTION 5", Links: []string{"lh51", "lh52"}},
	}
}

func TestFindBroadcasterCasos(t *testing.T) {
	conListado(t, listadoDePrueba())

	casos := []struct {
		nombre   string
		comp     string
		deporte  string
		esperado string // vacío = "filtrado" (BroadcasterInfo cero)
	}{
		// Sin maquillar
		{"dazn 2  ", "", "", "DAZN 2"}, // trims + mayúsculas + colapso de espacios
		{"M+ L.", "", "", "M+ LIGA"},   // abreviatura
		{"M+L.", "", "", "M+ LIGA"},
		{"DSports", "", "", "DS SPORT"},
		{"ESPN PREMIUM", "", "", "ESPN ARGENTINA"},
		{"ESPN 4", "", "", "ESPN ARGENTINA 4"},
		{"ESPN 3", "", "", "ESPN ARGENTINA 3"},
		{"ESPN 2", "", "", "ESPN ARGENTINA 2"},
		{"TNT SPORTS PREMIUM", "", "", "TNT SPORTS"},
		{"TNT SPORTS HD", "", "", "TNT SPORTS"},
		// Por competición
		{"SKY SPORTS", "Bundesliga", "", "SKY SPORTS BUNDESLIGA"},
		{"SKY SPORTS", "LaLiga", "", "SKY SPORTS LALIGA"},
		{"SKY SPORTS", "", "", ""}, // sin competición no reescribe: no está en el listado
		// Deporte baloncesto
		{"DAZN", "", "Baloncesto", "DAZN BALONCESTO 1"},
		{"DAZN", "Copa del Rey Baloncesto", "Baloncesto", "DAZN BALONCESTO 1"},
		// UFC/Paramount
		{"PARAMOUNT+ UFC", "", "", "PARAMOUNT+ UFC"},
		{"Paramount Network", "UFC", "", "UFC"}, // alias comp UFC sin estar explicito en nombre
		{"Paramount Network", "LaLiga", "", ""}, // Paramount en fútbol: filtrado
		{"CBS APP", "UFC", "", "UFC"},           // CBS en UFC: mismo pool
		{"UFC PREMIUM", "", "", "UFC"},          // FOX/FIGHT PASS/PREMIUM -> UFC
		// LaLiga TV M1-M4 solo Hypermotion
		{"LaLiga TV M3", "Serie A", "", ""}, // Serie A: BAR sin acestream
		{"LaLiga TV M3", "LaLiga Hypermotion", "", "LALIGA HYPERMOTION"},
		{"LaLiga TV M4", "LaLiga Hypermotion", "", "LALIGA HYPERMOTION 2"},
		{"LaLiga TV M1", "LaLiga Hypermotion", "", "LALIGA HYPERMOTION 3"},
		{"LALIGA TV HYPERMOTION", "", "", "LALIGA HYPERMOTION"},
		{"LALIGA HYPERMOTION 5", "", "", "LALIGA HYPERMOTION 5"},
		{"HYPERMOTION 4", "", "", "LALIGA HYPERMOTION 4"},
		{"HYPERMOTION 5", "", "", "LALIGA HYPERMOTION 5"},
		// Baloncesto M+
		{"M+ BALONCESTO", "", "Baloncesto", "M+ BALONCESTO"},
		{"M+ BALONCESTO 2", "", "", "M+ BALONCESTO 2"},
		{"M+ BALONCESTO 3", "", "", "M+ BALONCESTO 2"}, // no hay M+ 3: fallback documentado
		{"Movistar Baloncesto", "", "", "M+ BALONCESTO"},
		{"DAZN BALONCESTO 2", "", "", "DAZN BALONCESTO 2"},
		// Desconocidos
		{"CANAL QUE NO EXISTE", "", "", ""},
		{"", "", "", ""},
	}

	for _, caso := range casos {
		got := FindBroadcaster(caso.nombre, caso.comp, caso.deporte)
		nombreObtenido := got.Name
		if caso.esperado == "" {
			if !reflect.ValueOf(got).IsZero() {
				t.Errorf("Find(%q, %q, %q) = %q + %v, se esperaba filtro (vacío)", caso.nombre, caso.comp, caso.deporte, got.Name, got.Links)
			}
			continue
		}
		if nombreObtenido != caso.esperado {
			t.Errorf("Find(%q, %q, %q) = %q, se esperaba %q", caso.nombre, caso.comp, caso.deporte, nombreObtenido, caso.esperado)
		}
	}
}

func TestFindBroadcasterAplicaOrdenDelListadoVivo(t *testing.T) {
	// El M+ con links vacía hace fallback a DAZN BALONCESTO (rama del fallback)
	listado := map[string]BroadcasterInfo{
		"M+ BALONCESTO":     {Name: "M+ BALONCESTO", Links: nil}, // sin links
		"DAZN BALONCESTO 1": {Name: "DAZN BALONCESTO 1", Links: []string{"dxb1"}},
	}
	conListado(t, listado)
	got := FindBroadcaster("M+ BALONCESTO", "", "")
	if got.Name != "DAZN BALONCESTO 1" {
		t.Errorf("fallback a DAZN falla, hubo %q (links: %v)", got.Name, got.Links)
	}
}

func TestOrdenarListadoEnVivo(t *testing.T) {
	// Sin base: no explota
	if store.Global != nil {
		temp := store.Global
		store.Global = nil
		t.Cleanup(func() { store.Global = temp })
	}
	mapa := map[string]BroadcasterInfo{"DAZN": {Name: "DAZN", Links: []string{"a", "b"}}}
	ordenarListadoEnVivo(mapa)
	if len(mapa["DAZN"].Links) != 2 {
		t.Errorf("sin base el listado no debería cambiar: %v", mapa["DAZN"].Links)
	}

	// Con base y evidencia: muertos descartados, buenos al principio
	storeDePrueba := abrirStoreDePrueba(t)

	// Un canal con 2 links: bueno y muerto
	hashVivo, hashMuerto := "00000000000000000000000000000001", "00000000000000000000000000000002"
	storeDePrueba.RegistrarHashResultado(hashVivo, true) // segmento OK
	for i := 0; i < 3; i++ {
		storeDePrueba.RegistrarHashResultado(hashMuerto, false) // 3 fallos: muerto
	}

	canales := map[string]BroadcasterInfo{
		"VIVO Y MUERTO": {Name: "VIVO Y MUERTO", Links: []string{hashMuerto, hashVivo}},
		"Solo uno":      {Name: "Solo uno", Links: []string{hashVivo}},
		"Solo muerto":   {Name: "Solo muerto", Links: []string{hashMuerto}}, // solo muertos: NO lo vacía
	}
	ordenarListadoEnVivo(canales)

	got := canales["VIVO Y MUERTO"].Links
	if len(got) != 1 || got[0] != "acestream://"+hashVivo {
		t.Errorf("canal mixto: %v (el muerto se va y el vivo queda)", got)
	}
	if len(canales["Solo uno"].Links) != 1 || canales["Solo uno"].Links[0] != hashVivo {
		t.Errorf("canal de 1: no debería tocarse (queda sin prefijo), hay %v", canales["Solo uno"].Links)
	}
	if len(canales["Solo muerto"].Links) != 1 || canales["Solo muerto"].Links[0] != hashMuerto {
		t.Errorf("canal solo muerto: no debería vaciarse, hay %v", canales["Solo muerto"].Links)
	}
}

// abrirStoreDePrueba abre la base en disco temporal y la deja como Global
// (igual que producción), cerrándola al salir del test.
func abrirStoreDePrueba(t *testing.T) *store.Store {
	t.Helper()
	fmt.Println("nota de test: abriendo base temporal")
	s := store.AbrirStoreEn(t.TempDir() + "/test.db")
	if s == nil {
		t.Fatal("no se pudo abrir la base de pruebas")
	}
	store.Global = s
	t.Cleanup(func() {
		// CerrarStore cierra la Global actual y la pone a nil.
		store.CerrarStore()
	})
	return s
}
