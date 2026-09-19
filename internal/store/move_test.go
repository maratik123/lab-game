package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// newPlayerWithBackpack creates a player owner inside tx and returns its
// id and its backpack holder id.
func newPlayerWithBackpack(t *testing.T, ctx context.Context, tx pgx.Tx) (OwnerID, HolderID) {
	t.Helper()
	tg := nextTelegramID.Add(1)
	owner, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	return owner.ID, backpackHolder(t, ctx, tx, owner.ID)
}

// slotsAccounts returns holder's slots free and used account ids.
func slotsAccounts(t *testing.T, ctx context.Context, tx pgx.Tx, holder HolderID) (free, used AccountID) {
	t.Helper()
	rows, err := tx.Query(ctx,
		`SELECT d.capacity_role, a.id
		 FROM scope s
		 JOIN account a ON a.scope_id = s.id
		 JOIN account_definition d ON d.id = a.account_definition_id
		 WHERE s.id = $1 AND d.kind = 'slots' AND d.capacity_role IS NOT NULL`, holder)
	if err != nil {
		t.Fatalf("query slots accounts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var id AccountID
		if err := rows.Scan(&role, &id); err != nil {
			t.Fatalf("scan slots account: %v", err)
		}
		switch CapacityRole(role) {
		case CapacityFree:
			free = id
		case CapacityUsed:
			used = id
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return free, used
}

// grantSlots posts a ManualCorrection granting holder's slots_free budget
// units from World's uncontrolled slots_free account.
func grantSlots(t *testing.T, ctx context.Context, tx pgx.Tx, holder HolderID, budget int64) {
	t.Helper()
	free, _ := slotsAccounts(t, ctx, tx, holder)
	worldFree, _ := slotsAccounts(t, ctx, tx, WorldHolder)
	if err := Post(ctx, tx, &ManualCorrection{Actor: "test", Reason: "grant slots"},
		Posting{AccountID: free, Amount: decimal.NewFromInt(budget)},
		Posting{AccountID: worldFree, Amount: decimal.NewFromInt(-budget)},
	); err != nil {
		t.Fatalf("grant slots: %v", err)
	}
}

// postingSet reads back every posting of a journal_entry as a
// (account_id, amount) set, order-independent.
type postingLeg struct {
	Account AccountID
	Amount  string
}

// txQueryer is the Query method a *pgxpool.Pool and a pgx.Tx both satisfy.
type txQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func postingsOf(t *testing.T, ctx context.Context, q txQueryer, journalEntryID int64) []postingLeg {
	t.Helper()
	rows, err := q.Query(ctx, `SELECT account_id, amount FROM posting WHERE journal_entry_id = $1`, journalEntryID)
	if err != nil {
		t.Fatalf("query postings: %v", err)
	}
	defer rows.Close()
	var out []postingLeg
	for rows.Next() {
		var leg postingLeg
		var amt decimal.Decimal
		if err := rows.Scan(&leg.Account, &amt); err != nil {
			t.Fatalf("scan posting: %v", err)
		}
		leg.Amount = amt.String()
		out = append(out, leg)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Account != out[j].Account {
			return out[i].Account < out[j].Account
		}
		return out[i].Amount < out[j].Amount
	})
	return out
}

func movementJournalEntry(t *testing.T, ctx context.Context, tx pgx.Tx, itemID ItemID) int64 {
	t.Helper()
	var je int64
	if err := tx.QueryRow(ctx,
		`SELECT journal_entry_id FROM item_movement WHERE item_id = $1 ORDER BY id DESC LIMIT 1`, itemID,
	).Scan(&je); err != nil {
		t.Fatalf("select movement journal_entry_id for item %d: %v", itemID, err)
	}
	return je
}

func TestMove_happyPath(t *testing.T) {
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

	ids, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "mint"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if err != nil {
		t.Fatalf("Move mint: %v", err)
	}
	if len(ids) != 1 || ids[0] == NewItem {
		t.Fatalf("Move mint ids = %v, want one fresh id", ids)
	}
	item := ids[0]

	var fromH, toH HolderID
	var prev *int64
	if err := tx.QueryRow(ctx,
		`SELECT from_holder_id, to_holder_id, prev_movement_id FROM item_movement WHERE item_id = $1`, item,
	).Scan(&fromH, &toH, &prev); err != nil {
		t.Fatalf("select genesis movement: %v", err)
	}
	if fromH != WorldHolder || toH != backpackA || prev != nil {
		t.Fatalf("genesis movement = from:%d to:%d prev:%v, want World/%d/nil", fromH, toH, prev, backpackA)
	}

	worldFree, worldUsed := slotsAccounts(t, ctx, tx, WorldHolder)
	aFree, aUsed := slotsAccounts(t, ctx, tx, backpackA)
	je := movementJournalEntry(t, ctx, tx, item)
	got := postingsOf(t, ctx, tx, je)
	want := []postingLeg{
		{worldFree, "1"}, {worldUsed, "-1"}, {aFree, "-1"}, {aUsed, "1"},
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].Account != want[j].Account {
			return want[i].Account < want[j].Account
		}
		return want[i].Amount < want[j].Amount
	})
	if len(got) != len(want) {
		t.Fatalf("genesis postings = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Account != want[i].Account || !decimal.RequireFromString(got[i].Amount).Equal(decimal.RequireFromString(want[i].Amount)) {
			t.Fatalf("genesis postings = %+v, want %+v", got, want)
		}
	}

	ids2, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "move"},
		[]Movement{{ItemID: item, From: backpackA, To: backpackB}})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(ids2) != 1 || ids2[0] != item {
		t.Fatalf("Move ids = %v, want [%d]", ids2, item)
	}

	var secondPrev *int64
	var firstMovementID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM item_movement WHERE item_id = $1 AND prev_movement_id IS NULL`, item).Scan(&firstMovementID); err != nil {
		t.Fatalf("select first movement id: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT prev_movement_id FROM item_movement WHERE item_id = $1 AND to_holder_id = $2`, item, backpackB,
	).Scan(&secondPrev); err != nil {
		t.Fatalf("select second movement: %v", err)
	}
	if secondPrev == nil || *secondPrev != firstMovementID {
		t.Fatalf("second movement's prev = %v, want %d", secondPrev, firstMovementID)
	}
}

