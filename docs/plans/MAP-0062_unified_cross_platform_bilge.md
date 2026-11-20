---
ticket: MAP-0062
type: plan
status: done
owner: Codex
created: 2026-11-06
mode: approval-required
---
# MAP-0062 — Unified cross-platform bilge executable

## Context
Issues #62, #57, #59, and #66 call for one discoverable CLI, orchestration, shared JSON and exit behavior, OS-aware execution, and git-cleaner configuration. The requested macOS/Linux/Windows support also depends on platform cleaner behavior described by #60 and #61. Existing apps are isolated Go modules with main packages, and only bilge-macos currently covers system-level cleanup.

## Scope
- In scope: one unified bilge command backed by bundled task executables; full platform-appropriate scan/clean and task selection; macOS, Linux, and Windows builds; shared reports and exit codes; XDG-aware configuration and git-cleaner defaults; installation/release/CI/docs updates.
- Out of scope: removing existing standalone tools; any cleanup action that lacks a safe, documented platform command; changing unrelated target policies.

## Constraints
- Preserve dry-run as the default and require an explicit cleanup command.
- Honor XDG_CONFIG_HOME; use platform user config directories where XDG does not apply.
- Bundle the task executables beside the unified command so users install one toolkit and invoke one entry point; do not require Go or separate PATH setup at runtime.
- Treat Windows support as native Windows, distinct from WSL/Linux.

## Risks and Tradeoffs
- The existing tools remain separate executables bundled together; the unified command is the single user entry point and locates its siblings.
- Linux and Windows cleanup commands and cache paths differ materially from macOS; unsafe or unsupported tasks must report as unavailable rather than guess.
- Existing standalone JSON and exit semantics may be relied on by scripts; shared behavior needs explicit compatibility notes.
- Cleanup command review is safety-sensitive, especially Docker volumes, package-manager operations, and elevated system targets.

## Execution Checklist
- [x] Add unified CLI/task routing that bundles and locates task executables beside the command.
- [x] Add platform cleaner implementations and target defaults for Linux and Windows; retain macOS behavior with safe cleanup defaults.
- [x] Add unified XDG-aware configuration, git-cleaner config support, and flag precedence.
- [x] Standardize the unified JSON report envelope and aggregate exit codes.
- [x] Update build, release, installation, CI, and user documentation for three OSes.
- [x] Validate formatting, full test suites, and the host/cross-platform build matrix for supported command packages; inspect config paths and dispatch.
- [x] Fix review findings: built-in defaults for missing legacy default configs, Windows ZIP release archive, and safe unified flag routing/validation.
- [x] Fix follow-up review findings: honor explicit boolean values, fail on missing explicit config paths, and locate nested project repositories from cache directories.
- [x] Fix additional review findings: validate task names, honor all boolean flag values, reject missing explicit git-cleaner config, and show platform target cleanup commands during scans.
- [x] Fix latest review findings: preserve explicit-config errors in legacy tools when the path equals the default, and safely invoke Windows batch commands with arguments intact.
- [x] Fix new review findings: keep macOS scan progress off JSON stdout, stage local host binaries beside the unified launcher, reject nested Git repositories inside cache candidates, fail closed on cache inspection errors, reject percent signs in Windows batch commands, isolate cross-build outputs, install all unified task binaries when selected, clear macOS quarantine from the launcher, propagate platform inspection failures, and reject invalid target selections.

## Test Strategy
- Unit coverage for config location/precedence, task selection, platform dispatch, report schema, and exit-code mapping.
- Regression coverage for fresh default-config startup, task flag routing, and rejecting invalid multi-task cleanup arguments before execution.
- Regression coverage for explicit false values, missing explicit configs, and nested Git repositories in cache cleanup safety checks.
- Regression coverage for unknown task rejection, all boolean flag values, explicit git-cleaner config errors, and report-only/planned platform targets.
- Regression coverage for explicitly named missing legacy default paths and native Windows batch arguments containing spaces and metacharacters.
- Regression coverage for nested Git directories and gitfiles inside ignored cache candidates; native Windows tests reject percent signs in command paths and arguments.
- Regression coverage for valid macOS JSON output; local build verification checks the launcher and host task executables share `build/`.
- Installer smoke coverage verifies that `-a bilge` installs the complete current-platform bundle while standalone selection remains single-binary; cross-build targets use separate OS/architecture directories.
- Platform inspection tests distinguish absent optional paths from traversal failures and ensure filesystem errors can reach the report's partial-failure path.
- Target selection tests reject unknown-only, mixed-invalid, empty, and malformed `--targets` values before scan or cleanup dispatch.
- CLI integration checks for dry-run, clean --all, named task execution, and missing/unsupported platform tools.
- Cross-compilation/build checks for darwin, linux, and windows amd64/arm64.
- Safety checks that dry-run never executes clean commands and that sensitive targets remain explicitly gated.

## Rollback Strategy
- Trigger: platform build failure, incorrect task routing, or any cleanup action that may remove user data unexpectedly.
- Rollback steps: disable unified CLI from release/install targets and revert the new command/package extraction while preserving existing standalone binaries.
- Validation after rollback: build and verify the three existing standalone applications and restore previous release configuration.

## Outcome
- Result: Unified command, per-platform cleaners, XDG-aware configs, and installation/release wiring implemented. Legacy cleaners use in-memory defaults only for implicit defaults; explicit missing configs fail; boolean values are respected; task names are allowlisted; nested Git repositories are found from cache paths and rejected inside deletion candidates; cache inspection fails closed; platform scans report inspection failures as partial failures while ignoring absent optional paths; unknown and malformed platform target selections are rejected before scanning or cleanup; scans show planned commands for pathless targets; macOS scan JSON remains structured; local builds stage host task binaries beside the launcher; cross-build artifacts are isolated by OS and architecture; selecting the unified installer installs all required task binaries; Windows batch commands preserve supported argument boundaries and reject unsafe percent expansion; macOS post-install removes quarantine from the launcher; Windows archives match the PowerShell installer; and task arguments are validated before cleanup begins.
- Evidence links/commands: `make test`; installer smoke test through `scripts/test-install.sh`; macOS JSON integration test included in `make test`; local host bundle build; cross-build matrix for unified and platform commands; native Windows batch integration test in CI; `goreleaser check --config .goreleaser.yaml`; `git diff --check`.
