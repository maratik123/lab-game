package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/store"
)

// insertMembershipSQL inserts a membership row on the caller's own
// transaction. Selecting from owner twice, filtered on each side's own
// kind, is what makes a wrong-kind write nothing — a caller-side kind
// check is not enough, since the check-then-write pair is not atomic on
// its own. ON CONFLICT DO NOTHING is what keeps a repeat call
// idempotent.
const insertMembershipSQL = `
	INSERT INTO chat_membership (chat_id, player_id)
	SELECT $1, $2
	FROM owner c, owner p
	WHERE c.id = $1 AND c.kind = 'chat' AND p.id = $2 AND p.kind = 'player'
	ON CONFLICT (chat_id, player_id) DO NOTHING
`

// AddMembership records playerID's membership of chatID, on tx — the
// caller's own transaction. It reports whether a new row was written:
// true the first time, false on a repeat (nothing changes) and on a
// refusal. Returns ErrNotAChat when chatID does not name a chat, or
// ErrNotAPlayer when playerID does not name a player; either refusal
// writes nothing.
func AddMembership(ctx context.Context, tx pgx.Tx, chatID, playerID store.OwnerID) (bool, error) {
	tag, err := tx.Exec(ctx, insertMembershipSQL, chatID, playerID)
	if err != nil {
		return false, fmt.Errorf("chat: add membership: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}

	// Zero rows affected is ambiguous by itself: either chatID or
	// playerID names the wrong kind (the SELECT matched nothing) or this
	// pair was already a member (ON CONFLICT DO NOTHING skipped a
	// matching row). Reading each owner's own kind tells the cases
	// apart.
	var chatKind store.OwnerKind
	err = tx.QueryRow(ctx, `SELECT kind FROM owner WHERE id = $1`, chatID).Scan(&chatKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotAChat
	}
	if err != nil {
		return false, fmt.Errorf("chat: read chat owner: %w", err)
	}
	if chatKind != store.OwnerChat {
		return false, ErrNotAChat
	}

	var playerKind store.OwnerKind
	err = tx.QueryRow(ctx, `SELECT kind FROM owner WHERE id = $1`, playerID).Scan(&playerKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotAPlayer
	}
	if err != nil {
		return false, fmt.Errorf("chat: read player owner: %w", err)
	}
	if playerKind != store.OwnerPlayer {
		return false, ErrNotAPlayer
	}

	return false, nil
}
