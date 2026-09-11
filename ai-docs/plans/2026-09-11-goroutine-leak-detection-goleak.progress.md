# Progress: Goroutine-leak detection in the test suite: adopt goleak — ACTIVE
_Updated: 2026-09-11 16:33 UTC_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-11-goroutine-leak-detection-goleak
**base_commit:** 364657b8617cd8b4558f5f685fc14fe66437b8a5
**Last build:** PASS

**Issue:** #74
**Spec:** ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md
**Design:** ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.design.md

**current_step:** Step 8 — subtask 4 of 6 complete
**last_passed_gate:** golangci-lint run (whole tree) | 763f0d9
**entry_args:** 74

## Next action

**Do this immediately:** Group A — subtasks 1–5 of the design's `## Decomposition`, in the dependency order its `## Handoff plan` gives (1 → 2 → 3 → 4 → 5), one commit per subtask. Subtask 4's rollout STOP applies as the design states it: a leak any route reports that is not the D8 class goes back to the orchestrator, not fixed inside the subtask. Group B (subtask 6, docs) follows after Group A returns.

## Subtasks

Group A — code (`code-writer`):

- [x] 1. `internal/tgtest` — D8 fix, its tests (T1), and the `internal/tg/retry_test.go` doc-comment rewrite (commit 20037f7)
- [x] 2. `internal/leaktest` — `Main`, `Ignore`, the check, T2's unit tests, the package's own `TestMain`
- [x] 3. `internal/testdb` — D9: `callersOfMain` as a parse, T3's scratch case
- [x] 4. Rollout — every package with tests declares `TestMain` in the D2 form; the whole suite on every route
- [ ] 5. The D7 guard — `internal/leaktest/guard_test.go` (T5)  ← CURRENT

Group B — instructions (`general-purpose`, inherited model):

- [ ] 6. Docs — `ai-docs/go-test-conventions.md` section, one `AGENTS.md` bullet, KD-36 in `ai-docs/key-decisions.md`

## Decisions log

