package source

import (
	"strings"
	"testing"
	"time"

	"tevasalpalo/internal/store"
)

// CargarListadoDesdeDisco rehídRal el listado de la base con el formato
// "acestream://<hash>" y lo publica para que la web sirva de inmediato.
func TestCargarListadoDesdeDisco(t *testing.T) {
	// Sin base: tranquilo, no publica nada
	if store.Global != nil {
		temp := store.Global
		store.Global = nil
		t.Cleanup(func() { store.Global = temp })
	}
	conListado(t, map[string]BroadcasterInfo{}) // listado vacío como referencia
	CargarListadoDesdeDisco()
	listadoMu.RLock()
	if len(BroadcasterToAcestream) != 0 {
		t.Errorf("sin base publicó algo: %v", BroadcasterToAcestream)
	}
	listadoMu.RUnlock()

	// Con base sembrada y dos canales
	s := abrirStoreDePrueba(t)
	for _, h := range []string{"00000000000000000000000000000011", "00000000000000000000000000000021"} {
		s.RegistrarHashResultado(h, true) // vivos: no se descartan al ordenar
	}
	if _, _, err := s.GuardarRefresco(
		map[string][]string{
			"Canal A": {"00000000000000000000000000000011", "00000000000000000000000000000021"},
		},
		map[string]map[string][]string{"akademo": {"Canal A": nil}},
		map[string]string{"akademo": "ok"},
		map[string]int{"akademo": 1},
		map[string]int64{"akademo": 100},
		map[string]bool{"akademo": false},
	); err != nil {
		t.Fatalf("GuardarRefresco: %v", err)
	}

	CargarListadoDesdeDisco()

	listadoMu.RLock()
	got, ok := BroadcasterToAcestream["Canal A"]
	listadoMu.RUnlock()
	if !ok {
		t.Fatal("canal A no publicado tras cargar del disco")
	}
	// El canal tiene 2 hashes vivos: quedan, en enlaces prefijados
	if len(got.Links) != 2 {
		t.Errorf("links = %v, se esperaban 2 vivos", got.Links)
	}
	for _, l := range got.Links {
		if l != "acestream://00000000000000000000000000000011" && l != "acestream://00000000000000000000000000000021" {
			t.Errorf("link inesperado: %q", l)
		}
	}
}

// ordenarListadoEnVivo no debe tocar los enlaces URI-safe (";<base64>") que
// produce transformUriSafeBroadcasters: pasarlos por OrdenarHashes los pasa
// por ToLower y el base64 distingue mayúsculas, así que al decodificar en el
// player deja de salir la URL original. Solo los nativos se reordenan, y los
// URI-safe conservan su posición exacta.
func TestOrdenarListadoEnVivoPreservaURISafe(t *testing.T) {
	s := abrirStoreDePrueba(t)
	nativoLento := "00000000000000000000000000000011"
	nativoRapido := "00000000000000000000000000000021"
	// El rápido va al frente. El lento se queda por ENCIMA del corte de
	// muerte (0.30): si bajara de ahí, OrdenarHashes lo descartaría y el
	// caso sería "muerto fuera", que ya cubre el otro test.
	s.ActualizarDisponibilidad(nativoLento, 0.30, time.Now())
	s.ActualizarDisponibilidad(nativoRapido, 0.99, time.Now())

	// Base64 con MAYÚSCULAS a propósito: cualquier ToLower por la ruta de
	// OrdenarHashes los rompería y se detectaría aquí.
	const uriSafeA = ";QUJDREVGRw==" // base64("ABCDEFG")
	const uriSafeB = ";SEVMTE8="    // base64("HELLO")

	mapa := map[string]BroadcasterInfo{
		"Mezclado": {Name: "Mezclado", Links: []string{
			"acestream://" + nativoLento,
			uriSafeA,
			"acestream://" + nativoRapido,
			uriSafeB,
		}},
	}
	ordenarListadoEnVivo(mapa)

	got := mapa["Mezclado"].Links
	if len(got) != 4 {
		t.Fatalf("links = %v, se esperaban 4 (nativos reordenados, URI-safe intactos)", got)
	}
	// Nativos compactados al frente, reordenados: el rápido primero.
	if !strings.Contains(got[0], nativoRapido) {
		t.Errorf("primero = %q, se esperaba el nativo con más disponibilidad (%s)", got[0], nativoRapido)
	}
	if !strings.Contains(got[1], nativoLento) {
		t.Errorf("segundo = %q, se esperaba el nativo lento (%s)", got[1], nativoLento)
	}
	// URI-safe detrás de los nativos, EXACTAMENTE como entraron: sin ToLower
	// ni reescritura.
	if got[2] != uriSafeA {
		t.Errorf("URI-safe posición 2 alterado: %q (era %q)", got[2], uriSafeA)
	}
	if got[3] != uriSafeB {
		t.Errorf("URI-safe posición 3 alterado: %q (era %q)", got[3], uriSafeB)
	}
}

// Un nativo muerto se descarta y los URI-safe del mismo canal se conservan
// (y se reensamblan en su posición).
func TestOrdenarListadoEnVivoDescartaMuertoSinTocarURISafe(t *testing.T) {
	s := abrirStoreDePrueba(t)
	muerto := "00000000000000000000000000000031"
	vivo := "00000000000000000000000000000032"
	s.RegistrarEvidencia(muerto, store.EvSegmentoErr)
	s.RegistrarEvidencia(muerto, store.EvSegmentoErr) // -0.9, bajo el corte
	s.RegistrarEvidencia(vivo, store.EvSegmentoOK)

	const uriSafeA = ";QUJDREVGRw=="

	mapa := map[string]BroadcasterInfo{
		"Mixto": {Name: "Mixto", Links: []string{
			"acestream://" + muerto,
			uriSafeA,
			"acestream://" + vivo,
		}},
	}
	ordenarListadoEnVivo(mapa)

	got := mapa["Mixto"].Links
	if len(got) != 2 {
		t.Fatalf("links = %v, se esperaban 2 (muerto fuera, URI-safe dentro)", got)
	}
	if !strings.Contains(got[0], vivo) {
		t.Errorf("primero = %q, se esperaba el nativo vivo (%s)", got[0], vivo)
	}
	if got[1] != uriSafeA {
		t.Errorf("URI-safe alterado: %q (era %q)", got[1], uriSafeA)
	}
}
