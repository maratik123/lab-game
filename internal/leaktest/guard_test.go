package leaktest_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

// leaktestImportPath is this package's own import path, as another
// package's test file would spell it in its own import block.
const leaktestImportPath = "github.com/maratik123/lab-game/internal/leaktest"

// buildConfig is one of the two build configurations the gates in this
// module actually compile: the default build, and the default build with
// the race build tag added — the tag -race satisfies and the default
// build does not know about.
type buildConfig struct {
	name string
	ctx  build.Context
}

func buildConfigs() []buildConfig {
	withRace := build.Default
	withRace.BuildTags = append(append([]string{}, build.Default.BuildTags...), "race")
	return []buildConfig{
		{name: "default build", ctx: build.Default},
		{name: "race build", ctx: withRace},
	}
}

// excludedByDirName reports whether relPath — a file path relative to the
// tree's root — lies under a directory the go tool itself never
// descends into: testdata, or a directory whose name begins with "."
// or "_".
func excludedByDirName(relPath string) bool {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return false
	}
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "testdata" || strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_") {
			return true
		}
	}
	return false
}

// testDirs returns the sorted, root-relative set of directories under
// root holding at least one Go test source file that the go tool would
// ever look at — excludedByDirName's directories left out.
func testDirs(t *testing.T, root string) []string {
	t.Helper()
	set := map[string]bool{}
	srcguard.WalkSubtree(t, root, func(path string) {
		if !strings.HasSuffix(path, "_test.go") {
			return
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel(%s): %v", path, err)
		}
		if excludedByDirName(rel) {
			return
		}
		set[filepath.Dir(rel)] = true
	})
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// importLocalName reports the identifier f's own import block binds
// importPath to — the import's alias if it declares one, importPath's
// last segment otherwise — and whether f imports importPath at all.
func importLocalName(f *ast.File, importPath string) (string, bool) {
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		parts := strings.Split(path, "/")
		return parts[len(parts)-1], true
	}
	return "", false
}

// isSelectorOnImport reports whether expr is a selector expression
// selecting selName on the local identifier f's own imports bind to
// importPath.
func isSelectorOnImport(expr ast.Expr, f *ast.File, importPath, selName string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != selName {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	local, ok := importLocalName(f, importPath)
	return ok && ident.Name == local
}

// firstParamName returns the name of fn's first parameter and whether it
// has one at all.
func firstParamName(fn *ast.FuncDecl) (string, bool) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return "", false
	}
	names := fn.Type.Params.List[0].Names
	if len(names) == 0 {
		return "", false
	}
	return names[0].Name, true
}

// validateTestMainShape reports why fn, declared in f, does not have the
// shape this package's Main requires — a single statement calling
// os.Exit on the result of this package's own Main, passed fn's own
// first parameter — or "" when it does.
func validateTestMainShape(fn *ast.FuncDecl, f *ast.File) string {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return "TestMain's body is not exactly one statement"
	}
	exprStmt, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return "TestMain's one statement is not a call expression"
	}
	exitCall, ok := exprStmt.X.(*ast.CallExpr)
	if !ok || len(exitCall.Args) != 1 {
		return "TestMain's statement is not a single-argument os.Exit(...) call"
	}
	if !isSelectorOnImport(exitCall.Fun, f, "os", "Exit") {
		return "TestMain's statement does not call os.Exit"
	}
	mainCall, ok := exitCall.Args[0].(*ast.CallExpr)
	if !ok || len(mainCall.Args) < 2 {
		return "os.Exit's argument is not a call to this package's Main with a runner"
	}
	if !isSelectorOnImport(mainCall.Fun, f, leaktestImportPath, "Main") {
		return "os.Exit's argument does not call this package's Main"
	}
	paramName, ok := firstParamName(fn)
	if !ok {
		return "TestMain declares no parameter to pass through"
	}
	arg, ok := mainCall.Args[0].(*ast.Ident)
	if !ok || arg.Name != paramName {
		return "Main's first argument is not TestMain's own parameter"
	}
	return ""
}

// checkTestMain looks for TestMain across the compiled test files named
// by testFiles, in dir, and reports why the directory fails the guard
// under this configuration — no declaration, more than one, or a
// malformed one — or "" when exactly one is present and well formed.
func checkTestMain(t *testing.T, dir string, testFiles []string) string {
	t.Helper()
	sorted := append([]string{}, testFiles...)
	sort.Strings(sorted)

	var decls []*ast.FuncDecl
	var files []*ast.File
	for _, name := range sorted {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", filepath.Join(dir, name), err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" {
				continue
			}
			decls = append(decls, fn)
			files = append(files, f)
		}
	}

	switch len(decls) {
	case 0:
		return "no TestMain among this configuration's compiled test files"
	case 1:
		return validateTestMainShape(decls[0], files[0])
	default:
		return fmt.Sprintf("%d TestMain declarations among this configuration's compiled test files", len(decls))
	}
}

