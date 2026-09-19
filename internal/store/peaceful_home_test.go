package store

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mazeTouchingRelations returns the sorted names of every ordinary base
// table (relkind = 'r') in pool's own schema that either declares a
// column referencing maze (id) or declares a column named maze_id — read
// from the catalogs, never from a hard-coded table list, so a table
// added later is in scope without a change here. Both arms matter: a
// hostile mechanic's table is as likely to carry a surrogate id primary
// key (caught by the FK arm) as to name its column with no foreign key
// at all, the shape event already ships (caught by the name arm).
func mazeTouchingRelations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r'
		  AND (
		    EXISTS (
		      SELECT 1 FROM pg_constraint con
		      JOIN pg_class fc ON fc.oid = con.confrelid
		      WHERE con.conrelid = c.oid AND con.contype = 'f' AND fc.relname = 'maze'
		    )
		    OR EXISTS (
		      SELECT 1 FROM pg_attribute a
		      WHERE a.attrelid = c.oid AND a.attname = 'maze_id' AND NOT a.attisdropped
		    )
		  )
	`)
	if err != nil {
		t.Fatalf("query maze-touching relations: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan relation name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(got)
	return got
}

// fkPairsOnRelations returns "relation.column" for every foreign-key
// column in pool's own schema that references targetTable (id), one
// entry per referencing column, restricted to the relations named in
// onlyRelations when it is non-empty.
func fkPairsOnRelations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, targetTable string, onlyRelations []string) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.relname, a.attname
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_class fc ON fc.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)
		WHERE con.contype = 'f' AND fc.relname = $1 AND n.nspname = current_schema() AND c.relkind = 'r'
	`, targetTable)
	if err != nil {
		t.Fatalf("query fk pairs referencing %s: %v", targetTable, err)
	}
	defer rows.Close()
	only := make(map[string]bool, len(onlyRelations))
	for _, r := range onlyRelations {
		only[r] = true
	}
	var got []string
	for rows.Next() {
		var relname, attname string
		if err := rows.Scan(&relname, &attname); err != nil {
			t.Fatalf("scan fk pair: %v", err)
		}
		if len(onlyRelations) > 0 && !only[relname] {
			continue
		}
		got = append(got, relname+"."+attname)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(got)
	return got
}

// scopeOwnMazeOrCellColumns reports whether the scope relation itself
// declares a column named maze_id, q or r — the other half of Half A's
// rule: scope carries no maze or cell column of its own.
func scopeOwnMazeOrCellColumns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'scope'
		  AND column_name IN ('maze_id', 'q', 'r')
	`)
	if err != nil {
		t.Fatalf("query scope columns: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return got
}

// peacefulHomeAllowList is the reviewed allow list for owner (id)
// references on a maze-touching relation: every row states which owner
// kind the column holds and what keeps a home out of it.
//
//   - chunk.gate_chat_id — a chat's, but its GATE, which this task places
//     none of.
//   - node_discovery.player_id — a player's, kept so by the discovery
//     writer's own kind filter rather than by a constraint.
//   - event.player_id — the player an event is about, not a position.
//   - event.chat_id — the chat an event is about, not a position.
var peacefulHomeAllowList = []string{
	"chunk.gate_chat_id",
	"event.chat_id",
	"event.player_id",
	"node_discovery.player_id",
}

// TestPeacefulHome_shippedSchema asserts both halves of the guard on the
// migrated tree: the maze-touching set is exactly chunk, node_discovery
// and event; Half A finds no scope (id) reference on any of them, and
// scope itself declares no maze or cell column; Half B finds exactly the
// four allow-listed pairs and no other owner (id) reference on a
// maze-touching relation.
func TestPeacefulHome_shippedSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	touching := mazeTouchingRelations(t, ctx, pool)
	wantTouching := []string{"chunk", "event", "node_discovery"}
	if !slices.Equal(touching, wantTouching) {
		t.Fatalf("maze-touching relations = %v, want %v (an arm that silently matched nothing would make the whole guard a claim about its own extractor)", touching, wantTouching)
	}

	t.Run("half_a_no_scope_reference", func(t *testing.T) {
		t.Parallel()
		scopeRefs := fkPairsOnRelations(t, ctx, pool, "scope", touching)
		if len(scopeRefs) != 0 {
			t.Fatalf("maze-touching relations carrying a scope (id) reference = %v, want none", scopeRefs)
		}
		ownCols := scopeOwnMazeOrCellColumns(t, ctx, pool)
		if len(ownCols) != 0 {
			t.Fatalf("scope's own maze/cell columns = %v, want none", ownCols)
		}
	})

	t.Run("half_b_owner_allow_list_exact", func(t *testing.T) {
		t.Parallel()
		ownerRefs := fkPairsOnRelations(t, ctx, pool, "owner", touching)
		want := append([]string(nil), peacefulHomeAllowList...)
		sort.Strings(want)
		if !slices.Equal(ownerRefs, want) {
			t.Fatalf("owner (id) references on maze-touching relations = %v, want exactly the allow list %v (a pair with no row, or an allow-list row matching no pair, is a failure)", ownerRefs, want)
		}
	})
}

// TestPeacefulHome_halfAControl proves Half A actually fails a relation
// it should: a table sharing chunk/node_discovery's own primary-key
// shape — PRIMARY KEY (maze_id, q, r) — plus a scope (id) reference.
func TestPeacefulHome_halfAControl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE test_home_leak (
		    maze_id  bigint NOT NULL REFERENCES maze (id),
		    q        integer NOT NULL,
		    r        integer NOT NULL,
		    scope_id bigint NOT NULL REFERENCES scope (id),
		    PRIMARY KEY (maze_id, q, r)
		)
	`); err != nil {
		t.Fatalf("create control relation: %v", err)
	}

	touching := mazeTouchingRelations(t, ctx, pool)
	if !slices.Contains(touching, "test_home_leak") {
		t.Fatalf("maze-touching relations = %v, want test_home_leak among them", touching)
	}
	scopeRefs := fkPairsOnRelations(t, ctx, pool, "scope", touching)
	want := "test_home_leak.scope_id"
	if !slices.Contains(scopeRefs, want) {
		t.Fatalf("scope (id) references on maze-touching relations = %v, want %q among them (Half A must name the offending relation and column)", scopeRefs, want)
	}
}

