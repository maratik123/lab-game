# Progress: Spiral gate placement and nearest-gate depth — ACTIVE
_Updated: 2026-09-14 10:29_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-14-spiral-gate-placement-depth
**base_commit:** 360c61d385a81ddc0ec85dd6e83343f66a44d4cc
**Last build:** PASS

**Issue:** #120
**Spec:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.spec.md
**Design:** ai-docs/plans/2026-09-14-spiral-gate-placement-depth.design.md

**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | 2026-09-14T10:29:29Z | 360c61d385a81ddc0ec85dd6e83343f66a44d4cc
**entry_args:** 120

## Next action

**Do this immediately:** Group A (code, `code-writer`) — subtasks 1–4 of the design's `## Decomposition`, in order, gating and committing after each subtask.

## Subtasks

Groups per the design's `## Handoff plan`.

- [ ] 1. Group A — package scaffold and the spiral order (`internal/gate`: doc, errors, spiral, main_test, guards_test, spiral tests)  ← CURRENT
- [ ] 2. Group A — the next gate and its tests
- [ ] 3. Group A — `Set`, `NewSet`, `Depth`, with the external tests and the internal AC9 test
- [ ] 4. Group A — the reporting-only benchmark
- [ ] 5. Group B — documentation (KD-41, KD-38 consumer sentence, `context.md` layout line)

## Decisions log

- **Steps 1–5**: spec drafted over two interview rounds; owner answers recorded verbatim in the state file's `prior_qa` (k's meaning; ring start mid-side rounding down as the owner's own decision; direction and turn left to the design).
- **Steps 1–5**: orchestrator's round-1 verification of the ring-start question used an invalid regularity instrument (`(q-r) mod 3`); corrected by a coset test before the question reached the owner — logged in `ai-docs/learnings.md` 2026-09-14.
- **Step 6**: design-writer's scratch probe under `tmp/spiralprobe` was walked by `./...` and failed module-wide lint; moved to `tmp/_spiralprobe` with a symlink while the design cited it, deleted once round 2 dropped the citations — recurrence logged in `ai-docs/harness-gaps.md` 2026-09-14.
- **Step 7**: design-review round 1 ITERATE (three major, two minor); round 2 GO with one minor and one recommendation, both design-internal, folded by design-writer at 360c61d; design-review not re-run.

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
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- (none yet)
