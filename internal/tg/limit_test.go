package tg

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

func rate(count int, per time.Duration) config.Rate { return config.Rate{Count: count, Per: per} }

// acquireOK calls l.acquire and fails the test immediately if it reports
// the fixed-point-exhaustion defect error (finding 12) — none of these
// tests are constructed to hit it, so seeing that error here would itself
// be a finding.
func acquireOK(t *testing.T, l *Limiter, call Call, now, deadline time.Time, hasDeadline bool) (time.Time, bool) {
	t.Helper()
	tm, ok, err := l.acquire(call, now, deadline, hasDeadline)
	if err != nil {
		t.Fatalf("acquire: unexpected error: %v", err)
	}
	return tm, ok
}

func chatCall(class MethodClass, key string) Call {
	return Call{Method: "x", Class: class, Chat: ChatRef{Key: key, Target: ChatKnown}}
}

func noChatCall(class MethodClass) Call {
	return Call{Method: "x", Class: class, Chat: ChatRef{Target: ChatNone}}
}

func unknownChatCall(class MethodClass) Call {
	return Call{Method: "x", Class: class, Chat: ChatRef{Target: ChatUnknown}}
}

// windowRespected reports whether times (unsorted) satisfy w's invariant:
// no half-open interval of length w.per contains more than w.count of
// them.
func windowRespected(times []time.Time, w window) bool {
	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })
	for j := 0; j+w.count < len(sorted); j++ {
		if sorted[j+w.count].Sub(sorted[j]) < w.per {
			return false
		}
	}
	return true
}

// ---- Functional Limiter-level tests (AC10-AC15, AC30-AC33) ----

func TestLimiter_PerClassGlobalAdmission(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{Global: rate(3, time.Second)}}
	l := newLimiter(limits)
	var times []time.Time
	for i := 0; i < 6; i++ {
		tm, ok := acquireOK(t, l, noChatCall(ClassMessage), epoch, time.Time{}, false)
		if !ok {
			t.Fatalf("acquire[%d]: refused unexpectedly", i)
		}
		times = append(times, tm)
	}
	if !windowRespected(times, window{count: 3, per: time.Second}) {
		t.Errorf("global window(3,1s) violated by emissions %v", times)
	}
	// A call of an unbounded class must not be delayed by the message
	// class's global allowance (AC10's second clause).
	tm, ok := acquireOK(t, l, noChatCall(ClassOther), epoch, time.Time{}, false)
	if !ok || !tm.Equal(epoch) {
		t.Errorf("ClassOther acquire = (%v, %v), want (epoch, true) — unaffected by ClassMessage's global allowance", tm, ok)
	}
}

func TestLimiter_CrossChatNonBlockingWithGlobalBounded(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{
		Global:   rate(100, time.Second), // generous, bounded (AC11 requires the global class bounded)
		ChatRate: rate(1, time.Second),
		ChatCap:  rate(20, time.Minute),
	}}
	l := newLimiter(limits)
	// Chat A gets a heavy backlog.
	for i := 0; i < 5; i++ {
		if _, ok := acquireOK(t, l, chatCall(ClassMessage, "A"), epoch, time.Time{}, false); !ok {
			t.Fatalf("chat A acquire[%d]: refused", i)
		}
	}
	// Chat B, arriving at the same candidate, must not be pushed behind
	// chat A's backlog.
	tm, ok := acquireOK(t, l, chatCall(ClassMessage, "B"), epoch, time.Time{}, false)
	if !ok {
		t.Fatal("chat B acquire: refused unexpectedly")
	}
	if tm.After(epoch.Add(200 * time.Millisecond)) {
		t.Errorf("chat B granted %v, want near epoch (not blocked behind chat A's backlog)", tm.Sub(epoch))
	}
}

func TestLimiter_UndecodableBodyIsBoundedNotExempt(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}
	l := newLimiter(limits)
	var times []time.Time
	for i := 0; i < 4; i++ {
		tm, ok := acquireOK(t, l, unknownChatCall(ClassMessage), epoch, time.Time{}, false)
		if !ok {
			t.Fatalf("acquire[%d]: refused", i)
		}
		times = append(times, tm)
	}
	if !windowRespected(times, window{count: 1, per: time.Second}) {
		t.Errorf("ChatUnknown's shared reserved key must still be bounded: %v", times)
	}
}

