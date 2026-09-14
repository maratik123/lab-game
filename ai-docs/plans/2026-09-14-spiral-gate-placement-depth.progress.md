# Progress: Spiral gate placement and nearest-gate depth — ACTIVE
_Updated: 2026-09-14 10:58_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-spiral-gate-placement-depth
**base_commit:** 360c61d385a81ddc0ec85dd6e83343f66a44d4cc
**Last build:** PASS

**Issue:** #120
**Spec:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.spec.md
**Design:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.design.md

**current_step:** Step 10 — self-review REJECT (Round 2), addressing findings
**last_passed_gate:** golangci-lint run | 2026-09-14T11:29:59Z | f82feb8
**entry_args:** 120

## Next action

**Do this immediately:** Step 11 — verify SR2-1 … SR2-3; route SR2-1 (a `.go` doc comment) to `code-writer` Mode B with a cost-claim sweep of `internal/gate`; correct SR2-2 and SR2-3 in-thread by new append-only entries.

## Subtasks

Groups per the design's `## Handoff plan`.

- [x] 1. Group A — package scaffold and the spiral order (`internal/gate`: doc, errors, spiral, main_test, guards_test, spiral tests) — commit a2fa3d8
- [x] 2. Group A — the next gate and its tests — commit f162d29
- [x] 3. Group A — `Set`, `NewSet`, `Depth`, with the external tests and the internal AC9 test — commit 7f47611
- [x] 4. Group A — the reporting-only benchmark — commit adf4c1c
- [x] 5. Group B — documentation (KD-41, KD-38 consumer sentence, `context.md` layout line) — commit b33e5a4

## Decisions log

