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

// readNeighbors reads every already-created neighbour of ch, through
// tx, in canonical direction order.
func (m *Maze) readNeighbors(ctx context.Context, tx pgx.Tx, ch hexgrid.Chunk) ([]maze.Map, error) {
	var neighbors []maze.Map
	for d := hexgrid.DirE; d <= hexgrid.DirSE; d++ {
		nm, ok, err := m.readChunk(ctx, tx, ch.Neighbor(d))
		if err != nil {
			return nil, err
		}
		if ok {
			neighbors = append(neighbors, nm)
		}
	}
	return neighbors, nil
}

// EnsureChunkAt returns the map of the chunk holding cell, creating it
// whole under a short, per-maze locked transaction on a genuine miss.
// Its creation cause is always explorer — there is no cause parameter
// to get wrong. The whole call, unlocked read included, runs under
// Spec.CreateBudget: a caller that already holds its own transaction
// and calls this from inside it holds two pool connections at once for
// the call's own duration, so the composition root must size the pool
// at least two connections per such concurrent nesting caller, and
// either an exhausted pool or a long-held maze row lock surfaces as
// ErrCreateBudget rather than a hang.
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
// ErrCreateBudget wrapping ctx's own error — a deadline as well as a
// cancellation — when ctx is what ended the operation, and returns err
// unchanged otherwise.
func wrapBudget(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrCreateBudget, ctxErr)
	}
	return err
}

// lockedTxBody runs body on a fresh, READ-COMMITTED transaction over
// m's pool, taking the per-maze row lock first (ErrMazeMissing on a
// miss) and committing body's result on success, rolling back
// otherwise. It is the one place either a chunk creation or a gate
// allocation opens a transaction, so both run under the same lock and
// the same shape.
func lockedTxBody[T any](ctx context.Context, m *Maze, body func(tx pgx.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, wrapBudget(ctx, fmt.Errorf("world: begin transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var exists int
	if err := tx.QueryRow(ctx, lockMazeSQL, m.id).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, ErrMazeMissing
		}
		return zero, wrapBudget(ctx, fmt.Errorf("world: lock maze row: %w", err))
	}

	v, err := body(tx)
	if err != nil {
		return zero, err
	}

	if err := tx.Commit(ctx); err != nil {
		return zero, wrapBudget(ctx, fmt.Errorf("world: commit transaction: %w", err))
	}
	committed = true
	return v, nil
}

// generateInsertAndAppend reads ch's already-created neighbours,
// generates ch's map under typ against them, inserts its chunk row and
// appends its chunk_created event, all on tx. gateChatID and
// spiralIndex/ring are nil for a fabric chunk and non-nil for a gate
// one. Both creation paths route their neighbour read through here, so
// omitting it would take a change to this function rather than to a
// call site.
func (m *Maze) generateInsertAndAppend(
	ctx context.Context,
	tx pgx.Tx,
	ch hexgrid.Chunk,
	typ ChunkType,
	cause CreationCause,
	by Actor,
	gateChatID *store.OwnerID,
	spiralIndex, ring *int64,
) (maze.Map, error) {
	neighbors, err := m.readNeighbors(ctx, tx, ch)
	if err != nil {
		return maze.Map{}, wrapBudget(ctx, err)
	}

	mazeType, err := typ.toMaze()
	if err != nil {
		return maze.Map{}, err
	}
	generated, err := m.gen.Generate(ch, mazeType, neighbors...)
	if err != nil {
		return maze.Map{}, wrapBudget(ctx, fmt.Errorf("world: generate chunk %v: %w", ch, err))
	}

	if _, err := tx.Exec(ctx, insertChunkSQL,
		m.id, ch.Q, ch.R, typ, cause, gateChatID, spiralIndex, encode(generated), generated.Version(),
	); err != nil {
		return maze.Map{}, wrapBudget(ctx, fmt.Errorf("world: insert chunk %v: %w", ch, err))
	}

	if err := appendChunkCreated(ctx, tx, m.id, by, ch, typ, cause, generated.Version(), spiralIndex, ring); err != nil {
		return maze.Map{}, wrapBudget(ctx, err)
	}

	return generated, nil
}

// createLocked runs the locked creation transaction body for
// EnsureChunkAt's explorer cause only: re-check for an existing chunk
// under the lock (the loser of a creation race finds the winner's row
// here and returns it, writing nothing), then generate, insert and
// append the chunk_created event through generateInsertAndAppend, which
// reads the existing neighbours' stored maps itself. The
// chat_activation cause does not call this function — it discovers its
// chunk under the same lock, from the spiral rule evaluated over that
// transaction's own snapshot, so folding it into this body would mean
// opening a second transaction and re-taking the lock, splitting the
// choice of chunk from its insert. It calls generateInsertAndAppend
// directly instead, sharing this function's read-then-generate tail
// without sharing the chunk-choice step above it.
func (m *Maze) createLocked(
	bctx context.Context,
	ch hexgrid.Chunk,
	typ ChunkType,
	cause CreationCause,
	by Actor,
	gateChatID *store.OwnerID,
	spiralIndex, ring *int64,
) (maze.Map, error) {
	return lockedTxBody(bctx, m, func(tx pgx.Tx) (maze.Map, error) {
		if mp, ok, err := m.readChunk(bctx, tx, ch); err != nil {
			return maze.Map{}, wrapBudget(bctx, err)
		} else if ok {
			return mp, nil
		}

		return m.generateInsertAndAppend(bctx, tx, ch, typ, cause, by, gateChatID, spiralIndex, ring)
	})
}
