package maze

import (
	"strconv"
	"strings"
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
		PortalShareLower:  decimal.New(1, -1),
		PortalShareUpper:  decimal.New(2, -1),
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
		err := p.validate()
		if err == nil {
			t.Errorf("validate() with radius %d = nil, want an error", r)
			continue
		}
		if got := strconv.Itoa(int(r)); !strings.Contains(err.Error(), got) {
			t.Errorf("validate() with radius %d = %q, want it to name the radius %s", r, err, got)
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

// wantErrNaming fails t unless err is non-nil and its text contains
// name — the assertion that a refusal names its offending input, not
// only that validate refused.
func wantErrNaming(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Errorf("validate() = nil, want an error naming %q", name)
		return
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("validate() = %q, want it to name %q", err, name)
	}
}

func TestParams_ValidateRejectsShareOutOfRange(t *testing.T) {
	t.Parallel()
	for _, share := range []decimal.Decimal{decimal.NewFromInt(-1), decimal.NewFromInt(2)} {
		p := validParams()
		p.IslandShare = share
		wantErrNaming(t, p.validate(), "island share")

		p2 := validParams()
		p2.ExtraPassageShare = share
		wantErrNaming(t, p2.validate(), "extra-passage share")

		p3 := validParams()
		p3.GrowingTreeBias = share
		wantErrNaming(t, p3.validate(), "growing-tree bias")
	}
}

func TestParams_ValidateRejectsPortalShareOutOfRange(t *testing.T) {
	t.Parallel()
	for _, share := range []decimal.Decimal{decimal.NewFromInt(-1), decimal.NewFromInt(2)} {
		p := validParams()
		p.PortalShareLower = share
		wantErrNaming(t, p.validate(), "portal share lower bound")

		p2 := validParams()
		p2.PortalShareUpper = share
		wantErrNaming(t, p2.validate(), "portal share upper bound")
	}
}

func TestParams_ValidateRejectsPortalShareLowerAboveUpper(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.PortalShareLower = decimal.New(3, -1)
	p.PortalShareUpper = decimal.New(2, -1)
	wantErrNaming(t, p.validate(), "portal share lower bound")
}

func TestParams_ValidateRejectsPortalShareLowerRoundingToZero(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius // border length 13
	p.PortalShareLower = decimal.Zero
	p.PortalShareUpper = decimal.New(2, -1)
	wantErrNaming(t, p.validate(), "portal share lower bound")
}

func TestParams_ValidateRejectsPortalShareUpperPastRPlusOne(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius                       // border length 13, R+1 = 7 non-touching positions max
	p.PortalShareUpper = decimal.NewFromInt(1) // rounds to 13, far past 7
	wantErrNaming(t, p.validate(), "portal share upper bound")
}

// TestParams_ValidatePortalShareUpperAtAndPastTheBound checks the exact
// edge of the R+1 non-touching-position bound: hi == R+1 is accepted,
// hi == R+2 is refused naming the portal share upper bound.
func TestParams_ValidatePortalShareUpperAtAndPastTheBound(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius
	borderLength := 2*int(p.Radius) + 1

	// A share whose ceiling gives exactly hi = R+1: 0.5*13 = 6.5, ceil 7.
	p.PortalShareUpper = decimal.New(5, -1)
	if _, hi := portalBounds(p.PortalShareLower, p.PortalShareUpper, borderLength); hi != int(p.Radius)+1 {
		t.Fatalf("test setup: hi = %d, want %d", hi, p.Radius+1)
	}
	if err := p.validate(); err != nil {
		t.Errorf("validate() with portal share upper bound at hi=R+1 = %v, want nil", err)
	}

	// A share whose ceiling gives exactly hi = R+2: 0.6*13 = 7.8, ceil 8.
	p.PortalShareUpper = decimal.New(6, -1)
	if _, hi := portalBounds(p.PortalShareLower, p.PortalShareUpper, borderLength); hi != int(p.Radius)+2 {
		t.Fatalf("test setup: hi = %d, want %d", hi, p.Radius+2)
	}
	wantErrNaming(t, p.validate(), "portal share upper bound")
}

func TestPortalBounds_RoundsUpWithCeiling(t *testing.T) {
	t.Parallel()
	// L=13 (radius 6): lower 0.1 -> ceil(1.3)=2, upper 0.2 -> ceil(2.6)=3.
	lo, hi := portalBounds(decimal.New(1, -1), decimal.New(2, -1), 13)
	if lo != 2 || hi != 3 {
		t.Errorf("portalBounds(0.1,0.2,13) = (%d,%d), want (2,3)", lo, hi)
	}
}

func TestParams_ValidateRejectsIslandShareAboveCapacity(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius
	p.IslandShare = decimal.NewFromInt(1) // every cell — far more than the gate/fabric capacity
	wantErrNaming(t, p.validate(), "island share")
}

// TestParams_ValidateAcceptsIslandShareAtGateCapacity checks the other
// side of the same bound: a target that lands exactly on the gate
// chunk's capacity is accepted, not just refused above it.
func TestParams_ValidateAcceptsIslandShareAtGateCapacity(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius
	capacity := gateCapacity(p.Radius)
	p.IslandShare = shareForIslandTarget(capacity, p.lattice().CellCount())
	if got := islandTarget(p); got != capacity {
		t.Fatalf("test setup: islandTarget = %d, want the gate capacity %d", got, capacity)
	}
	if err := p.validate(); err != nil {
		t.Errorf("validate() with island share at the gate capacity = %v, want nil", err)
	}
}

// TestParams_ValidateRejectsIslandShareOneAboveGateCapacity checks the
// bound at the gate capacity's own edge: a target one above gateCapacity
// is refused naming the island share, at a radius where the gate
// capacity (nonBorderCellCount minus the centre) differs from the plain
// non-border cell count a fabric chunk would allow.
func TestParams_ValidateRejectsIslandShareOneAboveGateCapacity(t *testing.T) {
	t.Parallel()
	p := validParams()
	p.Radius = MinRadius
	capacity := gateCapacity(p.Radius)
	fabricCapacity := nonBorderCellCount(p.Radius)
	if capacity == fabricCapacity {
		t.Fatalf("test setup: gate capacity %d equals the fabric capacity %d, want them to differ", capacity, fabricCapacity)
	}
	p.IslandShare = shareForIslandTarget(capacity+1, p.lattice().CellCount())
	if got := islandTarget(p); got != capacity+1 {
		t.Fatalf("test setup: islandTarget = %d, want %d", got, capacity+1)
	}
	wantErrNaming(t, p.validate(), "island share")
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
