package app

// Tests del timeout por tipo de ruta en /ace/.
//
// El engine retiene los manifests (sobre todo los anidados /ace/m/*.m3u8)
// hasta que aparecen pares P2P. Con un único timeout de 30s para todo, esos
// manifests morían siempre: el player perdía el nivel de medio y la carga se
// cortaba aunque los segmentos que llegaban fluyeran bien.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestTimeoutDeAnidadoEsCorto(t *testing.T) {
	// Los anidados no llegan nunca: retenerlos solo rompe el player.
	if aceTimeoutAnidado >= aceTimeoutSegmento {
		t.Fatalf("el manifest anidado (%v) debe fallar antes que un segmento (%v)",
			aceTimeoutAnidado, aceTimeoutSegmento)
	}
}

func TestAceTimeoutForDistingueManifestDeSegmento(t *testing.T) {
	casos := []struct {
		ruta string
		want time.Duration
	}{
		{"manifest.m3u8", aceTimeoutManifest},
		{"m/c1959a27edb0b94c5005a2dea93b7a70d4312f1c/711b4ac7.m3u8", aceTimeoutAnidado},
		{"/m/711b4ac7.M3U8", aceTimeoutAnidado},
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

// El manifest anidado es el único que debe responder 204 en vez de 502:
// un 502 lo convierte en error fatal de red en HLS.js.
func TestIsAceNestedManifest(t *testing.T) {
	casos := []struct {
		ruta string
		want bool
	}{
		{"m/aa42/711b.m3u8", true},
		{"/m/aa42/711b.M3U8", true},
		{"manifest.m3u8", false},
		{"c/aa42/0.ts", false},
		{"", false},
	}
	for _, c := range casos {
		if got := isAceNestedManifest(c.ruta); got != c.want {
			t.Errorf("isAceNestedManifest(%q) = %v, se esperaba %v", c.ruta, got, c.want)
		}
	}
}

// Un manifest anidado en minúsculas también cuenta: el navegador puede
// pedirlo con cualquier capitalización.
func TestAceTimeoutForIgnoraMayusculas(t *testing.T) {
	if got := aceTimeoutFor("M/ABC.M3U8"); got != aceTimeoutAnidado {
		t.Errorf("un manifest anidado en mayúsculas debería usar el timeout corto, hubo %v", got)
	}
}

// aceDoTimeout debe respetar el timeout que le pasan, no el de segmento por
// defecto: es lo que permite que el manifest sobreviva.
func TestAceDoTimeoutRespetaElTimeoutRecibido(t *testing.T) {
	var visto time.Duration
	_, _, _, err := aceDoTimeout("http://127.0.0.1:1/ace/x", false, 90*time.Second,
		func(u string, c *http.Client) (*http.Response, error) {
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
	aceDo("http://127.0.0.1:1/ace/x", false, func(u string, c *http.Client) (*http.Response, error) {
		visto = c.Timeout
		return nil, errConexionSimulada
	})
	if visto != aceTimeoutSegmento {
		t.Errorf("aceDo usa %v, se esperaba el de segmento %v", visto, aceTimeoutSegmento)
	}
}

// Regresión del bug que tumbaba el arranque de los streams: el engine
// responde 302 en /ace/manifest.m3u8?id=<id> hacia /ace/m/<hash>/<hash>.m3u8
// en ~200ms, y ese destino está retenido esperando pares. Mientras el cliente
// directo seguía el redirect por su cuenta arrastraba el timeout del manifest
// principal (2m), así que la carga moría a los 120s con un 502 que HLS.js
// trata como error fatal de red. Aquí se comprueba que el timeout se recalcula
// para el destino real y que la ruta final llega al llamador.
func TestAceDoTimeoutRecalculaElTimeoutTrasRedirect(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ace/m/") {
			w.Write([]byte("#EXTM3U\n"))
			return
		}
		http.Redirect(w, r, "/ace/m/hash/hash.m3u8", http.StatusFound)
	}))
	defer engine.Close()

	var timeouts []time.Duration
	resp, _, finalURL, err := aceDoTimeout(engine.URL+"/ace/manifest.m3u8?id=abc", false, aceTimeoutManifest,
		func(u string, c *http.Client) (*http.Response, error) {
			timeouts = append(timeouts, c.Timeout)
			req, err := http.NewRequest("GET", u, nil)
			if err != nil {
				return nil, err
			}
			return c.Do(req)
		})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	defer resp.Body.Close()

	if len(timeouts) != 2 {
		t.Fatalf("se esperaban 2 saltos (principal + anidado), hubo %d", len(timeouts))
	}
	if timeouts[0] != aceTimeoutManifest {
		t.Errorf("el salto principal usó %v, se esperaba %v", timeouts[0], aceTimeoutManifest)
	}
	if timeouts[1] != aceTimeoutAnidado {
		t.Errorf("el salto anidado usó %v, se esperaba %v: el redirect debe heredar el timeout corto",
			timeouts[1], aceTimeoutAnidado)
	}
	if !strings.Contains(finalURL, "/ace/m/") {
		t.Errorf("la URL final %q no es la del manifest anidado", finalURL)
	}
}

