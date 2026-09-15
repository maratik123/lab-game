package testdb

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
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
			// 1 * Binaries(7) * 1 * (schemaMaxConns(4)+1) + ceilingSlack(32) = 67,
			// below imageDefaultCeiling(100).
			clients: 1, parallel: 1,
			want: imageDefaultCeiling,
		},
		{
			name: "mid-sized parallelism matches the formula exactly",
			// 2 * 7 * 3 * 5 + 32 = 242, above the floor.
			clients: 2, parallel: 3,
			want: 242,
		},
		{
			name:    "clients and parallel below one are treated as one",
			clients: 0, parallel: 0,
			want: imageDefaultCeiling,
		},
		{
			name: "above ceilingMax the wrapper refuses rather than clamps",
			// 10 * 7 * 10 * 5 + 32 = 3532, above ceilingMax(1000).
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

// walCheckpointFactor is how many times its WAL retention target a cluster
// is allowed to reach here before the mount is called too small for it. A
// checkpoint starts when that much WAL has accumulated and keeps writing
// while it runs, so the directory peaks above the target rather than at it;
// PostgreSQL documents the target as a soft limit for exactly that reason.
//
// clusterPeakMB is one client's PEAK cluster size under sustained load, and
// the WAL has to fit beside it, not instead of it.
//
// It is a peak rather than a residue on purpose: what exhausts the mount is
// the high-water mark while tests run, not what they leave behind when they
// stop, and the two differ by more than a safety factor covers — a full run
// of every database-backed package against one server *leaves* 193 MB, while
// the same packages under the contention target's own load peak at about
// 1.7× that per client.
//
// Re-measure rather than derive, because a doubling on paper is what made
// this number wrong before. The recipe, which is the durable part of this
// comment:
//
//  1. Start a server this package would start, except with a PGDATA mount
//     far larger than any value under consideration, so the cap cannot
//     truncate the measurement.
//  2. Point the suite at it through LAB_GAME_TEST_DSN, which the wrapper
//     uses as it stands.
//  3. Run the contention target against it, so both of its concurrent
//     whole-module workloads are on the mount at once.
//  4. Sample the mount's used space every half second FOR THE WHOLE RUN,
//     and take the maximum; divide by the client count the target passes.
//
// Step 4's coverage requirement is load-bearing, not advice: a sampler that
// spans part of a run reports a floor, not a peak. Measured on one workload,
// a sampler covering 37 s of a 45 s run read 416 MB where a fully covering
// one read 697 MB — the under-covering figure would have sized the mount
// below what the run actually needs, and the mount's own failure mode is a
// server PANIC, not a slow test.
const (
	walCheckpointFactor = 2
	clusterPeakMB       = 320

	// clusterPeakMeasuredAtClients is the client count clusterPeakMB was
	// measured at (step 3 of the recipe above ran the contention target,
	// which at the time of measurement provisioned two clients). The figure
	// is a per-client peak, but nothing establishes that a per-client peak
	// holds at a higher count — more clients sharing one server contend for
	// the same buffers, autovacuum workers and checkpoint I/O, so the peak
	// each one reaches is not guaranteed to be independent of how many
	// others are running beside it. A target that raises the provisioned
	// count past this figure is running on an assumption the measurement
	// never covered, and the budget inequality below cannot catch that: the
	// per-client mount grant scales with the count exactly as the peak term
	// does, so raising the count moves both sides together and the
	// inequality holds regardless of how high it goes.
	clusterPeakMeasuredAtClients = 2
)

// reClientsFlag matches a literal client count passed to the test-server
// wrapper. A count spelled as a variable reference is deliberately not
// matched: this test can only bound counts it can read.
var reClientsFlag = regexp.MustCompile(`--clients[[:space:]]+([0-9]+)`)

// targetClients returns the largest literal --clients count any target in
// this repository provisions a server for, so a target that raises the count
// without raising the mount fails this budget assertion by name instead of
// failing as disk exhaustion at run time. It fails tb when no literal count
// is found: an assertion whose input set came back empty reports a clean
// result for every possible mount size, which is the one answer it must
// never give silently.
func targetClients(tb testing.TB) int {
	tb.Helper()

	build, err := os.ReadFile(repotest.RootPath(tb, "Makefile"))
	if err != nil {
		tb.Fatalf("read the build file: %v", err)
	}

	most, err := mostClientsIn(string(build))
	if err != nil {
		tb.Fatalf("read the provisioned client counts: %v", err)
	}
	return most
}

// mostClientsIn returns the largest literal --clients count in content. It
// returns an error when there is none, rather than zero: zero would make
// every budget fit and report a clean result the input never established.
func mostClientsIn(content string) (int, error) {
	matches := reClientsFlag.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return 0, errors.New("no literal --clients count found: this assertion would bound nothing")
	}

	most := 0
	for _, m := range matches {
		clients, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("--clients %q is not a count: %w", m[1], err)
		}
		most = max(most, clients)
	}
	return most, nil
}

// mountCapMB returns the megabyte cap declared by a tmpfs mount-option
// string of the form this package builds — "rw,size=512m". It fails tb
// rather than returning an error, because a size this package cannot read
// back out of its own constant is a defect in the constant.
func mountCapMB(tb testing.TB, options string) int {
	tb.Helper()

	for opt := range strings.SplitSeq(options, ",") {
		size, ok := strings.CutPrefix(opt, "size=")
		if !ok {
			continue
		}
		mb, err := strconv.Atoi(strings.TrimSuffix(size, "m"))
		if err != nil {
			tb.Fatalf("tmpfs size option %q is not a megabyte count: %v", opt, err)
		}
		return mb
	}

	tb.Fatalf("tmpfs options %q declare no size", options)
	return 0
}

