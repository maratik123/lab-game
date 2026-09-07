// Package backoff implements the exponential ramp `internal/tg`,
// `internal/scheduler` and `internal/ingest` each adopt (design D2, D20,
// issue #22): a delay that grows geometrically from a base by a
// configurable factor, clamped at a ceiling, over a zero-based attempt
// index. Every adopter keeps its own retry loop and its own jitter
// policy — this package owns only the arithmetic, never a timer, a
// context, or a wait.
//
// Contract, over every attempt (including negative ones, clamped to
// zero), every base and ceiling — not only the positive ones the
// adopters' own constructors enforce — and every factor:
//
//   - base > 0, ceiling > 0 and factor strictly greater than 1 (a legal
//     factor per ValidFactor): the result is strictly positive, never
//     exceeds ceiling, and never decreases as attempt grows — though it
//     may hold STEADY across consecutive attempts rather than strictly
//     increase, when base and factor are small enough that the
//     time.Duration conversion truncates two consecutive attempts to the
//     same nanosecond. No time.Duration conversion happens until the
//     value has been proven strictly below ceiling: a finite value at or
//     above the ceiling, +Inf, or NaN each return ceiling itself
//     (already a time.Duration, returned unconverted), so no attempt,
//     however large, can wrap time.Duration's underlying int64 into a
//     negative value.
//   - factor <= 1, or NaN: normalised to exactly 1 before use — the
//     result is base at every attempt, clamped by the ceiling. This is
//     "treated as 1, with no lower clamp", not "not growing" merely by
//     coincidence.
//   - factor == +Inf: base at the zeroth attempt (factor^0 == 1), and
//     ceiling from the first attempt on.
//   - base > ceiling (still both positive, any factor): the result is
//     ceiling, exactly, at every attempt.
//   - base <= 0: the result is base, unchanged, at every attempt — no
//     growth, no lower clamp. This is what every replaced ramp already
//     answered at base == 0; it is deliberately not "the smallest
//     positive delay", because no replaced ramp raises a value, only
//     caps one that has grown too large.
//   - base > 0 and ceiling <= 0: the result is ceiling, unchanged, at
//     every attempt.
//
// None of these out-of-domain rows is reachable through any shipped
// adopter's own constructor, or through internal/config's validation —
// they are decided here so an implementor of a future adopter does not
// have to guess.
//
// A legal factor is finite and strictly greater than 1 — see
// ValidFactor — and DefaultFactor is the value that reproduces the
// doubling ramp every adopter shipped before the factor became
// configurable, exactly, for a base below 2^53 nanoseconds (about 104
// days); above that bound the float64 arithmetic this package now uses
// and the formerly shipped integer doubling diverge by a few
// nanoseconds at most, bounded and still clamped by the ceiling (design
// D20).
package backoff

import (
	"math"
	"time"
)

// DefaultFactor is the exponential ramp's compiled-in growth factor: the
// value Exponential and EqualJitter used before the growth factor became
// configurable (design D20). It reproduces the shipped doubling ramp
// exactly, and every adopter's config default is this constant rather
// than a re-typed literal, so the default and the shared boundary cannot
// drift apart.
const DefaultFactor = 2

// ValidFactor reports whether factor is a legal exponential growth
// factor: finite and strictly greater than 1. This is the single
// definition of that boundary — every adopter's constructor and
// internal/config's reader call it rather than each restating the
// boundary themselves (design D20).
func ValidFactor(factor float64) bool {
	return !math.IsNaN(factor) && !math.IsInf(factor, 0) && factor > 1
}

// Exponential returns the delay before an attempt-th (zero-based) retry:
// min(base*factor^attempt, ceiling), with no time.Duration conversion of
// a value not yet proven strictly below ceiling. See the package comment
// for the full contract, including every out-of-domain row and factor.
func Exponential(attempt int, base, ceiling time.Duration, factor float64) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if base <= 0 {
		return base
	}
	if ceiling <= 0 {
		return ceiling
	}
	if math.IsNaN(factor) || factor <= 1 {
		factor = 1
	}
	d := float64(base) * math.Pow(factor, float64(attempt))
	if math.IsNaN(d) || d >= float64(ceiling) {
		return ceiling
	}
	return time.Duration(d)
}

// EqualJitter returns Exponential's delay for attempt split into an
// equal-jitter draw: half the delay, plus a further draw of up to that
// same half, scaled by jitter() in [0, 1). Bounded within
// [Exponential(...)/2, Exponential(...)) for a jitter in [0, 1) — the
// same formula internal/tg's retry loop used before this package existed.
func EqualJitter(attempt int, base, ceiling time.Duration, factor float64, jitter func() float64) time.Duration {
	d := Exponential(attempt, base, ceiling, factor)
	half := d / 2
	return half + time.Duration(float64(half)*jitter())
}
