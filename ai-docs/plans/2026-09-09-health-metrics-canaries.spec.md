# Health metrics and canaries: /metrics, update and scheduler lag, dual getMe

**Source:** issue #23
**Date:** 2026-09-09
**Amended:** 2026-09-09 (design-review rounds 1 and 3; AC35 corrected at round 5)
**Tracked in:** #23

> **Amended after design-review round 1.** The canary families carry an outcome label and a
> failure-reason label. The Key-decisions allow-list — which AC23 makes binding by reference —
> enumerated neither, while AC14 already required a per-leg outcome counter and AC19 already
> required the failure classification. The list was written before the canary metric shapes
> existed, so AC23's guard would have passed against a label set the spec never authorised. The
> amendment is exactly: the **Which labels are allowed** row now names the canary outcome and the
> canary failure reason, with their value sets stated closed the same way the existing members
> are, and the **Which labels are forbidden** row separates a classified reason from raw error
> text in one clause. Approved by the product owner. No acceptance criterion changes, no scope
> moves, and nothing else in this spec is re-opened.

> **Amended after design-review round 3.** The helper that resolves a path relative to the
> repository root is declared independently in several packages' test binaries, and this task's
> own tests would have added one more. AC29 — "no file in `cmd/` is modified by this task" —
> blocked the consolidation, because one of those declarations lives in `cmd/bot`'s test binary.
> The product owner chose to widen this task rather than defer the hoist. The amendment is
> exactly: **Scope 15**, one clause on the `cmd/bot` **Out of scope** bullet, two **Key
> decisions** rows, a narrowed **AC29**, and the new **AC35** and **AC36**. The relaxation
> reaches `cmd/bot`'s test binary and the helper swap alone — composition, wiring, start-up and
> shutdown remain #24's, and no production file under `cmd/` is touched. Nothing else in this
> spec is re-opened.

> **AC35 corrected at round 5.** As first written, AC35 required exactly one declaration of
> `repoRootPath` in a shared package while every package called that one. No implementation can
> satisfy that: an unexported identifier is unreachable across a package boundary, so the shared
> declaration must carry an exported name and the old spelling must survive nowhere — a
> per-package alias under the old name would restore exactly the duplication Scope 15 forbids.
> The design-writer found it and refused to paper over it. The correction is exactly: AC35's
> wording, and the sentence of **Scope 15** that carried the same assumption. Approved by the
> product owner. AC29 and AC36 are untouched, and no other row changes.

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
9. **The two canary probes**, driven by an **in-process ticker** on the configured interval — a
   goroutine this package owns, touching neither Postgres nor the scheduler, so both legs keep
   reporting while the database is unreachable. The own-instance leg calls `getMe` with the
   production bot token against the configured instance base URL, exercising own server → MTProto →
   DC. The cloud leg calls `getMe` at `api.telegram.org` under a **separate cloud-side bot token**,
   so the production bot's cloud session is never reopened. Each leg exports its own series so the
   two are separable, and the cross-leg signal "own instance sick while cloud is healthy" is named
   for the infrastructure pass to consume.
10. **Configuration keys** for the metrics listen address, the canary interval, the cloud leg's
    own bot token and the cloud leg's base URL — every one of them optional-with-default, under one
    new `LAB_GAME_<SUBSYSTEM>_` prefix consistent with the existing transport / scheduler / ingest
    key groups, loaded and validated in the existing `internal/config` style. The cloud token is a
    secret and is carried in the type the config package already uses for the bot token and the
    DSN. **An absent cloud token disables the cloud leg**: the process starts, the own-instance leg
    runs, and configuration raises no error.
