# Design: Item machine — `item`, `item_movement`, and the shared holder address space

**Issue:** #25
**Date:** 2026-09-12

## Approach

The task ships the **second accounting machine** and welds it to the first. Nothing here is a
mechanic: no stamina, no loot, no death, no player-facing command. What lands is a forward
migration, one write function beside `Post`, and the queries that let an operator ask whether the
two machines still agree.

### A note on evidence

Three tag shapes appear below. `[measured 155bbc8:<path>:<lines> · <cmd> → <output>]` cites this
tree at that commit. `[measured probe · …]` cites a throwaway run against something outside the
tree — a `docker.io/library/postgres:18` container, `golangci-lint` over a scratch package, or a
module-cache read of a dependency's source — and carries **no** repository coordinate, because the
fact is Postgres's, the linter's or goose's, not this repository's. `[derived → …]` names the
acceptance criterion or test that will establish a claim about something this task creates.

### D1 — A holder is a `scope` row, and that is what "one address space" already means

`docs/DESIGN.md` §11 lists the addresses as «игрок-рюкзак, игрок-сундук, труп #123, стройка, Мир»
— *player backpack* and *player chest* are **different** addresses of the **same** owner, so the
shared address cannot be the `owner`. It is the `scope`: an owner's named compartment, pointing at
a `scope_definition` from a seeded catalog
`[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:15-36 · cat -n internal/store/migrations/00001_ledger_core.sql → "CREATE TABLE scope_definition (id smallint PRIMARY KEY, code text NOT NULL UNIQUE, owner_kind owner_kind NOT NULL);" and "CREATE TABLE scope (id …, owner_id … REFERENCES owner (id), scope_definition_id … REFERENCES scope_definition (id), CONSTRAINT scope_singleton_key UNIQUE (owner_id, scope_definition_id))"]`.
KD-17 already reads the scope list that way — «an open list of owner kinds and scopes (attributes,
backpack, inventory, storage, constructions …) that grows by seeding a catalog row»
`[measured 155bbc8:ai-docs/key-decisions.md:47 · grep -n 'KD-17' ai-docs/key-decisions.md → "an open list of owner kinds and scopes (attributes, backpack, inventory, storage, constructions …) that grows by seeding a catalog row, without a per-kind column on owner"]`.

Consequences, and they are the whole of AC6 and AC7:

- A **holder kind** the MVP does not use costs a seeded `scope_definition` row plus its capacity
  `account_definition` rows. No table, no column, no `CHECK` edit, no branch in any Go function
  `[derived → AC7, and the planted-holder-kind test of § Test Design subtask 4]`.
- **Instancing rides on `owner`.** `scope` is a singleton per `(owner_id, scope_definition_id)`
  `[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:34 · cat -n … → "CONSTRAINT scope_singleton_key UNIQUE (owner_id, scope_definition_id)"]`,
  so «труп #123» is not a second corpse-scope on one player — it is its **own owner row** with one
  corpse scope. `owner.id` is a generated identity and the `(kind, telegram_id)` uniqueness is
  partial on `telegram_id IS NOT NULL`
  `[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:6-13 · cat -n … → "CREATE UNIQUE INDEX owner_kind_telegram_id_key ON owner (kind, telegram_id) WHERE telegram_id IS NOT NULL"]`,
  so many anonymous owners of one kind already coexist. #40 therefore adds a corpse by adding an
  `owner_kind` member and a catalog row — never by relaxing `scope_singleton_key`, which is what
  "no restructuring" has to mean here.
- **Rejected: an instance key on `scope`.** A nullable discriminator plus a `NULLS NOT DISTINCT`
  swap of `scope_singleton_key` would also admit many corpses, at the price of altering the shared
  address space's core constraint and of changing what `CreateOwner` auto-creates — for a holder
  this task does not ship. The owner-row route needs neither.

The Go type is `HolderID`, not `ScopeID`: the API is about the role §11 names, and the table it
resolves to is stated in the type's doc comment. Naming it for the table would put the word
`scope` in every item-machine signature and still not say what it addresses.

### D2 — Capacity is a `free`/`used` account pair per holder per kind, and that pair is what makes AC4 and AC5 both true

AC4 requires capacity to be refused by `CHECK (balance >= 0)`, "not by a check written beside it"
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:62 · sed -n '62p' … → "A move that would leave a holder holding more than its capacity admits is refused by the quantitative ledger's CHECK (balance >= 0) path, not by a check written beside it."]`.
A non-negativity floor bounds a holder from **above** only if the balance counts **free**
capacity. AC5 requires the opposite reading — "the instance count at a holder equals that holder's
capacity balance"
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:63 · sed -n '63p' … → "whether the instance count at a holder equals that holder's capacity balance, are each answerable by a query over the shipped data"]`
— which only holds if the balance counts **occupied** capacity. Both are satisfied by giving each
holder, per capacity kind, **two accounts**: `free` and `used`.

The postings a movement of one instance from holder `A` to holder `B` produces, in the `slots`
kind — this is the **movement posting signature** #26's framework will check:

| Account | Amount |
|---|---|
| `A`'s `slots_free` | `+1` |
| `A`'s `slots_used` | `-1` |
| `B`'s `slots_free` | `-1` |
| `B`'s `slots_used` | `+1` |

Each holder's pair sums to zero on its own, so the group is zero-sum per kind by construction and
`Post`'s existing check confirms rather than discovers it
`[measured 155bbc8:internal/store/post.go:131-143 · cat -n internal/store/post.go → phase c, "for _, sum := range sumByKind { if !sum.IsZero() { return ErrUnbalanced } }"]`.
What the pair buys:

- **`free` is the ceiling.** Both accounts of a non-World holder are `controlled`, so both carry an
  `account_balance` row with `CHECK (balance >= 0)`
  `[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:47-51 · cat -n … → "CREATE TABLE account_balance (account_id bigint PRIMARY KEY REFERENCES account (id), balance numeric(30,5) NOT NULL DEFAULT 0, CONSTRAINT account_balance_nonnegative CHECK (balance >= 0));"]`.
  An over-stuffing move drives `B.slots_free` below zero and `Post` maps exactly that constraint to
  `ErrOverdraft`
  `[measured 155bbc8:internal/store/post.go:34-42,168-176 · cat -n internal/store/post.go → "constraintBalanceNonnegative = \"account_balance_nonnegative\"" and "if errors.As(err, &pgErr) && pgErr.Code == sqlstateCheckViolation && pgErr.ConstraintName == constraintBalanceNonnegative { return fmt.Errorf(\"%w: account %d: %w\", ErrOverdraft, id, err) }"]`.
