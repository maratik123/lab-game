package store

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fkRow struct {
	conname string
	relid   int64
	cols    []int16
}

type idxRow struct {
	relid int64
	pred  *string
	cols  []int16 // in ordinal order, lower bound 0 per pg_index.indkey
}

// uncoveredFKs returns the names of every foreign key in the current schema
// (contype = 'f') whose referencing columns are not covered — as a set, by
// the leading columns of some index on the same relation whose predicate is
// either absent or an "IS NOT NULL" partial predicate.
func uncoveredFKs(t *testing.T, ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
},
) []string {
	t.Helper()

	fkRows, err := q.Query(ctx, `
		SELECT conname, conrelid::bigint, conkey
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		WHERE c.contype = 'f' AND n.nspname = current_schema()
	`)
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	var fks []fkRow
	for fkRows.Next() {
		var r fkRow
		if err := fkRows.Scan(&r.conname, &r.relid, &r.cols); err != nil {
			t.Fatalf("scan fk row: %v", err)
		}
		fks = append(fks, r)
	}
	if err := fkRows.Err(); err != nil {
		t.Fatalf("fk rows: %v", err)
	}
	fkRows.Close()

	idxByRelid := map[int64][]idxRow{}
	idxRows, err := q.Query(ctx, `
		SELECT i.indrelid::bigint, pg_get_expr(i.indpred, i.indrelid), i.indkey::int2[]
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema()
	`)
	if err != nil {
		t.Fatalf("query pg_index: %v", err)
	}
	for idxRows.Next() {
		var r idxRow
		if err := idxRows.Scan(&r.relid, &r.pred, &r.cols); err != nil {
			t.Fatalf("scan idx row: %v", err)
		}
		idxByRelid[r.relid] = append(idxByRelid[r.relid], r)
	}
	if err := idxRows.Err(); err != nil {
		t.Fatalf("idx rows: %v", err)
	}
	idxRows.Close()

	var uncovered []string
	for _, fk := range fks {
		want := sortedCopy(fk.cols)
		covered := false
		for _, idx := range idxByRelid[fk.relid] {
			if idx.pred != nil && !strings.Contains(strings.ToUpper(*idx.pred), "IS NOT NULL") {
				continue
			}
			if len(idx.cols) < len(want) {
				continue
			}
			leading := sortedCopy(idx.cols[:len(want)])
			if slices.Equal(leading, want) {
				covered = true
				break
			}
		}
		if !covered {
			uncovered = append(uncovered, fk.conname)
		}
	}
	sort.Strings(uncovered)
	return uncovered
}

func sortedCopy(s []int16) []int16 {
	out := append([]int16(nil), s...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
