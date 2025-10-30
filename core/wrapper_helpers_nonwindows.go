//go:build !windows

package core

import (
	"os/exec"
)

// Non-Windows stubs to satisfy references; these should never be called on non-Windows.

func makeBatchScriptCmd(execPath string, args []string) *exec.Cmd {
	return exec.Command(execPath, args...)
}

func makePowerShellScriptCmd(execPath string, args []string) *exec.Cmd {
	return exec.Command(execPath, args...)
}
