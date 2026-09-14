package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func refParams() Params {
	return Params{
		Radius:            MinRadius,
		Weights:           AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   half,
		PortalShareLower:  decimal.New(1, -1),
		PortalShareUpper:  decimal.New(2, -1),
	}
}

// mapCache memoizes Generate calls per chunk for tests that read many
// coordinates against one Generator and one ChunkType: Generate's own
// determinism and purity are asserted elsewhere (TestGenerate_*), so
// memoizing here changes nothing about what a caller is testing.
type mapCache struct {
	t       *testing.T
	gen     *Generator
	lattice hexgrid.Lattice
	typ     ChunkType
	maps    map[hexgrid.Chunk]Map
}

func newMapCache(t *testing.T, gen *Generator, lattice hexgrid.Lattice, typ ChunkType) *mapCache {
	t.Helper()
	return &mapCache{t: t, gen: gen, lattice: lattice, typ: typ, maps: map[hexgrid.Chunk]Map{}}
}

func (c *mapCache) mapOf(ch hexgrid.Chunk) Map {
	c.t.Helper()
	if m, ok := c.maps[ch]; ok {
		return m
	}
	m, err := c.gen.Generate(ch, c.typ)
	if err != nil {
		c.t.Fatalf("Generate(%v,%v): %v", ch, c.typ, err)
	}
	c.maps[ch] = m
	return m
}

// faces returns coord's six faces, resolving coord to its own chunk and
// local coordinate first.
func (c *mapCache) faces(coord hexgrid.Coord) [6]FaceState {
	c.t.Helper()
	ch, local := c.lattice.Locate(coord)
	m := c.mapOf(ch)
	faces, ok := m.Faces(local)
	if !ok {
		c.t.Fatalf("Faces(%v) in chunk %v: ok=false", local, ch)
	}
	return faces
}

// TestNew_RefusesRadiusBelowSix_AcceptsSixAndGenerates checks that New
// refuses a radius below MinRadius, returning no generator, and accepts
// MinRadius and radii above it, each of which then Generates a map.
func TestNew_RefusesRadiusBelowSix_AcceptsSixAndGenerates(t *testing.T) {
	t.Parallel()
	p := refParams()
	p.Radius = MinRadius - 1
	gen, err := New(1, p)
	if err == nil {
		t.Fatal("New with radius below MinRadius = nil error, want an error")
	}
	if gen != nil {
		t.Fatalf("New with radius below MinRadius = %v, want nil generator", gen)
	}

	for _, r := range []int32{MinRadius, MinRadius + 3, 40} {
		p := refParams()
		p.Radius = r
		gen, err := New(1, p)
		if err != nil {
			t.Fatalf("New with radius %d: %v, want nil error", r, err)
		}
		if _, err := gen.Generate(hexgrid.Chunk{}, ChunkTypeFabric); err != nil {
			t.Fatalf("Generate with radius %d: %v, want nil error", r, err)
		}
	}
}

func TestNew_ValidParamsSucceeds(t *testing.T) {
	t.Parallel()
	if _, err := New(1, refParams()); err != nil {
		t.Fatalf("New = %v, want nil", err)
	}
}

