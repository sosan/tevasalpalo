package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func infisicalServer(t *testing.T, status int, cuerpo string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer tok-123" {
			w.WriteHeader(401)
			w.Write([]byte(`{"message":"bad token"}`))
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(cuerpo))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInfisicalLeeLosSecretos(t *testing.T) {
	srv := infisicalServer(t, 200,
		`{"secrets":[{"secretKey":"TELEMETRY_PASSWORD","secretValue":"glc_abc","environment":"prod","secretPath":"/streaming"},{"secretKey":"TELEMETRY_USER","secretValue":"251374","environment":"prod","secretPath":"/streaming"}]}`)

	cl := &infisicalClient{
		cfg: infisicalConfig{
			url: srv.URL, token: "tok-123",
			projectID: "p1", env: "prod", path: "/streaming",
			timeout: 2000000000,
		},
		client: srv.Client(),
	}

	secrets, err := cl.secrets()
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if secrets["TELEMETRY_PASSWORD"] != "glc_abc" {
		t.Errorf("TELEMETRY_PASSWORD = %q", secrets["TELEMETRY_PASSWORD"])
	}
	if secrets["TELEMETRY_USER"] != "251374" {
		t.Errorf("TELEMETRY_USER = %q", secrets["TELEMETRY_USER"])
	}
}

func TestInfisicalMandaProjectIdYEntorno(t *testing.T) {
	var q string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.RawQuery
		w.Write([]byte(`{"secrets":[]}`))
	}))
	defer srv.Close()

	cl := &infisicalClient{
		cfg: infisicalConfig{
			url: srv.URL, token: "t", projectID: "proj-abc",
			env: "prod", path: "/streaming",
		},
		client: srv.Client(),
	}
	cl.secrets()

	for _, quiero := range []string{"projectId=proj-abc", "environment=prod", "secretPath=%2Fstreaming"} {
		if !strings.Contains(q, quiero) {
			t.Errorf("la query %q no contiene %q", q, quiero)
		}
	}
}

// Lo definido en el entorno gana sobre lo que venga de Infisical: si el
// usuario apunta a otro Loki a mano, no se le pisa debajo.
func TestApplySecretsNoPisaElEntorno(t *testing.T) {
	t.Setenv("YA_PUESTO", "del-entorno")
	t.Setenv("NO_TOCAR", "original")
	os.Unsetenv("NUEVO")

	aplicadas := applySecrets(map[string]string{
		"YA_PUESTO": "de-infisical",
		"NO_TOCAR":  "de-infisical",
		"NUEVO":     "de-infisical",
		"VACIO":     "",
	})

	if os.Getenv("YA_PUESTO") != "del-entorno" {
		t.Errorf("pisó una variable ya definida: %q", os.Getenv("YA_PUESTO"))
	}
	if os.Getenv("NO_TOCAR") != "original" {
		t.Errorf("pisó NO_TOCAR: %q", os.Getenv("NO_TOCAR"))
	}
	if os.Getenv("NUEVO") != "de-infisical" {
		t.Errorf("no aplicó la nueva: %q", os.Getenv("NUEVO"))
	}
	if _, existe := os.LookupEnv("VACIO"); existe {
		t.Error("aplicó un secreto vacío")
	}

	// Solo debería haber aplicado NUEVO.
	if len(aplicadas) != 1 || aplicadas[0] != "NUEVO" {
		t.Errorf("aplicadas = %v, se esperaba [NUEVO]", aplicadas)
	}
}

// Sin token o sin project ID no se llama a la red: la app tiene que arrancar
// igual para un usuario que no usa Infisical.
func TestInfisicalSinConfigNoHaceNada(t *testing.T) {
	os.Unsetenv("INFISICAL_TOKEN")
	os.Unsetenv("INFISICAL_PROJECT_ID")
	os.Unsetenv("INFISICAL_ENV")

	cfg, ok := loadInfisicalConfig()
	if ok {
		t.Error("loadInfisicalConfig dijo que sí sin token ni project ID")
	}
	if cfg.url != infisicalDefaultURL {
		t.Errorf("la URL por defecto es %q", cfg.url)
	}
	if cfg.path != "/" {
		t.Errorf("la ruta por defecto es %q", cfg.path)
	}
}

// El error debe decir qué corregir. Un 404 pelado no ayuda a nadie.
func TestErroresDeInfisicalSonUtiles(t *testing.T) {
	casos := []struct {
		nombre string
		status int
		cuerpo string
		quiere []string
	}{
		{
			nombre: "401 dice que revise el token",
			status: 401,
			cuerpo: `{"message":"Unauthorized"}`,
			quiere: []string{"token", "/streaming", "prod"},
		},
		{
			nombre: "404 dice que cree la carpeta",
			status: 404,
			cuerpo: `{"error":"SecretPathNotFound","message":"Folder not found"}`,
			quiere: []string{"carpeta", "/streaming", "INFISICAL_PATH"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cl := &infisicalClient{cfg: infisicalConfig{path: "/streaming", env: "prod"}}
			err := cl.infisicalError(c.status, []byte(c.cuerpo))
			if err == nil {
				t.Fatal("no devolvió error")
			}
			for _, w := range c.quiere {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("el error no menciona %q: %v", w, err)
				}
			}
		})
	}
}

