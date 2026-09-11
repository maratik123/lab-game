# Agent docs — index

The full map of `ai-docs/**` and the harness's supporting pages. `AGENTS.md` § *Agent Docs* keeps only the handful read on nearly every task; everything else lives here, one click away.

| Path | Purpose |
|------|---------|
| [`ai-docs/context.md`](context.md) | Project context (orientation) — read on demand |
| [`ai-docs/context-status.md`](context-status.md) | Per-task implementation log — read on demand |
| [`ai-docs/plans/INDEX.md`](plans/INDEX.md) | Plan index — statuses and dependency order |
| `plans/*.spec.md` / `*.design.md` | Active task spec + design, committed from creation (`/interview` and Step 6); `*.progress.md` and `*.spec.md.state.md` are committed too and retired to `plans/ignored/` before the PR |
| `ai-docs/plans/done/` | Completed plans (spec + design, implemented) |
| `ai-docs/scripts/` | Shared shell guards: hook regression suites, CI gates over specs and registers, the document-edit guard, the script-shape checker — every script whose caller is not a single skill (`claude-tools-hierarchy.md` § Shell guards has the placement rule and one row per script) |
| [`ai-docs/deferred/_inbox.jsonl`](deferred/_inbox.jsonl) | Triage queue — rows from completed specs |
| [`ai-docs/templates/progress-format.md`](templates/progress-format.md) | Canonical `.progress.md` format |
| [`ai-docs/templates/inbox-row.md`](templates/inbox-row.md) | Canonical `_inbox.jsonl` row shape |
| [`ai-docs/domain-invariants.md`](domain-invariants.md) | Ledger, telemetry, scheduler and Telegram-safety invariants — read before touching those paths |
| [`ai-docs/key-decisions.md`](key-decisions.md) | Key design decisions with rationale |
| [`ai-docs/alert-contract.md`](alert-contract.md) | The health surface's metric catalogue and the alert shapes over it — names and shapes for the infrastructure pass, which sets every number |
| [`ai-docs/process-lifecycle.md`](process-lifecycle.md) | The bot process's lifecycle: the start-up order with a failure mode per step, the migration-apply policy, restart hygiene, readiness, the drain and the exit codes |
| [`ai-docs/code-style.md`](code-style.md) | Go code-style reference — read on demand |
| [`ai-docs/go-api-naming.md`](go-api-naming.md) | Naming rules incl. the `…Unchecked` contract |
| [`ai-docs/doc-convention.md`](doc-convention.md) | godoc conventions — read on demand |
| [`ai-docs/go-test-conventions.md`](go-test-conventions.md) | Table tests, `-race`, golden logs, Postgres fixtures |
| [`ai-docs/dependency-versions.md`](dependency-versions.md) | Live-lookup recipes for all six AXIOM categories |
| [`ai-docs/delegation-rules.md`](delegation-rules.md) | The four-phase delegation lifecycle — read before any committing/long-running spawn |
| [`ai-docs/hook-verification.md`](hook-verification.md) | The three MUSTs for proving a `settings.json` hook fires |
| [`ai-docs/agent-writing-style.md`](agent-writing-style.md) | Binary-rule writing style for dual-model readability |
| [`ai-docs/claude-tools-hierarchy.md`](claude-tools-hierarchy.md) | Project Tool/Subagent/Skill/Hook inventory |
| [`ai-docs/propagation-groups.md`](propagation-groups.md) | Per-file sync groups for the Propagation Rule |
| [`ai-docs/corrections-log.md`](corrections-log.md) | Learning-Log carve-outs + field glossary |
| [`ai-docs/improve-eval-contract.md`](improve-eval-contract.md) | Why `/improve`'s eval dispatch is the parent's, and the forbidden degraded paths |
| [`ai-docs/instruction-file-validation.md`](instruction-file-validation.md) | Dual-model instruction-clarity test methodology |
| [`ai-docs/task-run-schema.md`](task-run-schema.md) | `task-runs.jsonl` schema, operating rules and test-case registry |
| [`ai-docs/panic-index.md`](panic-index.md) | Every panicking call in production code, with its justification |
| [`ai-docs/templates/learnings-entry.md`](templates/learnings-entry.md) | Canonical `learnings.md` entry skeleton — consult instead of the live log |
| [`ai-docs/learnings.md`](learnings.md) | Corrections log — feed for `/improve` |
| [`ai-docs/harness-gaps.md`](harness-gaps.md) | Harness diagnoses — the second learning log, addressed to `/improve` (see AGENTS.md § *Learning Log*) |
| [`ai-docs/harness-restart-metrics.md`](harness-restart-metrics.md) | Restart/recovery measurements behind the harness's flow decisions |

**Reading order for a newcomer to this repo:** `context.md` → `docs/DESIGN.md` §0–§3 → `domain-invariants.md` → `key-decisions.md`. Everything else is read when the task touches it.
