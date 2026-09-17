package world

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/gate"
	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
)

// readOwnerKindSQL reads one owner row's kind, unlocked.
const readOwnerKindSQL = `SELECT kind FROM owner WHERE id = $1`

// readGateChunkSQL finds a chat's own gate chunk in this maze, if any.
const readGateChunkSQL = `SELECT q, r FROM chunk WHERE maze_id = $1 AND gate_chat_id = $2`

// readMazeChunksSQL reads every created chunk's coordinate in the
// maze — the created-chunk snapshot the spiral scan reads.
const readMazeChunksSQL = `SELECT q, r FROM chunk WHERE maze_id = $1`

// readMazeGatesSQL reads every gate chunk's coordinate in the maze.
const readMazeGatesSQL = `SELECT q, r FROM chunk WHERE maze_id = $1 AND gate_chat_id IS NOT NULL`

// readGateChunk reads chatID's own gate chunk through q, returning
// ok=false on a miss rather than an error.
func (m *Maze) readGateChunk(ctx context.Context, q rowQueryer, chatID store.OwnerID) (hexgrid.Chunk, bool, error) {
	var ch hexgrid.Chunk
	err := q.QueryRow(ctx, readGateChunkSQL, m.id, chatID).Scan(&ch.Q, &ch.R)
	if errors.Is(err, pgx.ErrNoRows) {
		return hexgrid.Chunk{}, false, nil
	}
	if err != nil {
		return hexgrid.Chunk{}, false, fmt.Errorf("world: read gate chunk for chat %d: %w", chatID, err)
	}
	return ch, true, nil
}

// readChunkCoords reads every chunk coordinate sql selects for m's own
// maze, through tx.
func (m *Maze) readChunkCoords(ctx context.Context, tx pgx.Tx, sql string) ([]hexgrid.Chunk, error) {
	rows, err := tx.Query(ctx, sql, m.id)
	if err != nil {
		return nil, fmt.Errorf("world: read chunk coordinates: %w", err)
	}
	defer rows.Close()

	var out []hexgrid.Chunk
	for rows.Next() {
		var ch hexgrid.Chunk
		if err := rows.Scan(&ch.Q, &ch.R); err != nil {
			return nil, fmt.Errorf("world: scan chunk coordinate: %w", err)
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("world: read chunk coordinates: %w", err)
	}
	return out, nil
}

// ActivateChat allocates chatID's own gate in m, returning its chunk
// coordinate — from which a caller takes the gate cell through
// Lattice().Center. The chat is the subject and playerID the
// initiator, so the event's two owner dimensions cannot disagree with
// each other. A chat that already has a gate here gets it back
// unchanged, writing nothing; a new allocation runs under the same
// locked transaction chunk creation uses and the same
// Spec.CreateBudget. Named error returns: ErrNotAChat when chatID does
// not name a chat, ErrMazeMissing from the lock statement, and the
// spiral placement rule's own negative-spacing refusal, propagated
// unchanged.
func (m *Maze) ActivateChat(ctx context.Context, chatID store.OwnerID, playerID *store.OwnerID) (hexgrid.Chunk, error) {
	bctx, cancel := context.WithTimeout(ctx, m.createBudget)
	defer cancel()

	var kind store.OwnerKind
	if err := m.pool.QueryRow(bctx, readOwnerKindSQL, chatID).Scan(&kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return hexgrid.Chunk{}, ErrNotAChat
		}
		return hexgrid.Chunk{}, wrapBudget(bctx, fmt.Errorf("world: read chat owner: %w", err))
	}
	if kind != store.OwnerChat {
		return hexgrid.Chunk{}, ErrNotAChat
	}

	if ch, ok, err := m.readGateChunk(bctx, m.pool, chatID); err != nil {
		return hexgrid.Chunk{}, wrapBudget(bctx, err)
	} else if ok {
		return ch, nil
	}

	by := Actor{PlayerID: playerID, ChatID: &chatID}
	return lockedTxBody(bctx, m, func(tx pgx.Tx) (hexgrid.Chunk, error) {
		if ch, ok, err := m.readGateChunk(bctx, tx, chatID); err != nil {
			return hexgrid.Chunk{}, wrapBudget(bctx, err)
		} else if ok {
			return ch, nil
		}

		created, err := m.readChunkCoords(bctx, tx, readMazeChunksSQL)
		if err != nil {
			return hexgrid.Chunk{}, wrapBudget(bctx, err)
		}
		gates, err := m.readChunkCoords(bctx, tx, readMazeGatesSQL)
		if err != nil {
			return hexgrid.Chunk{}, wrapBudget(bctx, err)
		}

		chosen, err := gate.Next(created, gates, m.gateSpacing)
		if err != nil {
			return hexgrid.Chunk{}, err
		}

		spiralIndex := gate.SpiralIndex(chosen)
		ring := hexgrid.ChunkDistance(hexgrid.Chunk{}, chosen)

		neighbors, err := m.readNeighbors(bctx, tx, chosen)
		if err != nil {
			return hexgrid.Chunk{}, wrapBudget(bctx, err)
		}
		if _, err := m.generateInsertAndAppend(bctx, tx, chosen, ChunkTypeGate, CreationCauseChatActivation, by, &chatID, &spiralIndex, &ring, neighbors); err != nil {
			return hexgrid.Chunk{}, err
		}
		return chosen, nil
	})
}
