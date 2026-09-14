package maze

import (
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// farCoordBases are the magnitudes exercised for "far from the origin":
// well past the region the exhaustive face-agreement sweep covers, up
// to the edge of the int32 domain Coord itself uses.
var farCoordBases = []int32{1 << 20, 1 << 28, 2147483600}

// drawFarCoord draws a coordinate whose Q and R each sit near one of
// farCoordBases, on either side of zero and jittered by a few cells so
// the case does not always land on the exact base value.
func drawFarCoord(rt *rapid.T) hexgrid.Coord {
	drawAxis := func(label string) int32 {
		base := farCoordBases[rapid.IntRange(0, len(farCoordBases)-1).Draw(rt, label+"_base")]
		if rapid.Bool().Draw(rt, label+"_neg") {
			base = -base
		}
		jitter := int32(rapid.IntRange(-3, 3).Draw(rt, label+"_jitter"))
		return base + jitter
	}
	return hexgrid.Coord{Q: drawAxis("q"), R: drawAxis("r")}
}

// islandRegionCells returns, for a rectangular block of chunks under
// dims spanning loChunk..hiChunk inclusive (both axes), the coordinate
// of every cell in the region and — separately — which of them
// selectIslands would mark as an island in its own chunk.
func islandRegionCells(dims hexgrid.Dims, seed int64, params Params, loChunk, hiChunk hexgrid.Chunk) (all []hexgrid.Coord, islandSet map[hexgrid.Coord]bool) {
	islandSet = map[hexgrid.Coord]bool{}
	for cq := loChunk.Q; cq <= hiChunk.Q; cq++ {
		for cr := loChunk.R; cr <= hiChunk.R; cr++ {
			ch := hexgrid.Chunk{Q: cq, R: cr}
			g := newChunkGraph(dims)
			islands := selectIslands(g, newStream(chunkKey(seed, purposeIsland, ch)), params)
			origin := dims.Origin(ch)
			for lr := int32(0); lr < dims.Rows; lr++ {
				for lq := int32(0); lq < dims.Cols; lq++ {
					c := hexgrid.Coord{Q: origin.Q + lq, R: origin.R + lr}
					all = append(all, c)
					if islands[g.localIndex(lq, lr)] {
						islandSet[c] = true
					}
				}
			}
		}
	}
	return all, islandSet
}

// TestIslandsAreWalled_EveryIslandFaceIsAWallFromBothSides asserts the
// island-walled property at the only entry point that can fail here:
// Cell, not the island selector. Connectivity (asserting the non-island
// set is one component) is blind to a face wrongly carved into an
// island, and face agreement is blind to a passage into an island that
// both sides happen to agree on — this scenario needs its own
// assertion for exactly that reason.
func TestIslandsAreWalled_EveryIslandFaceIsAWallFromBothSides(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	gen, err := New(goldenSeed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lo := hexgrid.Chunk{Q: -2, R: -2}
	hi := hexgrid.Chunk{Q: 1, R: 1}
	all, islandSet := islandRegionCells(params.Dims, goldenSeed, params, lo, hi)
	if len(islandSet) == 0 {
		t.Fatal("test setup: expected at least one island over this region")
	}

	for c := range islandSet {
		cell := gen.Cell(c)
		for _, f := range cell.Faces {
			if f != FaceWall {
				t.Errorf("island cell %v has a non-wall face: %+v", c, cell.Faces)
			}
		}
		for _, d := range sixDirections {
			n := c.Neighbor(d)
			nCell := gen.Cell(n)
			if nCell.Faces[d.Opposite()] != FaceWall {
				t.Errorf("island cell %v's neighbour %v (dir %v) reads a non-wall face into it: %v", c, n, d.Opposite(), nCell.Faces[d.Opposite()])
			}
		}
	}

	// The passage-reachable component over the region must equal
	// exactly the non-island set — set equality, not "one component" —
	// which also catches the opposite error, a non-island cell left
	// unreachable.
	regionSet := map[hexgrid.Coord]bool{}
	for _, c := range all {
		regionSet[c] = true
	}
	var start hexgrid.Coord
	found := false
	for _, c := range all {
		if !islandSet[c] {
			start = c
			found = true
			break
		}
	}
	if !found {
		t.Fatal("test setup: region has no non-island cell")
	}
	visited := map[hexgrid.Coord]bool{start: true}
	stack := []hexgrid.Coord{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		curCell := gen.Cell(cur)
		for _, d := range sixDirections {
			if curCell.Faces[d] != FacePassage {
				continue
			}
			n := cur.Neighbor(d)
			if !regionSet[n] || visited[n] {
				continue
			}
			visited[n] = true
			stack = append(stack, n)
		}
	}
	for _, c := range all {
		isIsland := islandSet[c]
		isVisited := visited[c]
		if isIsland == isVisited {
			// island and visited must never coincide: an island must
			// be unvisited, a non-island must be visited.
			if isIsland {
				t.Errorf("island cell %v was reached by the passage flood-fill", c)
			} else {
				t.Errorf("non-island cell %v was not reached by the passage flood-fill", c)
			}
		}
	}
}

// TestIslandShare_AchievedShareWithinTolerance pins every part of its
// instrument: dims 16x16, island share 0.05, the chunk block
// (-2,-2)..(1,1), an absolute tolerance of ±0.01 on the achieved share.
func TestIslandShare_AchievedShareWithinTolerance(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	lo := hexgrid.Chunk{Q: -2, R: -2}
	hi := hexgrid.Chunk{Q: 1, R: 1}
	all, islandSet := islandRegionCells(params.Dims, goldenSeed, params, lo, hi)

	achieved := decimal.NewFromInt(int64(len(islandSet))).Div(decimal.NewFromInt(int64(len(all))))
	target := params.IslandShare
	diff := achieved.Sub(target).Abs()
	tolerance := decimal.New(1, -2)
	if diff.GreaterThan(tolerance) {
		t.Errorf("achieved island share %s vs target %s: |diff|=%s exceeds tolerance %s (islands=%d of %d cells)",
			achieved, target, diff, tolerance, len(islandSet), len(all))
	}
}

// TestAlgorithmDraw_SingleWeightSweep asserts that under a weight set
// giving weight to exactly one algorithm, every chunk of a sweep draws
// that one, for each of the five in turn.
func TestAlgorithmDraw_SingleWeightSweep(t *testing.T) {
	t.Parallel()
	algos := []Algorithm{AlgorithmBacktracker, AlgorithmKruskal, AlgorithmPrim, AlgorithmGrowingTree, AlgorithmWilson}
	for _, want := range algos {
		var w AlgorithmWeights
		w[want] = 1
		for cq := int32(0); cq < 10; cq++ {
			for cr := int32(0); cr < 10; cr++ {
				ch := hexgrid.Chunk{Q: cq, R: cr}
				got := drawAlgorithm(newStream(chunkKey(555, purposeAlgorithm, ch)), w)
				if got != want {
					t.Fatalf("algorithm %v single-weight sweep: chunk %v drew %v", want, ch, got)
				}
			}
		}
	}
}

// TestConnectivity_MultiChunkRegionOverASeedSweep checks multi-chunk
// connectivity over a handful of world seeds.
func TestConnectivity_MultiChunkRegionOverASeedSweep(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	for _, seed := range []int64{1, 2, 3, 20260912} {
		gen, err := New(seed, params)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		lo := hexgrid.Chunk{Q: 0, R: 0}
		hi := hexgrid.Chunk{Q: 1, R: 1}
		all, islandSet := islandRegionCells(params.Dims, seed, params, lo, hi)
		regionSet := map[hexgrid.Coord]bool{}
		for _, c := range all {
			regionSet[c] = true
		}
		var start hexgrid.Coord
		found := false
		for _, c := range all {
			if !islandSet[c] {
				start = c
				found = true
				break
			}
		}
		if !found {
			continue
		}
		visited := map[hexgrid.Coord]bool{start: true}
		stack := []hexgrid.Coord{start}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			curCell := gen.Cell(cur)
			for _, d := range sixDirections {
				if curCell.Faces[d] != FacePassage {
					continue
				}
				n := cur.Neighbor(d)
				if !regionSet[n] || visited[n] {
					continue
				}
				visited[n] = true
				stack = append(stack, n)
			}
		}
		for _, c := range all {
			if !islandSet[c] && !visited[c] {
				t.Errorf("seed %d: non-island cell %v not reachable from %v", seed, c, start)
			}
		}
	}
}

// TestCell_FaceAgreementFarFromOriginAcrossWorldSeeds is the rapid
// property case the exhaustive face-agreement sweep does not cover: it
// varies the world seed and reaches coordinates far from the origin,
// where that sweep exercises one seed over a region straddling zero.
// The property checked is the same one the sweep checks — a shared
// border between two cells reads the same wall-or-passage state from
// both sides.
func TestCell_FaceAgreementFarFromOriginAcrossWorldSeeds(t *testing.T) {
	t.Parallel()
	params := refParams()
	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Int64().Draw(rt, "seed")
		c := drawFarCoord(rt)
		gen, err := New(seed, params)
		if err != nil {
			rt.Fatalf("New: %v", err)
		}
		cell := gen.Cell(c)
		for _, d := range sixDirections {
			n := c.Neighbor(d)
			nCell := gen.Cell(n)
			got := cell.Faces[d]
			want := nCell.Faces[d.Opposite()]
			if got != want {
				rt.Fatalf("face agreement fails at %v dir %v: %v vs %v (from %v dir %v)", c, d, got, want, n, d.Opposite())
			}
		}
	})
}
