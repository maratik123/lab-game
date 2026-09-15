package contract

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/store"
)

// Queryer is the reading contract NewMark and (*Registry).Check need:
// enough of pgx.Tx and *pgxpool.Pool to run a query and a single-row
// query. Both types satisfy it.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Mark is a point in the journal, read by NewMark. (*Registry).Check
// judges every journal entry with a greater id than mark's.
type Mark struct {
	id int64
}

// NewMark reads the greatest journal_entry.id visible to q, and returns
// the zero Mark when the journal holds no entry visible to q. The zero
// Mark admits every entry, because journal_entry.id starts at 1.
func NewMark(ctx context.Context, q Queryer) (Mark, error) {
	var id *int64
	if err := q.QueryRow(ctx, `SELECT max(id) FROM journal_entry`).Scan(&id); err != nil {
		return Mark{}, fmt.Errorf("contract: read journal mark: %w", err)
	}
	if id == nil {
		return Mark{}, nil
	}
	return Mark{id: *id}, nil
}

// entryRow is one journal entry read past a mark: its id and the
// DocumentType read from its own basis document. docType is the zero
// DocumentType when the entry references none of the basis tables this
// package knows — always unsigned, since NewRegistry refuses a
// declaration under the zero type.
type entryRow struct {
	id      int64
	docType DocumentType
}

