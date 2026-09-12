# Design: Scheduler — survive a panicking handler, and reclaim a handler that breached its deadline

**Issue:** #81
**Date:** 2026-09-12

## Approach

Two independent failure modes, one package each, joined by one shared primitive.

**1. A handler panic becomes a modelled failure.** Today the worker runs a handler on its own
goroutine and nothing on that goroutine recovers, so a panicking handler ends the process
[measured 69073e9:internal/scheduler/execute.go:135-138 · `sed -n '135,138p'` → the `go func()` body calls
`runHandlerWithSavepoint` and sends its result, with no `defer`/`recover`]. The fix is the scheduler's
application of the general rule this module already wrote down: *a goroutine that runs a handler
supplied through an interface recovers a panic at that boundary, records its stack, and routes it
into the same failure path a returned error takes; a package's own loops are not wrapped*
[measured 69073e9:ai-docs/code-style.md:89 · `sed -n '89p'` → ownership rule 9, verbatim]. The recovery
therefore wraps **only** the `Handler.Execute` call — not the savepoint statements around it, not
`RunOnce`, not `Run`.

The recovered attempt settles exactly as a returned error does, through the settlement statement
that already exists, so the attempt cap engages (AC2); it is classified apart from a returned error
by a **new value of the existing `FailureKind` enum**, which reaches
`labgame_scheduler_tasks_total`'s existing `failure` label through the mapper the health package
already owns (AC3, AC4) [measured 69073e9:internal/health/labels.go:67-82 · `sed -n '67,82p'` →
`schedulerFailureLabel` switch, one case per member, `default` → `unknownLabelValue`].

**The stack lands on both surfaces the owner named.** The row's `last_error` column is unbounded
`text` and needs no migration [measured 69073e9:internal/store/migrations/00002_scheduler.sql:12 ·
`sed -n '12p'` → `last_error           text,`], and the settlement statement already writes the
handler error's rendered text there [measured 69073e9:internal/scheduler/settle.go:221-226 ·
`sed -n '221,226p'` → `lastError = handlerErr.Error()`]. So an error whose rendering carries the stack
reaches the row with no schema change and no new statement. The log surface needs a logger, which
neither package takes today; both gain one as an `Options` field, following the module's existing
`*slog.Logger` parameter convention [measured 69073e9:internal/store/migrate.go:71 · `sed -n '71p'` →
`func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts ...MigrateOption) (err error)`].

**2. A breached deadline reclaims its row by terminating its own backend.** The owner's decision of
2026-09-11, carried in the issue body: *"After a deadline breach the scheduler terminates the task's
own database backend from another pooled connection (`pg_terminate_backend`)"*. Every premise of it
was re-measured for this design against `postgres:18` (server reports `PostgreSQL 18.6`) through
this module's own pgx version:

- An ordinary, **non-superuser** role terminates a backend of its own, and the row it held under
  `FOR NO KEY UPDATE` is claimable at once — before the hijacked connection is closed
  [measured postgres:18.6/pgx v5.10.0 · probe as role `plain` with `rolsuper = f`:
  `SELECT pg_terminate_backend($1)` bound with a Go `int64` and **no cast in the SQL** → `true`;
  the holder's in-flight statement → `FATAL: terminating connection due to administrator command (SQLSTATE 57P01)`;
  `SELECT id … FOR NO KEY UPDATE SKIP LOCKED` on that row → `id=1 err=<nil>`].
- Terminating a **pid that is already gone** returns `false`, not an error, so a second or late
  terminate is harmless [measured postgres:18.6 · same probe, repeated terminate → `ok, returned: false`].
