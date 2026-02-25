//go:build !windows
// +build !windows

package main

import (
	"log"
	"os/exec"
	"syscall"
	"time"
)

func StopTor(cmdTor *exec.Cmd) error {
	if cmdTor == nil || cmdTor.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(cmdTor.Process.Pid)
	if err != nil {
		log.Printf("❌ No se pudo obtener PGID: %v", err)
		return err
	}

	syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(1 * time.Second)
	syscall.Kill(-pgid, syscall.SIGKILL)

	cmdTor.Wait()
	log.Println("✅ TOR cerrado (grupo de procesos terminado)")
	return nil
}