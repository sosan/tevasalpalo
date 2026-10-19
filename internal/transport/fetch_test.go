package transport

// Tests del transporte directo: forzado de IPv4.
//
// Sin IPv6 en la red, el dialer por defecto de Go intentaba el AAAA y fallaba
// con "network is unreachable" tras agotar el timeout. Cada reintento de una
// fuente (10 por fuente) costaba lo mismo que uno que sí iba a funcionar.

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDirectDialContextFuerzaIPv4(t *testing.T) {
	// Un listener real en IPv4: si se le pide "tcp4" debe funcionar.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// "tcp" (lo que pide http.Transport) debe convertirse en tcp4 y conectar.
	conn, err := directDialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("directDialContext con network=tcp falló: %v", err)
	}
	conn.Close()

	// Si se pasa "" también debe interpretarse como tcp4.
	conn2, err := directDialContext(context.Background(), "", ln.Addr().String())
	if err != nil {
		t.Fatalf("directDialContext con network vacío falló: %v", err)
	}
	conn2.Close()
}

func TestDirectTransportUsaDialContext(t *testing.T) {
	tr := directTransport()
	if tr.DialContext == nil {
		t.Fatal("directTransport no fija DialContext: volvería al dialer por defecto con AAAA")
	}
}

func TestClientDirectUsaTransportePropio(t *testing.T) {
	// El cliente "direct" debe llevar el transporte con IPv4 forzado; sin esto
	// el cambio no llega a las fuentes de listas.
	//
	// Se pasa un timeout real: ClientForTransportTimeout no normaliza el 0 a
	// TimeTimeout, eso lo hace FetchWebDataTimeout antes de llamar.
	c, err := ClientForTransportTimeout("direct", 25*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport == nil {
		t.Fatal("el cliente direct no tiene Transport propio")
	}
	if c.Timeout != 25*time.Second {
		t.Errorf("timeout = %v, se esperaba 25s", c.Timeout)
	}
}
