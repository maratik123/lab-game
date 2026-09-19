package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestPresence_writtenAndReadBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(5001)
	owner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	changed, err := SetPresence(ctx, tx, owner.ID, true)
	if err != nil {
		t.Fatalf("SetPresence: %v", err)
	}
	if !changed {
		t.Fatalf("changed = %v, want true (first write)", changed)
	}

	present, ok, err := Present(ctx, tx, owner.ID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if !ok || !present {
		t.Fatalf("Present = (%v, %v), want (true, true)", present, ok)
	}
}

func TestPresence_repeatWriteReportsNoChangeAndLeavesChangedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(5002)
	owner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	if _, err := SetPresence(ctx, tx, owner.ID, true); err != nil {
		t.Fatalf("first SetPresence: %v", err)
	}
	var firstChangedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT changed_at FROM chat_presence WHERE chat_id = $1`, owner.ID).Scan(&firstChangedAt); err != nil {
		t.Fatalf("read changed_at: %v", err)
	}

	changed, err := SetPresence(ctx, tx, owner.ID, true)
	if err != nil {
		t.Fatalf("repeat SetPresence: %v", err)
	}
	if changed {
		t.Fatalf("repeat write changed = %v, want false", changed)
	}
	var secondChangedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT changed_at FROM chat_presence WHERE chat_id = $1`, owner.ID).Scan(&secondChangedAt); err != nil {
		t.Fatalf("read changed_at: %v", err)
	}
	if !secondChangedAt.Equal(firstChangedAt) {
		t.Fatalf("changed_at moved on a no-op write: %v -> %v", firstChangedAt, secondChangedAt)
	}
}

func TestPresence_flipReportsChangeAndMovesChangedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)

	// Each write runs in its own, committed transaction: Postgres' own
	// now() is stable within one transaction (it is transaction_timestamp()
	// under the hood), so proving changed_at actually moves needs two
	// separate transactions, exactly as two separate my_chat_member
	// updates would produce in production.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	tg := int64(5003)
	owner, err := store.CreateOwner(ctx, tx1, store.OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	if _, err := SetPresence(ctx, tx1, owner.ID, true); err != nil {
		t.Fatalf("first SetPresence: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}
	var firstChangedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT changed_at FROM chat_presence WHERE chat_id = $1`, owner.ID).Scan(&firstChangedAt); err != nil {
		t.Fatalf("read changed_at: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	changed, err := SetPresence(ctx, tx2, owner.ID, false)
	if err != nil {
		t.Fatalf("flip SetPresence: %v", err)
	}
	if !changed {
		t.Fatalf("flip changed = %v, want true", changed)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit tx2: %v", err)
	}

	var present bool
	var secondChangedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT present, changed_at FROM chat_presence WHERE chat_id = $1`, owner.ID).Scan(&present, &secondChangedAt); err != nil {
		t.Fatalf("read presence: %v", err)
	}
	if present {
		t.Fatalf("present after flip = %v, want false", present)
	}
	if !secondChangedAt.After(firstChangedAt) {
		t.Fatalf("changed_at did not move forward on a flip: %v -> %v", firstChangedAt, secondChangedAt)
	}
}

func TestPresence_notAChatRefusedWritingNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(5004)
	player, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	_, err = SetPresence(ctx, tx, player.ID, true)
	if !errors.Is(err, ErrNotAChat) {
		t.Fatalf("SetPresence on a player owner = %v, want ErrNotAChat", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_presence WHERE chat_id = $1`, player.ID).Scan(&count); err != nil {
		t.Fatalf("count chat_presence: %v", err)
	}
	if count != 0 {
		t.Fatalf("chat_presence rows for a refused write = %d, want 0", count)
	}
}

func TestPresence_noRowIsNotFalse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(5005)
	owner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	_, ok, err := Present(ctx, tx, owner.ID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if ok {
		t.Fatalf("ok = %v, want false (no presence row written yet)", ok)
	}
}

// TestSetPresence_nonexistentChatIDRefused pins the branch a wrong-kind
// chat id cannot reach: a chat id with no owner row at all (rather than
// an owner row of the wrong kind) is disambiguated through
// pgx.ErrNoRows on the follow-up read, not through a kind mismatch.
func TestSetPresence_nonexistentChatIDRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = SetPresence(ctx, tx, store.OwnerID(999999999), true)
	if !errors.Is(err, ErrNotAChat) {
		t.Fatalf("SetPresence(nonexistent chat id) = %v, want ErrNotAChat", err)
	}
}

// TestPresent_closedPoolSurfacesError asserts a closed pool's error is
// returned rather than papered over as "no row" — a pool built directly
// here, rather than through the shared test-pool helper, avoids a double
// Close from that helper's own cleanup.
func TestPresent_closedPoolSurfacesError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := testdb.Schema(t)
	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	pool.Close()

	if _, _, err := Present(ctx, pool, store.OwnerID(1)); err == nil {
		t.Fatal("Present over a closed pool: want an error, got nil")
	}
}
