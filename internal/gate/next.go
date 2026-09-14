package gate

import "github.com/maratik123/lab-game/internal/hexgrid"

// Next returns the next chunk gate placement should use: the earliest
// chunk in Spiral's order that is not already in created and that keeps
// hex distance at least k+1 from every chunk in gates. k caps how close
// two gates may sit; k < 0 returns ErrNegativeK, and the returned chunk
// is then not an answer — callers must not use it.
//
// A duplicate entry in created or gates changes nothing, and a gate
// chunk need not also appear in created, since a gate is at distance 0
// from itself.
//
// For k >= 0, Next returns a nil error.
func Next(created, gates []hexgrid.Chunk, k int) (hexgrid.Chunk, error) {
	if k < 0 {
		return hexgrid.Chunk{}, ErrNegativeK
	}
	kk := int64(k)

	m := int64(0)
	for _, ch := range created {
		if d := hexgrid.ChunkDistance(hexgrid.Chunk{}, ch); d > m {
			m = d
		}
	}
	for _, ch := range gates {
		if d := hexgrid.ChunkDistance(hexgrid.Chunk{}, ch); d > m {
			m = d
		}
	}
	bound := m + kk

	createdSet := make(map[hexgrid.Chunk]struct{}, len(created))
	for _, ch := range created {
		createdSet[ch] = struct{}{}
	}

	scanCount := 1 + 3*bound*(bound+1)
	i := int64(0)
	for ch := range Spiral() {
		if i >= scanCount {
			break
		}
		i++
		if _, dup := createdSet[ch]; dup {
			continue
		}
		if farEnough(ch, gates, kk) {
			return ch, nil
		}
	}

	fallbackRing := bound + 1
	//nolint:gosec // G115: ring arithmetic runs in int64 and wraps into int32 at chunk construction by design; no promise is made near the int32 seam.
	return hexgrid.Chunk{Q: int32(fallbackRing), R: int32(-(fallbackRing / 2))}, nil
}

// farEnough reports whether ch keeps hex distance at least k+1 from
// every chunk in gates.
func farEnough(ch hexgrid.Chunk, gates []hexgrid.Chunk, k int64) bool {
	for _, g := range gates {
		if hexgrid.ChunkDistance(ch, g) < k+1 {
			return false
		}
	}
	return true
}
