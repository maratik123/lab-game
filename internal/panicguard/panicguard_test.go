package panicguard

import (
	"strings"
	"testing"
)

// recoverFrom runs f, recovers any panic it raises via its own deferred
// function (the shape every real call site uses), and returns New's
// result.
func recoverFrom(f func()) *Recovered {
	var rec *Recovered
	func() {
		defer func() {
			rec = New(recover())
		}()
		f()
	}()
	return rec
}

// panickingHelper is the fixture whose own frame name the stack
// assertions look for.
func panickingHelper(value any) {
	panic(value)
}

// callsPanickingHelper is panickingHelper's caller, so a captured stack
// names both.
func callsPanickingHelper(value any) {
	panickingHelper(value)
}

func TestNew_nilRecoverResultYieldsNothing(t *testing.T) {
	t.Parallel()
	rec := recoverFrom(func() {})
	if rec != nil {
		t.Fatalf("New(recover()) = %+v, want nil for a call that never panicked", rec)
	}
}

func TestNew_capturesValueAndStackNamingFrameAndCaller(t *testing.T) {
	t.Parallel()
	rec := recoverFrom(func() { callsPanickingHelper("boom") })
	if rec == nil {
		t.Fatal("New(recover()) = nil, want non-nil for a call that panicked")
	}
	if rec.Value != "boom" {
		t.Errorf("Value = %v, want %q", rec.Value, "boom")
	}
	stack := string(rec.Stack)
	if !strings.Contains(stack, "panickingHelper") {
		t.Errorf("Stack does not name the panicking function panickingHelper:\n%s", stack)
	}
	if !strings.Contains(stack, "callsPanickingHelper") {
		t.Errorf("Stack does not name the panicking function's caller callsPanickingHelper:\n%s", stack)
	}
	rendered := rec.Error()
	if !strings.Contains(rendered, "boom") {
		t.Errorf("Error() = %q, want it to contain the panic value", rendered)
	}
}

// recurse panics at depth 0, after n non-tail recursive calls, so the
// call stack at the panic point is n frames deep.
func recurse(n int) {
	if n <= 0 {
		panic("deep boom")
	}
	recurse(n - 1)
}

// TestNew_deeplyRecursivePanicStackIsBounded asserts the captured stack
// does not grow without bound with recursion depth: the runtime's own
// traceback is frame-capped, so a deeply recursive panic renders a
// stack no larger than a generous ceiling far above any plateaued
// traceback — never a length proportional to the 50000 frames recursed
// here.
func TestNew_deeplyRecursivePanicStackIsBounded(t *testing.T) {
	t.Parallel()
	rec := recoverFrom(func() { recurse(50000) })
	if rec == nil {
		t.Fatal("New(recover()) = nil, want non-nil for a call that panicked")
	}
	const ceiling = 1 << 20 // 1 MiB: far above any plateaued runtime traceback.
	if len(rec.Stack) > ceiling {
		t.Fatalf("len(Stack) = %d, want bounded (<= %d) even for a deeply recursive panic", len(rec.Stack), ceiling)
	}
}

func TestRecovered_ErrorImplementsError(t *testing.T) {
	t.Parallel()
	rec := recoverFrom(func() { panic("boom") })
	var err error = rec
	if err.Error() == "" {
		t.Fatal("*Recovered.Error() = \"\", want a non-empty rendering")
	}
}
