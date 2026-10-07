package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// Un fallo de DNS o de conexión no se arregla reintentando contra la misma
// salida: hay que cambiar de transporte. Con filebase se midió 32 s de
// reintentos por directa frente a 1 s por proxy.
func TestIsNetworkErr(t *testing.T) {
	red := []error{
		errors.New(`Get "https://ipfs.filebase.io/x": dial tcp: lookup ipfs.filebase.io: no such host`),
		errors.New(`dial tcp 127.0.0.1:9050: connect: connection refused`),
		errors.New("read tcp: connection reset by peer"),
		errors.New("dial tcp: no route to host"),
		errors.New("dial tcp: network is unreachable"),
		errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
		errors.New("unexpected EOF"),
		errors.New("lookup server misbehaving"),
	}
	for _, err := range red {
		if !isNetworkErr(err) {
			t.Errorf("debería detectarse como error de red: %v", err)
		}
	}
	otros := []error{
		nil,
		errors.New("status code error: 403 Forbidden"),
		errors.New("status code error: 429 Too Many Requests"),
		errors.New("error al parsear el HTML: EOF inesperado"),
	}
	for _, err := range otros {
		if isNetworkErr(err) {
			t.Errorf("no debería detectarse como error de red: %v", err)
		}
	}
}

// Un 4xx sigue siendo fatal (no fallback), pero un error de red debe dejar
// paso al fallback por proxy: fatal4xx se queda en false a propósito.
func TestShouldProxyFallbackConErrorDeRed(t *testing.T) {
	src := Source{Name: "x", Proxied: false}
	if !shouldProxyFallback(src, false, false) {
		t.Error("con error de red (fatal=false) debe entrar el fallback por proxy")
	}
	if shouldProxyFallback(src, false, true) {
		t.Error("un 4xx fatal no debe entrar al fallback")
	}
	if shouldProxyFallback(src, true, false) {
		t.Error("si ya tuvo éxito no debe entrar al fallback")
	}
	proxied := Source{Name: "x", Proxied: true}
	if shouldProxyFallback(proxied, false, false) {
		t.Error("una fuente ya proxied no necesita fallback")
	}
	if maxNetworkRetries >= 10 {
		t.Errorf("maxNetworkRetries=%d anula el ahorro: el bucle tiene 10 intentos", maxNetworkRetries)
	}
}

// Una fuente con resolver cuyo índice devuelve 429 NO es un 4xx hopeless: se
// reintenta y, si no sale, entra el fallback por proxy. Antes el resolver
// marcaba fatal4xx siempre, así que un 429 del worker de elcano mataba la
// fuente en el primer intento y se perdía entera.
func TestFetchSourceBodyCon429EnResolverReintenta(t *testing.T) {
	var resolves int32
	src := Source{
		Name: "tokyo_elcano",
		URL:  "http://127.0.0.1:1/nunca-se-usa",
		Type: SourceTxtRaw,
		// Proxied: evita el fallback por proxy, que en test cuesta ~30 s de
		// timeouts contra SOCKS inexistentes. La decisión de entrar al proxy se
		// verifica en TestShouldProxyFallbackTras429DeResolver.
		Proxied: true,
		Resolve: func(index string) (string, error) {
			atomic.AddInt32(&resolves, 1)
			return "", fmt.Errorf("no se pudo leer el índice %s: status code error: 429 429 Too Many Requests", index)
		},
	}

	res := fetchSourceBodyConPolitica(src, false, 3, func(int) time.Duration { return 0 })

	if atomic.LoadInt32(&resolves) < 2 {
		t.Errorf("un 429 debe reintentarse: el resolver se llamó %d vez/veces", resolves)
	}
	if res.attempts < 2 {
		t.Errorf("un 429 debe reintentarse: solo hubo %d intento(s)", res.attempts)
	}
	if res.fatal4xx {
		t.Error("un 429 no debe marcarse fatal4xx (deja pasar el fallback por proxy)")
	}
	if res.success() {
		t.Error("nunca respondió bien: no debe darse por buena")
	}
}

// Con un 429 (fatal4xx=false) una fuente directa tiene que entrar al fallback
// por proxy: es justo lo que se perdía cuando el resolver marcaba fatal.
func TestShouldProxyFallbackTras429DeResolver(t *testing.T) {
	src := Source{
		Name: "tokyo_elcano",
		Type: SourceTxtRaw,
		Resolve: func(string) (string, error) {
			return "", fmt.Errorf("no se pudo leer el índice: status code error: 429 429 Too Many Requests")
		},
	}
	// Un 429 no es fatal: la fuente directa agotada debe poder entrar al proxy.
	if !shouldProxyFallback(src, false, false) {
		t.Error("tras un 429 la fuente debe poder reintentarse vía proxy")
	}
}

// El 4xx hopeless (403) sí es fatal y sí corta: no tiene sentido reintentar un
// 10 veces ni pedirle al proxy algo que va a seguir dando Forbidden.
func TestFetchSourceBodyCon403EnResolverEsFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	src := Source{
		Name: "tokyo_elcano",
		URL:  srv.URL,
		Type: SourceTxtRaw,
		Resolve: func(index string) (string, error) {
			return "", fmt.Errorf("no se pudo leer el índice %s: status code error: 403 Forbidden", index)
		},
	}

	res := fetchSourceBody(src, false)

	if !res.fatal4xx {
		t.Error("un 403 sí debe marcarse fatal4xx")
	}
	if res.attempts != 1 {
		t.Errorf("un 403 debe cortar en el intento 1, hizo %d", res.attempts)
	}
}

