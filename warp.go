package main

// Cloudflare WARP como transporte más, portado del cliente Android de
// Ace Stream Pro (WarpManager / WarpRegistration / WarpVpnService,classes6.dex).
//
// En el APK el túnel es un dispositivo TUN de Android: WarpVpnService levanta
// VpnService y el backend wireguard-go enruta TODO el tráfico de la app. Aquí no
// hay TUN ni permisos de administrador: se reutiliza el outbound wireguard de
// xray-core (ya embebido, ver xray.go) sobre un inbound SOCKS en localhost,
// igual que MaybeRunXray. Así WARP es un transporte más de la cadena existente
// y, si algo falla, la app sigue con Tor/directa: nunca es fatal.
//
// Lo portado del APK:
//   - Registro en api.cloudflareclient.com/v0a<build>/reg con cabeceras
//     CF-Client-Version y User-Agent okhttp, y cuerpo
//     {key, install_id, fcm_token, tos, model, serial_number, locale}.
//   - reserved = base64(config.client_id): 3 bytes, el identificador de cliente
//     que Cloudflare espera en el handshake WireGuard.
//   - Endpoint 162.159.192.x con el puerto del array "ports" (2408 primero),
//     porque el campo v4 de /reg viene con ":0".
//   - Ciclo on-demand: se levanta al entrar un stream y para a los 60 s sin
//     reproducción; el temporizador se cancela si vuelve a haber stream.
//
// La identidad (clave privada) se guarda en warp/account.json junto al
// ejecutable: credencial local de este dispositivo, nunca del repo.
//
// Opt-in: WARP=1 habilita el túnel; WARP_AUTO=1 lo habilita sin marcar.
// WARP=0 desactiva. Ver warpEnabled().

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/core"
	"golang.org/x/crypto/curve25519"
)

const (
	warpRegURL      = "https://api.cloudflareclient.com/v0a2158/reg"
	warpClientVer   = "a-6.10-2158"
	warpUserAgent   = "okhttp/3.12.1"
	warpDirName     = "warp"
	warpAccountFile = "account.json"
	warpRegTimeout  = 25 * time.Second
	// MTU 1280 y keepalive 25 s: los mismos valores que genera WarpVpnService.
	warpMTU       = 1280
	warpKeepAlive = 25
	warpStopDelay = 60 * time.Second
)

// ---------------------------------------------------------------------------
// Identidad
// ---------------------------------------------------------------------------

// warpIdentity es lo que se persiste en disco y lo que devuelve /reg.
type warpIdentity struct {
	PrivateKey   string    `json:"private_key"`
	PeerPublic   string    `json:"peer_public_key"`
	Reserved     []byte    `json:"reserved"`
	AddressV4    string    `json:"address_v4"`
	AddressV6    string    `json:"address_v6"`
	EndpointHost string    `json:"endpoint_host"`
	EndpointV4   string    `json:"endpoint_v4"`
	Ports        []int     `json:"ports"`
	Token        string    `json:"token"`
	DeviceID     string    `json:"device_id"`
	AccountType  string    `json:"account_type"`
	License      string    `json:"license"`
	WarpPlus     bool      `json:"warp_plus"`
	Registered   time.Time `json:"registered"`
}

