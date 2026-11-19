# Port dev-cache filters to the Bilge codebase

## Scope
- Update PR #73 with the current `main` branch.
- Port include/exclude path filters and min/max age filters to current `dev-cache`.
- Update the governing `PRD.md` requirements before implementation.
- Resolve verified review findings and failing CI, then validate and request review.

## Execution checklist
- [x] Merge latest `origin/main` into the PR branch and resolve conflicts without losing current safety behavior.
- [x] Update `PRD.md` with filter precedence, age handling, and cleanup behavior.
- [x] Add failing tests for review findings, confirm they fail, then implement the fixes.
- [x] Run formatting, targeted tests, full repository tests, vet, and coverage checks.
- [x] Run Bosun against the exact current PR base and address valid findings.
- [x] Push the branch and verify all GitHub Actions and review status.

## Test strategy
- Unit tests for include/exclude matching, duration parsing, age boundaries, and unknown modification times.
- Integration tests for filtered scan and cleanup paths.
- Full repository tests and coverage checks required by repository guidance.

## Rollback strategy
- Revert the filter commit if the behavior or CI is incorrect; retain the main-merge commit needed to keep the PR current.

## Outcome
- Result: Current-main merge integrated; filter precedence, relative/nested paths, include glob traversal, age-filter traversal/unknown times, excluded descendants, strict duration parsing, unified CLI filter forwarding, cache-named ancestors, partial inspections, empty CLI overrides, malformed globs, and zero age bounds addressed.
- Evidence: regression tests failed before fixes and passed afterward; full `make test` and `make vet` pass, and `dev-cache` coverage is 66.3%. The second and final Bosun review round reviewed `b363ce9`; its malformed-glob and zero-age findings were fixed in `36be7f4`. All seven GitHub Actions pass on `36be7f4`. No third review round was started, honoring the two-round limit.
