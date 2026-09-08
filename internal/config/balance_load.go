package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.yaml.in/yaml/v3"
)

// schemaEntry is one leaf of the balance schema: a dotted key path and a
// binder that validates the node's YAML tag, decodes it into the project's
// Go destination, and checks the project's own predicate for that key.
type schemaEntry struct {
	path []string
	bind func(n *yaml.Node) error
}

// schemaNode is one level of the tree built from the flat schemaEntry list
// (buildSchemaTree), so the walk can match it against the document's
// mapping nodes segment by segment. order preserves the schema's own
// declaration order, since map iteration order is never used for anything
// observable.
type schemaNode struct {
	entry    *schemaEntry
	children map[string]*schemaNode
	order    []string
}

// bindInt returns a binder that accepts only a "!!int" node (rejecting a
// "!!null" node and a truncating "!!float" node alike), decodes it into
// dst, and checks predicate.
//
//nolint:unparam // want is always "positive" today because every int-typed key in the schema happens to be positive; kept symmetric with bindDuration/bindDecimal for the next int key that isn't
func bindInt(dst *int, predicate func(int) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != "!!int" {
			return fmt.Errorf("must be an integer, got %s", n.Tag)
		}
		var v int
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode int: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %d", want, v)
		}
		*dst = v
		return nil
	}
}

// bindDuration returns a binder that accepts only a "!!str" node — a bare
// "!!int" is rejected rather than silently becoming nanoseconds — and
// checks predicate on the decoded value.
func bindDuration(dst *time.Duration, predicate func(time.Duration) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != "!!str" {
			return fmt.Errorf("must be a duration string, got %s", n.Tag)
		}
		var v time.Duration
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode duration: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %s", want, v)
		}
		*dst = v
		return nil
	}
}

// bindDecimal returns a binder that accepts a "!!int" or "!!float" node —
// rejecting a quoted "!!str" number, so every number has one spelling —
// and checks predicate on the decoded value.
func bindDecimal(dst *decimal.Decimal, predicate func(decimal.Decimal) bool, want string) func(*yaml.Node) error {
	return func(n *yaml.Node) error {
		if n.Tag != "!!int" && n.Tag != "!!float" {
			return fmt.Errorf("must be a number, got %s", n.Tag)
		}
		var v decimal.Decimal
		if err := n.Decode(&v); err != nil {
			return fmt.Errorf("decode decimal: %w", err)
		}
		if !predicate(v) {
			return fmt.Errorf("must be %s, got %s", want, v)
		}
		*dst = v
		return nil
	}
}

