package world

import (
	"testing"

	"golang.org/x/sync/errgroup"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/maze"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

// TestRace_NeighboringCreations creates two adjacent chunks
// concurrently; once both return, their shared border must agree face
// for face. Repeated over every direction so the case does not rest on
// one.
func TestRace_NeighboringCreations(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		d    hexgrid.Direction
	}{
		{"E", hexgrid.DirE},
		{"NE", hexgrid.DirNE},
		{"NW", hexgrid.DirNW},
		{"W", hexgrid.DirW},
		{"SW", hexgrid.DirSW},
		{"SE", hexgrid.DirSE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			pool := storetest.Pool(t)
			m := openTestMaze(t, pool, 600+int64(tc.d))

			chA := hexgrid.Chunk{Q: 0, R: 0}
			chB := chA.Neighbor(tc.d)
			cellA := m.Lattice().Center(chA)
			cellB := m.Lattice().Center(chB)

			var mapA, mapB maze.Map
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				var err error
				mapA, err = m.EnsureChunkAt(gctx, cellA, Actor{})
				return err
			})
			g.Go(func() error {
				var err error
				mapB, err = m.EnsureChunkAt(gctx, cellB, Actor{})
				return err
			})
			if err := g.Wait(); err != nil {
				t.Fatalf("concurrent creation: %v", err)
			}

			borderAgrees(t, m.Lattice(), mapA, mapB, tc.d)
		})
	}
}

// TestRace_RepeatedAndRacingRequests drives many goroutines racing
// EnsureChunkAt for one absent chunk: they must leave exactly one row,
// exactly one event, and identical returned maps; a second fan-out
// against the now existing chunk must leave the row and the event
// count unchanged.
func TestRace_RepeatedAndRacingRequests(t *testing.T) {
	t.Parallel()

	const fanOut = 8

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 700)
	cell := hexgrid.Coord{Q: 12, R: -6}
	ch, _ := m.Lattice().Locate(cell)

	first := make([]maze.Map, fanOut)
	g, gctx := errgroup.WithContext(ctx)
	for i := range first {
		g.Go(func() error {
			var err error
			first[i], err = m.EnsureChunkAt(gctx, cell, Actor{})
			return err
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("racing creation fan-out: %v", err)
	}
	for i := 1; i < fanOut; i++ {
		mapsEqual(t, first[i], first[0])
	}

	var chunkCount, eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`, m.id, ch.Q, ch.R).Scan(&chunkCount); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if chunkCount != 1 {
		t.Fatalf("chunk rows = %d, want 1", chunkCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1 AND maze_id = $2`, store.EventChunkCreated, m.id).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("chunk_created events = %d, want 1", eventCount)
	}

	var beforeType, beforeCause string
	var beforeFaces []byte
	if err := pool.QueryRow(ctx, `SELECT chunk_type::text, creation_cause::text, faces FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, ch.Q, ch.R).Scan(&beforeType, &beforeCause, &beforeFaces); err != nil {
		t.Fatalf("read chunk row: %v", err)
	}

	second := make([]maze.Map, fanOut)
	g2, gctx2 := errgroup.WithContext(ctx)
	for i := range second {
		g2.Go(func() error {
			var err error
			second[i], err = m.EnsureChunkAt(gctx2, cell, Actor{})
			return err
		})
	}
	if err := g2.Wait(); err != nil {
		t.Fatalf("racing existing-chunk fan-out: %v", err)
	}
	for i := range second {
		mapsEqual(t, second[i], first[0])
	}

	var afterType, afterCause string
	var afterFaces []byte
	if err := pool.QueryRow(ctx, `SELECT chunk_type::text, creation_cause::text, faces FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, ch.Q, ch.R).Scan(&afterType, &afterCause, &afterFaces); err != nil {
		t.Fatalf("re-read chunk row: %v", err)
	}
	if beforeType != afterType || beforeCause != afterCause || string(beforeFaces) != string(afterFaces) {
		t.Fatal("chunk row changed across a racing fan-out against an already-created chunk")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1 AND maze_id = $2`, store.EventChunkCreated, m.id).Scan(&eventCount); err != nil {
		t.Fatalf("re-count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("chunk_created events after the second fan-out = %d, want still 1", eventCount)
	}
}

// TestRace_CallerRollbackLeavesChunkBehind checks that a chunk created from
// inside a caller's own transaction survives that caller's own work
// failing and rolling back — the case that also exercises the
// two-pool-connections-at-once shape, since the caller's transaction
// holds one connection while EnsureChunkAt's own creation transaction
// holds a second.
func TestRace_CallerRollbackLeavesChunkBehind(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 800)
	player := createOwner(t, pool, store.OwnerPlayer, 7001)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin caller transaction: %v", err)
	}

	cell := hexgrid.Coord{Q: 9, R: -4}
	if err := m.RecordDiscovery(ctx, tx, cell, player); err != nil {
		t.Fatalf("RecordDiscovery: %v", err)
	}

	ch, _ := m.Lattice().Locate(cell)
	if _, err := m.EnsureChunkAt(ctx, cell, Actor{PlayerID: &player}); err != nil {
		t.Fatalf("EnsureChunkAt: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback caller transaction: %v", err)
	}

	var discoveryCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM node_discovery WHERE maze_id = $1`, m.id).Scan(&discoveryCount); err != nil {
		t.Fatalf("count node_discovery: %v", err)
	}
	if discoveryCount != 0 {
		t.Fatalf("node_discovery rows = %d, want 0 (the caller's own work was rolled back)", discoveryCount)
	}

	var chunkCount, eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1 AND q = $2 AND r = $3`, m.id, ch.Q, ch.R).Scan(&chunkCount); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if chunkCount != 1 {
		t.Fatalf("chunk rows = %d, want 1 (the created chunk must survive the caller's rollback)", chunkCount)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1 AND maze_id = $2`, store.EventChunkCreated, m.id).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("chunk_created events = %d, want 1 (the event must survive the caller's rollback)", eventCount)
	}
}

