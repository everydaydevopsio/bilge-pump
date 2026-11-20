package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	version             = "dev"
	commit              = "none"
	date                = "unknown"
	createMigrationTemp = os.CreateTemp
)

type Target struct {
	Name    string     `yaml:"name" json:"name"`
	Enabled bool       `yaml:"enabled" json:"enabled"`
	Paths   []string   `yaml:"paths" json:"paths,omitempty"`
	Cmds    [][]string `yaml:"cmds" json:"cmds,omitempty"`
	Notes   string     `yaml:"notes,omitempty" json:"notes,omitempty"`
}
type Config struct {
	Version int      `yaml:"version"`
	Targets []Target `yaml:"targets"`
}
type Finding struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Items     int    `json:"items"`
}
type Report struct {
	Hostname        string                `json:"hostname,omitempty"`
	OS              string                `json:"os"`
	Arch            string                `json:"arch"`
	When            time.Time             `json:"when"`
	DryRun          bool                  `json:"dry_run"`
	Version         string                `json:"version"`
	Findings        map[string][]Finding  `json:"findings"`
	Totals          map[string]int64      `json:"totals_by_target_bytes"`
	PlannedCommands map[string][][]string `json:"planned_commands"`
	TargetStatus    map[string]string     `json:"target_status,omitempty"`
	Errors          []string              `json:"errors,omitempty"`
}

func addTargetToReport(report *Report, target Target) {
	if report.Findings == nil {
		report.Findings = make(map[string][]Finding)
	}
	if report.Totals == nil {
		report.Totals = make(map[string]int64)
	}
	if report.PlannedCommands == nil {
		report.PlannedCommands = make(map[string][][]string)
	}
	if report.TargetStatus == nil {
		report.TargetStatus = make(map[string]string)
	}
	if _, ok := report.Findings[target.Name]; !ok {
		report.Findings[target.Name] = []Finding{}
	}
	for _, command := range target.Cmds {
		report.PlannedCommands[target.Name] = append(report.PlannedCommands[target.Name], append([]string(nil), command...))
	}
	if len(target.Cmds) == 0 {
		report.TargetStatus[target.Name] = "report-only"
	}
}

