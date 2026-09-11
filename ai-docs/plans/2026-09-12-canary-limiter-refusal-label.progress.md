# Progress: Canary limiter refusal label — ACTIVE
_Updated: 2026-09-11 23:14_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-canary-limiter-refusal-label
**base_commit:** c6b3ce89c401763dbcb102c51791094761ebb2d0
**Last build:** PASS

**Issue:** #89
**Spec:** ai-docs/plans/2026-09-12-canary-limiter-refusal-label.spec.md

**current_step:** Step 9.5 — docs updated
**last_passed_gate:** make verify (fmt-check, build, vet, lint, file-limits, test, test-race, tidy-check, actionlint, shellcheck, comment-refs, import-guard) | 2026-09-11T23:12Z | c8b4e385ae632d1ac7d46c18b043e2d26fdf4e20
**entry_args:** 89

## Next action

**Do this immediately:** Group A is complete (both subtasks committed). Proceed to Step 9 (Verify) / Step 10 (self-review) per `/task`.

## Subtasks

- [x] 1. `internal/tg/caller.go` + `caller_test.go` — read `now` per loop pass beside the `ctx.Deadline()` read, add the pure refusal-cause helper (deadline form carries D4's message wrapping `context.DeadlineExceeded`), call it from the `!ok` branch; tests first — commit 5b08da6
- [x] 2. `internal/health/probe_test.go` — the canary-label test: an already-passed deadline is counted a failure with `reason` `timeout`, fake handler never reached — commit 9dbd45e

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review returned GO on round 1; all four `## Issues` rows and both recommendations were design-internal, so they were folded by `design-writer` and design-review did not run again.
- **Step 8 (subtask 1)**: the helper's doc comment and the two new end-to-end test doc comments initially named "AC1"/"AC3"/"D4"/"KD-26" as their rationale; `make comment-refs` flagged all four as forbidden ac-id/decision-anchor references (DOC-4), so the comments were reworded to describe the property directly instead of citing the spec/design anchor. Re-ran the gate clean afterward.
- **Step 8 (subtask 2)**: verified the new `TestTelegramProber_ExpiredDeadlineCountsAsTimeout` test's discriminating power directly — checked out `internal/tg/caller.go` at the pre-subtask-1 commit (HEAD~1) with the new test still in place, ran it, and it failed with `classifyFailure = "network"` as expected; restored the post-subtask-1 file (`git diff` confirmed byte-identical) before continuing.
- **Step 9**: panic-index needs no row — the only changed production file carries no `panic(` / `log.Fatal*` / `Must…` site, and both scan patterns were confirmed against a constructed control before the clean result was accepted.
- **Step 9**: domain-invariant sweep — sweeps 1, 2, 3 and 5 clean (each pattern confirmed against a constructed control first); sweep 4 hits `time.Now()` in the transport caller and its tests. Legitimate: the determinism rule binds world generation, combat and PvP-trail replay, and the Bot API transport is none of them — it is wall-clock by nature (rate-limit windows, retry backoff, latency observation). The one new read, `caller.go:69`, is the per-pass hoist the design mandates so the limiter's decision and the caller's classification read one instant.
- **Step 9**: the per-AC sweep ran three mutants rather than trusting the green suite — a non-strict predicate (`!now.Before`), the already-passed condition dropped entirely, and the call site reverted to the pre-change cause. Each was confirmed to BUILD first; each turned the expected test RED with a failure line naming the asserted property. `internal/tg/caller.go` was restored from a cp-backup and `git status` re-checked clean afterwards.
- **Step 9.5**: `context.md` left untouched — no block's high-level state moved (no new package, no new capability; the change refines which cause one existing branch carries) and its open-question line points at `docs/DESIGN.md` §16, none of whose entries this answers. `alert-contract.md` also left untouched: its `reason` sentence enumerates a status code else `timeout` / `canceled` / `network`, and option 1 adds no value to that set, so the sentence stays true. No repo-root user-facing doc exists to contradict.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 1 | "D4 makes the log line the operator's discriminator between \"deadline breached during the HTTP wait\" and \"refusal decided past the deadline\", but the design never says what the deadline form's message *is*." | design-internal | folded | design § Key decisions D4 (message named) + § Decomposition subtask 1 @ c6b3ce8 |
| G2 | 1 | "\"`time.Now()` moves out of the `acquire` argument list into a `now` variable\" does not say where the variable lives." | design-internal | folded | design § The chosen shape (per-pass read, inside the loop) + § Decomposition subtask 1 @ c6b3ce8 |
| G3 | 1 | "§ Test Design opens \"All claims below are about tests that do not exist yet\" — a certification over the section's own tags" | design-internal | folded | design § Test Design — sentence deleted @ c6b3ce8 |
| G4 | 1 | "D2's strict `now.After(deadline)` deliberately disagrees with `context`'s own boundary" | design-internal | folded | design § Key decisions D2 — the `dur <= 0` clause @ c6b3ce8 |
| G5 | 1 | "Consider having the AC1 test also assert the cause's message differs from the wait-does-not-fit form" | design-internal | folded | design § Test Design items 1 and 2 @ c6b3ce8 |
| G6 | 1 | "`limit.go`'s `acquire` doc comment still describes the refusal solely as \"the required wait would end after deadline\"" | design-internal | folded | design § What this change does not touch @ c6b3ce8 |

## Key discoveries (don't re-investigate)

- The canary's prober builds its client with no `Gate`, so the gate-refusal path into `network` is unreachable for a canary leg — it is out of this task's reach.
- `internal/health` needs no production edit: `classifyFailure` already maps a `context.DeadlineExceeded` chain to `timeout`, so AC2 follows from AC1 and is discharged by a test.
- The only production sentinel consumers are the canary classifier and `internal/ingest/loop.go`, which tests `context.Canceled` only and cannot change behaviour.

## AC Status

| AC | Status | Verifying command (orchestrator's own, Step 9) | Mutant that turned it RED |
|----|--------|-----------------------------------------------|---------------------------|
| AC1 | PASS — `TestCaller_ExpiredDeadlineAtLimiterIsContextError`, `internal/tg/caller_test.go` | `go test ./internal/tg -run TestCaller_ExpiredDeadlineAtLimiterIsContextError -count=1` | call site reverted to the pre-change cause → `errors.Is(err, context.DeadlineExceeded) = false, want true` |
| AC2 | PASS — `TestTelegramProber_ExpiredDeadlineCountsAsTimeout`, `internal/health/probe_test.go` | `go test ./internal/health -run TestTelegramProber_ExpiredDeadlineCountsAsTimeout -count=1` | same mutant → `classifyFailure = "network", want "timeout"` |
| AC3 | PASS — `TestCaller_RealWaitPastDeadlineIsNotContextError`, `internal/tg/caller_test.go` | `go test ./internal/tg -run TestCaller_RealWaitPastDeadlineIsNotContextError -count=1` | already-passed condition dropped (`if hasDeadline {`) → `errors.Is(err, context.DeadlineExceeded) = true, want false` |
| — | boundary + message, `TestLimiterRefusalCause` | `go test ./internal/tg -run TestLimiterRefusalCause -count=1` | predicate relaxed to `!now.Before(deadline)` → the `now_equal_to_deadline` row fails on both the chain and the wording |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| — | — | — | — | — |

## Files touched

- `internal/tg/caller.go` — `now` read per loop pass; new `limiterRefusalCause` helper
- `internal/tg/caller_test.go` — `TestLimiterRefusalCause`, `TestCaller_ExpiredDeadlineAtLimiterIsContextError`, `TestCaller_RealWaitPastDeadlineIsNotContextError`
- `internal/health/probe_test.go` — `TestTelegramProber_ExpiredDeadlineCountsAsTimeout`
