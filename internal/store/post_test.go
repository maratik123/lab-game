package store

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// Positive control: the exact grep pattern this suite relies on must hit
// each of the four lines below and neither decoy. Never executed as SQL —
// these are comment text planted for a source-sweep test elsewhere in
// this package.
//
// UPDATE posting SET amount = 0
// delete from posting where id = 1
// UPDATE journal_entry SET ts = now()
// delete from journal_entry where id = 1
//
// Decoys (must NOT match): update postings_archive set amount = 0
// Decoys (must NOT match): delete from journal_entry_archive where id = 1

var nextTelegramID atomic.Int64

func init() {
	nextTelegramID.Store(1_000_000)
}

// createPlayer creates a player owner inside tx and returns its money and
// experience account ids.
func createPlayer(t *testing.T, ctx context.Context, tx pgx.Tx) (money, experience AccountID) {
	t.Helper()
	tg := nextTelegramID.Add(1)
	owner, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	for _, a := range owner.Accounts {
		switch a.Kind {
		case KindMoney:
			money = a.ID
		case KindExperience:
			experience = a.ID
		}
	}
	return money, experience
}

// fund posts a ManualCorrection crediting accountID by amount from the
// matching World account of the given kind.
func fund(t *testing.T, ctx context.Context, tx pgx.Tx, accountID AccountID, kind Kind, amount decimal.Decimal) {
	t.Helper()
	var worldID AccountID
	switch kind {
	case KindMoney:
		worldID = WorldMoney
	case KindExperience:
		worldID = WorldExperience
	}
	err := Post(ctx, tx, &ManualCorrection{Actor: "test", Reason: "fund"},
		Posting{AccountID: accountID, Amount: amount},
		Posting{AccountID: worldID, Amount: amount.Neg()},
	)
	if err != nil {
		t.Fatalf("fund: %v", err)
	}
}

func balanceOf(t *testing.T, ctx context.Context, q Queryer, accountID AccountID) decimal.Decimal {
	t.Helper()
	var b decimal.Decimal
	if err := q.QueryRow(ctx, `SELECT balance FROM account_balance WHERE account_id = $1`, accountID).Scan(&b); err != nil {
		t.Fatalf("balance of account %d: %v", accountID, err)
	}
	return b
}

func TestPost_fixed_fractional(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)

	err = Post(ctx, tx, &ManualCorrection{Actor: "test", Reason: "fixed"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("0.1")},
		Posting{AccountID: money, Amount: decimal.RequireFromString("0.2")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-0.3")},
	)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	got := balanceOf(t, ctx, tx, money)
	want := decimal.RequireFromString("0.30000")
	if !got.Equal(want) {
		t.Fatalf("balance = %s, want %s", got, want)
	}
}

func TestPost_KD7_same_account_twice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.RequireFromString("10"))

	t.Run("net_nonzero_one_update", func(t *testing.T) {
		before := balanceOf(t, ctx, tx, money)
		err := Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: money, Amount: decimal.RequireFromString("3")},
			Posting{AccountID: money, Amount: decimal.RequireFromString("-1")},
			Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-2")},
		)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		got := balanceOf(t, ctx, tx, money)
		want := before.Add(decimal.RequireFromString("2"))
		if !got.Equal(want) {
			t.Fatalf("balance = %s, want %s", got, want)
		}
	})

	t.Run("net_zero_no_update", func(t *testing.T) {
		before := balanceOf(t, ctx, tx, money)
		err := Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			Posting{AccountID: money, Amount: decimal.RequireFromString("1")},
			Posting{AccountID: money, Amount: decimal.RequireFromString("-1")},
		)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		got := balanceOf(t, ctx, tx, money)
		if !got.Equal(before) {
			t.Fatalf("balance = %s, want unchanged %s", got, before)
		}
	})
}

func TestPost_overdraft(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.RequireFromString("1.00000"))

	err = Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "overdraft"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("-1.00001")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("1.00001")},
	)
	if !errors.Is(err, ErrOverdraft) {
		t.Fatalf("overdraft Post = %v, want ErrOverdraft", err)
	}
}

