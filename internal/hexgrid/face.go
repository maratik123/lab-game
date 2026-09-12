package hexgrid

// Face identifies the single shared boundary between two neighbouring
// cells: the pair (Cell, Dir) such that the neighbouring cell is
// Cell.Neighbor(Dir). Face values are canonical only when built by
// FaceOf — the zero value and a hand-built literal carry no such
// guarantee.
type Face struct {
	Cell Coord
	Dir  Direction
}

// FaceOf returns the canonical Face for the boundary between c and
// c.Neighbor(d): Dir is always the earlier of the two opposite
// directions in canonical order (DirE < DirNE < DirNW), so FaceOf called
// from either side of a neighbouring pair returns one identical value.
func FaceOf(c Coord, d Direction) Face {
	opp := d.Opposite()
	if d < opp {
		return Face{Cell: c, Dir: d}
	}
	return Face{Cell: c.Neighbor(d), Dir: opp}
}
