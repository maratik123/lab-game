package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestBasis_nil_returns_ErrNoBasis_without_panicking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	var nilPO *PlayerOperation //nolint:staticcheck // SA4023: referenced by the deliberate typed-nil-in-interface comparison below
	if _, err := nilPO.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*PlayerOperation)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilPO.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*PlayerOperation)(nil).insert() = %v, want ErrNoBasis", err)
	}

	var nilMC *ManualCorrection
	if _, err := nilMC.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*ManualCorrection)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilMC.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*ManualCorrection)(nil).insert() = %v, want ErrNoBasis", err)
	}

	var nilDT *DeferredTask
	if _, err := nilDT.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*DeferredTask)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilDT.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*DeferredTask)(nil).insert() = %v, want ErrNoBasis", err)
	}

	var nilRT *RecurrentTask
	if _, err := nilRT.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*RecurrentTask)(nil).entrySQL() = %v, want ErrNoBasis", err)
	}
	if _, err := nilRT.insert(ctx, tx); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("(*RecurrentTask)(nil).insert() = %v, want ErrNoBasis", err)
	}

	// A typed-nil pointer stored in the interface is still != nil, and
	// entrySQL/insert on it must go through the same guarded path, not
	// panic. staticcheck SA4023 flags both lines below as statically
	// decidable — that decidability is exactly the Go gotcha under test.
	var basis PostingBasis = nilPO
	if basis == nil { //nolint:staticcheck // SA4023: never true by design — asserts the typed-nil-in-interface gotcha
		t.Fatalf("a typed-nil *PlayerOperation stored in PostingBasis must not compare equal to nil")
	}
	if _, err := basis.entrySQL(); !errors.Is(err, ErrNoBasis) {
		t.Fatalf("basis.entrySQL() via typed-nil interface = %v, want ErrNoBasis", err)
	}
}

func TestPlayerOperation_insert_replay_returns_ErrAlreadyPosted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	po := &PlayerOperation{Source: SourceTelegram, OperationID: "basis-replay-1"}

	id1, err := po.insert(ctx, tx)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if id1 == 0 {
		t.Fatalf("expected a non-zero id")
	}

	_, err = po.insert(ctx, tx)
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("replay insert = %v, want ErrAlreadyPosted", err)
	}

	// The transaction must still be usable — no 23505 was raised by the
	// database (ON CONFLICT DO NOTHING), so a further statement succeeds.
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("transaction unusable after replay: %v", err)
	}
}

func TestPost_underDeferredAndRecurrentTask_succeeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(100))

	runAt := time.Now().UTC()

	err = Post(ctx, tx, &DeferredTask{TaskID: 7, TaskType: "test.oneshot", InstanceKey: "k1", RunAt: runAt},
		Posting{AccountID: money, Amount: decimal.NewFromInt(-10)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(10)},
	)
	if err != nil {
		t.Fatalf("post under DeferredTask: %v", err)
	}

	var deferredCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_entry WHERE deferred_task_id IS NOT NULL`).Scan(&deferredCount); err != nil {
		t.Fatalf("count deferred journal entries: %v", err)
	}
	if deferredCount != 1 {
		t.Fatalf("deferred journal entries = %d, want 1", deferredCount)
	}

	err = Post(ctx, tx, &RecurrentTask{TaskID: 8, TaskType: "test.recurrent", InstanceKey: "test.recurrent", RunAt: runAt},
		Posting{AccountID: money, Amount: decimal.NewFromInt(-5)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(5)},
	)
	if err != nil {
		t.Fatalf("post under RecurrentTask: %v", err)
	}

	var recurrentCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_entry WHERE recurrent_task_id IS NOT NULL`).Scan(&recurrentCount); err != nil {
		t.Fatalf("count recurrent journal entries: %v", err)
	}
	if recurrentCount != 1 {
		t.Fatalf("recurrent journal entries = %d, want 1", recurrentCount)
	}

	// AC4: each of those journal_entry rows names exactly one non-null
	// basis column.
	var nonNullCount int
	if err := tx.QueryRow(ctx,
		`SELECT num_nonnulls(player_operation_id, manual_correction_id, deferred_task_id, recurrent_task_id)
		 FROM journal_entry WHERE deferred_task_id IS NOT NULL`,
	).Scan(&nonNullCount); err != nil {
		t.Fatalf("num_nonnulls: %v", err)
	}
	if nonNullCount != 1 {
		t.Fatalf("num_nonnulls on the deferred entry = %d, want 1", nonNullCount)
	}
}

