package maze

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// storedCopyOf returns a NewMap rebuild of m — the route a stored-map
// consumer takes to turn a row it read back into a Map.
func storedCopyOf(t *testing.T, m Map) Map {
	t.Helper()
	lattice := hexgrid.Lattice{Radius: m.Radius()}
	cells := lattice.LocalCells()
	faces := make([][6]FaceState, len(cells))
	for idx, local := range cells {
		f, ok := m.Faces(local)
		if !ok {
			t.Fatalf("Faces(%v): ok=false", local)
		}
		faces[idx] = f
	}
	stored, err := NewMap(m.Chunk(), m.Type(), m.Version(), faces)
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}
	return stored
}

func TestNewMap_RoundTripsAGeneratedMap(t *testing.T) {
	t.Parallel()
	for _, r := range []int32{MinRadius, MinRadius + 3} {
		p := refParams()
		p.Radius = r
		gen, err := New(20260912, p)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		for _, typ := range []ChunkType{ChunkTypeFabric, ChunkTypeGate} {
			m, err := gen.Generate(hexgrid.Chunk{Q: 1, R: 1}, typ)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			stored := storedCopyOf(t, m)
			if stored.Chunk() != m.Chunk() || stored.Type() != m.Type() || stored.Version() != m.Version() || stored.Radius() != m.Radius() {
				t.Fatalf("round trip metadata differs: %+v vs original chunk=%v type=%v version=%d radius=%d", stored, m.Chunk(), m.Type(), m.Version(), m.Radius())
			}
			lattice := hexgrid.Lattice{Radius: r}
			for _, local := range lattice.LocalCells() {
				want, _ := m.Faces(local)
				got, ok := stored.Faces(local)
				if !ok || got != want {
					t.Fatalf("radius %d type %v local %v: round-tripped faces = %v (ok=%v), want %v", r, typ, local, got, ok, want)
				}
			}
			// Outside the radius, both report ok=false.
			outside := hexgrid.Coord{Q: r + 100, R: 0}
			if _, ok := m.Faces(outside); ok {
				t.Fatalf("original Map.Faces(%v) = ok=true, want false (outside radius)", outside)
			}
			if _, ok := stored.Faces(outside); ok {
				t.Fatalf("rebuilt Map.Faces(%v) = ok=true, want false (outside radius)", outside)
			}
		}
	}
}

