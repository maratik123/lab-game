package config

import (
	"errors"
	"time"
)

// Environment variable names for internal/ingest's long-poll, batch and
// retry tuning (design D15, issue #22). Every one of these is optional:
// an absent value takes the compiled-in default named alongside it below
// — the same optional-with-default class Transport and Scheduler already
// established.
const (
	envIngestPollInterval     = "LAB_GAME_INGEST_POLL_INTERVAL"
	envIngestLongPollTimeout  = "LAB_GAME_INGEST_LONG_POLL_TIMEOUT"
	envIngestBatchLimit       = "LAB_GAME_INGEST_BATCH_LIMIT"
	envIngestRetryMaxAttempts = "LAB_GAME_INGEST_RETRY_MAX_ATTEMPTS"
	envIngestRetryBaseDelay   = "LAB_GAME_INGEST_RETRY_BASE_DELAY"
	envIngestRetryMaxDelay    = "LAB_GAME_INGEST_RETRY_MAX_DELAY"
)

// ingestEnvKeys returns the six ingest tuning variables, in declaration
// order (deterministic — AGENTS.md § Code Style). It is appended to
// EnvKeys() only; envKeys() itself is untouched, mirroring
// transportEnvKeys' and schedulerEnvKeys' rule (design D15).
func ingestEnvKeys() []string {
	return []string{
		envIngestPollInterval,
		envIngestLongPollTimeout,
		envIngestBatchLimit,
		envIngestRetryMaxAttempts,
		envIngestRetryBaseDelay,
		envIngestRetryMaxDelay,
	}
}

// ingestBatchLimitMax is the Bot API's own stated ceiling on
// GetUpdatesParams.Limit ("Values between 1-100 are accepted", design
// D15).
const ingestBatchLimitMax = 100

// Ingest holds internal/ingest's long-poll, batch and retry tuning. Every
// field is optional-with-default (design D15) — like Transport and
// Scheduler, an absent LAB_GAME_INGEST_ variable never fails Load. Every
// default here is chosen operational tuning, never a balance number.
type Ingest struct {
	// PollInterval is the wait between poll cycles, and — per design
	// D19 — the whole bound on the retry rate against a failing Bot API
	// (LAB_GAME_INGEST_POLL_INTERVAL, default 1s).
	PollInterval time.Duration
	// LongPollTimeout is the long-poll window transmitted on every
	// getUpdates call (LAB_GAME_INGEST_LONG_POLL_TIMEOUT, default 25s).
	// Load additionally requires it strictly below Transport.AttemptTimeout
	// and a whole number of seconds (design D16).
	LongPollTimeout time.Duration
	// BatchLimit is GetUpdatesParams.Limit, the Bot API's own accepted
	// range being 1-100 (LAB_GAME_INGEST_BATCH_LIMIT, default 100).
	BatchLimit int
	// RetryMaxAttempts is the maximum number of attempts one update's
	// handler is given before it is given up on
	// (LAB_GAME_INGEST_RETRY_MAX_ATTEMPTS, default 5).
	RetryMaxAttempts int
	// RetryBaseDelay is the backoff scale's base duration, consumed
	// through backoff.Exponential (LAB_GAME_INGEST_RETRY_BASE_DELAY,
	// default 1s).
	RetryBaseDelay time.Duration
	// RetryMaxDelay caps the backoff scale's doubling
	// (LAB_GAME_INGEST_RETRY_MAX_DELAY, default 8s).
	RetryMaxDelay time.Duration
}

// defaultIngest returns the compiled-in defaults every ingest key falls
// back to when its environment variable is absent (design D15's table).
// Under the default retry cap and ramp the worst-case head-of-line stall
// a poisoned update imposes on the sequential loop is
// 1s + 2s + 4s + 8s = 15s, the quarter-minute bound spec Scope 7 names.
func defaultIngest() Ingest {
	return Ingest{
		PollInterval:     time.Second,
		LongPollTimeout:  25 * time.Second,
		BatchLimit:       100,
		RetryMaxAttempts: 5,
		RetryBaseDelay:   time.Second,
		RetryMaxDelay:    8 * time.Second,
	}
}

// loadIngest reads and validates every LAB_GAME_INGEST_ variable through
// lookup, querying each one unconditionally regardless of any other key's
// presence (required so the disjointness test's recording lookup still
// records an absent optional key — mirrors loadTransport and
// loadScheduler). An absent variable takes its compiled-in default; a
// present, malformed value is reported as a *KeyError naming that
// variable. Every failure is collected and returned at once via
// errors.Join, in declaration order.
//
// loadIngest validates BatchLimit against the Bot API's own accepted
// range (design D15); it does NOT validate LongPollTimeout against
// Transport.AttemptTimeout or against whole-seconds-ness — both of those
// cross-checks need Transport too, so they live in Load, beside each
// other (design D16).
func loadIngest(lookup Lookup) (*Ingest, error) {
	var errs []error
	i := defaultIngest()

	if d, ok, err := lookupPositiveDuration(lookup, envIngestPollInterval); err != nil {
		errs = append(errs, err)
	} else if ok {
		i.PollInterval = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envIngestLongPollTimeout); err != nil {
		errs = append(errs, err)
	} else if ok {
		i.LongPollTimeout = d
	}

	if n, ok, err := lookupPositiveInt(lookup, envIngestBatchLimit); err != nil {
		errs = append(errs, err)
	} else if ok {
		if n > ingestBatchLimitMax {
			errs = append(errs, keyErrorf(envIngestBatchLimit, ErrInvalidValue, "must be between 1 and %d, got %d", ingestBatchLimitMax, n))
		} else {
			i.BatchLimit = n
		}
	}

	if n, ok, err := lookupPositiveInt(lookup, envIngestRetryMaxAttempts); err != nil {
		errs = append(errs, err)
	} else if ok {
		i.RetryMaxAttempts = n
	}

	if d, ok, err := lookupPositiveDuration(lookup, envIngestRetryBaseDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		i.RetryBaseDelay = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envIngestRetryMaxDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		i.RetryMaxDelay = d
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &i, nil
}

// checkIngestLongPollTimeout is design D16's pair of cross-checks on
// LAB_GAME_INGEST_LONG_POLL_TIMEOUT, run from Load once both Ingest and
// Transport have loaded successfully: the long-poll window must be
// strictly below Transport.AttemptTimeout (otherwise every poll that runs
// its full window is a guaranteed-cancelled attempt), and it must be a
// whole number of seconds (the Bot API's timeout parameter is an integer
// count of seconds, so a value the loop cannot transmit as configured is
// rejected here rather than silently truncated on the poll path). Both
// failures name the same key, because there is exactly one thing wrong
// with it either way.
func checkIngestLongPollTimeout(ingest Ingest, transport Transport) error {
	if ingest.LongPollTimeout%time.Second != 0 {
		return keyErrorf(envIngestLongPollTimeout, ErrInvalidValue,
			"must be a whole number of seconds, got %s", ingest.LongPollTimeout)
	}
	if ingest.LongPollTimeout >= transport.AttemptTimeout {
		return keyErrorf(envIngestLongPollTimeout, ErrInvalidValue,
			"must be strictly below LAB_GAME_TG_ATTEMPT_TIMEOUT (%s), got %s", transport.AttemptTimeout, ingest.LongPollTimeout)
	}
	return nil
}