func TestLimiter_UnboundedClassPassesThroughAndBoundCounterpartBinds(t *testing.T) {
	t.Parallel()
	// Other is unbounded by default: many calls at once, no delay.
	l := newLimiter(config.TransportLimits{})
	for i := 0; i < 10; i++ {
		tm, ok := acquireOK(t, l, chatCall(ClassOther, "chat"), epoch, time.Time{}, false)
		if !ok || !tm.Equal(epoch) {
			t.Fatalf("unbounded acquire[%d] = (%v,%v), want (epoch,true)", i, tm, ok)
		}
	}

	// Its bound counterpart: configuring a ChatRate for Other makes it bind.
	bounded := newLimiter(config.TransportLimits{Other: config.ClassLimits{ChatRate: rate(1, time.Second)}})
	first, ok := acquireOK(t, bounded, chatCall(ClassOther, "chat"), epoch, time.Time{}, false)
	if !ok {
		t.Fatal("bounded acquire[0]: refused")
	}
	second, ok := acquireOK(t, bounded, chatCall(ClassOther, "chat"), epoch, time.Time{}, false)
	if !ok {
		t.Fatal("bounded acquire[1]: refused")
	}
	if !second.After(first) {
		t.Errorf("second = %v, first = %v: configuring a bound must make it bind", second, first)
	}
}

// TestLimiter_ClassLimitsRoutesEachClassToItsOwnConfig asserts self-review
// round 5 finding 2: Limiter.classLimits' ClassEdit arm was never
// exercised by any test — flipping "case ClassEdit: return l.limits.Edit"
// to "return l.limits.Other" left the suite green, because every prior
// test left Edit and Other at the same zero ClassLimits. This test
// configures the two classes with distinct, distinguishable ChatRates so
// an edit-class call and an other-class call bind on different
// intervals, and table-drives over all three MethodClass values so
// ClassMessage's existing coverage does not regress silently either.
func TestLimiter_ClassLimitsRoutesEachClassToItsOwnConfig(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{
		Message: config.ClassLimits{ChatRate: rate(1, 1*time.Second)},
		Edit:    config.ClassLimits{ChatRate: rate(1, 2*time.Second)},
		Other:   config.ClassLimits{ChatRate: rate(1, 3*time.Second)},
	}
	cases := []struct {
		name  string
		class MethodClass
		want  time.Duration
	}{
		{"message", ClassMessage, 1 * time.Second},
		{"edit", ClassEdit, 2 * time.Second},
		{"other", ClassOther, 3 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ll := newLimiter(limits)
			key := chatCall(tc.class, tc.name)
			first, ok := acquireOK(t, ll, key, epoch, time.Time{}, false)
			if !ok {
				t.Fatalf("acquire[0]: refused")
			}
			second, ok := acquireOK(t, ll, key, epoch, time.Time{}, false)
			if !ok {
				t.Fatalf("acquire[1]: refused")
			}
			if got := second.Sub(first); got != tc.want {
				t.Errorf("second-first = %v, want exactly %v (classLimits must route %s to its own configured ChatRate, not another class's)", got, tc.want, tc.class)
			}
		})
	}
}

// TestLimiter_UnboundedClassNeverAllocatesPerChatSchedule asserts D9's
// registry claim directly: an unbounded class (no ChatRate, no ChatCap)
// must not accumulate one map entry per distinct chat id — a schedule
// with no windows never blocks, so storing one is pure unbounded growth.
func TestLimiter_UnboundedClassNeverAllocatesPerChatSchedule(t *testing.T) {
	t.Parallel()
	l := newLimiter(config.TransportLimits{})
	for i := 0; i < 10; i++ {
		if _, ok := acquireOK(t, l, chatCall(ClassOther, fmt.Sprintf("chat-%d", i)), epoch, time.Time{}, false); !ok {
			t.Fatalf("acquire[%d]: refused", i)
		}
	}
	if len(l.chats) != 0 {
		t.Errorf("len(l.chats) = %d, want 0 (an unbounded class must allocate no per-chat schedule)", len(l.chats))
	}
}

func TestLimiter_PrivateChatIsChargedLikeAnyOther(t *testing.T) {
	t.Parallel()
	// The mechanism has no chat-type branch at all — a "private" chat is
	// simply another Key. This test exercises that key and shows it is
	// bounded exactly like the cross-chat test's "A"/"B" keys (AC14).
	limits := config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}
	l := newLimiter(limits)
	first, ok := acquireOK(t, l, chatCall(ClassMessage, "private:42"), epoch, time.Time{}, false)
	if !ok {
		t.Fatal("acquire[0]: refused")
	}
	second, ok := acquireOK(t, l, chatCall(ClassMessage, "private:42"), epoch, time.Time{}, false)
	if !ok {
		t.Fatal("acquire[1]: refused")
	}
	if second.Sub(first) < time.Second {
		t.Errorf("second-first = %v, want >= 1s (private chat must be bounded)", second.Sub(first))
	}
}

