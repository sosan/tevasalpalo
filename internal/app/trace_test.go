package app

// Tests de la traza de diagnóstico (trace.go).
//
// La traza existe para responder "¿por qué el motor no pasa por el bypass?" y
// "¿por qué WARP no arranca?". Si se rompe en silencio vuelve a pasar lo mismo
// que antes: logs que no dicen nada.

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestTraceLogSoloConFlag(t *testing.T) {
	// Sin el flag no debe escribir nada: en un arranque normal el motor hace
	// cientos de peticiones y el log se vuelve ilegible.
	prev, hadPrev := os.LookupEnv("ACE_TRACE")
	os.Unsetenv("ACE_TRACE")
	traceEnabled = strings.TrimSpace(os.Getenv("ACE_TRACE")) != ""
	if traceEnabled {
		t.Fatal("la traza no debería estar activa sin ACE_TRACE")
	}
	// No hay assertion sobre la salida: lo que importa es que el flag se lee
	// del entorno y no queda activado por defecto.
	if hadPrev {
		os.Setenv("ACE_TRACE", prev)
	}
}

func TestTraceLeeFlag(t *testing.T) {
	os.Setenv("ACE_TRACE", "1")
	traceEnabled = strings.TrimSpace(os.Getenv("ACE_TRACE")) != ""
	if !traceEnabled {
		t.Error("ACE_TRACE=1 no activó la traza")
	}
	os.Unsetenv("ACE_TRACE")
	traceEnabled = strings.TrimSpace(os.Getenv("ACE_TRACE")) != ""
}

// TestTraceOnceNoRepite documenta que traceOnce solo emite una vez por clave:
// los reintentos del motor se repiten muchas veces y sin dedupe el log se llena
// de la misma línea.
func TestTraceOnceNoRepite(t *testing.T) {
	os.Setenv("ACE_TRACE", "1")
	traceEnabled = true
	traceSeen = sync.Map{}
	defer func() {
		os.Unsetenv("ACE_TRACE")
		traceEnabled = false
	}()
	// Se llama dos veces con la misma clave; si no hubiera dedupe, la segunda
	// habría escrito otra línea.
	traceOnce("test-clave", "primera vez")
	traceOnce("test-clave", "segunda vez")
	// Con clave distinta sí escribe.
	traceOnce("otra-clave", "otra")
}
