package config

import (
	"time"

	"github.com/shopspring/decimal"
)

// Balance holds every game constant, nested to mirror the balance file's
// YAML tree: a dotted key path such as "raid.stamina.cap" in an error
// message reads as the same path as cfg.Balance.Raid.Stamina.Cap at a call
// site. No field has a compiled-in fallback — every value is read from the
// balance file named by LAB_GAME_BALANCE_PATH, or Load fails naming the
// missing key.
type Balance struct {
	World   WorldBalance
	Raid    RaidBalance
	Combat  CombatBalance
	Economy EconomyBalance
}

// WorldBalance holds world-generation constants (docs/DESIGN.md §2.2.2).
type WorldBalance struct {
	Chunk ChunkBalance
}

// ChunkBalance is the hex-chunk grid size (docs/DESIGN.md §2.2.2).
type ChunkBalance struct {
	Cols int
	Rows int
}

// RaidBalance holds every raid-mechanic constant.
type RaidBalance struct {
	Stamina       StaminaBalance
	Standing      StandingBalance
	Death         DeathBalance
	AFK           AFKBalance
	Door          DoorBalance
	MonsterBudget MonsterBudgetBalance
}

// StaminaBalance is the stamina cap and per-step cost (docs/DESIGN.md §3.3).
type StaminaBalance struct {
	Cap      decimal.Decimal
	StepCost decimal.Decimal
}

// StandingBalance times the noise-standing escalation (docs/DESIGN.md §3.5).
type StandingBalance struct {
	CalmToNoises time.Duration
	NoisesToWave time.Duration
	WaveToWave   time.Duration
}

// DeathBalance times backpack expiry, the owning chat's head start, and the
// respawn debuff (docs/DESIGN.md §3.4).
type DeathBalance struct {
	BackpackTTL      time.Duration
	OwnChatHeadStart time.Duration
	RespawnDebuff    time.Duration
}

// AFKBalance is the AFK-leader cruelty dial (docs/DESIGN.md §3.5). Its
// shape — not only its number — is a placeholder; see the design's open
// questions.
type AFKBalance struct {
	Cruelty decimal.Decimal
}

// DoorBalance is the crafted-door price curve and the boss-reward door TTL
// (docs/DESIGN.md §2.2.3, §2.3). PriceBase, PricePerDistance and
// PriceDistanceExponent are a key triple whose combining formula is not
// fixed by this package — see the comment above the corresponding keys in
// config/balance.yaml.
type DoorBalance struct {
	PriceBase             decimal.Decimal
	PricePerDistance      decimal.Decimal
	PriceDistanceExponent decimal.Decimal
	BossRewardTTL         time.Duration
}

// MonsterBudgetBalance is the monster budget-by-distance curve
// (docs/DESIGN.md §4.6). Base, PerDistance and DistanceExponent are a key
// triple whose combining formula is not fixed by this package — see the
// comment above the corresponding keys in config/balance.yaml.
type MonsterBudgetBalance struct {
	Base             decimal.Decimal
	PerDistance      decimal.Decimal
	DistanceExponent decimal.Decimal
	ScalingPerLevel  decimal.Decimal
}

// CombatBalance holds the combat dice and multipliers (docs/DESIGN.md §4).
// The combat-system version is deliberately not here: it identifies the
// code that produced a stored log, so it lives as a constant in the combat
// package, not as an operator-tunable value.
type CombatBalance struct {
	HitDieSides             int
	BaseDefence             int
	CriticalNatural         int
	FumbleNatural           int
	WeaponDiceCount         int
	WeaponDieSides          int
	InitiativeDieSides      int
	MaxRounds               int
	VulnerabilityMultiplier decimal.Decimal
	ResistMultiplier        decimal.Decimal
}

// EconomyBalance holds the shop's economy constants.
type EconomyBalance struct {
	Shop ShopBalance
}

// ShopBalance is the shop's sell rate and buy markup (docs/DESIGN.md §6.3, §6.4).
type ShopBalance struct {
	SellRate  decimal.Decimal
	BuyMarkup decimal.Decimal
}
