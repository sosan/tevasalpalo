//go:build windows
// +build windows

package main

import (
	"fmt"
	"os/exec"
)

func StopTor(cmdTor *exec.Cmd) error {
	if cmdTor == nil || cmdTor.Process == nil {
		return nil
	}

	return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmdTor.Process.Pid)).Run()
}