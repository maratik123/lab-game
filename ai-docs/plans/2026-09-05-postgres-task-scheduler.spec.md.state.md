# Interview state — postgres task scheduler

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-05-postgres-task-scheduler.spec.md
issue_ref: "#20"
gh_issue:
  title: "Postgres task scheduler: scheduled_task, SKIP LOCKED worker, one-shot and recurrent tasks"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What
    
    The one exception to this project's no-cron rule: a Postgres-backed task scheduler. It is a **load-bearing component** — raid sessions advance through it, so its lag is visible as gameplay (`docs/DESIGN.md` §3.5).
    
    ## Design refs
    
    - `docs/DESIGN.md` §11 — table `scheduled_task` (run_at, type, payload, status); worker with `SELECT … WHERE run_at <= now() FOR UPDATE SKIP LOCKED LIMIT N`. **A task executes in one transaction with its own effects** — exactly-once without two-phase anything. Roughly 100 lines; the pattern is canonical.
    - `docs/DESIGN.md` §11 — it serves: raid-session timer edges (§3.5), the notification queue, one-shot tasks (corpse evaporation, assembly TTL) and recurrent tasks (day close).
    - `docs/DESIGN.md` §11 — **task types are ledger basis documents** — our own model, no mapping onto a foreign one. The starter registry in §11 already names "deferred one-shot" and "recurrent task" as basis-document types.
    - `docs/DESIGN.md` §11 — scaling, when needed and not before: `SKIP LOCKED` is horizontal out of the box; class separation via a priority/queue column with separate worker pools, never a second table. The early signal is fan-out delaying gameplay edges.
    - `docs/DESIGN.md` §11 — River is the ready-made exit if this grows features; gocron rejected (in-memory, one-shot tasks must survive a restart), Temporal is overkill.
    - `docs/DESIGN.md` §13.2 — **scheduler lag (run_at vs actual execution), broken down by task type**, is a health-dashboard star.
    - `AGENTS.md` § Domain Rules — scheduler tasks are idempotent and guard-checked; a stale task firing late is normal operation, not an error.
    
    ## Depends on
    
    _None — this one can start immediately._
    
    ## Scope
    
    - A forward migration for `scheduled_task` plus the two basis-document tables the exclusive arc needs (deferred one-shot, recurrent), with their arc columns and the `CHECK (num_nonnulls(...) = 1)` update.
    - The worker: batch claim with `FOR UPDATE SKIP LOCKED`, one transaction per task carrying its effects, handler registry keyed by task type.
    - Guard semantics: a handler may decide the task is stale and complete it as a no-op. That is a normal outcome, distinguished in metrics from a failure.
    - Failure policy: retry with backoff, a give-up state, and a way to see the dead ones.
    - Recurrent tasks: schedule-next-on-completion, idempotent by construction.
    - Per-type lag instrumentation.
    - Tests against a real Postgres (`internal/testdb`), including a concurrency test under `-race` proving two workers never execute the same task.
    
    ## Out of scope
    
    - The priority / queue column — designed, deferred until fan-out actually delays gameplay edges. **The MVP schema must not make adding it a rewrite.**
    - Task handlers themselves — each ships with its mechanic.
    
    ## Telemetry obligation
    
    - Metrics: scheduler lag by task type, tasks executed / no-op-on-guard / failed by type, claim batch size, worker loop duration.
    
    ## Open questions to close in the spec
    
    - Payload representation: JSONB versus a typed column set per task type.
    - Whether `scheduled_task` rows are deleted or tombstoned after completion, and the retention that follows from the answer.
    
    ---
    
    Part of #47 (MVP roadmap).
    
  comments: []
  linked_issues: ["#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 3
agent_id: a80e8c5c04428a94f
prior_qa:
  - round: 1
    question: "How does a `scheduled_task` row carry its type-specific data? (§11 names `payload` as a column, but not its representation.)"
    answer: "JSONB payload — one `payload jsonb` column; each handler decodes its own shape. A new task type needs no migration."
  - round: 1
    question: "What happens to a `scheduled_task` row once it completes successfully?"
    answer: "Delete on done — the completing transaction deletes the row. The table holds only pending and dead tasks; history lives in the basis documents and postings."
  - round: 1
    question: "Where does a recurrent task's next `run_at` come from?"
    answer: "how kagkarlsson/db-scheduler works with recurrent tasks?"
    note: "Not a choice — the owner asked for the prior art before deciding. Researched facts passed as extra_context in round 2; the question is to be re-asked informed by them."
  - round: 2
    question: "Round 1 settled that a completed task row is deleted. Does that also govern a recurrent occurrence, or does a recurring row persist and move forward?"
    answer: "<pending — surfaced to the owner>"
  - round: 2
    question: "Where is a recurrent task's cadence declared, and what restores a chain that has stopped?"
    answer: "<pending — surfaced to the owner>"
```