func TestGenerate_RepeatEvaluationIsIdentical(t *testing.T) {
	t.Parallel()
	gen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch := hexgrid.Chunk{Q: 3, R: -5}
	a, err := gen.Generate(ch, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := gen.Generate(ch, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, local := range refLattice().LocalCells() {
		fa, _ := a.Faces(local)
		fb, _ := b.Faces(local)
		if fa != fb {
			t.Fatalf("Generate(%v) not repeatable at local %v: %+v != %+v", ch, local, fa, fb)
		}
	}
}

func TestGenerate_RefusesUnknownChunkType(t *testing.T) {
	t.Parallel()
	gen, err := New(1, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, typ := range []ChunkType{chunkTypeUnset, ChunkType(99), ChunkType(-1)} {
		if _, err := gen.Generate(hexgrid.Chunk{}, typ); err == nil {
			t.Errorf("Generate with chunk type %v = nil error, want an error", typ)
		}
	}
}

// allWalled reports whether every one of faces is a wall — the
// Map.Faces-level definition of an island cell.
func allWalled(faces [6]FaceState) bool {
	for _, f := range faces {
		if f != FaceWall {
			return false
		}
	}
	return true
}

// shareForIslandTarget returns an IslandShare whose islandTarget, over a
// chunk of cellCount cells, is exactly target: target/cellCount rounded
// to far more decimal places than roundHalfUp's ½ threshold can be
// sensitive to.
func shareForIslandTarget(target, cellCount int64) decimal.Decimal {
	return decimal.NewFromInt(target).DivRound(decimal.NewFromInt(cellCount), 30)
}

// TestGenerate_IslandsOffEveryBorder_GateCentreNeverAnIsland checks,
// driven through Generate and Map.Faces and not through selectIslands:
// island cells (every face a wall) sit at local distance <= R-1 from the
// chunk's own centre. Over a seed sweep at an island share whose target
// equals the gate chunk's capacity, the centre is never an island in a
// gate chunk, while at least one fabric chunk in the same sweep has its
// centre as an island — so an exclusion applied to every chunk type
// would go red on that second half.
func TestGenerate_IslandsOffEveryBorder_GateCentreNeverAnIsland(t *testing.T) {
	t.Parallel()
	p := refParams()
	p.Radius = MinRadius
	lattice := p.lattice()
	capacity := gateCapacity(p.Radius)
	p.IslandShare = shareForIslandTarget(capacity, lattice.CellCount())
	if got := islandTarget(p); got != capacity {
		t.Fatalf("test setup: islandTarget = %d, want the gate capacity %d", got, capacity)
	}

	centreLocal := hexgrid.Coord{}
	fabricCentreIsland := false
	for seed := int64(0); seed < 40; seed++ {
		gen, err := New(seed, p)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		for _, typ := range []ChunkType{ChunkTypeFabric, ChunkTypeGate} {
			m, err := gen.Generate(hexgrid.Chunk{Q: int32(seed), R: 0}, typ)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			for _, local := range lattice.LocalCells() {
				faces, ok := m.Faces(local)
				if !ok {
					t.Fatalf("Faces(%v): ok=false", local)
				}
				if !allWalled(faces) {
					continue
				}
				if d := hexgrid.Distance(centreLocal, local); d > int64(p.Radius)-1 {
					t.Errorf("seed %d type %v: island at local %v has distance %d, want <= %d", seed, typ, local, d, p.Radius-1)
				}
				if local == centreLocal {
					if typ == ChunkTypeGate {
						t.Errorf("seed %d: gate chunk's centre is an island", seed)
					} else {
						fabricCentreIsland = true
					}
				}
			}
		}
	}
	if !fabricCentreIsland {
		t.Fatal("test setup: no fabric chunk in the sweep had its centre as an island — this run cannot discriminate the gate-only exclusion")
	}
}

// TestGenerate_TakesSharedBorderFromStoredNeighbour checks that a
// border shared with a stored neighbour reads that neighbour's own
// state; every stored neighbour here is built through NewMap, the route
// a stored-map consumer takes.
func TestGenerate_TakesSharedBorderFromStoredNeighbour(t *testing.T) {
	t.Parallel()
	p := refParams()
	lattice := p.lattice()
	c := hexgrid.Chunk{Q: 0, R: 0}
	n := hexgrid.Chunk{Q: 1, R: 0}

	gen, err := New(20260912, p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// N is generated by a second Generator with a different seed and
	// different portal shares, then rebuilt through NewMap.
	otherP := p
	otherP.PortalShareLower = decimal.New(15, -2)
	otherP.PortalShareUpper = decimal.New(3, -1)
	otherGen, err := New(999, otherP)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	nGenerated, err := otherGen.Generate(n, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	nStored := storedCopyOf(t, nGenerated)

	derivedC, err := gen.Generate(c, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate (no neighbours): %v", err)
	}
	cWithN, err := gen.Generate(c, ChunkTypeFabric, nStored)
	if err != nil {
		t.Fatalf("Generate (with N): %v", err)
	}

	// Setup check: N's border with C must differ from C's derived border
	// in at least one face, or this test cannot discriminate.
	differs := false
	for _, local := range lattice.LocalCells() {
		global := lattice.At(c, local)
		for _, d := range sixDirections {
			neighborChunk, _ := lattice.Locate(global.Neighbor(d))
			if neighborChunk != n {
				continue
			}
			derivedFaces, _ := derivedC.Faces(local)
			withNFaces, _ := cWithN.Faces(local)
			if derivedFaces[d] != withNFaces[d] {
				differs = true
			}
		}
	}
	if !differs {
		t.Fatal("test setup: N's border with C never differs from C's derived border — this run cannot discriminate")
	}

	// C generated against N reads, face by face, exactly N's states on
	// the C-N border, including possibly a count outside C's own range.
	for _, local := range lattice.LocalCells() {
		global := lattice.At(c, local)
		for _, d := range sixDirections {
			neighborGlobal := global.Neighbor(d)
			neighborChunk, neighborLocal := lattice.Locate(neighborGlobal)
			if neighborChunk != n {
				continue
			}
			withNFaces, _ := cWithN.Faces(local)
			nFaces, ok := nStored.Faces(neighborLocal)
			if !ok {
				t.Fatalf("Faces(%v) in N: ok=false", neighborLocal)
			}
			want := nFaces[d.Opposite()]
			if got := withNFaces[d]; got != want {
				t.Errorf("C-with-N face at local %v dir %v = %v, want %v (N's own state)", local, d, got, want)
			}
		}
	}

	assertOtherBordersUnchanged(t, lattice, c, n, derivedC, cWithN)

	// N rebuilt through NewMap with one of its border faces on the C-N
	// border flipped: C's border carries the flip.
	t.Run("flipped_border_face_propagates", func(t *testing.T) {
		t.Parallel()
		facesCopy := facesArrayOf(t, lattice, nGenerated)
		flipLocal, flipDir, found := findSharedBorderFace(lattice, n, c)
		if !found {
			t.Fatal("test setup: no C-N border face found from N's side")
		}
		flipIdx := localIndex(t, lattice, flipLocal)
		flipped := flipFaceState(facesCopy[flipIdx][flipDir])
		facesCopy[flipIdx][flipDir] = flipped

		nFlipped, err := NewMap(n, nGenerated.Type(), nGenerated.Version(), facesCopy)
		if err != nil {
			t.Fatalf("NewMap: %v", err)
		}
		cWithFlippedN, err := gen.Generate(c, ChunkTypeFabric, nFlipped)
		if err != nil {
			t.Fatalf("Generate (with flipped N): %v", err)
		}

		_, cLocal := lattice.Locate(lattice.At(n, flipLocal).Neighbor(flipDir))
		cWithFlippedFaces, ok := cWithFlippedN.Faces(cLocal)
		if !ok {
			t.Fatalf("Faces(%v) in C: ok=false", cLocal)
		}
		if got := cWithFlippedFaces[flipDir.Opposite()]; got != flipped {
			t.Errorf("C's border face at local %v dir %v = %v, want the flipped state %v", cLocal, flipDir.Opposite(), got, flipped)
		}
		assertOtherBordersUnchanged(t, lattice, c, n, derivedC, cWithFlippedN)
	})

	// N rebuilt through NewMap under a version other than Version: C's
	// border still equals N's.
	t.Run("neighbour_version_irrelevant", func(t *testing.T) {
		t.Parallel()
		facesCopy := facesArrayOf(t, lattice, nGenerated)
		otherVersion := Version + 41
		nOtherVersion, err := NewMap(n, nGenerated.Type(), otherVersion, facesCopy)
		if err != nil {
			t.Fatalf("NewMap: %v", err)
		}
		cWithOtherVersionN, err := gen.Generate(c, ChunkTypeFabric, nOtherVersion)
		if err != nil {
			t.Fatalf("Generate (with other-version N): %v", err)
		}
		for _, local := range lattice.LocalCells() {
			global := lattice.At(c, local)
			for _, d := range sixDirections {
				neighborGlobal := global.Neighbor(d)
				neighborChunk, neighborLocal := lattice.Locate(neighborGlobal)
				if neighborChunk != n {
					continue
				}
				got, _ := cWithOtherVersionN.Faces(local)
				want, ok := nOtherVersion.Faces(neighborLocal)
				if !ok {
					t.Fatalf("Faces(%v) in N: ok=false", neighborLocal)
				}
				if got[d] != want[d.Opposite()] {
					t.Errorf("local %v dir %v = %v, want %v (N's own state, version-independent)", local, d, got[d], want[d.Opposite()])
				}
			}
		}
		assertOtherBordersUnchanged(t, lattice, c, n, derivedC, cWithOtherVersionN)
	})
}

// facesArrayOf extracts m's faces in LocalCells order for lattice — the
// shape NewMap and a stored-map consumer both take.
func facesArrayOf(t *testing.T, lattice hexgrid.Lattice, m Map) [][6]FaceState {
	t.Helper()
	cells := lattice.LocalCells()
	faces := make([][6]FaceState, len(cells))
	for idx, local := range cells {
		f, ok := m.Faces(local)
		if !ok {
			t.Fatalf("Faces(%v): ok=false", local)
		}
		faces[idx] = f
	}
	return faces
}

// localIndex returns local's plain index in lattice's LocalCells order.
func localIndex(t *testing.T, lattice hexgrid.Lattice, local hexgrid.Coord) int {
	t.Helper()
	for idx, l := range lattice.LocalCells() {
		if l == local {
			return idx
		}
	}
	t.Fatalf("local %v not found in LocalCells", local)
	return -1
}

// findSharedBorderFace returns a local coordinate and direction in from's
// own lattice such that the face at (local, dir) leaves from and lands in
// to — the first such face found in LocalCells order.
func findSharedBorderFace(lattice hexgrid.Lattice, from, to hexgrid.Chunk) (local hexgrid.Coord, dir hexgrid.Direction, found bool) {
	for _, l := range lattice.LocalCells() {
		global := lattice.At(from, l)
		for _, d := range sixDirections {
			neighborChunk, _ := lattice.Locate(global.Neighbor(d))
			if neighborChunk == to {
				return l, d, true
			}
		}
	}
	return hexgrid.Coord{}, 0, false
}

// flipFaceState returns the other of the two valid FaceStates.
func flipFaceState(s FaceState) FaceState {
	if s == FaceWall {
		return FacePassage
	}
	return FaceWall
}

// assertOtherBordersUnchanged checks that c's five borders other than the
// one shared with n are identical between derivedC (c generated with no
// neighbours) and withN (c generated with some form of n supplied).
func assertOtherBordersUnchanged(t *testing.T, lattice hexgrid.Lattice, c, n hexgrid.Chunk, derivedC, withN Map) {
	t.Helper()
	for _, local := range lattice.LocalCells() {
		global := lattice.At(c, local)
		for _, d := range sixDirections {
			neighborGlobal := global.Neighbor(d)
			neighborChunk, _ := lattice.Locate(neighborGlobal)
			if neighborChunk == n || neighborChunk == c {
				continue
			}
			derivedFaces, _ := derivedC.Faces(local)
			withNFaces, _ := withN.Faces(local)
			if derivedFaces[d] != withNFaces[d] {
				t.Errorf("a border unrelated to N changed: local %v dir %v: derived %v, with-N %v", local, d, derivedFaces[d], withNFaces[d])
			}
		}
	}
}

func TestGenerate_RefusesInvalidNeighbourMaps(t *testing.T) {
	t.Parallel()
	p := refParams()
	c := hexgrid.Chunk{Q: 0, R: 0}
	n := hexgrid.Chunk{Q: 1, R: 0}
	notNeighbor := hexgrid.Chunk{Q: 5, R: 5}
	gen, err := New(1, p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	nMap, err := gen.Generate(n, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	selfMap, err := gen.Generate(c, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	otherRadiusP := p
	otherRadiusP.Radius = p.Radius + 1
	otherRadiusGen, err := New(1, otherRadiusP)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	otherRadiusMap, err := otherRadiusGen.Generate(n, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	notNeighborMap, err := gen.Generate(notNeighbor, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := gen.Generate(c, ChunkTypeFabric, selfMap); err == nil {
		t.Error("Generate with a neighbour map for ch itself = nil error, want an error")
	}
	if _, err := gen.Generate(c, ChunkTypeFabric, notNeighborMap); err == nil {
		t.Error("Generate with a non-adjacent neighbour map = nil error, want an error")
	}
	if _, err := gen.Generate(c, ChunkTypeFabric, otherRadiusMap); err == nil {
		t.Error("Generate with a neighbour map at a different radius = nil error, want an error")
	}
	if _, err := gen.Generate(c, ChunkTypeFabric, nMap, nMap); err == nil {
		t.Error("Generate with two maps for the same neighbour = nil error, want an error")
	}
}

func TestGenerate_NeighbourArgumentOrderIrrelevant(t *testing.T) {
	t.Parallel()
	p := refParams()
	c := hexgrid.Chunk{Q: 0, R: 0}
	gen, err := New(1, p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var neighborMaps []Map
	for _, d := range sixDirections {
		m, err := gen.Generate(hexgrid.Chunk{}.Neighbor(d), ChunkTypeFabric)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		neighborMaps = append(neighborMaps, storedCopyOf(t, m))
	}
	forward, err := gen.Generate(c, ChunkTypeFabric, neighborMaps...)
	if err != nil {
		t.Fatalf("Generate (forward order): %v", err)
	}
	reversed := make([]Map, len(neighborMaps))
	for i, m := range neighborMaps {
		reversed[len(neighborMaps)-1-i] = m
	}
	backward, err := gen.Generate(c, ChunkTypeFabric, reversed...)
	if err != nil {
		t.Fatalf("Generate (reversed order): %v", err)
	}
	for _, local := range refLattice().LocalCells() {
		fFwd, _ := forward.Faces(local)
		fBack, _ := backward.Faces(local)
		if fFwd != fBack {
			t.Fatalf("local %v: forward-order faces %v != reversed-order faces %v", local, fFwd, fBack)
		}
	}
}

// TestGenerate_NonIslandCellsConnectedInsideEveryChunk checks that a
// flood fill over Map.Faces inside the chunk reaches exactly the
// non-island set, for both types across a seed sweep. In a gate chunk
// it starts from the centre, which also checks the gate centre is one
// of the connected cells. In a fabric chunk it starts from a border
// cell, since a fabric chunk's centre may itself be an island.
func TestGenerate_NonIslandCellsConnectedInsideEveryChunk(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	ch := hexgrid.Chunk{Q: 2, R: -1}
	for seedByte := range 15 {
		seed := int64(seedByte) * 7919
		gen, err := New(seed, refParams())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		for _, typ := range []ChunkType{ChunkTypeFabric, ChunkTypeGate} {
			m, err := gen.Generate(ch, typ)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			island := map[hexgrid.Coord]bool{}
			for _, local := range lattice.LocalCells() {
				faces, _ := m.Faces(local)
				isIsland := true
				for _, f := range faces {
					if f == FacePassage {
						isIsland = false
					}
				}
				if isIsland {
					island[local] = true
				}
			}
			var start hexgrid.Coord
			if typ == ChunkTypeGate {
				start = hexgrid.Coord{}
			} else {
				for _, local := range lattice.LocalCells() {
					if hexgrid.Distance(local, hexgrid.Coord{}) == int64(lattice.Radius) {
						start = local
						break
					}
				}
			}
			if island[start] {
				t.Fatalf("seed %d type %v: start cell %v is itself an island — test setup invalid", seed, typ, start)
			}
			visited := map[hexgrid.Coord]bool{start: true}
			stack := []hexgrid.Coord{start}
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				faces, _ := m.Faces(cur)
				for _, d := range sixDirections {
					if faces[d] != FacePassage {
						continue
					}
					n := cur.Neighbor(d)
					if _, ok := m.Faces(n); !ok || visited[n] {
						continue
					}
					visited[n] = true
					stack = append(stack, n)
				}
			}
			for _, local := range lattice.LocalCells() {
				if island[local] == visited[local] {
					if island[local] {
						t.Errorf("seed %d type %v: island cell %v was reached by the flood-fill", seed, typ, local)
					} else {
						t.Errorf("seed %d type %v: non-island cell %v was not reached by the flood-fill", seed, typ, local)
					}
				}
			}
		}
	}
}

// TestGenerate_MapNamesGenerationVersion checks that every map from
// every generation path names the current generation version.
func TestGenerate_MapNamesGenerationVersion(t *testing.T) {
	t.Parallel()
	gen, err := New(1, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, typ := range []ChunkType{ChunkTypeFabric, ChunkTypeGate} {
		m, err := gen.Generate(hexgrid.Chunk{Q: 4, R: 4}, typ)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if m.Version() != Version {
			t.Errorf("Map.Version() = %d, want %d", m.Version(), Version)
		}
	}
}

// TestGenerate_SettledByItsInputsAlone checks that a second Generator
// built from equal inputs yields an identical map. The import-allowlist
// guard checks the database-free clause separately.
func TestGenerate_SettledByItsInputsAlone(t *testing.T) {
	t.Parallel()
	seed := int64(42)
	params := refParams()
	ch := hexgrid.Chunk{Q: -3, R: 8}

	gen1, err := New(seed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	gen2, err := New(seed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m1, err := gen1.Generate(ch, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	m2, err := gen2.Generate(ch, ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if m1.Version() != m2.Version() || m1.Type() != m2.Type() || m1.Radius() != m2.Radius() {
		t.Fatalf("two Generators from equal inputs disagree on metadata: %+v vs %+v", m1, m2)
	}
	for _, local := range refLattice().LocalCells() {
		f1, _ := m1.Faces(local)
		f2, _ := m2.Faces(local)
		if f1 != f2 {
			t.Fatalf("two Generators from equal inputs disagree at local %v: %v vs %v", local, f1, f2)
		}
	}
}

// TestGenerate_SequentialAgainstStoredNeighboursMatchesIndependentGeneration
// generates a small patch of chunks two ways — independently, with no
// neighbour maps, and sequentially, each chunk generated against the
// already-generated (and NewMap-rebuilt) neighbours it has — and checks
// both yield identical maps: the border rule settles a face the same
// way whether or not a stored neighbour happened to be available.
func TestGenerate_SequentialAgainstStoredNeighboursMatchesIndependentGeneration(t *testing.T) {
	t.Parallel()
	p := refParams()
	gen, err := New(555, p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chunks := []hexgrid.Chunk{{}}
	for _, d := range sixDirections {
		chunks = append(chunks, hexgrid.Chunk{}.Neighbor(d))
	}

	independent := map[hexgrid.Chunk]Map{}
	for _, ch := range chunks {
		m, err := gen.Generate(ch, ChunkTypeFabric)
		if err != nil {
			t.Fatalf("Generate(%v): %v", ch, err)
		}
		independent[ch] = m
	}

	sequential := map[hexgrid.Chunk]Map{}
	for _, ch := range chunks {
		var already []Map
		for _, d := range sixDirections {
			nc := ch.Neighbor(d)
			if m, ok := sequential[nc]; ok {
				already = append(already, storedCopyOf(t, m))
			}
		}
		m, err := gen.Generate(ch, ChunkTypeFabric, already...)
		if err != nil {
			t.Fatalf("Generate(%v, ...): %v", ch, err)
		}
		sequential[ch] = m
	}

	lattice := p.lattice()
	for _, ch := range chunks {
		for _, local := range lattice.LocalCells() {
			f1, _ := independent[ch].Faces(local)
			f2, _ := sequential[ch].Faces(local)
			if f1 != f2 {
				t.Fatalf("chunk %v local %v: independent %v != sequential-against-stored %v", ch, local, f1, f2)
			}
		}
	}
}

// TestGenerate_FaceAgreementOverAMultiChunkRegion asserts, over every
// coordinate of a multi-chunk region straddling the origin, that a
// cell's face and its neighbour's opposite face agree.
func TestGenerate_FaceAgreementOverAMultiChunkRegion(t *testing.T) {
	t.Parallel()
	gen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cache := newMapCache(t, gen, refLattice(), ChunkTypeFabric)
	for q := int32(-20); q <= 20; q++ {
		for r := int32(-20); r <= 20; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			faces := cache.faces(c)
			for _, d := range sixDirections {
				n := c.Neighbor(d)
				nFaces := cache.faces(n)
				got := faces[d]
				want := nFaces[d.Opposite()]
				if got != want {
					t.Fatalf("face agreement fails at %v dir %v: %v vs %v (from %v dir %v)", c, d, got, want, n, d.Opposite())
				}
			}
		}
	}
}

// TestGenerate_ConnectivityOverAMultiChunkRegion floods over a region of
// WHOLE chunks only — the centre chunk and its ring of six neighbours —
// since a partial chunk slice at the region's own edge is not itself
// internally connected: its cells' only guaranteed connectivity is
// through the rest of their own chunk, which a sliver excludes.
func TestGenerate_ConnectivityOverAMultiChunkRegion(t *testing.T) {
	t.Parallel()
	p := refParams()
	lattice := p.lattice()
	gen, err := New(777, p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cache := newMapCache(t, gen, lattice, ChunkTypeFabric)

	chunks := []hexgrid.Chunk{{}}
	for _, d := range sixDirections {
		chunks = append(chunks, hexgrid.Chunk{}.Neighbor(d))
	}
	region := map[hexgrid.Coord]bool{}
	for _, ch := range chunks {
		for _, local := range lattice.LocalCells() {
			region[lattice.At(ch, local)] = true
		}
	}

	visited := map[hexgrid.Coord]bool{}
	var start hexgrid.Coord
	found := false
	facesOf := map[hexgrid.Coord][6]FaceState{}
	for c := range region {
		faces := cache.faces(c)
		facesOf[c] = faces
		hasPassage := false
		for _, f := range faces {
			if f == FacePassage {
				hasPassage = true
			}
		}
		if hasPassage && !found {
			start = c
			found = true
		}
	}
	if !found {
		t.Fatal("no cell in the region has any passage at all")
	}

	stack := []hexgrid.Coord{start}
	visited[start] = true
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		curFaces := facesOf[cur]
		for _, d := range sixDirections {
			if curFaces[d] != FacePassage {
				continue
			}
			n := cur.Neighbor(d)
			if !region[n] || visited[n] {
				continue
			}
			visited[n] = true
			stack = append(stack, n)
		}
	}

	for c, faces := range facesOf {
		hasPassage := false
		for _, f := range faces {
			if f == FacePassage {
				hasPassage = true
			}
		}
		if hasPassage && !visited[c] {
			t.Errorf("cell %v has a passage but is not reachable from the flood-fill start %v", c, start)
		}
	}
}

// TestGenerate_SeedIsIndependentOfChunkRadius asserts the sharp form:
// two generators differing only in chunk radius yield the same cell seed
// for a coordinate, while the faces they yield for it differ.
func TestGenerate_SeedIsIndependentOfChunkRadius(t *testing.T) {
	t.Parallel()
	a := refParams()
	b := refParams()
	b.Radius = MinRadius + 3

	genA, err := New(20260912, a)
	if err != nil {
		t.Fatalf("New under radius %d: %v", a.Radius, err)
	}
	genB, err := New(20260912, b)
	if err != nil {
		t.Fatalf("New under radius %d: %v", b.Radius, err)
	}
	cacheA := newMapCache(t, genA, a.lattice(), ChunkTypeFabric)
	cacheB := newMapCache(t, genB, b.lattice(), ChunkTypeFabric)

	var facesDiffer bool
	for q := int32(-3); q <= 3; q++ {
		for r := int32(-3); r <= 3; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			seedA, seedB := genA.CellSeed(c), genB.CellSeed(c)
			if seedA != seedB {
				t.Fatalf("cell seed for %v is %d under radius %d and %d under radius %d; it must be settled by the world seed and the coordinate alone",
					c, seedA, a.Radius, seedB, b.Radius)
			}
			if cacheA.faces(c) != cacheB.faces(c) {
				facesDiffer = true
			}
		}
	}
	if !facesDiffer {
		t.Fatal("test setup: the two radii yielded identical faces at every sampled coordinate, so the seed assertion above would not discriminate")
	}
}

func TestGenerate_RaceSafeAcrossGoroutines(t *testing.T) {
	gen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	chunks := make([]hexgrid.Chunk, 0, 16)
	for q := int32(0); q < 4; q++ {
		for r := int32(0); r < 4; r++ {
			chunks = append(chunks, hexgrid.Chunk{Q: q, R: r})
		}
	}
	want := make([]Map, len(chunks))
	for i, ch := range chunks {
		m, err := gen.Generate(ch, ChunkTypeFabric)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		want[i] = m
	}

	done := make(chan bool, len(chunks))
	for i, ch := range chunks {
		go func(i int, ch hexgrid.Chunk) {
			m, err := gen.Generate(ch, ChunkTypeFabric)
			if err != nil {
				done <- false
				return
			}
			same := true
			for _, local := range refLattice().LocalCells() {
				fa, _ := m.Faces(local)
				fb, _ := want[i].Faces(local)
				if fa != fb {
					same = false
				}
			}
			done <- same
		}(i, ch)
	}
	for range chunks {
		if !<-done {
			t.Error("a concurrent Generate call disagreed with the single-goroutine result")
		}
	}
}
