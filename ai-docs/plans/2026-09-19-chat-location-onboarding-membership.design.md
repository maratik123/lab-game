# Design: Chat location, deep-link onboarding, and player-to-chat membership

**Issue:** #30
**Date:** 2026-09-19

## Approach

The game's front door is two Bot API updates and the link between them. `my_chat_member`
puts a group chat into the game and takes it out again; a `message` carrying `/start` in a
private chat puts a player into the game and — when the Start arrived through a chat's deep
link — records that player's membership of that chat. Everything else in the spec hangs off
those two: the chat's home, the peaceful rule, what a chat knows, the events this task owes
and the activation funnel.

Nothing here moves a balance. No `journal_entry`, no posting, no basis document, and so no
row in the posting-signature declared set — the same shape `chunk_created` already ships
`[measured 8e157d1:ai-docs/key-decisions.md:185 · grep -o "Chunk creation moves no balance[^.]*\." → "Chunk creation moves no balance, so it declares no posting signature and adds no row to the declared set internal/contract holds (KD-42)."]`. Creating an owner writes zero-balance `account_balance`
rows, which is account creation, not a balance move
`[measured 8e157d1:internal/store/owner.go:166-169 · read → the only balance-table write in CreateOwner is INSERT INTO account_balance (account_id) VALUES ($1), guarded by the definition's controlled flag]`.
No tuning value is needed either, so this task adds no balance key and no configuration key
at all.

### D1 — A chat's home is the chat owner's own `home` scope, and that is where the peaceful rule lives

`scope_definition` already carries the `(code, owner_kind)` catalog `store.CreateOwner`
walks: it creates every scope whose `owner_kind` matches the owner being created, then every
account of those scopes
`[measured 8e157d1:internal/store/owner.go:87-96 · read → the CTE inserts scope rows for every scope_definition of the kind, then accounts joined from account_definition]`.
Chats have no scope row today — the seeded definitions are `world`/`world`,
`attributes`/`player` and `backpack`/`player`
`[measured 8e157d1:internal/store/migrations/00001_ledger_core.sql:88 and 00007_item_machine.sql:21 · grep -n "INSERT INTO scope_definition" → ids 1 and 2, then id 3]`.
Seeding one more row — `home`, owner kind `chat`, at the next free id — is therefore the
whole of "adding the bot to a chat creates that chat's location": `CreateOwner` needs no
edit, the `scope_singleton_key` unique constraint makes the home singular per chat by
construction, and the home is already an address in the holder space the item machine uses,
which is where buildings and contributions attach later (`~/lab-private/DESIGN.md` §6.2).
The home carries no `account_definition` row: nothing posts against a home yet, which is the
same shape the biome resource kinds shipped in (KD-45).

**The peaceful rule is the address space, not a flag.** The design corpus puts the home
outside the game geometry — it lives "in the ordinary world", touching the mazes only through
gates, and it is a fully peaceful zone
`[measured ~/lab-private@4200ebc:DESIGN.md §2.1 · grep -o "Дом чата. Полностью мирная зона ([^)]*)" → "Дом чата. Полностью мирная зона (PvP и мародерство невозможны)"]`.
PvP and looting are acts at a position in a maze; a position in a maze is `(maze, q, r)`, the
key `chunk` and `node_discovery` are built on
`[measured 8e157d1:internal/store/migrations/00009_world_persistence.sql:55-63 · read → node_discovery PRIMARY KEY (maze_id, q, r, player_id)]`.
A home has no such address and cannot be given one: the only column in the schema that puts a
chat anywhere inside a maze is `chunk.gate_chat_id`, and a gate is not a home — AC3 leaves
this task placing no gate at all. So a mechanic that fights or loots **without asking**
cannot reach a home, because there is no home for its position argument to name. That is the
"cannot be bypassed" form, and § Test Design gives it an instrument with a constructed
positive control rather than leaving it as an assertion.

*Rejected: a `chat_home` table with a `peaceful boolean` (generated always true).* A column
that is always true only helps a mechanic that asks, and a mechanic that asks was never the
risk; it also invents a schema shape for PvP and looting tables that are out of the MVP and
not designed, which the data-contract carve-out makes expensive to undo.
*Rejected: a source-level guard forbidding a home in a combat signature.* There is no combat
and no looting code in the tree, so such a guard would scan an empty corpus and report clean
for every possible tree — the failure mode `AGENTS.md` § Patterns 2 names.

