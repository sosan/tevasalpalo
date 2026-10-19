//go:build windows
// +build windows

package transport

import "os/exec"

func SetSysProcAttr(cmd *exec.Cmd) {
	// No hacer nada en Windows
}
