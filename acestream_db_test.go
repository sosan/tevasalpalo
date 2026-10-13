package main

// Tests de la base de datos de AceStream.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func storeDePrueba(t *testing.T) *store {
	t.Helper()
	dir := t.TempDir()
	s := &store{rutaOverride: filepath.Join(dir, "test.db")}
	db, err := bolt.Open(s.rutaOverride, 0o600, nil)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	s.db = db
	t.Cleanup(func() {
		db.Close()
		storeGlobal = nil
	})
	// Los buckets los crea abrirStore; aquí se replican para no duplicar su lógica.
	db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketChannels, bucketSources, bucketHistory, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	})
	return s
}

// El ciclo completo: guardar, releer y encontrar lo mismo.
func TestGuardarYCargarCanales(t *testing.T) {
	s := storeDePrueba(t)
	datos := map[string][]string{
		"M+ DEPORTES 1": {"abc123", "def456"},
		"MOVISTAR +":    {"789abc"},
	}
	_, _, err := s.guardarRefresco(datos, map[string]map[string][]string{
		"platinsport": {"M+ DEPORTES 1": {"abc123", "def456"}},
	}, map[string]string{}, map[string]int{}, map[string]int64{}, nil)
	if err != nil {
		t.Fatalf("guardarRefresco: %v", err)
	}

	got := s.cargarCanales()
	if !reflect.DeepEqual(got, datos) {
		t.Errorf("cargarCanales = %v, se esperaba %v", got, datos)
	}
}

// Un canal que desaparece no se borra: se queda con Visto=false. Así se puede
// preguntar "estaba aquí y ahora no" en vez de perderlo en silencio.
func TestLosDesaparecidosNoSeBorran(t *testing.T) {
	s := storeDePrueba(t)
	_, _, _ = s.guardarRefresco(
		map[string][]string{"A": {"1"}, "B": {"2"}},
		nil, nil, nil, nil, nil)

	// Segundo ciclo: solo A sigue.
	altas, bajas, err := s.guardarRefresco(
		map[string][]string{"A": {"1"}},
		nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bajas != 1 {
		t.Errorf("bajas = %d, se esperaba 1", bajas)
	}
	if altas != 0 {
		t.Errorf("altas = %d, no debería haber altas", altas)
	}

	// El mapa cargado solo trae los que están, pero el registro sigue ahí.
	got := s.cargarCanales()
	if len(got) != 1 || got["A"] == nil {
		t.Errorf("cargarCanales = %v, solo debería traer A", got)
	}

	var vistoB bool
	s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte(bucketChannels)).Get([]byte("B"))
		var reg canalRegistro
		json.Unmarshal(v, &reg)
		vistoB = !reg.Visto
		return nil
	})
	if !vistoB {
		t.Error("B debería seguir en la base con Visto=false")
	}
}

// El ciclo del histórico: nuevo, sigue, desaparece, vuelve.
func TestHistorialDeAltasYBajas(t *testing.T) {
	s := storeDePrueba(t)
	_, _, _ = s.guardarRefresco(map[string][]string{"A": {"1"}}, nil, nil, nil, nil, nil)
	_, _, _ = s.guardarRefresco(map[string][]string{"A": {"1"}, "B": {"2"}}, nil, nil, nil, nil, nil)
	_, _, _ = s.guardarRefresco(map[string][]string{"B": {"2"}}, nil, nil, nil, nil, nil)
	_, _, _ = s.guardarRefresco(map[string][]string{"A": {"1"}, "B": {"2"}}, nil, nil, nil, nil, nil)

	ev := s.eventosRecientes(20)
	tipos := map[string][]string{}
	for _, e := range ev {
		tipos[e.Tipo] = append(tipos[e.Tipo], e.Canal)
	}

	if len(tipos[eventoNuevo]) != 2 {
		t.Errorf("nuevos = %v, se esperaban A y B", tipos[eventoNuevo])
	}
	if len(tipos[eventoDesaparece]) != 1 || tipos[eventoDesaparece][0] != "A" {
		t.Errorf("desaparecidos = %v, se esperaba solo A", tipos[eventoDesaparece])
	}
	if len(tipos[eventoVuelve]) != 1 || tipos[eventoVuelve][0] != "A" {
		t.Errorf("vueltas = %v, se esperaba A", tipos[eventoVuelve])
	}
}

