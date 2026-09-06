# Update ingestion: long polling, dispatch, operation idempotency, chat allowlist

**Source:** issue #22
**Date:** 2026-09-06
**Tracked in:** #22

> **Complete.** Every question this spec opened is answered. The allowlist policy was re-opened by
> the owner and re-answered at round 3. The earlier "every private chat is allowed" reading is
> superseded: where it appears below it is named as superseded and as refused, never applied.

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
for a Postgres-backed loop with a handler registry, a per-item transaction, a bounded retry with a
terminal give-up state, and an observation seam; `internal/tgtest` — an in-process fake Bot API
server whose handler is an arbitrary `func(http.ResponseWriter, *http.Request)`, so scripted
`getUpdates` responses need no new test infrastructure.

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
   start-up and advances it as it polls.

   **The offset never advances past an update that is not settled.** An update is *settled* when it
   was handled, refused as a duplicate, matched no route, or was given up on after the retry cap of
   Scope 7. This single invariant is what makes the crash window bounded and stated rather than
   accidental.

5. **Dispatch: a router and a handler contract.** The router maps an update's shape to at most one
   handler. The handler contract takes `ctx` first, the loop's own `pgx.Tx`, and the update; the
   loop owns the transaction and neither the router nor a handler commits or rolls back — the same
   division `store.Post` already uses, where the caller owns `tx`
   [source: 850d97f:internal/store/post.go:44-46 · `sed -n '44,46p' internal/store/post.go`], and
   the same one `internal/scheduler` uses for task handlers. An update with no registered route is
   not an error: it is counted and settled.

6. **Idempotency: the handler owns the basis document** (owner, round 1). The loop derives the
   update's operation key and hands it to the handler; the handler builds
   `store.PlayerOperation{Source: store.SourceTelegram, OperationID: <key>}` and passes it to
   `store.Post`. **`internal/store`'s posting path is untouched by this task** — no
   post-under-an-existing-document API is added, and the existing phase-d refusal is the whole
   mechanism: on a replay `Post` returns
   `ErrAlreadyPosted`, the transaction is usable, and nothing was written
   [source: 118e012:internal/store/errors.go:32-35 · `sed -n '32,35p' internal/store/errors.go`].

   Three consequences follow, and all three are requirements, not commentary:

   - **The loop classifies the outcome by sentinel.** §13.2 wants an idempotency-hit counter, and
     under this placement only the handler's return can carry that fact. A handler error that
     `errors.Is` matches `store.ErrAlreadyPosted` is a **duplicate**: settled, counted as an
     idempotency hit, offset advanced, and — decisively — **never retried**. Any other error is a
     failure and takes Scope 7's path. The contract therefore requires a handler to wrap with `%w`
     rather than reformat the sentinel away.
   - **An update that moves no balance writes no document and is replayed in full.** The owner
     accepted this. Re-delivery happens only when the process stops between handling an update and
     advancing the offset past it, so the exposure is the crash-and-restart window, not normal
     operation. §11 makes the player's screen an *edited* message rather than a stream of new ones
     [source: 118e012:docs/DESIGN.md:305 · `sed -n '305p' docs/DESIGN.md`], so a replayed edit
     tends to converge rather than accumulate — but that is the mechanic's property to preserve,
     never a guarantee this loop offers.
   - **The derivation is the loop's, the use is the handler's.** The loop hands over a key it built;
     a handler never assembles an `operation_id` from raw Telegram fields itself, which is what
     keeps the namespace grammar in one place and makes an unprefixed value unreachable.

