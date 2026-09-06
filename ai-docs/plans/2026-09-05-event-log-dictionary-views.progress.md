# Progress: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views — ACTIVE
_Updated: 2026-09-06 09:52_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-event-log-dictionary-views
**base_commit:** 8f9bb8da0177a70fb63713bb504b2d0f9a6e9f55
**Last build:** not run
**Issue:** #21
**Spec:** ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md
**current_step:** Step 8 — subtask 7 of 8 complete
**last_passed_gate:** ci relative-markdown-link check | 2026-09-06T09:50Z | 73e5c78
**entry_args:** 21

## Next action

**Do this immediately:** continue Group B — subtask 8, the propagation sweep by `AGENTS.md` § *Propagation Rule* step 4's criterion.

## Subtasks

- [x] 1. Migration: `event_volume_class`, `event_type_definition` + §13.4 seeds, `event` + indexes, the `journal_entry` arc extension; table-list and goose-count assertions
- [x] 2. Go mirrors: `EventVolumeClass`, `EventType`, `EventTypeDefinition` + registry slice; extend both mirror tests (AC5)
- [x] 3. Write API: `EventID`, `Event` as a `PostingBasis`, `AppendEvent`, `ErrUnknownEventType` + the doc edits (AC6–AC9)
- [x] 4. Append-only sweep: extend the pattern to `event`, planted controls, the `event_type_definition` decoy, non-vacuity guard (AC10)
- [x] 5. The views: five views with fixed column names/order/types + §13.3 comments, exact-view-set assertion, literal per-view expectations (AC11–AC14)
- [x] 6. `domain-invariants.md` §5: payload rule + reasoning, the whole obligations/non-obligations table, arc consequences, suspended §13.5 limb, deferred families (AC15, AC16, AC18)
- [x] 7. `docs/DESIGN.md` in Russian: the log table's name at §13.1/§13.5; «игровое событие (лог 13.1)» into §11's registry (AC17)
- [ ] 8. Propagation sweep by the Propagation Rule step-4 criterion (AC18)

## Decisions log