// Cuando el destino del redirect es el que falla, la ruta devuelta tiene que
// ser la del anidado: es lo que permite responder 204 en vez de 502.
func TestAceDoTimeoutDevuelveLaRutaFinalCuandoFallaElRedirect(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ace/m/hash/hash.m3u8", http.StatusFound)
	}))
	defer engine.Close()

	_, _, finalURL, err := aceDoTimeout(engine.URL+"/ace/manifest.m3u8?id=abc", false, aceTimeoutManifest,
		func(u string, c *http.Client) (*http.Response, error) {
			// El engine retiene el anidado: la petición acaba en timeout.
			if strings.Contains(u, "/ace/m/") {
				return nil, errConexionSimulada
			}
			req, err := http.NewRequest("GET", u, nil)
			if err != nil {
				return nil, err
			}
			return c.Do(req)
		})
	if err == nil {
		t.Fatal("se esperaba error en el engine")
	}
	if !isAceNestedManifest(acePathFromURL(finalURL)) {
		t.Errorf("la ruta final %q no se reconoce como manifest anidado", acePathFromURL(finalURL))
	}
}

// Un bucle de redirects no puede convertirse en una espera indefinida.
func TestAceDoTimeoutCortaLosRedirectsEnBucle(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ace/m/loop.m3u8", http.StatusFound)
	}))
	defer engine.Close()

	llamadas := 0
	resp, _, _, err := aceDoTimeout(engine.URL+"/ace/manifest.m3u8", false, aceTimeoutManifest,
		func(u string, c *http.Client) (*http.Response, error) {
			llamadas++
			req, err := http.NewRequest("GET", u, nil)
			if err != nil {
				return nil, err
			}
			return c.Do(req)
		})
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	resp.Body.Close()
	if llamadas > aceRedirectsMax+1 {
		t.Errorf("hizo %d saltos, el tope es %d", llamadas, aceRedirectsMax+1)
	}
}

func TestAcePathFromURLQuitaElPrefijoAce(t *testing.T) {
	if got := acePathFromURL("http://127.0.0.1:6878/ace/m/abc/def.m3u8"); got != "m/abc/def.m3u8" {
		t.Errorf("acePathFromURL devolvió %q", got)
	}
	if !isAceNestedManifest(acePathFromURL("http://127.0.0.1:6878/ace/m/abc/def.m3u8")) {
		t.Error("una URL /ace/m/ debe clasificarse como manifest anidado")
	}
	if isAceNestedManifest(acePathFromURL("http://127.0.0.1:6878/ace/manifest.m3u8")) {
		t.Error("el manifest principal no es anidado")
	}
}