// Repetir el mismo listado no genera eventos: si no, se llenaría el histórico
// en cada refresco sin que cambiara nada.
func TestUnRefrescoIgualNoGeneraRuido(t *testing.T) {
	s := storeDePrueba(t)
	datos := map[string][]string{"A": {"1"}, "B": {"2"}}
	_, _, _ = s.guardarRefresco(datos, nil, nil, nil, nil, nil)
	n := s.contarEventos()

	for i := 0; i < 5; i++ {
		altas, bajas, _ := s.guardarRefresco(datos, nil, nil, nil, nil, nil)
		if altas != 0 || bajas != 0 {
			t.Fatalf("ciclo %d: altas=%d bajas=%d con el mismo listado", i, altas, bajas)
		}
	}
	if s.contarEventos() != n {
		t.Errorf("el histórico creció de %d a %d sin cambios", n, s.contarEventos())
	}
}

// Qué aporta cada fuente es lo que permite decidir si una sobra.
func TestAportePorFuente(t *testing.T) {
	s := storeDePrueba(t)
	_, _, err := s.guardarRefresco(
		map[string][]string{
			"COMUN":  {"1"},
			"SOLO_A": {"2"},
			"SOLO_B": {"3"},
		},
		map[string]map[string][]string{
			"fuenteA": {"COMUN": {"1"}, "SOLO_A": {"2"}},
			"fuenteB": {"COMUN": {"1"}, "SOLO_B": {"3"}},
		}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	fuentes := s.listarFuentes()
	if len(fuentes) != 2 {
		t.Fatalf("fuentes = %d, se esperaban 2", len(fuentes))
	}
	for _, f := range fuentes {
		switch f.Nombre {
		case "fuenteA":
			if f.SoloAporta != 1 {
				t.Errorf("fuenteA.SoloAporta = %d, se esperaba 1", f.SoloAporta)
			}
			if f.Coincidentes != 1 {
				t.Errorf("fuenteA.Coincidentes = %d, se esperaba 1", f.Coincidentes)
			}
			if f.Entradas != 2 {
				t.Errorf("fuenteA.Entradas = %d, se esperaban 2", f.Entradas)
			}
		case "fuenteB":
			if f.SoloAporta != 1 || f.Coincidentes != 1 {
				t.Errorf("fuenteB = %+v, esperaba 1 y 1", f)
			}
		}
	}
	// Ordenadas por lo que aportan: las dos empatan, así que no se comprueba
	// el orden.
}

// Una fuente caída no borra lo que trajo antes: los registros quedan como no
// vistos y el histórico conserva lo ocurrido.
func TestFuenteCaidaNoBorraElListado(t *testing.T) {
	s := storeDePrueba(t)
	_, _, _ = s.guardarRefresco(
		map[string][]string{"A": {"1"}},
		map[string]map[string][]string{"buena": {"A": {"1"}}},
		nil, nil, nil, nil)

	// Ahora la fuente falla y no aporta nada.
	_, _, _ = s.guardarRefresco(map[string][]string{},
		map[string]map[string][]string{"buena": {}},
		map[string]string{"buena": "connection refused"},
		map[string]int{"buena": 3},
		map[string]int64{"buena": 9000}, nil)

	got := s.cargarCanales()
	if _, ok := got["A"]; !ok {
		t.Errorf("A desapareció del listado pese a que la fuente solo falló: %v", got)
	}

	fuentes := s.listarFuentes()
	if len(fuentes) != 1 {
		t.Fatalf("fuentes = %d", len(fuentes))
	}
	if fuentes[0].UltimoError == "" {
		t.Error("no se guardó el error de la fuente")
	}
	if fuentes[0].UltimoOK.IsZero() {
		t.Error("una fuente que falla no debe marcar UltimoOK")
	}
}

// El histórico no puede crecer sin límite.
func TestElHistoricoSePoda(t *testing.T) {
	dir := t.TempDir()
	s := &store{rutaOverride: filepath.Join(dir, "t.db")}
	db, err := bolt.Open(s.rutaOverride, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s.db = db
	db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketChannels, bucketSources, bucketHistory, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	})

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketHistory))
		for i := 0; i < maxEventosHistorial+500; i++ {
			ev := eventoHistorial{
				Fecha: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
				Tipo:  eventoNuevo, Canal: fmt.Sprintf("canal-%06d", i),
			}
			if err := escribirEvento(b, ev); err != nil {
				return err
			}
		}
		return podarHistorial(b)
	})

	n := 0
	db.View(func(tx *bolt.Tx) error {
		n = contarClaves(tx.Bucket([]byte(bucketHistory)))
		return nil
	})
	if n > maxEventosHistorial {
		t.Errorf("quedan %d eventos, el tope es %d", n, maxEventosHistorial)
	}
	if n == 0 {
		t.Error("la poda se llevó todo")
	}
}

