package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestChunkGraph_IndexMappingRoundTrips(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	for idx := 0; idx < g.cellCount(); idx++ {
		local := g.localCoord(idx)
		if got := g.localIndex(local); got != idx {
			t.Fatalf("localIndex(localCoord(%d)) = %d, want %d", idx, got, idx)
		}
	}
}

func TestChunkGraph_BorderCellPredicateMatchesDistance(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	borderCount, interiorCount := 0, 0
	for idx := 0; idx < g.cellCount(); idx++ {
		local := g.localCoord(idx)
		wantBorder := hexgrid.Distance(local, hexgrid.Coord{}) == 6
		if g.isBorderCell(idx) != wantBorder {
			t.Fatalf("isBorderCell(%d) = %v, want %v (distance %d)", idx, g.isBorderCell(idx), wantBorder, hexgrid.Distance(local, hexgrid.Coord{}))
		}
		if g.isBorderCell(idx) {
			borderCount++
		} else {
			interiorCount++
		}
	}
	if want := int(nonBorderCellCount(6)); interiorCount != want {
		t.Errorf("interior cell count = %d, want %d (matching nonBorderCellCount)", interiorCount, want)
	}
	if borderCount+interiorCount != g.cellCount() {
		t.Errorf("border+interior = %d, want %d", borderCount+interiorCount, g.cellCount())
	}
}

func TestChunkGraph_AdjacencyIsSymmetric(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
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

func TestChunkGraph_InteriorFacesEachCountedOnceAndAdjacent(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	seen := map[[2]int]bool{}
	for _, f := range g.interiorFaces() {
		key := [2]int{f.a, f.b}
		if seen[key] {
			t.Fatalf("face %v enumerated more than once", f)
		}
		seen[key] = true
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

// TestChunkGraph_NonBorderCellHasSixInteriorNeighbours checks that a
// cell chunkGraph never calls a border cell has all six of its
// hex-lattice neighbours inside the chunk.
func TestChunkGraph_NonBorderCellHasSixInteriorNeighbours(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	for idx := 0; idx < g.cellCount(); idx++ {
		if g.isBorderCell(idx) {
			continue
		}
		if n := len(g.neighbors(idx)); n != 6 {
			t.Errorf("non-border cell %v has %d interior neighbours, want 6", g.localCoord(idx), n)
		}
	}
}
