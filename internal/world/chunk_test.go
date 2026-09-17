package world

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestEnsureChunkAt_Creation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 1)

	cell := hexgrid.Coord{Q: 2, R: -1}
	wantCh, _ := m.Lattice().Locate(cell)

	mp, err := m.EnsureChunkAt(ctx, cell, Actor{})
	if err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}
	if mp.Chunk() != wantCh {
		t.Fatalf("chunk = %v, want %v", mp.Chunk(), wantCh)
	}
	if mp.Type() != maze.ChunkTypeFabric {
		t.Fatalf("type = %v, want fabric", mp.Type())
	}

	var chunkType, cause string
	if err := pool.QueryRow(ctx,
		`SELECT chunk_type::text, creation_cause::text FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, wantCh.Q, wantCh.R,
	).Scan(&chunkType, &cause); err != nil {
		t.Fatalf("read chunk row: %v", err)
	}
	if chunkType != string(ChunkTypeFabric) || cause != string(CreationCauseExplorer) {
		t.Fatalf("stored (type, cause) = (%s, %s), want (fabric, explorer)", chunkType, cause)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1`, m.id).Scan(&count); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if count != 1 {
		t.Fatalf("chunk rows = %d, want 1", count)
	}
}

func TestEnsureChunkAt_EqualsGeneration_Isolated(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 2)

	ch := hexgrid.Chunk{Q: 5, R: 5}
	cell := m.Lattice().Center(ch)

	got, err := m.EnsureChunkAt(ctx, cell, Actor{})
	if err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}
	want, err := m.gen.Generate(ch, maze.ChunkTypeFabric)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	mapsEqual(t, got, want)
}

func TestEnsureChunkAt_EqualsGeneration_BesideNeighbor(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 3)

	chA := hexgrid.Chunk{Q: 0, R: 0}
	chB := chA.Neighbor(hexgrid.DirE)
	cellA := m.Lattice().Center(chA)
	cellB := m.Lattice().Center(chB)

	mapA, err := m.EnsureChunkAt(ctx, cellA, Actor{})
	if err != nil {
		t.Fatalf("EnsureChunkAt(A): %v", err)
	}
	mapB, err := m.EnsureChunkAt(ctx, cellB, Actor{})
	if err != nil {
		t.Fatalf("EnsureChunkAt(B): %v", err)
	}
	want, err := m.gen.Generate(chB, maze.ChunkTypeFabric, mapA)
	if err != nil {
		t.Fatalf("Generate(B, with neighbour A): %v", err)
	}
	mapsEqual(t, mapB, want)
}

// indexOfCell returns the index of target in cells, or -1.
func indexOfCell(cells []hexgrid.Coord, target hexgrid.Coord) int {
	for i, c := range cells {
		if c == target {
			return i
		}
	}
	return -1
}

func TestEnsureChunkAt_ReadDoesNotRegenerate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 4)

	ch := hexgrid.Chunk{Q: 1, R: 1}
	cell := m.Lattice().Center(ch)
	if _, err := m.EnsureChunkAt(ctx, cell, Actor{}); err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}

	var typ ChunkType
	var faces []byte
	var version int32
	if err := pool.QueryRow(ctx, readChunkSQL, m.id, ch.Q, ch.R).Scan(&typ, &faces, &version); err != nil {
		t.Fatalf("read chunk row: %v", err)
	}

	lattice := m.Lattice()
	cells := lattice.LocalCells()
	centreIdx := indexOfCell(cells, hexgrid.Coord{})
	neighborCoord := hexgrid.Coord{}.Neighbor(hexgrid.DirE)
	neighborIdx := indexOfCell(cells, neighborCoord)
	faces[centreIdx] ^= 1 << uint(hexgrid.DirE)
	faces[neighborIdx] ^= 1 << uint(hexgrid.DirW)

	if _, err := pool.Exec(ctx, `UPDATE chunk SET faces = $1 WHERE maze_id = $2 AND q = $3 AND r = $4`,
		faces, m.id, ch.Q, ch.R); err != nil {
		t.Fatalf("mutate stored blob: %v", err)
	}

	want, err := decode(ch, typ, version, faces, lattice)
	if err != nil {
		t.Fatalf("decode mutated blob: %v", err)
	}

	got, err := m.EnsureChunkAt(ctx, cell, Actor{})
	if err != nil {
		t.Fatalf("second EnsureChunkAt: %v", err)
	}
	mapsEqual(t, got, want)
}

