# Design: Postgres task scheduler — `scheduled_task`, the `SKIP LOCKED` worker, one-shot and recurrent tasks

**Issue:** #20
**Date:** 2026-09-05

> **Claim-tag conventions in this document.** A repo fact carries the commit of the **read**
> (`ac09e61`, re-resolved for this round rather than carried forward from round 1), not of this
> document, so a tag may lag `HEAD` after a revision that touched only this file. A fact about
> **PostgreSQL's own behaviour** has no repo path, so its pin is the server version the probe ran
> against (`postgres:18.6`, a throwaway container started from the image already on this machine
> and stopped afterwards). A fact about an **external module** is pinned by its version. A claim
> about an artefact this task has **not yet built** carries `[derived → …]` and no locator.
> `docs/DESIGN.md` is cited by section per the design-writer contract, so those citations carry
> no `[measured …]` tag.

---

## Round-2 review resolutions

Where each round-1 finding and recommendation is answered, so Step 8 can verify the write-back
without re-reading the whole document.

| Round-1 item | Resolved in | Resolution in one line |
|---|---|---|
| **Issue 1 (major)** — `dead` + the identity index poisons an identity forever | D5, D7, D10, § Test Design, § Open questions | Both halves closed: the identity index is scoped to live rows (option (a), re-measured), and an undeclared type gets its own settlement that never reaches `dead` (option (c)) |
| **Issue 2 (major)** — the batch shape falsifies live "one transaction per task" claims | § Approach, D2, D11, D12, D15 | **The shape changed, not the wording.** One transaction per task, as §11 and its derived sentences state; the batch survives as a discovery claim |
| **Issue 3 (minor)** — nobody closes the hijacked connection; AC30's bound unnamed | D11 | A watchdog goroutine owns and closes it; the bound is named as ≈ 2 × `LAB_GAME_SCHEDULER_TASK_TIMEOUT` |
| **Issue 4 (minor)** — `LoopObservation.Duration` falsifies "no `time.Now`" | § Approach (*The database is the clock that every persisted instant is read from*), D3, D12 | The absolute is restated as the property it actually is, and the loop duration is admitted as a Go-measured interval with its reason |
| **Rec.** — D10's seed `$3` payload never given a value | D10 | The seed writes `'{}'::jsonb` literally; `Recurrence` carries no payload by design |
| **Rec.** — the `postgres:18.6` DDL tags name no AC | D5, D10, D11 | Each such tag now names the AC that re-proves it on the delivered schema |
| **Rec.** — Group B does not name its subagent | § Handoff plan | `general-purpose` named, symmetric with Group A |
| **Rec.** — subtask 4's dependency on 3 is spurious | § Decomposition | Removed; 4 now depends on nothing |
| **Rec.** — AC33's "a row that disagrees" does not name the direction | § Test Design | Named: the correction fires for a **shortened** cadence |
| **Spec amendment** (round 5) — `docs/DESIGN.md:310` is corrected, not excluded | D15, § Decomposition subtask 11, § Handoff plan | AC19's exclusion set rebuilt to exactly the classes the spec names; `docs/DESIGN.md` moved from *excluded* to *corrected*; the one-word bound and the trap clause on the same line stated where the edit is prescribed. Round 1's *Open question* asking the owner about §11's spelling is retired — the owner answered it |

---

## Approach

`internal/scheduler` is a worker over one table. Its whole shape falls out of the decisions
immediately below, and every later section is a consequence of one of them.

**One transaction per task — and the batch is a *discovery* claim, not the executing
transaction.** `docs/DESIGN.md` §11 fixes both the claim query
(`WHERE run_at <= now() FOR UPDATE SKIP LOCKED LIMIT N`) and the exactly-once argument (a task
executes in one transaction with its own effects). Round 1 read those sentences as being in
tension the moment the limit exceeds one, and resolved the tension by claiming and executing a
whole batch in one transaction with a savepoint per task. **That resolution is withdrawn.** It
made a task's durability depend on its neighbours — a later task's deadline breach discarded an
earlier task's already-executed effects — which is exactly the property §11's sentence buys, and
it falsified the live derived sentences that spell the cardinality out
[measured ac09e61:ai-docs/context.md:36 · `grep -n 'one transaction per task with its effects' ai-docs/context.md` →
``36:| Scheduler | `scheduled_tasks` worker (`FOR UPDATE SKIP LOCKED`), one transaction per task with its effects | §11 |``;
ac09e61:ai-docs/key-decisions.md:15 · `grep -n 'each task executing in one transaction' ai-docs/key-decisions.md` →
`15:**KD-4 …** … each task executing in one transaction with its effects.`;
ac09e61:ai-docs/domain-invariants.md:45 · `grep -n 'A task executes in one transaction' ai-docs/domain-invariants.md` →
`45:- **A task executes in one transaction with its effects** — exactly-once without two-phase machinery.`].
`docs/DESIGN.md`'s own sentence is not editable beyond the one authorised word (D15), so a shape
whose derived docs need rewording is a shape that disagrees with its source. The tension was not
in §11; it was in the assumption that the claim's locks must be the ones execution happens under.
They need not be. The cycle is:

1. **Discovery** — one `SELECT … FOR UPDATE SKIP LOCKED LIMIT N` statement, run outside any
   explicit transaction, so its row locks live only for the statement. It answers "which ids are
   due and not currently being executed", and its cardinality is AC12's batch size.
2. **Execution** — for each discovered id, its **own** transaction: re-claim that one row by id
   under `FOR UPDATE SKIP LOCKED`, run the handler, settle, commit. A row another worker took in
   the meantime returns zero rows and is skipped silently — not a failure, not an observation.

Both halves are measured. A second worker's per-id re-claim of a row held by an executing
transaction returns no rows rather than waiting, and a free row is returned
[measured postgres:18.6 · with one session holding `id = 5` in an open transaction,
`SELECT id … WHERE id = 5 AND state='pending' AND run_at <= now() FOR UPDATE SKIP LOCKED` → `(0 строк)`;
the same statement for `id = 6` → `6`]. And the discovery claim itself skips the row under
execution
[measured postgres:18.6 · same fixture, `SELECT id … WHERE state='pending' AND run_at <= now() ORDER BY run_at, id FOR UPDATE SKIP LOCKED LIMIT 5` →
`6` and `8`, not `5`]. Exactly-once therefore rests on the per-task lock, and AC5's clauses
rest on the discovery claim; nothing rests on a reading of §11.

**A savepoint still exists, and its scope is now exactly the handler call.** AC7 wants a
handler's writes gone but the failed attempt recorded, and AC8 wants a no-op's writes gone but
the row still settled in the same transaction as the (absent) effects. Only a subtransaction
around the handler satisfies both: roll back to it, then write the settlement, then commit
[measured postgres:18.6 · `BEGIN; <re-claim id=6 FOR UPDATE>; SAVEPOINT handler; INSERT INTO effect …; ROLLBACK TO SAVEPOINT handler; UPDATE scheduled_task SET consecutive_failures = consecutive_failures + 1, last_error = 'boom', run_at = clock_timestamp() + interval '1 s' WHERE id = 6; COMMIT` →
`effect_rows | failures | last_error` = `0 | 1 | boom`]. A subtransaction aborted by a constraint
violation is recoverable the same way and the outer transaction still commits
[measured postgres:18.6 · `BEGIN; SAVEPOINT h2; <duplicate INSERT>; ROLLBACK TO SAVEPOINT h2; SELECT count(*); COMMIT` →
`ERROR: duplicate key value violates unique constraint "scheduled_task_identity_key"` / `ROLLBACK` /
`rows_after_recovery | 6` / `COMMIT`]. pgx exposes it as a first-class API: `pgx.Tx.Begin` opens a
pseudo-nested transaction on a savepoint, `Commit` releases it and `Rollback` rolls back to it
[measured pgx/v5@v5.10.0:tx.go:171,309,322 · `grep -n 'savepoint sp_\|release savepoint\|rollback to savepoint' tx.go` →
`171:  _, err := tx.conn.Exec(ctx, "savepoint sp_"+…)` / `309:  … "release savepoint sp_"+…` / `322:  … "rollback to savepoint sp_"+…`].

**The database is the clock that every persisted instant is read from.** Due-ness is
`run_at <= now()`, evaluated by the server; the execution instant is a `clock_timestamp()` the
server returns; backoff and cadence are **pure functions of instants the database supplied**. No
Go instant is ever compared against a persisted one. This is not stylistic. `now()` is
`transaction_timestamp()` and is frozen for its transaction, while `clock_timestamp()` advances
inside it
[measured postgres:18.6 · `BEGIN; SELECT now(), clock_timestamp(); SELECT pg_sleep(0.4); SELECT now(), clock_timestamp(); COMMIT` →
`now_fn` identical (`…29.486061+00` twice), `clock_fn` `…29.486284+00` then `…29.887163+00`],
so mixing a Go instant into a comparison against either is a silent skew bug — and the lag metric
this whole task exists to produce is exactly such a comparison. It also settles the KD-26
question before it is asked: the package has no persisted-instant clock to fake, so no scheduler
test needs a `synctest` bubble around a real socket, and KD-26's binding half ("production code
carries no clock abstraction") is satisfied more strongly than by an injectable clock
[measured ac09e61:ai-docs/key-decisions.md:71 · `grep -n 'KD-26 ' ai-docs/key-decisions.md | cut -c1-95` →
``71:**KD-26 — Tests use `testing/synctest`; production code carries no clock abstraction.** The ``].

Go time survives in this package as **intervals only**, and there are exactly the kinds below —
enumerated because round 1 stated the absolute as "no production path reads `time.Now`", which
AC12's loop duration falsifies:

| Go-time value | How it is obtained | Why it is safe |
|---|---|---|
| the poll interval | `time.NewTimer` / `time.Ticker` from configuration | a wait, compared against nothing |
| the per-task execution deadline | `context.WithTimeout` from configuration | a wait, compared against nothing |
| `LoopObservation.Duration` (AC12) | `time.Since` around one cycle — which does read `time.Now`, twice, on the same host | an interval between two Go instants, never compared against a persisted instant and never written to a row |

