# Progress: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views — ACTIVE
_Updated: 2026-09-06 13:05_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-05-event-log-dictionary-views
**base_commit:** 8f9bb8da0177a70fb63713bb504b2d0f9a6e9f55
**Last build:** go build ./... green | 2026-09-06T10:05Z
**Issue:** #21
**Spec:** ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md
**current_step:** Step 10 — self-review APPROVE (Round 1)
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
- **Step 10**: APPROVE in round 1; seven register rows, all `accepted@1`, none above the severity floor. Two of them corrected my own Step-9 evidence lines rather than the code — the AC5 row quoted a grep whose real output is 3 (a doc-comment hit) and the AC3 locator had drifted as the migration grew.
- **Step 10**: commit `7412bcb`'s message over-claimed. It said it corrected those two evidence lines; the edit script had aborted on a non-unique anchor, so that commit carried only the reviewer's own register writes. The corrections landed in the following commit, which says so. Recorded because a commit message is a durable claim about work, and the tree is the only thing that settles it.

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
| AC3 | PASS | `sed -n '72,80p' internal/store/migrations/00003_event_log.sql` (coordinate re-resolved after R1-4 — the arc extension moved as the file grew) — the CHECK names all five arc columns; `journal_entry_event_key` is the partial unique index |
| AC4 | PASS | `go test -count=1 ./internal/store -run TestSchema` — 23514 zero-basis, 23514 two-basis, 23505 re-reference |
| AC5 | PASS | seed statement holds 16 rows, high_volume on 14/15 only; `grep -c 'Code: Event' catalog.go` → 16 and `grep -c 'VolumeClass: VolumeHigh' catalog.go` → 2 (bare `VolumeHigh` returns 3, one hit being the slice's own doc comment — corrected per R1-2); `TestCatalog_mirrors_database` compares element-for-element, ordered |
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
| R1-1 | round 1 | nit | accepted@1 — below severity floor. `Event`'s doc comment attributes the nil-payload default to "the migration's own DEFAULT"; on this path the column IS named in the INSERT, so the DEFAULT never fires and the `'{}'::jsonb` literal in `Event.insert`'s own SQL is what stores it. Reads as ambiguous rather than false; behaviour is correct and asserted. | `sed -n '30,33p' internal/store/event.go` |
| R1-2 | round 1 | minor | accepted@1 — below severity floor. The AC5 row records `` `VolumeHigh` → 2 ``; the literal command returns `3` (one hit is the mirror slice's own doc comment). The reproducing pattern is `VolumeClass: VolumeHigh`. AC5's substance is unaffected — it is established by `TestCatalog_mirrors_database`'s ordered element-for-element comparison, which is green. Evidence-record precision only. | `grep -c 'VolumeHigh' internal/store/catalog.go` → 3; `grep -c 'VolumeClass: VolumeHigh' internal/store/catalog.go` → 2 |
| R1-3 | round 1 | nit | accepted@1 — below severity floor. The funnel's determinism tie-break (`ORDER BY e.player_id, e.ts, e.chat_id`) is a clause no fixture reaches: no player has two `player_started` rows at equal `ts`, so deleting `, e.chat_id` leaves the suite green (this file's § Patterns 2). The *earliest*-wins half IS covered (player B, chat1 01-01 11:00 vs chat2 01-03 09:00). Design's Test Design lists no tie-break boundary. | `go test -count=1 ./internal/store -run TestViews_fixtureExpectations` after deleting `, e.chat_id` from the `attribution` CTE |
| R1-4 | round 1 | nit | accepted@1 — locator drift, re-resolved by the verifier (not an amendment trigger). The AC3 row cites `sed -n '74,78p' 00003_event_log.sql`; the arc extension now sits at **:72-80**. The citation still describes the artefact correctly. Unpinned coordinate. | `sed -n '72,80p' internal/store/migrations/00003_event_log.sql` |
| R1-5 | round 1 | minor | accepted@1 — pre-existing, not falsified by this diff. `ai-docs/key-decisions.md:47` (KD-17) still enumerates the exclusive arc as «`player_operation` or `manual_correction`». `00002_scheduler.sql` widened it to four columns without touching that line, so the claim was already false before this diff; AC18's criterion is "sites **this diff** falsifies". Surfaced in the Decisions log as `/triage` material. | `grep -n 'player_operation. or .manual_correction' ai-docs/key-decisions.md` |
| R1-6 | round 1 | minor | accepted@1 — instrument defect, correctly parked. `check-citations.sh` is RED on this branch because `#59` (a real OPEN issue, verified) exceeds the guard's ceiling, which reads `gh pr list` alone while issues and PRs share one numbering space. Not in `make verify`'s list and not in AC19. Recorded in `ai-docs/harness-gaps.md:208-214`. **Step 12 must re-run the guard, not assume it clears** when the PR raises the ceiling. | `bash .claude/skills/ai-audit/scripts/check-citations.sh` → `FAIL: 4 unresolvable citation(s)`; `gh issue view 59 --json state` → OPEN |
| R1-7 | round 1 | minor | accepted@1 — pre-existing flake, controlled. `go test -race ./...` is RED intermittently on `TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` in the untouched `internal/scheduler` (`git diff 8f9bb8d..HEAD -- internal/scheduler/` is empty), reproduced on the base commit in a clean worktree and filed as #59. This diff adds no goroutine and shares no state across requests. | `go test -race ./internal/store/ -count=1` (green 3/3, recorded); `go test -race ./...` |

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

## Self-Review (Round 1)

**Verdict:** APPROVE

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| — | — | — | No `blocker` or `major` finding clears the severity floor. Three `minor` and four `nit`/locator items ride along as `accepted@1` register rows R1-1…R1-7. | — |

**Minor/nit count and file list (no table rows, per § Rules):** 3 `minor` + 4 `nit`/drift —
`internal/store/event.go` (doc-comment attribution of the nil-payload default),
`internal/store/views_test.go` + `internal/store/migrations/00003_event_log.sql` (the funnel's
`, e.chat_id` tie-break is a clause no fixture reaches), this progress file's AC3 and AC5
evidence cells (a drifted `sed` range and an over-narrow `grep -c` count),
`ai-docs/key-decisions.md:47` (KD-17's arc enumeration, stale before this diff),
`.claude/skills/ai-audit/scripts/check-citations.sh` (ceiling read from PRs only),
`internal/scheduler` (#59, pre-existing `-race` flake).

### What was checked

**Spawn-prompt contract:** clean — invocation line, `Spec:`, `Design:`, `Progress:`, range. No
`PROMPT-CONTAMINATION`.

**Progress-file required fields:** `Branch`, `base_commit`, `Last build`, `current_step`,
`last_passed_gate`, `entry_args`, `## Decisions log` all present; `parent_skill` correctly
omitted (`/task` is the parent flow). Presence only — content not reviewed, per § Instructions 2.

**Spec conformance — every AC re-verified against the shipped tree, not against the record:**

| AC | Command re-run in this round | Result |
|----|------------------------------|--------|
| AC1 | `go test -count=1 ./internal/store` (incl. `TestMigrate_eventShape`: exact column set, per-column nullability, `identity_generation = ALWAYS`, named `event_type_fkey`) | PASS |
| AC2 | `grep -rniE '^\s*--\s*\+goose\s+Down' internal/store/migrations/` | PASS (no hit) |
| AC3 | `sed -n '72,80p' internal/store/migrations/00003_event_log.sql` — CHECK names all five arc columns; `journal_entry_event_key … WHERE event_id IS NOT NULL` | PASS (coordinate re-resolved, R1-4) |
| AC4 | `schema_test.go` — pre-existing `journal_entry_zero_bases` (23514) + new `journal_entry_event_and_player_operation` (23514) + `journal_entry_second_for_same_event` (23505) | PASS |
| AC5 | seed rows 1–16 with `high_volume` on 14/15 only; `TestCatalog_mirrors_database` compares ordered element-for-element | PASS |
| AC6–AC9 | `go test -count=1 ./internal/store -run TestEvent` + `TestAppendEvent` + `TestBasis_nil…`; typed-nil asserted with the statement recorder (zero statements) | PASS |
| AC10 | `append_only_test.go` pattern extended to `event`; planted controls fire, the `event_type_definition` decoy does not (`\b` before `_`); non-vacuity guard now names `event.go` and `00003_event_log.sql` | PASS |
| AC11 | `grep -oiE 'CREATE VIEW [a-z_]+' 00003_event_log.sql` → exactly the five; `TestViews_exactViewSet` + `TestViews_emptyLog_zeroRows` + `TestViews_columnContract` | PASS |
| AC12 | Every literal expectation **re-derived by hand from the fixture** in this round — funnel `(4,3,1)`/`(0,0,0)`, all eight retention rows incl. the 01-30 empty-denominator NULLs and the 0.4/0.5/1.5 fractions, the three death rows with the NULL-depth group, the three faucet/sink rows, the single notification row | PASS |
| AC13 | AC11 grep piped through `grep -iE 'outcome\|backpack\|ctr\|button\|stamina'` → no match; `TestViews_exactViewSet` is the real gate (an extra view fails set equality) | PASS |
| AC14 | `TestMetricFaucetSink_ledgerKindGenericity` — control (`count = 0` before the kind exists) precedes the `ALTER TYPE … ADD VALUE`, so a query that would match anything is excluded | PASS |
| AC15 | `domain-invariants.md` § *The payload rule* states the four-column line and all four reasons; the page is listed at `agent-docs-index.md:15` | PASS |
| AC16 | § *The §13.5 dashboard limb is suspended* (lift condition + the no-panel-line rule) and § *Four view families are owed* (#39/#40/#43+#37/#31) | PASS |
| AC17 | `grep -c 'events' docs/DESIGN.md` → **0**; `:331` carries «игровое событие (лог 13.1)» appended, no other content on the line changed | PASS |
| AC18 | Independent tree-wide `grep -rni '\bevents\b'` over `.claude/`, `AGENTS.md`, `ai-docs/`, `docs/`, `*.go/*.sql/*.yml/*.json/*.sh`, excluding history surfaces: **no surviving site names the table** — every remaining hit is the plural English word | PASS |
| AC19 | Re-run here: `go build ./...` + `go vet ./...` (exit 0) · `golangci-lint run` → `0 issues.` · `go test -count=1 ./...` → all 7 packages `ok` · `make file-limits` · `golangci-lint fmt -d` · `go mod tidy` + `git diff --exit-code go.mod go.sum` — all green | PASS |

**Design conformance.** Decomposition rows 1–8 all present in the diff and in `## Files touched`.
The two things the design refused to leave to the implementor both landed verbatim: every view's
column **names, order and types** are pinned by `TestViews_columnContract`, and every expected row
is a **literal**, never recomputed from the fixture. Views live in the goose migration; the
`metric_` prefix, the UTC day grain spelled `(ts AT TIME ZONE 'UTC')::date`, `NULLIF` on every
denominator, `::numeric` on every ratio numerator, `COALESCE(…, 0)` on every count and on both
faucet/sink legs, and `ad.kind::text` (rule 3) are all present. The registry is a seeded catalog
table with an enum volume class, `id` documented as order-carrying-and-referenced-by-nothing. The
funnel implements the round-3 attribution (`player_started`, earliest, tie by lower chat id) and
`raid_started.chat_id` is read nowhere. No architectural decision was taken on the fly.

**Mutation reasoning (§ Patterns 2 — is each guard's own clause reached?).** `::numeric` — row 1's
`d1_rate = 0.4` renders `0` under integer division, so the assertion fails. `NULLIF` — row 8
(2024-01-30) has `active_player = 0`, so without it the view raises `22012` rather than returning
NULL. `COALESCE` on `sum(…) FILTER` — the 01-06 faucet-only rows expect `sink = 0`, and a NULL
would fail the non-pointer `decimal.Decimal` scan. `::text` on `ad.kind` — the column contract
asserts `text`, which `USER-DEFINED` fails. The strengthened `journal_entry_exactly_one_basis`
substring names `event_id`, so a re-added CHECK omitting it fails (the bare `num_nonnulls` form did
not). The one clause the fixture does **not** reach is R1-3.

**Safety and correctness.** Panic sweep over non-test `internal/` + `cmd/` →
`grep -nE '(^|[^[:alnum:]_.])(panic\(|log\.(Fatal|Panic)[a-z]*\()'` returns nothing, so
`ai-docs/panic-index.md` correctly stays empty. No `_ = err`, no `== Err…` comparison, no new
`context.Background()` outside tests, `ctx` first everywhere, no context in a struct, no
`…Unchecked` function added. `ErrUnknownEventType` is a package-level `errors.New` sentinel,
wrapped with `%w`, distinctness asserted by a loop over the whole sentinel set.

**Domain invariants (§ 4a).** No balance column is UPDATEd and no holding row inserted outside
`store.Post` — the faucet/sink fixture writes a **balanced** posting pair by direct SQL with an
explicit `ts`, which the design requires so the view's `day` can be a literal. The arc extension
adds a fifth column and re-adds `CHECK (num_nonnulls(…) = 1)` over all five, so every group still
has exactly one basis. Not a mechanic: nothing is emitted, so §13.4's telemetry obligation does not
fire (the design argues this from KD-11's «Post is not a mechanic»). No tuning value in Go — `depth`,
the D1/D7 offsets and the day grain are the metric's definition. Forward migration only, nothing
renamed or renumbered, arc column added nullable so every pre-existing `journal_entry` still
satisfies the re-added CHECK. No chat-send path. No `time.Now()` on a pure path. No secret.

**Style and documentation.** Every new exported item (`EventVolumeClass`, `EventType`,
`EventTypeDefinition`, `EventID`, `Event`, `AppendEvent`, `ErrUnknownEventType`) carries a doc
comment opening with its own identifier in the third person; both const blocks carry a block
comment; every design citation is by section (`§11`, `§13.1`, `§13.4`) and none by line; no `TODO`,
no commented-out code. Largest files: `views_test.go` 651 and `migrate_test.go` 442 (limit 1500),
`post.go` 193 and the migration 275 — all inside the soft bands. `sqlstateForeignKeyViolation` and
`constraintEventTypeFKey` are named constants; the diff adds no `//nolint` (it removes a dead one).

**Objection quality (§ 7).** Not applicable — round 1, register was empty.
