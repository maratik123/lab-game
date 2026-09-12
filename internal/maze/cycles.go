package maze

import "github.com/shopspring/decimal"

// addExtraPassages opens round(share × (nonIslandCells-1)) further
// interior faces in open, beyond the ones the spanning-structure
// algorithm already opened — a share of the spanning structure's own
// edge count, which is what "additional passages" means. Candidates are
// the still-closed interior faces whose both endpoints are non-island,
// in canonical order, shuffled by s, capped by how many such faces
// exist. It never touches a border face, since only interior faces are
// ever candidates here.
func addExtraPassages(g chunkGraph, islands map[int]bool, open edgeSet, s stream, share decimal.Decimal) {
	nonIslandCount := g.cellCount() - len(islands)
	if nonIslandCount <= 1 {
		return
	}
	target := roundHalfUp(share.Mul(decimal.NewFromInt(int64(nonIslandCount - 1))))
	if target <= 0 {
		return
	}

	var candidates []interiorFace
	for _, f := range nonIslandInteriorFaces(g, islands) {
		if !open.has(f.a, f.b) {
			candidates = append(candidates, f)
		}
	}
	shuffle(s, candidates)

	n := target
	if int64(len(candidates)) < n {
		n = int64(len(candidates))
	}
	for i := int64(0); i < n; i++ {
		open.add(candidates[i].a, candidates[i].b)
	}
}
