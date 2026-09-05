# Postgres task scheduler: `scheduled_task`, the `SKIP LOCKED` worker, one-shot and recurrent tasks

**Source:** issue #20
**Date:** 2026-09-05
**Tracked in:** #20

This project deliberately has no cron: stamina, backpack evaporation and monster respawn
are lazy ticks computed from timestamps at read time. One component is the stated
exception — a hand-written task scheduler on Postgres. Its table — `scheduled_task`,
singular, per *Source conflicts* below — carries `run_at`, a type, a payload and a
status; its worker claims due rows by a `SELECT … WHERE run_at <= now() … SKIP LOCKED
LIMIT N`; and a task executes **in one transaction with its own effects**, which is what
makes it exactly-once with no two-phase machinery
[source: 6c63c88:docs/DESIGN.md:309-310 · `sed -n '309,310p' docs/DESIGN.md`]. §11 writes
that claim's row lock as `FOR UPDATE`; **the owner settled it at round 6 as
`FOR NO KEY UPDATE`**, and §11's own line is corrected to match rather than left to
diverge — see the lock-mode row in *Key decisions* for the reason, which is not the one a
reader expects.

It is load-bearing rather than ancillary. A raid session advances through timer edges
that *are* scheduler tasks, so the scheduler's lag is visible to a player as gameplay —
"a wave in five minutes" arriving in seven
[source: 7039e36:docs/DESIGN.md:162 · `sed -n '162p' docs/DESIGN.md`;
7039e36:ai-docs/domain-invariants.md:46 · `sed -n '46p' ai-docs/domain-invariants.md`].
`docs/DESIGN.md` §14 item 10 lists it among the infrastructure the MVP ships from day
one, naming it "несущий для FSM"
[source: 7039e36:docs/DESIGN.md:460 · `sed -n '460p' docs/DESIGN.md`].

The second structural property is the one most likely to be "fixed" by mistake: **a
stale task is expected traffic, not an error.** A timer edge carries `expected_seq`;
when the session has moved on, the guard misses and the task dies at execution.
Cancellation is unnecessary, orphans are harmless, and that path is *tested*, not
repaired
[source: 7039e36:docs/DESIGN.md:154 · `sed -n '154p' docs/DESIGN.md`;
7039e36:ai-docs/domain-invariants.md:43 · `sed -n '43p' ai-docs/domain-invariants.md`].

Task types are the ledger's own basis-document types — this project's model, with no
mapping onto a foreign job framework
[source: 7039e36:docs/DESIGN.md:310 · `sed -n '310p' docs/DESIGN.md`;
7039e36:ai-docs/domain-invariants.md:45 · `sed -n '45p' ai-docs/domain-invariants.md`].
That is why this task touches the ledger schema at all: two new basis documents join the
exclusive arc that `internal/store` already implements.

## Scope

1. **A new package for the worker.** Name settled here (issue #20 leaves it open):
   `internal/scheduler`, giving `scheduler.Worker`, `scheduler.Handler` and
   `scheduler.Task` — no stutter, no `SchedulerWorker` (`AGENTS.md` § API Naming).
   Every method that reaches the database takes `ctx context.Context` first, and no type
   in the package stores a context. Interfaces are declared by the consumer, so the
   handler registry's element type lives here and not with the mechanics that will
   implement it.

2. **A forward migration** creating `scheduled_task` plus the **two
   basis-document tables** the exclusive arc needs — the deferred one-shot and the
   recurrent task — each with its own nullable FK column on `journal_entry` and the
   exactly-one-basis `CHECK (num_nonnulls(…) = 1)` extended to name them. That CHECK
   today names exactly two columns, `player_operation_id` and `manual_correction_id`,
   and each basis column carries its own partial unique index
   [source: 7039e36:internal/store/migrations/00001_ledger_core.sql:67-75 ·
   `sed -n '67,75p' internal/store/migrations/00001_ledger_core.sql`]. §11 calls the
   CHECK the greppable registry of every basis type and calls a new type "a migration of
   a column plus the CHECK — deliberately: a new kind of document passes an explicit
   migration and review"
   [source: 7039e36:docs/DESIGN.md:323 · `sed -n '323p' docs/DESIGN.md`]. Migrations are
   goose files under `internal/store/migrations`, applied through `store.Migrate`, and
   **forward-only — no `-- +goose Down` section exists in this package**
   [source: 7039e36:internal/store/migrate.go:16-24 ·
   `sed -n '16,24p' internal/store/migrate.go`].

   Two of the table's properties are settled by the owner's round-1 answers.
   **The payload is one `payload jsonb` column**: each handler decodes its own shape and
   a new task type costs no migration for its payload; a payload that will not decode is
   caught at execution rather than by the database, and a reference held inside it can
   dangle. **A completed task row is deleted by the transaction that completes it**: the
   table holds pending and dead tasks only, so there is no terminal `completed` status
   value, no `executed_at` column, and no retention machinery — surviving history lives
   in the basis documents and the postings, and lag is measured in-process at execution
   (Scope 8). **Delete-on-done governs one-shots only** (owner, round 2): a recurrent
   task is one live row whose `run_at` moves forward, so its row is never deleted while
   the schedule stands, and the table's identity constraint is what makes double-seeding
   the same recurrence unrepresentable (Scope 7).

3. **The two new basis types in Go, and they must live in `internal/store`.**
   `PostingBasis` is a **sealed** sum type: both its methods are unexported, and its doc
   comment states it is satisfied by "exactly the store package's two implementations"
   [source: 7039e36:internal/store/basis.go:11-25 ·
   `sed -n '11,25p' internal/store/basis.go`]. A basis type therefore *cannot* be
   declared in `internal/scheduler` — the compiler forbids it. The two new documents are
   `internal/store` types alongside `PlayerOperation` and `ManualCorrection`, and that
   doc comment's "exactly two" stops being true in this change (see AC20).

4. **The worker: claim, execute, settle.** A batch claim of due rows with
   `FOR NO KEY UPDATE … SKIP LOCKED` (owner, round 6 — *Key decisions*; the per-id
   re-claim takes the same mode) and a configurable limit; one transaction per task
   carrying the task's own effects; a handler registry keyed by task type. The transaction is the
   worker's and is handed to the handler, mirroring `store.Post`, which takes a
   caller-owned `pgx.Tx` and neither commits nor rolls back
   [source: 7039e36:internal/store/post.go:74 · `grep -n '^func Post' internal/store/post.go`].
   A handler that posts does so through `store.Post` inside that same transaction — the
   ledger AXIOM is not relaxed for scheduler effects.

5. **Guard semantics as a first-class outcome.** A handler may decide the task is stale
   and complete it as a **no-op**. That is normal operation, and it is a *third* outcome
   class alongside "executed with effects" and "failed" — distinguished in the
   instrumentation, never counted as an error (Scope 8).

6. **Failure policy — and it differs by task kind, which is the owner's round-3
   decision, not a simplification.**
   - **One-shot:** a handler error re-schedules the task with a strictly positive,
     growing delay up to a configured attempt cap, after which the task reaches a
     terminal give-up state and is never claimed again. Tasks in that state are
     enumerable through the package with enough context to diagnose them (type,
     `run_at`, attempts, last error).
   - **Recurrent: the chain never stops.** A handler error re-schedules the row to its
     **next cadence instant** — the cadence *is* the retry, so the backoff schedule, the
     attempt cap and the give-up state do not apply to this kind at all. A permanently
     broken recurrent handler therefore fails at its cadence forever, which is the
     deliberate trade: the day close can never silently stop. Its evidence is a
     **persisted consecutive-failure count** on the row, which the answer itself names as
     the visibility mechanism, surfaced through the observation seam (Scope 8) so #23 can
     alert on it.

7. **Recurrent tasks: one live row per recurrence, moved forward** (owner, round 2).
   Completing an occurrence sets the row's next `run_at` **in place**, in the same
   transaction as the effects: a commit leaves exactly one live occurrence, a rollback
   leaves the current one still due, and there is no window in which a recurrence has
   zero or two live rows. The property the owner named as the reason for this shape is
   structural rather than procedural — **double-seeding one recurrence is unrepresentable
   because the table's identity constraint refuses it**, not because some code path
   remembers to check first. Which columns carry that identity, and the fact that a
   one-shot must not be forced to invent one, are in *Key decisions*.

   **The cadence that computes the next `run_at` is declared in Go, in the handler
   registry** (owner, round 3), with the cadence *value* still coming from configuration
   — a declaration names which key it reads, it does not embed a number
   (`AGENTS.md` § Code Style). Two start-up obligations follow, and together they are what
   makes "a stopped chain is healed by the next restart" a property rather than a hope:
   **seed** any declared recurrence that has no row, and **correct** the `run_at` of one
   that exists when the declaration has changed. The seed is idempotent because the
   round-2 identity constraint refuses the second insert — so several workers starting at
   once is safe by construction, not by a lock. The correction must not disturb an
   occurrence that is in flight or imminent. Changing a cadence is a deploy, which sits
   correctly with KD-24's read-configuration-once-at-start-up rule.

8. **Per-type lag instrumentation, through a seam rather than a registry.** The worker
   reports, per executed task, at least: task type, the lag between `run_at` and the
   execution instant, the outcome class of Scope 5, the size of the claim batch the task
   came from, and — for a failure — the row's **consecutive-failure count**; plus the
   worker loop's own duration. The consecutive count is the one field not named in issue
   #20's telemetry list, and it is here because the round-3 answer put the whole weight
   of "a permanently broken recurrent handler is noticed" on the failure counter: a
   per-type failure *rate* cannot distinguish one recurrence failing every time from many
   recurrences failing occasionally, and only the first is an outage. The shape follows the precedent
   this repository already set for the transport — a plain struct plus a consumer-declared
   one-method interface, with the implementation supplied elsewhere
   [source: 7039e36:internal/tg/observe.go:5-33 · `cat internal/tg/observe.go`].
   `github.com/prometheus/client_golang` is not reachable from this module today
   [source: 7039e36:go.mod · `go mod why -m github.com/prometheus/client_golang`], and
   `/metrics` plus the registry belong to #23 (open, verified
   `gh issue view 23 --json state`), which already names scheduler lag in its title.

