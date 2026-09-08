-- +goose Up
-- ingest_offset is the guarded singleton the update-ingestion loop reads
-- and advances. next_update_id holds the
-- offset to TRANSMIT on the next getUpdates call, not the last update_id
-- seen — the name says which: settling update_id = n writes n + 1, and
-- the loop transmits the stored value verbatim as GetUpdatesParams.Offset,
-- with the "+ 1" living in exactly one statement (the advance) and
-- nowhere on the poll path. Storing the raw update_id and adding one at
-- transmission would be a permanent live-lock under the guarded-monotone
-- advance below. Seeded at 0, which the Bot API reads as
-- "the earliest unconfirmed update".
CREATE TABLE ingest_offset (
    id             integer NOT NULL DEFAULT 1,
    next_update_id bigint  NOT NULL,
    CONSTRAINT ingest_offset_pkey PRIMARY KEY (id),
    CONSTRAINT ingest_offset_singleton CHECK (id = 1)
);
INSERT INTO ingest_offset (id, next_update_id) VALUES (1, 0);

-- ingest_dead_update is one give-up row for the update-ingestion loop —
-- the same shape the scheduler's own dead-task row already has: the
-- update's own identity and kind, its destination chat when the update
-- carries one, the attempt count and the last error. It carries no raw
-- update payload — the diagnostic surface is DeadUpdates' projection, not
-- a replay mechanism. Both identifying columns (chat_id and last_error)
-- are snapshot-sanitisation targets: a production snapshot rewrites
-- chat_id and blanks last_error, because free-text error prose can embed
-- a real chat or user id a column-targeted rewrite would walk past.
CREATE TABLE ingest_dead_update (
    id                   bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    update_id            bigint      NOT NULL,
    kind                 text        NOT NULL,
    chat_id              bigint,
    consecutive_failures integer     NOT NULL,
    last_error           text        NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ingest_dead_update_kind_nonempty CHECK (kind <> ''),
    CONSTRAINT ingest_dead_update_failures_positive CHECK (consecutive_failures > 0)
);
CREATE INDEX ingest_dead_update_created_at_idx ON ingest_dead_update (created_at);
