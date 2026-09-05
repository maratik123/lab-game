# Progress: Postgres task scheduler — ACTIVE
_Updated: 2026-09-05 11:07_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-postgres-task-scheduler
**base_commit:** 42c4c7ef29c98d46530ee8c05208c974e036c8ac
**Last build:** not run
**Issue:** #20
**Spec:** ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md
**current_step:** Step 8 — subtask 7 of 10 complete
**last_passed_gate:** golangci-lint run | 2026-09-05T14:55:00Z | (pending commit)
**entry_args:** 20

## Next action

**Do this immediately:** continue Group A at subtask 8 — the loop and the seam (`Run`, poll interval, emit-after-commit, loop observation).

## Subtasks

- [x] 1. Migration `00002_scheduler.sql`: `scheduled_task_state`, `scheduled_task`, the live-scoped identity index, the two basis tables and the `journal_entry` arc
- [x] 2. `store.DeferredTask` / `store.RecurrentTask` implementing `PostingBasis`, each carrying `TaskID`
- [x] 3. `config.Scheduler`, defaults, `loadScheduler`, `schedulerEnvKeys()`, `.env.example`
- [x] 4. Package foundation, no database: `doc.go`, `errors.go`, `task.go`, `registry.go`, `cadence.go`, `observe.go`
- [x] 5. The insertion surface: `(*Registry).Schedule`
- [x] 6. The execution cycle: `Options`, `New`, `RunOnce`, discovery + per-id re-claim
- [x] 7. The failure policy: attempt counting, backoff from the settlement instant
- [ ] 8. The loop and the seam: `Run`, poll interval, emit-after-commit, loop observation  ← CURRENT
- [ ] 5. The insertion surface: `(*Registry).Schedule`
- [ ] 6. The execution cycle: `Options`, `New`, `RunOnce`, discovery + per-id re-claim
- [ ] 7. The failure policy: attempt counting, backoff from the settlement instant
- [ ] 8. The loop and the seam: `Run`, poll interval, emit-after-commit, loop observation
- [ ] 9. The per-task deadline and the settlement of everything it abandons (`settle.go`)
- [ ] 10. `Reconcile`: the seed and the correction
- [ ] 11. Propagation (D15) — Group B

## Decisions log

