-- +goose Up
-- chunk_type and chunk_creation_cause are the persisted-world consumer's
-- own vocabulary, mirrored by that consumer's own test, following
-- scheduled_task_state's precedent of an enum with no mirror in this
-- package's own Go source. Both grow later through a forward migration
-- that adds a member to the type.
CREATE TYPE chunk_type AS ENUM ('fabric', 'gate');
CREATE TYPE chunk_creation_cause AS ENUM ('explorer', 'chat_activation');

-- maze is a persisted world: a biome run under a season, at a fixed
-- generation seed. UNIQUE (biome, season) makes "the maze of this biome
-- this season" a lookup and a rotation an insert.
CREATE TABLE maze (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    biome      text        NOT NULL,
    world_seed bigint      NOT NULL,
    season     integer     NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (biome, season)
);

-- chunk is one created chunk of a maze, keyed on its super-lattice
-- coordinate. chunk_type and creation_cause are NOT NULL because the CHECK
-- below compares chunk_type = 'gate'; a NULL there would make the
-- comparison NULL and the whole constraint pass. faces holds the stored
-- map's encoded bytes and is never regenerated once written.
CREATE TABLE chunk (
    maze_id            bigint               NOT NULL REFERENCES maze (id),
    q                  integer              NOT NULL,
    r                  integer              NOT NULL,
    chunk_type         chunk_type           NOT NULL,
    creation_cause     chunk_creation_cause NOT NULL,
    gate_chat_id       bigint               REFERENCES owner (id),
    spiral_index       bigint,
    faces              bytea                NOT NULL,
    generation_version integer              NOT NULL,
    created_at         timestamptz          NOT NULL DEFAULT now(),
    PRIMARY KEY (maze_id, q, r),
    CONSTRAINT chunk_gate_fields_together
        CHECK ((chunk_type = 'gate') = (gate_chat_id IS NOT NULL)
           AND (chunk_type = 'gate') = (spiral_index IS NOT NULL))
);
-- Makes "the chat's gate in this maze" unique; not the gate_chat_id FK's
-- cover — that FK is covered by the standalone index below, because a
-- covering index must lead with the FK's own columns and this one leads
-- with maze_id.
CREATE UNIQUE INDEX chunk_maze_gate_chat_key ON chunk (maze_id, gate_chat_id) WHERE gate_chat_id IS NOT NULL;
-- Covers the gate_chat_id foreign key.
CREATE INDEX chunk_gate_chat_idx ON chunk (gate_chat_id) WHERE gate_chat_id IS NOT NULL;

-- node_discovery is a player's personal record of having reached a cell —
-- no chat column, because a discovery is never a chat's. A repeat insert
-- of the same (maze, cell, player) is a caller decision (ON CONFLICT DO
-- NOTHING), not a schema one.
CREATE TABLE node_discovery (
    maze_id       bigint      NOT NULL REFERENCES maze (id),
    q             integer     NOT NULL,
    r             integer     NOT NULL,
    player_id     bigint      NOT NULL REFERENCES owner (id),
    discovered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (maze_id, q, r, player_id)
);
-- Covers the player_id foreign key (player_id trails the primary key, so
-- it needs an index of its own).
CREATE INDEX node_discovery_player_idx ON node_discovery (player_id);

-- The chunk-creation event: one row for every created chunk, whatever its
-- cause, per the dictionary's own subject-then-past-participle convention.
INSERT INTO event_type_definition (id, code, volume_class) VALUES
    (17, 'chunk_created', 'low_volume');
