package main

import (
	"archive/zip"
	"embed"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed assets/acestream-runtime-windows.zip
var runtimeZip embed.FS

const (
	runtimeDirName    = "runtime" // Carpeta junto al .exe
	httpPort          = 6878
	httpWebServerPort = 3000
	assetName         = "acestream-runtime-windows.zip"
)

func findBroadcaster(name string, competitionName string) BroadcasterInfo {
	// Coincidencia exacta
	// quizas pasarlo a minisculas
	nameUpper := strings.ToUpper(name)
	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS BUNDESLIGA"
	}
	if competitionName == "LaLiga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS LALIGA"
	}
	
	if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
		return dataAce
	}
	return BroadcasterInfo{}
}


// findLinkForBroadcaster busca un enlace para un nombre de broadcaster.
// Prioriza la coincidencia exacta, luego parcial.
func findLinkForBroadcaster(name string, competitionName string) []string {
	// Coincidencia exacta
	// quizas pasarlo a minisculas
	nameUpper := strings.ToUpper(name)
	if competitionName == "Bundesliga" && nameUpper == "SKY SPORTS" {
		nameUpper = "SKY SPORTS BUNDESLIGA"
	}
	if dataAce, exists := broadcasterToAcestream[nameUpper]; exists {
		return dataAce.Links
	}

	// Coincidencia parcial (como antes)
	// nameUpper := strings.ToUpper(name)
	for key, dataAce := range broadcasterToAcestream {
		baseKey := strings.Split(key, " [")[0]
		if strings.Contains(nameUpper, strings.ToUpper(baseKey)) {
			// Preferir coincidencia exacta de base si es posible
			if nameUpper == strings.ToUpper(baseKey) {
				return dataAce.Links
			}
			// Si no hay exacta, esta es una candidata (la última encontrada)
			// Para hacerlo más robusto, podrías tener lógica para elegir la mejor parcial
		}
	}
	// Si no se encontró parcial, devolver vacío
	return []string{}
}

func RunAceStream() (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("No se pudo obtener la ruta del ejecutable: ", err)
	}
	execDir := filepath.Dir(exePath)
	runtimePath := filepath.Join(execDir, runtimeDirName)
	enginePath := filepath.Join(runtimePath, "engine", "ace_console.exe")

	if !fileExists(enginePath) {
		log.Println("📦 No se encontró Lista Canales TV. Extrayendo por primera vez...")
		if err := extractRuntime(runtimePath); err != nil {
			log.Fatal("Error al extraer Lista Canales: ", err)
		}
		log.Println("✅ Lista Canales TV extraído exitosamente.")
	} else {
		log.Println("🔁 Lista Canales TV ya existe. Usando versión existente.")
	}

	log.Println("🚀 Actualizando Lista Canales TV...")
	args := []string{
		"--live-buffer", "60", // 30
		"--vod-buffer", "10",  // 30
		"--client-console",
	}
	cmd := exec.Command(enginePath, args...)
	cmd.Dir = filepath.Join(runtimePath, "engine")

	if err := cmd.Start(); err != nil {
		log.Fatal("No se pudo iniciar: ", err)
	}

	log.Println("⏳ Esperando a que termine de actualizarse la Lista Canales TV...")
	if !waitForAPI(fmt.Sprintf("http://localhost:%d/webui/api/service?method=get_version", httpPort), 30*time.Second) {
		log.Fatal("❌ No respondió después de 30 segundos")
	}

	log.Println("🎉 Lista Canales TV lista. Abriendo interfaz...")
	openBrowser(fmt.Sprintf("http://localhost:%d", httpWebServerPort))

	log.Println("✅ Todo listo. ¡A relajarse y disfrutar del contenido! 🍿")

	return cmd, err
}

// extractRuntime extrae el ZIP embebido en el directorio runtime
func extractRuntime(targetDir string) error {
	// Abrir el ZIP embebido
	zipFile, err := runtimeZip.Open("assets/acestream-runtime-windows.zip")
	if err != nil {
		return fmt.Errorf("no se pudo abrir el ZIP embebido: %w", err)
	}
	defer zipFile.Close()

	// Obtener tamaño
	zipInfo, _ := zipFile.Stat()
	zipSize := zipInfo.Size()

	// Crear reader ZIP
	zipReader, err := zip.NewReader(io.NewSectionReader(zipFile.(io.ReaderAt), 0, zipSize), zipSize)
	if err != nil {
		return fmt.Errorf("no se pudo leer el ZIP: %w", err)
	}

	// Extraer cada archivo
	for _, file := range zipReader.File {
		filePath := filepath.Join(targetDir, file.Name)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return err
			}
			continue
		}

		// Crear directorios intermedios
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		// Extraer archivo
		inFile, err := file.Open()
		if err != nil {
			return fmt.Errorf("no se pudo abrir archivo en ZIP: %s: %w", file.Name, err)
		}
		outFile, err := os.Create(filePath)
		if err != nil {
			inFile.Close()
			return fmt.Errorf("no se pudo crear archivo: %s: %w", filePath, err)
		}

		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("error al copiar %s: %w", file.Name, err)
		}

		// Aplicar permisos (opcional en Windows)
		os.Chmod(filePath, file.Mode())
	}
	return nil
}

// fileExists verifica si un archivo o directorio existe
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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

// openBrowser abre el navegador según el sistema
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		case "darwin":
			cmd = exec.Command("open", url)
		default: // Linux, etc.
			cmd = exec.Command("xdg-open", url)
		}
	}
	_ = cmd.Start()
}
