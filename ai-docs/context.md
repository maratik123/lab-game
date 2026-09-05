# Project context — lab-game

## Purpose

A **Telegram bot game** (working title «Лабиринты»): adding the bot to a group chat gives that chat a *home* in a shared, infinite, procedurally generated hex maze. Players issue commands in DM with the bot; the group chat receives only rare, status-worthy notifications. The chat is a collective entity — shared buildings, shared reputation, shared map knowledge.

The canonical, finalized specification is **[`docs/DESIGN.md`](../docs/DESIGN.md)** (Russian) — read it for anything non-obvious about the world, raids, combat, economy, PvP, seasons, or infrastructure. Its idea backlog is **[`docs/IDEAS.md`](../docs/IDEAS.md)**, which is *not* a source of implementable behaviour (`AGENTS.md` § Project carries the AXIOM). This file is the short orientation; the design document is the source of truth.

## The positioning that decides ties

> **The unit of acquisition is a chat, not a player.** When two designs compete, the one that strengthens *chat-as-an-entity* wins (shared map, doors as infrastructure, walking newcomers in, chat legends in notifications) over the one that strengthens an individual.

The mechanics are deliberately conventional (stamina, auto-combat, seasons, an extraction loop) so players understand the rules immediately. What is novel is the combination, not the bricks.

## Load-bearing invariants worth knowing before touching a block

- **The maze is a pure function of its seed.** Cell content and its edges are `f(world_seed, coord)`; a cell materialises in the database on first visit; connectivity is guaranteed *by construction* (per-chunk spanning structure + 1–2 generated portals per chunk border), never repaired after the fact. Rotation of a season = a new `world_seed` (`docs/DESIGN.md` §2.2.2).
- **Edges are not stored.** Passages and walls are derived from `f()`; the canonical face representation makes both sides of a border agree by construction (§2.2.4).
- **Danger is a property of space, not of the observer.** Depth = distance to the nearest *chat entrance* (crafted doors do not count). PvP tier, monster budget, wave multiplier and newcomer XP all hang off that one axis (§2.2).
- **A raid session is a row, not a process.** An explicit FSM in Postgres: player actions and timers are edges of the same kind, every transition is a guarded transaction plus a basis document, and timer edges are one-shot scheduler tasks that die on a stale guard. Bot restarts are invisible to sessions (§3.5).
- **Combat is a pure function** `combat(seed, party, monsters) → outcome + log`. Nothing outside sees its internals; the combat-system version is part of every stored log and PvP-trail snapshot, which is what buys the freedom to rewrite it wholesale (§4).
- **Every balance change is a ledger posting.** Double-entry with a global "World" account, zero-sum per kind inside each transaction, checked at write time in `store.Post`; instance-carrying items live in a second machine with an unbroken chain of holders. All game logistics — death, looting, contributions, corpse evaporation — are one "move under a document" operation over those machines (§11, and [`domain-invariants.md`](domain-invariants.md)).
- **Lazy ticks, no cron.** Stamina, corpse evaporation and respawn are computed from timestamps at read time; the single exception is the Postgres-backed scheduler that drives raid-session timer edges, notifications and day-close (§11).

## Architecture

The design document defines the blocks; the Go package layout lands one implementation spec at a time, never by assumption. **Layout so far:** `internal/store` — the ledger's write path (owner/scope/account catalog, forward migrations under `internal/store/migrations`, `store.Post`); `internal/testdb` — PostgreSQL provisioning for package tests (a `postgres:18` container or `LAB_GAME_TEST_DSN`, one schema per test); `internal/config` — the start-up configuration layer (the `LAB_GAME_` environment set, the tracked balance YAML, the world-set path), which `cmd/bot` loads and validates before any other work; `internal/tg` — the Bot API transport over `telego`'s low-level layer (the caller with retries and exact `retry_after`, the window-schedule rate limiter, the `Gate` seam #22 installs its allowlist into, the observation point #23 reads), with `internal/tgtest` as its in-process fake Bot API server; `internal/scheduler` — the Postgres-backed task scheduler over the `scheduled_task` table (the handler registry with its one-shot and recurrent declarations, the worker's claim/execute/settle cycle and its per-task deadline, the start-up reconciler that seeds and corrects recurrences, and the observation seam #23 reads), which ships no production handler — each of those arrives with its own mechanic. The blocks the layout has to house:

| Block | Responsibility | Design ref |
|---|---|---|
| Transport | Bot API client over a self-hosted `telegram-bot-api`, retries, 429/`retry_after`, rate limits, idempotency of updates | §11, §12.2 |
| World | Deterministic chunked generation, prefabs, materialisation, discovery/knowledge | §2.2 |
| Raid | Session FSM, leader screen (one edited message), stamina, standing-timer escalation | §3, §5 |
| Combat | Pure simulator + narrative rendering from the world's vocabulary | §4 |
| Economy | Double-entry ledger (`internal/store`), item machine, logistics addresses, shop/craft | §6, §11 |
| Scheduler | `scheduled_task` worker (`FOR NO KEY UPDATE SKIP LOCKED`), one transaction per task with its effects | §11 |
| Notifications | Outbound queue with a rate limiter; the notification budget is a design obligation | §1, §13.3 |
| Observability | `events` log (product) + Prometheus metrics and canaries (health) | §13 |

## Status (2026-09-05)

- **Design:** finalized in `docs/DESIGN.md`; open questions live in its §16 (loot split in a group, all balance numbers, the game's name, player↔chat membership).
- **Code:** the ledger core — `internal/store` (`Migrate`, `NewPool`, `CreateOwner`, `Post`) with its first migration, and `internal/testdb`; the configuration layer — `internal/config` with its tracked balance file; the Bot API transport — `internal/tg` (retries, exact `retry_after`, the window-schedule rate limiter, the observation seam) over `telego`'s low-level layer, with `internal/tgtest` as its in-process fake server; the task scheduler — `internal/scheduler` (the `scheduled_task` migration and the two new ledger basis documents, `Registry.Schedule`, the worker's claim/execute/settle cycle, the per-task execution deadline, `Reconcile`, the observation seam) with its six `LAB_GAME_SCHEDULER_*` tuning keys. `cmd/bot` still only loads and validates configuration at start-up: nothing constructs a client or a worker yet, because there is no update loop to feed it (#22), no queue to send through it (#43) and no production handler to register — the raid timer edges (#36), corpse evaporation (#40) and the day close (#45) each ship their own. MVP scope is `docs/DESIGN.md` §14.
- **Gates:** one entry point — `make verify` runs the whole gate list, and CI invokes the same sub-targets, so hook, CI and a local run cannot disagree. Format gate is `golangci-lint fmt -d` (gofumpt included); file size is gated at 1000 / 1500 lines. Per-task detail: [`context-status.md`](context-status.md).
- **Harness:** being ported from the `graphite-gp` project (which in turn evolved it from `quartzite`), adapted to Go and to this domain.
- **Repository:** `maratik123/lab-game`, private, default branch `main`. No server-side branch protection — see `AGENTS.md` § Permissions.

Key decisions with rationale: [`key-decisions.md`](key-decisions.md). Domain invariants that outrank convenience: [`domain-invariants.md`](domain-invariants.md).
