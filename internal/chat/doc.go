// Package chat owns the two facts a chat has beyond the ledger's own
// owner row: whether the bot is currently present in it, and which
// players are its members. SetPresence writes the current answer,
// change or no change, and Present reads it back. AddMembership records
// a player's link to a chat, accruing only — this package offers no way
// for a membership to lapse. PoolLookup answers the outbound gate's two
// questions — whether a telegram id names a player, and whether the bot
// is currently present in a telegram id's chat — over a pool, for the
// composition root to wire into the gate.
package chat
