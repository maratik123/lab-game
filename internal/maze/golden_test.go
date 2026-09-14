package maze

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var updateChunksGolden = flag.Bool("update-chunks", false, "update the chunks golden file")

const chunksGoldenPath = "testdata/chunks.golden"

// goldenRadius is the pinned chunk radius the chunks golden is minted
// under: the design's own reference value.
const goldenRadius = 9

// goldenParams is the pinned Params header: radius goldenRadius, island
// share 0.05, extra-passage share 0.15, growing-tree bias 0.5, portal
// shares 0.1/0.2, every one of the five algorithms weighted equally.
func goldenParams() Params {
	return Params{
		Radius:            goldenRadius,
		Weights:           AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   half,
		PortalShareLower:  decimal.New(1, -1),
		PortalShareUpper:  decimal.New(2, -1),
	}
}

// goldenSeed is the pinned world seed the chunks golden is minted under.
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

func renderCellLine(coord hexgrid.Coord, faces [6]FaceState, seed uint64) string {
	faceNames := make([]string, len(faces))
	for i, f := range faces {
		faceNames[i] = faceStateString(f)
	}
	return fmt.Sprintf("cell(%d,%d)=faces:%s seed:%016x", coord.Q, coord.R, strings.Join(faceNames, ","), seed)
}

// facePassageFromChunk reports whether face f (a border candidate
// between ch and some neighbour) is a passage, read from ch's own map
// m — whichever of f's two endpoints is ch's own local cell.
func facePassageFromChunk(t *testing.T, m Map, lattice hexgrid.Lattice, ch hexgrid.Chunk, f hexgrid.Face) bool {
	t.Helper()
	center := lattice.Center(ch)
	local := hexgrid.Coord{Q: f.Cell.Q - center.Q, R: f.Cell.R - center.R}
	if faces, ok := m.Faces(local); ok {
		return faces[f.Dir] == FacePassage
	}
	other := f.Cell.Neighbor(f.Dir)
	localOther := hexgrid.Coord{Q: other.Q - center.Q, R: other.R - center.R}
	faces, ok := m.Faces(localOther)
	if !ok {
		t.Fatalf("facePassageFromChunk: neither endpoint of %v belongs to chunk %v", f, ch)
	}
	return faces[f.Dir.Opposite()] == FacePassage
}

// directionName renders d as the short compass label its canonical
// order defines, for golden-file lines.
func directionName(d hexgrid.Direction) string {
	names := [6]string{"E", "NE", "NW", "W", "SW", "SE"}
	return names[d]
}

// borderPositionsLine renders ch's border with its neighbour in
// direction d, as the sorted 0-based positions (in borderCandidates'
// own path order) of every candidate face that is a passage in m.
func borderPositionsLine(t *testing.T, m Map, lattice hexgrid.Lattice, ch hexgrid.Chunk, d hexgrid.Direction) string {
	t.Helper()
	neighborChunk := ch.Neighbor(d)
	candidates := borderCandidates(lattice, ch, neighborChunk)
	var positions []string
	for i, f := range candidates {
		if facePassageFromChunk(t, m, lattice, ch, f) {
			positions = append(positions, strconv.Itoa(i))
		}
	}
	return fmt.Sprintf("chunk(%d,%d).border(%s)=%s", ch.Q, ch.R, directionName(d), strings.Join(positions, ","))
}

// dumpChunkBlock renders one chunk's full golden block: its type and
// drawn algorithm, its six borders' portal positions in canonical
// direction order, and every local cell's global coordinate and six
// face states in LocalCells order. neighbors are passed to Generate
// directly, so a chunk generated against a stored neighbour renders
// exactly what that route produces.
func dumpChunkBlock(t *testing.T, gen *Generator, lattice hexgrid.Lattice, ch hexgrid.Chunk, typ ChunkType, weights AlgorithmWeights, neighbors ...Map) []string {
	t.Helper()
	m, err := gen.Generate(ch, typ, neighbors...)
	if err != nil {
		t.Fatalf("Generate(%v,%v): %v", ch, typ, err)
	}
	algo := drawAlgorithm(newStream(chunkKey(goldenSeed, purposeAlgorithm, ch)), weights)

	lines := []string{fmt.Sprintf("chunk(%d,%d).type=%s algorithm=%s", ch.Q, ch.R, typ, algo)}
	for _, d := range sixDirections {
		lines = append(lines, borderPositionsLine(t, m, lattice, ch, d))
	}
	for _, local := range lattice.LocalCells() {
		c := lattice.At(ch, local)
		faces, ok := m.Faces(local)
		if !ok {
			t.Fatalf("Faces(%v): ok=false", local)
		}
		lines = append(lines, renderCellLine(c, faces, gen.CellSeed(c)))
	}
	return lines
}

