# Progress: Maze persistence — ACTIVE
_Updated: 2026-09-17 04:21_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-17-maze-persistence
**base_commit:** bbf761705064b277625c3e9efae4b03d31816872
**Last build:** PASS
**Issue:** #29
**Spec:** ai-docs/plans/2026-09-17-maze-persistence.spec.md
**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | 2026-09-17T04:21Z | bbf761705064b277625c3e9efae4b03d31816872
**entry_args:** 29

## Next action

**Do this immediately:** spawn the Group A implementor (`code-writer`, subtasks 1–7) per the design's `## Handoff plan`, starting at subtask 1 — the forward migration and the event-type registration.

## Subtasks

- [ ] 1. The forward migration and the event-type registration  ← CURRENT
- [ ] 2. Package skeleton and the stored-map codec
- [ ] 3. The maze handle — `Spec`, `Maze`, `Open`, `Lattice()`
- [ ] 4. Chunk creation — `EnsureChunkAt`
- [ ] 5. Gate allocation — `ActivateChat`
- [ ] 6. Depth and discovery
- [ ] 7. The concurrency suite
- [ ] 8. Key decisions, including the KD-41 amendment
- [ ] 9. The invariant, the architecture prose and the Propagation Rule sweep

Group A = 1–7 (code, `code-writer`). Group B = 8–9 (instructions/harness, `general-purpose`).

## Decisions log

- **Step 8**: created at the Step 7 GO (design-review round 2); Group A opens with subtask 1 per the design's `## Handoff plan`, which the orchestrator does not re-derive.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | D10 is the only API in the document without a Go signature | design-internal | folded | design § D10 @ bbf7617 — full receiver form, and `maze_id` stated as coming from the handle |
| G2 | 2 | Subtask 9's sweep set is `.claude/`, `AGENTS.md`, `ai-docs/` and repo-root docs, and it enumerates three falsified claims | design-internal | folded | design § Decomposition subtask 1 + subtask 9 @ bbf7617 — the `00003_event_log.sql` comment repair moved into Group A, subtask 9 states it is out of its sweep, `internal/maze` reachability added to its claim list |
| G3 | 2 | Subtask 4's AC3 case compares the stored map with `Generate(ch, fabric)` called with no neighbour maps | design-internal | folded | design § Test Design subtask 4 @ bbf7617 — split into an isolated case and a beside-a-neighbour case |
| G4 | 2 | Two counts that rot | design-internal | folded | design § D1 + § Test Design @ bbf7617 — literals removed, not re-measured; control: 2 hits at 95a4d16, 0 now |
| G5 | 2 | § Risks names the two-pool-connections shape but its mitigation is a doc comment plus one test | design-internal | folded | design § D5, § D11, § Risks, § Test Design @ bbf7617 — `Spec.CreateBudget` and `ErrCreateBudget`, plus the composition root's sizing rule |
| G6 | 2 | D2 does not state `NOT NULL` on `chunk_type` / `creation_cause` | design-internal | folded | design § D2 @ bbf7617 — both columns `NOT NULL`, with the CHECK's NULL semantics as the reason |
| G7 | 2 | Subtask 5 has no border-agreement case of its own | design-internal | folded | design § Test Design subtask 5 @ bbf7617 — a gate chunk beside an existing fabric neighbour, walking the shared border |
| G8 | 2 | D9's `ring` is carried separately from `spiral_index` — worth one clause saying why | design-internal | folded | design § D9 @ bbf7617 — the KD-41 no-inverse reason quoted |

Every row was confirmed present in the design by reading the region, not by a pattern match: three of the eight were false negatives of the orchestrator's own greps.

## Key discoveries (don't re-investigate)

- Editing a **shipped** migration's comments is established practice here, and the precedent is exact: commit `5db5aa2` edited `00003_event_log.sql`, and its only non-comment change was to that file's `maze_id` comment. DDL is still forward-only.
- `TestMigrate_hygiene` bans `ADD VALUE` in a file that also has CREATE TABLE / INSERT / UPDATE. This migration is unaffected because both enums are new (`CREATE TYPE … AS ENUM`), so tables and the registry `INSERT` may share one file.
- `internal/store/migrate_test.go`'s base-table assertion is an exhaustive `slices.Equal` set. It is **extended, never loosened** — loosening it to a subset check turns the gate green while deleting a schema guard.
- `fkcover_test.go` compares a FK's columns against the index's **leading** columns, so the partial `UNIQUE (maze_id, gate_chat_id)` does not cover the `gate_chat_id` FK; a standalone partial index does.

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

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

_None yet — Group A has not started._
