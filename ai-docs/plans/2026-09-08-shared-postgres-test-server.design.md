# Design: Shared PostgreSQL test server — one server for the whole suite, containers as the fallback

**Issue:** #67
**Spec:** `ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md`
**Branch:** `perf/2026-09-08-shared-postgres-test-server`
**Date:** 2026-09-08
**Round:** 1

**Tag forms used below.** A fact about something that already exists carries
`[measured <pin>:<path>[:<lines>] · <command> → <output>]`, where `<pin>` is `eef4c4e` for
this repository and a module version for an external package. A claim about an artefact
this task will create carries `[derived → <AC or test>]` and no coordinate, because there
is nothing yet to coordinate to.

## Approach

### The shape

Today every database-backed test binary starts its own container, and `testdb.Main`
already prefers an existing server: it reads the DSN environment variable and returns
`m.Run()` before it ever reaches testcontainers
`[measured eef4c4e:internal/testdb/testdb.go:75-113 · sed -n '75,113p' internal/testdb/testdb.go → the dsnEnv branch assigns baseDSN and returns m.Run() above the postgres.Run call]`.
So nothing about how a test *reaches* the database has to change. What has to exist is
something that provisions one server, puts its DSN in the environment of the process
that runs the gate, and takes the server away again.

That something is a **wrapper command**, `cmd/testpg`, invoked as
`go run ./cmd/testpg -- <the gate command>` `[derived → AC1]`. Its decision order:

1. The DSN variable is already set in the calling environment → run the child unchanged,
   provision nothing, remove nothing `[derived → AC2]`.
2. Otherwise, a **long-lived server this project's own targets started** is looked up
   through the locator D4 defines; if it answers, the child runs against it and it is
   left running afterwards `[derived → AC4's found-not-started clause]`.
3. Otherwise, an **anonymous** container is started with the ceiling D3 computes, the
   child runs against it, and it is removed on a passing run, on a failing gate and on
   an interrupted run `[derived → AC1, AC4]`.

A bare `go test ./...` reaches none of this and keeps the per-package container path
untouched `[derived → AC3]`, which is what makes the fallback a fallback rather than a
second implementation.

### Why a wrapper process rather than anything else

Three properties decide it, and only this shape has all three.

- **The container's lifetime is a process lifetime.** testcontainers' reaper filters
  on the labels of the session that created a container
  `[measured testcontainers-go@v0.44.0:reaper.go:557-566 · sed -n '557,566p' reaper.go → handshake sends core.DefaultLabels(r.SessionID) as the filter]`,
  and those labels carry the session id
  `[measured testcontainers-go@v0.44.0:internal/core/labels.go:36-49 · sed -n '36,49p' internal/core/labels.go → DefaultLabels always sets LabelSessionID and adds LabelReap only when the reaper is enabled]`.
  A process that starts a container and exits therefore has it reaped; a process that
  *stays alive for the whole gate* keeps the reaper as the crash net while removing the
  container itself on the ordinary paths. The wrapper is that process.
- **It reaches the runtime through the abstraction the suite already depends on**, not
  through a CLI. The permission list grants `Bash(podman *)` and names no `docker` entry
  `[measured eef4c4e:.claude/settings.json · jq -r '.permissions.allow[]' .claude/settings.json → Bash(go *), Bash(make *), Bash(psql *), Bash(podman *); no docker entry]`,
  and this machine's runtime is rootless podman reached over a socket
  `[measured eef4c4e · podman info --format '{{.Version.Version}}' → 5.8.2, with DOCKER_HOST naming the rootless podman socket]`,
  which the CI runner's is not. A target that shelled out to a runtime CLI would have to
  pick a binary name and would pick a name that is right on one host and absent on the
  other. testcontainers-go picks no name at all: it talks to the socket.
- **It is callable from outside `make`.** The pre-commit hook execs the coverage-ratchet
  script directly
  `[measured eef4c4e:.githooks/pre-commit.sh:46 · sed -n '46p' .githooks/pre-commit.sh → exec "$root/.githooks/coverage-ratchet.sh"]`,
  so the provisioning has to be reachable by a shell script, which `go run ./cmd/testpg --`
  is `[derived → AC6]`.

The precedent for a Go command as project tooling is `cmd/commentrefs`, whose package
comment shows the shape a `main` package's doc comment takes here
`[measured eef4c4e:cmd/commentrefs/main.go:1-4 · head -4 cmd/commentrefs/main.go → "// Command commentrefs is the comment reference gate: …" above package main]`.

### What the win actually is, and how to re-measure it

The instruction, not a stored figure: time an uncached whole-module run
(`go test -count=1 ./...`) with the DSN variable unset, then again with it naming one
server sized for every database-backed package at once, and take the difference. Run it on
the tree that exists at implementation time — nothing here stores the result, because a
wall clock is true of one machine on one afternoon
`[measured eef4c4e · go test -count=1 ./... run in both regimes, wall clock via time(1) → the shared regime's wall clock is a small fraction of the container regime's, and go test -count=1 ./internal/testdb alone on the container path is dominated by the container lifecycle rather than by that package's own tests]`.
The difference has two components and the design has to serve both. One container lifecycle
is most of the per-package cost, and the database-backed binaries pay it in parallel — so
provisioning one container per *gate invocation* collapses those lifecycles into a single
one, while reusing a server *across* invocations removes even that one. Decision D4 exists
for the second component; without it the pre-commit hook and every `make test` still pay
one container start.

### Rejected alternatives

- **`testdb.Main` reuses a named shared container itself**, so a bare `go test ./...`
  gets one server with no wrapper at all. Refused: AC3 requires the bare invocation to
  keep starting one container per database-backed binary, and the shape has no honest
  answer to "which binary removes it" short of cross-process refcounting.
- **A shell script driving `podman` or `docker`.** Refused on the permission grant and
  the host divergence above, and because the image tag, the tmpfs arguments and the
  credentials would then exist in a second place beside `internal/testdb`.