- **`used` is the cross-machine witness.** It is written by the ledger and compared against the
  item machine's own chain-derived count. A reconciliation between a number and itself catches
  nothing; this one compares two independently maintained representations, which is what §11's
  «межмашинная сверка … ловит рассинхрон» asks for.
- **`used`'s floor is a second guard on the chain.** `CHECK (balance >= 0)` on `A.slots_used`
  refuses removing an instance from a holder the ledger says holds none — from the ledger side,
  independently of the item machine's own constraints `[derived → AC2, AC4]`.
- **Capacity is granted, never assumed.** A holder starts at `free = 0` and can hold nothing until
  a document credits it. Granting is an ordinary posting (`B.slots_free +N` against a World slots
  account), which is how a purchased backpack upgrade (§6.5 «Размер рюкзака — прокачиваемый»)
  already wants to work. **This task grants nothing**: no mechanic ships here, so no number ships
  here either (see D8).

**World is the one uncontrolled holder**, exactly as it is for money: its capacity accounts carry
no `account_balance` row and may take any sign, so it is both the counterparty a capacity grant is
drawn against and the origin every instance's chain starts at
`[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:89-94 · cat -n … → the seeded world-scope account definitions carry controlled = false and no account_balance row is seeded]`.
That is a rule about *World*, not about *holder kinds*, so no code branches on kind.

### D3 — Chain continuity is a database invariant, not a code convention

`item_movement` carries `prev_movement_id`, and the declarative objects below make §11's invariant
(«from каждого движения == to предыдущего; экземпляр в один момент ровно у одного держателя»)
unrepresentable to violate:

```sql
CONSTRAINT item_movement_chain_key UNIQUE (id, item_id, to_holder_id),
CONSTRAINT item_movement_chain_fk  FOREIGN KEY (prev_movement_id, item_id, from_holder_id)
    REFERENCES item_movement (id, item_id, to_holder_id),
CONSTRAINT item_movement_genesis_from_world
    CHECK (prev_movement_id IS NOT NULL OR from_holder_id = <the seeded World scope id>),
CONSTRAINT item_movement_holders_differ CHECK (from_holder_id <> to_holder_id)
```

```sql
CREATE UNIQUE INDEX item_movement_successor_key
    ON item_movement (item_id, prev_movement_id, from_holder_id) NULLS NOT DISTINCT;
```

Every one of these was executed against the image the suite itself provisions before being written
down here:

- the DDL is accepted, and a self-referential composite FK under the default `MATCH SIMPLE` leaves
  a genesis row (`prev_movement_id IS NULL`) unconstrained by the chain
  `[measured probe · psql on docker.io/library/postgres:18 (server_version 18.6), the block above with a stand-in holder table in place of scope → "CREATE TABLE", "CREATE INDEX", then a genesis insert "INSERT 0 1"]`;
- a `from` that is not the predecessor's `to` is refused
  `[measured probe · psql on docker.io/library/postgres:18 → "ERROR: insert or update on table \"item_movement\" violates foreign key constraint \"item_movement_chain_fk\" … Key (prev_movement_id, item_id, from_holder_id)=(15, 1, 2) is not present in table \"item_movement\""]`;
- a predecessor belonging to a different instance is refused by the same constraint
  `[measured probe · psql on docker.io/library/postgres:18 → "ERROR: … item_movement_chain_fk … Key (prev_movement_id, item_id, from_holder_id)=(3, 3, 3) is not present"]`;
- a **fork** — a second successor of one movement — is refused
  `[measured probe · psql on docker.io/library/postgres:18 → "ERROR: duplicate key value violates unique constraint \"item_movement_successor_key\" … Key (item_id, prev_movement_id, from_holder_id)=(1, 18, 2) already exists"]`;
- a **second genesis** for one instance is refused, which is the case `NULLS NOT DISTINCT` exists
  for
  `[measured probe · psql on docker.io/library/postgres:18 → "ERROR: duplicate key value violates unique constraint \"item_movement_successor_key\" … Key (item_id, prev_movement_id, from_holder_id)=(4, null, 1) already exists"]`;
- a genesis whose `from` is not the World scope is refused
  `[measured probe · psql on docker.io/library/postgres:18 → "ERROR: new row for relation \"item_movement\" violates check constraint \"item_movement_genesis_from_world\""]`.

**Why the `from_holder_id` column rides in the unique index.** Given the chain FK, a
non-genesis movement's `from` is *determined* by its predecessor's `to`; given the genesis
`CHECK`, a genesis movement's `from` is *determined* to be the World scope. So uniqueness on
`(item_id, prev_movement_id, from_holder_id)` is equivalent to uniqueness on
`(item_id, prev_movement_id)` — and the wider form additionally **covers the composite FK** for
this repository's FK-coverage gate, which a two-column index cannot. One index does both jobs
`[measured probe · psql on docker.io/library/postgres:18, running the gate's own coverage query against the probe schema → with the three-column unique index in place every foreign key of the probe's item_movement reports covered = t; with a two-column form, item_movement_chain_fk reports f]`
`[measured 155bbc8:internal/store/fkcover_test.go:80-102 · cat -n internal/store/fkcover_test.go → the covering rule: an index whose leading columns, as a set, equal the FK's referencing columns, with an absent or "IS NOT NULL" partial predicate]`.

**Why a composite FK at all, when KD-17 says «No composite FKs».** KD-17's reason is stated with
its scope: «No composite FKs and no move-control: such moves are not planned by design, and the
owner judged the declarative guard illusory»
`[measured 155bbc8:ai-docs/key-decisions.md:47 · grep -n 'No composite FKs' ai-docs/key-decisions.md → "No composite FKs and no move-control: such moves are not planned by design, and the owner judged the declarative guard illusory."]`.
Neither half transfers. The guarded event here is not unplanned — a `from` that disagrees with the
chain is the exact failure §11 names as *the* invariant of this machine — and the guard is not
illusory: it refuses the row, as the probe output above shows. The decision is recorded here rather
than silently taken.

**Why the movement points at `journal_entry`, not at its own exclusive arc.** §11 says the movement
carries «документ-основание (тот же exclusive arc)». `journal_entry` **is** that arc: it is 1:1
with its basis document, it holds the one-of-N nullable FK columns and the
`num_nonnulls(...) = 1` `CHECK`, and it is the only holder of the timestamp
`[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:67-76 · cat -n … → "CREATE TABLE journal_entry (id …, ts timestamptz NOT NULL DEFAULT now(), player_operation_id …, manual_correction_id …, CONSTRAINT journal_entry_exactly_one_basis CHECK (num_nonnulls(player_operation_id, manual_correction_id) = 1))"]`
`[measured 155bbc8:internal/store/migrations/00003_event_log.sql:71-77 · sed -n '71,77p' internal/store/migrations/00003_event_log.sql → the arc extension re-stating the CHECK over the arc columns]`.
`item_movement.journal_entry_id NOT NULL` therefore reaches the same arc **through** the entity
that owns it, and buys what a duplicated arc would not: AC3's "the same basis document"
becomes structurally true rather than checked; a new basis type stays one `CHECK` edit, which is
the property §11 calls «grep-абельный реестр»; and a movement in mid-air is as unrepresentable as
a posting in mid-air.

