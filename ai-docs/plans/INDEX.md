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
| [2026-09-09 — health metrics and canaries: `/metrics`, update and scheduler lag, dual `getMe`](2026-09-09-health-metrics-canaries.spec.md) | 🟢 in progress | #23 | #19 · #20 · #22 |

**Statuses:** 🟡 spec only · 🔵 designed · 🟢 in progress · ✅ done (moved to `done/`) · 🔴 blocked · ⏸️ deferred (moved to `deferred/`).

Directory layout:

- `ai-docs/plans/*.spec.md` — active task specs with acceptance criteria
- `ai-docs/plans/*.design.md` — active task design documents
- `ai-docs/plans/*.progress.md` — active task progress / handoff state. Committed while the task runs, then `mv`d to `ai-docs/plans/ignored/` before the PR, so the PR diff stays clean and a finished run leaves nothing for `⚡ First` to match
- `ai-docs/plans/done/` — completed pairs (spec + design), implemented and merged
- `ai-docs/plans/deferred/` — specs written but not scheduled
