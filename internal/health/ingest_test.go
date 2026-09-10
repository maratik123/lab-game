package health

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/maratik123/lab-game/internal/ingest"
)

func newIngestObserverForTest(t *testing.T) *IngestObserver {
	t.Helper()
	o, err := NewIngestObserver(NewRegistry())
	if err != nil {
		t.Fatalf("NewIngestObserver: %v", err)
	}
	return o
}

// TestIngestObserver_OneObservationPerOutcome asserts the closed
// outcome value set and that panic, duplicate, unrouted and given-up
// are each individually visible.
func TestIngestObserver_OneObservationPerOutcome(t *testing.T) {
	t.Parallel()
	outcomes := []ingest.Outcome{
		ingest.OutcomeHandled,
		ingest.OutcomeDuplicate,
		ingest.OutcomeUnrouted,
		ingest.OutcomeFailed,
		ingest.OutcomePanic,
		ingest.OutcomeGivenUp,
	}
	o := newIngestObserverForTest(t)
	for _, outcome := range outcomes {
		o.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: outcome, Duration: time.Millisecond})
	}

	mfs, err := gatherFrom(t, o.updateOutcomes)
	if err != nil {
		t.Fatalf("gatherFrom: %v", err)
	}
	values := observedLabelValues(mfs, labelOutcome)
	want := map[string]bool{"handled": true, "duplicate": true, "unrouted": true, "failed": true, "panic": true, "given_up": true}
	if len(values) != len(want) {
		t.Fatalf("observed outcome values = %v, want %v", values, want)
	}
	for v := range want {
		if !values[v] {
			t.Errorf("outcome %q not observed", v)
		}
	}
}

// TestIngestObserver_LagOnlyWhenLagKnown asserts an observation with
// LagKnown false contributes no lag sample at all, while the counter
// still increments — "no sample" is distinguished from "a zero-valued
// sample" — and an observation with LagKnown true contributes exactly
// one.
func TestIngestObserver_LagOnlyWhenLagKnown(t *testing.T) {
	t.Parallel()
	o := newIngestObserverForTest(t)
	o.ObserveUpdate(ingest.Observation{Kind: ingest.KindCallbackQuery, Outcome: ingest.OutcomeHandled, LagKnown: false})

	if count := testutil.CollectAndCount(o.updateLag); count != 0 {
		t.Errorf("updateLag count = %d, want 0 when LagKnown is false", count)
	}
	if count := testutil.CollectAndCount(o.updateOutcomes); count != 1 {
		t.Errorf("updateOutcomes count = %d, want 1 regardless of LagKnown", count)
	}

	o.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: ingest.OutcomeHandled, Lag: 5 * time.Second, LagKnown: true})
	if count := testutil.CollectAndCount(o.updateLag); count != 1 {
		t.Errorf("updateLag count = %d, want 1 after one LagKnown observation", count)
	}
}

// TestIngestObserver_EmptyKindMapsToUnknown asserts the empty Kind an
// unrouted update leaves behind maps to the unknown label value — the
// one place that value is genuinely observed rather than merely
// declared.
func TestIngestObserver_EmptyKindMapsToUnknown(t *testing.T) {
	t.Parallel()
	o := newIngestObserverForTest(t)
	o.ObserveUpdate(ingest.Observation{Kind: "", Outcome: ingest.OutcomeUnrouted})

	mfs, err := gatherFrom(t, o.updateOutcomes)
	if err != nil {
		t.Fatalf("gatherFrom: %v", err)
	}
	values := observedLabelValues(mfs, labelKind)
	if !values[unknownLabelValue] {
		t.Errorf("observed kind values = %v, want %q present", values, unknownLabelValue)
	}
}

// TestIngestObserver_NonNilErrIncrementsUndecodable asserts a non-nil
// Err increments the undecodable counter.
func TestIngestObserver_NonNilErrIncrementsUndecodable(t *testing.T) {
	t.Parallel()
	o := newIngestObserverForTest(t)
	o.ObserveUpdate(ingest.Observation{Outcome: ingest.OutcomeUnrouted, Err: errors.New("malformed operation_id")})

	if count := testutil.CollectAndCount(o.undecodable); count != 1 {
		t.Fatalf("undecodable count = %d, want 1", count)
	}
	if got := testutil.ToFloat64(o.undecodable); got != 1 {
		t.Errorf("undecodable value = %v, want 1", got)
	}

	o.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: ingest.OutcomeHandled})
	if got := testutil.ToFloat64(o.undecodable); got != 1 {
		t.Errorf("undecodable value = %v, want unchanged at 1 after a nil-Err observation", got)
	}
}

// TestIngestObserver_ExemptFieldsDoNotAffectTheScrape asserts the exempt
// half too: two observations differing only in Attempt produce
// byte-identical scrapes.
func TestIngestObserver_ExemptFieldsDoNotAffectTheScrape(t *testing.T) {
	t.Parallel()
	o1 := newIngestObserverForTest(t)
	o1.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: ingest.OutcomeFailed, Attempt: 0, Duration: time.Millisecond})
	mfs1, err := gatherFrom(t, o1.updateOutcomes)
	if err != nil {
		t.Fatalf("gatherFrom: %v", err)
	}

	o2 := newIngestObserverForTest(t)
	o2.ObserveUpdate(ingest.Observation{Kind: ingest.KindMessage, Outcome: ingest.OutcomeFailed, Attempt: 7, Duration: time.Millisecond})
	mfs2, err := gatherFrom(t, o2.updateOutcomes)
	if err != nil {
		t.Fatalf("gatherFrom: %v", err)
	}

	if got, want := gatherText(t, mfs1), gatherText(t, mfs2); got != want {
		t.Errorf("scrapes differ solely by Attempt:\n got: %s\nwant: %s", got, want)
	}
}

// TestIngestObserver_ObserveLoop covers the loop observation's three
// families directly.
func TestIngestObserver_ObserveLoop(t *testing.T) {
	t.Parallel()
	o := newIngestObserverForTest(t)
	o.ObserveLoop(ingest.LoopObservation{Duration: 10 * time.Millisecond, BatchSize: 5})
	if count := testutil.CollectAndCount(o.pollDuration); count != 1 {
		t.Errorf("pollDuration count = %d, want 1", count)
	}
	if count := testutil.CollectAndCount(o.pollBatch); count != 1 {
		t.Errorf("pollBatch count = %d, want 1", count)
	}
	if got := testutil.ToFloat64(o.pollErrors); got != 0 {
		t.Errorf("pollErrors value = %v, want 0 when Err is nil", got)
	}

	o.ObserveLoop(ingest.LoopObservation{Err: errors.New("poll failed")})
	if got := testutil.ToFloat64(o.pollErrors); got != 1 {
		t.Errorf("pollErrors value = %v, want 1", got)
	}
}
