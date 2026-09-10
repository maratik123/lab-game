# Process lifecycle — start-up, migrations, readiness, shutdown

`cmd/bot` is the composition root: the one place every subsystem is
constructed, and the one place the process's own lifecycle is decided. This
page is that lifecycle written down — the start-up order with a failure mode
per step, the production migration-apply policy, restart hygiene, what
readiness means, and how the process drains and what it exits with.

It describes the process, not the packages: what each subsystem does once it
is running lives with that subsystem (`ai-docs/context.md` § *Architecture*
maps them), and the metric surface's own contract lives in
[`ai-docs/alert-contract.md`](alert-contract.md).

## 1. Entry points, streams and exit codes

```
main()  ->  os.Exit(run(os.Args[1:], os.LookupEnv, os.Stderr, os.Stdout))
run()   ->  identity line on stderr, then argv dispatch:
              (no args)    -> assemble, then serve until a signal
              "migrate"    -> apply pending migrations and exit
              -h | --help  -> usage on stdout, exit 0
              anything else-> usage on stderr, exit 2
```

`run` never terminates the process itself; `main` is a one-line `os.Exit`.

**The build identity line is `run`'s first action, on stderr** — before argv
dispatch and before configuration is read, so even an invocation that dies at
the configuration step says which binary it was. The one path that writes to
stdout is an explicit `-h`/`--help`, which writes the usage text and nothing
else. Serving, migrate-only and every failure path leave stdout **empty**;
every diagnostic is a `lab-game bot: <name>: <cause>` line on stderr, where
`<name>` is the start-up step, the runner or the closer that failed.

| Exit code | Meaning |
|---|---|
| `0` | a completed graceful drain; a successful `bot migrate` (including a no-op one); `-h`/`--help` |
| `1` | any start-up step failed; a runner returned on its own before a signal; the drain hit its budget or was ended early by a second signal; a closer other than the liveness final write failed |
| `2` | an unrecognised subcommand — "this binary was invoked wrong", deliberately distinct from "this binary tried and failed" |

## 2. Start-up: one written order, all-fatal, unwinding

`assemble` runs the fourteen steps below in this order. **Every failure is
fatal** — no step skips, disables or degrades a subsystem when it fails, the
canaries included. On a failure the closers appended so far are walked
**backwards** (each step appends one only after the call that acquires its
resource returned nil), and the error names the step.

| # | Step | Constructs / starts | Failure mode | stderr step name | A failure here unwinds |
|---|---|---|---|---|---|
| 1 | signal registration | a buffered `os.Signal` channel and `signal.Notify` over it for `SIGINT`/`SIGTERM`; the matching `signal.Stop` is the closer list's first entry | none to report — `Notify` reads no configuration and returns no error | — | itself: it is the entry every later unwind ends on |
| 2 | configuration | `config.Load` over the environment lookup | a missing or malformed key, as the joined per-key error the config package returns | `configuration` | the signal registration |
| 3 | logger | the process `slog.Logger` over stderr | none: no key read, no I/O at construction | — | — |
| 4 | database | DSN parse, the pgx pool, then one `Ping` bounded by `pingTimeout` (**5s**, a named constant, not a key) | an unparsable DSN; a database that does not answer inside the ping bound | `database` | the pool, the signal registration |
| 5 | readiness | the readiness value — both latches unset, over the pool. Built before the registry, because the readiness gauge takes this value's method | none | — | — |
| 6 | metrics registry | the registry, the Go-runtime and process collectors, the transport/scheduler/ingest observers, the pool collector, the process-identity collector | a family the registry refuses | `metrics registry` | the pool, the signal registration |
| 7 | health listener | the metrics/readiness server, then `Start` — which binds synchronously, so a bind error is returned rather than lost on a goroutine | a nil option field; an address already in use | `health listener` | the pool, the signal registration |
| 8 | migrations | with auto-apply on, the migration apply under the process advisory lock; with it off, the pending check plus a refusal; then the `migrated` latch | a migration that fails to apply; auto-apply off with a migration pending (§ 3) | `migrations` | the listener, the pool, the signal registration |
| 9 | restart hygiene | the liveness value, `AbsorbDowntime`, and the restart gauges over what it returns | the transaction fails | `restart hygiene` | the listener, the pool, the signal registration |
| 10 | scheduler | the worker over an **empty** task registry, then `Reconcile` — called here so a reconcile failure is a start-up failure, not a mid-life return from the worker | an option the constructor refuses; a reconcile that fails | `scheduler` | the final liveness write, the listener, the pool, the signal registration |
| 11 | telegram client | the allowlist gate over the pool, then the Bot API client over the token, base URL, transport tuning and transport observer | an option the constructor refuses — including a token the client library's own format check rejects, which is what the example file's placeholder is | `telegram client` | the final liveness write, the listener, the pool, the signal registration |
| 12 | ingest loop | the loop over the client, the pool, an **empty** router and the ingest observer | an option the constructor refuses | `ingest loop` | the final liveness write, the listener, the pool, the signal registration |
| 13 | canary | both canary legs over the process HTTP client, the canary, then `Start` | a leg option the constructor refuses; a canary already started | `canary` | the final liveness write, the listener, the pool, the signal registration |
| 14 | runners | the runner set — ingest loop, scheduler worker, liveness heartbeat — constructed here, started by `serve` | none | — | — |

