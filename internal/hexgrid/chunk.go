package hexgrid

// Chunk identifies one rectangular block of the world lattice on the
// chunk grid — the grid the danger formulas and the chunk-grid distance
// are measured on, distinct from the hex lattice itself.
type Chunk struct {
	Q int32
	R int32
}

// Dims is the chunk grid's dimensions: how many columns and rows of
// cells one chunk holds. It carries no validation of its own — a
// non-positive dimension is a caller's constructor's concern, not this
// package's, since this package returns no error anywhere.
type Dims struct {
	Cols int32
	Rows int32
}

// floorDiv32 returns the floor of a/b — Go's own "/" truncates toward
// zero, which puts a coordinate just below the origin into the origin's
// own chunk and destroys the chunk partition.
func floorDiv32(a, b int32) int32 {
	q := a / b
	if r := a % b; r != 0 && (r < 0) != (b < 0) {
		q--
	}
	return q
}

// ChunkOf returns the chunk holding c, under d's dimensions.
func (d Dims) ChunkOf(c Coord) Chunk {
	return Chunk{Q: floorDiv32(c.Q, d.Cols), R: floorDiv32(c.R, d.Rows)}
}

// Origin returns the coordinate of ch's first cell — its minimum corner
// under d's dimensions.
func (d Dims) Origin(ch Chunk) Coord {
	return Coord{Q: ch.Q * d.Cols, R: ch.R * d.Rows}
}

// Contains reports whether c lies inside ch under d's dimensions.
func (d Dims) Contains(ch Chunk, c Coord) bool {
	return d.ChunkOf(c) == ch
}

// Neighbor returns the chunk one step from ch in direction d on the
// super-lattice, using the same six direction deltas as a cell's
// Neighbor.
func (ch Chunk) Neighbor(d Direction) Chunk {
	delta := directionDeltas[d]
	return Chunk{Q: ch.Q + delta.Q, R: ch.R + delta.R}
}

// ChunkDistance returns the hex distance between a and b measured on the
// chunk grid — the axial hex-distance formula applied to chunk
// coordinates rather than cell coordinates, widened to int64 because the
// difference of two extreme chunk coordinates does not fit int32.
func ChunkDistance(a, b Chunk) int64 {
	dq := int64(a.Q) - int64(b.Q)
	dr := int64(a.R) - int64(b.R)
	return (abs64(dq) + abs64(dr) + abs64(dq+dr)) / 2
}

// Distance returns the chunk-grid distance between a and b under d's
// dimensions: zero exactly when both lie in the same chunk, and a
// function of their two chunks alone.
func (d Dims) Distance(a, b Coord) int64 {
	return ChunkDistance(d.ChunkOf(a), d.ChunkOf(b))
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
