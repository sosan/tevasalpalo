package app

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"tevasalpalo/internal/runtimefs"
	"tevasalpalo/internal/transport"
	"time"
)

const (
	runtimeDirName    = "runtime"
	httpPort          = 6878
	httpWebServerPort = 3000
	aceAssetNameWin   = "acestream-runtime-windows.zip"
	aceAssetNameLinux = "acestream-runtime-linux-x86_64.tar.gz"
)

func findBroadcaster(name string, competitionName, sport string) BroadcasterInfo {
	// Coincidencia exacta
	// quizas pasarlo a minisculas
	nameUpper := strings.ToUpper(strings.TrimSpace(name))
	// Normalizar abreviatura "M+ L." -> "M+ LIGA" (usuario solicita m+ l. de campeones)
	nameUpper = strings.ReplaceAll(nameUpper, "M+ L.", "M+ LIGA")
	nameUpper = strings.ReplaceAll(nameUpper, "M+L.", "M+ LIGA")
	// Colapsar espacios dobles por si acaso
	nameUpper = strings.Join(strings.Fields(nameUpper), " ")
	// Normalización para futbolenvivoargentina.com (DSports / ESPN Argentina / TNT)
	if strings.Contains(nameUpper, "DSPORTS") {
		nameUpper = "DS SPORT"
	} else if strings.Contains(nameUpper, "ESPN PREMIUM") {
		nameUpper = "ESPN ARGENTINA"
	} else if nameUpper == "ESPN 4" || strings.Contains(nameUpper, "ESPN 4") {
		nameUpper = "ESPN ARGENTINA 4"
	} else if nameUpper == "ESPN 3" || strings.Contains(nameUpper, "ESPN 3") {
		nameUpper = "ESPN ARGENTINA 3"
	} else if nameUpper == "ESPN 2" || strings.Contains(nameUpper, "ESPN 2") {
		nameUpper = "ESPN ARGENTINA 2"
	} else if strings.Contains(nameUpper, "TNT SPORTS PREMIUM") {
		nameUpper = "TNT SPORTS"
	} else if strings.Contains(nameUpper, "TNT SPORTS") {
		nameUpper = "TNT SPORTS"
	}
	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS BUNDESLIGA"
	}
	if competitionName == "LaLiga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS LALIGA"
	}
	if sport == "Baloncesto" {
		if nameUpper == "DAZN" {
			nameUpper = "DAZN BALONCESTO"
		}
		if competitionName == "Copa del Rey Baloncesto" {
			nameUpper = "DAZN BALONCESTO"
		}
	}

	// UFC en Paramount+ (2026+) — normalizar variantes Paramount+ / CBS
	// UFC dejó ESPN+ y pasó a Paramount+ en US/LatAm/Australia desde 01/01/2026
	// Solo mapear a UFC si hay indicio UFC (en nombre o en competición), si no filtrar (ej: Paramount+ en LaLiga no es Acestream)
	if strings.Contains(nameUpper, "PARAMOUNT") {
		if strings.Contains(nameUpper, "UFC") {
			if dataAce, exists := broadcasterToAcestream["PARAMOUNT+ UFC"]; exists && len(dataAce.Links) > 0 {
				return dataAce
			}
			if dataAce, exists := broadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
		if competitionName == "UFC" {
			if dataAce, exists := broadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
			// Alias genérico Paramount+ -> mapear a UFC solo si es UFC
			nameUpper = "PARAMOUNT+ UFC"
			if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
				return dataAce
			}
			nameUpper = "PARAMOUNT+"
			if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
				return dataAce
			}
			if dataAce, exists := broadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
		// Para LaLiga u otras competiciones, Paramount+ sin UFC no tiene pool Acestream → filtrar para no colar UFC en LaLiga
		return BroadcasterInfo{}
	}
	if strings.Contains(nameUpper, "CBS") && competitionName == "UFC" {
		if dataAce, exists := broadcasterToAcestream["UFC"]; exists {
			return dataAce
		}
	}

	// LALIGA TV M1-M4 solo aplica a Hypermotion — evitar contaminación en Serie A Italiana
	// Ej: futbolenlatv.es lista "LaLiga TV M3" para Genoa-Como (Serie A) que NO es Hypermotion
	switch nameUpper {
	case "LALIGA TV M2", "LALIGA TV M3":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION"
		} else {
			// Para Serie A y otras competiciones, M3 es canal BAR sin acestream fiable -> filtrar
			return BroadcasterInfo{}
		}
	case "LALIGA TV M4":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION 2"
		} else {
			return BroadcasterInfo{}
		}
	case "LALIGA TV M1":
		if strings.Contains(strings.ToUpper(competitionName), "HYPERMOTION") {
			nameUpper = "LALIGA HYPERMOTION 3"
		} else {
			return BroadcasterInfo{}
		}
	}

	// Unificar variantes LaLiga Hypermotion al mismo canal (incluye 4 y 5 nuevos)
	switch nameUpper {
	case "LALIGA TV HYPERMOTION":
		nameUpper = "LALIGA HYPERMOTION"
	case "LALIGA TV HYPERMOTION 2":
		nameUpper = "LALIGA HYPERMOTION 2"
	case "LALIGA TV HYPERMOTION 3":
		nameUpper = "LALIGA HYPERMOTION 3"
	case "LALIGA TV HYPERMOTION 4", "LALIGA TV HYPERMOTION 5", "LALIGA HYPERMOTION 4", "LALIGA HYPERMOTION 5", "HYPERMOTION 4", "HYPERMOTION 5":
		// normalizar a LALIGA HYPERMOTION 4/5 según número
		if strings.HasSuffix(nameUpper, "5") {
			nameUpper = "LALIGA HYPERMOTION 5"
		} else {
			nameUpper = "LALIGA HYPERMOTION 4"
		}
	}

	// M+ BALONCESTO / DAZN BALONCESTO — respetar prefijo (usuario añadió M+ BALONCESTO)
	if strings.Contains(nameUpper, "BALONCESTO") {
		isMPlus := strings.Contains(nameUpper, "M+") || strings.Contains(nameUpper, "MOVISTAR")
		if isMPlus {
			if strings.Contains(nameUpper, "2") {
				nameUpper = "M+ BALONCESTO 2"
			} else if strings.Contains(nameUpper, "3") {
				nameUpper = "M+ BALONCESTO 2" // no hay M+ 3, fallback a 2
			} else {
				nameUpper = "M+ BALONCESTO"
			}
			if dataAce, exists := broadcasterToAcestream[nameUpper]; exists && len(dataAce.Links) > 0 {
				return dataAce
			}
			// fallback a DAZN si M+ aún está vacío
		}
		if strings.Contains(nameUpper, "2") {
			nameUpper = "DAZN BALONCESTO 2"
		} else if strings.Contains(nameUpper, "3") {
			nameUpper = "DAZN BALONCESTO 3"
		} else {
			nameUpper = "DAZN BALONCESTO 1"
		}
		if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
			return dataAce
		}
	}

	// FOX UFC / UFC Fight Pass -> UFC (Paramount+ era)
	if strings.Contains(nameUpper, "UFC") {
		if strings.Contains(nameUpper, "FOX") || strings.Contains(nameUpper, "FIGHT PASS") || strings.Contains(nameUpper, "PREMIUM") {
			if dataAce, exists := broadcasterToAcestream["UFC"]; exists {
				return dataAce
			}
		}
	}

	if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
		return dataAce
	}
	return BroadcasterInfo{}
}

