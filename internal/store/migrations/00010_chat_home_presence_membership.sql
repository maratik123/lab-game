-- +goose Up
-- home is a chat's own location: a scope with no account_definition row,
-- so CreateOwner's generic scope-and-account CTE gives every chat exactly
-- one scope and zero accounts. It carries no maze or cell column and no
-- relation that touches a maze carries a reference to it — the address
-- space a hostile mechanic cannot reach because there is nothing there
-- for its position argument to name.
INSERT INTO scope_definition (id, code, owner_kind) VALUES (4, 'home', 'chat');

-- chat_presence holds the CURRENT answer only, one row per chat: whether
-- the bot is in the chat right now. The history is the event log's
-- (bot_added_to_chat / bot_kicked). present is a boolean, not a stored
-- Bot API status string — that vocabulary is Telegram's to change, and
-- the predicate the game needs is binary; the raw old/new statuses live
-- in the event payload instead, where they are evidence rather than a
-- contract this schema owns. changed_at is written explicitly on every
-- flip (never left to the column default, which fires on INSERT only),
-- so it answers "since when" without a scan of the event log.
CREATE TABLE chat_presence (
    chat_id    bigint      PRIMARY KEY REFERENCES owner (id),
    present    boolean     NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now()
);

-- chat_membership is the recorded link between a player and a chat,
-- written once and never removed: this task records no way for a
-- membership to lapse. A membership is independent of the chat's own
-- presence — the bot's removal governs what it may send, not whether a
-- player's own arrival through that chat's link is real.
CREATE TABLE chat_membership (
    chat_id    bigint      NOT NULL REFERENCES owner (id),
    player_id  bigint      NOT NULL REFERENCES owner (id),
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, player_id)
);
-- Covers the player_id foreign key (player_id trails the primary key, so
-- it needs an index of its own).
CREATE INDEX chat_membership_player_idx ON chat_membership (player_id);

-- chat_knowledge is the union, by chat, of every member's own
-- discoveries: a membership recorded today brings every discovery that
-- player already had into the chat's knowledge, with nothing to
-- back-fill and nothing to keep in step. The column set is chosen
-- minimal (chat, maze, cell) so a later first_discovered_at can be added
-- as an append (min(discovered_at) under a GROUP BY of these same
-- columns leaves this row set exactly where SELECT DISTINCT puts it
-- today); the discoverer is deliberately left out, reachable by joining
-- node_discovery directly, because adding it here would multiply the row
-- set rather than append to it. Its column names, order and types are
-- therefore a permanent contract: CREATE OR REPLACE VIEW can only append
-- a column, never drop, rename, reorder or retype one.
CREATE VIEW chat_knowledge AS
SELECT DISTINCT cm.chat_id, nd.maze_id, nd.q, nd.r
FROM chat_membership cm
JOIN node_discovery nd ON nd.player_id = cm.player_id;

-- Backfill: a scope_definition seeded here creates scopes only for
-- owners created after it (CreateOwner runs at owner-creation time, not
-- as a migration-time sweep) — the same obligation the item-machine
-- migration's own backfill states, and this migration owes exactly one
-- of its three statements: the account and balance statements are
-- omitted because home seeds no account_definition row, so both would
-- select from an empty join forever. This statement is a no-op on every
-- database this migration meets — nothing creates a chat owner before it
-- — and it is written anyway, because "this case is unreachable" is
-- exactly the argument this convention exists to survive.
INSERT INTO scope (owner_id, scope_definition_id)
SELECT o.id, 4 FROM owner o WHERE o.kind = 'chat'
ON CONFLICT (owner_id, scope_definition_id) DO NOTHING;
