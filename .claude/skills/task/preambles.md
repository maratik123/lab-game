# /task — Preambles (⚡ First–Fourth, detail)

The step-by-step sequences behind the four `⚡` preambles at the top of `SKILL.md`. The SKILL body keeps each preamble's guard sentence and routing summary; this page keeps the recipe. Extracted from `reference.md` so that each supporting file stays a thin, on-demand read (the thin-`SKILL.md` + supporting-file split `/ai-audit` Checklist K prescribes). Jump to the section the SKILL body names; do not read this page top-to-bottom on every invocation.

## ⚡ First — validation sequence (detail)

The ⚡ First preamble's glob `ls ai-docs/plans/*.progress.md` is a flat match — it ignores branch and merge state. Two failure modes have already burned cycles in this repo:

1. **Stale-merge.** The matched progress file's task already merged via a GitHub-UI merge that bypassed `/pr-merged` (gitignored `.progress.md` survived). RESUME-ing into this points at a completed task instead of starting the new one. _See the sibling **quartzite** project's `ai-docs/learnings.md` 2026-05-13 stale-`.progress.md` entry._
2. **Wrong-branch parallel PR.** The matched progress file belongs to an unrelated in-flight PR on a different feature branch. RESUME-ing here cross-contaminates the two flows. _See quartzite's `ai-docs/learnings.md` 2026-05-14 branch-aware-probe entry._

**Validation sequence (run before the RESUME jump):**

1. Read `**Branch:**` and `**base_commit:**` from the matched `.progress.md`.
2. **Stale-merge check.** `git merge-base --is-ancestor <base_commit> origin/main` — if exit code is `0`, the task's base commit is now an ancestor of `origin/main`, meaning the work merged. Stale candidate.
3. **Branch-match check.** Compare the progress file's `**Branch:**` against `git branch --show-current`. If they differ, the user is on a different branch from the progress file's owner. Wrong-branch candidate.
4. If **either** check signals a mismatch, surface the situation to the user with three options and wait for direction — do NOT jump to RESUME:
   - **delete** — `rm ai-docs/plans/<base>.progress.md`, then proceed with the new task (Steps 1–7 or whichever applies).
   - **park** — `mv ai-docs/plans/<base>.progress.md ai-docs/plans/<base>.progress.md.parked`; the `.parked` suffix takes it out of the glob, allowing the new `/task` to start cleanly. Restore via the reverse `mv` later.
   - **RESUME anyway** — user explicitly chooses to ignore the mismatch (rare; typically only when reviving an interrupted task whose branch happens to be re-checked-out).
5. If both checks pass (base_commit NOT in origin/main AND branch matches), proceed to the RESUME flow defined in the SKILL body.

## ⚡ Second — deferred plan activation sequence

If `$ARGUMENTS` contains words like "activate", "start", "proceed" **and** a matching plan exists in `ai-docs/plans/deferred/`:

1. Identify the matching `*.spec.md` (and `*.design.md` if present) in `ai-docs/plans/deferred/`.
2. Move them to `ai-docs/plans/`:
   ```bash
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.spec.md ai-docs/plans/
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.design.md ai-docs/plans/     # if exists
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.progress.md ai-docs/plans/   # if exists
   ```
3. Update `ai-docs/plans/INDEX.md`: move the plan row from the **Deferred plans** table to the **Active plans** table and mark its status as `🟢 ready` (or `🟡 spec-only` if no design).
4. Tell the user: "Activated plan [name] — moved spec (and design) to `ai-docs/plans/`."
5. **Verify the spec carries `**Tracked in:**`** — if missing, run `/interview`'s tracking-issue resolution to find or create one and add it to the spec header before continuing.
6. If a `.progress.md` was moved: treat it as an active task — read it and resume from `## Next action` (same as the RESUME path above).
7. Otherwise (no progress file): skip Steps 1–7 and jump directly to Step 8 (spec + design already exist).

## ⚡ Third — bare-issue activation sequence (full)

Activation sequence (bare-issue → matching deferred spec):

