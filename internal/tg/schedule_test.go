package tg

import (
	"math/rand/v2"
	"testing"
	"time"
)

var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// bruteForceEarliest is the oracle earliest is checked against: it scans
// forward in fixed steps from candidate until it finds an instant at
// which holds(t) is true, which is correct by the same reasoning as
// earliest's own doc comment (the invariant can only change at a grant's
// window boundary) but much slower — fine for a small, bounded search in
// a test.
func bruteForceEarliest(s *schedule, candidate time.Time) time.Time {
	if s.kind == orderedSchedule && len(s.grants) > 0 {
		if last := s.grants[len(s.grants)-1]; last.After(candidate) {
			candidate = last
		}
	}
	const step = time.Millisecond
	t := candidate
	for i := 0; i < 1_000_000; i++ {
		if s.holds(t) {
			return t
		}
		t = t.Add(step)
	}
	panic("bruteForceEarliest: did not converge")
}

// TestSchedule_EarliestMatchesBruteForceOracle asserts schedule.earliest
// against a slow, independent oracle over many small random scenarios
// (design D9's "the mechanism is correct by a checkable postcondition,
// not by an argument in this document").
func TestSchedule_EarliestMatchesBruteForceOracle(t *testing.T) {
	t.Parallel()
	rnd := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 200; trial++ {
		kind := orderedSchedule
		if rnd.IntN(2) == 0 {
			kind = unorderedSchedule
		}
		windows := []window{{count: 1 + rnd.IntN(3), per: time.Duration(1+rnd.IntN(5)) * time.Second}}
		if rnd.IntN(2) == 0 {
			windows = append(windows, window{count: 1 + rnd.IntN(5), per: time.Duration(5+rnd.IntN(20)) * time.Second})
		}
		s := newSchedule(kind, windows)
		oracle := newSchedule(kind, windows)

		base := epoch
		for i := 0; i < 5; i++ {
			candidate := base.Add(time.Duration(rnd.IntN(10)) * time.Second)
			got := s.earliest(candidate)
			want := bruteForceEarliest(oracle, candidate)
			if !got.Equal(want) {
				t.Fatalf("trial %d step %d: earliest(%v) = %v, want %v (windows=%v, grants=%v)",
					trial, i, candidate.Sub(epoch), got.Sub(epoch), want.Sub(epoch), windows, s.grants)
			}
			s.commit(got)
			oracle.commit(want)
			base = got
		}
	}
}

// TestSchedule_InvariantHoldsAfterEveryCommit asserts D9's stated
// invariant directly, in the form the design says a checkable
// postcondition takes: for every window (c, per), grant[j+c]-grant[j] >=
// per for every j.
func TestSchedule_InvariantHoldsAfterEveryCommit(t *testing.T) {
	t.Parallel()
	windows := []window{{count: 3, per: 10 * time.Second}, {count: 1, per: 2 * time.Second}}
	s := newSchedule(unorderedSchedule, windows)
	base := epoch
	for i := 0; i < 50; i++ {
		t2 := s.earliest(base)
		s.commit(t2)
		assertInvariant(t, s, windows)
		base = base.Add(time.Second)
	}
}

func assertInvariant(t *testing.T, s *schedule, windows []window) {
	t.Helper()
	for _, w := range windows {
		for j := 0; j+w.count < len(s.grants); j++ {
			gap := s.grants[j+w.count].Sub(s.grants[j])
			if gap < w.per {
				t.Fatalf("invariant violated: window(%d,%v): grants[%d..%d] = %v..%v, gap %v < per %v",
					w.count, w.per, j, j+w.count, s.grants[j], s.grants[j+w.count], gap, w.per)
			}
		}
	}
}

// TestSchedule_OrderedEmissionIsNonDecreasing asserts the ordered kind's
// non-decreasing property directly (design D9's table): a key's grants
// never go backwards, so emission order is arrival order.
func TestSchedule_OrderedEmissionIsNonDecreasing(t *testing.T) {
	t.Parallel()
	s := newSchedule(orderedSchedule, []window{{count: 1, per: time.Second}})
	candidates := []time.Time{
		epoch.Add(5 * time.Second),
		epoch, // arrives "later" logically but with an earlier candidate instant
		epoch.Add(2 * time.Second),
	}
	var last time.Time
	for i, c := range candidates {
		got := s.earliest(c)
		if i > 0 && got.Before(last) {
			t.Fatalf("step %d: earliest returned %v, before the previous grant %v", i, got, last)
		}
		s.commit(got)
		last = got
	}
}

// TestSchedule_UnboundedContributesNoWindow asserts design D9's "an
// unbounded value contributes no window": a schedule with no windows never
// blocks.
func TestSchedule_UnboundedContributesNoWindow(t *testing.T) {
	t.Parallel()
	s := newSchedule(unorderedSchedule, nil)
	t2 := s.earliest(epoch)
	if !t2.Equal(epoch) {
		t.Errorf("earliest(epoch) = %v, want epoch (no delay)", t2)
	}
	s.commit(t2)
	if len(s.grants) != 0 {
		t.Errorf("grants = %v, want none retained for an unbounded schedule", s.grants)
	}
}

