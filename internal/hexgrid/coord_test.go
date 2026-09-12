package hexgrid_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var allDirections = [6]hexgrid.Direction{
	hexgrid.DirE, hexgrid.DirNE, hexgrid.DirNW,
	hexgrid.DirW, hexgrid.DirSW, hexgrid.DirSE,
}

func drawCoord(rt *rapid.T, label string) hexgrid.Coord {
	return hexgrid.Coord{
		Q: int32(rapid.Int64Range(-1_000_000, 1_000_000).Draw(rt, label+"_q")),
		R: int32(rapid.Int64Range(-1_000_000, 1_000_000).Draw(rt, label+"_r")),
	}
}

func TestOpposite_IsAnInvolutionOverEveryDirection(t *testing.T) {
	t.Parallel()
	for _, d := range allDirections {
		if got := d.Opposite().Opposite(); got != d {
			t.Errorf("%v.Opposite().Opposite() = %v, want %v", d, got, d)
		}
		if d.Opposite() == d {
			t.Errorf("%v is its own opposite", d)
		}
	}
}

func TestNeighbor_SixDistinctSymmetricNeighbours(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		c := drawCoord(rt, "c")
		seen := map[hexgrid.Coord]bool{}
		for _, d := range allDirections {
			n := c.Neighbor(d)
			if n == c {
				rt.Fatalf("neighbor in direction %v equals the cell itself", d)
			}
			if seen[n] {
				rt.Fatalf("direction %v produced a neighbour already seen: %v", d, n)
			}
			seen[n] = true
			// Symmetry: n's neighbour in the opposite direction is c.
			if back := n.Neighbor(d.Opposite()); back != c {
				rt.Fatalf("neighbor(%v).neighbor(opposite) = %v, want %v", d, back, c)
			}
		}
		if len(seen) != 6 {
			rt.Fatalf("got %d distinct neighbours, want 6", len(seen))
		}
	})
}

func TestFaceOf_AgreesFromEitherSide(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		c := drawCoord(rt, "c")
		d := allDirections[rapid.IntRange(0, 5).Draw(rt, "d")]
		n := c.Neighbor(d)

		fromC := hexgrid.FaceOf(c, d)
		fromN := hexgrid.FaceOf(n, d.Opposite())
		if fromC != fromN {
			rt.Fatalf("FaceOf(%v,%v)=%v but FaceOf(%v,%v)=%v", c, d, fromC, n, d.Opposite(), fromN)
		}
	})
}

func TestFaceOf_DistinctDirectionsGiveDistinctFaces(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		c := drawCoord(rt, "c")
		faces := map[hexgrid.Face]bool{}
		for _, d := range allDirections {
			f := hexgrid.FaceOf(c, d)
			if faces[f] {
				rt.Fatalf("direction %v produced a face already seen from cell %v: %v", d, c, f)
			}
			faces[f] = true
		}
	})
}

func TestFaceOf_CanonicalDirectionIsAlwaysTheEarlierOfThePair(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		c := drawCoord(rt, "c")
		d := allDirections[rapid.IntRange(0, 5).Draw(rt, "d")]
		f := hexgrid.FaceOf(c, d)
		if f.Dir >= f.Dir.Opposite() {
			rt.Fatalf("FaceOf(%v,%v) = %v, whose Dir %v is not earlier than its opposite %v", c, d, f, f.Dir, f.Dir.Opposite())
		}
	})
}
