# /task — Reference

Reference material extracted from `SKILL.md` so the SKILL body stays a thin workflow (the thin-`SKILL.md` + supporting-file split `/ai-audit` Checklist K prescribes). The SKILL body owns the workflow steps; this file owns reference / troubleshooting / detail material; [`preambles.md`](preambles.md) owns the step-by-step sequences behind the four `⚡` preambles.

## Design Amendment recipe (re-entrant — triggered from Step 8 or Step 11)

If implementation (Step 8) reveals a necessary deviation from the design, **or** a self-review finding (Step 11) requires a design change rather than a code fix:

1. **Stop** the current step immediately. Do not silently continue with the deviated approach.
2. **Surface to user:** describe what changed and why the design must be updated. Wait for approval.
3. **Spawn the `design` Subagent** to update `ai-docs/plans/YYYY-MM-DD-name.design.md` to reflect the new approach. The orchestrator MUST NOT edit `*.design.md` directly — the `design` Subagent owns ALL writes to `*.design.md` (per the AXIOM in `SKILL.md` above the Design Amendment header). Orchestrator-side direct edits are FORBIDDEN.
   ```
   Agent(subagent_type="design", prompt="
     Read .claude/agents/design.md and follow it.
     Spec: ai-docs/plans/YYYY-MM-DD-name.spec.md
     Existing design: ai-docs/plans/YYYY-MM-DD-name.design.md
     Context: design must be amended during implementation / self-review — describe what changed and the user-approved direction.
   ")
   ```
   On Subagent return, immediately verify the design file was written (`ls ai-docs/plans/YYYY-MM-DD-name.design.md`). If missing — re-spawn the Subagent; do NOT transcribe its text output into the file.
4. Re-run design review — same as Step 7 (max 3 rounds total across all design-review runs). **The prompt is the closed list and nothing else** (`design-review.md` § Spawn prompt contract): five items, no `Context:` line, no description of what changed, no amendment history. The amended design is on disk and the reviewer reads it; anything you add becomes its finding #1 (`major`, `PROMPT-CONTAMINATION`) and is then ignored:
   ```
   Agent(subagent_type="design-review", prompt="
     Read .claude/agents/design-review.md and follow it.
     Spec: ai-docs/plans/YYYY-MM-DD-name.spec.md
     Design: ai-docs/plans/YYYY-MM-DD-name.design.md
     Progress: ai-docs/plans/YYYY-MM-DD-name.progress.md
     Round: <N>
   ")
   ```
5. **On GO** → resume from the step that triggered the amendment:
   - Triggered from Step 8 → resume Step 8 (continue the remaining subtasks)
   - Triggered from Step 11 → mark the finding `✅ Fixed (design amended)`, then return to Step 10
6. **On ITERATE** → fix the design and re-run design review (counts against the 3-round limit).
7. **On STOP** → surface to user; do not proceed until the design issue is resolved.

> Silently implementing a deviation without triggering Design Amendment — FORBIDDEN.

## Spec Amendment recipe (re-entrant — triggered from Step 7 GO-with-notes resolution)

If a Step 7 design-review GO verdict surfaces a `note` / `minor` / recommendation whose resolution requires a change to the **spec** (wording, AC, or technical-constraints edit) — not just an in-place design fold-in:

1. **Classify each note** at Step 7 close: **design-internal** (fold into the design doc in-place; no loop — current behaviour) vs **spec-amending** (the note implies a spec wording / AC / constraint change). Mixed batches are allowed; spec-amending notes trigger this recipe, design-internal notes proceed normally.
2. **Stop before Step 8.** Do not begin implementation against the pre-amendment spec, and do not fold spec-amending notes into the design alone — FORBIDDEN. The design doc is the implementation contract built **against the spec**; if the spec changes, the contract must be re-established and re-verified.
3. **Surface to user via `AskUserQuestion`** — describe the candidate spec amendment (which AC / which line / proposed new wording) and wait for explicit approval. Two paths are common: **Path A** — amend the spec to match the design's discovered shape; **Path B** — annotate the design with a "Spec amendment / supersession" subsection (still requires the re-loop below if the design's shape effectively changes the spec). Path A is the default.
4. **On user approval — amend the spec via the `spec-writer` Subagent.** The orchestrator MUST NOT edit `*.spec.md` directly (per the AXIOM in `SKILL.md` above the Design Amendment header). Spawn `spec-writer` with the user's approved amendment as a synthetic round (`extra_context` carries the amendment description); the Subagent re-writes the spec on disk. Orchestrator-side direct `*.spec.md` edits with `Edit` / `Write` are FORBIDDEN — mirrors `.claude/skills/interview/SKILL.md` § Anti-patterns ("Mutating the spec yourself").
5. **Re-enter Step 6 (`design` Subagent)** with explicit context: "spec was amended at Step 7 GO-with-notes resolution — re-verify decomposition and ACs against the new spec":
   ```
   Agent(subagent_type="design", prompt="
     Read .claude/agents/design.md and follow it.
     Spec: ai-docs/plans/YYYY-MM-DD-name.spec.md
     Existing design: ai-docs/plans/YYYY-MM-DD-name.design.md
     Context: spec was amended during Step 7 GO-with-notes resolution.
     Re-verify decomposition and ACs against the new spec. Update the design doc to reconcile any drift.
   ")
   ```
