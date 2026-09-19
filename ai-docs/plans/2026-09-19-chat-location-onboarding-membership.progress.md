# Progress: Chat location, deep-link onboarding, and player-to-chat membership — ACTIVE
_Updated: 2026-09-19 17:20_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-19-chat-location-onboarding-membership
**base_commit:** b48ce14778576085f52db79fcbeb6d7333decb18
**Last build:** PASS

**Issue:** #30
**Spec:** ai-docs/plans/2026-09-19-chat-location-onboarding-membership.spec.md

**current_step:** Step 9.5 — docs updated
**last_passed_gate:** make verify | 2026-09-19T14:17:17Z | fdd5a28
**entry_args:** 30

## Next action

**Do this immediately:** spawn the `self-review` subagent over `b48ce14..HEAD` (Step 10).

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
- [x] 11. Documentation and the propagation sweep  (c5b1546)

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review ran five rounds; the owner raised the cap twice by explicit decision (3 → 4 after round 3's ITERATE, 4 → 5 after round 4's), each time choosing another round with review over accepting the design as it stood.
- **Step 7**: round 5 returned GO with five `minor` notes and four recommendation bullets; every one is `design-internal`, so `design-writer` folded them in and design-review did not run again.
- **Step 8, subtask 1**: also fixed a second hard-coded `goose_db_version` count in `internal/store/migrate_process_test.go` (`TestMigrate_ConcurrentApplyUnderSameLockIDAppliesOnce`) that the design's file list for subtask 1 did not name — caught by the full `go test ./internal/store/...` gate.
- **Step 8, subtask 11**: the propagation sweep reached two files the design's file list did not name — `.claude/agents/self-review.md` and `.claude/agents/review-findings.md`, whose Chat-safety rows restated the allowlist as the whole gate. `ai-docs/propagation-groups.md` makes that mandatory rather than optional: its domain-invariant row names chat safety and both reviewers as siblings of `ai-docs/domain-invariants.md`.
- **Step 8, subtask 11**: `~/lab-private/DESIGN.md` §16.7 (player↔chat membership) was **not** edited. The owner's answers in this task's spec settle its MVP half — what makes a member, and which chat a raid starts from — but the corpus is outside subtask 11's file list, and editing it is the owner's call to make explicitly. Surfaced rather than absorbed.
- **Step 9**: `make verify` exit 0 — the target is `fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck comment-refs import-guard`, so the whole Step-9 gate list ran in it; the whole-module race gate was additionally run alone and reported 29 ok over 29 packages.
- **Step 9**: panic-index needs no row — no `panic(`, `log.Fatal` or `Must…` helper in the changed production files, with the pattern seen to match a constructed control line.
- **Step 9**: domain-invariant sweep found one `time.Now()` (`internal/ingest/loop.go:181`); it is not in this diff and measures poll duration, not a generation/combat/replay path. No postings or item movements, so no posting signature is owed.
- **Step 9**: per-AC sweep run with `-count=1` against a shared server, so no profile was replayed: 78 subtests PASS, 0 FAIL across store, onboard, ingest and chat.
- **Step 9**: the owner chose to record the settled halves of `~/lab-private/DESIGN.md` §16.7 in the corpus; committed there in place as 5b3bc41, outside this PR.
- **Step 9.5**: docs landed in subtask 11; the orchestrator ran the removal sweep itself — every live mention of `NewPoolGate` / `PlayerLookup` is a past-tense record of the rename, not a claim they exist, and KD-30 carries an *Amended by #30* clause.
- **Step 9.5**: `context-status.md` carries the entry with the literal `#TBD-at-Step-12` locator (exactly one occurrence in the file) and no tallies.

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
| AC1 | PASS — TestPresenceHandler_add, TestPresenceHandler_secondChatIsIndependent, TestEnsureOwner_chatGetsHomeScope |
| AC2 | PASS — TestPeacefulHome_shippedSchema; its halfA/halfB/surrogate controls plant a violating relation and require the scan to name it |
| AC3 | PASS — TestPresenceHandler_noChunkOrGateCreated |
| AC4 | PASS — TestOutbound_deepLinkThroughTheRealGateAndClient, TestChatStartLink_hostPathAndQuery |
| AC5 | PASS — TestStartHandler_withChatPayload |
| AC6 | PASS — TestStartHandler_secondChatMembershipKeepsFirst |
| AC7 | PASS — TestStartHandler_repeatSameChatLinkAppendsNothing, TestAddMembership_idempotentOnRepeat |
| AC8 | PASS — TestStartHandler_groupMessageCreatesNothing |
| AC9 | PASS — TestStartHandler_bareStartNoPayload |
| AC10 | PASS — TestChatKnowledge (a): exact-set equality, so "and nothing else" is asserted, not implied |
| AC11 | PASS — TestChatKnowledge (b): chat A unchanged by a non-member's discovery, and chat B holds none of A's cells |
| AC12 | PASS — TestChatKnowledge (c) asserts empty BEFORE the membership row, then the cell after; (c′) gives one cell to both chats |
| AC13 | PASS — TestGate_removedThenReAddedChat (refused after removal, nothing positive cached) + TestPresenceHandler_removalAndReAdd (chat, home and membership intact) |
| AC14 | PASS — TestPresenceHandler_removalAndReAdd, TestGate_removedThenReAddedChat (fourth transition) |
| AC15 | PASS — TestPresenceHandler_add, TestStartHandler_withChatPayload, TestPresenceHandler_removalAndReAdd |
| AC16 | PASS — TestPresenceHandler_removalAndReAdd: chat_id read as a column joined back to owner, not from the payload, plus actor, both statuses and update_id |
| AC17 | PASS — TestFunnel_addThenStartAttributesPlayer, TestFunnel_bareStartThenLinkAttributesFirstChat |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `internal/store/migrations/00010_chat_home_presence_membership.sql` — the `home` scope definition and its backfill, `chat_presence`, `chat_membership`, the `chat_knowledge` view
- `internal/store/catalog.go`, `views_test.go`, `migrate_test.go`, `migrate_process_test.go` — the hard-coded schema manifests the migration moves
- `internal/store/owner.go`, `owner_test.go` — `EnsureOwner`; the chat-scope test renamed and re-pointed
- `internal/store/move_test.go` — the comment the seeded row falsified
- `internal/store/chat_knowledge_test.go`, `peaceful_home_test.go` — the view's behaviour and the AC2 instrument
- `internal/chat/**` — presence, membership and the pool-backed `DestinationLookup`
- `internal/ingest/gate.go`, `errors.go`, `doc.go`, `loop.go`, `gate_test.go`, `guards_test.go` — the rename, the new branch order, and the prose that still called the allowlist sufficient
- `internal/onboard/**` — the payload codec, the link builder, both handlers, the outbound and funnel tests
- `internal/testdb/server.go`, `server_test.go` — `Binaries` raised once per new database-backed binary
- `cmd/bot/assemble.go`, `assemble_test.go`, `smoke_test.go` — both routes registered
- `ai-docs/coverage-ratchet.txt` — lowered 91.95 → 91.34 in the subtask-8 commit, justified there
- `ai-docs/key-decisions.md` — KD-50 to KD-55 under a new *Chat front door* section, plus KD-30's amendment clause
- `ai-docs/domain-invariants.md` — new § 10; § 5's no-balance-mechanic paragraph, the shipped-view obligations and the funnel-attribution paragraph; § 6's two gate bullets
- `ai-docs/process-lifecycle.md` — start-up steps 11 and 12
- `ai-docs/context.md` — the layout paragraph (both new packages, the gate, the store clause), the status date and the Code bullet
- `ai-docs/context-status.md` — the per-task entry, its PR locator still `#TBD-at-Step-12`
- `AGENTS.md` — § Domain Rules' chat-safety clause
- `.claude/agents/self-review.md`, `.claude/agents/review-findings.md` — the Chat-safety row (Review group + the domain-invariant propagation row)
