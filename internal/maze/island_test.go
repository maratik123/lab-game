package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestSelectIslands_ZeroShareYieldsNoIsland(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 16, Rows: 16})
	p := validParams()
	p.IslandShare = decimal.Zero
	s := newStream([32]byte{1})
	islands := selectIslands(g, s, p)
	if len(islands) != 0 {
		t.Errorf("selectIslands with zero share = %v, want none", islands)
	}
}

func TestSelectIslands_NoIslandIsABorderCell(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 16, Rows: 16})
	p := validParams()
	for seedByte := range 20 {
		s := newStream([32]byte{byte(seedByte)})
		islands := selectIslands(g, s, p)
		for idx := range islands {
			if g.isBorderCell(idx) {
				t.Fatalf("seed %d: island set contains border cell %d", seedByte, idx)
			}
		}
	}
}

func TestSelectIslands_NonIslandCellsStayConnectedOverInducedSubgraph(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 16, Rows: 16})
	p := validParams()
	for seedByte := range 20 {
		s := newStream([32]byte{byte(seedByte), 7})
		islands := selectIslands(g, s, p)
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

func TestSelectIslands_DegenerateShapesYieldNoIslandAtZeroShare(t *testing.T) {
	t.Parallel()
	for _, d := range []hexgrid.Dims{{Cols: 1, Rows: 1}, {Cols: 1, Rows: 16}, {Cols: 16, Rows: 1}} {
		g := newChunkGraph(d)
		p := validParams()
		p.Dims = d
		p.IslandShare = decimal.Zero
		s := newStream([32]byte{3})
		if islands := selectIslands(g, s, p); len(islands) != 0 {
			t.Errorf("dims %+v: selectIslands at zero share = %v, want none", d, islands)
		}
	}
}

func TestSelectIslands_UnchangedByAlgorithmWeights(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 16, Rows: 16})
	p1 := validParams()
	p1.Weights = AlgorithmWeights{1, 0, 0, 0, 0}
	p2 := validParams()
	p2.Weights = AlgorithmWeights{0, 0, 0, 0, 1}

	s1 := newStream([32]byte{11})
	s2 := newStream([32]byte{11})
	i1 := selectIslands(g, s1, p1)
	i2 := selectIslands(g, s2, p2)
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
	p := Params{Dims: hexgrid.Dims{Cols: 16, Rows: 16}, IslandShare: decimal.New(5, -2)}
	got := islandTarget(p)
	want := roundHalfUp(decimal.New(5, -2).Mul(decimal.NewFromInt(256)))
	if got != want {
		t.Errorf("islandTarget = %d, want %d", got, want)
	}
}

func TestConnectedOverInduced_SingleCellChunk(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 1, Rows: 1})
	if !connectedOverInduced(g, map[int]bool{}, 0) {
		t.Error("single-cell chunk with no islands is not reported connected")
	}
}