6. **Re-enter Step 7 (design-review)** against the new (spec, design) pair — same as the original Step 7 (counts against the 3-design-round-cap, which applies to the merged total of pre- and post-amendment iterations). **The prompt is the closed list and nothing else** (`design-review.md` § Spawn prompt contract): five items, no `Context:` line, no "verify the design now matches the amended spec" — that is a reading directive, and steering where a gate looks is contamination even when every word of it is true. This template shipped one (`ai-docs/harness-gaps.md` 2026-09-02):
   ```
   Agent(subagent_type="design-review", prompt="
     Read .claude/agents/design-review.md and follow it.
     Spec: ai-docs/plans/YYYY-MM-DD-name.spec.md
     Design: ai-docs/plans/YYYY-MM-DD-name.design.md
     Progress: ai-docs/plans/YYYY-MM-DD-name.progress.md
     Round: <N>
   ")
   ```
7. **On the new GO** → proceed to **Step 8**. Step 8's first-action GO-notes verification ("every note / minor / recommendation from the latest design-review GO has been written back into the design document") now references the **new** GO verdict; pre-amendment notes are no longer authoritative.
8. **On ITERATE** → fix the design (or re-amend the spec if a contradiction surfaces) and re-run design-review (counts against the 3-round cap).
9. **On STOP** → surface to user; do not proceed until the design / spec issue is resolved.

> Folding a spec-amending note into the design alone is FORBIDDEN — the design would be built against the old spec without ever being verified against the new one. _See quartzite's `ai-docs/learnings.md` 2026-05-15 (process) entry on spec amendment during GO-with-notes resolution._

## Steps 1–5 — spec creation delegation (detail)

**The `args` hand-off shape (binding — SKILL.md § Steps 1–5 AXIOM names the sections; this is the template to copy):**

```
## TASK (verbatim)
<the user's text, byte-for-byte, fenced>
## RECON (unverified claims)
READ: <files opened + commands run>
NOT READ: <what was not opened, or the sampling rule used>
<findings as claims — no verdicts, no instructions, no reading directives>
## DELTA
<every constraint present in this hand-off but absent from TASK, one line each, with its source — or the literal line `DELTA: none`>
```

`/task` does not duplicate the interview workflow. Scope extraction, key-decision confirmation, tracking-issue resolution, spec writing, and the cross-link comment are owned by `/interview` (`.claude/skills/interview/SKILL.md`). Treat these five steps as a single delegated phase.

**Already have a spec?** If a saved spec for this task already exists under `ai-docs/plans/` (e.g. the user previously ran `/interview` to draft the spec without implementing), confirm with the user that this is the spec to implement, then **skip to Step 6** — do not re-run the interview.

**Otherwise, run the interview** by invoking the `interview` Skill via the `Skill` Tool, passing the original `$ARGUMENTS` through:

```
Skill(skill="interview", args="$ARGUMENTS")
```

The interview will:

- detect entry mode (issue ref / free text / empty) and load the issue body if applicable
- extract and confirm scope (in / out / deferred)
- ask any clarifying questions (max 4 rounds, max 3 questions per round)
- resolve or create the tracking GitHub issue
- save the spec at `ai-docs/plans/YYYY-MM-DD-name.spec.md` with `**Tracked in:** #N` and an `## Acceptance Criteria` table
- post a cross-link comment on the tracking issue (unless `Tracked in: none`)

When the Skill call returns, `/interview`'s instructions and the saved spec are both in conversation context. Resume with the next paragraph of `/task` (the spec-only check, then Step 6).

