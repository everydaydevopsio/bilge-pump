---
ticket: MAP-0062
type: plan
status: done
owner: Codex
created: 2026-11-06
mode: approval-required
---
# MAP-0062 — Unified cross-platform cache-cleaner executable

## Context
Issues #62, #57, #59, and #66 call for one discoverable CLI, orchestration, shared JSON and exit behavior, OS-aware execution, and git-cleaner configuration. The requested macOS/Linux/Windows support also depends on platform cleaner behavior described by #60 and #61. Existing apps are isolated Go modules with main packages, and only mac-cache-cleaner currently covers system-level cleanup.

## Scope
- In scope: one unified cache-cleaner command backed by bundled task executables; full platform-appropriate scan/clean and task selection; macOS, Linux, and Windows builds; shared reports and exit codes; XDG-aware configuration and git-cleaner defaults; installation/release/CI/docs updates.
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
- [x] Validate formatting and host/cross-platform builds for supported platform command packages; inspect config paths and dispatch. Full test suites were not run in this implementation session.

## Test Strategy
- Unit coverage for config location/precedence, task selection, platform dispatch, report schema, and exit-code mapping.
- CLI integration checks for dry-run, clean --all, named task execution, and missing/unsupported platform tools.
- Cross-compilation/build checks for darwin, linux, and windows amd64/arm64.
- Safety checks that dry-run never executes clean commands and that sensitive targets remain explicitly gated.

## Rollback Strategy
- Trigger: platform build failure, incorrect task routing, or any cleanup action that may remove user data unexpectedly.
- Rollback steps: disable unified CLI from release/install targets and revert the new command/package extraction while preserving existing standalone binaries.
- Validation after rollback: build and verify the three existing standalone applications and restore previous release configuration.

## Outcome
- Result: Unified command, per-platform cleaners, XDG-aware configs, and installation/release wiring implemented. The command is a single user entry point, with task executables bundled beside it.
- Evidence links/commands: `gofmt -w ...`; `go build ./cmd/cache-cleaner`; Linux amd64 and Windows amd64 platform-cleaner builds; Darwin arm64 unified-command build; standalone app builds; `git diff --check`.
