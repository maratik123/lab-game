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

- Integration tests run against a real Postgres provisioned by `internal/testdb`: a `docker.io/library/postgres:18` container through testcontainers-go (major tag only — the standing image policy, [`key-decisions.md`](key-decisions.md) KD-19), or the server named by `LAB_GAME_TEST_DSN`. The suite never skips: with neither, `testdb.Main` exits non-zero. Each test takes its own schema from `testdb.Schema(t)` and applies the same migrations production applies (`store.Migrate`). The cluster lives on a tmpfs and `initdb` does not sync it — a test database that survives a crash buys nothing, and both were startup cost.
- **How a gate reaches that server: one shared server per run, the per-binary container path as the fallback.** `make test` and `make test-race` route through `cmd/testpg`, which provisions or finds one server, exports `LAB_GAME_TEST_DSN` into the child gate, and removes only a server it started itself. Decision order: a DSN already set in the environment is used as it stands; otherwise a long-lived server recorded by `make test-db-up` is used, but only if it answers *and* its `max_connections` admits this run; otherwise the wrapper starts an anonymous container sized for the run and tears it down on every exit path, an interrupt included. So a whole-module gate starts **one** container, not one per database-backed test binary. The coverage ratchet takes the same route, after its skip decision — a commit that stages nothing coverage-moving still provisions nothing.
- **Keeping one server across runs, and what it buys.** `make test-db-up` creates the named long-lived server and records its DSN in the ignored scratch directory; `make test-db-down` removes it, and it is the only thing that can — the container is created with the reaper disabled, which is what lets it outlive the process that made it. Pass `CLIENTS=N` when you intend `N` whole-module runs against it at once, because the ceiling is sized for the client count you asked for and a run needing more falls through to its own container instead of joining. A stable DSN is also what restores the test cache's usual behaviour: `testdb.Main` consults that variable, so the anonymous container's fresh ephemeral port makes every database-backed package a cache miss on every run.
- **The fallback path is still real, and it has its own gate.** A bare `go test ./...` with `LAB_GAME_TEST_DSN` unset starts one container per database-backed test binary, exactly as before. `make test-fallback` is that invocation with the variable explicitly cleared, and CI runs it as a Test-job step — without it the path would be true on the day it was checked and unchecked forever after, since no default gate executes it any more. The container start is retried, because testcontainers shares one Ryuk reaper across the binaries `go test ./...` runs in parallel and a binary that meets it mid-creation fails its whole package.
- **Write a test that survives a neighbour, because the shared server means it has some.** Every wall-clock constant in a database-backed suite is one of two things, and they are treated oppositely. An **instrument** is a patience budget — how long a test waits for a condition, or a deadline that must *not* fire for the subject to be observable — and it is made generous, or replaced by polling on the condition with a generous ceiling; widening one changes no asserted proposition. The **subject** is the property under test — a backoff bracket, a deadline breach — and it stays exact, with its reference instants read from the **database clock around the operation** so the bracket widens with the load instead of the assertion failing. Two rules follow: widen **per test**, never by editing a shared config value, since one constant can be an instrument for one suite and the subject of another (`TaskTimeout` is both, and the worker re-applies it server-side as `statement_timeout`); and never widen until the test asserts nothing.
- **`make test-contention` is how membership of that class is established rather than guessed.** It sizes one server for two clients at the same pinned parallelism, loads it with the database-backed packages' own tests in a `-count=1` loop, and requires the whole-module race gate to stay green underneath, both children logging to files under the scratch directory. A red run is read before it is believed: scan both logs for `sorry, too many clients already` first, because a run that exhausted the server's connections failed for a reason that says nothing about the tests.
- Concurrency stress cases (parallel craft + backpack move by one player) run under `-race`.
- Every test owns its data: a fresh schema or a transaction rolled back at the end. Tests must not depend on execution order.

## The scheduler and the FSM

- Every FSM edge gets a test, **including the timer edges whose guard fails** — a stale task firing late is expected traffic, not an error path (`docs/DESIGN.md` §3.5).
- Idempotency has its own tests: the same Telegram update delivered twice creates one basis document and one set of postings; a stale `seq` in `callback_data` redraws instead of acting.
- Time is injected, never read from the wall clock, so a timer edge can be tested without sleeping.

## Panics

No `panic` / `log.Fatal` in production code. A `PostToolUse` hook flags them on write. Any survivor is justified in its doc comment **and** listed in [`panic-index.md`](panic-index.md) — the comment states the justification itself and does not point at the index, because a comment naming a markdown path is what the reference ban forbids ([`doc-convention.md`](doc-convention.md) § DOC-4). Tests may panic freely (`t.Fatal` is the idiomatic failure).

## What not to do

- Do not assert on log text; assert on returned values and on database state.
- Do not build a mock for something a fake in-memory implementation of your own interface can do more honestly.
- Do not test unexported behaviour through exported APIs by contorting the input — if it needs a direct test, the test belongs in the same package.
- Do not let a test depend on map-iteration order; sort before comparing.
