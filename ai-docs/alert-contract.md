# Alert contract — the health surface

The bot exports a Prometheus metrics endpoint and runs two Telegram canary
legs. This page is the contract between that surface and the infrastructure
pass that wires Prometheus, the alert rules and the alert channel: it fixes
**names and shapes**, and it fixes **no numbers**.

| Fixed here | Fixed by the infrastructure pass |
|---|---|
| Metric family names; label names; each label's value set | Every threshold, window, quantile and `for:` duration |
| Which families an alert reads, and the expression shape over them | The scrape interval and the retention |
| The condition shape (what makes the expression fire) | Routing, grouping, inhibition, and the severity label's spelling |
| The severity **class** of each alert | Which humans each class reaches |

A rule that needs a series this page does not list needs a **code change
first** — the bot exports exactly the families catalogued below, and nothing
computes a derived health signal inside the process.

**Symbols used below, all unbound here:** `W` an evaluation window, `N` a
probe count, `q` a quantile, `T` a duration threshold, `D` a `for:`
duration. Where an expression needs a number, it carries one of these.

## 1. Where the surface lives

- The endpoint is `/metrics`, served by the bot's own HTTP server. Its
  listen address is the `LAB_GAME_HEALTH_METRICS_ADDR` variable, which
  defaults to a loopback address — a scrape from another host needs either
  a changed value or a local scraper, and that is the infrastructure pass's
  choice to make.
- The canary cadence is `LAB_GAME_HEALTH_CANARY_INTERVAL`, one value driving
  both legs on the same tick, defaulting to once a minute. Every expression
  below that counts probes assumes `W` covers at least `N` such intervals;
  the pass sets both against the configured cadence, not against the
  default.
- The cloud reference leg is enabled by `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN`
  and addressed by `LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL`. An empty or
  absent token disables the leg outright — see § 5.
- **The alert channel must not depend on the bot's own instance**
  (`docs/DESIGN.md` § 13.2): an alert delivered through the bot whose
  instance just died never arrives. Mail, a separate alerter bot living on
  the cloud Bot API, or anything else independent — the pass picks one, and
  "through the production bot" is not a candidate.

## 2. How a series comes into existence

**No label combination is pre-initialised.** A series appears at its first
observation and not before. This is deliberate — a pre-created
`leg="cloud"` series would make a leg that is switched off read as a leg
that is healthy — and every expression on this page is written to survive
it. Three consequences bind every rule the pass writes:

- **Never compare a possibly-absent series to zero.** `expr == 0` over an
  absent series yields an empty vector, not `1`: the rule silently never
  fires. Use `unless` against the positive form, or `absent()`.
- **A rate or an increase needs samples inside the window.** A subsystem
  that stops working entirely stops *feeding* its histogram, so a
  latency-quantile rule goes empty rather than going high. Where that
  matters, this page pairs the level rule with a silence rule.
- **`unknown` is a real label value, not a bug.** Every enum-to-label mapper
  is total: an out-of-range input maps to `unknown`. For the update kind it
  is genuinely observed — an unrouted update carries no kind — so a
  dashboard grouping by `kind` must expect it. For the scheduler and ingest
  outcome labels it is a declared-only branch that the shipped code has no
  path to produce; `unknown` appearing there is itself worth a look.

Histograms expose the usual `_bucket`, `_sum` and `_count` series. The
Go-runtime and process collectors, where the composition root installs them,
keep the client library's own `go_` and `process_` prefixes rather than this
project's.

## 3. The metric catalogue

Every family the bot registers, with its labels. One prefix, `labgame_`;
base units; `_total` on counters; `_seconds` on durations.

### Bot API transport

| Family | Type | Labels |
|---|---|---|
| `labgame_botapi_call_duration_seconds` | histogram | `method` |
| `labgame_botapi_responses_total` | counter | `method`, `code` |
| `labgame_botapi_rate_limited_total` | counter | `method` |
| `labgame_botapi_retries_total` | counter | `method` |

`code` is the HTTP status rendered decimal; `0` means no response was ever
received. Canary traffic never lands here — each leg has its own client and
its own observer, so these families describe production traffic only.

