package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func recurrentRegistry(t *testing.T, typ Type, period time.Duration) *Registry {
	t.Helper()
	reg, err := NewRegistry(Declaration{
		Type: typ, Handler: &fixedOutcomeHandler{outcome: OutcomeDone},
		Recurrence: &Recurrence{Cadence: Every(period), ConfigKey: "k"},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// TestReconcile_seedsMissingRow is AC33: a declared recurrence with no
// row is seeded, one cadence ahead, not immediately, and the seeded
// row's instance_key is its type name.
func TestReconcile_seedsMissingRow(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	period := time.Hour
	reg := recurrentRegistry(t, "recon.seed", period)
	w, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	before := time.Now()
	if err := w.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var instanceKey string
	var runAt time.Time
	if err := pool.QueryRow(ctx, `SELECT instance_key, run_at FROM scheduled_task WHERE type = 'recon.seed'`).Scan(&instanceKey, &runAt); err != nil {
		t.Fatalf("select seeded row: %v", err)
	}
	if instanceKey != "recon.seed" {
		t.Fatalf("instance_key = %q, want the type name %q", instanceKey, "recon.seed")
	}
	if !runAt.After(before.Add(period / 2)) {
		t.Fatalf("run_at = %v, want roughly one cadence ahead of %v, not immediate", runAt, before)
	}
}

// TestReconcile_seedsAgainstDeadRow is AC33: a declared recurrence whose
// only row is dead is seeded, leaving exactly one pending row beside the
// untouched dead one.
func TestReconcile_seedsAgainstDeadRow(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	reg := recurrentRegistry(t, "recon.dead", time.Hour)
	w, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_task (type, instance_key, payload, run_at, state) VALUES ('recon.dead', 'recon.dead', '{}'::jsonb, now(), 'dead')`,
	); err != nil {
		t.Fatalf("seed dead row: %v", err)
	}

	if err := w.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var pending, dead int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE state='pending'), count(*) FILTER (WHERE state='dead') FROM scheduled_task WHERE type = 'recon.dead'`,
	).Scan(&pending, &dead); err != nil {
		t.Fatalf("count: %v", err)
	}
	if pending != 1 || dead != 1 {
		t.Fatalf("pending/dead = %d/%d, want 1/1", pending, dead)
	}
}

// TestReconcile_correction covers AC33's directional and idempotence
// properties: a shortened cadence corrects run_at; a lengthened cadence
// leaves the row alone; running Reconcile twice changes nothing the
// second time.
func TestReconcile_correction(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	longPeriod := 2 * time.Hour

	t.Run("shortened_cadence_corrects", func(t *testing.T) {
		t.Parallel()
		regLong := recurrentRegistry(t, "recon.shorten", longPeriod)
		wLong, err := New(Options{Pool: pool, Registry: regLong, Config: testConfig()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := wLong.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (seed with long period): %v", err)
		}
		var originalRunAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.shorten'`).Scan(&originalRunAt); err != nil {
			t.Fatalf("select: %v", err)
		}

		shortPeriod := time.Minute
		regShort := recurrentRegistry(t, "recon.shorten", shortPeriod)
		wShort, err := New(Options{Pool: pool, Registry: regShort, Config: testConfig()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := wShort.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (shortened): %v", err)
		}
		var correctedRunAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.shorten'`).Scan(&correctedRunAt); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !correctedRunAt.Before(originalRunAt) {
			t.Fatalf("corrected run_at = %v, want it earlier than the original %v (cadence shortened)", correctedRunAt, originalRunAt)
		}

		// Running Reconcile again with the same (shortened) declaration
		// changes nothing further.
		if err := wShort.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (repeat): %v", err)
		}
		var repeatRunAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.shorten'`).Scan(&repeatRunAt); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !repeatRunAt.Equal(correctedRunAt) {
			t.Fatalf("run_at after a repeat Reconcile = %v, want it unchanged at %v", repeatRunAt, correctedRunAt)
		}
	})

	t.Run("lengthened_cadence_leaves_row_alone", func(t *testing.T) {
		t.Parallel()
		shortPeriod := time.Minute
		regShort := recurrentRegistry(t, "recon.lengthen", shortPeriod)
		wShort, err := New(Options{Pool: pool, Registry: regShort, Config: testConfig()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := wShort.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (seed with short period): %v", err)
		}
		var originalRunAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.lengthen'`).Scan(&originalRunAt); err != nil {
			t.Fatalf("select: %v", err)
		}

		regLong := recurrentRegistry(t, "recon.lengthen", longPeriod)
		wLong, err := New(Options{Pool: pool, Registry: regLong, Config: testConfig()})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := wLong.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile (lengthened): %v", err)
		}
		var afterRunAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.lengthen'`).Scan(&afterRunAt); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !afterRunAt.Equal(originalRunAt) {
			t.Fatalf("run_at after a lengthened cadence = %v, want it unchanged at %v (absorbed by one early occurrence)", afterRunAt, originalRunAt)
		}
	})
}

