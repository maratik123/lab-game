package maze

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Generator is an immutable, concurrency-safe deterministic chunk
// generator: New validates every input once, and Cell can neither fail
// nor panic afterwards. It holds no memo — the caching question is a
// later decision this package defers — so one Cell call builds its
// coordinate's whole chunk fabric (unless the coordinate is claimed by
// a PrefabClaimer) and then reads six faces out of it.
type Generator struct {
	seed    int64
	params  Params
	claimer PrefabClaimer
}

// New builds a Generator over seed and params, consulting claimer for
// every future Cell call — claimer may be nil, meaning every coordinate
// receives fabric. It rejects an invalid params, naming the offending
// input.
func New(seed int64, params Params, claimer PrefabClaimer) (*Generator, error) {
	if err := params.validate(); err != nil {
		return nil, fmt.Errorf("maze.New: %w", err)
	}
	return &Generator{seed: seed, params: params, claimer: claimer}, nil
}

// Cell is one hex cell's generated content: its prefab marker, its six
// face states in canonical direction order, and its per-cell seed. A
// prefab-claimed cell defers every interior face and carries no seed.
type Cell struct {
	Prefab bool
	Faces  [6]FaceState
	Seed   uint64
}

// chunksConsulted returns the deduplicated set of chunks Cell(coord)
// reads to answer coord's own six faces: coord's own chunk (always
// first), plus the chunk of each of coord's six face-neighbours. Cell
// calls this same function rather than restating the enumeration, so an
// assertion checked against chunksConsulted is an assertion about the
// path Cell actually takes.
func chunksConsulted(dims hexgrid.Dims, coord hexgrid.Coord) []hexgrid.Chunk {
	own := dims.ChunkOf(coord)
	seen := map[hexgrid.Chunk]bool{own: true}
	chunks := []hexgrid.Chunk{own}
	for _, d := range sixDirections {
		c := dims.ChunkOf(coord.Neighbor(d))
		if !seen[c] {
			seen[c] = true
			chunks = append(chunks, c)
		}
	}
	return chunks
}

// Cell generates coord's content: a pure function of gen's own seed and
// params, and coord alone.
func (gen *Generator) Cell(coord hexgrid.Coord) Cell {
	dims := gen.params.Dims
	own := chunksConsulted(dims, coord)[0]
	claimed := gen.claimer != nil && gen.claimer.Claims(coord)

	var (
		fabricBuilt bool
		fabricGraph chunkGraph
		open        edgeSet
		ownOrigin   hexgrid.Coord
	)
	buildOwnFabric := func() {
		if fabricBuilt {
			return
		}
		fabricBuilt = true
		fabricGraph = newChunkGraph(dims)
		ownOrigin = dims.Origin(own)
		islands := selectIslands(fabricGraph, newStream(chunkKey(gen.seed, purposeIsland, own)), gen.params)
		algo := drawAlgorithm(newStream(chunkKey(gen.seed, purposeAlgorithm, own)), gen.params.Weights)
		open = buildSpanningStructure(fabricGraph, islands, algo, newStream(chunkKey(gen.seed, purposeStructure, own)), biasThreshold(gen.params.GrowingTreeBias))
		addExtraPassages(fabricGraph, islands, open, newStream(chunkKey(gen.seed, purposeCycle, own)), gen.params.ExtraPassageShare)
	}

	var faces [6]FaceState
	for _, d := range sixDirections {
		neighbor := coord.Neighbor(d)
		neighborChunk := dims.ChunkOf(neighbor)

		if neighborChunk == own {
			if claimed {
				faces[d] = FaceDeferred
				continue
			}
			buildOwnFabric()
			a := fabricGraph.localIndex(coord.Q-ownOrigin.Q, coord.R-ownOrigin.R)
			b := fabricGraph.localIndex(neighbor.Q-ownOrigin.Q, neighbor.R-ownOrigin.R)
			if open.has(a, b) {
				faces[d] = FacePassage
			} else {
				faces[d] = FaceWall
			}
			continue
		}

		// A border face: the portal rule, unchanged by any claim on
		// either side — the very value the unclaimed cell across the
		// border reads.
		candidates := borderCandidates(dims, own, neighborChunk)
		portals := selectPortals(candidates, newStream(borderKey(gen.seed, own, neighborChunk)))
		if portals[hexgrid.FaceOf(coord, d)] {
			faces[d] = FacePassage
		} else {
			faces[d] = FaceWall
		}
	}

	var seed uint64
	if !claimed {
		seed = cellSeed(gen.seed, coord)
	}
	return Cell{Prefab: claimed, Faces: faces, Seed: seed}
}
