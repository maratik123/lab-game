package scheduler

import "time"

// Cadence computes a recurrence's next occurrence given its previous
// run_at (prev) and the instant now — both DB-supplied, never
// time.Now (design D3). A function type rather than a fixed duration, so
// a mechanic that needs a wall-clock anchor can supply its own without
// touching this package.
type Cadence func(prev, now time.Time) time.Time

// Every returns the one Cadence this task ships: a fixed period. The
// result is the smallest occurrence prev + k*period (k >= 1) strictly
// after now — never "one period after prev" — so a recurrence that
// missed several periods (an outage) catches up to exactly one future
// occurrence rather than firing once per missed period.
func Every(period time.Duration) Cadence {
	return func(prev, now time.Time) time.Time {
		elapsed := now.Sub(prev)
		if elapsed < 0 {
			return prev.Add(period)
		}
		// The number of whole periods elapsed, plus one, lands on the
		// smallest prev + k*period strictly after now — the catch-up
		// rule: one future occurrence, not a backlog of missed ones. The
		// multiplication is done in plain int64 (nanoseconds), not as a
		// Duration*Duration product, which durationcheck rightly treats
		// as a near-always-wrong shape.
		k := int64(elapsed/period) + 1
		return prev.Add(time.Duration(k * int64(period)))
	}
}

// backoff returns the delay before a one-shot or recurrent task's next
// attempt after failures consecutive failures: base*2^(failures-1),
// capped at ceiling. Pure, strictly positive for failures >= 1, and
// strictly growing until it reaches ceiling — never jittered, because
// SKIP LOCKED already de-collides workers and a deterministic delay is
// exactly assertable (design D8).
func backoff(failures int, base, ceiling time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	d := base
	for range failures - 1 {
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
