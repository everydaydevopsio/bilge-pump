package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed skills/bilge-disk-savings/SKILL.md
var diskSavingsSkill []byte

func initConfig(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Println("Existing config preserved:", path)
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := writeConfig(path, force); err != nil {
		return err
	}
	fmt.Println("Starter config written to:", path)
	return nil
}

type agentLookup func(string) (string, error)
type agentRunner func(string, ...string) (string, error)

func installAgentIntegrations() error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bilgePath, err := bilgeExecutablePath()
	if err != nil {
		return err
	}
	return installAgentIntegrationsAt(userHome, bilgePath, exec.LookPath, runAgentCommand)
}

func bilgeExecutablePath() (string, error) {
	actual, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Prefer the invoked symlink when it points to this binary. A Homebrew
	// Caskroom path changes on upgrade, while its bin symlink stays stable.
	invoked, err := exec.LookPath(os.Args[0])
	if err != nil {
		return actual, nil
	}
	invoked, err = filepath.Abs(invoked)
	if err != nil {
		return actual, nil
	}
	a, aErr := os.Stat(actual)
	b, bErr := os.Stat(invoked)
	if aErr == nil && bErr == nil && os.SameFile(a, b) {
		return invoked, nil
	}
	return actual, nil
}

func installAgentIntegrationsAt(userHome, bilgePath string, lookup agentLookup, run agentRunner) error {
	clients := []struct {
		name      string
		skillPath string
		getArgs   []string
		addArgs   []string
	}{
		{"Codex", filepath.Join(userHome, ".codex", "skills", "bilge-disk-savings", "SKILL.md"), []string{"mcp", "get", "bilge", "--json"}, []string{"mcp", "add", "bilge", "--", bilgePath, "mcp"}},
		{"Claude", filepath.Join(userHome, ".claude", "skills", "bilge-disk-savings", "SKILL.md"), []string{"mcp", "get", "bilge"}, []string{"mcp", "add", "--scope", "user", "bilge", "--", bilgePath, "mcp"}},
	}
	var failures []error
	for _, client := range clients {
		if err := installSkillFile(client.skillPath); err != nil {
			failures = append(failures, fmt.Errorf("%s skill: %w", client.name, err))
		}
		cli, err := lookup(strings.ToLower(client.name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s CLI not found; MCP registration skipped (rerun bilge --init after installing it)\n", client.name)
			continue
		}
		output, err := run(cli, client.getArgs...)
		if err == nil {
			fmt.Printf("%s already has an MCP server named bilge; existing registration preserved\n", client.name)
			continue
		}
		if !strings.Contains(strings.ToLower(output), "no mcp server") {
			failures = append(failures, fmt.Errorf("checking %s MCP registration: %s: %w", client.name, strings.TrimSpace(output), err))
			continue
		}
		output, err = run(cli, client.addArgs...)
		if err != nil {
			failures = append(failures, fmt.Errorf("registering %s MCP server: %s: %w", client.name, strings.TrimSpace(output), err))
			continue
		}
		fmt.Printf("Registered bilge MCP server in %s\n", client.name)
	}
	return errors.Join(failures...)
}

func installSkillFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, diskSavingsSkill) {
			fmt.Fprintf(os.Stderr, "Existing skill preserved at %s; review it manually for updates\n", path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(diskSavingsSkill); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	fmt.Println("Installed disk-savings skill:", path)
	return nil
}

func runAgentCommand(cli string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, args...)
	output := &limitWriter{limit: 64 * 1024}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	if output.full {
		return "CLI output exceeded 64 KiB", errors.New("CLI output too large")
	}
	if ctx.Err() != nil {
		return "CLI command timed out", ctx.Err()
	}
	return output.String(), err
}
