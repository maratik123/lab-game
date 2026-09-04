---
name: task
description: "Full task workflow from a user description OR a GitHub issue number: interview → spec → design → design-review → impl → verify → self-review. Steps are strictly ordered and cannot be skipped."
disable-model-invocation: true
argument-hint: "[issue-number | task description]"
allowed-tools: Bash(go build *) Bash(go test *) Bash(go vet *) Bash(go mod *) Bash(gofmt *) Bash(golangci-lint *) Bash(actionlint *) Bash(shellcheck *) Bash(git diff *) Bash(git rev-parse *) Bash(git checkout *) Bash(git branch *) Bash(git add *) Bash(git commit *) Bash(git push *) Bash(gh issue list *) Bash(gh issue view *) Bash(gh issue create *) Bash(gh issue comment *) Bash(gh pr create *) Bash(gh pr view *) Bash(.claude/skills/task/scripts/append-task-run.sh *) Bash(make *)
---

Full workflow for a task. Steps execute **strictly in sequence** — proceeding to N+1 before N is complete is FORBIDDEN.

> **Commit authorization.** The default rule "only commit when the user explicitly asks" does **not** apply inside this workflow. Commits at Step 8 (per subtask) and the commit + `git push` + `gh pr create` at Step 12 are pre-authorized by `/task` itself — perform them without an extra prompt. Pause to confirm only if the situation is ambiguous beyond the prescribed step (e.g., commits would touch main, files outside the task scope, or sensitive paths).

The task may originate from either:
- a **GitHub issue number** (e.g. `/task <N>` or `/task #<N>`) — `/interview` reads the issue body during Steps 1–5
- a **user description** (e.g. `/task add foo to bar`) or empty (`/interview` interviews the user)

> **⚡ Compaction recovery check — read FIRST on every invocation.**
> If you are re-entering this skill after auto-compaction (a
> summary/compaction block appears at the top of context, or workflow
> context feels thin), STOP before any tool call and:
>
> 1. **Locate the durable-state file via this skill's active-state probe**
>    — run the preamble glob (`ls ai-docs/plans/*.progress.md 2>/dev/null`) and apply the validation it
>    documents (stale-merge, branch-match, or PR-linkage as the preamble
>    prescribes). The probe both finds the path AND decides whether to
>    RESUME, delete, park, or treat the situation as fresh.
> 2. Once the probe identifies the correct durable-state file
>    (the matched `ai-docs/plans/<spec-base>.progress.md`), read it **top-to-bottom in one
>    pass** — every line, including older sections and the `## Decisions
>    log` section. Do not skim. The recorded `current_step` is a
>    cross-check, never an instruction to skip the read.
> 3. **Then re-enter this skill from the top of its body** — let the
>    preamble's probe / validation / RESUME sequence route control. Do
>    NOT jump to a numbered Step directly; the preamble owns the routing.
>    The probe will land you at the right next action without re-doing
>    completed work.
>
> If the probe finds no matching durable-state file (or returns a
> validated "no active task" result), this is a fresh invocation —
> proceed normally.
>
> See `.claude/skills/context-reset/SKILL.md` § **Compaction recovery
> (re-entry)** for the canonical handoff rationale.

## ⚡ First: check for active task

```bash
ls ai-docs/plans/*.progress.md 2>/dev/null
```

