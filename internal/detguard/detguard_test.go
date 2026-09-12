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
	src := "package pkg\n\nimport \"math/rand/v2\"\n\nvar _ = rand.Float64()\n"
	dir := scratchDir(t, src)
	problems := detguard.Check(t, dir)
	if len(problems) == 0 {
		t.Fatal("want a problem for a math/rand/v2 identifier other than NewChaCha8, got none")
	}
	if !strings.Contains(problems[0], "Float64") {
		t.Errorf("problems = %v, want one naming Float64", problems)
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
// shape the design calls for: no bare float64 declaration anywhere in
// the fixture, only a decimal float-valued method result — the shape a
// declaration-only or import-only guard would walk straight past.
func TestCheck_DecimalFloatAccessorBlindShapeFlagged(t *testing.T) {
	t.Parallel()
	src := "package pkg\n\nimport \"github.com/shopspring/decimal\"\n\nfunc f(d decimal.Decimal) float64 {\n\treturn d.Float64AsSomethingElse()\n}\n\nfunc g(d decimal.Decimal) {\n\t_, _ = d.Float64()\n}\n"
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
