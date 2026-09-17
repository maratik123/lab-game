package world

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
)

// Spec is Open's own input: the identity of the maze to resolve or
// create, and the generation and gate-placement inputs its handle
// carries. Season is a plain parameter with no default and no
// configuration key — whoever creates the maze supplies it; a season
// key arrives with whatever later task ships rotation.
type Spec struct {
	// Biome names the authored world this maze runs: the identity a
	// maze row and every telemetry event carry.
	Biome string
	// Season is the rotation season this maze runs under.
	Season int32
	// Seed is the generator's own seed argument for this maze.
	Seed int64
	// Generation carries the generator's own input parameters.
	Generation maze.Params
	// GateSpacing is the minimum hex distance ActivateChat keeps
	// between one chat's gate and any other's.
	GateSpacing int
	// CreateBudget bounds a chunk-creation transaction's own context:
	// the acquisition-and-work budget EnsureChunkAt and ActivateChat
	// apply so an exhausted connection pool or a long-held maze row
	// lock surfaces as ErrCreateBudget instead of a hang. Must be
	// strictly positive — there is no default to fall back to.
	CreateBudget time.Duration
}

// Maze is a handle onto one persisted maze: its pool, its row id, the
// generator its stored seed and generation inputs build, its lattice,
// and its gate spacing. Every exported method takes the caller's own
// context; the handle itself stores no context.
type Maze struct {
	pool         *pgxpool.Pool
	id           int64
	gen          *maze.Generator
	lattice      hexgrid.Lattice
	gateSpacing  int
	createBudget time.Duration
}

// openInsertSQL creates the maze row on the first Open for a
// (biome, season) pair; a later Open for the same pair changes nothing.
const openInsertSQL = `
	INSERT INTO maze (biome, world_seed, season)
	VALUES ($1, $2, $3)
	ON CONFLICT (biome, season) DO NOTHING
`

// openReadSQL reads back the row Open just ensured exists, whichever
// caller created it.
const openReadSQL = `SELECT id, world_seed FROM maze WHERE biome = $1 AND season = $2`

// Open resolves spec's maze row, creating it when absent, and returns a
// handle onto it. It refuses a non-positive spec.CreateBudget and a
// spec.Generation the generator's own constructor refuses, before
// issuing any statement. When a row already exists for spec's biome
// and season under a different world seed, it returns ErrSeedMismatch
// naming both seeds and leaves the row untouched.
func Open(ctx context.Context, pool *pgxpool.Pool, spec Spec) (*Maze, error) {
	if spec.CreateBudget <= 0 {
		return nil, fmt.Errorf("world: Spec.CreateBudget must be positive, got %v", spec.CreateBudget)
	}
	gen, err := maze.New(spec.Seed, spec.Generation)
	if err != nil {
		return nil, err
	}

	if _, err := pool.Exec(ctx, openInsertSQL, spec.Biome, spec.Seed, spec.Season); err != nil {
		return nil, fmt.Errorf("world: open: insert maze row: %w", err)
	}

	var id, storedSeed int64
	if err := pool.QueryRow(ctx, openReadSQL, spec.Biome, spec.Season).Scan(&id, &storedSeed); err != nil {
		return nil, fmt.Errorf("world: open: read maze row: %w", err)
	}
	if storedSeed != spec.Seed {
		return nil, fmt.Errorf("%w: stored %d, requested %d", ErrSeedMismatch, storedSeed, spec.Seed)
	}

	return &Maze{
		pool:         pool,
		id:           id,
		gen:          gen,
		lattice:      hexgrid.Lattice{Radius: spec.Generation.Radius},
		gateSpacing:  spec.GateSpacing,
		createBudget: spec.CreateBudget,
	}, nil
}

// Lattice returns the super-lattice m's chunks are generated over, so a
// caller can locate a cell's chunk or take a gate chunk's centre cell.
func (m *Maze) Lattice() hexgrid.Lattice {
	return m.lattice
}
