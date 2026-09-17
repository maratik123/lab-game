package world

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
	"github.com/maratik123/lab-game/internal/store"
)

// Actor carries the player and chat dimensions a created chunk's
// chunk_created event is stamped with. Either field may be nil; the
// dimensions are those of the caller's own session, not a property of
// the chunk itself.
type Actor struct {
	PlayerID *store.OwnerID
	ChatID   *store.OwnerID
}

// readChunkSQL reads one chunk row's stored content, unlocked.
const readChunkSQL = `SELECT chunk_type, faces, generation_version FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`

// lockMazeSQL takes the per-maze row lock: FOR NO KEY UPDATE, the
// weakest mode that still conflicts with itself, which is all the
// mutual exclusion a chunk creation needs. A miss (no such maze row)
// scans zero rows rather than erroring.
const lockMazeSQL = `SELECT 1 FROM maze WHERE id = $1 FOR NO KEY UPDATE`

// insertChunkSQL inserts a chunk row exactly once; gateChatID and
// spiralIndex are NULL for a fabric chunk and non-NULL for a gate one,
// which the CHECK on chunk_type = 'gate' holds together.
const insertChunkSQL = `
	INSERT INTO chunk (maze_id, q, r, chunk_type, creation_cause, gate_chat_id, spiral_index, faces, generation_version)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`

// rowQueryer is the single QueryRow method both a pool and a
// transaction satisfy — declared here, by this file's own consumer,
// because readChunk runs on either.
type rowQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// readChunk reads ch's stored map through q, returning ok=false on a
// miss rather than an error.
func (m *Maze) readChunk(ctx context.Context, q rowQueryer, ch hexgrid.Chunk) (maze.Map, bool, error) {
	var typ ChunkType
	var faces []byte
	var version int32
	err := q.QueryRow(ctx, readChunkSQL, m.id, ch.Q, ch.R).Scan(&typ, &faces, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return maze.Map{}, false, nil
	}
	if err != nil {
		return maze.Map{}, false, fmt.Errorf("world: read chunk (%d,%d): %w", ch.Q, ch.R, err)
	}
	mp, err := decode(ch, typ, version, faces, m.lattice)
	if err != nil {
		return maze.Map{}, false, err
	}
	return mp, true, nil
}

// EnsureChunkAt returns the map of the chunk holding cell, creating it
// whole under a short, per-maze locked transaction on a genuine
// miss. Its creation cause is always explorer — there is no cause
// parameter to get wrong. The whole call, unlocked read included, runs
// under Spec.CreateBudget: a caller that already holds its own
// transaction and calls this from inside it holds two pool connections
// at once for the call's own duration, so the composition root must
// size the pool at least two connections per such concurrent nesting
// caller, and either an exhausted pool or a long-held maze row lock
// surfaces as ErrCreateBudget rather than a hang.
func (m *Maze) EnsureChunkAt(ctx context.Context, cell hexgrid.Coord, by Actor) (maze.Map, error) {
	bctx, cancel := context.WithTimeout(ctx, m.createBudget)
	defer cancel()

	ch, _ := m.lattice.Locate(cell)

	if mp, ok, err := m.readChunk(bctx, m.pool, ch); err != nil {
		return maze.Map{}, wrapBudget(bctx, err)
	} else if ok {
		return mp, nil
	}

	return m.createLocked(bctx, ch, ChunkTypeFabric, CreationCauseExplorer, by, nil, nil, nil)
}

// wrapBudget names a failure inside a budgeted operation as
// ErrCreateBudget wrapping the context's own error when the budget's
// context is what ended the operation, and returns err unchanged
// otherwise.
func wrapBudget(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrCreateBudget, ctxErr)
	}
	return err
}

// createLocked runs the locked creation transaction body, shared by EnsureChunkAt's
// explorer cause and ActivateChat's chat_activation cause: lock the
// maze row, re-check for an existing chunk (the loser of a creation
// race finds the winner's row here and returns it, writing nothing),
// read the existing neighbours' stored maps, generate, insert, append
// the chunk_created event, commit. gateChatID and spiralIndex/ring are
// nil for typ = ChunkTypeFabric and non-nil for ChunkTypeGate. bctx is
// the caller's own already-budgeted context (see EnsureChunkAt).
func (m *Maze) createLocked(
	bctx context.Context,
	ch hexgrid.Chunk,
	typ ChunkType,
	cause CreationCause,
	by Actor,
	gateChatID *store.OwnerID,
	spiralIndex, ring *int64,
) (maze.Map, error) {
	tx, err := m.pool.BeginTx(bctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return maze.Map{}, wrapBudget(bctx, fmt.Errorf("world: begin creation transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(bctx)
		}
	}()

	var exists int
	if err := tx.QueryRow(bctx, lockMazeSQL, m.id).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return maze.Map{}, ErrMazeMissing
		}
		return maze.Map{}, wrapBudget(bctx, fmt.Errorf("world: lock maze row: %w", err))
	}

	if mp, ok, err := m.readChunk(bctx, tx, ch); err != nil {
		return maze.Map{}, wrapBudget(bctx, err)
	} else if ok {
		return mp, nil
	}

	var neighbors []maze.Map
	for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
		nc := ch.Neighbor(d)
		nm, ok, err := m.readChunk(bctx, tx, nc)
		if err != nil {
			return maze.Map{}, wrapBudget(bctx, err)
		}
		if ok {
			neighbors = append(neighbors, nm)
		}
	}

	mazeType, err := typ.toMaze()
	if err != nil {
		return maze.Map{}, err
	}
	generated, err := m.gen.Generate(ch, mazeType, neighbors...)
	if err != nil {
		return maze.Map{}, wrapBudget(bctx, fmt.Errorf("world: generate chunk %v: %w", ch, err))
	}

	if _, err := tx.Exec(bctx, insertChunkSQL,
		m.id, ch.Q, ch.R, typ, cause, gateChatID, spiralIndex, encode(generated), generated.Version(),
	); err != nil {
		return maze.Map{}, wrapBudget(bctx, fmt.Errorf("world: insert chunk %v: %w", ch, err))
	}

	if err := appendChunkCreated(bctx, tx, m.id, by, ch, typ, cause, generated.Version(), spiralIndex, ring); err != nil {
		return maze.Map{}, wrapBudget(bctx, err)
	}

	if err := tx.Commit(bctx); err != nil {
		return maze.Map{}, wrapBudget(bctx, fmt.Errorf("world: commit creation transaction: %w", err))
	}
	committed = true

	return generated, nil
}
