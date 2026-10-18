package app

// Tests de la política proxy-primero para las fuentes de canales que el ISP
// estrangula en salida directa (ipfs/filebase). Antes se reintentaba en
// directo hasta agotar maxFetchAttempts y solo entonces se probaba el proxy,
// lo que retrasaba el arranque varios segundos por fuente.

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestFuenteProxyFirstUsaProxyYNoTocaDirecto(t *testing.T) {
	var proxyCalls, directCalls atomic.Int32
	fetchWebData = func(u string, proxied bool) ([]byte, error) {
		if proxied {
			proxyCalls.Add(1)
			return []byte("EXTINF:-1,OK\n"), nil
		}
		directCalls.Add(1)
		return nil, fmt.Errorf("no debería llamarse directo")
	}
	defer func() { fetchWebData = FetchWebData }()

	src := Source{Name: "prueba", Proxied: true, ProxyFirst: true}
	res := fetchSourceBody(src, true)

	if !res.success() {
		t.Fatalf("esperaba éxito vía proxy, err=%v", res.err)
	}
	if n := proxyCalls.Load(); n != 1 {
		t.Errorf("llamadas al proxy = %d, se esperaba 1", n)
	}
	if n := directCalls.Load(); n != 0 {
		t.Errorf("el proxy funcionó pero se intentó directo %d veces", n)
	}
}

func TestFuenteProxyFirstCaeADirectoSiProxyFalla(t *testing.T) {
	var proxyCalls, directCalls atomic.Int32
	fetchWebData = func(u string, proxied bool) ([]byte, error) {
		if proxied {
			proxyCalls.Add(1)
			return nil, fmt.Errorf("proxy caído")
		}
		directCalls.Add(1)
		return []byte("EXTINF:-1,OK\n"), nil
	}
	defer func() { fetchWebData = FetchWebData }()

	src := Source{Name: "prueba", Proxied: true, ProxyFirst: true}
	res := fetchSourceBody(src, true)

	if !res.success() {
		t.Fatalf("esperaba éxito en directo tras caer el proxy, err=%v", res.err)
	}
	if n := proxyCalls.Load(); n != 3 {
		t.Errorf("intentos por proxy = %d, se esperaban 3 antes de caer a directo", n)
	}
	if n := directCalls.Load(); n != 1 {
		t.Errorf("intentos directos = %d, se esperaba 1", n)
	}
}

// Un 4xx hopeless en el proxy no debe gastar los tres reintentos: la ruta
// directa tampoco va a devolver contenido, pero al menos lo registra rápido.
func TestFuenteProxyFirstNoReintentaAnte4xx(t *testing.T) {
	var proxyCalls atomic.Int32
	fetchWebData = func(u string, proxied bool) ([]byte, error) {
		if proxied {
			proxyCalls.Add(1)
			return nil, err404()
		}
		return nil, err404()
	}
	defer func() { fetchWebData = FetchWebData }()

	src := Source{Name: "prueba", Proxied: true, ProxyFirst: true}
	res := fetchSourceBody(src, true)

	if res.success() {
		t.Fatal("un 404 no debería considerarse éxito")
	}
	if n := proxyCalls.Load(); n != 1 {
		t.Errorf("intentos por proxy ante 4xx = %d, se esperaba 1", n)
	}
}

func TestFetchSourceBodyProxyFirstRegistraCuantoTarda(t *testing.T) {
	var proxyCalls atomic.Int32
	fetchWebData = func(u string, proxied bool) ([]byte, error) {
		proxyCalls.Add(1)
		return nil, fmt.Errorf("proxy caído")
	}
	defer func() { fetchWebData = FetchWebData }()

	src := Source{Name: "prueba", Proxied: true, ProxyFirst: true}
	start := time.Now()
	fetchSourceBody(src, true)
	// Tres intentos con backoff 3s+6s = ~9s; nos quedamos bajo ese techo.
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("tardó %v, el backoff proxy-first es excesivo", d)
	}
}

func TestFuentesFilebaseMarcadasProxyFirst(t *testing.T) {
	want := map[string]bool{"ipfs_hashes": true, "listaplana": true}
	for _, s := range sources {
		if _, ok := want[s.Name]; ok && !s.ProxyFirst {
			t.Errorf("la fuente %s debería ser ProxyFirst", s.Name)
		}
	}
	if sources[0].Name == "" {
		t.Fatal("la lista de fuentes no puede empezar vacía")
	}
}

// err404 simula el 4xx hopeless que isFatalFetchErr detecta por texto.
func err404() error { return fmt.Errorf("status code error: 404 404 <none>") }
