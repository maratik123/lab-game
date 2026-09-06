package store

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// appendOnlyPattern is the spec's AC9 pattern (KD-3), extended by this task's
// AC10 to the event table: no statement may UPDATE or DELETE FROM the
// append-only tables posting, journal_entry and event. The word boundary
// after each alternative is what keeps "event" from matching
// "event_type_definition" — that table is a seeded catalog, not append-only,
// and is exercised by the decoy loop below.
var appendOnlyPattern = regexp.MustCompile(`(?i)update\s+(posting|journal_entry|event)\b|delete\s+from\s+(posting|journal_entry|event)\b`)

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
		"UPDATE event SET payload = '{}'",
		"delete from event where id = 1",
	} {
		if !appendOnlyPattern.MatchString(planted) {
			t.Fatalf("positive control failed: pattern does not match %q", planted)
		}
	}
	for _, decoy := range []string{
		"update postings_archive set amount = 0",
		"delete from journal_entry_archive where id = 1",
		// event_type_definition is a seeded catalog, not append-only; the
		// word boundary after "event" must not spill into its name.
		"update event_type_definition set volume_class = 'low_volume'",
		"delete from event_type_definition where id = 1",
	} {
		if appendOnlyPattern.MatchString(decoy) {
			t.Fatalf("positive control failed: pattern matches the decoy %q", decoy)
		}
	}

	// Collect first: non-test Go sources of this package (the test binary's
	// working directory is the package directory; _test.go files are
	// excluded because post_test.go carries the planted control lines) and
	// the embedded migrations (a migration is a code path under KD-3 too).
	type source struct {
		name    string
		content string
	}
	var sources []source
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sources = append(sources, source{name: name, content: string(content)})
	}
	migrations, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	for _, entry := range migrations {
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatalf("read migration %s: %v", entry.Name(), err)
		}
		sources = append(sources, source{name: "migrations/" + entry.Name(), content: string(content)})
	}

	// Non-vacuity guard, between collecting and scanning: a scan that never
	// saw the ledger's own source or its first migration proves nothing (a
	// cwd surprise or an empty embed would otherwise pass with zero hits).
	// The message lists what was collected so a real zero is
	// distinguishable from a vacuous one.
	names := make([]string, 0, len(sources))
	for _, src := range sources {
		names = append(names, src.name)
	}
	if !slices.Contains(names, "post.go") || !slices.Contains(names, "migrations/00001_ledger_core.sql") ||
		!slices.Contains(names, "event.go") || !slices.Contains(names, "migrations/00003_event_log.sql") {
		t.Fatalf("append-only scan is vacuous: post.go, event.go, migrations/00001_ledger_core.sql and migrations/00003_event_log.sql must be in the scanned set; collected %d files: %v", len(names), names)
	}

	// Scan: one failure per forbidden statement, naming file and match.
	for _, src := range sources {
		for _, loc := range appendOnlyPattern.FindAllStringIndex(src.content, -1) {
			t.Errorf("%s: forbidden statement on a ledger table: %q", src.name, src.content[loc[0]:loc[1]])
		}
	}
}
