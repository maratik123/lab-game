package maze

import "github.com/shopspring/decimal"

// islandTarget returns the number of islands a chunk of p.Radius targets
// under p.IslandShare: round(islandShare × cellsInChunk), the cell
// count over the whole chunk, not only its non-border cells — the
// share's own denominator.
func islandTarget(p Params) int64 {
	return roundHalfUp(p.IslandShare.Mul(decimal.NewFromInt(p.lattice().CellCount())))
}

// selectIslands returns the set of chunk-local cell indices chosen as
// islands, deterministically from g, s and p: candidates are the
// chunk's non-border cells in index order, shuffled by s; each
// candidate is taken tentatively and kept only if the chunk's
// remaining non-island cells stay connected over the chunk-induced
// subgraph (interior adjacency only) — a flood fill from the chunk's
// first border cell. The achieved count can fall short of islandTarget
// when the guard keeps rejecting candidates; it never exceeds it.
func selectIslands(g chunkGraph, s stream, p Params) map[int]bool {
	target := islandTarget(p)
	islands := map[int]bool{}
	if target <= 0 {
		return islands
	}

	var candidates []int
	firstBorder := -1
	for idx := 0; idx < g.cellCount(); idx++ {
		if g.isBorderCell(idx) {
			if firstBorder < 0 {
				firstBorder = idx
			}
			continue
		}
		candidates = append(candidates, idx)
	}
	if firstBorder < 0 {
		// Every cell is a border cell (a 1x1, single-row, or
		// single-column chunk) — Params.validate already refuses a
		// positive island share at such dims, so target is 0 and this
		// branch is unreachable in practice; kept total rather than
		// assumed.
		return islands
	}
	shuffle(s, candidates)

	for _, c := range candidates {
		if int64(len(islands)) >= target {
			break
		}
		islands[c] = true
		if !connectedOverInduced(g, islands, firstBorder) {
			delete(islands, c)
		}
	}
	return islands
}

// connectedOverInduced reports whether every cell of g not in islands
// is reachable from start through interior adjacency alone, ignoring
// any cell in islands. The scope is the chunk-induced subgraph and not
// the whole lattice: over the lattice a neighbouring chunk always
// reconnects the set, so the guard would accept every candidate.
func connectedOverInduced(g chunkGraph, islands map[int]bool, start int) bool {
	visited := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, n := range g.neighbors(cur) {
			if islands[n] || visited[n] {
				continue
			}
			visited[n] = true
			stack = append(stack, n)
		}
	}
	return len(visited) == g.cellCount()-len(islands)
}
