package backoff

import (
	"math"
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
			if got := Exponential(tc.attempt, tc.base, tc.ceiling, 2); got != tc.want {
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
	want := Exponential(0, base, ceiling, 2)
	for _, attempt := range []int{-1, -2, -1000} {
		if got := Exponential(attempt, base, ceiling, 2); got != want {
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
	prev := Exponential(0, base, ceiling, 2)
	if prev <= 0 {
		t.Fatalf("Exponential(0, ...) = %v, want strictly positive", prev)
	}
	for attempt := 1; attempt <= 10; attempt++ {
		cur := Exponential(attempt, base, ceiling, 2)
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
			got := Exponential(tc.attempt, tc.base, tc.ceiling, 2)
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
			if got := Exponential(attempt, 0, 30*time.Second, 2); got != 0 {
				t.Errorf("Exponential(%d, 0, ...) = %v, want 0 unchanged", attempt, got)
			}
		}
	})
	t.Run("base negative", func(t *testing.T) {
		t.Parallel()
		base := -5 * time.Second
		for _, attempt := range []int{0, 1, 1000} {
			if got := Exponential(attempt, base, 30*time.Second, 2); got != base {
				t.Errorf("Exponential(%d, %v, ...) = %v, want %v unchanged (no doubling, no lower clamp)", attempt, base, got, base)
			}
		}
	})
	t.Run("positive base, non-positive ceiling", func(t *testing.T) {
		t.Parallel()
		for _, ceiling := range []time.Duration{0, -time.Second} {
			for _, attempt := range []int{0, 1, 1000} {
				if got := Exponential(attempt, time.Second, ceiling, 2); got != ceiling {
					t.Errorf("Exponential(%d, 1s, %v) = %v, want %v unchanged", attempt, ceiling, got, ceiling)
				}
			}
		}
	})
}

// TestDefaultFactor_isExactlyTwo pins DefaultFactor's value: every
// behaviour-preserving claim in design D20's amendment rests on it, and
// nothing else in this suite would notice it moving (design D20).
func TestDefaultFactor_isExactlyTwo(t *testing.T) {
	t.Parallel()

	if DefaultFactor != 2 {
		t.Errorf("DefaultFactor = %v, want exactly 2", DefaultFactor)
	}
}

// TestValidFactor_exactTable pins ValidFactor's boundary: finite and
// strictly greater than 1 (design D20, AC42, AC43).
func TestValidFactor_exactTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		factor float64
		want   bool
	}{
		{"default factor", 2, true},
		{"non-integer factor", 1.3, true},
		{"smallest float64 strictly above 1", math.Nextafter(1, 2), true},
		{"large finite factor", 1e300, true},
		{"exactly one", 1, false},
		{"just below one", math.Nextafter(1, 0), false},
		{"zero", 0, false},
		{"negative", -1, false},
		{"NaN", math.NaN(), false},
		{"positive infinity", math.Inf(1), false},
		{"negative infinity", math.Inf(-1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidFactor(tc.factor); got != tc.want {
				t.Errorf("ValidFactor(%v) = %v, want %v", tc.factor, got, tc.want)
			}
		})
	}
}

// TestExponential_factorAtDefaultTwoIsByteIdentical re-runs the shipped
// zero-based contract table with a LITERAL 2 in the new argument — never
// DefaultFactor, so a later move of the default cannot silently carry a
// pinned table with it — asserting every expected value stays
// byte-identical (design D20's behaviour-preservation clause, AC40).
func TestExponential_factorAtDefaultTwoIsByteIdentical(t *testing.T) {
	t.Parallel()

	base := time.Second
	ceiling := 30 * time.Second
	cases := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"zeroth attempt", 0, time.Second},
		{"attempt 1", 1, 2 * time.Second},
		{"attempt 2", 2, 4 * time.Second},
		{"attempt 3", 3, 8 * time.Second},
		{"attempt 4", 4, 16 * time.Second},
		{"ceiling first reached", 5, 30 * time.Second},
		{"attempt past the ceiling", 6, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Exponential(tc.attempt, base, ceiling, 2); got != tc.want {
				t.Errorf("Exponential(%d, %v, %v, 2) = %v, want %v", tc.attempt, base, ceiling, got, tc.want)
			}
		})
	}
}

// TestExponential_integerBaseShiftAtNonDefaultFactor pins an independent
// EXACT expectation — base<<k as an integer, computed by the test rather
// than by calling Exponential's own formula — at a factor that is a
// small integer, over attempts where the shift stays below the ceiling
// and the base stays below 2^53 nanoseconds (design D20's exact-equality
// bound, AC40).
func TestExponential_integerBaseShiftAtNonDefaultFactor(t *testing.T) {
	t.Parallel()

	base := time.Millisecond
	ceiling := time.Hour
	const factor = 3
	for attempt := 0; attempt <= 6; attempt++ {
		want := base
		for range attempt {
			want *= factor
		}
		if got := Exponential(attempt, base, ceiling, factor); got != want {
			t.Errorf("Exponential(%d, %v, %v, %v) = %v, want %v", attempt, base, ceiling, float64(factor), got, want)
		}
	}
}