// TestRace_ConcurrentActivations activates two different chats in one
// maze concurrently: they must yield two distinct gate chunks, each
// keeping the world's gap k from the other, over a table of k values,
// and each chat must end with exactly one gate row.
func TestRace_ConcurrentActivations(t *testing.T) {
	t.Parallel()

	for _, k := range []int{0, 1, 4} {
		t.Run(fmtK(k), func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			pool := storetest.Pool(t)
			m, err := Open(ctx, pool, Spec{
				Biome: "test_biome", Season: 1, Seed: int64(900 + k),
				Generation: testParams(), GateSpacing: k, CreateBudget: testCreateBudget,
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			chatA := createOwner(t, pool, store.OwnerChat, int64(-9500-k))
			chatB := createOwner(t, pool, store.OwnerChat, int64(-9600-k))

			var gateA, gateB hexgrid.Chunk
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				var err error
				gateA, err = m.ActivateChat(gctx, chatA, nil)
				return err
			})
			g.Go(func() error {
				var err error
				gateB, err = m.ActivateChat(gctx, chatB, nil)
				return err
			})
			if err := g.Wait(); err != nil {
				t.Fatalf("concurrent activation: %v", err)
			}

			if gateA == gateB {
				t.Fatalf("two chats took the same gate chunk %v", gateA)
			}
			if d := hexgrid.ChunkDistance(gateA, gateB); d < int64(k)+1 {
				t.Fatalf("gate distance = %d, want at least k+1 = %d", d, k+1)
			}

			for _, chat := range []store.OwnerID{chatA, chatB} {
				var count int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE maze_id = $1 AND gate_chat_id = $2`, m.id, chat).Scan(&count); err != nil {
					t.Fatalf("count gate rows for chat %d: %v", chat, err)
				}
				if count != 1 {
					t.Fatalf("gate rows for chat %d = %d, want 1", chat, count)
				}
			}
		})
	}
}