func TestLimiter_SteadyOrderedEmission(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}
	l := newLimiter(limits)
	var times []time.Time
	for i := 0; i < 5; i++ {
		tm, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false)
		if !ok {
			t.Fatalf("acquire[%d]: refused", i)
		}
		times = append(times, tm)
	}
	for i := 1; i < len(times); i++ {
		if !times[i].After(times[i-1]) {
			t.Fatalf("emission %d (%v) not after emission %d (%v): arrival order must be preserved", i, times[i], i-1, times[i-1])
		}
		if times[i].Sub(times[i-1]) < time.Second {
			t.Errorf("spacing[%d] = %v, want >= 1s", i, times[i].Sub(times[i-1]))
		}
	}
}

// TestLimiter_LaterArrivalDoesNotJumpAnEarlierGrant is R3 finding 1
// (self-review round 3): AC30's "in the order they arrived" clause depends
// entirely on chatScheduleLocked building the per-chat schedule as
// orderedSchedule (limit.go), which clamps each candidate to be no earlier
// than the chat's own newest grant. Under the shipped chat windows
// (ChatRate 1/1s, ChatCap 20/1m) — with grants already committed at epoch
// and epoch+10s — a third acquire call whose own candidate ("now") is
// epoch+1s must still be granted AFTER the epoch+10s grant, because it
// arrives (is acquired) third: an unordered schedule has nothing stopping
// a 1s-spaced insertion between the two existing grants, which would let
// this later arrival jump ahead of the already-granted epoch+10s call.
// TestLimiter_SteadyOrderedEmission cannot see this because it configures
// ChatRate alone, where the pace and bound windows coincide and the two
// schedule kinds behave identically.
func TestLimiter_LaterArrivalDoesNotJumpAnEarlierGrant(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{
		ChatRate: rate(1, time.Second),
		ChatCap:  rate(20, time.Minute),
	}}
	l := newLimiter(limits)

	first, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false)
	if !ok || !first.Equal(epoch) {
		t.Fatalf("acquire[0] = (%v,%v), want (epoch,true)", first, ok)
	}
	second, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch.Add(10*time.Second), time.Time{}, false)
	if !ok || !second.Equal(epoch.Add(10*time.Second)) {
		t.Fatalf("acquire[1] = (%v,%v), want (epoch+10s,true)", second, ok)
	}

	// Arrives (is acquired) last, but its own candidate is earlier than the
	// second grant.
	third, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch.Add(time.Second), time.Time{}, false)
	if !ok {
		t.Fatal("acquire[2]: refused unexpectedly")
	}
	if !third.After(second) {
		t.Errorf("third arrival granted %v, not after second grant %v: a later arrival must not jump an already-granted call (AC30)", third, second)
	}
}

func TestLimiter_BurstThenCapShape(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{
		ChatRate: rate(1, time.Second),
		ChatCap:  rate(3, 5*time.Second),
	}}
	l := newLimiter(limits)
	var times []time.Time
	for i := 0; i < 6; i++ {
		tm, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false)
		if !ok {
			t.Fatalf("acquire[%d]: refused", i)
		}
		times = append(times, tm)
	}
	if !windowRespected(times, window{count: 1, per: time.Second}) {
		t.Errorf("chat rate window(1,1s) violated: %v", times)
	}
	if !windowRespected(times, window{count: 3, per: 5 * time.Second}) {
		t.Errorf("chat cap window(3,5s) violated: %v", times)
	}
}

func TestLimiter_RefusalLeavesScheduleUnchanged(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}
	l := newLimiter(limits)
	if _, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false); !ok {
		t.Fatal("acquire[0]: refused unexpectedly")
	}

	key := chatKey{key: "chat", class: ClassMessage}
	before := append([]time.Time(nil), l.chats[key].grants...)

	// The next slot is epoch+1s; an impossible deadline must refuse
	// without mutating anything.
	if _, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, epoch.Add(10*time.Millisecond), true); ok {
		t.Fatal("acquire[1]: expected refusal (deadline unreachable)")
	}

	after := l.chats[key].grants
	if len(before) != len(after) {
		t.Fatalf("grants changed after a refusal: before=%v after=%v", before, after)
	}
	for i := range before {
		if !before[i].Equal(after[i]) {
			t.Fatalf("grants changed after a refusal: before=%v after=%v", before, after)
		}
	}
}

func TestLimiter_SaturationEndsInEmissionOrError(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}
	l := newLimiter(limits)
	deadline := epoch.Add(3*time.Second + 500*time.Millisecond)
	granted, refused := 0, 0
	for i := 0; i < 10; i++ {
		if _, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), epoch, deadline, true); ok {
			granted++
		} else {
			refused++
		}
	}
	if granted+refused != 10 {
		t.Fatalf("granted+refused = %d, want 10 (every call must end in one or the other)", granted+refused)
	}
	if granted == 0 || refused == 0 {
		t.Errorf("granted=%d refused=%d, want both under saturation with a tight deadline", granted, refused)
	}
}

