package gate

import "github.com/maratik123/lab-game/internal/hexgrid"

// Set is a snapshot of a world's gate chunks, indexed once so that
// repeated Depth queries against the same lattice radius cost no more
// than the distance to the nearest gate, never the number of gates in
// the snapshot. Build it once per gate snapshot with NewSet; a caller
// that rebuilds it per query pays the gate count every time.
type Set struct {
	lattice hexgrid.Lattice
	members map[hexgrid.Chunk]struct{}
}

// NewSet builds a Set over lattice holding every chunk in gates. It
// refuses a negative lattice radius with ErrNegativeRadius: Depth's
// stop-rule bound only shrinks as the search widens when the radius is
// negative, and the search would then never terminate. An empty gates
// slice is accepted; the resulting Set's Depth always reports (0,
// false), the same as the zero Set.
func NewSet(lattice hexgrid.Lattice, gates []hexgrid.Chunk) (Set, error) {
	if lattice.Radius < 0 {
		return Set{}, ErrNegativeRadius
	}
	members := make(map[hexgrid.Chunk]struct{}, len(gates))
	for _, g := range gates {
		members[g] = struct{}{}
	}
	return Set{lattice: lattice, members: members}, nil
}

// Depth returns the hex-cell distance from cell to the nearest gate
// chunk's centre cell, and true — or (0, false) when the set holds no
// gate, the zero Set included. It stops at the first ring beyond which
// no chunk centre can lie nearer to cell than the best distance already
// found, so the rings it visits are bounded by that distance and the
// lattice radius, never by the number of gates the set holds.
func (s Set) Depth(cell hexgrid.Coord) (int64, bool) {
	depth, found, _ := s.depthSearch(cell)
	return depth, found
}

// depthSearch is Depth's search, exposing the last chunk ring it
// visited so the internal test suite can check the stated search bound
// directly rather than through timing. It returns (0, false, -1) at
// once for a set with no gate.
func (s Set) depthSearch(cell hexgrid.Coord) (depth int64, found bool, lastRing int64) {
	if len(s.members) == 0 {
		return 0, false, -1
	}
	c0, _ := s.lattice.Locate(cell)
	r := int64(s.lattice.Radius)
	for ring := int64(0); ; ring++ {
		if found && depth <= lowerBound(r, ring) {
			return depth, true, lastRing
		}
		for _, ch := range ringChunksAround(c0, ring) {
			if _, ok := s.members[ch]; !ok {
				continue
			}
			if d := hexgrid.Distance(cell, s.lattice.Center(ch)); !found || d < depth {
				depth, found = d, true
			}
		}
		lastRing = ring
	}
}

// ringChunksAround returns the chunks at ChunkDistance n from centre,
// found by translating the same ring walk Spiral uses.
func ringChunksAround(centre hexgrid.Chunk, n int64) []hexgrid.Chunk {
	if n == 0 {
		return []hexgrid.Chunk{centre}
	}
	out := make([]hexgrid.Chunk, 6*n)
	for p := int64(0); p < 6*n; p++ {
		off := ringChunk(n, p)
		out[p] = hexgrid.Chunk{Q: centre.Q + off.Q, R: centre.R + off.R}
	}
	return out
}

// lowerBound returns L(ring): the least possible hex-cell distance from
// any cell to the centre of any chunk at super-lattice distance ring
// from that cell's own chunk, for a lattice of radius r. It is
// attained, not merely valid, on the mid-side direction the ring walk
// itself starts from.
func lowerBound(r, ring int64) int64 {
	c := 3*r*r + 3*r + 1
	return ceilDiv64(ring*c, 2*r+1) - r
}

// ceilDiv64 returns the ceiling of a/b for a >= 0, b > 0.
func ceilDiv64(a, b int64) int64 {
	return (a + b - 1) / b
}