func TestMove_oneDocumentBothMachines(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(100))
	_, backpackA := newPlayerWithBackpack(t, ctx, tx)
	grantSlots(t, ctx, tx, backpackA, 5)

	basis := &PlayerOperation{Source: SourceTelegram, OperationID: "craft-1"}
	ids, err := Move(ctx, tx, basis,
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}},
		Posting{AccountID: money, Amount: decimal.NewFromInt(-10)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(10)},
	)
	if err != nil {
		t.Fatalf("Move craft: %v", err)
	}
	item := ids[0]

	var jeCount, docCount int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM journal_entry WHERE player_operation_id = (
			SELECT id FROM player_operation WHERE source = 'telegram' AND operation_id = 'craft-1')`,
	).Scan(&jeCount); err != nil {
		t.Fatalf("count journal_entry: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM player_operation WHERE source = 'telegram' AND operation_id = 'craft-1'`).Scan(&docCount); err != nil {
		t.Fatalf("count player_operation: %v", err)
	}
	if jeCount != 1 || docCount != 1 {
		t.Fatalf("journal_entry=%d player_operation=%d, want 1/1", jeCount, docCount)
	}

	je := movementJournalEntry(t, ctx, tx, item)
	var movementJE int64
	if err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM item_movement WHERE item_id = $1`, item).Scan(&movementJE); err != nil {
		t.Fatalf("select movement journal_entry_id: %v", err)
	}
	if movementJE != je {
		t.Fatalf("movement journal_entry_id = %d, want %d", movementJE, je)
	}

	worldFree, worldUsed := slotsAccounts(t, ctx, tx, WorldHolder)
	aFree, aUsed := slotsAccounts(t, ctx, tx, backpackA)
	got := postingsOf(t, ctx, tx, je)
	wantAccounts := map[AccountID]bool{money: true, WorldMoney: true, worldFree: true, worldUsed: true, aFree: true, aUsed: true}
	if len(got) != len(wantAccounts) {
		t.Fatalf("postings = %+v, want exactly the caller legs union the derived legs (%d accounts)", got, len(wantAccounts))
	}
	for _, leg := range got {
		if !wantAccounts[leg.Account] {
			t.Fatalf("unexpected posting account %d", leg.Account)
		}
	}

	// The discriminating control: a Post under the same PlayerOperation
	// basis, then a Move under an equal basis value, must replay rather
	// than write a second document.
	basis2 := &PlayerOperation{Source: SourceTelegram, OperationID: "craft-2"}
	if err := Post(ctx, tx, basis2,
		Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("first Post: %v", err)
	}
	_, err = Move(ctx, tx, basis2, []Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("Move under a replayed PlayerOperation basis = %v, want ErrAlreadyPosted", err)
	}

	// Under a ManualCorrection basis, an equal-shaped pair legitimately
	// writes two documents (no idempotency key on that basis).
	mc := &ManualCorrection{Actor: "test", Reason: "two docs"}
	if err := Post(ctx, tx, mc,
		Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("Post under ManualCorrection: %v", err)
	}
	if _, err := Move(ctx, tx, mc, []Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}}); err != nil {
		t.Fatalf("Move under the same ManualCorrection value: %v", err)
	}
}

func TestMove_grantAndFillUnderOneDocument(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	_, backpackC := newPlayerWithBackpack(t, ctx, tx)
	free, _ := slotsAccounts(t, ctx, tx, backpackC)
	worldFree, _ := slotsAccounts(t, ctx, tx, WorldHolder)

	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "grant and fill"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackC}},
		Posting{AccountID: free, Amount: decimal.NewFromInt(1)},
		Posting{AccountID: worldFree, Amount: decimal.NewFromInt(-1)},
	); err != nil {
		t.Fatalf("grant-and-fill Move: %v", err)
	}

	got := balanceOf(t, ctx, tx, free)
	if !got.IsZero() {
		t.Fatalf("backpackC free balance = %s, want 0 (grant net exactly consumed)", got)
	}

	// Control: the postings supplied in the opposite order succeed identically.
	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "grant and fill reordered"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackC}},
		Posting{AccountID: worldFree, Amount: decimal.NewFromInt(-1)},
		Posting{AccountID: free, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("grant-and-fill Move (reordered postings): %v", err)
	}
}

func TestMove_multiMintAndReturnedSlice(t *testing.T) {
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

	firstIDs, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "seed"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if err != nil {
		t.Fatalf("seed Move: %v", err)
	}
	existing := firstIDs[0]

	ids, err := Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "multi"},
		[]Movement{
			{ItemID: NewItem, From: WorldHolder, To: backpackA},
			{ItemID: NewItem, From: WorldHolder, To: backpackA},
			{ItemID: existing, From: backpackA, To: backpackB},
		})
	if err != nil {
		t.Fatalf("multi-mint Move: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("ids = %v, want 3 entries", ids)
	}
	if ids[0] == NewItem || ids[1] == NewItem || ids[0] == ids[1] {
		t.Fatalf("ids[0]/ids[1] = %v/%v, want two distinct fresh ids", ids[0], ids[1])
	}
	if ids[2] != existing {
		t.Fatalf("ids[2] = %d, want the echoed existing id %d", ids[2], existing)
	}

	for _, mint := range []ItemID{ids[0], ids[1]} {
		var count int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM item_movement WHERE item_id = $1 AND prev_movement_id IS NULL`, mint,
		).Scan(&count); err != nil {
			t.Fatalf("count genesis for %d: %v", mint, err)
		}
		if count != 1 {
			t.Fatalf("mint %d has %d genesis movements, want 1", mint, count)
		}
	}

	// Negative twin: naming the same existing instance twice is refused.
	_, err = Move(ctx, tx, &ManualCorrection{Actor: "test", Reason: "dup"},
		[]Movement{
			{ItemID: existing, From: backpackB, To: backpackA},
			{ItemID: existing, From: backpackB, To: backpackA},
		})
	if !errors.Is(err, ErrDuplicateItem) {
		t.Fatalf("Move with a duplicated existing instance = %v, want ErrDuplicateItem", err)
	}
}

