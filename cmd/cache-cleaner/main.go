package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type Config struct {
	Version  int      `yaml:"version"`
	ScanPath string   `yaml:"scanPath"`
	Tasks    []string `yaml:"tasks"`
}

type TaskResult struct {
	Task     string          `json:"task"`
	ExitCode int             `json:"exit_code"`
	Output   json.RawMessage `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type Report struct {
	Version string       `json:"version"`
	OS      string       `json:"os"`
	Arch    string       `json:"arch"`
	When    time.Time    `json:"when"`
	DryRun  bool         `json:"dry_run"`
	Tasks   []TaskResult `json:"tasks"`
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) && runtime.GOOS != "windows" {
		return filepath.Join(xdg, "bilge-pump", "config.yaml")
	}
	dir := ""
	if runtime.GOOS == "windows" {
		dir, _ = os.UserConfigDir()
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "bilge-pump", "config.yaml")
}

func defaultScanPath() string {
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, "src"), filepath.Join(home, "projects"), home} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "."
}

func loadConfig(path string) (Config, error) {
	cfg := Config{Version: 1, ScanPath: defaultScanPath(), Tasks: []string{"dev-cache", "git-cleaner", platformCleaner()}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.ScanPath == "" {
		cfg.ScanPath = defaultScanPath()
	}
	if len(cfg.Tasks) == 0 {
		cfg.Tasks = []string{"dev-cache", "git-cleaner", platformCleaner()}
	}
	return cfg, nil
}

func writeConfig(path string, force bool) error {
	if _, err := os.Stat(path); err == nil {
		if !force {
			return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		}
		backup := fmt.Sprintf("%s.%s", path, time.Now().Format("20060102-150405.000000000"))
		if err := os.Rename(path, backup); err != nil {
			return fmt.Errorf("backing up existing config: %w", err)
		}
		fmt.Println("Existing config backed up to:", backup)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg := Config{Version: 1, ScanPath: defaultScanPath(), Tasks: []string{"dev-cache", "git-cleaner", platformCleaner()}}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func platformCleaner() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac-cache-cleaner"
	case "linux":
		return "linux-cache-cleaner"
	case "windows":
		return "windows-cache-cleaner"
	default:
		return ""
	}
}

func executableFor(task string) (string, error) {
	name := task
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s executable not found beside cache-cleaner or on PATH", task)
}

func runTask(task string, args []string, jsonMode bool) TaskResult {
	result := TaskResult{Task: task}
	path, err := executableFor(task)
	if err != nil {
		result.ExitCode, result.Error = 1, err.Error()
		return result
	}
	if jsonMode && !containsArg(args, "--clean") {
		args = append(args, "--json")
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	var output bytes.Buffer
	var liveOutput io.Writer = os.Stdout
	if jsonMode {
		liveOutput = os.Stderr
	}
	cmd.Stdout = io.MultiWriter(&output, liveOutput)
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Error = err.Error()
		}
	} else {
		result.ExitCode = 0
	}
	if jsonMode {
		var compact bytes.Buffer
		if containsArg(args, "--clean") {
			encoded, _ := json.Marshal(strings.TrimSpace(output.String()))
			result.Output = encoded
		} else if json.Valid(output.Bytes()) {
			_ = json.Compact(&compact, output.Bytes())
			result.Output = compact.Bytes()
		} else if output.Len() != 0 {
			encoded, _ := json.Marshal(strings.TrimSpace(output.String()))
			result.Output = encoded
		}
	}
	return result
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version" || args[0] == "version") {
		fmt.Printf("version %s, commit %s, built at %s\n", version, commit, date)
		return 0
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		usage()
		return 0
	}
	path, scanOverride, init, force, jsonMode := configPath(), "", false, false, false
	commandArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key, value, hasValue := strings.Cut(arg, "=")
		switch key {
		case "--config":
			if !hasValue {
				i++
				if i >= len(args) {
					fmt.Fprintln(os.Stderr, "--config requires a path")
					return 1
				}
				value = args[i]
			}
			path = value
		case "--scan":
			if !hasValue {
				i++
				if i >= len(args) {
					fmt.Fprintln(os.Stderr, "--scan requires a path")
					return 1
				}
				value = args[i]
			}
			scanOverride = value
		case "--init":
			init = true
		case "--force":
			force = true
		case "--json":
			jsonMode = true
		default:
			commandArgs = append(commandArgs, arg)
		}
	}
	args = commandArgs
	if init {
		if err := writeConfig(path, force); err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			return 1
		}
		fmt.Println("Starter config written to:", path)
		return 0
	}
	cfg, err := loadConfig(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	if scanOverride != "" {
		cfg.ScanPath = scanOverride
	}
	command, selected, extra := parseCommand(args, cfg)
	if command == "clean" {
		for i, a := range extra {
			if a == "--all" {
				selected = cfg.Tasks
				extra = append(extra[:i], extra[i+1:]...)
				break
			}
		}
	}
	clean := command == "clean" || containsArg(extra, "--clean")
	results := make([]TaskResult, 0, len(selected))
	for _, task := range selected {
		childArgs := append([]string{}, extra...)
		switch task {
		case "dev-cache":
			childArgs = append(childArgs, "--scan", cfg.ScanPath)
		case "git-cleaner":
			childArgs = append(childArgs, "--scan", cfg.ScanPath)
		}
		if clean {
			childArgs = append(childArgs, "--clean")
		}
		if task == "" {
			continue
		}
		results = append(results, runTask(task, childArgs, jsonMode))
	}
	if jsonMode {
		report := Report{Version: version, OS: runtime.GOOS, Arch: runtime.GOARCH, When: time.Now().UTC(), DryRun: !clean, Tasks: results}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	}
	if !jsonMode {
		for _, r := range results {
			if r.Error != "" {
				fmt.Fprintln(os.Stderr, r.Error)
			}
		}
	}
	return exitCode(results)
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func parseCommand(args []string, cfg Config) (string, []string, []string) {
	if len(args) == 0 {
		return "scan", cfg.Tasks, nil
	}
	if args[0] == "scan" {
		return "scan", cfg.Tasks, args[1:]
	}
	if args[0] == "clean" {
		if len(args) == 1 || args[1] == "--all" {
			return "clean", cfg.Tasks, args[1:]
		}
		return "clean", []string{normalizeTask(args[1])}, args[2:]
	}
	task := normalizeTask(args[0])
	return "scan", []string{task}, args[1:]
}

func normalizeTask(task string) string {
	switch task {
	case "mac", "macos", "linux", "windows", "system":
		return platformCleaner()
	case "dev", "project":
		return "dev-cache"
	case "git":
		return "git-cleaner"
	default:
		return task
	}
}

func exitCode(results []TaskResult) int {
	failures := 0
	partial := false
	for _, r := range results {
		if r.ExitCode != 0 {
			failures++
		}
		if r.ExitCode == 3 {
			partial = true
		}
	}
	if failures == 0 {
		return 0
	}
	if failures == len(results) && !partial {
		return 1
	}
	return 3
}

func usage() {
	fmt.Println("cache-cleaner [scan] [flags]")
	fmt.Println("cache-cleaner clean --all [flags]")
	fmt.Println("cache-cleaner clean <task> [flags]")
	fmt.Println("cache-cleaner <task> [flags]")
	fmt.Println("\nTasks: dev-cache, git-cleaner, mac-cache-cleaner, linux-cache-cleaner, windows-cache-cleaner")
	fmt.Println("Flags: --config PATH --init [--force] --json")
	fmt.Println("Examples:")
	fmt.Println("  cache-cleaner scan                 # Dry-run all configured cleanup tasks")
	fmt.Println("  cache-cleaner clean --all          # Clean all configured tasks")
	fmt.Println("  cache-cleaner clean dev-cache      # Clean project caches only")
}
