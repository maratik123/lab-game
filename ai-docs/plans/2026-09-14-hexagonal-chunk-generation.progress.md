# Progress: World generation rework — hexagonal chunks on a super-lattice — ACTIVE
_Updated: 2026-09-14 05:06_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-hexagonal-chunk-generation
**base_commit:** ed9a4baaaffb0d3243f1c73ae6304a77a11cfe5b
**Last build:** PASS

**Issue:** #119
**Spec:** ai-docs/plans/2026-09-14-hexagonal-chunk-generation.spec.md
**Design:** ai-docs/plans/2026-09-14-hexagonal-chunk-generation.design.md

**current_step:** Step 8 — subtask 11 of 11 complete; Group A design gaps fixed at 761efa5
**last_passed_gate:** golangci-lint run | 2026-09-14T02:24:54Z | 761efa5b08c94cc7e5fa7ed93048481feb877a28

**entry_args:** 119

## Next action

**Do this immediately:** Step 9 — run the full verify list, then the per-AC sweep with a verifying command per AC.

## Subtasks

Titles are the design's `## Decomposition` rows, abridged; the design row is the contract.

- [x] 1. `hexgrid`: add the super-lattice beside the rhombic API — `Lattice` and its methods, `Chunk.Neighbor`, cell `Distance`; tests for AC1, AC2, AC3, AC5. (Group A) — 3c119a2
- [x] 2. `maze`: remove the prefab hook on the shipped core; the no-plug-in guard; re-mint `cells.golden`. (Group A)
- [x] 3. `maze` onto hexagonal chunks, portal count still one-or-two; `Params.Radius` with `MinRadius`; delete the rhombic `hexgrid` API. (Group A)
- [x] 4. `maze`: the portal rule — shares, count range, non-touching placement; AC6, AC7, AC8's pair-level clause. (Group A)
- [x] 5. `maze`: the chunk-level core — `ChunkType`, `Version`, `Map`, `Generate(ch, typ)`, `CellSeed`; AC8's type clause. (Group A)
- [x] 6. `maze`: stored neighbours — `NewMap` and `neighbors`; AC14; AC8's stored-neighbour clause. (Group A)
- [x] 7. `maze`: goldens in their final shape — `chunks.golden`; `derive.golden` byte-identical or stop and report. (Group A)
- [x] 8. Configuration — `world.chunk.radius`; `want` formatted from `maze.MinRadius`; drop `bindInt`'s `//nolint:unparam`. (Group A)
- [x] 9. The code-surface sweep (AC18) — closes Group A. (Group A)
- [x] 10. Revise KD-37…KD-40 (AC17). (Group B)
- [x] 11. The prose-surface sweep (AC18), including `docs/world-topology-redesign-plan.md`'s parallelogram-chunk sentence, rewritten in Russian. (Group B)

## Decisions log

