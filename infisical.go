package main

// Lectura de secretos desde Infisical.
//
// Para qué: rotar el token de Grafana sin publicar un binario nuevo. Se
// cambia en la consola y todas las instalaciones lo recogen en el siguiente
// arranque.
//
// Lo que NO hace: incrustar el token de Infisical. Ese token es una
// credencial de lectura y, si fuera dentro del binario, `strings portable.exe`
// lo sacaría junto con todo lo que hay detrás. El token de Infisical se
// considera "tu" configuración local (entorno o settings.json); lo que viaja a
// Infisical es el token de Grafana, que ya vive en un gestor de secretos.
//
// API: v4 (v1/v2/v3 están deprecadas). El projectId no va en el token, hay que
// configurarlo aparte, igual que la URL del proyecto.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const infisicalDefaultURL = "https://app.infisical.com"

type infisicalConfig struct {
	url       string
	token     string
	projectID string
	env       string
	path      string
	timeout   time.Duration
}

type infisicalClient struct {
	cfg    infisicalConfig
	client *http.Client
}

// secret de Infisical, tal y lo devuelve la v4.
type infisicalSecret struct {
	Key         string `json:"secretKey"`
	Value       string `json:"secretValue"`
	Environment string `json:"environment"`
	SecretPath  string `json:"secretPath"`
}

// loadInfisicalConfig lee la configuración. Todas las variables son
// obligatorias salvo la URL, que tiene default.
func loadInfisicalConfig() (infisicalConfig, bool) {
	cfg := infisicalConfig{
		url:       strings.TrimRight(strings.TrimSpace(os.Getenv("INFISICAL_URL")), "/"),
		token:     strings.TrimSpace(os.Getenv("INFISICAL_TOKEN")),
		projectID: strings.TrimSpace(os.Getenv("INFISICAL_PROJECT_ID")),
		env:       strings.TrimSpace(os.Getenv("INFISICAL_ENV")),
		path:      strings.TrimSpace(os.Getenv("INFISICAL_PATH")),
	}
	if cfg.url == "" {
		cfg.url = infisicalDefaultURL
	}
	if cfg.path == "" {
		cfg.path = "/"
	}
	cfg.timeout = time.Duration(envIntTel("INFISICAL_TIMEOUT_MS", 5000)) * time.Millisecond

	// Sin token, project ID o entorno no se puede pedir nada. No es un error:
	// la app funciona igual sin Infisical.
	if cfg.token == "" || cfg.projectID == "" || cfg.env == "" {
		return cfg, false
	}
	return cfg, true
}

func newInfisicalClient(cfg infisicalConfig) *infisicalClient {
	return &infisicalClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.timeout},
	}
}

// secrets descarga todos los secretos de la ruta configurada. Devuelve un mapa
// clave→valor para pasarlo a applySecrets.
func (c *infisicalClient) secrets() (map[string]string, error) {
	q := url.Values{}
	q.Set("projectId", c.cfg.projectID)
	q.Set("environment", c.cfg.env)
	q.Set("secretPath", c.cfg.path)

	req, err := http.NewRequest("GET",
		c.cfg.url+"/api/v4/secrets?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo contactar con Infisical: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer la respuesta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.infisicalError(resp.StatusCode, body)
	}

	var out struct {
		Secrets []infisicalSecret `json:"secrets"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("respuesta inesperada de Infisical: %w", err)
	}

	m := make(map[string]string, len(out.Secrets))
	for _, s := range out.Secrets {
		if s.Key != "" {
			m[s.Key] = s.Value
		}
	}
	return m, nil
}

// infisicalError traduce los errores que de verdad se dan al montar esto, para
// que el log diga qué corregir en vez de soltar un 404 pelado.
func (c *infisicalClient) infisicalError(status int, body []byte) error {
	var v struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(body, &v)

	detalle := strings.TrimSpace(v.Message)
	if detalle == "" {
		detalle = strings.TrimSpace(string(body))
	}

	switch {
	case status == 401 || status == 403:
		return fmt.Errorf("Infisical rechazó el token (%d): %s. "+
			"Comprueba INFISICAL_TOKEN y que el token tenga acceso a %s en %s",
			status, detalle, c.cfg.pathHint(), c.cfg.env)
	case status == 404:
		return fmt.Errorf("Infisical: %s. Crea la carpeta %s en el entorno %s, "+
			"o corrige INFISICAL_PATH / INFISICAL_ENV", detalle, c.cfg.pathHint(), c.cfg.env)
	}
	return fmt.Errorf("Infisical respondió %d: %s", status, detalle)
}

// pathHint es la ruta tal y como seConfigured, para los mensajes de error.
func (cfg infisicalConfig) pathHint() string {
	if cfg.path == "" {
		return "/"
	}
	return cfg.path
}

// applySecrets mete los secretos en el entorno del proceso, pero solo los que
// todavía no estén definidos: lo explícito del usuario gana sobre lo que venga
// de Infisical. Así se puede probar contra un Loki propio sin tocar nada.
//
// Devuelve los nombres de las claves que se aplicaron, para el log de arranque.
func applySecrets(secrets map[string]string) []string {
	var aplicadas []string
	for k, v := range secrets {
		if v == "" {
			continue
		}
		if _, ya := os.LookupEnv(k); ya {
			continue
		}
		if err := os.Setenv(k, v); err == nil {
			aplicadas = append(aplicadas, k)
		}
	}
	return aplicadas
}

// FetchSecretsFromInfisical es el punto de entrada que se llama al arrancar,
// antes de InitTelemetry. Si no hay Infisical configurado, no hace nada y
// devuelve false: la telemetría se queda con lo que haya en el entorno.
//
// El resultado no se loguea aquí a propósito: en este punto el log global
// todavía no está redirigido (InitTelemetry corre después) y estos mensajes se
// perderían. Se guardan en infisicalStatus y InitTelemetry los emite cuando el
// log ya está montado.
func FetchSecretsFromInfisical() bool {
	cfg, ok := loadInfisicalConfig()
	if !ok {
		return false
	}

	cl := newInfisicalClient(cfg)
	secrets, err := cl.secrets()
	if err != nil {
		// Un fallo aquí no debe impedir arrancar la app: la telemetría se
		// queda sin central, pero el usuario puede seguir viendo el vídeo.
		setInfisicalStatus(fmt.Sprintf("⚠️  Infisical: %v (sigo con la configuración local)", err))
		return false
	}

	aplicadas := applySecrets(secrets)
	if len(aplicadas) == 0 {
		setInfisicalStatus(fmt.Sprintf("🔑 Infisical: %d secretos descargados, ninguno aplicaba", len(secrets)))
		return false
	}
	setInfisicalStatus(fmt.Sprintf("🔑 Infisical: %d secretos de %s (%s) aplicados: %s",
		len(aplicadas), cfg.pathHint(), cfg.env, strings.Join(aplicadas, ", ")))
	return true
}

// infisicalStatus guarda los mensajes hasta que el log está montado.
var (
	infisicalMu      sync.Mutex
	infisicalMensaje string
)

func setInfisicalStatus(m string) {
	infisicalMu.Lock()
	infisicalMensaje = m
	infisicalMu.Unlock()
}

func takeInfisicalStatus() string {
	infisicalMu.Lock()
	defer infisicalMu.Unlock()
	m := infisicalMensaje
	infisicalMensaje = ""
	return m
}
