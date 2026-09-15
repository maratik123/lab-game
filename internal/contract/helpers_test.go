package contract_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/contract"
	"github.com/maratik123/lab-game/internal/store"
)

// beginTx starts a transaction on pool and rolls it back on tb's
// cleanup, so every database test leaves no trace for the next.
func beginTx(tb testing.TB, ctx context.Context, pool *pgxpool.Pool) pgx.Tx {
	tb.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		tb.Fatalf("begin: %v", err)
	}
	tb.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

// worldOwnerID is the World owner's seeded id.
const worldOwnerID = store.OwnerID(1)

// nextTelegramID hands out a distinct telegram id per call, so parallel
// subtests each get their own player without colliding on owner's
// (kind, telegram_id) uniqueness.
var nextTelegramID int64

func newTelegramID() *int64 {
	id := atomic.AddInt64(&nextTelegramID, 1)
	return &id
}

// newPlayer creates a player owner, with every scope and account its
// kind seeds.
func newPlayer(t *testing.T, ctx context.Context, tx pgx.Tx) store.Owner {
	t.Helper()
	owner, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, newTelegramID())
	if err != nil {
		t.Fatalf("create player: %v", err)
	}
	return owner
}

// accountID reads the id of the account belonging to ownerID at the
// given scope definition and account definition codes.
func accountID(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID store.OwnerID, scopeCode, accountCode string) store.AccountID {
	t.Helper()
	var id store.AccountID
	err := tx.QueryRow(ctx, `
SELECT a.id
FROM account a
JOIN scope s ON s.id = a.scope_id
JOIN scope_definition sd ON sd.id = s.scope_definition_id
JOIN account_definition ad ON ad.id = a.account_definition_id
WHERE s.owner_id = $1 AND sd.code = $2 AND ad.code = $3`, ownerID, scopeCode, accountCode).Scan(&id)
	if err != nil {
		t.Fatalf("account id for owner %d %s/%s: %v", ownerID, scopeCode, accountCode, err)
	}
	return id
}

// holderID reads the id of ownerID's scope at the given scope definition
// code.
func holderID(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID store.OwnerID, scopeCode string) store.HolderID {
	t.Helper()
	var id store.HolderID
	err := tx.QueryRow(ctx, `
SELECT s.id
FROM scope s
JOIN scope_definition sd ON sd.id = s.scope_definition_id
WHERE s.owner_id = $1 AND sd.code = $2`, ownerID, scopeCode).Scan(&id)
	if err != nil {
		t.Fatalf("holder id for owner %d %s: %v", ownerID, scopeCode, err)
	}
	return id
}

// worldAccountID reads a World account's id for kind, for kinds this
// helper's own fixtures need.
func worldAccountID(t *testing.T, ctx context.Context, tx pgx.Tx, kind store.Kind) store.AccountID {
	t.Helper()
	switch kind {
	case store.KindMoney:
		return store.WorldMoney
	case store.KindExperience:
		return store.WorldExperience
	default:
		return accountID(t, ctx, tx, worldOwnerID, "world", string(kind)+"_free")
	}
}

// fund posts a ManualCorrection crediting accountID by amount from the
// matching World account of the given kind.
func fund(t *testing.T, ctx context.Context, tx pgx.Tx, accountID store.AccountID, kind store.Kind, amount decimal.Decimal) {
	t.Helper()
	worldID := worldAccountID(t, ctx, tx, kind)
	err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "fund"},
		store.Posting{AccountID: accountID, Amount: amount},
		store.Posting{AccountID: worldID, Amount: amount.Neg()},
	)
	if err != nil {
		t.Fatalf("fund: %v", err)
	}
}

// grantSlots posts a ManualCorrection crediting holderID's slots_free
// account by budget from the World's own slots_free account, so a later
// mint into holderID has capacity.
func grantSlots(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID store.OwnerID, scopeCode string, budget int64) {
	t.Helper()
	free := accountID(t, ctx, tx, ownerID, scopeCode, "slots_free")
	worldFree := accountID(t, ctx, tx, worldOwnerID, "world", "slots_free")
	err := store.Post(ctx, tx, &store.ManualCorrection{Actor: "test", Reason: "grant slots"},
		store.Posting{AccountID: free, Amount: decimal.NewFromInt(budget)},
		store.Posting{AccountID: worldFree, Amount: decimal.NewFromInt(-budget)},
	)
	if err != nil {
		t.Fatalf("grant slots: %v", err)
	}
}

// testRegistry builds a Registry with made-up shop_sale/shop_purchase/
// probe_grant shapes that borrow existing catalog codes, for this
// package's own tests to write under. They are not the game's real
// signatures and must never be copied into the shipped declared set.
func testRegistry(t *testing.T) *contract.Registry {
	t.Helper()
	reg, err := contract.NewRegistry(
		contract.Declaration{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
			contract.PostingLeg("attributes", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
			contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(1)),
		)},
		contract.Declaration{Type: contract.Event(store.EventShopPurchase), Signature: contract.Expect(
			contract.PostingLeg("world", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
			contract.PostingLeg("attributes", "money", store.KindMoney, contract.Positive, contract.Exactly(1)),
		)},
		contract.Declaration{Type: contract.DeferredTask("probe_grant"), Signature: contract.Expect(
			contract.MovementLeg("world", "backpack", contract.Exactly(1)),
			contract.PostingLeg("world", "slots_free", store.KindSlots, contract.Positive, contract.Exactly(1)),
			contract.PostingLeg("world", "slots_used", store.KindSlots, contract.Negative, contract.Exactly(1)),
			contract.PostingLeg("backpack", "slots_free", store.KindSlots, contract.Negative, contract.Exactly(1)),
			contract.PostingLeg("backpack", "slots_used", store.KindSlots, contract.Positive, contract.Exactly(1)),
		)},
		contract.Declaration{Type: contract.ManualCorrection(), Signature: contract.AnyBalanced()},
	)
	if err != nil {
		t.Fatalf("testRegistry: %v", err)
	}
	return reg
}
