package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

// TestVersion_LinkTimeOverrideReachesTheIdentityLine builds this
// command into a temporary directory with -ldflags "-X
// main.version=<sentinel>", runs it with one required key deliberately
// absent, and asserts: a non-zero exit, the sentinel in the identity
// line on stderr, the failing step named, and stdout empty. It exists
// for the one proposition an in-process test cannot reach — that the
// release pipeline's own -ldflags mechanism actually rewrites the
// version var this binary reports. One subprocess, no ports, no
// database.
func TestVersion_LinkTimeOverrideReachesTheIdentityLine(t *testing.T) {
	t.Parallel()

	const sentinel = "SENTINEL-1.2.3-link-time-version"

	root := repotest.Root(t)
	binPath := filepath.Join(t.TempDir(), "bot")

	buildCmd := exec.CommandContext(context.Background(), "go", "build",
		"-ldflags", "-X main.version="+sentinel,
		"-o", binPath,
		"./cmd/bot",
	)
	buildCmd.Dir = root
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	runCmd := exec.CommandContext(context.Background(), binPath)
	// Deliberately leave every LAB_GAME_ environment variable absent —
	// the configuration step fails immediately, before any network or
	// database call, which is all this case needs.
	runCmd.Env = []string{}
	var stdout, stderr strings.Builder
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	err := runCmd.Run()
	if err == nil {
		t.Fatal("bot exited zero with no configuration, want non-zero")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run: %v (not an *exec.ExitError)", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Errorf("exit code = 0, want non-zero")
	}

	if !strings.Contains(stderr.String(), sentinel) {
		t.Errorf("stderr = %q, want it to carry the link-time sentinel %q", stderr.String(), sentinel)
	}
	if !strings.Contains(stderr.String(), "configuration") {
		t.Errorf("stderr = %q, want it to name the configuration step", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}
