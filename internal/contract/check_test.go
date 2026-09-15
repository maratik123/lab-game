package contract_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/contract"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestNewMark_emptyJournal(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark() on an empty journal: %v", err)
	}

	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
	fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(10))

	reg := testRegistry(t)
	if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
		t.Errorf("Check() = %v, want nil (the first entry must be inside the window)", err)
	}
}

func TestCheck_passesExactSignature(t *testing.T) {
	t.Parallel()

	t.Run("postings-only write", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}

		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale)); err != nil {
			t.Errorf("Check() = %v, want nil", err)
		}
	})

	t.Run("move mint", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		grantSlots(t, ctx, tx, player.ID, "backpack", 10)
		backpack := holderID(t, ctx, tx, player.ID, "backpack")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}

		if _, err := store.Move(ctx, tx, &store.DeferredTask{TaskType: "probe_grant", RunAt: time.Now()},
			[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
		); err != nil {
			t.Fatalf("move: %v", err)
		}

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.DeferredTask("probe_grant")); err != nil {
			t.Errorf("Check() = %v, want nil", err)
		}
	})
}

func TestCheck_failsOnPostingDifference(t *testing.T) {
	t.Parallel()

	t.Run("account_definition", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player1 := newPlayer(t, ctx, tx)
		player2 := newPlayer(t, ctx, tx)
		player1Money := accountID(t, ctx, tx, player1.ID, "attributes", "money")
		player2Money := accountID(t, ctx, tx, player2.ID, "attributes", "money")
		fund(t, ctx, tx, player1Money, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		// The credit leg goes to a second player's attributes/money
		// instead of world/money.
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: player1Money, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: player2Money, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("sign", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("cardinality", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		// Two debits of the player instead of one.
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-4)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-6)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("kind", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		// A registry that declares attributes/money with KindExperience
		// instead of the real KindMoney: NewRegistry accepts it because
		// contract reads no catalog.
		reg, err := contract.NewRegistry(contract.Declaration{
			Type: contract.Event(store.EventShopSale),
			Signature: contract.Expect(
				contract.PostingLeg("attributes", "money", store.KindExperience, contract.Negative, contract.Exactly(1)),
				contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(1)),
			),
		})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})
}

func TestCheck_failsOnMovementDifference(t *testing.T) {
	t.Parallel()

	t.Run("two mints against Exactly(1)", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		grantSlots(t, ctx, tx, player.ID, "backpack", 10)
		backpack := holderID(t, ctx, tx, player.ID, "backpack")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if _, err := store.Move(ctx, tx, &store.DeferredTask{TaskType: "probe_grant", RunAt: time.Now()},
			[]store.Movement{
				{ItemID: store.NewItem, From: store.WorldHolder, To: backpack},
				{ItemID: store.NewItem, From: store.WorldHolder, To: backpack},
			},
		); err != nil {
			t.Fatalf("move: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.DeferredTask("probe_grant"))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("minted before the mark, then moved after it", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		grantSlots(t, ctx, tx, player.ID, "backpack", 10)
		backpack := holderID(t, ctx, tx, player.ID, "backpack")

		ids, err := store.Move(ctx, tx, &store.DeferredTask{TaskType: "probe_grant", RunAt: time.Now()},
			[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
		)
		if err != nil {
			t.Fatalf("move (before mark): %v", err)
		}

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}

		if _, err := store.Move(ctx, tx, &store.DeferredTask{TaskType: "probe_grant", RunAt: time.Now()},
			[]store.Movement{{ItemID: ids[0], From: backpack, To: store.WorldHolder}},
		); err != nil {
			t.Fatalf("move (after mark): %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.DeferredTask("probe_grant"))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})
}

func TestCheck_failsOnUndeclaredWrite(t *testing.T) {
	t.Parallel()

	t.Run("an undeclared experience pair beside a shop_sale", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		playerExperience := accountID(t, ctx, tx, player.ID, "attributes", "experience")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))
		fund(t, ctx, tx, playerExperience, store.KindExperience, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
			store.Posting{AccountID: playerExperience, Amount: decimal.NewFromInt(-5)},
			store.Posting{AccountID: store.WorldExperience, Amount: decimal.NewFromInt(5)},
		); err != nil {
			t.Fatalf("post shop_sale: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("a move under a type that declares capacity legs but no movement leg", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		grantSlots(t, ctx, tx, player.ID, "backpack", 10)
		backpack := holderID(t, ctx, tx, player.ID, "backpack")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if _, err := store.Move(ctx, tx, &store.DeferredTask{TaskType: "no_movement_leg", RunAt: time.Now()},
			[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
		); err != nil {
			t.Fatalf("move: %v", err)
		}

		reg, err := contract.NewRegistry(contract.Declaration{
			Type: contract.DeferredTask("no_movement_leg"),
			Signature: contract.Expect(
				contract.PostingLeg("world", "slots_free", store.KindSlots, contract.Positive, contract.Exactly(1)),
				contract.PostingLeg("world", "slots_used", store.KindSlots, contract.Negative, contract.Exactly(1)),
				contract.PostingLeg("backpack", "slots_free", store.KindSlots, contract.Negative, contract.Exactly(1)),
				contract.PostingLeg("backpack", "slots_used", store.KindSlots, contract.Positive, contract.Exactly(1)),
			),
		})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		err = reg.Check(ctx, tx, mark, contract.DeferredTask("no_movement_leg"))
		if !errors.Is(err, contract.ErrNonconforming) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
		}
	})
}

func TestCheck_judgesByOwnType(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)
	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
	fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}
	// shop_sale-shaped postings written under shop_purchase.
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopPurchase},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}

	reg := testRegistry(t)

	if err := reg.Check(ctx, tx, mark, contract.Event(store.EventShopPurchase)); !errors.Is(err, contract.ErrNonconforming) {
		t.Errorf("Check(want=shop_purchase) error = %v, want errors.Is ErrNonconforming (judged by shop_purchase's own signature)", err)
	}

	mark2, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := reg.Check(ctx, tx, mark2, contract.Event(store.EventShopSale)); err != nil {
		t.Errorf("Check(want=shop_sale, after mark2) = %v, want nil (the control)", err)
	}

	// A third window holds the shop_sale-shaped shop_purchase entry
	// beside a conforming shop_sale entry, checked with want = shop_sale.
	// Own-type judging judges the shop_purchase entry by its own
	// shop_purchase signature and reports ErrNonconforming for it; a
	// Check that instead judged every entry by want's signature would
	// pass it, because the postings are shop_sale-shaped and want is
	// shop_sale.
	mark3, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopPurchase},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}
	err = reg.Check(ctx, tx, mark3, contract.Event(store.EventShopSale))
	if !errors.Is(err, contract.ErrNonconforming) {
		t.Errorf("Check(want=shop_sale, mixed window) error = %v, want errors.Is ErrNonconforming (the shop_purchase entry judged by its own type)", err)
	}
	if err != nil && !strings.Contains(err.Error(), "shop_purchase") {
		t.Errorf("Check(want=shop_sale, mixed window) error = %v, want it to name the shop_purchase entry", err)
	}
}

