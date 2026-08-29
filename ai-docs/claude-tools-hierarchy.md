# Tool / Subagent / Skill / Hook inventory

The live registry of the harness surface in this repository. **`AGENTS.md` § *Propagation Rule* requires updating this page in the same PR as any change to a Tool, Subagent, Skill or Hook contract.** A row that no longer matches reality is a defect, not documentation debt.

The harness is being ported from `graphite-gp` in phases; this page lists what exists **now**, not what is planned.

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

None yet — they land with the spec-driven workflow phase.

## Skills — `.claude/skills/`

None yet — they land with the spec-driven workflow phase. Built-in Claude Code commands (`/code-review`, `/simplify`, `/security-review`, `/init`) are **not** part of this harness and are not governed by this page; when the harness's own review surfaces land, this section records how the two relate.

## Permissions

`allow` covers the project's own toolchain (`go`, `gofmt`, `golangci-lint`, `git`, `gh`, `ast-index`, `psql`, `actionlint`, `shellcheck`) plus read-only text tools. `deny` covers `.idea/**`, `**/.env*` and `**/secrets*` — the bot token and the database DSN must be unreachable to both `Read` and `Edit`.
