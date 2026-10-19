// Package supervisor gestiona el ciclo de vida de los procesos hijos que la
// app necesita: Tor, xray en segundo plano, el engine AceStream embebido y
// WARP. Arranca cada pieza en su momento y las para en orden inverso al
// cierre, para que ninguna se quede huérfana ni muerda la que viene después.
package supervisor

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"tevasalpalo/internal/ace"
	"tevasalpalo/internal/transport"
)

// Procesos guarda los manejadores de los procesos hijos que haya lanzado.
// El cero value es válido: Parar sobre algo que nunca arrancó no hace nada.
type Procesos struct {
	mu sync.Mutex
	// cmdTor es el proceso de Tor, o nil si no se ha llegado a arrancar.
	cmdTor *exec.Cmd
	// xh es el handle de xray en-proceso, o nil si no está disponible o
	// todavía no terminó de arrancar (va en segundo plano).
	xh *transport.XrayHandle
	// cmdAce es el engine AceStream, o nil si no se ha llegado a arrancar
	// (p. ej. con ENV=dev, donde el engine corre en compose).
	cmdAce *exec.Cmd
}

// Nuevo devuelve un gestor de procesos sin nada lanzado todavía.
func Nuevo() *Procesos { return &Procesos{} }

// ArrancarProxies levanta Tor (bloqueante y fatal si falla: sin proxy no hay
// arranque que valga) y lanza xray en segundo plano (opcional: mientras no
// esté, la cadena usa Tor y cambia sola cuando xray esté listo).
func (p *Procesos) ArrancarProxies() {
	avisoEnlacesXray()

	log.Println("📡 Iniciando Tor...")
	cmdTor, err := transport.RunTor()
	if err != nil {
		log.Fatal("❌ Error al iniciar TOR: ", err)
	}
	p.mu.Lock()
	p.cmdTor = cmdTor
	p.mu.Unlock()

	// xray en-proceso opcional en segundo plano: con la sub hardcodeada puede
	// tardar (prueba endpoints con tráfico); no bloquea el arranque — la
	// cadena usa Tor mientras tanto y cambia sola a xray cuando esté listo.
	go func() {
		h, err := transport.MaybeRunXray()
		if err != nil {
			log.Printf("⚠️  xray no disponible, se usará Tor: %v", err)
			return
		}
		if h == nil {
			return
		}
		p.mu.Lock()
		p.xh = h
		p.mu.Unlock()
	}()
}

// ArrancarEngine lanza el engine AceStream embebido en segundo plano y abre
// el navegador cuando la lista está lista. Con ENV=dev no hace nada: el
// engine corre como servicio aparte en compose.
func (p *Procesos) ArrancarEngine() {
	if os.Getenv("ENV") == "dev" {
		return
	}
	go func() {
		cmd, err := ace.RunAceStream()
		if err != nil {
			log.Fatal("❌ Error al iniciar AceStream: ", err)
		}
		p.mu.Lock()
		p.cmdAce = cmd
		p.mu.Unlock()
		// log.Println("🎉 Lista Canales TV lista. Abriendo interfaz...")
		abrirNavegador(fmt.Sprintf("http://localhost:%d", ace.HttpWebServerPort))
	}()
}

// Parar corta todos los procesos en orden: primero la web ya habrá cerrado
// (eso lo hace la app), luego Tor, xray, WARP y por último el engine. Cada
// fallo se loguea y se sigue: al cierre interesa soltar todo lo posible.
func (p *Procesos) Parar() {
	p.mu.Lock()
	cmdTor, xh, cmdAce := p.cmdTor, p.xh, p.cmdAce
	p.mu.Unlock()

	if err := transport.StopTor(cmdTor); err != nil {
		log.Printf("❌ Error al detener TOR: %v %v", err, cmdTor)
	}

	if err := transport.StopXray(xh); err != nil {
		log.Printf("❌ Error al cerrar xray: %v", err)
	} else if xh != nil {
		log.Println("✅ Cerrado xray correctamente")
	}

	if err := transport.StopWarpIfRunning(); err != nil {
		log.Printf("❌ Error al cerrar WARP: %v", err)
	}

	if err := ace.StopAceStream(cmdAce); err != nil {
		log.Printf("❌ Error al cerrar Acestream: %v", err)
	} else if cmdAce != nil {
		log.Println("✅ Cerrado ace correctamente")
	}
}

// abrirNavegador abre la interfaz en el navegador del sistema. Best effort:
// un fallo (p. ej. xdg-open ausente) no debe tumbar la app.
func abrirNavegador(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // Linux, etc.
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// avisoEnlacesXray recuerda dónde pegar los enlaces de proxy, para no tener
// que buscar el nombre cada vez. Se llama al arrancar y solo loguea si el
// fichero no está: si ya existe, calla (el log de xray ya dice cuántos
// enlaces ha encontrado).
func avisoEnlacesXray() {
	if p := transport.XrayLocalFile(); p != "" {
		return
	}
	log.Printf("💡 Para usar enlaces de proxy (vless/vmess/trojan/ss/socks/wireguard): pégalos en %s, junto al ejecutable", transport.XrayLinksFile)
}