func TestCheck_unsignedTypeFails(t *testing.T) {
	t.Parallel()

	t.Run("event_type", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventNodeEntered},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		assertUnsigned(t, err, "node_entered")
	})

	t.Run("task_type", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.DeferredTask{TaskType: "unsigned", RunAt: time.Now()},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		assertUnsigned(t, err, "unsigned")
	})

	t.Run("player_operation", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.PlayerOperation{Source: store.SourceTelegram, OperationID: "probe-op"},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		assertUnsigned(t, err, "player_operation")
	})

	t.Run("unknown_basis", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}

		if _, err := tx.Exec(ctx, `CREATE TABLE probe_basis (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY)`); err != nil {
			t.Fatalf("create probe_basis: %v", err)
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE journal_entry ADD COLUMN probe_id bigint REFERENCES probe_basis (id)`); err != nil {
			t.Fatalf("alter journal_entry: %v", err)
		}
		if _, err := tx.Exec(ctx, `
ALTER TABLE journal_entry
    DROP CONSTRAINT journal_entry_exactly_one_basis,
    ADD  CONSTRAINT journal_entry_exactly_one_basis
         CHECK (num_nonnulls(player_operation_id, manual_correction_id,
                              deferred_task_id, recurrent_task_id, event_id, probe_id) = 1)`); err != nil {
			t.Fatalf("replace constraint: %v", err)
		}
		var probeID, entryID int64
		if err := tx.QueryRow(ctx, `INSERT INTO probe_basis DEFAULT VALUES RETURNING id`).Scan(&probeID); err != nil {
			t.Fatalf("insert probe_basis: %v", err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO journal_entry (probe_id) VALUES ($1) RETURNING id`, probeID).Scan(&entryID); err != nil {
			t.Fatalf("insert journal_entry: %v", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, $2, $3), ($1, $4, $5)`,
			entryID, playerMoney, decimal.NewFromInt(10), store.WorldMoney, decimal.NewFromInt(-10)); err != nil {
			t.Fatalf("insert posting: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
		assertUnsigned(t, err, "unknown_basis")
	})

	t.Run("want itself has no declared signature", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		reg := testRegistry(t)
		err = reg.Check(ctx, tx, mark, contract.Event(store.EventNodeEntered))
		if !errors.Is(err, contract.ErrNoSignature) {
			t.Fatalf("Check() error = %v, want errors.Is ErrNoSignature", err)
		}
	})
}

// assertUnsigned asserts err matches ErrNoSignature (naming want), and
// matches neither ErrNonconforming nor ErrNoDocument — the shop_sale
// entry written alongside the unsigned one keeps ErrNoDocument out.
func assertUnsigned(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if !errors.Is(err, contract.ErrNoSignature) {
		t.Fatalf("Check() error = %v, want errors.Is ErrNoSignature", err)
	}
	if err == nil || !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("Check() error = %v, want it to name %q", err, wantSubstr)
	}
	if errors.Is(err, contract.ErrNonconforming) {
		t.Errorf("Check() error = %v, want it not to match ErrNonconforming", err)
	}
}

func TestCheck_joinsEveryClass(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)
	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
	fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventNodeEntered},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post node_entered: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopPurchase},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post shop_purchase: %v", err)
	}

	reg := testRegistry(t)
	err = reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale))
	for _, sentinel := range []error{contract.ErrNoDocument, contract.ErrNoSignature, contract.ErrNonconforming} {
		if !errors.Is(err, sentinel) {
			t.Errorf("Check() error = %v, want errors.Is %v", err, sentinel)
		}
	}
}

func TestCheck_manualCorrectionAdmitsAnyBalancedSet(t *testing.T) {
	t.Parallel()

	t.Run("money pair", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe"},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
			t.Errorf("Check(want=ManualCorrection) = %v, want nil", err)
		}
		// The window's only entry is the manual correction above, which
		// conforms under its own type — so checking a different want
		// reports only that no entry of that type exists.
		if err := reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale)); !errors.Is(err, contract.ErrNoDocument) {
			t.Errorf("Check(want=shop_sale) error = %v, want errors.Is ErrNoDocument", err)
		}

		// The same shape, posted as a shop_sale event instead of a
		// manual correction: the direction (world debited, player
		// credited) is the direction shop_sale's own signature does not
		// declare, so own-type judging must reject it.
		mark2, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := reg.Check(ctx, tx, mark2, contract.Event(store.EventShopSale)); !errors.Is(err, contract.ErrNonconforming) {
			t.Errorf("Check(want=shop_sale, same shape as a shop_sale entry) error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("money and experience pairs together", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		playerExperience := accountID(t, ctx, tx, player.ID, "attributes", "experience")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe"},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
			store.Posting{AccountID: store.WorldExperience, Amount: decimal.NewFromInt(-5)},
			store.Posting{AccountID: playerExperience, Amount: decimal.NewFromInt(5)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
			t.Errorf("Check(want=ManualCorrection) = %v, want nil", err)
		}

		// The same shape, posted as a shop_sale event: the extra
		// experience legs, undeclared by shop_sale's signature, must be
		// rejected.
		mark2, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
			store.Posting{AccountID: store.WorldExperience, Amount: decimal.NewFromInt(-5)},
			store.Posting{AccountID: playerExperience, Amount: decimal.NewFromInt(5)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := reg.Check(ctx, tx, mark2, contract.Event(store.EventShopSale)); !errors.Is(err, contract.ErrNonconforming) {
			t.Errorf("Check(want=shop_sale, same shape as a shop_sale entry) error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("a slot grant", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		grantSlots(t, ctx, tx, player.ID, "backpack", 5)

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
			t.Errorf("Check(want=ManualCorrection) = %v, want nil", err)
		}

		// The same shape, posted as a shop_sale event: shop_sale's
		// signature declares no slot legs, so the postings must be
		// rejected.
		mark2, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		free := accountID(t, ctx, tx, player.ID, "backpack", "slots_free")
		worldFree := accountID(t, ctx, tx, worldOwnerID, "world", "slots_free")
		if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
			store.Posting{AccountID: free, Amount: decimal.NewFromInt(5)},
			store.Posting{AccountID: worldFree, Amount: decimal.NewFromInt(-5)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		if err := reg.Check(ctx, tx, mark2, contract.Event(store.EventShopSale)); !errors.Is(err, contract.ErrNonconforming) {
			t.Errorf("Check(want=shop_sale, same shape as a shop_sale entry) error = %v, want errors.Is ErrNonconforming", err)
		}
	})

	t.Run("a move mint beside a money pair", func(t *testing.T) {
		t.Parallel()
		pool := storetest.Pool(t)
		ctx := context.Background()
		tx := beginTx(t, ctx, pool)
		player := newPlayer(t, ctx, tx)
		playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
		grantSlots(t, ctx, tx, player.ID, "backpack", 5)
		backpack := holderID(t, ctx, tx, player.ID, "backpack")

		mark, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if _, err := store.Move(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe"},
			[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("move: %v", err)
		}

		reg := testRegistry(t)
		if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
			t.Errorf("Check(want=ManualCorrection) = %v, want nil", err)
		}

		// The same shape, posted as a shop_sale event: shop_sale's
		// signature declares no movement leg and no world/money leg, so
		// the mint-plus-money-pair set must be rejected.
		mark2, err := contract.NewMark(ctx, tx)
		if err != nil {
			t.Fatalf("NewMark: %v", err)
		}
		if _, err := store.Move(ctx, tx, &store.Event{Type: store.EventShopSale},
			[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
			store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
			store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
		); err != nil {
			t.Fatalf("move: %v", err)
		}
		if err := reg.Check(ctx, tx, mark2, contract.Event(store.EventShopSale)); !errors.Is(err, contract.ErrNonconforming) {
			t.Errorf("Check(want=shop_sale, same shape as a shop_sale entry) error = %v, want errors.Is ErrNonconforming", err)
		}
	})
}

func TestCheck_manualCorrectionRefusesUnbalancedSet(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)
	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}
	var entryID, correctionID int64
	if err := tx.QueryRow(ctx, `INSERT INTO manual_correction (actor, reason) VALUES ($1, $2) RETURNING id`,
		"test", "unbalanced probe").Scan(&correctionID); err != nil {
		t.Fatalf("insert manual_correction: %v", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO journal_entry (manual_correction_id) VALUES ($1) RETURNING id`,
		correctionID).Scan(&entryID); err != nil {
		t.Fatalf("insert journal_entry: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, $2, $3)`,
		entryID, playerMoney, decimal.NewFromInt(10)); err != nil {
		t.Fatalf("insert unbalanced posting: %v", err)
	}

	reg := testRegistry(t)
	err = reg.Check(ctx, tx, mark, contract.ManualCorrection())
	if !errors.Is(err, contract.ErrNonconforming) {
		t.Fatalf("Check() error = %v, want errors.Is ErrNonconforming", err)
	}
}

