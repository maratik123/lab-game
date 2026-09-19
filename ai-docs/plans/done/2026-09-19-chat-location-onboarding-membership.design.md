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
`[measured dd3778c:ai-docs/key-decisions.md:189 · grep -n "Chunk creation moves no balance[^.]*\." → "Chunk creation moves no balance, so it declares no posting signature and adds no row to the declared set internal/contract holds (KD-42)."]`. Creating an owner writes zero-balance `account_balance`
rows, which is account creation, not a balance move
`[measured dd3778c:internal/store/owner.go:166-169 · read → the only balance-table write in CreateOwner is INSERT INTO account_balance (account_id) VALUES ($1), guarded by the definition's controlled flag]`.
No tuning value is needed either, so this task adds no balance key and no configuration key
at all.

### D1 — A chat's home is the chat owner's own `home` scope, and that is where the peaceful rule lives

`scope_definition` already carries the `(code, owner_kind)` catalog `store.CreateOwner`
walks: it creates every scope whose `owner_kind` matches the owner being created, then every
account of those scopes
`[measured dd3778c:internal/store/owner.go:87-96 · read → the CTE inserts scope rows for every scope_definition of the kind, then accounts joined from account_definition]`.
Chats have no scope row today — the seeded definitions are `world`/`world`,
`attributes`/`player` and `backpack`/`player`
`[measured dd3778c:internal/store/migrations/00001_ledger_core.sql:88 and 00007_item_machine.sql:21 · grep -n "INSERT INTO scope_definition" → ids 1 and 2, then id 3]`.
Seeding one more row — `home`, owner kind `chat`, at the next free id — is therefore the
whole of "adding the bot to a chat creates that chat's location": `CreateOwner` needs no
edit, the `scope_singleton_key` unique constraint makes the home singular per chat by
construction
`[measured dd3778c:internal/store/migrations/00001_ledger_core.sql:34 · grep -n "scope_singleton_key" → "CONSTRAINT scope_singleton_key UNIQUE (owner_id, scope_definition_id)   -- singleton + covers owner FK"]`,
and the home is already an address in the holder space the item machine uses,
which is where buildings and contributions attach later (`~/lab-private/DESIGN.md` §6.2).
The home carries no `account_definition` row: nothing posts against a home yet, which is the
same shape the biome resource kinds shipped in (KD-45).

**The seeded row reverses an invariant the ledger suite currently asserts, so that assertion
moves in the same commit.** `TestCreateOwner_chat_has_no_scope_or_accounts` reads the scope
count for a freshly created chat owner and requires zero
`[measured dd3778c:internal/store/owner_test.go:132-158 · read → CreateOwner(ctx, tx, OwnerChat, &tg) then "chat accounts = %d, want 0" and a SELECT count(*) FROM scope WHERE owner_id = $1 asserted against 0]`,
and a chat now has exactly one scope. It becomes
`TestCreateOwner_chat_has_home_scope_and_no_accounts`: **exactly one `scope` row, and it is
the `home` definition; still zero accounts.** Both halves matter — the account assertion is
what keeps "a home posts nothing" honest, and dropping it while re-pointing the scope
assertion would quietly widen what the migration is allowed to do. `internal/store/move_test.go`
carries a comment saying a chat owner has no scopes at all, which becomes false in the same
commit; `make comment-refs` is lexical and cannot see it, so it is named in the subtask rather
than left to a reviewer. Nothing else in the suite moves: the catalog mirror compares against
the Go literal this migration also edits, and the migration test's seed counts are over the
World owner.

**The peaceful rule is the address space, not a flag.** The design corpus puts the home
outside the game geometry — it lives "in the ordinary world", touching the mazes only through
gates, and it is a fully peaceful zone
`[measured ~/lab-private@4200ebc:DESIGN.md §2.1 · grep -o "Дом чата. Полностью мирная зона ([^)]*)" → "Дом чата. Полностью мирная зона (PvP и мародерство невозможны)"]`.
PvP and looting are acts at a position in a maze, and a position in a maze is the
`(maze_id, q, r)` key `chunk` and `node_discovery` are built on
`[measured dd3778c:internal/store/migrations/00009_world_persistence.sql:38,61 · grep -n "PRIMARY KEY (maze_id" → chunk PRIMARY KEY (maze_id, q, r); node_discovery PRIMARY KEY (maze_id, q, r, player_id)]`.
A home is a `scope` row, and the claim that carries the AC is therefore about `scope`:
**no relation that names a maze carries a `scope` reference, and `scope` itself carries no
maze or cell column.** `scope` is referenced by `account.scope_id` and by the item
machine's two holder columns, and none of those three declares a maze reference or a column
named `maze_id` — none of them *touches a maze*, which is the property the guard is built on
`[measured dd3778c:internal/store/migrations/00001_ledger_core.sql:40 and 00007_item_machine.sql:111-112 · grep -n "REFERENCES scope (id)" → account.scope_id, item_movement.from_holder_id, item_movement.to_holder_id]`.
So a mechanic that fights or loots **without asking** cannot reach a home: there is no home
for its position argument to name.

