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
- A generation test pins `(world_seed, coord) → cell` for a fixed table of coordinates. Connectivity is a property test: over a generated multi-chunk region, every non-island cell is reachable from every other.

## Postgres is tested against Postgres

The ledger's invariants are database behaviour — the zero-sum check, the `CHECK` constraints, the capture order under concurrent writers, the daily-close reconciliation. A mock proves none of them.

- Integration tests run against a real Postgres provisioned by `internal/testdb`: a `docker.io/library/postgres:18` container through testcontainers-go (major tag only — the standing image policy, [`key-decisions.md`](key-decisions.md) KD-19), or the server named by `LAB_GAME_TEST_DSN`. The suite never skips: with neither, `testdb.Main` exits non-zero. Each test takes its own schema from `testdb.Schema(t)` and applies the same migrations production applies (`store.Migrate`). The cluster lives on a tmpfs and `initdb` does not sync it — a test database that survives a crash buys nothing, and both were startup cost.
- **How a gate reaches that server: one shared server per run, the per-binary container path as the fallback.** `make test` and `make test-race` route through `cmd/testpg`, which provisions or finds one server, exports `LAB_GAME_TEST_DSN` into the child gate, and removes only a server it started itself. Decision order: a DSN already set in the environment is used as it stands; otherwise a long-lived server recorded by `make test-db-up` is used, but only if it answers *and* its `max_connections` admits this run; otherwise the wrapper starts an anonymous container sized for the run and tears it down on every exit path, an interrupt included. So a whole-module gate starts **one** container, not one per database-backed test binary. The coverage ratchet takes the same route, after its skip decision — a commit that stages nothing coverage-moving still provisions nothing.
- **Keeping one server across runs, and what it buys.** `make test-db-up` creates the named long-lived server and records its DSN in the ignored scratch directory; `make test-db-down` is what removes it, and nothing removes it on its own — the container is created with the reaper disabled, which is what lets it outlive the process that made it, and equally what means no reaper will ever clean it up. Any container command reaches it too; the point is that forgetting about it leaves it running. **The name is the checkout's own,** derived from the base name of the directory the wrapper runs in plus `-test-postgres`: a checkout at `~/lab-game` brings up `lab-game-test-postgres` and one at `~/lab-game2` brings up `lab-game2-test-postgres`, so two checkouts on one host each own a server and `make test-db-down` in one leaves the other's running — only checkouts whose directories carry the *same* name share a server, the parent path contributing nothing. One-time, in a checkout whose locator file was recorded before the name became per-checkout: that file still points at the server created under the old fixed name, which may be another checkout's, so a gate run there joins it and `make test-db-down` reaches for the derived name instead — delete that checkout's `tmp/testpg-dsn` (ignored local state) or remove the old container, once, and the next `make test-db-up` records the server this checkout owns. Pass `CLIENTS=N` when you intend `N` whole-module runs against it at once, because the ceiling is sized for the client count you asked for and a run needing more falls through to its own container instead of joining. Ask for it before you need it: a server already running under that name keeps the capacity it was created with, so a later `CLIENTS=2` fails naming both numbers rather than resizing it in place — take it down and bring it up again.
- **The DSN is invisible to the test cache, so never reason from it.** `testdb.Main` reads `LAB_GAME_TEST_DSN` before it calls `m.Run()`, and `m.Run()` is where the testing package opens the log `cmd/go` reads to decide whether a cached result still applies — so the read is never recorded and the variable is not part of any cache key. Measured: with the cache cleared, a package re-run under a *different* DSN, and again with the variable *cleared*, answers `(cached)` both times. Two consequences. A gate that exists to exercise a particular provisioning path, or to assert a property of the run's *conditions* rather than of the code, must pass `-count=1` or it will be answered from a run that had neither — which is why `make test-fallback` carries it and why **both** of `test-contention`'s children do, the foreground race gate included: that gate's exit status is what the probe reports whenever the instrument is sound, and a cached one is a verdict about a run that saw no load. And the ordinary gates keep replaying as they always did — a cached pass is a real pass from an identical earlier run, and nothing about the shared server changed that.
- **The fallback path is still real, and it has its own gate.** A bare `go test ./...` with `LAB_GAME_TEST_DSN` unset starts one container per database-backed test binary, exactly as before. `make test-fallback` is that invocation with the variable explicitly cleared, and CI runs it in a job of its own, Test fallback — without it the path would be true on the day it was checked and unchecked forever after, since no default gate executes it any more. The container start is retried, because testcontainers shares one Ryuk reaper across the binaries `go test ./...` runs in parallel and a binary that meets it mid-creation fails its whole package.
- **Write a test that survives a neighbour, because the shared server means it has some.** Every wall-clock constant in a database-backed suite is one of two things, and they are treated oppositely. An **instrument** is a patience budget — how long a test waits for a condition, or a deadline that must *not* fire for the subject to be observable — and it is made generous, or replaced by polling on the condition with a generous ceiling; widening one changes no asserted proposition. The **subject** is the property under test — a backoff bracket, a deadline breach — and it stays exact, with its reference instants read from the **database clock around the operation** so the bracket widens with the load instead of the assertion failing. Two rules follow: widen **per test**, never by editing a shared config value, since one constant can be an instrument for one suite and the subject of another (`TaskTimeout` is both, and the worker re-applies it server-side as `statement_timeout`); and never widen until the test asserts nothing.
- **`make test-contention` is how membership of that class is established rather than guessed.** It sizes one server for two clients at the same pinned parallelism, loads it with the database-backed packages' own tests in a `-count=1` loop, and requires the whole-module race gate to stay green underneath, both children logging to files under the scratch directory. A red run is classified before it is believed, and a small built classifier does that rather than leaving it to whoever reads the logs: it scans both child logs for the shared server's own failure signatures — exhausted connections, exhausted disk, crash recovery or shutdown — across the literals and SQLSTATE codes each one prints, and, after the children have exited, it also probes the shared server directly for liveness, because a way the server dies without leaving one of those signatures behind is still a way the run says nothing about contention. Either kind of finding prints `INSTRUMENT FAILURE` naming the class and exits **2**; a clean run prints that the instrument was sound and passes the race gate's own status through unaltered. Exit 1 is the race gate's own verdict; exit 2 means the classifier, not the tests. The target builds this classifier and `cmd/testpg` into the scratch directory and runs them directly rather than through `go run`, because `go run` flattens any non-zero child exit status to 1 — the exit-2 half of this contract would never reach the target's own caller otherwise.
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
  mechanical half — listing a package's non-test files, walking a subtree, parsing, deciding which
  directories the go tool itself never descends into, and writing a scratch package into
  `t.TempDir()`. **Every predicate stays in the package that owns the
  proposition**, so a rule has exactly one place to look for it. *Scoped exception:*
  `internal/tg/guards_test.go`'s `walkGoFiles` is the third package's copy and was deliberately left
  hand-rolled — migrating it was out of the scope of the task that introduced `internal/srcguard`
  and only `internal/health` and `internal/ingest` moved onto it. The rule records no violation
  where the tree has not been migrated yet.
