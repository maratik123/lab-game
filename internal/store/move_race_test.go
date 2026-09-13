package store

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/testdb"
)

// TestMove_antiConflict races two transactions moving one instance from the
// same current holder to two different destinations. The holder's
// slots_used starts at exactly one — the instance being raced — so a
// racer whose own capacity leg reached the balance before the chain
// decision would drive that balance negative and be refused with the
// wrong sentinel (ErrOverdraft) instead of the chain conflict this test
// targets; the chain insert runs ahead of the balance UPDATEs precisely so
// that never happens. Both attempts share the exact same (item_id,
// prev_movement_id, from_holder_id) successor key regardless of
// destination — that identity, not luck, is what guarantees a collision
// every round; what varies round to round is which transaction wins and
// which sentinel names the loser's refusal. Run under -race.
func TestMove_antiConflict(t *testing.T) {
	t.Parallel()

	const rounds = 15
	var sawMoveConflict atomic.Bool

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
				grantSlots(t, ctx, tx, start, 1)
				grantSlots(t, ctx, tx, destA, 5)
				grantSlots(t, ctx, tx, destB, 5)
				ids, err := Move(ctx, tx, &ManualCorrection{Actor: "race", Reason: "seed"},
					[]Movement{{ItemID: NewItem, From: WorldHolder, To: start}})
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
					sawMoveConflict.Store(true)
					t.Logf("round %d: goroutine %d lost the chain race: %v", round, i, err)
				case errors.Is(err, ErrNotCurrentHolder):
					losers++
					t.Logf("round %d: goroutine %d lost before its own phase b ran: %v", round, i, err)
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
			if n := countRows(t, ctx, pool, `SELECT * FROM item_capacity_divergence`); n != 0 {
				t.Fatalf("round %d: item_capacity_divergence = %d rows, want 0", round, n)
			}
		})
	}

	if !sawMoveConflict.Load() {
		t.Fatalf("across %d rounds, no loser was ever refused with ErrMoveConflict — every round degraded to ErrNotCurrentHolder, never reaching the chain-conflict path this test exists to exercise", rounds)
	}
}

// blockPollCeiling bounds how long TestMove_orderedConflict waits for the
// loser's backend to report itself blocked on the lock a concurrent
// item_movement insert takes. It is a patience budget sized generously
// against a shared server under neighbours' load, not an assertion: on the
// ceiling the test fails with a named cause instead of hanging until the
// package's own test timeout does.
const blockPollCeiling = 10 * time.Second

// blockPollInterval is how often TestMove_orderedConflict re-checks
// pg_stat_activity while waiting for the loser to block.
const blockPollInterval = 10 * time.Millisecond

// TestMove_orderedConflict is the deterministic discriminator for the
// promise that a lost chain race is refused at the chain, not at a
// balance. Unlike TestMove_antiConflict, which lets the scheduler decide
// who wins, this test controls the order directly: the winner's Move runs
// to completion but does not commit, a second Move for the same instance
// is started in a goroutine and is expected to block on the winner's
// uncommitted successor-key entry, and only once that block is observed
// does the winner commit — so the loser's refusal is induced on demand
// rather than by luck.
func TestMove_orderedConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := testdb.Schema(t)

	setupPool, err := NewPool(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("new setup pool: %v", err)
	}
	t.Cleanup(setupPool.Close)
	if err := Migrate(ctx, setupPool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The single-item fixture: start's slots_used is exactly one, the
	// instance being raced, so a racer whose own capacity leg reached the
	// balance before the chain decision would drive it negative and be
	// refused with ErrOverdraft instead of the conflict this test targets.
	var item ItemID
	var start, destA, destB HolderID
	{
		tx, err := setupPool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin setup: %v", err)
		}
		_, start = newPlayerWithBackpack(t, ctx, tx)
		_, destA = newPlayerWithBackpack(t, ctx, tx)
		_, destB = newPlayerWithBackpack(t, ctx, tx)
		grantSlots(t, ctx, tx, start, 1)
		grantSlots(t, ctx, tx, destA, 5)
		grantSlots(t, ctx, tx, destB, 5)
		ids, err := Move(ctx, tx, &ManualCorrection{Actor: "race", Reason: "seed"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: start}})
		if err != nil {
			t.Fatalf("seed Move: %v", err)
		}
		item = ids[0]
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit setup: %v", err)
		}
	}

	// Two pools over the one schema, each with its own recorder, so every
	// statement is attributable to one racer instead of interleaved in a
	// pool-wide log.
	recWinner := &queryRecorder{}
	winnerCfg := cfg.Copy()
	winnerCfg.MaxConns = 1
	winnerCfg.ConnConfig.Tracer = recWinner
	winnerPool, err := NewPool(ctx, winnerCfg)
	if err != nil {
		t.Fatalf("new winner pool: %v", err)
	}
	t.Cleanup(winnerPool.Close)

	recLoser := &queryRecorder{}
	loserCfg := cfg.Copy()
	loserCfg.MaxConns = 1
	loserCfg.ConnConfig.Tracer = recLoser
	loserPool, err := NewPool(ctx, loserCfg)
	if err != nil {
		t.Fatalf("new loser pool: %v", err)
	}
	t.Cleanup(loserPool.Close)

	// Both transactions are rolled back unconditionally on cleanup —
	// rollback is a no-op past a commit (pgx.ErrTxClosed) — so a failed
	// assertion below cannot leave either connection checked out of its
	// one-connection pool and hang that pool's Close.
	winnerTx, err := winnerPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin winner tx: %v", err)
	}
	t.Cleanup(func() { rollback(t, ctx, winnerTx) })
	if _, err := Move(ctx, winnerTx, &ManualCorrection{Actor: "race", Reason: "winner"},
		[]Movement{{ItemID: item, From: start, To: destA}}); err != nil {
		t.Fatalf("winner Move: %v", err)
	}

	loserTx, err := loserPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin loser tx: %v", err)
	}
	t.Cleanup(func() { rollback(t, ctx, loserTx) })
	var loserPID int32
	if err := loserTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&loserPID); err != nil {
		t.Fatalf("select loser backend pid: %v", err)
	}
	recLoser.Reset()

	type loserResult struct {
		err error
	}
	resultCh := make(chan loserResult, 1)
	go func() {
		_, err := Move(ctx, loserTx, &ManualCorrection{Actor: "race", Reason: "loser"},
			[]Movement{{ItemID: item, From: start, To: destB}})
		resultCh <- loserResult{err: err}
	}()

	deadline := time.Now().Add(blockPollCeiling)
	blocked := false
	var early *loserResult
