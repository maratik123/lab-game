# Domain invariants

> Extracted from `AGENTS.md` § *Domain Rules*. That file keeps the binding AXIOMs; this page carries the mechanics and the reasoning. Every claim here traces to a section of `~/lab-private/DESIGN.md` — when the two disagree, the design document wins and this page is the bug.

## 1. The ledger — every balance moves by posting

**Rule.** Stamina, resources, money and item capacity change **only** through postings written by `store.Post` or `store.Move`, under exactly one basis document, and the postings of one transaction sum to zero per kind. Emission and burning are postings against the global **World** account, which alone may go negative (`~/lab-private/DESIGN.md` §11).

`store.Move` is the second write path and composes the first: it takes the caller's own postings alongside the instance movements, derives the capacity legs from those movements, and hands the whole batch to the same body `store.Post` uses — so the zero-sum check, the capture order and the deadlock discipline described below govern a move exactly as they govern a plain posting group.

| If you are about to... | Do this instead |
|---|---|
| `UPDATE players SET stamina = stamina - 3` | Post `player → World` for kind `stamina`, under the basis document for the move |
| Grant a reward by inserting a row into an inventory table | Call `store.Move`: it writes the instance's movement and the capacity postings it implies under one document, in one transaction |
| "Fix" a wrong balance with an `UPDATE` | Append a compensating posting under a `manual correction` document — history is append-only |
| Write a posting with no basis | There is no such thing. A posting in mid-air is a modelling error, and `store.Post` refuses it |

**Why the zero-sum check lives in `store.Post`.** It is the compiler of the economy: the "forgot the second leg" class of bug becomes impossible at write time, and by induction the whole ledger sums to zero per kind forever. A daily close then re-verifies the chain (opening + turnover == closing); a mismatch is an alert, and an alert here always means a code defect, because every source is in the same database.

**Deadlock discipline is `store.Post`'s job, never the caller's** (§11): dedupe and sort the touched controlled accounts by `(account_id, kind)`, do **all** balance `UPDATE`s first in that order — as separate statements, never one batched `UPDATE` whose lock order follows an unowned plan — then insert the postings. A wait cycle is impossible by construction.

**Negative balances.** `CHECK (balance >= 0)` on controlled accounts (players); the World account is exempt. Stamina may become a designed exception (a debt/debuff that regen pays off) — that is a balance decision, still open. For resources and money, a negative balance is a duplication exploit, never a feature.

## 2. The item machine — identity is not a kind

Items with identity (durability, enchantment) are **never** modelled as ledger kinds. They live in `item` + append-only `item_movement`, and the invariant is **chain continuity**: each movement's `from` equals the previous movement's `to`, so an instance has exactly one holder at any moment (§11). Continuity is a database property rather than a code convention: a self-referential composite foreign key ties a movement's `(predecessor, instance, from)` to the predecessor's `(id, instance, to)`, a `NULLS NOT DISTINCT` unique index gives each movement at most one successor and each instance at most one genesis, and a `CHECK` pins a genesis's `from` to World.

Holders of the item machine and accounts of the quantitative machine share **one address space**, and the shared address is a `scope` row — player backpack, player chest, corpse 123, construction site, World are all addresses of that one kind, so a holder the MVP does not use costs a catalog row, never a schema change or a branch on holder kind. Death, looting, contribution and evaporation are the *same* "move under a document" operation with a different holder; there is no bespoke corpse code to write. A movement also posts the capacity kind (slots/weight) in the main ledger under the same document, against a `free`/`used` account pair held per holder per kind — which is what makes an over-stuffed backpack unrepresentable rather than validated: the destination's `free` leg is debited, and `CHECK (balance >= 0)` refuses the move.

Cross-machine reconciliation ships as two views, so drift is answerable by a query over the shipped data rather than by new code. `item_chain_break` reports any instance whose movements are not one unbroken path from a World genesis to exactly one head. `item_capacity_divergence` reports any holder whose instance count disagrees with its controlled `slots`/`used` balance, or that holds instances with no such account at all.

**Free consequence:** the movement chain is an item's provenance — "crafted by Вася, lost on node 12, picked up by the fishermen's chat" — which notifications and season stories can read without any extra code.

## 3. Basis documents — exclusive arc, not a polymorphic pair

