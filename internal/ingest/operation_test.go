package ingest

import (
	"errors"
	"strings"
	"testing"
)

// TestOperationID_differentSpacesSameRawID asserts the update_id and
// callback_query.id spaces produce different values from the same raw
// identifier.
func TestOperationID_differentSpacesSameRawID(t *testing.T) {
	t.Parallel()

	const raw = "42"
	update, err := operationID(IDSpaceUpdate, raw)
	if err != nil {
		t.Fatalf("operationID(IDSpaceUpdate, %q): %v", raw, err)
	}
	callback, err := operationID(IDSpaceCallbackQuery, raw)
	if err != nil {
		t.Fatalf("operationID(IDSpaceCallbackQuery, %q): %v", raw, err)
	}
	if update == callback {
		t.Errorf("operationID(IDSpaceUpdate, %q) = operationID(IDSpaceCallbackQuery, %q) = %q, want different values", raw, raw, update)
	}
}

// TestOperationID_grammar asserts every derived value carries a
// non-empty space token, the separator, and a non-empty identifier.
func TestOperationID_grammar(t *testing.T) {
	t.Parallel()

	got, err := operationID(IDSpaceUpdate, "7")
	if err != nil {
		t.Fatalf("operationID: %v", err)
	}
	space, id, found := strings.Cut(got, operationIDSeparator)
	if !found {
		t.Fatalf("operationID = %q, want it to contain the separator %q", got, operationIDSeparator)
	}
	if space == "" {
		t.Errorf("operationID = %q, want a non-empty space token before the separator", got)
	}
	if id == "" {
		t.Errorf("operationID = %q, want a non-empty id after the separator", got)
	}
}

// TestOperationID_emptySpaceRefused and TestOperationID_emptyIDRefused
// pin the two refusal rows.
func TestOperationID_emptySpaceRefused(t *testing.T) {
	t.Parallel()

	_, err := operationID("", "7")
	if !errors.Is(err, ErrEmptyIDSpace) {
		t.Errorf("operationID(\"\", \"7\") error = %v, want it to wrap ErrEmptyIDSpace", err)
	}
}

func TestOperationID_emptyIDRefused(t *testing.T) {
	t.Parallel()

	_, err := operationID(IDSpaceUpdate, "")
	if !errors.Is(err, ErrEmptyID) {
		t.Errorf("operationID(IDSpaceUpdate, \"\") error = %v, want it to wrap ErrEmptyID", err)
	}
}

// TestIDSpace_noneContainsTheSeparator asserts every declared IDSpace
// member is free of the grammar's own separator, so a raw operation_id
// can always be split unambiguously.
func TestIDSpace_noneContainsTheSeparator(t *testing.T) {
	t.Parallel()

	for _, space := range []IDSpace{IDSpaceUpdate, IDSpaceCallbackQuery} {
		if strings.Contains(string(space), operationIDSeparator) {
			t.Errorf("IDSpace %q contains the separator %q", space, operationIDSeparator)
		}
	}
}
