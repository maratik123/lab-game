# Progress: Goroutine-leak detection in the test suite: adopt goleak — ACTIVE
_Updated: 2026-09-11 16:33 UTC_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-11-goroutine-leak-detection-goleak
**base_commit:** 364657b8617cd8b4558f5f685fc14fe66437b8a5
**Last build:** PASS

**Issue:** #74
**Spec:** ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md
**Design:** ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.design.md

**current_step:** Step 8 — subtask 1 of 6 complete
**last_passed_gate:** golangci-lint run ./internal/tgtest/... ./internal/tg/... | 20037f7
**entry_args:** 74

## Next action

**Do this immediately:** Group A — subtasks 1–5 of the design's `## Decomposition`, in the dependency order its `## Handoff plan` gives (1 → 2 → 3 → 4 → 5), one commit per subtask. Subtask 4's rollout STOP applies as the design states it: a leak any route reports that is not the D8 class goes back to the orchestrator, not fixed inside the subtask. Group B (subtask 6, docs) follows after Group A returns.

## Subtasks

Group A — code (`code-writer`):

- [x] 1. `internal/tgtest` — D8 fix, its tests (T1), and the `internal/tg/retry_test.go` doc-comment rewrite (commit 20037f7)
- [ ] 2. `internal/leaktest` — `Main`, `Ignore`, the check, T2's unit tests, the package's own `TestMain`  ← CURRENT
- [ ] 3. `internal/testdb` — D9: `callersOfMain` as a parse, T3's scratch case
- [ ] 4. Rollout — every package with tests declares `TestMain` in the D2 form; the whole suite on every route
- [ ] 5. The D7 guard — `internal/leaktest/guard_test.go` (T5)

Group B — instructions (`general-purpose`, inherited model):

- [ ] 6. Docs — `ai-docs/go-test-conventions.md` section, one `AGENTS.md` bullet, KD-36 in `ai-docs/key-decisions.md`

## Decisions log

- **Steps 1–5**: `/interview` ran three spec-writer rounds; the owner's answers are `prior_qa` rounds 1–3 in the spec's state file; spec approved in round 3 and the cross-link comment posted on #74.
- **Step 7**: design-review round 1 GO with a SPEC-REMIT against spec Scope 8; surfaced to the owner, who struck it (`prior_qa` round 4); spec-writer amended the spec at 779b08b; design round 2 reconciled the design and folded in round 1's design-internal notes.
- **Step 7**: design-review round 2 GO with design-internal notes only; folded in at 2fe8592 and re-reviewed as round 3, the orchestrator having judged the fold-in non-trivial; the harness's conflicting rules on that point are recorded in `ai-docs/harness-gaps.md` (2026-09-11 entry).
- **Step 7**: design-review round 3 GO, the 3-round cap reached; its notes folded in at 364657b. The owner exempted this fold-in from re-review, on the condition that the orchestrator checks each note in the design diff; the orchestrator read the diff 2fe8592..364657b — the race-tag guard pass with its T5 case, T2's wiring case through `Main`, the two corrected measurement tags, and the CI-runtime Risks row with T4's STOP extended to the first CI run are all present, and no reviewer suggestion was rejected or resolved by another route.
- **Subtask 1** (commit 20037f7): `tgtest.New` now gives its `*http.Server` a base context of its own, cancelled by `tb.Cleanup` after the server closes; `Delayed` waits on its duration or on the request's context, whichever comes first. T1's three cases (cleanup, ordering, synctest bubble) went into `internal/tgtest/tgtest_test.go`; `internal/tg/retry_test.go`'s `TestCaller_AttemptTimeoutAbandonsAttempt` doc comment lost its now-false real-time rationale, body unchanged. The design's two red demonstrations (today's `Delayed`, and the half-fix cancelling nothing) were exercised by hand against cp-backups during authoring, not committed. No decision-anchor token (`D8`, `KD-26`) survived into the committed comments — `make comment-refs`' equivalent check (the pre-commit hook's decision-anchor scan) caught two on the first attempt and both were rewritten to describe behaviour instead. Gates run and green: `go build ./...`, `go test ./internal/tgtest/...`, `go test -race ./internal/tgtest/...`, `golangci-lint fmt -d` (clean), `golangci-lint run ./internal/tgtest/... ./internal/tg/...`.

## Key discoveries (don't re-investigate)

- The design's probes (recipes in the design) record one leak class in today's tree: a handler made by `internal/tgtest`'s `Delayed` still sleeping after its test ends; D8 is its fix. Any other leak class reported during the rollout is subtask 4's STOP.
- Design-review round 3 reports reproducing that `-race` excludes a `//go:build !race` file which `build.Default` compiles — the reason the D7 guard classifies each directory under two configurations.

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

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `internal/tgtest/tgtest.go` — subtask 1
- `internal/tgtest/tgtest_test.go` — subtask 1
- `internal/tg/retry_test.go` — subtask 1
