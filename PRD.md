# Product Requirements Document (PRD): bilge

## Overview
Bilge is a cross-platform Go toolkit for reclaiming disk space by identifying and cleaning developer caches and repository metadata. The unified command and Homebrew cask are both named `bilge`; the macOS, Linux, and Windows platform tasks are `bilge-macos`, `bilge-linux`, and `bilge-windows`.

## Project Goals
- Safety first (no direct deletions where possible, use official tool commands)
- Transparent dry-run by default
- Cross-platform support (macOS, Linux, Windows WSL)
- Extensible via YAML configuration
- Transparent size reporting
- Selective filtering for targeted operations
- JSON output for automation/CI integration
- A single `bilge` executable that runs the full platform-appropriate cleanup or selected cleanup tasks
- A local MCP interface for the unified scan and cleanup capabilities, usable by Codex and Claude Code
- The public command, release archive prefix, installer selection, and Homebrew cask use the name `bilge` beginning with v2.1.0.
- Configuration stored under the XDG Base Directory Specification on Unix-like systems, honoring `XDG_CONFIG_HOME`

## Unified `bilge` CLI

### Overview
Ship one unified `bilge` command for macOS, Linux, and Windows. The release bundles its task executables in the same install directory; users invoke `bilge` for `scan`, `clean`, and named-tool commands. The command locates bundled tools beside itself, and does not require users to install Go or put each tool on `PATH`.

### Configuration
- The unified config is stored at `$XDG_CONFIG_HOME/bilge-pump/config.yaml` when `XDG_CONFIG_HOME` is set; otherwise use the platform user config directory (`~/.config/bilge-pump/config.yaml` on Unix and the OS config directory on Windows).
- `--config` overrides the default path.
- Existing tool configs remain readable at their XDG-aware per-tool paths, and `git-cleaner` gains config support for its default scan path.
- A first run works with built-in sensible defaults; `--init` writes a customizable config.
- A local `make bilge` build places the launcher and all host-platform task executables together under `build/`.
- Cross-build cleaner targets write into OS/architecture-specific subdirectories and do not overwrite the native bundle under `build/`.
- Selecting `bilge` in `install.sh` installs the launcher and all task executables it needs; selecting a standalone cleaner still installs only that tool.
- Legacy component executables use their built-in starter config when their default XDG/platform config is absent; an explicitly supplied missing `--config` path remains an error.
- Explicit missing component configs remain errors even when the supplied path happens to equal that tool's default config path.
- The unified CLI consumes orchestration flags itself, routes `--yes` only to project cache cleanup, and rejects task-specific arguments that are invalid for any selected task before starting a clean operation.
- Boolean orchestration flags honor explicit true/false values; invalid values fail before any child process starts.
- Missing explicitly named config files are errors in every tool; built-in defaults apply only when the default config path was implicit.
- The unified command accepts only known bundled task names and never launches arbitrary executables from configuration.
- Windows release archives are ZIP files matching `install.ps1`; macOS and Linux retain tar.gz archives.

### CLI and behavior
- `bilge scan` reports findings across tasks without changing files.
- `bilge clean --all` runs platform-appropriate cleanup tasks; `bilge clean <task>` or `bilge <task>` selects one task.
- Individual legacy tools remain available for compatibility.
- One shared JSON report schema and exit-code convention apply to unified orchestration.
- `bilge scan --json` emits a valid JSON envelope with each selected task's structured report; task progress stays on stderr.
- Platform builds cover macOS, Linux, and Windows (amd64 and arm64 where supported).
- Windows batch-based cleanup commands preserve executable paths and argument boundaries containing spaces and shell metacharacters; values containing percent signs are rejected because `cmd.exe` expands them before batch execution.
- Cleanup remains dry-run by default; destructive tasks require explicit `clean` intent and task-level safety controls.
- Project cache deletion skips paths with tracked files or paths that Git does not mark ignored unless the user supplies the explicit `--unsafe-clean` override.
- Git safety checks discover the repository from each cache directory, including projects nested under non-repository grouping directories.
- Platform cleaner scans list each selected target and its planned cleanup commands, including targets such as Docker that have no filesystem paths.
- Platform cleaner rejects unknown, empty, and mixed-invalid `--targets` selections before scanning or running cleanup commands.
- Platform cleaner scans report filesystem inspection failures and return a partial-failure exit code; nonexistent optional cache paths are omitted without error.
- macOS Homebrew post-install handling removes quarantine from the unified launcher as well as its bundled task executables.
- Docker cleanup preserves named volumes by default; Android SDK updates and commands that install software or start services are excluded from cleanup defaults.