> **Re-entry-after-compaction case (lost-arguments path).** If `$ARGUMENTS` is empty (lost to
> compaction) AND the glob above finds a matching file with `**entry_args:**` recorded, treat the recorded
> `entry_args` as the canonical entry reference. The `⚡ Second` and `⚡ Third` preambles must NOT fire on the **lost-arguments path** — they require a positive match against the live `$ARGUMENTS`, which by definition is unavailable on re-entry. Only `⚡ First` (active-task probe) is allowed to route a lost-arguments re-entry. If the probe finds no matching progress file, the re-entry is "fresh"; surface this to the user (do NOT proceed to Steps 1–5 silently, because the user's original task is unknown).

**If found → validate the match BEFORE jumping to RESUME.** The probe is a flat glob — it matches any `.progress.md` regardless of git branch or merge state. Run the four-step **stale-merge + branch-match** validation in `preambles.md` § ⚡ First — validation sequence (detail). If either check fails, surface to user with **delete / park / RESUME anyway** options before continuing; do NOT auto-RESUME on a mismatch.

**RESUME flow (skip Steps 1–7) — only after validation passes:**
1. Read the `.progress.md` file
2. Read spec — only `## Acceptance Criteria`
3. Read only files from `## Files touched`
4. Jump to `## Next action`
5. Tell user: "Found active task [X], resuming from [subtask Y]"

---

## ⚡ Second: check for deferred plan activation

> **Guard sentence.** This preamble fires only when `$ARGUMENTS` is **non-empty**. On a lost-arguments re-entry (empty `$ARGUMENTS` after compaction), do NOT enter this preamble — fall through to `⚡ First`'s active-task probe instead. See `⚡ First`'s lost-arguments clause.

If `$ARGUMENTS` contains words like "activate", "start", "proceed" **and** a matching plan exists in `ai-docs/plans/deferred/`, run the deferred-plan activation sequence in `preambles.md` § ⚡ Second — deferred plan activation sequence: move spec/design (and `.progress.md` if present) from `deferred/` to `ai-docs/plans/`, update `INDEX.md`, verify `**Tracked in:**`, then resume the moved `.progress.md` if any (else skip Steps 1–7 and jump to Step 8).

---

## ⚡ Third: bare-issue activation of a matching deferred spec

> **Guard sentence.** This preamble fires only when `$ARGUMENTS` is **non-empty** (a bare integer). On a lost-arguments re-entry (empty `$ARGUMENTS` after compaction), do NOT enter this preamble — fall through to `⚡ First`'s active-task probe instead. See `⚡ First`'s lost-arguments clause.

> **AXIOM — When `$ARGUMENTS` is a bare gh issue number, search deferred specs for `**Tracked in:** #N` BEFORE launching the interview.**
>
> | If gh issue #N has... | Action |
> |---|---|
> | Zero deferred specs with `Tracked in: #N` | Fall through to Steps 1–5 (fresh interview) |
> | Exactly one matching deferred spec | Move spec + design + progress into `ai-docs/plans/`, update INDEX.md, surface ACs for confirmation, jump to Step 6 |
> | Multiple matches | Surface to user |
>
> A bare integer does NOT trigger the keyword preamble (`⚡ Second`), so `/task <N>` would otherwise spin up a spurious interview state file even when a deferred spec already carries a matching `**Tracked in:**` line. See `preambles.md` § ⚡ Third — bare-issue activation decision table (detail) for the full resolution recipe.

Activation sequence: `preambles.md` § ⚡ Third — bare-issue activation sequence (full) — parse `$ARGUMENTS` (strip leading `#`), load `gh issue view <N> --json title,body,state,labels`, grep `ai-docs/plans/deferred/*.spec.md` for `**Tracked in:** #N`, then route per the table above. On exactly one match: move the spec (and `*.design.md` / `*.progress.md` siblings) into `ai-docs/plans/`, update `INDEX.md`, surface the ACs for confirmation, do NOT re-run the interview, do NOT create `*.state.md`, then jump to Step 6 (or RESUME if a `.progress.md` came along).

---

## ⚡ Fourth: blocked-label reconciliation

> **Guard sentence.** Fires when the resolved input is a gh issue — bare-number `/task <N>` (whether `⚡ Third` matched a deferred spec or fell through), OR `⚡ Second`/`⚡ Third` activated a deferred spec whose `**Tracked in:** #N` points to a real issue. Free-text `/task` invocations with no issue reference skip this preamble entirely. Runs AFTER `⚡ Third` and BEFORE Steps 1–5 / Step 6.

If the resolved issue's `labels` array (already fetched by `⚡ Third` step 2) contains `blocked`, run the reconciliation sequence in `preambles.md` § ⚡ Fourth — blocked-label reconciliation sequence before proceeding: enumerate the `Blocked by #M` / `Depends on #M` blockers (a free-text blocker with no `#M` form counts as an unresolvable open blocker), query each with `gh issue view <M> --json state`; **all CLOSED** → `gh issue edit <N> --remove-label blocked` and continue; **any OPEN or unresolvable** → pause and ask the user which blockers to wait on, which to disregard, or whether to start anyway (then the label stays — the issue is still semantically blocked). Never proceed silently when any blocker is open — the `blocked` label is the project's gate signal, and a `/task` started past it risks a spec/design that hits the unresolved dependency mid-implementation.

---

### Steps 1–5: Spec creation (delegated to `/interview`)

`/task` does not duplicate the interview workflow. Treat Steps 1–5 as a single delegated phase. If a saved spec already exists under `ai-docs/plans/`, confirm with the user and skip to Step 6. Otherwise invoke `Skill(skill="interview", args="$ARGUMENTS")` — the interview handles entry-mode detection, scope confirmation, clarifying-question rounds, tracking-issue resolution, spec writing, and the cross-link comment.

> **AXIOM — the `args` hand-off contract (binding here).** `args` is the user's task text, byte-for-byte — restating or "clarifying" it is FORBIDDEN. Reconnaissance rides only in the three-section shape — `## TASK (verbatim)` (the user's text, fenced) · `## RECON (unverified claims)` (a `READ:` line, a `NOT READ:` line, then findings as claims — no verdicts, no instructions, no reading directives) · `## DELTA` (one line per constraint present in the hand-off but absent from TASK, with its source, or the literal line `DELTA: none`); the exact fenced template is in `reference.md` § Steps 1–5 — spec creation delegation (detail).
>
> Bare text (no headers) = TASK alone, always legal. Restated task, instructions inside RECON, missing `NOT READ:` or missing DELTA = **malformed — the interview returns it without starting work** (`interview/SKILL.md` Step 1). Scope comes from TASK alone; RECON is claims with issue-body standing; DELTA makes every orchestrator-added constraint a visible line, not an ambient assumption. Spec-only runs move the spec to `ai-docs/plans/deferred/` and stop. **Before Step 6:** confirm the spec exists at `ai-docs/plans/YYYY-MM-DD-name.spec.md` and the user has approved its `## Acceptance Criteria`. Full delegation narrative: `reference.md` § Steps 1–5 — spec creation delegation (detail).

