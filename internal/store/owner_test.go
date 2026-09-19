package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/testdb"
)

// balanceInvariantViolations counts accounts whose account_balance
// row-presence disagrees with their definition's controlled flag.
func balanceInvariantViolations(t *testing.T, ctx context.Context, tx pgx.Tx) int {
	t.Helper()
	var count int
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM account a
		JOIN account_definition d ON d.id = a.account_definition_id
		LEFT JOIN account_balance ab ON ab.account_id = a.id
		WHERE (d.controlled AND ab.account_id IS NULL)
		   OR (NOT d.controlled AND ab.account_id IS NOT NULL)
	`).Scan(&count)
	if err != nil {
		t.Fatalf("balance invariant query: %v", err)
	}
	return count
}

func TestCreateOwner_player(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	if got := balanceInvariantViolations(t, ctx, tx); got != 0 {
		t.Fatalf("balance invariant violations after migration = %d, want 0", got)
	}

	tg := int64(100)
	owner, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if len(owner.Accounts) != 6 {
		t.Fatalf("player accounts = %d, want 6 (money, experience, and the backpack's slots/weight free/used pairs)", len(owner.Accounts))
	}
	for _, a := range owner.Accounts {
		if !a.Controlled {
			t.Fatalf("player account %+v should be controlled", a)
		}
	}

	if got := balanceInvariantViolations(t, ctx, tx); got != 0 {
		t.Fatalf("balance invariant violations after player creation = %d, want 0", got)
	}

	for _, a := range owner.Accounts {
		var balance string
		if err := tx.QueryRow(ctx,
			`SELECT balance::text FROM account_balance WHERE account_id = $1`, a.ID,
		).Scan(&balance); err != nil {
			t.Fatalf("balance for account %d: %v", a.ID, err)
		}
		if balance != "0.00000" {
			t.Fatalf("balance for account %d = %s, want 0.00000", a.ID, balance)
		}
	}

	// Deleting one row makes the invariant report a violation.
	if _, err := tx.Exec(ctx,
		`DELETE FROM account_balance WHERE account_id = $1`, owner.Accounts[0].ID); err != nil {
		t.Fatalf("delete balance row: %v", err)
	}
	if got := balanceInvariantViolations(t, ctx, tx); got != 1 {
		t.Fatalf("balance invariant violations after deleting a row = %d, want 1", got)
	}
}

func TestCreateOwner_duplicate_telegram_id(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := int64(200)
	if _, err := CreateOwner(ctx, tx, OwnerPlayer, &tg); err != nil {
		t.Fatalf("first CreateOwner: %v", err)
	}

	_, err = CreateOwner(ctx, tx, OwnerPlayer, &tg)
	sqlstate(t, err, "23505", "owner_kind_telegram_id_key")
}

func TestCreateOwner_second_world(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	_, err = CreateOwner(ctx, tx, OwnerWorld, nil)
	if !errors.Is(err, ErrInvalidOwner) {
		t.Fatalf("CreateOwner(OwnerWorld) = %v, want ErrInvalidOwner", err)
	}

	var ownerCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&ownerCount); err != nil {
		t.Fatalf("count owner: %v", err)
	}
	if ownerCount != 1 {
		t.Fatalf("owner count = %d, want 1 (no row created)", ownerCount)
	}
}

func TestCreateOwner_chat_has_home_scope_and_no_accounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := int64(300)
	owner, err := CreateOwner(ctx, tx, OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner(OwnerChat): %v", err)
	}
	if len(owner.Accounts) != 0 {
		t.Fatalf("chat accounts = %d, want 0", len(owner.Accounts))
	}

	rows, err := tx.Query(ctx,
		`SELECT scope_definition_id FROM scope WHERE owner_id = $1`, owner.ID,
	)
	if err != nil {
		t.Fatalf("query scope: %v", err)
	}
	var scopeDefIDs []int16
	for rows.Next() {
		var id int16
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan scope: %v", err)
		}
		scopeDefIDs = append(scopeDefIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(scopeDefIDs) != 1 || scopeDefIDs[0] != 4 {
		t.Fatalf("chat scope_definition ids = %v, want exactly [4] (the home scope)", scopeDefIDs)
	}
}

func TestCreateOwner_rejection_table(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	tg := int64(400)

	for _, tc := range []struct {
		name       string
		kind       OwnerKind
		telegramID *int64
	}{
		{"player_nil_telegram_id", OwnerPlayer, nil},
		{"chat_nil_telegram_id", OwnerChat, nil},
		{"world_with_telegram_id", OwnerWorld, &tg},
		{"unknown_kind", OwnerKind("corpse"), &tg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer rollback(t, ctx, tx)

			var before int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&before); err != nil {
				t.Fatalf("count owner before: %v", err)
			}

			_, err = CreateOwner(ctx, tx, tc.kind, tc.telegramID)
			if !errors.Is(err, ErrInvalidOwner) {
				t.Fatalf("CreateOwner(%s) = %v, want ErrInvalidOwner", tc.kind, err)
			}

			var after int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&after); err != nil {
				t.Fatalf("count owner after: %v", err)
			}
			if after != before {
				t.Fatalf("owner count changed from %d to %d — a statement was issued", before, after)
			}
		})
	}
}

// TestPlayerExists_kindPairIsThePredicate asserts the core
// property: the (kind, telegram_id) PAIR is what PlayerExists checks, not
// the telegram_id column alone — a chat sharing the same telegram_id as a
// player must report false.
func TestPlayerExists_kindPairIsThePredicate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	const telegramID = int64(777)
	if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('chat', $1)`, telegramID); err != nil {
		t.Fatalf("insert chat owner: %v", err)
	}

	got, err := PlayerExists(ctx, tx, telegramID)
	if err != nil {
		t.Fatalf("PlayerExists: %v", err)
	}
	if got {
		t.Fatalf("PlayerExists(%d) = true for a chat owner sharing the id, want false", telegramID)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('player', $1)`, telegramID); err != nil {
		t.Fatalf("insert player owner: %v", err)
	}

	got, err = PlayerExists(ctx, tx, telegramID)
	if err != nil {
		t.Fatalf("PlayerExists: %v", err)
	}
	if !got {
		t.Fatalf("PlayerExists(%d) = false after a player owner was created, want true", telegramID)
	}
}

// TestPlayerExists_noOwnerReportsFalse pins the third row of the
// predicate's own table: an id with no owner row at all reports false,
// not an error.
func TestPlayerExists_noOwnerReportsFalse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	got, err := PlayerExists(ctx, pool, 999999)
	if err != nil {
		t.Fatalf("PlayerExists: %v", err)
	}
	if got {
		t.Fatalf("PlayerExists(999999) = true for an id with no owner row, want false")
	}
}

// TestPlayerExists_closedPoolSurfacesError asserts a closed pool's error
// is returned rather than papered over as a false.
func TestPlayerExists_closedPoolSurfacesError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := testdb.Schema(t)
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	pool.Close()

	if _, err := PlayerExists(ctx, pool, 1); err == nil {
		t.Fatal("PlayerExists over a closed pool: want an error, got nil")
	}
}

// TestPlayerExists_txAndPoolAgree asserts the same call behaves
// identically through a pgx.Tx and through a *pgxpool.Pool.
func TestPlayerExists_txAndPoolAgree(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	const telegramID = int64(555)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('player', $1)`, telegramID); err != nil {
		t.Fatalf("insert player owner: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	viaTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (read): %v", err)
	}
	defer rollback(t, ctx, viaTx)

	gotTx, err := PlayerExists(ctx, viaTx, telegramID)
	if err != nil {
		t.Fatalf("PlayerExists(tx): %v", err)
	}
	gotPool, err := PlayerExists(ctx, pool, telegramID)
	if err != nil {
		t.Fatalf("PlayerExists(pool): %v", err)
	}
	if gotTx != gotPool {
		t.Fatalf("PlayerExists via tx = %v, via pool = %v, want them to agree", gotTx, gotPool)
	}
	// Both must actually report the row exists — `gotTx != gotPool` alone
	// passes for `false != false` just as readily as for the intended
	// `true == true`, so a PlayerExists that always returns false would
	// pass the equality check above unnoticed.
	if !gotTx || !gotPool {
		t.Fatalf("PlayerExists via tx = %v, via pool = %v, want both true (telegram_id=%d was inserted)", gotTx, gotPool, telegramID)
	}

	const missingTelegramID = int64(556)
	gotTxMissing, err := PlayerExists(ctx, viaTx, missingTelegramID)
	if err != nil {
		t.Fatalf("PlayerExists(tx, missing): %v", err)
	}
	gotPoolMissing, err := PlayerExists(ctx, pool, missingTelegramID)
	if err != nil {
		t.Fatalf("PlayerExists(pool, missing): %v", err)
	}
	if gotTxMissing || gotPoolMissing {
		t.Fatalf("PlayerExists via tx = %v, via pool = %v, want both false (telegram_id=%d was never inserted)", gotTxMissing, gotPoolMissing, missingTelegramID)
	}
}