func TestCheck_ignoresEntriesBeforeMark(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)
	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
	fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

	if err := store.Post(ctx, tx, &store.Event{Type: store.EventNodeEntered},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-1)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("post node_entered (before mark): %v", err)
	}

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}

	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post shop_sale (after mark): %v", err)
	}

	reg := testRegistry(t)
	if err := reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale)); err != nil {
		t.Errorf("Check() = %v, want nil (the unsigned node_entered entry is before the mark)", err)
	}
}

func TestCheck_noDocumentOfWantedType(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	_ = newPlayer(t, ctx, tx)

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}

	reg := testRegistry(t)
	err = reg.Check(ctx, tx, mark, contract.ManualCorrection())
	if !errors.Is(err, contract.ErrNoDocument) {
		t.Fatalf("Check() error = %v, want errors.Is ErrNoDocument", err)
	}
}

func TestCheck_rowsWrittenInsideReleasedSavepoint(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)
	player := newPlayer(t, ctx, tx)
	playerMoney := accountID(t, ctx, tx, player.ID, "attributes", "money")
	fund(t, ctx, tx, playerMoney, store.KindMoney, decimal.NewFromInt(100))

	mark, err := contract.NewMark(ctx, tx)
	if err != nil {
		t.Fatalf("NewMark: %v", err)
	}

	if _, err := tx.Exec(ctx, `SAVEPOINT probe`); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	if err := store.Post(ctx, tx, &store.Event{Type: store.EventShopSale},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post shop_sale: %v", err)
	}
	if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT probe`); err != nil {
		t.Fatalf("release savepoint: %v", err)
	}

	reg := testRegistry(t)
	if err := reg.Check(ctx, tx, mark, contract.Event(store.EventShopSale)); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

func TestCheck_cancelledContext(t *testing.T) {
	t.Parallel()
	pool := storetest.Pool(t)
	ctx := context.Background()
	tx := beginTx(t, ctx, pool)

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := contract.NewMark(cancelCtx, tx); err == nil {
		t.Error("NewMark() with a cancelled context = nil, want an error")
	} else {
		for _, sentinel := range []error{contract.ErrInvalidDeclaration, contract.ErrNonconforming, contract.ErrNoDocument, contract.ErrNoSignature} {
			if errors.Is(err, sentinel) {
				t.Errorf("NewMark() error = %v, want it not to match %v", err, sentinel)
			}
		}
	}

	reg := testRegistry(t)
	if err := reg.Check(cancelCtx, tx, contract.Mark{}, contract.ManualCorrection()); err == nil {
		t.Error("Check() with a cancelled context = nil, want an error")
	} else {
		for _, sentinel := range []error{contract.ErrInvalidDeclaration, contract.ErrNonconforming, contract.ErrNoDocument, contract.ErrNoSignature} {
			if errors.Is(err, sentinel) {
				t.Errorf("Check() error = %v, want it not to match %v", err, sentinel)
			}
		}
	}
}
