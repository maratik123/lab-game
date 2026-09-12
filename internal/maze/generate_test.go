package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func refParams() Params {
	return Params{
		Dims:              hexgrid.Dims{Cols: 16, Rows: 16},
		Weights:           AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   half,
	}
}

func TestNew_RejectsInvalidParams(t *testing.T) {
	t.Parallel()
	p := refParams()
	p.Dims = hexgrid.Dims{Cols: 0, Rows: 16}
	if _, err := New(1, p, nil); err == nil {
		t.Fatal("New with invalid dims = nil error, want an error")
	}
}

func TestNew_ValidParamsSucceeds(t *testing.T) {
	t.Parallel()
	if _, err := New(1, refParams(), nil); err != nil {
		t.Fatalf("New = %v, want nil", err)
	}
}

func TestCell_RepeatEvaluationIsIdentical(t *testing.T) {
	t.Parallel()
	gen, err := New(20260912, refParams(), nil)
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

func TestCell_FaceAgreementOverAMultiChunkRegion(t *testing.T) {
	t.Parallel()
	gen, err := New(20260912, refParams(), nil)
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

func TestCell_ConnectivityOverAMultiChunkRegionNilHook(t *testing.T) {
	t.Parallel()
	p := refParams()
	gen, err := New(777, p, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	type key struct{ q, r int32 }
	region := map[key]bool{}
	// Whole chunks only — (0,0), (1,0), (0,1), (1,1) at 16x16 — since a
	// partial chunk slice at the region's own edge is not itself
	// internally connected: its cells' only guaranteed connectivity is
	// through the rest of their own chunk, which a sliver excludes.
	const lo, hi = 0, 31
	for q := int32(lo); q <= hi; q++ {
		for r := int32(lo); r <= hi; r++ {
			region[key{q, r}] = true
		}
	}

	// Determine islands within the region by asking every cell whether
	// it has any passage at all — an island cell's every face is a
	// wall (asserted structurally by the golden/property suite in
	// subtask 9); here we simply flood-fill through open passages and
	// check every cell that has at least one passage lands in one
	// component.
	visited := map[key]bool{}
	var start key
	found := false
	cellOf := map[key]Cell{}
	for k := range region {
		c := gen.Cell(hexgrid.Coord{Q: k.q, R: k.r})
		cellOf[k] = c
		hasPassage := false
		for _, f := range c.Faces {
			if f == FacePassage {
				hasPassage = true
			}
		}
		if hasPassage && !found {
			start = k
			found = true
		}
	}
	if !found {
		t.Fatal("no cell in the region has any passage at all")
	}

	stack := []key{start}
	visited[start] = true
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		curCell := cellOf[cur]
		for _, d := range sixDirections {
			if curCell.Faces[d] != FacePassage {
				continue
			}
			nc := hexgrid.Coord{Q: cur.q, R: cur.r}.Neighbor(d)
			nk := key{nc.Q, nc.R}
			if !region[nk] || visited[nk] {
				continue
			}
			visited[nk] = true
			stack = append(stack, nk)
		}
	}

	for k, c := range cellOf {
		hasPassage := false
		for _, f := range c.Faces {
			if f == FacePassage {
				hasPassage = true
			}
		}
		if hasPassage && !visited[k] {
			t.Errorf("cell %v has a passage but is not reachable from the flood-fill start %v", k, start)
		}
	}
}

func TestChunksConsulted_SizeInvariantWithDistanceFromOrigin(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	// Same local offset within the chunk (a border cell), one beside
	// the origin and one far from it: the set size must match — it is
	// the coordinate's position within its own chunk that decides the
	// count, never its distance from the origin.
	near := chunksConsulted(dims, hexgrid.Coord{Q: 0, R: 5})
	far := chunksConsulted(dims, hexgrid.Coord{Q: 1_000_000 * 16, R: 1_000_000*16 + 5})
	if len(near) != len(far) {
		t.Errorf("chunksConsulted set size = %d near origin, %d far from it, want equal", len(near), len(far))
	}
}

func TestChunksConsulted_EveryMemberIsOwnOrANeighboursChunk(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	c := hexgrid.Coord{Q: 5, R: -3}
	own := dims.ChunkOf(c)
	valid := map[hexgrid.Chunk]bool{own: true}
	for _, d := range sixDirections {
		valid[dims.ChunkOf(c.Neighbor(d))] = true
	}
	for _, ch := range chunksConsulted(dims, c) {
		if !valid[ch] {
			t.Errorf("chunksConsulted returned %v, not the cell's own chunk or a neighbour's", ch)
		}
	}
}

func TestCell_RaceSafeAcrossGoroutines(t *testing.T) {
	gen, err := New(20260912, refParams(), nil)
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
