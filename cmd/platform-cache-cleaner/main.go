package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	Hostname string               `json:"hostname,omitempty"`
	OS       string               `json:"os"`
	Arch     string               `json:"arch"`
	When     time.Time            `json:"when"`
	DryRun   bool                 `json:"dry_run"`
	Version  string               `json:"version"`
	Findings map[string][]Finding `json:"findings"`
	Totals   map[string]int64     `json:"totals_by_target_bytes"`
	Errors   []string             `json:"errors,omitempty"`
}

func cleanerName() string {
	if runtime.GOOS == "windows" {
		return "windows-cache-cleaner"
	}
	return "linux-cache-cleaner"
}
func configPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) && runtime.GOOS != "windows" {
		return filepath.Join(x, cleanerName(), "config.yaml")
	}
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".config")
	}
	return filepath.Join(d, cleanerName(), "config.yaml")
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
func load(path string) (Config, error) {
	cfg := Config{Version: 1, Targets: defaultTargets()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultTargets()
	}
	return cfg, nil
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
func inspect(path string) Finding {
	f := Finding{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return f
	}
	if !info.IsDir() {
		f.SizeBytes = info.Size()
		f.Items = 1
		return f
	}
	_ = filepath.Walk(path, func(p string, i os.FileInfo, e error) error {
		if e != nil {
			return nil
		}
		if i.Mode().IsRegular() {
			f.SizeBytes += i.Size()
			f.Items++
		}
		return nil
	})
	return f
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
	if *init {
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
	cfg, err := load(*config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	selected := map[string]bool{}
	if *targetsArg != "all" {
		for _, n := range strings.Split(*targetsArg, ",") {
			selected[strings.TrimSpace(n)] = true
		}
	}
	rep := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, When: time.Now().UTC(), DryRun: !*clean, Version: version, Findings: map[string][]Finding{}, Totals: map[string]int64{}}
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
		for _, p := range t.Paths {
			f := inspect(expand(p))
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
				var cmd *exec.Cmd
				if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(exe), ".cmd") {
					cmdline := `"` + exe + `" ` + strings.Join(argv[1:], " ")
					cmd = exec.Command("cmd.exe", "/C", cmdline)
				} else {
					cmd = exec.Command(exe, argv[1:]...)
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
		for n, fs := range rep.Findings {
			fmt.Printf("%-22s %d bytes across %d paths\n", n, rep.Totals[n], len(fs))
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
