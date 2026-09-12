-- +goose Up
-- capacity_role names which half of a capacity kind's free/used pair an
-- account_definition row represents. NULL (the money and experience rows)
-- means "not a capacity account at all".
CREATE TYPE capacity_role AS ENUM ('free', 'used');

ALTER TABLE account_definition ADD COLUMN capacity_role capacity_role;
-- Single-valued resolution of "the free/used account of kind K at scope
-- definition S" for every holder kind that exists or ever will, without a
-- branch on kind in Go. The NULL-role money/experience rows are
-- deliberately left free to coexist by the partial predicate.
CREATE UNIQUE INDEX account_definition_capacity_role_key
    ON account_definition (scope_definition_id, kind, capacity_role)
    WHERE capacity_role IS NOT NULL;

-- backpack is the first holder kind beyond World and a player's own
-- attributes scope: a shared address, instanced by owner row, that a
-- corpse or a construction will later share the schema with by adding an
-- owner_kind member and a catalog row here — never by restructuring this
-- table or scope_singleton_key.
INSERT INTO scope_definition (id, code, owner_kind) VALUES (3, 'backpack', 'player');

-- The free/used account definitions for the world scope (uncontrolled —
-- capacity there admits any sign, exactly like its money and experience
-- accounts) and the backpack scope (controlled — CHECK (balance >= 0)
-- bounds it). Both ledger kinds ship now so a later mechanic that wants
-- weight needs no second ALTER TYPE; slots is the only kind Move posts to
-- until a future item definition declares a weight cost.
INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled, capacity_role) VALUES
    (5,  1, 'slots_free',  'slots',  false, 'free'),
    (6,  1, 'slots_used',  'slots',  false, 'used'),
    (7,  1, 'weight_free', 'weight', false, 'free'),
    (8,  1, 'weight_used', 'weight', false, 'used'),
    (9,  3, 'slots_free',  'slots',  true,  'free'),
    (10, 3, 'slots_used',  'slots',  true,  'used'),
    (11, 3, 'weight_free', 'weight', true,  'free'),
    (12, 3, 'weight_used', 'weight', true,  'used');

-- The world capacity accounts. Identity-assigned ids, with no setval:
-- Move resolves every holder's capacity accounts, World's included, by
-- (scope_id, kind, capacity_role) rather than by a Go constant, so no
-- constant needs a fixed id here — and an explicit id would additionally
-- assume no account row past the seeded pair exists, a premise about the
-- database's state rather than about this migration.
INSERT INTO account (scope_id, account_definition_id) VALUES
    (1, 5), (1, 6), (1, 7), (1, 8);

-- Backfill: a scope_definition seeded here creates scopes only for owners
-- created after it (CreateOwner runs at owner-creation time, not as a
-- migration-time sweep). No production path creates an owner today, so
-- every statement below is a no-op on every database this migration meets
-- — but the day a handler calls CreateOwner, a pre-existing player must
-- still end up with a backpack scope, its capacity accounts and their
-- balance rows. Every later scope-definition migration owes the same three
-- statements. ON CONFLICT DO NOTHING against each singleton constraint
-- makes re-running, and running against an empty owner set, both no-ops.
INSERT INTO scope (owner_id, scope_definition_id)
SELECT o.id, 3 FROM owner o WHERE o.kind = 'player'
ON CONFLICT (owner_id, scope_definition_id) DO NOTHING;

INSERT INTO account (scope_id, account_definition_id)
SELECT s.id, d.id
FROM scope s
JOIN account_definition d ON d.scope_definition_id = s.scope_definition_id
WHERE s.scope_definition_id = 3
ON CONFLICT (scope_id, account_definition_id) DO NOTHING;

INSERT INTO account_balance (account_id)
SELECT a.id
FROM account a
JOIN account_definition d ON d.id = a.account_definition_id
WHERE d.scope_definition_id = 3 AND d.controlled
ON CONFLICT (account_id) DO NOTHING;

-- item is an identity table like owner and scope: an instance's identity,
-- and nothing else yet. A later forward migration adds a definition
-- column; it carries nothing mutable today, which is why the append-only
-- guard does not cover it the way it covers item_movement below.
CREATE TABLE item (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY
);

-- item_movement is the append-only chain that makes the shared address
-- space's invariant ("from of every movement == to of its predecessor; an
-- instance is at exactly one holder at a time") a database property
-- rather than a code convention:
--   - item_movement_chain_key + item_movement_chain_fk: a self-referential
--     composite FK — a successor's (prev_movement_id, item_id,
--     from_holder_id) must reference a row whose (id, item_id,
--     to_holder_id) match, so a from that disagrees with the predecessor's
--     to, or that names a predecessor of a different instance, is refused
--     at the database.
--   - item_movement_genesis_from_world: under MATCH SIMPLE a genesis row
--     (prev_movement_id IS NULL) is unconstrained by the chain FK, so this
--     CHECK is what pins a genesis's from to the seeded World scope (id 1).
--   - item_movement_holders_differ: a movement that does not move anything
--     is unrepresentable.
--   - item_movement_successor_key: NULLS NOT DISTINCT so at most one
--     successor exists per (item, predecessor, from) — refusing both a
--     fork (a second successor of one movement) and a second genesis for
--     one instance (two NULL-predecessor rows for the same item) — and,
--     because a non-genesis row's from is *determined* by its
--     predecessor's to and a genesis row's from is *determined* to be
--     World, this index's three columns also cover the composite FK's
--     three referencing columns as a set, which a narrower two-column
--     index could not.
CREATE TABLE item_movement (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id          bigint NOT NULL REFERENCES item (id),
    prev_movement_id bigint,
    from_holder_id   bigint NOT NULL REFERENCES scope (id),
    to_holder_id     bigint NOT NULL REFERENCES scope (id),
    journal_entry_id bigint NOT NULL REFERENCES journal_entry (id),
    CONSTRAINT item_movement_chain_key UNIQUE (id, item_id, to_holder_id),
    CONSTRAINT item_movement_chain_fk  FOREIGN KEY (prev_movement_id, item_id, from_holder_id)
        REFERENCES item_movement (id, item_id, to_holder_id),
    CONSTRAINT item_movement_genesis_from_world
        CHECK (prev_movement_id IS NOT NULL OR from_holder_id = 1),
    CONSTRAINT item_movement_holders_differ CHECK (from_holder_id <> to_holder_id)
);
CREATE UNIQUE INDEX item_movement_successor_key
    ON item_movement (item_id, prev_movement_id, from_holder_id) NULLS NOT DISTINCT;