// TestLimiter_EvictRunsInsideAcquireBoundingMemory is self-review round
// 4 finding 1's fix: D9's bounded-retention claim ("retention is
// O(calls in flight), bounded by concurrency rather than by the window
// set alone") is a claim about Limiter.acquire, the only production
// caller of schedule.evict — not about schedule.evict in isolation.
// TestSchedule_EvictBoundsMemoryAcrossManyAcquires (schedule_test.go)
// hand-rolls s.evict/s.earliest/s.commit on a bare *schedule and never
// calls Limiter.acquire, so it cannot see acquire's two evict(now) call
// sites (limit.go: global.evict(now) and, when chat != nil,
// chat.evict(now)) go missing. This test drives real-time-separated
// calls through l.acquire itself — under the shipped message defaults'
// shape (Global 30/1s, ChatRate 1/1s, ChatCap 20/1m) — and asserts both
// the class-global schedule's and the per-chat schedule's retained
// grant counts stay small and bounded, never growing with the number of
// calls made.
//
// Mutation-verified: deleting either `global.evict(now)` (limit.go:318)
// or the `if chat != nil { chat.evict(now) }` block (limit.go:319-321)
// makes this test fail — grants then accumulate one per call (500,
// unbounded) instead of staying within the asserted bound.
func TestLimiter_EvictRunsInsideAcquireBoundingMemory(t *testing.T) {
	t.Parallel()
	limits := config.TransportLimits{Message: config.ClassLimits{
		Global:   rate(30, time.Second),
		ChatRate: rate(1, time.Second),
		ChatCap:  rate(20, time.Minute),
	}}
	l := newLimiter(limits)
	key := chatKey{key: "chat", class: ClassMessage}

	now := epoch
	const iterations = 500
	const warmup = 20 // let the per-chat 1-minute cap window fill once before asserting.
	for i := 0; i < iterations; i++ {
		if _, ok := acquireOK(t, l, chatCall(ClassMessage, "chat"), now, time.Time{}, false); !ok {
			t.Fatalf("acquire[%d]: refused unexpectedly", i)
		}
		if i >= warmup {
			if got := len(l.global[ClassMessage].grants); got > 5 {
				t.Fatalf("step %d: global grants retained = %d, want a small bounded count — evict(now) must run inside Limiter.acquire (design D9)", i, got)
			}
			chatGrants := len(l.chats[key].grants)
			if chatGrants > 15 {
				t.Fatalf("step %d: chat grants retained = %d, want a small bounded count — evict(now) must run inside Limiter.acquire (design D9)", i, chatGrants)
			}
		}
		now = now.Add(10 * time.Second) // real time advances well past every window's maxPer before the next acquire.
	}
}

// TestLimiter_IdenticalBehaviourAcrossBaseURLs builds two real Clients
// through New, differing ONLY in BaseURL, and asserts their limiters
// produce byte-identical acquire results (AC15) — unlike a test that
// compares two Limiters built with no BaseURL anywhere in the picture,
// this one actually varies the field AC15 names.
func TestLimiter_IdenticalBehaviourAcrossBaseURLs(t *testing.T) {
	t.Parallel()
	tr := validTransport()
	tr.Limits = config.TransportLimits{Message: config.ClassLimits{ChatRate: rate(1, time.Second)}}

	c1, err := New(Options{BaseURL: "https://api.telegram.org", Token: tgtest.Token, Transport: tr})
	if err != nil {
		t.Fatalf("New (BaseURL 1): %v", err)
	}
	c2, err := New(Options{BaseURL: "https://self-hosted.bot-api.invalid", Token: tgtest.Token, Transport: tr})
	if err != nil {
		t.Fatalf("New (BaseURL 2): %v", err)
	}

	for i := 0; i < 4; i++ {
		t1, ok1 := acquireOK(t, c1.limiter, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false)
		t2, ok2 := acquireOK(t, c2.limiter, chatCall(ClassMessage, "chat"), epoch, time.Time{}, false)
		if ok1 != ok2 || !t1.Equal(t2) {
			t.Fatalf("acquire[%d] diverged across BaseURLs: (%v,%v) vs (%v,%v)", i, t1, ok1, t2, ok2)
		}
	}
}

// ---- Subtask 4's binding red-first broken-variant table ----
//
// Each variant below is a small, self-contained reproduction of a named
// historical defect (design D9's history table). Every test runs the
// SAME assertion against the broken variant FIRST (asserting it fails —
// RED) and then against the real Limiter/schedule (asserting it passes —
// GREEN). A row whose broken variant does not go red is itself a finding
// (design's spawn-contract note) — none of the seven below stayed green.

