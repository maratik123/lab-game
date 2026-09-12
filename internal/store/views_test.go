package store

import (
	"context"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// viewNames is the exact set of views this migration set creates: the five
// MVP metric families plus the item machine's three reconciliation views,
// and no placeholder for any deferred one.
var viewNames = []string{
	"metric_activation_funnel",
	"metric_retention_daily",
	"metric_death_by_depth",
	"metric_faucet_sink",
	"metric_notification_per_chat_day",
	"item_holder",
	"item_chain_break",
	"item_capacity_divergence",
}

func TestViews_exactViewSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	rows, err := pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = current_schema() AND table_type = 'VIEW'`)
	if err != nil {
		t.Fatalf("list views: %v", err)
	}
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan view name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	want := append([]string(nil), viewNames...)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("views = %v, want exactly %v (AC11, AC13)", got, want)
	}
}

// TestViews_columnContract pins each view's column names, order and types
// — the one thing CREATE OR REPLACE VIEW can never loosen later.
func TestViews_columnContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	type col struct {
		name string
		typ  string
	}
	cases := []struct {
		view string
		cols []col
	}{
		{"metric_activation_funnel", []col{
			{"chat_id", "bigint"},
			{"added_at", "timestamp with time zone"},
			{"player_started", "bigint"},
			{"player_first_raided", "bigint"},
			{"player_returned_next_day", "bigint"},
		}},
		{"metric_retention_daily", []col{
			{"day", "date"},
			{"active_player", "bigint"},
			{"raid", "bigint"},
			{"raid_per_active_player", "numeric"},
			{"new_player", "bigint"},
			{"d1_retained", "bigint"},
			{"d7_retained", "bigint"},
			{"d1_rate", "numeric"},
			{"d7_rate", "numeric"},
		}},
		{"metric_death_by_depth", []col{
			{"day", "date"},
			{"depth", "integer"},
			{"death", "bigint"},
			{"player", "bigint"},
		}},
		{"metric_faucet_sink", []col{
			{"day", "date"},
			{"kind", "text"},
			{"faucet", "numeric"},
			{"sink", "numeric"},
			{"net_to_economy", "numeric"},
		}},
		{"metric_notification_per_chat_day", []col{
			{"day", "date"},
			{"chat_id", "bigint"},
			{"notification", "bigint"},
		}},
		{"item_holder", []col{
			{"item_id", "bigint"},
			{"holder_id", "bigint"},
			{"movement_id", "bigint"},
		}},
		{"item_chain_break", []col{
			{"item_id", "bigint"},
			{"reached_movement", "bigint"},
			{"recorded_movement", "bigint"},
			{"head_movement", "bigint"},
		}},
		{"item_capacity_divergence", []col{
			{"holder_id", "bigint"},
			{"item_count", "bigint"},
			{"slots_used_balance", "numeric"},
			{"reason", "text"},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			t.Parallel()
			rows, err := pool.Query(ctx,
				`SELECT column_name, data_type FROM information_schema.columns
				 WHERE table_schema = current_schema() AND table_name = $1
				 ORDER BY ordinal_position`, tc.view)
			if err != nil {
				t.Fatalf("list columns: %v", err)
			}
			var got []col
			for rows.Next() {
				var c col
				if err := rows.Scan(&c.name, &c.typ); err != nil {
					t.Fatalf("scan column: %v", err)
				}
				got = append(got, c)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
			if !slices.Equal(got, tc.cols) {
				t.Fatalf("%s columns = %v, want %v", tc.view, got, tc.cols)
			}
		})
	}
}

// TestViews_emptyLog_zeroRows asserts the "queryable and returns zero
// rows, never an error" property on a freshly migrated, event-free database.
func TestViews_emptyLog_zeroRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	for _, view := range viewNames {
		t.Run(view, func(t *testing.T) {
			t.Parallel()
			rows, err := pool.Query(ctx, `SELECT * FROM `+view) // view is always one of the fixed literals in viewNames above
			if err != nil {
				t.Fatalf("query %s: %v", view, err)
			}
			defer rows.Close()
			count := 0
			for rows.Next() {
				count++
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows: %v", err)
			}
			if count != 0 {
				t.Fatalf("%s on an empty log = %d rows, want 0", view, count)
			}
		})
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func at(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, time.UTC)
}

func ptrInt32(v int32) *int32 { return &v }

// insertEvent writes one event row by direct SQL with an explicit ts — the
// Go write API stamps now(), so a multi-day fixture must bypass it.
func insertEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, typ EventType, playerID, chatID *OwnerID, depth *int32, ts time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO event (type, player_id, chat_id, depth, ts) VALUES ($1, $2, $3, $4, $5)`,
		typ, playerID, chatID, depth, ts,
	); err != nil {
		t.Fatalf("insert event %s: %v", typ, err)
	}
}

