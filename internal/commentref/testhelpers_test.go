package commentref

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// readRepoFile reads rel, relative to root, failing the test on error.
func readRepoFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return content
}

// gitLsFiles lists tracked paths under root matching pattern.
func gitLsFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "-C", root, "ls-files", pattern).Output()
	if err != nil {
		t.Fatalf("git ls-files %s: %v", pattern, err)
	}
	return strings.Fields(string(out))
}

// assertComments compares got against want, both ordered by line, failing
// with a readable diff when they differ.
func assertComments(t *testing.T, got, want []Comment) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d comments, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("comment %d: got %+v, want %+v\nfull got:  %+v\nfull want: %+v", i, got[i], want[i], got, want)
		}
	}
}
