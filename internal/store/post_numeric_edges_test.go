package store

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPost_numeric_edges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)

	var ownerCount, postingCount int
	countRows := func(t *testing.T) (int, int) {
		t.Helper()
		var oc, pc int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&oc); err != nil {
			t.Fatalf("count owner: %v", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM posting`).Scan(&pc); err != nil {
			t.Fatalf("count posting: %v", err)
		}
		return oc, pc
	}
	ownerCount, postingCount = countRows(t)

	t.Run("sixth_fractional_digit_rejected", func(t *testing.T) {
		rec.Reset()
		err := Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: money, Amount: decimal.RequireFromString("0.000005")},
			Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-0.000005")},
		)
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("Post = %v, want ErrInvalidAmount", err)
		}
		if got := rec.Statements(); len(got) != 0 {
			t.Fatalf("recorder = %+v, want empty", got)
		}
		oc, pc := countRows(t)
		if oc != ownerCount || pc != postingCount {
			t.Fatalf("row counts changed: owner %d->%d, posting %d->%d", ownerCount, oc, postingCount, pc)
		}
	})

	t.Run("26_integer_digits_rejected", func(t *testing.T) {
		rec.Reset()
		amount := decimal.New(1, 25) // 10^25, 26 integer digits
		err := Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: money, Amount: amount},
			Posting{AccountID: WorldMoney, Amount: amount.Neg()},
		)
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("Post = %v, want ErrInvalidAmount", err)
		}
		if got := rec.Statements(); len(got) != 0 {
			t.Fatalf("recorder = %+v, want empty", got)
		}
		oc, pc := countRows(t)
		if oc != ownerCount || pc != postingCount {
			t.Fatalf("row counts changed: owner %d->%d, posting %d->%d", ownerCount, oc, postingCount, pc)
		}
	})

	t.Run("25_digit_amount_round_trips", func(t *testing.T) {
		amount := decimal.RequireFromString("9999999999999999999999999.99999")
		if err := Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: money, Amount: amount},
			Posting{AccountID: WorldMoney, Amount: amount.Neg()},
		); err != nil {
			t.Fatalf("Post: %v", err)
		}

		var got string
		if err := tx.QueryRow(ctx,
			`SELECT amount::text FROM posting WHERE account_id = $1 ORDER BY id DESC LIMIT 1`, money,
		).Scan(&got); err != nil {
			t.Fatalf("select posting: %v", err)
		}
		if got != amount.String() {
			t.Fatalf("round-tripped amount = %s, want %s", got, amount.String())
		}
	})
}
