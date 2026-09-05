# Interview state — event log, the §13.4 dictionary, and the MVP SQL views

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md
issue_ref: "#21"
gh_issue:
  title: "Event log: the event table, the §13.4 dictionary, and the MVP SQL views"
  state: open
  labels: ["mvp","area:observability"]
  body: |
    ## What

    The product-analytics half of observability: an append-only `event` table with a JSONB payload, plus the starter event dictionary. The design's stance is explicit — **log raw events, not aggregates**; at MVP scale every dashboard is a SQL query over the log, and aggregating early loses what you did not record (`docs/DESIGN.md` §13.1).

    ## Design refs

    - `docs/DESIGN.md` §13.1 — append-only `event` in the same Postgres: player, chat, type, JSONB payload, timestamp. A handful of SQL views answers every question up to hundreds of chats.
    - `docs/DESIGN.md` §13.4 — the starter dictionary: `bot_added_to_chat`, `player_started`, `raid_started`, `node_entered`, `combat_resolved`, `raid_finished` (with outcome), `death`, `backpack_dropped`, `backpack_looted`, `backpack_expired`, `stamina_burned_overflow`, `shop_sale`, `shop_purchase`, `notification_sent`, `button_clicked`, `bot_kicked`. Wherever applicable: player, chat, maze, depth.
    - `docs/DESIGN.md` §11 — **a game event is one of the ledger basis-document types**; postings reference it through the exclusive arc. **The ledger and the event log are different tables** (strict schema vs JSONB, different retention, different readers).
    - `docs/DESIGN.md` §13.4 + `AGENTS.md` § Domain Rules — **a new mechanic declares its events in the same PR that implements it.** Telemetry never lags code.

    ## Depends on

    _None — this one can start immediately._

    ## Scope

    - A forward migration: `event` table plus its arc column on `journal_entry` and the updated `CHECK (num_nonnulls(...) = 1)`.
    - A typed Go API for writing an event inside a caller's transaction, so an event and the postings that reference it are one atomic act.
    - The event-type registry as data the code and the database agree on (the `store` catalog-mirror test is the precedent worth copying).
    - Payload discipline: which fields are columns (player, chat, maze, depth) and which live in JSONB, with the reasoning written down.
    - The MVP SQL views that §13.3 asks for, as tracked files: activation funnel, retention, raid outcomes, deaths by depth, backpack fate, stamina utilisation, faucet/sink per resource, notifications per chat per day and button CTR.

    ## Out of scope

    - The Grafana Postgres datasource, panels and provisioning JSON — the infrastructure pass. **The views land here; the dashboard that reads them does not.** The §13.5 third limb of the telemetry invariant (`docs/DESIGN.md:440` — the harness updates the dashboard when an event is added) is **suspended until that pass, not dropped**, and this issue's registry is what keeps the suspension mechanical: the infrastructure pass ships a check that reads the event-type registry defined here and **fails while any registered type has no panel**. Mechanic issues add no panel line in the meantime — the registry is the ledger of what is owed.
    - Event retention and partitioning (§11) — not MVP.
    - Emitting the events themselves: each mechanic emits its own, in its own PR.

    ## Telemetry obligation

    This issue *is* the telemetry substrate. It must ship the dictionary as a registry that a later mechanic extends by migration, not by editing a comment.

    ## Open questions to close in the spec

    - Whether `notification_sent` and `button_clicked` (high-volume, health-flavoured) belong in the same table as the low-volume product events, or want their own retention from day one.
    - **Membership of the basis-document type registry** — recorded unresolved by the ledger-core spec (`ai-docs/plans/done/2026-09-02-ledger-post-core.spec.md:524-546`) and handed forward to "whoever lands the event log", which is this issue. `docs/DESIGN.md:331` labels the starting registry yet omits the game event that `:323` and `:332` both name, while `:323` omits the raid-session transition. In practice this issue adds one of the two missing arc columns and #36 adds the other, so the discrepancy closes by construction — **record it as closed rather than leaving it implicit**: the `CHECK (num_nonnulls(...) = 1)` is the grep-able registry of every type, and a reader who does not find the question answered will re-derive the same conflict.

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#36", "#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: af42b6350aa60c754
prior_qa:
  - round: 1
    question: "Do the two high-volume, health-flavoured types — notification_sent and button_clicked — share the `event` table with the fourteen low-volume product types, or get their own table from day one?"
    answer: "One table + class. All sixteen types land in `event`. The registry carries a volume class per type, so the later retention pass can split them without a rewrite. Every view is one FROM."
  - round: 1
    question: "Four of the nine MVP views read payload keys or ledger kinds no mechanic has defined yet — raid_finished's outcome, the backpack's owner, the click-to-notification linkage, and stamina postings. How do those four ship?"
    answer: "Ship the five. Only the families computable from the columns and today's ledger ship now. The other four land in their mechanic's PR, recorded as deferred items here."
  - round: 1
    question: "Both source conflicts close by touching docs/DESIGN.md: §13.1/§13.5 spell the table `events` against §11:313's singular-names decision, and §11:331's «Стартовый реестр типов оснований» omits the game event that §11:323 and §11:332 both name. Does this PR edit the design corpus?"
    answer: "Edit both. §13.1/§13.5 spelling becomes `event`, and §11:331 gains «игровое событие (лог 13.1)». Every live doc agrees and neither conflict is re-derivable by a later reader."
```
