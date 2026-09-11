package main

import (
	"bytes"
	"context"
	"go/ast"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// guardWalkRoots is the scope every guard in this file walks: every
// non-test Go source file under this module's two source trees, from
// the repository root this module's file-location-ascent resolver
// resolves.
func guardWalkRoots(tb testing.TB) []string {
	tb.Helper()
	root := repotest.Root(tb)
	var paths []string
	for _, sub := range []string{"cmd", "internal"} {
		srcguard.WalkSubtree(tb, root+"/"+sub, func(path string) {
			if srcguard.NonTestFile(path) {
				paths = append(paths, path)
			}
		})
	}
	return paths
}

// initFuncOffenses parses path and returns one string per package-scope
// func init() declaration found in it.
func initFuncOffenses(tb testing.TB, path string) []string {
	tb.Helper()
	f := srcguard.ParseFile(tb, path)
	var offenses []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		if fn.Name.Name == "init" {
			offenses = append(offenses, path)
		}
	}
	return offenses
}

// TestGuard_NoInitFuncAnywhere is the negative half of this design's
// no-init-func rule: no non-test Go source file in this module's two
// source trees declares a package-scope func init().
func TestGuard_NoInitFuncAnywhere(t *testing.T) {
	t.Parallel()
	for _, path := range guardWalkRoots(t) {
		if offenses := initFuncOffenses(t, path); len(offenses) != 0 {
			t.Errorf("%s declares a func init()", path)
		}
	}
}

// TestGuard_NoInitFuncAnywhere_ProvenDiscriminating proves the walk
// above is an instrument, not a tautology: the same predicate run over
// a scratch package that does declare a func init() must report it.
func TestGuard_NoInitFuncAnywhere_ProvenDiscriminating(t *testing.T) {
	t.Parallel()
	_, path := srcguard.WriteScratchFile(t, "scratch/scratch.go",
		"package scratch\n\nfunc init() {}\n")
	if offenses := initFuncOffenses(t, path); len(offenses) == 0 {
		t.Fatal("expected the scratch func init() to be flagged")
	}
}

// packageVarSubsystemOffenses parses path and flags a package-level var
// whose initialiser is a call to a New… identifier — unqualified, or
// qualified by a package name this file's own import block resolves to
// a path carrying this module's prefix. That is the syntactic proxy
// this design uses for "holds a package-level mutable subsystem
// instance": every subsystem in this module is reached through its
// package's New… constructor, so a package-level var initialised from
// one is the shape this rule forbids; a package-level var initialised
// from anything else — an errors.New sentinel, a compile-time
// interface assertion — is not.
func packageVarSubsystemOffenses(tb testing.TB, path string) []string {
	tb.Helper()
	f := srcguard.ParseFile(tb, path)

	localModuleImports := map[string]bool{}
	for _, imp := range f.Imports {
		v := strings.Trim(imp.Path.Value, `"`)
		if strings.HasPrefix(v, "github.com/maratik123/lab-game/") {
			name := v[strings.LastIndex(v, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			localModuleImports[name] = true
		}
	}

	var offenses []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, val := range vs.Values {
				call, ok := val.(*ast.CallExpr)
				if !ok {
					continue
				}
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					if strings.HasPrefix(fn.Name, "New") {
						offenses = append(offenses, path)
					}
				case *ast.SelectorExpr:
					pkgIdent, ok := fn.X.(*ast.Ident)
					if !ok {
						continue
					}
					if strings.HasPrefix(fn.Sel.Name, "New") && localModuleImports[pkgIdent.Name] {
						offenses = append(offenses, path)
					}
				}
			}
		}
	}
	return offenses
}

// TestGuard_NoPackageLevelSubsystemVarAnywhere is the package-level-var
// clause of the same rule: no non-test Go source file in this module's
// two source trees declares a package-level var initialised from a
// New… constructor reached from this module.
func TestGuard_NoPackageLevelSubsystemVarAnywhere(t *testing.T) {
	t.Parallel()
	for _, path := range guardWalkRoots(t) {
		if offenses := packageVarSubsystemOffenses(t, path); len(offenses) != 0 {
			t.Errorf("%s declares a package-level var initialised from a New… constructor", path)
		}
	}
}

// TestGuard_NoPackageLevelSubsystemVarAnywhere_ProvenDiscriminating
// proves the walk above is an instrument: a scratch package declaring
// var x = New(...) must be flagged.
func TestGuard_NoPackageLevelSubsystemVarAnywhere_ProvenDiscriminating(t *testing.T) {
	t.Parallel()
	_, path := srcguard.WriteScratchFile(t, "scratch/scratch.go",
		"package scratch\n\nfunc New() int { return 0 }\n\nvar x = New()\n")
	if offenses := packageVarSubsystemOffenses(t, path); len(offenses) == 0 {
		t.Fatal("expected the scratch package-level var to be flagged")
	}
}