func TestMove_sentinels(t *testing.T) {
	t.Parallel()

	t.Run("no_movements", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"}, nil)
		if !errors.Is(err, ErrNoMovements) {
			t.Fatalf("Move(nil) = %v, want ErrNoMovements", err)
		}
		assertNoWrite(t, rec)
	})

	t.Run("self_move", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: WorldHolder}})
		if !errors.Is(err, ErrSelfMove) {
			t.Fatalf("Move with From==To = %v, want ErrSelfMove", err)
		}
		assertNoWrite(t, rec)
	})

	t.Run("duplicate_existing_item", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		_, backpackB := newPlayerWithBackpack(t, ctx, tx)
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: 1, From: backpackA, To: backpackB}, {ItemID: 1, From: backpackA, To: backpackB}})
		if !errors.Is(err, ErrDuplicateItem) {
			t.Fatalf("Move with a duplicated instance = %v, want ErrDuplicateItem", err)
		}
		assertNoWrite(t, rec)
	})

	t.Run("mint_not_from_world", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		_, backpackB := newPlayerWithBackpack(t, ctx, tx)
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: NewItem, From: backpackA, To: backpackB}})
		if !errors.Is(err, ErrMintNotFromWorld) {
			t.Fatalf("Move minting not from World = %v, want ErrMintNotFromWorld", err)
		}
		assertNoWrite(t, rec)
	})

	t.Run("unknown_item", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		item := newBareItem(t, ctx, tx) // no movement, so no item_holder row
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: item, From: WorldHolder, To: backpackA}})
		if !errors.Is(err, ErrUnknownItem) {
			t.Fatalf("Move with an unknown item = %v, want ErrUnknownItem", err)
		}
		if errors.Is(err, ErrNotCurrentHolder) {
			t.Fatalf("ErrUnknownItem must not also be ErrNotCurrentHolder")
		}
		assertUsableNoWrite(t, ctx, tx, rec)
	})

	t.Run("not_current_holder", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		_, backpackB := newPlayerWithBackpack(t, ctx, tx)
		grantSlots(t, ctx, tx, backpackA, 5)
		grantSlots(t, ctx, tx, backpackB, 5)
		ids, err := Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "seed"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
		if err != nil {
			t.Fatalf("seed Move: %v", err)
		}
		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: ids[0], From: backpackB, To: WorldHolder}})
		if !errors.Is(err, ErrNotCurrentHolder) {
			t.Fatalf("Move with a wrong From = %v, want ErrNotCurrentHolder", err)
		}
		if errors.Is(err, ErrUnknownItem) {
			t.Fatalf("ErrNotCurrentHolder must not also be ErrUnknownItem")
		}
		assertUsableNoWrite(t, ctx, tx, rec)
	})

	t.Run("no_capacity_account", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool, rec := newStoreWithRecorder(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		// A chat owner's only scope is home, which has no slots accounts —
		// so a manually-inserted attributes scope (scope_definition_id 2,
		// never one a chat gets on its own) is what this test needs to
		// reach a holder with no capacity account.
		tg := nextTelegramID.Add(1)
		chat, err := CreateOwner(ctx, tx, OwnerChat, &tg)
		if err != nil {
			t.Fatalf("CreateOwner(chat): %v", err)
		}
		var chatAttrScopeID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO scope (owner_id, scope_definition_id) VALUES ($1, 2) RETURNING id`, chat.ID,
		).Scan(&chatAttrScopeID); err != nil {
			t.Fatalf("insert chat scope: %v", err)
		}

		rec.Reset()
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: HolderID(chatAttrScopeID)}})
		if !errors.Is(err, ErrNoCapacityAccount) {
			t.Fatalf("Move into a holder with no capacity account = %v, want ErrNoCapacityAccount", err)
		}
		assertUsableNoWrite(t, ctx, tx, rec)
	})

	t.Run("nil_basis", func(t *testing.T) {
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
		_, err = Move(ctx, tx, nil, []Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
		if !errors.Is(err, ErrNoBasis) {
			t.Fatalf("Move(nil basis) = %v, want ErrNoBasis", err)
		}
	})

	t.Run("typed_nil_basis", func(t *testing.T) {
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
		var mc *ManualCorrection
		_, err = Move(ctx, tx, mc, []Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
		if !errors.Is(err, ErrNoBasis) {
			t.Fatalf("Move(typed-nil basis) = %v, want ErrNoBasis", err)
		}
	})

	t.Run("caller_batch_unbalanced", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)
		money, _ := createPlayer(t, ctx, tx)
		_, backpackA := newPlayerWithBackpack(t, ctx, tx)
		grantSlots(t, ctx, tx, backpackA, 5)
		_, err = Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "r"},
			[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}},
			Posting{AccountID: money, Amount: decimal.NewFromInt(1)},
		)
		if !errors.Is(err, ErrUnbalanced) {
			t.Fatalf("Move with an unbalanced caller batch = %v, want ErrUnbalanced", err)
		}
	})
}

// assertNoWrite asserts the recorder holds no write since the last Reset.
func assertNoWrite(t *testing.T, rec *queryRecorder) {
	t.Helper()
	for _, s := range rec.Statements() {
		up := strings.ToUpper(s.SQL)
		if strings.Contains(up, "INSERT") || strings.Contains(up, "UPDATE") || strings.Contains(up, "DELETE") {
			t.Fatalf("rejected Move issued a write: %+v", s)
		}
	}
}

// assertUsableNoWrite is assertNoWrite plus a check that tx is still
// usable — the transaction state a pre-write sentinel's doc comment claims.
func assertUsableNoWrite(t *testing.T, ctx context.Context, tx pgx.Tx, rec *queryRecorder) {
	t.Helper()
	assertNoWrite(t, rec)
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("tx unusable after rejected Move: %v", err)
	}
}

func TestMove_capacityOverdraft(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin setup: %v", err)
	}
	_, backpackA := newPlayerWithBackpack(t, ctx, tx1)
	_, backpackB := newPlayerWithBackpack(t, ctx, tx1)
	grantSlots(t, ctx, tx1, backpackA, 1)
	grantSlots(t, ctx, tx1, backpackB, 5)
	ids, err := Move(ctx, tx1, &ManualCorrection{Actor: "t", Reason: "fill"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if err != nil {
		t.Fatalf("fill Move: %v", err)
	}
	filled := ids[0]
	aFree, _ := slotsAccounts(t, ctx, tx1, backpackA)
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit setup: %v", err)
	}

	var postingCountBefore, movementCountBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM posting`).Scan(&postingCountBefore); err != nil {
		t.Fatalf("count posting: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_movement`).Scan(&movementCountBefore); err != nil {
		t.Fatalf("count item_movement: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin overdraft: %v", err)
	}
	_, err = Move(ctx, tx2, &ManualCorrection{Actor: "t", Reason: "overflow"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackA}})
	if !errors.Is(err, ErrOverdraft) {
		t.Fatalf("Move over a full backpack = %v, want ErrOverdraft", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("account %d", aFree)) {
		t.Fatalf("overdraft error %v does not name the destination's slots_free account %d", err, aFree)
	}
	rollback(t, ctx, tx2)

	var postingCountAfter, movementCountAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM posting`).Scan(&postingCountAfter); err != nil {
		t.Fatalf("count posting: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_movement`).Scan(&movementCountAfter); err != nil {
		t.Fatalf("count item_movement: %v", err)
	}
	if postingCountAfter != postingCountBefore || movementCountAfter != movementCountBefore {
		t.Fatalf("row counts changed on overdraft rejection: posting %d->%d, item_movement %d->%d",
			postingCountBefore, postingCountAfter, movementCountBefore, movementCountAfter)
	}

	// Control: the same instance moves into a holder with headroom.
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin control: %v", err)
	}
	defer rollback(t, ctx, tx3)
	if _, err := Move(ctx, tx3, &ManualCorrection{Actor: "t", Reason: "headroom"},
		[]Movement{{ItemID: filled, From: backpackA, To: backpackB}}); err != nil {
		t.Fatalf("Move into a holder with headroom: %v", err)
	}
}

