package tg

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/maratik123/lab-game/internal/config"
)

// window is one window constraint: no half-open interval of length Per
// contains more than Count grants.
type window struct {
	count int
	per   time.Duration
}

// scheduleKind distinguishes the two obligations a schedule can serve:
// ordered (one chat key) raises a candidate to the key's own
// newest grant first, so grants are non-decreasing; unordered (the
// class-global key) answers the earliest admissible instant with no such
// clamp, which is what stops one chat's backlog blocking another
// (a head-of-line hazard).
type scheduleKind int

const (
	orderedSchedule scheduleKind = iota
	unorderedSchedule
)

// schedule is the whole state for one limiter key: its window set, and the
// emission instants already granted, held in ascending order.
// The zero value is not usable; construct with newSchedule. A schedule is
// not safe for concurrent use on its own — Limiter's single mutex is what
// makes it safe.
type schedule struct {
	kind    scheduleKind
	windows []window
	grants  []time.Time
}

func newSchedule(kind scheduleKind, windows []window) *schedule {
	return &schedule{kind: kind, windows: windows}
}

// maxPer returns the longest window's Per, or 0 when s has no windows
// (unbounded — never blocks, never retains history).
func (s *schedule) maxPer() time.Duration {
	var maxPer time.Duration
	for _, w := range s.windows {
		if w.per > maxPer {
			maxPer = w.per
		}
	}
	return maxPer
}

// holds reports whether inserting one more grant at t would keep every
// window's invariant on the FULL sorted grant set — grant[j+c]-grant[j] >=
// per for every j — not merely the window ending at t. On an
// unordered schedule t may sort before an already-committed later grant,
// and that later grant's own window can be the one a naive
// t-is-always-newest check would miss; only the O(count) positions
// touching t's insertion point can be affected by inserting it, so this
// checks exactly those.
func (s *schedule) holds(t time.Time) bool {
	idx := sort.Search(len(s.grants), func(i int) bool { return !s.grants[i].Before(t) })
	at := func(k int) time.Time {
		switch {
		case k < idx:
			return s.grants[k]
		case k == idx:
			return t
		default:
			return s.grants[k-1]
		}
	}
	n := len(s.grants) + 1
	for _, w := range s.windows {
		lo := idx - w.count
		if lo < 0 {
			lo = 0
		}
		for j := lo; j <= idx; j++ {
			k := j + w.count
			if k >= n {
				continue
			}
			if at(k).Sub(at(j)) < w.per {
				return false
			}
		}
	}
	return true
}

