package store

// OwnerID identifies a row in the owner table.
type OwnerID int64

// AccountID identifies a row in the account table. Distinct from a
// Telegram user or chat id, which is also an int64 but lives in owner's
// telegram_id column.
type AccountID int64

// WorldOwner is the seeded id of the singleton World owner, from the
// ledger-core migration's seed data.
const WorldOwner OwnerID = 1

// WorldMoney is the seeded id of the World's uncontrolled money account.
const WorldMoney AccountID = 1

// WorldExperience is the seeded id of the World's uncontrolled experience
// account.
const WorldExperience AccountID = 2

// EventID identifies a row in the event table.
type EventID int64
