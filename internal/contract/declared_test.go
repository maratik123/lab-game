package contract_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/contract"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestDeclared_builds(t *testing.T) {
	t.Parallel()

	if _, err := contract.Declared(); err != nil {
		t.Fatalf("Declared() error = %v, want nil", err)
	}
}

func TestDeclared_manualCorrectionAdmitsAnyBalancedSet(t *testing.T) {
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

	if err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe"},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := store.Move(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe mint"},
		[]store.Movement{{ItemID: store.NewItem, From: store.WorldHolder, To: backpack}},
	); err != nil {
		t.Fatalf("move: %v", err)
	}

	reg, err := contract.Declared()
	if err != nil {
		t.Fatalf("Declared() error = %v, want nil", err)
	}
	if err := reg.Check(ctx, tx, mark, contract.ManualCorrection()); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

func TestDeclared_refusesPlayerOperationWrite(t *testing.T) {
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

	if err := store.Post(ctx, tx, &store.PlayerOperation{Source: store.SourceTelegram, OperationID: "probe-op"},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-10)},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "probe"},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.NewFromInt(-1)},
		store.Posting{AccountID: playerMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}

	reg, err := contract.Declared()
	if err != nil {
		t.Fatalf("Declared() error = %v, want nil", err)
	}
	err = reg.Check(ctx, tx, mark, contract.ManualCorrection())
	if !errors.Is(err, contract.ErrNoSignature) {
		t.Fatalf("Check() error = %v, want errors.Is ErrNoSignature", err)
	}
}
