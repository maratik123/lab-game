package maze

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Generator is an immutable, concurrency-safe deterministic chunk
// generator: New validates every input once, and Cell can neither fail
// nor panic afterwards. It holds no memo — the caching question is a
// later decision this package defers — so one Cell call builds its
// coordinate's whole chunk fabric and then reads six faces out of it.
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

// chunksConsulted returns the deduplicated set of chunks Cell(coord)
// reads to answer coord's own six faces: coord's own chunk (always
// first), plus the chunk of each of coord's six face-neighbours. Cell
// calls this same function rather than restating the enumeration, so an
// assertion checked against chunksConsulted is an assertion about the
// path Cell actually takes.
func chunksConsulted(lattice hexgrid.Lattice, coord hexgrid.Coord) []hexgrid.Chunk {
	own, _ := lattice.Locate(coord)
	seen := map[hexgrid.Chunk]bool{own: true}
	chunks := []hexgrid.Chunk{own}
	for _, d := range sixDirections {
		c, _ := lattice.Locate(coord.Neighbor(d))
		if !seen[c] {
			seen[c] = true
			chunks = append(chunks, c)
		}
	}
	return chunks
}

// Cell is one hex cell's generated content: its six face states in
// canonical direction order, and its per-cell seed.
type Cell struct {
	Faces [6]FaceState
	Seed  uint64
}

// Cell generates coord's content: a pure function of gen's own seed and
// params, and coord alone.
func (gen *Generator) Cell(coord hexgrid.Coord) Cell {
	lattice := gen.params.lattice()
	own := chunksConsulted(lattice, coord)[0]
	_, ownLocal := lattice.Locate(coord)

	var (
		fabricBuilt bool
		fabricGraph chunkGraph
		open        edgeSet
	)
	buildOwnFabric := func() {
		if fabricBuilt {
			return
		}
		fabricBuilt = true
		fabricGraph = newChunkGraph(lattice)
		islands := selectIslands(fabricGraph, newStream(chunkKey(gen.seed, purposeIsland, own)), gen.params)
		algo := drawAlgorithm(newStream(chunkKey(gen.seed, purposeAlgorithm, own)), gen.params.Weights)
		open = buildSpanningStructure(fabricGraph, islands, algo, newStream(chunkKey(gen.seed, purposeStructure, own)), biasThreshold(gen.params.GrowingTreeBias))
		addExtraPassages(fabricGraph, islands, open, newStream(chunkKey(gen.seed, purposeCycle, own)), gen.params.ExtraPassageShare)
	}

	var faces [6]FaceState
	for _, d := range sixDirections {
		neighbor := coord.Neighbor(d)
		neighborChunk, neighborLocal := lattice.Locate(neighbor)

		if neighborChunk == own {
			buildOwnFabric()
			a := fabricGraph.localIndex(ownLocal)
			b := fabricGraph.localIndex(neighborLocal)
			if open.has(a, b) {
				faces[d] = FacePassage
			} else {
				faces[d] = FaceWall
			}
			continue
		}

		// A border face: the portal rule.
		candidates := borderCandidates(lattice, own, neighborChunk)
		portals := selectPortals(candidates, newStream(borderKey(gen.seed, own, neighborChunk)))
		if portals[hexgrid.FaceOf(coord, d)] {
			faces[d] = FacePassage
		} else {
			faces[d] = FaceWall
		}
	}

	return Cell{Faces: faces, Seed: cellSeed(gen.seed, coord)}
}
