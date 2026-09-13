package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// sqlstate asserts err wraps a *pgconn.PgError with the given SQLSTATE code
// (and, when constraint is non-empty, the given constraint name).
func sqlstate(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error %v does not wrap a *pgconn.PgError", err)
	}
	if pgErr.Code != code {
		t.Fatalf("SQLSTATE = %s, want %s (error: %v)", pgErr.Code, code, err)
	}
	if constraint != "" && pgErr.ConstraintName != constraint {
		t.Fatalf("constraint = %s, want %s (error: %v)", pgErr.ConstraintName, constraint, err)
	}
}

// rollback rolls tx back and fails the test on any error other than
// pgx.ErrTxClosed (already rolled back/committed).
func rollback(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("rollback: %v", err)
	}
}

func TestSchema_constraints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	t.Run("journal_entry_zero_bases", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx, `INSERT INTO journal_entry DEFAULT VALUES`)
		sqlstate(t, err, "23514", "journal_entry_exactly_one_basis")
	})

	t.Run("journal_entry_two_bases", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		var poID, mcID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'x1') RETURNING id`,
		).Scan(&poID); err != nil {
			t.Fatalf("insert player_operation: %v", err)
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO manual_correction (actor, reason) VALUES ('test', 'test') RETURNING id`,
		).Scan(&mcID); err != nil {
			t.Fatalf("insert manual_correction: %v", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO journal_entry (player_operation_id, manual_correction_id) VALUES ($1, $2)`, poID, mcID)
		sqlstate(t, err, "23514", "journal_entry_exactly_one_basis")
	})

	t.Run("journal_entry_second_for_same_document", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		var poID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'x2') RETURNING id`,
		).Scan(&poID); err != nil {
			t.Fatalf("insert player_operation: %v", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO journal_entry (player_operation_id) VALUES ($1)`, poID); err != nil {
			t.Fatalf("first entry: %v", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO journal_entry (player_operation_id) VALUES ($1)`, poID)
		sqlstate(t, err, "23505", "journal_entry_player_operation_key")
	})

	t.Run("journal_entry_event_and_player_operation", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		var poID, eventID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'arc-1') RETURNING id`,
		).Scan(&poID); err != nil {
			t.Fatalf("insert player_operation: %v", err)
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO event (type, payload) VALUES ('raid_started', '{}') RETURNING id`,
		).Scan(&eventID); err != nil {
			t.Fatalf("insert event: %v", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO journal_entry (player_operation_id, event_id) VALUES ($1, $2)`, poID, eventID)
		sqlstate(t, err, "23514", "journal_entry_exactly_one_basis")
	})

	t.Run("journal_entry_second_for_same_event", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		var eventID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO event (type, payload) VALUES ('raid_started', '{}') RETURNING id`,
		).Scan(&eventID); err != nil {
			t.Fatalf("insert event: %v", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO journal_entry (event_id) VALUES ($1)`, eventID); err != nil {
			t.Fatalf("first entry: %v", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO journal_entry (event_id) VALUES ($1)`, eventID)
		sqlstate(t, err, "23505", "journal_entry_event_key")
	})

	t.Run("second_world_owner", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx, `INSERT INTO owner (kind) VALUES ('world')`)
		sqlstate(t, err, "23505", "owner_single_world_key")
	})

	t.Run("duplicate_player_telegram_id", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('player', 42)`); err != nil {
			t.Fatalf("first player: %v", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('player', 42)`)
		sqlstate(t, err, "23505", "owner_kind_telegram_id_key")
	})

	t.Run("chat_and_player_may_share_a_telegram_id", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('player', 43)`); err != nil {
			t.Fatalf("player: %v", err)
		}
		// A chat with the same telegram_id is a different (kind, telegram_id) pair.
		if _, err := tx.Exec(ctx, `INSERT INTO owner (kind, telegram_id) VALUES ('chat', 43)`); err != nil {
			t.Fatalf("chat with same telegram_id should succeed: %v", err)
		}
	})

	t.Run("player_operation_replay", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		if _, err := tx.Exec(ctx,
			`INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'u1')`); err != nil {
			t.Fatalf("first insert: %v", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'u1')`)
		sqlstate(t, err, "23505", "player_operation_source_operation_id_key")
	})

	t.Run("posting_zero_amount", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		var poID, entryID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO player_operation (source, operation_id) VALUES ('telegram', 'x3') RETURNING id`,
		).Scan(&poID); err != nil {
			t.Fatalf("insert player_operation: %v", err)
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO journal_entry (player_operation_id) VALUES ($1) RETURNING id`, poID,
		).Scan(&entryID); err != nil {
			t.Fatalf("insert journal_entry: %v", err)
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, 1, 0)`, entryID)
		sqlstate(t, err, "23514", "posting_amount_nonzero")
	})

	t.Run("unknown_enum_labels", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name string
			sql  string
		}{
			{"owner_kind", `SELECT 'nope'::owner_kind`},
			{"ledger_kind", `SELECT 'nope'::ledger_kind`},
			{"operation_source", `SELECT 'nope'::operation_source`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatalf("begin: %v", err)
				}
				defer rollback(t, ctx, tx)

				_, err = tx.Exec(ctx, tc.sql)
				sqlstate(t, err, "22P02", "")
			})
		}
	})

	t.Run("explicit_id_into_generated_always", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx, `INSERT INTO player_operation (id, source, operation_id) VALUES (999, 'telegram', 'x4')`)
		sqlstate(t, err, "428C9", "")
	})

	// ingest_offset is a guarded singleton, seeded by the
	// migration itself — a second row is refused by the CHECK (id = 1)
	// working alongside the primary key, not merely by the primary key
	// alone (a second id could otherwise coexist).
	t.Run("ingest_offset_second_row_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx, `INSERT INTO ingest_offset (id, next_update_id) VALUES (2, 0)`)
		sqlstate(t, err, "23514", "ingest_offset_singleton")
	})

	t.Run("ingest_offset_seeded_exactly_once", func(t *testing.T) {
		t.Parallel()

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM ingest_offset`).Scan(&count); err != nil {
			t.Fatalf("count ingest_offset: %v", err)
		}
		if count != 1 {
			t.Fatalf("ingest_offset rows = %d, want exactly 1 (the seeded singleton)", count)
		}
		var nextUpdateID int64
		if err := pool.QueryRow(ctx, `SELECT next_update_id FROM ingest_offset WHERE id = 1`).Scan(&nextUpdateID); err != nil {
			t.Fatalf("read seeded next_update_id: %v", err)
		}
		if nextUpdateID != 0 {
			t.Fatalf("seeded next_update_id = %d, want 0", nextUpdateID)
		}
	})

	// A give-up row with an empty kind is refused.
	t.Run("ingest_dead_update_empty_kind_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx,
			`INSERT INTO ingest_dead_update (update_id, kind, consecutive_failures, last_error) VALUES (1, '', 1, 'boom')`)
		sqlstate(t, err, "23514", "ingest_dead_update_kind_nonempty")
	})
}

