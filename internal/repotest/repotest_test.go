package repotest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

// TestRoot_containsGoMod pins the ascent arithmetic: Root must resolve to
// the directory containing go.mod, so a future move of this package to a
// different depth fails loudly here rather than silently returning the
// wrong directory.
func TestRoot_containsGoMod(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	if !filepath.IsAbs(root) {
		t.Fatalf("Root() = %q, want an absolute path", root)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod under Root(): %v", err)
	}
}

// TestRootPath_resolvesToAnOpenableFile uses the tracked balance file as
// its fixture, since several replaced call sites resolve exactly this
// path.
func TestRootPath_resolvesToAnOpenableFile(t *testing.T) {
	t.Parallel()
	path := repotest.RootPath(t, "config/balance.yaml")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

// TestRootPath_dotEqualsRoot pins RootPath as a wrapper over Root rather
// than a second, independent resolver.
func TestRootPath_dotEqualsRoot(t *testing.T) {
	t.Parallel()
	if got, want := repotest.RootPath(t, "."), repotest.Root(t); got != want {
		t.Fatalf(`RootPath(t, ".") = %q, want %q`, got, want)
	}
}
