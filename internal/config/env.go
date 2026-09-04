package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Environment variable names, LAB_GAME_ prefixed to match the existing
// LAB_GAME_TEST_DSN (internal/testdb). Every variable declared here is
// required and none has a compiled-in default; the Bot API transport's
// tuning variables (transport.go) are a separate, optional-with-default
// class, added on top by EnvKeys() (design D10).
const (
	envBotToken = "LAB_GAME_BOT_TOKEN" //nolint:gosec // G101: this is an environment-variable NAME, not a credential value

	envDSN            = "LAB_GAME_DSN"
	envBotAPIBaseURL  = "LAB_GAME_BOT_API_BASE_URL"
	envAllowedChatIDs = "LAB_GAME_ALLOWED_CHAT_IDS"
	envBalancePath    = "LAB_GAME_BALANCE_PATH"
	envWorldPath      = "LAB_GAME_WORLD_PATH"
)

// envKeys returns the four variables loadEnv itself validates. envBalancePath
// and envWorldPath are validated by their own dedicated readers instead
// (world.go's resolveWorldPath and config.go's Load) — each owns both the
// presence check and the richer validation for its path (open/close for
// the world set, YAML decode for the balance file), so no variable's
// presence is checked in two places (design § Decomposition, subtasks
// 3-5).
func envKeys() []string {
	return []string{envBotToken, envDSN, envBotAPIBaseURL, envAllowedChatIDs}
}

// Lookup retrieves one environment variable's value, reporting whether it
// was present at all — distinct from present-but-empty, which AC2 and AC10
// treat differently (design D1). cmd/bot passes os.LookupEnv; tests pass a
// map-backed or recording implementation, so no test touches the process
// environment and every case may run under t.Parallel().
type Lookup func(key string) (value string, ok bool)

// EnvKeys returns config's full declared environment-variable set, freshly
// built on every call (AC1 — no package-level mutable state). It is the
// set AC16's "consults no environment variable outside the documented set"
// is checked against, and the set .env.example is asserted to equal
// exactly (AC8). transportEnvKeys() — the optional-with-default Bot API
// transport tuning keys — is appended alongside envBalancePath and
// envWorldPath, each validated by its own dedicated reader rather than by
// loadEnv (design D10).
func EnvKeys() []string {
	keys := append(envKeys(), envBalancePath, envWorldPath)
	return append(keys, transportEnvKeys()...)
}

// envValues holds the environment layer's validated results: the two
// secrets as plain strings (config.go's Load wraps them in Secret) and the
// parsed Bot API base URL and chat-id list. The balance-file and world-set
// paths are validated by their own dedicated readers, not by loadEnv (see
// envKeys).
type envValues struct {
	BotToken       string
	DSN            string
	BotAPIBaseURL  url.URL
	AllowedChatIDs []int64
}

// loadEnv reads and validates every LAB_GAME_ environment variable through
// lookup, returning every failure at once as a joined *KeyError, each
// naming its variable, in declaration order (deterministic — AGENTS.md
// § Code Style — never map-iteration order).
func loadEnv(lookup Lookup) (*envValues, error) {
	var errs []error
	v := &envValues{}

	required := func(key string) (string, bool) {
		val, ok := lookup(key)
		if !ok {
			errs = append(errs, keyErrorf(key, ErrMissing, "required"))
			return "", false
		}
		if val == "" {
			errs = append(errs, keyErrorf(key, ErrInvalidValue, "must not be empty"))
			return "", false
		}
		return val, true
	}

	if s, ok := required(envBotToken); ok {
		v.BotToken = s
	}
	if s, ok := required(envDSN); ok {
		v.DSN = s
	}
	if s, ok := required(envBotAPIBaseURL); ok {
		u, err := parseBotAPIBaseURL(s)
		if err != nil {
			errs = append(errs, keyErrorf(envBotAPIBaseURL, ErrInvalidValue, "%s", err))
		} else {
			v.BotAPIBaseURL = *u
		}
	}
	if s, ok := required(envAllowedChatIDs); ok {
		ids, err := parseAllowedChatIDs(s)
		if err != nil {
			errs = append(errs, keyErrorf(envAllowedChatIDs, ErrInvalidValue, "%s", err))
		} else {
			v.AllowedChatIDs = ids
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return v, nil
}

// parseBotAPIBaseURL validates s as an absolute http(s) URL with a
// non-empty host (AC9). net/url.Parse alone is not enough: it is documented
// as tolerant of a scheme-less host/path, and URL.IsAbs asserts only a
// non-empty scheme — so Parse, IsAbs, an explicit scheme check and a host
// check are all four required (design D4).
func parseBotAPIBaseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if !u.IsAbs() {
		return nil, errors.New("must be an absolute URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("must have a non-empty host")
	}
	return u, nil
}

// parseAllowedChatIDs splits s on commas and parses every element as an
// int64 (Telegram group ids are negative, AC10), preserving the file's
// order — the returned slice's order is what makes AllowedChatIDs
// deterministic (design D5).
func parseAllowedChatIDs(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("element %d is empty", i)
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("element %d (%q) is not an integer: %w", i, p, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
