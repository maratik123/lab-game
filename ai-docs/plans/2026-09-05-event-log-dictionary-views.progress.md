# Progress: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views — ACTIVE
_Updated: 2026-09-06 10:10_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-event-log-dictionary-views
**base_commit:** 8f9bb8da0177a70fb63713bb504b2d0f9a6e9f55
**Last build:** go build ./... green | 2026-09-06T10:05Z
**Issue:** #21
**Spec:** ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md
**current_step:** Step 9.5 — docs updated
**last_passed_gate:** golangci-lint run | 2026-09-06T09:25:39Z | e4f884ff2221740e6acce65831813d3a062c727f
**entry_args:** 21

## Next action

**Do this immediately:** all eight subtasks are committed. The orchestrator pushes the branch and runs Step 9 (verify) — no Go changes landed in Group B, so the full gate set (AC19) is still owed.

## Subtasks

- [x] 1. Migration: `event_volume_class`, `event_type_definition` + §13.4 seeds, `event` + indexes, the `journal_entry` arc extension; table-list and goose-count assertions
- [x] 2. Go mirrors: `EventVolumeClass`, `EventType`, `EventTypeDefinition` + registry slice; extend both mirror tests (AC5)
- [x] 3. Write API: `EventID`, `Event` as a `PostingBasis`, `AppendEvent`, `ErrUnknownEventType` + the doc edits (AC6–AC9)
- [x] 4. Append-only sweep: extend the pattern to `event`, planted controls, the `event_type_definition` decoy, non-vacuity guard (AC10)
- [x] 5. The views: five views with fixed column names/order/types + §13.3 comments, exact-view-set assertion, literal per-view expectations (AC11–AC14)
- [x] 6. `domain-invariants.md` §5: payload rule + reasoning, the whole obligations/non-obligations table, arc consequences, suspended §13.5 limb, deferred families (AC15, AC16, AC18)
- [x] 7. `docs/DESIGN.md` in Russian: the log table's name at §13.1/§13.5; «игровое событие (лог 13.1)» into §11's registry (AC17)
- [x] 8. Propagation sweep by the Propagation Rule step-4 criterion (AC18)

## Decisions log

