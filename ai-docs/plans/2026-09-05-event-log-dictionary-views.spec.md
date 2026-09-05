# Event log: the `event` table, the §13.4 dictionary, and the MVP SQL views

**Source:** issue #21
**Date:** 2026-09-05
**Tracked in:** #21

The product-analytics half of observability. `docs/DESIGN.md` §13.1 fixes the stance —
**raw events, never pre-aggregated counters**: at MVP scale every dashboard question is a
SQL query over the log, and aggregating early throws away what was never recorded. §14
item 10 puts the event log in the MVP slice explicitly, «лог событий + канарейки из 13 —
с первого дня, потом не прикручивается дешево»
[source: b1206c6:docs/DESIGN.md:460 · `sed -n '460p' docs/DESIGN.md`].

This issue ships the **substrate**, not the traffic: the table, the type registry, the
write API, the payload rule and the five MVP views that today's tree can compute. Each
mechanic emits its own events in its own PR, under the standing invariant that telemetry
never lags code (`AGENTS.md` § *Domain Rules*; `docs/DESIGN.md` §13.4
[source: b1206c6:docs/DESIGN.md:430 · `sed -n '430p' docs/DESIGN.md`]).

## Scope

1. **One forward migration** under `internal/store/migrations/`, next in sequence after
   `00001_ledger_core.sql` and `00002_scheduler.sql`, picked up by the existing
   `//go:embed migrations/*.sql` in `internal/store/migrate.go:16`. It carries:
   - the append-only `event` table (columns per Key decisions below);
   - the event-type registry as database data, including its volume class (Scope 2);
   - the **arc extension** on `journal_entry`: a new nullable FK column to `event`, the
     `journal_entry_exactly_one_basis` constraint dropped and re-added naming **all five**
     arc columns, and a partial unique index on the new column — exactly the shape
     `00002_scheduler.sql:38-46` established for `deferred_task` / `recurrent_task`;
   - the five MVP SQL views of Scope 5.

   Forward-only: no `-- +goose Down` section, matching both existing migration files.

2. **The event-type registry**, as data the database and Go agree on, seeded with the
   **sixteen** §13.4 starter types in the design's own order — `bot_added_to_chat`,
   `player_started`, `raid_started`, `node_entered`, `combat_resolved`, `raid_finished`,
   `death`, `backpack_dropped`, `backpack_looted`, `backpack_expired`,
   `stamina_burned_overflow`, `shop_sale`, `shop_purchase`, `notification_sent`,
   `button_clicked`, `bot_kicked`
   [source: b1206c6:docs/DESIGN.md:428 · `sed -n '428p' docs/DESIGN.md`] — plus a Go
   mirror of those rows and a mirror check in the shape of `TestCatalog_mirrors_database`
   (`internal/store/enums_test.go:46`), which the issue names as the precedent to copy.

   Each registered type carries a **volume class**, and the partition is fixed by the
   owner (round 1, Q1): `notification_sent` and `button_clicked` on the high-volume,
   health-flavoured side; the other **fourteen** on the low-volume product side. The class
   exists so the later retention pass can split by class without a migration to
   *classify* — only one to act. Member names are `design-writer`'s call and become a data
   contract once shipped.

   A later mechanic extends the registry **by migration**, never by editing a comment.

3. **A typed Go write API in `internal/store`**, two entry points because the traffic has
   two shapes:
   - an event that **is** a posting basis — a `PostingBasis` implementation so
     `store.Post(ctx, tx, basis, postings...)` writes the event row, the `journal_entry`
     and the postings inside the caller's one transaction, making the event and the
     balance movement a single atomic act;
   - an event with **no** postings (the majority: `node_entered`, `notification_sent`,
     `button_clicked`, …) — an append that writes the `event` row inside the caller's
     transaction and creates no `journal_entry`.

4. **Payload discipline, written down.** The four §13.4 dimensions — player, chat, maze,
   depth — are **columns**; everything else is JSONB. The rule *and its reasoning* land on
   a live agent-doc page reachable from `ai-docs/agent-docs-index.md`, not only in a
   migration comment.

