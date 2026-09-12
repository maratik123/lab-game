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

// ItemID identifies a row in the item table: one instance in the item
// machine. NewItem is not an instance id — it is a mint marker passed in a
// Movement's ItemID field to tell Move to create a fresh instance under
// this move's own document.
type ItemID int64

// NewItem is the mint-marker value of ItemID: a Movement carrying it names
// no existing instance, and Move creates one.
const NewItem ItemID = 0

// HolderID identifies a row in scope, read as an address of a holder's
// slot — a scope, not the table it resolves to, because the API is about
// the role a holder plays (a player's backpack, a corpse, a
// construction), never about the table storing it.
type HolderID int64

// WorldHolder is the seeded id of the singleton World scope — the origin
// every instance's chain starts at, and the one holder whose capacity
// accounts are uncontrolled.
const WorldHolder HolderID = 1