**Maze-touching relations do carry `owner` references, and none of them is constrained to a
non-home owner by the schema.** `chunk.gate_chat_id` is a chat's, and is a **gate** rather
than a home — AC3 leaves this task placing no gate at all. `node_discovery.player_id` is an
unconstrained `owner (id)` foreign key
`[measured dd3778c:internal/store/migrations/00009_world_persistence.sql:33,59 · grep -n "REFERENCES owner (id)" → chunk.gate_chat_id bigint REFERENCES owner (id); node_discovery.player_id bigint NOT NULL REFERENCES owner (id)]`;
what keeps a chat owner id out of it is the discovery writer's own `WHERE id = $4 AND
kind = 'player'`, which is application code, not a constraint. That hole is why the
instrument in § Test Design is a **reviewed allow list** with a reason per exempt column —
the `internal/gateguard` shape — rather than a blanket "no `owner` reference on a maze-touching
relation", which the shipped schema would fail on every one of those columns. The allow list
is the content of the guard, so it is written here rather than left to the implementor.

*Not claimed:* that a home is unrepresentable in a maze **for all time regardless of what
code does**. The schema forbids it for `scope`; for `owner` it forbids only what the allow
list does not exempt, and a new maze-touching relation with an `owner` reference turns the
guard red until someone states which owner it is.

**"Maze-touching" is deliberately wide in two directions, and both narrow readings would have
been holes.** The guard's subject is any **ordinary table** that **either** declares a column
referencing `maze (id)` **or** declares a column named `maze_id` — whatever its primary key.

*Ordinary tables, and that clause is load-bearing rather than tidy.* In Postgres a view is a
relation and `information_schema` lists it, so an unfiltered scan would pick up
`chat_knowledge` — whose own projection carries `maze_id` — and the guard would be red on
output its own dependency produces. A view is the right thing to exclude on the merits, not
merely the convenient one: it declares no foreign key and is no address anything occupies, so
nothing can be smuggled through it that is not already in the tables it reads. The module's
own base-table assertion made exactly this scoping decision when the first views landed
`[measured dd3778c:internal/store/migrate_test.go:19-25 · read → "Base tables only (plus goose's own version table) — information_schema.tables with no table_type filter also lists views, and the migration set now creates some, so this assertion is scoped to base tables", over "WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'"]`.
So the maze-touching set stays `chunk`, `node_discovery` and `event` after `chat_knowledge`
lands.

- *Not only a maze-keyed relation.* `chunk` and `node_discovery` both happen to key on
  `(maze_id, q, r)`, but the relations this rule exists to catch are the ones not yet written:
  a PvP record or a corpse with a surrogate `id` primary key and ordinary `maze_id`, `q`, `r`
  columns is exactly the shape a hostile mechanic takes, and a primary-key-shaped definition
  would let it carry a `scope` reference with the guard staying green.
- *Not only a foreign key, because the tree already contains the counterexample.* `event`
  declares `maze_id bigint` with **no** foreign key, deliberately and permanently as far as
  this schema goes, and carries two `owner (id)` references beside it
  `[measured dd3778c:internal/store/migrations/00003_event_log.sql:44-53 · grep -n "maze_id" and grep -rn "ALTER TABLE event" migrations → "maze_id   bigint,                                    -- no FK: an open question, not a missing table", with player_id and chat_id both REFERENCES owner (id), and no later ALTER adding the maze FK]`.
  An FK-only definition would make `event` invisible to both halves — and `event` is the live
  in-tree precedent a later gameplay relation is most likely to copy. The name-based arm is
  what closes that.

**`event` is therefore in scope, and Half B's allow list carries its two rows** — written
here, like the others, because the allow list is the content of the guard. `event.player_id`
is the player an event is about; `event.chat_id` is the chat an event is about. Neither is a
position: an `event` row is a log record whose `maze_id` says where something happened, not an
address anything occupies, and no `scope` reference exists on it for a home to arrive through.
That reasoning is what an allow-list row has to state, and it is why the row is cheap to write
and expensive to write falsely.

*Rejected: a `chat_home` table with a `peaceful boolean` (generated always true).* A column
that is always true only helps a mechanic that asks, and a mechanic that asks was never the
risk; it also invents a schema shape for PvP and looting tables that are out of the MVP and
not designed, which the data-contract carve-out makes expensive to undo.
*Rejected: a source-level guard forbidding a home in a combat signature.* No combat or
looting **function** exists in the tree — only the combat balance keys the configuration
package decodes
`[measured dd3778c:internal/config/balance.go:78-82 · grep -rniE "func .*(combat|loot)" over non-test Go → no match; grep -rn "CombatBalance" → "CombatBalance holds the combat dice and multipliers." and its struct]`
— so such a guard would scan an empty corpus of signatures and report clean for every possible
tree, which is the failure mode `AGENTS.md` § Patterns 2 names.

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
reads — have no Go reader either
`[measured dd3778c:non-test Go sources · grep -rln "metric_" --include=*.go . | grep -v "^./tmp" → internal/store/views_test.go alone; the same pattern matches a constructed control line and matches that test file, so the empty non-test result is the tree's and not the pattern's]`,
so a reader invented here would have no caller.

**The projection is a decision, not an implementor's choice**, because a view's column names,
order and types are what `CREATE OR REPLACE VIEW` can never loosen afterwards
(`AGENTS.md` § API Stability carve-out). In order: `chat_id bigint`, `maze_id bigint`,
`q integer`, `r integer` — the chat first because every reader filters on it, then the maze,
then the cell in the same `(q, r)` order the world's own tables use. It is chosen minimal so
a later `first_discovered_at` is an **append**, which that statement allows: computed as
`min(discovered_at)` under a `GROUP BY` of these same columns it leaves the row set exactly
where `SELECT DISTINCT` puts it today.

**`player_id` is deliberately out, and the append argument does not cover it.** The design
corpus's radar renders «кто что где нашел» and reads this view (`~/lab-private/DESIGN.md`
§2.4), so "who" is a real future column — but adding it would **multiply the row set**, one
row per discoverer per cell, which is not an append and which the module's own view convention
forbids outright
`[measured dd3778c:internal/store/migrations/00003_event_log.sql:89-90 · read → "Their column names, order and types are a permanent contract: CREATE OR REPLACE VIEW can only append a column, never drop, rename, reorder or retype one."]`.
The radar is out of scope, and "who" is reachable today by joining `node_discovery` directly,
which is where the per-player grain already lives. A view that answers *what a chat knows* is
one row per cell; a view that answers *who found it* is a different view, and it can be
created beside this one rather than by widening it. This one is `chat_knowledge` because
AC10's "what a chat knows" is a set of cells. The pin is `TestViews_columnContract`, whose per-view
`cases` slice is hard-coded — a view absent from it is silently unpinned
`[measured dd3778c:internal/store/views_test.go:59-74 · read → "TestViews_columnContract pins each view's column names, order and types — the one thing CREATE OR REPLACE VIEW can never loosen later", over a cases slice of {view, cols}]`,
so subtask 1 adds the case alongside the `viewNames` entry.

### D3 — Two packages: `internal/chat` for the facts, `internal/onboard` for the front door

`internal/chat` owns the two new tables and nothing Telegram-shaped: the bot's presence in a
chat, the membership rows, and the read the outbound gate needs. `internal/onboard` owns the
deep link and the two `ingest.Handler` implementations that translate a `telego.Update` into
`chat` and `store` calls. The split is the one KD-46 drew between `internal/world`
(persistence) and the raid edges that call it, and it keeps SQL and update parsing out of each
other's files.

Not `internal/store`: that package is the ledger's write path and the item machine, and the
persisted world was refused a home there for exactly that reason
`[measured dd3778c:ai-docs/key-decisions.md:149 · grep -n "KD-46 —" → "not internal/store, whose package comment scopes it to the ledger, the item machine and the event log. The migration takes the next free number in internal/store/migrations, the only directory Migrate embeds."]`. The one function that
does belong in `store` is `EnsureOwner` — find-or-create keyed on `(kind, telegram_id)`,
delegating to `CreateOwner` on a miss — because it is `owner`'s own concern and both handlers
create an owner: the `my_chat_member` handler creates the **chat**, the Start handler creates
the **player**. Those are its two call sites, with a third (the raid start) visible in #36,
which is the shape the ≥3-site rule says to lift rather than copy. **The Start handler's
*chat* side is not one of them** — it is a lookup, never a create, for the reason D8 gives.

**On a concurrent creator, `EnsureOwner` returns the unique violation wrapped, and the
caller's retry is what turns it into a hit** — it takes no `ON CONFLICT`. The read-then-create
race is lost at `CreateOwner`'s own `INSERT INTO owner`, against the partial unique index on
`(kind, telegram_id)`
`[measured dd3778c:internal/store/migrations/00001_ledger_core.sql:12 · read → "CREATE UNIQUE INDEX owner_kind_telegram_id_key ON owner (kind, telegram_id) WHERE telegram_id IS NOT NULL"]`,
which aborts the transaction; the ingest loop rolls that attempt back and the next attempt's
read finds the row. *Rejected: `ON CONFLICT … DO NOTHING` on that insert.* It would leave
`CreateOwner` continuing into a scope-and-account CTE for an owner that already has both, and
`scope_singleton_key` would then raise the very same class of violation one statement later —
so the conflict clause buys nothing and hides where the race was lost. Today the race is
unreachable, because the loop settles updates one at a time
`[measured dd3778c:internal/ingest/loop.go:207-209 · read → "for _, raw := range updates { if err := l.processUpdate(ctx, raw); err != nil {" — a sequential range, no fan-out]`,
but the function is being lifted for callers that are not the loop, and both of them already
retry a failed attempt in a **fresh** transaction — the ingest loop by beginning one per
attempt under its retry bound, the scheduler worker by beginning its own per claimed task
`[measured dd3778c:internal/ingest/attempt.go:69-71 and internal/scheduler/execute.go:91,103 · read → attemptOnce opens with "tx, err := l.pool.Begin(ctx)" and is called once per attempt by runAttempts; executeOne opens with "tx, err := conn.Begin(ctx)"]`.

Import edges: `chat → store`; `onboard → ingest, chat, store, telego`; `cmd/bot → onboard,
chat`. `internal/ingest` gains no new import (see D4); Go refuses an import cycle at compile
time, so the whole-module build is the gate that judges this edge set — `[derived → go build
./... green on subtask 10]`.

### D4 — The bot's presence in a chat is a row, and the outbound gate reads it

AC13 says that after the bot is removed the bot sends that chat **nothing**. The seam where
that cannot be bypassed is the outbound gate: today a destination chat id is allowed outright
when it is in `ALLOWED_CHAT_IDS`, and otherwise allowed when a player owner row exists for it
`[measured dd3778c:internal/ingest/gate.go:102-128 · read → allowKnown: allowlist hit returns nil, then the cache, then PlayerLookup]`.
An allowlisted chat the bot was removed from would still be written to. So the gate's chat
branch gains a presence requirement: **allowlisted AND the bot is currently present**.

**The branch order is part of the decision**, because the allowlist and the player carve-out
read the same `telegram_id` column and an operator may legitimately allowlist a player's DM
`[measured dd3778c:ai-docs/domain-invariants.md:143 · read → "A destination is allowed when its chat_id token parses as an int64 present in ALLOWED_CHAT_IDS, or when an owner row exists with kind = 'player' and that telegram_id … because a chat's own id lives in that same telegram_id column"]`.
`allowKnown` therefore composes as: **the positive player cache first** (in memory, so a
repeat DM costs nothing and the hot path does not move); then **allowlisted and present**,
which allows; then, on a **fall-through**, the player lookup, which allows and caches. An
allowlisted id that is not a present chat is *not* refused there — it falls through, so an
allowlisted player DM reaches the same allow/refuse outcome it reaches today. **Its failure
mode does move, deliberately:** an allowlisted DM now passes the presence question first, so a
presence lookup that *errors* refuses a destination today's gate allows unconditionally. That
is the fail-closed direction the gate already takes for a failed player lookup, and § Test
Design pins it rather than leaving it to be discovered. An allowlisted chat the bot was
removed from falls through too and is then refused by the player lookup, because a group id
names no player. A present chat that is not allowlisted never reaches the presence question
and is refused: the allowlist stays an outer bound, necessary-but-not-sufficient for a chat
destination and never weaker. AC14 falls out: a re-add flips the row back and the same
destination is allowed again.

The presence answer is **not cached**. The gate's existing cache is positive and lives as long
as the gate
`[measured dd3778c:internal/ingest/gate.go:44-48,58-59 · read → "The cache holds positive results only, for the Gate's lifetime — nothing negative is remembered", over an unexported cache map[int64]struct{} guarded by the gate's own mutex]`,
which is correct for "this telegram id is a player" (a player never stops existing) and wrong
for presence (a removal must take effect at once). Chat traffic is rare by design — the
notification budget is a standing design obligation (`~/lab-private/DESIGN.md` §1) — and the
cache-first order keeps player DMs off the database, so one query per outbound **chat** call
is the right trade.

`ingest.PlayerLookup` becomes `ingest.DestinationLookup`, carrying the player question and the
presence question; `ingest.NewPoolGate` and its unexported pool-backed lookup are **deleted**
and `internal/chat` supplies the implementation the composition root injects through the
already-exported `ingest.NewGate`. What that buys is narrow and worth stating precisely:
`internal/ingest` keeps its `internal/store` import either way — `attempt.go` and `offset.go`
both need it and are untouched here
`[measured dd3778c:internal/ingest/attempt.go:14 and internal/ingest/offset.go:9 · grep -n "internal/store" → both import github.com/maratik123/lab-game/internal/store]`
— so the claim is **not** that the package sheds a domain dependency. It is that `ingest`
gains no import of `internal/chat`, and that the gate's two facts are assembled where every
other subsystem is assembled rather than inside the package that consumes them. No compat
shim is left behind (`AGENTS.md` § API Stability).

**`internal/chat`'s lookup answers the player question by calling `store.PlayerExists`, not
by a query of its own.** That predicate checks the `(kind, telegram_id)` **pair** — a lookup
on the id alone would admit every chat the bot was ever added to — and it has a test that says
so; a second hand-rolled query would be a silent duplicate of a security-relevant predicate,
free to drift from the one the test guards. Reusing it also keeps `store.PlayerExists` with a
production caller after subtask 6 removes its current one.

**The deletion and the rewire are one subtask, because the deleted constructor has a caller
outside its own package.** `cmd/bot`'s start-up builds the gate through it
`[measured dd3778c:cmd/bot/assemble.go:326 · grep -n "NewPoolGate" → "gate := ingest.NewPoolGate(cfg.AllowedChatIDs, pool)"]`,
so splitting the rename from the rewire would leave `go build ./...` red across every
intervening subtask — and that is the first gate `/task` Step 8 runs after each one, with the
coverage ratchet refusing a commit whose suite is not green. `internal/ingest`'s own
`guards_test` allow list names the constructor too, so it goes in the same change.

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
`[measured dd3778c:internal/ingest/router.go:70-80 · read → "A Handler MUST NOT issue an outbound Bot API call whose permission rests on a row its own uncommitted transaction created"]`,
and the presence row the gate would read is created by that very transaction. The designed
route is the outbound queue, which is another issue's.

### D6 — Every event this task writes is conditioned on a state change, so a redelivery writes none

The ingest loop settles an update by advancing the offset inside the handler's own
transaction; a crash between handling and commit means the same update is delivered again, and
the loop imposes no per-update idempotency of its own
`[measured dd3778c:internal/ingest/attempt.go:66-110 · read → attemptOnce begins a tx, calls Handle, advances the offset and commits; the only dedupe branch is store.ErrAlreadyPosted, which a handler raises by posting]`.
`store.AppendEvent` is not idempotent by construction — two equal `Event` values write two
rows
`[measured dd3778c:internal/store/event.go:34-37 · read → "Event carries no occurrence-time field … and no idempotency key: unlike PlayerOperation, two Post or AppendEvent calls built from equal Event values write two distinct event rows"]`.
The handlers therefore emit **only** where committed state actually changes:

- `bot_added_to_chat` / `bot_kicked` — emitted iff the bot's presence in the chat changes.
  The prior value is the stored row when one exists, and otherwise the update's own
  `old_chat_member` membership, so the very first update the bot ever sees for a chat is
  judged against what Telegram says the prior state was rather than against an absence. A
  promotion (`member` → `administrator`) is not a change and emits nothing; a redelivery finds
  the stored row already agreeing and emits nothing.

  **Only the event is conditional. The chat owner, its home and the presence row are written
  on every handled update**, change or no change, so the gate has its row from the first
  `my_chat_member` the bot ever sees for that chat whatever kind of transition it reports.
  That separation is what keeps the funnel's gap (below) from also being an outbound-traffic
  gap.

  *The path this rule loses, named rather than left to be found:* if the **first** update the
  bot ever sees for a chat is a promotion, the prior value read from `old_chat_member` is
  already "present", so no `bot_added_to_chat` is ever emitted for it. The presence row is
  written and the gate works, but the funnel's chat stage keys on that event and `event` is
  append-only, so the chat is missing from the headline number permanently. It is reachable
  only in the pre-existing-chat window — a chat the bot joins after this ships always reports
  the join as its own status change first — and it carries the same remedy, which § Risks
  states once for both.
- `player_started` — emitted iff **this transaction creates the player owner row or inserts a
  membership row**. Either is an arrival: the first is the player arriving in the game, the
  second is the player arriving *from a chat*. A Start that changes neither — a repeat Start
  through a link for a chat the player already belongs to (AC7), a second bare Start — emits
  nothing, and so does a redelivery, because both of those find the rows already there.

  **This rule is a correction, and the funnel is why.** Attribution in
  `metric_activation_funnel` is not by a player's earliest `player_started`; it is by their
  earliest **chat-bearing** one — the predicate filters `chat_id IS NOT NULL` before
  `DISTINCT ON` picks a row, and the view's own comment says "the chat of their earliest such
  event", which is a view written in the expectation that a player may have several
  `[measured dd3778c:internal/store/migrations/00003_event_log.sql:96-110 · read → "each player with a player_started naming a chat belongs to the chat of their earliest such event, ties broken by the lower chat id" over "SELECT DISTINCT ON (e.player_id) … WHERE e.type = 'player_started' AND e.player_id IS NOT NULL AND e.chat_id IS NOT NULL ORDER BY e.player_id, e.ts, e.chat_id"]`.
  Emitting only on player creation would therefore drop a real onboarding path out of the
  MVP's headline number: a player who takes AC9's route (a bare Start, chat dimension null)
  and **later** arrives through chat X's link would record the membership, emit nothing, and
  be attributable to no chat forever. Emitting on a new membership repairs exactly that, at
  the cost of `player_started` firing more than once per player — which the two shipped views
  already absorb, because the funnel takes the earliest chat-bearing row and
  `metric_retention_daily` takes each player's `min` day
  `[measured dd3778c:internal/store/migrations/00003_event_log.sql:168-173 · read → "new_players AS (SELECT e.player_id, min((e.ts AT TIME ZONE 'UTC')::date) AS day FROM event e WHERE e.type = 'player_started' AND e.player_id IS NOT NULL GROUP BY e.player_id)"]`.
  AC6's second chat therefore gets its own event, and the funnel still attributes the player
  to the first chat, because that row is the earlier one.

Telego's `ChatMember` interface answers the presence question itself — `MemberIsMember()` is
true for owner, administrator and member, is the restricted member's own `IsMember`, and is
false for left and banned
`[measured telego@v1.11.2 · grep -n -A 3 "func (c \*ChatMember.*) MemberIsMember() bool" types.go → true, true, true, c.IsMember, false, false]`.
The handler uses it rather than hand-rolling a status table (`AGENTS.md` § Dependency
Versions), and keeps `MemberStatus()` for the event payload. A `nil` chat member on a
malformed update is an error return, never a panic — the panic index is empty and this task
keeps it so
`[measured dd3778c:ai-docs/panic-index.md · tail -3 → the table body is the placeholder row "| — | — | — |", with no entry above it]`.

### D7 — What each event carries

Each is already registered in the `event_type_definition` catalog, so no dictionary
migration is owed
`[measured dd3778c:internal/store/migrations/00003_event_log.sql:21,22,36 · grep -n → (1, 'bot_added_to_chat', 'low_volume'), (2, 'player_started', 'low_volume'), (16, 'bot_kicked', 'low_volume')]`.

- `bot_added_to_chat` — dimension `chat_id` (the chat owner's id); no player dimension,
  because the actor of the add need not be a player in the game and the column is an `owner`
  foreign key. Payload names the chat's telegram id, its type, its title, the actor's telegram
  id, the update's own date, and the old and new statuses.
- `player_started` — dimensions `player_id` and, when the link resolved to a chat that exists
  in the game, `chat_id`. Payload names the player's telegram id; the chat telegram id the
  link carried, when a payload was present, **whether or not it resolved**, so a link naming a
  chat the bot was never added to is visible rather than silent; and which of the two arrivals
  D6 emits on — the player's creation, the membership, or both in one transaction — so a
  reader of the log can tell a first arrival from a later one without joining back to `owner`.
- `bot_kicked` — dimension `chat_id` (the chat owner's id), **set, not null**; no player
  dimension, for the same reason as the add. It is the terminal product metric, every
  occurrence of which gets a post-mortem (`~/lab-private/DESIGN.md` §13.3), and a row written
  with a null `chat_id` would be permanently unjoinable to the chat it is about — `event` is
  append-only, so there is no later `UPDATE` to repair it, and AC13 keeps the chat owner row
  alive precisely so the join has a target. Payload then answers AC16 on its own and depends
  on nothing the removal ended: the chat's telegram id, type and **title** (no `getChat` is
  possible afterwards), the actor's telegram id, the update's own date, the old and new
  statuses, and the update id.

**Which side of the column/payload line each field falls on is not this design's choice.**
The four §13.4 dimensions are columns and everything else is JSONB, uniformly across types,
and the reason that bites here is referential integrity
`[measured dd3778c:ai-docs/domain-invariants.md:84,88 · read → "The §13.4 dimensions — player, chat, maze, depth — are columns on event; everything else about an event is payload JSONB. The rule is uniform across types" and "player_id and chat_id carry foreign keys to owner. JSONB cannot express referential integrity"]`.
So every event above carries its owner references as **columns**. The chat's **telegram** id
is not one of the four dimensions, so it stays in the payload beside the column, deliberately
and in all three events: the column is the join key, the payload value is what a post-mortem
reads when it has only the log and needs the id Telegram itself uses.

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
`my_chat_member` is a no-op, since a settlement is a group chat. Registering the two routes is
also what makes either kind arrive at all: `my_chat_member` **is** in the Bot API's default
set — the types excluded by default are `chat_member`, `message_reaction` and
`message_reaction_count`
`[measured core.telegram.org/bots/api#getupdates, fetched 2026-09-19 · WebFetch → "Specify an empty list to receive all update types except chat_member, message_reaction, and message_reaction_count (default)"]`
— but this loop never leaves `allowed_updates` unset: it transmits its own route set on every
call, carrying a reserved sentinel while that set is empty, so today the bot is asking for an
id space it can never receive
`[measured dd3778c:internal/ingest/loop.go:136-146 · read → allowedUpdates is built from Router.Kinds(), with a reserved sentinel only while the route set is empty]`.

**The Start handler LOOKS the link's chat UP and never creates one.** `EnsureOwner` is the
**player** side of that handler and nothing else: a chat exists in the game because the bot
was added to it (AC1), so the chat side is a read keyed on `(kind = 'chat', telegram_id)`,
and a payload naming a chat with no owner row records no membership. Using find-or-create
there would let an unauthenticated, guessable payload **conjure** a chat and its home scope —
widening the § Risks payload row from "join a chat you were not in" to "invent one" — so the
primitive is named here rather than left to whichever branch reads naturally at
implementation time.

**A link naming a chat the bot was removed from still records the membership.** The chat, its
home and everything it accumulated survive a removal by AC13, so that lookup finds the owner
exactly as it would for a live chat and the membership is written. That is the decision,
not an inference: membership is the recorded link between a player and a **chat**, and the
bot's presence is a fact about the bot, not about the chat's existence — the presence row
governs what the bot may *send*, and nothing else. The player's own arrival is real whether or
not the bot can answer in that chat, and a re-add (AC14) then finds the membership already
there.

Both handlers write through inserts that select the owner row by **kind**, the shape
`world.RecordDiscovery` already uses, so a membership whose chat is not a chat or whose player
is not a player is unwritable rather than merely unwritten
`[measured dd3778c:internal/world/discovery.go:14-22 · read → "INSERT INTO node_discovery … SELECT $1, $2, $3, $4 FROM owner WHERE id = $4 AND kind = 'player' ON CONFLICT … DO NOTHING"]`,
and the ambiguous zero-rows case is disambiguated by reading the owner's kind exactly as that
function does.

### D9 — The schema change, forward-only

One forward migration adds the `home` scope definition and the two tables below, both keyed on
`owner.id` rather than on a telegram id — every other satellite of `owner` in this schema is,
and a telegram id is an external identifier the game's own rows should not be joined by.

- **`chat_presence`** — `chat_id bigint PRIMARY KEY REFERENCES owner (id)`,
  `present boolean NOT NULL`, `changed_at timestamptz NOT NULL DEFAULT now()`. One row per
  chat, carrying the **current** answer only; the history is the event log's, which is what
  `bot_added_to_chat` and `bot_kicked` are for. `present` is a **boolean, not a stored Bot API
  status**: a status vocabulary is Telegram's to change and would become live data this
  project owns under the API-stability carve-out, while the predicate the game actually needs
  is binary. The raw `old`/`new` statuses go in the event payload (D7), where they are
  evidence rather than a contract. The primary key covers the foreign key. **`changed_at` is
  written explicitly on every flip** — `SET present = $2, changed_at = now()` — because the
  column default fires on `INSERT` only, and a column that silently meant "first seen" while
  being named "changed" is the kind of drift no gate catches. It is kept rather than dropped
  because it answers "since when" without scanning the event log, and adding it later would
  have no honest value to backfill for rows already there.
- **`chat_membership`** — `chat_id bigint NOT NULL REFERENCES owner (id)`,
  `player_id bigint NOT NULL REFERENCES owner (id)`,
  `joined_at timestamptz NOT NULL DEFAULT now()`, `PRIMARY KEY (chat_id, player_id)`, plus
  `CREATE INDEX chat_membership_player_idx ON chat_membership (player_id)`. Accruing only:
  this task records no way for a membership to lapse, and §16.7 names none.

**The gate's presence question takes a telegram id**, because that is all an outbound call
carries — `tg.ChatRef.Key` is the raw `chat_id` token from the request body. So
`DestinationLookup.BotPresentInChat(ctx, telegramID)` resolves the owner and the presence row
in one statement, selecting on `owner.kind = 'chat'` and `owner.telegram_id`, which is the
pair the partial unique index on `(kind, telegram_id)` serves. The handlers, which already
hold the owner id, write through the `owner.id` columns directly.

**The migration carries the scope backfill its own precedent makes standing**, and states why
it carries only one of the three statements. A `scope_definition` seeded by a migration
creates scopes for owners created *after* it, because `CreateOwner` runs at owner-creation
time and not as a migration-time sweep, so the item-machine migration wrote a backfill and
bound every later one to the same obligation
`[measured dd3778c:internal/store/migrations/00007_item_machine.sql:48-56 · read → "Backfill: a scope_definition seeded here creates scopes only for owners created after it … Every later scope-definition migration owes the same three statements."]`.
Here the scope statement is written —
`INSERT INTO scope (owner_id, scope_definition_id) SELECT o.id, <home id> FROM owner o WHERE
o.kind = 'chat' ON CONFLICT (owner_id, scope_definition_id) DO NOTHING` — and the account and
balance statements are **not**, because `home` seeds no `account_definition` row (D1), so both
would select from an empty join forever. That reason goes in the migration's own comment
beside the statement, so the omission is auditable rather than invisible. The scope statement
is a no-op on every database this migration meets — nothing creates a chat owner before it —
and it is written anyway, because "this case is unreachable" is exactly the argument the
convention exists to survive.

The migration rewrites nothing and renames nothing, so rows written before it are unaffected
and there is no deploy window in which two shapes are read.

**Rollback** is the forward-only posture KD-46 already states
`[measured dd3778c:ai-docs/key-decisions.md:155 · grep -n "The migration is forward-only[^.]*\." → "The migration is forward-only, so rolling the binary back leaves the tables in place and unread, and rolling forward again finds them."]`:
rolling the binary back leaves the tables and the seeded row in place and unread — the older
binary neither creates chats nor reads presence — and rolling forward again finds them. The one reader an older binary shares
is `store.CreateOwner`, which walks `scope_definition` generically and would create the home
scope for any chat it created; it creates none.

`chat_membership.player_id` gets an index of its own: it trails the primary key, so the
module's foreign-key coverage test would otherwise fail it
`[measured dd3778c:internal/store/fkcover_test.go:26-33 · read → uncoveredFKs returns every FK whose referencing columns are not covered by the leading columns of some index on the same relation]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Forward migration: the `home` scope definition with its scope backfill, `chat_presence`, `chat_membership` and its player index, and the `chat_knowledge` view; then every hard-coded schema manifest it moves — the Go catalog mirror, the exact-view-set list, the view column-contract case, the base-table set and the applied-migration count — **and the two ledger-suite assertions the seeded row reverses**: the chat-has-no-scope test, renamed and re-pointed per D1, and the now-false comment in the move suite | `internal/store/migrations/00010_chat_home_presence_membership.sql`, `internal/store/catalog.go`, `internal/store/views_test.go`, `internal/store/migrate_test.go`, `internal/store/owner_test.go`, `internal/store/move_test.go` | — |
| 2 | Behaviour tests for `chat_knowledge`: the union, the non-member exclusion, and the join-later case | `internal/store/chat_knowledge_test.go` | 1 |
| 3 | The peaceful-home instrument: the `scope`-vs-maze-cell disjointness assertion, the reviewed allow list for `owner` references on maze-touching relations (D1 states its rows), and the constructed positive control for each half | `internal/store/peaceful_home_test.go` | 1 |
| 4 | `store.EnsureOwner`: find-or-create keyed on `(kind, telegram_id)`, delegating to `CreateOwner` on a miss, returning `store.Owner` **unchanged in shape** and propagating a concurrent creator's unique violation per D3 | `internal/store/owner.go`, `internal/store/owner_test.go` | 1 |
| 5 | `internal/chat`: package doc, errors, presence write and read, membership write, and the `DestinationLookup` implementation over a pool. Its `main_test.go` is `leaktest.Main(m, testdb.Main)`, so this subtask **also raises `testdb.Binaries`** — the manifest test demands the constant move in the same commit as the binary | `internal/chat/doc.go`, `internal/chat/errors.go`, `internal/chat/presence.go`, `internal/chat/membership.go`, `internal/chat/lookup.go`, `internal/chat/main_test.go`, `internal/chat/presence_test.go`, `internal/chat/membership_test.go`, `internal/testdb/server.go` | 1, 4 |
| 6 | Outbound gate, **rename and rewire in one subtask**: `PlayerLookup` → `DestinationLookup`, the branch order of D4, deletion of `NewPoolGate` and its pool-backed lookup **together with the composition root's call of it and its `guards_test` allow-list row**, the gate tests, and the package's own prose that still calls the gate the allowlist alone — the `ErrChatRefused` sentinel's "neither allowlisted nor a known player", the package comment's "the Gate implementation the chat allowlist rides on", and `Options.Client`'s "is this task's chat allowlist" | `internal/ingest/gate.go`, `internal/ingest/errors.go`, `internal/ingest/doc.go`, `internal/ingest/loop.go`, `internal/ingest/gate_test.go`, `internal/ingest/guards_test.go`, `cmd/bot/assemble.go` | 5 |
| 7 | `internal/onboard`: the namespaced `start` payload codec and the deep-link URL builder, with their refusals. Pure functions, no database — its `main_test.go` is `leaktest.Main(m, (*testing.M).Run)` and this subtask leaves `testdb.Binaries` alone | `internal/onboard/doc.go`, `internal/onboard/errors.go`, `internal/onboard/link.go`, `internal/onboard/link_test.go`, `internal/onboard/main_test.go` | — |
| 8 | The `my_chat_member` handler: chat kind filter, owner ensure, presence change, and the two events with their payloads. This is what makes `internal/onboard` database-backed, so it switches that `main_test.go` to `leaktest.Main(m, testdb.Main)` and **raises `testdb.Binaries` in the same commit** | `internal/onboard/presence_handler.go`, `internal/onboard/presence_handler_test.go`, `internal/onboard/main_test.go`, `internal/testdb/server.go` | 5, 7 |
| 9 | The `/start` handler: private-chat filter, command parse, player ensure, membership, and `player_started` on either arrival per D6 | `internal/onboard/start_handler.go`, `internal/onboard/start_handler_test.go` | 5, 7, 8 |
| 10 | Composition root: register both routes; the outbound end-to-end test carrying the link, and the activation-funnel test over the two handlers' own events | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go`, `internal/onboard/outbound_test.go`, `internal/onboard/funnel_test.go` | 6, 8, 9 |
| 11 | Documentation and the propagation sweep: the new key decisions, the architecture and status entries, the start-up step whose router is no longer empty, the peaceful-home / membership / gate-presence invariants — the last of which also carries the pre-existing-chat bootstrap § Risks names — and a `grep -rni` sweep for every live surface restating the allowlist's old sufficiency | `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/process-lifecycle.md`, `ai-docs/domain-invariants.md`, `AGENTS.md` | 10 |

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
  `[measured dd3778c:internal/store/owner.go:80-120 · read → the WITH s AS (INSERT INTO scope … RETURNING) INSERT INTO account … pattern, then "if len(createdAccounts) == 0 { return owner, nil }"]`,
  which is exactly the path a chat now takes. If the CTE did not execute to completion the
  home would silently not exist. Mitigation: AC1's test asserts the home scope row directly,
  so the premise is executed rather than assumed — `[derived → the AC1 test in subtask 8]`.
- **An allowlisted destination that names a chat stops being allowed on the allowlist alone.**
  Any future sender aimed at a chat the bot was never added to, or was removed from, is refused
  where it previously succeeded. A player's DM is deliberately *not* affected — D4's
  fall-through keeps it — but that is a property of the branch order, which is the kind of
  thing a later edit loses silently. Mitigation: no production sender exists today; subtask 6
  pins the fall-through with a scenario of its own rather than leaving it implied in the code;
  and the prose that states the old sufficiency is corrected in the same subtask for
  `internal/ingest`'s own files and in subtask 11 for the document set —
  `[derived → the allowlisted-player-DM scenario in subtask 6 and the propagation sweep in subtask 11]`.
- **A chat the bot was already in when this code ships never gets a presence row.** Telegram
  delivers `my_chat_member` when the bot's status *changes*
  `[measured core.telegram.org/bots/api#update, fetched 2026-09-19 · WebFetch → "Optional. The bot's chat member status was updated in a chat."]`,
  so for a chat the bot joined before the handler existed no update ever arrives, the row is
  never written, and the gate's chat branch falls through to the player lookup, which refuses
  a group id. **Its neighbour costs the headline metric instead of the traffic:** if the first
  update that chat ever produces is a *promotion* rather than a join, the presence row is
  written and the gate works, but the prior value D6 reads from `old_chat_member` is already
  "present", so no `bot_added_to_chat` is emitted — and the activation funnel's chat stage
  keys on that event, on an append-only table, so the chat is absent from the MVP's headline
  number for good. There is no migration route to repair either: the set of chats the bot is
  in is not a table, and `ALLOWED_CHAT_IDS` is configuration, not a record of membership.
  **One remedy covers both, operational and stated here rather than discovered by the first
  sender task: the bot is removed from and re-added to each pre-existing chat once, which
  produces the `my_chat_member` the handler needs** — and, correctly, also produces the
  `bot_added_to_chat` that puts the chat into the funnel. Blast radius today is nil because no
  production sender exists; the cost of not writing it down is that the first one meets it
  against the MVP's single friendly chat. Subtask 11 carries the sentence into the gate
  invariant — `[derived → the gate-presence invariant in subtask 11]`.
- **The deep-link payload is guessable.** It carries the chat's telegram id in the clear, so
  someone who learns a chat id can become a member of that chat without ever having been in
  it. The link is a shareable capability either way, so an opaque token would stop guessing
  and not sharing. Mitigation and escape hatch: a per-chat random token is a forward migration
  adding one column plus the two readers the codec already isolates — `[derived → the payload
  codec's single encode/decode pair in subtask 7]`.
- **A group upgraded to a supergroup changes its chat id**, so the bot appears in a second
  chat with a second home and the first chat's memberships do not follow. Out of scope here;
  recorded in § Open questions rather than absorbed.
- **Two new database-backed test binaries move the connection-ceiling refusal threshold down,
  and the contention gate is the route that reaches it first.** `testdb.Binaries` is a term of
  `ceiling = clients × Binaries × parallel × (schemaMaxConns + 1) + ceilingSlack`, refused
  above `ceilingMax`
  `[measured dd3778c:internal/testdb/server.go:252-280 and internal/testdb/testdb.go:74 · read → the formula as quoted, ceilingMax 2000, ceilingSlack 32, schemaMaxConns 4]`.
  The threshold is therefore a **bound on the product `clients × parallel`**, and this task
  moves it from above 49 to above 39. The parallelism term defaults to the host's
  `runtime.GOMAXPROCS(0)`, and the contention gate passes two clients
  `[measured dd3778c:cmd/testpg/run.go:88 and Makefile:37,158 · grep -n → fs.Int("parallel", runtime.GOMAXPROCS(0), …); CONTENTION_PARALLEL ?= $(shell nproc 2>/dev/null || echo 4); testpg --clients 2 --parallel $(CONTENTION_PARALLEL)]`,
  so `make test` newly refuses on a host of forty cores or more and `make test-contention` on
  one of twenty or more. Mitigation: the refusal is loud, names
  every term and names the flags that lower it, and both `--clients` and `--parallel` are
  caller-supplied; and the constant is raised **by each subtask that adds a database-backed
  binary, in that subtask's own commit** (5 for `internal/chat`, 8 for `internal/onboard`),
  because `TestBinaries_matchesTree` fails by name the moment the tree and the constant
  disagree — `[derived → the raised constant and its manifest test in subtasks 5 and 8]`.
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

**Subtask 1 — migration.** Location: the module's existing store suite. The tests here already
exist; what this subtask owes is the **hard-coded manifest inside each of them**, because a
migration that adds a relation or a view moves a literal in four places and every one of them
is a `slices.Equal` against a compiled-in list
`[measured dd3778c:internal/store/migrate_test.go:41-54,142-146 and internal/store/views_test.go:17-26,71-74 · read → the base-table want list compared with slices.Equal; a count over goose_db_version asserted against a literal; the viewNames list; the per-view cases slice of TestViews_columnContract]`.
Entry points: the catalog mirror comparison, the base-table set, the applied-migration count,
the exact-view-set list, the view column contract, and the foreign-key coverage test. Scenarios:
the catalog mirror fails on either side seeding a scope definition the other does not know; the
base-table set names the two new tables; the applied-migration count matches the migration
directory; the view set matches exactly; `chat_knowledge`'s columns, their order and their
types match D2's projection; every new foreign key is covered by a leading-column index.
Fixtures: the migrated schema pool the suite already builds. `[derived → AC1]`

**Subtask 2 — `chat_knowledge`.** Location: `internal/store/chat_knowledge_test.go`. Entry
point: the view, queried directly, since it has no Go reader. Scenarios: (a) two members of one
chat each discover a distinct cell and the chat knows both, and a cell discovered by both
appears once — AC10; (b) a player who is a member of a second chat discovers a cell, and the
first chat does not know it — AC11; (c) discoveries recorded *before* the membership are in the
chat's knowledge as soon as the membership row exists — AC12's first clause; **(c′) one player
who is a member of chats A and B discovers one cell, and that cell is in A's knowledge and in
B's — AC12's second clause**, the knowledge counterpart of AC6's multi-chat player, which no
other scenario reaches (b has the player in some *other* chat, not in both); (d) a chat with
no members knows nothing. Fixtures: owners of both kinds, a maze row and `node_discovery` rows written as plain
SQL — the suite cannot import `internal/world`, which imports `internal/store`.
`[derived → AC10, AC11, AC12]`

**Subtask 3 — the peaceful home.** Location: `internal/store/peaceful_home_test.go`. The guard
has **two halves**, because the shipped schema answers them differently (D1), and a single
blanket rule would be red on the tree it is meant to certify.

A *maze-touching relation* is any **ordinary table** (`table_type = 'BASE TABLE'`, i.e.
`relkind = 'r'`) that **either** declares a column referencing `maze (id)` **or** declares a
column named `maze_id`, **whatever its primary key** — read from the catalogs, never from a
hard-coded table list, so a table added later is in scope without an edit. The base-table
filter is not optional: without it the scan finds `chat_knowledge`, which subtask 1 creates
and whose projection carries `maze_id`, and the asserted set below would be red on this
design's own output (D1 gives the reason a view is the right thing to exclude). Both arms are load-bearing, for the reasons D1 gives: a hostile mechanic's
table is as likely to carry a surrogate `id` primary key as to key on the cell, and `event`
is the shipped precedent for a `maze_id` with no foreign key at all. The shipped set the scan
must find is therefore `chunk`, `node_discovery` **and `event`**, and the test asserts that
set before judging anything — an arm that silently matched nothing would make the whole guard
a claim about its own extractor.

- **Half A — the home's own address space.** Rule: no maze-touching relation carries a
  reference to `scope (id)`, and `scope` itself declares no maze or cell column. **It
  has no exemptions**, which is what makes it the half the AC rests on. Scenarios: (a) the
  shipped schema passes — the three relations referencing `scope` are `account` and the item
  machine's two holder columns, none of them maze-touching, and the maze-touching set the scan
  found is asserted to be `chunk`, `node_discovery` and `event`; (b) **the control** — a relation
  created inside the test carrying a `scope (id)` reference beside a `maze (id)` reference and
  the cell pair makes the guard fail, and the failure message names that relation and column.
- **Half B — nobody smuggles a home in through `owner`.** Rule: every (relation, column) pair
  that is both an `owner (id)` reference and sits on a maze-touching relation must appear in a
  **reviewed allow list**, each row naming which owner kind that column holds and what keeps a
  home out of it. The list is D1's and is exactly four rows: `chunk.gate_chat_id` — a chat's,
  but its **gate**, which AC3 leaves this task placing none of; `node_discovery.player_id` — a
  player's, kept so by the discovery writer's own kind filter rather than by a constraint, and
  the row says so; and `event.player_id` and `event.chat_id` — the player and the chat an
  event is *about*, on a log record whose `maze_id` says where something happened rather than
  where anything is. A pair with no row is a failure naming it. This is the shape
  `internal/gateguard` already uses for bare `go` statements — an allow-list row per site,
  answering fixed questions
  `[measured dd3778c:internal/gateguard/guard_test.go:60,283,287 · grep -n → "launchTable is the reviewed allow list for every bare go statement", failures reported as "makes %d launch(es) with no allow-list row" and "allow-list row answers %d"]`.
  Scenarios: (c) the shipped schema passes with exactly those rows and no others — an allow-list
  row matching no pair is **also** a failure, so the list cannot rot into a blanket exemption;
  (d) **the control** — a relation created inside the test carrying an `owner (id)` reference
  beside a `maze (id)` reference and the cell pair fails, proving the list distinguishes a
  reviewed pair from a new one.
- **Two further controls, one per arm of the definition — this is what the width buys.**
  (e) A relation with a **surrogate `id` primary key** and ordinary `maze_id`, `q`, `r`
  columns, the `maze_id` carrying a real foreign key, plus a `scope (id)` reference: **both
  halves must go red on it**. Under a primary-key-shaped definition it would escape both.
  (f) The same relation with the **foreign key removed** — a bare `maze_id bigint`, the shape
  `event` already ships — again with a `scope (id)` reference: **both halves must go red on
  it too**. Under an FK-only definition it would escape both, which is the hole the name-based
  arm exists to close. Each control is a separate scenario rather than one parametrised case,
  so a definition that lost one arm fails by name.

Without (b), (d), (e) and (f) a clean result would be a claim about the guard's vocabulary
rather than about the schema. Fixtures: the migrated schema pool; each control relation is created and
dropped inside the test's own schema, and the test reads back the count of pairs the scan
found before judging, so an extractor that matched nothing cannot report clean. `[derived → AC2]`

**Subtask 4 — `store.EnsureOwner`.** Location: `internal/store/owner_test.go`. Entry point:
`EnsureOwner`. **`store.Owner` does not change shape**: it is `{ID, Kind, TelegramID,
Accounts}` and carries no scope field
`[measured dd3778c:internal/store/owner.go:49-56 · read → "Owner is the result of CreateOwner: the created owner row plus every account created for its scopes." over a struct of ID, Kind, TelegramID, Accounts]`,
and since the home scope carries no `account_definition` row (D1) a chat's `Accounts` is empty
before and after this migration. The home is therefore asserted **by reading the `scope` row**
for the created owner, never off the returned value — the same read § Risks names as AC1's
proof. Scenarios: creates on a miss and reports creation; returns the existing owner without
creating on a hit and reports no creation; a created chat owner has exactly one `scope` row
and it is the `home` definition; a created player owner's scope set is unchanged by this
migration; a unique violation from a concurrent creator is returned rather than swallowed, and
a second call after it succeeds (D3); the refusals `CreateOwner` already makes (the World
kind, an unknown kind, a nil telegram id) are unchanged and happen before any statement.
Fixtures: the store suite's own pool. `[derived → AC1, AC5, AC7]`

**Subtask 5 — `internal/chat`.** Location: `internal/chat/*_test.go`, a new database-backed
binary standing on `internal/storetest`'s migrated pool, whose `TestMain` is
`os.Exit(leaktest.Main(m, testdb.Main))` — the module's shape for a package that provisions a
database — and which therefore raises `testdb.Binaries` in this same commit, because the
manifest test fails by name in both directions the moment the tree and the constant disagree
`[measured dd3778c:internal/testdb/server.go:219-226 and internal/testdb/server_test.go:531-553 · read → "A new database-backed package must update this constant" and "a package added or removed from the set of testdb.Main callers must change the constant in the same commit, and this test fails by name in both directions when it does not"]`.
Entry points: the presence write, the presence read, and the membership write.
Scenarios: presence written for a chat and read back; a repeat write of the same value reports
no change **and leaves `changed_at` where it was**; a flip reports a change **and moves
`changed_at` forward**, read back from the column so the write rule D9 states is pinned rather
than assumed from the default; a presence write against an owner that is not a chat is
refused, writing nothing; a membership recorded once is idempotent on a repeat; a membership
whose chat is not a chat, or whose player is not a player, is refused, writing nothing; the
lookup answers both of the gate's questions over a pool, and its presence question — which
takes a **telegram** id, not an owner id — answers false both for a chat the bot left and for
a telegram id no chat owner exists for, without confusing the two with an error.
`[derived → AC7, AC13, AC14]`

**Subtask 6 — the gate.** Location: `internal/ingest/gate_test.go`. Entry point:
`(*Gate).AllowCall`. Scenarios: an allowlisted chat the bot is present in is allowed; the same
chat after a removal is refused, and the refusal names the chat; the same chat after a re-add
is allowed again, proving nothing positive was cached across the removal; a present chat that
is not allowlisted is refused **without the presence question being asked**, which is what
pins the branch order from the outside; **an allowlisted destination that is a player's DM —
allowlisted, not a present chat — is still allowed**, through the fall-through D4 states, so
the carve-out this change could silently have closed is held open; a player destination is
allowed and then served from the positive cache without a second lookup; a presence lookup
that errors refuses rather than allows, so the gate still fails closed; `ChatNone` is allowed
and `ChatUnknown` refused, both unchanged. Fixtures: a stub `DestinationLookup` carrying a
call counter per question, which is what makes both "not cached" and the branch order
observable rather than asserted. `[derived → AC13, AC14]`

**Subtask 7 — the deep link.** Location: `internal/onboard/link_test.go`, whose `TestMain` is
`os.Exit(leaktest.Main(m, (*testing.M).Run))` at this point — the codec touches no database,
so `internal/onboard` is not yet a `testdb.Main` caller and `testdb.Binaries` does not move
here; subtask 8 is where that file's runner and the constant both change. Entry points: the
payload encoder, the payload parser and the link builder. Scenarios: a negative supergroup id
round-trips through encode and parse; the encoded payload contains only characters the Bot API
admits and stays inside its length budget — asserted against the documented character class,
not against a remembered one; the parser refuses an empty payload, a payload with the wrong
prefix, a payload whose remainder is not an integer, and a payload carrying trailing text; the
built link's host, path and query are asserted through a parse of the result rather than by
string equality, so a different but equivalent spelling is not a false failure; an empty
username is refused. `[derived → AC4]`

**Subtask 8 — the `my_chat_member` handler.** Location:
`internal/onboard/presence_handler_test.go`. This is the subtask that makes `internal/onboard`
database-backed: its `main_test.go` switches to `os.Exit(leaktest.Main(m, testdb.Main))` and
`testdb.Binaries` moves in the same commit, for the manifest-test reason subtask 5 already
states. Entry point: `Handle` on the loop's own
transaction. Scenarios: (a) an add for a group chat creates the chat owner, its home scope and
a present row, and appends `bot_added_to_chat` with the chat dimension set — AC1; (b) a second
chat added yields a second chat with a home of its own, and neither is the other — AC1; (c) no
chunk and no gate row exists for either chat afterwards — AC3; (d) a removal flips presence,
appends `bot_kicked`, and leaves the chat, its home, every membership and every discovery in
place — AC13; (e) a re-add flips presence back against the **same** chat owner id, with what
the chat accumulated intact — AC14; (f) the removal event's payload answers which chat, when,
by whom and by what status transition, read back from the row alone — AC16 — **and its
`chat_id` column is non-null and equals the chat owner's id**, joined back to `owner` in the
assertion rather than read out of the payload, which is the half a payload-only reading would
have let through (D7); (g) a promotion
from member to administrator leaves presence true and appends nothing; **(g′) a promotion as
the FIRST update the chat ever produces still writes the chat owner, its home and a present
row — so the gate works — and still appends no `bot_added_to_chat`**, which is the funnel gap
§ Risks names: the scenario exists to pin the consequence as chosen rather than to let a green
(g) read as an endorsement of it; (h) the same update handled
twice appends nothing the second time — D6; (i) a `my_chat_member` for a private chat and one
for a channel are no-ops; (j) a malformed update with no new chat member returns an error and
does not panic. Fixtures: `telego.Update` values built in the test, the migrated pool, and a
helper that reads back the events of one type for a chat. `[derived → AC1, AC3, AC13, AC14,
AC15, AC16]`

**Subtask 9 — the `/start` handler.** Location: `internal/onboard/start_handler_test.go`.
Entry point: `Handle`. Scenarios: (a) a private `/start` with a chat payload creates the
player, records the membership and appends `player_started` with both dimensions — AC5; (b) an
existing player starting through a second chat's link is a member of both, the first
membership is untouched, and the second chat's own `player_started` is appended — AC6 and D6;
(c) a second Start through the same chat's link leaves one player and one membership, and
appends no second event, because neither row is new — AC7; (d) a group message from a
player who never pressed Start creates nothing at all — AC8; (e) a private `/start` with no
payload creates the player, appends `player_started` with no chat dimension, and records no
membership — AC9; **(e′) the AC9 player then starts through chat X's link: no player is
created, the membership is, and a `player_started` carrying chat X is appended** — the case
D6's emission rule exists for, and the one an emit-on-player-creation rule would have lost;
(f) a payload naming a chat the bot was never added to creates the player, records no
membership, appends the no-chat-dimension event, and leaves the chat telegram id visible in
its payload; **(f′) a payload naming a chat the bot was *removed* from records the membership
and appends the event with that chat's dimension, exactly as a live chat would** — the
decision D8 states, so it is pinned rather than inferred from whichever branch the implementor
writes first; (g) a malformed payload is treated as no payload rather than as an error that
dead-letters the update; (h) the same update handled twice appends nothing the second time —
D6; (i) a `/start` in a group chat is a no-op. Fixtures: as subtask 8. `[derived → AC5, AC6,
AC7, AC8, AC9, AC15]`

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
recorded events alone — AC17; **(d) the attribution case D6's rule turns on: a bare Start,
then a Start through that chat's link, and the funnel attributes the player to that chat** —
which is false under an emit-on-player-creation rule and is therefore the scenario that pins
the decision rather than restating it. Fixtures: the fake Bot API server, the migrated pool,
and the assemble options the bot suite already uses. `[derived → AC4, AC15, AC17]`

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
