# `.progress.md` format (canonical)

Single source of truth for the progress-file format. `/task`, `/project-review`, `/pr-commented`, `/bugfix`, and the `review-findings` / `self-review` Subagents all read and write it; the **required** fields below must be present in every progress file regardless of which workflow created it. `/interview`, `/verify-change`, and `/pr-merged` are exempt (see *Exemptions* below).

```markdown
# Progress: [task name] — ACTIVE
_Updated: YYYY-MM-DD HH:MM_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** [branch name]
**base_commit:** [git rev-parse HEAD output]
**Last build:** PASS / FAIL / not run

<!-- Optional, /task only — omit for /project-review: -->
**Issue:** [#number or URL]
**Spec:** ai-docs/plans/YYYY-MM-DD-name.spec.md

<!-- Compaction-recovery / re-entry fields (required for code-side orchestrator skills): -->
**current_step:** [skill-internal step or phase name, e.g. "Step 8 — Subtask 3", "Phase 1 — review-findings"]
**last_passed_gate:** [command + ISO-8601 timestamp + commit SHA, e.g. `golangci-lint run | 2026-05-15T18:42Z | 549282b`]

<!-- Optional re-entry fields: -->
**parent_skill:** [/task | /project-review | /pr-commented]    <!-- when this progress file is owned by a nested skill (e.g. /bugfix invoked from inside /task Step 8); omit when the current skill IS the parent flow -->
**entry_args:** [the original $ARGUMENTS that started this flow]   <!-- required for /task progress files (recorded at Step 8 creation, read-only thereafter); optional elsewhere. Routes /task's three preambles correctly on re-entry after compaction. -->

## Next action

**Do this immediately:** [one concrete sentence — file + what to do]

## Subtasks

- [x] 1. done thing
- [x] 2. done thing
- [ ] 3. current/next thing  ← CURRENT
- [ ] 4. pending

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step N**: [decision + reason in one line]
- **Step N+1**: [decision + reason in one line]

## Key discoveries (don't re-investigate)

- [finding]: [why it matters / what we decided]

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS / FAIL / NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-3 | round 1 | major | fixed@a1b2c3d | `go test ./internal/foo -run TestBar` |
| R1-7 | round 1 | minor | accepted@1 — [one-line reason] | `wc -c ai-docs/x.md` |

## Files touched

- `internal/<pkg>/<file>.go` — what changed
```

### `## Review register` semantics

- **id** `R<round>-<n>` is permanent; a finding keeps its id across rounds. A later finding that restates an earlier one is not a new row — it is the old row re-opened (`status: open 🔁@<round>`).
- **status vocabulary:** `open` · `fixed@<sha>` · `accepted@<round> — <one-line reason>` · `superseded→<id>`. `accepted` means a reviewer examined the item and ruled it not-a-defect or inherent; it is the durable form of "Recorded, not raised".
- **verifying command** is the command whose output settles the row (the failing command for a defect; the measuring command for a threshold). Required for every `blocker`/`major` row.
- The register is the **only** cross-round memory the loop has. A per-round findings table documents a round; the register is what the next round is scoped by.

## Required vs optional fields

**Required fields** (read by `self-review` at handoff and by the *compaction recovery check* callout in every code-side orchestrator SKILL.md): `**Branch:**`, `**base_commit:**`, `**Last build:**`, `**current_step:**`, `**last_passed_gate:**`, `## Decisions log` section.

**Optional fields** (added by `/task` only): `**Issue:**`, `**Spec:**`.