11. **The alert contract document**: for each alert the infrastructure pass must wire, the metric
    names, the expression over them, the condition shape and the severity — including the
    N-consecutive-canary-failures alert and the update-lag-growth alert the design names, and the
    cross-leg expression from Scope 9 — including what that expression means when the cloud leg is
    configured off, and the absence test that tells a disabled leg from a failing one. It states
    shapes and names; the numbers are the infrastructure pass's to set.
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
15. **Hoisting the repository-root test helper.** `repoRootPath` is declared independently in
    several packages' test binaries, and this task's own tests need it again. The helper moves into
    **one** shared test-helper package and is **exported** there: an unexported identifier is
    unreachable from another package's test binary, so the shared declaration necessarily carries a
    new, exported name. Every existing declaration is replaced by a call into it, and no declaration
    under the old unexported spelling survives — including in the package this task adds. Where that
    helper lives, and what its exported name is, are the design's call, under constraints it must
    satisfy: importable from the test binary of every
    package that needs it, `cmd/bot`'s included; importing it drags no machinery a consumer does
    not use into that consumer's test binary; and it is test-only, never linked into the bot
    command. Its package comment describes what it holds after the move.

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
  server, and graceful shutdown — #24. Scope 15 reaches `cmd/bot`'s test binary only, where it
  swaps a duplicated helper for the shared one; composition stays #24's.
- Product-dashboard work of any kind: no event type, no SQL view, no panel.
- Alert threshold values. The contract fixes the shapes and the names; the numbers are tuning.
- Provisioning the cloud-side canary bot: registering it with BotFather and getting its token into
  the deploy environment are the infrastructure pass's. This task defines the key the token arrives
  through and the behaviour when it is absent.

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
| Which labels are allowed | Bot API method name, Bot API method class, HTTP status code, scheduler task type, scheduler outcome, scheduler failure kind, ingest update kind, ingest outcome, canary leg, canary outcome, canary failure reason, pgx pool state. Every value set is bounded and never data-derived: an enum (the scheduler outcome and failure kind, the ingest update kind and outcome), the set of methods this module itself calls, the HTTP status-code space, the two canary legs, the canary's success-or-failure pair, and — for the canary failure reason — a status code where a response arrived plus a code-enumerated class where none did (timeout, cancellation, network failure). None is player-identifying, and none is taken from an error's text. The list is a ceiling, not an obligation. |
| Which labels are forbidden | Chat id, user or player id, update id, task id, operation id, raw error text, request URL, and any bot token in any form. Unbounded or player-identifying — a health series must not become a per-player series, and the product questions are the event log's. A classified canary failure reason is not error text: it is one of the enumerated classes the allowed row names, chosen from the error, never rendered from it. |
| Canary interval | A configuration key, optional-with-default, whose default is the design's once-a-minute cadence [source: 081213a:docs/DESIGN.md § 13.2 · sed -n '/^### 13.2\./,/^### 13.3\./p' docs/DESIGN.md]. |
| Canary cost against the rate limiters | Nil by construction: each leg gets its own client with its own limiter and a single attempt, so a canary neither draws on the production budget nor mixes into the production transport series. `getMe` classifies as `ClassOther` [source: 081213a:internal/tg/class.go § classifyMethod · sed -n '/^func classifyMethod/,/^}/p' internal/tg/class.go], whose windows are unbounded by default [source: 081213a:internal/config/transport.go § defaultTransport · sed -n '/^func defaultTransport/,/^}/p' internal/config/transport.go]. |
| Why one attempt, not the production retry policy | A retrying canary measures the retry loop, not the network: the failure it exists to show is masked by the retry and the latency it reports is mostly backoff. |
| The derived "own sick, cloud healthy" signal | Both legs export their own series; the cross-leg signal is a named expression in the alert contract, evaluated by the monitoring stack. The bot carries no second inference path, and the issue's own out-of-scope section asks this task to *name* the signals the alerting pass consumes. |
| Where the alert contract lives, and in which language | Under `ai-docs/`, in English, indexed in `ai-docs/agent-docs-index.md`. AGENTS.md restricts Russian to owner conversation and `docs/**`; the contract is an operations artefact the infrastructure pass reads, not part of the game-design corpus. |
| Handler panics | Already an ingest outcome (`OutcomePanic`), distinct from a plain failure. It reaches the dashboard as an outcome label value; this task adds no recovery logic of its own. |
| Idempotency hits | Already an ingest outcome (`OutcomeDuplicate`). Same treatment — duplicates are normal traffic and a spike is the signal. |
| Which credential the cloud canary leg uses | A **separate cloud-side bot token**, in its own optional configuration key — the test bot §12.5 already places on the cloud API. The production bot's cloud session stays closed as §12.2's runbook requires, and the leg's health is a real `getMe`: HTTP 200 carrying `ok:true`. Absent key, leg off. *Product owner, round-1 answer 1; see `## Source conflicts`.* |
| What schedules the canary probes | An **in-process ticker** on the configured interval, owned by this package: no task type, no persisted row, no scheduler coupling. A probe that stops when Postgres stops goes silent exactly during an incident, which is the ambiguity §13.2 exists to remove. *Product owner, round-1 answer 2.* |
| A disabled leg versus a failing one | A leg configured off exports no probe sample at all, and the alert contract names the absence test the monitoring stack applies. A leg that is off must never read as a leg that is healthy. |
| The cloud leg's base URL | A configuration value defaulting to the cloud API, not a compiled-in literal. §11 already makes the Bot API base URL a config value on a three-value axis — own instance, cloud, fake server [source: 776b987:docs/DESIGN.md § 11 · sed -n '/^## 11\./,/^## 12\./p' docs/DESIGN.md] — and a hard-wired endpoint would put the probe out of a fake server's reach. |
| The duplicated repository-root test helper | Hoisted by this task into one shared test-helper package, with every existing declaration replaced by a call into it. The alternative — a follow-up issue leaving this task's approved scope untouched — was offered and declined. *Product owner, design-review round 3.* |
| How far the `cmd/` relaxation goes | To `cmd/bot`'s test binary, and within it to the helper swap alone. AC29 exists because #24 owns `cmd/bot` composition; this decision transfers no composition, wiring, start-up or shutdown to this task, and a general licence to modify `cmd/` was neither granted nor implied. |
| One interval, both legs | A single cadence value drives both probes on the same tick. Reading a sick leg against a reference leg is the whole point, and two independent cadences turn that comparison into a question about sampling. |

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
10. **The canary ticker is independent of Postgres and of the scheduler.** It registers no task
    type, writes no row, shares no state with `internal/scheduler`, and keeps probing while the
    database is unreachable. Its lifecycle is an explicit start and shutdown the caller owns, and a
    shutdown is honoured without waiting out the current interval.