// TestGuard_CmdBotOwnPackageDeclaresNoVarBeyondVersion is this
// package's own tighter rule: after this task it declares no
// package-level var at all beyond the link-time version string.
func TestGuard_CmdBotOwnPackageDeclaresNoVarBeyondVersion(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	paths := srcguard.PackageFiles(t, root+"/cmd/bot")

	var names []string
	for _, path := range paths {
		f := srcguard.ParseFile(t, path)
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range vs.Names {
					if name.Name != "_" {
						names = append(names, name.Name)
					}
				}
			}
		}
	}
	if len(names) != 1 || names[0] != "version" {
		t.Errorf("cmd/bot package-level vars = %v, want exactly [version]", names)
	}
}

// TestGuard_ScrapeAndLogCarryNoSentinelSecret assembles with a sentinel
// bot token and a sentinel DSN password, drives one /metrics scrape,
// and asserts neither sentinel appears in the scrape body or in the
// captured process log — the bot token and the DSN stay inside the
// redacting secret type everywhere they are carried.
func TestGuard_ScrapeAndLogCarryNoSentinelSecret(t *testing.T) {
	t.Parallel()

	const (
		sentinelBotToken    = "1:SENTINEL-BOT-TOKEN-VALUE-----------"
		sentinelDSNPassword = "SENTINEL-DSN-PASSWORD"
	)

	srv := tgtest.New(t, tgtest.Success(nil))
	env := assembleTestEnv(t)
	// The fake server never validates the token it receives (it answers
	// every request with the installed handler regardless of path), so
	// the sentinel itself — not this package's own ordinary fake token —
	// can be the token the process actually carries: only then does its
	// absence from the scrape and the log test the bot-token half for
	// real, rather than vacuously.
	env["LAB_GAME_BOT_TOKEN"] = sentinelBotToken
	// Splice the sentinel into an application_name query parameter on
	// the already-provisioned schema DSN — Postgres accepts (and
	// ignores, for auth purposes) an arbitrary application_name, so the
	// connection stays valid while cfg.DSN's full string genuinely
	// carries the sentinel, making its absence from every scrape and
	// log line a real assertion rather than a vacuous one.
	env["LAB_GAME_DSN"] = withSentinelApplicationName(t, env["LAB_GAME_DSN"], sentinelDSNPassword)

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(env),
		Stderr:     &stderr,
		Version:    "test",
		StartedAt:  time.Now(),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	t.Cleanup(func() { teardown(t, a) })

	scrape := scrapeMetrics(t, a.healthSrv.Addr())

	if strings.Contains(scrape, sentinelBotToken) {
		t.Error("scrape carries the sentinel bot token")
	}
	if strings.Contains(scrape, sentinelDSNPassword) {
		t.Error("scrape carries the sentinel DSN password")
	}
	if strings.Contains(stderr.String(), sentinelBotToken) {
		t.Error("process log carries the sentinel bot token")
	}
	if strings.Contains(stderr.String(), sentinelDSNPassword) {
		t.Error("process log carries the sentinel DSN password")
	}
}

// withSentinelApplicationName splices sentinel into dsn's
// application_name query parameter, re-encoding a valid DSN Postgres
// accepts without changing which credentials it authenticates with.
func withSentinelApplicationName(tb testing.TB, dsn, sentinel string) string {
	tb.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		tb.Fatalf("parse DSN: %v", err)
	}
	q := u.Query()
	q.Set("application_name", sentinel)
	u.RawQuery = q.Encode()
	return u.String()
}

// scrapeMetrics GETs the /metrics path off addr and returns the body.
func scrapeMetrics(tb testing.TB, addr string) string {
	tb.Helper()
	//nolint:noctx // test-only fixed loopback address this test itself just bound; no user input.
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		tb.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		tb.Fatalf("read /metrics body: %v", err)
	}
	return string(body)
}

// TestGuard_ImportGraphDiscrimination proves this command's non-test
// dependency list carries no container-runtime path, while its test
// dependency list does — the second half is what makes the first half
// evidence rather than a tautology (the same reachable-with-tests
// shape the module's own import gate distinguishes).
func TestGuard_ImportGraphDiscrimination(t *testing.T) {
	t.Parallel()

	nonTest := goListDeps(t, false)
	if strings.Contains(nonTest, "testcontainers") {
		t.Error("non-test dependency graph of cmd/bot carries a testcontainers import")
	}

	withTests := goListDeps(t, true)
	if !strings.Contains(withTests, "testcontainers") {
		t.Fatal("test dependency graph of cmd/bot does not carry a testcontainers import — the guard above would pass vacuously")
	}
}

// goListDeps runs `go list [-test] -deps ./cmd/bot` from the repository
// root and returns its combined output.
func goListDeps(tb testing.TB, withTests bool) string {
	tb.Helper()
	args := []string{"list"}
	if withTests {
		args = append(args, "-test")
	}
	args = append(args, "-deps", "./cmd/bot")
	cmd := exec.CommandContext(context.Background(), "go", args...)
	cmd.Dir = repotest.Root(tb)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("go %v: %v\n%s", args, err, out)
	}
	return string(out)
}
