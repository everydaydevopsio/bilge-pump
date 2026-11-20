//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func windowsBatchCommand(executable string, args []string) (*exec.Cmd, error) {
	parts := make([]string, 0, len(args)+1)
	quotedExecutable, err := quoteCmdArgument(executable)
	if err != nil {
		return nil, fmt.Errorf("invalid batch executable path: %w", err)
	}
	parts = append(parts, quotedExecutable)
	for _, arg := range args {
		quoted, err := quoteCmdArgument(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid batch argument: %w", err)
		}
		parts = append(parts, quoted)
	}
	command := `"` + strings.Join(parts, " ") + `"`
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /D /V:OFF /S /C ` + command}
	return cmd, nil
}

func quoteCmdArgument(arg string) (string, error) {
	if strings.ContainsAny(arg, "\"%\r\n") {
		return "", fmt.Errorf("double quotes, percent signs, and newlines are unsupported by cmd.exe batch argument syntax")
	}
	return `"` + arg + `"`, nil
}
