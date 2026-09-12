# Progress: Test database container named per checkout — ACTIVE
_Updated: 2026-09-12 00:00_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-test-container-name-per-checkout
**base_commit:** 8e18df9f46e5b1ccec36e9c825a43dbb9b2e8f6f
**Last build:** not run
**Issue:** #91
**Spec:** ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md
**current_step:** Step 8 — Group A, subtask 1 of 2 complete
**last_passed_gate:** golangci-lint run | 2026-09-12T00:00:09Z | 8e18df9f46e5b1ccec36e9c825a43dbb9b2e8f6f
**entry_args:** сделать так, чтобы имя контейнера бд выводилось из имени каталога проекта, например lab-game-test-postgres для ~/lab-game и lab-game2-test-postgres для ~/lab-game2 (для параллелизации разработки)

## Next action

**Do this immediately:** hand off Group A (subtasks 1–2, `cmd/testpg/run.go` + `cmd/testpg/run_test.go` + `internal/testdb/server.go`) to `code-writer` per the design's `## Handoff plan`.

## Subtasks

- [x] 1. The pure derivation: suffix constant, compiled validity pattern, directory → container name or error. Table test first.
- [ ] 2. Wire it: the working-directory seam member, `runUp` / `runDown` derivation with their ordering pins, delete `testdb.SharedContainerName`.  ← CURRENT
- [ ] 3. Amend the live prose: KD-20's parenthetical and the test conventions' shared-server bullet.

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 ITERATE (1 major, 1 minor, 3 notes), round 2 ITERATE (1 major, 2 notes, 1 minor), round 3 GO (2 notes, 2 recommendations); 3 of 3 rounds used.
- **Step 7**: the round-1 `SPEC-REMIT` on AC4 went to the owner, who ruled "strike it" (state file `prior_qa` round 3); the spec was amended by `spec-writer` at 4043659 and the design reconciled at 4798daf.
- **Step 7**: the orchestrator's `AskUserQuestion` for that ruling offered "restate / strike / leave" instead of the recipe's "amend / fix the design only / leave" — logged in `ai-docs/learnings.md` 2026-09-12.
- **Step 8 subtask 1**: `containerNameForDir` implemented as designed — `filepath.Base(dir)` validated against `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`, then `+ "-test-postgres"`. Table test written first, observed red against a placeholder returning `("", nil)` (all non-trivial cases failed by name), then implemented and observed green. All gates (`go build`, `go test ./...`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run`) passed.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 3 | "D5 claims `runDown` derives after its locator and reachability checks … nothing pins 'after `sm.probe`'" | design-internal | folded | design § Test Design subtask 2, the unreachable-server case @ 8e18df9 |
| G2 | 3 | "Its remedy — 'delete the stale locator file once' — currently exists only in the design document" | design-internal | folded | design § Decomposition subtask 3 row + § Risks risk 2 @ 8e18df9 |
| G3 | 3 | "D6 ground 1's negative … was reproduced here independently … and is correct" | design-internal | folded | design § D6 ground 1, negative recorded as executed at two commits @ 8e18df9 |
| G4 | 3 | "The unpinned `[measured …]` coordinates were re-resolved rather than returned … No round should be spent on them" | design-internal | folded | no action owed — every tag in the touched paragraphs already carries `path:lines` @ 8e18df9 |

## Key discoveries (don't re-investigate)

- The locator (`tmp/testpg-dsn`) is already per-checkout — it is resolved relative to the invoking process's working directory, and every caller invokes the wrapper from the repository root. Only the container name is host-global.
- The container runtime's refusal of an invalid name does NOT contain the offending name: `podman create --name 'lab game' …` → rc 125, and a grep for `lab game` over the captured output returns 0. Verified independently by `design-writer` and by `design-review`. This is why the design keeps its own pre-check.
- `Binaries` counts packages whose test files reference `testdb.Main` — a test reaching `testdb.StartServer` moves it not at all. The real reasons `cmd/testpg`'s suite stays runtime-free are locator clobbering, reaper-disabled containers outliving the binary, and the package's `TestMain` being `leaktest.Main(m, (*testing.M).Run)`.
- The verify-time probe must pass `--parallel 1` on **every** wrapper invocation including the child runs, and run under `env -u LAB_GAME_TEST_DSN`. Without the flag a child run computes its need from `GOMAXPROCS` (432 on this host) against a server sized for `--parallel 1` (100), falls through, and prints an anonymous container's DSN — the probe would fail for a reason unrelated to the change.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `cmd/testpg/run.go` — `containerNameForDir`, `testServerSuffix`, `containerNamePattern` (subtask 1)
- `cmd/testpg/run_test.go` — `TestContainerNameForDir`, `TestContainerNameForDir_rootPath_hasNoBaseName`, `TestContainerNameForDir_sameBaseName_differentParents_isEqual` (subtask 1)
