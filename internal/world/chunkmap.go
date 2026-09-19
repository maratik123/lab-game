package world

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
)

// faceBitsMask is the mask of the six bits this format assigns to a
// cell's six faces, in the lattice's own canonical direction order.
// Every other bit of a stored byte must be zero.
const faceBitsMask = 0b0011_1111

// encode returns m's stored byte representation: one byte per cell in
// the lattice's own local-cell order, bit d set iff face d is a
// passage. m's own radius fixes the lattice this walks, so the result
// always has m's own cell count length.
func encode(m maze.Map) []byte {
	lattice := hexgrid.Lattice{Radius: m.Radius()}
	cells := lattice.LocalCells()
	out := make([]byte, len(cells))
	for i, c := range cells {
		faces, _ := m.Faces(c)
		var b byte
		for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
			if faces[d] == maze.FacePassage {
				b |= 1 << uint(d)
			}
		}
		out[i] = b
	}
	return out
}

// decode rebuilds ch's Map from its stored type, generation version and
// encoded blob, checking blob's length against lattice's own cell count
// (ErrRadiusMismatch, naming both) and every byte's two high bits
// (naming the offending cell and byte). It hands the decoded faces to
// the generator's own map constructor, so the interior-agreement and
// face-state refusals stay that constructor's.
func decode(ch hexgrid.Chunk, typ ChunkType, version int32, blob []byte, lattice hexgrid.Lattice) (maze.Map, error) {
	want := lattice.CellCount()
	if int64(len(blob)) != want {
		return maze.Map{}, fmt.Errorf("%w: stored blob has %d bytes, want %d", ErrRadiusMismatch, len(blob), want)
	}

	mazeType, err := typ.toMaze()
	if err != nil {
		return maze.Map{}, err
	}

	cells := lattice.LocalCells()
	faces := make([][6]maze.FaceState, len(blob))
	for i, b := range blob {
		if b&^faceBitsMask != 0 {
			return maze.Map{}, fmt.Errorf("world: cell %v byte %#x has a bit set outside the six face bits", cells[i], b)
		}
		for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
			if b&(1<<uint(d)) != 0 {
				faces[i][d] = maze.FacePassage
			} else {
				faces[i][d] = maze.FaceWall
			}
		}
	}

	return maze.NewMap(ch, mazeType, version, faces)
}