### MCP and agent setup
- `bilge mcp` serves the existing task capabilities over local stdio, without a network listener or separate release binary. It exposes dry-run scanning, explicit cleanup, platform target discovery, and macOS dependency checks.
- MCP cleanup requires an explicit confirmation argument and retains the CLI's project-cache Git safety rules. The server never treats a scan as cleanup authorization. Its child processes have bounded execution time and output.
- `bilge --init` installs a `bilge` MCP registration in available Codex and Claude Code CLIs and installs a disk-savings skill for both. Re-running init preserves an existing Bilge config, existing MCP registrations, and edited skill files so upgrades can complete agent setup without overwriting user work.
- The disk-savings skill uses the MCP scan tools for low-disk investigations, distinguishes estimated savings from command-only targets, requests authorization before cleanup, and measures again afterward.

---

## bilge-macos

### Overview
A safe, config-driven Go CLI for macOS that frees disk space using **official tool commands** (Docker, npm/yarn/pnpm, Go, Maven, Brew, and many more) instead of deleting files directly.

### Goals
- Safety first (no direct deletions)
- Transparent dry-run by default
- Extensible via YAML
- Transparent size reporting
- Target filtering for selective operations
- Intelligent tool requirement checking
- Installation guidance for missing tools

### Key Features

#### CLI Flags
- `--init` writes starter config to `~/.config/bilge-macos/config.yaml`
- `--force` forces overwrite of existing config when used with `--init`
- `--config` specifies path to YAML config (defaults to `~/.config/bilge-macos/config.yaml`)
- `--clean` executes cleanup commands (default: dry-run mode)
- `--targets` filters targets to scan/clean (comma-separated or "all")
- `--details` shows detailed per-directory information
- `--json` outputs structured JSON for automation/CI
- `--list-targets` lists all available targets in the config
- `--docker-prune` injects Docker prune commands at runtime
- `--check-tools` checks if required tools are installed and exits (can be combined with `--targets`)

#### Configuration Structure
```yaml
version: 1
options:
  dockerPruneByDefault: false
targets:
  - name: docker
    enabled: true
    notes: "Report Docker.raw size; optionally run prunes."
    paths: ["~/Library/Caches/docker/*", ...]  # measured for size
    cmds: [["docker", "builder", "prune", "-af"], ...]  # executed when --clean
    tools:  # required tools for this target
      - name: docker
        version: "24.0"  # optional: minimum version
        installCmd: "brew install --cask docker"  # installation command
        installNotes: "Optional installation notes"
        checkPath: "~/.docker/config"  # optional: check specific file path instead of PATH
```

