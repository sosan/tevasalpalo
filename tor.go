package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
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
	zipTorFile = "assets/" + torAssetNameLinux
	if runtime.GOOS == "windows" {
		zipTorFile = "assets/" + torAssetNameWin
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
		"--ClientOnly", "1", // Asegura modo cliente si es necesario
		"--RunAsDaemon", "0",
	}

	cmd := exec.Command(torExecutablePath, args...)
	cmd.Dir = extractedTorDir
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

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

	// go logPipe(stdout, "TOR")
    // go logPipe(stderr, "TOR-ERR")
	time.Sleep(10 * time.Second)
	log.Println("🚀 Tor iniciado exitosamente.")
	return cmd, nil
}

func logPipe(r io.Reader, prefix string) {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := scanner.Text()
        log.Printf("[%s] %s", prefix, line)
    }
}

func changeOwnership(path string, newUID, newGID int) error {
	return filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := os.Chown(filePath, newUID, newGID); err != nil {
			return fmt.Errorf("error cambiando propiedad de %s: %w", filePath, err)
		}
		return nil
	})
}

func StopTor(cmdTor *exec.Cmd) error {
	var err error
	var pgid int
	if cmdTor != nil && cmdTor.Process != nil {
		if runtime.GOOS == "windows" {
			exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmdTor.Process.Pid)).Run()
		} else {
			pgid, err = syscall.Getpgid(cmdTor.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGTERM)
				time.Sleep(1 * time.Second)
				syscall.Kill(-pgid, syscall.SIGKILL)
				cmdTor.Wait()
				log.Println("✅ TOR cerrado (grupo de procesos terminado)")
			} else {
				log.Printf("❌ No se pudo obtener PGID: %v", err)
			}
		}
	}
	return err
}