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
	"strconv"
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

func loadConfigWithDefaults(path string, allowMissing bool) (Config, error) {
	cfg := Config{Version: 1, ScanPath: defaultScanPath(), Tasks: []string{"dev-cache", "git-cleaner", platformCleaner()}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config file %s does not exist", path)
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	normalizeConfiguredTasks(&cfg)
	if cfg.ScanPath == "" {
		cfg.ScanPath = defaultScanPath()
	}
	if len(cfg.Tasks) == 0 {
		cfg.Tasks = []string{"dev-cache", "git-cleaner", platformCleaner()}
	}
	return cfg, nil
}

func normalizeConfiguredTasks(cfg *Config) bool {
	legacyNames := map[string]string{
		"mac-cache-cleaner":     "bilge-macos",
		"linux-cache-cleaner":   "bilge-linux",
		"windows-cache-cleaner": "bilge-windows",
	}
	changed := false
	for i, task := range cfg.Tasks {
		if normalized, ok := legacyNames[task]; ok {
			cfg.Tasks[i] = normalized
			changed = true
		}
	}
	return changed
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
		return "bilge-macos"
	case "linux":
		return "bilge-linux"
	case "windows":
		return "bilge-windows"
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
	return "", fmt.Errorf("%s executable not found beside bilge or on PATH", task)
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
	path, scanOverride, init, force, jsonMode, yes, forceClean, forceCleanSet, configExplicit := configPath(), "", false, false, false, false, false, false, false
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
			configExplicit = true
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
			parsed, err := parseBooleanFlag(value, hasValue)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid --init value:", err)
				return 1
			}
			init = parsed
		case "--force":
			parsed, err := parseBooleanFlag(value, hasValue)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid --force value:", err)
				return 1
			}
			force = parsed
		case "--json":
			parsed, err := parseBooleanFlag(value, hasValue)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid --json value:", err)
				return 1
			}
			jsonMode = parsed
		case "--yes":
			parsed, err := parseBooleanFlag(value, hasValue)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid --yes value:", err)
				return 1
			}
			yes = parsed
		case "--clean":
			parsed, err := parseBooleanFlag(value, hasValue)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid --clean value:", err)
				return 1
			}
			forceClean, forceCleanSet = parsed, true
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
	cfg, err := loadConfigWithDefaults(path, !configExplicit)
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
	clean := cleanupRequested(command, forceCleanSet, forceClean, extra)
	extra = withoutArgs(extra, "--all", "--clean")
	taskArgs, err := planTaskArgs(selected, extra, clean, cfg.ScanPath, yes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "argument error:", err)
		return 1
	}
	results := make([]TaskResult, 0, len(selected))
	for _, task := range selected {
		if task == "" {
			continue
		}
		results = append(results, runTask(task, taskArgs[task], jsonMode))
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

func parseBooleanFlag(value string, hasValue bool) (bool, error) {
	if !hasValue {
		return true, nil
	}
	return strconv.ParseBool(value)
}

func cleanupRequested(command string, flagSet, flagValue bool, extra []string) bool {
	if flagSet {
		return flagValue
	}
	return command == "clean" || containsArg(extra, "--clean")
}

func withoutArgs(args []string, remove ...string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if !containsArg(remove, arg) {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func planTaskArgs(tasks, extra []string, clean bool, scanPath string, yes bool) (map[string][]string, error) {
	allowed := map[string]map[string]bool{
		"dev-cache":     {"--depth": true, "--languages": true, "--unsafe-clean": false, "--exclude": true, "--include-only": true, "--min-age": true, "--max-age": true},
		"git-cleaner":   {},
		"bilge-macos":   {"--targets": true, "--docker-prune": false, "--details": false, "--check-tools": false, "--list-targets": false},
		"bilge-linux":   {"--targets": true},
		"bilge-windows": {"--targets": true},
	}
	for _, task := range tasks {
		if !isSupportedTask(task) {
			return nil, fmt.Errorf("unsupported task %q", task)
		}
	}
	for i := 0; i < len(extra); i++ {
		key, _, hasValue := strings.Cut(extra[i], "=")
		if !strings.HasPrefix(key, "--") {
			return nil, fmt.Errorf("unexpected positional argument %q", extra[i])
		}
		if key == "--help" || key == "--version" {
			continue
		}
		aritySet := len(tasks) > 0
		needsValue := false
		for _, task := range tasks {
			flags, taskKnown := allowed[task]
			arity, found := flags[key]
			if !taskKnown || !found {
				aritySet = false
				continue
			}
			if arity {
				needsValue = true
			}
		}
		if !aritySet {
			return nil, fmt.Errorf("flag %s is not supported by every selected task", key)
		}
		if needsValue && !hasValue {
			if i+1 >= len(extra) || strings.HasPrefix(extra[i+1], "--") {
				return nil, fmt.Errorf("%s requires a value", key)
			}
			i++
		}
	}
	planned := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		child := append([]string(nil), extra...)
		if task == "dev-cache" || task == "git-cleaner" {
			child = append(child, "--scan", scanPath)
		}
		if clean {
			child = append(child, "--clean")
		}
		if yes && task == "dev-cache" {
			child = append(child, "--yes")
		}
		planned[task] = child
	}
	return planned, nil
}

func isSupportedTask(task string) bool {
	return task == "dev-cache" || task == "git-cleaner" || (platformCleaner() != "" && task == platformCleaner())
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
	fmt.Println("bilge [scan] [flags]")
	fmt.Println("bilge clean --all [flags]")
	fmt.Println("bilge clean <task> [flags]")
	fmt.Println("bilge <task> [flags]")
	fmt.Println("\nTasks: dev-cache, git-cleaner, bilge-macos, bilge-linux, bilge-windows")
	fmt.Println("Flags: --config PATH --init [--force] --json --yes")
	fmt.Println("Examples:")
	fmt.Println("  bilge scan                 # Dry-run all configured cleanup tasks")
	fmt.Println("  bilge clean --all          # Clean all configured tasks")
	fmt.Println("  bilge clean dev-cache      # Clean project caches only")
}
