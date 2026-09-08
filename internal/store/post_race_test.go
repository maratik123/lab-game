package store

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/testdb"
)

// craftBatch builds a "крафт" shape: 3-5 distinct controlled
// accounts debited by an integer amount in 1..10, plus one balancing World
// credit per kind touched.
func craftBatch(r *rand.Rand, players []racePlayer) []Posting {
	n := 3 + r.IntN(3) // 3..5
	type leg struct {
		account AccountID
		kind    Kind
	}
	seen := map[AccountID]bool{}
	var legs []leg
	for len(legs) < n {
		p := players[r.IntN(len(players))]
		var l leg
		if r.IntN(2) == 0 {
			l = leg{account: p.money, kind: KindMoney}
		} else {
			l = leg{account: p.experience, kind: KindExperience}
		}
		if seen[l.account] {
			continue
		}
		seen[l.account] = true
		legs = append(legs, l)
	}

	worldDelta := map[Kind]int64{}
	postings := make([]Posting, 0, len(legs)+2)
	for _, l := range legs {
		amount := int64(1 + r.IntN(10)) // 1..10
		postings = append(postings, Posting{AccountID: l.account, Amount: decimal.New(-amount, 0)})
		worldDelta[l.kind] += amount
	}
	for kind, total := range worldDelta {
		if total == 0 {
			continue
		}
		worldID := WorldMoney
		if kind == KindExperience {
			worldID = WorldExperience
		}
		postings = append(postings, Posting{AccountID: worldID, Amount: decimal.New(total, 0)})
	}
	return postings
}

type racePlayer struct {
	money, experience AccountID
}

func TestPost_anti_deadlock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := testdb.Schema(t)
	const workers int32 = 8
	cfg.MaxConns = workers

	pool, err := NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const numPlayers = 4
	players := make([]racePlayer, numPlayers)
	{
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin setup: %v", err)
		}
		for i := range players {
			m, e := createPlayer(t, ctx, tx)
			players[i] = racePlayer{money: m, experience: e}
		}
		const funding = "1000000"
		for _, p := range players {
			fund(t, ctx, tx, p.money, KindMoney, decimal.RequireFromString(funding))
			fund(t, ctx, tx, p.experience, KindExperience, decimal.RequireFromString(funding))
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit setup: %v", err)
		}
	}

	const rounds = 25
	var deadlocks atomic.Int32
	var wg sync.WaitGroup
	for w := int32(0); w < workers; w++ {
		wg.Add(1)
		go func(worker int32) {
			defer wg.Done()

			const seed1 uint64 = 20260902
			seed2 := uint64(worker)
			t.Logf("worker %d seed=(%d,%d)", worker, seed1, seed2)
			r := rand.New(rand.NewPCG(seed1, seed2))

			a := players[worker%numPlayers]
			b := players[(worker+1)%numPlayers]

			for round := 0; round < rounds; round++ {
				var batch []Posting
				var reason string
				if round%2 == 0 {
					amount := decimal.New(int64(1+r.IntN(10)), 0)
					from, to := a.money, b.money
					if worker%2 == 1 {
						from, to = b.money, a.money
					}
					batch = []Posting{
						{AccountID: from, Amount: amount.Neg()},
						{AccountID: to, Amount: amount},
					}
					reason = "transfer"
				} else {
					batch = craftBatch(r, players)
					reason = "craft"
				}

				tx, txErr := pool.Begin(ctx)
				var err error
				if txErr != nil {
					err = txErr
				} else {
					err = Post(ctx, tx, &ManualCorrection{Actor: "race", Reason: reason}, batch...)
					if err == nil {
						err = tx.Commit(ctx)
					} else {
						if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
							t.Errorf("rollback: %v", rbErr)
						}
					}
				}
				if err != nil {
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
						deadlocks.Add(1)
					}
					t.Errorf("worker %d round %d: %v", worker, round, err)
				}
			}
		}(w)
	}
	wg.Wait()

	if n := deadlocks.Load(); n > 0 {
		t.Fatalf("observed %d deadlock(s) (40P01)", n)
	}

	for _, p := range players {
		for _, acc := range []AccountID{p.money, p.experience} {
			var balance, postedSum decimal.Decimal
			if err := pool.QueryRow(ctx, `SELECT balance FROM account_balance WHERE account_id = $1`, acc).Scan(&balance); err != nil {
				t.Fatalf("balance for %d: %v", acc, err)
			}
			if err := pool.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM posting WHERE account_id = $1`, acc).Scan(&postedSum); err != nil {
				t.Fatalf("posted sum for %d: %v", acc, err)
			}
			if !balance.Equal(postedSum) {
				t.Fatalf("account %d balance = %s, posting sum = %s", acc, balance, postedSum)
			}
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT d.kind, sum(p.amount)
		FROM posting p
		JOIN account a ON a.id = p.account_id
		JOIN account_definition d ON d.id = a.account_definition_id
		GROUP BY d.kind
	`)
	if err != nil {
		t.Fatalf("per-kind sum query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var sum decimal.Decimal
		if err := rows.Scan(&kind, &sum); err != nil {
			t.Fatalf("scan per-kind sum: %v", err)
		}
		if !sum.IsZero() {
			t.Fatalf("kind %s posting sum = %s, want 0", kind, sum)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
