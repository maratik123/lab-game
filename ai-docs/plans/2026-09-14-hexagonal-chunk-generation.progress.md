# Progress: World generation rework — hexagonal chunks on a super-lattice — ACTIVE
_Updated: 2026-09-14 00:33_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-hexagonal-chunk-generation
**base_commit:** ed9a4baaaffb0d3243f1c73ae6304a77a11cfe5b
**Last build:** PASS

**Issue:** #119
**Spec:** ai-docs/plans/2026-09-14-hexagonal-chunk-generation.spec.md
**Design:** ai-docs/plans/2026-09-14-hexagonal-chunk-generation.design.md

**current_step:** Step 8 — subtask 2 of 9 complete
**last_passed_gate:** golangci-lint run ./internal/maze/... | 2026-09-14 | (subtask 2 commit)

**entry_args:** 119

## Next action

**Do this immediately:** Group A — complete subtasks 1–9 of the design's `## Decomposition`, in order, one commit per subtask (design § Handoff plan: code group, `code-writer`).

## Subtasks

Titles are the design's `## Decomposition` rows, abridged; the design row is the contract.

- [x] 1. `hexgrid`: add the super-lattice beside the rhombic API — `Lattice` and its methods, `Chunk.Neighbor`, cell `Distance`; tests for AC1, AC2, AC3, AC5. (Group A) — 3c119a2
- [x] 2. `maze`: remove the prefab hook on the shipped core; the no-plug-in guard; re-mint `cells.golden`. (Group A)
- [ ] 3. `maze` onto hexagonal chunks, portal count still one-or-two; `Params.Radius` with `MinRadius`; delete the rhombic `hexgrid` API. (Group A) ← CURRENT
- [ ] 4. `maze`: the portal rule — shares, count range, non-touching placement; AC6, AC7, AC8's pair-level clause. (Group A)
- [ ] 5. `maze`: the chunk-level core — `ChunkType`, `Version`, `Map`, `Generate(ch, typ)`, `CellSeed`; AC8's type clause. (Group A)
- [ ] 6. `maze`: stored neighbours — `NewMap` and `neighbors`; AC14; AC8's stored-neighbour clause. (Group A)
- [ ] 7. `maze`: goldens in their final shape — `chunks.golden`; `derive.golden` byte-identical or stop and report. (Group A)
- [ ] 8. Configuration — `world.chunk.radius`; `want` formatted from `maze.MinRadius`; drop `bindInt`'s `//nolint:unparam`. (Group A)
- [ ] 9. The code-surface sweep (AC18) — closes Group A. (Group A)
- [ ] 10. Revise KD-37…KD-40 (AC17). (Group B)
- [ ] 11. The prose-surface sweep (AC18), including `docs/world-topology-redesign-plan.md`'s parallelogram-chunk sentence, rewritten in Russian. (Group B)

## Decisions log