9. **Operational tuning as configuration keys.** The poll interval, the claim batch
   limit, the retry attempt cap, the backoff base and ceiling, and the per-task execution
   deadline that bounds a hung handler (*Key decisions*) are `LAB_GAME_`-prefixed
   environment variables read by `internal/config`, in the optional-with-default class
   KD-27 established for operational tuning: absent means the documented default, present
   means parsed and validated with a start-up `*KeyError` naming the variable. They are
   appended through `EnvKeys()` — not through the unexported `envKeys()`, which the
   required-variable suites iterate
   [source: 7039e36:internal/config/env.go:33-55 · `sed -n '33,55p' internal/config/env.go`]
   — and each gets a line in `.env.example` carrying its default as a non-empty value.
   These are runtime settings, not game constants: no balance number gains a fallback and
   KD-23/KD-24 are untouched.

10. **Tests against a real PostgreSQL server, never a mock.** Through `internal/testdb`
    (a `postgres:18` container, or `LAB_GAME_TEST_DSN`, one schema per test), including
    a concurrency test under `-race` proving two workers never execute the same task
    [source: 7039e36:internal/testdb/testdb.go:1-30 · `sed -n '1,30p' internal/testdb/testdb.go`; 7039e36:internal/testdb/testdb.go:112 · `grep -n '^func Schema' internal/testdb/testdb.go`;
    7039e36:ai-docs/key-decisions.md:53 · `grep -n 'KD-20' ai-docs/key-decisions.md`].
    The claim path, the guard no-op path, the retry/give-up path and the recurrent
    successor path are database behaviour and are asserted against the database.

11. **Propagation, as a class — and it now reaches the source document.** This change
    renames a table that live documents name, **changes the row-lock mode those same
    documents quote**, adds an environment-variable class, and falsifies the "exactly two
    implementations" sentence in `internal/store/basis.go`. Every site whose claim the
    diff falsifies is updated in the same PR, membership decided by `AGENTS.md`
    § Propagation Rule step 4 — the sites named in AC22 illustrate the class and do not
    bound it. Two of the known sites now carry *two* falsified claims each rather than
    one, because they state the table name and the lock mode in the same sentence
    [source: 6c63c88:ai-docs/context.md:36 · `sed -n '36p' ai-docs/context.md`;
    6c63c88:ai-docs/key-decisions.md:15 · `sed -n '15p' ai-docs/key-decisions.md`].

    **`docs/DESIGN.md:310` is inside that class, and the licence for it is explicit.**
    `AGENTS.md` § Project holds the design document to "**Implement from it. Never
    redesign it** without an explicit user request"
    [source: 6c63c88:AGENTS.md:15 · `grep -n 'Never redesign it' AGENTS.md`]. The product
    owner supplied exactly that request twice, for two edits **to the same line**, and the
    bound is **two edits and no more**:

    - **Edit 1 (round 5) — the table's spelling.** `scheduled_tasks` becomes
      `scheduled_task`, so that §11's prose obeys the singular-table-names decision
      recorded three lines below it in the same section
      [source: 6c63c88:docs/DESIGN.md:313 · `sed -n '313p' docs/DESIGN.md`].
    - **Edit 2 (round 6) — the claim query's row-lock mode.** `FOR UPDATE SKIP LOCKED`
      becomes `FOR NO KEY UPDATE SKIP LOCKED` in §11's query sketch, so that §11 stays
      true of the worker this task ships and the implementation has no divergence from the
      canonical primitive to argue about.

    **Not authorised, and named so the bound is checkable.** The third clause on line 310
    — *"Таска исполняется **в одной транзакции со своими эффектами**"* — is untouched, and
    needs no change: the delivered per-task-transaction shape keeps it literally true.
    Neither is restructuring §11, any other sentence of `docs/DESIGN.md`, or any change to
    the scheduler's designed behaviour. **In particular the lock-mode edit is one line, not
    a document-wide substitution:** `docs/DESIGN.md` contains exactly two `FOR UPDATE`
    occurrences, and the other one — line 147, §3.5's `SELECT session FOR UPDATE` on the
    raid session — is outside this authorisation and outside this task
    [source: 6c63c88:docs/DESIGN.md:147 · `grep -n 'FOR UPDATE' docs/DESIGN.md`]. The
    licence is stated here, where the edits are prescribed, so that a later reader or a
    self-review meets it rather than an apparent AXIOM violation.

## Out of scope

- **The priority / queue column.** Designed and deferred until fan-out actually delays
  gameplay edges; the fix is a column with separate worker pools, never a second table
  [source: 7039e36:docs/DESIGN.md:311 · `sed -n '311p' docs/DESIGN.md`]. The obligation
  this task carries is negative and is AC16: adding it later must be an *additive*
  migration, not a rewrite.
