package testdb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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
			// 1 * Binaries(4) * 1 * (schemaMaxConns(4)+1) + ceilingSlack(32) = 52,
			// below imageDefaultCeiling(100).
			clients: 1, parallel: 1,
			want: imageDefaultCeiling,
		},
		{
			name: "mid-sized parallelism matches the formula exactly",
			// 2 * 4 * 3 * 5 + 32 = 152, above the floor.
			clients: 2, parallel: 3,
			want: 152,
		},
		{
			name:    "clients and parallel below one are treated as one",
			clients: 0, parallel: 0,
			want: imageDefaultCeiling,
		},
		{
			name: "above ceilingMax the wrapper refuses rather than clamps",
			// 10 * 4 * 10 * 5 + 32 = 2032, above ceilingMax(1000).
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

// repoRoot returns this module's root, derived from this file's own path
// rather than from the working directory, so the test is not sensitive to
// how `go test` was invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed")
	}
	// this file lives two directories below the module root.
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// callersOfMain walks the module tree and returns the set of package
// directories, relative to root, whose test files call testdb.Main —
// derived from the tree rather than typed by hand, so it cannot drift from
// what the constant is meant to track.
func callersOfMain(t *testing.T, root string) map[string]bool {
	t.Helper()
	callers := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), "testdb.Main(") {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			callers[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module tree: %v", err)
	}
	return callers
}

// TestBinaries_matchesTree keeps the Binaries constant honest against the
// tree: a package added or removed from the set of testdb.Main callers must
// change the constant in the same commit, and this test fails by name in
// both directions when it does not.
func TestBinaries_matchesTree(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
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
