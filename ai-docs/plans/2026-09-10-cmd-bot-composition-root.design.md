# Design: `cmd/bot` composition root — wiring, migration policy, graceful shutdown

**Issue:** #24
**Date:** 2026-09-10

## Approach

### The shape

`cmd/bot` stays the composition root — the spec names it in as many words, and AC1 asserts about
`cmd/bot` itself, so no `internal/app` package is introduced. Package `main` is split across files
by phase (dispatch, assemble, readiness, drain, migrate-only) and its tests sit beside them, the
shape `cmd/commentrefs` already uses
[measured 71ce0e4:cmd/commentrefs · ls cmd/commentrefs → git.go git_test.go main.go run.go run_test.go].

These values carry the whole design:

- **`app`** — the assembled process: the pool, the logger, the registry, every constructed
  subsystem, and a `[]runner`.
- **`runner`** — `{name string; run func(ctx) error; stop func()}`: the ingest loop, the scheduler
  worker, the liveness heartbeat. The name is what a failure message and a drain
  timeout report, so "naming the subsystem that failed" is a property of the data structure rather
  than of a hand-written string at each call site.
- **`readiness`** — the `migrated` and `draining` latches plus the pool. It is the *one* definition
  AC22 asks for, consumed by both the HTTP path and the metrics gauge.

Entry points:

```
main()  ->  os.Exit(run(os.Args[1:], os.LookupEnv, os.Stderr, os.Stdout))
run()   ->  argv dispatch:  (no args) -> serve ;  "migrate" -> migrate-only ;  -h/--help -> usage
```

`main` stays a one-line `os.Exit(run(...))` and `run` never terminates the process itself — the
existing contract, kept
[measured 71ce0e4:cmd/bot/main.go:20,27-28 · sed -n '20p;27,28p' cmd/bot/main.go → "	os.Exit(run(os.LookupEnv, os.Stderr, os.Stdout))", "// stdout. run never terminates the process itself: main exiting non-zero", "// is not a panic."].

### Start-up: one written order, all-fatal, unwinding

The table below **is** the order. Subtask 9 implements it and subtask 14 writes it into a new page,
`ai-docs/process-lifecycle.md` — the artefact AC2 and AC3 require — so it is fixed here once instead
of being derived twice, in two groups, from two constraints. One row per step: what it constructs or
starts, how it fails, the step name its stderr line carries (AC4), and what the unwind stops on the
way out (AC20).

| # | Step | Constructs / starts | Failure mode | stderr step name | A failure here unwinds |
|---|---|---|---|---|---|
| 1 | configuration | `config.Load(lookup)` | a missing or malformed key, reported as the joined per-key error the package already returns | `configuration` | nothing has started |
| 2 | logger | the process `slog.Logger` over stderr | none: no key is read and no I/O is done at construction | — | — |
| 3 | database | `pgxpool.ParseConfig` over the DSN, `store.NewPool`, then one `Ping` under a bounded context — the ping is what makes an unreachable database *this* step's failure rather than the migration step's | an unparsable DSN; a database that does not answer the ping | `database` | the pool |
| 4 | metrics registry | `health.NewRegistry`, `RegisterRuntime`, the transport, scheduler and ingest observers, `NewPoolCollector`, `NewProcess` | a family the registry refuses | `metrics registry` | the pool |
| 5 | readiness | the `readiness` value: both latches unset, over the pool | none | — | — |
| 6 | health listener | `health.NewServer(ServerOptions{Addr, Gatherer, Ready})`, then `Start` | a nil option field; an address already in use — `Start` binds synchronously and returns the bind error rather than losing it on a goroutine [measured f33b2bf:internal/health/server.go:54-57 · sed -n '54,57p' internal/health/server.go → "// Start binds the listener synchronously, returning a bind error before", "// spawning the serve goroutine — so a bind failure is always reported", "// through a returned error, never discovered later on a background", "// goroutine. Returns an error rather than panicking on a second call."] | `health listener` | the listener, the pool |
| 7 | migrations | with auto-apply on, `store.Migrate` under `WithAdvisoryLock`; with it off, `HasPendingMigrations` plus a refusal naming the disabling key when anything is pending; then the `migrated` latch | a migration that fails to apply; auto-apply off with a migration pending (AC8) | `migrations` | the listener, the pool |
| 8 | restart hygiene | `scheduler.NewLiveness`, `AbsorbDowntime`, and `Process.ObserveDowntime` over what it returns | the transaction fails | `restart hygiene` | the listener, the pool |
| 9 | scheduler | `scheduler.New` over the empty registry, then `Reconcile` | an option the constructor refuses; a reconcile that fails | `scheduler` | the listener, the pool |
| 10 | telegram client | `tg.New` over the token, the base URL, the allowlist gate, the transport tuning and the transport observer | an option the constructor refuses — including a token telego's own format check rejects, which is what the example file's placeholder is [measured f33b2bf:.env.example:19 · sed -n '19p' .env.example → "LAB_GAME_BOT_TOKEN=changeme"; go doc github.com/mymmrac/telego.ErrInvalidToken → "ErrInvalidToken bot token is invalid according to token regexp"] | `telegram client` | the listener, the pool |
| 11 | ingest loop | `ingest.New` over the client, the pool, the empty router and the ingest observer | an option the constructor refuses | `ingest loop` | the listener, the pool |
| 12 | canary | `health.NewLegs` with the process HTTP client, `NewCanary`, then `Start` | a leg option the constructor refuses; a canary already started | `canary` | the canary, the listener, the pool |
| 13 | runners | the `[]runner` values — ingest loop, scheduler worker, liveness heartbeat — constructed here, started by `serve` | none | — | — |

Two of those orderings are load-bearing rather than tidy:

1. **The health listener binds before the migration step** — step 6 before step 7, not after.
   Readiness is only observable as a *signal* while the listener is up; binding after migrations
   would make "starting" mean
   "connection refused" rather than 503, and AC22 asks a probe to distinguish starting, serving and
   draining. It also answers the spec's open question about scraping while not ready: the endpoint
   serves throughout.
2. **Restart hygiene runs after migrations and before `Reconcile`** — step 8 before step 9 — and
   `Reconcile` is called explicitly during assembly rather than left to `Worker.Run`. The scheduler
   package documents that
   a composition root may legitimately call it before any worker starts
   [measured 71ce0e4:internal/scheduler/reconcile.go:38-46 · sed -n '38,46p' internal/scheduler/reconcile.go → "It is idempotent — safe to call more than once, and safe for concurrent callers — so Run calls it once before its first cycle and a composition root may legitimately call it again before any worker starts."],
   and calling it here turns a reconcile failure into a *start-up* failure under the all-fatal rule
   instead of an early return from `Run` mid-life
   [measured 71ce0e4:internal/scheduler/worker.go:144-147 · sed -n '144,147p' internal/scheduler/worker.go → "func (w *Worker) Run(ctx context.Context) error { if err := w.Reconcile(ctx); err != nil { return fmt.Errorf(\"scheduler: run: %w\", err) }"].
   It also gives the shift its self-correction: a recurrence the shift pushed forward is pulled back
   by the reconcile that follows it.

Phases:

- **`assemble(ctx, opts) (*app, error)`** — construct everything, and start the subsystems whose
  start can fail or has an external effect (the health listener, then the canary). On any failure it
  shuts down, in reverse order, whatever it already started, then returns the error naming the step.
  That is AC20, implemented once rather than per call site.
- **`(*app).serve(ctx, signals) int`** — start the runners, wait for a signal or the first runner
  return, drain, close.

Every failure at either phase is fatal: a message on stderr naming the step and the cause, a
non-zero exit, nothing on stdout. The canaries are included with no exception, which is the owner's
round-1 answer.

### Shutdown: a stop seam rather than a context cancel

AC25 asks for polling to stop while in-flight work keeps its budget. `Loop.Run` and `Worker.Run`
today have exactly one lever — the context — and cancelling it stops the cycle *and* aborts the work
inside it
[measured 71ce0e4:internal/ingest/loop.go:183,189-190,194 · sed -n '183p;189,190p;194p' internal/ingest/loop.go → "func (l *Loop) Run(ctx context.Context) error {", "		case <-ctx.Done():", "			return ctx.Err()", "		_ = l.PollOnce(ctx)"]
[measured 71ce0e4:internal/scheduler/worker.go:154-155,159 · sed -n '154,155p;159p' internal/scheduler/worker.go → "		case <-ctx.Done():", "			return ctx.Err()", "		_ = w.RunOnce(ctx)"].
So both packages grow a second lever:

