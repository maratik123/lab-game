package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// wholeChunkClaimer claims every coordinate of one chunk, honouring the
// hook's own granularity contract, and records every coordinate it was
// asked about.
type wholeChunkClaimer struct {
	dims  hexgrid.Dims
	chunk hexgrid.Chunk
	asked map[hexgrid.Coord]bool
}

func newWholeChunkClaimer(dims hexgrid.Dims, chunk hexgrid.Chunk) *wholeChunkClaimer {
	return &wholeChunkClaimer{dims: dims, chunk: chunk, asked: map[hexgrid.Coord]bool{}}
}

func (c *wholeChunkClaimer) Claims(coord hexgrid.Coord) bool {
	c.asked[coord] = true
	return c.dims.ChunkOf(coord) == c.chunk
}

func TestCell_NoHookNoCoordinateIsMarkedAsPrefab(t *testing.T) {
	t.Parallel()
	gen, err := New(1, refParams(), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for q := int32(0); q < 16; q++ {
		for r := int32(0); r < 16; r++ {
			c := gen.Cell(hexgrid.Coord{Q: q, R: r})
			if c.Prefab {
				t.Fatalf("Cell(%d,%d).Prefab = true with no hook registered", q, r)
			}
			for _, f := range c.Faces {
				if f == FaceDeferred {
					t.Fatalf("Cell(%d,%d) has a deferred face with no hook registered", q, r)
				}
			}
		}
	}
}

func TestCell_ClaimedInteriorCoordinateDefersAllSixFacesAndHasNoSeed(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	claimer := newWholeChunkClaimer(dims, hexgrid.Chunk{Q: 0, R: 0})
	p := refParams()
	gen, err := New(1, p, claimer)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	interior := hexgrid.Coord{Q: 8, R: 8} // interior of chunk (0,0) at 16x16
	c := gen.Cell(interior)
	if !c.Prefab {
		t.Fatal("claimed interior coordinate: Prefab = false, want true")
	}
	if c.Seed != 0 {
		t.Errorf("claimed coordinate carries seed %d, want 0", c.Seed)
	}
	for i, f := range c.Faces {
		if f != FaceDeferred {
			t.Errorf("claimed interior coordinate face %d = %v, want FaceDeferred", i, f)
		}
	}
}

func TestCell_ClaimedBorderCoordinateCarriesPortalRuleBorderFaces(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	claimer := newWholeChunkClaimer(dims, hexgrid.Chunk{Q: 0, R: 0})
	p := refParams()

	claimedGen, err := New(1, p, claimer)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	unclaimedGen, err := New(1, p, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	border := hexgrid.Coord{Q: 15, R: 8} // rightmost column of chunk (0,0): a border cell
	claimedCell := claimedGen.Cell(border)
	if !claimedCell.Prefab {
		t.Fatal("claimed border coordinate: Prefab = false, want true")
	}

	// Every direction whose neighbour lies outside the chunk must match
	// what the unclaimed generator's own border-facing logic yields —
	// the portal rule, unchanged by the claim.
	own := dims.ChunkOf(border)
	for _, d := range sixDirections {
		n := border.Neighbor(d)
		if dims.ChunkOf(n) == own {
			continue // interior face: deferred, checked separately
		}
		unclaimedNeighbourCell := unclaimedGen.Cell(n)
		want := unclaimedNeighbourCell.Faces[d.Opposite()]
		got := claimedCell.Faces[d]
		if got != want {
			t.Errorf("border face at direction %v: claimed cell reports %v, unclaimed neighbour reports %v", d, got, want)
		}
	}
}

func TestCell_ClaimSuppressesFabricComparedToNoHook(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	claimer := newWholeChunkClaimer(dims, hexgrid.Chunk{Q: 0, R: 0})
	p := refParams()
	claimedGen, err := New(1, p, claimer)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	unclaimedGen, err := New(1, p, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	interior := hexgrid.Coord{Q: 8, R: 8}
	claimedCell := claimedGen.Cell(interior)
	unclaimedCell := unclaimedGen.Cell(interior)
	if unclaimedCell.Prefab {
		t.Fatal("unclaimed generator marked a coordinate as prefab")
	}
	allDeferred := true
	for _, f := range unclaimedCell.Faces {
		if f != FaceDeferred {
			allDeferred = false
		}
	}
	if allDeferred {
		t.Fatal("unclaimed generator's interior cell has every face deferred — test fixture cannot distinguish claim suppression")
	}
	if !claimedCell.Prefab {
		t.Error("claimed generator did not mark the same coordinate as prefab")
	}
}

func TestCell_ClaimerIsAskedAboutEveryCoordinate(t *testing.T) {
	t.Parallel()
	dims := hexgrid.Dims{Cols: 16, Rows: 16}
	claimer := newWholeChunkClaimer(dims, hexgrid.Chunk{Q: 5, R: 5})
	p := refParams()
	gen, err := New(1, p, claimer)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	coords := []hexgrid.Coord{{Q: 0, R: 0}, {Q: 8, R: 8}, {Q: -3, R: 20}}
	for _, c := range coords {
		gen.Cell(c)
		if !claimer.asked[c] {
			t.Errorf("claimer was never asked about %v", c)
		}
	}
}
