# Progress: Shared PostgreSQL test server — ACTIVE
_Updated: 2026-09-08 21:00_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** perf/2026-09-08-shared-postgres-test-server
**base_commit:** 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**Last build:** PASS

**Issue:** #67
**Spec:** ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md

**current_step:** Step 8 — Group A subtask 7 of 9 complete
**last_passed_gate:** `actionlint .github/workflows/ci.yml`, `make comment-refs` | pending commit
**entry_args:** 67

## Next action

**Do this immediately:** Group A, subtask 8 — contention tolerance per D9: classify EVERY wall-clock constant in `internal/scheduler/*_test.go` and `internal/ingest/*_test.go` as an instrument (widen per test, or replace with polling on the condition — never edit the shared `testConfig()`/`shortDeadlineConfig()`) or the subject (keep exact, move to database-clock brackets read via `clock_timestamp()` around the operation, matching `TestDeadline_successiveBreaches_growingDelay`'s existing shape). `cfg.TaskTimeout` is BOTH depending on the suite: an instrument for worker/observation tests (must not fire — it is also server-side `statement_timeout`/`idle_in_transaction_session_timeout`), the subject for the deadline suite (left exactly as is). Start from the named four scheduler tests in `ai-docs/context-status.md`'s recorded trap, but membership is every test whose result changes under cross-package load, not only those four — use `make test-contention` (subtask 9's territory) to find any others, or reason from each test's own wait/timeout constants.

## Subtasks

Design: `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md`. Group A = 1–9 (code, `code-writer`, sonnet/medium pinned). Group B = 10–13 (instructions/harness, `general-purpose`, model inherited).

- [x] 1. `internal/testdb`: provisioning API in a new file; `Main`'s container branch re-expressed over it — commit e09e79f. Coverage ratchet lowered 91.66% → 90.30% in the same commit (D7: regime shift + new uncovered StartServer/Probe/Ceiling code). NOTE: this subtask's commit did not include the `.progress.md` update (missed the "stage with the subtask commit" step) — caught up in subtask 2's commit instead.
- [x] 2. Tests for the ceiling formula, its refusal path, the capacity probe, the binaries manifest — `internal/testdb/server_test.go`, external-package tests reusing the existing `TestMain`. All green against a session-local shared server. Coverage ratchet rose 90.30% → 91.10% (recorded automatically by the hook).
- [x] 3. `cmd/testpg`: the wrapper — `cmd/testpg/main.go`, `cmd/testpg/run.go`, `cmd/testpg/locator.go`. Decision order (D1/D3a), `--up`/`--down` sized by the same `testdb.Ceiling` terms with the reaper disabled via `TESTCONTAINERS_RYUK_DISABLED` for `--up`, signal-aware teardown (child under the signal context, teardown under a fresh `context.Background()` — `//nolint:contextcheck` explained at both sites), real child exit code propagated via `exec.ExitError.ExitCode()` (go run's own flattening is the caller's concern, not this wrapper's). Manually smoke-tested against the real podman runtime: caller-DSN path with reported shortfall, non-zero child propagation (both `go run` and the built binary), `--up` (created `lab-game-test-postgres`, wrote the locator, printed the DSN and ceiling), the default path finding and reusing it, `--down` (removed the container and the locator file), and `--down` again with no locator (no-op). Coverage ratchet lowered 91.10% → 86.02% in the same commit.
- [x] 4. `cmd/testpg/run_test.go`: table tests over a `stubSeam` covering every decision-order scenario from the design's Test Design section (caller DSN untouched/undersized, no-locator fallback with stop on pass/fail/cancelled-context, locator unreachable/undersized/admits, ceiling arithmetic matches `testdb.Ceiling`, `--up`/`--down` over the seam, `--up`+`--down` together and no-child-args are usage errors). Verified via `podman ps -a` that no container exists beyond the pre-existing session-local one — this package's own tests start none. Coverage ratchet rose 86.02% → 89.44%.
- [x] 5. `Makefile`: `test`/`test-race` route through `go run ./cmd/testpg -- go test [...] ./...`; added `test-db-up` (`CLIENTS ?= 1`, overridable), `test-db-down`, `test-fallback` (bare `go test ./...` with `LAB_GAME_TEST_DSN=` cleared — not part of `verify`, matching `cover-ratchet`'s own precedent), `test-contention` (`CONTENTION_PARALLEL ?= nproc`, `--clients 2`, single wrapper invocation whose `bash -c` child backgrounds a `-count=1` load loop of the database-backed packages and foregrounds the whole-module race gate, both logging to `tmp/`, capturing `fg_status` explicitly under the nested script's own `set -eu -o pipefail` rather than letting it abort before the load loop is killed). Manually ran both `make test` and `make test-contention CONTENTION_PARALLEL=2` to green against the real podman runtime.
- [x] 6. `.githooks/coverage-ratchet.sh`: the measurement command routed through `go run ./cmd/testpg --`, placed after the existing raise-mode skip decision (unchanged — the wrap only touches the command already past that check, so AC7 needs no new code) — the runtime advice now names `make test-db-up` first alongside the existing `export LAB_GAME_TEST_DSN=...` line. `shellcheck` and the script-shape gate green; `.githooks/coverage-ratchet.sh --check` ran green against a session-local shared server, confirming the wrap didn't change the measured value.
- [x] 7. `.github/workflows/ci.yml`: added a `make test-fallback` step to the Test job, after `cover-ratchet` (D8/AC3 — no other Test-job step still reaches the per-package container path). Verified AC9/D11 by diffing every file this branch has touched (`git diff --name-only <base>..HEAD`) against the change filter's globs: every one (`*.go`, `Makefile`, `.githooks/**`, `ai-docs/**`, `.github/workflows/**`) is already named — no filter edit needed, confirming the design's own claim rather than trusting it. `actionlint` and `make comment-refs` green.  ← CURRENT (subtask 8 next)
- [ ] 2. Tests for the ceiling formula, its refusal path, the capacity probe, the binaries manifest
- [ ] 3. `cmd/testpg`: the wrapper — decision order, D3a per-path shortfall answers, locator, `--up`/`--down`, `--clients`/`--parallel`, signal-aware teardown, status passthrough, injectable seam
- [ ] 4. Wrapper tests over the injected seam
- [ ] 5. `Makefile`: route `test`/`test-race` through the wrapper; add `test-db-up`, `test-db-down`, `test-fallback`, `test-contention`
- [ ] 6. Coverage ratchet: wrap the measurement after the skip decision; update the runtime advice
- [ ] 7. CI: fallback step in the Test job; verify the change filter already names every added artefact
- [ ] 8. Contention tolerance per D9: classify every wall-clock constant, widen instruments per test, move timing assertions onto database-clock brackets
- [ ] 9. The AC11 demonstration: revert one instrument over a `cp` backup, require `make test-contention` RED, scan for the exhaustion class, restore, record the RED
- [ ] 10. `ai-docs/key-decisions.md`: KD-20 amendment — connection arithmetic + corrected consequence clause
- [ ] 11. `ai-docs/go-test-conventions.md`: the provisioning story, both paths
- [ ] 12. `AGENTS.md` § Build & Test: new targets, container-runtime row, the cache-replay sentence
- [ ] 13. The propagation sweep of D10's class over the live tree, with the two stated exclusions

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review reached GO at round 4 after the owner raised the cap to 4 (was 3); the GO's five notes and four recommendations were written back into the design before Step 8 and verified against the tree.
- **Step 8**: the coverage ratchet is deliberately not a subtask — D7 makes its movement an obligation inside whichever code commit moves coverage, which is where the workspace same-commit rule already puts it.

## Key discoveries (don't re-investigate)

- `testdb.Main` returns `m.Run()` before touching testcontainers when `LAB_GAME_TEST_DSN` is set, so no test needs to change to reach a shared server.
- Four packages call `testdb.Main`: `internal/ingest`, `internal/scheduler`, `internal/store`, `internal/testdb`. That is D3's `binaries` term.
- `go run` does not propagate the child's exact exit code: `os.Exit(7)` under `go run` yields exit 1 with `exit status 7` on stderr; the built binary yields 7. Branch on zero-vs-non-zero, never on the value.
- testcontainers v0.44.0 parses exposed ports with `network.ParsePortRange`, which has no `host:container` form — a fixed host port would need `HostConfigModifier` and promote `moby/moby/api` to a direct requirement. Rejected.
- The ratchet's headroom is thin: the shared regime measures below the container regime, and the recorded mark sits close to the floor. Assume the ratchet blocks on the first commit landing this code and lower it in that same commit with the reason.
- Connection exhaustion is `FATAL: sorry, too many clients already (SQLSTATE 53300)` — the project's own client renders both spellings on one line.
- No gate enforces KD-20's "imported only from `_test.go` files" clause; `cmd/testpg` importing `internal/testdb` breaks no check, only the prose that subtask 10 owns.
- Ephemeral host ports mean the shared DSN differs per wrapper invocation, so the database-backed packages are a test-cache miss on every ad-hoc run — including the ratchet's own measurement. Stable again under `--up`'s named container.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |
| AC12 | NOT_TESTED |
| AC13 | NOT_TESTED |
| AC14 | NOT_TESTED |
| AC15 | NOT_TESTED |
| AC16 | NOT_TESTED |
| AC17 | NOT_TESTED |
| AC18 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