- **Step 1–5**: spec approved after interview rounds 1–4; the storage-form scope widening chosen in round 2 was rolled back by the owner in round 3 (state file `prior_qa` 3.2).
- **Step 7**: design-review round 1 ITERATE. The owner answered the routed questions in `prior_qa` round 5: AC4 and AC1–AC3 stay as written, with no engineering for unreachable extremes (5.1, 5.2); AC18 amended to its outcome at 3fff413 (5.3); `docs/world-topology-redesign-plan.md` ruled live (5.4).
- **Step 7**: the design's `_inbox.jsonl` question — orchestrator ruling: this task neither edits nor routes those rows.
- **Step 7**: design-review round 2 GO with five minor issues and two recommendations, all design-internal; `design-writer` folded them in at ed9a4ba, and design-review did not run again (Step 7 table). The orchestrator read the fold-in diff item by item (§ GO notes).
- **Step 8**: progress file created; in-flight marker created and owned by this session.
- **Step 8, subtask 1**: `hexgrid.Lattice` implements the hexagon-of-hexagons mapping via the inverse super-basis in D3, checked against a bounded four-candidate search. `Border`'s path order was derived analytically for the three canonical directions (a "lone" direction's face at the range's minimum, then the secondary-then-lone pair for every following index) and verified by an independent vertex-sharing predicate in the test, not by re-deriving the order from the enumeration itself.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | Subtask 4 owns `TestBorder_IndependentOfChunkTypeAndThirdChunks` (AC8), and the test design reads it "through `Map.Faces` from chunk A … typed fabric vs gate". | design-internal | folded | design § Decomposition rows 4, 5, 6 and § Test Design → Portals @ ed9a4ba |
| G2 | 2 | `TestGenerate_NonIslandCellsConnectedInsideEveryChunk` (AC10) floods "from the centre of a gate chunk … for both types". | design-internal | folded | design § Test Design → `internal/maze` (AC10 row: gate from centre, fabric from a border cell) @ ed9a4ba |
| G3 | 2 | `bindInt` carries `//nolint:unparam // want is always "positive" today …` (`internal/config/balance_load.go`). | design-internal | folded | design § D12 and § Decomposition row 8 @ ed9a4ba |
| G4 | 2 | § Test Design opens with "Every claim in this section is about a test this task writes: `[derived → the test named]`." | design-internal | folded | design § Test Design — sentence deleted @ ed9a4ba |
| G5 | 2 | `[measured 3fff413:internal/detguard/detguard.go · Read …]` and `[measured 3fff413:internal/detguard/doc.go · Read …]` have a commit but no line range. | design-internal | folded | design § D3 and § Risks tags pinned with line ranges @ ed9a4ba |
| G6 | 2 | The fix for issue 1 is the only one that changes what the Group A implementer does. | design-internal | folded | design § Decomposition rows 4, 5, 6 @ ed9a4ba |
| G7 | 2 | **Round-trip required:** before Step 8, update the design doc to incorporate each note/recommendation above. | design-internal | folded | G1–G6 folded @ ed9a4ba |

## Key discoveries (don't re-investigate)

- Owner's scale ruling (`prior_qa` 5.1, 5.2): no design or tests for unreachable extremes — no upper radius bound, the `int32` coordinate seam not designed for; the radius is set deliberately in configuration.
- The owner rolled back "this task defines the stored form" (`prior_qa` 3.2); `NewMap` is an in-memory constructor only — no byte layout, no storage encoding.
- `ai-docs/deferred/_inbox.jsonl` is written only by `/task` Step 12 and `/triage`; the sweep excludes it (design D15).
- `docs/world-topology-redesign-plan.md` is live (`prior_qa` 5.4); `docs/**` is Russian.

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS (subtask 1) |
| AC2 | PASS (subtask 1) |
| AC3 | PASS (subtask 1) |
| AC4 | NOT_TESTED |
| AC5 | PASS (subtask 1) |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |
| AC12 | NOT_TESTED |
| AC13 | NOT_TESTED |
| AC14 | NOT_TESTED |
| AC15 | NOT_TESTED |
| AC16 | NOT_TESTED |
| AC17 | NOT_TESTED |
| AC18 | NOT_TESTED |
| AC19 | PASS (subtask 2, `TestGuard_NoPlugInPoint`) |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- internal/hexgrid/lattice.go (new)
- internal/hexgrid/lattice_test.go (new)
- internal/hexgrid/chunk.go (Chunk.Neighbor added)
- internal/hexgrid/doc.go (package doc names the lattice)
- internal/maze/prefab.go, internal/maze/prefab_test.go (deleted)
- internal/maze/generate.go, algorithm.go, doc.go (prefab hook removed; FaceState loses FaceDeferred)
- internal/maze/generate_test.go, bench_test.go, property_test.go, golden_test.go (New is now 2-arg; golden re-minted)
- internal/maze/guards_test.go (TestGuard_NoPlugInPoint added)
- internal/maze/testdata/cells.golden (re-minted, prefab marker dropped)
