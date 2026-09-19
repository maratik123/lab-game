# Progress: Chat location, deep-link onboarding, and player-to-chat membership — ACTIVE
_Updated: 2026-09-19 17:20_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-19-chat-location-onboarding-membership
**base_commit:** b48ce14778576085f52db79fcbeb6d7333decb18
**Last build:** PASS

**Issue:** #30
**Spec:** ai-docs/plans/2026-09-19-chat-location-onboarding-membership.spec.md

**current_step:** Step 11 — review fixes complete (Round 1)
**last_passed_gate:** make verify | 2026-09-19T14:40:26Z | 2b664ef
**entry_args:** 30

## Next action

**Do this immediately:** re-spawn `self-review` for Round 2 over `b48ce14..HEAD`.

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
- **Step 11 (Round 1)**: all four open findings fixed, none objected; the `.go` batch was authored by `code-writer` in Mode B and committed by the orchestrator after reading the diff.
- **Step 11 (Round 1)**: the orchestrator re-ran the `date` mutation itself — blanking it reds both presence tests and the failure lines name the new assertion, so AC16's "when" is pinned rather than merely present. Source restored from a cp backup and confirmed absent from `git diff --name-only`.
- **Step 11 (Round 1)**: every register row's own verifying command re-run — chat_title occurrences 0 → 4, SELECT in start_handler.go 1 → 0, no `r.Body.Read` left, `go doc` states the Accounts contract. `make verify` 0, five AC packages ok, ratchet 91.17% holds against 91.34%.

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
| R1-1 | round 1 | major | fixed@2b664ef | `grep -c chat_title internal/onboard/presence_handler_test.go` (0 = defect); mutation: blank `ChatType`/`ChatTitle`/`Date` in `presenceEventPayload`, then `go test -count=1 ./internal/onboard/` — currently exit 0, 0 FAIL |
| R1-2 | round 1 | major | fixed@2b664ef | `grep -c SELECT internal/onboard/start_handler.go` (1 = defect, 0 = fixed) |
| R1-3 | round 1 | minor | fixed@2b664ef | `grep -n 'r.Body.Read' internal/onboard/outbound_test.go` |
| R1-4 | round 1 | minor | fixed@2b664ef | `go doc ./internal/store EnsureOwner` |
| R1-5 | round 1 | minor | accepted@1 — the design's subtask-3 controls (e)/(f) say "both halves must go red on it", but the control relations carry a `scope (id)` reference and no `owner (id)` one, so Half B cannot be red on them and `TestPeacefulHome_surrogateKeyControls` exercises Half A only. Both halves derive their subject set from the single `mazeTouchingRelations` extractor, and the test asserts the control relation IS in that set — which is the property "would escape both halves" turns on. No code or design change owed. | `go test ./internal/store/ -run TestPeacefulHome` |
| R1-6 | round 1 | nit | accepted@1 — `TestStartHandler_groupMessageCreatesNothing` merges design scenarios (d) and (i) into one case sending `/start` in a group. The private-chat filter at `start_handler.go:59` returns before the text check, so an ordinary group message and a group `/start` take the identical branch; AC8 is covered in the stronger form. | `go test ./internal/onboard/ -run TestStartHandler_groupMessageCreatesNothing` |
| R1-7 | round 1 | nit | accepted@1 — `start_handler.go:67-69` (`msg.From == nil`) has no test. Below severity floor; the ratchet holds at 91.26% against 91.34%. | `make cover-ratchet` |
| R1-8 | round 1 | nit | accepted@1 — `/start@botusername` in a private chat is a no-op (`start_handler.go:64`). Telegram's deep link delivers `/start <payload>`; the `@bot` suffix is a group-chat spelling and no AC covers it. | `grep -n 'startCommand+\" \"' internal/onboard/start_handler.go` |
| R1-9 | round 1 | nit | accepted@1 — `internal/chat/lookup.go:20` names "the ingest package's DestinationLookup interface". Contract-stating rather than a pointer, and the same shape as the tree's shipped precedent (`internal/ingest/gate.go`: "the Telegram client's outbound Gate interface"); `make comment-refs` green. | `make comment-refs` |

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