// warpAccount es el JSON de /reg: solo los campos que nos interesan.
type warpAccount struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Account struct {
		AccountType string `json:"account_type"`
		License     string `json:"license"`
		WarpPlus    bool   `json:"warp_plus"`
	} `json:"account"`
	Config struct {
		ClientID  string `json:"client_id"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
		Peers []struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				Host  string `json:"host"`
				V4    string `json:"v4"`
				V6    string `json:"v6"`
				Ports []int  `json:"ports"`
			} `json:"endpoint"`
		} `json:"peers"`
	} `json:"config"`
}

// warpEnabled decide si WARP entra en la cadena. Opt-in, como PROXY_MEDIA:
// nada de tocar rutas del sistema ni activarse por defecto.
func warpEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WARP"))) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	}
	return parseBoolEnv("WARP_AUTO", false)
}

// warpSocksAddr es el inbound SOCKS que publica el túnel.
func warpSocksAddr() string {
	if addr := strings.TrimSpace(os.Getenv("WARP_SOCKS_ADDR")); addr != "" {
		return addr
	}
	return "127.0.0.1:10809"
}

func warpSocksListen() (string, int, error) {
	host, portStr, err := net.SplitHostPort(warpSocksAddr())
	if err != nil {
		return "", 0, fmt.Errorf("WARP_SOCKS_ADDR inválido %q (formato host:port)", warpSocksAddr())
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("WARP_SOCKS_ADDR con puerto inválido %q", portStr)
	}
	return host, port, nil
}

func warpAccountPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exePath), warpDirName, warpAccountFile), nil
}

// warpRandString genera install_id (22) y el relleno del fcm_token (134) con el
// mismo alfabeto que usa el cliente Android.
const warpAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func warpRandString(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(warpAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = warpAlphabet[v.Int64()]
	}
	return string(out), nil
}

// warpNewKeyPair genera una clave Curve25519 y devuelve (privada, pública) en
// base64, como espera /reg.
func warpNewKeyPair() (priv, pub string, err error) {
	var key [32]byte
	if _, err = rand.Read(key[:]); err != nil {
		return "", "", err
	}
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64
	pubBytes, err := curve25519.X25519(key[:], curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(key[:]), base64.StdEncoding.EncodeToString(pubBytes), nil
}

// ---------------------------------------------------------------------------
// Registro en api.cloudflareclient.com
// ---------------------------------------------------------------------------

// warpRegisterBody arma el cuerpo de /reg. fcm_token es el install_id con el
// prefijo que espera el cliente oficial.
func warpRegisterBody(publicKey, installID string) (map[string]string, error) {
	if publicKey == "" || installID == "" {
		return nil, fmt.Errorf("warp: clave pública o install_id vacío")
	}
	suffix, err := warpRandString(134)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"key":           publicKey,
		"install_id":    installID,
		"fcm_token":     installID + ":APA91b" + suffix,
		"tos":           time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"model":         "PC",
		"serial_number": installID,
		"locale":        "en_US",
	}, nil
}

// isAceStreamPath marca las rutas de /ace/ que sirven datos de reproducción.
// Es la traducción a Go de _is_stream_path del parche Python del APK
// (assets/engine/main.py), que intercepta SimpleServer.do_GET y dispara
// droid.onEvent("http_stream_start"/"http_stream_stop"). Los manifests y los
// comandos del webui no cuentan como stream.
func isAceStreamPath(acePath string) bool {
	p := strings.ToLower(acePath)
	for _, k := range []string{
		"getstream", "content/", "hls/", "manifest", "server/stream",
	} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// hostOnly quita el ":0" (o cualquier puerto) que Cloudflare devuelve en v4.
func hostOnly(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		return h
	}
	return strings.Trim(hostPort, "[]")
}

// warpIdentityFromAccount traduce la respuesta de /reg a una identidad usable.
// Replica WarpRegistration del APK: reserved sale de client_id (3 bytes) y el
// endpoint se reconstruye porque el campo v4 viene con puerto 0.
func warpIdentityFromAccount(acc warpAccount, privateKey string) (*warpIdentity, error) {
	if len(acc.Config.Peers) == 0 {
		return nil, fmt.Errorf("warp: /reg sin peers")
	}
	peer := acc.Config.Peers[0]
	if peer.PublicKey == "" {
		return nil, fmt.Errorf("warp: peer sin public_key")
	}
	reserved, err := base64.StdEncoding.DecodeString(acc.Config.ClientID)
	if err != nil {
		return nil, fmt.Errorf("warp: client_id inválido: %w", err)
	}
	if len(reserved) != 3 {
		return nil, fmt.Errorf("warp: reserved debe tener 3 bytes, tiene %d", len(reserved))
	}
	v4 := strings.TrimSpace(acc.Config.Interface.Addresses.V4)
	if v4 == "" {
		return nil, fmt.Errorf("warp: /reg sin address_v4")
	}
	return &warpIdentity{
		PrivateKey:   privateKey,
		PeerPublic:   peer.PublicKey,
		Reserved:     reserved,
		AddressV4:    v4,
		AddressV6:    strings.TrimSpace(acc.Config.Interface.Addresses.V6),
		EndpointHost: peer.Endpoint.Host,
		EndpointV4:   hostOnly(peer.Endpoint.V4),
		Ports:        peer.Endpoint.Ports,
		Token:        acc.Token,
		DeviceID:     acc.ID,
		AccountType:  acc.Account.AccountType,
		License:      acc.Account.License,
		WarpPlus:     acc.Account.WarpPlus,
		Registered:   time.Now().UTC(),
	}, nil
}

// warpEndpointCandidates son los host:puerto a probar, en orden: la IP v4 que
// da /reg con 2408, los puertos alternativos que también lista Cloudflare, y
// por último el host con nombre.
func warpEndpointCandidates(ident *warpIdentity) []string {
	var out []string
	seen := map[string]bool{}
	add := func(host string, port int) {
		if host == "" || port <= 0 || port > 65535 {
			return
		}
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		if seen[addr] {
			return
		}
		seen[addr] = true
		out = append(out, addr)
	}
	if ident.EndpointV4 != "" {
		add(ident.EndpointV4, 2408)
		for _, p := range ident.Ports {
			add(ident.EndpointV4, p)
		}
	}
	if h, p, err := net.SplitHostPort(ident.EndpointHost); err == nil {
		if port, err := strconv.Atoi(p); err == nil {
			add(h, port)
		}
	} else if ident.EndpointHost != "" {
		add(ident.EndpointHost, 2408)
	}
	return out
}

// warpLink construye el esquema wireguard:// que ya sabe leer parseXrayLink
// (mismo formato que un enlace wireguard de subscription, sin tocar el parser).
func warpLink(ident *warpIdentity, endpoint string) (string, error) {
	if len(ident.Reserved) != 3 {
		return "", fmt.Errorf("warp: reserved debe tener 3 bytes, tiene %d", len(ident.Reserved))
	}
	addrs := []string{ident.AddressV4}
	if ident.AddressV6 != "" {
		addrs = append(addrs, ident.AddressV6)
	}
	q := url.Values{}
	q.Set("publickey", ident.PeerPublic)
	q.Set("reserved", base64.StdEncoding.EncodeToString(ident.Reserved))
	q.Set("address", strings.Join(addrs, ","))
	q.Set("mtu", strconv.Itoa(warpMTU))
	q.Set("keepalive", strconv.Itoa(warpKeepAlive))
	// WARP+ usa la licencia del account como pre-shared key.
	if ident.WarpPlus && ident.License != "" {
		q.Set("presharedkey", ident.License)
	}
	return fmt.Sprintf("wireguard://%s@%s?%s#warp",
		url.PathEscape(ident.PrivateKey), endpoint, q.Encode()), nil
}

// ---------------------------------------------------------------------------
// Identidad en disco
// ---------------------------------------------------------------------------

func readWarpAccount(path string) (*warpIdentity, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ident warpIdentity
	if err := json.Unmarshal(raw, &ident); err != nil {
		return nil, err
	}
	if ident.PrivateKey == "" || ident.PeerPublic == "" || len(ident.Reserved) != 3 {
		return nil, fmt.Errorf("warp: identidad incompleta en %s", path)
	}
	return &ident, nil
}

func writeWarpAccount(path string, ident *warpIdentity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ident, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0600)
}

// registerWarp hace el POST a /reg y devuelve una identidad nueva.
func registerWarp() (*warpIdentity, error) {
	priv, pub, err := warpNewKeyPair()
	if err != nil {
		return nil, fmt.Errorf("warp: no se pudo generar la clave: %w", err)
	}
	installID, err := warpRandString(22)
	if err != nil {
		return nil, err
	}
	body, err := warpRegisterBody(pub, installID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", warpRegURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", warpUserAgent)
	req.Header.Set("CF-Client-Version", warpClientVer)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := (&http.Client{Timeout: warpRegTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("warp: registro falló: %w", err)
	}
	defer resp.Body.Close()
	respRaw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("warp: no se pudo leer la respuesta: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("warp: /reg status %d: %s", resp.StatusCode, strings.TrimSpace(string(respRaw)))
	}
	var acc warpAccount
	if err := json.Unmarshal(respRaw, &acc); err != nil {
		return nil, fmt.Errorf("warp: respuesta ilegible: %w", err)
	}
	ident, err := warpIdentityFromAccount(acc, priv)
	if err != nil {
		return nil, err
	}
	log.Printf("🛡️  WARP registrado: %s (%s) IP %s", ident.DeviceID, ident.AccountType, ident.AddressV4)
	return ident, nil
}

// loadOrRegisterWarp reutiliza la identidad guardada; si no hay o está
// corrupta, registra una nueva y la persiste.
func loadOrRegisterWarp() (*warpIdentity, error) {
	if path, err := warpAccountPath(); err == nil {
		if ident, err := readWarpAccount(path); err == nil {
			return ident, nil
		}
	}
	ident, err := registerWarp()
	if err != nil {
		return nil, err
	}
	if path, err := warpAccountPath(); err == nil {
		if err := writeWarpAccount(path, ident); err != nil {
			log.Printf("⚠️  WARP: no se pudo guardar la identidad: %v", err)
		}
	}
	return ident, nil
}

// ---------------------------------------------------------------------------
// Ciclo de vida on-demand
// ---------------------------------------------------------------------------

// warpHandle es el túnel WARP levantado, con su SOCKS en localhost.
type warpHandle struct {
	instance  *core.Instance
	desc      string
	socksAddr string
}

// warpState coordina el túnel: identidad cacheada en memoria, instancia activa,
// streams en vuelo y el temporizador de parada diferida.
type warpState struct {
	mu     sync.Mutex
	ident  *warpIdentity
	handle *warpHandle
	timer  *time.Timer

	streamsMu sync.Mutex
	streams   int
}

var warp warpState

// WarpStreamStart la llama el servidor web al entrar una petición de stream.
// Equivale a ensureRunning() de WarpManager.
func WarpStreamStart() {
	if !warpEnabled() {
		return
	}
	warp.streamsMu.Lock()
	warp.streams++
	first := warp.streams == 1
	warp.streamsMu.Unlock()

	warp.mu.Lock()
	if warp.timer != nil {
		warp.timer.Stop()
		warp.timer = nil
	}
	running := warp.handle != nil
	warp.mu.Unlock()

	if running || !first {
		// Ya está en marcha, o el arranque ya lo pidió otro stream.
		return
	}
	go func() {
		h, err := startWarp()
		if err != nil {
			log.Printf("⚠️  WARP no disponible, se continúa sin él: %v", err)
			return
		}
		log.Printf("🛡️  WARP activo: %s vía SOCKS %s", h.desc, h.socksAddr)
	}()
}

// WarpStreamStop la llama el servidor web al terminar un stream. Sin
// reproducción durante warpStopDelay (60 s, como en el APK) el túnel se para.
func WarpStreamStop() {
	if !warpEnabled() {
		return
	}
	warp.streamsMu.Lock()
	if warp.streams > 0 {
		warp.streams--
	}
	idle := warp.streams == 0
	warp.streamsMu.Unlock()

	if !idle {
		return
	}
	warp.mu.Lock()
	if warp.handle == nil || warp.timer != nil {
		warp.mu.Unlock()
		return
	}
	warp.timer = time.AfterFunc(warpStopDelay, func() {
		warp.streamsMu.Lock()
		stillIdle := warp.streams == 0
		warp.streamsMu.Unlock()
		if !stillIdle {
			// Volvió la reproducción: el túnel se queda.
			return
		}
		warp.mu.Lock()
		h := warp.handle
		warp.handle = nil
		warp.timer = nil
		warp.mu.Unlock()
		if h != nil {
			log.Println("🛡️  WARP sin reproducción 60s, parando túnel")
			_ = StopWarp(h)
		}
	})
	warp.mu.Unlock()
}

// startWarp registra si hace falta y levanta el túnel con el primer endpoint
// que pase la prueba de tráfico real.
func startWarp() (*warpHandle, error) {
	ident, err := loadOrRegisterWarp()
	if err != nil {
		return nil, err
	}
	listen, port, err := warpSocksListen()
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(listen, strconv.Itoa(port))
	for _, ep := range warpEndpointCandidates(ident) {
		link, err := warpLink(ident, ep)
		if err != nil {
			log.Printf("⚠️  WARP: config inválida para %s: %v", ep, err)
			continue
		}
		endpoint, err := parseXrayLink(link)
		if err != nil {
			log.Printf("⚠️  WARP: enlace inválido para %s: %v", ep, err)
			continue
		}
		inst, err := startXrayInstance(endpoint, listen, port)
		if err != nil {
			log.Printf("⚠️  WARP %s no arranca: %v", ep, err)
			continue
		}
		if !probeEndpoint(addr) {
			log.Printf("⚠️  WARP %s sin tráfico, probando siguiente...", ep)
			_ = inst.Close()
			continue
		}
		h := &warpHandle{instance: inst, desc: "wireguard " + ep, socksAddr: addr}
		warp.mu.Lock()
		warp.ident = ident
		warp.handle = h
		warp.mu.Unlock()
		return h, nil
	}
	return nil, fmt.Errorf("ningún endpoint WARP responde")
}

// StopWarp para el túnel indicado (nil-safe).
func StopWarp(h *warpHandle) error {
	if h == nil || h.instance == nil {
		return nil
	}
	return h.instance.Close()
}

// StopWarpIfRunning para el túnel activo del proceso, si lo hay, y cancela el
// temporizador de parada. Lo llama el cierre de la app.
func StopWarpIfRunning() error {
	warp.mu.Lock()
	h := warp.handle
	warp.handle = nil
	if warp.timer != nil {
		warp.timer.Stop()
		warp.timer = nil
	}
	warp.mu.Unlock()
	if h == nil {
		return nil
	}
	log.Println("🛡️  Cerrando WARP")
	return StopWarp(h)
}
