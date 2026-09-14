package maze

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Generator is an immutable, concurrency-safe deterministic chunk
// generator: New validates every input once, and Generate can neither
// fail (given a known ChunkType) nor panic afterwards. It holds no
// memo — the caching question is a later decision this package defers.
type Generator struct {
	seed   int64
	params Params
}

// New builds a Generator over seed and params. It rejects an invalid
// params, naming the offending input.
func New(seed int64, params Params) (*Generator, error) {
	if err := params.validate(); err != nil {
		return nil, fmt.Errorf("maze.New: %w", err)
	}
	return &Generator{seed: seed, params: params}, nil
}

// CellSeed returns the per-cell content seed the world-config and
// node-content layers sample their own content from: a pure function of
// gen's own seed and c alone.
func (gen *Generator) CellSeed(c hexgrid.Coord) uint64 {
	return cellSeed(gen.seed, c)
}

// Generate builds ch's whole map under typ, in a pinned pass order:
// islands (island stream) → algorithm draw → spanning structure → extra
// passages → the six borders in canonical direction order. It refuses
// the zero ChunkType and any value this package does not know, naming
// it.
func (gen *Generator) Generate(ch hexgrid.Chunk, typ ChunkType) (Map, error) {
	if !typ.valid() {
		return Map{}, fmt.Errorf("maze.Generate: unknown chunk type %v", typ)
	}
	lattice := gen.params.lattice()
	g := newChunkGraph(lattice)

	islands := selectIslands(g, newStream(chunkKey(gen.seed, purposeIsland, ch)), gen.params, typ)
	algo := drawAlgorithm(newStream(chunkKey(gen.seed, purposeAlgorithm, ch)), gen.params.Weights)
	open := buildSpanningStructure(g, islands, algo, newStream(chunkKey(gen.seed, purposeStructure, ch)), biasThreshold(gen.params.GrowingTreeBias))
	addExtraPassages(g, islands, open, newStream(chunkKey(gen.seed, purposeCycle, ch)), gen.params.ExtraPassageShare)

	// A per-neighbour-chunk cache of this chunk's own borders: each
	// border's portal set depends only on the seed, the two chunks and
	// the portal shares, so it is computed once per neighbour rather
	// than once per face.
	borders := map[hexgrid.Chunk]map[hexgrid.Face]bool{}
	borderPortals := func(neighborChunk hexgrid.Chunk) map[hexgrid.Face]bool {
		if p, ok := borders[neighborChunk]; ok {
			return p
		}
		candidates := borderCandidates(lattice, ch, neighborChunk)
		p := selectPortals(candidates, newStream(borderKey(gen.seed, ch, neighborChunk)), gen.params.PortalShareLower, gen.params.PortalShareUpper)
		borders[neighborChunk] = p
		return p
	}

	faces := make([][6]FaceState, g.cellCount())
	for idx, local := range g.cells {
		for _, d := range sixDirections {
			n := local.Neighbor(d)
			if j, ok := g.index[n]; ok {
				if open.has(idx, j) {
					faces[idx][d] = FacePassage
				} else {
					faces[idx][d] = FaceWall
				}
				continue
			}
			c := lattice.At(ch, local)
			globalNeighbor := c.Neighbor(d)
			neighborChunk, _ := lattice.Locate(globalNeighbor)
			portals := borderPortals(neighborChunk)
			if portals[hexgrid.FaceOf(c, d)] {
				faces[idx][d] = FacePassage
			} else {
				faces[idx][d] = FaceWall
			}
		}
	}

	return Map{chunk: ch, typ: typ, version: Version, graph: g, faces: faces}, nil
}