### D4 — `Move` composes `Post`; it does not bypass it

`AGENTS.md`'s ledger AXIOM says every balance change is «a set of postings written by
`store.Post`»
`[measured 155bbc8:AGENTS.md:179 · sed -n '179p' AGENTS.md → "Every change to stamina, resources, money, or items is a set of postings written by store.Post under exactly one basis document"]`.
`Move` honours that literally: the balance-moving half of a move **is** a `Post` call. Since the
movement rows need the `journal_entry` id that `Post` currently discards, `Post`'s body moves into
an unexported `post` returning it, and `Post` becomes the thin wrapper that drops it — no exported
signature changes, no call site moves, and the phase order, capture-order discipline and every
sentinel stay exactly where they are
`[measured 155bbc8:internal/store/post.go:44-193 · cat -n internal/store/post.go → the documented phases a–f, ending "INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, $2, $3)"]`
`[derived → AC3, and the unchanged-`Post`-behaviour assertions of § Test Design subtask 4]`.

The API, in `internal/store` because that is the only package that can insert a basis document —
`PostingBasis`'s methods are unexported, so the sum type is unsatisfiable and uncallable from
outside
`[measured 155bbc8:internal/store/basis.go:12-26 · cat -n internal/store/basis.go → "The unexported methods make it unrepresentable to satisfy from outside the package"]`:

```go
type ItemID int64        // a row in item
const NewItem ItemID = 0 // mint a new instance under this move's own document
type HolderID int64      // a row in scope, read as an address of the shared holder space
const WorldHolder HolderID = 1

type Movement struct {
    ItemID ItemID
    From   HolderID
    To     HolderID
}

func Move(ctx context.Context, tx pgx.Tx, basis PostingBasis, movements ...Movement) ([]ItemID, error)
```

`Move`'s phases, mirroring `Post`'s documented shape so a reader of one can read the other:

- **a. static shape, no SQL** — movements non-empty; no `From == To`; no instance named twice in
  one batch; a `NewItem` movement's `From` is `WorldHolder`. Transaction untouched.
- **b. two `SELECT`s, no writes** — the chain head of every named instance (its id and current
  holder), and the `free`/`used` account of every touched holder in the `slots` kind. Refuses an
  unknown instance, a `From` that is not the current holder (AC2), and a holder with no capacity
  account. Transaction usable, nothing written.
- **c. build the batch** — the postings of D2's table, per movement.
- **d. `post(...)`** — basis document, journal entry, balance `UPDATE`s in capture order, postings.
  `ErrOverdraft` here is AC4's refusal.
- **e. mint** — one `item` row per `NewItem` movement.
- **f. movements** — the `item_movement` rows, in ascending instance id, so two batches touching
  the same instances take the index entries in one order. A chain refusal that only a concurrent
  transaction could produce (`23503` on `item_movement_chain_fk`, or `23505` on
  `item_movement_successor_key`) surfaces as `ErrMoveConflict` with the transaction aborted.

**Why the caller supplies `From` rather than `Move` deriving it.** Deriving would make AC2
inexpressible and would silently convert "loot the corpse" into "move the item from wherever it
now is" — a mechanic must be able to lose the race, not win it by accident.

**Why minting lives inside `Move`.** A craft creates the instance and places it under one document;
splitting that would put an `item` row in the tree with no genesis movement, which is precisely the
anomaly `item_chain_break` reports. `NewItem` keeps "an instance always has a chain that starts at
World" true by construction `[derived → AC1, and the chain-break view's no-movement class]`.

**No panic path is added.** Every refusal is a returned error; the project's zero-production-panic
invariant and its empty index stand
`[measured 155bbc8:ai-docs/panic-index.md:5 · sed -n '5p' ai-docs/panic-index.md → "**The project targets zero production panics and currently holds it** — the table below is empty."]`.

### D5 — `slots` is posted at one per instance; `weight` ships as schema and waits for item weights

AC5 fixes the identity "instance count at a holder == that holder's capacity balance", which is
the *definition* of a slot: one instance, one slot. `Move` therefore owns that amount rather than
taking it from a caller, and the reconciliation is a genuine cross-machine check for every holder
forever, not a convention some future caller can break `[derived → AC5]`.

`weight` cannot be posted here at all: a per-instance weight is a property of an item **definition**,
and definitions are #32's
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:26 · sed -n '26p' … → "Item definitions and their stats — #32 delivers them; this task is the accounting layer under them."]`.
What AC9 asks for is that the **schema admit** both, and it does: both `ledger_kind` members and
both `free`/`used` account definitions on the world and backpack scope definitions ship here. The
kind a holder actually enforces is then decided by what a later mechanic brings — the budget
it grants in that kind (a configuration number) and whether it declares a cost in that kind —
neither of which is a property of the schema. That is the owner's answer, discharged:
«The schema admits both kinds; which one a holder enforces is configuration, so the choice can be
made per mechanic later»
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md.state.md · grep -n 'admits both kinds' … → "Both, config picks: The schema admits both kinds; which one a holder enforces is configuration, so the choice can be made per mechanic later."]`.

Enum members are permanent and cannot be added in the transaction that uses them, so shipping both
now is what avoids a second `ALTER TYPE` later
`[measured probe · psql on docker.io/library/postgres:18, `BEGIN; ALTER TYPE k2 ADD VALUE 'b'; SELECT 'b'::k2;` → "ERROR: 55P04: unsafe use of new value \"b\" of enum type k2", hint "New enum values must be committed before they can be used." — which is why the members and their consumers must sit in different migrations]`.

**How `Move` finds the accounts without branching on holder kind.** `account_definition` gains a
nullable `capacity_role` column (`free` / `used`, `NULL` for the money and experience rows), plus
`UNIQUE (scope_definition_id, kind, capacity_role) WHERE capacity_role IS NOT NULL`. Resolution is
then one uniform query on `(scope_id, kind, capacity_role)` for every holder that exists or ever
will `[measured probe · psql on docker.io/library/postgres:18 → the partial unique index refuses a second (scope_definition_id, kind, capacity_role) row with "duplicate key value violates unique constraint" while two capacity_role-NULL rows coexist]`.
Resolving by parsing the `code` string instead would put a per-holder-kind mapping in Go, which is
the branch AC7 forbids. A holder kind seeded without its `slots` pair is refused at its first move
with a sentinel, not silently mis-accounted.