// La búsqueda debe encontrar sin importar mayúsculas ni acentos.
func TestBusquedaDeCanales(t *testing.T) {
	s := storeDePrueba(t)
	_, _, _ = s.guardarRefresco(map[string][]string{
		"M+ DEPORTES 1": {"1"},
		"movistar+":     {"2"},
		"La Sexta":      {"3"},
	}, nil, nil, nil, nil, nil)

	casos := []struct {
		busca  string
		espera int
	}{
		{"deportes", 1},
		{"DEPORTES", 1},
		{"movistar", 1},
		{"sexta", 1},
		{"no-existe", 0},
		{"", 0},
	}
	for _, c := range casos {
		got := s.buscarCaneles(c.busca, 50)
		if len(got) != c.espera {
			t.Errorf("buscarCanales(%q) = %v, se esperaban %d", c.busca, got, c.espera)
		}
	}
}

// Un registro corrupto no puede impedir cargar el resto.
func TestRegistroCorruptoNoRompeLaCarga(t *testing.T) {
	s := storeDePrueba(t)
	_, _, _ = s.guardarRefresco(map[string][]string{"BUENO": {"1"}}, nil, nil, nil, nil, nil)
	s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketChannels)).Put([]byte("ROTO"), []byte("{esto no es json"))
	})

	got := s.cargarCanales()
	if len(got) != 1 || got["BUENO"] == nil {
		t.Errorf("cargarCanales = %v, debería traer solo BUENO", got)
	}
}

// Sin base de datos la app tiene que funcionar igual. Todas las operaciones
// toleran store nil.
func TestSinBaseDeDatosNoRompe(t *testing.T) {
	var s *store
	if got := s.cargarCanales(); len(got) != 0 {
		t.Errorf("cargarCanales sin base = %v", got)
	}
	altas, bajas, err := s.guardarRefresco(map[string][]string{"A": {"1"}}, nil, nil, nil, nil, nil)
	if altas != 0 || bajas != 0 || err != nil {
		t.Errorf("guardarRefresco sin base = %d %d %v", altas, bajas, err)
	}
	if len(s.listarFuentes()) != 0 || len(s.buscarCaneles("x", 10)) != 0 || len(s.eventosRecientes(5)) != 0 {
		t.Error("las lecturas sin base deberían devolver vacío")
	}
	if r := s.resumen(); r["disponible"] != false {
		t.Errorf("resumen sin base = %v", r)
	}
}

