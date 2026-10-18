//go:build windows
// +build windows

package app

import (
	"fmt"
	"os/exec"
)

func StopAceStream(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
}
