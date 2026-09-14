package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func refLattice() hexgrid.Lattice {
	return hexgrid.Lattice{Radius: MinRadius}
}

func TestSelectIslands_ZeroShareYieldsNoIsland(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p := validParams()
	p.IslandShare = decimal.Zero
	s := newStream([32]byte{1})
	islands := selectIslands(g, s, p, ChunkTypeFabric)
	if len(islands) != 0 {
		t.Errorf("selectIslands with zero share = %v, want none", islands)
	}
}

func TestSelectIslands_NoIslandIsABorderCell(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p := validParams()
	for seedByte := range 20 {
		s := newStream([32]byte{byte(seedByte)})
		islands := selectIslands(g, s, p, ChunkTypeFabric)
		for idx := range islands {
			if g.isBorderCell(idx) {
				t.Fatalf("seed %d: island set contains border cell %d", seedByte, idx)
			}
		}
	}
}

func TestSelectIslands_NonIslandCellsStayConnectedOverInducedSubgraph(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p := validParams()
	for seedByte := range 20 {
		s := newStream([32]byte{byte(seedByte), 7})
		islands := selectIslands(g, s, p, ChunkTypeFabric)
		var start int
		for idx := 0; idx < g.cellCount(); idx++ {
			if !islands[idx] {
				start = idx
				break
			}
		}
		if !connectedOverInduced(g, islands, start) {
			t.Fatalf("seed %d: non-island cells are not all connected; islands=%v", seedByte, islands)
		}
	}
}

// TestSelectIslands_GateChunkNeverSelectsTheCentre checks that, over a
// sweep at the reference island share, a gate chunk never selects its
// own centre cell as an island, while a fabric chunk (same seed, same
// params) sometimes does — a bare exclusion applied to every type would
// go red on that second half.
func TestSelectIslands_GateChunkNeverSelectsTheCentre(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p := validParams()
	centre := g.localIndex(hexgrid.Coord{})
	fabricCentreIsland := false
	for seedByte := range 40 {
		s := newStream([32]byte{byte(seedByte), 42})
		if islands := selectIslands(g, s, p, ChunkTypeGate); islands[centre] {
			t.Fatalf("seed %d: gate chunk selected its own centre as an island", seedByte)
		}
		s2 := newStream([32]byte{byte(seedByte), 42})
		if islands := selectIslands(g, s2, p, ChunkTypeFabric); islands[centre] {
			fabricCentreIsland = true
		}
	}
	if !fabricCentreIsland {
		t.Fatal("test setup: no fabric-chunk sweep selected the centre as an island, so the gate exclusion above was not discriminating")
	}
}

func TestSelectIslands_UnchangedByAlgorithmWeights(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p1 := validParams()
	p1.Weights = AlgorithmWeights{1, 0, 0, 0, 0}
	p2 := validParams()
	p2.Weights = AlgorithmWeights{0, 0, 0, 0, 1}

	s1 := newStream([32]byte{11})
	s2 := newStream([32]byte{11})
	i1 := selectIslands(g, s1, p1, ChunkTypeFabric)
	i2 := selectIslands(g, s2, p2, ChunkTypeFabric)
	if len(i1) != len(i2) {
		t.Fatalf("island sets differ in size across algorithm weights: %d vs %d", len(i1), len(i2))
	}
	for idx := range i1 {
		if !i2[idx] {
			t.Fatalf("island sets differ across algorithm weights: %v vs %v", i1, i2)
		}
	}
}

func TestIslandTarget_MatchesRoundHalfUpOfShareTimesTotalCells(t *testing.T) {
	t.Parallel()
	p := Params{Radius: MinRadius, IslandShare: decimal.New(5, -2)}
	got := islandTarget(p)
	want := roundHalfUp(decimal.New(5, -2).Mul(decimal.NewFromInt(refLattice().CellCount())))
	if got != want {
		t.Errorf("islandTarget = %d, want %d", got, want)
	}
}

func TestConnectedOverInduced_NoIslands(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	islands := map[int]bool{}
	if !connectedOverInduced(g, islands, 0) {
		t.Error("chunk with no islands is not reported connected from cell 0")
	}
}
