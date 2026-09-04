package config

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// recordingLookup wraps base, recording every key ever queried through it
// (deduplicated, first-seen order — not that order matters here, since the
// assertions below compare sorted sets). This is AC16's "disjoint in fact,
// not only in prose": the recorded set is what the loader actually
// consulted, not what a reader believes it consults.
func recordingLookup(base Lookup) (lookup Lookup, recorded func() []string) {
	seen := map[string]bool{}
	var keys []string
	return func(key string) (string, bool) {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
			return base(key)
		}, func() []string {
			out := make([]string, len(keys))
			copy(out, keys)
			return out
		}
}

// readEnvExampleKeys parses .env.example with godotenv (never a hand-split
// — .env quoting, comments and export prefixes are exactly where a naive
// splitter goes wrong, design D13(b)) and returns its keys and values.
func readEnvExampleKeys(t *testing.T) map[string]string {
	t.Helper()
	m, err := godotenv.Read(repoRootPath(t, ".env.example"))
	if err != nil {
		t.Fatalf("godotenv.Read(.env.example): %v", err)
	}
	return m
}

func sortedCopy(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}

// assertSameKeySet fails with a two-direction message — present in got,
// absent from want, and vice versa — naming both, per D13(b): the failure
// must explain itself rather than reading as a mysterious regression.
func assertSameKeySet(t *testing.T, gotName string, got []string, wantName string, want []string) {
	t.Helper()
	gotSet := map[string]bool{}
	for _, k := range got {
		gotSet[k] = true
	}
	wantSet := map[string]bool{}
	for _, k := range want {
		wantSet[k] = true
	}

	var onlyInGot, onlyInWant []string
	for _, k := range sortedCopy(got) {
		if !wantSet[k] {
			onlyInGot = append(onlyInGot, k)
		}
	}
	for _, k := range sortedCopy(want) {
		if !gotSet[k] {
			onlyInWant = append(onlyInWant, k)
		}
	}
	if len(onlyInGot) > 0 || len(onlyInWant) > 0 {
		t.Errorf("%s and %s disagree.\nin %s, not in %s: %v\nin %s, not in %s: %v\nSee .env.example's header comment.",
			gotName, wantName,
			gotName, wantName, onlyInGot,
			wantName, gotName, onlyInWant,
		)
	}
}

// TestEnvExample_MatchesLoaderAndEnvKeys is AC8+AC16's literal conjunction:
// .env.example's key set, the loader's actually-consulted key set, and
// EnvKeys() are all identical.
func TestEnvExample_MatchesLoaderAndEnvKeys(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	var exampleKeys []string
	for k := range example {
		exampleKeys = append(exampleKeys, k)
	}

	env := rewriteExamplePaths(t, example)
	lookup, recorded := recordingLookup(mapLookup(env))
	if _, err := Load(lookup); err != nil {
		t.Fatalf("Load with the example environment: unexpected error: %v", err)
	}

	assertSameKeySet(t, ".env.example", exampleKeys, "config.EnvKeys()", EnvKeys())
	assertSameKeySet(t, "loader-consulted keys", recorded(), "config.EnvKeys()", EnvKeys())
}

// TestEnvExample_ValuesAreNonEmpty is AC8's placeholder requirement.
func TestEnvExample_ValuesAreNonEmpty(t *testing.T) {
	t.Parallel()
	for k, v := range readEnvExampleKeys(t) {
		if strings.TrimSpace(v) == "" {
			t.Errorf(".env.example: %s has an empty value", k)
		}
	}
}

// rewriteExamplePaths returns a copy of example with LAB_GAME_BALANCE_PATH
// and LAB_GAME_WORLD_PATH rewritten from repo-root-relative to absolute,
// since .env.example documents repo-root-relative paths but a test may run
// from any working directory (design § Risks).
func rewriteExamplePaths(t *testing.T, example map[string]string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(example))
	for k, v := range example {
		out[k] = v
	}
	out[envBalancePath] = repoRootPath(t, filepath.ToSlash(example[envBalancePath]))
	out[envWorldPath] = repoRootPath(t, filepath.ToSlash(example[envWorldPath]))
	return out
}

// TestLoad_ExampleEnvironmentSucceeds is AC7's second sentence: the tracked
// balance file loads under the example environment.
func TestLoad_ExampleEnvironmentSucceeds(t *testing.T) {
	t.Parallel()
	env := rewriteExamplePaths(t, readEnvExampleKeys(t))
	if _, err := Load(mapLookup(env)); err != nil {
		t.Fatalf("Load with the example environment: unexpected error: %v", err)
	}
}

// TestLoad_BalanceIndependentOfOtherVariables is AC16's second clause: the
// tracked balance file, loaded under differing token/DSN/base-URL/chat-id
// environments, yields Balance values that compare equal — the balance
// file's content depends on nothing but LAB_GAME_BALANCE_PATH.
func TestLoad_BalanceIndependentOfOtherVariables(t *testing.T) {
	t.Parallel()
	env1 := rewriteExamplePaths(t, readEnvExampleKeys(t))
	env2 := rewriteExamplePaths(t, readEnvExampleKeys(t))
	env2[envBotToken] = "a-completely-different-token"
	env2[envDSN] = "postgres://other:other@otherhost:5432/otherdb"
	env2[envBotAPIBaseURL] = "https://other.example.com"
	env2[envAllowedChatIDs] = "1,2,3"

	cfg1, err := Load(mapLookup(env1))
	if err != nil {
		t.Fatalf("Load(env1): %v", err)
	}
	cfg2, err := Load(mapLookup(env2))
	if err != nil {
		t.Fatalf("Load(env2): %v", err)
	}
	assertBalanceEqual(t, &cfg1.Balance, &cfg2.Balance)
}
