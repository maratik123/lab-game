package health

import (
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/scheduler"
)

// Label names this package's families use. This is a ceiling, not an
// obligation: not every family carries every one of these, and no family
// this package declares carries a label outside this set.
const (
	labelMethod  = "method"
	labelCode    = "code"
	labelType    = "type"
	labelOutcome = "outcome"
	labelFailure = "failure"
	labelKind    = "kind"
	labelLeg     = "leg"
	labelReason  = "reason"
	labelState   = "state"
	labelVersion = "version"
)

// allowedLabelNames is the ceiling above, as a slice for a guard to walk.
var allowedLabelNames = []string{
	labelMethod,
	labelCode,
	labelType,
	labelOutcome,
	labelFailure,
	labelKind,
	labelLeg,
	labelReason,
	labelState,
	labelVersion,
}

// unknownLabelValue is the closed-set fallback every enum mapper in this
// package returns for an out-of-range input, keeping the value set closed
// however the source enum grows.
const unknownLabelValue = "unknown"

// schedulerOutcomeLabel maps a task outcome to its label value. The
// default branch keeps this switch total under the linter's
// default-signifies-exhaustive setting, and it is exercised directly by
// this package's own unit test; whether an out-of-range value can ever
// reach it depends on the worker's own internal validation, which this
// mapper does not assume.
func schedulerOutcomeLabel(o scheduler.Outcome) string {
	switch o {
	case scheduler.OutcomeDone:
		return "done"
	case scheduler.OutcomeNoop:
		return "noop"
	case scheduler.OutcomeFailed:
		return "failed"
	default:
		return unknownLabelValue
	}
}

// schedulerFailureLabel maps a task's failure classification to its label
// value. The default branch is unreachable in an observed series — every
// FailureKind an observation can carry is set by the worker itself — but
// it keeps this switch total, and it is exercised directly by this
// package's own unit test.
func schedulerFailureLabel(f scheduler.FailureKind) string {
	switch f {
	case scheduler.FailureNone:
		return "none"
	case scheduler.FailureHandler:
		return "handler"
	case scheduler.FailureUnregistered:
		return "unregistered"
	case scheduler.FailureDeadline:
		return "deadline"
	case scheduler.FailureRolledBack:
		return "rolled_back"
	default:
		return unknownLabelValue
	}
}

// ingestOutcomeLabel maps an update outcome to its label value. The
// default branch is unreachable in an observed series — this outcome is
// set by the ingest loop alone, never by a consumer — but it keeps this
// switch total, and it is exercised directly by this package's own unit
// test.
func ingestOutcomeLabel(o ingest.Outcome) string {
	switch o {
	case ingest.OutcomeHandled:
		return "handled"
	case ingest.OutcomeDuplicate:
		return "duplicate"
	case ingest.OutcomeUnrouted:
		return "unrouted"
	case ingest.OutcomeFailed:
		return "failed"
	case ingest.OutcomePanic:
		return "panic"
	case ingest.OutcomeGivenUp:
		return "given_up"
	default:
		return unknownLabelValue
	}
}

// ingestKindLabel carries k verbatim as the label value, except the empty
// Kind an unrouted update leaves behind, which maps to the same
// unknownLabelValue every other mapper in this package uses — so no
// series in this package ever carries an empty label value. Unlike the
// three mappers above, this is a genuinely observed mapping: an unrouted
// update really does carry the zero Kind.
func ingestKindLabel(k ingest.Kind) string {
	if k == "" {
		return unknownLabelValue
	}
	return string(k)
}
