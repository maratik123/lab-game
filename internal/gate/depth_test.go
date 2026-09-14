package gate_test

import (
	"errors"
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestNewSet_RefusesNegativeRadius(t *testing.T) {
	t.Parallel()
	_, err := gate.NewSet(hexgrid.Lattice{Radius: -1}, nil)
	if !errors.Is(err, gate.ErrNegativeRadius) {
		t.Errorf("NewSet(radius -1) error = %v, want ErrNegativeRadius", err)
	}
}

func TestSetDepth_NoGateReportsFalse(t *testing.T) {
	t.Parallel()
	cells := []hexgrid.Coord{
		{Q: 0, R: 0},
		{Q: 100, R: -50},
		{Q: -100000, R: 100000},
	}

	empty, err := gate.NewSet(hexgrid.Lattice{Radius: 6}, nil)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	for _, cell := range cells {
		if _, ok := empty.Depth(cell); ok {
			t.Errorf("Set built from an empty slice: Depth(%v) reported a gate", cell)
		}
	}

	var zero gate.Set
	for _, cell := range cells {
		if _, ok := zero.Depth(cell); ok {
			t.Errorf("zero Set: Depth(%v) reported a gate", cell)
		}
	}
}

func TestSetDepth_MatchesBruteForce(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		radius := rapid.SampledFrom([]int32{0, 6, 7, 8, 9, 10, 11, 12}).Draw(rt, "radius")
		lat := hexgrid.Lattice{Radius: radius}

		n := rapid.IntRange(1, 8).Draw(rt, "n_gates")
		gates := make([]hexgrid.Chunk, n)
		for i := range gates {
			gates[i] = hexgrid.Chunk{
				Q: int32(rapid.Int64Range(-15, 15).Draw(rt, "gq")),
				R: int32(rapid.Int64Range(-15, 15).Draw(rt, "gr")),
			}
		}
		set, err := gate.NewSet(lat, gates)
		if err != nil {
			rt.Fatalf("NewSet: %v", err)
		}

		bound := int64(60) * (2*int64(radius) + 1)
		cell := hexgrid.Coord{
			Q: int32(rapid.Int64Range(-bound, bound).Draw(rt, "cq")),
			R: int32(rapid.Int64Range(-bound, bound).Draw(rt, "cr")),
		}

		want := hexgrid.Distance(cell, lat.Center(gates[0]))
		for _, g := range gates[1:] {
			if d := hexgrid.Distance(cell, lat.Center(g)); d < want {
				want = d
			}
		}

		got, ok := set.Depth(cell)
		if !ok {
			rt.Fatalf("Depth(%v) reported no gate", cell)
		}
		if got != want {
			rt.Fatalf("Depth(%v) = %d, want %d (brute force over %v)", cell, got, want, gates)
		}
	})
}

func TestSetDepth_Table(t *testing.T) {
	t.Parallel()
	lat := hexgrid.Lattice{Radius: 6}

	t.Run("cell at a gate's centre", func(t *testing.T) {
		t.Parallel()
		gateChunk := hexgrid.Chunk{Q: 2, R: -1}
		set, err := gate.NewSet(lat, []hexgrid.Chunk{gateChunk})
		if err != nil {
			t.Fatalf("NewSet: %v", err)
		}
		got, ok := set.Depth(lat.Center(gateChunk))
		if !ok || got != 0 {
			t.Errorf("Depth(gate's own centre) = (%d, %v), want (0, true)", got, ok)
		}
	})

	t.Run("cell two chunks out", func(t *testing.T) {
		t.Parallel()
		set, err := gate.NewSet(lat, []hexgrid.Chunk{{Q: 0, R: 0}})
		if err != nil {
			t.Fatalf("NewSet: %v", err)
		}
		cell := lat.Center(hexgrid.Chunk{Q: 2, R: 0})
		got, ok := set.Depth(cell)
		if !ok || got != 26 {
			t.Errorf("Depth(%v) = (%d, %v), want (26, true)", cell, got, ok)
		}
	})

	t.Run("a farther-ring gate is nearer in cells", func(t *testing.T) {
		t.Parallel()
		// gate {0,1} is on ring 1 of the cell's own chunk (0,0), at cell
		// distance 19 from cell {0,-6}; gate {1,-2} is on ring 2, but its
		// centre is at cell distance 14 — nearer. A search that stops at
		// the first ring holding a hit would wrongly report 19.
		set, err := gate.NewSet(lat, []hexgrid.Chunk{{Q: 0, R: 1}, {Q: 1, R: -2}})
		if err != nil {
			t.Fatalf("NewSet: %v", err)
		}
		cell := hexgrid.Coord{Q: 0, R: -6}
		got, ok := set.Depth(cell)
		if !ok || got != 14 {
			t.Errorf("Depth(%v) = (%d, %v), want (14, true)", cell, got, ok)
		}
	})
}

func TestSetDepth_FarBeyondEveryGate(t *testing.T) {
	t.Parallel()
	lat := hexgrid.Lattice{Radius: 8}
	gates := []hexgrid.Chunk{
		{Q: 0, R: 0}, {Q: 1, R: -1}, {Q: -1, R: 1}, {Q: 2, R: -1}, {Q: -2, R: 0},
	}
	set, err := gate.NewSet(lat, gates)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}

	bruteForce := func(cell hexgrid.Coord) int64 {
		best := hexgrid.Distance(cell, lat.Center(gates[0]))
		for _, g := range gates[1:] {
			if d := hexgrid.Distance(cell, lat.Center(g)); d < best {
				best = d
			}
		}
		return best
	}

	cases := []struct {
		name string
		cell hexgrid.Coord
	}{
		{"corner direction", lat.Center(hexgrid.Chunk{Q: 300, R: 0})},
		{"mid-side direction", lat.Center(hexgrid.Chunk{Q: 300, R: -150})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			want := bruteForce(c.cell)
			got, ok := set.Depth(c.cell)
			if !ok || got != want {
				t.Errorf("Depth(%v) = (%d, %v), want (%d, true)", c.cell, got, ok, want)
			}
		})
	}
}
