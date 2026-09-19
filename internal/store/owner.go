package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Queryer is the single QueryRow method a read shared between a
// transaction and a pool-backed caller needs — satisfied by both pgx.Tx
// and *pgxpool.Pool. It is declared here, by PlayerExists' own package,
// per this project's interfaces-declared-by-the-consumer rule: PlayerExists
// runs both on a handler's tx and on the update-ingest loop's gate, which
// has no transaction to borrow.
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PlayerExists reports whether an owner row with kind = 'player' and the
// given telegramID exists, queried through q. The predicate names the
// kind because owner's uniqueness is on the (kind, telegram_id) pair and
// a chat's own id lives in the same column — a lookup keyed
// on telegram_id alone would match every chat the bot was ever added to.
func PlayerExists(ctx context.Context, q Queryer, telegramID int64) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM owner WHERE kind = $1 AND telegram_id = $2)`,
		OwnerPlayer, telegramID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("player exists: %w", err)
	}
	return exists, nil
}

// ChatOwnerID finds the owner id of the chat with kind = 'chat' and the
// given telegramID, reporting false on a miss. It finds only and never
// creates: a deep-link Start naming a chat the bot was never added to
// must record no membership, rather than conjuring an owner row for it.
func ChatOwnerID(ctx context.Context, q Queryer, telegramID int64) (OwnerID, bool, error) {
	var ownerID int64
	err := q.QueryRow(ctx,
		`SELECT id FROM owner WHERE kind = $1 AND telegram_id = $2`,
		OwnerChat, telegramID,
	).Scan(&ownerID)
	switch {
	case err == nil:
		return OwnerID(ownerID), true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("chat owner id: %w", err)
	}
}

// Account is one instance of a catalog AccountDefinition, created for a
// specific owner's scope. Kind and Controlled are copied from the
// definition at creation time for convenient access; the database remains
// the source of truth Post itself reads.
type Account struct {
	ID                  AccountID
	ScopeDefinitionID   int16
	AccountDefinitionID int16
	Kind                Kind
	Controlled          bool
}

// Owner is the result of CreateOwner: the created owner row plus every
// account created for its scopes.
type Owner struct {
	ID         OwnerID
	Kind       OwnerKind
	TelegramID *int64
	Accounts   []Account
}

// CreateOwner creates an owner of the given kind, with its telegramID where
// the kind requires one, and — in the same transaction — every scope whose
// scope_definition.owner_kind matches, every account of those scopes, and a
// zero-balance account_balance row for each controlled account. The
// World owner is seeded by migration and is never created here: kind ==
// OwnerWorld is rejected with ErrInvalidOwner, as is any kind outside the
// OwnerKind mirror, and OwnerPlayer/OwnerChat with a nil telegramID —
// each rejection happens before any statement is issued, because the
// schema itself admits a NULL telegram_id and would otherwise accept an
// anonymous player silently.
func CreateOwner(ctx context.Context, tx pgx.Tx, kind OwnerKind, telegramID *int64) (Owner, error) {
	switch {
	case !kind.known():
		return Owner{}, fmt.Errorf("%w: unknown owner kind %q", ErrInvalidOwner, kind)
	case kind == OwnerWorld:
		return Owner{}, fmt.Errorf("%w: the World owner is seeded by migration, never created", ErrInvalidOwner)
	case (kind == OwnerPlayer || kind == OwnerChat) && telegramID == nil:
		return Owner{}, fmt.Errorf("%w: kind %q requires a telegram_id", ErrInvalidOwner, kind)
	}

	var ownerID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO owner (kind, telegram_id) VALUES ($1, $2) RETURNING id`,
		kind, telegramID,
	).Scan(&ownerID); err != nil {
		return Owner{}, fmt.Errorf("insert owner: %w", err)
	}

	rows, err := tx.Query(ctx, `
		WITH s AS (
			INSERT INTO scope (owner_id, scope_definition_id)
			SELECT $1, id FROM scope_definition WHERE owner_kind = $2 ORDER BY id
			RETURNING id, scope_definition_id
		)
		INSERT INTO account (scope_id, account_definition_id)
		SELECT s.id, d.id
		FROM s JOIN account_definition d ON d.scope_definition_id = s.scope_definition_id
		ORDER BY d.id
		RETURNING id, account_definition_id
	`, ownerID, kind)
	if err != nil {
		return Owner{}, fmt.Errorf("create scopes and accounts: %w", err)
	}
	type created struct {
		accountID    int64
		definitionID int16
	}
	var createdAccounts []created
	for rows.Next() {
		var c created
		if err := rows.Scan(&c.accountID, &c.definitionID); err != nil {
			return Owner{}, fmt.Errorf("scan created account: %w", err)
		}
		createdAccounts = append(createdAccounts, c)
	}
	if err := rows.Err(); err != nil {
		return Owner{}, fmt.Errorf("create scopes and accounts: %w", err)
	}

	owner := Owner{ID: OwnerID(ownerID), Kind: kind, TelegramID: telegramID}
	if len(createdAccounts) == 0 {
		return owner, nil
	}

	defIDs := make([]int16, 0, len(createdAccounts))
	seen := make(map[int16]bool, len(createdAccounts))
	for _, c := range createdAccounts {
		if !seen[c.definitionID] {
			seen[c.definitionID] = true
			defIDs = append(defIDs, c.definitionID)
		}
	}

	defRows, err := tx.Query(ctx,
		`SELECT id, scope_definition_id, kind, controlled FROM account_definition WHERE id = ANY($1)`, defIDs)
	if err != nil {
		return Owner{}, fmt.Errorf("read account definitions: %w", err)
	}
	type def struct {
		scopeDefinitionID int16
		kind              Kind
		controlled        bool
	}
	defByID := make(map[int16]def, len(defIDs))
	for defRows.Next() {
		var id int16
		var d def
		var kindText string
		if err := defRows.Scan(&id, &d.scopeDefinitionID, &kindText, &d.controlled); err != nil {
			return Owner{}, fmt.Errorf("scan account definition: %w", err)
		}
		d.kind = Kind(kindText)
		defByID[id] = d
	}
	if err := defRows.Err(); err != nil {
		return Owner{}, fmt.Errorf("read account definitions: %w", err)
	}

	owner.Accounts = make([]Account, 0, len(createdAccounts))
	for _, c := range createdAccounts {
		d := defByID[c.definitionID]
		owner.Accounts = append(owner.Accounts, Account{
			ID:                  AccountID(c.accountID),
			ScopeDefinitionID:   d.scopeDefinitionID,
			AccountDefinitionID: c.definitionID,
			Kind:                d.kind,
			Controlled:          d.controlled,
		})
		if d.controlled {
			if _, err := tx.Exec(ctx,
				`INSERT INTO account_balance (account_id) VALUES ($1)`, c.accountID,
			); err != nil {
				return Owner{}, fmt.Errorf("insert account_balance for account %d: %w", c.accountID, err)
			}
		}
	}

	return owner, nil
}

