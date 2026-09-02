package store

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// appendOnlyPattern is the spec's AC9 pattern (KD-3): no statement may
// UPDATE or DELETE FROM the ledger tables posting and journal_entry.
var appendOnlyPattern = regexp.MustCompile(`(?i)update\s+(posting|journal_entry)\b|delete\s+from\s+(posting|journal_entry)\b`)

// TestAppendOnly_no_update_or_delete_on_ledger_tables is the in-suite twin
// of the Step-9 AC9 sweep (design § Test Design → AC9): every non-test Go
// source of this package and every embedded migration is scanned for a
// statement that would rewrite ledger history. The positive control runs
// first so a broken pattern fails the test before any file is scanned.
func TestAppendOnly_no_update_or_delete_on_ledger_tables(t *testing.T) {
	t.Parallel()

	// Positive control: the pattern must match each planted shape and
	// neither decoy, or the scan below proves nothing.
	for _, planted := range []string{
		"UPDATE posting SET amount = 0",
		"delete from posting where id = 1",
		"UPDATE journal_entry SET ts = now()",
		"delete from journal_entry where id = 1",
	} {
		if !appendOnlyPattern.MatchString(planted) {
			t.Fatalf("positive control failed: pattern does not match %q", planted)
		}
	}
	for _, decoy := range []string{
		"update postings_archive set amount = 0",
		"delete from journal_entry_archive where id = 1",
	} {
		if appendOnlyPattern.MatchString(decoy) {
			t.Fatalf("positive control failed: pattern matches the decoy %q", decoy)
		}
	}

	// Non-test Go sources of this package. The test binary's working
	// directory is the package directory; _test.go files are excluded
	// because post_test.go carries the planted control lines.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var scanned []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned = append(scanned, name)
		reportHits(t, name, content)
	}

	// Embedded migrations: a migration is a code path under KD-3 too.
	migrations, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	for _, entry := range migrations {
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		scanned = append(scanned, "migrations/"+entry.Name())
		reportHits(t, "migrations/"+entry.Name(), content)
	}

	// Non-vacuity guard: a scan that never saw the ledger's own source or
	// its first migration proves nothing (a cwd surprise or an empty embed
	// would otherwise pass with zero hits). The message lists what was
	// scanned so a real zero is distinguishable from a vacuous one.
	if !slices.Contains(scanned, "post.go") || !slices.Contains(scanned, "migrations/00001_ledger_core.sql") {
		t.Fatalf("append-only scan is vacuous: post.go and migrations/00001_ledger_core.sql must be in the scanned set; scanned %d files: %v", len(scanned), scanned)
	}
}

// reportHits records one failure per forbidden statement found in content.
func reportHits(t *testing.T, name string, content []byte) {
	t.Helper()
	for _, loc := range appendOnlyPattern.FindAllIndex(content, -1) {
		t.Errorf("%s: forbidden statement on a ledger table: %q", name, content[loc[0]:loc[1]])
	}
}
