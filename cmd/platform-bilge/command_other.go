//go:build !windows

package main

import (
	"os/exec"
)

func windowsBatchCommand(executable string, args []string) (*exec.Cmd, error) {
	return exec.Command(executable, args...), nil
}