// checkTree walks root for every directory the go tool would compile
// test files in, and checks each one, under both build configurations,
// for exactly one well-formed TestMain among the test files that
// configuration actually compiles. It returns the sorted, root-relative
// directories it examined — a directory with no test file the go tool
// would ever compile is never examined — and a map from a failing
// directory to its failure reasons, one per configuration it failed
// under.
func checkTree(t *testing.T, root string) (examined []string, failures map[string][]string) {
	t.Helper()
	failures = map[string][]string{}

	for _, rel := range testDirs(t, root) {
		examined = append(examined, rel)
		abs := filepath.Join(root, rel)
		for _, cfg := range buildConfigs() {
			pkg, err := cfg.ctx.ImportDir(abs, 0)
			if err != nil {
				var noGo *build.NoGoError
				if errors.As(err, &noGo) {
					continue // no test file this configuration compiles here.
				}
				t.Fatalf("ImportDir(%s) under %s: %v", abs, cfg.name, err)
			}
			var testFiles []string
			testFiles = append(testFiles, pkg.TestGoFiles...)
			testFiles = append(testFiles, pkg.XTestGoFiles...)
			if len(testFiles) == 0 {
				continue
			}
			if reason := checkTestMain(t, abs, testFiles); reason != "" {
				failures[rel] = append(failures[rel], cfg.name+": "+reason)
			}
		}
	}
	return examined, failures
}

