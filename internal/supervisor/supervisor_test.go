package supervisor

import "testing"

// Parar sobre un gestor que nunca arrancó nada no debe tocar nada ni
// bloquearse: los Stop* de transport/ace aceptan nil y WARP solo actúa si
// hay un proceso suyo en marcha.
func TestPararSinNadaLanzadoNoHaceNada(t *testing.T) {
	Nuevo().Parar()
}
