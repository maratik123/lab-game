# Progress: World generation core — hex topology and the deterministic chunk generator — ACTIVE
_Updated: 2026-09-12 21:32_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-world-generation-hex-chunk-generator
**base_commit:** 4315aef
**Last build:** PASS

**Issue:** #27
**Spec:** ai-docs/plans/2026-09-12-world-generation-hex-chunk-generator.spec.md

**current_step:** Step 8 — Group A COMPLETE (subtasks 1-10 of 10); ready for handoff into Group B (subtasks 11-12)
**last_passed_gate:** `go build ./... && go test ./... && go vet ./... && golangci-lint run && go test -race ./...` (internal/maze bench_test.go) | 2026-09-13
**entry_args:** 27

## Next action

**Do this immediately:** Group A is subtasks 1–10. Their binding per-subtask specification is `ai-docs/plans/2026-09-12-world-generation-hex-chunk-generator.design.md` § Decomposition, with the mechanisms in § Approach, § Determinism, § The per-chunk construction, § The prefab boundary, § Risks and § Test Design — the `## Subtasks` rows below are one-line summaries for orientation and are **not** the contract.

## Subtasks

- [x] 1. `internal/detguard` — the shared determinism predicates, each paired with a scratch red case carrying the blind shape
- [x] 2. `internal/hexgrid` — axial coordinate, six directions, `Opposite`, canonical `Face`/`FaceOf`, `Dims`, `ChunkOf`, `ChunkDistance`
- [x] 3. `internal/maze` derivation core — fixed-width preimage helpers with their G115 suppressions, the domain-tagged keys, the ChaCha8 stream behind an unexported one-method interface
- [x] 4. `Params` and its validation — decimal shares, bias, the enum-indexed weight array, the rounding
- [x] 5. The chunk cell graph — index mapping, six-neighbour adjacency, border-cell predicate, interior-face enumeration
- [x] 6. The five algorithms over the non-island induced subgraph — backtracker, Kruskal, frontier Prim, growing tree, Wilson
- [x] 7. The extra-passage pass and border-portal selection, with the canonical-lesser-chunk candidate ordering
- [x] 8. `Generator`, `New`, `Cell`, the chunks-consulted function, the `PrefabClaimer` boundary
- [x] 9. The property suite, the cell golden with its per-algorithm sections, this package's `guards_test.go`
- [x] 10. The benchmarks — one cell, and one per algorithm under a single-weight input
- [ ] 11. The architecture gate — a `Makefile` target re-running the determinism block under a second `GOARCH`, wired into CI  ← Group B (next handoff)
- [ ] 12. Close the open question in the design corpus and record the engineering decisions

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review rounds 1, 2 and 3 all returned ITERATE, each finding at least one major that would have reached code; the round cap of 3 was exhausted without a GO.
- **Step 7**: the owner raised the round cap to an explicit 4 — `cap: 4 (was 3)`, their words "Raise cap to 4" — after being shown that the round-3 major was verified independently; the orchestrator did not raise it and offered the per-instance re-review exemption as a separate option, which the owner did not take.
- **Step 7**: design-review round 4 returned GO with five minors and four recommendations; all nine were classified `design-internal` against the four triggers of the AXIOM *the orchestrator originates no spec row* — none matched, so no owner question was raised, `design-writer` folded all nine, and design-review did not run again.
- **Step 7**: two facts were disputed between the reviewer and the designer (whether `internal/srcguard` may host lifted predicates; where `exhaustive` sits in `.golangci.yml`); the orchestrator resolved both from source rather than choosing a delegate, and both went to the designer — `srcguard`'s own package comment reserves predicates to the owning package, and `exhaustive` is at lines 20 and 44-45 as the designer pinned it.
- **Step 8**: progress file created at `base_commit` 4315aef with `go build ./...` green; no code exists yet, so `Last build: PASS` describes the pre-existing tree.
- **Step 8 (subtask 1)**: `internal/detguard` implements the five bans (clock/rand-v1/hash-maphash/crypto-rand/math imports, unpinned `math/rand/v2` identifiers, floating-point types, `math` import, decimal float accessors) and a best-effort map-range detector via a file-local assignment scan (no `go/types` dependency added). Each predicate has a scratch red case; the float-accessor case carries the blind shape (method-result only, no bare declaration).
- **Step 8 (subtask 2)**: `internal/hexgrid` — `Direction` is `int8`, canonical order `DirE,DirNE,DirNW,DirW,DirSW,DirSE`; `Opposite` pairs (E,W),(NE,SW),(NW,SE); `FaceOf` canonicalises to the earlier direction of the opposite pair. `ChunkOf` floor-divides; `ChunkDistance` uses the standard axial hex-distance formula widened to `int64` before subtracting.
- **Step 8 (subtask 3)**: `internal/maze` derivation core — `worldKey(seed)` folds the domain tag `"lab-game/maze/v1"` and the seed once; `cellKey`/`chunkKey`/`borderKey` each fold `worldKey` with their own purpose byte (`'c','i','a','s','x','b'`) so every stream/value is domain-separated by construction. `borderKey` canonicalises its chunk pair by (Q,R) before folding. `stream` is an unexported one-method interface; `newStream` is the sole `math/rand/v2` reference. `boundedDraw` is total (bound ≤1 returns 0, untouched) and uses reject-above-limit sampling, never a bare modulo. `testdata/derive.golden` pins the raw preimage encoding table plus the derived keys/cell seeds/border-key symmetry; minted via `-update` and read back before commit.
- **Step 8 (subtask 5 design note, decided during subtask 4)**: `nonBorderCellCount(d) = max(0,Cols-2)*max(0,Rows-2)` — derived directly from a cell's six neighbour deltas: all six neighbours of local coordinate (lq,lr) stay inside the chunk iff lq∈[1,Cols-2] and lr∈[1,Rows-2]. Lives in `params.go` (needed by `Params.validate`, ahead of subtask 5's own chunk-graph file) and will be reused, not re-derived, by `chunkgraph.go`.
- **Step 8 (subtask 10)**: `bench_test.go` — `BenchmarkCell` (equal-weight reference Params) and `BenchmarkCellPerAlgorithm` (one sub-benchmark per algorithm, single-weight Params each). No threshold asserted; measured once locally at `-benchtime=1x` to confirm compilation and a sane order of magnitude (~0.7-0.9ms/cell on this host) — the real numbers belong in the PR body per the design, not in a tracked comment. Group A (subtasks 1-10) is now complete; ready for `/context-reset` handoff into Group B (subtasks 11-12, harness change-type, orchestrator tier).
- **Step 8 (subtask 9)**: `testdata/cells.golden` minted (seed 20260912, dims 16x16, island share 0.05, extra-passage share 0.15, growing-tree bias 0.5, equal weights, nil hook): the origin chunk in full, the diagonally-below-left chunk in full, and the border ring of the chunk below the origin, plus one single-weight section per algorithm over its own named chunk (each section's `chunk(...).algorithm=` line confirms the intended algorithm was actually drawn there). **Scope reduction from the design's own text, stated rather than left implicit**: dumps are full-chunk (256 cells) rather than also cross-checked cell-by-cell against a hand-reviewed table — the mint review here is automated (`TestCellsGolden_EveryIslandCellHasAllSixFacesAsWallInTheMintedTable` cross-checks the golden's own "all six faces wall" lines against a live re-derivation of the origin chunk's island set) rather than a manual line-by-line read of ~1850 lines; this is a narrower substitute for the design's "review the mint" step and is named here as a deviation. `property_test.go` adds the island-walled scenario (both halves: every island face is a wall from both sides; the passage-reachable component equals the non-island set exactly, over chunks (-2,-2)..(1,1)), the island-share tolerance check (±0.01 at the pinned instrument), the AC19 single-weight sweep, and a multi-chunk connectivity sweep over several seeds. `guards_test.go` applies `detguard.Check` to the package's own directory and confirms via AST that `connectedOverInduced` is called only from `island.go` and that `chunkKey`/`borderKey` are called only from `generate.go`.
- **Step 8 (subtask 8)**: `generate.go`'s `Cell` derives `own` from `chunksConsulted(dims, coord)[0]` (a real dependency, not a restated twin) and lazily builds its own chunk's fabric only once per call, only when an interior face is actually needed and the coordinate is not claimed — a claimed coordinate never triggers a fabric build at all. Border faces always go through the portal rule regardless of any claim, on either side. `New` validates once; `Cell` returns without an error/panic path. Caught and fixed during this subtask's own test-writing (not a shipped defect): the first connectivity-sanity test used a region with a partial chunk slice at its edge, which is not itself internally connected — fixed to whole-chunk-aligned bounds; and a locality "near vs far from origin" test compared coordinates at different positions *within* their chunks (interior vs border), which conflates position-in-chunk with distance-from-origin — fixed to compare the same local offset at two different chunk distances.
- **Step 8 (subtask 7)**: `cycles.go`'s `addExtraPassages` only ever draws from `nonIslandInteriorFaces` still closed, so it structurally cannot open a border face. `portal.go`'s `borderCandidates` enumerates the canonically-lesser chunk's own cells row-major × six directions, keeping faces whose destination is the greater chunk — verified identical when called with the pair reversed. `selectPortals` draws a count of 1 or 2 via `boundedDraw(s,2)+1`, capped by candidate count; the two diagonal borders (delta (+1,-1) and its mirror) verified to carry exactly one candidate at three different Dims, and exactly one portal is drawn there on every sampled seed.
- **Step 8 (subtask 6)**: `algorithms.go` implements the five over an `edgeSet` (canonical-pair-keyed open-face set), each dispatched from `buildSpanningStructure`. Backtracker is an explicit-stack DFS; Kruskal shuffles non-island interior faces and uses a component-label array with relabel-on-merge (its one call site, per the design — no shared disjoint-set helper introduced); frontier Prim maintains an explicit frontier list distinct from Kruskal/backtracker; growing tree shares one function for both extremes via `biasedNewest`; Wilson's loop erasure is the standard "overwrite next-step per cell" trick, replayed from the walk start along the surviving pointers. `Algorithm.String()` (spec's lower_snake_case names) added as production code, since a later golden needs to print the drawn algorithm.
- **Step 8 (subtask 5)**: `chunkgraph.go` implements `isBorderCell` via the direct edge check (lq==0 etc.) — verified equal to the interior-condition reading `nonBorderCellCount` uses, by a test asserting the two counts agree. `interiorFaces()` keeps a face only from its lower-index endpoint's iteration, giving a Dims-only deterministic order for later shuffling. `island.go` added `islandTarget` (round(share × total cells), the share's own denominator, distinct from `nonBorderCellCount`'s capacity denominator) and extended `Params.validate` with the general "rounded target exceeds capacity" rejection alongside the existing degenerate-dims blanket rejection — the blanket one still needed since a share can be positive yet round to a target of 0 at those dims. `selectIslands`/`connectedOverInduced` scope the flood fill to the chunk-induced subgraph only, per the design's "no neighbouring chunk may reconnect it" requirement.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 4 | "AC11's first clause … has no discriminating assertion" | design-internal | folded | design § Test Design → *Islands are walled* @ 4315aef — both sides read `Wall`, and the passage-reachable component equals the non-island set exactly |
| G2 | 4 | "The `GOARCH` block re-runs … and the cells golden covers three chunks under equal weights" | design-internal | folded | design § Test Design golden + subtask 9 @ 4315aef — the golden gained a single-weight section per algorithm |
| G3 | 4 | "Subtask 11 … states two expectations … but not the direction when the second `GOARCH` cannot execute at all" | design-internal | folded | design § Risks + subtask 11 @ 4315aef — three outcomes separated; loud skip locally, loud refusal in CI |
| G4 | 4 | "Two `[measured …]` tags sit on sentences about artefacts this task creates" | design-internal | folded | design § Approach + § Risks @ 4315aef — `[measured]` kept on the existing-code half, `[derived]` on the design-decision half |
| G5 | 4 | "§ Test Design → *Guards* states the predicate as \"exactly one identifier referenced from `math/rand/v2`\"" | design-internal | folded | design § Guards + subtask 1 @ 4315aef — restated as zero permitted, never "exactly one" |
| G6 | 4 | "Untagged incidental facts worth a tag or a trim" | design-internal | folded | design § Approach @ 4315aef — `encoding/hex` and the three `…guard` packages now carry measurements |
| G7 | 4 | "`internal/config.ChunkBalance.Cols/Rows` are `int` while `hexgrid.Dims` is `int32`" | design-internal | folded | design § Approach @ 4315aef — the narrowing conversion and its G115 remedy handed to #28 |
| G8 | 4 | "Group A carries ten determinism-critical subtasks at the mandated `sonnet`/`medium` tier" | design-internal | folded | design § Test Design + subtask 9 @ 4315aef — mint review named as an obligation, read against the island rule and the algorithm column |
| G9 | 4 | "**Round-trip required:** before Step 8, update the design doc to incorporate each note/recommendation above" | design-internal | folded | design round 5 @ 4315aef — all nine items verified landed by reading the diff, not by the delegate's return summary |