### D2 — What a chat knows is a view, `chat_knowledge`, over membership and personal discoveries

The design corpus states chat knowledge as the union across members
`[measured ~/lab-private@4200ebc:DESIGN.md §2.4, §16.7 · grep -o "знание чата = union по участникам" → present]`,
and #29's approved spec deferred exactly that view here because the membership it unions over
is this task's
`[measured gh:maratik123/lab-game#29, read 2026-09-19 · gh issue view 29 --json comments → "The union-by-chat knowledge view. The membership it unions over belongs to #30 … the view lands with #30's membership"]`.
A view rather than a table is what makes AC12 free: a membership recorded today brings
every discovery that player already had into that chat's knowledge, with nothing to back-fill
and nothing to keep in step. `SELECT DISTINCT` over `chat_membership ⋈ node_discovery` on
`player_id`, projecting the chat, the maze and the cell.

The view ships with **no Go reader**. The radar and the Mini App that render it are out of
scope, and the module's other read-only views — the `metric_*` family a Grafana datasource
reads — have no Go reader either, so a reader invented here would have no caller. Its column
set, order and types become a permanent contract the moment it merges
(`AGENTS.md` § API Stability carve-out); the projection is chosen minimal so a later
`first_discovered_at` is an append, which `CREATE OR REPLACE VIEW` allows, and the row set does
not move when it is added.

### D3 — Two packages: `internal/chat` for the facts, `internal/onboard` for the front door

`internal/chat` owns the two new tables and nothing Telegram-shaped: the bot's presence in a
chat, the membership rows, and the read the outbound gate needs. `internal/onboard` owns the
deep link and the two `ingest.Handler` implementations that translate a `telego.Update` into
`chat` and `store` calls. The split is the one KD-46 drew between `internal/world`
(persistence) and the raid edges that call it, and it keeps SQL and update parsing out of each
other's files.

Not `internal/store`: that package is the ledger's write path and the item machine, and the
persisted world was refused a home there for exactly that reason
`[measured 8e157d1:ai-docs/key-decisions.md:149 · grep -n "KD-46 —" → "not internal/store, whose package comment scopes it to the ledger, the item machine and the event log. The migration takes the next free number in internal/store/migrations, the only directory Migrate embeds."]`. The one function that
does belong in `store` is `EnsureOwner` — find-or-create keyed on `(kind, telegram_id)`,
delegating to `CreateOwner` on a miss — because both the chat handler and the Start handler
need it and it is `owner`'s own concern; two call sites with a third (the raid start) visible
in #36 is the shape the ≥3-site rule says to lift rather than copy.

Import edges: `chat → store`; `onboard → ingest, chat, store, telego`; `cmd/bot → onboard,
chat`. `internal/ingest` gains no new import (see D4); Go refuses an import cycle at compile
time, so the whole-module build is the gate that judges this edge set — `[derived → go build
./... green on subtask 10]`.

### D4 — The bot's presence in a chat is a row, and the outbound gate reads it

AC13 says that after the bot is removed the bot sends that chat **nothing**. The seam where
that cannot be bypassed is the outbound gate: today a destination chat id is allowed outright
when it is in `ALLOWED_CHAT_IDS`, and otherwise allowed when a player owner row exists for it
`[measured 8e157d1:internal/ingest/gate.go:102-128 · read → allowKnown: allowlist hit returns nil, then the cache, then PlayerLookup]`.
An allowlisted chat the bot was removed from would still be written to. So the gate's chat
branch gains a presence requirement: **allowlisted AND the bot is currently present**. The
allowlist stays an outer bound — it becomes necessary-but-not-sufficient for a chat
destination, never weaker — and the player carve-out is untouched, so a DM to a player costs
no new query. AC14 falls out: a re-add flips the row back and the same destination is allowed
again.

The presence answer is **not cached**. The gate's existing cache is positive and lives as long
as the gate, which is correct for "this telegram id is a player" (a player never stops
existing) and wrong for presence (a removal must take effect at once). Chat traffic is rare by
design — the notification budget is a standing design obligation
(`~/lab-private/DESIGN.md` §1) — so one query per outbound chat call is the right trade.

