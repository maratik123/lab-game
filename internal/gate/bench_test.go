package gate_test

import (
	"testing"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
)

// benchWorld returns a fixed 200-chunk world: the first 200 chunks of
// Spiral()'s own order as created, with every third one also a gate.
func benchWorld() (created, gates []hexgrid.Chunk) {
	i := 0
	for ch := range gate.Spiral() {
		if i == 200 {
			break
		}
		created = append(created, ch)
		if i%3 == 0 {
			gates = append(gates, ch)
		}
		i++
	}
	return created, gates
}

// BenchmarkNext reports Next's cost on a fixed 200-chunk world with k=2.
// No threshold is asserted here: whether Next's cost needs bounding is
// a later decision this task defers (go test -run=^$ -bench=. ./internal/gate/).
func BenchmarkNext(b *testing.B) {
	created, gates := benchWorld()
	b.ResetTimer()
	for range b.N {
		if _, err := gate.Next(created, gates, 2); err != nil {
			b.Fatalf("Next: %v", err)
		}
	}
}

// BenchmarkSetDepth reports Set.Depth's cost on a fixed 200-chunk gate
// snapshot for a cell far from every gate. No threshold is asserted
// here, for the same reason as BenchmarkNext.
func BenchmarkSetDepth(b *testing.B) {
	_, gates := benchWorld()
	lat := hexgrid.Lattice{Radius: 8}
	set, err := gate.NewSet(lat, gates)
	if err != nil {
		b.Fatalf("NewSet: %v", err)
	}
	cell := lat.Center(hexgrid.Chunk{Q: 500, R: -250})
	b.ResetTimer()
	for range b.N {
		if _, ok := set.Depth(cell); !ok {
			b.Fatalf("Depth: no gate found")
		}
	}
}
