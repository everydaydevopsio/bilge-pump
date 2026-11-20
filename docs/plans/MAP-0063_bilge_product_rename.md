---
ticket: MAP-0063
type: plan
status: in-progress
owner: Codex
created: 2026-11-10
mode: approval-required
---
# MAP-0063 — Rename the public product to Bilge

## Context
The repository and release identity are `bilge-pump`, but the public unified command and cask still use the old product name. The user requested a complete rename and a minor release after v2.0.1.

## Scope
- In scope: rename the public executable and Homebrew cask to `bilge`; rename platform executables and paths; update installers, GoReleaser, CI, config names, documentation, and tests; remove old product references from tracked source and the tap; publish v2.1.0 after both PRs and main CI are green; install and verify locally.
- Out of scope: renaming the GitHub repository `bilge-pump`, `dev-cache`, or `git-cleaner`.

## Constraints
- Keep `bilge-pump` as the GitHub repository and XDG config directory.
- Bundle the launcher and platform tasks together as before.
- Do not tag until the rename is merged and main CI is green.
- Update the Homebrew tap without leaving two competing casks.

## Risks and Tradeoffs
- Renaming executables and the cask is a breaking change for scripts and existing Homebrew installations; v2.1.0 is the explicit migration boundary.
- The tap is a separate repository and needs its own branch, PR, checks, and merge before the release is complete.
- Installers, task dispatch, release archives, cask artifacts, and config paths must agree on each new platform binary name.

## Execution Checklist
- [ ] Update PRD and implementation plan for the v2.1.0 product name.
- [ ] Rename unified and platform binaries, package directories, config names, installer selection, release assets, workflows, and docs.
- [ ] Update the Homebrew tap cask path/name and remove the previous cask.
- [ ] Verify no tracked file in either repository contains the previous product name.
- [ ] Run full tests, build matrix, installer checks, GoReleaser validation, and Homebrew cask checks.
- [ ] Merge both PRs and wait for main CI before tagging v2.1.0.
- [ ] Verify release assets and cask update, upgrade local install, and walk through safe commands.

## Test Strategy
- Run `make test` and platform cross-builds.
- Run installer smoke tests on Unix and validate the PowerShell installer references.
- Run `goreleaser check`, `sh -n install.sh`, and Ruby syntax/audit checks for the tap.
- Verify `rg` finds no former product references in tracked files after the rename.
- Verify the installed `bilge --version`, help, initialization, default dry-run, and task selection.

## Rollback Strategy
- Trigger: broken release assets, cask install, or task dispatch on a supported OS.
- Rollback steps: publish a corrected v2.1.x release and cask; restore the prior release only if the new binaries cannot safely run.
- Validation after rollback: confirm all assets match the cask and the installed command can scan without cleanup.

## Outcome
- Result: Pending implementation and release.
- Evidence links/commands: Pending.
