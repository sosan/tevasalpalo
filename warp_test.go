package main

// Tests de WARP. Todo offline: respuestas de /reg sintéticas y ninguna
// credencial real (la identidad que se probó a mano no entra al repo).

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// regResponseBuild sintetiza el JSON que devuelve /reg.
const regResponseBuild = `{
  "id": "5e466e7f-7134-4fe9-84a7-1ef5d1aa0d9e",
  "token": "7bf87e2d-cc7a-4c2c-9012-ba16c8844da6",
  "model": "PC",
  "account": {
    "account_type": "free",
    "license": "q89TSs73-0Tv593Lh-U68lB29J",
    "warp_plus": true
  },
  "config": {
    "client_id": "Qt0l",
    "interface": { "addresses": {
      "v4": "172.16.0.2",
      "v6": "2606:4700:110:8f7c:da91:2516:6b2f:a95d"
    }},
    "peers": [{
      "public_key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
      "endpoint": {
        "host": "engage.cloudflareclient.com:2408",
        "v4": "162.159.192.9:0",
        "v6": "[2606:4700:d0::a29f:c009]:0",
        "ports": [2408, 500, 1701, 4500]
      }
    }]
  }
}`

func TestWarpRegisterBody(t *testing.T) {
	body, err := warpRegisterBody("PUBKEY", "INSTALLID22chars")
	if err != nil {
		t.Fatalf("warpRegisterBody: %v", err)
	}
	if body["key"] != "PUBKEY" {
		t.Fatalf("key = %q", body["key"])
	}
	if body["install_id"] != "INSTALLID22chars" {
		t.Fatalf("install_id = %q", body["install_id"])
	}
	// fcm_token = install_id + ":APA91b" + 134 aleatorios.
	const installID = "INSTALLID22chars"
	fcm := body["fcm_token"]
	if !strings.HasPrefix(fcm, installID+":APA91b") {
		t.Fatalf("fcm_token no lleva install_id+prefijo: %q", fcm)
	}
	if want := len(installID) + len(":APA91b") + 134; len(fcm) != want {
		t.Fatalf("fcm_token len = %d, want %d", len(fcm), want)
	}
	if body["model"] != "PC" || body["serial_number"] != "INSTALLID22chars" {
		t.Fatalf("model/serial_number = %q/%q", body["model"], body["serial_number"])
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", body["tos"]); err != nil {
		t.Fatalf("tos %q no parsea: %v", body["tos"], err)
	}
	if _, err := warpRegisterBody("", "x"); err == nil {
		t.Fatal("se esperaba error con clave vacía")
	}
}

func TestWarpRegisterBodyFcmRandom(t *testing.T) {
	a, err := warpRegisterBody("K", "I")
	if err != nil {
		t.Fatal(err)
	}
	b, err := warpRegisterBody("K", "I")
	if err != nil {
		t.Fatal(err)
	}
	if a["fcm_token"] == b["fcm_token"] {
		t.Fatal("fcm_token debe ser aleatorio en cada registro")
	}
}

func TestWarpIdentityFromAccount(t *testing.T) {
	var acc warpAccount
	if err := jsonUnmarshalForTest([]byte(regResponseBuild), &acc); err != nil {
		t.Fatalf("json: %v", err)
	}
	ident, err := warpIdentityFromAccount(acc, "PRIVKEY")
	if err != nil {
		t.Fatalf("warpIdentityFromAccount: %v", err)
	}
	if ident.PrivateKey != "PRIVKEY" {
		t.Fatalf("private_key = %q", ident.PrivateKey)
	}
	// reserved sale de client_id: "Qt0l" son 3 bytes.
	if len(ident.Reserved) != 3 || ident.Reserved[0] != 66 || ident.Reserved[1] != 221 || ident.Reserved[2] != 37 {
		t.Fatalf("reserved = %v", ident.Reserved)
	}
	if ident.PeerPublic != "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=" {
		t.Fatalf("peer_public_key = %q", ident.PeerPublic)
	}
	if ident.AddressV4 != "172.16.0.2" {
		t.Fatalf("address_v4 = %q", ident.AddressV4)
	}
	if ident.AddressV6 != "2606:4700:110:8f7c:da91:2516:6b2f:a95d" {
		t.Fatalf("address_v6 = %q", ident.AddressV6)
	}
	// El campo v4 trae ":0": se quita el puerto.
	if ident.EndpointV4 != "162.159.192.9" {
		t.Fatalf("endpoint_v4 = %q", ident.EndpointV4)
	}
	if ident.Token == "" || ident.DeviceID == "" || !ident.WarpPlus || ident.AccountType != "free" {
		t.Fatalf("cuenta = %+v", ident)
	}
	if ident.License != "q89TSs73-0Tv593Lh-U68lB29J" {
		t.Fatalf("license = %q", ident.License)
	}
}

func TestWarpIdentityFromAccountRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"sin peers", `{"config":{"client_id":"Qt0l"}}`},
		{"peer sin public_key", `{"config":{"client_id":"Qt0l","peers":[{}]}}`},
		{"reserved de 4 bytes", `{"config":{"client_id":"AAECAw==","peers":[{"public_key":"k"}]}}`},
		{"client_id no base64", `{"config":{"client_id":"!!!","peers":[{"public_key":"k"}]}}`},
		{"sin address_v4", `{"config":{"client_id":"Qt0l","interface":{"addresses":{}},"peers":[{"public_key":"k"}]}}`},
	}
	for _, tc := range tests {
		var acc warpAccount
		if err := jsonUnmarshalForTest([]byte(tc.raw), &acc); err != nil {
			t.Fatalf("%s: json: %v", tc.name, err)
		}
		if _, err := warpIdentityFromAccount(acc, "PRIV"); err == nil {
			t.Fatalf("%s: se esperaba error", tc.name)
		}
	}
}

func TestWarpEndpointCandidates(t *testing.T) {
	ident := &warpIdentity{
		EndpointV4:   "162.159.192.9",
		EndpointHost: "engage.cloudflareclient.com:2408",
		Ports:        []int{2408, 500, 1701, 4500},
	}
	got := warpEndpointCandidates(ident)
	want := []string{
		"162.159.192.9:2408",
		"162.159.192.9:500",
		"162.159.192.9:1701",
		"162.159.192.9:4500",
		"engage.cloudflareclient.com:2408",
	}
	if len(got) != len(want) {
		t.Fatalf("candidatos = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidato %d = %q, want %q", i, got[i], want[i])
		}
	}
	// Sin IP v4, solo el host con nombre.
	hostOnly := warpEndpointCandidates(&warpIdentity{EndpointHost: "engage.cloudflareclient.com:2408"})
	if len(hostOnly) != 1 || hostOnly[0] != "engage.cloudflareclient.com:2408" {
		t.Fatalf("solo host = %v", hostOnly)
	}
	// Identidad inservible: sin candidatos en vez de inventar endpoints.
	if extra := warpEndpointCandidates(&warpIdentity{}); len(extra) != 0 {
		t.Fatalf("sin host = %v", extra)
	}
}

