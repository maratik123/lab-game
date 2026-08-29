# Tool / Subagent / Skill / Hook inventory

The live registry of the harness surface in this repository. **`AGENTS.md` § *Propagation Rule* requires updating this page in the same PR as any change to a Tool, Subagent, Skill or Hook contract.** A row that no longer matches reality is a defect, not documentation debt.

The harness is being ported from `graphite-gp` in phases; this page lists what exists **now**, not what is planned.

Rules files, hooks and permissions landed in phase 1; the subagents and the spec-driven skills in phase 2.

## Hooks — `.claude/settings.json`

| Event | Matcher | Hook | Contract |
|---|---|---|---|
| `SessionStart` | — | ast-index sync | Updates the index, or rebuilds it when absent. Never fails the session (`\|\| true`). |
| `SessionStart` | — | rules reminder | Injects the instruction to read `CLAUDE.md` (→ `AGENTS.md`) and summarise the rules. |
| `PreToolUse` | `Bash` | main-branch commit guard | **Blocks (exit 2)** any `git commit` while `git branch --show-current` is `main`, printing the recovery procedure. The only enforcement of AXIOM 1 — `origin` has none. |
| `PreToolUse` | `Bash` | ast-index refresh | Updates the index before a commit. Advisory, never blocks. |
| `PreToolUse` | `Bash` | piped-gate guard | **Blocks (exit 2)** `go build/test/vet` / `golangci-lint run` / `gofmt` piped into `tail`/`head` without `set -o pipefail`. Matches command *text*, so a quoted example also matches — fails closed by design. |
| `PostToolUse` | `Write\|Edit` | gofmt | Formats a written `.go` file in place. |
| `PostToolUse` | `Write\|Edit` | panic-gate | **Warns** on `panic(` / `log.Fatal*` / `log.Panic*` in a non-`_test.go` file; points at `ai-docs/panic-index.md`. |
| `PostToolUse` | `Write\|Edit` | secret-gate | **Blocks (exit 2)** when a written file contains a string shaped like a live Telegram bot token (`\d{8,10}:[A-Za-z0-9_-]{35}`). |
| `PostToolUse` | `Bash` | PR-body sync | After a `git push` on a branch with an open PR, reminds to re-read the PR body (AXIOM 2). Advisory. |

Verification protocol for any hook change: [`hook-verification.md`](hook-verification.md). All nine bodies pass `shellcheck -s bash`; the blocking ones were exercised against real commands (including innocent ones containing the matched substring) on 2026-08-29.

## Rules — `.claude/rules/`

| File | Loaded when | Contract |
|---|---|---|
| [`ast-index.md`](../.claude/rules/ast-index.md) | Any code-search task | ast-index first; grep only for regex / string literals / comments / non-Go files. Negative results are not evidence. |

## Subagents — `.claude/agents/`

| Agent | Model | Spawned by | Contract |
|---|---|---|---|
| `spec-writer` | opus | `/interview`, `/task` Steps 1–5 | Drafts the spec one interview round at a time; asks 0–3 questions per round or returns `ready` / `unresolvable`. Never implements. |
| `design` | opus | `/task` Step 6 | Produces the design document with decomposition and a `## Handoff plan`. Reads the binding-constraint file for anything it specifies. Writes no code. |
| `design-review` | opus | `/task` Step 7 | Reviews a design against the checklist, issues GO / ITERATE / STOP. Loops with `design`. |
| `code-writer` | sonnet, effort medium (pinned in frontmatter) | `/context-reset` group handoff | Implements a group's subtasks sequentially, gates and commits per subtask. **Never** pushes, opens a PR, runs self-review, or spawns anything. |
| `self-review` | inherited | `/task` Step 10, `/bugfix` Step 6, `/project-review` | Reviews the implementation diff against spec and design; APPROVE / REJECT. The push gate. |
| `review-findings` | inherited | `/project-review` | Walks the whole codebase (no diff, no spec) and writes a findings table into the progress file. |

Not ported from the source harness: `image-check` (verifies a golden *image* against its drawing code — this project's goldens are text, and `code-writer` reads them itself), `self-reflect`, `self-improve`, `learnings-escalation-audit`, `triage-runner` (they arrive with the learning-loop phase).

## Skills — `.claude/skills/`

| Skill | Invocation | Contract |
|---|---|---|
| `/task` | explicit | The full workflow: interview → spec → design → design-review → implementation → verify → self-review → PR. Steps are strictly ordered; 6 / 7 / 10 cannot be skipped. Pre-authorises its own commits, push and `gh pr create`. |
| `/interview` | explicit or via `/task` | Drives the spec-drafting rounds through `spec-writer`; never drafts the spec itself. |
| `/context-reset` | at every design-defined group boundary, and on compaction recovery | The handoff protocol: spawns the group's implementor and re-validates state on return. |
| `/project-review` | explicit | Whole-codebase review on the current branch: `review-findings` → fix loop → `self-review` until APPROVE, then commits. |
| `/bugfix` | model-invocable on failure signals | Trace → root cause → failing test → fix. Analysis before code; the test is written before the fix. |
| `/next` | explicit | Recommends one task to work on next, with runner-ups. |
| `/verify-change` | explicit | Runs `go test ./...`, optionally filtered. |

Built-in Claude Code commands (`/code-review`, `/simplify`, `/security-review`, `/init`) are **not** part of this harness and are not governed by this page. They overlap `self-review` / `project-review` in purpose but not in contract: the harness surfaces review against *this* project's spec, design and domain invariants, and gate the push; the built-ins review a diff on general principles and gate nothing. Use the harness surfaces inside a `/task` flow; the built-ins are fine ad hoc.

The learning-loop skills (`/improve`, `/reflect`, `/ai-audit`, `/triage`) and the CI/PR skills (`/pr-commented`, `/pr-ci-failed`, `/main-ci-failed`, `/pr-merged`, `/dependabot-pr`) are not yet ported.

## Permissions

`allow` covers the project's own toolchain (`go`, `gofmt`, `golangci-lint`, `git`, `gh`, `ast-index`, `psql`, `actionlint`, `shellcheck`) plus read-only text tools. `deny` covers `.idea/**`, `**/.env*` and `**/secrets*` — the bot token and the database DSN must be unreachable to both `Read` and `Edit`.
