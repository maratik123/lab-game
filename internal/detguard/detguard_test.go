package detguard_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/detguard"
	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/srcguard"
)

func TestCheck_OwnPackagePasses(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, filepath.Join("internal", "detguard"))
	if problems := detguard.Check(t, dir); len(problems) != 0 {
		t.Errorf("detguard.Check(detguard) = %v, want none", problems)
	}
}

func scratchDir(t *testing.T, content string) string {
	t.Helper()
	_, path := srcguard.WriteScratchFile(t, "pkg/f.go", content)
	return filepath.Dir(path)
}

func TestCheck_BannedImportsFlagged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		importPath string
		use        string
	}{
		{"time", "var _ = time.Now"},
		{"math/rand", "var _ = rand.Int"},
		{"hash/maphash", "var _ = maphash.Hash{}"},
		{"crypto/rand", "var _ = rand.Reader"},
		{"math", "var _ = math.Pi"},
	}
	for _, c := range cases {
		t.Run(c.importPath, func(t *testing.T) {
			t.Parallel()
			importName := c.importPath[strings.LastIndex(c.importPath, "/")+1:]
			src := "package pkg\n\nimport " + importName + " \"" + c.importPath + "\"\n\n" + c.use + "\n"
			dir := scratchDir(t, src)
			problems := detguard.Check(t, dir)
			if len(problems) == 0 {
				t.Fatalf("want a problem for importing %q, got none", c.importPath)
			}
			if !strings.Contains(problems[0], c.importPath) {
				t.Errorf("problems = %v, want one naming %q", problems, c.importPath)
			}
		})
	}
}

func TestCheck_RandV2NonConstructorFlagged(t *testing.T) {
	t.Parallel()
	// Uint64 rather than Float64: Float64 is also a banned decimal
	// selector, and that predicate matches on selector name alone, so
	// it would satisfy this assertion while the rand/v2 predicate did
	// nothing.
	src := "package pkg\n\nimport \"math/rand/v2\"\n\nvar _ = rand.Uint64()\n"
	dir := scratchDir(t, src)
	problems := detguard.Check(t, dir)
	if len(problems) == 0 {
		t.Fatal("want a problem for a math/rand/v2 identifier other than NewChaCha8, got none")
	}
	var found bool
	for _, p := range problems {
		if strings.Contains(p, "math/rand/v2 identifier Uint64") {
			found = true
		}
	}
	if !found {
		t.Errorf("problems = %v, want one naming the math/rand/v2 identifier Uint64", problems)
	}
}

func TestCheck_RandV2ConstructorAllowed(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nimport \"math/rand/v2\"\n\nvar _ = rand.NewChaCha8([32]byte{})\n"
	dir := scratchDir(t, src)
	if problems := detguard.Check(t, dir); len(problems) != 0 {
		t.Errorf("Check flagged the permitted NewChaCha8 constructor: %v", problems)
	}
}

func TestCheck_FloatDeclarationFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nvar x float64 = 1.0\n"
	dir := scratchDir(t, src)
	problems := detguard.Check(t, dir)
	if len(problems) == 0 {
		t.Fatal("want a problem for a bare float64 declaration, got none")
	}
	if !strings.Contains(problems[0], "float64") {
		t.Errorf("problems = %v, want one naming float64", problems)
	}
}

func TestCheck_MathImportFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nimport \"math\"\n\nvar _ = math.Floor(1.0)\n"
	dir := scratchDir(t, src)
	if problems := detguard.Check(t, dir); len(problems) == 0 {
		t.Fatal("want a problem for importing math, got none")
	}
}

