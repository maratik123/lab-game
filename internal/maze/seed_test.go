package maze

import (
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

var updateGolden = flag.Bool("update", false, "update the golden files")

// deriveGoldenLines builds every line the derivation golden pins:
// the raw preimage encoding for a table of signed inputs, then the
// derived keys and cell seed over a fixed world seed and a handful of
// coordinates, chunks and a border.
func deriveGoldenLines() []string {
	var lines []string
	add := func(name string, b []byte) {
		lines = append(lines, fmt.Sprintf("%s=%s", name, hex.EncodeToString(b)))
	}

	for _, v := range []int32{0, 1, -1, math.MaxInt32, math.MinInt32, 12345, -12345} {
		add(fmt.Sprintf("putInt32(%d)", v), putInt32(nil, v))
	}
	for _, v := range []int64{0, 1, -1, math.MaxInt64, math.MinInt64, 20260912} {
		add(fmt.Sprintf("putInt64(%d)", v), putInt64(nil, v))
	}

	const seed int64 = 20260912
	wk := worldKey(seed)
	add("worldKey", wk[:])

	coords := []hexgrid.Coord{{Q: 0, R: 0}, {Q: 5, R: -3}, {Q: -100, R: 100}}
	for _, c := range coords {
		k := cellKey(seed, c)
		add(fmt.Sprintf("cellKey(%d,%d)", c.Q, c.R), k[:])
		var seedBuf [8]byte
		putUint64(seedBuf[:0], cellSeed(seed, c))
		add(fmt.Sprintf("cellSeed(%d,%d)", c.Q, c.R), seedBuf[:])
	}

	chunks := []hexgrid.Chunk{{Q: 0, R: 0}, {Q: -2, R: 3}}
	purposes := []struct {
		name string
		p    byte
	}{
		{"island", purposeIsland},
		{"algorithm", purposeAlgorithm},
		{"structure", purposeStructure},
		{"cycle", purposeCycle},
	}
	for _, ch := range chunks {
		for _, p := range purposes {
			k := chunkKey(seed, p.p, ch)
			add(fmt.Sprintf("chunkKey(%s,%d,%d)", p.name, ch.Q, ch.R), k[:])
		}
	}

	bk := borderKey(seed, hexgrid.Chunk{Q: 0, R: 0}, hexgrid.Chunk{Q: 1, R: 0})
	add("borderKey((0,0),(1,0))", bk[:])
	bkReversed := borderKey(seed, hexgrid.Chunk{Q: 1, R: 0}, hexgrid.Chunk{Q: 0, R: 0})
	add("borderKey((1,0),(0,0))", bkReversed[:])

	return lines
}

func putUint64(buf []byte, v uint64) []byte {
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	return append(buf, b...)
}

const goldenPath = "testdata/derive.golden"

func TestDerive_Golden(t *testing.T) {
	t.Parallel()
	lines := deriveGoldenLines()
	got := strings.Join(lines, "\n") + "\n"

	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v (run with -update to mint it)", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("derivation golden mismatch — the domain tag, the encoding, the digest, or a reduction changed.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCellSeed_PairwiseDistinctOverACoordinateTable(t *testing.T) {
	t.Parallel()
	const seed int64 = 777
	seen := map[uint64]hexgrid.Coord{}
	for q := int32(-5); q <= 5; q++ {
		for r := int32(-5); r <= 5; r++ {
			c := hexgrid.Coord{Q: q, R: r}
			s := cellSeed(seed, c)
			if prev, ok := seen[s]; ok {
				t.Fatalf("cellSeed collision between %v and %v", prev, c)
			}
			seen[s] = c
		}
	}
}

func TestWorldKey_TwoDifferentSeedsMoveEveryKey(t *testing.T) {
	t.Parallel()
	c := hexgrid.Coord{Q: 1, R: 2}
	ch := hexgrid.Chunk{Q: 0, R: 0}
	other := hexgrid.Chunk{Q: 1, R: 0}
	if cellKey(1, c) == cellKey(2, c) {
		t.Error("cellKey unchanged across different world seeds")
	}
	if chunkKey(1, purposeIsland, ch) == chunkKey(2, purposeIsland, ch) {
		t.Error("chunkKey unchanged across different world seeds")
	}
	if borderKey(1, ch, other) == borderKey(2, ch, other) {
		t.Error("borderKey unchanged across different world seeds")
	}
}

func TestBorderKey_CanonicalOrderMakesBothSidesAgree(t *testing.T) {
	t.Parallel()
	a := hexgrid.Chunk{Q: 3, R: -1}
	b := hexgrid.Chunk{Q: 3, R: 0}
	const seed int64 = 99
	if borderKey(seed, a, b) != borderKey(seed, b, a) {
		t.Error("borderKey disagrees depending on call order")
	}
}

func TestChunkKey_PurposesAreIndependent(t *testing.T) {
	t.Parallel()
	const seed int64 = 42
	ch := hexgrid.Chunk{Q: 1, R: 1}
	keys := map[[32]byte]bool{}
	for _, p := range []byte{purposeIsland, purposeAlgorithm, purposeStructure, purposeCycle} {
		k := chunkKey(seed, p, ch)
		if keys[k] {
			t.Fatalf("purpose %q produced a chunkKey already seen", p)
		}
		keys[k] = true
	}
}
