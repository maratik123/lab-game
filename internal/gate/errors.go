package gate

import "errors"

// ErrNegativeK is the error Next returns when k is negative. k is the
// minimum chunk distance a placed gate must keep from every other gate,
// and a negative distance names no constraint.
var ErrNegativeK = errors.New("gate: k must not be negative")

// ErrNegativeRadius is the error NewSet returns when the lattice it is
// given carries a negative radius. Set.Depth's stop-rule bound holds
// only for a nonnegative radius; with a negative radius the bound would
// shrink as the search widened, and the search would never terminate.
var ErrNegativeRadius = errors.New("gate: lattice radius must not be negative")
