package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

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
