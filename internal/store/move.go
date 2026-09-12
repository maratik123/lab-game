package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// Movement is one instance's move from one holder to another within a
// single Move call. ItemID is NewItem to mint a fresh instance under this
// move's own document; otherwise it names an existing instance, and From
// must be its current holder.
type Movement struct {
	ItemID ItemID
	From   HolderID
	To     HolderID
}

const (
	// sqlstateUniqueViolation is PostgreSQL's SQLSTATE for a unique-index
	// violation. The foreign-key violation SQLSTATE this package also
	// checks is declared once, elsewhere in the package, and reused here.
	sqlstateUniqueViolation = "23505"
	// constraintChainFK and constraintSuccessorKey name item_movement's own
	// chain-continuity objects; Move maps a violation of either, surfacing
	// only under a concurrent writer, to ErrMoveConflict.
	constraintChainFK      = "item_movement_chain_fk"
	constraintSuccessorKey = "item_movement_successor_key"
)

// Move writes one document's whole effect on both the quantitative ledger
// and the item machine: the instance movements in movements, the capacity
// postings derived from them, and the caller's own postings — all under
// basis, in the caller's transaction, as exactly one basis document and
// one journal_entry. The caller owns the transaction: Move neither
// commits nor rolls back.
//
// The returned slice is parallel to movements: entry i is the instance
// movements[i] moved — the freshly minted id where that movement carried
// NewItem, the caller's own ItemID echoed back otherwise.
//
// Phases, in order (mirroring post's documented shape):
//
//	a. static shape, no SQL: movements non-empty (ErrNoMovements); no
//	   From == To (ErrSelfMove); no existing instance named twice in one
//	   batch — NewItem is exempt (ErrDuplicateItem); every NewItem
//	   movement's From is WorldHolder (ErrMintNotFromWorld). Transaction
//	   untouched.
//	b. two SELECTs, no writes: the chain head of every named existing
//	   instance (ErrUnknownItem on a miss, ErrNotCurrentHolder when From
//	   disagrees), and the slots free/used account of every touched holder
//	   (ErrNoCapacityAccount on a miss). Transaction usable, nothing
//	   written.
//	c. build the batch: the caller's postings, then the derived
//	   per-movement capacity legs. post sums per account before touching a
//	   balance, so a caller batch that is not itself zero-sum per kind
//	   fails post's own check as ErrUnbalanced.
//	d. post(...): one basis document, one journal_entry, the balance
//	   UPDATEs, then every posting. ErrOverdraft here is the capacity
//	   refusal.
//	e. mint: one item row per NewItem movement.
//	f. movements: the item_movement rows, in ascending instance id. A chain
//	   refusal that only a concurrent transaction could produce surfaces as
//	   ErrMoveConflict, transaction aborted.
func Move(ctx context.Context, tx pgx.Tx, basis PostingBasis, movements []Movement, postings ...Posting) ([]ItemID, error) {
	// Phase a.
	if len(movements) == 0 {
		return nil, ErrNoMovements
	}
	existingSeen := make(map[ItemID]bool, len(movements))
	for _, m := range movements {
		if m.From == m.To {
			return nil, ErrSelfMove
		}
		if m.ItemID == NewItem {
			if m.From != WorldHolder {
				return nil, ErrMintNotFromWorld
			}
			continue
		}
		if existingSeen[m.ItemID] {
			return nil, fmt.Errorf("%w: item %d", ErrDuplicateItem, m.ItemID)
		}
		existingSeen[m.ItemID] = true
	}

	// Phase b: current holder of every named existing instance.
	existingIDs := make([]ItemID, 0, len(existingSeen))
	for id := range existingSeen {
		existingIDs = append(existingIDs, id)
	}
	holderByItem := make(map[ItemID]HolderID, len(existingIDs))
	headMovementByItem := make(map[ItemID]int64, len(existingIDs))
	if len(existingIDs) > 0 {
		rows, err := tx.Query(ctx,
			`SELECT item_id, holder_id, movement_id FROM item_holder WHERE item_id = ANY($1)`, existingIDs)
		if err != nil {
			return nil, fmt.Errorf("move: select item_holder: %w", err)
		}
		for rows.Next() {
			var id ItemID
			var holder HolderID
			var movementID int64
			if err := rows.Scan(&id, &holder, &movementID); err != nil {
				rows.Close()
				return nil, fmt.Errorf("move: scan item_holder: %w", err)
			}
			holderByItem[id] = holder
			headMovementByItem[id] = movementID
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("move: select item_holder: %w", err)
		}
		if len(holderByItem) < len(existingIDs) {
			return nil, ErrUnknownItem
		}
		for _, m := range movements {
			if m.ItemID == NewItem {
				continue
			}
			if holderByItem[m.ItemID] != m.From {
				return nil, ErrNotCurrentHolder
			}
		}
	}

	// Phase b: the slots free/used account of every touched holder.
	holderSeen := make(map[HolderID]bool, 2*len(movements))
	for _, m := range movements {
		holderSeen[m.From] = true
		holderSeen[m.To] = true
	}
	holderIDs := make([]HolderID, 0, len(holderSeen))
	for id := range holderSeen {
		holderIDs = append(holderIDs, id)
	}
	type capacityAccounts struct {
		free, used AccountID
		haveFree   bool
		haveUsed   bool
	}
	capByHolder := make(map[HolderID]*capacityAccounts, len(holderIDs))
	rows, err := tx.Query(ctx,
		`SELECT s.id, d.capacity_role, a.id
		 FROM scope s
		 JOIN account a ON a.scope_id = s.id
		 JOIN account_definition d ON d.id = a.account_definition_id
		 WHERE s.id = ANY($1) AND d.kind = 'slots' AND d.capacity_role IS NOT NULL`, holderIDs)
	if err != nil {
		return nil, fmt.Errorf("move: select capacity accounts: %w", err)
	}
	for rows.Next() {
		var holder HolderID
		var role string
		var accountID AccountID
		if err := rows.Scan(&holder, &role, &accountID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("move: scan capacity account: %w", err)
		}
		accts, ok := capByHolder[holder]
		if !ok {
			accts = &capacityAccounts{}
			capByHolder[holder] = accts
		}
		switch CapacityRole(role) {
		case CapacityFree:
			accts.free, accts.haveFree = accountID, true
		case CapacityUsed:
			accts.used, accts.haveUsed = accountID, true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("move: select capacity accounts: %w", err)
	}
	for _, holder := range holderIDs {
		accts, ok := capByHolder[holder]
		if !ok || !accts.haveFree || !accts.haveUsed {
			return nil, fmt.Errorf("%w: holder %d", ErrNoCapacityAccount, holder)
		}
	}

	// Phase c.
	batch := make([]Posting, 0, len(postings)+4*len(movements))
	batch = append(batch, postings...)
	for _, m := range movements {
		from, to := capByHolder[m.From], capByHolder[m.To]
		batch = append(batch,
			Posting{AccountID: from.free, Amount: decimal.NewFromInt(1)},
			Posting{AccountID: from.used, Amount: decimal.NewFromInt(-1)},
			Posting{AccountID: to.free, Amount: decimal.NewFromInt(-1)},
			Posting{AccountID: to.used, Amount: decimal.NewFromInt(1)},
		)
	}

	// Phase d.
	journalEntryID, err := post(ctx, tx, basis, batch...)
	if err != nil {
		return nil, err
	}

	// Phase e: mint.
	ids := make([]ItemID, len(movements))
	for i, m := range movements {
		if m.ItemID != NewItem {
			ids[i] = m.ItemID
			continue
		}
		var minted int64
		if err := tx.QueryRow(ctx, `INSERT INTO item DEFAULT VALUES RETURNING id`).Scan(&minted); err != nil {
			return nil, fmt.Errorf("move: mint item: %w", err)
		}
		ids[i] = ItemID(minted)
	}

	// Phase f: movements, in ascending instance id.
	order := make([]int, len(movements))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(ids[a], ids[b]) })
	for _, i := range order {
		m := movements[i]
		var prev *int64
		if m.ItemID != NewItem {
			if v, ok := headMovementByItem[ids[i]]; ok {
				prev = &v
			}
		}
		var newHead int64
		err := tx.QueryRow(ctx,
			`INSERT INTO item_movement (item_id, prev_movement_id, from_holder_id, to_holder_id, journal_entry_id)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			ids[i], prev, m.From, m.To, journalEntryID,
		).Scan(&newHead)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && (pgErr.Code == sqlstateForeignKeyViolation && pgErr.ConstraintName == constraintChainFK ||
				pgErr.Code == sqlstateUniqueViolation && pgErr.ConstraintName == constraintSuccessorKey) {
				return nil, ErrMoveConflict
			}
			return nil, fmt.Errorf("move: insert item_movement: %w", err)
		}
	}

	return ids, nil
}
