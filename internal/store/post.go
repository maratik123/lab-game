package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// Posting is one half-entry of a batch passed to Post: the account it
// touches and the signed amount to add to that account's ledger. The
// account's ledger kind and controlled-ness come from the database
// (account_definition), never from the caller — "wrong kind on wrong
// account" is unrepresentable.
type Posting struct {
	AccountID AccountID
	Amount    decimal.Decimal
}

// maxIntegerDigitsBound is 10^25 — the exclusive upper bound on the
// absolute value of a representable amount (25 integer digits, D5).
var maxIntegerDigitsBound = decimal.New(1, 25)

// validAmount reports whether a is exactly representable at scale 5,
// non-zero, and within 25 integer digits (D5).
func validAmount(a decimal.Decimal) bool {
	return !a.IsZero() && a.Truncate(5).Equal(a) && a.Abs().LessThan(maxIntegerDigitsBound)
}

const (
	// sqlstateCheckViolation is PostgreSQL's SQLSTATE for a CHECK constraint
	// violation (class 23, integrity constraint violation).
	sqlstateCheckViolation = "23514"
	// constraintBalanceNonnegative names the CHECK (balance >= 0) on
	// account_balance in migration 00001; Post maps a violation of exactly
	// this constraint to ErrOverdraft.
	constraintBalanceNonnegative = "account_balance_nonnegative"
)

// Post applies one balanced batch of postings under basis, inside the
// caller's transaction tx. The caller owns the transaction: Post neither
// commits nor rolls back. Postings may reference an account more than
// once (KD-7) — deltas are summed per account before any balance is
// written.
//
// Phases, in order (docs/DESIGN.md §11, spec Scope 4):
//
//	a. static shape check, no SQL: basis non-nil and not a typed-nil basis
//	   (ErrNoBasis, transaction untouched); postings non-empty
//	   (ErrEmptyBatch, untouched); every amount valid (ErrInvalidAmount,
//	   untouched).
//	b. one SELECT over the batch's distinct accounts for kind/controlled
//	   (ErrUnknownAccount on a missing id, untouched — a SELECT writes
//	   nothing).
//	c. zero-sum check per kind, no SQL (ErrUnbalanced, untouched).
//	d. insert the basis document, then the journal_entry
//	   (ErrAlreadyPosted on a player-operation replay, untouched; any
//	   other error: aborted).
//	e. one plain UPDATE per controlled account with a non-zero delta, in
//	   ascending account_id (ErrOverdraft or ErrBalanceRowMissing, or any
//	   other wrapped database error — all leave the transaction aborted,
//	   except ErrBalanceRowMissing, which still requires a rollback
//	   because phase d already wrote). A balance that would exceed
//	   numeric(30,5)'s 25 integer digits is refused by the database with
//	   SQLSTATE 22003 (numeric_value_out_of_range) and surfaces as a
//	   wrapped *pgconn.PgError, not as a sentinel: it is unreachable
//	   through posting.amount (validated in phase a) and reachable only
//	   through the accumulated balance.
//	f. insert the postings, in the caller's order (aborted on error).
func Post(ctx context.Context, tx pgx.Tx, basis PostingBasis, postings ...Posting) error {
	// Phase a.
	if basis == nil {
		return ErrNoBasis
	}
	entrySQL, err := basis.entrySQL()
	if err != nil {
		return err
	}
	if len(postings) == 0 {
		return ErrEmptyBatch
	}
	for i, p := range postings {
		if !validAmount(p.Amount) {
			return fmt.Errorf("%w: posting %d, account %d, amount %s", ErrInvalidAmount, i, p.AccountID, p.Amount)
		}
	}

	ids := make([]AccountID, 0, len(postings))
	for _, p := range postings {
		ids = append(ids, p.AccountID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	// Phase b.
	type accountInfo struct {
		kind       Kind
		controlled bool
	}
	infoByID := make(map[AccountID]accountInfo, len(ids))
	rows, err := tx.Query(ctx,
		`SELECT a.id, d.kind, d.controlled
		 FROM account a JOIN account_definition d ON d.id = a.account_definition_id
		 WHERE a.id = ANY($1)`, ids)
	if err != nil {
		return fmt.Errorf("post: select accounts: %w", err)
	}
	for rows.Next() {
		var id AccountID
		var kindText string
		var info accountInfo
		if err := rows.Scan(&id, &kindText, &info.controlled); err != nil {
			rows.Close()
			return fmt.Errorf("post: scan account: %w", err)
		}
		info.kind = Kind(kindText)
		infoByID[id] = info
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("post: select accounts: %w", err)
	}
	if len(infoByID) < len(ids) {
		return ErrUnknownAccount
	}

	// Phase c.
	deltaByID := make(map[AccountID]decimal.Decimal, len(ids))
	sumByKind := make(map[Kind]decimal.Decimal, 2)
	for _, p := range postings {
		deltaByID[p.AccountID] = deltaByID[p.AccountID].Add(p.Amount)
		kind := infoByID[p.AccountID].kind
		sumByKind[kind] = sumByKind[kind].Add(p.Amount)
	}
	for _, sum := range sumByKind {
		if !sum.IsZero() {
			return ErrUnbalanced
		}
	}

	// Phase d.
	docID, err := basis.insert(ctx, tx)
	if err != nil {
		if errors.Is(err, ErrAlreadyPosted) {
			return err
		}
		return fmt.Errorf("post: insert basis document: %w", err)
	}
	var entryID int64
	if err := tx.QueryRow(ctx, entrySQL, docID).Scan(&entryID); err != nil {
		return fmt.Errorf("post: insert journal_entry: %w", err)
	}

	// Phase e.
	for _, id := range ids {
		info := infoByID[id]
		if !info.controlled {
			continue
		}
		delta := deltaByID[id]
		if delta.IsZero() {
			continue
		}
		tag, err := tx.Exec(ctx,
			`UPDATE account_balance SET balance = balance + $2 WHERE account_id = $1`, id, delta)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == sqlstateCheckViolation && pgErr.ConstraintName == constraintBalanceNonnegative {
				return fmt.Errorf("%w: account %d: %w", ErrOverdraft, id, err)
			}
			return fmt.Errorf("post: update balance for account %d: %w", id, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%w: account %d", ErrBalanceRowMissing, id)
		}
	}

	// Phase f.
	for _, p := range postings {
		if _, err := tx.Exec(ctx,
			`INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, $2, $3)`,
			entryID, p.AccountID, p.Amount,
		); err != nil {
			return fmt.Errorf("post: insert posting: %w", err)
		}
	}

	return nil
}