- **Step 1–5**: spec approved after interview rounds 1–4; the storage-form scope widening chosen in round 2 was rolled back by the owner in round 3 (state file `prior_qa` 3.2).
- **Step 7**: design-review round 1 ITERATE. The owner answered the routed questions in `prior_qa` round 5: AC4 and AC1–AC3 stay as written, with no engineering for unreachable extremes (5.1, 5.2); AC18 amended to its outcome at 3fff413 (5.3); `docs/world-topology-redesign-plan.md` ruled live (5.4).
- **Step 7**: the design's `_inbox.jsonl` question — orchestrator ruling: this task neither edits nor routes those rows.
- **Step 7**: design-review round 2 GO with five minor issues and two recommendations, all design-internal; `design-writer` folded them in at ed9a4ba, and design-review did not run again (Step 7 table). The orchestrator read the fold-in diff item by item (§ GO notes).
- **Step 8**: progress file created; in-flight marker created and owned by this session.
- **Step 8, subtask 1**: `hexgrid.Lattice` implements the hexagon-of-hexagons mapping via the inverse super-basis in D3, checked against a bounded four-candidate search. `Border`'s path order was derived analytically for the three canonical directions (a "lone" direction's face at the range's minimum, then the secondary-then-lone pair for every following index) and verified by an independent vertex-sharing predicate in the test, not by re-deriving the order from the enumeration itself.
- **Step 8, subtask 3**: `chunkGraph` now carries a `hexgrid.Lattice` and indexes `LocalCells()` directly, keyed by a `map[hexgrid.Coord]int`, rather than a row-major rectangle. `borderCandidates` derives from `Lattice.Border(dir)` translated by the lesser chunk's own centre — translation is uniform for canonical and opposite directions alike (both reduce to `Center(lesser) + local`), which was verified rather than assumed. The rhombic hex-chunk "diagonal border has exactly one candidate" test category no longer applies: every one of a hex chunk's six neighbours now has the same `2R+1`-face border, so `portal_test.go` was rewritten without it. The golden was re-minted at `MinRadius` (interim shape; subtask 7 moves it to the design's reference radius). One test-only defect found and fixed during this subtask: a flood-fill connectivity test built its region from an arbitrary square coordinate range rather than whole chunks, which is not internally connected at the region's own edge — fixed to build the region from whole chunks via `Lattice.LocalCells`/`At`, matching the pattern the original rhombic test already used. Also observed: `TestCellsGolden` and its cross-check test race on the golden file when both run with `-update-cells` and `t.Parallel()` in the same invocation (pre-existing test-design property, not introduced here) — minting and verifying were run as two separate invocations to avoid it.
- **Step 8, subtask 5**: `Generate` computes a border face's true neighbour chunk via `Lattice.Locate` on the actual global neighbour cell, never via `ch.Neighbor(d)` — a corner cell's face in direction `d` can lead to a DIFFERENT neighbour chunk than the one `d` alone would suggest (verified during subtask 1's `Border` derivation: e.g. a `DirNE`-direction face from a cell on the `DirE` border still crosses into the `DirE` neighbour, not the `DirNE` one). `gateCapacity` (`3R²-3R`) replaces the plain non-border count as the binding capacity check in `Params.validate`, since a gate chunk's capacity is always the smaller one. Every existing dims-shaped test that read faces through the deleted `Cell`/`chunksConsulted` was rewritten against `Generate`/`Map.Faces`, using a per-test `mapCache` helper (`generate_test.go`) that memoizes `Generate` calls per chunk so a multi-coordinate sweep does not rebuild the same chunk repeatedly.
- **Step 8, subtask 6**: `NewMap`'s radius-from-length inference (`radiusForCellCount`) solves `3r²+3r+1=n` via an integer-only Newton's-method square root (`isqrt`), since `internal/detguard` bans the `math` import and floats on this path — no float, no external dependency. Verified live (not merely asserted) that generating a patch of chunks sequentially, each against its already-generated neighbours rebuilt through `NewMap`, yields byte-identical maps to generating the same patch with no neighbour maps at all (`TestGenerate_SequentialAgainstStoredNeighboursMatchesIndependentGeneration`) — this is the load-bearing property #29's later consumer depends on.
- **Step 8, subtask 7**: `chunks.golden` moved to radius 9 (the design's reference value) and gained a version header, per-border sorted portal positions (as 0-based indices into `borderCandidates`' own path order, read back from each chunk's own `Map` via a `facePassageFromChunk` helper that resolves whichever of a border face's two endpoints belongs to the chunk being rendered), and a chunk generated against a stored (`NewMap`-rebuilt) neighbour. `derive.golden` was re-run and confirmed byte-identical — no diff — since nothing in this task changes the key chain itself. The mismatch message names `Version` as the bump target, per the design's own contract for what a diff means.
- **Step 8, subtask 8**: since `ChunkBalance` now has only one int-typed field, the loader test fixtures that exercised a YAML alias and a duplicate key against `world.chunk.cols`/`rows` (a two-key pair) were repointed to `combat.hit_die_sides`/`base_defence` — a still-two-key section — rather than dropped, so both fixtures keep exercising the same document-shape edge case they always did. `go run ./cmd/importguard` was re-run after wiring `internal/config` to `internal/maze`; it stayed green, since the guard forbids only module-path prefixes and `internal/maze`/`internal/hexgrid` carry none that are forbidden (this brings both packages into `cmd/bot`'s dependency graph, an accepted consequence subtask 10 records against KD-38).
- **Step 8, subtask 9**: ran the design's sweep recipe (narrow tier every-hit, broad tier filtered to a chunk/border-mentioning line) over every tracked file outside `*.md`, `docs/**`, `ai-docs/**` and `.claude/**`. Two real hits, both fixed: `internal/maze/island.go`'s unreachable-branch comment described a "1x1, single-row, or single-column chunk" (a rhombic-grid degenerate shape with no hex equivalent — rewritten to name the actual reason the branch is unreachable, `MinRadius` excluding radius 0); `internal/maze/seed.go`'s `cellKey` doc said "never of any chunk dimensions" (rewritten to "the chunk radius"). Every other hit (the current `floorDiv64` helper matching the `floor.?div` pattern; `guards_test.go`'s past-tense description of the deleted prefab hook) was read and judged accurate as a description of the CURRENT generator or of what was deliberately removed, per the recipe's own carve-out — left unchanged. The narrow tier's own literal `floorDiv32` control could not fire, because that identifier no longer exists anywhere in the tree after subtask 3 deleted it outright; the pattern's mechanism was instead confirmed live by its match against `floorDiv64` (a real, current identifier), which is the same regex firing correctly, not a different check.
- **Step 8, subtask 10**: KD-37…KD-40 rewritten in place to describe the core as Group A left it, each with an `*Amended by #119:*` clause naming what the rework replaced and an `*Amended:* 2026-09-14` date. Every claim was read against the code, not copied from the design checklist: `resolveNeighborMaps` checks chunk, radius, adjacency and duplicates but never the version (KD-37's "whatever version that map names"); `go list -deps ./cmd/bot` now lists `internal/maze` and `internal/hexgrid`, and `go run ./cmd/importguard` is green (KD-38); `gateCapacity(6)` is 90 (KD-40); `TestIslandShare_AchievedShareWithinTolerance` still asserts against the achieved share (KD-39's retained sentence). **Three Group A deviations found while verifying, reported to the orchestrator and not edited (Group B edits no code file):** (a) `Params.validate` still carries the `capacity == 0 && IslandShare.IsPositive()` refusal that design D7 says goes away; it is unreachable behind the `MinRadius` check, so KD-40 is worded to hold either way ("guards no reachable case"), not "is gone". (b) `TestGenerate_TakesSharedBorderFromStoredNeighbour` has only the different-seed-and-shares row; the design's § Test Design rows "N rebuilt with one border face flipped" and "N rebuilt under a version other than `Version`" are absent — no test passes `Generate` a `NewMap` map at a version other than the one it was generated under (every test `NewMap` call is in `map_test.go`). AC14's stated condition is still exercised by the first row, but the version-independence KD-37 now records is held by the code alone. (c) `internal/maze/doc.go` says every value is a pure function of the seed, the inputs, the chunk coordinate and its type, omitting the supplied neighbour maps AC12 lists.
- **Step 8, subtask 11**: ran the design's sweep recipe after subtask 10 landed (f9c1c0b), over the 98 tracked prose files (`*.md`, `docs/**`, `ai-docs/**`, `.claude/**`, minus the recipe's history surfaces, this task's spec/state/design and `ai-docs/deferred/**`). All three controls behaved: `floorDiv32` hit the narrow tier, `chunk: cols/rows` counted in the broad tier, and `rows.Err()` did not. Because the recipe's word list does not bound AC18's class, a third probe was added with its own control: per-coordinate, `cells.golden`, `world.chunk.cols/rows`, `Cols`, "one or two portals" in English and Russian, and `Cell`. Four sites were rewritten. `ai-docs/context.md`'s `internal/hexgrid` and `internal/maze` layout entries now name the super-lattice, the chunk type, the portal rule and `Generate`/`Map`/`NewMap`/`Version`, where they had named the floor-division chunk mapping, the chunk-grid distance, the prefab hook and `Cell`; their "no call site reaches yet" became "only the balance loader reaches, to check the chunk radius". `ai-docs/context.md`'s status line got the same change. `ai-docs/code-style.md`'s chunk-size row now reads `world.chunk.radius`, not `world.chunk.cols`/`rows` "until the core moves to hexagonal chunks". The parallelogram-chunk sentence in `docs/world-topology-redesign-plan.md` § «Поправки к исходному саммари» was rewritten in Russian in the past tense. Every remaining hit was read and kept under the recipe's carve-out or as unrelated. This file's own past-tense work records stayed. Line 74 of the redesign plan refers to the old model as old. The KD-38/39/40 amendment clauses and KD-40's "which the rhombic core needed" say what was replaced. `docs/DESIGN.md` and the plan's D-table describe the hexagonal design. `context.md:43` matched only on the `capacity_role` column, `.claude/skills/ai-audit/checklist-m.md:11` on "table rows", and `ai-docs/instruction-file-validation.md:45` names an unrelated example `Cell`. No code-surface hit turned up, and no code file was edited. The relative-link check and `check-citations.sh` are green after the last edit.
- **Step 8 (orchestrator, after Group B)**: Group B reported three places where Group A's code fell short of the design; the orchestrator re-resolved each against the code and the design and swept every test name the design gives, finding four more (AC4 and AC9 tests not driven through `New`/`Generate`, refusal messages not asserted to name their input, `TestCell_*` names left after `Cell`'s deletion). `code-writer` Mode B fixed all seven, committed by the orchestrator at 761efa5. Each new assertion was seen RED against a mutant; the two AC14 subtests were re-probed after the first mutant stopped at the parent test's setup `t.Fatal`. Before that commit the orchestrator ran `go build ./...`, `go vet ./...`, `go test -count=1` over `maze`/`hexgrid`/`config`, `go test -race` over `maze`, `golangci-lint fmt -d` (empty), `golangci-lint run` (0 issues), `make comment-refs` and `make import-guard`, all green; the commit's ratchet held at 91.58%. `TestConnectivity_MultiChunkRegionOverASeedSweep` and the split `TestParams_Validate*` tests are accepted as the design's `TestConnectivity_MultiChunkRegion` and `TestParams_Validate`.

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
| AC4 | PASS (subtask 3, `TestParams_ValidateRadiusBound`) |
| AC5 | PASS (subtask 1) |
| AC6 | PASS (subtask 4, `TestPortals_CountWithinRoundedUpShares`) |
| AC7 | PASS (subtask 4, `TestPortals_NoTwoPortalsOfABorderShareAVertex`) |
| AC8 | PASS — all three clauses (subtasks 4-6, `TestBorder_IndependentOfChunkTypeAndThirdChunks`) |
| AC9 | PASS (subtask 5, `TestSelectIslands_GateChunkNeverSelectsTheCentre`) |
| AC10 | PASS (subtask 5, `TestGenerate_NonIslandCellsConnectedInsideEveryChunk`) |
| AC11 | PASS (subtasks 5-6, `TestCell_ConnectivityOverAMultiChunkRegion` + `TestGenerate_SequentialAgainstStoredNeighboursMatchesIndependentGeneration`) |
| AC12 | PASS (subtask 5, `TestGenerate_SettledByItsInputsAlone` + `TestGuard_ImportsAllowlist`) |
| AC13 | PASS (subtask 5, `TestGenerate_MapNamesGenerationVersion`) |
| AC14 | PASS (subtask 6, `TestGenerate_TakesSharedBorderFromStoredNeighbour`) |
| AC15 | NOT_TESTED |
| AC16 | NOT_TESTED |
| AC17 | PASS (subtask 10, KD-37…KD-40 rewritten; checked item by item against the design's § Instruction subtasks checklist and the code) |
| AC18 | PASS — code-surface sweep (subtask 9) and prose-surface sweep (subtask 11); every remaining hit read and judged a carve-out or unrelated |
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
- internal/hexgrid/chunk.go, chunk_test.go (rhombic Dims/ChunkOf/Origin/Contains deleted)
- internal/maze/chunkgraph.go, params.go, island.go, portal.go, generate.go (rewritten over hexgrid.Lattice; Params.Radius replaces Params.Dims)
- internal/maze/{chunkgraph,params,island,portal,property,golden,generate,algorithms,cycles}_test.go (adapted to the hex lattice)
- internal/maze/params.go, portal.go, generate.go (subtask 4: PortalShareLower/Upper, portalBounds, ceilShare, nonConsecutivePositions bijection)
- internal/maze/params_test.go, portal_test.go, generate_test.go, golden_test.go (subtask 4: portal share fixtures and refusal rows)
- internal/maze/chunktype.go, map.go (new, subtask 5: ChunkType, Version, Map)
- internal/maze/generate.go (subtask 5: Cell/chunksConsulted deleted; Generate(ch,typ) and CellSeed added)
- internal/maze/island.go, params.go (subtask 5: selectIslands takes ChunkType; gateCapacity binds Params.validate)
- internal/maze/{generate,island,property,golden,bench,portal,guards}_test.go (subtask 5: rewritten against Generate/Map.Faces; TestGuard_ImportsAllowlist added)
- internal/maze/map.go (subtask 6: NewMap, radiusForCellCount, isqrt)
- internal/maze/map_test.go (new, subtask 6: NewMap round-trip, refusal rows, copy check)
- internal/maze/generate.go (subtask 6: Generate gains variadic neighbors, resolveNeighborMaps)
- internal/maze/generate_test.go, portal_test.go (subtask 6: AC14, refusal rows, order-independence, AC8's stored-neighbour clause)
- internal/maze/testdata/cells.golden (deleted), testdata/chunks.golden (new, subtask 7: radius 9, version header, per-border portal positions, chunk(1,0) generated against chunk(0,0)'s stored map)
- internal/maze/golden_test.go (subtask 7: rewritten for the final golden shape); bench_test.go (renamed to BenchmarkGenerate/BenchmarkGeneratePerAlgorithm)
- internal/maze/testdata/derive.golden (re-run, confirmed byte-identical — no diff)
- internal/config/balance.go (ChunkBalance{Radius int} replaces Cols/Rows)
- internal/config/balance_load.go (world.chunk.radius entry with a v>=maze.MinRadius predicate; bindInt's now-unused //nolint:unparam removed)
- internal/config/balance_load_test.go (fixture and every cols/rows-referencing case moved to radius; the alias and duplicate-key fixtures repointed to a still-two-key section)
- config/balance.yaml (chunk.cols/rows replaced by chunk.radius: 9)
- internal/maze/island.go (subtask 9 sweep: stale "1x1/single-row/single-column chunk" comment rewritten for the hex lattice)
- internal/maze/seed.go (subtask 9 sweep: "chunk dimensions" → "the chunk radius")
- ai-docs/key-decisions.md (subtask 10: KD-37…KD-40 rewritten for the hexagonal core, each with an `*Amended by #119:*` clause)
- ai-docs/context.md (subtask 11: the `internal/hexgrid` / `internal/maze` layout entries and the status line rewritten for the hexagonal core)
- ai-docs/code-style.md (subtask 11: the chunk-size row names `world.chunk.radius`)
- docs/world-topology-redesign-plan.md (subtask 11: the parallelogram-chunk sentence rewritten in Russian, in the past tense)