`ingest.PlayerLookup` becomes `ingest.DestinationLookup`, carrying the player question and the
presence question; `ingest.NewPoolGate` and its unexported pool-backed lookup are **deleted**
and `internal/chat` supplies the implementation the composition root injects through the
already-exported `ingest.NewGate`. That keeps the transport-side package free of a domain
import and puts the wiring where every other subsystem is wired. No compat shim is left
behind (`AGENTS.md` § API Stability).

### D5 — The deep link: a namespaced `start` payload carrying the chat's telegram id

`https://t.me/<bot_username>?start=<payload>`; the parameter is at most 64 characters drawn
from `A-Za-z0-9_-`, and the bot receives it as the text of a `/start` message
`[measured core.telegram.org/bots/features#deep-linking, fetched 2026-09-19 · WebFetch → "https://t.me/<bot_username>?start=<parameter>", "A-Z, a-z, 0-9, _ and -", "The parameter can be up to 64 characters long", bot receives "/start airplane"]`.
A group chat id is a signed decimal well inside that budget and its `-` is an allowed
character, so the payload is a fixed prefix followed by the chat's telegram id. The prefix
reserves the namespace: a later link kind (a raid invite, a referral) is a new prefix and a
new parser branch, never a re-reading of an existing payload. The parser refuses anything it
does not recognise.

**The bot's username is the sender's input, and this task adds no sender.** `telego`'s own
`Bot.Username()` is not usable here: it resolves through a `GetMe` on a fresh root context
and reports the empty string on failure
`[measured telego@v1.11.2 · sed -n '133,155p' bot.go → "func (b *Bot) updateMe() { me, err := b.GetMe(context.Background()) … } // Username returns bot username … if error occurs username will be empty"]`,
which turns an outage into silently dead links. Resolving the username at start-up instead
would add a synchronous Bot API dependency to an all-fatal start-up order
(`ai-docs/process-lifecycle.md` § 2) for a value nothing in this task consumes, and a
configuration key would be a second spelling of the token's own identity that can drift from
it. So the link builder takes the username as an argument, exactly as `internal/world`
shipped with no production caller at all (KD-46), and the task that sends notifications
chooses and wires the source. AC4 is still proved end to end: § Test Design pushes a real
`sendMessage` carrying the link through the real client and the real gate.

**No welcome message is sent from the handler.** A handler must not issue an outbound call
whose permission rests on a row its own uncommitted transaction created
`[measured 8e157d1:internal/ingest/router.go:70-80 · read → "A Handler MUST NOT issue an outbound Bot API call whose permission rests on a row its own uncommitted transaction created"]`,
and the presence row the gate would read is created by that very transaction. The designed
route is the outbound queue, which is another issue's.

### D6 — Every event this task writes is conditioned on a state change, so a redelivery writes none

The ingest loop settles an update by advancing the offset inside the handler's own
transaction; a crash between handling and commit means the same update is delivered again, and
the loop imposes no per-update idempotency of its own
`[measured 8e157d1:internal/ingest/attempt.go:66-110 · read → attemptOnce begins a tx, calls Handle, advances the offset and commits; the only dedupe branch is store.ErrAlreadyPosted, which a handler raises by posting]`.
`store.AppendEvent` is not idempotent by construction — two equal `Event` values write two
rows
`[measured 8e157d1:internal/store/event.go:34-37 · read → "Event carries no occurrence-time field … and no idempotency key: unlike PlayerOperation, two Post or AppendEvent calls built from equal Event values write two distinct event rows"]`.
The handlers therefore emit **only** where committed state actually changes:

- `bot_added_to_chat` / `bot_kicked` — emitted iff the bot's presence in the chat changes.
  The prior value is the stored row when one exists, and otherwise the update's own
  `old_chat_member` membership, so the very first update the bot ever sees for a chat is
  judged against what Telegram says the prior state was rather than against an absence. A
  promotion (`member` → `administrator`) is not a change and emits nothing; a redelivery finds
  the stored row already agreeing and emits nothing.
