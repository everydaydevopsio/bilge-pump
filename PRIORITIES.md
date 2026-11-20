# Fix priorities

Prioritized from the current code review and open GitHub issues. Do these in order; defer unattended cleanup and MCP integration until the safety items are resolved.

## P0 — Prevent cleanup from deleting user data or making disk pressure worse

- [x] Update repository references in release tooling: GoReleaser, the installer, Codecov, and the generated cask now use `everydaydevopsio/bilge-pump` and `everydaydevopsio/homebrew-bilge-pump`.
- [ ] Make `dev-cache --clean` refuse a match containing Git-tracked files by default. Cover checked-in `vendor/`, `bin/`, and similar generic directory names. Print the exact skipped path and reason. See [issue #80](https://github.com/everydaydevopsio/bilge-pump/issues/80).
- [ ] Remove `sdkmanager --update` from cleanup actions. Change the Android target to report accurate cache usage and only offer safe, explicit removal actions. See [issue #80](https://github.com/everydaydevopsio/bilge-pump/issues/80).
- [ ] Do not prune Docker volumes by default. Require a separate explicit opt-in for named/unused volume removal, and preview the affected data before execution. See [issues #55 and #80](https://github.com/everydaydevopsio/bilge-pump/issues/55).
- [ ] Replace the macOS cleaner's blanket `y` input with per-action confirmation or documented non-interactive flags. Do not let `--clean` silently approve every command prompt.

## P1 — Make scan results and cleanup actions accurate

- [ ] Fix or disable targets that currently fail or misrepresent cleanup: nvm shell-function detection, pnpm's active store path, Maven's project-context command, Ollama's non-cleaning `list` command, and Docker-compatible runtimes such as OrbStack. See [issue #80](https://github.com/everydaydevopsio/bilge-pump/issues/80).
- [ ] Distinguish measured usage from reclaimable space. Mark report-only targets clearly; implement safe cleanup for valuable targets such as Gradle, Terraform, Xcode DerivedData, and simulator caches only after their actions are validated. See [issues #37 and #80](https://github.com/everydaydevopsio/bilge-pump/issues/37).
- [ ] Ensure `--json` writes only valid JSON to stdout; send progress and warnings to stderr. Standardize exit codes and expose per-target command failures in the result. See [issues #63, #65, and #80](https://github.com/everydaydevopsio/bilge-pump/issues/63).
- [ ] Add age and project include/exclude controls. Keep broad patterns such as `bin`, `vendor`, `build`, and `target` constrained by project/language context. See [issues #45, #46, #47, and #80](https://github.com/everydaydevopsio/bilge-pump/issues/46).

## P2 — Deliver a coherent cross-platform CLI

- [ ] Establish one user-facing command with subcommands for scanning project caches, developer-tool caches, and Git repository maintenance. Reuse shared scanning, reporting, and policy code instead of making a wrapper shell out to loosely coupled binaries. See [issue #62](https://github.com/everydaydevopsio/bilge-pump/issues/62).
- [ ] Define supported targets separately for macOS, Linux, native Windows, and WSL; add native Windows paths and release artifacts or remove unsupported claims. Align installer, CI, GoReleaser, README, and Homebrew details. See [issues #19, #59, #60, and #61](https://github.com/everydaydevopsio/bilge-pump/issues/19).
- [ ] Add a largest-items report, including large items that the tool must not clean, so users can understand disk use beyond known caches. The real-world run in [issue #80](https://github.com/everydaydevopsio/bilge-pump/issues/80) found substantial space the current targets did not address.

## P3 — Automate only after policy and reporting are trustworthy

- [ ] Add free-space thresholds and an opt-in scheduled check. Start with notify/preview behavior; allow automatic cleanup only for actions explicitly classified as safe by the user. Cron/launchd documentation in [issue #69](https://github.com/everydaydevopsio/bilge-pump/issues/69) is an interim manual-scheduling step, not a low-disk monitor.
- [ ] Add a stdio MCP adapter over the shared scan and cleanup APIs. Expose scan and preview first; require explicit user approval for cleanup actions. MCP does not replace a local monitor because it runs only when its client starts it.

## P4 — Improve discoverability and maintainability

- [ ] Validate config schemas and versions with actionable errors, and document defaults consistently.
- [ ] Add consistent CLI examples and help text across commands. See [issue #68](https://github.com/everydaydevopsio/bilge-pump/issues/68).
- [ ] Review parallel scanning and other performance improvements after correctness and safety are covered. See [issue #64](https://github.com/everydaydevopsio/bilge-pump/issues/64).
