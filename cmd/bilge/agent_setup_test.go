package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSetupInstallsSkillsAndRegistersBothClients(t *testing.T) {
	userHome := t.TempDir()
	var calls []string
	lookup := func(name string) (string, error) { return name, nil }
	run := func(cli string, args ...string) (string, error) {
		calls = append(calls, cli+" "+strings.Join(args, " "))
		if len(args) > 1 && args[1] == "get" {
			return "No MCP server found with name: bilge", errors.New("not found")
		}
		return "added", nil
	}
	if err := installAgentIntegrationsAt(userHome, "/tmp/bilge", lookup, run); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{".codex", ".claude"} {
		path := filepath.Join(userHome, agent, "skills", "bilge-disk-savings", "SKILL.md")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, diskSavingsSkill) {
			t.Fatalf("wrong %s skill content", agent)
		}
	}
	if len(calls) != 4 {
		t.Fatalf("got calls %v", calls)
	}
	if calls[1] != "codex mcp add bilge -- /tmp/bilge mcp" || calls[3] != "claude mcp add --scope user bilge -- /tmp/bilge mcp" {
		t.Fatalf("wrong registration calls: %v", calls)
	}
}

func TestAgentSetupPreservesExistingSkillAndMCPRegistration(t *testing.T) {
	userHome := t.TempDir()
	path := filepath.Join(userHome, ".codex", "skills", "bilge-disk-savings", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	err := installAgentIntegrationsAt(userHome, "/tmp/bilge", func(name string) (string, error) { return name, nil }, func(cli string, args ...string) (string, error) {
		calls = append(calls, cli+" "+strings.Join(args, " "))
		return "already configured", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "custom skill" {
		t.Fatalf("existing skill changed: %q, %v", got, err)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "mcp get") || !strings.Contains(calls[1], "mcp get") {
		t.Fatalf("existing MCP registration was replaced: %v", calls)
	}
}

func TestRunInitPreservesConfigAndStillSetsUpAgents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("custom: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := 0
	code := runWithAgentSetup([]string{"--config", path, "--init"}, func() error { called++; return nil })
	if code != 0 || called != 1 {
		t.Fatalf("init returned %d and called setup %d times", code, called)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "custom: true\n" {
		t.Fatalf("existing config changed: %q, %v", got, err)
	}
}