func TestInfisicalAceptaSecretosVacios(t *testing.T) {
	srv := infisicalServer(t, 200, `{"secrets":[{"secretKey":"A"},{"secretValue":"B"}]}`)
	cl := &infisicalClient{
		cfg:    infisicalConfig{url: srv.URL, token: "tok-123", projectID: "p", env: "prod", path: "/streaming"},
		client: srv.Client(),
	}
	secrets, err := cl.secrets()
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(secrets) != 1 || secrets["A"] != "" {
		t.Errorf("secrets = %v, solo debía quedar A con valor vacío", secrets)
	}
}

// El token de Infisical es una credencial de lectura: si acabara incrustado en
// el binario, cualquiera con el portable podría leer todo lo que hay detrás.
// Este test falla si alguien lo mete como valor por defecto en el código.
func TestElTokenDeInfisicalNoEstaEnElCodigo(t *testing.T) {
	prohibido := "st.26028f0b" // prefijo del service token de este proyecto
	for _, f := range []string{"infisical.go", "telemetry.go", "main.go", "server.go"} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		if strings.Contains(string(raw), prohibido) {
			t.Errorf("%s contiene un service token de Infisical hardcodeado", f)
		}
		if strings.Contains(string(raw), "glc_") {
			t.Errorf("%s contiene un token de Grafana hardcodeado", f)
		}
	}
}

// Si la respuesta no es el JSON esperado, que lo diga claro.
func TestInfisicalRespuestaRara(t *testing.T) {
	srv := infisicalServer(t, 200, `esto no es json`)
	cl := &infisicalClient{
		cfg:    infisicalConfig{url: srv.URL, token: "tok-123", projectID: "p", env: "prod", path: "/streaming"},
		client: srv.Client(),
	}
	if _, err := cl.secrets(); err == nil {
		t.Fatal("debería fallar con una respuesta no-JSON")
	} else if !strings.Contains(err.Error(), "Infisical") {
		t.Errorf("el error no menciona Infisical: %v", err)
	}
}

func TestInfisicalJSONValidoDeReferencia(t *testing.T) {
	// Fija el formato que esperamos de la v4, para detectar cambios upstream.
	var v struct {
		Secrets []infisicalSecret `json:"secrets"`
	}
	err := json.Unmarshal([]byte(`{"secrets":[{"secretKey":"K","secretValue":"V","environment":"prod","secretPath":"/streaming"}]}`), &v)
	if err != nil {
		t.Fatalf("formato: %v", err)
	}
	if v.Secrets[0].Key != "K" || v.Secrets[0].Value != "V" {
		t.Errorf("no parseó bien: %+v", v.Secrets[0])
	}
}

// El fallo real: en un arranque en frío el primer request agota el timeout y la
// app se queda sin telemetría central. Con reintentos, dos segundos después la
// misma petición responde bien.
func TestInfisicalReintentaAnteTimeout(t *testing.T) {
	var intentos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentos++
		if intentos < 3 {
			// Simula lo que pasó: la primera y la segunda se pierden.
			w.WriteHeader(504)
			return
		}
		w.Write([]byte(`{"secrets":[{"secretKey":"TELEMETRY_USER","secretValue":"251374"}]}`))
	}))
	defer srv.Close()

	cl := &infisicalClient{
		cfg:    infisicalConfig{url: srv.URL, token: "t", projectID: "p", env: "prod", path: "/"},
		client: srv.Client(),
	}
	secrets, err := cl.secrets()
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if intentos != 3 {
		t.Errorf("hizo %d intentos, se esperaban 3", intentos)
	}
	if secrets["TELEMETRY_USER"] != "251374" {
		t.Errorf("secrets = %v", secrets)
	}
}

// Un 401 es definitivo: el token está mal. Reintentar tres veces solo retrasa
// el arranque y no arregla nada.
func TestInfisicalNoReintentaAnte401(t *testing.T) {
	var intentos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentos++
		w.WriteHeader(401)
		w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer srv.Close()

	cl := &infisicalClient{
		cfg:    infisicalConfig{url: srv.URL, token: "malo", projectID: "p", env: "prod", path: "/"},
		client: srv.Client(),
	}
	if _, err := cl.secrets(); err == nil {
		t.Fatal("debería fallar")
	}
	if intentos != 1 {
		t.Errorf("hizo %d intentos con un 401, debería hacer 1 solo", intentos)
	}
}

// Agotados los intentos, el error debe seguir siendo útil.
func TestInfisicalAgotaIntentos(t *testing.T) {
	var intentos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentos++
		w.WriteHeader(503)
	}))
	defer srv.Close()

	cl := &infisicalClient{
		cfg:    infisicalConfig{url: srv.URL, token: "t", projectID: "p", env: "prod", path: "/"},
		client: srv.Client(),
	}
	_, err := cl.secrets()
	if err == nil {
		t.Fatal("debería fallar tras agotar los intentos")
	}
	if intentos != intentosSecrets {
		t.Errorf("hizo %d intentos, se esperaban %d", intentos, intentosSecrets)
	}
}

// El timeout por defecto tiene que ser holgado: 5s fallaba en el arranque en
// frío, que es cuando hace falta.
func TestElTimeoutPorDefectoEsHolgado(t *testing.T) {
	os.Unsetenv("INFISICAL_TIMEOUT_MS")
	cfg, ok := loadInfisicalConfig()
	if !ok {
		// Sin token no valida la config; se comprueba el default aparte.
		t.Skip("sin INFISICAL_TOKEN configurado")
	}
	if cfg.timeout < 10*time.Second {
		t.Errorf("timeout por defecto = %v, se esperaba al menos 10s", cfg.timeout)
	}
}
