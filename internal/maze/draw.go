package maze

import "math/rand/v2"

// stream is the minimal source every reduction in this package draws
// from. math/rand/v2's ChaCha8 constructor is the only permitted
// math/rand/v2 identifier anywhere in this package; every reduction
// below consumes stream, never *rand.ChaCha8 directly, which is what
// keeps that permitted-identifier count at exactly one — a struct field
// typed with the concrete type would name a second.
type stream interface {
	Uint64() uint64
}

// newStream returns a stream keyed by key — the one call this package
// makes to math/rand/v2.
func newStream(key [32]byte) stream {
	return rand.NewChaCha8(key)
}

// boundedDraw returns a value uniformly distributed over the half-open
// interval [0,n), total: unlike the standard library's bounded draw, a
// bound of one or zero returns zero and consumes nothing from s, rather
// than panicking. Rejection sampling removes the modulo bias a plain
// v%n would carry.
func boundedDraw(s stream, n uint64) uint64 {
	if n <= 1 {
		return 0
	}
	limit := ^uint64(0) - (^uint64(0) % n)
	for {
		v := s.Uint64()
		if v < limit {
			return v % n
		}
	}
}

// shuffle permutes items in place via the Fisher-Yates algorithm driven
// by s.
func shuffle[T any](s stream, items []T) {
	for i := len(items) - 1; i > 0; i-- {
		j := boundedDraw(s, uint64(i+1))
		items[i], items[j] = items[j], items[i]
	}
}

// weightedPick draws an index into weights proportional to each entry's
// own weight. The caller guarantees at least one positive entry;
// weightedPick itself simply never returns an index whose own weight is
// zero, since a zero-weight entry's cumulative range has no width.
func weightedPick(s stream, weights []uint64) int {
	var total uint64
	for _, w := range weights {
		total += w
	}
	v := boundedDraw(s, total)
	var acc uint64
	for i, w := range weights {
		acc += w
		if v < acc {
			return i
		}
	}
	return len(weights) - 1
}

// biasedNewest reports whether a growing-tree draw should take the
// active list's newest entry rather than a random one: threshold is
// floor(bias × 2^32), compared strictly less than s's own top
// thirty-two bits, so a threshold of zero never selects the newest
// entry and a threshold of 2^32 always does.
func biasedNewest(s stream, threshold uint64) bool {
	top32 := s.Uint64() >> 32
	return top32 < threshold
}
