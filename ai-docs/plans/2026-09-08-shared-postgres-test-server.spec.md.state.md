# Interview state — shared PostgreSQL test server

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md
issue_ref: "#67"
gh_issue:
  title: "Test suite: one shared PostgreSQL server instead of a container per package (13 s → 2.4 s)"
  state: open
  labels: ["area:platform"]
  body: |
    ## What

    Run the whole test suite against **one** PostgreSQL server instead of one container per package binary, and keep the per-package container only as the fallback for a bare `go test`.

    ## Why

    `internal/testdb` starts a container per test binary, and after the tmpfs/`--no-sync` change (PR for `perf/2026-09-07-testdb-container-speedup`) the container lifecycle is what is left of the suite's wall clock — the tests themselves are almost free.

    Measured on the development machine, `go test -count=1` over the four database-backed packages:

    | provisioning | store | scheduler | ingest | testdb | wall |
    |---|---|---|---|---|---|
    | container per package (today, after tmpfs) | ~11 s | ~11 s | ~11 s | ~9 s | **13–14 s** |
    | one shared server via `LAB_GAME_TEST_DSN` | 1.74 s | 1.95 s | 1.09 s | 0.11 s | **2.4 s** |

    Same tests, same count (`internal/scheduler` reports its 56 top-level tests green either way).

    ## Shape

    - `make test` / `make test-race` start one container (or reuse a running one), wait for readiness, export `LAB_GAME_TEST_DSN`, run the suite, stop the container afterwards. `testdb.Main` already prefers that variable, so no test changes are needed.
    - A bare `go test ./...` with no variable set keeps today's per-package container path.

    ## What has to be decided in the design, not in the patch

    - **Connection ceiling.** D11's arithmetic (`ai-docs/plans/done/2026-09-02-ledger-post-core.design.md`) sizes `schemaMaxConns = 4` against one server per package: 16 parallel subtests × 4 = 64 against `max_connections = 100`. Four packages sharing one server can reach 256, so the shared server needs `-c max_connections=300` (measured: the 2.4 s run above used exactly that) or a lower per-pool cap. D11 needs the amendment written down, not implied.
    - **Isolation.** Schemas already isolate tests (`testdb.Schema`), and no test touches server-global state — verified: no `pg_stat_activity`, `pg_terminate_backend`, `ALTER SYSTEM`, or `SHOW`-based assertion anywhere in `internal/`. What a shared server does change is timing: one package's load is now visible to another's deadline tests.
    - **CI.** The same question applies to the CI job: either the Makefile owns the container on both sides, or CI gains a `services:` block and the Makefile path diverges from it.
    - **Crash cleanup.** A container the Makefile starts is not Ryuk's to reap; the target has to remove it on failure paths too.

    ## Not in scope

    The per-test schema model, the image tag policy (KD-19), and the never-skip rule (KD-20) all stay as they are.

    https://claude.ai/code/session_01QCSjJTZKZXvNZZYpCCj9oc

  comments: []
  linked_issues: []
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: a17d60505b5d24c8d
prior_qa:
  - round: 1
    question: "The pre-commit hook execs .githooks/coverage-ratchet.sh directly, outside make, so every commit staging Go or SQL runs the whole suite under coverage on the per-package container path. Does the shared server cover that path too?"
    answer: "In scope — the coverage-ratchet / pre-commit path is provisioned the same way, so a commit staging Go pays the shared-server cost. The change widens into .githooks/, and the provisioning has to be callable from outside make."
  - round: 1
    question: "Who owns the shared server in CI? The Makefile header states that CI invokes the same sub-targets precisely so a local run and a CI run cannot disagree about what any gate's command is - and CI's Test job runs three suite-executing steps (test, race, cover-ratchet) with no services: block today."
    answer: "Makefile both sides — one target starts/reuses and removes the container locally and on the runner; ci.yml gains no services block. Keeps the stated invariant exactly."
  - round: 1
    question: "ai-docs/context-status.md records that pointing every package at one shared server made go test -race ./... fail roughly half the time in four internal/scheduler tests. Nine consecutive -count=1 race runs on today's tree against a max_connections=300 shared server were all green, but that entry predates the tmpfs change and the backoff refactor of those tests. What should the spec require?"
    answer: "Fix the tests — shared server is the default for both gates; any test that proves contention-sensitive is made contention-tolerant in this task, including up to the four timing-sensitive scheduler tests."
```
