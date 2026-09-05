# Design: Postgres task scheduler — `scheduled_task`, the `SKIP LOCKED` worker, one-shot and recurrent tasks

**Issue:** #20
**Date:** 2026-09-05

> **Claim-tag conventions in this document.** A repo fact carries the commit of the **read**
> (`38630d2`), not of this document, so a tag may lag `HEAD` after a revision that touched only
> this file. A fact about **PostgreSQL's own behaviour** has no repo path, so its pin is the
> server version the probe ran against (`postgres:18.6`, a throwaway container started from the
> image already on this machine and stopped afterwards). A fact about an **external module** is
> pinned by its version. A claim about an artefact this task has **not yet built** carries
> `[derived → …]` and no locator. `docs/DESIGN.md` is cited by section per the design-writer
> contract, so those citations carry no `[measured …]` tag.

---

## Approach

`internal/scheduler` is a worker over one table. Its whole shape falls out of the decisions
immediately below, and every later section is a consequence of one of them.

**One transaction per claim batch, one savepoint per task.** `docs/DESIGN.md` §11 fixes both
the claim query (`WHERE run_at <= now() FOR UPDATE SKIP LOCKED LIMIT N`) and the exactly-once
argument (a task executes in one transaction with its own effects). Those two sentences are
in tension the moment the limit exceeds one: the `SKIP LOCKED` claim's row locks live only as
long as the transaction that took them, so a batch cannot be claimed in one transaction and
executed in another without a lease column — and a lease column is exactly the liveness
machinery AC31 forbids. The resolution is a **savepoint per task inside the batch's
transaction**: each handler runs inside its own subtransaction, so its effects and its
settlement are atomic *relative to each other* and a failure rolls back that task alone. That
is the property §11 is asking for; nothing in §11 asks that a task be the only thing in its
transaction.

The mechanism was executed before it was written down. A subtransaction aborted by a
constraint violation is recoverable, and the outer transaction still commits
[measured postgres:18.6 · `BEGIN; SAVEPOINT h; <duplicate INSERT>; SELECT …; ROLLBACK TO SAVEPOINT h; SELECT count(*); COMMIT` →
`ERROR: duplicate key value violates unique constraint "scheduled_task_identity_key"` /
`ERROR: current transaction is aborted, commands ignored until end of transaction block` /
`ROLLBACK` / `probe6_after_rollback_to_savepoint | 3` / `COMMIT`]. pgx exposes it as a
first-class API: `pgx.Tx.Begin` opens a pseudo-nested transaction on a savepoint, `Commit`
releases it and `Rollback` rolls back to it
[measured pgx/v5@v5.10.0:tx.go · `sed -n '/func (tx \*dbTx) Begin/,/^}/p' tx.go` → `tx.conn.Exec(ctx, "savepoint sp_"+…)`;
`grep -n 'release savepoint\|rollback to savepoint' tx.go` → `"release savepoint sp_"+…` / `"rollback to savepoint sp_"+…`].

**The database is the only clock.** No production path in this package reads `time.Now`.
Due-ness is `run_at <= now()`, evaluated by the server; the execution instant is a
`clock_timestamp()` the server returns; backoff and cadence are **pure functions of instants
the database supplied**. This is not stylistic. `now()` is `transaction_timestamp()` and is
frozen for the whole batch, while `clock_timestamp()` advances inside it
[measured postgres:18.6 · `BEGIN; SELECT now(), clock_timestamp(); SELECT pg_sleep(0.4); SELECT now(), clock_timestamp(); COMMIT` →
`now_fn` identical (`…29.486061+00` twice), `clock_fn` `…29.486284+00` then `…29.887163+00`],
so mixing a Go instant into a comparison against either is a silent skew bug — and the lag
metric this whole task exists to produce is exactly such a comparison. It also settles the
KD-26 question before it is asked: the package has no clock to fake, so no scheduler test
needs a `synctest` bubble around a real socket, and KD-26's binding half ("production code
carries no clock abstraction") is satisfied more strongly than by an injectable clock
[measured 38630d2:ai-docs/key-decisions.md:71 · `grep -n 'KD-26 ' ai-docs/key-decisions.md | cut -c1-90` →
``71:**KD-26 — Tests use `testing/synctest`; production code carries no clock abstraction.**``].
The only Go-time values left are *durations* — the poll interval and the per-task execution
deadline — neither of which is ever compared against a persisted instant.

**A task type is a Go registry entry, not a schema value.** The handler registry is
constructed once and is immutable; it maps a task type to its handler and, for a recurrence,
to the cadence that computes the next `run_at` (owner, round 3). The database stores the type
as `text` and validates nothing about it, because the authority is the registry — see D6,
which argues that departure from this repository's every-categorical-column-is-an-enum
precedent rather than assuming it.

**Rejected — a lease/`picked` column with a heartbeat.** It is the standard shape for job
runners that execute *outside* the claiming transaction, and this project specifies the
opposite (§11). With the handler inside the transaction there is no stranded state to
revive: an aborted transaction reverts its writes and drops its row locks, leaving the row
still due. AC31 asserts that property directly. The one failure this does not self-heal is a
handler that hangs rather than crashes, and D11 bounds it with a timeout rather than a
protocol.

**Rejected — one transaction per task with `LIMIT 1`.** It reads closer to Scope 4's phrasing
and needs no savepoints, but it contradicts AC5 ("one claim returns at most that many rows")
and leaves AC12's batch-size observation with nothing to report. It also multiplies the claim
query by the batch size for no gain.

**Rejected — computing backoff and the next cadence instant in SQL.** `run_at = now() + …`
keeps everything server-side, but it makes the growth rule an expression buried in an UPDATE
that can only be tested through the database. Computing them in Go from DB-supplied instants
gives two pure functions with exact table tests and one statement that merely writes the
answer.

---

### D1 — Package `internal/scheduler`: surface, files, and what it does not import

The name and the headline types are fixed by the spec: `scheduler.Worker`,
`scheduler.Handler`, `scheduler.Task`. Around them:

| Symbol | Role |
|---|---|
| `Type` | the persisted task-type name (`type Type string`) |
| `Task` | one claimed row handed to a handler: id, type, instance key, payload, `RunAt`, consecutive-failure count |
| `Handler` | consumer-declared one-method interface: `Execute(ctx, tx pgx.Tx, task Task) (Outcome, error)` |
| `Outcome` | `OutcomeDone` · `OutcomeNoop` · `OutcomeFailed` (the worker produces the last from a non-nil error) |
| `Cadence` | `func(prev, now time.Time) time.Time` — the next occurrence's instant |
| `Every` | the one shipped `Cadence` constructor: a fixed period |
| `Recurrence` | a cadence plus the configuration key its value came from |
| `Declaration` | one registry entry: type, handler, optional recurrence |
| `Registry` | the immutable declaration set; also the insertion surface (`Schedule`) |
| `Request` | what `Schedule` takes: type, instance key, payload, delay |
| `Worker` | claim → execute → settle, plus `Run`, `RunOnce`, `Reconcile` |
| `Options` | pool, registry, `config.Scheduler`, optional `Observer` |
| `Observation` / `LoopObservation` / `Observer` / `FailureKind` | the observation seam (D12) |
| `DeadTask` / `DeadTasks` | AC10's enumeration of give-up rows |

Files: `doc.go` (package comment), `task.go`, `registry.go`, `cadence.go`, `schedule.go`,
`worker.go`, `execute.go`, `claim.go`, `reconcile.go`, `observe.go`, `errors.go` — small and
one concern each, because the gated hard limit is 1000 lines for a non-test file and 1500 for
a `_test.go`
[measured 38630d2:Makefile:23-25 · `sed -n '23,25p' Makefile` → `GO_MAX_LINES ?= 1000` / `GO_MAX_TEST_LINES ?= 1500`].

