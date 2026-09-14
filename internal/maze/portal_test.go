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
// consecutive entries in candidates' own path order share a vertex.
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

// TestPortals_CountWithinRoundedUpShares checks that over a sweep of
// radii, chunk-pair directions, seeds and share pairs, the portal count
// lies within [⌈lower×L⌉, ⌈upper×L⌉]; the sweep observes both ends for
// the 0.1/0.2 pair, and a lo==hi share pair pins the count exactly.
func TestPortals_CountWithinRoundedUpShares(t *testing.T) {
	t.Parallel()
	radii := []int32{6, 7, 9, 12}
	sawLo, sawHi := false, false
	origin := hexgrid.Chunk{Q: 0, R: 0}
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		for _, d := range sixDirections {
			neighbor := origin.Neighbor(d)
			candidates := borderCandidates(lattice, origin, neighbor)
			lo, hi := portalBounds(refLowerShare, refUpperShare, len(candidates))
			for seedByte := range 60 {
				s := newStream([32]byte{byte(r), byte(d), byte(seedByte)})
				portals := selectPortals(candidates, s, refLowerShare, refUpperShare)
				if len(portals) < lo || len(portals) > hi {
					t.Fatalf("radius %d direction %v seed %d: portal count = %d, want within [%d,%d]", r, d, seedByte, len(portals), lo, hi)
				}
				if r == MinRadius && d == hexgrid.DirE {
					if len(portals) == lo {
						sawLo = true
					}
					if len(portals) == hi {
						sawHi = true
					}
				}
			}
		}
	}
	if !sawLo || !sawHi {
		t.Errorf("over the sweep at radius %d, sawLo=%v sawHi=%v, want both ends observed", MinRadius, sawLo, sawHi)
	}

	// A lo==hi share pair pins the count to that exact value, over
	// several chunk pairs and seeds.
	pinned := decimal.New(2, -1)
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		for _, d := range sixDirections {
			neighbor := origin.Neighbor(d)
			candidates := borderCandidates(lattice, origin, neighbor)
			lo, hi := portalBounds(pinned, pinned, len(candidates))
			if lo != hi {
				t.Fatalf("radius %d direction %v: portalBounds(0.2,0.2,%d) = (%d,%d), want lo==hi", r, d, len(candidates), lo, hi)
			}
			for seedByte := range 10 {
				s := newStream([32]byte{byte(r), byte(d), byte(seedByte), 7})
				portals := selectPortals(candidates, s, pinned, pinned)
				if len(portals) != lo {
					t.Fatalf("radius %d direction %v seed %d: portal count = %d, want exactly %d", r, d, seedByte, len(portals), lo)
				}
			}
		}
	}
}

// facesShareVertex decides vertex sharing independently of
// Lattice.Border's own path order: a hex vertex is touched by exactly
// three mutually adjacent cells, so two distinct faces {x,y} and {u,v}
// share one exactly when {x,y,u,v} collapses to three cells — one cell
// common to both — and the two cells left over are themselves adjacent,
// closing the triangle.
func facesShareVertex(f, g hexgrid.Face) bool {
	fx, fy := f.Cell, f.Cell.Neighbor(f.Dir)
	gx, gy := g.Cell, g.Cell.Neighbor(g.Dir)
	if (fx == gx && fy == gy) || (fx == gy && fy == gx) {
		return false
	}
	adjacent := func(a, b hexgrid.Coord) bool {
		for _, d := range sixDirections {
			if a.Neighbor(d) == b {
				return true
			}
		}
		return false
	}
	switch {
	case fx == gx:
		return adjacent(fy, gy)
	case fx == gy:
		return adjacent(fy, gx)
	case fy == gx:
		return adjacent(fx, gy)
	case fy == gy:
		return adjacent(fx, gx)
	default:
		return false
	}
}

// TestPortals_NoTwoPortalsOfABorderShareAVertex checks, over several
// chunk pairs and radii, that no two portal faces of one border share a
// vertex under the independent facesShareVertex predicate, reading the
// actual portal faces through Generate and Map.Faces rather than
// through candidates' own list order.
func TestPortals_NoTwoPortalsOfABorderShareAVertex(t *testing.T) {
	t.Parallel()
	radii := []int32{6, 7, 9, 12}
	for _, r := range radii {
		lattice := hexgrid.Lattice{Radius: r}
		origin := hexgrid.Chunk{Q: 0, R: 0}
		params := refParams()
		params.Radius = r
		for _, d := range sixDirections {
			neighbor := origin.Neighbor(d)
			candidates := borderCandidates(lattice, origin, neighbor)
			for seedByte := range 10 {
				seed := int64(r)<<16 | int64(d)<<8 | int64(seedByte)
				gen, err := New(seed, params)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				m, err := gen.Generate(origin, ChunkTypeFabric)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				var portals []hexgrid.Face
				for _, f := range candidates {
					// A candidate's canonical Cell may sit in either
					// chunk's own frame (FaceOf picks the direction, not
					// a side), so resolve origin's own local cell and
					// direction for this face before reading m.Faces.
					originCell, originDir := f.Cell, f.Dir
					if ch, _ := lattice.Locate(f.Cell); ch != origin {
						originCell, originDir = f.Cell.Neighbor(f.Dir), f.Dir.Opposite()
					}
					_, local := lattice.Locate(originCell)
					faces, ok := m.Faces(local)
					if !ok {
						t.Fatalf("Faces(%v): ok=false", local)
					}
					if faces[originDir] == FacePassage {
						portals = append(portals, f)
					}
				}
				for i := range portals {
					for j := range portals {
						if i != j && facesShareVertex(portals[i], portals[j]) {
							t.Fatalf("radius %d direction %v seed %d: portals %v and %v share a vertex",
								r, d, seedByte, portals[i], portals[j])
						}
					}
				}
			}
		}
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

// TestBorder_IndependentOfChunkTypeAndThirdChunks checks three clauses
// of the same independence: the pair-level clause, that A's border
// candidate faces equal, face by face, B's side, and depend on the
// pair's own border key alone with no third chunk entering the
// derivation; the type clause, that A's border with B is identical
// whether A is generated fabric or gate; and the stored-neighbour
// clause, that A's border with B does not change when NewMap-built
// maps are supplied for A's other neighbours.
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

	// Stored-neighbour clause: A's border with B is identical with and
	// without NewMap-built maps supplied for A's OTHER neighbours.
	var otherNeighborMaps []Map
	for _, d := range sixDirections {
		nc := a.Neighbor(d)
		if nc == b {
			continue
		}
		m, err := gen.Generate(nc, ChunkTypeFabric)
		if err != nil {
			t.Fatalf("Generate(%v,fabric): %v", nc, err)
		}
		otherNeighborMaps = append(otherNeighborMaps, storedCopyOf(t, m))
	}
	withOthers, err := gen.Generate(a, ChunkTypeFabric, otherNeighborMaps...)
	if err != nil {
		t.Fatalf("Generate(a,fabric,others...): %v", err)
	}
	for _, f := range candidatesFromA {
		_, aLocal := lattice.Locate(f.Cell)
		want := direct[f]
		got := func() bool {
			faces, ok := withOthers.Faces(aLocal)
			if !ok {
				t.Fatalf("Faces(%v): ok=false", aLocal)
			}
			return faces[f.Dir] == FacePassage
		}()
		if got != want {
			t.Errorf("A's border with B changed when unrelated neighbour maps were supplied: face %v got %v, want %v", f, got, want)
		}
	}
}
