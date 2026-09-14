package maze

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Generator is an immutable, concurrency-safe deterministic chunk
// generator: New validates params once. Generate can still refuse an
// unknown ChunkType or an invalid neighbour map, but never panics. It
// holds no memo — the caching question is a later decision this
// package defers.
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
// passages → the six borders in canonical direction order. A border
// with a chunk whose Map appears in neighbors is taken from that map
// directly; every other border comes from the portal rule. It refuses
// the zero ChunkType and any value this package does not know; a
// neighbor map for ch itself; a neighbor map not adjacent to ch; a
// neighbor map at a different radius; and two neighbor maps for the
// same chunk — each naming the offending map. The result does not
// depend on the order of neighbors.
func (gen *Generator) Generate(ch hexgrid.Chunk, typ ChunkType, neighbors ...Map) (Map, error) {
	if !typ.valid() {
		return Map{}, fmt.Errorf("maze.Generate: unknown chunk type %v (%d)", typ, int8(typ))
	}
	neighborMaps, err := resolveNeighborMaps(ch, gen.params.Radius, neighbors)
	if err != nil {
		return Map{}, err
	}

	lattice := gen.params.lattice()
	g := newChunkGraph(lattice)

	islands := selectIslands(g, newStream(chunkKey(gen.seed, purposeIsland, ch)), gen.params, typ)
	algo := drawAlgorithm(newStream(chunkKey(gen.seed, purposeAlgorithm, ch)), gen.params.Weights)
	open := buildSpanningStructure(g, islands, algo, newStream(chunkKey(gen.seed, purposeStructure, ch)), biasThreshold(gen.params.GrowingTreeBias))
	addExtraPassages(g, islands, open, newStream(chunkKey(gen.seed, purposeCycle, ch)), gen.params.ExtraPassageShare)

	// A per-neighbour-chunk cache of this chunk's own derived borders:
	// each border's portal set depends only on the seed, the two chunks
	// and the portal shares, so it is computed once per neighbour rather
	// than once per face.
	derivedBorders := map[hexgrid.Chunk]map[hexgrid.Face]bool{}
	derivedBorderPortals := func(neighborChunk hexgrid.Chunk) map[hexgrid.Face]bool {
		if p, ok := derivedBorders[neighborChunk]; ok {
			return p
		}
		candidates := borderCandidates(lattice, ch, neighborChunk)
		p := selectPortals(candidates, newStream(borderKey(gen.seed, ch, neighborChunk)), gen.params.PortalShareLower, gen.params.PortalShareUpper)
		derivedBorders[neighborChunk] = p
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
			neighborChunk, neighborLocal := lattice.Locate(globalNeighbor)

			var passage bool
			if nm, ok := neighborMaps[neighborChunk]; ok {
				nFaces, ok := nm.Faces(neighborLocal)
				if ok {
					passage = nFaces[d.Opposite()] == FacePassage
				} else {
					passage = derivedBorderPortals(neighborChunk)[hexgrid.FaceOf(c, d)]
				}
			} else {
				passage = derivedBorderPortals(neighborChunk)[hexgrid.FaceOf(c, d)]
			}
			if passage {
				faces[idx][d] = FacePassage
			} else {
				faces[idx][d] = FaceWall
			}
		}
	}

	return Map{chunk: ch, typ: typ, version: Version, graph: g, faces: faces}, nil
}

// resolveNeighborMaps validates neighbors against ch and radius, and
// returns them keyed by their own chunk. It refuses a map for ch
// itself, a map not adjacent to ch, a map at a radius other than
// radius, and two maps for the same neighbour — each naming the
// offending map.
func resolveNeighborMaps(ch hexgrid.Chunk, radius int32, neighbors []Map) (map[hexgrid.Chunk]Map, error) {
	out := make(map[hexgrid.Chunk]Map, len(neighbors))
	for _, nm := range neighbors {
		nc := nm.Chunk()
		if nc == ch {
			return nil, fmt.Errorf("maze.Generate: a supplied neighbour map is for ch itself (%v)", ch)
		}
		if nm.Radius() != radius {
			return nil, fmt.Errorf("maze.Generate: supplied neighbour map for %v has radius %d, want %d", nc, nm.Radius(), radius)
		}
		adjacent := false
		for _, d := range sixDirections {
			if ch.Neighbor(d) == nc {
				adjacent = true
				break
			}
		}
		if !adjacent {
			return nil, fmt.Errorf("maze.Generate: supplied neighbour map for %v is not adjacent to %v", nc, ch)
		}
		if _, exists := out[nc]; exists {
			return nil, fmt.Errorf("maze.Generate: two supplied neighbour maps for the same chunk %v", nc)
		}
		out[nc] = nm
	}
	return out, nil
}