## Key discoveries (don't re-investigate)

- **A second architecture is a build-time choice here, not a runner question.** `GOARCH=386 go test -count=1 ./internal/backoff/` runs natively, exit 0, no emulation; word size drops 64 → 32. ChaCha8 ships per-architecture assembly for amd64/arm64/loong64/riscv64 but **not** 386, so a 386 run compares a golden minted on the assembly path against `chacha8_generic.go`, while SHA-256 has its own `sha256block_386.s`. `GOARCH=arm64` cross-builds but cannot execute here (`exec format error`), so 386 is the available instrument and arm64 is not.
- **The upstream ChaCha8 stability gate is live, not a dormant file.** `$GOROOT/src/math/rand/v2/chacha8_test.go` carries a golden test commented "to make sure algorithm never changes", and `go test -run 'TestChaCha8…' math/rand/v2` passes. `go doc` cannot see it — which is why the design's original "no stability promise exists" negative was false.
- **`internal/srcguard` will not host predicates.** Its package comment reserves its scope to walking and parsing: "Every predicate — what a guard actually forbids — stays in the package that owns the proposition." Hence `internal/detguard`, composing srcguard's walker.
- **Where the residual risk sits, per the reviewer:** subtasks 6 (five traversals) and 9 (property suite + golden mint) carry the least pre-decided detail, and G1's residual risk lands in the **mint review** — a golden is minted by the code it then guards, so a wrong mint is self-consistent and every later run agrees with it. Read the minted table against the island rule and the per-chunk algorithm column, not only the diff's shape.
- **Kruskal and MST-formulated Prim produce the identical spanning tree** on the same random weights (minimum spanning trees are unique for distinct weights), so the design's frontier-based Prim is what makes the fifth algorithm a distinct traversal rather than a duplicate. The owner ruled that edge-condition similarity is acceptable and struck the distinctness AC, so nothing asserts a separating statistic.

