# Domain invariants

> Extracted from `AGENTS.md` § *Domain Rules*. That file keeps the binding AXIOMs; this page carries the mechanics and the reasoning. Every claim here traces to a section of [`docs/DESIGN.md`](../docs/DESIGN.md) — when the two disagree, the design document wins and this page is the bug.

## 1. The ledger — every balance moves by posting

**Rule.** Stamina, resources, money and item capacity change **only** through postings written by `store.Post`, under exactly one basis document, and the postings of one transaction sum to zero per kind. Emission and burning are postings against the global **World** account, which alone may go negative (`docs/DESIGN.md` §11).

| If you are about to... | Do this instead |
|---|---|
| `UPDATE players SET stamina = stamina - 3` | Post `player → World` for kind `stamina`, under the basis document for the move |
| Grant a reward by inserting a row into an inventory table | Move the instance in the item machine + post the capacity kind, both under one document |
| "Fix" a wrong balance with an `UPDATE` | Append a compensating posting under a `manual correction` document — history is append-only |
| Write a posting with no basis | There is no such thing. A posting in mid-air is a modelling error, and `store.Post` refuses it |

**Why the zero-sum check lives in `store.Post`.** It is the compiler of the economy: the "forgot the second leg" class of bug becomes impossible at write time, and by induction the whole ledger sums to zero per kind forever. A daily close then re-verifies the chain (opening + turnover == closing); a mismatch is an alert, and an alert here always means a code defect, because every source is in the same database.

**Deadlock discipline is `store.Post`'s job, never the caller's** (§11): dedupe and sort the touched controlled accounts by `(account_id, kind)`, do **all** balance `UPDATE`s first in that order — as separate statements, never one batched `UPDATE` whose lock order follows an unowned plan — then insert the postings. A wait cycle is impossible by construction.

**Negative balances.** `CHECK (balance >= 0)` on controlled accounts (players); the World account is exempt. Stamina may become a designed exception (a debt/debuff that regen pays off) — that is a balance decision, still open. For resources and money, a negative balance is a duplication exploit, never a feature.

## 2. The item machine — identity is not a kind

Items with identity (durability, enchantment) are **never** modelled as ledger kinds. They live in `items` + append-only `item_movements`, and the invariant is **chain continuity**: each movement's `from` equals the previous movement's `to`, so an instance has exactly one holder at any moment (§11).

Holders of the item machine and accounts of the quantitative machine share **one address space** — player backpack, player chest, corpse 123, construction site, World. Death, looting, contribution and evaporation are the *same* "move under a document" operation with a different holder; there is no bespoke corpse code to write. A movement also posts the capacity kind (slots/weight) in the main ledger under the same document, and a cross-machine reconciliation ("instances held == slots consumed") catches drift.

**Free consequence:** the movement chain is an item's provenance — "crafted by Вася, lost on node 12, picked up by the fishermen's chat" — which notifications and season stories can read without any extra code.

## 3. Basis documents — exclusive arc, not a polymorphic pair

Each basis-document type is its **own table** with its own schema and lifecycle (Telegram operation, raid-session transition, cron day-close, deferred one-shot, recurring task, manual correction, season close). A posting carries one nullable FK column per type plus `CHECK (num_nonnulls(...) = 1)`. A polymorphic `(doc_type, doc_id)` pair without referential integrity was considered and **rejected** (§11).

Adding a document type is a migration plus a `CHECK` edit — deliberately: a new kind of document passes an explicit review, and the `CHECK` doubles as a greppable registry of every type that exists.

**Idempotency rides on the same rail:** a unique constraint `(source, operation_id)` on the player-operation document (`player_operation`, `source` enum) means a replayed update creates no document, therefore no postings. There is no second idempotency mechanism to keep in sync.

## 4. The raid FSM and the scheduler

A session is a row: `(id, maze_id, position, state, leader, participants, arrived_at, screen_message_id, seq)`. One engine drives it — `transition(session_id, edge, expected_seq)` inside a transaction with a guard on `(state, seq)`; effects, then `state`/`seq` advance, then the new state's timer edges are scheduled (§3.5).

- **A player action and a timer are edges of the same kind.** Anything true of one is true of the other.
- **A stale task is expected traffic, not an error.** Timer edges carry `expected_seq`; when the guard misses, the task dies on execution. Cancellation is unnecessary and orphans are harmless — write tests for that path, do not "fix" it.
- **Button presses are idempotent through the same guard**: `seq` travels in `callback_data`, a stale press redraws instead of acting. This covers double clicks, Telegram retries, and presses on an old message.
- **A task executes in one transaction with its effects** — exactly-once without two-phase machinery. Task types are the ledger's basis-document types, not a foreign job model.
- **The scheduler is load-bearing and its lag is visible in gameplay** ("a wave in five minutes" arriving in seven). Lag by task type belongs on the health dashboard (§13.2).

## 5. Telemetry is part of the mechanic

> **A new mechanic declares its events in the same PR that implements it** (§13.4). If it moves balances, it also declares the **posting signature** of its basis document, and the contract test checks the actual postings against that signature.

The product dashboard reads the raw `events` log, never pre-aggregated counters — what you did not record, you cannot ask later. The ledger and the event log are **different tables** (strict schema vs JSONB, different retention, different readers); postings reference an event as one of their basis types.

## 6. Telegram safety

- **`ALLOWED_CHAT_IDS` is a code-level safety net**, always on in testing: the bot physically cannot message a chat outside the list even if snapshot sanitisation leaked (§12.5).
- **A production snapshot reaches testing only through sanitisation** — rewrite chat/user ids, drain the outbound notification queue, reset the updates offset. A raw restore means the test bot messages real people.
- **Rate limits are enforced by Telegram's core, not by our instance** (§12.2): ~30 messages/second globally, ~20/minute into one chat. Fan-out (season results) goes through the outbound queue with a rate limiter; DMs are request-response and cannot exceed the limit.
- **Honour `retry_after` on 429 and back off exponentially on network errors.** A tight retry loop earns a flood ban attached to the bot id, which reissuing the token does not clear.
- **The Bot API base URL is configuration** with three values on one axis: our own instance (production), cloud `api.telegram.org` (emergency fallback), fake server (evals).

## 7. Determinism

Generation, combat and trail replay are pure functions of `(seed, input)`: no wall clock, no map-iteration order, no unseeded RNG. This is what makes golden tests, replays and PvP-against-a-snapshot free rather than expensive (§2.2.2, §4, §9).

## 8. Balance numbers live in configuration

Stamina cap, step cost, backpack/respawn/standing timers, shop rates, the door price curve, `budget(dist)`, combat dice and scales — **all of them are configuration** (§16.5). A tuning value compiled into Go source is a defect even when it carries a good name: the season's balance is expected to move without a deploy.
