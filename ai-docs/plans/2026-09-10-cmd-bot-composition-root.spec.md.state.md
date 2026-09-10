# Interview state — cmd/bot composition root

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-10-cmd-bot-composition-root.spec.md
issue_ref: "#24"
gh_issue:
  title: "cmd/bot composition root: wiring, migration policy, graceful shutdown"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What

    `cmd/bot` is still the scaffold. This issue makes it the real composition root: read config, open the pool, decide and execute the migration policy, start the update loop, the scheduler worker and the metrics listener, and shut all of it down cleanly.

    ## Design refs

    - `docs/DESIGN.md` §11 — the bot is a stateless handler over Postgres; restarts and deploys are trivial. That property is only real if start-up and shutdown are actually clean.
    - `docs/DESIGN.md` §12.1 — the MVP topology the binary has to fit into.
    - `ai-docs/deferred/_inbox.jsonl` — **open question carried forward: production migration policy** — apply at `cmd/bot` start-up versus a separate command. Named as landing "with the bot's database wiring", i.e. here.
    - `ai-docs/context-status.md` — `internal/store` already has `Migrate` and `NewPool` (with the shopspring codec registered once in `AfterConnect`); this issue wires them, it does not reimplement them.
    - `AGENTS.md` § Go Test Conventions — `main` may exit non-zero; libraries return errors. No `panic` outside `main`'s own fatal path.

    ## Depends on

    #18 #19 #20 #22 #23

    ## Scope

    - Start-up sequence with an explicit order and a failure mode per step, each naming what failed.
    - The migration policy decision, implemented and written down.
    - Graceful shutdown on `SIGINT` / `SIGTERM`: stop polling, let in-flight updates and the claimed scheduler batch finish inside a bounded deadline, close the pool.
    - One place where every subsystem is constructed and injected — no package-level singletons, no `init()` wiring.
    - A health/readiness signal distinct from `/metrics` (readiness = migrations applied and the pool answers).
    - A smoke test that the binary starts against a test database and stops on a signal without leaking a goroutine.
    - `go list -deps ./cmd/bot` must keep not linking testcontainers (the existing invariant).

    ## Out of scope

    - Container images, compose topology, systemd units, restart policy, backup wiring (§12.1-12.3) — the infrastructure pass.
    - The testing environment (§12.5) — the infrastructure pass.

    ## Telemetry obligation

    - Metrics: process start timestamp, build version, readiness state.
    - Events: none.

    ## Open questions to close in the spec

    - Migrations at start-up or as a separate command — decide it here, and say what happens when two instances start at once.
    - Shutdown deadline, and what a scheduler task that overruns it looks like on the next start.

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#18", "#19", "#20", "#22", "#23", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 3
agent_id: aa819bf6502b63262
prior_qa:
  - round: 1
    question: "Production migration-apply policy: where do pending migrations get applied? (goose applies no lock unless one is configured, so the concurrent-start answer follows from this choice.)"
    answer: "Start-up, opt-out — Applied at start-up by default, with an environment switch turning it off for an operator who runs the separate command instead. Both paths exist and both need covering."
  - round: 1
    question: "DESIGN 12.1 restart hygiene — shifting overdue scheduler run_at forward by the downtime. Does that land in this task or its own issue? No issue in the #47 decomposition owns it today."
    answer: "why we need to shift run_at?"
  - round: 1
    question: "(orchestrator answered from docs/DESIGN.md §12.1 — the rule protects backpack/corpse TTL and raid-session timer edges from firing in one salvo after a home-machine outage; §11 names the standing timer as the one non-lazy subsystem — then re-put the same question verbatim)"
    answer: "In this task — The composition root records a liveness heartbeat and, at start-up, shifts overdue pending scheduled_task rows forward by the measured downtime. Grows a forward migration and a start-up step."
  - round: 1
    question: "A subsystem that fails to construct or bind at start-up — fatal, or does the bot come up degraded? (The example that already exists: .env.example's placeholder cloud-canary token refuses the whole canary, own leg included.)"
    answer: "All fatal — Any subsystem failing stops the process, naming it. One rule, no degraded modes — a placeholder canary token keeps the bot down until it is fixed."
  - round: 2
    question: "Approve the spec and post the cross-link comment on #24?"
    answer: "show me ACs"
  - round: 2
    question: "Что именно снести раундом 3? (после того, как владелец разобрал AC на две группы: пересказ стоячих правил и предписания «как реализовать»)"
    answer: "A целиком, срезать B (кроме AC46), а еще AC не должны дублировать то, что и так ловится механическими гейтами/линтерами"
```
