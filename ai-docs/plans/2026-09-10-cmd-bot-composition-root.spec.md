# `cmd/bot` composition root: wiring, migration policy, graceful shutdown

**Source:** issue #24
**Date:** 2026-09-10
**Tracked in:** #24

`cmd/bot` is still the configuration-loading scaffold: `main` calls a `run` function that loads
configuration, prints the build identity and returns an exit code, and does nothing else
[source: 87399a7:cmd/bot/main.go § run · ast-index symbol "run"]. Every subsystem the bot needs
already exists as a constructible component with an explicit lifecycle, and every one of them was
deliberately shipped *without* process assembly, because assembly is this task.

This task makes `cmd/bot` the real composition root: it opens the pool, decides and executes the
migration policy, constructs and injects every subsystem in one place, exposes readiness, and
shuts the whole thing down cleanly on a signal.

**What already exists, verified at 87399a7.**

- `store.NewPool(ctx, *pgxpool.Config) (*pgxpool.Pool, error)`, whose doc states the config must
  come from `pgxpool.ParseConfig` because `pgxpool` panics on a hand-built config with a nil
  `ConnConfig` [source: 87399a7:internal/store/store.go § NewPool · ast-index symbol "NewPool"].
- `store.Migrate(ctx, *pgxpool.Pool, *slog.Logger) error`, forward-only, safe to call more than
  once, and requiring a non-nil logger
  [source: 87399a7:internal/store/migrate.go § Migrate · ast-index symbol "Migrate"].
- `config.Load(Lookup) (*Config, error)`, read once at start-up with no reload path
  [source: 87399a7:internal/config/config.go § Load · ast-index symbol "Load"].
- `tg.New(Options) (*Client, error)`, `ingest.New(Options) (*Loop, error)` with `Run`/`PollOnce`,
  `scheduler.New(Options) (*Worker, error)` with `Run`/`RunOnce`/`Reconcile`, and the whole
  `internal/health` surface — registry, runtime collectors, the three observers, the pool
  collector, the metrics `Server` with `Start`/`Addr`/`Shutdown`, and the two-leg `Canary` with
  `Start`/`Shutdown`.
- `internal/tgtest` — an in-process fake Bot API server — and `internal/testdb` — the shared
  Postgres provisioning helper — both already used by sibling packages.

## Scope

1. **One composition root.** A single place assembles every subsystem from the validated
   `*config.Config` and injects it explicitly: the pgx pool, the process logger, the metrics
   registry and its runtime collectors, the three observation adapters, the pool collector, the
   Telegram client with its outbound chat gate, the ingest router and loop, the scheduler registry
   and worker, the metrics server, the canaries and the readiness surface. No package-level
   singleton, no `init()` wiring, no `slog.SetDefault`, and no subsystem reaching for another
   through a global.
2. **A start-up sequence with an explicit, written order**, one failure mode per step, each exit
   naming the step that failed and the underlying cause. Configuration is already the first step
   and stays first.
3. **The production migration-apply policy**, implemented and written into the repository's
   durable docs — including what happens when two instances start against the same database at
   once. *(Decision pending — Open question Q1.)*
4. **Readiness**, distinct from the metrics endpoint: readiness is true once migrations are
   applied and the pool answers a round-trip query, and false before that and during shutdown.
5. **Process identity metrics**: the process start instant, the build version, and the readiness
   state, all on the same registry the metrics endpoint already serves.
6. **Graceful shutdown on `SIGINT` and `SIGTERM`**, bounded by a single whole-shutdown deadline:
   polling stops first, in-flight updates and the claimed scheduler batch are given the remaining
   budget to finish, the canaries and the metrics listener stop, the pool closes last. A second
   signal during shutdown stops waiting immediately.
7. **A failure policy for a subsystem that cannot be constructed or started**, and for one that
   fails after the process is up. *(Decision pending — Open question Q3.)*
8. **Restart hygiene after a long downtime** — `docs/DESIGN.md` §12.1's one rule, shifting overdue
   scheduler `run_at` forward by the downtime
   [source: 87399a7:docs/DESIGN.md § 12.1 · sed -n '/^### 12.1\./,/^### 12.2\./p' docs/DESIGN.md].
   *(Scope boundary pending — Open question Q2.)*
