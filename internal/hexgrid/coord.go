package hexgrid

// Coord is an axial coordinate identifying one hex cell in the infinite
// world lattice.
type Coord struct {
	Q int32
	R int32
}

// Direction identifies one of a hex cell's six neighbours. The zero
// value is DirE; the six values are in the canonical order this package
// fixes, which every enum-indexed weight and every exhaustive switch
// over Direction relies on.
type Direction int8

// The six directions, in canonical order.
const (
	DirE Direction = iota
	DirNE
	DirNW
	DirW
	DirSW
	DirSE
)

// directionDeltas holds each Direction's axial neighbour offset, indexed
// by the Direction's own value.
var directionDeltas = [6]Coord{
	DirE:  {Q: 1, R: 0},
	DirNE: {Q: 1, R: -1},
	DirNW: {Q: 0, R: -1},
	DirW:  {Q: -1, R: 0},
	DirSW: {Q: -1, R: 1},
	DirSE: {Q: 0, R: 1},
}

// Opposite returns the direction pointing the opposite way: DirE and
// DirW, DirNE and DirSW, DirNW and DirSE are the three opposite pairs.
func (d Direction) Opposite() Direction {
	switch d {
	case DirE:
		return DirW
	case DirNE:
		return DirSW
	case DirNW:
		return DirSE
	case DirW:
		return DirE
	case DirSW:
		return DirNE
	case DirSE:
		return DirNW
	default:
		return d
	}
}

// Neighbor returns the coordinate one step from c in direction d.
// Arithmetic at the domain's extremes wraps, which Go defines for
// signed integer overflow, so there is no panic path here.
func (c Coord) Neighbor(d Direction) Coord {
	delta := directionDeltas[d]
	return Coord{Q: c.Q + delta.Q, R: c.R + delta.R}
}
