# Interview state — health metrics and canaries

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md
issue_ref: "#23"
gh_issue:
  title: "Health metrics and canaries: /metrics, update and scheduler lag, dual getMe"
  state: open
  labels: ["mvp", "area:observability"]
  body: |
    ## What

    The health half of observability: a Prometheus endpoint carrying the metrics the previous platform issues already instrument, plus the two end-to-end canaries the design asks for by name.

    ## Design refs

    - `docs/DESIGN.md` §13.1 — health is metrics (Prometheus format) from the transport layer, the handlers and the runtime. Two stacks, two dashboards; this is the health one.
    - `docs/DESIGN.md` §13.2 — the full list: Bot API latency and response codes by method, 429 and retry counters; **update lag — the dashboard's star**; **scheduler lag by task type** (its delay is visible as gameplay); handler duration, errors and panics, idempotency hits; notification queue depth, send lag and failures; pgx pool occupancy and waits; basic host metrics; the `telegram-bot-api` instance's `/stats`.
    - `docs/DESIGN.md` §13.2 — **canaries, end to end**: `getMe` once a minute through our own instance (which exercises own server → MTProto → DC) *and* the same `getMe` straight at cloud `api.telegram.org` as a reference. During an incident it is immediately visible which layer is sick — the diagnosis that was missing last time.
    - `docs/DESIGN.md` §13.2 — **the alert channel must live outside our own instance**: an alert routed through the same bot will not arrive when that instance is the thing that died.
    - `docs/DESIGN.md` §13.5 — Prometheus scrapes the bot's `/metrics` (promhttp) and the bot-api instance's `/stats`.

    ## Depends on

    #19 #20 #22

    ## Scope

    - `promhttp` on its own listener (never the same port as anything public), with a registry the packages register into.
    - Registration of every metric already instrumented by #19, #20, #22 and the pgx pool.
    - The two canaries as scheduled probes, with separate metrics so the two legs are distinguishable, plus the derived signal "own instance sick while cloud is healthy".
    - Runtime metrics (goroutines, GC, memory) via the standard collectors.
    - A documented list of what alerts on what, so the infrastructure pass wires alerting against a fixed contract.

    ## Out of scope

    - Prometheus and Grafana themselves, the scrape config, the dashboards-as-JSON and Grafana provisioning (§13.5) — the infrastructure pass.
    - The out-of-band alert channel (separate alerter bot on cloud API, or mail) — the infrastructure pass. **This issue must name the signals it will consume.**
    - Scraping the bot-api instance's `/stats` — that endpoint belongs to the instance; Prometheus scrapes it directly, no bot code involved.

    ## Telemetry obligation

    This issue is the telemetry surface for health; it adds no gameplay mechanic and moves no balance.

    ## Open questions to close in the spec

    - Cardinality discipline: which labels are allowed (method, task type, outcome) and which are forbidden (chat id, player id).
    - Canary interval and its cost against the transport rate limiters.

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#19", "#20", "#22", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: ac1667e9338683a66
prior_qa:
  - round: 1
    question: "The cloud canary leg calls getMe straight at api.telegram.org. Under which credential? (DESIGN §13.2 asks for the probe; §12.2's runbook keeps the production bot logged out of the cloud; §12.5 already puts a separate test bot there.)"
    answer: "Separate bot - a second, cloud-side bot token in its own optional config key (the §12.5 test bot). Production session untouched, healthy means a real 200 with ok:true, and the leg self-disables when the key is absent."
  - round: 1
    question: "What drives the two canary probes' cadence?"
    answer: "In-process tick - a goroutine on the configured interval, independent of Postgres, so both legs keep reporting while the database is down. Nothing persisted, no new task type, no scheduler coupling."
```