Each basis-document type is its **own table** with its own schema and lifecycle (player operation — `player_operation`, raid-session transition, cron day-close, deferred one-shot, recurring task, manual correction, season close, game event — `event`, the product log of §13.1). A posting carries one nullable FK column per type plus `CHECK (num_nonnulls(...) = 1)`. A polymorphic `(doc_type, doc_id)` pair without referential integrity was considered and **rejected** (§11).

Adding a document type is a migration plus a `CHECK` edit — deliberately: a new kind of document passes an explicit review, and the `CHECK` doubles as a greppable registry of every type that exists.

**Idempotency rides on the same rail:** a unique constraint `(source, operation_id)` on the player-operation document (`player_operation`, `source` enum) means a replayed update creates no document, therefore no postings. There is no second idempotency mechanism to keep in sync.

## 4. The raid FSM and the scheduler

A session is a row: `(id, maze_id, position, state, leader, participants, arrived_at, screen_message_id, seq)`. One engine drives it — `transition(session_id, edge, expected_seq)` inside a transaction with a guard on `(state, seq)`; effects, then `state`/`seq` advance, then the new state's timer edges are scheduled (§3.5).

- **A player action and a timer are edges of the same kind.** Anything true of one is true of the other.
- **A stale task is expected traffic, not an error.** Timer edges carry `expected_seq`; when the guard misses, the task dies on execution. Cancellation is unnecessary and orphans are harmless — write tests for that path, do not "fix" it.
- **Button presses are idempotent through the same guard**: `seq` travels in `callback_data`, a stale press redraws instead of acting. This covers double clicks, Telegram retries, and presses on an old message.
- **A restart absorbs downtime, so a `run_at` is not a promise about wall-clock arrival.** The process's start-up runs one restart-hygiene step before the worker starts: when the gap between the persisted liveness instant and the database's own `now()` exceeds the configured threshold, every *pending* task already past its `run_at` is moved forward by that same gap, so an outage does not fire every due timer edge in one salvo. It writes `scheduled_task` and nothing else — no basis-document row, no posting, no journal entry — and the `Reconcile` that follows pulls a pushed recurrence back. A mechanic must therefore be correct about *ordering* and about its own guard, never about a task firing at the instant it was scheduled for; the mechanics are in [`ai-docs/process-lifecycle.md`](process-lifecycle.md).
- **A task executes in one transaction with its effects** — exactly-once without two-phase machinery. Task types are the ledger's basis-document types, not a foreign job model.
- **The scheduler is load-bearing and its lag is visible in gameplay** ("a wave in five minutes" arriving in seven). Lag by task type belongs on the health dashboard (§13.2).

## 5. Telemetry is part of the mechanic

> **A new mechanic declares its events in the same PR that implements it** (§13.4). If it moves balances, it also declares the **posting signature** of its basis document, and the contract test checks the actual postings against that signature.

The product dashboard reads the raw `event` log, never pre-aggregated counters — what you did not record, you cannot ask later. The ledger and the event log are **different tables** (strict schema vs JSONB, different retention, different readers); postings reference an event as one of their basis types.

### How a mechanic declares and checks its posting signature

The rule above stands as written; this is its mechanics. The vocabulary, the declared set and the check live in `internal/contract`, a package only tests import ([`key-decisions.md`](key-decisions.md) KD-42). A balance-moving mechanic does three things, in its own pull request.

1. **Add its type's row to the declared set** — the `declarations` literal in `internal/contract/declared.go`. The row pairs the type — `contract.Event(store.EventShopSale)`, `contract.DeferredTask("<code>")` or `contract.RecurrentTask("<code>")` — with `contract.Expect(...)`, **one leg per line**: `contract.PostingLeg(scope, account, kind, sign, cardinality)` for each posting shape, `contract.MovementLeg(from, to, cardinality)` for each movement shape, spelled with catalog codes, a named `contract.Positive` / `contract.Negative`, and `contract.Exactly(n)` / `contract.AtLeast(n)`. No loop, helper or computed leg: a signature change must read in the diff as a changed line naming what changed. The row goes in the set's order — the manual correction, then event types in the event-log migration's declaration order, then deferred-task and recurrent-task types by code. **The capacity postings `store.Move` derives for a movement are declared like any other posting**; the check derives nothing from a movement, so a defect in the derivation cannot pass it.
2. **Check a real transaction in its contract test.** Take a migrated pool from `storetest.Pool(t)`, begin a transaction, set up the fixtures (players, funding, slot grants), and only then call `contract.NewMark(ctx, tx)`. Run the mechanic. Call `contract.Declared()`, and on the registry it returns call `Check(ctx, tx, mark, <the mechanic's type>)`, which must return nil.
3. **Read a failure by its class.** `Check` returns one joined error, and `errors.Is` matches every class present:

