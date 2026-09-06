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

-- The five MVP views (Scope 5). Every day grain below is UTC, spelled
-- (ts AT TIME ZONE 'UTC')::date rather than a bare ts::date, because a bare
-- cast resolves against the session TimeZone and this suite pins only
-- search_path — a container default and a LAB_GAME_TEST_DSN server could
-- otherwise disagree. Every denominator is wrapped in NULLIF(x, 0) and every
-- ratio's numerator cast to ::numeric, so a rate divides to NULL rather than
-- erroring or truncating to 0 under integer division. A count column is
-- always COALESCE(…, 0), never NULL, including where its stage matched
-- nothing (count() over an outer-joined miss is already 0; only ratio
-- columns are nullable, and only where their denominator is empty). Their
-- column names, order and types are a permanent contract: CREATE OR REPLACE
-- VIEW can only append a column, never drop, rename, reorder or retype one.

-- metric_activation_funnel answers §13.3's headline MVP number:
-- bot_added_to_chat → player_started → first raid_started → return the
-- next day. Attribution is a property of the PLAYER, not of the raid: each
-- player with a player_started naming a chat belongs to the chat of their
-- earliest such event, ties broken by the lower chat id, so the map is a
-- function and the view is deterministic. raid_started.chat_id is never
-- read — every raid stage joins on player_id alone — which is an owner
-- decision (round 3) that trades an unrecorded-chat_id obligation on
-- raid_started for one on player_started instead (ai-docs/domain-invariants.md
-- §5). "Returned" means a raid_started on the day immediately after that
-- player's own earliest raid day — not "raided again at any later point",
-- and not "was active the next day".
CREATE VIEW metric_activation_funnel AS
WITH attribution AS (
    SELECT DISTINCT ON (e.player_id)
        e.player_id,
        e.chat_id
    FROM event e
    WHERE e.type = 'player_started' AND e.player_id IS NOT NULL AND e.chat_id IS NOT NULL
    ORDER BY e.player_id, e.ts, e.chat_id
),
chats AS (
    SELECT e.chat_id, min(e.ts) AS added_at
    FROM event e
    WHERE e.type = 'bot_added_to_chat' AND e.chat_id IS NOT NULL
    GROUP BY e.chat_id
),
first_raid AS (
    SELECT e.player_id, min((e.ts AT TIME ZONE 'UTC')::date) AS first_raid_day
    FROM event e
    WHERE e.type = 'raid_started' AND e.player_id IS NOT NULL
    GROUP BY e.player_id
),
returned AS (
    SELECT DISTINCT fr.player_id
    FROM first_raid fr
    JOIN event e ON e.type = 'raid_started' AND e.player_id = fr.player_id
        AND (e.ts AT TIME ZONE 'UTC')::date = fr.first_raid_day + 1
)
SELECT
    c.chat_id,
    c.added_at,
    count(a.player_id)  AS player_started,
    count(fr.player_id) AS player_first_raided,
    count(r.player_id)  AS player_returned_next_day
FROM chats c
LEFT JOIN attribution a ON a.chat_id = c.chat_id
LEFT JOIN first_raid   fr ON fr.player_id = a.player_id
LEFT JOIN returned     r  ON r.player_id  = a.player_id
GROUP BY c.chat_id, c.added_at;

-- metric_retention_daily answers §13.3's "Возвраты: D1/D7 retention, рейдов
-- на игрока в день". Row set: one row per UTC day on which ANY event
-- occurred (not a generated calendar range, so a day with no events is
-- absent rather than zero-filled). Two readings are chosen here rather than
-- read off §13.3, both owner decisions (round 3): a "return" is a
-- raid_started, not any activity; and D1/D7 are exactly day+1 and day+7,
-- not "active within N days". A cohort born fewer than seven days ago
-- cannot yet have a D7 (nor one born today a D1) and reports 0, not NULL,
-- for it — a retention "cliff" at the series' recent end is this window
-- artefact, not the game (ai-docs/domain-invariants.md §5).
CREATE VIEW metric_retention_daily AS
WITH days AS (
    SELECT DISTINCT (e.ts AT TIME ZONE 'UTC')::date AS day
    FROM event e
),
active AS (
    SELECT (e.ts AT TIME ZONE 'UTC')::date AS day, count(DISTINCT e.player_id) AS active_player
    FROM event e
    WHERE e.player_id IS NOT NULL
    GROUP BY 1
),
raids AS (
    SELECT (e.ts AT TIME ZONE 'UTC')::date AS day, count(*) AS raid
    FROM event e
    WHERE e.type = 'raid_started'
    GROUP BY 1
),
new_players AS (
    SELECT e.player_id, min((e.ts AT TIME ZONE 'UTC')::date) AS day
    FROM event e
    WHERE e.type = 'player_started' AND e.player_id IS NOT NULL
    GROUP BY e.player_id
),
cohort AS (
    SELECT day, count(*) AS new_player
    FROM new_players
    GROUP BY day
),
d1 AS (
    SELECT np.day, count(DISTINCT np.player_id) AS d1_retained
    FROM new_players np
    JOIN event e ON e.type = 'raid_started' AND e.player_id = np.player_id
        AND (e.ts AT TIME ZONE 'UTC')::date = np.day + 1
    GROUP BY np.day
),
d7 AS (
    SELECT np.day, count(DISTINCT np.player_id) AS d7_retained
    FROM new_players np
    JOIN event e ON e.type = 'raid_started' AND e.player_id = np.player_id
        AND (e.ts AT TIME ZONE 'UTC')::date = np.day + 7
    GROUP BY np.day
)
SELECT
    d.day,
    COALESCE(a.active_player, 0) AS active_player,
    COALESCE(r.raid, 0)          AS raid,
    COALESCE(r.raid, 0)::numeric / NULLIF(COALESCE(a.active_player, 0), 0) AS raid_per_active_player,
    COALESCE(c.new_player, 0)    AS new_player,
    COALESCE(d1.d1_retained, 0)  AS d1_retained,
    COALESCE(d7.d7_retained, 0)  AS d7_retained,
    COALESCE(d1.d1_retained, 0)::numeric / NULLIF(COALESCE(c.new_player, 0), 0) AS d1_rate,
    COALESCE(d7.d7_retained, 0)::numeric / NULLIF(COALESCE(c.new_player, 0), 0) AS d7_rate
