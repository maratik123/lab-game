package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

// borderCandidates enumerates every face crossing the border between
// chunks a and b, in the lattice's own canonical border-path order:
// Lattice.Border(d) for d the direction from the lexicographically
// lesser chunk to the greater one, translated from the lattice's local
// frame to absolute coordinates by the lesser chunk's own centre.
// Computing it from either chunk's own call site yields the identical
// list, since the lesser/greater choice does not depend on which one
// calls.
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

// selectPortals draws this border's guaranteed passages from s: a count
// of one or two, then that many distinct faces from candidates,
// shuffled by s and capped by how many candidates exist.
func selectPortals(candidates []hexgrid.Face, s stream) map[hexgrid.Face]bool {
	portals := map[hexgrid.Face]bool{}
	if len(candidates) == 0 {
		return portals
	}
	//nolint:gosec // G115: boundedDraw(s, 2) returns 0 or 1, so the conversion and +1 always fit int
	count := int(boundedDraw(s, 2)) + 1
	if count > len(candidates) {
		count = len(candidates)
	}
	shuffled := append([]hexgrid.Face(nil), candidates...)
	shuffle(s, shuffled)
	for i := 0; i < count; i++ {
		portals[shuffled[i]] = true
	}
	return portals
}
