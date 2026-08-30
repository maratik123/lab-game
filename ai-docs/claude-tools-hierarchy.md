# Tool / Subagent / Skill / Hook inventory

The live registry of the harness surface in this repository. **`AGENTS.md` § *Propagation Rule* requires updating this page in the same PR as any change to a Tool, Subagent, Skill or Hook contract.** A row that no longer matches reality is a defect, not documentation debt.

The harness is being ported from `graphite-gp` in phases; this page lists what exists **now**, not what is planned.

Rules files, hooks and permissions landed in phase 1; the subagents and spec-driven skills in phase 2; the learning loop in phase 3; CI and the PR skills in phase 4.

## Hooks — `.claude/settings.json`

| Event | Matcher | Hook | Contract |
|---|---|---|---|
| `SessionStart` | — | ast-index sync | Updates the index, or rebuilds it when absent. Never fails the session (`\|\| true`). |
| `SessionStart` | — | rules reminder | Injects the instruction to read `CLAUDE.md` (→ `AGENTS.md`) and summarise the rules. |
| `PreToolUse` | `Bash` | main-branch commit guard | **Blocks (exit 2)** any `git commit` while `git branch --show-current` is `main`, printing the recovery procedure. The only enforcement of AXIOM 1 — `origin` has none. |
| `PreToolUse` | `Bash` | ast-index refresh | Updates the index before a commit. Advisory, never blocks. |
| `PreToolUse` | `Bash` | piped-gate guard | **Blocks (exit 2)** `go build/test/vet` / `golangci-lint run` / `golangci-lint fmt` / `gofmt` / `make` (matched as a class — every target of the `Makefile` is a gate) piped into `tail`/`head` without `set -o pipefail`. Matches command *text*, so a quoted example also matches — fails closed by design, and `make -n verify` piped into `head` is a known, accepted false positive. Regression suite: `ai-audit/scripts/test-piped-gate-guard.sh`. |
| `PostToolUse` | `Write\|Edit` | `golangci-lint fmt` | Formats a written `.go` file in place, path-scoped to that file — so it applies every formatter `.golangci.yml` enables (`gofumpt` included) and rewrites no sibling. Keeps the hook from re-introducing what the format gate rejects. |
| `PostToolUse` | `Write\|Edit` | panic-gate | **Warns** on `panic(` / `log.Fatal*` / `log.Panic*` in a non-`_test.go` file; points at `ai-docs/panic-index.md`. |
| `PostToolUse` | `Write\|Edit` | secret-gate | **Blocks (exit 2)** when a written file contains a string shaped like a live Telegram bot token (`\d{8,10}:[A-Za-z0-9_-]{35}`). |
| `PostToolUse` | `Bash` | PR-body sync | After a `git push` on a branch with an open PR, reminds to re-read the PR body (AXIOM 2). Advisory. |

Verification protocol for any hook change: [`hook-verification.md`](hook-verification.md). All **ten** bodies pass `shellcheck -s bash` — ten, not the nine this line claimed until 2026-08-30: `jq -r '.hooks[][].hooks[].command' .claude/settings.json` yields `SessionStart` 2 · `PreToolUse` 3 · `PostToolUse` 4 · `Stop` 1. The blocking ones were exercised against real commands, including innocent ones containing the matched substring. The two bodies changed on **2026-08-30** — the `Write\|Edit` formatter and the piped-gate guard — were re-verified against all three MUSTs on that date; the guard's MUST 2 is the 26-fixture matrix its regression suite now runs in CI, and MUST 3 confirmed in-session that an edited `.claude/settings.json` is live and the load-bearing `tool_input` field populated.

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

| `self-improve` | opus | `/improve` | Reads `learnings.md` for repeating patterns, proposes instruction diffs, escalates to hooks at ≥3 occurrences. Re-verifies every factual claim it carries out of an entry. Writes no code. |
| `self-reflect` | opus | `/reflect` | End-of-work retrospective: a structured good/bad list, each finding routed {learnings \| ticket \| none}. Assembles and yields; the parent performs every write. |
| `learnings-escalation-audit` | opus | `/ai-audit` Phase 1 | Verifies every entry's `Escalated?` and `Superseded by:` still point at something real; fixes drift **only** in those two fields. |
| `triage-runner` | opus | `/triage` | Promotes untracked `_inbox.jsonl` rows to issues, drains the queue, reconciles JSONL ↔ issue divergence. Mutation scope is `ai-docs/deferred/**` + `gh issue` only. |

