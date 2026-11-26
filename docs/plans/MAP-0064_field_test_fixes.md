---
ticket: MAP-0064
type: plan
status: ready
owner: Claude
created: 2026-11-26
mode: mixed (Phase 1 autonomous; Phases 2-3 approval-required)
---
# MAP-0064 — Fix issues found in the Claude Code field test

## Context
A Claude Code session used Bilge v2.2.3 to free disk space on a developer Mac. The MCP server rejected every call, several targets report space they cannot clean, and a stale user config would have pruned Docker volumes. Evidence, repro commands and the session log are in [docs/reports/2026-11-26-claude-code-field-test.md](../reports/2026-11-26-claude-code-field-test.md). Finding IDs (F1–F17) below refer to that report.

## Agent start here
1. `git checkout main && git pull --ff-only`. Line references assume `main` at or after `06a65b1` (v2.2.3).
2. Read `AGENTS.md`, `PRD.md`, this plan, and the field-test report.
3. Work Phase 1 items in order, **one branch and one PR per item** (branch `fix/map-0064-<item-id>-<slug>`). Follow TDD: write the failing test first, confirm it fails, then fix.
4. **Stop before Phase 2.** Each Phase 2 item needs a decision recorded in the Decisions table below. If its Decision column is empty, ask the human, record the answer here, then proceed.
5. For each item, update the governing `PRD.md` section in the same PR when behaviour changes, run `make test`, `make vet`, `make lint`, and keep coverage at or above the repo minimum. Request a Copilot review on each PR (`AGENTS.md`).
6. Tick items off in the Execution Checklist and fill in Outcome as PRs merge.
7. Keep `PRIORITIES.md` and GitHub issues in sync. Link each PR to the related issues below with `Refs #N` (or `Fixes #N` when it fully closes one). Tick the matching `PRIORITIES.md` box in the same PR.

## Related work already tracked
Much of this overlaps existing priorities. Don't open duplicate issues; comment on the existing one with the field-test evidence instead. Open a new issue only for F1, F2, F5, F8, F9 and F13.

| Finding | Existing tracking |
| --- | --- |
| F3 Docker context/OrbStack | #55, #80, `PRIORITIES.md` P0 (volumes) and P1 (OrbStack) |
| F4 Android | #80, `PRIORITIES.md` P0 |
| F6, F14 Exit codes and JSON | #63, #65, `PRIORITIES.md` P1 |
| F7 Measured vs cleanable, Gradle | #37, #80, `PRIORITIES.md` P1 |
| F10, F12 dev-cache safety and broad patterns | #80, `PRIORITIES.md` P0/P1 |
| F15 Speed | #64 |
| F17 Coverage gaps | #54, #80, `PRIORITIES.md` P2 (largest-items report) |
| F2 Config drift | `TODO.md` "Centralize config schema/version guidance" |

`PRIORITIES.md` looks stale: its P0 boxes for the Git-tracked refusal and for preserving Docker volumes are unchecked, yet `main` already skips Git-tracked dirs (observed in the field test) and the built-in docker default no longer uses `--volumes`. Verify both and tick them in the first PR. Note that the docker fix doesn't reach existing users until P2-1 (F2) lands.

## Scope
- In scope: F1–F16 in this repository.
- Out of scope: the Homebrew cask `postflight` warning (separate `homebrew-bilge-pump` repo; open an issue there instead); new targets in F17 beyond those listed in Phase 3; Linux/Windows cleaners unless a fix is shared code.

## Constraints
- Dry-run stays the default; nothing in this plan may make a scan destructive.
- Never weaken the dev-cache Git safety check.
- `AGENTS.md` principle 2 says "never direct `rm -rf`". Item P2-6 (`delete-paths`) changes that principle and needs decision D5 first.
- JSON output is consumed by the MCP server and by scripts. Schema changes must be additive, or versioned and called out in the PR.

## Decisions (human fills in; agent stops on any empty row it needs)