// TestPeacefulHome_halfBControl proves Half B actually fails a relation
// it should: an owner (id) reference on a maze-touching relation with no
// allow-list row.
func TestPeacefulHome_halfBControl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE test_owner_leak (
		    maze_id  bigint NOT NULL REFERENCES maze (id),
		    q        integer NOT NULL,
		    r        integer NOT NULL,
		    owner_id bigint NOT NULL REFERENCES owner (id),
		    PRIMARY KEY (maze_id, q, r)
		)
	`); err != nil {
		t.Fatalf("create control relation: %v", err)
	}

	touching := mazeTouchingRelations(t, ctx, pool)
	ownerRefs := fkPairsOnRelations(t, ctx, pool, "owner", touching)
	want := "test_owner_leak.owner_id"
	if !slices.Contains(ownerRefs, want) {
		t.Fatalf("owner (id) references on maze-touching relations = %v, want %q among them", ownerRefs, want)
	}
	for _, allowed := range peacefulHomeAllowList {
		if allowed == want {
			t.Fatalf("test setup error: %q collides with a real allow-list row", want)
		}
	}
}

// TestPeacefulHome_surrogateKeyControls is the width proof: a relation
// need not share chunk/node_discovery's own (maze_id, q, r) primary key
// shape to be maze-touching, and it need not carry a real foreign key on
// maze_id either — event already ships without one. Both variants below
// carry a surrogate id primary key, ordinary maze_id/q/r columns and a
// scope (id) reference; under a primary-key-shaped definition or an
// FK-only definition, either would escape Half A entirely.
func TestPeacefulHome_surrogateKeyControls(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		ddl      string
		relation string
	}{
		{
			name:     "surrogate_pk_with_fk",
			relation: "test_surrogate_fk",
			ddl: `
				CREATE TABLE test_surrogate_fk (
				    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				    maze_id  bigint NOT NULL REFERENCES maze (id),
				    q        integer NOT NULL,
				    r        integer NOT NULL,
				    scope_id bigint NOT NULL REFERENCES scope (id)
				)
			`,
		},
		{
			name:     "surrogate_pk_no_fk",
			relation: "test_surrogate_nofk",
			ddl: `
				CREATE TABLE test_surrogate_nofk (
				    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
				    maze_id  bigint,
				    q        integer,
				    r        integer,
				    scope_id bigint NOT NULL REFERENCES scope (id)
				)
			`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			pool := newStore(t)

			if _, err := pool.Exec(ctx, tc.ddl); err != nil {
				t.Fatalf("create control relation: %v", err)
			}

			touching := mazeTouchingRelations(t, ctx, pool)
			if !slices.Contains(touching, tc.relation) {
				t.Fatalf("maze-touching relations = %v, want %s among them (a surrogate-key or FK-less relation must still be caught)", touching, tc.relation)
			}
			scopeRefs := fkPairsOnRelations(t, ctx, pool, "scope", touching)
			want := fmt.Sprintf("%s.scope_id", tc.relation)
			if !slices.Contains(scopeRefs, want) {
				t.Fatalf("scope (id) references on maze-touching relations = %v, want %q among them", scopeRefs, want)
			}
		})
	}
}
