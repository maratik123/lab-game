package ingest

import (
	"sync"
	"testing"
	"time"
)

// recordingObserver collects every Observation and LoopObservation behind
// a mutex, so an assertion on the seam is exact and the collector is
// race-clean — reused by subtasks 9 and 10's loop and gate scenarios
// (Test Design subtask 8).
type recordingObserver struct {
	mu      sync.Mutex
	updates []Observation
	loops   []LoopObservation
}

func (o *recordingObserver) ObserveUpdate(obs Observation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.updates = append(o.updates, obs)
}

func (o *recordingObserver) ObserveLoop(obs LoopObservation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.loops = append(o.loops, obs)
}

func (o *recordingObserver) Updates() []Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Observation, len(o.updates))
	copy(out, o.updates)
	return out
}

func (o *recordingObserver) Loops() []LoopObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]LoopObservation, len(o.loops))
	copy(out, o.loops)
	return out
}

func TestObserveUpdate_nilObserverIsNotCalled(t *testing.T) {
	t.Parallel()

	// A nil Observer must be checked, not called (design D12, AC19); if
	// observeUpdate dereferenced it, this call would panic.
	observeUpdate(nil, Observation{Kind: KindMessage, Outcome: OutcomeHandled})
}

func TestObserveLoop_nilObserverIsNotCalled(t *testing.T) {
	t.Parallel()

	observeLoop(nil, LoopObservation{BatchSize: 3})
}

func TestObserveUpdate_reachesANonNilObserver(t *testing.T) {
	t.Parallel()

	rec := &recordingObserver{}
	obs := Observation{Kind: KindMessage, Attempt: 2, Outcome: OutcomeFailed, Duration: time.Second}
	observeUpdate(rec, obs)

	got := rec.Updates()
	if len(got) != 1 || got[0] != obs {
		t.Fatalf("Updates() = %+v, want exactly [%+v]", got, obs)
	}
}

func TestObserveLoop_reachesANonNilObserver(t *testing.T) {
	t.Parallel()

	rec := &recordingObserver{}
	obs := LoopObservation{Duration: time.Second, BatchSize: 5}
	observeLoop(rec, obs)

	got := rec.Loops()
	if len(got) != 1 || got[0].BatchSize != obs.BatchSize || got[0].Duration != obs.Duration {
		t.Fatalf("Loops() = %+v, want exactly [%+v]", got, obs)
	}
}

func TestOutcome_stringRendersADistinctNamePerMember(t *testing.T) {
	t.Parallel()

	cases := []struct {
		outcome Outcome
		want    string
	}{
		{OutcomeHandled, "OutcomeHandled"},
		{OutcomeDuplicate, "OutcomeDuplicate"},
		{OutcomeUnrouted, "OutcomeUnrouted"},
		{OutcomeFailed, "OutcomeFailed"},
		{OutcomePanic, "OutcomePanic"},
		{OutcomeGivenUp, "OutcomeGivenUp"},
	}
	seen := make(map[string]bool, len(cases))
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.outcome.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
		if seen[tc.want] {
			t.Errorf("name %q reused by more than one Outcome member", tc.want)
		}
		seen[tc.want] = true
	}
}

func TestOutcome_stringOutOfRangeRendersTheFallback(t *testing.T) {
	t.Parallel()

	if got := Outcome(999).String(); got != "OutcomeUnknown" {
		t.Errorf("String() for an out-of-range Outcome = %q, want %q", got, "OutcomeUnknown")
	}
}

func TestObservation_lagAbsentIsDistinguishableFromZeroLag(t *testing.T) {
	t.Parallel()

	absent := Observation{Kind: KindCallbackQuery, LagKnown: false}
	zero := Observation{Kind: KindMessage, Lag: 0, LagKnown: true}

	if absent.LagKnown {
		t.Error("absent.LagKnown = true, want false (KindCallbackQuery declares no date)")
	}
	if !zero.LagKnown {
		t.Error("zero.LagKnown = false, want true (a genuinely healthy zero lag)")
	}
	if absent.Lag != zero.Lag {
		t.Fatalf("absent.Lag = %v, zero.Lag = %v — both should be the zero Duration; LagKnown is what distinguishes them", absent.Lag, zero.Lag)
	}
}
