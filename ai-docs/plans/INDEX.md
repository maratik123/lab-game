# Plans index

Every spec/design pair, with its status and dependency order. Maintained by `/task` (Step 12 moves a completed pair into `done/` and updates the row here) and by `/interview` when a spec is written standalone.

| Plan | Status | Tracked in | Depends on |
|---|---|---|---|
| [2026-08-30 — mechanical code-style gates](done/2026-08-30-mechanical-code-style-gates.spec.md) | ✅ implemented (0 Go tests; gates verified by fixture) | none (PR #6) | — |
| [2026-09-02 — ledger core: `store.Post`](done/2026-09-02-ledger-post-core.spec.md) | ✅ implemented (33 tests) | none (owner's decision) | — |

**Statuses:** 🟡 spec only · 🔵 designed · 🟢 in progress · ✅ done (moved to `done/`) · 🔴 blocked · ⏸️ deferred (moved to `deferred/`).

Directory layout:

- `ai-docs/plans/*.spec.md` — active task specs with acceptance criteria
- `ai-docs/plans/*.design.md` — active task design documents
- `ai-docs/plans/*.progress.md` — active task progress / handoff state (**gitignored**, local-only)
- `ai-docs/plans/done/` — completed pairs (spec + design), implemented and merged
- `ai-docs/plans/deferred/` — specs written but not scheduled
