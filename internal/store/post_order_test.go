package store

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPost_phase_and_capture_order(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	pMoney, pExp := createPlayer(t, ctx, tx)
	qMoney, qExp := createPlayer(t, ctx, tx)

	// Given shuffled, per the spec's fixture.
	batch := []Posting{
		{AccountID: qMoney, Amount: decimal.RequireFromString("1")},
		{AccountID: pExp, Amount: decimal.RequireFromString("2")},
		{AccountID: pMoney, Amount: decimal.RequireFromString("4")},
		{AccountID: qMoney, Amount: decimal.RequireFromString("1")},
		{AccountID: WorldExperience, Amount: decimal.RequireFromString("-2")},
		{AccountID: WorldMoney, Amount: decimal.RequireFromString("-6")},
		{AccountID: qExp, Amount: decimal.RequireFromString("5")},
		{AccountID: qExp, Amount: decimal.RequireFromString("-5")},
	}

	rec.Reset()
	if err := Post(ctx, tx, &PlayerOperation{Source: SourceTelegram, OperationID: "order-1"}, batch...); err != nil {
		t.Fatalf("Post: %v", err)
	}

	stmts := rec.Statements()

	// 1. Exactly one SELECT over account/account_definition.
	var selects []recordedStmt
	for _, s := range stmts {
		if strings.Contains(s.SQL, "SELECT") && strings.Contains(s.SQL, "account_definition") {
			selects = append(selects, s)
		}
	}
	if len(selects) != 1 {
		t.Fatalf("SELECT statements = %d, want 1 (%+v)", len(selects), selects)
	}

	// 2. Exactly one INSERT INTO player_operation, exactly one INSERT INTO
	// journal_entry.
	countContaining := func(sql string) int {
		n := 0
		for _, s := range stmts {
			if strings.Contains(s.SQL, sql) {
				n++
			}
		}
		return n
	}
	if n := countContaining("INSERT INTO player_operation"); n != 1 {
		t.Fatalf("player_operation inserts = %d, want 1", n)
	}
	if n := countContaining("INSERT INTO journal_entry"); n != 1 {
		t.Fatalf("journal_entry inserts = %d, want 1", n)
	}

	// 3. UPDATE account_balance statements: exactly for P.money, P.experience,
	// Q.money, ascending account_id, each RowsAffected == 1, none upserts,
	// none touching more than one account_id argument, none for World or
	// Q.experience.
	var updates []recordedStmt
	for _, s := range stmts {
		if strings.Contains(s.SQL, "UPDATE account_balance") {
			updates = append(updates, s)
		}
	}
	wantIDs := []AccountID{pMoney, pExp, qMoney}
	sort.Slice(wantIDs, func(i, j int) bool { return wantIDs[i] < wantIDs[j] })

	if len(updates) != len(wantIDs) {
		t.Fatalf("UPDATE account_balance statements = %d, want %d (%+v)", len(updates), len(wantIDs), updates)
	}
	for i, u := range updates {
		if strings.Contains(strings.ToUpper(u.SQL), "ON CONFLICT") {
			t.Fatalf("update %+v must not be an upsert", u)
		}
		if len(u.Args) == 0 {
			t.Fatalf("update %+v has no args", u)
		}
		gotID, ok := u.Args[0].(AccountID)
		if !ok {
			t.Fatalf("update %+v first arg is not AccountID: %T", u, u.Args[0])
		}
		if gotID != wantIDs[i] {
			t.Fatalf("update %d account_id = %d, want %d (ascending order)", i, gotID, wantIDs[i])
		}
		if u.RowsAffected != 1 {
			t.Fatalf("update for account %d rows affected = %d, want 1", gotID, u.RowsAffected)
		}
		// Only one account_id argument: the WHERE clause references exactly
		// one placeholder for it ($1), never a multi-row form.
		if strings.Contains(u.SQL, "IN (") || strings.Contains(u.SQL, "ANY(") {
			t.Fatalf("update %+v looks like a multi-row statement", u)
		}
	}

	// 4. Eight INSERT INTO posting statements, in the caller's order.
	var postingInserts []recordedStmt
	for _, s := range stmts {
		if strings.Contains(s.SQL, "INSERT INTO posting") {
			postingInserts = append(postingInserts, s)
		}
	}
	if len(postingInserts) != len(batch) {
		t.Fatalf("posting inserts = %d, want %d", len(postingInserts), len(batch))
	}
	for i, ins := range postingInserts {
		gotAccount, ok := ins.Args[1].(AccountID)
		if !ok || gotAccount != batch[i].AccountID {
			t.Fatalf("posting insert %d account = %v, want %d", i, ins.Args[1], batch[i].AccountID)
		}
	}
	// 5. Inter-phase order and total count (AC3): SELECT < document INSERT
	// < journal_entry INSERT < first UPDATE; last UPDATE < first posting
	// INSERT; and nothing else was issued.
	firstIdx := func(sub string) int {
		for i, s := range stmts {
			if strings.Contains(s.SQL, sub) {
				return i
			}
		}
		return -1
	}
	lastIdx := func(sub string) int {
		for i := len(stmts) - 1; i >= 0; i-- {
			if strings.Contains(stmts[i].SQL, sub) {
				return i
			}
		}
		return -1
	}
	selIdx := firstIdx("account_definition")
	docIdx := firstIdx("INSERT INTO player_operation")
	entryIdx := firstIdx("INSERT INTO journal_entry")
	firstUpd, lastUpd := firstIdx("UPDATE account_balance"), lastIdx("UPDATE account_balance")
	firstPost := firstIdx("INSERT INTO posting")
	if selIdx >= docIdx || docIdx >= entryIdx || entryIdx >= firstUpd || lastUpd >= firstPost {
		t.Fatalf("phase order violated: select=%d document=%d entry=%d updates=%d..%d postings=%d",
			selIdx, docIdx, entryIdx, firstUpd, lastUpd, firstPost)
	}
	if wantTotal := 1 + 1 + 1 + len(wantIDs) + len(batch); len(stmts) != wantTotal {
		t.Fatalf("statements recorded = %d, want %d: %+v", len(stmts), wantTotal, stmts)
	}
}