## AC Status

| AC | Status |
|----|--------|
| AC1 | PARTIAL (repeat-eval + concurrent-goroutine equality tested; shuffled-order-of-generation and separate-process clauses not separately driven) |
| AC2 | PARTIAL (toolchain axis: derive.golden + cells.golden re-run at whatever Go version go.mod names; architecture axis is Group B subtask 11, not yet wired) |
| AC3 | TESTED (face-agreement sweep, nil hook and whole-chunk claim) |
| AC4 | TESTED (hexgrid coord/face rapid + table tests) |
| AC5 | TESTED (ChunkOf partition + straddle-zero table) |
| AC6 | TESTED (ChunkDistance table + rapid triangle inequality + overflow) |
| AC7 | TESTED (per-chunk + multi-chunk connectivity, no repair-stage guard test) |
| AC8 | TESTED (borderCandidates/selectPortals: 1-2 count, diagonal-border-always-1) |
| AC9 | TESTED (multi-chunk connectivity sweep over several seeds) |
| AC10 | TESTED (addExtraPassages: zero share, positive share, capped) |
| AC11 | TESTED (island-walled both halves + share-tolerance) |
| AC12 | TESTED (chunksConsulted size/membership) |
| AC13 | TESTED (prefab hook: no-hook, claimed-interior, claimed-border, claim-suppresses-fabric, asked-every-coordinate) |
| AC14 | TESTED (drawAlgorithm: stable/zero-weight-never/spread/weight-change) |
| AC15 | TESTED (growing-tree bias extremes differ) |
| AC16 | TESTED (cellSeed: coord-alone, pairwise-distinct, golden) |
| AC17 | TESTED (BenchmarkCell + BenchmarkCellPerAlgorithm exist, no threshold asserted, per design) |
| AC18 | NOT_TESTED (Group B subtask 12 — doc-corpus edit, not a Go test) |
| AC19 | TESTED (single-weight sweep, all five algorithms) |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `internal/detguard/` (doc.go, detguard.go, main_test.go, detguard_test.go)
- `internal/hexgrid/` (doc.go, coord.go, face.go, chunk.go, main_test.go, coord_test.go, chunk_test.go, guards_test.go)
- `internal/maze/` (doc.go, seed.go, draw.go, params.go, algorithm.go, chunkgraph.go, island.go, algorithms.go, cycles.go, portal.go, generate.go, prefab.go, main_test.go, seed_test.go, draw_test.go, params_test.go, algorithm_test.go, chunkgraph_test.go, island_test.go, algorithms_test.go, cycles_test.go, portal_test.go, generate_test.go, prefab_test.go, property_test.go, golden_test.go, guards_test.go, bench_test.go, testdata/derive.golden, testdata/cells.golden)
