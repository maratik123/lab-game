package maze

import (
	"testing"

	"github.com/shopspring/decimal"
)

func validParams() Params {
	return Params{
		Radius:            MinRadius,
		Weights:           AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   half,
	}
}

func TestParams_ValidateAcceptsAValidInput(t *testing.T) {
	t.Parallel()
	if err := validParams().validate(); err != nil {
		t.Errorf("validate() = %v, want nil", err)
	}
}

func TestParams_ValidateRadiusBound(t *testing.T) {
	t.Parallel()
	for _, r := range []int32{5, 0, -1} {
		p := validParams()
		p.Radius = r
		if err := p.validate(); err == nil {
			t.Errorf("validate() with radius %d = nil, want an error", r)
		}
	}
	for _, r := range []int32{6, 9, 40} {
		p := validParams()
		p.Radius = r
		if err := p.validate(); err != nil {
			t.Errorf("validate() with radius %d = %v, want nil", r, err)
		}
	}
}

func TestParams_ValidateRejectsAllZeroWeights(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Weights = AlgorithmWeights{}
	if err := p.validate(); err == nil {
		t.Error("validate() with all-zero weights = nil, want an error")
	}
}

func TestParams_ValidateRejectsShareOutOfRange(t *testing.T) {
	t.Parallel()
	for _, share := range []decimal.Decimal{decimal.NewFromInt(-1), decimal.NewFromInt(2)} {
		p := validParams()
		p.IslandShare = share
		if err := p.validate(); err == nil {
			t.Errorf("validate() with island share %s = nil, want an error", share)
		}
		p2 := validParams()
		p2.ExtraPassageShare = share
		if err := p2.validate(); err == nil {
			t.Errorf("validate() with extra-passage share %s = nil, want an error", share)
		}
		p3 := validParams()
		p3.GrowingTreeBias = share
		if err := p3.validate(); err == nil {
			t.Errorf("validate() with growing-tree bias %s = nil, want an error", share)
		}
	}
}

func TestParams_ValidateRejectsIslandShareAboveCapacity(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius
	p.IslandShare = decimal.NewFromInt(1) // every cell — far more than the gate/fabric capacity
	if err := p.validate(); err == nil {
		t.Error("validate() with island share 1 at the minimum radius = nil, want an error")
	}
}

func TestNonBorderCellCount_MatchesTheRadiusMinusOneHexagon(t *testing.T) {
	t.Parallel()
	cases := []struct {
		radius int32
		want   int64
	}{
		{6, 91},  // radius-5 hexagon: 3*25+15+1
		{7, 127}, // radius-6 hexagon: 3*36+18+1
		{9, 217}, // radius-8 hexagon: 3*64+24+1
	}
	for _, c := range cases {
		if got := nonBorderCellCount(c.radius); got != c.want {
			t.Errorf("nonBorderCellCount(%d) = %d, want %d", c.radius, got, c.want)
		}
	}
}

func TestRoundHalfUp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		x    decimal.Decimal
		want int64
	}{
		{decimal.New(5, -2).Mul(decimal.NewFromInt(256)), 13}, // 0.05*256=12.8 -> 13
		{decimal.Zero, 0},
		{decimal.New(15, -1), 2}, // 1.5 -> 2 (half rounds up under this rule)
		{decimal.New(25, -1), 3}, // 2.5 -> 3
	}
	for _, c := range cases {
		if got := roundHalfUp(c.x); got != c.want {
			t.Errorf("roundHalfUp(%s) = %d, want %d", c.x, got, c.want)
		}
	}
}

func TestBiasThreshold_ExtremesAndMonotone(t *testing.T) {
	t.Parallel()
	if got := biasThreshold(decimal.Zero); got != 0 {
		t.Errorf("biasThreshold(0) = %d, want 0", got)
	}
	if got := biasThreshold(decimal.NewFromInt(1)); got != 1<<32 {
		t.Errorf("biasThreshold(1) = %d, want 2^32", got)
	}
	lo := biasThreshold(decimal.New(25, -2)) // 0.25
	hi := biasThreshold(decimal.New(75, -2)) // 0.75
	if lo >= hi {
		t.Errorf("biasThreshold not monotone: biasThreshold(0.25)=%d >= biasThreshold(0.75)=%d", lo, hi)
	}
}
