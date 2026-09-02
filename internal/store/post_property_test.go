package store

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"
)

// bandedAmount draws a scale-5 decimal from AC2's three int64-safe bands:
// band 0 is a single digit whole part, band 1 up to nine digits, band 2 up
// to ~1.6e6 shifted 18 places (1e23..1.6e24), each plus a random scale-5
// fraction. A zero draw is lifted to 0.00002 so every leg is non-zero.
func bandedAmount(rt *rapid.T, label string) decimal.Decimal {
	frac := decimal.New(rapid.Int64Range(0, 99999).Draw(rt, label+"_frac"), -5)

	var whole decimal.Decimal
	switch rapid.IntRange(0, 2).Draw(rt, label+"_band") {
	case 0:
		whole = decimal.New(rapid.Int64Range(0, 9).Draw(rt, label+"_w1"), 0)
	case 1:
		whole = decimal.New(rapid.Int64Range(10, 999999999).Draw(rt, label+"_w2"), 0)
	default:
		whole = decimal.New(rapid.Int64Range(100000, 1600000).Draw(rt, label+"_w3"), 18)
	}

	amount := whole.Add(frac)
	if amount.IsZero() {
		return decimal.RequireFromString("0.00002")
	}
	return amount
}

func TestPost_zero_invariant_property(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)

	rapid.Check(t, func(rt *rapid.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			rt.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		type player struct {
			money, experience AccountID
		}
		players := make([]player, 3)
		for i := range players {
			m, e := createPlayer(t, ctx, tx)
			players[i] = player{money: m, experience: e}
		}

		funding := decimal.RequireFromString("1000000000000000000000000") // 1e24
		for _, p := range players {
			if err := Post(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "fund money"},
				Posting{AccountID: p.money, Amount: funding},
				Posting{AccountID: WorldMoney, Amount: funding.Neg()},
			); err != nil {
				rt.Fatalf("fund money: %v", err)
			}
			if err := Post(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "fund experience"},
				Posting{AccountID: p.experience, Amount: funding},
				Posting{AccountID: WorldExperience, Amount: funding.Neg()},
			); err != nil {
				rt.Fatalf("fund experience: %v", err)
			}
		}

		tracked := make(map[AccountID]decimal.Decimal, 8)
		for _, p := range players {
			tracked[p.money] = funding
			tracked[p.experience] = funding
		}

		var batch []Posting

		legsFor := func(accounts []AccountID, worldID AccountID) {
			n := rapid.IntRange(1, 5).Draw(rt, "legs")
			var worldDelta decimal.Decimal
			for i := 0; i < n; i++ {
				acc := accounts[rapid.IntRange(0, len(accounts)-1).Draw(rt, "acct")]
				amount := bandedAmount(rt, "amt")
				if rapid.Bool().Draw(rt, "sign") {
					amount = amount.Neg()
				}
				// A debit the tracked balance cannot afford becomes a credit.
				if amount.IsNegative() && tracked[acc].Add(amount).IsNegative() {
					amount = amount.Neg()
				}
				batch = append(batch, Posting{AccountID: acc, Amount: amount})
				tracked[acc] = tracked[acc].Add(amount)
				worldDelta = worldDelta.Add(amount)
			}
			if !worldDelta.IsZero() {
				batch = append(batch, Posting{AccountID: worldID, Amount: worldDelta.Neg()})
			}
		}

		moneyAccounts := []AccountID{players[0].money, players[1].money, players[2].money}
		expAccounts := []AccountID{players[0].experience, players[1].experience, players[2].experience}
		legsFor(moneyAccounts, WorldMoney)
		legsFor(expAccounts, WorldExperience)

		if err := Post(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "batch"}, batch...); err != nil {
			rt.Fatalf("Post accepted batch failed: %v", err)
		}

		for acc, want := range tracked {
			if want.IsNegative() || want.GreaterThanOrEqual(decimal.New(1, 25)) {
				rt.Fatalf("tracked balance for account %d = %s is out of the structural bound", acc, want)
			}
			got := balanceOf(t, ctx, tx, acc)
			if !got.Equal(want) {
				rt.Fatalf("balance for account %d = %s, want %s", acc, got, want)
			}
		}

		// Mutation: nudge one leg by one ULP away from zero, breaking the
		// per-kind sum; the mutated batch must be rejected before any write.
		mutated := make([]Posting, len(batch))
		copy(mutated, batch)
		idx := rapid.IntRange(0, len(mutated)-1).Draw(rt, "mutate_idx")
		ulp := decimal.New(1, -5)
		if mutated[idx].Amount.IsNegative() {
			ulp = ulp.Neg()
		}
		mutated[idx].Amount = mutated[idx].Amount.Add(ulp)
		// Keep the mutated amount itself individually valid (D5), so the
		// rejection is provably the per-kind sum, not the per-posting check.
		if mutated[idx].Amount.IsZero() || mutated[idx].Amount.Abs().GreaterThanOrEqual(decimal.New(1, 25)) {
			return // vanishingly rare; skip this draw
		}

		var ownerCountBefore, postingCountBefore int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&ownerCountBefore); err != nil {
			rt.Fatalf("count owner: %v", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM posting`).Scan(&postingCountBefore); err != nil {
			rt.Fatalf("count posting: %v", err)
		}

		rec.Reset()
		err = Post(ctx, tx, &ManualCorrection{Actor: "rapid", Reason: "mutated"}, mutated...)
		if !errors.Is(err, ErrUnbalanced) {
			rt.Fatalf("mutated batch Post = %v, want ErrUnbalanced", err)
		}

		var ownerCountAfter, postingCountAfter int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&ownerCountAfter); err != nil {
			rt.Fatalf("count owner: %v", err)
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM posting`).Scan(&postingCountAfter); err != nil {
			rt.Fatalf("count posting: %v", err)
		}
		if ownerCountAfter != ownerCountBefore || postingCountAfter != postingCountBefore {
			rt.Fatalf("row counts changed on rejection: owner %d->%d, posting %d->%d",
				ownerCountBefore, ownerCountAfter, postingCountBefore, postingCountAfter)
		}
	})
}