- `player_started` — emitted iff the player owner row is created in this transaction. A second
  Start (AC7) and a Start through a second chat's link (AC6) record membership and emit
  nothing, which is also what the activation funnel wants: it attributes a player to the chat
  of that player's **earliest** `player_started`
  `[measured 8e157d1:internal/store/migrations/00003_event_log.sql:105-110 · read → "SELECT DISTINCT ON (e.player_id) … WHERE e.type = 'player_started' AND e.player_id IS NOT NULL AND e.chat_id IS NOT NULL ORDER BY e.player_id, e.ts, e.chat_id"]`.

Telego's `ChatMember` interface answers the presence question itself — `MemberIsMember()` is
true for owner, administrator and member, is the restricted member's own `IsMember`, and is
false for left and banned
`[measured telego@v1.11.2 · grep -n -A 3 "func (c \*ChatMember.*) MemberIsMember() bool" types.go → true, true, true, c.IsMember, false, false]`.
The handler uses it rather than hand-rolling a status table (`AGENTS.md` § Dependency
Versions), and keeps `MemberStatus()` for the event payload. A `nil` chat member on a
malformed update is an error return, never a panic — the panic index is empty and this task
keeps it so
`[measured 8e157d1:ai-docs/panic-index.md · tail -3 → the table body is the placeholder row "| — | — | — |", with no entry above it]`.

### D7 — What each event carries

Each is already registered in the `event_type_definition` catalog, so no dictionary
migration is owed
`[measured 8e157d1:internal/store/migrations/00003_event_log.sql:21,22,36 · grep -n → (1, 'bot_added_to_chat', 'low_volume'), (2, 'player_started', 'low_volume'), (16, 'bot_kicked', 'low_volume')]`.

- `bot_added_to_chat` — dimension `chat_id` (the chat owner's id); no player dimension,
  because the actor of the add need not be a player in the game and the column is an `owner`
  foreign key. Payload names the chat's telegram id, its type, its title, the actor's telegram
  id, the update's own date, and the old and new statuses.
- `player_started` — dimensions `player_id` and, when the link resolved to a chat that exists
  in the game, `chat_id`. Payload names the player's telegram id and, when a payload was
  present, the chat telegram id the link carried — whether or not it resolved, so a link
  naming a chat the bot was never added to is visible rather than silent.
- `bot_kicked` — the terminal product metric, every occurrence of which gets a post-mortem
  (`~/lab-private/DESIGN.md` §13.3), so its payload answers AC16 on its own and depends on
  nothing the removal ended: the chat's telegram id, type and **title** (no `getChat` is
  possible afterwards), the actor's telegram id, the update's own date, the old and new
  statuses, and the update id.

Event `ts` is the database's `now()` — the write instant — and the update's own date is a
payload key, so "when" in AC16 is the act's time rather than the loop's.

### D8 — Which updates are handled, and what is deliberately not membership

The owner settled that only a Start through a chat's link makes a member
(`prior_qa` round 1: «Только Start-ссылка»), so the `message` route exists to read `/start` in
a **private** chat and does nothing else: a group message creates no player, no membership and
no event (AC8). All game commands happen in DM by design (`~/lab-private/DESIGN.md` §1), and
the bot cannot write to anyone who has not pressed Start
`[measured ~/lab-private@4200ebc:DESIGN.md §1 · grep -o "Бот не может писать в личку тому, кто не нажал Start" → present]`,
which is the whole reason the deep link exists.

`my_chat_member` is handled for `group` and `supergroup` chats only; a private or channel
`my_chat_member` is a no-op, since a settlement is a group chat. Registering these two routes
is also what puts them in the loop's transmitted `allowed_updates` — `my_chat_member` is not a
default update type and arrives only because a route asks for it
`[measured 8e157d1:internal/ingest/loop.go:136-146 · read → allowedUpdates is built from Router.Kinds(), with a reserved sentinel only while the route set is empty]`.

Both handlers write through inserts that select the owner row by **kind**, the shape
`world.RecordDiscovery` already uses, so a membership whose chat is not a chat or whose player
is not a player is unwritable rather than merely unwritten
`[measured 8e157d1:internal/world/discovery.go:14-22 · read → "INSERT INTO node_discovery … SELECT $1, $2, $3, $4 FROM owner WHERE id = $4 AND kind = 'player' ON CONFLICT … DO NOTHING"]`,
and the ambiguous zero-rows case is disambiguated by reading the owner's kind exactly as that
function does.

