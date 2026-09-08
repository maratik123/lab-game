# Progress: Shared PostgreSQL test server — ACTIVE
_Updated: 2026-09-08 21:00_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** perf/2026-09-08-shared-postgres-test-server
**base_commit:** 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**Last build:** PASS

**Issue:** #67
**Spec:** ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md

**current_step:** Step 8 — Group A not yet started (subtask 0 of 13)
**last_passed_gate:** `go build ./...` | 2026-09-08T20:59:14Z | 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**entry_args:** 67

## Next action

**Do this immediately:** start Group A, subtask 1 — create `internal/testdb/server.go` with the exported provisioning API (`ServerOptions`, `StartServer`, `Server.DSN`, `Server.Stop`, the capacity `Probe`, the DSN env-var name, the shared container name), the ceiling formula with its `clients` term, its constants and its refusal above `ceilingMax`; then re-express `Main`'s container branch over it in `internal/testdb/testdb.go` with behaviour preserved.

## Subtasks

Design: `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md`. Group A = 1–9 (code, `code-writer`, sonnet/medium pinned). Group B = 10–13 (instructions/harness, `general-purpose`, model inherited).

- [ ] 1. `internal/testdb`: provisioning API in a new file; `Main`'s container branch re-expressed over it  ← CURRENT
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

