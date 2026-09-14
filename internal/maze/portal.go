package maze

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// borderCandidates enumerates every face crossing the border between
// chunks a and b, in the lattice's own canonical border-path order:
// Lattice.Border(d) for d the direction from the lexicographically
// lesser chunk to the greater one, translated from the lattice's local
// frame to absolute coordinates by the lesser chunk's own centre.
// Computing it from either chunk's own call site yields the identical
// list, since the lesser/greater choice does not depend on which one
// calls. Path order matters beyond enumeration here: selectPortals
// reads "non-touching" as "non-consecutive in this list".
func borderCandidates(lattice hexgrid.Lattice, a, b hexgrid.Chunk) []hexgrid.Face {
	lesser, greater := canonicalOrder(a, b)
	var dir hexgrid.Direction
	for _, d := range sixDirections {
		if lesser.Neighbor(d) == greater {
			dir = d
			break
		}
	}
	center := lattice.Center(lesser)
	local := lattice.Border(dir)
	candidates := make([]hexgrid.Face, len(local))
	for i, f := range local {
		candidates[i] = hexgrid.Face{Cell: hexgrid.Coord{Q: center.Q + f.Cell.Q, R: center.R + f.Cell.R}, Dir: f.Dir}
	}
	return candidates
}

// ceilShare returns ⌈share × l⌉, rounding a share of l up to the nearest
// integer.
func ceilShare(share decimal.Decimal, l int) int {
	return int(share.Mul(decimal.NewFromInt(int64(l))).Ceil().IntPart())
}

// portalBounds returns the inclusive [lo, hi] count range this border's
// length l draws its portal count from: lo = ⌈lower×l⌉, hi = ⌈upper×l⌉.
func portalBounds(lower, upper decimal.Decimal, l int) (lo, hi int) {
	return ceilShare(lower, l), ceilShare(upper, l)
}

// selectPortals draws this border's guaranteed passages from s: a count
// uniformly drawn from [lo, hi] (portalBounds over candidates' own
// length and the given shares), then that many pairwise non-consecutive
// positions in candidates' own path order, via the standard
// shuffle-and-rank bijection — exact and uniform over non-touching
// placements, with no rejection loop.
func selectPortals(candidates []hexgrid.Face, s stream, lowerShare, upperShare decimal.Decimal) map[hexgrid.Face]bool {
	portals := map[hexgrid.Face]bool{}
	l := len(candidates)
	if l == 0 {
		return portals
	}
	lo, hi := portalBounds(lowerShare, upperShare, l)
	//nolint:gosec // G115: hi-lo+1 is a small positive count bounded by the border length, which Params.validate has already checked fits within [1, l]
	count := lo + int(boundedDraw(s, uint64(hi-lo+1)))
	for _, pos := range nonConsecutivePositions(s, l, count) {
		portals[candidates[pos]] = true
	}
	return portals
}

// nonConsecutivePositions chooses count pairwise non-consecutive
// positions from [0, l): shuffle the l-count+1 candidate ranks, take the
// first count, sort them ascending, and add each one's own rank — the
// bijection between a count-subset of an (l-count+1)-element set and a
// set of count pairwise non-consecutive positions in an l-element range.
func nonConsecutivePositions(s stream, l, count int) []int {
	if count <= 0 {
		return nil
	}
	slots := l - count + 1
	pool := make([]int, slots)
	for i := range pool {
		pool[i] = i
	}
	shuffle(s, pool)
	chosen := append([]int(nil), pool[:count]...)
	sort.Ints(chosen)
	for i := range chosen {
		chosen[i] += i
	}
	return chosen
}
