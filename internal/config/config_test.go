package config

import (
	"fmt"
	"strings"
	"testing"
)

// validConfigEnv returns a fresh, fully valid environment: validEnv()'s
// token/DSN/base-URL/chat-ids, plus a real temp balance file and a real
// temp world directory (validEnv()'s own placeholder paths do not exist).
func validConfigEnv(t *testing.T) map[string]string {
	t.Helper()
	env := validEnv()
	env[envBalancePath] = writeBalanceFile(t, validBalanceYAML)
	env[envWorldPath] = t.TempDir()
	return env
}

func TestLoad_HappyPath(t *testing.T) {
	t.Parallel()
	cfg, err := Load(mapLookup(validConfigEnv(t)))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.BotToken.Reveal() != "test-token" {
		t.Errorf("BotToken.Reveal() = %q", cfg.BotToken.Reveal())
	}
	if cfg.DSN.Reveal() != "postgres://user:pass@localhost/db" {
		t.Errorf("DSN.Reveal() = %q", cfg.DSN.Reveal())
	}
	if cfg.BotAPIBaseURL.String() != "https://api.telegram.org" {
		t.Errorf("BotAPIBaseURL = %q", cfg.BotAPIBaseURL.String())
	}
	if len(cfg.AllowedChatIDs) != 2 {
		t.Errorf("AllowedChatIDs = %v", cfg.AllowedChatIDs)
	}
	if cfg.WorldPath == "" {
		t.Error("WorldPath is empty")
	}
	assertBalanceEqual(t, &cfg.Balance, validBalance())
}

// TestLoad_EnvironmentFailureNoFileRead is the design's "an environment
// failure → the error names the variable and no file is read" case: with
// LAB_GAME_BALANCE_PATH itself unset, Load never learns which file to
// read, so the joined error is exactly the one missing-variable error —
// not also a follow-on file-read failure.
func TestLoad_EnvironmentFailureNoFileRead(t *testing.T) {
	t.Parallel()
	env := validConfigEnv(t)
	delete(env, envBalancePath)

	cfg, err := Load(mapLookup(env))
	if cfg != nil {
		t.Errorf("Load: expected a nil *Config on error, got %+v", cfg)
	}
	assertKeyError(t, err, ErrMissing, envBalancePath)
	if strings.Count(err.Error(), "\n") > 0 && strings.Contains(err.Error(), "no such file") {
		t.Errorf("Load: error mentions a file-read attempt that should never have happened: %v", err)
	}
}

// TestLoad_WorldAndBalanceBothFail asserts that a bad world path and a bad
// balance file are reported together in one joined error.
func TestLoad_WorldAndBalanceBothFail(t *testing.T) {
	t.Parallel()
	env := validConfigEnv(t)
	env[envWorldPath] += "/does-not-exist"
	env[envBalancePath] = writeBalanceFile(t, strings.Replace(validBalanceYAML, "    cap: 100\n", "", 1))

	_, err := Load(mapLookup(env))
	if err == nil {
		t.Fatal("Load: expected an error")
	}
	if !containsKeyError(err, envWorldPath) {
		t.Errorf("Load: error does not name %s: %v", envWorldPath, err)
	}
	if !containsKeyError(err, "raid.stamina.cap") {
		t.Errorf("Load: error does not name raid.stamina.cap: %v", err)
	}
}

func TestSecret_Redaction(t *testing.T) {
	t.Parallel()
	s := Secret("super-secret-value")

	if got := fmt.Sprintf("%v", s); got != redacted {
		t.Errorf("%%v = %q, want %q", got, redacted)
	}
	if got := fmt.Sprintf("%s", s); got != redacted { //nolint:staticcheck // S1025: deliberately exercising the %s verb through fmt, not calling String() directly
		t.Errorf("%%s = %q, want %q", got, redacted)
	}
	if got := fmt.Sprintf("%#v", s); got != redacted {
		t.Errorf("%%#v = %q, want %q", got, redacted)
	}
	if got := s.Reveal(); got != "super-secret-value" {
		t.Errorf("Reveal() = %q, want the underlying value", got)
	}
}
