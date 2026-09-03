# Progress: Rename the `design` Subagent — ACTIVE
_Updated: 2026-09-03 02:44_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-03-rename-design-subagent
**base_commit:** f00871d
**Last build:** PASS
**Issue:** #10
**Spec:** ai-docs/plans/2026-09-03-rename-design-subagent.spec.md
**current_step:** Step 9 — Verify (ALL PASS)
**last_passed_gate:** golangci-lint run | 2026-09-03T00:06:28Z | 4f88a69
**entry_args:** 10

## Next action

**Do this immediately:** Group A is complete. Resume `/task` at Step 9 (verify) — AC3/AC4 must be re-run on the terminal tree per § Risks R4, and AC12's Checklist O re-run is orchestrator-side.

## Subtasks

- [x] 1. `git mv` the definition to `.claude/agents/design-writer.md`; `name: design-writer`; retitle H1; fix its self-referential agent list. Small content delta so the commit records a rename.
- [x] 2. Task/Design sync group — dispatch examples, Step-6 heading/body, Design-Amendment prose + anti-pattern rows, handoff triggers, quality-gate enumerations, coordinate-drift ownership rows, `design-review`'s frontmatter description.
- [x] 3. Remaining Subagent definitions: `spec-writer`, `self-review`, `self-reflect`, `self-improve`.
- [x] 4. Remaining Skills: `interview`, `pr-commented`, `pr-ci-failed`, `main-ci-failed` (SKILL + reference). Most sites sit OUTSIDE the Spec-Amendment recipe — sweep each file whole.
- [x] 5. `ai-docs/` inventory pages: `claude-tools-hierarchy.md`, `propagation-groups.md`, `improve-eval-contract.md`.
- [x] 6. Rewrite Checklist O's severity rule to one rule, any axis `major`: `:152` justification, `:174` presuppositions, `:184` untouched, delete `:186`, add worked example.
- [x] 7. New dated KD section in `ai-docs/key-decisions.md`.
- [x] 8. Closing concept-level re-sweep of the live tree. Re-derive the class from the spec's § Scope tables, NEVER from subtasks 1–7's edit log.

## Decisions log

