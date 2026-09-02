package store

import (
	"context"
	"errors"
	"testing"
)

func TestBasis_nil_returns_ErrNoBasis_without_panicking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	var nilPO *PlayerOperation //nolint:staticcheck // SA4023: referenced by the deliberate typed-nil-in-interface comparison below
	if _, err := nilPO.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*PlayerOperation)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilPO.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*PlayerOperation)(nil).insert() = %v, want ErrNoBasis", err)
	}

	var nilMC *ManualCorrection
	if _, err := nilMC.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*ManualCorrection)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilMC.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*ManualCorrection)(nil).insert() = %v, want ErrNoBasis", err)
	}

	// A typed-nil pointer stored in the interface is still != nil, and
	// entrySQL/insert on it must go through the same guarded path, not
	// panic. staticcheck SA4023 flags both lines below as statically
	// decidable — that decidability is exactly the Go gotcha under test.
	var basis PostingBasis = nilPO
	if basis == nil { //nolint:staticcheck // SA4023: never true by design — asserts the typed-nil-in-interface gotcha
		t.Fatalf("a typed-nil *PlayerOperation stored in PostingBasis must not compare equal to nil")
	}
	if _, err := basis.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("basis.entrySQL() via typed-nil interface = %v, want ErrNoBasis", err)
	}
}

func TestPlayerOperation_insert_replay_returns_ErrAlreadyPosted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	po := &PlayerOperation{Source: SourceTelegram, OperationID: "basis-replay-1"}

	id1, err := po.insert(ctx, tx)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if id1 == 0 {
		t.Fatalf("expected a non-zero id")
	}

	_, err = po.insert(ctx, tx)
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("replay insert = %v, want ErrAlreadyPosted", err)
	}

	// The transaction must still be usable — no 23505 was raised by the
	// database (ON CONFLICT DO NOTHING), so a further statement succeeds.
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("transaction unusable after replay: %v", err)
	}
}
