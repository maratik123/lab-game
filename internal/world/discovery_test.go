package world

import (
	"errors"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/hexgrid"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestRecordDiscovery_Basic(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 300)
	player := createOwner(t, pool, store.OwnerPlayer, 6001)
	cell := hexgrid.Coord{Q: 4, R: -2}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := m.RecordDiscovery(ctx, tx, cell, player); err != nil {
		t.Fatalf("RecordDiscovery: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var gotQ, gotR int32
	var gotPlayer int64
	var discoveredAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT q, r, player_id, discovered_at FROM node_discovery WHERE maze_id = $1 AND q = $2 AND r = $3 AND player_id = $4`,
		m.id, cell.Q, cell.R, player,
	).Scan(&gotQ, &gotR, &gotPlayer, &discoveredAt); err != nil {
		t.Fatalf("read discovery row: %v", err)
	}
	if gotQ != cell.Q || gotR != cell.R || gotPlayer != int64(player) {
		t.Fatalf("row = (%d,%d,%d), want (%d,%d,%d)", gotQ, gotR, gotPlayer, cell.Q, cell.R, player)
	}
	if discoveredAt.IsZero() {
		t.Fatal("discovered_at is zero")
	}
}

func TestRecordDiscovery_RepeatKeepsFirstTimestamp(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 301)
	player := createOwner(t, pool, store.OwnerPlayer, 6002)
	cell := hexgrid.Coord{Q: 1, R: 1}

	recordOnce := func() {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := m.RecordDiscovery(ctx, tx, cell, player); err != nil {
			t.Fatalf("RecordDiscovery: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	recordOnce()

	var first time.Time
	if err := pool.QueryRow(ctx,
		`SELECT discovered_at FROM node_discovery WHERE maze_id = $1 AND q = $2 AND r = $3 AND player_id = $4`,
		m.id, cell.Q, cell.R, player).Scan(&first); err != nil {
		t.Fatalf("read first discovered_at: %v", err)
	}

	recordOnce()

	var second time.Time
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT discovered_at FROM node_discovery WHERE maze_id = $1 AND q = $2 AND r = $3 AND player_id = $4`,
		m.id, cell.Q, cell.R, player).Scan(&second); err != nil {
		t.Fatalf("read second discovered_at: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM node_discovery WHERE maze_id = $1 AND q = $2 AND r = $3 AND player_id = $4`,
		m.id, cell.Q, cell.R, player).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
	if !first.Equal(second) {
		t.Fatalf("discovered_at changed on a repeat: %v -> %v", first, second)
	}
}

func TestRecordDiscovery_TwoPlayersMakeTwoRows(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 302)
	playerA := createOwner(t, pool, store.OwnerPlayer, 6003)
	playerB := createOwner(t, pool, store.OwnerPlayer, 6004)
	cell := hexgrid.Coord{Q: -1, R: -1}

	for _, p := range []store.OwnerID{playerA, playerB} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := m.RecordDiscovery(ctx, tx, cell, p); err != nil {
			t.Fatalf("RecordDiscovery(%d): %v", p, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM node_discovery WHERE maze_id = $1 AND q = $2 AND r = $3`,
		m.id, cell.Q, cell.R).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("rows = %d, want 2", count)
	}
}

func TestRecordDiscovery_NotAPlayer(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 303)
	chat := createOwner(t, pool, store.OwnerChat, -6005)
	cell := hexgrid.Coord{Q: 2, R: 2}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = m.RecordDiscovery(ctx, tx, cell, chat)
	if !errors.Is(err, ErrNotAPlayer) {
		t.Fatalf("err = %v, want ErrNotAPlayer", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM node_discovery WHERE maze_id = $1`, m.id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows = %d, want 0 (RecordDiscovery with a non-player owner must write nothing)", count)
	}
}

func TestRecordDiscovery_RolledBackLeavesNoRow(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	m := openTestMaze(t, pool, 304)
	player := createOwner(t, pool, store.OwnerPlayer, 6006)
	cell := hexgrid.Coord{Q: 7, R: -3}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := m.RecordDiscovery(ctx, tx, cell, player); err != nil {
		t.Fatalf("RecordDiscovery: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM node_discovery WHERE maze_id = $1`, m.id).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows = %d, want 0 (a rolled-back discovery must leave no row)", count)
	}
}