- **Steps 1–5**: spec drafted over two interview rounds; owner answers recorded verbatim in the state file's `prior_qa` (k's meaning; ring start mid-side rounding down as the owner's own decision; direction and turn left to the design).
- **Steps 1–5**: orchestrator's round-1 verification of the ring-start question used an invalid regularity instrument (`(q-r) mod 3`); corrected by a coset test before the question reached the owner — logged in `ai-docs/learnings.md` 2026-09-14.
- **Step 6**: design-writer's scratch probe under `tmp/spiralprobe` was walked by `./...` and failed module-wide lint; moved to `tmp/_spiralprobe` with a symlink while the design cited it, deleted once round 2 dropped the citations — recurrence logged in `ai-docs/harness-gaps.md` 2026-09-14.
- **Step 7**: design-review round 1 ITERATE (three major, two minor); round 2 GO with one minor and one recommendation, both design-internal, folded by design-writer at 360c61d; design-review not re-run.
- **Step 8, subtask 1**: the pre-commit comment-refs gate (run over the staged diff) caught a decision-anchor ("D9") in a `//nolint` comment and a package-qualified symbol (`hexgrid.Chunk.Neighbor`) in a test-helper doc comment that `make comment-refs` (run before staging) had not yet seen against these files; both reworded to name no decision id and no cross-package symbol, and the commit succeeded on retry.
- **Step 8, subtask 3**: `TestSetDepth_Table`'s later-ring row (a ring-2 gate nearer in cells than a ring-1 gate, to red-flag a "stop at first ring with a hit" search) was found by a scratch Go program under `tmp/_probe/gatedepth` that brute-forced radius-6 chunk centres for an inversion, then deleted once the concrete cell/gate/distance values were copied into the test; radius-8 gates and two far chunk directions (corner and mid-side) were used the same way for `TestSetDepth_FarBeyondEveryGate`. Both `lowerBound`'s "+R" mutant and the "stop at first hit" mutant were run against `depth.go` and seen to fail the relevant test before being reverted, per the AC9/AC7 mutant notes in the design's Test Design section.
- **Step 8, Group A return**: orchestrator re-validated branch, base_commit and a clean tree, found no scratch Go under `tmp/`, and re-ran `go build ./...`, `go test -count=1 ./internal/gate/` and `golangci-lint run ./internal/gate/...` — green at d70f3b2; wrote two `ai-docs/learnings.md` entries (the comment-reference violation, and the delegate's decision to record it here instead of in the learnings log).
- **Step 8, subtask 5 (KD-41)**: added under a new `## Gate placement (2026-09-14)` section at the end of `ai-docs/key-decisions.md`, recording D1, D2, D4 and D5 (plus D6's build-once note). It states the AC9 search bound as resting on the stop-rule proof and `TestSearch_StaysWithinBound` / `TestSearch_FarGatesAddNoWork`, never on `detguard`, and records the zero `Set`'s `(0, false)`. Every test name KD-41 cites was confirmed to exist by a `^func <name>\(` grep with a negative control. `detguard`'s `"math"` ban, its name-based map-ness comment and `isMapRange`'s three recognised shapes were re-read at 2b76287. KD-41 also says no import-allowlist guard holds `internal/gate`'s import set: `internal/gate`'s tests carry no import check (grep for `Imports|go/parser|ImportDir|allowlist`, with a control).
- **Step 8, subtask 5 (KD-38)**: the consumer sentence now reads that a consumer of a distance, a coordinate vocabulary or the nearest-gate distance depends on `internal/hexgrid`, or on `internal/gate` over it. An `*Amended by #120:*` clause and a source entry were added. `go list -deps` confirms `internal/gate`'s only non-standard dependency is `internal/hexgrid`, and a grep shows no package imports `internal/gate` yet.
- **Step 8, subtask 5 (context.md)**: `internal/gate` added to the layout line after `internal/maze`. **Beyond the design's named line:** the Status "Code:" bullet said the generation core (`hexgrid` and `maze`) "reaches no call site beyond the balance loader's radius check", which became false once `internal/gate` imports `hexgrid`. It now reads "no call site outside the world packages", followed by one sentence naming `internal/gate`. The reasons are the Propagation Rule's step 4 (every live doc must agree) and the #119 precedent, whose PR changed that same bullet. The orchestrator may revert it.
- **Step 8, subtask 5 (propagation grep)**: `grep -rniE 'spiral|nearest[- ]gate|depth gradient'` (control line matched) over `.claude/`, `AGENTS.md`, `ai-docs/`, `README.md`. `context.md` lines 19, 32 and 42 remain true, as do KD-38 and the history surfaces (`harness-gaps.md`, `deferred/_inbox.jsonl`, left untouched). `docs/DESIGN.md` §2.2 fixes no start direction, turn or ring start, so nothing there contradicts D2. A post-edit re-sweep adding `vocabulary alone|no call site` found only intended sentences.
- **Step 8, subtask 5 (code-surface findings, reported, not fixed; Group A defects)**: a scratch copy of `spiral.go`/`depth.go`/`next.go`/`errors.go` under `tmp/_probe/gatedoc` (deleted after the run) established two false doc comments. (1) `Set.Depth`'s "It never looks at a chunk farther than the nearest gate's own ring" is false. With R=6, a sole gate in the cell's own chunk and the local cell farthest from the centre, `depthSearch` returned depth=6, lastRing=1 while the nearest gate is in ring 0, since `L(1)=4 < 6`. The true bound is `S(cell)`. (2) `Next`'s "So the fallback is never actually reached by an input the scan cannot already satisfy" contradicts the fallback it documents. With rings 0–2 created and k=0, `Next` returned `(3,-1)` from the fallback return, and `TestNext_Table` has that very row.
- **Step 8, Group B return**: orchestrator re-validated branch, base_commit and a clean tree and pushed; accepted Group B's `context.md` Status "Code:" edit beyond the design's list, because the added `internal/gate` import made that bullet's "reaches no call site beyond the balance loader" false and Step 9.5 would have required the same edit; a case-insensitive sweep of `key-decisions.md` and `context.md` found neither false doc-comment claim copied into KD-41.
- **Step 8, doc-comment defect**: Group B reported two false doc comments in Group A's code (`Set.Depth` "never looks at a chunk farther than the nearest gate's own ring"; `Next` "the fallback is never actually reached"); orchestrator reproduced both (radius-6 lower bound 4 < depth 6 visits ring 1; `TestNext_Table`'s fallback row) and routed a comment-only fix to a `code-writer` Mode B delegate; learnings entry written.
- **Step 9**: panic-index sync — no `panic(`, `log.Fatal`/`log.Panic` or `Must…` helper in `internal/gate` production files (patterns control-checked); no panic-index row.
- **Step 9**: domain-invariant sweep over `internal/gate` — patterns 1, 2, 4, 5 no hits; pattern 3 hit `internal/gate/next_test.go` `iterationCap`, a test oracle's loop bound, not a balance constant — legitimate. No ledger, scheduler, outbound message or migration in the diff.
- **Step 9**: `-race` is not required by the change (no goroutine, channel or `sync` use in `internal/gate` production files, pattern control-checked); `make verify` runs `test-race` regardless.
- **Step 9**: mutants run by the orchestrator via cp-backup, each confirmed to build before its result was read: three spiral mutants red on `TestSpiral_RingWalkTable`; `lowerBound` `+R`, "stop at first hit" (its first form did not build and was replaced by a building one) and "search until every gate is seen" red on their named tests; `spiral.go` and `depth.go` restored with no diff.
- **Step 9**: `make verify` exit 0 at e946162 (log `tmp/step9-verify.log`): no `FAIL` line; `internal/gate` ran fresh under both `test` and `test-race`, the unchanged packages replayed from the test cache. Harness-guard scripts CI runs over docs (`check-citations.sh`, `check-ac-shape.sh`, `check-spec-shape.sh`, `check-spec-anchors.sh`, `check-harness-gaps-forge.sh`) exit 0 locally, and the relative-link check is green; hook-body shellcheck and the guard regression suites were not run because neither `settings.json` nor any guard changed.
- **Step 9**: owner asked mid-step why the orchestrator authors code fixes after self-review instead of delegating; answered from a section-scoped read of `task/SKILL.md`, `task/reference.md`, `code-writer.md` and `delegation-rules.md`, and logged the Step 11 actor gap in `ai-docs/harness-gaps.md` 2026-09-14. Step 11 fixes in this run route by change-type: `.go` to `code-writer` Mode B, prose in-thread.
- **Step 9.5**: appended the task's `ai-docs/context-status.md` entry with the literal PR locator `#TBD-at-Step-12`; bumped `ai-docs/context.md`'s Status heading date (its Code bullet and layout line were already updated by Group B). No `docs/DESIGN.md` §16 open question is resolved by this task; no repo-root user-facing doc is contradicted.
- **Step 10**: self-review Round 1 REJECT — three major (SR1-1, SR1-2, SR1-3, all doc comments) and three nit (SR1-4, SR1-5, SR1-6) open, four accepted (SR1-A1 … SR1-A4); no finding touches a spec or design; re-litigation share 0 (first round).
- **Step 11 (Round 1)**: every open finding verified against the code before routing (SR1-1 call sites of `delta` / `ringChunk` and the absent `Next` disclaimer; SR1-2 reproduced at radius 6 — ring 1 bound 4 vs least 7, ring 3 24 vs 26, `C` at ring 13 walk position 1; SR1-4/5/6 lines and `Lattice.CellCount` present). All six are `.go` fixes, routed to a `code-writer` Mode B delegate with a sweep of the package's other doc comments in scope; a warm follow-up refined `Next`'s contract sentence to the nil error and dropped `ringChunk`'s int64 narration. No finding objected; no spec or design amendment.
- **Step 11 (Round 1)**: fix-round measurement at f82feb8 — SR1-1: `delta(` only in `ringChunk`, `ringChunk(` only in `Spiral` and `ringChunksAround`, no disclaimer pointer; SR1-2: `lowerBound`'s comment states a lower bound, attainment sentence gone, `+R` mutant on the new `lowerBound(lattice, ring)` signature built and went red on `TestLowerBound_HoldsAndIsAttained`; SR1-4, SR1-5, SR1-6 greps empty; `go build ./...`, `golangci-lint run`, `go test -count=1 ./internal/gate/` green; coverage ratchet holds on commit.
- **Step 11 (Round 1)**: the Mode B delegate found `tmp/sr1-bak/*.go` (self-review's mutation backups) breaking `go build ./...` and renamed them `.go.bak`; recurrence logged in `ai-docs/harness-gaps.md`. Two `ai-docs/learnings.md` entries: the recurring false/narrating doc comments, and the orchestrator's fix scoped to the two reported comments without a neighbour sweep.
- **Step 10**: self-review Round 2 REJECT — one major (SR2-1, `Set`'s cost claim) and two minor (SR2-2 harness-gaps entry, SR2-3 learnings entry) open, three accepted (SR2-A1 … SR2-A3); all six Round 1 rows hold. Re-litigation share: no register row re-opened; SR2-3 alone cites Round 1, 1 of 3 raised rows (below 50%); cap 3, continuing.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | D5's prose names the map field `s.gates`, and `NewSet` takes a `gates []hexgrid.Chunk` slice it ranges over to fill the map. | design-internal | folded | design § D5 and § Decomposition subtask 3 (map field `members`) @ 360c61d |
| G2 | 2 | The `TestSpiral_RingWalkTable` note "corner start (differs at ring 2)" is incomplete: a corner start also differs at ring 3. | design-internal | folded | design § Test Design (`TestSpiral_RingWalkTable` mutant note) @ 360c61d |

## Key discoveries (don't re-investigate)

- A Go package under a non-`_`-prefixed directory in `tmp/` is walked by `go build/test ./...` and `golangci-lint run`; scratch Go goes under `tmp/_probe/<name>/`.
- `detguard` recognises a map-typed struct field name as map-typed file-wide: the `Set`'s map field is `members`, a name no ranged identifier in `depth.go` carries.
- `grep` on this host is `ugrep`, which rejects a `.{0,90}(a|b|c).{0,90}` context pattern as exceeding its complexity limits; extract match context with Python instead.

## AC Status

| AC | Status | verifying command |
|----|--------|-------------------|
| AC1 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestSpiral_ListsEveryChunkOnceRingByRing|TestSpiralIndex_AgreesWithSpiral|TestSpiralIndex_RingRange)$' ./internal/gate/` — every named test `--- PASS` |
| AC2 | PASS (orchestrator, Step 9, at e946162; mutants seen red on TestSpiral_RingWalkTable: ceil start (ring 1), corner start (ring 2), mirrored turn (ring 1)) | `go test -count=1 -v -run '^(TestSpiral_RingWalkTable|TestSpiral_RingStartAndTurn)$' ./internal/gate/` — every named test `--- PASS` |
| AC3 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestNext_FirstGateIsCentre)$' ./internal/gate/` — every named test `--- PASS` |
| AC4 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestNext_Table|TestNext_SequentialFillReproducesSpiral|TestNext_MatchesSpiralScan)$' ./internal/gate/` — every named test `--- PASS` |
| AC5 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestNext_AlwaysReturns)$' ./internal/gate/` — every named test `--- PASS` |
| AC6 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestNext_RefusesNegativeK)$' ./internal/gate/` — every named test `--- PASS` |
| AC7 | PASS (orchestrator, Step 9, at e946162; mutant "stop at first hit" seen red on TestSetDepth_Table (farther-ring gate nearer in cells)) | `go test -count=1 -v -run '^(TestSetDepth_MatchesBruteForce|TestSetDepth_Table)$' ./internal/gate/` — every named test `--- PASS` |
| AC8 | PASS (orchestrator, Step 9, at e946162) | `go test -count=1 -v -run '^(TestSetDepth_FarBeyondEveryGate)$' ./internal/gate/` — every named test `--- PASS` |
| AC9 | PASS (orchestrator, Step 9, at e946162; mutant "search until every gate is seen" seen red on TestSearch_FarGatesAddNoWork; lowerBound "+R" mutant red on TestLowerBound_HoldsAndIsAttained) | `go test -count=1 -v -run '^(TestSearch_StaysWithinBound|TestSearch_FarGatesAddNoWork)$' ./internal/gate/` — every named test `--- PASS` |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| SR1-1 | 1 | major | fixed@f82feb8 | `for f in spiral.go next.go; do git show HEAD:internal/gate/$f \| grep -n -E '(ringChunk\|delta)\(\|int32\|seam' \| sed "s\|^\|$f:\|"; done` — expect `delta(` to occur in `next.go`, `ringChunk(` inside `SpiralIndex`, and an int32-seam disclaimer in `Next`'s doc comment; at 1dcb265 none of the three does (only `next.go:66`'s `//nolint` reason mentions the seam) |
| SR1-2 | 1 | major | fixed@f82feb8 | temporary `package gate` test logging `lowerBound(6, s)` beside the brute-force least `Distance(cell, lat.Center(ch))` over `lat.LocalCells()` × `ringChunksAround(Chunk{}, s)` for `s = 0..13`, and the `ringChunk(13, p)` positions whose centre is exactly `C` from the origin — at 1dcb265: `ring 1 lowerBound 4 leastActual 7`, `ring 3 lowerBound 24 leastActual 26`; `C attained at ring 13 walk position 1 chunk {13 -7}` (never position 0) |
| SR1-3 | 1 | major | fixed@f82feb8 | read `internal/gate/next.go:15-25`, `next.go:71-72`, `spiral.go:70-72`, `depth.go:70` against `ai-docs/doc-convention.md` DOC-4 *Narration* and *bare unqualified name used as a pointer* |
| SR1-4 | 1 | nit | fixed@f82feb8 | `grep -n 'unexported delta' internal/gate/spiral_test.go` → `215:` a `gate_test` comment naming package `gate`'s unexported `delta` |
| SR1-5 | 1 | nit | fixed@f82feb8 | `grep -n '_ = got' internal/gate/next_test.go` → `204:` a dead assignment in `TestNext_RefusesNegativeK` |
| SR1-6 | 1 | nit | fixed@f82feb8 | `grep -n -E '3\*r\*r \+ 3\*r \+ 1\|6 \* n\|6\*n\|% 6' internal/gate/depth.go internal/gate/spiral.go` — `C` recomputed where `hexgrid.Lattice.CellCount` exists; the six-sides literal unnamed |
| SR1-A1 | 1 | — | accepted@1 — mutant "DirE side also claims corner (n,−n)" (`r > -n` → `r >= -n` in `SpiralIndex`) stays green because it is equivalent: DirE with `t = n` gives `pos = n − ⌊n/2⌋`, identical to DirNE with `t = 0`; not a coverage gap | apply the mutant via cp-backup, `go test -count=1 -run '^TestSpiralIndex_AgreesWithSpiral$' ./internal/gate/` → ok |
| SR1-A2 | 1 | — | accepted@1 — `bench_test.go:17-19` carries `(go test -run=^$ -bench=. ./internal/gate/)`, the same form as `internal/maze/bench_test.go:9-12`; `make comment-refs` passes; not raised | `make comment-refs` → exit 0 |
| SR1-A3 | 1 | — | accepted@1 — `**parent_skill:**` absent from the progress header: `ai-docs/templates/progress-format.md:97` requires it only when a nested skill writes into a parent's file; this is `/task`'s own file | `rg -n parent_skill ai-docs/templates/progress-format.md` |
| SR1-A4 | 1 | — | accepted@1 — `TestSpiral_RingWalkTable` (3 cases, no `t.Run`, no case names) keys each case by ring and names the ring in every failure message; below severity floor | read `internal/gate/spiral_test.go:111-174` |
| SR2-1 | 2 | major | open | `grep -n 'cost no more' internal/gate/depth.go` → `6:`; then a temporary `package main` under `tmp/_probe/<name>/` that builds `gate.NewSet(hexgrid.Lattice{Radius: r}, []hexgrid.Chunk{{}})` and, for the cell `lat.Center(hexgrid.Chunk{Q: n, R: 0})`, prints `Set.Depth` beside `1 + 3g(g+1)` with `g = ChunkDistance(Locate(cell), centre)` — the chunk count of rings `0..g`, every one of which the search looks up before it can find the gate. At 1e33876: `R=0 chunk=(100,0) depth=100 … minChunkLookups=30301`, `R=6 chunk=(100,0) depth=1300 … minChunkLookups=30301`, `R=6 chunk=(300,0) depth=3900 … minChunkLookups=270901` |
| SR2-2 | 2 | minor | open | `grep -n -i 'mutation backup' AGENTS.md` → `98:` "a mutation backup or a throwaway probe written there by any other tool … goes to `tmp/` all the same"; `grep -n -E '^### 2026-09-1[34] — the (designated scratch|scratch-Go)' ai-docs/harness-gaps.md` → one 2026-09-13 entry and two 2026-09-14 entries |
| SR2-3 | 2 | minor | open | `git show 1dcb265:internal/gate/next.go` → `52: for ch := range Spiral() {`; `git show 1dcb265:internal/gate/spiral.go` → `60: yield(ringChunk(n, p))` — at the commit the learnings entry describes, `Next` reached `ringChunk` through `Spiral` |
| SR2-A1 | 2 | — | accepted@2 — `Set.Depth`'s "It stops at the first ring beyond which no chunk centre can lie nearer to cell than the best distance already found" describes the stop rule, but the sentence ends in the caller-visible cost contract AC9 asks for ("the rings it visits are bounded by that distance and the lattice radius, never by the number of gates"), and that contract is true (`TestSearch_StaysWithinBound`); below severity floor | read `internal/gate/depth.go:32-37` |
| SR2-A2 | 2 | — | accepted@2 — `depthSearch`'s "so the internal test suite can check the stated search bound directly rather than through timing" states why an unexported function exposes `lastRing`; it neither narrates the search nor names a place; below severity floor | read `internal/gate/depth.go:43-46` |
| SR2-A3 | 2 | — | accepted@2 — `context-status.md`'s doc-comment trap bullet names only the first two false comments, not round 1's further ones; incomplete, not false, and the two 2026-09-14 learnings entries carry the rest | read `ai-docs/context-status.md` § Spiral gate placement and nearest-gate depth → Traps found |