A failure *at* step 7 or step 13 does not unwind that step's own resource:
the closer for each is appended only after `Start` returned nil, and both
entry points refuse an unstarted value with an error rather than releasing
anything.

Three of those orderings are load-bearing rather than tidy:

- **The signal registration is first**, ahead of everything observable. Until
  `Notify` runs, `SIGINT`/`SIGTERM` carry their default disposition and kill
  the process outright — so every later step would otherwise be a window in
  which a `SIGTERM` is a sudden death with nothing on stderr, and the windows
  are not small (the migration step can sit on the advisory lock, restart
  hygiene is a transaction). Registering first buffers such a signal for
  `serve`, which finds it on its first select. **Assembly itself is not
  interruptible**, and that is deliberate: cancelling a half-applied
  migration buys nothing its transaction's rollback does not already give,
  and a start-up that hangs past the supervisor's grace period is the
  supervisor's `SIGKILL` to end. The channel is buffered for exactly two
  signals — the one that begins the drain and the one that ends the wait.
- **The health listener binds before the migration step** — 7 before 8, so
  "starting" reads as `503` rather than as connection refused, and a scrape
  during start-up and during the drain still answers.
- **Restart hygiene runs after migrations and before `Reconcile`** — 9 before
  10 — which also gives the shift its self-correction: a recurrence the shift
  pushed forward is pulled back by the reconcile that follows it.

## 3. Migration policy

**The default is apply-on-start.** `LAB_GAME_PROCESS_MIGRATE_ON_START` is an
optional-with-default key whose compiled-in default is `true`; step 8 applies
every pending migration before readiness can report ready and before any
runner starts.

**The lock.** The apply runs under a PostgreSQL **session-level advisory
lock**, held for the duration of the apply, taken under one exported
constant, `store.ProcessLockID` — whose value is the migration library's own
default lock id, so any other tooling driven by that library against the same
database contends on the same lock instead of applying beside us under a
private number. Both production callers — the start-up step and the
`bot migrate` subcommand — pass that same constant, which is what makes "two
processes starting concurrently never both apply the same migration" hold
across the *pair* of entry points rather than only within one of them. The
process that arrives second **waits** (the library retries the lock over a
five-minute window by default) and then proceeds to a normal start: it finds
nothing pending and serves. It does not exit.

The lock is **opt-in per call**, with a caller-supplied id, and stays off in
the default path: an advisory lock is database-wide while the test suite's
isolation is per-schema, so a lock inside the default apply path would
serialise every fixture in every test binary behind one lock.

**The opt-out, and what it costs.** With `LAB_GAME_PROCESS_MIGRATE_ON_START`
set false, step 8 asks the database whether anything is pending — a query
that deliberately takes **no** lock, so it neither blocks nor is blocked by a
concurrent apply — and then:

| Auto-apply | Pending migrations | Outcome |
|---|---|---|
| on (default) | any | applied under the lock, then the `migrated` latch is set and start-up continues |
| off | none | normal start: the latch is set and start-up continues |
| off | at least one | **refusal**: the process does not start. The stderr line names the step (`migrations`), the disabling variable by its literal name, and the pending state; the exit code is `1` |

**The migrate-only entry point.** `bot migrate` applies pending migrations
under the same lock id and exits — configuration, pool, ping, apply, and
nothing else: no signal registration, no readiness, no metrics listener, no
Telegram client, no ingest loop, no scheduler worker, no canary. A second
invocation against an already-migrated database is a no-op.

It deliberately does **not** register the signals. A registration with no
reader does not delay a signal, it swallows it — the default disposition is
disabled and the delivery goes to a channel nobody reads — so an operator's
`Ctrl-C` would become a no-op for as long as the lock wait runs. Dying inside
that wait costs nothing here: every migration in this repository runs inside
a transaction (none opts out), so a killed process rolls back the migration
in flight; the lock is session-level and dies with the connection, so nothing
survives to block the next run. Migrations are forward-only — no migration
carries a down section — so the correction for a bad one is the next forward
migration, and the answer to an interrupted run is to run it again.

