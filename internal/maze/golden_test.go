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

// goldenParams is the pinned Params header for the equal-weight region:
// dims 16x16, island share 0.05, extra-passage share 0.15, growing-tree
// bias 0.5, every one of the five algorithms weighted equally.
func goldenParams() Params {
	return Params{
		Dims:              hexgrid.Dims{Cols: 16, Rows: 16},
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
	case FaceDeferred:
		return "deferred"
	default:
		return "unknown"
	}
}

func renderCellLine(coord hexgrid.Coord, c Cell) string {
	faces := make([]string, len(c.Faces))
	for i, f := range c.Faces {
		faces[i] = faceStateString(f)
	}
	return fmt.Sprintf("cell(%d,%d)=faces:%s seed:%016x prefab:%t", coord.Q, coord.R, strings.Join(faces, ","), c.Seed, c.Prefab)
}

// dumpChunk renders every cell of chunk ch under dims, via gen, sorted
// by (Q,R) for a stable line order.
func dumpChunk(gen *Generator, dims hexgrid.Dims, ch hexgrid.Chunk) []string {
	origin := dims.Origin(ch)
	var lines []string
	for lr := int32(0); lr < dims.Rows; lr++ {
		for lq := int32(0); lq < dims.Cols; lq++ {
			c := hexgrid.Coord{Q: origin.Q + lq, R: origin.R + lr}
			lines = append(lines, renderCellLine(c, gen.Cell(c)))
		}
	}
	return lines
}

// dumpBorderRing renders only ch's own border-ring cells under dims,
// via gen.
func dumpBorderRing(gen *Generator, dims hexgrid.Dims, ch hexgrid.Chunk) []string {
	origin := dims.Origin(ch)
	var lines []string
	for lr := int32(0); lr < dims.Rows; lr++ {
		for lq := int32(0); lq < dims.Cols; lq++ {
			if lq != 0 && lq != dims.Cols-1 && lr != 0 && lr != dims.Rows-1 {
				continue
			}
			c := hexgrid.Coord{Q: origin.Q + lq, R: origin.R + lr}
			lines = append(lines, renderCellLine(c, gen.Cell(c)))
		}
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
	lines = append(lines, "# domain-tag=lab-game/maze/v1 seed=20260912 dims=16x16 island_share=0.05 extra_passage_share=0.15 growing_tree_bias=0.5 weights=equal prefab=nil")

	params := goldenParams()
	gen, err := New(goldenSeed, params, nil)
	if err != nil {
		panic(err)
	}

	lines = append(lines, "## equal-weight-region")
	origin := hexgrid.Chunk{Q: 0, R: 0}
	belowLeft := hexgrid.Chunk{Q: -1, R: -1}
	below := hexgrid.Chunk{Q: 0, R: -1}
	lines = append(lines, algorithmLine(origin, params.Weights))
	lines = append(lines, dumpChunk(gen, params.Dims, origin)...)
	lines = append(lines, algorithmLine(belowLeft, params.Weights))
	lines = append(lines, dumpChunk(gen, params.Dims, belowLeft)...)
	lines = append(lines, algorithmLine(below, params.Weights))
	lines = append(lines, dumpBorderRing(gen, params.Dims, below)...)

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
		g, err := New(goldenSeed, p, nil)
		if err != nil {
			panic(err)
		}
		lines = append(lines, "## single-weight-"+a.name)
		lines = append(lines, algorithmLine(a.chunk, w))
		lines = append(lines, dumpChunk(g, p.Dims, a.chunk)...)
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
	g := newChunkGraph(params.Dims)
	origin := hexgrid.Chunk{Q: 0, R: 0}
	islands := selectIslands(g, newStream(chunkKey(goldenSeed, purposeIsland, origin)), params)
	islandCoords := map[hexgrid.Coord]bool{}
	chunkOrigin := params.Dims.Origin(origin)
	for idx := range islands {
		lq, lr := g.localCoord(idx)
		islandCoords[hexgrid.Coord{Q: chunkOrigin.Q + lq, R: chunkOrigin.R + lr}] = true
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
