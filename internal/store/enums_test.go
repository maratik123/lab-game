package store

import (
	"context"
	"slices"
	"sort"
	"testing"
)

func TestEnums_mirror_database(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	for _, tc := range []struct {
		enum string
		want []string
	}{
		{"owner_kind", stringsOf(ownerKinds)},
		{"ledger_kind", stringsOf(kinds)},
		{"operation_source", stringsOf(operationSources)},
		{"event_volume_class", stringsOf(eventVolumeClasses)},
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

func stringsOf[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

func TestCatalog_mirrors_database(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	rows, err := pool.Query(ctx, `SELECT id, code, owner_kind FROM scope_definition ORDER BY id`)
	if err != nil {
		t.Fatalf("query scope_definition: %v", err)
	}
	var got []ScopeDefinition
	for rows.Next() {
		var sd ScopeDefinition
		var ownerKind string
		if err := rows.Scan(&sd.ID, &sd.Code, &ownerKind); err != nil {
			t.Fatalf("scan scope_definition: %v", err)
		}
		sd.OwnerKind = OwnerKind(ownerKind)
		got = append(got, sd)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if !slices.Equal(got, scopeDefinitions) {
		t.Fatalf("scope_definition mirror = %+v, database = %+v", scopeDefinitions, got)
	}

	arows, err := pool.Query(ctx,
		`SELECT id, scope_definition_id, code, kind, controlled FROM account_definition ORDER BY id`)
	if err != nil {
		t.Fatalf("query account_definition: %v", err)
	}
	var gotAcc []AccountDefinition
	for arows.Next() {
		var ad AccountDefinition
		var kind string
		if err := arows.Scan(&ad.ID, &ad.ScopeDefinitionID, &ad.Code, &kind, &ad.Controlled); err != nil {
			t.Fatalf("scan account_definition: %v", err)
		}
		ad.Kind = Kind(kind)
		gotAcc = append(gotAcc, ad)
	}
	if err := arows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if !slices.Equal(gotAcc, accountDefinitions) {
		t.Fatalf("account_definition mirror = %+v, database = %+v", accountDefinitions, gotAcc)
	}

	// event_type_definition, read ordered by id — the column that exists
	// solely so this comparison can be element-for-element ordered rather
	// than set-wise: a type present on one side only, one whose class
	// differs, or a reordering all fail this equality.
	erows, err := pool.Query(ctx, `SELECT id, code, volume_class FROM event_type_definition ORDER BY id`)
	if err != nil {
		t.Fatalf("query event_type_definition: %v", err)
	}
	var gotEvents []EventTypeDefinition
	for erows.Next() {
		var ed EventTypeDefinition
		var code, volumeClass string
		if err := erows.Scan(&ed.ID, &code, &volumeClass); err != nil {
			t.Fatalf("scan event_type_definition: %v", err)
		}
		ed.Code = EventType(code)
		ed.VolumeClass = EventVolumeClass(volumeClass)
		gotEvents = append(gotEvents, ed)
	}
	if err := erows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if !slices.Equal(gotEvents, eventTypeDefinitions) {
		t.Fatalf("event_type_definition mirror = %+v, database = %+v", eventTypeDefinitions, gotEvents)
	}
}
