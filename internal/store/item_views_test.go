package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// itemHolderRow mirrors one row of the item_holder view.
type itemHolderRow struct {
	ItemID     ItemID
	HolderID   HolderID
	MovementID int64
}

// selectItemHolder reads item_id's item_holder row via q — a pgx.Tx or a
// *pgxpool.Pool, both of which satisfy Queryer.
func selectItemHolder(t *testing.T, ctx context.Context, q Queryer, itemID ItemID) *itemHolderRow {
	t.Helper()
	var r itemHolderRow
	err := q.QueryRow(ctx,
		`SELECT item_id, holder_id, movement_id FROM item_holder WHERE item_id = $1`, itemID,
	).Scan(&r.ItemID, &r.HolderID, &r.MovementID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		t.Fatalf("select item_holder for item %d: %v", itemID, err)
	}
	return &r
}

// countRows reports the row count of query, run via q — a pgx.Tx or a
// *pgxpool.Pool, both of which satisfy Queryer.
func countRows(t *testing.T, ctx context.Context, q Queryer, query string, args ...any) int {
	t.Helper()
	var n int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (`+query+`) s`, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func TestItemViews_healthyIsEmptyAndNotVacuous(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	_, backpackA := newPlayerWithBackpack(t, ctx, tx)
	_, backpackB := newPlayerWithBackpack(t, ctx, tx)
	grantSlots(t, ctx, tx, backpackA, 5)
	grantSlots(t, ctx, tx, backpackB, 5)

	ids, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "seed"},
		[]Movement{
			{ItemID: NewItem, From: WorldHolder, To: backpackA},
			{ItemID: NewItem, From: WorldHolder, To: backpackA},
		})
	if err != nil {
		t.Fatalf("seed Move: %v", err)
	}
	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "move"},
		[]Movement{{ItemID: ids[0], From: backpackA, To: backpackB}}); err != nil {
		t.Fatalf("move Move: %v", err)
	}

	// item_holder is non-empty first: an empty reconciliation cannot be an
	// empty corpus reporting clean.
	holderCount := countRows(t, ctx, tx, `SELECT * FROM item_holder`)
	if holderCount != 2 {
		t.Fatalf("item_holder rows = %d, want 2", holderCount)
	}

	if n := countRows(t, ctx, tx, `SELECT * FROM item_chain_break`); n != 0 {
		t.Fatalf("item_chain_break on a healthy tree = %d rows, want 0", n)
	}
	if n := countRows(t, ctx, tx, `SELECT * FROM item_capacity_divergence`); n != 0 {
		t.Fatalf("item_capacity_divergence on a healthy tree = %d rows, want 0", n)
	}
}

func TestItemViews_itemHolderFollowsMoves(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	_, backpackA := newPlayerWithBackpack(t, ctx, tx)
	_, backpackB := newPlayerWithBackpack(t, ctx, tx)
	grantSlots(t, ctx, tx, backpackA, 5)
	grantSlots(t, ctx, tx, backpackB, 5)

	ids, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "seed"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if err != nil {
		t.Fatalf("seed Move: %v", err)
	}
	item := ids[0]

	row := selectItemHolder(t, ctx, tx, item)
	if row == nil || row.HolderID != backpackA {
		t.Fatalf("item_holder after genesis = %+v, want holder %d", row, backpackA)
	}
	genesisMovement := row.MovementID

	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "forward"},
		[]Movement{{ItemID: item, From: backpackA, To: backpackB}}); err != nil {
		t.Fatalf("forward Move: %v", err)
	}
	row = selectItemHolder(t, ctx, tx, item)
	if row == nil || row.HolderID != backpackB || row.MovementID == genesisMovement {
		t.Fatalf("item_holder after forward move = %+v, want holder %d and a new movement", row, backpackB)
	}
	forwardMovement := row.MovementID

	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "back"},
		[]Movement{{ItemID: item, From: backpackB, To: backpackA}}); err != nil {
		t.Fatalf("back Move: %v", err)
	}
	row = selectItemHolder(t, ctx, tx, item)
	if row == nil || row.HolderID != backpackA || row.MovementID == forwardMovement {
		t.Fatalf("item_holder after move back = %+v, want holder %d and a new movement", row, backpackA)
	}
}

// TestItemViews_chainBreakDetectsAnomalies plants each anomaly class inside
// a transaction that drops the chain constraints item_movement_chain_fk
// and item_movement_successor_key, and rolls back — the instrument is
// shown red before its green (the healthy-tree test above) is believed.
func TestItemViews_chainBreakDetectsAnomalies(t *testing.T) {
	t.Parallel()

	t.Run("instance_with_no_movement", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		item := newBareItem(t, ctx, tx)

		var reached, recorded, head int
		if err := tx.QueryRow(ctx,
			`SELECT reached_movement, recorded_movement, head_movement FROM item_chain_break WHERE item_id = $1`, item,
		).Scan(&reached, &recorded, &head); err != nil {
			t.Fatalf("select item_chain_break for item with no movement: %v", err)
		}
		if reached != 0 || recorded != 0 || head != 0 {
			t.Fatalf("item_chain_break for a movement-less item = %d/%d/%d, want 0/0/0", reached, recorded, head)
		}
	})

	t.Run("orphaned_segment", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		if _, err := tx.Exec(ctx, `ALTER TABLE item_movement DROP CONSTRAINT item_movement_chain_fk`); err != nil {
			t.Fatalf("drop chain fk: %v", err)
		}

		item1 := newBareItem(t, ctx, tx)
		item2 := newBareItem(t, ctx, tx)
		je1 := newJournalEntry(t, ctx, tx)
		genesis1, err := insertMovement(ctx, tx, item1, nil, WorldHolder, backpackA, je1)
		if err != nil {
			t.Fatalf("insert genesis for item1: %v", err)
		}
		// An orphaned segment: item2's movement names item1's genesis as its
		// predecessor — refused at the database were the FK still present.
		je2 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item2, &genesis1, backpackA, WorldHolder, je2); err != nil {
			t.Fatalf("insert orphaned segment: %v", err)
		}

		if n := countRows(t, ctx, tx, `SELECT * FROM item_chain_break WHERE item_id = $1`, item2); n != 1 {
			t.Fatalf("item_chain_break for the orphaned segment's item = %d rows, want 1", n)
		}
	})

	t.Run("fork", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		_, backpackB := newPlayerWithBackpack(t, ctx, tx)
		// item_movement_successor_key is a unique INDEX, not a table
		// constraint — dropped directly.
		if _, err := tx.Exec(ctx, `DROP INDEX item_movement_successor_key`); err != nil {
			t.Fatalf("drop successor index: %v", err)
		}

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)
		genesis, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je)
		if err != nil {
			t.Fatalf("insert genesis: %v", err)
		}
		je2 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, &genesis, backpackA, backpackB, je2); err != nil {
			t.Fatalf("insert first successor: %v", err)
		}
		je3 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, &genesis, backpackA, WorldHolder, je3); err != nil {
			t.Fatalf("insert forked successor: %v", err)
		}

		if n := countRows(t, ctx, tx, `SELECT * FROM item_chain_break WHERE item_id = $1`, item); n != 1 {
			t.Fatalf("item_chain_break for the forked item = %d rows, want 1", n)
		}
	})

	t.Run("off_world_genesis", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		_, backpackB := newPlayerWithBackpack(t, ctx, tx)
		if _, err := tx.Exec(ctx, `ALTER TABLE item_movement DROP CONSTRAINT item_movement_genesis_from_world`); err != nil {
			t.Fatalf("drop genesis check: %v", err)
		}

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)
		// This item's only movement is a genesis whose from is not World:
		// the recursive walk never reaches it, so this is the
		// discriminator for the join-from-item / COALESCE fix — a
		// reached<>recorded comparison without it would report this
		// instance as healthy.
		if _, err := insertMovement(ctx, tx, item, nil, backpackA, backpackB, je); err != nil {
			t.Fatalf("insert off-World genesis: %v", err)
		}

		var reached, recorded, head int
		if err := tx.QueryRow(ctx,
			`SELECT reached_movement, recorded_movement, head_movement FROM item_chain_break WHERE item_id = $1`, item,
		).Scan(&reached, &recorded, &head); err != nil {
			t.Fatalf("select item_chain_break for the off-World genesis: %v", err)
		}
		if reached != 0 || recorded != 1 || head != 1 {
			t.Fatalf("item_chain_break for the off-World genesis = %d/%d/%d, want 0/1/1", reached, recorded, head)
		}
	})

	t.Run("second_genesis", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		if _, err := tx.Exec(ctx, `DROP INDEX item_movement_successor_key`); err != nil {
			t.Fatalf("drop successor index: %v", err)
		}

		item := newBareItem(t, ctx, tx)
		je1 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je1); err != nil {
			t.Fatalf("insert first genesis: %v", err)
		}
		je2 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je2); err != nil {
			t.Fatalf("insert second genesis: %v", err)
		}

		if n := countRows(t, ctx, tx, `SELECT * FROM item_chain_break WHERE item_id = $1`, item); n != 1 {
			t.Fatalf("item_chain_break for the double-genesis item = %d rows, want 1", n)
		}
	})
}

// TestItemViews_capacityDivergence plants each divergence class, and shows
// World's absence from both branches is the missing controlled balance,
// not a dead branch: an uncontrolled planted holder kind sits beside a
// controlled one in the same rolled-back transaction.
func TestItemViews_capacityDivergence(t *testing.T) {
	t.Parallel()

	t.Run("count_mismatch", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		grantSlots(t, ctx, tx, backpackA, 5)
		// A slots_used leg with no matching movement.
		_, used := slotsAccounts(t, ctx, tx, backpackA)
		_, worldUsed := slotsAccounts(t, ctx, tx, WorldHolder)
		if err := Post(ctx, tx, &ManualCorrection{Actor: "test", Reason: "plant divergence"},
			Posting{AccountID: used, Amount: decimal.NewFromInt(1)},
			Posting{AccountID: worldUsed, Amount: decimal.NewFromInt(-1)},
		); err != nil {
			t.Fatalf("plant divergence posting: %v", err)
		}

		var itemCount int
		var balance decimal.Decimal
		var reason string
		if err := tx.QueryRow(ctx,
			`SELECT item_count, slots_used_balance, reason FROM item_capacity_divergence WHERE holder_id = $1`, backpackA,
		).Scan(&itemCount, &balance, &reason); err != nil {
			t.Fatalf("select item_capacity_divergence: %v", err)
		}
		if itemCount != 0 || !balance.Equal(decimal.NewFromInt(1)) || reason != "count_mismatch" {
			t.Fatalf("item_capacity_divergence = count:%d balance:%s reason:%s, want 0/1/count_mismatch", itemCount, balance, reason)
		}
	})

	t.Run("no_slots_used_account_vs_uncontrolled_absent", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		// Controlled holder kind with NO slots_used account_definition at
		// all: its instances must be reported.
		if _, err := tx.Exec(ctx,
			`INSERT INTO scope_definition (id, code, owner_kind) VALUES (60, 'planted_no_used', 'player')`); err != nil {
			t.Fatalf("insert planted scope_definition (no used): %v", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled, capacity_role)
			 VALUES (600, 60, 'slots_free', 'slots', true, 'free')`); err != nil {
			t.Fatalf("insert planted account_definition (no used): %v", err)
		}

		// Uncontrolled holder kind — like World — with a full free/used
		// pair: its instances must NOT be reported at all.
		if _, err := tx.Exec(ctx,
			`INSERT INTO scope_definition (id, code, owner_kind) VALUES (61, 'planted_uncontrolled', 'player')`); err != nil {
			t.Fatalf("insert planted scope_definition (uncontrolled): %v", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled, capacity_role) VALUES
			 (610, 61, 'slots_free', 'slots', false, 'free'),
			 (611, 61, 'slots_used', 'slots', false, 'used')`); err != nil {
			t.Fatalf("insert planted account_definition (uncontrolled): %v", err)
		}

		tg1 := nextTelegramID.Add(1)
		owner1, err := CreateOwner(ctx, tx, OwnerPlayer, &tg1)
		if err != nil {
			t.Fatalf("CreateOwner owner1: %v", err)
		}
		tg2 := nextTelegramID.Add(1)
		owner2, err := CreateOwner(ctx, tx, OwnerPlayer, &tg2)
		if err != nil {
			t.Fatalf("CreateOwner owner2: %v", err)
		}
		var noUsedScope, uncontrolledScope int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM scope WHERE owner_id = $1 AND scope_definition_id = 60`, owner1.ID,
		).Scan(&noUsedScope); err != nil {
			t.Fatalf("select no-used scope: %v", err)
		}
		if err := tx.QueryRow(ctx,
			`SELECT id FROM scope WHERE owner_id = $1 AND scope_definition_id = 61`, owner2.ID,
		).Scan(&uncontrolledScope); err != nil {
			t.Fatalf("select uncontrolled scope: %v", err)
		}
		noUsedHolder := HolderID(noUsedScope)
		uncontrolledHolder := HolderID(uncontrolledScope)

		// Genesis movements directly into each planted holder (no capacity
		// posting at all — this test is about the account's existence, not
		// its balance).
		itemNoUsed := newBareItem(t, ctx, tx)
		je1 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, itemNoUsed, nil, WorldHolder, noUsedHolder, je1); err != nil {
			t.Fatalf("insert genesis into no-used holder: %v", err)
		}
		itemUncontrolled := newBareItem(t, ctx, tx)
		je2 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, itemUncontrolled, nil, WorldHolder, uncontrolledHolder, je2); err != nil {
			t.Fatalf("insert genesis into uncontrolled holder: %v", err)
		}

		var reason string
		if err := tx.QueryRow(ctx,
			`SELECT reason FROM item_capacity_divergence WHERE holder_id = $1`, noUsedHolder,
		).Scan(&reason); err != nil {
			t.Fatalf("select item_capacity_divergence for no-used holder: %v", err)
		}
		if reason != "no_slots_used_account" {
			t.Fatalf("no-used holder reason = %s, want no_slots_used_account", reason)
		}

		if n := countRows(t, ctx, tx, `SELECT * FROM item_capacity_divergence WHERE holder_id = $1`, uncontrolledHolder); n != 0 {
			t.Fatalf("uncontrolled holder rows in item_capacity_divergence = %d, want 0 (absent like World)", n)
		}
	})

	t.Run("world_absent_as_genesis_source", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		grantSlots(t, ctx, tx, backpackA, 5)
		if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "mint"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}}); err != nil {
			t.Fatalf("mint Move: %v", err)
		}

		if n := countRows(t, ctx, tx, `SELECT * FROM item_capacity_divergence WHERE holder_id = $1`, WorldHolder); n != 0 {
			t.Fatalf("World rows in item_capacity_divergence = %d, want 0", n)
		}
	})
}