### D6 — The reconciliation queries ship as views, because that is how this repository answers "answerable by a query"

The migration set already ships operator-readable SQL as views with a pinned column contract
`[measured 155bbc8:internal/store/views_test.go:14-22 · sed -n '14,22p' internal/store/views_test.go → "viewNames is the exact set of views this migration set creates" listing the metric_* views]`.
Three views land, each named for what it answers:

- **`item_holder`** — one row per instance: its current holder and the movement that put it there.
  The read model both reconciliations use, and the answer to "where is this instance". Written as
  an anti-join carrying the redundant-but-true `s.item_id = m.item_id` predicate, so a lookup for a
  known instance uses `item_movement_successor_key` instead of scanning.
- **`item_chain_break`** — AC5's first question. Non-empty exactly when some instance's movements
  are **not** one unbroken path from a World genesis to exactly one head. A recursive walk from
  every genesis is compared, per instance, against the recorded movement count and the head count,
  so a disconnected segment, a missing genesis, an instance with no movement at all, and a fork are
  all one query. This is §11's «Сверка — по цепочкам», literally.
- **`item_capacity_divergence`** — AC5's second question. Non-empty exactly when a controlled
  `slots_used` balance disagrees with the instance count at that holder, or when a holder holding
  instances has no `slots_used` account at all. World is absent from both branches because it has
  no materialised balance, which is the same reason its money balance is not reconciled either.

Sketch of the reconciliations, in the shape probed
`[measured probe · psql on docker.io/library/postgres:18 → the recursive walk below, created and queried against a planted chain: it reports the instance with no movement, and, with the chain constraints dropped inside a rolled-back transaction, an orphaned segment as reached 1 / recorded 2 / heads 2 and a fork as reached 3 / recorded 3 / heads 2]`
`[derived → the view definitions of subtask 1 and the tests of subtask 3]`:

```sql
CREATE VIEW item_holder AS
SELECT m.item_id, m.to_holder_id AS holder_id, m.id AS movement_id
FROM item_movement m
WHERE NOT EXISTS (SELECT 1 FROM item_movement s
                  WHERE s.item_id = m.item_id AND s.prev_movement_id = m.id);

CREATE VIEW item_chain_break AS
WITH RECURSIVE walk AS (
        SELECT m.item_id, m.id, m.to_holder_id FROM item_movement m
        WHERE m.prev_movement_id IS NULL AND m.from_holder_id = <the seeded World scope id>
    UNION ALL
        SELECT n.item_id, n.id, n.to_holder_id
        FROM walk w JOIN item_movement n
          ON n.prev_movement_id = w.id AND n.item_id = w.item_id
         AND n.from_holder_id = w.to_holder_id
)
SELECT i.id AS item_id, reached_movement, recorded_movement, head_movement
FROM item i LEFT JOIN … -- per-instance counts from walk, item_movement and item_holder
WHERE reached_movement <> recorded_movement OR recorded_movement = 0 OR head_movement <> 1;
```

`item_capacity_divergence` joins `item_holder`'s per-holder count against the `account_balance` of
each `slots` / `used` account, and adds a second branch for a holder holding instances with no such
account, each row carrying the reason.

### D7 — The migration, as a forward migration, and its rollback

Forward-only: no `-- +goose Down`, per KD-3
`[measured 155bbc8:ai-docs/key-decisions.md:13 · sed -n '13p' ai-docs/key-decisions.md → "**Forward-only:** no `-- +goose Down` section anywhere — rolling a ledger table back is data loss, so a mistake is corrected by the next forward migration."]`.
The migration is split, and the split is forced rather than chosen: this repository's hygiene gate refuses an
`ADD VALUE` file that also creates, inserts or updates
`[measured 155bbc8:internal/store/migrate_test.go:202-206 · sed -n '202,206p' internal/store/migrate_test.go → "if addValueRe.MatchString(text) { if createTableRe.MatchString(text) || insertRe.MatchString(text) || updateRe.MatchString(text) { t.Errorf(\"%s: an ADD VALUE file must do nothing else (CREATE TABLE/INSERT/UPDATE found)\", entry.Name()) } }"]`,
and Postgres refuses a new enum member's use inside the transaction that added it
`[measured probe · psql on docker.io/library/postgres:18, `BEGIN; ALTER TYPE k2 ADD VALUE 'b'; SELECT 'b'::k2;` → "ERROR: 55P04: unsafe use of new value \"b\" of enum type k2"]`.
goose applies each file in a transaction of its own, so the file that seeds the capacity accounts
sees the members committed
`[measured probe · github.com/pressly/goose/v3@v3.27.3 provider_run.go, read from the module cache → "The default is to apply each migration sequentially on its own", with runIndividually opening one transaction per migration]`.

- `00006_capacity_kinds.sql` — the `slots` and `weight` `ledger_kind` members, and nothing else.
- `00007_item_machine.sql` — the `capacity_role` enum; the `account_definition` column and its
  partial unique index; the `backpack` `scope_definition` row with `owner_kind = 'player'`; the
  `free`/`used` account definitions for the world and backpack scope definitions, `controlled`
  false on the world's and true on the backpack's; the seeded world capacity `account` rows and the
  `setval` that positions the identity sequence past them; `item`; `item_movement` with D3's
  constraints and the indexes the FK-coverage gate requires
  (`item_movement_successor_key`, and one each on `journal_entry_id`, `from_holder_id`,
  `to_holder_id`); and D6's views.

**What happens to rows written before it.** Nothing: every object it touches is either new or
gains a nullable column. `account_definition.capacity_role` is `NULL` on the existing money and
experience rows, with no backfill, so both shapes read identically during the deploy window
`[measured 155bbc8:internal/store/migrations/00001_ledger_core.sql:89-91 · cat -n … → the seeded account_definition rows, which this migration does not rewrite]`.
**Rollback:** none, by KD-3 — a wrong object is corrected by the next forward migration. A `DROP`
of the item tables would be data loss of exactly the kind the carve-out in `AGENTS.md` §
*API Stability* forbids.

**No change to `CreateOwner`.** It already creates every scope whose `scope_definition.owner_kind`
matches, every account of those scopes, and a zero-balance row for each controlled account
`[measured 155bbc8:internal/store/owner.go:58-68,86-97,166-172 · cat -n internal/store/owner.go → "creates … every scope whose scope_definition.owner_kind matches, every account of those scopes, and a zero-balance account_balance row for each controlled account"]`,
so seeding the backpack `scope_definition` is the whole of AC9's "the backpack scope … exists for a
player". The player's account set grows accordingly, which existing assertions pin (§ Risks).

### D8 — This task is not a mechanic, and it ships no tuning value