// TestSchedule_RetentionIsTimeBasedNotCountBased asserts design D9's
// retention rule directly on an UNORDERED schedule, where a count bound
// would be unsound: grants still inside a live window must never be
// evicted merely for being "not the newest c".
func TestSchedule_RetentionIsTimeBasedNotCountBased(t *testing.T) {
	t.Parallel()
	windows := []window{{count: 2, per: 10 * time.Second}}
	s := newSchedule(unorderedSchedule, windows)
	// Three grants spread out so a naive "keep newest 2" would drop the
	// oldest even though it is still inside the 10s window of the third.
	s.commit(epoch)
	s.commit(epoch.Add(1 * time.Second))
	// A "keep newest max(c)=2" retention would now have discarded epoch.
	if len(s.grants) != 2 {
		t.Fatalf("grants = %v, want 2 retained (both still inside the 10s window)", s.grants)
	}
	// A third grant within the window must still be refused/delayed past
	// the window, proving the first grant is still remembered.
	got := s.earliest(epoch.Add(2 * time.Second))
	want := epoch.Add(10 * time.Second) // epoch + per, since count=2 already occupies [epoch, epoch+1s]
	if !got.Equal(want) {
		t.Errorf("earliest = %v, want %v (the oldest grant must still be remembered)", got, want)
	}
}

// TestSchedule_EvictIsRelativeToRealNowNotToAFutureGrant asserts the fix
// this design's own history warns about: eviction must never be computed
// against a newly-decided FUTURE grant instant, only against the real
// clock. A burst of calls, all decided from one real "now", must not have
// an early member evicted merely because a later member's grant lands far
// in the future — that would let the early member's slot be reused within
// the SAME burst, silently over-emitting.
func TestSchedule_EvictIsRelativeToRealNowNotToAFutureGrant(t *testing.T) {
	t.Parallel()
	s := newSchedule(unorderedSchedule, []window{{count: 1, per: time.Second}})
	// A burst of 3 calls, all arriving at the same real instant (epoch),
	// decided one after another without evict ever being called mid-burst
	// (as Limiter.acquire does: evict once per acquire, using the real
	// "now" passed in, never the grant it is about to commit).
	var grants []time.Time
	for i := 0; i < 3; i++ {
		t2 := s.earliest(epoch)
		s.commit(t2)
		grants = append(grants, t2)
	}
	assertInvariant(t, s, s.windows)
	if !windowRespected(grants, window{count: 1, per: time.Second}) {
		t.Fatalf("burst grants %v violate window(1,1s)", grants)
	}

	// Only NOW does real time actually advance (a later, genuinely new
	// call arrives at epoch+1s) — eviction may run, but must not have run
	// early during the burst above.
	s.evict(epoch.Add(1 * time.Second))
	got := s.earliest(epoch.Add(1 * time.Second))
	if got.Before(grants[len(grants)-1]) {
		t.Errorf("earliest after real-time eviction = %v, must not be before the burst's last real grant %v", got, grants[len(grants)-1])
	}
}

// TestSchedule_EvictThreshold asserts evict's retention boundary
// directly (finding 10 — evict had no test at all, so a weakened
// threshold or a flipped comparison left the suite green). Three checks,
// each targeting a specific named mutant:
//  1. Exactly at the boundary (now-maxPer == grant), the grant must be
//     RETAINED — Before is a strict comparison.
//  2. Just past the boundary, it must be EVICTED. Together, 1 and 2
//     distinguish "grants[i].Before(threshold)" from a flipped
//     "!grants[i].After(threshold)" — the two disagree only exactly at
//     the boundary.
//  3. A grant evicted at now+3*maxPer catches a weakened threshold of
//     now-2*maxPer, which check 2 alone would not.
func TestSchedule_EvictThreshold(t *testing.T) {
	t.Parallel()
	s := newSchedule(unorderedSchedule, []window{{count: 1, per: time.Second}})

	s.commit(epoch)
	s.evict(epoch.Add(time.Second)) // now - maxPer == epoch, exactly the grant's own instant
	if len(s.grants) != 1 {
		t.Fatalf("grants = %d after evict at the exact boundary, want 1 (retained)", len(s.grants))
	}

	s.evict(epoch.Add(time.Second + time.Nanosecond)) // just past the boundary
	if len(s.grants) != 0 {
		t.Fatalf("grants = %d after evict just past the boundary, want 0 (evicted)", len(s.grants))
	}

	s.commit(epoch)
	s.evict(epoch.Add(3 * time.Second)) // well past maxPer; catches a weakened now-2*maxPer threshold
	if len(s.grants) != 0 {
		t.Errorf("grants = %d after evict far past maxPer, want 0 (bounded retention)", len(s.grants))
	}
}

// TestSchedule_EvictBoundsMemoryAcrossManyAcquires asserts D9's bounded-
// memory claim directly: a long sequence of real-time-separated acquires
// (each evicted before the next) must never accumulate an unbounded
// grant history.
func TestSchedule_EvictBoundsMemoryAcrossManyAcquires(t *testing.T) {
	t.Parallel()
	s := newSchedule(unorderedSchedule, []window{{count: 1, per: time.Second}})
	now := epoch
	for i := 0; i < 1000; i++ {
		s.evict(now)
		t2 := s.earliest(now)
		s.commit(t2)
		if len(s.grants) > 1 {
			t.Fatalf("step %d: grants = %d, want at most 1 (bounded memory)", i, len(s.grants))
		}
		now = now.Add(2 * time.Second) // well past maxPer=1s before the next acquire
	}
}
