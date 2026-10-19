package web

import (
	"reflect"
	"testing"

	"tevasalpalo/internal/source"
)

func TestBroadcastersOrdenados(t *testing.T) {
	casos := []struct {
		nombre            string
		entrada           map[string]source.BroadcasterInfo
		nombresEsperados  []string
		compruebaSinOrden bool
	}{
		{
			nombre: "solo los que muestran canales, por Order",
			entrada: map[string]source.BroadcasterInfo{
				"B":      {Name: "B", ShowListChannels: true, Order: 2},
				"A":      {Name: "A", ShowListChannels: true, Order: 1},
				"Oculto": {Name: "Oculto", ShowListChannels: false, Order: 0},
			},
			nombresEsperados: []string{"A", "B"},
		},
		{
			// Los Order 0 (sin orden definido van) al final
			nombre: "Order 0 al final",
			entrada: map[string]source.BroadcasterInfo{
				"Zero":    {Name: "Zero", ShowListChannels: true},
				"Primero": {Name: "Primero", ShowListChannels: true, Order: 1},
			},
			nombresEsperados: []string{"Primero", "Zero"},
		},
		{
			nombre:           "mapa vacío",
			entrada:          map[string]source.BroadcasterInfo{},
			nombresEsperados: []string{},
		},
		{
			nombre: "todo oculto",
			entrada: map[string]source.BroadcasterInfo{
				"a": {Name: "a"},
				"b": {Name: "b"},
			},
			nombresEsperados: []string{},
		},
	}
	for _, caso := range casos {
		out := broadcastersOrdenados(caso.entrada)
		got := make([]string, 0, len(out))
		for _, b := range out {
			got = append(got, b.Name)
		}
		if !reflect.DeepEqual(got, caso.nombresEsperados) {
			t.Errorf("%s: nombres = %v, se esperaban %v", caso.nombre, got, caso.nombresEsperados)
		}
	}
}
