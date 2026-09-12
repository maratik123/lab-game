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

**The stack lands on the surfaces the amended spec names: the log record always, the row wherever
the attempt leaves the row an outcome of its own.** The row's `last_error` column is unbounded
`text` and needs no migration [measured 69073e9:internal/store/migrations/00002_scheduler.sql:12 ·
`sed -n '12p'` → `last_error           text,`], and the settlement statement already writes the
handler error's rendered text there [measured 69073e9:internal/scheduler/settle.go:221-226 ·
`sed -n '221,226p'` → `lastError = handlerErr.Error()`]. So an error whose rendering carries the stack
reaches the row with no schema change and no new statement, on both settlement paths: the inline one,
and the deferred drain, whose `reason` text is bound into the same column (D4's measurement of
`deferredFailedStatement`). Where the attempt leaves the row no outcome of its own — a panic on a
goroutine whose deadline was already breached, so the row records the breach; an ingest panic a later
attempt supersedes, so the give-up row records that later attempt or is never written at all — the
log record is the whole surface, which is why it is emitted at the recovery point rather than at
settlement (D8, where both paths are measured). The log surface needs a logger, which
neither package takes today; both gain one as an `Options` field, following the module's existing
`*slog.Logger` parameter convention [measured 69073e9:internal/store/migrate.go:71 · `sed -n '71p'` →
`func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, opts ...MigrateOption) (err error)`].

**2. A breached deadline reclaims its row by terminating its own backend — with the *waiting* form of
the call.** The owner's decision of 2026-09-11, carried in the issue body: *"After a deadline breach
the scheduler terminates the task's own database backend from another pooled connection
(`pg_terminate_backend`)"*. **Which of that function's two argument shapes to bind is the design's
call, and round 3 reverses round 2's answer.** There is one implementation, not two:
`pg_terminate_backend(pid integer, timeout bigint DEFAULT 0)`, and the one-argument call is that
function taking the default [measured postgres:18.6 · `select p.oid::regprocedure,
pg_get_function_arguments(p.oid) from pg_proc p where proname='pg_terminate_backend'` → one row:
`pg_terminate_backend(integer,bigint)` / `pid integer, timeout bigint DEFAULT 0`]. The two shapes
differ in what they promise: with `timeout` zero the function *"returns `true` whether the process
actually terminates or not, indicating only that the sending of the signal was successful"*, while
with `timeout` greater than zero it *"waits until the process is actually terminated or until the
given time has passed"* — `true` on termination, and on timeout *"a warning is emitted and `false` is
returned"* (<https://www.postgresql.org/docs/18/functions-admin.html>). AC7's property is the
backend's **death**, not the signal's delivery, so **the design binds the waiting form**.

Every premise was re-measured for round 3 against `postgres:18` through this module's own pgx
version, in the **production ordering** — terminate, then the claim probe, with nothing in between,
on a claimer connection established *before* the terminate. The ordering is load-bearing twice over:
reading the victim's own error first would synchronise on its death, and a claimer that connects
afterwards buys the probe about a millisecond of slack — either one hides the effect below.

- **The one-argument form does not leave the row claimable at the instant it returns; the waiting
  form does.** [measured postgres:18.6 (`select version()` → `PostgreSQL 18.6 (Debian
  18.6-1.pgdg13+2)`)/pgx v5.10.0 · holder parked in `pg_sleep(30)` under `FOR NO KEY UPDATE`,
  terminate from a pooled connection, one immediate `FOR NO KEY UPDATE SKIP LOCKED` probe on the
  pre-established claimer → one-argument form: `mode=one iters=200 first-probe-misses=2
  max-wait-after-miss=9.590664ms` and, repeated, `first-probe-misses=3
  max-wait-after-miss=9.419204ms`; two-argument form with `timeout = 5000`: `mode=two iters=200
  first-probe-misses=0` on each of two runs]. Round 2's design read AC7 off the one-argument form.
  That was wrong, and this is the correction.
- **The waiting form holds for the ordinary role and for both holder states.** A **non-superuser**
  role terminates a backend of its own — a role is a member of itself, which is the documented grant
  — and the claim succeeds on the first probe whether the victim is mid-statement or **idle in
  transaction**, the state a handler blocked on a Go channel leaves it in
  [measured postgres:18.6/pgx v5.10.0 · probe as role `plain` with `rolsuper = f`,
  `select pg_terminate_backend($1, $2)` bound with two Go `int64`s and **no cast in the SQL** →
  `mode=plain busy-holder=true iters=100 first-probe-misses=0` and `mode=idle busy-holder=false
  iters=100 first-probe-misses=0`; the holder's in-flight statement →
  `FATAL: terminating connection due to administrator command (SQLSTATE 57P01)`].
