# Design: Event log — the `event` table, the §13.4 dictionary, and the MVP SQL views

**Issue:** #21
**Date:** 2026-09-06

## Approach

The task ships the **substrate** of product analytics: one forward migration carrying the
append-only `event` table, its type registry as database data, the `journal_entry` arc
extension that makes an event a basis document, and the MVP views; plus the Go write API
and the doc-page edits. No mechanic emits anything here.

### A note on evidence

Two tag shapes appear below. `[measured <commit>:<path>:<lines> · …]` cites this tree at the
commit named. `[measured probe · psql on docker.io/library/postgres:18 → …]` cites a
throwaway container run against the image the suite itself uses — it carries **no** repo
coordinate, because the fact is Postgres's, not this repository's. A design rule resting on
remembered database behaviour is the same defect as one resting on remembered code.

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

### What the shipped views require of the mechanics that will emit events

This task emits nothing, but its views read columns that other PRs must populate. **The
obligation is real and it must be written down in both directions**, because every way of
getting it wrong is silent: a view whose input column is always null returns zero rows, not
an error, and this task's own fixture satisfies AC12 regardless of what any mechanic
actually emits.

| Event type | Dimension it must carry | What reads it | What a null does |
|---|---|---|---|
| `bot_added_to_chat` | `chat_id` | the funnel's **row set** | that chat has no funnel row at all — §13.3's headline MVP number is silently empty for it |
| `player_started` | `chat_id` | the funnel's **attribution map** | the player is attributed to no chat and is counted in no stage of any funnel |
| `player_started` | `player_id` | the funnel's attribution map; retention's new-player cohort | as above, and the cohort loses the player |
| `raid_started` | `player_id` | the funnel's raided and returned stages; retention's raid count and D1/D7 | the raid counts toward no player and no chat |
| `notification_sent` | `chat_id` | `metric_notification_per_chat_day` | the row is excluded — the spam-budget metric under-reports with no sign that it did |
| `death` | `depth` | `metric_death_by_depth` | the death lands in the null-depth group; **this is the one degradation that is visible**, because the group appears as its own row |
| `death` | `player_id` | that view's distinct-player column | the distinct-player count under-counts |

**And the non-obligations, which matter just as much because a reader looks for them.** No
shipped view reads `chat_id` from `raid_started` or from `death`; none reads `maze_id`; none
reads any payload key. A mechanic may set them and nothing here degrades if it does not —
which is a property of the views this task writes, so it is established by them and by
AC12's fixture, not by anything readable today
`[derived → the view definitions of AC11, and AC12's funnel fixture, whose `raid_started` rows carry no chat at all]`.

The whole table, both halves, goes into `ai-docs/domain-invariants.md` §5 (subtask 6) —
that is the page a mechanic author reads, and a page carrying only the non-obligations would
be worse than silence.

### The payload rule, and why it is the rule (AC15's "why")

**The §13.4 dimensions — player, chat, maze, depth — are columns; everything else is
JSONB.** Four reasons, and they are the reasoning subtask 6 transcribes rather than invents:

- §13.4 declares them *universal across types* — «Везде, где применимо: игрок, чат,
  лабиринт, глубина»
  `[measured 8617a7c:docs/DESIGN.md:428 · sed -n '428p' docs/DESIGN.md → "Стартовый словарь: `bot_added_to_chat`, … Везде, где применимо: игрок, чат, лабиринт, глубина."]`
  — so they are the one part of an event's shape that is not per-type.
- Every shipped view filters or groups on at least one of them (see the table above); no
  shipped view reads a payload key at all. A dimension a view groups by wants a btree index,
  and an index on a JSONB path is an expression index that has to be written per key.
- Two of them carry foreign keys to `owner`. JSONB cannot express referential integrity, so
  putting the player or the chat in the payload would abandon the guarantee the ledger's
  address space depends on.
- Everything else is per-type and unknowable in advance: a `combat_resolved` payload and a
  `shop_sale` payload share no field. Promoting either to a column would be a migration per
  mechanic plus a column that is null on every other type.

The rule is uniform across types, so it is one paragraph and not one per type. The escape
hatch runs one way only, deliberately: a payload key that turns out to be read by every view
can be promoted to a column by a later forward migration, whereas demoting a column is the
expensive direction — which is why the columns are the ones §13.4 already fixed, and not a
guess about what a future dashboard might want.

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

**Why the class may be an enum when the type registry may not.** The hygiene gate's cost —
an `ALTER TYPE … ADD VALUE` file may do nothing else — is paid per *addition*, and the two
axes are added at very different rates. A new event type is routine: every mechanic PR
brings one or more, and each would need a second migration file for its class row. A new
volume class is rare and is itself a retention-policy change that wants its own migration
and its own review, so the gate's cost lands where it belongs. The asymmetry also runs the
other way: a *type* carries an attribute (its class), so it needs a row; a *class* carries
nothing, so an enum member is the whole of it, and the database then refuses an unknown
class outright rather than needing a CHECK.

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
`[measured 8617a7c:internal/store/migrations/00001_ledger_core.sql:7,54 · grep -n 'GENERATED' internal/store/migrations/*.sql → BY DEFAULT on owner (:7), scope (:30) and account (:39); ALWAYS on player_operation (:54), manual_correction (:61), journal_entry (:68), posting (:79), and on scheduled_task, deferred_task and recurrent_task in 00002]`.
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
| `journal_entry_event_key` | `journal_entry (event_id) WHERE event_id IS NOT NULL`, UNIQUE | Covers the new arc FK **and** enforces the 1:1 arc — see the arc extension below |

The FK-coverage helper accepts a partial index whose predicate contains `IS NOT NULL`, and
compares the FK's column set against the index's **leading** columns
`[measured 8617a7c:internal/store/fkcover_test.go:84-96 · sed -n '84,96p' … → "if idx.pred != nil && !strings.Contains(strings.ToUpper(*idx.pred), \"IS NOT NULL\") { continue } … leading := sortedCopy(idx.cols[:len(want)])"]`,
which is what lets one composite `(type, ts)` index serve both roles.

