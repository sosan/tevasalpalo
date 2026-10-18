package telemetry

// Test contra el gateway real de Grafana. Solo se ejecuta si hay
// TELEMETRY_OTLP_URL y TELEMETRY_PASSWORD en el entorno, para no meter
// credenciales en el repo ni romper la suite en CI.
//
// Uso:
//   TELEMETRY_OTLP_URL=https://otlp-gateway-prod-eu-west-0.grafana.net/otlp/v1/logs \
//   TELEMETRY_USER=251374 TELEMETRY_PASSWORD=glc_... \
//   go test -run TestGatewayRealDeGrafana -v

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGatewayRealDeGrafana(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("TELEMETRY_OTLP_URL"))
	if url == "" {
		t.Skip("sin TELEMETRY_OTLP_URL: no se prueba el gateway real")
	}
	cfg := telemetryConfig{
		enabled:   true,
		otlpURL:   url,
		user:      os.Getenv("TELEMETRY_USER"),
		password:  os.Getenv("TELEMETRY_PASSWORD"),
		maxBatch:  EnvInt("TELEMETRY_MAX_BATCH", 200),
		bufferCap: EnvInt("TELEMETRY_BUFFER", 2000),
		clientID:  clientID(),
	}
	exp, err := newOTLPExporter(cfg)
	if err != nil {
		t.Fatalf("newOTLPExporter: %v", err)
	}

	marca := "probe-" + time.Now().UTC().Format("20060102T150405")
	batch := []map[string]any{
		buildRecord("player_hls_error", map[string]any{
			"link_id": "d3b0000000000000000000000000abc",
			"fatal":   true,
			"type":    "networkError",
			"details": "levelLoadTimeOut",
			"url":     "http://127.0.0.1:3000/ace/manifest.m3u8?id=d3b0",
			"prueba":  marca,
		}),
		buildRecord("server_log", map[string]any{
			"nivel":     "ERROR",
			"mensaje":   "❌ test de gateway " + marca,
			"prueba":    marca,
			"lineas":    float64(42),
			"repetidas": true,
		}),
		buildRecord("player_play", map[string]any{
			"link_id": "abc123",
			"prueba":  marca,
		}),
	}

	if err := exp.send(batch); err != nil {
		t.Fatalf("el gateway rechazó el payload: %v", err)
	}
	t.Cleanup(func() { exp.closeWithDeadline(time.Now().Add(5 * time.Second)) })
	t.Logf("gateway recibió %d registros (marca %s)", len(batch), marca)
}
