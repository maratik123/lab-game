package srcguard_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/srcguard"
)

// TestPackageFiles_ReturnsNonTestSourcesOnly enumerates a scratch
// directory holding one production source file, one test file and one
// nested subpackage: it must return only the production file, and never
// descend into the subdirectory.
func TestPackageFiles_ReturnsNonTestSourcesOnly(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "pkg/main.go", "package pkg\n")
	_ = srcguard.WriteScratchFileIn(t, root, "pkg/main_test.go", "package pkg\n")
	_ = srcguard.WriteScratchFileIn(t, root, "pkg/sub/nested.go", "package sub\n")

	got := srcguard.PackageFiles(t, filepath.Join(root, "pkg"))
	if len(got) != 1 {
		t.Fatalf("PackageFiles = %v, want exactly one non-test file", got)
	}
	if filepath.Base(got[0]) != "main.go" {
		t.Fatalf("PackageFiles = %v, want [main.go]", got)
	}
}

// TestWalkSubtree_SkipsGitAndTmp proves the subtree walker visits a
// nested package and skips both .git and the repository's designated
// scratch directory ("tmp" directly under root).
func TestWalkSubtree_SkipsGitAndTmp(t *testing.T) {
	t.Parallel()
	root, _ := srcguard.WriteScratchFile(t, "internal/pkg/a.go", "package pkg\n")
	_ = srcguard.WriteScratchFileIn(t, root, "internal/pkg/sub/b.go", "package sub\n")
	_ = srcguard.WriteScratchFileIn(t, root, ".git/objects/pack/c.go", "package ignored\n")
	_ = srcguard.WriteScratchFileIn(t, root, "tmp/d.go", "package ignored\n")

	var visited []string
	srcguard.WalkSubtree(t, root, func(path string) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		visited = append(visited, filepath.ToSlash(rel))
	})

	want := map[string]bool{
		"internal/pkg/a.go":     true,
		"internal/pkg/sub/b.go": true,
	}
	if len(visited) != len(want) {
		t.Fatalf("WalkSubtree visited %v, want exactly %v", visited, want)
	}
	for _, v := range visited {
		if !want[v] {
			t.Errorf("WalkSubtree visited %q, which should have been skipped", v)
		}
	}
}

// TestNonTestFile_FiltersTestFiles pins the filter a WalkSubtree
// caller applies to reach PackageFiles' own test-excluded scope.
func TestNonTestFile_FiltersTestFiles(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"a.go":      true,
		"a_test.go": false,
		"a.txt":     false,
	}
	for path, want := range cases {
		if got := srcguard.NonTestFile(path); got != want {
			t.Errorf("NonTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestParseFile_ReadsContent parses a scratch file and checks the
// resulting AST carries the declared package name.
func TestParseFile_ReadsContent(t *testing.T) {
	t.Parallel()
	_, path := srcguard.WriteScratchFile(t, "pkg/main.go", "package pkg\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n")
	f := srcguard.ParseFile(t, path)
	if f.Name.Name != "pkg" {
		t.Fatalf("ParseFile package name = %q, want %q", f.Name.Name, "pkg")
	}
}

// TestParseFiles_ParsesEveryPath proves the batch parser keys its
// result by each input path.
func TestParseFiles_ParsesEveryPath(t *testing.T) {
	t.Parallel()
	root, first := srcguard.WriteScratchFile(t, "pkg/a.go", "package pkg\n")
	second := srcguard.WriteScratchFileIn(t, root, "pkg/b.go", "package pkg\n")

	got := srcguard.ParseFiles(t, []string{first, second})
	if len(got) != 2 {
		t.Fatalf("ParseFiles returned %d entries, want 2", len(got))
	}
	for _, path := range []string{first, second} {
		if got[path] == nil {
			t.Errorf("ParseFiles missing entry for %s", path)
		}
	}
}

// TestImportPaths_UnquotesEveryImport proves the import enumerator
// returns the unquoted import path strings, matching AST import specs.
func TestImportPaths_UnquotesEveryImport(t *testing.T) {
	t.Parallel()
	_, path := srcguard.WriteScratchFile(t, "pkg/main.go", "package pkg\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nvar _ = fmt.Sprint\nvar _ = strings.ToUpper\n")
	f := srcguard.ParseFile(t, path)
	got := srcguard.ImportPaths(t, f)
	want := map[string]bool{"fmt": true, "strings": true}
	if len(got) != len(want) {
		t.Fatalf("ImportPaths = %v, want %v", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("ImportPaths returned unexpected import %q", p)
		}
	}
}

// TestWriteScratchFile_WritesUnderItsOwnTempDir proves the scratch
// writer's file lands under its own returned root, and never touches
// the working tree — the property every discriminating guard downstream
// depends on: a guard walked against the returned root sees only what
// this call wrote.
func TestWriteScratchFile_WritesUnderItsOwnTempDir(t *testing.T) {
	t.Parallel()
	root, path := srcguard.WriteScratchFile(t, "internal/scratch/scratch.go", "package scratch\n\nfunc f() { panic(\"boom\") }\n")
	if !strings.HasPrefix(path, root) {
		t.Fatalf("WriteScratchFile path %q is not under its own root %q", path, root)
	}
	found := srcguard.PackageFiles(t, filepath.Join(root, "internal", "scratch"))
	if len(found) != 1 || found[0] != path {
		t.Fatalf("PackageFiles(root/internal/scratch) = %v, want [%s]", found, path)
	}
}

// TestExcludedByDirName covers the shapes the go tool itself never
// compiles, and the shapes it does — including the case a directory-only
// predicate must NOT catch: a file whose own name is excluded-looking but
// whose directories are not.
func TestExcludedByDirName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		relPath string
		want    bool
	}{
		{"file directly under root", "main.go", false},
		{"under testdata", "internal/pkg/testdata/fixture.go", true},
		{"under a dot-prefixed directory", "internal/pkg/.hidden/f.go", true},
		{"under an underscore-prefixed directory", "internal/pkg/_ignored/f.go", true},
		{"underscore-prefixed file name, non-excluded directories", "internal/pkg/_helper.go", false},
		{"only an inner segment excluded", "internal/pkg/testdata/nested/deep.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := srcguard.ExcludedByDirName(tc.relPath); got != tc.want {
				t.Errorf("ExcludedByDirName(%q) = %v, want %v", tc.relPath, got, tc.want)
			}
		})
	}
}