pollLoop:
	for {
		select {
		case r := <-resultCh:
			early = &r
			break pollLoop
		default:
		}
		var waitEventType *string
		if err := setupPool.QueryRow(ctx,
			`SELECT wait_event_type FROM pg_stat_activity WHERE pid = $1`, loserPID,
		).Scan(&waitEventType); err != nil {
			t.Fatalf("poll pg_stat_activity: %v", err)
		}
		if waitEventType != nil && *waitEventType == "Lock" {
			blocked = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(blockPollInterval)
	}
	if !blocked {
		if early != nil {
			t.Fatalf("the loser never blocked: Move returned %v before a block was observed", early.err)
		}
		t.Fatalf("the loser never blocked: pg_stat_activity never reported wait_event_type = Lock for pid %d within %s", loserPID, blockPollCeiling)
	}

	if err := winnerTx.Commit(ctx); err != nil {
		t.Fatalf("commit winner: %v", err)
	}

	loserErr := (<-resultCh).err
	if !errors.Is(loserErr, ErrMoveConflict) {
		t.Fatalf("loser error = %v, want ErrMoveConflict", loserErr)
	}
	if errors.Is(loserErr, ErrOverdraft) {
		t.Fatalf("loser error %v must not also be ErrOverdraft — that is the defect this test exists to catch", loserErr)
	}
	rollback(t, ctx, loserTx)

	for _, s := range recLoser.Statements() {
		if strings.Contains(strings.ToUpper(s.SQL), "UPDATE ACCOUNT_BALANCE") {
			t.Fatalf("the loser issued a balance UPDATE before its refusal: %+v", s)
		}
	}

	// A refused Move leaves no half-written document behind: the loser's
	// basis document, its journal entry, its postings and its movement are
	// all absent once its transaction is rolled back. The winner's are
	// asserted present through the same queries, because a query that
	// cannot see rows at all would report the loser clean whatever Move
	// did.
	underReason := []struct {
		what  string
		query string
	}{
		{"basis document", `SELECT 1 FROM manual_correction mc WHERE mc.reason = $1`},
		{"journal entry", `SELECT 1 FROM journal_entry je
			JOIN manual_correction mc ON mc.id = je.manual_correction_id
			WHERE mc.reason = $1`},
		{"posting", `SELECT 1 FROM posting p
			JOIN journal_entry je ON je.id = p.journal_entry_id
			JOIN manual_correction mc ON mc.id = je.manual_correction_id
			WHERE mc.reason = $1`},
		{"movement", `SELECT 1 FROM item_movement m
			JOIN journal_entry je ON je.id = m.journal_entry_id
			JOIN manual_correction mc ON mc.id = je.manual_correction_id
			WHERE mc.reason = $1`},
	}
	for _, c := range underReason {
		if n := countRows(t, ctx, setupPool, c.query, "loser"); n != 0 {
			t.Fatalf("the loser left %d %s row(s) behind: a refused Move writes neither its document nor anything under it", n, c.what)
		}
		if n := countRows(t, ctx, setupPool, c.query, "winner"); n == 0 {
			t.Fatalf("the winner has no %s row, so the absence asserted for the loser is a blind query rather than a property of the refusal", c.what)
		}
	}

	row := selectItemHolder(t, ctx, setupPool, item)
	if row == nil || row.HolderID != destA {
		t.Fatalf("item_holder = %+v, want the winner's destination %d", row, destA)
	}
	if n := countRows(t, ctx, setupPool, `SELECT * FROM item_chain_break`); n != 0 {
		t.Fatalf("item_chain_break = %d rows, want 0", n)
	}
	if n := countRows(t, ctx, setupPool, `SELECT * FROM item_capacity_divergence`); n != 0 {
		t.Fatalf("item_capacity_divergence = %d rows, want 0", n)
	}
}
