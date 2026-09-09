# Health metrics and canaries: /metrics, update and scheduler lag, dual getMe

**Source:** issue #23
**Date:** 2026-09-09
**Tracked in:** #23

This is the health half of observability: a Prometheus registry the bot's already-instrumented
packages report into, an endpoint that serves it, and the two end-to-end `getMe` canaries the
design asks for by name. The product half — the `event` log and its SQL views — is a different
stack and a different dashboard, and it is already shipped elsewhere
[source: 081213a:docs/DESIGN.md § 13.1 · sed -n '/^### 13.1\./,/^### 13.2\./p' docs/DESIGN.md].

**What already exists, verified at 081213a.** Three packages each declare their own
consumer-side observation seam and call it from their own loop, and none of them has an
implementation: `tg.Observer` with `ObserveCall(Observation)`; `scheduler.Observer` with
`ObserveTask(Observation)` and `ObserveLoop(LoopObservation)`; `ingest.Observer` with
`ObserveUpdate(Observation)` and `ObserveLoop(LoopObservation)`
[source: 081213a:internal/tg/observe.go § Observer · ast-index symbol "Observer"]. Each doc
comment states that a nil `Observer` is checked, not called — so installing an implementation is
purely additive. `internal/store` exposes `NewPool`, returning a `*pgxpool.Pool`
[source: 081213a:internal/store/store.go § NewPool · ast-index symbol "NewPool"], whose `Stat`
method is the pool-occupancy source. `cmd/bot` is still the configuration-loading scaffold.

**Wiring is not this task's.** The roadmap puts `cmd/bot` composition — wiring, migration policy,
graceful shutdown — in #24, and lists #23 among #24's dependencies. This task therefore ships
constructible components with explicit lifecycles and changes no process assembly.

## Scope

1. **A new package** holding the health-metrics surface: a dedicated `prometheus.Registry`, the
   metric families, the observation adapters, the collectors and the canaries. Name, file split
   and internal shapes are the design's call.
2. **The registry is explicit and passed.** Metrics are registered on a registry the package
   constructs and hands out; nothing in the module registers on the client library's global
   default registerer, and the `promauto` convenience package is not used, so no import can
   register a metric as a side effect and a test can build an isolated registry.
3. **The endpoint**: a `promhttp` handler served by its own `http.Server` on its own listen
   address from configuration, with an explicit start and an explicit shutdown the caller owns.
   It shares a listener with nothing else
   [source: 081213a:docs/DESIGN.md § 13.5 · sed -n '/^### 13.5\./,/^## 14\./p' docs/DESIGN.md].
4. **An adapter implementing `tg.Observer`**, turning each per-call `Observation` into transport
   metrics: latency and response codes by Bot API method, plus the 429 and retry counters
   [source: 081213a:docs/DESIGN.md § 13.2 · sed -n '/^### 13.2\./,/^### 13.3\./p' docs/DESIGN.md].
5. **An adapter implementing `scheduler.Observer`**: scheduler lag by task type — the design calls
   its delay gameplay-visible — plus executed / guard-no-op / failed by type and failure kind, the
   claim batch size and the worker-loop duration.
