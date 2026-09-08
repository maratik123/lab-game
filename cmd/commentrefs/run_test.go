package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scratchRepo builds a fresh git repository under t.TempDir(), configures
// a commit identity local to it, and returns its root.
func scratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runOK(t, dir, "init", "-q")
	runOK(t, dir, "config", "user.email", "test@example.com")
	runOK(t, dir, "config", "user.name", "test")
	return dir
}

func runOK(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestRun_CleanFileExitsZeroAndPrintsNothing(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "clean.go", "package p\n\n// Foo does a thing.\nfunc Foo() {}\n")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"clean.go"}, &stdout, &stderr)

	if code != exitClean {
		t.Fatalf("run() = %d, want %d\nstdout: %s\nstderr: %s", code, exitClean, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRun_DirtyFileExitsOneAndReportsFileLineClassText(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "dirty.go", "package p\n\n// Foo does a thing (see AC6).\nfunc Foo() {}\n")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"dirty.go"}, &stdout, &stderr)

	if code != exitFindings {
		t.Fatalf("run() = %d, want %d\nstderr: %s", code, exitFindings, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"dirty.go", "3", "ac-id", "AC6"} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout = %q, want it to contain %q", got, want)
		}
	}
}

func TestRun_UnreadablePathExitsTwo(t *testing.T) {
	repo := scratchRepo(t)
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"does-not-exist.go"}, &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr is empty, want the instrument failure named")
	}
}

func TestRun_WholeTreeMode(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "a.go", "package p\n\n// Bar does a thing (see AC1).\nfunc Bar() {}\n")
	writeFile(t, repo, "clean.go", "package p\n\n// Baz does a thing.\nfunc Baz() {}\n")
	runOK(t, repo, "add", "a.go", "clean.go")
	runOK(t, repo, "commit", "-q", "-m", "add fixtures")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != exitFindings {
		t.Fatalf("run() = %d, want %d\nstderr: %s", code, exitFindings, stderr.String())
	}
	if !strings.Contains(stdout.String(), "a.go") || strings.Contains(stdout.String(), "clean.go:") {
		t.Fatalf("stdout = %q, want a.go's finding and no report line for clean.go", stdout.String())
	}
}

// TestRun_StagedMode_IndexDisagreesWithWorktree drives --staged against a
// scratch repository whose index and worktree disagree, asserting the
// index blob — what a commit would actually record — is what is judged,
// not the worktree's own, different content.
func TestRun_StagedMode_IndexDisagreesWithWorktree(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "staged.go", "package p\n\n// Foo does a thing (see AC1).\nfunc Foo() {}\n")
	runOK(t, repo, "add", "staged.go")
	// The worktree is now dirtied to something clean, but the index still
	// holds the banned reference above.
	writeFile(t, repo, "staged.go", "package p\n\n// Foo does a thing.\nfunc Foo() {}\n")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--staged"}, &stdout, &stderr)

	if code != exitFindings {
		t.Fatalf("run() = %d, want %d (the index blob still carries the finding)\nstdout: %s\nstderr: %s", code, exitFindings, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "ac-id") {
		t.Fatalf("stdout = %q, want the index blob's ac-id finding", stdout.String())
	}
}

func TestRun_StagedMode_NothingStagedExitsClean(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "untracked.go", "package p\n\n// Foo (see AC1).\nfunc Foo() {}\n")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--staged"}, &stdout, &stderr)

	if code != exitClean {
		t.Fatalf("run() = %d, want %d\nstdout: %s\nstderr: %s", code, exitClean, stdout.String(), stderr.String())
	}
}

func TestRun_NoGitWorktreeExitsFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("run() = %d, want %d", code, exitFailure)
	}
}

func TestScan_SymlinkIsSkippedAndReported(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := scan([]fileInput{{path: "cmd/bot/main.go", isSymlink: true}}, map[string]struct{}{}, &stdout, &stderr)

	if code != exitClean {
		t.Fatalf("scan() = %d, want %d", code, exitClean)
	}
	if !strings.Contains(stdout.String(), "SKIP cmd/bot/main.go") {
		t.Fatalf("stdout = %q, want a SKIP line for the symlink", stdout.String())
	}
}

func TestScan_RouteErrorIsAnInstrumentFailure(t *testing.T) {
	f := fileInput{path: ".githooks/README", isSymlink: false, content: []byte("not a shebang\n")}
	var stdout, stderr bytes.Buffer
	code := scan([]fileInput{f}, map[string]struct{}{}, &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("scan() = %d, want %d\nstderr: %s", code, exitFailure, stderr.String())
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr is empty, want the routing failure named")
	}
}

func TestScan_ExtractErrorIsAnInstrumentFailure(t *testing.T) {
	f := fileInput{path: "bad.go", isSymlink: false, content: []byte("package p\n\n/* unterminated\n")}
	var stdout, stderr bytes.Buffer
	code := scan([]fileInput{f}, map[string]struct{}{}, &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("scan() = %d, want %d\nstderr: %s", code, exitFailure, stderr.String())
	}
}

func TestScan_UngatedPathIsIgnored(t *testing.T) {
	f := fileInput{path: "README.md", isSymlink: false, content: []byte("# hello\n")}
	var stdout, stderr bytes.Buffer
	code := scan([]fileInput{f}, map[string]struct{}{}, &stdout, &stderr)

	if code != exitClean {
		t.Fatalf("scan() = %d, want %d", code, exitClean)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestGoPackageClause_Unparseable(t *testing.T) {
	if got := goPackageClause([]byte("not go source {{{")); got != "" {
		t.Fatalf("goPackageClause() = %q, want empty on a parse failure", got)
	}
}

func TestRun_ExplicitPathsMode_MultipleFiles(t *testing.T) {
	repo := scratchRepo(t)
	writeFile(t, repo, "clean.go", "package p\n\n// Foo does a thing.\nfunc Foo() {}\n")
	writeFile(t, repo, "dirty.go", "package p\n\n// Bar (see AC1).\nfunc Bar() {}\n")
	t.Chdir(repo)

	var stdout, stderr bytes.Buffer
	code := run([]string{"clean.go", "dirty.go"}, &stdout, &stderr)

	if code != exitFindings {
		t.Fatalf("run() = %d, want %d\nstderr: %s", code, exitFindings, stderr.String())
	}
	if strings.Contains(stdout.String(), "clean.go:") {
		t.Fatalf("stdout = %q, want no report line for clean.go", stdout.String())
	}
}