**Readiness does not re-ask the database whether migrations are pending.**
After step 8 the answer is established by construction: either they were
applied, or the check returned false, or the process refused to start. The
step latches `migrated`, and readiness costs one pool round trip thereafter.

## 4. Restart hygiene

A process that was down long enough for a pile of timers to come due would,
on the next start, fire all of them in one salvo. Restart hygiene absorbs
that.

The persisted surface is a guarded singleton table, `process_liveness`,
added by a forward migration that also seeds its one row with a **NULL**
instant. NULL means "this database has never had a live process" — the state
that must be distinguished from a short gap — and the row exists from the
migration rather than from the first start so the locking read always has a
row to lock.

**At start-up** (step 9), in one transaction:

```
BEGIN
  SELECT seen_at, now() - seen_at FROM process_liveness WHERE id = 1 FOR UPDATE
      seen_at IS NULL   ->  never run here: nothing moves, the instant is seeded
      gap  > threshold  ->  UPDATE scheduled_task SET run_at = run_at + gap
                              WHERE state = 'pending' AND run_at <= now()
      gap <= threshold  ->  nothing moves
  UPDATE process_liveness SET seen_at = now() WHERE id = 1
COMMIT
```

Four properties of that shape are contractual:

- **The gap is computed in SQL.** No Go clock value is an input to the
  decision or an output of it, so a process whose own clock disagrees with
  the database's measures the same gap and moves the same rows. The liveness
  source calls none of `time.Now`, `time.Since` or `time.Until`, and a guard
  in the scheduler package's test suite holds that structurally.
- **The row lock, not an advisory lock, supplies mutual exclusion.** Two
  processes starting at once would otherwise each see the pre-shift instant
  and each apply the shift, moving every overdue row by twice the gap. Under
  `FOR UPDATE` the second transaction waits and then re-reads what the first
  wrote: it measures a gap of about zero and moves nothing, which is the
  correct answer rather than a suppressed one. An advisory lock is not used
  here because it is database-wide while this table is per-schema.
- **The shift writes `scheduled_task` and nothing else** — no basis-document
  row, no posting, no journal entry. It rewrites a `run_at` on rows that
  already exist, so it moves no balance and has no posting signature to
  declare.
- **Every overdue pending row moves**, with no narrowing by task class. The
  `Reconcile` that follows at step 10 pulls a pushed recurrence back.

**While the process runs**, the liveness heartbeat rewrites the instant every
`LAB_GAME_PROCESS_LIVENESS_INTERVAL`, which bounds the recorded instant's
staleness at one interval. A failed heartbeat is tolerated — but not
indefinitely, because the failure is silent and its consequence lands on
persisted rows: a heartbeat that died while the process kept serving would
make the *next* start measure a gap that was never downtime. So the heartbeat
counts consecutive failures and gives up at `ceil(threshold / interval)` —
the point at which the stored instant has aged past the threshold and a
restart would begin shifting rows. One success resets the count. Giving up is
an unprompted runner return, which § 6 turns into a drain with a non-zero
exit.

**Telemetry.** The measured gap and the number of rows the shift moved are
exported as gauges on the metrics registry (`labgame_restart_downtime_seconds`
and `labgame_restart_shifted_tasks`) — a start-up step that silently rewrites
scheduler rows is the shape of an incident nobody can reconstruct afterwards,
and the gauges are what make it reconstructable.

## 5. Readiness

Readiness is served over HTTP at `/readyz`, beside `/metrics`, on the same
listener — the address is `LAB_GAME_HEALTH_METRICS_ADDR`. Both paths are
contracts with the probe, not configuration keys. `/metrics` is unchanged by
readiness: same handler, same exposition format, same content type.

There is exactly **one** definition of ready, consumed by both `/readyz` and
the `labgame_ready` gauge, and it is three checks in this order:

| Condition | Answer | `/readyz` |
|---|---|---|
| the `migrated` latch is unset (start-up has not reached step 8) | not ready | `503`, body `not ready` |
| the `draining` latch is set (shutdown has begun) | not ready, unconditionally | `503`, body `not ready` |
| a pool round trip fails | not ready | `503`, body `not ready` |
| otherwise | ready | `200`, body `ready` |

Both latches are one-way and have exactly one writer each: `migrated` is set
by start-up step 8, `draining` by the drain's **first** action. The refusal
reason never reaches the response body or a scrape — the bodies above are
fixed strings. The call is internally bounded (5s) on both surfaces, so
neither a probe nor a scrape can hang on it.

The endpoint keeps answering throughout the drain, because the health
listener is itself a closer and is stopped near the end of the shutdown
order.

**Process identity on the registry**, alongside readiness:

