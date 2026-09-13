package hexgrid_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var refDims = hexgrid.Dims{Cols: 16, Rows: 16}

func TestChunkOf_FloorDivisionStraddlesZeroCorrectly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c    hexgrid.Coord
		want hexgrid.Chunk
	}{
		{hexgrid.Coord{Q: 0, R: 0}, hexgrid.Chunk{Q: 0, R: 0}},
		{hexgrid.Coord{Q: 15, R: 15}, hexgrid.Chunk{Q: 0, R: 0}},
		{hexgrid.Coord{Q: 16, R: 16}, hexgrid.Chunk{Q: 1, R: 1}},
		{hexgrid.Coord{Q: -1, R: -1}, hexgrid.Chunk{Q: -1, R: -1}},
		{hexgrid.Coord{Q: -16, R: -16}, hexgrid.Chunk{Q: -1, R: -1}},
		{hexgrid.Coord{Q: -17, R: -17}, hexgrid.Chunk{Q: -2, R: -2}},
		{hexgrid.Coord{Q: -1, R: 15}, hexgrid.Chunk{Q: -1, R: 0}},
		{hexgrid.Coord{Q: 15, R: -1}, hexgrid.Chunk{Q: 0, R: -1}},
	}
	for _, c := range cases {
		if got := refDims.ChunkOf(c.c); got != c.want {
			t.Errorf("ChunkOf(%v) = %v, want %v", c.c, got, c.want)
		}
	}
}

// TestContains_RejectsAChunkThatDoesNotHoldTheCell is the half without
// which Contains has no discriminating test at all: asserting
// Contains(ChunkOf(c), c) is asserting ChunkOf(c) == ChunkOf(c), which a
// body of "return true" also satisfies.
func TestContains_RejectsAChunkThatDoesNotHoldTheCell(t *testing.T) {
	t.Parallel()
	for _, c := range []hexgrid.Coord{
		{Q: 0, R: 0},
		{Q: 15, R: 15},
		{Q: -1, R: -1},
		{Q: 33, R: -7},
	} {
		own := refDims.ChunkOf(c)
		for _, other := range []hexgrid.Chunk{
			{Q: own.Q + 1, R: own.R},
			{Q: own.Q, R: own.R + 1},
			{Q: own.Q - 1, R: own.R - 1},
		} {
			if refDims.Contains(other, c) {
				t.Errorf("Contains(%v, %v) = true, want false — %v lies in %v", other, c, c, own)
			}
		}
		if !refDims.Contains(own, c) {
			t.Errorf("Contains(%v, %v) = false, want true", own, c)
		}
	}
}

func TestChunkOf_PartitionsTheRegionExactly(t *testing.T) {
	t.Parallel()
	// A region straddling zero in both axes.
	seen := map[hexgrid.Chunk]int{}
	for q := int32(-40); q <= 40; q++ {
		for r := int32(-40); r <= 40; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			ch := refDims.ChunkOf(c)
			if !refDims.Contains(ch, c) {
				t.Fatalf("Contains(%v, %v) = false, want true", ch, c)
			}
			seen[ch]++
		}
	}
	for ch, count := range seen {
		if count > 16*16 {
			t.Errorf("chunk %v holds %d cells of the region, more than a chunk's capacity", ch, count)
		}
	}
}

func TestChunkOf_DifferentDimensionsGiveDifferentChunks(t *testing.T) {
	t.Parallel()
	c := hexgrid.Coord{Q: 20, R: 20}
	small := hexgrid.Dims{Cols: 8, Rows: 8}
	large := hexgrid.Dims{Cols: 32, Rows: 32}
	if small.ChunkOf(c) == large.ChunkOf(c) {
		t.Errorf("same coordinate under different dimensions mapped to the same chunk")
	}
}

func TestChunkDistance_ZeroExactlyWhenSameChunk(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		a := hexgrid.Coord{
			Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "aq")),
			R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "ar")),
		}
		b := hexgrid.Coord{
			Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "bq")),
			R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "br")),
		}
		dist := refDims.Distance(a, b)
		sameChunk := refDims.ChunkOf(a) == refDims.ChunkOf(b)
		if sameChunk != (dist == 0) {
			rt.Fatalf("Distance(%v,%v)=%d, sameChunk=%v", a, b, dist, sameChunk)
		}
	})
}

func TestChunkDistance_HandComputedTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b hexgrid.Chunk
		want int64
	}{
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 0, R: 0}, 0},
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 1, R: 0}, 1},
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 0, R: 1}, 1},
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 1, R: -1}, 1},
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 2, R: -1}, 2},
		{hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: -3, R: 5}, 5},
		{hexgrid.Chunk{Q: -2, R: -2}, hexgrid.Chunk{Q: 1, R: 1}, 6},
	}
	for _, c := range cases {
		if got := hexgrid.ChunkDistance(c.a, c.b); got != c.want {
			t.Errorf("ChunkDistance(%v,%v) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := hexgrid.ChunkDistance(c.b, c.a); got != c.want {
			t.Errorf("ChunkDistance(%v,%v) = %d, want %d (symmetry)", c.b, c.a, got, c.want)
		}
	}
}

func TestChunkDistance_TriangleInequality(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		draw := func(label string) hexgrid.Chunk {
			return hexgrid.Chunk{
				Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, label+"q")),
				R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, label+"r")),
			}
		}
		a, b, c := draw("a"), draw("b"), draw("c")
		if hexgrid.ChunkDistance(a, c) > hexgrid.ChunkDistance(a, b)+hexgrid.ChunkDistance(b, c) {
			rt.Fatalf("triangle inequality violated for %v,%v,%v", a, b, c)
		}
	})
}

func TestChunkDistance_NoOverflowAtExtremes(t *testing.T) {
	t.Parallel()
	a := hexgrid.Chunk{Q: -2147483648, R: -2147483648}
	b := hexgrid.Chunk{Q: 2147483647, R: 2147483647}
	got := hexgrid.ChunkDistance(a, b)
	if got <= 0 {
		t.Errorf("ChunkDistance at extreme chunk coordinates = %d, want a large positive value", got)
	}
}
