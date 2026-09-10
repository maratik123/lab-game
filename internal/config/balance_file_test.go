package config

import (
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

// TestBalanceFile_LoadsAndAgrees is the tracked balance file's agreement
// test: a single successful load through the same balance loader other
// tests use is, by construction, the proof that no schema key is absent
// from the tracked balance file (loadBalance would report ErrMissing) and
// that no key in the file is unknown to the schema (loadBalance would
// report ErrUnknownKey) — so the test asserts the success rather than
// re-deriving either key set by hand.
//
// The curve comments (the door-price and monster-budget key triples) are
// documentation for the generation/door work; no code reads them, so this
// test deliberately does not assert on their text.
func TestBalanceFile_LoadsAndAgrees(t *testing.T) {
	t.Parallel()
	path := repotest.RootPath(t, "config/balance.yaml")
	if _, err := loadBalance(path); err != nil {
		t.Fatalf("loadBalance(%s): %v — the tracked balance file and the schema have drifted apart", path, err)
	}
}
