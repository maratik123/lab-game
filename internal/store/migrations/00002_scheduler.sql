-- +goose Up
CREATE TYPE scheduled_task_state AS ENUM ('pending', 'dead');

CREATE TABLE scheduled_task (
    id                   bigint               GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    type                 text                 NOT NULL,
    instance_key         text,
    payload              jsonb                NOT NULL,
    run_at               timestamptz          NOT NULL,
    state                scheduled_task_state NOT NULL DEFAULT 'pending',
    consecutive_failures integer              NOT NULL DEFAULT 0,
    last_error           text,
    created_at           timestamptz          NOT NULL DEFAULT now(),
    CONSTRAINT scheduled_task_type_nonempty CHECK (type <> ''),
    CONSTRAINT scheduled_task_failures_nonnegative CHECK (consecutive_failures >= 0)
);
CREATE UNIQUE INDEX scheduled_task_identity_key ON scheduled_task (type, instance_key)
    WHERE instance_key IS NOT NULL AND state = 'pending';
CREATE INDEX scheduled_task_due_idx ON scheduled_task (run_at) WHERE state = 'pending';

CREATE TABLE deferred_task (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id      bigint,                      -- by value; deliberately NOT a foreign key (AC27)
    task_type    text        NOT NULL,
    instance_key text,
    run_at       timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE recurrent_task (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id      bigint,                      -- by value; deliberately NOT a foreign key (AC27)
    task_type    text        NOT NULL,
    instance_key text,
    run_at       timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE journal_entry
    ADD COLUMN deferred_task_id  bigint REFERENCES deferred_task  (id),
    ADD COLUMN recurrent_task_id bigint REFERENCES recurrent_task (id),
    DROP CONSTRAINT journal_entry_exactly_one_basis,
    ADD  CONSTRAINT journal_entry_exactly_one_basis
         CHECK (num_nonnulls(player_operation_id, manual_correction_id,
                             deferred_task_id, recurrent_task_id) = 1);
CREATE UNIQUE INDEX journal_entry_deferred_task_key  ON journal_entry (deferred_task_id)  WHERE deferred_task_id  IS NOT NULL;
CREATE UNIQUE INDEX journal_entry_recurrent_task_key ON journal_entry (recurrent_task_id) WHERE recurrent_task_id IS NOT NULL;