- **The repository root arrives as an argument**, resolved by `internal/repotest` — the module's
  one file-location-ascent resolver, itself held by a whole-tree walk that fails on any second one.
- **Prove the guard discriminating in the same file.** A guard that returns clean is a claim about
  the *instrument* until you have watched it go red: pair each one with a case that runs the same
  walk over a scratch package which *does* contain the forbidden construct, and asserts it is
  found. Without that pair, a walk over an empty file list and a walk over a clean tree are
  indistinguishable.

## Goroutine-leak detection

Every test binary ends with a check that no goroutine outside its package's declared ignore set is
still running once the package's tests have finished. A leak fails that package on every route
that runs tests, CI's included, so it is a failed gate rather than a finding review has to make.
The decision, its composition and its rejected alternatives are
[`key-decisions.md`](key-decisions.md) KD-36; what an author needs is below.

- **The `TestMain` form — one statement, with the runner named.** Every package with tests
  declares exactly one `TestMain`, in one of two forms:

  ```go
  func TestMain(m *testing.M) {
  	os.Exit(leaktest.Main(m, testdb.Main))      // a database-backed package
  }

  func TestMain(m *testing.M) {
  	os.Exit(leaktest.Main(m, (*testing.M).Run)) // every other package
  }
  ```

  The second argument is the runner: whatever owned the entry point before the check — the
  Postgres provisioner, or the standard library's own `M.Run`. `leaktest.Main` hands `m` to it and
  checks only after it returns, so on the fallback route the check looks at the process after
  `testdb.Main` has stopped the container it started. Either test package of a directory may hold
  the declaration, internal or external; the guard below counts them together.
