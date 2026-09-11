package health

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// gatherFamily returns the one *dto.MetricFamily named name from reg's
// gather, failing t when it is absent or duplicated.
func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var found *dto.MetricFamily
	for _, mf := range mfs {
		if mf.GetName() == name {
			if found != nil {
				t.Fatalf("family %s gathered more than once", name)
			}
			found = mf
		}
	}
	if found == nil {
		t.Fatalf("family %s not found in gather", name)
	}
	return found
}

func TestNewProcess_RefusesNilReady(t *testing.T) {
	t.Parallel()
	_, err := NewProcess(NewRegistry(), ProcessOptions{Version: "v1"})
	var optErr *OptionError
	if !errors.As(err, &optErr) || optErr.Field != "Ready" {
		t.Fatalf("NewProcess(nil Ready) = %v, want an *OptionError naming Ready", err)
	}
}

// TestNewProcess_RegistrationFailureIsReported drives the one error
// path NewProcess's own logic does not construct itself: a second call
// on a registry that already carries these families collides on the
// duplicate-registration error every prometheus.Registerer returns.
func TestNewProcess_RegistrationFailureIsReported(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: alwaysReady}); err != nil {
		t.Fatalf("first NewProcess: %v", err)
	}
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: alwaysReady}); err == nil {
		t.Fatal("second NewProcess on the same registry: expected a duplicate-registration error, got nil")
	}
}

// TestNewProcess_RegistersTheDocumentedFamilies asserts every
// process-identity and restart-hygiene family is present after
// registration.
func TestNewProcess_RegistersTheDocumentedFamilies(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: alwaysReady}); err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	for _, name := range []string{
		familyBuildInfo,
		familyStartTimeSeconds,
		familyReady,
		familyRestartDowntime,
		familyRestartShiftedTasks,
	} {
		gatherFamily(t, reg, name)
	}
}

// TestNewProcess_BuildInfoCarriesVersionAsLabelValue asserts that the
// build version is a label value, always 1, and never a substring of
// any metric NAME in this package.
func TestNewProcess_BuildInfoCarriesVersionAsLabelValue(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	const version = "v1.2.3-test"
	if _, err := NewProcess(reg, ProcessOptions{Version: version, StartedAt: time.Now(), Ready: alwaysReady}); err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	mf := gatherFamily(t, reg, familyBuildInfo)
	if len(mf.GetMetric()) != 1 {
		t.Fatalf("labgame_build_info has %d series, want 1", len(mf.GetMetric()))
	}
	m := mf.GetMetric()[0]
	if got := m.GetGauge().GetValue(); got != 1 {
		t.Errorf("labgame_build_info value = %v, want 1", got)
	}
	var gotVersion string
	for _, lp := range m.GetLabel() {
		if lp.GetName() == labelVersion {
			gotVersion = lp.GetValue()
		}
	}
	if gotVersion != version {
		t.Errorf("labgame_build_info version label = %q, want %q", gotVersion, version)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range mfs {
		if family.GetName() != familyBuildInfo && strings.Contains(family.GetName(), version) {
			t.Errorf("family name %q contains the build version — it must be a label value only", family.GetName())
		}
	}
}

// TestNewProcess_StartTimeCarriesTheSuppliedInstant asserts
// labgame_start_time_seconds reports the caller-supplied StartedAt,
// never a value this file computed from its own clock.
func TestNewProcess_StartTimeCarriesTheSuppliedInstant(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	startedAt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: startedAt, Ready: alwaysReady}); err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	mf := gatherFamily(t, reg, familyStartTimeSeconds)
	got := mf.GetMetric()[0].GetGauge().GetValue()
	if want := float64(startedAt.Unix()); got != want {
		t.Errorf("labgame_start_time_seconds = %v, want %v", got, want)
	}
}

// TestNewProcess_ReadyGaugeFollowsReadyFunc asserts labgame_ready tracks
// the injected ReadyFunc across two gathers, in both directions.
func TestNewProcess_ReadyGaugeFollowsReadyFunc(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	ready := true
	fn := func(context.Context) error {
		if ready {
			return nil
		}
		return errors.New("not ready")
	}
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: fn}); err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	if got := gatherFamily(t, reg, familyReady).GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Errorf("labgame_ready = %v, want 1 while Ready succeeds", got)
	}

	ready = false
	if got := gatherFamily(t, reg, familyReady).GetMetric()[0].GetGauge().GetValue(); got != 0 {
		t.Errorf("labgame_ready = %v, want 0 while Ready fails", got)
	}

	ready = true
	if got := gatherFamily(t, reg, familyReady).GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Errorf("labgame_ready = %v, want 1 again once Ready succeeds again", got)
	}
}

// TestNewProcess_ReadyGaugeSurvivesABlockingReadyFunc proves the
// gauge's own internal bound: a ReadyFunc that blocks past
// readyGaugeTimeout still lets the gather return.
func TestNewProcess_ReadyGaugeSurvivesABlockingReadyFunc(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	blocking := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if _, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: blocking}); err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	done := make(chan *dto.MetricFamily, 1)
	go func() { done <- gatherFamily(t, reg, familyReady) }()

	select {
	case mf := <-done:
		if got := mf.GetMetric()[0].GetGauge().GetValue(); got != 0 {
			t.Errorf("labgame_ready = %v, want 0 (Ready never returned nil)", got)
		}
	case <-time.After(readyGaugeTimeout + 5*time.Second):
		t.Fatal("Gather did not return within readyGaugeTimeout plus a generous guard")
	}
}

// TestProcess_ObserveDowntimeSetsTheRestartGauges asserts that ObserveDowntime sets both restart gauges.
func TestProcess_ObserveDowntimeSetsTheRestartGauges(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	p, err := NewProcess(reg, ProcessOptions{Version: "v1", StartedAt: time.Now(), Ready: alwaysReady})
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}

	p.ObserveDowntime(90*time.Second, 7)

	if got := testutil.ToFloat64(p.restartDowntime); got != 90 {
		t.Errorf("restartDowntime = %v, want 90", got)
	}
	if got := testutil.ToFloat64(p.restartShiftedTasks); got != 7 {
		t.Errorf("restartShiftedTasks = %v, want 7", got)
	}
}