| Family | Type | Carries |
|---|---|---|
| `labgame_build_info` | gauge, always 1 | the build version, as the `version` **label value** — never as part of a metric name |
| `labgame_start_time_seconds` | gauge | the instant this module began assembling (not the instant the kernel started the binary) |
| `labgame_ready` | gauge func | 1 when the readiness definition above says ready, 0 otherwise |
| `labgame_restart_downtime_seconds` | gauge | the gap § 4 measured |
| `labgame_restart_shifted_tasks` | gauge | how many rows § 4 moved |

The build version is a package-level `var` overridden at link time
(`-ldflags "-X main.version=..."`), so a release pipeline sets it with no
source edit.

## 6. Shutdown: the drain

`serve` starts every runner and then waits on exactly two things: a signal on
the channel step 1 registered, **or** the first runner to return on its own.

**A runner returning before a signal is itself a shutdown trigger.** After a
stop, a return is the success path; before one, it means that subsystem is
gone — the worker returns outright when its opening reconcile fails, and the
heartbeat returns when its failure tolerance is spent. Such a return begins
the same graceful shutdown a signal begins, in the same order and under the
same budget, reports that runner's name and error on stderr, and makes the
exit code non-zero: a process that has lost a subsystem must not go on
looking healthy.

The drain, in order:

1. **Latch `draining`** — before every stop and every close, so `/readyz`
   answers not-ready from the instant shutdown begins.
2. **Call every runner's `stop`.** Each runner has a stop seam beside its
   context: the loop, the worker and the heartbeat each return after the
   cycle in flight, `nil` when stopped and `ctx.Err()` when cancelled — which
   is what lets the drain tell a completed shutdown from an abandoned one.
   The ingest loop additionally cancels the `getUpdates` in flight, so a
   graceful shutdown does not wait out the long-poll window; that cancel is
   scoped to the API call alone — the offset read and every update already
   fetched keep the parent context and settle. A poll discarded that way is
   reported as a discarded poll, not as a poll error.
3. **Join the runners**, bounded by `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT`. If
   the budget expires, or a second signal arrives, the run context is
   cancelled and the join completes; the drain is then **abandoned** and the
   exit code is non-zero.
4. **Walk the closer list backwards** — the canary, the final liveness write,
   the health listener, the pool, the signal deregistration — under a fresh
   context bounded by the same duration. That walk is the process's own final
   close: the drain's bound covers the join, and the close that follows it is
   what the process spends *beyond* that bound. A closer that returns an
   error is reported by name; the **final liveness write is the one
   best-effort entry** whose failure never changes the exit code, because the
   next start then measures its gap from the last successful heartbeat, one
   interval old at worst.

The closer list is the single place the shutdown order is written down:
`assemble` appends to it as resources are taken, and both the start-up unwind
and the drain walk that same list backwards. There is no second, parallel
order to keep in step by hand.

**No goroutine of this module survives the drain.** `serve` and the drain
select on the signal channel directly rather than through a watcher, so there
is nothing parked on it to outlive the process, and the deregistration is the
closer list's first entry — hence the last thing every exit path performs.

**What the budget does and does not cover.** `30s` is the bound on the whole
drain, sized to the ingest side, where a cycle in flight is one already-
fetched batch. It does **not** promise that a claimed scheduler batch
finishes: a worker cycle is a whole claimed batch executed one task at a
time, bounded by the claim limit *times* the task timeout, which under this
module's own defaults is far above any budget an operator would wait through.
Once the first task type lands, a drain that begins mid-batch is expected to
hit the deadline, abandon the rest of the batch and exit non-zero — which is
safe, because an abandoned task loses its own transaction on connection
teardown, its row returns to `pending` at its original `run_at`, and the next
start rediscovers it as due. That key and the two scheduler keys are re-tuned
as a set by whoever ships that task type. Nothing in the current tree can
claim a task at all: the registry is wired empty.

## 7. The process configuration keys

All four are optional-with-default: an absent variable takes the compiled-in
default, and a present but malformed one fails `config.Load` at start-up
step 2, naming the key. These are operational tuning, never balance numbers.

| Key | Shape | Default | What it decides |
|---|---|---|---|
| `LAB_GAME_PROCESS_MIGRATE_ON_START` | bool | `true` | whether start-up applies pending migrations (§ 3) |
| `LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT` | duration | `30s` | the whole drain's bound (§ 6) |
| `LAB_GAME_PROCESS_LIVENESS_INTERVAL` | duration | `30s` | the heartbeat cadence, and with it the recorded instant's staleness (§ 4) |
| `LAB_GAME_PROCESS_DOWNTIME_THRESHOLD` | duration | `5m` | the gap above which the shift runs, and the heartbeat's failure tolerance (§ 4) |

`.env.example` carries a row for each, with the default as its value.
