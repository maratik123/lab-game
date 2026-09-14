package hexgrid

// Lattice is the super-lattice of chunks of radius Radius: chunk (i, j)
// is centred at cell i·(2R+1, −R) + j·(R, R+1) in axial coordinates, so
// the super-lattice is itself an axial hex lattice one level up. A chunk
// holds every cell within hex distance Radius of its centre — 3R²+3R+1
// of them. Its contract holds for the radius a caller has validated
// before constructing it; it makes no promise for any other radius, and
// in particular none at or beyond the int32 seam.
type Lattice struct {
	Radius int32
}

// CellCount returns the number of cells one chunk holds: 3R²+3R+1.
func (l Lattice) CellCount() int64 {
	r := int64(l.Radius)
	return 3*r*r + 3*r + 1
}

// Center returns the cell at chunk ch's centre.
func (l Lattice) Center(ch Chunk) Coord {
	r := int64(l.Radius)
	i, j := int64(ch.Q), int64(ch.R)
	q := i*(2*r+1) + j*r
	rr := -i*r + j*(r+1)
	//nolint:gosec // G115: Center makes no promise beyond the realistic chunk range; near the int32 seam this truncates by design, not oversight.
	return Coord{Q: int32(q), R: int32(rr)}
}

// Offset returns the cell offset from a chunk's centre to its
// neighbour's centre in direction d.
func (l Lattice) Offset(d Direction) Coord {
	return l.Center(Chunk{}.Neighbor(d))
}

// Locate returns the chunk holding cell c and c's coordinate relative to
// that chunk's centre (its local coordinate): the unique chunk whose
// centre lies within Radius of c.
func (l Lattice) Locate(c Coord) (Chunk, Coord) {
	r := int64(l.Radius)
	n := 3*r*r + 3*r + 1
	q, rr := int64(c.Q), int64(c.R)
	iStar := floorDiv64((r+1)*q-r*rr, n)
	jStar := floorDiv64(r*q+(2*r+1)*rr, n)
	for _, di := range [2]int64{0, 1} {
		for _, dj := range [2]int64{0, 1} {
			i, j := iStar+di, jStar+dj
			centerQ := i*(2*r+1) + j*r
			centerR := -i*r + j*(r+1)
			dq := q - centerQ
			dr := rr - centerR
			if dist := (abs64(dq) + abs64(dr) + abs64(dq+dr)) / 2; dist <= r {
				//nolint:gosec // G115: i,j are one of four candidates around a floor-divided estimate, and dq,dr are bounded by ±r once dist<=r holds — all fit int32 for any radius this package is validated for.
				return Chunk{Q: int32(i), R: int32(j)}, Coord{Q: int32(dq), R: int32(dr)}
			}
		}
	}
	// One of the four candidates above always satisfies the distance
	// bound — the four-candidate search over the inverse super-basis
	// partitions the plane exactly, for any radius a caller has
	// validated. This line is not meant to be reached.
	centerQ, centerR := iStar*(2*r+1)+jStar*r, -iStar*r+jStar*(r+1)
	//nolint:gosec // G115: this fallback path is unreached for any validated radius, for the same reason the four-candidate search above never falls through; the conversion mirrors the reached branch.
	return Chunk{Q: int32(iStar), R: int32(jStar)}, Coord{Q: int32(q - centerQ), R: int32(rr - centerR)}
}

// At returns the cell at chunk ch's local coordinate local — the inverse
// of Locate, exact for every int32 coordinate because both the centre
// and the sum are computed in int64 before the single conversion back.
func (l Lattice) At(ch Chunk, local Coord) Coord {
	r := int64(l.Radius)
	i, j := int64(ch.Q), int64(ch.R)
	q := i*(2*r+1) + j*r + int64(local.Q)
	rr := -i*r + j*(r+1) + int64(local.R)
	//nolint:gosec // G115: exact for every int32 coordinate — both the centre and the sum are computed in int64 before this single conversion recovers the original int32 value.
	return Coord{Q: int32(q), R: int32(rr)}
}

// LocalCells returns every local coordinate within Radius of a chunk's
// centre, in canonical order: rows r = −R..R, q ascending within each
// row.
func (l Lattice) LocalCells() []Coord {
	r := l.Radius
	cells := make([]Coord, 0, l.CellCount())
	for row := -r; row <= r; row++ {
		lo, hi := max32(-r, -r-row), min32(r, r-row)
		for q := lo; q <= hi; q++ {
			cells = append(cells, Coord{Q: q, R: row})
		}
	}
	return cells
}

// Border returns the 2R+1 canonical Faces on the boundary between a
// chunk centred at the origin and its neighbour in direction d, ordered
// along the border path: consecutive entries share a vertex. For the
// canonical directions (DirE, DirNE, DirNW) the list is built directly;
// for their opposites it is the canonical list translated by Offset(d).
func (l Lattice) Border(d Direction) []Face {
	r := l.Radius
	switch d {
	case DirE:
		return l.canonicalBorder(func(x int32) Coord { return Coord{Q: r, R: x} }, DirE, DirNE, -r, 0)
	case DirNE:
		return l.canonicalBorder(func(x int32) Coord { return Coord{Q: x, R: -r} }, DirNE, DirNW, 0, r)
	case DirNW:
		return l.canonicalBorder(func(x int32) Coord { return Coord{Q: x, R: -r - x} }, DirNW, DirW, -r, 0)
	default:
		base := l.Border(d.Opposite())
		offset := l.Offset(d)
		out := make([]Face, len(base))
		for i, f := range base {
			out[i] = Face{Cell: Coord{Q: f.Cell.Q + offset.Q, R: f.Cell.R + offset.R}, Dir: f.Dir}
		}
		return out
	}
}

// canonicalBorder builds one canonical border's path over index range
// [lo, hi]. The forward pass walks the lone direction's face at lo,
// then the secondary direction's face followed by the lone direction's
// face at every index after lo, up to hi; that order runs from the
// corner nearer the secondary neighbour to the one nearer the lone
// neighbour's other side, so the result is reversed before it is
// returned to run the other way: from the corner shared with the
// neighbour one step around from lone, to the corner shared with the
// neighbour one step around from secondary. cell maps an index to the
// local cell it names.
func (l Lattice) canonicalBorder(cell func(int32) Coord, lone, secondary Direction, lo, hi int32) []Face {
	faces := make([]Face, 0, 2*int(hi-lo)+1)
	faces = append(faces, FaceOf(cell(lo), lone))
	for x := lo + 1; x <= hi; x++ {
		faces = append(faces, FaceOf(cell(x), secondary), FaceOf(cell(x), lone))
	}
	for i, j := 0, len(faces)-1; i < j; i, j = i+1, j-1 {
		faces[i], faces[j] = faces[j], faces[i]
	}
	return faces
}

// Distance returns the hex distance between two cells, widened to int64
// because the difference of two extreme int32 coordinates does not fit
// int32.
func Distance(a, b Coord) int64 {
	dq := int64(a.Q) - int64(b.Q)
	dr := int64(a.R) - int64(b.R)
	return (abs64(dq) + abs64(dr) + abs64(dq+dr)) / 2
}

// floorDiv64 returns the floor of a/b.
func floorDiv64(a, b int64) int64 {
	q := a / b
	if r := a % b; r != 0 && (r < 0) != (b < 0) {
		q--
	}
	return q
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}
