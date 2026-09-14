package hexgrid_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestChunkDistance_ZeroExactlyWhenSame(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		a := hexgrid.Chunk{
			Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "aq")),
			R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, "ar")),
		}
		if dist := hexgrid.ChunkDistance(a, a); dist != 0 {
			rt.Fatalf("ChunkDistance(%v,%v) = %d, want 0", a, a, dist)
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
