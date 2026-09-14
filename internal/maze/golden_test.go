package maze

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var updateCellsGolden = flag.Bool("update-cells", false, "update the cells golden file")

const cellsGoldenPath = "testdata/cells.golden"

// goldenRadius is the pinned chunk radius the cells golden is minted
// under, kept at the package's own minimum while the golden is still in
// its interim shape.
const goldenRadius = MinRadius

// goldenParams is the pinned Params header for the equal-weight region:
// radius goldenRadius, island share 0.05, extra-passage share 0.15,
// growing-tree bias 0.5, every one of the five algorithms weighted
// equally.
func goldenParams() Params {
	return Params{
		Radius:            goldenRadius,
		Weights:           AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   half,
	}
}

// goldenSeed is the pinned world seed the cells golden is minted under.
const goldenSeed int64 = 20260912

func faceStateString(f FaceState) string {
	switch f {
	case FaceWall:
		return "wall"
	case FacePassage:
		return "passage"
	default:
		return "unknown"
	}
}

func renderCellLine(coord hexgrid.Coord, c Cell) string {
	faces := make([]string, len(c.Faces))
	for i, f := range c.Faces {
		faces[i] = faceStateString(f)
	}
	return fmt.Sprintf("cell(%d,%d)=faces:%s seed:%016x", coord.Q, coord.R, strings.Join(faces, ","), c.Seed)
}

// dumpChunk renders every cell of chunk ch under lattice, via gen,
// sorted by local (Q,R) for a stable line order — the lattice's own
// LocalCells order.
func dumpChunk(gen *Generator, lattice hexgrid.Lattice, ch hexgrid.Chunk) []string {
	var lines []string
	for _, local := range lattice.LocalCells() {
		c := lattice.At(ch, local)
		lines = append(lines, renderCellLine(c, gen.Cell(c)))
	}
	return lines
}

// dumpBorderRing renders only ch's own border-ring cells (those at
// exactly lattice's radius from the centre) under lattice, via gen.
func dumpBorderRing(gen *Generator, lattice hexgrid.Lattice, ch hexgrid.Chunk) []string {
	var lines []string
	for _, local := range lattice.LocalCells() {
		if hexgrid.Distance(local, hexgrid.Coord{}) != int64(lattice.Radius) {
			continue
		}
		c := lattice.At(ch, local)
		lines = append(lines, renderCellLine(c, gen.Cell(c)))
	}
	return lines
}

func algorithmLine(chunk hexgrid.Chunk, weights AlgorithmWeights) string {
	algo := drawAlgorithm(newStream(chunkKey(goldenSeed, purposeAlgorithm, chunk)), weights)
	return fmt.Sprintf("chunk(%d,%d).algorithm=%s", chunk.Q, chunk.R, algo.String())
}

// cellsGoldenLines builds every line the cells golden pins: the
// equal-weight region (the origin chunk and the chunk diagonally
// below-left of it in full, plus the border ring of the chunk below the
// origin), and one single-weight section per algorithm over its own
// named chunk.
func cellsGoldenLines() []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("# domain-tag=lab-game/maze/v1 seed=%d radius=%d island_share=0.05 extra_passage_share=0.15 growing_tree_bias=0.5 weights=equal", goldenSeed, goldenRadius))

	params := goldenParams()
	lattice := params.lattice()
	gen, err := New(goldenSeed, params)
	if err != nil {
		panic(err)
	}

	lines = append(lines, "## equal-weight-region")
	origin := hexgrid.Chunk{Q: 0, R: 0}
	belowLeft := hexgrid.Chunk{Q: -1, R: -1}
	below := hexgrid.Chunk{Q: 0, R: -1}
	lines = append(lines, algorithmLine(origin, params.Weights))
	lines = append(lines, dumpChunk(gen, lattice, origin)...)
	lines = append(lines, algorithmLine(belowLeft, params.Weights))
	lines = append(lines, dumpChunk(gen, lattice, belowLeft)...)
	lines = append(lines, algorithmLine(below, params.Weights))
	lines = append(lines, dumpBorderRing(gen, lattice, below)...)

	algos := []struct {
		name  string
		algo  Algorithm
		chunk hexgrid.Chunk
	}{
		{"backtracker", AlgorithmBacktracker, hexgrid.Chunk{Q: 100, R: 0}},
		{"kruskal", AlgorithmKruskal, hexgrid.Chunk{Q: 101, R: 0}},
		{"prim", AlgorithmPrim, hexgrid.Chunk{Q: 102, R: 0}},
		{"growing_tree", AlgorithmGrowingTree, hexgrid.Chunk{Q: 103, R: 0}},
		{"wilson_walk", AlgorithmWilson, hexgrid.Chunk{Q: 104, R: 0}},
	}
	for _, a := range algos {
		var w AlgorithmWeights
		w[a.algo] = 1
		p := params
		p.Weights = w
		g, err := New(goldenSeed, p)
		if err != nil {
			panic(err)
		}
		lines = append(lines, "## single-weight-"+a.name)
		lines = append(lines, algorithmLine(a.chunk, w))
		lines = append(lines, dumpChunk(g, p.lattice(), a.chunk)...)
	}

	return lines
}

func TestCellsGolden(t *testing.T) {
	t.Parallel()
	lines := cellsGoldenLines()
	got := strings.Join(lines, "\n") + "\n"

	if *updateCellsGolden {
		if err := os.WriteFile(cellsGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", cellsGoldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(cellsGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v (run with -update-cells to mint it)", cellsGoldenPath, err)
	}
	if got != string(want) {
		t.Errorf("cells golden mismatch — the domain tag, the encoding, the digest, the stream, a reduction, the island rule, an algorithm's traversal, the cycle pass, or the portal rule changed: every world already generated under this seed is now different.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestCellsGolden_EveryIslandCellHasAllSixFacesAsWallInTheMintedTable
// reads the minted golden back and cross-checks its own "wall on every
// face" cells against a live re-derivation of the origin chunk's island
// set — reviewing the mint against the island rule directly, not only
// the diff's shape.
func TestCellsGolden_EveryIslandCellHasAllSixFacesAsWallInTheMintedTable(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(cellsGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", cellsGoldenPath, err)
	}
	params := goldenParams()
	lattice := params.lattice()
	g := newChunkGraph(lattice)
	origin := hexgrid.Chunk{Q: 0, R: 0}
	islands := selectIslands(g, newStream(chunkKey(goldenSeed, purposeIsland, origin)), params)
	islandCoords := map[hexgrid.Coord]bool{}
	for idx := range islands {
		islandCoords[lattice.At(origin, g.localCoord(idx))] = true
	}
	if len(islandCoords) == 0 {
		t.Fatal("test setup: the origin chunk has no island at the golden's own share — nothing to check")
	}

	checked := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cell(") {
			continue
		}
		var q, r int32
		if _, err := fmt.Sscanf(line, "cell(%d,%d)=", &q, &r); err != nil {
			continue
		}
		c := hexgrid.Coord{Q: q, R: r}
		if !islandCoords[c] {
			continue
		}
		facesPart := strings.SplitN(strings.SplitN(line, "faces:", 2)[1], " ", 2)[0]
		for _, f := range strings.Split(facesPart, ",") {
			if f != "wall" {
				t.Errorf("island cell %v's minted line has a non-wall face: %s", c, line)
			}
		}
		checked++
	}
	if checked != len(islandCoords) {
		t.Errorf("checked %d island cells against the golden, want %d (some island coordinate was not dumped)", checked, len(islandCoords))
	}
}
