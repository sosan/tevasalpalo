//go:build !windows
// +build !windows

package transport

import (
	"os/exec"
	"syscall"
)

func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
