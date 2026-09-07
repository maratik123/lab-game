package ingest

import (
	"context"
	"testing"
)

func TestDeadUpdates_deterministicOrderAndLimit(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	chatA := int64(111)
	chatB := int64(222)
	rows := []DeadUpdate{
		{UpdateID: 3, Kind: KindMessage, ChatID: &chatA, ConsecutiveFailures: 5, LastError: "boom 3"},
		{UpdateID: 1, Kind: KindCallbackQuery, ChatID: nil, ConsecutiveFailures: 2, LastError: "boom 1"},
		{UpdateID: 2, Kind: KindMessage, ChatID: &chatB, ConsecutiveFailures: 5, LastError: "boom 2"},
	}
	for _, d := range rows {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := writeDeadUpdate(ctx, tx, d); err != nil {
			t.Fatalf("writeDeadUpdate: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (reader): %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	got, err := DeadUpdates(ctx, tx, 10)
	if err != nil {
		t.Fatalf("DeadUpdates: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("DeadUpdates returned %d rows, want 3", len(got))
	}
	// Inserted in the order UpdateID 3, 1, 2 — created_at is monotone with
	// insertion order, so the deterministic order is insertion order, not
	// UpdateID order.
	wantOrder := []int64{3, 1, 2}
	for i, w := range wantOrder {
		if got[i].UpdateID != w {
			t.Fatalf("row %d: UpdateID = %d, want %d (full: %+v)", i, got[i].UpdateID, w, got)
		}
	}
	if got[1].ChatID != nil {
		t.Fatalf("row for update_id=1: ChatID = %v, want nil", got[1].ChatID)
	}
	if got[0].ChatID == nil || *got[0].ChatID != chatA {
		t.Fatalf("row for update_id=3: ChatID = %v, want %d", got[0].ChatID, chatA)
	}

	limited, err := DeadUpdates(ctx, tx, 2)
	if err != nil {
		t.Fatalf("DeadUpdates (limited): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("DeadUpdates(limit=2) returned %d rows, want 2", len(limited))
	}
}

func TestWriteDeadUpdate_errorSurfacesOnAClosedTransaction(t *testing.T) {
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

	err = writeDeadUpdate(ctx, tx, DeadUpdate{UpdateID: 1, Kind: KindMessage, ConsecutiveFailures: 1, LastError: "e"})
	if err == nil {
		t.Fatalf("writeDeadUpdate on a closed transaction: want an error, got nil")
	}
}

func TestDeadUpdates_errorSurfacesOnAClosedTransaction(t *testing.T) {
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

	if _, err := DeadUpdates(ctx, tx, 10); err == nil {
		t.Fatalf("DeadUpdates on a closed transaction: want an error, got nil")
	}
}

func TestDeadUpdates_neitherCommitsNorRollsBackTheCallersTx(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := writeDeadUpdate(ctx, tx, DeadUpdate{
		UpdateID: 9, Kind: KindMessage, ConsecutiveFailures: 1, LastError: "e",
	}); err != nil {
		t.Fatalf("writeDeadUpdate: %v", err)
	}

	if _, err := DeadUpdates(ctx, tx, 10); err != nil {
		t.Fatalf("DeadUpdates: %v", err)
	}

	// tx is still usable — DeadUpdates neither committed nor rolled it
	// back — so a further statement on it succeeds.
	if _, err := DeadUpdates(ctx, tx, 10); err != nil {
		t.Fatalf("DeadUpdates (second call on the same tx): %v", err)
	}
}