// TestGuard_EveryPackageWithTestsRunsTheDetection is the real-tree case:
// every directory under the repository root that the go tool would ever
// compile a test file in must declare exactly one well-formed TestMain,
// under both build configurations. A positive control — the walk must
// have examined this package's own directory — guards against an empty
// or truncated walk reading as a clean tree.
func TestGuard_EveryPackageWithTestsRunsTheDetection(t *testing.T) {
	t.Parallel()

	root := repotest.Root(t)
	examined, failures := checkTree(t, root)

	self := filepath.Join("internal", "leaktest")
	found := false
	for _, dir := range examined {
		if dir == self {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("the walk never examined %s itself — an empty or truncated walk, not a clean tree; examined: %v", self, examined)
	}

	if len(failures) == 0 {
		return
	}
	var lines []string
	for dir, reasons := range failures {
		for _, reason := range reasons {
			lines = append(lines, dir+": "+reason)
		}
	}
	sort.Strings(lines)
	t.Errorf("packages failing the TestMain guard:\n%s", strings.Join(lines, "\n"))
}

// writeScratchTree writes every file in files — a map from a "/"-separated
// relative path to its content — under one fresh scratch tree and returns
// the tree's root.
func writeScratchTree(t *testing.T, files map[string]string) string {
	t.Helper()
	var rels []string
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	root, _ := srcguard.WriteScratchFile(t, rels[0], files[rels[0]])
	for _, rel := range rels[1:] {
		srcguard.WriteScratchFileIn(t, root, rel, files[rel])
	}
	return root
}

const wantTestMainBody = `func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, (*testing.M).Run))
}
`

// TestGuard_scratch drives checkTree over scratch trees, one per case,
// each built to discriminate one way a directory can fail or pass the
// guard.
func TestGuard_scratch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		files     map[string]string
		reported  []string // directories that must appear in failures
		clean     []string // directories that must be examined, but not in failures
		unvisited []string // directories that must never be examined at all
	}{
		{
			name: "no TestMain declared",
			files: map[string]string{
				"nomain/doc.go":    "package nomain\n",
				"nomain/x_test.go": "package nomain\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
			},
			reported: []string{"nomain"},
		},
		{
			name: "today's form: TestMain calling only the provisioner's Main",
			files: map[string]string{
				"todaysform/main_test.go": `package todaysform

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
}
`,
			},
			reported: []string{"todaysform"},
		},
		{
			name: "calls this package's Main and then os.Exit(0)",
			files: map[string]string{
				"discardresult/main_test.go": `package discardresult

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	leaktest.Main(m, (*testing.M).Run)
	os.Exit(0)
}
`,
			},
			reported: []string{"discardresult"},
		},
		{
			name: "passes something other than its own parameter",
			files: map[string]string{
				"wrongparam/main_test.go": `package wrongparam

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(nil, (*testing.M).Run))
}
`,
			},
			reported: []string{"wrongparam"},
		},
		{
			name: "a second statement",
			files: map[string]string{
				"secondstmt/main_test.go": `package secondstmt

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, (*testing.M).Run))
	panic("unreachable")
}
`,
			},
			reported: []string{"secondstmt"},
		},
		{
			name: "two TestMain declarations across internal and external test files",
			files: map[string]string{
				"twomains/a_test.go": "package twomains\n\n" + wantTestMainBody,
				"twomains/b_test.go": "package twomains_test\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"twomains"},
		},
		{
			name: "a //go:build line the default build does not satisfy",
			files: map[string]string{
				"buildtagpkg/doc.go": "package buildtagpkg\n",
				"buildtagpkg/sibling_test.go": "package buildtagpkg\n\nimport \"testing\"\n\n" +
					"func TestSibling(t *testing.T) {}\n",
				"buildtagpkg/main_test.go": "//go:build never_a_real_build_tag\n\n" +
					"package buildtagpkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"buildtagpkg"},
		},
		{
			name: "a legacy // +build line the default build does not satisfy",
			files: map[string]string{
				"legacybuildpkg/doc.go": "package legacybuildpkg\n",
				"legacybuildpkg/sibling_test.go": "package legacybuildpkg\n\nimport \"testing\"\n\n" +
					"func TestSibling(t *testing.T) {}\n",
				"legacybuildpkg/main_test.go": "// +build never_a_real_build_tag\n\n" +
					"package legacybuildpkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"legacybuildpkg"},
		},
		{
			name: "a GOOS filename suffix other than the host's",
			files: map[string]string{
				"goossuffixpkg/doc.go": "package goossuffixpkg\n",
				"goossuffixpkg/sibling_test.go": "package goossuffixpkg\n\nimport \"testing\"\n\n" +
					"func TestSibling(t *testing.T) {}\n",
				"goossuffixpkg/main_windows_test.go": "package goossuffixpkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"goossuffixpkg"},
		},
		{
			name: "a name beginning with an underscore",
			files: map[string]string{
				"underscorefilepkg/doc.go": "package underscorefilepkg\n",
				"underscorefilepkg/sibling_test.go": "package underscorefilepkg\n\nimport \"testing\"\n\n" +
					"func TestSibling(t *testing.T) {}\n",
				"underscorefilepkg/_main_test.go": "package underscorefilepkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"underscorefilepkg"},
		},
		{
			name: "a //go:build !race file beside an unconstrained test",
			files: map[string]string{
				"racepkg/unconstrained_test.go": "package racepkg\n\nimport \"testing\"\n\n" +
					"func TestSibling(t *testing.T) {}\n",
				"racepkg/main_test.go": "//go:build !race\n\npackage racepkg\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			reported: []string{"racepkg"},
		},
		{
			name: "a compliant package with entries and import aliases",
			files: map[string]string{
				"aliasedok/main_test.go": `package aliasedok

import (
	os2 "os"
	"testing"

	lt "github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	os2.Exit(lt.Main(m, (*testing.M).Run, lt.Ignore{Function: "no.such/pkg.Function", Reason: "scratch fixture"}))
}
`,
			},
			clean: []string{"aliasedok"},
		},
		{
			name: "a compliant TestMain in the external test package of a mixed directory",
			files: map[string]string{
				"mixedexternal/doc.go": "package mixedexternal\n",
				"mixedexternal/internal_test.go": "package mixedexternal\n\nimport \"testing\"\n\n" +
					"func TestSomething(t *testing.T) {}\n",
				"mixedexternal/main_test.go": "package mixedexternal_test\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t" +
					"\"github.com/maratik123/lab-game/internal/leaktest\"\n)\n\n" + wantTestMainBody,
			},
			clean: []string{"mixedexternal"},
		},
		{
			name: "_test.go files under testdata, an underscore directory and a dot directory",
			files: map[string]string{
				"testdata/x_test.go": "package testdata\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
				"_x/y_test.go":       "package underscoredir\n\nimport \"testing\"\n\nfunc TestY(t *testing.T) {}\n",
				".x/z_test.go":       "package dotdir\n\nimport \"testing\"\n\nfunc TestZ(t *testing.T) {}\n",
			},
			unvisited: []string{"testdata", "_x", ".x"},
		},
		{
			name: "a directory whose only test file is named with a leading underscore",
			files: map[string]string{
				"onlyunderscorefile/_x_test.go": "package onlyunderscorefile\n\nimport \"testing\"\n\n" +
					"func TestX(t *testing.T) {}\n",
			},
			clean: []string{"onlyunderscorefile"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeScratchTree(t, tc.files)
			examined, failures := checkTree(t, root)

			examinedSet := map[string]bool{}
			for _, d := range examined {
				examinedSet[d] = true
			}

			for _, dir := range tc.reported {
				if len(failures[dir]) == 0 {
					t.Errorf("directory %q: want it reported, got no failures (examined: %v, failures: %v)", dir, examined, failures)
				}
			}
			for _, dir := range tc.clean {
				if !examinedSet[dir] {
					t.Errorf("directory %q: want it examined, it was not (examined: %v)", dir, examined)
				}
				if len(failures[dir]) != 0 {
					t.Errorf("directory %q: want it clean, got failures: %v", dir, failures[dir])
				}
			}
			for _, dir := range tc.unvisited {
				if examinedSet[dir] {
					t.Errorf("directory %q: want it never examined, it was", dir)
				}
			}
		})
	}
}
