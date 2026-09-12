# Progress: Scheduler panic recovery and deadline reclaim — ACTIVE
_Updated: 2026-09-12 19:20_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-scheduler-panic-recovery-deadline-reclaim
**base_commit:** c7c4018
**Last build:** PASS

**Issue:** #81
**Spec:** ai-docs/plans/done/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md

**current_step:** Step 10 — self-review APPROVE (Round 2)
**last_passed_gate:** make verify | 2026-09-12T16:30Z | 8c12392
**entry_args:** 81

## Next action

**Do this immediately:** run Step 10 — spawn `self-review` over the branch diff with the closed-list prompt (invocation line, Spec, Design, Progress, commit range). Steps 9 and 9.5 are complete; the PR does not exist yet.

## Subtasks

Group A — code (`code-writer`, `sonnet`/`medium`):

- [x] 1. `internal/panicguard`: the shared recovered-panic type, its constructor from a `recover()` result, its `error` rendering, package comment, tests, leak-check `TestMain`
- [x] 2. `scheduler.Options` gains `Logger`; `Worker` carries it; nil → discard handler; correct `RunOnce`'s "no logger" clause
- [x] 3. `FailureKind` gains the panic member; `FailureRolledBack`'s doc excludes a panic; correct `LoopObservation.Err`'s "no logger" clause
- [x] 4. Handler-boundary recovery and its settlement; logger threaded into `runHandlerWithSavepoint` as a parameter; unusable transaction routed into the pending-settlement set
- [x] 5. Deadline reclaim: PID read before the handler goroutine launches; one-argument terminate; two non-success shapes reported apart; `TestDeadline_ctxIgnoringHandler_negativeCase` assertion reversed; falsified comments corrected
- [x] 6. Update ingestion: `Options` gains `Logger`; the recovery helper returns the shared value; stack into the give-up row and the log; reported outcome unchanged
- [x] 7. Health: failure-label mapper gains the panic case; label test, observer test and closed-set guard take the new value
- [x] 8. Composition root: thread the process logger into both constructors
- [x] 9. Goroutine-ownership allow list: the handler launch's `panicTo` and `stops`, and the watchdog launch's `stops`

Group B — instructions (`general-purpose`, `inherit`):

