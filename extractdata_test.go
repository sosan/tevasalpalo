package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// El worker de elcano publica detrás de un IPNS cuyo subdirectorio (CID) rota
// en cada republicación. Con el subdirectorio viejo responde 403, así que la
// URL no se puede hardcodear: hay que leer el <base href> del índice cada vez.
// Estos tests fijan ese comportamiento sin tocar la red.

func TestResolveElcanoFileURL(t *testing.T) {
	casos := []struct {
		nombre string
		file   string
		html   string
		want   string
	}{
		{
			nombre: "base href normal",
			file:   "hashes.txt",
			html:   `<html><head><base href="https://host/ipns/abc/39332e0b86377b9db6df226e/"></head></html>`,
			want:   "https://host/ipns/abc/39332e0b86377b9db6df226e/hashes.txt",
		},
		{
			nombre: "base href sin barra final",
			file:   "hashes.txt",
			html:   `<html><head><base href="https://host/ipns/abc/39332e0b86377b9db6df226e"></head></html>`,
			want:   "https://host/ipns/abc/39332e0b86377b9db6df226e/hashes.txt",
		},
		{
			nombre: "comillas simples y BASE en mayúsculas",
			file:   "hashes.txt",
			html:   `<html><head><BASE HREF='https://host/ipns/abc/deadbeef/'></head></html>`,
			want:   "https://host/ipns/abc/deadbeef/hashes.txt",
		},
		{
			nombre: "base href con atributos extra antes de href",
			file:   "hashes.txt",
			html:   `<html><head><base target="_blank" href="https://host/ipns/abc/cafe/" /></head></html>`,
			want:   "https://host/ipns/abc/cafe/hashes.txt",
		},
		{
			nombre: "otro fichero de la misma publicacion",
			file:   "hashes.m3u",
			html:   `<html><head><base href="https://host/ipns/abc/39332e0b/"></head></html>`,
			want:   "https://host/ipns/abc/39332e0b/hashes.m3u",
		},
	}

	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(tc.html))
			}))
			defer srv.Close()

			resolve := resolveElcanoFileURL(tc.file)
			got, err := resolve(srv.URL + "/")
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveElcanoFileURLSinBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head></head><body>sin base</body></html>"))
	}))
	defer srv.Close()

	if _, err := resolveElcanoFileURL("hashes.txt")(srv.URL + "/"); err == nil {
		t.Fatal("se esperaba error si el índice no trae <base href>")
	}
}

func TestResolveElcanoFileURLIndexCaido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := resolveElcanoFileURL("hashes.txt")(srv.URL + "/"); err == nil {
		t.Fatal("se esperaba error si el índice devuelve 4xx")
	}
}

// La fuente tokyo_elcano debe llevar resolver: es la que evita que se quede
// muda cuando el publicador rota el subdirectorio.
func TestFuenteElcanoTieneResolver(t *testing.T) {
	for _, s := range sources {
		if s.Name != "tokyo_elcano" {
			continue
		}
		if s.Resolve == nil {
			t.Fatal("tokyo_elcano sin resolver: la URL hardcodeada caduca en cada republicación")
		}
		return
	}
	t.Fatal("no existe la fuente tokyo_elcano")
}

// Una fuente repetida no aporta nada y solo duplica peticiones. Pasó con
// git.gay/canales_acestream.m3u, que es byte-idéntico a platinsportCanales.
func TestSourcesSinURLsDuplicadas(t *testing.T) {
	vistos := map[string]string{}
	for _, s := range sources {
		if prev, dup := vistos[s.URL]; dup {
			t.Errorf("URL duplicada en las fuentes: %q y %q comparten %s", prev, s.Name, s.URL)
		}
		vistos[s.URL] = s.Name
	}
}

// URLs que se responderon 404 / dejaron de existir y no deben volver a la
// lista sin comprobar. Ver el comentario de `sources` en extractdata.go.
func TestSourcesSinURLsConocidasMuertas(t *testing.T) {
	muertas := []struct {
		patron string
		motivo string
	}{
		{"lista_acestream_unificada.m3u", "404: el fichero se borró del repo"},
		{"git.gay/TokyoGhoulles/AceStream_IDs", "404: repo movido; lo sustituye tokyo_elcano"},
		{"ipns.dweb.link", "dweb.link ya solo es service-worker-gateway"},
		{"ipfs.inbrowser.link", "solo sirve un bootstrap JS; el contenido va por pares"},
		{"/hashes.m3u", "mismos 126 hashes que hashes_acestream.m3u"},
		{"lista_fuera_iptv", "mismos 400 hashes y nombres que listaplana"},
	}
	for _, s := range sources {
		for _, m := range muertas {
			if strings.Contains(s.URL, m.patron) {
				t.Errorf("la fuente %q usa una URL retirada (%s: %s)", s.Name, m.patron, m.motivo)
			}
		}
	}
}

// Las gateways IPFS grandes ya no sirven por HTTP: listaplana solo es
// alcanzable por una gateway que aún hable el protocolo como Filebase.
func TestFuenteListaplanaUsaGatewayHttp(t *testing.T) {
	for _, s := range sources {
		if s.Name != "listaplana" {
			continue
		}
		if !strings.HasPrefix(s.URL, ipfsGateway) {
			t.Fatalf("listaplana debe usar %s, usa %s", ipfsGateway, s.URL)
		}
		return
	}
	t.Fatal("no existe la fuente listaplana")
}

// Rellena un broadcaster que en vars.go solo tenía hashes congelados y sin
// entrada en el gateway. Se resuelve con normGateway, el mismo camino que usa
// updateBroadcasterMapWithGatewayTolerant en producción.
func TestGatewayRellenaTargets(t *testing.T) {
	ensureNormGateway()
	casos := []struct {
		nombreFuente string
		destino      string
	}{
		{"Eleven Sports 2", "ELEVEN SPORTS 2"},
		{"Eleven Sports 3", "ELEVEN SPORTS 3"},
		{"Eleven Sports 4", "ELEVEN SPORTS 4"},
		{"TNT", "TNT SPORTS 1"},
	}
	for _, tc := range casos {
		t.Run(tc.nombreFuente, func(t *testing.T) {
			targets := normGateway[normalizeTolerant(tc.nombreFuente)]
			if len(targets) == 0 {
				t.Fatalf("%q no resuelve a nada en el gateway", tc.nombreFuente)
			}
			if !contains(targets, tc.destino) {
				t.Fatalf("%q resuelve a %v, se esperaba %q", tc.nombreFuente, targets, tc.destino)
			}
			// el destino tiene que existir de verdad como broadcaster
			if _, ok := broadcasterToAcestream[tc.destino]; !ok {
				t.Fatalf("el destino %q no está en vars.go", tc.destino)
			}
		})
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Toda fuente sin resolver debe usar un tipo que el parser soporta.
func TestSourcesTipoValido(t *testing.T) {
	for _, s := range sources {
		switch s.Type {
		case SourceTxtRaw, SourceM3U:
		default:
			t.Errorf("fuente %q con tipo desconocido %q", s.Name, s.Type)
		}
		if s.Name == "" || s.URL == "" {
			t.Errorf("fuente incompleta: %+v", s)
		}
	}
}