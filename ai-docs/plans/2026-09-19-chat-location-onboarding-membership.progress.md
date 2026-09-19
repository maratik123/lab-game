# Progress: Chat location, deep-link onboarding, and player-to-chat membership — ACTIVE
_Updated: 2026-09-19 12:54_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-19-chat-location-onboarding-membership
**base_commit:** b48ce14778576085f52db79fcbeb6d7333decb18
**Last build:** PASS

**Issue:** #30
**Spec:** ai-docs/plans/2026-09-19-chat-location-onboarding-membership.spec.md

**current_step:** Step 8 — subtask 10 of 11 complete (Group A done)
**last_passed_gate:** go test ./... + golangci-lint run ./... + go vet ./... + make comment-refs + make import-guard + go test -race ./internal/onboard/... ./cmd/bot/... | subtask 10
**entry_args:** 30

## Next action

**Do this immediately:** spawn Group A (subtasks 1–10) through `/context-reset` with `code-writer`, starting at subtask 1 — the forward migration `internal/store/migrations/00010_chat_home_presence_membership.sql` and every schema manifest it moves.

## Subtasks

- [x] 1. Forward migration: `home` scope definition + backfill, `chat_presence`, `chat_membership`, the `chat_knowledge` view, and every hard-coded manifest it moves  (c75b2b2)
- [x] 2. Behaviour tests for `chat_knowledge`
- [x] 3. The peaceful-home instrument
- [x] 4. `store.EnsureOwner`
- [x] 5. `internal/chat` (presence, membership, `DestinationLookup`) + `testdb.Binaries`
- [x] 6. Outbound gate: rename and rewire in one subtask
- [x] 7. `internal/onboard`: the `start` payload codec and the link builder
- [x] 8. The `my_chat_member` handler + `testdb.Binaries`
- [x] 9. The `/start` handler
- [x] 10. Composition root: both routes, the outbound test, the funnel test
- [ ] 11. Documentation and the propagation sweep

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review ran five rounds; the owner raised the cap twice by explicit decision (3 → 4 after round 3's ITERATE, 4 → 5 after round 4's), each time choosing another round with review over accepting the design as it stood.
- **Step 7**: round 5 returned GO with five `minor` notes and four recommendation bullets; every one is `design-internal`, so `design-writer` folded them in and design-review did not run again.
- **Step 8, subtask 1**: also fixed a second hard-coded `goose_db_version` count in `internal/store/migrate_process_test.go` (`TestMigrate_ConcurrentApplyUnderSameLockIDAppliesOnce`) that the design's file list for subtask 1 did not name — caught by the full `go test ./internal/store/...` gate.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 5 | The peaceful-home guard's subject is "any **relation** that … declares a column named `maze_id`, **whatever its primary key**" | design-internal | folded | design § Approach D1 + § Test Design subtask 3 @ b48ce14 — scan narrowed to `table_type = 'BASE TABLE'` / `relkind = 'r'`, with `chat_knowledge` named as what an unfiltered scan would pick up |
| G2 | 5 | AC12 has two clauses: a joiner brings prior discoveries in, **and** "a player who is a member of two chats has each discovery counted in both" | design-internal | folded | design § Test Design subtask 2 @ b48ce14 — scenario (c′), one player in chats A and B, one cell, visible in both |
| G3 | 5 | D2 argues the minimal projection is safe because "a later `first_discovered_at` is an **append**" | design-internal | folded | design § Approach D2 @ b48ce14 — `player_id` stated as deliberately outside the projection, reachable by joining `node_discovery` |
| G4 | 5 | The § Risks row on pre-existing chats covers the case where **no** `my_chat_member` ever arrives | design-internal | folded | design § Approach D6 + § Risks + § Test Design subtask 8 @ b48ce14 — promotion-first path named, scenario (g′) added, presence stated as written on every handled update |
| G5 | 5 | Two incidental factual claims carry no tag | design-internal | folded | design § Risks + § Approach D1 @ b48ce14 — the host clause dropped; the combat sentence narrowed to "no combat or looting **function** exists" with its measurement tag |
| G6 | 5 | Notes 1 and 4 are the two worth folding in carefully | design-internal | folded | discharged by G1 and G4 |
| G7 | 5 | Note 2 is a single scenario line; without it AC12's multi-chat clause ships unpinned | design-internal | folded | discharged by G2 |
| G8 | 5 | The design's § Open questions (the supergroup-upgrade chat-id change) does reach `ai-docs/deferred/_inbox.jsonl` | design-internal | folded | no design edit owed — Step 12 sub-step 5 parses the design; orchestrator confirmed none of the five notes matches a trigger of the *orchestrator originates no spec row* AXIOM |
| G9 | 5 | **Round-trip required:** before Step 8, update the design doc to incorporate each note/recommendation above | design-internal | folded | design @ b48ce14 — orchestrator confirmed all five landed by reading the file, not the delegate's summary |

## Key discoveries (don't re-investigate)

- **The peaceful home is the address space, not a flag.** A home is a `scope` row and carries no `(maze, q, r)`; no relation carries both a `scope` reference and a maze-cell key, and that half has no exemptions, which is what lets AC2 rest on it.
- **`event.maze_id` carries no foreign key** (`-- no FK: an open question, not a missing table`), so a guard keyed on FK-declared maze references alone would miss the one shape the tree already ships. The guard's subject is FK **or** a column named `maze_id`, over ordinary tables only.
- **`chat_knowledge` ships with no Go reader**, following the `metric_*` precedent; its column set, order and types are a permanent contract from merge.
- **Subtask 1 turns `TestCreateOwner_chat_has_no_scope_or_accounts` red** — the test's name encodes the invariant this task reverses. It is renamed and re-pointed in the same subtask, along with the now-false comment in `internal/store/move_test.go`.
- **`go build ./...` runs after every subtask**, so a deleted constructor and its out-of-package caller must move in one subtask — this is why the gate rename and the `cmd/bot` rewire are both subtask 6.
- **`player_started` is emitted when the transaction creates the player row *or* inserts a membership row.** Attribution in `metric_activation_funnel` is by the earliest *chat-bearing* event, so a bare Start followed by a link Start would otherwise be attributable to no chat forever.
- **`bot_kicked` carries `chat_id` as a column dimension**, not only in the payload: `event` is append-only and JSONB cannot express referential integrity, so a null there is permanently unrepairable.

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

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

_None yet — Step 8 has not begun._