7. **Failure policy: bounded retry, then advance** (owner, round 1), mirroring the scheduler's
   one-shot policy and its terminal give-up state.

   - A handler error is retried **in a fresh transaction**: the failed attempt's transaction is
     rolled back before the next attempt begins, so no partial effect from a failed attempt can
     survive into a later one.
   - The delay between attempts grows, up to a configured cap on attempts. After the cap the loop
     **advances past the update** and records a give-up row (Scope 12).
   - `ErrAlreadyPosted` is not a failure: it consumes no attempt and never reaches this path.
   - The attempt cap and the delay bounds are operational tuning and join the optional-with-default
     `LAB_GAME_*` class, mirroring the six `LAB_GAME_SCHEDULER_*` keys already shipped
     [source: 118e012:internal/config/scheduler.go:15-20 ·
     `sed -n '15,20p' internal/config/scheduler.go`].
   - The retry stalls the sequential loop for the sum of its delays. That head-of-line cost is the
     accepted price of not dropping on first error, and the cap is what bounds it.
   - **Cancellation during a retry settles nothing.** If the loop's context is cancelled mid-retry,
     the update is left unsettled and the offset stays behind it, so Telegram re-delivers it after
     the restart. A shutdown is never a silent drop.

8. **The `ALLOWED_CHAT_IDS` gate, in the one place a handler cannot bypass.** The structural half is
   already settled by `internal/tg`: `Gate.AllowCall` is consulted before every attempt and before
   any limiter wait, `ChatNone` (a call addressing no chat, such as `getUpdates` itself) must be
   allowed and `ChatUnknown` (a destination this project could not read) must be refused
   [source: 850d97f:internal/tg/gate.go:66-79 · `sed -n '66,79p' internal/tg/gate.go`]. This task
   ships the implementation of that interface, backed by `Config.AllowedChatIDs`, and it is
   un-bypassable because `Options.Gate` is set once at `tg.New` and every outbound call in the
   project runs through that client.

   **The policy** (owner, round 3, replacing the round-1 answer): the list governs group and channel
   chats, and a chat id outside the list is allowed **only when it resolves to a player the database
   already knows**. The gate reads the database on the outbound path, cached in process. §1 puts
   every game command in DM [source: 118e012:docs/DESIGN.md:33-34 · `sed -n '33,34p' docs/DESIGN.md`]
   and §1's onboarding has the bot DM a player whose id nobody could have listed in advance
   [source: 0376234:docs/DESIGN.md:41 · `sed -n '41p' docs/DESIGN.md`], which is why a list-only gate
   was rejected; asking *who* an id belongs to is answerable from shipped schema, whereas asking
   *what kind of chat* it is is not (*Technical constraints* item 9).

   Four properties of that lookup are requirements, not implementation latitude:

   - **The predicate names the kind.** A known player is an `owner` row with `kind = 'player'` **and**
     `telegram_id` equal to the destination id. The uniqueness the schema provides is on the *pair*:
     `owner_kind_telegram_id_key` is `(kind, telegram_id)`, and a chat's own id lives in that same
     `telegram_id` column on a `kind = 'chat'` row
     [source: 0376234:internal/store/migrations/00001_ledger_core.sql:6-12 ·
     `sed -n '6,12p' internal/store/migrations/00001_ledger_core.sql`]. A lookup keyed on
     `telegram_id` alone therefore matches every chat the bot was ever added to and **voids the
     allowlist entirely** — the exact bypass this gate exists to prevent.
   - **The gate fails closed.** A lookup that errors refuses the call. A safety net that opens when
     the database hiccups is not a safety net, and the caller already treats a gate refusal as a
     first-class outcome with no attempt and no limiter spend.
   - **A negative result is not cached indefinitely.** The onboarding flow is: button in chat →
     player presses Start → the player's owner row is created → the bot DMs them. A gate that
     remembered "not a player" from before the Start would refuse that DM until the process
     restarted, breaking onboarding in a way no test of a single moment would show. A positive
     result may be held for the process's lifetime, since nothing in this task deletes an owner row
     — a later mechanic that does must invalidate the cache, and that obligation is recorded here.
   - **`internal/tg` stays free of the database.** `Gate` is consumer-declared, so the implementation
     lives outside that package and `internal/tg` gains no import of `internal/store`, no pool and
     no schema knowledge.

   Nothing here waits on #30. The `owner` table and `store.CreateOwner` are already shipped
   [source: 0376234:internal/store/owner.go:41 · `grep -n '^func CreateOwner' internal/store/owner.go`],
   so the gate is exercisable today by creating owner rows directly; until #30 lands there are simply
   no players, and the gate is correct and vacuous.

