package world

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
)

func TestChunkmap_RoundTrip(t *testing.T) {
	t.Parallel()

	gen := testGenerator(t, 20260917)
	lattice := hexgrid.Lattice{Radius: testParams().Radius}

	for _, ch := range []hexgrid.Chunk{{Q: 0, R: 0}, {Q: 1, R: -1}, {Q: 2, R: 3}} {
		ch := ch
		t.Run(fmt.Sprintf("chunk_%d_%d", ch.Q, ch.R), func(t *testing.T) {
			t.Parallel()

			m, err := gen.Generate(ch, maze.ChunkTypeFabric)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			wt, err := chunkTypeFromMaze(m.Type())
			if err != nil {
				t.Fatalf("chunkTypeFromMaze: %v", err)
			}
			blob := encode(m)
			got, err := decode(m.Chunk(), wt, m.Version(), blob, lattice)
			if err != nil {
				t.Fatalf("decode(encode(m)): %v", err)
			}
			mapsEqual(t, got, m)
		})
	}
}

// parseGolden reads a chunkmap golden file: a header comment line, then
// one "cell(q,r)=hh" line per cell, in the lattice's own local-cell
// order.
func parseGolden(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open golden: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		eq := strings.LastIndex(line, "=")
		if eq < 0 {
			t.Fatalf("malformed golden line: %q", line)
		}
		b, err := strconv.ParseUint(line[eq+1:], 16, 8)
		if err != nil {
			t.Fatalf("parse golden byte %q: %v", line, err)
		}
		out = append(out, byte(b))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan golden: %v", err)
	}
	return out
}

// TestChunkmap_Golden pins the encoded byte layout at the generator's
// own minimum radius,
// built from the parity rule stated in the design: a face is a passage
// iff the sum of its canonical Face's cell Q, cell R and direction
// ordinal is even — which makes the fixture satisfy the generator's own
// interior-agreement check by construction, with no generator and no
// seed involved. A diff here means the stored byte layout has changed,
// which is a data-migration event for every stored chunk, never a
// refactor.
func TestChunkmap_Golden(t *testing.T) {
	t.Parallel()

	lattice := hexgrid.Lattice{Radius: maze.MinRadius}
	cells := lattice.LocalCells()
	faces := make([][6]maze.FaceState, len(cells))
	for i, c := range cells {
		for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
			f := hexgrid.FaceOf(c, d)
			sum := int(f.Cell.Q) + int(f.Cell.R) + int(f.Dir)
			if sum%2 == 0 {
				faces[i][d] = maze.FacePassage
			} else {
				faces[i][d] = maze.FaceWall
			}
		}
	}
	ch := hexgrid.Chunk{Q: 0, R: 0}
	m, err := maze.NewMap(ch, maze.ChunkTypeFabric, 1, faces)
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	want := parseGolden(t, "testdata/chunkmap.golden")
	got := encode(m)
	if !bytes.Equal(got, want) {
		t.Fatalf("encode(m) = %x, want %x (the stored byte layout changed)", got, want)
	}

	// The generation version travels beside the blob, not inside it: a
	// separate round trip pins that it survives unchanged.
	wt, err := chunkTypeFromMaze(m.Type())
	if err != nil {
		t.Fatalf("chunkTypeFromMaze: %v", err)
	}
	back, err := decode(ch, wt, 7, got, lattice)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.Version() != 7 {
		t.Fatalf("decoded version = %d, want 7", back.Version())
	}
}

func TestChunkmap_Refusals(t *testing.T) {
	t.Parallel()

	lattice := hexgrid.Lattice{Radius: maze.MinRadius}
	ch := hexgrid.Chunk{Q: 0, R: 0}
	valid := make([]byte, lattice.CellCount())

	t.Run("blob_too_short", func(t *testing.T) {
		t.Parallel()
		_, err := decode(ch, ChunkTypeFabric, 1, valid[:len(valid)-1], lattice)
		if !errors.Is(err, ErrRadiusMismatch) {
			t.Fatalf("err = %v, want ErrRadiusMismatch", err)
		}
	})

	t.Run("blob_too_long", func(t *testing.T) {
		t.Parallel()
		_, err := decode(ch, ChunkTypeFabric, 1, append(append([]byte(nil), valid...), 0), lattice)
		if !errors.Is(err, ErrRadiusMismatch) {
			t.Fatalf("err = %v, want ErrRadiusMismatch", err)
		}
	})

	t.Run("high_bit_set", func(t *testing.T) {
		t.Parallel()
		blob := append([]byte(nil), valid...)
		blob[0] = 0x40
		if _, err := decode(ch, ChunkTypeFabric, 1, blob, lattice); err == nil {
			t.Fatal("decode with a high bit set: want an error, got nil")
		}
	})

	t.Run("interior_disagreement_surfaces_as_generator_refusal", func(t *testing.T) {
		t.Parallel()
		blob := append([]byte(nil), valid...)
		cells := lattice.LocalCells()
		// Pick an interior cell (not on the chunk's own radius) and give
		// it a passage on a face whose neighbour disagrees (still wall).
		for i, c := range cells {
			if hexgrid.Distance(c, hexgrid.Coord{}) < int64(lattice.Radius) {
				blob[i] = 1 // DirE passage, unmatched by its neighbour
				break
			}
		}
		_, err := decode(ch, ChunkTypeFabric, 1, blob, lattice)
		if err == nil {
			t.Fatal("decode with a disagreeing interior face: want an error, got nil")
		}
		if errors.Is(err, ErrRadiusMismatch) {
			t.Fatalf("err = %v, want the generator's own interior-agreement refusal, not ErrRadiusMismatch", err)
		}
	})
}
