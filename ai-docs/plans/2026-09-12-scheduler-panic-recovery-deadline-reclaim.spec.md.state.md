# Interview state — scheduler panic recovery and deadline reclaim

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md
issue_ref: "#81"
gh_issue:
  title: "Scheduler: survive a panicking handler, and reclaim a handler that breached its deadline"
  state: open
  labels: ["enhancement", "mvp", "area:platform"]
  body: |
    ## What

    Two failure modes of a scheduler handler that the worker does not survive today, both to be closed before the first task type is registered.

    **A panic in a handler takes the whole process down, and keeps taking it down.** The handler runs on its own goroutine — the deadline mechanism needs it there — and nothing on that goroutine recovers a panic. The claim writes nothing to the row before the handler runs, and an attempt is counted only when it settles, so after the restart the same task is due again with its failure count unchanged: one poisoned task keeps the whole bot, update ingestion included, in a crash loop, and the attempt cap never engages. `internal/ingest` already recovers a handler panic and reports it as its own `panic` outcome.

    **A handler that ignores its context past the deadline keeps its row locked and its goroutine alive.** The delivered layers reclaim the row only for a handler that honours the `ctx` it is handed. For one that does not, the row stays locked until the handler returns, and a watchdog goroutine waits on it holding the hijacked connection. For a recurrent task — whose chain never stops, by the owner's decision in the scheduler spec — each cadence period strands one more goroutine and one more connection, for ever.

    ## Decided by the owner (2026-09-11, while splitting #80)

    - **Recover at the handler boundary.** The goroutine that runs a handler recovers a panic, records its stack, and takes the same failure path a returned error takes: counted as an attempt, subject to the same failure policy, and distinguishable from a returned error in what the scheduler reports. The worker's own loops are not wrapped. This is the scheduler's application of the general rule #80 writes down.
    - **Terminate the breached task's backend.** After a deadline breach the scheduler terminates the task's own database backend from another pooled connection (`pg_terminate_backend`) — the layer the scheduler design proposed and measured: an ordinary role may terminate a backend of its own, and the hijacked connection exposes its PID. The row becomes claimable at once, and a handler blocked in the database gets an error and returns, which releases its goroutine and its connection.
    - **A handler blocked outside the database stays a contract breach.** No mechanism reclaims that goroutine. It is the explicit exception to #80's ownership rule, and the `Handler` contract — honour your `ctx` — covers it.

    ## Why now

    The registry is wired empty today, so neither mode can fire yet. The first task type arrives with #36, #40, #43 or #45, whichever is taken first — and #45's day close is a recurrent task, the exact shape of the second mode. This issue lands before any of them.

    ## What this reopens

    - The scheduler spec's deferred row *"Distinguishing a handler panic from a returned error in the failure policy … a handler panic is a defect, not a state the scheduler models"* (`ai-docs/plans/done/2026-09-05-postgres-task-scheduler.spec.md` § Deferred) — reversed by the first decision above.
    - The scheduler design's two open questions — the permanently hanging recurrent handler, and the `pg_terminate_backend` layer that would make row reclamation independent of the handler's cooperation (`ai-docs/plans/done/2026-09-05-postgres-task-scheduler.design.md` § Open questions) — answered by the second.

    Their rows in `ai-docs/deferred/_inbox.jsonl` are `/triage`'s to update.

    ## Scope

    - A panicking handler terminates neither the process nor the worker: its attempt is counted under the failure policy, its stack is recorded, and the panic is distinguishable from a returned error in what the scheduler reports.
    - After a deadline breach, the breached task's row is claimable again without waiting for its handler to return.
    - A handler blocked in the database when its deadline is breached returns, and its goroutine and its connection are released.

    ## Out of scope

    - **A handler blocked outside the database** — the `Handler` contract, excepted explicitly in #80.
    - **Capping a recurrence** — the owner's decision that the chain never stops stands.
    - **Update ingestion** — it already recovers a handler panic.
    - **The general handler-boundary rule** — #80 writes it down; this issue applies it to the scheduler.

    ## Depends on

    #74 — it touches the scheduler's tests and lands first.

    ## Blocks

    #36 #40 #43 #45 — every issue that registers the first scheduler task type.

    ## Telemetry obligation

    - Metrics: the scheduler's failure classification gains a value for a panic — a new value of the `failure` label that `ai-docs/alert-contract.md` § Scheduler catalogues for `labgame_scheduler_tasks_total`, not a new family.
    - Events: none — the event dictionary holds gameplay events only.

    ## Open questions to close in the spec

    - **Where the stack goes.** The scheduler takes no logger today, and `internal/ingest`'s recovery keeps the panic value but not its stack — whether the stack lands in the task row's `last_error`, on a logger the composition root passes in, or both; and whether ingest's recovery follows.

  comments: []
  linked_issues: ["#80", "#74", "#36", "#40", "#43", "#45"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 3
agent_id: a1dad3c5a4aed674f
prior_qa:
  - round: 1
    question: "Where does a scheduler handler's panic stack have to be observable?"
    answer: "Both — The row carries the stack and the log carries it — observable without log access, and searchable in the log stream."
  - round: 1
    question: "Does `internal/ingest`'s panic recovery also gain the stack, or does this task change the scheduler only?"
    answer: "Ingest too — Ingest's recovery records the stack the same way, in this task — one rule, both handler boundaries."
  - round: 3
    question: "AC6/AC9 требуют стек на обеих поверхностях («и строка, и лог»), но дизайн показал два случая, где строки физически нет: паника после срыва дедлайна (строка уже занята причиной «deadline exceeded») и паника в ingest, которую переживёт следующая попытка (строка пишется только при исчерпании всех попыток). Что делаем?"
    answer: "Amend the spec — spec-writer переформулирует AC6/AC9 как «строка везде, где попытка её оставляет, и лог всегда». Даёт выполнимый AC вместо невыполнимого. Цена: раунд spec-writer + полный цикл дизайн→ревью на изменённой паре (в него же уедут замечания 1/2/4)."
```