FROM days d
LEFT JOIN active a  ON a.day  = d.day
LEFT JOIN raids  r  ON r.day  = d.day
LEFT JOIN cohort c  ON c.day  = d.day
LEFT JOIN d1        ON d1.day = d.day
LEFT JOIN d7        ON d7.day = d.day;

-- metric_death_by_depth answers §13.3's "Смерти по глубине". Row set: one
-- row per (UTC day, depth) with at least one death; NULL depth is its own,
-- deliberately visible group — a death with no recorded depth, not a value
-- to discard. The day column is shipped from round 4 on: without it no
-- query recovers the daily split later (adding the grain would change the
-- row set, which CREATE OR REPLACE VIEW cannot do), whereas the all-time
-- distribution is one GROUP BY depth away from this shape.
CREATE VIEW metric_death_by_depth AS
SELECT
    (e.ts AT TIME ZONE 'UTC')::date AS day,
    e.depth,
    count(*)                    AS death,
    count(DISTINCT e.player_id) AS player
FROM event e
WHERE e.type = 'death'
GROUP BY 1, e.depth;

-- metric_faucet_sink answers §13.3's faucet/sink per resource, generic over
-- ledger_kind membership (AC14): it groups by account_definition.kind, not
-- by any named enum member, so a new kind appears as soon as postings of it
-- exist against a World-owned account — no edit to this view is needed.
-- account_definition.kind is cast to text (rule 3: a view-carried enum
-- value is cast so a Grafana reader, and a test that adds an enum member on
-- a pooled connection, never depends on when a connection resolved that
-- type's OID). Row set: one row per (UTC day, kind) with at least one
-- posting touching a World-owned account that day; day comes from
-- journal_entry.ts, not event.ts.
CREATE VIEW metric_faucet_sink AS
WITH world_postings AS (
    SELECT
        (je.ts AT TIME ZONE 'UTC')::date AS day,
        ad.kind::text                    AS kind,
        p.amount
    FROM posting p
    JOIN journal_entry je ON je.id = p.journal_entry_id
    JOIN account a ON a.id = p.account_id
    JOIN account_definition ad ON ad.id = a.account_definition_id
    JOIN scope s ON s.id = a.scope_id
    JOIN owner o ON o.id = s.owner_id
    WHERE o.kind = 'world'
)
SELECT
    day,
    kind,
    COALESCE(-sum(amount) FILTER (WHERE amount < 0), 0) AS faucet,
    COALESCE(sum(amount) FILTER (WHERE amount > 0), 0)  AS sink,
    COALESCE(-sum(amount), 0)                           AS net_to_economy
FROM world_postings
GROUP BY day, kind;

-- metric_notification_per_chat_day answers §13.3's per-chat spam budget.
-- Row set: one row per (UTC day, chat) with at least one chat-bearing
-- notification_sent; a quiet chat-day is absent, not a zero row. Rows with
-- a null chat_id are excluded — this is a per-chat number, not a global
-- one, and the exclusion is asserted by a fixture row, not left implicit.
CREATE VIEW metric_notification_per_chat_day AS
SELECT
    (e.ts AT TIME ZONE 'UTC')::date AS day,
    e.chat_id,
    count(*) AS notification
FROM event e
WHERE e.type = 'notification_sent' AND e.chat_id IS NOT NULL
GROUP BY 1, e.chat_id;
