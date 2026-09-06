# Update ingestion: long polling, dispatch, operation idempotency, chat allowlist

**Source:** issue #22
**Date:** 2026-09-06
**Tracked in:** #22

> **Round 1 draft.** Three decisions are open and marked **PENDING Q1 / Q2 / Q3** below; the
> sections that depend on them are written up to the fork and no further.

This is the bot's front door: the long poll against the configured Bot API base URL, the router
from an update to a handler, the idempotency of a repeated update, and the allowlist that keeps
the bot from writing to a chat it was never meant to reach. Four properties of it are already
decided and are not re-opened here.

**The bot is a stateless update handler over Postgres; restarts and deploys are trivial**
[source: 850d97f:docs/DESIGN.md:309 · `sed -n '309p' docs/DESIGN.md`]. Every piece of state this
loop needs — including its own polling offset — is a database row, not a process variable.

**Idempotency runs through the basis document, and no separate mechanism exists.** §11 fixes the
unique constraint on (source, `operation_id`) of `player_operation`, and states that a repeated
update creates no document and therefore no postings — one constraint protects the whole cascade
[source: 850d97f:docs/DESIGN.md:327 · `sed -n '327p' docs/DESIGN.md`]. The table, the enum
`operation_source` with its single member `telegram`, and the unique constraint are already shipped
[source: 850d97f:internal/store/migrations/00001_ledger_core.sql:4,53-59 ·
`sed -n '4p;53,59p' internal/store/migrations/00001_ledger_core.sql`], and `store.Post` already
raises `ErrAlreadyPosted` when a `PlayerOperation` basis replays a pair
[source: 850d97f:internal/store/basis.go:43-61 · `sed -n '43,61p' internal/store/basis.go`]. A
second, hand-rolled "seen updates" table is therefore **refused by the design**, not merely
unnecessary.

**telego's helper layer is never used.** §11 takes telego for its low-level, generated one-to-one
API and states that retries, idempotency and rate limits are this project's own thin layer on top
[source: 850d97f:docs/DESIGN.md:302 · `sed -n '302p' docs/DESIGN.md`]. The loop is therefore a
`getUpdates` call this project writes, issued through the `*telego.Bot` that `tg.Client.API()`
returns, so that it passes the same retry loop, `retry_after` wait, rate limiter, outbound gate and
observation point as every other call
[source: 850d97f:internal/tg/client.go:162-168 · `sed -n '162,168p' internal/tg/client.go`].

**Update lag is the star of the health dashboard.** §13.2 names update lag (update date versus the
moment of handling) the best early indicator of a stall, and puts handler duration, handler errors
and panics, and idempotency hits ("duplicates are normal; a spike is a signal") on the same health
surface [source: 850d97f:docs/DESIGN.md:408,410 · `sed -n '408p;410p' docs/DESIGN.md`].

What already exists that this task builds on, all verified at 850d97f: `internal/tg` — the client,
the `Gate` seam whose doc comment names this very issue as the installer of the `ALLOWED_CHAT_IDS`
allowlist [source: 850d97f:internal/tg/gate.go:66-79 · `sed -n '66,79p' internal/tg/gate.go`], and
the per-call `Observation`; `internal/config` — `Config.AllowedChatIDs`, the ordered set of chat ids
the bot may write to [source: 850d97f:internal/config/config.go:54-57 ·
`sed -n '54,57p' internal/config/config.go`]; `internal/store` — the ledger write path, the
`PlayerOperation` basis and `AppendEvent`; `internal/scheduler` — the closest structural precedent
for a Postgres-backed loop with a handler registry, a per-item transaction and an observation seam;
`internal/tgtest` — an in-process fake Bot API server whose handler is an arbitrary
`func(http.ResponseWriter, *http.Request)`, so scripted `getUpdates` responses need no new test
infrastructure.

## Scope

1. **A new package for the loop**, named here so the acceptance criteria can refer to it:
   `internal/ingest`, giving `ingest.Loop`, `ingest.Handler` and `ingest.Router`. No stutter, `ctx
   context.Context` first on every method that reaches the network or the database, no context
   stored in a struct, and interfaces declared by the consumer (`AGENTS.md` § API Naming). The
   scheduler's spec settled its package name the same way; a design amendment may rename this one,
   but the spec does not leave it open.

