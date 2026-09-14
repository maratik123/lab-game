package gate

import (
	"iter"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// delta returns the chunk-lattice unit vector for direction d — the
// step Spiral's ring walk and Next's fallback both take on the
// super-lattice.
func delta(d hexgrid.Direction) hexgrid.Chunk {
	return hexgrid.Chunk{}.Neighbor(d)
}

// floorMod64 returns a mod b, in [0, b), for b > 0.
func floorMod64(a, b int64) int64 {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}

// ringChunk returns the chunk at walk position p, p in [0, 6n), of ring
// n (n >= 1) — the one ring walk Spiral, SpiralIndex, Next and Set.Depth
// all share. Position 0 is (n, -floor(n/2)), mid-side of the ring's
// DirE-to-DirNE side rounding toward the DirE corner, and the walk turns
// through the canonical direction order: side d is walked in direction
// d+2 (mod 6). Arithmetic runs in int64 and the result is truncated to
// int32 only at the very end, matching Next's own disclaimer that no
// promise is made near the int32 seam.
func ringChunk(n, p int64) hexgrid.Chunk {
	ringLen := 6 * n
	e := floorMod64(p+n/2, ringLen)
	//nolint:gosec // G115: e is in [0, 6n) by floorMod64's own contract, so e/n is in [0, 6) and fits Direction's int8.
	d := hexgrid.Direction(e / n)
	t := e % n
	corner := delta(d)
	step := delta(hexgrid.Direction((int64(d) + 2) % 6))
	q := n*int64(corner.Q) + t*int64(step.Q)
	r := n*int64(corner.R) + t*int64(step.R)
	//nolint:gosec // G115: ring arithmetic runs in int64 and wraps into int32 at chunk construction by design; no promise is made near the int32 seam.
	return hexgrid.Chunk{Q: int32(q), R: int32(r)}
}

// Spiral returns the deterministic gate-placement order over the chunk
// super-lattice: the centre chunk, then every chunk of ring 1 in the
// order ringChunk walks it, then ring 2, and so on without end. A
// consumer that needs only a prefix stops ranging early — Spiral is an
// infinite sequence.
func Spiral() iter.Seq[hexgrid.Chunk] {
	return func(yield func(hexgrid.Chunk) bool) {
		if !yield(hexgrid.Chunk{}) {
			return
		}
		for n := int64(1); ; n++ {
			ringLen := 6 * n
			for p := int64(0); p < ringLen; p++ {
				if !yield(ringChunk(n, p)) {
					return
				}
			}
		}
	}
}

// SpiralIndex returns ch's position in Spiral's order: 0 for the centre
// chunk, and for a chunk on ring n >= 1 the index 1 + 3n(n-1) (ring n's
// first index) plus its walk position. The side and step within the
// ring are recovered from ch's own coordinates, an independent
// derivation from the walk ringChunk performs, so the two must agree.
func SpiralIndex(ch hexgrid.Chunk) int64 {
	n := hexgrid.ChunkDistance(hexgrid.Chunk{}, ch)
	if n == 0 {
		return 0
	}
	q, r := int64(ch.Q), int64(ch.R)
	s := -q - r
	var d hexgrid.Direction
	var t int64
	switch {
	case q == n && r > -n:
		d, t = hexgrid.DirE, -r
	case r == -n && q > 0:
		d, t = hexgrid.DirNE, n-q
	case s == n && q > -n:
		d, t = hexgrid.DirNW, -q
	case q == -n && r < n:
		d, t = hexgrid.DirW, r
	case r == n && q < 0:
		d, t = hexgrid.DirSW, q+n
	default:
		d, t = hexgrid.DirSE, q
	}
	ringLen := 6 * n
	pos := floorMod64(int64(d)*n+t-n/2, ringLen)
	return 1 + 3*n*(n-1) + pos
}