func TestNewMap_Refuses(t *testing.T) {
	t.Parallel()
	lattice := hexgrid.Lattice{Radius: MinRadius}
	validLen := len(lattice.LocalCells())
	validFaces := make([][6]FaceState, validLen)

	t.Run("zero_type", func(t *testing.T) {
		t.Parallel()
		_, err := NewMap(hexgrid.Chunk{}, chunkTypeUnset, 1, validFaces)
		if err == nil {
			t.Fatal("NewMap with zero type = nil error, want an error")
		}
		if !strings.Contains(err.Error(), "0") {
			t.Errorf("NewMap with zero type error = %q, want it to name the value 0", err)
		}
	})
	t.Run("out_of_range_type", func(t *testing.T) {
		t.Parallel()
		_, err := NewMap(hexgrid.Chunk{}, ChunkType(99), 1, validFaces)
		if err == nil {
			t.Fatal("NewMap with an out-of-range type = nil error, want an error")
		}
		if !strings.Contains(err.Error(), "99") {
			t.Errorf("NewMap with an out-of-range type error = %q, want it to name the value 99", err)
		}
	})
	t.Run("version_zero", func(t *testing.T) {
		t.Parallel()
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 0, validFaces)
		if err == nil {
			t.Fatal("NewMap with version 0 = nil error, want an error")
		}
		if !strings.Contains(err.Error(), "0") {
			t.Errorf("NewMap with version 0 error = %q, want it to name the value 0", err)
		}
	})
	t.Run("version_negative", func(t *testing.T) {
		t.Parallel()
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, -1, validFaces)
		if err == nil {
			t.Fatal("NewMap with a negative version = nil error, want an error")
		}
		if !strings.Contains(err.Error(), "-1") {
			t.Errorf("NewMap with a negative version error = %q, want it to name the value -1", err)
		}
	})
	t.Run("length_not_a_hexagon", func(t *testing.T) {
		t.Parallel()
		badLen := validLen + 1
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, make([][6]FaceState, badLen))
		if err == nil {
			t.Fatal("NewMap with a non-hexagon length = nil error, want an error")
		}
		if !strings.Contains(err.Error(), strconv.Itoa(badLen)) {
			t.Errorf("NewMap with a non-hexagon length error = %q, want it to name the length %d", err, badLen)
		}
	})
	t.Run("radius_below_minimum", func(t *testing.T) {
		t.Parallel()
		below := hexgrid.Lattice{Radius: MinRadius - 1}
		badLen := len(below.LocalCells())
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, make([][6]FaceState, badLen))
		if err == nil {
			t.Fatal("NewMap with a radius-5 hexagon's cell count = nil error, want an error")
		}
		if !strings.Contains(err.Error(), strconv.Itoa(badLen)) {
			t.Errorf("NewMap with a radius-5 hexagon's cell count error = %q, want it to name the length %d", err, badLen)
		}
	})
	t.Run("face_state_out_of_range", func(t *testing.T) {
		t.Parallel()
		faces := make([][6]FaceState, validLen)
		faces[0][0] = FaceState(99)
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, faces)
		if err == nil {
			t.Fatal("NewMap with an out-of-range face state = nil error, want an error")
		}
		if !strings.Contains(err.Error(), "99") {
			t.Errorf("NewMap with an out-of-range face state error = %q, want it to name the value 99", err)
		}
	})
	t.Run("interior_face_disagreement", func(t *testing.T) {
		t.Parallel()
		g := newChunkGraph(lattice)
		faces := make([][6]FaceState, validLen)
		intFace := g.interiorFaces()[0]
		faces[intFace.a][intFace.dir] = FacePassage
		faces[intFace.b][intFace.dir.Opposite()] = FaceWall
		_, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, faces)
		if err == nil {
			t.Fatal("NewMap with one interior face flipped on one side only = nil error, want an error")
		}
		aLocal := fmt.Sprintf("%v", g.localCoord(intFace.a))
		bLocal := fmt.Sprintf("%v", g.localCoord(intFace.b))
		dirFragment := fmt.Sprintf("(direction %v)", intFace.dir)
		msg := err.Error()
		if !strings.Contains(msg, aLocal) || !strings.Contains(msg, bLocal) || !strings.Contains(msg, dirFragment) {
			t.Errorf("NewMap interior-face-disagreement error = %q, want it to name both local coordinates %s, %s and %s",
				err, aLocal, bLocal, dirFragment)
		}
	})
	t.Run("control_border_face_flip_is_accepted", func(t *testing.T) {
		t.Parallel()
		g := newChunkGraph(lattice)
		faces := make([][6]FaceState, validLen)
		var borderIdx int
		var borderDir hexgrid.Direction
		found := false
		for idx, local := range g.cells {
			if !g.isBorderCell(idx) {
				continue
			}
			for _, d := range sixDirections {
				if _, ok := g.index[local.Neighbor(d)]; !ok {
					borderIdx, borderDir, found = idx, d, true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatal("test setup: no border-leaving face found")
		}
		faces[borderIdx][borderDir] = FacePassage
		if _, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, faces); err != nil {
			t.Fatalf("NewMap with a border-leaving face flipped = %v, want nil (border faces are taken as given)", err)
		}
	})
}

func TestNewMap_CopiesItsInput(t *testing.T) {
	t.Parallel()
	lattice := hexgrid.Lattice{Radius: MinRadius}
	faces := make([][6]FaceState, len(lattice.LocalCells()))
	m, err := NewMap(hexgrid.Chunk{}, ChunkTypeFabric, 1, faces)
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}
	faces[0][0] = FacePassage
	got, ok := m.Faces(lattice.LocalCells()[0])
	if !ok {
		t.Fatal("Faces: ok=false")
	}
	if got[0] != FaceWall {
		t.Errorf("mutating the input slice after NewMap changed the Map's own Faces: got %v, want unchanged FaceWall", got[0])
	}
}
