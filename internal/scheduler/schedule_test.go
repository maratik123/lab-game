package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func testRegistry(tb testing.TB) *Registry {
	tb.Helper()
	reg, err := NewRegistry(
		Declaration{Type: "test.oneshot", Handler: &fixedOutcomeHandler{outcome: OutcomeDone}},
		Declaration{Type: "test.recurrent", Handler: &fixedOutcomeHandler{outcome: OutcomeDone}, Recurrence: &Recurrence{Cadence: Every(time.Hour), ConfigKey: "test.recurrent.period"}},
	)
	if err != nil {
		tb.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestSchedule_payloadRoundTrip is AC24: a payload with a nested object
// and a null value round-trips by value, never by raw bytes — jsonb
// normalises key order, whitespace and duplicate keys (design D5), so a
// byte comparison would be asserting a property the storage type does
// not have.
func TestSchedule_payloadRoundTrip(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)

	want := map[string]any{"n": float64(5), "nested": map[string]any{"a": nil}}
	payload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	id := dueNow(t, pool, reg, Request{Type: "test.oneshot", Payload: payload})

	var stored json.RawMessage
	if err := pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_task WHERE id = $1`, int64(id)).Scan(&stored); err != nil {
		t.Fatalf("select payload: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stored, &got); err != nil {
		t.Fatalf("unmarshal stored payload: %v", err)
	}
	if len(got) != len(want) || got["n"] != want["n"] {
		t.Fatalf("decoded payload = %v, want %v", got, want)
	}
	gotNested, _ := got["nested"].(map[string]any)
	wantNested := want["nested"].(map[string]any)
	if gotNested["a"] != wantNested["a"] {
		t.Fatalf("decoded nested payload = %v, want %v", gotNested, wantNested)
	}
}

func TestSchedule_unregisteredType_writesNoRow(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = reg.Schedule(ctx, tx, Request{Type: "no.such.type"})
	if !errors.Is(err, ErrUnknownType) {
		t.Fatalf("Schedule(unregistered) = %v, want ErrUnknownType", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM scheduled_task`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("scheduled_task has %d rows after a refused Schedule, want 0", count)
	}
}

func TestSchedule_negativeDelay_returnsErrInvalidDelay(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = reg.Schedule(ctx, tx, Request{Type: "test.oneshot", Delay: -time.Second})
	if !errors.Is(err, ErrInvalidDelay) {
		t.Fatalf("Schedule(negative delay) = %v, want ErrInvalidDelay", err)
	}
}

// TestSchedule_duplicateLiveIdentity_returnsErrDuplicateTask is AC29's
// live-duplicate direction. The database still refuses the second live
// row (via ON CONFLICT ... DO NOTHING, design D9) — Schedule maps the
// resulting zero-rows-returned to ErrDuplicateTask rather than letting a
// raw 23505 abort the caller's transaction, so the transaction below
// stays usable afterwards.
func TestSchedule_duplicateLiveIdentity_returnsErrDuplicateTask(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := reg.Schedule(ctx, tx, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"}); err != nil {
		t.Fatalf("first schedule: %v", err)
	}

	_, err = reg.Schedule(ctx, tx, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"})
	if !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("second schedule = %v, want ErrDuplicateTask", err)
	}

	// The transaction must still be usable — no SQLSTATE was raised.
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("transaction unusable after refused duplicate: %v", err)
	}
}

func TestSchedule_keylessOneShots_coexist(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id1, err := reg.Schedule(ctx, tx, Request{Type: "test.oneshot"})
	if err != nil {
		t.Fatalf("first keyless schedule: %v", err)
	}
	id2, err := reg.Schedule(ctx, tx, Request{Type: "test.oneshot"})
	if err != nil {
		t.Fatalf("second keyless schedule: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected two distinct rows, both got %d", id1)
	}
}

// TestSchedule_reschedulingDeadIdentity_succeeds closes the round-1 trap
// (design D5): a dead row of an identity must not block re-scheduling it.
func TestSchedule_reschedulingDeadIdentity_succeeds(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := testRegistry(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at, state)
		 VALUES ('test.recurrent', 'test.recurrent', '{}'::jsonb, now(), 'dead')`,
	); err != nil {
		t.Fatalf("seed dead row: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := reg.Schedule(ctx, tx, Request{Type: "test.recurrent", InstanceKey: "test.recurrent"}); err != nil {
		t.Fatalf("schedule against a dead identity: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var pending, dead int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state = 'pending'), count(*) FILTER (WHERE state = 'dead') FROM scheduled_task WHERE type = 'test.recurrent'`).Scan(&pending, &dead); err != nil {
		t.Fatalf("count: %v", err)
	}
	if pending != 1 || dead != 1 {
		t.Fatalf("pending/dead = %d/%d, want 1/1", pending, dead)
	}
}

func TestDeadTasks_returnsGiveUpRowsOnly(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at, state, consecutive_failures, last_error)
		 VALUES ('test.oneshot', 'dead-1', '{}'::jsonb, now(), 'dead', 5, 'boom')`,
	); err != nil {
		t.Fatalf("seed dead row: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at)
		 VALUES ('test.oneshot', 'pending-1', '{}'::jsonb, now())`,
	); err != nil {
		t.Fatalf("seed pending row: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := DeadTasks(ctx, tx, 10)
	if err != nil {
		t.Fatalf("DeadTasks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("DeadTasks returned %d rows, want 1 (pending rows must be excluded)", len(rows))
	}
	got := rows[0]
	if got.Type != "test.oneshot" || got.InstanceKey != "dead-1" || got.ConsecutiveFailures != 5 || got.LastError != "boom" {
		t.Fatalf("DeadTasks row = %+v, want type test.oneshot, instance dead-1, failures 5, error boom", got)
	}
}
