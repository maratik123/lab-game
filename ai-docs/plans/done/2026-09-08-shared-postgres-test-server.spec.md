# Shared PostgreSQL test server: one server for the whole suite, containers as the fallback

**Source:** issue #67
**Date:** 2026-09-08
**Tracked in:** #67

The suite's database-backed packages — `internal/store`, `internal/scheduler`,
`internal/ingest` and `internal/testdb` itself — each start their own PostgreSQL
container today, one per test binary. After the tmpfs / `--no-sync` change the
container lifecycle is what is left of the suite's wall clock: the tests
themselves are nearly free. `testdb.Main` already prefers an existing server
whenever `LAB_GAME_TEST_DSN` is set, returning `m.Run()` before it ever reaches
the container path, so pointing every package at one server needs no change to
the way any test *reaches* the database
[source: 582ad6a:internal/testdb/testdb.go § Main · `sed -n '/^func Main/,/^}/p' internal/testdb/testdb.go`].

This task moves the project's own test entry points onto that path, keeps the
per-package container as the fallback for a bare `go test`, and pays off the one
debt the shared path exposes: a test that passes only because it had the server
to itself.

**How the win is measured** (the instruction, not a stored figure): time an
uncached whole-module test run with `LAB_GAME_TEST_DSN` unset, and again with it
naming one server sized for every database-backed package at once. Both runs
disable the Go test cache; the delta is container lifecycle, and it is paid once
per test binary today.

## Scope

1. The project's test entry points provision **one** PostgreSQL server for the
   whole run and point every database-backed package at it through
   `LAB_GAME_TEST_DSN`, instead of letting each test binary start its own
   container.
2. Provisioning is idempotent about an already-named server: when
   `LAB_GAME_TEST_DSN` is set in the calling environment, the entry point starts
   nothing and runs the gates against the server it names.
3. The provisioned server's connection ceiling is sized for every
   database-backed package running concurrently against it at the per-pool cap
   `internal/testdb` sets. The arithmetic that sized that cap against
   *one server per package* is amended in the live decision record.
4. Teardown covers every exit path of the entry point — a passing run, a failing
   gate, and an interrupted run. A container this project's own target started is
   not the testcontainers reaper's to remove.
5. A bare whole-module `go test` with `LAB_GAME_TEST_DSN` unset keeps today's
   per-package container path, unchanged and passing.
6. **The coverage-ratchet path is inside this scope** (owner, round 1). The
   pre-commit hook reaches the ratchet script directly rather than through
   `make`, so the provisioning is callable from outside `make` and a commit that
   stages a coverage-moving file measures against the shared server. The
   provisioning must sit *after* the ratchet's own skip decision, so a commit
   that stages nothing coverage-moving still starts nothing.
7. **CI reaches the shared server through the same targets** (owner, round 1).
   `.github/workflows/ci.yml` gains no `services:` block; the Test job keeps
   invoking the same sub-targets a local run invokes, which is the invariant the
   Makefile header already states.
8. **Contention debt is paid here, not deferred** (owner, round 1). Any test the
   shared server exposes as dependent on exclusive access to that server is made
   contention-tolerant in this task. The four `internal/scheduler` tests the
   status log names are the expected members and the owner has accepted them
   entering scope; they are candidates, not the boundary. Membership is "every
   test whose result depends on being the sole client of the server".
9. Every artefact this change introduces is registered, in this same change, in
   the CI change filter of each job that runs a gate over it — the workflow's own
   filter block states that obligation, and an unregistered artefact means the
   gate silently stops running.
