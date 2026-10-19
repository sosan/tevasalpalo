package web

// Tests del enlace que el player pasa a HLS.js y del muestreo de telemetría.
//
// Son dos decisiones que viven en el JS de la plantilla, donde no llega go vet
// ni el compilador: si el enlace se pasa mal, HLS.js lo sube como error fatal
// y solo se ve en la consola del navegador; si el muestreo de telemetría
// descarta el primer error, en Grafana no aparece nada aunque el player lo
// tenga registrado.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func playerHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("views/player.html")
	if err != nil {
		t.Fatalf("leer views/player.html: %v", err)
	}
	return string(b)
}

// El esquema acestream:// solo existe dentro del motor: si se le pasa a HLS.js
// el navegador lo rechaza por CORS y el stream no arranca nunca. Los enlaces
// llegan como ";base64(acestream://<hash>)" desde el listado, así que es
// justo el camino habitual el que tenía que convertir el hash.
func TestPlayerNoPasaAcestreamDirectoAHls(t *testing.T) {
	html := playerHTML(t)
	if !strings.Contains(html, "function hashDeAcestream") {
		t.Error("player.html lost hashDeAcestream: acestream:// URLs would reach HLS.js directly")
	}
	prohibido := regexp.MustCompile(`loadHlsVideo\(\s*(decoded|fromBase64Url)`)
	if m := prohibido.FindString(html); m != "" {
		t.Errorf("player.html still passes a raw decoded link to HLS.js: %q", m)
	}
}

// El muestreo debe descartar los repetidos, nunca el primero: el primer error
// es el que dice la causa y sin él Grafana se queda en silencio.
func TestPlayerTelemetriaMandaElPrimerError(t *testing.T) {
	html := playerHTML(t)
	i := strings.Index(html, "send(event, fields, cadaN)")
	if i < 0 {
		t.Fatal("no se encuentra tel.send en player.html")
	}
	cuerpo := html[i : i+700]
	if !strings.Contains(cuerpo, "n > 1") {
		t.Error("el muestreo sigue descartando el primer error: debe comprobar n > 1 antes de aplicar cadaN")
	}
	if !strings.Contains(cuerpo, "n % cadaN !== 0") {
		t.Error("el muestreo perdió la comparación n % cadaN")
	}
}

// Un enlace no reproducible debe avisar, no lanzar HLS.js contra un esquema
// que el navegador no sabe cargar.
func TestPlayerValidaElEnlaceAntesDePedirlo(t *testing.T) {
	html := playerHTML(t)
	if !strings.Contains(html, "const hlsUrl = hlsUrlPara(linkID, pid);") {
		t.Error("loadVideo/loadVideoForLink no pasan por hlsUrlPara")
	}
	if n := strings.Count(html, "const hlsUrl = hlsUrlPara(linkID, pid);"); n != 2 {
		t.Errorf("hlsUrlPara se usa %d veces; se esperan 2 (loadVideo y loadVideoForLink)", n)
	}
}
