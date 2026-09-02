package store

import "github.com/shopspring/decimal"

// Posting is one half-entry of a batch passed to Post: the account it
// touches and the signed amount to add to that account's ledger. The
// account's ledger kind and controlled-ness come from the database
// (account_definition), never from the caller — "wrong kind on wrong
// account" is unrepresentable.
type Posting struct {
	AccountID AccountID
	Amount    decimal.Decimal
}