Every method that reaches the database takes `ctx context.Context` first and no type in the
package has a `context.Context` field (AC1). Every exported item carries a doc comment
starting with its name and the package carries a package comment, because `revive`'s
`exported` and `package-comments` rules are enabled
[measured 38630d2:.golangci.yml:45-48 · `sed -n '45,48p' .golangci.yml` → `revive:` / `rules:` / `- name: exported` / `- name: package-comments`].

**What it does not import.** No metrics-registry package (AC13) — the seam is a plain struct
plus a consumer-declared interface, the precedent `internal/tg` already set
[measured 38630d2:internal/tg/observe.go:27-33 · `sed -n '27,33p' internal/tg/observe.go` →
`type Observer interface {` / `ObserveCall(Observation)` / `}`]. And **no non-test file
imports `internal/store`**: the scheduler never writes a basis document, because a basis
document exists to anchor postings and the code that posts is the handler (spec *Key
decisions*). `internal/store` therefore never appears in this package's non-test dependency
set, and there is no direction in which a cycle could form.

---

### D2 — The batch transaction, step by step

One `RunOnce` cycle, on one pooled connection:

1. `BEGIN`.
2. `SET LOCAL statement_timeout` and `SET LOCAL idle_in_transaction_session_timeout`, both to
   the configured per-task deadline (D11).
3. The claim (D4). Zero rows → `COMMIT`, report the loop observation, done.
4. For each claimed row, in `run_at` order:
   a. `SELECT clock_timestamp()` — the task's **execution instant**, the single value from
      which its lag, its retry instant and its next cadence instant are all computed.
   b. `SAVEPOINT` (`tx.Begin`).
   c. Run the handler on a separate goroutine with a deadline-bearing context, and wait for
      either its result or the deadline (D11).
   d. **Done** → release the savepoint, then settle (D7). **No-op** → *roll back to* the
      savepoint, then settle. **Failed** → roll back to the savepoint, then record the
      failure (D7). **Deadline breached** → abandon the batch (D11).
   e. Collect the task's observation; do not emit it yet.
5. `COMMIT`, then emit every collected observation, then the loop observation.

**Why the no-op path rolls back to its savepoint.** AC8 requires a guard-miss no-op to leave
no writes; AC25 requires the deletion to happen in the same transaction as the effects. Only
the savepoint satisfies both at once — rolling the *whole* transaction back would move the
deletion out of it, and committing without a rollback would make "its writes are absent" a
contract the worker asks handlers to honour rather than a property it enforces. With the
savepoint it is enforced: a handler that writes and then reports a no-op leaves nothing
behind, and the row is still settled in the same commit.

**Why observations are emitted after `COMMIT`.** An observation emitted before the commit is
a claim about work that may still roll back — the "green instrument" failure in its purest
form. If the commit fails, every collected observation is emitted as a failure with
`FailureRolledBack` instead of the outcome the handler reported.

**Consequence, stated rather than discovered later.** A batch commits as one unit, so a
worker crash — or a deadline abandonment — discards every uncommitted task in that batch.
Nothing was committed, so exactly-once holds and every affected row is still due; the cost is
repeated work, and the lever that bounds it is the claim limit.

---

### D3 — The clock, and where each instant comes from

| Value | Source | Used for |
|---|---|---|
| due-ness | `now()` in the claim's `WHERE` | which rows are claimable |
| execution instant | `clock_timestamp()`, one `SELECT` per task | lag, retry instant, next cadence instant |
| `run_at` on insert | `clock_timestamp() + $delay` in the INSERT | a mechanic schedules a *delay*, never an absolute instant |
| `run_at` on reconcile | a `Cadence` result computed from a DB-supplied instant | seed and correction |
| poll interval, execution deadline | Go `time.Duration` from configuration | never compared against a persisted instant |

`Request` therefore carries `Delay time.Duration`, not `RunAt time.Time`: "a wave in five
minutes" is what §3.5's timer edges actually express, and a relative delay keeps the caller
out of the clock business entirely. A negative delay is refused (`ErrInvalidDelay`); zero
means due immediately.

---

### D4 — The claim query, its index, and what a priority column would change (AC16)

```sql
SELECT id, type, instance_key, payload, run_at, consecutive_failures
FROM scheduled_task
WHERE state = 'pending' AND run_at <= now()
ORDER BY run_at, id
FOR UPDATE SKIP LOCKED
LIMIT $1
```

served by

```sql
CREATE INDEX scheduled_task_due_idx ON scheduled_task (run_at) WHERE state = 'pending';
```

The `LIMIT` is applied **after** rows locked by another transaction are skipped, which is what
makes a small limit safe under concurrency
[measured postgres:18.6 · with the two earliest due rows locked by an open transaction,
`SELECT id … ORDER BY run_at, id FOR UPDATE SKIP LOCKED LIMIT 1` → `5` (the third row), not zero rows].
A claim that meets rows held by another worker skips them rather than waiting
[measured postgres:18.6 · same fixture, `LIMIT 10` → one row, the unlocked one].

**AC16, stated as the criterion asks.** Adding the deferred priority/queue column later is
**a column, an index and an ordering**: `ALTER TABLE scheduled_task ADD COLUMN priority
smallint NOT NULL DEFAULT 0`, a new partial index `(priority, run_at) WHERE state =
'pending'`, and `ORDER BY priority, run_at, id` in the query above. No second table, no Go
type rename, no data backfill (the default fills existing rows). Nothing in this change is
shaped against that: `Task` and `Request` are structs with named fields, so a field is
additive; the claim is one statement in one file; and separate worker pools are separate
`Worker` values over the same table, which the type already permits.

---

### D5 — `scheduled_task`: the columns, and the ones deliberately absent

```sql
CREATE TYPE scheduled_task_state AS ENUM ('pending', 'dead');

CREATE TABLE scheduled_task (
    id                   bigint               GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type                 text                 NOT NULL,
    instance_key         text,
    payload              jsonb                NOT NULL,
    run_at               timestamptz          NOT NULL,
    state                scheduled_task_state NOT NULL DEFAULT 'pending',
    consecutive_failures integer              NOT NULL DEFAULT 0,
    last_error           text,
    created_at           timestamptz          NOT NULL DEFAULT now(),
    CONSTRAINT scheduled_task_type_nonempty CHECK (type <> ''),
    CONSTRAINT scheduled_task_failures_nonnegative CHECK (consecutive_failures >= 0)
);
CREATE UNIQUE INDEX scheduled_task_identity_key ON scheduled_task (type, instance_key) WHERE instance_key IS NOT NULL;
CREATE INDEX        scheduled_task_due_idx      ON scheduled_task (run_at)             WHERE state = 'pending';
```

**No `completed` state, no `executed_at`** — a completed one-shot row is deleted (owner,
round 1), so there is nothing for either to describe (AC25). **No liveness machinery** — no
column recording that a task is executing, no heartbeat, no revival path (AC31). **No
`kind` column**: whether a type is one-shot or recurrent is the registry's answer, and
duplicating it on the row would create a second source of truth that a deploy could
contradict.

**One counter, not two.** A one-shot never survives a success — it is deleted — so its
attempt count and its consecutive-failure count are the same number, and AC10's "attempt
count" reads `consecutive_failures` directly. A recurrent row's *lifetime* execution count is
deliberately not kept: it would be a history column on a table the owner's answer stripped of
history.