func TestEnsureOwner_createsOnMiss(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := nextTelegramID.Add(1)
	owner, created, err := EnsureOwner(ctx, tx, OwnerChat, &tg)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	if !created {
		t.Fatalf("created = %v, want true (no owner existed yet)", created)
	}
	if owner.ID == 0 {
		t.Fatalf("owner.ID is zero, want a created row's id")
	}
}

func TestEnsureOwner_returnsExistingOnHit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := nextTelegramID.Add(1)
	first, created, err := EnsureOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("first EnsureOwner: %v", err)
	}
	if !created {
		t.Fatalf("first call created = %v, want true", created)
	}

	second, created, err := EnsureOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("second EnsureOwner: %v", err)
	}
	if created {
		t.Fatalf("second call created = %v, want false (the owner already existed)", created)
	}
	if second.ID != first.ID {
		t.Fatalf("second EnsureOwner returned id %d, want the same id %d", second.ID, first.ID)
	}
	if second.Accounts != nil {
		t.Fatalf("second EnsureOwner (a hit) Accounts = %v, want nil", second.Accounts)
	}
}

// TestEnsureOwner_chatGetsHomeScope pins the created-chat scope shape
// exactly as CreateOwner's own test does — proof this is read from the
// scope row, never off the returned value.
func TestEnsureOwner_chatGetsHomeScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := nextTelegramID.Add(1)
	owner, _, err := EnsureOwner(ctx, tx, OwnerChat, &tg)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}

	rows, err := tx.Query(ctx, `SELECT scope_definition_id FROM scope WHERE owner_id = $1`, owner.ID)
	if err != nil {
		t.Fatalf("query scope: %v", err)
	}
	var scopeDefIDs []int16
	for rows.Next() {
		var id int16
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan scope: %v", err)
		}
		scopeDefIDs = append(scopeDefIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(scopeDefIDs) != 1 || scopeDefIDs[0] != 4 {
		t.Fatalf("chat scope_definition ids = %v, want exactly [4] (the home scope)", scopeDefIDs)
	}
}