- **Step 8**: Group A (1–5, `code-writer`, model pinned by frontmatter) runs before Group B (6–8, `general-purpose`, inherit) — subtask 8's sweep must run against the tree the code produced.
- **Step 8 subtask 1**: `event_volume_class` members are `low_volume`/`high_volume` per the design (names the axis, not the reader). `event.type`'s FK to `event_type_definition (code)` is named `event_type_fkey` so the write API (subtask 3) can map its SQLSTATE 23503 violation to `ErrUnknownEventType`. Strengthened the pre-existing `journal_entry_exactly_one_basis` substring check to name all five arc columns rather than the bare `num_nonnulls` substring that the pre-change form already satisfied. Filtered `TestMigrate_shape_and_seeds`'s table-list query to `table_type = 'BASE TABLE'` ahead of subtask 5 introducing views. A migration comment using the word "grant" (in prose, not SQL) tripped `TestMigrate_hygiene`'s `GRANT` pattern — reworded to avoid the substring.
- **Step 8 subtask 3**: `Event.insert` maps SQLSTATE 23503 on constraint `event_type_fkey` specifically (not any FK violation) to `ErrUnknownEventType`, mirroring `Post`'s `ErrOverdraft` mapping pattern in `post.go`. `sqlstateForeignKeyViolation`/`constraintEventTypeFKey` live in `event.go` since nothing else in the package needs them. Verified with `go test -race ./internal/store/` (green) in addition to the plain suite, since the write API adds a new `pgx.Tx`-facing code path.
- **Step 8 subtask 4**: one full-suite run failed with `unable to find network with name or ID reaper_default: network not found` — a transient podman/testcontainers infra hiccup, not a code defect; an immediate re-run of the same unchanged command was green. Recorded here because the instructions require isolating and reproducing a gate failure before treating it as transient: re-running the identical `go test ./internal/store/ -count=1` a second time is what established that, not an assumption.
- **Step 8 subtask 5**: chose a single-schema, single-fixture design across all five view subtests (per the design's "each view's subtest reads the same world"), which means `metric_retention_daily`'s expected table has to account for every event any other view's fixture data planted (deaths, notifications) since it aggregates over the whole `event` table regardless of type. Picked fixture day offsets and player counts by hand so every ratio column lands on a terminating decimal (0, 0.4, 0.5, 1.5, 1.0) rather than a repeating one, to avoid having to guess Postgres's numeric-division display scale; ratio/faucet/sink columns are scanned into `decimal.Decimal`/`*decimal.Decimal` and compared with `.Equal`, which is scale-independent, as the actual mechanism that sidesteps the formatting question. All literal expectations were verified by hand before running, then confirmed to pass unmodified on first execution against real Postgres — no expectation was adjusted to match observed output. `metric_faucet_sink`'s fixture is written by direct SQL against `manual_correction`/`journal_entry`/`posting` with explicit `ts` values, independent of the `event` table entirely, so it does not interact with the retention/funnel/death fixture at all. Removed an unused `//nolint:gosec` (that linter isn't enabled in this repo's `.golangci.yml`) that `nolintlint` correctly flagged as dead.
- **Step 8 subtask 6**: `domain-invariants.md` §5 gained five named subsections rather than a single run-on block, because the section now carries five separable obligations (payload rule, the view-input table, the arc consequences, the suspended §13.5 limb, the deferred families) and a mechanic author arrives looking for one of them. The obligation table is transcribed from the design's § *What the shipped views require* with both halves — obligations and non-obligations — and carries no issue column, per the design's reasoning that a wrong-but-existing `#N` passes the citation guard unchallenged; the obligation is recorded against the event type instead. Every factual claim in the new text was read back against the landed migration and `basis.go` in this invocation: the five `CREATE VIEW` names, the funnel's `ORDER BY e.player_id, e.ts, e.chat_id` tie-break, `journal_entry_event_key`'s partial unique index, the five-column `num_nonnulls` CHECK, and `PlayerOperation.insert`'s `ON CONFLICT … DO NOTHING` → `ErrAlreadyPosted`. AC15/AC16 are recorded `PASS`, not `TESTED`: the design's § Test Design says subtasks 6–8 have no Go tests and their acceptance is established by reading, so `TESTED` would be a false claim while `PASS` is the template's own token (`ai-docs/templates/progress-format.md:53`).
- **Step 8 subtask 7**: «игровое событие (лог 13.1)» is appended to the END of §11:331's list rather than inserted at the position :323 uses, because the owner authorised the membership and not an ordering, and appending leaves every existing item of the line untouched — the smallest diff that satisfies AC17's "no other content on the touched lines changes". The `events`→`event` rename is identifier-only at all three sites; `grep -n 'events' docs/DESIGN.md` now returns nothing, which is what establishes AC17's "at every site that names it" rather than the spec's three line numbers. :323's missing «переход рейд-сессии» is left open on purpose — #36's, per the spec's Deferred section.
- **Step 8 (group boundary)**: pushed the branch at the first group return per the Step-8 visibility rule; no PR yet, and CI triggers only on `main` and pull requests, so nothing ran.

## Key discoveries (don't re-investigate)

- The design's 83 repo tags all pin `8617a7c`; `git diff --stat 8617a7c HEAD` over the source tree is empty, so every pin is still current at implementation start.
- `go test ./internal/store/ -count=1` is green in ~15 s with no `LAB_GAME_TEST_DSN`: testcontainers reaches podman over `DOCKER_HOST`, and `postgres:18` is in the local image store.
- Only two existing assertions go RED on the new schema (the table list and the `goose_db_version` count). Everything else in the suite stays green while an AC still forces the coverage — see the design's § *What the existing suite catches, and what it does not*.

- **`TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` is a pre-existing flake in `internal/scheduler`, not this task's doing.** It asserts a wall-clock backoff delta grows strictly, and the delta includes the time spent inside `RunOnce`, so under `-race` with the store package's containers running in parallel the measurement noise can exceed the backoff growth. Measured: RED once in a full `go test -race ./...` on this branch (22.561ms after 28.438ms), 5/5 green in isolation on HEAD, 20/20 green in isolation on the base commit, and **RED 1-of-3 on a full `-race` run of the base commit in a clean worktree** (22.909ms after 27.101ms) — the control reproduced it without this change. `git diff 8617a7c..HEAD -- internal/scheduler/` is empty. Do not chase it as a defect of this task; do not treat a single green `-race` run as proof either.

## AC Status

| AC | Status |
|----|--------|
| AC1 | TESTED |
| AC2 | TESTED |
| AC3 | TESTED |
| AC4 | TESTED |
| AC5 | TESTED |
| AC6 | TESTED |
| AC7 | TESTED |
| AC8 | TESTED |
| AC9 | TESTED |
| AC10 | TESTED |
| AC11 | TESTED |
| AC12 | TESTED |
| AC13 | TESTED |
| AC14 | TESTED |
| AC15 | PASS |
| AC16 | PASS |
| AC17 | PASS |
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
- `internal/store/append_only_test.go`
- `internal/store/views_test.go` (new)
- `ai-docs/domain-invariants.md`
- `docs/DESIGN.md`