// TestReconcile_concurrentCallsProduceOneRow is AC33/AC29 under -race:
// two Reconcile calls racing produce exactly one row, the loser's insert
// resolved by the constraint's no-op rather than a lock wait.
func TestReconcile_concurrentCallsProduceOneRow(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()
	reg := recurrentRegistry(t, "recon.concurrent", time.Hour)
	w1, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w2, err := New(Options{Pool: pool, Registry: reg, Config: testConfig()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	for _, w := range []*Worker{w1, w2} {
		go func(w *Worker) {
			defer wg.Done()
			if err := w.Reconcile(ctx); err != nil {
				errs <- err
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Reconcile: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_task WHERE type = 'recon.concurrent'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count after two concurrent Reconcile calls = %d, want 1", count)
	}
}

// TestReconcile_leavesImminentAndInFlightAlone is AC33/AC35: an imminent
// occurrence (inside a poll interval) and an in-flight occurrence (its
// row held by an open transaction under the same locking mode the
// correction takes) are both left untouched — the correction returns
// having updated nothing rather than waiting on the lock. The same case
// drives the seed against the in-flight row too, so "neither move
// disturbs an occurrence in flight" is asserted for both moves.
func TestReconcile_leavesImminentAndInFlightAlone(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	ctx := context.Background()

	t.Run("imminent", func(t *testing.T) {
		t.Parallel()
		cfg := testConfig()
		reg := recurrentRegistry(t, "recon.imminent", time.Minute)
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// An occurrence due just inside the poll interval.
		imminentRunAt := time.Now().Add(cfg.PollInterval / 2)
		if _, err := pool.Exec(ctx,
			`INSERT INTO scheduled_task (type, instance_key, payload, run_at) VALUES ('recon.imminent', 'recon.imminent', '{}'::jsonb, $1)`,
			imminentRunAt,
		); err != nil {
			t.Fatalf("seed imminent row: %v", err)
		}
		if err := w.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		var runAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE type = 'recon.imminent'`).Scan(&runAt); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !runAt.Equal(imminentRunAt.Truncate(time.Microsecond)) && runAt.Sub(imminentRunAt).Abs() > time.Millisecond {
			t.Fatalf("run_at = %v, want it left alone at %v (imminent — inside the poll interval)", runAt, imminentRunAt)
		}
	})

	t.Run("in_flight", func(t *testing.T) {
		t.Parallel()
		reg := recurrentRegistry(t, "recon.inflight", time.Millisecond) // a cadence far shorter than the seeded row's, so the correction would otherwise fire
		cfg := testConfig()
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		var id int64
		distantRunAt := time.Now().Add(2 * time.Hour)
		if err := pool.QueryRow(ctx,
			`INSERT INTO scheduled_task (type, instance_key, payload, run_at) VALUES ('recon.inflight', 'recon.inflight', '{}'::jsonb, $1) RETURNING id`,
			distantRunAt,
		).Scan(&id); err != nil {
			t.Fatalf("seed in-flight row: %v", err)
		}

		holder, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin holder: %v", err)
		}
		defer func() { _ = holder.Rollback(ctx) }()
		if _, err := holder.Exec(ctx, `SELECT id FROM scheduled_task WHERE id = $1 FOR NO KEY UPDATE`, id); err != nil {
			t.Fatalf("hold lock: %v", err)
		}

		if err := w.Reconcile(ctx); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		var runAt time.Time
		if err := pool.QueryRow(ctx, `SELECT run_at FROM scheduled_task WHERE id = $1`, id).Scan(&runAt); err != nil {
			t.Fatalf("select: %v", err)
		}
		if !runAt.Equal(distantRunAt.Truncate(time.Microsecond)) && runAt.Sub(distantRunAt).Abs() > time.Millisecond {
			t.Fatalf("run_at = %v, want it left alone at %v (in flight — locked)", runAt, distantRunAt)
		}

		// The seed's ON CONFLICT arbiter still matches the live in-flight
		// row and inserts nothing, without waiting on its lock.
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_task WHERE type = 'recon.inflight'`).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Fatalf("row count for recon.inflight = %d, want 1 (the seed must not insert a second row)", count)
		}
	})
}

// TestRun_reconcilesBeforeFirstCycle_RunOnceDoesNot is design D10: Run
// seeds a declared recurrence's row before any task executes, while
// RunOnce does not reconcile at all; and a Reconcile that fails stops
// Run from entering the loop.
func TestRun_reconcilesBeforeFirstCycle_RunOnceDoesNot(t *testing.T) {
	t.Parallel()

	pool := newScheduler(t)
	reg := recurrentRegistry(t, "recon.run", time.Hour)
	cfg := testConfig()
	cfg.PollInterval = 20 * time.Millisecond

	t.Run("RunOnce_does_not_reconcile", func(t *testing.T) {
		t.Parallel()
		w, err := New(Options{Pool: pool, Registry: reg, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := w.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_task WHERE type = 'recon.run'`).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatalf("row count after RunOnce (no Reconcile) = %d, want 0", count)
		}
	})

	t.Run("Run_reconciles_before_first_cycle", func(t *testing.T) {
		t.Parallel()
		reg2 := recurrentRegistry(t, "recon.run2", time.Hour)
		w, err := New(Options{Pool: pool, Registry: reg2, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		err = w.Run(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run returned %v, want context.DeadlineExceeded", err)
		}
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_task WHERE type = 'recon.run2'`).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Fatalf("row count after Run = %d, want 1 (seeded before the first cycle)", count)
		}
	})

	t.Run("Run_stops_if_Reconcile_fails", func(t *testing.T) {
		t.Parallel()
		badPool := newScheduler(t)
		badPool.Close()
		reg3 := recurrentRegistry(t, "recon.run3", time.Hour)
		w, err := New(Options{Pool: badPool, Registry: reg3, Config: cfg})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := w.Run(context.Background()); err == nil {
			t.Fatalf("Run with a closed pool = nil, want an error from the failed Reconcile")
		}
	})
}
