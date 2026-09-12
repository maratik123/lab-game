# Progress: Scheduler panic recovery and deadline reclaim — ACTIVE
_Updated: 2026-09-12 18:05_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-scheduler-panic-recovery-deadline-reclaim
**base_commit:** c7c4018
**Last build:** not run

**Issue:** #81
**Spec:** ai-docs/plans/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md

**current_step:** Step 8 — Group A subtask 2 of 9 complete
**last_passed_gate:** go build ./... + go test ./internal/scheduler/... + golangci-lint run ./internal/scheduler/... + go vet | subtask 2
**entry_args:** 81

## Next action

**Do this immediately:** hand off into Group A per the design's `## Handoff plan` — spawn `code-writer` for subtasks 1–9 (`internal/panicguard`, `internal/scheduler`, `internal/ingest`, `internal/health`, `cmd/bot`, `internal/gateguard`), gating and committing per subtask.

## Subtasks

Group A — code (`code-writer`, `sonnet`/`medium`):

- [x] 1. `internal/panicguard`: the shared recovered-panic type, its constructor from a `recover()` result, its `error` rendering, package comment, tests, leak-check `TestMain`
- [x] 2. `scheduler.Options` gains `Logger`; `Worker` carries it; nil → discard handler; correct `RunOnce`'s "no logger" clause
- [ ] 3. `FailureKind` gains the panic member; `FailureRolledBack`'s doc excludes a panic; correct `LoopObservation.Err`'s "no logger" clause
- [ ] 4. Handler-boundary recovery and its settlement; logger threaded into `runHandlerWithSavepoint` as a parameter; unusable transaction routed into the pending-settlement set
- [ ] 5. Deadline reclaim: PID read before the handler goroutine launches; one-argument terminate; two non-success shapes reported apart; `TestDeadline_ctxIgnoringHandler_negativeCase` assertion reversed; falsified comments corrected
- [ ] 6. Update ingestion: `Options` gains `Logger`; the recovery helper returns the shared value; stack into the give-up row and the log; reported outcome unchanged
- [ ] 7. Health: failure-label mapper gains the panic case; label test, observer test and closed-set guard take the new value
- [ ] 8. Composition root: thread the process logger into both constructors
- [ ] 9. Goroutine-ownership allow list: the handler launch's `panicTo` and `stops`, and the watchdog launch's `stops`

Group B — instructions (`general-purpose`, `inherit`):

- [ ] 10. Documentation propagation (AC5): failure-value enumeration, ownership rule 4's clause, the detached-launch paragraph, the `internal/` layout enumeration

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 raised a spec-amending note (AC6/AC9 unsatisfiable on both surfaces); the owner chose option (1), amend the spec — recorded verbatim as `answer 3.1` in the interview state file.
- **Step 7**: `spec-writer` amended four rows rather than the two named; the owner confirmed the four-row reach — recorded verbatim as `answer 3.2`.
- **Step 7**: round 3 returned ITERATE with the cap exhausted; the owner raised the design-review round cap to **5 (was 3)**. No orchestrator-invented bypass was used.
- **Step 7**: design round 4 reversed the terminate mechanism from the waiting two-argument form back to the one-argument signalling form, on the measurement that the wait accelerates nothing about the row and costs a structural ~100 ms inside `executeOne`.
- **Step 7**: GO at design-review round 4 (4 of a cap of 5). All five notes and three recommendations are design-internal; folded, and design-review did not run again.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 4 | `D5's debug-level branch argues "the two clocks race on the same value and the server's own layer routinely terminates the backend first"` | design-internal | folded | design § Key decisions → D5's terminate-failure bullet, and § Risks' terminate-failure bullet @ c7c4018 |
| G2 | 4 | `§ Approach and § Risks both report "row claimable at min=… med=… max=…" as what the row does after the signalling terminate` | design-internal | folded | design § Approach preamble + bullets 2 and 3, § Risks residue bullet, D5, D5a, § Test Design @ c7c4018 |
| G3 | 4 | `§ Approach opens the measurement bullets with "Every premise below was measured against docker.io/library/postgres:18 through this module's own pgx version"` | design-internal | folded | design § Approach preamble deleted; D5b's negative rewritten @ c7c4018 |
| G4 | 4 | `D11's "the terminate's four falsified comments" and subtask 9's "three answers narrow, not one" state cardinalities of code sites` | design-internal | folded | design D11 and § Decomposition subtask 9; enumerations kept, numerals struck @ c7c4018 |
| G5 | 4 | `D5 fixes the terminate's bound as "a named constant, placeholder 5s"` | design-internal | folded | design D5 — decided as `5s` on the watchdog close bound's precedent @ c7c4018 |
| G6 | 4 | `The reversal itself is sound and now independently confirmed` | design-internal | folded | no design change required; the round-4 reversal stands as written @ c7c4018 |
| G7 | 4 | `D5a's hot-spin caveat is worth keeping verbatim in the implementor's hands` | design-internal | folded | design D5a and § Decomposition subtask 5 both keep the caveat verbatim @ c7c4018 |
| G8 | 4 | `Round-trip required: before Step 8, update the design doc to incorporate each note/recommendation above` | design-internal | folded | discharged by the fold-in round itself; each row above verified absent/present in the design @ c7c4018 |

## Key discoveries (don't re-investigate)

- **The waiting `pg_terminate_backend(pid, timeout)` form was evaluated and rejected.** It returns on a ~100 ms structural floor (the server checks the victim at the top of each wait step), it frees the row no sooner than the one-argument form, and it collapses "pid already gone" and "did not die in time" into one bare `false`. Do not reintroduce it.
- **`err == nil` is not success for a terminate.** The one-argument form returns `false` with a nil Go error on an already-dead pid, and a Go error carrying `SQLSTATE 42501` on permission denial. The branch must read the boolean.
- **`internal/ingest` writes a row only on give-up**, carrying the *last* attempt's error — so a recovered panic on an earlier attempt persists no row. This is why the amended AC6/AC9 require the log record always and the row only where the attempt leaves one.
- **`TestDeadline_ctxIgnoringHandler_negativeCase` currently asserts the opposite of AC7** and its fixture blocks in the database, so the terminate reverses it. The reversal is designed, not a surprise; the test must be seen red against the pre-change tree.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `internal/panicguard/panicguard.go` (new)
- `internal/panicguard/panicguard_test.go` (new)
- `internal/panicguard/main_test.go` (new)
- `internal/scheduler/worker.go` (Options.Logger, discard-handler substitution, RunOnce doc fix)
- `internal/scheduler/worker_test.go` (Logger nil/given coverage)