- The backend PID is a plain field read on the connection
  [measured pgx/v5@v5.10.0:pgconn/pgconn.go · `go doc github.com/jackc/pgx/v5/pgconn.PgConn.PID` →
  `func (pgConn *PgConn) PID() uint32` / `PID returns the backend PID.`], and the value read on the
  **idle pooled connection** equals the value read on the hijacked connection mid-statement
  [measured postgres:18.6/pgx v5.10.0 · probe → `pid-read-on-idle-pooled-conn: 85`,
  `pid-read-on-hijacked-conn-mid-statement: 85 equal: true`]. The design reads it on the idle
  connection anyway, which is the state pgx's own documentation asks for
  [measured pgx/v5@v5.10.0 · `go doc github.com/jackc/pgx/v5.Conn.PgConn` → *"It is strongly
  recommended that the connection be idle (no in-progress queries) before the underlying
  \*pgconn.PgConn is used"*].

This makes AC7 and AC8 unconditional where the delivered layers left them conditional on the
`Handler` contract: the row is claimable when `executeOne` returns, and a handler blocked in the
database gets `57P01` and returns, which releases its goroutine and its connection.

**Rejected alternatives.**

| Alternative | Why not |
|---|---|
| Recover in the worker's own loops (`RunOnce` / `Run`) as well | Forbidden by the rule this task applies: a panic in this module's own loop is this module's defect, and swallowing it hides the bug the crash names [measured 69073e9:ai-docs/code-style.md:89 · `sed -n '89p'`]. Also excluded by the spec's own Key decision (*"The worker's own loops are not wrapped"*). |
| Carry the stack on `Observation` and let `internal/health` write it | The observer is the metrics adapter; a stack as a label value is unbounded cardinality, and the closed-set label guard would refuse it [measured 69073e9:internal/health/guards_test.go:365-366 · `sed -n '365,366p'` → `assertExactSet(t, labelValuesFor(mfs, []string{familySchedulerTasks}, labelFailure), …, "labgame_scheduler_tasks_total.failure")`]. |
| A new metric family for panics | Refused by the spec (AC4) and by the telemetry obligation in the issue body. |
| A lease/heartbeat protocol to reclaim the row | Argued and rejected in the delivered scheduler design; the terminate is the layer that design named as the alternative and the owner has now chosen. |
| Issue the terminate inside the existing watchdog goroutine | Makes AC7 an asynchronous promise instead of a property that holds when `executeOne` returns, and puts the one statement whose failure an operator must see on the module's one deliberately detached launch, where no error has anywhere to go. |
| Duplicate the recover-and-capture code in both packages | Two handler boundaries today and an open trajectory (the general rule is written for *any* goroutine running a supplied handler, and the outbound-notification sender is a named future one), against this workspace's ≥3-site / ≥2-with-trajectory lift rule. See D1. |
| A size cap on the recorded stack | Unnecessary: a recovered panic's stack is bounded by the runtime's own frame cap [measured go1.26.5 linux/amd64 · a deferred `recover` calling `debug.Stack()` at recursion depth 1/50/200/5000 → `bytes=1117/8080/14185/14088`, `frames=9/58/101/101`]. Introducing a cap would add a tunable nobody needs. |

**On the spec's mechanism-naming rows.** The spec's `## Key decisions` rows that name mechanisms —
`pg_terminate_backend`, the handler-boundary recovery point, and the `failure`-label shape — are each
traced to the owner's own words in the issue body under *"Decided by the owner (2026-09-11, while
splitting #80)"* and to the issue's *Telemetry obligation*, so they are binding input rather than
spec-originated design, and they are not flagged `SPEC-REMIT`. Recorded here so the judgement is
visible rather than silent.

## Key decisions

**D1 — One shared primitive, `internal/panicguard`, not a copy per package.** It exports a type
carrying the panic value and the stack captured at the recovery point, and a constructor taking the
`recover()` result (which the deferred function must call itself). The type implements `error`,
rendering the value followed by the stack, so the caller's usual `fmt.Errorf("<pkg>: handler panic:
%w", …)` wrap puts the stack into whatever the caller already persists — and exposes the stack as a
field, so a log record can carry it as its own attribute instead of inside a message. Call sites:
the scheduler's handler boundary and update ingestion's; the trajectory is the general ownership
rule, which is written for any goroutine running a supplied handler. The package imports the
standard library only, so it adds nothing to the bot's dependency graph that the import gate
polices [measured 69073e9:cmd/importguard/run.go:37-44 · `sed -n '37,44p'` → one rule, `cmd/bot`
against `github.com/testcontainers/testcontainers-go`].

**D2 — `FailureKind` gains a member for a panic, appended after the existing members.** The numeric
value is not a data contract: no column stores it, and the metric label is derived by a switch in
the health package [measured 69073e9:internal/store/migrations/00002_scheduler.sql:4-16 ·
`sed -n '4,16p'` → `scheduled_task` has no failure-classification column;
measured 69073e9:internal/health/scheduler.go:62 · `sed -n '62p'` → the label comes from
`schedulerFailureLabel(obs.Failure)`]. Appending therefore needs no forward migration and changes no
existing member's value. Its label value is `panic`, matching the value update ingestion's own
outcome label already uses for the same event [measured 69073e9:internal/health/labels.go:99-100 ·
`sed -n '99,100p'` → `case ingest.OutcomePanic:` / `return "panic"`]. The health dashboard the game
design specifies already asks for handler errors and panics as a visible quantity (`docs/DESIGN.md`
§13.2), so this is that line's scheduler half rather than a new observability surface.

**D3 — A panic outranks a rollback failure in the classification.** `FailureRolledBack` is documented
as *the outcome the handler reported did not survive* [measured 69073e9:internal/scheduler/observe.go:25-28
· `sed -n '25,28p'`]. A panicking handler reports no outcome, so that classification cannot describe
it: the settlement's classification switch tests the recovered panic **first**, and any savepoint or
release error is folded into the recorded error rather than replacing the classification. The
existing commit-failure branch is untouched.

**D4 — A panic that leaves the transaction unusable settles through the deferred set, not by
returning an error.** Measured: a handler that panics with pgx rows still open leaves the connection
busy, and every subsequent statement on that transaction — the savepoint rollback, a settlement
`UPDATE`, even `COMMIT` — fails with `conn busy`
[measured postgres:18.6/pgx v5.10.0 · probe → `rollback-on-busy-conn err=conn busy`,
`IsBusy=true txStatus="T"`, `rollback-to-savepoint-after-panic-with-rows-open err=conn busy`].
Returning that error from `executeOne` would abort the cycle and leave the attempt uncounted, which
is precisely the hole AC2 closes. So a post-panic savepoint-rollback failure is treated as an
unusable transaction and routed into the pending-settlement set the deadline breach already uses
[measured 69073e9:internal/scheduler/settle.go:29-46 · `sed -n '29,46p'` → `pendingSettlement` and
`enqueuePending`], whose reason text is what the later drain writes to `last_error`
[measured 69073e9:internal/scheduler/settle.go:132-154 · `sed -n '132,154p'` → `deferredFailedStatement`
binds `p.reason` as the `last_error` argument in each of its shapes]. Releasing that connection is what
frees the row: pgx destroys a busy connection on release rather than returning it to the pool
[measured pgx/v5@v5.10.0:pgxpool/conn.go:19-38 · `sed -n '19,38p'` → `if conn.IsClosed() ||
conn.PgConn().IsBusy() || conn.PgConn().TxStatus() != 'I' { res.Destroy() … }`], and the row goes
lock-free after it [measured postgres:18.6/pgx v5.10.0 · probe → `row lock-free after Release`].
The common case is unaffected: a panic with no open rows leaves the transaction fully usable
[measured postgres:18.6/pgx v5.10.0 · probe → `rollback-to-savepoint err=<nil>`,
`settlement-update err=<nil>`, `commit err=<nil>`, and the handler's own write rolled back].

**D5 — The terminate is synchronous in the breach branch, on a context that survives cancellation.**
It runs after the pending settlement is enqueued and before the watchdog is launched, so AC7 holds
at the moment `executeOne` returns. Its context is derived with `context.WithoutCancel` plus a
named-constant timeout rather than a fresh root: the terminate must still happen when the deadline
context — and possibly the worker's own — is already done, and `context.Background` is a lint-gated
pattern needing a stated reason and a named owner [measured 69073e9:.golangci.yml:46-51 ·
`sed -n '46,51p'` → `forbidigo` forbids `^context\.Background$` and `^context\.TODO$`]. The timeout is
a named constant, not a configuration key, for the same reason the watchdog's close bound is one: it
bounds a local cleanup, not a tuning value [measured 69073e9:internal/scheduler/execute.go:36-42 ·
`sed -n '36,42p'` → `detachedCloseTimeout`'s stated reason]. A failed terminate is logged and falls
through to the pre-existing layers (the server's idle-in-transaction timeout, and the watchdog's
close), which are unchanged.

**D6 — The PID binds as an `int64` and the SQL carries no cast.** Both halves are measured: Postgres
resolves `pg_terminate_backend($1)` with a Go `int64` bound and no cast (D-probe above), and the
narrowing conversion the alternative would need trips the security linter while the widening one is
clean [measured 69073e9:.golangci.yml:31 · `sed -n '31p'` → `    - gosec`; and, under this repo's own
config, `golangci-lint run` over a scratch package holding both conversions →
`G115: integer overflow conversion uint32 -> int32 (gosec)` on the `int32` one and no finding on the
`int64` one].

**D7 — A nil `Logger` is accepted and replaced with a discard handler; it is not a refused option.**
Both packages already treat a nil `Observer` that way [measured 69073e9:internal/scheduler/worker.go:47-49
· `sed -n '47,49p'` → *"A nil Observer is checked, not called"*], every existing test constructs a
`Worker`/`Loop` without one, and the discard handler is what this module's own tests already pass
where a logger is mandatory [measured 69073e9:internal/scheduler/scheduler_test.go:37 · `sed -n '37p'`
→ `store.Migrate(ctx, pool, slog.New(slog.DiscardHandler))`].

**D8 — The log record is emitted at the recovery point, on the handler's own goroutine.** Not at
settlement: a handler that panics *after* its deadline was already breached has no settlement of its
own to ride, and the log is then the only surface its stack can reach. The record carries the stack
as its own attribute plus the task/update identity, at error level.

**D9 — No new configuration key, no balance number.** Nothing this design adds is a tuning value in
the sense `docs/DESIGN.md` §16.5 reserves for balance; the only numbers are the terminate's local
timeout bound (D5) and the failure-label spelling (D2).

**D10 — No schema change, therefore no forward migration.** The stack rides the existing
`last_error` text column in both packages' existing give-up/settlement statements, and no persisted
enum, column or payload key is renamed, re-numbered or repurposed. Rows written before this change
parse unchanged, and rows written after differ only in the free text of a column whose only readers
are the operator-facing projections.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | New shared primitive: the recovered-panic type (value + captured stack), its constructor taking a `recover()` result, its `error` rendering, package comment, tests, and the package's leak-check `TestMain` | `internal/panicguard/panicguard.go`, `internal/panicguard/panicguard_test.go`, `internal/panicguard/main_test.go` | — |
| 2 | `scheduler.Options` gains `Logger *slog.Logger`; `Worker` carries it; `New` substitutes a discard handler for nil (D7) | `internal/scheduler/worker.go`, `internal/scheduler/worker_test.go` | — |
| 3 | `FailureKind` gains the panic member with its doc comment; `FailureRolledBack`'s doc gains the clause that excludes a panic from it (D2, D3) | `internal/scheduler/observe.go`, `internal/scheduler/observe_test.go` | — |
| 4 | Handler-boundary recovery and its settlement: recover around `Handler.Execute` only; carry the recovered panic on the handler result; classify it first in the settlement switch; route an unusable transaction into the pending-settlement set; emit the log record at the boundary (D1, D3, D4, D8) | `internal/scheduler/execute.go`, `internal/scheduler/settle.go`, `internal/scheduler/panic_test.go` | 1, 2, 3 |
| 5 | Deadline reclaim: read the backend PID on the idle pooled connection; terminate the breached backend from the pool after the breach; log a failed terminate; correct the two doc comments whose claims this falsifies — the timeouts constant's *"one of two things"* lock-release sentence and `Handler`'s *"its row stays locked until the handler returns"* clause (D5, D6) | `internal/scheduler/execute.go`, `internal/scheduler/task.go`, `internal/scheduler/reclaim_test.go` | 2 |
| 6 | Update ingestion: `Options` gains `Logger`; the recovery helper returns the shared recovered-panic value instead of a bare `panicked` flag; its rendering carries the stack into the give-up row's `last_error`, and a log record carries it as an attribute. The reported outcome is unchanged (D1, D7, D8) | `internal/ingest/loop.go`, `internal/ingest/attempt.go`, `internal/ingest/retry_test.go`, `internal/ingest/loop_test.go` | 1 |
| 7 | Health: the failure-label mapper gains the panic case; the label unit test, the observer's failure-label test and the closed-set label guard's driver and expectation all take the new value | `internal/health/labels.go`, `internal/health/labels_test.go`, `internal/health/scheduler_test.go`, `internal/health/guards_test.go` | 3 |
| 8 | Composition root: thread the process logger into both constructors | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go` | 2, 6 |
| 9 | Goroutine-ownership allow list: the handler launch's panic answer stops saying the recovery is follow-up work, and the watchdog launch's stop answer records that a handler blocked in the database is now bounded by the terminate | `internal/gateguard/guard_test.go` | 4, 5 |
| 10 | Documentation propagation (AC5): the scheduler failure-value enumeration; the ownership rule's explanatory clause about the ctx-ignoring handler; the detached-launch paragraph that currently points at this issue as unfinished work | `ai-docs/alert-contract.md`, `ai-docs/code-style.md`, `ai-docs/process-lifecycle.md` | 3, 5 |

## Handoff plan

Grouping is required for every `M ≥ 1`, and this design has `M = 10`. Group size is capped at `10`
consecutive subtasks — a maximum, not an exact count — and a group ends at whichever comes first: the
cap, a change-type switch, or a dependency-forced boundary. Each group is homogeneous by change-type:
either **code** (`*.go`, migrations) or **instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`,
`ai-docs/**`), never both. Same-change-type subtasks are clustered into the fewest groups the
dependency order and the cap allow. The terminal group's size must fall in `1..=10`. The default
maximum is 4 groups per task; this design defines 2, so no user gate applies.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The handoff is bound at the start of **every** group, the first
  included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–9 (code change-type: `*.go`). Every same-change-type subtask is clustered
  into ONE group rather than interleaved with the documentation subtask; 9 subtasks, within the size
  cap of 10. Non-terminal.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtask 10 (instructions/harness change-type: `ai-docs/**`). Terminal group
  (1 subtask; within the `1..=10` range).

The boundary between A and B is forced by change-type homogeneity, not by the size cap: subtask 10
edits only markdown under `ai-docs/**` and depends on subtasks 3 and 5, both of which land in
Group A.

## Risks

- **A handler that panics after its deadline was already breached puts its stack only in the log,
  not in the row.** The breach branch has already enqueued the pending settlement whose reason text
  is the deadline, so the row records what settled the attempt rather than what the orphaned
  goroutine did afterwards. Mitigation: none needed — the log record is emitted at the recovery
  point on the handler's own goroutine (D8), so the stack is never lost; recorded as a consequence of
  the settlement order, not a defect — `[derived → the boundary-emitted log record asserted by the
  scheduler panic test in § Test Design]`.
- **The terminate can fail, and then AC7's guarantee falls back to the pre-existing layers.** A
  saturated pool, a closing pool on shutdown, or a backend that is already gone all make the
  statement fail or report `false`. Mitigation: the failure is logged and the pre-existing layers —
  the server's idle-in-transaction timeout and the watchdog's close of the hijacked connection —
  are left in place unchanged, so the behaviour degrades to today's rather than to nothing.
  Terminating an already-dead pid is harmless [measured postgres:18.6 · probe → repeated terminate
  → `ok, returned: false`].
- **An instantly claimable row can be re-claimed by another worker before this worker's drain counts
  the attempt.** The drain's guard already handles it: a row whose `run_at` moved on matches nothing
  and the id is dropped rather than counted against work it did not do
  [measured 69073e9:internal/scheduler/settle.go:89-127 · `sed -n '89,127p'` → `drainOneCandidateSQL`'s
  `run_at = $2` guard and `drainOne`'s "moved on without us — drop it" probe]. The drain runs at the
  top of a cycle **before** discovery [measured 69073e9:internal/scheduler/worker.go:117-130 ·
  `sed -n '117,130p'` → `drainPending` precedes `discoverDue`], so the same worker always settles
  before it can rediscover the row. Recorded as a consequence of a single-worker deployment rather
  than a new hazard.
- **`contextcheck` may object to the terminate's derived context.** The context is derived from the
  caller's `ctx` via `context.WithoutCancel`, which is the shape the linter is written to accept; if
  it fires anyway, the escape is a `//nolint:contextcheck` carrying the specific linter and the
  stated reason, as the existing watchdog launch already does
  [measured 69073e9:internal/scheduler/execute.go:158-160 · `sed -n '158,160p'` → the existing
  `//nolint:gosec,contextcheck` and `//nolint:forbidigo` with their reasons] — `[derived → the lint
  gate re-run at subtask 5]`.
- **The documentation subtask touches a file a `PreToolUse` hook blocks during an interview.**
  `ai-docs/code-style.md` is one of the files Boundary rule 2's hook guards while a spec state file
  exists and `ai-docs/plans/.task-inflight` does not [measured 69073e9:AGENTS.md:373 ·
  `grep -n "Machine-enforced while an interview is live" AGENTS.md` → the Boundary-rule-2 hook
  paragraph naming `ai-docs/code-style.md` among the guarded files]. Steps 8–12 carry the marker and
  are exempt, and subtask 10 runs in Step 8 — but an attempt to make that edit earlier will be
  refused. Mitigation: the edit is scheduled in Group B, inside Step 8.
- **A truncating gate hides later failures.** `go build ./...` caps errors per package and
  `golangci-lint run`'s own caps are switched off in this repo's config
  [measured 69073e9:.golangci.yml:85-88 · `sed -n '85,88p'` → `max-issues-per-linter: 0`,
  `max-same-issues: 0`, `uniq-by-line: false`]. Each subtask re-runs its own gate after its cleanup
  rather than trusting the first run's enumeration; any newly revealed out-of-contract class is
  surfaced to the orchestrator rather than absorbed.
- **The new failure value must be driven for the closed-set guard to accept it.** The guard asserts
  an exact set over the values actually observed in one gather, so adding the mapper case without
  driving the new member leaves the guard red — and adding the driver without the expectation leaves
  it red the other way [measured 69073e9:internal/health/guards_test.go:252-257 · `sed -n '252,257p'`
  → `driveEveryAdapterOnce`'s `FailureKind` loop]. Subtask 7 changes both halves together.
- **The propagation set the Decomposition names is a floor, not a closed list.** AC5 says in terms that the
  alert-contract enumeration *does not bound the class*. The sites subtasks 3, 7, 9 and 10 name are
  what a sweep at this commit found: `ai-docs/alert-contract.md` § Scheduler's `failure` value
  enumeration, the label mapper and its tests, the closed-set label guard's driver and expectation,
  the `FailureKind` enum's own doc comments, and the goroutine-ownership allow-list row that today
  calls this recovery follow-up work. The implementor re-runs the sweep (`grep -rni` over `.claude/`,
  `AGENTS.md`, `ai-docs/`, `docs/` and the tree, per AGENTS.md § *Propagation Rule* steps 1 and 4)
  **after** the last edit and treats any further site as in scope — `[derived → AC5]`.
- **A stack in `scheduled_task.last_error` is free text, exactly as a returned handler error already
  is.** The snapshot-sanitisation obligation names `ingest_dead_update.last_error` as the column to
  blank and does not change class here [measured 69073e9:ai-docs/domain-invariants.md:118 ·
  `sed -n '118p'` → the concrete sanitisation rows, naming `ingest_dead_update.last_error`]. Update
  ingestion's own panic stack lands in exactly that already-sanitised column, so the ingest half adds
  no new sanitisation target; the scheduler half writes into a column whose content was already
  arbitrary handler text. Recorded so the sanitisation obligation is not reopened on suspicion.

## Test Design

Every claim below is about a test that does not exist yet.

**Shared primitive — `internal/panicguard`** (table-driven, `t.Parallel()`, no database)
- Location: `internal/panicguard/panicguard_test.go`; leak check in `internal/panicguard/main_test.go`.
- Entry point: the constructor taking a `recover()` result, and the type's `error` rendering.
- Scenarios: a nil `recover()` result yields nothing; a non-nil one captures a stack that names the
  panicking function **and** its caller, and the rendering contains the panic value; the captured
  stack is bounded for a deeply recursive panic. Assertions are on properties of the mechanism —
  a frame naming the test's own panicking helper, and the presence of the value — never on the whole
  stack text, which varies with toolchain and inlining — `[derived → AC6, AC9]`.
- Fixtures: a helper that panics through one intermediate frame, and a recursive one.

**Scheduler handler panic** (`internal/scheduler/panic_test.go`, database-backed)
- Entry point: `(*Worker).RunOnce` driving a registered handler that panics.
- Fixtures: a handler that always panics with a recognisable value; a handler that panics on its
  first call and succeeds afterwards; a handler that opens rows, leaves them open and panics
  (the unusable-transaction case of D4); a recording `slog.Handler` installed through the new
  `Options.Logger`, collecting records behind a mutex.
- Scenarios:
  - AC1 — after the panicking task, the same worker's next cycle claims and runs a second due task,
    and neither `RunOnce` call returns an error — `[derived → AC1]`.
  - AC2 — a one-shot whose handler panics on every attempt reaches the `dead` state within the
    configured attempt cap, mirroring the existing deadline give-up test's shape — `[derived → AC2]`.
  - AC3 — the observation for the panicking attempt carries the failed outcome and the panic failure
    kind, and a sibling case with a handler that *returns* an error carries the handler failure kind,
    so the two are asserted apart rather than one in isolation — `[derived → AC3]`.
  - AC6 — the task row's `last_error` contains both the panic value and a frame naming the
    panicking fixture, and the recording logger holds a record whose stack attribute contains the
    same frame — `[derived → AC6]`.
  - D3 — a handler that panics **and** whose savepoint rollback succeeds still classifies as a panic,
    not as rolled-back — `[derived → AC3]`.
  - D4 — the rows-left-open handler: the attempt is still counted (the row's failure count rises
    after the next cycle's drain), the row becomes claimable, and `RunOnce` returns no error —
    `[derived → AC2]`.
- Discriminating direction: each assertion must be able to go red. The panic-versus-error pair is
  the control for AC3; for AC2 the red direction is the pre-change behaviour, where the process
  ends instead of counting an attempt.

**Scheduler deadline reclaim** (`internal/scheduler/reclaim_test.go`, database-backed)
- Entry point: `(*Worker).RunOnce` with a handler genuinely blocked in the database past its
  deadline.
- Fixture — and the fixture is the whole instrument: the handler must **neutralise the
  transaction's own `statement_timeout` first**, then issue a long sleep on a context of its own.
  Without that, the server's per-statement timeout ends the sleep by itself and the test would pass
  with the terminate removed — a green instrument measuring nothing. The fixture also reads and
  records its own backend pid at the top of `Execute`, so the test can assert on the backend
  directly.
  - AC7 variant: after its sleep errors, the handler blocks on a channel the test controls, so the
    handler has demonstrably **not** returned when the claim is probed.
  - AC8 variant: after its sleep errors, the handler records the error and returns.
- Scenarios:
  - AC7 — immediately after `RunOnce` returns, and with the AC7 fixture still inside `Execute`, a
    `FOR NO KEY UPDATE SKIP LOCKED` probe on the breached row succeeds on its **first** attempt, with
    no polling loop — `[derived → AC7]`.
  - AC8 — the AC8 fixture's `Execute` returns within a bound; the error it recorded is the
    terminate's own class rather than a context cancellation; and the backend it recorded is gone
    from `pg_stat_activity`, which is the connection-released assertion in a form the test can make
    exactly — `[derived → AC8]`.
  - The observation still carries the deadline failure kind, unchanged — `[derived → AC7]`.
- Fixtures/helpers: the existing lock-free poll helper stays for the tests that legitimately wait;
  the AC7 assertion deliberately does **not** use it. Each test releases its fixture on cleanup so
  the package's leak check ends clean.

**Update ingestion** (`internal/ingest`, database-backed)
- Entry point: the loop's attempt path with a route whose handler panics.
- Scenarios:
  - AC9 — after every attempt panics, the give-up row's `last_error` contains the panic value and a
    frame naming the panicking fixture, and the recording logger holds a record per recovered panic
    whose stack attribute contains the same frame — `[derived → AC9]`.
  - AC9 (unchanged outcome) — the per-attempt observations still report the panic outcome for each
    attempt and the give-up outcome once, in the counts the existing recovery test already pins, so
    the reported outcome is asserted unchanged rather than assumed — `[derived → AC9]`.
- Fixtures: the package's existing always-panicking handler, plus the same recording `slog.Handler`
  shape the scheduler tests use.

**Health** (`internal/health`, no database)
- Entry point: the failure-label mapper, the scheduler observer, and the closed-set label guard.
- Scenarios: the mapper maps the new member to its label value and still maps an out-of-range value
  to the unknown fallback; the observer's per-member drive observes the new value; the closed-set
  guard's exact-set expectation includes it and the adapter driver produces it — `[derived → AC4]`.

**Composition root** (`cmd/bot`)
- Scenario: assembly still succeeds and the constructed worker and loop carry the process logger —
  asserted through the existing assembly test's observation of constructed subsystems rather than by
  reaching into unexported state — `[derived → AC6, AC9]`.

**Gates.** Every subtask re-runs `go build ./...`, `go test ./...` and `golangci-lint run` for its
own package; the whole-tree `make verify` including the race gate is the Step-9 obligation, and the
race gate is mandatory here because the change touches the scheduler and goroutine boundaries.

## Open questions

- **AC9's "the same two surfaces AC6 names", for update ingestion.** AC6's two surfaces are the
  task's own row and a log record. Update ingestion's row analogue is the give-up row, which is
  written only when **every** attempt has failed; a panic that a later attempt recovers from
  persists no row at all, by the existing design — that is exactly what the distinct panic outcome
  exists to report [measured 69073e9:internal/ingest/observe.go:33-37 · `sed -n '33,37p'` →
  *"Reported distinctly from OutcomeFailed so a recovered panic that a later attempt fixes is never
  erased"*]. The design proceeds on the reading that AC9 binds the surfaces update ingestion has:
  the log record on **every** recovered panic, and the give-up row's `last_error` wherever ingestion
  persists an error at all. If the owner meant a persisted row per recovered panic, that is a new
  table and a spec amendment, not a wiring change — flagged rather than assumed.
