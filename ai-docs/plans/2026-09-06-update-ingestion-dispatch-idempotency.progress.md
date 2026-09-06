# Progress: Update ingestion — long polling, dispatch, operation idempotency, chat allowlist — ACTIVE
_Updated: 2026-09-06 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-06-update-ingestion-dispatch-idempotency
**base_commit:** a09d26f56df4b72ab15c49e328b007b900668ca4
**Last build:** not run
**Issue:** #22
**Spec:** ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md
**current_step:** Step 8 — Implementation start
**last_passed_gate:** design-review GO (round 5) | 2026-09-06T22:59:07Z | a09d26f56df4b72ab15c49e328b007b900668ca4
**entry_args:** 22

## Next action

**Do this immediately:** start Group A at subtask 1.

**Design:** `ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.design.md` — read its `## Handoff plan` before touching subtask 2. That section carries subtask 2's binding contract in three parts, and the order is not negotiable: (a) land the literal one-based ramp in `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go` and see it GREEN against the still-shipped `backoff`, (b) run the mutation probe — `backoff(k+1, …)` over a `cp` backup of `settle.go`, which must turn the two named tests RED, and **a green probe is a STOP: return to the orchestrator rather than proceeding**, (c) only then delete the local ramps and re-point. Restore from the `cp` backup, never with `git checkout -- <file>`. `go test ./internal/tg/ ./internal/scheduler/` must be green as its own step before the group returns, and `go test ./internal/store/` likewise after subtask 5.

## Subtasks

- [ ] 1. `internal/backoff`: `Exponential` + `EqualJitter`, table and monotonicity tests  ← CURRENT (Group A)
- [ ] 2. Adopt `internal/backoff` in `internal/tg` and `internal/scheduler` — call-site gate first, then mutation probe, then re-point (Group A)
- [ ] 3. `config.Ingest` + the `LAB_GAME_INGEST_` reader and both `Load` cross-checks (Group A)
- [ ] 4. Migration `00004_ingest.sql`: `ingest_offset`, `ingest_dead_update`; extend the exact base-table assertion (Group A)
- [ ] 5. `store.Queryer` + `store.PlayerExists`; delete and re-point `queryRower` (Group A)
- [ ] 6. `internal/ingest` core types: `Kind`, `IDSpace`, the `operation_id` builder, `Handler`/`Update`, `Router` (Group A)
- [ ] 7. Offset and give-up storage; the package's `TestMain` lands here (Group B)
- [ ] 8. The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer` (Group B)
- [ ] 9. The loop: `Options`/`New`, requested kinds, `PollOnce`, `Run` with D19's poll-error policy (Group B)
- [ ] 10. The gate: `PlayerLookup`, pool-backed lookup, `Gate`/`NewGate`, the positive-only cache (Group B)
- [ ] 11. Guard tests and the structural source walks (Group B)
- [ ] 12. Propagation: `context.md`, `domain-invariants.md` (Group C)

## Decisions log

- **Step 7**: design-review reached GO on round 5; the owner raised the round cap to 5 (was 3) after round 3, and every round found new material rather than re-opening an earlier one.
- **Step 7**: the four round-5 GO notes were folded into the design before Step 8, per Step 8's first-action rule; none was spec-amending, so no Spec Amendment recipe ran.

## Key discoveries (don't re-investigate)

- The shipped scheduler suite is **invariant** to D2's one-based→zero-based call-site translation, not blind to it: `failure_test.go:154` and `deadline_test.go:252` both compute `want :=` through the same function `settle.go:142,242` calls. The literal-ramp gate plus the mutation probe are the only instruments that separate a correct translation from an incorrect one.
- The mutation probe must be written as `backoff(k+1, …)` in the shipped ramp's own units: while `func backoff` still stands, no file of `internal/scheduler` can import `internal/backoff` — the name collision is package-wide, not per-file.
- `encoding/json` erases an **empty** slice under `omitempty`, not only a nil one; the same tag sits on `GetUpdatesParams.Offset`, so a stored offset of `0` is legitimately absent from the request body.
- A handler must not issue an outbound call whose permission rests on a row its own uncommitted transaction created (D18) — a Telegram send is not rollback-able, so this holds even if the gate were made transaction-aware.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC38 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| (none yet — self-review runs at Step 10) | | | | |

## Files touched

- (none yet)
