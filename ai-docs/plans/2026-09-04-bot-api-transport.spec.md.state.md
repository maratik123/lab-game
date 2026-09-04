# Interview state — Bot API transport over telego

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-04-bot-api-transport.spec.md
issue_ref: "#19"
gh_issue:
  title: "Bot API transport over telego: retries, retry_after, rate limits, base-URL axis"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What

    A thin transport layer over the `telego` low-level client: retries, honest 429 handling, rate limits, and the three-value base-URL axis. Every outbound Telegram call in the project goes through it.

    ## Design refs

    - `docs/DESIGN.md` §11 — **library: `mymmrac/telego`, low-level layer only** (one-to-one generated types/methods; full Bot API coverage by construction). Swap fasthttp/go-json for `net/http` and `encoding/json` (supported natively). **Do not use telego's helper layer**; this thin layer is ours: retries, idempotency, rate limits (30 msg/sec global, ~20 msg/min into one chat).
    - `docs/DESIGN.md` §11 — transport is honest from day one: respect `retry_after`, exponential backoff on network errors. **A tight retry loop is the classic way to earn a flood ban, and a flood ban attaches to the bot id — reissuing the token does not clear it** (`AGENTS.md` § Domain Rules).
    - `docs/DESIGN.md` §11 — base URL is a config value with three settings: own instance / cloud / fake server.
    - `docs/DESIGN.md` §11 — the only scenario that meets the global limit is fan-out; an outbound queue with a rate limiter spreads it over time (the queue itself is #43).
    - `docs/DESIGN.md` §13.2 — the health dashboard wants per-method latency and response codes, plus 429 and retry counters. Expose the hooks here; #23 wires them.
    - Fallback if `telego` disappoints: `go-telegram/bot`. **Never** `go-telegram-bot-api` (frozen).

    ## Depends on

    #18

    ## Scope

    - `internal/tg` (name to be settled in the spec): a client wrapping telego's low-level API with `net/http` + `encoding/json`.
    - Retry policy: exponential backoff with jitter on network / 5xx; `retry_after` honoured exactly on 429, never shortened.
    - Two rate limiters: global 30/sec and per-chat ~20/min, both config-driven.
    - Per-method instrumentation hooks (latency, status code, 429 count, retry count) with no metric registry dependency baked in.
    - A minimal fake Bot API server sufficient for this package's own tests (the full eval harness is #44).
    - Context cancellation honoured on every call; no unbounded blocking.

    ## Out of scope

    - The update loop and dispatch — #22.
    - The notification queue — #43.
    - Running a self-hosted `telegram-bot-api` instance (§12.2) — the infrastructure pass.

    ## Telemetry obligation

    - Metrics (exposed here, registered in #23): request latency by method, response codes by method, 429 counter, retry counter.

    ## Open questions to close in the spec

    - Whether the per-chat limiter keys on chat id alone or on (chat id, method class).
    - Retry budget: attempts, ceiling, and what a give-up looks like to the caller.

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#18", "#22", "#23", "#43", "#44", "#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
