package hexgrid_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var testRadii = []int32{6, 7, 9}

// TestLattice_LocateRoundTripsAndPartitions is an exhaustive check,
// over a region several chunk diameters wide around the origin, that
// Locate finds the one chunk a brute-force scan also finds, that At
// inverts it, and that LocalCells enumerates exactly the cells within
// Radius with no duplicate.
func TestLattice_LocateRoundTripsAndPartitions(t *testing.T) {
	t.Parallel()
	for _, r := range testRadii {
		l := hexgrid.Lattice{Radius: r}
		window := 3 * r
		for q := -window; q <= window; q++ {
			for row := -window; row <= window; row++ {
				c := hexgrid.Coord{Q: q, R: row}
				ch, local := l.Locate(c)
				if dist := hexgrid.Distance(local, hexgrid.Coord{}); dist > int64(r) {
					t.Fatalf("R=%d Locate(%v) local=%v at distance %d > %d", r, c, local, dist, r)
				}
				if back := l.At(ch, local); back != c {
					t.Fatalf("R=%d At(Locate(%v)) = %v, want %v", r, c, back, c)
				}
				// Brute force: find every chunk in a small window
				// around the fast estimate whose centre lies within R
				// of c, and require it be exactly ch.
				found := 0
				for di := int32(-2); di <= 2; di++ {
					for dj := int32(-2); dj <= 2; dj++ {
						cand := hexgrid.Chunk{Q: ch.Q + di, R: ch.R + dj}
						if hexgrid.Distance(l.Center(cand), c) <= int64(r) {
							found++
							if cand != ch {
								t.Fatalf("R=%d cell %v: brute force also matched %v, Locate returned %v", r, c, cand, ch)
							}
						}
					}
				}
				if found != 1 {
					t.Fatalf("R=%d cell %v matched %d chunks in the brute-force window, want 1", r, c, found)
				}
			}
		}
	}
	for _, r := range testRadii {
		l := hexgrid.Lattice{Radius: r}
		seen := map[hexgrid.Coord]bool{}
		cells := l.LocalCells()
		if int64(len(cells)) != l.CellCount() {
			t.Fatalf("R=%d LocalCells has %d entries, want CellCount() = %d", r, len(cells), l.CellCount())
		}
		for _, local := range cells {
			if seen[local] {
				t.Fatalf("R=%d LocalCells duplicate entry %v", r, local)
			}
			seen[local] = true
			if hexgrid.Distance(local, hexgrid.Coord{}) > int64(r) {
				t.Fatalf("R=%d LocalCells entry %v lies outside the radius", r, local)
			}
			ch, back := l.Locate(l.At(hexgrid.Chunk{}, local))
			if ch != (hexgrid.Chunk{}) || back != local {
				t.Fatalf("R=%d At/Locate round trip for local %v gave chunk=%v local=%v", r, local, ch, back)
			}
		}
	}
}

// TestLattice_LocateRoundTripsFarFromOrigin checks the round trip's
// int64 arithmetic over the full int32 domain; it promises nothing
// beyond the realistic chunk range.
func TestLattice_LocateRoundTripsFarFromOrigin(t *testing.T) {
	t.Parallel()
	for _, r := range []int32{6, 9} {
		r := r
		t.Run("", func(t *testing.T) {
			t.Parallel()
			l := hexgrid.Lattice{Radius: r}
			rapid.Check(t, func(rt *rapid.T) {
				c := hexgrid.Coord{
					Q: int32(rapid.Int64Range(-2_000_000_000, 2_000_000_000).Draw(rt, "q")),
					R: int32(rapid.Int64Range(-2_000_000_000, 2_000_000_000).Draw(rt, "r")),
				}
				ch, local := l.Locate(c)
				if back := l.At(ch, local); back != c {
					rt.Fatalf("R=%d At(Locate(%v)) = %v, want %v", r, c, back, c)
				}
			})
		})
	}
}