**Spec-only run.** If the user wants to stop after the interview ("just draft the spec, defer the implementation"), move the spec to `ai-docs/plans/deferred/`, update `INDEX.md` (move the row to **Deferred plans**, status `🟡 spec-only`), and **do not proceed to Step 6**. The spec can be picked up later via the deferred-plan-activation preamble above.

**Before Step 6:** confirm `ai-docs/plans/YYYY-MM-DD-name.spec.md` exists and the user has approved its `## Acceptance Criteria`.

## Step 8 — progress-file creation template (detail)

Create `ai-docs/plans/YYYY-MM-DD-name.progress.md` at start using the canonical format spec at [`ai-docs/templates/progress-format.md`](../../../ai-docs/templates/progress-format.md). Required fields: `**Branch:**`, `**base_commit:**`, `**Last build:**`, `**current_step:**`, `**last_passed_gate:**`, `**entry_args:**`, plus a `## Decisions log` h2 section. Optional: `**parent_skill:**` (when `/task` itself was invoked from another skill — rare). For `/task` flows also include `**Issue:**` and `**Spec:**`.

Record base commit, branch, and `entry_args` in the progress file header immediately:

```
**Branch:** feat/YYYY-MM-DD-name
**base_commit:** <output of `git rev-parse HEAD`>
**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | <ISO-8601 UTC timestamp> | <commit SHA from git rev-parse HEAD>
**entry_args:** <original $ARGUMENTS at /task entry — bare issue ref (`#348`/`348`), `activate paint-style`, free text (`add foo to bar`), or `(none)` for empty entry>
```

The `**entry_args:**` field is recorded ONCE at Step 8 creation and **read-only thereafter** — Steps 9–12 do NOT touch it. On a lost-arguments re-entry (empty `$ARGUMENTS` after compaction), this recorded value is the canonical entry reference per `⚡ First`'s lost-arguments clause.

## Step 9.5 — documentation update (detail)

Update content files only — **do not move spec/design to `done/` yet** (that happens at Step 12):

1. **`ai-docs/context-status.md`** (detailed per-issue log) — append this task's implementation-status entry: the per-issue bullet capturing design decisions, traps, and invariants worth not rediscovering (the same shape as the existing `## Status` bullets there). This is where the growing per-issue log lives — **not** `context.md`, which stays a thin orientation page.
2. **`ai-docs/context.md`** (orientation) — update only if a block's high-level state changed: bump the `## Status` summary bullet for the affected block, resolve open questions answered during implementation, keep the Architecture / Track-artifact orientation current.
3. **Repo-root user-facing docs** — update any that this change contradicts (a README status line, a runbook). Skip when the change touches none.

**Two measurement rules — full text (the SKILL body carries the binding sentence of each):**

**Every number and every `file:line` you write here is a measurement, not a recollection.** A test count, a package tally, a panic-index row locator, a "N sites" figure — re-derive each one **in this turn, after the last edit**, with the command that produces it (`go test ./internal/<package>/ -count=1` for a per-package count; the locator `rg` re-run after the final `golangci-lint fmt`). **NEVER** transcribe a figure from a subagent's return summary — it is a claim (AGENTS.md § *Workflow*), and a `code-writer` has already reported one package's count for another's. If a new figure contradicts an existing durable baseline (README's test line, a panic-index row), the resolution is a **fresh measurement**, never picking one of the two.

**A diff that REMOVES something has a wider doc surface than one that adds.** Prose enumerates what exists, so a deleted dep edge / flag / module / panic-index row leaves assertions scattered through documents you are otherwise editing correctly. Adding a description of what is now true does **not** discharge the obligation to delete what is now false — and the two routinely live in the same file, paragraphs apart. Concrete trigger: for every name your diff removed, `grep -ni '<removed-name>' <every doc you are touching>` **before** closing the edit — case-insensitive, because this is a completeness sweep over prose (AGENTS.md § *Propagation Rule*; identifier-like names get capitalised mid-sentence). The salience of a removal that was an *acceptance criterion* is what makes this feel already-handled.

## Step 8 — first-action GO-notes verification (detail)