- **The task handlers themselves.** Each ships with its mechanic — raid timer edges
  (#36), the chat notification queue (#43), corpse evaporation (#40), day close (#45),
  all open. This task ships the registry and at least one test-only handler; it ships no
  production handler.
- **The posting signature of each new basis type.** §13.4 requires a mechanic that moves
  balances to declare its basis document's posting signature and a contract test for it
  [source: 7039e36:docs/DESIGN.md:430 · `grep -n 'проводочная сигнатура' docs/DESIGN.md`].
  The scheduler moves no balance of its own; each mechanic declares the signature of the
  effects it posts under a deferred one-shot or a recurrent task when it ships its
  handler.
- **Game events in the event dictionary.** §13.4's obligation is on mechanics; issue
  #20's own telemetry obligation names metrics only, and the `events` table has no
  migration yet.
- **Wiring the worker into `cmd/bot`.** `cmd/bot` today only loads and validates
  configuration
  [source: 7039e36:ai-docs/context.md:43 · `sed -n '43p' ai-docs/context.md`]; the
  composition root, its start-up migration policy and graceful shutdown are #24 (open).
  This task ships a package a composition root can construct, exactly as the transport
  task did.
- **`/metrics`, the Prometheus registry and the health dashboard** — #23.
- **Editing `docs/DESIGN.md` beyond the two authorised edits on line 310.** It is
  decisions, and a task does not redesign it (`AGENTS.md` § Project). The only exceptions
  this task carries are the table's spelling and the claim query's row-lock mode, both on
  line 310, for each of which the owner gave the explicit request that AXIOM requires —
  see Scope 11 and *Source conflicts*. The rest of line 310 — the
  one-transaction-with-its-effects clause included — every other sentence of the document,
  the rest of §11, and §3.5's own `FOR UPDATE` on line 147 are all untouched, and no
  scheduler behaviour described there is revisited.
- **Retention of the ledger's own tables** (postings, daily balances) — §11 puts that
  outside MVP and #45 owns day close
  [source: 7039e36:docs/DESIGN.md:330 · `sed -n '330p' docs/DESIGN.md`].

## Deferred

- The priority / queue column and separate worker pools | the early signal §11 names —
  fan-out delaying gameplay edges — cannot be observed before there is fan-out, and the
  per-type lag metric this task ships is what will show it | no — #20's own *Out of
  scope* records it and the lag metric is the trigger
- A second worker instance in production | `SKIP LOCKED` is horizontally scalable with
  no coordination, so this is a deployment decision, not code
  [source: 7039e36:docs/DESIGN.md:311 · `sed -n '311p' docs/DESIGN.md`] | no
- Migrating to River | the ready-made exit if the scheduler grows features; `gocron` is
  refused as in-memory (one-shot tasks must survive a restart) and Temporal as overkill
  [source: 7039e36:docs/DESIGN.md:312 · `sed -n '312p' docs/DESIGN.md`;
  7039e36:ai-docs/key-decisions.md:15 · `grep -n 'KD-4' ai-docs/key-decisions.md`].
  `github.com/riverqueue/river` is not reachable from this module today
  [source: 7039e36:go.mod · `go mod why -m github.com/riverqueue/river`] | no — the
  escape hatch is already recorded in KD-4
- `LISTEN`/`NOTIFY` to cut the poll interval out of the lag budget | §11 pins the worker
  to the polling query shape, and the lag metric is the evidence that would justify
  changing it | yes, if measured lag makes the poll interval the dominant term
- Distinguishing a handler *panic* from a returned error in the failure policy | the
  project forbids panics in production code and indexes every surviving instance
  (`AGENTS.md` § Go Test Conventions), so a handler panic is a defect, not a state the
  scheduler models | no
- An admin surface for re-queuing a dead task | AC10 makes the dead ones enumerable,
  which is what #20 asks for; a command to act on them needs a bot-side operator surface
  that does not exist | yes, once there is an operator surface

## Key decisions

| Question | Decision |
|---|---|
| Package name (#20 leaves it open) | **`internal/scheduler`**, exposing `scheduler.Worker` / `scheduler.Handler` / `scheduler.Task`. No stutter (`AGENTS.md` § API Naming). |
| Where the migration lives | **`internal/store/migrations`**, as the second goose file, applied by `store.Migrate`. There is one migration set in this module and one `embed.FS` behind it [source: 7039e36:internal/store/migrate.go:16-24 · `sed -n '16,24p' internal/store/migrate.go`]. |
| Where the two basis types live | **`internal/store`, not `internal/scheduler` — forced, not chosen.** `PostingBasis`'s two methods are unexported, so no other package can satisfy it [source: 7039e36:internal/store/basis.go:11-25 · `sed -n '11,25p' internal/store/basis.go`]. Consequence: the scheduler package depends on `store` for its basis types, never the reverse. |
| Who owns the task's transaction | **The worker opens it; the handler receives it.** Same contract as `store.Post`, which neither commits nor rolls back its caller's `pgx.Tx` [source: 7039e36:internal/store/post.go:74 · `grep -n '^func Post' internal/store/post.go`]. A task and its effects therefore commit or roll back as one, which is the whole exactly-once argument of §11. |
| The claim's row-lock mode | **`FOR NO KEY UPDATE … SKIP LOCKED`, for the batch claim and the per-id re-claim alike** (owner, round 6). **The reason is not performance, and writing that down is half the decision.** It is the weakest lock that expresses what a claim actually does: a claim reads a row and marks it taken; it does not change the row's key. The second half of the owner's reasoning is the operative one and it measures out — a one-shot's `DELETE` acquires `FOR UPDATE` strength anyway, **at the moment it is needed** rather than for the whole handler's duration, and claim-then-`DELETE` in one transaction succeeds [measured postgres:18.6 · `BEGIN; SELECT id FROM task WHERE id=1 AND state='pending' AND run_at <= now() FOR NO KEY UPDATE SKIP LOCKED; DELETE FROM task WHERE id=1; COMMIT` → `1` then `DELETE 1` then `COMMIT`, row absent afterwards]. The exclusion protocol the whole design rests on is unchanged and was checked rather than assumed: a row held under `FOR NO KEY UPDATE` is skipped by another worker exactly as before [measured postgres:18.6 · holder open on `id=1`; a second session's `SELECT id FROM task WHERE run_at <= now() ORDER BY run_at, id FOR NO KEY UPDATE SKIP LOCKED LIMIT 5` → `2` (the free row, not the held one), and its per-id re-claim of `id=1` → `(0 rows)`]. **Explicitly not a throughput decision:** the round-6 investigation reported no measurable separation between the modes on this task's own claim pattern — a claim of issue-body standing, not re-run here, and it is recorded to *remove* a justification rather than to supply one. Nobody may later trade away the FK condition of the row below on the belief that the weaker lock is buying speed; if a future change wants a performance argument for either mode, it measures one. |
| What the lock-mode equivalence rests on, and how far the FK ban actually reaches | **The equivalence rests on a condition the spec only partially enforces, and that gap is the point of this row.** The condition is: *no table anywhere references `scheduled_task` by a foreign key.* While it holds, nothing concurrently takes `FOR KEY SHARE` on a task row and the mode is free. **What is enforced is narrower than the condition:** AC27 forbids such an FK only from a **basis-document** table (owner, round 7 — the criterion was deliberately kept at that scope). An FK from any *other* table is prevented by nothing but this note. A future reader meets a residual risk here, not a guarantee. Add such an FK from anywhere and the weaker mode starts **minting multixacts** under ordinary concurrency, where `FOR UPDATE` would have blocked the referential-integrity check instead. *The multixact minting is measured below; the costs that follow from it — a `MultiXact/CREATE_ID` WAL record, SLRU retention and multixact-freeze burden per co-lock — are the round-6 investigation's claim, of the same standing as an issue body, and are not pinned here as fact. They are recorded because they are the shape of the risk, not because they were checked.* Measured both ways on a real FK: under `FOR NO KEY UPDATE` the child insert succeeds and the parent tuple's `xmax` becomes a MultiXactId [measured postgres:18.6 · holder `SELECT id FROM parent WHERE id=1 FOR NO KEY UPDATE` open; concurrent `INSERT INTO child VALUES (11,1)` → `INSERT 0 1`; `heap_page_items(get_raw_page('parent',0))` → `t_infomask=0x11c2`, `HEAP_XMAX_IS_MULTI` set]; under `FOR UPDATE` the same insert blocks until it is cancelled, and PostgreSQL names the culprit in its own error [measured postgres:18.6 · holder on `id=2` `FOR UPDATE`; concurrent `INSERT INTO child VALUES (12,2)` under `statement_timeout='4s'` → `ERROR: canceling statement due to statement timeout / CONTEXT: while locking tuple (0,2) in relation "parent" / SQL statement "SELECT 1 FROM ONLY "public"."parent" x WHERE "id" OPERATOR(pg_catalog.=) $1 FOR KEY SHARE OF x"`]. That `FOR KEY SHARE OF x` is the referential-integrity trigger's own query, which is why the FK is the whole hazard. AC27 and the lock-mode row therefore point at each other: **neither may be revisited alone** — and because AC27 covers only part of the condition, a change that adds a foreign key to `scheduled_task` from a non-basis table would pass every criterion in this spec while silently invalidating the equivalence. That is the known, accepted gap; closing it would take a criterion the owner declined at round 7. |
| Whether a guard no-op is a failure | **No — it is a third outcome class.** `docs/DESIGN.md` §3.5 and `ai-docs/domain-invariants.md` § 4 both state a stale task dying at execution is expected traffic; the instrumentation must be able to tell it from a failure, or the health dashboard will read normal operation as an incident [source: 7039e36:ai-docs/domain-invariants.md:43 · `sed -n '43p' ai-docs/domain-invariants.md`]. |
| How a task type relates to a basis document | **A task type names the basis document its effects post under, and the two new tables cover only the tasks that have no other basis.** §11's starter registry lists the raid-session transition and the day-close cron as basis types of their own [source: 7039e36:docs/DESIGN.md:331 · `sed -n '331p' docs/DESIGN.md`], and §3.5 makes a raid transition its own document [source: 7039e36:docs/DESIGN.md:153 · `sed -n '153p' docs/DESIGN.md`]. So a raid timer edge posts under #36's transition document; corpse evaporation posts under a deferred one-shot; the day close posts under a recurrent task. Issue #20 scopes this task to the latter two tables, and that is what it ships. |
| Whether every executed task writes a basis row | **No.** A basis document exists to anchor postings; a task whose effects move no balance (a notification send) writes none. The basis row is created inside the executing transaction, by the code that posts. |
| The task-type value set | **Left to the design, with the project's precedent as the default:** every existing categorical column is a PostgreSQL enum (`owner_kind`, `ledger_kind`, `operation_source`) [source: 7039e36:internal/store/migrations/00001_ledger_core.sql:2-4 · `sed -n '2,4p' internal/store/migrations/00001_ledger_core.sql`], and a persisted enum value is a data contract that changes by forward migration only (`AGENTS.md` § API Stability carve-out). Whichever representation the design picks, AC21 binds: an unregistered type must be a refusal, never a row that stays due forever. |
| Where the operational tuning values live | **Environment keys with compiled-in defaults**, in KD-27's optional-with-default class, read by `internal/config` and listed in `.env.example`. KD-27's boundary is respected exactly: it names what stays required — secrets, base URL, chat allowlist, the two file paths — and forbids a fallback for any balance number; a poll interval is none of those. |
| Metric coupling | **No metrics-registry import in `internal/scheduler`.** The package defines the observation point; #23 supplies the implementation, exactly as `internal/tg` did [source: 7039e36:internal/tg/observe.go:27-33 · `sed -n '27,33p' internal/tg/observe.go`]. |
| Whether this task wires the worker into `cmd/bot` | **No — #24.** The transport task set the precedent: ship the package, let the composition root construct it [source: 7039e36:ai-docs/context.md:43 · `sed -n '43p' ai-docs/context.md`]. |
| The table's name | **`scheduled_task`, singular**, matching issue #20's title and Scope and the 2026-09-02 table-naming decision that every table in migration 00001 already follows. §11's prose spells it plural in one place; see *Source conflicts*. |
| Whether this task may edit §11 in `docs/DESIGN.md`, and how far | **Yes for two edits, both on line 310 — the owner gave the explicit request `AGENTS.md` § Project requires, at round 5 for the first and round 6 for the second** — and the bounds are themselves the decision. **Authorised, exactly two:** (1) line 310's `scheduled_tasks` becomes `scheduled_task`, closing §11's disagreement with the singular-names decision three lines below it; (2) line 310's `FOR UPDATE SKIP LOCKED` becomes `FOR NO KEY UPDATE SKIP LOCKED`, so §11 stays true of the shipped worker [source: 6c63c88:docs/DESIGN.md:310,313 · `sed -n '310p;313p' docs/DESIGN.md`]. **Not authorised:** the third clause on that same line (*"Таска исполняется в одной транзакции со своими эффектами"*, which needs no change and stays literally true), redesigning or restructuring §11, editing any other sentence of `docs/DESIGN.md` — §3.5's `SELECT session FOR UPDATE` on line 147 explicitly included — or altering any scheduler behaviour §11 describes. Neither edit revisits a decision: the first applies a decision §11 already made on line 313, the second records one the owner made at round 6. |
| Payload representation | **One `payload jsonb` column** (owner, round 1). Each handler decodes its own shape; a new task type costs no migration for its payload. The two costs are accepted explicitly in the same answer: a malformed payload is caught at execution rather than by the database, and a reference held inside the payload can dangle. Consistent with the repository's existing split — the ledger is strict schema, the event log is JSONB, and they are deliberately different tables [source: 00a58ac:ai-docs/domain-invariants.md:52 · `sed -n '52p' ai-docs/domain-invariants.md`]. |
| Whether JSONB relaxes the data-contract rule for payload keys | **No, and this is the trap the choice creates.** `AGENTS.md` names the scheduler's payloads, by that word, among the live data that outlives every deploy and changes by forward migration only [source: 00a58ac:AGENTS.md:84 · `grep -n 'scheduler.s .scheduled_tasks. payloads' AGENTS.md`]. A payload key is a persisted name: renaming one, re-typing one, or repurposing one is the same defect as doing it to a column, and a pending row written by the previous deploy must still decode. "It is only JSON" is not a licence. |
| What a payload the handler cannot decode means | **A failure, not a guard no-op** — and one that must not loop. It is not the "state moved on" case §3.5 describes; it is a defect that will reproduce identically on every attempt, so it must reach the terminal give-up state and become visible (AC26) rather than consume the retry budget forever. |
| What a dangling reference inside a payload means | **A guard miss — the default, and a handler may classify otherwise.** §3.5 makes orphaned tasks normal by design: cancellation is unnecessary and a task whose subject has moved on dies at execution [source: 7039e36:docs/DESIGN.md:154 · `sed -n '154p' docs/DESIGN.md`]. A referent that no longer exists is that same case. What the spec fixes is that both classifications are representable and observable; which one a given handler picks travels with the mechanic. |
| Lifecycle of a completed row | **Deleted by the transaction that completes it** (owner, round 1). The table holds pending and dead tasks only. Consequences, all of them intended: no retention step and no growth curve to manage; no terminal `completed` status value and no `executed_at`; the observation seam is the **only** place execution lag ever exists, since there is no row left to compute it from (AC12 carries the whole weight); and a guard no-op leaves no per-occurrence trace beyond its counter — see *Open questions*. |
| Retention | **None, and none is needed** — it follows from the row above rather than being a separate decision. §11 places retention outside MVP in any case [source: 7039e36:docs/DESIGN.md:330 · `sed -n '330p' docs/DESIGN.md`]. |
| Which direction the basis-document / task reference points | **Never from a basis document to `scheduled_task`.** Delete-on-done means the task row is gone while its basis document and postings live on, so a hard FK that way would either block the delete or cascade away ledger history. §11 fixes the direction for exactly this reason: FKs point from postings to bases, which is what makes dropping postings first always safe [source: 3de3dfe:docs/DESIGN.md:344 · `grep -n 'FK направлен от проводок' docs/DESIGN.md`]. A basis document carries by value whatever it needs to identify the task that produced it; which fields those are is the design's. |
| The recurrent row's lifecycle | **One live row per recurrence, moved forward in place** (owner, round 2). Completing an occurrence sets the next `run_at` on the same row inside the same transaction as the effects; the row is never deleted while the schedule stands. Delete-on-done therefore governs **one-shots only**, and the two kinds differ in the schema by how they settle, not by living in different tables — which keeps §11's "class separation by a column, never a second table" intact [source: 7039e36:docs/DESIGN.md:311 · `sed -n '311p' docs/DESIGN.md`]. |
| What makes double-seeding one recurrence impossible | **A uniqueness constraint on the task's identity, not a check in code** — the owner's stated reason for the move-forward shape. The identity is the task type plus an instance key. **A one-shot must not be forced to invent one:** a corpse-evaporation task has a natural instance key and a fan-out notification may not, so the constraint is a *partial* unique index over rows that carry an instance key, with a surrogate primary key — exactly the shape migration 00001 already uses for the basis columns of `journal_entry` [source: 7039e36:internal/store/migrations/00001_ledger_core.sql:74-75 · `sed -n '74,75p' internal/store/migrations/00001_ledger_core.sql`]. A composite primary key over (type, instance) would force every one-shot to supply a key it may not have. The design fixes the column names; AC29 fixes the property. |
| Whether the scheduler needs a `picked` flag, a heartbeat and a dead-execution detector | **No — and the reason is a decision `docs/DESIGN.md` already made, not a preference.** A liveness protocol exists to release rows stranded by a worker that died *while holding them in a non-transactional executing state*. §11 specifies the opposite: the task executes **in one transaction with its own effects** [source: 7039e36:docs/DESIGN.md:310 · `sed -n '310p' docs/DESIGN.md`]. A worker that dies mid-execution aborts its transaction, which releases its row locks and reverts its writes, leaving the row exactly as it was — still due, and claimable by the next worker on its next poll. There is no stranded state for a detector to find, so a heartbeat column, a `picked` column and a revival handler would all be machinery guarding an unreachable state. |
| The one failure the transactional shape does **not** self-heal | **A handler that hangs rather than crashes** — its transaction stays open, its row stays locked, and `SKIP LOCKED` means every other worker passes over that row *silently and forever*. This is the residual risk the row above creates, and it is answered by a **bounded per-task execution deadline** (Scope 9), not by a liveness protocol: a bound turns an invisible stall into a timed failure that enters the ordinary retry path and the ordinary instrumentation. Whether the bound is enforced through the handler's context, a database-side session timeout, or both is the design's; that a bound exists and that exceeding it is observable is AC30. |
| Where a recurrent task's cadence is declared | **In Go, in the handler registry** (owner, round 3) — the declaration names the configuration key its value comes from rather than embedding a number, so `AGENTS.md` § Code Style and KD-24 both stay intact and changing a cadence remains a deploy. |
| What re-seeds a recurrence whose row is missing | **Start-up, from the declaration set, in two moves:** seed any declared recurrence that has no row, and correct the `run_at` of one that exists when the declaration has changed. The seed needs no lock and no existence check — the round-2 identity constraint refuses the second insert, so concurrent start-up of several workers is safe by construction. The correction must leave an in-flight or imminent occurrence alone; a start-up that yanks a task about to run is a worse failure than a cadence that takes one cycle to take effect. |
| Whether a recurrent chain can end in the terminal give-up state | **No — it never stops** (owner, round 3). This **retracts the round-3 forecast recorded here**: that row previously said the question survived because start-up seeding recreates a *missing* row while a *dead* row would merely be adopted. Under this answer there is no dead recurrent row for the seeding rule to meet, so the revive branch that reasoning implied is **not** built, and the seeding rule stays the two moves above. |
| What the give-up state and the retry budget now govern | **One-shots, and only one-shots.** The owner's answer reschedules a failed recurrence to its *next cadence instant*, which means the backoff schedule, the attempt cap and the terminal give-up state have no effect on the recurring kind: for it, the cadence **is** the retry. Stating this as a scope boundary rather than leaving it implied is deliberate — a design that applied the attempt cap to both kinds would silently reintroduce the terminal state the owner just removed. |
| How a permanently broken recurrence is made visible | **A persisted consecutive-failure count on the row, carried in the observation.** The owner's answer names "the failure counter" as the visibility mechanism, and under move-forward the row is the only place a count can survive across occurrences. It is carried into the observation seam because a per-type failure *rate* cannot separate "one recurrence failing every single time" from "many recurrences failing now and then", and only the first is an outage. The count resets on success. Alert rules on it belong to #23. |
| Which recurrent types this task declares | **None in production, one in tests.** Cadence declarations arrive with their mechanics — the day close is #45 — so this task ships the declaration mechanism, the seeding and correction path, and a test-only recurrent type that exercises them. The first real declaration and its configuration key land with the mechanic that needs them, matching the handler boundary issue #20 already drew. |
| How a recurrence is retired | **By an explicit removal, never by an omission** — and the declaration set is now the authority for what *should* exist. Removing a type from the registry does not stop its row coming due; it makes the row unclaimable, which AC21 requires to be a visible refusal. Retiring a recurrence deletes the row in the same change that drops its declaration. No automatic collector of undeclared rows is built — see *Open questions*. |

## Technical constraints

- **Provenance of the `db-scheduler` prior art — read this before treating any of it as
  fact.** The owner's round-1 answer to the recurrence question was a research request,
  not a choice, and the investigation was run by the interview orchestrator because this
  spec's author has **no web access**. Everything it returned about
  `kagkarlsson/db-scheduler` — its shipped PostgreSQL DDL, its `Schedule` abstraction,
  its start-up seeding, its dead-execution heartbeat, its static-versus-dynamic recurring
  kinds — is an **unverified claim of the same standing as an issue body**, and none of
  it is pinned anywhere in this spec as a fact about anything. It is a **Java library
  this project does not use and will not depend on**; it appears here only because the
  owner asked what it does before deciding, and because it named two forks the round-1
  options had collapsed into one. Nothing in it is a constraint, a default, or an
  instruction to imitate. If the design ever wants to rely on one of those claims, it
  reads the library's own source and pins it there. The round-3 block extends the same
  research to that library's three healing mechanisms and carries the same standing.
  **One observation inside it is load-bearing and was therefore checked against this
  repository rather than accepted:** the library needs a heartbeat because it runs a
  handler outside the claiming transaction, and §11 specifies the opposite for this
  project — see the `picked`-flag row in *Key decisions*, whose conclusion rests on §11's
  own sentence and not on the research.
- **Module and toolchain:** `github.com/maratik123/lab-game`, `go 1.26`
  [source: 7039e36:go.mod:1-3 · `sed -n '1,3p' go.mod`].
- **The ledger schema this migration extends.** `journal_entry` has one nullable FK
  column per basis type, a `CHECK (num_nonnulls(…) = 1)` naming all of them, and one
  partial unique index per basis column; `posting` references `journal_entry`
  [source: 7039e36:internal/store/migrations/00001_ledger_core.sql:67-84 ·
  `sed -n '67,84p' internal/store/migrations/00001_ledger_core.sql`]. The new columns
  follow that shape rather than inventing one.
- **Migrations are forward-only and there is no down path.** A shipped column is a data
  contract; the way to change it is another migration
  [source: 7039e36:internal/store/migrate.go:16-24 · `sed -n '16,24p' internal/store/migrate.go`]. This is the reason the payload and
  lifecycle questions are asked at spec time rather than left to the design: both are
  persisted shape, and `AGENTS.md` § API Stability's carve-out names the scheduler's
  payloads explicitly as live data that outlives every deploy.
- **`store.Post`'s error surface is already rich and is not to be duplicated.** It
  returns typed sentinels including `ErrAlreadyPosted` and `ErrOverdraft`
  [source: 7039e36:internal/store/post.go:56-72 · `sed -n '56,72p' internal/store/post.go`].
  A handler failure and a posting failure both roll the task's transaction back; the
  scheduler classifies the outcome, it does not re-implement the ledger's diagnostics.
- **Tests may not skip when no database is available.** `testdb.Main` returns a non-zero
  exit code rather than reporting a false pass
  [source: 7039e36:internal/testdb/testdb.go:1-9 · `sed -n '1,9p' internal/testdb/testdb.go`].
- **`internal/testdb` must stay out of `cmd/bot`'s dependency graph** (KD-20): it is
  non-test Go imported only from `_test.go` files, so the container runtime never links
  into the binary [source: 7039e36:ai-docs/key-decisions.md:53 · `grep -n 'KD-20' ai-docs/key-decisions.md`].
- **Concurrency is a required race gate.** `go test -race ./...` is required for any
  change touching goroutines, the scheduler, or shared state, and a race is a defect,
  never a flake (`AGENTS.md` § Go Test Conventions). The worker loop is goroutine code by
  construction.
- **Tests get virtual time without a production clock abstraction** (KD-26): production
  code calls `time.Now` / `time.NewTimer` directly and tests run inside a
  `testing/synctest` bubble. Backoff growth, the poll interval and a retry schedule are
  therefore assertable exactly, without sleeping
  [source: 7039e36:ai-docs/key-decisions.md:71 · `grep -n 'KD-26' ai-docs/key-decisions.md`].
- **Balance values stay out of Go source** (`AGENTS.md` § Code Style). None of this
  task's tuning values is a balance number, but the discipline holds for the ones that
  are: a task's *game-meaningful* delay — a corpse TTL, an escalation timer — is set by
  the mechanic that schedules it, from the balance file, never by the scheduler.
- **No new module is required by this task.** `go mod why -m` reports none of
  `github.com/riverqueue/river`, `github.com/robfig/cron/v3` and
  `github.com/prometheus/client_golang` reachable from this module today
  [source: 7039e36:go.mod · `go mod why -m <each>`]. If the design concludes it needs
  one — a cron-expression parser is the plausible case, and only under one answer to the
  recurrence question — `AGENTS.md` § Dependency Versions governs: the established
  package is the default and hand-rolling is what must be argued, with KD-4 as the model
  for an argued wheel.

## Source conflicts

`docs/DESIGN.md` §11 disagrees with itself about the scheduler table's name. **This
section is about that conflict only.** The other edit line 310 receives — the row-lock
mode — is not a source conflict: §11 states `FOR UPDATE` and nothing in the corpus
contradicts it. It is a decision the owner changed at round 6, recorded in *Key
decisions* and authorised in Scope 11. A reader of this section must not infer that line
310 receives exactly one edit; it receives two.

- `docs/DESIGN.md:310` — *"таблица `scheduled_tasks` (run_at, тип, payload, статус)"*
  [source: e64bcc6:docs/DESIGN.md:310 · `sed -n '310p' docs/DESIGN.md`]
- `docs/DESIGN.md:313` — *"(таблица `posting`; **имена таблиц — в единственном числе,
  решение 2026-09-02**)"*
  [source: e64bcc6:docs/DESIGN.md:313 · `sed -n '313p' docs/DESIGN.md`]

