# Postgres task scheduler: `scheduled_task`, the `SKIP LOCKED` worker, one-shot and recurrent tasks

**Source:** issue #20
**Date:** 2026-09-05
**Tracked in:** #20

This project deliberately has no cron: stamina, backpack evaporation and monster respawn
are lazy ticks computed from timestamps at read time. One component is the stated
exception — a hand-written task scheduler on Postgres. Its table — `scheduled_task`,
singular, per *Source conflicts* below — carries `run_at`, a type, a payload and a
status; its worker claims due rows with
`SELECT … WHERE run_at <= now() FOR UPDATE SKIP LOCKED LIMIT N`; and a task executes
**in one transaction with its own effects**, which is what makes it exactly-once with no
two-phase machinery
[source: 7039e36:docs/DESIGN.md:309-310 · `sed -n '309,310p' docs/DESIGN.md`].

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
   (Scope 8). Whether that delete also governs a *recurrent* occurrence is the open
   round-2 question; the rest of the schema does not depend on its answer.

3. **The two new basis types in Go, and they must live in `internal/store`.**
   `PostingBasis` is a **sealed** sum type: both its methods are unexported, and its doc
   comment states it is satisfied by "exactly the store package's two implementations"
   [source: 7039e36:internal/store/basis.go:11-25 ·
   `sed -n '11,25p' internal/store/basis.go`]. A basis type therefore *cannot* be
   declared in `internal/scheduler` — the compiler forbids it. The two new documents are
   `internal/store` types alongside `PlayerOperation` and `ManualCorrection`, and that
   doc comment's "exactly two" stops being true in this change (see AC20).

4. **The worker: claim, execute, settle.** A batch claim of due rows with
   `FOR UPDATE SKIP LOCKED` and a configurable limit; one transaction per task carrying
   the task's own effects; a handler registry keyed by task type. The transaction is the
   worker's and is handed to the handler, mirroring `store.Post`, which takes a
   caller-owned `pgx.Tx` and neither commits nor rolls back
   [source: 7039e36:internal/store/post.go:74 · `grep -n '^func Post' internal/store/post.go`].
   A handler that posts does so through `store.Post` inside that same transaction — the
   ledger AXIOM is not relaxed for scheduler effects.

5. **Guard semantics as a first-class outcome.** A handler may decide the task is stale
   and complete it as a **no-op**. That is normal operation, and it is a *third* outcome
   class alongside "executed with effects" and "failed" — distinguished in the
   instrumentation, never counted as an error (Scope 8).

6. **Failure policy: bounded retry, a terminal give-up state, and visibility of the
   dead.** A handler error re-schedules the task with a strictly positive, growing delay
   up to a configured attempt cap, after which the task reaches a terminal give-up state
   and is never claimed again. Tasks in that state are enumerable through the package
   with enough context to diagnose them (type, `run_at`, attempts, last error).

7. **Recurrent tasks: schedule-next-on-completion, idempotent by construction.** The
   property this task must have, stated without committing to a representation: the next
   occurrence becomes due in the **same transaction** that completes the current one, so
   a commit leaves exactly one live occurrence of that recurrence and a rollback leaves
   the current one still due. Whether that is the same row moved forward or a fresh
   successor row — and where the cadence is declared — is the open round-2 question, and
   it is the last thing this spec is waiting on.

8. **Per-type lag instrumentation, through a seam rather than a registry.** The worker
   reports, per executed task, at least: task type, the lag between `run_at` and the
   execution instant, the outcome class of Scope 5, and the size of the claim batch the
   task came from; plus the worker loop's own duration. The shape follows the precedent
   this repository already set for the transport — a plain struct plus a consumer-declared
   one-method interface, with the implementation supplied elsewhere
   [source: 7039e36:internal/tg/observe.go:5-33 · `cat internal/tg/observe.go`].
   `github.com/prometheus/client_golang` is not reachable from this module today
   [source: 7039e36:go.mod · `go mod why -m github.com/prometheus/client_golang`], and
   `/metrics` plus the registry belong to #23 (open, verified
   `gh issue view 23 --json state`), which already names scheduler lag in its title.