- **When it runs, and how long it waits.** Once per test binary, after all of its tests, and only
  when the runner returned zero: a failing run already fails the package, so the runner's exit
  code passes through unchanged and nothing is checked. The only patience is goleak's own bounded retry
  inside `goleak.Find`, which `v1.3.0` offers no option to change; `internal/leaktest` adds no
  window of its own. A goroutine still exiting when that retry runs out is reported. The answer is
  an owner that ends it at cleanup — never a wider window, which would let a goroutine that
  outlives its test by seconds pass as clean.
- **What it reports.** One `leaktest:` line saying goroutines were still running after the
  package's tests finished, then goleak's report: each goroutine's state, its full stack and its
  `created by` line. It goes to standard error, and `go test` prints a failing binary's output, so
  the stack is in the gate log with nothing further to run.
- **What a detection looks like in a gate log, and how to reproduce it.** The package's `FAIL`
  line arrives with **no** `--- FAIL:` test line: every test passed, the binary printed its own
  `PASS`, and the check failed after it. There is no test name to hand to `-run`, so a
  one-named-test reproducer does not apply. Reproduce the whole package, with `-count=1`, on the
  route it failed on:
  - `make test` — `go run ./cmd/testpg -- go test -count=1 ./<pkg>/`
  - `make test-race` — `go run ./cmd/testpg -- go test -race -count=1 ./<pkg>/`
  - `make test-fallback` — `LAB_GAME_TEST_DSN= go test -count=1 ./<pkg>/`

  `-count=1` because whether a goroutine is still running at the check can depend on timing, and a
  cached pass — which ignores the DSN, so it may even come from the other route — would answer
  instead of a fresh run. Never narrow it with `-run`: the test that started the goroutine may be
  the one filtered out, and a filtered run also switches off the per-entry evaluation below.
- **The ignore set — declared at the package's own `TestMain`.** An entry is
  `leaktest.Ignore{Function, Anywhere, Reason}`, passed after the runner. `Function` is the fully
  qualified function goleak matches; with `Anywhere` false it must be the top frame (goleak's
  `IgnoreTopFunction`), with `Anywhere` true any frame (`IgnoreAnyFunction`); `Reason` states why
  the goroutine is not a leak. An entry applies to its package's test binary on every route and is
  deleted with the package — there is no central table.
- **Admission — judged in review.** An entry is for a goroutine that neither this module's code
  nor its test fixtures can stop: one a dependency starts for the life of the process. A keep-alive
  connection a test opened, a pool a test did not close, a handler still running — each has an
  owner that can end it, and ending it is the fix. A dependency goroutine that only CI's container
  runtime produces is a **STOP** for an owner decision, never an entry added on CI's evidence
  alone: an entry cannot be scoped to a runtime, and an unscoped one fails every local run as not
  needed.
- **Refusals — before any test runs.** `leaktest.Main` returns non-zero without calling the
  runner, naming the refusal, when the runner is nil, when the test binary's build information
  cannot name its module path, or when an entry has a blank `Reason`, a blank `Function`, or a
  `Function` that is this module's own code — a name beginning with the module path followed by
  `/` or `.`. That one prefix covers the commands too: inside a test binary a `package main`
  function is named by its import path, not by `main`. The module path is read from the binary's
  build information, never from a literal.
- **Per-entry evaluation — each entry is checked by leaving it out.** After a clean check with the
  whole set, `leaktest.Main` runs `goleak.Find` once more per entry, with that entry removed:
  - nothing reported → the entry is **not needed**: it matches no goroutine that another entry
    does not also match — stale after a dependency move, or redundant beside another entry (two
    entries matching the same goroutine are both reported);
  - something reported, with a frame or a `created by` line naming this module's code → the entry
    **excuses this module's own code**, and the report carries those stacks.

  Either fails the package. goleak stays the only matcher, so "matches" means what goleak means.
  Each entry in use costs one full goleak retry in its binary's check.
- **The filtered-run skip.** Under `-run`, `-skip`, `-list` or `-short` a package's tests may never
  start the goroutine an entry exists for, so the per-entry evaluation is skipped, with one
  `leaktest:` line saying so; the leak check itself still runs. No gate filters — `make test`,
  `make test-race`, `make test-fallback`, `make test-contention` and the coverage ratchet pass none
  of those flags — so a stale entry that survives a developer's filtered run fails the next gate.