func TestPost_twoNewBasesRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	var deferredID, recurrentID int64
	runAt := time.Now().UTC()
	if err := tx.QueryRow(ctx,
		`INSERT INTO deferred_task (task_type, run_at) VALUES ($1, $2) RETURNING id`, "t", runAt,
	).Scan(&deferredID); err != nil {
		t.Fatalf("insert deferred_task: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO recurrent_task (task_type, run_at) VALUES ($1, $2) RETURNING id`, "t", runAt,
	).Scan(&recurrentID); err != nil {
		t.Fatalf("insert recurrent_task: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO journal_entry (deferred_task_id, recurrent_task_id) VALUES ($1, $2)`, deferredID, recurrentID)
	sqlstate(t, err, "23514", "journal_entry_exactly_one_basis")
}

func TestDeferredTask_survivesTaskRowDeletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(100))

	var scheduledTaskID int64
	runAt := time.Now().UTC()
	if err := tx.QueryRow(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at) VALUES ($1, $2, '{}'::jsonb, $3) RETURNING id`,
		"test.oneshot", "survive-1", runAt,
	).Scan(&scheduledTaskID); err != nil {
		t.Fatalf("insert scheduled_task: %v", err)
	}

	if err := Post(ctx, tx, &DeferredTask{TaskID: scheduledTaskID, TaskType: "test.oneshot", InstanceKey: "survive-1", RunAt: runAt},
		Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
	); err != nil {
		t.Fatalf("post: %v", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_task WHERE id = $1`, scheduledTaskID); err != nil {
		t.Fatalf("delete scheduled_task: %v", err)
	}

	var storedTaskID int64
	var journalCount, postingCount int
	if err := tx.QueryRow(ctx,
		`SELECT dt.task_id, count(DISTINCT je.id), count(p.id)
		 FROM deferred_task dt
		 JOIN journal_entry je ON je.deferred_task_id = dt.id
		 JOIN posting p ON p.journal_entry_id = je.id
		 WHERE dt.task_id = $1
		 GROUP BY dt.task_id`, scheduledTaskID,
	).Scan(&storedTaskID, &journalCount, &postingCount); err != nil {
		t.Fatalf("survivor query: %v", err)
	}
	if storedTaskID != scheduledTaskID {
		t.Fatalf("deferred_task.task_id = %d, want %d (by value, survives the task row's deletion)", storedTaskID, scheduledTaskID)
	}
	if journalCount != 1 || postingCount != 2 {
		t.Fatalf("journal/posting counts after task deletion = %d/%d, want 1/2 — the ledger must still balance", journalCount, postingCount)
	}

	if got, want := balanceOf(t, ctx, tx, money), decimal.NewFromInt(99); !got.Equal(want) {
		t.Fatalf("balance after post-and-task-deletion = %s, want %s", got, want)
	}
}

// TestBasisTables_keylessOneShots_distinguishedByTaskID is D14's stated
// reason for TaskID: two concurrent keyless one-shots of the same type and
// instant produce basis documents that would otherwise be byte-identical,
// and delete-on-done has already removed the scheduled_task rows that
// would have told them apart.
func TestBasisTables_keylessOneShots_distinguishedByTaskID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	runAt := time.Now().UTC()
	first := &DeferredTask{TaskID: 101, TaskType: "test.oneshot", RunAt: runAt}
	second := &DeferredTask{TaskID: 102, TaskType: "test.oneshot", RunAt: runAt}

	id1, err := first.insert(ctx, tx)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	id2, err := second.insert(ctx, tx)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected two distinct deferred_task rows")
	}

	var taskID1, taskID2 int64
	if err := tx.QueryRow(ctx, `SELECT task_id FROM deferred_task WHERE id = $1`, id1).Scan(&taskID1); err != nil {
		t.Fatalf("select task_id 1: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT task_id FROM deferred_task WHERE id = $1`, id2).Scan(&taskID2); err != nil {
		t.Fatalf("select task_id 2: %v", err)
	}
	if taskID1 == taskID2 {
		t.Fatalf("task_id must distinguish the two keyless one-shots, both got %d", taskID1)
	}
}

// TestBasisTables_noForeignKeyToScheduledTask is AC27's own scope, not
// D4's wider condition: neither new basis table declares a referential
// constraint back to scheduled_task, so a delete there cannot cascade or
// block on a ledger row (design D14, D4 — the residual risk is that
// nothing in this schema enforces the wider no-FK-anywhere condition the
// lock-mode equivalence actually rests on).
func TestBasisTables_noForeignKeyToScheduledTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_constraint c
		 JOIN pg_namespace n ON n.oid = c.connamespace
		 JOIN pg_class rel ON rel.oid = c.conrelid
		 JOIN pg_class ref ON ref.oid = c.confrelid
		 WHERE c.contype = 'f' AND n.nspname = current_schema()
		   AND rel.relname IN ('deferred_task', 'recurrent_task')
		   AND ref.relname = 'scheduled_task'`,
	).Scan(&count); err != nil {
		t.Fatalf("query foreign keys: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d foreign key(s) from a basis-document table to scheduled_task, want 0 (AC27)", count)
	}
}