1. Parse `$ARGUMENTS` — strip leading `#`, confirm it's a positive integer `N`.
2. Load issue body: `gh issue view <N> --json title,body,state,labels`. The `labels` field feeds `⚡ Fourth` (blocked-label reconciliation) on the next preamble.
3. Grep deferred specs for the tracking reference:
   ```bash
   grep -l "^\*\*Tracked in:\*\* #<N>\b" ai-docs/plans/deferred/*.spec.md
   ```
   If grep returns **zero matches**: fall through to Steps 1–5 (interview-driven flow). The issue body loaded in step 2 is available as context for the interview.
   If grep returns **one match**: continue with step 4.
   If grep returns **multiple matches** (unexpected — `**Tracked in:**` should be 1:1 with an issue): surface the list to the user and ask which spec to activate before proceeding.
4. Move the matched spec (and its `*.design.md` / `*.progress.md` siblings if present) from `ai-docs/plans/deferred/` to `ai-docs/plans/`:
   ```bash
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.spec.md ai-docs/plans/
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.design.md ai-docs/plans/     # if exists
   mv ai-docs/plans/deferred/YYYY-MM-DD-name.progress.md ai-docs/plans/   # if exists
   ```
5. Update `ai-docs/plans/INDEX.md`: move the plan row from **Deferred plans** to **Active plans**; status `🟢 ready` (or `🟡 spec-only` if no design exists yet).
6. Surface the spec's existing `## Acceptance Criteria` table to the user verbatim and ask: *"Confirm these ACs, or revise before continuing?"* — wait for the user's response before proceeding. Any revisions must be applied to the spec file before Step 6 launches.
7. **Do NOT run the interview.** **Do NOT create an `*.state.md` interview state file.** **Do NOT re-resolve the tracking issue** — the spec's existing `**Tracked in:** #<N>` is authoritative.
8. If a `.progress.md` was moved (rare — a prior `/task` run on this spec was interrupted): treat the activated task as a resume and jump to the RESUME path's `## Next action`.
9. Otherwise jump directly to **Step 6** (design phase) — the spec exists, ACs are confirmed, the interview phase is satisfied.

## ⚡ Third — bare-issue activation decision table (detail)

The keyword trigger in `⚡ Second` ("activate", "start", "proceed") does NOT fire on a bare integer, so `/task <N>` would otherwise enter the interview machinery and create a spurious `*.state.md` file even when `ai-docs/plans/deferred/2026-05-01-paint-style.spec.md` already carries `**Tracked in:** #<N>`. The `⚡ Third` preamble catches this case.

| If `$ARGUMENTS` resolves to... | Action |
|---|---|
| A bare issue number (`/task <N>` or `/task #<N>`) AND a deferred spec exists with a `**Tracked in:**` line matching that number | Run `⚡ Third`'s activation sequence — do NOT launch the interview, do NOT create a state file. |
| A bare issue number AND no matching deferred spec | Fall through to the Steps 1–5 interview phase (the issue's body becomes the interview seed). |
| Free text / keyword-triggered activation / empty args | Skip this phase; the active-task probe above (if applicable) or Steps 1–5 cover those entry modes. |

## ⚡ Fourth — blocked-label reconciliation sequence

Fires when the resolved input is a gh issue whose `labels` array (fetched by `⚡ Third` step 2) contains `blocked`. Runs AFTER `⚡ Third` and BEFORE Steps 1–5 / Step 6.

1. Enumerate blockers from the issue body — `Blocked by #M` / `Depends on #M` references. Treat any free-text blocker ("blocked on the auth rewrite") with no `#M` form as an unresolvable open blocker for step 4.
2. Query each `#M` blocker: `gh issue view <M> --json state`.
3. **All blockers CLOSED** → `gh issue edit <N> --remove-label blocked` (the stale label removal is part of the deliverable; it keeps `/next` / `/triage` filter accuracy honest), then continue with the normal flow.
4. **At least one blocker OPEN, or any unresolvable free-text reference** → pause and ask the user one of:
   - which blockers to wait on (do **not** start work);
   - which blockers to disregard for this issue (closed-as-not-planned, unrelated cross-ref) — if zero open blockers remain after the user's answer, proceed with step 3 (remove the label);
   - or whether to start work anyway accepting the risk — in this case do **NOT** remove the label (the issue is still semantically blocked; the user is overriding the gate explicitly).

Never proceed silently when any blocker is open. The `blocked` label is the project's gate signal; starting `/task` past it without reconciliation defeats the gate and risks producing a spec/design that hits the unresolved dependency mid-implementation.
