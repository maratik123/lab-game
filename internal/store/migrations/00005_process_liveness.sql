-- +goose Up
-- process_liveness is the guarded singleton the composition root's
-- restart-hygiene step reads and refreshes. seen_at is NULL until the
-- first process ever writes it — that NULL is "this database has never
-- had a live process", the state the restart-hygiene shift must
-- distinguish from an ordinary short gap. The row is seeded by this
-- migration rather than by the first start, so AbsorbDowntime's locking
-- SELECT always has a row to lock: an empty table has no row for
-- SELECT ... FOR NO KEY UPDATE to serialise on, and Postgres describes a
-- concurrent INSERT racing a SELECT as merely "might block" rather than
-- the ordering the row-lock conflict table states outright for this
-- mode against itself.
CREATE TABLE process_liveness (
    id      integer     NOT NULL DEFAULT 1,
    seen_at timestamptz,
    CONSTRAINT process_liveness_pkey PRIMARY KEY (id),
    CONSTRAINT process_liveness_singleton CHECK (id = 1)
);
INSERT INTO process_liveness (id, seen_at) VALUES (1, NULL);
