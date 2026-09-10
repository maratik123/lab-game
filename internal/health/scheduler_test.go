package health

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/maratik123/lab-game/internal/scheduler"
)

func newSchedulerObserverForTest(t *testing.T) *SchedulerObserver {
	t.Helper()
	o, err := NewSchedulerObserver(NewRegistry())
	if err != nil {
		t.Fatalf("NewSchedulerObserver: %v", err)
	}
	return o
}

func TestSchedulerObserver_ObserveTask_OutcomeLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		outcome scheduler.Outcome
		want    string
	}{
		{scheduler.OutcomeDone, "done"},
		{scheduler.OutcomeNoop, "noop"},
		{scheduler.OutcomeFailed, "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			o := newSchedulerObserverForTest(t)
			o.ObserveTask(scheduler.Observation{Type: "raid.tick", Outcome: tc.outcome})
			want := `
# HELP labgame_scheduler_tasks_total Executed scheduler tasks, by type, outcome and failure classification.
# TYPE labgame_scheduler_tasks_total counter
labgame_scheduler_tasks_total{failure="none",outcome="` + tc.want + `",type="raid.tick"} 1
`
			if err := testutil.CollectAndCompare(o.tasks, strings.NewReader(want)); err != nil {
				t.Errorf("tasks: %v", err)
			}
		})
	}
}

// TestSchedulerObserver_ObserveTask_FailureLabel drives one observation
// per FailureKind member and asserts the observed failure label set is
// exactly that enum's members.
func TestSchedulerObserver_ObserveTask_FailureLabel(t *testing.T) {
	t.Parallel()
	members := []struct {
		kind scheduler.FailureKind
		want string
	}{
		{scheduler.FailureNone, "none"},
		{scheduler.FailureHandler, "handler"},
		{scheduler.FailureUnregistered, "unregistered"},
		{scheduler.FailureDeadline, "deadline"},
		{scheduler.FailureRolledBack, "rolled_back"},
	}
	o := newSchedulerObserverForTest(t)
	for _, m := range members {
		o.ObserveTask(scheduler.Observation{Type: "raid.tick", Outcome: scheduler.OutcomeFailed, Failure: m.kind})
	}

	mfs, err := gatherFrom(t, o.tasks)
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := observedLabelValues(mfs, labelFailure)
	want := map[string]bool{"none": true, "handler": true, "unregistered": true, "deadline": true, "rolled_back": true}
	if len(got) != len(want) {
		t.Fatalf("observed failure values = %v, want exactly %v", got, want)
	}
	for v := range want {
		if !got[v] {
			t.Errorf("observed failure values %v missing %q", got, v)
		}
	}
}

func TestSchedulerObserver_ObserveTask_LagUnderTypeLabel(t *testing.T) {
	t.Parallel()
	o := newSchedulerObserverForTest(t)
	o.ObserveTask(scheduler.Observation{Type: "raid.tick", Lag: 2 * time.Second})

	if count := testutil.CollectAndCount(o.taskLag); count != 1 {
		t.Errorf("taskLag count = %d, want 1", count)
	}
}

func TestSchedulerObserver_ObserveLoop_ErrorIncrementsLoopErrors(t *testing.T) {
	t.Parallel()
	o := newSchedulerObserverForTest(t)
	o.ObserveLoop(scheduler.LoopObservation{Duration: time.Second, BatchSize: 5, Err: errors.New("boom")})

	if got := testutil.ToFloat64(o.loopErrors); got != 1 {
		t.Errorf("loopErrors = %v, want 1", got)
	}
	if count := testutil.CollectAndCount(o.loopDuration); count != 1 {
		t.Errorf("loopDuration count = %d, want 1 (Duration is meaningful even on a discovery error)", count)
	}
	if count := testutil.CollectAndCount(o.claimBatch); count != 1 {
		t.Errorf("claimBatch count = %d, want 1 (BatchSize is meaningful even on a discovery error)", count)
	}
}

func TestSchedulerObserver_ObserveLoop_NoErrorDoesNotIncrementLoopErrors(t *testing.T) {
	t.Parallel()
	o := newSchedulerObserverForTest(t)
	o.ObserveLoop(scheduler.LoopObservation{Duration: time.Second, BatchSize: 5})

	if got := testutil.ToFloat64(o.loopErrors); got != 0 {
		t.Errorf("loopErrors = %v, want 0", got)
	}
}

// TestSchedulerObserver_BatchSizeAndConsecutiveFailuresAreExempt asserts
// the exempt half of the observation-field register: two task
// observations differing only in BatchSize and ConsecutiveFailures
// produce byte-identical scrapes of the tasks family.
func TestSchedulerObserver_BatchSizeAndConsecutiveFailuresAreExempt(t *testing.T) {
	t.Parallel()
	o1 := newSchedulerObserverForTest(t)
	o1.ObserveTask(scheduler.Observation{Type: "raid.tick", Outcome: scheduler.OutcomeDone, BatchSize: 1, ConsecutiveFailures: 0})
	mfs1, err := gatherFrom(t, o1.tasks)
	if err != nil {
		t.Fatalf("gather 1: %v", err)
	}

	o2 := newSchedulerObserverForTest(t)
	o2.ObserveTask(scheduler.Observation{Type: "raid.tick", Outcome: scheduler.OutcomeDone, BatchSize: 100, ConsecutiveFailures: 7})
	mfs2, err := gatherFrom(t, o2.tasks)
	if err != nil {
		t.Fatalf("gather 2: %v", err)
	}

	text1, text2 := gatherText(t, mfs1), gatherText(t, mfs2)
	if text1 != text2 {
		t.Errorf("scrapes differ despite BatchSize/ConsecutiveFailures being the only difference:\n%s\nvs\n%s", text1, text2)
	}
}