func TestEnsureChunkAt_BorderAgreement(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		createAFirst bool
	}{
		{"a_then_b", true},
		{"b_then_a", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			pool := storetest.Pool(t)
			m := openTestMaze(t, pool, 10)

			chA := hexgrid.Chunk{Q: 0, R: 0}
			chB := chA.Neighbor(hexgrid.DirNE)
			cellA := m.Lattice().Center(chA)
			cellB := m.Lattice().Center(chB)

			var mapA, mapB maze.Map
			var err error
			if tc.createAFirst {
				mapA, err = m.EnsureChunkAt(ctx, cellA, Actor{})
				if err != nil {
					t.Fatalf("EnsureChunkAt(A): %v", err)
				}
				mapB, err = m.EnsureChunkAt(ctx, cellB, Actor{})
				if err != nil {
					t.Fatalf("EnsureChunkAt(B): %v", err)
				}
			} else {
				mapB, err = m.EnsureChunkAt(ctx, cellB, Actor{})
				if err != nil {
					t.Fatalf("EnsureChunkAt(B): %v", err)
				}
				mapA, err = m.EnsureChunkAt(ctx, cellA, Actor{})
				if err != nil {
					t.Fatalf("EnsureChunkAt(A): %v", err)
				}
			}
			borderAgrees(t, m.Lattice(), mapA, mapB, hexgrid.DirNE)
		})
	}
}

func TestEnsureChunkAt_Idempotent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 11)

	ch := hexgrid.Chunk{Q: 0, R: 0}
	centre := m.Lattice().Center(ch)
	offCentre := m.Lattice().At(ch, hexgrid.Coord{Q: 1, R: 0})

	first, err := m.EnsureChunkAt(ctx, centre, Actor{})
	if err != nil {
		t.Fatalf("first EnsureChunkAt: %v", err)
	}

	var beforeType, beforeCause string
	var beforeFaces []byte
	if err := pool.QueryRow(ctx,
		`SELECT chunk_type::text, creation_cause::text, faces FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, ch.Q, ch.R).Scan(&beforeType, &beforeCause, &beforeFaces); err != nil {
		t.Fatalf("read chunk row: %v", err)
	}

	second, err := m.EnsureChunkAt(ctx, offCentre, Actor{})
	if err != nil {
		t.Fatalf("second EnsureChunkAt: %v", err)
	}
	mapsEqual(t, first, second)

	var afterType, afterCause string
	var afterFaces []byte
	if err := pool.QueryRow(ctx,
		`SELECT chunk_type::text, creation_cause::text, faces FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, ch.Q, ch.R).Scan(&afterType, &afterCause, &afterFaces); err != nil {
		t.Fatalf("re-read chunk row: %v", err)
	}
	if beforeType != afterType || beforeCause != afterCause || string(beforeFaces) != string(afterFaces) {
		t.Fatal("chunk row changed across a second request for an already-created chunk")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1`, m.id).Scan(&count); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if count != 1 {
		t.Fatalf("chunk rows = %d, want 1", count)
	}
}

func TestEnsureChunkAt_Event(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 12)

	player := createOwner(t, pool, store.OwnerPlayer, 1001)
	chat := createOwner(t, pool, store.OwnerChat, -1001)
	by := Actor{PlayerID: &player, ChatID: &chat}

	ch := hexgrid.Chunk{Q: 3, R: -2}
	cell := m.Lattice().Center(ch)
	mp, err := m.EnsureChunkAt(ctx, cell, by)
	if err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}

	var count int
	var gotPlayer, gotChat, gotMaze *int64
	var gotDepth *int32
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT count(*), max(player_id), max(chat_id), max(maze_id), max(depth) FROM event WHERE type = $1 AND maze_id = $2`,
		store.EventChunkCreated, m.id,
	).Scan(&count, &gotPlayer, &gotChat, &gotMaze, &gotDepth); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("chunk_created events = %d, want 1", count)
	}
	if gotPlayer == nil || *gotPlayer != int64(player) {
		t.Fatalf("event player_id = %v, want %d", gotPlayer, player)
	}
	if gotChat == nil || *gotChat != int64(chat) {
		t.Fatalf("event chat_id = %v, want %d", gotChat, chat)
	}
	if gotMaze == nil || *gotMaze != m.id {
		t.Fatalf("event maze_id = %v, want %d", gotMaze, m.id)
	}
	if gotDepth != nil {
		t.Fatalf("event depth = %v, want null", *gotDepth)
	}

	if err := pool.QueryRow(ctx, `SELECT payload FROM event WHERE type = $1 AND maze_id = $2`, store.EventChunkCreated, m.id).Scan(&payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded["chunk_type"] != string(ChunkTypeFabric) || decoded["creation_cause"] != string(CreationCauseExplorer) {
		t.Fatalf("payload = %v, want chunk_type/creation_cause fabric/explorer", decoded)
	}
	if int32(decoded["generation_version"].(float64)) != mp.Version() {
		t.Fatalf("payload generation_version = %v, want %d", decoded["generation_version"], mp.Version())
	}
	if int32(decoded["q"].(float64)) != ch.Q || int32(decoded["r"].(float64)) != ch.R {
		t.Fatalf("payload coordinate = (%v,%v), want (%d,%d)", decoded["q"], decoded["r"], ch.Q, ch.R)
	}
	if _, ok := decoded["spiral_index"]; ok {
		t.Fatal("payload carries spiral_index for a fabric chunk")
	}
	if _, ok := decoded["ring"]; ok {
		t.Fatal("payload carries ring for a fabric chunk")
	}
}

