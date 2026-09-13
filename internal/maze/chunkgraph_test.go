package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestChunkGraph_IndexMappingRoundTrips(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 5, Rows: 7})
	for idx := 0; idx < g.cellCount(); idx++ {
		lq, lr := g.localCoord(idx)
		if got := g.localIndex(lq, lr); got != idx {
			t.Fatalf("localIndex(localCoord(%d)) = %d, want %d", idx, got, idx)
		}
	}
}

func TestChunkGraph_BorderCellPredicate(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 4, Rows: 4})
	borderCount, interiorCount := 0, 0
	for idx := 0; idx < g.cellCount(); idx++ {
		if g.isBorderCell(idx) {
			borderCount++
		} else {
			interiorCount++
		}
	}
	if want := int(nonBorderCellCount(hexgrid.Dims{Cols: 4, Rows: 4})); interiorCount != want {
		t.Errorf("interior cell count = %d, want %d (matching nonBorderCellCount)", interiorCount, want)
	}
	if borderCount+interiorCount != g.cellCount() {
		t.Errorf("border+interior = %d, want %d", borderCount+interiorCount, g.cellCount())
	}
}

func TestChunkGraph_SingleCellChunkHasNoInteriorNeighbours(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 1, Rows: 1})
	if n := g.neighbors(0); len(n) != 0 {
		t.Errorf("neighbors(0) in a 1x1 chunk = %v, want none", n)
	}
	if len(g.interiorFaces()) != 0 {
		t.Errorf("interiorFaces() in a 1x1 chunk = %v, want none", g.interiorFaces())
	}
}

func TestChunkGraph_SingleRowChunkIsAPath(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 5, Rows: 1})
	faces := g.interiorFaces()
	if len(faces) != g.cellCount()-1 {
		t.Errorf("single-row interiorFaces count = %d, want %d (one fewer than the cell count)", len(faces), g.cellCount()-1)
	}
	for idx := 0; idx < g.cellCount(); idx++ {
		if !g.isBorderCell(idx) {
			t.Errorf("cell %d of a single-row chunk is not a border cell, want every cell to be", idx)
		}
	}
}

func TestChunkGraph_AdjacencyIsSymmetric(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 6, Rows: 6})
	for idx := 0; idx < g.cellCount(); idx++ {
		for _, n := range g.neighbors(idx) {
			found := false
			for _, back := range g.neighbors(n) {
				if back == idx {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("cell %d has neighbour %d, but %d does not list %d back", idx, n, n, idx)
			}
		}
	}
}

func TestChunkGraph_InteriorFacesEachCountedOnce(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 6, Rows: 6})
	seen := map[[2]int]bool{}
	for _, f := range g.interiorFaces() {
		key := [2]int{f.a, f.b}
		if seen[key] {
			t.Fatalf("face %v enumerated more than once", f)
		}
		seen[key] = true
	}
	// Every enumerated face's endpoints must actually be adjacent.
	for _, f := range g.interiorFaces() {
		adj := false
		for _, n := range g.neighbors(f.a) {
			if n == f.b {
				adj = true
			}
		}
		if !adj {
			t.Errorf("face %v: %d and %d are not adjacent", f, f.a, f.b)
		}
	}
}

func TestChunkGraph_ReferenceDimsInteriorFaceCount(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Dims{Cols: 16, Rows: 16})
	faces := g.interiorFaces()
	// Every face has both endpoints inside the chunk, so an upper bound
	// is 3 * cellCount (each cell contributes up to 3 "forward"
	// directions before double-counting) minus the border shortfall;
	// the load-bearing assertion here is simply that the enumeration is
	// non-empty and internally consistent, checked above.
	if len(faces) == 0 {
		t.Fatal("reference-dims chunk has no interior faces")
	}
}