10. **Propagation class:** every site whose claim this diff falsifies, per
    `AGENTS.md` § *Propagation Rule* step 4 — the documented provisioning story,
    the decision record's connection arithmetic, the description of what the
    pre-commit hook and CI run, and any instruction text that states how the
    suite reaches a database. Known members illustrate the class and do not bound
    it: `ai-docs/go-test-conventions.md` § *Postgres is tested against Postgres*,
    `ai-docs/key-decisions.md` KD-20, `AGENTS.md` § *Build & Test* (including the
    coverage-ratchet table's "container runtime missing?" row), the ratchet
    script's own container-runtime advice, and `ai-docs/context-status.md`'s
    recorded shared-server trap.

## Out of scope

- The per-test schema model. `testdb.Schema` keeps its shape: a uniquely named
  schema per call, `search_path`-scoped, dropped in `tb.Cleanup`.
- KD-19's image-tag policy. The shared server runs the same major-tag image.
- KD-20's never-skip rule. With neither a DSN nor a reachable container runtime,
  `testdb.Main` still exits non-zero rather than skipping.
- Pointing any committed file at the developer's own local PostgreSQL instance —
  KD-20 retired it for this purpose and that stands.
- A `services:` block in CI, and any arrangement that lets a CI job reach a
  database by a route a local run does not use.
- Redesigning the scheduler's timing model. A contention-sensitive test is made
  tolerant of cross-package load; the behaviour it asserts does not change, and a
  test is not made tolerant by widening it until it asserts nothing.

## Deferred

- what | why | separate issue needed?
- Reuse of a *long-lived* developer-owned server across runs by container-name
  convention rather than by `LAB_GAME_TEST_DSN` | the environment variable is
  already the documented contract and covers the case; a name convention is an
  ergonomics addition, not a requirement | no — record only if the design finds
  the variable insufficient.
- Holding one server across CI's three suite-executing steps instead of one
  lifecycle per step | the owner accepted the per-step cost when choosing
  Makefile ownership on both sides; holding is an optimisation with its own
  cleanup surface | no — the design may take it if it is free, otherwise it is a
  follow-up.

## Key decisions

| Question | Decision |
|---|---|
| Does the shared path become the default for a bare `go test`? | No. With `LAB_GAME_TEST_DSN` unset, the per-package container path is unchanged — that is the fallback the issue preserves deliberately. |
| Does the coverage-ratchet / pre-commit path share the server? | **Yes** (owner, round 1). The change widens into the hook's side of the tree and the provisioning becomes callable without `make`. |
| Who owns the container in CI? | **The same targets a local run uses** (owner, round 1). No `services:` block; CI pays a container lifecycle per suite-executing step unless the design can hold one across them for free. |
| What if the race gate proves contention-sensitive? | **Fix the tests, in this task** (owner, round 1). The shared server is the default for both gates; the four timing-sensitive `internal/scheduler` tests entering scope is accepted. |
| Server-side ceiling, or a smaller per-pool cap? | Raise the ceiling on the shared server; leave `internal/testdb`'s per-pool cap alone. The cap is still correct for the fallback path, and lowering it would constrain every pool for a configuration only the shared path reaches. Reviewable by the design if the arithmetic disagrees. |
| Where does the connection-arithmetic amendment land? | In the **live** decision record (`ai-docs/key-decisions.md`), as an amendment clause on KD-20 or a new decision. Not by editing the merged design under `ai-docs/plans/done/` — that tree is a history surface under `AGENTS.md` § *Propagation Rule* step 4, and the project already has precedent for carrying a design amendment into the decision record instead. |
| What happens when the caller already set `LAB_GAME_TEST_DSN`? | The entry point provisions nothing and runs against the named server, so a developer with a server running keeps it and an outside supplier can hand one in. |
| What is the provisioned server's lifetime? | Reuse a server the project's own locator already names; remove only a server this invocation started. Never remove one it found. Whether an explicit bring-up / take-down pair exists so a developer can keep one across commits is the design's call — the reuse rule above is what makes either shape safe. |
| How is "contention-tolerant" demonstrated? | By a probe that has been shown able to fail. A repeated clean run against a shared server is evidence about the run's conditions, not about the tests: `AGENTS.md` § *Patterns* 2. The design induces cross-package load, shows the probe turning a known exclusive-access-dependent assertion red, and only then records a green gate against it. |

## Technical constraints

- **`testdb.Main` needs no change to reach a shared server.** It reads
  `LAB_GAME_TEST_DSN` first and returns `m.Run()` before touching testcontainers
  [source: 582ad6a:internal/testdb/testdb.go § Main · `sed -n '/^func Main/,/^}/p' internal/testdb/testdb.go`].
  What may change is the tests themselves, under Scope item 8.
- **The Makefile's stated invariant, now load-bearing.** Its header records that
  CI never runs `verify` but invokes the same sub-targets, "so a local run and a
  CI run cannot disagree about what any gate's command is"
  [source: 582ad6a:Makefile § header comment · `grep -n 'cannot disagree' Makefile`].
  The round-1 answer chose that invariant over a CI-side service container, so the
  design does not get to narrow it.
- **CI runs three suite-executing steps in one job.** The Test job invokes the
  test target, the race target and the coverage-ratchet target as separate steps,
  and the workflow carries no `services:` block
  [source: 582ad6a:.github/workflows/ci.yml § jobs.test · `grep -n 'name: Test' .github/workflows/ci.yml`].
  A server whose lifetime is one target is therefore provisioned once per step.
- **The change filter is the thing that decides whether a gate runs at all.** The
  workflow's filter block states that any future artefact must be added there in
  the same pull request that introduces it, or its gate silently stops running;
  the job that runs the Go gates is filtered on a path list that names no script
  directory
  [source: 582ad6a:.github/workflows/ci.yml § jobs.changes · `grep -n 'silently stops running' .github/workflows/ci.yml`].
  A provisioning helper added outside those paths would leave the suite ungated
  against its own changes.
- **The pre-commit hook bypasses `make`, and it skips more often than it runs.**
  It execs the coverage-ratchet script directly
  [source: 582ad6a:.githooks/pre-commit.sh § final exec · `grep -n 'coverage-ratchet.sh' .githooks/pre-commit.sh`],
  and that script exits early and silently when nothing coverage-moving is staged
  — which is most commits of a task run
  [source: 582ad6a:.githooks/coverage-ratchet.sh § raise-mode staged check · `grep -n 'Nothing that can move coverage is staged' .githooks/coverage-ratchet.sh`].
  Provisioning placed before that decision would start a container on every
  documentation commit.
- **Runtime reachability and the permission list.** `permissions.allow` in
  `.claude/settings.json` grants `Bash(podman *)`, `Bash(make *)`, `Bash(go *)`
  and `Bash(psql *)`; it names no `docker` entry
  [source: 582ad6a:.claude/settings.json § permissions.allow · `jq -r '.permissions.allow[]' .claude/settings.json`].
  A target that shells out to a runtime CLI by the name `docker` therefore cannot
  be run unattended by an agent unless the grant is added in the same change —
  and the local machine runs rootless podman while the CI runner does not. Whether
  the target drives a CLI at all, or reaches the runtime through the abstraction
  `internal/testdb` already depends on, is the design's call; the permission
  consequence is not.
- **A recorded contention trap sits directly on this task's path, and it is not
  retracted here.** `ai-docs/context-status.md` records that pointing every
  package at one shared server made the race gate fail in four named
  `internal/scheduler` tests about half the time, while per-package containers
  were green run after run, and that CI never sees it because there is no
  `services:` block
  [source: 582ad6a:ai-docs/context-status.md § "The scheduler suite assumes exclusive database access" · `grep -n 'assumes exclusive database access' ai-docs/context-status.md`].
  That entry predates both the tmpfs change and the backoff refactor of those same
  tests, and it names no connection ceiling for the server it used — so it is
  neither confirmed nor withdrawn by this spec. The round-1 answer makes closing
  it this task's work, by measurement on the tree that exists then.
- **A green run is a claim about the run's conditions until the probe has gone
  red.** `AGENTS.md` § *Patterns* 2. Repeating a clean suite against a shared
  server distinguishes "no contention dependency" from "not enough contention to
  expose one" not at all; only an instrument demonstrated able to fail does.
- **Gate-log discipline applies to whatever wraps the suite.** A gate whose exit
  status is load-bearing is never piped; output is captured to a file under the
  ignored scratch directory and read from there (`AGENTS.md` § *Build & Test*).
- **New shell and workflow surface is gated three ways.** Any script the change
  adds or edits passes `shellcheck` and the project's help-flag shape gate, and
  carries no outward reference in any comment; any workflow file it touches passes
  `actionlint`.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | With `LAB_GAME_TEST_DSN` unset in the calling environment, the project's test entry point runs every database-backed package against one PostgreSQL server it provisioned, and no test binary starts a container of its own during that run. |
| AC2 | With `LAB_GAME_TEST_DSN` set in the calling environment, the entry point provisions no server and the gates execute against the server it names. |
| AC3 | A bare whole-module Go test invocation with `LAB_GAME_TEST_DSN` unset still provisions one container per database-backed test binary, and passes. |
| AC4 | The entry point removes the server it provisioned on a passing run, on a failing gate, and on an interrupted run; after any of the three, no container the entry point created remains. A server the entry point found rather than started is still running afterwards. |
| AC5 | The provisioned server's connection ceiling admits every database-backed package running concurrently at the per-pool cap `internal/testdb` sets, with headroom; the design states the arithmetic term by term and the source of each term. |
| AC6 | A commit that stages a coverage-moving file measures its whole-module coverage against one provisioned shared server, reached without going through `make`. |
| AC7 | A commit that stages nothing coverage-moving provisions no server: provisioning happens after the ratchet's skip decision, never before it. |
| AC8 | `.github/workflows/ci.yml` declares no service container, and its Test job reaches the shared server through the same sub-targets a local run invokes. |
| AC9 | Every artefact this change adds is named in the CI change filter of each job that runs a gate over it, so a commit touching only that artefact still reaches those jobs. |
| AC10 | Under induced cross-package contention against the shared server, the race gate over the whole module reports no failure. |
| AC11 | The contention probe AC10 is measured with is demonstrated able to fail — applied to an assertion that depends on exclusive access to the server, it goes red — and that demonstration is recorded before any green result from it is. |
| AC12 | No test in the module depends on being the sole client of the PostgreSQL server it runs against; membership of the class is "every test whose result changes when another package's tests run concurrently against the same server". |
| AC13 | `ai-docs/key-decisions.md` carries the connection arithmetic for the shared-server configuration, and no file under `ai-docs/plans/done/` is edited to carry it. |
| AC14 | Every live document whose statement about how the test suite reaches a database this change falsifies agrees with the shipped behaviour. Membership is "all sites whose claim this diff falsifies", per `AGENTS.md` § *Propagation Rule* step 4; the Scope item 10 list illustrates the class without bounding it. |
| AC15 | No gate this change introduces routes a load-bearing exit status through a pipe; each captures output to a file under the ignored scratch directory. |
| AC16 | Every script this change adds or edits satisfies the project's shell gate and its help-flag shape gate, and no comment in it carries an outward reference. |
| AC17 | `make verify` is green on the branch, and every CI job the change's paths reach is green on the pull request. |
| AC18 | No committed file names the developer's own local PostgreSQL instance; the shared server is addressed by a locator this change itself defines. |

## Open questions

- **Fixed host port or an ephemeral one.** A fixed port collides with a developer's
  other PostgreSQL; an ephemeral port has to be discovered after start and threaded
  into the DSN. Default the design may take: ephemeral, discovered — it is the shape
  `internal/testdb` already uses for the container path.
- **What form the provisioning helper takes.** A shell script keeps the hook's
  dependency surface at zero; a small Go command reuses `internal/testdb`'s image
  constant and container settings, so KD-19's tag and the tmpfs arguments stay in
  one place, at the cost of a compile before the first gate. The project has
  precedent for the Go-command shape in its comment-reference gate. Design's call —
  whichever it is, the artefact still owes Scope item 9.
- **Whether contention tolerance is bought by widening the assertions or by pinning
  package parallelism.** Serialising test binaries removes cross-package load
  entirely, at some of the speedup and without making any test more honest; widening
  a timing bound risks the test asserting nothing. A third route — computing the
  expected value outside the code under test, the shape the scheduler suite already
  had to adopt once — is likely the one that survives review. The design weighs
  these; the spec requires only that the result satisfies AC10 through AC12.
- **Whether the shared server is held across CI's three suite-executing steps.**
  Recorded under Deferred; the design may take it if it costs nothing, and the
  per-step lifecycle is the accepted baseline.
