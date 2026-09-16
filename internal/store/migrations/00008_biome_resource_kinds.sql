-- +goose Up
-- spun_sugar, pastel_fleece and glitter_dust are the MVP biome's resource
-- kinds. This file adds only the enum members: Postgres refuses a new
-- enum member's use inside the same transaction that adds it, and no
-- account_definition row is added here — nothing posts these kinds yet.
ALTER TYPE ledger_kind ADD VALUE 'spun_sugar';
ALTER TYPE ledger_kind ADD VALUE 'pastel_fleece';
ALTER TYPE ledger_kind ADD VALUE 'glitter_dust';
