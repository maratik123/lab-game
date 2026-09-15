package contract

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/maratik123/lab-game/internal/store"
)

// fakeRow is a pgx.Row whose Scan returns a fixed error.
type fakeRow struct {
	err error
}

func (r fakeRow) Scan(...any) error {
	return r.err
}

// fakeRowsStage names which of a fakeRows' methods fails.
type fakeRowsStage int

const (
	// fakeRowsEmpty yields no row and no error: every read before the
	// chosen one in a TestReads_returnEveryError case.
	fakeRowsEmpty fakeRowsStage = iota
	// fakeRowsQueryErr: the Query call itself returns the error, before
	// any fakeRows exists.
	fakeRowsQueryErr
	// fakeRowsScanErr yields one row whose Scan returns the error.
	fakeRowsScanErr
	// fakeRowsErrAfter yields no row, and Err returns the error.
	fakeRowsErrAfter
)

// fakeRows is a pgx.Rows implementing every method the interface
// declares, driven by stage.
type fakeRows struct {
	stage   fakeRowsStage
	err     error
	yielded bool
}

func (r *fakeRows) Close() {}

func (r *fakeRows) Err() error {
	if r.stage == fakeRowsErrAfter {
		return r.err
	}
	return nil
}

func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	if r.stage != fakeRowsScanErr {
		return false
	}
	if r.yielded {
		return false
	}
	r.yielded = true
	return true
}

func (r *fakeRows) Scan(...any) error {
	if r.stage == fakeRowsScanErr {
		return r.err
	}
	return nil
}

func (r *fakeRows) Values() ([]any, error) { return nil, nil }

func (r *fakeRows) RawValues() [][]byte { return nil }

func (r *fakeRows) Conn() *pgx.Conn { return nil }

// readKind names which of Check's three reads a fakeQueryer's Query
// calls answer, in call order: entries, then postings, then movements.
type readKind int

const (
	readKindEntries readKind = iota
	readKindPostings
	readKindMovements
)

// fakeQueryer answers Check's reads in call order. Every call before
// failAt gets an empty fakeRows; the call at failAt fails at stage. Its
// QueryRow, used only by NewMark, always answers rowErr.
type fakeQueryer struct {
	rowErr error
	failAt readKind
	stage  fakeRowsStage
	err    error
	calls  int
}

func (f *fakeQueryer) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeRow{err: f.rowErr}
}

func (f *fakeQueryer) Query(context.Context, string, ...any) (pgx.Rows, error) {
	idx := readKind(f.calls)
	f.calls++
	if idx != f.failAt {
		return &fakeRows{stage: fakeRowsEmpty}, nil
	}
	switch f.stage {
	case fakeRowsScanErr, fakeRowsErrAfter:
		return &fakeRows{stage: f.stage, err: f.err}, nil
	default:
		return nil, f.err
	}
}

func TestNewMark_scanError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("probe: scan failed")
	q := &fakeQueryer{rowErr: wantErr}
	_, err := NewMark(context.Background(), q)
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewMark() error = %v, want errors.Is %v", err, wantErr)
	}
}

func TestReads_returnEveryError(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(Declaration{Type: ManualCorrection(), Signature: AnyBalanced()})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cases := []struct {
		name   string
		failAt readKind
		stage  fakeRowsStage
	}{
		{name: "entries: Query error", failAt: readKindEntries, stage: fakeRowsQueryErr},
		{name: "entries: scan error", failAt: readKindEntries, stage: fakeRowsScanErr},
		{name: "entries: rows.Err error", failAt: readKindEntries, stage: fakeRowsErrAfter},
		{name: "postings: Query error", failAt: readKindPostings, stage: fakeRowsQueryErr},
		{name: "postings: scan error", failAt: readKindPostings, stage: fakeRowsScanErr},
		{name: "postings: rows.Err error", failAt: readKindPostings, stage: fakeRowsErrAfter},
		{name: "movements: Query error", failAt: readKindMovements, stage: fakeRowsQueryErr},
		{name: "movements: scan error", failAt: readKindMovements, stage: fakeRowsScanErr},
		{name: "movements: rows.Err error", failAt: readKindMovements, stage: fakeRowsErrAfter},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wantErr := errors.New("probe: " + tc.name)
			q := &fakeQueryer{failAt: tc.failAt, stage: tc.stage, err: wantErr}
			err := reg.Check(context.Background(), q, Mark{}, ManualCorrection())
			if !errors.Is(err, wantErr) {
				t.Fatalf("Check() error = %v, want errors.Is %v", err, wantErr)
			}
			for _, sentinel := range []error{ErrInvalidDeclaration, ErrNonconforming, ErrNoDocument, ErrNoSignature} {
				if errors.Is(err, sentinel) {
					t.Errorf("Check() error = %v, want it not to match %v", err, sentinel)
				}
			}
		})
	}
}

func TestCheck_wantWithNoSignature(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(Declaration{Type: ManualCorrection(), Signature: AnyBalanced()})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	q := &fakeQueryer{failAt: readKind(-1)} // never matches any real call index; Check must not call Query at all
	err = reg.Check(context.Background(), q, Mark{}, Event(store.EventShopSale))
	if !errors.Is(err, ErrNoSignature) {
		t.Fatalf("Check() error = %v, want errors.Is ErrNoSignature", err)
	}
	if q.calls != 0 {
		t.Errorf("Check() issued %d read(s), want 0 (refused before reading anything)", q.calls)
	}
}