// TestEnsureOwner_playerScopeUnchanged asserts a created player's account
// set is unaffected by this migration — still the six accounts
// TestCreateOwner_player pins.
func TestEnsureOwner_playerScopeUnchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	tg := nextTelegramID.Add(1)
	owner, _, err := EnsureOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	if len(owner.Accounts) != 6 {
		t.Fatalf("player accounts = %d, want 6", len(owner.Accounts))
	}
}

// TestEnsureOwner_concurrentCreatorViolationPropagates reproduces a
// concurrent creator deterministically: tx2 reads first — exactly
// EnsureOwner's own opening SELECT — and correctly finds no row, since
// tx1 has not committed yet. tx1 then wins the race, via EnsureOwner,
// and commits. tx2 then does what EnsureOwner would do next on its own
// (now stale) miss: call CreateOwner. That insert loses the race against
// the unique index tx1's commit just populated, and the violation is
// returned wrapped, not swallowed. A fresh call afterwards, in a fresh
// transaction, finds the winner's row.
func TestEnsureOwner_concurrentCreatorViolationPropagates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tg := nextTelegramID.Add(1)

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer rollback(t, ctx, tx2)

	var preExisting int64
	err = tx2.QueryRow(ctx,
		`SELECT id FROM owner WHERE kind = $1 AND telegram_id = $2`, OwnerPlayer, tg,
	).Scan(&preExisting)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tx2 pre-check = %v, want pgx.ErrNoRows (tx1 has not committed yet)", err)
	}

	winner, created, err := EnsureOwner(ctx, tx1, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("tx1 EnsureOwner: %v", err)
	}
	if !created {
		t.Fatalf("tx1 created = %v, want true", created)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	if _, err := CreateOwner(ctx, tx2, OwnerPlayer, &tg); err == nil {
		t.Fatalf("tx2 CreateOwner after tx1 committed: want the unique violation, got nil")
	} else {
		sqlstate(t, err, "23505", "owner_kind_telegram_id_key")
	}
	if err := tx2.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback tx2: %v", err)
	}

	// A fresh attempt, in a fresh transaction, now finds the winner's row.
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer rollback(t, ctx, tx3)
	retried, created, err := EnsureOwner(ctx, tx3, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("tx3 EnsureOwner: %v", err)
	}
	if created {
		t.Fatalf("tx3 created = %v, want false (the row already exists)", created)
	}
	if retried.ID != winner.ID {
		t.Fatalf("tx3 EnsureOwner id = %d, want %d", retried.ID, winner.ID)
	}
}

// TestEnsureOwner_rejectionsUnchanged pins CreateOwner's own refusal
// table unchanged on this call, issuing no statement before the refusal.
func TestEnsureOwner_rejectionsUnchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	tg := nextTelegramID.Add(1)

	for _, tc := range []struct {
		name       string
		kind       OwnerKind
		telegramID *int64
	}{
		{"player_nil_telegram_id", OwnerPlayer, nil},
		{"chat_nil_telegram_id", OwnerChat, nil},
		{"world_with_telegram_id", OwnerWorld, &tg},
		{"unknown_kind", OwnerKind("corpse"), &tg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer rollback(t, ctx, tx)

			var before int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&before); err != nil {
				t.Fatalf("count owner before: %v", err)
			}

			_, created, err := EnsureOwner(ctx, tx, tc.kind, tc.telegramID)
			if !errors.Is(err, ErrInvalidOwner) {
				t.Fatalf("EnsureOwner(%s) = %v, want ErrInvalidOwner", tc.kind, err)
			}
			if created {
				t.Fatalf("EnsureOwner(%s) created = %v, want false", tc.kind, created)
			}

			var after int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&after); err != nil {
				t.Fatalf("count owner after: %v", err)
			}
			if after != before {
				t.Fatalf("owner count changed from %d to %d — a statement was issued", before, after)
			}
		})
	}
}
