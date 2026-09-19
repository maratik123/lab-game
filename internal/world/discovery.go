package world

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
)

// insertDiscoverySQL inserts a personal discovery row on the caller's
// own transaction. Selecting from owner, filtered on kind = 'player',
// is what makes a non-player insert nothing; ON CONFLICT DO NOTHING is
// what keeps a repeat's first discovered_at.
const insertDiscoverySQL = `
	INSERT INTO node_discovery (maze_id, q, r, player_id)
	SELECT $1, $2, $3, $4 FROM owner WHERE id = $4 AND kind = 'player'
	ON CONFLICT (maze_id, q, r, player_id) DO NOTHING
`

// RecordDiscovery records playerID's own discovery of cell in m, on
// tx — the caller's own transaction, so a discovery rolls back with
// whatever move edge produced it. The maze_id half of the row's key
// comes from m itself, never a parameter, so a discovery cannot be
// filed against a maze the caller never opened. A repeat call for the
// same (maze, cell, player) triple changes nothing. Returns
// ErrNotAPlayer, writing nothing, when playerID does not name a
// player.
func (m *Maze) RecordDiscovery(ctx context.Context, tx pgx.Tx, cell hexgrid.Coord, playerID store.OwnerID) error {
	tag, err := tx.Exec(ctx, insertDiscoverySQL, m.id, cell.Q, cell.R, playerID)
	if err != nil {
		return fmt.Errorf("world: record discovery: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	// Zero rows affected is ambiguous by itself: either playerID names
	// no player (the SELECT matched nothing) or this triple was already
	// discovered (ON CONFLICT DO NOTHING skipped a matching row).
	// Reading the owner's own kind tells the two apart.
	var kind store.OwnerKind
	err = tx.QueryRow(ctx, readOwnerKindSQL, playerID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotAPlayer
	}
	if err != nil {
		return fmt.Errorf("world: read player owner: %w", err)
	}
	if kind != store.OwnerPlayer {
		return ErrNotAPlayer
	}
	return nil
}
