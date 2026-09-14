package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var (
	refLowerShare = decimal.New(1, -1)
	refUpperShare = decimal.New(2, -1)
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

// nonConsecutiveInPositionList reports whether none of the chosen
// positions (indices into candidates) are consecutive integers — the
// vertex-touching predicate for two portals of one border, since
// consecutive entries in candidates' own path order share a vertex
// (Lattice.Border's own contract).
func nonConsecutiveInPositionList(positions []int) bool {
	for i := range positions {
		for j := range positions {
			if i != j {
				diff := positions[i] - positions[j]
				if diff == 1 || diff == -1 {
					return false
				}
			}
		}
	}
	return true
}

func positionsOf(candidates []hexgrid.Face, portals map[hexgrid.Face]bool) []int {
	var out []int
	for i, c := range candidates {
		if portals[c] {
			out = append(out, i)
		}
	}
	return out
}

// TestPortals_CountWithinRoundedUpShares checks that over a sweep of
// radii, seeds and share pairs, the portal count lies within
// [⌈lower×L⌉, ⌈upper×L⌉], and the sweep observes both ends for the
// 0.1/0.2 pair.
func TestPortals_CountWithinRoundedUpShares(t *testing.T) {
	t.Parallel()
	radii := []int32{6, 7, 9, 12}
	sawLo, sawHi := false, false
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		origin := hexgrid.Chunk{Q: 0, R: 0}
		east := origin.Neighbor(hexgrid.DirE)
		candidates := borderCandidates(lattice, origin, east)
		lo, hi := portalBounds(refLowerShare, refUpperShare, len(candidates))
		for seedByte := range 60 {
			s := newStream([32]byte{byte(r), byte(seedByte)})
			portals := selectPortals(candidates, s, refLowerShare, refUpperShare)
			if len(portals) < lo || len(portals) > hi {
				t.Fatalf("radius %d seed %d: portal count = %d, want within [%d,%d]", r, seedByte, len(portals), lo, hi)
			}
			if r == MinRadius {
				if len(portals) == lo {
					sawLo = true
				}
				if len(portals) == hi {
					sawHi = true
				}
			}
		}
	}
	if !sawLo || !sawHi {
		t.Errorf("over the sweep at radius %d, sawLo=%v sawHi=%v, want both ends observed", MinRadius, sawLo, sawHi)
	}
}

// TestPortals_NoTwoPortalsOfABorderShareAVertex checks a border's
// portals against the independent nonConsecutiveInPositionList
// predicate, and confirms the predicate itself is seen RED against an
// adjacent-positions mutant.
func TestPortals_NoTwoPortalsOfABorderShareAVertex(t *testing.T) {
	t.Parallel()
	radii := []int32{6, 7, 9, 12}
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		origin := hexgrid.Chunk{Q: 0, R: 0}
		east := origin.Neighbor(hexgrid.DirE)
		candidates := borderCandidates(lattice, origin, east)
		for seedByte := range 30 {
			s := newStream([32]byte{byte(r), byte(seedByte), 1})
			portals := selectPortals(candidates, s, refLowerShare, refUpperShare)
			positions := positionsOf(candidates, portals)
			if !nonConsecutiveInPositionList(positions) {
				t.Fatalf("radius %d seed %d: portal positions %v are not pairwise non-consecutive", r, seedByte, positions)
			}
		}
	}
	// The predicate itself must be discriminating: an adjacent-positions
	// mutant is seen RED.
	if nonConsecutiveInPositionList([]int{2, 3}) {
		t.Fatal("test setup: nonConsecutiveInPositionList did not flag adjacent positions 2,3")
	}
}

func TestNonConsecutivePositions_NeverAdjacent(t *testing.T) {
	t.Parallel()
	for l := 3; l <= 25; l++ {
		for count := 1; count <= (l+1)/2; count++ {
			s := newStream([32]byte{byte(l), byte(count)})
			positions := nonConsecutivePositions(s, l, count)
			if len(positions) != count {
				t.Fatalf("l=%d count=%d: got %d positions, want %d", l, count, len(positions), count)
			}
			for _, p := range positions {
				if p < 0 || p >= l {
					t.Fatalf("l=%d count=%d: position %d out of range", l, count, p)
				}
			}
			if !nonConsecutiveInPositionList(positions) {
				t.Fatalf("l=%d count=%d: positions %v are not pairwise non-consecutive", l, count, positions)
			}
		}
	}
}

func TestSelectPortals_EmptyCandidatesYieldsNoPortal(t *testing.T) {
	t.Parallel()
	if portals := selectPortals(nil, newStream([32]byte{1}), refLowerShare, refUpperShare); len(portals) != 0 {
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
	a := selectPortals(candidates, newStream(key), refLowerShare, refUpperShare)
	b := selectPortals(candidates, newStream(key), refLowerShare, refUpperShare)
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
	direct := selectPortals(candidatesFromA, newStream(borderKey(seed, a, b)), refLowerShare, refUpperShare)
	gen, err := New(seed, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Type clause: A's border with B is identical whether A is typed
	// fabric or gate, and equals B's side of the same border.
	for _, aType := range []ChunkType{ChunkTypeFabric, ChunkTypeGate} {
		mapA, err := gen.Generate(a, aType)
		if err != nil {
			t.Fatalf("Generate(a,%v): %v", aType, err)
		}
		mapB, err := gen.Generate(b, ChunkTypeFabric)
		if err != nil {
			t.Fatalf("Generate(b,fabric): %v", err)
		}
		for _, f := range candidatesFromA {
			want := direct[f]
			_, aLocal := lattice.Locate(f.Cell)
			aFaces, ok := mapA.Faces(aLocal)
			if !ok {
				t.Fatalf("Faces(%v) in chunk a (type %v): ok=false", aLocal, aType)
			}
			got := aFaces[f.Dir] == FacePassage
			if got != want {
				t.Errorf("A typed %v, face %v: direct portal rule says %v, Map says %v", aType, f, want, got)
			}

			neighborCell := f.Cell.Neighbor(f.Dir)
			if bChunk, _ := lattice.Locate(neighborCell); bChunk != b {
				continue
			}
			_, bLocal := lattice.Locate(neighborCell)
			bFaces, ok := mapB.Faces(bLocal)
			if !ok {
				t.Fatalf("Faces(%v) in chunk b: ok=false", bLocal)
			}
			if bGot := bFaces[f.Dir.Opposite()] == FacePassage; bGot != got {
				t.Errorf("face %v: A side says %v, B side says %v", f, got, bGot)
			}
		}
	}
}
