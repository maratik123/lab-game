# Progress: Rename the `design` Subagent — ACTIVE
_Updated: 2026-09-03 02:44_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-03-rename-design-subagent
**base_commit:** f00871d
**Last build:** PASS
**Issue:** #10
**Spec:** ai-docs/plans/2026-09-03-rename-design-subagent.spec.md
**current_step:** Step 8 — subtask 1 of 8 complete
**last_passed_gate:** awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/agents/design-writer.md | 2026-09-02T23:47:03Z | 6b27bab
**entry_args:** 10

## Next action

**Do this immediately:** continue Group A at subtask 2 — the Task/Design sync group.

## Subtasks

- [x] 1. `git mv` the definition to `.claude/agents/design-writer.md`; `name: design-writer`; retitle H1; fix its self-referential agent list. Small content delta so the commit records a rename.
- [ ] 2. Task/Design sync group — dispatch examples, Step-6 heading/body, Design-Amendment prose + anti-pattern rows, handoff triggers, quality-gate enumerations, coordinate-drift ownership rows, `design-review`'s frontmatter description.  ← CURRENT
- [ ] 3. Remaining Subagent definitions: `spec-writer`, `self-review`, `self-reflect`, `self-improve`.
- [ ] 4. Remaining Skills: `interview`, `pr-commented`, `pr-ci-failed`, `main-ci-failed` (SKILL + reference). Most sites sit OUTSIDE the Spec-Amendment recipe — sweep each file whole.
- [ ] 5. `ai-docs/` inventory pages: `claude-tools-hierarchy.md`, `propagation-groups.md`, `improve-eval-contract.md`.
- [ ] 6. Rewrite Checklist O's severity rule to one rule, any axis `major`: `:152` justification, `:174` presuppositions, `:184` untouched, delete `:186`, add worked example.
- [ ] 7. New dated KD section in `ai-docs/key-decisions.md`.
- [ ] 8. Closing concept-level re-sweep of the live tree. Re-derive the class from the spec's § Scope tables, NEVER from subtasks 1–7's edit log.

## Decisions log

- **Step 6**: design resolved the spec's three open questions — Checklist O carries a worked example; the rationale lands in both `key-decisions.md` and the checklist prose; a second embedded-name clash is deferred to its own issue.
- **Step 7 round 1**: ITERATE. AC4 found unsatisfiable (major) → routed through the Spec Amendment recipe with owner approval, mirroring AC3's history-surface exclusion. Four further findings fixed design-side.
- **Step 7 round 2**: GO with five notes and two recommendations, all classified design-internal and folded in without a further round. AC9-vs-AC11 checked against the spec's `:177` disposition row before accepting the reviewer's permissive reading.
- **Step 7**: design's rebuttal of the round-1 reviewer's `ci.yml` line-pin "correction" verified and upheld — `:152` is the guard-suites step; the correction would have introduced the drift it claimed to fix.
- **Step 8**: gate reachability settled without widening permissions — AC7, AC8 and AC12's script half are discharged by CI's *Harness guards* job, which `paths-filter` reaches on this diff.
- **Step 8 subtask 1**: three IN-class sites in the renamed file, per a whole-file `grep -niw design` read — frontmatter `name:`, the H1, and sub-point (g)'s `design` / `design-review` / `self-review` / `spec-writer` enumeration. `Designer Subagent.` at `:9` stays (role noun, per the design's judgement call), and every other token is the design *document*, the design *phase*, `docs/DESIGN.md`, or ordinary English. `git status` records `RM`, so AC2's rename continuity holds.

## Key discoveries (don't re-investigate)

- The dispatchable `subagent_type` set is session state, loaded at session start. `design-writer` becomes dispatchable only in a later session; the old name keeps resolving in this one. Neither is a defect, and no verification step may depend on dispatching the new name from the session that creates it.
- AC3 and AC4 are **terminal-tree** criteria. This run's own spec and design contain both forbidden literals and only reach the excluded `ai-docs/plans/done/**` at Step 12 — so their commands run AFTER Step 12's `git mv`, not at Step 9.
- `ls` and `comm` are granted in neither `.claude/settings.json` `permissions.allow` nor `/task`'s `allowed-tools`. `git`, `grep`, `awk` and `jq` are. Use `git ls-files` for existence checks and `grep -Fxf` for intersections.
- The token `design` has five referents in this tree; only the Subagent renames. A bare grep count is not the boundary — the spec's § Scope membership tables are.
- `check-citations.sh` checks `#N`/date namespaces only and excludes `ai-docs/plans/**`, so the deliberately-stale `target:` paths at `ai-docs/harness-gaps.md:110,117` cannot fail AC8. They are left alone on purpose.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED — terminal-tree, runs after Step 12 |
| AC4 | NOT_TESTED — terminal-tree, runs after Step 12 |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED — CI *Harness guards* |
| AC8 | NOT_TESTED — CI *Harness guards* |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |
| AC12 | NOT_TESTED — orchestrator-side at Step 9 |
| AC13 | NOT_TESTED |

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