`Observation.Lag`, by contrast, is `execution instant − run_at` with **both** operands supplied
by the server, and it is the value the absolute exists to protect.

**A task type is a Go registry entry, not a schema value.** The handler registry is constructed
once and is immutable; it maps a task type to its handler and, for a recurrence, to the cadence
that computes the next `run_at` (owner, round 3). The database stores the type as `text` and
validates nothing about it, because the authority is the registry — see D6, which argues that
departure from this repository's every-categorical-column-is-an-enum precedent rather than
assuming it.

**Rejected — one transaction per claim batch with a savepoint per task** (the round-1 design).
It needs one transaction per cycle instead of one per task, and its savepoints give each task
atomicity relative to its own effects. It is rejected because that atomicity is not the property
§11 states: under it a task's committed-to-savepoint effects are discarded when a *later* task in
the same batch breaches its deadline (D11) or when the batch's commit fails, so "a task executes
in one transaction with its effects" holds only as a reading of "subtransaction", and the
derived sentences measured above — which spell out *per task* — become false. Rewording derived
documents to fit a shape their non-editable source forbids is not propagation; it is redesigning
§11 sideways.

**Rejected — one transaction per task with `LIMIT 1`.** It satisfies §11 as literally as the
chosen shape and needs no discovery step, but it contradicts AC5 ("one claim returns at most that
many rows") and leaves AC12's batch-size observation with nothing to report. The discovery claim
costs one extra statement per cycle and buys both criteria back.

**Rejected — a lease/`picked` column with a heartbeat.** It is the standard shape for job runners
that execute *outside* the claiming transaction, and this project specifies the opposite (§11).
With the handler inside the task's transaction there is no stranded state to revive: an aborted
transaction reverts its writes and drops its row locks, leaving the row still due. AC31 asserts
that property directly. The one failure this does not self-heal is a handler that hangs rather
than crashes, and D11 bounds it with a timeout rather than a protocol.

**Rejected — computing backoff and the next cadence instant in SQL.** `run_at = now() + …` keeps
everything server-side, but it makes the growth rule an expression buried in an UPDATE that can
only be tested through the database. Computing them in Go from DB-supplied instants gives
pure functions with exact table tests and one statement that merely writes the answer.

---

### D1 — Package `internal/scheduler`: surface, files, and what it does not import

The name and the headline types are fixed by the spec: `scheduler.Worker`, `scheduler.Handler`,
`scheduler.Task`. Around them:

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
| `Worker` | discover → execute → settle, plus `Run`, `RunOnce`, `Reconcile` |
| `Options` | pool, registry, `config.Scheduler`, optional `Observer` |
| `Observation` / `LoopObservation` / `Observer` / `FailureKind` | the observation seam (D12) |
| `DeadTask` / `DeadTasks` | AC10's enumeration of give-up rows |

Files: `doc.go` (package comment), `task.go`, `registry.go`, `cadence.go`, `schedule.go`,
`worker.go`, `execute.go`, `claim.go`, `reconcile.go`, `observe.go`, `errors.go` — small and one
concern each, because the gated hard limit is 1000 lines for a non-test file and 1500 for a
`_test.go`
[measured ac09e61:Makefile:23-24 · `sed -n '23,24p' Makefile` → `GO_MAX_LINES ?= 1000` / `GO_MAX_TEST_LINES ?= 1500`].

Every method that reaches the database takes `ctx context.Context` first and no type in the
package has a `context.Context` field (AC1). Every exported item carries a doc comment starting
with its name and the package carries a package comment, because `revive`'s `exported` and
`package-comments` rules are enabled
[measured ac09e61:.golangci.yml:45-48 · `sed -n '45,48p' .golangci.yml` → `revive:` / `rules:` / `- name: exported` / `- name: package-comments`].

**What it does not import.** No metrics-registry package (AC13) — the seam is a plain struct plus
a consumer-declared interface, the precedent `internal/tg` already set
[measured ac09e61:internal/tg/observe.go:30-33 · `sed -n '30,33p' internal/tg/observe.go` →
`type Observer interface {` / `// ObserveCall reports one completed outbound call.` / `ObserveCall(Observation)` / `}`].
And **no non-test file imports `internal/store`**: the scheduler never writes a basis document,
because a basis document exists to anchor postings and the code that posts is the handler (spec
*Key decisions*). `internal/store` therefore never appears in this package's non-test dependency
set, and there is no direction in which a cycle could form.

---

### D2 — One cycle, step by step

`RunOnce` is one discovery statement plus one transaction per discovered id.

**Discovery** — a single statement on a pooled connection, outside any explicit transaction, so
its row locks are released when the statement's implicit transaction ends:

```sql
SELECT id FROM scheduled_task
WHERE state = 'pending' AND run_at <= now()
ORDER BY run_at, id
FOR UPDATE SKIP LOCKED
LIMIT $1
```

Zero ids → report the loop observation and return. Otherwise the ids, in order, each get their own
transaction.

**One task's transaction**, on one pooled connection:

1. `BEGIN`, then `SET LOCAL statement_timeout` and `SET LOCAL
   idle_in_transaction_session_timeout`, both to the configured per-task deadline (D11).
2. **Re-claim** — `SELECT id, type, instance_key, payload, run_at, consecutive_failures FROM
   scheduled_task WHERE id = $1 AND state = 'pending' AND run_at <= now() FOR UPDATE SKIP LOCKED`.
   Zero rows means another worker took it, it was already settled, or a recurrence's `run_at` was
   already advanced: `COMMIT` and move to the next id, **silently** — no observation, because
   nothing executed.
3. `SELECT clock_timestamp()` — the task's **execution instant**, the single value from which its
   lag, its retry instant and its next cadence instant are all computed.
4. If the type has no declaration: skip to settlement with `FailureUnregistered` (D7) — the
   handler is never called.
5. `SAVEPOINT` (`tx.Begin`), then run the handler on its own goroutine with a deadline-bearing
   context and wait for either its result or the deadline (D11).
6. **Done** → release the savepoint. **No-op** or **Failed** → *roll back to* the savepoint.
   **Deadline breached** → abandon this task's transaction (D11) and continue the cycle with the
   next id.
7. Settle (D7), `COMMIT`, then emit this task's observation.
8. If the commit fails, emit the observation as a failure with `FailureRolledBack` instead of the
   outcome the handler reported.

After the last id, emit the loop observation.

**Why the no-op path rolls back to its savepoint.** AC8 requires a guard-miss no-op to leave no
writes; AC25 requires the deletion to happen in the same transaction as the effects. Only the
savepoint satisfies both at once — rolling the *whole* transaction back would move the deletion
out of it, and committing without a rollback would make "its writes are absent" a contract the
worker asks handlers to honour rather than a property it enforces.

**Why the observation is emitted after `COMMIT`.** An observation emitted before the commit is a
claim about work that may still roll back — the "green instrument" failure in its purest form.

**Consequences, stated rather than discovered later.** A crash or a deadline breach now costs
**one** task, not a batch: every other task in the cycle has already committed or will commit on
its own transaction. The costs the shape does carry are a transaction per task rather than per
cycle, and a discovery race — two workers polling at the same instant discover overlapping id
sets, and each loses the re-claim on the ids the other took first. The loser's cost is one
primary-key lookup that returns no rows.

---

### D3 — The clock, and where each instant comes from

| Value | Source | Used for |
|---|---|---|
| due-ness | `now()` in the discovery and re-claim `WHERE` | which rows are claimable |
| execution instant | `clock_timestamp()`, one `SELECT` per task | lag, retry instant, next cadence instant |
| `run_at` on insert | `clock_timestamp() + $delay` in the INSERT | a mechanic schedules a *delay*, never an absolute instant |
| `run_at` on reconcile | a `Cadence` result computed from a DB-supplied instant | seed and correction |
| poll interval, execution deadline | Go `time.Duration` from configuration | a wait, compared against nothing |
| loop duration | `time.Since` around one cycle | reported, never persisted or compared against a row |

`Request` therefore carries `Delay time.Duration`, not `RunAt time.Time`: "a wave in five minutes"
is what §3.5's timer edges actually express, and a relative delay keeps the caller out of the
clock business entirely. A negative delay is refused (`ErrInvalidDelay`); zero means due
immediately.

---

### D4 — The claim queries, their index, and what a priority column would change (AC16)

The discovery statement of D2 is served by

```sql
CREATE INDEX scheduled_task_due_idx ON scheduled_task (run_at) WHERE state = 'pending';
```

and the re-claim is a primary-key lookup with the same `state`/`run_at` predicate re-checked, so
it needs no index of its own.

The `LIMIT` is applied **after** rows locked by another transaction are skipped, which is what
makes a small limit safe under concurrency
[measured postgres:18.6 · with the two earliest due rows locked by an open transaction,
`SELECT id … ORDER BY run_at, id FOR UPDATE SKIP LOCKED LIMIT 1` → `5` (the third row), not zero rows;
AC5 re-proves this on the delivered schema].

**AC16, stated as the criterion asks.** Adding the deferred priority/queue column later is **a
column, an index and an ordering**: `ALTER TABLE scheduled_task ADD COLUMN priority smallint NOT
NULL DEFAULT 0`, a new partial index `(priority, run_at) WHERE state = 'pending'`, and `ORDER BY
priority, run_at, id` in the discovery statement. No second table, no Go type rename, no data
backfill (the default fills existing rows). Nothing in this change is shaped against that: `Task`
and `Request` are structs with named fields, so a field is additive; discovery is one statement in
one file; and separate worker pools are separate `Worker` values over the same table, which the
type already permits.

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
CREATE UNIQUE INDEX scheduled_task_identity_key ON scheduled_task (type, instance_key)
    WHERE instance_key IS NOT NULL AND state = 'pending';
CREATE INDEX scheduled_task_due_idx ON scheduled_task (run_at) WHERE state = 'pending';
```

**No `completed` state, no `executed_at`** — a completed one-shot row is deleted (owner, round 1),
so there is nothing for either to describe (AC25). **No liveness machinery** — no column recording
that a task is executing, no heartbeat, no revival path (AC31). **No `kind` column**: whether a
type is one-shot or recurrent is the registry's answer, and duplicating it on the row would create
a second source of truth that a deploy could contradict.

**One counter, not two.** A one-shot never survives a success — it is deleted — so its attempt
count and its consecutive-failure count are the same number, and AC10's "attempt count" reads
`consecutive_failures` directly. A recurrent row's *lifetime* execution count is deliberately not
kept: it would be a history column on a table the owner's answer stripped of history.

**The identity index is partial in two dimensions, and the second one closes a trap.** Round 1
scoped it to `instance_key IS NOT NULL` only. That predicate lets a `state = 'dead'` row occupy an
identity **permanently**: the seed's `ON CONFLICT` finds it and inserts nothing, D10's correction
filters `state = 'pending'` and matches nothing, and `Schedule` hits the unique violation — a
recurrence or a keyed one-shot silently stopped forever, with nothing in the design to revive it.
Adding `AND state = 'pending'` scopes the identity to **live** rows, which is the property the
spec actually asks for: the constraint exists so that *double-seeding a live recurrence* is
unrepresentable (spec *Key decisions*), not so that a settled row keeps its name. Every branch is
measured against the two-predicate index:

- a live duplicate is refused by the database, not by a code path that looks first
  [measured postgres:18.6 · with a pending `('day.close','day.close')` row present, a plain
  `INSERT` of the same identity → `ERROR: duplicate key value violates unique constraint "scheduled_task_identity_key"`;
  AC29 re-proves it on the delivered schema];
- the seed against that live row is a no-op — the `ON CONFLICT` arbiter still infers this index
  when the inference clause repeats both predicates
  [measured postgres:18.6 · `INSERT … ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL AND state = 'pending' DO NOTHING` →
  `INSERT 0 0`, leaving `pending | 1`; AC33 re-proves it];
- the seed against a **dead** row of the same identity inserts, leaving exactly one pending row
  beside it
  [measured postgres:18.6 · with only a `state='dead'` `('day.close','day.close')` row present, the same
  `ON CONFLICT` seed → `INSERT 0 1`, then `pending | dead` = `1 | 1`; AC33 re-proves it];
- a dead keyed one-shot no longer blocks re-scheduling that identity
  [measured postgres:18.6 · a dead `('corpse.evaporate','corpse:42')` row plus a plain `INSERT` of the
  same identity → `INSERT 0 1`, then `pending | dead` = `1 | 1`; AC29 re-proves it];
- a one-shot with no natural identity passes `instance_key` empty, which is stored `NULL`
  (`NULLIF($2, '')`) and falls outside the index's predicate, so two such one-shots of the same
  type coexist
  [measured postgres:18.6 · two inserts of `('one', NULL, …)` → `keyless_rows | 2`; AC29 re-proves it].

The two together are AC29 in both directions, and the dead-row branches are what make "a stopped
chain is healed by the next restart" (spec Scope 7) a property rather than a hope. The consequence
accepted with them: a keyed one-shot that gives up repeatedly leaves one dead row per attempt-set,
and there is no retention (spec *Key decisions*). That is history the owner chose to keep, not a
leak — AC10 is exactly the surface that reads it.

**Payload is `jsonb`, and `jsonb` normalises.** Key order is sorted, whitespace is dropped and a
duplicate key keeps the last value
[measured postgres:18.6 · `SELECT '{"b":1,"a":{"c":null},"b":2}'::jsonb` → `{"a": {"c": null}, "b": 2}`;
AC24 re-proves the round trip on the delivered schema]. So AC24's round-trip is a statement about
the payload's *value*, and its test compares decoded values, never raw bytes — a byte-comparison
would be asserting a property the storage type does not have.

**A payload key is a data contract.** `AGENTS.md`'s API-stability carve-out names the scheduler's
payloads by that word among the live data that outlives every deploy
[measured ac09e61:AGENTS.md:84 · `grep -n "scheduler's .scheduled_tasks. payloads" AGENTS.md` →
one match, at `AGENTS.md:84`, the `> **CARVE-OUT — data contracts are the opposite…**` paragraph].
The package doc comment says so in the same words, and the payload keys this change ships are
named in § Test Design (they belong to test-only types; no production type ships here).

---

### D6 — The task type is `text`, not a PostgreSQL enum — argued, not assumed

The precedent points the other way: every categorical column in migration 00001 is a PostgreSQL
enum
[measured ac09e61:internal/store/migrations/00001_ledger_core.sql:2-4 · `sed -n '2,4p' …/00001_ledger_core.sql` →
`CREATE TYPE owner_kind AS ENUM ('world', 'player', 'chat');` / `ledger_kind` / `operation_source`],
and the spec's *Key decisions* names that precedent as the default. The facts that overturn it for
**this** column, and only this one — `state` stays an enum, because its value set is the schema's
own.

1. **The authoritative registry is in Go, by the owner's round-3 answer.** A recurrence's cadence
   is declared in the handler registry; a type with no declaration must be refused at runtime
   whatever the column's type is (AC21). An enum would therefore be a *second* registry that a
   deploy can contradict — the schema saying a type exists while no handler claims it — and
   keeping the two in step costs a migration per mechanic for no property gained.
2. **This task ships no production task type** (spec *Out of scope*), so the enum would ship with
   no legitimate member and every test would have to mutate the schema with `ALTER TYPE … ADD
   VALUE` before it could insert a row. The migration hygiene test already treats `ADD VALUE` as a
   special, isolated kind of migration file
   [measured ac09e61:internal/store/migrate_test.go:188-190 · `grep -n 'addValueRe.MatchString' -A 3 internal/store/migrate_test.go` →
   `188:  if addValueRe.MatchString(text) {` / `189-   if createTableRe.MatchString(text) || insertRe.MatchString(text) || updateRe.MatchString(text) {` / `190-    t.Errorf("%s: an ADD VALUE file must do nothing else (CREATE TABLE/INSERT/UPDATE found)", entry.Name())`].
3. **§11's greppable-registry argument is about basis-document types, not task types.** That
   argument is honoured exactly where it was made: the `journal_entry` CHECK gains both new basis
   columns by migration (D14). A task type is a different axis — a task whose effects move no
   balance has no basis document at all.

The persisted string is still a data contract: a type name is added, never renamed or repurposed,
and the package doc comment says so. The refusal AC21 requires exists at both ends — `Schedule`
refuses an unregistered type before the INSERT, and the execution path refuses a row whose type
has no declaration (D7), so a type dropped from the registry produces a visible, counted refusal
instead of a row that quietly stays due.

---

### D7 — Settlement: one statement per outcome

Let `t` be the execution instant (D3), `k` the row's `consecutive_failures` **after** this
attempt, `cap`/`base`/`ceiling` the configured retry values, and `rec` the type's declaration.

| Outcome | Kind | Statement |
|---|---|---|
| Done or No-op | one-shot | `DELETE FROM scheduled_task WHERE id = $1` |
| Done or No-op | recurrent | `UPDATE … SET run_at = $2, consecutive_failures = 0, last_error = NULL WHERE id = $1`, `$2 = rec.Next(run_at, t)` |
| Failed | one-shot, `k < cap` | `UPDATE … SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`, `$4 = t + backoff(k, base, ceiling)` |
| Failed | one-shot, `k >= cap` | `UPDATE … SET state = 'dead', consecutive_failures = $2, last_error = $3 WHERE id = $1` |
| Failed | recurrent | `UPDATE … SET consecutive_failures = $2, last_error = $3, run_at = $4 WHERE id = $1`, `$4 = rec.Next(run_at, t)` |
| Failed (`FailureUnregistered`) | **undeclared — no kind to ask** | `UPDATE … SET run_at = $2 WHERE id = $1`, `$2 = t + ceiling`; `state`, `consecutive_failures` and `last_error` untouched |

**"Kind" is the registry's answer, which is why the last row exists.** Round 1's table branched on
`Recurrent?` — a question only a declaration can answer — and then routed an *undeclared* row
through the same table. The only rows it could match were the one-shot ones, so an undeclared
recurrence (a rolled-back deploy, a mis-ordered deploy) reached `state = 'dead'` after `cap`
attempts and was **silently stopped forever**: it stopped coming due, so the `FailureUnregistered`
counter that the spec makes the sole visibility mechanism stopped rising, and the identity was
poisoned besides (D5). That contradicted spec Scope 7, the spec's own "there is no dead recurrent
row for the seeding rule to meet", and its *Open questions* row, which states the property as *"a
row that comes due forever and is refused every time"*.

The last row of the table is that property, made structural. An undeclared row is **never**
executed, so it accrues no attempt; its `consecutive_failures` and `state` are untouched, so it
can never reach `dead`; and its `run_at` is pushed out by the configured backoff **ceiling** so
the refusal recurs at a bounded rate instead of once per poll interval — which also stops a pile
of undeclared rows from occupying the head of the `ORDER BY run_at, id` discovery window and
starving live tasks. The ceiling is reused rather than made a seventh key: it is already "the
longest this scheduler ever defers a retry". When the declaration comes back (the deploy is
re-rolled), the row is claimed within at most that interval, and for a recurrence `Reconcile`
leaves the early occurrence alone exactly as it leaves a lengthened cadence's early occurrence
alone (D10).

The remaining properties fall straight out and are what the tests assert. A recurrence **never**
reaches `dead`: no row above sets it for a declared recurrence, no row above sets it for an
undeclared one, and the cap is consulted only on the one-shot branch (AC32) — which is the scope
boundary the spec asked to be stated rather than implied, because applying the cap to both kinds
would silently reintroduce the terminal state the owner removed. A recurrence is **one row moved
forward in place**, so the live-row count for one recurrence is one at every commit boundary
(AC11). And `consecutive_failures` rises on each failure and resets on a success *or* a no-op
(AC34).

**The one path that still reaches `dead` with a live identity**, named so it is not a surprise: a
type whose declaration changes from recurrent to one-shot between deploys, whose row then fails
`cap` times. D5's live-scoped index is what keeps that survivable — the dead row does not hold the
identity, so a later re-declaration is seeded fresh.

**A decode failure is just a handler error.** AC26 needs it classified as a failure and terminated
by the cap, which the one-shot branch already does, with the decode message landing in
`last_error` as AC26's "recorded reason". No sentinel is imposed on handlers for this.

`state = 'dead'` rows are never claimed again — discovery and re-claim both filter
`state = 'pending'` — and are enumerable through `DeadTasks(ctx, q, limit)`, ordered by
`run_at, id`, each carrying type, instance key, `run_at`, attempt count and `last_error` (AC10).

---

### D8 — Backoff and cadence as pure functions

```go
func backoff(failures int, base, ceiling time.Duration) time.Duration   // min(base·2^(failures-1), ceiling)
func Every(period time.Duration) Cadence                                // smallest k ≥ 1 with prev + k·period > now
```

Both are pure, take no clock, and are tested by exact table tests. `backoff` is strictly positive
and strictly growing until it reaches the ceiling, which is what AC9's "grows rather than repeats"
asks of the attempts inside the cap. **No jitter**: the transport's backoff jitters because many
callers race one remote rate limit, while `SKIP LOCKED` already de-collides workers, and a
deterministic delay is exactly assertable.

`Every`'s catch-up rule matters: after an outage, a recurrence does not fire once per missed
period — it advances to the next instant strictly after `now`, so a day-close that missed a week
runs once, not seven times.

`Cadence` is a function type rather than a duration so that the mechanic which needs a wall-clock
anchor (the day close, #45) can supply its own without touching this package. This task ships
`Every` only; `Recurrence` pairs a cadence with the **configuration key its value came from**, and
the registry refuses a recurrence whose key is empty — that is what keeps "a declaration names
which key it reads, it does not embed a number" checkable rather than aspirational.

---

### D9 — The registry is immutable by construction

```go
func NewRegistry(decls ...Declaration) (*Registry, error)
```

Built once, never mutated, safe for concurrent use without a lock — the same posture
`internal/tg`'s `Options` takes
[measured ac09e61:internal/tg/client.go:17 · `grep -n 'type Options struct' internal/tg/client.go` → `17:type Options struct {`].
Construction refuses an empty type, a nil handler, a duplicate type, and a recurrence with a nil
cadence or an empty configuration key, each wrapping `ErrInvalidDeclaration` with the offending
type named.

`Registry` is also the insertion surface:
`(*Registry).Schedule(ctx, tx, Request) (TaskID, error)` takes a **caller-owned** `pgx.Tx` and
neither commits nor rolls back, exactly as `store.Post` does
[measured ac09e61:internal/store/post.go:74 · `grep -n '^func Post' internal/store/post.go` →
`74:func Post(ctx context.Context, tx pgx.Tx, basis PostingBasis, postings ...Posting) error {`].
That is not a convenience: §3.5 schedules a raid session's timer edges **inside the transition's
own transaction**, so a transaction-taking insert is the only shape that keeps a transition and
its timers atomic. `Schedule` returns `ErrUnknownType`, `ErrInvalidDelay`, `ErrDuplicateTask`
(SQLSTATE 23505 on `scheduled_task_identity_key`) or `ErrInvalidPayload`.

`Schedule` deliberately does **not** adopt or clear a dead row of the same identity: under D5's
live-scoped index there is nothing in its way, and a caller re-scheduling an identity that
previously gave up should leave that dead row for AC10 to report.

---

### D10 — Start-up reconciliation: seed, correct, nothing else

`(*Worker).Reconcile(ctx)` runs the two moves the owner's round-3 answer names, for each declared
recurrence, and no third move.

**Seed** — one statement, idempotent by the constraint rather than by a lock, so several workers
starting at once is safe by construction (AC33):

```sql
INSERT INTO scheduled_task (type, instance_key, payload, run_at)
VALUES ($1, $2, '{}'::jsonb, $3)
ON CONFLICT (type, instance_key) WHERE instance_key IS NOT NULL AND state = 'pending' DO NOTHING
```

The payload is the literal empty object, not a parameter: neither `Declaration` nor `Recurrence`
carries a payload, because a recurrence's occurrence is identified by its type and instance key
and has nothing type-specific to say. `payload` is `NOT NULL` (D5), so the column needs a value
and `'{}'::jsonb` is it. A recurrence whose mechanic later needs payload data gets it by widening
`Recurrence`, not by relaxing the column.

`$3` is `rec.Next(now, now)` with `now` read from the server — one cadence ahead, never
"immediately", because a fresh deploy must not fire a day close on start-up. The inference clause
repeats **both** of the index's predicates, which is what lets the arbiter match it; that, and the
dead-row branch this seed now heals, are measured in D5.

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

*In flight* is `FOR UPDATE SKIP LOCKED` — a row a worker is currently executing is skipped, not
waited on. *Imminent* is derived rather than configured: an occurrence that could be claimed
before the correction takes effect is one within a poll interval, which needs no seventh tuning
key. The disagreement test is the same expression as the correction, which is what makes the
statement idempotent — a healthy recurrence always satisfies `run_at <= rec.Next(now, now)`, so
re-running it is a no-op.

**The correction fires in exactly one direction, and § Test Design names it.** `run_at > $3` is
true only when the row's next occurrence is *later* than the declaration now says it should be —
i.e. the cadence was **shortened**. A **lengthened** cadence leaves `run_at <= $3`, no row
matches, and the change is absorbed by one early occurrence that then advances on the new cadence.
A test written against "a row that disagrees" without naming the direction can pick the branch
that never fires and pass forever.

**No third move.** There is no revive branch, because a declared recurrence has no dead state to
revive from — under D7's undeclared-row settlement no path puts one there, and under D5's index a
dead row would not block the seed anyway. And there is no collector of rows whose type is no
longer declared: retiring a recurrence deletes its row in the same change that drops its
declaration (spec *Key decisions*), and an automatic deleter would remove the evidence of the
mistake it covers for.

---

### D11 — The per-task execution deadline, and who owns what afterwards

The transactional shape self-heals a worker that *crashes*. It does not self-heal a handler that
*hangs*: its transaction stays open, its row stays locked, and `SKIP LOCKED` means every other
worker passes over that row silently and forever. AC30 bounds it. The layers, each covering what
the others cannot:

1. **The handler's context** — `context.WithTimeout(ctx, deadline)`. Every pgx call the handler
   makes through its `pgx.Tx` fails once it expires, so a handler that merely blocks on the
   database stops.
2. **The database** — `SET LOCAL statement_timeout` and `SET LOCAL
   idle_in_transaction_session_timeout`, both set to the deadline at the top of the task's
   transaction. The first bounds a single long statement and leaves the transaction recoverable
   through the savepoint
   [measured postgres:18.6 · `SET LOCAL statement_timeout='200ms'; SAVEPOINT h; SELECT pg_sleep(2); ROLLBACK TO SAVEPOINT h; SELECT count(*); COMMIT` →
   `ERROR: canceling statement due to statement timeout` / `ROLLBACK` / `stmt_timeout_recovered | 3` / `COMMIT`].
   The second is the one that matters for a handler hung in Go code: the server terminates the
   backend, which aborts the transaction and releases its row locks **without our process
   participating at all**
   [measured postgres:18.6 · a session holding `id = 8` under `SET LOCAL idle_in_transaction_session_timeout = '2s'`;
   a concurrent `… WHERE id = 8 … FOR UPDATE SKIP LOCKED` at `t≈0.5s` → `rows=[]`, the same statement at
   `t≈3.0s` → `rows=[8]`, and the holder's next statement → `FATAL: terminating connection due to idle-in-transaction timeout`;
   AC30 and AC31 re-prove the pair on the delivered schema]. Both settings are transaction-scoped
   and settable by an **ordinary, non-superuser** role
   [measured postgres:18.6 · as a role with `rolsuper = f`, `BEGIN; SET LOCAL statement_timeout='250ms'; SET LOCAL idle_in_transaction_session_timeout='250ms'; SELECT current_setting(…), current_setting(…)` →
   `250ms | 250ms`].
3. **The worker stops waiting.** Each handler runs on its own goroutine and the worker selects on
   the result or the deadline. Without this the worker could not *report* the breach, which AC30
   requires; the layers above release the row but say nothing.

**On a breach** the worker reports `FailureDeadline` for that task, abandons **that task's**
transaction — not the cycle — and continues with the next discovered id. Every other task in the
cycle is on its own transaction and is unaffected, which is the second thing the per-task shape
buys after §11.

**Who owns the connection afterwards, and when it is closed.** The worker must not touch `tx`
again: the orphaned handler goroutine may issue a statement on that connection at any moment, and
a pgx connection is not safe for concurrent use. So the worker **hijacks** it —
`pgxpool.Conn.Hijack` takes it out of the pool so the pool can never hand it to another caller,
and its own doc makes the ownership transfer explicit
[measured pgx/v5@v5.10.0:pgxpool/conn.go:69-73 · `sed -n '69,73p' pgxpool/conn.go` →
`// Hijack assumes ownership of the connection from the pool. Caller is responsible for closing the connection. Hijack` /
`// will panic if called on an already released or hijacked connection.` / `func (c *Conn) Hijack() *pgx.Conn {` /
`if c.res == nil {` / `panic("cannot hijack already released or hijacked connection")`].
It is **not** closed immediately, and that is deliberate: closing underneath a goroutine that may
be mid-statement is a use-after-close race. Instead a watchdog goroutine waits on the handler
goroutine's completion signal and then calls `(*pgx.Conn).Close`
[measured pgx/v5@v5.10.0:conn.go:301 · `grep -n '^func (c \*Conn) Close' conn.go` → `301:func (c *Conn) Close(ctx context.Context) error {`].
`Hijack` panics on a connection already released, so it is called at most once and the release
path is guarded by the same flag; no path reaches both.

**The bound AC30's test waits on is ≈ 2 × `LAB_GAME_SCHEDULER_TASK_TIMEOUT`**, measured from the
handler's start, and the test must be written against that rather than against one deadline. The
worker gives up one deadline after the handler starts, but the *row* is released by layer 2, whose
clock starts when the session goes **idle** — so a handler whose last statement completes just
before the worker's deadline fires pushes the release out to nearly twice the deadline. A handler
that blocks without touching the database at all goes idle immediately and is released at ≈ 1 ×.

**The residue, named rather than hidden.** The orphaned handler goroutine survives until it
touches the dead connection (its next call fails, because the server has terminated that backend)
or returns. A handler that neither returns nor touches the database is an infinite loop in Go
code, which no scheduler can reclaim: it leaks one goroutine and one client-side file descriptor
per breach, bounded by the number of breaches, and the server-side resources are already released
by layer 2. It is a handler defect and the deadline observation is what makes it visible. Nothing
in this path panics — `Hijack`'s panic is guarded by its documented precondition, and the
project's panic index holds no row today and gains none here
[measured ac09e61:ai-docs/panic-index.md · `sed -n '/^| File:line/,$p' ai-docs/panic-index.md` →
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
criterion or by an open question the spec left standing: `unregistered` is the refusal counter the
spec names as the only signal that an undeclared recurrence's row is coming due forever — and
under D7 that row now keeps coming due, so the counter keeps rising, which is what the spec's
property needs; `deadline` is AC30's "reported as a failure rather than passing silently";
`rolledBack` is what stops D2's emit-after-commit from reporting a phantom success when the
task's own commit fails. `exhaustive` is enabled with `default-signifies-exhaustive: true`, so
every switch over `Outcome` or `FailureKind` is total or carries a default
[measured ac09e61:.golangci.yml:40-41 · `sed -n '40,41p' .golangci.yml` → `exhaustive:` / `default-signifies-exhaustive: true`].

`BatchSize` is the **discovery** cardinality (D2), which is what AC12's "the size of the claim
batch the task came from" names; a task skipped at re-claim produces no observation at all,
because nothing executed.

`ConsecutiveFailures` is carried on every observation and is the field AC12 adds beyond issue
#20's own list, for the reason the spec gives: a per-type failure *rate* cannot separate one
recurrence failing every time from many recurrences failing occasionally, and only the first is an
outage. On a `FailureUnregistered` observation it carries the row's stored value unchanged, since
D7 does not increment it.

`Observer` is optional. A nil observer is checked, not called, so the package compiles and its
tests pass with no implementation installed (AC13). `LoopObservation.Err` exists because `Run`
must **not** stop on a transient discovery failure — without it the error would be silently
dropped, and this package has no logger. `LoopObservation.Duration` is the one Go-measured
interval in the package (§ Approach).

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
corpse TTL, an escalation timer) is set by the mechanic that schedules it, from the balance file,
never here. `RETRY_MAX_DELAY` carries a second job under D7: it is also the interval at which an
undeclared row's refusal repeats.

The mechanics of the existing layer that bind, and are followed exactly:

- `loadScheduler` reuses `lookupPositiveInt` and `lookupPositiveDuration` — already in the
  package, so no helper is duplicated
  [measured ac09e61:internal/config/transport.go:207,223 · `grep -n '^func lookupPositive' internal/config/transport.go` →
  `207:func lookupPositiveInt(lookup Lookup, key string) (int, bool, error) {` / `223:func lookupPositiveDuration(lookup Lookup, key string) (time.Duration, bool, error) {`].
- The keys go into `EnvKeys()`, **never** into the unexported `envKeys()`, which the
  required-variable suites iterate — putting an optional key there fails them immediately, and
  loosening those suites would attack the requiredness they exist to hold
  [measured ac09e61:internal/config/env.go:33-35 · `sed -n '33,35p' internal/config/env.go` →
  `func envKeys() []string {` / `return []string{envBotToken, envDSN, envBotAPIBaseURL, envAllowedChatIDs}` / `}`].
- Every key is queried **unconditionally** on every load, because the disjointness test's
  recording lookup compares the set of keys the loader actually consulted against `EnvKeys()` and
  `.env.example`
  [measured ac09e61:internal/config/disjoint_test.go:103-104 · `sed -n '103,104p' internal/config/disjoint_test.go` →
  `assertSameKeySet(t, ".env.example", exampleKeys, "config.EnvKeys()", EnvKeys())` /
  `assertSameKeySet(t, "loader-consulted keys", recorded(), "config.EnvKeys()", EnvKeys())`].
- Each key gets a `.env.example` line carrying its default as a **non-empty** value, because one
  test requires every value non-empty and another requires the whole file to load
  [measured ac09e61:internal/config/disjoint_test.go:108-115 · `sed -n '108,115p' internal/config/disjoint_test.go` →
  `func TestEnvExample_ValuesAreNonEmpty(t *testing.T) {` … `t.Errorf(".env.example: %s has an empty value", k)`].

`Config` gains a `Scheduler` field and `Load` calls `loadScheduler` alongside `loadTransport`,
joining failures the same way. `scheduler.Options` carries `config.Scheduler` by value, the shape
`tg.Options` already uses for `config.Transport`, and `scheduler.New` refuses any non-positive
member with an `*OptionError` naming the field — which is also what makes AC14's "no literal value
at a call site in `internal/scheduler`" a property rather than a promise.

---

### D14 — The basis documents, the CHECK migration, and its rollback

Migration `internal/store/migrations/00002_scheduler.sql`, forward-only, with **no `-- +goose
Down` section** — the package has none and a test enforces it
[measured ac09e61:internal/store/migrate_test.go:174 · `grep -n 'contains -- +goose Down' internal/store/migrate_test.go` →
`174:    t.Errorf("%s: contains -- +goose Down", entry.Name())`].

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

This is the shape migration 00001 established, extended rather than reinvented: one nullable FK
column per basis type, one `CHECK (num_nonnulls(…) = 1)` naming all of them, one partial unique
index per basis column
[measured ac09e61:internal/store/migrations/00001_ledger_core.sql:70-75 · `sed -n '70,75p' …/00001_ledger_core.sql` →
`player_operation_id  bigint      REFERENCES player_operation (id),` … `CONSTRAINT journal_entry_exactly_one_basis CHECK (num_nonnulls(player_operation_id, manual_correction_id) = 1)` … `CREATE UNIQUE INDEX journal_entry_player_operation_key  ON journal_entry (player_operation_id)  WHERE player_operation_id  IS NOT NULL;`].
The partial predicate is also what keeps the FK-coverage test green, since it accepts an index
whose predicate is an `IS NOT NULL`
[measured ac09e61:internal/store/fkcover_test.go:85 · `sed -n '85p' internal/store/fkcover_test.go` →
`if idx.pred != nil && !strings.Contains(strings.ToUpper(*idx.pred), "IS NOT NULL") {`].

**Rows written before this migration.** Both new columns are `NULL` for every existing
`journal_entry`, so `num_nonnulls` is unchanged for them and the re-added CHECK validates without
a single row failing. The CHECK swap and the column additions are one `ALTER TABLE` inside the
migration's transaction, so no window exists in which the table has the columns but not the
constraint.

**The rollback, stated as the rule requires: it is another forward migration, never a down
section.** A `00003` would drop `journal_entry_deferred_task_key` /
`journal_entry_recurrent_task_key`, drop the two columns, restore the two-column CHECK, and drop
`scheduled_task`, `recurrent_task`, `deferred_task` and `scheduled_task_state`. It is safe **only
while no `journal_entry` row has either column non-null** — once a mechanic has posted under one
of these documents, the rollback is a data-destroying operation and the answer is a compensating
migration, not a reversal.

**The Go types live in `internal/store` — forced, not chosen.** `PostingBasis`'s methods are
unexported, so no other package can satisfy it
[measured ac09e61:internal/store/basis.go:18-25 · `sed -n '18,25p' internal/store/basis.go` →
`type PostingBasis interface {` / `entrySQL() (string, error)` / `insert(ctx context.Context, tx pgx.Tx) (int64, error)`].
`DeferredTask` and `RecurrentTask` are `internal/store` types beside `PlayerOperation` and
`ManualCorrection`, each carrying `TaskType string`, `InstanceKey string` (empty stored as `NULL`
via `NULLIF`) and `RunAt time.Time` — **by value, with no foreign key back to `scheduled_task`**
(AC27), because delete-on-done means the task row is gone while its basis document and postings
live on. And the doc comment that today says the sum type has "exactly the store package's two
implementations" is rewritten to describe the set the package actually has (AC20)
[measured ac09e61:internal/store/basis.go:11-13 · `sed -n '11,13p' internal/store/basis.go` →
`// PostingBasis is the sealed sum type of documents a journal_entry may` / `// reference — exactly the store package's two implementations,` / `// *PlayerOperation and *ManualCorrection (D8).`].

**No posting signature ships here.** The scheduler moves no balance of its own; each mechanic
declares the signature of the effects it posts under a deferred one-shot or a recurrent task when
it ships its handler (`docs/DESIGN.md` §13.4, spec *Out of scope*).

---

### D15 — Propagation: the sites this diff falsifies, and the one word authorised in `docs/DESIGN.md`

Membership is decided by `AGENTS.md` § Propagation Rule step 4 — every live doc must agree,
history surfaces are left untouched. The list below is the class as known at design time;
membership is re-derived at Step 8 by the same rule, not bounded by this table.

Two claims travel together through §11 and its derived documents, and they need separate verdicts:
the **table's spelling**, which this change corrects at every live site, and the **"one
transaction per task with its effects"** invariant, which round 1's batch shape falsified and the
chosen shape does not. The sites the round-1 design listed for the spelling only — plus
`ai-docs/domain-invariants.md`, which it omitted entirely although `AGENTS.md` lists that file
among the pages read on nearly every task
[measured ac09e61:AGENTS.md:260,265 · `grep -n 'Read on nearly every task' AGENTS.md` → `260:Read on nearly every task:`;
`sed -n '265p' AGENTS.md` → ``| [`ai-docs/domain-invariants.md`](…) | Ledger, telemetry, scheduler and Telegram-safety invariants |``] — are the reason the shape changed rather than the
wording (§ Approach).

| Site | What becomes false | Verdict |
|---|---|---|
| `docs/DESIGN.md` §11, the table's spelling | spells the table `scheduled_tasks` | **corrected — one word, under the owner's round-5 request** (below) |
| `docs/DESIGN.md` §11, "в одной транзакции со своими эффектами" | nothing | **untouched, and true of the delivered shape** |
| `AGENTS.md` § API Stability carve-out | spells the table `scheduled_tasks` | spelling only |
| `ai-docs/key-decisions.md` KD-4 | spells the table `scheduled_tasks`; also says "each task executing in one transaction with its effects" | spelling only — the invariant clause **survives verbatim** |
| `ai-docs/context.md` Scheduler block row | spells the table `scheduled_tasks`; also says "one transaction per task with its effects" | spelling only — the invariant clause **survives verbatim** |
| `ai-docs/domain-invariants.md` § 4, "A task executes in one transaction with its effects" | nothing — it does not name the table | **no edit** — recorded here because it is the invariant the shape was chosen to keep, and a reviewer must be able to see it was checked rather than missed |
| `ai-docs/key-decisions.md` KD-27 | "the relaxation reaches **only** these keys" — a second optional-with-default class now exists | reworded |
| `ai-docs/context.md` Architecture + Status paragraphs | do not name `internal/scheduler` | extended |
| `internal/store/basis.go` | "exactly the store package's two implementations" | rewritten (AC20) |
| `internal/config/env.go` header comment | names the transport keys as *the* optional-with-default class | reworded |
| `internal/config/config.go` `Config` doc comment | "Transport is the one exception" | reworded |
| `internal/store/migrate_test.go` | hard-codes the table list, the `goose_db_version` row count and the index list | updated with the migration (subtask 1) |
| `.env.example` | does not document the new keys | extended |
| `ai-docs/plans/INDEX.md` | has no row for this pair | row added |

The plural-spelling rows and the invariant sentences are measured, not remembered
[measured ac09e61 · `grep -rn 'scheduled_tasks' AGENTS.md ai-docs/context.md ai-docs/key-decisions.md docs/DESIGN.md | cut -c1-60` →
`AGENTS.md:84:> **CARVE-OUT — data contracts are the opposite` /
`ai-docs/key-decisions.md:15:**KD-4 — A self-written schedule` /
``ai-docs/context.md:36:| Scheduler | `scheduled_tasks` worker`` /
`docs/DESIGN.md:310:  - Единственное исключение — **шедулер тасок`;
the invariant sentences are quoted with their locators in § Approach], and so is the
`env.go` sentence this change falsifies
[measured ac09e61:internal/config/env.go:11-15 · `sed -n '11,15p' internal/config/env.go` →
`// Environment variable names, LAB_GAME_ prefixed …` … `// tuning variables (transport.go) are a separate, optional-with-default` / `// class, added on top by EnvKeys() (design D10).`]
and KD-27's boundary sentence
[measured ac09e61:ai-docs/key-decisions.md:73 · `grep -o 'The boundary, stated exactly:[^.]*\.' ai-docs/key-decisions.md` →
`The boundary, stated exactly:* the relaxation reaches **only** these keys.`].

**`docs/DESIGN.md:310` — the authorisation is for ONE WORD, and the implementor meets that bound
at the point of the edit.** `AGENTS.md` § Project holds the design document to "**Implement from
it. Never redesign it** without an explicit user request"
[measured ac09e61:AGENTS.md:15 · `grep -n 'Never redesign it' AGENTS.md | cut -c1-120` →
`15:> | `docs/DESIGN.md` | **Implement from it. Never redesign it** without an explicit user request.`];
the product owner supplied exactly that request at round 5, and the spec records it in Scope 11,
*Key decisions* and *Source conflicts*
[measured ac09e61:ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md · `grep -n 'explicit round-5 request\|round-5 amendment\|owner gave the explicit request' …spec.md` →
matches in *Out of scope*, the *Key decisions* row on §11's plural spelling, the Acceptance-Criteria preamble, and AC22].
What it authorises: the table's spelling in §11 becomes `scheduled_task`, so §11's prose obeys the
singular-table-names decision §11 states itself — the disagreement the spec's *Source conflicts*
records, with its own pin on both lines, and which this design does not re-derive because
`docs/DESIGN.md`'s rules are cited by section, not by line
[measured ac09e61:ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md:373-377 ·
`sed -n '373,377p' …spec.md` → the two quoted §11 lines and their `[source: e64bcc6:docs/DESIGN.md:…]` pins].

**The trap on that same line, stated because the implementor will be looking straight at it.**
Line 310 also carries *"Таска исполняется **в одной транзакции со своими эффектами** —
exactly-once без двухфазных танцев."* — the edit target's own text, so it is pinned by line here
rather than by section
[measured ac09e61:docs/DESIGN.md:310 · `sed -n '310p' docs/DESIGN.md` → one line containing both
``таблица `scheduled_tasks` (run_at, тип, payload, статус)`` and `Таска исполняется **в одной транзакции со своими эффектами** — exactly-once без двухфазных танцев.`].
That clause is **not** authorised, not adjustable, and not
in need of adjustment: the shape this design delivers is one transaction per task, chosen so the
sentence stays literally true (§ Approach). Editing it — to admit a batch, to gloss
"subtransaction", to soften it in any direction — would be redesigning §11 under cover of a
spelling fix, which is precisely what the authorisation excludes. The edit is: the token
`scheduled_tasks` → `scheduled_task`, and nothing else on that line, in that paragraph, or in that
section. `docs/**` is Russian by decision, so nothing on the line is translated either
[measured ac09e61:AGENTS.md:4 · `grep -n 'Russian for two surfaces only' AGENTS.md | cut -c1-190` →
`4:1) **English for every durable artefact** … **Russian for two surfaces only:** conversation w`].

**AC19's exclusion set is exactly the classes the spec names, and no others.** After this change the
string `scheduled_tasks` survives only at:

- `.gitignore`, where the match is the harness lock-file path `.claude/scheduled_tasks.lock` — a
  file belonging to the agent harness, not a spelling of this table at all, and renaming it would
  break the harness
  [measured ac09e61:.gitignore:35 · `grep -n 'scheduled_tasks' .gitignore` → `35:.claude/scheduled_tasks.lock`];
- any file under `ai-docs/plans/done/`, which is history rather than a live surface;
- this task's own spec and design, which quote the superseded spelling as the evidence for
  correcting it and would falsify themselves if they could not.

`docs/DESIGN.md` is **not** in that set — it is corrected, per the row above. Round 1 excluded it,
and that exclusion is withdrawn.

---

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Migration `00002_scheduler.sql`: `scheduled_task_state`, `scheduled_task` with the live-scoped identity index, the due index and its CHECKs, `deferred_task`, `recurrent_task`, the `journal_entry` column + CHECK + partial-unique-index extension (D5, D14). Update the migration suite: table list, `goose_db_version` row count, index list, `journal_entry_exactly_one_basis` definition; add the `scheduled_task` column-set assertion that carries AC31 and the AC3 shape assertion | `internal/store/migrations/00002_scheduler.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 2 | `store.DeferredTask` / `store.RecurrentTask` implementing `PostingBasis`; rewrite the `PostingBasis` doc comment (AC20); tests for posting under each new basis, the two-non-null refusal, and ledger survival of a deleted task row (AC4, AC27) | `internal/store/basis.go`, `internal/store/basis_test.go`, `internal/store/schema_test.go` | 1 |
| 3 | `config.Scheduler`, its defaults, `loadScheduler`, `schedulerEnvKeys()`, the `EnvKeys()` append, the `Config.Scheduler` field and `Load` wiring; the falsified doc comments in `env.go` and `config.go`; the `.env.example` block; the absent/present/malformed and example-matches-defaults tests (D13, AC15) | `internal/config/scheduler.go`, `internal/config/env.go`, `internal/config/config.go`, `internal/config/scheduler_test.go`, `.env.example` | — |
| 4 | Package foundation with no database: `doc.go`, `errors.go`, `task.go` (`Type`, `TaskID`, `Task`, `Request`, `Outcome`, `DeadTask`), `observe.go` (`Observation`, `LoopObservation`, `Observer`, `FailureKind`), `registry.go` (`Declaration`, `Recurrence`, `Registry`, `NewRegistry`), `cadence.go` (`Cadence`, `Every`) and the pure `backoff`; exact table tests for `backoff` and `Every` and the registry's refusals (D1, D8, D9, D12). Depends on nothing: none of these files reads `config.Scheduler`, which arrives with `Options` in subtask 6 | `internal/scheduler/doc.go`, `errors.go`, `task.go`, `observe.go`, `registry.go`, `cadence.go`, `cadence_test.go`, `registry_test.go` | — |
| 5 | The insertion surface: `(*Registry).Schedule` and `DeadTasks`; the package's `TestMain` and schema fixture; tests for the payload round-trip, the unregistered-type refusal at insertion, the duplicate-identity refusal, the coexisting keyless one-shots, and re-scheduling an identity whose earlier row is dead (AC21 insertion, AC24, AC29) | `internal/scheduler/schedule.go`, `schedule_test.go`, `scheduler_test.go` | 1, 4 |
| 6 | The execution cycle: `Options`, `New`, `RunOnce`, the discovery statement, the per-task transaction with its re-claim and handler savepoint, the Done and No-op settlement paths; tests for batch bounding and skip-not-wait, two concurrent workers, effects-and-settlement atomicity, the no-op's absent writes, the silent skip of a row another worker took, delete-on-done both directions and the recurrence's single live row (AC5, AC6, AC7, AC8, AC11, AC25) | `internal/scheduler/worker.go`, `execute.go`, `claim.go`, `worker_test.go` | 5 |
| 7 | The failure policy: attempt counting, persisted backoff, the one-shot give-up, the recurrent reschedule-to-next-cadence, and the undeclared-type settlement that never reaches `dead` and never increments the counter; tests for exact attempt counts and the terminal state, dead-row enumeration, the decode failure, the never-terminal recurrence, the undeclared recurrence still coming due after more than `cap` refusals, and the rising/resetting counter (AC9, AC10, AC21 execution time, AC26, AC32, AC34) | `internal/scheduler/execute.go`, `failure_test.go` | 6 |
| 8 | The loop and the seam: `Run`, the poll interval, emit-after-commit, the loop observation; tests collecting observations for a success, a no-op, a retry, a give-up and a repeatedly failing recurrence, plus non-default tuning values changing observed behaviour and the nil-observer path (AC12, AC13, AC14 worker half) | `internal/scheduler/worker.go`, `observe.go`, `observe_test.go` | 7 |
| 9 | The per-task execution deadline: the `SET LOCAL` statement and idle-in-transaction timeouts, the handler goroutine, abandonment of that task's transaction, the connection hijack and its watchdog close, `FailureDeadline` / `FailureRolledBack`; tests for the blocked handler, the row becoming claimable within ≈ 2 × the deadline, the observation, the untouched neighbours in the same cycle, and the aborted-transaction self-healing property (AC30, AC31 behavioural half) | `internal/scheduler/execute.go`, `worker.go`, `deadline_test.go` | 8 |
| 10 | `Reconcile`: the seed and the correction; tests for the missing-row seed, the seed against a dead row of the same identity, the shortened-cadence correction, two concurrent start-ups producing one row, and the imminent occurrence left alone (AC33) | `internal/scheduler/reconcile.go`, `reconcile_test.go` | 8 |
| 11 | Propagation (D15): the table's spelling at its live sites — **`docs/DESIGN.md:310` included, one word only, per D15's bound** — KD-27's boundary sentence, KD-4, the `context.md` Architecture/Status/Scheduler-row updates, the `INDEX.md` row | `docs/DESIGN.md`, `AGENTS.md`, `ai-docs/context.md`, `ai-docs/key-decisions.md`, `ai-docs/plans/INDEX.md` | 1–10 |

---

## Handoff plan

- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–10 (code change-type: `*.go`, `*.sql` migrations, and `.env.example`, which
  is the Go configuration loader's own gated manifest and is asserted by a Go test, so it travels
  with the code that reads it). Entering this group is itself a handoff: spawn `/context-reset`
  per `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry) before subtask 1.
  All same-change-type subtasks are clustered into ONE group rather than interleaved with the
  documentation work, which is the minimization (f) requires; the group is at the size cap of 10.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task` resumes
  in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not pinned** — 1M-token window, via the `general-purpose` subagent with no
  inline `model=` override — subtask 11 (instructions/harness change-type: `*.md` only —
  `docs/DESIGN.md`, `AGENTS.md`, `ai-docs/**`). Terminal group (1 subtask; within the `1..=10`
  range).

Two groups, within the default maximum of 4; no user approval is required.

---

## Risks

- **A hung handler is the one failure the transactional shape does not self-heal**; a row locked
  forever is invisible, because `SKIP LOCKED` makes every other worker pass over it silently.
  Mitigated by D11's layers, the database's being the one that does not depend on our process —
  `[measured postgres:18.6 · a session holding `id = 8` under `SET LOCAL idle_in_transaction_session_timeout = '2s'`; a concurrent `… WHERE id = 8 … FOR UPDATE SKIP LOCKED` at `t≈0.5s` → `rows=[]`, at `t≈3.0s` → `rows=[8]`; the holder's next statement → `FATAL: terminating connection due to idle-in-transaction timeout`]`.
- **The row's release after a deadline breach takes up to ≈ 2 × the configured deadline**, because
  layer 2's clock starts when the session goes idle, not when the handler starts. A test written
  against a single deadline would be flaky by construction —
  `[derived → the AC30 test waits the doubled bound named in D11]`.
- **The orphaned handler goroutine after a deadline breach** survives until it touches the
  hijacked connection; the watchdog closes that connection only when the goroutine finishes, so a
  handler that neither returns nor touches the database leaks one goroutine and one client file
  descriptor per breach — `[derived → the AC30 test asserts the row is claimable and the observation made; the doc comment names the residue]`.
- **A discovery race wastes work**: two workers polling at the same instant discover overlapping
  ids and each loses the re-claim on the ids the other took first. Bounded — the loser's cost is a
  primary-key lookup returning no rows, and no observation is emitted —
  `[measured postgres:18.6 · a second worker's `… WHERE id = 5 … FOR UPDATE SKIP LOCKED` against a row held by an executing transaction → `(0 строк)`, and the same statement for the free `id = 6` → `6`]`.
- **One transaction per task costs a transaction per task.** The cycle now opens `BEGIN`/`COMMIT`
  per discovered id instead of once per batch. Accepted deliberately: it is what keeps §11's
  sentence and its derived sentences literally true (§ Approach, D15), and the extra work is
  a `BEGIN`, the `SET LOCAL` statements and a primary-key `SELECT … FOR UPDATE` — `[derived → the AC5/AC6 tests, which run whole cycles against a real server]`.
- **A dead identity that never frees would stop a chain forever.** Closed by scoping the identity
  index to live rows and by never routing an undeclared row to `dead` (D5, D7) —
  `[measured postgres:18.6 · with only a dead `('day.close','day.close')` row, the D10 seed → `INSERT 0 1`, leaving `pending | dead` = `1 | 1`; with a pending row, the same seed → `INSERT 0 0`]`.
- **`pgxpool.Conn.Hijack` panics on an already-released connection**, and this project targets zero
  production panics — `[measured pgx/v5@v5.10.0:pgxpool/conn.go:69-73 · `sed -n '69,73p' pgxpool/conn.go` → `// Hijack assumes ownership of the connection from the pool. Caller is responsible for closing the connection. Hijack` … `panic("cannot hijack already released or hijacked connection")`]`.
  Mitigated by a single guarded call site: one flag decides hijack-or-release, and no path reaches
  both. No row is added to the panic index.
- **The migration suite hard-codes facts this migration changes** — the table list, the
  `goose_db_version` row count and the index list all fail unless updated in the same subtask —
  `[measured ac09e61:internal/store/migrate_test.go:36 · `grep -n 'want := \[\]string{' internal/store/migrate_test.go` → `36:	want := []string{`; ac09e61:internal/store/migrate_test.go:131 · `grep -n 'goose_db_version rows' internal/store/migrate_test.go` → `131:		t.Fatalf("goose_db_version rows = %d, want 2", count)`]`.
- **KD-27's boundary sentence and `Config`'s doc comment become false** the moment a second
  optional-with-default class exists — `[measured ac09e61:ai-docs/key-decisions.md:73 · `grep -o 'The boundary, stated exactly:[^.]*\.' ai-docs/key-decisions.md` → `The boundary, stated exactly:* the relaxation reaches **only** these keys.`; ac09e61:internal/config/config.go:38-39 · `sed -n '38,39p' internal/config/config.go` → `// returns an error naming every rejected key. Transport is the one` / `// exception: its fields are individually optional-with-default, so an`]`.
- **`jsonb` normalises, so a byte-exact payload round trip does not hold** — a test asserting raw
  bytes would pass by luck and fail on a reordered key —
  `[measured postgres:18.6 · `SELECT '{"b":1,"a":{"c":null},"b":2}'::jsonb` → `{"a": {"c": null}, "b": 2}`]`.
  Mitigated by comparing decoded values (§ Test Design, AC24).
- **`go test -race ./...` is a required gate and the worker is goroutine code by construction**;
  the handler goroutine, the watchdog goroutine, the observer callbacks and the shared test handler
  state are the surfaces — `[derived → every scheduler test package runs under the race gate `make verify` invokes, and the AC6 concurrency test asserts exactly-once under it]`.
- **A truncating gate hides later failures.** `go build ./...` prints at most ten errors per
  package and `golangci-lint run` caps issues per linter, so the site counts a subtask discovers
  are floors. Each subtask re-runs its gate after its own cleanup, and any newly-revealed
  out-of-contract class is surfaced to the orchestrator rather than absorbed —
  `[derived → each subtask's gate re-run, per the /task Step-8 loop]`.
- **`internal/testdb` must stay out of `cmd/bot`'s dependency graph** (KD-20)
  `[measured ac09e61:ai-docs/key-decisions.md:53 · `grep -n 'KD-20 ' ai-docs/key-decisions.md | cut -c1-88` → ``53:**KD-20 — Tests provision Postgres through testcontainers-go, and never skip.** `inte``]`,
  so the scheduler's fixture is imported only from `_test.go` files — `[derived → AC18's check on the resulting tree]`.
- **Subtask 11 edits a decisions document.** The authorisation covers one token on one line; the
  same line carries the invariant clause this design was reshaped to preserve, so an implementor
  editing "while they are there" would violate `AGENTS.md` § Project. Mitigated by D15 stating the
  bound where the edit is prescribed — `[derived → the AC22 check that no other sentence of `docs/DESIGN.md` differs from its pre-change text]`.

---

## Test Design

Every claim in this section is about a test that does not yet exist, so every tag is
`[derived → …]`. Every database test runs against a real PostgreSQL server through
`internal/testdb`, each in its own schema, with `TestMain` calling `testdb.Main` exactly as
`internal/store` does
[measured ac09e61:internal/store/store_test.go:15 · `grep -n 'os.Exit(testdb.Main(m))' internal/store/store_test.go` → `15:	os.Exit(testdb.Main(m))`]
(AC17) — no fake, no mock, and no skip when no database is available.

**Fixtures and helpers, in `internal/scheduler`:**

- `newScheduler(tb)` — a `testdb.Schema` pool with `store.Migrate` applied, closed on cleanup; the
  shape `internal/store`'s `newStore`
  [measured ac09e61:internal/store/store_test.go:20 · `grep -n '^func newStore' internal/store/store_test.go` → `20:func newStore(tb testing.TB) *pgxpool.Pool {`]
  already uses `[derived → subtask 5]`.
- `recordingObserver` — collects `Observation` and `LoopObservation` values behind a mutex, so
  every assertion on the seam is exact and the collector is race-clean `[derived → AC12]`.
- Test handler types and **their payload keys, named here because AC28 asks the design to name
  them**: `test.oneshot` reads `n` (integer — the value its handler writes into
  `manual_correction.reason`) and `nested` (object, present only in the round-trip case);
  `test.recurrent` reads no payload key. Handlers write through a real table
  (`manual_correction`) rather than a bespoke fixture table, so "its writes are absent" is asserted
  against the migrated schema `[derived → AC8, AC24, AC28]`.
- `dueNow(tb, pool, req)` — inserts a task through `Registry.Schedule` inside a committed
  transaction with zero delay `[derived → subtask 5]`.

**Pure functions — no database, no clock** (`cadence_test.go`, subtask 4):

- `backoff`: a table over the attempt index asserting **exact** durations for a given base and
  ceiling, that each is strictly greater than the previous until the ceiling, that the ceiling
  clamps, and that the result is strictly positive for the first attempt `[derived → AC9]`.
- `Every`: a table asserting the next instant is the smallest `prev + k·period` strictly after
  `now`; the catch-up case (a `now` many periods past `prev` yields **one** next instant, not a
  backlog); and the just-completed case `[derived → AC11, AC32]`.
- `NewRegistry`: refuses an empty type, a nil handler, a duplicate type, a recurrence with a nil
  cadence and a recurrence with an empty configuration key, each error naming the type
  `[derived → AC21 registration, D9]`.

**Insertion** (`schedule_test.go`, subtask 5):

- Round-trip: a payload with a nested object and a null value is scheduled, claimed and delivered
  to the handler; the assertion decodes both sides and compares values, never raw bytes
  `[derived → AC24]`.
- `Schedule` with an unregistered type returns `ErrUnknownType` and writes no row `[derived → AC21]`.
- A recurrence already live: a second `Schedule` of the same type and instance key returns
  `ErrDuplicateTask` wrapping SQLSTATE 23505 on `scheduled_task_identity_key`; two keyless
  one-shots of the same type both succeed and coexist `[derived → AC29]`.
- **The dead-identity direction**: with a row of that identity already in `state = 'dead'`,
  `Schedule` of the same type and instance key **succeeds**, and afterwards exactly one **pending**
  row and the untouched dead row exist for it. This is the assertion that the round-1 trap is
  closed; it is asserted here under AC29's "the constraint does not force a one-shot to invent an
  identity" half rather than under a new criterion, and § Open questions flags it as a candidate
  AC for the owner `[derived → AC29, D5]`.
- A negative delay returns `ErrInvalidDelay` and writes no row `[derived → D3]`.
- `DeadTasks` returns give-up rows only, ordered, each carrying type, `run_at`, attempt count and
  last error `[derived → AC10]`.

**The execution cycle** (`worker_test.go`, subtask 6):

- More due tasks than the limit: one `RunOnce` executes at most the limit, and with a second
  transaction holding some rows the discovery claim returns the unlocked ones rather than blocking
  `[derived → AC5]`.
- Two workers, many due tasks, run concurrently under `-race`: every task's handler ran exactly
  once, counted behind a mutex `[derived → AC6]`.
- A row discovered but taken by another transaction before its re-claim is skipped **silently**:
  no handler call, no observation, and the cycle continues to the next id `[derived → AC6, D2]`.
- A handler that writes and then returns an error: none of its writes are visible after the cycle,
  and the row records the failed attempt rather than a success `[derived → AC7]`.
- A handler that writes and then reports `OutcomeNoop`: its writes are absent, the row is settled,
  and the observation's outcome is neither success nor failure `[derived → AC8]`.
- Delete-on-done, both directions: a committed one-shot leaves no row; a cycle whose task
  transaction is rolled back leaves the row present and still due `[derived → AC25]`.
- A recurrence across a committed execution, a rolled-back execution and a failed execution: the
  row's `id` is unchanged and the live-row count for that recurrence is exactly one at every commit
  boundary `[derived → AC11]`.

**Failure policy** (`failure_test.go`, subtask 7):

- A one-shot whose handler always fails, driven attempt by attempt (each cycle preceded by making
  the row due, so the test waits for no backoff): the persisted `run_at` deltas grow, the **exact**
  attempt count at give-up equals the configured cap, the state is terminal, and a subsequent
  discovery does not return it `[derived → AC9]`.
- A handler that cannot decode its payload: classified as a failure, not a no-op; reaches give-up
  within the cap; enumerable through `DeadTasks` with the decode message as its recorded reason
  `[derived → AC26]`.
- A recurrence whose handler fails more times than the one-shot cap: still `pending`, still
  advancing by its cadence, never terminal `[derived → AC32]`.
- The counter rises across successive failures of one recurrence and returns to zero after a
  success, and after a no-op `[derived → AC34]`.
- **A row whose type is not declared, driven more times than the one-shot cap**: every cycle
  produces an observation carrying the unregistered-type failure kind; after all of them the row
  is still `state = 'pending'`, its `consecutive_failures` is still its starting value, and its
  `run_at` has advanced by the configured ceiling each time — so it keeps coming due and keeps
  being refused, which is the spec's stated property and the one round 1 silently broke
  `[derived → AC21]`.

**The seam and the loop** (`observe_test.go`, subtask 8):

- One observation per executed task, carrying type, lag, outcome, batch size and — on failure —
  the consecutive-failure count; collected for a success, a guard no-op, a one-shot retry, a
  one-shot give-up and a repeatedly failing recurrence, plus one loop observation carrying a
  duration `[derived → AC12]`.
- Lag is positive and is measured against `run_at`, asserted as a bound rather than an exact value
  (it is a real elapsed interval, not a virtual one) `[derived → AC12]`.
- The loop observation's `BatchSize` equals the discovery cardinality even when some of those ids
  were skipped at re-claim `[derived → AC12, D12]`.
- With no observer installed, the same cycles run and the package's tests pass `[derived → AC13]`.
- A worker constructed with non-default tuning values shows the changed behaviour: a smaller claim
  limit bounds the batch, a smaller attempt cap gives up sooner, a different backoff base changes
  the persisted delays, and a short poll interval makes a freshly inserted task run within a
  bounded wait `[derived → AC14]`.

**The deadline** (`deadline_test.go`, subtask 9):

- A handler that blocks past a short configured deadline: the worker returns, the observation is
  made with the deadline failure kind, and the row becomes claimable again within **≈ 2 × the
  configured deadline** — the bound D11 derives, not one deadline; a following cycle executes it
  `[derived → AC30]`.
- The neighbours survive: a cycle containing a task that breaches its deadline and a task that
  succeeds leaves the successful task's effects committed, because they were never in the same
  transaction `[derived → AC30, D2]`.
- The self-healing property that replaces a liveness protocol: a transaction that claims a row,
  writes, and is then rolled back leaves the row still due with none of its writes visible, and
  the next discovery returns it `[derived → AC31]`.

**Reconciliation** (`reconcile_test.go`, subtask 10):

- A declared recurrence with no row is seeded, one cadence ahead, not immediately `[derived → AC33]`.
- **A declared recurrence whose only row is `state = 'dead'`** is seeded: afterwards exactly one
  pending row exists for it, beside the untouched dead one. Unreachable for a recurrence under D7
  and D5 together, and asserted anyway, because it is the property that keeps "a stopped chain is
  healed by the next restart" true if any future path ever does mark one dead `[derived → AC33]`.
- **A declared recurrence whose cadence was SHORTENED** — the only direction the correction fires
  in (D10) — has its `run_at` corrected; a **lengthened** cadence leaves the row alone and is
  absorbed by one early occurrence; running `Reconcile` twice changes nothing the second time
  `[derived → AC33]`.
- Two `Reconcile` calls racing under `-race` produce exactly one row, and the loser's insert is the
  constraint's no-op rather than a lock wait `[derived → AC33, AC29]`.
- An imminent occurrence (inside a poll interval) and an in-flight occurrence (its row locked by an
  open transaction) are both left untouched `[derived → AC33]`.

**In `internal/store`** (subtasks 1 and 2):

- `store.Migrate` against an empty database yields `scheduled_task`, `deferred_task` and
  `recurrent_task`; the migration file has no down section `[derived → AC2]`.
- `journal_entry` carries one nullable FK column per basis type, the CHECK names every one of them,
  and each new basis column has its own partial unique index; the FK-coverage test stays green
  `[derived → AC3]`.
- A balanced batch through `store.Post` under each new basis type succeeds and its `journal_entry`
  row has exactly one non-null basis column; a row attempting two is refused by the database with
  the CHECK's name `[derived → AC4]`.
- A task posts under a new basis type, the task row is then deleted, and the `journal_entry` and
  `posting` rows survive and still balance `[derived → AC27]`.
- `scheduled_task`'s column set contains no execution marker, no heartbeat and no `completed` state
  value; the identity index's predicate names both `instance_key IS NOT NULL` and
  `state = 'pending'` `[derived → AC31, AC25, AC29]`.

**No golden fixture is specified by this task.** Nothing here is a pure simulation whose output is
snapshotted; the combat-golden rule applies to `combat()` and its callers, not to a worker whose
observable outputs are database rows and observation structs.

---

## Open questions

- **The dead-identity property is asserted as a design-level test, not as a criterion.** The
  round-2 review asked for an AC-level assertion that a seed against a dead row of the same
  identity leaves exactly one pending row. A new criterion is a spec amendment and routes through
  the owner, so the design asserts the property under AC29 and AC33 instead (§ Test Design). If the
  owner wants it binding at criterion level, it is a criterion-level addition and no code change.
- **The task type is `text`, not a PostgreSQL enum** (D6). The argument is made in full and the
  decision is the design's to make per the spec, but it is a visible departure from "every
  categorical column is a PostgreSQL enum" and is flagged here so the owner sees it rather than
  discovering it in the migration.
- **An undeclared row's refusal repeats at the backoff ceiling** (D7), which reuses
  `LAB_GAME_SCHEDULER_RETRY_MAX_DELAY` rather than adding a key for a state that should not occur.
  If undeclared rows ever become common enough that the refusal rate matters in either direction,
  that is the key to split — and the refusal counter is what would show it.
- **The poll interval is fixed: a full claim batch does not trigger an immediate re-poll.**
  Throughput is therefore bounded by claim-limit-per-poll-interval. The batch-size observation
  (AC12) is exactly the signal that would justify changing it, and the change is local to the loop.
  Recorded as a consequence, not reopened.
- **One transaction per task is a transaction per task.** The cycle's cost scales with the batch
  rather than being constant in it. Chosen deliberately (§ Approach); if throughput ever makes it
  matter, the lever is a shorter poll interval or a second worker, never a batch transaction —
  that shape is the one §11 rules out.
- **`consecutive_failures` doubles as the one-shot attempt count** (D5). The two are the same
  number for a one-shot by construction, and a second column would be lifetime history on a table
  the owner's answer stripped of history. Flagged because AC9/AC10 and AC34 name the two concepts
  separately.