| `errors.Is` matches | What it means | Where the fix goes |
|---|---|---|
| `contract.ErrNonconforming` | An entry past the mark differs from its **own** type's row: a declared leg's count is off, an actual row matches no leg, or a kind does not sum to zero. A row that differs from a leg in one dimension (account, kind or sign) reports as a pair — the declared key short, the actual key unexpected — and the message prints both keys whole. | The mechanic or its row, whichever is wrong. |
| `contract.ErrNoSignature` | An entry past the mark has a type with no row — every player-operation entry included, and an entry under a basis table `contract` does not know. | Add the row. A player operation cannot have one until a mechanic gives that table a type within it and `contract` a constructor for that type. |
| `contract.ErrNoDocument` | No entry of the wanted type exists past the mark. | The mechanic wrote nothing, wrote under another type (a deferred task declared as a recurrent one is this case), or the mark was taken after the write. |

**Three limits a mechanic author must be told.**

- **Nothing else may write to the schema between the mark and the check.** `Check` judges every entry past the mark, so a conforming entry committed there by another writer can pass a mechanic that wrote nothing. A schema per test meets this by construction; `Check` therefore belongs in a single-writer contract test, never in a concurrency test.
- **Every entry past the mark is judged by its own type, fixtures included.** Funding taken before the mark is not judged at all; a manual correction written after it is judged against the manual correction's row, `AnyBalanced()`, which admits any balanced postings and any movements. `NewRegistry` refuses `AnyBalanced()` for every other type, so that permission loosens nothing else.
- **Nothing forces a mechanic to call `Check`.** No gate asks whether every balance-moving type has a row; the review checklists rate a balance-moving mechanic with no posting signature or no contract test `major`.

### The payload rule — four dimensions are columns, everything else is JSONB

**The §13.4 dimensions — player, chat, maze, depth — are columns on `event`; everything else about an event is `payload` JSONB.** The rule is uniform across types, so it is one paragraph and not one per type. Four reasons, and they are why the line falls exactly there:

- §13.4 declares those four *universal across types* — «Везде, где применимо: игрок, чат, лабиринт, глубина» — so they are the one part of an event's shape that is not per-type.
- Every shipped view filters or groups on at least one of them; no shipped view reads a payload key at all. A dimension a view groups by wants a btree index, and an index on a JSONB path is an expression index that has to be written per key.
- `player_id` and `chat_id` carry foreign keys to `owner`. JSONB cannot express referential integrity, so putting the player or the chat in the payload would abandon the guarantee the ledger's address space depends on.
- Everything else is per-type and unknowable in advance: a `combat_resolved` payload and a `shop_sale` payload share no field. Promoting either to a column would be a migration per mechanic plus a column that is null on every other type.

**The escape hatch runs one way only, deliberately.** A payload key that turns out to be read by every view can be promoted to a column by a later forward migration, whereas demoting a column is the expensive direction — which is why the columns are the ones §13.4 already fixed, and not a guess about what a future dashboard might want.

### What the shipped views require of the mechanics that emit events

The five MVP views read columns that the mechanic PRs must populate, and **every way of getting it wrong is silent**: a view whose input column is always null returns zero rows rather than an error, and the views' own test fixture passes regardless of what any mechanic actually emits. The obligation is recorded here against the **event type**, because that is what the emitting PR can check itself against.

| Event type | Dimension it must carry | What reads it | What a null does |
|---|---|---|---|
| `bot_added_to_chat` | `chat_id` | `metric_activation_funnel`'s **row set** | that chat has no funnel row at all — §13.3's headline MVP number is silently empty for it |
| `player_started` | `chat_id` | the funnel's **attribution map** | the player is attributed to no chat and is counted in no stage of any funnel |
| `player_started` | `player_id` | the funnel's attribution map; retention's new-player cohort | as above, and the cohort loses the player |
| `raid_started` | `player_id` | the funnel's raided and returned stages; `metric_retention_daily`'s raid count and D1/D7 | the raid counts toward no player and no chat |
| `notification_sent` | `chat_id` | `metric_notification_per_chat_day` | the row is excluded — the spam-budget metric under-reports with no sign that it did |
| `death` | `depth` | `metric_death_by_depth` | the death lands in the null-depth group; **this is the one degradation that is visible**, because the group appears as its own row |
| `death` | `player_id` | that view's distinct-player column | the distinct-player count under-counts |