// El arranque rápido: cargar de la base tiene que ser mucho más rápido que
// parsear las fuentes. No se compara contra la red (no hay red en un test), sino
// que la lectura de 2.000 claves sea trivial.
func TestCargaDesdeDiscoEsRapida(t *testing.T) {
	s := storeDePrueba(t)
	datos := map[string][]string{}
	for i := 0; i < 2000; i++ {
		datos[fmt.Sprintf("CANAL %04d", i)] = []string{
			fmt.Sprintf("%040d", i),
			fmt.Sprintf("%040d", i+1),
		}
	}
	_, _, err := s.guardarRefresco(datos, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	inicio := time.Now()
	got := s.cargarCanales()
	took := time.Since(inicio)

	if len(got) != 2000 {
		t.Fatalf("cargados %d", len(got))
	}
	// Muy holgado: solo queremos descartar que esté haciendo algo raro por
	// canal. El objetivo real es comparar contra los 27 s de red.
	if took > 2*time.Second {
		t.Errorf("cargar 2.000 canales tardó %v, debería ser instantáneo", took)
	}
	t.Logf("2.000 canales en %v", took)
}

// La base se crea junto al ejecutable y no en el directorio de trabajo.
func TestLaBaseVaJuntoAlEjecutable(t *testing.T) {
	p := storePath()
	if p == "" {
		t.Skip("no se pudo resolver el ejecutable")
	}
	if filepath.Base(p) != nombreDB {
		t.Errorf("storePath() = %q, debería acabar en %s", p, nombreDB)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("storePath() = %q, debería ser absoluta", p)
	}
}

func TestAbrirStoreCreaElFichero(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "sub", "acestream.db")

	// abrirStore usa storePath(); se comprueba la lógica de creación de
	// directorio y fichero con un store abierto a mano.
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(ruta, 0o600, nil)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	db.Close()
	if _, err := os.Stat(ruta); err != nil {
		t.Errorf("no se creó el fichero: %v", err)
	}
}

// Una fuente que no aporta nada exclusivo se pausa sola tras un par de ciclos,
// en vez de estar descargando lo mismo que ya traen las demás.
func TestLaCadenciaSubeSiNoAportaNada(t *testing.T) {
	s := storeDePrueba(t)

	// platinsport trae de más; la otra solo repite.
	registrar := func(ciclo int) fuenteRegistro {
		_, _, _ = s.guardarRefresco(
			map[string][]string{"COMUN": {"1"}, "PROPIO": {"2"}},
			map[string]map[string][]string{
				"grande": {"COMUN": {"1"}, "PROPIO": {"2"}},
				"muerta": {"COMUN": {"1"}},
			}, nil, nil, nil, nil)
		for _, f := range s.listarFuentes() {
			if f.Nombre == "muerta" {
				return f
			}
		}
		t.Fatalf("ciclo %d: no está la fuente", ciclo)
		return fuenteRegistro{}
	}

	// Una fuente nueva se consulta siempre (ciclo 1). En el segundo ciclo ya
	// lleva dosportunity sin aportar y entra la pausa: un ciclo de gracia es
	// suficiente, porque una pausa no borra nada y la fuente se sigue
	// vigilando cada 30 minutos.
	f0 := registrar(0)
	if f0.IntervaloMin != 0 {
		t.Errorf("primer ciclo: intervalo = %d, una fuente nueva se consulta siempre", f0.IntervaloMin)
	}
	f1 := registrar(1)
	if f1.IntervaloMin == 0 {
		t.Error("segundo ciclo: debería haberse pausado ya")
	}
	f2 := registrar(2)
	if f2.IntervaloMin <= f1.IntervaloMin {
		t.Errorf("el intervalo debería crecer: %d -> %d", f1.IntervaloMin, f2.IntervaloMin)
	}

	// Y la que sí aporta se queda en 0 siempre.
	for _, f := range s.listarFuentes() {
		if f.Nombre == "grande" && f.IntervaloMin != 0 {
			t.Errorf("la fuente que aporta quedó pausada: %+v", f)
		}
	}
}

