# Progress: Test database container named per checkout — ACTIVE
_Updated: 2026-09-12 00:00_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-test-container-name-per-checkout
**base_commit:** 8e18df9f46e5b1ccec36e9c825a43dbb9b2e8f6f
**Last build:** PASS
**Issue:** #91
**Spec:** ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md
**current_step:** Step 8 — Group A, subtask 2 of 2 complete (group A done)
**last_passed_gate:** golangci-lint run | 2026-09-12T00:13:18Z | a806d3a5318ec2fb4030f378b9b7589bc5552934
**entry_args:** сделать так, чтобы имя контейнера бд выводилось из имени каталога проекта, например lab-game-test-postgres для ~/lab-game и lab-game2-test-postgres для ~/lab-game2 (для параллелизации разработки)

## Next action

**Do this immediately:** hand off Group B (subtask 3, `ai-docs/key-decisions.md` + `ai-docs/go-test-conventions.md`) to `general-purpose` per the design's `## Handoff plan`.

## Subtasks

- [x] 1. The pure derivation: suffix constant, compiled validity pattern, directory → container name or error. Table test first.
- [x] 2. Wire it: the working-directory seam member, `runUp` / `runDown` derivation with their ordering pins, delete `testdb.SharedContainerName`.
- [ ] 3. Amend the live prose: KD-20's parenthetical and the test conventions' shared-server bullet.

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 ITERATE (1 major, 1 minor, 3 notes), round 2 ITERATE (1 major, 2 notes, 1 minor), round 3 GO (2 notes, 2 recommendations); 3 of 3 rounds used.
- **Step 7**: the round-1 `SPEC-REMIT` on AC4 went to the owner, who ruled "strike it" (state file `prior_qa` round 3); the spec was amended by `spec-writer` at 4043659 and the design reconciled at 4798daf.
- **Step 7**: the orchestrator's `AskUserQuestion` for that ruling offered "restate / strike / leave" instead of the recipe's "amend / fix the design only / leave" — logged in `ai-docs/learnings.md` 2026-09-12.
- **Step 8 subtask 1**: `containerNameForDir` implemented as designed — `filepath.Base(dir)` validated against `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`, then `+ "-test-postgres"`. Table test written first, observed red against a placeholder returning `("", nil)` (all non-trivial cases failed by name), then implemented and observed green. All gates (`go build`, `go test ./...`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run`) passed.
- **Step 8 subtask 2**: added `workDir func() (string, error)` to `seam`, wired to `os.Getwd` in `productionSeam` and to a stub field in `stubSeam` (defaulting to `/stub/lab-game` when unset, so existing cases keep passing unmodified). `runUp` derives the container name before disabling the reaper/provisioning; `runDown` derives it after `sm.locate()`/`sm.probe()`, before its own `os.Setenv` — matching D5's ordering. `--up`'s success report line now names the provisioned container (D8). Deleted `testdb.SharedContainerName` and its doc comment from `internal/testdb/server.go`; `grep -rn SharedContainerName --include='*.go' .` returns nothing. Added the full case list from the design's Test Design subtask-2 section (differing/same-basename `--up` pairs, invalid-directory and workDir-lookup-failure cases for both `--up`/`--down`, the reaper-untouched assertions pinning pre-check position, the stale-locator-cleanup-with-invalid-dir case pinning D5's reachability half, and `runChild` never deriving a name). Confirmed the discriminating `--up` different-directory test fails (red) against a `containerNameForDir` mutant hardcoded to a constant, and passes (green) against the real implementation — checked the mutant **builds** first. Two comment-ref violations (`D5` cited as a decision anchor in two test comments) were caught by `go run ./cmd/commentrefs` and fixed by dropping the anchor, restating in prose. Full gate run: `go build ./...`, `go test ./... -count=1`, `go vet ./...`, `golangci-lint fmt -d`, `golangci-lint run`, `go run ./cmd/commentrefs`, `go run ./cmd/importguard` — all green.

- **Step 8 group boundary**: the orchestrator re-ran the group's gates itself rather than recording the delegate's claim — `go build ./...`, `go test ./cmd/testpg -count=1`, `go vet ./...`, `golangci-lint run`, `make comment-refs`, all green — and read the derivation call sites: `runUp` derives at run.go:267 above the reaper `Setenv` at :282, `runDown` derives at :350 below `sm.locate()` (:330) and `sm.probe()` (:336) and above its `Setenv` (:360), and `runChild` calls it nowhere. The delegate left `Last build`, `last_passed_gate` and `Next action` at their Step-8-creation values; refreshed here.

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

- `cmd/testpg/run.go` — `containerNameForDir`, `testServerSuffix`, `containerNamePattern` (subtask 1); `seam.workDir`, `productionSeam.workDir`, `runUp`/`runDown` derivation wiring, `--up` report line naming the container (subtask 2)
- `cmd/testpg/run_test.go` — `TestContainerNameForDir`, `TestContainerNameForDir_rootPath_hasNoBaseName`, `TestContainerNameForDir_sameBaseName_differentParents_isEqual` (subtask 1); `stubSeam.workDirDir`/`workDirErr`/`forgetCalled`, and the `--up`/`--down`/`runChild` wiring cases (subtask 2)
- `internal/testdb/server.go` — deleted `SharedContainerName` (subtask 2)
