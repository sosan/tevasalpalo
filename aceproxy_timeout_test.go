package main

// Tests del timeout por tipo de ruta en /ace/.
//
// El engine retiene los manifests (sobre todo los anidados /ace/m/*.m3u8)
// hasta que aparecen pares P2P. Con un único timeout de 30s para todo, esos
// manifests morían siempre: el player perdía el nivel de medio y la carga se
// cortaba aunque los segmentos que llegaban fluyeran bien.

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

var errConexionSimulada = errors.New("conexión simulada")

func TestTimeoutDeManifestEsMasLargoQueElDeSegmento(t *testing.T) {
	if aceTimeoutManifest <= aceTimeoutSegmento {
		t.Fatalf("el manifest (%v) debería tolerar más que el segmento (%v)",
			aceTimeoutManifest, aceTimeoutSegmento)
	}
}

func TestAceTimeoutForDistingueManifestDeSegmento(t *testing.T) {
	casos := []struct {
		ruta string
		want time.Duration
	}{
		{"manifest.m3u8", aceTimeoutManifest},
		{"m/c1959a27edb0b94c5005a2dea93b7a70d4312f1c/711b4ac7.m3u8", aceTimeoutManifest},
		{"/m/711b4ac7.M3U8", aceTimeoutManifest},
		{"c/c1959a27edb0b94c5005a2dea93b7a70d4312f1c/0.ts", aceTimeoutSegmento},
		{"c/c1959a27/12.ts", aceTimeoutSegmento},
		{"", aceTimeoutSegmento},
	}
	for _, c := range casos {
		if got := aceTimeoutFor(c.ruta); got != c.want {
			t.Errorf("aceTimeoutFor(%q) = %v, se esperaba %v", c.ruta, got, c.want)
		}
	}
}

// Un manifest anidado en minúsculas también cuenta: el navegador puede
// pedirlo con cualquier capitalización.
func TestAceTimeoutForIgnoraMayusculas(t *testing.T) {
	if got := aceTimeoutFor("M/ABC.M3U8"); got != aceTimeoutManifest {
		t.Errorf("un manifest en mayúsculas debería usar el timeout largo, hubo %v", got)
	}
}

// aceDoTimeout debe respetar el timeout que le pasan, no el de segmento por
// defecto: es lo que permite que el manifest sobreviva.
func TestAceDoTimeoutRespetaElTimeoutRecibido(t *testing.T) {
	var visto time.Duration
	_, _, err := aceDoTimeout("http://127.0.0.1:1/ace/x", false, 90*time.Second,
		func(c *http.Client) (*http.Response, error) {
			visto = c.Timeout
			return nil, errConexionSimulada
		})
	if err == nil {
		t.Fatal("se esperaba error de la petición simulada")
	}
	if visto != 90*time.Second {
		t.Errorf("el cliente recibió timeout %v, se esperaba 90s", visto)
	}
}

// La ruta más común: segmentos con timeout corto, para no retener conexiones.
func TestAceDoPorDefectoUsaElTimeoutDeSegmento(t *testing.T) {
	var visto time.Duration
	aceDo("http://127.0.0.1:1/ace/x", false, func(c *http.Client) (*http.Response, error) {
		visto = c.Timeout
		return nil, errConexionSimulada
	})
	if visto != aceTimeoutSegmento {
		t.Errorf("aceDo usa %v, se esperaba el de segmento %v", visto, aceTimeoutSegmento)
	}
}
