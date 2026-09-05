# Progress: Postgres task scheduler — ACTIVE
_Updated: 2026-09-05 11:07_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-postgres-task-scheduler
**base_commit:** 42c4c7ef29c98d46530ee8c05208c974e036c8ac
**Last build:** not run
**Issue:** #20
**Spec:** ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md
**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | 2026-09-05T11:07:14Z | 42c4c7ef29c98d46530ee8c05208c974e036c8ac
**entry_args:** 20

## Next action

**Do this immediately:** spawn Group A (subtasks 1–10) via `/context-reset` with the `code-writer` subagent, starting at subtask 1 — the `00002_scheduler.sql` migration.

## Subtasks

- [ ] 1. Migration `00002_scheduler.sql`: `scheduled_task_state`, `scheduled_task`, the live-scoped identity index, the two basis tables and the `journal_entry` arc  ← CURRENT
- [ ] 2. `store.DeferredTask` / `store.RecurrentTask` implementing `PostingBasis`, each carrying `TaskID`
- [ ] 3. `config.Scheduler`, defaults, `loadScheduler`, `schedulerEnvKeys()`, `.env.example`
- [ ] 4. Package foundation, no database: `doc.go`, `errors.go`, `task.go`, `registry.go`, `cadence.go`, `observe.go`
- [ ] 5. The insertion surface: `(*Registry).Schedule`
- [ ] 6. The execution cycle: `Options`, `New`, `RunOnce`, discovery + per-id re-claim
- [ ] 7. The failure policy: attempt counting, backoff from the settlement instant
- [ ] 8. The loop and the seam: `Run`, poll interval, emit-after-commit, loop observation
- [ ] 9. The per-task deadline and the settlement of everything it abandons (`settle.go`)
- [ ] 10. `Reconcile`: the seed and the correction
- [ ] 11. Propagation (D15) — Group B

## Decisions log

- **Step 7**: design-review reached GO at round 4; the owner raised the round cap to 5 (was 3) after round 3's two confirmed majors.

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

- (none yet)