// Row 1: commit-at-candidate — commit to the global schedule at the
// candidate instant, then let a chat window push the emission later
// (round-3b: a bound that holds on reservations, not emissions).
func TestRedFirst_CommitAtCandidate(t *testing.T) {
	t.Parallel()
	globalWindow := window{count: 1, per: time.Second}

	brokenAcquire := func(global, chatBusy *schedule, candidate time.Time) time.Time {
		t2 := global.earliest(candidate)
		global.commit(t2) // BUG: commits at the candidate, before the chat can push it later.
		if c := chatBusy.earliest(t2); c.After(t2) {
			t2 = c
		}
		chatBusy.commit(t2)
		return t2
	}

	// Broken: RED. The busy chat already has a backlog reaching to
	// epoch+104s (window count=1 per=100s, ordered — its next grant must
	// land 100s after that). Under commit-at-candidate, the busy call's
	// TRUE emission (epoch+104s) is never recorded on the global schedule
	// — only its candidate (epoch) is — so a later, unrelated call
	// legitimately requesting the global schedule's next free instant
	// near epoch+104s is wrongly admitted there too: two calls emitting
	// at the same real instant on a global window that allows only one
	// per second.
	{
		global := newSchedule(unorderedSchedule, []window{globalWindow})
		busyChat := newSchedule(orderedSchedule, []window{{count: 1, per: 100 * time.Second}})
		freeChat := newSchedule(orderedSchedule, nil)
		busyChat.commit(epoch.Add(4 * time.Second))

		var emissions []time.Time
		emissions = append(emissions, brokenAcquire(global, busyChat, epoch)) // true emission: epoch+104s.
		for i := 0; i < 5; i++ {
			c := epoch.Add(time.Duration(i+1) * 100 * time.Millisecond)
			emissions = append(emissions, brokenAcquire(global, freeChat, c))
		}
		// A 7th call, arriving right around the busy call's TRUE emission
		// instant, with its own free chat.
		emissions = append(emissions, brokenAcquire(global, newSchedule(orderedSchedule, nil), epoch.Add(104*time.Second)))

		if windowRespected(emissions, globalWindow) {
			t.Fatalf("RED expected: commit-at-candidate must violate the global window, got %v", emissions)
		}
	}

	// Real mechanism: GREEN — the identical arrival sequence through
	// Limiter.acquire, which always commits at the final decided instant.
	{
		l := newLimiter(config.TransportLimits{Message: config.ClassLimits{Global: rate(1, time.Second)}})
		var emissions []time.Time
		tm, ok := acquireOK(t, l, chatCall(ClassMessage, "busy"), epoch, time.Time{}, false)
		if !ok {
			t.Fatal("real acquire (busy chat): refused")
		}
		emissions = append(emissions, tm)
		for i := 0; i < 5; i++ {
			c := epoch.Add(time.Duration(i+1) * 100 * time.Millisecond)
			tm, ok := acquireOK(t, l, chatCall(ClassMessage, "free"), c, time.Time{}, false)
			if !ok {
				t.Fatalf("real acquire (free chat, %d): refused", i)
			}
			emissions = append(emissions, tm)
		}
		tm, ok = acquireOK(t, l, chatCall(ClassMessage, "free2"), epoch.Add(104*time.Second), time.Time{}, false)
		if !ok {
			t.Fatal("real acquire (7th call): refused")
		}
		emissions = append(emissions, tm)
		if !windowRespected(emissions, globalWindow) {
			t.Errorf("GREEN expected: the real decide-then-commit mechanism must respect the global window, got %v", emissions)
		}
	}
}

// Row 2: commit-then-undo — commit the grant, then remove it when the
// deadline check refuses (round-3a: a refusal that corrupts the window's
// memory).
func TestRedFirst_CommitThenUndo(t *testing.T) {
	t.Parallel()
	w := window{count: 5, per: 10 * time.Second}

	// Broken: an optimistic-commit-then-naive-undo schedule, modelling
	// the abandoned composed-leg architecture. Undo pops the LAST element
	// of the insertion-order log — correct only when nothing else
	// committed in between, which round-3a's own history says cannot be
	// assumed.
	type leg struct {
		grants []time.Time // insertion order, NOT value order — the bug.
	}
	commitOptimistic := func(l *leg, t2 time.Time) { l.grants = append(l.grants, t2) }
	undoNaive := func(l *leg) {
		if len(l.grants) == 0 {
			return
		}
		l.grants = l.grants[:len(l.grants)-1] // "pop the one we just added"
	}

	l := &leg{}
	// Call A (broken flow): commits optimistically, then discovers its
	// deadline cannot be met.
	commitOptimistic(l, epoch)
	// Call B interleaves before A's undo runs (a second goroutine's
	// legitimate call, landing later).
	commitOptimistic(l, epoch.Add(9*time.Second))
	// Call A's deadline check now fails; undo pops the LAST element —
	// which is call B's legitimate grant, not call A's own.
	undoNaive(l)

	wantAfterUndo := []time.Time{epoch.Add(9 * time.Second)} // B's grant should be all that remains.
	if len(l.grants) == len(wantAfterUndo) && l.grants[0].Equal(wantAfterUndo[0]) {
		t.Fatalf("RED expected: naive undo must corrupt the leg's memory, got %v", l.grants)
	}

	// Real mechanism: GREEN. The real acquire() never commits before the
	// deadline decision, so there is nothing to undo and no corruption is
	// possible.
	s := newSchedule(unorderedSchedule, []window{w})
	// Call A never actually commits (its deadline is unreachable —
	// simulated directly: the caller simply never calls commit for it).
	// Call B commits for real.
	s.commit(epoch.Add(9 * time.Second))
	if len(s.grants) != 1 || !s.grants[0].Equal(epoch.Add(9*time.Second)) {
		t.Errorf("GREEN expected: grants = %v, want exactly B's grant", s.grants)
	}
}

