package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	flagClean       = flag.Bool("clean", false, "Delete found cache directories")
	flagYes         = flag.Bool("yes", false, "Skip confirmation prompt for cleanup")
	flagJSON        = flag.Bool("json", false, "Output results as JSON")
	flagConfig      = flag.String("config", defaultConfigPath(), "Path to YAML config")
	flagInit        = flag.Bool("init", false, "Write a starter config to --config and exit")
	flagForce       = flag.Bool("force", false, "Force overwrite existing config (use with --init)")
	flagScan        = flag.String("scan", "", "Directory to scan (overrides config default)")
	flagDepth       = flag.Int("depth", 0, "Max scan depth (0 = use config default, overrides config)")
	flagLangs       = flag.String("languages", "", "Comma-separated list of languages to scan")
	flagExclude     = flag.String("exclude", "", "Comma-separated path patterns to exclude from scan")
	flagInclude     = flag.String("include-only", "", "Comma-separated path patterns to include only")
	flagMinAge      = flag.String("min-age", "", "Only include caches older than this (e.g. 30d, 48h)")
	flagMaxAge      = flag.String("max-age", "", "Only include caches no older than this (e.g. 7d, 24h)")
	flagUnsafeClean = flag.Bool("unsafe-clean", false, "Allow deleting cache paths that are not ignored by Git (unsafe)")
)

// ----- Config types -----

type Config struct {
	Version   int        `yaml:"version"`
	Options   Options    `yaml:"options"`
	Languages []Language `yaml:"languages"`
}

type Options struct {
	DefaultScanPath string   `yaml:"defaultScanPath"`
	MaxDepth        int      `yaml:"maxDepth"`
	DetectLanguage  bool     `yaml:"detectLanguage"` // If true, detect language per directory and search only relevant patterns
	ExcludePaths    []string `yaml:"excludePaths"`
	IncludeOnly     []string `yaml:"includeOnly"`
	MinAge          string   `yaml:"minAge"`
	MaxAge          string   `yaml:"maxAge"`
}

type Language struct {
	Name       string   `yaml:"name"`
	Enabled    bool     `yaml:"enabled"`
	Priority   int      `yaml:"priority"` // Detection priority (lower number = higher priority, default: 5)
	Patterns   []string `yaml:"patterns"`
	Signatures []string `yaml:"signatures"` // Files/directories that indicate this language (e.g., "package.json" for node)
}

// ----- Finding types -----

type Finding struct {
	Path        string    `json:"path"`
	ProjectRoot string    `json:"project_root,omitempty"` // Directory where language was detected
	SizeBytes   int64     `json:"size_bytes"`
	Items       int       `json:"items"`
	Pattern     string    `json:"pattern"`
	Language    string    `json:"language"`
	Err         string    `json:"error,omitempty"`
	ModMax      time.Time `json:"latest_mtime"`
}

type Report struct {
	Hostname string    `json:"hostname"`
	OS       string    `json:"os"`
	Arch     string    `json:"arch"`
	DryRun   bool      `json:"dry_run"`
	Version  string    `json:"version"`
	When     time.Time `json:"when"`
	ScanPath string    `json:"scan_path"`
	MaxDepth int       `json:"max_depth"`
	Total    int64     `json:"total_bytes"`
	Findings []Finding `json:"findings"`
	Warnings []string  `json:"warnings"`
}

type ScanFilters struct {
	ExcludePaths []string
	IncludeOnly  []string
	MinAge       time.Duration
	MaxAge       time.Duration
	Now          time.Time
	Warn         func(string)
}

type compiledPathPattern struct {
	literalPath  string
	literalOnly  bool
	literalBase  string
	patternParts []string
}

// ----- Utilities -----

func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) && runtime.GOOS != "windows" {
		return filepath.Join(xdg, "dev-cache", "config.yaml")
	}
	if runtime.GOOS == "windows" {
		if dir, err := os.UserConfigDir(); err == nil && dir != "" {
			return filepath.Join(dir, "dev-cache", "config.yaml")
		}
	}
	h, _ := os.UserHomeDir()
	if h == "" {
		return "./config.yaml"
	}
	return filepath.Join(h, ".config", "dev-cache", "config.yaml")
}

func ensureDir(p string) error { return os.MkdirAll(filepath.Dir(p), 0o755) }

func home() string { h, _ := os.UserHomeDir(); return h }
func expand(p string) string {
	p = os.ExpandEnv(p)
	if p == "~" {
		return home()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		return filepath.Join(home(), p[2:])
	}
	return p
}

func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func applyFilterOverrides(options Options, values map[string]string, explicitlySet map[string]bool) Options {
	if explicitlySet["exclude"] {
		options.ExcludePaths = splitCSV(values["exclude"])
	}
	if explicitlySet["include-only"] {
		options.IncludeOnly = splitCSV(values["include-only"])
	}
	if explicitlySet["min-age"] {
		options.MinAge = values["min-age"]
	}
	if explicitlySet["max-age"] {
		options.MaxAge = values["max-age"]
	}
	return options
}

func expandFilterPatterns(patterns []string) ([]string, error) {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		var unresolved string
		expanded := os.Expand(strings.TrimSpace(p), func(key string) string {
			value, ok := os.LookupEnv(key)
			if !ok && unresolved == "" {
				unresolved = key
			}
			return value
		})
		if unresolved != "" {
			return nil, fmt.Errorf("environment variable %q is not set", unresolved)
		}
		if strings.TrimSpace(expanded) == "" {
			return nil, fmt.Errorf("path pattern %q expands to an empty value", p)
		}
		out = append(out, expanded)
	}
	return out, nil
}