9. **Configuration for anything this task makes tunable** follows the established
   optional-with-default class: a `LAB_GAME_*` key, a compiled-in default named in its doc
   comment, a row in `.env.example`, and membership of the config package's exported key
   enumeration — the manifest/loader/declared-set identity check is a live gate
   [source: 87399a7:.env.example § header · head -20 .env.example].
10. **A machine-checked gate for the KD-20 invariant**: the container runtime never enters
    `cmd/bot`'s non-test import graph. The invariant is documented today and enforced by nothing
    [source: 87399a7:ai-docs/key-decisions.md § KD-20 · sed -n '/\*\*KD-20 —/,/\*\*KD-21 —/p' ai-docs/key-decisions.md].
11. **A smoke condition over the assembled process**: it starts against a real Postgres database,
    reaches readiness, serves metrics, and stops on a signal within the deadline leaving no
    goroutine of this module's packages running. The shape of the check — in-process versus
    subprocess, fake Bot API server, leak detector — is the design's call.
12. **Propagation.** This task changes a gate set and a documented start-up contract; every live
    site in the repository whose claim the diff falsifies is updated in the same PR, per
    AGENTS.md § *Propagation Rule* step 4. Known members of that class illustrate it, they do not
    bound it: AGENTS.md § *Build & Test*, `ai-docs/key-decisions.md`, `ai-docs/context-status.md`,
    `ai-docs/context.md`, `README.md`, the CI workflow's job/paths-filter set, and `.env.example`.

## Out of scope

- Container images, compose topology, systemd units, restart policy and backup wiring
  (`docs/DESIGN.md` §12.1–§12.3) — the infrastructure pass. This task names the signals that pass
  consumes (readiness, exit codes) and wires none of it.
- The testing environment and snapshot sanitisation (§12.5) — the infrastructure pass.
- Prometheus, Grafana, scrape config, dashboards and alert routing (§13.5) — the infrastructure
  pass; `ai-docs/` already carries the alert contract this task must not re-open.
- Any gameplay handler, any scheduler task type and any ingest route. None exists yet; they ship
  with their mechanics. The loop is wired with an empty router and the worker with an empty
  registry, both of which the two packages already declare legal.
- Reimplementing `store.Migrate` or `store.NewPool`. Extending `Migrate`'s options to serialise
  concurrent callers is in scope only if Q1's answer requires it.