func cleanerName() string {
	if runtime.GOOS == "windows" {
		return "bilge-windows"
	}
	return "bilge-linux"
}
func configPath() string {
	return filepath.Join(configRoot(), cleanerName(), "config.yaml")
}
func configRoot() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) && runtime.GOOS != "windows" {
		return x
	}
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".config")
	}
	return d
}
func legacyConfigPath() string {
	name := "linux-cache-cleaner"
	if runtime.GOOS == "windows" {
		name = "windows-cache-cleaner"
	}
	return filepath.Join(configRoot(), name, "config.yaml")
}
func rejectLegacyConfigInit(path string, force bool) error {
	if force || filepath.Clean(path) != filepath.Clean(configPath()) {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(legacyConfigPath()); err == nil {
		return fmt.Errorf("existing configuration found at %s; run a normal command to migrate it, or use --force to replace it", legacyConfigPath())
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func defaultTargets() []Target {
	home, _ := os.UserHomeDir()
	cache, _ := os.UserCacheDir()
	targets := []Target{
		{Name: "npm", Enabled: true, Paths: []string{filepath.Join(home, ".npm")}, Cmds: [][]string{{"npm", "cache", "clean", "--force"}}},
		{Name: "go", Enabled: true, Paths: []string{filepath.Join(home, "go", "pkg", "mod"), filepath.Join(cache, "go-build")}, Cmds: [][]string{{"go", "clean", "-cache", "-testcache", "-modcache"}}},
		{Name: "python", Enabled: true, Paths: []string{filepath.Join(cache, "pip")}, Cmds: [][]string{{"pip", "cache", "purge"}}},
		{Name: "uv", Enabled: true, Paths: []string{filepath.Join(cache, "uv")}, Cmds: [][]string{{"uv", "cache", "clean"}}},
		{Name: "docker", Enabled: true, Paths: []string{}, Cmds: [][]string{{"docker", "builder", "prune", "-af"}, {"docker", "system", "prune", "-af"}}},
	}
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local != "" {
			targets[0].Paths = []string{filepath.Join(local, "npm-cache")}
			targets[2].Paths = []string{filepath.Join(local, "pip", "Cache")}
			targets[3].Paths = []string{filepath.Join(local, "uv", "cache")}
		}
		targets = append(targets, Target{Name: "dotnet", Enabled: true, Paths: []string{filepath.Join(home, ".nuget", "packages")}, Cmds: [][]string{{"dotnet", "nuget", "locals", "all", "--clear"}}})
		if local != "" {
			targets = append(targets, Target{Name: "windows-temp-cache", Enabled: true, Paths: []string{filepath.Join(local, "Temp")}, Notes: "Report only; temporary files can belong to running applications."})
		}
	} else {
		targets = append(targets, Target{Name: "cargo", Enabled: true, Paths: []string{filepath.Join(home, ".cargo", "registry"), filepath.Join(home, ".cargo", "git")}, Cmds: [][]string{{"cargo", "cache", "-a"}}, Notes: "Requires cargo-cache; otherwise this target is report only."})
	}
	return targets
}
func load(path string, allowMissing bool) (Config, error) {
	cfg := Config{Version: 1, Targets: defaultTargets()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			if filepath.Clean(path) == filepath.Clean(configPath()) {
				legacyPath := legacyConfigPath()
				if legacy, legacyErr := os.ReadFile(legacyPath); legacyErr == nil {
					if err := yaml.Unmarshal(legacy, &cfg); err != nil {
						return cfg, fmt.Errorf("parse legacy config %s: %w", legacyPath, err)
					}
					cfg = withDefaultTargets(cfg)
					if err := migrateConfigFile(path, legacy); err != nil {
						fmt.Fprintf(os.Stderr, "warning: could not copy migrated config to %s: %v\n", path, err)
					} else {
						fmt.Fprintf(os.Stderr, "Migrated configuration from %s to %s\n", legacyPath, path)
					}
					return cfg, nil
				} else if !errors.Is(legacyErr, os.ErrNotExist) {
					return cfg, fmt.Errorf("read legacy config %s: %w", legacyPath, legacyErr)
				}
			}
			return cfg, nil
		}
		return cfg, fmt.Errorf("config file %s does not exist", path)
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	cfg = withDefaultTargets(cfg)
	return cfg, nil
}

func withDefaultTargets(cfg Config) Config {
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultTargets()
	}
	return cfg
}

func migrateConfigFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := createMigrationTemp(dir, ".bilge-config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Link(tmpPath, path)
}

func selectTargets(selection string, cfg Config) (map[string]bool, error) {
	selection = strings.TrimSpace(selection)
	if selection == "all" {
		return nil, nil
	}
	if strings.TrimSpace(selection) == "" {
		return nil, fmt.Errorf("target selection must not be empty")
	}

	known := make(map[string]struct{}, len(cfg.Targets))
	for _, target := range cfg.Targets {
		known[target.Name] = struct{}{}
	}
	selected := make(map[string]bool)
	for _, name := range strings.Split(selection, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("target selection contains an empty target")
		}
		if name == "all" {
			return nil, fmt.Errorf(`target "all" cannot be combined with other targets`)
		}
		if _, ok := known[name]; !ok {
			return nil, fmt.Errorf("unknown target %q", name)
		}
		selected[name] = true
	}
	return selected, nil
}

func expand(p string) string {
	h, _ := os.UserHomeDir()
	if p == "~" {
		p = h
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		p = filepath.Join(h, p[2:])
	}
	return os.ExpandEnv(p)
}
func inspect(path string) (Finding, error) {
	return inspectWithWalk(path, filepath.WalkDir)
}

func inspectWithWalk(path string, walk func(string, fs.WalkDirFunc) error) (Finding, error) {
	f := Finding{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return f, nil
		}
		return f, fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		f.SizeBytes = info.Size()
		f.Items = 1
		return f, nil
	}
	err = walk(path, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f.SizeBytes += info.Size()
			f.Items++
		}
		return nil
	})
	if err != nil {
		return f, fmt.Errorf("walk: %w", err)
	}
	return f, nil
}

func targetCommand(executable string, args []string) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(executable), ".cmd") {
		return windowsBatchCommand(executable, args)
	}
	return exec.Command(executable, args...), nil
}

