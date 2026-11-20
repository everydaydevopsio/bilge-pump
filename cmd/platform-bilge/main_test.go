package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v3"
)

func TestInspectTreatsMissingOptionalCacheAsEmpty(t *testing.T) {
	finding, err := inspect(filepath.Join(t.TempDir(), "not-installed-cache"))
	if err != nil {
		t.Fatalf("inspect missing optional path: %v", err)
	}
	if finding.Items != 0 || finding.SizeBytes != 0 {
		t.Fatalf("missing optional path produced finding: %#v", finding)
	}
}

func TestInspectReturnsTraversalErrors(t *testing.T) {
	root := t.TempDir()
	wantErr := fs.ErrPermission
	_, err := inspectWithWalk(root, func(_ string, _ fs.WalkDirFunc) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("inspect error = %v, want %v", err, wantErr)
	}
}

func TestSelectTargetsValidatesEveryRequestedTarget(t *testing.T) {
	cfg := Config{Targets: []Target{
		{Name: "npm", Enabled: true},
		{Name: "go", Enabled: true},
	}}
	tests := []struct {
		name      string
		selection string
		want      map[string]bool
		wantErr   bool
	}{
		{name: "all", selection: "all"},
		{name: "selected targets", selection: "npm, go", want: map[string]bool{"npm": true, "go": true}},
		{name: "unknown target", selection: "dokcer", wantErr: true},
		{name: "mixed valid and unknown", selection: "npm,dokcer", wantErr: true},
		{name: "empty selection", selection: "", wantErr: true},
		{name: "empty target", selection: "npm,", wantErr: true},
		{name: "all mixed with a target", selection: "all,npm", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectTargets(tt.selection, cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("selectTargets(%q) error = %v, wantErr %v", tt.selection, err, tt.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selectTargets(%q) = %#v, want %#v", tt.selection, got, tt.want)
			}
		})
	}
}

func TestScanTargetReportIncludesPathlessDockerCommands(t *testing.T) {
	target := Target{Name: "docker", Enabled: true, Cmds: [][]string{{"docker", "builder", "prune", "-af"}, {"docker", "system", "prune", "-af"}}}
	report := Report{Findings: map[string][]Finding{}, Totals: map[string]int64{}}
	addTargetToReport(&report, target)
	if findings, ok := report.Findings["docker"]; !ok || len(findings) != 0 {
		t.Fatalf("docker findings = %#v, present=%v; want empty entry", findings, ok)
	}
	if !reflect.DeepEqual(report.PlannedCommands["docker"], target.Cmds) {
		t.Fatalf("planned Docker commands = %#v, want %#v", report.PlannedCommands["docker"], target.Cmds)
	}
}

func TestLoadRequiresExplicitConfigWhenRequested(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := load(missing, false); err == nil {
		t.Fatal("expected missing explicit config to fail")
	}
	if cfg, err := load(missing, true); err != nil || len(cfg.Targets) == 0 {
		t.Fatalf("implicit default config = %#v, %v; want default targets", cfg, err)
	}
}

func TestLoadMigratesLegacyDefaultConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	legacyPath := legacyConfigPath()
	newPath := configPath()
	for _, path := range []string{legacyPath, newPath} {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("config path %q is outside temporary APPDATA %q", path, root)
		}
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := Config{Version: 1, Targets: []Target{{Name: "docker", Enabled: false}}}
	data, err := yaml.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rejectLegacyConfigInit(newPath, false); err == nil {
		t.Fatal("initialization should refuse to replace existing legacy settings")
	}

	got, err := load(configPath(), true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Targets) != 1 || got.Targets[0].Name != "docker" || got.Targets[0].Enabled {
		t.Fatalf("migrated targets = %#v, want disabled docker target", got.Targets)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new config was not written: %v", err)
	}
}

func TestLoadUsesSameDefaultsForLegacyAndMigratedConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	legacyPath := legacyConfigPath()
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(Config{Version: 1, Targets: []Target{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := load(configPath(), true)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := load(configPath(), true)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(first.Targets) == 0 {
		t.Fatal("first migration load returned no default targets")
	}
	if !reflect.DeepEqual(first.Targets, second.Targets) {
		t.Fatalf("first-load targets differ from subsequent load: first=%#v second=%#v", first.Targets, second.Targets)
	}
}

func TestLoadKeepsLegacyConfigWhenMigrationWriteFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	legacyPath := legacyConfigPath()
	newPath := configPath()
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := Config{Version: 1, Targets: []Target{{Name: "docker", Enabled: false}}}
	data, err := yaml.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	oldCreateTemp := createMigrationTemp
	createMigrationTemp = func(string, string) (*os.File, error) { return nil, errors.New("simulated disk error") }
	defer func() { createMigrationTemp = oldCreateTemp }()

	got, err := load(newPath, true)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Targets) != 1 || got.Targets[0].Enabled {
		t.Fatalf("legacy settings were not retained: %#v", got.Targets)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("failed migration left a destination config: %v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy config was not preserved: %v", err)
	}
}

func TestMigrateConfigFileDoesNotReplaceExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := []byte("user config")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateConfigFile(path, []byte("legacy config")); err == nil {
		t.Fatal("migration should not replace a config created concurrently")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("existing config = %q, want %q", got, want)
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".bilge-config-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("migration left temporary files: %v", leftovers)
	}
}

func TestWindowsBatchLauncherPreservesExecutableAndArgumentBoundaries(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows batch invocation test")
	}
	t.Setenv("TEMP", `C:\bilge-expanded`)
	executable := filepath.Join(t.TempDir(), "tool with spaces.cmd")
	if err := os.WriteFile(executable, []byte("@echo off\r\necho %*\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"argument with spaces", "literal & echo INJECTED", "pipe|caret^"}
	cmd, err := windowsBatchCommand(executable, args)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("batch command failed: %v: %s", err, output)
	}
	text := strings.TrimSpace(string(output))
	if strings.Count(text, "\n") != 0 || !strings.Contains(text, `"argument with spaces"`) || !strings.Contains(text, `"literal & echo INJECTED"`) || !strings.Contains(text, `"pipe|caret^"`) {
		t.Fatalf("arguments were changed or split: %q", text)
	}
	if _, err := windowsBatchCommand(executable, []string{`literal " quote`}); err == nil {
		t.Fatal("expected unrepresentable literal quote to be rejected")
	}
	if _, err := windowsBatchCommand(executable, []string{"100%TEMP%"}); err == nil {
		t.Fatal("expected percent expansion syntax to be rejected")
	}
	if _, err := windowsBatchCommand("tool%TEMP%.cmd", nil); err == nil {
		t.Fatal("expected percent expansion syntax in executable path to be rejected")
	}
}
