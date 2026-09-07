# Go test conventions — detail

Detail behind `AGENTS.md` § *Go Test Conventions*.

## Table-driven subtests are the default

```go
func TestStaminaDebit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		balance int
		cost    int
		wantErr error
	}{
		{name: "spends_what_it_has", balance: 5, cost: 5},
		{name: "rejects_overdraft", balance: 1, cost: 2, wantErr: ErrNoStamina},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// …
		})
	}
}
```

Names describe behaviour (`rejects_overdraft`), not mechanics (`test2`). `t.Parallel()` unless the case owns shared state.

## Determinism is testable — assert exactly

Generation, combat and trail replay take an explicit seed and are pure (`docs/DESIGN.md` §2.2.2, §4). So:

- Assert exact output, not "roughly". A fuzzy assertion on a deterministic function hides the regression it was written to catch.
- Keep a **golden combat log** in the repository. `combat()` is a pure function precisely so the log can be snapshotted; the freedom to rewrite the combat system later depends on that snapshot existing now.
- A generation test pins `(world_seed, coord) → cell` for a fixed table of coordinates. Connectivity is a property test: from any materialised cell, every other reachable cell is reachable.

## Postgres is tested against Postgres

The ledger's invariants are database behaviour — the zero-sum check, the `CHECK` constraints, the capture order under concurrent writers, the daily-close reconciliation. A mock proves none of them.

- Integration tests run against a real Postgres provisioned by `internal/testdb`: a `docker.io/library/postgres:18` container through testcontainers-go (major tag only — the standing image policy, [`key-decisions.md`](key-decisions.md) KD-19), or the server named by `LAB_GAME_TEST_DSN`. The suite never skips: with neither, `testdb.Main` exits non-zero. Each test takes its own schema from `testdb.Schema(t)` and applies the same migrations production applies (`store.Migrate`). The cluster lives on a tmpfs and `initdb` does not sync it — a test database that survives a crash buys nothing, and both were startup cost. `testdb.Main` retries the container start, because testcontainers shares one Ryuk reaper across the binaries `go test ./...` runs in parallel and a binary that meets it mid-creation fails its whole package.
- Concurrency stress cases (parallel craft + backpack move by one player) run under `-race`.
- Every test owns its data: a fresh schema or a transaction rolled back at the end. Tests must not depend on execution order.

## The scheduler and the FSM

- Every FSM edge gets a test, **including the timer edges whose guard fails** — a stale task firing late is expected traffic, not an error path (`docs/DESIGN.md` §3.5).
- Idempotency has its own tests: the same Telegram update delivered twice creates one basis document and one set of postings; a stale `seq` in `callback_data` redraws instead of acting.
- Time is injected, never read from the wall clock, so a timer edge can be tested without sleeping.

## Panics

No `panic` / `log.Fatal` in production code. A `PostToolUse` hook flags them on write. Any survivor is justified in its doc comment **and** listed in [`panic-index.md`](panic-index.md); tests may panic freely (`t.Fatal` is the idiomatic failure).

## What not to do

- Do not assert on log text; assert on returned values and on database state.
- Do not build a mock for something a fake in-memory implementation of your own interface can do more honestly.
- Do not test unexported behaviour through exported APIs by contorting the input — if it needs a direct test, the test belongs in the same package.
- Do not let a test depend on map-iteration order; sort before comparing.
