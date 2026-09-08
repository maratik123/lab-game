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
any test
[source: 582ad6a:internal/testdb/testdb.go § Main · `sed -n '/^func Main/,/^}/p' internal/testdb/testdb.go`].

This task moves the project's own test entry points onto that path and keeps the
per-package container as the fallback for a bare `go test`.

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
6. **Propagation class:** every site whose claim this diff falsifies, per
   `AGENTS.md` § *Propagation Rule* step 4 — the documented provisioning story,
   the decision record's connection arithmetic, the description of what CI runs,
   and any instruction text that states how the suite reaches a database. Known
   members illustrate the class and do not bound it: `ai-docs/go-test-conventions.md`
   § *Postgres is tested against Postgres*, `ai-docs/key-decisions.md` KD-20,
   `AGENTS.md` § *Build & Test* (including the coverage-ratchet table's
   "container runtime missing?" row), and `ai-docs/context-status.md`'s recorded
   shared-server trap.
7. Whether the pre-commit / coverage-ratchet path is inside this scope is
   **pending** — question 1 below.
8. Whether CI reaches the shared server through the same target or through a
   `services:` block is **pending** — question 2 below.

## Out of scope

- The per-test schema model. `testdb.Schema` keeps its shape: a uniquely named
  schema per call, `search_path`-scoped, dropped in `tb.Cleanup`.
- KD-19's image-tag policy. The shared server runs the same major-tag image.
- KD-20's never-skip rule. With neither a DSN nor a reachable container runtime,
  `testdb.Main` still exits non-zero rather than skipping.
- Pointing any committed file at the developer's own local PostgreSQL instance —
  KD-20 retired it for this purpose and that stands.
- Rewriting test assertions for any reason other than a contention dependency the
  shared server exposes (see Key decisions).

## Deferred

- what | why | separate issue needed?
- Reuse of a *long-lived* developer-owned server across runs by container-name
  convention rather than by `LAB_GAME_TEST_DSN` | the environment variable is
  already the documented contract and covers the case; a name convention is an
  ergonomics addition, not a requirement | no — record only if the design finds
  the variable insufficient.

## Key decisions

| Question | Decision |
|---|---|
| Does the shared path become the default for a bare `go test`? | No. With `LAB_GAME_TEST_DSN` unset, the per-package container path is unchanged — that is the fallback the issue preserves deliberately. |
| Server-side ceiling, or a smaller per-pool cap? | Raise the ceiling on the shared server; leave `internal/testdb`'s per-pool cap alone. The cap is still correct for the fallback path, and lowering it would constrain every pool for a configuration only the shared path reaches. Reviewable by the design if the arithmetic disagrees. |
| Where does the connection-arithmetic amendment land? | In the **live** decision record (`ai-docs/key-decisions.md`), as an amendment clause on KD-20 or a new decision. Not by editing the merged design under `ai-docs/plans/done/` — that tree is a history surface under `AGENTS.md` § *Propagation Rule* step 4, and the project already has precedent for carrying a design amendment into the decision record instead. |
| What happens when the caller already set `LAB_GAME_TEST_DSN`? | The entry point provisions nothing and runs against the named server, so a developer with a server running keeps it and CI can supply one from outside. |
| Does the coverage-ratchet / pre-commit path share the server? | **TBD — question 1.** |
| Who owns the container in CI? | **TBD — question 2.** |
| What does the spec require if the race gate proves contention-sensitive? | **TBD — question 3.** |

## Technical constraints

- **`testdb.Main` needs no test-side change.** It reads `LAB_GAME_TEST_DSN`
  first and returns `m.Run()` before touching testcontainers
  [source: 582ad6a:internal/testdb/testdb.go § Main · `sed -n '/^func Main/,/^}/p' internal/testdb/testdb.go`].
- **The Makefile's stated invariant.** Its header records that CI never runs
  `verify` but invokes the same sub-targets, "so a local run and a CI run cannot
  disagree about what any gate's command is"
  [source: 582ad6a:Makefile § header comment · `grep -n 'cannot disagree' Makefile`].
  Any answer to question 2 that lets CI provision differently is a deliberate
  narrowing of that invariant and says so in the design.