// TestChunk_SixSymmetricNeighboursJoinedByAFace checks that every chunk has
// six distinct neighbours, each pointing back via the opposite
// direction, and two chunks are neighbours (by Chunk.Neighbor) exactly
// when some cell of one has a face-neighbour in the other.
func TestChunk_SixSymmetricNeighboursJoinedByAFace(t *testing.T) {
	t.Parallel()
	for _, r := range testRadii {
		l := hexgrid.Lattice{Radius: r}
		for _, ch := range []hexgrid.Chunk{{}, {Q: 2, R: -3}, {Q: -1, R: 4}} {
			seen := map[hexgrid.Chunk]bool{}
			for _, d := range allDirections {
				n := ch.Neighbor(d)
				if n == ch {
					t.Fatalf("R=%d chunk %v neighbour in direction %v equals itself", r, ch, d)
				}
				if seen[n] {
					t.Fatalf("R=%d chunk %v direction %v produced a neighbour already seen", r, ch, d)
				}
				seen[n] = true
				if back := n.Neighbor(d.Opposite()); back != ch {
					t.Fatalf("R=%d chunk %v.Neighbor(%v).Neighbor(opposite) = %v, want %v", r, ch, d, back, ch)
				}
				// Brute force: some cell of ch has a face-neighbour in n.
				joined := false
				for _, local := range l.LocalCells() {
					c := l.At(ch, local)
					for _, e := range allDirections {
						if other, _ := l.Locate(c.Neighbor(e)); other == n {
							joined = true
						}
					}
				}
				if !joined {
					t.Fatalf("R=%d chunk %v and its claimed %v-neighbour %v share no face", r, ch, d, n)
				}
			}
			if len(seen) != 6 {
				t.Fatalf("R=%d chunk %v has %d distinct neighbours, want 6", r, ch, len(seen))
			}
			// The converse: every chunk a face of ch leads into is
			// either ch itself or one of the six neighbours just found.
			for _, local := range l.LocalCells() {
				c := l.At(ch, local)
				for _, e := range allDirections {
					other, _ := l.Locate(c.Neighbor(e))
					if other != ch && !seen[other] {
						t.Fatalf("R=%d chunk %v: cell %v's %v-face leads into %v, which is not among its six neighbours",
							r, ch, local, e, other)
					}
				}
			}
		}
	}
}

