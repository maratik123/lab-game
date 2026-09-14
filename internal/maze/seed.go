package maze

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// domainTag versions every hash this package computes. Changing it
// re-mints every world already generated under an existing seed, so a
// change here is a season rotation, never a refactor.
const domainTag = "lab-game/maze/v1"

// The purpose bytes that domain-separate every keyed stream and every
// derived value this package computes. A pass whose own draw count
// changes can never shift another pass's values, because each reads
// from a stream keyed by its own tag.
const (
	purposeCell      byte = 'c'
	purposeIsland    byte = 'i'
	purposeAlgorithm byte = 'a'
	purposeStructure byte = 's'
	purposeCycle     byte = 'x'
	purposeBorder    byte = 'b'
)

// putInt64 appends v's fixed-width, big-endian two's-complement
// encoding to buf. The reinterpretation is the documented encoding this
// package's determinism rests on, not an overflow.
//
//nolint:gosec // G115: two's-complement reinterpretation is the documented fixed-width encoding, not an overflow
func putInt64(buf []byte, v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return append(buf, b[:]...)
}

// putInt32 appends v's fixed-width, big-endian two's-complement
// encoding to buf.
//
//nolint:gosec // G115: two's-complement reinterpretation is the documented fixed-width encoding, not an overflow
func putInt32(buf []byte, v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return append(buf, b[:]...)
}

// worldKey is the root digest every other derivation in this package
// folds itself into: the domain tag followed by the world seed.
func worldKey(seed int64) [32]byte {
	buf := make([]byte, 0, len(domainTag)+8)
	buf = append(buf, domainTag...)
	buf = putInt64(buf, seed)
	return sha256.Sum256(buf)
}

// derive folds purpose and every int32 in coords into wk, in order,
// producing a new domain-separated digest.
func derive(wk [32]byte, purpose byte, coords ...int32) [32]byte {
	buf := make([]byte, 0, len(wk)+1+4*len(coords))
	buf = append(buf, wk[:]...)
	buf = append(buf, purpose)
	for _, c := range coords {
		buf = putInt32(buf, c)
	}
	return sha256.Sum256(buf)
}

// cellKey is the digest for coordinate c under the given world seed —
// a function of the world seed and the coordinate alone, never of the
// chunk radius.
func cellKey(seed int64, c hexgrid.Coord) [32]byte {
	return derive(worldKey(seed), purposeCell, c.Q, c.R)
}

// cellSeed is the per-cell value the world-config and node-content
// layers sample their own content from: the first eight bytes of
// cellKey(seed, c), read big-endian.
func cellSeed(seed int64, c hexgrid.Coord) uint64 {
	k := cellKey(seed, c)
	return binary.BigEndian.Uint64(k[:8])
}

// chunkKey is the digest for chunk ch under the given world seed and
// purpose — one distinct digest per purpose, so the island stream, the
// algorithm-draw stream, the spanning-structure stream and the
// extra-passage stream of one chunk are all independent of one another.
func chunkKey(seed int64, purpose byte, ch hexgrid.Chunk) [32]byte {
	return derive(worldKey(seed), purpose, ch.Q, ch.R)
}

// chunkLess orders two chunks: by Q, then by R — the canonical order a
// border's two chunk coordinates are placed in before deriving its key,
// so both sides of a border compute the identical key.
func chunkLess(a, b hexgrid.Chunk) bool {
	if a.Q != b.Q {
		return a.Q < b.Q
	}
	return a.R < b.R
}

// canonicalOrder returns a and b in their canonical (lesser, greater)
// order.
func canonicalOrder(a, b hexgrid.Chunk) (lesser, greater hexgrid.Chunk) {
	if chunkLess(b, a) {
		return b, a
	}
	return a, b
}

// borderKey is the digest for the border between chunks a and b under
// the given world seed. It is a function of the world key and the two
// chunk coordinates in canonical order alone — computing it from either
// chunk's own call site yields the identical key.
func borderKey(seed int64, a, b hexgrid.Chunk) [32]byte {
	lesser, greater := canonicalOrder(a, b)
	return derive(worldKey(seed), purposeBorder, lesser.Q, lesser.R, greater.Q, greater.R)
}
