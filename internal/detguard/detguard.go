package detguard

import (
	"go/ast"
	"sort"
	"strconv"
	"testing"

	"github.com/maratik123/lab-game/internal/srcguard"
)

// bannedImports names every import path a determinism-path package's
// non-test source may never carry: the wall clock, and every
// non-deterministic or non-pinned random source.
var bannedImports = []string{
	"time",
	"math/rand",
	"hash/maphash",
	"crypto/rand",
	"math",
}

// bannedDecimalSelectors names shopspring/decimal's float-valued
// constructors and accessors. A float value reaching a determinism-path
// package by this route defeats the pinned decimal rounding just as
// surely as a bare float32/float64 declaration would, and neither a
// type-declaration guard nor a bare import guard sees it: the float
// arrives as a method result, not as a named type.
var bannedDecimalSelectors = map[string]bool{
	"NewFromFloat":             true,
	"NewFromFloat32":           true,
	"NewFromFloatWithExponent": true,
	"BigFloat":                 true,
	"Float64":                  true,
	"InexactFloat64":           true,
}

// Check parses every non-test Go source file directly inside dir and
// returns one problem description per determinism-predicate violation
// found — nil when dir's non-test source honours every predicate this
// package holds.
func Check(tb testing.TB, dir string) []string {
	tb.Helper()
	var problems []string
	for _, path := range srcguard.PackageFiles(tb, dir) {
		problems = append(problems, checkFile(tb, path, srcguard.ParseFile(tb, path))...)
	}
	sort.Strings(problems)
	return problems
}

// checkFile applies every predicate to one already-parsed file.
func checkFile(tb testing.TB, path string, f *ast.File) []string {
	tb.Helper()
	var problems []string

	randV2Local := ""
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			tb.Fatalf("Unquote(%s): %v", imp.Path.Value, err)
		}
		for _, banned := range bannedImports {
			if p == banned {
				problems = append(problems, path+": imports "+p)
			}
		}
		if p == "math/rand/v2" {
			if imp.Name != nil {
				randV2Local = imp.Name.Name
			} else {
				randV2Local = "rand"
			}
		}
	}

	mapTyped := map[string]bool{}
	collectMapTypedIdents(f, mapTyped)

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if node.Name == "float32" || node.Name == "float64" {
				problems = append(problems, path+": names floating-point type "+node.Name)
			}
		case *ast.SelectorExpr:
			if randV2Local != "" {
				if ident, ok := node.X.(*ast.Ident); ok && ident.Name == randV2Local && node.Sel.Name != "NewChaCha8" {
					problems = append(problems, path+": references math/rand/v2 identifier "+node.Sel.Name+" (only NewChaCha8 is permitted)")
				}
			}
			if bannedDecimalSelectors[node.Sel.Name] {
				problems = append(problems, path+": references float-valued decimal member "+node.Sel.Name)
			}
		case *ast.RangeStmt:
			if isMapRange(node.X, mapTyped) {
				problems = append(problems, path+": ranges over a map")
			}
		}
		return true
	})
	return problems
}

// collectMapTypedIdents does a best-effort, file-local scan for
// identifiers assigned a map value — via make(map[...]...), a map
// composite literal, or an explicit "var x map[...]..." declaration —
// so a later range over that identifier is recognised as a map range
// even though the range statement itself only ever names the
// identifier.
func collectMapTypedIdents(f *ast.File, out map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if node.Type != nil {
					noteIfMapType(out, name.Name, node.Type)
				}
				if i < len(node.Values) {
					noteIfMapExpr(out, name.Name, node.Values[i])
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || i >= len(node.Rhs) {
					continue
				}
				noteIfMapExpr(out, ident.Name, node.Rhs[i])
			}
		}
		return true
	})
}

func noteIfMapType(out map[string]bool, name string, typ ast.Expr) {
	if _, ok := typ.(*ast.MapType); ok {
		out[name] = true
	}
}

func noteIfMapExpr(out map[string]bool, name string, rhs ast.Expr) {
	switch e := rhs.(type) {
	case *ast.CompositeLit:
		if _, ok := e.Type.(*ast.MapType); ok {
			out[name] = true
		}
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "make" && len(e.Args) > 0 {
			if _, ok := e.Args[0].(*ast.MapType); ok {
				out[name] = true
			}
		}
	}
}

// isMapRange reports whether x — a range statement's ranged expression —
// is recognisably a map: a map composite literal, a make(map[...]...)
// call, or an identifier collectMapTypedIdents already flagged.
func isMapRange(x ast.Expr, mapTyped map[string]bool) bool {
	for {
		paren, ok := x.(*ast.ParenExpr)
		if !ok {
			break
		}
		x = paren.X
	}
	switch e := x.(type) {
	case *ast.CompositeLit:
		_, ok := e.Type.(*ast.MapType)
		return ok
	case *ast.CallExpr:
		ident, ok := e.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" || len(e.Args) == 0 {
			return false
		}
		_, ok = e.Args[0].(*ast.MapType)
		return ok
	case *ast.Ident:
		return mapTyped[e.Name]
	default:
		return false
	}
}
