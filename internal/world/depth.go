package world

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
)

// Queryer is the multi-row read Depth needs — this package's own,
// because the ledger's own read-sharing interface carries a single-row
// query alone and this read is multi-row. Both a transaction and a
// pool-backed caller satisfy it, so a raid transition can read depth
// inside its own transaction.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// readGatesSQL reads every gate chunk's coordinate in the maze — the
// only input Depth's metric reads. A crafted door is bound to a cell
// and lives in its own table, never a chunk row, so there is no filter
// to forget: the metric's input type already excludes it.
const readGatesSQL = `SELECT q, r FROM chunk WHERE maze_id = $1 AND gate_chat_id IS NOT NULL`

// Depth returns the hex-cell distance from cell to the nearest gate
// chunk's centre cell in m, and true — or (0, false) when the maze
// holds no gate, matching the placement package's own Set.Depth
// contract. It is built fresh from the maze's gate rows on every call:
// no stored column, no invalidation.
func (m *Maze) Depth(ctx context.Context, q Queryer, cell hexgrid.Coord) (depth int64, found bool, err error) {
	rows, err := q.Query(ctx, readGatesSQL, m.id)
	if err != nil {
		return 0, false, fmt.Errorf("world: read gate chunks: %w", err)
	}
	defer rows.Close()

	var gates []hexgrid.Chunk
	for rows.Next() {
		var ch hexgrid.Chunk
		if err := rows.Scan(&ch.Q, &ch.R); err != nil {
			return 0, false, fmt.Errorf("world: scan gate chunk: %w", err)
		}
		gates = append(gates, ch)
	}
	if err := rows.Err(); err != nil {
		return 0, false, fmt.Errorf("world: read gate chunks: %w", err)
	}

	set, err := gate.NewSet(m.lattice, gates)
	if err != nil {
		return 0, false, fmt.Errorf("world: build gate set: %w", err)
	}
	depth, found = set.Depth(cell)
	return depth, found, nil
}