The two sites are three lines apart in the same section. The plural spelling has also
propagated to three live derived sites: `AGENTS.md:84` (the API-stability carve-out),
`ai-docs/context.md:36` (the Scheduler block row) and `ai-docs/key-decisions.md:15` (KD-4)
[source: e64bcc6 · `grep -rn 'scheduled_tasks' AGENTS.md ai-docs/context.md ai-docs/key-decisions.md`].
Every table in migration 00001 is singular — `owner`, `scope`, `account`, `posting`,
`journal_entry`, `player_operation`, `manual_correction`
[source: 7039e36:internal/store/migrations/00001_ledger_core.sql · `grep -n '^CREATE TABLE' internal/store/migrations/00001_ledger_core.sql`].

**Resolution: singular — and this task closes the conflict at the source, not only in the
derived sites.** The name is `scheduled_task`, chosen by the product owner in issue #20,
whose title and whose Scope both spell it that way, consistent with the dated naming
decision three lines below the offending line in the same §11.

**The round-4 reasoning for leaving line 310 alone no longer holds.** That reasoning was
that a task does not edit `docs/DESIGN.md` (`AGENTS.md` § Project) — a rule whose own
text makes an explicit user request the exception, and at round 5 the owner supplied
exactly that request. All four sites are therefore corrected in this PR under Scope 11:
the source line and the three derived ones. The two quoted lines stay on the record above
because they document *why* the correction is right — §11 is not being redesigned, it is
being made to obey a decision it already states.

