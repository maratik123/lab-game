package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"go.yaml.in/yaml/v3"

	"github.com/maratik123/lab-game/internal/maze"
)

// World is one decoded world: the identity and generation inputs bound
// here, plus tone fields — the resource profile, the naming style, the
// lexicon and the bestiary — added as further fields on this same struct.
type World struct {
	// ID is the world's explicit, lower_snake_case identifier — the
	// canonical identity a maze row and every telemetry event carry,
	// never derived from the file name. Uniqueness across the world set
	// is the loader's concern, not this schema's.
	ID string
	// Seed is the generator's own seed argument for this world.
	Seed int64
	// GateSpacing is gate placement's own spacing argument: how close two
	// gates may sit, in hex distance.
	GateSpacing int
	// Generation carries the generator's own input parameters, decoded
	// directly onto them so the generator's own constructor stays the
	// sole source of every cross-field refusal.
	Generation maze.Params
}

// lowerSnakeCaseToken matches a token this format uses as authored
// identity or vocabulary: one or more lower_snake_case segments, each
// alphanumeric, joined by single underscores.
var lowerSnakeCaseToken = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

// bindLowerSnakeCaseString returns a binder that accepts only a "!!str"
// node matching lowerSnakeCaseToken, decodes it into dst.
func bindLowerSnakeCaseString(dst *string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != "!!str" {
			return fmt.Errorf("must be a string, got %s", n.Tag)
		}
		var v string
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode string: %w", err)
		}
		if !lowerSnakeCaseToken.MatchString(v) {
			return fmt.Errorf("must be a lower_snake_case token, got %q", v)
		}
		*dst = v
		return nil
	}
}

// bindInt32 returns a binder that accepts only a "!!int" node, decodes it
// directly into the int32 destination dst — never through a wider integer
// narrowed afterwards, so an out-of-range value is refused by the decode
// itself — and checks predicate.
func bindInt32(dst *int32, predicate func(int32) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != tagInt {
			return fmt.Errorf("must be an integer, got %s", n.Tag)
		}
		if err := n.Decode(dst); err != nil {
			return fmt.Errorf("decode int32: %w", err)
		}
		if !predicate(*dst) {
			return fmt.Errorf("must be %s, got %d", want, *dst)
		}
		return nil
	}
}

// bindInt64 returns a binder that accepts only a "!!int" node and decodes
// it directly into the int64 destination dst, checking predicate.
func bindInt64(dst *int64, predicate func(int64) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != tagInt {
			return fmt.Errorf("must be an integer, got %s", n.Tag)
		}
		if err := n.Decode(dst); err != nil {
			return fmt.Errorf("decode int64: %w", err)
		}
		if !predicate(*dst) {
			return fmt.Errorf("must be %s, got %d", want, *dst)
		}
		return nil
	}
}

// bindUint64 returns a binder that accepts only a "!!int" node and decodes
// it directly into the uint64 destination dst, checking predicate. Decoding
// directly into the uint64 destination is what refuses a negative value —
// go.yaml.in/yaml/v3 rejects "!!int -1" into a uint64 destination.
func bindUint64(dst *uint64, predicate func(uint64) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != tagInt {
			return fmt.Errorf("must be an integer, got %s", n.Tag)
		}
		if err := n.Decode(dst); err != nil {
			return fmt.Errorf("decode uint64: %w", err)
		}
		if !predicate(*dst) {
			return fmt.Errorf("must be %s, got %d", want, *dst)
		}
		return nil
	}
}

// worldScalarSchema returns the scalar half of one world document's
// key-path schema — id, seed, gate spacing and the generation inputs
// bound onto w.Generation — writing into w. Called fresh on every decode:
// no package-level state.
func worldScalarSchema(w *World) []schemaEntry {
	entry := func(path string, bind func(n *yaml.Node) error) schemaEntry {
		return schemaEntry{path: strings.Split(path, "."), bind: bind}
	}

	zero := decimal.Zero
	one := decimal.NewFromInt(1)
	unitFraction := func(v decimal.Decimal) bool { return v.GreaterThanOrEqual(zero) && v.LessThanOrEqual(one) }

	radiusAtLeastMin := func(v int32) bool { return v >= maze.MinRadius }
	radiusWant := fmt.Sprintf("at least %d", maze.MinRadius)
	anyInt64 := func(int64) bool { return true }
	nonNegativeInt := func(v int) bool { return v >= 0 }
	nonNegativeUint64 := func(uint64) bool { return true }

	entries := []schemaEntry{
		entry("id", bindLowerSnakeCaseString(&w.ID)),
		entry("seed", bindInt64(&w.Seed, anyInt64, "any integer")),
		entry("k", bindInt(&w.GateSpacing, nonNegativeInt, "non-negative")),

		entry("generation.radius", bindInt32(&w.Generation.Radius, radiusAtLeastMin, radiusWant)),
		entry("generation.island_share", bindDecimal(&w.Generation.IslandShare, unitFraction, "between 0 and 1 inclusive")),
		entry("generation.extra_passage_share", bindDecimal(&w.Generation.ExtraPassageShare, unitFraction, "between 0 and 1 inclusive")),
		entry("generation.growing_tree_bias", bindDecimal(&w.Generation.GrowingTreeBias, unitFraction, "between 0 and 1 inclusive")),
		entry("generation.portal_share_lower", bindDecimal(&w.Generation.PortalShareLower, unitFraction, "between 0 and 1 inclusive")),
		entry("generation.portal_share_upper", bindDecimal(&w.Generation.PortalShareUpper, unitFraction, "between 0 and 1 inclusive")),
	}

	for a := maze.AlgorithmBacktracker; a <= maze.AlgorithmWilson; a++ {
		entries = append(entries, entry("generation.weights."+a.String(), bindUint64(&w.Generation.Weights[a], nonNegativeUint64, "non-negative")))
	}

	return entries
}
