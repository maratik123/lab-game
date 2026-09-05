package scheduler

import (
	"testing"
	"time"
)

func TestBackoff_exactTable(t *testing.T) {
	t.Parallel()

	base := time.Second
	ceiling := 30 * time.Second
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second}, // 32s clamped to the ceiling
		{7, 30 * time.Second},
	}
	for _, tc := range cases {
		got := backoff(tc.failures, base, ceiling)
		if got != tc.want {
			t.Errorf("backoff(%d, %v, %v) = %v, want %v", tc.failures, base, ceiling, got, tc.want)
		}
	}
}

func TestBackoff_strictlyGrowingUntilCeiling(t *testing.T) {
	t.Parallel()

	base := 100 * time.Millisecond
	ceiling := 2 * time.Second
	prev := backoff(1, base, ceiling)
	if prev <= 0 {
		t.Fatalf("backoff(1, ...) = %v, want strictly positive", prev)
	}
	for f := 2; f <= 10; f++ {
		cur := backoff(f, base, ceiling)
		if cur < prev {
			t.Fatalf("backoff(%d) = %v is less than backoff(%d) = %v, want non-decreasing", f, cur, f-1, prev)
		}
		if cur > ceiling {
			t.Fatalf("backoff(%d) = %v exceeds the ceiling %v", f, cur, ceiling)
		}
		prev = cur
	}
	if prev != ceiling {
		t.Fatalf("backoff(10) = %v, want it to have reached the ceiling %v", prev, ceiling)
	}
}

func TestEvery_smallestOccurrenceStrictlyAfterNow(t *testing.T) {
	t.Parallel()

	period := time.Hour
	cadence := Every(period)
	prev := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"immediately after seeding", prev, prev.Add(period)},
		{"just before the next occurrence", prev.Add(period).Add(-time.Second), prev.Add(period)},
		{"exactly at the next occurrence (just-completed)", prev.Add(period), prev.Add(2 * period)},
		{"just after the next occurrence", prev.Add(period).Add(time.Second), prev.Add(2 * period)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cadence(prev, tc.now)
			if !got.Equal(tc.want) {
				t.Errorf("Every(%v)(%v, %v) = %v, want %v", period, prev, tc.now, got, tc.want)
			}
			if !got.After(tc.now) {
				t.Errorf("Every(%v)(%v, %v) = %v, want it strictly after now %v", period, prev, tc.now, got, tc.now)
			}
		})
	}
}

func TestEvery_catchUpAfterOutageYieldsOneOccurrence(t *testing.T) {
	t.Parallel()

	period := 24 * time.Hour
	cadence := Every(period)
	prev := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// A week has passed with nobody advancing run_at.
	now := prev.Add(7 * 24 * time.Hour)

	got := cadence(prev, now)
	want := prev.Add(8 * period) // the smallest occurrence strictly after now
	if !got.Equal(want) {
		t.Fatalf("Every(24h)(prev, prev+7d) = %v, want %v (exactly one occurrence past the outage)", got, want)
	}
	if !got.After(now) {
		t.Fatalf("Every(24h)(prev, prev+7d) = %v, want it after now %v", got, now)
	}
}
