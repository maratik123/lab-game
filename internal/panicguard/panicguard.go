// Package panicguard is the shared primitive for the ownership rule that
// a goroutine running a handler supplied through an interface recovers a
// panic at that boundary and records its stack. It exports one type
// carrying the recovered value and the stack captured at the recovery
// point, and a constructor built from a recover() result — the deferred
// function that calls recover() must still call it itself, since recover
// only unwinds a panic when invoked directly by a deferred function; this
// package only shapes what that call returns. The type implements error
// so a caller's usual fmt.Errorf("<pkg>: handler panic: %w", …) wrap puts
// the stack into whatever the caller already persists, and it exposes
// the stack as its own field so a log record can carry it as its own
// attribute instead of buried inside a message.
//
// The package imports the standard library only.
package panicguard

import (
	"fmt"
	"runtime/debug"
)

// Recovered is a panic caught at a handler boundary.
type Recovered struct {
	// Value is the value recover() returned.
	Value any
	// Stack is the stack captured at the recovery point, via
	// debug.Stack() — the runtime's own traceback, frame-capped by the
	// runtime itself, so a deeply recursive panic renders no larger a
	// stack than a moderately deep one.
	Stack []byte
}

// New builds a *Recovered from r, a recover() result captured by the
// caller's own deferred function — New cannot call recover() on the
// caller's behalf. It returns nil when r is nil, so a caller writes
//
//	if rec := panicguard.New(recover()); rec != nil { ... }
//
// unconditionally inside its own deferred function.
func New(r any) *Recovered {
	if r == nil {
		return nil
	}
	return &Recovered{Value: r, Stack: debug.Stack()}
}

// Error renders p's panic value followed by its captured stack, so a
// caller's %w wrap carries the stack into whatever it persists.
func (p *Recovered) Error() string {
	return fmt.Sprintf("panic: %v\n%s", p.Value, p.Stack)
}
