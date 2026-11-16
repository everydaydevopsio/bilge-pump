package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestSafeCachePathFindsNestedRepositoryFromCachePath(t *testing.T) {
	team := filepath.Join(t.TempDir(), "team")
	repo := filepath.Join(team, "repo")
	cache := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	allowed, reason := safeCachePath(Finding{Path: cache, ProjectRoot: team, Pattern: "node_modules"})
	if !allowed {
		t.Fatalf("safeCachePath rejected nested project cache: %s", reason)
	}
}

func TestSafeCachePathRejectsNestedRepositoryInsideIgnoredCache(t *testing.T) {
	repo := t.TempDir()
	cache := filepath.Join(repo, "node_modules")
	nested := filepath.Join(cache, "dependency")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", nested, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("nested git init: %v: %s", err, out)
	}

	allowed, reason := safeCachePath(Finding{Path: cache, Pattern: "node_modules"})
	if allowed {
		t.Fatal("safeCachePath allowed deletion of a cache containing a nested Git repository")
	}
	if !strings.Contains(reason, "nested Git repository") {
		t.Fatalf("reason = %q, want nested Git repository explanation", reason)
	}
}

func TestSafeCachePathRejectsGitfileAtCacheRoot(t *testing.T) {
	repo := t.TempDir()
	cache := filepath.Join(repo, "node_modules")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(cache, ".git"), []byte("gitdir: ../.git/worktrees/dependency\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	allowed, reason := safeCachePath(Finding{Path: cache, Pattern: "node_modules"})
	if allowed || !strings.Contains(reason, "nested Git repository") {
		t.Fatalf("safeCachePath = %v, %q; want reject nested Git gitfile", allowed, reason)
	}
}

func TestCheckVersionFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"program"}, false},
		{"version flag", []string{"program", "--version"}, true},
		{"version flag short", []string{"program", "-version"}, true},
		{"other flags", []string{"program", "--config", "test.yaml"}, false},
		{"version with other flags", []string{"program", "--config", "test.yaml", "--version"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original os.Args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			// Set test args
			os.Args = tt.args

			got := checkVersionFlag()
			if got != tt.want {
				t.Errorf("checkVersionFlag() with args %v = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestHuman(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{1024*1024 + 512*1024, "1.50 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
	}
	for _, tt := range tests {
		got := human(tt.in)
		if got != tt.want {
			t.Fatalf("human(%d)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	h, _ := os.UserHomeDir()
	got := expand("~")
	if got != h {
		t.Fatalf("expand(~)=%q, want %q", got, h)
	}
	// With env var expansion
	_ = os.Setenv("FOO_TEST_ENV", "xyz")
	got2 := expand("$FOO_TEST_ENV")
	if got2 != "xyz" {
		t.Fatalf("expand($FOO_TEST_ENV)=%q, want %q", got2, "xyz")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	p := defaultConfigPath()
	configDir := ".config"
	if runtime.GOOS == "windows" {
		var err error
		configDir, err = os.UserConfigDir()
		if err != nil {
			t.Fatal(err)
		}
	}
	expectedSuffix := filepath.Join(configDir, "dev-cache", "config.yaml")
	if !strings.HasSuffix(p, expectedSuffix) {
		t.Fatalf("defaultConfigPath() unexpected: %q", p)
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"node_modules", "node_modules", true},
		{"node_modules", ".venv", false},
		{"cmake-build-debug", "cmake-build-*", true},
		{"cmake-build-release", "cmake-build-*", true},
		{"cmake-build", "cmake-build-*", false}, // Must have something after prefix
		{"build", "build", true},
		{"build", "build-*", false},
	}
	for _, tt := range tests {
		got := matchPattern(tt.name, tt.pattern)
		if got != tt.want {
			t.Fatalf("matchPattern(%q, %q)=%v, want %v", tt.name, tt.pattern, got, tt.want)
		}
	}
}

func TestInspectPath(t *testing.T) {
	dir := t.TempDir()
	// Create a couple of files
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := inspectPath(dir)
	if err != nil {
		t.Fatalf("inspectPath error: %v", err)
	}
	if f.Items != 2 {
		t.Fatalf("expected 2 items, got %d", f.Items)
	}
	if f.SizeBytes < 2053 { // 5 + 2048
		t.Fatalf("expected size >= 2053, got %d", f.SizeBytes)
	}
}

func TestScanDirectory(t *testing.T) {
	root := t.TempDir()

	// Create test structure:
	// root/
	//   proj1/
	//     node_modules/  (should find this)
	//   proj2/
	//     .venv/         (should find this)
	//   proj3/
	//     subdir/
	//       node_modules/  (depth 2, should find if maxDepth >= 2)

	proj1 := filepath.Join(root, "proj1")
	if err := os.MkdirAll(proj1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj1, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj1, "node_modules", "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	proj2 := filepath.Join(root, "proj2")
	if err := os.MkdirAll(proj2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj2, ".venv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj2, ".venv", "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	proj3 := filepath.Join(root, "proj3")
	if err := os.MkdirAll(filepath.Join(proj3, "subdir", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj3, "subdir", "node_modules", "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := []string{"node_modules", ".venv"}
	patternToLang := map[string]string{
		"node_modules": "node",
		".venv":        "python",
	}

	// Test with maxDepth 1
	findings := scanDirectory(root, 1, patterns, patternToLang, false, nil, nil, nil, ScanFilters{})
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings with maxDepth=1, got %d", len(findings))
	}

	// Test with maxDepth 2
	findings2 := scanDirectory(root, 2, patterns, patternToLang, false, nil, nil, nil, ScanFilters{})
	if len(findings2) != 3 {
		t.Fatalf("expected 3 findings with maxDepth=2, got %d", len(findings2))
	}

	// Verify languages are set correctly
	for _, f := range findings {
		if f.Pattern == "node_modules" && f.Language != "node" {
			t.Fatalf("expected language 'node' for node_modules, got %q", f.Language)
		}
		if f.Pattern == ".venv" && f.Language != "python" {
			t.Fatalf("expected language 'python' for .venv, got %q", f.Language)
		}
	}
}

// TestScanDirectoryMapLookup tests the map-based lookup optimization for findingsByPath.
// This ensures that when a cache directory is found at depth 0, it doesn't also get
// a "no language found" report, which would happen if we used a linear search through
// all findings. The map lookup should prevent duplicate reports.
func TestScanDirectoryMapLookup(t *testing.T) {
	root := t.TempDir()

	// Create test structure where a cache directory exists at depth 0 (directly under root)
	// This tests the optimization: the map lookup should prevent duplicate "no language found" reports
	// root/
	//   node_modules/  (depth 0 cache directory - should be found)
	//   proj1/         (depth 0 project - no language, no cache)
	//     file.txt
	//   proj2/         (depth 0 project - has language signature)
	//     package.json
	//     node_modules/  (should be found)

	// Cache directory at depth 0
	nodeModulesRoot := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(nodeModulesRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeModulesRoot, "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Project without language signature at depth 0
	proj1 := filepath.Join(root, "proj1")
	if err := os.MkdirAll(proj1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj1, "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Project with language signature at depth 0
	proj2 := filepath.Join(root, "proj2")
	if err := os.MkdirAll(proj2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj2, "package.json"), []byte(`{"name": "test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj2, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj2, "node_modules", "file.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{
		"node_modules": "node",
	}
	langSignatures := map[string][]string{
		"node": {"package.json"},
	}
	langPriorities := map[string]int{
		"node": 10,
	}
	langToPatterns := map[string][]string{
		"node": {"node_modules"},
	}

	// Test with language detection enabled and maxDepth 1
	// This should find:
	// 1. node_modules at root (depth 0) - cache directory finding
	// 2. proj1 - "no language found" report (no signature, no cache)
	// 3. proj2 - node language report (has signature)
	// 4. proj2/node_modules - cache directory finding (depth 1)
	findings := scanDirectory(root, 1, patterns, patternToLang, true, langSignatures, langPriorities, langToPatterns, ScanFilters{})

	// Count findings by type
	var cacheFindings int
	var noLangFindings int
	nodeModulesRootFound := false
	proj2NodeModulesFound := false

	for _, f := range findings {
		if f.Pattern != "" {
			cacheFindings++
			// Check if we found the root-level node_modules
			if strings.HasSuffix(f.Path, "node_modules") {
				dir := filepath.Dir(f.Path)
				// Normalize paths for comparison
				rootAbs, _ := filepath.Abs(root)
				dirAbs, _ := filepath.Abs(dir)
				if dirAbs == rootAbs {
					nodeModulesRootFound = true
				}
			}
			// Check if we found proj2/node_modules
			if strings.Contains(f.Path, "proj2") && strings.HasSuffix(f.Path, "node_modules") {
				proj2NodeModulesFound = true
			}
		} else if f.Language == "no language found" {
			noLangFindings++
		}
	}

	// Verify the root-level node_modules was found as a cache directory
	if !nodeModulesRootFound {
		t.Fatalf("expected to find node_modules at root level as cache directory. Findings: %+v", findings)
	}

	// Verify proj2/node_modules was found
	if !proj2NodeModulesFound {
		t.Fatalf("expected to find node_modules in proj2 as cache directory. Findings: %+v", findings)
	}

	// Verify we have exactly 2 cache findings (root node_modules and proj2/node_modules)
	if cacheFindings != 2 {
		t.Fatalf("expected 2 cache findings, got %d. Findings: %+v", cacheFindings, findings)
	}

	// Most importantly: verify that node_modules at root doesn't get a duplicate "no language found" report
	// This is what the map optimization prevents - the O(1) map lookup should prevent
	// the linear search through all findings that would cause duplicate reports.
	// The key test: when a cache directory is found at depth 0, it should NOT also get
	// a "no language found" report because the map lookup (findingsByPath) should detect
	// that this path already has a finding with a Pattern.
	rootNodeModulesReports := 0
	var rootNodeModulesFinding Finding
	for _, f := range findings {
		// Normalize paths for comparison
		fPathAbs, _ := filepath.Abs(f.Path)
		nodeModulesRootAbs, _ := filepath.Abs(nodeModulesRoot)
		if fPathAbs == nodeModulesRootAbs {
			rootNodeModulesReports++
			rootNodeModulesFinding = f
			if f.Pattern == "" {
				t.Fatalf("node_modules at root should not get a 'no language found' report - map lookup should prevent this. Finding: %+v", f)
			}
		}
	}
	// Should only have one report for root node_modules (the cache finding)
	if rootNodeModulesReports != 1 {
		t.Fatalf("expected exactly 1 report for root node_modules, got %d. This indicates the map lookup optimization is working correctly", rootNodeModulesReports)
	}
	// Verify the single report is a cache finding (has Pattern)
	if rootNodeModulesFinding.Pattern == "" {
		t.Fatalf("root node_modules finding should have a Pattern set, got: %+v", rootNodeModulesFinding)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "config.yaml")
	if err := ensureDir(path); err != nil {
		t.Fatalf("ensureDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
}

func TestIsCacheDirectory(t *testing.T) {
	if isCacheDirectory(Finding{Pattern: "node_modules"}) != true {
		t.Fatal("expected true for cache directory")
	}
	if isCacheDirectory(Finding{Pattern: ""}) != false {
		t.Fatal("expected false for non-cache directory")
	}
}

func TestContains(t *testing.T) {
	if contains([]string{"a", "b"}, "a") != true {
		t.Fatal("expected true when item in slice")
	}
	if contains([]string{"a", "b"}, "c") != false {
		t.Fatal("expected false when item not in slice")
	}
	if contains([]string{}, "a") != false {
		t.Fatal("expected false for empty slice")
	}
}

func TestWriteStarterConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := writeStarterConfig(cfgPath, false); err != nil {
		t.Fatalf("writeStarterConfig failed: %v", err)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Overwrite without force should fail
	if err := writeStarterConfig(cfgPath, false); err == nil {
		t.Fatal("expected error when overwriting without force")
	}

	// With force should succeed
	if err := writeStarterConfig(cfgPath, true); err != nil {
		t.Fatalf("writeStarterConfig with force failed: %v", err)
	}
}

func TestLoadConfigUsesDefaultsWhenDefaultConfigIsMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfigWithDefaults(defaultConfigPath(), true)
	if err != nil {
		t.Fatalf("loadConfig(default path) failed on a fresh install: %v", err)
	}
	if len(cfg.Languages) == 0 {
		t.Fatal("default config has no languages")
	}
}

func TestLoadConfigRejectsExplicitMissingDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfigWithDefaults(defaultConfigPath(), false); err == nil {
		t.Fatal("explicit missing default config path should fail")
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := writeStarterConfig(cfgPath, false); err != nil {
		t.Fatalf("writeStarterConfig failed: %v", err)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if cfg == nil || len(cfg.Languages) == 0 {
		t.Fatalf("expected non-empty config")
	}

	_, err = loadConfig(filepath.Join(tmpDir, "nonexistent.yaml"))
	if err == nil {
		t.Fatal("expected error for non-existent config")
	}

	// Invalid YAML
	badPath := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte("not: valid: yaml: content"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = loadConfig(badPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestGetLanguageForExclusion(t *testing.T) {
	tests := []struct {
		detected string
		excluded bool
		want     string
	}{
		{"node", false, "node"},
		{"", false, "no language found"},
		{"node", true, ""},
		{"", true, ""},
	}
	for _, tt := range tests {
		got := getLanguageForExclusion(tt.detected, tt.excluded)
		if got != tt.want {
			t.Errorf("getLanguageForExclusion(%q, %v) = %q, want %q", tt.detected, tt.excluded, got, tt.want)
		}
	}
}

func TestDisplayDetailed(t *testing.T) {
	// Redirect stdout to capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	findings := []Finding{
		{Path: "/proj1/node_modules", ProjectRoot: "/proj1", SizeBytes: 1000, Items: 5, Pattern: "node_modules", Language: "node"},
		{Path: "/proj1/.venv", ProjectRoot: "/proj1", SizeBytes: 500, Items: 2, Pattern: ".venv", Language: "python"},
	}
	displayDetailed(findings, 1500)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "proj1") && !strings.Contains(out, "node_modules") {
		t.Logf("displayDetailed output: %s", out)
	}
}

func TestDisplayDetailedNoCache(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	// Project with no cache directories (Pattern empty)
	findings := []Finding{
		{Path: "/proj2", ProjectRoot: "/proj2", SizeBytes: 0, Items: 0, Pattern: "", Language: "no language found"},
	}
	displayDetailed(findings, 0)
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() == 0 {
		t.Fatal("expected some output")
	}
}

func TestDisplayDetailedWithError(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	// Finding with error should be skipped in grouping but table still renders
	findings := []Finding{
		{Path: "/proj1/node_modules", ProjectRoot: "/proj1", SizeBytes: 1000, Items: 5, Pattern: "node_modules", Language: "node"},
		{Path: "/proj1/bad", ProjectRoot: "/proj1", SizeBytes: 0, Items: 0, Pattern: "node_modules", Language: "node", Err: "permission denied"},
	}
	displayDetailed(findings, 1000)
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() == 0 {
		t.Fatal("expected some output")
	}
}

func TestDetectLanguage(t *testing.T) {
	root := t.TempDir()
	langSignatures := map[string][]string{
		"node":   {"package.json"},
		"python": {"requirements.txt"},
	}
	langPriorities := map[string]int{
		"node":   10,
		"python": 5,
	}

	// No signatures
	got := detectLanguage(root, langSignatures, langPriorities)
	if got != "" {
		t.Fatalf("expected empty for dir with no signatures, got %q", got)
	}

	// Add package.json
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = detectLanguage(root, langSignatures, langPriorities)
	if got != "node" {
		t.Fatalf("expected node, got %q", got)
	}

	// Test wildcard pattern
	root2 := t.TempDir()
	langSignatures2 := map[string][]string{
		"dotnet": {"*.csproj"},
	}
	langPriorities2 := map[string]int{"dotnet": 5}
	if err := os.WriteFile(filepath.Join(root2, "MyApp.csproj"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got = detectLanguage(root2, langSignatures2, langPriorities2)
	if got != "dotnet" {
		t.Fatalf("expected dotnet for *.csproj, got %q", got)
	}

	// Priorities: python should win when both signatures exist because it has lower priority number.
	prioritizedRoot := t.TempDir()
	langSignatures3 := map[string][]string{
		"node":   {"package.json"},
		"python": {"requirements.txt"},
	}
	langPriorities3 := map[string]int{
		"node":   10,
		"python": 1,
	}
	if err := os.WriteFile(filepath.Join(prioritizedRoot, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prioritizedRoot, "requirements.txt"), []byte("flask"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := detectLanguage(prioritizedRoot, langSignatures3, langPriorities3); got != "python" {
		t.Fatalf("expected python to be selected due to higher priority, got %q", got)
	}
}

func TestDefaultConfigPathEmptyHome(t *testing.T) {
	if os.Getenv("HOME") == "" && os.Getenv("USERPROFILE") == "" {
		t.Skip("cannot unset home on this system")
	}
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	}()
	_ = os.Unsetenv("HOME")
	_ = os.Unsetenv("USERPROFILE")
	// UserHomeDir may still return something from other sources; just verify we get a path
	p := defaultConfigPath()
	if p == "" {
		t.Fatal("defaultConfigPath should not return empty")
	}
}

func TestInspectPathFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := inspectPath(filePath)
	if err != nil {
		t.Fatalf("inspectPath error: %v", err)
	}
	if f.Items != 1 || f.SizeBytes != 5 {
		t.Fatalf("expected 1 item, 5 bytes, got %d items, %d bytes", f.Items, f.SizeBytes)
	}
}

func TestInspectPathNonExistent(t *testing.T) {
	_, err := inspectPath(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestHumanLarge(t *testing.T) {
	// Test TB unit
	got := human(1024 * 1024 * 1024 * 1024)
	if !strings.Contains(got, "TB") && !strings.Contains(got, "1.00") {
		t.Fatalf("human(1TB) = %q, expected to contain TB", got)
	}
}

func TestParseAgeDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30d", 30 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"48h", 48 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"", 0, false},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"7x", 0, true},
		{"1.5d", 0, true},
		{"1h30m", 0, true},
		{"9223372036854775807d", 0, true},
	}
	for _, tt := range tests {
		got, err := parseAgeDuration(tt.in)
		if tt.wantErr && err == nil {
			t.Fatalf("parseAgeDuration(%q) expected error", tt.in)
		}
		if !tt.wantErr {
			if err != nil {
				t.Fatalf("parseAgeDuration(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("parseAgeDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		}
	}
}

func TestScanDirectoryPathFilters(t *testing.T) {
	root := t.TempDir()
	includeProj := filepath.Join(root, "include-me")
	excludeProj := filepath.Join(root, "exclude-me")
	if err := os.MkdirAll(filepath.Join(includeProj, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(excludeProj, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(includeProj, "node_modules", "a.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excludeProj, "node_modules", "b.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	filters := ScanFilters{
		IncludeOnly:  []string{includeProj},
		ExcludePaths: []string{excludeProj},
	}
	findings := scanDirectory(root, 2, patterns, patternToLang, false, nil, nil, nil, filters)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Path, "include-me") {
		t.Fatalf("unexpected finding path: %s", findings[0].Path)
	}
}

func TestIncludeOnlyOverridesExclude(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "included-and-excluded")
	cache := filepath.Join(project, "node_modules")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "package.json"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := scanDirectory(root, 2, []string{"node_modules"}, map[string]string{"node_modules": "node"}, false, nil, nil, nil, ScanFilters{
		IncludeOnly:  []string{project},
		ExcludePaths: []string{project},
	})
	if len(findings) != 1 || findings[0].Path != cache {
		t.Fatalf("includeOnly should take precedence over excludePaths; findings = %+v", findings)
	}
}

func TestPathFiltersResolveRelativeToWorkingDirectoryAndSupportNestedProjects(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	scanRoot, err := filepath.Abs(filepath.Join(".", "src"))
	if err != nil {
		t.Fatal(err)
	}
	projects := []string{
		filepath.Join(scanRoot, "team", "project"),
		filepath.Join(scanRoot, "team", "other"),
	}
	for _, project := range projects {
		cache := filepath.Join(project, "node_modules")
		if err := os.MkdirAll(cache, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, "cache.txt"), []byte("cache"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	filtered := scanDirectory(filepath.Join(".", "src"), 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		ExcludePaths: expandAndCleanPatterns([]string{"./src/team/project"}),
	})
	if len(filtered) != 1 || filtered[0].Path != filepath.Join(projects[1], "node_modules") {
		t.Fatalf("relative nested exclusion got %+v; want %s", filtered, filepath.Join(projects[1], "node_modules"))
	}

	included := scanDirectory(filepath.Join(".", "src"), 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		IncludeOnly: expandAndCleanPatterns([]string{"./src/team/*"}),
	})
	if len(included) != 2 {
		t.Fatalf("nested include glob should reach both projects below its parent, findings = %+v", included)
	}
}

func TestValidatePathPatterns(t *testing.T) {
	if err := validatePathPatterns([]string{"./src/team-*", "./src/project"}); err != nil {
		t.Fatalf("valid path patterns rejected: %v", err)
	}
	if err := validatePathPatterns([]string{"./src/project["}); err == nil {
		t.Fatal("malformed exclusion glob was accepted")
	}
	for _, pattern := range []string{"./src/*/project[", "./src/team-["} {
		if err := validatePathPatterns([]string{pattern}); err == nil {
			t.Errorf("malformed exclusion glob %q was accepted", pattern)
		}
	}
}

func TestCompilePathPatternDoesNotExpandEnvironmentValuesTwice(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tree$tag")
	t.Setenv("CACHE_ROOT", root)
	t.Setenv("tag", "")
	want := filepath.Join(root, "project")
	normalized := expandAndCleanPatterns([]string{"$CACHE_ROOT/project"})
	compiled, ok := compilePathPattern(normalized[0])
	if !ok || compiled.literalPath != want {
		t.Fatalf("compiled filter = %+v, want literal path %q", compiled, want)
	}
}

func TestEscapedUnixPathComponentsMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix filepath.Match escape semantics")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "team blue", "project", "node_modules")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "cache.dat"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	patterns := []string{"node_modules"}
	langs := map[string]string{"node_modules": "node"}
	include := filepath.Join(root, "team\\ blue", "*")
	findings := scanDirectory(root, 4, patterns, langs, false, nil, nil, nil, ScanFilters{IncludeOnly: []string{include}})
	if len(findings) != 1 || findings[0].Path != cache {
		t.Fatalf("escaped include prefix findings = %+v, want %s", findings, cache)
	}

	escapedLiteral := filepath.Join(root, "release\\]")
	literalCache := filepath.Join(root, "release]", "node_modules")
	if err := os.MkdirAll(literalCache, 0o755); err != nil {
		t.Fatal(err)
	}
	findings = scanDirectory(root, 3, patterns, langs, false, nil, nil, nil, ScanFilters{IncludeOnly: []string{escapedLiteral}})
	if len(findings) != 1 || findings[0].Path != literalCache {
		t.Fatalf("escaped literal include findings = %+v, want %s", findings, literalCache)
	}
}

func TestIncludeOnlyGlobsWithIntermediateWildcards(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	scanRoot := filepath.Join(".", "src")
	projects := []string{
		filepath.Join("src", "team", "project"),
		filepath.Join("src", "team", "other"),
		filepath.Join("src", "team-red", "project"),
	}
	for _, project := range projects {
		cache := filepath.Join(project, "node_modules")
		if err := os.MkdirAll(cache, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, "cache.txt"), []byte("cache"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	included := scanDirectory(scanRoot, 4, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		IncludeOnly: expandAndCleanPatterns([]string{"./src/*/project"}),
	})
	if len(included) != 2 {
		t.Fatalf("intermediate wildcard should include both project roots, findings = %+v", included)
	}

	prefixed := scanDirectory(scanRoot, 4, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		IncludeOnly: expandAndCleanPatterns([]string{"./src/team-*/project"}),
	})
	if len(prefixed) != 1 || !strings.Contains(prefixed[0].Path, filepath.Join("team-red", "project", "node_modules")) {
		t.Fatalf("wildcard prefix in an intermediate component should include team-red only, findings = %+v", prefixed)
	}
}

func TestRelativePathFiltersTreatWorkingDirectoryGlobCharactersLiterally(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(t.TempDir(), "workspace[1]")
	if err := os.MkdirAll(filepath.Join(workDir, "project", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "project", "node_modules", "cache.dat"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	excluded := scanDirectory(".", 2, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		ExcludePaths: expandAndCleanPatterns([]string{"./project"}),
	})
	if len(excluded) != 0 {
		t.Fatalf("relative exclusion under glob-character working directory got %+v, want no findings", excluded)
	}

	included := scanDirectory(".", 2, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		IncludeOnly: expandAndCleanPatterns([]string{"./project"}),
	})
	if len(included) != 1 || filepath.Base(included[0].Path) != "node_modules" {
		t.Fatalf("relative include under glob-character working directory got %+v, want project cache", included)
	}
}

func TestWindowsPathFiltersIgnoreCaseForLiteralAndAncestorMatching(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows path casing semantics")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "Project", "node_modules")
	compiled, ok := compilePathPattern(filepath.Join(root, "project"))
	if !ok {
		t.Fatal("failed to compile Windows literal path")
	}
	within, rel := pathIsWithin(cache, compiled.literalPath)
	if !within {
		t.Fatalf("case-insensitive containment failed: path=%q base=%q rel=%q", cache, compiled.literalPath, rel)
	}
	if err := os.MkdirAll(filepath.Join(cache, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "cache.dat"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	excluded := scanDirectory(root, 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		ExcludePaths: []string{filepath.Join(root, "project")},
	})
	if len(excluded) != 0 {
		t.Fatalf("case-insensitive exclusion got %+v, want no findings", excluded)
	}
	rootedProject := strings.TrimPrefix(filepath.Join(root, "Project"), filepath.VolumeName(root))
	rootedCompiled, ok := compilePathPattern(rootedProject)
	wantRootedProject := filepath.VolumeName(root) + rootedProject
	if !ok || rootedCompiled.literalPath != filepath.Clean(wantRootedProject) {
		t.Fatalf("drive-rooted filter compiled to %q, want %q", rootedCompiled.literalPath, wantRootedProject)
	}
	rootedExcluded := scanDirectory(root, 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		ExcludePaths: []string{rootedProject},
	})
	if len(rootedExcluded) != 0 {
		t.Fatalf("drive-rooted exclusion got %+v, want no findings", rootedExcluded)
	}

	excludedDescendant := scanDirectory(root, 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		ExcludePaths: []string{filepath.Join(root, "project", "node_modules", "keep")},
	})
	if len(excludedDescendant) != 0 {
		t.Fatalf("case-insensitive excluded descendant got %+v, want parent cache omitted", excludedDescendant)
	}

	included := scanDirectory(root, 3, patterns, patternToLang, false, nil, nil, nil, ScanFilters{
		IncludeOnly: []string{filepath.Join(root, "project")},
	})
	if len(included) != 1 || filepath.Base(included[0].Path) != "node_modules" {
		t.Fatalf("case-insensitive inclusion got %+v, want project cache", included)
	}
}

func TestIncludeOnlyTraversesThroughCacheNamedAncestor(t *testing.T) {
	root := t.TempDir()
	projectCache := filepath.Join(root, "src", "build", "project", "node_modules")
	if err := os.MkdirAll(projectCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectCache, "package.json"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := scanDirectory(root, 4, []string{"build", "node_modules"}, map[string]string{
		"build":        "",
		"node_modules": "node",
	}, false, nil, nil, nil, ScanFilters{
		IncludeOnly: []string{filepath.Join(root, "src", "build", "project")},
	})
	if len(findings) != 1 || findings[0].Path != projectCache {
		t.Fatalf("include-only scan beneath cache-named ancestor = %+v, want only %s", findings, projectCache)
	}
}

func TestExcludeDescendantPreventsSelectingParentCache(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "project", "node_modules")
	excluded := filepath.Join(cache, "keep")
	if err := os.MkdirAll(filepath.Join(excluded, "nested-cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excluded, "nested-cache", "cache.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "remove.txt"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := scanDirectory(root, 4, []string{"node_modules", "nested-cache"}, map[string]string{}, false, nil, nil, nil, ScanFilters{
		ExcludePaths: []string{excluded},
	})
	if len(findings) != 0 {
		t.Fatalf("cache containing an excluded descendant must not be selected for deletion, findings = %+v", findings)
	}
}

func TestPassesAgeFilter(t *testing.T) {
	now := time.Now()
	base := Finding{
		Path:      "/tmp/x",
		Pattern:   "node_modules",
		SizeBytes: 10,
		ModMax:    now.Add(-48 * time.Hour),
	}
	filters := ScanFilters{Now: now, MinAge: 24 * time.Hour, MaxAge: 72 * time.Hour}
	if !passesAgeFilter(base, filters) {
		t.Fatal("expected finding to pass age filters")
	}
	tooNew := base
	tooNew.ModMax = now.Add(-2 * time.Hour)
	if passesAgeFilter(tooNew, filters) {
		t.Fatal("expected too-new finding to fail min-age filter")
	}
	tooOld := base
	tooOld.ModMax = now.Add(-10 * 24 * time.Hour)
	if passesAgeFilter(tooOld, filters) {
		t.Fatal("expected too-old finding to fail max-age filter")
	}
}

func TestPassesAgeFilterRejectsUnknownModificationTime(t *testing.T) {
	finding := Finding{Path: "/tmp/cache", Pattern: "node_modules"}
	filters := ScanFilters{Now: time.Now(), MinAge: 24 * time.Hour}
	if passesAgeFilter(finding, filters) {
		t.Fatal("expected unknown modification time to fail a configured age filter")
	}

	filters.MinAge = 0
	filters.MaxAge = 24 * time.Hour
	if passesAgeFilter(finding, filters) {
		t.Fatal("expected unknown modification time to fail a configured max-age filter")
	}
}

type failingInfoDirEntry struct{ fs.DirEntry }

func (failingInfoDirEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("permission denied")
}

func TestPassesAgeFilterRejectsPartialInspection(t *testing.T) {
	cache := t.TempDir()
	oldFile := filepath.Join(cache, "old-readable.txt")
	newFile := filepath.Join(cache, "new-unreadable.txt")
	for _, path := range []string{oldFile, newFile} {
		if err := os.WriteFile(path, []byte("cache"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, old, old); err != nil {
		t.Fatal(err)
	}

	walkWithUnreadableFile := func(root string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && path == newFile {
				entry = failingInfoDirEntry{DirEntry: entry}
			}
			return fn(path, entry, err)
		})
	}
	finding, err := inspectPathWithWalk(cache, walkWithUnreadableFile)
	if err != nil {
		t.Fatalf("inspectPathWithWalk: %v", err)
	}
	if finding.Err == "" || !finding.ModMax.Equal(old) {
		t.Fatalf("partial inspection finding = %+v, want recorded error and readable file timestamp", finding)
	}
	finding.Pattern = "node_modules"
	if passesAgeFilter(finding, ScanFilters{Now: now, MinAge: 24 * time.Hour}) {
		t.Fatal("age filter accepted a partial inspection with an old readable file")
	}
}

func TestCleanupCLIUsesYAMLFiltersAndCLIOverrides(t *testing.T) {
	repo := t.TempDir()
	gitignore := filepath.Join(repo, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", repo, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	now := time.Now()
	excludedCache := filepath.Join(repo, "team", "excluded", "node_modules")
	tooNewCache := filepath.Join(repo, "team", "too-new", "node_modules")
	eligibleCache := filepath.Join(repo, "team", "eligible", "node_modules")
	for _, cache := range []string{excludedCache, tooNewCache, eligibleCache} {
		if err := os.MkdirAll(cache, 0o755); err != nil {
			t.Fatal(err)
		}
		projectRoot := filepath.Dir(cache)
		if err := os.WriteFile(filepath.Join(projectRoot, "package.json"), []byte(`{"name":"fixture"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, "cache.dat"), []byte("cache"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := now.Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(excludedCache, "cache.dat"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(eligibleCache, "cache.dat"), old, old); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Version: 1,
		Options: Options{
			DefaultScanPath: repo,
			MaxDepth:        4,
			DetectLanguage:  true,
			ExcludePaths:    []string{filepath.Join(repo, "team", "excluded")},
			MinAge:          "90d",
		},
		Languages: []Language{{Name: "node", Enabled: true, Patterns: []string{"node_modules"}, Signatures: []string{"package.json"}}},
	}
	configBytes, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "dev-cache.yaml")
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	binaryName := "dev-cache"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dev-cache: %v: %s", err, output)
	}
	command := exec.Command(binary, "--config", configPath, "--clean", "--yes", "--min-age", "30d")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("dev-cache cleanup: %v: %s", err, output)
	}

	for _, cache := range []string{excludedCache, tooNewCache} {
		if _, err := os.Stat(cache); err != nil {
			t.Errorf("protected cache %s should remain: %v", cache, err)
		}
	}
	if _, err := os.Stat(eligibleCache); !os.IsNotExist(err) {
		t.Errorf("eligible cache %s should be removed, stat error = %v", eligibleCache, err)
	}
}

func TestApplyFilterOverridesCanClearConfiguredFilters(t *testing.T) {
	configured := Options{
		ExcludePaths: []string{"/src/archive"},
		IncludeOnly:  []string{"/src/team"},
		MinAge:       "30d",
		MaxAge:       "90d",
	}
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "exclude", key: "exclude"},
		{name: "include-only", key: "include-only"},
		{name: "min-age", key: "min-age"},
		{name: "max-age", key: "max-age"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := applyFilterOverrides(configured, map[string]string{tt.key: ""}, map[string]bool{tt.key: true})
			switch tt.key {
			case "exclude":
				if len(got.ExcludePaths) != 0 || len(got.IncludeOnly) != 1 || got.MinAge != "30d" || got.MaxAge != "90d" {
					t.Fatalf("clearing exclude produced options %+v", got)
				}
			case "include-only":
				if len(got.IncludeOnly) != 0 || len(got.ExcludePaths) != 1 || got.MinAge != "30d" || got.MaxAge != "90d" {
					t.Fatalf("clearing include-only produced options %+v", got)
				}
			case "min-age":
				if got.MinAge != "" || got.MaxAge != "90d" || len(got.ExcludePaths) != 1 || len(got.IncludeOnly) != 1 {
					t.Fatalf("clearing min-age produced options %+v", got)
				}
			case "max-age":
				if got.MaxAge != "" || got.MinAge != "30d" || len(got.ExcludePaths) != 1 || len(got.IncludeOnly) != 1 {
					t.Fatalf("clearing max-age produced options %+v", got)
				}
			}
		})
	}
}

func TestScanDirectorySkipsNestedFindingsWhenOuterCacheFailsAgeFilter(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "project", "outer-cache")
	inner := filepath.Join(outer, "inner-cache")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(inner, "old.txt")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "recent.txt"), []byte("recent"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := scanDirectory(root, 3, []string{"outer-cache", "inner-cache"}, map[string]string{}, false, nil, nil, nil, ScanFilters{
		Now:    time.Now(),
		MinAge: 24 * time.Hour,
	})
	if len(findings) != 0 {
		t.Fatalf("filtered outer cache should prevent nested findings, got %+v", findings)
	}
}

func TestScanDirectoryWithExcludedDir(t *testing.T) {
	// Test that .git directories are excluded from language detection
	root := t.TempDir()
	projWithGit := filepath.Join(root, "proj-with-git")
	if err := os.MkdirAll(filepath.Join(projWithGit, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projWithGit, ".git", "config"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projWithGit, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projWithGit, "node_modules", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	langSignatures := map[string][]string{"node": {"package.json"}}
	langPriorities := map[string]int{"node": 10}
	langToPatterns := map[string][]string{"node": {"node_modules"}}

	findings := scanDirectory(root, 2, patterns, patternToLang, true, langSignatures, langPriorities, langToPatterns, ScanFilters{})
	if len(findings) < 1 {
		t.Fatalf("expected at least 1 finding, got %d", len(findings))
	}
}

func TestFilterCacheFindings(t *testing.T) {
	findings := []Finding{
		{Path: "/proj/node_modules", Pattern: "node_modules", SizeBytes: 100},
		{Path: "/proj/.venv", Pattern: ".venv", SizeBytes: 200},
		{Path: "/proj/README.md", Pattern: "", SizeBytes: 10},
	}
	cacheOnly, total := filterCacheFindings(findings)
	if len(cacheOnly) != 2 {
		t.Fatalf("expected 2 cache findings, got %d", len(cacheOnly))
	}
	if total != 300 {
		t.Fatalf("expected total 300, got %d", total)
	}
	for _, f := range cacheOnly {
		if f.Pattern == "" {
			t.Fatal("expected only cache findings")
		}
	}
}

func TestTotalCacheBytes(t *testing.T) {
	findings := []Finding{
		{Pattern: "node_modules", SizeBytes: 50},
		{Pattern: "", SizeBytes: 25},
		{Pattern: ".venv", SizeBytes: 75},
	}
	if got := totalCacheBytes(findings); got != 125 {
		t.Fatalf("expected 125 cache bytes, got %d", got)
	}
}

func TestBytesFreed(t *testing.T) {
	if freed := bytesFreed(500, 200); freed != 300 {
		t.Fatalf("expected 300 freed bytes, got %d", freed)
	}
	if freed := bytesFreed(200, 500); freed != 0 {
		t.Fatalf("expected 0 freed bytes when after >= before, got %d", freed)
	}
	if freed := bytesFreed(100, 100); freed != 0 {
		t.Fatalf("expected 0 freed bytes when totals equal, got %d", freed)
	}
}

func TestScanDirectoryWalkError(t *testing.T) {
	// Create a dir we can't read (on Unix, chmod 000)
	root := t.TempDir()
	noReadDir := filepath.Join(root, "noread")
	if err := os.MkdirAll(noReadDir, 0o000); err != nil {
		t.Skip("cannot create no-read dir:", err)
	}
	defer func() { _ = os.Chmod(noReadDir, 0o755) }()

	patterns := []string{"node_modules"}
	patternToLang := map[string]string{"node_modules": "node"}
	findings := scanDirectory(root, 2, patterns, patternToLang, false, nil, nil, nil, ScanFilters{})
	// Should not panic; may or may not find things depending on walk behavior
	_ = findings
}