9. **Operational tuning as configuration keys.** The poll interval, the claim batch
   limit, the retry attempt cap and the backoff base and ceiling are `LAB_GAME_`-prefixed
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

11. **Propagation, as a class.** This change renames a table that live documents name,
    adds an environment-variable class, and falsifies the "exactly two implementations"
    sentence in `internal/store/basis.go`. Every site whose claim the diff falsifies is
    updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4 —
    the sites named in AC22 illustrate the class and do not bound it.

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
- **Editing `docs/DESIGN.md`.** It is decisions, and a task does not redesign it
  (`AGENTS.md` § Project). The naming inconsistency this task meets is recorded under
  *Source conflicts* and surfaced in *Open questions* instead.
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
| Whether a guard no-op is a failure | **No — it is a third outcome class.** `docs/DESIGN.md` §3.5 and `ai-docs/domain-invariants.md` § 4 both state a stale task dying at execution is expected traffic; the instrumentation must be able to tell it from a failure, or the health dashboard will read normal operation as an incident [source: 7039e36:ai-docs/domain-invariants.md:43 · `sed -n '43p' ai-docs/domain-invariants.md`]. |
| How a task type relates to a basis document | **A task type names the basis document its effects post under, and the two new tables cover only the tasks that have no other basis.** §11's starter registry lists the raid-session transition and the day-close cron as basis types of their own [source: 7039e36:docs/DESIGN.md:331 · `sed -n '331p' docs/DESIGN.md`], and §3.5 makes a raid transition its own document [source: 7039e36:docs/DESIGN.md:153 · `sed -n '153p' docs/DESIGN.md`]. So a raid timer edge posts under #36's transition document; corpse evaporation posts under a deferred one-shot; the day close posts under a recurrent task. Issue #20 scopes this task to the latter two tables, and that is what it ships. |
| Whether every executed task writes a basis row | **No.** A basis document exists to anchor postings; a task whose effects move no balance (a notification send) writes none. The basis row is created inside the executing transaction, by the code that posts. |
| The task-type value set | **Left to the design, with the project's precedent as the default:** every existing categorical column is a PostgreSQL enum (`owner_kind`, `ledger_kind`, `operation_source`) [source: 7039e36:internal/store/migrations/00001_ledger_core.sql:2-4 · `sed -n '2,4p' internal/store/migrations/00001_ledger_core.sql`], and a persisted enum value is a data contract that changes by forward migration only (`AGENTS.md` § API Stability carve-out). Whichever representation the design picks, AC21 binds: an unregistered type must be a refusal, never a row that stays due forever. |
| Where the operational tuning values live | **Environment keys with compiled-in defaults**, in KD-27's optional-with-default class, read by `internal/config` and listed in `.env.example`. KD-27's boundary is respected exactly: it names what stays required — secrets, base URL, chat allowlist, the two file paths — and forbids a fallback for any balance number; a poll interval is none of those. |
| Metric coupling | **No metrics-registry import in `internal/scheduler`.** The package defines the observation point; #23 supplies the implementation, exactly as `internal/tg` did [source: 7039e36:internal/tg/observe.go:27-33 · `sed -n '27,33p' internal/tg/observe.go`]. |
| Whether this task wires the worker into `cmd/bot` | **No — #24.** The transport task set the precedent: ship the package, let the composition root construct it [source: 7039e36:ai-docs/context.md:43 · `sed -n '43p' ai-docs/context.md`]. |
| The table's name | **`scheduled_task`, singular**, matching issue #20's title and Scope and the 2026-09-02 table-naming decision that every table in migration 00001 already follows. §11's prose spells it plural in one place; see *Source conflicts*. |
| Payload representation | **One `payload jsonb` column** (owner, round 1). Each handler decodes its own shape; a new task type costs no migration for its payload. The two costs are accepted explicitly in the same answer: a malformed payload is caught at execution rather than by the database, and a reference held inside the payload can dangle. Consistent with the repository's existing split — the ledger is strict schema, the event log is JSONB, and they are deliberately different tables [source: 00a58ac:ai-docs/domain-invariants.md:52 · `sed -n '52p' ai-docs/domain-invariants.md`]. |
| Whether JSONB relaxes the data-contract rule for payload keys | **No, and this is the trap the choice creates.** `AGENTS.md` names the scheduler's payloads, by that word, among the live data that outlives every deploy and changes by forward migration only [source: 00a58ac:AGENTS.md:84 · `grep -n 'scheduler.s .scheduled_tasks. payloads' AGENTS.md`]. A payload key is a persisted name: renaming one, re-typing one, or repurposing one is the same defect as doing it to a column, and a pending row written by the previous deploy must still decode. "It is only JSON" is not a licence. |
| What a payload the handler cannot decode means | **A failure, not a guard no-op** — and one that must not loop. It is not the "state moved on" case §3.5 describes; it is a defect that will reproduce identically on every attempt, so it must reach the terminal give-up state and become visible (AC26) rather than consume the retry budget forever. |
| What a dangling reference inside a payload means | **A guard miss — the default, and a handler may classify otherwise.** §3.5 makes orphaned tasks normal by design: cancellation is unnecessary and a task whose subject has moved on dies at execution [source: 7039e36:docs/DESIGN.md:154 · `sed -n '154p' docs/DESIGN.md`]. A referent that no longer exists is that same case. What the spec fixes is that both classifications are representable and observable; which one a given handler picks travels with the mechanic. |
| Lifecycle of a completed row | **Deleted by the transaction that completes it** (owner, round 1). The table holds pending and dead tasks only. Consequences, all of them intended: no retention step and no growth curve to manage; no terminal `completed` status value and no `executed_at`; the observation seam is the **only** place execution lag ever exists, since there is no row left to compute it from (AC12 carries the whole weight); and a guard no-op leaves no per-occurrence trace beyond its counter — see *Open questions*. |
| Retention | **None, and none is needed** — it follows from the row above rather than being a separate decision. §11 places retention outside MVP in any case [source: 7039e36:docs/DESIGN.md:330 · `sed -n '330p' docs/DESIGN.md`]. |
| Which direction the basis-document / task reference points | **Never from a basis document to `scheduled_task`.** Delete-on-done means the task row is gone while its basis document and postings live on, so a hard FK that way would either block the delete or cascade away ledger history. §11 fixes the direction for exactly this reason: FKs point from postings to bases, which is what makes dropping postings first always safe [source: 7039e36:docs/DESIGN.md:330 · `sed -n '330p' docs/DESIGN.md`]. A basis document carries by value whatever it needs to identify the task that produced it; which fields those are is the design's. |
| Where a recurrent task's next `run_at` comes from | **TBD — re-asked in round 2** after the owner asked for prior art rather than choosing. The round-2 options are informed by the `kagkarlsson/db-scheduler` research recorded under *Technical constraints* § *Provenance*, which surfaced two forks the round-1 options had collapsed: whether a recurring task is one row moved forward or a successor row, and where its cadence is declared. |
| Whether a recurrent chain can end in the terminal give-up state | **TBD — depends on the round-2 answers, and may dissolve.** If a recurrence is declared in code and re-seeded at start-up, a give-up is self-healing and the question does not arise; if the cadence lives only in a handler's return value, a give-up ends the day close permanently and the policy needs deciding. Issue #20's scope names a give-up state without saying it governs every task kind. |

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
  reads the library's own source and pins it there.
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

