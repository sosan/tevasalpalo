//go:build windows
// +build windows

package app

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No hacer nada en Windows
}