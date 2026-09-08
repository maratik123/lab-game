package main

import (
	"testing"
)

func TestRunGit_ErrorNamesTheCommand(t *testing.T) {
	dir := t.TempDir() // not a git repository
	_, err := runGit(t.Context(), dir, "rev-parse", "--show-toplevel")
	if err == nil {
		t.Fatal("runGit() error = nil, want an error outside a git worktree")
	}
}

func TestTrackedPaths(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "a.go", "package p\n")
	runOK(t, repo, "add", "a.go")
	runOK(t, repo, "commit", "-q", "-m", "add a.go")

	got, err := trackedPaths(t.Context(), repo)
	if err != nil {
		t.Fatalf("trackedPaths() error = %v", err)
	}
	if len(got) != 1 || got[0] != "a.go" {
		t.Fatalf("trackedPaths() = %v, want [a.go]", got)
	}
}

func TestTrackedPaths_Error(t *testing.T) {
	dir := t.TempDir()
	if _, err := trackedPaths(t.Context(), dir); err == nil {
		t.Fatal("trackedPaths() error = nil, want an error outside a git worktree")
	}
}

func TestStagedEntries(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "staged.go", "package p\n")
	runOK(t, repo, "add", "staged.go")

	got, err := stagedEntries(t.Context(), repo)
	if err != nil {
		t.Fatalf("stagedEntries() error = %v", err)
	}
	if len(got) != 1 || got[0].path != "staged.go" || got[0].mode != "100644" {
		t.Fatalf("stagedEntries() = %+v, want one 100644 entry for staged.go", got)
	}
}

func TestStagedEntries_Empty(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "untracked.go", "package p\n")

	got, err := stagedEntries(t.Context(), repo)
	if err != nil {
		t.Fatalf("stagedEntries() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("stagedEntries() = %v, want none", got)
	}
}

func TestStagedEntries_Error(t *testing.T) {
	dir := t.TempDir()
	if _, err := stagedEntries(t.Context(), dir); err == nil {
		t.Fatal("stagedEntries() error = nil, want an error outside a git worktree")
	}
}

func TestIndexBlob(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "a.go", "package p\n")
	runOK(t, repo, "add", "a.go")

	got, err := indexBlob(t.Context(), repo, "a.go")
	if err != nil {
		t.Fatalf("indexBlob() error = %v", err)
	}
	if string(got) != "package p\n" {
		t.Fatalf("indexBlob() = %q, want the staged content", got)
	}
}

func TestModulePackageNames_Success(t *testing.T) {
	root := repoRootForTest(t)
	got, err := modulePackageNames(t.Context(), root)
	if err != nil {
		t.Fatalf("modulePackageNames() error = %v", err)
	}
	if _, ok := got["commentref"]; !ok {
		t.Fatalf("modulePackageNames() = %v, want it to include commentref", got)
	}
}

func TestModulePackageNames_Error(t *testing.T) {
	dir := t.TempDir()
	if _, err := modulePackageNames(t.Context(), dir); err == nil {
		t.Fatal("modulePackageNames() error = nil, want an error with no go.mod")
	}
}

// repoRootForTest resolves this module's own repository root, for tests
// that need real Go package data rather than a scratch fixture.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	out, err := runGit(t.Context(), ".", "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	root := string(out)
	for len(root) > 0 && (root[len(root)-1] == '\n' || root[len(root)-1] == '\r') {
		root = root[:len(root)-1]
	}
	return root
}

func TestSplitNulTerminated_Empty(t *testing.T) {
	if got := splitNulTerminated(nil); got != nil {
		t.Fatalf("splitNulTerminated(nil) = %v, want nil", got)
	}
}
