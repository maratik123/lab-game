package hexgrid

// Chunk identifies one hexagonal chunk on the super-lattice: the
// super-lattice's own axial coordinate, one level up from a cell's own
// Coord. It holds every cell within a validated radius of its centre.
type Chunk struct {
	Q int32
	R int32
}

// Neighbor returns the chunk one step from ch in direction d on the
// super-lattice, using the same six direction deltas as a cell's
// Neighbor.
func (ch Chunk) Neighbor(d Direction) Chunk {
	delta := directionDeltas[d]
	return Chunk{Q: ch.Q + delta.Q, R: ch.R + delta.R}
}

// ChunkDistance returns the hex distance between chunks a and b on the
// super-lattice — the axial hex-distance formula applied to chunk
// coordinates rather than cell coordinates, widened to int64 because the
// difference of two extreme chunk coordinates does not fit int32.
func ChunkDistance(a, b Chunk) int64 {
	dq := int64(a.Q) - int64(b.Q)
	dr := int64(a.R) - int64(b.R)
	return (abs64(dq) + abs64(dr) + abs64(dq+dr)) / 2
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
