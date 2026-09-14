package maze

import (
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// BenchmarkCell reports one cell's generation cost under the equal-weight
// reference Params — a whole-chunk cost, not a per-face cost, since one
// Cell call builds its coordinate's whole chunk fabric. No threshold is asserted here:
// whether a cache is required is a later decision this task defers: the
// measurement is on demand only (go test -run=^$ -bench=. ./internal/maze/).
func BenchmarkCell(b *testing.B) {
	gen, err := New(goldenSeed, goldenParams())
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	coord := hexgrid.Coord{Q: 5, R: 5}
	b.ResetTimer()
	for range b.N {
		gen.Cell(coord)
	}
}

// BenchmarkCellPerAlgorithm reports one cell's generation cost under a
// single-weight Params for each of the five algorithms in turn, so the
// deferred caching decision has the spread across algorithms and not
// only an average — Wilson's loop-erased walk in particular has no
// worst-case bound on its own draw count.
func BenchmarkCellPerAlgorithm(b *testing.B) {
	algos := []struct {
		name string
		algo Algorithm
	}{
		{"backtracker", AlgorithmBacktracker},
		{"kruskal", AlgorithmKruskal},
		{"prim", AlgorithmPrim},
		{"growing_tree", AlgorithmGrowingTree},
		{"wilson_walk", AlgorithmWilson},
	}
	for _, a := range algos {
		b.Run(a.name, func(b *testing.B) {
			var w AlgorithmWeights
			w[a.algo] = 1
			p := goldenParams()
			p.Weights = w
			gen, err := New(goldenSeed, p)
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			coord := hexgrid.Coord{Q: 5, R: 5}
			b.ResetTimer()
			for range b.N {
				gen.Cell(coord)
			}
		})
	}
}
