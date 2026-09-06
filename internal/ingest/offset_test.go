package ingest

import (
	"context"
	"testing"
)

func TestReadOffset_freshSchemaReadsSeededZero(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	got, err := readOffset(ctx, tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if got != 0 {
		t.Fatalf("readOffset on a fresh schema = %d, want 0", got)
	}
}

func TestAdvanceOffset_higherValueIsStoredAndReadBackByANewReader(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := advanceOffset(ctx, tx, 42); err != nil {
		t.Fatalf("advanceOffset: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// A brand-new transaction — "no in-process value is the sole record"
	// (Test Design subtask 7).
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (reader): %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	got, err := readOffset(ctx, tx2)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if got != 42 {
		t.Fatalf("readOffset after advance = %d, want 42", got)
	}
}

func TestAdvanceOffset_atOrBelowStoredValueIsANoop(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := advanceOffset(ctx, tx, 100); err != nil {
		t.Fatalf("advanceOffset(100): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, next := range []int64{100, 50, 0} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := advanceOffset(ctx, tx, next); err != nil {
			t.Fatalf("advanceOffset(%d): %v", next, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (reader): %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	got, err := readOffset(ctx, tx2)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if got != 100 {
		t.Fatalf("offset after at-or-below advances = %d, want unchanged 100", got)
	}
}

func TestReadOffset_errorSurfacesOnAClosedTransaction(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if _, err := readOffset(ctx, tx); err == nil {
		t.Fatalf("readOffset on a closed transaction: want an error, got nil")
	}
}

func TestAdvanceOffset_errorSurfacesOnAClosedTransaction(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if err := advanceOffset(ctx, tx, 1); err == nil {
		t.Fatalf("advanceOffset on a closed transaction: want an error, got nil")
	}
}

func TestAdvanceOffset_rolledBackLeavesStoredValueUntouched(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := advanceOffset(ctx, tx, 77); err != nil {
		t.Fatalf("advanceOffset: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (reader): %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	got, err := readOffset(ctx, tx2)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if got != 0 {
		t.Fatalf("offset after a rolled-back advance = %d, want the untouched seed 0", got)
	}
}