Verify both spec and design (with GO verdict) exist AND that **every note / minor / recommendation from the latest design-review GO verdict has been written back into the design document**. "Applied in code later" is NOT the same as "resolved in the design"; the design doc is the implementation contract. Scan the most recent `## Self-Review (Round N)` / `## Verdict: GO` block emitted by `.claude/agents/design-review.md` — for each `## Issues` row of `Severity: note` / `minor` and each `## Recommendations` bullet, confirm the corresponding API table / helper list / risk table / decomposition section of `ai-docs/plans/YYYY-MM-DD-name.design.md` was updated to match. If any note is unresolved → stop, edit the design doc to incorporate it (and re-run design-review if the change is non-trivial per the Design Amendment rule), and only then begin coding. Missing spec, missing design, missing GO verdict, OR unresolved GO-notes = previous steps incomplete.

## Step 9 — verify list (full)

1. `go build ./...` — compiles clean
2. `go test ./...` — all green
3. `go test -race ./...` — green, **required** when the change touches goroutines, the scheduler, or shared state; skipped otherwise (say which, and why, in the verify table)
4. `golangci-lint fmt -d` — no diff
5. `golangci-lint run` — clean (it lints `_test.go` files too; if you narrowed it to one package while iterating, re-run it whole before Step 10)
6. `go vet ./...` — clean
7. `go mod tidy` then `git diff --exit-code go.mod go.sum` — only when dependencies moved; a non-empty diff means the change was not what you thought
8. **actionlint / shellcheck gate** — `actionlint <file>` on every created or modified `.github/workflows/*.yml`, `shellcheck <file>` on every created or modified `*.sh`. Skip only when none were touched. See AGENTS.md § *Build & Test*.
9. **`make file-limits`** — clean. CI's Lint job runs it, and no other gate on this list covers it: `golangci-lint run` stays green on a file that breaks the 1000 / 1500-line limit, so skipping this one records `ALL PASS` on a tree CI will reject. Running `make verify` discharges items 1–8 and this one together.
10. **Panic-index sync** — see `## Step 9 — panic-index sync (detail)` below.
11. **Domain-invariant sweep** — see `## Step 9 — domain-invariant sweep` below.
12. For each AC — confirm covered by test or manual verification. **An AC states a condition, not a command** (`spec-writer.md` Rule 9/PROC-3): for a **measurable** AC — one naming a regexp, a glob, a scope, a symbol or a test — **you write the command that checks it**, run it over that AC's own stated scope, and treat the result as authoritative — not `design-review`'s narrower operative reading, not a delegate's "flagged, left as-is". Record the command you used in the progress file's `verifying command` column; it belongs to you and it is expected to change between rounds. An AC row that *does* carry a shell command is a spec defect — re-derive the criterion, run your own command, and raise it. See § *Patterns* 1 in [`SKILL.md`](SKILL.md#1-step-9s-per-ac-sweep-is-load-bearing-not-ceremonial).
13. Show a `| # | Criterion | Test / Verification | Status |` summary table.
14. On ALL PASS → proceed to Step 9.5.

## Every-group handoff (rationale)

During Step 8 the orchestrator NEVER executes subtask code in its own context. Every group fans out through `/context-reset` — including the first group, and including designs whose total subtask count is one. The orchestrator's role during Step 8 is strictly *to spawn group handoffs, parse each subagent's progress-file delta, re-validate state, and spawn the next group's handoff* until the design's `## Handoff plan` is exhausted. Re-state the rule to yourself before deciding the next action: *"Did I just receive a group return? Then the next action is to spawn the next group's `/context-reset` handoff, until the design's `## Handoff plan` is exhausted. No exceptions for 'one more quick subtask in this turn' or 'the first group is small enough to do inline'."* See `.claude/skills/context-reset/SKILL.md` for the handoff protocol.

**Failure modes this prevents.** Two PR-level incidents in the sibling **quartzite** project (`maratik123/quartzite`, whence this harness was adapted) motivated the every-group redesign that replaced the prior runtime-gate regime:

- **`maratik123/quartzite#339`** — a long-lived orchestrator session hit auto-compaction mid-task. The compaction summary did not reproduce the strict step sequence faithfully and Step 10 (self-review) was silently omitted.
- **`maratik123/quartzite#374`** — a sonnet-model orchestrator session hit auto-compaction mid-Step-8. The post-compaction session showed *"context rot"*: skipped Step 9 verify gates, missed the Step 10 self-review spawn, and missed the runtime handoff trigger that should have fired after the 3rd subtask. The runtime trigger was load-bearing precisely when the compacted context could no longer reproduce it — the prior runtime-gate regime relied on the same context that compaction had just degraded.

The every-group fan-out removes the failure mode structurally: the orchestrator's own context never grows long enough to trip compaction (Step 8 subtask work runs in short-lived subagent invocations), and the `## Handoff plan` is the per-group spec the orchestrator reads at each return.

**Trigger source: design's `## Handoff plan` section.** As of the every-group redesign (quartzite's PR for `maratik123/quartzite#375`), the `design` Subagent produces a `## Handoff plan` section in the design document for **every** decomposition with M ≥ 1 (per `.claude/agents/design.md` § Rules → handoff-grouping). That section names the exact group boundaries and the per-group spawn order — pre-computed at design time. Single-subtask designs (M = 1) carry a `## Handoff plan` with one group, fanned out via one `/context-reset` invocation; M = 9 → 3 groups, fanned out via 3 `/context-reset` invocations. Every M ≥ 1 design now carries explicit per-group fan-out.

