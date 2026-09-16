package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.yaml.in/yaml/v3"

	"github.com/maratik123/lab-game/internal/maze"
)

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

	radiusAtLeastMin := func(v int) bool { return v >= maze.MinRadius }
	radiusWant := fmt.Sprintf("at least %d", maze.MinRadius)

	return []schemaEntry{
		entry("world.chunk.radius", bindInt(&b.World.Chunk.Radius, radiusAtLeastMin, radiusWant)),

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
	if errs := walkNode(root, tree, nil, "balance"); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return b, nil
}
