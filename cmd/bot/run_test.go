package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

// validEnv returns a fresh, fully valid environment for run's
// configuration step, pointing the balance and world paths at the
// repository's own tracked configuration artefacts. It is deliberately
// not a database-backed environment — run's configuration step is
// exercised here without a real Postgres or a real Bot API; the
// assembled happy path is a sibling fixture's own environment
// builder.
func validEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"LAB_GAME_BOT_TOKEN":        "test-token",
		"LAB_GAME_DSN":              "postgres://user:pass@localhost/db",
		"LAB_GAME_BOT_API_BASE_URL": "https://api.telegram.org",
		"LAB_GAME_ALLOWED_CHAT_IDS": "-100123456789",
		"LAB_GAME_BALANCE_PATH":     repotest.RootPath(t, "config/balance.yaml"),
		"LAB_GAME_WORLD_PATH":       repotest.RootPath(t, "config/world"),
	}
}

func TestRun_MissingVariable(t *testing.T) {
	t.Parallel()
	env := validEnv(t)
	delete(env, "LAB_GAME_BOT_TOKEN")

	var stderr, stdout bytes.Buffer
	code := run(mapLookup(env), &stderr, &stdout)

	if code == 0 {
		t.Error("run: expected a non-zero exit code")
	}
	if !strings.Contains(stderr.String(), "LAB_GAME_BOT_TOKEN") {
		t.Errorf("stderr = %q, want it to name LAB_GAME_BOT_TOKEN", stderr.String())
	}
	if !strings.Contains(stderr.String(), version) {
		t.Errorf("stderr = %q, want it to carry the build identity %q", stderr.String(), version)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// TestRun_IdentityIsOnStderr proves the build identity appears on
// stderr and stdout is empty, on the configuration-failure path
// (TestRun_MissingVariable, above) and on the path that gets past
// configuration alike — run's own configuration step succeeding is
// enough to prove the stream never flips, independent of whether the
// database it names is reachable (that boundary belongs to the
// assembly step's own fixture).
func TestRun_IdentityIsOnStderr(t *testing.T) {
	t.Parallel()
	var stderr, stdout bytes.Buffer
	code := run(mapLookup(validEnv(t)), &stderr, &stdout)

	if code != 0 {
		t.Errorf("run: exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), version) {
		t.Errorf("stderr = %q, want it to carry the build identity %q", stderr.String(), version)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — run's argv-dispatch shape is the only stdout writer", stdout.String())
	}
}
