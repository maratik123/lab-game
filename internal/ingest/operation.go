package ingest

import (
	"fmt"
)

// IDSpace names one namespace an operation_id's raw identifier is drawn
// from. The ledger package treats operation_id as opaque, so the grammar
// and the set of spaces are this package's to define. A third
// space costs a new member and a new field on Update, never a migration
// of stored rows.
type IDSpace string

const (
	// IDSpaceUpdate is the update_id sequence — every update's canonical
	// idempotency key.
	IDSpaceUpdate IDSpace = "update_id"
	// IDSpaceCallbackQuery is the callback_query.id sequence — present
	// only on an update carrying a callback query, in addition to its
	// canonical update_id key.
	IDSpaceCallbackQuery IDSpace = "callback_query.id"
)

// operationIDSeparator joins an IDSpace and a raw identifier into one
// operation_id. No IDSpace member may contain it.
const operationIDSeparator = ":"

// operationID builds one operation_id as "<space>:<id>", refusing an
// empty space (ErrEmptyIDSpace) or an empty id
// (ErrEmptyID). Unexported: a handler has no function to assemble an
// operation_id from raw Telegram fields — only Update's own derived
// fields carry one.
func operationID(space IDSpace, id string) (string, error) {
	if space == "" {
		return "", ErrEmptyIDSpace
	}
	if id == "" {
		return "", ErrEmptyID
	}
	return fmt.Sprintf("%s%s%s", space, operationIDSeparator, id), nil
}
