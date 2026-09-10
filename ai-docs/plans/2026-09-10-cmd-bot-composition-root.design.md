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
  subsystem, the registered signal channel, a `[]runner` and a `[]closer`.
- **`runner`** — `{name string; run func(ctx) error; stop func()}`: the ingest loop, the scheduler
  worker, the liveness heartbeat. The name is what a failure message and a drain
  timeout report, so "naming the subsystem that failed" is a property of the data structure rather
  than of a hand-written string at each call site.
- **`closer`** — `{name string; close func(ctx) error}`: **the shutdown seam, and the one place the
  shutdown order is written down.** `assemble` appends an entry the moment a step in the order below
  takes something that has to be given back — the signal registration, the pool, the health
  listener, the liveness row's final write, the canary — and nothing else in this design writes that
  list. **Decision: an entry is appended only after the call that acquires its resource has
  returned nil, never before it.** For the pool that call is the constructor, so a failed `Ping`
  still unwinds the pool the constructor handed back; for the health listener and the canary it is
  `Start`, which is what binds the listener and what begins the canary's ticking — the
  constructors produce neither
  [measured f6d0c28 · go doc ./internal/health Server.Start → "Start binds the listener synchronously, returning a bind error before spawning the serve goroutine"; go doc ./internal/health Canary.Start → "Start begins ticking: the first probe fires immediately, and one more fires per Interval thereafter, until Shutdown."].
  So a failure *at* step 7 or step 13 does not unwind that step's own resource, and the table's
  last column says exactly that. The alternative — appending before `Start` and suppressing the
  close error an unstarted value returns — is rejected: both entry points refuse an unstarted
  value with an error rather than panicking
  [measured f6d0c28:internal/health/server.go:109 · sed -n '109p' internal/health/server.go → "		return errors.New(\"health: server not started\")"]
  [measured f6d0c28:internal/health/canary.go:248 · sed -n '248p' internal/health/canary.go → "		return errors.New(\"health: canary not started\")"],
  so that shape would print a manufactured `not started` beside the real bind error under the
  "a `close` that returns an error is reported by name" rule below, and suppressing it would mean
  matching another package's error text. The unwind of a failed start-up (AC20) and the drain's
  ordered stop (AC25) are then the same
  list walked backwards rather than two hand-maintained parallel orders: the start-up table's last
  column becomes a consequence of the data structure instead of a claim beside it, and `serve`'s
  ordered shutdown is drivable by a test that hands an `app` fake closers (§ Test Design T10a).
  `close(ctx)` is the shape the subsystems' own shutdown entry points already have
  [measured a1b0a81:internal/health · go doc ./internal/health Server → "func (s *Server) Shutdown(ctx context.Context) error"; go doc ./internal/health Canary → "func (c *Canary) Shutdown(ctx context.Context) error"],
  which is what lets AC20's "through their own shutdown entry points" be satisfied by the list rather
  than by a wrapper per subsystem; the two that have no such method — `signal.Stop` and the pool's
  `Close` — are the entries whose `close` ignores its context and returns nil. The context each
  entry receives is the drain's bounded one, so no close can outlast the budget, and a `close` that
  returns an error is reported by name.
- **`readiness`** — the `migrated` and `draining` latches plus the pool. It is the *one* definition
  AC22 asks for, consumed by both the HTTP path and the metrics gauge. Each latch has exactly one
  writer and is one-way: `migrated` is set by start-up step 8, `draining` by `drain`'s **first**
  action, ahead of every stop and every close (§ *Shutdown*).

Entry points:

```
main()  ->  os.Exit(run(os.Args[1:], os.LookupEnv, os.Stderr, os.Stdout))
run()   ->  argv dispatch:  (no args) -> serve ;  "migrate" -> migrate-only ;  -h/--help -> usage
```

`main` stays a one-line `os.Exit(run(...))` and `run` never terminates the process itself — the
existing contract, kept
[measured 71ce0e4:cmd/bot/main.go:20,27-28 · sed -n '20p;27,28p' cmd/bot/main.go → "	os.Exit(run(os.LookupEnv, os.Stderr, os.Stdout))", "// stdout. run never terminates the process itself: main exiting non-zero", "// is not a panic."].

**The build identity moves to stderr, and stdout is left to the usage text alone.** `run` writes the
identity line as its **first** action — before argv dispatch and before configuration — so every
invocation says which binary it is, including one that dies at the configuration step with nothing
else to go on.
Today that line goes to stdout
[measured 9462d7e:cmd/bot/main.go:37 · sed -n '37p' cmd/bot/main.go → "	if _, err := fmt.Fprintf(stdout, \"lab-game bot %s\\n\", version); err != nil {"],
which made sense while printing it *was* the whole success path; it stops making sense once the
success path is a process that serves until a signal. It is written straight to the stderr writer
`run` was handed, not through the process logger — the logger is step 3 of the start-up order below
and does not exist yet at the moment the line is written, which is exactly why the line can survive a
configuration failure. Three consequences, all deliberate: the
identity is diagnostic context, so it belongs on the stream the logger and every failure line
already use (AC4); an explicit `-h`/`--help` is the one path that writes to stdout, and it writes
the usage text and nothing else; every other path — serving, migrate-only, and every failure —
leaves stdout empty, which is what makes AC4's "stdout carries no error text" observable as
"stdout is empty" rather than as a substring search. This is a clean break in `run`'s documented
contract, which this module's no-downstream-clients rule permits, and it is why the existing
`run` case asserting the identity **on stdout** is rewritten in the same subtask that flips the
stream rather than left to rot.

### Start-up: one written order, all-fatal, unwinding

The table below **is** the order. Subtask 9 implements it and subtask 14 writes it into a new page,
`ai-docs/process-lifecycle.md` — the artefact AC2 and AC3 require — so it is fixed here once instead
of being derived twice, in two groups, from two constraints. One row per step: what it constructs or
starts, how it fails, the step name its stderr line carries (AC4), and what the unwind stops on the
way out (AC20).

| # | Step | Constructs / starts | Failure mode | stderr step name | A failure here unwinds |
|---|---|---|---|---|---|
| 1 | signal registration | the buffered `os.Signal` channel, and `signal.Notify` over it for `SIGINT` and `SIGTERM`; the matching `signal.Stop` is appended to the closer list as its first entry | none to report: `Notify` reads no configuration and returns no error, and the background goroutine it starts is the stdlib's own | — | itself — it is the entry every later row's unwind ends on |
| 2 | configuration | `config.Load(lookup)` | a missing or malformed key, reported as the joined per-key error the package already returns | `configuration` | the signal registration |
| 3 | logger | the process `slog.Logger` over stderr | none: no key is read and no I/O is done at construction | — | — |
| 4 | database | `pgxpool.ParseConfig` over the DSN, `store.NewPool`, then one `Ping` bounded by `pingTimeout` — a named constant in `cmd/bot`, **5s**, not a configuration key: a database that has not answered a trivial round trip in that long is not one this process can serve against, and the all-fatal rule wants the failure now rather than after an unbounded hang; retrying a cold host is the supervisor's job, which the spec defers | an unparsable DSN; a database that does not answer inside `pingTimeout` | `database` | the pool, the signal registration |
| 5 | readiness | the `readiness` value: both latches unset, over the pool step 4 built. It is constructed **before** the registry because `NewProcess` takes the `Ready` func the `labgame_ready` gauge calls, and that func is this value's method | none | — | — |
| 6 | metrics registry | `health.NewRegistry`, `RegisterRuntime`, the transport, scheduler and ingest observers, `NewPoolCollector`, `NewProcess` over the readiness value step 5 built | a family the registry refuses | `metrics registry` | the pool, the signal registration |
| 7 | health listener | `health.NewServer(ServerOptions{Addr, Gatherer, Ready})`, then `Start` | a nil option field; an address already in use — `Start` binds synchronously and returns the bind error rather than losing it on a goroutine [measured f33b2bf:internal/health/server.go:54-57 · sed -n '54,57p' internal/health/server.go → "// Start binds the listener synchronously, returning a bind error before", "// spawning the serve goroutine — so a bind failure is always reported", "// through a returned error, never discovered later on a background", "// goroutine. Returns an error rather than panicking on a second call."] | `health listener` | the pool, the signal registration |
| 8 | migrations | with auto-apply on, `store.Migrate` under `WithAdvisoryLock`; with it off, `HasPendingMigrations` plus a refusal naming the disabling key when anything is pending; then the `migrated` latch | a migration that fails to apply; auto-apply off with a migration pending (AC8) | `migrations` | the listener, the pool, the signal registration |
| 9 | restart hygiene | `scheduler.NewLiveness`, `AbsorbDowntime`, and `Process.ObserveDowntime` over what it returns | the transaction fails | `restart hygiene` | the listener, the pool, the signal registration |
| 10 | scheduler | `scheduler.New` over the empty registry, then `Reconcile` | an option the constructor refuses; a reconcile that fails | `scheduler` | the final liveness write, the listener, the pool, the signal registration |
| 11 | telegram client | `ingest.NewPoolGate(cfg.AllowedChatIDs, pool)` — the package's own documented wiring shape, taking the pool step 4 built, which is why the gate is constructed here and not before it [measured ad410c3:internal/ingest/gate.go:75-77 · sed -n '75,77p' internal/ingest/gate.go → "// NewPoolGate builds a Gate whose PlayerLookup reads pool directly — the", "// wiring shape this package uses: gate, then client, then loop.", "func NewPoolGate(allowedChatIDs []int64, pool *pgxpool.Pool) *Gate {"] — then `tg.New` over the token, the base URL, that gate, the transport tuning and the transport observer. `NewGate` with a hand-supplied lookup is **not** used: its `Gate` dereferences the lookup on the first non-allowlisted destination with no nil guard, so a nil there is a production panic against a zero-row panic index [measured ad410c3:internal/ingest/gate.go:119 · sed -n '119p' internal/ingest/gate.go → "	exists, err := g.lookup.PlayerExists(ctx, id)"] | an option the constructor refuses — including a token telego's own format check rejects, which is what the example file's placeholder is [measured f33b2bf:.env.example:19 · sed -n '19p' .env.example → "LAB_GAME_BOT_TOKEN=changeme"; go doc github.com/mymmrac/telego.ErrInvalidToken → "ErrInvalidToken bot token is invalid according to token regexp"] | `telegram client` | the final liveness write, the listener, the pool, the signal registration |
| 12 | ingest loop | `ingest.New` over the client, the pool, the empty router and the ingest observer | an option the constructor refuses | `ingest loop` | the final liveness write, the listener, the pool, the signal registration |
| 13 | canary | `health.NewLegs` with the process HTTP client, `NewCanary`, then `Start` | a leg option the constructor refuses; a canary already started | `canary` | the final liveness write, the listener, the pool, the signal registration |
| 14 | runners | the `[]runner` values — ingest loop, scheduler worker, liveness heartbeat — constructed here, started by `serve` | none | — | — |