// TestMove_holderKindTheMVPDoesNotUse plants a scope_definition for a
// non-MVP holder kind together with its capacity account definitions,
// inside a rolled-back transaction, and moves an instance through it with
// no kind-specific argument to Move.
func TestMove_holderKindTheMVPDoesNotUse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO scope_definition (id, code, owner_kind) VALUES (50, 'planted_corpse', 'player')`); err != nil {
		t.Fatalf("insert planted scope_definition: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled, capacity_role) VALUES
		 (500, 50, 'slots_free', 'slots', true, 'free'),
		 (501, 50, 'slots_used', 'slots', true, 'used')`); err != nil {
		t.Fatalf("insert planted account_definition: %v", err)
	}

	// CreateOwner itself creates the planted holder's scope, accounts and
	// balance rows — the same generic join every player scope goes
	// through, with no branch on kind. That genericity is exactly what
	// this test exercises: nothing beyond the two INSERTs above is needed
	// for a brand-new holder kind to work.
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

	var scope1, scope2 int64
	if err := tx.QueryRow(ctx,
		`SELECT id FROM scope WHERE owner_id = $1 AND scope_definition_id = 50`, owner1.ID,
	).Scan(&scope1); err != nil {
		t.Fatalf("select planted scope 1: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id FROM scope WHERE owner_id = $1 AND scope_definition_id = 50`, owner2.ID,
	).Scan(&scope2); err != nil {
		t.Fatalf("select planted scope 2: %v", err)
	}

	planted1, planted2 := HolderID(scope1), HolderID(scope2)
	grantSlots(t, ctx, tx, planted1, 5)
	grantSlots(t, ctx, tx, planted2, 5)
	_, backpackStart := newPlayerWithBackpack(t, ctx, tx)
	grantSlots(t, ctx, tx, backpackStart, 5)

	ids, err := Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "mint"},
		[]Movement{{ItemID: NewItem, From: WorldHolder, To: backpackStart}})
	if err != nil {
		t.Fatalf("mint into backpack: %v", err)
	}
	item := ids[0]

	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "into planted"},
		[]Movement{{ItemID: item, From: backpackStart, To: planted1}}); err != nil {
		t.Fatalf("Move into planted holder kind: %v", err)
	}
	if _, err := Move(ctx, tx, &ManualCorrection{Actor: "t", Reason: "between planted"},
		[]Movement{{ItemID: item, From: planted1, To: planted2}}); err != nil {
		t.Fatalf("Move between planted holder kind instances: %v", err)
	}
}
