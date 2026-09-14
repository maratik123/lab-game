package maze

// ChunkType selects which fabric rule Generate builds a chunk's cells
// under. The zero value is not a type — Generate refuses it, along with
// any value this package does not know, naming the offending value.
type ChunkType int8

// The two chunk types. ChunkTypeFabric and ChunkTypeGate are both
// nonzero so a caller that forgot to set a ChunkType field is refused
// rather than silently generating fabric.
const (
	chunkTypeUnset ChunkType = iota
	ChunkTypeFabric
	ChunkTypeGate
)

// String renders t for logging and test failure messages.
func (t ChunkType) String() string {
	switch t {
	case ChunkTypeFabric:
		return "fabric"
	case ChunkTypeGate:
		return "gate"
	default:
		return "unknown"
	}
}

// valid reports whether t is one of this package's known nonzero chunk
// types.
func (t ChunkType) valid() bool {
	switch t {
	case ChunkTypeFabric, ChunkTypeGate:
		return true
	default:
		return false
	}
}
