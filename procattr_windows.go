//go:build windows
// +build windows

package main

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No hacer nada en Windows
}