### Scheduler

| Family | Type | Labels |
|---|---|---|
| `labgame_scheduler_task_lag_seconds` | histogram | `type` |
| `labgame_scheduler_tasks_total` | counter | `type`, `outcome`, `failure` |
| `labgame_scheduler_loop_duration_seconds` | histogram | — |
| `labgame_scheduler_claim_batch_size` | histogram | — |
| `labgame_scheduler_loop_errors_total` | counter | — |

`outcome`: `done`, `noop`, `failed`, `unknown`. `failure`: `none`,
`handler`, `unregistered`, `deadline`, `rolled_back`, `unknown`. Task lag is
a gameplay-visible quantity — a wave promised in five minutes arriving in
seven — and the `type` cut is what shows whether notifications are delaying
gameplay edges. No scheduler alert is required by this contract; the series
are here so the pass can add one against its own operating experience.

### Update ingestion

| Family | Type | Labels |
|---|---|---|
| `labgame_ingest_update_lag_seconds` | histogram | `kind` |
| `labgame_ingest_handler_duration_seconds` | histogram | `kind`, `outcome` |
| `labgame_ingest_update_outcomes_total` | counter | `kind`, `outcome` |
| `labgame_ingest_undecodable_updates_total` | counter | — |
| `labgame_ingest_poll_duration_seconds` | histogram | — |
| `labgame_ingest_poll_batch_size` | histogram | — |
| `labgame_ingest_poll_errors_total` | counter | — |

`outcome`: `handled`, `duplicate`, `unrouted`, `failed`, `panic`,
`given_up`, `unknown`. `kind` carries the Bot API's own update-type token,
with `unknown` for an update no route claimed.

Two shapes to carry into any rule over these:

- **The lag histogram is sampled only when the update's own timestamp is
  known.** An update that carries none contributes no sample rather than a
  zero one, so the histogram's `_count` is not the update count — the
  outcome counter is.
- **The outcome counter counts handler attempts and attempt-less
  settlements, not updates.** A retried update contributes several. That is
  what the family's name says, and a rule that reads it as traffic volume
  overstates under retry.

### pgx connection pool

| Family | Type | Labels |
|---|---|---|
| `labgame_pgxpool_conns` | gauge | `state` (`idle`, `acquired`, `constructing`) |
| `labgame_pgxpool_total_conns` | gauge | — |
| `labgame_pgxpool_max_conns` | gauge | — |
| `labgame_pgxpool_acquires_total` | counter | — |
| `labgame_pgxpool_acquire_duration_seconds_total` | counter | — |
| `labgame_pgxpool_empty_acquires_total` | counter | — |
| `labgame_pgxpool_empty_acquire_wait_seconds_total` | counter | — |
| `labgame_pgxpool_canceled_acquires_total` | counter | — |
| `labgame_pgxpool_new_conns_total` | counter | — |
| `labgame_pgxpool_max_lifetime_destroys_total` | counter | — |
| `labgame_pgxpool_max_idle_destroys_total` | counter | — |

Read at scrape time, never from a cached snapshot. Saturation is
`labgame_pgxpool_conns{state="acquired"} / labgame_pgxpool_max_conns`.

### Canary

| Family | Type | Labels |
|---|---|---|
| `labgame_canary_probes_total` | counter | `leg` (`own`, `cloud`), `outcome` (`success`, `failure`) |
| `labgame_canary_probe_failures_total` | counter | `leg`, `reason` |
| `labgame_canary_probe_duration_seconds` | histogram | `leg`, `outcome` |

`reason` is a classification, never error text: an HTTP status code rendered
decimal where a response arrived, and otherwise one of `timeout`, `canceled`
or `network`. It is the right label to render in an alert body and to route
on; it is not what any condition below tests.

`leg="own"` probes the chain this project owns — own `telegram-bot-api`
instance, MTProto, the DC behind it. `leg="cloud"` probes the cloud Bot API
directly, with a **different bot's** credential, as a reference.

## 4. The alerts

### 4.1 Consecutive canary failures

| | |
|---|---|
| **Reads** | `labgame_canary_probes_total` |
| **Severity** | `leg="own"` → **page**. `leg="cloud"` → **ticket** |

