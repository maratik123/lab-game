# Item machine: `item`, `item_movement`, and the shared holder address space

**Source:** issue #25
**Date:** 2026-09-12
**Tracked in:** #25

The quantitative ledger already accounts for fungibles. This task adds the second
accounting machine — ownership of instances that have identity — and joins the two
by giving them one address space, so that every logistics operation in the game is
"move under a document" and nothing else.

## Scope
1. A forward migration brings the item machine into being: an instance with its own identity, an append-only record of every movement of an instance between holders under exactly one basis document, the holder address space the quantitative ledger's accounts already live in, the ledger kinds that measure capacity with the account definitions that carry them, and the player's backpack and storage scopes. [task: "Forward migration: `item`, `item_movement`, the holder address space, the capacity `ledger_kind` members and the `account_definition` rows that carry them, plus the player's backpack and storage scopes"]
2. Moving item instances is one operation under one basis document: it takes that document and a set of movements, writes the quantitative capacity postings under the same document in the same transaction, and refuses anything that would break the chain. [task: "it takes a basis document and a set of movements, writes the quantitative capacity postings under the same document in the same transaction, and refuses anything that would break the chain"]
3. Capacity is enforced through the quantitative ledger's existing `CHECK (balance >= 0)` path, so an over-stuffed backpack is unrepresentable rather than validated. [task: "Capacity enforcement through the existing `CHECK (balance >= 0)` path, so an over-stuffed backpack is unrepresentable rather than validated"]
4. Chain continuity, and the agreement between the two machines, are each answerable by a query over the shipped data. [task: "Chain-continuity reconciliation as a query, plus the cross-machine check"]
5. A holder the MVP does not use — a corpse, a construction, a chest — is an ordinary address in the same space, reachable by the same move operation and needing no code of its own. [task: "There is no separate code for corpses, constructions or chests"] [task: "the address space must not make them a schema change"]
6. Every live document whose claim about the item machine this migration falsifies states what shipped instead; the class is every site that describes the item machine's tables or its holder address space, per AGENTS.md § *Propagation Rule* step 4, and any site known at drafting illustrates the class rather than bounding it. [task: "`item` (identity and per-instance properties) plus append-only `item_movement` (item_id, from_holder, to_holder, basis document through the same exclusive arc)"]

## Out of scope
- Item definitions and their stats — #32 delivers them; this task is the accounting layer under them. [task: "Item *definitions* and their stats — #32."]
- Corpse holders and their TTL rules — #40 delivers them; this task only makes a corpse a representable address. [task: "Corpse holders and their TTL rules — #40"]
- Construction holders — post-MVP. [task: "Construction holders — post-MVP (§14 excludes buildings)"]
- Durability and repair — not designed. [task: "Durability and repair — not designed (§6.5 points at IDEAS.md)."]
- The framework that checks a basis document's declared posting and movement sets against what it actually wrote — #26 delivers it. [task: "the framework that checks this is #26"]

## Deferred
- The inventory, chest and construction scopes beyond what this task's migration creates, and the kinds they need | each arrives with the mechanic that uses it | no — part of those tasks.

## Key decisions
| Question | Decision |
|---|---|
| What capacity measures | TBD — asked in round 1. |
| Whether an item instance carries mutable per-instance properties here, and whether their earlier values must stay recoverable | TBD — asked in round 1. |
| The player's storage scope and its chat binding | TBD — asked in round 1. |

## Source conflicts
`docs/DESIGN.md` §11 names the item machine's two tables in the plural while the
same section's naming decision requires the singular. Both sites, verbatim
[source: 7276882:docs/DESIGN.md § 11. Технические решения · `grep -n 'единственном числе\|item_movements' docs/DESIGN.md`]:

- «имена таблиц — в единственном числе, решение 2026-09-02»
- «Таблица `items` (identity и свойства экземпляра) + append-only `item_movements`: item_id, from_holder, to_holder, документ-основание (тот же exclusive arc)»

Resolution: the singular. Chosen by the task text itself, whose Scope line names
`item` and `item_movement`, and which agrees with the naming decision of the same
section; the plural prose of the item-machine bullet is the outlier of the three.

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | An item instance is a record with its own identity, and every movement of an instance is an appended record naming the instance, the holder it came from, the holder it went to, and exactly one basis document. [task: "`item` (identity and per-instance properties) plus append-only `item_movement` (item_id, from_holder, to_holder, basis document through the same exclusive arc)"] |
| AC2 | A movement whose `from` holder is not the instance's current holder is refused, and no accepted movement leaves an instance at other than exactly one holder. [task: "every movement's `from` equals the previous movement's `to`; an instance has exactly one holder at any moment"] |
| AC3 | An accepted set of movements and the capacity postings it produces in the quantitative ledger carry the same basis document and land in the same transaction: neither is durable without the other. [task: "an instance movement emits a quantitative posting in the main ledger under the same document, in whatever kind measures capacity (slots / weight)"] |
| AC4 | A move that would leave a holder holding more than its capacity admits is refused by the quantitative ledger's `CHECK (balance >= 0)` path, not by a check written beside it. [task: "Capacity enforcement through the existing `CHECK (balance >= 0)` path, so an over-stuffed backpack is unrepresentable rather than validated"] |
| AC5 | Whether every instance's chain is continuous, and whether the instance count at a holder equals that holder's capacity balance, are each answerable by a query over the shipped data. [task: "Chain-continuity reconciliation as a query, plus the cross-machine check"] |
| AC6 | A player backpack, a player chest, a corpse, a construction and World are each an address in the one space the item machine and the quantitative ledger share, and a move between any two of them is the same operation. [task: "player backpack, player chest, corpse #123, construction, World"] |
| AC7 | Nothing in the machine branches on a holder's kind, and admitting a holder kind the MVP does not use costs no restructuring of the item machine. [task: "There is no separate code for corpses, constructions or chests"] [task: "the address space must not make them a schema change"] |
| AC8 | Of two moves of the same instance attempted at the same time, at most one is accepted, and the instance's chain is continuous afterwards. [task: "two simultaneous moves of the same instance"] |
| AC9 | TBD — which ledger kinds measure capacity, and which scopes the migration creates, follow from the round-1 questions. |

## Open questions
- None yet; the three questions in flight are the TBD rows of *Key decisions*.