- **Step 8**: Group A (1–5, `code-writer`, model pinned by frontmatter) runs before Group B (6–8, `general-purpose`, inherit) — subtask 8's sweep must run against the tree the code produced.
- **Step 8 subtask 1**: `event_volume_class` members are `low_volume`/`high_volume` per the design (names the axis, not the reader). `event.type`'s FK to `event_type_definition (code)` is named `event_type_fkey` so the write API (subtask 3) can map its SQLSTATE 23503 violation to `ErrUnknownEventType`. Strengthened the pre-existing `journal_entry_exactly_one_basis` substring check to name all five arc columns rather than the bare `num_nonnulls` substring that the pre-change form already satisfied. Filtered `TestMigrate_shape_and_seeds`'s table-list query to `table_type = 'BASE TABLE'` ahead of subtask 5 introducing views. A migration comment using the word "grant" (in prose, not SQL) tripped `TestMigrate_hygiene`'s `GRANT` pattern — reworded to avoid the substring.
- **Step 8 subtask 3**: `Event.insert` maps SQLSTATE 23503 on constraint `event_type_fkey` specifically (not any FK violation) to `ErrUnknownEventType`, mirroring `Post`'s `ErrOverdraft` mapping pattern in `post.go`. `sqlstateForeignKeyViolation`/`constraintEventTypeFKey` live in `event.go` since nothing else in the package needs them. Verified with `go test -race ./internal/store/` (green) in addition to the plain suite, since the write API adds a new `pgx.Tx`-facing code path.
- **Step 8 subtask 4**: one full-suite run failed with `unable to find network with name or ID reaper_default: network not found` — a transient podman/testcontainers infra hiccup, not a code defect; an immediate re-run of the same unchanged command was green. Recorded here because the instructions require isolating and reproducing a gate failure before treating it as transient: re-running the identical `go test ./internal/store/ -count=1` a second time is what established that, not an assumption.
- **Step 8 subtask 5**: chose a single-schema, single-fixture design across all five view subtests (per the design's "each view's subtest reads the same world"), which means `metric_retention_daily`'s expected table has to account for every event any other view's fixture data planted (deaths, notifications) since it aggregates over the whole `event` table regardless of type. Picked fixture day offsets and player counts by hand so every ratio column lands on a terminating decimal (0, 0.4, 0.5, 1.5, 1.0) rather than a repeating one, to avoid having to guess Postgres's numeric-division display scale; ratio/faucet/sink columns are scanned into `decimal.Decimal`/`*decimal.Decimal` and compared with `.Equal`, which is scale-independent, as the actual mechanism that sidesteps the formatting question. All literal expectations were verified by hand before running, then confirmed to pass unmodified on first execution against real Postgres — no expectation was adjusted to match observed output. `metric_faucet_sink`'s fixture is written by direct SQL against `manual_correction`/`journal_entry`/`posting` with explicit `ts` values, independent of the `event` table entirely, so it does not interact with the retention/funnel/death fixture at all. Removed an unused `//nolint:gosec` (that linter isn't enabled in this repo's `.golangci.yml`) that `nolintlint` correctly flagged as dead.
- **Step 8 subtask 6**: `domain-invariants.md` §5 gained five named subsections rather than a single run-on block, because the section now carries five separable obligations (payload rule, the view-input table, the arc consequences, the suspended §13.5 limb, the deferred families) and a mechanic author arrives looking for one of them. The obligation table is transcribed from the design's § *What the shipped views require* with both halves — obligations and non-obligations — and carries no issue column, per the design's reasoning that a wrong-but-existing `#N` passes the citation guard unchallenged; the obligation is recorded against the event type instead. Every factual claim in the new text was read back against the landed migration and `basis.go` in this invocation: the five `CREATE VIEW` names, the funnel's `ORDER BY e.player_id, e.ts, e.chat_id` tie-break, `journal_entry_event_key`'s partial unique index, the five-column `num_nonnulls` CHECK, and `PlayerOperation.insert`'s `ON CONFLICT … DO NOTHING` → `ErrAlreadyPosted`. AC15/AC16 are recorded `PASS`, not `TESTED`: the design's § Test Design says subtasks 6–8 have no Go tests and their acceptance is established by reading, so `TESTED` would be a false claim while `PASS` is the template's own token (`ai-docs/templates/progress-format.md:53`).
- **Step 8 subtask 7**: «игровое событие (лог 13.1)» is appended to the END of §11:331's list rather than inserted at the position :323 uses, because the owner authorised the membership and not an ordering, and appending leaves every existing item of the line untouched — the smallest diff that satisfies AC17's "no other content on the touched lines changes". The `events`→`event` rename is identifier-only at all three sites; `grep -n 'events' docs/DESIGN.md` now returns nothing, which is what establishes AC17's "at every site that names it" rather than the spec's three line numbers. :323's missing «переход рейд-сессии» is left open on purpose — #36's, per the spec's Deferred section.
- **Step 8 subtask 8**: the sweep found four members — `context.md`'s Observability row, its Architecture "Layout so far" clause for `internal/store`, its Status → Code inventory (with the Status date moved to match), and `domain-invariants.md` §3's basis-document type list, which omitted the game event that `docs/DESIGN.md` §11's registry now names. **Non-members, each checked rather than assumed:** `ai-docs/context-status.md` and `ai-docs/metrics/task-runs.jsonl` declare themselves append-only per-task records written by `/task`, so they are history rather than live claim surfaces; `.claude/**` names the table nowhere, and its `self-review.md` § 4a / `review-findings.md` § 1a / `design-writer.md` § Rules already route the reader to `domain-invariants.md` with **Telemetry lag** triggers that stay true — adding a review row for the new per-type dimension obligations would be a NEW rule, which is unapproved scope, so it was not done. **`ai-docs/key-decisions.md` KD-17 is stale and this diff is not why:** it enumerates the exclusive arc as «`player_operation` or `manual_correction`», which `00002_scheduler.sql` already falsified by widening the arc to four columns without touching the line. This diff does not change its truth value, so it is surfaced here rather than fixed as unrelated scope — a candidate for `/triage` or the next ledger-touching task. **Instrument check (`AGENTS.md` § Patterns 2):** the identical grep run against `8f9bb8d` fires on exactly the five pre-change sites and returns nothing against the tree now, so the clean result is evidence about the sites and not about the pattern.
- **Step 8 (group boundary)**: pushed the branch at the first group return per the Step-8 visibility rule; no PR yet, and CI triggers only on `main` and pull requests, so nothing ran.
- **Step 9**: no panic-index change — the non-test scan of `internal/` and `cmd/` for `panic(` / `log.Fatal` returns nothing, and the index's table is empty by design.
- **Step 9**: domain-invariant sweep clean. Its only hits are the ledger's own `UPDATE account_balance` and `INSERT INTO posting` inside `store.Post` — whose diff this round is doc-comment-only — and the faucet/sink test fixture, which writes a balanced posting pair by direct SQL with an explicit `ts`, exactly as the design requires so that view's expectations can be literals. No balance constant in Go, no new `time.Now()`, no secret.
- **Step 9**: `go test -race ./...` is RED 2-of-3 whole-suite runs, always on `TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` in the untouched `internal/scheduler` — the pre-existing flake now filed as #59. `internal/store` was green in every one of those runs and 3/3 under `-race` in isolation, so the race gate is clean for this diff's own package.
- **Step 9.5**: no open question in `context.md` is resolved by this task — §16.7 (player↔chat membership) is *touched* by the funnel's attribution choice and deliberately left open, with #30 recorded as its owner.
- **Step 9.5**: `check-citations.sh` is RED locally on this branch and GREEN on the base, and the citation it rejects is correct. Its ceiling is the newest **PR** (#58), while #59 is an **issue** filed by this run; the two share a numbering space. Recorded in `ai-docs/harness-gaps.md` rather than worked around. Expected to clear once this branch's PR raises the ceiling — **verify that at Step 12 by re-running the guard, do not assume it**.

## Key discoveries (don't re-investigate)

- The design's 83 repo tags all pin `8617a7c`; `git diff --stat 8617a7c HEAD` over the source tree is empty, so every pin is still current at implementation start.
- `go test ./internal/store/ -count=1` is green in ~15 s with no `LAB_GAME_TEST_DSN`: testcontainers reaches podman over `DOCKER_HOST`, and `postgres:18` is in the local image store.
- Only two existing assertions go RED on the new schema (the table list and the `goose_db_version` count). Everything else in the suite stays green while an AC still forces the coverage — see the design's § *What the existing suite catches, and what it does not*.

- **`TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` is a pre-existing flake in `internal/scheduler`, not this task's doing.** It asserts a wall-clock backoff delta grows strictly, and the delta includes the time spent inside `RunOnce`, so under `-race` with the store package's containers running in parallel the measurement noise can exceed the backoff growth. Measured: RED once in a full `go test -race ./...` on this branch (22.561ms after 28.438ms), 5/5 green in isolation on HEAD, 20/20 green in isolation on the base commit, and **RED 1-of-3 on a full `-race` run of the base commit in a clean worktree** (22.909ms after 27.101ms) — the control reproduced it without this change. `git diff 8617a7c..HEAD -- internal/scheduler/` is empty. Do not chase it as a defect of this task; do not treat a single green `-race` run as proof either.

## AC Status

Each row's command is the verifier's own, written for this AC and run over the AC's own stated
scope (`/task` § Patterns 1). Grep-shaped rows were run with a positive control first.

| AC | Status | Verifying command / evidence |
|----|--------|------------------------------|
| AC1 | PASS | `go test -count=1 ./internal/store -run TestMigrate` — shape + second-apply |
| AC2 | PASS | `grep -rniE '^\s*--\s*\+goose\s+Down' internal/store/migrations/` → none; the pattern fired on a planted line |
| AC3 | PASS | `sed -n '74,78p' 00003_event_log.sql` — the CHECK names all five arc columns; `journal_entry_event_key` is the partial unique index |
| AC4 | PASS | `go test -count=1 ./internal/store -run TestSchema` — 23514 zero-basis, 23514 two-basis, 23505 re-reference |
| AC5 | PASS | seed statement holds 16 rows, high_volume on 14/15 only; `grep -c 'Code: Event' catalog.go` → 16 and `VolumeHigh` → 2; `TestCatalog_mirrors_database` compares element-for-element, ordered |
| AC6 | PASS | `go test -count=1 ./internal/store -run TestEvent` |
| AC7 | PASS | `go test -count=1 ./internal/store -run TestEvent` |
| AC8 | PASS | `go test -count=1 ./internal/store -run TestEvent` |
| AC9 | PASS | `go test -count=1 ./internal/store -run TestEvent` |
| AC10 | PASS | `grep -rniE 'update\s+event\b\|delete\s+from\s+event\b' --include='*.go' internal/store/ \| grep -v _test.go` → none; migrations swept for `CREATE TRIGGER\|CREATE RULE\|GRANT` → none. Both patterns fired on planted lines |
| AC11 | PASS | `grep -oiE 'CREATE VIEW [a-z_]+' 00003_event_log.sql` → exactly the five; `go test -count=1 ./internal/store -run TestViews` covers the empty-log case |
| AC12 | PASS | `go test -count=1 ./internal/store -run TestViews` |
| AC13 | PASS | the AC11 grep piped through `grep -iE 'outcome\|backpack\|ctr\|button\|stamina'` → no match |
| AC14 | PASS | `go test -count=1 ./internal/store -run TestViews` |
| AC15 | PASS | `domain-invariants.md:54-63` states the rule and its four reasons; the page is listed at `agent-docs-index.md:15` |
| AC16 | PASS | `domain-invariants.md:88-97` (suspended limb + lift condition + no-panel-line rule) and `:105-108` (the four deferred families with owners) |
| AC17 | PASS | `grep -c 'events' docs/DESIGN.md` → 0; `:331` carries «игровое событие (лог 13.1)» |
| AC18 | PASS | case-insensitive tree sweep for `` `events` `` excluding history surfaces and `_inbox.jsonl` → none. **Instrument control:** the identical sweep at `8f9bb8d` fires on five sites (context.md 1, domain-invariants.md 1, DESIGN.md 3), so the clean result is evidence about the sites and not about the pattern |
| AC19 | PASS | build · vet · `go test -count=1 ./...` · `golangci-lint fmt -d` · `golangci-lint run` · `go mod tidy` (no delta) · `make file-limits` — all green; `make cover-ratchet` → 89.60% holds against 89.60%. `-race`: `internal/store` green in 3/3 isolated and in all three full runs; the full suite is RED 2-of-3 on `TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` in the untouched `internal/scheduler`, filed as #59 |

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
- `ai-docs/context.md`
