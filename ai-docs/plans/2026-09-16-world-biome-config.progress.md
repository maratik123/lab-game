# Progress: World and biome config: generation inputs, lexicon, bestiary — ACTIVE
_Updated: 2026-09-16 04:30_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-16-world-biome-config
**base_commit:** da3e6a8e755fb91129f795e2b73aa4cccf828b19
**Last build:** PASS

**Issue:** #28
**Spec:** ai-docs/plans/2026-09-16-world-biome-config.spec.md

**current_step:** Step 8 — Group A spawning (subtasks 1-8)
**last_passed_gate:** go build ./... | 2026-09-16T07:28:50Z | 986d240
**entry_args:** 28

## Next action

**Do this immediately:** spawn Group A (subtasks 1–8) through `/context-reset` into `code-writer`, per the design's `## Handoff plan`; no inline model override — the subagent's sonnet/medium is frontmatter-pinned.

## Subtasks

- [ ] 1. Biome resource kinds in the ledger — migration 00008, Go mirror, member accessor  ← CURRENT
- [ ] 2. Lift the YAML schema walker into its own file; add the non-scalar leaf kind
- [ ] 3. World value types + scalar half of the world schema (id, seed, generation inputs, k)
- [ ] 4. Content half of the world schema (resource profile, naming style, lexicon, bestiary)
- [ ] 5. Author the MVP world; delete the placeholder; re-word both falsified `.env.example` clauses
- [ ] 6. World-set loader and wiring; `resolveWorldPath` to directory-only; `Config.WorldPath` removed
- [ ] 7. Remove the chunk radius from the balance side
- [ ] 8. Tracked-set gates + the CODE half of the AC3 propagation sweep
- [ ] 9. Design corpus (`~/lab-private`): promote the sketch, amend the sketch-count sentence, re-file §16 item 5
- [ ] 10. Key decisions: new entries and the KD amendments
- [ ] 11. Documentation half of the AC3 sweep over `ai-docs/**`

## Decisions log

- **Step 7**: design-review reached GO on round 3; rounds 1 and 2 returned ITERATE. All four GO notes and both recommendations are design-internal — none met a spec-amending trigger — so they were folded by `design-writer` and design-review did not run again, per the Step 7 table.
- **Step 7**: the owner's three answers (round 3 of `prior_qa`) were recorded in the design under the decision each question's stated route named — D8, D13, D3 — and added no spec row. Verified independently: the spec is byte-unchanged across all five design rounds.
- **Step 7**: the orchestrator's fourth `design-writer` pass carried the owner's decisions rather than review findings, and was not counted against the design-review round cap; the cap is attached to design-review cycles. Surfaced to the owner as an interpretation open to correction.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 3 | Decomposition subtask 8: "Two sites it reaches are assessed still-true" — a count of grep-reached sites | design-internal | folded | design § Decomposition subtask 8 @ da3e6a8 |
| G2 | 3 | Subtask 11 names two sweep sites I could not resolve to a falsified claim | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |
| G3 | 3 | D6's byte-for-byte lift list omits `schemaNode` | design-internal | folded | design § Key decisions D6 @ da3e6a8 |
| G4 | 3 | Subtask 6 says "the new `Config` field" without stating whether `Config.WorldPath` survives | design-internal | folded | design § Key decisions D1 and Decomposition subtask 6 @ da3e6a8 |
| G5 | 3 | Note 2 is a decomposition-accuracy item, not a scope change | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |
| G6 | 3 | `ai-docs/deferred/_inbox.jsonl:31` is answered by this task, but that file is written only by Step 12 and `/triage` | design-internal | folded | design § Decomposition subtask 11 @ da3e6a8 |

## Key discoveries (don't re-investigate)

- The spec-anchor gate resolves `[task: …]` and `[answer N.n: …]` by CONTENT against the interview state file, not by presence — proven by four controls inside the Acceptance Criteria table. Probes placed in `## Open questions` are invisible: the gate's parser judges only `scope`, `kd` and `ac` sections.
- The design corpus `~/lab-private` is a separate repository, outside the PR and outside CI. Subtask 9 edits it in place with `git -C ~/lab-private`. Verified untouched by every design round so far.
- `go.yaml.in/yaml/v3 v3.0.5` refuses an out-of-range value into an `int32` destination and accepts it silently into `int` — the reason D4 binds the radius through `int32` and no G115 conversion exists.
- The owner's three round-3 answers are in the interview state file's `prior_qa` as round 3; design tags `[answer 3.1]`–`[answer 3.3]` resolve against them.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- (none yet — Step 8 not started)