- `(*Loop).Stop()` — `Run` returns after the cycle in flight, **and** the in-flight `getUpdates` is
  cancelled. Without the second half every graceful shutdown would wait out the long-poll window,
  whose default is 25s
  [measured 71ce0e4:.env.example:122 · sed -n '122p' .env.example → "LAB_GAME_INGEST_LONG_POLL_TIMEOUT=25s"];
  a poll that returns nothing is worth nothing, so cancelling it discards nothing. The offset is a
  guarded, monotone database row, so a discarded poll simply re-arrives.
- `(*Worker).Stop()` — `Run` returns after the cycle in flight; the claimed batch finishes. No
  poll-cancel counterpart, because a scheduler cycle parks in no long window.

Both are a `sync.Once`-guarded channel closed once and selected on beside `ctx.Done()`; `Run`
returns `nil` when stopped and `ctx.Err()` when cancelled, which is what lets the drain tell a
completed shutdown from an abandoned one (AC28).

**The stop-driven cancel is scoped to the `getUpdates` call and to nothing else.** `PollOnce` takes
one context today and hands that same one to the offset read, to the API call and to every
`processUpdate` in the batch
[measured f33b2bf:internal/ingest/loop.go:145,148,160,167 · grep -n 'func (l \*Loop) PollOnce\|readOffset(ctx\|GetUpdates(ctx\|processUpdate(ctx' internal/ingest/loop.go → "145:func (l *Loop) PollOnce(ctx context.Context) error {", "148:	offset, err := readOffset(ctx, l.pool)", "160:	updates, err := l.client.API().GetUpdates(ctx, params)", "167:		if err := l.processUpdate(ctx, raw); err != nil {"],
so a `Stop` that cancelled *that* context would abort update handling mid-batch — the outcome the
*Rejected* paragraph below rules out. `PollOnce` therefore derives one child context per cycle,
cancelled when either its own `ctx` is done or `Stop` was called, and passes it to `GetUpdates`
**only**; `readOffset` and every `processUpdate` keep the parent `ctx` and run to completion. The
resulting shape, which tests drive directly because `PollOnce` is itself an exported entry point:

```
PollOnce(ctx):
    offset  := readOffset(ctx)                   // parent ctx
    pollCtx := per-cycle child of ctx, also cancelled by Stop
    updates := GetUpdates(pollCtx, params)       // the only stop-cancellable call
    cancel pollCtx and join its watcher before the batch is touched
    for each update: processUpdate(ctx, update)  // parent ctx — the batch settles
```

A `getUpdates` that failed **because** `Stop` cancelled it, with the parent context still live, is a
discarded poll and not a failed one: the cycle reports its one `LoopObservation` with no `Err`, and
`PollOnce` returns an exported sentinel a caller can match. That distinction is not cosmetic — the
health package increments the poll-error counter on exactly `LoopObservation.Err`
[measured f33b2bf:internal/health/ingest.go:92-97 · sed -n '92,97p' internal/health/ingest.go → "func (o *IngestObserver) ObserveLoop(obs ingest.LoopObservation) {", "	o.pollDuration.Observe(obs.Duration.Seconds())", "	o.pollBatch.Observe(float64(obs.BatchSize))", "	if obs.Err != nil {", "		o.pollErrors.Inc()", "	}"] —
so without it every graceful shutdown would mint one poll error that never happened.

**A runner returning before a signal is itself a shutdown trigger.** After `Stop`, a return is the
success path; before it, a return means that subsystem is gone — `Worker.Run` returns outright when
its opening `Reconcile` fails, and the liveness heartbeat returns when its tolerance is spent (§
*Restart hygiene* below). `serve` therefore waits on two things, not one: a signal, and the first
runner return — the wrapper the group runs around each `run` publishes `{name, err}` to a buffered
channel as it returns, and `serve` selects on that channel beside the signal channel; one slot per
runner, so a late return never blocks on a `serve` that has already moved on. A runner that returns
first begins the same graceful shutdown a signal begins, in the
same order and under the same budget, reports that runner's name and its error on stderr, and makes
the exit code non-zero — a process that has lost a subsystem must not go on looking healthy, and
AC19's all-fatal posture at start-up would be hollow if mid-life loss were silent. It is also why
the group joining the runners is a plain `errgroup.Group`: `Wait` supplies the drain's join and its
first error, while the decision to begin the drain stays in `serve`, where AC25's ordering lives.
`errgroup.WithContext` would instead cancel the siblings the moment one returns, aborting exactly
the in-flight work the drain exists to fund.

The drain's own final liveness write is best-effort: a failure there is reported and does not change
the exit code, because the next start then measures its gap from the last successful heartbeat,
which is one interval old at worst and far below the downtime threshold.

Rejected: driving `PollOnce`/`RunOnce` from the composition root and dropping `Run` — it duplicates
each package's tick loop in `cmd/bot`, leaves `Run` dead in production, and moves `Reconcile`'s
"once before the first cycle" contract into a place that cannot see it. Also rejected: accepting a
plain context cancel and re-wording the ACs — the spec's own *Technical constraints* say an
interrupted update and an interrupted task are *safe*, not that they are *free*, and AC25 asks for
the drain.

### Migration policy

`store.Migrate` gains functional options, keeping its current call shape working for every test
fixture:

```
store.Migrate(ctx, pool, logger, opts ...MigrateOption)
store.WithAdvisoryLock(id int64) MigrateOption
store.HasPendingMigrations(ctx, pool, logger) (bool, error)
```

The lock is **opt-in, with a caller-supplied id**, and both halves matter. Opt-in because a
PostgreSQL session-level advisory lock is database-wide while the test suite's isolation is
*schema*-wide — `testdb.Schema` hands out schemas on one shared database
[measured 71ce0e4:internal/testdb/testdb.go:120 · sed -n '120p' internal/testdb/testdb.go → "cfg.ConnConfig.RuntimeParams[\"search_path\"] = name"],
so a lock inside the default `Migrate` path would serialise every fixture in every binary behind one
lock, and goose's own default waits 5 minutes before failing
[measured 71ce0e4 · go doc github.com/pressly/goose/v3/lock.WithLockTimeout → "By default, the lock timeout is 300s (5min), where the lock is retried every 5 seconds (period) up to 60 times (failure threshold)."].
Caller-supplied id so a test that is *about* concurrency can take a lock of its own without
colliding with a neighbouring package's run on the same shared server.

The locker itself is goose's, not a hand-rolled `pg_advisory_lock`: the package ships one, and
locking is simply off until it is configured
[measured 71ce0e4 · go doc github.com/pressly/goose/v3.WithSessionLocker → "If WithSessionLocker is not called, locking is disabled."]
[measured 71ce0e4 · go doc github.com/pressly/goose/v3/lock.NewPostgresSessionLocker → "returns a SessionLocker that utilizes PostgreSQL's exclusive session-level advisory lock mechanism"].
`store.Migrate` builds its provider with a logger option and nothing else today
[measured 71ce0e4:internal/store/migrate.go:35 · sed -n '35p' internal/store/migrate.go → "provider, err := goose.NewProvider(goose.DialectPostgres, db, sub, goose.WithSlog(logger))"].

`HasPendingMigrations` is `Provider.HasPending`, which deliberately ignores a configured locker
[measured 71ce0e4 · go doc github.com/pressly/goose/v3.Provider.HasPending → "this method will not use a SessionLocker or Locker if one is configured. This allows callers to check for pending migrations without blocking or being blocked by other operations."] —
exactly the semantics the auto-apply-off refusal needs.

**Readiness does not re-ask the database whether migrations are pending.** After the migration step
the answer is established by construction: with auto-apply on, `Migrate` applied them; with it off,
`HasPendingMigrations` returned false or the process refused to start. So the step latches
`migrated` and readiness costs one `Pool.Ping` thereafter
[measured 71ce0e4 · go doc github.com/jackc/pgx/v5/pgxpool.Pool.Ping → "Ping acquires a connection from the Pool and executes an empty sql statement against it."].
A goose provider per scrape would be the alternative, and it is the wrong price for an answer that
cannot change while the process runs.

### The migrate-only entry point: a subcommand

