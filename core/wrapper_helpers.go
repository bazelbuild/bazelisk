//go:build windows

package core

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// For .bat files, use cmd.exe and properly escape the command line
func makeBatchScriptCmd(execPath string, args []string) *exec.Cmd {
	builder := strings.Builder{}
	builder.WriteString("/c \"")
	builder.WriteString(syscall.EscapeArg(execPath))
	for _, arg := range args {
		builder.WriteString(" ")
		builder.WriteString(syscall.EscapeArg(arg))
	}
	builder.WriteString("\"")

	cmd := exec.Command(os.Getenv("SystemRoot") + "\\system32\\cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.CmdLine = builder.String()

	return cmd
}

// For .ps1 files, use powershell.exe
func makePowerShellScriptCmd(execPath string, args []string) *exec.Cmd {
	cmd := exec.Command(os.Getenv("SystemRoot") + "\\system32\\WindowsPowerShell\\v1.0\\powershell.exe")
	cmd.Args = append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", execPath}, args...)
	return cmd
}