- **A `services:` block in CI.** Refused by the spec, and by the Makefile's own stated
  invariant that CI invokes the same sub-targets so a local run and a CI run cannot
  disagree about what any gate's command is
  `[measured eef4c4e:Makefile:1-13 · sed -n '1,13p' Makefile → "CI never runs `verify` — it invokes the same sub-targets … so a local run and a CI run cannot disagree about what any gate's command is"]`.
- **A fixed host port for the ad-hoc container.** Refused twice over: it collides with a
  developer's own PostgreSQL, and testcontainers v0.44.0 parses every exposed-port entry
  with a range parser that accepts only `port[-port][/proto]`
  `[measured testcontainers-go@v0.44.0:lifecycle.go:596-608 · sed -n '596,608p' lifecycle.go → parseExposedPorts calls network.ParsePortRange on each spec]`
  `[measured moby/api@v1.55.0:types/network/port.go:187-213 · sed -n '187,213p' types/network/port.go → ParsePortRange cuts on "/" then "-" and parses port numbers; no host:container form]`,
  so a fixed binding would have to go through `HostConfigModifier` and would promote
  `github.com/moby/moby/api` from an indirect requirement to a direct one for the sake of
  one port. The locator D4 defines needs neither.
- **Lowering the per-pool cap instead of raising the server's ceiling.** Refused by the
  spec's key decision, and the arithmetic in D3 agrees: the cap is still correct for the
  fallback path, where one server serves one binary.
- **Holding one server across CI's suite-executing steps.** The Test job invokes
  the test, race and coverage-ratchet targets as separate steps
  `[measured eef4c4e:.github/workflows/ci.yml:114-131 · sed -n '114,131p' .github/workflows/ci.yml → three run steps: make test, make test-race, make cover-ratchet; no services: block]`,
  so a server whose lifetime is one target is provisioned once per step. Holding one
  across them means a bring-up step and an `if: always()` teardown step that only CI
  runs — a CI-only lifecycle order whose failure mode is a leak nobody sees. The spec
  recorded it as deferred and admits it only "if it is free"; it is not free, so it stays
  a follow-up and the per-step lifecycle is the accepted baseline.

## Key decisions

**D1 — `cmd/testpg`, the wrapper, and its modes.** Default mode runs a child
command against a provisioned or discovered server; `--up` creates the long-lived server
and leaves it running; `--down` removes it. The child's argument vector follows a literal
`--` separator so the wrapper's own flags stay unambiguous. Standard streams pass through
untouched, and a non-zero child yields a non-zero wrapper status — but not the child's own
number: run under go run, a program exiting with a code reports exit 1 and prints an
"exit status N" line on stderr
`[measured go1.26.5 · a program calling os.Exit(7), run under go run . and then built and executed → exit 1 with "exit status 7" on stderr under go run; exit 7 from the built binary]`.
Zero stays zero and non-zero stays non-zero, so no gate can mis-report green and no status
is routed through a pipe `[derived → AC15]`. No gate in this design reads the exact code —
D12's recipe is explicitly forbidden to — and the "exit status N" line landing inside the
ratchet's captured log is documented rather than removed; where an exact code ever had to
survive, the wrapper would be built once into the scratch directory instead of run through
go run. The wrapper never reads a gate's output and never summarises it. Interrupt handling: the wrapper installs a signal context for
interrupt and terminate, and performs container teardown under a *fresh* background
context with the package's existing terminate timeout, so a cancelled run still tears
down `[derived → AC4]`. If the wrapper itself is killed outright, the reaper removes the
container, because the ad-hoc container is created with the reaper enabled and therefore
carries this session's labels
`[measured testcontainers-go@v0.44.0:internal/core/labels.go:36-49 · sed -n '36,49p' internal/core/labels.go → LabelReap is added when the reaper is enabled]`.
The invocation is `go run ./cmd/testpg -- <command>`; `gosec` will flag the child
`exec.Command` with a variable argument vector, so that call carries a specific,
explained `//nolint` — the lint config requires both
`[measured eef4c4e:.golangci.yml:42-48 · sed -n '42,48p' .golangci.yml → nolintlint require-explanation: true, require-specific: true]`.
No `panic`, no `log.Fatal`: the wrapper returns an exit code, keeping the panic index
empty `[measured eef4c4e:ai-docs/panic-index.md · cat ai-docs/panic-index.md → the table holds one em-dash placeholder row and the page states the project holds zero production panics]`.

**D2 — the provisioning API moves into `internal/testdb`, and `Main` keeps its
behaviour.** One package already owns the image, the credentials, the tmpfs settings, the
wait strategy and the start retry
`[measured eef4c4e:internal/testdb/testdb.go:24-162 · sed -n '24,162p' internal/testdb/testdb.go → the Image constant, the tmpfs mount with its PGDATA override, the database/user/password constants, BasicWaitStrategies, and retryRun around postgres.Run]`;
a second copy in `cmd/testpg` would be the duplication the project's own rule refuses. So `internal/testdb` gains an exported
`StartServer(ctx, ServerOptions) (*Server, error)` with `DSN` and `Stop`, an exported
`Reachable(ctx, dsn) error` probe, an exported name for the DSN environment variable, and
`Main`'s container branch is re-expressed in terms of `StartServer` with an empty options
value — same image, same tmpfs, same wait strategy, same retry, same terminate timeout,
same image-default connection ceiling, so the fallback path is unchanged in effect
`[derived → AC3, and the fallback gate D8 adds]`. `ServerOptions` carries a container
name (empty means anonymous) and a connection ceiling (zero means the image default).
Every new exported item takes a doc comment beginning with its own name and the package
keeps its package comment, because `revive`'s `exported` and `package-comments` rules are
enabled `[measured eef4c4e:.golangci.yml:45-48 · sed -n '45,48p' .golangci.yml → revive rules exported and package-comments]`.
**Consequence for KD-20:** `internal/testdb` stops being Go that only `_test.go` files
import. The invariant that actually matters — the container runtime never enters
`cmd/bot`'s import graph — is untouched, and D10 rewrites the clause that states the
wrong version of it `[derived → AC14]`.