`docs/DESIGN.md` §11 disagrees with itself about the scheduler table's name.

- `docs/DESIGN.md:310` — *"таблица `scheduled_tasks` (run_at, тип, payload, статус)"*
  [`sed -n '310p' docs/DESIGN.md`]
- `docs/DESIGN.md:313` — *"(таблица `posting`; **имена таблиц — в единственном числе,
  решение 2026-09-02**)"* [`sed -n '313p' docs/DESIGN.md`]

The plural spelling has also propagated to three live derived sites:
`AGENTS.md:84` (the API-stability carve-out), `ai-docs/context.md:36` (the Scheduler
block row) and `ai-docs/key-decisions.md:15` (KD-4)
[`grep -rn 'scheduled_tasks' AGENTS.md ai-docs/context.md ai-docs/key-decisions.md`].
Every table in migration 00001 is singular — `owner`, `scope`, `account`, `posting`,
`journal_entry`, `player_operation`, `manual_correction`
[source: 7039e36:internal/store/migrations/00001_ledger_core.sql · `grep -n '^CREATE TABLE' internal/store/migrations/00001_ledger_core.sql`].

**Resolution: singular. Chosen by the product owner in issue #20**, whose title and whose
Scope both spell it `scheduled_task`, consistent with the dated naming decision in the
same §11 paragraph. The plural in §11's prose predates that decision and is left
untouched, because a task does not edit `docs/DESIGN.md` (`AGENTS.md` § Project); the
three derived sites are corrected under Scope 11. Whether §11 itself should be fixed is
in *Open questions*, and nothing here blocks on the answer.

