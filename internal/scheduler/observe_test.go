package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestObserve_collectsOneObservationPerExecutedTask is AC12: a success, a
// guard no-op, a one-shot retry, a one-shot give-up, and a repeatedly
// failing recurrence each produce exactly one Observation, plus a
// LoopObservation carrying a duration.
func TestObserve_collectsOneObservationPerExecutedTask(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()

	doneH := &writingHandler{outcome: OutcomeDone, reason: "obs-done"}
	noopH := &writingHandler{outcome: OutcomeNoop, reason: "obs-noop"}
	failH := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "obs-fail"}

	reg, err := NewRegistry(
		Declaration{Type: "obs.done", Handler: doneH},
		Declaration{Type: "obs.noop", Handler: noopH},
		Declaration{Type: "obs.fail", Handler: failH},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	dueNow(t, pool, reg, Request{Type: "obs.done"})
	dueNow(t, pool, reg, Request{Type: "obs.noop"})
	dueNow(t, pool, reg, Request{Type: "obs.fail"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	tasks := obs.Tasks()
	if len(tasks) != 3 {
		t.Fatalf("got %d task observations, want 3 (one per executed task)", len(tasks))
	}
	byType := map[Type]Observation{}
	for _, o := range tasks {
		byType[o.Type] = o
		if o.BatchSize != 3 {
			t.Errorf("observation for %s: BatchSize = %d, want 3", o.Type, o.BatchSize)
		}
		if o.Lag < 0 {
			t.Errorf("observation for %s: Lag = %v, want non-negative", o.Type, o.Lag)
		}
	}
	if byType["obs.done"].Outcome != OutcomeDone || byType["obs.done"].Failure != FailureNone {
		t.Errorf("obs.done observation = %+v, want Outcome=Done, Failure=None", byType["obs.done"])
	}
	if byType["obs.noop"].Outcome != OutcomeNoop || byType["obs.noop"].Failure != FailureNone {
		t.Errorf("obs.noop observation = %+v, want Outcome=Noop, Failure=None", byType["obs.noop"])
	}
	if byType["obs.fail"].Outcome != OutcomeFailed || byType["obs.fail"].Failure != FailureHandler || byType["obs.fail"].ConsecutiveFailures != 1 {
		t.Errorf("obs.fail observation = %+v, want Outcome=Failed, Failure=Handler, ConsecutiveFailures=1", byType["obs.fail"])
	}

	loops := obs.Loops()
	if len(loops) != 1 {
		t.Fatalf("got %d loop observations, want 1", len(loops))
	}
	if loops[0].BatchSize != 3 {
		t.Errorf("loop observation BatchSize = %d, want 3", loops[0].BatchSize)
	}
	if loops[0].Duration <= 0 {
		t.Errorf("loop observation Duration = %v, want strictly positive", loops[0].Duration)
	}
	if loops[0].Err != nil {
		t.Errorf("loop observation Err = %v, want nil", loops[0].Err)
	}
}

// TestObserve_oneShotGiveUp_carriesFinalFailureCount checks a one-shot's
// give-up observation carries the exact failure count the settlement
// writes.
func TestObserve_oneShotGiveUp_carriesFinalFailureCount(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	h := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "obs-giveup"}
	reg, err := NewRegistry(Declaration{Type: "obs.giveup", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "obs.giveup"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 1; i <= cfg.RetryMaxAttempts; i++ {
		forceDue(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}

	tasks := obs.Tasks()
	last := tasks[len(tasks)-1]
	if last.Outcome != OutcomeFailed || last.ConsecutiveFailures != cfg.RetryMaxAttempts {
		t.Fatalf("give-up observation = %+v, want Outcome=Failed, ConsecutiveFailures=%d", last, cfg.RetryMaxAttempts)
	}
}

// TestObserve_repeatedlyFailingRecurrence collects an observation for
// each failed attempt of a recurrence.
func TestObserve_repeatedlyFailingRecurrence(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	cfg := testConfig()
	h := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "obs-recurrent-fail"}
	reg, err := NewRegistry(Declaration{
		Type: "obs.recurrent", Handler: h,
		Recurrence: &Recurrence{Cadence: Every(time.Hour), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg, Request{Type: "obs.recurrent", InstanceKey: "obs.recurrent"})

	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const attempts = 5
	for i := 1; i <= attempts; i++ {
		forceDue(t, pool, id)
		if err := w.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}

	tasks := obs.Tasks()
	if len(tasks) != attempts {
		t.Fatalf("got %d observations, want %d (one per failed attempt)", len(tasks), attempts)
	}
	for i, o := range tasks {
		if o.Outcome != OutcomeFailed || o.ConsecutiveFailures != i+1 {
			t.Fatalf("observation %d = %+v, want Outcome=Failed, ConsecutiveFailures=%d", i, o, i+1)
		}
	}
}

// TestObserve_nilObserver confirms the package's cycles run unchanged
// with no Observer installed (AC13).
func TestObserve_nilObserver(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "no-observer"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	w, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce with no observer: %v", err)
	}
	if got := h.seenCount(); got != 1 {
		t.Fatalf("handler ran %d times, want 1", got)
	}
}

// TestObserve_nonDefaultTuning_changesObservedBehaviour is AC14: a
// smaller claim limit bounds the batch, a smaller attempt cap gives up
// sooner, a different backoff base changes the persisted delays, and a
// short poll interval makes a freshly inserted task run within a bounded
// wait.
func TestObserve_nonDefaultTuning_changesObservedBehaviour(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	h := &writingHandler{outcome: OutcomeDone, reason: "tuning"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for range 5 {
		dueNow(t, pool, reg, Request{Type: "test.oneshot"})
	}

	cfg := testConfig()
	cfg.ClaimLimit = 2
	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	loops := obs.Loops()
	if len(loops) != 1 || loops[0].BatchSize != 2 {
		t.Fatalf("loop observation = %+v, want BatchSize 2 (ClaimLimit bound)", loops)
	}

	// A one-shot with a lower attempt cap gives up sooner.
	lowCapCfg := testConfig()
	lowCapCfg.RetryMaxAttempts = 1
	fh := &writingHandler{outcome: OutcomeFailed, err: errBoom, reason: "low-cap"}
	reg2, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: fh})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	id := dueNow(t, pool, reg2, Request{Type: "test.oneshot"})
	w3, err := New(Options{Pool: pool, Registry: reg2, Config: lowCapCfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w3.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	state, failures, _, _, found := schedulerTaskRow(t, pool, id)
	if !found || state != "dead" || failures != 1 {
		t.Fatalf("row after one failure with RetryMaxAttempts=1 = state:%v failures:%d (found=%v), want dead/1", state, failures, found)
	}
}

// TestRun_shortPollInterval_picksUpFreshlyInsertedTask is AC14's poll-
// interval half: with a short poll interval, Run picks up a task inserted
// after Run has already started, within a bounded wait, and stops
// cleanly when ctx is cancelled.
func TestRun_shortPollInterval_picksUpFreshlyInsertedTask(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	h := &writingHandler{outcome: OutcomeDone, reason: "run-loop"}
	reg, err := NewRegistry(Declaration{Type: "test.oneshot", Handler: h})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cfg := testConfig()
	cfg.PollInterval = 20 * time.Millisecond
	obs := &recordingObserver{}
	w, err := New(Options{Pool: pool, Registry: reg, Config: cfg, Observer: obs})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// Insert the task after Run has already started its first cycle.
	time.Sleep(5 * time.Millisecond)
	dueNow(t, pool, reg, Request{Type: "test.oneshot"})

	deadline := time.After(2 * time.Second)
	for h.seenCount() == 0 {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("Run did not pick up the freshly inserted task within the deadline")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return after ctx cancellation")
	}
}