`docs/DESIGN.md` §13.4's telemetry obligation binds mechanics. This task declares no event and
emits none — the same standing as the ledger core's «Post is not a mechanic»
`[measured 155bbc8:ai-docs/plans/done/2026-09-02-ledger-post-core.spec.md:435 · grep -n 'KD-11' ai-docs/plans/done/2026-09-02-ledger-post-core.spec.md → "KD-11 | Does this task declare events or posting signatures? | **No** — Post is not a mechanic"]`.
What it does fix is the **movement posting signature** of D2's table, which every document that
moves an instance will carry and which #26's framework will check — #26 being out of scope by the
spec's own line
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:31 · sed -n '31p' … → "The framework that checks a basis document's declared posting and movement sets against what it actually wrote — #26 delivers it."]`.

No balance number ships. A backpack's slot budget is a tuning value (§6.5 lists backpack slots as a
money sink) and belongs in `config/balance.yaml` — but its reader is the mechanic that grants
capacity, and this configuration loader pairs every key with a schema entry and fails start-up when
only one side has it
`[measured 155bbc8:config/balance.yaml:1-10 · head -10 config/balance.yaml → "Every key below is required by the configuration loader's schema; adding a key to only one side (this file or the schema) fails start-up."]`.
Shipping an unread key would therefore be a live, load-bearing entry nothing consults. The key
lands with its reader.

### D9 — The propagation class

Scope 6 obliges every live document whose claim this migration falsifies to state what shipped
instead, and says any site known at drafting illustrates the class rather than bounding it
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:23 · sed -n '23p' … → "the class is every site that describes the item machine's tables or its holder address space, per AGENTS.md § Propagation Rule step 4, and any site known at drafting illustrates the class rather than bounding it"]`.
The class is: **every live site naming the item machine's tables, or describing its holder address
space, that this migration makes false.** The sweep is re-derived at implementation time over the
live surfaces — `AGENTS.md`, `.claude/**`, `ai-docs/**` (excluding the history surfaces
`ai-docs/learnings.md` and `ai-docs/plans/done/**`), `docs/**` and any repo-root user-facing doc —
with a case-insensitive pattern covering both the plural and singular spellings and the phrase
*item machine*, per `AGENTS.md` § *Propagation Rule* step 1's `-i` requirement. Sites known at
drafting, as illustration only:

| Site | What it now says | Why the diff falsifies it |
|---|---|---|
| `AGENTS.md` § *API Stability* carve-out | «the append-only `posting` / `item_movements` ledgers» `[measured 155bbc8:AGENTS.md:153 · sed -n '153p' AGENTS.md → "The Postgres schema, the append-only `posting` / `item_movements` ledgers, the basis-document tables…"]` | the table is `item_movement` |
| `AGENTS.md` § *Domain Rules* AXIOM | «Item instances move through `item_movements`» `[measured 155bbc8:AGENTS.md:179 · sed -n '179p' AGENTS.md]` | same |
| `ai-docs/domain-invariants.md` § 2 | «They live in `items` + append-only `item_movements`» `[measured 155bbc8:ai-docs/domain-invariants.md:24 · sed -n '24p' ai-docs/domain-invariants.md]` | both names, and the section can now name the holder as a `scope` and the capacity pair |
| `.claude/skills/task/reference.md` § domain sweep | `rg -n 'INSERT INTO (postings\|item_movements)\|store\.Post\('` `[measured 155bbc8:.claude/skills/task/reference.md:213 · sed -n '213p' .claude/skills/task/reference.md]` | **both** alternatives are wrong — the tables are `posting` and `item_movement` — and the sweep should also catch `store.Move(` |
| `docs/DESIGN.md` § 11 | «Таблица `items` … + append-only `item_movements`» | the spec's § *Source conflicts* resolves the naming to the singular, agreeing with the same section's own «имена таблиц — в единственном числе» decision |

The `docs/DESIGN.md` edit is a **name sync inside the Russian corpus, not a redesign**: it changes
the item-machine bullet's table names to the ones the same section's naming decision already
requires, and the spec's
§ *Source conflicts* is the authority that chose them
`[measured 155bbc8:ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md:44-54 · sed -n '44,54p' … → "Resolution: the singular. Chosen by the task text itself, whose Scope line names `item` and `item_movement`, and which agrees with the naming decision of the same section"]`.
It is flagged in § Open questions all the same, because editing the decision corpus is the owner's
call even when the edit only removes a contradiction.

