# Progress: Spiral gate placement and nearest-gate depth — ACTIVE
_Updated: 2026-09-14 10:58_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-spiral-gate-placement-depth
**base_commit:** 360c61d385a81ddc0ec85dd6e83343f66a44d4cc
**Last build:** PASS

**Issue:** #120
**Spec:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.spec.md
**Design:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.design.md

**current_step:** Step 9.5 — docs updated
**last_passed_gate:** make verify (fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck comment-refs import-guard) | 2026-09-14T11:08:49Z | e946162e2d19b7d7c11ae4e2c9071fcc88f36f14
**entry_args:** 120

## Next action

**Do this immediately:** Step 10 — spawn `self-review` with the closed five-item prompt (invocation line, spec, design, progress, `360c61d..HEAD`).

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

## Files touched

- `internal/gate/doc.go`, `internal/gate/errors.go`, `internal/gate/spiral.go`, `internal/gate/spiral_test.go`, `internal/gate/main_test.go`, `internal/gate/guards_test.go` (subtask 1, commit a2fa3d8)
- `internal/gate/next.go`, `internal/gate/next_test.go` (subtask 2, commit f162d29)
- `internal/gate/depth.go`, `internal/gate/depth_test.go`, `internal/gate/depth_internal_test.go` (subtask 3, commit 7f47611)
- `internal/gate/bench_test.go` (subtask 4)
- `ai-docs/key-decisions.md`, `ai-docs/context.md` (subtask 5, commit b33e5a4)