2. **The poll itself goes through `tg.Client`.** The loop calls `getUpdates` on the `*telego.Bot`
   from `Client.API()` and constructs no HTTP client, no second base-URL axis and no second
   ingestion path. The base URL is already one configuration value with three settings — the
   self-hosted instance, the cloud API, and a fake server in tests
   [source: 850d97f:docs/DESIGN.md:304 · `sed -n '304p' docs/DESIGN.md`].

3. **The set of update types requested is derived from the registered routes**, not written out by
   hand: a kind with a route is always requested, and a kind with no route can never arrive. The
   parameter is always transmitted explicitly — see *Technical constraints* for the two traps
   (`omitempty` erases an empty list, and an absent parameter silently inherits the server's
   previous setting, which is hidden state outside this repository).

4. **The offset is a database row.** §12.5 requires the testing-environment restore script to reset
   the updates offset inside its one sanitisation transaction
   [source: 850d97f:docs/DESIGN.md:393 · `sed -n '393p' docs/DESIGN.md`]; a value reachable by a SQL
   transaction over a restored snapshot is a row, not a process variable, and §11's stateless-bot
   sentence points the same way. This task therefore carries a **forward migration** (goose, under
   `internal/store/migrations`, no down section) for the offset's home, and the loop reads it at
   start-up and advances it as it polls. When exactly it advances is **PENDING Q2**.

5. **Dispatch: a router and a handler contract.** The router maps an update's shape to at most one
   handler. The handler contract takes `ctx` first, the loop's own `pgx.Tx`, and the update; the
   loop owns the transaction and neither the router nor a handler commits or rolls back — the same
   division `store.Post` already uses, where the caller owns `tx`
   [source: 850d97f:internal/store/post.go:44-46 · `sed -n '44,46p' internal/store/post.go`], and
   the same one `internal/scheduler` uses for task handlers. An update with no registered route is
   not an error: it is counted and skipped.

6. **Idempotency: `operation_id` derivation and where the duplicate is caught.** The derivation is
   settled (see *Key decisions*, `operation_id` grammar). **Where the check happens — the loop,
   before dispatch, or the handler, at its `store.Post` — is PENDING Q1**, and it decides whether
   `internal/store` gains a way to post under a basis document that already exists.

7. **Failure policy for a handler that returns an error — PENDING Q2.** The answer decides whether
   the offset advances past a failed update, and therefore whether a failed update is replayed at
   all.

8. **The `ALLOWED_CHAT_IDS` gate, in the one place a handler cannot bypass.** The structural half is
   already settled by `internal/tg`: `Gate.AllowCall` is consulted before every attempt and before
   any limiter wait, `ChatNone` (a call addressing no chat, such as `getUpdates` itself) must be
   allowed and `ChatUnknown` (a destination this project could not read) must be refused
   [source: 850d97f:internal/tg/gate.go:66-79 · `sed -n '66,79p' internal/tg/gate.go`]. This task
   ships the implementation of that interface, backed by `Config.AllowedChatIDs`, and it is
   un-bypassable because `Options.Gate` is set once at `tg.New` and every outbound call in the
   project runs through that client. **The policy the gate applies to a chat id outside the list is
   PENDING Q3.**

9. **Panic recovery per update.** A handler panic is recovered by the loop, recorded, and does not
   reach the process; the update's transaction is rolled back. `AGENTS.md` § Go Test Conventions
   forbids `panic` in production code, and a recovered panic in a dispatch loop needs no
   `ai-docs/panic-index.md` row because it raises none.

10. **The observation seam, and only the seam.** The loop declares its own `Observer` interface and
    reports update lag, handler duration, handler outcome (including error and recovered panic),
    idempotency hits and poll-cycle duration through it. Exposition — `/metrics`, Prometheus, the
    canaries — is #23's, exactly as `internal/tg` and `internal/scheduler` already ship an
    observation seam and no exporter.

11. **Telemetry declaration.** This task adds **no event type** to the §13.4 dictionary; its
    obligations are metrics only. See *Key decisions* for the issue-body wording that reads
    otherwise and why it resolves this way.

## Out of scope

