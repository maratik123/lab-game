package store

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestMigrate_shape_and_seeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	// Base tables only (plus goose's own version table) — information_schema.tables
	// with no table_type filter also lists views, and the migration set now
	// creates some, so this assertion is scoped to base tables and the
	// views get their own exact-set assertion in a sibling test file.
	rows, err := pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(tables)
	want := []string{
		"account", "account_balance", "account_definition",
		"chat_membership", "chat_presence",
		"chunk",
		"deferred_task", "event", "event_type_definition", "goose_db_version",
		"ingest_dead_update", "ingest_offset",
		"item", "item_movement",
		"journal_entry", "manual_correction",
		"maze",
		"node_discovery",
		"owner", "player_operation", "posting", "process_liveness", "recurrent_task", "scheduled_task",
		"scope", "scope_definition",
	}
	sort.Strings(want)
	if !slices.Equal(tables, want) {
		t.Fatalf("tables = %v, want %v", tables, want)
	}

	// Enums.
	for enum, wantMembers := range map[string][]string{
		"owner_kind":           {"world", "player", "chat"},
		"ledger_kind":          {"money", "experience", "slots", "weight", "spun_sugar", "pastel_fleece", "glitter_dust"},
		"operation_source":     {"telegram"},
		"scheduled_task_state": {"pending", "dead"},
		"event_volume_class":   {"low_volume", "high_volume"},
		"capacity_role":        {"free", "used"},
	} {
		var members []string
		if err := pool.QueryRow(ctx, `SELECT enum_range(NULL::`+enum+`)::text[]`).Scan(&members); err != nil {
			t.Fatalf("enum_range(%s): %v", enum, err)
		}
		got := append([]string(nil), members...)
		wantSorted := append([]string(nil), wantMembers...)
		sort.Strings(got)
		sort.Strings(wantSorted)
		if !slices.Equal(got, wantSorted) {
			t.Fatalf("%s members = %v, want (as set) %v", enum, members, wantMembers)
		}
	}

	// Seeds.
	var ownerCount, scopeCount, accountCount, balanceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM owner`).Scan(&ownerCount); err != nil {
		t.Fatalf("count owner: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scope`).Scan(&scopeCount); err != nil {
		t.Fatalf("count scope: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account`).Scan(&accountCount); err != nil {
		t.Fatalf("count account: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_balance`).Scan(&balanceCount); err != nil {
		t.Fatalf("count account_balance: %v", err)
	}
	if ownerCount != 1 || scopeCount != 1 || accountCount != 6 || balanceCount != 0 {
		t.Fatalf("seed counts = owner:%d scope:%d account:%d balance:%d, want 1/1/6/0",
			ownerCount, scopeCount, accountCount, balanceCount)
	}

	var wKind string
	var wTelegramID *int64
	if err := pool.QueryRow(ctx, `SELECT kind, telegram_id FROM owner WHERE id = 1`).Scan(&wKind, &wTelegramID); err != nil {
		t.Fatalf("seeded owner: %v", err)
	}
	if wKind != "world" || wTelegramID != nil {
		t.Fatalf("seeded owner = kind:%s telegram_id:%v, want world/nil", wKind, wTelegramID)
	}

	// Identity sequences positioned past the seeds: each was setval'd with
	// is_called = true, so last_value + 1 is the next id nextval() returns.
	for _, tc := range []struct {
		table string
		want  int64
	}{
		{"owner", 2}, {"scope", 2}, {"account", 7},
	} {
		var lastValue int64
		if err := pool.QueryRow(ctx,
			`SELECT last_value FROM pg_sequences
			 WHERE schemaname = current_schema()
			   AND sequencename = substring(pg_get_serial_sequence($1, 'id') from '[^.]+$')`,
			tc.table,
		).Scan(&lastValue); err != nil {
			t.Fatalf("sequence for %s: %v", tc.table, err)
		}
		if next := lastValue + 1; next != tc.want {
			t.Fatalf("%s identity sequence next = %d, want %d", tc.table, next, tc.want)
		}
	}
}

func TestMigrate_noop_reapply(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	if err := Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM goose_db_version`).Scan(&count); err != nil {
		t.Fatalf("count goose_db_version: %v", err)
	}
	if count != 11 {
		t.Fatalf("goose_db_version rows = %d, want 11", count)
	}
}

var (
	downRe        = regexp.MustCompile(`(?i)--\s*\+goose\s+Down`)
	renameValueRe = regexp.MustCompile(`(?i)RENAME\s+VALUE`)
	dropValueRe   = regexp.MustCompile(`(?i)DROP\s+VALUE`)
	createTrigRe  = regexp.MustCompile(`(?i)CREATE\s+TRIGGER`)
	createRuleRe  = regexp.MustCompile(`(?i)CREATE\s+RULE`)
	grantRe       = regexp.MustCompile(`(?i)\bGRANT\b`)
	upRe          = regexp.MustCompile(`(?i)--\s*\+goose\s+Up`)
	addValueRe    = regexp.MustCompile(`(?i)ADD\s+VALUE`)
	createTableRe = regexp.MustCompile(`(?i)CREATE\s+TABLE`)
	insertRe      = regexp.MustCompile(`(?i)\bINSERT\b`)
	updateRe      = regexp.MustCompile(`(?i)\bUPDATE\b`)
)

func TestMigrate_hygiene(t *testing.T) {
	t.Parallel()

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no migration files found")
	}

	for _, entry := range entries {
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()

			content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
			if err != nil {
				t.Fatalf("read %s: %v", entry.Name(), err)
			}
			text := string(content)

			if !upRe.MatchString(text) {
				t.Errorf("%s: missing -- +goose Up", entry.Name())
			}
			if downRe.MatchString(text) {
				t.Errorf("%s: contains -- +goose Down", entry.Name())
			}
			if renameValueRe.MatchString(text) {
				t.Errorf("%s: contains RENAME VALUE", entry.Name())
			}
			if dropValueRe.MatchString(text) {
				t.Errorf("%s: contains DROP VALUE", entry.Name())
			}
			if createTrigRe.MatchString(text) {
				t.Errorf("%s: contains CREATE TRIGGER", entry.Name())
			}
			if createRuleRe.MatchString(text) {
				t.Errorf("%s: contains CREATE RULE", entry.Name())
			}
			if grantRe.MatchString(text) {
				t.Errorf("%s: contains GRANT", entry.Name())
			}
			if addValueRe.MatchString(text) {
				if createTableRe.MatchString(text) || insertRe.MatchString(text) || updateRe.MatchString(text) {
					t.Errorf("%s: an ADD VALUE file must do nothing else (CREATE TABLE/INSERT/UPDATE found)", entry.Name())
				}
			}
		})
	}
}

func TestMigrate_indexes_constraints_and_column_types(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	// Every index the DDL names must exist in the test schema.
	rows, err := pool.Query(ctx, `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema()`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for _, want := range []string{
		"owner_kind_telegram_id_key", "owner_single_world_key",
		"scope_singleton_key", "scope_definition_idx",
		"account_scope_definition_key", "account_definition_idx",
		"journal_entry_player_operation_key", "journal_entry_manual_correction_key", "journal_entry_ts_idx",
		"posting_journal_entry_idx", "posting_account_idx",
		"scheduled_task_identity_key", "scheduled_task_due_idx",
		"journal_entry_deferred_task_key", "journal_entry_recurrent_task_key",
		"event_type_ts_idx", "event_player_idx", "event_chat_idx", "event_ts_idx",
		"journal_entry_event_key",
		"account_definition_capacity_role_key",
		"item_movement_successor_key", "item_movement_journal_entry_idx",
		"item_movement_from_holder_idx", "item_movement_to_holder_idx",
	} {
		if !found[want] {
			t.Errorf("index %s is missing (have %v)", want, found)
		}
	}

	// CHECK constraints carry the definitions the tests rely on.
	for name, wantSub := range map[string]string{
		"account_balance_nonnegative": "balance >= 0",
		"posting_amount_nonzero":      "amount <> 0",
		// Strengthened past a bare "num_nonnulls" substring (which the
		// pre-arc-extension form already satisfied): this names every one of
		// the five arc columns, so a re-added CHECK that omits event_id fails.
		"journal_entry_exactly_one_basis":     "num_nonnullsplayer_operation_id, manual_correction_id, deferred_task_id, recurrent_task_id, event_id",
		"scheduled_task_type_nonempty":        "type <> ''",
		"scheduled_task_failures_nonnegative": "consecutive_failures >= 0",
		"item_movement_holders_differ":        "from_holder_id <> to_holder_id",
		"item_movement_genesis_from_world":    "prev_movement_id is not null or from_holder_id = 1",
	} {
		var def string
		err := pool.QueryRow(ctx,
			`SELECT pg_get_constraintdef(c.oid) FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
			 WHERE c.conname = $1 AND n.nspname = current_schema()`, name).Scan(&def)
		if err != nil {
			t.Fatalf("constraint %s: %v", name, err)
		}
		// pg_get_constraintdef renders casts and parentheses
		// ("CHECK ((balance >= (0)::numeric))"); compare the bare expression.
		bare := strings.NewReplacer("(", "", ")", "", "::numeric", "").Replace(strings.ToLower(def))
		if !strings.Contains(bare, wantSub) {
			t.Errorf("constraint %s = %q, want it to contain %q", name, def, wantSub)
		}
	}

	// Money columns are numeric(30,5).
	for _, col := range [][2]string{{"posting", "amount"}, {"account_balance", "balance"}} {
		var typ string
		var prec, scale int
		err := pool.QueryRow(ctx,
			`SELECT data_type, numeric_precision, numeric_scale FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, col[0], col[1]).Scan(&typ, &prec, &scale)
		if err != nil {
			t.Fatalf("column %s.%s: %v", col[0], col[1], err)
		}
		if typ != "numeric" || prec != 30 || scale != 5 {
			t.Errorf("%s.%s = %s(%d,%d), want numeric(30,5)", col[0], col[1], typ, prec, scale)
		}
	}
}

