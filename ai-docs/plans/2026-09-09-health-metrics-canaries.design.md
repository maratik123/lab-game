# Design: Health metrics and canaries — `/metrics`, update and scheduler lag, dual `getMe`

**Issue:** #23
**Date:** 2026-09-09
**Amended:** 2026-09-09 — round 2, against the spec at 91a5515. The amendment authorises the
canary `outcome` and `reason` labels this design already carried (D6), so nothing in the
catalogue moves. The round-1 findings are folded in: the observation-field register becomes
normative and moves into the package's own doc comments (D16); D3's panic analysis is corrected —
it named the wrong surface and missed the one D7 creates; D6 stops mis-citing AC10/AC11 and states
the divergence as a divergence; and D10 names the endpoint's path. The recommendations are
taken: D13 owns KD-27's substantive clause, the ingest counter is renamed to what it counts, and
D5 states the `kind` exemption where the rule is. No AC changed and no scope moved.
**Amended:** 2026-09-09 — round 3. D11 gains the credential-to-endpoint pairing AC15 and AC16
turn on, which round 2 left unstated and unasserted; D12's scrape guard is re-keyed per
`(family, label)` because one label *name* spans disjoint value sets, and gains a panic-source
guard; D13's KD-27 amendment is narrowed to the two keys the spec authorises; D16's
unreachability claim is narrowed to the two enums that own it. No AC changed and no scope moved.
**Amended:** 2026-09-09 — round 4. The owner directed that the test-helper hoist be taken in this
task rather than deferred to a follow-up. D17 designs that package and D18 routes the Go doc comments this change falsifies;
the decomposition grows a leading subtask and the handoff plan grows to three groups. Round 3's
positive claim that a handler can make `unknown` an observed scheduler `outcome` is **withdrawn
as false** (D6). D9 names the sink for a collector error and D12's source walk is extended to the
library's `Must…` spellings, which the round-3 § Risks row wrongly claimed were already covered.
**Amended:** 2026-09-09 — round 5. The shared declaration is settled as **exported**, since an
unexported one is unreachable from another package's test binary. This round also pays the
propagation debt the round-4 widening left: two sentences still cited a superseded AC29 wording to
claim `cmd/` was untouched. Group A/B is
rebalanced so the canary work does not run last in the most-degraded context, D13 decides the
example-file placeholder trap, and guard (g) gains the detection rule it was missing.
**Amended:** 2026-09-09 — round 6. The hoist's scope was measured by **identifier** while the
class it belongs to is defined by **behaviour**, so two live repository-root resolvers were missed; D19 records
the widened set and the lesson, subtask 1 takes all of them, and D18's routing extends to the doc
comments the widened move falsifies. D17 settles the two call-site signatures. Guard (g)'s
detection rule is widened to the semantic class — one of the missed sites uses an ascent spelling
the round-5 rule provably could not see — and is validated against the pre-change tree rather
than only against scratch files it was written to catch.
**Amended:** 2026-09-09 — round 7. The helper class is defined by **mechanism** rather than by
outcome, which resolves the round-6 blocker without an exception list. D19
withdraws a false claim it made in the same breath as diagnosing the same error — that
`runtime.Caller` is coextensive with the resolver class — and records the durable form of the
lesson. Guard (g) is scoped to the mechanism class explicitly and its restore list is corrected.
Every discriminating proof moves off the working tree onto a `t.TempDir()` copy, which removes a
guard-against-guard collision and a dirty-tree failure mode the design had stated the halves of
but never joined.
**Amended:** 2026-09-09 — round 8, the last, and a standing sweep rather than a redesign. Spec
d63c7a1 removes the acceptance criteria and the scope item that had encoded the test-helper
hoist: where the helper lives, what it is called and which guard checks it are design and code,
never acceptance. **The hoist itself does not move** — D17, subtask 1, `internal/repotest`, the
derived member set and guard (g) all stand exactly as designed. What changes is their standing:
they are this design's own decision and this design's own test, carrying their own rationale
rather than discharging a criterion. Every citation of the removed criteria is gone, the one
passage that existed only to reconcile with their wording is deleted rather than re-anchored, and
AC29's remaining citations are re-read against its minimal text.

## Approach

One new package, `internal/health`, holds the whole health-metrics surface: the registry
constructor, the metric families, the adapters implementing each consumer-declared observer
seam, the pgx-pool collector, the `promhttp` endpoint's server, and the canary probes with their
ticker. Nothing in `internal/tg`, `internal/scheduler` or `internal/ingest` changes, because
each of those already declares the seam this package plugs into and each documents a nil observer
as checked rather than called
[measured 9ff1fce:internal/ingest/observe.go:119-127 · `sed -n '119,127p' internal/ingest/observe.go`
→ `// Observer receives this package's observations. A nil Observer is` / `// checked, not called, so
the loop compiles and its` / `// tests pass with no implementation installed.`]. Installing an
implementation is therefore purely additive, and this task changes no signature in any of them.

Nothing here assembles a process. **No production file under `cmd/` changes** (AC29); the single
`cmd/` edit this task makes is subtask 1's helper swap in `cmd/bot/main_test.go`, which is a test
file and moves no behaviour. `cmd/bot` itself still loads and validates
configuration and prints the build identity
[measured 9ff1fce:cmd/bot/main.go:29-41 · `sed -n '29,41p' cmd/bot/main.go` → `func run(lookup
config.Lookup, stderr, stdout io.Writer) int {` whose body is `config.Load`, a `Fprintf` to stderr on
failure and a `Fprintf` of `version` to stdout otherwise]. Every component this package exports is
constructible in isolation with an explicit lifecycle, which is exactly what #24 needs to wire.

### Why one package, and why these seams

