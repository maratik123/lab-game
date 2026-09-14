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
}

// sharesVertex reports whether two distinct faces share a vertex: two
// faces {x,y} and {u,v} (each named by the pair of cells it separates)
// share a vertex exactly when the four cells lie inside one triangle of
// three mutually adjacent cells — equivalently, one face's two cells
// each neighbour one of the other's two cells (or coincide with it).
func sharesVertex(f, g hexgrid.Face) bool {
	fx, fy := f.Cell, f.Cell.Neighbor(f.Dir)
	gx, gy := g.Cell, g.Cell.Neighbor(g.Dir)
	cellsAdjacentOrEqual := func(a, b hexgrid.Coord) bool {
		if a == b {
			return true
		}
		for _, d := range allDirections {
			if a.Neighbor(d) == b {
				return true
			}
		}
		return false
	}
	// {fx,fy} and {gx,gy} share a vertex when, pairing up the cells
	// correctly, one pair coincides and the crossing pair is adjacent —
	// i.e. all four cells are mutually adjacent-or-equal in the two
	// possible pairings across the two faces.
	pairingOK := func(a1, a2, b1, b2 hexgrid.Coord) bool {
		return cellsAdjacentOrEqual(a1, b1) && cellsAdjacentOrEqual(a2, b2)
	}
	return pairingOK(fx, fy, gx, gy) || pairingOK(fx, fy, gy, gx)
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
			// Translating the neighbour's Border(d.Opposite()) by
			// Offset(d) gives the identical list, order included.
			neighborBorder := l.Border(d.Opposite())
			offset := l.Offset(d)
			if len(neighborBorder) != len(border) {
				t.Fatalf("R=%d Border(%v) and Border(%v) differ in length", r, d, d.Opposite())
			}
			for i, f := range neighborBorder {
				translated := hexgrid.Face{Cell: hexgrid.Coord{Q: f.Cell.Q + offset.Q, R: f.Cell.R + offset.R}, Dir: f.Dir}
				if translated != border[i] {
					t.Fatalf("R=%d Border(%v)[%d] = %v, translating Border(%v)[%d]=%v by Offset(%v)=%v gives %v",
						r, d, i, border[i], d.Opposite(), i, f, d, offset, translated)
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
