package maze

import "testing"

func TestDrawAlgorithm_RepeatEvaluationIsStable(t *testing.T) {
	t.Parallel()
	w := AlgorithmWeights{1, 3, 1, 5, 0}
	key := [32]byte{1, 2, 3}
	a := drawAlgorithm(newStream(key), w)
	b := drawAlgorithm(newStream(key), w)
	if a != b {
		t.Fatalf("drawAlgorithm not repeatable under the same key: %v != %v", a, b)
	}
}

func TestDrawAlgorithm_ZeroWeightNeverDrawn(t *testing.T) {
	t.Parallel()
	w := AlgorithmWeights{0, 3, 0, 5, 2}
	s := newStream([32]byte{9, 9})
	for i := 0; i < 10000; i++ {
		a := drawAlgorithm(s, w)
		if w[a] == 0 {
			t.Fatalf("drawAlgorithm returned %v, whose weight is zero", a)
		}
	}
}

func TestDrawAlgorithm_SpreadOfWeightsProducesMoreThanOneAlgorithm(t *testing.T) {
	t.Parallel()
	w := AlgorithmWeights{1, 1, 1, 1, 1}
	seen := map[Algorithm]bool{}
	for i := int64(0); i < 200; i++ {
		key := [32]byte{byte(i), byte(i >> 8)}
		seen[drawAlgorithm(newStream(key), w)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("equal weights over 200 keys produced only %d distinct algorithm(s)", len(seen))
	}
}

func TestDrawAlgorithm_WeightChangeChangesTheDraw(t *testing.T) {
	t.Parallel()
	key := [32]byte{42}
	a := drawAlgorithm(newStream(key), AlgorithmWeights{1, 0, 0, 0, 0})
	if a != AlgorithmBacktracker {
		t.Fatalf("single-weight backtracker draw = %v, want AlgorithmBacktracker", a)
	}
	b := drawAlgorithm(newStream(key), AlgorithmWeights{0, 0, 0, 0, 1})
	if b != AlgorithmWilson {
		t.Fatalf("single-weight wilson draw = %v, want AlgorithmWilson", b)
	}
}

func TestAlgorithmWeights_TotalWeight(t *testing.T) {
	t.Parallel()
	w := AlgorithmWeights{1, 2, 3, 4, 5}
	if got := w.totalWeight(); got != 15 {
		t.Errorf("totalWeight() = %d, want 15", got)
	}
}