// balanceSchema returns the balance file's key-path schema — path,
// destination and predicate for every configurable balance key — writing
// into b. Called fresh on every load: no package-level state.
func balanceSchema(b *Balance) []schemaEntry { //nolint:funlen // one row per balance key; splitting loses the at-a-glance schema shape
	zero := decimal.Zero
	one := decimal.NewFromInt(1)
	entry := func(path string, bind func(n *yaml.Node) error) schemaEntry {
		return schemaEntry{path: strings.Split(path, "."), bind: bind}
	}
	positiveInt := func(v int) bool { return v > 0 }
	positiveDuration := func(v time.Duration) bool { return v > 0 }
	nonNegativeDuration := func(v time.Duration) bool { return v >= 0 }
	positiveDecimal := func(v decimal.Decimal) bool { return v.GreaterThan(zero) }
	unitFraction := func(v decimal.Decimal) bool { return v.GreaterThanOrEqual(zero) && v.LessThanOrEqual(one) }
	openUnitFraction := func(v decimal.Decimal) bool { return v.GreaterThan(zero) && v.LessThan(one) }
	halfOpenUnitFraction := func(v decimal.Decimal) bool { return v.GreaterThan(zero) && v.LessThanOrEqual(one) }

	return []schemaEntry{
		entry("world.chunk.cols", bindInt(&b.World.Chunk.Cols, positiveInt, "positive")),
		entry("world.chunk.rows", bindInt(&b.World.Chunk.Rows, positiveInt, "positive")),

		entry("raid.stamina.cap", bindDecimal(&b.Raid.Stamina.Cap, positiveDecimal, "positive")),
		entry("raid.stamina.step_cost", bindDecimal(&b.Raid.Stamina.StepCost, positiveDecimal, "positive")),

		entry("raid.standing.calm_to_noises", bindDuration(&b.Raid.Standing.CalmToNoises, positiveDuration, "positive")),
		entry("raid.standing.noises_to_wave", bindDuration(&b.Raid.Standing.NoisesToWave, positiveDuration, "positive")),
		entry("raid.standing.wave_to_wave", bindDuration(&b.Raid.Standing.WaveToWave, positiveDuration, "positive")),

		entry("raid.death.backpack_ttl", bindDuration(&b.Raid.Death.BackpackTTL, positiveDuration, "positive")),
		entry("raid.death.own_chat_head_start", bindDuration(&b.Raid.Death.OwnChatHeadStart, positiveDuration, "positive")),
		entry("raid.death.respawn_debuff", bindDuration(&b.Raid.Death.RespawnDebuff, nonNegativeDuration, "non-negative")),

		entry("raid.afk.cruelty", bindDecimal(&b.Raid.AFK.Cruelty, unitFraction, "between 0 and 1 inclusive")),

		entry("raid.door.price_base", bindDecimal(&b.Raid.Door.PriceBase, positiveDecimal, "positive")),
		entry("raid.door.price_per_distance", bindDecimal(&b.Raid.Door.PricePerDistance, positiveDecimal, "positive")),
		entry("raid.door.price_distance_exponent", bindDecimal(&b.Raid.Door.PriceDistanceExponent, positiveDecimal, "positive")),
		entry("raid.door.boss_reward_ttl", bindDuration(&b.Raid.Door.BossRewardTTL, positiveDuration, "positive")),

		entry("raid.monster_budget.base", bindDecimal(&b.Raid.MonsterBudget.Base, positiveDecimal, "positive")),
		entry("raid.monster_budget.per_distance", bindDecimal(&b.Raid.MonsterBudget.PerDistance, positiveDecimal, "positive")),
		entry("raid.monster_budget.distance_exponent", bindDecimal(&b.Raid.MonsterBudget.DistanceExponent, halfOpenUnitFraction, "greater than 0 and at most 1")),
		entry("raid.monster_budget.scaling_per_level", bindDecimal(&b.Raid.MonsterBudget.ScalingPerLevel, positiveDecimal, "positive")),

		entry("combat.hit_die_sides", bindInt(&b.Combat.HitDieSides, positiveInt, "positive")),
		entry("combat.base_defence", bindInt(&b.Combat.BaseDefence, positiveInt, "positive")),
		entry("combat.critical_natural", bindInt(&b.Combat.CriticalNatural, positiveInt, "positive")),
		entry("combat.fumble_natural", bindInt(&b.Combat.FumbleNatural, positiveInt, "positive")),
		entry("combat.weapon_dice_count", bindInt(&b.Combat.WeaponDiceCount, positiveInt, "positive")),
		entry("combat.weapon_die_sides", bindInt(&b.Combat.WeaponDieSides, positiveInt, "positive")),
		entry("combat.initiative_die_sides", bindInt(&b.Combat.InitiativeDieSides, positiveInt, "positive")),
		entry("combat.max_rounds", bindInt(&b.Combat.MaxRounds, positiveInt, "positive")),
		entry("combat.vulnerability_multiplier", bindDecimal(&b.Combat.VulnerabilityMultiplier, func(v decimal.Decimal) bool {
			return v.GreaterThan(one)
		}, "greater than 1")),
		entry("combat.resist_multiplier", bindDecimal(&b.Combat.ResistMultiplier, openUnitFraction, "between 0 and 1 exclusive")),

		entry("economy.shop.sell_rate", bindDecimal(&b.Economy.Shop.SellRate, openUnitFraction, "between 0 and 1 exclusive")),
		entry("economy.shop.buy_markup", bindDecimal(&b.Economy.Shop.BuyMarkup, positiveDecimal, "positive")),
	}
}

// buildSchemaTree groups the flat schema entry list into a tree keyed by
// path segment, so walkNode can match it against the document's mapping
// nodes one segment at a time.
func buildSchemaTree(entries []schemaEntry) *schemaNode {
	root := &schemaNode{children: map[string]*schemaNode{}}
	for i := range entries {
		e := &entries[i]
		cur := root
		for _, seg := range e.path[:len(e.path)-1] {
			next, ok := cur.children[seg]
			if !ok {
				next = &schemaNode{children: map[string]*schemaNode{}}
				cur.children[seg] = next
				cur.order = append(cur.order, seg)
			}
			cur = next
		}
		last := e.path[len(e.path)-1]
		cur.children[last] = &schemaNode{entry: e}
		cur.order = append(cur.order, last)
	}
	return root
}