// TestMigrate_scheduledTaskShape asserts the schema's deliberately-absent
// columns (no execution marker, no heartbeat, no "completed" state value)
// and the identity index's two-predicate scope. Neither the index
// presence loop nor the CHECK substring map above can fail on a shape that
// merely adds these things back, so this test is what actually holds that
// property.
func TestMigrate_scheduledTaskShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'scheduled_task'`)
	if err != nil {
		t.Fatalf("list scheduled_task columns: %v", err)
	}
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(cols)
	want := []string{
		"consecutive_failures", "created_at", "id", "instance_key",
		"last_error", "payload", "run_at", "state", "type",
	}
	sort.Strings(want)
	if !slices.Equal(cols, want) {
		t.Fatalf("scheduled_task columns = %v, want %v (AC31: no execution marker, heartbeat, or executed_at)", cols, want)
	}

	var enumMembers []string
	if err := pool.QueryRow(ctx, `SELECT enum_range(NULL::scheduled_task_state)::text[]`).Scan(&enumMembers); err != nil {
		t.Fatalf("enum_range(scheduled_task_state): %v", err)
	}
	sort.Strings(enumMembers)
	if !slices.Equal(enumMembers, []string{"dead", "pending"}) {
		t.Fatalf("scheduled_task_state members = %v, want exactly pending/dead (AC31: no completed state)", enumMembers)
	}

	var pred string
	if err := pool.QueryRow(ctx,
		`SELECT pg_get_expr(indpred, indrelid) FROM pg_index i JOIN pg_class c ON c.oid = i.indrelid
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = current_schema() AND c.relname = 'scheduled_task' AND i.indexrelid::regclass::text = 'scheduled_task_identity_key'`,
	).Scan(&pred); err != nil {
		t.Fatalf("scheduled_task_identity_key predicate: %v", err)
	}
	lower := strings.ToLower(pred)
	if !strings.Contains(lower, "instance_key is not null") || !strings.Contains(lower, "state = 'pending'::scheduled_task_state") {
		t.Fatalf("scheduled_task_identity_key predicate = %q, want both instance_key IS NOT NULL and state = 'pending' (AC29, D5)", pred)
	}
}

// TestMigrate_eventShape asserts the exact column enumeration for event:
// the table-list assertion above only proves the table exists, not that its
// shape matches, and no other test pins the column set.
func TestMigrate_eventShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'event'`)
	if err != nil {
		t.Fatalf("list event columns: %v", err)
	}
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(cols)
	want := []string{"chat_id", "depth", "id", "maze_id", "payload", "player_id", "ts", "type"}
	sort.Strings(want)
	if !slices.Equal(cols, want) {
		t.Fatalf("event columns = %v, want %v (AC1)", cols, want)
	}

	// Nullability: only type, payload and ts are NOT NULL.
	nrows, err := pool.Query(ctx,
		`SELECT column_name, is_nullable FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'event'`)
	if err != nil {
		t.Fatalf("list event nullability: %v", err)
	}
	nullable := map[string]bool{}
	for nrows.Next() {
		var name, isNullable string
		if err := nrows.Scan(&name, &isNullable); err != nil {
			t.Fatalf("scan nullability: %v", err)
		}
		nullable[name] = isNullable == "YES"
	}
	if err := nrows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	wantNotNull := []string{"id", "type", "payload", "ts"}
	for _, col := range wantNotNull {
		if nullable[col] {
			t.Errorf("event.%s is nullable, want NOT NULL", col)
		}
	}
	for _, col := range []string{"player_id", "chat_id", "maze_id", "depth"} {
		if !nullable[col] {
			t.Errorf("event.%s is NOT NULL, want nullable", col)
		}
	}

	// The identity flavour is ALWAYS: event is seeded with nothing.
	var identityGeneration string
	if err := pool.QueryRow(ctx,
		`SELECT identity_generation FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = 'event' AND column_name = 'id'`,
	).Scan(&identityGeneration); err != nil {
		t.Fatalf("identity_generation: %v", err)
	}
	if identityGeneration != "ALWAYS" {
		t.Fatalf("event.id identity_generation = %s, want ALWAYS", identityGeneration)
	}

	// event.type's foreign key is named, so the write API can map the
	// SQLSTATE 23503 violation of exactly this constraint to
	// ErrUnknownEventType.
	var fkExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
			WHERE c.conname = 'event_type_fkey' AND c.contype = 'f' AND n.nspname = current_schema()
		)`,
	).Scan(&fkExists); err != nil {
		t.Fatalf("event_type_fkey exists: %v", err)
	}
	if !fkExists {
		t.Fatalf("named foreign key event_type_fkey is missing")
	}
}