The `AGENTS.md` and `.claude/**` edits are legal in this group: Learning-Log Boundary rule 2's
`PreToolUse` guard blocks them only while a spec state file exists **without**
`ai-docs/plans/.task-inflight`, and Steps 8–12 carry that marker
`[measured 155bbc8:AGENTS.md:373 · grep -n 'Machine-enforced while an interview is live' AGENTS.md → "blocks any `Edit`/`Write` to the files this rule names … while an `ai-docs/plans/*.spec.md.state.md` exists on the branch and `ai-docs/plans/.task-inflight` does not … Steps 8–12 carry the marker and are exempt"]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | The forward migration (D7) and the Go mirrors that must move with it, in one commit because neither is green alone. `00006` carries the capacity `ledger_kind` members and nothing else; `00007` carries the `capacity_role` enum, the `account_definition` column and its partial unique index, the `backpack` scope definition, the capacity account definitions for the world and backpack scope definitions, the seeded world capacity accounts and their `setval`, `item`, `item_movement` with D3's constraints and covering indexes, and D6's views. Alongside: the enum and catalog mirrors, the `ItemID` / `NewItem` / `HolderID` / `WorldHolder` identity declarations, and the existing assertions the schema change moves — the table set, enum members, seed counts, identity sequences, goose row count and index list in `migrate_test.go`, the view-name set and column contract in `views_test.go`, the player's account set in `owner_test.go`, and the totality `exhaustive` forces on `post_test.go`'s `Kind` switches (§ Risks). Serves Scope 1, AC1, AC5, AC6, AC7, AC9, AC10. | `internal/store/migrations/00006_capacity_kinds.sql`, `internal/store/migrations/00007_item_machine.sql`, `internal/store/enums.go`, `internal/store/catalog.go`, `internal/store/ids.go`, `internal/store/migrate_test.go`, `internal/store/views_test.go`, `internal/store/owner_test.go`, `internal/store/post_test.go` | — |
| 2 | The schema's own refusals, asserted by SQLSTATE and constraint name in the existing `schema_test.go` idiom: same-holder move, genesis not from World, a `from` disagreeing with the predecessor, a predecessor of another instance, a fork, a second genesis, a duplicate `(scope_definition, kind, capacity_role)`, and an explicit id into `item`'s `GENERATED ALWAYS` identity. Additive; green on top of subtask 1. Serves AC1, AC2, AC8, AC10. | `internal/store/schema_test.go` | 1 |
| 3 | The reconciliation views' tests: both views empty on a healthy tree, `item_holder` naming the right holder after a hand-built chain, and each anomaly class of `item_chain_break` and `item_capacity_divergence` planted and seen reported — the plant made inside a transaction that drops the chain constraints and is rolled back, so the instrument is shown going red before its green is believed. Serves AC5. | `internal/store/item_views_test.go` | 1 |
| 4 | `Move` (D4): extract `Post`'s body into the unexported `post` returning the journal-entry id, add `Movement`, `Move`, the sentinels, and the package comment's second write path. Tests first: the happy path and the postings it writes, each sentinel with the transaction state its doc comment claims, the capacity `CHECK` path returning `ErrOverdraft`, the same-document/same-transaction property (an induced failure at the movement insert leaves no postings), a mint, and the planted-holder-kind case that shows a holder the MVP does not use needs no code. Serves Scope 2, Scope 5, AC2, AC3, AC4, AC6, AC7. | `internal/store/move.go`, `internal/store/errors.go`, `internal/store/post.go`, `internal/store/store.go`, `internal/store/move_test.go` | 1 |
| 5 | The tests that need a shape of their own: a `rapid` property test driving random move sequences against a Go model of holder-per-instance and per-holder occupancy, asserting `item_holder`, both reconciliation views and both capacity balances after every accepted move and no write after every rejected one; and a `-race` concurrency test in which two transactions move one instance at once, asserting exactly one commit, `ErrMoveConflict` for the loser, and a continuous chain afterwards. Serves AC2, AC8. | `internal/store/move_property_test.go`, `internal/store/move_race_test.go` | 4 |
| 6 | Sweep every live surface for a claim this diff falsifies and fix each (D9). The class is every live site naming the item machine's tables or describing its holder address space, re-derived by a case-insensitive sweep at implementation time — not the illustrative list in D9. Serves Scope 6. | `AGENTS.md`, `ai-docs/domain-invariants.md`, `ai-docs/context.md`, `.claude/skills/task/reference.md`, `docs/DESIGN.md`, plus whatever the sweep finds | 1–5 |

## Handoff plan

Grouping is required for every `M ≥ 1`, and this design's `M` is the Decomposition table's subtask
count; the contract's sub-points (a)–(h) are applied below. Every group is homogeneous by
change-type, marked with its implementor model and effort, and the group count is the minimum the
change-type split and the dependency order allow.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The first group gets a handoff exactly as every later one does.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–5 (code change-type: `*.go` and the migrations). Every same-change-type
  subtask is clustered into this one group rather than interleaved with the prose work; the group
  is within the `≤ 10` size cap, and the dependency order inside it (2→1, 3→1, 4→1, 5→4) is
  respected by the numbering.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`
  override, 1M-token window — subtask 6 (instructions/harness change-type: `AGENTS.md`,
  `.claude/**`, `ai-docs/**`, `docs/**`). Terminal group, sized 1, within the `1..=10` range.

Group-count check: the change-type switch between subtask 5 and subtask 6 forces the boundary, and
subtask 6 depends on the whole code group, so no reordering collapses the two groups into one. The
total is within the default maximum of 4 design-defined groups, so no user approval is needed.

## Risks

- **Adding `Kind` members breaks the existing `Kind` switches, and the failure is a lint gate, not
  a compile error.** `exhaustive` is enabled and `_test.go` is not among its exclusions
  `[measured 155bbc8:.golangci.yml:20,70-83 · cat -n .golangci.yml → "- exhaustive # every FSM state / enum switch is total", and the `_test\.go` exclusion listing goconst, gosec, unparam, forbidigo only]`.
  The `Kind` switches in `createPlayer` and in `fund` carry no default
  `[measured 155bbc8:internal/store/post_test.go:43,58 · sed -n '43p;58p' internal/store/post_test.go → "switch a.Kind {" and "switch kind {", each followed only by the KindMoney and KindExperience cases]`.
  Mitigation: subtask 1 gives each the case or the default it needs, in the same commit as the
  mirrors. Reproduced outside the tree before being asserted here
  `[measured probe · golangci-lint run --enable-only=exhaustive over a scratch package with a four-member Kind and a two-case switch → "missing cases in switch of type exhprobe.Kind: exhprobe.KindSlots, exhprobe.KindWeight (exhaustive)" in both the non-test and the _test.go file]`.
- **Several existing assertions pin the pre-change schema exactly and will fail as a set.** The
  table list, the enum member sets, the seed counts, the identity-sequence positions and the goose
  row count
  `[measured 155bbc8:internal/store/migrate_test.go:38-49,54-60,86-90,113-122,138-142 · cat -n internal/store/migrate_test.go → the base-table want list, the enum map, "want 1/1/2/0", the {owner,2},{scope,2},{account,3} sequence table, and "if count != 6 { … want 6 }"]`;
  the exact view set
  `[measured 155bbc8:internal/store/views_test.go:16-22,47-51 · sed -n '16,22p;47,51p' internal/store/views_test.go → viewNames and "want exactly %v (AC11, AC13)"]`;
  and the player's account-set assertion
  `[measured 155bbc8:internal/store/owner_test.go:52-53 · sed -n '52,53p' internal/store/owner_test.go → "if len(owner.Accounts) != 2 { t.Fatalf(\"player accounts = %d, want 2\", …) }"]`.
  Mitigation: subtask 1 carries all of them; the gate is green at its commit or the subtask is not
  done. Their failing is the designed signal, not an accident.
- **Capacity postings will appear in `metric_faucet_sink`, and a mint reads as an equal faucet and
  sink.** The view is deliberately generic over `ledger_kind` and reads every posting touching a
  World-owned account
  `[measured 155bbc8:internal/store/migrations/00003_event_log.sql:228-259 · grep -n -A 30 'metric_faucet_sink' internal/store/migrations/00003_event_log.sql → "generic over ledger_kind membership: it groups by account_definition.kind, not by any named enum member, so a new kind appears as soon as postings of it exist against a World-owned account"]`.
  A genesis touches both of World's `slots` accounts with opposite signs, so it contributes one to
  faucet and one to sink with a net of zero; a capacity grant contributes a real net faucet. This
  is the view's designed genericity, not a defect, and it is recorded here so it is not "fixed"
  `[derived → the faucet/sink expectations of § Test Design subtask 4]`.
- **A holder kind seeded without its `slots` pair fails at its first move, not at migration time.**
  Nothing declarative marks a scope definition as a holder, so the guarantee is a catalog
  convention plus a clear sentinel. Mitigation: `Move` refuses with a named sentinel rather than
  mis-accounting, and the planted-holder-kind test exercises the seeded-correctly path so a
  mechanic author has a worked example `[derived → AC7, subtask 4]`.
- **The coverage ratchet may block the first commit that adds `move.go` before its tests.** The
  ratchet refuses a drop past 0.60 pp and is measured on the working tree
  `[measured 155bbc8:AGENTS.md:111,123 · grep -n 'Unstaged edits to such files\|tolerance is' AGENTS.md → "| Unstaged edits to such files | **Blocked.** The measurement is taken on the working tree…" and "The tolerance is **0.60 pp**"]`.
  Mitigation: subtask 4 is written test-first and commits code and tests together, which is the
  project's standing TDD rule anyway.
- **`go build ./...` truncates its per-package error list**, so a first green build after the
  migration is not proof that the enumerated fixes were all of them
  `[measured probe · go build ./... over a scratch package with fifteen undefined-symbol errors → ten "undefined:" lines followed by "./a.go:12:25: too many errors"]`.
  Mitigation: subtask 1 re-runs
  the full gate set after the enumerated assertions clear, and any newly revealed out-of-contract
  class is surfaced to the orchestrator rather than absorbed.
- **The composite chain FK is written against KD-17's «No composite FKs» line.** Mitigation: D3
  states the decision, quotes KD-17's own reason, and shows why neither half of that reason
  transfers. If the owner reads KD-17 as an unconditional ban, the fallback is the same design with
  the chain check in `Move` only — strictly weaker, and § Open questions says so.
- **Editing `docs/DESIGN.md` touches the decision corpus.** Mitigation: the change is the table
  names in §11's item-machine bullet, chosen by the spec's § *Source conflicts*, and it removes a
  contradiction with the same
  section's own naming decision rather than introducing one; § Open questions surfaces it for the
  owner regardless.

## Test Design

Every test below is a Postgres test against Postgres, in `package store`, on the schema-scoped
migrated pool the package's helpers already provide — `newStore` for the plain case and
`newStoreWithRecorder` when a "nothing was written" claim needs the statement log
`[measured 155bbc8:internal/store/store_test.go:19-64 · cat -n internal/store/store_test.go → newStore and newStoreWithRecorder, each building a testdb.Schema pool and running Migrate]`.
The package's `TestMain` already satisfies the goroutine-leak rule and does not change
`[measured 155bbc8:internal/store/store_test.go:15-17 · sed -n '15,17p' internal/store/store_test.go → "os.Exit(leaktest.Main(m, testdb.Main))"]`.
`pgregory.net/rapid` is already a direct requirement, so the property test adds no dependency
`[measured 155bbc8:go.mod · go list -m pgregory.net/rapid → "pgregory.net/rapid v1.3.0"]`.

**Subtask 1 — schema shape.** Extends the existing shape assertions rather than adding new tests:
the base-table set gains the item machine's tables, the enum map gains `ledger_kind`'s new members
and the `capacity_role` enum, the seed counts and identity sequences move with the seeded world
capacity accounts, the goose row count moves with the added files, the index list gains
`item_movement`'s, the `CHECK`-definition map gains the genesis and holders-differ constraints, and
the view-name set gains D6's views with their column contract pinned. `TestEnums_mirror_database`
and `TestCatalog_mirrors_database` need no edit — they compare the Go mirrors against the database
and will fail on either side moving alone, which is the property being relied on
`[measured 155bbc8:internal/store/enums_test.go:10-45,47-121 · cat -n internal/store/enums_test.go → the enum_range comparison and the element-for-element catalog comparison]`
`[derived → AC9, AC10]`.

**Subtask 2 — `internal/store/schema_test.go`.** Entry point: raw `INSERT`s inside a transaction
rolled back by the file's existing `rollback` helper, asserted with its existing `sqlstate` helper
naming both the SQLSTATE and the constraint
`[measured 155bbc8:internal/store/schema_test.go:12-35 · cat -n internal/store/schema_test.go → sqlstate and rollback]`.
Scenarios, each a subtest: a movement whose holders coincide; a genesis whose `from` is not the
World scope; a successor whose `from` is not the predecessor's `to`; a successor naming a
predecessor of a different instance; a second successor of one movement; a second genesis for one
instance; a second `account_definition` row for one `(scope_definition, kind, capacity_role)`; and
an explicit id into `item`. Fixtures: none beyond the seeded world scope and one player created by
the existing `createPlayer` helper
`[measured 155bbc8:internal/store/post_test.go:33-51 · grep -n -A 22 'func createPlayer' internal/store/post_test.go → the helper creating a player owner inside tx and returning its account ids]`
`[derived → AC1, AC2, AC8, AC10]`.

**Subtask 3 — `internal/store/item_views_test.go`.** Entry points: the views of D6.

- *Healthy is empty, and the emptiness is not vacuous.* Both reconciliation views return no rows on
  a tree with instances actually moved through `Move`; the test first asserts `item_holder` is
  non-empty, so an empty reconciliation cannot be an empty corpus reporting clean.
- *The instrument goes red.* Inside a transaction that drops `item_movement_chain_fk` and
  `item_movement_successor_key` and is then rolled back, plant each anomaly separately and assert
  the class is reported: an instance with no movement; an orphaned segment (a successor whose
  predecessor belongs to another instance); a fork; a genesis whose `from` is not World. Each
  planted shape was executed against the image before being specified
  `[measured probe · psql on docker.io/library/postgres:18 → with the chain constraints dropped inside a rolled-back transaction, the view reports the no-movement instance, the orphan as reached 1 / recorded 2 / heads 2, and the fork as reached 3 / recorded 3 / heads 2]`.
- *Capacity divergence.* Plant a divergence by posting a `slots_used` leg through `Post` with no
  matching movement, and assert the holder appears with both numbers and the `count_mismatch`
  reason; plant a holder holding instances whose scope definition has no `slots_used` account and
  assert the second reason. Assert World is in neither branch even while it is the `from` of every
  genesis — and show that its absence is the missing materialised balance rather than a dead
  branch, by planting a holder kind whose capacity definitions are uncontrolled (absent, like
  World) beside one whose are controlled (reported), in the same rolled-back transaction.
- *`item_holder`.* After a chain of moves, the view names the last destination and the movement that
  put the instance there; after a move back, it follows.

Fixtures: a helper that creates a player, grants its backpack a slot budget through a
`ManualCorrection` posting, and mints instances into it `[derived → AC5]`.

**Subtask 4 — `internal/store/move_test.go`.** Entry point: `Move`.

- *Happy path.* A mint into a granted backpack, then a move to a second holder. Assert the
  `item_movement` rows (instance, holders, predecessor, journal entry), that the journal entry is
  the one the basis document produced, and that the postings are exactly D2's table for each
  movement — read back from `posting`, compared as a set of `(account, amount)` pairs, so an extra
  or missing leg fails.
- *Every sentinel, with the transaction state its doc comment claims.* Empty movements; `From ==
  To`; one instance named twice; a `NewItem` whose `From` is not `WorldHolder`; an unknown
  instance; a `From` that is not the current holder; a holder with no capacity account; a nil and a
  typed-nil basis. For each pre-write sentinel, `newStoreWithRecorder` plus `rec.Reset()`
  immediately before the call asserts no `INSERT`/`UPDATE`/`DELETE` was issued and the transaction
  is still usable — the recorder discipline the `Post` property test already uses
  `[measured 155bbc8:internal/store/post_property_test.go:151-168 · sed -n '151,168p' internal/store/post_property_test.go → rec.Reset() before the rejected Post and the assertion that only the phase-b SELECT was recorded]`.
- *AC4, the capacity path.* Grant a backpack a budget, fill it exactly, then attempt one more move
  in. Assert `errors.Is(err, ErrOverdraft)`, that the failing account is the destination's
  `slots_free`, and that after rollback neither a posting nor a movement survives. A control in the
  same test moves the same instance into a holder with headroom and succeeds, so the refusal is
  shown to be the budget and not the fixture.
- *AC3, one document or neither.* Induce a failure at the movement insert (a concurrent transaction
  that takes the successor index entry first) and assert that, after the required rollback, the
  basis document, the journal entry and the postings are all absent — neither machine is durable
  without the other.
- *AC6 and AC7, a holder kind the MVP does not use.* Inside a rolled-back transaction, plant a
  `scope_definition` row for a non-MVP holder kind together with its capacity account definitions,
  create two owners so the kind has two distinct instances, grant each holder a budget, and move an
  instance backpack → planted holder → the other planted holder. Assert success through the same
  `Move` call with no kind-specific argument. The planting-inside-a-rolled-back-transaction
  technique is this repository's own
  `[measured 155bbc8:internal/store/fkcover_ac17_test.go:18-44 · cat -n internal/store/fkcover_ac17_test.go → a table planted inside a transaction, asserted against, then rolled back]`.
- *`Post` is unchanged.* The existing `Post` tests are the assertion; the extraction is a refactor
  and any behaviour change fails them.

`[derived → Scope 2, Scope 5, AC2, AC3, AC4, AC6, AC7]`

**Subtask 5 — property and race.**

- *`internal/store/move_property_test.go`.* `rapid.Check` draws a sequence of moves over a small
  fixed set of holders and instances, each step choosing an instance and a destination. A Go model
  tracks the holder of every instance and the occupancy of every holder. After each accepted move,
  assert `item_holder` equals the model, `item_chain_break` and `item_capacity_divergence` are
  empty, and each holder's `slots_used` and `slots_free` balances equal the model's occupancy and
  its budget minus that occupancy. Each step also draws a **deliberately wrong** `From` and asserts
  `ErrNotCurrentHolder` with no statement written, so the rejection path is exercised on every
  draw. Shape and banding follow the existing property test
  `[measured 155bbc8:internal/store/post_property_test.go:37-49 · sed -n '37,49p' internal/store/post_property_test.go → rapid.Check over a transaction begun per draw and rolled back]`.
- *`internal/store/move_race_test.go`.* Two goroutines, each on its own transaction from a pool
  sized for them, move one instance from its current holder to two different destinations, then
  commit. Assert exactly one commit succeeds, the loser's error is `ErrMoveConflict`, `item_holder`
  names the winner's destination, and `item_chain_break` is empty. Run under `-race`; the shape
  follows the anti-deadlock test
  `[measured 155bbc8:internal/store/post_race_test.go:68-80 · sed -n '68,80p' internal/store/post_race_test.go → testdb.Schema with cfg.MaxConns set to the worker count]`.
  The behaviour being asserted was executed before being specified: the loser blocks on the
  uncommitted index entry and is then refused
  `[measured probe · psql on docker.io/library/postgres:18, two sessions inserting the same successor while the first holds its transaction open → the second waited 3756 ms and then "ERROR: duplicate key value violates unique constraint \"item_movement_successor_key\" … Key (item_id, prev_movement_id)=(1, 13) already exists"; the index carried its two-column form at that point in the probe session, and the refusals under the shipped three-column form are the ones cited in D3]`.
  A repeat count large enough that an all-green run is not one Bernoulli trial, with the failing
  goroutine's identity reported rather than only the count.

**Subtask 6.** No test. Its gate is `make comment-refs`, the markdown link check, and the
re-derived sweep coming back empty on a pattern shown to match a constructed positive first.

## Open questions

- **`SPEC-REMIT`: several spec rows name a mechanism rather than only an outcome** — Scope 3 and
  AC4 name the `CHECK (balance >= 0)` path; Scope 1 and AC1 name the tables and their columns;
  Scope 4 and AC5 name "a query". Each carries a `[task: …]` tag, i.e. each is quoted from the
  issue body — the owner's own text, not spec-writer origination — and each is also the design this
  document would have chosen on its merits, so each is adopted rather than worked around. The
  outcomes they protect are, respectively: over-stuffing unrepresentable rather than validated; the
  singular table names of the §11 naming decision; and reconciliation an operator can run without
  new code. Raised because a design that silently obeys a mechanism-naming row is indistinguishable
  from one that never noticed.
- **Should `docs/DESIGN.md` §11's plural table names be corrected in this PR?** The spec's
  § *Source conflicts* settles the *names*; what is the owner's is whether the decision corpus is
  edited here at all. The design proceeds with the edit (D9) because leaving it would leave a live
  document contradicting both the shipped schema and its own naming decision. Answer "no" and the
  only change is that subtask 6 leaves `docs/**` alone; nothing else in the design moves.
- **Is KD-17's «No composite FKs» meant unconditionally?** D3 reads it as scoped to the guard it
  was written about, and takes the composite chain FK on the strength of the probe output. If the
  owner reads it as a blanket rule, the chain check falls back to `Move` alone plus the successor
  unique index — the same behaviour for every write that goes through `Move`, and no guarantee at
  all for one that does not.
- **One instance, one slot — for how long?** AC5's identity fixes it, and D5 builds on it. A future
  item definition that wants a two-slot greatsword (#32's territory) would break the identity, and
  the reconciliation would have to compare against a summed per-definition cost instead. Not
  blocking, and named now because the cheapest moment to say "slots are per instance, weight is per
  item" is before #32 assumes otherwise.
- **The player's storage scope and its chat binding.** Carried forward from the spec unchanged: it
  waits on #30 closing §16.7, and this task creates the backpack scope alone, so neither answer is
  foreclosed. No design decision here depends on it.
