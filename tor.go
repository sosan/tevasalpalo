package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	torDirName        = "socks"
	torAssetNameWin   = "tor-expert-bundle-windows-x86_64.zip"
	torAssetNameLinux = "tor-expert-bundle-linux-x86_64.zip"
	portTor		 = "9050"
)

func RunTor() (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("No se pudo obtener la ruta del ejecutable: ", err)
	}
	execDir := filepath.Dir(exePath)
	if err != nil {
		return nil, err
	}

	var zipTorFile string
	zipTorFile = "assets/" + torAssetNameWin
	if runtime.GOOS == "linux" {
		zipTorFile = "assets/" + torAssetNameLinux
	}

	extractedTorDir := filepath.Join(execDir, torDirName)

	torExecutablePath := filepath.Join(extractedTorDir, "tor", "tor")
	if runtime.GOOS == "windows" {
		torExecutablePath += ".exe"
	}

	if !fileExists(torExecutablePath) {
		log.Println("📦 No se encontró Tor. Extrayendo...")
		if err := extractRuntime(torDirName, zipTorFile); err != nil {
			return nil, err
		}
		log.Println("✅ Tor extraído exitosamente.")
	} else {
		log.Println("🔁 Tor ya existe. Usando versión local.")
	}

	args := []string{
		"--SocksPort", portTor,
		"--GeoIPFile", filepath.Join(extractedTorDir, "data", "geoip"),
		"--GeoIPv6File", filepath.Join(extractedTorDir, "data", "geoip6"),
		// "--ClientOnly", "1", // Asegura modo cliente si es necesario
		"--RunAsDaemon", "0", // Asegura que no se demonice si lo controlas desde Go
	}

	// 5. Crear el comando
	cmd := exec.Command(torExecutablePath, args...)
	cmd.Dir = extractedTorDir

	// stdout, err := cmd.StdoutPipe()
	// if err != nil {
	// 	return nil, err
	// }
	// stderr, err := cmd.StderrPipe()
	// if err != nil {
	// 	return nil, err
	// }
	
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	
	// go func() {
	// 	scanner := bufio.NewScanner(stdout)
	// 	for scanner.Scan() {
	// 		log.Printf("Tor STDOUT: %s", scanner.Text()) // O manejar como prefieras
	// 	}
	// }()

	// go func() {
	// 	scanner := bufio.NewScanner(stderr)
	// 	for scanner.Scan() {
	// 		log.Printf("Tor STDERR: %s", scanner.Text()) // Aquí es probable que aparezcan errores
	// 	}
	// }()
	time.Sleep(10 * time.Second)
	log.Println("🚀 Tor iniciado exitosamente.")
	return cmd, nil // Devuelve el comando sin el error original (si Start tuvo éxito, err es nil)
}

func changeOwnership(path string, newUID, newGID int) error {
	return filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			// Puedes manejar errores específicos aquí si es necesario
			return err
		}
		// Cambia la propiedad del archivo/directorio actual
		if err := os.Chown(filePath, newUID, newGID); err != nil {
			return fmt.Errorf("error cambiando propiedad de %s: %w", filePath, err)
		}
		return nil
	})
}