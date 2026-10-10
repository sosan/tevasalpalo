package main

// Tests de la política de reintentos de las webs de horarios (EPG).
//
// Con el ISP cortando por DNS, empezar por directo costaba 10 intentos y
// varios minutos antes de llegar al proxy que sí respondía. Estos tests fijan
// el orden proxy-primero y que los errores no recuperables no se repitan.

import (
	"errors"
	"fmt"
	"testing"
)

func TestRetryWorthIt(t *testing.T) {
	casos := []struct {
		err    error
		nombre string
		want   bool
	}{
		{nil, "sin error", true},
		{errors.New("dial tcp: no such host"), "DNS: se reintenta", true},
		{errors.New("context deadline exceeded"), "timeout: se reintenta", true},
		{errors.New("connection refused"), "conexión rechazada: se reintenta", true},
		{errors.New("status code error: 404 Not Found"), "404: no se reintenta", false},
		{errors.New("status code error: 403 Forbidden"), "403: no se reintenta", false},
		{errors.New("status code error: 500 Internal Server Error"), "500: se reintenta", true},
		{errors.New("status code error: 429 Too Many Requests"), "429: se reintenta", true},
		{fmt.Errorf("envuelto: status code error: 404"), "404 envuelto: no se reintenta", false},
	}
	for _, c := range casos {
		if got := retryWorthIt(c.err); got != c.want {
			t.Errorf("retryWorthIt(%s) = %v, se esperaba %v", c.nombre, got, c.want)
		}
	}
}
