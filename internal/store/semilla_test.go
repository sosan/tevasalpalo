package store

// Tests de la base sembrada.
//
// La base existe para que una instalación nueva no arranque con la lista
// vacía, así que lo que hay que fijar es justo eso: que siembra cuando no hay
// nada, que no pisa una base que ya tiene contenido, y que lo sembrado no
// desaparece por el mecanismo normal de bajas, que es lo que lo convertiría en
// una lista vacía otra vez en cuanto pasara el primer refresco.

import (
	"encoding/json"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestLaSemillaEmbecidaTraeCanalesValidos(t *testing.T) {
	doc, err := leerSemilla()
	if err != nil {
		t.Fatalf("la semilla embebida no se pudo leer: %v", err)
	}
	if len(doc.Canales) == 0 {
		t.Fatal("la semilla viene vacía: la instalación nueva arrancaría sin nada")
	}
	conHashes := 0
	for _, c := range doc.Canales {
		if c.Nombre == "" {
			t.Fatalf("un canal sin nombre: %+v", c)
		}
		for _, h := range c.Hashes {
			if !EsHashAceStream(h) {
				t.Fatalf("hash con mal formato en %q: %q", c.Nombre, h)
			}
			conHashes++
		}
	}
	if conHashes == 0 {
		t.Fatal("ningún canal trae hashes: no hay nada que reproducir")
	}
	t.Logf("semilla: %d canales, %d hashes", len(doc.Canales), conHashes)
}

func TestSiembraUnaBaseVacia(t *testing.T) {
	s := storeDePrueba(t)
	Global = s
	if n := len(s.CargarCanales()); n != 0 {
		t.Fatalf("la base de prueba debería estar vacía, tiene %d", n)
	}

	insertados, err := SembrarBaseVacia()
	if err != nil {
		t.Fatalf("sembrar: %v", err)
	}
	if insertados == 0 {
		t.Fatal("no se sembró ningún canal")
	}

	cargados := s.CargarCanales()
	if len(cargados) != insertados {
		t.Errorf("CargarCanales ve %d canales, se sembraron %d", len(cargados), insertados)
	}
	// Todos tienen que verse: es justo lo que hace que la lista no salga vacía.
	for nombre, hashes := range cargados {
		if len(hashes) == 0 {
			t.Errorf("%q quedó sin hashes y no se cargaría", nombre)
			break
		}
	}
}

func TestSiembraNoPisaUnaBaseQueYaTieneCosas(t *testing.T) {
	s := storeDePrueba(t)
	Global = s
	if _, _, err := s.GuardarRefresco(map[string][]string{
		"CANAL PROPIO": {"1111111111111111111111111111111111111111"},
	}, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	insertados, _ := SembrarBaseVacia()
	if insertados != 0 {
		t.Fatalf("sembró %d canales en una base que ya tenía contenido", insertados)
	}
	cargados := s.CargarCanales()
	if len(cargados) != 1 {
		t.Fatalf("la base debería seguir teniendo 1 canal, tiene %d", len(cargados))
	}
	if _, ok := cargados["CANAL PROPIO"]; !ok {
		t.Error("el canal que ya estaba ha desaparecido al sembrar")
	}
}

// El motivo de existir de la base: un refresco que no publica estos canales no
// puede vaciar la lista. Si esto falla, la instalación nueva muestra canales un
// momento y luego se queda a cero.
func TestLoSembradoNoSeRetiraAlRefrescar(t *testing.T) {
	s := storeDePrueba(t)
	Global = s
	sembrados, _ := SembrarBaseVacia()
	if sembrados == 0 {
		t.Fatal("no hay nada sembrado que probar")
	}
	antes := len(s.CargarCanales())

	// Un refresco donde ninguna fuente publica nada de lo sembrado.
	_, bajas, err := s.GuardarRefresco(map[string][]string{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bajas != 0 {
		t.Errorf("se Retirearon %d canales sembrados, no deberían retirarse", bajas)
	}
	if ahora := len(s.CargarCanales()); ahora != antes {
		t.Errorf("la lista pasó de %d a %d canales tras un refresco vacío", antes, ahora)
	}
}

// Cuando una fuente real publica un canal de la base, manda la fuente: sus
// hashes sustituyen a los de la instantánea, pero el canal no pierde el
// respaldo de la base.
func TestUnaFuenteAdoptaUnCanalSembrado(t *testing.T) {
	s := storeDePrueba(t)
	Global = s
	sembrados, _ := SembrarBaseVacia()

	// Se coge un canal sembrado cualquiera.
	var nombre string
	for n := range s.CargarCanales() {
		nombre = n
		break
	}
	if nombre == "" {
		t.Fatal("no hay canales sembrados")
	}

	nuevo := "2222222222222222222222222222222222222222"
	s.GuardarRefresco(map[string][]string{nombre: {nuevo}},
		map[string]map[string][]string{"fuente": {nombre: {nuevo}}}, nil, nil, nil, nil)

	cargados := s.CargarCanales()
	hashes := cargados[nombre]
	if len(hashes) != 1 || hashes[0] != nuevo {
		t.Errorf("la fuente real debería haber sustituido los hashes: %v", hashes)
	}
	reg := s.hashRegistroDe(nombre)
	if !reg.Permanente {
		t.Error("un canal de la base no debe perder el respaldo al aparecer en una fuente")
	}
	if len(reg.Fuentes) == 1 && reg.Fuentes[0] == nombreBaseSrc {
		t.Error("las fuentes reales deberían sustituir a la marca de la base")
	}
	if sembrados == 0 {
		t.Fatal("sin sembrar no se podía probar nada")
	}
}

// hashRegistroDe lee el registro crudo de un canal, que CargarCanales no da.
func (s *Store) hashRegistroDe(nombre string) canalRegistro {
	var reg canalRegistro
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketChannels))
		v := b.Get([]byte(nombre))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &reg)
	})
	return reg
}

// Sembrar dos veces no debe duplicar nada ni generar altas nuevas en el
// histórico: sembrar 2.000 canales no son 2.000 altas.
func TestLaSiembraNoGeneraAltas(t *testing.T) {
	s := storeDePrueba(t)
	Global = s
	SembrarBaseVacia()
	_, err := SembrarBaseVacia()
	if err != nil {
		t.Fatal(err)
	}
	if n := s.ContarEventos(); n != 0 {
		t.Errorf("la siembra dejó %d eventos en el histórico, debería dejar 0", n)
	}
}
