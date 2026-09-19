package world

import (
	"errors"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/storetest"
)

func TestOpen_HappyPath(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	spec := Spec{Biome: "cotton_candy", Season: 1, Seed: 42, Generation: testParams(), GateSpacing: 3, CreateBudget: time.Second}

	first, err := Open(ctx, pool, spec)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	second, err := Open(ctx, pool, spec)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if first.id != second.id {
		t.Fatalf("maze id = %d, want %d (a second Open of the same spec)", second.id, first.id)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM maze`).Scan(&count); err != nil {
		t.Fatalf("count maze: %v", err)
	}
	if count != 1 {
		t.Fatalf("maze rows = %d, want 1", count)
	}

	var biome string
	var seed int64
	var season int32
	if err := pool.QueryRow(ctx, `SELECT biome, world_seed, season FROM maze WHERE id = $1`, first.id).
		Scan(&biome, &seed, &season); err != nil {
		t.Fatalf("read maze row: %v", err)
	}
	if biome != spec.Biome || seed != spec.Seed || season != spec.Season {
		t.Fatalf("stored row = (%q, %d, %d), want (%q, %d, %d)", biome, seed, season, spec.Biome, spec.Seed, spec.Season)
	}
}

func TestOpen_SeedMismatch(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	spec := Spec{Biome: "cotton_candy", Season: 1, Seed: 42, Generation: testParams(), GateSpacing: 3, CreateBudget: time.Second}

	if _, err := Open(ctx, pool, spec); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	mismatched := spec
	mismatched.Seed = 43
	_, err := Open(ctx, pool, mismatched)
	if !errors.Is(err, ErrSeedMismatch) {
		t.Fatalf("err = %v, want ErrSeedMismatch", err)
	}

	var seed int64
	if err := pool.QueryRow(ctx, `SELECT world_seed FROM maze WHERE biome = $1 AND season = $2`, spec.Biome, spec.Season).
		Scan(&seed); err != nil {
		t.Fatalf("read maze row: %v", err)
	}
	if seed != spec.Seed {
		t.Fatalf("stored seed = %d, want unchanged %d", seed, spec.Seed)
	}
}

func TestOpen_InvalidGeneration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)
	bad := testParams()
	bad.Radius = 0 // below the generator's own minimum

	_, err := Open(ctx, pool, Spec{Biome: "cotton_candy", Season: 1, Seed: 1, Generation: bad, CreateBudget: time.Second})
	if err == nil {
		t.Fatal("Open with an invalid Generation: want an error, got nil")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM maze`).Scan(&count); err != nil {
		t.Fatalf("count maze: %v", err)
	}
	if count != 0 {
		t.Fatalf("maze rows = %d, want 0 (refused before any statement)", count)
	}
}

func TestOpen_InvalidCreateBudget(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)

	for _, budget := range []time.Duration{0, -time.Second} {
		_, err := Open(ctx, pool, Spec{Biome: "cotton_candy", Season: 1, Seed: 1, Generation: testParams(), CreateBudget: budget})
		if err == nil {
			t.Fatalf("Open with CreateBudget %v: want an error, got nil", budget)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM maze`).Scan(&count); err != nil {
		t.Fatalf("count maze: %v", err)
	}
	if count != 0 {
		t.Fatalf("maze rows = %d, want 0 (refused before any statement)", count)
	}
}
