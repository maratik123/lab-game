package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

// sixDirections is hexgrid's six directions in canonical order.
var sixDirections = [6]hexgrid.Direction{
	hexgrid.DirE, hexgrid.DirNE, hexgrid.DirNW,
	hexgrid.DirW, hexgrid.DirSW, hexgrid.DirSE,
}

// chunkGraph is one hexagonal chunk's cell graph under a Lattice: the
// mapping between a chunk-local coordinate and a plain index — fixed to
// the lattice's own LocalCells order — and the interior (within-chunk)
// six-neighbour adjacency the island guard, the spanning-structure
// algorithms, and the extra-passage pass all read through. It carries no
// seed and no state beyond the lattice.
type chunkGraph struct {
	lattice hexgrid.Lattice
	cells   []hexgrid.Coord
	index   map[hexgrid.Coord]int
}

// newChunkGraph builds the cell graph for a chunk of lattice — lattice's
// radius is assumed already validated (Params.validate), so no error
// return is needed here.
func newChunkGraph(lattice hexgrid.Lattice) chunkGraph {
	cells := lattice.LocalCells()
	index := make(map[hexgrid.Coord]int, len(cells))
	for i, c := range cells {
		index[c] = i
	}
	return chunkGraph{lattice: lattice, cells: cells, index: index}
}

// cellCount returns how many cells the chunk holds.
func (g chunkGraph) cellCount() int {
	return len(g.cells)
}

// localIndex returns the plain index for local coordinate local.
func (g chunkGraph) localIndex(local hexgrid.Coord) int {
	return g.index[local]
}

// localCoord returns idx's chunk-local coordinate — localIndex's
// inverse.
func (g chunkGraph) localCoord(idx int) hexgrid.Coord {
	return g.cells[idx]
}

// isBorderCell reports whether idx's cell has at least one face leaving
// the chunk — equivalently, whether it lies at exactly the lattice's own
// radius from the chunk's centre.
func (g chunkGraph) isBorderCell(idx int) bool {
	return hexgrid.Distance(g.cells[idx], hexgrid.Coord{}) == int64(g.lattice.Radius)
}

// neighbors returns idx's interior neighbours — those of its six
// hex-lattice neighbours that stay inside the chunk. A border cell may
// return fewer than six.
func (g chunkGraph) neighbors(idx int) []int {
	local := g.cells[idx]
	out := make([]int, 0, 6)
	for _, d := range sixDirections {
		if j, ok := g.index[local.Neighbor(d)]; ok {
			out = append(out, j)
		}
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
// once, in a deterministic order depending only on the lattice: cell
// index order, then direction order, keeping each face only from the
// lower of its two endpoints' iteration. This is the same list the
// island guard, the spanning-structure algorithms, and the
// extra-passage pass each build from, so it is built once here rather
// than re-derived per caller.
func (g chunkGraph) interiorFaces() []interiorFace {
	var faces []interiorFace
	for idx, local := range g.cells {
		for _, d := range sixDirections {
			j, ok := g.index[local.Neighbor(d)]
			if !ok || j <= idx {
				continue
			}
			faces = append(faces, interiorFace{a: idx, b: j, dir: d})
		}
	}
	return faces
}
