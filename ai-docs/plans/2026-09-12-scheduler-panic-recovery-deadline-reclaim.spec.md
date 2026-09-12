# Scheduler: survive a panicking handler, and reclaim a handler that breached its deadline

**Source:** issue #81
**Date:** 2026-09-12
**Tracked in:** #81

## Scope
1. A panicking scheduler handler ends neither the process nor the worker; the worker goes on claiming and running tasks. [task: "A panicking handler terminates neither the process nor the worker"]
2. A handler panic counts as an attempt under the same failure policy a returned error takes, so the attempt cap engages on a task whose handler panics every time. [task: "counted as an attempt, subject to the same failure policy"]
3. The panic's stack is recorded. [task: "its stack is recorded"]
4. A handler panic is distinguishable from a returned error in what the scheduler reports. [task: "the panic is distinguishable from a returned error in what the scheduler reports"]
5. After a deadline breach, the breached task's row is claimable again without waiting for its handler to return. [task: "After a deadline breach, the breached task's row is claimable again without waiting for its handler to return"]
6. A handler blocked in the database when its deadline is breached returns, and its goroutine and its connection are released. [task: "A handler blocked in the database when its deadline is breached returns, and its goroutine and its connection are released"]
7. The scheduler's failure classification carries a value for a panic, as a value of the label it already has rather than as a new metric family, and every live site whose claim that addition falsifies states it. [task: "a new value of the `failure` label that `ai-docs/alert-contract.md` § Scheduler catalogues for `labgame_scheduler_tasks_total`, not a new family"]

## Out of scope
- Reclaiming a handler blocked outside the database after its deadline is breached — no mechanism does, and the `Handler` contract is what covers it; #80 excepted it explicitly. [task: "A handler blocked outside the database stays a contract breach"]
- Bounding how many times a recurrence repeats — the chain never stops. [task: "Capping a recurrence — the owner's decision that the chain never stops stands."]
- A change to how update ingestion reports a handler panic — it already recovers one. Whether ingest's recovery additionally records the *stack* is a round-1 question and is not settled by this row. [task: "Update ingestion — it already recovers a handler panic."]
- Writing down the general handler-boundary rule for goroutines that run a supplied handler — #80 owns that outcome; this task is its application to the scheduler. [task: "The general handler-boundary rule — #80 writes it down; this issue applies it to the scheduler."]
- Updating the `ai-docs/deferred/_inbox.jsonl` rows this issue reopens — `/triage` owns them. [task: "Their rows in `ai-docs/deferred/_inbox.jsonl` are `/triage`'s to update."]

## Deferred
- (none)

## Key decisions
| Question | Decision |
|---|---|
| Is a handler panic a state the scheduler models, or a defect it leaves outside the failure policy? | Modelled. The scheduler spec's deferral of the panic-versus-returned-error distinction is reversed. [task: "reversed by the first decision above"] |
| Where is a handler panic recovered? | On the goroutine that runs the handler, at the handler boundary. The worker's own loops are not wrapped. [task: "The goroutine that runs a handler recovers a panic, records its stack, and takes the same failure path a returned error takes"] [task: "The worker's own loops are not wrapped"] |
| How does a breached task's row become claimable without the handler's cooperation? | The scheduler terminates the task's own database backend from another pooled connection. [task: "After a deadline breach the scheduler terminates the task's own database backend from another pooled connection (`pg_terminate_backend`)"] |
| Where does a handler panic's stack land? | TBD |
| Does update ingestion's panic recovery also record the stack? | TBD |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | A scheduler handler that panics ends neither the worker nor the process: after the panic the worker goes on claiming and running due tasks. [task: "A panicking handler terminates neither the process nor the worker"] |
| AC2 | A handler panic settles its task the way a returned error does — it counts as one attempt against the same failure policy — so a task whose handler panics on every attempt reaches the attempt cap instead of staying due unchanged. [task: "counted as an attempt, subject to the same failure policy"] |
| AC3 | What the scheduler reports for a handler panic tells it apart from a returned error. [task: "the panic is distinguishable from a returned error in what the scheduler reports"] |
| AC4 | `labgame_scheduler_tasks_total` carries a value of its existing `failure` label for a handler panic, distinct from every value that label already takes, and no metric family is added for it. [task: "a new value of the `failure` label that `ai-docs/alert-contract.md` § Scheduler catalogues for `labgame_scheduler_tasks_total`, not a new family"] |
| AC5 | Every live site whose claim the new `failure` value falsifies states it — the enumeration of that label's values under `ai-docs/alert-contract.md` § Scheduler is one such site, and it does not bound the class — per AGENTS.md § *Propagation Rule* step 4. [task: "a new value of the `failure` label that `ai-docs/alert-contract.md` § Scheduler catalogues for `labgame_scheduler_tasks_total`, not a new family"] |
| AC6 | TBD — the surface on which a handler panic's stack is recorded. |
| AC7 | After a deadline breach, another worker can claim the breached task's row without waiting for its handler to return. [task: "After a deadline breach, the breached task's row is claimable again without waiting for its handler to return"] |
| AC8 | A handler that is blocked in the database when its deadline is breached returns, and both its goroutine and its database connection are released. [task: "A handler blocked in the database when its deadline is breached returns, and its goroutine and its connection are released"] |

## Open questions
- Where a handler panic's stack is recorded — the task row's `last_error`, a logger the composition root passes in, or both. Asked in round 1; AC6 and its Key decisions row stay TBD until the answer lands.
- Whether update ingestion's own panic recovery also records the stack. Asked in round 1; it decides the third `## Out of scope` row.