// // findLinkForBroadcaster busca un enlace para un nombre de broadcaster.
// // Prioriza la coincidencia exacta, luego parcial.
// func findLinkForBroadcaster(name string, competitionName string) []string {
// 	// Coincidencia exacta
// 	// quizas pasarlo a minisculas
// 	nameUpper := strings.ToUpper(name)
// 	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
// 		nameUpper = "SKY SPORTS BUNDESLIGA"
// 	}
// 	if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
// 		return dataAce.Links
// 	}

// 	// Coincidencia parcial (como antes)
// 	// nameUpper := strings.ToUpper(name)
// 	for key, dataAce := range broadcasterToAcestream {
// 		baseKey := strings.Split(key, " [")[0]
// 		if strings.Contains(nameUpper, strings.ToUpper(baseKey)) {
// 			// Preferir coincidencia exacta de base si es posible
// 			if nameUpper == strings.ToUpper(baseKey) {
// 				return dataAce.Links
// 			}
// 			// Si no hay exacta, esta es una candidata (la última encontrada)
// 			// Para hacerlo más robusto, podrías tener lógica para elegir la mejor parcial
// 		}
// 	}
// 	// Si no se encontró parcial, devolver vacío
// 	return []string{}
// }

// engineSpec describe cómo extraer y arrancar el motor para una plataforma.
// Los dos builds no comparten layout: el de Windows es un ejecutable
// (ace_console.exe) dentro de runtime/engine/ con python38 y módulos .pyd, y el
// de Linux es un binario ELF (acestreamengine) en la raíz del runtime con los
// módulos en runtime/lib/acestreamengine y las wheels al lado. Por eso no se
// puede usar el ZIP de Windows en el build linux (antes se intentaba y
// fallaba al lanzar ace_console.exe).
type engineSpec struct {
	// marker es el archivo cuya presencia indica "ya extraído".
	marker string
	// asset es la ruta dentro del embed.FS.
	asset string
	// binary es la ruta del ejecutable, relativa a runtimePath.
	binary string
	// workDir es el directorio de trabajo del proceso, relativo a runtimePath.
	workDir string
	// dataDir es donde el motor guarda plugins y schema, relativo a runtimePath.
	dataDir string
	// args son los flags de arranque.
	args []string
}