// viewsFixture is the ids the shared fixture created, needed by the
// per-view literal expectation tables.
type viewsFixture struct {
	chat1, chat2           OwnerID
	chat1Added, chat2Added time.Time
}

// seedViewsFixture writes the ONE shared fixture every view subtest below
// reads — each view's subtest reads the same world. Every boundary case
// below is present:
//   - every raid_started carries a null chat_id (proves attribution runs
//     through player_started, not the raid)
//   - D raids twice the same day (not a next-day return)
//   - E's second raid is two days later (not a next-day return either)
//   - A raids again exactly the next day (IS a return)
//   - B has a player_started in a chat later than its first (still
//     credited to the first chat)
//   - F's player_started names no chat (attributed nowhere)
//   - G/H register on the same day, only G returns the next day (a
//     fractional Day-1 rate for that day's retention cohort)
//   - a death with a null depth, and deaths spanning two days
//   - a notification_sent with a null chat_id (excluded from its view)
//   - a faucet-only day for metric_faucet_sink (no sink that day)
func seedViewsFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) viewsFixture {
	t.Helper()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture tx: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			rollback(t, ctx, tx)
		}
	}()

	newOwner := func(kind OwnerKind) Owner {
		tg := nextTelegramID.Add(1)
		o, err := CreateOwner(ctx, tx, kind, &tg)
		if err != nil {
			t.Fatalf("create owner: %v", err)
		}
		return o
	}

	chat1 := newOwner(OwnerChat)
	chat2 := newOwner(OwnerChat)
	playerA := newOwner(OwnerPlayer)
	playerB := newOwner(OwnerPlayer)
	playerD := newOwner(OwnerPlayer)
	playerE := newOwner(OwnerPlayer)
	playerF := newOwner(OwnerPlayer)
	playerG := newOwner(OwnerPlayer)
	playerH := newOwner(OwnerPlayer)
	pd1 := newOwner(OwnerPlayer)
	pd2 := newOwner(OwnerPlayer)
	fx := newOwner(OwnerPlayer) // faucet/sink's poster

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit owners: %v", err)
	}
	committed = true

	chat1Added := at(2024, 1, 1, 8, 0)
	chat2Added := at(2024, 1, 1, 9, 0)
	insertEvent(t, ctx, pool, EventBotAddedToChat, nil, &chat1.ID, nil, chat1Added)
	insertEvent(t, ctx, pool, EventBotAddedToChat, nil, &chat2.ID, nil, chat2Added)

	// player_started: day0 registrants A, B, D, E (chat1) and F (null chat).
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerA.ID, &chat1.ID, nil, at(2024, 1, 1, 10, 0))
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerB.ID, &chat1.ID, nil, at(2024, 1, 1, 11, 0))
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerD.ID, &chat1.ID, nil, at(2024, 1, 1, 13, 0))
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerE.ID, &chat1.ID, nil, at(2024, 1, 1, 14, 0))
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerF.ID, nil, nil, at(2024, 1, 1, 15, 0))
	// B's later, second player_started names chat2 — attribution stays chat1.
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerB.ID, &chat2.ID, nil, at(2024, 1, 3, 9, 0))
	// day2 registrants G, H — both null chat.
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerG.ID, nil, nil, at(2024, 1, 3, 8, 0))
	insertEvent(t, ctx, pool, EventPlayerStarted, &playerH.ID, nil, nil, at(2024, 1, 3, 8, 30))

	// raid_started — every row carries a null chat_id by construction.
	insertEvent(t, ctx, pool, EventRaidStarted, &playerA.ID, nil, nil, at(2024, 1, 2, 10, 0)) // day1, A's first
	insertEvent(t, ctx, pool, EventRaidStarted, &playerA.ID, nil, nil, at(2024, 1, 3, 10, 0)) // day2 = day1+1: return
	insertEvent(t, ctx, pool, EventRaidStarted, &playerD.ID, nil, nil, at(2024, 1, 2, 9, 0))  // day1, D's first
	insertEvent(t, ctx, pool, EventRaidStarted, &playerD.ID, nil, nil, at(2024, 1, 2, 20, 0)) // day1 again: same day, no return
	insertEvent(t, ctx, pool, EventRaidStarted, &playerE.ID, nil, nil, at(2024, 1, 3, 11, 0)) // day2, E's first
	insertEvent(t, ctx, pool, EventRaidStarted, &playerE.ID, nil, nil, at(2024, 1, 5, 11, 0)) // day4 = day2+2: no return
	insertEvent(t, ctx, pool, EventRaidStarted, &playerG.ID, nil, nil, at(2024, 1, 4, 9, 0))  // day3 = G's day2+1

	// death — a null-depth row and deaths spanning two days.
	insertEvent(t, ctx, pool, EventDeath, &pd1.ID, nil, ptrInt32(3), at(2024, 1, 25, 12, 0))
	insertEvent(t, ctx, pool, EventDeath, &pd2.ID, nil, nil, at(2024, 1, 25, 13, 0))
	insertEvent(t, ctx, pool, EventDeath, &pd1.ID, nil, ptrInt32(5), at(2024, 1, 26, 12, 0))

	// notification_sent — two chat-bearing rows and one excluded null-chat row.
	insertEvent(t, ctx, pool, EventNotificationSent, nil, &chat1.ID, nil, at(2024, 1, 30, 9, 0))
	insertEvent(t, ctx, pool, EventNotificationSent, nil, &chat1.ID, nil, at(2024, 1, 30, 10, 0))
	insertEvent(t, ctx, pool, EventNotificationSent, nil, nil, nil, at(2024, 1, 30, 11, 0))

	// faucet/sink: independent of the event table, so it uses its own
	// player and its own days. Written by direct SQL with an explicit ts,
	// since Post always stamps now().
	var fxMoney, fxExperience AccountID
	if err := pool.QueryRow(ctx,
		`SELECT a.id FROM account a JOIN scope s ON s.id = a.scope_id
		 JOIN account_definition d ON d.id = a.account_definition_id
		 WHERE s.owner_id = $1 AND d.code = 'money'`, fx.ID,
	).Scan(&fxMoney); err != nil {
		t.Fatalf("select fixture money account: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT a.id FROM account a JOIN scope s ON s.id = a.scope_id
		 JOIN account_definition d ON d.id = a.account_definition_id
		 WHERE s.owner_id = $1 AND d.code = 'experience'`, fx.ID,
	).Scan(&fxExperience); err != nil {
		t.Fatalf("select fixture experience account: %v", err)
	}

	insertLedgerLeg := func(ts time.Time, worldAccount, otherAccount AccountID, worldAmount decimal.Decimal) {
		var mcID, jeID int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO manual_correction (actor, reason) VALUES ('fixture', 'faucet-sink') RETURNING id`,
		).Scan(&mcID); err != nil {
			t.Fatalf("insert manual_correction: %v", err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO journal_entry (manual_correction_id, ts) VALUES ($1, $2) RETURNING id`, mcID, ts,
		).Scan(&jeID); err != nil {
			t.Fatalf("insert journal_entry: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO posting (journal_entry_id, account_id, amount) VALUES ($1, $2, $3), ($1, $4, $5)`,
			jeID, worldAccount, worldAmount, otherAccount, worldAmount.Neg(),
		); err != nil {
			t.Fatalf("insert postings: %v", err)
		}
	}
	// day 2024-01-06: money and experience both faucet only (no sink that day).
	insertLedgerLeg(day(2024, 1, 6), WorldMoney, fxMoney, decimal.NewFromInt(-50))
	insertLedgerLeg(day(2024, 1, 6), WorldExperience, fxExperience, decimal.NewFromInt(-5))
	// day 2024-01-07: money sink only.
	insertLedgerLeg(day(2024, 1, 7), WorldMoney, fxMoney, decimal.NewFromInt(20))

	return viewsFixture{chat1: chat1.ID, chat2: chat2.ID, chat1Added: chat1Added, chat2Added: chat2Added}
}