// Una fuente sin resolver que responde bien tiene que devolver el cuerpo tal
// cual, sin pasar por el fallback ni perderlo.
func TestFetchSourceBodySinResolverOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("cuerpo"))
	}))
	defer srv.Close()

	res := fetchSourceBody(Source{Name: "x", URL: srv.URL, Type: SourceTxtRaw}, false)

	if !res.success() || string(res.body) != "cuerpo" {
		t.Fatalf("res = %+v, se esperaba el cuerpo \"cuerpo\"", res)
	}
	if res.attempts != 1 {
		t.Errorf("un acierto debe gastar un intento, gastó %d", res.attempts)
	}
}

// Un hash mal publicado (39 hex + una letra) no puede quedarse como nombre de
// canal ni contaminar el emparejado del par bueno de al lado. Antes producía
// dos avisos en cascada que parecían una pérdida de entrada.
func TestTxtRawDescartaHashCorrupto(t *testing.T) {
	body := []byte(strings.Join([]string{
		"CANAL ROTO",
		"d4ff041287a43e3114d411d671c4b4e92e21f33y", // corrupto: 39 hex + "y"
		"M+ LALIGA FHD --> NEW ERA VI",
		"acestream://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"LA 1",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}, "\n"))

	got := extractDataFromWebTxtRaw(body)

	if _, ok := got["d4ff041287a43e3114d411d671c4b4e92e21f33y"]; ok {
		t.Error("el hash corrupto acabó siendo nombre de canal")
	}
	if _, ok := got["CANAL ROTO"]; ok {
		t.Error("un nombre sin hash no debe crear entrada: se emparejó con el hash corrupto")
	}
	// "M+ LALIGA FHD --> NEW ERA VI" se normaliza a "M+ LALIGA" (se recorta
	// tras --> y la etiqueta de calidad).
	if h := got["M+ LALIGA"]; len(h) != 1 || h[0] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("el par bueno no se recuperó: %v", got)
	}
	if h := got["LA 1"]; len(h) != 1 || h[0] != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("el par siguiente se vio afectado: %v", got["LA 1"])
	}
}

// Un hash válido (40 hex) NUNCA se descarta, ni pelado ni con acestream://.
func TestTxtRawNoDescartaHashesValidos(t *testing.T) {
	validos := []string{
		"0123456789abcdef0123456789abcdef01234567",
		"acestream://0123456789abcdef0123456789abcdef01234567",
	}
	for _, h := range validos {
		if esHashCorrupto(h) {
			t.Errorf("hash válido descartado por esHashCorrupto: %q", h)
		}
	}
	// Un nombre de canal con espacios y letras no puede ser un hash roto.
	if esHashCorrupto("M+ LALIGA") {
		t.Error("un nombre de canal no es un hash corrupto")
	}
	if esHashCorrupto("") {
		t.Error("línea vacía no es un hash corrupto")
	}
}

// Los nombres largos en cirílico/ruso de listaplana (НТВ, КХЛ ТВ) deben
// sobrevivir al filtro de ruido: tienen espacios y van más allá de 42
// caracteres cuando llevan coletilla.
func TestTxtRawConservaNombresNoASCII(t *testing.T) {
	body := []byte("НТВ\n0123456789abcdef0123456789abcdef01234567\nКХЛ ТВ\nfedcba9876543210fedcba9876543210fedcba98\n")
	got := extractDataFromWebTxtRaw(body)
	if len(got["НТВ"]) != 1 {
		t.Errorf("НТВ perdido: %v", got)
	}
	if len(got["КХЛ ТВ"]) != 1 {
		t.Errorf("КХЛ ТВ perdido: %v", got)
	}
}

// La resolución de enlaces "p;" no puede dejar el arranque en manos de un
// único servidor caído: el timeout tiene que ser menor que el global.
func TestPLinkTimeoutMenorQueGlobal(t *testing.T) {
	if pLinkTimeout >= timeTimeout {
		t.Errorf("pLinkTimeout=%s debe ser menor que timeTimeout=%s: aquí solo se busca la URL final", pLinkTimeout, timeTimeout)
	}
	if pLinkWorkers < 2 {
		t.Errorf("pLinkWorkers=%d deja la resolución en serie", pLinkWorkers)
	}
}

// Un host caído se recuerda: dos enlaces del mismo servidor muerto no pueden
// costar dos timeouts.
func TestHostMuerto(t *testing.T) {
	h := newHostMuerto()
	if h.isDead("181.78.106.127:9000") {
		t.Error("un host recién creado no puede estar muerto")
	}
	h.markDead("181.78.106.127:9000")
	if !h.isDead("181.78.106.127:9000") {
		t.Error("el host marcado debe recordarse")
	}
	if h.isDead("190.92.10.66:4000") {
		t.Error("un host distinto no debe marcarse")
	}
	if host := hostDeURL("http://181.78.106.127:9000/play/ca028/index.m3u8"); host != "181.78.106.127:9000" {
		t.Errorf("hostDeURL = %q", host)
	}
}

// Los nombres de evento ACB se renombran a los de DAZN en el mapa de broadcasters.
func TestTxtRawRenombraEventosACB(t *testing.T) {
	body := []byte("ACB EVENTO 01\n0123456789abcdef0123456789abcdef01234567\n")
	got := extractDataFromWebTxtRaw(body)
	if len(got["DAZN BALONCESTO 1"]) != 1 {
		t.Errorf("ACB EVENTO 01 no se renombró: %v", got)
	}
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
