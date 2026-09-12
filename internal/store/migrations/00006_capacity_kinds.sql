-- +goose Up
-- slots and weight are the two capacity ledger kinds the item machine's
-- holder address space posts against. This file adds only the enum
-- members: Postgres refuses a new enum member's use inside the same
-- transaction that adds it, so every consumer of these two members waits
-- for the next migration, which runs in its own transaction after this
-- one has committed.
ALTER TYPE ledger_kind ADD VALUE 'slots';
ALTER TYPE ledger_kind ADD VALUE 'weight';
