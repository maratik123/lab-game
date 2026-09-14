package gate

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// TestLowerBound_HoldsAndIsAttained checks lowerBound against brute
// force over an actual Lattice: no cell of the origin chunk is nearer
// than the bound to the centre of any chunk in the ring the bound is
// computed for, and at ring 2R+1 the least distance from the origin
// chunk's own centre to a centre in that ring is exactly 3R²+3R+1.
func TestLowerBound_HoldsAndIsAttained(t *testing.T) {
	t.Parallel()
	const maxRadius = 5
	for radius := int32(0); radius <= maxRadius; radius++ {
		lat := hexgrid.Lattice{Radius: radius}
		r := int64(radius)
		localCells := lat.LocalCells()
		for ring := int64(0); ring <= 2*r+1; ring++ {
			bound := lowerBound(r, ring)
			ringChunks := ringChunksAround(hexgrid.Chunk{}, ring)
			for _, cell := range localCells {
				for _, ch := range ringChunks {
					if d := hexgrid.Distance(cell, lat.Center(ch)); d < bound {
						t.Errorf("radius %d ring %d: cell %v to chunk %v centre distance %d < lower bound %d",
							radius, ring, cell, ch, d, bound)
					}
				}
			}
		}

		s := 2*r + 1
		ringChunks := ringChunksAround(hexgrid.Chunk{}, s)
		origin := hexgrid.Coord{}
		least := int64(-1)
		for _, ch := range ringChunks {
			if d := hexgrid.Distance(origin, lat.Center(ch)); least == -1 || d < least {
				least = d
			}
		}
		want := 3*r*r + 3*r + 1
		if least != want {
			t.Errorf("radius %d: least centre-to-centre distance over ring %d = %d, want %d", radius, s, least, want)
		}
	}
}

func drawGates(rt *rapid.T) []hexgrid.Chunk {
	n := rapid.IntRange(1, 6).Draw(rt, "n_gates")
	gates := make([]hexgrid.Chunk, n)
	for i := range gates {
		gates[i] = hexgrid.Chunk{
			Q: int32(rapid.Int64Range(-15, 15).Draw(rt, "gq")),
			R: int32(rapid.Int64Range(-15, 15).Draw(rt, "gr")),
		}
	}
	return gates
}

// TestSearch_StaysWithinBound checks depthSearch's own stated cost
// bound: it never visits a ring past S(cell), the largest ring the
// returned depth's lower bound could still admit.
func TestSearch_StaysWithinBound(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		radius := rapid.SampledFrom([]int32{0, 6, 8, 10, 12}).Draw(rt, "radius")
		lat := hexgrid.Lattice{Radius: radius}
		gates := drawGates(rt)
		set, err := NewSet(lat, gates)
		if err != nil {
			rt.Fatalf("NewSet: %v", err)
		}

		bound := int64(60) * (2*int64(radius) + 1)
		cell := hexgrid.Coord{
			Q: int32(rapid.Int64Range(-bound, bound).Draw(rt, "cq")),
			R: int32(rapid.Int64Range(-bound, bound).Draw(rt, "cr")),
		}

		depth, found, lastRing := set.depthSearch(cell)
		if !found {
			rt.Fatalf("depthSearch(%v) reported no gate", cell)
		}
		r := int64(radius)
		c := 3*r*r + 3*r + 1
		s := (depth + r) * (2*r + 1) / c
		if lastRing > s {
			rt.Fatalf("depthSearch(%v): last visited ring %d exceeds S=%d for depth %d", cell, lastRing, s, depth)
		}
	})
}

// TestSearch_FarGatesAddNoWork checks that gates beyond the search's own
// bound change neither the reported depth nor the rings the search
// visits — a stop rule that instead kept searching until every gate had
// been seen would visit more rings once far gates were added.
func TestSearch_FarGatesAddNoWork(t *testing.T) {
	t.Parallel()
	lat := hexgrid.Lattice{Radius: 8}
	nearGates := []hexgrid.Chunk{{Q: 0, R: 0}, {Q: 1, R: -1}}
	cell := hexgrid.Coord{Q: 3, R: -2}

	baseSet, err := NewSet(lat, nearGates)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	depth, found, lastRing := baseSet.depthSearch(cell)
	if !found {
		t.Fatalf("depthSearch(%v) reported no gate", cell)
	}

	r := int64(lat.Radius)
	c := 3*r*r + 3*r + 1
	s := (depth + r) * (2*r + 1) / c

	c0, _ := lat.Locate(cell)
	far := append([]hexgrid.Chunk(nil), nearGates...)
	for ring := s + 2; ring < s+52; ring++ {
		far = append(far, ringChunksAround(c0, ring)...)
	}

	fullSet, err := NewSet(lat, far)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	depth2, found2, lastRing2 := fullSet.depthSearch(cell)
	if !found2 || depth2 != depth || lastRing2 != lastRing {
		t.Errorf("adding gates beyond the search bound changed the result: (%d,%v,%d) -> (%d,%v,%d)",
			depth, found, lastRing, depth2, found2, lastRing2)
	}
}