func TestViews_fixtureExpectations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	fx := seedViewsFixture(t, ctx, pool)

	t.Run("metric_activation_funnel", func(t *testing.T) {
		type row struct {
			chatID                                                  OwnerID
			addedAt                                                 time.Time
			playerStarted, playerFirstRaided, playerReturnedNextDay int64
		}
		rows, err := pool.Query(ctx,
			`SELECT chat_id, added_at, player_started, player_first_raided, player_returned_next_day
			 FROM metric_activation_funnel ORDER BY chat_id`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var got []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.chatID, &r.addedAt, &r.playerStarted, &r.playerFirstRaided, &r.playerReturnedNextDay); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		want := []row{
			{fx.chat1, fx.chat1Added, 4, 3, 1},
			{fx.chat2, fx.chat2Added, 0, 0, 0},
		}
		sort.Slice(want, func(i, j int) bool { return want[i].chatID < want[j].chatID })
		if !slices.EqualFunc(got, want, func(a, b row) bool {
			return a.chatID == b.chatID && a.addedAt.Equal(b.addedAt) &&
				a.playerStarted == b.playerStarted && a.playerFirstRaided == b.playerFirstRaided &&
				a.playerReturnedNextDay == b.playerReturnedNextDay
		}) {
			t.Fatalf("metric_activation_funnel = %+v, want %+v", got, want)
		}
	})

	t.Run("metric_retention_daily", func(t *testing.T) {
		type row struct {
			day                                 time.Time
			activePlayer, raid, newPlayer       int64
			d1Retained, d7Retained              int64
			raidPerActivePlayer, d1Rate, d7Rate *decimal.Decimal
		}
		rows, err := pool.Query(ctx,
			`SELECT day, active_player, raid, raid_per_active_player, new_player,
			        d1_retained, d7_retained, d1_rate, d7_rate
			 FROM metric_retention_daily ORDER BY day`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var got []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.day, &r.activePlayer, &r.raid, &r.raidPerActivePlayer, &r.newPlayer,
				&r.d1Retained, &r.d7Retained, &r.d1Rate, &r.d7Rate); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		dec := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
		want := []row{
			{day(2024, 1, 1), 5, 0, 5, 2, 0, dec("0"), dec("0.4"), dec("0")},
			{day(2024, 1, 2), 2, 3, 0, 0, 0, dec("1.5"), nil, nil},
			{day(2024, 1, 3), 5, 2, 2, 1, 0, dec("0.4"), dec("0.5"), dec("0")},
			{day(2024, 1, 4), 1, 1, 0, 0, 0, dec("1"), nil, nil},
			{day(2024, 1, 5), 1, 1, 0, 0, 0, dec("1"), nil, nil},
			{day(2024, 1, 25), 2, 0, 0, 0, 0, dec("0"), nil, nil},
			{day(2024, 1, 26), 1, 0, 0, 0, 0, dec("0"), nil, nil},
			{day(2024, 1, 30), 0, 0, 0, 0, 0, nil, nil, nil},
		}
		if len(got) != len(want) {
			t.Fatalf("metric_retention_daily rows = %d, want %d\ngot: %+v", len(got), len(want), got)
		}
		for i := range want {
			g, w := got[i], want[i]
			eq := g.day.Equal(w.day) && g.activePlayer == w.activePlayer && g.raid == w.raid &&
				g.newPlayer == w.newPlayer && g.d1Retained == w.d1Retained && g.d7Retained == w.d7Retained &&
				decEqual(g.raidPerActivePlayer, w.raidPerActivePlayer) &&
				decEqual(g.d1Rate, w.d1Rate) && decEqual(g.d7Rate, w.d7Rate)
			if !eq {
				t.Fatalf("row %d = %+v (ratios %v/%v/%v), want %+v (ratios %v/%v/%v)",
					i, g, decStr(g.raidPerActivePlayer), decStr(g.d1Rate), decStr(g.d7Rate),
					w, decStr(w.raidPerActivePlayer), decStr(w.d1Rate), decStr(w.d7Rate))
			}
		}
	})

	t.Run("metric_death_by_depth", func(t *testing.T) {
		type row struct {
			day           time.Time
			depth         *int32
			death, player int64
		}
		rows, err := pool.Query(ctx,
			`SELECT day, depth, death, player FROM metric_death_by_depth ORDER BY day, depth NULLS LAST`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var got []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.day, &r.depth, &r.death, &r.player); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		want := []row{
			{day(2024, 1, 25), ptrInt32(3), 1, 1},
			{day(2024, 1, 25), nil, 1, 1},
			{day(2024, 1, 26), ptrInt32(5), 1, 1},
		}
		if len(got) != len(want) {
			t.Fatalf("metric_death_by_depth rows = %d, want %d: %+v", len(got), len(want), got)
		}
		for i := range want {
			g, w := got[i], want[i]
			depthEq := (g.depth == nil && w.depth == nil) || (g.depth != nil && w.depth != nil && *g.depth == *w.depth)
			if !g.day.Equal(w.day) || !depthEq || g.death != w.death || g.player != w.player {
				t.Fatalf("row %d = %+v, want %+v", i, g, w)
			}
		}
	})

	t.Run("metric_faucet_sink", func(t *testing.T) {
		type row struct {
			day                     time.Time
			kind                    string
			faucet, sink, netToEcon decimal.Decimal
		}
		rows, err := pool.Query(ctx,
			`SELECT day, kind, faucet, sink, net_to_economy FROM metric_faucet_sink ORDER BY day, kind`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var got []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.day, &r.kind, &r.faucet, &r.sink, &r.netToEcon); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		want := []row{
			{day(2024, 1, 6), "experience", decimal.NewFromInt(5), decimal.Zero, decimal.NewFromInt(5)},
			{day(2024, 1, 6), "money", decimal.NewFromInt(50), decimal.Zero, decimal.NewFromInt(50)},
			{day(2024, 1, 7), "money", decimal.Zero, decimal.NewFromInt(20), decimal.NewFromInt(-20)},
		}
		if len(got) != len(want) {
			t.Fatalf("metric_faucet_sink rows = %d, want %d: %+v", len(got), len(want), got)
		}
		for i := range want {
			g, w := got[i], want[i]
			if !g.day.Equal(w.day) || g.kind != w.kind || !g.faucet.Equal(w.faucet) ||
				!g.sink.Equal(w.sink) || !g.netToEcon.Equal(w.netToEcon) {
				t.Fatalf("row %d = %+v, want %+v", i, g, w)
			}
		}
	})

	t.Run("metric_notification_per_chat_day", func(t *testing.T) {
		type row struct {
			day          time.Time
			chatID       OwnerID
			notification int64
		}
		rows, err := pool.Query(ctx,
			`SELECT day, chat_id, notification FROM metric_notification_per_chat_day ORDER BY day, chat_id`)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		var got []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.day, &r.chatID, &r.notification); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}

		want := []row{
			{day(2024, 1, 30), fx.chat1, 2},
		}
		if len(got) != len(want) {
			t.Fatalf("metric_notification_per_chat_day rows = %d, want %d: %+v (the null-chat row must be excluded)", len(got), len(want), got)
		}
		for i := range want {
			g, w := got[i], want[i]
			if !g.day.Equal(w.day) || g.chatID != w.chatID || g.notification != w.notification {
				t.Fatalf("row %d = %+v, want %+v", i, g, w)
			}
		}
	})
}

func decEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func decStr(d *decimal.Decimal) string {
	if d == nil {
		return "<nil>"
	}
	return d.String()
}

// TestMetricFaucetSink_ledgerKindGenericity asserts that adding a new
// ledger_kind member, without editing the view, makes postings of that kind
// appear. The member must be committed before it can be used (Postgres
// refuses "unsafe use of new value" inside the same transaction that added
// it), so it is added on the pool outside any transaction — safe here
// because ledger_kind is created fresh inside this test's own schema and
// dropped with it.
func TestMetricFaucetSink_ledgerKindGenericity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	// Control: no row for the new kind before it exists at all.
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM metric_faucet_sink WHERE kind = 'gem'`).Scan(&before); err != nil {
		t.Fatalf("control count: %v", err)
	}
	if before != 0 {
		t.Fatalf("control count = %d, want 0 before the kind exists", before)
	}

	if _, err := pool.Exec(ctx, `ALTER TYPE ledger_kind ADD VALUE 'gem'`); err != nil {
		t.Fatalf("add enum value: %v", err)
	}

	// A new account_definition for the world scope and the player scope,
	// past the seeded ids (1-4).
	if _, err := pool.Exec(ctx,
		`INSERT INTO account_definition (id, scope_definition_id, code, kind, controlled) VALUES
		 (100, 1, 'gem', 'gem', false), (101, 2, 'gem', 'gem', true)`,
	); err != nil {
		t.Fatalf("insert account_definition: %v", err)
	}

	tg := nextTelegramID.Add(1)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	player, err := CreateOwner(ctx, tx, OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("create player: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var worldGem, playerGem AccountID
	if err := pool.QueryRow(ctx,
		`INSERT INTO account (scope_id, account_definition_id)
		 SELECT id, 100 FROM scope WHERE owner_id = $1 AND scope_definition_id = 1
		 RETURNING id`, WorldOwner,
	).Scan(&worldGem); err != nil {
		t.Fatalf("insert world gem account: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT a.id FROM account a JOIN scope s ON s.id = a.scope_id
		 WHERE s.owner_id = $1 AND a.account_definition_id = 101`, player.ID,
	).Scan(&playerGem); err != nil {
		t.Fatalf("select player gem account: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin post: %v", err)
	}
	defer rollback(t, ctx, tx2)
	if err := Post(ctx, tx2, &ManualCorrection{Actor: "test", Reason: "gem genericity"},
		Posting{AccountID: worldGem, Amount: decimal.NewFromInt(-3)},
		Posting{AccountID: playerGem, Amount: decimal.NewFromInt(3)},
	); err != nil {
		t.Fatalf("post gem legs: %v", err)
	}

	var faucet, sink decimal.Decimal
	if err := tx2.QueryRow(ctx,
		`SELECT faucet, sink FROM metric_faucet_sink WHERE kind = 'gem'`,
	).Scan(&faucet, &sink); err != nil {
		t.Fatalf("query metric_faucet_sink for gem: %v", err)
	}
	if !faucet.Equal(decimal.NewFromInt(3)) || !sink.Equal(decimal.Zero) {
		t.Fatalf("gem faucet/sink = %s/%s, want 3/0", faucet, sink)
	}
}