- **Step 6**: design resolved the spec's three open questions — Checklist O carries a worked example; the rationale lands in both `key-decisions.md` and the checklist prose; a second embedded-name clash is deferred to its own issue.
- **Step 7 round 1**: ITERATE. AC4 found unsatisfiable (major) → routed through the Spec Amendment recipe with owner approval, mirroring AC3's history-surface exclusion. Four further findings fixed design-side.
- **Step 7 round 2**: GO with five notes and two recommendations, all classified design-internal and folded in without a further round. AC9-vs-AC11 checked against the spec's `:177` disposition row before accepting the reviewer's permissive reading.
- **Step 7**: design's rebuttal of the round-1 reviewer's `ci.yml` line-pin "correction" verified and upheld — `:152` is the guard-suites step; the correction would have introduced the drift it claimed to fix.
- **Step 8**: gate reachability settled without widening permissions — AC7, AC8 and AC12's script half are discharged by CI's *Harness guards* job, which `paths-filter` reaches on this diff.
- **Step 8 subtask 1**: three IN-class sites in the renamed file, per a whole-file `grep -niw design` read — frontmatter `name:`, the H1, and sub-point (g)'s `design` / `design-review` / `self-review` / `spec-writer` enumeration. `Designer Subagent.` at `:9` stays (role noun, per the design's judgement call), and every other token is the design *document*, the design *phase*, `docs/DESIGN.md`, or ordinary English. `git status` records `RM`, so AC2's rename continuity holds.
- **Step 8 subtask 2**: two sites beyond the design's row-2 enumeration were found by sweeping each file whole and are IN class — `task/reference.md:183`'s second quality-gate enumeration (`design` / `design-review` / `self-review` quality gates), and `task/reference.md:314`'s "the design agent raised genuine scope questions" in the Amendment-route recurrence history, which names the agent in bare prose. OUT and left alone: every "design-defined group", "the design's `## Handoff plan`", "Step 6 design → Step 7 design-review" phase pairing, `### Step 7: Design review`, `# Design Review Subagent`, and the `description:` phase chain "interview → spec → design → design-review" in `task/SKILL.md`'s frontmatter. `### Step 6: Design Subagent` → `### Step 6: Design-Writer Subagent` per the design's judgement call (it names the agent, and nothing links to the anchor).
- **Step 8 subtask 3**: one unbackticked coin-flip resolved IN — `spec-writer.md`'s "take the default and let design choose otherwise via Design Amendment". Ruled IN because the construction is an *actor* one ("let X choose"), and the two sentences before it name that actor as the Subagent; the spec's OUT table's phase shapes are all noun phrases ("the design phase", "`design` rounds", "interview → spec → design → design-review"). The mirror-image shape is ruled OUT for the same reason: `self-review.md:152`'s "re-run design-review (and design, for spec amendments)" and `task/SKILL.md:206`'s "re-run Step 6 design → Step 7 design-review" are step pairings, not actors, and both stay `design`.
- **Step 8 subtask 4**: sweeping whole files past the Spec-Amendment recipe paid twice — `pr-commented/SKILL.md:316` carries a *second* agent enumeration in its trailing clause ("which spawns the `spec-writer` / `design` / `design-review` Subagents") after the one in its lead sentence, and `pr-commented/SKILL.md:236`'s binding spec-touching-round rule names the agent in both its bold lead and its body. OUT and left alone: `main-ci-failed/SKILL.md:239`'s "record the design / design-review verdicts" (a step pairing, same shape as `task/SKILL.md:206`), `pr-commented/SKILL.md:25`'s "(Question 3 of design)" (the design document), and every `/task` design-review bail route.
- **Step 8 subtask 5**: AC6's two files are done — `claude-tools-hierarchy.md`'s Subagent-table row is re-keyed and its `design-review` neighbour's "Loops with" clause follows it; `propagation-groups.md`'s Task/Design anchor row, its reverse row, and the domain-invariant row are all keyed by file path, so each carries the new path. The table's row order is deliberately left alone (it is workflow order, not alphabetical). OUT: `claude-tools-hierarchy.md:76`'s "the design-system skill" names a source-harness skill this project does not have.
- **Step 8 subtask 6**: the worked example was drafted naming the old definition path and was rewritten to say "this project's design Subagent" instead — spelling the old definition path inside `.claude/**` would have failed AC3 outright. The example's demonstrator is a two-column table ("What the clash did not do" / "What it did") rather than a fenced block, and the closing sentence states the ground positively rather than quoting the deleted carve-out, so the section-scoped `cross-axis|dispatch time|dispatches through different tools|not automatically|minor` grep leaves exactly the one hit AC11 requires. Audit sync group re-swept after the edit: the only sibling hit is `ai-audit/SKILL.md:116`, which names no severity and no axis and whose anchor link still resolves — no sibling edit, as the design predicted.
- **Step 8 subtask 6 (finding, no action taken)**: the design's R7 assumes Checklist M governs this rewrite, but `checklist-m.md`'s audited corpus is `AGENTS.md` + every `.claude/skills/**/SKILL.md` + every `.claude/agents/**.md` + four `ai-docs/` pages + `.claude/rules/**`; `.claude/skills/ai-audit/reference.md` is a `reference.md`, so sub-checks 2 and 6 never reach it. The mitigation was applied anyway (one bold-uppercase verb in the lead paragraph, a demonstrator two lines below it) because it costs nothing and the prose is better for it. No design edit — R7's mitigation is satisfiable either way, so nothing in the design is falsified for this run.
- **Step 8 subtask 7**: KD-21 lands in a new trailing `## Harness naming (2026-09-03)` section rather than under `## Repository and harness`, so the KD numbers keep ascending in reading order (the last existing section is `## Ledger core (2026-09-02)` holding KD-17–20). The row records the *decision* and points at Checklist O for the reader-vs-parser *argument* — no duplication, per the design's "both surfaces, different jobs" call. The old definition path is deliberately not spelled anywhere in the row: `ai-docs/**` is inside AC3's glob.
- **Step 8 subtask 8 — cold re-sweep, method**: LIVE was rebuilt from the spec's two § Scope tables and the § Out of scope exclusion list, not from subtasks 1–7's edit log — `git ls-files` minus `docs/`, `ai-docs/learnings.md`, `ai-docs/harness-gaps.md`, `ai-docs/plans/`, `ai-docs/metrics/`, `ai-docs/deferred/` (106 tracked paths). `grep -niw design` over that set returned 392 lines, triaged by referent rather than by tally, plus two shape sweeps aimed at the two failure modes the design names: a high-signal one for the IN shapes (backticked bare token, `agents/design` path, `subagent_type` value, "design Subagent"/"design agent") and a co-occurrence one for the under-reach shape — a bare stem on a line that also names another agent, which is how `self-improve.md:69` had hidden from the design's own shape pass.
- **Step 8 subtask 8 — result**: **no missed IN-class site.** AC3 and AC4 are both empty over LIVE. Three `` `design` `` tokens survive in LIVE and all three are correct: `ai-audit/reference.md:154` and `key-decisions.md:57` quote the *old* frontmatter value and the *embedded* Skill's name (both are the point of the worked example and the KD row), and `ai-docs/task-run-schema.md:335`'s "`design` rounds" is the round name the spec's OUT table names verbatim. Other OUT verdicts recorded so they are not re-examined: `triage-runner.md:85`'s wrapped "by design."; `task/scripts/test-append-task-run.sh`'s `design=` shell variable (it holds the *schema document's* path); `append-task-run.sh:35`'s "/interview, design and design-review rounds" (round names, same shape as `task-run-schema.md:335`); `templates/improve-eval-reproducer.md:74,87,89`'s "design → design-review re-loop" (step pairing); and every `docs/DESIGN.md`, `*.design.md`, "the design document", "Design conformance", "API design", "by design" and "design-affecting" hit across `AGENTS.md`, `ai-docs/context.md`, `instruction-file-validation.md`, `review-findings.md`, `code-writer.md` and the Go tree.
- **Step 8 subtask 8 — the in-flight sites that are NOT defects**: this run's own `*.spec.md`, `*.design.md` and `*.spec.md.state.md` still carry `.claude/agents/design.md` and `subagent_type="design"`, and must, to stay intelligible. They are pruned from LIVE and reach `ai-docs/plans/done/**` (spec + design) or gitignored `ignored/` (progress + state) at Step 12, which is when AC3 and AC4 are measured — § Risks R4. `ai-docs/plans/INDEX.md`, the only other tracked live file under `ai-docs/plans/`, carries neither literal. `ai-docs/harness-gaps.md:110,117`'s stale `target:` paths are left alone on purpose, per the design.
- **Step 8 subtask 8 — collateral checks**: no `design-review` site was rewritten into `design-writer-review` or similar (grep empty); both `subagent_type="design-review"` dispatch sites in `task/reference.md` survive; `git diff -M --summary f00871d...HEAD -- .claude/agents/` prints the rename at 98% similarity and no delete/create pair (AC2); `git diff --name-only f00871d...HEAD` contains no `cmd/**`, `internal/**`, `docs/**`, `.go`, `.sql`, `go.mod` or `go.sum` path (AC13); `go build ./...` green.

