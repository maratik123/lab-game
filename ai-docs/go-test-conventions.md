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
- **Keeping one server across runs, and what it buys.** `make test-db-up` creates the named long-lived server and records its DSN in the ignored scratch directory; `make test-db-down` is what removes it, and nothing removes it on its own — the container is created with the reaper disabled, which is what lets it outlive the process that made it, and equally what means no reaper will ever clean it up. Any container command reaches it too; the point is that forgetting about it leaves it running. Pass `CLIENTS=N` when you intend `N` whole-module runs against it at once, because the ceiling is sized for the client count you asked for and a run needing more falls through to its own container instead of joining. Ask for it before you need it: a server already running under that name keeps the capacity it was created with, so a later `CLIENTS=2` fails naming both numbers rather than resizing it in place — take it down and bring it up again.
- **The DSN is invisible to the test cache, so never reason from it.** `testdb.Main` reads `LAB_GAME_TEST_DSN` before it calls `m.Run()`, and `m.Run()` is where the testing package opens the log `cmd/go` reads to decide whether a cached result still applies — so the read is never recorded and the variable is not part of any cache key. Measured: with the cache cleared, a package re-run under a *different* DSN, and again with the variable *cleared*, answers `(cached)` both times. Two consequences. A gate that exists to exercise a particular provisioning path, or to assert a property of the run's *conditions* rather than of the code, must pass `-count=1` or it will be answered from a run that had neither — which is why `make test-fallback` carries it and why **both** of `test-contention`'s children do, the foreground race gate included: that gate's exit status is the probe's whole verdict, and a cached one is a verdict about a run that saw no load. And the ordinary gates keep replaying as they always did — a cached pass is a real pass from an identical earlier run, and nothing about the shared server changed that.
- **The fallback path is still real, and it has its own gate.** A bare `go test ./...` with `LAB_GAME_TEST_DSN` unset starts one container per database-backed test binary, exactly as before. `make test-fallback` is that invocation with the variable explicitly cleared, and CI runs it as a Test-job step — without it the path would be true on the day it was checked and unchecked forever after, since no default gate executes it any more. The container start is retried, because testcontainers shares one Ryuk reaper across the binaries `go test ./...` runs in parallel and a binary that meets it mid-creation fails its whole package.
- **Write a test that survives a neighbour, because the shared server means it has some.** Every wall-clock constant in a database-backed suite is one of two things, and they are treated oppositely. An **instrument** is a patience budget — how long a test waits for a condition, or a deadline that must *not* fire for the subject to be observable — and it is made generous, or replaced by polling on the condition with a generous ceiling; widening one changes no asserted proposition. The **subject** is the property under test — a backoff bracket, a deadline breach — and it stays exact, with its reference instants read from the **database clock around the operation** so the bracket widens with the load instead of the assertion failing. Two rules follow: widen **per test**, never by editing a shared config value, since one constant can be an instrument for one suite and the subject of another (`TaskTimeout` is both, and the worker re-applies it server-side as `statement_timeout`); and never widen until the test asserts nothing.
- **`make test-contention` is how membership of that class is established rather than guessed.** It sizes one server for two clients at the same pinned parallelism, loads it with the database-backed packages' own tests in a `-count=1` loop, and requires the whole-module race gate to stay green underneath, both children logging to files under the scratch directory. A red run is classified before it is believed, and the target does that itself rather than leaving it to whoever reads the logs: it scans both for `sorry, too many clients already` / `SQLSTATE 53300` and, on a hit, prints `INSTRUMENT FAILURE` and exits **2** — a run that exhausted the server's connections says nothing about contention in either direction, so it is neither a pass nor a finding. Exit 1 is the race gate's own verdict; exit 2 means the probe, not the tests.
- Concurrency stress cases (parallel craft + backpack move by one player) run under `-race`.
- Every test owns its data: a fresh schema or a transaction rolled back at the end. Tests must not depend on execution order.

## The scheduler and the FSM

- Every FSM edge gets a test, **including the timer edges whose guard fails** — a stale task firing late is expected traffic, not an error path (`docs/DESIGN.md` §3.5).
- Idempotency has its own tests: the same Telegram update delivered twice creates one basis document and one set of postings; a stale `seq` in `callback_data` redraws instead of acting.
- Time is injected, never read from the wall clock, so a timer edge can be tested without sleeping.

## Structural guards

A proposition about the *shape* of the source — no `func init()`, no package-level subsystem
variable, no Go clock call on a determinism path, no `promauto` — is asserted by a test that
parses the package's own non-test files, not by a grep in a `Makefile`.

- **Enumerate and parse through `internal/srcguard`**, never a hand-rolled walk. Three packages
  had grown their own copies and had already drifted apart in shape; the shared package owns the
  mechanical half — listing a package's non-test files, walking a subtree, parsing, and writing a
  scratch package into `t.TempDir()`. **Every predicate stays in the package that owns the
  proposition**, so a rule has exactly one place to look for it.
- **The repository root arrives as an argument**, resolved by `internal/repotest` — the module's
  one file-location-ascent resolver, itself held by a whole-tree walk that fails on any second one.
- **Prove the guard discriminating in the same file.** A guard that returns clean is a claim about
  the *instrument* until you have watched it go red: pair each one with a case that runs the same
  walk over a scratch package which *does* contain the forbidden construct, and asserts it is
  found. Without that pair, a walk over an empty file list and a walk over a clean tree are
  indistinguishable.

## Panics

No `panic` / `log.Fatal` in production code. A `PostToolUse` hook flags them on write. Any survivor is justified in its doc comment **and** listed in [`panic-index.md`](panic-index.md) — the comment states the justification itself and does not point at the index, because a comment naming a markdown path is what the reference ban forbids ([`doc-convention.md`](doc-convention.md) § DOC-4). Tests may panic freely (`t.Fatal` is the idiomatic failure).

## What not to do

- Do not assert on log text; assert on returned values and on database state.
- Do not build a mock for something a fake in-memory implementation of your own interface can do more honestly.
- Do not test unexported behaviour through exported APIs by contorting the input — if it needs a direct test, the test belongs in the same package.
- Do not let a test depend on map-iteration order; sort before comparing.