**The identity constraint is partial, and that is what keeps one-shots free.** A recurrence
carries an instance key (the seeder supplies the declaration's, defaulting to the type name),
so a second insert of the same recurrence is refused by the index rather than by a code path
that looks first
[measured postgres:18.6 · with the row present, a plain `INSERT` of the same `(type, instance_key)` →
`ERROR: duplicate key value violates unique constraint "scheduled_task_identity_key"`;
the same insert with `ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL DO NOTHING` →
`INSERT 0 0`, leaving `count = 1`]. A one-shot with no natural identity passes `instance_key`
empty, which is stored `NULL` (`NULLIF($2, '')`) and falls outside the index's predicate, so
two such one-shots of the same type coexist
[measured postgres:18.6 · two inserts of `('one', NULL, …)` → `probe3_null_instance_rows | 2`].
That is AC29 in both directions.

**Payload is `jsonb`, and `jsonb` normalises.** Key order is sorted, whitespace is dropped and
a duplicate key keeps the last value
[measured postgres:18.6 · `SELECT '{"b":1,"a":{"c":null},"b":2}'::jsonb` → `{"a": {"c": null}, "b": 2}`].
So AC24's round-trip is a statement about the payload's *value*, and its test compares decoded
values, never raw bytes — a byte-comparison would be asserting a property the storage type
does not have.

**A payload key is a data contract.** `AGENTS.md`'s API-stability carve-out names the
scheduler's payloads by that word among the live data that outlives every deploy
[measured 38630d2:AGENTS.md:84 · `grep -n "scheduler's .scheduled_tasks. payloads" AGENTS.md` →
one match, at `AGENTS.md:84`, the `> **CARVE-OUT — data contracts are the opposite…**` paragraph].
The package doc comment says so in the same words, and the payload keys this change ships are
named in § Test Design (they belong to test-only types; no production type ships here).

---

### D6 — The task type is `text`, not a PostgreSQL enum — argued, not assumed

The precedent points the other way: every categorical column in migration 00001 is a
PostgreSQL enum
[measured 38630d2:internal/store/migrations/00001_ledger_core.sql:2-4 · `sed -n '2,4p' …/00001_ledger_core.sql` →
`CREATE TYPE owner_kind AS ENUM ('world', 'player', 'chat');` / `ledger_kind` / `operation_source`],
and the spec's *Key decisions* names that precedent as the default. The facts that overturn it
for **this** column, and only this one — `state` stays an enum, because its value set is the
schema's own.

1. **The authoritative registry is in Go, by the owner's round-3 answer.** A recurrence's
   cadence is declared in the handler registry; a type with no declaration must be refused at
   runtime whatever the column's type is (AC21). An enum would therefore be a *second*
   registry that a deploy can contradict — the schema saying a type exists while no handler
   claims it — and keeping the two in step costs a migration per mechanic for no property
   gained.
2. **This task ships no production task type** (spec *Out of scope*), so the enum would ship
   with no legitimate member and every test would have to mutate the schema with `ALTER TYPE
   … ADD VALUE` before it could insert a row. The migration hygiene test already treats
   `ADD VALUE` as a special, isolated kind of migration file
   [measured 38630d2:internal/store/migrate_test.go:188-191 · `grep -n 'addValueRe.MatchString' -A 3 internal/store/migrate_test.go` →
   `188:  if addValueRe.MatchString(text) {` / `189-   if createTableRe.MatchString(text) || insertRe.MatchString(text) || updateRe.MatchString(text) {` / `190-    t.Errorf("%s: an ADD VALUE file must do nothing else (CREATE TABLE/INSERT/UPDATE found)", entry.Name())`].
3. **§11's greppable-registry argument is about basis-document types, not task types.** That
   argument is honoured exactly where it was made: the `journal_entry` CHECK gains both new
   basis columns by migration (D14). A task type is a different axis — a task whose effects
   move no balance has no basis document at all.

The persisted string is still a data contract: a type name is added, never renamed or
repurposed, and the package doc comment says so. The refusal AC21 requires exists at both
ends — `Schedule` refuses an unregistered type before the INSERT, and the claim path refuses
a row whose type has no declaration (D7), so a type dropped from the registry produces a
visible, counted refusal instead of a row that quietly stays due.

---

### D7 — Settlement: one statement per outcome

Let `t` be the execution instant (D3), `k` the row's `consecutive_failures` **after** this
attempt, `cap`/`base`/`ceiling` the configured retry values, and `rec` the type's declaration.

| Outcome | Recurrent? | Statement |
|---|---|---|
| Done or No-op | no | `DELETE FROM scheduled_task WHERE id = $1` |
| Done or No-op | yes | `UPDATE … SET run_at = $2, consecutive_failures = 0, last_error = NULL WHERE id = $1`, `$2 = rec.Next(run_at, t)` |
| Failed | no, and `k < cap` | `UPDATE … SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`, `$4 = t + backoff(k, base, ceiling)` |
| Failed | no, and `k >= cap` | `UPDATE … SET state = 'dead', consecutive_failures = $2, last_error = $3 WHERE id = $1` |
| Failed | yes | `UPDATE … SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`, `$4 = rec.Next(run_at, t)` |

The properties below fall straight out and are what the tests assert. A recurrence **never**
reaches `dead`: no row above sets it, and the cap is consulted only on the one-shot branch
(AC32) — which is the scope boundary the spec asked to be stated rather than implied, because
applying the cap to both kinds would silently reintroduce the terminal state the owner
removed. A recurrence is **one row moved forward in place**, so the live-row count for one
recurrence is one at every commit boundary (AC11). And `consecutive_failures` rises on each
failure and resets on a success *or* a no-op (AC34).

**A failure that is the worker's, not the handler's.** A claimed row whose type has no
declaration takes the Failed path with `FailureUnregistered`; a decode failure is just a
handler error (AC26 needs it classified as a failure and terminated by the cap, which the
one-shot branch already does, with the decode message landing in `last_error` as AC26's
"recorded reason"). No sentinel is imposed on handlers for this.

`state = 'dead'` rows are never claimed again — the claim filters `state = 'pending'` — and
are enumerable through `DeadTasks(ctx, q, limit)`, ordered by `run_at, id`, each carrying
type, instance key, `run_at`, attempt count and `last_error` (AC10).

---

### D8 — Backoff and cadence as pure functions

```go
func backoff(failures int, base, ceiling time.Duration) time.Duration   // min(base·2^(failures-1), ceiling)
func Every(period time.Duration) Cadence                                // smallest k ≥ 1 with prev + k·period > now
```

Both are pure, take no clock, and are tested by exact table tests. `backoff` is strictly
positive and strictly growing until it reaches the ceiling, which is what AC9's "grows rather
than repeats" asks of the attempts inside the cap. **No jitter**: the transport's backoff
jitters because many callers race one remote rate limit, while `SKIP LOCKED` already
de-collides workers, and a deterministic delay is exactly assertable.

`Every`'s catch-up rule matters: after an outage, a recurrence does not fire once per missed
period — it advances to the next instant strictly after `now`, so a day-close that missed a
week runs once, not seven times.

`Cadence` is a function type rather than a duration so that the mechanic which needs a
wall-clock anchor (the day close, #45) can supply its own without touching this package. This
task ships `Every` only; `Recurrence` pairs a cadence with the **configuration key its value
came from**, and the registry refuses a recurrence whose key is empty — that is what keeps
"a declaration names which key it reads, it does not embed a number" checkable rather than
aspirational.

---

### D9 — The registry is immutable by construction

```go
func NewRegistry(decls ...Declaration) (*Registry, error)
```

Built once, never mutated, safe for concurrent use without a lock — the same posture
`internal/tg`'s `Options` takes
[measured 38630d2:internal/tg/client.go:17 · `grep -n 'type Options struct' internal/tg/client.go` → `17:type Options struct {`].
Construction refuses an empty type, a nil handler, a
duplicate type, and a recurrence with a nil cadence or an empty configuration key, each
wrapping `ErrInvalidDeclaration` with the offending type named.

`Registry` is also the insertion surface: `(*Registry).Schedule(ctx, tx, Request) (TaskID, error)`
takes a **caller-owned** `pgx.Tx` and neither commits nor rolls back, exactly as `store.Post`
does
[measured 38630d2:internal/store/post.go:74 · `grep -n '^func Post' internal/store/post.go` →
`func Post(ctx context.Context, tx pgx.Tx, basis PostingBasis, postings ...Posting) error`].
That is not a convenience: §3.5 schedules a raid session's timer edges **inside the
transition's own transaction**, so a transaction-taking insert is the only shape that keeps a
transition and its timers atomic. `Schedule` returns `ErrUnknownType`, `ErrInvalidDelay`,
`ErrDuplicateTask` (SQLSTATE 23505 on `scheduled_task_identity_key`) or `ErrInvalidPayload`.

---

### D10 — Start-up reconciliation: seed, correct, nothing else

`(*Worker).Reconcile(ctx)` runs the two moves the owner's round-3 answer names, for each
declared recurrence, and no third move.

**Seed** — one statement, idempotent by the constraint rather than by a lock, so several
workers starting at once is safe by construction (AC33):

```sql
INSERT INTO scheduled_task (type, instance_key, payload, run_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL DO NOTHING
```

`$4` is `rec.Next(now, now)` with `now` read from the server — one cadence ahead, never
"immediately", because a fresh deploy must not fire a day close on start-up. The partial-index
inference in that `ON CONFLICT` is measured in D5.

**Correct** — one statement, which leaves an in-flight or imminent occurrence alone:

```sql
WITH candidate AS (
    SELECT id FROM scheduled_task
    WHERE type = $1 AND instance_key = $2 AND state = 'pending'
      AND run_at > $3                    -- $3 = rec.Next(now, now): the declaration disagrees
      AND run_at > now() + $4            -- $4 = the poll interval: not imminent
    FOR UPDATE SKIP LOCKED
)
UPDATE scheduled_task s SET run_at = $3 FROM candidate c WHERE s.id = c.id
```

*In flight* is `FOR UPDATE SKIP LOCKED` — a row a worker is currently executing is skipped,
not waited on. *Imminent* is derived rather than configured: an occurrence that could be
claimed before the correction takes effect is one within a poll interval, which needs no
seventh tuning key. The disagreement test is the same expression as the correction, which is
what makes the statement idempotent — a healthy recurrence always satisfies `run_at <=
rec.Next(now, now)`, so re-running it is a no-op, and a *lengthened* cadence is absorbed by
one early occurrence rather than by a correction that pushes work away.

**No third move.** There is no revive branch, because a recurrence has no dead state to
revive from (owner, round 3), and no collector of rows whose type is no longer declared —
retiring a recurrence deletes its row in the same change that drops its declaration (spec
*Key decisions*), and an automatic deleter would remove the evidence of the mistake it covers
for.

---

### D11 — The per-task execution deadline: three layers, one of them the database's

The transactional shape self-heals a worker that *crashes*. It does not self-heal a handler
that *hangs*: its transaction stays open, its row stays locked, and `SKIP LOCKED` means every
other worker passes over that row silently and forever. AC30 bounds it. Three layers, each
covering what the others cannot:

1. **The handler's context** — `context.WithTimeout(ctx, deadline)`. Every pgx call the
   handler makes through its `pgx.Tx` fails once it expires, so a handler that merely blocks
   on the database stops.
2. **The database** — `SET LOCAL statement_timeout` and `SET LOCAL
   idle_in_transaction_session_timeout`, both set to the deadline at the top of the batch
   transaction. The first bounds a single long statement and leaves the transaction
   recoverable through the savepoint
   [measured postgres:18.6 · `SET LOCAL statement_timeout='200ms'; SAVEPOINT h; SELECT pg_sleep(2); ROLLBACK TO SAVEPOINT h; SELECT count(*); COMMIT` →
   `ERROR: canceling statement due to statement timeout` / `ROLLBACK` / `stmt_timeout_recovered | 3` / `COMMIT`].
   The second is the one that matters for a handler hung in Go code: the server terminates the
   backend, which aborts the transaction and releases its row locks **without our process
   participating at all**
   [measured postgres:18.6 · `BEGIN; SET LOCAL idle_in_transaction_session_timeout='300ms'; SELECT id … FOR UPDATE; <idle 3s>; SELECT 'still alive';` →
   `FATAL: terminating connection due to idle-in-transaction timeout` / `server closed the connection unexpectedly`;
   a concurrent claim then returned that same row]. Both settings are transaction-scoped and
   settable by an ordinary role
   [measured postgres:18.6 · `SELECT current_setting('idle_in_transaction_session_timeout'), current_setting('statement_timeout')` after `SET LOCAL` → `250ms | 250ms`].
3. **The worker stops waiting.** Each handler runs on its own goroutine and the worker selects
   on the result or the deadline. Without this the worker could not *report* the breach, which
   AC30 requires; the two layers above release the row but say nothing.

**On a breach** the worker reports `FailureDeadline` for that task and `FailureRolledBack` for
every earlier task in the batch, then abandons the batch: it does not touch `tx` again, it
**hijacks** the pooled connection so the pool can never hand it to another caller, and it
returns to the loop. `pgxpool.Conn.Hijack` panics on a connection already released, so it is
called at most once and the release path is guarded by the same flag
[measured pgx/v5@v5.10.0:pgxpool/conn.go:71-83 · `grep -n 'func (c \*Conn) Hijack' -A 12 pgxpool/conn.go` →
`if c.res == nil { panic("cannot hijack already released or hijacked connection") }`].

**The residue, named rather than hidden.** The orphaned handler goroutine survives until it
touches the dead connection (its next call fails) or returns. A handler that neither returns
nor touches the database is an infinite loop in Go code, which no scheduler can reclaim; it is
a handler defect and the deadline observation is what makes it visible. Nothing in this path
panics — `Hijack`'s panic is guarded by its documented precondition, and the project's panic
index holds no row today and gains none here
[measured 38630d2:ai-docs/panic-index.md · `sed -n '/^| File:line/,$p' ai-docs/panic-index.md` →
`| File:line | Call | Why it cannot fire (or is unrecoverable) |` / `|---|---|---|` / `| — | — | — |`].

---

### D12 — The observation seam

```go
type Observation struct {
    Type                Type
    Lag                 time.Duration   // execution instant − run_at
    Outcome             Outcome
    Failure             FailureKind
    BatchSize           int
    ConsecutiveFailures int
}
type LoopObservation struct {
    Duration  time.Duration
    BatchSize int
    Err       error
}
type Observer interface {
    ObserveTask(Observation)
    ObserveLoop(LoopObservation)
}
```

`FailureKind`: none · handler · unregistered type · deadline · rolled back. Every member but the
zero value is worker knowledge that no other surface can recover, and each is required by a
criterion or by an open question the spec left standing: `unregistered` is the refusal counter the spec
names as the only signal that an undeclared recurrence's row is coming due forever;
`deadline` is AC30's "reported as a failure rather than passing silently"; `rolledBack` is
what stops D2's emit-after-commit from reporting a phantom success. `exhaustive` is enabled
with `default-signifies-exhaustive: true`, so every switch over `Outcome` or `FailureKind` is
total or carries a default
[measured 38630d2:.golangci.yml:40-42 · `sed -n '40,42p' .golangci.yml` → `exhaustive:` / `default-signifies-exhaustive: true`].

`ConsecutiveFailures` is carried on every observation and is the field AC12 adds beyond issue
#20's own list, for the reason the spec gives: a per-type failure *rate* cannot separate one
recurrence failing every time from many recurrences failing occasionally, and only the first
is an outage.

`Observer` is optional. A nil observer is checked, not called, so the package compiles and its
tests pass with no implementation installed (AC13). `LoopObservation.Err` exists because
`Run` must **not** stop on a transient claim failure — without it the error would be silently
dropped, and this package has no logger.

---

### D13 — Configuration: the second optional-with-default class

The `LAB_GAME_SCHEDULER_`-prefixed variables below, read by a dedicated `loadScheduler` in a new
`internal/config/scheduler.go`, in KD-27's optional-with-default class:

| Variable | Shape | Default |
|---|---|---|
| `LAB_GAME_SCHEDULER_POLL_INTERVAL` | positive duration | `1s` |
| `LAB_GAME_SCHEDULER_CLAIM_LIMIT` | positive integer | `32` |
| `LAB_GAME_SCHEDULER_RETRY_MAX_ATTEMPTS` | positive integer | `5` |
| `LAB_GAME_SCHEDULER_RETRY_BASE_DELAY` | positive duration | `1s` |
| `LAB_GAME_SCHEDULER_RETRY_MAX_DELAY` | positive duration | `5m` |
| `LAB_GAME_SCHEDULER_TASK_TIMEOUT` | positive duration | `30s` |

Every default is **chosen operational tuning, not a balance number and not sourced from
`docs/DESIGN.md`** — §16.5's discipline is untouched, and a task's *game-meaningful* delay (a
corpse TTL, an escalation timer) is set by the mechanic that schedules it, from the balance
file, never here.

The mechanics of the existing layer that bind, and are followed exactly:

- `loadScheduler` reuses `lookupPositiveInt` and `lookupPositiveDuration` — already in the
  package, so no helper is duplicated
  [measured 38630d2:internal/config/transport.go:207-233 · `grep -n '^func lookupPositive' internal/config/transport.go` →
  `func lookupPositiveInt(lookup Lookup, key string) (int, bool, error)` / `func lookupPositiveDuration(…)`].
- The keys go into `EnvKeys()`, **never** into the unexported `envKeys()`, which the
  required-variable suites iterate — putting an optional key there fails them immediately, and
  loosening those suites would attack the requiredness they exist to hold
  [measured 38630d2:internal/config/env.go:33-35 · `sed -n '33,35p' internal/config/env.go` →
  `func envKeys() []string {` / `return []string{envBotToken, envDSN, envBotAPIBaseURL, envAllowedChatIDs}`].
- Every key is queried **unconditionally** on every load, because the disjointness test's
  recording lookup compares the set of keys the loader actually consulted against
  `EnvKeys()` and `.env.example`
  [measured 38630d2:internal/config/disjoint_test.go:103-104 · `sed -n '103,104p' internal/config/disjoint_test.go` →
  `assertSameKeySet(t, ".env.example", exampleKeys, "config.EnvKeys()", EnvKeys())` /
  `assertSameKeySet(t, "loader-consulted keys", recorded(), "config.EnvKeys()", EnvKeys())`].
- Each key gets a `.env.example` line carrying its default as a **non-empty** value, because
  one test requires every value non-empty and another requires the whole file to load
  [measured 38630d2:internal/config/disjoint_test.go:108-115 · `sed -n '108,115p' internal/config/disjoint_test.go` →
  `func TestEnvExample_ValuesAreNonEmpty` … `t.Errorf(".env.example: %s has an empty value", k)`].

`Config` gains a `Scheduler` field and `Load` calls `loadScheduler` alongside `loadTransport`,
joining failures the same way. `scheduler.Options` carries `config.Scheduler` by value, the
shape `tg.Options` already uses for `config.Transport`, and `scheduler.New` refuses any
non-positive member with an `*OptionError` naming the field — which is also what makes AC14's
"no literal value at a call site in `internal/scheduler`" a property rather than a promise.

---

### D14 — The basis documents, the CHECK migration, and its rollback

Migration `internal/store/migrations/00002_scheduler.sql`, forward-only, with **no
`-- +goose Down` section** — the package has none and a test enforces it
[measured 38630d2:internal/store/migrate_test.go · `grep -n 'contains -- +goose Down' internal/store/migrate_test.go` →
`t.Errorf("%s: contains -- +goose Down", entry.Name())`].

```sql
CREATE TABLE deferred_task (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_type    text        NOT NULL,
    instance_key text,
    run_at       timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE recurrent_task ( … the same shape … );

ALTER TABLE journal_entry
    ADD COLUMN deferred_task_id  bigint REFERENCES deferred_task  (id),
    ADD COLUMN recurrent_task_id bigint REFERENCES recurrent_task (id),
    DROP CONSTRAINT journal_entry_exactly_one_basis,
    ADD  CONSTRAINT journal_entry_exactly_one_basis
         CHECK (num_nonnulls(player_operation_id, manual_correction_id,
                             deferred_task_id, recurrent_task_id) = 1);
CREATE UNIQUE INDEX journal_entry_deferred_task_key  ON journal_entry (deferred_task_id)  WHERE deferred_task_id  IS NOT NULL;
CREATE UNIQUE INDEX journal_entry_recurrent_task_key ON journal_entry (recurrent_task_id) WHERE recurrent_task_id IS NOT NULL;
```

This is the shape migration 00001 established, extended rather than reinvented: one nullable
FK column per basis type, one `CHECK (num_nonnulls(…) = 1)` naming all of them, one partial
unique index per basis column
[measured 38630d2:internal/store/migrations/00001_ledger_core.sql:70-75 · `sed -n '70,75p' …/00001_ledger_core.sql` →
`player_operation_id  bigint      REFERENCES player_operation (id),` … `CONSTRAINT journal_entry_exactly_one_basis CHECK (num_nonnulls(player_operation_id, manual_correction_id) = 1)` … `CREATE UNIQUE INDEX journal_entry_player_operation_key  ON journal_entry (player_operation_id)  WHERE player_operation_id  IS NOT NULL;`]. The partial predicate is also what keeps the FK-coverage
test green, since it accepts an index whose predicate is an `IS NOT NULL`
[measured 38630d2:internal/store/fkcover_test.go:85 · `sed -n '85p' internal/store/fkcover_test.go` →
`if idx.pred != nil && !strings.Contains(strings.ToUpper(*idx.pred), "IS NOT NULL") {`].

**Rows written before this migration.** Both new columns are `NULL` for every existing
`journal_entry`, so `num_nonnulls` is unchanged for them and the re-added CHECK validates
without a single row failing. The CHECK swap and the column additions are one `ALTER TABLE`
inside the migration's transaction, so no window exists in which the table has the columns but
not the constraint.

**The rollback, stated as the rule requires: it is another forward migration, never a down
section.** A `00003` would drop `journal_entry_deferred_task_key` /
`journal_entry_recurrent_task_key`, drop the two columns, restore the two-column CHECK, and
drop `scheduled_task`, `recurrent_task`, `deferred_task` and `scheduled_task_state`. It is
safe **only while no `journal_entry` row has either column non-null** — once a mechanic has
posted under one of these documents, the rollback is a data-destroying operation and the
answer is a compensating migration, not a reversal.

**The Go types live in `internal/store` — forced, not chosen.** `PostingBasis`'s methods are
unexported, so no other package can satisfy it
[measured 38630d2:internal/store/basis.go:18-25 · `sed -n '18,25p' internal/store/basis.go` →
`type PostingBasis interface {` / `entrySQL() (string, error)` / `insert(ctx context.Context, tx pgx.Tx) (int64, error)`].
`DeferredTask` and `RecurrentTask` are `internal/store` types beside `PlayerOperation` and
`ManualCorrection`, each carrying `TaskType string`, `InstanceKey string` (empty stored as
`NULL` via `NULLIF`) and `RunAt time.Time` — **by value, with no foreign key back to
`scheduled_task`** (AC27), because delete-on-done means the task row is gone while its basis
document and postings live on. And the doc comment that today says the sum type has "exactly
the store package's two implementations" is rewritten to describe the set the package actually
has (AC20)
[measured 38630d2:internal/store/basis.go:11-13 · `sed -n '11,13p' internal/store/basis.go` →
`// PostingBasis is the sealed sum type of documents a journal_entry may` / `// reference — exactly the store package's two implementations,` / `// *PlayerOperation and *ManualCorrection (D8).`].

**No posting signature ships here.** The scheduler moves no balance of its own; each mechanic
declares the signature of the effects it posts under a deferred one-shot or a recurrent task
when it ships its handler (`docs/DESIGN.md` §13.4, spec *Out of scope*).

---

### D15 — Propagation: the sites this diff falsifies

Membership is decided by `AGENTS.md` § Propagation Rule step 4 — every live doc must agree,
history surfaces are left untouched. The sites known now, each with the claim the diff
falsifies:

| Site | What becomes false |
|---|---|
| `AGENTS.md` § API Stability carve-out | spells the table `scheduled_tasks` |
| `ai-docs/key-decisions.md` KD-4 | spells the table `scheduled_tasks` |
| `ai-docs/key-decisions.md` KD-27 | "the relaxation reaches **only** these keys" — a second optional-with-default class now exists |
| `ai-docs/context.md` Scheduler block row | spells the table `scheduled_tasks` |
| `ai-docs/context.md` Architecture + Status paragraphs | do not name `internal/scheduler` |
| `internal/store/basis.go` | "exactly the store package's two implementations" |
| `internal/config/env.go` header comment | names the transport keys as *the* optional-with-default class |
| `internal/config/config.go` `Config` doc comment | "Transport is the one exception" |
| `internal/store/migrate_test.go` | hard-codes the table list, the `goose_db_version` row count and the index list |
| `.env.example` | does not document the new keys |
| `ai-docs/plans/INDEX.md` | has no row for this pair |

The plural-spelling rows are measured, not remembered
[measured 38630d2 · `grep -rn 'scheduled_tasks' AGENTS.md ai-docs/context.md ai-docs/key-decisions.md | cut -c1-60` →
`AGENTS.md:84:> **CARVE-OUT — data contracts are the opposite` /
`ai-docs/key-decisions.md:15:**KD-4 — A self-written schedule` /
``ai-docs/context.md:36:| Scheduler | `scheduled_tasks` worker``], and so is the `env.go`
sentence this change falsifies
[measured 38630d2:internal/config/env.go:11-15 · `sed -n '11,15p' internal/config/env.go` →
`// Environment variable names, LAB_GAME_ prefixed …` … `// tuning variables (transport.go) are a separate, optional-with-default` / `// class, added on top by EnvKeys() (design D10).`].
The list is the class the diff falsifies as known at design time; membership is re-derived at
Step 8 by the same rule, not bounded by this table.

**AC19's exclusion set, stated so a verifier does not "fix" the wrong thing.** The plural
spelling survives at sites that are **not** in scope, and each is excluded for a
reason the spec already settled:

- **`docs/DESIGN.md` §11's own prose**, which spells the table plural — a task does not edit
  `docs/DESIGN.md` (`AGENTS.md` § Project); the
  spec's *Source conflicts* records the inconsistency and its *Open questions* asks the owner
  whether §11's prose should be corrected. Nothing here blocks on the answer.
- `.gitignore:35` — `.claude/scheduled_tasks.lock` is a harness lock **file**, not this
  table, and renaming it would break the harness
  [measured 38630d2:.gitignore:35 · `grep -n 'scheduled_tasks' .gitignore` → `35:.claude/scheduled_tasks.lock`].
- `ai-docs/plans/done/2026-09-04-bot-api-transport.design.md` and this task's own spec —
  history and a verbatim quotation of §11 respectively; AC19 excludes `done/` explicitly.

---

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Migration `00002_scheduler.sql`: `scheduled_task_state`, `scheduled_task` with its indexes and CHECKs, `deferred_task`, `recurrent_task`, the `journal_entry` column + CHECK + partial-unique-index extension (D5, D14). Update the migration suite: table list, `goose_db_version` row count, index list, `journal_entry_exactly_one_basis` definition; add the `scheduled_task` column-set assertion that carries AC31 and the AC3 shape assertion | `internal/store/migrations/00002_scheduler.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 2 | `store.DeferredTask` / `store.RecurrentTask` implementing `PostingBasis`; rewrite the `PostingBasis` doc comment (AC20); tests for posting under each new basis, the two-non-null refusal, and ledger survival of a deleted task row (AC4, AC27) | `internal/store/basis.go`, `internal/store/basis_test.go`, `internal/store/schema_test.go` | 1 |
| 3 | `config.Scheduler`, its defaults, `loadScheduler`, `schedulerEnvKeys()`, the `EnvKeys()` append, the `Config.Scheduler` field and `Load` wiring; the falsified doc comments in `env.go` and `config.go`; the `.env.example` block; the absent/present/malformed and example-matches-defaults tests (D13, AC15) | `internal/config/scheduler.go`, `internal/config/env.go`, `internal/config/config.go`, `internal/config/scheduler_test.go`, `.env.example` | — |
| 4 | Package foundation with no database: `doc.go`, `errors.go`, `task.go` (`Type`, `TaskID`, `Task`, `Request`, `Outcome`, `DeadTask`), `observe.go` (`Observation`, `LoopObservation`, `Observer`, `FailureKind`), `registry.go` (`Declaration`, `Recurrence`, `Registry`, `NewRegistry`), `cadence.go` (`Cadence`, `Every`) and the pure `backoff`; exact table tests for `backoff` and `Every` and the registry's refusals (D1, D8, D9, D12) | `internal/scheduler/doc.go`, `errors.go`, `task.go`, `observe.go`, `registry.go`, `cadence.go`, `cadence_test.go`, `registry_test.go` | 3 |
| 5 | The insertion surface: `(*Registry).Schedule` and `DeadTasks`; the package's `TestMain` and schema fixture; tests for the payload round-trip, the unregistered-type refusal at insertion, the duplicate-identity refusal and the coexisting keyless one-shots (AC21 insertion, AC24, AC29) | `internal/scheduler/schedule.go`, `schedule_test.go`, `scheduler_test.go` | 1, 4 |
| 6 | The batch transaction: `Options`, `New`, `RunOnce`, the claim, the per-task savepoint, the Done and No-op settlement paths; tests for batch bounding and skip-not-wait, two concurrent workers, effects-and-settlement atomicity, the no-op's absent writes, delete-on-done both directions and the recurrence's single live row (AC5, AC6, AC7, AC8, AC11, AC25) | `internal/scheduler/worker.go`, `execute.go`, `claim.go`, `worker_test.go` | 5 |
| 7 | The failure policy: attempt counting, persisted backoff, the one-shot give-up, the recurrent reschedule-to-next-cadence, the unregistered-type refusal at claim time; tests for exact attempt counts and the terminal state, dead-row enumeration, the decode failure, the never-terminal recurrence and the rising/resetting counter (AC9, AC10, AC21 claim time, AC26, AC32, AC34) | `internal/scheduler/execute.go`, `failure_test.go` | 6 |
| 8 | The loop and the seam: `Run`, the poll interval, emit-after-commit, the loop observation; tests collecting observations for a success, a no-op, a retry, a give-up and a repeatedly failing recurrence, plus non-default tuning values changing observed behaviour and the nil-observer path (AC12, AC13, AC14 worker half) | `internal/scheduler/worker.go`, `observe.go`, `observe_test.go` | 7 |
| 9 | The per-task execution deadline: the `SET LOCAL` statement and idle-in-transaction timeouts, the handler goroutine, batch abandonment, connection hijack, `FailureDeadline` / `FailureRolledBack`; tests for the blocked handler, the row becoming claimable, the observation, and the aborted-transaction self-healing property (AC30, AC31 behavioural half) | `internal/scheduler/execute.go`, `worker.go`, `deadline_test.go` | 8 |
| 10 | `Reconcile`: the seed and the correction; tests for the missing-row seed, the changed-declaration correction, two concurrent start-ups producing one row, and the imminent occurrence left alone (AC33) | `internal/scheduler/reconcile.go`, `reconcile_test.go` | 8 |
| 11 | Propagation (D15): the table's spelling at its live sites, KD-27's boundary sentence, KD-4, the `context.md` Architecture/Status/Scheduler-row updates, the `INDEX.md` row | `AGENTS.md`, `ai-docs/context.md`, `ai-docs/key-decisions.md`, `ai-docs/plans/INDEX.md` | 1–10 |

---

## Handoff plan

- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–10 (code change-type: `*.go`, `*.sql` migrations, and
  `.env.example`, which is the Go configuration loader's own gated manifest and is asserted by
  a Go test, so it travels with the code that reads it). Entering this group is itself a
  handoff: spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md` § Compaction
  recovery (re-entry) before subtask 1. All ten same-change-type subtasks are clustered into
  ONE group rather than interleaved with the documentation work, which is the minimization
  (f) requires; the group is at the size cap of 10.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task`
  resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh), 1M-token window — subtask 11 (instructions/harness change-type:
  `AGENTS.md`, `ai-docs/**`). Terminal group (1 subtask; within the `1..=10` range).

Two groups, within the default maximum of 4; no user approval is required.

---

## Risks

- **A hung handler is the one failure the transactional shape does not self-heal**; a row
  locked forever is invisible, because `SKIP LOCKED` makes every other worker pass over it
  silently. Mitigated by D11's layers, the database's being the one that does not depend on
  our process — `[measured postgres:18.6 · `BEGIN; SET LOCAL idle_in_transaction_session_timeout = '300ms'; SELECT id FROM scheduled_task ORDER BY id DESC LIMIT 1 FOR UPDATE; <3s idle>; SELECT 'still alive';` → `idle_holder_locked | 5` then `FATAL:  terminating connection due to idle-in-transaction timeout` / `server closed the connection unexpectedly`; a concurrent `… FOR UPDATE SKIP LOCKED LIMIT 10` then returned row `5`]`.
- **The orphaned handler goroutine after a deadline breach** survives until it touches the
  hijacked connection. A handler that neither returns nor touches the database leaks one
  goroutine per breach — `[derived → the AC30 test asserts the row is claimable and the observation made; the doc comment names the residue]`.
- **A deadline breach or a crash discards the whole batch's uncommitted work**, so tasks
  already executed in that batch run again. Correct (nothing committed) but wasteful; the
  claim limit is the lever — `[derived → D2's stated consequence and the AC30 test, which asserts the earlier task's writes are absent afterwards]`.
- **`pgxpool.Conn.Hijack` panics on an already-released connection**, and this project targets
  zero production panics — `[measured pgx/v5@v5.10.0:pgxpool/conn.go:71-83 · `grep -n 'func (c \*Conn) Hijack' -A 12 pgxpool/conn.go` → `panic("cannot hijack already released or hijacked connection")`]`.
  Mitigated by a single guarded call site: one flag decides hijack-or-release, and no path
  reaches both. No row is added to the panic index.
- **The migration suite hard-codes facts this migration changes** — the table list, the
  `goose_db_version` row count and the index list all fail unless updated in the same subtask
  — `[measured 38630d2:internal/store/migrate_test.go:36 · `grep -n 'want := \[\]string{' internal/store/migrate_test.go` → `36:	want := []string{`; 38630d2:internal/store/migrate_test.go:131 · `grep -n 'goose_db_version rows' internal/store/migrate_test.go` → `131:		t.Fatalf("goose_db_version rows = %d, want 2", count)`]`.
- **KD-27's boundary sentence and `Config`'s doc comment become false** the moment a second
  optional-with-default class exists — `[measured 38630d2:ai-docs/key-decisions.md:73 · `grep -n 'the relaxation reaches' ai-docs/key-decisions.md` → one match, at line 73 (the KD-27 paragraph), containing `*The boundary, stated exactly:* the relaxation reaches **only** these keys.`; 38630d2:internal/config/config.go:38-39 · `sed -n '38,39p' internal/config/config.go` → `// returns an error naming every rejected key. Transport is the one` / `// exception: its fields are individually optional-with-default, so an`]`.
- **`jsonb` normalises, so a byte-exact payload round trip does not hold** — a test asserting
  raw bytes would pass by luck and fail on a reordered key —
  `[measured postgres:18.6 · `SELECT '{"b":1,"a":{"c":null},"b":2}'::jsonb` → `{"a": {"c": null}, "b": 2}`]`.
  Mitigated by comparing decoded values (§ Test Design, AC24).
- **`go test -race ./...` is a required gate and the worker is goroutine code by
  construction**; the handler goroutine, the observer callbacks and the shared test handler
  state are the surfaces — `[derived → every scheduler test package runs under the race gate `make verify` invokes, and the AC6 concurrency test asserts exactly-once under it]`.
- **A truncating gate hides later failures.** `go build ./...` prints at most ten errors per
  package and `golangci-lint run` caps issues per linter, so the site counts a subtask
  discovers are floors. Each subtask re-runs its gate after its own cleanup, and any
  newly-revealed out-of-contract class is surfaced to the orchestrator rather than absorbed —
  `[derived → each subtask's gate re-run, per the /task Step-8 loop]`.
- **`internal/testdb` must stay out of `cmd/bot`'s dependency graph** (KD-20)
  `[measured 38630d2:ai-docs/key-decisions.md:53 · `grep -n 'KD-20 ' ai-docs/key-decisions.md | cut -c1-88` → ``53:**KD-20 — Tests provision Postgres through testcontainers-go, and never skip.** `inte``]`,
  so the scheduler's fixture is imported only from `_test.go` files — `[derived → AC18's check on the resulting tree]`.

---

## Test Design

Every claim in this section is about a test that does not yet exist, so every tag is
`[derived → …]`. Every database test runs against a real PostgreSQL server through
`internal/testdb`, each in its own schema, with `TestMain` calling `testdb.Main` exactly as
`internal/store` does
[measured 38630d2:internal/store/store_test.go:15 · `grep -n 'os.Exit(testdb.Main(m))' internal/store/store_test.go` → `15:	os.Exit(testdb.Main(m))`]
(AC17) — no fake, no mock, and no skip when no database is available.

**Fixtures and helpers, in `internal/scheduler`:**

- `newScheduler(tb)` — a `testdb.Schema` pool with `store.Migrate` applied, closed on cleanup;
  the shape `internal/store`'s `newStore`
  [measured 38630d2:internal/store/store_test.go:20 · `grep -n '^func newStore' internal/store/store_test.go` → `20:func newStore(tb testing.TB) *pgxpool.Pool {`]
  already uses `[derived → subtask 5]`.
- `recordingObserver` — collects `Observation` and `LoopObservation` values behind a mutex, so
  every assertion on the seam is exact and the collector is race-clean `[derived → AC12]`.
- Test handler types and **their payload keys, named here because AC28 asks the design to name
  them**: `test.oneshot` reads `n` (integer — the value its handler writes into
  `manual_correction.reason`) and `nested` (object, present only in the round-trip case);
  `test.recurrent` reads no payload key. Handlers write through a real table
  (`manual_correction`) rather than a bespoke fixture table, so "its writes are absent" is
  asserted against the migrated schema `[derived → AC8, AC24, AC28]`.
- `dueNow(tb, pool, req)` — inserts a task through `Registry.Schedule` inside a committed
  transaction with zero delay `[derived → subtask 5]`.

**Pure functions — no database, no clock** (`cadence_test.go`, subtask 4):

- `backoff`: a table over the attempt index asserting **exact** durations for a given base and
  ceiling, that each is strictly greater than the previous until the ceiling, that the ceiling
  clamps, and that the result is strictly positive for the first attempt `[derived → AC9]`.
- `Every`: a table asserting the next instant is the smallest `prev + k·period` strictly after
  `now`; the catch-up case (a `now` many periods past `prev` yields **one** next instant, not a
  backlog); and the just-completed case `[derived → AC11, AC32]`.
- `NewRegistry`: refuses an empty type, a nil handler, a duplicate type, a recurrence with a
  nil cadence and a recurrence with an empty configuration key, each error naming the type
  `[derived → AC21 registration, D9]`.

**Insertion** (`schedule_test.go`, subtask 5):

- Round-trip: a payload with a nested object and a null value is scheduled, claimed and
  delivered to the handler; the assertion decodes both sides and compares values, never raw
  bytes `[derived → AC24]`.
- `Schedule` with an unregistered type returns `ErrUnknownType` and writes no row `[derived → AC21]`.
- A recurrence already live: a second `Schedule` of the same type and instance key returns
  `ErrDuplicateTask` wrapping SQLSTATE 23505 on `scheduled_task_identity_key`; two keyless
  one-shots of the same type both succeed and coexist `[derived → AC29]`.
- A negative delay returns `ErrInvalidDelay` and writes no row `[derived → D3]`.
- `DeadTasks` returns give-up rows only, ordered, each carrying type, `run_at`, attempt count
  and last error `[derived → AC10]`.

**The batch transaction** (`worker_test.go`, subtask 6):

- More due tasks than the limit: one `RunOnce` executes at most the limit, and with a second
  transaction holding some rows the claim returns the unlocked ones rather than blocking
  `[derived → AC5]`.
- Two workers, many due tasks, run concurrently under `-race`: every task's handler ran
  exactly once, counted behind a mutex `[derived → AC6]`.
- A handler that writes and then returns an error: none of its writes are visible after the
  cycle, and the row records the failed attempt rather than a success `[derived → AC7]`.
- A handler that writes and then reports `OutcomeNoop`: its writes are absent, the row is
  settled, and the observation's outcome is neither success nor failure `[derived → AC8]`.
- Delete-on-done, both directions: a committed one-shot leaves no row; a cycle whose
  transaction is rolled back leaves the row present and still due `[derived → AC25]`.
- A recurrence across a committed execution, a rolled-back execution and a failed execution:
  the row's `id` is unchanged and the live-row count for that recurrence is exactly one at
  every commit boundary `[derived → AC11]`.

**Failure policy** (`failure_test.go`, subtask 7):

- A one-shot whose handler always fails, driven attempt by attempt (each cycle preceded by
  making the row due, so the test waits for no backoff): the persisted `run_at` deltas grow,
  the **exact** attempt count at give-up equals the configured cap, the state is terminal, and
  a subsequent claim does not return it `[derived → AC9]`.
- A handler that cannot decode its payload: classified as a failure, not a no-op; reaches
  give-up within the cap; enumerable through `DeadTasks` with the decode message as its
  recorded reason `[derived → AC26]`.
- A recurrence whose handler fails more times than the one-shot cap: still `pending`, still
  advancing by its cadence, never terminal `[derived → AC32]`.
- The counter rises across successive failures of one recurrence and returns to zero after a
  success, and after a no-op `[derived → AC34]`.
- A row whose type is not declared: refused visibly — the observation carries the
  unregistered-type failure kind, and the row does not silently accumulate retries beyond the
  refusal `[derived → AC21]`.

**The seam and the loop** (`observe_test.go`, subtask 8):

- One observation per executed task, carrying type, lag, outcome, batch size and — on failure
  — the consecutive-failure count; collected for a success, a guard no-op, a one-shot retry, a
  one-shot give-up and a repeatedly failing recurrence, plus one loop observation carrying a
  duration `[derived → AC12]`.
- Lag is positive and is measured against `run_at`, asserted as a bound rather than an exact
  value (it is a real elapsed interval, not a virtual one) `[derived → AC12]`.
- With no observer installed, the same cycles run and the package's tests pass `[derived → AC13]`.
- A worker constructed with non-default tuning values shows the changed behaviour: a smaller
  claim limit bounds the batch, a smaller attempt cap gives up sooner, a different backoff base
  changes the persisted delays, and a short poll interval makes a freshly inserted task run
  within a bounded wait `[derived → AC14]`.

**The deadline** (`deadline_test.go`, subtask 9):

- A handler that blocks past a short configured deadline: the worker returns, the observation
  is made with the deadline failure kind, and the row becomes claimable again within a bounded
  wait; a following cycle executes it `[derived → AC30]`.
- The self-healing property that replaces a liveness protocol: a transaction that claims a
  row, writes, and is then rolled back leaves the row still due with none of its writes
  visible, and the next claim returns it `[derived → AC31]`.

**Reconciliation** (`reconcile_test.go`, subtask 10):

- A declared recurrence with no row is seeded, one cadence ahead, not immediately `[derived → AC33]`.
- A declared recurrence whose row disagrees with the declaration has its `run_at` corrected;
  running `Reconcile` twice changes nothing the second time `[derived → AC33]`.
- Two `Reconcile` calls racing under `-race` produce exactly one row, and the loser's insert is
  the constraint's no-op rather than a lock wait `[derived → AC33, AC29]`.
- An imminent occurrence (inside a poll interval) and an in-flight occurrence (its row locked
  by an open transaction) are both left untouched `[derived → AC33]`.

**In `internal/store`** (subtasks 1 and 2):

- `store.Migrate` against an empty database yields `scheduled_task`, `deferred_task` and
  `recurrent_task`; the migration file has no down section `[derived → AC2]`.
- `journal_entry` carries one nullable FK column per basis type, the CHECK names every one of
  them, and each new basis column has its own partial unique index; the FK-coverage test stays
  green `[derived → AC3]`.
- A balanced batch through `store.Post` under each new basis type succeeds and its
  `journal_entry` row has exactly one non-null basis column; a row attempting two is refused
  by the database with the CHECK's name `[derived → AC4]`.
- A task posts under a new basis type, the task row is then deleted, and the `journal_entry`
  and `posting` rows survive and still balance `[derived → AC27]`.
- `scheduled_task`'s column set contains no execution marker, no heartbeat and no `completed`
  state value `[derived → AC31, AC25]`.

**No golden fixture is specified by this task.** Nothing here is a pure simulation whose
output is snapshotted; the combat-golden rule applies to `combat()` and its callers, not to a
worker whose observable outputs are database rows and observation structs.

---

## Open questions

- **`docs/DESIGN.md` §11's prose still spells the table plural**, six lines after the
  singular-names decision it states. Carried forward from the spec unchanged: a task does not
  edit `docs/DESIGN.md`, so the design excludes it from AC19's sweep (D15) and nothing blocks
  on the answer. The owner may want it corrected in a separate change.
- **The task type is `text`, not a PostgreSQL enum** (D6). The argument is made in full and
  the decision is the design's to make per the spec, but it is a visible departure from "every
  categorical column is a PostgreSQL enum" and is flagged here so the owner sees it rather than
  discovering it in the migration.
- **The poll interval is fixed: a full claim batch does not trigger an immediate re-poll.**
  Throughput is therefore bounded by claim-limit-per-poll-interval. The batch-size observation
  (AC12) is exactly the signal that would justify changing it, and the change is local to the
  loop. Recorded as a consequence, not reopened.
- **A deadline breach costs the whole batch's uncommitted work.** Correct but wasteful; the
  claim limit bounds the blast radius. If it is ever measured to matter, the fix is a smaller
  limit for the affected worker, not a second transaction.
- **`consecutive_failures` doubles as the one-shot attempt count** (D5). The two are the same
  number for a one-shot by construction, and a second column would be lifetime history on a
  table the owner's answer stripped of history. Flagged because AC9/AC10 and AC34 name the two
  concepts separately.
