package app

// Telemetría y logs centralizados.
//
// Idea: no se toca ninguno de los ~120 log.Printf que ya hay. Al arrancar se
// sustituye la salida global de log por un MultiWriter que reparte cada línea
// a tres destinos: stdout (lo que ve el usuario), fichero local rotado (red de
// seguridad si no hay red) y un buffer circular (descarga rápida y diagnóstico).
//
// Aparte, Record() permite eventos estructurados (métricas, errores del player,
// eventos con nombre) que se envían como OTLP a Grafana Cloud Loki si hay
// credenciales, y se quedan en el fichero local si no.
//
// Privacidad: la IP se hashea con salt (TELEMETRY_HASH_SALT) de forma estable
// para poder correlacionar una sesión sin exponer la real. El resto va literal
// a petición explícita: las rutas /ace/... y los errores son justamente lo que
// sirve para depurar.
//
// Todo el envío es asíncrono y nunca bloquea al player: si el central no
// responde, se acumulan y se descartan los más antiguos.

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

type telemetryConfig struct {
	enabled    bool
	otlpURL    string
	user       string
	password   string
	logDir     string
	fileName   string
	eventFile  string
	maxSize    int64
	maxFiles   int
	hashSalt   string
	flushEvery time.Duration
	maxBatch   int
	bufferCap  int
	clientID   string
	exportLogs bool
	// Spool en disco: cola que sobrevive a los reinicios.
	spoolPath    string
	spoolMaxSize int64
}

// eventServerLog es el nombre del evento con el que al central van las líneas
// del log del servidor que no son eventos estructurados.
const eventServerLog = "server_log"

var (
	telMu      sync.RWMutex
	telCfg     telemetryConfig
	telBuf     *ringBuffer
	telRot     *rotatingWriter
	telEvents  *rotatingWriter
	telBatch   *otlpExporter
	telReady   bool
	ipHasher   func(string) string
	telVersion = "dev"
	telOS      = ""
	telArch    = ""
)

func envBoolTel(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on", "si", "sí":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envIntTel(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}

// hashIP devuelve un identificador estable de una IP. El salt permite
// correlacionar todas las peticiones de una misma sesión (para_debug) sin
// exponer la IP real, y sin poder revertirla sin conocer el salt.
func hashIP(raw string) string {
	if raw == "" {
		return ""
	}
	telMu.RLock()
	salt := telCfg.hashSalt
	telMu.RUnlock()
	sum := sha256.Sum256([]byte(salt + "|" + raw))
	return hex.EncodeToString(sum[:8])
}

func loadTelemetryConfig() telemetryConfig {
	cfg := telemetryConfig{
		enabled:      envBoolTel("TELEMETRY_ENABLED", false),
		otlpURL:      strings.TrimSpace(os.Getenv("TELEMETRY_OTLP_URL")),
		user:         strings.TrimSpace(os.Getenv("TELEMETRY_USER")),
		password:     os.Getenv("TELEMETRY_PASSWORD"),
		logDir:       strings.TrimSpace(os.Getenv("TELEMETRY_LOG_DIR")),
		fileName:     "streaming.log",
		eventFile:    "events.jsonl",
		maxSize:      int64(envIntTel("TELEMETRY_LOG_MAX_BYTES", 8<<20)),
		maxFiles:     envIntTel("TELEMETRY_LOG_MAX_FILES", 5),
		hashSalt:     os.Getenv("TELEMETRY_HASH_SALT"),
		flushEvery:   time.Duration(envIntTel("TELEMETRY_FLUSH_MS", 5000)) * time.Millisecond,
		maxBatch:     envIntTel("TELEMETRY_MAX_BATCH", 200),
		bufferCap:    envIntTel("TELEMETRY_BUFFER", 2000),
		clientID:     clientID(),
		exportLogs:   envBoolTel("TELEMETRY_EXPORT_LOGS", true),
		spoolMaxSize: int64(envIntTel("TELEMETRY_SPOOL_BYTES", 16<<20)),
	}
	if dir := cfg.logDir; dir != "" {
		cfg.spoolPath = filepath.Join(dir, "spool.jsonl")
	}
	return cfg
}

// clientID identifica esta instalación ante el central sin mandar nada
// sensible: host + versión. Es lo que permite agrupar "errores de la 1.2.3
// en la versión X" en el visor sin reducirlo a una sola IP.
func clientID() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "desconocido"
	}
	return host + "@" + telVersion
}

