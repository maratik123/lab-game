# Interview state — update ingestion: polling, dispatch, idempotency, allowlist

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md
issue_ref: "#22"
gh_issue:
  title: "Update ingestion: long polling, dispatch, operation idempotency, chat allowlist"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What
    
    The bot's front door: long polling against the configured Bot API base URL, dispatch to handlers, idempotency of updates, and the chat allowlist that keeps the bot from ever writing to a chat it was not meant for.
    
    ## Design refs
    
    - `docs/DESIGN.md` §11 — **the bot is a stateless update handler over Postgres**; restarts and deploys are trivial. No in-memory session state.
    - `docs/DESIGN.md` §11 — **idempotency runs through the basis document**: a unique constraint on (source, operation_id) on `player_operation`. A repeated update creates no document, therefore no postings — one constraint protects the whole cascade. No separate mechanism is needed. (`internal/store` already has the table and the `operation_source` enum with `telegram`.)
    - `docs/DESIGN.md` §13.2 — handler duration per update, handler errors and panics, **idempotency hits (duplicates are normal; a spike is a signal)**, and **update lag (update date vs handling moment) — the single best early indicator of a stall, the star of the health dashboard**.
    - `AGENTS.md` § Domain Rules — **never write to a chat that is not the intended one**: `ALLOWED_CHAT_IDS` gates outbound traffic.
    - `AGENTS.md` § Go Test Conventions — no `panic` in production code; a handler panic is caught, recorded and does not take the process down.
    - `ai-docs/deferred/_inbox.jsonl` — Telegram's `update_id` and `callback_query.id` live in different id spaces; `operation_id` must not conflate them (a prefix is the obvious answer, to be settled here).
    
    ## Depends on
    
    #19 #21
    
    ## Scope
    
    - The polling loop: offset persistence, allowed-update filtering, backoff on transport errors, clean shutdown.
    - Dispatch: a router from update shape to handler, with a handler contract that takes a context and a transaction-capable store.
    - Idempotency: derive `operation_id` per update kind with a namespace prefix; create the `player_operation` basis document; a duplicate short-circuits without side effects.
    - The `ALLOWED_CHAT_IDS` gate, applied where it cannot be bypassed by a handler.
    - Panic recovery per update, with the metric and the event.
    - Instrumentation hooks for update lag, handler duration, errors and idempotency hits.
    
    ## Out of scope
    
    - Any specific handler — those ship with their mechanics (#30, #42, #37).
    - Webhooks. Long polling is the design's choice; do not build a second ingestion path.
    
    ## Telemetry obligation
    
    - Events: none directly, but the loop carries the plumbing every handler's event write uses.
    - Metrics: update lag, handler duration, handler errors, handler panics, idempotency hits, poll cycle duration.
    
    ## Open questions to close in the spec
    
    - The `operation_id` namespace scheme, and what it must look like so a third id space later does not collide retroactively.
    - Whether a handler failure re-polls the same update or drops it (and what that means for idempotency).
    
    ---
    
    Part of #47 (MVP roadmap).
    
  comments: []
  linked_issues: ["#19", "#21", "#30", "#37", "#42", "#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: a1ae9486e8e4caa8e
prior_qa:
  - round: 1
    question: "Where is a repeated update caught - in the loop before dispatch, or at the handler's store.Post?"
    answer: "Handler's Post - the handler builds the PlayerOperation basis from the update key the loop hands it and passes it to store.Post; the existing ErrAlreadyPosted path refuses the replay and internal/store is untouched. An update that moves no balance writes no document and its non-ledger effects replay in full."
  - round: 1
    question: "When a handler returns an error, does the polling offset advance past that update?"
    answer: "Retry, then advance - bounded in-process retry with a growing delay up to a cap, then advance past it and record a poison update. Mirrors the scheduler's one-shot failure policy and its terminal give-up state."
  - round: 1
    question: "What does the outbound gate do with a chat id that is not in ALLOWED_CHAT_IDS?"
    answer: "DM always allowed - the list governs group and channel chats; every private chat is allowed. Simplest, no database read; the safety net then covers group traffic only."
```
