package maze

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
)

// MinRadius is the lowest chunk radius this package generates for: the
// MVP's own lower bound. There is no upper bound — a radius this package
// makes no promise for exhausts memory long before any coordinate
// arithmetic would overflow.
const MinRadius = 6

// Params carries every input a Generator's chunk build reads: the
// chunk's radius, the per-algorithm weights, the island share, the
// extra-passage share, the growing-tree bias, and the portal share
// bounds. It carries no seed — the seed is New's own argument — and
// fixes no configuration key name; mapping a biome file onto Params is
// the composition root's concern.
type Params struct {
	Radius            int32
	Weights           AlgorithmWeights
	IslandShare       decimal.Decimal
	ExtraPassageShare decimal.Decimal
	GrowingTreeBias   decimal.Decimal
	PortalShareLower  decimal.Decimal
	PortalShareUpper  decimal.Decimal
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
// clamped share would go on to under-deliver against the achieved share
// its caller asked for, and report success while doing it.
func (p Params) validate() error {
	if p.Radius < MinRadius {
		return fmt.Errorf("maze: radius must be at least %d, got %d", MinRadius, p.Radius)
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
	if err := validateShare("portal share lower bound", p.PortalShareLower); err != nil {
		return err
	}
	if err := validateShare("portal share upper bound", p.PortalShareUpper); err != nil {
		return err
	}
	if p.PortalShareLower.GreaterThan(p.PortalShareUpper) {
		return fmt.Errorf("maze: portal share lower bound %s is above the upper bound %s", p.PortalShareLower, p.PortalShareUpper)
	}
	borderLength := 2*int(p.Radius) + 1
	lo, hi := portalBounds(p.PortalShareLower, p.PortalShareUpper, borderLength)
	if lo < 1 {
		return fmt.Errorf("maze: portal share lower bound %s rounds to %d guaranteed portals on a border of %d faces, but a border needs at least one", p.PortalShareLower, lo, borderLength)
	}
	if hi > int(p.Radius)+1 {
		return fmt.Errorf("maze: portal share upper bound %s rounds to %d guaranteed portals on a border of %d faces, more non-touching positions than a path of that length has (%d)", p.PortalShareUpper, hi, borderLength, int(p.Radius)+1)
	}
	// The gate chunk's capacity is the binding constraint: it excludes
	// the centre cell on top of every border cell, so an island share
	// valid against it is valid for a fabric chunk too.
	capacity := gateCapacity(p.Radius)
	if capacity == 0 && p.IslandShare.IsPositive() {
		return fmt.Errorf("maze: island share %s is positive but radius %d has no non-border, non-centre cell to draw islands from", p.IslandShare, p.Radius)
	}
	if target := islandTarget(p); target > capacity {
		return fmt.Errorf("maze: island share %s rounds to %d islands, more than radius %d can hold in a gate chunk (%d cells, excluding the border and the centre)", p.IslandShare, target, p.Radius, capacity)
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

// nonBorderCellCount returns how many of a radius-r chunk's cells have
// every one of their six neighbours inside the same chunk — the
// capacity the island share's rounded count is drawn from: the cell
// count of the radius-(r-1) hexagon centred on the same point.
func nonBorderCellCount(radius int32) int64 {
	r := int64(radius) - 1
	if r < 0 {
		return 0
	}
	return 3*r*r + 3*r + 1
}

// gateCapacity returns how many candidate cells a gate chunk of radius r
// offers to the island draw: nonBorderCellCount(r), less the one centre
// cell a gate chunk also excludes — 3r²−3r.
func gateCapacity(radius int32) int64 {
	capacity := nonBorderCellCount(radius) - 1
	if capacity < 0 {
		return 0
	}
	return capacity
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

// lattice returns the Lattice this Params generates a chunk over.
func (p Params) lattice() hexgrid.Lattice {
	return hexgrid.Lattice{Radius: p.Radius}
}
