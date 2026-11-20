# dev-cache

[![CI](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml)
[![Release](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/everydaydevopsio/bilge-pump?filename=dev-cache%2Fgo.mod)](https://go.dev/)
[![License](https://img.shields.io/github/license/everydaydevopsio/bilge-pump)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/everydaydevopsio/bilge-pump)](https://github.com/everydaydevopsio/bilge-pump/releases)

A cross-platform Go CLI tool that scans source code directories to find and optionally delete local project cache directories (like `node_modules`, `build`, `dist`, `.venv`, etc.) across multiple programming languages.

## Features

- **Cross-platform**: Works on macOS, Linux, and Windows WSL
- **Multi-language support**: Detects cache directories for Node.js, Python, Go, Rust, Java, PHP, Ruby, .NET, C/C++, Flutter, and more
- **Configurable scanning**: Customize scan paths, depth, and language patterns
- **Dry-run by default**: Reports disk usage without deleting files
- **Safe deletion**: Requires explicit `--clean` flag with confirmation prompt
- **Table output**: Aggregated per-project overview with cache types, languages, and sizes
- **JSON output**: Machine-readable output for automation

## Installation

### From Source

```bash
git clone <repository>
cd dev-cache
make build
```

The binary will be in `build/dev-cache`.

### Cross-compilation

Build for multiple platforms:

```bash
make build-all  # Builds for Linux (amd64/arm64) and macOS (amd64/arm64)
```

Or build individually:

```bash
make build-linux   # Linux builds only
make build-darwin  # macOS builds only
```

## Quick Start

1. **Initialize config**:
```bash
./build/dev-cache --init
```

This creates a config file at `$XDG_CONFIG_HOME/dev-cache/config.yaml` when `XDG_CONFIG_HOME` is set, or `~/.config/dev-cache/config.yaml` otherwise (Windows uses the OS user config directory).

2. **Scan for cache directories** (dry-run):
```bash
./build/dev-cache
```

3. **Delete found cache directories**:
```bash
./build/dev-cache --clean
```

4. **Scan a different directory**:
```bash
./build/dev-cache --scan ~/projects
```

## Command-Line Arguments

| Flag | Description |
|------|-------------|
| `--init` | Create starter config file and exit |
| `--force` | Force overwrite existing config (use with --init) |
| `--config PATH` | Path to YAML config (default: XDG-aware dev-cache config path) |
| `--scan PATH` | Directory to scan (overrides config default) |
| `--depth N` | Max scan depth (overrides config default, 0 = use config) |
| `--languages LIST` | Comma-separated list of languages to scan (e.g., `node,python,go`) |
| `--exclude LIST` | Comma-separated paths/patterns to exclude |
| `--include-only LIST` | Comma-separated paths/patterns to include only |
| `--min-age DURATION` | Only include caches older than duration (e.g., `30d`, `48h`) |
| `--max-age DURATION` | Only include caches no older than duration (e.g., `7d`, `24h`) |
| `--unsafe-clean` | Allow cleanup of cache paths Git does not mark ignored (unsafe override) |
| `--clean` | Delete found cache directories |
| `--yes` | Skip confirmation prompt for cleanup |
| `--json` | Output results as JSON |

## Configuration

The config file defines which directories to scan and which patterns to look for:

```yaml
version: 1
options:
  defaultScanPath: ~/src
  maxDepth: 1  # How many levels deep to scan
  excludePaths: []
  includeOnly: []
  minAge: ""
  maxAge: ""

languages:
  - name: node
    enabled: true
    patterns:
      - node_modules
      - .npm
      - .yarn
      - .pnpm-store

  - name: python
    enabled: true
    patterns:
      - .venv
      - venv
      - __pycache__
      - .pytest_cache
      - .mypy_cache

  # ... more languages
```

### Default Languages

The starter config includes patterns for:

- **Node.js**: `node_modules`, `.npm`, `.yarn`, `.pnpm-store`
- **Python**: `.venv`, `venv`, `__pycache__`, `.pytest_cache`, `.mypy_cache`, `.tox`
- **Go**: `vendor`
- **Rust**: `target`
- **Java/Kotlin**: `target`, `.gradle`, `build`
- **Next.js**: `.next`, `dist`, `build`, `out`, `.cache`
- **Vue/Nuxt**: `.nuxt`, `dist`, `build`, `out`, `.cache`, `.parcel-cache`
- **PHP**: `vendor`
- **Ruby**: `vendor/bundle`
- **C#/.NET**: `bin`, `obj`
- **C/C++**: `build`, `cmake-build-*` (wildcard supported)
- **Flutter/Dart**: `.dart_tool`

### Pattern Matching

Patterns support:
- **Exact match**: `node_modules` matches only `node_modules`
- **Wildcard prefix**: `cmake-build-*` matches `cmake-build-debug`, `cmake-build-release`, etc.

### Path and Age Filters

Use `--exclude` or config `excludePaths` to skip matching project roots. Use `--include-only` or `includeOnly` to scan only matching project roots; when `includeOnly` is set, it takes precedence over `excludePaths`. Filters accept exact paths, descendant paths, and valid path globs. Malformed globs fail before scanning. Relative patterns resolve from the current working directory, whose characters are treated literally. On Windows, path and glob comparisons are case-insensitive. Filters apply to nested projects. CLI flags override their matching config values; explicitly passing an empty value (for example, `--include-only=`) clears that configured filter.

Age filters use the newest file modification time in each cache. `--min-age 30d` includes caches at least 30 days old; `--max-age 7d` includes caches no older than 7 days. Durations must be greater than zero; an empty value disables that filter. When both are set, a cache must satisfy both. If an active age filter cannot determine a cache's modification time or the cache inspection is incomplete, the cache is skipped. Excluded cache directories are not traversed for nested findings. These filters apply to both reporting and cleanup.

If an excluded path is inside a cache directory, the enclosing cache is also skipped so cleanup cannot delete the excluded path.

```bash
./build/dev-cache --scan ~/src --depth 2 --include-only '~/src/team/*' --min-age 30d
./build/dev-cache --scan ~/src --exclude ~/src/archive --max-age 7d --json
```

Include-only filters do not increase the configured scan depth. Set `--depth` high enough to reach nested project caches.

## Examples

### Scan specific languages only

```bash
./build/dev-cache --languages node,python
```

### Scan with custom depth

```bash
./build/dev-cache --depth 2
```

### Scan different directory

```bash
./build/dev-cache --scan ~/projects --depth 2
```

### JSON output for scripting

```bash
./build/dev-cache --json > scan-results.json
```

### Automatic cleanup (no prompt)

```bash
./build/dev-cache --clean --yes
```

## Platform Support

This tool works on:

- **macOS**: Native support (amd64 and arm64)
- **Linux**: Full support (amd64 and arm64)
- **Windows WSL**: Full support through Linux compatibility

All paths use `filepath.Join()` for cross-platform compatibility. Home directory expansion (`~`) works on all platforms via `os.UserHomeDir()`.

## Output Modes

### Aggregated Table Output

dev-cache always prints a single table grouped by project root. Each row aggregates all cache directories discovered for that project, including cache types, detected language, total size, and item counts.

```
+----------------------+---------------------+----------+-----------+--------+
| Project Path         | Cache Types         | Language | Total Size| Items  |
+----------------------+---------------------+----------+-----------+--------+
| ~/src/proj1          | node_modules, .venv | node     | 635.79 MB | 46912 |
| ~/src/proj2          | .venv               | python   | 123.45 MB |  1234 |
+----------------------+---------------------+----------+-----------+--------+
| TOTAL                |                     |          | 759.24 MB | 48146 |
```

### JSON Output

Use `--json` to get structured output suitable for automation:

```bash
./build/dev-cache --json > scan-results.json
```

## Safety Considerations

- **Dry-run by default**: The tool never deletes files unless `--clean` is explicitly provided
- **Confirmation prompt**: When using `--clean`, you must confirm the deletion (unless `--yes` is used)
- **Shows what will be deleted**: The tool displays all findings before asking for confirmation
- **Git-aware cleanup**: Cleanup skips caches containing tracked files, nested Git repositories, or paths not ignored by Git. `--unsafe-clean` is the explicit override.
- **Error handling**: Deletion errors are logged and reported, but don't stop the process

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Format, Lint, Vet

```bash
make fmt
make lint
make vet
```

Or run all checks:

```bash
make all
```

## License

See [LICENSE](../LICENSE) in the parent directory.