The observer interfaces are consumer-declared and satisfied implicitly, so a single package
can satisfy them all without any of them importing it. Splitting the health surface into
`internal/metrics` + `internal/canary` would produce two packages that only ever import each other
and the same registry — the shape the update-ingestion design already rejected for the same
reason [measured 9ff1fce:ai-docs/key-decisions.md:77 · `rg -n -o 'whose sub-packages would only
ever import each other' ai-docs/key-decisions.md` → `77:whose sub-packages would only ever import
each other`]. `internal/health` imports `internal/tg`, `internal/scheduler`, `internal/ingest`,
`internal/config` and `github.com/jackc/pgx/v5/pgxpool`; none of those imports it, so there is no
cycle. **`internal/ingest` and `internal/tg` structurally refuse the metrics library themselves**,
each with a package-scoped guard test, and this task must leave both green and untouched rather than
relax either [measured 9ff1fce:internal/ingest/guards_test.go:97-104 and
internal/tg/guards_test.go:101-103 · `rg -n 'client_golang' internal/ingest/guards_test.go
internal/tg/guards_test.go` → `100: if strings.Contains(string(content),
"prometheus/client_golang") {` erroring `internal/ingest must expose only the Observer interface`,
and the same shape erroring `internal/tg must expose only the Observer interface`; each walks its
own package's non-test files only, so a sibling package importing the library does not trip it].
That is independent confirmation that the exposition belongs here and nowhere else. It
deliberately does **not** import `internal/store`: the pool collector takes an accessor
function rather than a pool or a store handle, which keeps the health test binary free of a database
(D9).

### Why `github.com/prometheus/client_golang`

`docs/DESIGN.md` §13.5 names promhttp for the endpoint, and promhttp is that module's subpackage
[measured 9ff1fce:docs/DESIGN.md · `rg -n promhttp docs/DESIGN.md` → the §13.5 bullet «здоровье:
**Prometheus** скрейпит `/metrics` бота (promhttp) и `/stats` инстанса bot-api»];
`AGENTS.md` § *Dependency Versions* makes an established ecosystem package the default and refuses
hand-rolling in its place. The version taken is the newest published
[measured 9ff1fce · `go list -m -versions github.com/prometheus/client_golang` → the version list
ending `v1.24.0-rc.0 v1.24.0 v1.24.1`]. Its build closure, resolved in a scratch module importing
`prometheus`, `collectors`, `promhttp` and `testutil`, adds `beorn7/perks`, `kylelemons/godebug`,
`munnerz/goautoneg`, `prometheus/client_model`, `prometheus/common`, `prometheus/procfs` and
`google.golang.org/protobuf`, and reuses `cespare/xxhash/v2` and `golang.org/x/sys`, which this
module already requires
[measured 9ff1fce · `go mod tidy && cat go.mod` in a scratch module requiring
`github.com/prometheus/client_golang v1.24.1` → a require block naming `beorn7/perks`,
`cespare/xxhash/v2`, `kylelemons/godebug`, `munnerz/goautoneg`, `prometheus/client_model`,
`prometheus/common`, `prometheus/procfs`, `golang.org/x/sys`, `google.golang.org/protobuf`; and
`grep -n 'xxhash\|golang.org/x/sys' go.mod` in this repository → `github.com/cespare/xxhash/v2
v2.3.0 // indirect` and `golang.org/x/sys v0.47.0 // indirect`]. That closure is the expected shape,
not the contract: the binding statement is the `git diff go.mod go.sum` the implementor reads before
staging (AC28, subtask 2).

*Rejected:* hand-rolling an exposition format. The Prometheus text format has a specification, an
escaping rule and a histogram layout this project would then own and test forever, and `promhttp` is
named by the design document itself — and `AGENTS.md`'s refused-argument table already answers both
arguments that would be offered for writing one [measured 9ff1fce:AGENTS.md:196-197 · `sed -n
'196,197p' AGENTS.md` → `| *"It's only 10–20 lines — cheaper than writing the import"* | **REFUSED.**
…` and `| *"Better to write our own than to pull in an established dependency"* | **REFUSED.** …`]. *Rejected:* `VictoriaMetrics/metrics`
— smaller, but it has no `promhttp`, no `collectors` package for the Go-runtime and process series
AC13 asks for, and no `testutil`/`promlint` for the naming gate D12 builds on. *Escape hatch:* every
registration in this package goes through a `prometheus.Registerer` parameter, so the registry type
is one import away from being swapped behind the same constructors.

### Rejected alternatives, structural

*Rejected — registering on the client library's global default registerer, or using `promauto`.*
Both make a registration invisible at the call site, collide between tests in the same binary, and
make an isolated registry impossible. AC2 forbids both, and D12 gates the forbidding.

*Rejected — the canaries as scheduler tasks.* Settled by the product owner (spec, round-1 answer 2)
and by the failure mode: a probe that stops when Postgres stops goes silent exactly during the
incident it exists to localise.

*Rejected — an in-process gauge for the cross-leg "own sick, cloud healthy" signal.* Settled in the
spec's Key decisions: both legs export their own series and the inference is an expression in the
alert contract, evaluated by the monitoring stack.

*Rejected — the aggregate constructor.* An earlier shape had one `New()` returning a struct holding
the registry and every adapter. It is dropped: AC1 requires every family to register on a
**supplied** registerer, #24 owns "one place where every subsystem is constructed and injected", and
an aggregate here would be a second, competing composition root. Every constructor takes a
`prometheus.Registerer` and returns its own component.

## Key decisions

**D1 — `internal/health`, one package, one file per component.** Files: `doc.go` (the package
comment, which carries the observation-field register of D16 and the field-disposition table
behind it), `registry.go` (`NewRegistry`, `RegisterRuntime`,
the name prefix, the bucket variables), `labels.go` (label-name constants, the enum label-value
mappers, the allow-list), `transport.go`, `scheduler.go`, `ingest.go`, `pool.go`, `server.go`,
`canary.go` (the runner and the `Prober` seam), `probe.go` (the Telegram prober, its status recorder
and the failure classifier). The split keeps every file well inside the workspace's soft band and
gives each `_test.go` an obvious home [derived → the file-limits gate in subtask 11].

**D2 — the registry is constructed empty and every registration takes a `prometheus.Registerer`.**
`NewRegistry() *prometheus.Registry` returns a vanilla registry with nothing on it;
`RegisterRuntime(reg prometheus.Registerer) error` puts the client library's Go-runtime and process
collectors on whatever registerer it is handed. Every other constructor takes a `Registerer` as its
first parameter. This is what makes AC1's second clause literally true of *every* family, including
the library's own, and it is what lets a test build an isolated registry per case. The library
supplies both collectors
[measured 9ff1fce · `go doc github.com/prometheus/client_golang/prometheus/collectors` in a scratch
module → `func NewGoCollector(opts ...func(o *internal.GoCollectorOptions)) prometheus.Collector` and
`func NewProcessCollector(opts ProcessCollectorOpts) prometheus.Collector`].

**D3 — no `MustRegister`, no `MustNewConstMetric`, and `promhttp.HandlerOpts.Registry` stays nil.**
`MustRegister` panics where `Register` returns an error
[measured 9ff1fce · `go doc github.com/prometheus/client_golang/prometheus.Registerer` in a scratch
module → `// MustRegister works like Register but registers any number of` / `// Collectors and
panics upon the first registration that causes an` / `// error.`, beside `Register(Collector)
error`, whose own doc names `AlreadyRegisteredError` as the re-registration outcome]. `promhttp.HandlerOpts.Registry` is documented as panicking on a failed
registration [measured 9ff1fce · `go doc
github.com/prometheus/client_golang/prometheus/promhttp.HandlerOpts` in a scratch module → `// If
Registry is not nil, it is used to register a metric` … `// A failed registration causes a panic.`],
so this package leaves it nil and does not export promhttp's own error counter. The pool collector
builds its metrics with `prometheus.NewConstMetric`, which returns an error rather than panicking
[measured 9ff1fce · `go doc github.com/prometheus/client_golang/prometheus.NewConstMetric` in a
scratch module → `func NewConstMetric(desc *Desc, valueType ValueType, value float64, labelValues
...string) (Metric, error)`]. The project's panic index is empty and the project targets zero
production panics [measured 9ff1fce:ai-docs/panic-index.md · `sed -n '/^| File:line/,$p'
ai-docs/panic-index.md` → a header row and a `| — | — | — |` body row], and this task adds no row to
it (AC32).

**Round 1 of this design claimed the only remaining panic surface was `…Vec.WithLabelValues`'s
cardinality check. That was wrong, and the review's correction is confirmed at the source.**
The round-1 evidence was `CounterVec.GetMetricWithLabelValues`'s doc generalised to every `Vec`,
and it missed the surface D7 itself creates. There are **two** surfaces, and the histogram one is
the dangerous one:

*Surface one — histogram buckets.* `newHistogram` panics on a bucket slice that is not strictly
increasing, and on an `le` label in either the variable or the const label set
[measured 9ff1fce · `sed -n '539,596p' $(go env
GOMODCACHE)/github.com/prometheus/client_golang@v1.24.1/prometheus/histogram.go` → `func
newHistogram(desc *Desc, opts HistogramOpts, labelValues ...string) Histogram {` whose body
panics with `makeInconsistentCardinalityError(...)`, twice with `errBucketLabelNotAllowed`, and
with `fmt.Errorf("histogram buckets must be in increasing order: %f >= %f", ...)`; and `rg -n
'bucketLabel\s*=' …/prometheus/histogram.go` → `265:const bucketLabel = "le"`]. **For a
`HistogramVec` that call is lazy**: `NewHistogramVec` closes `newHistogram` into the vec's
`newMetric` function, so the panic fires at the first observation of a label combination — on the
ingest loop's or the scheduler worker's own goroutine, in production
[measured 9ff1fce · `sed -n '1183,1205p' …/prometheus/histogram.go` → `return &HistogramVec{`
`MetricVec: NewMetricVec(desc, func(lvs ...string) Metric {` `return newHistogram(desc,
opts.HistogramOpts, lvs...)` `}),`]. For a plain `NewHistogram` it fires at construction, and its
own doc comment says so [measured 9ff1fce · `sed -n '520,527p' …/prometheus/histogram.go` → `//
NewHistogram creates a new Histogram based on the provided HistogramOpts. It` / `// panics if the
buckets in HistogramOpts are not in strictly increasing order.`]. The bucket-builder helpers panic
on bad arguments too, which for a package-level variable means package init
[measured 9ff1fce · `rg -n 'panic\(' …/prometheus/histogram.go` → `297: panic("LinearBuckets
needs a positive count")`, `317`/`320`/`323` for `ExponentialBuckets`, `341`/`344` for
`ExponentialBucketsRange`].

**Two precisions the finding did not carry, both verified, and both change the mitigation.**
First, `GetMetricWithLabelValues` does **not** shield the bucket panic: it returns an error only
from its own hash/cardinality step and then calls the same `newMetric` closure, so switching
spelling buys nothing here [measured 9ff1fce · `sed -n '214,222p' $(go env
GOMODCACHE)/github.com/prometheus/client_golang@v1.24.1/prometheus/vec.go` → `h, err :=
m.hashLabelValues(lvs)` / `if err != nil {` / `return nil, err` / `}` / `return
m.getOrCreateMetricWithLabelValues(h, lvs, m.curry), nil`]. Second, an **empty** bucket slice does
not panic at all — it is silently replaced by `DefBuckets`
[measured 9ff1fce · `sed -n '573,575p' …/prometheus/histogram.go` → `if len(h.upperBounds) == 0 &&
opts.NativeHistogramBucketFactor <= 1 {` / `h.upperBounds = DefBuckets` / `}`], so an empty
update-lag variable would measure a stall indicator in the default millisecond band and report
nothing wrong. That is a silent-wrong-measurement defect, not a crash, and it needs the same test
for a different reason. D7 carries the mitigation.

*Surface two — `…Vec.WithLabelValues`'s cardinality check, which is used deliberately.*
`WithLabelValues` panics where `GetMetricWithLabelValues` returns an error, and for a histogram
that check is `newHistogram`'s own first guard rather than only `CounterVec`'s documented one
(both cited above). Every call site in this package passes a literal argument list against a
`Desc` declared in the same file, so a mismatch is a compile-adjacent mistake rather than a
runtime input, and D12's scrape guard drives every family through its adapter, so it reds at the
first run rather than in production. Neither surface is a `panic` call in this module's code, so
neither adds a panic-index row; the alternative — threading a cardinality error out of
`ObserveCall`, whose signature returns nothing and which is documented as running on the caller's
goroutine — has nowhere to go and would land as a swallowed error, which `AGENTS.md` § *Code
Style* forbids outright.

**D4 — the name prefix is `labgame_`, and every family is named in the catalogue below.** One
module-wide prefix, base units, `_total` on counters, `_seconds` on duration histograms
(spec constraint 5). The Go-runtime and process collectors keep the library's own `go_` and
`process_` prefixes, which is what a Prometheus dashboard expects of them.

**D5 — label-value strings are this package's, mapped in an exhaustive switch, never delegated to a
source package's `String()`.** A label value is a monitoring-facing data contract that the alert
contract names and a dashboard queries; a Go constant may be renamed under `AGENTS.md` § *API
Stability*, and a renamed constant must not silently rename a series. `internal/scheduler` declares
no renderer at all for either of its enums
[measured 9ff1fce:internal/scheduler · `go doc github.com/maratik123/lab-game/internal/scheduler`
→ `type FailureKind int` and `type Outcome int` each followed by its `const` line and then the next
type, with no method listed under either],
and `ingest.Outcome`'s renderer emits Go identifier names rather than label values
[measured 9ff1fce:internal/ingest/observe.go:48-65 · `sed -n '48,65p' internal/ingest/observe.go` →
`func (o Outcome) String() string {` returning `"OutcomeHandled"`, `"OutcomeDuplicate"` and the
rest]. Each mapper is a `switch` with a `default` returning a single named `unknown` value, which
keeps the value set closed however the source enum grows; `default-signifies-exhaustive` is set, so
the `exhaustive` linter is satisfied by that clause
[measured 9ff1fce:.golangci.yml:40-41 · `sed -n '40,41p' .golangci.yml` → `exhaustive:` /
`default-signifies-exhaustive: true`].

*The `kind` label is the one exemption to this rule, and it is stated here rather than left to be
noticed in D6.* `ingest.Kind`'s values are the Bot API's own update-type tokens — an external
vocabulary this project does not own and cannot rename — mirroring telego's constants, with a
drift check that reflects over `telego.Update` and reds when the two part company
[measured 9ff1fce:internal/ingest/kind.go:9-11 and internal/ingest/guards_test.go:139 · `sed -n
'9,11p' internal/ingest/kind.go; rg -n 'func TestGuard_TelegoUpdateFieldsMatchKindTable'
internal/ingest/guards_test.go` → `// Kind identifies one Bot API update type — the exact tokens`
/ `// GetUpdatesParams.AllowedUpdates and telego's own "Update types you want` / `// your bot to
receive" constants use.`, and `139:func TestGuard_TelegoUpdateFieldsMatchKindTable(t *testing.T)
{`]. So taking the value verbatim delegates the series name to the Bot API, not to a Go constant,
and a change to it would be a Bot API change rather than a refactor. **The residual is real and
named:** `Kind` is a string type, so nothing structurally prevents someone editing a token; that
drift check is what makes such an edit visible, and it lives in `internal/ingest`, not here.

**D6 — the metric catalogue.** Every family this task adds, with its labels and its source field.

*Transport, from `tg.Observation` (AC4, AC9):*

| Family | Type | Labels | Source |
|---|---|---|---|
| `labgame_botapi_call_duration_seconds` | histogram | `method` | `Latency` |
| `labgame_botapi_responses_total` | counter | `method`, `code` | `StatusCode`, rendered decimal; `0` means no response was received |
| `labgame_botapi_rate_limited_total` | counter | `method` | `RateLimited` |
| `labgame_botapi_retries_total` | counter | `method` | `Retries`, added as a count |

Every field of `tg.Observation` reaches a metric, so this package's register names no transport
exemption. `Observation` has no method-class field and `classifyMethod` is unexported
[measured 9ff1fce:internal/tg/class.go:46 · `sed -n '46p' internal/tg/class.go` → `func
classifyMethod(name string) MethodClass {`], so the class label the spec's allow-list permits is not
carried: the allow-list is a ceiling, not an obligation, and reaching that value would need a
signature change spec constraint 1 forbids.

*Scheduler, from `scheduler.Observation` and `scheduler.LoopObservation` (AC5, AC8, AC11):*

| Family | Type | Labels | Source |
|---|---|---|---|
| `labgame_scheduler_task_lag_seconds` | histogram | `type` | `Observation.Lag` |
| `labgame_scheduler_tasks_total` | counter | `type`, `outcome`, `failure` | `Outcome`, `Failure` |
| `labgame_scheduler_loop_duration_seconds` | histogram | — | `LoopObservation.Duration` |
| `labgame_scheduler_claim_batch_size` | histogram | — | `LoopObservation.BatchSize` |
| `labgame_scheduler_loop_errors_total` | counter | — | `LoopObservation.Err != nil` |

`outcome` values: `done`, `noop`, `failed`. `failure` values: `none`, `handler`, `unregistered`,
`deadline`, `rolled_back` — one per member of `scheduler.FailureKind`, which is the set AC11
requires. Each mapper additionally declares an `unknown` branch; D16 states why that is not a
widening of AC11, and which of the two branches is genuinely unreachable. The `failure` one is:
every `FailureKind` an observation can carry is set by the worker itself. The `outcome` one is
**not**, and the difference matters — `scheduler.Outcome` arrives from the consumer-declared
`Handler`, and the worker passes it through to the observation without normalising it
[measured d8ead96:internal/scheduler/execute.go:226 and :173 · `rg -n 'handler.Execute\(ctx|
outcome := r.outcome' internal/scheduler/execute.go` → `226: outcome, handlerErr =
handler.Execute(ctx, tx, task)` and `173: outcome := r.outcome`, the value then assigned to
`obs.Outcome`]. **Round 3 concluded from that pass-through that a handler returning an
out-of-range value can make `unknown` an observed scheduler `outcome`. That is false, and it is
withdrawn.** The value never reaches an observation: `settleOutcome` runs first and its `default`
branch refuses an unknown outcome, and the caller rolls back and returns on that error, before
either the assignment or the observe call
[measured 9396d3c:internal/scheduler/execute.go:186-188,204,209 and internal/scheduler/settle.go:190-196 ·
`sed -n '186,188p;204p;209p' internal/scheduler/execute.go; sed -n '190,196p'
internal/scheduler/settle.go` → `if err := settleOutcome(ctx, tx, task, decl, outcome,
handlerErr, w.cfg); err != nil {` / `_ = tx.Rollback(ctx)` / `return fmt.Errorf("scheduler:
settle task %d: %w", task.ID, err)`, with `obs.Outcome = outcome` and `w.observeTask(obs)` below
it, and `switch outcome {` … `default:` / `return fmt.Errorf("scheduler: unknown outcome %d",
outcome)`]. Every other site that observes sets `OutcomeFailed` explicitly first, so no path
emits an out-of-range scheduler outcome. **The branch exists for totality, not for a reachable
case:** `exhaustive` requires the switch to be total, and the branch keeps the value set closed
however the source enum grows. What this design still declines to do is *claim* unreachability
for `scheduler.Outcome` the way it does for the two enums the ACs bind — that guarantee rests on
another package's internal control flow rather than on a contract, and D16 keeps the conservative
framing. Nothing downstream moves: D12's table already asserts the observed set `done`, `noop`,
`failed`.

*Per-field ruling, `scheduler.Observation` (AC5).* `Type`, `Lag`, `Outcome` and `Failure` reach
the families above. The other two are **deliberately unexported to metrics**, and the reason is
stated in the package's own documentation per D16, not only here:

- `BatchSize` — an alias, not an omission. It is the discovery cardinality of the cycle the task
  came from, so it repeats once per task in a batch; `labgame_scheduler_claim_batch_size` takes
  the same number once per cycle from `LoopObservation.BatchSize`. Exporting it here as well
  would weight the distribution by batch size and make a large batch look like many large
  batches.
- `ConsecutiveFailures` — exported by no family, and this is the ruling the review was right to
  demand rather than let pass silently, because the field is not a throwaway: its own doc comment
  makes it the only truthful failure count for a `FailureDeadline` observation, whose settlement
  is deferred [measured 9ff1fce:internal/scheduler/observe.go:47-51 · `sed -n '47,51p'
  internal/scheduler/observe.go` → `// ConsecutiveFailures is the value the settlement will write
  —` / `// computed from the row's failure count read at claim time, not` / `// re-read, so it is
  truthful even for a FailureDeadline observation` / `// whose settlement is deferred.`]. It is
  still not a *health series*: it is a per-row property, so a gauge of it is last-write-wins
  across concurrently executing tasks and means nothing at scrape time, and a histogram of it
  would double-count a row that fails repeatedly. What the dashboard actually needs — "tasks are
  retrying, and by kind" — is already the `failure`-labelled counter, and "this specific row is
  stuck" is a per-row question the dead-task listing answers. Revisit if an operator ever needs
  retry *depth* rather than retry *volume*; that would be a histogram keyed by `type`, and it is
  a change to this ruling, not a gap in it.

*Per-field ruling, `scheduler.LoopObservation` (AC5).* `Duration`, `BatchSize` and `Err` each
reach a family above; `Err` reaches it as a presence, never as text.

*Ingest, from `ingest.Observation` and `ingest.LoopObservation` (AC6, AC7, AC10):*

| Family | Type | Labels | Source |
|---|---|---|---|
| `labgame_ingest_update_lag_seconds` | histogram | `kind` | `Lag`, sampled only when `LagKnown` |
| `labgame_ingest_handler_duration_seconds` | histogram | `kind`, `outcome` | `Duration` |
| `labgame_ingest_update_outcomes_total` | counter | `kind`, `outcome` | one per observation |
| `labgame_ingest_undecodable_updates_total` | counter | — | `Err != nil` |
| `labgame_ingest_poll_duration_seconds` | histogram | — | `LoopObservation.Duration` |
| `labgame_ingest_poll_batch_size` | histogram | — | `LoopObservation.BatchSize` |
| `labgame_ingest_poll_errors_total` | counter | — | `LoopObservation.Err != nil` |

**The counter is named for what it counts.** An `ingest.Observation` is reported once per handler
attempt plus once per attempt-less settlement, so a retried update contributes several
[measured 9ff1fce:internal/ingest/observe.go:67-70 · `sed -n '67,70p' internal/ingest/observe.go`
→ `// Observation is reported to an Observer once per handler call (attempt),` / `// plus once for
each attempt-less settlement — unrouted and given-up:` / `// a single terminal observation per
update would erase a` / `// panic that a later attempt recovered from.`]. A family called
`…_updates_total` would read as a per-update count and quietly overstate traffic under retry;
`…_update_outcomes_total` says what the series is, and promlint has no opinion either way. The
same multiplicity applies to the lag histogram and is carried in § Open questions.

`outcome` values: `handled`, `duplicate`, `unrouted`, `failed`, `panic`, `given_up` — exactly the
members of `ingest.Outcome`, which is the set AC10 requires, and which is what makes recovered
panics and idempotency duplicates individually visible. An `unknown` branch is declared here too,
and unlike the scheduler's `outcome` mapper this one really is unreachable — `ingest.Outcome` is
set by the loop alone, never by a consumer; D16 states what follows. `kind` carries
the derived kind verbatim; the empty kind an unrouted update leaves behind maps to the same named
`unknown` value, so no series carries an empty label value — and that mapping IS reachable, since
an unrouted update genuinely carries the zero `Kind`, so `unknown` is an observed `kind` value
rather than a declared-only one. `Duration` means a different thing per
outcome [measured 9ff1fce:internal/ingest/observe.go:79-89 · `sed -n '79,89p'
internal/ingest/observe.go` → `// Duration is how long this observation's own unit of work took:` …
`For OutcomeUnrouted, no handler ever runs, so Duration is the` / `// unrouted-settlement statement
instead. For OutcomeGivenUp, no` / `// handler runs either`], which is why `outcome` is a label on
that histogram rather than folded away.

*pgx pool, from `pgxpool.Stat` (AC12):*

| Family | Type | Labels | Source |
|---|---|---|---|
| `labgame_pgxpool_conns` | gauge | `state` (`idle`, `acquired`, `constructing`) | `IdleConns`, `AcquiredConns`, `ConstructingConns` |
| `labgame_pgxpool_total_conns` | gauge | — | `TotalConns` |
| `labgame_pgxpool_max_conns` | gauge | — | `MaxConns` |
| `labgame_pgxpool_acquires_total` | counter | — | `AcquireCount` |
| `labgame_pgxpool_acquire_duration_seconds_total` | counter | — | `AcquireDuration` |
| `labgame_pgxpool_empty_acquires_total` | counter | — | `EmptyAcquireCount` |
| `labgame_pgxpool_empty_acquire_wait_seconds_total` | counter | — | `EmptyAcquireWaitTime` |
| `labgame_pgxpool_canceled_acquires_total` | counter | — | `CanceledAcquireCount` |
| `labgame_pgxpool_new_conns_total` | counter | — | `NewConnsCount` |
| `labgame_pgxpool_max_lifetime_destroys_total` | counter | — | `MaxLifetimeDestroyCount` |
| `labgame_pgxpool_max_idle_destroys_total` | counter | — | `MaxIdleDestroyCount` |

The whole `Stat` surface is covered [measured 9ff1fce · `go doc
github.com/jackc/pgx/v5/pgxpool.Stat` → the method set `AcquireCount`, `AcquireDuration`,
`AcquiredConns`, `CanceledAcquireCount`, `ConstructingConns`, `EmptyAcquireCount`,
`EmptyAcquireWaitTime`, `IdleConns`, `MaxConns`, `MaxIdleDestroyCount`, `MaxLifetimeDestroyCount`,
`NewConnsCount`, `TotalConns`].

*Canary (AC14, AC19):*

| Family | Type | Labels | Source |
|---|---|---|---|
| `labgame_canary_probes_total` | counter | `leg` (`own`, `cloud`), `outcome` (`success`, `failure`) | one per probe |
| `labgame_canary_probe_failures_total` | counter | `leg`, `reason` | the classifier of D11 |
| `labgame_canary_probe_duration_seconds` | histogram | `leg`, `outcome` | the probe's measured latency |

`outcome` and `reason` are named in the spec's allow-list, each with its value set stated closed:
the success-or-failure pair, and — for `reason` — a status code where a response arrived plus a
code-enumerated class where none did
[measured 91a5515:ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md:137-138 · `sed -n '137,138p'
ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md` → the allowed row naming `canary leg, canary outcome, canary
failure reason` and closing each set, and the forbidden row's `A classified canary failure reason
is not error text: it is one of the enumerated classes the allowed row names, chosen from the
error, never rendered from it.`]. That is the authorising side of AC19 and AC23; D11 is the
mechanism, and it renders no error text into either label.

**D7 — histogram buckets are compiled-in named variables, not configuration, and one set does not
serve every family.** Buckets are not persisted, are cheap to change, and are not a game constant,
so `docs/DESIGN.md` §16.5 does not reach them; making them an operator key would let an operator
silently break a recorded histogram's comparability across a restart. Named variables in
`registry.go`, one per shape: a transport/probe/handler-duration set spanning milliseconds to the
default attempt timeout; an update-lag set spanning a fraction of a second to an hour, because the
star of the dashboard is a stall indicator and a stall is measured in minutes; a scheduler-lag set
spanning a fraction of a second to half an hour; a poll-duration set whose upper reach covers the
default long-poll window; a scheduler-loop set in the sub-second band; and two batch-size sets
covering the ranges their own configuration bounds — the ingest batch limit's stated ceiling
[measured 9ff1fce:internal/config/ingest.go:41-43 · `sed -n '41,43p' internal/config/ingest.go` →
`// ingestBatchLimitMax is the Bot API's own stated ceiling on` / `//
GetUpdatesParams.Limit ("Values between 1-100 are accepted").` / `const ingestBatchLimitMax = 100`]
and the scheduler's claim limit
[measured 9ff1fce:.env.example:90 · `rg -n LAB_GAME_SCHEDULER_CLAIM_LIMIT .env.example` →
`90:LAB_GAME_SCHEDULER_CLAIM_LIMIT=32`]. This closes the spec's second Open question.

**Every bucket variable is written as an explicit ascending literal, and a test of its own proves
it.** D3 establishes that a bucket slice is a panic input, that for a `HistogramVec` the panic
fires lazily on the observing goroutine, and that the error-returning spelling does not shield it.
Two consequences bind the implementor:

- **No `LinearBuckets`, `ExponentialBuckets` or `ExponentialBucketsRange` at package scope.** Each
  panics on a bad argument, and at package scope that is an init-time crash with no test between
  the mistake and the binary. An explicit literal has no arguments to get wrong, and it also makes
  the boundaries readable in review, which matters more here than brevity.
- **A table-driven test over every bucket variable, in subtask 4, independent of the scrape
  guard.** For each variable: non-empty, strictly increasing, every boundary finite and positive,
  and no `+Inf` written by hand. Strictly-increasing is the panic guard. **Non-empty is a
  different guard for a different failure**: an empty slice does not panic, it is silently
  replaced by `DefBuckets` (D3), so an empty update-lag variable would measure the dashboard's
  stall indicator in the default millisecond band and report nothing wrong. The scrape guard
  cannot catch either one, because it observes families rather than declarations, so this test is
  separate on purpose.

No metric this task adds declares an `le` label — the label-name allow-list of D5/D12 has no such
member — and the same table test asserts it, since `le` is the other `newHistogram` panic.

**D8 — no label combination is pre-initialised.** The common Prometheus advice is to touch every
label combination at start-up so a series exists at zero. It is refused here, because AC17 and spec
constraint 11 require a configured-off cloud leg to export **no probe sample of any kind**, and the
alert contract's absence test (D14) is what distinguishes a disabled leg from a failing one. A
pre-initialised `leg="cloud"` series would read as a healthy leg, which constraint 11 names as the
one thing that must never happen. The rule is package-wide rather than canary-only, so there is no
second convention to remember.

**D9 — the pool collector takes an accessor, and its zero value is not constructible by hand.**
`NewPoolCollector(reg prometheus.Registerer, stat func() *pgxpool.Stat) (*PoolCollector, error)`;
`Collect` calls `stat` on every scrape, which is AC12's "at scrape time rather than a cached
snapshot". The accessor is the seam the spec asks for ("fed a pool accessor at construction rather
than constructing a pool itself"), and #24 passes `pool.Stat`. **A hand-built `&pgxpool.Stat{}` is a
nil dereference, not an empty snapshot** — `Stat` holds an unexported `*puddle.Stat` that every
method delegates to
[measured 9ff1fce · `sed -n '9,21p' $(go env GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/pgxpool/stat.go`
→ `type Stat struct {` / `s *puddle.Stat` … and `func (s *Stat) AcquireCount() int64 { return
s.s.AcquireCount() }`] — so the accessor's documented contract is "returns a snapshot taken from a
real pool, or nil", and `Collect` emits nothing when it returns nil. That nil branch is not
defensive decoration: it is the honest answer for a caller whose pool has been closed, and it keeps
the collector from taking the process down at scrape time.

**`Collect` has no error return, and the sink for one is named here rather than left to the
implementor — because the obvious way out of that corner is a panicking constructor.**
`prometheus.NewConstMetric` returns `(Metric, error)`; `Collect` cannot return it, `_ = err` is
forbidden outright by `AGENTS.md` § *Code Style*, and the escape hatch sitting right beside it in
the library is `MustNewConstMetric`, which panics. The library supplies the correct sink instead:
a non-nil error becomes `prometheus.NewInvalidMetric(desc, err)`, sent on the channel like any
other metric, and the registry reports it as a collection error on the scrape
[measured 9396d3c · `go doc github.com/prometheus/client_golang/prometheus.NewInvalidMetric` in a
scratch module → `func NewInvalidMetric(desc *Desc, err error) Metric` / `NewInvalidMetric returns
a metric whose Write method always returns the provided error. It is useful if a Collector finds
itself unable to collect a metric and wishes to report an error to the registry.`]. So the error
is neither swallowed nor fatal, and no `Must…` call is needed anywhere in this package — which is
what D12's extended source walk then enforces rather than assumes. **A real pool needs no reachable server
to answer `Stat()`**, which is what keeps this package's test binary database-free
[measured 9ff1fce · a scratch probe running `pgxpool.NewWithConfig` against a DSN pointing at a
closed port, then `pool.Stat()` → `new err: <nil>` followed by a populated snapshot line].

**D10 — `Server` and `Canary` both carry `Start` / `Shutdown`, deliberately unlike this module's
existing foreground loops.** Both of those are `Run(ctx)`
[measured 9ff1fce:internal/ingest/loop.go:183 and internal/scheduler/worker.go:144 · `rg -n 'func
\(l \*Loop\) Run' internal/ingest/loop.go; rg -n 'func \(w \*Worker\) Run'
internal/scheduler/worker.go` → `183:func (l *Loop) Run(ctx context.Context) error {` and `144:func
(w *Worker) Run(ctx context.Context) error {`]. The spec asks twice for "an explicit start and an explicit shutdown the caller
owns" (Scope 3, constraint 10), and AC21 asks for a shutdown entry point that *returns*. `Run(ctx)`
is the right shape for a foreground loop the composition root owns a goroutine for; these two are
background services #24 starts and stops around everything else, and giving both the same pair keeps
#24's start-up and shutdown sequences symmetric. `Server.Start` binds the listener **synchronously**
and returns the bind error before spawning the serve goroutine, which is what makes AC32's "a bind
failure is reported through a returned error" true; `Server.Addr()` exposes the bound address so a
test may bind port `0`. `Server.Shutdown(ctx)` joins the graceful-shutdown error with the serve
goroutine's terminal error, so no error is dropped; `http.ErrServerClosed` is the expected terminal
value and is not reported as a failure. `Canary.Start` returns an error on a second call rather than
panicking, and `Canary.Shutdown(ctx)` cancels the run context — which cancels the in-flight
per-tick contexts with it — and waits for the probe goroutine, so a shutdown never waits out the
current interval (AC21). **The endpoint's path is `/metrics`, a compiled-in constant, not a fifth configuration key.**
`docs/DESIGN.md` §13.5 fixes it as Prometheus's scrape target
[measured 9ff1fce:docs/DESIGN.md:436 · `rg -n promhttp docs/DESIGN.md` → `436:  - здоровье:
**Prometheus** скрейпит `/metrics` бота (promhttp) и `/stats` инстанса bot-api;`], and the alert
contract plus the infrastructure pass both wire against it. The *address* is the operator's knob
because a port collides; the *path* is a contract with the scraper, and making it movable would
let an operator silently break every alert without editing an alert. `Server` mounts that one
path on its own `http.ServeMux`; every other path is the mux's own 404, which is what subtask 9
asserts against a fixed target. The server sets an explicit `ReadHeaderTimeout` from a named
constant;
`gosec` is enabled and its Slowloris rule is what makes that non-optional
[measured 9ff1fce:.golangci.yml:26-27 · `sed -n '26,27p' .golangci.yml` → `# Security` / `- gosec`].

**D11 — the canary probes through `internal/tg`, one client per leg, one attempt per tick, and its
own observer.** Each leg gets its own `*tg.Client` built with its own limiter, so a canary draws on
no production budget; `getMe` classifies as `ClassOther`
[measured 9ff1fce:internal/tg/class_test.go:64 · `sed -n '64p' internal/tg/class_test.go` →
`{"getMe", ClassOther},`], whose windows default to unbounded
[measured 9ff1fce:.env.example:75-78 · `sed -n '75,78p' .env.example` → `# Every other call:
unbounded by default. Same grammar as above.` / `LAB_GAME_TG_LIMIT_OTHER_GLOBAL=off` /
`LAB_GAME_TG_LIMIT_OTHER_CHAT_RATE=off` / `LAB_GAME_TG_LIMIT_OTHER_CHAT_CAP=off`]. `NewTelegramProber` copies the supplied `config.Transport` and sets
`RetryMaxAttempts` to one itself, documenting that the field is ignored — that is AC18's "exactly one
attempt per tick", structural rather than configured, and `tg.New` accepts one as the minimum
[measured 9ff1fce:internal/tg/client.go:105-107 · `sed -n '105,107p' internal/tg/client.go` → `if
opts.Transport.RetryMaxAttempts < 1 {` returning `optionErrorf("Transport.RetryMaxAttempts", "must be
at least 1, got %d", …)`]. **The canary client's `Observer` is the leg's own status recorder, never
the transport adapter**, which is what makes AC18's second clause structural: a canary call has no
path to the transport series.

*The recorder is how AC16's success condition becomes literal.* `tg.Observation` carries the last
HTTP status code and the whole-call latency
[measured 9ff1fce:internal/tg/observe.go:9-25 · `sed -n '9,25p' internal/tg/observe.go` → `type
Observation struct {` with `Method`, `Latency`, `StatusCode`, `RateLimited`, `Retries`], and the
package's caller returns success only on a decoded envelope whose `Ok` is true
[measured 9ff1fce:internal/tg/caller.go:92-95 · `sed -n '92,95p' internal/tg/caller.go` → `if
attemptErr == nil && resp != nil && resp.Ok {` … `return resp, nil`]. So a probe reports success
**only** when `GetMe` returned no error *and* the recorded status code is 200 — `ok:true` from the
caller, `200` from the recorder, which is exactly the pair AC16 names. The recorder holds one
observation under a mutex and its contract states its precondition: at most one call in flight on
the client it is installed on, guaranteed by the runner running one probe per leg per tick and
waiting for it.

*Failure classification carries no error text* (AC19): with a recorded status code above zero the
`reason` label is that code rendered decimal; with a zero status code it is `timeout`
(`context.DeadlineExceeded` in the error chain), `canceled` (`context.Canceled`) or `network`. The
transport's own error type is already token-sanitised and names the method rather than the URL
[measured 9ff1fce:internal/tg/errors.go:8-15 · `sed -n '8,15p' internal/tg/errors.go` → `// Error is
the one typed error this package's caller ever returns for a` … `Err is sanitised at construction
time` … `the bot token never appears in Err, in Error()'s rendering, or in any` /
`// instrumentation observation.`], and no part of it becomes a label value regardless.

**`NewLegs` is where a credential meets an endpoint, and the pairing is stated here because a
swap is silent.** Round 2 named `NewLegs` only in § Test Design and never said what it is handed.
That is the one place this design failed to apply its own standard — D11 makes the attempt count
structural rather than configured, and then left the pairing to be inferred. The harm is specific
and is the whole reason the spec has a § Source conflicts section: swap the two and the
**production** bot token issues `getMe` at the cloud frontend every minute, reopening the session
§12.2's runbook keeps closed, against a spec constraint that says the own leg "opens no session
anywhere else"
[measured d8ead96:ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md · `sed -n '/^13\. /,/^14\./p'
ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md` → `13. **The own-instance leg carries
the production bot token against the configured instance base` / `URL** — the chain the design
asks it to exercise — and opens no session anywhere else.`]. Every test round 2 listed passes
under the swap: both probers run against a fake server with a fake token, both get 200 and
`ok:true`, and the leg-builder scenario only checked the cloud prober's nil-ness.

*The pairing, stated.* `LegsOptions` carries each leg's credential beside its own endpoint and
nothing else routes one to the other:

| Leg | Token | Base URL | Source |
|---|---|---|---|
| own instance | `LegsOptions.OwnToken` | `LegsOptions.OwnBaseURL` | `Config.BotToken` and `Config.BotAPIBaseURL` |
| cloud reference | `LegsOptions.CloudToken` | `LegsOptions.CloudBaseURL` | `Config.Health.CanaryCloudToken` and `Config.Health.CanaryCloudBaseURL` |

Both token fields are `config.Secret`, so a `%v` of the options struct redacts them. An empty
`CloudToken` returns a nil cloud `Prober` and builds no cloud client at all (AC17).

*And the pairing is asserted, at each place it can break.* The design owns every one of them but
the last, which it names rather than assumes:

- **Inside `NewLegs`** — routing the own token to the cloud endpoint. Closed by an injectable
  constructor: `ProberFactory func(TelegramProberOptions) (Prober, error)`, a field of
  `LegsOptions` that is nil in production and then means this package's own
  `NewTelegramProber`. A test passes a recording factory and asserts the exact `(Token, BaseURL)`
  pair of each call. The seam exists for this assertion and for no other reason; without it the
  routing is unobservable short of the network.
- **Inside `NewTelegramProber`** — building the client with the right pair but transmitting
  something else. Closed at the wire: telego addresses `<base>/bot<token>/<method>`
  [measured d8ead96:internal/tgtest/tgtest_test.go:15 · `sed -n '15p'
  internal/tgtest/tgtest_test.go` → `req, err := http.NewRequestWithContext(ctx,
  http.MethodPost, BaseURL+"/bot"+Token+"/getMe", nil)`], so the fake server records the path it
  received and the test reads the transmitted token out of it. That is a stronger assertion than
  any constructor-argument check: it asserts what left the process.
- **In #24's wiring of `LegsOptions`** — passing the cloud token as `OwnToken`. This one is *not*
  this task's to close, and saying so is the point rather than an omission. The reason is AC29's
  operative clause — **no composition, wiring, start-up or shutdown behaviour moves into this
  task** — and wiring `LegsOptions` is composition. The mapping in the table above is
  therefore a boundary obligation handed to #24, named here so it
  arrives as a stated contract instead of an assumption. The field names are the mitigation this
  task can supply, and their doc comments state the source variable for each.

*The probe seam is an interface declared here* — `Prober` with `Probe(ctx) (ProbeResult, error)`,
`ProbeResult` carrying the latency and the status code — so the runner's tests need neither telego
nor a socket, and the runner applies the 200-and-no-error rule once for both legs.

**D12 — the structural guards are tests, not prose.** Each closes an AC that a reviewer would
otherwise have to take on faith: a walk of the module's Go files asserting no import
of `prometheus/promauto` and no use of the default registerer (AC2); a reflection guard binding
the observation-field register to the structs it claims to cover, both directions (D16; AC4, AC5,
AC6); a scrape of a registry whose every adapter has been driven once, asserting every label name
belongs to the allow-list and every closed-set label's **observed** values, **keyed by `(family,
label)`** (AC10, AC11, AC23); a walk of this package's own non-test source for `panic(`,
`log.Fatal`, `os.Exit` **and the client library's panicking `Must…` spellings** (AC32); a pair of
walks over the module holding D17's consolidation of the shared test helper — one that it stayed
consolidated, one that it stayed test-only; a file-scoped import guard
keeping the canary's own files free of every database and scheduler symbol (AC21); the same
scrape asserting the body contains none of the sentinel secrets the fixture was built with — the bot
token, the cloud token, the DSN, a chat id, an update id, a task id, an operation id (AC23, AC24);
and `testutil.GatherAndLint` over that registry asserting no problem
[measured 9ff1fce · `go doc github.com/prometheus/client_golang/prometheus/testutil` in a scratch
module → `func GatherAndLint(g prometheus.Gatherer, metricNames ...string) ([]promlint.Problem,
error)` and `CollectAndLint can be used to detect metrics that have issues with their name, type, or
metadata`]. A problem promlint reports is a naming defect to fix in the name, never an assertion to
relax — nothing scrapes these names yet, so the names are still free to move and the alert contract
moves with them in the same PR.

**The value-set assertion is keyed by `(family, label)`, never by label name alone.** `outcome` is
one label *name* over three disjoint value sets, and `kind` wants `unknown` included where
`outcome` and `failure` want it excluded (D16), so a guard grouping by name observes their union
and fails against every one of them. The table it asserts:

| Family | Label | Observed values asserted |
|---|---|---|
| `labgame_scheduler_tasks_total` | `outcome` | `done`, `noop`, `failed` |
| `labgame_scheduler_tasks_total` | `failure` | exactly the members of `scheduler.FailureKind` |
| `labgame_ingest_update_outcomes_total`, `labgame_ingest_handler_duration_seconds` | `outcome` | exactly the members of `ingest.Outcome` |
| the ingest families carrying it | `kind` | the kinds the fixture drives, `unknown` included |
| `labgame_canary_probes_total`, `labgame_canary_probe_duration_seconds` | `outcome` | `success`, `failure` |
| the canary families carrying it | `leg` | `own`, `cloud` |
| `labgame_pgxpool_conns` | `state` | `idle`, `acquired`, `constructing` |

`method`, `code` and `reason` are **not** closed-set labels in this sense and get membership
assertions rather than set equality: the first two range over the Bot API's own method and status
spaces, and `reason` is a decimal status code or one of the enumerated transport classes, so the
guard asserts each observed value matches that shape.

**The sixth guard closes AC32 structurally, and round 3's version of it did not.** Round 3 wrote
the forbidden set as `panic(`, `log.Fatal`, `os.Exit`, while § Risks claimed the same walk was
"the place a `MustRegister` would be caught". It was not, and could not be: `reg.MustRegister(…)`
and `prometheus.MustNewConstMetric(…)` are *library* calls, so they contain none of those tokens
and the walk stays green on a file carrying one. That is the green-instrument shape this design
polices elsewhere — a safeguard whose instrument cannot detect the category it claims — and the
discriminating proof round 3 specified (a scratch file with a bare `panic(`) passes without ever
touching the hazard. **The forbidden set therefore also carries `MustRegister` and `MustNew`**,
the second as a family prefix rather than an enumeration, because the library ships
`MustNewConstMetric`, `MustNewConstHistogram`, `MustNewConstNativeHistogram` and the two
`…WithCreatedTimestamp` variants, and an enumerated list would miss the next one
[measured 9396d3c · `go doc github.com/prometheus/client_golang/prometheus` in a scratch module →
`func MustRegister(cs ...Collector)` at package level, plus — among others, this being a sample
of the family and not a census of it — `MustNewConstHistogram`,
`MustNewConstHistogramWithCreatedTimestamp`, `MustNewConstMetric`,
`MustNewConstMetricWithCreatedTimestamp` and `MustNewConstNativeHistogram`. Which is exactly why
the forbidden token is the `MustNew` **prefix**: an enumeration would already be missing
`MustNewConstSummary` and its siblings]. Guard (a) catches
only the package-level `prometheus.MustRegister`, and only because that one is a *default
registerer* use; `reg.MustRegister` on this package's own registry is invisible to it, and that
is the spelling an implementor actually reaches for. The walk is on the in-repo model
[measured d8ead96:internal/ingest/guards_test.go:78-92 · `sed -n '78,92p'
internal/ingest/guards_test.go` → `func TestGuard_NoPanicLogFatalOrOsExit(t *testing.T) {` with
`forbidden := []string{"panic(", "log.Fatal", "os.Exit"}`], extended by the two entries above.
Round 2 mapped AC32 only to the bucket table test and the server test, and neither asserts the
source property; a lint rule does not either. **Proved discriminating against both categories**,
not just the cheap one: a scratch file carrying a bare `panic(` must red it, and a second scratch
file carrying `prometheus.MustRegister(c)` must red it too — the second proof is the one round 3
lacked, and without it the extension is itself an untested instrument.

**Guard (a) and guard (f) both need the repository root, and the helper that finds it is
consolidated rather than copied again.** This is the design's own call, and the argument is the
workspace's own duplication threshold: the helper is already declared independently in several
test binaries, subtask 11's walks would otherwise add another copy, and consolidating first is
cheaper than exempting — an exemption would have to be argued once here and again at every future
site. Round 3 judged the remedy out of reach and routed it to the orchestrator; the owner directed
that it be taken in this task. So it is: D17 designs the package, subtask 1 performs the move
before any subtask needs it, and guards (g) and (h) verify that it stayed done. The one edit this
requires under `cmd/` is to `cmd/bot/main_test.go`, a test file — AC29 governs *production* files
under `cmd/` and the movement of composition behaviour, and this move is neither. The durable
statement of which declarations move is subtask 1's own file list, which names each package
rather than a line number that goes stale on the next commit to touch it. **The scope of that
list is set by behaviour, not by spelling — see D19, which is where an earlier version of this
very sentence went wrong.** **`walkGoFiles` is not part of this and is deliberately left alone:**
it has a single declaration module-wide, so no rule is triggered and consolidating it would be
unrequested scope.

**D13 — configuration: a new `LAB_GAME_HEALTH_` optional-with-default class.** The keys below, in
`internal/config/health.go`, read by a dedicated `loadHealth` and appended to `EnvKeys()` alongside
the transport, scheduler and ingest classes — the shape those three already established
[measured 9ff1fce:internal/config/env.go:52-57 · `sed -n '52,57p' internal/config/env.go` → `func
EnvKeys() []string {` whose body appends `transportEnvKeys()`, `schedulerEnvKeys()` and
`ingestEnvKeys()`].

| Key | Type | Default | Notes |
|---|---|---|---|
| `LAB_GAME_HEALTH_METRICS_ADDR` | `string` | `127.0.0.1:9095` | loopback only (AC22) |
| `LAB_GAME_HEALTH_CANARY_INTERVAL` | `time.Duration` | `1m` | one cadence, both legs (AC20) |
| `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN` | `Secret` | empty | empty or absent disables the cloud leg (AC17) |
| `LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL` | `url.URL` | the cloud Bot API origin | validated by the same parser the instance base URL uses |

*The interval default is the design's own cadence* [measured 9ff1fce:docs/DESIGN.md § 13.2 ·
`sed -n '/^### 13.2\./,/^### 13.3\./p' docs/DESIGN.md` → the canary bullet reading «`getMe` раз в
минуту через свой инстанс … + тот же `getMe` напрямую в облачный api.telegram.org как референс»].

*The listen-address default is a decision with a source.* It must not collide with anything
`docs/DESIGN.md` §13.5 puts on the same host — Prometheus itself and Grafana — nor with the
Postgres and `telegram-bot-api` ports `.env.example` already documents
[measured 9ff1fce:.env.example:23,27 · `rg -n 'LAB_GAME_DSN=|LAB_GAME_BOT_API_BASE_URL=' .env.example`
→ `23:LAB_GAME_DSN=postgres://user:password@localhost:5432/labgame?sslmode=disable` and
`27:LAB_GAME_BOT_API_BASE_URL=http://localhost:8081`]. `9095` sits just past the
Prometheus core block and carries no allocation in the Prometheus port registry
[measured 9ff1fce · `curl -sS https://raw.githubusercontent.com/wiki/prometheus/prometheus/Default-port-allocations.md`
then `grep -nE '^\| *90(9[0-9]) ' → | 9090 | http | Prometheus server |, | 9091 | http | Pushgateway
|, | 9092 | n/a | UNALLOCATED …, | 9093 | http | Alertmanager |, | 9094 | ? | Alertmanager clustering
|` and `grep -n '9095\|9096' → no match`]. The address is a configuration value regardless, so an
operator collision is a one-variable fix.

*The cloud base URL is a value, not a compiled-in literal*, because §11 already makes the Bot API
base URL a three-value axis — own instance, cloud, fake server — and a hard-wired endpoint would put
the probe out of a fake server's reach.

*Present-but-empty means off, exactly as absent does*, for the cloud token alone. An operator who
writes the key with no value plainly means "off", and refusing it would buy nothing. Every other key
in the class treats present-but-malformed as a `*KeyError` naming the variable, per the class's
existing shape.

**That clause is not a convenience — it is the escape hatch from a trap the example file creates,
and the trap is decided here rather than discovered in production.** Every value in
`.env.example` must be non-empty, so the cloud token ships a placeholder; an operator who copies
the example to `.env` and fills in the real values therefore starts with the cloud leg **enabled
under a dead credential**. The consequence is not a quiet no-op: every probe fails, `getMe` hits
the cloud API once a minute with a bogus token, and the consecutive-failure alert fires forever —
so AC17's "absent means off" is easy to state and, by default, hard to reach. The decisions:

- **The placeholder is `changeme`**, the same token the required bot token already carries in that
  file, so it reads unmistakably as a value the operator must replace rather than as a working
  default.
- **The way to turn the leg off is to set the key empty** — `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN=` —
  which is exactly what the present-but-empty clause above exists for, and which is the *only*
  spelling available to an operator who started from the example, since deleting the line is a
  divergence from a manifest a test asserts.
- **The example file's comment for that key says so in prose**, beside the placeholder: what the
  leg is, that an empty value disables it, and that a placeholder left in place means a probe
  failing every interval. The comment names no URL and no other file, so the reference ban holds.
- **The alert contract carries the same sentence** (D14), because the operator who meets this is
  reading an alert, not the configuration loader.

*Rejected: exempting the cloud token from the non-empty rule.* The rule is asserted by a shipped
test and it is what makes the example file a manifest rather than a suggestion; carving a hole in
it to fix a documentation problem trades a real invariant for a comment. *Rejected: making the
example's value empty and relaxing the test to allow it.* Same trade, one step further.

**This class crosses a standing decision, and the crossing is owned here rather than left to a
propagation sweep.** KD-27's substantive clause is not only the enumeration of member scopes: it
states that secrets and the base URL stay **required with no default**
[measured 9ff1fce:ai-docs/key-decisions.md:73 · `rg -n -o 'Secrets, the base URL, the chat allowlist and the
balance-file and world-set paths stay \*\*required\*\* with no default' ai-docs/key-decisions.md` → that
clause, on line 73]. `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN` is this project's first **optional
secret** and `LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL` its first **optional base URL**, so both are
exceptions to that sentence. They are a *recording*, not a reopening: the spec authorises each
outright (Scope 10, AC17 — an absent cloud token must load without error and disable the leg), and
the reason the original clause does not reach them is that KD-27 was written about credentials the
process **cannot run without**, whereas these two configure an optional *diagnostic leg* whose
absence is a supported operating mode. The bot token and the DSN stay required and gain no
default; the instance base URL stays required and gains none. **Subtask 13 records these two keys
as named exceptions to KD-27's clause, and rewrites no boundary.** An earlier draft of this
paragraph generalised the clause to "required unless the value's own absence is a designed
operating mode"; that is withdrawn. The spec authorises *these two keys* (Scope 10, AC17) and
authorises nothing wider, and loosening a rule nobody asked to loosen is unapproved scope exactly
as tightening one is. So the amendment names `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN` and
`LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL` as exceptions carrying the reason above, and leaves
KD-27's clause standing for every other secret and every other base URL — a future optional
secret argues its own case rather than inheriting one.

**D14 — the alert contract lives at `ai-docs/alert-contract.md`, in English.** `AGENTS.md` restricts
Russian to owner conversation and `docs/**` [measured 9ff1fce:AGENTS.md:4 · `rg -n -o 'Russian for
two surfaces only:\*\* conversation with the product owner, and `docs/\*\*`' AGENTS.md` → that
sentence, on line 4]; this is an operations artefact the infrastructure pass
reads. It carries the metric catalogue of D6 and one row per alert: name, the metrics it reads, the
expression shape over them, the condition shape, and the severity — the consecutive-canary-failure
alert, the update-lag-growth alert, the cross-leg "own instance sick while cloud healthy"
expression, what that expression means when the cloud leg is configured off, **the placeholder
trap of D13** — that a cloud leg failing every interval from first start is most likely a
placeholder token rather than an outage, and that the fix is to set the key empty — and the
absence test
that separates a disabled leg from a failing one. **The absence test is written over the whole
`labgame_canary_probes_total{leg="cloud"}` vector, never over one `outcome` value**, because D8
leaves an outcome's series absent until that outcome first occurs: a healthy leg has no `failure`
series and a dead leg has no `success` series, so only the vector's total absence means "not
configured". Thresholds stay the infrastructure pass's, and the document says so.

**D15 — this task moves no balance and declares no event.** It registers no ledger basis document,
writes no posting, and adds no event type: the telemetry-with-the-mechanic invariant binds a
*mechanic*, and #23 is the health surface itself. `docs/DESIGN.md` §13.4's event dictionary and the
product SQL views are the other half of observability and are already shipped
[measured 9ff1fce:ai-docs/context.md:27 · `rg -n -o 'the product event log \(the `event` table' ai-docs/context.md`
→ `27:the product event log (the `event` table`]. Nothing
here reaches `store.Post`, so there is no posting signature to state.

**D16 — the observation-field register lives in the package's own documentation, and a guard binds
it to the structs.** Round 1 left the register named in D1 and never populated, which is why AC5
and AC6 could not be discharged from the document. Both halves are fixed here.

*Where it lives.* AC4 is the strictest of the three: an exempt field is "named in that package's
documentation". A register living only in a design document satisfies neither that clause nor the
spec's constraint 2, and pointing at one from a Go comment is forbidden outright — a comment may
name no markdown path (constraint 8, AC31). So the register is **doc comments in
`internal/health`**: `doc.go`'s package comment carries one entry per observation field that
reaches no family, and each entry **states its own reason in prose** rather than pointing anywhere.
The mapping half — which field feeds which family — is stated on each adapter type's own doc
comment. D6's per-field rulings above are the source text for those entries, not a substitute for
them.

*What it covers.* Every field of `tg.Observation`, `scheduler.Observation`,
`scheduler.LoopObservation`, `ingest.Observation` and `ingest.LoopObservation`. Transport carries
no exemption; the scheduler's `Observation.BatchSize` and `Observation.ConsecutiveFailures` and
ingest's `Observation.Attempt` are the exempt entries, each with the reason D6 gives.
`ingest.Observation.Attempt` is exempt because an attempt ordinal as a label value is a set bounded
only by an operator-set retry cap, and the retry volume it would describe is already the
`failed`- and `panic`-labelled outcome counts; the ordinal itself is a per-update property, not a
fleet-wide one.

*What binds it.* Prose drifts, so the register is backed by a package-level table mapping each
observation struct to each of its fields and that field's disposition — a family name, or an
exemption. D12's reflection guard asserts, **in both directions**, that the table's field set is
exactly the struct's own (a new upstream field reds it; a stale entry reds it too), and that every
field marked exempt has its name present in the package's own doc comment, parsed rather than
grepped. Without the second half the prose and the table could part company silently, which is the
whole failure the register exists to prevent. The precedent is in this module already:
`internal/ingest` binds its kind table to `telego.Update` by reflection for exactly this reason
[measured 9ff1fce:internal/ingest/guards_test.go:139 · `rg -n 'func
TestGuard_TelegoUpdateFieldsMatchKindTable' internal/ingest/guards_test.go` → `139:func
TestGuard_TelegoUpdateFieldsMatchKindTable(t *testing.T) {`].

**The same decision settles what a label's "value set" means, because D5's mappers and AC10/AC11
do not read the same way, and the divergence is stated as a divergence rather than papered over.**
AC10 and AC11 require the value set to be *exactly the members* of `ingest.Outcome` and
`scheduler.FailureKind`. D5's mappers each declare one branch more than that: a `default` returning
`unknown`, which the `exhaustive` linter's `default-signifies-exhaustive` setting is what makes
lint-clean, and which keeps the value set closed if the source enum ever grows. Round 1 wrote that
this was "what AC10 asks for". It is not, and the sentence is withdrawn.

The reconciliation is that **a label's value set is the set of values the series actually carry**,
not the range of the function that computes them. For **the two enums the ACs actually bind** —
`ingest.Outcome`, which only the ingest loop sets, and `scheduler.FailureKind`, which only the
worker sets — the `unknown` branch is unreachable, so it emits nothing and the observed set is
exactly the members, which is what AC10 and AC11 require. The claim is deliberately **not** made
for `scheduler.Outcome`, which a consumer-declared `Handler` supplies and the worker passes
through unnormalised (D6): there `unknown` is reachable in principle, no AC binds the set, and the
branch is doing real work rather than sitting unreachable. **D12's guard therefore asserts
the OBSERVED set, not the declared one**, and no § Test Design scenario drives an out-of-range enum
value into it: doing so to "make `unknown` observable" would be manufacturing the very series the
AC forbids. Widening the AC instead was rejected — it would authorise the silent-mislabel case the
criterion exists to catch.

**AC30 is discharged by the lint gate, and that is worth one sentence rather than a re-derivation
later.** Every exported item added here needs a doc comment opening with its name and every new
package needs a package comment; `revive`'s `exported` and `package-comments` rules are enabled
[measured d8ead96:.golangci.yml:45-48 · `sed -n '45,48p' .golangci.yml` → `revive:` / `rules:` /
`- name: exported` / `- name: package-comments`], so `golangci-lint run` refuses the diff that
misses one and AC33 carries that gate. No separate guard is written for it, and none is needed —
which is the opposite of AC32's case, where no lint rule asserts the source property and D12's
sixth guard therefore does.

Two consequences worth keeping straight. The mapper's `default` branch **is** still tested, as a
unit test of the function in subtask 4, called directly with an out-of-range value; that is a
statement about the mapper, never about a series, and the two assertions must not be conflated.
And ingest's `kind` is the one place `unknown` is genuinely *observed* rather than merely declared,
because an unrouted update really does carry the zero `Kind` — so the observed-set assertion for
`kind` includes it, and for `outcome` and `failure` it does not.

**D17 — the shared test helper is a new `internal/repotest`, exporting `Root` and `RootPath`.**
Where it lives is this design's call, and three constraints of its own decide it — each one
eliminating a candidate. It must be importable from the test binary of every package that needs
it, `cmd/bot`'s included; importing it must drag no machinery a consumer does not use into that
consumer's test binary; and it must be test-only, never linked into the bot command.

*Not `internal/tgtest`, although its package comment is what names the ≥3-site threshold in the
first place.* It is a fake Bot API server — `net`, `net/http`, `encoding/json`, a pipe listener
and a goroutine per server — which is exactly the machinery the second constraint refuses to drag
into a consumer that does not use it. `internal/config`'s test binary resolves a path to the balance file;
it has no business linking an HTTP server to do it. *Not `internal/testdb` either, and that one is
sharper:* it provisions PostgreSQL through testcontainers, and one of the call sites is
`cmd/bot/main_test.go`. Hosting the helper there would pull the container runtime into the bot
command's own test binary — not into the command, so KD-20's literal invariant would survive, but
squarely in the direction that invariant exists to keep clean. *So a new package, holding one
thing:* `internal/repotest` imports `path/filepath`, `runtime` and `testing` and nothing else, which
is the smallest import cost any of the call sites can pay.

*The name follows `internal/tgtest`'s own pattern* — `<domain>test` for a shared, test-only
package that is not itself a `_test.go` file — so the name states the test-only property that
guard (h) then enforces. `repotest.Root(tb)` and `repotest.RootPath(tb, rel)` do not stutter, and read
at the call site as the copies they replace read today.

**Two exported forms, because the tree already wants two, and exactly one of them resolves
anything.** The call sites divide cleanly: some want the repository root itself — `internal/tg`'s
guards spell that `repoRootPath(t, ".")` today, and both resolvers D19 adds to the move return
the root directly — while others want a path under it, the balance file, the world set and the
example environment file among them. So `repotest` exports `Root(tb) string`, **the one function
that resolves**, and `RootPath(tb, rel) string`, which is `filepath.Join` over it and resolves
nothing of its own. *Rejected: a single `RootPath` with the root-wanting sites passing `"."`.* It
works — `Join(root, ".")` cleans to the root — but it makes the commonest call in the tree read
as a special case of the rarer one, and it preserves a spelling those sites only ever used because
no better one existed. *Rejected: a single `Root`, with each joined site writing its own
`filepath.Join`.* That pushes an import and a line into several packages to save one wrapper here.
**The rule this design holds itself to is one resolver, not one exported name:** exactly one
declaration in the module derives the repository root from its own file's location, and guard (g)
enforces that behaviourally rather than by counting exported names, so `RootPath` — which calls no
`runtime.Caller` and performs no ascent — is not a second helper by the only definition the guard
can act on.

*The mechanism is preserved exactly, not improved.* `runtime.Caller(0)` on the package's own file,
then three `filepath.Dir` ascents — the package sits two directories below the repository root,
which is where every existing copy sits, so the arithmetic is unchanged. The parameter is
`testing.TB` rather than `*testing.T`, so a benchmark or a helper may call it. *Rejected: walking
upward for `go.mod`.* It is more robust and removes a hidden coupling to the package's own depth,
but it is a behaviour change inside a consolidation whose whole value is that nothing else moves —
and the fixed ascent's failure mode is a loud failure to open the resolved path, not a silent
wrong answer. The package comment states the depth requirement so a later mover meets it.

**The exported name is forced, and it is worth one sentence because it is the reason the old
spelling cannot survive the move.** An unexported identifier is unreachable from another
package's test binary, so `repoRootPath` cannot be what the shared declaration is called; every
call site therefore changes name as well as location, and no package keeps a local `repoRootPath`
to fall back on. Guard (g) checks both halves.

**D18 — live Go doc comments assert what this change falsifies, and they belong to subtask 3, not
to the propagation sweep.** § Decomposition declares the falsification class as "any live
surface asserting what the bot exposes, what environment variables it reads, or which packages
exist", and then measures it with an `rg` over `*.md` — an instrument that cannot reach Go source,
so the class and the instrument disagreed. Three comments in `internal/config` are in the class
and are true today, which is what makes this diff the thing that falsifies them:

- `EnvKeys`'s doc comment names "transportEnvKeys(), schedulerEnvKeys() and ingestEnvKeys() — the
  three optional-with-default tuning classes". A fourth arrives, and *tuning* stops being the
  right word for a class carrying a secret and a base URL
  [measured 9396d3c:internal/config/env.go:48-49 · `sed -n '48,49p' internal/config/env.go` → `//
  to equal exactly. transportEnvKeys(), schedulerEnvKeys() and` / `// ingestEnvKeys() — the three
  optional-with-default tuning classes — are`].
- `Config`'s doc comment says "Transport and Scheduler are the two exceptions … every other field
  has no compiled-in fallback", which `Config.Health`'s defaulted secret and base URL make false
  [measured 9396d3c:internal/config/config.go:38-41 · `sed -n '38,41p' internal/config/config.go` →
  `// returns an error naming every rejected key. Transport and Scheduler are` / `// the two
  exceptions: their fields are individually optional-with-default,` / `// so an absent LAB_GAME_TG_
  or LAB_GAME_SCHEDULER_ variable never fails` / `// Load — every other field has no compiled-in
  fallback.`]. It is already stale for `Ingest` and this change is not the place to leave it
  staler.
- The environment-variable const block's own comment enumerates the optional classes as the
  transport's and the scheduler's
  [measured 9396d3c:internal/config/env.go:11-16 · `sed -n '11,16p' internal/config/env.go` →
  `// Environment variable names, LAB_GAME_ prefixed …` through `// tuning variables are separate,
  optional-with-default classes, added on` / `// top by EnvKeys().`].

**They are subtask 3's obligation, and the routing is structural rather than a preference.**
Subtask 3's file list already carries `internal/config/env.go` and `internal/config/config.go`, it
is the commit that makes the sentences false, and KD-27 records this precedent in the same words —
the two sentences the *previous* optional class falsified "were rewritten in the same change". The
propagation subtask cannot take them: it sits in the instructions/harness group, where a `.go`
edit would break change-type homogeneity. So subtask 13's sweep stays `.md`-scoped by
construction and the Go-comment half of AC34 is subtask 3's, which is what makes the declared
class and the measuring instrument agree. Nothing gates a doc comment's truth, so this is the
kind of AC that fails silently and green if it is not assigned to a specific commit.

**The same rule binds subtask 1, and D19's widened move makes that concrete.** Every file the
hoist touches whose comment describes the helper it is deleting is falsified by the deletion, and
the fix belongs in subtask 1's own commit for exactly the reason above. Two are known before the
keyboard: `internal/commentref`'s helper is documented as "matching the helper every package's
test suite in this module already uses", which stops being true the moment there is one shared
declaration and this is no longer a local copy of it
[measured ca5f71f:internal/commentref/testhelpers_test.go · `sed -n '12,14p'
internal/commentref/testhelpers_test.go` → `// repoRoot resolves the repository root from this
test file's own` / `// location, matching the helper every package's test suite in this module` /
`// already uses.`]; and `internal/testdb`'s says its root is "derived from this file's own path",
which after the move is the shared package's file, not that one
[measured ca5f71f:internal/testdb/server_test.go · `sed -n '129,131p'
internal/testdb/server_test.go` → `// repoRoot returns this module's root, derived from this
file's own path` / `// rather than from the working directory, so the test is not sensitive to` /
`// how ` + "`go test`" + ` was invoked.`]. Subtask 1 re-reads the comment above **every**
declaration it removes and rewrites or deletes the ones the move falsifies; naming only these two
would repeat, at a smaller scale, the enumerate-what-I-happened-to-look-at mistake D19 exists to
record.

**D19 — the move is scoped by behaviour, not by spelling, and the correction is recorded because
the mistake is reusable.** Rounds 3 through 5 measured the duplication with `rg 'func
repoRootPath'` — an **identifier** search — and every later round inherited that set, D12 going so
far as to say the measurement "stands as taken and is not re-taken". But what is being
consolidated is a **behaviour**, not a spelling, so a search keyed to one identifier could not
answer the question, and two live resolvers fell through the gap — `internal/commentref`'s and
`internal/testdb`'s, both named `repoRoot` and both returning the root itself rather than a
joined path.

**So this design defines the class it is consolidating, by *mechanism*, and derives the members
from that definition.** The class is the **file-location-ascent kind**: a helper that derives the
root from its own source file's location and ascends a fixed number of directory levels, under
whatever spelling. Defining it this way rather than by an exception list is what makes the set
derivable instead of remembered — and it is what guard (g) can be written against. Derived, the
in-class resolvers are those of `cmd/bot`, `internal/config`, `internal/ingest`, `internal/tg`,
`internal/commentref` and `internal/testdb`
[measured d63c7a1 · `rg -n 'runtime\.Caller' --type go` → matches in `cmd/bot/main_test.go`,
`internal/ingest/guards_test.go`, `internal/commentref/testhelpers_test.go`,
`internal/tg/guards_test.go`, `internal/testdb/server_test.go`,
`internal/config/repo_root_test.go`]. **All of them are in the move**, and subtask 1's file list is
that set. The only one under `cmd/` is a test file, which AC29 — governing production files and
the movement of composition behaviour — does not reach.

**Out of class, by mechanism and not by exception: the two git-based resolvers in
`cmd/commentrefs`.** Its production `repoRoot(ctx)` and its test-side `repoRootForTest(t)` both
ask git for the worktree root
[measured 9571152:cmd/commentrefs/git.go and cmd/commentrefs/git_test.go · `rg -n
'rev-parse", "--show-toplevel"' cmd/commentrefs/` → `func repoRoot(ctx context.Context) (string,
error)` and `func repoRootForTest(t *testing.T) string`, each calling `runGit(…, ".",
"rev-parse", "--show-toplevel")`]. Neither derives anything from its own file's location, so
neither is of the file-location-ascent kind; the test-side one deliberately exercises the same git
mechanism the tool ships, which is the point of it. That is the whole reason they are out: not an
exemption granted to them, but a class they were never in. (The production one is additionally a
production file under `cmd/`, which this task does not touch regardless.)

*The self-inflicted part is worth stating plainly, because it is what makes this more than a miss.*
Guard (g) enforces the behaviour class. Had subtask 1 moved only the identifier-matched set,
subtask 11's guard would have redded against files no subtask touched, at the end of Group B, with
the remedy outside every declared file list — a task-stopping failure manufactured by the design
itself. The general form: **when an AC is written about a behaviour, the measurement that scopes
the work must search for that behaviour**; an identifier is a proxy, and a proxy that has never
been checked against the thing it proxies is the same green-instrument shape this design polices
everywhere else.

**And the round-6 statement of that lesson was itself too narrow — withdrawn here, in the same
shape as the error it was diagnosing.** Round 6 wrote that `runtime.Caller` "is currently
coextensive with the class", and that the search returned those files "and nowhere else". Both are
**false**: `cmd/commentrefs` holds two repository-root resolvers that call no `runtime.Caller` at
all. What is true, and all that was ever measured, is narrower — `runtime.Caller` is coextensive
with the *file-location-ascent* kind, which is a different and smaller class than "resolves the
repository root". Round 6 diagnosed a proxy failure and committed one in the same paragraph.

**So the durable form of the rule is not "do not use an identifier proxy".** The failure was not
*an identifier* proxy; it was *a proxy*, and `runtime.Caller` is a proxy too — for a mechanism,
one step closer to the behaviour and still not the behaviour. The rule that survives: **a
measurement that scopes work against a stated behaviour must be run with at least two independent
mechanisms, or the class must be defined by mechanism up front.** This design does the latter, in
the paragraph above, which is why no third enumeration was needed — the class is a definition to
apply rather than a set to go looking for, and guard (g) is written against that definition
instead of against whatever the last search happened to return.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | The shared test-helper package of D17: `internal/repotest` holding `Root` and `RootPath`, with **every** repository-root resolver in the module replaced by a call into it and none left behind — the set is D19's behaviour-scoped one, not the identifier-scoped one — plus the doc comments the removals falsify (D18) | `internal/repotest/repotest.go`, `internal/repotest/repotest_test.go`, `cmd/bot/main_test.go`, `internal/ingest/guards_test.go`, `internal/tg/guards_test.go`, `internal/config/repo_root_test.go`, `internal/commentref/testhelpers_test.go`, `internal/testdb/server_test.go` | — |
| 2 | Take the module requirement: `go get github.com/prometheus/client_golang@v1.24.1`, `go mod tidy`, read `git diff go.mod go.sum` before staging | `go.mod`, `go.sum` | — |
| 3 | The `LAB_GAME_HEALTH_` configuration class: keys, `Health` struct, defaults, `loadHealth`, `healthEnvKeys()`, `EnvKeys()` and `Config` wiring, the example-file entries, the example file's own prose for the cloud-token placeholder (D13), the class's tests including both the sibling `ExampleMatchesDefaults` and `QueriesEveryKeyUnconditionally` shapes, **and the Go doc comments this class falsifies** (D18) | `internal/config/health.go`, `internal/config/health_test.go`, `internal/config/config.go`, `internal/config/env.go`, `.env.example` | — |
| 4 | Package skeleton: the package comment carrying the observation-field register of D16 and the field-disposition table behind it, `NewRegistry`, `RegisterRuntime`, the name prefix, the bucket variables **and the bucket-validity table test of D7**, the label-name constants, the enum label-value mappers and the allow-list | `internal/health/doc.go`, `internal/health/registry.go`, `internal/health/labels.go`, and their tests | 2 |
| 5 | The transport adapter satisfying `tg.Observer` | `internal/health/transport.go`, `internal/health/transport_test.go` | 4 |
| 6 | The scheduler adapter satisfying `scheduler.Observer` | `internal/health/scheduler.go`, `internal/health/scheduler_test.go` | 4 |
| 7 | The ingest adapter satisfying `ingest.Observer`, with the `LagKnown` sampling gate | `internal/health/ingest.go`, `internal/health/ingest_test.go` | 4 |
| 8 | The pgx pool collector over a `func() *pgxpool.Stat` accessor, with the `NewInvalidMetric` error sink of D9 | `internal/health/pool.go`, `internal/health/pool_test.go` | 4 |
| 9 | The `promhttp` endpoint's server with synchronous bind, `Addr`, and joined-error shutdown | `internal/health/server.go`, `internal/health/server_test.go` | 4 |
| 10 | The canaries: the `Prober` seam and the `ProberFactory` seam, the Telegram prober with its status recorder and failure classifier, the leg builder with the credential-to-endpoint pairing of D11 (and the cloud leg disabled on an absent token), and the ticker runner | `internal/health/canary.go`, `internal/health/probe.go`, `internal/health/canary_test.go`, `internal/health/probe_test.go` | 3, 4 |
| 11 | The structural guards of D12 — the D16 register-vs-struct reflection guard, the per-`(family, label)` value-set guard, the panicking-call walk, the D17 helper guards with their pre-change-tree validation, and the canary import guard — then the whole `make verify` plus the coverage ratchet | `internal/health/guards_test.go` | 1, 5, 6, 7, 8, 9, 10 |
| 12 | The alert contract, including the placeholder-trap note of D13/D14 | `ai-docs/alert-contract.md` | 11 |
| 13 | Propagation per `AGENTS.md` § *Propagation Rule* step 4 (AC27, AC34): index the contract, correct every live surface this diff falsifies, and record the two KD-27 exceptions D13 names — the clause itself is left standing | `ai-docs/agent-docs-index.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/key-decisions.md` | 12 |

**Subtask 13's sweep is an obligation, not a fixed list.** The class is "any live surface asserting
what the bot exposes, what environment variables it reads, or which packages exist". The files
named are the members a sweep at this commit finds
[measured 9ff1fce · `rg -l -i "LAB_GAME_INGEST_|optional-with-default|internal/ingest" --glob '*.md'
--glob '!tmp/**' --glob '!ai-docs/plans/**' --glob '!ai-docs/learnings.md' .` →
`./ai-docs/context.md`, `./ai-docs/context-status.md`, `./ai-docs/key-decisions.md`], plus
`ai-docs/agent-docs-index.md`, which AC27 names outright. The implementor re-runs the sweep against
the finished diff rather than trusting this row; history surfaces (`ai-docs/learnings.md`,
`ai-docs/plans/done/**`) are left untouched. Known falsifications to fix rather than rediscover:
`key-decisions.md`'s KD-27 states the optional-with-default class's boundary by enumerating its
member scopes [measured 9ff1fce:ai-docs/key-decisions.md · `rg -n 'the relaxation reaches'
ai-docs/key-decisions.md` → `*The boundary, stated exactly:* the relaxation reaches **only**
operational tuning keys — these fourteen, the seven `LAB_GAME_SCHEDULER_*` keys … and the seven
`LAB_GAME_INGEST_*` keys update ingestion added as the third`], and `context.md`'s layout paragraph
enumerates the packages and names each observation seam as "the observation seam #23 reads" — which
stops being true when #23 lands [measured 9ff1fce:ai-docs/context.md:27 · `rg -n -o 'the observation
seam #23 reads' ai-docs/context.md` → the phrase, on line 27, more than once].

## Handoff plan

Grouping is required for **every M ≥ 1** — this section is mandatory in every design, including a
single-subtask one, whose one group is also terminal and runs in its own `/context-reset` subagent.
A group holds **up to 10** consecutive subtasks; ten is a **maximum**, not an exact count, and a
group ends at whichever comes first: the size cap, a change-type switch, or a dependency-forced
boundary. The terminal group's size is in `1..=10`. Each group is homogeneous by change-type —
**code** (`*.go`, migrations) or **instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`,
`ai-docs/**`) — never both. The classes are the harness's and are not restated here. The files of
this change that fall in **neither** enumerated class — `go.mod`, `go.sum` and `.env.example` — are
grouped with the code, because each is machine-read build or configuration input asserted by a Go
test in the same commit, not instructions an agent reads. Nothing that *is* enumerated is
reclassified. Same-change-type subtasks are clustered into the **fewest groups possible**, bounded
by the size cap, by dependency order and by homogeneity; naive interleaving is the least-desirable
fallback and is not used here. The default maximum is **4** groups per task, and more than 4 is
surfaced to the user for approval; this design defines **3**.

**Why 3 rather than the 2 of round 3, and why the split is forced rather than chosen.** D17's
test-helper consolidation is Go test code and therefore a code subtask. The code
change-type now holds subtasks 1–11, which is past the size cap, and the harness's own rule is that
a change-type with more than 10 subtasks splits into multiple same-model groups of `≤ 10`. So a
second code group is mandatory, not a clustering preference, and the minimisation rule is satisfied
at 3. **Where the boundary falls is free, and it is placed on risk.** The rules bound the split
only by the size cap, dependency order and homogeneity, all of which many placements satisfy;
round 4 put it after subtask 10, which left subtask 10 — the credential-to-endpoint pairing, the
`ProberFactory`, the failure classifier, the subtask § Risks itself calls the sharpest
correctness risk in the change — running **last, in the most degraded context of a ten-subtask
group**, while subtask 11 got a whole fresh group for a guards file whose every assertion D12
already specifies down to the table. That is backwards. The boundary now falls after subtask 6,
so the canary work opens Group B's second slot on fresh context, and the guards keep a
late-but-not-last position where their own discriminating proofs still get attention. Group sizes
are 6 and 5, both inside the cap; every cross-group dependency runs forward.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Every group is entered through it, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–6 (code change-type: `*.go`, plus `go.mod`, `go.sum` and `.env.example`).
  Inside the size cap. Subtask 1 leads because subtask 11's guard walk calls the shared helper and
  must never declare a copy of it, not even transiently; 5 and 6 depend only on 4 and are
  independent of each other, so their order inside the group is free.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 7–11 (code change-type: `*.go`). Inside the size cap. Every dependency it
  needs from outside itself is already complete: 7, 8 and 9 depend only on 4, and 10 on 3 and 4,
  all in Group A; 11 depends on 1, 5 and 6 from Group A and on 7–10 from within this group.
  **Order inside Group B is pinned, not free: 10, 7, 8, 9, 11.** Subtask 10 goes first because it
  is the sharpest correctness risk in the change and this boundary exists to give it fresh
  context; leaving the order open would have let it land fourth of five and reproduce, in weakened
  form, the very placement the rebalance was made to fix — a rationale the plan does not enforce
  is not a mitigation. Subtask 11 is last by dependency. Only 7, 8 and 9 are interchangeable.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group C with fresh context.
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via the `general-purpose` subagent with no inline `model=`
  override, 1M-token window — subtasks 12–13 (instructions/harness change-type: `ai-docs/**`).
  Terminal group, within the `1..=10` range. Both depend on subtask 11, which Group B completes
  first, so dependency order holds across the boundary.

**Group sizes are unchanged by round 6's widening.** D19 adds files to subtask 1, not subtasks to
the design: M stays at 13, the groups stay 1–6, 7–11 and 12–13, every group stays inside the size
cap, the terminal group stays in `1..=10`, and no dependency crosses a boundary backwards. Both
newly-added files are Go test files, so Group A stays homogeneous by change-type.

**The Go-comment half of AC34 is deliberately not in Group C.** Group C is homogeneous
instructions/harness, so a `.go` edit cannot land in it without breaking homogeneity — which is why
D18 routes the falsified Go doc comments to subtask 3, in Group A, beside the change that falsifies
them. Subtask 13's sweep is `.md`-scoped by construction.

Marker-to-implementor routing is applied at spawn: a **code** group routes to
`subagent_type="code-writer"`, whose `model: sonnet` and `effort: medium` are frontmatter-pinned, with
no inline override; an **instructions/harness** group routes to `subagent_type="general-purpose"`
with no inline `model=` and inherited effort. The `design-writer`, `design-review`, `self-review` and
`spec-writer` subagents run on the orchestrator's model regardless of any group marker.

## Risks

- **A registration or metric constructor that panics defeats AC32 and the empty panic index.** The
  library ships a panicking spelling beside every fallible one, and `promhttp.HandlerOpts.Registry`
  panics on a failed registration by its own documentation. Mitigation: D3 fixes the non-panicking
  spelling for each, D9 names `NewInvalidMetric` as the collector's error sink so no corner pushes
  an implementor toward `MustNewConstMetric`, and **subtask 11's source walk carries `MustRegister`
  and `MustNew` in its forbidden set** — which round 3 asserted and did not deliver: its walk
  looked only for `panic(`, `log.Fatal` and `os.Exit`, none of which appears in a library `Must…`
  call, so the row claimed structural enforcement the instrument could not provide —
  `[measured 9ff1fce · go doc github.com/prometheus/client_golang/prometheus/promhttp.HandlerOpts in
  a scratch module → "A failed registration causes a panic."]`.
- **A mis-ordered bucket variable panics on a production goroutine, not at start-up.** This is the
  sharpest risk in the change and round 1 missed it: for a `HistogramVec` the library builds the
  histogram lazily, so a non-increasing bucket slice — or an `le` label — panics at the *first
  observation* of a label combination, which is the ingest loop's or the scheduler worker's own
  goroutine. The error-returning spelling does not shield it, and an empty slice fails the other
  way, silently becoming `DefBuckets`. Mitigation: D7 forbids the panicking bucket helpers at
  package scope and puts a table test over every bucket variable in subtask 4, ahead of every
  adapter that observes into one — `[measured 9ff1fce · sed -n '1183,1205p' $(go env
  GOMODCACHE)/github.com/prometheus/client_golang@v1.24.1/prometheus/histogram.go → NewMetricVec's
  newMetric closure calling newHistogram, and sed -n '590,595p' of the same file → the
  "histogram buckets must be in increasing order" panic]`.
- **A credential reaches the wrong endpoint and every test still passes.** The own leg carries the
  production bot token; the cloud leg must not. A swap inside `NewLegs` sends that token to the
  cloud frontend once a minute, reopening the session the migration runbook keeps closed — and it
  is invisible to every behavioural test, because both legs answer 200 with `ok:true` against a
  fake server holding a fake token. This is the sharpest correctness risk in the change and round
  2 carried no mitigation for it at all. Mitigation: D11 states the pairing, the `ProberFactory`
  seam makes the routing observable, and subtask 10 asserts both the recorded `(Token, BaseURL)`
  pair per leg and the token actually transmitted on the wire; #24 owns the remaining hop and is
  told so — `[derived → subtask 10's leg-pairing scenarios]`.
- **The observation-field register drifts from the structs it claims to cover.** A telego or
  scheduler change adding an `Observation` field would leave the register silently incomplete,
  which is exactly the state round 1 shipped in this document. Mitigation: D16's table plus D12's
  both-directions reflection guard, and the doc-comment presence assertion that stops the prose
  and the table parting company — `[derived → subtask 11's register guard]`.
- **A hand-built `pgxpool.Stat` nil-dereferences at scrape time.** A test author reaching for
  `&pgxpool.Stat{}` as a fixture writes a panic into the scrape path. Mitigation: D9 makes the real
  pool the fixture — a pool built against an unreachable DSN answers `Stat()` with no server — and
  the accessor's contract is "a snapshot from a real pool, or nil" with `Collect` emitting nothing on
  nil — `[measured 9ff1fce · sed -n '9,21p' $(go env
  GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/pgxpool/stat.go → type Stat struct { s *puddle.Stat …
  } with every method delegating through s.s]`.
- **The example-environment tests reject an empty default.** Every value in the example file must be
  non-empty, and each optional class has a test asserting the example's own values reload to the
  compiled-in defaults — while the cloud token's compiled-in default is empty by AC17. Mitigation:
  subtask 3's `ExampleMatchesDefaults` compares the class with the token field zeroed on the loaded
  value, and asserts separately that the example documents a non-empty placeholder while an absent
  key yields the empty default — `[measured 9ff1fce:internal/config/disjoint_test.go:110-116 ·
  sed -n '110,116p' internal/config/disjoint_test.go → func TestEnvExample_ValuesAreNonEmpty(t
  *testing.T) whose body errors on any key whose value trims to empty]`.
- **A comment or an example-file comment naming the cloud origin trips the comment-reference gate.**
  A URL is a banned class in every gated file, and `.env.example` is in the gated set. Mitigation:
  the origin appears as a *value* in the example file and as a string literal in Go, never inside a
  comment; the same rule keeps the alert contract's name out of every comment — `[measured
  9ff1fce:ai-docs/doc-convention.md:13-14 · sed -n '13,14p' ai-docs/doc-convention.md → "DOC-4 and
  DOC-5 apply to every comment in the gated set: *.go, *.sh, *.sql, *.yml, *.yaml, .gitignore,
  .env.example, Makefile, and everything under .githooks/."]`.
- **A `promlint` problem fails subtask 11 late, after the alert contract has been drafted against the
  offending name.** Mitigation: subtask 11 precedes subtask 12 in the decomposition precisely so the
  names are settled before the contract quotes them; a reported problem changes the name, not the
  assertion — `[derived → subtask 11's `GatherAndLint` assertion, and subtask 12's dependency on 11]`.
- **The canary's goroutine outlives a test, or races its own shutdown.** The race gate is required
  for concurrent code and a leaked goroutine fails a `synctest` bubble outright. Mitigation: `Start`
  refuses a second call, `Shutdown` cancels the run context and waits, and the runner tests run
  inside a bubble whose root cannot return while the probe goroutine lives —
  `[derived → the canary lifecycle tests of § Test Design, run under `make test-race`]`.
- **The coverage ratchet blocks on a large new package.** A new package's uncovered error branches
  drag the module's statement coverage down, and the ratchet refuses a drop past its tolerance.
  Mitigation: every subtask ships its tests with its code (TDD), and the error branches this design
  creates — a registration failure, a bind failure, a nil accessor, a double `Start` — are each
  reachable from a test without a fixture the package could not build —
  `[derived → the per-subtask tests of § Test Design, and subtask 11's ratchet run]`.
- **The `method` label is bounded only by what this module calls.** A future caller looping a method
  name derived from user input would make it unbounded. Mitigation: the allow-list of the spec's Key
  decisions binds the label *name* set, and the value set is closed by the methods this module itself
  issues; the alert contract records the bound so the infrastructure pass can alarm on series growth
  — `[derived → subtask 11's label allow-list guard, and subtask 12's contract]`.

## Test Design

Every entry here describes a test that does not exist yet.

**Subtask 1 — `internal/repotest/repotest_test.go`, plus the replaced call sites.** Entry points
`repotest.Root` and `repotest.RootPath`. Scenarios: `Root` returns an absolute path to a directory
containing `go.mod`, which is the assertion that pins the ascent arithmetic and fails loudly if
the package is ever moved to another depth; `RootPath` with a known repository-root-relative path
resolves to a file that opens (the tracked balance file is the natural fixture, since one of the
replaced call sites already resolves it); `RootPath` with `"."` equals `Root`, which pins the
wrapper to the resolver rather than letting the two drift.

**The move itself is verified by the suite it does not change**: every package whose declaration
is replaced keeps its existing tests green, which is the whole content of "behaviour-preserving"
here — `internal/config`'s example-environment tests resolve real paths through it, `cmd/bot`'s do
too, and `internal/commentref`'s and `internal/testdb`'s both read real files under the root, so a
wrong root fails them rather than passing quietly. **Each converted site keeps the form it
actually wants** (D17): the sites that took `(t, ".")` and the two that returned the root
directly become `repotest.Root(t)`, and only the genuinely joined ones become
`repotest.RootPath(t, rel)` — converting a root-wanting site to the joined form would preserve a
spelling those sites only ever used because no better one existed.
`[derived → D17's one-resolver rule and its behaviour-preserving claim, verified by guards (g)
and (h)]`

**Subtask 3 — `internal/config/health_test.go`.** Entry points `loadHealth` and `Load`. Table
subtests, `t.Parallel()`: all keys absent yields the compiled-in defaults, and the cloud token is
empty so the leg is off; the example file's own values reload to those defaults with the token field
excluded from the comparison; a malformed duration, a malformed address and a malformed base URL each
produce a `*KeyError` naming their own variable and matching `ErrInvalidValue`; a present-but-empty
cloud token is treated as absent; the default address parses as loopback. **Plus the sibling this
class would otherwise be missing: a `QueriesEveryKeyUnconditionally` test**, matching the one each
of the transport, scheduler and ingest classes already carries
[measured ca5f71f:internal/config · `rg -n 'func Test.*QueriesEveryKeyUnconditionally'
internal/config/*_test.go` → one in `scheduler_test.go`, one in `transport_test.go`, one in
`ingest_test.go`]. It is the only test that catches a conditional read — a loader that skips
looking up the cloud base URL when the token is absent, say — because the identity test that would
otherwise notice runs with the example file's **non-empty** placeholder token and so never
exercises the absent-token path at all. Without it, a conditional read ships green and the
declared-key-set identity of AC25 quietly stops holding for the one configuration this task's own
AC17 makes normal. Plus the existing
package-level identity assertions, which must still pass with the new keys present. Fixtures: the
package's own `mapLookup` and `readEnvExampleKeys` helpers, and the recording lookup the sibling
tests use.
`[derived → AC17, AC20, AC22, AC24, AC25]`

**Subtask 4 — `internal/health/registry_test.go`, `labels_test.go`.** Entry points `NewRegistry`,
`RegisterRuntime`, and each label-value mapper. Scenarios: a fresh registry gathers nothing;
`RegisterRuntime` on it makes the Go-runtime and process families present in a scrape;
`RegisterRuntime` twice on the same registerer returns an error rather than panicking; each mapper is
table-driven over every member of its source enum plus an out-of-range value, asserting the closed
value set and the single `unknown` fallback. **That last case is a unit test of the mapper called
directly — it is not, and must not become, an assertion about an exported series** (D16). Fixture:
an isolated `prometheus.Registry` per case. `[derived → AC1, AC13]`

*Bucket validity, same subtask, its own table test and deliberately independent of any scrape.*
Entry point: each bucket variable, read directly. For every one: non-empty; strictly increasing;
every boundary finite and positive; no hand-written `+Inf`. Plus an assertion that no metric
declared by this package names a label `le`. The test names each variable explicitly rather than
reflecting over the package, so adding a bucket set without adding its row is a visible omission
in review. `[derived → AC32, and D7's mitigation of D3's histogram panic surface]`

**Subtask 5 — `internal/health/transport_test.go`.** Entry point `(*TransportObserver).ObserveCall`.
Scenarios: a successful call adds one duration sample and one response count under its own method and
code; a rate-limited call increments the 429 counter; a call with retries adds that many to the retry
counter; a call with no response records code `0`; two calls on different methods stay in separate
series. Assertion style: `testutil.CollectAndCompare` against an inline exposition fixture for the
counters, and `testutil.CollectAndCount` for the histogram, so a wrong label or a wrong family name
fails rather than passing on a total. `[derived → AC4, AC9]`

**Subtask 6 — `internal/health/scheduler_test.go`.** Entry points `ObserveTask` and `ObserveLoop`.
Scenarios: one observation per outcome, asserting the `outcome` label; one per member of
`FailureKind`, asserting the `failure` label and that the observed value set is exactly the members
of that enum; lag lands in the lag histogram under its `type` label; a loop observation with a
non-nil error increments the loop-error counter and contributes nothing else; a loop observation
with a nil error increments neither. **The exempt half is asserted too**: two observations
differing only in `BatchSize` and `ConsecutiveFailures` produce byte-identical scrapes, so a later
author who quietly wires one of them into a family reds this test rather than shipping a series
the register denies. `[derived → AC5, AC8, AC11]`

**Subtask 7 — `internal/health/ingest_test.go`.** Entry points `ObserveUpdate` and `ObserveLoop`.
Scenarios: one observation per member of `ingest.Outcome`, asserting the closed `outcome` value set
and that panic, duplicate, unrouted and given-up are each individually visible; **an observation with
`LagKnown` false contributes no lag sample at all**, checked as a count of zero on the lag family and
a non-zero count on the counter from the same observation, so "no sample" is distinguished from "a
zero-valued sample"; an observation with `LagKnown` true does contribute one; the empty kind maps to
the `unknown` label value — the one place that value is genuinely observed rather than merely
declared (D16); a non-nil `Err` increments the undecodable counter. **The exempt half is asserted
too**: two observations differing only in `Attempt` produce byte-identical scrapes.
`[derived → AC6, AC7, AC10]`

**Subtask 8 — `internal/health/pool_test.go`.** Entry points `NewPoolCollector` and its `Collect`.
Scenarios: with an accessor returning a real pool's snapshot, a scrape carries every family of D6's
pool table; with an accessor returning nil, a scrape carries none of them and does not fail; **the
accessor is called once per scrape and its return value is not cached**, checked by an accessor that
counts its own calls and by two scrapes whose values differ. Fixture: `pgxpool.NewWithConfig` against
a DSN pointing at a closed port — no server, no container, no `internal/testdb` import.
`[derived → AC12]`

**Subtask 9 — `internal/health/server_test.go`.** Entry points `NewServer`, `Start`, `Addr`,
`Shutdown`. Scenarios: `Start` on `127.0.0.1:0` binds and `Addr` reports the bound port; a GET of the
metrics path returns 200 with a body naming a family registered on the supplied gatherer; a GET of
another path returns 404; `Start` on an address already bound returns the bind error and starts no
goroutine; `Shutdown` returns nil on a clean stop and the serve goroutine's terminal
`http.ErrServerClosed` is not reported as a failure; `Shutdown` on a server never started returns an
error rather than panicking. Runs outside a `synctest` bubble — a real listener is not durably
blockable inside one [measured 9ff1fce:ai-docs/key-decisions.md:71 · `rg -n -o 'a pipe conn being durably
blockable inside a bubble while a real listener is not' ai-docs/key-decisions.md` → that clause of KD-26]. `[derived → AC3, AC22, AC32]`

**Subtask 10 — `internal/health/probe_test.go`, `canary_test.go`.** Entry points
`NewTelegramProber`, `(*TelegramProber).Probe`, `NewLegs`, `NewCanary`, `Start`, `Shutdown`.

*Prober scenarios*, against `internal/tgtest`'s in-process fake server wired through the client's
`HTTPClient` — a server that reaches no network by construction
[measured 9ff1fce:internal/tgtest/tgtest.go:11 · `rg -n -o 'BaseURL is under the reserved
".invalid" TLD' internal/tgtest/tgtest.go` → that clause of the package comment]: a 200 carrying `ok:true` yields no error and a status of 200, and the runner counts it a
success; a 200 carrying `ok:false` yields an error and is counted a failure; a 500 is counted a
failure with the code as its `reason`; a dial failure is counted a failure with `network`; a context
deadline is counted a failure with `timeout`; **a probe makes exactly one attempt** — asserted by a
handler counting the requests it receives while answering 500, which the production retry policy
would have retried; and **no probe writes to a transport-adapter registry** — asserted by running a
probe with a `TransportObserver` registered on a separate registry and gathering zero transport
families from it. **And the transmitted credential is asserted, not assumed**: the fake server
records the request path it received, and the test requires it to carry the prober's own token and
the `getMe` method — the half of AC15/AC16 that says *this* token left the process for *this*
endpoint. A prober built with one token and transmitting another reds here.
`[derived → AC16, AC18, AC19]`

*Leg-builder scenarios — the credential-to-endpoint pairing of D11, which round 2 left unasserted
and which no other scenario reaches.* `NewLegs` is called with a recording `ProberFactory` and a
distinguishable token and base URL per leg — the own leg's pair differing from the cloud leg's in
both components. Assertions: the factory was called once per enabled leg; the own call
received exactly `(OwnToken, OwnBaseURL)` and the cloud call exactly `(CloudToken, CloudBaseURL)`.
**A swap in either direction fails this test**, which is the whole reason it exists — every other
scenario in this subtask passes under a swap, because both legs answer 200 with `ok:true` against a
fake server holding a fake token. Composed with the prober scenario above, the chain from a
configuration value to the bytes on the wire is covered end to end. Then: **with the cloud token
empty the factory is called once, for the own leg only**, `NewLegs` returns a nil cloud prober, and
a canary built from it, run over several
ticks, exports no `leg="cloud"` series of any kind — asserted as the absence of the label value in
the gathered families, not as a zero-valued sample, because D8's absence is what the alert contract's
absence test rests on. `[derived → AC14, AC15, AC16, AC17]`

*Runner scenarios*, inside a `testing/synctest` bubble with fake `Prober`s so no socket is involved:
the first probe fires at `Start` rather than after one interval; a probe fires on each subsequent
tick; both legs probe on the same tick from one cadence value; a `Shutdown` issued mid-interval
returns promptly rather than after the remaining interval, asserted against the bubble's virtual
clock; a second `Start` returns an error; a prober that blocks past the interval is cancelled and
counted a failure rather than overlapping the next tick; the probe goroutine has exited when the
bubble's root returns. The whole file also runs under `make test-race`. `[derived → AC14, AC20,
AC21]`

**Subtask 11 — `internal/health/guards_test.go`.** Each guard is a `t.Parallel()` test, **and that
is only sound because of the isolation rule below.**

**Every walk takes a root parameter, and every discriminating proof runs over a `t.TempDir()`
copy — no proof mutates the repository.** Round 6 specified four tree-mutating proofs against the
live module: a scratch `promauto` file for (a), two scratch files for (f), a scratch import into
the tracked `probe.go` for (i), and a `git show` restore for (g). The design had stated the halves
of two consequences and never joined them. **First, the guards would have collided with each
other:** guard (a) rejects any use of the library's default registerer, and this design says
elsewhere that package-level `prometheus.MustRegister` *is* such a use — so guard (f)'s second
scratch file reds guard (a) whenever the two overlap in time, which under `t.Parallel()` is
whenever they run. **Second, any failure between mutation and restore leaves tracked source
dirty**, and the coverage ratchet blocks outright on unstaged edits to `.go` files — so a red in
the middle of a proof would stop the run at the gate rather than at the assertion, with the cause
one layer away from the symptom. Both dissolve the same way: each walk is `walk(t, root, …)`, the
real run passes `repotest.Root(t)`, and each proof copies the `.go` files under `cmd/` and
`internal/` into its own `t.TempDir()`, mutates *that*, and points its own guard at it. Nothing is
shared, so nothing collides and nothing needs restoring; a proof that fails leaves a temp
directory behind and a clean worktree. Mutating the repository to prove a test is a defect
independent of parallelism, and the root parameter is what makes the alternative available.

(a) A walk of every `.go` file under `cmd/` and `internal/`, parsed rather than grepped, asserting no
import path ending in `prometheus/promauto` and no reference to the library's default registerer or
default gatherer. **It obtains the repository root by calling `repotest.Root`** — the root itself,
which is what a walk wants, per D17's ruling that `RootPath` is for genuinely joined paths —
**and declares no resolver of its own** — subtask 1 has already deleted the per-package copies, and writing a local
`runtime.Caller` resolver here would re-create, in the package this task adds, exactly the
duplication D17 consolidated — and guard (g) would red on it — which the handoff plan bars even
transiently. Note that the shared helper
resolves from **its own** file's location, not the caller's, so nothing about this guard's own
position matters. **The walk is proved discriminating before it is
trusted**: it is run once against a scratch file carrying the banned import and required to fail,
then that file is removed — a guard that has never gone red is a claim about the guard. That
scratch file lives in the proof's temp copy, never in the worktree.
(b) The register guard of D16: reflect over `tg.Observation`, `scheduler.Observation`,
`scheduler.LoopObservation`, `ingest.Observation` and `ingest.LoopObservation`, and assert in both
directions that the package's field-disposition table names exactly those fields — a field with no
row fails, a row naming no field fails — and that every field the table marks exempt has its name
present in the package's own doc comment, obtained by parsing the package rather than by grepping
a string. Proved discriminating the same way guard (a) is: run once against a table with one row
removed and once against a doc comment with one exempt name removed, each required to fail.
(c) A registry driven once through every adapter, then gathered: every label name of every family
belongs to the allow-list, and every closed-set label's **observed** values are asserted **per
`(family, label)`**, against D12's table. Keying by label name alone would be a defect rather than
a shortcut: `outcome` is one name over three disjoint sets — the scheduler's, the ingest loop's and
the canary's — so a name-keyed guard observes their union and fails against all three, and `kind`
expects `unknown` where the ingest `outcome` and the scheduler `failure` expect its absence. The
fixture drives only in-range enum values, so `unknown` is absent from those two by construction and
that absence is the assertion, never an exception to it (D16). `method`, `code` and `reason` get a
shape assertion instead of set equality, per the same table. (d) The same scrape's text contains
none of the sentinel secrets its fixture was built with —
a bot token, a cloud token, a DSN, a chat id, an update id, a task id, an operation id — each a
distinctive literal that could only appear by leaking. (e) `testutil.GatherAndLint` over that
registry reports no problem. (f) A walk of `internal/health`'s own non-test files — rooted the same
way (a) is, through `repotest.Root` — asserting none
contains `panic(`, `log.Fatal`, `os.Exit`, `MustRegister` or `MustNew` — AC32's source property,
which neither the bucket table test nor the server test asserts and which no enabled lint rule
asserts either. **Proved discriminating against both categories it covers**, which is the half
round 3 got wrong: run once against a scratch file carrying a bare `panic(` and required to fail,
**and once against a scratch file carrying `prometheus.MustRegister(c)` and required to fail** —
the second is the proof that separates this walk from round 3's, which could not have detected a
library `Must…` call at all. Both scratch files live in this proof's own temp copy — which is also
what stops the `MustRegister` one from redding guard (a).
(g) **The design consolidated the helper in subtask 1; this is how it verifies the helper stayed
consolidated.** Over every `.go` file in the module: **zero repository-root helpers *of the
file-location-ascent kind* outside `internal/repotest`, exactly one inside it, and zero
declarations anywhere under the old unexported spelling `repoRootPath`.** Without this the
consolidation is a one-time tidy that the next test binary needing a root quietly undoes — which
is precisely how the module reached several copies in the first place.
**The class qualifier is not decoration — without it the "and no other file" half of the
validation below is unsatisfiable.** D19 defines the kind by mechanism: derives the root from its
own source file's location and ascends a fixed number of directory levels. A resolver that obtains
the root another way is out of class by definition, not by exemption, and `cmd/commentrefs` holds
two that ask git for it. The guard must not flag them, and would have to if its scope were
"resolves the repository root".
**How the guard recognises "a resolver" is the load-bearing part, and naming it by identifier
would make the guard green for the case it exists to catch** — a copy called `healthRepoRoot` or
`rootPath` passes a name-matching rule while re-introducing exactly what was consolidated. So the
guard recognises the
*behaviour*, over the parsed AST rather than the text: **a function whose result derives from
`runtime.Caller` and ascends a fixed number of directory levels by any spelling** — a chain of
`filepath.Dir`, a `filepath.Join` carrying `".."` components, or a mixture of the two. Round 5
wrote the rule as "repeated `filepath.Dir` ascents", which was itself a green instrument: one of
the resolvers live in this repository right now spells its ascent
`filepath.Join(filepath.Dir(file), "..", "..")` and that rule cannot see it (D19). The
identifier check for `repoRootPath` stays as a second, cheaper half: that spelling is the one the
module actually carried, so it is the one a half-finished revert would leave behind. `runtime.Caller` is the cheap first filter, and it is coextensive with **the
file-location-ascent kind** — not with repository-root resolution in general, which is the
over-claim D19 withdraws — so the AST ascent test refines that filter rather than replacing it,
and a resolver reaching the root without consulting its own file's location never enters the
guard's scope at all.

**Guard (g)'s own validation is against the pre-change tree, not against scratch files it was
written to catch.** Scratch files prove only that a rule detects the shape its author had in mind,
which is exactly how round 5's rule passed while missing a live site. The protocol, in a
`t.TempDir()` copy and never in the worktree: copy the module's `.go` files into it, then
overwrite **the files that exist at subtask 1's parent commit** with their content from `git show
<subtask-1 parent>:<path>` — that is the in-class resolvers' own files and nothing else;
`internal/repotest`'s files do not exist at that commit, so they are *removed* from the copy
rather than restored, which is what makes the copy a faithful pre-change tree. Run the guard
against that root and **require it to name every in-class resolver D19 derives and no other
file** — the git-based pair in `cmd/commentrefs` being present in the copy and required *not* to
appear, which is the assertion that proves the class qualifier is doing work. Then run the guard
against the real root and require green. Red-with, green-without, over a real tree, in both
directions, with the worktree untouched throughout. The three scratch-file proofs stay as cheap
regression cover for the detection rules themselves —
`func repoRootPath(...)`, a differently-named `filepath.Dir`-chain clone, and a differently-named
`filepath.Join(..., "..", "..")` clone, the third being the spelling round 5 could not see.
(h) No **non-test** file in the module imports `internal/repotest`, parsed rather than
grepped so a mention in a comment is not a false positive. This holds D17's third constraint: the
package imports `testing`, so a non-test importer would link the testing package into whatever
imports it, and the one thing this module already guards against in that direction is machinery
reaching `cmd/bot`'s link graph. The constraint is the design's own; nothing outside this document
asserts it, which is why it is asserted here.
(i) AC21's independence clause, which nothing else asserts: `internal/health/canary.go` and
`internal/health/probe.go` import no `pgx`, no `pgxpool`, no `internal/store` and no
`internal/scheduler` symbol. It must be **file-scoped**, not package-scoped, because the same
package legitimately imports `pgxpool` in `pool.go` and `internal/scheduler` in `scheduler.go` —
so no package-wide rule can cover it, and "the canary keeps probing while the database is
unreachable" is otherwise discharged by construction alone, with a later edit wiring a store
handle into the probe path shipping green. The runner scenarios cannot cover it either: they use
fake `Prober`s and never touch the property. It is a few lines on the walk guard (a) already
performs. Proved discriminating against a scratch import added to the temp copy's `probe.go` —
the tracked file is never edited, which matters more here than for the other proofs because
`probe.go` is production source and a failed restore would leave it dirty.
**AC31 is not in this subtask's list and was wrongly claimed there before**: none of these guards
inspects comments. It is discharged by `make comment-refs`, which rides in AC33's gate run at the
end of this same subtask.
`[derived → AC2, AC4, AC5, AC6, AC10, AC11, AC21, AC23, AC24, AC32]` — guards (g) and (h) carry no
AC by design: they verify D17's own consolidation and its test-only property, which are decisions
this document makes rather than criteria it discharges.

**Subtask 12 — no test.** The alert contract is prose; AC26 and AC27 are checked by reading it and by
the index entry.

## Open questions

- **An in-process gauge for the cross-leg derived signal.** Deferred by the spec's Key decisions;
  revisit if the infrastructure pass finds the expression awkward to write against scrape gaps.
- **Alert threshold values** — how many consecutive canary failures, and what rate of update-lag
  growth. The contract fixes the shapes and the names; the numbers are the infrastructure pass's.
- **Whether the cloud canary bot is §12.5's test bot or a third one.** This task defines only the key
  the token arrives through and the behaviour when it is absent.
- **A readiness endpoint beside the metrics path.** Not asked for by `docs/DESIGN.md`; #24 lists a
  readiness signal in its own scope and may want one when it owns start-up.
- **The `telegram-bot-api` instance's `/stats` shape.** Out of scope here; the alert contract may
  eventually name the series the infrastructure pass scrapes from it, which that pass owns.
- **Whether `labgame_ingest_update_lag_seconds` should sample only an update's first observation.**
  The design samples every observation whose `LagKnown` is true, which is what AC7 asks for and no
  more: a retried update contributes a sample per attempt, each with a larger lag. That is deliberate
  — a retry storm *is* the loop falling behind, and the star of the dashboard is a stall indicator —
  but it means the histogram is weighted by attempt count, and the alert contract says so. Revisit if
  the infrastructure pass wants a per-update distribution instead of a per-handling one.
- **`repoRootPath`'s duplication — decided, not open.** Round 3 raised it as a routing question
  between a follow-up issue and a scope widening. The owner took the widening: spec 9396d3c
  directed that it be taken in this task. The consolidation is subtask 1, the package is D17's
  `internal/repotest`, and guards (g) and (h) hold it. It carries no acceptance criterion and needs
  none: consolidating a helper the design would otherwise have copied again is a design decision,
  and guards (g) and (h) are how the design tests its own decision. Recorded here as the answer rather than deleted, so a later reader meets the decision
  instead of re-deriving the question. **`walkGoFiles` is explicitly not part of it** — one
  declaration module-wide triggers no rule, and consolidating it would be scope nobody asked for.
