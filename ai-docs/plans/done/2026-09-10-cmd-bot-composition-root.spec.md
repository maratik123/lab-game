# `cmd/bot` composition root: wiring, migration policy, graceful shutdown

**Source:** issue #24
**Date:** 2026-09-10
**Tracked in:** #24

`cmd/bot` is still the configuration-loading scaffold: `main` calls a `run` function that loads
configuration, prints the build identity and returns an exit code, and does nothing else
[source: 137eb7a:cmd/bot/main.go § run · ast-index symbol "run"]. Every subsystem the bot needs
already exists as a constructible component with an explicit lifecycle, and every one of them was
deliberately shipped *without* process assembly, because assembly is this task.

This task makes `cmd/bot` the real composition root: it opens the pool, applies the migration
policy, performs the design's restart hygiene, constructs and injects every subsystem in one
place, exposes readiness, and shuts the whole thing down cleanly on a signal.

**What already exists, verified at 137eb7a.**

- `store.NewPool(ctx, *pgxpool.Config) (*pgxpool.Pool, error)`, whose doc states the config must
  come from `pgxpool.ParseConfig` because `pgxpool` panics on a hand-built config with a nil
  `ConnConfig` [source: 137eb7a:internal/store/store.go § NewPool · ast-index symbol "NewPool"].
- `store.Migrate(ctx, *pgxpool.Pool, *slog.Logger) error`, forward-only, safe to call more than
  once, and requiring a non-nil logger
  [source: 137eb7a:internal/store/migrate.go § Migrate · ast-index symbol "Migrate"].
- `config.Load(Lookup) (*Config, error)`, read once at start-up with no reload path
  [source: 137eb7a:internal/config/config.go § Load · ast-index symbol "Load"].
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
3. **The production migration-apply policy, decided and implemented.** Pending migrations are
   applied at start-up by default. An environment switch in the established
   optional-with-default class turns that off, for an operator who applies them out of band; a
   migrate-only entry point exists for exactly that operator. Both paths take a database-held lock,
   so two processes starting at once cannot both apply the same migration. With auto-apply off and
   a migration still pending, the process refuses to start rather than serving against a schema it
   was not built for.
4. **Readiness**, distinct from the metrics endpoint: readiness is true once no migration is
   pending **and** the pool answers a round-trip query — one definition that holds in both
   migration modes — and false before that and from the moment shutdown begins.
5. **Process identity metrics**: the process start instant, the build version, and the readiness
   state, all on the same registry the metrics endpoint already serves.
6. **Graceful shutdown on `SIGINT` and `SIGTERM`**, bounded by a single whole-shutdown deadline:
   polling stops first, in-flight updates and the claimed scheduler batch are given the remaining
   budget to finish, the canaries and the metrics listener stop, the pool closes last. A second
   signal during shutdown stops waiting immediately.
7. **One failure rule for start-up: everything is fatal.** A subsystem that cannot be constructed,
   bound or started stops the process, naming it. There are no degraded modes and nothing is
   silently skipped — including the canaries, whose failure keeps the bot down until the operator
   fixes the credential.
8. **Restart hygiene after a long downtime**, the design's one rule: the process maintains a
   persisted liveness instant, and at start-up shifts every overdue pending scheduler row's
   `run_at` forward by the measured downtime, so backpack and corpse TTLs and raid-session timer
   edges do not fire in one salvo after a home-machine outage
   [source: 137eb7a:docs/DESIGN.md § 12.1 · sed -n '/^### 12.1\./,/^### 12.2\./p' docs/DESIGN.md].
   This grows a forward migration for the liveness surface and a start-up step that runs after
   migrations and before the scheduler worker.
9. **Configuration for anything this task makes tunable** follows the established
   optional-with-default class: a `LAB_GAME_*` key, a compiled-in default named in its doc
   comment, a row in `.env.example`, and membership of the config package's exported key
   enumeration — the manifest/loader/declared-set identity check is a live gate
   [source: 137eb7a:.env.example § header · head -20 .env.example].
10. **A machine-checked gate for the KD-20 invariant**: the container runtime never enters
    `cmd/bot`'s non-test import graph. The invariant is documented today and enforced by nothing
    [source: 137eb7a:ai-docs/key-decisions.md § KD-20 · sed -n '/\*\*KD-20 —/,/\*\*KD-21 —/p' ai-docs/key-decisions.md].
11. **A smoke condition over the assembled process**: it starts against a real Postgres database,
    reaches readiness, serves metrics, and stops on a signal within the deadline leaving no
    goroutine of this module's packages running. The shape of the check — in-process versus
    subprocess, fake Bot API server, leak detector — is the design's call.
