package config

import (
	"errors"
	"time"
)

// Environment variable names for internal/scheduler's polling, claim-batch
// and retry tuning (design D13). Every one of these is optional: an
// absent value takes the compiled-in default named alongside it below —
// the same optional-with-default class internal/tg's transport keys
// established (transport.go), and the second such class this package now
// carries.
const (
	envSchedulerPollInterval    = "LAB_GAME_SCHEDULER_POLL_INTERVAL"
	envSchedulerClaimLimit      = "LAB_GAME_SCHEDULER_CLAIM_LIMIT"
	envSchedulerRetryMaxAttempt = "LAB_GAME_SCHEDULER_RETRY_MAX_ATTEMPTS"
	envSchedulerRetryBaseDelay  = "LAB_GAME_SCHEDULER_RETRY_BASE_DELAY"
	envSchedulerRetryMaxDelay   = "LAB_GAME_SCHEDULER_RETRY_MAX_DELAY"
	envSchedulerTaskTimeout     = "LAB_GAME_SCHEDULER_TASK_TIMEOUT"
)

// schedulerEnvKeys returns the six scheduler tuning variables, in
// declaration order (deterministic — AGENTS.md § Code Style). It is
// appended to EnvKeys() only; envKeys() itself is untouched, mirroring
// transportEnvKeys' rule (design D13).
func schedulerEnvKeys() []string {
	return []string{
		envSchedulerPollInterval,
		envSchedulerClaimLimit,
		envSchedulerRetryMaxAttempt,
		envSchedulerRetryBaseDelay,
		envSchedulerRetryMaxDelay,
		envSchedulerTaskTimeout,
	}
}

// Scheduler holds internal/scheduler's polling, claim-batch and retry
// tuning. Every field is optional-with-default (design D13) — like
// Transport, an absent LAB_GAME_SCHEDULER_ variable never fails Load.
// Every default here is chosen operational tuning, never a balance number
// and never sourced from docs/DESIGN.md — a task's game-meaningful delay
// is set by the mechanic that schedules it, from the balance file, never
// here.
type Scheduler struct {
	// PollInterval is how often the worker polls for due tasks
	// (LAB_GAME_SCHEDULER_POLL_INTERVAL, default 1s).
	PollInterval time.Duration
	// ClaimLimit bounds how many due tasks one discovery statement claims
	// (LAB_GAME_SCHEDULER_CLAIM_LIMIT, default 32).
	ClaimLimit int
	// RetryMaxAttempts is the maximum number of attempts a one-shot task
	// makes before it is given up on (LAB_GAME_SCHEDULER_RETRY_MAX_ATTEMPTS,
	// default 5).
	RetryMaxAttempts int
	// RetryBaseDelay is the backoff scale's base duration
	// (LAB_GAME_SCHEDULER_RETRY_BASE_DELAY, default 1s).
	RetryBaseDelay time.Duration
	// RetryMaxDelay caps the backoff scale's doubling
	// (LAB_GAME_SCHEDULER_RETRY_MAX_DELAY, default 5m). It also bounds how
	// often an undeclared task type's row is retried (design D7).
	RetryMaxDelay time.Duration
	// TaskTimeout bounds a single task's execution — the handler's context
	// deadline and the transaction-local statement_timeout and
	// idle_in_transaction_session_timeout (design D11)
	// (LAB_GAME_SCHEDULER_TASK_TIMEOUT, default 30s).
	TaskTimeout time.Duration
}

// defaultScheduler returns the compiled-in defaults every scheduler key
// falls back to when its environment variable is absent (design D13's
// table).
func defaultScheduler() Scheduler {
	return Scheduler{
		PollInterval:     time.Second,
		ClaimLimit:       32,
		RetryMaxAttempts: 5,
		RetryBaseDelay:   time.Second,
		RetryMaxDelay:    5 * time.Minute,
		TaskTimeout:      30 * time.Second,
	}
}

// loadScheduler reads and validates every LAB_GAME_SCHEDULER_ variable
// through lookup, querying each one unconditionally regardless of any
// other key's presence (required so the disjointness test's recording
// lookup still records an absent optional key — mirrors loadTransport).
// An absent variable takes its compiled-in default; a present, malformed
// value is reported as a *KeyError naming that variable. Every failure is
// collected and returned at once via errors.Join, in declaration order.
func loadScheduler(lookup Lookup) (*Scheduler, error) {
	var errs []error
	s := defaultScheduler()

	if d, ok, err := lookupPositiveDuration(lookup, envSchedulerPollInterval); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.PollInterval = d
	}

	if n, ok, err := lookupPositiveInt(lookup, envSchedulerClaimLimit); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.ClaimLimit = n
	}

	if n, ok, err := lookupPositiveInt(lookup, envSchedulerRetryMaxAttempt); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.RetryMaxAttempts = n
	}

	if d, ok, err := lookupPositiveDuration(lookup, envSchedulerRetryBaseDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.RetryBaseDelay = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envSchedulerRetryMaxDelay); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.RetryMaxDelay = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envSchedulerTaskTimeout); err != nil {
		errs = append(errs, err)
	} else if ok {
		s.TaskTimeout = d
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &s, nil
}
