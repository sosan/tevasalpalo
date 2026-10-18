package app

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
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"tevasalpalo/internal/telemetry"
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
	// 15s, no 5: el primer request tras un arranque en frío tiene que negociar
	// TLS y resolver DNS, y eso se come los primeros segundos. Con 5s fallaba
	// de forma intermitente justo en el arranque, que es cuando más lo
	// necesitas. Una vez caliente, la misma petición responde en ~100 ms.
	cfg.timeout = time.Duration(telemetry.EnvInt("INFISICAL_TIMEOUT_MS", 15000)) * time.Millisecond

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
// intentosSecrets es cuántas veces se reintenta la petición. El fallo que se
// vio en producción no era permanente: un arranque en frío agotaba los 5s de
// timeout y el startup se quedaba sin telemetría central, cuando dos segundos
// después la misma petición respondía bien.
const intentosSecrets = 3

func (c *infisicalClient) secrets() (map[string]string, error) {
	var ultimo error
	for intento := 1; intento <= intentosSecrets; intento++ {
		m, err := c.secretsUnaVez()
		if err == nil {
			return m, nil
		}
		ultimo = err

		// Solo se reintenta lo transitorio: timeouts, DNS, conexión, y los
		// status que el servidor declara reintentables (429, 5xx). Un 401 o un
		// 404 son definitivos y reintentar solo retrasa el arranque.
		if !reintentableInfisical(err) {
			return nil, err
		}
		if intento < intentosSecrets {
			// Espera creciente pero corta: en un arranque conviene no tardar
			// más de un par de segundos por el secreto.
			espera := time.Duration(intento) * 700 * time.Millisecond
			log.Printf("🔑 Infisical: intento %d/%d falló (%v), reintento en %v",
				intento, intentosSecrets, err, espera)
			time.Sleep(espera)
		}
	}
	return nil, ultimo
}

// reintentableInfisical decide si un error merece otro intento.
func reintentableInfisical(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *infisicalHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.reintentable()
	}
	// timeouts y errores de transporte de net/http.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return strings.Contains(err.Error(), "no se pudo contactar") ||
		strings.Contains(err.Error(), "no se pudo leer la respuesta")
}

// secretsUnaVez hace la petición sin reintentos.
func (c *infisicalClient) secretsUnaVez() (map[string]string, error) {
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

// infisicalHTTPError marca un error que viene de un status HTTP concreto, para
// poder distinguirlo de un fallo de red: un 401 no mejora reintentando, un
// timeout sí.
type infisicalHTTPError struct {
	status  int
	message string
}

// reintentable dice si un status HTTP merece otro intento. Un 5xx o un 429 son
// del servidor y pueden pasar; un 4xx (401 token malo, 404 ruta equivocada) es
// definitivo y reintentar solo retrasa el arranque.
func (e *infisicalHTTPError) reintentable() bool {
	return e.status == http.StatusTooManyRequests ||
		e.status == http.StatusRequestTimeout ||
		e.status >= 500
}

func (e *infisicalHTTPError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("Infisical respondió %d", e.status)
	}
	return fmt.Sprintf("Infisical respondió %d: %s", e.status, e.message)
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
	return &infisicalHTTPError{status: status, message: detalle}
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
// perderían. Se guarda en telemetría (AvisoDiferido) y InitTelemetry lo emite cuando
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
		telemetry.AvisoDiferido(fmt.Sprintf("⚠️  Infisical: %v (sigo con la configuración local)", err))
		return false
	}

	aplicadas := applySecrets(secrets)
	if len(aplicadas) == 0 {
		telemetry.AvisoDiferido(fmt.Sprintf("🔑 Infisical: %d secretos descargados, ninguno aplicaba", len(secrets)))
		return false
	}
	telemetry.AvisoDiferido(fmt.Sprintf("🔑 Infisical: %d secretos de %s (%s) aplicados: %s",
		len(aplicadas), cfg.pathHint(), cfg.env, strings.Join(aplicadas, ", ")))
	return true
}

