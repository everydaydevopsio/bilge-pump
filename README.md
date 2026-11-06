# cache-cleaner

[![CI](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml)
[![Release](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/everydaydevopsio/bilge-pump?filename=dev-cache%2Fgo.mod)](https://go.dev/)
[![License](https://img.shields.io/github/license/everydaydevopsio/bilge-pump)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/everydaydevopsio/bilge-pump)](https://github.com/everydaydevopsio/bilge-pump/releases)

A cross-platform Go cleanup toolkit with a unified `cache-cleaner` command for macOS, Linux, and Windows.

## Repository move

This project has moved from [`markcallen/cache-cleaner`](https://github.com/markcallen/cache-cleaner) to [`everydaydevopsio/bilge-pump`](https://github.com/everydaydevopsio/bilge-pump). The Homebrew tap moved from `markcallen/homebrew-cache-cleaner` to [`everydaydevopsio/homebrew-bilge-pump`](https://github.com/everydaydevopsio/homebrew-bilge-pump).

Update an existing Git checkout with:

```bash
git remote set-url origin git@github.com:everydaydevopsio/bilge-pump.git
git fetch origin
git pull --ff-only
```

To move an existing Homebrew installation to the new tap:

```bash
brew untap markcallen/cache-cleaner
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/cache-cleaner
brew reinstall --cask cache-cleaner
```

For a new Homebrew installation, tap the repository, trust its cask, and install:

```bash
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/cache-cleaner
brew install --cask cache-cleaner
```

## Apps

### cache-cleaner
Unified entry point for scanning or cleaning project caches, Git repositories, and the current OS's developer caches. Run `cache-cleaner scan` for a dry run, `cache-cleaner clean --all` to run the configured cleanup tasks, or select one task with `cache-cleaner clean dev-cache` / `cache-cleaner clean git-cleaner`.

The release installs the unified command alongside its task executables in one directory. The unified command locates those executables beside itself, so users run one command without adding each tool to `PATH`.

### mac-cache-cleaner
macOS cache cleaner that runs **safe, official cleanup commands** for developer tools (Docker, npm, Homebrew, etc.), never `rm -rf`.

### dev-cache
Cross-platform tool that scans source directories for project cache directories (like `node_modules`, `.venv`, `target`, etc.) across multiple languages.

### git-cleaner
Cross-platform tool that finds `.git` directories, reports their sizes, and optionally optimizes repositories with `git gc`.

### linux-cache-cleaner and windows-cache-cleaner
Platform-specific developer cache cleaners for Linux and native Windows. Both support dry-run, `--clean`, `--targets`, `--json`, `--init`, and `--config`.

## Install

### Recommended: Homebrew (macOS)

```bash
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/cache-cleaner
brew install --cask cache-cleaner
```

Run the trust command before installing or upgrading this third-party cask.

This installs `cache-cleaner`, `dev-cache`, `git-cleaner`, and the macOS platform cleaner.

### Linux

```bash
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b "$HOME/.local/bin"
```

### Windows

Download and run [`install.ps1`](install.ps1) from PowerShell:

```powershell
.\install.ps1
```

It installs the Windows executables under `%LOCALAPPDATA%\Programs\cache-cleaner`. Add that directory to your user `PATH`.

The release includes the unified command plus the platform tasks in the same directory. Run `cache-cleaner --init` once to create the editable unified config; platform cleaner configs can be initialized with `linux-cache-cleaner --init` or `windows-cache-cleaner --init`.

### Alternative: Direct Download

If you don't have Homebrew, you can use the install script:

```bash
# Install the unified command and platform tools to ~/.local/bin
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b $HOME/.local/bin

# Install a specific app only
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b $HOME/.local/bin -a mac-cache-cleaner
```

**Important**: The install script requires the `-b` flag to specify an installation directory. If you omit the `-b` flag, the script will error out and recommend using Homebrew instead.

**Note**: Make sure `~/.local/bin` is in your PATH:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

Add this to your shell profile (`~/.zshrc` or `~/.bashrc`) to make it permanent.

#### install.sh behavior

- The script installs from the latest GitHub release archives.
- `-b <dir>` is mandatory and determines where binaries are written.
- Use `-a <app>` to install a single binary (`dev-cache`, `git-cleaner`, or `mac-cache-cleaner`); omit `-a` to install all three.
- The script exits early with an error (without partial installs) if required flags or tools are missing.

## Quick Start

### Unified command
```bash
cache-cleaner --init                # create a starter config
cache-cleaner scan                  # dry-run all configured tasks
cache-cleaner clean --all           # run all configured cleanup tasks
cache-cleaner clean dev-cache       # run one cleanup task
cache-cleaner --json scan           # shared JSON envelope
```

The unified config lives at `$XDG_CONFIG_HOME/bilge-pump/config.yaml` when `XDG_CONFIG_HOME` is set, otherwise under `~/.config/bilge-pump/` on macOS/Linux or the Windows user config directory. It selects a default scan path and task list. Each task can also be configured with its standalone tool's config file. The platform cleaner YAML exposes target `enabled`, `paths`, and `cmds` entries. `git-cleaner --init` creates its own config and supports `--config` and `--scan` overrides.

### mac-cache-cleaner
```bash
mac-cache-cleaner --init
mac-cache-cleaner                    # dry-run (summary)
mac-cache-cleaner --details          # show per-directory details
mac-cache-cleaner --clean            # execute cleanup
mac-cache-cleaner --targets brew,npm --clean
```

### dev-cache
```bash
dev-cache --init
dev-cache                            # scan for cache directories (dry-run)
dev-cache --json                     # machine-readable output
dev-cache --scan ~/projects --clean  # delete found caches
```

### git-cleaner
```bash
git-cleaner --scan ~/src             # find and report .git sizes
git-cleaner --scan ~/src --clean     # optimize with git gc
```

Project-cache deletion skips paths that are not Git-ignored or contain tracked files. `dev-cache --unsafe-clean` explicitly bypasses that check.

## Troubleshooting

### Cask not found

If `brew install cache-cleaner` fails with "No available cask":

1. Update Homebrew and retry:
   ```bash
   brew update
   brew tap everydaydevopsio/bilge-pump
   brew trust --cask everydaydevopsio/bilge-pump/cache-cleaner
   brew install --cask cache-cleaner
   ```

2. Verify the cask exists at: https://github.com/everydaydevopsio/homebrew-bilge-pump/tree/main/Casks

### Installation issues

If you encounter issues during installation:

1. Check that Homebrew is up to date:
   ```bash
   brew update
   brew doctor
   ```

2. Try installing with verbose output to see detailed logs:
   ```bash
   brew install cache-cleaner --verbose
   ```

3. If problems persist, check the [GitHub Issues](https://github.com/everydaydevopsio/bilge-pump/issues)

## Documentation

Each app has detailed documentation in its own directory:
- [`mac-cache-cleaner/README.md`](mac-cache-cleaner/README.md) - Full mac-cache-cleaner docs
- [`dev-cache/README.md`](dev-cache/README.md) - Full dev-cache docs
- [`git-cleaner/README.md`](git-cleaner/README.md) - Full git-cleaner docs

## Development

### Prerequisites

To develop on this project, you need the following tools installed:

- **Go** 1.21 or higher ([download](https://go.dev/dl/))
- **Make** (typically pre-installed on macOS/Linux)
- **GoReleaser** (optional, for releases): `brew install goreleaser`
- **pre-commit** (optional, for Git hooks): `pip install pre-commit`

You can verify all required tools are installed by running:
```bash
make check-tools
```

### Building

Build all apps:
```bash
make all
```

Build specific app:
```bash
make dev-cache
make git-cleaner
make mac-cache-cleaner
```

Run tests/lint for all:
```bash
make test
make lint
make vet
```

### Release

**How to trigger a release**

Releases are triggered by pushing a version tag matching `vX.Y.Z` (e.g. `v1.0.0`, `v2.3.1`):

```bash
git tag v1.0.0
git push origin v1.0.0
```

**Prerequisites**

- `GITHUB_TOKEN` is provided automatically by GitHub Actions
- `HOMEBREW_TAP_GITHUB_TOKEN` must be configured in the repository's GitHub secrets for Homebrew tap updates

**What should happen:**

1. GitHub Actions will trigger the release workflow
2. GoReleaser will:
   - Build all 3 binaries for darwin/amd64 and darwin/arm64
   - Create a GitHub release with binaries attached
   - Generate a Homebrew cask
   - Push the cask to `homebrew-bilge-pump` repository

3. Check the results:
   - Release: https://github.com/everydaydevopsio/bilge-pump/releases
   - Cask: https://github.com/everydaydevopsio/homebrew-bilge-pump/tree/main/Casks
   - Actions log: https://github.com/everydaydevopsio/bilge-pump/actions


Testing locally

```bash
# Install GoReleaser for local testing
brew install goreleaser

# Test the configuration
make goreleaser-check

# Test a snapshot build (without releasing)
make release-snapshot

# Test release process without building
make release-dry-run
```

### Testing Release After First Release

After creating your first release, verify the installation works correctly:

```bash
# Add the tap and install
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/cache-cleaner
brew install --cask cache-cleaner --verbose

# Verify all binaries are installed
which dev-cache
which git-cleaner
which mac-cache-cleaner

# Test functionality
dev-cache --version
git-cleaner --version
mac-cache-cleaner --version

# Audit the cask
brew audit --strict --online cache-cleaner

# Test uninstall (optional)
brew uninstall --cask cache-cleaner
brew untap everydaydevopsio/bilge-pump
```

### Release Smoke Test

For each tagged release:

1. Download the desired binary from the GitHub release assets.
2. Make it executable and confirm the version string:
   ```bash
   chmod +x dev-cache-darwin-arm64
   ./dev-cache-darwin-arm64 --version
   ```
3. Run a dry-run command to ensure the binary starts up correctly (no cleanup):
   ```bash
   ./dev-cache-darwin-arm64 --help
   ```
4. Repeat for `git-cleaner` and `mac-cache-cleaner` (the latter requires macOS).

## Questions?

Open an issue on GitHub: https://github.com/everydaydevopsio/bilge-pump/issues

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Author

Mark C Allen ([@markcallen](https://github.com/markcallen))