### D9 — The schema change, forward-only

One forward migration adds the `home` scope definition, `chat_presence` (one row per chat, the
current answer, with the event log carrying the history), `chat_membership` (keyed on the pair,
accruing only) and the `chat_knowledge` view. It rewrites nothing and renames nothing, so rows
written before it are unaffected and there is no deploy window in which two shapes are read.
**Rollback** is the forward-only posture KD-46 already states
`[measured 8e157d1:ai-docs/key-decisions.md:149 · grep -o "The migration is forward-only[^.]*\." → "The migration is forward-only, so rolling the binary back leaves the tables in place and unread, and rolling forward again finds them."]`:
rolling the binary back leaves the tables and the seeded row in place and unread — the older
binary neither creates chats nor reads presence — and rolling forward again finds them. The one reader an older binary shares
is `store.CreateOwner`, which walks `scope_definition` generically and would create the home
scope for any chat it created; it creates none.

`chat_membership.player_id` gets an index of its own: it trails the primary key, so the
module's foreign-key coverage test would otherwise fail it
`[measured 8e157d1:internal/store/fkcover_test.go:26-33 · read → uncoveredFKs returns every FK whose referencing columns are not covered by the leading columns of some index on the same relation]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Forward migration: the `home` scope definition, `chat_presence`, `chat_membership` and its player index, and the `chat_knowledge` view; grow the Go catalog mirror and the exact-view-set list so both mirror tests pass | `internal/store/migrations/00010_chat_home_presence_membership.sql`, `internal/store/catalog.go`, `internal/store/views_test.go` | — |
| 2 | Behaviour tests for `chat_knowledge`: the union, the non-member exclusion, and the join-later case | `internal/store/chat_knowledge_test.go` | 1 |
| 3 | The peaceful-home instrument: a schema guard asserting no relation gives a chat a maze position, with its constructed positive control | `internal/store/peaceful_home_test.go` | 1 |
| 4 | `store.EnsureOwner`: find-or-create keyed on `(kind, telegram_id)`, delegating to `CreateOwner` on a miss, with the existing-owner read returning the same shape | `internal/store/owner.go`, `internal/store/owner_test.go` | 1 |
| 5 | `internal/chat`: package doc, errors, presence write and read, membership write, and the `DestinationLookup` implementation over a pool | `internal/chat/doc.go`, `internal/chat/errors.go`, `internal/chat/presence.go`, `internal/chat/membership.go`, `internal/chat/lookup.go`, `internal/chat/main_test.go`, `internal/chat/presence_test.go`, `internal/chat/membership_test.go` | 1, 4 |
| 6 | Outbound gate: `PlayerLookup` → `DestinationLookup`, the presence requirement on the chat branch, deletion of `NewPoolGate` and its pool-backed lookup, and the gate tests for refusal after removal and allowance after a re-add | `internal/ingest/gate.go`, `internal/ingest/gate_test.go`, `internal/ingest/guards_test.go` | 5 |
| 7 | `internal/onboard`: the namespaced `start` payload codec and the deep-link URL builder, with their refusals | `internal/onboard/doc.go`, `internal/onboard/errors.go`, `internal/onboard/link.go`, `internal/onboard/link_test.go`, `internal/onboard/main_test.go` | — |
| 8 | The `my_chat_member` handler: chat kind filter, owner ensure, presence change, and the two events with their payloads | `internal/onboard/presence_handler.go`, `internal/onboard/presence_handler_test.go` | 5, 7 |
| 9 | The `/start` handler: private-chat filter, command parse, player ensure, membership, and `player_started` | `internal/onboard/start_handler.go`, `internal/onboard/start_handler_test.go` | 5, 7 |
| 10 | Composition root: register both routes and inject the chat-backed gate lookup; raise `testdb.Binaries` for the two new database-backed binaries; the outbound end-to-end test carrying the link, and the activation-funnel test over the two handlers' own events | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go`, `internal/testdb/server.go`, `internal/onboard/outbound_test.go`, `internal/onboard/funnel_test.go` | 6, 8, 9 |
| 11 | Documentation and the propagation sweep: the new key decisions, the architecture and status entries, the start-up step whose router is no longer empty, the peaceful-home / membership / gate-presence invariants, and a `grep -rni` sweep for every live surface restating the allowlist's old sufficiency | `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/process-lifecycle.md`, `ai-docs/domain-invariants.md`, `AGENTS.md` | 10 |