11. **A disabled leg is silent, never healthy.** With no cloud token configured, the cloud leg
    produces no probe sample of any kind, and nothing in the exported set lets its absence be read
    as a success.
12. **The cloud canary token is a secret.** It is carried in the config package's existing secret
    type, so a formatted rendering of a configuration value redacts it; it is never a metric name,
    a label, or a label value.
13. **The own-instance leg carries the production bot token against the configured instance base
    URL** — the chain the design asks it to exercise — and opens no session anywhere else.
14. **Configuration keeps its existing shape**: keys read only by `internal/config`, every added key
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

**Resolved by the product owner, round-1 answer 1: the separate cloud-side bot.** The cloud leg
carries its own cloud-side bot token in its own optional configuration key — the bot §12.5 already
places on the cloud API — so §13.2's reference probe exists without ever reopening the production
bot's cloud session, and §12.2's runbook keeps its meaning unchanged. The leg's success condition
is a real `getMe`: HTTP 200 carrying `ok:true`. With the key absent the leg does not run at all,
rather than running degraded. No site of the design was rewritten to reach this: §13.2 asked for a
cloud-side reference probe, and it gets one.

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
| AC15 | The own-instance leg issues `getMe` with the production bot token against the configured instance base URL. |
| AC16 | The cloud leg issues `getMe` with a cloud-side bot token taken from its own configuration key, against a cloud base URL that is itself a configuration value; the leg counts a probe successful only on an HTTP 200 response carrying `ok:true`. |
| AC17 | With the cloud leg's token key absent, configuration loads without error, the own-instance leg still runs, and no cloud-leg probe sample of any kind is exported. |
| AC18 | A canary probe makes exactly one attempt per tick, and no canary call contributes a sample to the transport series of AC9. |
| AC19 | A canary failure is classified by HTTP status code or by transport-error class, and no canary series carries raw error text as a label value. |
| AC20 | The canary cadence is a single configuration value driving both legs on the same tick, whose compiled-in default is the design's once-a-minute cadence. |
| AC21 | The canary runner registers no scheduler task type, writes no database row, and continues probing while the database is unreachable; its shutdown entry point returns without waiting out the current interval. |
| AC22 | The metrics listen address is a configuration value whose compiled-in default binds the loopback interface only. |
| AC23 | Every label on every metric this task adds belongs to the allowed set enumerated in Key decisions, and no exported metric carries a chat id, a user or player id, an update id, a task id, an operation id, raw error text, a URL, or any part of any bot token as a name, a label or a label value. |
| AC24 | The cloud canary token is carried in the config package's secret type, so a default-verb rendering of the value holding it yields that type's redaction placeholder; the token reaches no log line, no metric name, no label and no label value. |
| AC25 | Every environment variable this task adds is returned by the config package's exported key enumeration and documented in `.env.example`, and the config package's manifest/loader/declared-set identity check passes with them present. |
| AC26 | An alert-contract document exists under `ai-docs/`, in English, naming for each alert: the metric names it reads, the expression over them, the condition shape, and the severity — covering at minimum the consecutive-canary-failure alert, the update-lag-growth alert, the cross-leg "own instance sick while cloud healthy" expression, what that expression means when the cloud leg is configured off, and the absence test distinguishing a disabled leg from a failing one. |
| AC27 | `ai-docs/agent-docs-index.md` lists the alert-contract document. |
| AC28 | `github.com/prometheus/client_golang` is a require entry of `go.mod`, and module tidiness leaves `go.mod` and `go.sum` unchanged. |
| AC29 | Under `cmd/`, this task modifies `cmd/bot/main_test.go` and no other file, and the only change to that file is the replacement of its own `repoRootPath` declaration with a call into the shared test helper; no production file under `cmd/` changes, and no composition, wiring, start-up or shutdown behaviour moves into this task. |
| AC30 | Every exported item added by this task carries a doc comment beginning with its name, and every new package carries a package comment. |
| AC31 | No comment added by this task names a markdown path, a design section number, an acceptance-criterion id, a repository path, a URL, an issue number outside `TODO(#…)`, or a package-qualified symbol of this module outside its own package. |
| AC32 | Production code added by this task contains no `panic` and no `log.Fatal`; a bind failure and a probe failure are both reported through a returned error. |
| AC33 | Every gate in AGENTS.md § *Build & Test* is green on the branch, including the race gate and the coverage ratchet at its recorded high-water mark or above. |
| AC34 | Every live site in the repository whose claim this diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4. |
| AC35 | Exactly one declaration of the repository-root path helper exists in the module — in a shared test-helper package, under an exported name, since an unexported one is unreachable from another package's test binary — and no declaration under the former unexported spelling `repoRootPath` survives anywhere in the module, including in the package this task adds. Every package that resolves a repository-root path in its tests calls the shared declaration. |
| AC36 | No non-test file in the module imports the shared test-helper package, so the helper never links into the bot command. |

## Open questions

- **An in-process gauge for the cross-leg derived signal.** The decision above keeps the inference
  in the monitoring stack and the bot's output at two independent legs. Revisit if the
  infrastructure pass finds the expression awkward to write against scrape gaps.
- **Histogram bucket boundaries per family.** Transport latency lives in milliseconds and update
  lag in seconds-to-minutes, so one bucket set cannot serve both. The design's call, and cheap to
  change later since buckets are not persisted.
- **Alert threshold values** — how many consecutive canary failures, and what rate of update-lag
  growth. The contract states the shapes; the infrastructure pass sets the numbers.
- **Whether the cloud canary bot is §12.5's test bot or a third one.** The infrastructure pass
  provisions it either way; this task only names the key the token arrives through and the
  behaviour when it is absent.
- **A readiness endpoint beside `/metrics`.** Not asked for by the design; #24 may want one when it
  owns start-up and shutdown.
- **The `telegram-bot-api` `/stats` shape** is out of scope here, but the alert contract may want to
  name the series the infrastructure pass will scrape from it. Left to that pass, which owns the
  instance's configuration.
