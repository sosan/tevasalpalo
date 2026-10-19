package ace

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"tevasalpalo/internal/runtimefs"
	"tevasalpalo/internal/transport"
)

const (
	runtimeDirName    = "runtime"
	HttpPort          = 6878
	HttpWebServerPort = 3000
	aceAssetNameWin   = "acestream-runtime-windows.zip"
	aceAssetNameLinux = "acestream-runtime-linux-x86_64.tar.gz"
)

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
	if !waitForAPI(fmt.Sprintf("http://localhost:%d/webui/api/service?method=get_version", HttpPort), 30*time.Second) {
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