// TestDistance_EqualsLatticeStepCount checks that hex distance between cells
// straddling a chunk border equals a hand-computed table, and it is
// symmetric.
func TestDistance_EqualsLatticeStepCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b hexgrid.Coord
		want int64
	}{
		{hexgrid.Coord{}, hexgrid.Coord{}, 0},
		{hexgrid.Coord{Q: 1}, hexgrid.Coord{}, 1},
		{hexgrid.Coord{Q: 3, R: -1}, hexgrid.Coord{}, 3},
		{hexgrid.Coord{Q: 10, R: -3}, hexgrid.Coord{Q: 1, R: 1}, 9},
	}
	for _, c := range cases {
		if got := hexgrid.Distance(c.a, c.b); got != c.want {
			t.Errorf("Distance(%v,%v) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := hexgrid.Distance(c.b, c.a); got != c.want {
			t.Errorf("Distance(%v,%v) = %d, want %d (symmetry)", c.b, c.a, got, c.want)
		}
	}
	rapid.Check(t, func(rt *rapid.T) {
		draw := func(label string) hexgrid.Coord {
			return hexgrid.Coord{
				Q: int32(rapid.Int64Range(-1000, 1000).Draw(rt, label+"q")),
				R: int32(rapid.Int64Range(-1000, 1000).Draw(rt, label+"r")),
			}
		}
		a, b := draw("a"), draw("b")
		// Breadth-first step count via repeated Neighbor from a should
		// equal Distance(a,b) for at least the direct-path check: moving
		// one step toward b along a shortest hex path strictly decreases
		// distance by exactly one, for a Distance-many steps.
		steps := int64(0)
		cur := a
		for cur != b && steps <= hexgrid.Distance(a, b)+1 {
			var best hexgrid.Coord
			bestDist := int64(-1)
			for _, d := range allDirections {
				n := cur.Neighbor(d)
				if dist := hexgrid.Distance(n, b); bestDist == -1 || dist < bestDist {
					bestDist, best = dist, n
				}
			}
			cur = best
			steps++
		}
		if steps != hexgrid.Distance(a, b) {
			rt.Fatalf("greedy walk from %v to %v took %d steps, Distance = %d", a, b, steps, hexgrid.Distance(a, b))
		}
	})

	// A genuine breadth-first step count over a small region of the open
	// lattice, from several sources, equals Distance.
	window := int32(15)
	sources := []hexgrid.Coord{{}, {Q: 4, R: -6}, {Q: -5, R: 5}}
	for _, src := range sources {
		steps := bfsStepCounts(src, -window, window, -window, window)
		for q := -window; q <= window; q++ {
			for row := -window; row <= window; row++ {
				c := hexgrid.Coord{Q: q, R: row}
				want := hexgrid.Distance(src, c)
				got, ok := steps[c]
				if !ok || got != want {
					t.Fatalf("BFS step count from %v to %v = %v (ok=%v), want %d", src, c, got, ok, want)
				}
			}
		}
	}

	// A pair of cells straddling a chunk border (built through Lattice)
	// and a pair of adjacent cells within one chunk give the same
	// Distance and the same genuine BFS step count for an equal number
	// of Neighbor steps: crossing the border costs nothing extra.
	for _, r := range []int32{6, 9} {
		l := hexgrid.Lattice{Radius: r}
		borderCell := l.At(hexgrid.Chunk{}, hexgrid.Coord{Q: r})
		acrossBorder := borderCell.Neighbor(hexgrid.DirE)
		if ch, _ := l.Locate(acrossBorder); ch == (hexgrid.Chunk{}) {
			t.Fatalf("R=%d: test setup: %v did not cross into a different chunk", r, acrossBorder)
		}
		within := l.At(hexgrid.Chunk{}, hexgrid.Coord{})
		withinNeighbor := within.Neighbor(hexgrid.DirE)

		straddlingDist := hexgrid.Distance(borderCell, acrossBorder)
		withinDist := hexgrid.Distance(within, withinNeighbor)
		if straddlingDist != withinDist {
			t.Fatalf("R=%d: straddling-pair Distance = %d, within-chunk-pair Distance = %d, want equal", r, straddlingDist, withinDist)
		}
		box := func(c hexgrid.Coord) (loQ, hiQ, loR, hiR int32) {
			return c.Q - 2, c.Q + 2, c.R - 2, c.R + 2
		}
		loQ, hiQ, loR, hiR := box(borderCell)
		if got := bfsStepCounts(borderCell, loQ, hiQ, loR, hiR)[acrossBorder]; got != straddlingDist {
			t.Fatalf("R=%d: straddling-pair BFS steps = %d, want Distance = %d", r, got, straddlingDist)
		}
		loQ, hiQ, loR, hiR = box(within)
		if got := bfsStepCounts(within, loQ, hiQ, loR, hiR)[withinNeighbor]; got != withinDist {
			t.Fatalf("R=%d: within-chunk-pair BFS steps = %d, want Distance = %d", r, got, withinDist)
		}
	}
}

// bfsStepCounts returns, for every cell within the absolute box
// [loQ,hiQ]x[loR,hiR] (which must contain src), the fewest Neighbor
// steps from src — a genuine breadth-first search over the open
// lattice, independent of Distance's own formula.
func bfsStepCounts(src hexgrid.Coord, loQ, hiQ, loR, hiR int32) map[hexgrid.Coord]int64 {
	inBox := func(c hexgrid.Coord) bool {
		return c.Q >= loQ && c.Q <= hiQ && c.R >= loR && c.R <= hiR
	}
	dist := map[hexgrid.Coord]int64{src: 0}
	queue := []hexgrid.Coord{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range allDirections {
			n := cur.Neighbor(d)
			if _, seen := dist[n]; seen || !inBox(n) {
				continue
			}
			dist[n] = dist[cur] + 1
			queue = append(queue, n)
		}
	}
	return dist
}

