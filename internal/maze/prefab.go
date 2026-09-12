package maze

import "github.com/maratik123/lab-game/internal/hexgrid"

// PrefabClaimer decides whether coord belongs to a prefab a later layer
// authors. This package owns no prefab identity — Claims answers a bare
// yes/no — and asks it before generating any fabric for coord, per the
// design's own ordering.
//
// Precondition (unchecked here, and this package's own Cell is not the
// guarantor of it): a claimer must answer identically for every cell of
// one whole chunk, or of a whole group of chunks — never for part of
// one. The prefab layer implementing Claims is that guarantor; Cell
// asks about a single coordinate at a time and performs no verification
// of its own, since doing so would mean asking about every cell of a
// chunk on every call.
type PrefabClaimer interface {
	Claims(coord hexgrid.Coord) bool
}