// Row 3: bucket-cap — express the quota window as a token bucket sized to
// its count (round-2: b + r*W, double the configured figure).
func TestRedFirst_BucketCap(t *testing.T) {
	t.Parallel()
	count, per := 5, 10*time.Second
	quotaWindow := window{count: count, per: per}

	// Broken: a classic token bucket, capacity=count, refilled at
	// count/per tokens per second.
	type bucket struct {
		cap, tokens, rate float64
		last              time.Time
	}
	tryTake := func(b *bucket, now time.Time) bool {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens += elapsed * b.rate
		if b.tokens > b.cap {
			b.tokens = b.cap
		}
		b.last = now
		if b.tokens >= 1 {
			b.tokens--
			return true
		}
		return false
	}

	// Broken: RED.
	{
		b := &bucket{cap: float64(count), tokens: float64(count), rate: float64(count) / per.Seconds(), last: epoch}
		var admitted []time.Time
		// Instant burst: drains the full bucket.
		for i := 0; i < count; i++ {
			if tryTake(b, epoch) {
				admitted = append(admitted, epoch)
			}
		}
		// Over the rest of the window, refills trickle in and get consumed too.
		for i := 1; i <= count; i++ {
			at := epoch.Add(time.Duration(i) * (per / time.Duration(count)))
			if tryTake(b, at) {
				admitted = append(admitted, at)
			}
		}
		if windowRespected(admitted, quotaWindow) {
			t.Fatalf("RED expected: a token bucket must admit more than %d within %v, got %v", count, per, admitted)
		}
	}

	// Real mechanism: GREEN.
	{
		s := newSchedule(unorderedSchedule, []window{quotaWindow})
		var admitted []time.Time
		base := epoch
		for i := 0; i < 2*count; i++ {
			t2 := s.earliest(base)
			s.commit(t2)
			admitted = append(admitted, t2)
			base = t2
		}
		if !windowRespected(admitted, quotaWindow) {
			t.Errorf("GREEN expected: the quota window must never be exceeded, got %v", admitted)
		}
	}
}

// Row 4: ordered-global — make the class-global schedule ordered, so
// earliest answers "after everything already granted" (cross-chat
// head-of-line blocking and, with deadlines, starvation).
func TestRedFirst_OrderedGlobal(t *testing.T) {
	t.Parallel()
	globalWindow := window{count: 1000, per: time.Second} // generous — count is not the bottleneck here, ORDER is.

	// Broken: RED — a wrongly-ordered global schedule.
	{
		globalBroken := newSchedule(orderedSchedule, []window{globalWindow})
		busyChat := newSchedule(orderedSchedule, []window{{count: 1, per: 100 * time.Second}})
		freeChat := newSchedule(orderedSchedule, nil)

		// Chat A already has a backlog reaching far into the future.
		busyChat.commit(epoch.Add(100 * time.Second))
		tA := max2(globalBroken.earliest(epoch), busyChat.earliest(epoch))
		globalBroken.commit(tA)
		busyChat.commit(tA)

		// Chat B, independent and free, arrives right after.
		tB := max2(globalBroken.earliest(epoch), freeChat.earliest(epoch))

		if tB.Before(epoch.Add(50 * time.Second)) {
			t.Fatalf("RED expected: an ordered global schedule must starve chat B behind chat A's backlog, got %v", tB.Sub(epoch))
		}
	}

	// Real mechanism: GREEN — unordered global.
	{
		globalReal := newSchedule(unorderedSchedule, []window{globalWindow})
		busyChat := newSchedule(orderedSchedule, []window{{count: 1, per: 100 * time.Second}})
		freeChat := newSchedule(orderedSchedule, nil)

		busyChat.commit(epoch.Add(100 * time.Second))
		tA := max2(globalReal.earliest(epoch), busyChat.earliest(epoch))
		globalReal.commit(tA)
		busyChat.commit(tA)

		tB := max2(globalReal.earliest(epoch), freeChat.earliest(epoch))
		if tB.After(epoch.Add(time.Second)) {
			t.Errorf("GREEN expected: an unordered global schedule must not starve chat B, got %v", tB.Sub(epoch))
		}
	}
}