Three of those orderings are load-bearing rather than tidy:

1. **The signal registration is the first step, ahead of everything observable** — step 1, before
   the listener binds at step 7 and before the `migrated` latch at step 8. Until `signal.Notify`
   runs, `SIGINT` and `SIGTERM` carry their default disposition and kill the process outright, so
   every step after the registration would otherwise be a window in which a `SIGTERM` is a sudden
   death with nothing on stderr — and the windows are not small: the migration step can wait on the
   production advisory lock for the retry window § *Migration policy* measures, and restart hygiene
   is a transaction. Registering first closes the window for the whole order: a signal arriving at
   any later step is buffered rather than fatal, and `serve` finds it on its first select and begins
   the drain as soon as there is something to drain — or, if that step is the one that fails, the
   process is already exiting non-zero through the unwind, which is the outcome the signal asked
   for and reaches it by the path AC20 specifies. The channel is buffered for exactly the two
   signals that decide an outcome — the one that begins the drain and the second one that ends the
   wait (AC27) — because the package explicitly will not block delivering to it
   [measured a1b0a81 · go doc os/signal.Notify → "Package signal will not block sending to c: the caller must ensure that c has sufficient buffer space to keep up with the expected signal rate."],
   and a third signal changes nothing those two have not already settled. **Assembly itself is not
   interruptible, and that is deliberate:** cancelling a half-applied migration or a half-run
   `AbsorbDowntime` buys nothing their transactions' rollback does not already give, while a
   start-up that hangs past the supervisor's grace period is the supervisor's `SIGKILL` to end —
   the same division of labour that puts retrying a cold database outside this process. What the
   registration is *for* at that stage is the opposite of interrupting: not losing the signal, and
   not dying by default disposition without a diagnostic. No goroutine is parked on the channel
   either; `serve` and `drain` select on it directly, so there is no watcher to outlive the process
   (AC29), and the matching `signal.Stop` is the closer list's first entry, which makes it the last
   thing every exit path — a failed step's unwind and a completed drain alike — performs.
2. **The health listener binds before the migration step** — step 7 before step 8, not after.
   Readiness is only observable as a *signal* while the listener is up; binding after migrations
   would make "starting" mean
   "connection refused" rather than 503, and AC22 asks a probe to distinguish starting, serving and
   draining. It also answers the spec's open question about scraping while not ready: the endpoint
   serves throughout.
3. **Restart hygiene runs after migrations and before `Reconcile`** — step 9 before step 10 — and
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

- **`assemble(ctx, opts) (*app, error)`** — register the signal channel, construct everything, and
  start the subsystems whose start can fail or has an external effect (the health listener, then the
  canary), appending a `closer` once a step has taken something that must be given back. On any
  failure it walks the closers it has appended so far **backwards**, then returns the error
  naming the step. That is
  AC20, and it is the closer list that implements it — one traversal, not a shutdown call per
  failure site, which is why the table's last column can be read off the list rather than kept in
  step with it by hand. The one entry whose close is not a release is the liveness row's final
  write: on an unwind it re-writes an instant `AbsorbDowntime` set moments earlier, which is a no-op
  in effect, and the alternative — a second, unwind-only list — would reintroduce exactly the
  parallel ordering this seam removes.
- **`(*app).serve(ctx) int`** — start the runners, wait on `app`'s registered signal channel or the
  first runner return, drain — which begins by latching `readiness.draining` — then walk the
  closers backwards. The channel is a field of `app` rather than a parameter, because it is the
  registration `assemble` made and the closer list already owns; a test drives the same field
  (§ Test Design T10a).

Every failure at either phase is fatal: a message on stderr naming the step and the cause, a
non-zero exit, nothing on stdout. The canaries are included with no exception, which is the owner's
round-1 answer.

### Shutdown: a stop seam rather than a context cancel

AC25 asks for polling to stop while in-flight work keeps its budget. `Loop.Run` and `Worker.Run`
today have exactly one lever — the context — and cancelling it stops the cycle *and* aborts the work
inside it
[measured 71ce0e4:internal/ingest/loop.go:183,189-190,194 · sed -n '183p;189,190p;194p' internal/ingest/loop.go → "func (l *Loop) Run(ctx context.Context) error {", "		case <-ctx.Done():", "			return ctx.Err()", "		_ = l.PollOnce(ctx)"]
[measured 71ce0e4:internal/scheduler/worker.go:154-155,159 · sed -n '154,155p;159p' internal/scheduler/worker.go → "		case <-ctx.Done():", "			return ctx.Err()", "		_ = w.RunOnce(ctx)"].
So **every runner in the set carries a second lever — the two existing ones grow it, the new one is
born with it, and all three share one contract**:

- `(*Loop).Stop()` — `Run` returns after the cycle in flight, **and** the in-flight `getUpdates` is
  cancelled. Without the second half every graceful shutdown would wait out the long-poll window,
  whose default is 25s
  [measured 71ce0e4:.env.example:122 · sed -n '122p' .env.example → "LAB_GAME_INGEST_LONG_POLL_TIMEOUT=25s"];
  a poll that returns nothing is worth nothing, so cancelling it discards nothing. The offset is a
  guarded, monotone database row, so a discarded poll simply re-arrives.
- `(*Worker).Stop()` — `Run` returns after the cycle in flight; the claimed batch finishes. No
  poll-cancel counterpart, because a scheduler cycle parks in no long window.
- `(*Liveness).Stop()` — `Run` returns after the heartbeat in flight; no poll-cancel counterpart
  either, because a heartbeat is one bounded write.

All three are a `sync.Once`-guarded channel closed once and selected on beside `ctx.Done()`; `Run`
returns `nil` when stopped and `ctx.Err()` when cancelled, which is what lets the drain tell a
completed shutdown from an abandoned one (AC28).

**Decision: the liveness heartbeat gets the same `Stop()` as its two siblings, rather than a
composition-root wrapper that cancels a per-runner context.** The wrapper was the alternative — it
would leave `internal/scheduler`'s `Liveness` API untouched and let `serve` synthesise a `stop` for
it — and it is rejected because it makes a *stopped* runner return `context.Canceled`, which is the
value the drain reads to mean **abandoned**. `serve` would then need a second rule mapping one
runner's `context.Canceled` to a zero exit while its siblings' means non-zero, and AC28's whole
distinction would rest on a per-runner exception rather than on the `nil`-vs-`ctx.Err()` contract.
Three consequences of the chosen form, all load-bearing: `runner.stop` is total, so `serve` has no
per-runner special case; a clean `SIGTERM` returns every runner promptly and `errgroup.Wait` joins
inside the budget, so the drain never runs to `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT` on the ordinary
path and AC28's zero exit is reachable in the assembled process; and the stop contract is one
contract with one scenario list, exercised identically for all three runners (§ Test Design T4 and
T5/T6).

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
its opening `Reconcile` fails, and the liveness heartbeat returns when its failure tolerance is
spent (§ *Restart hygiene* below), which is its one *unprompted* exit and is distinct from the
`nil` a `Stop` produces. `serve` therefore waits on two things, not one: a signal, and the first
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

