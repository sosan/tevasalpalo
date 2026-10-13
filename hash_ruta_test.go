package main

// Tests de la extracción del infohash en las rutas /ace/*.
//
// Es el punto que decide si la base aprende algo: si el hash no se reconoce,
// la señal se pierde y el bucket hashes se queda vacío para siempre, que es lo
// que pasaba antes de que existiera hashDeRutaAce.

import "testing"

func TestHashDeRutaAceSacaElHashDeCadaForma(t *testing.T) {
	const h = "540df8fcfb0da23419f22e5a1153bc5503fc1431"
	casos := []struct {
		nombre string
		path   string
		query  string
		final  string
		want   string
	}{
		{"manifest anidado", "m/" + h + "/1b53.m3u8", "", "", h},
		{"segmento", "c/" + h + "/0.ts", "", "", h},
		{"datos h", "h/" + h + "/index", "", "", h},
		{"manifest principal", "manifest.m3u8", "id=" + h + "&pid=2", "", h},
		{"manifest que redirige", "manifest.m3u8", "", "m/" + h + "/1b53.m3u8", h},
		{"segmento con query", "c/" + h + "/3.ts", "pid=2", "", h},
		{"sin hash", "manifest.m3u8", "", "", ""},
		{"no es una ruta de stream", "server/load_all", "", "", ""},
		{"hash corrupto", "m/no-es-un-hash/x.m3u8", "", "", ""},
	}
	for _, c := range casos {
		if got := hashDeRutaAce(c.path, c.query, true, c.final); got != c.want {
			t.Errorf("%s: hashDeRutaAce(%q, %q, final=%q) = %q, se esperaba %q",
				c.nombre, c.path, c.query, c.final, got, c.want)
		}
	}
}

// El manifest principal lleva el hash en la query, así que se reconoce sin
// ayuda del redirect; el redirect solo hace falta cuando la ruta no lo trae.
func TestHashDeRutaAceSinRedirect(t *testing.T) {
	const h = "540df8fcfb0da23419f22e5a1153bc5503fc1431"
	if got := hashDeRutaAce("manifest.m3u8", "id="+h, false, ""); got != h {
		t.Errorf("el hash va en la query del manifest principal: %q", got)
	}
	// Y sin query ni ruta final no hay que inventarse nada.
	if got := hashDeRutaAce("manifest.m3u8", "", false, ""); got != "" {
		t.Errorf("sin query ni redirect no debe salir un hash: %q", got)
	}
}

// hashDePeticionAce sigue reservando los manifests, porque para anotar fallos
// hay que distinguir "no responde" de "no está". Un manifest retenido no es un
// fallo, así que no puede entrar por esa vía.
func TestHashDePeticionAceSigueIgnorandoLosManifests(t *testing.T) {
	const h = "540df8fcfb0da23419f22e5a1153bc5503fc1431"
	if got := hashDePeticionAce("manifest.m3u8", "id="+h); got != "" {
		t.Errorf("un manifest no debe anotar fallos, dio %q", got)
	}
	if got := hashDePeticionAce("m/"+h+"/x.m3u8", ""); got != "" {
		t.Errorf("un manifest anidado tampoco, dio %q", got)
	}
	if got := hashDePeticionAce("getstream", "id="+h); got != h {
		t.Errorf("getstream sí debe devolver el hash, dio %q", got)
	}
}
