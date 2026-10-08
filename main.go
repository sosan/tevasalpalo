package main

import (
	"context"
	"fmt"
	"log"
	"main/update"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var shutdownChan = make(chan struct{})

func init() {
	env := os.Getenv("ENV")
	fmt.Println(update.GetVersionBuild())
	if env == "dev" {
		return
	}
	needUpdate, updatedOk := update.AutoUpdate()
	if needUpdate && !updatedOk {
		log.Printf("NECESARIO ACTUALIZAR PERO NO HA SIDO POSIBLE!!")
	}
}

func main() {
	avisoEnlacesXray()

	// log.Println("📡 Iniciando Tor...")
	cmdTor, err := RunTor()
	if err != nil {
		log.Fatal("❌ Error al iniciar TOR: ", err)
	}

	// xray en-proceso opcional en segundo plano: con la sub hardcodeada puede
	// tardar (prueba endpoints con tráfico); no bloquea el arranque — la
	// cadena usa Tor mientras tanto y cambia sola a xray cuando esté listo.
	var xh *xrayHandle
	var xhMu sync.Mutex
	go func() {
		h, err := MaybeRunXray()
		if err != nil {
			log.Printf("⚠️  xray no disponible, se usará Tor: %v", err)
			return
		}
		if h == nil {
			return
		}
		xhMu.Lock()
		xh = h
		xhMu.Unlock()
	}()

	err = FetchUpdatedList()
	if err != nil {
		log.Printf("Error al obtener la programación")
	}

	webServer, err := StartWebServer()
	if err != nil {
		log.Printf("❌ Error en servidor web: %v", err)
	}

	log.Println("🌐 Servidor web iniciando en http://localhost:3000")
	time.Sleep(10 * time.Second)
	var (
		cmdAcestream   *exec.Cmd
		cmdAcestreamMu sync.Mutex
	)

	env := os.Getenv("ENV")
	if env != "dev" {
		go func() {
			cmd, err := RunAceStream()
			if err != nil {
				log.Fatal("❌ Error al iniciar AceStream: ", err)
			}
			cmdAcestreamMu.Lock()
			cmdAcestream = cmd
			cmdAcestreamMu.Unlock()
			// log.Println("🎉 Lista Canales TV lista. Abriendo interfaz...")
			openBrowser(fmt.Sprintf("http://localhost:%d", httpWebServerPort))
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("⏳ Señal del sistema recibida, cerrando...")
	case <-shutdownChan:
		log.Println("⏳ Señal de autoupdate recibida, cerrando limpio...")
	}

	log.Println("🛑 Cerrando...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := webServer.ShutdownWithContext(ctx); err != nil {
		log.Printf("❌ Error al cerrar servidor: %v", err)
	} else {
		log.Println("✅ Cerrado webserver correctamente")
	}

	if err := StopTor(cmdTor); err != nil {
		log.Printf("❌ Error al detener TOR: %v %v", err, cmdTor)
	}

	xhMu.Lock()
	xhStopped := xh
	xhMu.Unlock()
	if err := StopXray(xhStopped); err != nil {
		log.Printf("❌ Error al cerrar xray: %v", err)
	} else if xhStopped != nil {
		log.Println("✅ Cerrado xray correctamente")
	}

	if err := StopWarpIfRunning(); err != nil {
		log.Printf("❌ Error al cerrar WARP: %v", err)
	}

	cmdAcestreamMu.Lock()
	aceCmd := cmdAcestream
	cmdAcestreamMu.Unlock()
	if err := StopAceStream(aceCmd); err != nil {
		log.Printf("❌ Error al cerrar Acestream: %v", err)
	} else if aceCmd != nil {
		log.Println("✅ Cerrado ace correctamente")
	}
}

// avisoEnlacesXray recuerda dónde pegar los enlaces de proxy, para no tener que
// buscar el nombre cada vez. Se llama al arrancar y solo loguea si el fichero
// no está: si ya existe, calla (el log de xray ya dice cuántos enlaces ha
// encontrado).
func avisoEnlacesXray() {
	if p := xrayLocalFile(); p != "" {
		return
	}
	log.Printf("💡 Para usar enlaces de proxy (vless/vmess/trojan/ss/socks/wireguard): pégalos en %s, junto al ejecutable", xrayLinksFile)
}
