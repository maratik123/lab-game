package config

import (
	"errors"
	"strings"
	"testing"
)

// mapLookup builds a Lookup backed by m, so no test touches the process
// environment (design D1).
func mapLookup(m map[string]string) Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// validEnv returns a fresh map with every required variable set to a value
// that passes validation, so each negative test case is one documented
// mutation of it.
func validEnv() map[string]string {
	return map[string]string{
		envBotToken:       "test-token",
		envDSN:            "postgres://user:pass@localhost/db",
		envBotAPIBaseURL:  "https://api.telegram.org",
		envAllowedChatIDs: "-100123456789,42",
		envBalancePath:    "/tmp/balance.yaml",
		envWorldPath:      "/tmp/world",
	}
}

func TestLoadEnv_HappyPath(t *testing.T) {
	t.Parallel()
	v, err := loadEnv(mapLookup(validEnv()))
	if err != nil {
		t.Fatalf("loadEnv: unexpected error: %v", err)
	}
	if v.BotToken != "test-token" {
		t.Errorf("BotToken = %q", v.BotToken)
	}
	if v.DSN != "postgres://user:pass@localhost/db" {
		t.Errorf("DSN = %q", v.DSN)
	}
	if v.BotAPIBaseURL.String() != "https://api.telegram.org" {
		t.Errorf("BotAPIBaseURL = %q", v.BotAPIBaseURL.String())
	}
	wantIDs := []int64{-100123456789, 42}
	if len(v.AllowedChatIDs) != len(wantIDs) || v.AllowedChatIDs[0] != wantIDs[0] || v.AllowedChatIDs[1] != wantIDs[1] {
		t.Errorf("AllowedChatIDs = %v, want %v (order preserved)", v.AllowedChatIDs, wantIDs)
	}
}

func TestLoadEnv_RequiredVariableUnset(t *testing.T) {
	t.Parallel()
	for _, key := range envKeys() {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			delete(env, key)
			_, err := loadEnv(mapLookup(env))
			assertKeyError(t, err, ErrMissing, key)
		})
	}
}

func TestLoadEnv_RequiredVariableEmpty(t *testing.T) {
	t.Parallel()
	for _, key := range envKeys() {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			env[key] = ""
			_, err := loadEnv(mapLookup(env))
			assertKeyError(t, err, ErrInvalidValue, key)
		})
	}
}

func TestLoadEnv_BotAPIBaseURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value string
	}{
		{"relative", "/bot123"},
		{"no_host", "https:///path"},
		{"non_http_scheme", "ftp://api.telegram.org"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			env[envBotAPIBaseURL] = tc.value
			_, err := loadEnv(mapLookup(env))
			assertKeyError(t, err, ErrInvalidValue, envBotAPIBaseURL)
		})
	}
}

func TestLoadEnv_AllowedChatIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value string
	}{
		{"non_integer_element", "abc,42"},
		{"empty_element", "1,,2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := validEnv()
			env[envAllowedChatIDs] = tc.value
			_, err := loadEnv(mapLookup(env))
			assertKeyError(t, err, ErrInvalidValue, envAllowedChatIDs)
		})
	}
}

// TestLoadEnv_Aggregation asserts AC12: several variables unset at once
// report all of them, in the same (declaration) order on repeated runs —
// a determinism assertion, not an incidental one.
func TestLoadEnv_Aggregation(t *testing.T) {
	t.Parallel()
	env := validEnv()
	delete(env, envBotToken)
	delete(env, envDSN)

	_, err1 := loadEnv(mapLookup(env))
	_, err2 := loadEnv(mapLookup(env))
	if err1 == nil || err2 == nil {
		t.Fatal("loadEnv: expected an error")
	}
	if err1.Error() != err2.Error() {
		t.Fatalf("loadEnv is not deterministic:\n%v\n%v", err1, err2)
	}
	if !errors.Is(err1, ErrMissing) {
		t.Errorf("errors.Is(err, ErrMissing) = false; err = %v", err1)
	}
	if !strings.Contains(err1.Error(), envBotToken) || !strings.Contains(err1.Error(), envDSN) {
		t.Errorf("joined error %q does not name both missing variables", err1)
	}
}

// TestEnvKeys_IncludesPathVariables asserts that EnvKeys (the full declared
// set AC8/AC16 check against) carries the balance-file and world-set
// variables even though loadEnv does not itself validate them.
func TestEnvKeys_IncludesPathVariables(t *testing.T) {
	t.Parallel()
	keys := EnvKeys()
	for _, want := range []string{envBalancePath, envWorldPath} {
		found := false
		for _, k := range keys {
			if k == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("EnvKeys() = %v, missing %q", keys, want)
		}
	}
}