// InitTelemetry engancha la telemetría. Safe de llamar aunque esté todo
// deshabilitado: en ese caso solo deja stdout.
func InitTelemetry() {
	telMu.Lock()
	if telReady {
		telMu.Unlock()
		return
	}
	telCfg = loadTelemetryConfig()
	ipHasher = hashIP
	telBuf = newRingBuffer(telCfg.bufferCap)
	telMu.Unlock()

	var writers []io.Writer
	writers = append(writers, os.Stdout)

	// Fichero local: siempre, para tener historial aunque no haya central.
	// Dos ficheros: el texto plano del log y los eventos estructurados, que
	// van en JSON Lines. No se mezclan para que events.jsonl siga siendo
	// parseable línea a línea.
	if dir := telCfg.logDir; dir != "" {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			rw, err := newRotatingWriter(filepath.Join(dir, telCfg.fileName), telCfg.maxSize, telCfg.maxFiles)
			if err == nil {
				telRot = rw
				writers = append(writers, rw)
			} else {
				log.Printf("⚠️  telemetría: no se pudo abrir fichero de log: %v", err)
			}
			if ev, err := newRotatingWriter(filepath.Join(dir, telCfg.eventFile), telCfg.maxSize, telCfg.maxFiles); err == nil {
				telEvents = ev
			} else {
				log.Printf("⚠️  telemetría: no se pudo abrir %s: %v", telCfg.eventFile, err)
			}
		} else {
			log.Printf("⚠️  telemetría: no se pudo crear TELEMETRY_LOG_DIR: %v", err)
		}
	}

	// Buffer en memoria para /api/logs.
	writers = append(writers, telBuf)

	// Copia estructurada de las líneas de log hacia el central. Se pone al
	// final del MultiWriter para que, si falla, no afecte a stdout ni al
	// fichero.
	if telCfg.enabled && telCfg.otlpURL != "" && telCfg.exportLogs {
		writers = append(writers, &logMirror{})
	}

	log.SetOutput(io.MultiWriter(writers...))

	// Exportador asíncrono (solo si hay destino configurado).
	if telCfg.enabled && telCfg.otlpURL != "" {
		exp, err := newOTLPExporter(telCfg)
		if err != nil {
			log.Printf("⚠️  telemetría: exportador no disponible: %v", err)
		} else {
			telBatch = exp
			go exp.run()
		}
	}

	telMu.Lock()
	telReady = true
	telMu.Unlock()

	dest := "fichero local"
	if telBatch != nil {
		dest = "fichero local + central"
	}
	log.Printf("📊 Telemetría activa (%s)", dest)
	if telBatch == nil {
		log.Printf("📊 Sin envío central: define TELEMETRY_ENABLED=1 y TELEMETRY_OTLP_URL para exportar")
	}

	// Los avisos de Infisical se emiten aquí, no en su propia función: cuando
	// se descargan, el log global aún no estaba redirigido y se habrían perdido.
	if m := takeInfisicalStatus(); m != "" {
		log.Print(m)
	}
}

// Record emite un evento estructurado. No bloquea: si el buffer está lleno
// descarta el más antiguo. Pensado para métricas y errores del player.
func Record(event string, fields map[string]any) {
	rec := buildRecord(event, fields)
	// events.jsonl queda siempre parseable aunque no haya central.
	if telEvents != nil {
		if b, err := json.Marshal(rec); err == nil {
			telEvents.WriteLine(b)
		}
	}
	if telBatch != nil {
		telBatch.enqueue(rec)
	}
}

// logMirror reparte las líneas del log global al exportador central. Es un
// writer más del MultiWriter: no altera el comportamiento de los ~120
// log.Printf existentes, solo añade una copia estructurada.
//
// Sin esto, al central solo irían los eventos con nombre del player y se
// perdería justo lo que sirve para depurar el motor (timeouts de /ace, fallos
// de fuente, errores de Xray...).
type logMirror struct {
	mu       sync.Mutex
	last     string
	lastSeen int
}

func (m *logMirror) Write(p []byte) (int, error) {
	if telBatch == nil {
		return len(p), nil
	}
	msg := strings.TrimSpace(stripLogTimestamp(string(p)))
	if msg == "" {
		return len(p), nil
	}

	m.mu.Lock()
	if msg == m.last {
		m.lastSeen++
		n := m.lastSeen
		m.mu.Unlock()
		// Se emite la primera y luego en potencias de dos (2, 4, 8...):
		// sigue viéndose que algo se repite sin inundar el central con las
		// miles de líneas idénticas que genera un bucle de reintentos.
		if !isPowerOfTwo(n) {
			return len(p), nil
		}
		telBatch.enqueue(buildRecord(eventServerLog, map[string]any{
			"nivel":        levelFromMessage(msg),
			"mensaje":      msg,
			"repeticiones": n,
		}))
		return len(p), nil
	}
	m.last, m.lastSeen = msg, 1
	m.mu.Unlock()

	telBatch.enqueue(buildRecord(eventServerLog, map[string]any{
		"nivel":   levelFromMessage(msg),
		"mensaje": msg,
	}))
	return len(p), nil
}

func isPowerOfTwo(n int) bool { return n > 0 && n&(n-1) == 0 }

