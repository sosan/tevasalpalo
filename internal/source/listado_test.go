package source

import (
	"testing"

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
