package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	yaml "gopkg.in/yaml.v3"
)

func TestPlanTaskArgsRoutesYesOnlyToDevCache(t *testing.T) {
	platformTask := platformCleaner()
	got, err := planTaskArgs([]string{"dev-cache", "git-cleaner", platformTask}, nil, true, "/tmp/src", true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"dev-cache":   {"--scan", "/tmp/src", "--clean", "--yes"},
		"git-cleaner": {"--scan", "/tmp/src", "--clean"},
		platformTask:  {"--clean"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planTaskArgs() = %#v, want %#v", got, want)
	}
}

func TestPlanTaskArgsRejectsUnknownTasksBeforeLaunching(t *testing.T) {
	for _, task := range []string{"sh", "./cleaner", "bilge"} {
		t.Run(task, func(t *testing.T) {
			if _, err := planTaskArgs([]string{task}, nil, false, "/tmp/src", false); err == nil {
				t.Fatalf("planTaskArgs accepted unknown task %q", task)
			}
		})
	}
}

func TestPlanTaskArgsRejectsUnsupportedArgumentBeforeLaunching(t *testing.T) {
	_, err := planTaskArgs([]string{"dev-cache", "git-cleaner"}, []string{"--depth", "2"}, true, "/tmp/src", false)
	if err == nil {
		t.Fatal("expected --depth to be rejected for a multi-task cleanup")
	}
}

func TestPlanTaskArgsForwardsDevCacheFilterFlags(t *testing.T) {
	extra := []string{"--exclude", "./src/archive", "--include-only=./src/team/project", "--min-age", "30d", "--max-age=90d"}
	got, err := planTaskArgs([]string{"dev-cache"}, extra, false, "/tmp/src", false)
	if err != nil {
		t.Fatalf("planTaskArgs rejected dev-cache filter flags: %v", err)
	}
	want := map[string][]string{"dev-cache": append(extra, "--scan", "/tmp/src")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planTaskArgs() = %#v, want %#v", got, want)
	}
}

func TestParseBooleanFlagHonorsExplicitValue(t *testing.T) {
	for _, tt := range []struct {
		name     string
		value    string
		hasValue bool
		want     bool
		wantErr  bool
	}{
		{name: "implicit true", want: true},
		{name: "explicit true", value: "true", hasValue: true, want: true},
		{name: "explicit false", value: "false", hasValue: true, want: false},
		{name: "invalid", value: "sometimes", hasValue: true, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBooleanFlag(tt.value, tt.hasValue)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("parseBooleanFlag() = %v, %v; want %v, error=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestLoadConfigWithDefaultsRejectsMissingExplicitConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := loadConfigWithDefaults(missing, false); err == nil {
		t.Fatal("expected missing explicit config to fail")
	}
	if cfg, err := loadConfigWithDefaults(missing, true); err != nil || len(cfg.Tasks) == 0 {
		t.Fatalf("implicit default config = %#v, %v; want default tasks", cfg, err)
	}
}

func TestLoadConfigMigratesRenamedPlatformTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := Config{Version: 1, ScanPath: "/work", Tasks: []string{"dev-cache", "linux-cache-cleaner", "git-cleaner"}}
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}

	got, err := loadConfigWithDefaults(path, false)
	if err != nil {
		t.Fatalf("loadConfigWithDefaults: %v", err)
	}
	want := []string{"dev-cache", "bilge-linux", "git-cleaner"}
	if !reflect.DeepEqual(got.Tasks, want) {
		t.Fatalf("tasks = %#v, want %#v", got.Tasks, want)
	}
	if got.ScanPath != original.ScanPath {
		t.Fatalf("scan path = %q, want %q", got.ScanPath, original.ScanPath)
	}
}

func TestRunMissingExplicitConfigStopsBeforeTasks(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := run([]string{"--config", missing, "clean", "--all"})
	os.Stderr = oldStderr
	_ = writer.Close()
	var output bytes.Buffer
	_, _ = output.ReadFrom(reader)
	_ = reader.Close()
	if code == 0 {
		t.Fatal("run succeeded with a missing explicit config")
	}
	if !bytes.Contains(output.Bytes(), []byte("config file "+missing+" does not exist")) {
		t.Fatalf("expected config error before launching tasks, got %q", output.String())
	}
}

func TestParseBooleanFalseDoesNotRequestCleanup(t *testing.T) {
	clean, err := parseBooleanFlag("false", true)
	if err != nil {
		t.Fatal(err)
	}
	if clean {
		t.Fatal("--clean=false must not request cleanup")
	}
	yes, err := parseBooleanFlag("false", true)
	if err != nil {
		t.Fatal(err)
	}
	if yes {
		t.Fatal("--yes=false must not skip confirmation")
	}
	if cleanupRequested("scan", true, clean, nil) {
		t.Fatal("scan --clean=false must remain a dry run")
	}
	if cleanupRequested("clean", true, clean, nil) {
		t.Fatal("clean --clean=false must remain a dry run")
	}
	if !cleanupRequested("clean", false, false, nil) {
		t.Fatal("clean command should request cleanup without a --clean override")
	}
}

func TestRunInitFalseDoesNotCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")
	_ = run([]string{"--config", path, "--init=false"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("--init=false created config: stat error %v", err)
	}
}

func TestRunForceFalsePreservesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.yaml")
	original := []byte("custom: value\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = run([]string{"--config", path, "--init", "--force=false"})
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("--force=false replaced config: got %q, want %q", got, original)
	}
}

func TestMacCleanerJSONIsStructured(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires the native macOS cleaner")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "bilge-macos")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(root, "bilge-macos")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bilge-macos: %v: %s", err, output)
	}
	path := filepath.Join(binDir, "config.yaml")
	cfg := map[string]any{
		"version": 1,
		"options": map[string]any{},
		"targets": []map[string]any{{"name": "integration", "enabled": true, "paths": []string{binDir}}},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result := runTask("bilge-macos", []string{"--config", path, "--targets", "integration"}, true)
	if result.ExitCode != 0 {
		t.Fatalf("bilge-macos exited %d: %s", result.ExitCode, result.Error)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(result.Output, &report); err != nil {
		t.Fatalf("task output is not a JSON object: %v; output=%s", err, result.Output)
	}
	var findings map[string][]map[string]any
	if err := json.Unmarshal(report["findings"], &findings); err != nil || len(findings["integration"]) == 0 {
		t.Fatalf("structured macOS findings missing: %v; output=%s", err, result.Output)
	}
}