- **Step 7**: design-review reached GO at round 4; the owner raised the round cap to 5 (was 3) after round 3's two confirmed majors.
- **Step 8 subtask 7**: no production-code changes were needed — subtask 6's `execute.go` already implements D7's full settlement table correctly (verified, not assumed: every new test in this subtask passed on its first run against the subtask-6 code, including the exact-attempt-count give-up, the strictly-growing backoff deltas, the decode-failure classification, the never-terminal recurrence, the undeclared-type row's untouched counters, and the counter rising/resetting). This subtask's work was entirely `failure_test.go`: attempt-by-attempt driving (each cycle preceded by forcing `run_at = now()` rather than sleeping) with exact attempt counts and strictly-growing `run_at` deltas at give-up (AC9), a decode-failure handler reaching give-up within the cap with the decode message in `last_error` and enumerable through `DeadTasks` (AC26), a recurrence driven past the one-shot cap staying pending and advancing by cadence forever (AC32), the failure counter rising then resetting to zero after both a success and a no-op, for one-shot and recurrent alike (AC34), and an undeclared type's row driven past the cap staying pending with its failure count and `last_error` untouched while `run_at` keeps advancing by the configured ceiling (AC21's execution-time half). All gates green: `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./internal/scheduler/...`, `golangci-lint fmt -d`, `golangci-lint run`.
- **Step 8 subtask 6**: added `worker.go` (`Options`, `OptionError`, `New` refusing a nil `Pool`/`Registry` or any non-positive `config.Scheduler` field, `RunOnce`), `claim.go` (the discovery statement and the per-id re-claim, both `FOR NO KEY UPDATE ... SKIP LOCKED`, never `FOR UPDATE` — AC35), and `execute.go` (the per-task transaction: `set_config` for the two timeouts per D2 step 1, the raw-SQL `SAVEPOINT`/`RELEASE`/`ROLLBACK TO` around the handler call — never `pgx.Tx.Begin`'s pseudo-nested transaction — and D7's full settlement table for Done/No-op/Failed, including the one-shot give-up and the undeclared-type row, all computed from a freshly-read settlement instant `s`, never the pre-handler instant). The per-task execution deadline (goroutine + `context.WithTimeout`) and the deferred-settlement drain are deliberately NOT here — design D11/D7 route those to subtask 9's `settle.go`; today a hung handler would hang the whole cycle, which is expected and untested until then. Tests in `worker_test.go`: batch bounded + skip-not-wait against a concurrently-held row (AC5/AC35), two workers running concurrently under `-race` with exactly-once execution (AC6/AC35), a row deleted before re-claim skipped silently, a failing handler's writes rolled back with the attempt recorded (AC7), a no-op's writes absent with the row still settled (AC8), the round-6 regression guard — a handler that swallows a real unique-violation and reports `OutcomeDone` settles as a failure because the savepoint release fails (confirmed empirically: `ROLLBACK TO SAVEPOINT` undoes *both* statements in the subtransaction, not just the failed one — the test's initial expectation of a surviving first insert was wrong and was corrected against the measured behaviour, not assumed), delete-on-done in both directions (AC25), and a recurrence's single live row surviving a committed/rolled-back/failed execution with its id unchanged (AC11). All gates green: `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./internal/scheduler/...`, `golangci-lint fmt -d`, `golangci-lint run` (one `goconst` finding on repeated `"must be positive"` strings, fixed by a table-driven refusal loop plus a named constant).
- **Step 8 subtask 5**: added `(*Registry).Schedule` and `DeadTasks` in `schedule.go`. `Payload` and `Request.Payload` are typed `json.RawMessage` (not `[]byte`) so pgx's registered JSON codec encodes/decodes them against the `jsonb` column correctly — verified empirically via `TestSchedule_payloadRoundTrip` against real Postgres, not assumed. `Schedule` mirrors `store.Post`'s `ON CONFLICT ... DO NOTHING RETURNING id` shape exactly, mapping `pgx.ErrNoRows` to `ErrDuplicateTask` (D9) — confirmed the transaction stays usable afterwards (no raw SQLSTATE 23505 is ever raised). **Noted, not chased further**: the Test Design section's insertion bullet says the duplicate-identity test wraps "SQLSTATE 23505 on scheduled_task_identity_key", which contradicts D9's own mechanism (`ON CONFLICT DO NOTHING` raises no SQLSTATE at all) — treated as a stale/imprecise summary sentence rather than a load-bearing design defect, since D9's mechanism is unambiguous and repeated three times; the test asserts `errors.Is(err, ErrDuplicateTask)` and transaction-usability instead. Added `internal/scheduler`'s `TestMain`/`newScheduler`/`dueNow`/`testRegistry`/`fixedOutcomeHandler` fixtures to `scheduler_test.go` (trimmed to only what this subtask uses — `golangci-lint run`'s `unused` linter caught and this subtask removed the not-yet-used `recordingObserver` and `seenCount`, to be added back in the subtask that first calls them). Tests in `schedule_test.go`: payload round-trip by decoded value (AC24), unregistered-type refusal writing no row, negative-delay refusal, duplicate live identity, two coexisting keyless one-shots, re-scheduling a dead identity (closing the round-1 trap, D5), and `DeadTasks` returning only give-up rows. All gates green: `go build ./...`, `go test ./...`, `golangci-lint fmt -d`, `golangci-lint run`.
- **Step 8 subtask 4**: created `internal/scheduler` with no database dependency: `doc.go` (package comment — no `internal/store` import, payload-as-data-contract), `errors.go` (five sentinels), `task.go` (`Type`, `TaskID`, `Task`, `Request`, `Outcome`, `DeadTask`, `Handler` with the ctx-propagation contract in its doc comment per D11), `observe.go` (`Observation`, `LoopObservation`, `Observer`, `FailureKind` with five members), `registry.go` (`Declaration`, `Recurrence`, `Registry`, `NewRegistry`'s five refusals), `cadence.go` (`Cadence`, `Every`, pure `backoff`). `Every`'s catch-up arithmetic multiplies in plain `int64` nanoseconds rather than `Duration*Duration`, which `durationcheck` (part of `golangci-lint run`) correctly flags as a near-always-wrong shape — caught and fixed in this subtask, not deferred. Exact table tests for `backoff` (growth, ceiling clamp, strict positivity) and `Every` (smallest-occurrence, just-completed, catch-up-after-outage) in `cadence_test.go`; `NewRegistry`'s five refusals plus the accepting case in `registry_test.go`. All gates green: `go build ./...`, `go test ./internal/scheduler/...`, `go vet ./...`, `golangci-lint fmt -d`, `golangci-lint run`.
- **Step 8 subtask 3**: added `internal/config/scheduler.go` mirroring `transport.go`'s shape — `schedulerEnvKeys()`, `Scheduler` struct, `defaultScheduler()`, `loadScheduler` — for the six `LAB_GAME_SCHEDULER_*` keys (D13's table). Appended `schedulerEnvKeys()` to `EnvKeys()` (never to the required `envKeys()`), added the `Config.Scheduler` field and `Load` wiring, and reworded the falsified doc comments in `env.go`/`config.go` naming Transport as "the one exception". Added the six keys to `.env.example` in the same commit as the loader (the design's own gate: `disjoint_test.go` requires the three key sets equal). `scheduler_test.go` mirrors `transport_test.go`'s test shape: all-absent-yields-defaults, `.env.example`-matches-defaults, values-parsed, one malformed case per key, and the unconditional-query check. All gates green: `go build ./...`, `go test ./...`, `golangci-lint fmt -d`, `golangci-lint run`.
- **Step 8 subtask 2**: added `store.DeferredTask`/`store.RecurrentTask` to `basis.go`, each carrying `TaskID int64`/`TaskType`/`InstanceKey`/`RunAt`, `NULLIF($n,0)` and `NULLIF($n,'')` mapping the Go zero values to NULL exactly as D14 specifies; rewrote `PostingBasis`'s doc comment to name all four implementations (AC20). Tests added to `basis_test.go`: nil-receiver ErrNoBasis for both new types, a balanced post under each new basis (AC4), the two-non-null CHECK refusal using the two *new* basis columns, survival of a deleted `scheduled_task` row (its journal_entry/posting rows and the by-value `task_id` on the basis row all survive, AC27/D14), two keyless one-shots distinguished by `task_id` (D14's stated reason for the column), and a `pg_constraint` sweep confirming neither new basis table carries an FK to `scheduled_task` (AC27's actual scope, not D4's wider condition). All gates green: `go build ./...`, `go test ./internal/store/...` (one transient testcontainers/podman network flake on first run, green on retry — an environment issue, not a code issue), `golangci-lint fmt -d`, `golangci-lint run`.
- **Step 8 subtask 1**: wrote `00002_scheduler.sql` exactly per D5/D14 (scheduled_task_state enum, scheduled_task with the two-predicate identity index, deferred_task/recurrent_task with `task_id` by value and no FK, the journal_entry CHECK/column/index extension). Extended migrate_test.go's table list and goose_db_version count (gate-forced), plus the index list and CHECK substring map (AC3-forced, ungated per the design's own warning) and added a new `TestMigrate_scheduledTaskShape` asserting the column set (AC31: no execution marker/heartbeat/completed state), the enum's exact two members, and the identity index's two-predicate definition (AC29). All gates green: `go build ./...`, `go test ./internal/store/...`, `golangci-lint fmt -d`, `golangci-lint run`.

