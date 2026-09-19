package world

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/maze"
)

// ChunkType mirrors the database enum chunk_type: this package's own
// vocabulary, kept distinct from the generator's own type because a
// stored value must not move when a rendering does.
type ChunkType string

// ChunkType members, matching the database enum's declaration order.
const (
	ChunkTypeFabric ChunkType = "fabric"
	ChunkTypeGate   ChunkType = "gate"
)

// chunkTypes lists every ChunkType member, for a total mapping check
// driven from a table rather than trusted to the exhaustive linter,
// which treats a default branch as exhaustive on its own.
var chunkTypes = []ChunkType{ChunkTypeFabric, ChunkTypeGate}

// toMaze maps t onto the generator's own ChunkType, refusing a value
// this package does not know.
func (t ChunkType) toMaze() (maze.ChunkType, error) {
	switch t {
	case ChunkTypeFabric:
		return maze.ChunkTypeFabric, nil
	case ChunkTypeGate:
		return maze.ChunkTypeGate, nil
	default:
		return 0, fmt.Errorf("world: unknown chunk type %q", t)
	}
}

// chunkTypeFromMaze maps the generator's own ChunkType onto this
// package's vocabulary, refusing a value this package does not know.
func chunkTypeFromMaze(t maze.ChunkType) (ChunkType, error) {
	switch t {
	case maze.ChunkTypeFabric:
		return ChunkTypeFabric, nil
	case maze.ChunkTypeGate:
		return ChunkTypeGate, nil
	default:
		return "", fmt.Errorf("world: unknown generator chunk type %v", t)
	}
}

// CreationCause mirrors the database enum chunk_creation_cause: why a
// chunk was created — an explorer crossing into it, or a chat's
// activation.
type CreationCause string

// CreationCause members, matching the database enum's declaration order.
const (
	CreationCauseExplorer       CreationCause = "explorer"
	CreationCauseChatActivation CreationCause = "chat_activation"
)

// creationCauses lists every CreationCause member, for the same
// total-mapping reason chunkTypes exists.
var creationCauses = []CreationCause{CreationCauseExplorer, CreationCauseChatActivation}
