-- +goose Up
-- event_volume_class partitions the event-type registry by traffic volume so
-- a later retention pass can split storage by class without a migration to
-- classify events first (docs/DESIGN.md §11, §13.4). Members name the axis
-- itself (volume), not a reader (product/health) or a retention policy this
-- migration does not ship.
CREATE TYPE event_volume_class AS ENUM ('low_volume', 'high_volume');

-- event_type_definition is the event-type registry: a new type is added by a
-- forward migration inserting one row, never by editing this comment. Its id
-- exists solely to carry the §13.4 declaration order so the Go mirror's
-- comparison can be element-for-element ordered rather than set-wise; the id
-- is referenced by nothing — event.type references code, not id.
CREATE TABLE event_type_definition (
    id           smallint           PRIMARY KEY,
    code         text               NOT NULL UNIQUE,
    volume_class event_volume_class NOT NULL
);

INSERT INTO event_type_definition (id, code, volume_class) VALUES
    (1,  'bot_added_to_chat',       'low_volume'),
    (2,  'player_started',          'low_volume'),
    (3,  'raid_started',            'low_volume'),
    (4,  'node_entered',            'low_volume'),
    (5,  'combat_resolved',         'low_volume'),
    (6,  'raid_finished',           'low_volume'),
    (7,  'death',                   'low_volume'),
    (8,  'backpack_dropped',        'low_volume'),
    (9,  'backpack_looted',         'low_volume'),
    (10, 'backpack_expired',        'low_volume'),
    (11, 'stamina_burned_overflow', 'low_volume'),
    (12, 'shop_sale',               'low_volume'),
    (13, 'shop_purchase',           'low_volume'),
    (14, 'notification_sent',       'high_volume'),
    (15, 'button_clicked',          'high_volume'),
    (16, 'bot_kicked',              'low_volume');

-- event is the append-only product-analytics log (docs/DESIGN.md §13.1): the
-- §13.4 dimensions that are universal across types — player, chat, maze,
-- depth — are columns; everything else is payload (ai-docs/domain-invariants.md
-- §5). maze_id carries no foreign key because no maze table exists yet (#29
-- adds it). No UPDATE/DELETE is ever issued against this table outside a
-- test (KD-3's append-only posture, enforced in code, never by a database
-- privilege).
CREATE TABLE event (
    id        bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type      text        NOT NULL,
    player_id bigint      REFERENCES owner (id),
    chat_id   bigint      REFERENCES owner (id),
    maze_id   bigint,                                    -- no FK yet: no maze table (#29)
    depth     integer,
    payload   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ts        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT event_type_fkey FOREIGN KEY (type) REFERENCES event_type_definition (code)
);
-- Covers the type FK (leading column) and every per-type, time-ranged view.
CREATE INDEX event_type_ts_idx ON event (type, ts);
-- Covers the player FK; the funnel and retention views group by player.
CREATE INDEX event_player_idx ON event (player_id) WHERE player_id IS NOT NULL;
-- Covers the chat FK; the funnel and notification views group by chat.
CREATE INDEX event_chat_idx ON event (chat_id) WHERE chat_id IS NOT NULL;
-- The all-type day grain the retention view scans; mirrors journal_entry_ts_idx.
CREATE INDEX event_ts_idx ON event (ts);

-- The arc extension: an event may be the basis document of a journal_entry,
-- exactly as a player_operation, manual_correction, deferred_task or
-- recurrent_task may (docs/DESIGN.md §11). The partial unique index makes
-- the arc 1:1 — one event backs at most one journal_entry; a mechanic that
-- must move balances twice under one event needs two events, or a different
-- basis. There is no idempotency key on event: unlike player_operation,
-- replaying an equal Event value writes a second row, not ErrAlreadyPosted.
ALTER TABLE journal_entry
    ADD COLUMN event_id bigint REFERENCES event (id),
    DROP CONSTRAINT journal_entry_exactly_one_basis,
    ADD  CONSTRAINT journal_entry_exactly_one_basis
         CHECK (num_nonnulls(player_operation_id, manual_correction_id,
                             deferred_task_id, recurrent_task_id, event_id) = 1);
CREATE UNIQUE INDEX journal_entry_event_key ON journal_entry (event_id) WHERE event_id IS NOT NULL;