func TestWarpHostOnly(t *testing.T) {
	cases := map[string]string{
		"162.159.192.9:0":             "162.159.192.9",
		"162.159.192.9:2408":          "162.159.192.9",
		"[2606:4700:d0::a29f:c009]:0": "2606:4700:d0::a29f:c009",
		"162.159.192.9":               "162.159.192.9",
		"":                            "",
	}
	for in, want := range cases {
		if got := hostOnly(in); got != want {
			t.Fatalf("hostOnly(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWarpLink(t *testing.T) {
	ident := &warpIdentity{
		PrivateKey: "cHJpdmF0YS1rZXk=",
		PeerPublic: "cGVlci1wdWJsaWM=",
		Reserved:   []byte{66, 221, 37},
		AddressV4:  "172.16.0.2",
		AddressV6:  "2606:4700:110::1",
	}
	link, err := warpLink(ident, "162.159.192.9:2408")
	if err != nil {
		t.Fatalf("warpLink: %v", err)
	}
	// Debe ser parseable por el parser de xray.go sin cambios.
	ep, err := parseXrayLink(link)
	if err != nil {
		t.Fatalf("parseXrayLink: %v", err)
	}
	if ep.Protocol != "wireguard" {
		t.Fatalf("protocol = %q", ep.Protocol)
	}
	if ep.WgPrivate != ident.PrivateKey {
		t.Fatalf("WgPrivate = %q, want %q", ep.WgPrivate, ident.PrivateKey)
	}
	if ep.WgPeer != ident.PeerPublic {
		t.Fatalf("WgPeer = %q", ep.WgPeer)
	}
	if ep.Address != "162.159.192.9" || ep.Port != 2408 {
		t.Fatalf("endpoint = %s:%d", ep.Address, ep.Port)
	}
	if len(ep.WgRes) != 3 || ep.WgRes[0] != 66 || ep.WgRes[1] != 221 || ep.WgRes[2] != 37 {
		t.Fatalf("reserved = %v", ep.WgRes)
	}
	if ep.WgAddr != "172.16.0.2,2606:4700:110::1" {
		t.Fatalf("WgAddr = %q", ep.WgAddr)
	}
	if ep.WgMTU != warpMTU || ep.WgKeep != warpKeepAlive {
		t.Fatalf("mtu/keepalive = %d/%d", ep.WgMTU, ep.WgKeep)
	}
	if ep.WgPsk != "" {
		t.Fatalf("psk inesperado sin WARP+: %q", ep.WgPsk)
	}
}

func TestWarpLinkPlusUsesLicenseAsPsk(t *testing.T) {
	ident := &warpIdentity{
		PrivateKey: "cHJpdmF0YS1rZXk=",
		PeerPublic: "cGVlci1wdWJsaWM=",
		Reserved:   []byte{1, 2, 3},
		AddressV4:  "172.16.0.2",
		WarpPlus:   true,
		License:    "q89TSs73-0Tv593Lh-U68lB29J",
	}
	ep, err := parseXrayLink(mustWarpLink(t, ident, "162.159.192.9:2408"))
	if err != nil {
		t.Fatal(err)
	}
	if ep.WgPsk != ident.License {
		t.Fatalf("psk = %q, want licencia %q", ep.WgPsk, ident.License)
	}
}

func TestWarpLinkRejectsBadReserved(t *testing.T) {
	ident := &warpIdentity{
		PrivateKey: "k", PeerPublic: "p", AddressV4: "172.16.0.2",
		Reserved: []byte{1, 2},
	}
	if _, err := warpLink(ident, "1.2.3.4:2408"); err == nil {
		t.Fatal("se esperaba error con reserved de 2 bytes")
	}
}

func mustWarpLink(t *testing.T, ident *warpIdentity, endpoint string) string {
	t.Helper()
	link, err := warpLink(ident, endpoint)
	if err != nil {
		t.Fatalf("warpLink: %v", err)
	}
	return link
}

func TestWarpAccountRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warp", "account.json")
	ident := &warpIdentity{
		PrivateKey: "priv", PeerPublic: "pub", Reserved: []byte{1, 2, 3},
		AddressV4: "172.16.0.2", DeviceID: "dev", Token: "tok",
		Registered: time.Now().UTC().Truncate(time.Second),
	}
	if err := writeWarpAccount(path, ident); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readWarpAccount(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.PrivateKey != "priv" || got.PeerPublic != "pub" || len(got.Reserved) != 3 {
		t.Fatalf("identidad = %+v", got)
	}
	if !got.Registered.Equal(ident.Registered) {
		t.Fatalf("registered = %v, want %v", got.Registered, ident.Registered)
	}
}

func TestWarpAccountRejectsIncomplete(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"sin privada":         `{"peer_public_key":"pub","reserved":"AQID"}`,
		"sin peer":            `{"private_key":"priv","reserved":"AQID"}`,
		"reserved de 4 bytes": `{"private_key":"priv","peer_public_key":"pub","reserved":"AAECAw=="}`,
		"reserved de 2 bytes": `{"private_key":"priv","peer_public_key":"pub","reserved":"AQI="}`,
		"json roto":           `{`,
	}
	for name, content := range cases {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readWarpAccount(path); err == nil {
			t.Fatalf("%s: se esperaba error", name)
		}
	}
}

func TestWarpNewKeyPair(t *testing.T) {
	priv, pub, err := warpNewKeyPair()
	if err != nil {
		t.Fatalf("warpNewKeyPair: %v", err)
	}
	privRaw, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(privRaw) != 32 {
		t.Fatalf("priv key = %q (%v)", priv, err)
	}
	// Clamping de Curve25519.
	if privRaw[0]&7 != 0 {
		t.Fatalf("priv[0] sin clamp: %d", privRaw[0])
	}
	if privRaw[31]&0x80 != 0 || privRaw[31]&0x40 == 0 {
		t.Fatalf("priv[31] sin clamp: %d", privRaw[31])
	}
	pubRaw, err := base64.StdEncoding.DecodeString(pub)
	if err != nil || len(pubRaw) != 32 {
		t.Fatalf("pub key = %q (%v)", pub, err)
	}
	// Claves distintas en cada llamada.
	priv2, pub2, err := warpNewKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if priv == priv2 || pub == pub2 {
		t.Fatal("se esperaba un par de claves nuevo por llamada")
	}
}

func TestWarpRandString(t *testing.T) {
	s, err := warpRandString(22)
	if err != nil {
		t.Fatalf("warpRandString: %v", err)
	}
	if len(s) != 22 {
		t.Fatalf("len = %d", len(s))
	}
	for _, c := range s {
		if !strings.ContainsRune(warpAlphabet, c) {
			t.Fatalf("carácter fuera del alfabeto: %q", c)
		}
	}
}

func TestWarpEnabled(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		"1":     true,
		"true":  true,
		"on":    true,
		"0":     false,
		"false": false,
		"off":   false,
	}
	for val, want := range tests {
		t.Setenv("WARP", val)
		t.Setenv("WARP_AUTO", "")
		if got := warpEnabled(); got != want {
			t.Fatalf("WARP=%q -> %v, want %v", val, got, want)
		}
	}
	// WARP vacío pero WARP_AUTO=1 habilita.
	t.Setenv("WARP", "")
	t.Setenv("WARP_AUTO", "1")
	if !warpEnabled() {
		t.Fatal("WARP_AUTO=1 debe habilitar")
	}
	// WARP=0 gana sobre WARP_AUTO=1.
	t.Setenv("WARP", "0")
	if warpEnabled() {
		t.Fatal("WARP=0 debe mandar sobre WARP_AUTO")
	}
}

func TestWarpSocksListen(t *testing.T) {
	t.Setenv("WARP_SOCKS_ADDR", "127.0.0.1:10809")
	host, port, err := warpSocksListen()
	if err != nil || host != "127.0.0.1" || port != 10809 {
		t.Fatalf("= %s %d %v", host, port, err)
	}
	t.Setenv("WARP_SOCKS_ADDR", ":9999")
	host, port, err = warpSocksListen()
	if err != nil || host != "127.0.0.1" || port != 9999 {
		t.Fatalf("host vacío = %s %d %v", host, port, err)
	}
	for _, bad := range []string{"sin-puerto", "host:0", "host:abc", "host:70000"} {
		t.Setenv("WARP_SOCKS_ADDR", bad)
		if _, _, err := warpSocksListen(); err == nil {
			t.Fatalf("%q: se esperaba error", bad)
		}
	}
}

func TestIsAceStreamPath(t *testing.T) {
	// Rutas que en el APK disparan http_stream_start.
	for _, p := range []string{
		"getstream/abc", "/ace/getstream", "content/123", "/hls/segment.ts",
		"manifest.m3u8", "server/stream",
	} {
		if !isAceStreamPath(p) {
			t.Fatalf("%q debería contar como stream", p)
		}
	}
	// Comandos y manifests del webui no.
	for _, p := range []string{
		"version", "settings/get", "app/tok/cmd/isonline", "playlist.m3u8", "",
	} {
		if isAceStreamPath(p) {
			t.Fatalf("%q no debería contar como stream", p)
		}
	}
}

func TestWarpStopDelay(t *testing.T) {
	// El APK para WARP a los 60 s sin reproducción; no lo reducimos.
	if warpStopDelay != 60*time.Second {
		t.Fatalf("warpStopDelay = %v, want 60s", warpStopDelay)
	}
}

func TestProxyChainWithWarp(t *testing.T) {
	t.Setenv("PROXY_MODE", "socks5")
	t.Setenv("PROXY_FALLBACK_DIRECT", "1")
	t.Setenv("WARP", "0")
	t.Setenv("WARP_AUTO", "")
	base := proxyChainWithDirectFallback()
	for _, tr := range base {
		if tr == "warp" {
			t.Fatal("warp no debe entrar en la cadena si está deshabilitado")
		}
	}
	t.Setenv("WARP", "1")
	withWarp := proxyChainWithDirectFallback()
	if len(withWarp) != len(base)+1 {
		t.Fatalf("len = %d, want %d", len(withWarp), len(base)+1)
	}
	if withWarp[0] != "warp" {
		t.Fatalf("warp debe ir primero: %v", withWarp)
	}
	// "direct" sigue siendo el último y no se duplica.
	if withWarp[len(withWarp)-1] != "direct" {
		t.Fatalf("último transporte = %q", withWarp[len(withWarp)-1])
	}
	nDirect := 0
	for _, tr := range withWarp {
		if tr == "direct" {
			nDirect++
		}
	}
	if nDirect != 1 {
		t.Fatalf("direct aparece %d veces: %v", nDirect, withWarp)
	}
}
func jsonUnmarshalForTest(data []byte, v any) error { return json.Unmarshal(data, v) }
