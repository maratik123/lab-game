package world

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
)

// testParams returns a small, valid generation Params: the generator's
// own minimum radius, so a chunk is small enough to compare cell by
// cell, and every share well inside the bounds New accepts.
func testParams() maze.Params {
	return maze.Params{
		Radius:            maze.MinRadius,
		Weights:           maze.AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   decimal.New(5, -1),
		PortalShareLower:  decimal.New(1, -1),
		PortalShareUpper:  decimal.New(2, -1),
	}
}

// testGenerator builds a Generator over testParams and a seed of the
// caller's choosing, failing tb on refusal.
func testGenerator(tb testing.TB, seed int64) *maze.Generator {
	tb.Helper()
	gen, err := maze.New(seed, testParams())
	if err != nil {
		tb.Fatalf("maze.New: %v", err)
	}
	return gen
}

// mapsEqual asserts a and b agree on chunk, type, version and every
// local cell's face set.
func mapsEqual(t *testing.T, a, b maze.Map) {
	t.Helper()
	if a.Chunk() != b.Chunk() {
		t.Fatalf("chunk = %v, want %v", a.Chunk(), b.Chunk())
	}
	if a.Type() != b.Type() {
		t.Fatalf("type = %v, want %v", a.Type(), b.Type())
	}
	if a.Version() != b.Version() {
		t.Fatalf("version = %d, want %d", a.Version(), b.Version())
	}
	lattice := hexgrid.Lattice{Radius: a.Radius()}
	for _, c := range lattice.LocalCells() {
		af, ok := a.Faces(c)
		if !ok {
			t.Fatalf("a.Faces(%v): not ok", c)
		}
		bf, ok := b.Faces(c)
		if !ok {
			t.Fatalf("b.Faces(%v): not ok", c)
		}
		if af != bf {
			t.Fatalf("cell %v faces = %v, want %v", c, af, bf)
		}
	}
}
