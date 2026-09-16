package config

import (
	"slices"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/store"
)

// TestWorldFile_LoadsAndAgrees is the tracked world set's agreement test:
// a single successful load through the same world-set loader other tests
// use is, by construction, the proof that no schema key is absent from
// the tracked world file and no key in it is unknown to the schema — the
// balance file's own agreement argument.
func TestWorldFile_LoadsAndAgrees(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "config/world")
	worlds, err := loadWorldSet(dir)
	if err != nil {
		t.Fatalf("loadWorldSet(%s): %v — the tracked world set and the schema have drifted apart", dir, err)
	}
	if len(worlds) != 1 {
		t.Fatalf("worlds = %+v, want exactly one world", worlds)
	}
	if worlds[0].ID != "cotton_candy" {
		t.Errorf("worlds[0].ID = %q, want cotton_candy", worlds[0].ID)
	}
}

// TestWorldFile_ResourceKindsAreLedgerMembers reads the ledger's own kind
// list from a test file only, so the production package this test lives
// beside stays free of a storage dependency: every resource kind the
// tracked world set names is a member of the ledger's kind list.
func TestWorldFile_ResourceKindsAreLedgerMembers(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "config/world")
	worlds, err := loadWorldSet(dir)
	if err != nil {
		t.Fatalf("loadWorldSet(%s): %v", dir, err)
	}

	ledgerKinds := store.Kinds()
	for _, w := range worlds {
		for _, r := range w.ResourceProfile {
			if !slices.Contains(ledgerKinds, store.Kind(r.Kind)) {
				t.Errorf("world %q names resource kind %q, which is not a ledger_kind member (%v)", w.ID, r.Kind, ledgerKinds)
			}
		}
	}
}