// sharesVertex reports whether two distinct faces share a vertex. Each
// face is named by the pair of cells it separates, and a hex vertex is
// touched by exactly three mutually adjacent cells, so two distinct
// faces {x,y} and {u,v} share a vertex exactly when {x,y,u,v} collapses
// to three cells — one cell common to both pairs — and the two cells
// left over (one from each face) are themselves adjacent, closing the
// triangle.
func sharesVertex(f, g hexgrid.Face) bool {
	fx, fy := f.Cell, f.Cell.Neighbor(f.Dir)
	gx, gy := g.Cell, g.Cell.Neighbor(g.Dir)
	if (fx == gx && fy == gy) || (fx == gy && fy == gx) {
		return false
	}
	adjacent := func(a, b hexgrid.Coord) bool {
		for _, d := range allDirections {
			if a.Neighbor(d) == b {
				return true
			}
		}
		return false
	}
	switch {
	case fx == gx:
		return adjacent(fy, gy)
	case fx == gy:
		return adjacent(fy, gx)
	case fy == gx:
		return adjacent(fx, gy)
	case fy == gy:
		return adjacent(fx, gx)
	default:
		return false
	}
}

// TestLattice_BorderHasTwoRPlusOneFacesInOneOrderFromEitherSide checks
// each border's face count, order and cross-chunk agreement.
func TestLattice_BorderHasTwoRPlusOneFacesInOneOrderFromEitherSide(t *testing.T) {
	t.Parallel()
	for _, r := range testRadii {
		l := hexgrid.Lattice{Radius: r}
		wantLen := 2*int(r) + 1
		for _, d := range allDirections {
			border := l.Border(d)
			if len(border) != wantLen {
				t.Fatalf("R=%d Border(%v) has %d faces, want %d", r, d, len(border), wantLen)
			}
			// Brute-force set check: a face belongs to this border
			// exactly when its owning cell is local to the chunk in
			// direction d.Opposite() from the neighbour... equivalently,
			// exactly when it appears in the brute-force enumeration
			// over local cells whose neighbour crosses into the
			// d-neighbour chunk.
			bruteSet := map[hexgrid.Face]bool{}
			target := l.Center(hexgrid.Chunk{}.Neighbor(d))
			for _, local := range l.LocalCells() {
				for _, e := range allDirections {
					n := local.Neighbor(e)
					if hexgrid.Distance(n, target) <= int64(r) && hexgrid.Distance(n, hexgrid.Coord{}) > int64(r) {
						bruteSet[hexgrid.FaceOf(local, e)] = true
					}
				}
			}
			gotSet := map[hexgrid.Face]bool{}
			for _, f := range border {
				gotSet[f] = true
			}
			if len(gotSet) != len(bruteSet) {
				t.Fatalf("R=%d Border(%v): got %d distinct faces, brute force %d", r, d, len(gotSet), len(bruteSet))
			}
			for f := range bruteSet {
				if !gotSet[f] {
					t.Fatalf("R=%d Border(%v) missing brute-forced face %v", r, d, f)
				}
			}
			// Consecutive entries share a vertex.
			for i := 1; i < len(border); i++ {
				if !sharesVertex(border[i-1], border[i]) {
					t.Fatalf("R=%d Border(%v)[%d]=%v and [%d]=%v do not share a vertex", r, d, i-1, border[i-1], i, border[i])
				}
			}
			// Reconstruct the order independently of canonicalBorder:
			// start at the face whose tip locates into the designed
			// start corner, then repeatedly walk to the one remaining
			// face sharing a vertex with the current one.
			// This also covers the neighbour's translated agreement,
			// since bruteSet already proves the two chunks' borders are
			// the identical set of faces.
			canonicalDirs := map[hexgrid.Direction]bool{hexgrid.DirE: true, hexgrid.DirNE: true, hexgrid.DirNW: true}
			firstStep := -1
			if !canonicalDirs[d] {
				firstStep = 1
			}
			startCorner := hexgrid.Chunk{}.Neighbor(ringNeighbor(d, firstStep))
			remaining := make(map[hexgrid.Face]bool, len(bruteSet))
			for f := range bruteSet {
				remaining[f] = true
			}
			var cur hexgrid.Face
			found := false
			for f := range remaining {
				for _, z := range flankCells(f.Cell, f.Cell.Neighbor(f.Dir)) {
					if ch, _ := l.Locate(z); ch == startCorner {
						cur, found = f, true
					}
				}
			}
			if !found {
				t.Fatalf("R=%d Border(%v): no brute-forced face touches the designed start corner %v", r, d, startCorner)
			}
			walked := []hexgrid.Face{cur}
			delete(remaining, cur)
			for len(remaining) > 0 {
				var next hexgrid.Face
				ok := false
				for f := range remaining {
					if sharesVertex(cur, f) {
						next, ok = f, true
						break
					}
				}
				if !ok {
					t.Fatalf("R=%d Border(%v): walk stuck after %d of %d faces at %v", r, d, len(walked), len(bruteSet), cur)
				}
				walked = append(walked, next)
				delete(remaining, next)
				cur = next
			}
			if len(walked) != len(border) {
				t.Fatalf("R=%d Border(%v): walked %d faces, Border returned %d", r, d, len(walked), len(border))
			}
			for i, f := range walked {
				if f != border[i] {
					t.Fatalf("R=%d Border(%v)[%d] = %v, independent walk gives %v", r, d, i, border[i], f)
				}
			}
		}
		// The six borders together are every face leaving the chunk,
		// each once.
		total := map[hexgrid.Face]bool{}
		for _, d := range allDirections {
			for _, f := range l.Border(d) {
				if total[f] {
					t.Fatalf("R=%d face %v appears in more than one border", r, f)
				}
				total[f] = true
			}
		}
	}
}