**Expression shape** — per leg, `N` failures inside `W` and no success
inside `W`:

```
  sum by (leg) (increase(labgame_canary_probes_total{outcome="failure"}[W])) >= N
unless
  sum by (leg) (increase(labgame_canary_probes_total{outcome="success"}[W])) > 0
```

**Condition shape.** `W` covers at least `N` probe intervals; both sides
carry only `leg`, so the `unless` matches leg to leg. The `unless` — rather
than an `and … == 0` — is what makes the rule work on a leg whose `success`
series has never existed: a leg failing since process start has no success
series at all, and a zero-comparison against it yields nothing.

**Why not literally "N in a row".** A counter pair records how many probes
succeeded and how many failed in a window; it does not record their order,
and no PromQL expression recovers it. "`N` failures and no success in a
window covering `N` intervals" is the expressible form of the design's *N
canaries in a row*, and it is strictly stronger against flapping: a single
success anywhere in the window clears it.

**Why the two legs differ in severity.** A failing own leg means players
cannot reach the bot; a failing cloud leg means the *reference* is
unavailable, which costs diagnosis, not gameplay — and, from a fresh
deployment, most often means the credential rather than the cloud API
(§ 5).

### 4.2 Update-lag growth

| | |
|---|---|
| **Reads** | `labgame_ingest_update_lag_seconds`, `labgame_ingest_update_outcomes_total` |
| **Severity** | **page** |

Update lag — the distance between an update's own timestamp and the moment
it is handled — is the earliest indicator that ingestion is stuck, and the
design calls it the star of the dashboard.

**Expression shape, the level form** — a high quantile above a threshold:

```
histogram_quantile(q, sum by (le) (rate(labgame_ingest_update_lag_seconds_bucket[W]))) > T
```

**Expression shape, the growth form** — the same quantile rising rather than
merely being high, which is what catches a slow stall before it reaches any
fixed `T`:

```
  histogram_quantile(q, sum by (le) (rate(labgame_ingest_update_lag_seconds_bucket[W])))
>
  histogram_quantile(q, sum by (le) (rate(labgame_ingest_update_lag_seconds_bucket[W] offset W)))
```

**Condition shape.** Either form, sustained for `D`. Both aggregate the
`kind` label away by summing over `le` alone; a per-kind cut is available by
adding `kind` to the `by` clause, and is a dashboard question rather than an
alerting one, since one quiet update kind can hold a high quantile
indefinitely on a handful of samples.

**Its necessary companion — silence.** A loop that has stopped entirely
feeds the histogram nothing, so the quantile expressions above go **empty**,
not high: the lag alert cannot fire on the worst case it exists to catch.
The pass therefore wires a second rule beside it:

```
absent(rate(labgame_ingest_update_outcomes_total[W])) or sum(rate(labgame_ingest_update_outcomes_total[W])) == 0
```

**Severity: page**, and the two rules are wired as one alert with two
conditions rather than as two independently-tuned rules — they cover the two
halves of the same failure. Note the second condition's own limit: a bot in
a genuinely idle chat also observes no updates, so `W` must be long enough
that ordinary quiet does not trip it. That is a threshold decision, and it
belongs to the pass.

### 4.3 The cross-leg expression: own instance sick, cloud healthy

| | |
|---|---|
| **Reads** | `labgame_canary_probes_total`, both legs |
| **Severity** | **page** — and it is the one that tells an operator *which layer* to look at |

**Expression shape:**

```
(
    sum by (leg) (increase(labgame_canary_probes_total{leg="own",outcome="failure"}[W])) >= N
  unless
    sum by (leg) (increase(labgame_canary_probes_total{leg="own",outcome="success"}[W])) > 0
)
and on ()
  sum(increase(labgame_canary_probes_total{leg="cloud",outcome="success"}[W])) > 0
```

**Condition shape.** The left operand is § 4.1 restricted to the own leg;
the right asserts the cloud leg answered at least once in the same window.
`and on ()` matches on the empty label set, so the label-less right operand
gates the whole left side.

