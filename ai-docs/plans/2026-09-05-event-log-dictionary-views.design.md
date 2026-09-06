# Design: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views

**Issue:** #21
**Date:** 2026-09-06

## Approach

The task ships the **substrate** of product analytics: one forward migration carrying the
append-only `event` table, its type registry as database data, the `journal_entry` arc
extension that makes an event a basis document, and the MVP views; plus the Go write API
and the doc-page edits. No mechanic emits anything here.

### Not a mechanic — the telemetry and posting-signature rules do not fire

`docs/DESIGN.md` §13.4's obligation («любая новая механика … обязана объявить свои
события») binds **mechanics**. This task adds none: it moves no stamina, resources, money
or items, declares no event of its own, and emits nothing. It is the ledger-core spec's
KD-11 situation exactly — «Post is not a mechanic»
`[measured 8617a7c:ai-docs/plans/done/2026-09-02-ledger-post-core.spec.md:435 · sed -n '435p' … → "KD-11 | Does this task declare events or posting signatures? | **No** — Post is not a mechanic"]`.
What this task *does* fix is the **shape** of an event-backed posting group: the basis
document is one `event` row, the arc column is `journal_entry.event_id`, and the group is
zero-sum per kind under it like every other basis. The *signature* of any particular
group — which accounts and kinds a `shop_sale` moves — is defined by the mechanic that
emits `shop_sale`, in that mechanic's PR, and checked by #26's framework
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:119-120 · sed -n '119,120p' … → "Posting-signature contract tests — issue #26 owns the framework; this task only makes an event usable as a basis document."]`.

Likewise **no tuning value ships**. `depth`, the D1/D7 offsets and the day grain are the
metric's own definition (§13.3 names D1/D7 retention) and query structure, not game
balance — the spec settles this in its own Key decisions row
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:175 · sed -n '175p' … → "Balance numbers | None. This task ships no tuning value; depth, counts and window lengths inside a view are query structure, not game balance (§16.5)."]`.

### The schema, as a forward migration

One new file under `internal/store/migrations/`, next in sequence, picked up by the
existing embed `[measured 8617a7c:internal/store/migrate.go:16-17 · sed -n '16,17p' internal/store/migrate.go → "//go:embed migrations/*.sql" / "var migrationsFS embed.FS"]`.
Forward-only, no `-- +goose Down` — KD-3's standing policy
`[measured 8617a7c:ai-docs/key-decisions.md:13 · grep -n 'KD-3 ' ai-docs/key-decisions.md → "**Forward-only:** no `-- +goose Down` section anywhere — rolling a ledger table back is data loss"]`.

