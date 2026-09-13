# Plans index

Every spec/design pair, with its status and dependency order. Maintained by `/task` (Step 12 moves a completed pair into `done/` and updates the row here) and by `/interview` when a spec is written standalone.

| Plan | Status | Tracked in | Depends on |
|---|---|---|---|
| [2026-08-30 — mechanical code-style gates](done/2026-08-30-mechanical-code-style-gates.spec.md) | ✅ implemented (0 Go tests; gates verified by fixture) | none (PR #6) | — |
| [2026-09-02 — ledger core: `store.Post`](done/2026-09-02-ledger-post-core.spec.md) | ✅ implemented | none (owner's decision) | — |
| [2026-09-03 — rename the `design` Subagent](done/2026-09-03-rename-design-subagent.spec.md) | ✅ implemented | #10 | — |
| [2026-09-04 — configuration layer: runtime settings and the balance/world constant files](done/2026-09-04-config-layer-balance-files.spec.md) | ✅ implemented | #18 | — |
| [2026-09-04 — Bot API transport over telego: retries, `retry_after`, rate limits, base-URL axis](done/2026-09-04-bot-api-transport.spec.md) | ✅ implemented | #19 | #18 |
| [2026-09-05 — Postgres task scheduler: `scheduled_task`, the `SKIP LOCKED` worker, one-shot and recurrent tasks](done/2026-09-05-postgres-task-scheduler.spec.md) | ✅ implemented | #20 | #18 · the ledger core (2026-09-02) |
| [2026-09-05 — event log: the `event` table, the §13.4 dictionary, and the MVP SQL views](done/2026-09-05-event-log-dictionary-views.spec.md) | ✅ implemented | #21 | #18 · the ledger core (2026-09-02) |
| [2026-09-06 — update ingestion: long polling, dispatch, operation idempotency, chat allowlist](done/2026-09-06-update-ingestion-dispatch-idempotency.spec.md) | ✅ implemented | #22 | #18 · #19 · the ledger core (2026-09-02) |
| [2026-09-08 — comment reference ban: doc comments stay, outward references go](done/2026-09-08-comment-reference-ban.spec.md) | ✅ implemented | #68 | — |
| [2026-09-08 — shared PostgreSQL test server: one server per run, containers as the fallback](done/2026-09-08-shared-postgres-test-server.spec.md) | ✅ implemented | #67 | the ledger core (2026-09-02) |
| [2026-09-09 — health metrics and canaries: `/metrics`, update and scheduler lag, dual `getMe`](done/2026-09-09-health-metrics-canaries.spec.md) | ✅ implemented | #23 | #19 · #20 · #22 |
| [2026-09-10 — `cmd/bot` composition root: wiring, migration policy, graceful shutdown](done/2026-09-10-cmd-bot-composition-root.spec.md) | ✅ implemented | #24 | #18 · #19 · #20 · #22 · #23 |
| [2026-09-11 — goroutine-leak detection: every test binary ends with a goleak check](done/2026-09-11-goroutine-leak-detection-goleak.spec.md) | ✅ implemented | #74 | #24 · #67 |
| [2026-09-12 — canary limiter refusal label: an expired deadline reports `timeout`](done/2026-09-12-canary-limiter-refusal-label.spec.md) | ✅ implemented | #89 | #19 · #23 |
| [2026-09-12 — test database container named per checkout](done/2026-09-12-test-container-name-per-checkout.spec.md) | ✅ implemented | #91 | #67 |
| [2026-09-12 — split the CI Test job: each test gate gets its own concurrently-scheduled job](done/2026-09-12-split-ci-test-job.spec.md) | ✅ implemented | #99 | #67 |
| [2026-09-12 — goroutine-leak prevention: ownership rules and the gates that hold them](done/2026-09-12-goroutine-ownership-rules-gates.spec.md) | ✅ implemented | #80 | #74 · #24 |
| [2026-09-12 — scheduler handler panics and deadline reclaim: recover at the boundary, terminate the breached backend](done/2026-09-12-scheduler-panic-recovery-deadline-reclaim.spec.md) | ✅ implemented | #81 | #80 · #74 · #20 |
| [2026-09-12 — item machine: `item`, `item_movement`, and the shared holder address space](done/2026-09-12-item-machine-holder-address-space.spec.md) | ✅ implemented | #25 | the ledger core (2026-09-02) |
| [2026-09-12 — world generation core: hex topology and the deterministic chunk generator](done/2026-09-12-world-generation-hex-chunk-generator.spec.md) | ✅ implemented | #27 | #18 |

**Statuses:** 🟡 spec only · 🔵 designed · 🟢 in progress · ✅ done (moved to `done/`) · 🔴 blocked · ⏸️ deferred (moved to `deferred/`).

Directory layout:

- `ai-docs/plans/*.spec.md` — active task specs with acceptance criteria
- `ai-docs/plans/*.design.md` — active task design documents
- `ai-docs/plans/*.progress.md` — active task progress / handoff state. Committed while the task runs, then `mv`d to `ai-docs/plans/ignored/` before the PR, so the PR diff stays clean and a finished run leaves nothing for `⚡ First` to match
- `ai-docs/plans/done/` — completed pairs (spec + design), implemented and merged
- `ai-docs/plans/deferred/` — specs written but not scheduled