#### Supported Targets
- **docker** - Docker caches and images
- **brew** - Homebrew packages and caches
- **npm** - npm cache
- **yarn** - Global Yarn cache
- **pnpm** - pnpm store and cache
- **node-versions** - Node version managers (nvm, volta)
- **expo** - Expo and React Native caches
- **go** - Go build & module caches
- **rust** - Rust registry and build caches
- **python** - pip, pipenv, and poetry caches
- **conda** - Conda package and cache cleanup
- **maven** - Maven local repository
- **gradle** - Gradle build caches and wrappers
- **xcode** - Xcode build artifacts and caches
- **ruby** - Ruby and Bundler caches
- **php** - Composer PHP cache
- **dotnet** - .NET SDK and NuGet caches
- **vscode** - VS Code caches and logs
- **jetbrains** - JetBrains IDE caches (IntelliJ, PyCharm, WebStorm, etc.)
- **build-tools** - Compiler and build caches (ccache, bazel, Xcode)
- **chrome** - Chrome caches (informational only, no CLI)
- **macos** - macOS system caches (advanced users only, disabled by default)
- **flutter** - Flutter and Dart caches
- **android** - Android SDK and emulator caches
- **android-studio** - Android Studio IDE caches
- **terraform** - Terraform plugin cache
- **packer** - Packer plugins directory
- **ollama** - Ollama models and cache (uses official prune)
- **home-cache** - Top-level ~/.cache subdirectories (informational only)
- **pyenv** - Pyenv installed versions and downloads (informational)
- **rustup** - Rustup toolchains and targets (informational)
- **vscode-extensions** - VS Code extensions and data under ~/.vscode (informational)
- **rvm** - RVM installed rubies and archives (informational/has cleanup)
- **dropbox** - Dropbox metadata and state (informational only)
- **cursor** - Cursor editor state and cache (informational)

#### Path Expansion
- Supports `~` for home directory
- Expands environment variables (`$VAR`)
- Supports `$(brew --cache)` expansion
- Glob patterns for flexible matching
- Command substitutions are whitelist-only for safety; currently only `brew --cache` is supported. Other substitutions (e.g., `$(docker ...)`) are rejected.

#### Reporting Features
- Size measurement in bytes with human-readable output
- Item count tracking
- Latest modification time per path
- Command availability detection
- JSON output for programmatic processing
- Warnings for glob errors
- Before/after comparison when `--clean` is used
- Total space freed calculation
- Docker usage is measured via `docker system df` (JSON/template parsing), not by reading `Docker.raw` directly

### Behavior
1. **Dry-run by default** - Reports sizes without making changes
2. **Tool requirement checking** - Checks if required tools are installed before running commands
3. **Size scanning** - Measures all configured paths before any cleanup
4. **Command execution** - Runs official tool CLI commands when `--clean` is set
5. **Target filtering** - Process specific targets with `--targets` flag
6. **Docker prune injection** - Adds prune commands when configured/flagged
7. **Auto-confirm prompts** - Automatically sends 'y' to interactive prompts when running commands
8. **Re-scan after cleanup** - Shows before/after sizes and freed space when `--clean` is used

#### Tool Management
- Before executing commands, the app checks if required tools (specified in `tools` array) are installed
- If a tool is missing, a warning is shown with installation guidance
- Installation commands default to `brew install` if Homebrew is available
- Version checking: When a version is specified, the app performs a basic `--version` contains check to verify presence; it does not perform full semantic version comparison
- Commands are marked as "not found" when their prerequisite tools are missing
- Custom path checking: If `checkPath` is specified, the app checks for the existence of that file instead of using PATH lookup
- Tool status is displayed with ✓ for installed tools and ✗ for missing tools

#### Check Tools Flag
The `--check-tools` flag provides a quick way to verify tool requirements:
- Shows ✓ for installed tools with version info (if available)
- Shows ✗ for missing tools with installation commands
- Lists which targets require each tool
- Respects `--targets` flag to check tools for specific targets only
- Exits with status 0 (all OK) or 1 (some missing) - useful for CI/CD scripts

#### List Targets Flag
The `--list-targets` flag shows all available targets:
- Lists all targets from the config file
- Shows [ENABLED] or [DISABLED] status for each target
- Displays notes for each target
- Useful for discovering available cleanup options

#### JSON Mode Behavior
- When `--json` is provided, the app outputs the initial scan results (including totals and warnings) and exits. Cleanup is not performed even if `--clean` is also set.

### Out of Scope
- Windows/Linux paths (future)
- Direct file deletion (unsafe)
- Chrome deletion automation (no stable CLI)
- Interactive prompts (auto-confirmed with 'y')

### Future Enhancements
- TUI interface
- Homebrew cask
- Scheduled runs
- CI/CD integration examples
- Windows/Linux support