// newJournalEntry inserts a throwaway ManualCorrection-basis journal_entry
// and returns its id, for a test that needs a valid journal_entry_id
// without exercising Post itself.
func newJournalEntry(t *testing.T, ctx context.Context, tx pgx.Tx) int64 {
	t.Helper()
	var mcID, jeID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO manual_correction (actor, reason) VALUES ('test', 'item schema') RETURNING id`,
	).Scan(&mcID); err != nil {
		t.Fatalf("insert manual_correction: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO journal_entry (manual_correction_id) VALUES ($1) RETURNING id`, mcID,
	).Scan(&jeID); err != nil {
		t.Fatalf("insert journal_entry: %v", err)
	}
	return jeID
}

// backpackHolder returns the given owner's backpack scope id.
func backpackHolder(t *testing.T, ctx context.Context, tx pgx.Tx, owner OwnerID) HolderID {
	t.Helper()
	var id int64
	if err := tx.QueryRow(ctx,
		`SELECT id FROM scope WHERE owner_id = $1 AND scope_definition_id = 3`, owner,
	).Scan(&id); err != nil {
		t.Fatalf("select backpack scope for owner %d: %v", owner, err)
	}
	return HolderID(id)
}

// newBareItem inserts a fresh item row with no movement, for a test that
// builds an item_movement fixture by raw SQL.
func newBareItem(t *testing.T, ctx context.Context, tx pgx.Tx) ItemID {
	t.Helper()
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO item DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	return ItemID(id)
}

// insertMovement issues a raw item_movement INSERT and returns its id on
// success — the low-level entry point subtask 2's refusal scenarios drive
// directly, bypassing Move so each constraint is exercised in isolation.
func insertMovement(ctx context.Context, tx pgx.Tx, itemID ItemID, prev *int64, from, to HolderID, journalEntryID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO item_movement (item_id, prev_movement_id, from_holder_id, to_holder_id, journal_entry_id)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		itemID, prev, from, to, journalEntryID,
	).Scan(&id)
	return id, err
}

