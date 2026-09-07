package config

import (
	"errors"
	"net/url"
)

// redacted is what Secret renders as under %v, %s and %#v.
const redacted = "[redacted]"

// Secret wraps a value whose accidental leak is an incident — the bot
// token and the database DSN (AGENTS.md § Permissions: a leaked token is
// rotated through BotFather, not edited out of history). It is the one
// place both secrets sit together in a single struct, so a future %v of it
// is the cheapest leak path; String, GoString and the %q spelling below
// make that mistake harder, not impossible — %q still prints the
// underlying string, so this is a guard-rail, not a guarantee.
type Secret string

// String renders s as "[redacted]", so %v and %s never print the secret.
func (s Secret) String() string {
	return redacted
}

// GoString renders s as "[redacted]" under %#v.
func (s Secret) GoString() string {
	return redacted
}

// Reveal returns the underlying secret value.
func (s Secret) Reveal() string {
	return string(s)
}

// Config is lab-game's fully validated runtime configuration: the process
// environment's secrets, runtime settings and file paths, plus the balance
// file's decoded constants. Every field is populated by Load or Load
// returns an error naming every rejected key. Transport and Scheduler are
// the two exceptions: their fields are individually optional-with-default,
// so an absent LAB_GAME_TG_ or LAB_GAME_SCHEDULER_ variable never fails
// Load — every other field has no compiled-in fallback (design D10, D13).
// Treat the returned value as read-only; Config is not defended against
// mutation by the type system.
type Config struct {
	// BotToken is the Telegram bot token (LAB_GAME_BOT_TOKEN).
	BotToken Secret
	// DSN is the Postgres connection string (LAB_GAME_DSN). Not parsed
	// here — internal/store already owns DSN parsing.
	DSN Secret
	// BotAPIBaseURL is the self-hosted telegram-bot-api instance's base
	// URL (LAB_GAME_BOT_API_BASE_URL): absolute, scheme http or https,
	// non-empty host.
	BotAPIBaseURL url.URL
	// AllowedChatIDs is the ordered set of chat ids the bot is allowed to
	// write to (LAB_GAME_ALLOWED_CHAT_IDS), in the order the operator
	// wrote them.
	AllowedChatIDs []int64
	// WorldPath is the world-set target (LAB_GAME_WORLD_PATH) — the path
	// only. This package declares no type describing the world set's
	// interior (AC15); nothing inside it is read here.
	WorldPath string
	// Balance holds every game constant, decoded from LAB_GAME_BALANCE_PATH.
	Balance Balance
	// Transport holds internal/tg's retry and rate-limit tuning
	// (LAB_GAME_TG_*), each field optional-with-default (design D10).
	Transport Transport
	// Scheduler holds internal/scheduler's polling, claim-batch and retry
	// tuning (LAB_GAME_SCHEDULER_*), each field optional-with-default
	// (design D13).
	Scheduler Scheduler
	// Ingest holds internal/ingest's long-poll, batch and retry tuning
	// (LAB_GAME_INGEST_*), each field optional-with-default (design D15).
	Ingest Ingest
}

// Load reads and validates lab-game's whole configuration through lookup —
// the process environment (secrets, runtime settings, the balance-file and
// world-set paths), the balance YAML file, and the world-set path's
// readability — and returns a populated *Config, or an error naming every
// rejected or missing key. All three sources are validated independently
// against the same lookup and their failures are joined (errors.Join), so
// one call reports every problem rather than the first.
//
// Configuration is read once, at start-up: there is no reload path and no
// mechanism to pick up a changed balance file or environment variable
// without restarting the process.
//
// Every returned error wraps one or more *KeyError, each naming the
// environment variable or dotted balance-file path it belongs to and
// classifying the failure as ErrMissing (required key absent or empty),
// ErrInvalidValue (present but the wrong shape, tag, or failing its
// predicate), ErrUnknownKey (a balance key the schema does not declare), or
// ErrUnreadable (a path named by configuration that cannot be opened).
// Compare with errors.Is.
func Load(lookup Lookup) (*Config, error) {
	var errs []error

	env, err := loadEnv(lookup)
	if err != nil {
		errs = append(errs, err)
	}

	transport, err := loadTransport(lookup)
	if err != nil {
		errs = append(errs, err)
	}

	scheduler, err := loadScheduler(lookup)
	if err != nil {
		errs = append(errs, err)
	}

	ingest, err := loadIngest(lookup)
	if err != nil {
		errs = append(errs, err)
	}

	// D16's pair of cross-checks needs both Ingest and Transport, so it
	// runs here, once each has loaded successfully on its own — a
	// malformed LongPollTimeout or a malformed AttemptTimeout is already
	// reported above, and this check would otherwise report a second,
	// confusing error about a value that never validated in the first
	// place.
	if ingest != nil && transport != nil {
		if err := checkIngestLongPollTimeout(*ingest, *transport); err != nil {
			errs = append(errs, err)
		}
	}

	worldPath, err := resolveWorldPath(lookup)
	if err != nil {
		errs = append(errs, err)
	}

	var balance *Balance
	if balancePath, err := requiredBalancePath(lookup); err != nil {
		errs = append(errs, err)
	} else if b, err := loadBalance(balancePath); err != nil {
		errs = append(errs, err)
	} else {
		balance = b
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &Config{
		BotToken:       Secret(env.BotToken),
		DSN:            Secret(env.DSN),
		BotAPIBaseURL:  env.BotAPIBaseURL,
		AllowedChatIDs: env.AllowedChatIDs,
		WorldPath:      worldPath,
		Balance:        *balance,
		Transport:      *transport,
		Scheduler:      *scheduler,
		Ingest:         *ingest,
	}, nil
}

// requiredBalancePath validates LAB_GAME_BALANCE_PATH's presence and
// non-emptiness — the same shape check every other required variable gets
// — leaving the richer validation (the file opens, parses and matches the
// schema) to loadBalance.
func requiredBalancePath(lookup Lookup) (string, error) {
	val, ok := lookup(envBalancePath)
	if !ok {
		return "", keyErrorf(envBalancePath, ErrMissing, "required")
	}
	if val == "" {
		return "", keyErrorf(envBalancePath, ErrInvalidValue, "must not be empty")
	}
	return val, nil
}
