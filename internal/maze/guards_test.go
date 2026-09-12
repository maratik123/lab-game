package maze

import (
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