**The drain has one written order, it begins with the readiness latch, and the rest of it is the
closer list backwards.** `drain`'s **first** action — before any `runner.stop`, and so before
anything else in the sequence — is setting `readiness.draining`. The placement is load-bearing
rather than tidy: the health listener is itself a closer, so it keeps serving throughout the
drain, and a latch set anywhere after the runner join would leave `/readyz` answering 200 while
polling had already stopped. AC22's "not ready from the moment shutdown begins" is therefore a
property of *where* the latch is set rather than of the readiness definition alone, and § Test
Design T10a's ordered log asserts it in that position. `drain` then calls each
`runner.stop`, joins the group (cancelling the run context first if the budget expired or a second
signal arrived), and only then walks `app.closers` from the last appended to the first: the canary,
the liveness row's final write, the health listener, the pool, the signal deregistration. That is
AC25 — polling and claiming stop, in-flight work keeps the remaining budget, the canary and the
listener stop, the pool closes after them — read off the order the steps were registered in rather
than restated here as a second list that could disagree with the table above. The final liveness
write is the one best-effort entry: a failure there is reported and does not change the exit code,
because the next start then measures its gap from the last successful heartbeat, which is one
interval old at worst and far below the downtime threshold. Every other closer's error is reported
by its name and, like it, leaves the exit code to AC28's completed-versus-abandoned distinction —
the process is already leaving, and a close that failed is a diagnostic, not a second verdict.

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

**The production id is one exported constant, `store.ProcessLockID`, and its value is goose's own
`lock.DefaultLockID`.** Both production callers — the start-up migration step and the `bot migrate`
subcommand — pass that constant and nothing else, which is what makes AC7's "two processes starting
concurrently never both apply" hold across the *pair* of entry points rather than only within one of
them. Taking goose's published id rather than minting a fresh number is deliberate: it is the id the
library documents for exactly this purpose
[measured 9462d7e · go doc github.com/pressly/goose/v3/lock.DefaultLockID → "DefaultLockID is the id used to lock the database for migrations. It is a crc64 hash of the string \"goose\". This is used to ensure that the lock is unique to goose."],
so any other goose-driven tooling ever pointed at the same database contends on the same lock
instead of applying migrations beside us under a private one. The constant is declared in
`internal/store` beside the option that consumes it, so `cmd/bot` names a storage concept rather
than reaching into goose's package for a number.

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

