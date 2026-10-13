package main

// Tests de la disponibilidad calculada por nosotros.
//
// La capa entera nació para no depender de que haya una API que diga qué
// enlaces están vivos, y para eso solo sirve si el bucket hashes recibe algo.
// El fallo que la dejó inútil fue de señal, no de fórmula: se anotaba
// únicamente getstream, que HLS.js no pide nunca. Estos tests fijan que cada
// señal llega, que pesa lo que debe y que el tiempo la amortigua.

import (
	"testing"
	"time"
)

func TestEvidenciaDeSegmentoSubeLaDisponibilidad(t *testing.T) {
	s := storeDePrueba(t)
	h := "0000000000000000000000000000000000000031"
	s.registrarEvidencia(h, evSegmentoOK)

	reg := s.hash(h)
	if v, conocida, origen := reg.disponibilidadEfectiva(time.Now()); !conocida || origen != "propia" || v <= 0.9 {
		t.Errorf("tras un segmento ok: v=%v conocida=%v origen=%q", v, conocida, origen)
	}
	if !reg.vivo() {
		t.Error("un enlace que acaba de entregar media no puede estar muerto")
	}
	if reg.SegmentosOK != 1 || reg.FallosSeguros != 0 {
		t.Errorf("contadores: segmentos_ok=%d fallos_seguros=%d", reg.SegmentosOK, reg.FallosSeguros)
	}
}

// Dos fallos de segmento seguidos deben bastar para retirarlo de la lista. Con
// el peso de -0.45 la evidencia queda en -0.9, muy por debajo del corte.
func TestDosFallosDeSegmentoMatanElEnlace(t *testing.T) {
	s := storeDePrueba(t)
	h := "0000000000000000000000000000000000000032"
	s.registrarEvidencia(h, evSegmentoErr)
	if !s.hash(h).vivo() {
		t.Fatal("un solo fallo de segmento no debe matar el enlace")
	}
	s.registrarEvidencia(h, evSegmentoErr)
	if s.hash(h).vivo() {
		t.Error("dos fallos de segmento seguidos deben darlo por muerto")
	}
	if muertos := s.hashesMuertos(); len(muertos) != 1 || muertos[0] != h {
		t.Errorf("hashesMuertos debería listarlo: %v", muertos)
	}
}

// El acierto borra la racha de fallos: un enlace que vuelve a funcionar deja
// de estar en la lista de muertos, sin esperar a que el tiempo la borre.
func TestUnAciertoLimpiaLaRachaDeFallos(t *testing.T) {
	s := storeDePrueba(t)
	h := "0000000000000000000000000000000000000033"
	s.registrarEvidencia(h, evSegmentoErr)
	s.registrarEvidencia(h, evSegmentoErr)
	s.registrarEvidencia(h, evSegmentoOK)

	reg := s.hash(h)
	if reg.FallosSeguros != 0 {
		t.Errorf("tras un acierto fallos_seguros = %d, se esperaba 0", reg.FallosSeguros)
	}
	if !reg.vivo() {
		t.Error("un enlace que volvió a entregar media está vivo")
	}
}

// Un manifest sin segmentos pesa poco a propósito: cuatro seguidos hunden el
// enlace, uno solo no dice nada.
func TestPlaylistVaciaPocaYAcumulada(t *testing.T) {
	s := storeDePrueba(t)
	h := "0000000000000000000000000000000000000034"
	s.registrarEvidencia(h, evPlaylistVacia)
	if !s.hash(h).vivo() {
		t.Error("un único manifest vacío no debe hundir el enlace")
	}
	for i := 0; i < 4; i++ {
		s.registrarEvidencia(h, evPlaylistVacia)
	}
	if s.hash(h).vivo() {
		t.Error("varios manifests vacíos seguidos deberían hundir el enlace")
	}
}

// La evidencia se amortigua con el tiempo: un enlace que funcionó hace un día
// no puede seguir saliendo como el mejor, pero tampoco debe marcarse muerto,
// sino volver a "no se sabe".
func TestEvidenciaSeAmortiguaConElTiempo(t *testing.T) {
	base := time.Now()
	reg := hashRegistro{Hash: "x", Propia: 1, PropiaEn: base}

	v0, ok := reg.evidenciaVigente(base)
	if !ok || v0 < 0.99 {
		t.Fatalf("recién medida debería valer 1, dio %v", v0)
	}
	vUnaVidaMedia, _ := reg.evidenciaVigente(base.Add(vidaMediaEvidencia))
	if vUnaVidaMedia > 0.51 || vUnaVidaMedia < 0.49 {
		t.Errorf("tras una vida media debería quedar a la mitad, dio %v", vUnaVidaMedia)
	}
	vUnDia, _ := reg.evidenciaVigente(base.Add(24 * time.Hour))
	if vUnDia > 0.1 {
		t.Errorf("tras un día debería ser casi cero, dio %v", vUnDia)
	}
	if !reg.vivoEn(base.Add(24 * time.Hour)) {
		t.Error("una señal vieja no debe matar un enlace: solo dejar de saber")
	}
	if p := reg.puntuarEn(base.Add(24 * time.Hour)); p > 0.55 {
		t.Errorf("con la señal apagada la puntuación debe volver a neutra, dio %v", p)
	}
}