// pathKey renders a schema path for a *KeyError. An empty path (the
// document root itself) renders as "<root>" rather than the empty string,
// so a root-level failure still names something in its message.
func pathKey(path []string) string {
	if len(path) == 0 {
		return "<root>"
	}
	return strings.Join(path, ".")
}

// nodeKindName renders a yaml.Kind for an error message.
func nodeKindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

// walkNode dispatches to walkLeaf or walkInterior depending on whether node
// is a schema leaf or an interior (mapping) level.
func walkNode(doc *yaml.Node, node *schemaNode, path []string) []error {
	if node.entry != nil {
		return walkLeaf(doc, node.entry)
	}
	return walkInterior(doc, node, path)
}

// walkLeaf validates and binds one scalar schema entry. doc is nil when the
// key is absent at this path.
func walkLeaf(doc *yaml.Node, e *schemaEntry) []error {
	key := pathKey(e.path)
	if doc == nil {
		return []error{keyErrorf(key, ErrMissing, "required")}
	}
	switch doc.Kind {
	case yaml.AliasNode:
		return []error{keyErrorf(key, ErrInvalidValue, "must not be an alias")}
	case yaml.MappingNode, yaml.SequenceNode:
		return []error{keyErrorf(key, ErrInvalidValue, "must be a scalar, got %s", nodeKindName(doc.Kind))}
	default:
		if err := e.bind(doc); err != nil {
			return []error{keyErrorf(key, ErrInvalidValue, "%s", err)}
		}
		return nil
	}
}

// walkInterior validates one mapping-level schema node against doc (nil when
// the whole subtree is absent, in which case every leaf beneath it reports
// missing — this is how an "everything missing" report falls out of the
// same code path as a normal partial mapping), reports every key doc
// carries that the schema does not declare, and recurses into every schema
// child in declaration order.
func walkInterior(doc *yaml.Node, node *schemaNode, path []string) []error {
	var errs []error
	if doc != nil {
		switch doc.Kind {
		case yaml.AliasNode:
			return []error{keyErrorf(pathKey(path), ErrInvalidValue, "must not be an alias")}
		case yaml.MappingNode:
			// fall through to the walk below
		default:
			return []error{keyErrorf(pathKey(path), ErrInvalidValue, "must be a mapping, got %s", nodeKindName(doc.Kind))}
		}
	}

	valuesByKey := map[string]*yaml.Node{}
	if doc != nil {
		for i := 0; i+1 < len(doc.Content); i += 2 {
			k := doc.Content[i].Value
			valuesByKey[k] = doc.Content[i+1]
			if _, known := node.children[k]; !known {
				childPath := append(append([]string{}, path...), k)
				errs = append(errs, keyErrorf(pathKey(childPath), ErrUnknownKey, "not a recognised balance key"))
			}
		}
	}

	for _, k := range node.order {
		child := node.children[k]
		childPath := append(append([]string{}, path...), k)
		errs = append(errs, walkNode(valuesByKey[k], child, childPath)...)
	}
	return errs
}

// checkDuplicateKeys unmarshals data into a map, which — unlike unmarshalling
// into a yaml.Node — reports a duplicated mapping key at any depth. A
// duplicate is returned as the library's own error, unmodified; every other
// unmarshal failure (a syntax error, a non-mapping root) is left for the
// node-based parse to report with better context, so it is deliberately
// swallowed here.
func checkDuplicateKeys(data []byte) error {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil && strings.Contains(err.Error(), "already defined") {
		return fmt.Errorf("config: duplicate key: %w", err)
	}
	return nil
}

// loadBalance reads, parses and validates the balance file at path against
// balanceSchema, returning a populated Balance or a joined error naming
// every rejected or missing key.
func loadBalance(path string) (*Balance, error) {
	//nolint:gosec // G304: path is the operator-supplied LAB_GAME_BALANCE_PATH value — reading it is the feature.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: balance file %s: %w", path, err)
	}
	if err := checkDuplicateKeys(data); err != nil {
		return nil, fmt.Errorf("config: balance file %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config: balance file %s: parse: %w", path, err)
	}

	var root *yaml.Node
	if !doc.IsZero() && len(doc.Content) > 0 {
		root = doc.Content[0]
	}

	b := &Balance{}
	tree := buildSchemaTree(balanceSchema(b))
	if errs := walkNode(root, tree, nil); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return b, nil
}
