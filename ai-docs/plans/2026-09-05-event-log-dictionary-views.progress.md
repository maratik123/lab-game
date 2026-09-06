# Progress: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views — ACTIVE
_Updated: 2026-09-06 08:27_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-event-log-dictionary-views
**base_commit:** 8f9bb8da0177a70fb63713bb504b2d0f9a6e9f55
**Last build:** not run
**Issue:** #21
**Spec:** ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md
**current_step:** Step 8 — subtask 3 of 8 complete
**last_passed_gate:** golangci-lint run | 2026-09-06T08:41:48Z | 121e7a3a793d801b0207df2c9eab4f5fbecda349
**entry_args:** 21

## Next action

**Do this immediately:** spawn Group A (subtasks 1–5) through `/context-reset` into `code-writer`, per the design's `## Handoff plan`.

## Subtasks

- [x] 1. Migration: `event_volume_class`, `event_type_definition` + §13.4 seeds, `event` + indexes, the `journal_entry` arc extension; table-list and goose-count assertions
- [x] 2. Go mirrors: `EventVolumeClass`, `EventType`, `EventTypeDefinition` + registry slice; extend both mirror tests (AC5)
- [x] 3. Write API: `EventID`, `Event` as a `PostingBasis`, `AppendEvent`, `ErrUnknownEventType` + the doc edits (AC6–AC9)
- [ ] 4. Append-only sweep: extend the pattern to `event`, planted controls, the `event_type_definition` decoy, non-vacuity guard (AC10)
- [ ] 5. The views: five views with fixed column names/order/types + §13.3 comments, exact-view-set assertion, literal per-view expectations (AC11–AC14)
- [ ] 6. `domain-invariants.md` §5: payload rule + reasoning, the whole obligations/non-obligations table, arc consequences, suspended §13.5 limb, deferred families (AC15, AC16, AC18)
- [ ] 7. `docs/DESIGN.md` in Russian: the log table's name at §13.1/§13.5; «игровое событие (лог 13.1)» into §11's registry (AC17)
- [ ] 8. Propagation sweep by the Propagation Rule step-4 criterion (AC18)

## Decisions log

- **Step 8**: Group A (1–5, `code-writer`, model pinned by frontmatter) runs before Group B (6–8, `general-purpose`, inherit) — subtask 8's sweep must run against the tree the code produced.
- **Step 8 subtask 1**: `event_volume_class` members are `low_volume`/`high_volume` per the design (names the axis, not the reader). `event.type`'s FK to `event_type_definition (code)` is named `event_type_fkey` so the write API (subtask 3) can map its SQLSTATE 23503 violation to `ErrUnknownEventType`. Strengthened the pre-existing `journal_entry_exactly_one_basis` substring check to name all five arc columns rather than the bare `num_nonnulls` substring that the pre-change form already satisfied. Filtered `TestMigrate_shape_and_seeds`'s table-list query to `table_type = 'BASE TABLE'` ahead of subtask 5 introducing views. A migration comment using the word "grant" (in prose, not SQL) tripped `TestMigrate_hygiene`'s `GRANT` pattern — reworded to avoid the substring.
- **Step 8 subtask 3**: `Event.insert` maps SQLSTATE 23503 on constraint `event_type_fkey` specifically (not any FK violation) to `ErrUnknownEventType`, mirroring `Post`'s `ErrOverdraft` mapping pattern in `post.go`. `sqlstateForeignKeyViolation`/`constraintEventTypeFKey` live in `event.go` since nothing else in the package needs them. Verified with `go test -race ./internal/store/` (green) in addition to the plain suite, since the write API adds a new `pgx.Tx`-facing code path.

## Key discoveries (don't re-investigate)

- The design's 83 repo tags all pin `8617a7c`; `git diff --stat 8617a7c HEAD` over the source tree is empty, so every pin is still current at implementation start.
- `go test ./internal/store/ -count=1` is green in ~15 s with no `LAB_GAME_TEST_DSN`: testcontainers reaches podman over `DOCKER_HOST`, and `postgres:18` is in the local image store.
- Only two existing assertions go RED on the new schema (the table list and the `goose_db_version` count). Everything else in the suite stays green while an AC still forces the coverage — see the design's § *What the existing suite catches, and what it does not*.

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

- `internal/store/migrations/00003_event_log.sql` (new)
- `internal/store/migrate_test.go`
- `internal/store/schema_test.go`
- `internal/store/enums.go`
- `internal/store/catalog.go`
- `internal/store/enums_test.go`
- `internal/store/ids.go`
- `internal/store/errors.go`
- `internal/store/basis.go`
- `internal/store/post.go`
- `internal/store/event.go` (new)
- `internal/store/basis_test.go`
- `internal/store/event_test.go` (new)
