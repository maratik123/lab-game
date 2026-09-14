package gate_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
)

// spiralPrefix collects the first n chunks of gate.Spiral().
func spiralPrefix(n int) []hexgrid.Chunk {
	out := make([]hexgrid.Chunk, 0, n)
	for ch := range gate.Spiral() {
		if len(out) == n {
			break
		}
		out = append(out, ch)
	}
	return out
}

func TestSpiral_ListsEveryChunkOnceRingByRing(t *testing.T) {
	t.Parallel()
	const maxRing = 4
	// 1 + 3n(n+1) chunks cover rings 0..maxRing exactly.
	count := 1 + 3*maxRing*(maxRing+1)
	prefix := spiralPrefix(count)

	if len(prefix) != count {
		t.Fatalf("spiralPrefix(%d) returned %d chunks", count, len(prefix))
	}
	if prefix[0] != (hexgrid.Chunk{}) {
		t.Fatalf("first chunk = %v, want the centre", prefix[0])
	}

	seen := make(map[hexgrid.Chunk]bool, count)
	lastRing := int64(0)
	ringCount := 0
	for i, ch := range prefix {
		if seen[ch] {
			t.Fatalf("chunk %v (index %d) appears more than once", ch, i)
		}
		seen[ch] = true
		ring := hexgrid.ChunkDistance(hexgrid.Chunk{}, ch)
		if ring < lastRing {
			t.Fatalf("index %d: ring %d after ring %d — ring distances decreased", i, ring, lastRing)
		}
		if ring != lastRing {
			if lastRing > 0 && int64(ringCount) != 6*lastRing {
				t.Fatalf("ring %d contributed %d chunks, want %d", lastRing, ringCount, 6*lastRing)
			}
			lastRing = ring
			ringCount = 0
		}
		ringCount++
	}
	if int64(ringCount) != 6*lastRing {
		t.Fatalf("ring %d contributed %d chunks, want %d", lastRing, ringCount, 6*lastRing)
	}

	// Every chunk within maxRing of the centre, found by brute-force box
	// scan, appears in the prefix.
	for q := int32(-maxRing); q <= maxRing; q++ {
		for r := int32(-maxRing); r <= maxRing; r++ {
			ch := hexgrid.Chunk{Q: q, R: r}
			if hexgrid.ChunkDistance(hexgrid.Chunk{}, ch) > maxRing {
				continue
			}
			if !seen[ch] {
				t.Errorf("chunk %v within ring %d not found in Spiral prefix", ch, maxRing)
			}
		}
	}
}

func TestSpiralIndex_AgreesWithSpiral(t *testing.T) {
	t.Parallel()
	prefix := spiralPrefix(1 + 3*5*6)
	for i, ch := range prefix {
		if got := gate.SpiralIndex(ch); got != int64(i) {
			t.Errorf("SpiralIndex(%v) = %d, want %d (its position in Spiral())", ch, got, i)
		}
	}
}

func TestSpiralIndex_RingRange(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ch := hexgrid.Chunk{
			Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "q")),
			R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "r")),
		}
		n := hexgrid.ChunkDistance(hexgrid.Chunk{}, ch)
		idx := gate.SpiralIndex(ch)
		if n == 0 {
			if idx != 0 {
				rt.Fatalf("SpiralIndex(centre) = %d, want 0", idx)
			}
			return
		}
		lo := 1 + 3*n*(n-1)
		hi := 1 + 3*n*(n+1)
		if idx < lo || idx >= hi {
			rt.Fatalf("SpiralIndex(%v) = %d, want in [%d, %d) for ring %d", ch, idx, lo, hi, n)
		}
	})
}

func TestSpiral_RingWalkTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ring int
		want []hexgrid.Chunk
	}{
		{1, []hexgrid.Chunk{
			{Q: 1, R: 0},
			{Q: 1, R: -1},
			{Q: 0, R: -1},
			{Q: -1, R: 0},
			{Q: -1, R: 1},
			{Q: 0, R: 1},
		}},
		{2, []hexgrid.Chunk{
			{Q: 2, R: -1},
			{Q: 2, R: -2},
			{Q: 1, R: -2},
			{Q: 0, R: -2},
			{Q: -1, R: -1},
			{Q: -2, R: 0},
			{Q: -2, R: 1},
			{Q: -2, R: 2},
			{Q: -1, R: 2},
			{Q: 0, R: 2},
			{Q: 1, R: 1},
			{Q: 2, R: 0},
		}},
		{3, []hexgrid.Chunk{
			{Q: 3, R: -1},
			{Q: 3, R: -2},
			{Q: 3, R: -3},
			{Q: 2, R: -3},
			{Q: 1, R: -3},
			{Q: 0, R: -3},
			{Q: -1, R: -2},
			{Q: -2, R: -1},
			{Q: -3, R: 0},
			{Q: -3, R: 1},
			{Q: -3, R: 2},
			{Q: -3, R: 3},
			{Q: -2, R: 3},
			{Q: -1, R: 3},
			{Q: 0, R: 3},
			{Q: 1, R: 2},
			{Q: 2, R: 1},
			{Q: 3, R: 0},
		}},
	}

	prefix := spiralPrefix(1 + 3*3*4)
	for _, c := range cases {
		start := 1 + 3*c.ring*(c.ring-1)
		got := prefix[start : start+6*c.ring]
		if len(got) != len(c.want) {
			t.Fatalf("ring %d: got %d chunks, want %d", c.ring, len(got), len(c.want))
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ring %d position %d: got %v, want %v", c.ring, i, got[i], c.want[i])
			}
		}
	}
}

func TestSpiral_RingStartAndTurn(t *testing.T) {
	t.Parallel()
	const maxN = 10
	dirs := []hexgrid.Direction{
		hexgrid.DirE, hexgrid.DirNE, hexgrid.DirNW,
		hexgrid.DirW, hexgrid.DirSW, hexgrid.DirSE,
	}
	for n := 1; n <= maxN; n++ {
		start := hexgrid.Chunk{Q: int32(n), R: int32(-(n / 2))}
		startIdx := gate.SpiralIndex(start)
		wantIdx := int64(1 + 3*n*(n-1))
		if startIdx != wantIdx {
			t.Errorf("ring %d: SpiralIndex(%v) = %d, want %d (ring's first index)", n, start, startIdx, wantIdx)
		}

		ring := spiralPrefix(1 + 3*n*(n+1))[1+3*n*(n-1) : 1+3*n*(n+1)]
		for i := range ring {
			next := ring[(i+1)%len(ring)]
			if dist := hexgrid.ChunkDistance(ring[i], next); dist != 1 {
				t.Errorf("ring %d position %d->%d: distance %d, want 1 (%v -> %v)", n, i, (i+1)%len(ring), dist, ring[i], next)
			}
		}

		for _, d := range dirs {
			corner := delta(d, n)
			wantPos := (int64(d)*int64(n) - int64(n/2)) % int64(6*n)
			if wantPos < 0 {
				wantPos += int64(6 * n)
			}
			gotIdx := gate.SpiralIndex(corner)
			wantIdx := int64(1+3*n*(n-1)) + wantPos
			if gotIdx != wantIdx {
				t.Errorf("ring %d corner %v (dir %v): SpiralIndex = %d, want %d", n, corner, d, gotIdx, wantIdx)
			}
		}
	}
}

// delta returns n * the direction's unit chunk vector, computed by
// driving a chunk's exported Neighbor method n times.
func delta(d hexgrid.Direction, n int) hexgrid.Chunk {
	ch := hexgrid.Chunk{}
	for range n {
		ch = ch.Neighbor(d)
	}
	return ch
}