// TestExponential_factorOutOfDomainRows pins D20's normalisation table:
// a factor not strictly greater than 1 (below 1, exactly 1, or NaN)
// normalises to exactly 1 — base at every attempt, clamped by the
// ceiling, never raised — and +Inf gives base at the zeroth attempt
// (factor^0 == 1) and ceiling from the first attempt on (design D20).
func TestExponential_factorOutOfDomainRows(t *testing.T) {
	t.Parallel()

	base, ceiling := time.Second, 30*time.Second

	t.Run("normalised to one", func(t *testing.T) {
		t.Parallel()
		for _, factor := range []float64{0.5, 1, math.NaN()} {
			for _, attempt := range []int{0, 1, 1000} {
				if got := Exponential(attempt, base, ceiling, factor); got != base {
					t.Errorf("Exponential(%d, %v, %v, %v) = %v, want %v (normalised to factor 1)", attempt, base, ceiling, factor, got, base)
				}
			}
		}
	})

	t.Run("positive infinity", func(t *testing.T) {
		t.Parallel()
		if got := Exponential(0, base, ceiling, math.Inf(1)); got != base {
			t.Errorf("Exponential(0, %v, %v, +Inf) = %v, want %v (factor^0 == 1)", base, ceiling, got, base)
		}
		for _, attempt := range []int{1, 2, 1000} {
			if got := Exponential(attempt, base, ceiling, math.Inf(1)); got != ceiling {
				t.Errorf("Exponential(%d, %v, %v, +Inf) = %v, want %v", attempt, base, ceiling, got, ceiling)
			}
		}
	})

	t.Run("D2's out-of-domain rows re-asserted at a non-default factor", func(t *testing.T) {
		t.Parallel()
		const factor = 1.3
		for _, attempt := range []int{0, 1, 1000} {
			if got := Exponential(attempt, 0, ceiling, factor); got != 0 {
				t.Errorf("Exponential(%d, 0, %v, %v) = %v, want 0 unchanged", attempt, ceiling, factor, got)
			}
			negBase := -5 * time.Second
			if got := Exponential(attempt, negBase, ceiling, factor); got != negBase {
				t.Errorf("Exponential(%d, %v, %v, %v) = %v, want %v unchanged", attempt, negBase, ceiling, factor, got, negBase)
			}
			if got := Exponential(attempt, base, 0, factor); got != 0 {
				t.Errorf("Exponential(%d, %v, 0, %v) = %v, want 0 unchanged", attempt, base, factor, got)
			}
		}
	})
}

// TestExponential_nonDecreaseAtNonDefaultFactor re-runs the monotonicity
// case at a factor that is not the default and at a factor barely above
// 1, asserting only non-decrease — a strict-increase assertion is
// deliberately never made here, because it would pass at every default
// this tree ships while encoding a claim D20's contract table refutes:
// a small base with a near-1 factor stands still for many attempts
// because the conversion truncates to a nanosecond. Beside it, the
// truncation edge itself: two consecutive attempts at a base and factor
// whose step is below a nanosecond return the SAME delay, asserted as
// equality (design D20).
func TestExponential_nonDecreaseAtNonDefaultFactor(t *testing.T) {
	t.Parallel()

	t.Run("non-decrease, never strict increase", func(t *testing.T) {
		t.Parallel()
		for _, factor := range []float64{1.3, 1 + 1e-9} {
			base, ceiling := 100*time.Millisecond, 2*time.Second
			prev := Exponential(0, base, ceiling, factor)
			for attempt := 1; attempt <= 20; attempt++ {
				cur := Exponential(attempt, base, ceiling, factor)
				if cur < prev {
					t.Fatalf("factor %v: Exponential(%d, ...) = %v is less than Exponential(%d, ...) = %v, want non-decreasing", factor, attempt, cur, attempt-1, prev)
				}
				if cur > ceiling {
					t.Fatalf("factor %v: Exponential(%d, ...) = %v exceeds the ceiling %v", factor, attempt, cur, ceiling)
				}
				prev = cur
			}
		}
	})

	t.Run("truncation edge: consecutive attempts equal, not strictly increasing", func(t *testing.T) {
		t.Parallel()
		// A tiny base and a factor barely above 1 keep the delay's growth
		// well under a nanosecond for the first several attempts, so the
		// time.Duration conversion truncates two consecutive attempts to
		// the same value.
		base, ceiling := time.Nanosecond, time.Second
		const factor = 1 + 1e-12
		a := Exponential(0, base, ceiling, factor)
		b := Exponential(1, base, ceiling, factor)
		if a != b {
			t.Errorf("Exponential(0, ...) = %v, Exponential(1, ...) = %v, want equal (truncation edge)", a, b)
		}
	})
}