5. **Five MVP SQL views** — the §13.3 families computable from the four columns, the type,
   the timestamp and today's ledger (owner, round 1, Q2 — *ship the five*):
   1. **activation funnel** — `bot_added_to_chat` → `player_started` → first
      `raid_started` → return the next day (§13.3's headline MVP number);
   2. **retention** — D1/D7 plus raids per player per day;
   3. **deaths by depth** — `death` over the `depth` column;
   4. **faucet/sink per resource** — over `posting`, `account`, `account_definition` and
      the World owner; generic over whatever `ledger_kind` members exist at query time,
      so it does not need editing when #31 adds stamina;
   5. **notifications per chat per day** — `notification_sent` by chat by day.

   Each view's name and the §13.3 question it answers are recorded together in the tracked
   SQL. The other four families are deferred with their blocking input named below.

6. **`docs/DESIGN.md` edits** — authorised by the owner (round 1, Q3), minimal, and in
   **Russian**; the corpus is not translated and nothing else in §11 or §13 is touched:
   - `:402`, `:437`, `:442` — the table's name becomes `event`, matching §11:313's dated
     singular-names decision. Only the identifier changes; the surrounding prose does not.
   - `:331` — the «Стартовый реестр типов оснований» list gains «игровое событие
     (лог 13.1)», which `:323` and `:332` already name. This closes the event half of
     Source conflict 2; the raid-session half at `:323` stays #36's (see *Deferred*).

7. **Propagation, as a class.** Every live site whose claim this diff falsifies, per
   `AGENTS.md` § *Propagation Rule* step 4 — the sweep runs over `.claude/`, `AGENTS.md`,
   `ai-docs/` and, because a table name is a factual claim rather than a rule keyword,
   `docs/**` and any repo-root user-facing doc (the tree has **no `README.md` today**;
   membership is decided by the criterion, not by that absence). Known members at
   `b1206c6`, illustrative and not exhaustive:
   `ai-docs/context.md:38` («`events` log (product)…») and
   `ai-docs/domain-invariants.md:52` («reads the raw `events` log…»), plus the four
   `docs/DESIGN.md` lines of Scope 6. A sweep of `.claude/**` at `b1206c6` found the table
   named nowhere in it.

   **Two exclusions, both deliberate.** History surfaces — `ai-docs/learnings.md`,
   `ai-docs/plans/done/**` — are left untouched, per step 4's completeness test. And
   `ai-docs/deferred/_inbox.jsonl:106`, which quotes the scheduler spec's «the `events`
   table has no migration yet», is **not** hand-edited: `AGENTS.md` makes that file
   writable only by `/task` Step 12 and `/triage`, and the row is a record of what an
   earlier spec said, not a live claim about the schema.

## Out of scope

- **The Grafana Postgres datasource, panels and provisioning JSON.** The views land here;
  the dashboard that reads them does not.
- **Event retention and partitioning** (§11's three-step retention is explicitly
  «не MVP» [source: b1206c6:docs/DESIGN.md:330 · `sed -n '330p' docs/DESIGN.md`]). This
  task ships the volume class the retention pass will read; it ships no retention
  behaviour, no partitioning and no delete path.
- **Emitting the events.** Each mechanic emits its own, in its own PR.
- **Health metrics and canaries** — the other half of §13, issue #23.
- **Posting-signature contract tests** — issue #26 owns the framework; this task only
  makes an event usable as a basis document.
- **The raid-session arc column** — #36's, and with it the `:323` half of Source
  conflict 2.

## Deferred

- **The §13.5 third limb of the telemetry invariant** — «харнесс обновляет дашборд при
  добавлении событий»
  [source: b1206c6:docs/DESIGN.md:440 · `sed -n '440p' docs/DESIGN.md`] — is **suspended,
  not dropped**. The lift condition is a check, shipped by the infrastructure pass, that
  reads this task's registry and fails while any registered type has no panel; until then
  a mechanic issue adds no panel line, and the registry is the ledger of what is owed. |
  There is no dashboard to update yet, and a limb nobody can satisfy is a limb everybody
  learns to skip. | **Yes — no Grafana/provisioning issue exists** (the open set runs
  #21–#47; #23 is health metrics, not the dashboard stack).
- **Raid-outcomes view** — needs `raid_finished`'s outcome and a key pairing a finish with
  its start; neither is a column and no mechanic has defined the payload. | Writing it now
  would fix a mechanic-facing payload shape before that mechanic is designed. | No — lands
  with the PR that emits `raid_finished`: #39 (terminal edges) over #36's session FSM.
- **Backpack-fate view** — the bare dropped/looted/expired counts are computable from the
  types alone, but §13.3's social metric («Доля подобранных чужих рюкзаков», `:421`) needs the
  backpack's owner, which is payload. Shipping half a family under its own name would
  misreport it. | Same reason. | No — #40 (death, corpse and backpack).
- **Button-CTR view** — needs the linking key between `notification_sent` and
  `button_clicked`; no mechanic has defined it, and the two events are emitted by
  different PRs, so those PRs must agree on the key. | Same reason. | No — #43 (chat
  notification queue, deep-link buttons) and #37 (leader screen, inline buttons).
- **Stamina-utilisation view** — «потрачено/накапало» needs stamina postings, and
  `ledger_kind` today has exactly two members, `money` and `experience`
  [source: b1206c6:internal/store/migrations/00001_ledger_core.sql:3 · `sed -n '3p' internal/store/migrations/00001_ledger_core.sql`];
  burn-at-cap needs an amount in `stamina_burned_overflow`'s payload. | The inputs do not
  exist. | No — #31 (stamina: lazy accrual, cap, per-step spend, burn on death).
- **`maze` foreign key.** The `maze` column ships without a foreign key because no maze
  table exists; #29 (maze persistence) adds the FK by migration. | Column now, integrity
  later, so events recorded before #29 are not lost. | No — #29 covers it.
- **The `:323` half of Source conflict 2.** `docs/DESIGN.md:323`'s illustrative list still
  omits «переход рейд-сессии», which `:331` names. | Q3 authorised the registry-membership
  edit at `:331`, not a second edit at `:323`; #36 adds that arc column and closes it by
  the same construction. | No — #36 covers it.

## Key decisions

| Question | Decision |
|---|---|
| Table name | **`event`**, singular. Chosen by the owner in the issue body (three occurrences), consistent with §11:313's «имена таблиц — в единственном числе, решение 2026-09-02», and propagated to every live site by Scope 6 and 7. |
| One table or two, for the high-volume types? | **One table plus a per-type volume class** (owner, round 1, Q1). All sixteen types land in `event`; `notification_sent` and `button_clicked` carry the high-volume class, the other fourteen the product class. Every view is one `FROM`, and the retention pass splits by class later without re-classifying. |
| Which MVP views ship now? | **The five computable families** (owner, round 1, Q2): activation funnel, retention, deaths by depth, faucet/sink per resource, notifications per chat per day. The other four land in their mechanic's PR — see *Deferred*, each with its blocking input and owning issue. |
| Does this PR edit `docs/DESIGN.md`? | **Yes, both conflicts** (owner, round 1, Q3): the `events`→`event` spelling at `:402`/`:437`/`:442`, and «игровое событие (лог 13.1)» added to the starting registry at `:331`. Russian, minimal, no other prose touched. |
| Is the event log itself the basis document, or does a wrapper table sit between? | **The log is the document.** §11:323 names the basis type as «игровое событие (из лога 13.1)» and §11:332 as «проводки ссылаются на событие как на один из типов оснований» — one table, `journal_entry.<event arc column>` referencing it. No wrapper. |
| Arc extension shape | Copy `00002_scheduler.sql:38-46` in form: `ALTER TABLE journal_entry ADD COLUMN … , DROP CONSTRAINT journal_entry_exactly_one_basis, ADD CONSTRAINT … CHECK (num_nonnulls(<all five columns>) = 1)`, then the partial unique index `WHERE <col> IS NOT NULL`. The CHECK stays the grep-able registry of every basis type (§11:323). |
| Registry storage — seeded catalog table or Postgres enum? | **Default: a seeded catalog table** mirrored in Go. The issue names the *catalog*-mirror test as the precedent; the infrastructure pass must **query** the registry for its panel check; and a row can carry the volume class, which an enum member cannot. `design-writer` may argue the `ledger_kind`-style enum instead only if it also finds a home for the class — the binding requirements are that a new type costs a migration, that the class travels with the type, and that Go and the database are checked to agree. |
| `player` / `chat` columns | Nullable foreign keys to `owner(id)`. The ledger addresses players and chats as `owner` rows, and the funnel/retention views must join events to postings; nullable because `button_clicked` can arrive from a Telegram user who has no owner row yet. |
| `maze` / `depth` columns | Both nullable, both without a foreign key (no maze table exists — *Deferred*). `depth` is an integer dimension, not a balance number. |
| Where the views live | In the goose migration, as `CREATE VIEW`. Views are derived, not data: a later change is a later migration using `CREATE OR REPLACE VIEW`, which keeps one schema source applied by `store.Migrate` and keeps the views testable in the same suite. It also means a deferred family arrives as a migration in its mechanic's PR, not as an edit to this one. |
| Append-only enforcement | **Code-level, mirroring the ledger's KD-3 posture**: no `UPDATE`/`DELETE` against `event` in non-test `internal/store` code, no trigger and no grant in any migration. |
| Balance numbers | None. This task ships no tuning value; `depth`, counts and window lengths inside a view are query structure, not game balance (§16.5). |
| Payload rule | Four dimensions as columns, everything else JSONB, reasoning on a live doc page (Scope 4). The rule is uniform across types, so it is one paragraph, not sixteen. |
| Shared-helper placement | The Go registry mirror is consumed by `internal/store` and, later, by every mechanic package that emits an event — a call-site count well past three. Its placement (inside `internal/store` vs a dedicated package) is **left to `design-writer`**; this spec does not bake in duplication. |

## Technical constraints

- **Data contract.** `event`, its registry rows, the volume-class values and the new arc
  column are live data from the first deploy. They change by forward migration only; a
  registry entry or a class value is never renamed or renumbered in place
  (`AGENTS.md` § *API Stability* carve-out).
- **The exclusive arc is 1:1.** Each arc column carries a partial unique index, so one
  event backs **at most one** `journal_entry`. A mechanic that must move balances twice
  under one event needs two events or a different basis — a constraint the design must
  state, not discover.
- **Four arc columns exist today** — `player_operation_id`, `manual_correction_id`,
  `deferred_task_id`, `recurrent_task_id`
  [source: b1206c6:internal/store/migrations/00002_scheduler.sql:43-44 · `sed -n '38,46p' internal/store/migrations/00002_scheduler.sql`] —
  and `store.PostingBasis` is a **sealed** interface with two unexported methods
  (`entrySQL`, `insert`), every implementation nil-receiver-safe and returning
  `ErrNoBasis`. A fifth implementation follows that contract exactly, including the
  typed-nil rejection `Post` relies on.
- **The five shipped views run against an empty log.** Nothing emits events yet, so every
  view's correctness is established on a fixture, and every view must return zero rows
  rather than an error on a freshly-migrated database.
- **Never redesign `docs/DESIGN.md`.** §13's mechanics are decisions to implement. The
  Scope 6 edits reconcile the corpus with its own §11 — a table name and a registry
  membership line — and change no mechanic.
- **Tests provision Postgres through testcontainers** (KD-20): `postgres:18`, major tag
  only (KD-16); `LAB_GAME_TEST_DSN` points the suite at a running server instead. Postgres
  behaviour — the CHECK, the FKs, the views — is tested against Postgres, never a mock.
- **Executability of unattended checks.** `.claude/settings.json` `permissions.allow`
  grants `Bash(psql *)`; it grants **neither `podman` nor `docker`**, so every acceptance
  check must be reachable through `go test` / `make` (or `psql` against an already-running
  server). A design step that needs a container command unattended specs its grant as its
  own line.
- **Coverage ratchet.** This task stages `.go` and `.sql`, so `.githooks/coverage-ratchet.sh`
  measures on every commit; new Go code carries its tests in the same commit.
- **Language split.** The `docs/` corpus is Russian, so the Scope 6 edits are written in
  Russian; the spec, the design, the code and every comment are English.

## Source conflicts

### Conflict 1 — the log table's name (`docs/DESIGN.md`)

- `docs/DESIGN.md:313` (§11, dated general rule): «Все изменения балансов … — проводки в
  append-only леджере (таблица `posting`; имена таблиц — в единственном числе, решение
  2026-09-02)…»
- `docs/DESIGN.md:402` (§13.1): «Append-only таблица `events` в том же Postgres:
  игрок, чат, тип, JSONB-payload, timestamp.»
- `docs/DESIGN.md:437` (§13.5): «продуктовый: **Postgres-датасорс** — панели напрямую из
  SQL-вью поверх `events`, без нового компонента в стеке.»
- `docs/DESIGN.md:442` (§13.5): «…структурированное уже в `events`, для текстовых
  логов контейнеров хватает journald/docker logs.»
- Downstream repeats, same spelling: `ai-docs/domain-invariants.md:52` («The product
  dashboard reads the raw `events` log…»), `ai-docs/context.md:38` («`events` log
  (product)…»).

**Resolution, and who chose it.** The name is `event`, singular — chosen by the **owner**,
first in the issue #21 body (three occurrences) and then explicitly in **round 1, Q3**,
where the owner also authorised the corpus edit: *"EDIT BOTH. §13.1/§13.5 spelling becomes
`event`."* Scope 6 rewrites the three `docs/DESIGN.md` sites; Scope 7 carries the two
`ai-docs` sites and the open class. **Closed.**

### Conflict 2 — membership of the basis-document type registry (`docs/DESIGN.md` §11)

Handed forward to this issue by the ledger-core spec, which recorded it unresolved
(`ai-docs/plans/done/2026-09-02-ledger-post-core.spec.md:524-546`: «the discrepancy stays
open for whoever lands the event log»).

- `docs/DESIGN.md:323` (illustrative, open-ended): «Типы оснований … : игровое событие
  (из лога 13.1), ручная коррекция, cron-задача (закрытие дня), отложенная one-time,
  рекуррентная таска, закрытие сезона, операция игрока (`player_operation`, источник —
  enum; 2026-09-02)…» — **omits the raid-session transition.**
- `docs/DESIGN.md:331` (labelled the starting registry): «**Стартовый реестр типов
  оснований** (exclusive arc, расширение — миграцией): операция игрока
  (`player_operation`, источник — enum; 2026-09-02), **переход рейд-сессии (3.5)**,
  cron-задача (закрытие дня), отложенная one-time (испарение трупа), рекуррентная таска,
  ручная коррекция, закрытие сезона.» — **omits the game event.**
- `docs/DESIGN.md:332`: «…связь — проводки ссылаются на событие как на один из типов
  оснований.»

**Resolution, and who chose it.** The **owner**, round 1, Q3: *"§11:331 gains «игровое
событие (лог 13.1)»."* Scope 6 makes that edit, and this task adds the `event` arc column
to the one registry that is executable — the `num_nonnulls(...)` CHECK, which §11:323
itself calls «grep-абельный реестр всех типов».

**Half-closed, deliberately, and this is the part not to misread.** After this PR, `:331`
and `:323` and `:332` agree that the game event is a basis type. They do **not** yet agree
about «переход рейд-сессии», which `:331` names and `:323` still omits. That half is #36's
— it adds the raid-session arc column and closes the discrepancy by the same construction
— and it is carried in *Deferred* so #36's spec-writer finds it recorded rather than
re-deriving it from scratch, which is precisely the failure the owner's *"record it as
closed"* instruction is aimed at.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Applying the embedded migrations to an empty `postgres:18` database yields, in addition to the pre-existing schema, an `event` table whose columns are exactly: an identity primary key, a non-null reference to the event-type registry, a nullable player reference to `owner`, a nullable chat reference to `owner`, a nullable maze identifier, a nullable depth integer, a non-null `jsonb` payload, and a non-null `timestamptz` occurrence time. Applying the migration set a second time changes nothing. |
| AC2 | No file under `internal/store/migrations/` contains a `-- +goose Down` section. |
| AC3 | `journal_entry` carries a nullable foreign-key column referencing `event`, and the `journal_entry_exactly_one_basis` constraint names **five** arc columns — the four that existed before this change plus the new one. A partial unique index on the new column exists, restricted to its non-null rows. |
| AC4 | Against the migrated database: a direct SQL insert into `journal_entry` naming none of the five arc columns is refused with SQLSTATE `23514`; one naming two of them is refused with `23514`; a second `journal_entry` referencing an already-referenced `event` row is refused with `23505`. |
| AC5 | The database's event-type registry contains exactly the sixteen §13.4 starter types, each carrying a volume class, and exactly two of them — `notification_sent` and `button_clicked` — carry the high-volume class. The Go registry mirror equals the database's rows exactly, class included, in the database's own order; a type or class present in one and absent or different in the other is a failing state. |
| AC6 | `internal/store` exports a game-event posting basis that satisfies the package's sealed `PostingBasis` interface, so that a single `store.Post` call under that basis leaves, in the caller's one transaction: one `event` row, one `journal_entry` referencing it through the new arc column, and the caller's postings. On a typed-nil basis the call returns `ErrNoBasis` and issues no statement, matching the four existing implementations. |
| AC7 | `internal/store` exports a no-posting append path that writes one `event` row inside a caller-supplied transaction and creates no `journal_entry` and no `posting`. An event written this way is referenced by no `journal_entry`. |
| AC8 | An attempt to write an event whose type is not in the registry is refused by the database, and the Go API surfaces that refusal as a package-level sentinel distinguishable with `errors.Is` from every existing `store` sentinel. |
| AC9 | Both write paths leave the transaction's ownership with the caller: neither commits nor rolls back, and a caller that rolls back leaves no `event` row behind. |
| AC10 | No non-test statement in `internal/store` targets the `event` table with `UPDATE` or `DELETE`, and no migration creates a trigger, rule or grant on it. The sweep used to establish this has a positive control that fires on planted lines; a sweep with no positive control is not evidence (`AGENTS.md` § Patterns 2). |
| AC11 | Exactly the five views of Scope 5 exist in the migrated database — activation funnel, retention, deaths by depth, faucet/sink per resource, notifications per chat per day. Each is queryable against a freshly-migrated, event-free database and returns zero rows rather than an error. Each view's name and the §13.3 question it answers are recorded together in the tracked SQL. |
| AC12 | For a fixture of events spanning several types, players, chats and days, each of the five views returns the values a hand-computed expectation states for that fixture — including at least one boundary case per view where the window edge (a same-day versus next-day return, a player with no second day, an empty denominator) decides the answer. |
| AC13 | The migration creates no view, and no placeholder for one, covering any of the four deferred families — raid outcomes, backpack fate, button CTR, stamina utilisation. |
| AC14 | The faucet/sink view is generic over `ledger_kind` membership: adding a new member to that enum, without editing the view, makes postings of the new kind appear in the view's output. |
| AC15 | A live agent-doc page states the payload rule — which dimensions are columns, which live in JSONB, and why — and is listed in `ai-docs/agent-docs-index.md`. |
| AC16 | The live page carrying the telemetry invariant records two things a later reader would otherwise re-derive: that the §13.5 dashboard limb is **suspended, not dropped**, naming its lift condition (the infrastructure pass's registry-versus-panel check) and the fact that mechanic issues owe no panel line until then; and that the four deferred view families are owed by their named issues. |
| AC17 | `docs/DESIGN.md` names the log table `event` at every site that names it, and `docs/DESIGN.md` §11's «Стартовый реестр типов оснований» line lists the game event alongside the types it already lists. Both edits are in Russian, and no other content on the touched lines changes. |
| AC18 | Every live site whose claim this diff falsifies agrees with the tree after the change — membership determined by `AGENTS.md` § *Propagation Rule* step 4's criterion, not by the illustrative list in Scope 7. History surfaces (`ai-docs/learnings.md`, `ai-docs/plans/done/**`) and `ai-docs/deferred/_inbox.jsonl` are left untouched. |
| AC19 | The repository's standard gate set is clean on the branch — build, vet, the whole test suite including the race gate, lint, format and module hygiene — and statement coverage has not fallen past the ratchet's recorded tolerance. |

## Open questions

None blocking design. The four below have defensible defaults; `design-writer` may settle
any of them and the owner may revisit later.

- **`bot_kicked` as a view.** §13.3:425 calls it the terminal metric — «Каждое событие —
  вскрытие» — but the issue's family list names no view for it, and a count over one event
  type is an ad-hoc query. Not among the five; `design-writer` may fold it into the
  notifications view or leave it out.
- **Volume-class member names.** The partition is fixed (Scope 2); the names are not, and
  they become a data contract once shipped, so `design-writer` should choose names that
  describe the retention axis rather than the current two members.
- **`event.ts` versus a caller-supplied occurrence time.** A default of `now()` is the
  obvious start, but a mechanic replaying a scheduled task that fired late may want to
  record when the thing happened rather than when the row was written. A second column is
  a later migration if it ever matters.
- **A JSONB GIN index on the payload.** No view among the five filters on a payload key,
  and an index on a write-heavy append-only table is a cost with no reader. Revisit when a
  deferred family's view actually needs one.