// levelFromMessage saca el nivel del emoji o de las palabras que ya usan los
// logs del proyecto. No es un parser perfecto, pero evita tener que tocar los
// ~120 log.Printf uno a uno para que Grafana pueda filtrar por ERROR.
func levelFromMessage(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "❌"), strings.Contains(m, "error"), strings.Contains(m, "falló"),
		strings.Contains(m, "fallo"), strings.Contains(m, "fatal"):
		return "ERROR"
	case strings.Contains(m, "⚠️"), strings.Contains(m, "⚠"), strings.Contains(m, "aviso"),
		strings.Contains(m, "warn"):
		return "WARN"
	}
	return "INFO"
}

// stripLogTimestamp quita el prefijo que log.Printf pone por defecto
// ("2026/10/12 17:36:00 ") porque el timestamp ya viaja en el propio registro.
func stripLogTimestamp(line string) string {
	if len(line) < 20 {
		return line
	}
	if line[4] != '/' || line[7] != '/' || line[13] != ':' || line[16] != ':' {
		return line
	}
	return strings.TrimSpace(line[20:])
}

func buildRecord(event string, fields map[string]any) map[string]any {
	telMu.RLock()
	cfg := telCfg
	telMu.RUnlock()
	rec := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339),
		"event":   event,
		"version": telVersion,
		"os":      telOS,
		"arch":    telArch,
	}
	if cfg.clientID != "" {
		rec["client_id"] = cfg.clientID
	}
	for k, v := range fields {
		// No sobreescribir los campos base.
		switch k {
		case "time", "event", "version":
			continue
		}
		rec[k] = v
	}
	return rec
}

// RecordPlayer ingiere eventos que llegan del navegador. Enriquece con la IP
// hasheada del cliente y con la versión/plataforma local.
func RecordPlayer(event string, fields map[string]any, remoteIP string) {
	f := map[string]any{}
	for k, v := range fields {
		f[k] = v
	}
	f["client_ip"] = hashIP(remoteIP)
	f["source"] = "player"
	Record(event, f)
}

// Tope de lo que se acepta por petición en /api/telemetry. El player manda
// lotes de 20; se deja margen pero sin permitir que una racha de errores de
// HLS.js llene la cola central.
const (
	telMaxBody     = 64 << 10
	telMaxEvents   = 50
	telMaxEventLen = 80
	telMaxFieldLen = 60
)

// telAutorizado decide quién puede leer /api/logs: el loopback siempre (es el
// caso normal: el navegador está en la misma máquina) y el resto solo si hay
// TELEMETRY_API_TOKEN configurado. Comparación en tiempo constante para no
// filtrar el token por temporización.
func telAutorizado(c fiber.Ctx) bool {
	token := strings.TrimSpace(os.Getenv("TELEMETRY_API_TOKEN"))
	if token == "" {
		return isLoopbackIP(c.IP())
	}
	got := c.Query("token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
		return true
	}
	h := c.Get("X-Telemetry-Token")
	return subtle.ConstantTimeCompare([]byte(h), []byte(token)) == 1
}

func isLoopbackIP(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "::1" || ip == "0:0:0:0:0:0:0:1" {
		return true
	}
	if strings.HasPrefix(ip, "127.") {
		return true
	}
	if strings.HasPrefix(ip, "::ffff:") {
		return isLoopbackIP(strings.TrimPrefix(ip, "::ffff:"))
	}
	return false
}

// tailRecent devuelve las últimas líneas del buffer (para /api/logs).
func tailRecent(limit int) []string {
	if telBuf == nil {
		return nil
	}
	return telBuf.tail(limit)
}

// ShutdownTelemetry intenta vaciar lo que quede en la cola antes de morir. Sin
// esto, los últimos eventos (y a menudo justo el error que provoca el cierre)
// se pierden
// porque el proceso termina antes del siguiente tick. Best effort: si el
// central no responde, se sigue cerrando igual.
func ShutdownTelemetry(timeout time.Duration) {
	if telBatch == nil {
		closeTelemetryWriters()
		return
	}
	telBatch.closeWithDeadline(time.Now().Add(timeout))
	closeTelemetryWriters()
}

func closeTelemetryWriters() {
	if telRot != nil {
		telRot.Close()
	}
	if telEvents != nil {
		telEvents.Close()
	}
}

// telemetryConfigSnapshot para exponer el estado en /api/logs.
func telemetryConfigSnapshot() map[string]any {
	telMu.RLock()
	defer telMu.RUnlock()
	return map[string]any{
		"enabled":  telCfg.enabled,
		"exported": telBatch != nil,
		"otlp_url": telCfg.otlpURL != "",
		"log_dir":  telCfg.logDir,
		"buffered": telBufLen(),
		"spool":    telSpoolPath(),
	}
}

// telSpoolPath dice dónde está la cola en disco, para poder comprobarla desde
// /api/logs sin dar acceso al fichero.
func telSpoolPath() string {
	if telBatch == nil || telBatch.spool == nil {
		return ""
	}
	return telBatch.spool.path
}

func telBufLen() int {
	if telBuf == nil {
		return 0
	}
	return telBuf.len()
}