**The arc extension**, copying `00002_scheduler.sql`'s established form in one
`ALTER TABLE`: add `event_id bigint REFERENCES event (id)`, drop and re-add
`journal_entry_exactly_one_basis` naming the pre-existing arc columns plus `event_id`, then
the partial unique index above
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
before this migration keeps exactly one non-null basis and satisfies the re-added CHECK.
That is load-bearing rather than incidental: `ADD CONSTRAINT` validates the new CHECK
against existing rows and refuses the whole migration if any row fails it
`[measured probe · psql on docker.io/library/postgres:18, INSERT -1 into a table then ALTER TABLE … ADD CONSTRAINT CHECK (n > 0) → "23514: check constraint \"g_pos\" of relation \"g\" is violated by some row"]`,
so it is the *old* CHECK that guarantees the new one applies cleanly. During a deploy window
the **old** binary runs correctly against the **new** schema (it never names `event_id`, and
its inserts still satisfy the CHECK); the **new** binary against the **old** schema does
not, which is why `store.Migrate` runs at start-up before anything else.

**Rollback:** there is no down migration by policy (KD-3). Reverting the *code* is safe on
its own — the new tables and column are inert to a binary that does not name them.
Reverting the *schema* would be a new forward migration dropping `event_id`, and it must
not be written once any `journal_entry` references an event, because that drops the basis
of a live posting group. The correct correction is always a further forward migration.

**A shipped view's column set is a contract too, and `CREATE OR REPLACE VIEW` is narrower
than "replace" suggests.** The Key-decisions rationale for putting views in migrations
leans on a later family arriving by `CREATE OR REPLACE VIEW` — but that statement may only
**append** columns
`[measured probe · psql on docker.io/library/postgres:18, CREATE VIEW v AS SELECT a, b FROM e then four CREATE OR REPLACE VIEW variants → appending c succeeded; dropping c → "42P16: cannot drop columns from view"; renaming b → "42P16: cannot change name of view column \"b\" to \"bb\""; reordering → "42P16: cannot change name of view column \"a\" to \"b\""; retyping b to text → "42P16: cannot change data type of view column \"b\" from integer to text"]`.
So the column names, order and types this task ships are as durable as the table's: a later
change that is not an append is `DROP VIEW` + `CREATE VIEW` in a forward migration, and any
Grafana panel bound to the old shape breaks with it. **This is why § The views names every
column of every view rather than describing them** — a contract that permanent is not the
implementor's to choose.

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
`INSERT`. (The same gate is why the *class* stays an enum — see above.)

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
`Depth *int32` — the `…ID` suffix following `DeferredTask.TaskID`'s precedent
`[measured 8617a7c:internal/store/basis.go:104-109 · sed -n '104,109p' internal/store/basis.go → "type DeferredTask struct { TaskID int64; TaskType string; InstanceKey string; RunAt time.Time }"]`.
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

**No occurrence-time parameter.** `ts` defaults to `now()`, and inside a transaction `now()`
is the transaction's start instant
`[measured probe · psql on docker.io/library/postgres:18, BEGIN then SELECT now() = transaction_timestamp() → t]`
— so an event written as a posting basis carries the *same* instant as its
`journal_entry.ts`, whose default is also `now()`
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
  phase list becomes the incomplete comment DOC-5 forbids
  `[measured 8617a7c:ai-docs/doc-convention.md:51,56 · sed -n '51p;56p' ai-docs/doc-convention.md → "## DOC-5 — What not to write" / "- No stale comment: changing behaviour without updating the comment above it is the same defect class as a broken test."]`.

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
2. **Every denominator is wrapped in `NULLIF(x, 0)`, every aggregate that can be empty in
   `COALESCE(…, 0)`, and every ratio's numerator cast to `numeric`.** None of them is
   cosmetic: division by zero raises rather than yielding NULL; a `FILTER` that matches
   nothing yields NULL rather than zero; and `count(…) / count(…)` is **integer** division,
   which silently returns `0` for every ratio below 1 — the worst of them, because it
   is a plausible-looking wrong answer rather than an error
   `[measured probe · psql on docker.io/library/postgres:18 → SELECT 1/0 raises "22012: division by zero"; sum(v) FILTER (WHERE v < 0) IS NULL returns t over a table holding only a positive row; count(*) / NULLIF(count(*)*3, 0) returns 0 while count(*)::numeric / NULLIF(count(*)*3, 0) returns 0.33333333333333333333]`.
3. **A column whose type is a custom enum is cast to `text` in the view.** Only
   `metric_faucet_sink` has one (`account_definition.kind`). The cast is not there because
   a failure was observed — pgx already decodes `ledger_kind` as text elsewhere in this
   package
   `[measured 8617a7c:internal/store/post.go:113-120 · sed -n '113,120p' internal/store/post.go → "var kindText string … rows.Scan(&id, &kindText, &info.controlled) … info.kind = Kind(kindText)"]`
   — but AC14's test adds an enum member at run time on a pooled connection, and `::text`
   removes any dependence on when a given connection resolved that type's OID. A view is
   also read by Grafana, which is not pgx.
4. **A count column is `0`, never NULL, including when its stage matched nothing.** This is
   structural rather than a guard to remember: `count(expr)` over an outer-joined miss
   returns `0`
   `[measured probe · psql on docker.io/library/postgres:18, SELECT count(x.id), count(x.id) IS NULL FROM e LEFT JOIN (SELECT 99 AS id) x ON x.id = e.id → 0, f]`.
   Only the *ratio* columns are nullable, and only where their denominator is empty.

Naming: a `metric_` prefix. It groups the views together in the Grafana Postgres
datasource's picker, and it keeps them clear of §11's singular-table-name decision, which
governs entity tables — a view named for a question is not an entity.