6. **An adapter implementing `ingest.Observer`**: update lag (the dashboard's star), handler
   duration, handler errors, recovered handler panics, idempotency hits, unrouted and given-up
   updates, and the poll-cycle duration and batch size.
7. **A pgx pool collector** reading `pgxpool.Pool.Stat()` at scrape time — occupancy, waits and the
   rest of the `Stat` surface — fed a pool accessor at construction rather than constructing a
   pool itself.
8. **Runtime and process collectors** from the client library's `collectors` package: goroutines,
   GC and memory.
9. **The two canary probes**: `getMe` through our own instance and `getMe` straight at cloud
   `api.telegram.org`, each with its own metrics so the legs are separable, and a named signal for
   "own instance sick while cloud is healthy" that the infrastructure pass consumes.
10. **Configuration keys** for the metrics listen address, the canary interval, per-leg
    enablement, and whatever credential the cloud leg's decision requires — every one of them
    optional-with-default, under one new `LAB_GAME_<SUBSYSTEM>_` prefix consistent with the
    existing transport / scheduler / ingest key groups, loaded and validated in the existing
    `internal/config` style.
11. **The alert contract document**: for each alert the infrastructure pass must wire, the metric
    names, the expression over them, the condition shape and the severity — including the
    N-consecutive-canary-failures alert and the update-lag-growth alert the design names, and the
    cross-leg expression from Scope 9. It states shapes and names; the numbers are the
    infrastructure pass's to set.
12. **The module requirement** `github.com/prometheus/client_golang`, taken with `go get` followed
    by `go mod tidy`, with `git diff go.mod go.sum` read before staging. The design names promhttp
    as the endpoint's implementation, and AGENTS.md § *Dependency Versions* makes an established
    ecosystem package the default over anything hand-rolled.
13. **Cardinality discipline**, as an enumerated allow-list and an enumerated deny-list applied to
    every metric this task adds (Key decisions below).
14. **Propagation**: every site in the repository whose claim this diff falsifies is updated in the
    same PR, per AGENTS.md § *Propagation Rule* step 4. The class is "any live surface asserting
    what the bot exposes, what environment variables it reads, or which packages exist"; `.env.example`,
    `ai-docs/context.md` and `ai-docs/agent-docs-index.md` are known members that illustrate the
    class, not its boundary.

## Out of scope

- Prometheus and Grafana themselves, the scrape configuration, dashboards-as-JSON and Grafana
  provisioning — the infrastructure pass.
- The out-of-band alert channel (a separate alerter bot on the cloud API, or mail). This task names
  the signals that channel consumes; it does not build the channel.
- Scraping the `telegram-bot-api` instance's `/stats`. That endpoint belongs to the instance and
  Prometheus reaches it directly; no bot code is involved.
- Host metrics. The design's "basic host metrics" come from the host, not from this process; the Go
  runtime and process collectors are the bot's whole share.
- Notification-queue depth, send lag and failures. The queue is #43 and declares its own telemetry
  in its own PR, per the telemetry-with-the-mechanic invariant.
- `cmd/bot` assembly: constructing the pool, starting the loops, starting or stopping the metrics
  server, and graceful shutdown — #24.
- Product-dashboard work of any kind: no event type, no SQL view, no panel.
- Alert threshold values. The contract fixes the shapes and the names; the numbers are tuning.

## Deferred

- Notification-queue metrics | the queue does not exist yet | no — #43 owns them
- Dashboards-as-JSON, and the check that fails while a registered event type has no panel | #47
  suspended the limb until the infrastructure pass | no — #47 records it
- An in-process gauge for the cross-leg derived signal, instead of an expression in the alert
  contract | the default keeps inference in the monitoring stack | no — Open questions carries it
- A readiness or liveness endpoint next to `/metrics` | the design asks for neither; #24 may want
  one when it owns start-up | no

## Key decisions

| Question | Decision |
|---|---|
| Which Prometheus client library | `github.com/prometheus/client_golang`. The design names promhttp for the endpoint, and promhttp is that module's subpackage; AGENTS.md § *Dependency Versions* refuses hand-rolling in its place. |
| Global registry or an explicit one | An explicit `prometheus.Registry` constructed by the package and passed to every registration. No global default registerer, no `promauto`: a package-level registration is invisible at the call site, collides between tests, and cannot be isolated. |
| Where the endpoint's lifecycle lives | Here as a constructible server with explicit start and shutdown; #24 starts and stops it. The roadmap lists #23 as a dependency of #24, so the assembly direction is fixed. |
| Which labels are allowed | Bot API method name, Bot API method class, HTTP status code, scheduler task type, scheduler outcome, scheduler failure kind, ingest update kind, ingest outcome, canary leg, pgx pool state. Each value set is closed by an enum or by the set of methods this module itself calls. |
| Which labels are forbidden | Chat id, user or player id, update id, task id, operation id, raw error text, request URL, and the bot token in any form. Unbounded or player-identifying — a health series must not become a per-player series, and the product questions are the event log's. |
| Canary interval | A configuration key, optional-with-default, whose default is the design's once-a-minute cadence [source: 081213a:docs/DESIGN.md § 13.2 · sed -n '/^### 13.2\./,/^### 13.3\./p' docs/DESIGN.md]. |
| Canary cost against the rate limiters | Nil by construction: each leg gets its own client with its own limiter and a single attempt, so a canary neither draws on the production budget nor mixes into the production transport series. `getMe` classifies as `ClassOther` [source: 081213a:internal/tg/class.go § classifyMethod · sed -n '/^func classifyMethod/,/^}/p' internal/tg/class.go], whose windows are unbounded by default [source: 081213a:internal/config/transport.go § defaultTransport · sed -n '/^func defaultTransport/,/^}/p' internal/config/transport.go]. |
| Why one attempt, not the production retry policy | A retrying canary measures the retry loop, not the network: the failure it exists to show is masked by the retry and the latency it reports is mostly backoff. |
| The derived "own sick, cloud healthy" signal | Both legs export their own series; the cross-leg signal is a named expression in the alert contract, evaluated by the monitoring stack. The bot carries no second inference path, and the issue's own out-of-scope section asks this task to *name* the signals the alerting pass consumes. |
| Where the alert contract lives, and in which language | Under `ai-docs/`, in English, indexed in `ai-docs/agent-docs-index.md`. AGENTS.md restricts Russian to owner conversation and `docs/**`; the contract is an operations artefact the infrastructure pass reads, not part of the game-design corpus. |
| Handler panics | Already an ingest outcome (`OutcomePanic`), distinct from a plain failure. It reaches the dashboard as an outcome label value; this task adds no recovery logic of its own. |
| Idempotency hits | Already an ingest outcome (`OutcomeDuplicate`). Same treatment — duplicates are normal traffic and a spike is the signal. |
| Which credential the cloud canary leg uses | **Open — round 1, question 1.** See `## Source conflicts`. |
| What schedules the canary probes | **Open — round 1, question 2.** |

## Technical constraints

1. **The three `Observer` interfaces are consumer-declared and must not move.** Satisfying them is
   implicit in Go; this task adds no interface to `internal/tg`, `internal/scheduler` or
   `internal/ingest`, and changes no signature there.
2. **Every field of every `Observation` is accounted for.** A field that reaches no metric is named
   in the package's own documentation as deliberately unexported to metrics, so a later reader can
   tell an omission from a decision. `ingest.Observation.Lag` is meaningful only when `LagKnown` is
   true, so an unknown lag contributes no sample rather than a zero one.
3. **The canary client installs no outbound gate.** `getMe` addresses no chat, and the allowlist
   gate exists to stop writes to unintended chats; a canary sends no message.
4. **Nothing on the metrics endpoint is a secret or game data.** No token, DSN, chat id or player
   id appears in a metric name, a label, or a label value.
5. **Metric naming follows the Prometheus convention**: one module-wide prefix, base units
   (seconds, bytes), `_total` on counters, `_seconds` on duration histograms.
6. **Production code added by this task contains no `panic` and no `log.Fatal`.** A listener that
   fails to bind and a canary probe that fails are reported through returned errors and metrics;
   neither ends the process.
7. **The probe's clock and its HTTP endpoint are injectable**, so the canary's behaviour is
   observable without wall-clock waiting and without reaching the public internet.
8. **Comments in added code point at nothing outside themselves** — no markdown path, design
   section number, acceptance-criterion id, repository path, URL, issue number outside `TODO(#…)`,
   or package-qualified symbol of this module named outside its own package. The alert contract is
   found by name, never by a pointer from a comment.
9. **Every gate in AGENTS.md § *Build & Test* applies unchanged**, including the race gate — the
   canary runner and the metrics server are concurrent code — and the coverage ratchet.
10. **Configuration keeps its existing shape**: keys read only by `internal/config`, every added key
    optional-with-default, failures reported as a `*KeyError` naming the variable, and the
    manifest/loader/declared-key-set identity the config package already enforces stays true.

## Source conflicts

`docs/DESIGN.md` asks for a per-minute `getMe` at the cloud frontend while also fixing a runbook
that keeps the bot's cloud session closed. Both sites, verbatim:

**§13.2, the canary requirement**
[source: 081213a:docs/DESIGN.md § 13.2 · sed -n '/^### 13.2\./,/^### 13.3\./p' docs/DESIGN.md]:

> - **Канарейки, сквозные**: `getMe` раз в минуту через свой инстанс (проверяет цепочку свой сервер → MTProto → DC) + тот же `getMe` напрямую в облачный api.telegram.org как референс. При инциденте мгновенно видно, какой слой болит — та диагностика, которой не хватило в прошлый инцидент.

**§12.2, the instance-migration runbook**
[source: 081213a:docs/DESIGN.md § 12.2 · sed -n '/^### 12.2\./,/^### 12.3\./p' docs/DESIGN.md]:

> - **Runbook миграций (задокументировать в репе):**
>   - облако → свой: `logOut` на облачном API → дождаться закрытия сессии → первый запрос на свой инстанс. Без logOut апдейты размазываются между инстансами.
>   - свой → облако (аварийный откат): `close` на своем → до ~10 минут карантина, прежде чем облако примет бота.

**§12.5, a third site bearing on the same question**
[source: 081213a:docs/DESIGN.md § 12.5 · sed -n '/^### 12.5\./,/^## 13\./p' docs/DESIGN.md]:

> - **Отдельный тест-бот через облачный api.telegram.org** (не через свой инстанс). Причины: потеря бота нестрашна (пользователей двое — человек и агент), пересоздание через BotFather — дешевая операция; и главное — тестинг на облаке при проде на своем инстансе дает **два независимых пути до Telegram**: «тест-бот работает, прод висит» мгновенно локализует слой инцидента. Вторая канарейка бесплатно.

The tension: §13.2 reads naturally as the production bot's own token hitting the cloud frontend
every minute, while §12.2 states that the production bot is logged out of the cloud precisely so
that no second session exists there, and §12.5 already places a *different* bot on the cloud API
and calls it a second canary. Which credential the cloud leg carries decides a configuration key,
what "healthy" means for that leg, and whether the probe touches the production session at all.

**Resolution: pending.** Round 1, question 1 puts the choice to the product owner. Nothing in this
spec resolves it silently in either direction.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | A package under `internal/` constructs and exposes a dedicated `prometheus.Registry`, and every metric family this task adds is registered on a registry supplied by its caller. |
| AC2 | No file in the module imports `github.com/prometheus/client_golang/prometheus/promauto`, and no metric in the module is registered on the client library's global default registerer. |
| AC3 | The package exposes a `promhttp` handler over that registry, mounted on an `http.Server` whose listen address comes from configuration, with exported start and shutdown entry points and no listener shared with any other server in the module. |
| AC4 | A type in the package satisfies `tg.Observer`, and each field of `tg.Observation` — Method, Latency, StatusCode, RateLimited, Retries — either reaches an exported metric or is named in that package's documentation as deliberately unexported to metrics. |
| AC5 | A type in the package satisfies `scheduler.Observer`, and each field of `scheduler.Observation` and `scheduler.LoopObservation` either reaches an exported metric or is named as deliberately unexported to metrics. |
| AC6 | A type in the package satisfies `ingest.Observer`, and each field of `ingest.Observation` and `ingest.LoopObservation` either reaches an exported metric or is named as deliberately unexported to metrics. |
| AC7 | Update lag is exported as a duration histogram that takes a sample only from an observation whose `LagKnown` is true; an update kind carrying no date contributes no sample. |
| AC8 | Scheduler lag is exported as a duration histogram carrying a task-type label. |
| AC9 | Bot API call latency is exported as a duration histogram labelled by method; response codes are exported as a counter labelled by method and status code; 429 responses and retries are exported as their own counters. |
| AC10 | Ingest outcomes are separable in the exported series by an outcome label whose value set is exactly the members of `ingest.Outcome`, so recovered panics, idempotency duplicates, unrouted updates and given-up updates are each individually visible. |
| AC11 | Scheduler failures are separable in the exported series by a failure-kind label whose value set is exactly the members of `scheduler.FailureKind`. |
| AC12 | pgx pool occupancy and wait statistics are exported by a collector that reads `pgxpool.Pool.Stat()` during collection, so a scrape reports the pool's state at scrape time rather than a cached snapshot. |
| AC13 | The Go runtime collector and the process collector from the client library are registered on the same registry, so goroutine, GC and memory series are present in a scrape. |
| AC14 | Two canary probes exist — own instance and cloud reference — separable in the exported series by a leg label with exactly those two values, each exporting its own outcome counter and its own latency histogram. |
| AC15 | A canary call makes exactly one attempt, and no canary call contributes a sample to the transport series of AC9. |
| AC16 | A canary outcome distinguishes success from failure by the Bot API response, and classifies a failure by status code or by transport-error class; no canary failure classification carries raw error text as a label. |
| AC17 | The canary cadence is a configuration value whose compiled-in default is the design's once-a-minute cadence. |
| AC18 | The metrics listen address is a configuration value whose compiled-in default binds the loopback interface only. |
| AC19 | Every label on every metric this task adds belongs to the allowed set enumerated in Key decisions, and no exported metric carries a chat id, a user or player id, an update id, a task id, an operation id, raw error text, a URL, or any part of the bot token as a name, a label or a label value. |
| AC20 | Every environment variable this task adds is returned by the config package's exported key enumeration and documented in `.env.example`, and the config package's manifest/loader/declared-set identity check passes with them present. |
| AC21 | An alert-contract document exists under `ai-docs/`, in English, naming for each alert: the metric names it reads, the expression over them, the condition shape, and the severity — covering at minimum the consecutive-canary-failure alert, the update-lag-growth alert, and the cross-leg "own instance sick while cloud healthy" expression. |
| AC22 | `ai-docs/agent-docs-index.md` lists the alert-contract document. |
| AC23 | `github.com/prometheus/client_golang` is a require entry of `go.mod`, and module tidiness leaves `go.mod` and `go.sum` unchanged. |
| AC24 | No file in `cmd/` is modified by this task. |
| AC25 | Every exported item added by this task carries a doc comment beginning with its name, and every new package carries a package comment. |
| AC26 | No comment added by this task names a markdown path, a design section number, an acceptance-criterion id, a repository path, a URL, an issue number outside `TODO(#…)`, or a package-qualified symbol of this module outside its own package. |
| AC27 | Production code added by this task contains no `panic` and no `log.Fatal`; a bind failure and a probe failure are both reported through a returned error. |
| AC28 | Every gate in AGENTS.md § *Build & Test* is green on the branch, including the race gate and the coverage ratchet at its recorded high-water mark or above. |
| AC29 | Every live site in the repository whose claim this diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4. |

## Open questions

- **An in-process gauge for the cross-leg derived signal.** The decision above keeps the inference
  in the monitoring stack and the bot's output at two independent legs. Revisit if the
  infrastructure pass finds the expression awkward to write against scrape gaps.
- **Histogram bucket boundaries per family.** Transport latency lives in milliseconds and update
  lag in seconds-to-minutes, so one bucket set cannot serve both. The design's call, and cheap to
  change later since buckets are not persisted.
- **Alert threshold values** — how many consecutive canary failures, and what rate of update-lag
  growth. The contract states the shapes; the infrastructure pass sets the numbers.
- **A readiness endpoint beside `/metrics`.** Not asked for by the design; #24 may want one when it
  owns start-up and shutdown.
- **The `telegram-bot-api` `/stats` shape** is out of scope here, but the alert contract may want to
  name the series the infrastructure pass will scrape from it. Left to that pass, which owns the
  instance's configuration.
