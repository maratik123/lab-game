package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// noopHandler is a Handler that does nothing, for router scenarios that
// only care about registration.
type noopHandler struct{}

func (noopHandler) Handle(context.Context, pgx.Tx, Update) error { return nil }

// TestNewRouter_duplicateKindRefused asserts registering the same Kind
// twice is refused (design D4).
func TestNewRouter_duplicateKindRefused(t *testing.T) {
	t.Parallel()

	_, err := NewRouter(
		Route{Kind: KindMessage, Handler: noopHandler{}},
		Route{Kind: KindMessage, Handler: noopHandler{}},
	)
	if !errors.Is(err, ErrDuplicateRoute) {
		t.Errorf("NewRouter with a duplicate kind: error = %v, want it to wrap ErrDuplicateRoute", err)
	}
}

// TestNewRouter_unknownKindRefused asserts registering a Kind with no
// kindTable row is refused (design D4 — a route can never be registered
// into a hole).
func TestNewRouter_unknownKindRefused(t *testing.T) {
	t.Parallel()

	_, err := NewRouter(Route{Kind: Kind("not_a_real_kind"), Handler: noopHandler{}})
	if !errors.Is(err, ErrUnknownKind) {
		t.Errorf("NewRouter with an unknown kind: error = %v, want it to wrap ErrUnknownKind", err)
	}
}

// TestRouter_kindsDeterministicOrder asserts Kinds returns the registered
// set in a deterministic (sorted) order, independent of registration
// order.
func TestRouter_kindsDeterministicOrder(t *testing.T) {
	t.Parallel()

	r, err := NewRouter(
		Route{Kind: KindPoll, Handler: noopHandler{}},
		Route{Kind: KindMessage, Handler: noopHandler{}},
		Route{Kind: KindCallbackQuery, Handler: noopHandler{}},
	)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Sorted lexicographically: callback_query < message < poll.
	want := []Kind{KindCallbackQuery, KindMessage, KindPoll}

	got := r.Kinds()
	if len(got) != len(want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Kinds()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Called twice: same order both times (deterministic, not merely
	// stable-by-accident).
	got2 := r.Kinds()
	for i := range got2 {
		if got2[i] != got[i] {
			t.Errorf("Kinds() second call[%d] = %q, want %q (same as first call)", i, got2[i], got[i])
		}
	}
}

// TestRouter_lookupUnregisteredKindReportsAbsence asserts a lookup for a
// kind nobody registered reports absence, not a zero Handler mistaken
// for a real one.
func TestRouter_lookupUnregisteredKindReportsAbsence(t *testing.T) {
	t.Parallel()

	r, err := NewRouter(Route{Kind: KindMessage, Handler: noopHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if _, ok := r.Lookup(KindCallbackQuery); ok {
		t.Error("Lookup(KindCallbackQuery) reported present, want absent (never registered)")
	}
	if _, ok := r.Lookup(KindMessage); !ok {
		t.Error("Lookup(KindMessage) reported absent, want present (registered)")
	}
}

// TestNewRouter_empty asserts an empty router builds successfully with
// an empty Kinds set — the routeless-loop case D3 relies on.
func TestNewRouter_empty(t *testing.T) {
	t.Parallel()

	r, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter(): %v", err)
	}
	if got := r.Kinds(); len(got) != 0 {
		t.Errorf("Kinds() = %v, want empty", got)
	}
}