## Acceptance Criteria

One criterion is still owed and lands once the recurrence question is answered: how a
recurrent occurrence is represented, and therefore whether AC25's delete governs it. The
payload and completed-row criteria below are settled by the owner's round-1 answers.

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
| AC9 | A failing handler is retried with a strictly positive delay between attempts that grows rather than repeats, bounded by the configured attempt cap; after the cap the task is in a terminal give-up state and is never returned by a subsequent claim. A test asserts the exact attempt count and the terminal state. |
| AC10 | Tasks in the give-up state are enumerable through the package, each carrying at least its type, its `run_at`, its attempt count and the last failure's message. |
| AC11 | A recurrence's next occurrence becomes due in the same transaction as the execution that completes the current one, and the count of live occurrences of one recurrence is exactly one at every commit boundary: one after a committed execution, one after a rolled-back execution (the current one, still due), and one after a retried-then-succeeded execution — never zero, never two. The criterion is stated on the count, not on the row identity, so it holds under either representation. |
| AC12 | For every executed task the worker reports exactly one observation carrying at least the task type, the lag between `run_at` and the execution instant, the outcome class of AC8, and the size of the claim batch the task came from; the worker also reports its loop duration. A test collects observations for a success, a guard no-op, a retry and a give-up. |
| AC13 | No non-test Go file in `internal/scheduler` imports a metrics-registry package, and the package compiles and its tests pass with no observation implementation installed. |
| AC14 | Every operational tuning value — poll interval, claim batch limit, attempt cap, backoff base and backoff ceiling — is a `LAB_GAME_`-prefixed environment variable that `internal/config` reads and exposes as a typed field. No literal value for any of them appears at a call site in `internal/scheduler`, and a test constructs the worker with non-default values and observes the changed behaviour. |
| AC15 | Each new key is optional: loading with all of them absent succeeds and yields the documented default for each; one present and well-formed yields that value; one present and malformed fails with an error naming that variable. The three-way key-set equality the configuration layer asserts — `.env.example`, the loader's consulted keys, and `config.EnvKeys()` — holds with the new keys included, each documented in `.env.example` with a non-empty value. |
| AC16 | Adding the deferred priority / queue column later is an additive migration, not a rewrite: the design document names the claim query and the index serving it, and states what an added priority column changes — a column, an index and an ordering — with no second table, no Go type rename and no data backfill. No Go type or query in this change is named or shaped so that a priority dimension would contradict it. |
| AC17 | Every test in this change that touches the database runs against a real PostgreSQL server through `internal/testdb`, each in its own schema. No test in the change substitutes a fake or mock database for it. |
| AC18 | `internal/testdb` is imported only from `_test.go` files, so `cmd/bot`'s dependency graph stays free of the container runtime. |
| AC19 | No live surface in the tree names the scheduler's table in a spelling other than `scheduled_task`. Surfaces under `ai-docs/plans/done/` are history and are excluded from this criterion, not counterexamples to it. |
| AC20 | `store.PostingBasis`'s doc comment describes the set of implementations the package actually has after this change; no live sentence in the tree still asserts the sum type has exactly two. |
| AC21 | An unregistered task type is a refusal — at registration, at insertion or at claim time — and never a claimed row that fails silently or a due row that no worker will ever take. A test asserts the refusal and asserts that such a row does not accumulate retries invisibly. |
| AC22 | Propagation is complete for this change: every site whose claim the diff falsifies is updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4. Sites known at spec time — illustrative, not exhaustive: `ai-docs/context.md` (the Scheduler block row and the Status code paragraph), `ai-docs/key-decisions.md` KD-4, `AGENTS.md` § API Stability's carve-out sentence, `internal/store/basis.go`'s `PostingBasis` doc comment, `.env.example`, `internal/config/env.go`, and `ai-docs/plans/INDEX.md`. |
| AC23 | Every gate `make verify` runs is green on the resulting tree, including the race-enabled test gate. |
| AC24 | `scheduled_task` carries its type-specific data in a single JSONB payload column. A test round-trips a payload through insert, claim and execution and asserts the handler receives the value that was scheduled, including for a payload containing a nested object and a null. |
| AC25 | A task that completes — whether with effects or as a guard no-op — leaves no row in `scheduled_task`: the deletion happens in the same transaction as the effects, so a rolled-back execution leaves the row present and still due. A test asserts both directions. There is no terminal `completed` status value in the schema and no retention step in the change. |
| AC26 | A payload the handler cannot decode is classified as a failure rather than as a guard no-op, and it terminates: the task reaches the give-up state within the configured attempt cap rather than being retried indefinitely, and it is enumerable under AC10 with the decode failure as its recorded reason. |
| AC27 | No basis-document table carries a foreign key to `scheduled_task`: deleting a completed task row neither fails nor cascades, and the basis document and its postings survive it. A test posts under a new basis type, lets the task complete and be deleted, and asserts the `journal_entry` and `posting` rows are still present and still balance. |
| AC28 | Every persisted payload key is treated as a data contract: the design document names the payload keys each shipped task type reads, and a pending row written before a change still decodes after it. No live sentence in the tree claims that a JSONB payload is exempt from `AGENTS.md` § API Stability's forward-migration carve-out. |