// TestCheck_DecimalFloatAccessorBlindShapeFlagged carries the blind
// shape: no float64 spelled anywhere in the fixture, only a decimal
// float-valued method result — the shape a declaration-only or
// import-only guard walks straight past. The fixture must stay free of
// a bare float64 declaration, or the float-type predicate satisfies the
// assertion and this case stops discriminating.
func TestCheck_DecimalFloatAccessorBlindShapeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nimport \"github.com/shopspring/decimal\"\n\nfunc g(d decimal.Decimal) {\n\t_, _ = d.Float64()\n}\n"
	dir := scratchDir(t, src)
	problems := detguard.Check(t, dir)
	if len(problems) == 0 {
		t.Fatal("want a problem for the decimal Float64() accessor call with no bare float64 declaration, got none")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "Float64") {
		t.Errorf("problems = %v, want one naming Float64", problems)
	}
}

func TestCheck_MapRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc f() {\n\tm := map[string]int{}\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	problems := detguard.Check(t, dir)
	if len(problems) == 0 {
		t.Fatal("want a problem for ranging over a map, got none")
	}
	if !strings.Contains(problems[0], "map") {
		t.Errorf("problems = %v, want one naming a map range", problems)
	}
}

func TestCheck_SliceRangeNotFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc f() {\n\ts := []int{1, 2}\n\tfor _, v := range s {\n\t\t_ = v\n\t}\n}\n"
	dir := scratchDir(t, src)
	if problems := detguard.Check(t, dir); len(problems) != 0 {
		t.Errorf("Check flagged a slice range: %v", problems)
	}
}

