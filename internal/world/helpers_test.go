package world

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
	"github.com/maratik123/lab-game/internal/store"
)

// testCreateBudget is a generous, instrument-only ceiling for every
// case but the budget refusal case itself, which builds its own.
const testCreateBudget = 5 * time.Second

// testParams returns a small, valid generation Params: the generator's
// own minimum radius, so a chunk is small enough to compare cell by
// cell, and every share well inside the bounds New accepts.
func testParams() maze.Params {
	return maze.Params{
		Radius:            maze.MinRadius,
		Weights:           maze.AlgorithmWeights{1, 1, 1, 1, 1},
		IslandShare:       decimal.New(5, -2),
		ExtraPassageShare: decimal.New(15, -2),
		GrowingTreeBias:   decimal.New(5, -1),
		PortalShareLower:  decimal.New(1, -1),
		PortalShareUpper:  decimal.New(2, -1),
	}
}

// testGenerator builds a Generator over testParams and a seed of the
// caller's choosing, failing tb on refusal.
func testGenerator(tb testing.TB, seed int64) *maze.Generator {
	tb.Helper()
	gen, err := maze.New(seed, testParams())
	if err != nil {
		tb.Fatalf("maze.New: %v", err)
	}
	return gen
}

// mapsEqual asserts a and b agree on chunk, type, version and every
// local cell's face set.
func mapsEqual(t *testing.T, a, b maze.Map) {
	t.Helper()
	if a.Chunk() != b.Chunk() {
		t.Fatalf("chunk = %v, want %v", a.Chunk(), b.Chunk())
	}
	if a.Type() != b.Type() {
		t.Fatalf("type = %v, want %v", a.Type(), b.Type())
	}
	if a.Version() != b.Version() {
		t.Fatalf("version = %d, want %d", a.Version(), b.Version())
	}
	lattice := hexgrid.Lattice{Radius: a.Radius()}
	for _, c := range lattice.LocalCells() {
		af, ok := a.Faces(c)
		if !ok {
			t.Fatalf("a.Faces(%v): not ok", c)
		}
		bf, ok := b.Faces(c)
		if !ok {
			t.Fatalf("b.Faces(%v): not ok", c)
		}
		if af != bf {
			t.Fatalf("cell %v faces = %v, want %v", c, af, bf)
		}
	}
}

// borderAgrees walks the shared border between chunks a and b (b in
// direction d from a) and asserts every face pair the two maps report
// agrees, from both sides.
func borderAgrees(t *testing.T, lattice hexgrid.Lattice, a, b maze.Map, d hexgrid.Direction) {
	t.Helper()
	if b.Chunk() != a.Chunk().Neighbor(d) {
		t.Fatalf("b.Chunk() = %v, want %v's neighbour in direction %v", b.Chunk(), a.Chunk(), d)
	}
	for _, face := range lattice.Border(d) {
		aFaces, ok := a.Faces(face.Cell)
		if !ok {
			t.Fatalf("a.Faces(%v): not ok", face.Cell)
		}
		globalNeighbor := lattice.At(a.Chunk(), face.Cell).Neighbor(d)
		_, bLocal := lattice.Locate(globalNeighbor)
		bFaces, ok := b.Faces(bLocal)
		if !ok {
			t.Fatalf("b.Faces(%v): not ok", bLocal)
		}
		if aFaces[d] != bFaces[d.Opposite()] {
			t.Fatalf("border disagreement at %v/%v: %v vs %v", face.Cell, bLocal, aFaces[d], bFaces[d.Opposite()])
		}
	}
}

// openTestMaze opens a maze on pool with testParams and a caller-chosen
// seed, failing tb on refusal.
func openTestMaze(tb testing.TB, pool *pgxpool.Pool, seed int64) *Maze {
	tb.Helper()
	m, err := Open(tb.Context(), pool, Spec{
		Biome: "test_biome", Season: 1, Seed: seed,
		Generation: testParams(), GateSpacing: 1, CreateBudget: testCreateBudget,
	})
	if err != nil {
		tb.Fatalf("Open: %v", err)
	}
	return m
}

// createOwner creates an owner of kind through the ledger's own owner
// constructor — the only route that also creates its scopes and
// accounts — on its own committed transaction, failing tb on refusal.
func createOwner(tb testing.TB, pool *pgxpool.Pool, kind store.OwnerKind, telegramID int64) store.OwnerID {
	tb.Helper()
	ctx := tb.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		tb.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	owner, err := store.CreateOwner(ctx, tx, kind, &telegramID)
	if err != nil {
		tb.Fatalf("CreateOwner: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		tb.Fatalf("commit: %v", err)
	}
	return owner.ID
}