**D3 — the connection ceiling, term by term, and the population it is a bound on.** The
ceiling the wrapper passes to a server it starts is computed, not a literal, because its
terms scale with the host's CPU count and with how many test runs share the server, so a
constant sized for one host and one run is wrong on another:

```
ceiling = clients × binaries × parallel × (schemaMaxConns + 1) + slack,
          clamped to [imageDefaultCeiling, ceilingMax]
```

| Term | What it is | Source |
|---|---|---|
| `clients` | whole-module test runs this one server must serve **at the same time**. Every gate target passes one; the contention probe is the only target that deliberately runs two at once and passes its own value | `[derived → AC5, AC10, and D12's recipe]` |
| `schemaMaxConns` | the cap `testdb.Schema` sets on every per-test pool | `[measured eef4c4e:internal/testdb/testdb.go:164-172 · sed -n '164,172p' internal/testdb/testdb.go → schemaMaxConns = 4, commented as 16 parallel subtests × 4 against max_connections = 100]` |
| `parallel` | tests running simultaneously inside one binary | `[measured eef4c4e · go help testflag → "By default, -parallel is set to the value of GOMAXPROCS"]` — obtained as described below, not assumed |
| `binaries` | database-backed test binaries that can run at once; bounded above by `go test -p` | `[measured eef4c4e · go help build → "-p n … the number of programs, such as build commands or test binaries, that can be run in parallel. The default is GOMAXPROCS"]` and `[measured eef4c4e · grep -rn 'testdb.Main' --include='*.go' . → the TestMain of internal/store, internal/scheduler, internal/ingest and internal/testdb, plus the Fatalf text inside testdb.go itself]` |
| `+ 1` per running test | `Schema`'s transient admin pool and its cleanup dropper pool, one connection each | `[measured eef4c4e:internal/testdb/testdb.go:189-236 · sed -n '189,236p' internal/testdb/testdb.go → Schema opens an admin pool to CREATE SCHEMA and a dropper pool in tb.Cleanup, both closed immediately]` |
| `slack` | the server's own reserved slots, plus the one test that raises its pool cap above `schemaMaxConns` | `[measured docker.io/library/postgres:18, session-local probe · psql -At -c "select name||'='||setting from pg_settings where name in ('superuser_reserved_connections','reserved_connections')" → superuser_reserved_connections=3, reserved_connections=0]` and `[measured eef4c4e:internal/store/post_race_test.go:73-75 · sed -n '73,75p' internal/store/post_race_test.go → const workers int32 = 8; cfg.MaxConns = workers]` |
| `imageDefaultCeiling` | the image's own `max_connections`, the floor below which computing a smaller number would *reduce* what the server already offers | `[measured eef4c4e:internal/testdb/testdb.go:164-172 · sed -n '164,172p' internal/testdb/testdb.go → the per-pool cap's own comment records max_connections = 100 as the image ceiling it was sized against]` |

**How `parallel` is obtained, and why it is not simply read.** The wrapper's default is
`runtime.GOMAXPROCS(0)`, which is correct only while the child runs at the toolchain's
defaults; `GOFLAGS`, or a target's own `-p` / `-parallel` flags, move the child's effective
values without the wrapper seeing them. So the wrapper takes an explicit `--parallel`
override alongside `--clients`, and **any target that pins the child's parallelism passes
the matching value** — D12's probe is the target that does `[derived → AC5]`.

`binaries` is a named constant in `internal/testdb` whose doc comment states that it
counts the test binaries provisioning a database through this package, and subtask 2's
manifest test keeps it honest against the tree rather than against memory `[derived → AC5]`.
`slack` is **32** and `ceilingMax` is **500**: the first covers the reserved slots and the
raised-cap test with room, the second bounds what a many-core host can ask a container's
shared memory for.

**The bound is load-bearing, not decoration.** It is the number the server is actually
started with, so a population that exceeds it does not degrade gracefully: it fails with a
named condition, which D12 turns into a discriminator rather than a mystery. That is why
every target that changes the population gets a term rather than an exemption, and why the
rule when the clamp binds is to **bound the population** — fewer clients, a lower
`-parallel` passed to both the child and the wrapper — and never to raise the ceiling past
what a container's shared memory should be asked for. D12 wires that rule into the probe's
recipe instead of leaving it as prose here.

The ceiling reaches the server as a command argument: the
postgres module sets the container command and then applies caller options, and the
append-form option appends to it
`[measured testcontainers-go/modules/postgres@v0.44.0:postgres.go:155-166 · sed -n '155,166p' postgres.go → moduleOpts appends WithCmd("postgres", "-c", "fsync=off") before appending the caller's opts]`
`[measured testcontainers-go@v0.44.0:options.go:490-504 · sed -n '490,504p' options.go → WithCmd replaces req.Cmd, WithCmdArgs appends to it]`.
The **fallback path keeps the image default**, because there one server serves one
binary and the existing comment's arithmetic still holds `[derived → AC3]`.

**D4 — the locator, and the bring-up / take-down pair.** AC18 requires the shared server
to be addressed by a locator this change itself defines, and the spec's lifetime decision
delegates the pair outright — "whether an explicit bring-up / take-down pair exists so a
developer can keep one across commits is the design's call" — while fixing the rule that
makes either shape safe: remove only a server this invocation started, never one it found.
**The pair exists, and that is settled here rather than left open**: the delegation is the
spec's own, so taking it amends nothing. The locator is a pair —
a **container name** constant in `internal/testdb`, and a **DSN file** written under the
ignored scratch directory `[measured eef4c4e:.gitignore · cat .gitignore § Scratch → /tmp/ is the one ignored scratch directory every gate log and throwaway probe is written to]`.
`--up` creates the named container (reusing one already under that name), writes the DSN
file and prints the DSN; `--down` removes the container and the file. The wrapper's
default mode reads the file, **probes the DSN before trusting it**, and falls through to
its own anonymous container when the probe fails — so a file left behind by a reboot or by
another worktree's `--down` degrades into the ad-hoc path with a message, never into a run
against nothing `[derived → AC4]`. `--up` runs with the reaper disabled, which is what
lets its container outlive the process that created it; a container created that way
carries no reap label and a foreign session id, so no reaper will ever remove it and
`--down` is its only remover
`[measured testcontainers-go@v0.44.0:reaper.go:557-566 · sed -n '557,566p' reaper.go → the handshake filter is core.DefaultLabels(r.SessionID)]`.
That is exactly the spec's "a container this project's own target started is not the
reaper's to remove". **Why the pair earns its place:** the environment variable alone
cannot carry a server across an agent's tool calls, because shell state does not persist
between them, and the pre-commit path — in scope by the owner's round-1 answer — runs in
whatever environment git hands it. Without the locator the whole coverage-ratchet path
pays a container start on every commit that stages Go.

**D5 — the ad-hoc container is anonymous, and that is what makes concurrent runs safe.**
The wrapper never creates the *named* container; only `--up` does. So the only container
the wrapper can remove is one no other run can have found, and two gate runs at the same
time either both discover the long-lived server (and share it — schemas are already
uniquely named per test `[measured eef4c4e:internal/testdb/testdb.go:189-236 · sed -n '189,236p' internal/testdb/testdb.go → Schema creates t_<unix-nanos>_<counter> and drops it in tb.Cleanup]`)
or each start their own. Neither run can pull a server out from under the other
`[derived → AC4]`.

**D6 — the coverage-ratchet path, and where the provisioning goes in it.** The script
skips silently when nothing coverage-moving is staged, and the measurement runs after that
decision `[measured eef4c4e:.githooks/coverage-ratchet.sh:86-119 · sed -n '86,119p' .githooks/coverage-ratchet.sh → the raise-mode staged check exits 0 before the go test -coverprofile invocation]`,
so wrapping **the measurement command only** satisfies both criteria at once: a commit
that stages a coverage-moving file measures against a shared server `[derived → AC6]`, and
a commit that stages nothing coverage-moving still starts nothing `[derived → AC7]`. The
existing redirect to a log file under the scratch directory is preserved verbatim, so the
gate's exit status still goes nowhere near a pipe `[derived → AC15]`. The script's advice
for a container-runtime failure names only the environment variable today
`[measured eef4c4e:.githooks/coverage-ratchet.sh:116-118 · sed -n '116,118p' .githooks/coverage-ratchet.sh → "point the suite at a running server: export LAB_GAME_TEST_DSN=postgres://..."]`;
it gains the bring-up target as the first suggestion `[derived → AC14]`.

**D7 — the coverage regime shifts, and the recorded mark is re-centred in the same
commit.** Under the shared path `internal/testdb`'s own binary takes the DSN branch, so
its container-provisioning statements stop being executed by its own tests — and coverage
is attributed per package from that package's own tests
`[measured eef4c4e:.githooks/coverage-ratchet.sh:10-13 · sed -n '10,13p' .githooks/coverage-ratchet.sh → "each package covered by ITS OWN tests — the Go default … -coverpkg is deliberately not used"]`.
The shift is small, systematic, and consumes most of the band the tolerance exists to
absorb. It does **not** make the mark fall past the tolerance, so nothing blocks today; what
it leaves is a margin narrower than the run-to-run drift the tolerance was sized for
`[measured eef4c4e · go test -count=1 -covermode=atomic -coverprofile ./... run in both regimes, totals summed from the profile with awk → the shared regime's total is lower, it still holds against the recorded mark inside the tolerance, and every block that differs is in internal/testdb's container branch]`
`[measured eef4c4e:.githooks/coverage-ratchet.sh:61 · sed -n '61p' .githooks/coverage-ratchet.sh → TOLERANCE_PP=0.60 is the whole band]`.
Left alone, ordinary commits start blocking as soon as an unlucky draw lands. So the
recorded value is re-centred on a value **measured on the final tree** — the implementor
takes it with `-count=1` in the shared regime, and no figure is carried here to copy — with
the reason in the commit message, which is one of the two exits the workspace rules
sanction. Because the shift holds inside the tolerance rather than blocking, the re-centre
does not have to share a commit with the code that caused it, which is what lets it sit in
its own group (§ Handoff plan). Re-measure in the CI environment too: the runner's core
count differs, and the tolerance's drifting statements are timing-dependent `[derived → AC17]`.

**D8 — the fallback gets a gate, because nothing else executes it any more.** D7 is the
evidence: after this change no default gate runs the per-package container path, so AC3
would be true on the day it was checked and unchecked forever after. `make test-fallback`
runs the whole-module bare invocation with the DSN variable explicitly cleared, and CI's
Test job runs it as a step. It is not part of `verify`, exactly as the coverage ratchet is
not `[measured eef4c4e:Makefile:71-75 · sed -n '71,75p' Makefile → the cover-ratchet target is check-only and deliberately outside verify]` `[derived → AC3]`.

**D9 — contention tolerance: every wall-clock constant is either an instrument or the
subject, and they are treated oppositely.** The recorded trap is not retracted by this
design; it is the work `[measured eef4c4e:ai-docs/context-status.md:161 · grep -n 'assumes exclusive database access' ai-docs/context-status.md → the entry names reconcile, worker, deadline and observe tests failing about half the time against one shared server, green run after run under per-package containers, and invisible to CI for want of a services: block]`.
The rule the fixes follow:

- **An instrument** is a patience budget — how long a test waits for a condition, or a
  deadline that must *not* fire for the test's subject to be observable. Examples on this
  tree: the reconcile test's run-context bound
  `[measured eef4c4e:internal/scheduler/reconcile_test.go:356 · sed -n '356p' internal/scheduler/reconcile_test.go → context.WithTimeout(context.Background(), 500*time.Millisecond)]`,
  the lock-free wait's ceiling
  `[measured eef4c4e:internal/scheduler/deadline_test.go § waitLockFree · sed -n '73,98p' internal/scheduler/deadline_test.go → polls every 10 ms until the row can be locked, failing at a caller-supplied timeout]`,
  the shared scheduler task timeout that non-deadline tests rely on never breaching
  `[measured eef4c4e:internal/scheduler/worker_test.go § testConfig · sed -n '24,34p' internal/scheduler/worker_test.go → TaskTimeout: time.Second, PollInterval: 50 ms]`,
  and the fixed waits in the observation, failure and ingest retry suites
  `[measured eef4c4e · grep -rn 'time.Sleep\|WithTimeout\|time.After' internal/scheduler/*_test.go internal/ingest/*_test.go internal/store/*_test.go → observe_test.go and retry_test.go carry fixed sleeps and two-second deadlines, and failure_test.go's slow-first-attempt handler waits on time.After(h.firstDelay)]`.
  The enumeration illustrates the class; subtask 8's mandate is to classify **every**
  wall-clock constant in the database-backed suites, not only these.
  An instrument is made generous, or — better — replaced by polling on the condition with
  a generous ceiling. **This changes no asserted proposition**: the reconcile test still
  asserts that reconciliation seeded exactly one row before the first cycle; it simply
  stops asserting that the database answered inside half a second while other packages
  hammered the same server.
**One constant is both, and that decides how it is widened.** `cfg.TaskTimeout` is not
only a Go-side context deadline: the worker re-applies it server-side as
`statement_timeout` and `idle_in_transaction_session_timeout` for the task's transaction
`[measured eef4c4e:internal/scheduler/execute.go:33,75 · sed -n '33p;75p' internal/scheduler/execute.go → setTimeoutsSQL sets both from one bound parameter, and the parameter is w.cfg.TaskTimeout in milliseconds]`,
which is exactly the `statement_timeout` failure the trap entry records. For the worker and
observation suites it is an **instrument** — it must not fire — while for the deadline suite
it is the **subject**, because those tests exist to observe it firing. So the widening is
**per test**, never an edit to the shared `testConfig()` or to `shortDeadlineConfig()`: a
test that must not breach raises its own copy, and the deadline suite's values are left
exactly as they are. Blunting the deadline suite to buy tolerance elsewhere would be the
"widen until it asserts nothing" this rule exists to refuse `[derived → AC10, AC12]`.

- **The subject** is the property under test — a backoff bracket, a deadline breach. It is
  kept exact, and its reference instants are read from the **database clock around the
  operation**, so the bracket widens with the load instead of the assertion failing. The
  shipped model is already in the tree
  `[measured eef4c4e:internal/scheduler/deadline_test.go § TestDeadline_successiveBreaches_growingDelay · sed -n '268,274p' internal/scheduler/deadline_test.go → run_at is required within [drainInstant+want, after+want], both instants read with clock_timestamp() around the drain]`,
  and it is the shape every remaining timing assertion is moved to.

Neither route is "widen until it asserts nothing", and neither is "serialise the binaries",
which would give back the speedup without making any test more honest. **Membership is
not the set of tests the trap entry names**: it is every test whose result changes when another package's
tests run concurrently against the same server, and the probe below is how membership is
established rather than guessed `[derived → AC12]`.

**D10 — the propagation class, what the sweep returns, and what it returns that is not a
member.** The class is `AGENTS.md` § *Propagation Rule* step 4's: every live site whose
claim this diff falsifies. The sweep returns more files than the class has members, so both
lists are recorded rather than only the conclusion
`[measured eef4c4e · grep -rni 'LAB_GAME_TEST_DSN\|testcontainers\|container per' --include='*.md' --include='*.sh' --include='*.yml' --include='*.json' . with ai-docs/plans and tmp excluded → AGENTS.md, ai-docs/context.md, ai-docs/context-status.md, ai-docs/dependency-versions.md, ai-docs/go-test-conventions.md, ai-docs/key-decisions.md, ai-docs/scripts/test-ac-shape.sh, docs/DESIGN.md, .githooks/coverage-ratchet.sh]`.

**Members** — each states something this diff falsifies: `ai-docs/key-decisions.md` § KD-20
(the provisioning story and the importer clause, which is also where AC13's arithmetic
lands); `ai-docs/go-test-conventions.md` § *Postgres is tested against Postgres*;
`AGENTS.md` § *Build & Test* (the target list and the coverage-ratchet table's
container-runtime row); `.githooks/coverage-ratchet.sh`'s runtime advice; and
`ai-docs/context.md`, whose layout paragraph describes `internal/testdb` as PostgreSQL
provisioning **for package tests**
`[measured eef4c4e:ai-docs/context.md:27 · grep -n 'LAB_GAME_TEST_DSN' ai-docs/context.md → "internal/testdb — PostgreSQL provisioning for package tests (a postgres:18 container or LAB_GAME_TEST_DSN, one schema per test)"]`
— which D2 makes incomplete the moment `cmd/testpg` imports the package.

**Returned but not members**, each with its reason so the next reader does not re-derive
it: `ai-docs/dependency-versions.md` matches on `testcontainers-go` used as an example
inside the dependency-reason rule; `ai-docs/scripts/test-ac-shape.sh` matches inside a gate
fixture's payload string, where the text is the thing under test; `docs/DESIGN.md` matches
in the Russian design corpus's testing note, which is DECISIONS and is not redesigned here.
Two more exclusions, each stated so nobody re-derives them:
`ai-docs/context-status.md` is append-only by its own header
`[measured eef4c4e:ai-docs/context-status.md:1-5 · head -5 ai-docs/context-status.md → "The detailed, append-only implementation log … Written by /task Step 9.5"]`,
so the trap entry is superseded by the new entry Step 9.5 writes, never edited; and the
harness instruction files that tell an agent to run a bare `go test ./...` state nothing
this diff falsifies — the bare invocation still works, it is simply the fallback by AC3's
own design. Sweeping them onto `make test` is a real ergonomics gap and is recorded as a
follow-up rather than absorbed here.

**D11 — no CI change filter edit is expected, and the check is part of the work.** Every
artefact this change adds is a Go file, the Makefile, the coverage-ratchet script or the
workflow itself, and the filter already names each class
`[measured eef4c4e:.github/workflows/ci.yml:38-82 · sed -n '38,82p' .github/workflows/ci.yml → go: '**/*.go', 'Makefile', '.github/workflows/**'; harness: '.githooks/**', 'Makefile', 'ai-docs/**'; commentrefs: '**/*.go', '**/*.sh', 'Makefile', '.githooks/**']`.
The contention probe is therefore a **Makefile target**, not a new script in a new
directory: a new path class would have to be registered, and the target needs registering
nowhere. The obligation still has to be *discharged by looking*, not by trusting this
paragraph — the implementor re-runs the comparison against the final file list `[derived → AC9]`.

**D12 — the contention probe: its own client population, its clamp, and the demonstration
that it can fail.** `make test-contention` runs **one** wrapper invocation whose child
starts a load run in the background against the provisioned server and runs the race gate
over the whole module in the foreground, both logging to files under the scratch directory,
killing the load run afterwards and exiting on the foreground gate's status. The load is
the database-backed packages' own tests repeated — cross-package load by construction,
needing no second implementation of "work that hits Postgres". The recipe's shell must
capture the foreground status explicitly rather than letting the `-e` flag the Makefile
sets abort it
`[measured eef4c4e:Makefile:15-17 · sed -n '15,17p' Makefile → SHELL := /bin/bash with .SHELLFLAGS := -eu -o pipefail -c]`,
must branch on zero versus non-zero rather than on the value, because go run does not
surface the child's own code (D1), and pipes nothing `[derived → AC15]`.

**This is the one target that serves more than one test run at once, so it is the one
target that passes `--clients` above one.** Two runs against one server is two client
populations, and D3's formula has a term for exactly that. On a host where two clients push
the formula past `ceilingMax`, the clamp binds and the granted ceiling sits *below* the
formula's worst case — and the rule is then D3's, wired in here rather than left as prose:
the probe **bounds the population instead of raising the ceiling**. The load run is pinned
to small explicit `-p` and `-parallel` values, the same `--parallel` is passed to the
wrapper so the ceiling is computed against what the children will actually do, and the
target echoes the granted ceiling and both pinned values into the log, so the arithmetic a
run relied on is readable after the fact `[derived → AC5, AC10]`.

**A connection-exhaustion red is not a contention red, and the probe establishes which
before anything is recorded.** Exhaustion announces itself: Postgres refuses the connection
with `FATAL: sorry, too many clients already`, SQLSTATE `53300`. So the discharge step is
mechanical — both captured logs are scanned for that class, and a run whose logs carry it
is an **instrument failure, not a finding**: the ceiling or the pinned parallelism is
corrected and the run repeated. Such a red is not eligible to satisfy AC11 and such a green
is not eligible to satisfy AC10. Without this step the "demonstrated able to fail" evidence
would certify an instrument that may be failing for the wrong reason, which is the
§ Patterns 2 failure the spec invoked in the first place `[derived → AC10, AC11]`.

**The probe is an instrument until it has gone red.** Before any green result is recorded,
one instrument budget D9 widened is reverted over a `cp` backup, `make test-contention` is
required to go RED, that red is required to survive the exhaustion scan above, and only
then is the budget restored and a green run recorded. A green probe at that step is a
STOP, not a pass — the project has run exactly this protocol before
`[measured eef4c4e:ai-docs/context-status.md:157 · grep -n 'green probe was defined as a STOP' ai-docs/context-status.md → the scheduler mutation probe was written over a cp backup and required to go RED before the re-point]` `[derived → AC11]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | `internal/testdb`: exported provisioning API (`ServerOptions`, `StartServer`, `Server.DSN`, `Server.Stop`, `Reachable`, the DSN env-var name, the shared container name), the ceiling formula and its constants; `Main`'s container branch re-expressed over it with behaviour preserved | `internal/testdb/testdb.go` | — |
| 2 | Tests for the ceiling formula and the binaries-constant manifest | `internal/testdb/testdb_test.go` | 1 |
| 3 | `cmd/testpg`: the wrapper — decision order, locator read/write with the reachability probe, `--up` / `--down`, `--clients` / `--parallel` feeding D3's ceiling, the granted ceiling echoed to stderr, signal-aware teardown, non-zero-status passthrough, injectable provisioner seam | `cmd/testpg/main.go`, `cmd/testpg/run.go` | 1 |
| 4 | Wrapper tests over the injected seam: no container is started by this package's own tests | `cmd/testpg/run_test.go` | 3 |
| 5 | `Makefile`: route `test` and `test-race` through the wrapper; add `test-db-up`, `test-db-down`, `test-fallback`, and `test-contention` with its `--clients`, its pinned load `-p` / `-parallel`, the matching `--parallel`, and both logs under the scratch directory | `Makefile` | 3 |
| 6 | Coverage ratchet: wrap the measurement command, after the skip decision; update the runtime advice | `.githooks/coverage-ratchet.sh` | 3 |
| 7 | CI: add the fallback step to the Test job; verify every added artefact is already named in the change filter and record the comparison | `.github/workflows/ci.yml` | 5 |
| 8 | Contention tolerance per D9: classify every wall-clock constant in the database-backed suites, widen the instruments **per test** rather than through the shared config values, move the remaining timing assertions onto database-clock brackets | `internal/scheduler/*_test.go`, `internal/ingest/*_test.go` | 5 |
| 9 | The AC11 demonstration: revert one widened instrument over a `cp` backup, require `make test-contention` RED, scan both logs for the connection-exhaustion class and reject the run if it is there, restore, record the RED before any green | `internal/scheduler/*_test.go` (restored) | 8 |
| 10 | Re-centre the coverage ratchet: measure on the final tree in the shared regime, record with the reason in the commit message | `ai-docs/coverage-ratchet.txt` | 8 |
| 11 | `ai-docs/key-decisions.md`: KD-20 amendment — the shared configuration's connection arithmetic, and the corrected consequence clause about who may import `internal/testdb` | `ai-docs/key-decisions.md` | 10 |
| 12 | `ai-docs/go-test-conventions.md` § *Postgres is tested against Postgres*: the provisioning story, both paths | `ai-docs/go-test-conventions.md` | 10 |
| 13 | `AGENTS.md` § *Build & Test*: the new targets, and the coverage-ratchet table's container-runtime row | `AGENTS.md` | 10 |
| 14 | The propagation sweep of D10's class over the whole live tree, with the two stated exclusions | live `*.md` the sweep finds | 11, 12, 13 |

## Handoff plan

Grouping is required for **every M ≥ 1** — this section is mandatory in every design,
including a single-subtask one, whose one group is also terminal and runs in its own
`/context-reset` subagent. A group holds **up to 10** consecutive subtasks; ten is a
**maximum**, not an exact count, and a group ends at whichever comes first: the size cap,
a change-type switch, or a dependency-forced boundary. The terminal group's size is in
`1..=10`. Each group is homogeneous by change-type — **code** (`*.go`, migrations) or
**instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`, `ai-docs/**`) — never both.
The classes are the harness's and are not restated here. The files of this change that fall
in **neither** enumerated class — `Makefile`, `.githooks/**` and `.github/workflows/**` —
are grouped with the code, because they are executable build plumbing rather than
instructions an agent reads. Nothing that *is* enumerated is reclassified:
`ai-docs/coverage-ratchet.txt` is `ai-docs/**` and therefore instructions/harness, so
subtask 10 sits in Group B rather than beside the code that moves the number. That is safe
because the coverage shift holds inside the tolerance rather than blocking (D7), so Group
A's own commits pass and the same-commit rule — which binds only when the ratchet
**blocks** — does not reach subtask 10. Same-change-type subtasks are clustered into the
**fewest groups possible**, bounded by the size cap, by dependency order and by
homogeneity; naive interleaving is the least-desirable fallback and is not used here. The
default maximum is **4** groups per task, and more than 4 is surfaced to the user for
approval; this design defines **2**.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Every group is entered through it, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–9 (code change-type: `*.go`, plus the build plumbing
  `Makefile`, `.githooks/**` and `.github/workflows/**`). Nine subtasks, inside the size cap.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the
  orchestrator (typically xHigh) — NOT pinned — via the `general-purpose` subagent with no
  inline `model=` override, 1M-token window — subtasks 10–14 (instructions/harness
  change-type: `ai-docs/**`, `AGENTS.md`). Terminal group, within the `1..=10` range.
  Subtask 10 depends on subtask 8, which Group A completes first, so dependency order holds
  across the boundary.

Marker-to-implementor routing is applied at spawn: a **code** group routes to
`subagent_type="code-writer"`, whose model and effort are frontmatter-pinned, with no
inline override; an **instructions/harness** group routes to
`subagent_type="general-purpose"` with no inline `model=`. The `design-writer`,
`design-review`, `self-review` and `spec-writer` subagents run on the orchestrator's model
regardless of any group marker — only the code group's implementor is pinned below it.

## Risks

- **The coverage regime shifts and the ratchet's remaining band is narrower than the
  drift it was sized for**, so ordinary commits start blocking on an unlucky draw.
  Mitigation: D7 re-centres the recorded mark on a value measured on the final tree, with a
  stated reason; no figure is carried in this document to copy — `[measured eef4c4e · go test -count=1 -covermode=atomic -coverprofile ./... run in both regimes → the shared regime's total is lower and still holds inside the tolerance, so nothing blocks today and the margin is what shrinks]`.
- **The contention probe exhausts the server's connections and its red is read as
  contention**, certifying an instrument that failed for the wrong reason. Mitigation:
  D3's `clients` term, D12's pinned load parallelism passed to both the child and the
  wrapper, and the mandatory scan of both logs for the exhaustion class before any red or
  green is allowed to discharge an AC — `[derived → AC10, AC11]`.
- **A gate's exact exit code is lost through `go run`**, which reports a non-zero child as
  exit 1 with an `exit status N` line on stderr. Harmless for every gate here, which reads
  zero versus non-zero — but a recipe that branched on the value would be silently wrong.
  Mitigation: D1 states the property and D12's recipe is required to branch on zero versus
  non-zero — `[measured go1.26.5 · a program calling os.Exit(7) run under go run . and then built and executed → exit 1 with "exit status 7" on stderr, against exit 7 from the binary]`.
- **The contention probe comes back green because it never had the strength to go red**,
  and a clean race gate is then recorded as evidence about the tests when it is evidence
  about the load. Mitigation: D12's revert-first protocol, RED required before any green
  is recorded — `[derived → AC11]`.
- **The ceiling formula is under-sized because its binaries term drifted**, when a fifth
  package starts provisioning a database and nobody updates the constant. Mitigation:
  subtask 2's manifest test, which derives the term from the tree and fails by name —
  `[derived → AC5 and the manifest test in § Test Design]`.
- **The ceiling formula is over-sized on a many-core host** and the container's shared
  memory refuses it. Mitigation: the clamp at `ceilingMax`, and the stated rule that the
  answer past the clamp is a lower `-parallel`, never a higher ceiling — `[derived → AC5]`.
- **An interrupted run leaves a container behind.** Mitigation: D1's signal-aware teardown
  under a fresh context, with the reaper as the net for an outright kill, because the
  ad-hoc container carries this session's reap label — `[measured testcontainers-go@v0.44.0:internal/core/labels.go:36-49 · sed -n '36,49p' internal/core/labels.go → LabelReap is added when the reaper is enabled]` and `[derived → AC4]`.
- **A stale locator file points at a server that is gone**, and the run fails obscurely.
  Mitigation: D4's probe before trust, falling through to the ad-hoc path with a message —
  `[derived → AC4]`.
- **`--up` leaves a container running forever** when nobody runs `--down`. Accepted and
  documented: that is what a long-lived server is, it is named so `podman ps` shows it, and
  no reaper will remove it — `[measured testcontainers-go@v0.44.0:reaper.go:557-566 · sed -n '557,566p' reaper.go → the filter is DefaultLabels(sessionID)]`.
- **The wrapper's own tests start a container** and break AC1's "no test binary starts a
  container of its own during that run". Mitigation: the injectable provisioner seam of
  subtask 3, so subtask 4's tests exercise the decision order without a runtime —
  `[derived → AC1 and the wrapper tests in § Test Design]`.
- **`go mod tidy` moves a module from indirect to direct** because the wrapper reached for
  a moby type. Mitigation: the design uses no type from those modules — the fixed-port
  route that would have needed one is rejected in § Approach — and the tidy gate is run and
  its diff read before staging — `[measured eef4c4e:Makefile:60-63 · sed -n '60,63p' Makefile → the tidy-check target runs go mod tidy and refuses a dirty go.mod or go.sum]`.
- **CI's Test job grows by a fallback step and by one container lifecycle per suite-executing step.** Accepted: the
  per-step lifecycle is the spec's stated baseline, and D8 argues the fallback step is the
  only thing that keeps AC3 true after the day it is checked — `[derived → AC3, AC8]`.
- **A `//nolint` on the child `exec.Command` is rejected by the lint gate** for lacking a
  specific linter or an explanation. Mitigation: D1 writes it with both, which the config
  requires — `[measured eef4c4e:.golangci.yml:42-48 · sed -n '42,48p' .golangci.yml → nolintlint require-explanation: true, require-specific: true]`.
- **A comment in a new Go or shell artefact carries an outward reference** and the comment
  gate reds. Mitigation: the new artefacts are written to the ban from the start, and the
  gate runs over the whole tracked gated set — `[measured eef4c4e:Makefile:77-80 · sed -n '77,80p' Makefile → the comment-refs target runs go run ./cmd/commentrefs over the whole tracked gated set]` and `[derived → AC16]`.

## Test Design

Every claim below is about a test that does not exist yet, so every tag here is `[derived → …]`.

**Ceiling arithmetic — `internal/testdb`.**
Location: `internal/testdb/testdb_test.go`, beside the code. Entry point: the exported
ceiling function. Scenarios: a small parallelism and a large one produce the formula's
value; the clamp binds at the top and returns `ceilingMax`; the clamp binds at the bottom
and never returns less than the image default. No database, no container — the function is
arithmetic `[derived → AC5]`.

**Binaries-constant manifest — `internal/testdb`.**
Location: the same file. Entry point: the constant. Scenario: the set of packages whose
test files call `testdb.Main` is derived from the tree and compared with the constant,
failing **by name in both directions** — the shape the configuration manifest test in this
repository already uses, so a new database-backed package cannot silently under-size the
ceiling. Fixture: the module's own file tree; no database `[derived → AC5]`.

**Wrapper decision order — `cmd/testpg`.**
Location: `cmd/testpg/run_test.go`. Entry point: the package's `run` function, taking the
argument vector, an environment lookup and a provisioner seam. Scenarios, each asserting
both the child's environment and whether the seam was called: the DSN variable set in the
calling environment → the seam is never called and the child sees that same DSN
`[derived → AC2]`; no variable and no locator → the seam is called, its stop function runs
on a passing child, on a failing child, and after a delivered interrupt `[derived → AC4]`;
a locator naming an unreachable server → it is ignored, the seam is called, and the
message says so `[derived → AC4]`; a locator naming a reachable server → the seam is
**not** called and no stop runs, which is the "found, not started" clause `[derived → AC4]`;
a non-zero child yields a non-zero wrapper status `[derived → AC15]`; and the ceiling handed
to the seam is D3's formula evaluated over the `--clients` and `--parallel` the caller
passed, with the clamp binding at both ends `[derived → AC5]`. Fixtures:
a stub provisioner and a trivial child command; **no container is started by this package's
own tests**, which is what keeps AC1 true while the suite itself runs under the wrapper
`[derived → AC1]`.

**Contention tolerance — `internal/scheduler`, `internal/ingest`.**
No new test functions: the existing assertions are kept and their instruments retimed per
D9. The evidence is the probe, not a unit test. Location of the probe: the
`test-contention` target. Entry point: the whole-module race gate under induced
cross-package load against one shared server. Scenarios: the reverted-instrument run must
be **RED** `[derived → AC11]`, and only then the restored run **GREEN** `[derived → AC10]`.
Fixtures: the database-backed packages' own tests as the load generator, both runs logged
to files under the scratch directory `[derived → AC15]`.

**Fallback path — the whole module.**
Location: the `test-fallback` target, run by CI's Test job. Entry point: the bare
whole-module invocation with the DSN variable cleared. Scenario: it passes, and each
database-backed binary provisions its own container as before `[derived → AC3]`.

**What is verified by running rather than by a test.** AC1's "no test binary starts a
container of its own" is a property of a run, and the recipe is to watch the runtime's
container list across `make test` and see **one postgres container plus the Ryuk reaper**
— the reaper is there by D1's own design — and **no per-binary postgres container**
`[derived → AC1]`. AC13's landing place and AC14's agreement are read in the diff
`[derived → AC13, AC14]`.

## Open questions

- **Whether the harness instruction files that tell an agent to run a bare `go test ./...`
  should be swept onto `make test`.** They state nothing this diff falsifies, so D10 puts
  them outside the propagation class — but the fast path is unreachable from a bare
  invocation by AC3's own design, so the dominant workflow keeps paying for containers
  until they are swept. Recorded as a follow-up for `/task` Step 12's deferred inbox rather
  than absorbed here, because widening scope needs an ask, not a notification.
- **Whether `slack` and `ceilingMax` survive a many-core host.** Both are decisions made
  against this development machine and a GitHub-hosted runner; a host far outside that
  range should re-derive them, and the clamp's stated rule (lower `-parallel`, never
  raise the ceiling) is the answer that does not need re-deriving.
