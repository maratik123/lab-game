package maze

import (
	"fmt"
	"slices"
	"sort"
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
	}
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

func TestCell_RepeatEvaluationIsIdentical(t *testing.T) {
	t.Parallel()
	gen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c := hexgrid.Coord{Q: 3, R: -5}
	a := gen.Cell(c)
	b := gen.Cell(c)
	if a != b {
		t.Fatalf("Cell(%v) not repeatable: %+v != %+v", c, a, b)
	}
}

// TestCell_ShuffledEvaluationOrderMatchesSortedOrder evaluates a
// coordinate table twice — once in sorted order, once in an order
// shuffled by a test-local fixed key — and asserts every coordinate
// yields the identical Cell either way: generation is a pure function
// of the coordinate alone, never of evaluation order.
//
// The two passes each get their OWN freshly-constructed Generator, over
// the same seed and params, rather than sharing one. This is
// load-bearing, not a style choice: a cross-call memo on Generator that
// reused an earlier chunk's build for a later chunk is the failure mode
// this scenario exists to catch, and it would still agree with itself if
// both passes read through the same warm memo. So the two passes must
// not be able to share any state that outlives a single Cell call. The
// repeat-evaluation and the -race multi-goroutine tests above and below
// cover the other two order-independence shapes.
func TestCell_ShuffledEvaluationOrderMatchesSortedOrder(t *testing.T) {
	t.Parallel()

	var coords []hexgrid.Coord // built in sorted (Q,R) order
	for q := int32(-5); q <= 5; q++ {
		for r := int32(-5); r <= 5; r++ {
			coords = append(coords, hexgrid.Coord{Q: q, R: r})
		}
	}

	shuffled := append([]hexgrid.Coord(nil), coords...)
	shuffle(newStream([32]byte{0xab, 0xcd, 0xef}), shuffled)
	if fmt.Sprint(shuffled) == fmt.Sprint(coords) {
		t.Fatal("test setup: the shuffle produced the same order as sorted, so this run would not discriminate")
	}

	sortedGen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sortedResults := make(map[hexgrid.Coord]Cell, len(coords))
	for _, c := range coords {
		sortedResults[c] = sortedGen.Cell(c)
	}

	shuffledGen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, c := range shuffled {
		got := shuffledGen.Cell(c)
		want := sortedResults[c]
		if got != want {
			t.Fatalf("shuffled-order evaluation of %v = %+v, want %+v (the sorted-order value)", c, got, want)
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
	for q := int32(-20); q <= 20; q++ {
		for r := int32(-20); r <= 20; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			cell := gen.Cell(c)
			for _, d := range sixDirections {
				n := c.Neighbor(d)
				nCell := gen.Cell(n)
				got := cell.Faces[d]
				want := nCell.Faces[d.Opposite()]
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

	// Determine islands within the region by asking every cell whether
	// it has any passage at all; then simply flood-fill through open
	// passages and check every cell that has at least one passage lands
	// in one component.
	visited := map[hexgrid.Coord]bool{}
	var start hexgrid.Coord
	found := false
	cellOf := map[hexgrid.Coord]Cell{}
	for c := range region {
		cell := gen.Cell(c)
		cellOf[c] = cell
		hasPassage := false
		for _, f := range cell.Faces {
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
		curCell := cellOf[cur]
		for _, d := range sixDirections {
			if curCell.Faces[d] != FacePassage {
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

	for c, cell := range cellOf {
		hasPassage := false
		for _, f := range cell.Faces {
			if f == FacePassage {
				hasPassage = true
			}
		}
		if hasPassage && !visited[c] {
			t.Errorf("cell %v has a passage but is not reachable from the flood-fill start %v", c, start)
		}
	}
}

// TestCell_SeedIsIndependentOfChunkRadius asserts the sharp form: two
// generators differing only in chunk radius yield the same cell seed for
// a coordinate, while the faces they yield for it differ. The second
// half is not decoration — without it the first would hold just as well
// for two generators that were effectively the same, and a cell seed
// secretly folding the radius in would pass.
func TestCell_SeedIsIndependentOfChunkRadius(t *testing.T) {
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

	var facesDiffer bool
	for q := int32(-3); q <= 3; q++ {
		for r := int32(-3); r <= 3; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			ca, cb := genA.Cell(c), genB.Cell(c)
			if ca.Seed != cb.Seed {
				t.Fatalf("cell seed for %v is %d under radius %d and %d under radius %d; it must be settled by the world seed and the coordinate alone",
					c, ca.Seed, a.Radius, cb.Seed, b.Radius)
			}
			if ca.Faces != cb.Faces {
				facesDiffer = true
			}
		}
	}
	if !facesDiffer {
		t.Fatal("test setup: the two radii yielded identical faces at every sampled coordinate, so the seed assertion above would not discriminate")
	}
}

func TestChunksConsulted_RelativeOffsetsAgreeAtTheSamePositionInAChunk(t *testing.T) {
	t.Parallel()
	lattice := refLattice()

	offsets := func(c hexgrid.Coord) []hexgrid.Chunk {
		own, _ := lattice.Locate(c)
		set := chunksConsulted(lattice, c)
		out := make([]hexgrid.Chunk, 0, len(set))
		for _, ch := range set {
			out = append(out, hexgrid.Chunk{Q: ch.Q - own.Q, R: ch.R - own.R})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Q != out[j].Q {
				return out[i].Q < out[j].Q
			}
			return out[i].R < out[j].R
		})
		return out
	}

	near := offsets(hexgrid.Coord{Q: 0, R: 5})
	far := offsets(hexgrid.Coord{Q: 1_000_000, R: 1_000_005})
	if !slices.Equal(near, far) {
		t.Errorf("relative chunk offsets = %v near the origin and %v far from it, want equal", near, far)
	}
}

func TestChunksConsulted_EveryMemberIsOwnOrANeighboursChunk(t *testing.T) {
	t.Parallel()
	lattice := refLattice()
	c := hexgrid.Coord{Q: 5, R: -3}
	own, _ := lattice.Locate(c)
	valid := map[hexgrid.Chunk]bool{own: true}
	for _, d := range sixDirections {
		n, _ := lattice.Locate(c.Neighbor(d))
		valid[n] = true
	}
	for _, ch := range chunksConsulted(lattice, c) {
		if !valid[ch] {
			t.Errorf("chunksConsulted returned %v, not the cell's own chunk or a neighbour's", ch)
		}
	}
}

func TestCell_RaceSafeAcrossGoroutines(t *testing.T) {
	gen, err := New(20260912, refParams())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	coords := make([]hexgrid.Coord, 0, 64)
	for q := int32(0); q < 8; q++ {
		for r := int32(0); r < 8; r++ {
			coords = append(coords, hexgrid.Coord{Q: q, R: r})
		}
	}
	want := make([]Cell, len(coords))
	for i, c := range coords {
		want[i] = gen.Cell(c)
	}

	done := make(chan bool, len(coords))
	for i, c := range coords {
		go func(i int, c hexgrid.Coord) {
			done <- gen.Cell(c) == want[i]
		}(i, c)
	}
	for range coords {
		if !<-done {
			t.Error("a concurrent Cell call disagreed with the single-goroutine result")
		}
	}
}