Each view records its own name and the §13.3 question it answers in a SQL comment directly
above it (AC11). Those comments are **English**, and they cite the section, never a line —
the Russian surface is `docs/**` and owner conversation, nothing else
`[measured 8617a7c:AGENTS.md:4 · grep -n 'English for every durable artefact' AGENTS.md → "**English for every durable artefact** — code, comments, … **Russian for two surfaces only:** conversation with the product owner, and `docs/**`"]`,
and a line reference into a document that gets edited rots
`[measured 8617a7c:ai-docs/doc-convention.md:49 · grep -n 'Cite the section number' ai-docs/doc-convention.md → "Cite the section number, never a line number: the design document is edited, and `§2.2.4` survives what `:118` does not."]`.

**Column names, order and types are fixed here, not by the implementor** — they are the
permanent contract the forward-migration paragraph describes. Types are what the listed
expression yields
`[measured probe · psql on docker.io/library/postgres:18, information_schema.columns over a view of (ts AT TIME ZONE 'UTC')::date, count(*), and count(*)::numeric / NULLIF(count(*),0) → date, bigint, numeric]`.

#### `metric_activation_funnel` — attribution runs through `player_started`

**This is an owner decision (round 3), and it replaces an earlier design that attributed a
raid to a chat by reading `chat_id` off the `raid_started` event itself.** That earlier
shape imposed an unrecorded obligation on whoever emits `raid_started` to populate
`chat_id`, and it failed *silently* in both directions: a null chat produced no anomaly row,
and this task's own fixture would have satisfied AC12 forever regardless.

The attribution is therefore a property of the **player**, not of the raid:

- **The attribution map.** Each player with a `player_started` naming a chat belongs to the
  chat of their **earliest** such event; ties are broken by the lower chat id, so the map is
  a function and the view is deterministic. A `player_started` with a null chat attributes
  its player nowhere, and that player is then counted in no chat's funnel.
- **The row set** is still one row per chat holding a `bot_added_to_chat`, with that event's
  earliest instant. A chat that was never added has no funnel row even if players attribute
  to it.
- **The raid stages** join on **`player_id` alone**. `raid_started.chat_id` is never read.

Because every stage filters the same attributed population, the funnel is **monotone by
construction** — each stage is a subset of the one before it, which the earlier shape did
not guarantee.

| Column | Type | Meaning |
|---|---|---|
| `chat_id` | `bigint` | the chat's `owner.id` |
| `added_at` | `timestamptz` | earliest `bot_added_to_chat` instant for that chat |
| `player_started` | `bigint` | players attributed to this chat; `0` when none |
| `player_first_raided` | `bigint` | of those, players with any `raid_started`; `0` when none |
| `player_returned_next_day` | `bigint` | of those, players with a `raid_started` on the day after their own earliest raid day; `0` when none |

Every stage column obeys rule 4: an empty stage is `0`, never NULL, so a chat whose players
never raided still shows its `player_started` count against explicit zeroes rather than
against blanks a reader must interpret.

**The cost, which the owner accepted and which must be stated.** A player active in several
chats is always credited to the chat they started from, even for raids that conceptually
belong to another. This touches `docs/DESIGN.md` §16.7, «Привязка игрок↔чат (membership)» —
an **open question**, not something this design resolves
`[measured 8617a7c:docs/DESIGN.md:486 · sed -n '486p' docs/DESIGN.md → "7. **Привязка игрок↔чат (membership)** — … игрок может принадлежать нескольким чатам; рейд всегда стартует **от конкретного чата** (сессия привязана к чату) …"]`.
Membership is owned by #30
`[measured 8617a7c:. · gh issue view 30 --json number,title,state → {"number":30,"state":"OPEN","title":"Chat location, deep-link onboarding, and player-to-chat membership"}]`.
Note what §16.7's own proposed default implies: once a raid session is bound to a chat, a
better attribution exists and this view becomes a candidate for replacement — a
same-column-set `CREATE OR REPLACE VIEW`, so cheap under the constraint above.

**Consequences for other PRs, both directions.** `raid_started` owes this view **no**
`chat_id`. `bot_added_to_chat` and `player_started` owe it one, and owe it a `player_id`
where the table in § What the shipped views require says so — that table, whole, is what
subtask 6 carries into `domain-invariants.md`.

#### The remaining views

**`metric_retention_daily`**

| Column | Type | Meaning |
|---|---|---|
| `day` | `date` | the UTC day |
| `active_player` | `bigint` | distinct players with any event that day |
| `raid` | `bigint` | `raid_started` count that day |
| `raid_per_active_player` | `numeric` | `raid` over `active_player`; NULL when no player was active |
| `new_player` | `bigint` | players whose earliest `player_started` is that day |
| `d1_retained` | `bigint` | of those, players with a `raid_started` on day + 1 |
| `d7_retained` | `bigint` | of those, players with a `raid_started` on day + 7 |
| `d1_rate` | `numeric` | `d1_retained` over `new_player`; NULL when the cohort is empty |
| `d7_rate` | `numeric` | `d7_retained` over `new_player`; NULL when the cohort is empty |

**Row set:** one row per UTC day on which **any** event occurred — not a generated calendar
range and not only days that have a cohort. A day with no events is therefore absent rather
than zero-filled, and a day whose `new_player` is `0` still appears because something else
happened on it. (`CREATE OR REPLACE VIEW` polices the column list and not the row set, so
every view below states its own.)

Two readings are *chosen* here rather than read off §13.3, and both go in the view's SQL
comment as well as § Open questions, because §13.3 names the metric without defining it
`[measured 8617a7c:docs/DESIGN.md:419 · sed -n '419p' docs/DESIGN.md → "- Возвраты: D1/D7 retention, рейдов на игрока в день."]`:

- **The return signal is a raid, not any activity** (owner, round 3). "Any event that day"
  is the commoner industry reading and is literally one column over in this same view, so a
  reader will assume it unless told. Declared: a cohort member counts as returned on day + N
  only if they have a `raid_started` that day.
- **D1 and D7 are exactly day + 1 and day + 7**, not "within N days".

The comment must also carry the **cohort-immaturity** warning, because the numbers lie
without it: a cohort born fewer than seven days ago cannot yet have a D7, and by rule 4 the
view reports `0`, not NULL, for it. A reader scanning the recent end of the series sees a
retention cliff that is an artefact of the window, not of the game. The same holds for D1 on
today's cohort.

**`metric_death_by_depth`**

| Column | Type | Meaning |
|---|---|---|
| `day` | `date` | the UTC day |
| `depth` | `integer` | the depth; **NULL is a real group** — deaths with no recorded depth |
| `death` | `bigint` | deaths at that depth that day |
| `player` | `bigint` | distinct players who died at that depth that day |

**Row set:** one row per (UTC day, `depth`) that has at least one `death`, with NULL depth
forming its own group.

**Why a `day` column, when §13.3 asks only «Смерти по глубине»**
`[measured 8617a7c:docs/DESIGN.md:421 · sed -n '421p' docs/DESIGN.md → "- Смерти по глубине; судьба рюкзаков: брошен / подобран / испарился. …"]`.
Round 4 shipped this view all-time, which left a Grafana panel unable to restrict it to a
range and left the choice unmade rather than made. The two directions are not symmetric: with
a day column a reader recovers the all-time distribution with `GROUP BY depth`, whereas
without one no query recovers the daily split, and adding the grain later is not an append —
it changes the row set, so it costs `DROP VIEW` + `CREATE VIEW` plus every panel bound to the
old shape. The day leads the column list, as it does in the other day-grained views. The
argument against — that a daily cut at MVP scale is the noise §13.4 warns about — is a
panel-configuration matter, and the collapse is one clause away.

The null-depth group is deliberate: the row is a data anomaly worth seeing, and silently
discarding it would misreport the total. It is also the only degradation in the § What the
shipped views require table that a reader can actually notice.

**`metric_faucet_sink`**

| Column | Type | Meaning |
|---|---|---|
| `day` | `date` | the UTC day, taken from `journal_entry.ts` |
| `kind` | `text` | `account_definition.kind`, cast per rule 3 |
| `faucet` | `numeric` | value leaving World for the economy; `0` when none |
| `sink` | `numeric` | value returning to World; `0` when none |
| `net_to_economy` | `numeric` | `faucet` − `sink` |

**Row set:** one row per (UTC day, kind) for which at least one posting touched a
World-owned account that day. A kind with no World traffic on a day is absent, not zero.

Reached through `posting → journal_entry` (for the day), `posting → account →
account_definition` (for the kind) and `account → scope → owner` restricted to
`owner.kind = 'world'`. Faucet is the negated sum of the World legs that are negative; sink
is the sum of the positive ones; net is the negated total. Both are `COALESCE`d to `0`.
**AC14 falls out of the construction**: the view names no `ledger_kind` member anywhere — it
groups by `account_definition.kind` — so a new member appears as soon as postings of that
kind exist.

**`metric_notification_per_chat_day`**

| Column | Type | Meaning |
|---|---|---|
| `day` | `date` | the UTC day |
| `chat_id` | `bigint` | the chat's `owner.id` |
| `notification` | `bigint` | `notification_sent` events for that chat that day |

**Row set:** one row per (UTC day, chat) with at least one chat-bearing `notification_sent`.
A quiet chat-day is absent, not a zero row — which matters for a spam-budget panel, because
"no row" and "zero notifications" read the same on a graph and differently in a query.

Rows with a null chat are **excluded**: §13.3's metric is the per-chat spam budget
`[measured 8617a7c:docs/DESIGN.md:424 · sed -n '424p' docs/DESIGN.md → "- **Нотификации: штук в чат в день + CTR кнопок.** Одновременно продукт и здоровье: спам-бюджет — дизайн-обязательство (см. 1), за ним надзор."]`,
and a chat-less notification is not a per-chat number. The exclusion is
asserted by a fixture row, not left implicit — and it is why `notification_sent` appears in
the obligation table above.

**`bot_kicked` gets no view, and that is the answer, not an omission.** §13.3 calls it the
terminal metric — *every event is an autopsy*
`[measured 8617a7c:docs/DESIGN.md:425 · sed -n '425p' docs/DESIGN.md → "- Терминальная метрика: **бот кикнут из чата.** Каждое событие — вскрытие."]`
— i.e. the reader wants the individual rows. An aggregate over `bot_kicked` destroys
exactly what the metric is for; the query is
`SELECT * FROM event WHERE type = 'bot_kicked'`. The spec left this to this design
`[measured 8617a7c:ai-docs/plans/2026-09-05-event-log-dictionary-views.spec.md:307-310 · sed -n '307,310p' … → "**`bot_kicked` as a view.** … `design-writer` may fold it into the notifications view or leave it out."]`.
No JSONB GIN index either, for the reason the spec gives: no shipped view filters on a
payload key.

### What the existing suite catches, and what it does not

The distinction below is the completeness net for the implementor, so it is drawn by
measurement rather than by intuition. **Only Group 1 below goes red on the new schema.**
Everything in Group 2 passes green *without* being touched — the edits there are coverage
the ACs require, not failures the gate reports. Reading "suite green" as "table discharged"
would leave AC3, AC5, AC10 and AC11 unwritten, which is the instrument-is-a-claim shape this
project's own append-only positive control exists to prevent (`AGENTS.md` § Patterns 2).

**Group 1 — fails red without the edit.**

| Assertion | Why it goes red |
|---|---|
| `TestMigrate_shape_and_seeds`'s table list | An exact-set comparison over a query with no `table_type` filter, so the first `CREATE VIEW` lands in `tables` and set equality fails — `[measured 8617a7c:internal/store/migrate_test.go:20-45 · sed -n '20p;36,45p' internal/store/migrate_test.go → "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()" … "want := []string{\"account\", … \"scope_definition\"}" … "if !slices.Equal(tables, want)"]`; and views do land there — `[measured probe · psql on docker.io/library/postgres:18, SELECT table_name, table_type FROM information_schema.tables after a CREATE VIEW → the view is listed, with table_type = VIEW]` |
| `TestMigrate_noop_reapply`'s goose row count | A literal `count != 3`; a further migration moves it — `[measured 8617a7c:internal/store/migrate_test.go:128-134 · sed -n '128,134p' internal/store/migrate_test.go → "SELECT count(*) FROM goose_db_version … if count != 3 { t.Fatalf(\"goose_db_version rows = %d, want 3\", count) }"]` |

**Group 2 — passes green without the edit. The AC forces the coverage, the gate does not.**

| Assertion | Why it stays green | Forced by |
|---|---|---|
| `TestMigrate_shape_and_seeds`'s enum map | A hard-coded `map[string][]string` literal that is ranged over, so an enum it does not name is never queried — `[measured 8617a7c:internal/store/migrate_test.go:48-53 · sed -n '48,53p' internal/store/migrate_test.go → "for enum, wantMembers := range map[string][]string{ \"owner_kind\": …, \"ledger_kind\": …, \"operation_source\": …, \"scheduled_task_state\": … }"]` | AC1 |
| `TestMigrate_indexes_constraints_and_column_types`'s index list | A membership loop (`if !found[want]`) over a literal `want` list — an index absent from the list cannot fail it — `[measured 8617a7c:internal/store/migrate_test.go:221-233 · sed -n '221,224p;229,233p' internal/store/migrate_test.go → "for _, want := range []string{ \"owner_kind_telegram_id_key\", … }" … "if !found[want] { t.Errorf(\"index %s is missing (have %v)\", want, found) }"]` | AC1 |
| The `journal_entry_exactly_one_basis` assertion | Matched on the substring `num_nonnulls`, which the pre-change form already contains, so it passes on a CHECK that never gained the new arc column — `[measured 8617a7c:internal/store/migrate_test.go:239 · sed -n '239p' internal/store/migrate_test.go → "\"journal_entry_exactly_one_basis\":     \"num_nonnulls\","]` | AC3 |
| `TestEnums_mirror_database` | An explicit `{enum, want}` slice; a new enum is simply not among the cases — `[measured 8617a7c:internal/store/enums_test.go:16-23 · sed -n '16,23p' internal/store/enums_test.go → "{\"owner_kind\", stringsOf(ownerKinds)}, {\"ledger_kind\", stringsOf(kinds)}, {\"operation_source\", stringsOf(operationSources)}"]` | AC5 |
| `TestCatalog_mirrors_database` | Hand-written queries against the seeded catalogs; a further catalog is never read — `[measured 8617a7c:internal/store/enums_test.go:52,73-74 · sed -n '52p;73,74p' internal/store/enums_test.go → "SELECT id, code, owner_kind FROM scope_definition ORDER BY id" / "SELECT id, scope_definition_id, code, kind, controlled FROM account_definition ORDER BY id"]` | AC5 |
| `TestMigrate_hygiene` | Its regex set has no rule pattern, so a `CREATE RULE` in a migration is invisible to it — `[measured 8617a7c:internal/store/migrate_test.go:137-148 · sed -n '137,148p' internal/store/migrate_test.go → downRe, renameValueRe, dropValueRe, createTrigRe, grantRe, upRe, addValueRe, createTableRe, insertRe, updateRe — no CREATE RULE pattern]` | AC10 |
| `TestAppendOnly_…` | Its pattern names only the ledger tables, so it cannot fire on `event` — `[measured 8617a7c:internal/store/append_only_test.go:13 · sed -n '13p' internal/store/append_only_test.go → "regexp.MustCompile(`(?i)update\\s+(posting\\|journal_entry)\\b\\|delete\\s+from\\s+(posting\\|journal_entry)\\b`)"]` | AC10 |
| `TestBasis_nil_returns_ErrNoBasis_without_panicking` | Hand-enumerates each existing implementation, asserting `ErrNoBasis` on **both** `entrySQL()` and `insert()`; a new implementation with a missing nil guard is neither reached nor asserted — `[measured 8617a7c:internal/store/basis_test.go:12-67 · sed -n '12,67p' internal/store/basis_test.go → "var nilPO *PlayerOperation … nilPO.entrySQL() … nilPO.insert(ctx, tx)" and the same pair for ManualCorrection, DeferredTask and RecurrentTask]` | the spec's "every implementation nil-receiver-safe … A fifth implementation follows that contract exactly" |
| The view-set assertion | Does not exist at all today; nothing enumerates views | AC11, AC13 |
| `TestFKCoverage` | Needs no edit — it is generic over the schema and it is the gate the index set above is built to satisfy — `[measured 8617a7c:internal/store/fkcover_test.go:35-40 · sed -n '35,40p' internal/store/fkcover_test.go → "SELECT conname, conrelid::bigint, conkey FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace WHERE c.contype = 'f' AND n.nspname = current_schema()"]` | — |

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Migration: `event_volume_class`, `event_type_definition` with its §13.4 seeds and the `id`-is-for-ordering comment, `event` with its indexes, the `journal_entry` arc extension. Update the table-list and goose-count assertions (Group 1) and add the enum-map, index-list, strengthened `num_nonnulls` and `CREATE RULE` coverage (Group 2), plus the arc-rejection subtests (AC1–AC4, AC10's hygiene half) | `internal/store/migrations/00003_event_log.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 2 | Go mirrors: `EventVolumeClass`, `EventType` + the §13.4 members, `EventTypeDefinition` + the registry slice; extend both mirror tests (AC5) | `internal/store/enums.go`, `internal/store/catalog.go`, `internal/store/enums_test.go` | 1 |
| 3 | Write API: `EventID`, `Event` as a `PostingBasis` implementation, `AppendEvent`, `ErrUnknownEventType` with the `errors.go`/`post.go` doc edits § Approach names, `PostingBasis`'s doc comment; tests for both paths, the nil-guard pair, the sentinel and transaction ownership (AC6–AC9) | `internal/store/ids.go`, `internal/store/event.go`, `internal/store/errors.go`, `internal/store/post.go`, `internal/store/basis.go`, `internal/store/basis_test.go`, `internal/store/event_test.go` | 2 |
| 4 | Append-only sweep: extend the pattern to `event`, add its planted controls and the `event_type_definition` decoy, and extend the non-vacuity guard to the new migration (AC10) | `internal/store/append_only_test.go` | 1 |
| 5 | The views: append them to the migration **with the exact column names, order and types § The views tables fix** and their English `§13.3` comments (the funnel's `player_started` attribution **and its next-day return semantic**, retention's cohort-immaturity and raid-signal clauses), add the exact-view-set assertion, and the fixture-driven per-view expectations with their boundary cases plus the `ledger_kind` genericity test. **Each expected row is a literal in the test, never recomputed from the fixture** — a test that recomputes a view's own logic asserts nothing, and this is the one instruction that stops it (AC11–AC14) | `internal/store/migrations/00003_event_log.sql`, `internal/store/migrate_test.go`, `internal/store/views_test.go` | 1, 2 |
| 6 | `domain-invariants.md` §5: the payload rule **with the reasoning § Approach states**; the whole § *What the shipped views require* table — obligations **and** non-obligations together; the mechanic-facing arc consequences (1:1, and no idempotency key); the §13.5 dashboard limb recorded as suspended with its lift condition; the deferred view families with their owning issues; the `events` → `event` spelling (AC15, AC16, and §5's share of AC18) | `ai-docs/domain-invariants.md` | — |
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

Subtask 5 carries the most judgement in Group A, and it runs at the `sonnet`/`medium` tier
like the rest of the group. That is deliberate rather than overlooked, and it is why
§ The views now fixes each view's **column names, order and types** as well as its grain,
exclusions and guards: nothing durable is left to the implementor's choice, so the subtask
is transcription against a spec. The one instruction that must not be softened in transit is
the literal-expectations rule, which is why it appears in the decomposition row, in § Test
Design and here.

## Risks

- **"Suite green" is not "table discharged".** Most of § *What the existing suite catches*
  Group 2 passes without any edit, so an implementor who works to the gate rather than to
  the ACs ships a green branch missing AC3's strengthened constraint assertion, AC5's mirror
  extensions, AC10's hygiene and sweep extensions, and AC11's view-set assertion. Mitigation:
  the two-group split above, with the forcing AC named on every Group 2 row, and Step 9's
  per-AC sweep as the backstop — `[derived → the per-AC verification of AC3, AC5, AC10, AC11]`.
- **A view whose input column is never populated returns zero rows, not an error**, and no
  gate in this task can catch it, because this task writes its own fixture. The funnel's row
  set needs `chat_id` on `bot_added_to_chat`, its attribution needs `chat_id` on
  `player_started`, and the notification metric needs `chat_id` on `notification_sent` — if
  any of those is emitted without it, §13.3's headline number and the spam-budget metric are
  silently empty forever. The obligation is recorded against the **event type**, not against
  an issue number, because a wrong-but-existing `#N` passes the citation guard unchallenged;
  the emitters whose issue titles actually name them are #30 and #43
  `[measured 8617a7c:. · gh issue view 30/43 --json number,title,state → #30 [OPEN] "Chat location, deep-link onboarding, and player-to-chat membership"; #43 [OPEN] "Chat notification queue: rate limiter, the MVP set, deep-link buttons"]`,
  and no open issue title names an emitter for `bot_added_to_chat`. Mitigation: § *What the
  shipped views require* states each obligation, the metric it feeds and its failure mode,
  and subtask 6 carries that table —
  both halves — into the page a mechanic author actually reads —
  `[derived → subtask 6's `domain-invariants.md` §5 content, and AC15/AC16's reading check]`.
- **The permanent half of a view is its column set**, and `CREATE OR REPLACE VIEW` can only
  append: a drop, rename, reorder or retype is `42P16` and forces `DROP VIEW` plus every
  reader bound to it. Mitigation: § The views fixes every column's name, order and type, and
  § Handoff plan explains that this is what keeps subtask 5 transcription rather than design
  — `[measured probe · psql on docker.io/library/postgres:18 → the four CREATE OR REPLACE VIEW variants above; appending succeeded, dropping/renaming/reordering/retyping each raised 42P16]`.
- **The table-list assertion silently becomes a view-list assertion.**
  `information_schema.tables` reports views alongside base tables, and the existing exact-set
  assertion applies no type filter, so the first `CREATE VIEW` breaks it in a way that reads
  like a spurious failure. Mitigation: subtask 1 filters the assertion to base tables and
  subtask 5 adds the views' own exact-set assertion, which is also what establishes AC13 —
  an extra view fails set equality, so no separate "no deferred view" negative has to be
  written —
  `[measured 8617a7c:internal/store/migrate_test.go:20-22 · sed -n '20,22p' internal/store/migrate_test.go → "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()" — no table_type filter]`.
- **The goose version-row assertion is a hard-coded count.** A further migration moves it.
  Mitigation: subtask 1 updates it —
  `[measured 8617a7c:internal/store/migrate_test.go:128-134 · sed -n '128,134p' internal/store/migrate_test.go → "pool.QueryRow(ctx, `SELECT count(*) FROM goose_db_version`).Scan(&count) … if count != 3 { t.Fatalf(\"goose_db_version rows = %d, want 3\", count) }"]`.
- **A new foreign key with no covering index fails a gate, not a review.** `event` adds
  foreign keys on `type`, `player_id` and `chat_id`, and `journal_entry` one on `event_id`.
  Mitigation: the index table in § Approach, which lists a covering index for each of those
  — including `journal_entry_event_key` — and whose composite `(type, ts)` covers the type
  FK by its leading column —
  `[measured 8617a7c:internal/store/fkcover_ac17_test.go:14-16 · sed -n '14,16p' … → "if got := uncoveredFKs(t, ctx, pool); len(got) != 0 { t.Fatalf(\"uncovered FKs on the migrated schema: %v, want none\", got) }"]`.
- **The `(*Event)(nil).insert` guard is unreachable from both entry points**, so a missing
  one would be invisible: `Post` rejects a typed-nil basis in phase a via `entrySQL` and
  never calls `insert`, and `AppendEvent` takes a value. Mitigation: subtask 3 extends the
  package-internal nil test, which asserts both methods for every implementation —
  `[measured 8617a7c:internal/store/post.go:76-82 · sed -n '76,82p' internal/store/post.go → "if basis == nil { return ErrNoBasis } entrySQL, err := basis.entrySQL() if err != nil { return err }" — phase a returns before phase d's insert]`.
- **An uncast ratio is a plausible wrong answer, not an error.** `count(…) / count(…)` is
  integer division, so every rate below 1 renders as `0` and a reader sees "no retention"
  rather than a failure. Mitigation: view rule 2's `::numeric` on every numerator, and
  fixture expectations with a fractional rate so the truncation cannot pass — probed under
  rule 2 above, `[derived → AC12's retention expectations, whose D1 rate is a fraction]`.
- **A view that divides by an empty denominator errors instead of returning NULL**, and a
  freshly-migrated database hides it because there are no rows to divide. Mitigation:
  `NULLIF` on every denominator, and a fixture day whose new-player count is zero —
  probed under rule 2 above, `[derived → AC12's empty-denominator boundary in `metric_retention_daily`]`.
- **`sum(…) FILTER (…)` returns NULL over an empty filter**, so a kind with only a faucet
  would report a NULL sink rather than `0`. Mitigation: `COALESCE(…, 0)` on both legs —
  probed under rule 2 above, `[derived → AC12's faucet-only fixture row in `metric_faucet_sink`]`.
- **A bare `ts::date` makes the same view return different numbers on different machines**,
  because the suite pins only `search_path` and a `LAB_GAME_TEST_DSN` server's `TimeZone`
  need not match a container's. Mitigation: `(ts AT TIME ZONE 'UTC')::date` everywhere —
  `[measured 8617a7c:internal/testdb/testdb.go:155 · sed -n '155p' internal/testdb/testdb.go → "cfg.ConnConfig.RuntimeParams[\"search_path\"] = name" — the only runtime parameter set]`.
- **`ErrUnknownEventType` leaves an aborted transaction**, and a caller that treats it like
  `ErrUnknownAccount` (transaction still usable) will fail confusingly on its next
  statement. Mitigation: the doc comment says so in the register the aborting sentinels
  already use —
  `[measured 8617a7c:internal/store/errors.go:36-48 · sed -n '36,48p' internal/store/errors.go → "ErrOverdraft … The transaction is aborted; the caller must roll back." / "ErrBalanceRowMissing … the caller must still roll back."]`.
- **Adding the sentinel falsifies the doc comments that describe the sentinel set and the
  phase that raises each**, and a stale comment is the same defect class as a broken test
  (DOC-5 — measured under § Approach's sentinel paragraph). Mitigation: the edits
  § Approach names for subtask 3 —
  `[measured 8617a7c:internal/store/errors.go:5-6 · sed -n '5,6p' internal/store/errors.go → "// Post's eight sentinels. Compare with errors.Is; see Post's doc comment for" / "// which phase raises each and what state the transaction is left in."]`.
- **The AC14 genericity test adds an enum member, which cannot be used by the transaction
  that added it.** Postgres refuses the use outright
  `[measured probe · psql on docker.io/library/postgres:18, BEGIN; ALTER TYPE k ADD VALUE 'b'; SELECT 'b'::k → "55P04: unsafe use of new value \"b\" of enum type k" with the hint "New enum values must be committed before they can be used."]`,
  so the test must add the member outside the transaction that then posts under it. It is
  safe to run at all only because `ledger_kind` is created inside the per-test schema and
  dropped with it —
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
    extended with the new names. The enum-member and index assertions are Group 2 rows —
    they pass today and must be written because AC1 says so, not because anything is red.
    `[derived → AC1]`
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
- **What this fixture is and is not evidence about.** It is evidence about the **SQL**: that
  each view computes what § The views specifies, on rows the test wrote itself. It is **not**
  evidence about the emission pipeline — nothing emits events yet, and no assertion here can
  tell whether a future mechanic populates a column the way the view expects. That gap is
  closed by writing the obligation down (§ What the shipped views require, carried into
  `domain-invariants.md` by subtask 6), not by a test this task can run. `[derived → AC12]`
- **Scenarios.**
  - **Column contract.** Each view's column names, order and types match § The views'
    tables, read from `information_schema.columns`. This is cheap and it is the only thing
    that pins a contract `CREATE OR REPLACE VIEW` cannot later loosen. `[derived → AC11]`
  - **Empty log.** On a freshly migrated database every view is queryable and returns zero
    rows — the assertion is "zero rows", never "no error", because an error is the failure
    mode being excluded. `[derived → AC11]`
  - **Exact view set.** The migrated schema's views equal the named set exactly; an extra
    view fails, which is what discharges AC13 without an untestable negative. Nothing
    enumerates views today, so this assertion is wholly new. `[derived → AC11, AC13]`
  - **One shared fixture, hand-computed expectations per view.** Events spanning several
    types, players, chats and days, inserted by direct SQL with explicit `ts` values (the Go
    API stamps `now()` by design, so a multi-day fixture is necessarily SQL). The faucet/sink
    view's rows come the same way and for the same reason: its `day` is taken from
    `journal_entry.ts`, which `Post` would stamp `now()`, so the fixture writes its
    `journal_entry` and `posting` rows by **direct SQL with an explicit `ts`** — otherwise the
    expected `day` could not be a literal, and computing it in the test would reintroduce a
    midnight-boundary edge the literal exists to avoid. (The new `event_id` arc is available
    to that fixture, so a ledger row written this way can still name its event.) **Each
    view's expected rows are written out in the test as a literal table, never recomputed
    from the fixture** — a test that recomputes the
    view's own logic asserts nothing, and would pass against almost any wrong view.
    `[derived → AC12]`
  - **Boundary case per view, each chosen so the window edge decides the answer:**
    - funnel — the fixture emits **every `raid_started` with a null `chat_id`**, which is
      what proves the attribution runs through `player_started` rather than through the raid;
      a player whose second raid is the **same** day and one whose second raid is **two**
      days later (neither is a next-day return, while a player raiding on exactly the next
      day is); a player who pressed Start but never raided (counted at the started stage
      only, and the later stages must read `0`, not blank, per rule 4); a player with a
      `player_started` in a second chat *later* (credited to the first, per the
      earliest-then-lowest-id rule); and a `player_started` with a null chat (attributed
      nowhere, so present in no chat's row). `[derived → AC12]`
    - retention — a day with events but **no** new players, so both rate columns are NULL
      rather than a division error; and a cohort whose D1 rate is a **fraction**, so an
      uncast integer division would render `0` and fail. `[derived → AC12]`
    - deaths by depth — a `death` with a null `depth`, which forms its own row; and deaths
      on more than one UTC day, so the new `day` column is exercised rather than assumed
      constant. `[derived → AC12]`
    - faucet/sink — a kind with a faucet and no sink, so the sink column is `0` and not
      NULL. `[derived → AC12]`
    - notifications — a `notification_sent` with a null chat, which is absent from the
      output. `[derived → AC12]`
  - **`ledger_kind` genericity.** The fixture needs **two opposing legs of the new kind**,
    because `Post` refuses an unbalanced batch
    `[measured 8617a7c:internal/store/post.go:138-142 · sed -n '138,142p' internal/store/post.go → "for _, sum := range sumByKind { if !sum.IsZero() { return ErrUnbalanced } }"]`,
    and one of them must sit on a World-owned account or the view will not see it. Order
    matters, because `CreateOwner` builds a new owner's accounts from the
    `account_definition` rows that exist **at creation time**
    `[measured 8617a7c:internal/store/owner.go:59-70 · sed -n '59,70p' internal/store/owner.go → "INSERT INTO scope (owner_id, scope_definition_id) SELECT $1, id FROM scope_definition WHERE owner_kind = $2 … INSERT INTO account (scope_id, account_definition_id) SELECT s.id, d.id FROM s JOIN account_definition d ON d.scope_definition_id = s.scope_definition_id"]`:
    add the enum member on the pooled connection outside any transaction; insert an
    `account_definition` of the new kind for the world scope and one for the player scope;
    create the player owner (which then gets the new-kind account and its balance row);
    insert the world-scope account of the new kind; then `Post` the two opposing legs and
    query the view. The test's own control is that the same query returned no row for that
    kind before the posting — without that control it cannot distinguish a working view from
    a query that would have matched anything. `[derived → AC14]`
- **Fixtures / helpers:** one seeding helper returning the owner ids it created, so each
  view's subtest reads the same world; and the existing rollback helper where a subtest
  mutates. `[derived → AC12]`
- **Budget note, not a projected problem.** This file is one shared fixture plus a literal
  expectation table per view, and `_test.go` is hard-gated
  `[measured 8617a7c:Makefile:29-30,48-50 · sed -n '29,30p;48,50p' Makefile → "GO_MAX_LINES ?= 1000" / "GO_MAX_TEST_LINES ?= 1500" / the file-limits recipe applying the test limit to any path matching _test\.go$]`.
  Nothing here is expected to approach it; the point is that its implementor should know the
  gate exists **before** choosing how verbose the expectation tables get, because the cheap
  fix is a second `_test.go`, not a terser table — terseness is what the literal-expectations
  rule forbids.

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
The citation guard is **not** that broad: it reads `.claude/`, `AGENTS.md` and `ai-docs/`,
minus `ai-docs/learnings.md`, `ai-docs/bugfix/`, `ai-docs/plans/`, `ai-docs/deferred/` and
the guards' own test fixtures — so it covers subtasks 6 and 8 and does **not** reach
`docs/DESIGN.md`
`[measured 8617a7c:.claude/skills/ai-audit/scripts/check-citations.sh:130-134 · sed -n '130,134p' … → "grep -rnoE '(^|[^a-zA-Z0-9/_-])#[0-9]+\\b' .claude/ AGENTS.md ai-docs/ 2>/dev/null | grep -v learnings.md | grep -v '^ai-docs/bugfix/' | grep -v '^ai-docs/plans/' | grep -v '^ai-docs/deferred/' | grep -v '/scripts/test-[a-z-]*\\.sh:'"]`.
Consequence for subtask 6: every `#N` it writes into `ai-docs/domain-invariants.md` — for a
deferred family's owning issue, for #30 beside the funnel's attribution note, and for the
emitters named in the obligation table — must resolve against this repository's own
numbering, which the guard checks at run time against the live pull-request high-water mark
`[measured 8617a7c:.claude/skills/ai-audit/scripts/check-citations.sh:58 · sed -n '58p' … → "LOCAL_MAX=$(gh pr list --state all --limit 1 --json number --jq '.[0].number // 0' 2>/dev/null)"]`.
Every issue named by the spec's *Deferred* section and by this design is inside this
repository's numbering; a number outside it would need its namespace spelled.

Repo-root user-facing docs are `AGENTS.md` and `CLAUDE.md` only — the tree tracks no
`README.md`, so the Propagation Rule step 4 sweep reaches no repo-root member beyond those
`[measured 8617a7c:. · git ls-files | grep -i readme → (no output)]`.

## Open questions

These are the readings this design *chose* where the design corpus is silent. Each is
recorded so a later reader finds a decision rather than an assumption. The view-shaped ones
are cheap to revisit: each is a same-column-set replacement, which is the one kind
`CREATE OR REPLACE VIEW` allows.

- **The reporting day is UTC.** Pinned because a view must be deterministic across machines.
  If the owner wants a game-local day boundary (the settlement's timezone, say), that turns
  each view's day expression into a parameter, which a plain `CREATE VIEW` cannot carry.
- **A "return" in D1/D7 retention means a raid, not any activity** (owner, round 3). §13.3
  writes only «Возвраты: D1/D7 retention». "Any event that day" is the commoner industry
  reading and sits one column over in the same view, so the choice is declared in the view's
  comment as well as here.
- **D1 and D7 are exactly day + 1 and day + 7**, not "active within the first N days".
- **The funnel's last stage reads "returned" the same way**: a `raid_started` on the day
  immediately after that player's own earliest raid day — not "raided again at any later
  point", and not "was active the next day". It is the same class of chosen reading as the
  two above and it feeds §13.3's headline MVP number, so it is declared in the view's SQL
  comment rather than left inside the column table.
- **The activation funnel attributes a player to the chat they started in** (owner, round 3),
  so a player active in several chats is credited to one. The general question is
  `docs/DESIGN.md` §16.7 «Привязка игрок↔чат (membership)», owned by #30 — this design does
  not resolve it, and says so in `domain-invariants.md` so the next reader does not think it
  was.
- **`event_type_definition.id` is written by hand in the migration**, like
  `scope_definition` and `account_definition` before it. Every mechanic that registers a
  type must pick the next free id, and nothing enforces that it is next. If that becomes
  friction, the fix is a later migration adding a default — not an edit to this one.