// readEntries reads every journal entry with an id greater than
// mark.id, together with the DocumentType read from its own basis
// document.
func readEntries(ctx context.Context, q Queryer, mark Mark) ([]entryRow, error) {
	rows, err := q.Query(ctx, `
SELECT je.id,
       je.player_operation_id IS NOT NULL,
       je.manual_correction_id IS NOT NULL,
       ev.type,
       dt.task_type,
       rt.task_type
FROM journal_entry je
LEFT JOIN event ev ON ev.id = je.event_id
LEFT JOIN deferred_task dt ON dt.id = je.deferred_task_id
LEFT JOIN recurrent_task rt ON rt.id = je.recurrent_task_id
WHERE je.id > $1
ORDER BY je.id`, mark.id)
	if err != nil {
		return nil, fmt.Errorf("contract: read journal entries: %w", err)
	}
	defer rows.Close()

	var entries []entryRow
	for rows.Next() {
		var (
			id                                     int64
			isPlayerOperation, isManualCorrection  bool
			eventType, deferredType, recurrentType *string
		)
		if err := rows.Scan(&id, &isPlayerOperation, &isManualCorrection, &eventType, &deferredType, &recurrentType); err != nil {
			return nil, fmt.Errorf("contract: scan journal entry: %w", err)
		}
		entries = append(entries, entryRow{id: id, docType: classify(isPlayerOperation, isManualCorrection, eventType, deferredType, recurrentType)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contract: read journal entries: %w", err)
	}
	return entries, nil
}

// classify reads the DocumentType a journal entry's own basis columns
// carry. It returns the zero DocumentType when none of the known basis
// columns is set — the shape a later migration's new arc column
// produces, and which NewRegistry can never have a declaration for.
func classify(isPlayerOperation, isManualCorrection bool, eventType, deferredType, recurrentType *string) DocumentType {
	switch {
	case isPlayerOperation:
		return playerOperation()
	case isManualCorrection:
		return ManualCorrection()
	case eventType != nil:
		return Event(store.EventType(*eventType))
	case deferredType != nil:
		return DeferredTask(*deferredType)
	case recurrentType != nil:
		return RecurrentTask(*recurrentType)
	default:
		return DocumentType{}
	}
}

// readPostings reads every posting on a journal entry with an id greater
// than mark.id, keyed by its journal entry id.
func readPostings(ctx context.Context, q Queryer, mark Mark) (map[int64][]postingRow, error) {
	rows, err := q.Query(ctx, `
SELECT p.journal_entry_id, sd.code, ad.code, ad.kind, p.amount
FROM posting p
JOIN journal_entry je ON je.id = p.journal_entry_id
JOIN account a ON a.id = p.account_id
JOIN account_definition ad ON ad.id = a.account_definition_id
JOIN scope s ON s.id = a.scope_id
JOIN scope_definition sd ON sd.id = s.scope_definition_id
WHERE je.id > $1
ORDER BY p.journal_entry_id`, mark.id)
	if err != nil {
		return nil, fmt.Errorf("contract: read postings: %w", err)
	}
	defer rows.Close()

	byEntry := make(map[int64][]postingRow)
	for rows.Next() {
		var (
			entryID            int64
			scopeCode, account string
			kind               store.Kind
			amount             decimal.Decimal
		)
		if err := rows.Scan(&entryID, &scopeCode, &account, &kind, &amount); err != nil {
			return nil, fmt.Errorf("contract: scan posting: %w", err)
		}
		sign := Positive
		if amount.IsNegative() {
			sign = Negative
		}
		byEntry[entryID] = append(byEntry[entryID], postingRow{
			key:    postingKey{scopeCode: scopeCode, accountCode: account, kind: kind, sign: sign},
			amount: amount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contract: read postings: %w", err)
	}
	return byEntry, nil
}

// readMovements reads every item movement on a journal entry with an id
// greater than mark.id, keyed by its journal entry id.
func readMovements(ctx context.Context, q Queryer, mark Mark) (map[int64][]movementRow, error) {
	rows, err := q.Query(ctx, `
SELECT im.journal_entry_id, fsd.code, tsd.code
FROM item_movement im
JOIN journal_entry je ON je.id = im.journal_entry_id
JOIN scope fs ON fs.id = im.from_holder_id
JOIN scope_definition fsd ON fsd.id = fs.scope_definition_id
JOIN scope ts ON ts.id = im.to_holder_id
JOIN scope_definition tsd ON tsd.id = ts.scope_definition_id
WHERE je.id > $1
ORDER BY im.journal_entry_id`, mark.id)
	if err != nil {
		return nil, fmt.Errorf("contract: read item movements: %w", err)
	}
	defer rows.Close()

	byEntry := make(map[int64][]movementRow)
	for rows.Next() {
		var (
			entryID          int64
			fromCode, toCode string
		)
		if err := rows.Scan(&entryID, &fromCode, &toCode); err != nil {
			return nil, fmt.Errorf("contract: scan item movement: %w", err)
		}
		byEntry[entryID] = append(byEntry[entryID], movementRow{key: movementKey{fromScopeCode: fromCode, toScopeCode: toCode}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contract: read item movements: %w", err)
	}
	return byEntry, nil
}

// entryFailure names one entry a class of failure concerns, for
// rendering into a joined error.
type entryFailure struct {
	id         int64
	docType    DocumentType
	mismatches []mismatch
}

// String renders f as "#<id> <type>", with its mismatches when present.
func (f entryFailure) String() string {
	if len(f.mismatches) == 0 {
		return fmt.Sprintf("#%d %s", f.id, f.docType)
	}
	parts := make([]string, len(f.mismatches))
	for i, m := range f.mismatches {
		parts[i] = m.String()
	}
	return fmt.Sprintf("#%d %s (%s)", f.id, f.docType, strings.Join(parts, "; "))
}

// renderFailures joins every entryFailure's rendering with "; ".
func renderFailures(failures []entryFailure) string {
	parts := make([]string, len(failures))
	for i, f := range failures {
		parts[i] = f.String()
	}
	return strings.Join(parts, "; ")
}

// Check judges every journal entry with an id greater than mark's, each
// by its own type's declared Signature — never by want's.
//
// It returns ErrNoSignature immediately if want itself has no declared
// Signature in r. Otherwise it returns a joined error (errors.Join)
// carrying one wrap per class of failure present: ErrNoDocument if no
// entry of type want exists past mark, ErrNoSignature naming every entry
// whose own type has no declared Signature (including every
// player-operation entry, and an entry whose basis this package does not
// recognise), and ErrNonconforming naming every entry whose actual
// postings or item movements do not match its own type's Signature —
// including a set that does not sum to zero per kind, which only a write
// that bypasses this project's write path can produce.
//
// Check's precondition: no other writer adds a journal entry to the
// schema q reads from between the call to NewMark that produced mark and
// this call. A schema built for one test, and read by nothing else,
// meets it by construction.
func (r *Registry) Check(ctx context.Context, q Queryer, mark Mark, want DocumentType) error {
	if _, ok := r.byType[want]; !ok {
		return fmt.Errorf("%w: %s", ErrNoSignature, want)
	}

	entries, err := readEntries(ctx, q, mark)
	if err != nil {
		return err
	}
	postingsByEntry, err := readPostings(ctx, q, mark)
	if err != nil {
		return err
	}
	movementsByEntry, err := readMovements(ctx, q, mark)
	if err != nil {
		return err
	}

	foundWant := false
	var unsigned []entryFailure
	var nonconforming []entryFailure
	for _, e := range entries {
		if e.docType == want {
			foundWant = true
		}
		sig, ok := r.byType[e.docType]
		if !ok {
			unsigned = append(unsigned, entryFailure{id: e.id, docType: e.docType})
			continue
		}
		doc := document{postings: postingsByEntry[e.id], movements: movementsByEntry[e.id]}
		if mismatches := conform(sig, doc); len(mismatches) > 0 {
			nonconforming = append(nonconforming, entryFailure{id: e.id, docType: e.docType, mismatches: mismatches})
		}
	}

	var errs []error
	if !foundWant {
		errs = append(errs, fmt.Errorf("%w: %s", ErrNoDocument, want))
	}
	if len(unsigned) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", ErrNoSignature, renderFailures(unsigned)))
	}
	if len(nonconforming) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", ErrNonconforming, renderFailures(nonconforming)))
	}
	return errors.Join(errs...)
}
