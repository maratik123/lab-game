package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

// validEnv returns a fresh, fully valid environment for run's
// configuration step, pointing the balance and world paths at the
// repository's own tracked configuration artefacts, and an unroutable
// DSN (port 1 on loopback: nothing ever listens there) so a test that
// gets past configuration fails fast at the database step rather than
// waiting out pingTimeout against a DSN that merely looks unreachable.
// It is deliberately not a database-backed environment; the assembled
// happy path is a sibling fixture's own environment builder.
func validEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"LAB_GAME_BOT_TOKEN":        "test-token",
		"LAB_GAME_DSN":              "postgres://user:pass@127.0.0.1:1/db",
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
	code := run(nil, mapLookup(env), &stderr, &stdout)

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
// assembly step's own fixture, and validEnv's DSN is deliberately
// unreachable, so this case exercises the no-args path's database
// failure, not a full assembled success).
func TestRun_IdentityIsOnStderr(t *testing.T) {
	t.Parallel()
	var stderr, stdout bytes.Buffer
	code := run(nil, mapLookup(validEnv(t)), &stderr, &stdout)

	if code == 0 {
		t.Error("run: expected a non-zero exit code — validEnv's DSN is unreachable by design")
	}
	if !strings.Contains(stderr.String(), version) {
		t.Errorf("stderr = %q, want it to carry the build identity %q", stderr.String(), version)
	}
	if !strings.Contains(stderr.String(), "database") {
		t.Errorf("stderr = %q, want it to name the database step", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — run's argv-dispatch shape is the only stdout writer", stdout.String())
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	var stderr, stdout bytes.Buffer
	code := run([]string{"bogus"}, mapLookup(validEnv(t)), &stderr, &stdout)

	if code != exitUsage {
		t.Errorf("run: exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "Usage") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRun_HelpFlags(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			var stderr, stdout bytes.Buffer
			code := run([]string{flag}, mapLookup(validEnv(t)), &stderr, &stdout)

			if code != 0 {
				t.Errorf("run: exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout.String(), "Usage") {
				t.Errorf("stdout = %q, want usage text", stdout.String())
			}
			if !strings.Contains(stderr.String(), version) {
				t.Errorf("stderr = %q, want the build identity, and nothing else — the only writer of stdout is -h/--help", stderr.String())
			}
		})
	}
}

func TestRun_MigrateDispatchesToMigrateOnly(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)

	var stderr, stdout bytes.Buffer
	code := run([]string{"migrate"}, mapLookup(env), &stderr, &stdout)

	if code != 0 {
		t.Errorf("run([migrate]) = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on the migrate-only path", stdout.String())
	}
}
