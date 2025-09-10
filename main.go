package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"streaming/update"
	"syscall"
	"time"
)

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
	go func() {
		// donwload updated lists
		err := fetchUpdatedList()
		if err != nil {
			log.Printf("Error al obtener la programación")
		}

		log.Println("🌐 Servidor web iniciando en http://localhost:3000")
		if err := startWebServer(); err != nil {
			log.Printf("❌ Error en servidor web: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	env := os.Getenv("ENV")
	if env != "dev" {
		if err := runAceStream(); err != nil {
			log.Fatal("❌ Error al iniciar AceStream: ", err)
		}
	} else {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
	}
}