- **CI runs three suite-executing steps in one job.** The Test job invokes the
  test target, the race target and the coverage-ratchet target as separate
  steps, and the workflow carries no `services:` block
  [source: 582ad6a:.github/workflows/ci.yml § jobs.test · `grep -n 'name: Test' .github/workflows/ci.yml`].
  A server whose lifetime is one target is therefore provisioned once per step.
- **The pre-commit hook bypasses `make`.** It execs the coverage-ratchet script
  directly, and that script runs the whole module under coverage instrumentation
  [source: 582ad6a:.githooks/pre-commit.sh § final exec · `grep -n 'coverage-ratchet.sh' .githooks/pre-commit.sh`].
  A Makefile-only change leaves that path on the per-package container.
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
  That entry predates both the tmpfs change and the backoff refactor of those
  same tests, and it names no connection ceiling for the server it used — so it
  is neither confirmed nor withdrawn by this spec. It is a live claim about the
  path this task makes the default, and the design closes it **by measurement on
  the tree that exists then**, stating the repetition its evidence covers.
- **Gate-log discipline applies to whatever wraps the suite.** A gate whose exit
  status is load-bearing is never piped; output is captured to a file under the
  ignored scratch directory and read from there (`AGENTS.md` § *Build & Test*).
- **New shell and workflow surface is gated.** Any script the change adds or
  edits passes `shellcheck`; any workflow file it touches passes `actionlint`.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | With `LAB_GAME_TEST_DSN` unset in the calling environment, the project's test entry point runs every database-backed package against one PostgreSQL server it provisioned, and no test binary starts a container of its own during that run. |
| AC2 | With `LAB_GAME_TEST_DSN` set in the calling environment, the entry point provisions no server and the gates execute against the server it names. |
| AC3 | A bare whole-module Go test invocation with `LAB_GAME_TEST_DSN` unset still provisions one container per database-backed test binary, and passes. |
| AC4 | The entry point removes the server it provisioned on a passing run, on a failing gate, and on an interrupted run; after any of the three, no container the entry point created remains. |
| AC5 | The provisioned server's connection ceiling admits every database-backed package running concurrently at the per-pool cap `internal/testdb` sets, with headroom; the design states the arithmetic term by term and the source of each term. |
| AC6 | The race gate reports no failure against the shared-server path across repeated consecutive uncached runs; the design fixes what repetition its evidence must cover and argues why that is enough. |
| AC7 | `ai-docs/key-decisions.md` carries the connection arithmetic for the shared-server configuration, and no file under `ai-docs/plans/done/` is edited to carry it. |
| AC8 | Every live document whose statement about how the test suite reaches a database this change falsifies agrees with the shipped behaviour. Membership is "all sites whose claim this diff falsifies", per `AGENTS.md` § *Propagation Rule* step 4; the Scope item 6 list illustrates the class without bounding it. |
| AC9 | No gate this change introduces routes a load-bearing exit status through a pipe; each captures output to a file under the ignored scratch directory. |
| AC10 | `make verify` is green on the branch, and every CI job the change's paths reach is green on the pull request. |
| AC11 | No committed file names the developer's own local PostgreSQL instance; the shared server is addressed by a locator this change itself defines. |

## Open questions

- **Fixed host port or an ephemeral one.** A fixed port collides with a developer's
  other PostgreSQL; an ephemeral port has to be discovered after start and threaded
  into the DSN. Default the design may take: ephemeral, discovered — it is the shape
  `internal/testdb` already uses for the container path.
- **Whether the shared server is torn down between the test and race gates or held
  across them.** Holding is faster and widens the crash-cleanup window; tearing down
  per gate is simpler and pays one lifecycle per step. Design's call once question 2
  is answered.
- **Whether the race gate's contention exposure warrants pinning `-p` (package
  parallelism) rather than widening the connection ceiling.** Serialising package
  binaries removes cross-package load entirely at some of the speedup; it is a lever
  the design should weigh against the ceiling, not a decision this spec makes.
