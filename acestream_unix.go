//go:build !windows
// +build !windows

package main

import (
	"log"
	"os/exec"
	"syscall"
	"time"
)

func StopAceStream(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// fallback si ya no tiene pgid (proceso ya salió)
		_ = cmd.Process.Signal(syscall.SIGTERM)
		time.Sleep(time.Second)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.Sleep(time.Second)
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = cmd.Wait()
	log.Println("✅ Acestream cerrado (grupo de procesos terminado)")
	return nil
}