- **Steps 1–5**: `/interview` ran three spec-writer rounds; the owner's answers are `prior_qa` rounds 1–3 in the spec's state file; spec approved in round 3 and the cross-link comment posted on #74.
- **Step 7**: design-review round 1 GO with a SPEC-REMIT against spec Scope 8; surfaced to the owner, who struck it (`prior_qa` round 4); spec-writer amended the spec at 779b08b; design round 2 reconciled the design and folded in round 1's design-internal notes.
- **Step 7**: design-review round 2 GO with design-internal notes only; folded in at 2fe8592 and re-reviewed as round 3, the orchestrator having judged the fold-in non-trivial; the harness's conflicting rules on that point are recorded in `ai-docs/harness-gaps.md` (2026-09-11 entry).
- **Step 7**: design-review round 3 GO, the 3-round cap reached; its notes folded in at 364657b. The owner exempted this fold-in from re-review, on the condition that the orchestrator checks each note in the design diff; the orchestrator read the diff 2fe8592..364657b — the race-tag guard pass with its T5 case, T2's wiring case through `Main`, the two corrected measurement tags, and the CI-runtime Risks row with T4's STOP extended to the first CI run are all present, and no reviewer suggestion was rejected or resolved by another route.
- **Subtask 1** (commit 20037f7): `tgtest.New` now gives its `*http.Server` a base context of its own, cancelled by `tb.Cleanup` after the server closes; `Delayed` waits on its duration or on the request's context, whichever comes first. T1's three cases (cleanup, ordering, synctest bubble) went into `internal/tgtest/tgtest_test.go`; `internal/tg/retry_test.go`'s `TestCaller_AttemptTimeoutAbandonsAttempt` doc comment lost its now-false real-time rationale, body unchanged. The design's two red demonstrations (today's `Delayed`, and the half-fix cancelling nothing) were exercised by hand against cp-backups during authoring, not committed. No decision-anchor token (`D8`, `KD-26`) survived into the committed comments — `make comment-refs`' equivalent check (the pre-commit hook's decision-anchor scan) caught two on the first attempt and both were rewritten to describe behaviour instead. Gates run and green: `go build ./...`, `go test ./internal/tgtest/...`, `go test -race ./internal/tgtest/...`, `golangci-lint fmt -d` (clean), `golangci-lint run ./internal/tgtest/... ./internal/tg/...`.
- **Subtask 2**: `internal/leaktest/leaktest.go` implements `Ignore`, `Main` and the unexported `check` behind it — refusals (nil runner, blank Reason, blank Function, a Function naming this module's own code, an undeterminable module path), the clean check via `goleak.Find`, the filtered-run skip, and the per-entry evaluation (accumulating every not-needed or module-code-excusing entry into one report rather than returning on the first, so two entries matching the same goroutine are both named). The module path comes from `debug.ReadBuildInfo().Main.Path`, never a literal; the filtered predicate reads `test.run`/`test.skip`/`test.list`/`test.short` through an injected `flag.Lookup`-shaped function. `internal/leaktest/leaktest_test.go` (internal, whitebox) carries every T2 scenario, including the two half-alone cases (a goroutine excused at the top of its stack that also carries a module frame lower down; a goroutine whose frames are all standard library but whose creator line is this module's code) and the real module-path/real filter-predicate/wiring cases; `internal/leaktest/main_test.go` (external) is the package's own `TestMain` in the `(*testing.M).Run` form. Two authoring-time bugs caught by the tests themselves before commit: (1) the AfterFunc-callback half-alone case initially raced — `goleak.Find` ran before the timer's goroutine had even started, so both the excused and unexcused checks read clean regardless of the fix; fixed by synchronizing on a `started` channel closed inside the callback, confirmed against a scratch `go.uber.org/goleak@v1.3.0` fixture outside the module first. (2) the two-identical-entries case failed because the per-entry loop originally returned on the first not-needed entry, so only one of the two was ever named — fixed by accumulating across the whole ignore set before returning. Gates run and green: `go build ./...`, `go test ./internal/leaktest/...`, `go test -race ./internal/leaktest/...`, `golangci-lint fmt -d` (clean), `golangci-lint run ./internal/leaktest/...` (0 issues after fixing 6 errcheck findings on the unchecked `fmt.Fprint*` returns and 1 noctx finding on `net.Listen`, switched to `(*net.ListenConfig).Listen`). No decision-anchor token survived into the committed files (checked by grep before commit).
- **Subtask 3**: `internal/testdb/server_test.go`'s `callersOfMain` is now a parse through `internal/srcguard`'s `WalkSubtree`/`ParseFile` rather than a `strings.Contains` substring match: it resolves each test file's own import of this package (default name "testdb", or the import's alias) and walks the syntax tree for a selector expression naming `Main` on that identifier — matching a call, a value reference, or an aliased-import reference alike, and never matching a string literal. `TestCallersOfMain_scratch` drives it over a scratch tree with five directories (calls, passes as a value, aliased import, string-literal-only, no reference at all) via `srcguard.WriteScratchFile`/`WriteScratchFileIn`; `TestBinaries_matchesTree` keeps its own assertion unchanged and stays green against the real tree. `testdb.Main`'s doc comment is untouched here — the design scopes that edit to subtask 4. Gates run and green: `go build ./...`, `go run ./cmd/testpg -- go test ./internal/testdb/...`, `go vet ./...`, `golangci-lint fmt -d` (clean), `golangci-lint run ./internal/testdb/...`.
- **Subtask 4** (commit 763f0d9): every package with tests now declares `TestMain` as `os.Exit(leaktest.Main(m, run))`. Five sites edited in place with `run = testdb.Main` (`cmd/bot/assemble_test.go`, `internal/ingest/main_test.go`, `internal/scheduler/scheduler_test.go`, `internal/store/store_test.go`, `internal/testdb/testdb_test.go`); eleven new `main_test.go` files with `run = (*testing.M).Run` (`cmd/commentrefs`, `cmd/importguard`, `cmd/testpg`, `internal/backoff`, `internal/commentref`, `internal/config`, `internal/health`, `internal/tg`, `internal/tgtest` as their own internal package; `internal/repotest` and `internal/srcguard` as `..._test` external packages, matching their existing external-only test files). `testdb.Main`'s doc comment no longer says its exit code goes straight to `os.Exit`. `internal/leaktest` itself was untouched (already correct from subtask 2). Every command's own package (`cmd/commentrefs`, `cmd/importguard`, `cmd/testpg`) is `package main`; each new `main_test.go` there matches that. No `leaktest.Ignore` entry exists anywhere outside `internal/leaktest`'s own tests (`rg -n 'leaktest\.Ignore' --type go` → no matches). All three gate routes ran green on the whole tree: `go run ./cmd/testpg -- go test -count=1 ./...`, `LAB_GAME_TEST_DSN= go test -count=1 ./...`, and `go run ./cmd/testpg -- go test -race -count=1 ./...`. The first `-race` run hit one failure — `TestRun_reconcilesBeforeFirstCycle_RunOnceDoesNot` in `internal/scheduler`, a `write tcp ...: i/o timeout` on the DB connection, not a goleak report — that reproduced neither in isolation nor on a full second `-race` run of the whole tree; treated as the pre-existing shared-server contention the design's Open Questions section already names as out of scope, not a new leak and not this subtask's STOP condition. `go vet ./...` and `golangci-lint run` (whole tree) are green; `golangci-lint fmt -d` is clean.

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
- `internal/leaktest/leaktest.go` — subtask 2 (new)
- `internal/leaktest/leaktest_test.go` — subtask 2 (new)
- `internal/leaktest/main_test.go` — subtask 2 (new)
- `internal/testdb/server_test.go` — subtask 3
- `cmd/bot/assemble_test.go`, `internal/ingest/main_test.go`, `internal/scheduler/scheduler_test.go`, `internal/store/store_test.go`, `internal/testdb/testdb_test.go` — subtask 4 (edited in place, runner `testdb.Main`)
- `internal/testdb/testdb.go` — subtask 4 (doc comment only)
- new `main_test.go` in `cmd/commentrefs`, `cmd/importguard`, `cmd/testpg`, `internal/backoff`, `internal/commentref`, `internal/config`, `internal/health`, `internal/repotest`, `internal/srcguard`, `internal/tg`, `internal/tgtest` — subtask 4 (runner `(*testing.M).Run`)
