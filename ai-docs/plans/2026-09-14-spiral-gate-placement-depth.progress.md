# Progress: Spiral gate placement and nearest-gate depth — ACTIVE
_Updated: 2026-09-14 10:29_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-spiral-gate-placement-depth
**base_commit:** 360c61d385a81ddc0ec85dd6e83343f66a44d4cc
**Last build:** PASS

**Issue:** #120
**Spec:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.spec.md
**Design:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.design.md

**current_step:** Step 8 — subtask 4 of 5 complete, Group A done
**last_passed_gate:** golangci-lint run ./internal/gate/... | 2026-09-14 | adf4c1c
**entry_args:** 120

## Next action

**Do this immediately:** Group A (code, `code-writer`) — subtasks 1–4 of the design's `## Decomposition`, in order, gating and committing after each subtask.

## Subtasks

Groups per the design's `## Handoff plan`.

- [x] 1. Group A — package scaffold and the spiral order (`internal/gate`: doc, errors, spiral, main_test, guards_test, spiral tests) — commit a2fa3d8
- [x] 2. Group A — the next gate and its tests — commit f162d29
- [x] 3. Group A — `Set`, `NewSet`, `Depth`, with the external tests and the internal AC9 test — commit 7f47611
- [x] 4. Group A — the reporting-only benchmark — commit adf4c1c
- [ ] 5. Group B — documentation (KD-41, KD-38 consumer sentence, `context.md` layout line)  ← CURRENT

## Decisions log

- **Steps 1–5**: spec drafted over two interview rounds; owner answers recorded verbatim in the state file's `prior_qa` (k's meaning; ring start mid-side rounding down as the owner's own decision; direction and turn left to the design).
- **Steps 1–5**: orchestrator's round-1 verification of the ring-start question used an invalid regularity instrument (`(q-r) mod 3`); corrected by a coset test before the question reached the owner — logged in `ai-docs/learnings.md` 2026-09-14.
- **Step 6**: design-writer's scratch probe under `tmp/spiralprobe` was walked by `./...` and failed module-wide lint; moved to `tmp/_spiralprobe` with a symlink while the design cited it, deleted once round 2 dropped the citations — recurrence logged in `ai-docs/harness-gaps.md` 2026-09-14.
- **Step 7**: design-review round 1 ITERATE (three major, two minor); round 2 GO with one minor and one recommendation, both design-internal, folded by design-writer at 360c61d; design-review not re-run.
- **Step 8, subtask 1**: the pre-commit comment-refs gate (run over the staged diff) caught a decision-anchor ("D9") in a `//nolint` comment and a package-qualified symbol (`hexgrid.Chunk.Neighbor`) in a test-helper doc comment that `make comment-refs` (run before staging) had not yet seen against these files; both reworded to name no decision id and no cross-package symbol, and the commit succeeded on retry.
- **Step 8, subtask 3**: `TestSetDepth_Table`'s later-ring row (a ring-2 gate nearer in cells than a ring-1 gate, to red-flag a "stop at first ring with a hit" search) was found by a scratch Go program under `tmp/_probe/gatedepth` that brute-forced radius-6 chunk centres for an inversion, then deleted once the concrete cell/gate/distance values were copied into the test; radius-8 gates and two far chunk directions (corner and mid-side) were used the same way for `TestSetDepth_FarBeyondEveryGate`. Both `lowerBound`'s "+R" mutant and the "stop at first hit" mutant were run against `depth.go` and seen to fail the relevant test before being reverted, per the AC9/AC7 mutant notes in the design's Test Design section.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | D5's prose names the map field `s.gates`, and `NewSet` takes a `gates []hexgrid.Chunk` slice it ranges over to fill the map. | design-internal | folded | design § D5 and § Decomposition subtask 3 (map field `members`) @ 360c61d |
| G2 | 2 | The `TestSpiral_RingWalkTable` note "corner start (differs at ring 2)" is incomplete: a corner start also differs at ring 3. | design-internal | folded | design § Test Design (`TestSpiral_RingWalkTable` mutant note) @ 360c61d |

## Key discoveries (don't re-investigate)

- A Go package under a non-`_`-prefixed directory in `tmp/` is walked by `go build/test ./...` and `golangci-lint run`; scratch Go goes under `tmp/_probe/<name>/`.
- `detguard` recognises a map-typed struct field name as map-typed file-wide: the `Set`'s map field is `members`, a name no ranged identifier in `depth.go` carries.

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS (spiral_test.go) |
| AC2 | PASS (spiral_test.go) |
| AC3 | PASS (next_test.go) |
| AC4 | PASS (next_test.go) |
| AC5 | PASS (next_test.go) |
| AC6 | PASS (next_test.go) |
| AC7 | PASS (depth_test.go) |
| AC8 | PASS (depth_test.go) |
| AC9 | PASS (depth_internal_test.go) |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `internal/gate/doc.go`, `internal/gate/errors.go`, `internal/gate/spiral.go`, `internal/gate/spiral_test.go`, `internal/gate/main_test.go`, `internal/gate/guards_test.go` (subtask 1, commit a2fa3d8)
- `internal/gate/next.go`, `internal/gate/next_test.go` (subtask 2, commit f162d29)
- `internal/gate/depth.go`, `internal/gate/depth_test.go`, `internal/gate/depth_internal_test.go` (subtask 3, commit 7f47611)
- `internal/gate/bench_test.go` (subtask 4)