// TestExponential_overflowAtNonIntegerFactor is D20's overflow contract:
// at a non-integer factor, rows at attempts far past any doubling
// survives — including math.MaxInt — assert EXACTLY the ceiling and
// strict positivity, never merely "it did not panic", because a wrapped
// time.Duration is a silent negative rather than a crash. If the
// math.MaxInt row times out rather than failing, the implementation has
// reintroduced an O(attempt) loop (design D20).
func TestExponential_overflowAtNonIntegerFactor(t *testing.T) {
	t.Parallel()

	base, ceiling := time.Second, 30*time.Second
	const factor = 1.3
	for _, attempt := range []int{64, 1000, math.MaxInt} {
		got := Exponential(attempt, base, ceiling, factor)
		if got != ceiling {
			t.Errorf("Exponential(%d, %v, %v, %v) = %v, want exactly the ceiling %v", attempt, base, ceiling, factor, got, ceiling)
		}
		if got <= 0 {
			t.Errorf("Exponential(%d, %v, %v, %v) = %v, want strictly positive", attempt, base, ceiling, factor, got)
		}
	}
}

// TestExponential_toleratedNonIntegerFactorValues pins literal durations
// at non-integer factors with the max(1ns, 1e-9*want) tolerance design
// D20 bounds — never a re-computation of Exponential's own formula, which
// would pin nothing (design D20).
func TestExponential_toleratedNonIntegerFactorValues(t *testing.T) {
	t.Parallel()

	base, ceiling := time.Second, time.Hour
	cases := []struct {
		factor  float64
		attempt int
		want    time.Duration
	}{
		{1.3, 1, 1300 * time.Millisecond},
		{1.3, 2, 1690 * time.Millisecond},
		{1.5, 1, 1500 * time.Millisecond},
		{1.5, 3, 3375 * time.Millisecond},
	}
	for _, tc := range cases {
		got := Exponential(tc.attempt, base, ceiling, tc.factor)
		tolerance := time.Duration(math.Max(1, 1e-9*float64(tc.want)))
		if diff := got - tc.want; diff < -tolerance || diff > tolerance {
			t.Errorf("Exponential(%d, %v, %v, %v) = %v, want %v +/- %v", tc.attempt, base, ceiling, tc.factor, got, tc.want, tolerance)
		}
	}
}

// TestEqualJitter_nonIntegerFactorBracket asserts EqualJitter's result
// stays within [Exponential(...)/2, Exponential(...)) — Exponential's own
// return at the same arguments, at a non-integer factor — an inequality,
// so no tolerance is needed (design D20).
func TestEqualJitter_nonIntegerFactorBracket(t *testing.T) {
	t.Parallel()

	base, ceiling := 100*time.Millisecond, 10*time.Second
	const factor = 1.3
	for attempt := 0; attempt <= 10; attempt++ {
		d := Exponential(attempt, base, ceiling, factor)
		for _, jitter := range []float64{0, 0.25, 0.5, 0.75, 0.999999} {
			got := EqualJitter(attempt, base, ceiling, factor, stubJitter(jitter))
			if got < d/2 || got >= d {
				t.Errorf("EqualJitter(%d, ..., jitter=%v) = %v, want within [%v, %v)", attempt, jitter, got, d/2, d)
			}
		}
	}
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

	if got := EqualJitter(0, base, ceiling, 2, stubJitter(0)); got != base/2 {
		t.Errorf("EqualJitter(0, jitter=0) = %v, want %v (half, no jitter added)", got, base/2)
	}
	if got := EqualJitter(0, base, ceiling, 2, stubJitter(1)); got != base {
		t.Errorf("EqualJitter(0, jitter=1) = %v, want %v (half plus the full other half)", got, base)
	}

	// base > ceiling counterpart, at ceiling/2.
	if got := EqualJitter(0, 40*time.Second, ceiling, 2, stubJitter(0)); got != ceiling/2 {
		t.Errorf("EqualJitter(base>ceiling, jitter=0) = %v, want %v", got, ceiling/2)
	}
	if got := EqualJitter(0, 40*time.Second, ceiling, 2, stubJitter(1)); got != ceiling {
		t.Errorf("EqualJitter(base>ceiling, jitter=1) = %v, want %v", got, ceiling)
	}

	// Monotonicity for a fixed jitter.
	prev := EqualJitter(0, base, ceiling, 2, stubJitter(0.5))
	for attempt := 1; attempt <= 10; attempt++ {
		cur := EqualJitter(attempt, base, ceiling, 2, stubJitter(0.5))
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
			got := EqualJitter(tc.attempt, tc.base, tc.ceiling, 2, stubJitter(0.5))
			if got < tc.ceiling/2 || got >= tc.ceiling {
				t.Errorf("EqualJitter(%d, %v, %v) = %v, want within [%v, %v)", tc.attempt, tc.base, tc.ceiling, got, tc.ceiling/2, tc.ceiling)
			}
		})
	}
}
