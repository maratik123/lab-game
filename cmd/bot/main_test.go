package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/repotest"
)

func mapLookup(m map[string]string) config.Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// validEnv returns a fresh, fully valid environment, pointing the balance
// and world paths at the repository's own tracked configuration artefacts.
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
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — configuration is loaded before anything is written to stdout", stdout.String())
	}
}

func TestRun_ValidEnvironment(t *testing.T) {
	t.Parallel()
	var stderr, stdout bytes.Buffer
	code := run(mapLookup(validEnv(t)), &stderr, &stdout)

	if code != 0 {
		t.Errorf("run: exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("stdout = %q, want it to contain the build identity %q", stdout.String(), version)
	}
}