// TestMountOptions drives MountOptions over the client counts its callers
// actually pass, asserting the exact mount-option string rather than just
// its size, so a change to the option string's own shape (not only its
// number) fails here too.
func TestMountOptions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		clients int
		want    string
	}{
		{
			name:    "zero sizes for one client, the fallback's own zero value",
			clients: 0,
			want:    "rw,size=512m",
		},
		{
			name:    "a negative count sizes for one client too",
			clients: -1,
			want:    "rw,size=512m",
		},
		{
			name:    "one client reproduces the historical single-client mount byte-for-byte",
			clients: 1,
			want:    "rw,size=512m",
		},
		{
			name:    "two clients double the mount",
			clients: 2,
			want:    "rw,size=1024m",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := MountOptions(tc.clients); got != tc.want {
				t.Errorf("MountOptions(%d) = %q, want %q", tc.clients, got, tc.want)
			}
		})
	}
}

// TestMostClientsIn drives the client-count parse over scratch inputs rather
// than the real build file, so both directions are exercised directly: the
// literal shapes it must find, and the shapes that must NOT be read as a
// count — a variable reference, and a file with no flag at all, where a zero
// would silently make every mount budget fit.
func TestMostClientsIn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		want    int
		wantErr bool
	}{
		{
			name:    "single literal count",
			content: "test-contention:\n\ttmp/testpg --clients 2 --parallel 16 -- bash -c 'x'\n",
			want:    2,
		},
		{
			name:    "largest of several literal counts wins",
			content: "a:\n\ttestpg --clients 2 -- x\nb:\n\ttestpg --clients 7 -- y\nc:\n\ttestpg --clients 1 -- z\n",
			want:    7,
		},
		{
			name:    "a variable reference is not a literal count",
			content: "test-db-up:\n\ttmp/testpg --up --clients $(CLIENTS)\n",
			wantErr: true,
		},
		{
			name:    "a literal count beside a variable one is still found",
			content: "a:\n\ttestpg --clients $(CLIENTS)\nb:\n\ttestpg --clients 3 -- y\n",
			want:    3,
		},
		{
			name:    "no flag at all is an error, never zero",
			content: "test:\n\tgo test ./...\n",
			wantErr: true,
		},
		{
			name:    "a tab between flag and count is matched",
			content: "a:\n\ttestpg --clients\t4 -- y\n",
			want:    4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := mostClientsIn(tc.content)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("mostClientsIn(%q) = %d, want an error", tc.content, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("mostClientsIn(%q): %v", tc.content, err)
			}
			if got != tc.want {
				t.Errorf("mostClientsIn(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// TestStartServer_PGDATAMountHoldsEveryClientCountProvisioned asserts a
// server this package provisions cannot be filled by the clients it is
// provisioned for, nor by its own write-ahead log beside them. PGDATA lives
// on a fixed-size tmpfs, and max_wal_size is how much WAL the server lets
// accumulate before a checkpoint recycles it — so a budget the mount cannot
// hold means sustained write load exhausts the mount whatever the load is
// doing. The server then PANICs, goes into recovery and exits, and every
// connection open at that instant dies mid-statement.
//
// The client count is part of the budget, not a detail outside it: one
// server admits every client at once, each client populates its own schemas
// on the one mount, and a count that scales the connection ceiling while
// leaving the mount alone is how that mount gets overrun. The counts checked
// are the ones this repository's own targets provision for, read from the
// build file, so raising a count there without sizing the mount fails here.
//
// Two separate assertions cover two separate axes, and neither substitutes
// for the other. The measured-basis check fails by name when a provisioned
// count exceeds clusterPeakMeasuredAtClients, because that is the one axis
// the budget inequality below is structurally unable to fail on: the
// per-client mount grant scales with the client count exactly as the peak
// term does, so the count cancels out of the inequality and no count, however
// high, makes it fail. The budget inequality still catches every other axis
// — a smaller per-client grant, a larger measured peak, or a larger WAL
// allowance eating into the same headroom.
func TestStartServer_PGDATAMountHoldsEveryClientCountProvisioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server, err := StartServer(ctx, ServerOptions{})
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Errorf("stopping the server: %v", err)
		}
	})

	pool, err := pgxpool.New(ctx, server.DSN())
	if err != nil {
		t.Fatalf("connect to the provisioned server: %v", err)
	}
	defer pool.Close()

	var (
		walSize int
		unit    string
	)
	if err := pool.QueryRow(ctx,
		"SELECT setting::int, unit FROM pg_settings WHERE name = 'max_wal_size'",
	).Scan(&walSize, &unit); err != nil {
		t.Fatalf("read max_wal_size: %v", err)
	}
	if unit != "MB" {
		t.Fatalf("max_wal_size is reported in %q, this assertion reads megabytes", unit)
	}

	// One client is the default every ordinary gate provisions; the largest
	// literal count in the build file is what the contention target
	// provisions. Both share this package's mount sizing, so both are budgets
	// this assertion has to hold.
	for _, clients := range []int{1, targetClients(t)} {
		if clients > clusterPeakMeasuredAtClients {
			t.Errorf("this repository provisions a server for %d client(s), but clusterPeakMB was only measured at %d — re-measure at the higher count (recipe recorded beside clusterPeakMB) before trusting it for %d",
				clients, clusterPeakMeasuredAtClients, clients)
		}

		mountMB := mountCapMB(t, MountOptions(clients))
		if peak := walCheckpointFactor*walSize + clients*clusterPeakMB; peak > mountMB {
			t.Errorf("a server provisioned for %d client(s) needs %d MB — %d× max_wal_size %d MB between checkpoints, beside %d × %d MB of peak cluster — of a %d MB PGDATA mount",
				clients, peak, walCheckpointFactor, walSize, clients, clusterPeakMB, mountMB)
		}
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
