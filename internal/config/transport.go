package config

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Environment variable names for the Bot API transport's retry and
// rate-limit tuning (design D10). Unlike envKeys' six variables, every one
// of these is optional: an absent value takes the compiled-in default
// named alongside it below, and loadTransport is the dedicated reader that
// validates them — mirroring envBalancePath/envWorldPath's shape, not
// envKeys' (design D10, § "must NOT be implemented by adding the
// transport keys to envKeys()").
const (
	envTGRetryMaxAttempts = "LAB_GAME_TG_RETRY_MAX_ATTEMPTS"
	envTGRetryBaseDelay   = "LAB_GAME_TG_RETRY_BASE_DELAY"
	envTGRetryMaxDelay    = "LAB_GAME_TG_RETRY_MAX_DELAY"
	envTGAttemptTimeout   = "LAB_GAME_TG_ATTEMPT_TIMEOUT"

	envTGLimitMessageGlobal   = "LAB_GAME_TG_LIMIT_MESSAGE_GLOBAL"
	envTGLimitMessageChatRate = "LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE"
	envTGLimitMessageChatCap  = "LAB_GAME_TG_LIMIT_MESSAGE_CHAT_CAP"

	envTGLimitEditGlobal   = "LAB_GAME_TG_LIMIT_EDIT_GLOBAL"
	envTGLimitEditChatRate = "LAB_GAME_TG_LIMIT_EDIT_CHAT_RATE"
	envTGLimitEditChatCap  = "LAB_GAME_TG_LIMIT_EDIT_CHAT_CAP"

	envTGLimitOtherGlobal   = "LAB_GAME_TG_LIMIT_OTHER_GLOBAL"
	envTGLimitOtherChatRate = "LAB_GAME_TG_LIMIT_OTHER_CHAT_RATE"
	envTGLimitOtherChatCap  = "LAB_GAME_TG_LIMIT_OTHER_CHAT_CAP"
)

// rateOff is the "off" literal a rate-limit value may spell, meaning
// unbounded — Rate's zero value (design D10).
const rateOff = "off"

// transportEnvKeys returns the thirteen transport tuning variables, in
// declaration order (deterministic — AGENTS.md § Code Style). It is
// appended to EnvKeys() only; envKeys() itself is untouched, because
// env_test.go's TestLoadEnv_RequiredVariableUnset/Empty iterate envKeys()
// and assert every member is required (design D10).
func transportEnvKeys() []string {
	return []string{
		envTGRetryMaxAttempts,
		envTGRetryBaseDelay,
		envTGRetryMaxDelay,
		envTGAttemptTimeout,
		envTGLimitMessageGlobal,
		envTGLimitMessageChatRate,
		envTGLimitMessageChatCap,
		envTGLimitEditGlobal,
		envTGLimitEditChatRate,
		envTGLimitEditChatCap,
		envTGLimitOtherGlobal,
		envTGLimitOtherChatRate,
		envTGLimitOtherChatCap,
	}
}

// Rate is one window constraint: at most Count emissions in any interval
// of length Per. The zero value means unbounded — a class configured "off"
// contributes no window to the limiter (design D9 § "An unbounded value
// contributes no window").
type Rate struct {
	Count int
	Per   time.Duration
}

// ClassLimits holds the three windows a bounded method class may carry:
// Global (the class-wide ceiling, shared across every chat), ChatRate (the
// short-window steady-emission rate into one chat) and ChatCap (the
// longer-window cap into one chat). Each is independently boundable or
// unbounded (design D9 § "Configuration maps onto windows").
type ClassLimits struct {
	Global   Rate
	ChatRate Rate
	ChatCap  Rate
}

// TransportLimits holds one ClassLimits per Bot API method class — Message
// (send/copy/forward), Edit (edit/delete) and Other (everything else) —
// matching internal/tg's MethodClass enum (design D4, D10).
type TransportLimits struct {
	Message ClassLimits
	Edit    ClassLimits
	Other   ClassLimits
}

// Transport holds internal/tg's retry and rate-limit tuning: how many
// times a retryable failure is attempted, how the backoff between attempts
// grows, how long a single HTTP attempt may run, and the window
// constraints each method class's global and per-chat schedules carry.
// Every field is optional-with-default (design D10) — unlike Config's
// other fields, an absent environment variable does not fail Load.
type Transport struct {
	// RetryMaxAttempts is the maximum number of attempts one outbound call
	// makes before giving up (LAB_GAME_TG_RETRY_MAX_ATTEMPTS, default 3).
	RetryMaxAttempts int
	// RetryBaseDelay is the backoff scale's base duration
	// (LAB_GAME_TG_RETRY_BASE_DELAY, default 500ms) — design D6's d_0.
	RetryBaseDelay time.Duration
	// RetryMaxDelay caps the backoff scale's doubling
	// (LAB_GAME_TG_RETRY_MAX_DELAY, default 30s) — design D6's max.
	RetryMaxDelay time.Duration
	// AttemptTimeout bounds a single HTTP attempt
	// (LAB_GAME_TG_ATTEMPT_TIMEOUT, default 30s); the caller's own context
	// remains the bound on the whole call (design D10).
	AttemptTimeout time.Duration
	// Limits holds the per-class window constraints (design D9, D10).
	Limits TransportLimits
}

