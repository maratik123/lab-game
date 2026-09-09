package health

import (
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewRegistry_GathersNothing(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(mfs) != 0 {
		t.Errorf("Gather() returned %d families, want 0", len(mfs))
	}
}

func TestRegisterRuntime_MakesRuntimeFamiliesPresent(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("Gather() returned no families after RegisterRuntime")
	}
}

func TestRegisterRuntime_TwiceReturnsErrorNotPanic(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime (first): %v", err)
	}
	if err := RegisterRuntime(reg); err == nil {
		t.Fatal("RegisterRuntime (second): want an error, got nil")
	}
}

// bucketSets names every bucket variable explicitly, rather than
// reflecting over the package, so adding a new bucket set without adding
// its row here is a visible omission in review.
func bucketSets(t *testing.T) map[string][]float64 {
	t.Helper()
	return map[string][]float64{
		"durationBuckets":           durationBuckets,
		"updateLagBuckets":          updateLagBuckets,
		"schedulerLagBuckets":       schedulerLagBuckets,
		"pollDurationBuckets":       pollDurationBuckets,
		"schedulerLoopBuckets":      schedulerLoopBuckets,
		"ingestBatchSizeBuckets":    ingestBatchSizeBuckets,
		"schedulerBatchSizeBuckets": schedulerBatchSizeBuckets,
	}
}

func TestBucketSets_NonEmptyStrictlyIncreasingFinitePositive(t *testing.T) {
	t.Parallel()
	for name, buckets := range bucketSets(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if len(buckets) == 0 {
				t.Fatalf("%s is empty — an empty slice is silently replaced by the client library's own default buckets rather than panicking", name)
			}
			prev := math.Inf(-1)
			for i, b := range buckets {
				if math.IsInf(b, 0) || math.IsNaN(b) {
					t.Errorf("%s[%d] = %v, want a finite value (no hand-written +Inf)", name, i, b)
				}
				if b <= 0 {
					t.Errorf("%s[%d] = %v, want strictly positive", name, i, b)
				}
				if b <= prev {
					t.Errorf("%s[%d] = %v, not strictly greater than the previous boundary %v", name, i, b, prev)
				}
				prev = b
			}
		})
	}
}

// TestAllowedLabelNames_NamesNoLeLabel asserts the other histogram panic
// surface: no label this package declares is ever named "le", which is
// the client library's own reserved bucket-boundary label.
func TestAllowedLabelNames_NamesNoLeLabel(t *testing.T) {
	t.Parallel()
	for _, name := range allowedLabelNames {
		if name == "le" {
			t.Errorf("allowedLabelNames contains %q — \"le\" is the client library's own reserved histogram label", name)
		}
	}
}

func TestAllowedLabelNames_MatchesTheNamedConstants(t *testing.T) {
	t.Parallel()
	want := []string{labelMethod, labelCode, labelType, labelOutcome, labelFailure, labelKind, labelLeg, labelReason, labelState}
	if len(allowedLabelNames) != len(want) {
		t.Fatalf("allowedLabelNames has %d entries, want %d", len(allowedLabelNames), len(want))
	}
	for i, w := range want {
		if allowedLabelNames[i] != w {
			t.Errorf("allowedLabelNames[%d] = %q, want %q", i, allowedLabelNames[i], w)
		}
	}
}

// TestObservationRegister_CoversTheFiveObservationStructs is a subtask-4
// sanity check over the table's shape; the full both-directions
// reflection guard is a later subtask's job.
func TestObservationRegister_CoversTheFiveObservationStructs(t *testing.T) {
	t.Parallel()
	want := []string{
		"tg.Observation",
		"scheduler.Observation",
		"scheduler.LoopObservation",
		"ingest.Observation",
		"ingest.LoopObservation",
	}
	if len(observationRegister) != len(want) {
		t.Fatalf("observationRegister has %d structs, want %d", len(observationRegister), len(want))
	}
	for _, name := range want {
		entries, ok := observationRegister[name]
		if !ok {
			t.Errorf("observationRegister has no entry for %q", name)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("observationRegister[%q] is empty", name)
		}
	}
}

// TestObservationRegister_EveryEntryHasAFamilyOrIsExplicitlyExempt pins
// the table's own invariant: an entry either names a family or is in the
// explicit exempt set — never neither.
func TestObservationRegister_EveryEntryHasAFamilyOrIsExplicitlyExempt(t *testing.T) {
	t.Parallel()
	exempt := map[string]bool{
		"BatchSize":           true, // the task-level field only; the loop-level alias carries a family
		"ConsecutiveFailures": true,
		"Attempt":             true,
	}
	for structName, entries := range observationRegister {
		for _, e := range entries {
			if e.Family == "" && !exempt[e.Field] {
				t.Errorf("observationRegister[%q][%q] has no family and is not in the exempt set", structName, e.Field)
			}
		}
	}
}

// TestRegisterRuntime_CollectAndCountAgrees cross-checks Gather's own
// count against testutil.CollectAndCount, the assertion helper the rest
// of this package's suite (later subtasks) will use throughout.
func TestRegisterRuntime_CollectAndCountAgrees(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if err := RegisterRuntime(reg); err != nil {
		t.Fatalf("RegisterRuntime: %v", err)
	}
	if count := testutil.CollectAndCount(reg); count == 0 {
		t.Fatal("CollectAndCount returned 0 after RegisterRuntime")
	}
}
