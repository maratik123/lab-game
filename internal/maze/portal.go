package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

// borderCandidates enumerates every face crossing the border between
// chunks a and b, in a deterministic order that depends only on dims
// and the two chunks: the lexicographically lesser chunk's own cells in
// row-major order, its own six directions in canonical order, keeping
// only the faces whose destination lies in the greater chunk. Computing
// it from either chunk's own call site yields the identical list, since
// the lesser/greater choice does not depend on which one calls.
func borderCandidates(dims hexgrid.Dims, a, b hexgrid.Chunk) []hexgrid.Face {
	lesser, greater := canonicalOrder(a, b)
	origin := dims.Origin(lesser)

	var candidates []hexgrid.Face
	for lr := int32(0); lr < dims.Rows; lr++ {
		for lq := int32(0); lq < dims.Cols; lq++ {
			if lq != 0 && lq != dims.Cols-1 && lr != 0 && lr != dims.Rows-1 {
				continue // an interior cell can have no face leaving the chunk
			}
			c := hexgrid.Coord{Q: origin.Q + lq, R: origin.R + lr}
			for _, d := range sixDirections {
				if dims.ChunkOf(c.Neighbor(d)) == greater {
					candidates = append(candidates, hexgrid.FaceOf(c, d))
				}
			}
		}
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
