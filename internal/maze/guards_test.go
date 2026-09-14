package maze

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/detguard"
	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

func TestGuard_DeterminismPredicates(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/maze")
	if problems := detguard.Check(t, dir); len(problems) != 0 {
		t.Errorf("detguard.Check(maze) problems:\n%v", problems)
	}
}

// identifierCallSites returns the basename of every non-test source
// file directly inside dir that contains a call expression whose
// function is exactly the bare identifier name.
func identifierCallSites(t *testing.T, dir, name string) []string {
	t.Helper()
	var files []string
	for _, path := range srcguard.PackageFiles(t, dir) {
		f := srcguard.ParseFile(t, path)
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if ok && ident.Name == name {
				found = true
			}
			return true
		})
		if found {
			files = append(files, filepath.Base(path))
		}
	}
	sort.Strings(files)
	return files
}

func TestGuard_ConnectivityHelperCalledOnlyFromIslandSelection(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/maze")
	got := identifierCallSites(t, dir, "connectedOverInduced")
	want := []string{"island.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("connectedOverInduced is called from %v, want exactly %v", got, want)
	}
}

func TestGuard_ChunkAndBorderKeyDerivationsCalledOnlyFromFabricAndPortalBuilders(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/maze")
	for _, name := range []string{"chunkKey", "borderKey"} {
		got := identifierCallSites(t, dir, name)
		want := []string{"generate.go"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s is called from %v, want exactly %v", name, got, want)
		}
	}
}

// noPlugInPointProblems reports every declaration in dir's non-test
// source that would reintroduce a plug-in point: an exported interface
// type, or an exported function or method taking a function- or
// interface-typed parameter.
func noPlugInPointProblems(t *testing.T, dir string) []string {
	t.Helper()
	var problems []string
	for _, path := range srcguard.PackageFiles(t, dir) {
		f := srcguard.ParseFile(t, path)
		base := filepath.Base(path)
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					if _, isInterface := ts.Type.(*ast.InterfaceType); isInterface {
						problems = append(problems, fmt.Sprintf("%s: exported interface type %s", base, ts.Name.Name))
					}
				}
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Type.Params == nil {
					continue
				}
				for _, param := range d.Type.Params.List {
					switch param.Type.(type) {
					case *ast.FuncType:
						problems = append(problems, fmt.Sprintf("%s: exported func %s takes a function-typed parameter", base, d.Name.Name))
					case *ast.InterfaceType:
						problems = append(problems, fmt.Sprintf("%s: exported func %s takes an interface-typed parameter", base, d.Name.Name))
					}
				}
			}
		}
	}
	return problems
}

// TestGuard_NoPlugInPoint asserts the package's non-test source declares
// no exported interface type and no exported function or method taking
// a function- or interface-typed parameter — the prefab hook's shape is
// a failing test now, not a review catch.
func TestGuard_NoPlugInPoint(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/maze")
	if problems := noPlugInPointProblems(t, dir); len(problems) != 0 {
		t.Errorf("no-plug-in-point guard found:\n%v", problems)
	}
}

// TestGuard_NoPlugInPoint_CatchesAReintroducedInterface is the guard's
// own control: a scratch fixture declaring an exported interface must be
// seen RED, or the guard above is not discriminating.
func TestGuard_NoPlugInPoint_CatchesAReintroducedInterface(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/hook.go", "package pkg\n\n// Hook is a fixture plug-in point.\ntype Hook interface {\n\tClaims() bool\n}\n")
	if problems := noPlugInPointProblems(t, filepath.Join(root, "pkg")); len(problems) == 0 {
		t.Fatal("no-plug-in-point guard found no problems in a fixture declaring an exported interface")
	}
}

// allowedMazeImports is every import path this package's non-test
// source may use: the standard library it needs, the one third-party
// decimal package, and this module's own topology vocabulary. Anything
// else — a database driver or a store package included — is refused,
// since the import graph is where a database reach would have to enter.
var allowedMazeImports = map[string]bool{
	"crypto/sha256":   true,
	"encoding/binary": true,
	"errors":          true,
	"fmt":             true,
	"math/rand/v2":    true,
	"sort":            true,

	"github.com/shopspring/decimal":                   true,
	"github.com/maratik123/lab-game/internal/hexgrid": true,
}

// importAllowlistProblems reports every non-test import in dir that
// allowed does not list, one entry per (file, import) pair.
func importAllowlistProblems(t *testing.T, dir string, allowed map[string]bool) []string {
	t.Helper()
	var problems []string
	for _, path := range srcguard.PackageFiles(t, dir) {
		f := srcguard.ParseFile(t, path)
		for _, imp := range srcguard.ImportPaths(t, f) {
			if !allowed[imp] {
				problems = append(problems, fmt.Sprintf("%s: disallowed import %q", filepath.Base(path), imp))
			}
		}
	}
	return problems
}

// TestGuard_ImportsAllowlist is the structural half of the "reads no
// database" clause: maze's non-test imports are checked against an
// allowlist, so a database driver or a store package reaching this
// package is a failing test.
func TestGuard_ImportsAllowlist(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/maze")
	if problems := importAllowlistProblems(t, dir, allowedMazeImports); len(problems) != 0 {
		t.Errorf("import allowlist guard found:\n%v", problems)
	}
}

// TestGuard_ImportsAllowlist_CatchesADisallowedImport is the guard's own
// control.
func TestGuard_ImportsAllowlist_CatchesADisallowedImport(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/db.go", "package pkg\n\nimport \"database/sql\"\n\nvar _ = sql.ErrNoRows\n")
	if problems := importAllowlistProblems(t, filepath.Join(root, "pkg"), allowedMazeImports); len(problems) == 0 {
		t.Fatal("import allowlist guard found no problems in a fixture importing database/sql")
	}
}