// EnsureOwner finds an owner row keyed on (kind, telegramID), creating one
// through CreateOwner on a miss, and reports whether it created the row.
// CreateOwner's own refusals — an unknown kind, OwnerWorld, or
// OwnerPlayer/OwnerChat with a nil telegramID — are unchanged and happen
// before any statement is issued, on this call exactly as they do on a
// direct CreateOwner call.
//
// The returned Owner's Accounts field carries the row's scopes and
// accounts only when this call created them: on a hit it is nil, since a
// find does not re-read what an earlier CreateOwner already returned; on
// a miss it is whatever CreateOwner populated.
//
// On a concurrent creator racing this call, the read finds no row, the
// insert loses the race against the partial unique index on (kind,
// telegram_id), and that unique violation is returned wrapped rather
// than swallowed: this call takes no ON CONFLICT. The caller's own retry,
// in a fresh transaction, is what turns the next attempt's read into a
// hit.
func EnsureOwner(ctx context.Context, tx pgx.Tx, kind OwnerKind, telegramID *int64) (Owner, bool, error) {
	switch {
	case !kind.known():
		return Owner{}, false, fmt.Errorf("%w: unknown owner kind %q", ErrInvalidOwner, kind)
	case kind == OwnerWorld:
		return Owner{}, false, fmt.Errorf("%w: the World owner is seeded by migration, never created", ErrInvalidOwner)
	case (kind == OwnerPlayer || kind == OwnerChat) && telegramID == nil:
		return Owner{}, false, fmt.Errorf("%w: kind %q requires a telegram_id", ErrInvalidOwner, kind)
	}

	var ownerID int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM owner WHERE kind = $1 AND telegram_id = $2`,
		kind, telegramID,
	).Scan(&ownerID)
	switch {
	case err == nil:
		return Owner{ID: OwnerID(ownerID), Kind: kind, TelegramID: telegramID}, false, nil
	case errors.Is(err, pgx.ErrNoRows):
		owner, createErr := CreateOwner(ctx, tx, kind, telegramID)
		if createErr != nil {
			return Owner{}, false, createErr
		}
		return owner, true, nil
	default:
		return Owner{}, false, fmt.Errorf("ensure owner: find: %w", err)
	}
}
