package backoff

import (
	"testing"
	"time"
)

// TestExponential_exactTable pins Exponential's zero-based contract at an
// exact table of (attempt, base, ceiling) -> delay, including the
// zeroth attempt, the attempt at which the ceiling is first reached, and
// the attempt right after it, plus D2's base > ceiling row at the
// zeroth attempt (design D2, AC23).
func TestExponential_exactTable(t *testing.T) {
	t.Parallel()

	base := time.Second
	ceiling := 30 * time.Second
	cases := []struct {
		name    string
		attempt int
		base    time.Duration
		ceiling time.Duration
		want    time.Duration
	}{
		{"zeroth attempt", 0, base, ceiling, time.Second},
		{"attempt 1", 1, base, ceiling, 2 * time.Second},
		{"attempt 2", 2, base, ceiling, 4 * time.Second},
		{"attempt 3", 3, base, ceiling, 8 * time.Second},
		{"attempt 4", 4, base, ceiling, 16 * time.Second},
		{"ceiling first reached", 5, base, ceiling, 30 * time.Second}, // 32s clamped
		{"attempt past the ceiling", 6, base, ceiling, 30 * time.Second},
		// D2: base already above ceiling, at the zeroth attempt.
		{"base greater than ceiling", 0, 40 * time.Second, ceiling, ceiling},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Exponential(tc.attempt, tc.base, tc.ceiling); got != tc.want {
				t.Errorf("Exponential(%d, %v, %v) = %v, want %v", tc.attempt, tc.base, tc.ceiling, got, tc.want)
			}
		})
	}
}

// TestExponential_negativeAttemptClampsToZeroth asserts a negative
// attempt behaves exactly as the zeroth (AC23).
func TestExponential_negativeAttemptClampsToZeroth(t *testing.T) {
	t.Parallel()

	base, ceiling := time.Second, 30*time.Second
	want := Exponential(0, base, ceiling)
	for _, attempt := range []int{-1, -2, -1000} {
		if got := Exponential(attempt, base, ceiling); got != want {
			t.Errorf("Exponential(%d, %v, %v) = %v, want %v (clamped to the zeroth attempt)", attempt, base, ceiling, got, want)
		}
	}
}

// TestExponential_strictlyPositiveAndNonDecreasing walks a run of
// attempts and asserts the value is strictly positive, never exceeds the
// ceiling, and never decreases (AC23).
func TestExponential_strictlyPositiveAndNonDecreasing(t *testing.T) {
	t.Parallel()

	base, ceiling := 100*time.Millisecond, 2*time.Second
	prev := Exponential(0, base, ceiling)
	if prev <= 0 {
		t.Fatalf("Exponential(0, ...) = %v, want strictly positive", prev)
	}
	for attempt := 1; attempt <= 10; attempt++ {
		cur := Exponential(attempt, base, ceiling)
		if cur <= 0 {
			t.Fatalf("Exponential(%d, ...) = %v, want strictly positive", attempt, cur)
		}
		if cur < prev {
			t.Fatalf("Exponential(%d, ...) = %v is less than Exponential(%d, ...) = %v, want non-decreasing", attempt, cur, attempt-1, prev)
		}
		if cur > ceiling {
			t.Fatalf("Exponential(%d, ...) = %v exceeds the ceiling %v", attempt, cur, ceiling)
		}
		prev = cur
	}
	if prev != ceiling {
		t.Fatalf("Exponential(10, ...) = %v, want it to have reached the ceiling %v", prev, ceiling)
	}
}

// TestExponential_overflowCasesNeverWrap is the table D2's contract
// exists for: attempts far past any a naive base<<attempt survives (64,
// where a nanosecond base has already consumed int64's range; 1000,
// where no shift is even defined), asserted on the VALUE -- exactly the
// ceiling and strictly positive -- since a wrapped time.Duration is a
// silent negative, not a crash (AC23).
func TestExponential_overflowCasesNeverWrap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		attempt int
		base    time.Duration
		ceiling time.Duration
	}{
		{"nanosecond base, attempt 64", 64, time.Nanosecond, time.Second},
		{"nanosecond base, attempt 1000", 1000, time.Nanosecond, time.Second},
		{"one-second base, attempt 64", 64, time.Second, 30 * time.Second},
		{"one-second base, attempt 1000", 1000, time.Second, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Exponential(tc.attempt, tc.base, tc.ceiling)
			if got != tc.ceiling {
				t.Errorf("Exponential(%d, %v, %v) = %v, want exactly the ceiling %v", tc.attempt, tc.base, tc.ceiling, got, tc.ceiling)
			}
			if got <= 0 {
				t.Errorf("Exponential(%d, %v, %v) = %v, want strictly positive", tc.attempt, tc.base, tc.ceiling, got)
			}
		})
	}
}

