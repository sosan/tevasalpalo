package supervisor

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"tevasalpalo/internal/transport"
)

// Parar sobre un gestor que nunca arrancó nada no debe tocar nada ni
// bloquearse: los Stop* de transport/ace aceptan nil y WARP solo actúa si
// hay un proceso suyo en marcha. Se llama dos veces a propósito: el flujo
// real no lo repite, pero un cierre a medias sí podría.
func TestPararSinNadaLanzadoNoHaceNada(t *testing.T) {
	p := Nuevo()
	p.Parar()
	p.Parar()
}

// Parar debe cortar el hijo de Tor aunque siga vivo, ANTES de volver. Se usa
// un "sleep 300" como reemplazo de Tor y el mismo arrangement de proceso de
// producción (grupo propio), porque ya existe un caso en producción: StopTor
// mata al GRUPO de procesos, no al pid, y Tor se lanza con Setpgid para no
// llevarse por delante al proceso de la app; si alguien quita ese
// SetSysProcAttr del arranque, este test lo detecta: la señal al grupo
// vaciaría al propio binario de pruebas y el test moriría con el "sleep".
func TestPararCortaTorVivo(t *testing.T) {
	// Reemplazo del binario de tor: "sh -c sleep 300" queda colgado hasta
	// que se muera de old age o alguien lo mate.
	cmd := exec.Command("sh", "-c", "for i in $(seq 1 300); do sleep 1; done")
	// Producción: los rangos de la app (ver RunTor) asignan Setpgid para
	// matar al grupo y no al caller.
	transport.SetSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("no se pudo arrancar el reemplazo de tor: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	p := Nuevo()
	p.mu.Lock()
	p.cmdTor = cmd
	p.mu.Unlock()

	// Parar debe bloquear hasta que el STOP de tor haya terminado (y tor
	// quede muerto, no huérfano esperando al reap del sistema).
	inicio := time.Now()
	p.Parar()
	duracion := time.Since(inicio)
	if duracion < 0 {
		t.Errorf("Parar volvió en una máquina del tiempo: %v", duracion)
	}

	// El hijo debe haber sido esperado (Wait ya corrió dentro de StopTor):
	// su ProcessState no es nil y el proceso ya no existe.
	if cmd.ProcessState == nil {
		t.Errorf("tor reemplazo sigue vivo tras Parar")
	} else {
		estado := cmd.ProcessState
		if !estado.Exited() && !estado.Sys().(syscall.WaitStatus).Signaled() {
			t.Errorf("hijo vivo tras Parar: %s", estado.String())
		}
	}

	// Comprobación de superficie: el pid ya no pertenece a la red de la
	// máquina (empezó muerto en wait, el pid queda inactivo).
	if cmd.ProcessState == nil || cmd.ProcessState.Pid() < 0 {
		t.Error("pid inválido tras Parar")
	}
}

// Parar debe tolerar dos llamadas seguidas sobre el mismo hijo ya cortado:
// el segundo intento no puede bloquearse ni asustar al caller con un panic.
func TestPararMinimoHijoYaseCortado(t *testing.T) {
	cmd := exec.Command("sh", "-c", "for i in $(seq 1 300); do sleep 1; done")
	transport.SetSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("no se pudo arrancar el reemplazo de tor: %v", err)
	}

	p := Nuevo()
	p.mu.Lock()
	p.cmdTor = cmd
	p.mu.Unlock()

	p.Parar()
	p.Parar() // segunda vez: hijo ya reaped
	if cmd.ProcessState == nil {
		t.Errorf("tor reemplazo sigue vivo tras Parar doble")
	}
}

// Los manejadores al cero-value no deben invpiar el cierre (regresión del
// zero-value válido de Procesos): cmdTor/cmdAce/commander manager nil.
func TestZeroValueConHijoDeOtraFuente(t *testing.T) {
	p := &Procesos{}
	p.mu.Lock()
	p.xh = nil
	p.cmdAce = nil
	p.mu.Unlock()
	p.Parar()
}
