package transport

// Traza de diagnóstico del camino de red.
//
// Existe para responder a una pregunta concreta que los logs normales no
// permitían responder: si el motor usa el bypass, si el bypass corta lo que
// debe, y por qué WARP no levanta.
//
// Se activa con ACE_TRACE=1 porque es ruidosa: en un arranque normal el motor
// hace cientos de peticiones y no interesa verlas todas. Sin el flag, solo se
// registran los eventos que cambian el comportamiento (bloqueos, respuestas
// vacías, intentos de WARP), que son raros por definición.

import (
	"log"
	"os"
	"strings"
	"sync"
)

// traceEnabled indica si ACE_TRACE está activo.
var traceEnabled = strings.TrimSpace(os.Getenv("ACE_TRACE")) != ""

// TraceLog escribe una línea de traza solo si ACE_TRACE está activo.
func TraceLog(format string, args ...any) {
	if traceEnabled {
		log.Printf("🔍 "+format, args...)
	}
}

// TraceOnceLog escribe una línea de traza solo la primera vez que se cumple una
// condición. Sirve para eventos que se repiten mucho (el motor reintenta) y que
// solo interesa ver una vez, sin llenar el log igual que con TraceLog.
var traceSeen sync.Map

func TraceOnce(key, format string, args ...any) {
	if !traceEnabled {
		return
	}
	if _, dup := traceSeen.LoadOrStore(key, true); dup {
		return
	}
	log.Printf("🔍 "+format, args...)
}
