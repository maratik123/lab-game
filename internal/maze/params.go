package maze

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Params carries every input a Generator's chunk build reads: the chunk
// grid's dimensions, the per-algorithm weights, the island share, the
// extra-passage share, and the growing-tree bias. It carries no seed —
// the seed is New's own argument — and fixes no configuration key name;
// mapping a biome file onto Params is the composition root's concern.
type Params struct {
	Dims              hexgrid.Dims
	Weights           AlgorithmWeights
	IslandShare       decimal.Decimal
	ExtraPassageShare decimal.Decimal
	GrowingTreeBias   decimal.Decimal
}

// zeroShare and oneShare bound every share and the bias to [0,1],
// built without a floating-point literal reaching decimal's float
// constructors.
var (
	zeroShare = decimal.Zero
	oneShare  = decimal.NewFromInt(1)
	half      = decimal.New(5, -1)
	twoPow32  = decimal.NewFromInt(1 << 32)
)

// validate checks p against every precondition New rejects a bad Params
// for, naming the offending input. Rejecting beats clamping: a silently
// clamped share would under-deliver against the very criterion that
// asks the achieved share to match the input.
func (p Params) validate() error {
	if p.Dims.Cols <= 0 || p.Dims.Rows <= 0 {
		return fmt.Errorf("maze: dims must be positive, got %+v", p.Dims)
	}
	if p.Weights.totalWeight() == 0 {
		return errors.New("maze: weights has no positive entry")
	}
	if err := validateShare("island share", p.IslandShare); err != nil {
		return err
	}
	if err := validateShare("extra-passage share", p.ExtraPassageShare); err != nil {
		return err
	}
	if err := validateShare("growing-tree bias", p.GrowingTreeBias); err != nil {
		return err
	}
	if nonBorderCellCount(p.Dims) == 0 && p.IslandShare.IsPositive() {
		return fmt.Errorf("maze: island share %s is positive but dims %+v have no non-border cell to draw islands from", p.IslandShare, p.Dims)
	}
	return nil
}

// validateShare rejects v when it lies outside [0,1] — a share above
// one is a units mistake, not a value to clamp.
func validateShare(name string, v decimal.Decimal) error {
	if v.LessThan(zeroShare) || v.GreaterThan(oneShare) {
		return fmt.Errorf("maze: %s must be within [0,1], got %s", name, v)
	}
	return nil
}

// nonBorderCellCount returns how many of a d-sized chunk's cells have
// every one of their six neighbours inside the same chunk — the
// capacity the island share's rounded count is drawn from. A 1x1,
// single-row, or single-column chunk has none.
func nonBorderCellCount(d hexgrid.Dims) int64 {
	interiorCols := int64(d.Cols) - 2
	interiorRows := int64(d.Rows) - 2
	if interiorCols < 0 {
		interiorCols = 0
	}
	if interiorRows < 0 {
		interiorRows = 0
	}
	return interiorCols * interiorRows
}

// roundHalfUp implements this package's pinned rounding: floor(x + ½).
// An exact-half input resolves consistently under this rule, where
// half-away-from-zero and half-to-even would disagree.
func roundHalfUp(x decimal.Decimal) int64 {
	return x.Add(half).Floor().IntPart()
}

// biasThreshold turns a growing-tree bias in [0,1] into the pinned
// threshold biasedNewest compares against: floor(bias × 2^32), so a
// bias of zero never selects the newest active-list entry and a bias
// of one always does, with no overflow branch and no floating-point
// comparison anywhere on the path.
//
//nolint:gosec // G115: bias is validated to [0,1], so bias*2^32 never exceeds 2^32 and always fits uint64
func biasThreshold(bias decimal.Decimal) uint64 {
	return uint64(bias.Mul(twoPow32).Floor().IntPart())
}
