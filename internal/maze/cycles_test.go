package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestAddExtraPassages_ZeroShareYieldsExactlyATree(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	islands := map[int]bool{}
	open := buildBacktracker(g, islands, newStream([32]byte{1}))
	before := len(open)
	addExtraPassages(g, islands, open, newStream([32]byte{2}), decimal.Zero)
	if len(open) != before {
		t.Errorf("zero-share extra passages changed edge count from %d to %d", before, len(open))
	}
}

func TestAddExtraPassages_PositiveShareAddsACycle(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	islands := map[int]bool{}
	open := buildBacktracker(g, islands, newStream([32]byte{3}))
	treeEdges := len(open)
	addExtraPassages(g, islands, open, newStream([32]byte{4}), decimal.New(2, -1))
	if len(open) <= treeEdges {
		t.Fatalf("positive-share extra passages did not add any edge: %d -> %d", treeEdges, len(open))
	}
	nonIslandCount := g.cellCount() - len(islands)
	want := roundHalfUp(decimal.New(2, -1).Mul(decimal.NewFromInt(int64(nonIslandCount - 1))))
	if int64(len(open)-treeEdges) != want {
		t.Errorf("extra passages added = %d, want %d", len(open)-treeEdges, want)
	}
}

func TestAddExtraPassages_CappedByAvailability(t *testing.T) {
	t.Parallel()
	// A share of 1 asks for as many extra passages as the tree itself
	// has edges, which the lattice's remaining closed faces cannot
	// always supply — the pass must cap rather than panic or overshoot.
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	islands := map[int]bool{}
	open := buildBacktracker(g, islands, newStream([32]byte{5}))
	addExtraPassages(g, islands, open, newStream([32]byte{6}), decimal.NewFromInt(1))
	maxPossible := len(g.interiorFaces())
	if len(open) > maxPossible {
		t.Errorf("open edge count %d exceeds the total interior face count %d", len(open), maxPossible)
	}
}

func TestAddExtraPassages_NeverOpensABorderFace(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	islands := map[int]bool{}
	open := buildBacktracker(g, islands, newStream([32]byte{7}))
	addExtraPassages(g, islands, open, newStream([32]byte{8}), decimal.NewFromInt(1))

	interior := map[edgeKey]bool{}
	for _, f := range g.interiorFaces() {
		interior[newEdgeKey(f.a, f.b)] = true
	}
	for k := range open {
		if !interior[k] {
			t.Errorf("open edge %v is not an interior face of the chunk", k)
		}
	}
}
