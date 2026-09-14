package maze

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Version is the current chunk generation version: every Map Generate
// returns carries it. A generation change that leaves the key chain,
// the stream, or any reduction different from what it was bumps this
// constant; a stored map's own version travels with it and never
// changes on its own.
const Version int32 = 1

// Map is one chunk's generated content: its chunk coordinate, its
// ChunkType, the generation Version it was built under, and every
// local cell's six face states. Faces answers ok=false outside the
// chunk's own radius.
type Map struct {
	chunk   hexgrid.Chunk
	typ     ChunkType
	version int32
	graph   chunkGraph
	faces   [][6]FaceState
}

// Chunk returns the chunk m describes.
func (m Map) Chunk() hexgrid.Chunk {
	return m.chunk
}

// Type returns the ChunkType m was generated under.
func (m Map) Type() ChunkType {
	return m.typ
}

// Version returns the generation version m was built under.
func (m Map) Version() int32 {
	return m.version
}

// Radius returns the chunk radius m's lattice was generated over.
func (m Map) Radius() int32 {
	return m.graph.lattice.Radius
}

// Faces returns local's six face states in canonical direction order.
// ok is false when local lies outside the chunk's own radius.
func (m Map) Faces(local hexgrid.Coord) (faces [6]FaceState, ok bool) {
	idx, ok := m.graph.index[local]
	if !ok {
		return [6]FaceState{}, false
	}
	return m.faces[idx], true
}

// isqrt returns the integer square root of n (n >= 0): the largest
// integer whose square does not exceed n, by Newton's method over
// integers — no float, no math package import.
func isqrt(n int64) int64 {
	if n < 2 {
		return n
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}

// radiusForCellCount returns the radius r such that
// Lattice{Radius: r}.CellCount() == n, solving 3r²+3r+1=n for its
// unique non-negative integer root, and ok=false when n is not the cell
// count of any hexagon.
func radiusForCellCount(n int64) (r int32, ok bool) {
	disc := 12*n - 3
	if disc < 0 {
		return 0, false
	}
	s := isqrt(disc)
	if s*s != disc {
		return 0, false
	}
	num := s - 3
	if num < 0 || num%6 != 0 {
		return 0, false
	}
	root := num / 6
	if root > 2147483647 {
		return 0, false
	}
	return int32(root), true
}

// NewMap rebuilds a Map from what a stored map holds: the chunk, its
// type, its generation version, and every cell's six face states, in
// LocalCells order for whatever radius that many faces names. It copies
// faces, and it refuses, naming each: an unknown or zero type; a
// version below 1; a length that is not the cell count of a hexagon of
// radius MinRadius or more; a face state that is neither wall nor
// passage; and an interior face on which the two cells it joins
// disagree, naming both local coordinates and the direction. A face
// leaving the chunk is taken as given — NewMap has no second side to
// check it against.
func NewMap(ch hexgrid.Chunk, typ ChunkType, version int32, faces [][6]FaceState) (Map, error) {
	if !typ.valid() {
		return Map{}, fmt.Errorf("maze.NewMap: unknown chunk type %v", typ)
	}
	if version < 1 {
		return Map{}, fmt.Errorf("maze.NewMap: version must be at least 1, got %d", version)
	}
	r, ok := radiusForCellCount(int64(len(faces)))
	if !ok || r < MinRadius {
		return Map{}, fmt.Errorf("maze.NewMap: %d faces is not the cell count of a hexagon of radius %d or more", len(faces), MinRadius)
	}
	g := newChunkGraph(hexgrid.Lattice{Radius: r})

	for idx, cellFaces := range faces {
		for _, state := range cellFaces {
			if state != FaceWall && state != FacePassage {
				return Map{}, fmt.Errorf("maze.NewMap: cell %v has a face state outside wall and passage: %v", g.localCoord(idx), state)
			}
		}
	}
	for _, intFace := range g.interiorFaces() {
		aState := faces[intFace.a][intFace.dir]
		bState := faces[intFace.b][intFace.dir.Opposite()]
		if aState != bState {
			return Map{}, fmt.Errorf("maze.NewMap: interior face between %v and %v (direction %v) disagrees: %v vs %v",
				g.localCoord(intFace.a), g.localCoord(intFace.b), intFace.dir, aState, bState)
		}
	}

	copied := make([][6]FaceState, len(faces))
	copy(copied, faces)
	return Map{chunk: ch, typ: typ, version: version, graph: g, faces: copied}, nil
}
