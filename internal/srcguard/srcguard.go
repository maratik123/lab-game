// Package srcguard holds the mechanical half of every non-test-source
// structural guard in this module: enumerating a package directory's or
// a subtree's Go source files, parsing them, and writing a scratch
// package into a fresh t.TempDir() so a discriminating guard has
// something of its own to fail on. Every predicate — what a guard
// actually forbids — stays in the package that owns the proposition;
// this package only walks and parses. The repository root arrives from
// the caller as an argument: this package performs no ascent of its
// own.
package srcguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// PackageFiles returns the sorted, absolute paths of every non-test Go
// source file directly inside dir — no recursion into subdirectories.
// It is the enumerator a guard scoped to one package directory uses.
func PackageFiles(tb testing.TB, dir string) []string {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	return paths
}

// WalkSubtree calls fn, in lexical order, with the path of every Go
// source file found under root — test files included, since a caller
// that wants only non-test source filters fn itself (as
// TestFilesOnly does for the common case). It skips version-control
// directories a real repository root never needs walked, and the
// repository's own designated scratch directory ("tmp" directly under
// root), which a live session is expected to write throwaway probes
// into and which is never part of the tree a guard polices.
func WalkSubtree(tb testing.TB, root string, fn func(path string)) {
	tb.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if path == filepath.Join(root, "tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			fn(path)
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("WalkDir(%s): %v", root, err)
	}
}

// NonTestFile reports whether path is a non-test Go source file — the
// filter a WalkSubtree caller applies when it wants the same
// test-excluded scope PackageFiles already applies for a single
// directory.
func NonTestFile(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

// ParseFile parses path with its comments retained, failing the test
// fatally — naming the path — on a parse error rather than returning a
// nil file a caller would have to nil-check.
func ParseFile(tb testing.TB, path string) *ast.File {
	tb.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		tb.Fatalf("ParseFile(%s): %v", path, err)
	}
	return f
}

// ParseFiles parses every path in paths, returning a map keyed by each
// file's own path.
func ParseFiles(tb testing.TB, paths []string) map[string]*ast.File {
	tb.Helper()
	out := make(map[string]*ast.File, len(paths))
	for _, path := range paths {
		out[path] = ParseFile(tb, path)
	}
	return out
}

// ImportPaths returns f's own imports, unquoted.
func ImportPaths(tb testing.TB, f *ast.File) []string {
	tb.Helper()
	paths := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		v, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			tb.Fatalf("Unquote(%s): %v", imp.Path.Value, err)
		}
		paths = append(paths, v)
	}
	return paths
}

// WriteScratchFile creates rel — a "/"-separated relative path naming a
// scratch source file — under a fresh t.TempDir() and writes content to
// it, creating parent directories as needed. It returns the
// temporary directory's root, suitable as a root argument to
// WalkSubtree or (joined with rel's own directory) to PackageFiles, and
// the written file's absolute path — so a discriminating guard has a
// scratch package of its own to fail on, never the working tree.
func WriteScratchFile(tb testing.TB, rel, content string) (root, path string) {
	tb.Helper()
	root = tb.TempDir()
	return root, WriteScratchFileIn(tb, root, rel, content)
}

// WriteScratchFileIn writes a second (or further) file into a root a
// prior WriteScratchFile call already returned, so a scratch fixture
// that needs more than one file — a package plus a nested subpackage,
// say — stays under one t.TempDir() rather than one per file.
func WriteScratchFileIn(tb testing.TB, root, rel, content string) (path string) {
	tb.Helper()
	path = filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		tb.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		tb.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}