- **A `false` return is not a Go error, and the boolean is the only thing that reports a failed
  terminate.** Both non-success shapes of the waiting form come back with `err == nil`: a pid that is
  already gone, and a wait that ran out [measured postgres:18.6/pgx v5.10.0 · probe →
  `gone-pid two-arg: returned=false err=<nil>`; and, forcing the timeout branch with `timeout = 1`,
  `1ms-timeout two-arg: returned=false goErr=<nil>` with the server's `WARNING: backend with PID …
  did not terminate within 1 millisecond` arriving on pgx's notice channel]. So the code branches on
  the boolean, not on the error alone — D5. An already-gone pid remains harmless, as it was under the
  one-argument form; only its return value differs.
- **When the waiting form returns, the victim is already out of `pg_stat_activity`.**
  [measured postgres:18.6/pgx v5.10.0 · probe → `mode=statgone iters=200
  still-in-pg_stat_activity-on-first-read=0`]. § Test Design still polls that assertion rather than
  reading once, because what this design owns is the call's return contract, not the server's
  bookkeeping latency under someone else's load.
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
`Handler` contract: the terminate returns only once the backend is gone, so the row is claimable when
`executeOne` returns, and a handler blocked in the database gets `57P01` and returns, which releases
its goroutine and its connection. The one path that degrades is a terminate that reports `false`, and
it degrades to today's behaviour rather than to nothing (§ Risks).

**Rejected alternatives.**

| Alternative | Why not |
|---|---|
| Recover in the worker's own loops (`RunOnce` / `Run`) as well | Forbidden by the rule this task applies: a panic in this module's own loop is this module's defect, and swallowing it hides the bug the crash names [measured 69073e9:ai-docs/code-style.md:89 · `sed -n '89p'`]. Also excluded by the spec's own Key decision (*"The worker's own loops are not wrapped"*). |
| Carry the stack on `Observation` and let `internal/health` write it | The observer is the metrics adapter; a stack as a label value is unbounded cardinality, and the closed-set label guard would refuse it [measured 69073e9:internal/health/guards_test.go:365-366 · `sed -n '365,366p'` → `assertExactSet(t, labelValuesFor(mfs, []string{familySchedulerTasks}, labelFailure), …, "labgame_scheduler_tasks_total.failure")`]. |
| A new metric family for panics | Refused by the spec (AC4) and by the telemetry obligation in the issue body. |
| A lease/heartbeat protocol to reclaim the row | Argued and rejected in the delivered scheduler design; the terminate is the layer that design named as the alternative and the owner has now chosen. |
| Issue the terminate inside the existing watchdog goroutine | Makes AC7 an asynchronous promise instead of a property that holds when `executeOne` returns, and puts the one statement whose failure an operator must see on the module's one deliberately detached launch, where no error has anywhere to go. |
| The one-argument `pg_terminate_backend($1)`, keeping AC7's single-read claim probe | Measured red: the signal's delivery is not the backend's death, so the row stays locked for up to tens of milliseconds after the call returns and a single-read probe misses a few times in two hundred (§ Approach). A test red at that rate is a flake, and under `make test-contention` a worse one. |
| The one-argument form, with AC7's claim probe **polling** within a bound (the review's option (b)) | It would work *as a test*, and the design says so rather than arguing otherwise: with the AC7 fixture's `statement_timeout` neutralised the backend stays **active** inside its sleep, and `idle_in_transaction_session_timeout` bounds only the Go-side gaps *between* statements, *"not any single query"* [measured 36ef3e1:internal/scheduler/execute.go:13-34 · `sed -n '13,34p'` → the constant's doc naming the distinction and the two pre-existing release paths, and `setTimeoutsSQL` binding `$1` to both timeouts], so neither pre-existing layer reaches that backend and a bounded poll goes red against the pre-change tree. Rejected because it repairs the **test** and leaves the **product** where it was: AC7 asks that the row be claimable without waiting for the handler, and under the one-argument form there is a window of tens of milliseconds after `executeOne` returns in which it is not (§ Approach). It also moves the assertion's discriminating power out of the mechanism and into the fixture — it holds only while the neutralisation stays in place and the poll ceiling stays under the sleep. The waiting form makes the property true instead, for the price of one extra bound (D5). |
| Duplicate the recover-and-capture code in both packages | Two handler boundaries today and an open trajectory (the general rule is written for *any* goroutine running a supplied handler, and the outbound-notification sender is a named future one), against this workspace's ≥3-site / ≥2-with-trajectory lift rule. See D1. |
| A size cap on the recorded stack | Unnecessary: the traceback the runtime renders for a recovered panic is frame-capped by the runtime itself, so the rendered stack stops growing once that cap is reached however deep the recursion goes [measured go1.26.5 linux/amd64 (`go version` → `go1.26.5-X:nodwarf5 linux/amd64`) · a deferred `recover` calling `debug.Stack()` at rising recursion depth → the rendered byte length plateaus instead of growing with depth, and the deepest probe renders no longer than the mid one]. Introducing a cap would add a tunable nobody needs. |

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

**What this branch reports, in full.** It borrows the deadline breach's *mechanism* and none of its
*classification*. Each value below is fixed here rather than left to the implementor, because this
branch is where AC2, AC3 and AC6 all land on code with no settlement statement to ride:

- the observation's `Failure` is the **panic member** (D2) — not `FailureDeadline`, which the breach
  branch uses [measured 100928c:internal/scheduler/execute.go:164-166 · `sed -n '163,168p'` → the
  breach branch's `obs.Outcome = OutcomeFailed`, `obs.Failure = FailureDeadline`,
  `obs.ConsecutiveFailures = task.ConsecutiveFailures + 1`], and not `FailureRolledBack`, which the
  nearest existing enqueue-and-observe branch uses
  [measured 100928c:internal/scheduler/execute.go:202-212 · `sed -n '200,224p'` → `settleAndAfter`'s
  commit-failure branch: `enqueuePending` with `consecutiveFailures: task.ConsecutiveFailures` and a
  `reason` built from the error, then `obs.Failure = FailureRolledBack`]. The attempt ended in a
  panic, and D3's outranking rule decides it;
- the observation's `Outcome` is failed and its `ConsecutiveFailures` is the claimed count plus one,
  and the enqueued `consecutiveFailures` is the claimed count itself — the commit-failure branch's
  shape verbatim (same measurement), since the drain adds its own one;
- `pendingSettlement.reason` is the **rendered recovered-panic error** — the same text the inline
  settlement would have written — never the deadline's fixed string. That is the only way the stack
  reaches `last_error` on this branch, since `reason` is what the drain binds there
  [measured 69073e9:internal/scheduler/settle.go:132-154 · `sed -n '132,154p'`]. The drain's write is
  this attempt's own outcome on the row, so amended AC6's row clause binds here and is met.

The common case is unaffected: a panic with no open rows leaves the transaction fully usable
[measured postgres:18.6/pgx v5.10.0 · probe → `rollback-to-savepoint err=<nil>`,
`settlement-update err=<nil>`, `commit err=<nil>`, and the handler's own write rolled back].

**D5 — The terminate is synchronous in the breach branch, waits for the backend's death, and runs on
a context that survives cancellation.** It runs after the pending settlement is enqueued and before
the watchdog is launched. Three properties are fixed here rather than left to the implementor:

- **The waiting form, so AC7 is a property and not a race.** The statement is
  `SELECT pg_terminate_backend($1, $2)` with `$2` a positive millisecond bound, which returns only
  once the backend is actually gone (§ Approach). The one-argument form reports the signal's delivery
  and not the death; round 2's design read AC7 off it and was wrong.
- **Two bounds, ordered — the SQL wait bound strictly the smaller.** Both are named constants. The
  branch that must fire on a backend that will not die is the **server's own**, which returns `false`
  on a connection that stays usable — not a Go-side context cancellation, which would abort the query
  mid-flight and cost the pooled connection. Placeholders, and they are bounds rather than tuning
  knobs: `2s` for the SQL wait and `5s` for the derived context, against a measured worst case of
  about ten milliseconds (§ Approach). Neither is a configuration key, for the same reason the
  watchdog's close bound is not — each bounds a local cleanup, not a balance value
  [measured 69073e9:internal/scheduler/execute.go:36-42 · `sed -n '36,42p'` →
  `detachedCloseTimeout`'s stated reason]. The wait is paid only on the breach path, which has
  already spent a whole `TaskTimeout` by the time it is reached.
- **`err == nil` is not success.** The call reports both of its non-success shapes — an already-gone
  pid, and an expired wait — as a `false` return with no Go error (§ Approach), so the code branches
  on the boolean. The two are **not** told apart, and need not be: the response to each is the same —
  log it and fall through to the pre-existing layers (the transaction-local
  `idle_in_transaction_session_timeout`, and the watchdog's close), which are unchanged. Reading the
  server's `WARNING` through pgx's per-connection notice hook would distinguish them and is rejected:
  machinery bought for a distinction that changes no behaviour.

Its context is derived with `context.WithoutCancel` plus that named timeout rather than a fresh root:
the terminate must still happen when the deadline context — and possibly the worker's own — is
already done, and `context.Background` is a lint-gated pattern needing a stated reason and a named
owner [measured 69073e9:.golangci.yml:46-51 · `sed -n '46,51p'` → `forbidigo` forbids
`^context\.Background$` and `^context\.TODO$`]. The statement runs on a **pooled** connection, never
the hijacked one, and the task transaction's own `statement_timeout` cannot reach it there: that one
is set transaction-locally [measured 69073e9:internal/scheduler/execute.go:34 · `sed -n '34p'` →
`set_config('statement_timeout', $1, true)`, whose third argument is the is_local flag].

**D5a — The terminate reverses a shipped test's central assertion, and the reversal is the design's
call, not the implementor's.** `TestDeadline_ctxIgnoringHandler_negativeCase` asserts today that the
row **stays locked** while its ctx-ignoring handler runs
[measured 100928c:internal/scheduler/deadline_test.go:759 · `grep -n "want it to stay locked"` →
`t.Fatalf("row became claimable while the ctx-ignoring handler was still running, want it to stay locked")`].
Its fixture keeps the transaction's backend alive by looping over a short `pg_sleep` on a context of
its own [measured 100928c:internal/scheduler/deadline_test.go:57-65 · `sed -n '40,90p'` →
`ctxIgnoringHandler.Execute`'s `for` loop over
`tx.Exec(context.Background(), "SELECT pg_sleep(0.02)")`] — and the breach branch now terminates that
backend, which takes the row's lock with it whether the handler is mid-statement or between
statements, **before `RunOnce` returns**. That fixture alternates between exactly those two states,
and the waiting form was measured against both: `first-probe-misses=0` for a busy holder and for one
idle in transaction (§ Approach). *First probe* is therefore what D5's choice of form buys — under
the one-argument form the inverted assertion would itself have been a flake. That assertion inverts
to *claimable on the first
probe*, and the test's own doc comment, which today promises the row "becomes claimable once it
returns and the watchdog closes the hijacked connection"
[measured 100928c:internal/scheduler/deadline_test.go:709 ·
`grep -n "locked while the handler keeps running"`], is rewritten to what it now asserts. The test
keeps its place as the weaker, already-shipped witness for AC7; the new reclaim test is the
instrumented one (§ Test Design). Its name says `negativeCase` of a defence that no longer has this
residue, so the name moves with the assertion.

**D5b — The lock half of the ownership rule's named exception narrows; the goroutine half survives.**
Rule 4's exception covers *a scheduler handler blocked outside the database after its deadline*, and
its explanatory clause today reads that no mechanism reclaims that goroutine
[measured 100928c:ai-docs/code-style.md:84 · `grep -n "The one named exception is a scheduler handler"` →
ownership rule 4, verbatim]. After this
change the clause is true of a handler blocked **outside** the database only: one blocked **in** it
gets `57P01` and returns, so both its goroutine and its connection are reclaimed
— `[derived → AC8]`. The row's lock leaves the exception entirely, in **both** cases, and that half
is measured rather than reasoned: a backend that is **idle in transaction** — the state a handler
blocked on a Go channel leaves it in — holding `FOR NO KEY UPDATE` is terminated just as a busy one
is, and the row is claimable immediately afterwards
[measured postgres:18.6 (`select version()` → `PostgreSQL 18.6 (Debian 18.6-1.pgdg13+2)`) · holder
session left idle in transaction holding the row → `holder state: idle in transaction`,
`claim-before-terminate: blocked-or-skipped`, `terminate returned: t`,
`claim-after-terminate: got:1`, `backend still in pg_stat_activity: 0`, and the holder's own session
→ `FATAL: terminating connection due to administrator command`], and re-measured in round 3 against
the **waiting** form the design now binds, in the production ordering
[measured postgres:18.6/pgx v5.10.0 · `mode=idle busy-holder=false iters=100
first-probe-misses=0`]. So the row half of the exception
disappears for every handler; only the goroutine half, and only outside the database, survives.
Subtask 10 carries that narrowing; the same falsification is what subtasks 5 and 9 carry in the code
comments and the goroutine-ownership allow list.

**D6 — Both parameters bind as Go `int64`s and the SQL carries no cast.** Postgres resolves
`pg_terminate_backend($1, $2)` with two `int64`s bound and nothing cast in the statement — `$1`
against the `integer` parameter, `$2` against the `bigint` one — which is how every § Approach probe
of the waiting form bound it. The wait bound reaches `$2` as its `time.Duration`'s millisecond count,
already an `int64`. For `$1` the narrowing conversion the alternative would need trips the security
linter while the widening one is clean [measured 69073e9:.golangci.yml:31 · `sed -n '31p'` →
`    - gosec`; and, under this repo's own config, `golangci-lint run` over a scratch package holding
both conversions → `G115: integer overflow conversion uint32 -> int32 (gosec)` on the `int32` one and
no finding on the `int64` one].

**D7 — A nil `Logger` is accepted and replaced with a discard handler; it is not a refused option.**
Both packages already treat a nil `Observer` that way [measured 69073e9:internal/scheduler/worker.go:47-49
· `sed -n '47,49p'` → *"A nil Observer is checked, not called"*], every existing test constructs a
`Worker`/`Loop` without one, and the discard handler is what this module's own tests already pass
where a logger is mandatory [measured 69073e9:internal/scheduler/scheduler_test.go:37 · `sed -n '37p'`
→ `store.Migrate(ctx, pool, slog.New(slog.DiscardHandler))`].

**D8 — The log record is emitted at the recovery point, on the handler's own goroutine.** Not at
settlement: an attempt does not always leave the row an outcome of its own, and the log is then the
only surface its stack can reach. The amended spec's row clause is satisfied on every path that does
settle; these are the paths it does not, and each is why the emission point is the recovery point:

- a scheduler handler that panics *after* its deadline was already breached — the pending settlement
  the breach enqueued records the breach, and this goroutine is an orphan with no settlement of its
  own;
- an ingest panic a later attempt supersedes — ingest persists a row only when every attempt is
  spent, and the give-up row's `last_error` is the **last** attempt's error
  [measured 100928c:internal/ingest/settle.go:64-66 · `sed -n '63,66p'` → `lastErrText := ""` /
  `if lastErr != nil {` / `lastErrText = lastErr.Error()`, inside `settleGivenUp`],
  so a panic on any earlier attempt — whether a later one succeeds or a later one fails differently —
  leaves the row nothing of its own.

The record carries the stack as its own attribute plus the task/update identity, at error level.

The scheduler's recovery point sits inside `runHandlerWithSavepoint`, which is a package-level
function with no `*Worker` receiver [measured 100928c:internal/scheduler/execute.go:232 ·
`grep -n "func runHandlerWithSavepoint"` → `func runHandlerWithSavepoint(ctx context.Context, tx pgx.Tx, handler Handler, task Task) (outcome Outcome, handlerErr, releaseErr error)`],
so the logger reaches the emission site as an added **parameter** on that function, passed by
`executeOne` from the `Worker` field subtask 2 adds. It is not reached by giving the function a
receiver, and not by a package-level logger variable.

**D9 — No new configuration key, no balance number, and no event-dictionary row.** Nothing this
design adds is a tuning value in the sense `docs/DESIGN.md` §16.5 reserves for balance; the only
numbers are the terminate's local timeout bound (D5) and the failure-label spelling (D2). The
telemetry AXIOM's two halves are discharged asymmetrically and deliberately: the **metrics** half is
the `failure`-label value (D2, AC4) with its propagation (AC5), and the **events** half is nothing —
the issue's *Telemetry obligation* states *"Events: none — the event dictionary holds gameplay events
only"*, and a scheduler handler panic is infrastructure, not a gameplay event
[measured 100928c:ai-docs/plans/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md.state.md:63
· `grep -n "Events: none"` → the issue body's Telemetry obligation, verbatim]. Recorded here so the
AXIOM is visibly discharged rather than silently skipped. This change moves no balance, so it has no
posting signature to declare.

**D10 — No schema change, therefore no forward migration.** The stack rides the existing
`last_error` text column in both packages' existing give-up/settlement statements, and no persisted
enum, column or payload key is renamed, re-numbered or repurposed. Rows written before this change
parse unchanged, and rows written after differ only in the free text of a column whose only readers
are the operator-facing projections.

**D11 — Giving each package a logger falsifies a class of doc comment, and the class is corrected
with the addition, not after it.** Both packages document a design choice by its *absence*: an
observation field exists so an error is not silently dropped *"since this package has no logger"*
[measured 36ef3e1:internal/scheduler/worker.go:115, internal/scheduler/observe.go:64,
internal/ingest/observe.go:115 · `grep -n "no logger" internal/scheduler/worker.go
internal/scheduler/observe.go internal/ingest/observe.go` → `worker.go:115: as well as returned,
since this package has no logger and`, `observe.go:64: // this package has no logger.` on
`LoopObservation.Err`, and `ingest/observe.go:115: // dropped, since this package has no logger.` on
ingest's `LoopObservation.Err`]. `Options.Logger` makes each of those sentences
false, so the subtask that adds the field to a package also corrects that package's clauses — the
sites are named in the Decomposition so they are inside the implementor's file lists rather than
outside its scope. The fields themselves stay: an observation is the `internal/health` adapter's
input, which a log record does not replace. This is the same class subtask 5 handles for the
terminate's four falsified comments, and the enumeration is a floor: the sweep in § Risks covers it,
and any further site the sweep finds is in scope.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | New shared primitive: the recovered-panic type (value + captured stack), its constructor taking a `recover()` result, its `error` rendering, package comment, tests, and the package's leak-check `TestMain` | `internal/panicguard/panicguard.go`, `internal/panicguard/panicguard_test.go`, `internal/panicguard/main_test.go` | — |
| 2 | `scheduler.Options` gains `Logger *slog.Logger`; `Worker` carries it; `New` substitutes a discard handler for nil (D7). Correct the doc comment the addition falsifies: `RunOnce`'s *"since this package has no logger"* clause (D11) | `internal/scheduler/worker.go`, `internal/scheduler/worker_test.go` | — |
| 3 | `FailureKind` gains the panic member with its doc comment; `FailureRolledBack`'s doc gains the clause that excludes a panic from it (D2, D3). Correct `LoopObservation.Err`'s *"since this package has no logger"* clause, falsified by subtask 2 (D11) | `internal/scheduler/observe.go`, `internal/scheduler/observe_test.go` | — |
| 4 | Handler-boundary recovery and its settlement: recover around `Handler.Execute` only; thread the logger into `runHandlerWithSavepoint` as a parameter so the emission site can reach it; carry the recovered panic on the handler result; classify it first in the settlement switch; route an unusable transaction into the pending-settlement set with the panic failure kind and the rendered panic as its reason; emit the log record at the boundary (D1, D3, D4, D8) | `internal/scheduler/execute.go`, `internal/scheduler/settle.go`, `internal/scheduler/panic_test.go` | 1, 2, 3 |
| 5 | Deadline reclaim: read the backend PID on the idle pooled connection; terminate the breached backend from the pool after the breach with the **waiting** two-argument form under the ordered bounds D5 fixes; log a terminate the **boolean** reports unsuccessful; **reverse `TestDeadline_ctxIgnoringHandler_negativeCase`'s row-stays-locked assertion to claimable-on-the-first-probe** and rewrite that test's doc comment and name (D5a); correct every doc comment whose claim the terminate falsifies — the timeouts constant's *"one of two things"* lock-release sentence, `Handler`'s *"its row stays locked until the handler returns"* clause, the watchdog launch's `//nolint` reason claiming the close is *what finally releases the row's lock*, and `waitLockFree`'s doc comment enumerating the two pre-existing release mechanisms (D5, D5a, D6) | `internal/scheduler/execute.go`, `internal/scheduler/task.go`, `internal/scheduler/deadline_test.go`, `internal/scheduler/reclaim_test.go` | 2 |
| 6 | Update ingestion: `Options` gains `Logger`; the recovery helper returns the shared recovered-panic value instead of a bare `panicked` flag; its rendering carries the stack into the give-up row's `last_error`, and a log record carries it as an attribute. The reported outcome is unchanged. Correct `LoopObservation.Err`'s *"since this package has no logger"* clause, falsified by the same addition (D1, D7, D8, D11) | `internal/ingest/loop.go`, `internal/ingest/attempt.go`, `internal/ingest/observe.go`, `internal/ingest/retry_test.go`, `internal/ingest/loop_test.go` | 1 |
| 7 | Health: the failure-label mapper gains the panic case; the label unit test, the observer's failure-label test and the closed-set label guard's driver and expectation all take the new value | `internal/health/labels.go`, `internal/health/labels_test.go`, `internal/health/scheduler_test.go`, `internal/health/guards_test.go` | 3 |
| 8 | Composition root: thread the process logger into both constructors | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go` | 2, 6 |
| 9 | Goroutine-ownership allow list: the handler launch's panic answer stops saying the recovery is follow-up work, and the watchdog launch's stop answer records that a handler blocked in the database is now bounded by the terminate | `internal/gateguard/guard_test.go` | 4, 5 |
| 10 | Documentation propagation (AC5): the scheduler failure-value enumeration; ownership rule 4's explanatory clause, which narrows to a handler blocked *outside* the database — the row's lock leaves the exception entirely (D5b); the detached-launch paragraph that currently points at this issue as unfinished work; the `internal/` layout enumeration, which gains the shared recovered-panic primitive subtask 1 adds | `ai-docs/alert-contract.md`, `ai-docs/code-style.md`, `ai-docs/process-lifecycle.md`, `ai-docs/context.md` | 1, 3, 5 |

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
  goroutine did afterwards. This is no longer a deviation to argue: the round-3 amendment narrowed
  AC6 to *the log record in every case, the row wherever the attempt leaves the row an outcome*, and
  this path is one of the two the narrowing was written for. Mitigation: the log record is emitted at
  the recovery point on the handler's own goroutine (D8), so the stack is never lost, and § Test
  Design gives the path its own witness rather than leaving it argued —
  `[derived → the post-breach panic scenario under AC6 in § Test Design]`.
- **The terminate can report `false`, and then AC7's guarantee falls back to the pre-existing
  layers.** A saturated pool, a closing pool on shutdown, a backend already gone, or a backend that
  outlives the wait bound all end with no confirmed death. Mitigation: the outcome is read off the
  **boolean**, not the error — both non-success shapes return `err == nil`, so a code path that
  checked only the error would never log anything (D5) — and the pre-existing layers, the
  transaction-local `idle_in_transaction_session_timeout` and the watchdog's close of the hijacked
  connection, are left in place unchanged, so the behaviour degrades to today's rather than to
  nothing. Terminating an already-dead pid stays harmless [measured postgres:18.6/pgx v5.10.0 ·
  probe → `gone-pid two-arg: returned=false err=<nil>`].
- **The waiting terminate blocks `executeOne` on the breach path.** The call returns only once the
  backend is gone, so the breach branch pays that wait before returning. Measured worst case in the
  production ordering is about ten milliseconds (§ Approach), on a branch that has already spent a
  whole `TaskTimeout`. Mitigation: the SQL wait bound caps it, and D5 orders that bound strictly
  below the derived context's timeout so a wait that will not finish is ended by the server — which
  leaves the pooled connection usable — rather than by a Go-side cancellation, which would abort the
  query mid-flight and cost the connection.
- **The terminate turns a shipped, green test red by design, and a subtask that only adds files would
  leave it red.** `TestDeadline_ctxIgnoringHandler_negativeCase` asserts the pre-change outcome
  directly [measured 100928c:internal/scheduler/deadline_test.go:759 ·
  `grep -n "want it to stay locked"`], so its inversion is part of subtask 5's contract rather than
  an unexpected failure to diagnose mid-implementation. Mitigation: D5a fixes the new assertion, and
  subtask 5's file list names `deadline_test.go` so the edit is in the group's scope from the start.
  The other `waitLockFree` callers are expected to keep passing: the helper polls until the row can
  be locked and fails only on its own timeout, so an earlier release satisfies it
  [measured 100928c:internal/scheduler/deadline_test.go:76-101 · `sed -n '68,101p'` → `waitLockFree`
  loops on a `FOR NO KEY UPDATE SKIP LOCKED` probe until its patience budget expires] — asserted by
  running the package rather than assumed, `[derived → the package suite re-run at subtask 5]`. And
  `TestDeadline_drainDoesNotBlockOnLockedRow` holds its
  lock in a transaction of the test's own, not through a breach, so no terminate reaches it
  [measured 100928c:internal/scheduler/deadline_test.go:559-596 · `sed -n '540,630p'` → the test
  begins its own `holder` transaction and takes `FOR NO KEY UPDATE` on the row itself].
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
  alert-contract enumeration *does not bound the class*. The sites subtasks 2, 3, 5, 6, 7, 9 and 10
  name are what a sweep at this commit found: `ai-docs/alert-contract.md` § Scheduler's `failure`
  value enumeration, the label mapper and its tests, the closed-set label guard's driver and
  expectation, the `FailureKind` enum's own doc comments, the goroutine-ownership allow-list row that
  today calls this recovery follow-up work, the doc comments the terminate falsifies (subtask 5), the
  *"this package has no logger"* clauses the new `Options.Logger` falsifies (D11), and
  `ai-docs/context.md`'s `internal/` layout enumeration, which does not yet name the package subtask 1
  adds. The implementor re-runs the sweep (`grep -rni` over `.claude/`,
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
  (the unusable-transaction case of D4); a handler that blocks on a test-controlled channel until
  after its deadline is breached and then panics (the log-only surface case); a recording
  `slog.Handler` installed through the new `Options.Logger`, collecting records behind a mutex.
- Scenarios:
  - AC1 — after the panicking task, the same worker's next cycle claims and runs a second due task,
    and neither `RunOnce` call returns an error — `[derived → AC1]`.
  - AC2 — a one-shot whose handler panics on every attempt reaches the `dead` state within the
    configured attempt cap, mirroring the existing deadline give-up test's shape — `[derived → AC2]`.
  - AC3 — the observation for the panicking attempt carries the failed outcome and the panic failure
    kind, and a sibling case with a handler that *returns* an error carries the handler failure kind,
    so the two are asserted apart rather than one in isolation — `[derived → AC3]`.
  - AC6 (row surface) — the task row's `last_error` contains both the panic value and a frame naming
    the panicking fixture, and the recording logger holds a record whose stack attribute contains the
    same frame — `[derived → AC6]`.
  - AC6 (log-only surface) — the narrowed clause gets its own witness rather than an argument: a
    handler that blocks past its deadline and **then** panics, released by the test after `RunOnce`
    has returned. The recording logger holds a record whose stack attribute names the panicking
    fixture, while the row — after the next cycle's drain — records the deadline, not the panic.
    Asserting both halves is what makes this a test of the amended rule rather than of the log alone
    — `[derived → AC6]`.
  - D3 — a handler that panics **and** whose savepoint rollback succeeds still classifies as a panic,
    not as rolled-back — `[derived → AC3]`.
  - D4 — the rows-left-open handler, asserted on each of the values D4 fixes: the attempt is
    still counted (the row's failure count rises after the next cycle's drain), the row becomes
    claimable — **through the package's existing lock-free poll helper**, since this claimability
    rides `pgxpool`'s destroy-on-release of a busy connection, which puddle performs on its own
    goroutine, and is therefore an eventual property rather than AC7's instantaneous one — and
    `RunOnce` returns no error — `[derived → AC2]`; the observation for that attempt
    carries the panic failure kind, not the deadline or rolled-back one — `[derived → AC3]`; and the
    `last_error` the drain writes contains the panic value and a frame naming the fixture, which is
    the only route the stack has to the row on this branch — `[derived → AC6]`.
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
  - AC7 — immediately after `RunOnce` returns, a `FOR NO KEY UPDATE SKIP LOCKED` probe on the
    breached row succeeds on its **first** attempt, with no polling loop. The assertion is a
    **pair**, not a single read: the same check reads the fixture's own still-inside-`Execute`
    marker, so the claim's success is recorded alongside proof that the handler had not returned.
    Without that half the test asserts *claimable*, which the pre-change tree also reaches once the
    handler returns; AC7's clause is *claimable **without waiting for** the handler to return*, and
    only the pair says it — `[derived → AC7]`.
  - AC8 — the AC8 fixture's `Execute` returns within a bound; the error it recorded is the
    terminate's own class rather than a context cancellation; and the backend it recorded is gone
    from `pg_stat_activity`, which is the connection-released assertion in a form the test can make
    exactly — `[derived → AC8]`.
  - The observation still carries the deadline failure kind, unchanged — `[derived → AC7]`.
- **Polling, and where it is banned.** The AC7 claim probe must succeed on its **first** attempt with
  no polling loop — and after round 3 that is a property the mechanism genuinely has rather than a
  race the test would lose: the waiting form of the terminate returns only once the backend is gone
  (§ Approach, D5), measured `first-probe-misses=0` across four hundred iterations in the production
  ordering. The ban is **not** there because a bounded poll would fail to discriminate — against this
  fixture it would, since neither pre-existing layer reaches a backend that is actively sleeping
  (§ Approach's rejected-alternatives row for option (b)). It is there because AC7's words are
  *without waiting for its handler to return*, and the single read is the assertion that says exactly
  that; a poll would put the discrimination in the fixture's neutralisation and ceiling instead of in
  the mechanism — `[derived → AC7]`.
- **The claimer's connection is established before `RunOnce` is called**, never opened after the
  breach. Connecting costs about a millisecond, and a millisecond is enough slack to hide a failure
  of exactly the kind round 3 caught; a probe that connects afterwards is a green instrument rather
  than a passing subject, and that sensitivity is what would make this test pass locally and fail
  under `make test-contention` — `[derived → AC7]`.
- The `pg_stat_activity` assertion is the opposite and deliberately so. It came back clean on the
  first read in every round-3 iteration (§ Approach), but what this design owns is the call's return
  contract, not the server's bookkeeping latency under someone else's load, so that assertion
  **polls within a generous bound**: a patience budget in the shape the package's existing lock-free
  helper already uses, never a fixed sleep and never a single immediate read — `[derived → AC8]`.
- Fixtures/helpers: the existing lock-free poll helper stays for the tests that legitimately wait —
  D4's claimability assertion in the panic suite is one of them, and it is **not** the same shape as
  AC7's: that one rides `pgxpool`'s destroy-on-release of a busy connection, which puddle performs on
  its own goroutine, so it is an eventual property and polls. The AC7 assertion deliberately does
  **not** use the helper. Each test releases its fixture on cleanup so the package's leak check ends
  clean.
- **The shipped negative case moves with the mechanism** (D5a): `TestDeadline_ctxIgnoringHandler_negativeCase`'s
  row-stays-locked assertion is rewritten to claimable-on-the-first-probe, and its doc comment and
  name follow. For AC7 it is a sound witness — the row's lock dies with the backend in both of the
  states that fixture alternates between, each measured at `first-probe-misses=0` under the waiting
  form (§ Approach) — but it witnesses **nothing**
  about AC8: its loop discards the error each `tx.Exec` returns and keeps spinning until the test
  closes its release channel (D5a's measurement of the fixture), so it never demonstrates a handler
  *returning* on the terminate.
  That is what the instrumented fixture above is for, and why it stays the primary one —
  `[derived → AC7]`.

**Update ingestion** (`internal/ingest`, database-backed)
- Entry point: the loop's attempt path with a route whose handler panics.
- Scenarios:
  - AC9 (row surface) — after every attempt panics, the give-up row's `last_error` contains the panic
    value and a frame naming the panicking fixture, and the recording logger holds a record per
    recovered panic whose stack attribute contains the same frame — `[derived → AC9]`.
  - AC9 (log-only surface) — the narrowed clause's ingest half, with its own witness: a route whose
    handler panics on its first attempt and succeeds on a later one. No give-up row is written at
    all, and the recording logger still holds a record whose stack attribute names the fixture.
    Asserting the row's absence alongside the record is what makes this a test of the amended rule
    — `[derived → AC9]`.
  - AC9 (unchanged outcome) — the per-attempt observations still report the panic outcome for each
    attempt and the give-up outcome once, in the counts the existing recovery test already pins, so
    the reported outcome is asserted unchanged rather than assumed — `[derived → AC9]`.
- Fixtures: the package's existing always-panicking handler; a handler that panics once and then
  succeeds; plus the same recording `slog.Handler` shape the scheduler tests use.

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

- None. Round 1's one open question — whether AC6/AC9's "both surfaces" bound a row that some
  attempts physically never leave — went to the owner and was answered by amending the spec: Scope
  item 3, the `Where does a handler panic's stack land?` Key decisions row, AC6 and AC9 now read *the
  log record in every case, the row wherever the attempt leaves the row an outcome*, and the owner
  confirmed that four-row reach in a second round-3 answer
  [measured 100928c:ai-docs/plans/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md.state.md:84-89
  · `sed -n '84,89p'` → the two round-3 `prior_qa` entries, *"Amend the spec"* and *"Keep all four"*].
  This design is reconciled to the amended pair: the narrowed clause is stated in § Approach, its
  emission point is D8, and each of its two log-only paths now carries its own test rather than an
  argument (§ Test Design). The ingest reading round 1 flagged — the give-up row is where ingestion
  persists an error, and a panic a later attempt supersedes persists none — is what the amendment
  ratified, so nothing is left assumed
  [measured 69073e9:internal/ingest/observe.go:33-37 · `sed -n '33,37p'` → *"Reported distinctly from
  OutcomeFailed so a recovered panic that a later attempt fixes is never erased"*].
