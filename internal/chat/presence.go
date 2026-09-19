package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/store"
)

// upsertPresenceSQL writes chatID's current presence on the caller's own
// transaction. Selecting from owner, filtered on kind = 'chat', is what
// makes a non-chat write nothing — a caller-side kind check is not
// enough, since the check-then-write pair is not atomic on its own. ON
// CONFLICT DO UPDATE with an IS DISTINCT FROM guard is
// what keeps changed_at where it was on a repeat write of the same
// value: the column default fires on INSERT only, so a flip writes it
// explicitly and a no-op write does not touch it at all. RETURNING true
// fires exactly when a row was inserted or actually updated — never on
// a skipped, predicate-failed UPDATE — which is what lets the caller
// tell "changed" apart from "already had this value" with one round
// trip in the common case.
const upsertPresenceSQL = `
	INSERT INTO chat_presence (chat_id, present)
	SELECT $1, $2 FROM owner WHERE id = $1 AND kind = 'chat'
	ON CONFLICT (chat_id) DO UPDATE
	    SET present = EXCLUDED.present, changed_at = now()
	    WHERE chat_presence.present IS DISTINCT FROM EXCLUDED.present
	RETURNING true
`

// SetPresence writes chatID's current presence as present, on tx — the
// caller's own transaction. It reports whether the stored value actually
// changed: true for a first-ever write or a flip, false for a repeat
// write of the same value (changed_at is left untouched in that case).
// Returns ErrNotAChat, writing nothing, when chatID does not name a
// chat.
func SetPresence(ctx context.Context, tx pgx.Tx, chatID store.OwnerID, present bool) (bool, error) {
	var changed bool
	err := tx.QueryRow(ctx, upsertPresenceSQL, chatID, present).Scan(&changed)
	if err == nil {
		return changed, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("chat: set presence: %w", err)
	}

	// Zero rows is ambiguous by itself: either chatID names no chat (the
	// SELECT matched nothing, so the INSERT wrote nothing and ON
	// CONFLICT never triggered) or a row already existed with this same
	// value (ON CONFLICT triggered but its WHERE predicate skipped the
	// update). Reading the owner's own kind tells the two apart.
	var kind store.OwnerKind
	kindErr := tx.QueryRow(ctx, `SELECT kind FROM owner WHERE id = $1`, chatID).Scan(&kind)
	if errors.Is(kindErr, pgx.ErrNoRows) {
		return false, ErrNotAChat
	}
	if kindErr != nil {
		return false, fmt.Errorf("chat: read chat owner: %w", kindErr)
	}
	if kind != store.OwnerChat {
		return false, ErrNotAChat
	}
	return false, nil
}

// Present reads chatID's current presence row on q. ok is false when no
// row exists yet — a chat this package has never processed a presence
// write for — which is not the same as a stored present=false.
func Present(ctx context.Context, q store.Queryer, chatID store.OwnerID) (present, ok bool, err error) {
	err = q.QueryRow(ctx, `SELECT present FROM chat_presence WHERE chat_id = $1`, chatID).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("chat: read presence: %w", err)
	}
	return present, true, nil
}