## Acceptance Criteria

Every criterion below is settled: the payload and completed-row shape by the round-1
answers, the recurrent row by round 2, the cadence and the failure policy by round 3,
AC19's exclusion set plus the `docs/DESIGN.md` spelling correction by the round-5
amendment, and the claim's row-lock mode — AC35 and AC36 — by the round-6
amendment. AC27 keeps its basis-document scope by the round-7 amendment: the condition
AC35's equivalence rests on is wider than any criterion here enforces, and *Key
decisions* records that gap rather than papering over it.
Note that the failure criteria are **split by task kind** — AC9/AC10/AC26 govern one-shots
and AC32 governs recurrences — because the two kinds settle failure differently by
decision, not by oversight.

| # | Criterion |
|---|-----------|
| AC1 | Package `internal/scheduler` exists and carries a package comment. Every exported item carries a doc comment starting with its name. Every method that reaches the database takes `ctx context.Context` as its first parameter, and no type in the package has a field of type `context.Context`. |
| AC2 | A forward migration under `internal/store/migrations` creates `scheduled_task` and the two basis-document tables. `store.Migrate` against an empty database yields all three; the migration file contains no down section. |
| AC3 | `journal_entry` carries one nullable FK column per basis type, its exactly-one-basis CHECK names every one of them including the two added here, and each new basis column has its own partial unique index — the shape migration 00001 established. |
| AC4 | A balanced posting batch written through `store.Post` under each of the two new basis types succeeds, and the resulting `journal_entry` row has exactly one non-null basis column. A row attempting two non-null basis columns is refused by the database. |
| AC5 | The claim is a batch bounded by the configured limit: with more due tasks than the limit, one claim returns at most that many rows, and rows already claimed by another open transaction are skipped rather than waited on. |
| AC6 | Two workers running concurrently against the same table never execute the same task. A test with many due tasks and two concurrent workers asserts every task's handler ran exactly once, and passes under the race detector. |
| AC7 | A task and its effects commit or roll back together: after a handler that writes and then returns an error, none of its writes are visible and the task row records the failed attempt rather than a success. |
| AC8 | A handler can report a guard-miss no-op. The task reaches a terminal completed state, its writes are absent, and the outcome is reported to the observation seam as an outcome class distinct from both success-with-effects and failure. |
| AC9 | A failing **one-shot** handler is retried with a strictly positive delay between attempts that grows rather than repeats, bounded by the configured attempt cap; after the cap the task is in a terminal give-up state and is never returned by a subsequent claim. A test asserts the exact attempt count and the terminal state. Neither the backoff schedule nor the attempt cap has any effect on a recurrent task — AC32 governs that kind. |
| AC10 | One-shot tasks in the give-up state are enumerable through the package, each carrying at least its type, its `run_at`, its attempt count and the last failure's message. |
| AC11 | A recurrent task is one row whose `run_at` advances in place: completing an occurrence updates that row rather than deleting it and inserting another, and the row's identity is unchanged across occurrences. The count of live rows for one recurrence is exactly one at every commit boundary — after a committed execution, after a rolled-back execution (the current one, still due), and after a failed execution rescheduled to its next cadence instant. Never zero, never two. |
| AC12 | For every executed task the worker reports exactly one observation carrying at least the task type, the lag between `run_at` and the execution instant, the outcome class of AC8, the size of the claim batch the task came from, and — when the outcome is a failure — the row's consecutive-failure count; the worker also reports its loop duration. A test collects observations for a success, a guard no-op, a one-shot retry, a one-shot give-up and a repeatedly failing recurrence. |
| AC13 | No non-test Go file in `internal/scheduler` imports a metrics-registry package, and the package compiles and its tests pass with no observation implementation installed. |
| AC14 | Every operational tuning value — poll interval, claim batch limit, attempt cap, backoff base and backoff ceiling — is a `LAB_GAME_`-prefixed environment variable that `internal/config` reads and exposes as a typed field. No literal value for any of them appears at a call site in `internal/scheduler`, and a test constructs the worker with non-default values and observes the changed behaviour. |
| AC15 | Each new key is optional: loading with all of them absent succeeds and yields the documented default for each; one present and well-formed yields that value; one present and malformed fails with an error naming that variable. The three-way key-set equality the configuration layer asserts — `.env.example`, the loader's consulted keys, and `config.EnvKeys()` — holds with the new keys included, each documented in `.env.example` with a non-empty value. |
| AC16 | Adding the deferred priority / queue column later is an additive migration, not a rewrite: the design document names the claim query and the index serving it, and states what an added priority column changes — a column, an index and an ordering — with no second table, no Go type rename and no data backfill. No Go type or query in this change is named or shaped so that a priority dimension would contradict it. |
| AC17 | Every test in this change that touches the database runs against a real PostgreSQL server through `internal/testdb`, each in its own schema. No test in the change substitutes a fake or mock database for it. |
| AC18 | `internal/testdb` is imported only from `_test.go` files, so `cmd/bot`'s dependency graph stays free of the container runtime. |
| AC19 | The string `scheduled_tasks` occurs in no tracked file, subject to exactly these three exclusions and no others: (a) `.gitignore`, where the match is part of the harness lock-file path `.claude/scheduled_tasks.lock` — a file belonging to the agent harness, not a spelling of this table at all; (b) any file under `ai-docs/plans/done/`, which is history rather than a live surface; (c) this task's own `ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md` and `ai-docs/plans/2026-09-05-postgres-task-scheduler.design.md`, which quote the superseded spelling as the evidence for correcting it and would falsify themselves if they could not. Every other tracked surface spells the table `scheduled_task` — `docs/DESIGN.md` included, since Scope 11 corrects it rather than excluding it. |
| AC20 | `store.PostingBasis`'s doc comment describes the set of implementations the package actually has after this change; no live sentence in the tree still asserts the sum type has exactly two. |
| AC21 | An unregistered task type is a refusal — at registration, at insertion or at claim time — and never a claimed row that fails silently or a due row that no worker will ever take. A test asserts the refusal and asserts that such a row does not accumulate retries invisibly. |
| AC22 | Propagation is complete for this change: every site whose claim the diff falsifies is updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4. **Two members are named rather than illustrative, and both are required**, because `AGENTS.md` § Project's AXIOM makes them the edits a reviewer must find authorised — and they land on the *same* line, `docs/DESIGN.md:310`: after this change that line spells the table `scheduled_task` (owner's round-5 request) and writes the claim query's row lock as `FOR NO KEY UPDATE SKIP LOCKED` (owner's round-6 request), both recorded in Scope 11. **The bound is those two edits and no more:** the rest of line 310 — the one-transaction-with-its-effects clause included — and every other line of `docs/DESIGN.md`, §3.5's `SELECT session FOR UPDATE` among them, are byte-identical to their pre-change text. The remaining sites known at spec time are illustrative, not exhaustive: `ai-docs/context.md` (the Scheduler block row and the Status code paragraph), `ai-docs/key-decisions.md` KD-4, `AGENTS.md` § API Stability's carve-out sentence, `internal/store/basis.go`'s `PostingBasis` doc comment, `.env.example`, `internal/config/env.go`, and `ai-docs/plans/INDEX.md`. The Scheduler block row and KD-4 each carry two falsified claims rather than one, stating the table name and the lock mode in the same sentence. |
| AC23 | Every gate `make verify` runs is green on the resulting tree, including the race-enabled test gate. |
| AC24 | `scheduled_task` carries its type-specific data in a single JSONB payload column. A test round-trips a payload through insert, claim and execution and asserts the handler receives the value that was scheduled, including for a payload containing a nested object and a null. |
| AC25 | A **one-shot** task that completes — whether with effects or as a guard no-op — leaves no row in `scheduled_task`: the deletion happens in the same transaction as the effects, so a rolled-back execution leaves the row present and still due. A test asserts both directions. There is no terminal `completed` status value in the schema and no retention step in the change. A recurrent task is the stated exception and is governed by AC11 instead. |
| AC26 | A payload the handler cannot decode is classified as a failure rather than as a guard no-op. For a one-shot it terminates: the task reaches the give-up state within the configured attempt cap rather than being retried indefinitely, and it is enumerable under AC10 with the decode failure as its recorded reason. For a recurrence it does not terminate — AC32 applies — and the evidence that it is permanently broken is the rising consecutive-failure count of AC34. |
| AC27 | No basis-document table carries a foreign key to `scheduled_task`: deleting a completed task row neither fails nor cascades, and the basis document and its postings survive it. A test posts under a new basis type, lets the task complete and be deleted, and asserts the `journal_entry` and `posting` rows are still present and still balance. *Cross-reference, not part of the condition:* this ban is one half of a pair with AC35's lock mode, and it is the **narrower** half — see the equivalence row in *Key decisions* for what the pairing does and does not cover. |
| AC28 | Every persisted payload key is treated as a data contract: the design document names the payload keys each shipped task type reads, and a pending row written before a change still decodes after it. No live sentence in the tree claims that a JSONB payload is exempt from `AGENTS.md` § API Stability's forward-migration carve-out. |
| AC29 | Scheduling a recurrence twice is refused by the database, not by a code path that checks first: with a recurrence already live, a second insert of the same task type and instance key fails on a uniqueness constraint. A one-shot task carrying no instance key is accepted, and two such one-shots of the same type coexist — the constraint does not force a one-shot to invent an identity. |
| AC30 | A handler that neither returns nor fails within the configured per-task execution deadline is abandoned rather than left holding its row: the task's transaction ends, the row becomes claimable again, and the event is reported through the observation seam as a failure rather than passing silently. A test drives a handler that blocks past the deadline and asserts the row is claimable afterwards and the observation was made. |
| AC31 | The schema contains no liveness-protocol machinery: no column recording that a task is currently being executed, no heartbeat timestamp, and no worker path that revives a task on the basis of a missed heartbeat. A test asserts the self-healing property this replaces — a worker whose transaction is aborted mid-execution leaves the task still due, with none of its writes visible, and the next claim returns it. |
| AC32 | A failing **recurrent** handler never reaches a terminal state: the row is rescheduled to its next cadence instant and remains claimable, and no number of consecutive failures converts it into a give-up. A test fails a recurrence's handler more times than the one-shot attempt cap and asserts the row is still live, still recurring, and still advancing by its cadence. |
| AC33 | Start-up reconciles the declared recurrences against the table in two moves and no more: a declared recurrence with no row is seeded, and a declared recurrence whose row disagrees with the declaration has its `run_at` corrected. Seeding is idempotent — two workers starting concurrently produce one row, refused by the constraint of AC29 rather than by a lock — and neither move disturbs an occurrence that is in flight or imminent. A test covers the missing-row seed, the changed-declaration correction, the concurrent start, and the imminent occurrence left alone. |
| AC34 | The consecutive-failure count is persisted on the task row, incremented on each failed execution, reset on a success or a guard no-op, and carried in the failure observation of AC12. A test asserts the count rises across successive failures of one recurrence and returns to zero after a success. |
| AC35 | The worker's row lock is `FOR NO KEY UPDATE`: the batch claim and the per-id re-claim each take `FOR NO KEY UPDATE … SKIP LOCKED`, and no query against `scheduled_task` in the change takes `FOR UPDATE`. Every live document that states **the scheduler's claim query** states it with `FOR NO KEY UPDATE` — `docs/DESIGN.md` §11 included, per AC22. Four surfaces match a search for the old clause without violating this criterion, and they are named so the check needs no judgement call: files under `ai-docs/plans/done/` (history); this task's own spec and design (they quote the pre-change clause as the evidence for changing it); this task's `*.spec.md.state.md` interview state, which is retired before the PR in any case; and `docs/DESIGN.md` line 147, which is §3.5's raid-session lock rather than the scheduler's claim. `.claude/agents/self-review.md`'s Postgres-invariants rule is outside the criterion too — it names `SKIP LOCKED` behaviour as an example of a database-enforced invariant, not this worker's query — but it remains inside AC22's propagation class if its example now misleads. The exclusion protocol is unchanged under this mode and is asserted rather than assumed: AC5 and AC6 hold as written. |
| AC36 | The recorded reason for AC35's mode is the one that survives measurement, and no live sentence in the tree claims otherwise: no document, comment or commit message in the change justifies `FOR NO KEY UPDATE` as a throughput, latency, contention or lock-weight improvement over `FOR UPDATE`. The reason each such surface gives is the pair in *Key decisions* — the weakest lock expressing what a claim does, with a one-shot's `DELETE` taking `FOR UPDATE` strength at the moment it is needed — together with the condition that equivalence rests on — that no foreign key anywhere references `scheduled_task` — which AC27 enforces for basis-document tables only. A surface that states the coupling states it at that reach, never as a completed guarantee. |