| ID | Question | Recommended default | Decision |
| --- | --- | --- | --- |
| D1 | Config model: overrides-only merged over built-in defaults, or versioned full config with warnings when a target differs destructively from the default? | Overrides-only, with a one-time migration that rewrites existing full configs to just their differences | Overrides-only + one-time migration (backup to `config.yaml.bak`). Decided 2026-11-26. |
| D2 | Docker on a non-local context: refuse, or warn and require a flag? | Refuse unless `--docker-context-remote` is passed | Refuse unless `--docker-context-remote`. Decided 2026-11-26. |
| D3 | `brew autoremove`: drop from default brew target, or keep with a warning? | Move to an opt-in `brew-autoremove` target, disabled by default | Opt-in `brew-autoremove` target, disabled by default. Decided 2026-11-26. |
| D4 | Exit codes: 0 = all clean, 1 = partial (skips or some targets failed), 2 = fatal? Or keep 3? | 0 success incl. intended safety skips; 1 one or more commands failed; 2 usage/config error | 0 success incl. safety skips / 1 command failed / 2 usage-config error; exit 3 removed. Decided 2026-11-26. |
| D5 | Allow a `delete-paths` action (Bilge deletes a directory itself) for caches with no official CLI (Gradle, Xcode DerivedData, Terraform plugin cache, `~/.npm/_npx`)? | Yes, for an explicit allow-list of cache dirs, never user-configurable globs; update `AGENTS.md` principle 2 | Yes, fixed allow-list only; update `AGENTS.md` principle 2. Decided 2026-11-26. |
| D6 | dev-cache default discovery: raise default depth, or discover repos by finding `.git` dirs? | Discover by `.git` (bounded at depth 4), keep `--depth` as an override | Discover by `.git` (depth 4), `--depth` override. Decided 2026-11-26. |

## Execution Checklist

### Phase 1 — Autonomous, small, no contract changes
- [ ] **P1-1 (F1) MCP accepts `_meta`.** In `cmd/bilge/mcp.go`, decode `tools/call` params into a struct that tolerates unknown top-level fields (at minimum `_meta`); keep `DisallowUnknownFields` for `arguments`. Return the decoder error text in the `-32602` message. Tests: Claude Code-shaped request with `params._meta` succeeds for all four tools; unknown field inside `arguments` still fails; missing `name` still fails. Afterwards, verify from a real Claude Code session that `bilge_scan` works. **Ship first, as a patch release.**
- [ ] **P1-2 (F13) Python pip resolution.** Prefer `python3 -m pip cache purge`; if pip is below 20.1 or missing, skip with a warning instead of failing. Test with a fake `PATH` containing a stub old pip.
- [ ] **P1-3 (F12) Language labels.** Confirm the cause in `dev-cache/main.go` (`patternToLang`, language rows around lines 780–790). Label each finding with the project's signature-detected language. Fall back to `unknown`, not the last language that registered the pattern. Add table tests for a Next.js repo, a plain Node repo, and a Go repo with `bin/`.
- [ ] **P1-4 (F8) Deduplicate paths.** In `bilge-macos`, resolve each target path (`EvalSymlinks` + `Clean`) and drop exact duplicates within a target. Across targets, count a nested path only once, under the more specific target, and exclude it from the broader one (e.g. `home-cache` excludes `~/.cache/puppeteer` and `~/.cache/uv`). Tests: duplicate brew paths count once; home-cache total excludes paths owned by other targets.
- [ ] **P1-5 (F10) Safety check in dry run.** Run the Git-tracked/not-in-repo check during dev-cache scan and mark findings `skip_reason`. Totals exclude skipped findings. Test: tracked `bin/` shows as skipped in dry run with the same reason clean prints.
- [ ] **P1-6 (F14) JSON hygiene.** With `--json`, stderr carries progress/log lines only, never the report (`cmd/bilge/main.go` `liveOutput`). Emit every `when` as RFC 3339 with offset from one helper. Populate `commands` in scan JSON with the exact commands clean would run. Tests: stderr contains no `{` report; `commands` non-empty for a target with cmds.
- [ ] **P1-7 (F16) Small UX.** Hide 0 B rows in the dev-cache table unless `--all`; support `bilge help <task>`. Tests for both.