---

## dev-cache

### Overview
A cross-platform Go CLI tool that scans source code directories to find and optionally delete local project cache directories (like `node_modules`, `build`, `dist`, `.venv`, etc.) across multiple programming languages.

### Goals
- Cross-platform support (macOS, Linux, Windows WSL)
- Multi-language cache detection
- Configurable scanning depth and patterns
- Safe deletion with confirmation prompts
- Detailed reporting by language and project

### Key Features

#### CLI Flags
- `--init` creates starter config file and exits
- `--force` forces overwrite of existing config (use with --init)
- `--config PATH` specifies path to YAML config (default: `~/.config/dev-cache/config.yaml`)
- `--scan PATH` directory to scan (overrides config default)
- `--depth N` max scan depth (overrides config default, 0 = use config)
- `--languages LIST` comma-separated list of languages to scan (e.g., `node,python,go`)
- `--unsafe-clean` allow cleanup of cache paths that Git does not mark ignored (unsafe override)
- `--exclude PATHS` comma-separated project paths to exclude from scanning and cleanup
- `--include-only PATHS` comma-separated project paths to scan; when set, this whitelist takes precedence over `excludePaths`
- `--min-age DURATION` only include caches at least this old (for example, `30d`)
- `--max-age DURATION` only include caches no older than this (for example, `7d`)
- Unified `bilge dev-cache` and `bilge clean dev-cache` commands forward `--exclude`, `--include-only`, `--min-age`, and `--max-age` to the selected task.
- `--clean` deletes found cache directories
- `--yes` skips confirmation prompt for cleanup
- `--json` outputs results as JSON
- `--details` shows detailed per-project breakdown

