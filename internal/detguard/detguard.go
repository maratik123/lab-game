package detguard

import (
	"go/ast"
	"go/token"
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
	paths := srcguard.PackageFiles(tb, dir)
	files := make([]*ast.File, len(paths))
	for i, path := range paths {
		files[i] = srcguard.ParseFile(tb, path)
	}
	// A named map type ("type edgeSet map[edgeKey]bool") can be declared
	// in one file of the package and ranged over in another, so the
	// named-type set is collected across every file before any per-file
	// predicate runs.
	namedMapTypes := collectNamedMapTypes(files)

	var problems []string
	for i, path := range paths {
		problems = append(problems, checkFile(tb, path, files[i], namedMapTypes)...)
	}
	sort.Strings(problems)
	return problems
}

// checkFile applies every predicate to one already-parsed file.
// namedMapTypes is the package-wide set of type names declared as a map,
// collected by collectNamedMapTypes.
func checkFile(tb testing.TB, path string, f *ast.File, namedMapTypes map[string]bool) []string {
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
	collectMapTypedIdents(f, mapTyped, namedMapTypes)

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
			if isMapRange(node.X, mapTyped, namedMapTypes) {
				problems = append(problems, path+": ranges over a map")
			}
		}
		return true
	})
	return problems
}

// collectNamedMapTypes scans every file's top-level type declarations
// and returns the set of type names whose underlying type is a map —
// either directly ("type edgeSet map[edgeKey]bool") or, through one
// level of naming, via another name already known to be a map ("type
// edgeAlias edgeSet"). It is package-wide and file-order independent:
// the fixpoint loop resolves a name defined after the name it aliases
// just as it resolves one defined before it.
func collectNamedMapTypes(files []*ast.File) map[string]bool {
	// A slice, not a map: this package's own determinism guard forbids
	// ranging over a map, and the fixpoint loop below ranges over
	// whatever holds the declared types.
	type namedType struct {
		name string
		typ  ast.Expr
	}
	var decls []namedType
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					decls = append(decls, namedType{name: ts.Name.Name, typ: ts.Type})
				}
			}
		}
	}

	named := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, d := range decls {
			if named[d.name] {
				continue
			}
			if isMapTypeExpr(d.typ, named) {
				named[d.name] = true
				changed = true
			}
		}
	}
	return named
}

// isMapTypeExpr reports whether typ is recognisably a map type: a
// literal "map[...]..." type, or an identifier naming a type namedMaps
// already carries.
func isMapTypeExpr(typ ast.Expr, namedMaps map[string]bool) bool {
	switch t := typ.(type) {
	case *ast.MapType:
		return true
	case *ast.Ident:
		return namedMaps[t.Name]
	default:
		return false
	}
}

// collectMapTypedIdents does a best-effort, file-local scan for
// identifiers that carry a map value, so a later range over one is
// recognised as a map range even though the range statement itself only
// ever names the identifier. Four shapes reach an identifier: a
// make(...) call or a composite literal assigned to it, an explicit
// "var x ..." declaration, a map-typed function parameter, receiver or
// named result, and a map-typed struct field. In each shape the type
// may be spelled as a literal "map[...]..." or as the name of a type
// namedMapTypes already carries — a "type edgeSet map[edgeKey]bool"
// declared anywhere in the package, possibly in a different file than
// this one. The parameter shape is the one a map most often arrives in,
// so a scan that stopped at declarations and assignments would miss the
// common case while reporting clean.
func collectMapTypedIdents(f *ast.File, out, namedMapTypes map[string]bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if node.Type != nil {
					noteIfMapType(out, name.Name, node.Type, namedMapTypes)
				}
				if i < len(node.Values) {
					noteIfMapExpr(out, name.Name, node.Values[i], namedMapTypes)
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || i >= len(node.Rhs) {
					continue
				}
				noteIfMapExpr(out, ident.Name, node.Rhs[i], namedMapTypes)
			}
		case *ast.FuncDecl:
			noteMapFields(out, node.Recv, namedMapTypes)
		case *ast.FuncType:
			noteMapFields(out, node.Params, namedMapTypes)
			noteMapFields(out, node.Results, namedMapTypes)
		case *ast.StructType:
			noteMapFields(out, node.Fields, namedMapTypes)
		}
		return true
	})
}

// noteMapFields records every named field of list whose type is a map.
// A nil list and an unnamed field are both no-ops.
func noteMapFields(out map[string]bool, list *ast.FieldList, namedMapTypes map[string]bool) {
	if list == nil {
		return
	}
	for _, field := range list.List {
		for _, name := range field.Names {
			noteIfMapType(out, name.Name, field.Type, namedMapTypes)
		}
	}
}

func noteIfMapType(out map[string]bool, name string, typ ast.Expr, namedMapTypes map[string]bool) {
	if isMapTypeExpr(typ, namedMapTypes) {
		out[name] = true
	}
}

func noteIfMapExpr(out map[string]bool, name string, rhs ast.Expr, namedMapTypes map[string]bool) {
	switch e := rhs.(type) {
	case *ast.CompositeLit:
		if isMapTypeExpr(e.Type, namedMapTypes) {
			out[name] = true
		}
	case *ast.CallExpr:
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "make" && len(e.Args) > 0 {
			if isMapTypeExpr(e.Args[0], namedMapTypes) {
				out[name] = true
			}
		}
	}
}

// isMapRange reports whether x — a range statement's ranged expression —
// is recognisably a map: a map composite literal, a make(map[...]...)
// call (either spelled as a literal map type or as a name
// namedMapTypes carries), or an identifier collectMapTypedIdents
// already flagged.
func isMapRange(x ast.Expr, mapTyped, namedMapTypes map[string]bool) bool {
	for {
		paren, ok := x.(*ast.ParenExpr)
		if !ok {
			break
		}
		x = paren.X
	}
	switch e := x.(type) {
	case *ast.CompositeLit:
		return isMapTypeExpr(e.Type, namedMapTypes)
	case *ast.CallExpr:
		ident, ok := e.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" || len(e.Args) == 0 {
			return false
		}
		return isMapTypeExpr(e.Args[0], namedMapTypes)
	case *ast.Ident:
		return mapTyped[e.Name]
	default:
		return false
	}
}