**And the non-obligations, which matter just as much because a reader looks for them.** No shipped view reads `chat_id` from `raid_started` or from `death`; none reads `maze_id`; none reads any payload key. A mechanic may set them and nothing here degrades if it does not.

**The funnel attributes a player to the chat they started in, and that is not a membership decision.** A player active in several chats is credited to the chat of their earliest `player_started` — ties broken by the lower chat id — even for raids that conceptually belong to another chat. `~/lab-private/DESIGN.md` §16.7 «Привязка игрок↔чат (membership)» is an **open question**, owned by #30; this view does not resolve it. Once a raid session is bound to a chat, a better attribution exists and the view becomes a candidate for a same-column-set replacement.

### An event as a basis document — two consequences a mechanic author must be told

1. **One event backs at most one posting group.** `journal_entry.event_id` carries a partial unique index, so the arc is 1:1. A mechanic that must move balances twice under one conceptual occurrence needs **two events, or a different basis type**. A second `Post` under an already-referenced event is refused by the database with `23505`, not silently merged.
2. **An event carries no idempotency key**, and that is the difference from `player_operation`, which deduplicates on `(source, operation_id)` and turns a replay into `ErrAlreadyPosted`. Two `Post` calls built from equal `Event` values write two event rows and two journal entries. Retry safety for an event-backed mechanic therefore rides on the rail it already has — `player_operation` for a player-initiated action, the scheduler's `(state, seq)` guard for a timer edge (§3.5) — never on the event. Adding a key later is a forward migration; **assuming one exists is a duplication bug.**

### The §13.5 dashboard limb is suspended, not dropped

§13.5's third limb — «харнесс обновляет дашборд при добавлении событий» — is **suspended, not dropped**. There is no dashboard to update: the SQL views ship, the Grafana Postgres datasource and its panels do not.

| If you are... | Then |
|---|---|
| Adding a mechanic that registers new event types | You owe the registry rows, by migration — and **no panel line**, until the check below exists |
| Shipping the infrastructure pass (Grafana datasource, panels, provisioning) | You owe the **lift condition**: a check that reads `event_type_definition` and fails while any registered type has no panel |

Until that check exists, `event_type_definition` is the ledger of what is owed. The limb is written down as suspended rather than left silently unenforced because a limb nobody can satisfy is a limb everybody learns to skip.

### Four view families are owed, each by a named issue

Five of §13.3's families ship as views — `metric_activation_funnel`, `metric_retention_daily`, `metric_death_by_depth`, `metric_faucet_sink`, `metric_notification_per_chat_day`. The other four are blocked on an input that does not exist yet, and each lands as a **later forward migration in its mechanic's PR**, never as an edit to the migration that shipped the first five.

| Family | Blocking input | Owed by |
|---|---|---|
| Raid outcomes | `raid_finished`'s outcome, and a key pairing a finish with its start — neither is a column and no mechanic has defined the payload | #39 |
| Backpack fate | the bare dropped/looted/expired counts follow from the types alone, but §13.3's social metric («Доля подобранных чужих рюкзаков») needs the backpack's owner, which is payload | #40 |
| Button CTR | the linking key between `notification_sent` and `button_clicked` — the two are emitted by different PRs, which must agree on it | #43 and #37 |
| Stamina utilisation | stamina postings, which need a stamina `ledger_kind` member; and an amount in `stamina_burned_overflow`'s payload | #31 |

`bot_kicked` gets no view deliberately, and that is an answer rather than an omission: §13.3 calls it the terminal metric — «Каждое событие — вскрытие» — so the reader wants the individual rows, and the query is `SELECT * FROM event WHERE type = 'bot_kicked'`.

## 6. Telegram safety