## Key discoveries (don't re-investigate)

- **pgx's pseudo-nested transaction cannot recover a failed savepoint release.** `tx.go:304-311` sets `sp.closed = true` unconditionally after the `release savepoint` Exec, so `sp.Rollback` then returns `ErrTxClosed` without issuing `ROLLBACK TO SAVEPOINT`. Savepoints are managed in raw SQL on the worker's own `pgx.Tx`.
- **`FOR NO KEY UPDATE` and `FOR UPDATE` are equivalent here, and it is not a performance decision.** AC36 forbids recording it as one. The equivalence rests on no FK referencing `scheduled_task`; AC27 enforces that for basis-document tables only — a named residual risk.
- **`docs/DESIGN.md` has exactly two `FOR UPDATE` occurrences.** Line 310 is authorised for two edits; line 147 is §3.5's raid-session guard and is NOT. Line 311's `SKIP LOCKED` is also out of bounds.
- **`migrate_test.go`'s index list is a presence loop and its CHECK map matches by substring** — neither goes red on the new migration, so AC3's assertions must be added with no gate to remind you.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC36 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| D3-1 | design round 3 | major | fixed@2dcc769 | `sed -n '303,333p' $(go env GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/tx.go` |
| D3-2 | design round 3 | major | fixed@2dcc769 | `awk '/^### D7/,/^### D8/' <design> \| grep -c Deadline` |
| D4-1 | design round 4 | minor | fixed@42c4c7e | `grep -n 'settlement instant' <design>` |

## Files touched

- `internal/store/migrations/00002_scheduler.sql` (new)
- `internal/store/migrate_test.go`
- `internal/store/basis.go`
- `internal/store/basis_test.go`
- `internal/config/scheduler.go` (new)
- `internal/config/scheduler_test.go` (new)
- `internal/config/env.go`
- `internal/config/config.go`
- `.env.example`
- `internal/scheduler/doc.go` (new)
- `internal/scheduler/errors.go` (new)
- `internal/scheduler/task.go` (new)
- `internal/scheduler/observe.go` (new)
- `internal/scheduler/registry.go` (new)
- `internal/scheduler/cadence.go` (new)
- `internal/scheduler/cadence_test.go` (new)
- `internal/scheduler/registry_test.go` (new)
- `internal/scheduler/schedule.go` (new)
- `internal/scheduler/schedule_test.go` (new)
- `internal/scheduler/scheduler_test.go` (new)
- `internal/scheduler/worker.go` (new)
- `internal/scheduler/claim.go` (new)
- `internal/scheduler/execute.go` (new)
- `internal/scheduler/worker_test.go` (new)
- `internal/scheduler/failure_test.go` (new)
