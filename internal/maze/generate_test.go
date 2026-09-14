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

func TestNew_RejectsInvalidParams(t *testing.T) {
	t.Parallel()
	p := refParams()
	p.Radius = 0
	if _, err := New(1, p); err == nil {
		t.Fatal("New with invalid radius = nil error, want an error")
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

// TestCell_FaceAgreementOverAMultiChunkRegion asserts, over every
// coordinate of a multi-chunk region straddling the origin, that a
// cell's face and its neighbour's opposite face agree.
func TestCell_FaceAgreementOverAMultiChunkRegion(t *testing.T) {
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

// TestCell_ConnectivityOverAMultiChunkRegion floods over a region of
// WHOLE chunks only — the centre chunk and its ring of six neighbours —
// since a partial chunk slice at the region's own edge is not itself
// internally connected: its cells' only guaranteed connectivity is
// through the rest of their own chunk, which a sliver excludes.
func TestCell_ConnectivityOverAMultiChunkRegion(t *testing.T) {
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
