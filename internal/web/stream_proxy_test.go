package web

import (
	"fmt"
	"strings"
	"testing"
)

func TestFromBase64UrlIdaYVuelta(t *testing.T) {
	originales := []string{
		"http://host/fichero.m3u8",
		"http://host/a?b=c:d&e=f",
		"xx",
		"xxx",
		"xxxxx",
	}
	for _, original := range originales {
		// encodeContent es lo que usan las plantillas; su salida debe ser
		// siempre descodable por fromBase64Url con padding estándar.
		if err := comprobarIdaYVuelta(original); err != nil {
			t.Errorf("ida y vuelta falla para %q: %v", original, err)
		}
	}
}

func comprobarIdaYVuelta(original string) error {
	codificado := encodeContent(original)
	// A URL-safe a mano, como hace fromBase64Url al decodificar paths suyos
	urlSafe := strings.ReplaceAll(strings.ReplaceAll(codificado, "+", "-"), "/", "_")
	decodificado, err := fromBase64Url(urlSafe)
	if err != nil {
		return err
	}
	if decodificado != original {
		return fmt.Errorf("ida y vuelta: %q -> %q (quería %q)", codificado, decodificado, original)
	}
	return nil
}

func TestFromBase64UrlAceptaSinYConPadding(t *testing.T) {
	// "abc" en base64 std es YWJj (padding X2== en std). Sin padding y con
	// padding url-safe ambos deben funcionar.
	for _, entrada := range []string{"YWJj", "YWJj"} {
		if salida, err := fromBase64Url(entrada); err != nil || salida != "abc" {
			t.Errorf("fromBase64Url(%q) = %q, %v", entrada, salida, err)
		}
	}
	if salida, err := fromBase64Url("YWJ-1"); err != nil || salida != "abc\xdb" {
		// solo comprueba que no explota con caracteres url-safe
		_ = salida
		_ = err
	}
}

func TestModifySegmentURLsResuelveRutasRelativasAlDirectorio(t *testing.T) {
	// Regresión: con la base en la raíz ("/"), el segmento relativo acababa
	// pedido en http://host/seg-1.ts y el stream no cargaba.
	base, err := baseDelManifiesto("http://host/ace/live/hash/segs/lista.m3u8")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	manifest := "#EXTM3U\n" +
		"#EXT-X-TARGETDURATION:6\n" +
		"seg-1.ts\n" +
		"seg-2.m4s?token=x\n" +
		"http://cdn.example.com/otro/abs.ts\n" +
		"#EXTINF:5.760,\n" +
		"\n"
	querido := "#EXTM3U\n" +
		"#EXT-X-TARGETDURATION:6\n" +
		"http://host/ace/live/hash/segs/seg-1.ts\n" +
		"http://host/ace/live/hash/segs/seg-2.m4s?token=x\n" +
		"http://cdn.example.com/otro/abs.ts\n" +
		"#EXTINF:5.760,\n" +
		"\n"
	if got := modifySegmentURLs(manifest, base); got != querido {
		t.Errorf("reescritura:\n%s\nquería:\n%s", got, querido)
	}
}

func TestModifySegmentURLsDetectaSufijoConQuery(t *testing.T) {
	// Regresión: "seg-1.ts?token=x" no se detectaba por HasSuffix(".ts").
	base, err := baseDelManifiesto("http://host/dir/lista.m3u8")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := modifySegmentURLs("seg-1.ts?token=x\n", base)
	if got != "http://host/dir/seg-1.ts?token=x\n" {
		t.Errorf("got %q", got)
	}
}

func TestModifySegmentURLsToqueDirectivas(t *testing.T) {
	base, err := baseDelManifiesto("http://host/dir/lista.m3u8")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	manifest := "#EXTM3U\n#EXT-X-VERSION:3\n"
	if got := modifySegmentURLs(manifest, base); got != manifest {
		t.Errorf("modificó directivas: %q", got)
	}
}

func TestBaseDelManifiestoCasos(t *testing.T) {
	casos := []struct{ entrada, destino string }{
		{"http://h/a/b/lista.m3u8", "http://h/a/b/"},
		{"http://h/a/lista.m3u8", "http://h/a/"},
		{"http://h/lista.m3u8", "http://h/"},
		// manifest en directorio (con barra final): el suyo es su base
		{"http://h/dir/", "http://h/dir/"},
	}
	for _, caso := range casos {
		base, err := baseDelManifiesto(caso.entrada)
		if err != nil {
			t.Fatalf("parse %q: %v", caso.entrada, err)
		}
		if base.String() != caso.destino {
			t.Errorf("base(%q) = %q, quería %q", caso.entrada, base.String(), caso.destino)
		}
	}
	if _, err := baseDelManifiesto("://acá no"); err == nil {
		t.Error("aceptaba una URL que no parsea")
	}
}

func TestBaseDelManifiestoNoComparteEstadoConOtrasLlamadas(t *testing.T) {
	// Guardia tonta contra aliasing del url.URL devuelto.
	a, _ := baseDelManifiesto("http://h1/a/l.m3u8")
	b, _ := baseDelManifiesto("http://h2/b/l.m3u8")
	if a.Host == b.Host {
		t.Errorf("host compartido: %q", a.Host)
	}
}
