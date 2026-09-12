# Progress: World generation core — hex topology and the deterministic chunk generator — ACTIVE
_Updated: 2026-09-12 21:32_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-world-generation-hex-chunk-generator
**base_commit:** 4315aef
**Last build:** PASS

**Issue:** #27
**Spec:** ai-docs/plans/2026-09-12-world-generation-hex-chunk-generator.spec.md

**current_step:** Step 8 — Group A subtask 3 of 10 complete
**last_passed_gate:** `go build ./... && go test ./... && go vet ./... && golangci-lint run` (internal/maze derivation core) | 2026-09-13
**entry_args:** 27

## Next action

**Do this immediately:** Group A is subtasks 1–10. Their binding per-subtask specification is `ai-docs/plans/2026-09-12-world-generation-hex-chunk-generator.design.md` § Decomposition, with the mechanisms in § Approach, § Determinism, § The per-chunk construction, § The prefab boundary, § Risks and § Test Design — the `## Subtasks` rows below are one-line summaries for orientation and are **not** the contract.

## Subtasks

- [x] 1. `internal/detguard` — the shared determinism predicates, each paired with a scratch red case carrying the blind shape
- [x] 2. `internal/hexgrid` — axial coordinate, six directions, `Opposite`, canonical `Face`/`FaceOf`, `Dims`, `ChunkOf`, `ChunkDistance`
- [x] 3. `internal/maze` derivation core — fixed-width preimage helpers with their G115 suppressions, the domain-tagged keys, the ChaCha8 stream behind an unexported one-method interface
- [ ] 4. `Params` and its validation  ← CURRENT — decimal shares, bias, the enum-indexed weight array, the rounding
- [ ] 5. The chunk cell graph — index mapping, six-neighbour adjacency, border-cell predicate, interior-face enumeration
- [ ] 6. The five algorithms over the non-island induced subgraph — backtracker, Kruskal, frontier Prim, growing tree, Wilson
- [ ] 7. The extra-passage pass and border-portal selection, with the canonical-lesser-chunk candidate ordering
- [ ] 8. `Generator`, `New`, `Cell`, the chunks-consulted function, the `PrefabClaimer` boundary
- [ ] 9. The property suite, the cell golden with its per-algorithm sections, this package's `guards_test.go`
- [ ] 10. The benchmarks — one cell, and one per algorithm under a single-weight input
- [ ] 11. The architecture gate — a `Makefile` target re-running the determinism block under a second `GOARCH`, wired into CI
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
| AC12 | NOT_TESTED |
| AC13 | NOT_TESTED |
| AC14 | NOT_TESTED |
| AC15 | NOT_TESTED |
| AC16 | NOT_TESTED |
| AC17 | NOT_TESTED |
| AC18 | NOT_TESTED |
| AC19 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- (none yet — Group A has not been handed off)
