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

// islandRegionCells returns, for a block of chunks spanning loChunk..hiChunk
// inclusive (both axes), every generated as ChunkTypeFabric, the coordinate
// of every cell in the region and — separately — which of them selectIslands
// would mark as an island in its own chunk.
func islandRegionCells(lattice hexgrid.Lattice, seed int64, params Params, loChunk, hiChunk hexgrid.Chunk) (all []hexgrid.Coord, islandSet map[hexgrid.Coord]bool) {
	islandSet = map[hexgrid.Coord]bool{}
	for cq := loChunk.Q; cq <= hiChunk.Q; cq++ {
		for cr := loChunk.R; cr <= hiChunk.R; cr++ {
			ch := hexgrid.Chunk{Q: cq, R: cr}
			g := newChunkGraph(lattice)
			islands := selectIslands(g, newStream(chunkKey(seed, purposeIsland, ch)), params, ChunkTypeFabric)
			for idx, local := range g.cells {
				c := lattice.At(ch, local)
				all = append(all, c)
				if islands[idx] {
					islandSet[c] = true
				}
			}
		}
	}
	return all, islandSet
}

// TestIslandsAreWalled_EveryIslandFaceIsAWallFromBothSides asserts the
// island-walled property at the only entry point that can fail here:
// Generate, not the island selector. Connectivity (asserting the
// non-island set is one component) is blind to a face wrongly carved
// into an island, and face agreement is blind to a passage into an
// island that both sides happen to agree on — this scenario needs its
// own assertion for exactly that reason.
func TestIslandsAreWalled_EveryIslandFaceIsAWallFromBothSides(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	lattice := params.lattice()
	gen, err := New(goldenSeed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cache := newMapCache(t, gen, lattice, ChunkTypeFabric)
	lo := hexgrid.Chunk{Q: -2, R: -2}
	hi := hexgrid.Chunk{Q: 1, R: 1}
	all, islandSet := islandRegionCells(lattice, goldenSeed, params, lo, hi)
	if len(islandSet) == 0 {
		t.Fatal("test setup: expected at least one island over this region")
	}

	for c := range islandSet {
		faces := cache.faces(c)
		for _, f := range faces {
			if f != FaceWall {
				t.Errorf("island cell %v has a non-wall face: %+v", c, faces)
			}
		}
		for _, d := range sixDirections {
			n := c.Neighbor(d)
			nFaces := cache.faces(n)
			if nFaces[d.Opposite()] != FaceWall {
				t.Errorf("island cell %v's neighbour %v (dir %v) reads a non-wall face into it: %v", c, n, d.Opposite(), nFaces[d.Opposite()])
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
		curFaces := cache.faces(cur)
		for _, d := range sixDirections {
			if curFaces[d] != FacePassage {
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
// instrument: the golden radius, island share 0.05, the chunk block
// (-2,-2)..(1,1), an absolute tolerance of ±0.01 on the achieved share.
func TestIslandShare_AchievedShareWithinTolerance(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	lo := hexgrid.Chunk{Q: -2, R: -2}
	hi := hexgrid.Chunk{Q: 1, R: 1}
	all, islandSet := islandRegionCells(params.lattice(), goldenSeed, params, lo, hi)

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

// TestConnectivity_MultiChunkRegion checks a patch of the centre
// chunk and its ring of neighbours, with the centre and one ring chunk
// typed gate and the rest fabric, is connected end to end. A flood fill
// over passages across chunks (via Lattice) reaches exactly the
// region's non-island cells, at R in {6,9} over a seed sweep, under
// both layouts: every chunk generated with no neighbour maps, and every
// chunk generated in a fixed outward order against the maps already
// generated for its earlier-built neighbours.
func TestConnectivity_MultiChunkRegion(t *testing.T) {
	t.Parallel()
	for _, r := range []int32{6, 9} {
		params := goldenParams()
		params.Radius = r
		lattice := params.lattice()
		centre := hexgrid.Chunk{}
		region := []hexgrid.Chunk{centre}
		for _, d := range sixDirections {
			region = append(region, centre.Neighbor(d))
		}
		typeOf := map[hexgrid.Chunk]ChunkType{centre: ChunkTypeGate, region[1]: ChunkTypeGate}
		for _, ch := range region[2:] {
			typeOf[ch] = ChunkTypeFabric
		}

		for _, seed := range []int64{1, 2, 3, 20260912} {
			for _, sequential := range []bool{false, true} {
				gen, err := New(seed, params)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				maps := map[hexgrid.Chunk]Map{}
				for _, ch := range region {
					var neighbors []Map
					if sequential {
						for _, d := range sixDirections {
							if nm, ok := maps[ch.Neighbor(d)]; ok {
								neighbors = append(neighbors, nm)
							}
						}
					}
					m, err := gen.Generate(ch, typeOf[ch], neighbors...)
					if err != nil {
						t.Fatalf("Generate(%v): %v", ch, err)
					}
					maps[ch] = m
				}

				var all []hexgrid.Coord
				regionSet := map[hexgrid.Coord]bool{}
				islandSet := map[hexgrid.Coord]bool{}
				for _, ch := range region {
					g := newChunkGraph(lattice)
					for _, local := range g.cells {
						c := lattice.At(ch, local)
						all = append(all, c)
						regionSet[c] = true
						faces, ok := maps[ch].Faces(local)
						if !ok {
							t.Fatalf("Faces(%v) in chunk %v: ok=false", local, ch)
						}
						if allWalled(faces) {
							islandSet[c] = true
						}
					}
				}

				faceAt := func(c hexgrid.Coord, d hexgrid.Direction) FaceState {
					t.Helper()
					ch, local := lattice.Locate(c)
					m, ok := maps[ch]
					if !ok {
						t.Fatalf("cell %v locates to chunk %v, outside the region", c, ch)
					}
					faces, ok := m.Faces(local)
					if !ok {
						t.Fatalf("Faces(%v) in chunk %v: ok=false", local, ch)
					}
					return faces[d]
				}

				var start hexgrid.Coord
				found := false
				for _, c := range all {
					if !islandSet[c] {
						start, found = c, true
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
					for _, d := range sixDirections {
						if faceAt(cur, d) != FacePassage {
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
						t.Errorf("R=%d seed %d sequential=%v: non-island cell %v not reachable from %v", r, seed, sequential, c, start)
					}
				}
			}
		}
	}
}

// TestGenerate_FaceAgreementFarFromOriginAcrossWorldSeeds is the rapid
// property case the exhaustive face-agreement sweep does not cover: it
// varies the world seed and reaches coordinates far from the origin,
// where that sweep exercises one seed over a region straddling zero.
// The property checked is the same one the sweep checks — a shared
// border between two cells reads the same wall-or-passage state from
// both sides.
func TestGenerate_FaceAgreementFarFromOriginAcrossWorldSeeds(t *testing.T) {
	t.Parallel()
	params := refParams()
	rapid.Check(t, func(rt *rapid.T) {
		seed := rapid.Int64().Draw(rt, "seed")
		c := drawFarCoord(rt)
		gen, err := New(seed, params)
		if err != nil {
			rt.Fatalf("New: %v", err)
		}
		lattice := params.lattice()
		ch, local := lattice.Locate(c)
		m, err := gen.Generate(ch, ChunkTypeFabric)
		if err != nil {
			rt.Fatalf("Generate: %v", err)
		}
		faces, ok := m.Faces(local)
		if !ok {
			rt.Fatalf("Faces(%v) in chunk %v: ok=false", local, ch)
		}
		for _, d := range sixDirections {
			n := c.Neighbor(d)
			nCh, nLocal := lattice.Locate(n)
			nMap, err := gen.Generate(nCh, ChunkTypeFabric)
			if err != nil {
				rt.Fatalf("Generate: %v", err)
			}
			nFaces, ok := nMap.Faces(nLocal)
			if !ok {
				rt.Fatalf("Faces(%v) in chunk %v: ok=false", nLocal, nCh)
			}
			got := faces[d]
			want := nFaces[d.Opposite()]
			if got != want {
				rt.Fatalf("face agreement fails at %v dir %v: %v vs %v (from %v dir %v)", c, d, got, want, n, d.Opposite())
			}
		}
	})
}
