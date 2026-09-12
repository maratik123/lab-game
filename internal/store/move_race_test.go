package store

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

// TestMove_antiConflict races two transactions moving one instance from the
// same current holder to two different destinations. Both attempts share
// the exact same (item_id, prev_movement_id, from_holder_id) successor
// key regardless of destination — that identity, not luck, is what
// guarantees a collision every round; what varies round to round is which
// transaction wins and whether the loser's refusal ever surfaces as
// anything other than the expected block-then-refuse. Run under -race.
func TestMove_antiConflict(t *testing.T) {
	t.Parallel()

	const rounds = 15

	for round := 0; round < rounds; round++ {
		round := round
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			cfg := testdb.Schema(t)
			cfg.MaxConns = 2
			pool, err := NewPool(ctx, cfg)
			if err != nil {
				t.Fatalf("new pool: %v", err)
			}
			t.Cleanup(pool.Close)
			if err := Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			var item ItemID
			var start, destA, destB HolderID
			{
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatalf("begin setup: %v", err)
				}
				_, start = newPlayerWithBackpack(t, ctx, tx)
				_, destA = newPlayerWithBackpack(t, ctx, tx)
				_, destB = newPlayerWithBackpack(t, ctx, tx)
				grantSlots(t, ctx, tx, start, 5)
				grantSlots(t, ctx, tx, destA, 5)
				grantSlots(t, ctx, tx, destB, 5)
				// A second, unraced "anchor" instance shares start with the
				// raced one: with start's slots_used at 1 alone, the
				// loser's own capacity legs (posted before either
				// transaction's chain insert is attempted) would decrement
				// an already-decremented balance to -1 and surface a
				// spurious ErrOverdraft rather than the chain conflict this
				// test targets. At 2, both transactions' independent
				// decrements land at 1 and 0 — never negative — so the
				// race is decided where it is meant to be: the chain
				// insert.
				ids, err := Move(ctx, tx, &ManualCorrection{Actor: "race", Reason: "seed"},
					[]Movement{
						{ItemID: NewItem, From: WorldHolder, To: start},
						{ItemID: NewItem, From: WorldHolder, To: start},
					})
				if err != nil {
					t.Fatalf("seed Move: %v", err)
				}
				item = ids[0]
				if err := tx.Commit(ctx); err != nil {
					t.Fatalf("commit setup: %v", err)
				}
			}

			tx1, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin tx1: %v", err)
			}
			tx2, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin tx2: %v", err)
			}

			results := make([]error, 2)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, err := Move(ctx, tx1, &ManualCorrection{Actor: "race", Reason: "to A"},
					[]Movement{{ItemID: item, From: start, To: destA}})
				results[0] = err
				if err == nil {
					if cErr := tx1.Commit(ctx); cErr != nil {
						t.Errorf("commit tx1: %v", cErr)
					}
				} else {
					rollback(t, ctx, tx1)
				}
			}()
			go func() {
				defer wg.Done()
				_, err := Move(ctx, tx2, &ManualCorrection{Actor: "race", Reason: "to B"},
					[]Movement{{ItemID: item, From: start, To: destB}})
				results[1] = err
				if err == nil {
					if cErr := tx2.Commit(ctx); cErr != nil {
						t.Errorf("commit tx2: %v", cErr)
					}
				} else {
					rollback(t, ctx, tx2)
				}
			}()
			wg.Wait()

			winners, losers := 0, 0
			var winnerDest HolderID
			for i, err := range results {
				switch {
				case err == nil:
					winners++
					if i == 0 {
						winnerDest = destA
					} else {
						winnerDest = destB
					}
				case errors.Is(err, ErrMoveConflict):
					losers++
					t.Logf("round %d: goroutine %d lost the chain race: %v", round, i, err)
				default:
					t.Fatalf("round %d: goroutine %d returned an unexpected error: %v", round, i, err)
				}
			}
			if winners != 1 || losers != 1 {
				t.Fatalf("round %d: winners=%d losers=%d, want exactly one of each (results: %v)", round, winners, losers, results)
			}

			row := selectItemHolder(t, ctx, pool, item)
			if row == nil || row.HolderID != winnerDest {
				t.Fatalf("round %d: item_holder = %+v, want holder %d", round, row, winnerDest)
			}
			if n := countRows(t, ctx, pool, `SELECT * FROM item_chain_break`); n != 0 {
				t.Fatalf("round %d: item_chain_break = %d rows, want 0", round, n)
			}
		})
	}
}
