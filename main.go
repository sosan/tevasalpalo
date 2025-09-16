package main

import (
	"context"
	"fmt"
	"log"
	"main/update"
	"os"
	"os/exec"
	"os/signal"
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
	log.Println("📡 Obteniendo lista de emisiones TV...")
	// donwload updated lists
	err := FetchUpdatedList()
	if err != nil {
		log.Printf("Error al obtener la programación")
	}

	log.Println("🌐 Servidor web iniciando en http://localhost:3000")
	webServer, err := StartWebServer()
	if err != nil {
		log.Printf("❌ Error en servidor web: %v", err)
	}

	time.Sleep(1 * time.Second)
	var cmdAcestream *exec.Cmd

	env := os.Getenv("ENV")
	if env != "dev" {
		go func() {
			cmdAcestream, err = RunAceStream()
			if err != nil {
				log.Fatal("❌ Error al iniciar AceStream: ", err)
			}
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
		log.Println("✅ Cerrado 1 correctamente")
	}

	if cmdAcestream != nil && cmdAcestream.Process != nil {
		if err := cmdAcestream.Process.Kill(); err != nil {
			log.Printf("❌ Error al cerrar: %v", err)
		} else {
			log.Println("✅ Cerrado 2 correctamente")
		}
	}

}
