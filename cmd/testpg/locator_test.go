package main

import (
	"os"
	"path/filepath"
	"testing"
)

// withLocatorDir runs the test in a temporary directory so it exercises the
// real locatorPath ("tmp/testpg-dsn") without touching this checkout's own
// locator file.
func withLocatorDir(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

func TestLocator_roundTripsTheClientCountItWasWrittenWith(t *testing.T) {
	withLocatorDir(t)

	const dsn = "postgres://labgame:labgame@localhost:5432/labgame_test?sslmode=disable"
	if err := writeLocator(dsn, 4); err != nil {
		t.Fatalf("writeLocator: %v", err)
	}

	gotDSN, gotClients, ok := readLocator()
	if !ok {
		t.Fatalf("readLocator: ok = false, want true")
	}
	if gotDSN != dsn {
		t.Errorf("readLocator DSN = %q, want %q", gotDSN, dsn)
	}
	if gotClients != 4 {
		t.Errorf("readLocator clients = %d, want 4", gotClients)
	}
}

func TestLocator_legacyDSNOnlyFile_parsesAsOneClient(t *testing.T) {
	withLocatorDir(t)

	const dsn = "postgres://labgame:labgame@localhost:5432/labgame_test?sslmode=disable"
	if err := os.MkdirAll(filepath.Dir(locatorPath), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(locatorPath, []byte(dsn+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gotDSN, gotClients, ok := readLocator()
	if !ok {
		t.Fatalf("readLocator: ok = false, want true")
	}
	if gotDSN != dsn {
		t.Errorf("readLocator DSN = %q, want %q", gotDSN, dsn)
	}
	if gotClients != 1 {
		t.Errorf("readLocator clients = %d, want 1 (legacy fail-closed default)", gotClients)
	}
}

func TestLocator_unparseableSecondLine_parsesAsOneClient(t *testing.T) {
	withLocatorDir(t)

	const dsn = "postgres://labgame:labgame@localhost:5432/labgame_test?sslmode=disable"
	if err := os.MkdirAll(filepath.Dir(locatorPath), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(locatorPath, []byte(dsn+"\nnot-a-number\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, gotClients, ok := readLocator()
	if !ok {
		t.Fatalf("readLocator: ok = false, want true")
	}
	if gotClients != 1 {
		t.Errorf("readLocator clients = %d, want 1 for an unparseable count", gotClients)
	}
}

func TestLocator_absentFile_isNotOK(t *testing.T) {
	withLocatorDir(t)

	_, _, ok := readLocator()
	if ok {
		t.Errorf("readLocator: ok = true for an absent file, want false")
	}
}

func TestLocator_writeLocator_belowOneClient_recordsAsOne(t *testing.T) {
	withLocatorDir(t)

	if err := writeLocator("postgres://x/y", 0); err != nil {
		t.Fatalf("writeLocator: %v", err)
	}

	_, gotClients, ok := readLocator()
	if !ok {
		t.Fatalf("readLocator: ok = false, want true")
	}
	if gotClients != 1 {
		t.Errorf("readLocator clients = %d, want 1 for a non-positive write", gotClients)
	}
}