// TestExponential_outOfDomainRows pins the decided answers outside the
// domain either shipped adopter can reach: base <= 0 returns base
// unchanged (no doubling, no lower clamp), and a positive base with a
// non-positive ceiling returns the ceiling unchanged (design D2, AC23).
func TestExponential_outOfDomainRows(t *testing.T) {
	t.Parallel()

	t.Run("base zero", func(t *testing.T) {
		t.Parallel()
		for _, attempt := range []int{0, 1, 1000} {
			if got := Exponential(attempt, 0, 30*time.Second); got != 0 {
				t.Errorf("Exponential(%d, 0, ...) = %v, want 0 unchanged", attempt, got)
			}
		}
	})
	t.Run("base negative", func(t *testing.T) {
		t.Parallel()
		base := -5 * time.Second
		for _, attempt := range []int{0, 1, 1000} {
			if got := Exponential(attempt, base, 30*time.Second); got != base {
				t.Errorf("Exponential(%d, %v, ...) = %v, want %v unchanged (no doubling, no lower clamp)", attempt, base, got, base)
			}
		}
	})
	t.Run("positive base, non-positive ceiling", func(t *testing.T) {
		t.Parallel()
		for _, ceiling := range []time.Duration{0, -time.Second} {
			for _, attempt := range []int{0, 1, 1000} {
				if got := Exponential(attempt, time.Second, ceiling); got != ceiling {
					t.Errorf("Exponential(%d, 1s, %v) = %v, want %v unchanged", attempt, ceiling, got, ceiling)
				}
			}
		}
	})
}

// stubJitter builds a jitter func() float64 stub returning a fixed value.
func stubJitter(v float64) func() float64 {
	return func() float64 { return v }
}

// TestEqualJitter_boundedWithinExpectedRange asserts EqualJitter's bound
// within [d/2, d) for a jitter at each end of its draw, its base >
// ceiling counterpart at ceiling/2, and its monotonicity for a fixed
// jitter (AC23, D2's base > ceiling contract clause).
func TestEqualJitter_boundedWithinExpectedRange(t *testing.T) {
	t.Parallel()

	base, ceiling := 100*time.Millisecond, 10*time.Second

	if got := EqualJitter(0, base, ceiling, stubJitter(0)); got != base/2 {
		t.Errorf("EqualJitter(0, jitter=0) = %v, want %v (half, no jitter added)", got, base/2)
	}
	if got := EqualJitter(0, base, ceiling, stubJitter(1)); got != base {
		t.Errorf("EqualJitter(0, jitter=1) = %v, want %v (half plus the full other half)", got, base)
	}

	// base > ceiling counterpart, at ceiling/2.
	if got := EqualJitter(0, 40*time.Second, ceiling, stubJitter(0)); got != ceiling/2 {
		t.Errorf("EqualJitter(base>ceiling, jitter=0) = %v, want %v", got, ceiling/2)
	}
	if got := EqualJitter(0, 40*time.Second, ceiling, stubJitter(1)); got != ceiling {
		t.Errorf("EqualJitter(base>ceiling, jitter=1) = %v, want %v", got, ceiling)
	}

	// Monotonicity for a fixed jitter.
	prev := EqualJitter(0, base, ceiling, stubJitter(0.5))
	for attempt := 1; attempt <= 10; attempt++ {
		cur := EqualJitter(attempt, base, ceiling, stubJitter(0.5))
		if cur < prev {
			t.Fatalf("EqualJitter(%d, ...) = %v is less than EqualJitter(%d, ...) = %v, want non-decreasing", attempt, cur, attempt-1, prev)
		}
		if cur >= ceiling {
			t.Fatalf("EqualJitter(%d, ...) = %v, want strictly below the ceiling %v", attempt, cur, ceiling)
		}
		prev = cur
	}
}

// TestEqualJitter_overflowCasesStayBounded repeats the overflow table for
// EqualJitter, asserting the result stays within [ceiling/2, ceiling)
// (AC23, D2's clamp-before-overflow contract).
func TestEqualJitter_overflowCasesStayBounded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		attempt int
		base    time.Duration
		ceiling time.Duration
	}{
		{"nanosecond base, attempt 64", 64, time.Nanosecond, time.Second},
		{"nanosecond base, attempt 1000", 1000, time.Nanosecond, time.Second},
		{"one-second base, attempt 64", 64, time.Second, 30 * time.Second},
		{"one-second base, attempt 1000", 1000, time.Second, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EqualJitter(tc.attempt, tc.base, tc.ceiling, stubJitter(0.5))
			if got < tc.ceiling/2 || got >= tc.ceiling {
				t.Errorf("EqualJitter(%d, %v, %v) = %v, want within [%v, %v)", tc.attempt, tc.base, tc.ceiling, got, tc.ceiling/2, tc.ceiling)
			}
		})
	}
}