## Open questions

- **Should `docs/DESIGN.md` §11's prose be corrected to the singular table name?** §11
  states the singular-names decision and then spells this one table plural, six lines
  apart. That is an inconsistency inside a decisions document rather than a decision to
  revisit, but `docs/DESIGN.md` is the owner's and a task does not edit it — the same
  posture the transport task took when it found §11 incomplete on a rate-limit figure.
  Nothing here blocks: the code, the migration and the derived documents use the singular
  name either way.
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
- **How a dead recurrent chain is noticed, if the round-2 answers leave one possible.**
  Where nothing re-seeds a recurrence, a give-up ends the day close permanently and
  silently. The dead-task enumeration (AC10) and the per-type counters (AC12) make it
  *visible*; the *alert* belongs to #23's dashboard, which has no rules yet. If the
  answers make a chain self-healing, this question closes with them.
- **A guard no-op now leaves no per-occurrence trace.** Delete-on-done plus
  counter-shaped instrumentation means a stale task that died is afterwards a number, not
  a record: nothing says *which* occurrence died or what its payload was. §3.5 makes that
  path normal traffic rather than an incident, so what is lost is post-mortem material,
  not correctness — and the place to record it, if a raid post-mortem ever wants it, is
  the event log of §13.1, which has no migration yet. Flagged rather than solved, because
  solving it here would build a second history table the owner's answer deliberately
  removed.
