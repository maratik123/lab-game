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

	// Tables (plus goose's own version table).
	rows, err := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()`)
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
		"goose_db_version", "journal_entry", "manual_correction",
		"owner", "player_operation", "posting", "scope", "scope_definition",
	}
	sort.Strings(want)
	if !slices.Equal(tables, want) {
		t.Fatalf("tables = %v, want %v", tables, want)
	}

	// Enums.
	for enum, wantMembers := range map[string][]string{
		"owner_kind":       {"world", "player", "chat"},
		"ledger_kind":      {"money", "experience"},
		"operation_source": {"telegram"},
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
	if ownerCount != 1 || scopeCount != 1 || accountCount != 2 || balanceCount != 0 {
		t.Fatalf("seed counts = owner:%d scope:%d account:%d balance:%d, want 1/1/2/0",
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
		{"owner", 2}, {"scope", 2}, {"account", 3},
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
	if count != 2 {
		t.Fatalf("goose_db_version rows = %d, want 2", count)
	}
}

var (
	downRe        = regexp.MustCompile(`(?i)--\s*\+goose\s+Down`)
	renameValueRe = regexp.MustCompile(`(?i)RENAME\s+VALUE`)
	dropValueRe   = regexp.MustCompile(`(?i)DROP\s+VALUE`)
	createTrigRe  = regexp.MustCompile(`(?i)CREATE\s+TRIGGER`)
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
	} {
		if !found[want] {
			t.Errorf("index %s is missing (have %v)", want, found)
		}
	}

	// CHECK constraints carry the definitions the tests rely on.
	for name, wantSub := range map[string]string{
		"account_balance_nonnegative":     "balance >= 0",
		"posting_amount_nonzero":          "amount <> 0",
		"journal_entry_exactly_one_basis": "num_nonnulls",
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
