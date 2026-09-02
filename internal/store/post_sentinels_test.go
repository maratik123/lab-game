package store

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPost_sentinels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)

	setupTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin setup: %v", err)
	}
	money, _ := createPlayer(t, ctx, setupTx)
	fund(t, ctx, setupTx, money, KindMoney, decimal.RequireFromString("1"))
	if err := setupTx.Commit(ctx); err != nil {
		t.Fatalf("commit setup: %v", err)
	}

	var nilPO *PlayerOperation

	tests := []struct {
		name       string
		basis      PostingBasis
		postings   []Posting
		wantErr    error
		preSQLOnly bool // recorder must be empty after rec.Reset()
		wantOneSQL bool // recorder must hold exactly one statement (phase b's SELECT)
	}{
		{
			name:       "nil_interface_basis",
			basis:      nil,
			postings:   []Posting{{AccountID: money, Amount: decimal.RequireFromString("1")}},
			wantErr:    ErrNoBasis,
			preSQLOnly: true,
		},
		{
			name:       "typed_nil_basis",
			basis:      nilPO,
			postings:   []Posting{{AccountID: money, Amount: decimal.RequireFromString("1")}},
			wantErr:    ErrNoBasis,
			preSQLOnly: true,
		},
		{
			name:       "empty_batch",
			basis:      &ManualCorrection{Actor: "t", Reason: "r"},
			postings:   nil,
			wantErr:    ErrEmptyBatch,
			preSQLOnly: true,
		},
		{
			name:       "zero_amount",
			basis:      &ManualCorrection{Actor: "t", Reason: "r"},
			postings:   []Posting{{AccountID: money, Amount: decimal.Zero}},
			wantErr:    ErrInvalidAmount,
			preSQLOnly: true,
		},
		{
			name:       "sub_scale_amount",
			basis:      &ManualCorrection{Actor: "t", Reason: "r"},
			postings:   []Posting{{AccountID: money, Amount: decimal.RequireFromString("0.000005")}},
			wantErr:    ErrInvalidAmount,
			preSQLOnly: true,
		},
		{
			name:  "excess_magnitude_amount",
			basis: &ManualCorrection{Actor: "t", Reason: "r"},
			postings: []Posting{
				{AccountID: money, Amount: decimal.New(1, 25)},
			},
			wantErr:    ErrInvalidAmount,
			preSQLOnly: true,
		},
		{
			name:  "unknown_account",
			basis: &ManualCorrection{Actor: "t", Reason: "r"},
			postings: []Posting{
				{AccountID: AccountID(1 << 40), Amount: decimal.RequireFromString("1")},
			},
			wantErr:    ErrUnknownAccount,
			wantOneSQL: true,
		},
		{
			name:  "unbalanced",
			basis: &ManualCorrection{Actor: "t", Reason: "r"},
			postings: []Posting{
				{AccountID: money, Amount: decimal.RequireFromString("1")},
			},
			wantErr:    ErrUnbalanced,
			wantOneSQL: true,
		},
		{
			name:  "overdraft",
			basis: &ManualCorrection{Actor: "t", Reason: "r"},
			postings: []Posting{
				{AccountID: money, Amount: decimal.RequireFromString("-2")},
				{AccountID: WorldMoney, Amount: decimal.RequireFromString("2")},
			},
			wantErr: ErrOverdraft,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Not t.Parallel(): these subtests share one queryRecorder via
			// Reset()/Statements(), so they must run sequentially.
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer rollback(t, ctx, tx)

			rec.Reset()
			err = Post(ctx, tx, tc.basis, tc.postings...)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Post = %v, want %v", err, tc.wantErr)
			}
			switch {
			case tc.preSQLOnly:
				if got := rec.Statements(); len(got) != 0 {
					t.Fatalf("recorder after %s = %+v, want empty", tc.name, got)
				}
			case tc.wantOneSQL:
				if got := rec.Statements(); len(got) != 1 {
					t.Fatalf("recorder after %s = %+v, want exactly one statement (phase b's SELECT)", tc.name, got)
				}
			}
		})
	}

	t.Run("replay", func(t *testing.T) {
		t.Parallel()

		basis := &PlayerOperation{Source: SourceTelegram, OperationID: "sentinel-replay"}

		tx1, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx1: %v", err)
		}
		if err := Post(ctx, tx1, basis,
			Posting{AccountID: money, Amount: decimal.RequireFromString("1")},
			Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-1")},
		); err != nil {
			t.Fatalf("first Post: %v", err)
		}
		if err := tx1.Commit(ctx); err != nil {
			t.Fatalf("commit tx1: %v", err)
		}

		tx2, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx2: %v", err)
		}
		defer rollback(t, ctx, tx2)

		err = Post(ctx, tx2, basis,
			Posting{AccountID: money, Amount: decimal.RequireFromString("1")},
			Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-1")},
		)
		if !errors.Is(err, ErrAlreadyPosted) {
			t.Fatalf("replay Post = %v, want ErrAlreadyPosted", err)
		}
	})

	t.Run("balance_row_missing", func(t *testing.T) {
		t.Parallel()

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		localMoney, _ := createPlayer(t, ctx, tx)
		if _, err := tx.Exec(ctx, `DELETE FROM account_balance WHERE account_id = $1`, localMoney); err != nil {
			t.Fatalf("delete balance row: %v", err)
		}

		err = Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: localMoney, Amount: decimal.RequireFromString("-1")},
			Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("1")},
		)
		if !errors.Is(err, ErrBalanceRowMissing) {
			t.Fatalf("Post = %v, want ErrBalanceRowMissing", err)
		}
		if errors.Is(err, ErrOverdraft) {
			t.Fatalf("ErrBalanceRowMissing must be distinguishable from ErrOverdraft")
		}
	})
}