**What it means.** Telegram is up and reachable from this host, and the part
that is failing is the part this project owns: the self-hosted
`telegram-bot-api` instance, or its link to Telegram. That is the
diagnosis the dual canary exists to produce, and it is why the two legs
carry different credentials against different endpoints.

**What it means when the cloud leg is configured off.** The right operand
matches **no series at all**, so the `and` yields an empty vector and this
expression can never fire — for any state of the own leg. It is not
"healthy" and it is not "sick": it is **unevaluable**, and its silence is
evidence of nothing. Two obligations follow for the pass:

- Never read a quiet cross-leg rule as a healthy own instance. With the
  cloud leg off, § 4.1's own-leg rule is the only canary alert that can
  fire, and it is sufficient for *whether* the bot is reachable — only the
  *which layer* half is lost.
- Wire § 5's absence test, so the difference between "no cloud leg
  configured" and "the cloud leg is failing" is visible on the dashboard
  rather than inferred from a rule that stayed quiet.

## 5. The absence test — a disabled leg is not a healthy leg

**The test, over the whole vector:**

```
absent(labgame_canary_probes_total{leg="cloud"})
```

**This is a recording / inhibition rule, not a page.** A cloud leg switched
off is a supported operating mode, not an incident. Its uses are to annotate
the dashboard, and to inhibit the cloud-leg and cross-leg rules so their
silence is explained rather than merely observed.

**Never write it over a single `outcome` value.** No label combination is
pre-initialised (§ 2), so:

- a **healthy** cloud leg has no `outcome="failure"` series, and
- a **failing** cloud leg has no `outcome="success"` series.

`absent(labgame_canary_probes_total{leg="cloud",outcome="success"})` is
therefore true of a leg that is failing every probe *and* of a leg that was
never configured — the exact confusion this test exists to remove. Only the
absence of the **whole** `leg="cloud"` vector means "not configured".

**One more discriminator the pass must add.** `absent()` is also satisfied
when the process is down or the scrape is failing, because then *every*
series is absent. Pair it with the scrape's own `up` series for the bot's
job, or with the presence of any other `labgame_` family, so "the leg is
off" is distinguished from "the bot is gone".

### The placeholder trap

`.env.example` ships a placeholder in `LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN`,
because every value in that file must be non-empty. An operator who copies
it to `.env` and fills in the real values therefore starts with the cloud
leg **enabled under a credential that is not the cloud bot's**. Two distinct
things happen, and an operator reading an alert should recognise both:

- **The shipped placeholder is not a well-formed Bot API token**, so the
  cloud leg's client cannot be built at all. The builder refuses the
  **whole** canary — own leg included — with an error naming the
  cloud-reference leg, and no canary series is exported by anything. A
  deployment where `labgame_canary_probes_total` is absent for *both* legs
  while the rest of the surface scrapes normally is this case, not an
  outage.
- **A credential that is well-formed but wrong** — a revoked token, another
  environment's token, the production bot's token — builds the leg
  normally, and it then fails **every** probe from the very first tick, with
  a `reason` label carrying an HTTP status code rather than `timeout` /
  `canceled` / `network`, because the request reached the cloud frontend and
  was refused there. A cloud leg that has never once succeeded since process
  start is far more likely this than an outage of the cloud Bot API.

**The fix in both cases is the same, and it is the only spelling available
to an operator who started from the example: set the key empty.**

```
LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN=
```

An absent key and a present-but-empty key behave identically — the leg is
off, the own leg still runs, and start-up raises no error. Deleting the line
is **not** the answer: the example file is a manifest whose key set a test
asserts against the loader's, and a deleted line diverges from it.

## 6. Left to the infrastructure pass

- Every number, per the table at the top of this page: thresholds, windows, quantiles, `for:`
  durations, the scrape interval.
- The alert channel and its independence from this project's own instance.
- The `telegram-bot-api` instance's own `/stats` surface. It is not part of
  this contract — the bot does not read it and does not re-export it — and
  the pass that owns the instance's configuration decides which of its
  series to scrape and how to name them.
- A readiness endpoint beside `/metrics`, if start-up and shutdown
  orchestration ever wants one.
