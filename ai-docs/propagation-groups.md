# Propagation sync groups

Per-file sync groups behind `AGENTS.md` § *Propagation Rule*. **Editing any file in a group obliges you to apply the corresponding change to its siblings in the SAME PR.** The AXIOM and the generic triggers stay in `AGENTS.md`; this page carries the table, which is consulted only while editing one of these files.

| If you edit... | You MUST also check / update... |
|---|---|
| `.claude/agents/self-review.md` | `.claude/agents/review-findings.md` AND `.claude/skills/project-review/SKILL.md` (Review group) |
| `.claude/agents/review-findings.md` | `.claude/agents/self-review.md` AND `.claude/skills/project-review/SKILL.md` (Review group) |
| `.claude/skills/project-review/SKILL.md` | `.claude/agents/self-review.md` AND `.claude/agents/review-findings.md` (Review group) |
| `.claude/skills/interview/SKILL.md` | `.claude/agents/spec-writer.md` (Interview group) |
| `.claude/agents/spec-writer.md` | `.claude/skills/interview/SKILL.md` (Interview group) |
| `.claude/skills/task/SKILL.md` (Steps 6–8, the design phase) | `.claude/agents/design.md` AND `.claude/agents/design-review.md` AND `.claude/skills/context-reset/SKILL.md` (Task/Design group) |
| `.claude/agents/design.md` OR `.claude/agents/design-review.md` OR `.claude/skills/context-reset/SKILL.md` | See the *Task/Design group* anchor row above |
| `.claude/skills/task/SKILL.md` verify list | `.claude/skills/task/reference.md` § *Step 9 — verify list (full)* — the SKILL names the gates, the reference details them; they must not drift |
| A domain-invariant rule (ledger, telemetry, balance constants, chat safety, determinism) | `ai-docs/domain-invariants.md` AND `.claude/agents/self-review.md` § 4a AND `.claude/agents/review-findings.md` § 1a AND `.claude/agents/design.md` § Rules |
| A gate command (adding, removing, or renaming one) | `AGENTS.md` § *Build & Test* AND every skill's `allowed-tools` line that grants it AND `.claude/skills/task/reference.md` § *Gate checklist* |

Groups are added here as their files land. The learning-loop group (`/improve` + `self-improve` + `learnings-escalation-audit`) and the CI group (`/pr-ci-failed`, `/main-ci-failed`, `/dependabot-pr`) arrive with those skills.
