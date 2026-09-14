package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestBorderCandidates_AgreeFromEitherSide(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	a := hexgrid.Chunk{Q: 0, R: 0}
	b := hexgrid.Chunk{Q: 1, R: 0}
	fromA := borderCandidates(lattice, a, b)
	fromB := borderCandidates(lattice, b, a)
	if len(fromA) != len(fromB) {
		t.Fatalf("borderCandidates(a,b) has %d entries, borderCandidates(b,a) has %d", len(fromA), len(fromB))
	}
	for i := range fromA {
		if fromA[i] != fromB[i] {
			t.Errorf("entry %d differs: %v vs %v", i, fromA[i], fromB[i])
		}
	}
}

func TestBorderCandidates_HasTwoRPlusOneEntriesForEveryNeighbourDirection(t *testing.T) {
	t.Parallel()
	radii := []int32{6, 7, 9}
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		origin := hexgrid.Chunk{Q: 0, R: 0}
		for _, d := range sixDirections {
			n := origin.Neighbor(d)
			candidates := borderCandidates(lattice, origin, n)
			if want := 2*int(r) + 1; len(candidates) != want {
				t.Errorf("radius %d direction %v: borderCandidates has %d entries, want %d", r, d, len(candidates), want)
			}
		}
	}
}

func TestSelectPortals_OneOrTwoCappedByCandidates(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}
	candidates := borderCandidates(lattice, origin, east)

	sawOne, sawTwo := false, false
	for seedByte := range 50 {
		s := newStream([32]byte{byte(seedByte), 9})
		portals := selectPortals(candidates, s)
		if len(portals) < 1 || len(portals) > 2 {
			t.Fatalf("seed %d: portal count = %d, want 1 or 2", seedByte, len(portals))
		}
		for f := range portals {
			found := false
			for _, c := range candidates {
				if c == f {
					found = true
				}
			}
			if !found {
				t.Errorf("seed %d: portal %v is not among the candidates", seedByte, f)
			}
		}
		if len(portals) == 1 {
			sawOne = true
		}
		if len(portals) == 2 {
			sawTwo = true
		}
	}
	if !sawOne || !sawTwo {
		t.Errorf("over 50 seeds, sawOne=%v sawTwo=%v, want both to occur", sawOne, sawTwo)
	}
}

func TestSelectPortals_EmptyCandidatesYieldsNoPortal(t *testing.T) {
	t.Parallel()
	if portals := selectPortals(nil, newStream([32]byte{1})); len(portals) != 0 {
		t.Errorf("selectPortals(nil, _) = %v, want none", portals)
	}
}

func TestSelectPortals_ReproducibleFromTheSameKey(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}
	candidates := borderCandidates(lattice, origin, east)
	key := [32]byte{4, 4, 4}
	a := selectPortals(candidates, newStream(key))
	b := selectPortals(candidates, newStream(key))
	if len(a) != len(b) {
		t.Fatalf("portal count differs across identical keys: %d vs %d", len(a), len(b))
	}
	for f := range a {
		if !b[f] {
			t.Errorf("portal sets differ across identical keys: %v vs %v", a, b)
		}
	}
}

// TestBorder_IndependentOfChunkTypeAndThirdChunks (pair-level, subtask
// 3/4): a border's candidate faces from A's side equal, face by face,
// those from B's side, and depend on the pair's own border key alone —
// no third chunk enters the derivation.
func TestBorder_IndependentOfChunkTypeAndThirdChunks(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	a := hexgrid.Chunk{Q: 0, R: 0}
	b := hexgrid.Chunk{Q: 1, R: 0}
	candidatesFromA := borderCandidates(lattice, a, b)
	candidatesFromB := borderCandidates(lattice, b, a)
	for i := range candidatesFromA {
		if candidatesFromA[i] != candidatesFromB[i] {
			t.Fatalf("entry %d differs between sides: %v vs %v", i, candidatesFromA[i], candidatesFromB[i])
		}
	}
	seed := int64(20260912)
	direct := selectPortals(candidatesFromA, newStream(borderKey(seed, a, b)))
	viaCell, err := New(seed, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, d := range sixDirections {
		if a.Neighbor(d) != b {
			continue
		}
		for _, f := range candidatesFromA {
			want := direct[f]
			got := viaCell.Cell(f.Cell).Faces[f.Dir] == FacePassage
			if got != want {
				t.Errorf("face %v: direct portal rule says %v, Cell says %v", f, want, got)
			}
		}
	}
}
