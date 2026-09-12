package maze

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

func validParams() Params {
	return Params{
		Dims:              hexgrid.Dims{Cols: 16, Rows: 16},
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

func TestParams_ValidateRejectsNonPositiveDims(t *testing.T) {
	t.Parallel()
	for _, d := range []hexgrid.Dims{{Cols: 0, Rows: 16}, {Cols: 16, Rows: 0}, {Cols: -1, Rows: 16}} {
		p := validParams()
		p.Dims = d
		if err := p.validate(); err == nil {
			t.Errorf("validate() with dims %+v = nil, want an error", d)
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

func TestParams_ValidateRejectsPositiveIslandShareAtDegenerateDims(t *testing.T) {
	t.Parallel()
	for _, d := range []hexgrid.Dims{{Cols: 1, Rows: 1}, {Cols: 1, Rows: 16}, {Cols: 16, Rows: 1}} {
		p := validParams()
		p.Dims = d
		p.IslandShare = decimal.New(1, -2)
		if err := p.validate(); err == nil {
			t.Errorf("validate() with dims %+v and a positive island share = nil, want an error", d)
		}
		p.IslandShare = decimal.Zero
		if err := p.validate(); err != nil {
			t.Errorf("validate() with dims %+v and a zero island share = %v, want nil", d, err)
		}
	}
}

func TestNonBorderCellCount_DegenerateShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d    hexgrid.Dims
		want int64
	}{
		{hexgrid.Dims{Cols: 1, Rows: 1}, 0},
		{hexgrid.Dims{Cols: 1, Rows: 16}, 0},
		{hexgrid.Dims{Cols: 16, Rows: 1}, 0},
		{hexgrid.Dims{Cols: 2, Rows: 16}, 0},
		{hexgrid.Dims{Cols: 16, Rows: 16}, 14 * 14},
	}
	for _, c := range cases {
		if got := nonBorderCellCount(c.d); got != c.want {
			t.Errorf("nonBorderCellCount(%+v) = %d, want %d", c.d, got, c.want)
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
