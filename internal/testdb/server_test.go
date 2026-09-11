package testdb

import (
	"context"
	"go/ast"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

func TestCeiling_formula(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		clients, parallel int
		want              int
		wantErr           bool
	}{
		{
			name: "small parallelism floors at the image default",
			// 1 * Binaries(5) * 1 * (schemaMaxConns(4)+1) + ceilingSlack(32) = 57,
			// below imageDefaultCeiling(100).
			clients: 1, parallel: 1,
			want: imageDefaultCeiling,
		},
		{
			name: "mid-sized parallelism matches the formula exactly",
			// 2 * 5 * 3 * 5 + 32 = 182, above the floor.
			clients: 2, parallel: 3,
			want: 182,
		},
		{
			name:    "clients and parallel below one are treated as one",
			clients: 0, parallel: 0,
			want: imageDefaultCeiling,
		},
		{
			name: "above ceilingMax the wrapper refuses rather than clamps",
			// 10 * 5 * 10 * 5 + 32 = 2532, above ceilingMax(1000).
			clients: 10, parallel: 10,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Ceiling(tc.clients, tc.parallel)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Ceiling(%d, %d) = %d, nil; want a refusal error", tc.clients, tc.parallel, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Ceiling(%d, %d): unexpected error: %v", tc.clients, tc.parallel, err)
			}
			if got != tc.want {
				t.Errorf("Ceiling(%d, %d) = %d, want %d", tc.clients, tc.parallel, got, tc.want)
			}
		})
	}
}

// TestProbe_matchesOrdinaryQuery asserts the probe's answer against the
// server the run is already using agrees with reading max_connections
// through an ordinary pool query, so the probe is not a second, divergent
// source of truth.
func TestProbe_matchesOrdinaryQuery(t *testing.T) {
	t.Parallel()

	if baseDSN == "" {
		t.Fatalf("testdb: Probe test needs Main to have provisioned a database first")
	}

	ctx := context.Background()
	got, err := Probe(ctx, baseDSN)
	if err != nil {
		t.Fatalf("Probe(baseDSN): unexpected error: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("parse base DSN: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var want int
	if err := pool.QueryRow(ctx, "SELECT setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&want); err != nil {
		t.Fatalf("read max_connections: %v", err)
	}

	if got != want {
		t.Errorf("Probe returned %d, ordinary query returned %d", got, want)
	}
}

// TestProbe_unreachablePort asserts a DSN naming a port nothing listens on
// returns an error rather than hanging or panicking.
func TestProbe_unreachablePort(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if _, err := Probe(ctx, "postgres://labgame:labgame@127.0.0.1:1/nosuchdb?sslmode=disable"); err == nil {
		t.Fatalf("Probe against an unreachable port: want an error, got nil")
	}
}

// TestProbe_invalidDSN asserts a syntactically invalid DSN returns an error
// without ever dialling — parsing fails before any connection attempt.
func TestProbe_invalidDSN(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if _, err := Probe(ctx, "not a dsn at all://\x00"); err == nil {
		t.Fatalf("Probe against a syntactically invalid DSN: want an error, got nil")
	}
}

// testdbImportPath is this package's own import path, as another test
// file's import block would spell it.
const testdbImportPath = "github.com/maratik123/lab-game/internal/testdb"

// testdbLocalName reports the identifier f's own import block binds this
// package's Main to — the import's alias if it declares one, "testdb"
// otherwise — and whether f imports this package at all.
func testdbLocalName(f *ast.File) (string, bool) {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != testdbImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return "testdb", true
	}
	return "", false
}

// selectsMain reports whether f's syntax tree holds a selector expression
// naming Main on the identifier local — called, passed as a value, or
// merely referenced. A string literal spelling the same text is not this
// syntax and is not counted.
func selectsMain(f *ast.File, local string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if ok && ident.Name == local && sel.Sel.Name == "Main" {
			found = true
			return false
		}
		return true
	})
	return found
}

// callersOfMain walks the module tree and returns the set of package
// directories, relative to root, whose test files hold a syntactic
// reference to this package's Main through their own import of it —
// derived from the tree rather than typed by hand, so it cannot drift
// from what the constant is meant to track.
func callersOfMain(t *testing.T, root string) map[string]bool {
	t.Helper()
	callers := map[string]bool{}

	srcguard.WalkSubtree(t, root, func(path string) {
		if srcguard.NonTestFile(path) || !strings.HasSuffix(path, "_test.go") {
			return
		}
		f := srcguard.ParseFile(t, path)
		local, ok := testdbLocalName(f)
		if !ok {
			return
		}
		if !selectsMain(f, local) {
			return
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			t.Fatalf("Rel(%s): %v", path, err)
		}
		callers[filepath.ToSlash(rel)] = true
	})
	return callers
}

// TestBinaries_matchesTree keeps the Binaries constant honest against the
// tree: a package added or removed from the set of testdb.Main callers must
// change the constant in the same commit, and this test fails by name in
// both directions when it does not.
func TestBinaries_matchesTree(t *testing.T) {
	t.Parallel()

	root := repotest.Root(t)
	got := callersOfMain(t, root)

	// This package's own tests call testdb.Main through the external test
	// package's TestMain, which this walk also finds — it counts itself,
	// exactly as the constant's doc comment says.
	var names []string
	for k := range got {
		names = append(names, k)
	}
	sort.Strings(names)

	if len(got) != Binaries {
		t.Errorf("found %d package(s) calling testdb.Main (%v), Binaries constant says %d", len(got), names, Binaries)
	}
}

// TestCallersOfMain_scratch drives callersOfMain over a scratch tree
// rather than the real one, so each way a test file can or cannot refer
// to Main is exercised directly: calling it, passing it as a value,
// reaching it through an aliased import, naming it only inside a string
// literal, and not referencing it at all.
func TestCallersOfMain_scratch(t *testing.T) {
	t.Parallel()

	root, _ := srcguard.WriteScratchFile(t, "calls/main_test.go", `package callspkg

import (
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	testdb.Main(m)
}
`)
	srcguard.WriteScratchFileIn(t, root, "value/main_test.go", `package valuepkg

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	run := testdb.Main
	os.Exit(run(m))
}
`)
	srcguard.WriteScratchFileIn(t, root, "aliased/main_test.go", `package aliasedpkg

import (
	"testing"

	tdb "github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	tdb.Main(m)
}
`)
	srcguard.WriteScratchFileIn(t, root, "literal/main_test.go", `package literalpkg

import "testing"

func TestSomething(t *testing.T) {
	_ = "testdb.Main("
}
`)
	srcguard.WriteScratchFileIn(t, root, "none/main_test.go", `package nonepkg

import "testing"

func TestSomething(t *testing.T) {}
`)

	got := callersOfMain(t, root)
	want := map[string]bool{"calls": true, "value": true, "aliased": true}
	if len(got) != len(want) {
		t.Fatalf("callersOfMain() = %v, want exactly %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("callersOfMain() missing %q", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("callersOfMain() unexpectedly includes %q", k)
		}
	}
}
