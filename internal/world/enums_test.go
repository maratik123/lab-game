package world

import (
	"slices"
	"sort"
	"testing"

	"github.com/maratik123/lab-game/internal/maze"
	"github.com/maratik123/lab-game/internal/storetest"
)

func TestChunkType_MazeRoundTrip(t *testing.T) {
	t.Parallel()

	for _, mt := range []maze.ChunkType{maze.ChunkTypeFabric, maze.ChunkTypeGate} {
		t.Run(mt.String(), func(t *testing.T) {
			t.Parallel()
			wt, err := chunkTypeFromMaze(mt)
			if err != nil {
				t.Fatalf("chunkTypeFromMaze(%v): %v", mt, err)
			}
			back, err := wt.toMaze()
			if err != nil {
				t.Fatalf("toMaze(%v): %v", wt, err)
			}
			if back != mt {
				t.Fatalf("round trip = %v, want %v", back, mt)
			}
		})
	}

	// Every known world ChunkType round-trips too, driven from the
	// package's own member table rather than trusted to the exhaustive
	// linter's default-signifies-exhaustive posture.
	for _, wt := range chunkTypes {
		mt, err := wt.toMaze()
		if err != nil {
			t.Fatalf("toMaze(%v): %v", wt, err)
		}
		back, err := chunkTypeFromMaze(mt)
		if err != nil {
			t.Fatalf("chunkTypeFromMaze(%v): %v", mt, err)
		}
		if back != wt {
			t.Fatalf("round trip = %v, want %v", back, wt)
		}
	}
}

func TestChunkType_UnrecognisedRefused(t *testing.T) {
	t.Parallel()

	if _, err := ChunkType("bogus").toMaze(); err == nil {
		t.Fatal("toMaze on an unrecognised ChunkType: want an error, got nil")
	}
	if _, err := chunkTypeFromMaze(maze.ChunkType(99)); err == nil {
		t.Fatal("chunkTypeFromMaze on an unrecognised maze.ChunkType: want an error, got nil")
	}
}

// TestEnums_mirror_database reads the database enums' own member sets
// and compares each with this package's member table, in the shape the
// ledger's own enum mirror test uses.
func TestEnums_mirror_database(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	pool := storetest.Pool(t)

	for _, tc := range []struct {
		enum string
		want []string
	}{
		{"chunk_type", stringsOfChunkTypes(chunkTypes)},
		{"chunk_creation_cause", stringsOfCauses(creationCauses)},
	} {
		var members []string
		if err := pool.QueryRow(ctx, `SELECT enum_range(NULL::`+tc.enum+`)::text[]`).Scan(&members); err != nil {
			t.Fatalf("enum_range(%s): %v", tc.enum, err)
		}
		got := append([]string(nil), members...)
		want := append([]string(nil), tc.want...)
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s mirror = %v, database = %v", tc.enum, want, members)
		}
	}
}

func stringsOfChunkTypes(vs []ChunkType) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

func stringsOfCauses(vs []CreationCause) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}
