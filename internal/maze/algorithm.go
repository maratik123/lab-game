package maze

// Algorithm identifies one of the five spanning-structure algorithms a
// chunk's own weighted draw may choose. The zero value is
// AlgorithmBacktracker; the five values are in the canonical order
// every enum-indexed weight array and exhaustive switch over Algorithm
// relies on.
type Algorithm int8

// The five algorithms, in canonical order.
const (
	AlgorithmBacktracker Algorithm = iota
	AlgorithmKruskal
	AlgorithmPrim
	AlgorithmGrowingTree
	AlgorithmWilson
)

// String renders a using the spec's own lower_snake_case names — the
// same spelling a biome file's weight-set keys use — for logging and
// test failure messages.
func (a Algorithm) String() string {
	switch a {
	case AlgorithmBacktracker:
		return "backtracker"
	case AlgorithmKruskal:
		return "kruskal"
	case AlgorithmPrim:
		return "prim"
	case AlgorithmGrowingTree:
		return "growing_tree"
	case AlgorithmWilson:
		return "wilson_walk"
	default:
		return "unknown"
	}
}

// numAlgorithms is the width of the enum-indexed weight array — the
// count of Algorithm's own values, kept as a plain constant rather than
// a further Algorithm value so no switch over Algorithm ever has to
// treat it as a case to cover.
const numAlgorithms = 5

// AlgorithmWeights is the per-algorithm weight set a chunk's algorithm
// draw reads, indexed by Algorithm's own value. It is an array, not a
// map: an array has no iteration order to depend on and no
// unrepresentable key to reject — an unknown algorithm name in a biome
// file is the decoder's error, on the side of the boundary that knows
// names.
type AlgorithmWeights [numAlgorithms]uint64

// totalWeight sums every entry of w.
func (w AlgorithmWeights) totalWeight() uint64 {
	var total uint64
	for _, v := range w {
		total += v
	}
	return total
}

// drawAlgorithm draws the algorithm a chunk's spanning structure is
// built by, from s, weighted by w. A zero-weight algorithm is
// unreachable by construction: weightedPick never returns an index
// whose own weight is zero.
//
//nolint:gosec // G115: weightedPick(s, w[:]) returns an index in [0,len(w)), always < numAlgorithms, so the conversion never overflows int8
func drawAlgorithm(s stream, w AlgorithmWeights) Algorithm {
	return Algorithm(weightedPick(s, w[:]))
}

// FaceState is one face's state as Cell reports it: a wall, a passage,
// or — for a coordinate the prefab hook claims — deferred to the prefab
// layer. FaceDeferred is deliberately the zero value, so a claimed
// cell's interior face is honest rather than silently a wall.
type FaceState int8

// The three face states.
const (
	FaceDeferred FaceState = iota
	FaceWall
	FacePassage
)