func TestPost_overdraft_integer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.RequireFromString("5"))

	err = Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "overdraft"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("-6")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("6")},
	)
	if !errors.Is(err, ErrOverdraft) {
		t.Fatalf("overdraft Post = %v, want ErrOverdraft", err)
	}
}

func TestPost_world_may_go_negative(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)

	err = Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "world grant"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("1.00001")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-1.00001")},
	)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	var sum decimal.Decimal
	if err := tx.QueryRow(ctx, `SELECT sum(amount) FROM posting WHERE account_id = $1`, WorldMoney).Scan(&sum); err != nil {
		t.Fatalf("sum: %v", err)
	}
	if !sum.IsNegative() {
		t.Fatalf("World money sum = %s, want negative", sum)
	}
}

func TestPost_balance_row_missing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	if _, err := tx.Exec(ctx, `DELETE FROM account_balance WHERE account_id = $1`, money); err != nil {
		t.Fatalf("delete balance row: %v", err)
	}

	err = Post(ctx, tx, &ManualCorrection{Actor: "t", Reason: "missing row"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("-1")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("1")},
	)
	if !errors.Is(err, ErrBalanceRowMissing) {
		t.Fatalf("Post = %v, want ErrBalanceRowMissing", err)
	}
	if errors.Is(err, ErrOverdraft) {
		t.Fatalf("ErrBalanceRowMissing must not also be ErrOverdraft")
	}
}

func TestPost_player_operation_replay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	money, _ := createPlayer(t, ctx, tx1)
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit setup: %v", err)
	}

	basis := &PlayerOperation{Source: SourceTelegram, OperationID: "replay-1"}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	if err := Post(ctx, tx2, basis,
		Posting{AccountID: money, Amount: decimal.RequireFromString("1")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-1")},
	); err != nil {
		t.Fatalf("first Post: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit tx2: %v", err)
	}

	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer rollback(t, ctx, tx3)

	err = Post(ctx, tx3, basis,
		Posting{AccountID: money, Amount: decimal.RequireFromString("1")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("-1")},
	)
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("replay Post = %v, want ErrAlreadyPosted", err)
	}

	// Transaction still usable.
	var one int
	if err := tx3.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("tx3 unusable after replay: %v", err)
	}

	var poCount, entryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM player_operation WHERE source = 'telegram' AND operation_id = 'replay-1'`).Scan(&poCount); err != nil {
		t.Fatalf("count player_operation: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM journal_entry WHERE player_operation_id = (SELECT id FROM player_operation WHERE source='telegram' AND operation_id='replay-1')`).Scan(&entryCount); err != nil {
		t.Fatalf("count journal_entry: %v", err)
	}
	if poCount != 1 || entryCount != 1 {
		t.Fatalf("player_operation=%d journal_entry=%d, want 1/1", poCount, entryCount)
	}

	got := balanceOf(t, ctx, pool, money)
	want := decimal.RequireFromString("1.00000")
	if !got.Equal(want) {
		t.Fatalf("balance after replay = %s, want %s (applied once)", got, want)
	}
}

func TestPost_cancelled_context_writes_nothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.RequireFromString("5"))

	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	rec.Reset()
	err = Post(cancelled, tx, &ManualCorrection{Actor: "t", Reason: "cancelled"},
		Posting{AccountID: money, Amount: decimal.RequireFromString("-1")},
		Posting{AccountID: WorldMoney, Amount: decimal.RequireFromString("1")},
	)
	if err == nil {
		t.Fatal("Post with a cancelled context succeeded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Post = %v, want an error wrapping context.Canceled", err)
	}
	for _, s := range []error{ErrNoBasis, ErrEmptyBatch, ErrInvalidAmount, ErrUnknownAccount, ErrUnbalanced, ErrAlreadyPosted, ErrOverdraft, ErrBalanceRowMissing} {
		if errors.Is(err, s) {
			t.Fatalf("Post = %v wrongly matches the domain sentinel %v", err, s)
		}
	}
	for _, s := range rec.Statements() {
		up := strings.ToUpper(s.SQL)
		if strings.Contains(up, "INSERT") || strings.Contains(up, "UPDATE") {
			t.Fatalf("cancelled Post issued a write: %+v", s)
		}
	}
}
