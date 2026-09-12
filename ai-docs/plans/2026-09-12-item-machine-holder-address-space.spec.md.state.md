# Interview state — item machine: item, item_movement, and the shared holder address space

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md
issue_ref: "#25"
gh_issue:
  title: "Item machine: item, item_movement, and the shared holder address space"
  state: open
  labels: ["mvp", "area:ledger"]
  issue_body_status: current
  body: |
    ## What

    The second accounting machine. The quantitative ledger handles fungibles; an item with identity (a sword with durability and an enchantment) cannot be squeezed into a `kind` — the design calls that a fundamental modelling error (`docs/DESIGN.md` §11). Items get an ownership machine instead, and the two machines share one address space, which is what makes all of the game's logistics a single operation.

    ## Design refs

    - `docs/DESIGN.md` §11 — `item` (identity and per-instance properties) plus append-only `item_movement` (item_id, from_holder, to_holder, basis document through the same exclusive arc).
    - `docs/DESIGN.md` §11 — **the invariant is chain continuity, not a sum**: every movement's `from` equals the previous movement's `to`; an instance has exactly one holder at any moment. Reconciliation walks chains.
    - `docs/DESIGN.md` §11 — **machine linkage**: an instance movement emits a quantitative posting in the main ledger under the same document, in whatever kind measures capacity (slots / weight). The main ledger enforces backpack capacity through its `CHECK`; a cross-machine reconciliation ("instance count at a holder equals the slot balance") catches divergence. One document, postings in both machines, both signatures in the contract test.
    - `docs/DESIGN.md` §11 — **logistics sits on top**: item-machine holders and ledger accounts are **one address space** — player backpack, player chest, corpse #123, construction, World. Death, pickup, contribution, backpack-to-chest, corpse evaporation are all the same "move under a document" operation. **There is no separate code for corpses, constructions or chests** — the game logic above the machines is only the rules about *who is allowed* to initiate a move.
    - `docs/DESIGN.md` §11 — free bonus: the movement chain is an item's provenance, material for notifications and seasonal stories at no extra cost.
    - `ai-docs/deferred/_inbox.jsonl` — backpack / inventory / construction scopes, their kinds and the capacity sub-account mechanism were deferred to "their mechanics"; the **address space and the capacity mechanism** are this issue's, the per-mechanic scopes arrive with each mechanic.
    - `AGENTS.md` § Domain Rules — balances move only through the ledger; an item moves only through `item_movement`. A handler mutating either directly is rejected in review.

    ## Depends on

    _None — this one can start immediately._

    ## Scope

    - Forward migration: `item`, `item_movement`, the holder address space, the capacity `ledger_kind` members and the `account_definition` rows that carry them, plus the player's backpack and storage scopes — **whose chat binding is the open question below, not a decision this line makes**.
    - A `Move` API in `internal/store` mirroring `Post`: it takes a basis document and a set of movements, writes the quantitative capacity postings under the same document in the same transaction, and refuses anything that would break the chain.
    - Capacity enforcement through the existing `CHECK (balance >= 0)` path, so an over-stuffed backpack is unrepresentable rather than validated.
    - Chain-continuity reconciliation as a query, plus the cross-machine check.
    - Property tests (`pgregory.net/rapid`, the `Post` precedent) for chain continuity and single-holder-at-a-time, and a `-race` concurrency test for two simultaneous moves of the same instance.

    ## Out of scope

    - Item *definitions* and their stats — #32.
    - Corpse holders and their TTL rules — #40; this issue only makes "corpse" a representable address.
    - Construction holders — post-MVP (§14 excludes buildings), but **the address space must not make them a schema change**.
    - Durability and repair — not designed (§6.5 points at IDEAS.md).

    ## Telemetry obligation

    - Posting signature: every basis document that moves an item declares its movement set alongside its posting set; the framework that checks this is #26.

    ## Open questions to close in the spec

    - What capacity measures: slots, weight, or both, and whether that choice is per-kind config.
    - Whether an item's mutable properties (durability, enchantment) live on `item` as columns or as an append-only property history.
    - **Player storage scope(s) and their chat binding — undecided, and the owner's call.** Parked twice in `ai-docs/deferred/_inbox.jsonl`: `:21` "Player storage scope(s) and their chat binding — undecided (owner, round 9)", and `:28` quoting the owner verbatim — «с привязкой к групповому чату, или без привязки, пока не решено» (*bound to the group chat, or unbound — not yet decided*). It interacts with §16.7, which #30 closes: if a raid is always bound to a chat, a chat-bound storage scope is coherent; if storage is unbound, the backpack head start of §3.4 still has to get the session's chat from somewhere. **The spec surfaces the trade-off; it does not pick.**

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#26", "#30", "#32", "#40", "#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: aefe725063b6523c3
prior_qa: []
```