- [x] 10. Documentation propagation (AC5): failure-value enumeration, ownership rule 4's clause, the detached-launch paragraph, the `internal/` layout enumeration

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 raised a spec-amending note (AC6/AC9 unsatisfiable on both surfaces); the owner chose option (1), amend the spec — recorded verbatim as `answer 3.1` in the interview state file.
- **Step 7**: `spec-writer` amended four rows rather than the two named; the owner confirmed the four-row reach — recorded verbatim as `answer 3.2`.
- **Step 7**: round 3 returned ITERATE with the cap exhausted; the owner raised the design-review round cap to **5 (was 3)**. No orchestrator-invented bypass was used.
- **Step 7**: design round 4 reversed the terminate mechanism from the waiting two-argument form back to the one-argument signalling form, on the measurement that the wait accelerates nothing about the row and costs a structural ~100 ms inside `executeOne`.
- **Step 7**: GO at design-review round 4 (4 of a cap of 5). All five notes and three recommendations are design-internal; folded, and design-review did not run again.
- **Step 8, subtask 5**: verified `TestReclaim_AC7_…` and `TestReclaim_AC8_…` red against the pre-terminate tree (git-stashing only `execute.go`'s subtask-5 edits, re-running, then popping the stash) before trusting them green: AC7 failed on its own lock-free budget, AC8 failed on the handler never returning within its bound — both the exact red directions the design names, neither a context cancellation nor a 57P01.
- **Step 8, subtask 5**: `TestPanic_AC6_logOnlySurface_blockedPastDeadlineThenPanics` (subtask 4) initially asserted exactly one log entry; running it after the terminate landed showed a second, benign "no live backend" debug entry, because `blockThenPanicHandler` never touches `tx`, so its transaction goes idle in transaction right after the savepoint statement and the server's own `idle_in_transaction_session_timeout` (armed against the same `TaskTimeout` as the breach branch's terminate, per D5) can beat the terminate to killing the backend. Amended the test to assert on the panic's own log record by message rather than on the total entry count, since which of the two clocks wins is undecided by design and both orderings are benign.
- **Step 8, subtask 10**: `ai-docs/context.md`'s § Status *Code:* bullet was considered as a second enumeration the new package falsifies and deliberately left alone — it already omits `internal/srcguard`, `internal/leaktest` and `internal/gateguard`, so it enumerates feature blocks rather than packages, and adding one support package there would misdescribe the bullet rather than complete it. The § *Layout so far* enumeration on the same page is the one that rosters packages, and it took the addition.
- **Step 8, subtask 10**: the post-edit sweep (`grep -rni` over `.claude/`, `AGENTS.md`, `ai-docs/`, `docs/` and `README.md` for the failure-value enumeration, `#81`, the reclaim/terminate wording, ownership rule 4's clause and the package roster) found no live site beyond the design's four. `ai-docs/context-status.md`'s PR #105 entry carries the superseded reading — *"the reclamation belongs to its own issue"* — and was left as written: that file declares itself the append-only per-task log, and this task appends its own entry at Step 9.5.

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

Verified at Step 9 against `8c12392`. The command in each row is the orchestrator's own, run over that AC's stated scope; it is expected to change between rounds.

| AC | Status | Verifying command |
|----|--------|-------------------|
| AC1 | PASS | `go test ./internal/scheduler -run TestPanic_AC1 -count=1 -v` |
| AC2 | PASS | `go test ./internal/scheduler -run TestPanic_AC2 -count=1 -v` |
| AC3 | PASS | `go test ./internal/scheduler -run TestPanic_AC3 -count=1 -v` |
| AC4 | PASS | `grep -A22 'func schedulerFailureLabel' internal/health/labels.go` (the `panic` case is a value of the existing `failure` label, distinct from every other) **and** `git diff c7c4018..HEAD -- 'internal/health/*.go' \| grep -E '^\+.*(NewCounterVec\|NewGaugeVec\|NewHistogramVec\|MustRegister)'` — empty, so no family was added; the pattern was shown to match elsewhere in the tree |
| AC5 | PASS | `git ls-files '*.md' \| xargs grep -l rolled_back` — one live site (`ai-docs/alert-contract.md`), one history site under `plans/done/`; the live one enumerates `panic` in the mapper's own order. Pipeline controlled in both directions (a token hitting 5 files, a token hitting none) |
| AC6 | PASS | `go test ./internal/scheduler -run TestPanic_AC6 -count=1 -v` (both the row+log case and the log-only post-breach case) |
| AC7 | PASS | `go test ./internal/scheduler -run TestReclaim_AC7 -count=1 -v` |
| AC8 | PASS | `go test ./internal/scheduler -run TestReclaim_AC8 -count=1 -v` |
| AC9 | PASS | `go test ./internal/ingest -run TestLoop_panic -count=1 -v` — the give-up row plus log case, the superseded-attempt log-only case, and the unchanged reported outcome |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| SR1-1 | 1 | minor | fixed@f7b9508 — confirmed@2 | `sed -n '231p' internal/scheduler/execute.go` — the watchdog launch's `//nolint` reason text ends *"to free pooled resources and to unblock a handler still writing on it"*. Both halves are false of this code: `Hijack` already removed the connection from the pool (measured: `pgxpool.Conn.Hijack` sets `c.res = nil` and calls `res.Hijack()`), and the goroutine's own `<-resultCh` on the next line means the close runs only after the handler has returned, so no handler can be writing on it. Re-open by quoting the line beside the `<-resultCh` it precedes. |
| SR1-2 | 1 | minor | accepted@1 — below severity floor | `grep -n 'Logger:   logger' cmd/bot/assemble.go` returns two sites; `grep -c Logger cmd/bot/assemble_test.go` returns 0. The design's § Decomposition subtask 8 lists `cmd/bot/assemble_test.go` and § Test Design has a *Composition root* scenario `[derived → AC6, AC9]`, but the file is unchanged — deleting either `Logger: logger` fails no test. Accepted because the same § Test Design entry forbids reaching into unexported state and the worker's `logger` field is unexported in another package, so `TestAssemble_HappyPath`'s assembly-succeeds assertion is all that scenario can reach. |
| SR1-3 | 1 | nit | accepted@1 — below severity floor | `sed -n '25,34p' internal/scheduler/observe.go` — `FailureRolledBack`'s doc ends *"see FailurePanic, which outranks this classification"*, the bare-name-as-pointer shape `ai-docs/doc-convention.md` § DOC-4 leaves to review. Same-package and same const block, so it is close to the contract-symbol exemption; a rephrasing that states the rule instead of pointing would settle it. |
| SR1-4 | 1 | nit | accepted@1 — below severity floor | `grep -c 'func Test' internal/panicguard/panicguard_test.go` returns 4 separate functions where § Test Design says *"(table-driven, `t.Parallel()`, no database)"*. Accepted: the four scenarios differ structurally (nil result, capture, boundedness, interface satisfaction), so a table would carry a different fixture per row; coverage is equivalent and `t.Parallel()` is present on each. |
| SR1-5 | 1 | — | accepted@1 — examined, not a defect | `sed -n '220,229p' internal/scheduler/execute.go` — `termCancel()` is called, not deferred, against `ai-docs/code-style.md` ownership rule 5. `grep -rn 'cancel()' --include='*.go' internal/ cmd/ \| grep -v _test \| grep -v 'defer '` returns four pre-existing sites (`internal/health/canary.go`, `internal/ingest/loop.go` ×2, `cmd/bot/assemble.go`, `cmd/bot/migrate.go`), and the block between derivation and call has no return, so the cancel runs on every path. |
| SR1-6 | 1 | — | accepted@1 — examined, not a defect | `internal/scheduler/observe_test.go` and `internal/scheduler/settle.go` are named in § Decomposition subtasks 3 and 4 but unchanged. `grep -rn FailureRolledBack --include='*.go' .` shows `observe_test.go` enumerates no failure kind, and the unusable-transaction routing lands in `settleAndAfter`, which lives in `execute.go`; `pendingSettlement`/`enqueuePending` needed no change. Over-listed prediction entries, not a design contradiction. |
| SR1-7 | 1 | — | accepted@1 — examined, not a defect | `grep -n 'terminateTimeout' internal/scheduler/execute.go` — a Go constant, not configuration. D5 decides it at `5s` on `detachedCloseTimeout`'s shipped precedent; both bound a local cleanup rather than a gameplay timer, so `docs/DESIGN.md` §16.5's balance-constant rule does not reach it. |
| SR1-8 | 1 | — | accepted@1 — examined, not a defect | `grep -rn 'postgres://user:pass' --include='*.go' .` — `internal/scheduler/worker_test.go`'s new placeholder DSN matches nine pre-existing sites across `internal/config`, `internal/health`, `internal/tg` and `cmd/bot`, and points at port 1. A fixture, not a secret. |
| SR1-9 | 1 | — | accepted@1 — examined, not a defect | `grep -n parent_skill ai-docs/templates/progress-format.md` — the field is conditional (*"omit when the current skill IS the parent flow"*), and `/task` is the parent flow here. `current_step`, `last_passed_gate` and `entry_args` are all present. |
| SR2-1 | 2 | — | accepted@2 — examined, not a defect | `sed -n '231p' internal/scheduler/execute.go` — SR1-1's replacement text says an unclosed connection *"holds its socket and its server-side session for the life of the process"*. The socket half is unconditionally true; the server-side half is true exactly on the paths where the close is load-bearing (a terminate that returned a Go error, so the backend is still alive), and vacuous where the terminate landed, since the backend is already gone. Not the class SR1-1 was: it asserts a general property offered as the close's reason, not a causal role the goroutine's own ordering forbids. |

## Files touched

- `internal/panicguard/panicguard.go` (new)
- `internal/panicguard/panicguard_test.go` (new)
- `internal/panicguard/main_test.go` (new)
- `internal/scheduler/worker.go` (Options.Logger, discard-handler substitution, RunOnce doc fix)
- `internal/scheduler/worker_test.go` (Logger nil/given coverage)
- `internal/scheduler/observe.go` (FailurePanic member, doc fixes)
- `internal/scheduler/execute.go` (handler-boundary recovery, unusable-transaction routing)
- `internal/scheduler/panic_test.go` (new; later amended in subtask 5 for the terminate/idle_in_transaction_session_timeout race)
- `internal/scheduler/execute.go` (pid read, synchronous one-argument terminate, comment fixes)
- `internal/scheduler/task.go` (Handler doc comment corrected)
- `internal/scheduler/deadline_test.go` (waitLockFree doc fix; ctxIgnoringHandler negative case reversed and renamed)
- `internal/scheduler/reclaim_test.go` (new: AC7/AC8)
- `internal/ingest/loop.go` (Options.Logger, discard-handler default)
- `internal/ingest/attempt.go` (safeHandle uses panicguard, logs at recovery point)
- `internal/ingest/observe.go` (LoopObservation.Err doc fix)
- `internal/ingest/retry_test.go` (row + log-only surface tests)
- `internal/ingest/loop_test.go` (newLoopWithLogger helper)
- `internal/health/labels.go` (schedulerFailureLabel gains the panic case)
- `internal/health/labels_test.go`, `internal/health/scheduler_test.go`, `internal/health/guards_test.go` (panic value driven/expected)
- `cmd/bot/assemble.go` (Logger threaded into scheduler.New and ingest.New)
- `internal/gateguard/guard_test.go` (`(*Worker).executeOne` launch table rows updated: panicTo/stops narrowed for both the handler launch and the watchdog launch)
- `ai-docs/alert-contract.md` (§ Scheduler: `panic` added to the `failure` value enumeration, with the clause telling it apart from `handler`)
- `ai-docs/code-style.md` (ownership rule 4 narrowed to the goroutine alone and only outside the database; the row's lock put outside the exception in both cases)
- `ai-docs/process-lifecycle.md` (the detached-launch paragraph: the watchdog's wait bounded for the in-database case, the out-of-database case named as why the launch stays detached)
- `ai-docs/context.md` (§ *Layout so far* gains `internal/panicguard`)
- **Step 8, Group A boundary (orchestrator)**: re-ran `go build ./...`, `go vet ./...`, `golangci-lint run` and `golangci-lint fmt -d` at `e69270b` — all green; the group's own gate claim is confirmed against the tree rather than taken from its return summary. Header fields `Last build`, `last_passed_gate` and `Next action` were left stale by the group and are corrected here.
- **Step 9**: `make verify` green at `8c12392` — it runs fmt-check, build, vet, lint, file-limits, test, test-race, tidy-check, actionlint, shellcheck, comment-refs and import-guard in one pass, so Step 9 items 1–9b are discharged by it; dependencies did not move and no workflow or shell script was touched, so the tidy and actionlint/shellcheck stages had nothing to act on.
- **Step 9**: panic-index sync — no `panic(`, `log.Fatal`/`log.Panic` or `Must…` helper was added to any of the ten changed production files (scan controlled against a constructed matching line); `ai-docs/panic-index.md` needs no row and stays empty.
- **Step 9**: domain-invariant sweep — no balance mutation, no posting or item-movement write in production code, no balance constant in Go, no secret. Two classes of hit are legitimate and recorded here rather than fixed: `store.Post` appears only in `internal/ingest`'s test fixtures, which seed ledger state for the panic tests; and every `time.Now()` hit is either an observability duration measurement on an already-existing pattern (`worker.go`, `loop.go`, `attempt.go`) or a test's polling deadline — this diff touches no generation, combat or replay path, which is what the determinism rule governs. All five sweep patterns were controlled against constructed matching lines.
- **Step 9**: no telemetry obligation beyond AC4/AC5 — the change adds a value to an existing `failure` label, no metric family and no event; the issue's own Telemetry obligation states "Events: none", and the design records that in D9.
- **Step 9.5**: appended this task's entry to `ai-docs/context-status.md` with the literal `#TBD-at-Step-12` locator (exactly one occurrence in the file, to be substituted at Step 12 sub-step 10a), and bumped `ai-docs/context.md`'s `internal/scheduler` clause in § *Layout so far* with the handler-boundary recovery and the deadline reclaim. No open question in `context.md` was resolved by this task — its § Status open questions are `docs/DESIGN.md` §16's, untouched here. `README.md` names neither the scheduler nor handler panics, so nothing there is contradicted; `docs/DESIGN.md` §13.2's telemetry line asking for handler panics is satisfied by this change, not falsified, and the design corpus is not edited.
- **Step 9.5**: removal sweep for claims the diff falsifies — no live surface still says the scheduler or ingest has no logger, still points at #81 as unfinished, or still says a breached row stays locked until its handler returns. Every remaining hit is either this task's own spec/design/progress (which describe the change) or `ai-docs/plans/done/**`, a history surface. `ai-docs/context-status.md`'s PR #105 entry keeps its then-true sentence: that file declares itself the append-only per-task log, so a superseded entry is superseded by the new one rather than edited.

## Self-Review (Round 1)

**Verdict:** APPROVE

No `blocker` or `major` row is open, so the table carries no rows.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|

**`minor` / `nit` count: 4**, across `internal/scheduler/execute.go`, `cmd/bot/assemble.go` (+ `cmd/bot/assemble_test.go`, unchanged), `internal/scheduler/observe.go`, `internal/panicguard/panicguard_test.go`. Each is entered in the register as `SR1-1` … `SR1-4` with its verifying command; the orchestrator may promote one. The two worth naming here: the watchdog launch's `//nolint` reason text at `internal/scheduler/execute.go:231` now ends in a clause that is false of the code it sits on — the close cannot *"unblock a handler still writing on it"*, because the goroutine's own `<-resultCh` on the next line means the handler has already returned, and the connection is no longer pooled after `Hijack` (SR1-1); and the composition root's `Logger: logger` wiring is asserted by no test, though the design's own no-unexported-state constraint is what leaves it that way (SR1-2). A further five items were examined and ruled not-a-defect; they are registered as `SR1-5` … `SR1-9` rather than left as prose, so the next round does not re-litigate them.

### What was checked

**Diff window.** `d0ba255..HEAD` (the whole branch; `d0ba255` is the merge-base with `main` and is wider than the header's `base_commit` `c7c4018`). 16 commits, 33 files. The three commits after `8c12392` — the commit `last_passed_gate` records — touch `ai-docs/*.md` only (`git diff --stat 8c12392..HEAD`), so the recorded `make verify` covers every `.go` change.

**Spawn prompt.** Within the closed list — invocation line, `Spec:`, `Design:`, `Progress:`, commit range. No `PROMPT-CONTAMINATION`.

**Task and spec conformance.** Read the issue body, the owner's three 2026-09-11 decisions and all four `prior_qa` answers in `…spec.md.state.md` (`issue_body_status: current`). The body's *"Out of scope — Update ingestion"* is overridden by `answer 1.2` (*"one rule, both handler boundaries"*), and the spec's Out-of-scope row carries the narrowed form; Scope 1–8 map onto AC1–AC9 with nothing outside them. The issue's *Telemetry obligation* — *"Events: none"* — is the design's D9 citation, verified verbatim at `…state.md:63`.

**Every AC re-run against the shipped tree**, not taken from the AC Status table:

- AC1/AC2/AC3/AC6 — `go test ./internal/scheduler -run 'TestPanic_AC1|TestPanic_AC2|TestPanic_AC3|TestPanic_AC6|TestPanic_D4|TestPanic_recovers' -count=1 -v` → **PASS**, 7/7.
- AC4 — `grep -A22 'func schedulerFailureLabel' internal/health/labels.go` → the `panic` case present, distinct from every other value; `git diff c7c4018..HEAD -- 'internal/health/*.go' | grep -E '^\+.*(NewCounterVec|NewGaugeVec|NewHistogramVec|MustRegister)'` → empty (exit 1), so no family was added. **PASS**.
- AC5 — `git ls-files '*.md' | xargs grep -l rolled_back` → `ai-docs/alert-contract.md` (live, now enumerating `panic` in the mapper's own order), this task's own progress file, and one `plans/done/` history site. Pipeline controlled with a token present in no file (exit 123). **PASS**.
- AC7/AC8 — `go test ./internal/scheduler -run 'TestReclaim_AC7|TestReclaim_AC8|TestDeadline_ctxIgnoringHandler' -count=1 -v` → **PASS**, 3/3.
- AC9 — `go test ./internal/ingest -run TestLoop_panic -count=1 -v` → **PASS**, 3/3 (row+log, superseded-attempt log-only, unchanged reported outcome).

**Three mutations — the instrument was seen red before any green was believed.** Each applied to a `cp` backup of `internal/scheduler/execute.go` and reverted by `cp` (never `git checkout`); the tree is clean afterwards.

1. `terminateBackendSQL` → `SELECT $1::bigint IS NOT NULL` (the call still runs and still returns `true`, only the termination is removed): `TestReclaim_AC7` red on its own 2s budget (*"row 1 never became lock-free within 2s"*), `TestReclaim_AC8` red on the handler never returning (*"did not return within the bound"*), `TestDeadline_ctxIgnoringHandler_rowReclaimedDespiteIgnoredCtx` red on 10s. All three red in the exact directions § Test Design names.
2. Log emission deleted from `callHandlerRecovered`: `TestPanic_AC6_rowAndLogBothCarryStack` red (*"log entries = 0, want exactly 1"*), `TestPanic_AC6_logOnlySurface_…` red after 10s. The AC6 log clause is load-bearing, not incidental.
3. `panicked = true` → `panicked = false`: `TestPanic_AC3` red on the classification, `TestPanic_D4` red on `conn busy` reaching `settleOutcome`. The panic-outranks-rolled-back rule (D3) and the unusable-transaction route (D4) are both genuinely exercised.

**Gates at HEAD**, each captured to a file and read by grep, never piped: `go build ./...` **GREEN**; `go vet ./...` **GREEN**; `golangci-lint run` **GREEN**; `golangci-lint fmt -d` **GREEN** (0 bytes of diff); `make comment-refs` **GREEN**; `make file-limits` **GREEN**; `make import-guard` **GREEN**; `go test ./... -count=1` **GREEN**; `go test -race ./internal/scheduler ./internal/ingest ./internal/panicguard ./internal/health ./internal/gateguard -count=1` **GREEN** — the race gate re-run at HEAD rather than inherited from the Step-9 record, since the diff touches the scheduler worker.

**Design conformance.** All eight `## GO notes` rows are routed `folded @ c7c4018`, the commit immediately before the first implementation commit — so the fold-in preceded the diff. Spot-checked three against the shipped design: G3's *"Every premise below was measured…"* preamble is gone (grep exit 1), G2's claimability figures are reframed as the claimer's *detection instant* (2 occurrences), G5's `5s` is decided in D5 and matches `terminateTimeout`. D5's ordering holds in code — pending settlement enqueued, then the terminate, then the watchdog launch. D5's pid-read point holds: `conn.Conn().PgConn().PID()` sits between `deadlineCtx` and the `go func()`, the last point at which the connection is provably idle. D8's emission point is the recovery point, which is what mutation 2 proved load-bearing for the log-only path. D6's `int64(pid)` is the widening conversion, and `gosec` is silent.

**Safety and correctness.** Panicking-call audit over the ten changed production files — `grep -nE '(^|[^[:alnum:]_.])(panic\(|log\.(Fatal|Panic)[a-z]*\()'` → no hits (exit 1), control line matched, so `ai-docs/panic-index.md` needs no row. No `_ = err` added; the terminate's two non-success shapes are both read (boolean and Go error) and logged at the levels D5 fixes. Context discipline: `ctx` first everywhere, nothing stored in a struct, and the terminate derives from the caller's `ctx` via `context.WithoutCancel` rather than inventing a root. Goroutine ownership: the diff adds no `go` statement; both pre-existing launches' allow-list rows were updated and `internal/gateguard` passes. No `…Unchecked` function added. Verified there is no pool deadlock in the breach branch: `pgxpool.Conn.Hijack` sets `c.res = nil` and calls `res.Hijack()`, freeing the slot before `w.pool.QueryRow` asks for one.

**Domain invariants.** No ledger or item-movement write in changed production code (grep with a matching control line). No balance constant in Go — `terminateTimeout` is a local cleanup bound on `detachedCloseTimeout`'s precedent (SR1-7). No `.sql` in the diff, so no schema break; D10 holds — the stack rides the existing unbounded `last_error` text. No outbound send path touched. No generation, combat or replay path touched, so the determinism rule is not engaged. No secret (SR1-8).

**Documentation.** Every exported item added — `panicguard.Recovered`, its `Value`/`Stack` fields, `New`, `Error`, `scheduler.FailurePanic`, `scheduler.Options.Logger`, `ingest.Options.Logger` — carries a name-first third-person doc comment, and `internal/panicguard` carries a package comment. `make comment-refs` green. The four falsified-by-absence *"this package has no logger"* clauses (D11) are all corrected, and the `Handler` contract, `setTimeoutsSQL`'s lock-release sentence and `waitLockFree`'s enumeration are rewritten to what the terminate makes true. The one comment whose replacement text is itself false is SR1-1.

**Prose half of the diff, verified rather than read** (`ai-docs/alert-contract.md`, `ai-docs/code-style.md`, `ai-docs/process-lifecycle.md`, `ai-docs/context.md`): the `failure` enumeration matches the mapper's own order; ownership rule 4's narrowing matches what `TestReclaim_AC8` demonstrates; the process-lifecycle claim that the terminate precedes the watchdog launch matches the code order. Propagation sweep for the claims the diff falsifies — *"no logger"*, *"stays locked"* / *"still locked"* / *"remains locked"*, `#81` — over `.go` and `.md` outside `ai-docs/plans/`: every survivor is either a history surface under `plans/done/`, this task's own artefacts, or a true statement about something else (`settle.go`'s drain probe, `deadline_test.go`'s un-settled row). Nothing live is left falsified.
- **Step 10**: self-review round 1 returned **APPROVE**; no `blocker` or `major` row, nine register rows all `accepted@1`. Its instrument check was three mutations on a `cp` backup of `execute.go`, each reverted: removing the termination reddened AC7, AC8 and the reversed ctx-ignoring case; deleting the log emission reddened both AC6 tests; flipping the recovered flag reddened AC3 and the unusable-transaction route.
- **Step 11**: promoted `SR1-1` from `accepted@1 — below severity floor` to a fix, overriding the reviewer's wave-through in the direction of more correctness. Its two premises were re-verified first — `pgxpool.Conn.Hijack` nils the pool resource and hijacks it (read in the module cache at the version `go.mod` pins), and the watchdog's own receive on the handler result channel precedes the close — so both halves of the reason were false of the code they sit on. The design's subtask 5 listed this very comment as one the terminate falsifies, so replacing one false claim with another left that subtask unfinished rather than done. Gates re-run after the edit: build, vet, lint, `fmt -d`, comment-refs — all green.

## Self-Review (Round 2)

**Verdict:** APPROVE

No `blocker` or `major` row is open, so the table carries no rows.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|

**New `minor` / `nit` count this round: 0.** `SR1-2`, `SR1-3` and `SR1-4` stand as `accepted@1`; nothing in the round-2 diff changes the grounds any of them was accepted on, so none is re-raised. One item was examined and ruled not-a-defect and is registered as `SR2-1` rather than left as prose.

### What was checked

**Round-2 diff window.** `f7dfed4..HEAD` (`b7074e3`) — two commits, two files: `internal/scheduler/execute.go` (one line, comment-only) and this progress file. The register scoped the round: `SR1-1` carries `fixed@f7b9508`, so it was re-examined at that sha; the eight `accepted@1` rows were left alone, per the register rule that a re-raise needs both the accepting reason quoted and a command showing what changed — nothing in this diff touches any of their subjects.

**`SR1-1` — fix verified, not taken on the commit message.** Ran the row's own verifying command, `sed -n '231p' internal/scheduler/execute.go`. Both clauses the row named are gone. The replacement makes three claims and each was re-derived:

- *"hijacking took it out of the pool, so no other owner will ever close it"* — **true**. `go doc github.com/jackc/pgx/v5/pgxpool.Conn.Hijack` → *"Hijack assumes ownership of the connection from the pool. Caller is responsible for closing the connection."*, and the source at the pinned `v5.10.0` sets `c.res = nil` then calls `res.Hijack()`.
- *"an unclosed connection holds its socket and its server-side session for the life of the process"* — **true as the general property it is offered as**; the one nuance is registered as `SR2-1` rather than raised.
- *"The receive below orders the close after the orphaned handler has returned, so the close never races a handler still using the connection"* — **true**, and it is the correct inversion of the clause `SR1-1` falsified: `<-resultCh` is the goroutine's first statement and `pconn.Close` its last.

The new text carries no outward reference — no path, no `§`, no acceptance-criterion id, no package-qualified symbol of this module — and *"The receive below"* points inside the comment's own call site rather than out of it. It states the close's guarantee and the mechanism that supplies it, which is what a `//nolint` reason must carry; it does not narrate the body step by step.

**Gates re-run at `b7074e3`**, each captured to a file and read by grep: `go build ./...` **GREEN**; `go vet ./...` **GREEN**; `golangci-lint run` **GREEN** (the `nolintlint` requirement that the directive name a linter and state a reason survives the rewrite); `golangci-lint fmt -d` **GREEN** (0 bytes); `make comment-refs` **GREEN**; `make file-limits` **GREEN**. And, because the commit touches `internal/scheduler/execute.go` — the scheduler worker — the race gate was re-run rather than inherited from round 1: `go test -race ./internal/scheduler ./internal/ingest ./internal/panicguard ./internal/health ./internal/gateguard -count=1` → **GREEN**, 5/5. A shared test server was brought up for it and removed again.

**Round-1 section integrity.** `git diff f7dfed4..HEAD` over this file removes exactly one line — the `**current_step:**` header field, whose content is the calling skill's to own. The `## Self-Review (Round 1)` section is untouched; this round is appended beside it, not over it.

**Everything round 1 established stands unretested here, deliberately.** The round-2 diff changes no compiled statement, so the nine ACs, the three mutations, the domain-invariant sweep, the propagation sweep and the full suite are not re-derived — re-running them would be spending the round on a comment. What could have broken on a comment-only change is the lint, format and comment-reference gates, and each was run.