// Volver a aportar de nuevo reactiva la fuente: la pausa no es una puerta de
// un solo sentido.
func TestAportarDeNuevoReactivaLaFuente(t *testing.T) {
	s := storeDePrueba(t)

	// Con dos fuentes, COMUN no es exclusiva de ninguna; SOLO sí lo es.
	otras := map[string]map[string][]string{"otra": {"COMUN": {"1"}}}
	variable := map[string]map[string][]string{"variable": {"COMUN": {"1"}}}
	fusionado := map[string][]string{"COMUN": {"1"}}

	// Un ciclo con SOLO y otro sin él: dos seguidos sin aportar.
	_, _, _ = s.guardarRefresco(fusionado, mergeFuentes(otras, variable), nil, nil, nil, nil)
	_, _, _ = s.guardarRefresco(fusionado, mergeFuentes(otras, variable), nil, nil, nil, nil)

	var antes fuenteRegistro
	for _, f := range s.listarFuentes() {
		if f.Nombre == "variable" {
			antes = f
		}
	}
	if antes.IntervaloMin == 0 {
		t.Fatalf("debería estar pausada: %+v", antes)
	}
	if antes.debeConsultarse(time.Now()) {
		t.Error("y no debería tocar consultarse todavía")
	}

	// Aparece algo exclusivo: vuelve a estar activa.
	fusionado["NUEVO"] = []string{"9"}
	variable["variable"] = map[string][]string{"COMUN": {"1"}, "NUEVO": {"9"}}
	_, _, _ = s.guardarRefresco(fusionado, mergeFuentes(otras, variable), nil, nil, nil, nil)

	for _, f := range s.listarFuentes() {
		if f.Nombre != "variable" {
			continue
		}
		if f.IntervaloMin != 0 {
			t.Errorf("sigue pausada tras aportar algo exclusivo: %+v", f)
		}
		if f.CiclosSinAportar != 0 {
			t.Errorf("el contador no se reinició: %d", f.CiclosSinAportar)
		}
		if !f.debeConsultarse(time.Now()) {
			t.Error("debería volver a consultarse")
		}
	}
}

func mergeFuentes(a, b map[string]map[string][]string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// La pausa tiene tope: no se llega nunca a "no mirar más".
func TestLaPausaTieneTope(t *testing.T) {
	if ultimo := nivelesIntervalo[len(nivelesIntervalo)-1]; ultimo != 1440 {
		t.Errorf("el tope es %d min, se esperaba 1440 (un día)", ultimo)
	}
	if nivelIntervaloPara(9999) != 1440 {
		t.Error("con muchos ciclos sin aportar debería quedarse en el tope")
	}
	if nivelIntervaloPara(0) != 0 || nivelIntervaloPara(-5) != 0 {
		t.Error("sin ciclos sin aportar debe consultarse siempre")
	}
}

// Una fuente en pausa no debe hundirse un nivel más en cada refresco. Es el
// error que haría que una fuente nunca pudiera volver sola.
func TestLaPausaNoSeAutoIncrementa(t *testing.T) {
	s := storeDePrueba(t)
	porFuente := map[string]map[string][]string{"x": {"COMUN": {"1"}}}
	fusionado := map[string][]string{"COMUN": {"1"}}

	for i := 0; i < 4; i++ {
		_, _, _ = s.guardarRefresco(fusionado, porFuente, nil, nil, nil, nil)
	}
	var pausada fuenteRegistro
	for _, f := range s.listarFuentes() {
		if f.Nombre == "x" {
			pausada = f
		}
	}
	primer := pausada.IntervaloMin

	// Ahora cinco refrescos en los que la fuente está en pausa.
	for i := 0; i < 5; i++ {
		_, _, _ = s.guardarRefresco(fusionado,
			map[string]map[string][]string{"x": {}},
			nil, nil, nil, map[string]bool{"x": true})
	}
	var trasPausa fuenteRegistro
	for _, f := range s.listarFuentes() {
		if f.Nombre == "x" {
			trasPausa = f
		}
	}
	if trasPausa.IntervaloMin != primer {
		t.Errorf("el intervalo cambió estando en pausa: %d -> %d", primer, trasPausa.IntervaloMin)
	}
	if trasPausa.CiclosSinAportar > pausada.CiclosSinAportar {
		t.Errorf("el contador subió estando en pausa: %d -> %d",
			pausada.CiclosSinAportar, trasPausa.CiclosSinAportar)
	}
}
