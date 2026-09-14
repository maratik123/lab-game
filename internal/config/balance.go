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

// WorldBalance holds world-generation constants.
type WorldBalance struct {
	Chunk ChunkBalance
}

// ChunkBalance is the hex-chunk radius: how many cells out from a
// chunk's centre its own cells reach.
type ChunkBalance struct {
	Radius int
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

// StaminaBalance is the stamina cap and per-step cost.
type StaminaBalance struct {
	Cap      decimal.Decimal
	StepCost decimal.Decimal
}

// StandingBalance times the noise-standing escalation.
type StandingBalance struct {
	CalmToNoises time.Duration
	NoisesToWave time.Duration
	WaveToWave   time.Duration
}

// DeathBalance times backpack expiry, the owning chat's head start, and the
// respawn debuff.
type DeathBalance struct {
	BackpackTTL      time.Duration
	OwnChatHeadStart time.Duration
	RespawnDebuff    time.Duration
}

// AFKBalance is the AFK-leader cruelty dial. Its shape — not only its
// number — is a placeholder.
type AFKBalance struct {
	Cruelty decimal.Decimal
}

// DoorBalance is the crafted-door price curve and the boss-reward door
// TTL. PriceBase, PricePerDistance and PriceDistanceExponent are a key
// triple whose combining formula is not fixed by this package.
type DoorBalance struct {
	PriceBase             decimal.Decimal
	PricePerDistance      decimal.Decimal
	PriceDistanceExponent decimal.Decimal
	BossRewardTTL         time.Duration
}

// MonsterBudgetBalance is the monster budget-by-distance curve. Base,
// PerDistance and DistanceExponent are a key triple whose combining
// formula is not fixed by this package.
type MonsterBudgetBalance struct {
	Base             decimal.Decimal
	PerDistance      decimal.Decimal
	DistanceExponent decimal.Decimal
	ScalingPerLevel  decimal.Decimal
}

// CombatBalance holds the combat dice and multipliers.
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

// ShopBalance is the shop's sell rate and buy markup.
type ShopBalance struct {
	SellRate  decimal.Decimal
	BuyMarkup decimal.Decimal
}