12. **Propagation.** This task changes a gate set, a start-up contract and a documented claim about
    running the binary; every live site in the repository whose claim the diff falsifies is updated
    in the same PR, per AGENTS.md § *Propagation Rule* step 4. Known members of that class
    illustrate it, they do not bound it: AGENTS.md § *Build & Test* — whose `go run ./cmd/bot` line
    asserts that exporting `.env.example`'s variables runs the bot, which stops being true (see
    *Technical constraints*) — plus `ai-docs/key-decisions.md`, `ai-docs/context-status.md`,
    `ai-docs/context.md`, `README.md`, the CI workflow's job and paths-filter set, and
    `.env.example`.

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
- Reimplementing `store.Migrate` or `store.NewPool`. Extending the storage package with the
  locking option and the pending-migration query the policy needs **is** in scope.
- Webhooks. Long polling is the design's ingestion path and there is no second one.
- Filling in balance numbers (#46) and promoting anything from `docs/IDEAS.md`.
- The `-ldflags` release pipeline itself. This task makes the build version *settable and
  exported*; who sets it at release time is the infrastructure pass.
- Replacing `.env.example`'s placeholder credentials with values that would let the shipped example
  start the bot. A placeholder that is not a well-formed token stays a placeholder; the start-up
  failure naming the key is the intended behaviour under the all-fatal rule.

## Deferred

- Log level and log format as configuration keys | the process logger is constructed here, but a
  tunable level is operator convenience with no consumer yet | no — a Design Amendment or a later
  issue can add the key inside the established optional-with-default class.
- A readiness signal reachable from outside the host's loopback interface | the metrics listen
  address defaults to loopback and the consumer (a container healthcheck, an orchestrator probe)
  is defined by the infrastructure pass | no — the address is already a configuration key.
- Retrying a start-up step that failed for a transient reason (Postgres not yet accepting
  connections on a cold host boot) | the all-fatal rule plus the supervisor's restart policy is the
  designed answer, and restart policy is the infrastructure pass's | no.
- A ceiling on the restart-hygiene shift | the design's motivation is that nobody loses a backpack
  they could not physically walk to, so a long outage shifting a TTL by that long is the intent,
  not an overshoot | no.
- Draining the outbound notification queue on shutdown | the queue is #43 and does not exist | no
  — #43 owns its own shutdown participation.
- Horizontal scale-out of the bot process | long polling is single-consumer by design (§12.4) | no.

## Key decisions

| Question | Decision |
|---|---|
| Migration-apply policy in production | **Applied at start-up by default, with an opt-out.** A configuration key in the optional-with-default class controls it, defaulting to apply. A migrate-only entry point covers the operator who turns it off; its home — a subcommand of `cmd/bot` or a second `cmd/` binary — is the design's call, and whichever it is, the KD-20 import-graph gate covers it. Owner's answer, round 1. |
| Two instances starting at once | **Serialised by a database-held lock taken around the apply.** The migration package applies no lock unless one is configured (see *Technical constraints*), so this task configures one. The instance that loses the race waits, then finds nothing pending — it does not fail. Both the start-up path and the migrate-only entry point take the same lock. |
| Auto-apply off with a migration still pending | **Refuse to start**, naming the key that disabled auto-apply and the fact that a migration is pending. Serving against a schema the binary was not built for is the failure the opt-out would otherwise introduce silently, and it is also what makes readiness's definition checkable in both modes. |
| Restart hygiene: the §12.1 downtime shift | **In this task.** The process maintains a persisted liveness instant; at start-up it measures the gap to the database's current instant and, when the gap exceeds a configured threshold, moves every overdue pending scheduler row's `run_at` forward by that gap. Owner's answer, round 1. No issue in the #47 decomposition is scoped to the rule — the word does not occur in it — and the two mechanics whose behaviour it protects, corpse and backpack TTL (#40) and raid-session timer edges (#36), name neither the rule nor this issue among their dependencies, so nothing in the roadmap would otherwise have placed it. |
| Which rows the shift moves | **Every overdue pending row, as the design states it** — no narrowing by task class. A declared recurrence pushed forward is pulled back by the scheduler's own reconciliation, which runs before the worker's first cycle, so the literal reading needs no exception (see *Technical constraints*). |
| Where the downtime measurement's instants come from | **The database, both of them.** The scheduler package's standing rule is that every persisted instant is server-supplied and Go time survives only as an interval; the shift is a scheduler-semantics operation and follows the same rule. No `time.Now` participates. |
| The shift's threshold and the liveness cadence | Two configuration keys in the optional-with-default class. Their values are the design's call. |
| Start-up failure policy | **Every failure is fatal.** A subsystem that cannot be constructed, bound or started stops the process, naming it; nothing is skipped, disabled or degraded. Owner's answer, round 1, with the canary consequence explicitly accepted. Subsystems already started when a later step fails are shut down through their own entry points before the process exits. |
| Where readiness is served | On the health listener the metrics endpoint already binds, at a path distinct from the metrics path. `internal/health` grows that path; the module gains no second listen address and no second configuration key. A separate listener stays available as a Design Amendment if the infrastructure pass needs one. |
| What readiness means | No migration is pending **and** the pool answers a round-trip query. It is false before the migration step completes, false while the pool is unreachable, and false from the moment shutdown begins — so a probe distinguishes "starting", "serving" and "draining". |
| Shutdown budget shape | One whole-shutdown deadline, not a per-subsystem budget: a `LAB_GAME_*` optional-with-default duration key in the established class. The default value is the design's call. |
| Signal set and second-signal behaviour | `SIGINT` and `SIGTERM` begin graceful shutdown. A second signal abandons the wait immediately and exits non-zero — an operator who asks twice is not made to wait out the deadline. |
| Process logger | One `*slog.Logger` constructed in the composition root and passed explicitly to every subsystem that takes one, including `store.Migrate`, which rejects a nil logger. Nothing calls `slog.SetDefault`; no package holds a package-level logger. Handler shape is the design's call. |
| Build version | `version` becomes a settable string **variable**, not a constant — see *Technical constraints*. It is exported as a metric label, never parsed or compared. |
| Observability of the shift | The measured gap and the number of rows moved reach the metrics registry. A start-up step that silently rewrites scheduler rows is the shape of an incident nobody can reconstruct afterwards. |
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
  [source: 137eb7a:cmd/bot/main.go § version · ast-index symbol "version"]. That claim cannot hold
  as written; the build-version metric this task must export makes it load-bearing, so the
  declaration changes and the comment stops asserting something the toolchain refuses.
- **The migration package applies no lock unless one is configured.** `WithSessionLocker`'s own
  documentation states that if it is not called, locking is disabled
  [source: github.com/pressly/goose/v3@v3.27.3 § WithSessionLocker · go doc github.com/pressly/goose/v3.WithSessionLocker].
  `store.Migrate` builds its provider with a logger option and no locker, so two processes calling
  it concurrently are serialised by nothing above the database's own DDL locks. The package offers
  a Postgres session locker built on advisory locks, which is what the decided policy needs.
- **A pending-migration query already exists in the migration package.** `Provider.HasPending`
  reports whether migrations remain to apply
  [source: github.com/pressly/goose/v3@v3.27.3 § Provider.HasPending · go doc github.com/pressly/goose/v3.Provider.HasPending].
  `store.Migrate` constructs its provider internally and returns nothing about pending state, so
  the readiness definition and the auto-apply-off refusal both need the storage package to expose
  that answer.
- **The shift is self-correcting for recurrences, which is why no exception is needed.**
  Reconciliation seeds each declared recurrence at its cadence's next instant and corrects any
  stored `run_at` later than that instant, and the worker runs it before its first cycle
  [source: 137eb7a:internal/scheduler/reconcile.go § Reconcile · ast-index symbol "Reconcile"].
  So a recurrence whose `run_at` the shift pushed forward is pulled back by the worker's own
  start-up reconciliation — provided the shift runs **before** the worker starts, which is the
  ordering this task fixes.
- **`instance_key` is not a recurrence marker.** Any scheduled task may carry one — the scheduling
  entry point takes it from the caller's request
  [source: 137eb7a:internal/scheduler/schedule.go § Schedule · ast-index symbol "Schedule"] — and
  the registry, not the schema, is the authority on which types recur. A predicate over
  `instance_key` therefore does not select recurrences.
- **The shift touches no basis document.** The two task basis-document tables are written by the
  ledger's own basis constructors at posting time and carry the `run_at` the handler supplied
  [source: 137eb7a:internal/store/basis.go § DeferredTask · ast-index symbol "DeferredTask"], so
  moving a scheduler row's `run_at` neither writes nor rewrites live ledger data.
- **An interrupted scheduler task returns to `pending` at its original `run_at`.** Discovery
  claims with `FOR NO KEY UPDATE ... SKIP LOCKED` and each task executes inside its own
  transaction, so a process that dies mid-task loses that transaction on connection teardown: the
  row is not settled, `consecutive_failures` is not incremented, and the next start rediscovers it
  as due [source: 137eb7a:internal/scheduler/claim.go § discoverDue · ast-index symbol "discoverDue"].
  That is the answer to "what a task that overran the shutdown deadline looks like on the next
  start" — it looks like a task that was never claimed, and then the restart-hygiene shift applies
  to it like any other overdue row.
- **An interrupted update re-arrives and is short-circuited.** The ingest offset is a database row
  advanced per settled update under a monotone guard, so a process that dies mid-batch re-polls
  from the last persisted offset; the re-delivered updates hit `player_operation`'s uniqueness
  constraint and settle as duplicates
  [source: 137eb7a:internal/ingest/offset.go § advanceOffset · ast-index symbol "advanceOffset"].
- **The metrics server binds synchronously.** `Server.Start` binds the listener before spawning the
  serve goroutine, so a bind failure is a returned error at start-up rather than a background
  surprise; `Shutdown` is idempotent and every caller's wait is bounded by the context it passes
  [source: 137eb7a:internal/health/server.go § NewServer · ast-index symbol "NewServer"]. The mux
  answers the metrics path and 404s everything else, so adding readiness to it is an edit to
  `internal/health`, not a mux the composition root assembles.
- **`.env.example`'s placeholder credentials are not well-formed bot tokens, and under the
  all-fatal rule that now stops the process.** The Telegram library validates the token against
  `^\d+:[\w-]{35}$` before returning a bot
  [source: github.com/mymmrac/telego@v1.11.2 § tokenRegexp · sed -n '/tokenRegexp = /p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/bot.go],
  and the example file ships `changeme` for both the bot token and the cloud-canary token
  [source: 137eb7a:.env.example § LAB_GAME_BOT_TOKEN · grep -n LAB_GAME_BOT_TOKEN .env.example].
  The configuration loader accepts them — it validates presence, not token shape — so the refusal
  lands at client construction. Consequence: AGENTS.md § *Build & Test* claims that exporting
  `.env.example`'s variables runs the bot
  [source: 137eb7a:AGENTS.md § Build & Test · sed -n '/^## Build & Test$/,/^## API Stability$/p' AGENTS.md];
  after this change it does not, for the **bot** token before the canary one ever matters. That
  sentence is corrected in this PR under the propagation class.
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

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | `cmd/bot` assembles every subsystem in one place from a validated configuration value, and no package under `cmd/` or `internal/` declares an `init()` function that constructs or registers a subsystem, or holds a package-level mutable subsystem instance. |
| AC2 | The start-up sequence's step order is written down in a durable document under `ai-docs/`, and for every step the document names the failure mode and what the process does about it. |
| AC3 | That document also states the production migration-apply policy: the default, the opt-out key, the migrate-only entry point, the lock that serialises concurrent starts, and what a process does when auto-apply is off and a migration is pending. |
| AC4 | Each start-up failure path produces a message on stderr naming the step that failed and the underlying cause, and a non-zero process exit code; stdout carries no error text. |
| AC5 | Pending migrations are applied during start-up before readiness can report ready and before the update loop or the scheduler worker starts. |
| AC6 | Whether start-up applies migrations is controlled by a configuration key in the optional-with-default class whose compiled-in default applies them. |
| AC7 | The apply step holds a database-level lock for its duration, so two processes starting concurrently against the same database never both apply the same migration; the one that acquires the lock second proceeds to a normal start rather than exiting. |
| AC8 | With auto-apply disabled and at least one migration pending, the process refuses to start, names both the disabling key and the pending state, and exits non-zero. |
| AC9 | With auto-apply disabled and no migration pending, the process starts normally. |
| AC10 | A migrate-only entry point applies pending migrations under the same lock and exits, constructing no Telegram client, no update loop, no scheduler worker, no metrics listener and no canary. |
| AC11 | A forward migration adds the persisted liveness surface. |
| AC12 | While the process runs it refreshes the persisted liveness instant at a cadence taken from a configuration key in the optional-with-default class. |
| AC13 | At start-up, after the migration step and before the scheduler worker starts, the process computes the gap between the persisted liveness instant and the database's current instant; when the gap exceeds a configured threshold, every pending scheduler row whose `run_at` is already past has its `run_at` moved forward by that gap. |
| AC14 | A process whose own clock disagrees with the database's computes the same gap, and moves the same rows by the same amount, as one whose clock agrees. |
| AC15 | With no persisted liveness instant — a database this process has never run against — no row's `run_at` changes and the instant is seeded. |
| AC16 | With a measured gap at or below the threshold, no row's `run_at` changes. |
| AC17 | The restart-hygiene step writes to the scheduler task table only; it inserts into and updates neither of the two task basis-document tables, and it writes no posting and no journal entry. |
| AC18 | The measured gap and the count of rows the shift moved are exported on the metrics registry. |
| AC19 | A subsystem that fails to construct, bind or start stops the process with a message naming that subsystem and the underlying cause and a non-zero exit; no start-up path skips, disables or degrades a subsystem on failure, the canaries included. |
| AC20 | When a start-up step fails after earlier subsystems have started, those subsystems are stopped through their own shutdown entry points before the process exits. |
| AC21 | The readiness signal is served over HTTP at a path distinct from the metrics path, on a listener whose address is an existing configuration value; the metrics path continues to answer a scrape in the same exposition format and under the same content type, and every metric family exposed there before this task is still exposed under its original name, this task's additions being the only difference in the family set. |
| AC22 | Readiness reports not-ready while any migration is pending, ready once none is pending and a round-trip query against the pool succeeds, not-ready while that query fails, and not-ready from the instant shutdown begins. |
| AC23 | The metrics registry carries the process start instant, the build version and the readiness state, and the build version is carried as a label value rather than as a metric name component. |
| AC24 | The build version the process reports can be replaced at link time, with no source edit. |
| AC25 | `SIGINT` and `SIGTERM` each begin graceful shutdown: the update loop stops polling, the scheduler stops claiming, in-flight work is given the remaining budget, the canaries and the metrics listener stop, and the pool closes after them. |
| AC26 | The whole shutdown is bounded by one duration taken from a configuration key in the optional-with-default class, and the process exits within that bound plus the time its own final close takes, whether or not in-flight work finished. |
| AC27 | A second `SIGINT` or `SIGTERM` received during shutdown ends the wait immediately, and the process exits non-zero to distinguish an abandoned drain from a completed one. |
| AC28 | A completed graceful shutdown exits zero; a shutdown that hit the deadline with work still in flight exits non-zero. |
| AC29 | After a signal-driven shutdown, no goroutine started by this module's packages remains running in the process. |
| AC30 | A message-delivering call to a chat outside the configured allowlist is refused, whatever code path in the assembled process issues it. |
| AC31 | The composition root wires the ingest loop with a router carrying no routes and the scheduler worker with a registry carrying no declarations. |
| AC32 | Starting the assembled process against a Postgres database, with the Bot API base URL pointed at an in-process fake server, reaches readiness, serves a metrics scrape containing the runtime, pool and process-identity families, and terminates on a signal within the shutdown bound. |
| AC33 | A gate in the repository's gate set fails when `cmd/bot`'s non-test import graph contains the container-runtime module, and passes while a `_test.go` file under `cmd/bot` imports the module's test-database helper. |
| AC34 | That gate runs in CI on a pull request that touches Go source, and AGENTS.md § *Build & Test* lists the command that runs it. |
| AC35 | AGENTS.md § *Build & Test* no longer claims that exporting `.env.example`'s variables runs the bot, and states instead what the placeholder credentials do at start-up. |
| AC36 | No secret reaches a log line, a metric name, a label or a label value; the bot token and the DSN stay inside the config package's redacting secret type everywhere they are carried. |

## Open questions

- **Where the migrate-only entry point lives** — a subcommand of `cmd/bot` (one binary, one image,
  argument parsing added to a `main` that has none today) or a second `cmd/` binary. Default: the
  design's call. Both satisfy the acceptance criteria; the infrastructure pass consumes whichever
  is chosen, and it is out of scope here.
- **Whether the metrics endpoint should refuse to serve while not ready.** Default answer: no — a
  scrape during start-up and during drain is exactly when the metrics are most wanted, and the
  readiness gauge already distinguishes the states.
- **The readiness consumer.** Whether a container healthcheck, an orchestrator probe or a human
  reads it is settled by the infrastructure pass. This spec fixes the signal and its meaning; the
  loopback-only default of the listen address is already a configuration key, so the consumer is
  accommodated without reopening this task.
- **Log handler shape and level.** Left to the design; a level key can be added later inside the
  optional-with-default class without touching this task's decisions.
- **Whether the liveness instant later grows a second consumer.** Today only the restart-hygiene
  shift reads it. A future uptime or last-seen surface would read the same row; nothing in this
  task's shape prevents that, and nothing in it anticipates it either.