- Webhooks. Long polling is the design's ingestion path and there is no second one.
- Filling in balance numbers (#46) and promoting anything from `docs/IDEAS.md`.
- The `-ldflags` release pipeline itself. This task makes the build version *settable and
  exported*; who sets it at release time is the infrastructure pass.

## Deferred

- Log level and log format as configuration keys | the process logger is constructed here, but a
  tunable level is operator convenience with no consumer yet | no — a Design Amendment or a later
  issue can add the key inside the established optional-with-default class.
- A readiness signal reachable from outside the host's loopback interface | the metrics listen
  address defaults to loopback and the consumer (a container healthcheck, an orchestrator probe)
  is defined by the infrastructure pass | no — the address is already a configuration key.
- Draining the outbound notification queue on shutdown | the queue is #43 and does not exist | no
  — #43 owns its own shutdown participation.
- Horizontal scale-out of the bot process | long polling is single-consumer by design (§12.4) | no.

## Key decisions

| Question | Decision |
|---|---|
| Migration-apply policy in production | **TBD — Open question Q1.** Carried forward from the ledger-post-core spec's inbox row, which names this task as its landing site. |
| Restart hygiene: the §12.1 downtime `run_at` shift | **TBD — Open question Q2.** |
| What a subsystem's construction/start failure does to the process | **TBD — Open question Q3.** |
| Where readiness is served | On the health listener the metrics endpoint already binds, at a path distinct from the metrics path. `internal/health` grows that path; the module gains no second listen address and no second configuration key. A separate listener stays available as a Design Amendment if the infrastructure pass needs one. |
| What readiness means | Migrations are applied **and** the pool answers a round-trip query. It is false before the migration step completes, false while the pool is unreachable, and false from the moment shutdown begins — so a probe distinguishes "starting", "serving" and "draining". |
| Shutdown budget shape | One whole-shutdown deadline, not a per-subsystem budget: a `LAB_GAME_*` optional-with-default duration key in the established class. The default value is the design's call and is stated in the key's doc comment and its `.env.example` row, never as a literal in a handler. |
| Signal set and second-signal behaviour | `SIGINT` and `SIGTERM` begin graceful shutdown. A second signal abandons the wait immediately and exits non-zero — an operator who asks twice is not made to wait out the deadline. |
| Process logger | One `*slog.Logger` constructed in the composition root and passed explicitly to every subsystem that takes one, including `store.Migrate`, which rejects a nil logger. Nothing calls `slog.SetDefault`; no package holds a package-level logger. Handler shape is the design's call. |
| Build version | `version` becomes a settable string **variable**, not a constant — see *Technical constraints*. It is exported as a metric label, never parsed or compared. |
| Scheduler `Reconcile` at start-up | The composition root may call `Reconcile` once before starting the worker, which the scheduler's own doc sanctions; whether it does is the design's call, because `Worker.Run` already reconciles before its first cycle. |
| Empty router and empty registry | Wired as they are. `ingest.Options.Router` documents an empty router as legal, and the loop substitutes a reserved sentinel into the transmitted allowed-updates list while the route set is empty; `scheduler.NewRegistry()` with no declarations is legal. No placeholder handler is invented to make the wiring look populated. |
| Chat allowlist placement | The outbound gate is installed on the Telegram client at construction, which is where `ingest` documents it belongs, so no handler can bypass it. |
| Ordering of the pool collector | The pool collector takes an accessor, so it is registered against the pool built in the same root; it never constructs a pool of its own. |
| Where the KD-20 invariant is enforced | A gate in the repository's own gate set, running in CI, over `cmd/bot`'s **non-test** import graph. The test-only import graph is explicitly not the subject: a `_test.go` under `cmd/bot` importing the test-database helper is legitimate and must stay legal. |

## Technical constraints

- **`-ldflags -X` cannot set a constant.** The linker's own documentation says `-X` "Set the value
  of the string variable in importpath named name to value", and that it is effective only when
  the variable is declared uninitialized or initialized to a constant string expression
  [source: go toolchain § cmd/link -X · go doc cmd/link]. `cmd/bot` currently declares
  `version` as a `const` whose comment claims the release pipeline overrides it with `-ldflags`
  [source: 87399a7:cmd/bot/main.go § version · ast-index symbol "version"]. That claim cannot hold
  as written; the build-version metric this task must export makes it load-bearing, so the
  declaration changes and the comment stops asserting something the toolchain refuses.
- **goose applies no lock unless one is configured.** `WithSessionLocker`'s own documentation
  states that if it is not called, locking is disabled
  [source: github.com/pressly/goose/v3@v3.27.3 § WithSessionLocker · go doc github.com/pressly/goose/v3.WithSessionLocker].
  `store.Migrate` builds its provider with a logger option and no locker, so two processes calling
  it concurrently are serialised by nothing above the database's own DDL locks. The package offers
  a Postgres session locker (`lock.NewPostgresSessionLocker`) built on advisory locks. Whatever Q1
  decides, the two-instances-at-once answer must be stated against this fact rather than assumed.
- **An interrupted scheduler task returns to `pending` at its original `run_at`.** Discovery
  claims with `FOR NO KEY UPDATE ... SKIP LOCKED` and each task executes inside its own
  transaction, so a process that dies mid-task loses that transaction on connection teardown: the
  row is not settled, `consecutive_failures` is not incremented, and the next start rediscovers it
  as due [source: 87399a7:internal/scheduler/claim.go § discoverDue · ast-index symbol "discoverDue"].
  That is the answer to "what a task that overran the shutdown deadline looks like on the next
  start" — it looks like a task that was never claimed.
- **An interrupted update re-arrives and is short-circuited.** The ingest offset is a database row
  advanced per settled update under a monotone guard, so a process that dies mid-batch re-polls
  from the last persisted offset; the re-delivered updates hit `player_operation`'s uniqueness
  constraint and settle as duplicates
  [source: 87399a7:internal/ingest/offset.go § advanceOffset · ast-index symbol "advanceOffset"].
- **The metrics server binds synchronously.** `Server.Start` binds the listener before spawning the
  serve goroutine, so a bind failure is a returned error at start-up rather than a background
  surprise; `Shutdown` is idempotent and every caller's wait is bounded by the context it passes
  [source: 87399a7:internal/health/server.go § NewServer · ast-index symbol "NewServer"]. The mux
  answers the metrics path and 404s everything else, so adding readiness to it is an edit to
  `internal/health`, not a mux the composition root assembles.
- **A malformed canary token refuses the whole canary at construction.** `tg.New` returns an
  `*OptionError` naming `Token` when the underlying client rejects the token, and the canary's leg
  builder wraps that failure, so `.env.example`'s placeholder cloud token stops the canary — own
  leg included — rather than merely failing cloud probes
  [source: 87399a7:internal/tg/client.go § New · go doc github.com/maratik123/lab-game/internal/tg.New]. This is exactly the case
  Q3 decides.
- **`go list -deps` excludes test-only imports.** The KD-20 invariant is about `cmd/bot`'s non-test
  import graph, and KD-20 says so in as many words — it warns that the invariant is *not* the same
  proposition as "only `_test.go` files import the test-database helper". A smoke check under
  `cmd/bot` may therefore import that helper; the gate must be written so it stays legal.
- **`cmd/bot` becomes a database-backed test binary.** A smoke condition against a real Postgres
  puts `cmd/bot` into the set of packages the shared-server route provisions, and into the set a
  bare `go test ./...` gives its own container. Both regimes must stay green.
- **Configuration is read once.** There is no reload path, so every value the composition root
  injects is fixed for the process lifetime, and a changed balance file or environment variable
  needs a restart. Nothing in this task introduces a reload.
- **No `panic` and no `log.Fatal` outside `main`'s own exit path.** `main` may exit non-zero;
  everything below it returns errors. `run` already documents that it never terminates the process
  itself.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | `cmd/bot` assembles every subsystem in one place from a validated configuration value, and no package under `cmd/` or `internal/` declares an `init()` function that constructs or registers a subsystem, holds a package-level mutable subsystem instance, or calls `slog.SetDefault`. |
| AC2 | The start-up sequence's step order is written down in a durable document under `ai-docs/`, and for every step the document names the failure mode and what the process does about it. |
| AC3 | Each start-up failure path produces a message on stderr naming the step that failed and the underlying cause, and a non-zero process exit code; stdout carries no error text. |
| AC4 | A single `*slog.Logger` is constructed in the composition root and passed to every subsystem that accepts one, the migration call included; no production file under `cmd/` or `internal/` obtains a logger from a package-level variable or from `slog.Default`. |
| AC5 | The pgx pool is built from a config produced by `pgxpool.ParseConfig` over the configured DSN, through the storage package's pool constructor, so the decimal codec registration applies to every connection. |
| AC6 | The readiness signal is served over HTTP at a path distinct from the metrics path, on a listener whose address is an existing configuration value, and the metrics path's response body is unchanged by this task. |
| AC7 | Readiness reports not-ready before the migration step has completed, ready once migrations are applied and a round-trip query against the pool succeeds, not-ready while that query fails, and not-ready from the instant shutdown begins. |
| AC8 | The metrics registry carries the process start instant, the build version and the readiness state, and the build version is carried as a label value rather than as a metric name component. |
| AC9 | The build version is a Go string variable declared uninitialized or initialized to a constant string expression, so `-ldflags -X` can set it, and no comment in the module claims a constant is settable at link time. |
| AC10 | `SIGINT` and `SIGTERM` each begin graceful shutdown: the update loop stops polling, the scheduler stops claiming, in-flight work is given the remaining budget, the canaries and the metrics listener stop, and the pool closes after them. |
| AC11 | The whole shutdown is bounded by one duration taken from a configuration key in the optional-with-default class, and the process exits within that bound plus the time its own final close takes, whether or not in-flight work finished. |
| AC12 | A second `SIGINT` or `SIGTERM` received during shutdown ends the wait immediately, and the process exits non-zero to distinguish an abandoned drain from a completed one. |
| AC13 | A completed graceful shutdown exits zero; a shutdown that hit the deadline with work still in flight exits non-zero. |
| AC14 | After a signal-driven shutdown, no goroutine started by this module's packages remains running in the process. |
| AC15 | Every environment variable this task adds is returned by the config package's exported key enumeration and documented in `.env.example`, and the manifest/loader/declared-set identity check passes with them present. |
| AC16 | Every duration, deadline and interval this task makes tunable reaches its subsystem from configuration; no such value is a literal in a production Go file. |
| AC17 | The Telegram client is constructed with the outbound chat gate installed, so no code path in the module can issue a message-delivering call to a chat outside the configured allowlist without passing that gate. |
| AC18 | The composition root wires the ingest loop with a router carrying no routes and the scheduler worker with a registry carrying no declarations, and neither package is modified to accept a placeholder handler or a placeholder task type. |
| AC19 | The pool collector registered on the metrics registry reads the same pool the ingest loop and the scheduler worker were given. |
| AC20 | Starting the assembled process against a Postgres database, with the Bot API base URL pointed at an in-process fake server, reaches readiness, serves a metrics scrape containing the runtime, pool and process-identity families, and terminates on a signal within the shutdown bound. |
| AC21 | A gate in the repository's gate set fails when `cmd/bot`'s non-test import graph contains the container-runtime module, and passes while a `_test.go` file under `cmd/bot` imports the module's test-database helper. |
| AC22 | That gate runs in CI on a pull request that touches Go source, and AGENTS.md § *Build & Test* lists the command that runs it. |
| AC23 | This task registers no event type and declares no posting signature, because it adds no mechanic and moves no balance. |
| AC24 | Every exported item added by this task carries a doc comment beginning with its name, and every new package carries a package comment. |
| AC25 | No comment added by this task names a markdown path, a design section number, an acceptance-criterion id, a repository path, a URL, an issue number outside `TODO(#…)`, or a package-qualified symbol of this module outside its own package. |
| AC26 | Production code added by this task contains no `panic` and no `log.Fatal`; `main`'s own non-zero exit is the only process termination. |
| AC27 | No secret reaches a log line, a metric name, a label or a label value; the bot token and the DSN stay inside the config package's redacting secret type everywhere they are carried. |
| AC28 | Every gate in AGENTS.md § *Build & Test* is green on the branch, including the race gate and the coverage ratchet at its recorded high-water mark or above. |
| AC29 | Module tidiness leaves `go.mod` and `go.sum` unchanged after the change. |
| AC30 | Every live site in the repository whose claim this diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4. |

Three further criteria land once Q1, Q2 and Q3 are answered: the migration policy's own condition
(including its concurrent-start behaviour), the restart-hygiene boundary, and the
construction-failure policy.

## Open questions

- **Q1 — production migration-apply policy.** Asked this round. Carried forward from
  `ai-docs/deferred/_inbox.jsonl`, whose open-question row names this task as the landing site.
- **Q2 — the §12.1 restart-hygiene `run_at` shift.** Asked this round. The design states the rule
  as MVP restart hygiene; no issue in the #47 decomposition owns it, and it plausibly lands with
  the process that does the restarting.
- **Q3 — construction/start failure policy for a subsystem.** Asked this round.
- **The readiness consumer.** Whether a container healthcheck, an orchestrator probe or a human
  reads it is settled by the infrastructure pass. This spec fixes the signal and its meaning; the
  loopback-only default of the listen address is already a configuration key, so the consumer is
  accommodated without reopening this task.
- **Whether the metrics endpoint should refuse to serve while not ready.** Default answer: no — a
  scrape during start-up and during drain is exactly when the metrics are most wanted, and the
  readiness gauge already distinguishes the states.
- **Log handler shape and level.** Left to the design; a level key can be added later inside the
  optional-with-default class without touching this task's decisions.
