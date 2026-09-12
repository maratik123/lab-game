package maze

// edgeKey canonicalises an unordered pair of chunk-local cell indices,
// so an edgeSet needs no separate reverse lookup.
type edgeKey [2]int

func newEdgeKey(a, b int) edgeKey {
	if a > b {
		a, b = b, a
	}
	return edgeKey{a, b}
}

// edgeSet is the set of open interior faces a spanning-structure
// algorithm, the extra-passage pass, or a caller checking their union
// builds up — keyed by the unordered cell-index pair, since openness
// does not depend on direction.
type edgeSet map[edgeKey]bool

func (e edgeSet) add(a, b int) {
	e[newEdgeKey(a, b)] = true
}

func (e edgeSet) has(a, b int) bool {
	return e[newEdgeKey(a, b)]
}

// nonIslandCells returns every cell index of g not in islands, in index
// order.
func nonIslandCells(g chunkGraph, islands map[int]bool) []int {
	out := make([]int, 0, g.cellCount())
	for idx := 0; idx < g.cellCount(); idx++ {
		if !islands[idx] {
			out = append(out, idx)
		}
	}
	return out
}

// nonIslandNeighbors returns idx's interior neighbours that are not
// themselves islands.
func nonIslandNeighbors(g chunkGraph, islands map[int]bool, idx int) []int {
	all := g.neighbors(idx)
	out := make([]int, 0, len(all))
	for _, n := range all {
		if !islands[n] {
			out = append(out, n)
		}
	}
	return out
}

// nonIslandInteriorFaces returns every interior face of g whose two
// endpoints are both non-island, in g's own deterministic enumeration
// order.
func nonIslandInteriorFaces(g chunkGraph, islands map[int]bool) []interiorFace {
	all := g.interiorFaces()
	out := make([]interiorFace, 0, len(all))
	for _, f := range all {
		if !islands[f.a] && !islands[f.b] {
			out = append(out, f)
		}
	}
	return out
}

// buildSpanningStructure builds the spanning structure for the
// non-island cells of g, by algo, drawn from s. biasThresh is only
// consulted by AlgorithmGrowingTree.
func buildSpanningStructure(g chunkGraph, islands map[int]bool, algo Algorithm, s stream, biasThresh uint64) edgeSet {
	switch algo {
	case AlgorithmKruskal:
		return buildKruskal(g, islands, s)
	case AlgorithmPrim:
		return buildFrontierPrim(g, islands, s)
	case AlgorithmGrowingTree:
		return buildGrowingTree(g, islands, s, biasThresh)
	case AlgorithmWilson:
		return buildWilson(g, islands, s)
	case AlgorithmBacktracker:
		return buildBacktracker(g, islands, s)
	default:
		return buildBacktracker(g, islands, s)
	}
}

// buildBacktracker grows a spanning tree by depth-first traversal over
// an explicit stack — never recursion, so the walk has no call-depth
// to exhaust — carving into a random unvisited neighbour and
// backtracking once a cell has none left.
func buildBacktracker(g chunkGraph, islands map[int]bool, s stream) edgeSet {
	open := edgeSet{}
	nonIsland := nonIslandCells(g, islands)
	if len(nonIsland) == 0 {
		return open
	}
	start := nonIsland[boundedDraw(s, uint64(len(nonIsland)))]
	visited := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		var unvisited []int
		for _, n := range nonIslandNeighbors(g, islands, cur) {
			if !visited[n] {
				unvisited = append(unvisited, n)
			}
		}
		if len(unvisited) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		next := unvisited[boundedDraw(s, uint64(len(unvisited)))]
		visited[next] = true
		open.add(cur, next)
		stack = append(stack, next)
	}
	return open
}

// buildKruskal shuffles every non-island interior face and accepts each
// one that joins two still-separate components, tracked by a
// component-label array with relabel-on-merge — this design's one call
// site for that bookkeeping, so no shared disjoint-set helper is
// introduced.
func buildKruskal(g chunkGraph, islands map[int]bool, s stream) edgeSet {
	open := edgeSet{}
	faces := nonIslandInteriorFaces(g, islands)
	shuffle(s, faces)

	label := make([]int, g.cellCount())
	for i := range label {
		label[i] = i
	}
	for _, f := range faces {
		if label[f.a] == label[f.b] {
			continue
		}
		open.add(f.a, f.b)
		old, replacement := label[f.a], label[f.b]
		for i := range label {
			if label[i] == old {
				label[i] = replacement
			}
		}
	}
	return open
}

