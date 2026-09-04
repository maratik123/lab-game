# Bot API transport over telego: retries, `retry_after`, rate limits, base-URL axis

**Source:** issue #19
**Date:** 2026-09-04
**Tracked in:** #19

Every outbound Telegram call in this project goes through one package. That package is
the only place a retry is decided, the only place a `retry_after` is honoured, the only
place a rate limiter delays a call, and the only place the per-method health numbers of
§13.2 are observed. It is deliberately thin: telego's low-level layer is generated
one-to-one from the official Bot API documentation, so coverage is complete by
construction, and telego's helper layer is not used — retries, idempotency and rate
limits are ours
[source: 751c2ba:docs/DESIGN.md:302 · `grep -n "telego" docs/DESIGN.md`].

The failure this package exists to prevent is a tight retry loop: it is the classic way
to earn a flood ban, the ban attaches to the bot id, and reissuing the token does not
clear it
[source: 751c2ba:docs/DESIGN.md:303 · `sed -n '303p' docs/DESIGN.md`;
`ai-docs/domain-invariants.md` § 6].

## Scope

1. **A new package `internal/tg`** — a client over telego's low-level generated API.
   Name settled here (issue #19 left it to the spec): `tg`, giving `tg.Client` and not
   a stuttering `tg.TGClient` (`AGENTS.md` § API Naming). Every outbound-call method
   takes `ctx context.Context` first and no type in the package stores a context.
2. **The HTTP and JSON swap.** telego's defaults are `fasthttp` and `go-json`; this
   project uses `net/http` and `encoding/json`
   [source: 751c2ba:docs/DESIGN.md:302 · `sed -n '302p' docs/DESIGN.md`]. Confirming
   that the swap is expressible in the telego version this task pins is the design
   document's obligation (see *Technical constraints*).
3. **Adding `github.com/mymmrac/telego` to the module graph** — via `go get`, never a
   hand-edited `go.mod`, with the stated reason in the design document
   (`AGENTS.md` § Dependency Versions). Today `go mod why -m github.com/mymmrac/telego`
   answers *"main module does not need module github.com/mymmrac/telego"*
   [source: 751c2ba:go.mod · `go mod why -m github.com/mymmrac/telego`], so this task
   is where the KD-2 choice first becomes real code.
4. **Retry policy — retry only what provably never reached Telegram, plus 429 and 5xx**
   (owner, round 1: *"Never on doubt"*). A failure where the request went out and no
   response came back is **ambiguous** and is **not** retried: a Bot API call carries no
   idempotency key, so a re-sent `sendMessage` would be a second message in a player's
   chat. Retries use exponential backoff with jitter, and between any two attempts of one
   call there is a strictly positive delay — "no tight loop" is a property of the code,
   not a hope about timing. The retrying is bounded by a **maximum attempt count**
   (owner, round 3), configurable per Scope 7, with the caller's context deadline
   binding on top of it per Scope 5.
5. **Honest 429 handling.** A `retry_after` from Telegram is honoured exactly and never
   shortened. It is also never waited out past the caller's context deadline: when the
   required wait would outlive the deadline, the call returns a typed error carrying the
   `retry_after` value so the caller (ultimately the queue in #43) can reschedule.
6. **Two rate limiters, bounded only where a figure exists.** A **global** limiter and a
   **per-chat** limiter keyed on **(chat id, method class)**, each class carrying its own
   configurable allowance (owner, round 1). Two properties the mechanism must have, both
   consequences of the round-3 investigation recorded in *Technical constraints*:
   - **A class is bounded by default only where a published figure supports the bound.**
     The message-delivering class is bounded — 30 messages/second globally and about
     20 messages/minute into one chat
     [source: 751c2ba:docs/DESIGN.md:302 · `grep -n "30 msg/sec" docs/DESIGN.md`].
     Every other class exists in the mechanism with its own key, and defaults to
     unbounded, because no figure this repository can cite supports throttling it. An
     operator can bound any class later without a code change; that is the whole point
     of the class key.
   - **The algorithm is a leaky bucket in its shaping form** (owner, round 3): calls
     leave a key at a steady interval rather than in a burst. Two things this does
     **not** license, both in *Technical constraints*: a call is never discarded to
     make room, and one steady interval is not by itself a longer-window cap.
   - **One chat key must be able to carry more than one window.** The published guidance
     the investigation reports contains a short-window per-chat figure *alongside* the
     per-minute one, so a bucket that can express only "N per minute" would permit a
     burst the guidance advises against. The mechanism must express both a short-window
     rate and a longer-window cap on the same key; which values each carries is settled
     by the design after it verifies them (see *Technical constraints*).
7. **The configuration surface: environment keys carrying compiled-in defaults**
   (owner, round 2: *"env-keys with backed default values"*). Each retry and rate-limit
   value is a `LAB_GAME_`-prefixed variable read by `internal/config` and exposed as a
   typed field, **optional** — absent means the documented default applies, present means
   the value is parsed and validated with the same fail-loud, name-the-key discipline as
   every other variable. Each key is listed in `.env.example` carrying its default as the
   documented value. This is a new *class* of variable for that layer, and *Technical
   constraints* records exactly which live claim it falsifies and which it does not.
8. **Per-method instrumentation hooks.** An observation point the client calls for every
   outbound call, carrying at least: method name, latency, response code, whether the
   response was a 429, and how many retries the call consumed — exactly the four things
   the health dashboard asks of the transport
   [source: 751c2ba:docs/DESIGN.md:406 · `sed -n '406p' docs/DESIGN.md`]. The hook is an
   interface or callback owned by this package; no metric-registry package is imported
   here. #23 registers the values against Prometheus.
9. **A minimal fake Bot API server** sufficient for this package's own tests: it must be
   able to answer with success, with a 429 carrying `retry_after`, with 5xx, with a
   transport-level failure, and with a delayed response. Bot API is plain HTTP+JSON,
   which is what makes the fake cheap
   [source: 751c2ba:docs/DESIGN.md:348 · `sed -n '348p' docs/DESIGN.md`]. Its **placement**
   — a test-only helper inside `internal/tg`, or a package importable by other test
   binaries — is left to the design document: at least three consumers are already
   named (this package's tests; #43's ordering / rate-limit / 429 tests; #44's eval
   harness), which is the call-site count that makes the shared-package-versus-duplication
   choice a design decision rather than a spec one.
10. **Context cancellation on every call, with no unbounded blocking.** A call parked in
    a backoff sleep, in a `retry_after` wait, or behind either limiter returns promptly
    when its context is cancelled or its deadline passes.
11. **The base URL is consumed as one configuration value, and it changes no limit.**
    `internal/config` already supplies it as `Config.BotAPIBaseURL url.URL`
    [source: 751c2ba:internal/config/config.go:47-50 · `go doc ./internal/config.Config`];
    the three design values — own instance, cloud `api.telegram.org`, fake server — are
    three values of that one variable, not an enum
    [source: 751c2ba:docs/DESIGN.md:304 · `sed -n '304p' docs/DESIGN.md`]. The limiters
    are **not** conditioned on which value is in use: a self-hosted instance does not
    relax message rate limits, which are enforced by Telegram's core
    [source: 751c2ba:docs/DESIGN.md:364 · `grep -n "рейт-лимиты сообщений" docs/DESIGN.md`;
    751c2ba:ai-docs/key-decisions.md:27 · `grep -n "KD-9" ai-docs/key-decisions.md`].
12. **Propagation** — every site whose claim this diff falsifies is updated in the same
    PR; membership is decided by `AGENTS.md` § Propagation Rule step 4, and the sites
    known at spec time illustrate the class without bounding it: the generalised
    requiredness sentence in `internal/config/env.go`, `.env.example`,
    `ai-docs/context.md` § Status (which currently says there is no Telegram client),
    `ai-docs/plans/INDEX.md`, and `ai-docs/key-decisions.md` (KD-2, once telego is real).
    History surfaces are **excluded by the same rule**: `ai-docs/plans/done/**` records
    what was decided then and is left untouched.

## Out of scope

- **The update loop, dispatch and inbound idempotency — #22.** Long polling, the offset,
  the router and `operation_id` are that issue's.
- **The `ALLOWED_CHAT_IDS` outbound gate — #22.** Its scope already claims "the
  `ALLOWED_CHAT_IDS` gate, applied where it cannot be bypassed by a handler"
  [source: issue #22 · `gh issue view 22 --json body`], and #22 depends on #19. This
  task therefore does **not** enforce the allowlist; it must leave the un-bypassable
  place available — the per-call seam that scope item 8 already requires — so #22 can
  put the gate there without reworking the client. Nothing here may make a handler able
  to route around that seam.
- **The notification queue — #43.** It owns the queue and the per-chat daily budget;
  this task owns the limiters the queue sends through.
- **The `/metrics` endpoint, the registry, and the canaries — #23.** This task exposes
  observations; #23 registers them and schedules `getMe` probes.
- **The full eval harness — #44.** The fake server here covers this package's tests only.
- **Running a self-hosted `telegram-bot-api` instance** (§12.2), its `api_id`/`api_hash`,
  and the migration runbook — the infrastructure pass.
- **telego's helper layer**, in whole or in part
  [source: 751c2ba:docs/DESIGN.md:302 · `sed -n '302p' docs/DESIGN.md`].
- **Webhooks.** Long polling is the design's ingestion choice (§12.1); no second path.
- **Making every existing configuration variable optional.** The new keys are optional
  because they carry defaults; the six variables already declared stay exactly as they
  are — required, no default. See *Technical constraints*.
- **Editing `docs/DESIGN.md`.** The round-3 investigation reports a published per-chat
  figure that §11 does not name. `docs/DESIGN.md` is decisions and is never redesigned
  without an explicit user request (`AGENTS.md` § Project); the gap is recorded under
  *Open questions* for the owner, not closed here.

## Deferred

- Swapping telego for `go-telegram/bot` | the design names it as the fallback if telego
  disappoints, and nothing observed so far triggers that | no — the escape hatch is
  recorded in KD-2; `go-telegram-bot-api` stays refused as frozen
  [source: 751c2ba:docs/DESIGN.md:307 · `sed -n '307p' docs/DESIGN.md`]
- Paid broadcasts, which the round-3 investigation reports as the documented way past the
  global send figure | the reported entry requirements are far outside an MVP shipping to
  one friendly chat (§14), and the fan-out it would serve is a post-MVP season feature |
  no — revisit only if fan-out volume ever approaches the free figure
- A circuit breaker in front of the base URL (fail fast while the instance is wedged,
  instead of retrying every call) | the incident that motivates it is real (§12.2) but
  the diagnosis lives in #23's canaries, which do not exist yet | yes, once the canaries
  produce the signal it would trip on
- Automatic failover between the own instance and cloud `api.telegram.org` | the design
  makes the cloud an **emergency fallback** switched by configuration, and the documented
  migration needs `logOut`/`close` and up to ten minutes of quarantine — automatic
  flipping would split updates between instances | no — it is a runbook, §12.2
- Priority or fairness between callers competing for the global limiter | there is one
  fan-out scenario today and the queue in #43 is its only heavy user | yes, if a second
  bulk sender ever appears
- Recovering an ambiguous send by asking Telegram what landed (read-back / dedupe by
  content) | it is the only way to be both duplicate-free and loss-free, and it is far
  more machinery than the MVP's traffic justifies | yes, if ambiguous failures turn out
  to lose player-visible messages in practice

## Key decisions

| Question | Decision |
|---|---|
| Package name (#19 left it to the spec) | **`internal/tg`**, exposing `tg.Client`. No stutter, no `Telegram` prefix inside the package (`AGENTS.md` § API Naming). |
| Which telego layer | **Low-level only** — generated types and methods, complete coverage by construction. The helper layer is not used, in whole or in part (KD-2). |
| Which failures may be retried, given a Bot API call has no idempotency key | **Never on doubt** (owner, round 1). Retried: failures that provably never reached Telegram, plus 429 and 5xx. Not retried: any failure where the request was sent and the outcome is unknown — the caller sees the typed error and decides. The cost is accepted explicitly: a message may be lost where a retry would have delivered it, and that is preferred to a duplicate arriving in a player's chat. |
| Does running our own Bot API instance change the limits — the round-2 research directive | **No.** This repository already says so: a self-hosted instance does not relax message rate limits, which Telegram's core enforces [source: 751c2ba:docs/DESIGN.md:364 · `grep -n "рейт-лимиты сообщений" docs/DESIGN.md`; 751c2ba:ai-docs/key-decisions.md:27 · `grep -n "KD-9" ai-docs/key-decisions.md`]. The round-3 investigation reports the upstream maintainer answering the same question the same way in a closed `tdlib/telegram-bot-api` issue, and reports that the server's own README lists what `--local` mode enables without mentioning rate limits in either direction. Consequence: the limiters are unconditional across all three values of the base-URL axis — there is no relaxed mode to implement, and no branch on base URL anywhere in the limiter path. |
| What one per-chat rate-limit bucket covers | **Keyed on (chat id, method class)**, each class with its own configurable allowance (owner, round 1) — the mechanism is exactly the one chosen, unchanged. |
| Which classes are bounded **by default** | **Only the message-delivering class.** Its bound comes from the figures this repository states [source: 751c2ba:docs/DESIGN.md:302 · `grep -n "30 msg/sec" docs/DESIGN.md`]. Every other class has its key and defaults to **unbounded**. Reason, from the round-3 investigation: the only figures with an official source are about messages, while the frequently-quoted per-chat *edit* figure is labelled by the very page that reports it as revealed outside the official documentation, and that page advises against relying on undocumented limits for throttling. Throttling on an unciteable number would delay gameplay against a limit Telegram may not impose; the honest posture is to bound what is published and handle the rest reactively — which the transport already does correctly, since a 429 is honoured exactly and is safe to retry. This is a **default**, not a ceiling: bounding another class later is an environment change, not a code change. |
| Whether the per-chat limiter applies to private chats as well as groups | **Yes, uniformly.** The design's observation that private chats are request-response and cannot exceed the limit [source: 751c2ba:docs/DESIGN.md:305 · `sed -n '305p' docs/DESIGN.md`] is a statement about traffic patterns, not a licence to remove the guard — and the round-3 investigation reports the upstream maintainer making the same traffic-pattern point ("nothing needs to be passed to rate-limiter if all messages are sent in response to user actions"). A bucket that is never approached costs nothing; a chat class exempted by construction is a flood-ban path no test would notice. |
| How many windows one chat key carries | **At least two: a short-window rate and a longer-window cap.** The investigation reports a published per-chat per-second figure alongside the per-minute one, and a bucket expressing only the per-minute cap would allow a burst that the per-second guidance advises against. The spec fixes the *expressibility*; the design fixes the values after verifying them, and a window whose figure it cannot verify defaults to unbounded under the row above. |
| What charges the buckets | **Message-delivering calls**, not every HTTP request. Every figure in play is stated in *messages* — 30 msg/sec and ~20 msg/min [source: 751c2ba:docs/DESIGN.md:302 · `grep -n "30 msg/sec" docs/DESIGN.md`] — so charging every request would throttle against a limit no source states. |
| Where the retry and rate-limit values live | **Environment keys with compiled-in defaults** (owner, round 2, verbatim: *"env-keys with backed default values"*), declared in `internal/config` alongside the existing variables and listed in `.env.example` with their defaults as the documented values. An absent key means the default; a present key is parsed and validated, and an invalid value is a start-up error naming the key. Rationale for putting them in `internal/config` rather than having `internal/tg` read the environment itself: that package's stated domain already includes *runtime settings*, and configuration is read once at start-up through one loader with no reload path — a second reader would fragment both properties. |
| What the defaulted-key class does to the config layer's invariants | **It narrows one over-broad sentence and contradicts nothing else** — the analysis, with pins, is in *Technical constraints*. In scope terms: `internal/config/env.go`'s "every variable is required; none has a compiled-in default" becomes a statement about the six variables it was written for, and the new tuning keys are documented as the optional-with-default class. KD-24 is untouched, because its requiredness consequence is about **file paths**, and no balance value gains a fallback. |
| How far the optional-with-default relaxation reaches | **Only to operational tuning keys** introduced by this task. Secrets, the base URL, the chat allowlist and the two file paths stay required with no default. Widening it further is nobody's decision here; see *Open questions*. |
| Handling of `retry_after` | **Honoured exactly, never shortened** (§11, §13.2 and `ai-docs/domain-invariants.md` § 6 all say so), which is also what the investigation reports the community guidance prescribing: wait the stated number of seconds, then retry. It is bounded only by the caller's context: if the wait cannot fit before the deadline, the call returns a typed error carrying the value instead of sleeping past it. That keeps "never shorten `retry_after`" and "no unbounded blocking" simultaneously true, and hands the reschedule to the layer that owns scheduling (#43). |
| What a give-up looks like to the caller | **A typed error** the caller can inspect — carrying at least the method, the last response code, the `retry_after` when there was one, the number of attempts made, and whether the failure was ambiguous (sent, outcome unknown). #43 must be able to branch on "retry later at time T" versus "this chat is gone" versus "unknown — do not re-send". Its exact shape is the design's. |
| What bounds the retrying — the retry budget | **A maximum attempt count** (owner, round 3). Exponential growth with jitter between attempts; the count is an environment key with a default. The sub-question of whether a `retry_after` wait is charged against the budget **dissolves** under this answer: a count-based bound has no time budget for a wait to consume, so a 429 pause costs one attempt and no more. The caller's context deadline remains the only bound on elapsed time, exactly as in Scope 5. The count's default value is the design's to choose and to justify — no source states one, so it is recorded there as a chosen number, not a sourced one. |
| Which algorithm the limiters use | **Leaky bucket, in the shaping (queue) sense — steady emission, no burst** (owner, round 3). The cited article describes two mechanisms under that one name, and the round-4 investigation reports it calling the *meter* version "exactly equivalent to (a mirror image of) the token bucket algorithm". That equivalence is what settles the reading rather than clouding it: the owner chose this option **over** a token-bucket option offered in the same question and described as burst-allowing, so the version meant is the one that is actually distinct from the rejected option — the queue/shaping version, whose stated property is a rigid output pattern at the average rate however bursty the input. Two supports for reading it that way rather than the other: choosing the meter version would have been choosing the rejected option by another name, and the shaping reading is the *stricter* of the two, which is the safe direction when the failure being designed against is a flood ban. Recorded as an inference, and flagged under *Open questions* so it costs one sentence to overturn. |
| Whether the limiter is a package or ours | **Left to the design, with an argued choice required.** `golang.org/x/time` is not reachable from this module today [source: 751c2ba:go.mod · `go mod why -m golang.org/x/time`]. **The established-package answer does not fall out automatically here, and this spec does not decide which package qualifies**: whether any given limiter package shapes at a steady interval or implements the burst-tolerant shape the owner rejected is a claim about that package's semantics, and this spec's author had no way to read its source. The design verifies a candidate's behaviour against its own pinned source before adopting it — a package's name is not evidence of its algorithm. The design must therefore either name a package it has confirmed matches, or carry the rejected-alternatives comparison `AGENTS.md` § Dependency Versions requires of an argued wheel, with KD-4 as the model. "It is only a few lines" and bare dependency aversion are refused outright as arguments. |
| Which telego version | Pinned by the design at implementation time via `go get`. The published list ends v1.11.2, v1.12.0, v1.12.1 [source: 751c2ba:go.mod · `go list -m -versions github.com/mymmrac/telego`]. |
| Instrumentation coupling | **No metric-registry import in `internal/tg`.** The package defines the observation point; #23 supplies the implementation. Neither `github.com/prometheus/client_golang` nor `golang.org/x/time` is reachable from this module today [source: 751c2ba:go.mod · `go mod why -m github.com/prometheus/client_golang`, `go mod why -m golang.org/x/time`], so any package chosen for the limiters is a dependency decision the design document must argue (`AGENTS.md` § Dependency Versions). |
| Fake-server placement | **Left to the design** — see Scope 9. Three consumers are already named, so the shared-package-versus-duplication choice is design's, and the spec must not pre-empt it in either direction. |
| Allowlist enforcement | **Not here — #22**, whose scope claims it. This task's obligation is negative: leave the per-call seam un-bypassable so #22 can install the gate there. |

## Technical constraints

- **Module and toolchain:** `github.com/maratik123/lab-game`, `go 1.26`
  [source: 751c2ba:go.mod:1-3 · `cat go.mod`].
- **Provenance of the round-3 investigation — read this before pinning any limiter
  default.** The owner's round-2 answer was a research directive, and the investigation
  was run by the interview orchestrator because this spec's author has **no web access**.
  Everything it returned is therefore an **unverified claim of the same standing as an
  issue body**, and none of it is pinned below as fact. Two rules follow, and the design
  document discharges both:
  - **In-repo figures are this spec's only authoritative numbers.** The 30 messages/second
    global and ~20 messages/minute per-chat figures are cited from `docs/DESIGN.md`
    because they are in this repository and re-readable at a commit. Every other figure
    the investigation reported — a per-chat per-second send figure, a per-group edit
    figure, the paid-broadcast thresholds — is deliberately **not** written into this
    spec as a number. Writing it here would give an unverifiable figure a spec's
    authority.
  - **The design verifies before it defaults.** Any limiter default beyond the in-repo
    figures is verified by the design against the upstream page it is attributed to, and
    the design records the verified wording **and its modality** — the investigation
    reports that the published phrasings differ in kind between figures ("avoid sending
    more than…" against "are not able to…"), which is exactly the label-versus-number
    distinction that survives transit badly. A figure the design cannot verify does not
    become a default; its class stays unbounded.
  - **One reported item is a caution, not an input.** The investigation could not reach
    the `ResponseParameters` definition — the page served truncated — so the exact
    wording of the `retry_after` field is unverified. The design does not need the web
    page for this: once telego is pinned, that type is **generated code in the module
    cache**, so the design confirms the field's name, type and semantics by reading the
    pinned dependency, which is re-readable at a version.
- **The `net/http` + `encoding/json` swap is a claim the design must confirm against the
  version it pins.** `docs/DESIGN.md` §11 records the swap as natively supported
  [source: 751c2ba:docs/DESIGN.md:302 · `sed -n '302p' docs/DESIGN.md`]; that is a
  project decision about an external library's surface, and the design document verifies
  it against the pinned version before building on it. If the pinned version cannot
  express the swap, that is a design-blocking finding, not something to work around
  quietly.
- **The optional-with-default keys against the config layer — what actually breaks, and
  what only looks like it does.** Three separate claims, checked one at a time:
  - **The disjointness test survives, and not by luck.** `recordingLookup` records a key
    at the moment it is *queried*, before delegating to the underlying lookup
    [source: 751c2ba:internal/config/disjoint_test.go:17-31 · `sed -n '17,33p' internal/config/disjoint_test.go`],
    so a key the loader consults and finds *absent* is still in the recorded set. The
    three-way equality — `.env.example` keys, loader-consulted keys, `config.EnvKeys()`
    — therefore holds for an optional key exactly as for a required one
    [source: 751c2ba:internal/config/disjoint_test.go:103-104 · `sed -n '89,105p' internal/config/disjoint_test.go`],
    provided the key is added to all three and the loader always queries it. A loader
    that skips the query on some branch would break the test — which is the correct
    failure, and worth knowing before the design picks an implementation.
  - **`.env.example` values must be non-empty**, because a test asserts it
    [source: 751c2ba:internal/config/disjoint_test.go:108-114 · `sed -n '107,115p' internal/config/disjoint_test.go`].
    Each new key therefore appears there carrying its default as the documented value —
    which is the honest documentation anyway.
  - **KD-24 is not contradicted.** Its requiredness consequence is scoped to file paths:
    *"each file path is a required environment variable with no compiled-in default"*,
    motivated by the balance file having no second source of truth
    [source: 751c2ba:ai-docs/key-decisions.md:65 · `grep -n "KD-24" ai-docs/key-decisions.md`].
    A rate limit is not a file path and not a balance number, so KD-24 stands unchanged
    and `internal/config/doc.go`'s no-fallback clause, which is about balance constants,
    stands too.
  - **The one live sentence this falsifies** is `internal/config/env.go`'s
    *"Every variable is required; none has a compiled-in default (design D10)"*
    [source: 751c2ba:internal/config/env.go:11-13 · `sed -n '9,22p' internal/config/env.go`].
    It generalises past the decision it cites, and this task narrows it. The merged
    design document that states D10
    [source: 751c2ba:ai-docs/plans/done/2026-09-04-config-layer-balance-files.design.md:358 · `sed -n '355,358p' ai-docs/plans/done/2026-09-04-config-layer-balance-files.design.md`]
    is a **history surface** and is left untouched, per `AGENTS.md` § Propagation Rule
    step 4.
- **The shaping leaky bucket — what the choice does and does not settle.** The algorithm
  is decided; four constraints ride on it, and three of them are places a faithful
  reading of the source would produce a defect here:
  - **We shape, we never police.** The round-4 investigation reports the article
    offering two responses to non-conforming traffic — delay it (shaping) or drop it
    (policing) — and the queue version discarding when the queue is full. **Discarding is
    not available to this transport in any form.** A packet is droppable; a `sendMessage`
    that loses its turn is a player-visible message that silently never happened. A call
    that cannot be emitted within its caller's deadline returns the typed error of Scope 5
    — the caller, and ultimately #43, then owns the reschedule. Silent drop is a defect,
    not a tuning choice.
  - **One steady interval is not a longer-window cap.** A single leaky bucket enforces one
    average rate. The per-chat requirement carries two windows (Scope 6), and a bucket
    whose interval satisfies the short window will still pass far more than the
    longer-window cap allows over a minute. The design therefore composes — buckets in
    series, or a bucket plus a counter for the longer window — and a single-rate
    implementation does not satisfy Scope 6 however correct its emission is.
  - **This queue is in-process and lives only as long as a call's context.** It is not a
    durable queue and it does not overlap #43, whose queue is persistent, rides the
    scheduler and survives restarts. Nothing here may acquire persistence, a table, or a
    lifetime beyond the calls currently in flight; a restart loses nothing because
    anything worth surviving a restart was #43's to hold.
  - **The behavioural contract the algorithm still has to meet**: honour the caller's
    context while waiting (a cancelled wait returns promptly, never after the full delay),
    take time from the same injectable clock the retry path uses so no test sleeps real
    seconds, hold state per (chat id, method class) key, and be safe under `-race` with
    concurrent callers. A call passing both the global and a per-chat bucket acquires them
    in a fixed order, for the same reason `store.Post` fixes its capture order.
- **Provenance of the round-4 read.** The owner cited a Wikiwand mirror; the investigation
  reports that host returning 403 to it and reads the Wikipedia article the mirror is
  built from instead. **The owner's exact page is therefore unread**, and mirror drift
  cannot be ruled out. This changes nothing structural — the two-mechanism ambiguity and
  its resolution stand on the option wording the owner chose between, which is in this
  interview's own record — but no wording is attributed to the page the owner cited.
- **"Provably never reached Telegram" has to be operational, not aspirational.** The
  retry decision of Scope 4 turns on distinguishing a request that was never written from
  one that was written and left unanswered. Go's HTTP errors do not hand that
  distinction over as a single flag, so the design names the mechanism it relies on and
  the classifier defaults to **ambiguous** whenever the evidence is not conclusive — the
  safe direction is fewer retries, since the alternative is a duplicate in a player's
  chat.
- **The balance YAML is not a candidate home for these values.** It is the sole source of
  *game constants*, and `internal/config`'s package doc scopes it that way
  (`go doc ./internal/config`); a rate limit imposed by Telegram's core is not a balance
  number, so `docs/DESIGN.md` §16.5 does not reach it.
- **Telemetry obligation.** This task adds no gameplay mechanic and moves no balance, so
  it declares no event and no posting signature (`AGENTS.md` § Domain Rules). What it
  does owe is the health surface: latency by method, response codes by method, a 429
  counter and a retry counter
  [source: 751c2ba:docs/DESIGN.md:406 · `sed -n '406p' docs/DESIGN.md`].
- **No `panic` / `log.Fatal` in production code** (`AGENTS.md` § Go Test Conventions);
  the client returns errors.
- **Concurrency is real here** — limiters and retries are shared state across goroutines,
  so `go test -race` is a required gate for this change, not an optional one
  (`AGENTS.md` § Go Test Conventions).
- **No wall-clock dependence in tests.** Timing assertions that sleep real seconds are
  slow and flaky; the design chooses an injectable clock or an equivalent, and the tests
  assert ordering and waits through it.
- **No secret in a tracked file.** The bot token reaches the client as a value
  (`config.Secret`), never as a literal in source, a test fixture, or a log line — the
  token must not appear in an error message or in an instrumentation label
  (`AGENTS.md` § Permissions).
- **Documentation:** package comment plus a doc comment on every exported item
  (`ai-docs/doc-convention.md`).
- **Executability of the acceptance criteria.** `Bash(go *)` and `Bash(make *)` are in
  `permissions.allow` [source: 751c2ba:.claude/settings.json · `jq -r '.permissions.allow[]?' .claude/settings.json`],
  so every criterion below is checkable through `go test` / `make verify` with no new
  permission grant. The fake server binds a loopback listener inside the test process;
  no criterion requires network egress or a running Telegram instance.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Package `internal/tg` exists, carries a package comment, and exposes a client constructed from a base URL, a bot token and its options. Every exported item carries a doc comment starting with its name. Every method that performs an outbound call takes `ctx context.Context` as its first parameter, and no type in the package has a field of type `context.Context`. |
| AC2 | No non-test Go file under `cmd/` or `internal/` imports a fasthttp or go-json package: the client's HTTP transport is `net/http` and its JSON codec is `encoding/json`. |
| AC3 | `github.com/mymmrac/telego` is a direct requirement of `go.mod` at a pinned released version, `go.sum` agrees, and the module hygiene gate reports no delta. The design document states the reason for the dependency. |
| AC4 | A test against the fake server asserts that a 429 response carrying `retry_after: N` is followed by no earlier attempt of that call than N later, measured through the injected clock — never a shortened wait. |
| AC5 | A test asserts that between any two attempts of one call there is a strictly positive delay, and that successive delays on repeated network failures grow rather than repeat. No code path performs an immediate re-attempt. |
| AC6 | A failure in which the request reached Telegram and the outcome is unknown is not retried: the call makes exactly one attempt and returns the typed error with its ambiguity flag set. A test drives this case against the fake server and asserts the attempt count. |
| AC7 | A failure that provably never reached Telegram is retried, as are 429 and 5xx responses. A test drives each of those three cases and asserts that more than one attempt was made. |
| AC8 | When the retry budget is exhausted, the call returns a typed error, distinguishable by the caller from a success and from a context error, carrying the method name, the last response code, the attempt count, the ambiguity flag, and the `retry_after` value when the final failure was a 429. A test asserts each field. |
| AC9 | When honouring a `retry_after` or a backoff would require waiting past the caller's context deadline, the call returns that typed error immediately instead of sleeping, and the error still carries the `retry_after` value. |
| AC10 | A test asserts that the global limiter admits no more than the configured number of message-delivering calls per unit time, and that a call which delivers no message into a chat does not consume global allowance. |
| AC11 | Per-chat allowance is keyed on both the chat id and the method class: a test asserts that traffic into one chat is limited, that traffic into a different chat is not delayed by it, and that a call of an unbounded class into a rate-limited chat is not delayed by the bounded class's allowance. |
| AC12 | One chat key carries at least two windows: with a short-window rate and a longer-window cap both configured, a test asserts that a burst within the longer window is spread by the short-window rate, and that the longer-window cap still binds once the burst is spread. |
| AC13 | Every class bounded by default has its default traceable to a figure the design document cites and has verified; every class whose figure the design could not verify is unbounded by default. A test asserts that an unbounded-by-default class imposes no delay under the default configuration, and that configuring a bound for it makes it bind. |
| AC14 | The per-chat limiter applies to a private chat on the same terms as to a group chat — no chat is exempt by construction. A test asserts the guard is charged for a private chat. |
| AC15 | No limiter behaviour is conditioned on the base URL: no Go file in the change branches on the configured base URL value, and a test asserts identical limiter behaviour for two different base URLs. |
| AC16 | For every outbound call the client invokes an observation point exactly once, carrying the method name, the call's latency, the response code, whether the response was a 429, and the number of retries the call consumed. A test collects those observations for a success, a retried success and a give-up. |
| AC17 | No non-test Go file in `internal/tg` imports a metrics-registry package, and the package compiles and its tests pass with no observation point installed. |
| AC18 | A call whose context is cancelled or whose deadline passes while it is waiting — in a backoff, in a `retry_after` wait, or behind either limiter — returns a context error promptly rather than after the full wait. A test covers all three waiting sites. |
| AC19 | The fake Bot API server used by this package's tests can produce, on demand: a success response, a 429 with `retry_after`, a 5xx, a transport-level failure with no HTTP response, and a delayed response. Every retry, limiter and cancellation criterion above is exercised against it, with no network egress and no running Telegram instance. |
| AC20 | The client sends its requests to the configured base URL: a test constructs the client with the fake server's URL taken from the same configuration field production uses for the self-hosted instance, proving the three-value axis is one configuration value and not a branch in code. |
| AC21 | Every retry and rate-limit value is a `LAB_GAME_`-prefixed environment variable that `internal/config` reads and exposes as a typed field. No literal retry or rate-limit value appears at a call site in `internal/tg`, and a test constructs the client with non-default values and observes the changed behaviour. |
| AC22 | Each new key is optional: loading with all of them absent succeeds and yields the documented default for each. Loading with one present and well-formed yields that value. Loading with one present and malformed fails with an error naming that variable. |
| AC23 | The three-way key-set equality the configuration layer already asserts — `.env.example`, the loader's consulted keys, and the exported key list — holds with the new keys included, and `.env.example` documents each new key with its default as a non-empty value. |
| AC24 | The six previously declared variables remain required with no compiled-in default: loading with any one of them absent still fails, naming it. |
| AC25 | No live claim in the tree still asserts that every configuration variable is required or that none has a compiled-in default. Surfaces under `ai-docs/plans/done/` are history and are excluded from this criterion, not counterexamples to it. |
| AC26 | The bot token appears in no error message, no instrumentation observation, and no test fixture; no tracked file added by this change contains a real token, DSN, `api_id` or `api_hash`. |
| AC27 | The client's per-call seam is reachable by a caller outside the package for the purpose #22 needs (an outbound gate that a handler cannot route around), and no exported API of `internal/tg` lets a caller issue an outbound call that bypasses it. |
| AC28 | Every gate `make verify` runs is green on the resulting tree, including the race-enabled test gate. |
| AC29 | Propagation is complete for this change: every site whose claim the diff falsifies is updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4. Sites known at spec time — illustrative, not exhaustive: the requiredness sentence in `internal/config/env.go`, `.env.example`, `ai-docs/context.md` § Status (its "no Telegram client" claim), `ai-docs/plans/INDEX.md`, and `ai-docs/key-decisions.md` KD-2. |
| AC30 | Emission from one key is steady rather than bursty: with a bucket configured at a known interval, a test releasing many calls at once against one key asserts the calls leave one interval apart, in the order they arrived, rather than all at once. |
| AC31 | No call is ever discarded to relieve pressure. A call that cannot be emitted before its context deadline returns the typed error of AC9 and no other call is displaced; a test asserts that under saturation every submitted call ends in either an emission or a returned error, never in silence. |
| AC32 | The shaping state is in-process only: no Go file in the change writes limiter or queue state to the database, to disk, or to any store that outlives the process, and no migration is added by this task. |
| AC33 | A call is retried at most the configured number of times: a test with the attempt cap set to a known value against a permanently failing retry-eligible response asserts exactly that many attempts, and asserts that a `retry_after` pause costs one attempt rather than exhausting the budget by itself. |

## Open questions

- **Whether `docs/DESIGN.md` §11 should gain the per-chat per-second figure.** The
  round-3 investigation reports an official per-chat send figure on a shorter window than
  the ~20/minute one §11 names. If that holds, §11 is incomplete rather than wrong — but
  `docs/DESIGN.md` is decisions, and this task does not edit it (`AGENTS.md` § Project).
  The owner may want to fold the figure in; the transport is built to express it either
  way, so nothing here blocks on the answer.
- **The leaky-bucket reading, flagged so it costs one sentence to correct.** "Leaky
  bucket" names two mechanisms in the cited article, and one of them the article calls
  exactly equivalent to the token bucket offered as the competing option. This spec
  reads the answer as the shaping/queue version — steady emission, no burst — because
  that is the version distinct from the option the owner did not pick, and because it
  is the stricter of the two against a flood ban. If the meter version was meant, the
  consequence is contained: the per-key mechanism becomes burst-tolerant within the
  same windows, and AC30 is the only criterion that changes.
- **A 5xx is a response, so it proves the request arrived.** The retry rule adopted in
  round 1 is "never retry on doubt", and 429 is safe by definition — Telegram states it
  did not act. A 5xx from the self-hosted instance or a proxy in front of it is *usually*
  un-executed but not provably so, which makes retrying it a narrow exception to the rule
  it sits beside. The spec follows the owner's answer literally (5xx is retried) and flags
  the residue: if duplicate messages are ever observed in practice, this is the row to
  revisit first.
- **Whether the edit class should be bounded once real traffic exists.** It is unbounded
  by default because no citable figure supports a bound. The transport will reveal the
  truth cheaply: the 429 counter broken down by method (#23) shows whether Telegram is
  actually throttling edits, and an operator can bound the class from the environment the
  same day, with no code change and no redeploy of a new binary.
- **Whether the optional-with-default key class should generalise.** This task narrows one
  over-broad sentence for its own tuning keys. Whether the configuration layer should
  offer defaults as a standing facility, and with what rule for deciding which keys get
  one, is a question for whoever next wants it — not something to settle from inside a
  transport task.
- **Whether `getMe` canaries should bypass the limiters.** #23 asks the same thing from
  the other side ("canary interval and its cost against the transport rate limiters").
  Under the decision above, `getMe` delivers no message and so charges no bucket by
  default; the question only returns if an operator bounds that class.
- **Whether the per-chat limiter needs eviction for chats that go quiet.** With one
  friendly chat in the MVP (§14) the bucket map cannot grow to matter; at fan-out scale
  it could, and keying on (chat, class) multiplies the entries. The design may either
  handle it now or record it as a known bound.
- **Whether media-carrying methods need their own treatment.** Multipart uploads have
  different timeouts and different retry economics from a `sendMessage`. The MVP sends
  no media, so nothing forces the question; a later media mechanic reopens it.