func TestEnsureChunkAt_MovesNoBalance(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 13)

	var before int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM journal_entry`).Scan(&before); err != nil {
		t.Fatalf("read journal mark: %v", err)
	}

	if _, err := m.EnsureChunkAt(ctx, hexgrid.Coord{}, Actor{}); err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}

	var after int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM journal_entry`).Scan(&after); err != nil {
		t.Fatalf("read journal mark: %v", err)
	}
	if after != before {
		t.Fatalf("journal_entry max id moved from %d to %d — a chunk creation must post no journal entry", before, after)
	}
}

func TestEnsureChunkAt_MazeMissing(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 14)

	if _, err := pool.Exec(ctx, `DELETE FROM maze WHERE id = $1`, m.id); err != nil {
		t.Fatalf("delete maze row: %v", err)
	}

	_, err := m.EnsureChunkAt(ctx, hexgrid.Coord{}, Actor{})
	if !errors.Is(err, ErrMazeMissing) {
		t.Fatalf("err = %v, want ErrMazeMissing", err)
	}
}

// TestEnsureChunkAt_BudgetIsABound is subtask 3's Test Design bullet on
// Spec.CreateBudget, deferred here because it drives EnsureChunkAt,
// which does not exist before this subtask: with a pool sized to one
// connection and a caller transaction already holding it, EnsureChunkAt
// must return ErrCreateBudget wrapping context.DeadlineExceeded within
// the budget rather than blocking — the outer 5s bound is this test's
// own safety net, not the property under test, so a budget that was
// never applied fails loudly here instead of hanging the suite.
func TestEnsureChunkAt_BudgetIsABound(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	cfg := testdb.Schema(t)
	cfg.MaxConns = 1

	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	m, err := Open(ctx, pool, Spec{
		Biome: "budget_test", Season: 1, Seed: 99,
		Generation: testParams(), GateSpacing: 1, CreateBudget: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder transaction: %v", err)
	}
	t.Cleanup(func() { _ = holder.Rollback(ctx) })

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = m.EnsureChunkAt(callCtx, hexgrid.Coord{}, Actor{})
	if !errors.Is(err, ErrCreateBudget) {
		t.Fatalf("err = %v, want ErrCreateBudget", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
}