- **The guard — coverage is enforced, not remembered.**
  `TestGuard_EveryPackageWithTestsRunsTheDetection` in `internal/leaktest` fails the suite when a
  directory holding tests lacks the form above, naming each such directory with the build
  configuration and the reason. Under each configuration in which a directory has tests, it
  requires exactly one `TestMain` across the test files that configuration compiles — internal and
  external test package together — whose body is the single statement
  `os.Exit(leaktest.Main(<its own parameter>, <runner>, <entries…>))`, with `os` and `leaktest`
  resolved through that file's own imports, aliases included. No `TestMain`, two of them, one that
  discards the result, one that passes something other than its own parameter, one with a second
  statement, and a correctly shaped one in a file that configuration never compiles all fail it;
  `TestGuard_scratch` holds each of those as a discriminating case beside the shapes that pass.
  - **It walks in-process, and that is load-bearing.** The walk goes through
    `srcguard.WalkSubtree` — `filepath.WalkDir` in the test's own process — and skips the
    directories the go tool ignores: `testdata`, and any whose name begins with `.` or `_`.
    `go test` replays a cached pass unless an input the test itself opened has changed; a
    directory the test listed is such an input, and files an exec'd child reads are not. A guard
    that enumerated packages through `go list` would let a package added later pass on a cached
    run; this one re-runs and fails, naming it.
  - **It asks `go/build` which files count**, rather than restating the go tool's file rules:
    `ImportDir`'s `TestGoFiles` and `XTestGoFiles` already apply a file name beginning with `.` or
    `_`, a `//go:build` line, a legacy `// +build` line, and a GOOS or GOARCH filename suffix.
    `go/build` reads those files in the test's own process too, so a constraint added to a
    `TestMain` file invalidates a cached pass. A directory where both lists are empty —
    `ImportDir` answering `*build.NoGoError` — has no test that configuration compiles and is
    skipped under it; any other `ImportDir` error fails the guard.
  - **Under two configurations, because the gates compile two.** `build.Default` carries no build
    tags, but `make test-race` and `make test-contention`'s foreground run pass `-race`, which
    satisfies the `race` build constraint the default build does not know about: a `TestMain` in a
    `//go:build !race` file runs under `make test` and never under `make test-race`. So the guard
    classifies every directory under `build.Default` and under a copy with `race` added to its
    `BuildTags`, and the rule must hold under each. A gate that ever passes a tag of its own —
    `-tags`, `-msan` or `-asan` — needs a third configuration in the guard, added in the same
    change as the gate, or a package whose `TestMain` that gate excludes runs no detection there.
- **Fix a leak at the goroutine's owner.** A fixture that starts a goroutine ends it in its own
  cleanup. The one leak class found when the check was adopted shows the shape: a fake Bot API
  handler built by `tgtest.Delayed`, still sleeping after its test ended. `net/http` watches the
  connection for a client that went away only once the request's body has been consumed, and a
  `Delayed` handler never reads the body every consumer sends — so a client giving up never ended
  the handler's request context. `tgtest.New` therefore gives its `http.Server` a base context that its own cleanup
  cancels, and `Delayed` waits for its duration or for the request's context, whichever ends first.
  The base context is the fixture's own, not the test's `Context()`: that one is cancelled before
  **any** cleanup runs, so the fake server would stop answering while cleanups registered after
  `New` still use it.

## Panics

No `panic` / `log.Fatal` in production code. A `PostToolUse` hook flags them on write. Any survivor is justified in its doc comment **and** listed in [`panic-index.md`](panic-index.md) — the comment states the justification itself and does not point at the index, because a comment naming a markdown path is what the reference ban forbids ([`doc-convention.md`](doc-convention.md) § DOC-4). Tests may panic freely (`t.Fatal` is the idiomatic failure).

## What not to do

- Do not assert on log text; assert on returned values and on database state.
- Do not build a mock for something a fake in-memory implementation of your own interface can do more honestly.
- Do not test unexported behaviour through exported APIs by contorting the input — if it needs a direct test, the test belongs in the same package.
- Do not let a test depend on map-iteration order; sort before comparing.
