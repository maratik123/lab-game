// Package backoff implements the exponential ramp `internal/tg` and
// `internal/scheduler` each duplicated (design D2, issue #22): a delay
// that doubles from a base, clamped at a ceiling, over a zero-based
// attempt index. Both adopters keep their own retry loop and their own
// jitter policy — this package owns only the arithmetic, never a
// timer, a context, or a wait.
//
// Contract, over every attempt (including negative ones, clamped to
// zero) and every base and ceiling — not only the positive ones the
// adopters' own constructors enforce:
//
//   - base > 0 and ceiling > 0: the result is strictly positive, never
//     exceeds ceiling, and never decreases as attempt grows. The ceiling
//     is tested and clamped BEFORE the doubling that would overflow it,
//     so no attempt, however large, can wrap time.Duration's underlying
//     int64 into a negative value.
//   - base > ceiling (still both positive): the result is ceiling,
//     exactly, at every attempt — the post-loop clamp neither replaced
//     ramp skips.
//   - base <= 0: the result is base, unchanged, at every attempt — no
//     doubling, no lower clamp. This is what both replaced ramps already
//     answer at base == 0; it is deliberately not "the smallest positive
//     delay", because neither replaced ramp raises a value, only caps
//     one that has grown too large.
//   - base > 0 and ceiling <= 0: the result is ceiling, unchanged, at
//     every attempt.
//
// None of these out-of-domain rows is reachable through either shipped
// adopter's own constructor, or through internal/ingest's or
// internal/config's validation — they are decided here so an
// implementor of a future adopter does not have to guess.
//
// The ramp's growth factor is becoming configurable (design D20): a
// legal factor is finite and strictly greater than 1 — see ValidFactor —
// and DefaultFactor is the value that reproduces the doubling ramp
// documented above exactly.
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
// min(base*2^attempt, ceiling), clamped before the doubling that would
// overflow time.Duration's underlying int64. See the package comment for
// the full contract, including every out-of-domain row.
func Exponential(attempt int, base, ceiling time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if base <= 0 {
		return base
	}
	if ceiling <= 0 {
		return ceiling
	}
	d := base
	for range attempt {
		if d >= ceiling {
			return ceiling
		}
		d *= 2
	}
	if d > ceiling {
		return ceiling
	}
	return d
}

// EqualJitter returns Exponential's delay for attempt split into an
// equal-jitter draw: half the delay, plus a further draw of up to that
// same half, scaled by jitter() in [0, 1). Bounded within
// [Exponential(...)/2, Exponential(...)) for a jitter in [0, 1) — the
// same formula internal/tg's retry loop used before this package existed.
func EqualJitter(attempt int, base, ceiling time.Duration, jitter func() float64) time.Duration {
	d := Exponential(attempt, base, ceiling)
	half := d / 2
	return half + time.Duration(float64(half)*jitter())
}
