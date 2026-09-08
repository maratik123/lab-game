package scheduler

import "time"

// FailureKind classifies an Observation's failure, when Outcome is
// OutcomeFailed. Every non-zero member is worker knowledge no other
// surface can recover.
type FailureKind int

const (
	// FailureNone means the observation is not a failure.
	FailureNone FailureKind = iota
	// FailureHandler means the Handler returned a non-nil error.
	FailureHandler
	// FailureUnregistered means the claimed row's type has no
	// Declaration. The row is never executed, and this is the only
	// counter that shows an undeclared type's row is coming due
	// forever.
	FailureUnregistered
	// FailureDeadline means the task's per-task execution deadline was
	// breached: the worker abandoned that task's
	// transaction and reports the failure immediately, though the
	// settlement itself is deferred to the next cycle's drain.
	FailureDeadline
	// FailureRolledBack means the outcome the handler reported did not
	// survive: either its savepoint release failed (a swallowed database
	// error), or the task's own COMMIT failed.
	FailureRolledBack
)

// Observation is reported to an Observer exactly once per executed task
// — a task skipped at re-claim produces none, because nothing executed.
type Observation struct {
	// Type is the task's registered type.
	Type Type
	// Lag is the execution instant minus RunAt — a real, server-measured
	// elapsed interval, never a virtual one.
	Lag time.Duration
	// Outcome is the outcome this attempt settled as.
	Outcome Outcome
	// Failure classifies the failure when Outcome is OutcomeFailed;
	// FailureNone otherwise.
	Failure FailureKind
	// BatchSize is the discovery cardinality of the cycle this task came
	// from.
	BatchSize int
	// ConsecutiveFailures is the value the settlement will write —
	// computed from the row's failure count read at claim time, not
	// re-read, so it is truthful even for a FailureDeadline observation
	// whose settlement is deferred.
	ConsecutiveFailures int
}

// LoopObservation is reported to an Observer once per RunOnce cycle.
type LoopObservation struct {
	// Duration is the one Go-measured interval in this package: the wall
	// time the cycle took, via time.Since around it. Never persisted or
	// compared against a row.
	Duration time.Duration
	// BatchSize is the discovery statement's cardinality.
	BatchSize int
	// Err is a transient discovery failure, if any. Run does not stop on
	// it; without this field the error would be silently dropped, since
	// this package has no logger.
	Err error
}

// Observer receives this package's observations. A nil Observer is
// checked, not called, so the package compiles and its tests pass with
// no implementation installed.
type Observer interface {
	// ObserveTask reports one executed task.
	ObserveTask(Observation)
	// ObserveLoop reports one completed RunOnce cycle.
	ObserveLoop(LoopObservation)
}