9. **Panic recovery per update.** A handler panic is recovered by the loop, recorded, and does not
   reach the process; the update's transaction is rolled back. `AGENTS.md` § Go Test Conventions
   forbids `panic` in production code, and a recovered panic in a dispatch loop needs no
   `ai-docs/panic-index.md` row because it raises none. A recovered panic is a **failure** for
   Scope 7's purposes, distinguished from a returned error in the observation.

10. **The observation seam, and only the seam.** The loop declares its own `Observer` interface and
    reports update lag, handler duration, handler outcome (handled, duplicate, unrouted, error,
    recovered panic, given up) and poll-cycle duration through it. Exposition — `/metrics`,
    Prometheus, the canaries — is #23's, exactly as `internal/tg` and `internal/scheduler` already
    ship an observation seam and no exporter.

11. **Telemetry declaration.** This task adds **no event type** to the §13.4 dictionary; its
    obligations are metrics only. See *Key decisions* for the issue-body wording that reads
    otherwise and why it resolves this way.

12. **The give-up record: a row, without the update's body** (owner, round 3). A dropped update
    leaves one row carrying its identity, its update kind, the destination chat id where the update
    had one, the attempt count and the last error — the shape `scheduler.DeadTask` already has
    [source: 0376234:internal/scheduler/task.go:78-90 · `sed -n '78,90p' internal/scheduler/task.go`]
    — and **not** the raw update payload. The table lands in this task's forward migration alongside
    the offset's,
    and the package exposes a read function over it in the shape `scheduler.DeadTasks` already uses:
    a caller-owned `pgx.Tx` and a limit, ordered deterministically
    [source: 0376234:internal/scheduler/schedule.go:60-67 · `sed -n '60,67p' internal/scheduler/schedule.go`].

    Two consequences are stated rather than discovered later. The chat id is a real Telegram id at
    rest, so §12.5's sanitisation transaction gains a table to rewrite (*Technical constraints*
    item 8). And because the body is not kept, **a given-up update cannot be replayed by hand once
    its handler is fixed** — the row diagnoses, it does not recover. The chat id column is nullable:
    not every update kind addresses a chat.

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
  sequential loop keeps per-chat ordering free, and Scope 7's retry stall is the term to watch |
  yes, if measured lag makes dispatch the dominant term
- A command-line or chat surface that *displays* give-up rows | The read function ships here
  (Scope 12); a human-facing view of it has no process to run in until the composition root exists |
  yes, with the composition root
- Reading `update_id` gaps as a signal (a gap means an update was confirmed but never handled) | A
  diagnosis tool, not part of the ingestion contract | no
- Idempotency for a handler that moves no balance | Follows from the round-1 placement decision:
  no document, therefore no constraint. Revisit only if a replayed non-ledger effect is observed
  to matter | no

## Key decisions