-- Covers the journal_entry_id, from_holder_id and to_holder_id FKs; the
-- item_id and the composite chain FK are covered by the successor index
-- above (item_id is its leading column, and the other two together with
-- item_id are exactly the chain FK's referencing columns as a set).
CREATE INDEX item_movement_journal_entry_idx ON item_movement (journal_entry_id);
CREATE INDEX item_movement_from_holder_idx   ON item_movement (from_holder_id);
CREATE INDEX item_movement_to_holder_idx     ON item_movement (to_holder_id);

-- item_holder: one row per instance still short of a successor — its
-- current holder and the movement that put it there. The
-- s.item_id = m.item_id predicate is redundant given the chain FK but lets
-- a lookup for a known instance use item_movement_successor_key instead of
-- scanning.
CREATE VIEW item_holder AS
SELECT m.item_id, m.to_holder_id AS holder_id, m.id AS movement_id
FROM item_movement m
WHERE NOT EXISTS (
    SELECT 1 FROM item_movement s
    WHERE s.item_id = m.item_id AND s.prev_movement_id = m.id
);

-- item_chain_break: non-empty exactly when some instance's movements are
-- not one unbroken path from a World genesis to exactly one head. The walk
-- starts only at a World genesis, so an instance whose only movement is an
-- off-World genesis contributes
-- no row to walk at all — driving the outer join from item, with every
-- count COALESCEd, is what keeps that instance visible instead of being
-- dropped by a NULL-blind comparison.
CREATE VIEW item_chain_break AS
WITH RECURSIVE walk AS (
        SELECT m.item_id, m.id, m.to_holder_id
        FROM item_movement m
        WHERE m.prev_movement_id IS NULL AND m.from_holder_id = 1
    UNION ALL
        SELECT n.item_id, n.id, n.to_holder_id
        FROM walk w
        JOIN item_movement n
          ON n.prev_movement_id = w.id AND n.item_id = w.item_id AND n.from_holder_id = w.to_holder_id
),
reached  AS (SELECT item_id, count(*) AS n FROM walk          GROUP BY item_id),
recorded AS (SELECT item_id, count(*) AS n FROM item_movement GROUP BY item_id),
heads    AS (SELECT item_id, count(*) AS n FROM item_holder   GROUP BY item_id)
SELECT
    i.id                    AS item_id,
    COALESCE(reached.n,  0) AS reached_movement,
    COALESCE(recorded.n, 0) AS recorded_movement,
    COALESCE(heads.n,    0) AS head_movement
FROM item i
LEFT JOIN reached  ON reached.item_id  = i.id
LEFT JOIN recorded ON recorded.item_id = i.id
LEFT JOIN heads    ON heads.item_id    = i.id
WHERE COALESCE(reached.n, 0) <> COALESCE(recorded.n, 0)
   OR COALESCE(recorded.n, 0) = 0
   OR COALESCE(heads.n,    0) <> 1;

-- item_capacity_divergence: non-empty exactly when a controlled slots/used
-- balance disagrees with the instance count at that holder, or when a
-- holder holding instances has no slots/used account_definition at all.
-- A holder whose slots/used account_definition exists but is
-- uncontrolled — World, or any holder kind seeded the same way — is
-- excluded from both branches: it has no materialised balance to compare,
-- which is the same reason World's money balance is not reconciled
-- either, and is why that exclusion is a controlled flag, not a
-- named holder id. The two sides are FULL JOINed, not driven from
-- item_holder alone, so a controlled slots/used balance that has drifted
-- away from zero at a holder currently holding NOTHING is still reported.
CREATE VIEW item_capacity_divergence AS
WITH holder_counts AS (
    SELECT holder_id, count(*) AS item_count
    FROM item_holder
    GROUP BY holder_id
),
slots_used AS (
    SELECT s.id AS holder_id, d.controlled, ab.balance
    FROM scope s
    JOIN account a ON a.scope_id = s.id
    JOIN account_definition d ON d.id = a.account_definition_id
    LEFT JOIN account_balance ab ON ab.account_id = a.id
    WHERE d.kind = 'slots' AND d.capacity_role = 'used'
)
SELECT
    COALESCE(hc.holder_id, su.holder_id) AS holder_id,
    COALESCE(hc.item_count, 0)           AS item_count,
    su.balance                           AS slots_used_balance,
    CASE WHEN su.holder_id IS NULL THEN 'no_slots_used_account' ELSE 'count_mismatch' END AS reason
FROM holder_counts hc
FULL JOIN slots_used su ON su.holder_id = hc.holder_id
WHERE su.holder_id IS NULL
   OR (su.controlled AND su.balance IS DISTINCT FROM COALESCE(hc.item_count, 0));