## Key discoveries (don't re-investigate)

- The dispatchable `subagent_type` set is session state, loaded at session start. `design-writer` becomes dispatchable only in a later session; the old name keeps resolving in this one. Neither is a defect, and no verification step may depend on dispatching the new name from the session that creates it.
- AC3 and AC4 are **terminal-tree** criteria. This run's own spec and design contain both forbidden literals and only reach the excluded `ai-docs/plans/done/**` at Step 12 — so their commands run AFTER Step 12's `git mv`, not at Step 9.
- `ls` and `comm` are granted in neither `.claude/settings.json` `permissions.allow` nor `/task`'s `allowed-tools`. `git`, `grep`, `awk` and `jq` are. Use `git ls-files` for existence checks and `grep -Fxf` for intersections.
- The token `design` has five referents in this tree; only the Subagent renames. A bare grep count is not the boundary — the spec's § Scope membership tables are.
- `check-citations.sh` checks `#N`/date namespaces only and excludes `ai-docs/plans/**`, so the deliberately-stale `target:` paths at `ai-docs/harness-gaps.md:110,117` cannot fail AC8. They are left alone on purpose.

- **The "registration is session state" constraint is FALSE in this harness — measured, not inferred.** The spec's § Technical constraints and the design's R6 both assert that the new `subagent_type` becomes dispatchable only in a session started after the rename lands, and that the old name keeps resolving for the rest of the current one. After subtask 1's commit (`cd8869c`) this session's agent registry updated live: `design-writer` became available and `design` was removed, with no restart. Both halves of the claim are false. Nothing in the implementation depends on it — the design deliberately built no verification step on dispatching the new name — so it is a conservative falsehood, not a broken deliverable. It nonetheless ships in the PR inside two artefacts. Step 10 self-review rules on whether it warrants a spec/design amendment.

## AC Status