#### Configuration Structure
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
```

#### Supported Languages
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

#### Pattern Matching
- **Exact match**: `node_modules` matches only `node_modules`
- **Wildcard prefix**: `cmake-build-*` matches `cmake-build-debug`, `cmake-build-release`, etc.
- **Language labels**: with language detection on, a cache takes the language of the nearest enclosing directory with a signature file, up to and including the scan root (so `~/src/<org>/<repo>/node_modules` is labelled from `<repo>`, and a Python service inside a Node monorepo is Python), and only that language's patterns are matched there; the finding's project root is that directory. With no signature the label is `unknown`, never a guess from the pattern name. With detection off, a pattern owned by one language keeps that language and a shared pattern (`node_modules`, `build`, `target`) is `unknown`.

#### Path and Age Filters
- `excludePaths` and `includeOnly` accept exact paths, descendant paths, and valid path globs; relative patterns resolve from the process working directory, whose characters are treated literally. Malformed globs fail before scanning. On Windows, path and glob comparisons are case-insensitive. Filters apply throughout the scan tree so nested projects can be selected.
- CLI `--exclude`, `--include-only`, `--min-age`, and `--max-age` values override their corresponding config values.
- Explicitly supplying any of these CLI flags with an empty value clears that filter from the config.
- When `includeOnly` is configured, it acts as a whitelist and takes precedence over `excludePaths`; otherwise matching excluded roots are skipped.
- Age filters use the newest file modification time within each cache directory. `minAge` keeps caches at least that old; `maxAge` keeps caches no older than that. Durations must be greater than zero; an empty value disables that filter. If both are set, a cache must satisfy both.
- When an age filter is active and a cache's modification time cannot be determined or its inspection is incomplete, the cache is omitted from results and cleanup. A cache directory excluded by age is not traversed for nested findings.
- If an excluded path is inside a cache directory, the enclosing cache is omitted from scan results and cleanup because removing it would delete the excluded path.
- Path and age filters apply to both scans and cleanup.
- Include-only scanning continues through cache-pattern directories that are ancestors of an included project, without selecting or deleting those ancestor directories.

#### Reporting Features
- Summary mode (default): Groups findings by language with totals
- Detailed mode (`--details`): Shows each cache directory found with project path, cache type, language, size, and item count
- JSON output for programmatic processing
- Size measurement in bytes with human-readable output
- Item count tracking per directory

### Behavior
1. **Dry-run by default** - Reports cache directories without deleting files
2. **Recursive scanning** - Walks through directory tree looking for cache patterns
3. **Language filtering** - Can target specific languages via `--languages` flag
4. **Depth control** - Configurable maximum scan depth to limit traversal
5. **Safe deletion** - Requires explicit `--clean` flag with confirmation prompt (unless `--yes` is used)
6. **Error handling** - Deletion errors are logged and reported, but don't stop the process

### Platform Support
- **macOS**: Native support (amd64 and arm64)
- **Linux**: Full support (amd64 and arm64)
- **Windows WSL**: Full support through Linux compatibility

All paths use `filepath.Join()` for cross-platform compatibility. Home directory expansion (`~`) works on all platforms via `os.UserHomeDir()`.

### Safety Considerations
- **Dry-run by default**: The tool never deletes files unless `--clean` is explicitly provided
- **Confirmation prompt**: When using `--clean`, you must confirm the deletion (unless `--yes` is used)
- **Shows what will be deleted**: The tool displays all findings before asking for confirmation
- **Error handling**: Deletion errors are logged and reported, but don't stop the process
- **Git-aware cleanup**: Cleanup skips caches containing tracked files, nested Git repositories, or paths not ignored by Git. `--unsafe-clean` is the explicit override.

### Future Enhancements
- Backup before deletion
- TUI interface

---

## git-cleaner

### Overview
A cross-platform Go CLI tool that scans directories for `.git` directories, reports their sizes, and optionally optimizes repositories using `git gc`.

### Goals
- Cross-platform support (macOS, Linux, Windows WSL)
- Efficient repository discovery
- Safe optimization using official Git commands
- Clear reporting of disk savings

### Key Features

#### CLI Flags
- `--scan PATH` directory to scan for .git directories (required)
- `--clean` runs `git gc` in each repository and shows disk savings

#### Reporting Features
- Table output showing repository paths and `.git` directory sizes
- Item count tracking per repository
- Before/after comparison when `--clean` is used
- Total disk savings calculation with percentage

### Behavior
1. **Recursive scanning** - Walks through directory tree looking for `.git` directories
2. **Size calculation** - Calculates total size by walking through all files in each `.git` directory
3. **Optimization** (with `--clean`) - Runs `git gc` in each repository's parent directory
4. **Rescan after optimization** - After optimization, rescans to calculate disk space saved

### Platform Support
- **macOS**: Native support (amd64 and arm64)
- **Linux**: Full support (amd64 and arm64)
- **Windows WSL**: Full support through Linux compatibility

All paths use `filepath.Join()` for cross-platform compatibility. Home directory expansion (`~`) works on all platforms via `os.UserHomeDir()`.

### Requirements
- Go 1.21 or later
- Git must be installed and available in PATH (for `--clean` functionality)

### Safety Considerations
- Uses official `git gc` command for optimization (safe)
- Only optimizes Git repositories, never deletes them
- Shows clear before/after comparison

### Future Enhancements
- Configuration file support for default scan paths
- Filtering by repository size (only optimize large repos)
- Filtering by last activity date
- Integration with git worktree detection
- Support for Git LFS optimization
- JSON output mode

---

## Common Features Across All Apps

### Cross-Platform Support
All three apps support:
- macOS (amd64 and arm64)
- Linux (amd64 and arm64)
- Windows WSL (via Linux compatibility)

### Development Standards
- Go 1.21+ required
- Consistent Makefile structure
- Pre-commit hooks support
- Test coverage tracking
- Linting with golangci-lint
- Formatting with gofmt

### Output Modes
- Human-readable table output (default)
- JSON output for automation (`--json` flag)
- Detailed mode for granular information (`--details` flag where applicable)

### Safety Features
- Dry-run by default
- Explicit confirmation required for destructive operations
- Clear reporting of what will be affected
- Error handling that doesn't silently fail

---

## Development and Release Workflow

### Makefile Targets

| Target | Description |
|--------|-------------|
| `help` | Show all available make targets |
| `all` | Build all applications (default) |
| `dev-cache` | Build dev-cache application only |
| `git-cleaner` | Build git-cleaner application only |
| `bilge-macos` | Build bilge-macos application only |
| `build` | Build all applications |
| `test` | Run tests in all applications |
| `fmt` | Format code in all applications |
| `lint` | Lint all applications |
| `vet` | Run go vet on all applications |
| `goreleaser-check` | Validate GoReleaser configuration |
| `release-dry-run` | Test GoReleaser without building/publishing |
| `release-snapshot` | Build snapshot release locally (no git tag required) |
| `clean` | Clean all applications and build artifacts |

### Testing a Release Locally

Before creating an actual release, you can test the entire build process:

```bash
# Verify GoReleaser configuration
make goreleaser-check

