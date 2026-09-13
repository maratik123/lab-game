package maze

import "testing"

// fakeStream replays a fixed sequence of values, cycling once exhausted,
// and counts how many values it has handed out — the reduction tests'
// way of asserting a bound of one or zero touches the stream not at
// all.
type fakeStream struct {
	vals  []uint64
	calls int
}

func (f *fakeStream) Uint64() uint64 {
	v := f.vals[f.calls%len(f.vals)]
	f.calls++
	return v
}

func TestBoundedDraw_ZeroOrOneBoundConsumesNothing(t *testing.T) {
	t.Parallel()
	for _, n := range []uint64{0, 1} {
		f := &fakeStream{vals: []uint64{42}}
		if got := boundedDraw(f, n); got != 0 {
			t.Errorf("boundedDraw(_, %d) = %d, want 0", n, got)
		}
		if f.calls != 0 {
			t.Errorf("boundedDraw(_, %d) consumed %d values from the stream, want 0", n, f.calls)
		}
	}
}

func TestBoundedDraw_ResultInRange(t *testing.T) {
	t.Parallel()
	s := newStream([32]byte{1, 2, 3})
	for i := 0; i < 10000; i++ {
		if got := boundedDraw(s, 7); got >= 7 {
			t.Fatalf("boundedDraw(_, 7) = %d, want < 7", got)
		}
	}
}

func TestBoundedDraw_RejectsAboveTheLimitRatherThanBiasing(t *testing.T) {
	t.Parallel()
	const n = 5
	// limit itself always fails v < limit, so it is always rejected —
	// the accepted value that follows must then be a fresh draw, never
	// a modulo reduction of the rejected one.
	limit := ^uint64(0) - (^uint64(0) % n)
	f := &fakeStream{vals: []uint64{limit, 3}}
	got := boundedDraw(f, n)
	if got != 3%n {
		t.Errorf("boundedDraw rejected value = %d, want %d", got, 3%n)
	}
	if f.calls != 2 {
		t.Errorf("boundedDraw consumed %d values, want 2 (one rejection then the accepted draw)", f.calls)
	}
}

func TestBoundedDraw_ReproducibleFromTheSameKey(t *testing.T) {
	t.Parallel()
	key := [32]byte{9, 9, 9}
	s1 := newStream(key)
	s2 := newStream(key)
	for i := 0; i < 100; i++ {
		a := boundedDraw(s1, 1000)
		b := boundedDraw(s2, 1000)
		if a != b {
			t.Fatalf("draw %d: %d != %d for the same key", i, a, b)
		}
	}
}

func TestShuffle_IsAPermutation(t *testing.T) {
	t.Parallel()
	s := newStream([32]byte{7})
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	shuffle(s, items)
	seen := map[int]bool{}
	for _, v := range items {
		seen[v] = true
	}
	if len(seen) != 10 {
		t.Fatalf("shuffle produced %v, not a permutation of 0..9", items)
	}
}

func TestShuffle_ReproducibleFromTheSameKey(t *testing.T) {
	t.Parallel()
	key := [32]byte{3, 1, 4}
	a := []int{0, 1, 2, 3, 4, 5, 6, 7}
	b := append([]int(nil), a...)
	shuffle(newStream(key), a)
	shuffle(newStream(key), b)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("shuffle(%v) != shuffle(%v) under the same key", a, b)
		}
	}
}

func TestWeightedPick_NeverReturnsAZeroWeightIndex(t *testing.T) {
	t.Parallel()
	s := newStream([32]byte{5, 5, 5})
	weights := []uint64{0, 5, 0, 3}
	for i := 0; i < 10000; i++ {
		idx := weightedPick(s, weights)
		if weights[idx] == 0 {
			t.Fatalf("weightedPick returned index %d, whose weight is zero", idx)
		}
	}
}

func TestWeightedPick_ReproducibleFromTheSameKey(t *testing.T) {
	t.Parallel()
	key := [32]byte{2, 7, 1}
	weights := []uint64{1, 2, 3, 4}
	s1 := newStream(key)
	s2 := newStream(key)
	for i := 0; i < 100; i++ {
		a := weightedPick(s1, weights)
		b := weightedPick(s2, weights)
		if a != b {
			t.Fatalf("draw %d: weightedPick diverged under the same key", i)
		}
	}
}

func TestBiasedNewest_ZeroThresholdNeverNewest_MaxAlwaysNewest(t *testing.T) {
	t.Parallel()
	s := newStream([32]byte{4, 4, 4})
	for i := 0; i < 1000; i++ {
		if biasedNewest(s, 0) {
			t.Fatal("biasedNewest(_, 0) selected the newest entry")
		}
	}
	for i := 0; i < 1000; i++ {
		if !biasedNewest(s, 1<<32) {
			t.Fatal("biasedNewest(_, 2^32) did not select the newest entry")
		}
	}
}
