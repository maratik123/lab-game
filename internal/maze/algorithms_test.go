package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// countComponents returns how many connected components the open edge
// set forms over the non-island cells of g, via a flood fill through
// open edges only.
func countComponents(g chunkGraph, islands map[int]bool, open edgeSet) int {
	visited := map[int]bool{}
	components := 0
	for idx := 0; idx < g.cellCount(); idx++ {
		if islands[idx] || visited[idx] {
			continue
		}
		components++
		stack := []int{idx}
		visited[idx] = true
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, n := range g.neighbors(cur) {
				if islands[n] || visited[n] || !open.has(cur, n) {
					continue
				}
				visited[n] = true
				stack = append(stack, n)
			}
		}
	}
	return components
}

func assertSpanningTree(t *testing.T, name string, g chunkGraph, islands map[int]bool, open edgeSet) {
	t.Helper()
	nonIslandCount := g.cellCount() - len(islands)
	if got := countComponents(g, islands, open); got != 1 {
		t.Errorf("%s: %d connected components over the non-island cells, want 1", name, got)
	}
	if len(open) != nonIslandCount-1 {
		t.Errorf("%s: edge count = %d, want %d (one below the non-island cell count)", name, len(open), nonIslandCount-1)
	}
}

func TestAlgorithms_EachBuildsASpanningTree(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	algos := []Algorithm{AlgorithmBacktracker, AlgorithmKruskal, AlgorithmPrim, AlgorithmGrowingTree, AlgorithmWilson}
	for _, algo := range algos {
		for seedByte := range 10 {
			s := newStream([32]byte{byte(algo), byte(seedByte)})
			open := buildSpanningStructure(g, map[int]bool{}, algo, s, biasThreshold(half))
			assertSpanningTree(t, algo.String(), g, map[int]bool{}, open)
		}
	}
}

func TestAlgorithms_SpanningTreeOverIslandChunk(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	p := validParams()
	islandStream := newStream([32]byte{55})
	islands := selectIslands(g, islandStream, p, ChunkTypeFabric)
	if len(islands) == 0 {
		t.Fatal("test setup: expected at least one island at the reference share")
	}
	algos := []Algorithm{AlgorithmBacktracker, AlgorithmKruskal, AlgorithmPrim, AlgorithmGrowingTree, AlgorithmWilson}
	for _, algo := range algos {
		s := newStream([32]byte{byte(algo), 77})
		open := buildSpanningStructure(g, islands, algo, s, biasThreshold(half))
		assertSpanningTree(t, algo.String(), g, islands, open)
		for k := range open {
			if islands[k[0]] || islands[k[1]] {
				t.Errorf("%s: spanning edge %v touches an island cell", algo.String(), k)
			}
		}
	}
}

func TestGrowingTree_TwoBiasesProduceDifferentStructures(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	key := [32]byte{3, 3, 3}
	low := buildGrowingTree(g, map[int]bool{}, newStream(key), biasThreshold(zeroShare))
	high := buildGrowingTree(g, map[int]bool{}, newStream(key), biasThreshold(oneShare))
	if len(low) != len(high) {
		t.Fatalf("both are spanning trees over the same chunk so edge counts should match: %d vs %d", len(low), len(high))
	}
	same := true
	for k := range low {
		if !high[k] {
			same = false
			break
		}
	}
	if same {
		t.Error("growing tree at bias 0 and bias 1 produced the identical edge set, want different structures")
	}
}

func TestKruskal_ComponentTrackingHandlesEveryFace(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(refLattice())
	s := newStream([32]byte{8, 8})
	open := buildKruskal(g, map[int]bool{}, s)
	assertSpanningTree(t, "kruskal", g, map[int]bool{}, open)
}

func TestBacktracker_NeverRevisitsACell(t *testing.T) {
	t.Parallel()
	g := newChunkGraph(hexgrid.Lattice{Radius: 6})
	s := newStream([32]byte{1, 1, 1})
	open := buildBacktracker(g, map[int]bool{}, s)
	// A tree has exactly cellCount-1 edges and no cycle; countComponents
	// already checks single-component + edge count via
	// assertSpanningTree, exercised elsewhere. Here we additionally
	// check no self-loop was recorded.
	for k := range open {
		if k[0] == k[1] {
			t.Errorf("backtracker recorded a self-loop edge %v", k)
		}
	}
}