| # | Criterion | Test / Verification | Status |
|---|-----------|---------------------|--------|
| AC1 | new file exists, old gone, `name:` matches basename | `git ls-files .claude/agents/` + `head -3` → `name: design-writer` | PASS |
| AC2 | recorded as a rename, not delete+add | `git diff -M --summary f00871d...HEAD` → `rename … (98%)` | PASS |
| AC3 | no `.claude/agents/design.md` literal | grep over 109 tracked paths, run's own artefacts pruned | PASS on LIVE — re-run on the terminal tree after Step 12 |
| AC4 | no `subagent_type="design"` literal | same 109-path sweep | PASS on LIVE — re-run on the terminal tree after Step 12 |
| AC5 | every live in-class site renamed; OUT-of-class untouched | six in-class shape greps → 0 files; three residual `\`design\`` hits adjudicated individually | PASS |
| AC6 | `design-writer` in both inventory pages | `grep -n design-writer ai-docs/{claude-tools-hierarchy,propagation-groups}.md` | PASS |
| AC7 | every relative markdown link resolves | the CI *Harness guards* python3 check, run verbatim locally, after the last edit | PASS |
| AC8 | citation invariant + its regression suite | `check-citations.sh` → `PASS`; `test-check-citations.sh`, `test-append-task-run.sh`, `test-piped-gate-guard.sh` → all green | PASS |
| AC9 | exactly one severity rule, `major`, any axis | `grep -niE 'cross-axis\|dispatch time\|not automatically\|minor\|serious case'` scoped to §  Checklist O → one hit, the instrument-coverage clause AC11 requires | PASS |
| AC10 | no dispatch-ambiguity justification | same scoped sweep → no `dispatch time` hit; intro states the reader/model ground | PASS |
| AC11 | four sites match their disposition rows | `:152` rewritten, `:174` keeps all three procedural claims and loses both severity presuppositions, `:184` intact, `:186` deleted | PASS |
| AC12 | Checklist O re-run, embedded list non-empty | 26 project names vs 31 embedded; `grep -Fxf` intersection empty; `design-writer` unmatched; project no longer defines `design` | PASS |
| AC13 | no Go / SQL / docs file touched | `git diff --name-only main...HEAD` filtered | PASS |

Gates: `go build` GREEN · `go vet` GREEN · `go test ./...` GREEN · `golangci-lint fmt -d` no diff · `golangci-lint run` 0 issues. `shellcheck` / `actionlint` / `go mod tidy` N/A — no `.sh`, `.yml` or module change. Panic-index N/A (no Go). Domain-invariant sweep N/A (no ledger/scheduler/Telegram code).

- **Step 9**: AC7 and AC8 ran locally and passed — the design predicted both were unreachable without a permission grant and routed them to CI. The grant was never needed; local green is the stronger evidence and CI still re-checks on the PR.
- **Step 9**: KD-21 in `ai-docs/key-decisions.md` carried the falsified session-state claim verbatim from the spec; corrected in place to say the registry refreshed mid-session. `key-decisions.md` is neither spec nor design, so this was an ordinary edit, not an amendment.

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| D1-1 | design round 1 | major | fixed@9a38af1 — AC4 amended to mirror AC3's exclusion | `grep -n '^| AC4 ' ai-docs/plans/2026-09-03-rename-design-subagent.spec.md` |
| D1-2 | design round 1 | major | fixed@0bc14a4 — Audit sync group named, discharged by inspection | `grep -n 'Audit sync group' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D1-3 | design round 1 | minor | fixed@0bc14a4 — both measurement tags re-transcribed | `grep -c '08271a3' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D1-4 | design round 1 | note | fixed@0bc14a4 — § Decomposition rows tagged | `grep -n 'Where rows 2–5' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D1-5 | design round 1 | minor | fixed@0bc14a4 — AC1 uses `git ls-files`, not ungranted `ls` | `grep -n 'git ls-files' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D2-1 | design round 2 | note | fixed@f00871d — four other triggered sync groups recorded as inspected/OUT | `grep -n 'Reflect' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D2-2 | design round 2 | minor | fixed@f00871d — self-certifying sentences deleted | `grep -c 'was re-read at base' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D2-3 | design round 2 | minor | fixed@f00871d — out-of-remit prose positions removed, not re-measured | `grep -c 'the mark is at or above' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D2-4 | design round 2 | minor | fixed@f00871d — `#step-6` tag narrowed to the anchor form | `grep -n 'step-6' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
| D2-5 | design round 2 | note | fixed@f00871d — decomposition rows 2 and 4 widened past the recipe | `grep -n 'do not stop at the recipe' ai-docs/plans/2026-09-03-rename-design-subagent.design.md` |
