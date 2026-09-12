# Progress: Split the CI Test job into four parallel jobs — ACTIVE
_Updated: 2026-09-12 02:15_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-12-split-ci-test-job
**base_commit:** ad9414eaf672116d4f67b3989d3a53246d402af3
**Last build:** not run

**Issue:** #99
**Spec:** ai-docs/plans/2026-09-12-split-ci-test-job.spec.md

**current_step:** Step 8 — entering Group A (subtask 1 of 7)
**last_passed_gate:** check-spec-anchors.sh + check-spec-shape.sh + check-ac-shape.sh | 2026-09-12T02:05:00Z | 65af2b051793e6f39c5e6ba4c268a7b8b1b3c4ec
**entry_args:** ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback

## Next action

**Do this immediately:** `.github/workflows/ci.yml` — subtask 1: replace the single `test` job with the four sibling jobs of the design's § Approach table, then run that subtask's own gate list before `git add`.

## Subtasks

- [ ] 1. `.github/workflows/ci.yml` — four sibling jobs, comment rewrites, cluster comment  ← CURRENT (Group A)
- [ ] 2. `AGENTS.md` — ratchet AXIOM + § Build & Test gate enumeration (Group B)
- [ ] 3. `ai-docs/claude-tools-hierarchy.md` — CI job table (Group B)
- [ ] 4. `ai-docs/go-test-conventions.md` — the fallback gate's CI sentence (Group B)
- [ ] 5. `.claude/skills/pr-ci-failed/SKILL.md` + CI sync-group siblings (Group B)
- [ ] 6. `ai-docs/context-status.md` — the shared-test-server entry's falsified sentence (Group B)
- [ ] 7. AC4 falsified-claim sweep with its positive control (Group B, terminal)

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 4 (interview)**: tracking issue #99 created from the approved spec rather than found — no open issue concerned CI wall-clock; `issue_ref` in the state file stays `free-text` because the entry mode was free text and the spec's anchors resolve against its `task_description` block.
- **Step 7**: design-review round 1 returned ITERATE; the orchestrator re-ran the major finding's three load-bearing measurements before forwarding it, and all three resolved.
- **Step 7**: design-review round 2 returned GO; its one spec-amending note was routed to the owner, who chose "fix the design only", which is also his per-instance exemption from a re-review. design-review did not run again.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | "AC3 is discharged as paths-filter parity, and that reading is never stated." | spec-amending (b) | owner (2) design only | design § Approach — *What AC3's "set of changes" is, and what it is not* + § Test Design *gating parity* @ 65af2b0; answer 3.1 |
| G2 | 2 | "The 'jobs run in parallel by default' claim is load-bearing for AC2 and rests only on a `docs.github.com` quotation with a bare URL" | design-internal | folded | design § Approach parallelism paragraph — in-repo run observation primary, docs quote corroboration, `fetched 2026-09-12` on all three external URLs @ 65af2b0 |
| G3 | 2 | "Subtask 7 routes a falsified site in a code change-type file to the orchestrator 'to be routed back to a code group or ruled out of scope'." | design-internal | folded | design § Decomposition row 7 + § Test Design failure scenario + § Handoff plan contingent third group @ 65af2b0 |
| G4 | 2 | "'Every claim below is about something this task creates.' That last sentence certifies a property over the section's own claims" | design-internal | folded | design § Test Design preamble — sentence deleted, `actionlint` tag's justification rewritten @ 65af2b0 |
| G5 | 2 | "Cite `ai-docs/propagation-groups.md:30` beside the row-28/29 citation." | design-internal | folded | design § Decomposition, after the table @ 65af2b0 |
| G6 | 2 | "Subtask 1 says 'add the cluster comment' without saying where it lands." | design-internal | folded | design § Decomposition row 1 — immediately above the first of the four job declarations @ 65af2b0 |
| G7 | 2 | "Group A will run `go test ./...` for a YAML-only subtask under `code-writer` Mode A step 3." | design-internal | folded | design § Handoff plan, Group A gate list @ 65af2b0 |

## Key discoveries (don't re-investigate)

- **The current `test` job has a second, implicit suppressor.** No `continue-on-error` anywhere in the workflow and every `if:` is job-level, so today a red `make test` step means the three later steps never execute. The split removes that suppression — which is information the run gains, not a regression. The owner ruled this a design matter, not an AC3 rewording.
- **Sibling jobs of `changes` are already observed concurrent in this repo's own CI.** The jobs that ran started within a second of one another after `changes` completed, and overlapped. Two of the reviewer's figures were wrong and were corrected from the run itself: `Lint` started one second later than the others, and `Actionlint` was skipped rather than started.
- **`ai-docs/context-status.md` is a live document for propagation purposes, not a history surface.** The Propagation Rule's carve-out names only `ai-docs/learnings.md` and `ai-docs/plans/done/**`; a prior commit corrected a false claim inside an existing entry of this file in place; and Step 12 sub-step 10a mandates an in-place edit of it. "Append-only" governs how the log grows, not whether a false sentence inside an entry may be fixed.
- **`ai-docs/key-decisions.md` KD-20's "run as its own CI step" survives the split** — it stays true and names no job, which is what separates it from the `context-status.md` sentence that does name one.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- (none yet)
