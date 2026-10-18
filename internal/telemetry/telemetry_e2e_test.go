package telemetry

// Prueba de extremo a extremo del camino completo: un log.Printf normal debe
// acabar en el exportador y llegar al destino OTLP. No necesita credenciales

// reales: levanta un servidor local que hace de central.

import (
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogPrintfLlegaAlCentral(t *testing.T) {
	var (
		mu      sync.Mutex
		cuerpos []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := leerCuerpoDelTest(r)
		mu.Lock()
		cuerpos = append(cuerpos, string(b))
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("TELEMETRY_ENABLED", "1")
	t.Setenv("TELEMETRY_OTLP_URL", srv.URL)
	t.Setenv("TELEMETRY_USER", "u")
	t.Setenv("TELEMETRY_PASSWORD", "p")
	t.Setenv("TELEMETRY_LOG_DIR", dir)
	t.Setenv("TELEMETRY_HASH_SALT", "sal")
	t.Setenv("TELEMETRY_FLUSH_MS", "200")

	telMu.Lock()
	telReady = false
	telCfg = telemetryConfig{}
	telBatch = nil
	telRot = nil
	telEvents = nil
	telBuf = nil
	telMu.Unlock()

	InitTelemetry()
	if telBatch == nil {
		t.Fatal("InitTelemetry no levantó el exportador")
	}

	// Un log.Printf corriente, de los cientos que ya hay en el proyecto.
	log.Printf("❌ Error al iniciar TOR: %v", "no such file")
	log.Printf("✅ Configuración del player lista")
	Record("player_play", map[string]any{"link_id": "abc", "ms": float64(900)})

	ShutdownTelemetry(5 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(cuerpos) == 0 {
		t.Fatal("no se envió nada al central")
	}
	todo := strings.Join(cuerpos, "\n")

	for _, quiero := range []string{
		"Error al iniciar TOR",
		"Configuración del player lista",
		"player_play",
		"server_log",
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("no llegó %q al central", quiero)
		}
	}
	// Y que sea OTLP de verdad. El SDK manda protobuf, así que el cuerpo no
	// contiene los nombres de campo ("resourceLogs"): buscar texto a pelo
	// daría un falso negativo. Se decodifica con el proto oficial.
	for i, c := range cuerpos {
		var logs collogpb.ExportLogsServiceRequest
		if err := proto.Unmarshal([]byte(c), &logs); err != nil {
			t.Fatalf("el cuerpo %d no es protobuf OTLP válido: %v", i, err)
		}
		if len(logs.ResourceLogs) == 0 {
			t.Fatalf("el cuerpo %d no lleva resourceLogs", i)
		}
		if len(aplanar(&logs)) == 0 {
			t.Fatalf("el cuerpo %d no lleva registros", i)
		}
	}
	// Y el log local también.
	if b, err := os.ReadFile(dir + "/streaming.log"); err != nil || !strings.Contains(string(b), "Error al iniciar TOR") {
		t.Errorf("no se escribió el fichero local: %v", err)
	}
	if b, err := os.ReadFile(dir + "/events.jsonl"); err != nil || !strings.Contains(string(b), `"player_play"`) {
		t.Errorf("no se escribió events.jsonl: %v", err)
	}
}
