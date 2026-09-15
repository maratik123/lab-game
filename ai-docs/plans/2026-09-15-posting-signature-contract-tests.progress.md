# Progress: posting-signature contract tests — ACTIVE
_Updated: 2026-09-15 12:21 UTC_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-15-posting-signature-contract-tests
**base_commit:** 47b42e978cafc9f0a991ce3267f5b6612de44ec4
**Last build:** not run

**Issue:** #26
**Spec:** ai-docs/plans/2026-09-15-posting-signature-contract-tests.spec.md

**current_step:** Step 8 — subtask 3 of 6 complete
**last_passed_gate:** go build ./...; go vet ./...; golangci-lint fmt -d; golangci-lint run (whole module); go test ./internal/contract/...; make comment-refs; make import-guard
**entry_args:** 26

## Next action

**Do this immediately:** Group A (code-writer, Mode A) — subtasks 1–6 of ai-docs/plans/2026-09-15-posting-signature-contract-tests.design.md § Decomposition, in order, one commit per subtask.

## Subtasks

Group A — code, `sonnet`/`medium` via `code-writer`:
- [x] 1. The shared pool fixture — `internal/storetest` (D13)
- [x] 2. Move the scheduler, ingest and bot suites onto `storetest.Pool` (D14)
- [x] 3. Package scaffold, vocabulary and registry — `internal/contract` (D1–D5, D12)
- [ ] 4. The pure matcher (D4, D7, D11)  ← CURRENT
- [ ] 5. The contract check against real transactions (D6, D11)
- [ ] 6. The declared set, then close the group with the whole-module gates (D8, D10)

