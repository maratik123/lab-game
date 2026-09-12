package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

// TestMove_property drives a sequence of moves over a small fixed set of
// holders and instances against a Go model of holder-per-instance and
// per-holder occupancy, asserting item_holder, both reconciliation views
// and both capacity balances after every accepted move, and no write
// after every deliberately-wrong-holder rejection.
func TestMove_property(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)

	const numHolders = 3
	const numItems = 3
	const budget = 5

	rapid.Check(t, func(rt *rapid.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			rt.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		holders := make([]HolderID, numHolders)
		for i := range holders {
			_, h := newPlayerWithBackpack(t, ctx, tx)
			grantSlots(t, ctx, tx, h, budget)
			holders[i] = h
		}

		items := make([]ItemID, numItems)
		holderOf := make(map[ItemID]HolderID, numItems)
		occupancy := make(map[HolderID]int, numHolders)
		for i := range items {
			dest := holders[i%numHolders]
			ids, err := Move(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "seed"},
				[]Movement{{ItemID: NewItem, From: WorldHolder, To: dest}})
			if err != nil {
				rt.Fatalf("seed mint: %v", err)
			}
			items[i] = ids[0]
			holderOf[items[i]] = dest
			occupancy[dest]++
		}

		assertModel := func() {
			t.Helper()
			for _, item := range items {
				row := selectItemHolder(t, ctx, tx, item)
				if row == nil || row.HolderID != holderOf[item] {
					rt.Fatalf("item_holder for %d = %+v, want holder %d", item, row, holderOf[item])
				}
			}
			if n := countRows(t, ctx, tx, `SELECT * FROM item_chain_break`); n != 0 {
				rt.Fatalf("item_chain_break = %d rows, want 0", n)
			}
			if n := countRows(t, ctx, tx, `SELECT * FROM item_capacity_divergence`); n != 0 {
				rt.Fatalf("item_capacity_divergence = %d rows, want 0", n)
			}
			for _, h := range holders {
				free, used := slotsAccounts(t, ctx, tx, h)
				gotFree := balanceOf(t, ctx, tx, free)
				gotUsed := balanceOf(t, ctx, tx, used)
				wantUsed := int64(occupancy[h])
				wantFree := int64(budget) - wantUsed
				if !gotFree.Equal(decimal.NewFromInt(wantFree)) || !gotUsed.Equal(decimal.NewFromInt(wantUsed)) {
					rt.Fatalf("holder %d balances free=%s used=%s, want free=%d used=%d", h, gotFree, gotUsed, wantFree, wantUsed)
				}
			}
		}
		assertModel()

		steps := rapid.IntRange(1, 8).Draw(rt, "steps")
		for s := 0; s < steps; s++ {
			idx := rapid.IntRange(0, numItems-1).Draw(rt, "item_idx")
			item := items[idx]
			current := holderOf[item]

			others := make([]HolderID, 0, numHolders-1)
			for _, h := range holders {
				if h != current {
					others = append(others, h)
				}
			}
			dest := others[rapid.IntRange(0, len(others)-1).Draw(rt, "dest_idx")]

			// A deliberately wrong From on every draw: the instance's
			// actual holder is current, so naming any other holder as
			// From must be refused, with nothing written.
			// wrongFrom must differ from both current (or it would be the
			// TRUE holder) and dest (or it would collide with the
			// self-move check instead of exercising ErrNotCurrentHolder).
			var wrongFrom HolderID
			for _, h := range holders {
				if h != current && h != dest {
					wrongFrom = h
					break
				}
			}
			rec.Reset()
			if _, err := Move(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "wrong from"},
				[]Movement{{ItemID: item, From: wrongFrom, To: dest}}); !errors.Is(err, ErrNotCurrentHolder) {
				rt.Fatalf("Move with a wrong From = %v, want ErrNotCurrentHolder", err)
			}
			for _, stmt := range rec.Statements() {
				up := strings.ToUpper(stmt.SQL)
				if strings.Contains(up, "INSERT") || strings.Contains(up, "UPDATE") || strings.Contains(up, "DELETE") {
					rt.Fatalf("rejected Move issued a write: %+v", stmt)
				}
			}

			if _, err := Move(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "move"},
				[]Movement{{ItemID: item, From: current, To: dest}}); err != nil {
				rt.Fatalf("accepted move failed: %v", err)
			}
			occupancy[current]--
			occupancy[dest]++
			holderOf[item] = dest
			assertModel()
		}
	})
}