**Decision: `bot migrate` does not register the signals, and the default disposition is the right
answer for a one-shot command.** Step 1 of the serving order registers them because a `SIGTERM`
arriving mid-assembly must be *buffered* for a `serve` that will read it; this path has no
`serve`, no runner set and nothing to drain, so a registration here would have no reader — and a
registration with no reader does not delay a signal, it swallows it: `Notify` disables the
default behaviour and delivers to the channel instead
[measured f6d0c28 · go doc os/signal → "Notify disables the default behavior for a given set of asynchronous signals and instead delivers them over one or more registered channels."],
and the delivery never blocks
[measured f6d0c28 · go doc os/signal.Notify → "Package signal will not block sending to c: the caller must ensure that c has sufficient buffer space to keep up with the expected signal rate."],
so an operator's `Ctrl-C` would become a no-op for as long as the lock wait runs and the operator
would escalate to the `SIGKILL` that the default disposition delivers immediately and honestly.
The lock wait is the same window the serving path's step 8 can sit in — this command passes the
same `store.ProcessLockID`, over goose's retry window § *Migration policy* measures — so the
window is real; what differs is that dying inside it costs nothing here. Every migration in this
package runs inside a transaction, goose's default, because none carries the annotation that opts
out
[measured f6d0c28 · grep -rn 'NO TRANSACTION' internal/store/migrations/*.sql || echo "no match" → "no match"]
[measured f6d0c28 · sed -n '298p' "$(go env GOMODCACHE)"/github.com/pressly/goose/v3@v3.27.3/README.md → "By default, all migrations are run within a transaction. Some statements like `CREATE DATABASE`,"],
so a killed process rolls back the migration in flight and leaves the applied set consistent; the
lock is session-level and dies with the connection
[measured f6d0c28 · go doc github.com/pressly/goose/v3/lock.SessionLocker → "SessionLocker is the interface to lock and unlock the database for the duration of a session. The session is defined as the duration of a single connection and both methods must be called on the same connection."],
so nothing survives to block the next run; and migrations are forward-only here — no migration in
this package carries a down section, as § *Restart hygiene* measures — so the operator's answer
is to run the command again. The serving path is unaffected — its migration step runs under
the registration step 1 made.

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
func (l *Liveness) Run(ctx) error                           // Refresh at Interval until stopped, ctx is done, or the tolerance is spent
func (l *Liveness) Stop()                                   // the drain's lever: Run returns nil after the heartbeat in flight
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
threshold and a restart would begin shifting rows. `Stop` is the other, prompted exit: `Run` returns
`nil` after the heartbeat in flight, on the `sync.Once`-channel contract § *Shutdown* fixes for all
three runners, and a spent tolerance never masquerades as a clean stop because the two return
different values. One success resets the count. Under the defaults
below that is a database this process has been unable to write to for about the threshold; it could
not have claimed a task or read an offset either, so stopping is both honest and self-correcting —
the gap the next start measures is then real downtime, and shifting by it is exactly right. `serve`
turns that return into the drain, per the runner-exit rule in § *Shutdown*.

The count is a count and the cadence is the ticker's, so nothing here reads a Go clock and the
liveness source stays free of `time.Now` / `time.Since` / `time.Until` — which is what keeps the
AC14 guard (§ Test Design T4) a whole-file walk.

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
ledger participation is asserted, not assumed — see the basis-and-ledger case in § Test Design T4.

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
`process_start_time_seconds`, for two reasons. That one comes from the process collector
`RegisterRuntime` installs
[measured f6d0c28:internal/health/registry.go:58 · sed -n '58p' internal/health/registry.go → "	if err := reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {"],
which collects nothing at all outside a Linux-style procfs and Windows
[measured f6d0c28 · go doc github.com/prometheus/client_golang/prometheus/collectors.NewProcessCollector → "The collector only works on operating systems with a Linux-style proc filesystem and on Microsoft Windows. On other operating systems, it will not collect any metrics."].
And this one is the instant *this* module began assembling, which is what an operator correlates
with a deploy rather than with the moment the kernel started the binary.

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
| `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT` | duration | `30s` | an operator-facing bound sized to the ingest side's cycle in flight, deliberately **not** to the scheduler's worst-case one — the paragraph below the table states what that costs and when it is re-tuned |
| `LAB_GAME_PROCESS_LIVENESS_INTERVAL` | duration | `30s` | bounds the recorded instant's staleness at one interval, far below the threshold |
| `LAB_GAME_PROCESS_DOWNTIME_THRESHOLD` | duration | `5m` | above any ordinary restart or deploy, below the power/internet outage §12.1 names as the motivation |

**What the shutdown budget does and does not cover.** `Worker.Stop()`'s contract in § *Shutdown* is
that `Run` returns after the cycle in flight, and a cycle is a whole claimed batch executed one task
at a time, each under its own execution deadline
[measured a1b0a81:internal/scheduler/worker.go:113,119-120 · sed -n '113p;119,120p' internal/scheduler/worker.go → "	ids, err := discoverDue(ctx, w.pool, w.cfg.ClaimLimit)", "	for _, id := range ids {", "		if err := w.executeOne(ctx, id, len(ids)); err != nil {"].
So the worker's cycle in flight is bounded by the claim limit *times* the task timeout, not by one
task deadline, and under this module's own defaults for those two keys that product is far above
any budget an operator would wait through
[measured a1b0a81:internal/config/scheduler.go:80,85 · sed -n '80p;85p' internal/config/scheduler.go → "		ClaimLimit:       32,", "		TaskTimeout:      30 * time.Second,"].
`30s` therefore does **not** promise that a claimed batch finishes: it is the bound on the whole
drain, chosen to cover the ingest side — where a cycle in flight is one already-fetched batch handed
to a router this task wires empty — and to stay inside the grace period a supervisor gives a process
before it escalates. The consequence is stated rather than discovered: once the first task type
lands, a drain that begins mid-batch is expected to hit the deadline, abandon the rest of the batch
and exit non-zero under AC28, which is the honest report and is safe by the spec's own *Technical
constraints*: an abandoned task loses its own transaction on connection teardown, so its row returns
to `pending` at its original `run_at` and the next start rediscovers it as due. This key and the
two scheduler keys are then re-tuned as a set, by whoever ships that task type and knows what its
handlers cost; nothing in this task's tree can claim a task at all, because the registry is wired
empty (AC31). These are operational tuning, not balance numbers, so a compiled-in default is correct
here and a `.env.example` row documents each. A `lookupBool` helper joins the lookup family this
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

### One source-walk guard helper, not one per package

This design's structural propositions — the AC14 clock rule and AC1's no-`init()` rule — are each
checked the same way: parse a package's non-test `.go` files, assert a construct is absent, then
re-run the same walk over a scratch file in `t.TempDir()` that *does* contain it, so the guard is an
instrument rather than a tautology. They arrive as the clock guard (§ Test Design T4) and the
`func init()` guard (§ Test Design T12a). The module already hand-rolls that enumerate-and-parse step
in `internal/health` and in `internal/ingest`, and those copies have already drifted apart in shape —
`internal/ingest`'s lists a single package directory, `internal/health`'s walks a subtree and filters
by top-level directory
[measured 9462d7e · grep -rl 'parser.ParseFile' --include='*_test.go' . | sort → "internal/health/guards_test.go", "internal/ingest/guards_test.go"].

Writing each new guard its own copy would put the consumers past the three-site threshold at which
`design-writer` § Rules requires a shared package instead of per-site duplication, so the
enumerate-parse-and-scratch step is lifted into a new test-support package, `internal/srcguard`, and
the consumers are named rather than counted: `internal/health`'s guards, `internal/ingest`'s guards,
`internal/scheduler`'s clock guard and `cmd/bot`'s composition-root guard. The existing copies
migrate to it in the subtask that creates it, because a shared package with the old copies still
standing beside it is the duplication the rule names, not a fix for it. What moves is only the mechanical part — file
enumeration, parsing, and writing a scratch package into `t.TempDir()`. Every predicate stays in the
package that owns the proposition: `promauto` bans stay in `internal/health`, the pgx-type and
context-position rules stay in `internal/ingest`, the clock ban stays in `internal/scheduler`. A
helper that knew what each package forbids would be a second place to look for a rule.

`srcguard` takes the repository root as an argument and derives none of its own: `internal/repotest`
is the module's single file-location-ascent resolver by an explicit contract
[measured 9462d7e:internal/repotest/repotest.go:1-7 · sed -n '1,7p' internal/repotest/repotest.go → "// Package repotest resolves the repository root for a test binary,", "// regardless of the working directory it was invoked from. Every package", "// in this module that needs the repository root during a test imports this", "// package rather than declaring its own copy of the ascent — it is the", "// module's one file-location-ascent resolver. This package must stay", "// exactly two directories below the repository root: Root's ascent", "// arithmetic is fixed to that depth."],
and that contract is itself guarded by a whole-tree walk that fails on any second resolver
[measured 9462d7e:internal/health/guards_test.go:667 · sed -n '667p' internal/health/guards_test.go → "func TestGuard_SingleRepoRootResolver_RealTree(t *testing.T) {"].
So `internal/repotest` is what resolves the root for `cmd/bot`'s guard — whose walk spans `cmd/` and
`internal/` from inside `cmd/bot`, the package that already imports `repotest` today
[measured 9462d7e · grep -l repotest cmd/bot/*_test.go → "cmd/bot/main_test.go"] —
and `srcguard` receives that root. Like `repotest`, `srcguard` is a non-test package that imports
`testing` and takes a `testing.TB`; that shape is this module's established one for test support, so
it needs no new argument.

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
| 1 | `internal/srcguard`: the shared source-walk guard support — non-test file enumeration for one package directory and for a subtree, parsing, and the `t.TempDir()` scratch-package writer the discriminating half needs; the repository root arrives as an argument (from `internal/repotest`), never from an ascent of its own. `internal/health`'s and `internal/ingest`'s hand-rolled copies migrate onto it **in this same step**, each package keeping every predicate of its own | `internal/srcguard/srcguard.go`, `internal/srcguard/srcguard_test.go`, `internal/health/guards_test.go`, `internal/ingest/guards_test.go` | — |
| 2 | The `LAB_GAME_PROCESS_` optional-with-default class: `Process` struct, the keys and defaults § Approach tabulates, `lookupBool`, `EnvKeys()` membership, `Config.Process`, `.env.example` rows (non-empty values), doc-comment updates | `internal/config/process.go`, `internal/config/process_test.go`, `internal/config/transport.go`, `internal/config/env.go`, `internal/config/config.go`, `internal/config/doc.go`, `.env.example` | — |
| 3 | `internal/store`: the forward migration adding the liveness singleton; `MigrateOption` + `WithAdvisoryLock`; the `ProcessLockID` constant both production callers pass; `HasPendingMigrations`; update every exact-set schema assertion in the package | `internal/store/migrations/00005_process_liveness.sql`, `internal/store/migrate.go`, `internal/store/migrate_test.go`, `internal/store/schema_test.go`, `internal/store/views_test.go`, `internal/store/fkcover_test.go` | — |
| 4 | `internal/scheduler`: `Liveness` — `NewLiveness`, `AbsorbDowntime`, `Refresh`, `Run`, `Stop`, `Downtime`; the all-SQL gap computation and the overdue shift; `Stop` on the `sync.Once`-channel contract § Approach → *Shutdown* fixes for all three runners (subtasks 5 and 6 give `Worker` and `Loop` the same one, independently), so the drain's lever is total over the runner set; the no-Go-clock source guard, written against `internal/srcguard` | `internal/scheduler/liveness.go`, `internal/scheduler/liveness_test.go`, `internal/scheduler/doc.go` | 1, 3 |
| 5 | `internal/scheduler`: `(*Worker).Stop()` — `Run` returns after the cycle in flight, `nil` when stopped and `ctx.Err()` when cancelled | `internal/scheduler/worker.go`, `internal/scheduler/worker_test.go` | — |
| 6 | `internal/ingest`: `(*Loop).Stop()` — same contract, plus cancelling the in-flight `getUpdates` so a long poll does not hold the drain | `internal/ingest/loop.go`, `internal/ingest/loop_test.go` | — |
| 7 | `internal/health`: `ReadyFunc`; `ServerOptions` + `NewServer` returning an error; the `/readyz` path; the `Process` collector and its families; `version` in the label ceiling; `LegsOptions.HTTPClient`; guard-fixture and package-doc updates, the source-walk half now driven through `internal/srcguard` | `internal/health/server.go`, `internal/health/process.go`, `internal/health/labels.go`, `internal/health/canary.go`, `internal/health/probe.go`, `internal/health/doc.go`, `internal/health/server_test.go`, `internal/health/process_test.go`, `internal/health/guards_test.go`, `internal/health/gather_test.go` | 1 |
| 8 | `internal/testdb`: `SchemaDSN` — one fresh schema handed to a caller outside this package as a connection string, carrying both the `search_path` that isolates it and the per-test pool cap the ceiling arithmetic assumes, so the consumer's own pool lands inside that arithmetic rather than beside it; the schema's drop stays on the same registered cleanup. `Binaries` is deliberately untouched here — the constant moves in subtask 9, the step that adds the caller | `internal/testdb/testdb.go`, `internal/testdb/testdb_test.go` | — |
| 9 | `cmd/bot`: `version` as a settable variable, the process logger, the build identity moved to stderr, `readiness` (the latches plus the pool round trip), the `closer` seam, and `assemble` — the whole start-up order, beginning with the signal registration, all-fatal, unwinding its closer list backwards — each landing **with its own tests in this same step**: the readiness cases, the start-up failure paths and the assembled happy path. That happy-path fixture is what makes this command a `testdb.Main` caller, so `TestMain` arrives here too and the `Binaries` constant is raised to the value the manifest test derives from the tree, the ceiling worked-example comment moving with it **in this same commit** — the manifest test going red→green here rather than sitting red across a group boundary. The existing `run` cases move out of `main_test.go` into `run_test.go` against the flipped stream | `cmd/bot/main.go`, `cmd/bot/readiness.go`, `cmd/bot/readiness_test.go`, `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go`, `cmd/bot/run_test.go`, `cmd/bot/main_test.go`, `internal/testdb/server.go`, `internal/testdb/server_test.go` | 2, 3, 4, 7, 8 |
| 10 | `cmd/bot`: the `runner` set, `serve` (the signal channel `assemble` registered **or** the first runner return begins the drain), `drain` (budget, second signal, exit codes, then the closer list walked backwards — the canary, the best-effort final liveness write, the listener, the pool, the signal deregistration), and the migrate-only path — all **with their tests in this same step**: `serve` and `drain` driven together with fake runners inside a `synctest` bubble, and the migrate-only path against a real schema. `errgroup` becomes a direct requirement here, so `go mod tidy`'s delta on the module files lands in this step | `cmd/bot/serve.go`, `cmd/bot/serve_test.go`, `cmd/bot/drain.go`, `cmd/bot/migrate.go`, `cmd/bot/migrate_test.go`, `go.mod`, `go.sum` | 5, 6, 9 |
| 11 | `cmd/bot`: `run`'s final shape — argv dispatch, usage, stdout left to an explicit `-h` alone — wiring the serve and migrate-only paths in, plus the package comment that stops calling this a scaffold; `run_test.go` grows the dispatch cases beside the configuration case subtask 9 moved there | `cmd/bot/main.go`, `cmd/bot/run.go`, `cmd/bot/run_test.go` | 10 |
| 12 | `cmd/bot` whole-root tests — the ones that need the assembled process and cannot be written beside a single file: the composition-root guards (the `func init()` walk, the package-level-var assertion, the sentinel-secret scrape-and-log guard, the import-graph discrimination proof), the in-process smoke condition against a real Postgres and a fake Bot API with a real `SIGTERM` and a leak check, and the subprocess link-time-version test. `goleak` becomes a direct test-only requirement here | `cmd/bot/guards_test.go`, `cmd/bot/smoke_test.go`, `cmd/bot/version_test.go`, `go.mod`, `go.sum` | 1, 11 |
| 13 | `cmd/importguard` and its gate wiring: the rule table, the classifier, its unit tests, `make import-guard`, the `verify` aggregate, the CI Build-job step | `cmd/importguard/main.go`, `cmd/importguard/run.go`, `cmd/importguard/run_test.go`, `Makefile`, `.github/workflows/ci.yml` | — |
| 14 | The lifecycle document (AC2 + AC3): the start-up order with a failure mode per step, and the migration policy in full — the default, the opt-out key, the migrate-only entry point, the lock that serialises concurrent starts, and what happens with auto-apply off and a migration pending — plus restart hygiene, readiness, the shutdown sequence and exit codes; its index row and the plans-index row | `ai-docs/process-lifecycle.md`, `ai-docs/agent-docs-index.md`, `ai-docs/plans/INDEX.md` | — |
| 15 | Propagation, per `AGENTS.md` § *Propagation Rule* step 4 — every live site whose claim this diff falsifies | `AGENTS.md`, `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/claude-tools-hierarchy.md`, `ai-docs/alert-contract.md`, `ai-docs/domain-invariants.md`, `ai-docs/code-style.md`, `ai-docs/go-test-conventions.md`, `.claude/skills/task/SKILL.md`, `.claude/skills/task/reference.md`, `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md` | 13, 14 |

**Every subtask that adds production code adds that code's tests in the same subtask.** That is
`AGENTS.md` § *Workflow*'s TDD rule, and it is also what the commit-level gate requires: the
group implementor commits once per subtask
[measured 9462d7e:.claude/agents/code-writer.md:57 · sed -n '57p' .claude/agents/code-writer.md → "**First rule of Mode A: you COMMIT after each subtask.** (Mode B never does — do not confuse them.)"],
and every one of those commits runs the coverage ratchet (§ Risks). A decomposition that shipped
`assemble.go` or `drain.go` in one subtask and their tests in the next would be asking that gate to
accept a statement-coverage drop it exists to refuse — so `readiness`/`assemble` and their tests are
subtask 9, `serve`/`drain`/migrate-only and their tests are subtask 10, and subtask 12 holds only
what genuinely needs the whole root standing.

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
new key decision block starts at the next unused KD number.
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
  window — subtasks 1–8 (code change-type: `*.go`, `*.sql`, `.env.example`). The library side: the
  shared guard support, then every package `cmd/bot` will assemble, each landing with its own tests
  and gates green before the composition root exists to depend on it. The boundary is where it is
  because subtask 9 is the first that depends on the library side being finished.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 9–13 (code change-type: `*.go`, `Makefile`, `.github/workflows/ci.yml`). The
  composition root itself — each production file with its own tests in its own subtask — the
  whole-root tests, and the import gate.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group C with fresh context.
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh), 1M-token window, via `general-purpose` with no inline `model=` — subtasks
  14–15 (instructions/harness change-type: `ai-docs/**`, `AGENTS.md`, `.claude/**`). Terminal group
  (2 subtasks; within the `1..=10` range).

## Risks

- **The coverage ratchet runs on every commit of this task, and a subtask that ships production code
  without its tests is a commit it refuses.** `.githooks/pre-commit` ends by execing the ratchet
  [measured 9462d7e:.githooks/pre-commit.sh:46 · sed -n '46p' .githooks/pre-commit.sh → "exec \"$root/.githooks/coverage-ratchet.sh\""],
  which blocks a statement-coverage drop past its stated tolerance
  [measured 9462d7e:.githooks/coverage-ratchet.sh:61 · sed -n '61p' .githooks/coverage-ratchet.sh → "TOLERANCE_PP=0.60"],
  and `--no-verify` is refused by a `PreToolUse` hook, so there is no way around it. The whole
  headroom is a fraction of a percentage point of the module's statements, while `assemble` alone is
  a step per start-up-table row, each with a failure branch and a reverse-order unwind. Mitigation:
  the decomposition pairs every production file with its tests in the same subtask — the paragraph
  under § Decomposition states the rule and § Test Design assigns the cases — so no commit in this
  task adds an uncovered block. Subtask 12 is the one step that adds no production statement at all —
  it is the whole-root tests — and a step that only adds tests can only move the measurement upward
  — [derived → each subtask's own gate run at commit time].
- **A drain that abandons in-flight work can still leave a goroutine of this module running, which
  AC29 forbids.** Mitigation: the deadline path cancels the run context and *then* joins the
  runners, and only then closes the pool; the join and the close are the "own final close" AC26
  places outside the bound. The signal channel is the same trap in miniature — a goroutine parked on
  `<-signals` would outlive the process's own shutdown — and this design has none: `serve` and
  `drain` select on the channel directly rather than through a watcher, so there is nothing to join,
  and the closer list's first entry (`signal.Stop`) is what releases the registration on every exit
  path. The registration's *own* background goroutine is the stdlib's, not this module's, and the
  leak checker's defaults already exclude it in both of its historical names plus the runtime
  goroutine `Notify` starts
  [measured a1b0a81 · sed -n '189,197p' "$(go env GOMODCACHE)"/go.uber.org/goleak@v1.3.0/options.go → "	// Importing os/signal starts a background goroutine.", "	if f := s.FirstFunction(); f == \"os/signal.signal_recv\" || f == \"os/signal.loop\" {", "	// Using signal.Notify will start a runtime goroutine.", "	return s.HasFunction(\"runtime.ensureSigM\")"],
  so making the registration a step of the order does not put a permanent stdlib goroutine in front
  of AC29's check — [derived → the smoke test's leak check (T12b) and the serve/drain cases (T10a)].
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
  The `Binaries` constant in `internal/testdb` sizes the shared server, and the manifest test
  re-derives its value from the tree and fails by name in both directions when the constant disagrees
  — so the constant's value is the tree's to state and this design's only to place
  [measured 9462d7e:internal/testdb/server_test.go:171-174 · sed -n '171,174p' internal/testdb/server_test.go → "// TestBinaries_matchesTree keeps the Binaries constant honest against the", "// tree: a package added or removed from the set of testdb.Main callers must", "// change the constant in the same commit, and this test fails by name in", "// both directions when it does not."].
  Mitigation: the raise lands in **subtask 9**, the step whose `assemble` happy-path fixture adds the
  caller, so the constant and the tree change in one commit and the manifest test is never left red
  across a subtask — let alone a group — boundary; `testdb.SchemaDSN` embeds the per-pool cap so the
  composition root's own pool stays inside the arithmetic the ceiling assumes.
- **A production-shaped advisory lock taken by an ordinary test fixture would serialise the whole
  suite on one database.** Mitigation: the lock is an opt-in option with a caller-supplied id, and
  the production id is a single named constant both production callers pass; `store.Migrate`'s
  default path is unchanged, so every existing fixture keeps its current behaviour — [derived → the
  store tests that call Migrate with no options, and the concurrent-apply case (T3c)].
  **The opposite direction is real and is accepted, not avoided: `cmd/bot`'s own production-path
  cases contend on `ProcessLockID` by construction.** T9b, T9c, T10b, T12a and T12b each hold cases
  that drive the assembled start-up with auto-apply on, and every one of those takes the
  database-wide production lock on the one shared test server — against every other such case,
  across the concurrent whole-module runs `CLIENTS=N` sizes and under `make test-contention`. They
  cannot opt out: taking that exact id is the thing AC7 makes these cases assert about. (The
  auto-apply-*off* cases are the exception, and for the reason § *Migration policy* already gives:
  `HasPendingMigrations` deliberately bypasses a configured locker, so they neither take the lock
  nor wait for it.) Correctness is unaffected, because goose retries rather than failing, and the
  bound is its documented default window — 5s between attempts, 60 attempts, 300s total
  [measured ad410c3 · go doc github.com/pressly/goose/v3/lock.WithLockTimeout → "By default, the lock timeout is 300s (5min), where the lock is retried every 5 seconds (period) up to 60 times (failure threshold)."] —
  and the loser then finds nothing pending and returns. Throughput is affected: these cases
  serialise against each other, so a `make test-contention` run that got slower after this task is
  that serialisation rather than a regression, and the window above is the number to compare it
  against. Stated here so it is diagnosed from the design rather than rediscovered from a
  stopwatch — [derived → T9b, T9c, T10b, T12a and T12b, whose fixtures all drive the production
  migration step].
- **A secret can reach a log line or a label the moment the composition root starts logging.**
  Mitigation: the bot token and the DSN stay inside the redacting secret type everywhere they are
  carried, the readiness body carries a fixed reason token rather than the underlying error, and a
  sentinel-secret guard in `cmd/bot` mirrors the one the health package already runs
  [measured 71ce0e4:internal/health/guards_test.go:438,445-447 · sed -n '438p;445,447p' internal/health/guards_test.go → "func TestGuard_ScrapeCarriesNoSentinelSecret(t *testing.T) {", "		sentinelBotToken    = \"1:SENTINEL-BOT-TOKEN-VALUE-----------\"", "		sentinelCloudToken  = \"1:SENTINEL-CLOUD-TOKEN-VALUE---------\"", "		sentinelDSNPassword = \"SENTINEL-DSN-PASSWORD\""].
- **A smoke test that sends the test process a real signal can kill the test binary if the handler
  is not installed yet.** Mitigation: the registration is step 1 of the start-up order — ahead of
  the listener's bind at step 7 and of the `migrated` latch at step 8, both of which a 200 from
  `/readyz` requires — so an observed 200 is *evidence* the registration already ran rather than a
  stage that merely tends to follow it, and the smoke test sends its signal only after that 200.
  This is the mitigation the ordering now supports: before the registration became a step of the
  order there was a window — every step between the latch and the runners starting — in which the
  test process carried the default disposition and the signal would have killed the binary with no
  diagnostic; after it, no step following step 1 has that window. And it does not call
  `t.Parallel()` — [derived → the smoke test (T12b), against the start-up order § Approach fixes].
- **A port chosen by the test can be taken between the choice and the bind, under
  `make test-contention`'s concurrent runs.** Mitigation: the metrics address is configured as
  `127.0.0.1:0` and the bound address is read back from the assembled value, so no port is ever
  guessed — [derived → the smoke test (T12b)].
- **Subtask 13 modifies `.github/workflows/ci.yml`, which binds the `actionlint` AXIOM to that
  subtask's commit.** `actionlint` MUST pass before `git add` on any modified workflow file, at
  the same standing as `go build ./...`
  [measured f6d0c28:AGENTS.md:75 · sed -n '75p' AGENTS.md → "> **AXIOM — `actionlint` MUST pass before `git add` on any modified `.github/workflows/*.yml`; `shellcheck` MUST pass before `git add` on any modified `*.sh`.**"].
  It is named here rather than left to be rediscovered mid-group: every other gate this section
  enumerates is Go-side, and subtask 13's workflow edit is the one diff in this task that no Go
  gate reaches. Mitigation: the group-B implementor runs `actionlint` on the changed workflow file
  before staging it — [derived → subtask 13's own gate run at commit time].
- **The project holds zero production panics and the index is empty**
  [measured 71ce0e4:ai-docs/panic-index.md · tail -1 ai-docs/panic-index.md → "| — | — | — |"].
  Nothing in this design adds a panicking call: `main` exits non-zero, which needs no row, and every
  new library entry point returns an error — [derived → the panic-gate hook on every non-test `.go`
  write, and `make lint`].

## Test Design

Every claim in this section is about a test that does not exist yet. Section numbers map to the
decomposition's subtask numbers, and every production file is covered by cases in **its own**
subtask — the pairing § Decomposition states and § Risks' first row explains. Subtasks 14–15 write
prose the repository's own link, citation and shape gates check.

### T1 — `internal/srcguard` (`internal/srcguard/srcguard_test.go`, beside the code)

- Entry points: the package-directory enumerator, the subtree walker, the parser, the scratch-package
  writer.
- Scenarios: the enumerator over a fixture directory returns the non-test sources and omits the
  `_test.go` ones and the subdirectories; the subtree walker visits a nested package and skips
  whatever the caller's filter rejects; the parser reports a parse error as a fatal naming the path
  rather than returning a nil file; the scratch writer creates a package under `t.TempDir()` whose
  path the enumerator then finds, which is the property every discriminating guard downstream
  depends on. Its own root argument is supplied by the caller, so the package needs no fixture
  repository [derived → the guards in T4 and T12a, which are its consumers].
- The migration half is verified by the moved guards themselves: `internal/health`'s and
  `internal/ingest`'s existing guard tests keep their names, their propositions and their
  discriminating proofs, and go green against the shared helper — a rename of the plumbing under
  assertions that do not change is the evidence that nothing was lost [derived → the two packages'
  existing guard suites, unchanged in what they assert].

### T2 — `internal/config` (`internal/config/process_test.go`, beside the code)

- Entry point: `Load`, `loadProcess`.
- Scenarios: every key absent → the documented defaults; each key present and well-formed →
  parsed; `MIGRATE_ON_START` present with a value `strconv.ParseBool` refuses → a `*KeyError` naming
  that variable with `ErrInvalidValue`; each duration present but zero, negative or malformed →
  a `*KeyError` naming it; every one malformed at once → every one reported through `errors.Join`.
- The existing disjointness fixture is what proves the manifest identity: the recording lookup must
  observe every one of them, and `.env.example`'s key set must equal `EnvKeys()`
  — [derived → AC6, and the config package's existing three-way key-set identity assertion].

### T3 — `internal/store` (`internal/store/migrate_test.go`, against a real Postgres)

- Entry points: `Migrate`, `WithAdvisoryLock`, `ProcessLockID`, `HasPendingMigrations`.
- Scenarios: (a) the exact base-table set gains the liveness table and no other, and the migration
  leaves it holding its singleton row with a **NULL** instant — the never-run-before state, and the
  row `AbsorbDowntime` locks on its first statement; the singleton `CHECK` refuses a second row; (b)
  `HasPendingMigrations` is true on a fresh schema and false after `Migrate`; (c) two concurrent
  `Migrate` calls under the *same* caller-supplied lock id against the same schema both return nil
  and the migration set applies once — the loser proceeds to a normal return rather than an error;
  (d) `Migrate` with no options behaves exactly as today, which is what keeps every other fixture
  in the module unchanged — [derived → AC7, AC11].
- Fixtures: the package's existing schema-scoped pool helper; case (c) needs two pools on one schema
  and a per-test lock id — never `ProcessLockID`, whose whole point is that it is database-wide.

### T4 — `internal/scheduler` liveness and the shift (`internal/scheduler/liveness_test.go`)

- Entry points: `AbsorbDowntime`, `Refresh`, `Run`, `Stop`.
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
    ride along silently until the next start reads a stale instant [derived → AC12, AC13];
  - **the stop seam** → the shared scenario list § Test Design T5/T6 states is required of
    `Liveness.Run` too, case for case: `Stop` before `Run`; `Stop` during the inter-heartbeat wait;
    `Stop` twice; and a context cancelled instead of stopped. It is one contract across the three
    runners, so it is one list, and this is the case that makes the drain's lever total rather than
    two-thirds present — without it `errgroup.Wait` cannot return on a clean signal and AC28's zero
    exit is unreachable in the assembled process [derived → AC28, and the drain ordering in T10a];
  - **a spent tolerance is not a stop** → `Run` that exhausted its failure tolerance returns its
    last error, while a stopped `Run` returns `nil`, so the drain's completed-versus-abandoned
    reading cannot confuse them [derived → AC28].
- **The clock-independence proof (AC14) is structural, because a test cannot make the two clocks
  disagree.** A guard in this package walks the liveness source through `internal/srcguard` and
  asserts it calls no `time.Now` / `time.Since` / `time.Until`, and — to prove the guard
  discriminates rather than passing vacuously — runs the same walk over a scratch package
  `internal/srcguard` wrote into `t.TempDir()` that *does* call `time.Now`, requiring a hit. The
  package's own clock rule is the proposition; the guard is the instrument [derived → AC14].
- Fixtures: a schema-scoped migrated pool; rows inserted with `run_at` expressed relative to `now()`
  in SQL.

### T5 / T6 — the stop seams (`internal/scheduler/worker_test.go`, `internal/ingest/loop_test.go`)

- Entry points: `(*Worker).Stop` + `Run`, `(*Loop).Stop` + `Run`.
- **Shared scenarios — the stop contract is one contract over the three runners, so this list is
  required of `Liveness.Run` as well** (§ Test Design T4 states it there, in that package's own
  suite): `Stop` before `Run` → `Run` returns `nil` without starting a cycle; `Stop`
  during the inter-cycle wait → `Run` returns `nil` promptly, well inside the poll interval; `Stop`
  called twice → no panic, no double close; context cancelled instead of stopped → `Run` still
  returns `ctx.Err()`, the existing contract. A runner missing any of them is a runner `serve`
  cannot drain, which is why the list is stated once and applied three times
  [derived → AC25, AC28].
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

### T7 — `internal/health` (`internal/health/process_test.go`, `server_test.go`, `guards_test.go`)

- Entry points: `NewServer`, `NewProcess`, the `/readyz` handler, `NewLegs`.
- Scenarios:
  - `NewServer` refuses a nil gatherer and a nil `Ready` with an error naming the field; a valid one
    binds, serves `/readyz`, and 404s everything else.
  - **`/metrics` is asserted exactly as amended AC21 words it**, and never as a byte-for-byte body —
    a golden of the body would be falsified by AC23's own additive families. Three propositions, all
    over one gather-and-scrape pair taken from a registry driven the way the package's existing
    gather fixtures drive it: the response's **content type and exposition format** are the ones the
    path answers with today; **every family name present before this task is still present**, with
    its type and its label set; and the **set difference** between the family names after and the
    family names before is **exactly** the families § Approach's table names — asserted as a set
    equality against that named list, so an accidental extra family fails just as a missing one
    does. The before-set is the family names a registry carrying only the pre-task collectors
    gathers, built in the test from the same constructors rather than copied into a literal
    [derived → AC21, AC23];
  - `/readyz` answers 200 with a fixed body when `Ready` returns nil and 503 with a fixed reason
    token when it returns an error, and the error's own text never reaches the body [derived → AC22, AC36];
  - `NewProcess` registers the families § Approach names; `labgame_build_info` carries the version as a label
    value and no version substring appears in any metric *name* [derived → AC23];
  - `labgame_ready` follows the injected `ReadyFunc` in both directions across two gathers, and a
    `ReadyFunc` that blocks past the gauge's internal bound still lets the gather return
    [derived → AC22];
  - `ObserveDowntime` sets the restart gauges [derived → AC18];
  - `NewLegs` with an `HTTPClient` builds both legs against it, and the existing pairing assertion
    is unchanged.
- The package's existing guards are extended rather than duplicated: the promlint guard and the label
  guard both drive the new collector, and the sentinel-secret guard gathers with it registered. Their
  source-walk half runs through `internal/srcguard` from subtask 1 onward.
- Fixtures: an isolated registry per case, the in-process fake Bot API server, the client-library
  test utilities the package already uses.

### T8 — `internal/testdb` (`internal/testdb/testdb_test.go`)

- Entry point: `SchemaDSN`.
- Scenarios: the returned string parses; its runtime parameters name the fresh schema; its pool cap
  equals the per-test cap the ceiling arithmetic assumes; a pool opened from it lands in that schema
  and the registered cleanup drops it [derived → the shape subtask 9's fixture consumes]. The
  binary-count manifest constant is *not* touched in this subtask — it moves with its caller, in
  subtask 9.

### T9 — `cmd/bot` readiness and assembly (`readiness_test.go`, `assemble_test.go`, `run_test.go`)

- **T9a — readiness.** Entry point: the readiness value's check method. Scenarios: neither latch set
  → not ready with the migrating reason; migrated, not draining, pool answers → ready; migrated, pool
  unreachable → not ready with the database reason; draining → not ready with the draining reason,
  *even when* the pool answers and even after migration; the draining latch is one-way
  [derived → AC22].
- **T9b — assembly, the happy path.** Entry point: `assemble`. `TestMain` provisions the shared test
  server — which is what puts this command in the set of `testdb.Main` callers, so the raised
  `Binaries` constant and this fixture land in one commit, the manifest test going red→green there
  [derived → the manifest test named in § Risks]. The fixture takes a fresh schema DSN, starts the
  in-process fake Bot API server, and assembles with its HTTP client,
  `LAB_GAME_HEALTH_METRICS_ADDR=127.0.0.1:0` and an empty cloud-canary token. Assertions:
  `assemble` returns with the schema migrated and **no** request yet at the fake Bot API and **no**
  scheduler row seeded — migrations precede the loop and the worker [derived → AC5]; `/readyz` on
  the bound address answers 200 [derived → AC22]; a `/metrics` scrape carries a Go-runtime family, a
  process-collector family, a pool family and every process-identity family [derived → AC32]; a send
  to a chat outside the allowlist is refused with the ingest package's refusal sentinel in the error
  chain [derived → AC30]; the assembled loop's router carries no route and the assembled worker's
  registry no declaration, observed through what each was constructed from rather than asserted about
  the constructor [derived → AC31]. The assembled value's `closer` list names its
  entries in start-up order, beginning with the signal registration — asserted here because it is
  the one list AC20's unwind and AC25's drain both walk, and a list built in the wrong order is a
  defect neither of those two cases would localise [derived → AC20, AC25]. That the registration is
  *live* rather than merely listed is T12b's to prove, since the only honest observation of it is a
  real signal reaching a real handler. The fixture shuts the assembled value down through the same
  backwards walk `assemble`'s own unwind uses, so no case leaks a listener into the next.
- **T9c — assembly, the failure paths.** Entry point: `assemble`, and `run` for the configuration
  case. Scenarios: a missing required configuration key → non-zero, stderr names the key and the
  configuration step, stdout empty [derived → AC4]; an unreachable DSN → the database step named and
  the cause carried [derived → AC4, AC19]; a DSN that parses but points at a database that never
  answers → the `pingTimeout` bound expires and the step still fails by name, rather than hanging
  [derived → AC19]; the example file's placeholder bot token → the Telegram-client step named
  [derived → AC19, AC35]; auto-apply disabled with a migration pending → both the disabling key and
  the pending state named [derived → AC8]; auto-apply disabled with nothing pending → assembly
  succeeds [derived → AC9]; a non-empty cloud-canary token that telego's format check rejects — the
  value the example file carries
  [measured f6d0c28:.env.example:165 · sed -n '165p' .env.example → "LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN=changeme"] —
  is refused by the leg builder, which builds the cloud leg exactly when that token is non-empty
  [measured f6d0c28 · go doc ./internal/health NewLegs → "NewLegs builds the own-instance leg and, when CloudToken is non-empty, the cloud reference leg — each leg's own credential paired with its own endpoint, never swapped."],
  so `assemble` fails at the **canary** step: stderr names that step, the exit is non-zero, stdout
  is empty and nothing serves — no degraded start. It is the case AC19's canary clause asks for,
  the owner's round-1 answer that a canary failure keeps the bot down until the operator fixes the
  credential; without it that clause's only cover is the generic post-listener case that follows
  [derived → AC19]; a failure at a step after the listener bound → the error names that step
  **and** the listener's port is free again, which is how "the started subsystems were shut down" is
  observed, the walk being the closer list backwards rather than a per-failure shutdown call
  [derived → AC20].
- **T9d — the identity stream.** Entry point: `run`. The case that exists today, rewritten for the
  flipped stream: the build identity appears on **stderr** and stdout is empty, on the
  configuration-failure path and on the path that gets past configuration alike [derived → AC4].
- Fixtures: the shared-server schema DSN from `testdb.SchemaDSN`, the in-process fake Bot API server,
  and a recording `io.Writer` pair standing in for stderr and stdout.

### T10 — `cmd/bot` serving, draining and migrate-only (`serve_test.go`, `migrate_test.go`)

- **T10a — serve and drain.** Entry point: `serve`, on an `app` value the in-package test builds
  directly — fake `runner` entries, a signal channel the test itself writes to, and a `closer` list
  whose entries record their name and return what the case wants them to. The `closer` seam
  § Approach names is what makes this case buildable at all: the subsystems `drain` walks are
  entries in a list rather than concrete fields, so no case here needs a real listener or a real
  pool.
  **The clock rule — per case, not per file: the subject is exact, the instrument is generous.**
  This module treats every wall-clock constant in a database-backed test binary as one of two
  things, and treats the two oppositely
  [measured 2bbeacf:ai-docs/go-test-conventions.md · sed -n 47p ai-docs/go-test-conventions.md → "Every wall-clock constant in a database-backed suite is one of two things, and they are treated oppositely."],
  and this binary is database-backed — its `TestMain` provisions the shared server, so every case
  here runs while sibling packages contend for it
  [measured 2bbeacf:cmd/bot/assemble_test.go · rg --no-line-number -o 'testdb\.Main\(m\)' cmd/bot/*_test.go → "cmd/bot/assemble_test.go:testdb.Main(m)"].
  An **instrument** is a patience budget — how long a case waits for `serve` to return before it
  gives up, or a `serve` budget deliberately large enough never to fire. It stays generous
  wall-clock time under one named package-level constant, and widening it changes no proposition any
  case asserts, so this design fixes neither its value nor a bracket around it
  [measured 2bbeacf:cmd/bot/serve_test.go · rg --no-line-number -o 'const patience\b' cmd/bot/serve_test.go → "const patience"].
  A **subject** is a duration that is itself the asserted property, and it must stay exact — which
  is what a wall clock under that contention cannot deliver. Hence the rule this design binds:
  **a case enters a `testing/synctest` bubble exactly when a duration is its subject, and nowhere
  else.** Inside the bubble the timing assertion is an equality rather than a bracket, because a
  virtual clock makes it bit-exact rather than merely bounded. Outside it, a case that asserts no
  duration buys nothing from the bubble and would still pay its cost — the bubble excludes the
  network its own guidelines exclude, and it constrains what the case may reach. The earlier draft
  of this section put every case in a bubble; that was the instrument half of the split being
  charged for a subject it never had.
  **Subject cases — inside a bubble, asserting an exact duration**
  [measured 2bbeacf:cmd/bot/serve_test.go · rg -U --no-line-number -o 'func (TestServe_[A-Za-z_]+)\(t \*testing\.T\) \{\n\tt\.Parallel\(\)\n\tsynctest\.Test' -r '$1' cmd/bot/serve_test.go → "TestServe_RunnerIgnoresStop_BudgetExpires_ExitNonZero", "TestServe_SecondSignalMidDrain_EndsAtOnce"]:
  one runner ignores `stop` and returns only on cancellation → the run context is cancelled, the
  runners are joined, the exit code is non-zero, and the drain is abandoned **at exactly the
  budget** [derived → AC26, AC28]; a second signal arrives mid-drain against a budget deliberately
  far larger → the wait ends **at exactly the instant the second signal arrived**, exit code
  non-zero [derived → AC27]. The equality is the point in both: a bracket ("well inside the budget")
  also passes for a drain that ended for the wrong reason, and exactness is what the virtual clock
  buys [measured 2bbeacf:cmd/bot/serve_test.go · rg --no-line-number -o 'elapsed != [a-zA-Z]+' cmd/bot/serve_test.go → "elapsed != budget", "elapsed != secondSignalAt"].
  Bubble rules: `synctest.Test` sits inside the test function and `t.Parallel()` outside it — the
  shape `internal/health`'s canary test already establishes in this module
  [measured 2bbeacf:internal/health/canary_test.go · rg --no-line-number -o 'synctest\.Test' internal/health/canary_test.go → "synctest.Test"].
  **Instrument cases — no bubble, one generous patience budget**
  [measured 2bbeacf:cmd/bot/serve_test.go · comm -23 <(rg --no-line-number -o 'func (TestServe_[A-Za-z_]+)' -r '$1' cmd/bot/serve_test.go | sort) <(rg -U --no-line-number -o 'func (TestServe_[A-Za-z_]+)\(t \*testing\.T\) \{\n\tt\.Parallel\(\)\n\tsynctest\.Test' -r '$1' cmd/bot/serve_test.go | sort) → "TestServe_DrainLatchesReadinessBeforeStoppingRunners", "TestServe_FailingFinalLivenessWrite_ReportedNotFatal", "TestServe_OtherClosingFailure_IsFatal", "TestServe_RunnerReturnsOnOwnWithNoSignal", "TestServe_SignalCleanStopExitZero"]:
  a signal arrives and every runner returns promptly on `stop` → exit code zero and no cancellation
  of the run context [derived → AC28], and the shutdown order is asserted by recording each fake's
  `stop` and each closer's call into one ordered log — runners stopped, runners joined, then the
  closer list backwards: canary, final liveness write, listener, pool, signal deregistration
  [derived → AC25]; and, because that order is the reverse of the order `assemble` appended the
  entries in, the same case asserts the log is exactly the reverse of the list it was handed rather
  than a hand-written expected sequence [derived → AC20, AC25]. The draining latch is latched
  **before any runner's `stop`**, and that gets a case of its own, which reads the readiness value
  from inside a fake runner's own `stop` as it runs: a re-check taken after `serve` has returned
  cannot distinguish "latched first" from "latched last", because both leave the latch set by then,
  so the proposition needs an observer positioned mid-drain rather than a trailing assertion nothing
  orders [derived → AC22]. A fake runner returns on its own with **no** signal sent → the same drain
  runs in the same order, that runner's name and its error reach stderr, and the exit code is
  non-zero [derived → the runner-exit rule § Approach → *Shutdown* fixes, under AC26's budget]. A
  failing final liveness write is reported and leaves the exit code alone; any other closer's error
  is fatal. The criterion that keeps this group out of a bubble is uniform: not one of these
  propositions is a duration — each is an exit code, an ordering, a latch reading or a stderr
  substring — so each is decided by the fake the case handed `serve` rather than by how long
  anything took, and the patience budget covering them may be widened without review.
  **Why fakes here and the real runner set in T12b.** The ordered log is an assertion about
  `serve`'s sequence, and a deterministic sequence needs a `stop` each fake honours on command — a
  real ingest loop, worker and heartbeat carry their own timing and their own database. So T10a
  proves the *ordering and the exit-code mapping* over fakes, and T12b proves that the **real**
  three-runner set returns on `stop` inside the budget, end to end. The seam between them is the
  shared stop contract T4 and T5/T6 assert per runner: each real runner is shown to behave as the
  fakes do, and `serve` is shown to do the right thing with runners that behave that way
  [derived → AC25, AC26, AC27, AC28].
- **T10b — migrate-only.** Entry point: the migrate-only path. Scenarios: it applies pending
  migrations under `ProcessLockID` and returns zero; a second invocation is a no-op and returns zero;
  no listener is bound (the configured metrics port stays free), no Bot API request reaches the fake
  server, and no scheduler row is seeded — the observable consequences of "constructs no client, no
  loop, no worker, no listener and no canary" [derived → AC10].

### T11 — `cmd/bot` argv dispatch (`run_test.go`)

- Entry point: `run(argv, lookup, stderr, stdout)`, the cases growing beside T9d's in the same file.
- Scenarios: an unknown subcommand → usage on stderr, the usage exit code, stdout empty; `-h` and
  `--help` → usage on **stdout**, exit zero, stderr carrying only the identity line — the one path
  this design lets write to stdout; `migrate` → the migrate-only path is the one taken, observed
  through its consequences rather than through a spy (the schema is migrated, the metrics port stays
  free) [derived → AC10]; no arguments → the serving path is the one taken [derived → AC4].

### T12 — `cmd/bot` whole-root tests

- **T12a — guards.** The composition-root shape, which is AC1's negative half and the one claim
  § Test Design otherwise assigns nowhere: a source walk, through `internal/srcguard` over the root
  `internal/repotest` resolves, across the non-test `.go` files under `cmd/` and `internal/`,
  asserting that none declares a `func init()` — none does today
  [measured f33b2bf · grep -rn '^func init()' --include='*.go' cmd internal | grep -v _test || echo "no match" → "no match"] —
  and, so the walk is an instrument rather than a tautology, the same walk over a scratch package
  `srcguard` wrote into `t.TempDir()` that *does* declare one must report it, the shape the AC14
  clock guard uses.
  **AC1's package-level-`var` clause is mechanical too, on the same walk and at the same scope.**
  It is not narrowed to `cmd/bot` and it is not left review-judged: the walk already visits every
  non-test `.go` file under `cmd/` and `internal/`, so the guard adds one predicate over each
  `GenDecl` of token `var` at file scope — **flag it when its initialiser is a call whose callee is
  a `New…` identifier that is either unqualified or qualified by a package name the file's own
  import block resolves to a path carrying this module's prefix.** That is a syntactic test needing
  no type checker, and it is the honest proxy for "holds a package-level mutable subsystem
  instance": in this module a subsystem is reached through its package's `New…` constructor, so a
  package-level `var` initialised from one is the shape the AC forbids, and a package-level `var`
  initialised from anything else is not a subsystem instance. Nothing under `cmd/` or `internal/`
  matches today — the only `New…` callees among package-level `var` initialisers there resolve
  outside this module
  [measured ad410c3 · grep -rnE '^var[[:space:]].*=.*\bNew' --include='*.go' cmd internal | grep -v '_test.go' | sed -E 's/.*=[[:space:]]*//;s/\(.*//' | sort -u → "decimal.New", "errors.New"] —
  and the guard is discriminated the same way the `init()` half is, by re-running the walk over a
  scratch package `srcguard` wrote into `t.TempDir()` that *does* declare such a `var`, requiring a
  hit. `cmd/bot`'s own tighter rule stays beside it, because this is where the risk concentrates:
  after this task the package is to declare no package-level `var` at all beyond the link-time
  `version` string subtask 9 installs, and the guard asserts exactly that.
  Deliberately **not** flagged by the predicate, and the reason each is not a subsystem instance:
  an `errors.New` sentinel and a `decimal.New` bound are immutable values, and a
  `var _ Iface = (*T)(nil)` compile-time assertion binds no instance to a name. The whole-process
  property the AC is really about — every subsystem constructed in `assemble` and handed to its
  user — is what T9b and T12b observe end to end; this guard is what stops a *later* package
  reintroducing the package-level form the AC forbids [derived → AC1].
  A sentinel-secret scrape-and-log guard: assemble with a sentinel bot token and a sentinel DSN
  password, drive one scrape and capture the process log writer, assert neither sentinel appears in
  either [derived → AC36]. An import-graph discrimination proof: the non-test
  dependency list of this command carries no container-runtime path, **and** its test dependency list
  does — the second half is what makes the first half evidence rather than a tautology
  [derived → AC33].
- **T12b — the in-process smoke condition.** Entry point: `run` with no arguments, on the fixture
  T9b established. Assertions, in order: `/readyz` answers 200 [derived → AC22]; a `/metrics` scrape
  carries a Go-runtime family, a process-collector family, a pool family and every process-identity
  family [derived → AC32]; the `getUpdates` the fake server received carries the reserved sentinel
  the ingest package substitutes for an empty route set [derived → AC31]; a real `SIGTERM` sent to
  the test process begins the drain, `/readyz` answers 503 while it drains, and `run` returns exit
  code zero within the budget — **this is the real three-runner stop set's proof**, because the zero
  exit is reachable only when the ingest loop, the scheduler worker *and* the liveness heartbeat each
  returned on `stop` rather than on the budget's cancel, and the assertion is additionally taken
  **well inside** `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT` rather than merely within it, so a runner
  without a stop lever fails here by elapsed time as well as by exit code
  [derived → AC25, AC28]. That same assertion is step 1's own proof: the signal
  reaches the drain rather than the test binary's default disposition only because `assemble`
  registered the channel before any of the observables this case waits on existed, and a design that
  registered later would fail here by killing the binary rather than by an assertion [derived → the
  start-up order § Approach fixes, AC25]. Then a leak check taken against a snapshot from
  before assembly reports nothing left running, the signal registration having been released by the
  closer list's first entry [derived → AC29]. Not parallel, and the leak check is
  the last assertion. This is the one case that drives a real signal through a real handler; the
  second-signal behaviour is T10a's, where the budget is virtual.
- **T12c — the link-time version.** Build this command into a temporary directory with
  `-ldflags "-X main.version=<sentinel>"`, run it with one required key deliberately absent, and
  assert: a non-zero exit, the sentinel in the identity line **on stderr** — the stream § Approach
  fixes — the failing step named, and stdout empty. One subprocess, no ports, no database — it exists
  for the one proposition an in-process test cannot reach [derived → AC24, AC4].

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