## Handoff plan

Grouping is required for every `M ≥ 1`; this design has `M = 11`, in two groups. Each group is
homogeneous by change-type, each is at most `10` consecutive subtasks, the terminal group's
size is inside `1..=10`, and the count is the fewest the dependency order and the change-type
boundary allow — the code subtasks are clustered into one group rather than interleaved with
the documentation one. Two groups is within the default maximum of `4`, so no user approval is
needed.

- **Entry into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The handoff binds at the start of every group, this first
  one included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–10 (code change-type: `*.go`, migrations). At the size cap, and
  every subtask in it changes code only.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task`
  resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — not pinned — via the `general-purpose` subagent, 1M-token window —
  subtask 11 (instructions/harness change-type: `*.md`, `ai-docs/**`, `AGENTS.md`). Terminal
  group (one subtask; within the `1..=10` range).

## Risks

- **The Postgres data-modifying CTE must create a scope that has no account definitions.**
  `CreateOwner` builds scopes and accounts in one statement and returns early when the account
  set comes back empty
  `[measured 8e157d1:internal/store/owner.go:80-120 · read → the WITH s AS (INSERT INTO scope … RETURNING) INSERT INTO account … pattern, then "if len(createdAccounts) == 0 { return owner, nil }"]`,
  which is exactly the path a chat now takes. If the CTE did not execute to completion the
  home would silently not exist. Mitigation: AC1's test asserts the home scope row directly,
  so the premise is executed rather than assumed — `[derived → the AC1 test in subtask 8]`.
- **An allowlisted chat destination stops being allowed on the allowlist alone.** Any future
  sender aimed at a chat the bot was never added to, or was removed from, is refused where it
  previously succeeded. Mitigation: no production sender exists today, the gate's own tests
  pin both directions, and subtask 11 sweeps every live document restating the old
  sufficiency — `[derived → the gate tests in subtask 6 and the propagation sweep in subtask 11]`.
- **The deep-link payload is guessable.** It carries the chat's telegram id in the clear, so
  someone who learns a chat id can become a member of that chat without ever having been in
  it. The link is a shareable capability either way, so an opaque token would stop guessing
  and not sharing. Mitigation and escape hatch: a per-chat random token is a forward migration
  adding one column plus the two readers the codec already isolates — `[derived → the payload
  codec's single encode/decode pair in subtask 7]`.
- **A group upgraded to a supergroup changes its chat id**, so the bot appears in a second
  chat with a second home and the first chat's memberships do not follow. Out of scope here;
  recorded in § Open questions rather than absorbed.
- **Two new database-backed test binaries raise the provisioned connection ceiling.**
  `testdb.Binaries` is a term of the ceiling formula and the ceiling is refused above its
  maximum
  `[measured 8e157d1:internal/testdb/server.go:252-280 · read → "ceiling = clients × Binaries × parallel × (schemaMaxConns + 1) + ceilingSlack", refused above ceilingMax]`.
  Mitigation: subtask 10 raises the constant and `TestBinaries_matchesTree` keeps it honest
  against the tree; the shared-server route's own gate reports a ceiling refusal by name —
  `[derived → the raised constant and its manifest test in subtask 10]`.
- **Comment-reference ban on the new SQL and Go.** `make comment-refs` gates `*.go` and
  `*.sql`, and a migration comment naming a design section, an issue number or a markdown path
  fails it. Mitigation: the migration's comments state what each table is and why its columns
  are shaped as they are, naming nothing outside themselves — the shape migration 00009's own
  comments already take — `[derived → make comment-refs green on the subtask-1 commit]`.
- **A redelivered update must not write a second event.** Mitigation is structural (D6): every
  emission is conditioned on a state change committed in the same transaction, and § Test
  Design replays each update to prove it — `[derived → the redelivery scenarios in subtasks 8
  and 9]`.

## Test Design

Every claim below is about a test that does not exist yet.

**Subtask 1 — migration.** Location: the module's existing store suite. Entry points: the
catalog mirror comparison, the exact-view-set test, and the foreign-key coverage test, all of
which already run against whatever the migrations create. Scenarios: the mirror fails on
either side seeding a scope definition the other does not know; the view set matches exactly;
every new foreign key is covered. Fixtures: the migrated schema pool the suite already builds.
`[derived → AC1]`

**Subtask 2 — `chat_knowledge`.** Location: `internal/store/chat_knowledge_test.go`. Entry
point: the view, queried directly, since it has no Go reader. Scenarios: (a) two members of one
chat each discover a distinct cell and the chat knows both, and a cell discovered by both
appears once — AC10; (b) a player who is a member of a second chat discovers a cell, and the
first chat does not know it — AC11; (c) discoveries recorded *before* the membership are in the
chat's knowledge as soon as the membership row exists — AC12; (d) a chat with no members knows
nothing. Fixtures: owners of both kinds, a maze row and `node_discovery` rows written as plain
SQL — the suite cannot import `internal/world`, which imports `internal/store`.
`[derived → AC10, AC11, AC12]`

**Subtask 3 — the peaceful home.** Location: `internal/store/peaceful_home_test.go`. Entry
point: a query over the catalogs that enumerates every relation carrying a reference to
`owner` and asserts none of them also carries a maze-cell key, together with the assertion that
the chat's home is a `scope` row and that `scope` carries no maze or cell column. Scenarios:
(a) the shipped schema passes; (b) **the control** — a relation created inside the test that
carries both a chat reference and a maze cell makes the guard fail, and the guard's message
names it. Without (b) a clean result would be a claim about the guard's vocabulary rather than
about the schema. Fixtures: the migrated schema pool; the control relation is created and
dropped inside the test's own schema. `[derived → AC2]`

**Subtask 4 — `store.EnsureOwner`.** Location: `internal/store/owner_test.go`. Entry point:
`EnsureOwner`. Scenarios: creates on a miss and reports creation; returns the existing owner
without creating on a hit and reports no creation; a chat owner's returned shape carries the
home scope; the refusals `CreateOwner` already makes (the World kind, an unknown kind, a nil
telegram id) are unchanged and happen before any statement. Fixtures: the store suite's own
pool. `[derived → AC1, AC5, AC7]`

**Subtask 5 — `internal/chat`.** Location: `internal/chat/*_test.go`, a new database-backed
binary standing on `internal/storetest`'s migrated pool with the module's goroutine-leak
`TestMain`. Entry points: the presence write, the presence read, and the membership write.
Scenarios: presence written for a chat and read back; a repeat write of the same value reports
no change; a flip reports a change; a presence write against an owner that is not a chat is
refused, writing nothing; a membership recorded once is idempotent on a repeat; a membership
whose chat is not a chat, or whose player is not a player, is refused, writing nothing; the
lookup answers both of the gate's questions over a pool. `[derived → AC7, AC13, AC14]`

**Subtask 6 — the gate.** Location: `internal/ingest/gate_test.go`. Entry point:
`(*Gate).AllowCall`. Scenarios: an allowlisted chat the bot is present in is allowed; the same
chat after a removal is refused, and the refusal names the chat; the same chat after a re-add
is allowed again, proving nothing positive was cached across the removal; a present chat that
is not allowlisted is refused; a player destination is allowed and still served from the
positive cache without a second lookup; `ChatNone` is allowed and `ChatUnknown` refused, both
unchanged. Fixtures: a stub `DestinationLookup` carrying per-question call counters, which is
what makes "not cached" observable. `[derived → AC13, AC14]`

**Subtask 7 — the deep link.** Location: `internal/onboard/link_test.go`. Entry points: the
payload encoder, the payload parser and the link builder. Scenarios: a negative supergroup id
round-trips through encode and parse; the encoded payload contains only characters the Bot API
admits and stays inside its length budget — asserted against the documented character class,
not against a remembered one; the parser refuses an empty payload, a payload with the wrong
prefix, a payload whose remainder is not an integer, and a payload carrying trailing text; the
built link's host, path and query are asserted through a parse of the result rather than by
string equality, so a different but equivalent spelling is not a false failure; an empty
username is refused. `[derived → AC4]`

**Subtask 8 — the `my_chat_member` handler.** Location:
`internal/onboard/presence_handler_test.go`. Entry point: `Handle` on the loop's own
transaction. Scenarios: (a) an add for a group chat creates the chat owner, its home scope and
a present row, and appends `bot_added_to_chat` with the chat dimension set — AC1; (b) a second
chat added yields a second chat with a home of its own, and neither is the other — AC1; (c) no
chunk and no gate row exists for either chat afterwards — AC3; (d) a removal flips presence,
appends `bot_kicked`, and leaves the chat, its home, every membership and every discovery in
place — AC13; (e) a re-add flips presence back against the **same** chat owner id, with what
the chat accumulated intact — AC14; (f) the removal event's payload answers which chat, when,
by whom and by what status transition, read back from the row alone — AC16; (g) a promotion
from member to administrator changes nothing and appends nothing; (h) the same update handled
twice appends nothing the second time — D6; (i) a `my_chat_member` for a private chat and one
for a channel are no-ops; (j) a malformed update with no new chat member returns an error and
does not panic. Fixtures: `telego.Update` values built in the test, the migrated pool, and a
helper that reads back the events of one type for a chat. `[derived → AC1, AC3, AC13, AC14,
AC15, AC16]`

**Subtask 9 — the `/start` handler.** Location: `internal/onboard/start_handler_test.go`.
Entry point: `Handle`. Scenarios: (a) a private `/start` with a chat payload creates the
player, records the membership and appends `player_started` with both dimensions — AC5; (b) an
existing player starting through a second chat's link is a member of both, and the first
membership is untouched — AC6; (c) a second Start through the same chat's link leaves one
player and one membership, and appends no second event — AC7; (d) a group message from a
player who never pressed Start creates nothing at all — AC8; (e) a private `/start` with no
payload creates the player, appends `player_started` with no chat dimension, and records no
membership — AC9; (f) a payload naming a chat the bot was never added to creates the player,
records no membership, and leaves the chat telegram id visible in the event payload; (g) a
malformed payload is treated as no payload rather than as an error that dead-letters the
update; (h) the same update handled twice appends nothing the second time — D6; (i) a
`/start` in a group chat is a no-op. Fixtures: as subtask 8. `[derived → AC5, AC6, AC7, AC8,
AC9, AC15]`

**Subtask 10 — wiring and the two cross-cutting proofs.** Locations: `cmd/bot/assemble_test.go`,
`internal/onboard/outbound_test.go`, `internal/onboard/funnel_test.go`. Entry points: the
assembled router's registered kinds; a real `sendMessage` through the real Telegram client
against the in-process fake Bot API server, with the real gate installed; the activation-funnel
view. Scenarios: (a) the assembled router carries exactly the two kinds, and the loop's
transmitted `allowed_updates` therefore carries them — the existing assertion that the router
is empty is replaced, not deleted; (b) a message sent to a present, allowlisted chat carries an
inline button whose URL is the deep link for that chat, asserted from the request body the fake
server received, and the same send to the same chat after a removal is refused by the gate —
AC4 and AC13 together; (c) handling an add update and then a Start update through that chat's
link makes the funnel report that chat with the player attributed to it, computed from the
recorded events alone — AC17. Fixtures: the fake Bot API server, the migrated pool, and the
assemble options the bot suite already uses. `[derived → AC4, AC15, AC17]`

**Subtask 11 — documentation.** No test. The gates that bind it are the repository link check
and the `grep -rni` sweep the Propagation Rule requires, whose empty output after the edit is
the evidence. `[derived → the sweep's own empty result recorded in subtask 11]`

## Open questions

- **A group upgraded to a supergroup gets a new chat id, and Telegram reports it as a distinct
  chat.** The bot then holds a chat and a home on each side of the upgrade, with the
  memberships recorded before it stranded on the old one, for what the players experience as
  one settlement. Nothing in `~/lab-private/DESIGN.md` §2.1 or §16.7 settles what
  should happen, the issue body does not raise it, and neither does #29's or #36's; looked for
  in `~/lab-private/DESIGN.md` §1, §2.1, §16.7 and in the bodies and comments of #29, #36 and
  #118. It is out of this task's scope either way — the question is whether it becomes its own
  issue now or waits for the first observed migration.