// buildFrontierPrim grows a tree from one random non-island cell,
// repeatedly picking a random frontier cell (a non-tree cell adjacent
// to the tree) and joining it to a random in-tree neighbour of its
// own.
func buildFrontierPrim(g chunkGraph, islands map[int]bool, s stream) edgeSet {
	open := edgeSet{}
	nonIsland := nonIslandCells(g, islands)
	if len(nonIsland) == 0 {
		return open
	}
	start := nonIsland[boundedDraw(s, uint64(len(nonIsland)))]
	inTree := map[int]bool{start: true}

	var frontier []int
	frontierSet := map[int]bool{}
	addFrontier := func(cell int) {
		for _, n := range nonIslandNeighbors(g, islands, cell) {
			if !inTree[n] && !frontierSet[n] {
				frontierSet[n] = true
				frontier = append(frontier, n)
			}
		}
	}
	addFrontier(start)

	for len(frontier) > 0 {
		//nolint:gosec // G115: boundedDraw(s, uint64(len(frontier))) returns a value < len(frontier), which fits int since it came from one
		i := int(boundedDraw(s, uint64(len(frontier))))
		cell := frontier[i]
		frontier[i] = frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		delete(frontierSet, cell)

		var treeNeighbors []int
		for _, n := range nonIslandNeighbors(g, islands, cell) {
			if inTree[n] {
				treeNeighbors = append(treeNeighbors, n)
			}
		}
		partner := treeNeighbors[boundedDraw(s, uint64(len(treeNeighbors)))]
		open.add(cell, partner)
		inTree[cell] = true
		addFrontier(cell)
	}
	return open
}

// buildGrowingTree grows a tree from one random non-island cell,
// maintaining an active list; each step chooses the active list's
// newest entry or a uniformly random entry per biasedNewest(s,
// biasThresh), then carves into a random unvisited neighbour of the
// chosen cell or drops it from the active list once it has none left.
// At biasThresh's extremes this coincides with the backtracker
// (always-newest) or with a Prim-like random-active-cell walk
// (always-random) by construction of the family, not by a special
// case here.
func buildGrowingTree(g chunkGraph, islands map[int]bool, s stream, biasThresh uint64) edgeSet {
	open := edgeSet{}
	nonIsland := nonIslandCells(g, islands)
	if len(nonIsland) == 0 {
		return open
	}
	start := nonIsland[boundedDraw(s, uint64(len(nonIsland)))]
	visited := map[int]bool{start: true}
	active := []int{start}

	for len(active) > 0 {
		var idx int
		if biasedNewest(s, biasThresh) {
			idx = len(active) - 1
		} else {
			//nolint:gosec // G115: boundedDraw(s, uint64(len(active))) returns a value < len(active), which fits int since it came from one
			idx = int(boundedDraw(s, uint64(len(active))))
		}
		cell := active[idx]

		var candidates []int
		for _, n := range nonIslandNeighbors(g, islands, cell) {
			if !visited[n] {
				candidates = append(candidates, n)
			}
		}
		if len(candidates) == 0 {
			active[idx] = active[len(active)-1]
			active = active[:len(active)-1]
			continue
		}
		next := candidates[boundedDraw(s, uint64(len(candidates)))]
		visited[next] = true
		open.add(cell, next)
		active = append(active, next)
	}
	return open
}

// buildWilson grows a tree by loop-erased random walks: each walk
// starts at an arbitrary not-yet-in-tree cell and takes uniformly
// random steps, recording only the latest outgoing step per cell —
// which is what erases a loop the walk takes, since replaying from the
// walk's start along the recorded steps skips straight past it — until
// it reaches the tree, at which point the recorded path is carved in
// and added to the tree.
func buildWilson(g chunkGraph, islands map[int]bool, s stream) edgeSet {
	open := edgeSet{}
	nonIsland := nonIslandCells(g, islands)
	if len(nonIsland) == 0 {
		return open
	}
	inTree := map[int]bool{}
	inTree[nonIsland[boundedDraw(s, uint64(len(nonIsland)))]] = true

	remaining := make([]int, 0, len(nonIsland))
	for _, c := range nonIsland {
		if !inTree[c] {
			remaining = append(remaining, c)
		}
	}

	for len(remaining) > 0 {
		startIdx := -1
		for i, c := range remaining {
			if !inTree[c] {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			break
		}
		walkStart := remaining[startIdx]

		// nextStep[cell] holds only the LATEST outgoing step taken from
		// cell during this walk — overwriting an earlier entry is what
		// erases a loop, since replaying from walkStart along nextStep
		// never revisits the discarded branch.
		nextStep := map[int]int{}
		cur := walkStart
		for !inTree[cur] {
			neighbors := nonIslandNeighbors(g, islands, cur)
			if len(neighbors) == 0 {
				// An isolated non-island cell with no non-island
				// neighbour: the connectivity guard on island
				// selection makes this unreachable in practice: kept
				// total rather than assumed, so the walk terminates
				// instead of looping forever.
				break
			}
			nxt := neighbors[boundedDraw(s, uint64(len(neighbors)))]
			nextStep[cur] = nxt
			cur = nxt
		}

		c := walkStart
		for !inTree[c] {
			inTree[c] = true
			nc, ok := nextStep[c]
			if !ok {
				break
			}
			open.add(c, nc)
			c = nc
		}

		filtered := remaining[:0]
		for _, x := range remaining {
			if !inTree[x] {
				filtered = append(filtered, x)
			}
		}
		remaining = filtered
	}
	return open
}