// aceEngineSpec devuelve la ruta del motor según el SO compilado.
func aceEngineSpec(runtimePath string) engineSpec {
	args := []string{
		"--live-buffer", "60", // 30
		"--vod-buffer", "10", // 30
		"--client-console",
	}
	if runtime.GOOS == "windows" {
		return engineSpec{
			marker:  filepath.Join(runtimePath, "engine", "ace_console.exe"),
			asset:   "assets/" + aceAssetNameWin,
			binary:  filepath.Join("engine", "ace_console.exe"),
			workDir: "engine",
			dataDir: filepath.Join("engine", "data"),
			args:    args,
		}
	}
	return engineSpec{
		marker:  filepath.Join(runtimePath, "acestreamengine"),
		asset:   "assets/" + aceAssetNameLinux,
		binary:  "acestreamengine",
		workDir: ".",
		dataDir: "data",
		args:    args,
	}
}

func RunAceStream() (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("No se pudo obtener la ruta del ejecutable: ", err)
	}
	execDir := filepath.Dir(exePath)

	runtimePath := filepath.Join(execDir, runtimeDirName)
	spec := aceEngineSpec(runtimePath)
	engineAcePath := filepath.Join(runtimePath, spec.binary)

	if !runtimefs.ExisteFichero(spec.marker) {
		log.Println("📦 No se encontró Lista Canales TV. Extrayendo por primera vez...")
		if err := runtimefs.ExtraerRuntime(runtimePath, spec.asset); err != nil {
			log.Fatal("Error al extraer Lista Canales: ", err)
		}
		log.Println("✅ Lista Canales TV extraído exitosamente.")
	} else {
		log.Println("🔁 Lista Canales TV ya existe. Usando versión existente.")
	}

	log.Println("🚀 Actualizando Lista Canales TV...")
	// Hooks de red y plugins: se aplican sobre el motor recién extraído.
	applyEngineOverlay(runtimePath, spec)

	// Bypass DNS/VAST por proxy: el motor no se puede parchear en Windows, así
	// que se le pone delante un proxy local que corta los hosts muertos y
	// responde VAST vacío. Va por variables de entorno, que es la única vía
	// que el motor 3.2.8 respeta (no expone claves de proxy en acestream.conf).
	env := os.Environ()
	if engineHooksEnabled() {
		if addr, err := serveBypass(); err != nil {
			log.Printf("⚠️ bypass del motor no disponible: %v", err)
		} else {
			env = append(env, engineProxyEnv(addr)...)
			log.Printf("🛡️ Bypass DNS/VAST del motor activo en %s", addr)
			transport.TraceLog("bypass escuchando en %s; el motor recibe HTTP_PROXY/HTTPS_PROXY", addr)
		}
	} else {
		transport.TraceLog("bypass desactivado con %s=0", hooksEnabledEnv)
	}
	if runtime.GOOS != "windows" {
		// El motor de Linux enlaza sus .so desde runtime/lib (así lo hace su
		// start-engine con LD_LIBRARY_PATH) e importa sitecustomize del overlay.
		env = append(env,
			"LD_LIBRARY_PATH="+filepath.Join(runtimePath, "lib"),
			"PYTHONPATH="+overlayDir(runtimePath),
		)
	}

	cmd := exec.Command(engineAcePath, spec.args...)
	cmd.Dir = filepath.Join(runtimePath, spec.workDir)
	cmd.Env = env
	transport.SetSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		log.Fatal("No se pudo iniciar: ", err)
	}

	log.Println("⏳ Esperando a que termine de actualizarse la Lista Canales TV...")
	if !waitForAPI(fmt.Sprintf("http://localhost:%d/webui/api/service?method=get_version", httpPort), 30*time.Second) {
		log.Fatal("❌ No respondió después de 30 segundos")
	}

	log.Println("✅ Todo listo. ¡A relajarse y disfrutar del contenido! 🍿")

	return cmd, err
}

// waitForAPI espera a que la API responda con 200 OK
func waitForAPI(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