Group B — instructions/harness, `inherit` via `general-purpose`:
- [ ] 7. Documentation (KD-42, domain-invariants, context.md layout line)

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 GO; its notes 1–4 folded (e6bced8); note 5 put to the owner, whose answers 3.1 and 3.2 narrowed the scope — spec amended in interview round 4 (7883ed1), issue #26 body updated.
- **Step 7**: design reconciled with the amended spec (b4b1240); design-review round 2 GO; its notes and recommendations folded (4fc6316).
- **Step 7**: GO note 3 (pool-helper duplication) raised an owner-scope question; owner answers 5.1 «Да, перевести» and 6.1 «Да, со всеми вызовами» — D14 and subtask 2 (3e5762b, 47b42e9). Pre-amendment round-1 GO notes are no longer authoritative (Spec Amendment recipe step 7).
- **Step 8, subtask 1** (9c23758): built `internal/storetest` per D13 exactly as designed (`Pool`, its own `TestMain` via `leaktest.Main(m, testdb.Main)`, and `TestPool_migratedAndEmpty`). `Pool`'s root context is `context.Background()` with a `//nolint:forbidigo` in the same shape as `testdb.Schema`'s own root, since `storetest.go` is a non-test file forbidigo's `_test.go` exclusion does not cover. Adding `storetest` as a new `testdb.Main` caller made `internal/testdb`'s own `TestBinaries_matchesTree` fail; `Binaries` moved 5→6 and `TestCeiling_formula`'s fixture (comments and `want` values) were recomputed for the new constant — both in the same commit, in `internal/testdb`, outside subtask 1's own file list, because the manifest test ties them together.
- **Step 8, subtask 2**: deleted `newScheduler` (`internal/scheduler/scheduler_test.go`), `newIngestPool` (`internal/ingest/main_test.go`) and `newBotPool` (`cmd/bot/readiness_test.go`), each with its doc comment, and replaced every call (the 18 files the design's subtask-2 row lists) with `storetest.Pool(t)`. `golangci-lint fmt` (scoped to the three package trees only) added the `storetest` import and dropped the now-unused `context`/`log/slog`/`store`/`testdb`/`pgxpool` imports each definition file no longer needed; `git status --short` after confirms only the 18 listed files plus the deleted-helper files moved, nothing outside the three trees. `rg -n 'func new(Scheduler|IngestPool|BotPool)\('` returns no output — no copy remains. `go build`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run`, `go test` and `go test -race` over `internal/scheduler`, `internal/ingest` and `cmd/bot` all green; `make comment-refs` green.
- **Step 8, subtask 3**: built `internal/contract`'s scaffold, vocabulary and registry per D1–D5, D12. `DocumentType` is an unexported `(basis, code string)` pair with `ManualCorrection`/`Event`/`DeferredTask`/`RecurrentTask` constructors, an unexported `playerOperation()` (no exported constructor, per D2), a `known()` predicate folding both the zero-`DocumentType` refusal and the empty-event/task-code refusal into one rule (a type-carrying basis needs a non-empty code, a table-only basis needs an empty one), and `String()`. `Sign` is `Positive`/`Negative` with the zero value unset (refused). `Cardinality` is `Exactly`/`AtLeast` with `n>=1` required. `Leg` is one struct with a `legClass` discriminator (not an interface — `PostingLeg` and `MovementLeg` both return `Leg`, matching D1's "the Leg constructors" phrasing), an unexported `key()` returning a `postingKey`/`movementKey` used for both the registry's duplicate-leg-key refusal and (subtask 4) the matcher. `Registry`/`NewRegistry`/`Declaration` hold every refusal D3–D5, D12 list; every refusal wraps `ErrInvalidDeclaration` and names the offending `DocumentType`, precedent `scheduler.NewRegistry` (D12). No `Check`/`Mark`/`Queryer` yet — those are subtask 5's D6/D11 file (`check.go`), matching the decomposition's file split. **A `Registry.signatureFor` accessor and `Cardinality.satisfiedBy` were drafted then removed**: with no caller until subtask 5/4 respectively, `golangci-lint run`'s `unused` linter (part of `default: standard`) would have failed subtask 3's own gate on its own commit; they will be (re)added with the subtask that first calls them. `String()` methods on `postingKey`/`movementKey`/`Sign`/`Cardinality` stay even though nothing calls them yet — `unused` exempts `fmt.Stringer` implementations. `internal/contract` is also a new `testdb.Main` caller (its `main_test.go`), so `Binaries` moved 6→7 (same manifest-test mechanism as subtask 1) and `TestCeiling_formula`'s fixture was recomputed again. That third bump pushed `2 * Binaries(7) * parallel * (schemaMaxConns+1) + ceilingSlack` past `ceilingMax(1000)` on this machine's `runtime.GOMAXPROCS(0)` (16) for the six `cmd/testpg` tests that call `run([]string{"--up", "--clients", "2"}, …)` with no explicit `--parallel` — a pre-existing gap the suite's own convention (`--parallel 1` pinned on every other ceiling-sensitive case, e.g. `run_test.go` line 572) was already guarding against everywhere else. Fixed by adding `"--parallel", "1"` to those six call sites (`TestRun_upOnAnUndersizedExistingServer_failsNamingTheCapacity`, `TestRun_upOnAServerSizedForFewerClients_failsNamingTheMount`, `TestRun_upReattachesWithoutVouching_refusesAndDoesNotRecordAskedCount`, `TestRun_upContainerExistsCheckFails_treatedAsAlreadyExisted_doesNotRecordAskedCount`, `TestRun_upRefusedOnMount_doesNotRaiseRecordedClients`, `TestRun_upStaleLocatorNamesAnotherServer_notRefused_recordsThisCount`), matching the sibling tests' own style — no assertion changed. `go build`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run` (whole module), `go test ./internal/contract/...`, `go test ./internal/testdb/...`, `go test ./cmd/testpg/...`, `make comment-refs`, `make import-guard` all green.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | D6 does not define the mark when no journal entry exists yet. | design-internal | folded | design § D6 zero mark, `TestNewMark_emptyJournal` @ 4fc6316 |
| G2 | 2 | § Risks says "A cancelled-context test drives each read's query-error branch" | design-internal | folded | design § D11 reads-before-judging, `TestReads_returnEveryError`, § Risks @ 4fc6316 |
| G3 | 2 | The migrated schema-pool helper (`testdb.Schema` → `store.NewPool` → `store.Migrate`) is already copied per package in 4 places | design-internal | folded | design § D13 @ 4fc6316; owner-scope extension § D14 + subtask 2 on answers 5.1 and 6.1 @ 47b42e9 |
| G4 | 2 | Two out-of-remit sentences. | design-internal | folded | both sentences removed @ 4fc6316 |
| G5 | 2 | D8's rules for the literal (catalog codes, named sign and cardinality, no helper) do not require one leg per line. | design-internal | folded | design § D8 "each leg on its own line" @ 4fc6316 |
| G6 | 2 | In the Test Design fixture list, label the test registry's rows as made-up shapes that borrow existing type codes, not the §11 signatures. | design-internal | folded | § Test Design fixture label @ 4fc6316 |
| G7 | 2 | `internal/store/errors.go:10`, cited in D1, points at the comment above `ErrNoBasis`; the declaration is on line 11. | design-internal | folded | D1 citation re-pinned to line 11 @ 4fc6316 |

## Key discoveries (don't re-investigate)

- No production code writes under any basis document today: the framework is exercised by its own test fixtures only.
- `internal/store`'s own pool helper cannot move to `storetest` (import cycle, measured in D13).

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

- `internal/storetest/storetest.go`, `internal/storetest/storetest_test.go`, `internal/storetest/main_test.go` (new — subtask 1)
- `internal/testdb/server.go`, `internal/testdb/server_test.go` (Binaries 5→6, `TestCeiling_formula` fixture — subtask 1, forced by `TestBinaries_matchesTree`)
- `internal/scheduler/scheduler_test.go` (helper deleted), `internal/scheduler/{observe,liveness,worker_stop,schedule,reclaim,reconcile,failure,panic,worker,deadline}_test.go` (call sites moved — subtask 2)
- `internal/ingest/main_test.go` (helper deleted), `internal/ingest/{retry,gate,loop_stop,dead,offset,loop}_test.go` (call sites moved — subtask 2)
- `cmd/bot/readiness_test.go` (helper deleted, call sites moved — subtask 2)
- `internal/contract/contract.go`, `internal/contract/registry.go`, `internal/contract/errors.go`, `internal/contract/main_test.go`, `internal/contract/registry_test.go`, `internal/contract/registry_internal_test.go` (new — subtask 3)
- `internal/testdb/server.go`, `internal/testdb/server_test.go` (Binaries 6→7, `TestCeiling_formula` fixture — subtask 3, forced by `TestBinaries_matchesTree`)
- `cmd/testpg/run_test.go` (pinned `--parallel 1` on 6 pre-existing tests whose fixed `--clients 2` crossed `ceilingMax` at Binaries=7 on this machine's core count — subtask 3, forced by the Binaries bump)