func max2(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Row 5: count-retention — retain the newest max(c) grants instead of
// retaining by time (an unordered schedule silently forgets live grants
// and over-emits).
func TestRedFirst_CountRetention(t *testing.T) {
	t.Parallel()
	w := window{count: 2, per: 10 * time.Second}

	// Broken: RED — retention keeps only the `count` grants with the
	// LARGEST timestamp VALUES (the naive reading of "keep the newest c"
	// on a schedule whose grants array is sorted BY VALUE, design D9's
	// exact phrasing: "on an unordered schedule, whose inserts are sorted
	// rather than appended, a count bound silently discards grants that
	// are still inside a live window"). A far-future backlog (a busy
	// chat's already-reserved slots) permanently outranks any genuinely
	// imminent, real near-term grant — so every near-term admission gets
	// forgotten the instant it is granted, and the window's true cap is
	// never enforced among near-term emissions at all.
	{
		var grants []time.Time // kept sorted by value, like the real schedule.
		commitKeepLargest := func(t2 time.Time) {
			idx := sort.Search(len(grants), func(i int) bool { return !grants[i].Before(t2) })
			grants = append(grants, time.Time{})
			copy(grants[idx+1:], grants[idx:])
			grants[idx] = t2
			if len(grants) > w.count {
				grants = grants[len(grants)-w.count:] // drop the SMALLEST values first.
			}
		}
		wouldAdmit := func(t2 time.Time) bool {
			count := 1
			for _, g := range grants {
				if g.After(t2.Add(-w.per)) && !g.After(t2) {
					count++
				}
			}
			return count <= w.count
		}

		// A busy chat's far-future backlog, already reserved.
		commitKeepLargest(epoch.Add(50 * time.Second))
		commitKeepLargest(epoch.Add(60 * time.Second))

		// Three genuinely imminent, real near-term calls, each checked
		// and admitted against the (corrupted) retained memory.
		var trueEmissions []time.Time
		for _, candidate := range []time.Time{
			epoch.Add(500 * time.Millisecond),
			epoch.Add(600 * time.Millisecond),
			epoch.Add(700 * time.Millisecond),
		} {
			if !wouldAdmit(candidate) {
				t.Fatalf("test setup: expected the broken schedule to (wrongly) admit %v — its far-future-biased memory should already be corrupted", candidate)
			}
			commitKeepLargest(candidate)
			trueEmissions = append(trueEmissions, candidate)
		}
		trueEmissions = append(trueEmissions, epoch.Add(50*time.Second), epoch.Add(60*time.Second))

		if windowRespected(trueEmissions, w) {
			t.Fatalf("RED expected: count-based retention must let true near-term emissions violate window(2,10s), got %v", trueEmissions)
		}
	}

	// Real mechanism: GREEN — the identical arrival sequence through the
	// real, time-based-retention schedule.
	{
		s := newSchedule(unorderedSchedule, []window{w})
		s.commit(epoch.Add(50 * time.Second))
		s.commit(epoch.Add(60 * time.Second))
		var emissions []time.Time
		for _, candidate := range []time.Time{
			epoch.Add(500 * time.Millisecond),
			epoch.Add(600 * time.Millisecond),
			epoch.Add(700 * time.Millisecond),
		} {
			t2 := s.earliest(candidate)
			s.commit(t2)
			emissions = append(emissions, t2)
		}
		emissions = append(emissions, epoch.Add(50*time.Second), epoch.Add(60*time.Second))
		if !windowRespected(emissions, w) {
			t.Errorf("GREEN expected: time-based retention must never let true emissions violate the window, got %v", emissions)
		}
	}
}

// Row 6: pace-only — map a pacing key to its pacing window alone, dropping
// the quota window, with a truncated interval (N+1 emissions per W: the
// quota window is what forbids the extra one).
func TestRedFirst_PaceOnly(t *testing.T) {
	t.Parallel()
	count, per := 3, 10*time.Second
	quotaWindow := window{count: count, per: per}

	// Broken: RED — pacing window only, interval truncated (floor, not
	// ceil), no quota window at all.
	{
		interval := per / time.Duration(count) // integer division truncates.
		s := newSchedule(unorderedSchedule, []window{{count: 1, per: interval}})
		var emissions []time.Time
		base := epoch
		for i := 0; i < count+1; i++ {
			t2 := s.earliest(base)
			s.commit(t2)
			emissions = append(emissions, t2)
			base = t2
		}
		if windowRespected(emissions, quotaWindow) {
			t.Fatalf("RED expected: pace-only (truncated interval, no quota window) must admit %d+1 within %v, got %v", count, per, emissions)
		}
	}

	// Real mechanism: GREEN — paceWindows + quotaWindows together.
	{
		windows := append(paceWindows(rate(count, per)), quotaWindows(rate(count, per))...)
		s := newSchedule(unorderedSchedule, windows)
		var emissions []time.Time
		base := epoch
		for i := 0; i < count+1; i++ {
			t2 := s.earliest(base)
			s.commit(t2)
			emissions = append(emissions, t2)
			base = t2
		}
		if !windowRespected(emissions, quotaWindow) {
			t.Errorf("GREEN expected: the quota window must forbid the %d-th emission within %v, got %v", count+1, per, emissions)
		}
	}
}

// Row 7: quota-only — map a pacing key to its quota window alone, dropping
// the pacing window (N emissions at one instant then an idle W: the
// pacing window is what makes emission steady).
func TestRedFirst_QuotaOnly(t *testing.T) {
	t.Parallel()
	count, per := 3, 10*time.Second

	// Broken: RED — quota window only, no pacing window: all N grants
	// land at the exact same instant.
	{
		s := newSchedule(unorderedSchedule, quotaWindows(rate(count, per)))
		var emissions []time.Time
		for i := 0; i < count; i++ {
			t2 := s.earliest(epoch)
			s.commit(t2)
			emissions = append(emissions, t2)
		}
		allSame := true
		for _, e := range emissions {
			if !e.Equal(epoch) {
				allSame = false
			}
		}
		if !allSame {
			t.Fatalf("RED expected: quota-only must burst every grant at the same instant, got %v", emissions)
		}
	}

	// Real mechanism: GREEN — paceWindows + quotaWindows together spread
	// emission steadily.
	{
		windows := append(paceWindows(rate(count, per)), quotaWindows(rate(count, per))...)
		s := newSchedule(unorderedSchedule, windows)
		var emissions []time.Time
		for i := 0; i < count; i++ {
			t2 := s.earliest(epoch)
			s.commit(t2)
			emissions = append(emissions, t2)
		}
		for i := 1; i < len(emissions); i++ {
			if !emissions[i].After(emissions[i-1]) {
				t.Errorf("GREEN expected: emission %d (%v) must be strictly after emission %d (%v) — steady, not bursty", i, emissions[i], i-1, emissions[i-1])
			}
		}
	}
}

// ---- paceWindows dedup (finding 15) ----

// TestPaceWindows_DedupesWhenTheyCoincide asserts design D9's
// pacing-windows paragraph's claim directly: "When N is 1 the two windows
// coincide and the schedule holds one." Before the fix, N=1 produced two
// textually identical windows.
func TestPaceWindows_DedupesWhenTheyCoincide(t *testing.T) {
	t.Parallel()
	got := paceWindows(rate(1, time.Second))
	want := []window{{count: 1, per: time.Second}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("paceWindows(1/1s) = %v, want %v (the steady-shape and bound windows coincide)", got, want)
	}
}

// TestPaceWindows_KeepsBothWhenDistinct asserts the N>1 case still gets
// both windows — the dedup must not over-fire.
func TestPaceWindows_KeepsBothWhenDistinct(t *testing.T) {
	t.Parallel()
	got := paceWindows(rate(30, time.Second))
	if len(got) != 2 {
		t.Fatalf("paceWindows(30/1s) = %v, want 2 distinct windows", got)
	}
	if got[0] == got[1] {
		t.Errorf("paceWindows(30/1s): both windows are %v, want distinct", got[0])
	}
}

// ---- acquireFixedPoint exhaustion (finding 12) ----

// TestAcquireFixedPoint_ConvergesNormally asserts the ordinary case: an
// earliest function that never advances past the candidate converges
// immediately.
func TestAcquireFixedPoint_ConvergesNormally(t *testing.T) {
	t.Parallel()
	stay := func(t time.Time) time.Time { return t }
	got, converged := acquireFixedPoint(epoch, stay)
	if !converged {
		t.Fatal("acquireFixedPoint: converged = false, want true")
	}
	if !got.Equal(epoch) {
		t.Errorf("acquireFixedPoint = %v, want %v", got, epoch)
	}
}

// TestAcquireFixedPoint_ExhaustionIsReportedNotSwallowed asserts finding
// 12's fix directly: an earliest function constructed to never settle
// (each pass strictly later than the last) must exhaust
// maxAcquirePasses and report converged=false — a defect signalled to
// the caller, never a silent fallback that commits at whatever instant
// the search happened to reach.
func TestAcquireFixedPoint_ExhaustionIsReportedNotSwallowed(t *testing.T) {
	t.Parallel()
	neverSettles := func(t time.Time) time.Time { return t.Add(time.Nanosecond) }
	_, converged := acquireFixedPoint(epoch, neverSettles)
	if converged {
		t.Fatal("acquireFixedPoint with a never-settling earliest fn: converged = true, want false")
	}
}