**Per-group implementor selection — the file is the only lever.** The *orchestrator* model is per-invocation; pinning it was considered and rejected (Key Decision Q3 of the every-group redesign). Per-group *implementor* selection is a different decision — do not conflate the two. A **code** group (marked `sonnet` in the `## Handoff plan`) spawns `subagent_type="code-writer"`, whose `model: sonnet` + `effort: medium` are frontmatter-pinned; pass NO inline `model=`/effort override, because there is no per-invocation `effort` parameter, so an inline `general-purpose` code spawn could never enforce a "medium (pinned)" tier. An **instructions/harness** group (marked `inherit`) spawns `subagent_type="general-purpose"` with NO inline `model=` (it inherits the orchestrator's model) and effort inherited — both in a 1M-token window. No spawn takes an inline `model=` override; the orchestrator itself is per-invocation and the `design` / `design-review` / `self-review` quality gates inherit it (`model: inherit`).

**Why the clean-tree check is two commands.** `git diff --quiet && git diff --cached --quiet` (or an empty `git status --porcelain`) — never bare `git diff --quiet`, which compares the working tree to the **index** and so reports a staged-only change as clean. That is exactly the pre-spawn state AGENTS.md § *Workflow* phase (2) forbids: a staged file lands in the delegate's commit.

## Step 8 — local FAIL investigation before push (AGENTS.md workflow corollary)

When `go test ./...` returns `FAILED`, identify the specific failing test (`grep "FAILED"` on the output) and reproduce it in isolation (`go test ./... -run TestNameSubstring -v`) before deciding the failure was transient. A subsequent green run is NOT proof of transience — different test-thread assignments or environment vars (DISPLAY, WAYLAND_DISPLAY) can flip the result. Only accept "transient" when the test is known flaky AND multiple reruns are consistently green. _See quartzite's `ai-docs/learnings.md` 2026-05-11 entry for the winit-`EventLoop::new()`-on-worker-thread case._

## Step 9 — panic-index sync (detail)

Scan new/modified production sources for documented or direct panic sites and update `ai-docs/panic-index.md` if any are introduced:

- `rg -n '(^|[^[:alnum:]_.])(panic\(|log\.(Fatal|Panic)[a-z]*\()' --type go <changed-files>` — direct panic sites; walk the hits and skip `_test.go` files
- `rg -n 'func Must[A-Z]' --type go <changed-files>` — `Must…` helpers, which are panics by contract

For each new production hit, add a row to `ai-docs/panic-index.md` (location, trigger, invariant, why not an error return). Stage `panic-index.md` with the implementation commit. Skip when this task added no new production panics. `main` exiting non-zero at startup is not a panic and needs no row.

## Step 9 — domain-invariant sweep

Run this sweep whenever the diff touches balances, items, sessions, the scheduler, outbound messages, or a migration. Full rules: [`ai-docs/domain-invariants.md`](../../../ai-docs/domain-invariants.md).

```bash
# 1. Balance mutation outside the ledger
rg -n 'UPDATE\s+\w*(balance|stamina|money|resource)' --type go --type sql <changed-files>
# 2. Postings and item movements — each under exactly one basis document
rg -n 'INSERT INTO (postings|item_movements)|store\.Post\(' --type go --type sql <changed-files>
# 3. Balance constants compiled into Go instead of configuration
rg -n '(?i)(cap|cost|timer|ttl|rate|price|budget|dice)\s*[:=]+\s*[0-9]' --type go <changed-files>
# 4. Non-determinism on a pure path (generation / combat / replay)
rg -n 'time\.Now\(\)|math/rand' --type go <those files>
# 5. Secrets in a tracked file
rg -n '[0-9]{8,10}:[A-Za-z0-9_-]{35}' <changed-files>
```

For each hit, decide and record: a legitimate case gets one line in the progress file's decisions log saying **why**; anything else is fixed before Step 10. A new mechanic that moves balances must additionally have (a) its event(s) in the dictionary, (b) its posting signature declared, (c) the contract test asserting actual postings against that signature — `docs/DESIGN.md` §13.4 makes that same-PR, never follow-up.

## Step 11 — review-fix narrative (detail)

For each `⬜ Open` finding in the latest `## Self-Review (Round N)` section of the progress file:

- **Fix it** → mark `✅ Fixed` in the progress file, implement the change.
- **Requires a design change** → trigger the **Design Amendment** recipe above (user approval required); on return mark `✅ Fixed (design amended)`.
- **Object to it** (finding is wrong or intentionally out of scope):
  - `nit` / `minor`: Subagent may object autonomously — write reason, mark `⚠️ Objected: <reason>`.
  - `major` / `blocker`: **surface to user first** before objecting. User must approve the objection.

After all findings are resolved (`✅ Fixed` or `⚠️ Objected`), run the **full** gate set — `go build ./...`, `go test ./...`, `golangci-lint fmt -d`, `golangci-lint run`, `go vet ./...` — **after EVERY fix, including ones an agent (or you) calls trivial: "one character", "just a typo", "doc-comment only"**. A fix's *size* is not evidence of its *risk*; "the fix is one character" is a claim about the edit, never about the gates. A doc-comment edit can fail `revive` while build, vet and tests stay green — `go vet` does not subsume `golangci-lint run`, and vice versa. Then update `.progress.md`, re-read the PR body (`gh pr view <N> --json title,body`) and edit only if it contradicts new commits, and resolve every fixed review thread per the GraphQL recipe in AGENTS.md § *Workflow* (PR review comment resolution) — reply via REST, query unresolved thread IDs via `reviewThreads`, then `resolveReviewThread` each fixed thread and verify `isResolved: true`. Threads behind `⚠️ Objected` findings stay open. Skipping this resolution earned the same correction twice in the **graphite-gp** harness.

**Batch ordering — full rationale.** A Step-11 fix batch routinely contains both a correction to a *number about* file F and an edit *to* F; batch order does not naturally put them in the valid order. Before writing any figure into a durable surface, answer: **"does anything else in this batch touch the file I am measuring?"** If yes, that edit lands first and the measurement is re-derived after it. A figure that was correct when you measured it is not thereby correct when you commit it — this is the Step-11 instance of AGENTS.md § *Communication* ("never record-then-edit") and of Step 9.5's measurement rule, and it is where that rule has recurred.

**Recurrence history behind the Step-11 amendment AXIOM.** Recurrences (in the sibling **quartzite** project's log): 2026-05-13 (notes not folded back), 2026-05-15 GO-with-notes resolution, 2026-05-21 design doc change committed directly during self-review fix (the latest is the propagation gap — Step 11 self-review fix flow was missing from the prior escalation set; see quartzite's `ai-docs/learnings.md` 2026-05-21).

## Step 12 — inbox propagation (detail)

The Step 12 sub-step 5 parser specification lives in a dedicated reference file: **[inbox-propagation.md](inbox-propagation.md)**. It covers the six shape rules (NONE / TABLE / PIPEBULLET3 / PIPEBULLET2 / BOLDBULLET / PLAINBULLET), the unrecognised-shape warning behaviour, the per-row mapping format (one JSON line per item appended to `_inbox.jsonl` — canonical row shape: [`ai-docs/templates/inbox-row.md`](../../../ai-docs/templates/inbox-row.md)), and the file-level dedupe rule against the thematic `.jsonl` files. Load it on demand when implementing or modifying Step 12's propagation logic.

**Per-step recap** of the Step 12 inbox-propagation sub-step:

- Run the parser against `ai-docs/plans/done/YYYY-MM-DD-name.spec.md` (and the matching `*.design.md` if it exists).
- Build the live dedupe set `H`: every `.source_path` value in the thematic `.jsonl` files in `ai-docs/deferred/` — every `*.jsonl` sibling of `_inbox.jsonl` (created by `/triage` as it drains the inbox into topic files; none may exist yet in a fresh repo, in which case `H` is empty), harvested via `jq -r '.source_path' <file>.jsonl | sort -u`.
- For each candidate row, dedupe at *file* granularity: if the candidate's `source_path` is in `H`, skip the entire file (all of its sections); otherwise append the JSON line to `ai-docs/deferred/_inbox.jsonl` below the existing body.
- Emit one `WARN: <spec-path> :: <section heading> — unrecognised body shape, no rows emitted` line to stdout for any section whose body matches none of the six shape rules; the row count for that section is zero and Step 12 continues normally.
- The Step 12 commit stages `_inbox.jsonl` alongside the existing artefacts.

## Step 12 — PR-body template (detail)

The `gh pr create` body (SKILL.md Step 12 item 10) carries these sections, in this order:

- **Summary** — what landed and why.
- **Tracking** — `Closes #N` when the PR fully resolves the tracking issue, `Refs #N` when it resolves it partially; omit the section when the spec carries `Tracked in: none`.
- **Test plan** (checklist: one line per AC, plus the gate results by name) — including the two results of `ai-docs/task-run-schema.md` § *Step-12 verification block* (sub-step 5a).

## Step 12 — step-skip gate (recurrence history)

Step 10 (self-review) has been silently skipped on "simple" tasks and post-compaction. Recurrence pattern, all in the sibling **quartzite** project ([its log](https://github.com/maratik123/quartzite/blob/main/ai-docs/learnings.md)): 2026-05-07 design-skip; 2026-05-14 `maratik123/quartzite#339` Step-10 skip; 2026-05-14 `maratik123/quartzite#281` compaction-induced skip (an issue, not a PR); 2026-05-16 `maratik123/quartzite#374` context-rot. The gate fires regardless of how trivial the diff appears — no "too simple" exemption (quartzite's `ai-docs/learnings.md` 2026-05-07 entry).

## FORBIDDEN

- Declaring done with uncovered ACs.
- Skipping design review.
- Writing code before the spec is confirmed.
- `rm`ing `.progress.md` from within `/task` (it's gitignored and lives until `/pr-merged`).
- Staging `.progress.md` into a commit.
- Pushing from the main branch.
- Silently deviating from the design without triggering Design Amendment.

## Gate checklist

| Before | Check |
|---|---|
| Steps 1–5 | Spec saved at `ai-docs/plans/YYYY-MM-DD-name.spec.md`? `**Tracked in:** #N` present (or `none` with reason)? Cross-link comment posted on the tracking issue (unless tracking skipped)? ACs confirmed by user and verifiable? See `/interview` gate checklist for the full per-step list. |
| Step 6 | Spec exists? ACs confirmed? Not a "spec-only / defer" run? |
| Step 8 | Design doc with GO? Test Design section present? **Every note / minor / recommendation from the GO verdict written back into the design doc?** |
| Step 8 start | Feature branch created? Run `git branch --show-current` before every `git commit` — must not be `main`. `base_commit` + `branch` recorded in progress file? |
| Each subtask | `go build ./...` ✅? Tests run? `.progress.md` updated? |
| Step 9 | `go build ./...` ✅? `go test ./...` green? `go test -race ./...` green when the change touches goroutines / the scheduler / shared state? `golangci-lint fmt -d` clean? `golangci-lint run` clean? `go vet ./...` clean? `go mod tidy` leaves `go.mod`/`go.sum` unchanged (only if deps moved)? `make file-limits` clean (no other gate here covers it — `golangci-lint run` stays green on an over-limit file)? `actionlint` clean on every changed workflow and `shellcheck` clean on every changed script (skip if none)? Any new `panic(` / `log.Fatal*` / `Must…` outside `_test.go` → `ai-docs/panic-index.md` updated and staged? Domain-invariant sweep run, every hit resolved or justified in the decisions log? All ACs covered? |
| Step 9.5 | context-status.md entry appended + context.md summary/README.md updated? (spec/design NOT moved yet — happens at Step 12) |
| Step 10 | Self-review APPROVE? (Progress file persists in working tree — gitignored — until `/pr-merged`. Do NOT `rm` it here.) |
| Step 11 | `major`/`blocker` objections confirmed by user? Design change → Design Amendment triggered? `gh pr view <N>` re-read after every push (unconditional) — `gh pr edit` only if body contradicts new commits? |
| Design Amendment | User approved the amendment? Design review returned GO before resuming? |
| Step 12 | Branch ≠ main? INDEX.md ✅? spec/design moved to done/? `_inbox.jsonl` parsed and appended (or warning logged for unrecognised shape) and staged? `go.sum` refreshed? PR body references the tracking issue (`Closes #N` or `Refs #N`)? PR created and URL posted? |

## In-flight marker — full contract (Stop hook)

`ai-docs/plans/.task-inflight` (gitignored, untracked) exists exactly while a `/task` run is between Step 8 entry and Step 12 completion. The `Stop` hook in `.claude/settings.json` blocks ending a turn while the marker exists, unless the marker's tail carries a hand-back token.

- **Create** at Step 8 entry: `date -u +%FT%TZ > ai-docs/plans/.task-inflight`. **Remove** at Step 12 item 13 (`rm -f`), or when the user aborts the task.
- **Hand-back token** — append in the SAME turn that legitimately ends with control at the user (an `AskUserQuestion` posted, a blocker surfaced for direction, an explicit user stop):
  `echo "handback: $(date -u +%FT%TZ) <reason, ≤ 10 words>" >> ai-docs/plans/.task-inflight`
  The hook reads only the last 3 lines, so stale tokens do not accumulate influence; the next orchestrator turn deletes the token line and resumes.
- **Hook mechanics:** fail-open when the marker is absent; respects `stop_hook_active` (never re-blocks its own continuation); on block, the stderr message restates this contract, so a session that has never read this page still gets the recipe at the moment it needs it.
- **What it enforces:** naming the next step is not performing it. Inside an active `/task` a turn has exactly two legal shapes — advance the flow with tool calls, or hand back explicitly. The announce-and-idle third shape was written into `learnings.md` three times in one session and violated three times; a rule that failed as text ships here as a gate.


## Subagent-owned writes — temptation table (SKILL.md § Design Amendment AXIOM)

| If the orchestrator is tempted to... | Do this instead |
|---|---|
| `Edit` a `*.design.md` to apply a self-review finding | Spawn `design` Subagent with the finding text |
| `Write` a `*.design.md` because the Subagent's text output didn't land on disk | Re-spawn the `design` Subagent; do NOT transcribe its text |
| `Edit` a `*.spec.md` to apply a user tweak after `interview` returned `ready` | Spawn `spec-writer` with the tweak as a synthetic round |
| `Edit` a `done/*.spec.md` during `/pr-commented` Spec Amendment | Same — route through `spec-writer` |

**Recurrence history.** Recurrences (in the sibling **quartzite** project's log, 2026-05-24 — 4 process entries): orchestrator wrote design file when subagent's text response landed without a `Write` call; orchestrator transcribed design output instead of re-running the agent; orchestrator directly edited spec on user tweak after `status: ready`; orchestrator edited design doc during spec-amendment sub-flow.

## Amendment-route rule — rationale (SKILL.md § Design Amendment AXIOM)

The obligation to route a spec/design change through its amendment recipe used to live at named POINTS: Step 11's fix-diff detection table, and the reviewer's literal "Amendment trigger" wording. Observed gaps, one per phase the points missed:

1. Session `ec78f817`: a reviewer-emitted spec-amendment trigger was closed in-thread — fixed by the two-exits rule at Step 11.
2. First post-forge run, DESIGN phase: the design agent raised genuine scope questions (propagation breadth; a `settings.json` permission) surfaced to the user BARE — correct questions, no route attached, so the user's "yes" had no defined next step.

The generalisation keys on the QUESTION'S SUBJECT, not the phase or the originator: if a "yes" changes Scope / an AC / a KD / a standing constraint, the question names its route; the answer authorises the change, and the route runs regardless. Detection is deliberately coarse — ask "what is this question ABOUT?", never "who asked" or "which step".

## Reviewer reuse — contract (SKILL.md Step 10)

A WARM reviewer (resumed agent) carries its prior rounds in context. That memory is an asset for exactly one job: re-verifying the fixes of its OWN earlier findings against the fix diff. For anything else it is anchoring — a warm round judging a new group's diff or an amended artefact re-derives nothing and sees what it expects. Rule: warm resume is legal only when the round's whole scope is fix-verification of that reviewer's own register rows; a round containing ANY new material spawns cold. The register (not the reviewer's memory) is the loop's durable cross-round state, so a cold spawn loses nothing the harness relies on.

## In-flight marker — handback vocabulary addendum

`awaiting delegate return` is a LEGAL handback reason: a turn that spawned a background delegate and ends while it runs has genuinely handed the wheel — not to the user, but out of the orchestrator's hands; the task-notification resumes it. Measured in the first post-forge run: the hook fired once, on exactly this shape, and the token resolved it at the cost of one line. That is the intended failure direction (loud + cheap), not a defect.