### Phase 2 — Approval-required (needs the decision noted)
- [ ] **P2-1 (F2, D1) Config model.** Implement D1, including migration of existing full configs and a scan-time warning while a config still overrides a default with a destructive command. Acceptance: an old config with `--volumes` in docker produces a warning on scan and the migrated config keeps volumes.
- [ ] **P2-2 (F3, D2) Docker safety.** Show active context name and endpoint in scan output and JSON. Apply D2 for non-local endpoints. Detect OrbStack (and Colima) data dirs for measurement. After pruning, report that VM space returns to the host later. Volume pruning only behind an explicit flag, with a per-volume list in dry run.
- [ ] **P2-3 (F4) Android.** Remove `sdkmanager --update`. Make the target informational: list AVDs and system images with sizes, and print `avdmanager delete avd -n <name>` hints.
- [ ] **P2-4 (F5, D3) Brew autoremove.** Implement D3.
- [ ] **P2-5 (F6, D4) Exit codes.** Implement D4 across `bilge`, `dev-cache`, `git-cleaner`, `bilge-macos`. Document the codes in `README.md` and `docs/scheduling.md`. Put skips and errors in separate JSON fields.
- [ ] **P2-6 (F7, D5) Cleanable vs measured.** Add `action` (`command` | `delete-paths` | `informational`) and `cleanable_bytes` to each target finding. Totals and the MCP summary report cleanable bytes, with informational shown separately. If D5 = yes, add `delete-paths` for Gradle caches (after `gradle --stop`), Xcode DerivedData, Terraform plugin cache and `~/.npm/_npx`. For git-cleaner, estimate `gc` savings from `git count-objects -v` instead of total `.git` size.
- [ ] **P2-7 (F9) Freed space.** Report filesystem free-space delta (`statfs` before/after) next to per-path deltas in clean output and JSON.
- [ ] **P2-8 (F11, D6) Repo discovery.** Implement D6. Test with a `root/<org>/<repo>/node_modules` fixture.

### Phase 3 — Performance and coverage
- [ ] **P3-1 (F15) Speed.** Measure scan time per target and scan targets concurrently (bounded worker pool). Send MCP `notifications/progress` when the request carries a `progressToken`. Goal: full scan under 60s on the field-test machine profile.
- [ ] **P3-2 (F16) MCP granularity.** Let `bilge_clean` take sub-actions per target (e.g. `docker:build-cache`, `docker:images`).
- [ ] **P3-3 (F16) Task naming.** One canonical name for the OS cleaner task. Keep `mac-cache-cleaner`/`system` as aliases with a deprecation warning.
- [ ] **P3-4 (F17) New informational targets.** `pre-commit` (`pre-commit gc`), `act`, rustup and pyenv "unused versions" listings.
- [ ] **P3-5** Open an issue in `everydaydevopsio/homebrew-bilge-pump` for the `postflight` deprecation.

## Risks and Tradeoffs
- P1-1 widens what the MCP server accepts. Keep strictness on `arguments` so typos in tool inputs still fail loudly.
- P1-4 and P2-6 change reported totals sharply (104 GB to a much smaller cleanable figure). Call this out in release notes so users don't think cleanup got worse.
- P2-1 rewrites user config files. Back up to `config.yaml.bak` before migration and never migrate during a clean run.
- P2-6 with D5 adds Bilge-owned deletion. Restrict it to fixed cache dirs, never follow symlinks out of them, and dry-run first.
- P2-5 changes exit codes, which breaks scripts that check for 3. Call it out as a breaking change and bump the minor version.

## Test Strategy
- Unit: every item above names its failing test. Table tests per `AGENTS.md`.
- Integration: MCP stdio test harness sending Claude Code-shaped requests (with `_meta` and `progressToken`) for all tools.
- Failure paths: stale config with destructive commands; non-local Docker context; old pip; Git-tracked cache dirs; unreadable dirs.
- Manual: rerun the field-test session log steps 1–9 on a Mac and compare against the report.
- Requirement-to-test mapping: each PR lists finding ID → test name in its description.

## Rollback Strategy
- Trigger: a release that deletes anything outside its documented paths, breaks MCP, or misreports a destructive command as safe.
- Rollback steps: revert the offending PR and publish a patch release; for P2-1, restore `config.yaml.bak`.
- Validation after rollback: MCP stdio harness passes; `bilge scan` dry run makes no changes (checksum of a fixture tree before/after).

## Outcome
- Result: Phase 1 PRs open, CI green, awaiting review/merge. Phase 2 decisions D1–D6 recorded above.
- Evidence links/commands: P1-1 #96 (issue #95), P1-2 #98 (#97), P1-3 #99, P1-4 #101 (#100), P1-5 #102, P1-6 #103, P1-7 #104. New issues for Phase 2 findings: F2 #105, F5 #106, F9 #107. Field-test evidence commented on #37, #54, #55, #63, #64, #65, #80.
- PRD updates: each Phase 1 PR updates its governing PRD section.
- Notes: Copilot reviewer could not be requested (not enabled on the repo). Pre-existing coverage below the 60% minimum on `main`: git-cleaner 41.7%, bilge-macos 59.8% (raised to 60–63% by #98/#101/#103).