- **Any specific handler.** The mechanics own them: chat location and deep-link onboarding (#30),
  the leader screen (#37), the DM surface (#42). This task ships the router with no production
  route registered, exactly as `internal/scheduler` shipped with no production task type.
- **Webhooks.** Long polling is the design's ingestion choice; no second ingestion path is built.
- **`/metrics` exposition, the Prometheus registry, and the canaries** — #23.
- **The outbound notification queue** (§13.2's queue depth and send lag) — not this issue's surface.
- **Wiring `cmd/bot` into a running process.** `cmd/bot` is a scaffold whose package comment already
  says the update loop lands with its own spec
  [source: 850d97f:cmd/bot/main.go:1-5 · `sed -n '1,5p' cmd/bot/main.go`], and #19 deliberately left
  it untouched rather than construct a client nothing feeds. With every handler out of scope, a
  wired process would poll and dispatch to an empty router. The composition root — pool, migrations,
  client, scheduler, loop, signal handling — belongs with the first mechanic that gives it something
  to do. See *Open questions* if the owner wants it here instead.

## Deferred

- Concurrent dispatch (a worker pool, per-chat ordering) | The MVP runs one friendly chat, and the
  §13.2 update-lag metric this task ships is the evidence that would justify changing it; a
  sequential loop keeps per-chat ordering free | yes, if measured lag makes dispatch the dominant term
- A dead-letter surface for updates that failed every attempt | Depends on Q2's answer; the
  scheduler's terminal give-up state is the precedent if one is wanted | yes, if Q2 makes updates droppable
- Reading `update_id` gaps as a signal (a gap means an update was confirmed but never handled) | A
  diagnosis tool, not part of the ingestion contract | no

## Key decisions

| Question | Decision |
|---|---|
| `operation_id` grammar — the issue's first open question | **`<space>:<id>`**, where `<space>` is a short stable token naming the **Telegram id space** the identifier was read from (not the update's kind, not the handler), `:` is the separator and never occurs inside `<space>`, and `<id>` is that space's identifier verbatim as Telegram sent it. Two spaces exist at the start: the `update_id` sequence and the `callback_query.id` sequence. The rule that prevents a retroactive collision is that **no `operation_id` is ever written without a prefix**, so the unprefixed string space stays empty forever and a third id space costs a new token rather than a migration of old rows. `operation_id` is `text` and opaque to `internal/store`, which is what makes the grammar this package's to define [source: 850d97f:internal/store/basis.go:28-34 · `sed -n '28,34p' internal/store/basis.go`]. Which spaces are actually used depends on Q1: a loop-level check needs only the `update_id` space. |
| The `source` enum member | `telegram`, the enum's only member today [source: 850d97f:internal/store/migrations/00001_ledger_core.sql:4 · `sed -n '4p' internal/store/migrations/00001_ledger_core.sql`]. No new member; a persisted enum value is a data contract that changes by forward migration only (`AGENTS.md` § API Stability carve-out). |
| Does a handler panic also write an event? | **No — metrics only.** The issue's scope line reads "Panic recovery per update, with the metric and the event", while its own Telemetry section reads "Events: none directly". §13.2 places handler errors and panics on the health/metrics stack [source: 850d97f:docs/DESIGN.md:410 · `sed -n '410p' docs/DESIGN.md`], and the shipped §13.4 dictionary has no member for a panic or a handler error [source: 850d97f:internal/store/migrations/00003_event_log.sql:20-36 · `sed -n '20,36p' internal/store/migrations/00003_event_log.sql`]. Adding one would be redesigning §13.4. Recorded rather than silently resolved. |
| Update lag for an update kind that carries no date | Report lag **only** where the update itself carries a date, and make its absence explicit in the observation rather than reporting zero — a zero lag is indistinguishable from a healthy poll and would poison §13.2's star metric. `callback_query` carries no date field of its own [source: telego@v1.11.2/types.go:3729-3754 · `sed -n '3729,3754p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go`]; the date on its originating message is the bot's own send time, not the player's click. |
| Long-poll timeout, poll interval, and batch limit | Operational tuning, so they join the existing optional-with-default `LAB_GAME_*` class (KD-27's boundary: tuning keys only — secrets, base URL, allowlist and file paths stay required). Values are the design's to pick, bounded by the `AttemptTimeout` constraint below. |
| Who owns the transaction | The loop. One transaction per update, handed to the handler, which neither commits nor rolls back — `store.Post`'s division, and `internal/scheduler`'s. |
| Observation vs exposition | The loop ships an `Observer` interface and no exporter, matching `internal/tg` and `internal/scheduler`; #23 reads all three. |
| A separate "processed updates" table | Refused by §11: "Отдельного механизма не нужно" [source: 850d97f:docs/DESIGN.md:327 · `sed -n '327p' docs/DESIGN.md`]. Idempotency is the basis document's unique constraint or it is nothing. |
| Idempotency check placement | **PENDING Q1.** |
| Offset advance on handler failure | **PENDING Q2.** |
| Gate policy for a chat outside the list | **PENDING Q3.** |

## Technical constraints

1. **`Transport.AttemptTimeout` bounds every attempt, including the long poll.** It defaults to 30s
   [source: 850d97f:internal/config/transport.go:123-128 · `sed -n '123,128p' internal/config/transport.go`]
   and the caller applies it on top of the caller's context on every attempt
   [source: 850d97f:internal/tg/caller.go:170-176 · `sed -n '170,176p' internal/tg/caller.go`]. A
   long-poll timeout at or above it turns every healthy poll into a cancelled attempt, a retry and a
   transport error. The relationship between the two values is the design's to fix and to state.

2. **The `allowed_updates` parameter has two failure modes, both silent.** The Bot API treats an
   absent parameter as "use the previous setting" — server-side state this repository cannot see —
   and an empty list as "every default type"
   [source: telego@v1.11.2/methods.go:28-35 · `sed -n '28,35p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go`].
   The field carries `json:"allowed_updates,omitempty"` and this project marshals request parameters with
   `encoding/json` [source: 850d97f:internal/tg/constructor.go:22-27 · `sed -n '22,27p' internal/tg/constructor.go`],
   so an empty or nil slice is **erased from the request body** and lands on the first failure mode.
   The design states what the loop transmits when no route is registered.

3. **At most one `journal_entry` may reference one `player_operation`.** The partial unique index
   `journal_entry_player_operation_key` makes a second entry under the same document impossible
   [source: 850d97f:internal/store/migrations/00001_ledger_core.sql:67-76 · `sed -n '67,76p' internal/store/migrations/00001_ledger_core.sql`],
   so one update maps to at most one posting batch under its own operation document. A handler
   needing two batches needs two documents, and therefore two operation ids from distinct id spaces
   or a grammar extension — which is the collision the `operation_id` grammar above exists to keep
   representable.

4. **Only one poller may run against one bot token.** `getUpdates` confirms updates by offset — an
   update is confirmed as soon as `getUpdates` is called with a higher offset
   [source: telego@v1.11.2/methods.go:12-18 · `sed -n '12,18p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go`]
   — so two concurrent pollers would confirm each other's updates and lose them. Whatever the design
   does about deploys and restarts, a second concurrent poll within one process is unrepresentable.

5. **`telego.Update` carries a `context.Context` inside it** — `Update.Context()` and
   `Update.WithContext()`
   [source: telego@v1.11.2/types.go:165-181 · `sed -n '165,181p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go`].
   The handler's context is the one passed
   as the first argument, never the one riding in the update — `AGENTS.md` § API Naming forbids a
   context stored in a struct, and this project's rule is not relaxed by a dependency's shape.

6. **Migrations are goose files under `internal/store/migrations`, applied through `store.Migrate`,
   forward-only, with no down section.** The offset's table joins them under that discipline.

7. **The ledger AXIOM is not relaxed for handlers.** Any balance a handler moves moves through
   `store.Post` inside the loop's transaction. This task moves no balance of its own.

8. **Propagation.** This change alters the layout of `internal/` and the standing of `cmd/bot`, and
   it closes two items recorded in `ai-docs/deferred/_inbox.jsonl`. Every site whose claim this diff
   falsifies must be updated in the same PR, per `AGENTS.md` § Propagation Rule step 4 — the known
   members of that class are `ai-docs/context.md`'s layout paragraph, `ai-docs/context-status.md`,
   `ai-docs/plans/INDEX.md` and `ai-docs/key-decisions.md`; the class is not bounded by that list.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | A package exists under `internal/` owning the update loop, exporting a loop type, a handler contract and a router; every exported item carries a doc comment starting with its name, and the package carries a package comment. |
| AC2 | No type in that package has a `context.Context` field, and every exported method that reaches the network or the database takes `ctx context.Context` as its first parameter. |
| AC3 | The package issues its `getUpdates` calls through the `*telego.Bot` obtained from `tg.Client.API()`; it constructs no `http.Client`, no `telego.Bot` of its own, and reads no base URL other than the one the client was built with. |
| AC4 | The set of update types requested from the Bot API is computed from the routes registered on the router: for any registered route set, every registered kind is present in the requested set. |
| AC5 | The request body of every `getUpdates` the package issues contains an `allowed_updates` field — including the case where no route is registered. |
| AC6 | The polling offset is read from and written to the database; no in-process value survives a restart as the sole record of it, and a fresh process resumes from the persisted value. |
| AC7 | A forward migration under `internal/store/migrations` creates the offset's storage; the migration file contains no `-- +goose Down` section, and the shipped migration files' contents are otherwise unchanged. |
| AC8 | A handler receives the loop's transaction and the update; the loop, not the handler, commits or rolls back — no exported entry point of the package lets a handler obtain a transaction the loop does not own. |
| AC9 | An update matching no registered route leaves the database unchanged, is reported to the observer as unrouted, and does not stop the loop. |
| AC10 | A handler that panics does not terminate the process: the loop recovers, the update's transaction leaves no row behind, the panic is reported to the observer distinctly from a returned error, and the loop continues polling. |
| AC11 | No `panic(`, `log.Fatal` or `os.Exit` call appears in the package's non-test source. |
| AC12 | Replaying an update already handled produces no new `player_operation` row, no new `journal_entry`, no new `posting`, and is reported to the observer as an idempotency hit. |
| AC13 | Every `operation_id` the package derives matches the namespace grammar in *Key decisions*: a non-empty space token, the separator, and a non-empty identifier; no derived value omits the prefix. |
| AC14 | Two updates from different Telegram id spaces that carry the same raw identifier derive different `operation_id` values. |
| AC15 | The package supplies a `tg.Gate` implementation backed by `config.Config.AllowedChatIDs` that allows a `ChatNone` call and refuses a `ChatUnknown` call. |
| AC16 | An outbound call to a chat the gate refuses is stopped before any HTTP attempt and before any rate-limiter wait — the refusal consumes no limiter allowance. |
| AC17 | The observer receives, per update, the handler's outcome and duration, and per poll cycle, the cycle's duration and the number of updates returned. |
| AC18 | Update lag is reported only for updates carrying their own date; for an update kind with no date the observation states the absence rather than reporting a zero duration. |
| AC19 | The package declares an observer interface and imports no metrics library; a nil observer is checked rather than called, so the package works with no observer installed. |
| AC20 | The event-type dictionary in `internal/store/migrations` and its Go mirror are unchanged by this task. |
| AC21 | A long poll in flight when the loop's context is cancelled returns promptly, the loop stops without leaking a goroutine, and no update is confirmed that was not handled under the policy Q2 settles. |
| AC22 | Every behaviour AC1-AC21 names is reachable without a live Telegram server and without a hand-provisioned database: the package's Bot API dependency is satisfiable by `internal/tgtest` and its storage dependency by `internal/testdb`, with no production path requiring either to be replaced by a mock of this package's own making. |
| AC23 | `make verify` passes in full — every gate that target chains, none excepted. |
| AC24 | Statement coverage does not fall past the recorded ratchet's tolerance. |
| AC25 | Every live document whose claim this diff falsifies is updated in the same PR, per `AGENTS.md` § Propagation Rule step 4 — the class defined in *Technical constraints* item 8, not only its named members. |
| AC26 | *(pending Q1)* The idempotency check's placement is realised as answered, and a duplicate is stopped at that point with no side effect past it. |
| AC27 | *(pending Q2)* A handler error moves the offset exactly as answered, and the observable consequence for a repeated failure is the one the answer implies. |
| AC28 | *(pending Q3)* The gate's verdict on a chat id outside `AllowedChatIDs` is the one answered, for both a private and a group destination. |

## Open questions

- **Does `cmd/bot` become a running process in this task?** The spec says no, on #19's precedent and
  because every handler is out of scope. If the owner wants a bot that polls before any mechanic
  exists — for the §13.2 canaries and the lag metric to have a live process — that is a scope
  change, not a design amendment.
- **Where the loop's poll interval and long-poll timeout land relative to `AttemptTimeout`.** The
  design fixes the numbers; the constraint that they interact is recorded above.
- **Whether the offset row is per-bot or global.** One bot per database is the MVP shape; a design
  that leaves room for a token change costs nothing and is the design's call.
