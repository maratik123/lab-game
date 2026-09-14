package gate_test

import (
	"errors"
	"math"
	"testing"

	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
)

func TestNext_FirstGateIsCentre(t *testing.T) {
	t.Parallel()
	nonCentre := []hexgrid.Chunk{{Q: 3, R: -2}, {Q: -1, R: 5}}
	cases := []struct {
		name    string
		created []hexgrid.Chunk
		k       int
	}{
		{"nil created, k=0", nil, 0},
		{"nil created, k=1", nil, 1},
		{"nil created, k=5", nil, 5},
		{"non-centre created, k=0", nonCentre, 0},
		{"non-centre created, k=1", nonCentre, 1},
		{"non-centre created, k=5", nonCentre, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := gate.Next(c.created, nil, c.k)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got != (hexgrid.Chunk{}) {
				t.Errorf("Next(%v, nil, %d) = %v, want the centre", c.created, c.k, got)
			}
		})
	}
}

func TestNext_Table(t *testing.T) {
	t.Parallel()
	ringThrough := func(n int) []hexgrid.Chunk {
		out := make([]hexgrid.Chunk, 0, 1+3*n*(n+1))
		for ch := range gate.Spiral() {
			if hexgrid.ChunkDistance(hexgrid.Chunk{}, ch) > int64(n) {
				break
			}
			out = append(out, ch)
		}
		return out
	}

	cases := []struct {
		name    string
		created []hexgrid.Chunk
		gates   []hexgrid.Chunk
		k       int
		want    hexgrid.Chunk
	}{
		{"centre created, k=0", []hexgrid.Chunk{{}}, nil, 0, hexgrid.Chunk{Q: 1, R: 0}},
		{"gate at centre, k=1", nil, []hexgrid.Chunk{{}}, 1, hexgrid.Chunk{Q: 2, R: -1}},
		{"rings 0-2 all created, k=0 (fallback)", ringThrough(2), nil, 0, hexgrid.Chunk{Q: 3, R: -1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := gate.Next(c.created, c.gates, c.k)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got != c.want {
				t.Errorf("Next(%v, %v, %d) = %v, want %v", c.created, c.gates, c.k, got, c.want)
			}
		})
	}
}

// TestNext_SequentialFillReproducesSpiral checks that, with k = 0,
// repeatedly calling Next and adding its result to both created and
// gates reproduces Spiral()'s own prefix — the way k = 0 fills
// concentric rings one chunk at a time.
func TestNext_SequentialFillReproducesSpiral(t *testing.T) {
	t.Parallel()
	const n = 40
	want := make([]hexgrid.Chunk, 0, n)
	for ch := range gate.Spiral() {
		if len(want) == n {
			break
		}
		want = append(want, ch)
	}

	var created, gates []hexgrid.Chunk
	for i := range n {
		got, err := gate.Next(created, gates, 0)
		if err != nil {
			t.Fatalf("Next at step %d: %v", i, err)
		}
		if got != want[i] {
			t.Fatalf("Next at step %d = %v, want %v (Spiral()'s own order)", i, got, want[i])
		}
		created = append(created, got)
		gates = append(gates, got)
	}
}

func drawChunks(rt *rapid.T, label string) []hexgrid.Chunk {
	n := rapid.IntRange(0, 6).Draw(rt, label+"_n")
	out := make([]hexgrid.Chunk, n)
	for i := range out {
		out[i] = hexgrid.Chunk{
			Q: int32(rapid.Int64Range(-6, 6).Draw(rt, label+"_q")),
			R: int32(rapid.Int64Range(-6, 6).Draw(rt, label+"_r")),
		}
	}
	return out
}

func TestNext_MatchesSpiralScan(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		created := drawChunks(rt, "created")
		gates := drawChunks(rt, "gates")
		k := rapid.IntRange(0, 4).Draw(rt, "k")

		createdSet := make(map[hexgrid.Chunk]bool, len(created))
		for _, ch := range created {
			createdSet[ch] = true
		}

		var want hexgrid.Chunk
		found := false
		i := 0
		const iterationCap = 1 + 3*40*41 // generous: well past any drawn bound
		for ch := range gate.Spiral() {
			if i >= iterationCap {
				break
			}
			i++
			if createdSet[ch] {
				continue
			}
			ok := true
			for _, g := range gates {
				if hexgrid.ChunkDistance(ch, g) < int64(k+1) {
					ok = false
					break
				}
			}
			if ok {
				want = ch
				found = true
				break
			}
		}
		if !found {
			rt.Fatalf("oracle scan exhausted %d chunks without a qualifying one", iterationCap)
		}

		got, err := gate.Next(created, gates, k)
		if err != nil {
			rt.Fatalf("Next: %v", err)
		}
		if got != want {
			rt.Fatalf("Next(%v, %v, %d) = %v, want %v (oracle)", created, gates, k, got, want)
		}
	})
}

func TestNext_AlwaysReturns(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		created := drawChunks(rt, "created")
		gates := drawChunks(rt, "gates")
		k := rapid.IntRange(0, 20).Draw(rt, "k")

		got, err := gate.Next(created, gates, k)
		if err != nil {
			rt.Fatalf("Next: %v", err)
		}
		for _, ch := range created {
			if got == ch {
				rt.Fatalf("Next(%v, %v, %d) = %v, which is already in created", created, gates, k, got)
			}
		}
		for _, g := range gates {
			if d := hexgrid.ChunkDistance(got, g); d < int64(k+1) {
				rt.Fatalf("Next(%v, %v, %d) = %v, distance %d to gate %v < k+1", created, gates, k, got, d, g)
			}
		}
	})
}

func TestNext_RefusesNegativeK(t *testing.T) {
	t.Parallel()
	for _, k := range []int{-1, math.MinInt} {
		got, err := gate.Next(nil, nil, k)
		if !errors.Is(err, gate.ErrNegativeK) {
			t.Errorf("Next(nil, nil, %d) error = %v, want ErrNegativeK", k, err)
		}
		_ = got
	}
}