# Build a local snapshot (no git tag required)
make release-snapshot

# Artifacts will be in ./dist/
# - Individual binaries: dev-cache-darwin-amd64, etc.
# - Homebrew tarball: bilge-VERSION-darwin-{amd64,arm64}.tar.gz
# - Homebrew cask: dist/homebrew/Casks/bilge.rb
```

### Creating a Release

1. Merge changes to `main` and wait for CI to pass on that merge commit.
2. Dispatch GitHub Actions **Publish** on `main` and select patch, minor, or major. The workflow checks the exact commit's CI result, rejects a moving main branch or existing version tag, then creates the next semver tag. Do not push release tags manually.
3. GoReleaser automatically:
   - Builds the unified CLI and task executables for supported Linux, macOS, and Windows architectures
   - Creates GitHub release with:
     - Individual binaries for direct download
     - Combined tarballs for Homebrew
     - Checksums for verification
     - Auto-generated changelog
   - Updates the Homebrew cask in `everydaydevopsio/homebrew-bilge-pump`
   - Commits cask changes with proper checksums

4. Publish checks release assets and platform-specific cask links. Verify the installed version:
   - Check GitHub Releases: https://github.com/everydaydevopsio/bilge-pump/releases
   - Check Homebrew cask: https://github.com/everydaydevopsio/homebrew-bilge-pump/tree/main/Casks
   - Trust and test installation:
     ```bash
     brew trust --cask everydaydevopsio/bilge-pump/bilge
     brew upgrade --cask bilge
     ```

### Troubleshooting Releases

**Cask not found after release:**

If `brew install bilge` fails with "No available cask":

1. Check that the cask was pushed to the tap:
   - Visit: https://github.com/everydaydevopsio/homebrew-bilge-pump/tree/main/Casks
   - You should see `bilge.rb`

2. Update Homebrew and retry:
   ```bash
   brew update
   brew tap everydaydevopsio/bilge-pump
   brew trust --cask everydaydevopsio/bilge-pump/bilge
   brew install --cask bilge
   ```

**Token errors in GitHub Actions:**

If you see authentication errors in the release workflow:

1. Verify the secret exists:
   - Go to: https://github.com/everydaydevopsio/bilge-pump/settings/secrets/actions
   - Look for `HOMEBREW_TAP_GITHUB_TOKEN`

2. Verify the token has the correct permissions:
   - It needs `public_repo` scope
   - It must not be expired

3. Regenerate the token if needed and update the secret

**Checksum mismatches:**

If users report checksum errors:

1. The cask might be out of sync with the release
2. Investigate the failed Publish run and the generated cask before any retry
3. Check that GoReleaser successfully updated the tap
4. Verify the cask at: https://github.com/everydaydevopsio/homebrew-bilge-pump/tree/main/Casks

### GoReleaser Configuration

The project uses GoReleaser for automated releases with the following configuration:

**Builds:**
- 3 separate builds (one per application)
- Each build specifies `dir` to point to the app directory
- Main file is `./main.go` relative to the app directory
- CGO is disabled for static binaries
- Targets darwin/amd64 and darwin/arm64

**Archives:**
- Individual binaries for direct download (format: `{app}-darwin-{arch}`)
- Combined tarball for Homebrew (format: `bilge-{version}-darwin-{arch}.tar.gz`)
- The tarball includes all three binaries

**Homebrew Integration:**
- Automatically generates a cask in `everydaydevopsio/homebrew-bilge-pump`
- The cask installs all three binaries
- Includes caveats with quick start instructions
- Includes version tests for all binaries

**Changelog:**
- Auto-generated from Git commits
- Grouped by type (Features, Bug Fixes, Performance, etc.)
- Excludes internal changes (docs, tests, ci)

### Distribution Strategy

**Primary: Homebrew (Recommended)**
```bash
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/bilge
brew install --cask bilge
```
Run the trust command before installing or upgrading this third-party cask.

Benefits:
- Automatic PATH configuration
- Easy updates via `brew upgrade`
- No sudo required
- Standard for macOS users
- Handles dependencies automatically

**Secondary: Direct Download**
```bash
# Via install script (requires -b flag)
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b $HOME/.local/bin

