# Propagation sync groups

Per-file sync groups behind `AGENTS.md` § *Propagation Rule*. **Editing any file in a group obliges you to apply the corresponding change to its siblings in the SAME PR.** The AXIOM and the generic triggers stay in `AGENTS.md`; this page carries the table, which is consulted only while editing one of these files.

| If you edit... | You MUST also check / update... |
|---|---|
| `.claude/agents/self-review.md` | `.claude/agents/review-findings.md` AND `.claude/skills/project-review/SKILL.md` (Review group) |
| `.claude/agents/review-findings.md` | `.claude/agents/self-review.md` AND `.claude/skills/project-review/SKILL.md` (Review group) |
| `.claude/skills/project-review/SKILL.md` | `.claude/agents/self-review.md` AND `.claude/agents/review-findings.md` (Review group) |
| `.claude/skills/interview/SKILL.md` | `.claude/agents/spec-writer.md` (Interview group) |
| `.claude/agents/spec-writer.md` | `.claude/skills/interview/SKILL.md` (Interview group) |
| `.claude/skills/task/SKILL.md` (Steps 6–8, the design phase) | `.claude/agents/design-writer.md` AND `.claude/agents/design-review.md` AND `.claude/skills/context-reset/SKILL.md` (Task/Design group) |
| `.claude/agents/design-writer.md` OR `.claude/agents/design-review.md` OR `.claude/skills/context-reset/SKILL.md` | See the *Task/Design group* anchor row above |
| `.claude/skills/task/SKILL.md` verify list | `.claude/skills/task/reference.md` § *Step 9 — verify list (full)* — the SKILL names the gates, the reference details them; they must not drift |
| A domain-invariant rule (ledger, telemetry, balance constants, chat safety, determinism) | `ai-docs/domain-invariants.md` AND `.claude/agents/self-review.md` § 4a AND `.claude/agents/review-findings.md` § 1a AND `.claude/agents/design-writer.md` § Rules |
| A gate command (adding, removing, or renaming one) | `AGENTS.md` § *Build & Test* AND every skill's `allowed-tools` line that grants it AND `.claude/skills/task/reference.md` § *Gate checklist* |
| `.claude/skills/reflect/SKILL.md` | `.claude/agents/self-reflect.md` (Reflect group) |
| `.claude/agents/self-reflect.md` | `.claude/skills/reflect/SKILL.md` (Reflect group) |
| `.claude/skills/improve/SKILL.md` | `.claude/agents/self-improve.md` (Improve group) |
| `.claude/agents/self-improve.md` | `.claude/skills/improve/SKILL.md` AND `ai-docs/improve-eval-contract.md` (Improve group) |
| `.claude/skills/triage/SKILL.md` | `.claude/agents/triage-runner.md` AND `.claude/skills/next/SKILL.md` (Triage group) |
| `.claude/agents/triage-runner.md` | `.claude/skills/triage/SKILL.md` AND `.claude/skills/next/SKILL.md` (Triage group) |
| `.claude/skills/next/SKILL.md` | `.claude/skills/triage/SKILL.md` AND `.claude/agents/triage-runner.md` (Triage group) |
| `.claude/skills/ai-audit/SKILL.md` | `.claude/skills/ai-audit/reference.md` AND `checklist-m.md` AND `.claude/agents/learnings-escalation-audit.md` (Audit group) |
| A case added to `.claude/skills/task/scripts/test-append-task-run.sh` | `ai-docs/task-run-schema.md` § *Cases* — the suite's AC6 asserts the two agree |
| `ai-docs/agent-writing-style.md` § Patterns | `.claude/skills/ai-audit/checklist-m.md` — the audit checklist that enforces those patterns |
| `.claude/skills/pr-ci-failed/SKILL.md` | `.claude/skills/main-ci-failed/SKILL.md` AND `.claude/skills/dependabot-pr/reference.md` (CI group — the failure-class taxonomy and the per-class reproducers must agree) |
| `.claude/skills/main-ci-failed/SKILL.md` | See the *CI group* anchor row above |
| `.github/workflows/ci.yml` (a job added, renamed, or removed) | The CI group's class tables AND `AGENTS.md` § *Build & Test* AND `ai-docs/claude-tools-hierarchy.md` — a class with no job, or a job with no class, is how a red run becomes unclassifiable |
| A reviewer spawn template — `.claude/skills/task/SKILL.md` Step 10, `.claude/skills/task/reference.md` (both amendment recipes), `.claude/skills/bugfix/SKILL.md`, `.claude/skills/project-review/SKILL.md` | The closed-list contract in `.claude/agents/self-review.md` AND `.claude/agents/design-review.md` AND the reviewer-spawn-contract hook in `.claude/settings.json` AND `.claude/skills/ai-audit/scripts/test-spawn-contract-guard.sh` (Spawn group — a template the guard refuses is a template no round can use, and a permitted item the guard does not know is a contract the guard silently narrows) |
| `.claude/skills/pr-commented/SKILL.md` | `.claude/skills/pr-ci-failed/SKILL.md` (shared Step-5 self-review + Step-6 push/PR-body contract) |

Groups are added here as their files land. Every group the harness declares is now live.