func compilePathPattern(pattern string) (compiledPathPattern, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return compiledPathPattern{}, false
	}
	homePrefix := pattern == "~" || strings.HasPrefix(pattern, "~/") || strings.HasPrefix(pattern, "~\\")
	if pattern == "~" {
		return compiledPathPattern{literalPath: filepath.Clean(home()), literalOnly: true}, true
	}
	if homePrefix {
		pattern = pattern[2:]
		if runtime.GOOS == "windows" {
			pattern = strings.ReplaceAll(pattern, "/", `\`)
		}
		parts := strings.Split(pattern, string(filepath.Separator))
		normalized := make([]string, 0, len(parts))
		for _, part := range parts {
			switch part {
			case "", ".":
				continue
			case "..":
				if n := len(normalized); n > 0 && normalized[n-1] != ".." {
					normalized = normalized[:n-1]
				} else {
					normalized = append(normalized, part)
				}
			default:
				normalized = append(normalized, part)
			}
		}
		pattern = strings.Join(normalized, string(filepath.Separator))
	} else {
		pattern = filepath.Clean(pattern)
	}
	parts := splitPathParts(pattern)
	globIndex := -1
	for i, part := range parts {
		if strings.ContainsAny(part, "*?[") || (runtime.GOOS != "windows" && strings.Contains(part, "\\")) {
			globIndex = i
			break
		}
	}
	if globIndex < 0 {
		literalPath := pattern
		if homePrefix {
			literalPath = filepath.Join(home(), literalPath)
		} else if !isAbsolutePath(literalPath) && filepath.VolumeName(literalPath) == "" {
			literalPath = filepath.Join(currentWorkingDirectory(), literalPath)
		}
		return compiledPathPattern{literalPath: absolutePath(literalPath), literalOnly: true}, true
	}

	volume := filepath.VolumeName(pattern)
	isAbsolute := isAbsolutePath(pattern)
	literalBase := ""
	if isAbsolute {
		literalBase = string(filepath.Separator)
		if volume != "" {
			literalBase = volume + string(filepath.Separator)
		}
	} else if homePrefix {
		literalBase = home()
	} else if volume != "" {
		literalBase = volume
	}
	for _, part := range parts[:globIndex] {
		literalBase = filepath.Join(literalBase, part)
	}
	if !isAbsolute && volume == "" && !homePrefix {
		literalBase = filepath.Join(currentWorkingDirectory(), literalBase)
	}
	literalBase = absolutePath(literalBase)
	return compiledPathPattern{
		literalBase:  filepath.Clean(literalBase),
		patternParts: append([]string(nil), parts[globIndex:]...),
	}, true
}

func compilePathPatterns(patterns []string) []compiledPathPattern {
	compiled := make([]compiledPathPattern, 0, len(patterns))
	for _, pattern := range patterns {
		if p, ok := compilePathPattern(pattern); ok {
			compiled = append(compiled, p)
		}
	}
	return compiled
}

func currentWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func isAbsolutePath(path string) bool {
	if runtime.GOOS == "windows" && len(path) > 0 && (path[0] == '\\' || path[0] == '/') {
		return true
	}
	if filepath.IsAbs(path) {
		return true
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return (path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')
	}
	return false
}

func absolutePath(path string) string {
	if runtime.GOOS == "windows" && filepath.VolumeName(path) == "" && len(path) > 0 && (path[0] == '\\' || path[0] == '/') {
		return filepath.Clean(filepath.VolumeName(currentWorkingDirectory()) + path)
	}
	if isAbsolutePath(path) && (runtime.GOOS != "windows" || filepath.VolumeName(path) != "") {
		return filepath.Clean(path)
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return filepath.Clean(path)
}

func splitPathParts(path string) []string {
	path = strings.TrimPrefix(path, filepath.VolumeName(path))
	path = strings.Trim(path, string(filepath.Separator))
	if path == "" || path == "." {
		return nil
	}
	return strings.Split(path, string(filepath.Separator))
}

func validatePathPatterns(patterns []string) error {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		for _, component := range splitPathParts(pattern) {
			if err := validateGlobComponent(component); err != nil {
				return fmt.Errorf("invalid path pattern %q: %w", pattern, err)
			}
		}
		if _, err := filepath.Match(pattern, strings.Repeat("x", len(pattern)+1)); err != nil {
			return fmt.Errorf("invalid path pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func validateGlobComponent(pattern string) error {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if runtime.GOOS == "windows" {
				continue
			}
			i++
			if i >= len(pattern) {
				return fmt.Errorf("trailing escape")
			}
		case '[':
			j := i + 1
			if j < len(pattern) && pattern[j] == '^' {
				j++
			}
			ranges := 0
			for {
				if j < len(pattern) && pattern[j] == ']' && ranges > 0 {
					i = j
					break
				}
				lo, next, err := readGlobClassRune(pattern, j)
				if err != nil {
					return err
				}
				j = next
				if j < len(pattern) && pattern[j] == '-' {
					hi, after, err := readGlobClassRune(pattern, j+1)
					if err != nil {
						return err
					}
					if hi < lo {
						return fmt.Errorf("descending character range")
					}
					j = after
				}
				ranges++
			}
		}
	}
	return nil
}

func readGlobClassRune(pattern string, index int) (rune, int, error) {
	if index >= len(pattern) || pattern[index] == '-' || pattern[index] == ']' {
		return 0, index, fmt.Errorf("invalid character class")
	}
	if pattern[index] == '\\' && runtime.GOOS != "windows" {
		index++
		if index >= len(pattern) {
			return 0, index, fmt.Errorf("trailing escape in character class")
		}
	}
	r, size := utf8.DecodeRuneInString(pattern[index:])
	if r == utf8.RuneError && size == 1 {
		return 0, index, fmt.Errorf("invalid UTF-8 in character class")
	}
	return r, index + size, nil
}

func matchesPathComponent(pattern, name string) bool {
	if runtime.GOOS == "windows" {
		pattern = strings.ToLower(pattern)
		name = strings.ToLower(name)
	}
	matched, err := filepath.Match(pattern, name)
	return err == nil && matched
}

func pathIsWithin(path, base string) (bool, string) {
	pathAbs := absolutePath(path)
	baseAbs := absolutePath(base)
	if within, rel := pathIsWithinAbsolute(pathAbs, baseAbs); within {
		return true, rel
	}
	// Lexical comparison cannot see through a symlink alias. Resolve both
	// existing paths as a fallback so equivalent scan and filter paths agree.
	resolvedPath, pathErr := filepath.EvalSymlinks(pathAbs)
	resolvedBase, baseErr := filepath.EvalSymlinks(baseAbs)
	if pathErr == nil && baseErr == nil {
		return pathIsWithinAbsolute(filepath.Clean(resolvedPath), filepath.Clean(resolvedBase))
	}
	return false, ""
}

func pathIsWithinAbsolute(pathAbs, baseAbs string) (bool, string) {
	if runtime.GOOS == "windows" {
		// filepath.Rel compares path components using Go's lexical rules, but
		// Windows filesystems commonly treat component casing as equivalent.
		// Compare normalized separator paths explicitly so literal, glob, and
		// ancestor checks all use the same case-insensitive semantics.
		pathAbs = strings.ToLower(filepath.ToSlash(filepath.Clean(pathAbs)))
		baseAbs = strings.ToLower(filepath.ToSlash(filepath.Clean(baseAbs)))
		if pathAbs == baseAbs {
			return true, "."
		}
		prefix := strings.TrimSuffix(baseAbs, "/") + "/"
		if strings.HasPrefix(pathAbs, prefix) {
			return true, filepath.FromSlash(strings.TrimPrefix(pathAbs, prefix))
		}
		return false, ""
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, rel
	}
	return true, rel
}

func matchesPathPattern(path string, pattern compiledPathPattern) bool {
	if pattern.literalOnly {
		within, _ := pathIsWithin(path, pattern.literalPath)
		return within
	}
	within, rel := pathIsWithin(path, pattern.literalBase)
	if !within {
		return false
	}
	parts := splitPathParts(rel)
	if len(parts) != len(pattern.patternParts) {
		return false
	}
	for i, part := range parts {
		if !matchesPathComponent(pattern.patternParts[i], part) {
			return false
		}
	}
	return true
}

func matchesAnyPattern(path string, patterns []compiledPathPattern) bool {
	for _, pattern := range patterns {
		if matchesPathPattern(path, pattern) {
			return true
		}
	}
	return false
}

func pathMatchesPatternOrAncestor(path string, patterns []compiledPathPattern) bool {
	path = filepath.Clean(path)
	for current := path; ; current = filepath.Dir(current) {
		if matchesAnyPattern(current, patterns) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
}

func mayContainPatternMatch(path string, patterns []compiledPathPattern) bool {
	for _, pattern := range patterns {
		if pattern.literalOnly {
			if targetWithin, _ := pathIsWithin(pattern.literalPath, path); targetWithin {
				return true
			}
			continue
		}
		if prefixWithin, _ := pathIsWithin(pattern.literalBase, path); prefixWithin {
			return true
		}
		within, rel := pathIsWithin(path, pattern.literalBase)
		if !within {
			continue
		}
		pathParts := splitPathParts(rel)
		if len(pathParts) > len(pattern.patternParts) {
			continue
		}
		canMatch := true
		for i, pathPart := range pathParts {
			if !matchesPathComponent(pattern.patternParts[i], pathPart) {
				canMatch = false
				break
			}
		}
		if canMatch {
			return true
		}
	}
	return false
}

func parseAgeDuration(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	s = strings.TrimSpace(strings.ToLower(s))
	unit := s[len(s)-1]
	num := strings.TrimSpace(s[:len(s)-1])
	if num == "" {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}
	var mult time.Duration
	switch unit {
	case 'm':
		mult = time.Minute
	case 'h':
		mult = time.Hour
	case 'd':
		mult = 24 * time.Hour
	case 'w':
		mult = 7 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("unsupported duration unit in %q (use m/h/d/w)", s)
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid duration value: %q", s)
	}
	if n == 0 {
		return 0, fmt.Errorf("duration must be greater than zero: %q", s)
	}
	if n > math.MaxInt64/int64(mult) {
		return 0, fmt.Errorf("duration overflows time.Duration: %q", s)
	}
	return time.Duration(n) * mult, nil
}

func passesAgeFilter(f Finding, filters ScanFilters) bool {
	if !isCacheDirectory(f) {
		return true
	}
	if (filters.MinAge > 0 || filters.MaxAge > 0) && f.Err != "" {
		return false
	}
	if f.ModMax.IsZero() {
		return filters.MinAge <= 0 && filters.MaxAge <= 0
	}
	now := filters.Now
	if now.IsZero() {
		now = time.Now()
	}
	age := now.Sub(f.ModMax)
	if filters.MinAge > 0 && age < filters.MinAge {
		return false
	}
	if filters.MaxAge > 0 && age > filters.MaxAge {
		return false
	}
	return true
}

func recheckAgeEligibility(f Finding, filters ScanFilters, now func() time.Time) (Finding, bool, error) {
	if filters.MinAge <= 0 && filters.MaxAge <= 0 {
		return f, true, nil
	}
	fresh, err := inspectPath(f.Path)
	if err != nil {
		return fresh, false, err
	}
	if fresh.Err != "" {
		return fresh, false, fmt.Errorf("incomplete cache inspection: %s", fresh.Err)
	}
	fresh.Pattern = f.Pattern
	filters.Now = now()
	return fresh, passesAgeFilter(fresh, filters), nil
}

func removeCacheIfEligible(f Finding, filters ScanFilters, checkSafe func() error, now func() time.Time, remove func(string) error) (Finding, bool, bool, error) {
	if checkSafe != nil {
		if err := checkSafe(); err != nil {
			return f, false, false, err
		}
	}
	fresh, eligible, err := recheckAgeEligibility(f, filters, now)
	if err != nil {
		return fresh, false, false, fmt.Errorf("cannot verify age eligibility: %w", err)
	}
	if !eligible {
		return fresh, false, false, nil
	}
	if err := remove(f.Path); err != nil {
		return fresh, false, true, err
	}
	return fresh, true, true, nil
}

func measureFreedBytes(f Finding) (int64, error) {
	remaining, err := inspectPath(f.Path)
	if os.IsNotExist(err) {
		return f.SizeBytes, nil
	}
	if err != nil {
		return 0, err
	}
	if remaining.Err != "" {
		return 0, fmt.Errorf("remaining cache inspection is incomplete: %s", remaining.Err)
	}
	return bytesFreed(f.SizeBytes, remaining.SizeBytes), nil
}

func pathWithScanAlias(path, resolvedRoot, filterRoot string) string {
	if filterRoot == resolvedRoot {
		return path
	}
	rel, err := filepath.Rel(resolvedRoot, path)
	if err != nil {
		return path
	}
	return filepath.Join(filterRoot, rel)
}

func checkVersionFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			return true
		}
	}
	return false
}

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

func inspectPath(root string) (Finding, error) {
	return inspectPathWithWalk(root, filepath.WalkDir)
}

func inspectPathWithWalk(root string, walkDir func(string, fs.WalkDirFunc) error) (Finding, error) {
	f := Finding{Path: root}
	fi, err := os.Stat(root)
	if err != nil {
		return f, err
	}
	if !fi.IsDir() {
		f.Items = 1
		f.SizeBytes = fi.Size()
		f.ModMax = fi.ModTime()
		return f, nil
	}
	errWalk := walkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if f.Err == "" {
				f.Err = err.Error()
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			if f.Err == "" {
				f.Err = e.Error()
			}
			return nil
		}
		f.Items++
		f.SizeBytes += info.Size()
		if info.ModTime().After(f.ModMax) {
			f.ModMax = info.ModTime()
		}
		return nil
	})
	if errWalk != nil && f.Err == "" {
		f.Err = errWalk.Error()
	}
	return f, errWalk
}

// isCacheDirectory returns true if the finding represents a cache directory (has a non-empty pattern)
func isCacheDirectory(f Finding) bool {
	return f.Pattern != ""
}

// getLanguageForExclusion returns the appropriate language string based on detected language and exclusion status.
// Excluded directories should have empty language (not "no language found").
// If not excluded and no language detected, returns "no language found".
// Otherwise returns the detected language.
func getLanguageForExclusion(detectedLang string, isExcluded bool) string {
	if isExcluded {
		return ""
	}
	if detectedLang == "" {
		return "no language found"
	}
	return detectedLang
}

// ----- Config IO -----

func defaultConfig() Config {
	return Config{
		Version: 1,
		Options: Options{
			DefaultScanPath: "~/src",
			MaxDepth:        1,
			DetectLanguage:  true,
			ExcludePaths:    []string{},
			IncludeOnly:     []string{},
			MinAge:          "",
			MaxAge:          "",
		},
		Languages: []Language{
			{Name: "node", Enabled: true, Priority: 10, Patterns: []string{"node_modules", ".npm", ".yarn", ".pnpm-store"}, Signatures: []string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml"}},
			{Name: "python", Enabled: true, Priority: 5, Patterns: []string{".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".tox"}, Signatures: []string{"requirements.txt", "setup.py", "pyproject.toml", "Pipfile", "setup.cfg"}},
			{Name: "go", Enabled: true, Priority: 5, Patterns: []string{"vendor"}, Signatures: []string{"go.mod", "go.sum", "Gopkg.toml"}},
			{Name: "rust", Enabled: true, Priority: 5, Patterns: []string{"target"}, Signatures: []string{"Cargo.toml", "Cargo.lock"}},
			{Name: "java", Enabled: true, Priority: 5, Patterns: []string{"target", ".gradle", "build"}, Signatures: []string{"pom.xml", "build.gradle", "build.gradle.kts", ".mvn"}},
			{Name: "nextjs", Enabled: true, Priority: 1, Patterns: []string{"node_modules", ".npm", ".yarn", ".pnpm-store", ".next", "dist", "build", "out", ".cache"}, Signatures: []string{"next.config.js", "next.config.ts", "next.config.mjs"}},
			{Name: "vue", Enabled: true, Priority: 2, Patterns: []string{"node_modules", ".npm", ".yarn", ".pnpm-store", ".nuxt", "dist", "build", "out", ".cache", ".parcel-cache"}, Signatures: []string{"nuxt.config.js", "nuxt.config.ts", "nuxt.config.mjs", "vue.config.js"}},
			{Name: "php", Enabled: true, Priority: 5, Patterns: []string{"vendor"}, Signatures: []string{"composer.json", "composer.lock"}},
			{Name: "ruby", Enabled: true, Priority: 5, Patterns: []string{"vendor/bundle"}, Signatures: []string{"Gemfile", "Gemfile.lock", "Rakefile"}},
			{Name: "dotnet", Enabled: true, Priority: 5, Patterns: []string{"bin", "obj"}, Signatures: []string{"*.csproj", "*.sln", "*.fsproj", "*.vbproj", "project.json"}},
			{Name: "cpp", Enabled: true, Priority: 5, Patterns: []string{"cmake-build-*"}, Signatures: []string{"CMakeLists.txt", "Makefile", "configure", "configure.ac"}},
			{Name: "flutter", Enabled: true, Priority: 5, Patterns: []string{"build", ".dart_tool"}, Signatures: []string{"pubspec.yaml", "pubspec.lock"}},
		},
	}
}

func writeStarterConfig(path string, force bool) error {
	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		if !force {
			return fmt.Errorf("config file already exists at %s. Use --force to overwrite", path)
		}
		// Backup existing file with timestamp
		backupPath := fmt.Sprintf("%s.%s", path, time.Now().Format("20060102-150405"))
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
		fmt.Printf("Existing config backed up to: %s\n", backupPath)
	}

	starter := defaultConfig()
	if err := ensureDir(path); err != nil {
		return err
	}
	b, err := yaml.Marshal(starter)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func loadConfig(path string) (*Config, error) {
	return loadConfigWithDefaults(path, true)
}

func loadConfigWithDefaults(path string, allowMissing bool) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && os.IsNotExist(err) && filepath.Clean(path) == filepath.Clean(defaultConfigPath()) {
			cfg := defaultConfig()
			return &cfg, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ----- Main -----

func main() {
	if checkVersionFlag() {
		fmt.Printf("version %s, commit %s, built at %s\n", version, commit, date)
		return
	}

	flag.Parse()
	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})
	configExplicit := explicitFlags["config"]

	if *flagInit {
		if err := writeStarterConfig(*flagConfig, *flagForce); err != nil {
			fmt.Println("init error:", err)
			os.Exit(1)
		}
		fmt.Println("Starter config written to:", *flagConfig)
		return
	}

	cfg, err := loadConfigWithDefaults(*flagConfig, !configExplicit)
	if err != nil {
		fmt.Println("config error:", err)
		fmt.Println("Tip: run with --init to create a starter config")
		os.Exit(1)
	}
	filterOptions := applyFilterOverrides(cfg.Options, map[string]string{
		"exclude":      *flagExclude,
		"include-only": *flagInclude,
		"min-age":      *flagMinAge,
		"max-age":      *flagMaxAge,
	}, explicitFlags)
	filterOptions.ExcludePaths, err = expandFilterPatterns(filterOptions.ExcludePaths)
	if err != nil {
		fmt.Println("invalid path filter:", err)
		os.Exit(1)
	}
	filterOptions.IncludeOnly, err = expandFilterPatterns(filterOptions.IncludeOnly)
	if err != nil {
		fmt.Println("invalid path filter:", err)
		os.Exit(1)
	}
	for _, patterns := range [][]string{filterOptions.ExcludePaths, filterOptions.IncludeOnly} {
		if err := validatePathPatterns(patterns); err != nil {
			fmt.Println("invalid path filter:", err)
			os.Exit(1)
		}
	}

	// Determine scan path
	scanPath := cfg.Options.DefaultScanPath
	if *flagScan != "" {
		scanPath = *flagScan
	}
	scanPath = expand(scanPath)

	// Determine max depth
	maxDepth := cfg.Options.MaxDepth
	if *flagDepth > 0 {
		maxDepth = *flagDepth
	}

	minAgeStr := filterOptions.MinAge
	maxAgeStr := filterOptions.MaxAge
	minAge, err := parseAgeDuration(minAgeStr)
	if err != nil {
		fmt.Println("invalid --min-age:", err)
		os.Exit(1)
	}
	maxAge, err := parseAgeDuration(maxAgeStr)
	if err != nil {
		fmt.Println("invalid --max-age:", err)
		os.Exit(1)
	}

	filters := ScanFilters{
		ExcludePaths: filterOptions.ExcludePaths,
		IncludeOnly:  filterOptions.IncludeOnly,
		MinAge:       minAge,
		MaxAge:       maxAge,
		Now:          time.Now(),
	}

	// Filter languages
	selectedLangs := map[string]bool{}
	if *flagLangs != "" {
		for _, l := range strings.Split(*flagLangs, ",") {
			if s := strings.TrimSpace(strings.ToLower(l)); s != "" {
				selectedLangs[s] = true
			}
		}
	}

	var languages []Language
	var allPatterns []string
	patternToLang := make(map[string]string)
	langSignatures := make(map[string][]string)
	langPriorities := make(map[string]int)
	langToPatterns := make(map[string][]string)
	for _, lang := range cfg.Languages {
		if !lang.Enabled {
			continue
		}
		if len(selectedLangs) > 0 && !selectedLangs[strings.ToLower(lang.Name)] {
			continue
		}
		languages = append(languages, lang)
		for _, pattern := range lang.Patterns {
			allPatterns = append(allPatterns, pattern)
			patternToLang[pattern] = lang.Name
		}
		// Build language signature, priority, and pattern maps
		if len(lang.Signatures) > 0 {
			langSignatures[lang.Name] = lang.Signatures
		}
		// Use priority from config, default to 5 if not set (0 means not set in YAML)
		priority := lang.Priority
		if priority == 0 {
			priority = 5
		}
		langPriorities[lang.Name] = priority
		langToPatterns[lang.Name] = lang.Patterns
	}

	if len(languages) == 0 {
		fmt.Println("No languages selected.")
		os.Exit(0)
	}

	rep := Report{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		DryRun:   !*flagClean,
		Version:  version,
		When:     time.Now(),
		ScanPath: scanPath,
		MaxDepth: maxDepth,
		Findings: []Finding{},
		Warnings: []string{},
	}
	if h, _ := os.Hostname(); h != "" {
		rep.Hostname = h
	}
	filters.Warn = func(message string) {
		rep.Warnings = append(rep.Warnings, message)
	}

	// Scan for cache directories
	progress := os.Stdout
	if *flagJSON {
		progress = os.Stderr
	}
	_, _ = fmt.Fprintf(progress, "Scanning %s (max depth: %d)...\n", scanPath, maxDepth)
	if cfg.Options.DetectLanguage {
		_, _ = fmt.Fprintln(progress, "Language detection enabled - scanning with language-specific patterns")
	}
	findings := scanDirectory(scanPath, maxDepth, allPatterns, patternToLang, cfg.Options.DetectLanguage, langSignatures, langPriorities, langToPatterns, filters)
	rep.Findings = findings

	var total int64
	for _, f := range findings {
		total += f.SizeBytes
	}
	rep.Total = total

	// Output results
	if *flagJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Println("json error:", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	fmt.Printf("Config: %s\n", *flagConfig)
	fmt.Printf("Scan: %s\n", rep.When.Format(time.RFC3339))
	fmt.Printf("Dry-run: %v\n\n", rep.DryRun)

	if len(findings) == 0 {
		fmt.Println("No cache directories found.")
		return
	}

	// Display results
	displayDetailed(findings, total)

	partialFailure := false
	// Cleanup if requested
	if *flagClean {
		cacheFindings, _ := filterCacheFindings(findings)

		if len(cacheFindings) == 0 {
			fmt.Println("\nNo cache directories found to delete.")
			return
		}

		if !*flagYes {
			fmt.Printf("\nWARNING: This will delete %d cache directories:\n", len(cacheFindings))
			// Sort findings by size (largest first) for display
			sortedFindings := make([]Finding, len(cacheFindings))
			copy(sortedFindings, cacheFindings)
			sort.Slice(sortedFindings, func(i, j int) bool {
				return sortedFindings[i].SizeBytes > sortedFindings[j].SizeBytes
			})
			for _, f := range sortedFindings {
				if !*flagUnsafeClean {
					allowed, reason := safeCachePath(f)
					if !allowed {
						fmt.Printf("  - SKIP %s (%s)\n", f.Path, reason)
						continue
					}
				}
				fmt.Printf("  - %s (%s)\n", f.Path, human(f.SizeBytes))
			}
			fmt.Printf("\nContinue? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("input error:", err)
				return
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Println("Cancelled.")
				return
			}
		}

		fmt.Println("\nDeleting cache directories...")
		var deletedCount int
		var freedBytes int64
		var cleanupErrors []string

		for _, f := range cacheFindings {
			var checkSafe func() error
			if !*flagUnsafeClean {
				checkSafe = func() error {
					allowed, reason := safeCachePath(f)
					if !allowed {
						return errors.New(reason)
					}
					return nil
				}
			}
			fresh, deleted, attempted, deleteErr := removeCacheIfEligible(f, filters, checkSafe, time.Now, os.RemoveAll)
			if deleteErr != nil {
				if !attempted {
					cleanupErrors = append(cleanupErrors, fmt.Sprintf("skipped %s: %v", f.Path, deleteErr))
					continue
				}
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("deletion incomplete for %s: %v", f.Path, deleteErr))
				freed, measureErr := measureFreedBytes(fresh)
				if measureErr != nil {
					cleanupErrors = append(cleanupErrors, fmt.Sprintf("could not measure partial deletion of %s: %v", f.Path, measureErr))
				} else {
					freedBytes += freed
				}
				if _, statErr := os.Stat(f.Path); os.IsNotExist(statErr) {
					deletedCount++
				}
				continue
			}
			if !deleted {
				cleanupErrors = append(cleanupErrors, fmt.Sprintf("skipped %s: age filter no longer passes", f.Path))
				continue
			}
			deletedCount++
			freedBytes += fresh.SizeBytes
		}

		fmt.Printf("\nDeleted %d directories", deletedCount)
		if freedBytes > 0 {
			fmt.Printf(", freed %s", human(freedBytes))
		}
		fmt.Println()

		if len(cleanupErrors) > 0 {
			partialFailure = true
			fmt.Println("\nErrors:")
			for _, e := range cleanupErrors {
				fmt.Printf("  - %s\n", e)
			}
		}
	}

	if len(rep.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range rep.Warnings {
			fmt.Println(" -", w)
		}
	}
	if partialFailure {
		os.Exit(3)
	}
}

func safeCachePath(f Finding) (bool, string) {
	start := filepath.Dir(f.Path)
	if start == "." || start == "" {
		return false, "project root is unknown; pass --unsafe-clean to override"
	}
	cmd := exec.Command("git", "-C", start, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return false, "not inside a Git repository; pass --unsafe-clean to override"
	}
	root := strings.TrimSpace(string(out))
	cachePath := f.Path
	resolved, err := filepath.EvalSymlinks(cachePath)
	if err != nil {
		return false, "could not resolve cache path before checking nested repositories"
	}
	cachePath = resolved
	nestedGitRepoErr := errors.New("nested Git repository found")
	err = filepath.WalkDir(cachePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == ".git" {
			return fmt.Errorf("%w at %s", nestedGitRepoErr, path)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, nestedGitRepoErr) {
			return false, err.Error() + "; pass --unsafe-clean to override"
		}
		return false, "could not inspect cache contents for nested Git repositories"
	}
	rel, err := filepath.Rel(root, cachePath)
	if err != nil {
		return false, "cache path is outside its Git repository"
	}
	if runtime.GOOS == "windows" {
		rel = filepath.ToSlash(rel)
	}
	tracked := exec.Command("git", "-C", root, "ls-files", "--", rel)
	if out, err := tracked.Output(); err != nil {
		return false, "could not check Git tracking status"
	} else if len(out) > 0 {
		return false, "contains Git-tracked files; pass --unsafe-clean to override"
	}
	ignored := exec.Command("git", "-C", root, "check-ignore", "-q", "--no-index", "--", rel)
	if err := ignored.Run(); err != nil {
		return false, "not ignored by Git; pass --unsafe-clean to override"
	}
	return true, ""
}

// filterCacheFindings returns only findings representing cache directories and the total bytes they consume.
func filterCacheFindings(findings []Finding) ([]Finding, int64) {
	var cacheFindings []Finding
	var total int64
	for _, f := range findings {
		if isCacheDirectory(f) {
			cacheFindings = append(cacheFindings, f)
			total += f.SizeBytes
		}
	}
	return cacheFindings, total
}

// totalCacheBytes calculates the total bytes consumed by cache directories within the findings slice.
func totalCacheBytes(findings []Finding) int64 {
	var total int64
	for _, f := range findings {
		if isCacheDirectory(f) {
			total += f.SizeBytes
		}
	}
	return total
}

// bytesFreed computes the positive difference between the totals recorded before and after cleanup.
func bytesFreed(before, after int64) int64 {
	if before <= after {
		return 0
	}
	return before - after
}

// detectLanguage checks a directory for language signature files and returns the detected language name
// Returns empty string if no language is detected
// Languages are checked in priority order (more specific frameworks first)
// Multiple languages can have the same priority - they will be checked in map iteration order
func detectLanguage(dirPath string, langSignatures map[string][]string, langPriorities map[string]int) string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return ""
	}

	// Build list of languages and sort by priority
	type langEntry struct {
		name       string
		signatures []string
		priority   int
	}
	var langs []langEntry
	for lang, signatures := range langSignatures {
		priority, ok := langPriorities[lang]
		if !ok || priority == 0 {
			priority = 5 // Default priority for languages not in the map or not set
		}
		langs = append(langs, langEntry{
			name:       lang,
			signatures: signatures,
			priority:   priority,
		})
	}

	// Sort by priority (lower number = higher priority = checked first)
	// Languages with the same priority will maintain their relative order
	sort.Slice(langs, func(i, j int) bool {
		return langs[i].priority < langs[j].priority
	})

	// Check for signature files in priority order
	for _, langEntry := range langs {
		for _, sig := range langEntry.signatures {
			// Handle wildcard patterns (e.g., "*.csproj")
			if strings.HasPrefix(sig, "*.") {
				ext := strings.TrimPrefix(sig, "*")
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), strings.ToLower(ext)) {
						return langEntry.name
					}
				}
			} else {
				// Exact file match
				for _, entry := range entries {
					if !entry.IsDir() && entry.Name() == sig {
						return langEntry.name
					}
				}
			}
		}
	}

	return ""
}

// scanDirectory walks through the directory tree up to maxDepth and matches directory names against patterns
func scanDirectory(root string, maxDepth int, patterns []string, patternToLang map[string]string, detectLang bool, langSignatures map[string][]string, langPriorities map[string]int, langToPatterns map[string][]string, filters ScanFilters) []Finding {
	return scanDirectoryWithInspect(root, maxDepth, patterns, patternToLang, detectLang, langSignatures, langPriorities, langToPatterns, filters, inspectPath)
}

func scanDirectoryWithInspect(root string, maxDepth int, patterns []string, patternToLang map[string]string, detectLang bool, langSignatures map[string][]string, langPriorities map[string]int, langToPatterns map[string][]string, filters ScanFilters, inspect func(string) (Finding, error)) []Finding {
	var findings []Finding
	excludePathPatterns := compilePathPatterns(filters.ExcludePaths)
	includePathPatterns := compilePathPatterns(filters.IncludeOnly)

	// Directories that should not have language detection performed
	// These directories will still be listed but with null/empty language
	excludedFromLanguageDetection := []string{
		".git",
	}

	// Normalize root path for consistent separator counting
	root = filepath.Clean(root)
	rootAbs, err := filepath.Abs(root)
	if err == nil {
		root = rootAbs
	}
	filterRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		resolvedRoot := filepath.Clean(resolved)
		if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
			root = resolvedRoot
		}
	}

	// Cache for language detection results to avoid repeated directory reads
	langCache := make(map[string]string)
	// Track depth 0 directories without languages to report at max depth
	depth0NoLang := make(map[string]bool)
	// Track all depth 0 directories that were scanned (for final reporting)
	depth0Scanned := make(map[string]bool)
	// Map to track findings by path for O(1) lookups (only for findings with non-empty Pattern)
	findingsByPath := make(map[string]bool)
	// Track excluded directories (will be reported with empty language)
	excludedDirs := make(map[string]bool)
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		if !d.IsDir() {
			return nil
		}

		// Normalize and get absolute path for comparison
		cleanPath := filepath.Clean(path)
		pathAbs, err := filepath.Abs(cleanPath)
		if err == nil {
			cleanPath = pathAbs
		}

		// Skip the root directory itself
		if cleanPath == root {
			return nil
		}

		// Calculate current depth relative to root
		// Count separators in the relative path from root
		relPath, err := filepath.Rel(root, cleanPath)
		if err != nil {
			// Can't calculate relative path, skip
			return nil
		}
		filterPath := cleanPath
		if filterRoot != root {
			filterPath = filepath.Join(filterRoot, relPath)
		}
		canonicalPath := cleanPath
		if resolved, err := filepath.EvalSymlinks(cleanPath); err == nil {
			canonicalPath = filepath.Clean(resolved)
		}

		// Count separators in relative path to get depth
		// Empty relPath means root (shouldn't happen due to check above)
		// "proj1" = depth 0 (but we treat as depth 1 for the first level)
		// "proj1/node_modules" = depth 1
		depth := strings.Count(relPath, string(filepath.Separator))
		if depth > maxDepth {
			return filepath.SkipDir
		}

		included := len(includePathPatterns) == 0 || pathMatchesPatternOrAncestor(cleanPath, includePathPatterns) || pathMatchesPatternOrAncestor(filterPath, includePathPatterns) || pathMatchesPatternOrAncestor(canonicalPath, includePathPatterns)
		if len(includePathPatterns) > 0 {
			if !included && !mayContainPatternMatch(cleanPath, includePathPatterns) && !mayContainPatternMatch(filterPath, includePathPatterns) && !mayContainPatternMatch(canonicalPath, includePathPatterns) {
				return filepath.SkipDir
			}
		} else if pathMatchesPatternOrAncestor(cleanPath, excludePathPatterns) || pathMatchesPatternOrAncestor(filterPath, excludePathPatterns) || pathMatchesPatternOrAncestor(canonicalPath, excludePathPatterns) {
			return filepath.SkipDir
		}

		// Determine which patterns to check for this directory
		patternsToCheck := patterns
		var detectedLang string
		var projectRoot string

		// First, check if this directory matches a cache pattern
		// Cache directories should not be treated as project roots
		dirName := filepath.Base(cleanPath)
		matchesCachePattern := false
		for _, pattern := range patterns {
			if matchPattern(dirName, pattern) {
				matchesCachePattern = true
				break
			}
		}

		// Check if this directory should be excluded from language detection
		isExcluded := false
		for _, excluded := range excludedFromLanguageDetection {
			if dirName == excluded {
				isExcluded = true
				break
			}
		}

		if detectLang {
			// Detect language at project root level (depth 0 or 1)
			// For depth 0, this is a direct child of root (first project level)
			// For depth > 0, we're deeper in the tree - check parent directories for language
			if depth == 0 && !matchesCachePattern {
				// This is a project root directory - detect language here
				// But skip if it matches a cache pattern (cache dirs are not project roots)
				projectRoot = cleanPath
			} else if depth == 0 && matchesCachePattern {
				// This is a cache directory at depth 0 - use scan root as project root
				projectRoot = root
			} else {
				// We're deeper in the tree - walk up to find the project root
				// Find the project root (first directory at depth 0 from root)
				parts := strings.Split(relPath, string(filepath.Separator))
				if len(parts) > 0 {
					projectRootAbs, err := filepath.Abs(filepath.Join(root, parts[0]))
					if err == nil {
						projectRoot = projectRootAbs
					} else {
						projectRoot = filepath.Join(root, parts[0])
					}
				}
			}

			// Track depth 0 directories
			if depth == 0 {
				depth0Scanned[projectRoot] = true
			}

			// If this directory is excluded, skip language detection and mark it
			if isExcluded {
				// If this is the project root itself (depth 0), mark the project root as excluded
				if depth == 0 {
					excludedDirs[projectRoot] = true
					langCache[projectRoot] = "" // Explicitly set to empty, not "no language found"
				}
				detectedLang = ""
				// Skip all language detection for this directory at any depth
			}

			// Check cache first for project root
			if cachedLang, ok := langCache[projectRoot]; ok {
				detectedLang = cachedLang
				// Remove from tracking if language was found
				if detectedLang != "" {
					delete(depth0NoLang, projectRoot)
				}
			} else if projectRoot != "" && !excludedDirs[projectRoot] {
				// Detect language at project root and cache result (skip if excluded)
				detectedLang = detectLanguage(projectRoot, langSignatures, langPriorities)
				if projectRoot != "" {
					langCache[projectRoot] = detectedLang
					// Track depth 0 directories without languages
					if depth == 0 && detectedLang == "" {
						depth0NoLang[projectRoot] = true
					}
				}
			}

			// If no language found at project root and we haven't reached max depth,
			// try detecting language at current directory level (might be a nested project)
			if detectedLang == "" && depth < maxDepth && !excludedDirs[projectRoot] {
				// Try detecting language at current directory (skip if excluded)
				currentLang := detectLanguage(cleanPath, langSignatures, langPriorities)
				if currentLang != "" {
					detectedLang = currentLang
					// Cache this as a project root for this subtree
					langCache[cleanPath] = detectedLang
				}
				// Continue walking deeper to find language or reach max depth
				// Don't return early - we still need to scan for patterns at this level
			}

			// Try detecting language at current level if we're at max depth and haven't found one yet
			if depth == maxDepth && detectedLang == "" && !excludedDirs[projectRoot] {
				// Try one more time to detect language at current level (skip if excluded)
				currentLang := detectLanguage(cleanPath, langSignatures, langPriorities)
				if currentLang == "" {
					// Only report "no language found" for directories at depth 0 (project roots)
					// Don't report for subdirectories like .git, docs, etc. that are not project roots
					if depth == 0 {
						f := Finding{
							Path:        path,
							ProjectRoot: cleanPath, // Same as Path since this is the project root
							Language:    getLanguageForExclusion("", excludedDirs[cleanPath]),
							Pattern:     "",
							SizeBytes:   0,
							Items:       0,
							ModMax:      time.Time{},
						}
						findings = append(findings, f)
						// Remove from tracking since we've reported it
						delete(depth0NoLang, cleanPath)
					}
					// At max depth with no language, still scan for patterns using all patterns
					// (patternsToCheck already contains all patterns)
					// Don't report "no language found" here - pattern matching happens below
					// and fallback reporting happens at the end
				} else {
					detectedLang = currentLang
					langCache[cleanPath] = currentLang
				}
			}

			// When we reach max depth, check if we need to report any depth 0 directories without languages
			if depth == maxDepth && projectRoot != "" {
				if _, needsReport := depth0NoLang[projectRoot]; needsReport {
					// Check if project root still doesn't have a language
					if cachedLang, ok := langCache[projectRoot]; !ok || cachedLang == "" {
						f := Finding{
							Path:        projectRoot,
							ProjectRoot: projectRoot, // Same as Path since this is the project root
							Language:    getLanguageForExclusion("", excludedDirs[projectRoot]),
							Pattern:     "",
							SizeBytes:   0,
							Items:       0,
							ModMax:      time.Time{},
						}
						findings = append(findings, f)
						// Remove from tracking since we've reported it
						delete(depth0NoLang, projectRoot)
					}
				}
			}

			// If language detected, use only patterns for that language
			if detectedLang != "" {
				if langPatterns, ok := langToPatterns[detectedLang]; ok {
					patternsToCheck = langPatterns
				}
			}
		}

		// Check if directory name matches any pattern BEFORE any fallback reporting
		// This ensures cache directories are detected even if they're at depth 0
		for _, pattern := range patternsToCheck {
			if matchPattern(dirName, pattern) {
				// An include-only target can be nested below a cache-named directory.
				// Keep walking through that ancestor instead of selecting it and
				// skipping the subtree before the included project is reached.
				if len(includePathPatterns) > 0 && !included {
					break
				}
				// An excluded descendant cannot be preserved if the parent cache
				// directory itself is deleted, so leave the whole cache untouched.
				if len(includePathPatterns) == 0 && (mayContainPatternMatch(cleanPath, excludePathPatterns) || mayContainPatternMatch(filterPath, excludePathPatterns) || mayContainPatternMatch(canonicalPath, excludePathPatterns)) {
					break
				}
				// Found a match - calculate size
				f, err := inspect(path)
				if err != nil {
					f.Err = err.Error()
				}
				f.Pattern = pattern
				// Use detected language when available; patternToLang can be wrong when
				// multiple languages share a pattern (e.g. node_modules in node, nextjs, vue)
				if detectLang && detectedLang != "" {
					f.Language = detectedLang
				} else {
					f.Language = patternToLang[pattern]
				}
				// Set project root to where language was detected, or parent if no language detection
				if detectLang && projectRoot != "" {
					f.ProjectRoot = projectRoot
				} else {
					// If no language detection, use parent directory as project root
					f.ProjectRoot = filepath.Dir(cleanPath)
				}
				if (filters.MinAge > 0 || filters.MaxAge > 0) && f.Err != "" && filters.Warn != nil {
					filters.Warn(fmt.Sprintf("cache inspection incomplete for %s: %s", f.Path, f.Err))
				}
				if !passesAgeFilter(f, filters) {
					// Don't traverse a cache directory that the age filter excludes.
					return filepath.SkipDir
				}
				findings = append(findings, f)
				// Track this path in the map for O(1) lookups
				findingsByPath[cleanPath] = true

				// Skip subdirectories of matched directories
				return filepath.SkipDir
			}
		}

		// Only report "no language found" at max depth if we didn't match a pattern
		// and this is a depth 0 directory (project root)
		if detectLang && depth == 0 && detectedLang == "" && !matchesCachePattern {
			// Check if this directory already has a finding (as a cache directory) using O(1) map lookup
			hasFinding := findingsByPath[cleanPath]
			// Only report if no cache directory finding exists
			if !hasFinding {
				// Check if we need to report this depth 0 directory
				if _, needsReport := depth0NoLang[projectRoot]; needsReport {
					f := Finding{
						Path:        projectRoot,
						ProjectRoot: projectRoot,
						Language:    "no language found",
						Pattern:     "",
						SizeBytes:   0,
						Items:       0,
						ModMax:      time.Time{},
					}
					findings = append(findings, f)
					delete(depth0NoLang, projectRoot)
				}
			}
		}

		return nil
	}); err != nil {
		// Log walk error but continue processing findings
		fmt.Fprintf(os.Stderr, "warning: directory walk error: %v\n", err)
	}

	// After scanning, report all depth 0 directories that were scanned but don't have cache directory findings
	// This ensures directories with detected languages are always reported, even if they don't have cache directories
	// Track which depth 0 directories have findings (cache directories found)
	depth0HasFindings := make(map[string]bool)
	for _, f := range findings {
		// Check if this finding is for a depth 0 directory or a subdirectory of one
		for depth0Root := range depth0Scanned {
			if f.Path == depth0Root || strings.HasPrefix(f.Path, depth0Root+string(filepath.Separator)) {
				depth0HasFindings[depth0Root] = true
				break
			}
		}
	}

	// Report all depth 0 directories that don't have cache directory findings
	// This includes directories with detected languages (even if no cache directories found)
	for projectRoot := range depth0Scanned {
		if !depth0HasFindings[projectRoot] {
			// Get the detected language for this directory
			// Always report the language if found, even if there are no cache directories
			detectedLang := langCache[projectRoot]
			f := Finding{
				Path:        projectRoot,
				ProjectRoot: projectRoot, // Same as Path since this is the project root
				Language:    getLanguageForExclusion(detectedLang, excludedDirs[projectRoot]),
				Pattern:     "",
				SizeBytes:   0,
				Items:       0,
				ModMax:      time.Time{},
			}
			findings = append(findings, f)
		}
	}

	if len(includePathPatterns) > 0 {
		filtered := findings[:0]
		for _, f := range findings {
			candidate := f.Path
			if !isCacheDirectory(f) && f.ProjectRoot != "" {
				candidate = f.ProjectRoot
			}
			aliasCandidate := pathWithScanAlias(candidate, root, filterRoot)
			canonicalCandidate := candidate
			if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
				canonicalCandidate = filepath.Clean(resolved)
			}
			if pathMatchesPatternOrAncestor(candidate, includePathPatterns) || pathMatchesPatternOrAncestor(aliasCandidate, includePathPatterns) || pathMatchesPatternOrAncestor(canonicalCandidate, includePathPatterns) {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	return findings
}

// matchPattern checks if a directory name matches a pattern
// Supports exact match and simple wildcard patterns
func matchPattern(name, pattern string) bool {
	// Exact match
	if name == pattern {
		return true
	}

	// Simple wildcard support (e.g., "cmake-build-*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}

	return false
}

func displayDetailed(findings []Finding, total int64) {
	// Group findings by project path, then by cache type
	type cacheTypeSummary struct {
		SizeBytes int64
		Items     int
		Language  string
		Patterns  []string // Track all patterns of this type
	}

	// First pass: identify which projects have actual cache directories
	projectsWithCache := make(map[string]bool)
	for _, f := range findings {
		if f.Err != "" {
			continue
		}
		if isCacheDirectory(f) {
			// Use ProjectRoot if available, otherwise fall back to Path
			projectPath := f.ProjectRoot
			if projectPath == "" {
				projectPath = f.Path
			}
			projectsWithCache[projectPath] = true
		}
	}

	// Map: ProjectRoot -> CacheType -> Summary
	projectGroups := make(map[string]map[string]*cacheTypeSummary)

	for _, f := range findings {
		if f.Err != "" {
			continue
		}
		// Use ProjectRoot if available, otherwise fall back to Path
		projectPath := f.ProjectRoot
		if projectPath == "" {
			projectPath = f.Path
		}

		// Skip "(no cache directories)" entries if this project has actual cache directories
		if f.Pattern == "" && projectsWithCache[projectPath] {
			continue
		}

		// Use Pattern as cache type key, or "no cache" if empty
		cacheType := f.Pattern
		if cacheType == "" {
			cacheType = "(no cache directories)"
		}

		if projectGroups[projectPath] == nil {
			projectGroups[projectPath] = make(map[string]*cacheTypeSummary)
		}

		if projectGroups[projectPath][cacheType] == nil {
			projectGroups[projectPath][cacheType] = &cacheTypeSummary{
				Language: f.Language,
				Patterns: []string{},
			}
		}

		projectGroups[projectPath][cacheType].SizeBytes += f.SizeBytes
		projectGroups[projectPath][cacheType].Items += f.Items
		if isCacheDirectory(f) && !contains(projectGroups[projectPath][cacheType].Patterns, f.Pattern) {
			projectGroups[projectPath][cacheType].Patterns = append(projectGroups[projectPath][cacheType].Patterns, f.Pattern)
		}
	}

	// Calculate totals for each project and prepare for sorting
	type projectEntry struct {
		path       string
		totalSize  int64
		totalItems int
		cacheTypes map[string]*cacheTypeSummary
		language   string
	}

	var projectEntries []projectEntry
	for path, cacheTypes := range projectGroups {
		var projectTotalSize int64
		var projectTotalItems int
		var detectedLanguage string

		// Calculate totals and find language
		for _, summary := range cacheTypes {
			projectTotalSize += summary.SizeBytes
			projectTotalItems += summary.Items
			if detectedLanguage == "" && summary.Language != "" {
				detectedLanguage = summary.Language
			}
		}

		projectEntries = append(projectEntries, projectEntry{
			path:       path,
			totalSize:  projectTotalSize,
			totalItems: projectTotalItems,
			cacheTypes: cacheTypes,
			language:   detectedLanguage,
		})
	}

	// Sort projects by total size (largest first)
	sort.Slice(projectEntries, func(i, j int) bool {
		return projectEntries[i].totalSize > projectEntries[j].totalSize
	})

	// Display grouped results - one line per project with all cache types listed
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("Project Path", "Cache Types", "Language", "Total Size", "Total Items")

	var totalItems int
	for _, project := range projectEntries {
		cacheTypes := project.cacheTypes

		// Sort cache types by size (largest first) within each project
		type cacheTypeEntry struct {
			name    string
			summary *cacheTypeSummary
		}
		var sortedCacheTypes []cacheTypeEntry
		for cacheType, summary := range cacheTypes {
			sortedCacheTypes = append(sortedCacheTypes, cacheTypeEntry{
				name:    cacheType,
				summary: summary,
			})
		}
		sort.Slice(sortedCacheTypes, func(i, j int) bool {
			return sortedCacheTypes[i].summary.SizeBytes > sortedCacheTypes[j].summary.SizeBytes
		})

		// Build cache types list string
		var cacheTypeList []string
		for _, entry := range sortedCacheTypes {
			cacheTypeList = append(cacheTypeList, entry.name)
		}

		// Join cache types with semicolons for readability
		cacheTypesStr := strings.Join(cacheTypeList, "; ")

		// Add single row for this project
		if err := table.Append(project.path, cacheTypesStr, project.language, human(project.totalSize), fmt.Sprintf("%d", project.totalItems)); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to append table row: %v\n", err)
		}

		totalItems += project.totalItems
	}

	table.Footer("TOTAL", "", "", human(total), fmt.Sprintf("%d", totalItems))
	if err := table.Render(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to render table: %v\n", err)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