# Or download binaries directly from GitHub Releases
# https://github.com/everydaydevopsio/bilge-pump/releases
```

**install.sh Behavior:**
- Requires `-b` flag to specify installation directory
- Errors if Homebrew is detected (recommends using `brew install`)
- Downloads pre-built binaries from GitHub Releases
- Supports version selection
- Supports individual app installation with `-a` flag

### Required Setup for Maintainers

To enable automated Homebrew releases:

1. **Create tap repository:**
   - Go to: https://github.com/new
   - Repository name: `homebrew-bilge-pump` (must start with `homebrew-`)
   - Description: "Homebrew tap for bilge tools"
   - Visibility: Public
   - Initialize with README
   - After creation, clone and create `Casks/` directory:
     ```bash
     git clone git@github.com:everydaydevopsio/homebrew-bilge-pump.git
     cd homebrew-bilge-pump
     mkdir Casks
     git add Casks
     git commit -m "Add Casks directory"
     git push origin main
     ```

2. **Configure GitHub token:**
   - Go to: https://github.com/settings/tokens/new
   - Note: "GoReleaser Homebrew Tap"
   - Expiration: Choose appropriate expiration
   - Scopes: Select `public_repo` (allows read/write to public repositories)
   - Click "Generate token"
   - **IMPORTANT**: Copy the token immediately (you won't see it again)
   - Add as repository secret in the bilge repository:
     - Go to: https://github.com/everydaydevopsio/bilge-pump/settings/secrets/actions
     - Click "New repository secret"
     - Name: `HOMEBREW_TAP_GITHUB_TOKEN`
     - Value: Paste the token you copied
     - Click "Add secret"

3. **Verify workflow:**
   - GitHub Actions workflow is in `.github/workflows/publish.yml`
   - Uses `goreleaser/goreleaser-action@v7`
   - Requires Go 1.22.x

4. **Test after first release:**
   ```bash
   # Add the tap and install
   brew tap everydaydevopsio/bilge-pump
   brew trust --cask everydaydevopsio/bilge-pump/bilge
   brew install --cask bilge --verbose

   # Verify all binaries are installed
   which dev-cache
   which git-cleaner
   which bilge-macos

   # Test functionality
   dev-cache --version
   git-cleaner --version
   bilge-macos --version

   # Audit the cask
   brew audit --strict --online bilge

   # Test uninstall (optional)
   brew uninstall bilge
   brew untap everydaydevopsio/bilge-pump
   ```

### Future: Migrating to Homebrew Core

Once the cask is stable and has users, the project can be submitted to homebrew-core for wider distribution:

1. Review the [Homebrew contribution guidelines](https://docs.brew.sh/How-To-Open-a-Homebrew-Pull-Request)
2. Ensure the cask passes `brew audit --strict --online`
3. Submit a PR to [Homebrew/homebrew-core](https://github.com/Homebrew/homebrew-core)
4. After acceptance, users can install with just:
   ```bash
   brew install bilge
   ```
   (no tap required)