func TestCheck_DirectMapCompositeLitRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc f() {\n\tfor k := range (map[string]int{\"a\": 1}) {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	if problems := detguard.Check(t, dir); len(problems) == 0 {
		t.Fatal("want a problem for ranging over an inline map composite literal, got none")
	}
}

// wantMapRangeMessage fails t unless problems carries the map-range
// predicate's exact message, so a fixture that happens to also trip a
// different predicate does not satisfy the assertion.
func wantMapRangeMessage(t *testing.T, problems []string) {
	t.Helper()
	if len(problems) == 0 {
		t.Fatal("want a problem for ranging over a map, got none")
	}
	for _, p := range problems {
		if strings.Contains(p, "ranges over a map") {
			return
		}
	}
	t.Errorf("problems = %v, want one naming a map range", problems)
}

// TestCheck_MapParameterRangeFlagged carries the parameter shape: the
// map arrives as a function parameter, never assigned or declared
// inside the function body, so a scan that only tracked make(map...)
// calls and var declarations would walk straight past it.
func TestCheck_MapParameterRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc f(m map[string]int) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_MapReceiverRangeFlagged carries the receiver shape: the map
// arrives as a method's receiver identifier.
func TestCheck_MapReceiverRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc (m map[string]int) f() {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_MapNamedResultRangeFlagged carries the named-result shape:
// the map arrives as a function's named return value, ranged over
// before being returned.
func TestCheck_MapNamedResultRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc f() (m map[string]int) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n\treturn m\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_MapStructFieldRangeFlagged carries the struct-field shape:
// a same-file struct field types the name "m" as a map, and a function
// elsewhere in the file assigns that same name from an ordinary call —
// not a recognised make(...) construction — before ranging over it. The
// collector is a best-effort, file-scope name match rather than a real
// scope resolution, so the struct field's declaration is what makes
// this occurrable spelling flagged at all: the paired control below,
// identical but for the struct declaration, is clean, which is what
// isolates the struct-field arm as the one doing the work here.
func TestCheck_MapStructFieldRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype T struct {\n\tm map[string]int\n}\n\nfunc newMap() map[string]int { return nil }\n\nfunc f() {\n\tm := newMap()\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_MapStructFieldRangeFlagged_ControlWithoutStructField is the
// control for the case above: the identical fixture with the struct
// field declaration removed. newMap's return value is an ordinary call
// result, which the collector's best-effort scan does not recognise as
// a map source (only a make(...) call is), so with no struct field to
// type the name "m" as a map anywhere in the file, the range is clean.
func TestCheck_MapStructFieldRangeFlagged_ControlWithoutStructField(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nfunc newMap() map[string]int { return nil }\n\nfunc f() {\n\tm := newMap()\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	if got := detguard.Check(t, dir); len(got) != 0 {
		t.Fatalf("Check = %v, want no problems", got)
	}
}

// TestCheck_NamedMapTypeParameterRangeFlagged carries the parameter
// shape for a map that arrives under a declared name — "type edgeSet
// map[edgeKey]bool" — rather than a literal "map[...]..." spelling.
func TestCheck_NamedMapTypeParameterRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\nfunc f(m edgeSet) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeReceiverRangeFlagged carries the receiver shape
// for a named map type.
func TestCheck_NamedMapTypeReceiverRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\nfunc (m edgeSet) f() {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeNamedResultRangeFlagged carries the named-result
// shape for a named map type.
func TestCheck_NamedMapTypeNamedResultRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\nfunc f() (m edgeSet) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n\treturn m\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeStructFieldRangeFlagged carries the struct-field
// shape for a named map type: a same-file struct field types the name
// "m" as edgeSet, not as a literal "map[...]...", and a function
// elsewhere in the file assigns that same name from an ordinary call
// before ranging over it. As with the literal-type case above, the
// struct field's declaration is what makes this occurrable spelling
// flagged; the paired control, identical but for the struct
// declaration, is clean.
func TestCheck_NamedMapTypeStructFieldRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\ntype T struct {\n\tm edgeSet\n}\n\nfunc newSet() edgeSet { return nil }\n\nfunc f() {\n\tm := newSet()\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeStructFieldRangeFlagged_ControlWithoutStructField
// is the control for the case above: the identical fixture with the
// struct field declaration removed, leaving the range clean.
func TestCheck_NamedMapTypeStructFieldRangeFlagged_ControlWithoutStructField(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\nfunc newSet() edgeSet { return nil }\n\nfunc f() {\n\tm := newSet()\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	if got := detguard.Check(t, dir); len(got) != 0 {
		t.Fatalf("Check = %v, want no problems", got)
	}
}

// TestCheck_NamedMapTypeVarAssignRangeFlagged carries the var/assign
// shape for a named map type: a make(edgeSet) call assigned to a plain
// (":=") variable, with no literal "map[...]..." spelled anywhere in the
// fixture.
func TestCheck_NamedMapTypeVarAssignRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype edgeSet map[string]bool\n\nfunc f() {\n\tm := make(edgeSet)\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeMultiHopChainRangeFlagged carries the shape the
// fixpoint loop exists for: a chain of type names three deep, each
// aliasing the next ("type c b", "type b a", "type a map[...]...") and
// declared in that same, worst-case order — the alias appearing before
// the name it depends on, at every link. A single pass over the
// declarations in source order resolves only "a" and never reaches "b"
// or "c"; only repeating the pass until nothing new is found resolves
// the whole chain, which is what this case is for.
func TestCheck_NamedMapTypeMultiHopChainRangeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\ntype c b\n\ntype b a\n\ntype a map[string]bool\n\nfunc f(m c) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n"
	dir := scratchDir(t, src)
	wantMapRangeMessage(t, detguard.Check(t, dir))
}

// TestCheck_NamedMapTypeCrossFileRangeFlagged carries the shape that
// forced Check to collect named map types across the whole package
// before running any per-file predicate: the "type edgeSet
// map[...]..." declaration lives in one file of the scratch package,
// and the parameter that is ranged over lives in a second file of that
// same package.
func TestCheck_NamedMapTypeCrossFileRangeFlagged(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/types.go", "package pkg\n\ntype edgeSet map[string]bool\n")
	srcguard.WriteScratchFileIn(t, root, "pkg/use.go", "package pkg\n\nfunc f(m edgeSet) {\n\tfor k := range m {\n\t\t_ = k\n\t}\n}\n")
	dir := filepath.Join(root, "pkg")
	wantMapRangeMessage(t, detguard.Check(t, dir))
}