## Self-Review (Round 1)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/onboard/presence_handler_test.go:216 (subject: internal/onboard/presence_handler.go:99-107) | major | **AC16 is recorded PASS on a partial execution of its own scenario.** Design D7 enumerates seven payload fields as what makes `bot_kicked` answer AC16 "on its own and depends on nothing the removal ended": chat telegram id, **type**, **title**, actor telegram id, **the update's own date**, old/new statuses, update id. `TestPresenceHandler_removalAndReAdd` asserts four of them; `chat_type`, `chat_title` and `date` are asserted **nowhere in the tree** (`grep -rn 'chat_title\|\"date\"\|chat_type' internal/onboard/` matches only the struct tags at `presence_handler.go:22-25` and an unrelated fixture, with the pattern seen to match a constructed control line). Mutation proof: blanking all three in `presenceEventPayload` builds and leaves `go test -count=1 ./internal/onboard/` at **exit 0, 0 FAIL**. `date` is AC16's "when"; `chat_title` is the field D7 singles out as unrecoverable after the fact ("no `getChat` is possible afterwards"). Fix: assert `chat_type`, `chat_title` and `date` on the `bot_kicked` row in `TestPresenceHandler_removalAndReAdd`, and on `bot_added_to_chat` in `TestPresenceHandler_add`. | ✅ Fixed |
| 2 | internal/onboard/start_handler.go:92-94 | major | **Raw SQL inside the update-parsing file contradicts D3, and the contradiction has shipped as a false sentence in a durable document.** The handler issues `tx.QueryRow` against a `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1` literal, which is the only SQL in `internal/onboard`'s non-test code. Design D3 — and `ai-docs/key-decisions.md` KD-52, which this PR adds — state the two-package split "keeps SQL and update parsing out of each other's files". It also hand-rolls the `(kind, telegram_id)` predicate `store.EnsureOwner` already holds at `internal/store/owner.go:200-203`, which is the duplication KD-53 forbids one decision later: "a second hand-rolled query would be a silent duplicate of a security-relevant predicate, free to drift from the one the test guards". Fix: lift the read into `internal/chat` (or `internal/store`) as a named function with its own test and call it from the handler. Choosing instead to keep the inline read makes this a **Design Amendment trigger** — spawn the `design-writer` Subagent to amend `ai-docs/plans/2026-09-19-chat-location-onboarding-membership.design.md` D3 and the KD-52 sentence with it; recipe at `.claude/skills/task/SKILL.md` Step 11 fail-loud table. | ✅ Fixed |
| 3 | internal/onboard/outbound_test.go:51-52 | minor | `body := make([]byte, r.ContentLength); _, _ = r.Body.Read(body)` — one `Read` is not guaranteed to fill the buffer, and both the byte count and the error are discarded (AGENTS.md § Code Style, "Never `_ = err`"). A short read truncates `lastBody`, and AC4's assertion then fails at `json.Unmarshal`, so the test is latently flaky. The tree's own precedent is `body, _ := io.ReadAll(r.Body)` (`internal/ingest/loop_test.go:130`); this is the only `ContentLength`-sized read in the module. | ✅ Fixed |
| 4 | internal/store/owner.go:178-190 | minor | `EnsureOwner`'s doc comment does not state its return contract for `Owner.Accounts`: on a **hit** it is always nil (`owner.go:207` returns `Owner{ID, Kind, TelegramID}`), on a **miss** `CreateOwner` fills it. `TestEnsureOwner_playerScopeUnchanged` asserts six accounts on the miss path and nothing asserts the hit shape, so the asymmetry is neither documented nor pinned — a trap for the third call site D3 anticipates (#36). DOC-3 requires the return contract. | ✅ Fixed |

**Minor/nit items recorded, not raised:** 5 — entered in the register as `accepted@1` with their reasons (R1-5 peaceful-home control scope, R1-6 merged group-message scenarios, R1-7 untested `From == nil` branch, R1-8 `/start@bot` spelling, R1-9 cross-package interface name in a comment). Files: `internal/store/peaceful_home_test.go`, `internal/onboard/start_handler.go`, `internal/onboard/start_handler_test.go`, `internal/chat/lookup.go`.

### What was checked

**Gates re-run on the shipped tree (HEAD = 16a49c2), not taken from the progress file:** `go vet ./...` exit 0 · `golangci-lint run` → `0 issues.` · `make test` → GATE-GREEN, 0 `FAIL` lines · `make cover-ratchet` → `91.26% holds against 91.34% (tolerance 0.60 pp)` · `make comment-refs` green · `make import-guard` green · `make file-limits` green. The ratchet drop 91.95 → 91.34 is justified in its own commit (fd81a8a) as AGENTS.md requires.

**Design conformance.** All eleven decomposition rows' files are present; the two file-list departures (`migrate_process_test.go` in subtask 1, the two `.claude/agents/*.md` in subtask 11) are recorded in the Decisions log and the second is mandated by `ai-docs/propagation-groups.md`. GO-notes round trip: the design doc is **unchanged** across `b48ce14..HEAD`, so every `folded` row landed before the implementation diff started; G1 (the `relkind = 'r'` filter), G2 (scenario c′), G3 (`player_id` out of the projection), G4 (D6 promotion-first + scenario g′) and G5 (the narrowed combat sentence with its measurement tag) were each read in the design at HEAD. The design carries no `AC<N> verified by:` command lines (grepped), so §2's AC-verification-grep re-run has no commands to execute; the per-AC proof is the Test Design's named tests, and all 18 tests the `## AC Status` table names exist and run.