### Step 6: Design-Writer Subagent

First action: confirm the spec exists. Spawn the `design-writer` Subagent (per `.claude/agents/design-writer.md`) with the spec path; result: `ai-docs/plans/YYYY-MM-DD-name.design.md`.

**Commit the design as soon as it lands, and after every design round** — `git add <design>` and commit. The branch already exists (`/interview` Step 2 created it). The design is read by every delegate for hours and is rewritten whole each round; untracked, one truncating rewrite is unrecoverable, which is not hypothetical (`ai-docs/learnings.md` 2026-09-02: a 172-line design reduced to its header, restored only as a reconstruction).

### Step 7: Design review

Spawn the `design-review` Subagent with **exactly** these five things: the invocation line (`Read .claude/agents/design-review.md and follow it.`), the spec path, the design path, the progress-file path (when one exists), and the round number — **nothing else**. No `Context:` paragraph, no amendment history, no "verify that X now matches Y", no framing of what changed. Anything beyond the list becomes a `major` `PROMPT-CONTAMINATION` finding against this orchestrator, and the reviewer then ignores the content it flagged. The amended artefacts are on disk; the round number is the only state a gate prompt carries. (Enumerated here rather than left as "per `design-review.md`" because the one in-flow spawn example an orchestrator used to meet — the amendment recipes' template — carried a `Context:` line and shipped the contamination: `ai-docs/harness-gaps.md` 2026-09-02.)

Verdict: GO / ITERATE / STOP.
- **GO** → proceed to Step 8. Spec-amending notes (AC/constraint changes) need Step 6 → Step 7 re-run, not a fold-in — see `reference.md` § Spec Amendment recipe.
- **ITERATE** → back to Step 6 (max 3 rounds total).
- **STOP** → fundamental flaw with the approach. Surface the verdict and `Issues` table to the user, do not start Step 8. Wait for direction (e.g., narrow scope, change approach, abandon).

> **AXIOM — `*.spec.md` and `*.design.md` writes are subagent-owned.**
> The `/task` orchestrator NEVER writes to `ai-docs/plans/*.spec.md` or `ai-docs/plans/*.design.md` (including `done/` siblings). All such writes go through the responsible Subagent: `spec-writer` for `*.spec.md`, `design-writer` for `*.design.md`. Orchestrator-side direct edits with `Edit` / `Write` are FORBIDDEN.
> Temptation table (four shapes, each routed to the owning subagent) and the recurrence history: `reference.md` § Subagent-owned writes.

### Design Amendment (re-entrant — triggered from Step 8 or Step 11)

If implementation (Step 8) reveals a necessary deviation from the design, **or** a self-review finding (Step 11) requires a design change rather than a code fix: stop the step, surface to user for approval, **spawn the `design-writer` Subagent to update the design doc** (orchestrator MUST NOT edit `*.design.md` directly — see the AXIOM above), re-run Step 7 design-review (max 3 rounds total). On GO → resume the triggering step. See `reference.md` § Design Amendment recipe for the full procedure.

> Silently implementing a deviation without triggering Design Amendment — FORBIDDEN.
> Orchestrator-side direct edits to `*.design.md` / `*.spec.md` — FORBIDDEN (per the AXIOM above).

> **AXIOM — a scope question carries its route.** A question whose "yes" changes Scope, an AC, a KD, or a standing constraint (incl. `settings.json` permissions) names its amendment route in its text: `→ spec amendment via spec-writer + design-review re-run` (or `→ design amendment via design-writer`). Every phase, every originator. "Yes" to a bare question authorises the CHANGE, never a silent fold-in (`reference.md` § Amendment-route rule).

> **Amendment re-review is unconditional — exemption is the owner's, per instance.** No category of "mechanical" edits exempts a re-run; only the owner's explicit word in the surfaced answer does, for that instance only. Cap exhausted + amendment required → the owner raises the cap as an explicit number; the orchestrator never invents a bypass.

---

### Step 8: Implementation

> First action: verify spec + design + GO verdict exist AND that every `note`/`minor`/recommendation from the latest design-review GO has been written back into the design document. "Applied in code later" is NOT the same as "resolved in the design"; the design doc is the implementation contract. See `reference.md` § Step 8 — first-action GO-notes verification (detail). Unresolved GO-notes = previous steps incomplete.

- **The feature branch already exists** — `/interview` Step 2 created it before the first commit of this flow, and Steps 6–7 have been committing to it. Verify, do not re-create:
  ```bash
  git branch --show-current    # must not be main; must match the spec's date-name
  ```
  If it *is* `main`, the interview was skipped (a saved spec was reused): `git checkout -b feat/YYYY-MM-DD-name` now, using the spec file's date-name. Record the branch name in the progress file. Create the in-flight marker: `date -u +%FT%TZ > ai-docs/plans/.task-inflight` (gitignored; Stop-hook contract — see § In-flight marker).
- **Visibility from the first group return (binding).** When the FIRST Step-8 group returns and its subtask commits are in: `git push -u origin <branch>`. Every subsequent group return and every Step-11 fix round pushes. **No PR yet** — the PR is created at Step 12, after self-review APPROVE; CI on the PR is the FINAL gate (AGENTS.md § Workflow carve-out). The push is visibility and survivability, not presentation.
- **Before every `git commit` in this step:** run `git branch --show-current` and confirm it is NOT `main`. If it is — stop immediately, do not commit, apply the recovery procedure in AGENTS.md.
- **Before every `git commit` in this step:** stage a modified/untracked `ai-docs/learnings.md` with the related code change, and after every push give a later-written entry its own commit in the same turn — AGENTS.md § *Workflow* (learnings are part of the deliverable). Order: write learning → `git add ai-docs/learnings.md` → commit → push.
- **Commit the progress file at creation** — `git add -f ai-docs/plans/YYYY-MM-DD-name.progress.md` (the `-f` is needed exactly once, because the path matches a `.gitignore` glob; once tracked the glob no longer applies) and commit it with the current spec/design state. Every later writer stages it with a plain `git add` in its own commit. Step 12 retires it. **Why:** six agents write this file over hours, and the `## Review register` inside it is the only cross-round memory the loop has — the re-litigation tripwire is computed from those rows.
- Create `ai-docs/plans/YYYY-MM-DD-name.progress.md` at start using the canonical schema at [`ai-docs/templates/progress-format.md`](../../../ai-docs/templates/progress-format.md). Required: `**Branch:**`, `**base_commit:**`, `**Last build:**`, `**current_step:**`, `**last_passed_gate:**`, `**entry_args:**`, plus a `## Decisions log` h2 section. For `/task` flows also include `**Issue:**` / `**Spec:**`. Record `**entry_args:**` (original `$ARGUMENTS` — bare ref, keyword phrase, free text, or `(none)`) ONCE and **read-only thereafter**; on lost-arguments re-entry it is the canonical entry reference. Full header template + write-once rule: `reference.md` § Step 8 — progress-file creation template (detail).
- After each subtask:
  1. `go build ./...` — must compile
  2. `go test ./... -run TestName` — if subtask adds tests
  3. `golangci-lint fmt`; `golangci-lint run`
  4. Update `.progress.md` — **at this subtask boundary, rewrite `**current_step:**` to `Step 8 — subtask N of M complete`; rewrite `**last_passed_gate:**` to `golangci-lint run | <ISO-8601 UTC timestamp> | <commit SHA from git rev-parse HEAD>`; append a `## Decisions log` bullet for any non-trivial choice made during this subtask (one line, prefixed `Step 8 subtask N:`; omit if none).**
  5. **Every-group handoff (binding, not optional).** During Step 8 the orchestrator NEVER executes subtask code in its own context — every group fans out through `/context-reset`, including the first group and designs with M = 1, per the design's `## Handoff plan` (required for every M ≥ 1 per `.claude/agents/design-writer.md` § Rules → handoff-grouping). Between group returns the orchestrator reads the subagent's progress-file delta (`current_step`, `last_passed_gate`, tail of `## Decisions log`) and re-validates state — branch matches `**Branch:**`, `base_commit` unchanged in the progress header, and no uncommitted dirt: `git diff --quiet && git diff --cached --quiet` (or an empty `git status --porcelain`), **never bare `git diff --quiet`**, which is blind to staged-only changes — before spawning the next group's handoff. If the runtime delta disagrees with the `## Handoff plan` (extra subtasks completed, group boundary moved), trigger the **Design Amendment recipe** — do NOT silently advance. **Per-group implementor selection:** read the group's marked model from the `## Handoff plan` — a **code** group (`sonnet`) → `subagent_type="code-writer"` with NO inline `model=`/effort override (its `model: sonnet` + `effort: medium` are frontmatter-pinned; the file is the only lever); an **instructions/harness** group (`inherit`) → `subagent_type="general-purpose"` with NO inline `model=` (it inherits the orchestrator's model), effort inherited. The orchestrator model itself and the `design-writer` / `design-review` / `self-review` gates are never overridden. Rationale, the rejected orchestrator-pinning alternative (Key Decision Q3) and the failure modes this prevents: `reference.md` § Every-group handoff (rationale); protocol: `.claude/skills/context-reset/SKILL.md`.
- Unknown API → read sources → grep codebase → ask user. Don't guess.
- Bug report during impl → activate `/bugfix`, then return here.
- **Irreversible outward-facing deliverable in a Step-8 subtask → hold the publish for Step 12.** The group implementor drafts the content to scratch and PREPARES — does NOT run — the publish command (a `gh issue comment` / PR comment, an external post); the orchestrator runs it at Step 12 after self-review APPROVE (and, for a public post, explicit user confirm). A wrong analysis must not go public before self-review can catch it.
- Implementation reveals design must change → trigger **Design Amendment** above, then resume here.
- **Local FAIL investigation before push (AGENTS.md workflow corollary).** When `go test ./...` returns `FAILED`, isolate and reproduce the failing test before treating it as transient. See `reference.md` § Step 8 — local FAIL investigation before push for the recipe.

### Step 9: Verify

Run the full verify list in `reference.md` § Step 9 — verify list (full): `go build ./...`, `go test ./...`, `go test -race ./...` (when the change touches goroutines / the scheduler / shared state), `golangci-lint fmt -d`, `golangci-lint run`, `go vet ./...`, `go mod tidy` + `git diff --exit-code go.mod go.sum` (only if dependencies moved), `actionlint` (only if workflows changed), `shellcheck` (only if a shell script changed), panic-index sync (see `reference.md` § Step 9 — panic-index sync (detail)), domain-invariant sweep (see `reference.md` § Step 9 — domain-invariant sweep), then per-AC coverage check and a `| # | Criterion | Test / Verification | Status |` summary table. On ALL PASS → Step 9.5.

**Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 9 — Verify (ALL PASS)`; rewrite `**last_passed_gate:**` to `golangci-lint run | <ISO-8601 UTC timestamp> | <commit SHA from git rev-parse HEAD>`; append a `## Decisions log` bullet recording any panic-index addition and any posting-signature / event-dictionary addition (one line, prefixed `Step 9:`; omit if none).

### Step 9.5: Update documentation

Update content files only — **do not move spec/design to `done/` yet** (that happens at Step 12). Append this task's per-issue implementation-status entry to `ai-docs/context-status.md` (the detailed log), bump the affected block's summary bullet + resolve open questions in `ai-docs/context.md` (thin orientation page — summary bullets only, never detail), and update any repo-root user-facing doc that the change contradicts. Full detail: `reference.md` § Step 9.5 — documentation update (detail).

**The entry's PR locator is the literal `#TBD-at-Step-12`, filled at Step 12 sub-step 10a.** `context-status.md`'s own heading template is `## <name> — <outcome> (<PR #N>, <YYYY-MM-DD>)`, and the number does not exist yet — the PR is created at Step 12 sub-step 10. Write the placeholder, never a guess and never a blank: it is the token sub-step 10a substitutes and CI's Harness-guards job greps for, so an unfilled locator turns the PR red instead of shipping. It shipped once with no mechanism to catch it (`ai-docs/harness-gaps.md` 2026-09-03).

**No counts here — name the things, do not tally them.** A test count, a file count, a package tally, an "N sites" figure: none of it goes into `context-status.md` or `context.md`. Nothing reads such a number, every one of them is one command away for anyone who wants it, and the only thing storing it guarantees is that it will be false later. Write what landed — the tables the migration creates, the gates the Makefile adds, the files the sweep touched — and let the reader count if they ever need to. **Where a run's figures DO belong:** the telemetry record appended at Step 12 sub-step 5a (`ai-docs/metrics/task-runs.jsonl`), which is a time series over runs that no longer exist and cannot be re-measured. That is the one place a measured number earns storage. A `file:line` you must write anyway (a panic-index locator) is still re-derived in this turn, after the last edit, with the command that produces it — **NEVER** transcribed from a subagent's return summary, which is a claim (AGENTS.md § *Workflow*).

**A diff that REMOVES something has a wider doc surface than one that adds** — for every name your diff removed, `grep -ni '<removed-name>' <every doc you are touching>` before closing the edit (case-insensitive: a completeness sweep over prose, AGENTS.md § *Propagation Rule*). Adding what is now true does not discharge deleting what is now false. Rationale for both rules: `reference.md` § Step 9.5 — documentation update (detail).

**Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 9.5 — docs updated`; append a `## Decisions log` bullet recording any open questions resolved in `context.md` (one line, prefixed `Step 9.5:`; omit if none).

Then proceed to Step 10.

### Step 10: Self-review loop (max 3 rounds)

Spawn the `self-review` Subagent with **exactly**: the invocation line (`Read .claude/agents/self-review.md and follow it.`), the spec path, the design path, the progress-file path, and the commit range — nothing else. A `PreToolUse` hook refuses the spawn on any line outside those shapes and names it (`.claude/settings.json`, matcher `Task|Agent`); a line that reaches the reviewer anyway is a `major` `PROMPT-CONTAMINATION` finding against the orchestrator, discharged only by re-spawning — one whole round (`ai-docs/learnings.md` 2026-09-03, round 2 of that run). **Caps, round history, routing state and priorities never enter a reviewer prompt** — routing is decided after the verdict, not signalled before it. **Reviewer reuse:** warm only to re-verify fixes of its OWN findings; new material (new diff, amended artefact) spawns COLD — `reference.md` § Reviewer reuse.

> **Cap arithmetic (binding).** The round cap is an absolute integer; the charter value is 3. A user's raise sets it to an explicit number; a multiplicative raise ("x2", "x3") applies to the CHARTER value, never to a previously raised one — raises do not compound. The turn that applies a raise MUST echo `cap: N (was M)`.
>
> **Re-litigation tripwire (binding).** After each round: `share = rows re-opening or citing an earlier round ÷ rows raised`. `share ≥ 50%`, or any register row at its SECOND re-opening → the loop STOPS regardless of remaining cap; surface the register to the user via `AskUserQuestion`. A loop feeding on its own rounds is not converging; continuing is a user decision, never an orchestrator one.

**On APPROVE:** proceed to Step 12. The progress file is tracked until Step 12 retires it and **stays on disk either way** — `/pr-commented` extends it across reviewer rounds; nothing deletes it. Do NOT `rm` it here. **Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 10 — self-review APPROVE (Round N)`; append a `## Decisions log` bullet recording the round count (one line, prefixed `Step 10:`).

**On REJECT:** proceed to Step 11. After Step 11, loop back here. **Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 10 — self-review REJECT (Round N), addressing findings`; append a `## Decisions log` bullet recording the finding count + severity breakdown (one line, prefixed `Step 10:`).

**After round 3 with REJECT:** surface all remaining `⬜ Open` findings to the user and ask how to proceed.

### Step 11: Review fixes

> **AXIOM — A self-review finding whose proposed fix diff touches `*.design.md` or `*.spec.md` in `ai-docs/plans/` is a Design/Spec Amendment trigger, NOT an ordinary Step-11 code-fix.**
> Mechanical detection at the top of every fix round; the trigger fires regardless of how small the doc edit appears. Recurrence history (three quartzite incidents, including the propagation gap that added this flow): `reference.md` § Step 11 — review-fix narrative (detail).
>
> | If the proposed Step 11 fix diff includes... | Action |
> |---|---|
> | A `*.design.md` file under `ai-docs/plans/` (active or `done/`) | **STOP.** Trigger the **Design Amendment recipe** above — surface to user, update the design, re-run Step 7 design-review on the amended design (max 3 rounds), then resume Step 11 from the GO verdict. Mark the originating finding `✅ Fixed (design amended)`. Do NOT commit the design-doc edit as a code-fix commit. |
> | A `*.spec.md` file under `ai-docs/plans/` (active or `done/`) | **STOP.** Trigger the **Spec Amendment recipe** (`reference.md` § Spec Amendment recipe) — surface to user, update the spec, re-run Step 6 design → Step 7 design-review on the amended (spec, design) pair, then resume Step 11. Mark the originating finding `✅ Fixed (spec amended)`. |
> | A `*.spec.md` / `*.design.md` **coordinate only** — the cited claim still describes the artefact correctly, but its line number, path or offset moved (an added import, a `git mv`, a re-ordered block) | **NOT an amendment.** Re-resolve the coordinate yourself, record the re-resolved value in the register row, and continue the code-fix path. No `spec-writer`, no `design-writer`, no re-review, no round. This row is the cheap half of the pair: four self-review rounds and two design-review rounds were spent on this class before it existed (`ai-docs/harness-gaps.md` 2026-09-02). A changed **criterion** or a changed **design decision** is not a coordinate drift and takes the rows above. |
> | Only `*.go` / `go.mod` / migrations / non-`ai-docs/plans/` `*.md` / other source files | Normal Step 11 code-fix path — apply, re-run gates, push. |

**Order the batch so every measurement is taken after the last content edit.** Before writing any figure into a durable surface, ask: *does anything else in this batch touch the file I am measuring?* If yes, that edit lands first and the figure is re-derived after it — the Step-11 instance of AGENTS.md § *Communication* (never record-then-edit); full rationale in `reference.md` § Step 11 — review-fix narrative (detail).

For each `⬜ Open` finding in the latest `## Self-Review (Round N)` section of the progress file: **fix** (mark `✅ Fixed`), **design-amend** (trigger the Design Amendment recipe per the table above, mark `✅ Fixed (design amended)`), **spec-amend** (trigger the Spec Amendment recipe per the table above, mark `✅ Fixed (spec amended)`), or **object** (`nit`/`minor` autonomously; `major`/`blocker` only after user approval — mark `⚠️ Objected: <reason>`). See `reference.md` § Step 11 — review-fix narrative for the full procedure including the unconditional PR-body re-read and review-thread-resolution recipe.

> **A reviewer-emitted Spec/Design Amendment trigger has exactly two exits (binding):** executed (the recipe runs), or **surfaced to the user with the reviewer's wording quoted verbatim** and the user's answer recorded in the register row. Closing one in-thread — "decided not to", reasoning in a commit message, a fix that routes around the amendment — is not an exit; it is the orchestrator overruling the review machinery it is supposed to route.

After all findings are resolved, run gates (`go build ./...`, `go test ./...`, `golangci-lint run`) **and the fix-round measurement pass (binding):** execute the `verifying command` of every register row this round touched, plus every AC whose measurement the touched files could move (at minimum: every AC the design lists a verification command for over a file in this round's diff), quoting outputs into the register. Fix rounds feed the very prose files the size/count ACs measure — re-measure them each round. Then:

1. Update `.progress.md`.
1a. `git push` the fix-round commits — the Step-8 draft PR tracks every round; an unpushed fix round is invisible work.
2. **PR body sync (when a PR exists — i.e. fix rounds after Step 12, via `/pr-commented`).** `gh pr view <N> --json title,body`, re-read, then `gh pr edit` only if the body contradicts the new commits. Never skip the read. During pre-PR Step 11 rounds this item is a no-op.
3. **Resolve fixed review threads (unconditional).** GraphQL recipe per `reference.md` and AGENTS.md § *Workflow* (PR review comment resolution).
4. **Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 11 — review fixes complete (Round N)`; rewrite `**last_passed_gate:**` to `golangci-lint run | <ISO-8601 UTC timestamp> | <commit SHA from git rev-parse HEAD>`; append a `## Decisions log` bullet recording any `⚠️ Objected` rationale or Design-Amendment trigger (one line, prefixed `Step 11:`; omit if none).
5. Return to Step 10.

### Step 12: Finalise docs, commit, and create PR

1. **Step-skip gate (binding, not optional).** Read `**current_step:**` from `ai-docs/plans/<spec-base>.progress.md`. It MUST be one of `Step 10 — self-review APPROVE (Round N)` or `Step 11 — review fixes complete (Round N)`. If it is anything else (e.g. `Step 9 — Verify (ALL PASS)`, `Step 9.5 — docs updated`, `Step 8 — subtask N of M complete`) — **STOP**, do not proceed to commit, surface the gap to the user, and loop back to the missing step. The gate fires regardless of how trivial the diff appears — no "too simple" exemption; Step 10 has been silently skipped on "simple" tasks and post-compaction (four upstream incidents: `reference.md` § Step 12 — step-skip gate (recurrence history)). **Compaction-recovery exception:** if `**current_step:**` is unparseable or absent and the post-compaction summary suggests Step 10 ran, ask the user explicitly before treating Step 10 as satisfied — do NOT auto-pass the gate.
2. **The progress file and the interview state file are TRACKED at this point** (committed since `/interview` Step 2 and Step 8 respectively) and are retired in sub-step 9a below, immediately before `gh pr create`. Do not unstage them here and do not `rm` them: 9a is the single place they leave the tree, and they must keep receiving this step's writes until then.
3. Confirm `git branch --show-current` is **not** `main`. If it is — stop, do not push, tell the user, apply the AGENTS.md recovery procedure.
4. **Finalise INDEX.md and move plan files:**
   - Change the plan row status to `✅ implemented` — the status and nothing else. **No test count, no file count, no size.** A tally written here is true for one commit and false for the next, nothing reads it, and re-measuring it on each touch is what keeps it alive; `INDEX.md`'s own status legend has always said `✅ done`. The suite's real size is one `go test -list` away for anyone who wants it.
   - `git mv` the spec and design files to `ai-docs/plans/done/` — both are tracked (spec since `/interview`, design since Step 6). They are deliverables and stay in the PR.
   - Update dependency tree and **Suggested next steps**
5. **Inbox propagation.** Parse the just-finalised spec (and its design if present) and append one JSON line per item to `ai-docs/deferred/_inbox.jsonl` (row shape: [`ai-docs/templates/inbox-row.md`](../../../ai-docs/templates/inbox-row.md)); dedupe over `.source_path` against the thematic `*.jsonl` siblings via `jq`; emit a `WARN:` line on unrecognised body shapes and continue. Per-shape parser + dedupe rules: `reference.md` § Step 12 — inbox propagation (detail) and [`inbox-propagation.md`](inbox-propagation.md). Sub-step 7 stages `_inbox.jsonl` with the other artefacts.
   - **Sub-step 5a — append the task-run telemetry record** — single writer, append-only; schema, field table and operating rules: [`ai-docs/task-run-schema.md`](../../../ai-docs/task-run-schema.md). Run that page's § *Precondition assertion* first, then `.claude/skills/task/scripts/append-task-run.sh ai-docs/plans/<spec-base>.progress.md`. Exit 0 (full **or** degraded) → continue; non-zero → hand-write the `fallback-required` fields per its § *Fallback recipe*, then continue. **Never halt Step 12 here.** Finally run its § *Step-12 verification block* and record both results in the PR body **Test plan**.
6. `go build ./...` — ensures `go.sum` is refreshed and included if changed.
7. Stage all changed files: implementation files from `## Files touched`, `context-status.md` (+ `context.md` if its summary changed), any repo-root doc the change touched, `ai-docs/learnings.md` (if modified), updated `INDEX.md`, `ai-docs/deferred/_inbox.jsonl` (rows appended in sub-step 5), `ai-docs/metrics/task-runs.jsonl` (record appended in sub-step 5a), spec/design now in `done/`, and `ai-docs/plans/<spec-base>.progress.md` (tracked; its Step-10/11/12 writes ride in this commit, and sub-step 9a retires it).
8. Commit `feat(<package>): <imperative summary>` with a 1–3 line body and `N new tests; all M tests green.`
9. `git push` (the branch tracks `origin` since the first Step-8 group return).
9a. **Retire the run's state files — the last commit before the PR (binding).** Move both out of `ai-docs/plans/`, then commit the deletion of the old paths:
    ```bash
    mkdir -p ai-docs/plans/ignored
    mv ai-docs/plans/<spec-base>.progress.md ai-docs/plans/ignored/
    mv ai-docs/plans/<spec-base>.spec.md.state.md ai-docs/plans/ignored/   # if it exists
    git add -u && git commit -m "chore(plans): retire the run's state files before the PR"
    git push
    ```
    Measured, so the shortcut is not attempted: `mv` leaves both files **intact on disk** under an ignored directory, `git add -u` stages the two deletions, and after the commit `git diff <base>...HEAD` carries none of them, `git status` is clean, and — the part a plain `git rm --cached` does **not** give — the paths also leave the `ls ai-docs/plans/*.progress.md` and `ls ai-docs/plans/*.spec.md.state.md` probes that `⚡ First` and `/interview` scan. **Verify before continuing:** both files readable under `ai-docs/plans/ignored/`, `git status --porcelain` empty, and each probe returning nothing. If any of the three fails — STOP: a state file in the PR diff violates the standing constraint, and one deleted from disk destroys `/pr-commented`'s input.
10. `gh pr create` with the full title + body — body must include **Summary** / **Tracking** (`Closes #N` for full-resolve or `Refs #N` for partial; omit if `Tracked in: none`) / **Test plan** (one line per AC + the gate results by name). Full body template: `reference.md` § Step 12 — PR-body template (detail).
10a. **Fill the PR locator — the first action after `gh pr create` (binding).** The number Step 9.5 could not know now exists. `Edit` `ai-docs/context-status.md`, replacing the literal `#TBD-at-Step-12` in this run's entry heading with `#<N>` — the number `gh pr create` just returned. Use `Edit`, never a stream editor: `Edit` refuses a non-unique match, and the CI guard keeps every other entry free of the token, so an edit that succeeds is proof there was exactly one occurrence and it was this run's. Then commit and push:
    ```bash
    git add ai-docs/context-status.md
    git commit -m "docs(context-status): fill the PR locator"
    git push
    ```
    **Verify before continuing:** `grep -n 'TBD-at-Step-12' ai-docs/context-status.md` returns nothing. If it still matches, the edit did not land — fix it now, not in a note: the previous run recorded the intent to backfill in its progress file and shipped the placeholder anyway, because sub-step 9a retires that file before the PR exists, so an intention parked there is a promise in a document that has already left the tree (`ai-docs/harness-gaps.md` 2026-09-03).
11. Post the PR URL to the user.
12. **Write progress at this step boundary** before further tool calls: rewrite `**current_step:**` to `Step 12 — PR opened (PR #<N>)`; append a `## Decisions log` bullet recording the PR number and the spec/design `done/` move (one line, prefixed `Step 12:`).
13. Remove the in-flight marker: `rm -f ai-docs/plans/.task-inflight`. (The Stop hook blocks ending a turn while the marker exists unless the turn handed control to the user — see § *In-flight marker* below.)

After the PR is created, the unconditional PR-body re-read rule (AGENTS.md *Workflow*) applies to any subsequent push on this branch: `gh pr view <N>` first, then `gh pr edit` only if the body now contradicts the diff.

**Reviewer comments arrive after Step 12** — run `/pr-commented` (one round per invocation, re-invocable). Do not re-enter `/task` for routine reviewer feedback; architectural-rework requests are the exception (`/pr-commented` bails → fresh `/task` design-review cycle).

---

## In-flight marker (Stop-hook contract)

`ai-docs/plans/.task-inflight` (gitignored) exists from Step 8 entry to Step 12 item 13. The `Stop` hook blocks ending a turn while it exists, unless the turn appended a hand-back line `handback: <ISO-8601 UTC> <reason ≤ 10 words>` to the marker — do that only when the turn genuinely hands control to the user (an `AskUserQuestion`, a surfaced blocker, a user stop); the next orchestrator turn deletes the token line. Full contract: `reference.md` § In-flight marker. The rule it enforces: **naming the next step is not performing it** — a turn inside an active `/task` either advances the flow with tool calls or explicitly hands back.

## Patterns

### 1. Step 9's per-AC sweep is load-bearing, not ceremonial

*Prefer* running **your own command for each measurable AC, over that AC's own stated scope**,
and treating that result as authoritative. The AC states a condition, never a command
(`spec-writer.md` Rule 9/PROC-3) — writing the command is the verifier's job, and re-writing
it when the tree moves is why it lives in the progress file's `verifying command` column
rather than in the spec. Two upstream readings do **not** override it:
`design-review`'s narrower operative reading of an AC (an AC saying "anywhere in
`internal/`, grep-clean" binds every subtask, including one a later design section adds for
a *different* AC's traceability), and a delegate's "flagged, left as-is per the design".

*Default to* **building** missing coverage over recording a PARTIAL. When a delegate
waves a clause through as "untestable on the fixture I used", check whether the coverage
is genuinely unachievable or just needs a purpose-built fixture — the delegate's local
fixture choice can silently narrow coverage below the approved spec.

Earned in the **graphite-gp** harness — its `ai-docs/learnings.md` 2026-07-23 AC7-fixture entry,
and (graphite-gp again) its 2026-07-24 whole-tree grep-clean entry, both past a design-review GO.
The delegate-skepticism half also lives in AGENTS.md § *Patterns* 1.

---

**Reference:** [`preambles.md`](preambles.md) — the four `⚡` preamble sequences in full; [`reference.md`](reference.md) — anti-patterns, gate checklist, Design Amendment recipe, validation procedures, FORBIDDEN list, Step-specific narrative detail (every-group handoff rationale, local-FAIL investigation, panic-index sync, domain-invariant sweep, Step 11 review-fix narrative, Step 12 inbox-propagation rules).
