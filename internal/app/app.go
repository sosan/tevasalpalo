package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"tevasalpalo/internal/secrets"
	"tevasalpalo/internal/source"
	"tevasalpalo/internal/store"
	"tevasalpalo/internal/supervisor"
	"tevasalpalo/internal/telemetry"
	"tevasalpalo/internal/web"
	"tevasalpalo/update"
)

var shutdownChan = make(chan struct{})

// autoActualizar imprime la versión y, salvo en dev, intenta la autoupdate.
// Antes vivía en init() del paquete: cualquier import de app la disparaba
// (incluidos los tests y las herramientas de build). Ahora solo corre cuando
// Main la llama y después de las herramientas de build (--genera-secretos /
// --genera-semilla), que no arrancan la app y antes podían arrastrar una
// autoupdate y su red de gratis.
func autoActualizar() {
	fmt.Println(update.GetVersionBuild())
	if os.Getenv("ENV") == "dev" {
		return
	}
	if needUpdate, updatedOk := update.AutoUpdate(); needUpdate && !updatedOk {
		log.Printf("NECESARIO ACTUALIZAR PERO NO HA SIDO POSIBLE!!")
	}
}

func Main() {
	// Herramienta de build: generar el contenedor cifrado de secretos. Se
	// atiende antes que nada porque no arranca la app, solo escribe los
	// -ldflags en stdout.
	for _, arg := range os.Args[1:] {
		if arg == "--genera-secretos" || arg == "--generar-secretos" {
			if err := secrets.GenerarSecretosIncrustados(); err != nil {
				log.Fatal("❌ ", err)
			}
			return
		}
	}

	// Herramienta de build: volcar la base actual a la instantánea que se
	// embebe. Se atiende aparte porque no arranca la app, solo lee la base y
	// escribe el fichero.
	generarSemillaPedida := false
	for _, arg := range os.Args[1:] {
		if arg == "--genera-semilla" {
			generarSemillaPedida = true
		}
	}

	// Y se resuelve aquí, antes que la telemetría y Tor: es una herramienta de
	// build, no necesita proxy, red ni central, y si fuera más tarde
	// arrancaría todo eso para nada y podría morir por un Tor ocupado.
	if generarSemillaPedida {
		if s := store.AbrirStore(); s != nil {
			defer store.CerrarStore()
			if err := store.GenerarSemilla(); err != nil {
				log.Fatal("❌ ", err)
			}
			return
		}
		log.Fatal("❌ no se pudo abrir la base de datos")
	}

	autoActualizar()

	// Telemetría antes de nada más: a partir de aquí todo lo que escriba el
	// log (incluido lo de arranque) queda en fichero, buffer y central.
	telemetry.TelVersion = update.VersionBuild
	if telemetry.TelVersion == "" {
		telemetry.TelVersion = "dev"
	}
	telemetry.TelOS = runtime.GOOS
	telemetry.TelArch = runtime.GOARCH

	// Secretos incrustados antes que el resto: pueden traer la configuración
	// de Infisical y otras variables que la telemetría necesita. Solo rellena
	// lo que esté vacío, así que el entorno gana siempre.
	secrets.AplicarSecretosIncrustados()

	// Secrets antes de la telemetría: el token de Grafana puede venir de
	// Infisical, y para entonces el log global todavía no está redirigido (por
	// si hay que avisar de algo). Es no bloqueante para el arranque: si falla,
	// se sigue con lo que haya en el entorno.
	if os.Getenv("INFISICAL_ENABLED") != "0" {
		secrets.FetchSecretsFromInfisical()
	}

	telemetry.InitTelemetry()

	// Tor, xray y más tarde el engine: el supervisor guarda los manejadores
	// y los corta en orden al cierre.
	proc := supervisor.Nuevo()
	proc.ArrancarProxies()

	// Base de datos de AceStream. Se abre antes de nada porque permite
	// cargar el listado del arranque anterior al instante: sin esto, la web
	// tarda entre 27 y 70 segundos en levantarse mientras espera a las 7
	// fuentes remotas.
	if s := store.AbrirStore(); s != nil {
		defer store.CerrarStore()

		// Antes de leer nada: si la base está vacía (instalación nueva o
		// borrada a mano), se siembra con la instantánea embebida. Así el
		// listado que se carga un instante después ya tiene canales y la web
		// no muestra una lista vacía mientras llegan las fuentes.
		store.SembrarBaseVacia()

		if guardados := len(s.CargarCanales()); guardados > 0 {
			log.Printf("💾 Base de datos lista: %d canales del último arranque", guardados)
		}
	}

	// El listado se carga de la base si hay, y la red se consulta después. Al
	// revés, la web no arranca hasta que respondan las 7 fuentes.
	source.CargarListadoDesdeDisco()

	web.OnShutdownApp = func() {
		select {
		case shutdownChan <- struct{}{}:
		case <-time.After(3 * time.Second):
			log.Println("⚠️ nadie escuchaba el shutdown; sigo")
		}
	}
	webServer, err := web.StartWebServer()
	if err != nil {
		log.Printf("❌ Error en servidor web: %v", err)
	}

	// Refresco de las fuentes: inmediato al arrancar, luego cada 6h (alineado
	// con el ticker de datos de la web). La web ya está sirviendo con lo que
	// hay en la base y se actualiza sola cuando lleguen los datos.
	source.IniciarRefrescoCiclico(6 * time.Hour)

	log.Println("🌐 Servidor web iniciando en http://localhost:3000")
	time.Sleep(10 * time.Second)
	proc.ArrancarEngine()

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

	proc.Parar()

	// Último envío al central: sin esto se pierden justo los eventos del
	// cierre, que suelen ser los que explican un fallo.
	telemetry.ShutdownTelemetry(3 * time.Second)
}
