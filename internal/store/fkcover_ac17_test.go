package store

import (
	"context"
	"testing"
)

func TestFKCoverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	if got := uncoveredFKs(t, ctx, pool); len(got) != 0 {
		t.Fatalf("uncovered FKs on the migrated schema: %v, want none", got)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	if _, err := tx.Exec(ctx, `
		CREATE TABLE planted (
			id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			owner_id bigint NOT NULL CONSTRAINT planted_owner_fk REFERENCES owner (id)
		)
	`); err != nil {
		t.Fatalf("create planted table: %v", err)
	}

	got := uncoveredFKs(t, ctx, tx)
	if len(got) != 1 || got[0] != "planted_owner_fk" {
		t.Fatalf("uncovered FKs with planted table = %v, want [planted_owner_fk]", got)
	}

	if _, err := tx.Exec(ctx, `CREATE INDEX planted_owner_idx ON planted (owner_id)`); err != nil {
		t.Fatalf("create index: %v", err)
	}

	if got := uncoveredFKs(t, ctx, tx); len(got) != 0 {
		t.Fatalf("uncovered FKs after indexing = %v, want none", got)
	}
}
