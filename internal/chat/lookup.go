package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
)

// PoolLookup answers the outbound gate's two destination questions over
// a *pgxpool.Pool: the gate's own connection, taken at wiring time
// rather than borrowed from any in-flight transaction, so a row a
// handler's own uncommitted transaction wrote is not visible here, by
// design. It structurally satisfies the ingest package's
// DestinationLookup interface without importing that package — declared
// there, by its own consumer.
type PoolLookup struct {
	pool *pgxpool.Pool
}

// NewPoolLookup builds a PoolLookup over pool.
func NewPoolLookup(pool *pgxpool.Pool) PoolLookup {
	return PoolLookup{pool: pool}
}

// PlayerExists reports whether a player owner row exists for telegramID.
func (l PoolLookup) PlayerExists(ctx context.Context, telegramID int64) (bool, error) {
	return store.PlayerExists(ctx, l.pool, telegramID)
}

// BotPresentInChat reports whether the bot is currently present in the
// chat named by telegramID — the raw chat id token an outbound call
// carries, never an owner id. It answers false, without an error, both
// for a chat with a stored presence row of false (the bot was removed)
// and for a telegram id no chat owner exists for at all: neither case is
// a row this package can find, and the gate's own fail-closed posture
// applies only to a genuine read error.
func (l PoolLookup) BotPresentInChat(ctx context.Context, telegramID int64) (bool, error) {
	var present bool
	err := l.pool.QueryRow(ctx, `
		SELECT cp.present
		FROM owner o
		JOIN chat_presence cp ON cp.chat_id = o.id
		WHERE o.kind = 'chat' AND o.telegram_id = $1
	`, telegramID).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("chat: bot present in chat: %w", err)
	}
	return present, nil
}