| Question | Decision |
|---|---|
| Where is a repeated update caught? | **At the handler's `store.Post`** (owner, round 1). The posting path is untouched — the separate read-only owner lookup the round-3 gate needs is not part of it; `ErrAlreadyPosted` is the whole mechanism; the loop classifies by sentinel and never retries a duplicate. Scope 6 states the three consequences. |
| Does a handler error re-poll the update or drop it? | **Bounded retry, then advance** (owner, round 1). Fresh transaction per attempt, growing delay, configured attempt cap, then a recorded give-up. The scheduler's one-shot policy, transposed. |
| What does the gate do with a chat id outside the list? | **Allow it when it resolves to a known player, refuse it otherwise** (owner, round 3, superseding the round-1 answer). The list governs group and channel traffic; a DM is allowed because of *who* the destination is, never because of *what kind* of chat it is. The round-1 reading — every private chat allowed — is superseded, and with it the need to classify a chat's kind at the outbound seam at all. |
| `operation_id` grammar — the issue's first open question | **`<space>:<id>`**, where `<space>` is a short stable token naming the **Telegram id space** the identifier was read from (not the update's kind, not the handler), `:` is the separator and never occurs inside `<space>`, and `<id>` is that space's identifier verbatim as Telegram sent it. Two spaces exist at the start: the `update_id` sequence and the `callback_query.id` sequence. The rule that prevents a retroactive collision is that **no `operation_id` is ever written without a prefix**, so the unprefixed string space stays empty forever and a third id space costs a new token rather than a migration of old rows. `operation_id` is `text` and opaque to `internal/store`, which is what makes the grammar this package's to define [source: 850d97f:internal/store/basis.go:28-34 · `sed -n '28,34p' internal/store/basis.go`]. This closes the item `ai-docs/deferred/_inbox.jsonl` recorded from the ledger-core spec. |
| Which id space does the loop hand a handler? | The **`update_id` space**, as the update's canonical key. The `callback_query.id` space has a reserved token and a derivation so that a handler needing the click's own identity has a prefixed value to use rather than an invented one; no route uses it yet, because no route exists yet. |
| The `source` enum member | `telegram`, the enum's only member today [source: 850d97f:internal/store/migrations/00001_ledger_core.sql:4 · `sed -n '4p' internal/store/migrations/00001_ledger_core.sql`]. No new member; a persisted enum value is a data contract that changes by forward migration only (`AGENTS.md` § API Stability carve-out). |
| Does a handler panic also write an event? | **No — metrics only.** The issue's scope line reads "Panic recovery per update, with the metric and the event", while its own Telemetry section reads "Events: none directly". §13.2 places handler errors and panics on the health/metrics stack [source: 850d97f:docs/DESIGN.md:410 · `sed -n '410p' docs/DESIGN.md`], and the shipped §13.4 dictionary has no member for a panic or a handler error [source: 850d97f:internal/store/migrations/00003_event_log.sql:20-36 · `sed -n '20,36p' internal/store/migrations/00003_event_log.sql`]. Adding one would be redesigning §13.4. Recorded rather than silently resolved. |
| Update lag for an update kind that carries no date | Report lag **only** where the update itself carries a date, and make its absence explicit in the observation rather than reporting zero — a zero lag is indistinguishable from a healthy poll and would poison §13.2's star metric. `callback_query` carries no date field of its own [source: telego@v1.11.2/types.go:3729-3754 · `sed -n '3729,3754p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go`]; the date on its originating message is the bot's own send time, not the player's click. |
| Long-poll timeout, poll interval, batch limit, retry cap and delays | Operational tuning, so they join the existing optional-with-default `LAB_GAME_*` class (KD-27's boundary: tuning keys only — secrets, base URL, allowlist and file paths stay required). Values are the design's to pick, bounded by the `AttemptTimeout` constraint below. |
| Who owns the transaction | The loop. One transaction per attempt, handed to the handler, which neither commits nor rolls back — `store.Post`'s division, and `internal/scheduler`'s. |
| Observation vs exposition | The loop ships an `Observer` interface and no exporter, matching `internal/tg` and `internal/scheduler`; #23 reads all three. |
| A separate "processed updates" table | Refused by §11: "Отдельного механизма не нужно" [source: 850d97f:docs/DESIGN.md:327 · `sed -n '327p' docs/DESIGN.md`]. Idempotency is the basis document's unique constraint or it is nothing. A give-up record (Scope 7) is not this: it records what was *dropped*, never what was *seen*, and no code path consults it to decide whether to act. |
| Does the gate classify a chat's kind? | **No, and the positive-id convention is refused outright.** The seam sees only a raw `chat_id` token, and no citable rule maps a token to a chat kind (*Technical constraints* item 9). The gate asks an identity question the shipped schema answers instead. Recorded so a later reader does not "simplify" the lookup into a sign check. |
| What happens when the player lookup errors? | **The call is refused — fail closed.** The direction is not a preference: `AGENTS.md` § Domain Rules makes writing to an unintended chat the failure this gate exists to prevent, and a net that opens under load is not one. |
| May the lookup's result be cached? | **A positive result, yes, for the process's lifetime; a negative result, not in a way that outlives a player's Start.** Onboarding creates the player row *after* the bot has had reason to look the id up, so an unbounded negative cache would refuse the very first DM until a restart. |
| Is `ALLOWED_CHAT_IDS` ever absent, making the gate a no-op? | **No.** It is required and non-empty in every environment including production; unset or empty is a start-up error naming the variable [source: 0376234:ai-docs/plans/done/2026-09-04-config-layer-balance-files.spec.md:121 · `sed -n '121p' ai-docs/plans/done/2026-09-04-config-layer-balance-files.spec.md`]. There is no disabled state to specify. |
| What a give-up record consists of | **A row without the payload** (owner, round 3): identity, update kind, chat id, attempts, last error — `scheduler.DeadTask`'s shape. Enumerable and diagnosable; one real chat id for §12.5 to sanitise; the update body is not persisted, so a give-up is not replayable. |

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
   The field carries `json:"allowed_updates,omitempty"` and this project marshals request parameters
   with `encoding/json` [source: 850d97f:internal/tg/constructor.go:22-27 · `sed -n '22,27p' internal/tg/constructor.go`],
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
   The handler's context is the one passed as the first argument, never the one riding in the
   update — `AGENTS.md` § API Naming forbids a context stored in a struct, and this project's rule
   is not relaxed by a dependency's shape.

6. **Migrations are goose files under `internal/store/migrations`, applied through `store.Migrate`,
   forward-only, with no down section.** The offset's table and the give-up table both join them
   under that discipline.

7. **The ledger AXIOM is not relaxed for handlers.** Any balance a handler moves moves through
   `store.Post` inside the loop's transaction. This task moves no balance of its own.

8. **Propagation.** This change alters the layout of `internal/` and the standing of `cmd/bot`, and
   it closes two items recorded in `ai-docs/deferred/_inbox.jsonl`. Every site whose claim this diff
   falsifies must be updated in the same PR, per `AGENTS.md` § Propagation Rule step 4 — the known
   members of that class are `ai-docs/context.md`'s layout paragraph, `ai-docs/context-status.md`,
   `ai-docs/plans/INDEX.md` and `ai-docs/key-decisions.md`; the class is not bounded by that list.
   §12.5's sanitisation obligation is a member of the same class and is now definite: the restore
   script rewrites real `chat_id`/`user_id` values in one transaction
   [source: 850d97f:docs/DESIGN.md:393 · `sed -n '393p' docs/DESIGN.md`], and the give-up table's
   chat id column is a new value for it to rewrite.

9. **The outbound seam cannot see a chat's kind — which is why the gate asks who, not what.** The
   gate receives `tg.ChatRef.Key`, the destination's raw JSON token as it appeared in the request
   body, and a `chat_id` may be a `@channelusername` string rather than a number
   [source: 850d97f:internal/tg/gate.go:43-52 · `sed -n '43,52p' internal/tg/gate.go`]. The Bot API's
   documented discriminator for a chat's kind is `Chat.Type` — "private", "group", "supergroup" or
   "channel" — and it exists only on **inbound** data; `Chat.ID`'s own documentation states solely
   that the value fits in 52 significant bits, and states no relation between a chat's kind and its
   id's sign or magnitude
   [source: telego@v1.11.2/types.go:290-298 · `sed -n '290,298p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go`].
   The familiar "a positive id is a private chat" rule is therefore **not citable from this
   repository or its dependencies**, and `AGENTS.md` § Dependency Versions forbids asserting external
   behaviour from memory. The round-3 policy sidesteps the question rather than answering it: a
   destination is allowed because the list names it or because the database knows whose it is. A
   token that parses as no integer is refused under that rule without any special case — it cannot
   be in `AllowedChatIDs`, which is `[]int64`, and it cannot equal an `owner.telegram_id`, which is
   `bigint`.

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
| AC21 | `internal/store`'s **posting path** is unchanged by this task: no new posting entry point, no new `PostingBasis` implementation, and no change to `Post`'s signature or to `ErrAlreadyPosted`'s meaning. A read-only owner lookup added for the gate is not a change to that path; `internal/store` exports no owner read today. |
| AC22 | A handler error whose chain `errors.Is`-matches `store.ErrAlreadyPosted` is settled as a duplicate: it is counted as an idempotency hit, it consumes no retry attempt, and the offset advances past its update. The classification holds when the handler wrapped the sentinel with `%w`. |
| AC23 | A handler error that is not a duplicate is retried, each attempt in a transaction of its own, with a strictly positive delay between attempts that does not shrink, up to a configured attempt cap. |
| AC24 | After a failed attempt the database holds nothing that attempt wrote; after the attempt cap is exhausted the loop advances past the update and continues polling. |
| AC25 | The offset never advances past an update that is not settled, where settled means handled, refused as a duplicate, unrouted, or given up on. Cancelling the loop's context mid-retry leaves the update unsettled and the offset behind it. |
| AC26 | A long poll in flight when the loop's context is cancelled returns promptly and the loop stops without leaking a goroutine. |
| AC27 | The attempt cap, the delay bounds, the poll interval, the long-poll timeout and the batch limit are each readable from an environment variable with a compiled-in default, and an invalid value is rejected at start-up naming the variable. |
| AC28 | The gate allows a `ChatKnown` destination whose id is in `AllowedChatIDs`; allows one outside the list when an `owner` row exists with `kind = 'player'` and that `telegram_id`; and refuses every other `ChatKnown` destination, including a `chat_id` token that parses as no integer. |
| AC29 | Every behaviour AC1-AC28 names is reachable without a live Telegram server and without a hand-provisioned database: the package's Bot API dependency is satisfiable by `internal/tgtest` and its storage dependency by `internal/testdb`, with no production path requiring either to be replaced by a mock of this package's own making. |
| AC30 | `make verify` passes in full — every gate that target chains, none excepted. |
| AC31 | Statement coverage does not fall past the recorded ratchet's tolerance. |
| AC32 | Every live document whose claim this diff falsifies is updated in the same PR, per `AGENTS.md` § Propagation Rule step 4 — the class defined in *Technical constraints* item 8, not only its named members. |
| AC33 | A destination id absent from `AllowedChatIDs` that matches an `owner` row of a kind other than `player` — a chat the bot was added to, whose id lives in the same `telegram_id` column — is refused. The allowlist is not bypassed by the bot's own knowledge of a chat. |
| AC34 | A player lookup that fails refuses the call: no outbound attempt is made when the gate cannot establish that the destination is permitted. |
| AC35 | A destination refused because no matching player row existed is allowed once that row exists, within the same process — no restart is required for a player who presses Start after the first refusal. |
| AC36 | A given-up update leaves exactly one row carrying its identity, its update kind, the destination chat id where the update had one, the attempt count and the last error; no raw update payload is persisted anywhere by this package. |
| AC37 | The package exposes a read over give-up rows taking a caller-owned transaction and a limit, returning them in a deterministic order. |
| AC38 | No production code path outside `internal/tg` can issue an outbound Bot API call that skips the gate: the gate is installed at client construction and the client is the only route to the generated method surface. |

## Open questions

- **Does `cmd/bot` become a running process in this task?** The spec says no, on #19's precedent and
  because every handler is out of scope. If the owner wants a bot that polls before any mechanic
  exists — for the §13.2 canaries and the lag metric to have a live process — that is a scope
  change, not a design amendment.
- **Where the loop's poll interval and long-poll timeout land relative to `AttemptTimeout`.** The
  design fixes the numbers; the constraint that they interact is recorded above.
- **Whether the offset row is per-bot or global.** One bot per database is the MVP shape; a design
  that leaves room for a token change costs nothing and is the design's call.
- **Recovery of a given-up update is out of reach by construction.** The round-3 answer keeps the
  identity and the error but not the body, so a dropped player action can be diagnosed and never
  replayed. Recorded so a later reader meets the trade deliberately rather than discovering it
  during an incident.
- **The cache's shape.** The two requirements are fixed (a positive result may persist for the
  process; a negative one may not outlive a player's Start); whether that is a TTL, an
  invalidate-on-write, or simply never caching a negative is the design's to pick and to defend.
