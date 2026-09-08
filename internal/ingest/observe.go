package ingest

import "time"

// Outcome classifies one Observation. Every non-zero member
// beyond OutcomeHandled is knowledge only this package's loop has —
// nothing downstream can recover it from the database alone.
type Outcome int

const (
	// OutcomeHandled means the update's Handler ran and returned nil: its
	// writes and the offset advance committed together.
	OutcomeHandled Outcome = iota
	// OutcomeDuplicate means the Handler's error chain matched the
	// ledger package's ErrAlreadyPosted sentinel: the update's effects
	// already exist from an earlier delivery, no retry attempt was
	// consumed, and
	// the offset still advanced.
	OutcomeDuplicate
	// OutcomeUnrouted means the update's derived Kind has no registered
	// Handler: no attempt ran, and the offset still advanced.
	OutcomeUnrouted
	// OutcomeFailed means one attempt did not succeed. Either the
	// Handler returned a non-nil, non-duplicate error, or the Handler
	// succeeded and the attempt's own transaction work did not — the
	// guarded offset advance, or the COMMIT. Both are reported here
	// because both leave the update unsettled and consume an attempt;
	// distinguishing them is the observer's business, not the loop's.
	// The attempt's writes were rolled back and the update is retried,
	// unless this was the final attempt — see OutcomeGivenUp.
	OutcomeFailed
	// OutcomePanic means one attempt's Handler panicked. recover caught
	// it, the attempt's writes were rolled back, and — like OutcomeFailed
	// — the update is retried unless this was the final attempt. Reported
	// distinctly from OutcomeFailed so a recovered panic that a later
	// attempt fixes is never erased.
	OutcomePanic
	// OutcomeGivenUp means every configured attempt failed or panicked:
	// the update's identity was written to ingest_dead_update and the
	// offset still advanced, so a poisoned update never blocks the loop.
	OutcomeGivenUp
)

// String renders o's name, for logs and test failure messages. An
// out-of-range value renders "OutcomeUnknown(<n>)" — golangci-lint's
// exhaustive linter is satisfied by this default clause, since this
// module's lint config sets default-signifies-exhaustive.
func (o Outcome) String() string {
	switch o {
	case OutcomeHandled:
		return "OutcomeHandled"
	case OutcomeDuplicate:
		return "OutcomeDuplicate"
	case OutcomeUnrouted:
		return "OutcomeUnrouted"
	case OutcomeFailed:
		return "OutcomeFailed"
	case OutcomePanic:
		return "OutcomePanic"
	case OutcomeGivenUp:
		return "OutcomeGivenUp"
	default:
		return "OutcomeUnknown"
	}
}

// Observation is reported to an Observer once per handler call (attempt),
// plus once for each attempt-less settlement — unrouted and given-up:
// a single terminal observation per update would erase a
// panic that a later attempt recovered from.
type Observation struct {
	// Kind is the update's derived Kind.
	Kind Kind
	// Attempt is this attempt's ordinal, starting at 0. Meaningless for
	// OutcomeUnrouted, which never attempts a handler call.
	Attempt int
	// Outcome classifies this observation.
	Outcome Outcome
	// Duration is how long this observation's own unit of work took:
	// the h.Handle call itself for every attempt outcome
	// (OutcomeHandled, OutcomeDuplicate, OutcomeFailed, OutcomePanic) —
	// never the surrounding transaction plumbing (advanceOffset, Commit,
	// Rollback), so a slow database has no bearing on this number. For
	// OutcomeUnrouted, no handler ever runs, so Duration is the
	// unrouted-settlement statement instead. For OutcomeGivenUp, no
	// handler runs either — every failed attempt already reported its
	// own Duration on its own Observation — so this is the give-up
	// transaction alone (the dead-update write and the offset advance).
	Duration time.Duration
	// Lag is the update's own date subtracted from the observation
	// instant — meaningful only when LagKnown is true: a
	// kind whose payload declares no date (e.g. KindCallbackQuery) leaves
	// Lag at its zero value, which LagKnown distinguishes from a
	// genuinely healthy zero lag.
	Lag time.Duration
	// LagKnown reports whether Lag is meaningful.
	LagKnown bool
	// Err is the error NewUpdate returned while deriving this update,
	// when Outcome is OutcomeUnrouted because the raw payload could not
	// be parsed (a malformed operation_id component). Nil in every other
	// case, including a genuinely-unrouted Kind with no registered
	// Handler.
	Err error
}

// LoopObservation is reported to an Observer once per poll cycle.
type LoopObservation struct {
	// Duration is the wall time the cycle took.
	Duration time.Duration
	// BatchSize is the number of updates the cycle's getUpdates call
	// returned.
	BatchSize int
	// Err is the cycle's poll error, if any. Run does not stop on it;
	// without this field the error would be silently
	// dropped, since this package has no logger.
	Err error
}

// Observer receives this package's observations. A nil Observer is
// checked, not called, so the loop compiles and its
// tests pass with no implementation installed.
type Observer interface {
	// ObserveUpdate reports one Observation.
	ObserveUpdate(Observation)
	// ObserveLoop reports one LoopObservation.
	ObserveLoop(LoopObservation)
}

// observeUpdate reports obs to o, when o is non-nil.
func observeUpdate(o Observer, obs Observation) {
	if o != nil {
		o.ObserveUpdate(obs)
	}
}

// observeLoop reports obs to o, when o is non-nil.
func observeLoop(o Observer, obs LoopObservation) {
	if o != nil {
		o.ObserveLoop(obs)
	}
}