// ringNeighbor returns the direction one step around allDirections'
// fixed ring from d, forward for step +1 and backward for step -1.
func ringNeighbor(d hexgrid.Direction, step int) hexgrid.Direction {
	idx := 0
	for i, e := range allDirections {
		if e == d {
			idx = i
			break
		}
	}
	return allDirections[(idx+step+len(allDirections))%len(allDirections)]
}

// flankCells returns the (up to) two cells adjacent to both a and b —
// the two hex cells that, together with a and b, could close a triangle
// around one of the edge (a,b)'s two vertices.
func flankCells(a, b hexgrid.Coord) []hexgrid.Coord {
	var out []hexgrid.Coord
	for _, e := range allDirections {
		z := a.Neighbor(e)
		if z == b {
			continue
		}
		for _, e2 := range allDirections {
			if b.Neighbor(e2) == z {
				out = append(out, z)
				break
			}
		}
	}
	return out
}

// TestLattice_BorderStartsAndEndsAtTheDesignedCorner checks, independently
// of Border's own construction, that Border(d)'s first face sits at the
// corner where the chunk, its d-neighbour and its ring-previous neighbour
// meet, and its last face sits at the corner shared with its
// ring-next neighbour (the other way around for d's opposites). It
// finds the third chunk by locating the two cells flanking the face's
// edge and checking which one lands in the designed neighbour, rather
// than reusing canonicalBorder's cell/lone/secondary shape.
func TestLattice_BorderStartsAndEndsAtTheDesignedCorner(t *testing.T) {
	t.Parallel()
	canonical := map[hexgrid.Direction]bool{hexgrid.DirE: true, hexgrid.DirNE: true, hexgrid.DirNW: true}
	for _, r := range testRadii {
		l := hexgrid.Lattice{Radius: r}
		for _, d := range allDirections {
			border := l.Border(d)
			// Canonical directions run previous-corner to next-corner;
			// their opposites run the other way.
			firstStep, lastStep := -1, 1
			if !canonical[d] {
				firstStep, lastStep = 1, -1
			}
			check := func(label string, f hexgrid.Face, ringStep int) {
				wantChunk := hexgrid.Chunk{}.Neighbor(ringNeighbor(d, ringStep))
				found := false
				for _, z := range flankCells(f.Cell, f.Cell.Neighbor(f.Dir)) {
					if ch, _ := l.Locate(z); ch == wantChunk {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("R=%d Border(%v) %s face %v: no flanking cell locates into the designed neighbour %v",
						r, d, label, f, wantChunk)
				}
			}
			check("first", border[0], firstStep)
			check("last", border[len(border)-1], lastStep)
		}
	}
}
