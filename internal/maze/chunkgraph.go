package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

// sixDirections is hexgrid's six directions in canonical order.
var sixDirections = [6]hexgrid.Direction{
	hexgrid.DirE, hexgrid.DirNE, hexgrid.DirNW,
	hexgrid.DirW, hexgrid.DirSW, hexgrid.DirSE,
}

// chunkGraph is one chunk's cell graph under Dims: the mapping between
// a chunk-local coordinate and a plain index, and the interior
// (within-chunk) six-neighbour adjacency the island guard, the
// spanning-structure algorithms, and the extra-passage pass all read
// through. It carries no seed and no state beyond the dimensions.
type chunkGraph struct {
	dims hexgrid.Dims
}

// newChunkGraph builds the cell graph for a chunk of dims — dims itself
// is assumed already validated (Params.validate), so no error return is
// needed here.
func newChunkGraph(dims hexgrid.Dims) chunkGraph {
	return chunkGraph{dims: dims}
}

// cellCount returns how many cells the chunk holds.
func (g chunkGraph) cellCount() int {
	return int(g.dims.Cols) * int(g.dims.Rows)
}

// localIndex returns the plain index for the chunk-local coordinate
// (lq,lr), row-major.
func (g chunkGraph) localIndex(lq, lr int32) int {
	return int(lr)*int(g.dims.Cols) + int(lq)
}

// localCoord returns idx's chunk-local coordinate — localIndex's
// inverse.
//
//nolint:gosec // G115: idx is always in [0,cellCount()), and cellCount() fits int32 since Dims itself is int32-valued, so both results fit int32
func (g chunkGraph) localCoord(idx int) (lq, lr int32) {
	cols := int(g.dims.Cols)
	return int32(idx % cols), int32(idx / cols)
}

// isBorderCell reports whether idx's cell has at least one face
// leaving the chunk — equivalently, whether it lies on the chunk
// rectangle's own edge.
func (g chunkGraph) isBorderCell(idx int) bool {
	lq, lr := g.localCoord(idx)
	return lq == 0 || lq == g.dims.Cols-1 || lr == 0 || lr == g.dims.Rows-1
}

// neighbors returns idx's interior neighbours — those of its six
// hex-lattice neighbours that stay inside the chunk. A border cell may
// return fewer than six.
func (g chunkGraph) neighbors(idx int) []int {
	lq, lr := g.localCoord(idx)
	out := make([]int, 0, 6)
	for _, d := range sixDirections {
		n := hexgrid.Coord{Q: lq, R: lr}.Neighbor(d)
		if n.Q < 0 || n.Q >= g.dims.Cols || n.R < 0 || n.R >= g.dims.Rows {
			continue
		}
		out = append(out, g.localIndex(n.Q, n.R))
	}
	return out
}

// interiorFace is one interior face of the chunk: the two chunk-local
// cell indices it joins (a < b in the enumeration order below, though
// neither index is itself canonical) and the direction from a to b.
type interiorFace struct {
	a, b int
	dir  hexgrid.Direction
}

// interiorFaces enumerates every interior face of the chunk exactly
// once, in a deterministic order depending only on Dims: cell index
// order, then direction order, keeping each face only from the lower
// of its two endpoints' iteration. This is the same list the island
// guard, the spanning-structure algorithms, and the extra-passage pass
// each build from, so it is built once here rather than re-derived per
// caller.
func (g chunkGraph) interiorFaces() []interiorFace {
	var faces []interiorFace
	for idx := 0; idx < g.cellCount(); idx++ {
		lq, lr := g.localCoord(idx)
		for _, d := range sixDirections {
			n := hexgrid.Coord{Q: lq, R: lr}.Neighbor(d)
			if n.Q < 0 || n.Q >= g.dims.Cols || n.R < 0 || n.R >= g.dims.Rows {
				continue
			}
			nIdx := g.localIndex(n.Q, n.R)
			if nIdx <= idx {
				continue
			}
			faces = append(faces, interiorFace{a: idx, b: nIdx, dir: d})
		}
	}
	return faces
}