- **`ALLOWED_CHAT_IDS` is a code-level safety net**, always on in testing: the bot physically cannot message a destination the outbound gate refuses, even if snapshot sanitisation leaked (§12.5). The gate is `ingest.Gate`, written to occupy `internal/tg`'s per-call `Gate` seam — `tg.Options.Gate`, consulted before every attempt and before any limiter wait, with the client the only route to the generated method surface. **The composition root installs it:** `cmd/bot`'s start-up order builds the gate over the configured allowlist and the pool and hands it to the client constructor, so every outbound call the assembled process could make already passes it. What is still absent is a *sender* — the ingest router and the scheduler registry are both wired empty, so no code path in the shipped tree issues an outbound call. Whichever task first sends one (#43 and the mechanics) inherits a gate that is already installed, which is what makes that a route declaration rather than a search for every send.
- **The gate asks *who*, never what kind of chat — and the allowlist has exactly one carve-out: a player.** A destination is allowed when its `chat_id` token parses as an `int64` present in `ALLOWED_CHAT_IDS`, **or** when an `owner` row exists with `kind = 'player'` and that `telegram_id` — the DM of a player who pressed Start, which is in no operator's list and which §1 nonetheless puts every game command in. The predicate names the kind because a chat's own id lives in that same `telegram_id` column, so a lookup keyed on the id alone would silently admit every chat the bot was ever added to. A call with no destination (`getMe`, `getUpdates`) is allowed; a destination the transport could not read, a token that parses as no integer (a `@channelusername`), and a failed lookup are all refused — the gate fails closed. **Only positive results are cached**, for the gate's lifetime: nothing negative is remembered, so a player refused before their row existed is allowed on the next attempt with no restart and no cache to invalidate. Admitting a player therefore needs no deploy; *revoking* one needs a restart, because a cached positive is never dropped.
- **A handler never issues an outbound call whose permission rests on a row its own uncommitted transaction created.** The gate reads `owner` on a pool connection of its own, so a row the loop's still-open transaction wrote is invisible to it and the call is refused — on that attempt and identically on every retry, each retry being a fresh transaction. The refusal is correct behaviour, not a defect to work around: a Telegram send cannot be rolled back, so a message justified by a row the transaction then rolls back has already reached a real person. The designed route is the outbound queue (#43) — enqueue inside the transaction, send after the commit; until it lands, a mechanic needing a post-commit send performs it outside the handler, after the loop has committed.
- **A production snapshot reaches testing only through sanitisation** — rewrite chat/user ids, drain the outbound notification queue, reset the updates offset. A raw restore means the test bot messages real people. **Three of those obligations are now concrete rows** (#22): reset `ingest_offset`'s singleton row, which is the table «reset the updates offset» had no referent for until it existed; rewrite `ingest_dead_update.chat_id`; and **blank `ingest_dead_update.last_error`**, whose free text is whatever a handler returned and can embed a real chat or user id that a column-targeted id rewrite walks straight past. Blanking is enough because the column's only reader is the operator-facing give-up projection, which branches on nothing in it.
- **Rate limits are enforced by Telegram's core, not by our instance** (§12.2): ~30 messages/second globally, ~20/minute into one chat. Fan-out (season results) goes through the outbound queue with a rate limiter; DMs are request-response and cannot exceed the limit.
- **Honour `retry_after` on 429 and back off exponentially on network errors.** A tight retry loop earns a flood ban attached to the bot id, which reissuing the token does not clear.
- **The Bot API base URL is configuration** with three values on one axis: our own instance (production), cloud `api.telegram.org` (emergency fallback), fake server (evals).

## 7. Determinism

Generation, combat and trail replay are pure functions of `(seed, input)`: no wall clock, no map-iteration order, no unseeded RNG. This is what makes golden tests, replays and PvP-against-a-snapshot free rather than expensive (§2.2.2, §4, §9).

## 8. Balance numbers live in configuration

Stamina cap, step cost, backpack/respawn/standing timers, shop rates, the door price curve, `budget(dist)`, combat dice and scales — **all of them are configuration** (§16.5). A tuning value compiled into Go source is a defect even when it carries a good name: the season's balance is expected to move without a deploy. **The reload policy is start-up only** — `internal/config` reads and validates the tracked balance YAML and the tracked world set once, during start-up, and nothing re-reads either while the process runs, so an edited number takes effect no earlier than the next restart. Without a *deploy*, yes; without a *restart*, no — hot reload is not designed ([`key-decisions.md`](key-decisions.md) KD-24).

**A world's numbers are configuration but are not balance numbers**, and the two are different files. A world's seed, its generation inputs (the chunk radius R, the algorithm weights, the growing-tree bias, and the island, extra-passage and portal shares) and its gate-spacing parameter k are the world's own, authored per world in the world set (§2.2, §2.2.2) — §16.5 does not file them among the open balance numbers, and the balance file carries none of them. The no-compiled-in-fallback rule binds them exactly as hard: an absent world key is a start-up refusal naming it, never a default, because an authored zero is a real value there ([`key-decisions.md`](key-decisions.md) KD-43, KD-44).
