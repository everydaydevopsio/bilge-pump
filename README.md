# bilge

[![CI](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/ci.yml)
[![Release](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml/badge.svg)](https://github.com/everydaydevopsio/bilge-pump/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/everydaydevopsio/bilge-pump?filename=dev-cache%2Fgo.mod)](https://go.dev/)
[![License](https://img.shields.io/github/license/everydaydevopsio/bilge-pump)](LICENSE)
[![GitHub Release](https://img.shields.io/github/v/release/everydaydevopsio/bilge-pump)](https://github.com/everydaydevopsio/bilge-pump/releases)

A cross-platform Go cleanup toolkit with a unified `bilge` command for macOS, Linux, and Windows.

Install with Homebrew by tapping the repository, trusting its cask, and installing:

```bash
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/bilge
brew install --cask bilge
```

## Apps

### bilge
Unified entry point for scanning or cleaning project caches, Git repositories, and the current OS's developer caches. Run `bilge scan` for a dry run, `bilge clean --all` to run the configured cleanup tasks, or select one task with `bilge clean dev-cache` / `bilge clean git-cleaner`. Add `--yes` to skip the project-cache confirmation prompt; the unified command sends it only to `dev-cache`.

The release installs the unified command alongside its task executables in one directory. The unified command locates those executables beside itself, so users run one command without adding each tool to `PATH`.

### bilge-macos
`bilge-macos` runs **safe, official cleanup commands** for developer tools (Docker, npm, Homebrew, etc.), never `rm -rf`.

### dev-cache
Cross-platform tool that scans source directories for project cache directories (like `node_modules`, `.venv`, `target`, etc.) across multiple languages.

### git-cleaner
Cross-platform tool that finds `.git` directories, reports their sizes, and optionally optimizes repositories with `git gc`.

### bilge-linux and bilge-windows
`bilge-linux` and `bilge-windows` provide platform-specific developer cleanup for Linux and native Windows. Both support dry-run, `--clean`, `--targets`, `--json`, `--init`, and `--config`.

## Install

### Recommended: Homebrew (macOS)

```bash
brew tap everydaydevopsio/bilge-pump
brew trust --cask everydaydevopsio/bilge-pump/bilge
brew install --cask bilge
```

Run the trust command before installing or upgrading this third-party cask.

This installs `bilge`, `dev-cache`, `git-cleaner`, and the macOS platform cleaner.

### Linux

```bash
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b "$HOME/.local/bin"
```

### Windows

Download and run [`install.ps1`](install.ps1) from PowerShell:

```powershell
.\install.ps1
```

It installs the Windows executables under `%LOCALAPPDATA%\Programs\bilge`. Add that directory to your user `PATH`.

The release includes the unified command plus the platform tasks in the same directory. Run `bilge --init` to create the editable unified config and set up the Bilge MCP server and disk-savings skill in installed Codex and Claude CLIs. Running it again preserves an existing config and agent registrations. Platform cleaner configs can be initialized with `bilge-linux --init` or `bilge-windows --init`.

### Alternative: Direct Download

If you don't have Homebrew, you can use the install script:

```bash
# Install the unified command and platform tools to ~/.local/bin
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b $HOME/.local/bin

# Install a specific app only
curl -sSfL https://raw.githubusercontent.com/everydaydevopsio/bilge-pump/HEAD/install.sh | sh -s -- -b $HOME/.local/bin -a bilge-macos
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
- Use `-a bilge` to install the unified launcher with `dev-cache`, `git-cleaner`, and the current OS platform cleaner. Use `-a <standalone-app>` to install only that standalone tool; omit `-a` to install the complete toolkit.
- The script exits early with an error (without partial installs) if required flags or tools are missing.

## Quick Start

### Unified command
```bash
bilge --init                # create a starter config
bilge scan                  # dry-run all configured tasks
bilge clean --all           # run all configured cleanup tasks
bilge clean --all --yes     # skip project-cache confirmation
bilge clean dev-cache       # run one cleanup task
bilge --json scan           # shared JSON envelope
```

The unified config lives at `$XDG_CONFIG_HOME/bilge-pump/config.yaml` when `XDG_CONFIG_HOME` is set, otherwise under `~/.config/bilge-pump/` on macOS/Linux or the Windows user config directory. It selects a default scan path and task list. Each task can also be configured with its standalone tool's config file. The platform cleaner YAML exposes target `enabled`, `paths`, and `cmds` entries. `git-cleaner --init` creates its own config and supports `--config` and `--scan` overrides.

### MCP server and disk-savings skill

`bilge mcp` runs a local MCP server over stdio. It wraps the installed Bilge task executables; no network listener or separate binary is needed. After installing Bilge, run:

```bash
bilge --init
```

This registers the current `bilge` executable as an MCP server named `bilge` in Codex and Claude Code when their CLIs are installed. It also installs the `bilge-disk-savings` skill to `~/.codex/skills/` and `~/.claude/skills/`. On Windows these directories are under the user home directory. The registration uses the stable Homebrew `bin/bilge` symlink when available. Restart an existing agent session to pick up the new server and skill. If either CLI is missing, Bilge reports that registration was skipped; install the CLI and run `bilge --init` again. Existing MCP registrations and edited skill files are preserved.

| MCP tool | Purpose |
| --- | --- |
| `bilge_scan` | Dry-run all tasks or one of `dev-cache`, `git-cleaner`, or `system`. Supports the corresponding scan path, target, language, depth, path, and age filters. |
| `bilge_clean` | Run the same selected cleanup, with `confirm=true` after user authorization. Project-cache confirmation is supplied non-interactively. `unsafe_clean=true` bypasses the Git safety check and needs separate authorization. |
| `bilge_list_targets` | List enabled and disabled targets for the current OS cleaner. |
| `bilge_check_tools` | Check command dependencies for macOS cleaner targets; advertised only on macOS. |

The skill uses scans to rank measured cache sizes when disk space is tight, asks before cleanup, and scans again afterward. System commands such as Docker prune may free space that Bilge cannot estimate from directory sizes. The server caps command output and execution time and returns an error for oversized or timed-out scans; narrow the scope and retry if needed.

### bilge-macos
```bash
bilge-macos --init
bilge-macos                    # dry-run (summary)
bilge-macos --details          # show per-directory details
bilge-macos --clean            # execute cleanup
bilge-macos --targets brew,npm --clean
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

Project-cache deletion skips paths that are not Git-ignored or contain tracked files. `dev-cache --unsafe-clean` explicitly bypasses that check. The unified command forwards `--exclude`, `--include-only`, `--min-age`, and `--max-age` when `dev-cache` is selected, for example `bilge dev-cache --include-only ~/src/project --min-age 30d` or `bilge clean dev-cache --exclude ~/src/archive`.

## Troubleshooting

### Cask not found

If `brew install bilge` fails with "No available cask":

1. Update Homebrew and retry:
   ```bash
   brew update
   brew tap everydaydevopsio/bilge-pump
   brew trust --cask everydaydevopsio/bilge-pump/bilge
   brew install --cask bilge
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
   brew install bilge --verbose
   ```

3. If problems persist, check the [GitHub Issues](https://github.com/everydaydevopsio/bilge-pump/issues)

## Documentation

Each app has detailed documentation in its own directory:
- [`bilge-macos/README.md`](bilge-macos/README.md) - Full bilge-macos docs
- [`dev-cache/README.md`](dev-cache/README.md) - Full dev-cache docs
- [`git-cleaner/README.md`](git-cleaner/README.md) - Full git-cleaner docs
- [`docs/scheduling.md`](docs/scheduling.md) - cron and launchd scheduling examples

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
make bilge-macos
```

Run tests/lint for all:
```bash
make test
make lint
make vet
```

### Release

**How to trigger a release**

After CI passes on `main`, open GitHub Actions → **Publish**, select **Run workflow** on `main`, and choose patch, minor, or major. Publish checks that CI passed on that exact commit, calculates the next version tag, and publishes through GoReleaser. If `main` moves before tagging, dispatch Publish again. Do not push release tags manually.

**Prerequisites**

- `GITHUB_TOKEN` is provided automatically by GitHub Actions
- `HOMEBREW_TAP_GITHUB_TOKEN` must be configured in the repository's GitHub secrets for Homebrew tap updates

**What should happen:**

1. Publish validates the current main commit and creates its version tag
2. GoReleaser will:
   - Build the unified CLI and task executables for supported Linux, macOS, and Windows architectures
   - Create a GitHub release with binaries attached
   - Generate a Homebrew cask
   - Push the cask to `homebrew-bilge-pump` repository

3. Publish verifies release assets and the platform-specific Homebrew cask links. Check the results:
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
brew uninstall --cask bilge
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
4. Repeat for `git-cleaner` and `bilge-macos` (the latter requires macOS).

## Questions?

Open an issue on GitHub: https://github.com/everydaydevopsio/bilge-pump/issues

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Author

Mark C Allen ([@markcallen](https://github.com/markcallen))