## Open questions

- **Whether "recurrent task" and "cron task (day close)" stay two basis types or
  collapse into one.** §11's starter registry names both
  [source: 7039e36:docs/DESIGN.md:331 · `sed -n '331p' docs/DESIGN.md`]; issue #20 scopes this task to two tables and
  names them "deferred one-shot" and "recurrent". The day close (#45) is the only cron
  in the system and reads naturally as a recurrent task, so this spec ships two tables
  and leaves the third name unclaimed. If #45 wants its own document type, that is one
  additive migration and one CHECK line — exactly the friction §11 designed in.
- **Whether the scheduler should ever refuse to claim when the database is behind.** A
  claim batch that is always full is the signal that lag is accumulating; §11's stated
  early signal is fan-out delaying gameplay edges, and the remedy it names is priority,
  which is out of scope here. The batch-size observation (AC12) is what will show it.
- **A transient failure of a slow recurrence costs a full cycle.** "Reschedule to the
  next cadence instant" applied to a daily job means one failed run waits a day, where a
  bounded backoff would have retried in minutes. The owner chose this knowingly — the
  option said exactly that, and it is what removes the terminal state — so it is recorded
  as a consequence, not reopened. If it bites, the two ways out that do not reintroduce a
  give-up state are a more frequent cadence for the affected type, or a handler that
  schedules its own one-shot retry; both are the mechanic's decision, not the scheduler's.
- **Where the alert for a stopped recurrence lives.** Three signals make a stopped
  recurrence *visible*: AC34's consecutive-failure count, which is the one that separates
  a single permanently broken recurrence from ordinary noise, together with the one-shot
  dead-task enumeration (AC10) and the per-type counters (AC12). Turning visible into
  *noticed* is an alert rule, and alert rules belong to #23's dashboard, which has none
  yet. This spec deliberately ships the signal and not the rule.
- **Whether a row whose task type is no longer registered should eventually be removed
  automatically.** Under move-forward a recurrence outlives every deploy, so a type
  dropped from the registry leaves a row that comes due forever and is refused every time
  (AC21). This spec makes retiring a recurrence an explicit act (*Key decisions*) rather
  than adding a garbage collector, because an automatic deleter of rows nobody claims is
  a mechanism that removes evidence of the mistake it is covering for. If unclaimable
  rows ever accumulate in practice, the refusal counter is what will show it.
- **A guard no-op now leaves no per-occurrence trace.** Delete-on-done plus
  counter-shaped instrumentation means a stale task that died is afterwards a number, not
  a record: nothing says *which* occurrence died or what its payload was. §3.5 makes that
  path normal traffic rather than an incident, so what is lost is post-mortem material,
  not correctness — and the place to record it, if a raid post-mortem ever wants it, is
  the event log of §13.1, which has no migration yet. Flagged rather than solved, because
  solving it here would build a second history table the owner's answer deliberately
  removed.
