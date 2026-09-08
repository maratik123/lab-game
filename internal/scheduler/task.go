package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Type is the persisted task-type name. The database stores it as text
// and validates nothing about it; the Registry is the sole authority on
// which types exist.
type Type string

// TaskID identifies a row in scheduled_task.
type TaskID int64

// Task is one claimed row handed to a Handler.
type Task struct {
	// ID is the scheduled_task row's id.
	ID TaskID
	// Type is the task's registered type.
	Type Type
	// InstanceKey is the identity's instance component, or the empty
	// string for a keyless one-shot.
	InstanceKey string
	// Payload is the row's raw JSON payload, exactly as stored (jsonb
	// normalises key order, whitespace and duplicate keys).
	Payload json.RawMessage
	// RunAt is the instant this task became due.
	RunAt time.Time
	// ConsecutiveFailures is the row's failure count before this attempt.
	ConsecutiveFailures int
}

// Request is what Schedule takes to insert a new task. Delay is relative,
// never an absolute instant: "a wave in five minutes" is what
// a mechanic's timer edge actually expresses, and a relative delay keeps
// the caller out of the clock business entirely.
type Request struct {
	// Type is the task's registered type. An unregistered type is refused
	// with ErrUnknownType.
	Type Type
	// InstanceKey identifies a live occurrence of this type for the
	// database's identity constraint. Empty means keyless: no identity is
	// enforced, and any number of such one-shots may coexist.
	InstanceKey string
	// Payload is the task's JSON payload, stored as jsonb. Nil is stored
	// as JSON null; callers that want an empty object pass
	// json.RawMessage(`{}`). A malformed value (not valid JSON) is refused
	// with ErrInvalidPayload.
	Payload json.RawMessage
	// Delay is how far in the future this task becomes due, relative to
	// the database's own clock at insertion. Negative is refused with
	// ErrInvalidDelay; zero means due immediately.
	Delay time.Duration
}

// Outcome is what a Handler reports for a task it executed.
type Outcome int

const (
	// OutcomeDone means the handler's writes should be kept and the task
	// is finished — deleted if one-shot, rescheduled to its next
	// occurrence if recurrent.
	OutcomeDone Outcome = iota
	// OutcomeNoop means the handler's writes should be discarded (a guard
	// miss) but the task is still settled as if it had run: deleted if
	// one-shot, rescheduled if recurrent.
	OutcomeNoop
	// OutcomeFailed means the handler's writes should be discarded and
	// the attempt counted as a failure. The worker also produces this
	// outcome itself from a Handler that returned a non-nil error.
	OutcomeFailed
)

// DeadTask is one give-up row, enumerated by DeadTasks.
type DeadTask struct {
	// Type is the task's registered type.
	Type Type
	// InstanceKey is the identity's instance component, or empty.
	InstanceKey string
	// RunAt is the instant the row was due when it gave up.
	RunAt time.Time
	// ConsecutiveFailures is the attempt count at give-up.
	ConsecutiveFailures int
	// LastError is the last recorded failure reason.
	LastError string
}

// Handler executes one task type's effects. Implementations are declared
// by the consumer and registered once, at start-up, through Declaration.
//
// A Handler MUST propagate the ctx it is handed to every call it makes on
// tx: this is the contract the per-task execution deadline
// depends on to reclaim a task's locked row after a breach. A Handler
// that issues statements on a context of its own — context.Background(),
// or a fresh context.WithTimeout — never sees the deadline's
// cancellation, and its row stays locked until the handler returns on its
// own, however long that takes.
type Handler interface {
	// Execute runs task's effects inside tx, the worker's own transaction
	// for this attempt. tx is already under a per-task statement and
	// idle-in-transaction timeout; Execute must honour ctx's deadline on
	// every call it makes on tx.
	Execute(ctx context.Context, tx pgx.Tx, task Task) (Outcome, error)
}