**Conditional re-entry fields:** `**parent_skill:**` (required when a nested skill is currently writing into the parent's progress file; omit otherwise); `**entry_args:**` (required for `/task` progress files; optional elsewhere).

## Lifecycle by field

| Field | Writer(s) | Lifecycle |
|---|---|---|
| `**Branch:**` | Creator (`/task` Step 8 or `/project-review` Phase 1) | Immutable after creation |
| `**base_commit:**` | Creator | Immutable after creation |
| `**Last build:**` | Every step boundary | Overwritten — most recent state only |
| `**Issue:**`, `**Spec:**` | `/task` only, at creation | Immutable after creation |
| `**current_step:**` | Every step boundary (per AC4) | Overwritten — most recent step only; on re-entry the value is a hint, NOT an instruction to skip to that step (per the *Full-read-on-re-entry invariant*) |
| `**last_passed_gate:**` | After each successful `go build ./...` / `golangci-lint run` / `go test ./...` / etc. | Overwritten — most recent passed gate only |
| `**parent_skill:**` | Set at creation when a nested skill owns this file | Immutable after creation |
| `**entry_args:**` | `/task` at Step 8 (initial flow); preserved through nested skills | Immutable after creation — read-only thereafter; routes the active-task probe on re-entry after compaction |
| `## Decisions log` | Every non-trivial decision, append-only | Append-only — never edit or remove prior entries; the audit trail across steps |
| `## Review register` | Reviewer (new rows + `accepted@N`), fixer (`fixed@<sha>`) | **Append rows, update only the `status` cell of existing rows; never rewrite or delete a row.** One row per finding across ALL rounds — the cross-round memory of the loop |
| `## Subtasks`, `## Key discoveries`, `## AC Status`, `## Files touched` | Per-subtask updates | Updated in-place as work progresses |

## Lifecycle (process)

- **Created by:** `/task` Step 8 (start of implementation) or `/project-review` Phase 1 (review-findings agent).
- **Extended by:** subtask updates (Step 8 per-subtask); `self-review` agent appends `## Self-Review (Round N)` sections at each round (Step 10); `/pr-commented` appends `## Comment cycle round M` sections after merge-base on reviewer comments. The new compaction-recovery fields (`**current_step:**`, `**last_passed_gate:**`, `## Decisions log`) are written at every step boundary, before further tool calls.
- **Tracked, then retired — two phases (`/task` and `/main-ci-failed`).** The creating flow commits this file (`git add -f` once, because the path matches a `.gitignore` glob; once tracked the glob stops applying) and every later writer stages it with a plain `git add` in its own commit. As the last commit before its PR, the flow `mv`s it into an ignored directory — `ai-docs/plans/ignored/` for `/task`, `ai-docs/main-ci/ignored/` for `/main-ci-failed` — and commits the deletion of the old path. **Why the tracked phase:** six agents write this file over hours, and the `## Review register` in it is the loop's only cross-round memory; untracked, one truncating edit is unrecoverable. **Why the move rather than a plain untrack:** it also takes the path out of the `ls ai-docs/plans/*.progress.md` probe that `⚡ First` scans, so a finished run cannot mis-route the next one. **What it costs:** the content stays in the branch's commit objects and reaches `main` through the merge commit, even though the net diff and the final tree are clean.
- **Not deleted.** `/pr-merged` removes only the fallback surfaces of PRs no flow produced (`ai-docs/pr-comments/`, `ai-docs/ci-fixes/`). A retired state file stays on disk under its ignored directory. `/project-review` and `/triage` keep their own untracked-throughout lifecycles, each deleting its own file.
- **Exception:** `/project-review`'s own `ai-docs/plans/YYYY-MM-DD-project-review.progress.md` is deleted explicitly by `/project-review` SKILL on self-review APPROVE — a separate lifecycle from `/task`'s.

## Exemptions

These skills do NOT participate in `.progress.md` discipline:

- **`/interview`** — its durable state is the in-flight spec at `<spec_path>` plus the `.state.md` sibling; no separate `.progress.md`. The compaction-recovery callout in `/interview` SKILL.md routes through the `.state.md` `round:` counter.
- **`/bugfix`** — extends its existing trace file (`ai-docs/bugfix/trace-YYYY-MM-DD-<name>.md`) with the same `**current_step:**` / `**last_passed_gate:**` header lines plus a `## Decisions log` section, instead of creating a parallel `.progress.md`. The trace file IS the `/bugfix` durable-state surface.
- **`/verify-change`** and **`/pr-merged`** — near-stateless. No `.progress.md` discipline applies; re-entry consists of re-invoking the skill.