Not ported from the source harness: `image-check` (verifies a golden *image* against its drawing code — this project's goldens are text, and `code-writer` reads them itself).

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
| `/improve` | explicit | Launches `self-improve`; the escalation path from the corrections log into instruction files and hooks. Run at ≥3 unescalated corrections or ≥2 validations. |
| `/reflect` | explicit | Launches `self-reflect`, then applies each finding per the user's per-finding routing consent. |
| `/ai-audit` | explicit | Two phases: (1) `learnings-escalation-audit` fixes field drift; (2) the main session audits the whole instruction surface for dead references, format violations and size-cap breaches. Ships three shell guards — `check-citations.sh`, its regression test, and `test-piped-gate-guard.sh`, the piped-gate hook's regression suite. |
| `/triage` | explicit | Launches `triage-runner`; batched promotion of deferred rows to issues. Default threshold ≥3 unhandled rows. |
| `/pr-commented` | explicit | One round of reviewer-comment response: classify each unresolved thread, bundle fixes into one commit, self-review, push, reply and resolve per category. |
| `/pr-ci-failed` | model-invocable | One round of CI-failure response on the current PR: classify, reproduce locally, fix, self-review, push, re-read the PR body. |
| `/main-ci-failed` | model-invocable | Same, for a red run on `main` — the fix lands on a NEW branch and a new PR; `main` is never modified directly. |
| `/pr-merged` | explicit | After a merge: switch to `main`, pull, delete the merged branch's local progress files, delete the local branch. |
| `/dependabot-pr` | explicit | One round of triage on a Dependabot **gomod** PR. Never auto-merges, never pushes to the bot branch; prints the merge command and pauses. |

Built-in Claude Code commands (`/code-review`, `/simplify`, `/security-review`, `/init`) are **not** part of this harness and are not governed by this page. They overlap `self-review` / `project-review` in purpose but not in contract: the harness surfaces review against *this* project's spec, design and domain invariants, and gate the push; the built-ins review a diff on general principles and gate nothing. Use the harness surfaces inside a `/task` flow; the built-ins are fine ad hoc.

The port is complete: every subagent and skill the source harness carried, minus the ones whose subject this project does not have (`image-check`, the design-system skill).

## Shell guards — `.claude/skills/**/scripts/`

| Script | Run by | Contract |
|---|---|---|
| `ai-audit/scripts/check-citations.sh` | `/ai-audit` Phase 2, or standalone | Every `#N` / learnings-date / memory-file citation must resolve **for its reader**: local refs resolve here, inherited ones name `graphite-gp` or `quartzite`. Exit 1 on any unresolvable citation. |
| `ai-audit/scripts/test-check-citations.sh` | before editing the guard | Locks the content-addressed (never line-pinned) exclusion, and that the guard restores the tree it edits. |
| `ai-audit/scripts/test-piped-gate-guard.sh` | before editing the piped-gate hook, and by CI's Harness-guards job | 26 fixtures through the hook body **extracted from `.claude/settings.json`**, never a retyped regex: 15 must-block, 10 must-allow, plus the known false positive asserted as blocked. Fails when a regex edit breaks either direction. |
| `task/scripts/append-task-run.sh` | `/task` Step 12 sub-step 5a | Single writer of `ai-docs/metrics/task-runs.jsonl`. Degrades rather than halting Step 12. |
| `task/scripts/test-append-task-run.sh` | before editing the writer | 20 cases; AC6 asserts the case count equals `ai-docs/task-run-schema.md` § *Cases* — add a row there in the same commit as a new case. |
| `pr-merged/scripts/cleanup-progress.sh` | `/pr-merged` step 3 | Derives the merged PR's issue number from its body, finds the matching spec in `plans/done/`, and deletes only that branch's local progress files. |

All three regression suites must pass `shellcheck -s bash` and run green before `git add` (`AGENTS.md` § *Build & Test*).

## Permissions

`allow` covers the project's own toolchain (`go`, `gofmt`, `golangci-lint`, `make`, `git`, `gh`, `ast-index`, `psql`, `actionlint`, `shellcheck`) plus read-only text tools. `deny` covers `.idea/**`, `**/.env*` and `**/secrets*` — the bot token and the database DSN must be unreachable to both `Read` and `Edit`.

## CI — `.github/workflows/ci.yml`

| Job | Runs when | Gates |
|---|---|---|
| Format | `go` paths changed | `make fmt-check` — `golangci-lint fmt -d` prints no diff |
| Build | `go` paths changed | `make build`, `make vet`, `make tidy-check` (`go mod tidy` leaves no delta) |
| Test | `go` paths changed | `make test` then `make test-race` |
| Lint | `go` paths changed | `make lint` at the pinned `golangci-lint` version, then `make file-limits` (hard 1000 / 1500 line limits) |
| Harness guards | `.claude/**`, `ai-docs/**`, `AGENTS.md`, `CLAUDE.md` changed | shellcheck on every script **and** every hook body; the citation guard; all three guard suites; the 40k instruction-file cap (35k warns); every relative markdown link resolves |
| Actionlint | `.github/workflows/**` changed | `actionlint` via reviewdog |

**A filtered-out job is not a passing job.** `dorny/paths-filter` decides what runs; a new artefact class must be added to its filter in the same PR that introduces it, or its gate silently stops running.

**No check is required at the merge button** — GitHub refuses rulesets on a private repository on the free plan (`AGENTS.md` § Permissions). CI reports; discipline enforces.

## Dependabot — `.github/dependabot.yml`

Weekly, two ecosystems: `gomod` (commit prefix `build`) and `github-actions` (prefix `ci`), 5 open PRs each. `/dependabot-pr` triages the `gomod` ones; a `github-actions` PR bails at preconditions as out of scope for v1.