// Sin ningún dato el enlace se conserva y no se ordena por él: no se puede
// descartar un stream por no saber nada de él.
func TestHashSinDatosSeConservaYNoSeMandaAdelante(t *testing.T) {
	reg := hashRegistro{Hash: "y"}
	if !reg.vivo() {
		t.Fatal("un hash recién visto debe considerarse vivo")
	}
	if _, conocida, origen := reg.disponibilidadEfectiva(time.Now()); conocida || origen != "" {
		t.Errorf("sin datos no debe declararse conocido: conocida=%v origen=%q", conocida, origen)
	}
	if p := reg.puntuar(); p != 0.5 {
		t.Errorf("la puntuación neutra es 0.5, dio %v", p)
	}
}

// Si llega disponibilidad de la API y también hay señal nuestra, manda la más
// reciente; y si solo hay una de las dos, esa vale.
func TestDisponibilidadEfectivaPrefiereLoMasReciente(t *testing.T) {
	base := time.Now().UTC()
	apiVieja := hashRegistro{Conocida: true, Disponibilidad: 0.9, MedidaEn: base}
	apiVieja.Propia, apiVieja.PropiaEn = -0.9, base.Add(time.Minute)
	if v, _, origen := apiVieja.disponibilidadEfectiva(base); origen != "propia" || v > 0.2 {
		t.Errorf("con la propia más reciente debe mandar la propia: v=%v origen=%q", v, origen)
	}

	apiNueva := hashRegistro{Conocida: true, Disponibilidad: 0.9, MedidaEn: base.Add(time.Minute)}
	apiNueva.Propia, apiNueva.PropiaEn = -0.9, base
	if v, _, origen := apiNueva.disponibilidadEfectiva(base); origen != "api" || v != 0.9 {
		t.Errorf("con la API más reciente debe mandar la API: v=%v origen=%q", v, origen)
	}

	soloAPI := hashRegistro{Conocida: true, Disponibilidad: 0.4}
	if v, conocida, origen := soloAPI.disponibilidadEfectiva(base); !conocida || origen != "api" || v != 0.4 {
		t.Errorf("solo API: v=%v conocida=%v origen=%q", v, conocida, origen)
	}
}

// La API diciendo 0 significa "sin peers ahora" y debe seguir significando
// muerto, como antes: por eso ambas señales comparten escala.
func TestDisponibilidadCeroDeLaAPIMataElEnlace(t *testing.T) {
	reg := hashRegistro{Hash: "z", Conocida: true, Disponibilidad: 0}
	if reg.vivo() {
		t.Error("la API diciendo 0 debe dar el enlace por muerto")
	}
}

// resumenHashes tiene que distinguir "aprendiendo" de "bucket vacío", que fue
// el estado en el que vivía esta capa sin que nada lo delatara.
func TestResumenHashesDistingueLoQueHaAprendido(t *testing.T) {
	s := storeDePrueba(t)
	if r := s.resumenHashes(); r["total"] != 0 {
		t.Errorf("sin datos el resumen debería ir a 0, dio %v", r)
	}

	bueno := "0000000000000000000000000000000000000041"
	malo := "0000000000000000000000000000000000000042"
	s.registrarEvidencia(bueno, evSegmentoOK)
	s.registrarEvidencia(malo, evSegmentoErr)
	s.registrarEvidencia(malo, evSegmentoErr)

	r := s.resumenHashes()
	if r["total"] != 2 {
		t.Errorf("total = %v, se esperaban 2", r["total"])
	}
	if r["con_evidencia"] != 2 || r["solo_propia"] != 2 {
		t.Errorf("con_evidencia=%v solo_propia=%v, se esperaban 2 y 2", r["con_evidencia"], r["solo_propia"])
	}
	if r["muertos"] != 1 || r["vivos"] != 1 {
		t.Errorf("vivos=%v muertos=%v, se esperaba 1 y 1", r["vivos"], r["muertos"])
	}
	if r["segmentos_ok"] != 1 {
		t.Errorf("segmentos_ok = %v, se esperaba 1", r["segmentos_ok"])
	}
}

// El detalle por canal es lo que permite entender una puntuación sin tener que
// adivinarla: de dónde sale el número y si hay dato o no.
func TestDetalleDeHashesExplicaLaPuntuacion(t *testing.T) {
	s := storeDePrueba(t)
	h := "0000000000000000000000000000000000000051"
	otro := "0000000000000000000000000000000000000052"
	s.registrarEvidencia(h, evSegmentoOK)

	detalle := s.hashesOrdenadosPara([]string{h, otro})
	if len(detalle) != 2 {
		t.Fatalf("se esperaban 2 entradas, hubo %d", len(detalle))
	}
	primero := detalle[0]
	if primero["hash"] != h || primero["origen"] != "propia" {
		t.Errorf("la entrada con señal debe ser la del hash probado: %v", primero)
	}
	if primero["vivo"] != true || primero["segmentos_ok"] != 1 {
		t.Errorf("detalle incompleto: %v", primero)
	}
	segundo := detalle[1]
	if segundo["origen"] != "" || segundo["con_medida"] != false {
		t.Errorf("un hash sin datos debe decir que no los hay: %v", segundo)
	}
}

// Un enlace recién visto no tiene registro todavía: eso no puede hacer que
// desaparezca de la lista.
func TestOrdenarHashesIgnoraLosQueNoHayEnLaBase(t *testing.T) {
	s := storeDePrueba(t)
	entradas := []string{"0000000000000000000000000000000000000061",
		"0000000000000000000000000000000000000062"}
	salida, descartados := s.ordenarHashesPuntuados(entradas)
	if descartados != 0 || len(salida) != len(entradas) {
		t.Errorf("sin registros no se descarta nada: %v descartados=%d", salida, descartados)
	}
}