// chunksGoldenLines builds every line the chunks golden pins: chunk
// (0,0) as a gate chunk with all borders derived; chunk (1,0) as
// fabric, generated against (0,0)'s stored map; and one fabric chunk
// per algorithm under that algorithm's single weight.
func chunksGoldenLines(t *testing.T) []string {
	t.Helper()
	params := goldenParams()
	lattice := params.lattice()
	gen, err := New(goldenSeed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	lines := []string{fmt.Sprintf(
		"# domain-tag=lab-game/maze/v1 version=%d seed=%d radius=%d island_share=0.05 extra_passage_share=0.15 growing_tree_bias=0.5 portal_share_lower=0.1 portal_share_upper=0.2 weights=equal",
		Version, goldenSeed, goldenRadius,
	)}

	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}

	originMap, err := gen.Generate(origin, ChunkTypeGate)
	if err != nil {
		t.Fatalf("Generate(origin,gate): %v", err)
	}
	lines = append(lines, dumpChunkBlock(t, gen, lattice, origin, ChunkTypeGate, params.Weights)...)
	lines = append(lines, dumpChunkBlock(t, gen, lattice, east, ChunkTypeFabric, params.Weights, storedCopyOf(t, originMap))...)

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
			t.Fatalf("New: %v", err)
		}
		lines = append(lines, dumpChunkBlock(t, g, p.lattice(), a.chunk, ChunkTypeFabric, w)...)
	}

	return lines
}

func TestChunksGolden(t *testing.T) {
	t.Parallel()
	lines := chunksGoldenLines(t)
	got := strings.Join(lines, "\n") + "\n"

	if *updateChunksGolden {
		if err := os.WriteFile(chunksGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", chunksGoldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(chunksGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v (run with -update-chunks to mint it)", chunksGoldenPath, err)
	}
	if got != string(want) {
		t.Errorf("chunks golden mismatch — the key chain, a stream, a reduction, the canonical local order, the border enumeration order, the island rule, the gate exclusion, an algorithm's traversal, the cycle pass, the portal count or placement, or stored-border precedence changed: bump Version if this is a deliberate generation change.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestChunksGolden_EveryIslandCellHasAllSixFacesAsWallInTheMintedTable
// reads the minted golden back and cross-checks its own "wall on every
// face" cells against a live re-derivation of the origin chunk's island
// set — reviewing the mint against the island rule directly, not only
// the diff's shape.
func TestChunksGolden_EveryIslandCellHasAllSixFacesAsWallInTheMintedTable(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(chunksGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", chunksGoldenPath, err)
	}
	params := goldenParams()
	lattice := params.lattice()
	g := newChunkGraph(lattice)
	origin := hexgrid.Chunk{Q: 0, R: 0}
	islands := selectIslands(g, newStream(chunkKey(goldenSeed, purposeIsland, origin)), params, ChunkTypeGate)
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

// TestChunksGolden_EveryPortalLineWithinBoundsAndNonConsecutive reads
// the minted golden's border(...) lines and re-derives, live, that
// every position lies in [lo, hi] and holds no consecutive pair — the
// same cross-check the design pins, re-derived from the generator, not
// from the file.
func TestChunksGolden_EveryPortalLineWithinBoundsAndNonConsecutive(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(chunksGoldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", chunksGoldenPath, err)
	}
	params := goldenParams()
	borderLength := 2*int(params.Radius) + 1
	lo, hi := portalBounds(params.PortalShareLower, params.PortalShareUpper, borderLength)

	checked := 0
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, ".border(") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		checked++
		var positions []int
		for _, tok := range strings.Split(parts[1], ",") {
			v, err := strconv.Atoi(tok)
			if err != nil {
				t.Fatalf("line %q: bad position %q: %v", line, tok, err)
			}
			positions = append(positions, v)
		}
		if len(positions) < lo || len(positions) > hi {
			t.Errorf("line %q: %d positions, want within [%d,%d]", line, len(positions), lo, hi)
		}
		if !nonConsecutiveInPositionList(positions) {
			t.Errorf("line %q: positions %v are not pairwise non-consecutive", line, positions)
		}
	}
	if checked == 0 {
		t.Fatal("test setup: no non-empty border(...) line found in the golden — nothing checked")
	}
}

// TestChunksGolden_EastChunkBorderWithOriginMatchesOrigin re-derives
// live that chunk (1,0)'s border with (0,0), generated against (0,0)'s
// stored map, is identical to (0,0)'s own side of that same border.
func TestChunksGolden_EastChunkBorderWithOriginMatchesOrigin(t *testing.T) {
	t.Parallel()
	params := goldenParams()
	lattice := params.lattice()
	gen, err := New(goldenSeed, params)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	origin := hexgrid.Chunk{Q: 0, R: 0}
	east := hexgrid.Chunk{Q: 1, R: 0}

	originMap, err := gen.Generate(origin, ChunkTypeGate)
	if err != nil {
		t.Fatalf("Generate(origin): %v", err)
	}
	eastMap, err := gen.Generate(east, ChunkTypeFabric, storedCopyOf(t, originMap))
	if err != nil {
		t.Fatalf("Generate(east, with origin): %v", err)
	}

	candidates := borderCandidates(lattice, origin, east)
	for _, f := range candidates {
		originSide := facePassageFromChunk(t, originMap, lattice, origin, f)
		eastSide := facePassageFromChunk(t, eastMap, lattice, east, f)
		if originSide != eastSide {
			t.Errorf("face %v: origin says passage=%v, east (generated against origin) says passage=%v", f, originSide, eastSide)
		}
	}
}