// TestSchema_itemMachineConstraints asserts item_movement's declarative
// chain refusals by SQLSTATE and constraint name, and the capacity
// catalog's own uniqueness rule.
func TestSchema_itemMachineConstraints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	t.Run("same_holder_move_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)

		_, err = insertMovement(ctx, tx, item, nil, WorldHolder, WorldHolder, je)
		sqlstate(t, err, "23514", "item_movement_holders_differ")
	})

	t.Run("genesis_not_from_world_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		tg := nextTelegramID.Add(1)
		owner, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpack := backpackHolder(t, ctx, tx, owner.ID)

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)

		_, err = insertMovement(ctx, tx, item, nil, backpack, WorldHolder, je)
		sqlstate(t, err, "23514", "item_movement_genesis_from_world")
	})

	t.Run("successor_from_disagrees_with_predecessor_to", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		tg := nextTelegramID.Add(1)
		ownerA, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpackA := backpackHolder(t, ctx, tx, ownerA.ID)

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)
		genesisID, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je)
		if err != nil {
			t.Fatalf("insert genesis movement: %v", err)
		}

		je2 := newJournalEntry(t, ctx, tx)
		// from is WorldHolder, but the predecessor's to was backpackA — the
		// chain FK has nothing to reference.
		_, err = insertMovement(ctx, tx, item, &genesisID, WorldHolder, backpackA, je2)
		sqlstate(t, err, "23503", "item_movement_chain_fk")
	})

	t.Run("successor_names_predecessor_of_a_different_instance", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		tg := nextTelegramID.Add(1)
		ownerA, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpackA := backpackHolder(t, ctx, tx, ownerA.ID)

		item1 := newBareItem(t, ctx, tx)
		item2 := newBareItem(t, ctx, tx)
		je1 := newJournalEntry(t, ctx, tx)
		genesis1ID, err := insertMovement(ctx, tx, item1, nil, WorldHolder, backpackA, je1)
		if err != nil {
			t.Fatalf("insert genesis movement for item1: %v", err)
		}

		je2 := newJournalEntry(t, ctx, tx)
		// genesis1ID belongs to item1, not item2 — no referenced row exists
		// with (id=genesis1ID, item_id=item2, to_holder_id=backpackA).
		_, err = insertMovement(ctx, tx, item2, &genesis1ID, backpackA, WorldHolder, je2)
		sqlstate(t, err, "23503", "item_movement_chain_fk")
	})

	t.Run("fork_second_successor_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		tg := nextTelegramID.Add(1)
		ownerA, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpackA := backpackHolder(t, ctx, tx, ownerA.ID)
		tg2 := nextTelegramID.Add(1)
		ownerB, err := CreateOwner(ctx, tx, OwnerPlayer, &tg2)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpackB := backpackHolder(t, ctx, tx, ownerB.ID)

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)
		genesisID, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je)
		if err != nil {
			t.Fatalf("insert genesis movement: %v", err)
		}

		je2 := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, &genesisID, backpackA, backpackB, je2); err != nil {
			t.Fatalf("insert first successor: %v", err)
		}

		je3 := newJournalEntry(t, ctx, tx)
		_, err = insertMovement(ctx, tx, item, &genesisID, backpackA, WorldHolder, je3)
		sqlstate(t, err, "23505", "item_movement_successor_key")
	})

	t.Run("second_genesis_for_one_instance_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		tg := nextTelegramID.Add(1)
		ownerA, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
		if err != nil {
			t.Fatalf("CreateOwner: %v", err)
		}
		backpackA := backpackHolder(t, ctx, tx, ownerA.ID)

		item := newBareItem(t, ctx, tx)
		je := newJournalEntry(t, ctx, tx)
		if _, err := insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je); err != nil {
			t.Fatalf("insert first genesis: %v", err)
		}

		je2 := newJournalEntry(t, ctx, tx)
		_, err = insertMovement(ctx, tx, item, nil, WorldHolder, backpackA, je2)
		sqlstate(t, err, "23505", "item_movement_successor_key")
	})

	t.Run("duplicate_capacity_role_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx,
			`INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled, capacity_role)
			 VALUES (999, 1, 'slots_free_dup', 'slots', false, 'free')`)
		sqlstate(t, err, "23505", "account_definition_capacity_role_key")
	})

	t.Run("explicit_id_into_item_refused", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = tx.Exec(ctx, `INSERT INTO item (id) VALUES (999)`)
		sqlstate(t, err, "428C9", "")
	})
}
