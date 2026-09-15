package storetest_test

import (
	"context"
	"testing"

	"github.com/maratik123/lab-game/internal/storetest"
)

// TestPool_migratedAndEmpty proves the pool Pool returns is migrated (the
// journal_entry table exists) and seeds no journal entry.
func TestPool_migratedAndEmpty(t *testing.T) {
	ctx := context.Background()
	pool := storetest.Pool(t)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM journal_entry`).Scan(&count); err != nil {
		t.Fatalf("count journal_entry: %v", err)
	}
	if count != 0 {
		t.Fatalf("journal_entry count = %d, want 0", count)
	}
}