`bot migrate`, not a second `cmd/` binary. One binary, one image, one place for the KD-20 import
gate to look, and the infrastructure pass gets `command: [..., "migrate"]` rather than a second
artefact to build and ship. `cmd/testpg` already establishes argv handling in a testable
`run(argv, …)` signature in this module
[measured 71ce0e4:cmd/testpg/run.go:73 · sed -n '73p' cmd/testpg/run.go → "func run(argv []string, lookup envLookup, sm seam, stdout, stderr io.Writer) int {"].
`bot` needs no flags, so the dispatch is a `switch` over `argv`, not a `FlagSet`.

### Restart hygiene lives in `internal/scheduler`

The shift writes `scheduled_task.run_at`, and that table's package is the sole authority over it —
every statement that writes it today is inside that package
[measured 71ce0e4:internal/scheduler · grep -rn 'INSERT INTO scheduled_task\|SET run_at' internal/scheduler/*.go | grep -v _test | sort → "internal/scheduler/reconcile.go:16:\tINSERT INTO scheduled_task (type, instance_key, payload, run_at)", "internal/scheduler/reconcile.go:35:\tUPDATE scheduled_task s SET run_at = $3 FROM candidate c WHERE s.id = c.id", "internal/scheduler/schedule.go:45:\t\t`INSERT INTO scheduled_task (type, instance_key, payload, run_at)", "internal/scheduler/settle.go:179:\t_, err = tx.Exec(ctx, `UPDATE scheduled_task SET run_at = $2 WHERE id = $1`, int64(id), s.Add(ceiling))", "internal/scheduler/settle.go:213:\t\t`UPDATE scheduled_task SET run_at = $2, consecutive_failures = 0, last_error = NULL WHERE id = $1`,"]. The
package's standing clock rule is the same rule the spec fixes for the measurement
[measured 71ce0e4:internal/scheduler/doc.go:9-10,13-14 · sed -n '9,10p;13,14p' internal/scheduler/doc.go → "// Every persisted instant is read from the database, never from", "// time.Now: due-ness, the execution instant and the settlement instant", "// instants the database returned. Go time survives in this package only", "// as intervals — the poll interval, the per-task execution deadline, and"].
So `internal/scheduler` grows one type:

```
type Liveness struct{ … }
func NewLiveness(opts LivenessOptions) (*Liveness, error)   // Pool, Interval, DowntimeThreshold
func (l *Liveness) AbsorbDowntime(ctx) (Downtime, error)    // the start-up step
func (l *Liveness) Refresh(ctx) error                       // one heartbeat write
func (l *Liveness) Run(ctx) error                           // Refresh at Interval until ctx is done or the tolerance is spent
type Downtime struct{ Gap time.Duration; Shifted int; Seeded bool }
```

One type rather than two, because the persisted instant exists for exactly one reason and the shift
is that reason. `AbsorbDowntime` is one transaction whose **first** touch of the liveness row is a
locking one:

```
BEGIN
  SELECT seen_at, now() - seen_at AS gap FROM <liveness> WHERE id = 1 FOR UPDATE
      seen_at IS NULL  ->  never run here: nothing moves, Seeded is true
      gap  > threshold ->  UPDATE scheduled_task SET run_at = run_at + gap
                             WHERE state = 'pending' AND run_at <= now()   -> Shifted
      gap <= threshold ->  nothing moves
  UPDATE <liveness> SET seen_at = now() WHERE id = 1
COMMIT
```

The gap is computed **in SQL**, and the shift's predicate is the one the scheduler's own partial
index is built on
[measured f33b2bf:internal/store/migrations/00002_scheduler.sql:19 · sed -n '19p' internal/store/migrations/00002_scheduler.sql → "CREATE INDEX scheduled_task_due_idx ON scheduled_task (run_at) WHERE state = 'pending';"].
No Go clock value is an input or an output of the decision — `Downtime.Gap` is a value the database
computed and Go only reports.

**Atomicity is not mutual exclusion, and the row lock is what supplies the second.** The premise that
made the migration step take a lock — two processes starting at once — applies here unchanged, and
under Postgres' default READ COMMITTED two concurrent readers would each see the pre-shift instant
and each apply the shift, moving every overdue row by twice the gap. `FOR UPDATE` on the singleton
row removes that: the second transaction waits for the first and then re-reads what the first one
wrote — *"In the case of `SELECT FOR UPDATE` and `SELECT FOR SHARE`, this means it is the updated
version of the row that is locked and returned to the client"*
(https://www.postgresql.org/docs/18/transaction-iso.html § Read Committed). The loser therefore
measures a gap of about zero and moves nothing, which is the correct answer rather than a suppressed
one. An advisory lock is deliberately *not* used here: it is database-wide while this table is
per-schema, so it would serialise unrelated schemas' fixtures on the one shared test server — the
same trap the migration lock is opt-in to avoid.

**A failed `Refresh`, and what bounds the damage.** The two sibling runners tolerate a failed cycle
and keep going
[measured f33b2bf:internal/ingest/loop.go:177 · sed -n '177p' internal/ingest/loop.go → "// returning ctx.Err(). Run does not stop on a PollOnce"]
[measured f33b2bf:internal/scheduler/worker.go:140 · sed -n '140p' internal/scheduler/worker.go → "// picked up on start-up. Run does not stop on a RunOnce error — each"],
and `Liveness.Run` tolerates one too — but not indefinitely, because this failure is silent and its
consequence lands on persisted rows: a heartbeat that died while the process kept serving makes the
*next* start measure a gap that was never downtime and shift every overdue row by it. So the
tolerance is bounded, and bounded by the one quantity that makes the stored instant meaningful:
`Run` counts consecutive failed refreshes and returns the last error once that count reaches
`DowntimeThreshold / Interval` rounded up — the point at which the stored instant has aged past the
threshold and a restart would begin shifting rows. One success resets the count. Under the defaults
below that is a database this process has been unable to write to for about the threshold; it could
not have claimed a task or read an offset either, so stopping is both honest and self-correcting —
the gap the next start measures is then real downtime, and shifting by it is exactly right. `serve`
turns that return into the drain, per the runner-exit rule in § *Shutdown*.

The count is a count and the cadence is the ticker's, so nothing here reads a Go clock and the
liveness source stays free of `time.Now` / `time.Since` / `time.Until` — which is what keeps the
AC14 guard (§ Test Design T3) a whole-file walk.

`Every overdue pending row` is the literal reading the spec fixed, with no narrowing by task class:
the reconcile that follows pulls a pushed recurrence back, and `instance_key` is not a recurrence
marker anyway. The shift writes `scheduled_task` and nothing else — the task basis tables carry
`task_id` by value with deliberately no foreign key and are written by the ledger's own constructors
at posting time
[measured 71ce0e4:internal/store/migrations/00002_scheduler.sql:23 · sed -n '23p' internal/store/migrations/00002_scheduler.sql → "    task_id      bigint,                      -- by value; deliberately NOT a foreign key"].

**The forward migration, and what a deploy window sees.** The liveness surface is a new guarded
singleton table in a new numbered migration, seeded by that migration with its instant **NULL** —
the shape `ingest_offset` already establishes in this package: a one-row table with a primary key, a
singleton `CHECK`, and its row inserted by the migration itself
[measured f33b2bf:internal/store/migrations/00004_ingest.sql:12-18 · sed -n '12,18p' internal/store/migrations/00004_ingest.sql → "CREATE TABLE ingest_offset (", "    id             integer NOT NULL DEFAULT 1,", "    next_update_id bigint  NOT NULL,", "    CONSTRAINT ingest_offset_pkey PRIMARY KEY (id),", "    CONSTRAINT ingest_offset_singleton CHECK (id = 1)", ");", "INSERT INTO ingest_offset (id, next_update_id) VALUES (1, 0);"].
A NULL instant is "this database has never had a live process" — the state the design has to
distinguish, and the reason the column is nullable. The row exists from the migration rather than
from the first start because that is what lets `AbsorbDowntime` take its lock on its first
statement: an empty table has no row to lock, and the serialisation above would then rest on
concurrent-insert behaviour the manual describes as *might* block rather than on the READ COMMITTED
rule it states outright
(https://www.postgresql.org/docs/18/sql-insert.html § Notes). Nothing exists under this name before
the migration, so nothing reads a prior shape and no dual-shape read is needed during a deploy: a
binary from before the migration never queries the table, and one from after it treats a NULL
instant exactly as it treats a database it has not run against — it seeds, and shifts nothing.
Rollback is the project's forward-only migration rule: no migration in this package carries a down
section
[measured f33b2bf:internal/store/migrations · grep -rn 'goose Down' internal/store/migrations/*.sql || echo "no match" → "no match"],
and a mistake here is corrected by the next forward migration, not by reversing this one.

**Telemetry: metrics, no events, no postings.** The shift moves no balance, so it has no basis
document, no posting signature and no ledger participation to declare — it rewrites a `run_at` on
rows that already exist. The issue's telemetry obligation is metrics only, and the restart gauges
named below carry it: a start-up step that silently rewrites scheduler rows is the shape of an incident
nobody can reconstruct afterwards, and the gauges are what make it reconstructable. The absence of
ledger participation is asserted, not assumed — see the basis-and-ledger case in § Test Design T3.

### Readiness and process identity in `internal/health`

`internal/health` is where the metrics listener lives, and the spec fixes readiness onto that same
listener. It grows:

- `type ReadyFunc func(ctx context.Context) error` — nil is ready, an error names why not. A func
  rather than a one-method interface, matching `NewPoolCollector`'s existing accessor-as-func seam
  [measured 71ce0e4:internal/health/pool.go:64 · sed -n '64p' internal/health/pool.go → "func NewPoolCollector(reg prometheus.Registerer, stat func() *pgxpool.Stat) (*PoolCollector, error) {"].
- `NewServer(opts ServerOptions) (*Server, error)` replacing `NewServer(addr, gatherer) *Server`
  [measured 71ce0e4:internal/health/server.go:42 · sed -n '42p' internal/health/server.go → "func NewServer(addr string, gatherer prometheus.Gatherer) *Server"],
  with `{Addr, Gatherer, Ready}` and an `*OptionError`-shaped refusal for a nil field — the shape
  `tg.New`, `ingest.New` and `scheduler.New` already share. A clean break; this module has no
  downstream clients.
- A second path, `/readyz`, beside the existing `/metrics`. Both are contracts with the probe, not
  configuration keys, exactly as `metricsPath` already is. `/metrics` keeps its handler and its
  serving unchanged (AC21); the new families appear in its output because they are on the registry,
  which is content, not a change to how the path is served.
- A `Process` collector: `NewProcess(reg, ProcessOptions{Version, StartedAt, Ready}) (*Process, error)`
  plus `(*Process).ObserveDowntime(gap, shifted)`.

Families, under the module's one prefix
[measured 71ce0e4:internal/health/registry.go:14 · sed -n '14p' internal/health/registry.go → "const namePrefix = \"labgame_\""]:

| Family | Type | Carries |
|---|---|---|
| `labgame_build_info` | gauge, always 1 | the build version, as the `version` **label value** |
| `labgame_start_time_seconds` | gauge | the instant the composition root began |
| `labgame_ready` | gauge func | 1 when `Ready` returns nil, 0 otherwise |
| `labgame_restart_downtime_seconds` | gauge | `Downtime.Gap` |
| `labgame_restart_shifted_tasks` | gauge | `Downtime.Shifted` |

`labgame_start_time_seconds` is exported rather than left to the client library's own
`process_start_time_seconds`, for two reasons: that one is the OS process start read from `/proc`
and is absent on platforms with no procfs, and this one is the instant *this* module began
assembling, which is what an operator correlates with a deploy.

`version` is not in the label ceiling today, so the ceiling gains it
[measured 71ce0e4:internal/health/labels.go:12,20,24 · sed -n '12p;20p;24p' internal/health/labels.go → "	labelMethod  = \"method\"", "	labelState   = \"state\"", "var allowedLabelNames = []string{"].
The readiness gauge is a `GaugeFunc` calling `Ready` with an internally-bounded context, so a scrape
and a probe share one definition and neither can hang a scrape; the bound is a named constant in
`internal/health`, not a configuration key.

`LegsOptions` gains `HTTPClient *http.Client`, threaded into each leg's `TelegramProberOptions`.
The existing `ProberFactory` seam is documented as existing for one assertion and no other reason
[measured 71ce0e4:internal/health/canary.go:55-59 · sed -n '55,59p' internal/health/canary.go → "A test installs a recording factory to assert the exact (Token, BaseURL) pair each leg is built from — the seam exists for that assertion and no other reason."],
so the smoke test does not repurpose it; a field that says what it is costs less than a contract
bent out of shape.

### Configuration: a fifth optional-with-default class

New keys under one new prefix, `LAB_GAME_PROCESS_`, in an `internal/config/process.go` with its own
`processEnvKeys()`, `defaultProcess()` and `loadProcess()`, appended by `EnvKeys()` and never by the
unexported `envKeys()` — the shape every tuning-class reader this package already declares follows
[measured 71ce0e4:internal/config · grep -n '^func load\(Transport\|Scheduler\|Ingest\|Health\)' internal/config/*.go | sort → "internal/config/health.go:108:func loadHealth(lookup Lookup) (*Health, error) {", "internal/config/ingest.go:111:func loadIngest(lookup Lookup) (*Ingest, error) {", "internal/config/scheduler.go:96:func loadScheduler(lookup Lookup) (*Scheduler, error) {", "internal/config/transport.go:155:func loadTransport(lookup Lookup) (*Transport, error) {"]:

| Key | Shape | Default | Why that default |
|---|---|---|---|
| `LAB_GAME_PROCESS_MIGRATE_ON_START` | bool | `true` | the decided policy: apply by default, opt out |
| `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT` | duration | `30s` | equal to the default per-task execution deadline, so a task claimed at the signal gets its own deadline before the budget expires [measured 71ce0e4:.env.example:110 · sed -n '110p' .env.example → "LAB_GAME_SCHEDULER_TASK_TIMEOUT=30s"] |
| `LAB_GAME_PROCESS_LIVENESS_INTERVAL` | duration | `30s` | bounds the recorded instant's staleness at one interval, far below the threshold |
| `LAB_GAME_PROCESS_DOWNTIME_THRESHOLD` | duration | `5m` | above any ordinary restart or deploy, below the power/internet outage §12.1 names as the motivation |

These are operational tuning, not balance numbers, so a compiled-in default is correct here and a
`.env.example` row documents each. A `lookupBool` helper joins the lookup family this
package already declares
[measured 71ce0e4:internal/config/transport.go:220,236,254,270 · grep -n '^func lookup' internal/config/transport.go | sort -t: -k1,1n → "220:func lookupPositiveInt(lookup Lookup, key string) (int, bool, error) {", "236:func lookupPositiveDuration(lookup Lookup, key string) (time.Duration, bool, error) {", "254:func lookupFactor(lookup Lookup, key string) (float64, bool, error) {", "270:func lookupRate(lookup Lookup, key string) (Rate, bool, error) {"].

### The KD-20 gate

`go list -deps` answers the transitive question KD-20 actually asks, and it excludes test-only
imports — verified against the tree in both directions
[measured 71ce0e4 · go list -deps ./cmd/bot | grep testcontainers || echo "no match" → "no match"; go list -deps ./cmd/testpg | grep testcontainers | sort | head -1 → "github.com/testcontainers/testcontainers-go"].
`golangci-lint`'s `depguard` was considered and rejected: it decides a file's own **package imports**
[measured 71ce0e4 · golangci-lint help linters | grep -i '^depguard' → "depguard: Go linter that checks if package imports are in a list of acceptable packages. [fast]"],
so a future `internal/` package that imports the container runtime and is itself imported by
`cmd/bot` passes it. That is a weaker proposition than the invariant, which is about the transitive
graph.

The gate is a small Go command, `cmd/importguard`, run by `make import-guard` — the shape
`make comment-refs` / `cmd/commentrefs` already establishes for a Go-source gate with its own unit
tests
[measured 71ce0e4:Makefile:184-185 · sed -n '184,185p' Makefile → "comment-refs:\n\tgo run ./cmd/commentrefs"].
It holds a rule table (a package, plus the module prefixes its non-test deps may not contain) so the
classifier can be driven red on a synthetic dep list; `cmd/testpg` is deliberately absent from the
table, because provisioning containers is its job. A shell one-liner in the `Makefile` was rejected
for one reason: a gate nobody has seen go red is a claim about the gate.

### Dependencies

- **`golang.org/x/sync/errgroup`** — promoted from indirect to a direct requirement. It is already
  in the module graph through goose
  [measured 71ce0e4 · go mod why -m golang.org/x/sync → "github.com/maratik123/lab-game/internal/store → github.com/pressly/goose/v3 → github.com/pressly/goose/v3/internal/sqlparser → golang.org/x/sync/errgroup"],
  so the promotion adds no module to the build. It expresses "start N named runners, wait for all,
  keep the first error" precisely; the plain `errgroup.Group` is used, never `WithContext`, because
  a runner's terminal error must not tear down its siblings outside the drain — `serve`, not the
  group, decides when the drain begins (§ *Shutdown*).
- **`go.uber.org/goleak`** — a new **test-only** direct requirement, for AC29. Its checksums are
  already recorded and nothing imports it today
  [measured 71ce0e4 · grep goleak go.mod || echo "no match" → "no match"; grep goleak go.sum | sort | head -1 → "go.uber.org/goleak v1.3.0/go.mod h1:CoHD4mav9JJNrW/WLlf7HGZPjdw8EucARQHekz1X6bE="].
  Hand-rolling a goroutine-stack scan was considered and rejected: `AGENTS.md` refuses line count as
  an argument, `goleak.IgnoreCurrent` neutralises the provisioning harness's own background
  goroutines cleanly, and a `_test.go` import never reaches `cmd/bot`'s non-test import graph, so it
  cannot touch KD-20.

### What this task does not touch

`store.Migrate`'s and `store.NewPool`'s behaviour, every gameplay handler, route and task type,
webhooks, balance numbers, the container/compose/systemd layer, and `.env.example`'s placeholder
credentials. The router is wired empty and the registry is wired empty; both packages declare that
legal and neither gets a placeholder to make the wiring look populated.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | The `LAB_GAME_PROCESS_` optional-with-default class: `Process` struct, the keys and defaults § Approach tabulates, `lookupBool`, `EnvKeys()` membership, `Config.Process`, `.env.example` rows (non-empty values), doc-comment updates | `internal/config/process.go`, `internal/config/process_test.go`, `internal/config/transport.go`, `internal/config/env.go`, `internal/config/config.go`, `internal/config/doc.go`, `.env.example` | — |
| 2 | `internal/store`: the forward migration adding the liveness singleton; `MigrateOption` + `WithAdvisoryLock`; `HasPendingMigrations`; update every exact-set schema assertion in the package | `internal/store/migrations/00005_process_liveness.sql`, `internal/store/migrate.go`, `internal/store/migrate_test.go`, `internal/store/schema_test.go`, `internal/store/views_test.go`, `internal/store/fkcover_test.go` | — |
| 3 | `internal/scheduler`: `Liveness` — `NewLiveness`, `AbsorbDowntime`, `Refresh`, `Run`, `Downtime`; the all-SQL gap computation and the overdue shift; the no-Go-clock source guard | `internal/scheduler/liveness.go`, `internal/scheduler/liveness_test.go`, `internal/scheduler/doc.go` | 2 |
| 4 | `internal/scheduler`: `(*Worker).Stop()` — `Run` returns after the cycle in flight, `nil` when stopped and `ctx.Err()` when cancelled | `internal/scheduler/worker.go`, `internal/scheduler/worker_test.go` | — |
| 5 | `internal/ingest`: `(*Loop).Stop()` — same contract, plus cancelling the in-flight `getUpdates` so a long poll does not hold the drain | `internal/ingest/loop.go`, `internal/ingest/loop_test.go` | — |
| 6 | `internal/health`: `ReadyFunc`; `ServerOptions` + `NewServer` returning an error; the `/readyz` path; the `Process` collector and its families; `version` in the label ceiling; `LegsOptions.HTTPClient`; guard-fixture and package-doc updates | `internal/health/server.go`, `internal/health/process.go`, `internal/health/labels.go`, `internal/health/canary.go`, `internal/health/probe.go`, `internal/health/doc.go`, `internal/health/server_test.go`, `internal/health/process_test.go`, `internal/health/guards_test.go`, `internal/health/gather_test.go` | — |
| 7 | `internal/testdb`: `SchemaDSN` — one fresh schema handed to a caller outside this package as a connection string, carrying both the `search_path` that isolates it and the per-test pool cap the ceiling arithmetic assumes, so the consumer's own pool lands inside that arithmetic rather than beside it; the schema's drop stays on the same registered cleanup. `Binaries` is deliberately untouched here — the constant moves in subtask 12, the step that adds the caller | `internal/testdb/testdb.go`, `internal/testdb/testdb_test.go` | — |
| 8 | `cmd/bot`: argv dispatch, usage, the process logger, `version` as a settable variable, the package comment. `run` grows its argv parameter, so the existing `run` cases move to `cmd/bot/run_test.go` against the new signature **in this same step** and `cmd/bot/main_test.go` goes with them — subtask 12 recreates that file holding only `TestMain` | `cmd/bot/main.go`, `cmd/bot/run.go`, `cmd/bot/run_test.go`, `cmd/bot/main_test.go` | 1 |
| 9 | `cmd/bot`: `readiness` (the latches plus the pool round trip) and `assemble` — the whole start-up order, all-fatal, unwinding what it started | `cmd/bot/readiness.go`, `cmd/bot/assemble.go` | 1, 2, 3, 6, 8 |
| 10 | `cmd/bot`: the signal watcher, the `runner` set, `serve` (a signal **or** the first runner return begins the drain), `drain` (budget, second signal, exit codes), the best-effort final liveness write and the pool close; the migrate-only subcommand. `errgroup` becomes a direct requirement here, so `go mod tidy`'s delta on the module files lands in this step | `cmd/bot/serve.go`, `cmd/bot/drain.go`, `cmd/bot/migrate.go`, `go.mod`, `go.sum` | 4, 5, 9 |
| 11 | `cmd/bot` tests: readiness latch, drain under `synctest` with fake runners, argv dispatch, start-up failure paths, the secret-scrape guard, the import-graph discrimination proof | `cmd/bot/readiness_test.go`, `cmd/bot/drain_test.go`, `cmd/bot/run_test.go`, `cmd/bot/guards_test.go` | 10 |
| 12 | `cmd/bot` end-to-end: `TestMain` over the shared test server — which adds this command to the set of `testdb.Main` callers, so `Binaries` is raised to the value the manifest test derives from the tree and the ceiling worked-example comment moves with it **in this same commit**, the manifest test going red→green here rather than sitting red across a group boundary — the in-process smoke test against a fake Bot API with a real `SIGTERM` and a leak check, and the subprocess link-time-version test | `cmd/bot/main_test.go`, `cmd/bot/smoke_test.go`, `cmd/bot/version_test.go`, `internal/testdb/server.go`, `internal/testdb/server_test.go`, `go.mod`, `go.sum` | 7, 10 |
| 13 | `cmd/importguard` and its gate wiring: the rule table, the classifier, its unit tests, `make import-guard`, the `verify` aggregate, the CI Build-job step | `cmd/importguard/main.go`, `cmd/importguard/run.go`, `cmd/importguard/run_test.go`, `Makefile`, `.github/workflows/ci.yml` | — |
| 14 | The lifecycle document (AC2 + AC3): the start-up order with a failure mode per step, and the migration policy in full — the default, the opt-out key, the migrate-only entry point, the lock that serialises concurrent starts, and what happens with auto-apply off and a migration pending — plus restart hygiene, readiness, the shutdown sequence and exit codes; its index row and the plans-index row | `ai-docs/process-lifecycle.md`, `ai-docs/agent-docs-index.md`, `ai-docs/plans/INDEX.md` | — |
| 15 | Propagation, per `AGENTS.md` § *Propagation Rule* step 4 — every live site whose claim this diff falsifies | `AGENTS.md`, `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/claude-tools-hierarchy.md`, `ai-docs/alert-contract.md`, `ai-docs/domain-invariants.md`, `ai-docs/code-style.md`, `ai-docs/go-test-conventions.md`, `.claude/skills/task/SKILL.md`, `.claude/skills/task/reference.md`, `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md` | 13, 14 |

The propagation set in task 15 is the class, not a bound: the swept members are AGENTS.md's
`go run ./cmd/bot` line and its gate list and CI job enumeration; KD-20 (the gate now enforces it,
and the invariant is the **non-test** graph), KD-24, KD-27 (which must *name* the new keys, not
generalise its clause), KD-7 and KD-8 (whose "restarts are trivial" is what the shift makes true);
`context.md`'s architecture, status and gates paragraphs; a new `context-status.md` entry;
`claude-tools-hierarchy.md`'s per-job gate table; `alert-contract.md` § 1, § 3 and the readiness
deferral in § 6 that this task discharges
[measured 71ce0e4:ai-docs/alert-contract.md:377-378 · sed -n '377,378p' ai-docs/alert-contract.md → "- A readiness endpoint beside `/metrics`, if start-up and shutdown\n  orchestration ever wants one."];
`domain-invariants.md`'s "`cmd/bot` constructs no client" sentence; `code-style.md`'s "every gate"
sentence; the `/task` gate checklist and verify list; and the CI-failure skills' class tables. A
new key decision block starts at **KD-32** — KD-31 is the highest today
[measured 71ce0e4:ai-docs/key-decisions.md · grep -o 'KD-[0-9]*' ai-docs/key-decisions.md | sort -t- -k2 -n | tail -1 → "KD-31"].
There is no `README.md` in this repository, so the repo-root user-facing sweep has no member
[measured 71ce0e4 · git ls-files | grep -i readme || echo "no match" → "no match"].

## Handoff plan

Conformance with the every-group handoff contract, stated explicitly: **(a)** `M = 15`, so a
`## Handoff plan` is required and every group below — the first included — is entered through
`/context-reset`. **(b)** every group is `≤ 10` consecutive subtasks. **(c)** the handoff
destination at every boundary, the entry into Group A included, is `/context-reset` per
`.claude/skills/context-reset/SKILL.md` § *Compaction recovery (re-entry)*. **(d)** the terminal
group holds 2 subtasks, inside `1..=10`. **(e)** every group is homogeneous by change-type: A and B
are **code** (`*.go`, `*.sql`, `Makefile`, `.env.example`, `.github/workflows/**` — the artefact
class this repository's own `go` paths-filter groups together), C is **instructions/harness**
(`*.md`, `.claude/**`, `AGENTS.md`, `ai-docs/**`). **(f)** the count is minimised: 13 code subtasks
cannot fit one group under the size cap of 10, so two code groups is the floor, and the single
change-type switch forces the third — no group is an artefact of interleaving. **(g)** each group is
marked with its implementor model and effort below. **(h)** 3 groups, within the default maximum of
4, so no user approval is required.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–7 (code change-type: `*.go`, `*.sql`, `.env.example`). The library side: every
  package `cmd/bot` will assemble, each landing with its own tests and gates green before the
  composition root exists to depend on it.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 8–13 (code change-type: `*.go`, `Makefile`, `.github/workflows/ci.yml`). The
  composition root itself, its tests, and the import gate.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group C with fresh context.
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh), 1M-token window, via `general-purpose` with no inline `model=` — subtasks
  14–15 (instructions/harness change-type: `ai-docs/**`, `AGENTS.md`, `.claude/**`). Terminal group
  (2 subtasks; within the `1..=10` range).

## Risks

- **A drain that abandons in-flight work can still leave a goroutine of this module running, which
  AC29 forbids.** Mitigation: the deadline path cancels the run context and *then* joins the
  runners, and only then closes the pool; the join and the close are the "own final close" AC26
  places outside the bound. The signal watcher is the same trap in miniature — a goroutine parked on
  `<-signals` outlives the process's own shutdown — so it selects on a quit channel too and is
  joined before `serve` returns — [derived → the smoke test's leak check (T12a) and the drain
  cases (T11b)].
- **`contextcheck` rejects a function that takes a `ctx` and then builds one from
  `context.Background()`**, which is exactly what a shutdown budget must do while the run context is
  being cancelled. Mitigation: `serve` keeps the process-level parent and derives both the run
  context and the shutdown context from it, so no `context.Background()` call appears inside a
  ctx-taking function; the linter is enabled and is the decider
  [measured 71ce0e4:.golangci.yml:15 · sed -n '15p' .golangci.yml → "    - contextcheck"].
- **`exhaustive` forces every enum switch total** and `revive`'s `exported` rule forces a doc comment
  on every exported item this design adds
  [measured 71ce0e4:.golangci.yml:18,35,40-48 · sed -n '18p;35p;40,48p' .golangci.yml → "    - exhaustive", "    - revive", "    exhaustive:\n      default-signifies-exhaustive: true", "    revive:\n      rules:\n        - name: exported"].
  Mitigation: no new enum is introduced by this design, and every new exported symbol is named with
  its doc comment in § Approach — [derived → `make lint` in the group's own gate run].
- **A new metric family can fail `promlint`, which the health package gates on a fully-driven
  registry.** Mitigation: the new families use the module's one prefix, base units, no type
  word in the name and no `_total` on a gauge; the package's own promlint guard is extended to drive
  the new collector, so the check runs rather than being reasoned about
  [measured 71ce0e4:internal/health/guards_test.go:508,511-513 · sed -n '508p;511,513p' internal/health/guards_test.go → "func TestGuard_PromlintReportsNoProblem(t *testing.T) {", "	driveEveryAdapterOnce(t, reg)", "", "	problems, err := testutil.GatherAndLint(reg)"].
- **A `version` label breaks the package's closed label ceiling.** Mitigation: `version` joins
  `allowedLabelNames` in the same change; the guard that walks a gathered scrape against that
  ceiling is what decides
  [measured 71ce0e4:internal/health/guards_test.go:361,372 · sed -n '361p;372p' internal/health/guards_test.go → "func TestGuard_LabelNamesAndClosedSetValues(t *testing.T) {", "	for _, n := range allowedLabelNames {"].
- **`cmd/bot` becoming a database-backed test binary silently under-sizes the shared test server.**
  The manifest test re-derives the binary count from the tree and fails by name when the constant
  disagrees
  [measured 71ce0e4:internal/testdb/server.go:198 · sed -n '198p' internal/testdb/server.go → "const Binaries = 4"]
  [measured 71ce0e4:internal/testdb/server_test.go:171-172,175 · sed -n '171,172p;175p' internal/testdb/server_test.go → "// TestBinaries_matchesTree keeps the Binaries constant honest against the", "// tree: a package added or removed from the set of testdb.Main callers must", "func TestBinaries_matchesTree(t *testing.T) {"].
  Mitigation: the raise lands in **subtask 12**, the step that adds the caller, so the constant and
  the tree change in one commit and the manifest test is never left red across a group boundary;
  `testdb.SchemaDSN` embeds the per-pool cap so the composition root's own pool stays inside the
  arithmetic the ceiling assumes.
- **A production-shaped advisory lock taken by an ordinary test fixture would serialise the whole
  suite on one database.** Mitigation: the lock is an opt-in option with a caller-supplied id;
  `store.Migrate`'s default path is unchanged, so every existing fixture keeps its current
  behaviour — [derived → the store tests that call Migrate with no options, and the concurrent-apply
  case (T2c)].
- **A secret can reach a log line or a label the moment the composition root starts logging.**
  Mitigation: the bot token and the DSN stay inside the redacting secret type everywhere they are
  carried, the readiness body carries a fixed reason token rather than the underlying error, and a
  sentinel-secret guard in `cmd/bot` mirrors the one the health package already runs
  [measured 71ce0e4:internal/health/guards_test.go:438,445-447 · sed -n '438p;445,447p' internal/health/guards_test.go → "func TestGuard_ScrapeCarriesNoSentinelSecret(t *testing.T) {", "		sentinelBotToken    = \"1:SENTINEL-BOT-TOKEN-VALUE-----------\"", "		sentinelCloudToken  = \"1:SENTINEL-CLOUD-TOKEN-VALUE---------\"", "		sentinelDSNPassword = \"SENTINEL-DSN-PASSWORD\""].
- **A smoke test that sends the test process a real signal can kill the test binary if the handler
  is not installed yet.** Mitigation: the smoke test sends its signal only after `/readyz` has
  answered 200, which is downstream of the watcher's installation, and it does not call
  `t.Parallel()` — [derived → the smoke test (T12a)].
- **A port chosen by the test can be taken between the choice and the bind, under
  `make test-contention`'s concurrent runs.** Mitigation: the metrics address is configured as
  `127.0.0.1:0` and the bound address is read back from the assembled value, so no port is ever
  guessed — [derived → the smoke test (T12a)].
- **The project holds zero production panics and the index is empty**
  [measured 71ce0e4:ai-docs/panic-index.md · tail -1 ai-docs/panic-index.md → "| — | — | — |"].
  Nothing in this design adds a panicking call: `main` exits non-zero, which needs no row, and every
  new library entry point returns an error — [derived → the panic-gate hook on every non-test `.go`
  write, and `make lint`].

## Test Design

Every claim in this section is about a test that does not exist yet. Section numbers map to the
decomposition's subtask numbers; subtasks 8–10 build the composition root and are covered by T11 and
T12, and subtasks 14–15 write prose the repository's own link, citation and shape gates check.

### T1 — `internal/config` (`internal/config/process_test.go`, beside the code)

- Entry point: `Load`, `loadProcess`.
- Scenarios: every key absent → the documented defaults; each key present and well-formed →
  parsed; `MIGRATE_ON_START` present with a value `strconv.ParseBool` refuses → a `*KeyError` naming
  that variable with `ErrInvalidValue`; each duration present but zero, negative or malformed →
  a `*KeyError` naming it; every one malformed at once → every one reported through `errors.Join`.
- The existing disjointness fixture is what proves the manifest identity: the recording lookup must
  observe every one of them, and `.env.example`'s key set must equal `EnvKeys()`
  — [derived → AC6, and the config package's existing three-way key-set identity assertion].

### T2 — `internal/store` (`internal/store/migrate_test.go`, against a real Postgres)

- Entry points: `Migrate`, `WithAdvisoryLock`, `HasPendingMigrations`.
- Scenarios: (a) the exact base-table set gains the liveness table and no other, and the migration
  leaves it holding its singleton row with a **NULL** instant — the never-run-before state, and the
  row `AbsorbDowntime` locks on its first statement; the singleton `CHECK` refuses a second row; (b)
  `HasPendingMigrations` is true on a fresh schema and false after `Migrate`; (c) two concurrent
  `Migrate` calls under the *same* caller-supplied lock id against the same schema both return nil
  and the migration set applies once — the loser proceeds to a normal return rather than an error;
  (d) `Migrate` with no options behaves exactly as today, which is what keeps every other fixture
  in the module unchanged — [derived → AC7, AC11].
- Fixtures: the package's existing schema-scoped pool helper; case (c) needs two pools on one schema
  and a per-test lock id.

### T3 — `internal/scheduler` liveness and the shift (`internal/scheduler/liveness_test.go`)

- Entry points: `AbsorbDowntime`, `Refresh`, `Run`.
- Scenarios:
  - **no stored instant** → nothing moves, one instant is written, `Downtime.Seeded` is true
    [derived → AC15];
  - **stored instant far in the past** (written by the test as a database-side expression, never a Go
    instant) with three pending rows — two already due, one due in the future — → exactly the two
    overdue rows move, each by the gap the database itself computed, the future row is untouched,
    and `Downtime.Shifted` reports two [derived → AC13];
  - **stored instant inside the threshold** → nothing moves, `Downtime.Shifted` is zero
    [derived → AC16];
  - **a dead row and a future row** are both left alone — the predicate is pending-and-overdue;
  - **run twice** → the second call measures a gap at or below the threshold and moves nothing
    (the first call refreshed the instant);
  - **the task basis tables and the ledger are untouched** — row counts of `deferred_task`,
    `recurrent_task`, `journal_entry` and `posting` are identical either side of a shift that moved
    rows [derived → AC17];
  - **two starts at once** → two `AbsorbDowntime` calls against one schema, released together, with
    a stored instant far in the past and overdue rows present: the rows move **once** and by **one**
    gap, one call reports the shift and the other measures a gap at or below the threshold and moves
    nothing — the serialisation § Approach names, observed rather than reasoned about
    [derived → AC13];
  - `Run` writes the instant once per configured interval — asserted by counting writes across a
    span of several intervals, not by timing one — and returns when its context is done, leaving no
    goroutine [derived → AC12];
  - **a heartbeat that keeps failing** → `Run` against a pool whose liveness write fails returns the
    last error once the derived tolerance is spent and **not before**, and one success between
    failures resets the count — the mid-life policy § Approach fixes, so a dead heartbeat cannot
    ride along silently until the next start reads a stale instant [derived → AC12, AC13].
- **The clock-independence proof (AC14) is structural, because a test cannot make the two clocks
  disagree.** A guard in this package parses the liveness source file and asserts it calls no
  `time.Now` / `time.Since` / `time.Until`, and — to prove the guard discriminates rather than
  passing vacuously — runs the same walk over a scratch file in `t.TempDir()` that does call
  `time.Now` and requires a hit. The package's own clock rule is the proposition; the guard is the
  instrument [derived → AC14].
- Fixtures: a schema-scoped migrated pool; rows inserted with `run_at` expressed relative to `now()`
  in SQL.

### T4 / T5 — the stop seams (`internal/scheduler/worker_test.go`, `internal/ingest/loop_test.go`)

- Entry points: `(*Worker).Stop` + `Run`, `(*Loop).Stop` + `Run`.
- Scenarios, for each: `Stop` before `Run` → `Run` returns `nil` without starting a cycle; `Stop`
  during the inter-cycle wait → `Run` returns `nil` promptly, well inside the poll interval; `Stop`
  called twice → no panic, no double close; context cancelled instead of stopped → `Run` still
  returns `ctx.Err()`, the existing contract.
- Ingest only: `Stop` while a `getUpdates` is parked in the fake server's long poll → the call
  returns and `Run` returns without waiting out the long-poll window; the offset row is unchanged
  by the discarded poll; the discarded cycle's `LoopObservation` carries no `Err`, so a graceful
  shutdown mints no poll error; the per-cycle cancellation helper leaves no goroutine behind.
- Ingest only, the other half of the two-context split: **`Stop` while a batch is mid-`processUpdate`**
  → every update in that batch settles before `Run` returns — each one's offset advance is visible
  afterwards — and `Run` returns `nil`; `Stop` never cancels the parent context, which is the whole
  point of scoping the cancel to `getUpdates`. Fixture: a handler that blocks until the test releases
  it, so `Stop` lands provably inside the batch [derived → AC25].
- Fixtures: the ingest suite's existing in-process fake Bot API server and its delayed handler; the
  scheduler suite's existing schema-scoped pool. Timing assertions are patience budgets, generous by
  construction — the asserted proposition is "returns without waiting out the window", so the budget
  is set well below the window and well above any scheduling slop [derived → AC25].

### T6 — `internal/health` (`internal/health/process_test.go`, `server_test.go`, `guards_test.go`)

- Entry points: `NewServer`, `NewProcess`, the `/readyz` handler, `NewLegs`.
- Scenarios:
  - `NewServer` refuses a nil gatherer and a nil `Ready` with an error naming the field; a valid one
    binds, serves `/readyz`, and 404s everything else. `/metrics` is asserted as **unchanged
    serving**, never as a byte-for-byte body: the same handler on the same path with the same
    content type, and every family the registry carried before this task still present under the
    same names and label sets. A golden of the body would be falsified by AC23's own additive
    families, so the assertion is about the path and the surviving families
    [derived → AC21, AC23];
  - `/readyz` answers 200 with a fixed body when `Ready` returns nil and 503 with a fixed reason
    token when it returns an error, and the error's own text never reaches the body [derived → AC22, AC36];
  - `NewProcess` registers the families § Approach names; `labgame_build_info` carries the version as a label
    value and no version substring appears in any metric *name* [derived → AC23];
  - `labgame_ready` follows the injected `ReadyFunc` in both directions across two gathers;
  - `ObserveDowntime` sets the restart gauges [derived → AC18];
  - `NewLegs` with an `HTTPClient` builds both legs against it, and the existing pairing assertion
    is unchanged.
- The package's existing guards are extended rather than duplicated: the promlint guard and the label
  guard both drive the new collector, and the sentinel-secret guard gathers with it registered.
- Fixtures: an isolated registry per case, the in-process fake Bot API server, the client-library
  test utilities the package already uses.

### T7 — `internal/testdb` (`internal/testdb/testdb_test.go`, `server_test.go`)

- Entry point: `SchemaDSN`.
- Scenarios: the returned string parses; its runtime parameters name the fresh schema; its pool cap
  equals the per-test cap the ceiling arithmetic assumes; a pool opened from it lands in that schema
  and the registered cleanup drops it [derived → the shape subtask 12's fixture consumes]. The
  binary-count manifest constant is *not* touched in this subtask — it moves with its caller, in
  subtask 12.

### T11 — `cmd/bot` unit level (`readiness_test.go`, `drain_test.go`, `run_test.go`, `guards_test.go`)

- **T11a — readiness.** Entry point: the readiness value's check method. Scenarios: neither latch set
  → not ready with the migrating reason; migrated, not draining, pool answers → ready; migrated, pool
  unreachable → not ready with the database reason; draining → not ready with the draining reason,
  *even when* the pool answers and even after migration; the draining latch is one-way
  [derived → AC22].
- **T11b — drain.** Entry point: the drain function, driven with fake runners inside a
  `testing/synctest` bubble so the budget is virtual and exact. Scenarios: every runner returns
  promptly on `stop` → exit code zero and no cancellation of the run context [derived → AC28]; one
  runner ignores `stop` and returns only on cancellation → the budget expires, the run context is
  cancelled, the runners are joined, the exit code is non-zero [derived → AC26, AC28]; a second
  signal arrives mid-drain → the wait ends at once, well inside the budget, exit code non-zero
  [derived → AC27]; a fake runner returns on its own with **no** signal sent → the same drain runs
  in the same order, that runner's name and its error reach stderr, and the exit code is non-zero
  [derived → the runner-exit rule § Approach → *Shutdown* fixes, under AC26's budget]; the shutdown
  order is asserted by recording each fake's stop and each subsystem shutdown into one ordered log — runners stopped, runners joined, canary, listener, final
  liveness write, pool [derived → AC25]; a stopped runner returning an error is reported with its
  own name. Bubble rules: `synctest.Test` sits inside each subtest and `t.Parallel()` outside it.
- **T11c — dispatch and start-up failures.** Entry point: `run(argv, lookup, stderr, stdout)`.
  Scenarios: an unknown subcommand → usage on stderr, the usage exit code, stdout empty; `-h` →
  usage, exit zero; a missing required configuration key → non-zero, stderr names the key and the
  configuration step, stdout empty [derived → AC4]; an unreachable DSN → non-zero, stderr names
  the database step and the cause [derived → AC4, AC19]; the example file's placeholder bot token →
  non-zero, stderr names the Telegram-client step [derived → AC19, AC35]; auto-apply disabled with
  a migration pending → non-zero, stderr names both the disabling key and the pending state
  [derived → AC8]; auto-apply disabled with nothing pending → a normal start [derived → AC9];
  a failure at a step after the listener bound → the process exits non-zero **and** the listener's
  port is free again, which is how "the started subsystems were shut down" is observed
  [derived → AC20].
- **T11d — guards.** The composition-root shape, which is AC1's negative half and the one claim
  § Test Design otherwise assigned nowhere: a source walk over the non-test `.go` files under `cmd/`
  and `internal/` asserts that none declares a `func init()` — none does today
  [measured f33b2bf · grep -rn '^func init()' --include='*.go' cmd internal | grep -v _test || echo "no match" → "no match"] —
  and, so the walk is an instrument rather than a tautology, the same walk over a scratch file in
  `t.TempDir()` that *does* declare one must report it, the shape the AC14 clock guard uses. A second
  assertion, scoped to this package because this is where the risk concentrates: `cmd/bot` declares
  no package-level `var` beyond the link-time `version` string. The AC's remaining clause — that no
  package holds a package-level **mutable subsystem instance** — is **review-judged, and stated here
  as such**: deciding whether a package-level value *is* a subsystem instance needs types and
  judgment, while the property this task actually installs (every subsystem constructed in
  `assemble` and handed to its user) is what T12a observes end to end [derived → AC1].
  A sentinel-secret scrape-and-log guard: assemble with a sentinel bot token and a sentinel DSN
  password, drive one scrape and capture the process log writer, assert neither sentinel appears in
  either [derived → AC36]. An import-graph discrimination proof: the non-test
  dependency list of this command carries no container-runtime path, **and** its test dependency list
  does — the second half is what makes the first half evidence rather than a tautology
  [derived → AC33].
- **T11e — migrate-only.** Entry point: the migrate subcommand. Scenarios: it applies pending
  migrations under the lock and exits zero; a second invocation is a no-op and exits zero; no
  listener is bound (the configured metrics port stays free), no Bot API request reaches the fake
  server, and no scheduler row is seeded — the observable consequences of "constructs no client, no
  loop, no worker, no listener and no canary" [derived → AC10].

### T12 — `cmd/bot` end to end

- **T12a — the in-process smoke condition.** `TestMain` provisions the shared server — which is
  what puts this command in the set of `testdb.Main` callers, so the raised `Binaries` constant and
  this fixture land in one commit, with the manifest test going red→green there
  [derived → the manifest test named in § Risks]. The fixture takes a fresh schema DSN, starts the
  in-process fake Bot API server, and assembles the process with its HTTP client, `LAB_GAME_HEALTH_METRICS_ADDR=127.0.0.1:0` and an empty cloud-canary token.
  Assertions, in order: `assemble` returns with the schema migrated and **no** request yet at the
  fake Bot API and **no** scheduler row seeded — migrations precede the loop and the worker
  [derived → AC5]; `/readyz` answers 200 [derived → AC22]; a `/metrics` scrape carries a
  Go-runtime family, a process-collector family, a pool family and every process-identity family
  [derived → AC32]; the `getUpdates` the fake server received carries the reserved sentinel the
  ingest package substitutes for an empty route set, and `scheduled_task` holds no row after a
  reconcile — the empty router and the empty registry, observed rather than asserted about the
  constructor [derived → AC31]; a send to a chat outside the allowlist is refused with the ingest
  package's refusal sentinel in the error chain [derived → AC30]; a real `SIGTERM` sent to the test
  process begins the drain, `/readyz` answers 503 while it drains, and the process returns exit code
  zero within the budget [derived → AC25, AC27, AC28]; a leak check taken against a snapshot from
  before assembly reports nothing left running [derived → AC29]. Not parallel, and the leak check
  is the last assertion.
- **T12b — the link-time version.** Build this command into a temporary directory with
  `-ldflags "-X main.version=<sentinel>"`, run it with one required key deliberately absent, and
  assert: a non-zero exit, the sentinel in the reported build identity on stderr, the failing step
  named, and stdout empty. One subprocess, no ports, no database — it exists for the one proposition
  an in-process test cannot reach [derived → AC24, AC4].

### T13 — `cmd/importguard` (`cmd/importguard/run_test.go`)

- Entry point: the classifier and the command's own run function.
- Scenarios: a synthetic dependency list containing the container-runtime module against a rule that
  forbids it → a violation naming the package and the module — the red case, proved without touching
  the tree; the same list against a rule that does not forbid it → clean; an empty list → clean; the
  real tree → clean, and a rule whose package does not exist → an error rather than a silent pass,
  because a gate that quietly examines nothing is the failure mode this whole gate exists to prevent
  [derived → AC33, AC34].

## Open questions

- **The readiness consumer** — a container healthcheck, an orchestrator probe or a human. The
  infrastructure pass settles it; this task fixes the path, the meaning and the starting/serving/
  draining states, and the listen address is already a configuration key, so accommodating the
  consumer reopens nothing here. Looked for in the design corpus's own §12 and in the alert
  contract's deferral list, and named in neither
  [measured 71ce0e4:docs/DESIGN.md § 12 · sed -n '/^## 12\./,/^## 13\./p' docs/DESIGN.md | grep -niE 'readiness|healthcheck|probe' || echo "no match" → "no match"]
  [measured 71ce0e4:ai-docs/alert-contract.md:377-378 · sed -n '377,378p' ai-docs/alert-contract.md → "- A readiness endpoint beside `/metrics`, if start-up and shutdown", "  orchestration ever wants one."].
- **Whether the shutdown budget's default of 30s survives contact with the deployment.** It is
  chosen against the default per-task execution deadline, not against a supervisor's stop timeout,
  because no supervisor exists yet; the container/systemd pass owns the other side of that pair and
  may want them changed together. The key exists, so changing it needs no code.
- **Whether the liveness instant grows a second consumer.** Today only the shift reads it. Nothing
  in the shape prevents an uptime or last-seen surface reading the same row, and nothing in it
  anticipates one.