func main() { os.Exit(run()) }
func run() int {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" || arg == "version" {
			fmt.Printf("version %s, commit %s, built at %s\n", version, commit, date)
			return 0
		}
	}
	clean := flag.Bool("clean", false, "Execute target cleanup commands; default is dry-run")
	init := flag.Bool("init", false, "Write starter config and exit")
	force := flag.Bool("force", false, "Overwrite config when used with --init")
	jsonOut := flag.Bool("json", false, "Output JSON report")
	targetsArg := flag.String("targets", "all", "Comma-separated target names or all")
	config := flag.String("config", configPath(), "Config YAML path")
	flag.Parse()
	configExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			configExplicit = true
		}
	})
	if *init {
		if err := rejectLegacyConfigInit(*config, *force); err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			return 1
		}
		if _, err := os.Stat(*config); err == nil {
			if !*force {
				fmt.Fprintln(os.Stderr, "config exists; use --force to overwrite")
				return 1
			}
			backup := fmt.Sprintf("%s.%s", *config, time.Now().Format("20060102-150405.000000000"))
			if err := os.Rename(*config, backup); err != nil {
				fmt.Fprintln(os.Stderr, "config backup error:", err)
				return 1
			}
			fmt.Println("Existing config backed up to:", backup)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "config error:", err)
			return 1
		}
		cfg := Config{Version: 1, Targets: defaultTargets()}
		b, err := yaml.Marshal(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err = os.MkdirAll(filepath.Dir(*config), 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err = os.WriteFile(*config, b, 0600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Starter config written to:", *config)
		return 0
	}
	cfg, err := load(*config, !configExplicit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	selected, err := selectTargets(*targetsArg, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "target selection error:", err)
		return 1
	}
	rep := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, When: time.Now().UTC(), DryRun: !*clean, Version: version, Findings: map[string][]Finding{}, Totals: map[string]int64{}, PlannedCommands: map[string][][]string{}, TargetStatus: map[string]string{}}
	if hostname, err := os.Hostname(); err == nil {
		rep.Hostname = hostname
	}
	progress := os.Stdout
	if *jsonOut {
		progress = os.Stderr
	}
	for _, t := range cfg.Targets {
		if !t.Enabled || (len(selected) > 0 && !selected[t.Name]) {
			continue
		}
		addTargetToReport(&rep, t)
		for _, p := range t.Paths {
			f, inspectErr := inspect(expand(p))
			if inspectErr != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("%s: could not inspect %s: %v", t.Name, p, inspectErr))
				continue
			}
			if f.Items > 0 {
				rep.Findings[t.Name] = append(rep.Findings[t.Name], f)
				rep.Totals[t.Name] += f.SizeBytes
			}
		}
		if *clean {
			for _, argv := range t.Cmds {
				if len(argv) == 0 {
					continue
				}
				exe, e := exec.LookPath(argv[0])
				if e != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %s is not installed", t.Name, argv[0]))
					continue
				}
				cmd, e := targetCommand(exe, argv[1:])
				if e != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", t.Name, e))
					continue
				}
				cmd.Stdout = progress
				cmd.Stderr = progress
				_, _ = fmt.Fprintf(progress, "[%s] %s\n", t.Name, strings.Join(argv, " "))
				if e = cmd.Run(); e != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", t.Name, e))
				}
			}
		}
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		for _, t := range cfg.Targets {
			if !t.Enabled || (len(selected) > 0 && !selected[t.Name]) {
				continue
			}
			fmt.Printf("%-22s %d bytes across %d paths\n", t.Name, rep.Totals[t.Name], len(rep.Findings[t.Name]))
			if !*clean {
				if len(t.Cmds) == 0 {
					fmt.Println("  Report only: no cleanup command configured.")
				}
				for _, command := range t.Cmds {
					fmt.Printf("  Planned cleanup: %s\n", strings.Join(command, " "))
				}
			}
		}
		if !*clean {
			fmt.Println("Dry run. Pass --clean to execute available cleanup commands.")
		}
	}
	if len(rep.Errors) > 0 {
		for _, e := range rep.Errors {
			fmt.Fprintln(os.Stderr, "warning:", e)
		}
		return 3
	}
	return 0
}