**`event_volume_class`** — a database enum with the members `low_volume` and
`high_volume`, mirrored in Go. The spec fixes the partition and leaves the names to this
design; it asks for names that describe the axis rather than today's members
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:311-313 · sed -n '311,313p' … → "**Volume-class member names.** The partition is fixed (Scope 2); the names are not … choose names that describe the retention axis rather than the current two members."]`.
The axis the spec itself names is **volume** («volume class»), so the members name volume,
not the readers (`product` / `health`) and not a retention policy this task does not ship.

**`event_type_definition`** — the registry, shaped exactly like the existing seeded
catalogs: a `smallint` primary key, a `text` `code` with a UNIQUE constraint, and
`volume_class`. The naming follows `scope_definition` / `account_definition`
`[measured 8617a7c:internal/store/migrations/00001_ledger_core.sql:15-27 · sed -n '15,27p' … → "CREATE TABLE scope_definition (id smallint PRIMARY KEY, code text NOT NULL UNIQUE, owner_kind owner_kind NOT NULL); CREATE TABLE account_definition (…)"]`,
which also keeps the name clear of the `event` table's own `type` column and its index.

**`event_type_definition.id` is referenced by nothing, deliberately.** `event.type`
references `code`, not `id`; the `id` exists solely to carry §13.4's declaration order so
AC5's mirror comparison can be *ordered* rather than set-wise. That must be written into
the migration's comment, or the next reader either adds a second foreign key to it or
deletes it as dead weight — and deleting it turns AC5's ordered comparison into an
unordered one without anything failing.

**`event`** — identity primary key, `type text NOT NULL` with a **named** foreign key to
`event_type_definition (code)`, nullable `player_id` and `chat_id` referencing `owner (id)`,
nullable `maze_id` with no foreign key (#29 adds it), nullable `depth integer`,
`payload jsonb NOT NULL DEFAULT '{}'::jsonb`, and `ts timestamptz NOT NULL DEFAULT now()`.

The identity flavour is `GENERATED ALWAYS AS IDENTITY`, which is what this schema uses for
every table that is not seeded with explicit ids — the `BY DEFAULT` tables are exactly the
ones the first migration seeds with explicit ids and `setval`s
`[measured 8617a7c:internal/store/migrations/ · grep -n 'GENERATED' internal/store/migrations/*.sql → BY DEFAULT on owner, scope and account; ALWAYS on player_operation, manual_correction, journal_entry, posting, scheduled_task, deferred_task and recurrent_task]`.
`event` is seeded with nothing, so it takes `ALWAYS`.

Indexes, and why each exists. The FK-coverage gate makes every one but `event_ts_idx`
mandatory rather than optional
`[measured 8617a7c:internal/store/fkcover_ac17_test.go:14-16 · sed -n '14,16p' … → "if got := uncoveredFKs(t, ctx, pool); len(got) != 0 { t.Fatalf(\"uncovered FKs on the migrated schema: %v, want none\", got) }"]`:

| Index | On | Why |
|---|---|---|
| `event_type_ts_idx` | `event (type, ts)` | Covers the `type` FK (leading column) **and** every per-type, time-ranged view |
| `event_player_idx` | `event (player_id) WHERE player_id IS NOT NULL` | Covers the player FK; the funnel and retention views group by player |
| `event_chat_idx` | `event (chat_id) WHERE chat_id IS NOT NULL` | Covers the chat FK; the funnel and notification views group by chat |
| `event_ts_idx` | `event (ts)` | The all-type day grain the retention view scans; mirrors `journal_entry_ts_idx` |

The FK-coverage helper accepts a partial index whose predicate contains `IS NOT NULL`, and
compares the FK's column set against the index's **leading** columns
`[measured 8617a7c:internal/store/fkcover_test.go:84-96 · sed -n '84,96p' … → "if idx.pred != nil && !strings.Contains(strings.ToUpper(*idx.pred), \"IS NOT NULL\") { continue } … leading := sortedCopy(idx.cols[:len(want)])"]`,
which is what lets one composite `(type, ts)` index serve both roles.

**The arc extension**, copying `00002_scheduler.sql`'s established form in one
`ALTER TABLE`: add `event_id bigint REFERENCES event (id)`, drop and re-add
`journal_entry_exactly_one_basis` naming the pre-existing arc columns plus `event_id`, then
the partial unique index `WHERE event_id IS NOT NULL`
`[measured 8617a7c:internal/store/migrations/00002_scheduler.sql:38-46 · sed -n '38,46p' … → "ALTER TABLE journal_entry ADD COLUMN deferred_task_id … DROP CONSTRAINT journal_entry_exactly_one_basis, ADD CONSTRAINT journal_entry_exactly_one_basis CHECK (num_nonnulls(player_operation_id, manual_correction_id, deferred_task_id, recurrent_task_id) = 1); CREATE UNIQUE INDEX journal_entry_deferred_task_key …"]`.

### Two consequences a mechanic author must be told, not left to discover

The spec asks for the first of these explicitly — «a constraint the design must state, not
discover»
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:185-188 · sed -n '185,188p' … → "**The exclusive arc is 1:1.** Each arc column carries a partial unique index, so one event backs **at most one** `journal_entry`. A mechanic that must move balances twice under one event needs two events or a different basis — a constraint the design must state, not discover."]`.
Both belong in `ai-docs/domain-invariants.md` §5 (subtask 6), not only here, because the
audience is a mechanic author who will never read this file.

1. **One event backs at most one posting group.** The partial unique index makes the arc
   1:1, so a mechanic that must move balances twice under one conceptual occurrence needs
   two events, or a different basis type. A second `Post` under an already-referenced event
   is refused by the database with `23505`, not silently merged.
2. **An event carries no idempotency key, and that is a difference from `PlayerOperation`.**
   `PlayerOperation.insert` deduplicates on `(source, operation_id)` and turns a replay into
   `ErrAlreadyPosted`
   `[measured 8617a7c:internal/store/basis.go:48-56 · sed -n '48,56p' internal/store/basis.go → "INSERT INTO player_operation (source, operation_id) VALUES ($1, $2) ON CONFLICT (source, operation_id) DO NOTHING RETURNING id … if errors.Is(err, pgx.ErrNoRows) { return 0, ErrAlreadyPosted }"]`.
   `Event` has no such key: two `Post` calls built from equal `Event` values write two event
   rows and two journal entries. Retry safety for an event-backed mechanic therefore rides
   on the rail it already has — `player_operation` for a player-initiated action, the
   scheduler's `(state, seq)` guard for a timer edge (§3.5) — never on the event. Adding a
   key later is a forward migration; assuming one exists is a duplication bug.

### Forward-migration paragraph (schema change, rollback stated)

Nothing is renamed or redefined. `event` and `event_type_definition` are new, so no row
predates them. The arc column is added **nullable**, so every `journal_entry` row written
before this migration keeps exactly one non-null basis and satisfies the re-added CHECK —
Postgres validates the new constraint against existing rows at `ADD CONSTRAINT` time, and
the old CHECK is what guarantees it passes. During a deploy window the **old** binary runs
correctly against the **new** schema (it never names `event_id`, and its inserts still
satisfy the CHECK); the **new** binary against the **old** schema does not, which is why
`store.Migrate` runs at start-up before anything else.

**Rollback:** there is no down migration by policy (KD-3). Reverting the *code* is safe on
its own — the new tables and column are inert to a binary that does not name them.
Reverting the *schema* would be a new forward migration dropping `event_id`, and it must
not be written once any `journal_entry` references an event, because that drops the basis
of a live posting group. The correct correction is always a further forward migration.

**A shipped view's column set is a contract too, and `CREATE OR REPLACE VIEW` is narrower
than "replace" suggests.** The Key-decisions rationale for putting views in migrations
leans on a later family arriving by `CREATE OR REPLACE VIEW` — but that statement may only
**append** columns. Dropping, renaming, reordering or retyping one is refused with SQLSTATE
`42P16`
`[measured 8617a7c:internal/store/migrations/ · psql on docker.io/library/postgres:18, CREATE VIEW v AS SELECT a, b FROM e then four CREATE OR REPLACE VIEW variants → appending c succeeded; dropping c → "42P16: cannot drop columns from view"; renaming b → "42P16: cannot change name of view column \"b\" to \"bb\""; reordering → "42P16: cannot change name of view column \"a\" to \"b\""; retyping b to text → "42P16: cannot change data type of view column \"b\" from integer to text"]`.
So the column names, order and types this task ships are as durable as the table's: a later
change that is not an append is `DROP VIEW` + `CREATE VIEW` in a forward migration, and any
Grafana panel bound to the old shape breaks with it. Name the columns as if they were
columns of a table, because for maintenance purposes they are.

### The registry as a table, not a Postgres enum

The spec leaves the door open to a `ledger_kind`-style enum if the volume class finds a
home `[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:170 · sed -n '170p' … → "Registry storage — seeded catalog table or Postgres enum? **Default: a seeded catalog table** mirrored in Go … `design-writer` may argue the `ledger_kind`-style enum instead only if it also finds a home for the class"]`.
**Rejected**, for a reason the spec does not list: an enum member is added with
`ALTER TYPE … ADD VALUE`, and this repository's own hygiene gate forbids an `ADD VALUE`
file from doing anything else
`[measured 8617a7c:internal/store/migrate_test.go:190-194 · sed -n '190,194p' … → "if addValueRe.MatchString(text) { if createTableRe.MatchString(text) || insertRe.MatchString(text) || updateRe.MatchString(text) { t.Errorf(\"%s: an ADD VALUE file must do nothing else …\") } }"]`
— so every future mechanic that registers a type would need its label in a migration file
that may do nothing else, and its class row in another, with a window between them where a
label exists with no class. The catalog table registers a type and its class in one
`INSERT`.

### The Go surface, placed by the package's existing by-kind layout

`internal/store` groups by kind of thing, not by domain
`[measured 8617a7c:internal/store/ · for f in enums catalog basis errors ids; do grep -hoE '^(type|func|var|const) [A-Za-z(*]+' internal/store/$f.go; done → enums.go: OwnerKind, ownerKinds, Kind, kinds, OperationSource, operationSources; catalog.go: ScopeDefinition, AccountDefinition, scopeDefinitions, accountDefinitions; basis.go: PostingBasis, PlayerOperation, ManualCorrection, DeferredTask, RecurrentTask and their methods; errors.go: the sentinel var block and ErrInvalidOwner; ids.go: OwnerID, AccountID, WorldOwner, WorldMoney, WorldExperience]`.
The event surface follows that:

| Where | What |
|---|---|
| `enums.go` | `EventVolumeClass` + its members + the mirror slice |
| `catalog.go` | `EventType` + the §13.4 members + `EventTypeDefinition` + the mirror slice |
| `ids.go` | `EventID` |
| `errors.go` | `ErrUnknownEventType`, plus the block-comment edit below |
| `event.go` (new) | `Event` (the basis) with its sealed methods, and `AppendEvent` |
| `basis.go` | `PostingBasis`'s doc comment, extended to name `*Event` |

**Placement, and why not a new package** (`design-writer.md` § Rules → ≥3-site
duplication asks for this trade-off to be recorded). Nothing is *duplicated* here: the
registry and the type constants are declared once and imported. Every future emitter
already imports `internal/store` to reach `store.Post`, so a dedicated `internal/event`
package would add an import edge and split the mirror from the migration that seeds it —
and from the mirror test, which needs a migrated database and therefore lives in
`internal/store`. The call-site count is open-ended (every mechanic that emits an event),
which is exactly why it must have **one** home; `internal/store` is that home.

**Two entry points, because the traffic has two shapes.**

- `Post(ctx, tx, &Event{…}, postings…)` — the event's `PostingBasis` implementation.
  `Post` already runs §11's transaction order (basis document → balances → postings)
  `[measured 8617a7c:internal/store/post.go:144-155 · sed -n '144,155p' internal/store/post.go → "// Phase d. docID, err := basis.insert(ctx, tx) … tx.QueryRow(ctx, entrySQL, docID).Scan(&entryID)"]`,
  so the event row, the `journal_entry` and the postings land in the caller's one
  transaction with no new machinery.
- `AppendEvent(ctx, tx, Event{…}) (EventID, error)` — the no-posting path, which reuses the
  same unexported `insert` and creates no `journal_entry`.

The asymmetry is deliberate: `Post` takes a pointer because the sealed interface's methods
have pointer receivers, and `AppendEvent` takes the struct **by value** so the no-posting
path has no nil case to define at all. *Rejected:* a `*Event` parameter, which would force
`AppendEvent(…, nil)` to return `ErrNoBasis` — a sentinel whose name is a lie on a path
that has no basis to be nil. Note the consequence for testing, which § Test Design acts on:
because neither `Post` nor `AppendEvent` can reach `(*Event)(nil).insert`, that nil guard is
**unreachable from either entry point** and only the package-internal test asserts it.

**Nullable fields are pointers, uniformly**, each named after its column so the struct and
the schema read the same: `PlayerID *OwnerID`, `ChatID *OwnerID`, `MazeID *int64`,
`Depth *int32` — the `…ID` suffix following `DeferredTask.TaskID`'s precedent.
*Rejected:* the zero-as-NULL shape `DeferredTask` uses
`[measured 8617a7c:internal/store/basis.go:124-127 · sed -n '124,127p' internal/store/basis.go → "INSERT INTO deferred_task (task_id, task_type, instance_key, run_at) VALUES (NULLIF($1, 0), $2, NULLIF($3, ''), $4)"]`
— it works for ids that are never zero, but **depth 0 is a real depth** (the entrance), so
that rule would hold for the id fields and break on `Depth`. A per-field rule the caller
must remember is worse than one `&`.

**Payload** is `json.RawMessage`, which pgx v5.10.0's `JSONCodec` handles as a distinct
case rather than re-marshalling
`[measured 8617a7c:go.mod:7 · grep -n 'jackc/pgx/v5 ' go.mod → "github.com/jackc/pgx/v5 v5.10.0"; and sed -n '31,33p' "$(go env GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/pgtype/json.go" → "// Handle json.RawMessage specifically because if it is run through json.Marshal it may be mutated." / "case json.RawMessage:"]`.
A nil payload becomes `{}` in SQL via `COALESCE($n, '{}'::jsonb)`, so the Go side needs no
branch and the NOT NULL column is never fought.

**No occurrence-time parameter.** `ts` defaults to `now()`, which inside a transaction is
`transaction_timestamp()` — so an event written as a posting basis carries the *same*
instant as its `journal_entry.ts`, whose default is also `now()`
`[measured 8617a7c:internal/store/migrations/00001_ledger_core.sql:69 · sed -n '69p' … → "ts                   timestamptz NOT NULL DEFAULT now(),"]`.
The spec's open question notes that a caller-supplied occurrence time is a later migration
if it ever matters; it is not needed to test the views, because the view fixtures are SQL.

**`ErrUnknownEventType`, and the doc-comment edit it forces.** The database is the
authority: an unregistered type violates the named foreign key and Postgres raises SQLSTATE
`23503`. `insert` maps *that constraint name* to the sentinel, exactly as `Post` maps
`account_balance_nonnegative` to `ErrOverdraft`
`[measured 8617a7c:internal/store/post.go:167-175 · sed -n '167,175p' internal/store/post.go → "if errors.As(err, &pgErr) && pgErr.Code == sqlstateCheckViolation && pgErr.ConstraintName == constraintBalanceNonnegative { return fmt.Errorf(\"%w: account %d: %w\", ErrOverdraft, id, err) }"]`.
*Rejected:* a Go-side pre-check against the mirror. It would leave the transaction usable,
which is nicer — but it would also mean the database path is never exercised through the Go
API, and AC8 asks for exactly the database's refusal to be surfaced. Because a constraint
violation aborts the transaction, the sentinel's doc comment states that the caller must
roll back, in the register the aborting sentinels already use
`[measured 8617a7c:internal/store/errors.go:36-48 · sed -n '36,48p' internal/store/errors.go → "ErrOverdraft … The transaction is aborted; the caller must roll back." / "ErrBalanceRowMissing … the caller must still roll back."]`.
`Post` wraps phase-d errors with `%w`, so `errors.Is` reaches the sentinel through the wrap
`[measured 8617a7c:internal/store/post.go:147-150 · sed -n '147,150p' internal/store/post.go → "if errors.Is(err, ErrAlreadyPosted) { return err } return fmt.Errorf(\"post: insert basis document: %w\", err)"]`.

The sentinel joins the existing block rather than sitting outside it, because it *is*
raised through `Post` (in phase d, via `basis.insert`) and a reader who meets it there will
look for it in that list. That makes the block's own opening comment stale in both halves
`[measured 8617a7c:internal/store/errors.go:5-6 · sed -n '5,6p' internal/store/errors.go → "// Post's eight sentinels. Compare with errors.Is; see Post's doc comment for" / "// which phase raises each and what state the transaction is left in."]`,
so subtask 3 carries more than the new declaration:

- the block comment **drops the count** rather than bumping it — a tally in a doc comment
  is a rot surface, and this is the second time it would have to move;
- the block comment gains "…and one which `AppendEvent` also raises", so "see Post's doc
  comment for which phase raises each" stops being a false universal;
- `Post`'s own phase-d line names `ErrUnknownEventType` beside `ErrAlreadyPosted`, or the
  phase list becomes the incomplete comment DOC-5 forbids.

### The views

They live in the same goose migration as `CREATE VIEW`, per the spec's Key decisions row —
a later family arrives as a later migration in its mechanic's PR, not as an edit here
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:173 · sed -n '173p' … → "Where the views live | In the goose migration, as `CREATE VIEW`."]`.

These rules bind **every** view, and each is correctness, not style:

1. **The day grain is UTC, spelled `(ts AT TIME ZONE 'UTC')::date`.** A bare `ts::date`
   resolves against the *session* `TimeZone`, and this suite sets only `search_path`
   `[measured 8617a7c:internal/testdb/testdb.go:155 · sed -n '155p' internal/testdb/testdb.go → "cfg.ConnConfig.RuntimeParams[\"search_path\"] = name"]`,
   so a container default and a `LAB_GAME_TEST_DSN` server could disagree and the same
   query would return different numbers on different machines.
2. **Every denominator is wrapped in `NULLIF(x, 0)` and every aggregate that can be empty
   in `COALESCE(…, 0)`.** Division by zero is an error in Postgres, not a NULL, and
   `sum(…) FILTER (…)` over an empty filter yields NULL, not `0`.
3. **A column whose type is a custom enum is cast to `text` in the view.** Only
   `metric_faucet_sink` has one (`account_definition.kind`). The cast is not there because
   a failure was observed — pgx already decodes `ledger_kind` as text elsewhere in this
   package
   `[measured 8617a7c:internal/store/post.go:113-120 · sed -n '113,120p' internal/store/post.go → "var kindText string … rows.Scan(&id, &kindText, &info.controlled) … info.kind = Kind(kindText)"]`
   — but AC14's test adds an enum member at run time on a pooled connection, and `::text`
   removes any dependence on when a given connection resolved that type's OID. A view is
   also read by Grafana, which is not pgx.

Naming: a `metric_` prefix. It groups the views together in the Grafana Postgres
datasource's picker, and it keeps them clear of §11's singular-table-name decision, which
governs entity tables — a view named for a question is not an entity.

Each view records its own name and the §13.3 question it answers in a SQL comment directly
above it (AC11). Those comments are **English**, and they cite the section, never a line —
the Russian surface is `docs/**` and owner conversation, nothing else
`[measured 8617a7c:AGENTS.md:4 · grep -n 'English for every durable artefact' AGENTS.md → "**English for every durable artefact** — code, comments, … **Russian for two surfaces only:** conversation with the product owner, and `docs/**`"]`,
and a line reference into a document that gets edited rots
`[measured 8617a7c:ai-docs/doc-convention.md:49 · grep -n 'Cite the section number' ai-docs/doc-convention.md → "Cite the section number, never a line number: the design document is edited, and `§2.2.4` survives what `:118` does not."]`.

| View | The §13.3 question it answers | Grain |
|---|---|---|
| `metric_activation_funnel` | the activation funnel — §13.3's headline MVP number | one row per chat that has a `bot_added_to_chat` |
| `metric_retention_daily` | returns: D1/D7 retention, and raids per player per day | one row per UTC day that has any event |
| `metric_death_by_depth` | deaths by depth | one row per `depth` value among `death` events |
| `metric_faucet_sink` | faucet/sink balance per resource | one row per (UTC day, `ledger_kind`) |
| `metric_notification_per_chat_day` | notifications per chat per day | one row per (UTC day, chat) |

**`metric_activation_funnel`** — columns: the chat, the earliest `bot_added_to_chat`
instant, the count of distinct players with a `player_started` in that chat, the count of
distinct players with any `raid_started` in that chat, and the count of those players whose
raiding spans a **next** UTC day — a player has a `raid_started` on the day immediately
after the day of their own first `raid_started` in that chat. Rows with a null chat are
excluded from every stage; a stage with no rows reports `0`, not NULL.

**`metric_retention_daily`** — columns: the day; distinct players with any event that day;
`raid_started` count that day; raids per active player (guarded denominator); the day's new
players (those whose earliest `player_started` is that day); how many of them raided on
day + 1 and on day + 7; and the rates over the new-player denominator (guarded, so a day
with no new players yields NULL rather than an error). D1 and D7 are read as **exactly**
day + 1 and day + 7, which is what §13.3's «D1/D7» names; a "within N days" reading is a
different metric and would need the owner's word.

Its SQL comment must also carry the **cohort-immaturity** warning, because the numbers lie
without it: a cohort born fewer than seven days ago cannot yet have a D7, and the view
reports `0`, not NULL, for it. A reader scanning the recent end of the series sees a
retention cliff that is an artefact of the window, not of the game. The same holds for D1
on today's cohort. One clause in the comment naming this is the cheapest possible fix, and
AC11 requires that comment to exist anyway.

**`metric_death_by_depth`** — the `depth` value, the death count, and the distinct-player
count. A `death` whose `depth` is null forms its own group rather than being dropped: the
row is a data anomaly worth seeing, and silently discarding it would misreport the total.

**`metric_faucet_sink`** — reached through `posting → journal_entry` (for the day),
`posting → account → account_definition` (for the kind, cast to `text` per rule 3) and
`account → scope → owner` restricted to `owner.kind = 'world'`. Faucet is the negated sum of
the World legs that are negative (value leaving World for the economy); sink is the sum of
the positive ones; net is the negated total. Both are `COALESCE`d to `0`. **AC14 falls out
of the construction**: the view names no `ledger_kind` member anywhere — it groups by
`account_definition.kind` — so a new member appears as soon as postings of that kind exist.

**`metric_notification_per_chat_day`** — the day, the chat, and the count of
`notification_sent`. Rows with a null chat are **excluded**: §13.3's metric is the per-chat
spam budget («штук в чат в день»), and a chat-less notification is not a per-chat number.
The exclusion is asserted by a fixture row, not left implicit.

**`bot_kicked` gets no view, and that is the answer, not an omission.** §13.3 calls it the
terminal metric with «Каждое событие — вскрытие» — *every event is an autopsy*, i.e. the
reader wants the individual rows. An aggregate over `bot_kicked` destroys exactly what the
metric is for; the query is `SELECT * FROM event WHERE type = 'bot_kicked'`. The spec left
this to this design
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:307-310 · sed -n '307,310p' … → "**`bot_kicked` as a view.** … `design-writer` may fold it into the notifications view or leave it out."]`.
No JSONB GIN index either, for the reason the spec gives: no shipped view filters on a
payload key.

### What the existing suite catches, and what it does not

The distinction below is the completeness net for the implementor, so it is drawn by
measurement rather than by intuition. **Only Group 1 below goes red on the new schema.** Everything else in the table after it passes green *without* being touched — the
edits there are coverage the ACs require, not failures the gate reports. Reading "suite
green" as "table discharged" would leave AC3, AC5, AC10 and AC11 unwritten, which is the
instrument-is-a-claim shape this project's own append-only positive control exists to
prevent (`AGENTS.md` § Patterns 2).

**Group 1 — fails red without the edit.**

| Assertion | Why it goes red |
|---|---|
| `TestMigrate_shape_and_seeds`'s table list | An exact-set comparison over a query with no `table_type` filter, so the first `CREATE VIEW` lands in `tables` and set equality fails — `[measured 8617a7c:internal/store/migrate_test.go:20-45 · sed -n '20p;36,45p' internal/store/migrate_test.go → "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()" … "want := []string{\"account\", … \"scope_definition\"}" … "if !slices.Equal(tables, want)"]`; and views do appear there — `[measured 8617a7c:internal/store/migrate_test.go:20 · psql on docker.io/library/postgres:18, SELECT table_name, table_type FROM information_schema.tables after a CREATE VIEW → the view is listed with table_type = VIEW]` |
| `TestMigrate_noop_reapply`'s goose row count | A literal `count != 3`; a further migration moves it — `[measured 8617a7c:internal/store/migrate_test.go:128-134 · sed -n '128,134p' internal/store/migrate_test.go → "SELECT count(*) FROM goose_db_version … if count != 3 { t.Fatalf(\"goose_db_version rows = %d, want 3\", count) }"]` |

**Group 2 — passes green without the edit. The AC forces the coverage, the gate does not.**

| Assertion | Why it stays green | Forced by |
|---|---|---|
| `TestMigrate_shape_and_seeds`'s enum map | A hard-coded `map[string][]string` literal that is ranged over, so an enum it does not name is never queried — `[measured 8617a7c:internal/store/migrate_test.go:48-53 · sed -n '48,53p' internal/store/migrate_test.go → "for enum, wantMembers := range map[string][]string{ \"owner_kind\": …, \"ledger_kind\": …, \"operation_source\": …, \"scheduled_task_state\": … }"]` | AC1 |
| `TestMigrate_indexes_constraints_and_column_types`'s index list | A membership loop (`if !found[want]`) over a literal `want` list — an index absent from the list cannot fail it — `[measured 8617a7c:internal/store/migrate_test.go:221-233 · sed -n '221,224p;229,233p' internal/store/migrate_test.go → "for _, want := range []string{ \"owner_kind_telegram_id_key\", … }" … "if !found[want] { t.Errorf(\"index %s is missing (have %v)\", want, found) }"]` | AC1 |
| The `journal_entry_exactly_one_basis` assertion | Matched on the substring `num_nonnulls`, which the pre-change form already contains, so it passes on a CHECK that never gained the new arc column — `[measured 8617a7c:internal/store/migrate_test.go:239 · sed -n '239p' internal/store/migrate_test.go → "\"journal_entry_exactly_one_basis\":     \"num_nonnulls\","]` | AC3 |
| `TestEnums_mirror_database` | An explicit `{enum, want}` slice; a new enum is simply not among the cases — `[measured 8617a7c:internal/store/enums_test.go:16-23 · sed -n '16,23p' internal/store/enums_test.go → "{\"owner_kind\", stringsOf(ownerKinds)}, {\"ledger_kind\", stringsOf(kinds)}, {\"operation_source\", stringsOf(operationSources)}"]` | AC5 |
| `TestCatalog_mirrors_database` | Two hand-written queries against the two existing catalogs; a third catalog is never read — `[measured 8617a7c:internal/store/enums_test.go:52,73-74 · sed -n '52p;73,74p' internal/store/enums_test.go → "SELECT id, code, owner_kind FROM scope_definition ORDER BY id" / "SELECT id, scope_definition_id, code, kind, controlled FROM account_definition ORDER BY id"]` | AC5 |
| `TestMigrate_hygiene` | Its regex set has no rule pattern, so a `CREATE RULE` in a migration is invisible to it — `[measured 8617a7c:internal/store/migrate_test.go:137-148 · sed -n '137,148p' internal/store/migrate_test.go → downRe, renameValueRe, dropValueRe, createTrigRe, grantRe, upRe, addValueRe, createTableRe, insertRe, updateRe — no CREATE RULE pattern]` | AC10 |
| `TestAppendOnly_…` | Its pattern names only the two ledger tables, so it cannot fire on `event` — `[measured 8617a7c:internal/store/append_only_test.go:13 · sed -n '13p' internal/store/append_only_test.go → "regexp.MustCompile(`(?i)update\\s+(posting\\|journal_entry)\\b\\|delete\\s+from\\s+(posting\\|journal_entry)\\b`)"]` | AC10 |
| `TestBasis_nil_returns_ErrNoBasis_without_panicking` | Hand-enumerates the four existing implementations, asserting `ErrNoBasis` on **both** `entrySQL()` and `insert()` for each; a fifth implementation with a missing nil guard is neither reached nor asserted — `[measured 8617a7c:internal/store/basis_test.go:12-67 · sed -n '12,67p' internal/store/basis_test.go → "var nilPO *PlayerOperation … nilPO.entrySQL() … nilPO.insert(ctx, tx)" and the same pair for ManualCorrection, DeferredTask and RecurrentTask]` | the spec's "every implementation nil-receiver-safe … A fifth implementation follows that contract exactly" |
| The view-set assertion | Does not exist at all today; nothing enumerates views | AC11, AC13 |
| `TestFKCoverage` | Needs no edit — it is generic over the schema and it is the gate the index set above is built to satisfy — `[measured 8617a7c:internal/store/fkcover_test.go:35-40 · sed -n '35,40p' internal/store/fkcover_test.go → "SELECT conname, conrelid::bigint, conkey FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace WHERE c.contype = 'f' AND n.nspname = current_schema()"]` | — |

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Migration: `event_volume_class`, `event_type_definition` with its §13.4 seeds and the `id`-is-for-ordering comment, `event` with its indexes, the `journal_entry` arc extension. Update the table-list and goose-count assertions (Group 1) and add the enum-map, index-list, strengthened `num_nonnulls` and `CREATE RULE` coverage (Group 2), plus the arc-rejection subtests (AC1–AC4, AC10's hygiene half) | `internal/store/migrations/00003_event_log.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 2 | Go mirrors: `EventVolumeClass`, `EventType` + the §13.4 members, `EventTypeDefinition` + the registry slice; extend both mirror tests (AC5) | `internal/store/enums.go`, `internal/store/catalog.go`, `internal/store/enums_test.go` | 1 |
| 3 | Write API: `EventID`, `Event` as a `PostingBasis` implementation, `AppendEvent`, `ErrUnknownEventType` with the three `errors.go`/`post.go` doc edits § Approach names, `PostingBasis`'s doc comment; tests for both paths, the nil-guard pair, the sentinel and transaction ownership (AC6–AC9) | `internal/store/ids.go`, `internal/store/event.go`, `internal/store/errors.go`, `internal/store/post.go`, `internal/store/basis.go`, `internal/store/basis_test.go`, `internal/store/event_test.go` | 2 |
| 4 | Append-only sweep: extend the pattern to `event`, add its planted controls and the `event_type_definition` decoy, and extend the non-vacuity guard to the new migration (AC10) | `internal/store/append_only_test.go` | 1 |
| 5 | The views: append them to the migration with their English `§13.3` comments (including the cohort-immaturity clause), add the exact-view-set assertion, and the fixture-driven per-view expectations with their boundary cases plus the `ledger_kind` genericity test (AC11–AC14) | `internal/store/migrations/00003_event_log.sql`, `internal/store/migrate_test.go`, `internal/store/views_test.go` | 1, 2 |
| 6 | `domain-invariants.md` §5: the payload rule with its reasoning; the two mechanic-facing arc consequences (1:1, and no idempotency key); the §13.5 dashboard limb recorded as suspended with its lift condition; the deferred view families with their owning issues; the `events` → `event` spelling (AC15, AC16, and §5's share of AC18) | `ai-docs/domain-invariants.md` | — |
| 7 | `docs/DESIGN.md`, in Russian: the log table's name at §13.1 and §13.5; «игровое событие (лог 13.1)» added to §11's «Стартовый реестр типов оснований» (AC17) | `docs/DESIGN.md` | — |
| 8 | Propagation sweep by `AGENTS.md` § *Propagation Rule* step 4's criterion over `.claude/`, `AGENTS.md`, `ai-docs/`, `docs/**` and repo-root user-facing docs; the known members are `context.md`'s observability row and the architecture/status prose the landed code falsifies. History surfaces and `_inbox.jsonl` untouched (AC18) | `ai-docs/context.md`, plus whatever the criterion returns | 6, 7 |

`ai-docs/plans/INDEX.md`'s row for this plan is **not** a subtask: that file states it is
maintained by `/task` Step 12 and `/interview`
`[measured 8617a7c:ai-docs/plans/INDEX.md:3 · sed -n '3p' ai-docs/plans/INDEX.md → "Maintained by `/task` (Step 12 moves a completed pair into `done/` and updates the row here) and by `/interview` when a spec is written standalone."]`.

## Handoff plan

Grouping per `design-writer.md` § Rules → handoff-grouping. The two change-types are
disjoint and neither direction of the dependency graph crosses them, so the minimum is two
groups: all code first, all instructions/harness second. Code first because subtask 8's
sweep must run against the tree the code produced — `ai-docs/context.md`'s architecture and
status prose describes what `internal/store` contains, and that claim is only settled once
Group A has landed.

- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–5 (code change-type: `*.go`, migrations). All
  same-change-type subtasks clustered into ONE group rather than interleaved; within the
  `≤ 10` size cap. Entered through `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry).
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task`
  resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the
  orchestrator (typically xHigh), 1M-token window — subtasks 6–8
  (instructions/harness change-type: `*.md`, `ai-docs/**`, `docs/**`). Terminal group
  (3 subtasks; within the `1..=10` range).

Two design-defined groups, inside the default maximum of 4 — no user approval needed.

## Risks

- **"Suite green" is not "table discharged".** Most of § *What the existing suite catches*
  Group 2 passes without any edit, so an implementor who works to the gate rather than to
  the ACs ships a green branch missing AC3's strengthened constraint assertion, AC5's mirror
  extensions, AC10's hygiene and sweep extensions, and AC11's view-set assertion. Mitigation:
  the two-group split above, with the forcing AC named on every Group 2 row, and Step 9's
  per-AC sweep as the backstop — `[derived → the per-AC verification of AC3, AC5, AC10, AC11]`.
- **The table-list assertion silently becomes a view-list assertion.**
  `information_schema.tables` reports views with `table_type = 'VIEW'`, and the existing
  exact-set assertion applies no type filter, so the first `CREATE VIEW` breaks it in a way
  that reads like a spurious failure. Mitigation: subtask 1 filters the assertion to base
  tables and subtask 5 adds the views' own exact-set assertion, which is also what
  establishes AC13 — an extra view fails set equality, so no separate "no deferred view"
  negative has to be written —
  `[measured 8617a7c:internal/store/migrate_test.go:20-22 · sed -n '20,22p' internal/store/migrate_test.go → "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()" — no table_type filter]`.
- **The goose version-row assertion is a hard-coded count.** A further migration moves it.
  Mitigation: subtask 1 updates it —
  `[measured 8617a7c:internal/store/migrate_test.go:128-134 · sed -n '128,134p' internal/store/migrate_test.go → "pool.QueryRow(ctx, `SELECT count(*) FROM goose_db_version`).Scan(&count) … if count != 3 { t.Fatalf(\"goose_db_version rows = %d, want 3\", count) }"]`.
- **A new foreign key with no covering index fails a gate, not a review.** `event` adds
  foreign keys on `type`, `player_id` and `chat_id`, and `journal_entry` one on `event_id`.
  Mitigation: the index table in § Approach, whose composite `(type, ts)` covers the type FK
  by its leading column —
  `[measured 8617a7c:internal/store/fkcover_ac17_test.go:14-16 · sed -n '14,16p' … → "if got := uncoveredFKs(t, ctx, pool); len(got) != 0 { t.Fatalf(\"uncovered FKs on the migrated schema: %v, want none\", got) }"]`.
- **The `(*Event)(nil).insert` guard is unreachable from both entry points**, so a missing
  one would be invisible: `Post` rejects a typed-nil basis in phase a via `entrySQL` and
  never calls `insert`, and `AppendEvent` takes a value. Mitigation: subtask 3 extends the
  package-internal nil test, which asserts both methods for every implementation —
  `[measured 8617a7c:internal/store/post.go:76-82 · sed -n '76,82p' internal/store/post.go → "if basis == nil { return ErrNoBasis } entrySQL, err := basis.entrySQL() if err != nil { return err }" — phase a returns before phase d's insert]`.
- **A view that divides by an empty denominator errors instead of returning NULL**, and a
  freshly-migrated database hides it because there are no rows to divide. Mitigation:
  `NULLIF` on every denominator, and a fixture day whose new-player count is zero —
  `[derived → AC12's empty-denominator boundary in `metric_retention_daily`]`.
- **`sum(…) FILTER (…)` returns NULL over an empty filter**, so a kind with only a faucet
  would report a NULL sink rather than `0`. Mitigation: `COALESCE(…, 0)` on both legs —
  `[derived → AC12's faucet-only fixture row in `metric_faucet_sink`]`.
- **A bare `ts::date` makes the same view return different numbers on different machines**,
  because the suite pins only `search_path` and a `LAB_GAME_TEST_DSN` server's `TimeZone`
  need not match a container's. Mitigation: `(ts AT TIME ZONE 'UTC')::date` everywhere —
  `[measured 8617a7c:internal/testdb/testdb.go:155 · sed -n '155p' internal/testdb/testdb.go → "cfg.ConnConfig.RuntimeParams[\"search_path\"] = name" — the only runtime parameter set]`.
- **A shipped view column set cannot be narrowed later.** `CREATE OR REPLACE VIEW` appends
  only; a drop, rename, reorder or retype is `42P16` and forces `DROP VIEW` plus whatever
  reads it. Mitigation: § Approach's forward-migration paragraph, and naming the columns as
  deliberately as table columns — measured there.
- **`ErrUnknownEventType` leaves an aborted transaction**, and a caller that treats it like
  `ErrUnknownAccount` (transaction still usable) will fail confusingly on its next
  statement. Mitigation: the doc comment says so in the register the aborting sentinels
  already use —
  `[measured 8617a7c:internal/store/errors.go:36-48 · sed -n '36,48p' internal/store/errors.go → "ErrOverdraft … The transaction is aborted; the caller must roll back." / "ErrBalanceRowMissing … the caller must still roll back."]`.
- **Adding the sentinel falsifies the doc comments that describe the sentinel set and the
  phase that raises each**, and a stale comment is the same defect class as a broken test
  (DOC-5). Mitigation: the edits § Approach names for subtask 3 —
  `[measured 8617a7c:internal/store/errors.go:5-6 · sed -n '5,6p' internal/store/errors.go → "// Post's eight sentinels. Compare with errors.Is; see Post's doc comment for" / "// which phase raises each and what state the transaction is left in."]`.
- **The AC14 genericity test adds an enum member, which is irreversible within a
  connection.** Postgres refuses to *use* a value added by `ALTER TYPE … ADD VALUE` in the
  transaction that added it, so the test must add the member outside the transaction that
  then posts under it. It is safe to run at all only because `ledger_kind` is created inside
  the per-test schema and dropped with it —
  `[measured 8617a7c:internal/store/migrations/00001_ledger_core.sql:3 · sed -n '3p' … → "CREATE TYPE ledger_kind AS ENUM ('money', 'experience');" — created under the test's own search_path, and testdb.Schema drops that schema CASCADE on cleanup]`.
- **The append-only pattern could over- or under-match.** `event` as a bare alternative
  must match `UPDATE event SET …` and must not match `UPDATE event_type_definition SET …`.
  Mitigation: AC10's positive control gains the planted `event` statements and a decoy on
  the catalog table; a sweep whose control does not fire is not evidence
  (`AGENTS.md` § Patterns 2) — `[derived → AC10's positive control and decoy set]`.
- **Panic surface: none is added.** Every new path returns an error — the FK refusal, the
  scan failures, the nil-basis rejection. The index this would otherwise grow is empty and
  the project holds it at zero —
  `[measured 8617a7c:ai-docs/panic-index.md:5 · sed -n '5p' ai-docs/panic-index.md → "**The project targets zero production panics and currently holds it** — the table below is empty."]`.
- **`revive`'s `exported` rule makes a doc comment mandatory on every new exported item**,
  which here includes each dictionary member; the existing enum files carry one comment
  above the const block, which satisfies it —
  `[measured 8617a7c:.golangci.yml:45-48 · sed -n '45,48p' .golangci.yml → "revive:" / "rules:" / "- name: exported" / "- name: package-comments"]`.
  No enum `switch` is introduced by this design; if the implementor adds one, `exhaustive`
  requires it to be total —
  `[measured 8617a7c:.golangci.yml:18 · sed -n '18p' .golangci.yml → "- exhaustive         # every FSM state / enum switch is total (DESIGN §3.5)"]`.
- **The coverage ratchet measures on every commit of this task**, because each subtask
  stages `.go` or `.sql`. Mitigation: every subtask in Group A ships its tests in the same
  commit, which the decomposition already pairs —
  `[measured 8617a7c:AGENTS.md:77 · grep -n 'No `.go` / `.sql`' AGENTS.md → "| No `.go` / `.sql` / `go.mod` / `go.sum` staged | Skipped, silently — coverage cannot have moved. |"]`.
- **A propagation miss is the likeliest defect in Group B**, because the criterion is
  "every live site whose claim this diff falsifies", not the spec's illustrative list.
  Mitigation: subtask 8 runs the case-insensitive sweep over the full set and re-greps the
  files it has already edited, per the Procedure's corollary —
  `[measured 8617a7c:AGENTS.md:243 · grep -n 'a file you have already edited is not thereby done' AGENTS.md → "Corollary: **a file you have already edited is not thereby done** — re-grep it whole, after the edit"]`.

## Test Design

Each scenario below is a test that does not exist yet, so it carries `[derived → AC…]`.
Where it reuses an existing seam, that seam carries its own `[measured …]` — the seam is a
fact about the tree, the assertion is not.

### Subtask 1 — schema (`migrate_test.go`, `schema_test.go`)

- **Location:** the existing files, beside the code. Both already run against a real
  Postgres through the package's migrated-pool helper
  `[measured 8617a7c:internal/store/store_test.go:18-20 · sed -n '18,20p' internal/store/store_test.go → "// newStore builds a fresh, migrated schema-scoped pool for tb and closes it // on cleanup." / "func newStore(tb testing.TB) *pgxpool.Pool {"]`.
- **Entry points:** `Migrate`, then `information_schema` / `pg_catalog` reads, and direct
  SQL inserts inside a rolled-back transaction, using the existing SQLSTATE and rollback
  helpers
  `[measured 8617a7c:internal/store/schema_test.go:12-14,28-30 · sed -n '12,14p;28,30p' internal/store/schema_test.go → "// sqlstate asserts err wraps a *pgconn.PgError with the given SQLSTATE code // (and, when constraint is non-empty, the given constraint name)." / "func sqlstate(t *testing.T, err error, code, constraint string)" / "// rollback rolls tx back and fails the test on any error other than // pgx.ErrTxClosed" / "func rollback(t *testing.T, ctx context.Context, tx pgx.Tx)"]`.
- **Scenarios.**
  - Exact column set of `event`, by name, matching AC1's enumeration; exact base-table set
    (view-filtered); the enum member set extended with `event_volume_class`; the index list
    extended with the new names. The enum-member and index assertions are Group 2 rows — they
    pass today and must be written because AC1 says so, not because anything is red. `[derived → AC1]`
  - `journal_entry_exactly_one_basis`'s rendered definition **names the new arc column**.
    The existing substring check is satisfied by the pre-change form, so this assertion has
    to be strengthened, not merely re-run. `[derived → AC3]`
  - Idempotent re-apply leaves the schema and the goose ledger unchanged. `[derived → AC1]`
  - Arc rejections, as subtests: an insert naming no arc column → `23514` on
    `journal_entry_exactly_one_basis`; one naming `event_id` **and** another arc column →
    `23514`; a second `journal_entry` on an already-referenced event → `23505` on the event
    arc's partial unique index — which is also what makes the 1:1 consequence in § Approach
    executable rather than advisory. `[derived → AC4]`
  - Hygiene: no `-- +goose Down` in any migration, and a new `CREATE RULE` pattern beside
    the existing trigger and grant patterns. `[derived → AC2, AC10]`
- **Fixtures:** none new — the migrated-pool helper gives a schema-scoped database, and each
  rejection subtest opens and rolls back its own transaction. `[derived → AC4]`

### Subtask 2 — mirrors (`enums_test.go`)

- **Entry point:** the existing enum-mirror case slice and the existing catalog-mirror test,
  both of which enumerate explicitly and so must be *extended*, not merely re-run
  `[measured 8617a7c:internal/store/enums_test.go:16-23,52,73-74 · sed -n '16,23p;52p;73,74p' internal/store/enums_test.go → "{\"owner_kind\", stringsOf(ownerKinds)}, {\"ledger_kind\", stringsOf(kinds)}, {\"operation_source\", stringsOf(operationSources)}" / "SELECT id, code, owner_kind FROM scope_definition ORDER BY id" / "SELECT id, scope_definition_id, code, kind, controlled FROM account_definition ORDER BY id"]`.
- **Scenarios.** `event_volume_class`'s database members equal the Go slice as a set. The
  `event_type_definition` rows, read ordered by the `id` that exists for exactly this
  purpose, equal the Go registry slice **element for element including the class** — so a
  type present on one side only, or one whose class differs, fails; and because the
  assertion is on the whole slice rather than on membership, a reordering fails too.
  Fixture-free: the rows come from the migration. `[derived → AC5]`
- **Boundary:** the class partition is asserted by the same equality — exactly the
  high-volume members the spec names carry the high-volume class, and no others, because
  any other assignment changes an element. `[derived → AC5]`

### Subtask 3 — the write API (`event_test.go`, `basis_test.go`)

- **Location:** `internal/store/event_test.go` for the write paths; the nil-guard pair goes
  into the existing per-implementation nil test rather than a new one, so the new
  implementation sits beside the ones it must match
  `[measured 8617a7c:internal/store/basis_test.go:12-67 · sed -n '12,67p' internal/store/basis_test.go → a block per implementation asserting ErrNoBasis from both entrySQL() and insert(ctx, tx), plus the typed-nil-in-interface case]`.
- **Entry points:** `Post` with an `*Event` basis, `AppendEvent`, and the sealed interface's
  methods on a nil `*Event`.
- **Scenarios.**
  - Nil guards: `(*Event)(nil).entrySQL()` and `(*Event)(nil).insert(ctx, tx)` each return
    `ErrNoBasis`. Neither is reachable through the public API — see § Risks — so this test
    is the only thing that holds the spec's nil-receiver-safe contract for `Event`.
    `[derived → the spec's "A fifth implementation follows that contract exactly"]`
  - Happy path, basis: one `Post` under an `*Event` leaves one `event` row, one
    `journal_entry` whose event arc column references it and whose other arc columns are
    null, and the caller's postings — all read back inside the same transaction before
    rollback. `[derived → AC6]`
  - Typed-nil basis through `Post`: returns `ErrNoBasis` and issues no statement. The "no
    statement" half reuses the recorder seam and its reset discipline, which is how the
    existing sentinel table proves the same property for the other bases
    `[measured 8617a7c:internal/store/store_test.go:39-44 · sed -n '39,44p' internal/store/store_test.go → "// newStoreWithRecorder … Test Design's \"Recorder rule\": it also records Migrate, CreateOwner and funding, so every tracer-based assertion calls rec.Reset() immediately before the Post under test"]`,
    `[measured 8617a7c:internal/store/tracer_test.go:48-49,55-57 · sed -n '48,49p;55,57p' internal/store/tracer_test.go → "// Reset discards every recorded statement." / "func (r *queryRecorder) Reset()" / "// Statements returns a snapshot of every statement recorded since the last Reset." / "func (r *queryRecorder) Statements() []recordedStmt"]`,
    `[measured 8617a7c:internal/store/post_sentinels_test.go:34,120 · sed -n '34p;120p' internal/store/post_sentinels_test.go → "preSQLOnly bool // recorder must be empty after rec.Reset()" / "rec.Reset()"]`.
    `[derived → AC6]`
  - Happy path, append: `AppendEvent` writes one `event` row, returns its id, and leaves no
    `journal_entry` and no `posting` referencing it. `[derived → AC7]`
  - Unknown type: both paths, with a type absent from the registry, return an error
    satisfying `errors.Is(err, ErrUnknownEventType)`, and that sentinel is distinct from
    every existing `store` sentinel — asserted as a loop over the sentinel set rather than a
    single comparison, so a future sentinel added by aliasing fails. `[derived → AC8]`
  - No idempotency key: two `Post` calls built from equal `Event` values, in one
    transaction, produce two event rows and two journal entries — asserting the § Approach
    statement rather than leaving it as prose, so a later `ON CONFLICT` added without
    thought fails a test instead of silently changing the contract.
    `[derived → the § Approach consequence subtask 6 records]`
  - Transaction ownership: after either path the caller's `tx` is still open (a subsequent
    statement succeeds), and after the caller rolls back, a fresh connection sees no `event`
    row. `[derived → AC9]`
  - Payload: a nil payload round-trips as `{}`; a non-nil `json.RawMessage` round-trips
    equal after `jsonb` normalisation (compared as parsed JSON, not as bytes, because
    `jsonb` reorders keys). `[derived → AC1's non-null payload column]`
  - Nullable fields: an event with every optional field absent, and one with every optional
    field present including `Depth` pointing at zero, both round-trip — the second is the
    case a zero-as-NULL encoding would corrupt. `[derived → AC1]`
- **Fixtures:** a helper creating a chat owner and a player owner through the package's
  existing owner constructor
  `[measured 8617a7c:internal/store/owner.go:41 · sed -n '41p' internal/store/owner.go → "func CreateOwner(ctx context.Context, tx pgx.Tx, kind OwnerKind, telegramID *int64) (Owner, error) {"]`,
  plus the seeded World accounts for the posting legs
  `[measured 8617a7c:internal/store/ids.go:13-20 · sed -n '13,20p' internal/store/ids.go → "const WorldOwner OwnerID = 1" / "const WorldMoney AccountID = 1" / "const WorldExperience AccountID = 2"]`.

### Subtask 4 — the append-only sweep (`append_only_test.go`)

- **Entry point:** the existing pattern with its planted-control and decoy loops and its
  non-vacuity guard
  `[measured 8617a7c:internal/store/append_only_test.go:25-42,89-91 · sed -n '25,42p;89,91p' internal/store/append_only_test.go → the planted-statement loop, the decoy loop, and "if !slices.Contains(names, \"post.go\") || !slices.Contains(names, \"migrations/00001_ledger_core.sql\") { t.Fatalf(\"append-only scan is vacuous: …\") }"]`.
- **Scenarios.** The control fires on planted `UPDATE event` / `DELETE FROM event` lines;
  the decoy loop proves it does **not** fire on the catalog table's name, which is the
  word-boundary failure mode this addition introduces. The non-vacuity guard is extended to
  require the new migration in the collected set, so a cwd or embed surprise reports as
  vacuous rather than as a pass. `[derived → AC10]`

### Subtask 5 — the views (`views_test.go`)

- **Location:** `internal/store/views_test.go` — Postgres behaviour, tested against
  Postgres, on the same migrated-pool helper.
- **Entry points:** each view, queried with an explicit `ORDER BY` (the views carry none, so
  the test owns the ordering).
- **Scenarios.**
  - **Empty log.** On a freshly migrated database every view is queryable and returns zero
    rows — the assertion is "zero rows", never "no error", because an error is the failure
    mode being excluded. `[derived → AC11]`
  - **Exact view set.** The migrated schema's views equal the named set exactly; an extra
    view fails, which is what discharges AC13 without an untestable negative. Nothing
    enumerates views today, so this assertion is wholly new. `[derived → AC11, AC13]`
  - **One shared fixture, hand-computed expectations per view.** Events spanning several
    types, players, chats and days, inserted by direct SQL with explicit `ts` values (the Go
    API stamps `now()` by design, so a multi-day fixture is necessarily SQL), plus a small
    posting group for the faucet/sink view. Each view's expected rows are written out in the
    test as a literal table, **not** recomputed from the fixture — a test that recomputes the
    view's own logic asserts nothing. `[derived → AC12]`
  - **Boundary case per view, each chosen so the window edge decides the answer:**
    - funnel — a player whose second raid is the **same** day, and one whose second raid is
      **two** days later: neither counts as a next-day return, while a player raiding on
      exactly the next day does; and a player who pressed Start but never raided counts at
      the started stage only. `[derived → AC12]`
    - retention — a day with events but **no** new players, so both rate columns are NULL
      rather than a division error. `[derived → AC12]`
    - deaths by depth — a `death` with a null `depth`, which forms its own row.
      `[derived → AC12]`
    - faucet/sink — a kind with a faucet and no sink, so the sink column is `0` and not
      NULL. `[derived → AC12]`
    - notifications — a `notification_sent` with a null chat, which is absent from the
      output. `[derived → AC12]`
  - **`ledger_kind` genericity.** Add a member to the enum on the pooled connection (outside
    a transaction), seed a scope/account definition and an account of the new kind, post
    under a manual correction, and assert the new kind appears in the view's output with the
    view's SQL untouched. The test's own control is that the same query returned no row for
    that kind before the posting — without that control it cannot distinguish a working view
    from a query that would have matched anything. `[derived → AC14]`
- **Fixtures / helpers:** one seeding helper returning the owner ids it created, so each
  view's subtest reads the same world; and the existing rollback helper where a subtest
  mutates. `[derived → AC12]`

### Subtasks 6–8 — the documentation subtasks

No Go tests. Their acceptance is established by reading: AC15 and AC16 by the content of
`ai-docs/domain-invariants.md` §5 and its listing in `ai-docs/agent-docs-index.md` (already
present, so no index edit is required — the page is listed there as "Ledger, telemetry,
scheduler and Telegram-safety invariants"
`[measured 8617a7c:ai-docs/agent-docs-index.md:15 · grep -n 'domain-invariants' ai-docs/agent-docs-index.md → the index row for `ai-docs/domain-invariants.md`, described as "Ledger, telemetry, scheduler and Telegram-safety invariants — read before touching those paths"]`);
AC17 by reading the touched `docs/DESIGN.md` lines; AC18 by re-running the sweep after the
last edit. `[derived → AC15–AC18]`

The mechanical half is narrower than it looks, and the difference matters to subtask 7.
CI's link check globs **every** `.md` in the tree, so a relative link written in any of
these subtasks is checked
`[measured 8617a7c:.github/workflows/ci.yml:198 · sed -n '198p' .github/workflows/ci.yml → "for f in (p for p in pathlib.Path(\".\").rglob(\"*.md\") if \".git/\" not in str(p)):"]`.
The citation guard is **not** that broad: it reads `.claude/`, `AGENTS.md` and `ai-docs/`
only, minus `ai-docs/plans/` — so it covers subtasks 6 and 8 and does **not** reach
`docs/DESIGN.md`
`[measured 8617a7c:.claude/skills/ai-audit/scripts/check-citations.sh:130-132 · sed -n '130,132p' … → "grep -rnoE '(^|[^a-zA-Z0-9/_-])#[0-9]+\\b' .claude/ AGENTS.md ai-docs/ … | grep -v '^ai-docs/plans/' | grep -v '^ai-docs/deferred/'"]`.
Consequence for subtask 6: every `#N` it writes into `ai-docs/domain-invariants.md` for a
deferred family's owning issue must resolve against this repository's own numbering, which
the guard checks at run time against the live pull-request high-water mark
`[measured 8617a7c:.claude/skills/ai-audit/scripts/check-citations.sh:58 · sed -n '58p' … → "LOCAL_MAX=$(gh pr list --state all --limit 1 --json number --jq '.[0].number // 0' 2>/dev/null)"]`.
Every issue the spec's *Deferred* section names is inside this repository's numbering; a
number outside it would need its namespace spelled.

Repo-root user-facing docs are `AGENTS.md` and `CLAUDE.md` only — the tree tracks no
`README.md`, so the Propagation Rule step 4 sweep reaches no repo-root member beyond those
`[measured 8617a7c:. · git ls-files | grep -i readme → (no output)]`.

## Open questions

- **The reporting day is UTC.** Pinned by this design because a view must be deterministic
  across machines. If the owner wants a game-local day boundary (the settlement's timezone,
  say), that is a later change and it is not free: it turns each view's day expression into
  a parameter, which a plain `CREATE VIEW` cannot carry.
- **D1/D7 are read as exactly day + 1 and day + 7.** The common industry alternative is
  "active within the first N days". §13.3 writes «D1/D7 retention» without disambiguating.
  This design takes the exact reading; the owner may prefer the windowed one — which, given
  the `CREATE OR REPLACE VIEW` constraint above, is a same-column-set replacement and so
  remains cheap.
- **`event_type_definition.id` is written by hand in the migration**, like
  `scope_definition` and `account_definition` before it. Every mechanic that registers a
  type must pick the next free id, and nothing enforces that it is next. If that becomes
  friction, the fix is a later migration adding a default — not an edit to this one.