## Files touched

- `internal/gate/doc.go`, `internal/gate/errors.go`, `internal/gate/spiral.go`, `internal/gate/spiral_test.go`, `internal/gate/main_test.go`, `internal/gate/guards_test.go` (subtask 1, commit a2fa3d8)
- `internal/gate/next.go`, `internal/gate/next_test.go` (subtask 2, commit f162d29)
- `internal/gate/depth.go`, `internal/gate/depth_test.go`, `internal/gate/depth_internal_test.go` (subtask 3, commit 7f47611)
- `internal/gate/bench_test.go` (subtask 4)
- `ai-docs/key-decisions.md`, `ai-docs/context.md` (subtask 5, commit b33e5a4)

## Self-Review (Round 1)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/gate/spiral.go:9-11, 25-32 | major | SR1-1. Three claims in unexported doc comments are false against the code, and one of them is also a DOC-4 bare-name pointer. (a) `delta`: "the step Spiral's ring walk and Next's fallback both take". `Next`'s fallback builds `hexgrid.Chunk{Q: int32(fallbackRing), R: int32(-(fallbackRing / 2))}` directly, and `delta(` occurs only at `spiral.go:39-40`. (b) `ringChunk`: "the one ring walk Spiral, SpiralIndex, Next and Set.Depth all share". `SpiralIndex` never calls `ringChunk`, and its own doc comment (`spiral.go:70-72`) calls itself "an independent derivation from the walk ringChunk performs". The two comments contradict each other. (c) `ringChunk`: "matching Next's own disclaimer that no promise is made near the int32 seam". `Next`'s doc comment (`next.go:5-25`) carries no such disclaimer. The only seam text in `next.go` is the `//nolint:gosec` reason at `next.go:66`. So the comment points the reader at something that does not exist, which DOC-4 refuses in review even when the target is real. Verifying command output at 1dcb265: `spiral.go:39: corner := delta(d)`, `spiral.go:40: step := delta(...)`, `spiral.go:60: yield(ringChunk(n, p))`, `depth.go:77: off := ringChunk(n, p)`; `next.go` int32/seam hits: only `66:` (the nolint) and `67:` (the conversion). This is the same class of defect as the two false doc comments this run already fixed at e946162 and recorded as a trap in `context-status.md`. Fix: say only what `delta` and `ringChunk` are and what they return, and drop the pointer sentences. | ✅ Fixed |
| 2 | internal/gate/depth.go:83-87 | major | SR1-2. The `lowerBound` doc comment states "L(ring): the least possible hex-cell distance from any cell to the centre of any chunk at super-lattice distance ring", then "It is attained, not merely valid, on the mid-side direction the ring walk itself starts from". Both are false. The function is a lower bound: it is not the least distance, and it is attained only at some rings. The centre distance `C` at ring `2R+1` is reached at walk position 1, not at the walk's start. A temporary `package gate` probe at 1dcb265 (radius 6) printed `ring 1 lowerBound 4 leastActual 7`, `ring 3 lowerBound 24 leastActual 26`, `ring 5 lowerBound 43 leastActual 45`, and `centre distance C attained at ring 13 walk position 1 chunk {13 -7}` with no position-0 line. The design's Risks section names a wrong lower bound as the silently-wrong-depth risk, so a comment calling it exact invites the edit that risk describes. KD-41 states it correctly ("a lower bound … attained … the chunk `(2R+1, −(R+1))`"); only the code comment is wrong. Fix: call it a lower bound that the stop rule relies on, and drop the attainment sentence or state it correctly without pointing at the ring walk. | ✅ Fixed |
| 3 | internal/gate/next.go:15-25, next.go:71-72, spiral.go:70-72, depth.go:70 | major | SR1-3. DOC-4 *Narration*. Each of these comments tells how the function is implemented rather than what it is or what the caller may rely on. `Next`'s third paragraph ("If the scan finds no qualifying chunk within the bound it searches, it returns the first chunk of the ring one past that bound instead. That fallback is always valid: with M the largest ring … by the triangle inequality … The fallback is reached exactly when …") walks through the bounded scan, its fallback and the proof. The caller's contract is already complete in the first paragraph (the earliest qualifying chunk in `Spiral`'s order), and the result is the same whichever branch produces it. Any contract content ("Next always returns for k ≥ 0") fits in one sentence. `farEnough` "It loops over the slice rather than a set, since gates is exactly the caller's input" is implementation. `SpiralIndex` "The side and step within the ring are recovered from ch's own coordinates, an independent derivation from the walk ringChunk performs, so the two must agree" is implementation plus a bare-name pointer to `ringChunk`. `ringChunksAround` "found by translating the same ring walk Spiral uses" is implementation plus a bare-name pointer. The mechanism and its proof already live in KD-41, so deleting them from the comments loses nothing. | ✅ Fixed |
| 4 | internal/gate/spiral_test.go:215 | nit | SR1-4. The `gate_test` helper comment "computed independently of the package's own unexported delta helper" names a symbol from another package by bare name. The independence claim is about the method used, and "by driving a chunk's exported Neighbor method n times" already says that. | ✅ Fixed |
| 5 | internal/gate/next_test.go:204 | nit | SR1-5. `_ = got` is a dead statement: `got` is bound only to be discarded. Use `_, err := gate.Next(...)`. | ✅ Fixed |
| 6 | internal/gate/depth.go:89 | nit | SR1-6. `lowerBound` recomputes `C = 3*r*r + 3*r + 1` although `hexgrid.Lattice.CellCount` already computes it; the six-sides literal `6` in `6 * n` / `% 6` (spiral.go, depth.go) is unnamed. | ✅ Fixed |

**What was checked.**
- **Spawn prompt:** the five permitted items only; no contamination.
- **Spec conformance, AC1–AC9:**
  - AC1: `TestSpiral_ListsEveryChunkOnceRingByRing`, `TestSpiralIndex_AgreesWithSpiral`, `TestSpiralIndex_RingRange`.
  - AC2: `TestSpiral_RingWalkTable` and `TestSpiral_RingStartAndTurn`. I checked AC2's wording by hand against `directionDeltas`: DirNE (1,−1), DirNW (0,−1), and ring 3's start (3,−1).
  - AC3: `TestNext_FirstGateIsCentre`.
  - AC4: `TestNext_Table`, `TestNext_SequentialFillReproducesSpiral`, `TestNext_MatchesSpiralScan`.
  - AC5: `TestNext_AlwaysReturns`.
  - AC6: `TestNext_RefusesNegativeK` (`errors.Is`).
  - AC7: `TestSetDepth_MatchesBruteForce` and `TestSetDepth_Table`. I recomputed the later-ring row by hand: cell (0,−6), gate (0,1) → 19, gate (1,−2) at ring 2 → 14.
  - AC8: `TestSetDepth_FarBeyondEveryGate`.
  - AC9: `TestSearch_StaysWithinBound` and `TestSearch_FarGatesAddNoWork`. `S = ⌊(depth+R)(2R+1)/C⌋` is the largest `s` with `L(s) ≤ depth`.
  - The 16-test AC Status command set, re-run at 1dcb265 with `-count=1 -v`: 16 `--- PASS`, `ok` (`tmp/sr1-ac.log`). No scope creep. The `context.md` Code-bullet edit is propagation the orchestrator recorded and accepted.
- **Design conformance:**
  - D1: the exported surface matches exactly; imports are `errors`, `iter` and `internal/hexgrid` (`go list`); nothing imports `internal/gate`; `guards_test.go` applies `detguard`; `TestMain` uses the leaktest form.
  - D2: start `(n, −⌊n/2⌋)`, turn `d+2`, index formula, and a side-and-step switch identical to the design.
  - D4: scan bound `1+3B(B+1)`, fallback `(B+1, −⌊(B+1)/2⌋)`.
  - D5: map field `members`, the empty-set early return, and the stop rule `found && depth ≤ L(ring)`.
  - D6, D7: `NewSet` builds the set outside the query; the benchmarks assert no threshold.
  - D8: zero chunk on error.
  - Every Decomposition file is present. The GO notes: G1 and G2 were folded at 360c61d, and the design file is unchanged in the range. The design has no `AC<N> verified by:` lines; the AC Status commands above serve as that set.
- **Mutants,** each confirmed to build before its result was read (cp-backup, tree restored, `git status --porcelain internal/gate` empty):
  - "stop at first hit" → `TestSetDepth_Table` red.
  - `lowerBound` `+R` → `TestLowerBound_HoldsAndIsAttained` red.
  - "search until every gate seen" → `TestSearch_FarGatesAddNoWork` and `TestSearch_StaysWithinBound` red.
  - scan one ring short → `TestNext_SequentialFillReproducesSpiral` red.
  - fallback at the corner → `TestNext_Table` fallback row red.
  - `SpiralIndex` ceil start → `TestSpiralIndex_AgreesWithSpiral` red.
  - spacing `< k` → `TestNext_Table` and `TestNext_AlwaysReturns` red.
  - created ignored → `TestNext_Table` red.
  - "DirE claims corner (n,−n)" → green, an equivalent mutant (SR1-A1).
- **Safety:** no `panic(` / `log.Fatal*` / `log.Panic*` / `must…` in `internal/gate` production files, so no panic-index row is needed. No goroutine or `sync`, so `-race` is not load-bearing, but it was run anyway: `go test -count=1 -race -v ./internal/gate/` → ok (`tmp/sr1-gate-test.log`). No returned error is dropped; there is no context parameter and no `…Unchecked` function.
- **Domain invariants:** no ledger, posting, scheduler, outbound send, migration or secret in the diff. No balance constant: `k` and the radius are inputs. No `time.Now`, `math/rand` or map range on the pure paths (`detguard` green).
- **Gates at 1dcb265:** `go build ./...`, `go vet ./internal/gate/...`, `golangci-lint run ./...` (0 issues), `make comment-refs`, `make file-limits`, `golangci-lint fmt -d` all exit 0 (`tmp/sr1-gates.log`). `check-ac-shape.sh`, `check-spec-shape.sh`, `check-spec-anchors.sh` and `check-harness-gaps-forge.sh` exit 0. The coverage ratchet rose from 91.59 to 91.78.
- **Docs (Pattern 1, prose claims):**
  - KD-41's import set, "no import-allowlist guard … unlike maze's" (`internal/maze/guards_test.go:177` `TestGuard_ImportsAllowlist`; `internal/gate` has none, grep with a control line), the attaining chunk `(2R+1, −(R+1))` (probe: `{13 -7}` at R=6), and every cited test name exist and are true.
  - The KD-38 amendment and the `context.md` layout and Code bullets are true.
  - The `context-status.md` entry is consistent with the code.
- **Doc comments (DOC-1–DOC-4):** every exported item and the package carry name-first comments. The false claims and narration are rows 1–3.

**Not raised (register `accepted@1`):** SR1-A1 (the equivalent mutant), SR1-A2 (the benchmark command comment follows the `maze` precedent and passes the gate), SR1-A3 (`parent_skill` not required for `/task`'s own file), SR1-A4 (`TestSpiral_RingWalkTable` without `t.Run`).

**Routing note:** rows 1–3 are comment-only fixes in `internal/gate/*.go`, not a Spec or Design Amendment trigger. The design and KD-41 state these facts correctly; only the code comments contradict them.

## Self-Review (Round 2)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/gate/depth.go:5-9 | major | SR2-1. The `Set` doc comment says the set is "indexed once so that repeated Depth queries against the same lattice radius cost no more than the distance to the nearest gate, never the number of gates in the snapshot". The first half is false. The work of a query is chunk lookups over every ring from 0 to at least the nearest gate's ring `g`, which is `1 + 3g(g+1)` lookups. That grows with the square of the distance, not linearly. Probe at 1e33876, with one gate at the centre chunk and the cell at the centre of chunk `(n, 0)`: `R=0 chunk=(100,0) depth=100 … minChunkLookups=30301`, `R=6 chunk=(100,0) depth=1300 … minChunkLookups=30301`, `R=6 chunk=(300,0) depth=3900 … minChunkLookups=270901`. This is AC9's cost contract stated wrongly, and it contradicts the design's § Risks ("`Depth` costs chunk lookups over the hexagon of radius `S(cell)`, which grows with distance from the nearest gate"). Whether depth is cached, which is #29's open question, is decided by reading exactly this cost. Round 1's neighbour sweep missed it. `Set.Depth`'s own comment, KD-41 and the `context-status.md` entry all bound the **rings** visited, which is true, so only this comment is wrong. Fix (comment only, not an amendment trigger): state the bound as `Depth`'s comment does, by the distance to the nearest gate and the lattice radius, never by the gate count, or drop the cost clause from `Set`'s comment. | ⬜ Open |
| 2 | ai-docs/harness-gaps.md:519-524 | minor | SR2-2. Two claims in the new entry are false. (a) "This is the third instance in one run": only two of the three are in this run. The first is the 2026-09-13 entry, whose own heading shows it came before this run; the in-run instances are the design probe and the reviewer's backups. (b) "Nothing tells a mutant runner where a cp-backup of a `.go` file may live", and "The recipe `AGENTS.md` § Workflow gives … names no location": `AGENTS.md:98` (§ Build & Test) does name one. It says "a mutation backup or a throwaway probe written there by any other tool … goes to `tmp/` all the same". That instruction sends a `.go` backup into the walked directory, so the diagnosis names the wrong gap: the gap is not a missing location but a named location that is unsafe for a `.go` file. The proposed edit still fits. Fix: correct both sentences so the supervisor reading the log targets `AGENTS.md:98`'s sentence. | ⬜ Open |
| 3 | ai-docs/learnings.md:1070 | minor | SR2-3. The new entry says "`ringChunk`'s [comment] said `SpiralIndex` and `Next` share it — neither calls it". The part about `Next` misreports the defect. At 1dcb265 `Next` ranges over `Spiral()` (`next.go:52`), and `Spiral` calls `ringChunk` (`spiral.go:60`), so `Next` did share the ring walk. SR1-1 raised `SpiralIndex` alone. The log is append-only, so the fix is a new correcting entry, never an edit of this one. | ⬜ Open |

**Minor and nit items without a row:** none beyond the three `accepted@2` register rows (SR2-A1, SR2-A2, SR2-A3).

**What was checked.**
- **Spawn prompt:** the five permitted items only (invocation line, `Spec:`, `Design:`, `Progress:`, `360c61d..HEAD`); no contamination.
- **Register, round 1 rows** (re-examined over the f82feb8 diff and the tree at 1e33876):
  - SR1-1 holds: `delta(` occurs only inside `ringChunk` (`spiral.go:38-39`), and `ringChunk(` only in `Spiral` (`spiral.go:59`) and `ringChunksAround` (`depth.go:75`). A grep for comments naming other users (`share|both take|Next.s own|disclaimer|independent derivation|translating`) finds nothing; its control line matched. `ringChunk`'s comment now states its own seam disclaimer.
  - SR1-2 holds: `lowerBound`'s comment now states only a lower bound, which is true. I rebuilt the `+R` mutant against the new `lowerBound(lattice, ring)` and confirmed it builds. It went red on `TestLowerBound_HoldsAndIsAttained` (`radius 1 ring 0: cell {0 0} to chunk {0 0} centre distance 0 < lower bound 1`).
  - SR1-3 holds: `Next`'s fallback narration is replaced by the contract sentence "For k >= 0, Next returns a nil error." The narration and bare-name pointers in `farEnough`, `SpiralIndex` and `ringChunksAround` are gone.
  - SR1-4, SR1-5 and SR1-6 hold: each grep is empty and each control line matched. `sides` is named, and `lowerBound` uses `Lattice.CellCount`, whose value `3R²+3R+1` I read at `internal/hexgrid/lattice.go:15-18`.
  - None of the f82feb8 changes touches a spec or design file.
- **Spec conformance, AC1–AC9:** the saved `go test -count=1 -race -v ./internal/gate/` log at 1e33876 (`tmp/sr2-gate-test.log`) ends in `ok` and has exactly one `--- PASS` line for each of the 16 AC Status tests, plus `TestLowerBound_HoldsAndIsAttained`, `TestSetDepth_NoGateReportsFalse`, `TestNewSet_RefusesNegativeRadius` and `TestGuard_DeterminismPredicates`; the grep's control line matched. The fix commit adds no scope.
- **Design conformance:** f82feb8 changes no behaviour. `lowerBound`'s signature change (`r` becomes `lattice`) and the `sides` constant are internal to D5 and D2, and the design names neither signature. The GO notes G1 and G2 are unchanged (folded at 360c61d), and the design file is unchanged in the range.
- **Mutants,** each built before its result was read, with backups kept in the session scratchpad outside the module and the tree restored afterwards (`git status --porcelain internal/gate` empty):
  - `lowerBound` `+R` → `TestLowerBound_HoldsAndIsAttained` red.
  - "stop at first hit" (`if found {`) → `TestSetDepth_Table/a_farther-ring_gate_is_nearer_in_cells` red.
- **Gates at 1e33876:** `go build ./...`, `go vet ./internal/gate/...`, `golangci-lint run ./...` (0 issues), `golangci-lint fmt -d`, `make comment-refs` and `make file-limits` exit 0 (`tmp/sr2-gates.log`). `check-ac-shape.sh`, `check-spec-shape.sh`, `check-spec-anchors.sh` and `check-harness-gaps-forge.sh` exit 0. A find for `*.go` and `go.mod` under `tmp/` outside `_`-prefixed directories is empty; round 1's backups are now `tmp/sr1-bak/*.go.bak`.
- **Safety and domain invariants:** f82feb8 adds no `panic(`, no goroutine and no error path. Nothing ledger, scheduler, outbound-send, migration or secret related is in the range. `sides` is a geometric constant, not a balance value.
- **Doc comments (DOC-1–DOC-4),** every comment in `internal/gate` at 1e33876: name-first summaries and a package comment are present. The one false claim is row 1. `Depth`'s stop-rule sentence and `depthSearch`'s rationale were examined and accepted (SR2-A1, SR2-A2).
- **Prose added since round 1 (Pattern 1):**
  - The two 2026-09-14 learnings entries: the `lowerBound` figures, the ring-13 position-1 attainment and the narration list match round 1's probe and table. The false part is row 3.
  - The harness-gaps entry: `tmp/sr1-bak` holding three `.go.bak` files is true. The false parts are row 2.
  - `context-status.md`'s #120 entry: the "rings it visits are bounded by the returned depth and the radius" sentence is true (SR2-A3).
- **Objections:** none were recorded in round 1, so there is nothing to evaluate.

**Routing note:** row 1 is a comment-only fix in `internal/gate/depth.go`. The design's § Risks, KD-41 and `Set.Depth`'s own comment already state the cost correctly, so it is not a Spec or Design Amendment trigger. Rows 2 and 3 are prose fixes to log surfaces; row 3 needs a new entry, since the learnings log is append-only.