// defaultTransport returns the compiled-in defaults every transport key
// falls back to when its environment variable is absent (design D10's
// table). The message-class defaults are traceable to core.telegram.org's
// FAQ ("avoid sending more than one message per second", "not able to
// send more than 20 messages per minute", "not able to broadcast more
// than about 30 messages per second") and to docs/DESIGN.md §11; the
// retry defaults and AttemptTimeout are chosen, not sourced (design D10).
func defaultTransport() Transport {
	return Transport{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   500 * time.Millisecond,
		RetryMaxDelay:    30 * time.Second,
		AttemptTimeout:   30 * time.Second,
		Limits: TransportLimits{
			Message: ClassLimits{
				Global:   Rate{Count: 30, Per: time.Second},
				ChatRate: Rate{Count: 1, Per: time.Second},
				ChatCap:  Rate{Count: 20, Per: time.Minute},
			},
			// Edit and Other stay unbounded by default: no published
			// figure supports a bound (design D10).
		},
	}
}

// loadTransport reads and validates every LAB_GAME_TG_ variable through
// lookup, querying each one unconditionally regardless of any other
// key's presence (required so the disjointness test's recordingLookup
// still records an absent optional key — design D10). An absent variable
// takes its compiled-in default; a present, malformed value is reported as
// a *KeyError naming that variable. Every failure is collected and
// returned at once via errors.Join, in declaration order.
func loadTransport(lookup Lookup) (*Transport, error) {
	var errs []error
	t := defaultTransport()

	if n, ok, err := lookupPositiveInt(lookup, envTGRetryMaxAttempts); err != nil {
		errs = append(errs, err)
	} else if ok {
		t.RetryMaxAttempts = n
	}

	if d, ok, err := lookupPositiveDuration(lookup, envTGRetryBaseDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		t.RetryBaseDelay = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envTGRetryMaxDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		t.RetryMaxDelay = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envTGAttemptTimeout); err != nil {
		errs = append(errs, err)
	} else if ok {
		t.AttemptTimeout = d
	}

	loadClass := func(global, chatRate, chatCap string, cl *ClassLimits) {
		if r, ok, err := lookupRate(lookup, global); err != nil {
			errs = append(errs, err)
		} else if ok {
			cl.Global = r
		}
		if r, ok, err := lookupRate(lookup, chatRate); err != nil {
			errs = append(errs, err)
		} else if ok {
			cl.ChatRate = r
		}
		if r, ok, err := lookupRate(lookup, chatCap); err != nil {
			errs = append(errs, err)
		} else if ok {
			cl.ChatCap = r
		}
	}

	loadClass(envTGLimitMessageGlobal, envTGLimitMessageChatRate, envTGLimitMessageChatCap, &t.Limits.Message)
	loadClass(envTGLimitEditGlobal, envTGLimitEditChatRate, envTGLimitEditChatCap, &t.Limits.Edit)
	loadClass(envTGLimitOtherGlobal, envTGLimitOtherChatRate, envTGLimitOtherChatCap, &t.Limits.Other)

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &t, nil
}

// lookupPositiveInt queries key through lookup, returning (0, false, nil)
// when absent, the parsed value and true when present and a strictly
// positive integer, or a *KeyError when present and not.
func lookupPositiveInt(lookup Lookup, key string) (int, bool, error) {
	val, ok := lookup(key)
	if !ok {
		return 0, false, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || n <= 0 {
		return 0, false, keyErrorf(key, ErrInvalidValue, "must be a positive integer, got %q", val)
	}
	return n, true, nil
}

// lookupPositiveDuration queries key through lookup, returning
// (0, false, nil) when absent, the parsed value and true when present and
// a strictly positive time.ParseDuration string, or a *KeyError when
// present and not.
func lookupPositiveDuration(lookup Lookup, key string) (time.Duration, bool, error) {
	val, ok := lookup(key)
	if !ok {
		return 0, false, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(val))
	if err != nil || d <= 0 {
		return 0, false, keyErrorf(key, ErrInvalidValue, "must be a positive duration, got %q", val)
	}
	return d, true, nil
}

// lookupRate queries key through lookup, returning (Rate{}, false, nil)
// when absent, the parsed Rate and true when present and either the
// literal "off" (Rate{}, unbounded) or "<count>/<duration>" with both
// parts positive, or a *KeyError when present and neither (design D10's
// grammar).
func lookupRate(lookup Lookup, key string) (Rate, bool, error) {
	val, ok := lookup(key)
	if !ok {
		return Rate{}, false, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == rateOff {
		return Rate{}, true, nil
	}
	countStr, perStr, found := strings.Cut(trimmed, "/")
	if !found {
		return Rate{}, false, keyErrorf(key, ErrInvalidValue, "must be %q or <count>/<duration>, got %q", rateOff, val)
	}
	count, err := strconv.Atoi(strings.TrimSpace(countStr))
	if err != nil || count <= 0 {
		return Rate{}, false, keyErrorf(key, ErrInvalidValue, "count must be a positive integer, got %q", val)
	}
	per, err := time.ParseDuration(strings.TrimSpace(perStr))
	if err != nil || per <= 0 {
		return Rate{}, false, keyErrorf(key, ErrInvalidValue, "duration must be positive, got %q", val)
	}
	return Rate{Count: count, Per: per}, true, nil
}
