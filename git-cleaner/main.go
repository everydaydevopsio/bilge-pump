package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	yaml "gopkg.in/yaml.v3"
)

// ----- Version info -----
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// ----- CLI flags -----
var (
	flagScan   = flag.String("scan", "", "Directory to scan for .git directories (overrides config)")
	flagClean  = flag.Bool("clean", false, "Run git gc in each repository and show disk savings")
	flagConfig = flag.String("config", defaultConfigPath(), "Path to YAML config")
	flagInit   = flag.Bool("init", false, "Write a starter config and exit")
	flagForce  = flag.Bool("force", false, "Overwrite config when used with --init")
	flagJSON   = flag.Bool("json", false, "Output a shared JSON report")
)

type Config struct {
	Version int `yaml:"version"`
	Options struct {
		DefaultScanPath string `yaml:"defaultScanPath"`
	} `yaml:"options"`
}

func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) && runtime.GOOS != "windows" {
		return filepath.Join(xdg, "git-cleaner", "config.yaml")
	}
	if runtime.GOOS == "windows" {
		if dir, err := os.UserConfigDir(); err == nil && dir != "" {
			return filepath.Join(dir, "git-cleaner", "config.yaml")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "git-cleaner", "config.yaml")
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func writeStarterConfig(path string, force bool) error {
	if _, err := os.Stat(path); err == nil {
		if !force {
			return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		}
		backup := fmt.Sprintf("%s.%s", path, time.Now().Format("20060102-150405.000000000"))
		if err := os.Rename(path, backup); err != nil {
			return fmt.Errorf("backing up existing config: %w", err)
		}
		fmt.Println("Existing config backed up to:", backup)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	home, _ := os.UserHomeDir()
	defaultPath := filepath.Join(home, "src")
	if _, err := os.Stat(defaultPath); err != nil {
		defaultPath = home
	}
	cfg := Config{Version: 1}
	cfg.Options.DefaultScanPath = defaultPath
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// ----- Finding types -----

type Finding struct {
	Path      string `json:"path"`
	RepoPath  string `json:"repo_path"` // Parent directory containing .git
	SizeBytes int64  `json:"size_bytes"`
	Items     int    `json:"items"`
}

type Report struct {
	Hostname         string    `json:"hostname,omitempty"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	When             time.Time `json:"when"`
	DryRun           bool      `json:"dry_run"`
	Version          string    `json:"version"`
	ScanPath         string    `json:"scan_path"`
	TotalBytes       int64     `json:"total_bytes"`
	Findings         []Finding `json:"findings"`
	DiskSavingsBytes int64     `json:"disk_savings_bytes,omitempty"`
	DiskSavingsPct   float64   `json:"disk_savings_percent,omitempty"`
	Warnings         []string  `json:"warnings,omitempty"`
}

// ----- Utilities -----

func human(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for i, u := range units {
		v /= 1024
		if v < 1024 || i == len(units)-1 {
			return fmt.Sprintf("%.2f %s", v, u)
		}
	}
	return fmt.Sprintf("%.2f TB", v/1024)
}

func checkVersionFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			return true
		}
	}
	return false
}

func inspectPath(root string) (Finding, error) {
	f := Finding{Path: root}
	fi, err := os.Stat(root)
	if err != nil {
		return f, err
	}
	if !fi.IsDir() {
		f.Items = 1
		f.SizeBytes = fi.Size()
		return f, nil
	}
	errWalk := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		f.Items++
		f.SizeBytes += info.Size()
		return nil
	})
	return f, errWalk
}

// scanDirectory walks through the directory tree and finds all .git directories
func scanDirectory(root string) []Finding {
	var findings []Finding

	root = filepath.Clean(root)
	rootAbs, err := filepath.Abs(root)
	if err == nil {
		root = rootAbs
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		if !d.IsDir() {
			return nil
		}

		// Check if this directory is named .git
		if d.Name() == ".git" {
			// Get the repository path (parent of .git)
			repoPath := filepath.Dir(path)

			// Inspect the .git directory
			f, err := inspectPath(path)
			if err != nil {
				return nil
			}
			f.RepoPath = repoPath
			findings = append(findings, f)

			// Skip subdirectories of .git
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		// Log error but continue - return what we found so far
		fmt.Fprintf(os.Stderr, "Warning: error walking directory: %v\n", err)
	}

	return findings
}

// runGitGC runs git gc in the specified repository directory
func runGitGC(repoPath string) error {
	cmd := exec.Command("git", "gc")
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// displayResults displays findings in a table
func displayResults(findings []Finding, total int64) {
	if len(findings) == 0 {
		fmt.Println("No .git directories found.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("Repository Path", ".git Size", "Items")

	// Sort by size (largest first)
	sortedFindings := make([]Finding, len(findings))
	copy(sortedFindings, findings)
	sort.Slice(sortedFindings, func(i, j int) bool {
		return sortedFindings[i].SizeBytes > sortedFindings[j].SizeBytes
	})

	var totalItems int
	for _, f := range sortedFindings {
		if err := table.Append(f.RepoPath, human(f.SizeBytes), fmt.Sprintf("%d", f.Items)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error appending to table: %v\n", err)
		}
		totalItems += f.Items
	}

	table.Footer("TOTAL", human(total), fmt.Sprintf("%d", totalItems))
	if err := table.Render(); err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering table: %v\n", err)
	}
}

// expandScanPath expands ~ and env vars, resolves to absolute path, and verifies it exists.
// Returns the expanded path or an error.
func expandScanPath(scanPath string) (string, error) {
	if scanPath == "" {
		return "", fmt.Errorf("scan path is required")
	}
	if strings.HasPrefix(scanPath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if len(scanPath) > 1 && scanPath[1] == filepath.Separator {
				scanPath = filepath.Join(home, scanPath[2:])
			} else if len(scanPath) == 1 {
				scanPath = home
			} else {
				scanPath = filepath.Join(home, scanPath[1:])
			}
		}
	}
	scanPath = os.ExpandEnv(scanPath)
	abs, err := filepath.Abs(scanPath)
	if err != nil {
		return "", fmt.Errorf("invalid scan path: %w", err)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return "", fmt.Errorf("scan path does not exist: %s", abs)
	}
	return abs, nil
}

func main() {
	if checkVersionFlag() {
		fmt.Printf("version %s, commit %s, built at %s\n", version, commit, date)
		return
	}

	flag.Parse()

	if *flagInit {
		if err := writeStarterConfig(*flagConfig, *flagForce); err != nil {
			fmt.Fprintln(os.Stderr, "init error:", err)
			os.Exit(1)
		}
		fmt.Println("Starter config written to:", *flagConfig)
		return
	}
	cfg, configErr := loadConfig(*flagConfig)
	if configErr != nil && !os.IsNotExist(configErr) {
		fmt.Fprintln(os.Stderr, "config error:", configErr)
		os.Exit(1)
	}
	scanRoot := *flagScan
	if scanRoot == "" && configErr == nil {
		scanRoot = cfg.Options.DefaultScanPath
	}
	scanPath, err := expandScanPath(scanRoot)
	if err != nil {
		if err.Error() == "scan path is required" {
			fmt.Println("Error: --scan flag is required")
			fmt.Println("Usage: git-cleaner --scan <directory> [--clean]")
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		os.Exit(1)
	}

	progress := os.Stdout
	if *flagJSON {
		progress = os.Stderr
	}
	_, _ = fmt.Fprintf(progress, "Scanning %s for .git directories...\n", scanPath)

	// Initial scan
	findings := scanDirectory(scanPath)

	var totalBefore int64
	for _, f := range findings {
		totalBefore += f.SizeBytes
	}

	rep := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, When: time.Now().UTC(), DryRun: !*flagClean, Version: version, ScanPath: scanPath, TotalBytes: totalBefore, Findings: findings}
	if host, err := os.Hostname(); err == nil {
		rep.Hostname = host
	}
	if len(findings) == 0 {
		if *flagJSON {
			_ = json.NewEncoder(os.Stdout).Encode(rep)
		} else {
			_, _ = fmt.Fprintln(os.Stdout, "No .git directories found.")
		}
		return
	}

	// Display initial results
	if !*flagJSON {
		fmt.Printf("\nFound %d repositories:\n\n", len(findings))
		displayResults(findings, totalBefore)
	}

	// Clean if requested
	if *flagClean {
		_, _ = fmt.Fprintf(progress, "\nRunning git gc in %d repositories...\n", len(findings))
		var errors []string
		var cleanedCount int

		for _, f := range findings {
			_, _ = fmt.Fprintf(progress, "  Cleaning %s...\n", f.RepoPath)
			if err := runGitGC(f.RepoPath); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", f.RepoPath, err))
				continue
			}
			cleanedCount++
		}

		if len(errors) > 0 && !*flagJSON {
			fmt.Println("\nErrors:")
			for _, e := range errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		// Rescan after cleanup
		_, _ = fmt.Fprintln(progress, "\nRescanning after cleanup...")
		time.Sleep(100 * time.Millisecond) // Brief pause to ensure file system updates
		afterFindings := scanDirectory(scanPath)

		var totalAfter int64
		for _, f := range afterFindings {
			totalAfter += f.SizeBytes
		}

		diskSavings := totalBefore - totalAfter
		rep.DiskSavingsBytes = diskSavings
		if totalBefore > 0 {
			rep.DiskSavingsPct = float64(diskSavings) / float64(totalBefore) * 100
		}
		if len(errors) > 0 {
			rep.Warnings = errors
		}
		if !*flagJSON {
			fmt.Printf("\nResults after cleanup:\n\n")
			displayResults(afterFindings, totalAfter)
			fmt.Printf("\nDisk savings: %s (%.2f%%)\n", human(diskSavings), rep.DiskSavingsPct)
			fmt.Printf("Cleaned %d repositories\n", cleanedCount)
		}
	}
	if *flagJSON {
		if err := json.NewEncoder(os.Stdout).Encode(rep); err != nil {
			fmt.Fprintln(os.Stderr, "json error:", err)
			os.Exit(1)
		}
	}
	if *flagClean && len(rep.Warnings) > 0 {
		os.Exit(3)
	}
}