// earliest returns the earliest instant t >= candidate at which one more
// grant still satisfies every window's invariant. It is a pure read,
// mutating nothing: for the ordered kind, candidate is first
// raised to the schedule's own newest grant, so grants stay
// non-decreasing; the search then tests candidate itself and every
// grant+window.per instant in ascending order, returning the first that
// holds. That set is exhaustive — the invariant can only change where a
// grant's window expires — and its maximum element always holds (every
// existing grant is then outside every window), so the search always
// terminates.
func (s *schedule) earliest(candidate time.Time) time.Time {
	if s.kind == orderedSchedule && len(s.grants) > 0 {
		if last := s.grants[len(s.grants)-1]; last.After(candidate) {
			candidate = last
		}
	}
	if len(s.windows) == 0 {
		return candidate
	}

	candidates := make([]time.Time, 0, len(s.grants)*len(s.windows)+1)
	candidates = append(candidates, candidate)
	for _, w := range s.windows {
		for _, g := range s.grants {
			if t := g.Add(w.per); t.After(candidate) {
				candidates = append(candidates, t)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })

	for _, t := range candidates {
		if s.holds(t) {
			return t
		}
	}
	// Unreachable: the last (largest) candidate always holds — see the
	// doc comment above. Kept as a defensive, panic-free fallback rather
	// than a silent infinite loop.
	return candidates[len(candidates)-1]
}

// commit inserts t in sorted order. It does not evict — see evict — so
// that a single decision computed from one real "now" (a burst of
// simultaneously-arriving calls, each granted a different future instant)
// never has an earlier member of that same burst forgotten merely because
// a later member's grant lands further in the future: the invariant must
// hold over the whole burst, not just pairwise against the most recently
// committed instant.
func (s *schedule) commit(t time.Time) {
	if len(s.windows) == 0 {
		// Unbounded: no window to constrain, so nothing to remember — an
		// unbounded value contributes no window and never allocates history.
		return
	}

	idx := sort.Search(len(s.grants), func(i int) bool { return !s.grants[i].Before(t) })
	s.grants = append(s.grants, time.Time{})
	copy(s.grants[idx+1:], s.grants[idx:])
	s.grants[idx] = t
}

// evict drops every grant that can no longer constrain any window as of
// real time now — retention is by TIME alone, never by count:
// a grant is dropped once grant+maxPer(s) is in the past RELATIVE TO NOW,
// never relative to a future grant instant this schedule has computed.
// Eviction depends only on the window set and the clock, never on a
// refusal, a cancellation, or any call outcome, which is what makes the
// round-3a defect (a refusal corrupting the window's memory) structurally
// impossible here.
func (s *schedule) evict(now time.Time) {
	maxPer := s.maxPer()
	if maxPer <= 0 || len(s.grants) == 0 {
		return
	}
	threshold := now.Add(-maxPer)
	i := 0
	for i < len(s.grants) && s.grants[i].Before(threshold) {
		i++
	}
	if i > 0 {
		s.grants = s.grants[i:]
	}
}

// chatKey identifies one per-chat schedule: the destination chat's raw key
// (or a reserved unknown-chat token) paired with the method
// class.
type chatKey struct {
	key   string
	class MethodClass
}

// unknownChatKey is the single reserved key every ChatUnknown call of one
// class shares: bounded, never exempt, conservative when
// several distinct real chats would otherwise collapse into it.
const unknownChatKey = "\x00unknown"

// Limiter owns every window schedule — the three class-global schedules
// and every per-(chat, class) schedule of a bounded class — under one
// mutex: one lock, one instant, commit-after-decide. It is
// in-process only: nothing here writes to a table, to disk, or to any
// store that outlives the process.
type Limiter struct {
	mu     sync.Mutex
	global [3]*schedule
	chats  map[chatKey]*schedule
	limits config.TransportLimits
}

// newLimiter builds a Limiter from limits: one unordered class-global
// schedule per MethodClass, built eagerly; per-chat schedules are ordered
// and built lazily, on first reference, only for bounded classes — an
// unbounded class allocates no history at all.
func newLimiter(limits config.TransportLimits) *Limiter {
	l := &Limiter{
		limits: limits,
		chats:  make(map[chatKey]*schedule),
	}
	l.global[ClassMessage] = newSchedule(unorderedSchedule, paceWindows(limits.Message.Global))
	l.global[ClassEdit] = newSchedule(unorderedSchedule, paceWindows(limits.Edit.Global))
	l.global[ClassOther] = newSchedule(unorderedSchedule, paceWindows(limits.Other.Global))
	return l
}

// classLimits returns class's configured ClassLimits.
func (l *Limiter) classLimits(class MethodClass) config.ClassLimits {
	switch class {
	case ClassMessage:
		return l.limits.Message
	case ClassEdit:
		return l.limits.Edit
	default:
		return l.limits.Other
	}
}

// chatScheduleLocked returns call's per-chat schedule, or nil when call
// addresses no chat (ChatNone — such a call charges its
// class-global schedule only). Must be called with l.mu held.
func (l *Limiter) chatScheduleLocked(call Call) *schedule {
	if call.Chat.Target == ChatNone {
		return nil
	}
	key := chatKey{key: call.Chat.Key, class: call.Class}
	if call.Chat.Target == ChatUnknown {
		key = chatKey{key: unknownChatKey, class: call.Class}
	}
	if s, ok := l.chats[key]; ok {
		return s
	}
	cl := l.classLimits(call.Class)
	windows := append(paceWindows(cl.ChatRate), quotaWindows(cl.ChatCap)...)
	if len(windows) == 0 {
		// An unbounded class allocates no per-chat history at all: a
		// schedule with no windows never blocks, so
		// there is nothing a stored entry would ever be consulted for —
		// storing one anyway would let a distinct chat id per call grow
		// this map without bound.
		return nil
	}
	s := newSchedule(orderedSchedule, windows)
	l.chats[key] = s
	return s
}

// maxAcquirePasses bounds the decide-then-commit fixed-point search.
// The loop provably terminates — t strictly increases on
// every non-final pass and both schedules admit all sufficiently large
// instants — in at most the number of distinct candidate instants either
// schedule could propose, which is bounded by calls in flight. This bound
// is a defensive safety valve for a search that is not believed to reach
// it, never a fallback path a correct run relies on.
const maxAcquirePasses = 4096

// acquireFixedPoint runs the decide-then-commit fixed-point
// search from start, consulting earliestFns (each a schedule's earliest,
// or an equivalent) until none advances t any further, or until
// maxAcquirePasses is exhausted. It mutates nothing — pure decision.
// Extracted from acquire so the exhaustion branch — exhaustion is a
// defect, not a silent fallback — is directly
// testable with a deliberately non-converging stub, independent of any
// real schedule's own termination proof.
func acquireFixedPoint(start time.Time, earliestFns ...func(time.Time) time.Time) (t time.Time, converged bool) {
	t = start
	for pass := 0; pass < maxAcquirePasses; pass++ {
		next := t
		for _, f := range earliestFns {
			if c := f(t); c.After(next) {
				next = c
			}
		}
		if !next.After(t) {
			return t, true
		}
		t = next
	}
	return t, false
}

// acquire decides the emission instant for call, not before now, honoring
// deadline when hasDeadline is true, and commits to every relevant
// schedule at that same instant — decide
// against every constraint first, commit to every constraint at that same
// instant, never commit before the answer is final and never partially
// undo. It returns (t, true, nil) on a grant, or (zero, false, nil) when
// the required wait would end after deadline — in which case NOTHING has
// been mutated: a refusal cannot corrupt anything, because a
// refusal happens before any mutation. A non-nil error means the
// fixed-point search did not converge within maxAcquirePasses — believed
// unreachable given a correct window configuration, but treated as the
// defect it is, never as a
// silent fallback: nothing is committed and no grant is returned.
func (l *Limiter) acquire(call Call, now, deadline time.Time, hasDeadline bool) (time.Time, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	global := l.global[call.Class]
	chat := l.chatScheduleLocked(call)
	global.evict(now)
	if chat != nil {
		chat.evict(now)
	}

	earliestFns := []func(time.Time) time.Time{global.earliest}
	if chat != nil {
		earliestFns = append(earliestFns, chat.earliest)
	}
	t, converged := acquireFixedPoint(now, earliestFns...)
	if !converged {
		return time.Time{}, false, fmt.Errorf("tg: limiter: fixed-point search did not converge within %d passes", maxAcquirePasses)
	}

	if hasDeadline && t.After(deadline) {
		return time.Time{}, false, nil
	}

	global.commit(t)
	if chat != nil {
		chat.commit(t)
	}
	return t, true, nil
}

// paceWindows expresses a pacing key's rate — steady emission at N per W,
// no burst, a leaky bucket in the shaping sense — as both
// windows it needs: (1, ceil(W/N)), which bears the STEADY SHAPE, and
// (N, W), which bears the BOUND itself. r's zero value (unbounded)
// contributes no window. ceil rather than truncation buys evenness only —
// the quota window is what forbids the N+1-th emission either way
// (configuration maps onto windows). When N is 1 the two windows
// coincide exactly, so only one is kept.
func paceWindows(r config.Rate) []window {
	if r.Count <= 0 {
		return nil
	}
	pace := window{count: 1, per: ceilDiv(r.Per, r.Count)}
	bound := window{count: r.Count, per: r.Per}
	if pace == bound {
		return []window{pace}
	}
	return []window{pace, bound}
}

// quotaWindows expresses a quota key's cap — at most N in any window of
// length W — as the one window that states it directly. r's zero value
// (unbounded) contributes no window.
func quotaWindows(r config.Rate) []window {
	if r.Count <= 0 {
		return nil
	}
	return []window{{count: r.Count, per: r.Per}}
}

// ceilDiv returns ceil(w/n) as a time.Duration, n > 0.
func ceilDiv(w time.Duration, n int) time.Duration {
	return (w + time.Duration(n) - 1) / time.Duration(n)
}