**Mutation checks (Pattern 2 — a green test is a claim about the test).** Three run, each rebuilt before the test result was read: (1) reverting `allowKnown`'s allowlist branch to allow outright → `TestGate_presenceIsNeverCached`, `TestGate_presenceLookupErrorRefuses`, `TestGate_removedThenReAddedChat` all FAIL by name; (2) narrowing `player_started`'s emission to `if !playerCreated` → `TestStartHandler_laterLinkAfterBareStart`, `TestStartHandler_secondChatMembershipKeepsFirst`, `TestFunnel_bareStartThenLinkAttributesFirstChat` all FAIL by name; (3) blanking `chat_type`/`chat_title`/`date` → suite stays GREEN, which is finding 1. All three files restored; `git status --porcelain` empty afterwards.

**Domain invariants (§ 4a).** No balance column is written outside `store.Post`/`store.Move` and no `item_movement` insert is added; no posting group, so no basis-document or zero-sum question arises and no posting signature is owed — the three event types are already in `event_type_definition` (migration 00003), which is what the telemetry row demands. No balance/tuning constant enters Go (`chatStartPrefix = "c"` is a namespace token; `startPayloadMaxLen = 64` is the Bot API's documented budget, asserted in a test). Migration 00010 renames and re-purposes nothing and re-numbers no enum — it adds `scope_definition` id 4 and two tables, forward-only. Chat safety is strengthened, not bypassed: the gate now requires allowlisted **and** present, the presence answer is read on every chat call and never cached (`TestGate_presenceIsNeverCached`), and no outbound sender is added. No `time.Now()`, unseeded `math/rand` or map-iteration order on a generation/combat/replay path — `time.Unix(cm.Date, 0)` derives from the update itself. No token, DSN or `api_id`/`api_hash` in any added file.

**Safety and correctness.** Panicking-call audit over every changed non-test `.go` file: no `panic(` / `log.Fatal*` / `log.Panic*` / `Must…` site, with the pattern seen to match a constructed control line — `ai-docs/panic-index.md` needs no row. Every error is wrapped with `%w` and an operation-naming message; `errors.Is` is used for every `pgx.ErrNoRows` and sentinel comparison; no `_ = err` outside the deferred-rollback idiom the suite already uses (finding 3 is the one exception). `ctx` is first on every new function, none stores a context, and no `context.Background()` appears on a request path. No `go` statement is added in production code, and both handlers are stateless empty structs; the race gate is recorded in the Decisions log and was green in `make verify` at fdd5a28, with no code-side change after it. No `…Unchecked` function is added.

**Doc convention.** Every exported item added carries a name-first, third-person doc comment; both new packages carry a package comment (`internal/chat/doc.go`, `internal/onboard/doc.go`). Each new sentinel (`ErrNotAChat`, `ErrNotAPlayer`, `ErrEmptyPayload`, `ErrUnknownPayloadPrefix`, `ErrMalformedPayload`, `ErrEmptyUsername`) is named in the doc of the function that returns it. No `TODO` without an issue reference, no commented-out code. `make comment-refs` green over the whole gated set; the review-judged halves (narration, a bare unqualified name used as a pointer) were read by hand across the new files — R1-9 is the one borderline case, accepted with its reason.

**Prose diff (Pattern 1 — verify every factual claim).** Re-derived rather than read: § 6's gate predicate, branch order, fall-through and never-cached presence against `gate.go:82-118`; § 10's home/membership/knowledge claims against migration 00010 and `chat_knowledge`; § 5's "three of the rows below are now discharged" against the two handlers' `store.Event` values; KD-30's `Amended by #30` clause and KD-50 to KD-55 against the shipped code; `process-lifecycle.md` steps 11 and 12 against `cmd/bot/assemble.go:328,342-346`. The propagation sweep was re-run independently (`grep -rni` over `*.go`/`*.md`/`*.yml`, with a control line): every surviving `NewPoolGate` / `PlayerLookup` mention is history (`ai-docs/plans/done/**`, this task's own design), a past-tense record (KD-30, `context-status.md`), or `TestGate_playerLookupErrorRefuses`, whose subject still exists. KD-52's "keeps SQL and update parsing out of each other's files" is the one claim that does **not** re-derive — that is finding 2.

**Progress-file fields.** `current_step`, `last_passed_gate` and `entry_args` present; `parent_skill` is correctly **omitted** per `ai-docs/templates/progress-format.md` ("omit when the writing skill IS the parent flow"), so its absence is conformance, not a gap.
