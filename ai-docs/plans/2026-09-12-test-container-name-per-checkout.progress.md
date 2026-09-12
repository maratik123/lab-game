# Progress: Test database container named per checkout — ACTIVE
_Updated: 2026-09-12 00:18_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-test-container-name-per-checkout
**base_commit:** 8e18df9f46e5b1ccec36e9c825a43dbb9b2e8f6f
**Last build:** PASS
**Issue:** #91
**Spec:** ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md
**current_step:** Step 11 — review fixes complete (Round 1)
**last_passed_gate:** make test | 2026-09-12T00:42:00Z | dbfad7a
**entry_args:** сделать так, чтобы имя контейнера бд выводилось из имени каталога проекта, например lab-game-test-postgres для ~/lab-game и lab-game2-test-postgres для ~/lab-game2 (для параллелизации разработки)

## Next action

**Do this immediately:** Step 10 — re-spawn `self-review` (warm) to re-verify its own round-1 findings.

## Subtasks

- [x] 1. The pure derivation: suffix constant, compiled validity pattern, directory → container name or error. Table test first.
- [x] 2. Wire it: the working-directory seam member, `runUp` / `runDown` derivation with their ordering pins, delete `testdb.SharedContainerName`.
- [x] 3. Amend the live prose: KD-20's parenthetical and the test conventions' shared-server bullet.

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 ITERATE (1 major, 1 minor, 3 notes), round 2 ITERATE (1 major, 2 notes, 1 minor), round 3 GO (2 notes, 2 recommendations); 3 of 3 rounds used.
- **Step 7**: the round-1 `SPEC-REMIT` on AC4 went to the owner, who ruled "strike it" (state file `prior_qa` round 3); the spec was amended by `spec-writer` at 4043659 and the design reconciled at 4798daf.
- **Step 7**: the orchestrator's `AskUserQuestion` for that ruling offered "restate / strike / leave" instead of the recipe's "amend / fix the design only / leave" — logged in `ai-docs/learnings.md` 2026-09-12.
- **Step 8 subtask 1**: `containerNameForDir` implemented as designed — `filepath.Base(dir)` validated against `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`, then `+ "-test-postgres"`. Table test written first, observed red against a placeholder returning `("", nil)` (all non-trivial cases failed by name), then implemented and observed green. All gates (`go build`, `go test ./...`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run`) passed.
- **Step 8 subtask 2**: added `workDir func() (string, error)` to `seam`, wired to `os.Getwd` in `productionSeam` and to a stub field in `stubSeam` (defaulting to `/stub/lab-game` when unset, so existing cases keep passing unmodified). `runUp` derives the container name before disabling the reaper/provisioning; `runDown` derives it after `sm.locate()`/`sm.probe()`, before its own `os.Setenv` — matching D5's ordering. `--up`'s success report line now names the provisioned container (D8). Deleted `testdb.SharedContainerName` and its doc comment from `internal/testdb/server.go`; `grep -rn SharedContainerName --include='*.go' .` returns nothing. Added the full case list from the design's Test Design subtask-2 section (differing/same-basename `--up` pairs, invalid-directory and workDir-lookup-failure cases for both `--up`/`--down`, the reaper-untouched assertions pinning pre-check position, the stale-locator-cleanup-with-invalid-dir case pinning D5's reachability half, and `runChild` never deriving a name). Confirmed the discriminating `--up` different-directory test fails (red) against a `containerNameForDir` mutant hardcoded to a constant, and passes (green) against the real implementation — checked the mutant **builds** first. Two comment-ref violations (`D5` cited as a decision anchor in two test comments) were caught by `go run ./cmd/commentrefs` and fixed by dropping the anchor, restating in prose. Full gate run: `go build ./...`, `go test ./... -count=1`, `go vet ./...`, `golangci-lint fmt -d`, `golangci-lint run`, `go run ./cmd/commentrefs`, `go run ./cmd/importguard` — all green.

- **Step 8 group boundary**: the orchestrator re-ran the group's gates itself rather than recording the delegate's claim — `go build ./...`, `go test ./cmd/testpg -count=1`, `go vet ./...`, `golangci-lint run`, `make comment-refs`, all green — and read the derivation call sites: `runUp` derives at run.go:267 above the reaper `Setenv` at :282, `runDown` derives at :350 below `sm.locate()` (:330) and `sm.probe()` (:336) and above its `Setenv` (:360), and `runChild` calls it nowhere. The delegate left `Last build`, `last_passed_gate` and `Next action` at their Step-8-creation values; refreshed here.

- **Step 8 subtask 3**: amended the two live prose surfaces at 57d104e. KD-20's parenthetical states the derivation (project-directory base name plus `-test-postgres`, both worked examples) instead of naming one fixed container, and a new `*Amended by #91*` clause records the derivation on `--up`/`--down`, the deleted `testdb.SharedContainerName`, the per-checkout consequence, the deliberate sharing between same-named directories, the refuse-rather-than-sanitise rule and the unchanged anonymous fallback; the `*Source:*` line names this design and issue beside the shared-server one and `*Amended:*` moves to 2026-09-12. The test conventions' shared-server bullet gains the derivation, the parallel-checkout consequence and one sentence carrying the one-time stale-locator remedy (delete that checkout's `tmp/testpg-dsn`, or remove the old container). The design's untouched-surface list was re-verified rather than trusted: a repository-wide sweep for the old name and the deleted symbol outside `.git` and `tmp` returns only this task's own plan files, the derivation and its table test, and the KD-20 line rewritten here — `AGENTS.md`, `ai-docs/context.md` and `ai-docs/context-status.md` stay as the design ruled. Gates: `go build ./...`, `go vet ./...`, `golangci-lint fmt -d`, `golangci-lint run`, `make comment-refs`, `make shellcheck`, the citation guard, `check-ac-shape` / `check-spec-shape` / `check-spec-anchors` / `check-script-shape` / `check-harness-gaps-forge` and CI's relative-markdown-link check — all green. The citation guard's green was made evidence about this file first: planting an out-of-range `#9999` on the edited KD-20 line turned it RED naming `ai-docs/key-decisions.md:53`, and the file was restored from a cp-backup.

- **Step 9**: `make verify` green in full at e547cce (fmt · build · vet · lint · file-limits · test · test-race · tidy · actionlint · shellcheck · comment-refs · import-guard) — 0 FAIL over the log, control confirmed the FAIL pattern matches. Deps unmoved (no `go.mod`/`go.sum` in the branch diff), no workflow or shell script touched, no `panic(`/`log.Fatal` added (control confirmed). Domain-invariant sweep not applicable: the change touches no balance, ledger, basis document, scheduler task, event or outbound message — the Go diff is `cmd/testpg` and `internal/testdb` only. Panic index unchanged: the only addition is a package-level `regexp.MustCompile`, which the index carries no row for by established practice in `internal/commentref`.
- **Step 9**: the design's verify-time probe ran end to end on podman 5.8.2 — two probe servers under derived names, a same-base-name twin joining the first, a child run picking its own checkout's server, `--down` in one leaving the other running and reachable, and teardown leaving no probe container (control confirmed the teardown grep matches). The unrelated `pgshared` container on this host was neither touched nor counted.

- **Step 9.5**: appended this task's entry to `ai-docs/context-status.md` with the literal PR placeholder, and extended the `internal/testdb` clause of `ai-docs/context.md` with the per-checkout naming and its consequence. No open question in `context.md` is resolved by this change. Removed-name sweep for `SharedContainerName` over every doc touched: the only surviving mention outside the deletion clause is the #67 entry of `context-status.md`, which is append-only history describing what landed in that PR — left standing, with the deletion recorded in this task's own entry. `README.md` and `docs/**` name no container and no wrapper target, so neither is contradicted.

- **Step 11**: both open findings fixed and each re-verified by execution, not by reading — the built wrapper run from a directory named `-badname` now prints one `testpg:` prefix, and the pointer comment is gone (control confirmed both greps match). `R1-4` was accepted by the reviewer and fixed anyway: `runDown` dials the located server through `sm.probe` before the derivation, so "on `--down` before anything is reached for at all" was false, and the append-only ground does not cover an entry added in this PR. The same over-strong clause survives in the design's D6 ground 3, whose correction is a Design Amendment trigger with the round cap already spent — surfaced to the owner rather than edited.
- **Step 11**: a `git show` of a pre-change source file was redirected to `tmp/pre.go`, which put a package inside the module and turned `go build ./...` and `golangci-lint run` RED; deleted, gates green again, logged in `ai-docs/learnings.md` 2026-09-12.

- **Step 11**: the owner ruled on the D6 ground-3 correction (state file `prior_qa` round 4): amend, with a per-instance exemption from a repeat design-review. `design-writer` corrected the one clause after re-reading `runDown` itself; the design now states the property is about the container, never about the dial, and names what `--down` does not spare. A scan of the whole design for the same over-strong shape returns nothing.

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

| # | Criterion | Test / Verification | Status |
|----|-----------|---------------------|--------|
| AC1 | The name is derived from the checkout's project directory name; `lab-game` → `lab-game-test-postgres`, `lab-game2` → `lab-game2-test-postgres` | `go test ./cmd/testpg -count=1 -run TestContainerNameForDir` — the table carries both worked examples verbatim (run_test.go rows "first checkout" and "sibling checkout"); probe step 4 — the runtime listed `alpha-test-postgres` and `beta-test-postgres` after an `--up` from each directory | PASS |
| AC2 | Taking the server down in one checkout leaves a differently-named checkout's server running and reachable | probe step 7 — after `--down` from alpha the listing still carried `beta-test-postgres`, and a child run from beta printed beta's DSN (listed *and* reachable, both read) | PASS |
| AC3 | Gates run in one checkout address that checkout's own server, never another's | probe step 6 — a child run from alpha printed alpha's DSN, with no `admits … falling through` line on stderr (instrument check, control confirmed the pattern matches); `go test ./cmd/testpg -run TestRunChild` — `runChild` derives no name at all | PASS |
| AC4 | Two checkouts whose project directories carry the same name address one and the same server | `go test ./cmd/testpg -run 'TestContainerNameForDir_sameBaseName_differentParents_isEqual|TestRun_up_sameBaseName_differentParents_sameContainerName'`; probe step 5 — the alpha twin's `--up` raised no container (listing unchanged) and its locator took alpha's DSN, both read because a silently-failed provision would also leave the listing unchanged | PASS |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-1 | round 1 | major | fixed@dbfad7a | `sed -n '509,510p' cmd/testpg/run_test.go` — the comment must carry no bare-name pointer |
| R1-2 | round 1 | minor | fixed@dbfad7a | `cd <dir named '-x'> && <built testpg> --up --parallel 1 2>&1` — the message must carry one `testpg:` prefix, not two |
| R1-3 | round 1 | nit | accepted@1 — established module-wide practice, 48 `_ = <call>` discard sites including production (`internal/scheduler/execute.go`, `internal/ingest/settle.go`, `internal/tgtest/tgtest.go`), most without the why-comment `code-style.md`:28 asks for; a new test-cleanup pair is consistent with the tree, not a regression | `git grep -cE '^[[:space:]]*_ = [a-zA-Z_][A-Za-z0-9_.]*\(' -- '*.go'` |
| R1-4 | round 1 | nit | fixed@dbfad7a — acceptance overridden by the orchestrator. The reviewer's ground was that the file is append-only history; that does not hold for an entry this same PR adds, and the claim itself is refuted by the code — `context-status.md`'s "on `--down` before anything is reached for at all" is true of the *container* but not of `sm.probe`, which dials the located DSN before the derivation; the sentence reproduces the design's own D6 ground-3 wording and the file is append-only history | `sed -n '330,345p' cmd/testpg/run.go` |
| R1-5 | round 1 | nit | accepted@1 — `run_test.go` at 711 lines crosses the 500 soft band but is one package's cohesive wrapper-test file; the checklist's counter-rule forbids flagging it, and `make file-limits` is green | `wc -l cmd/testpg/run_test.go && make file-limits` |
| R1-6 | round 1 | nit | accepted@1 — `runUp` now derives above `testdb.Ceiling`, so an invalid directory name outranks a bad `--clients`/`--parallel` pair in error precedence; neither an AC nor the design pins that order and both exits are non-zero and named | `sed -n '261,278p' cmd/testpg/run.go` |

## Files touched

- `cmd/testpg/run.go` — `containerNameForDir`, `testServerSuffix`, `containerNamePattern` (subtask 1); `seam.workDir`, `productionSeam.workDir`, `runUp`/`runDown` derivation wiring, `--up` report line naming the container (subtask 2)
- `cmd/testpg/run_test.go` — `TestContainerNameForDir`, `TestContainerNameForDir_rootPath_hasNoBaseName`, `TestContainerNameForDir_sameBaseName_differentParents_isEqual` (subtask 1); `stubSeam.workDirDir`/`workDirErr`/`forgetCalled`, and the `--up`/`--down`/`runChild` wiring cases (subtask 2)
- `internal/testdb/server.go` — deleted `SharedContainerName` (subtask 2)
- `ai-docs/key-decisions.md` — KD-20's parenthetical and its `Amended by #91` clause (subtask 3)
- `ai-docs/go-test-conventions.md` — the shared-server bullet: the derivation, the parallel-checkout consequence and the stale-locator remedy (subtask 3)

## Self-Review (Round 1)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | cmd/testpg/run_test.go:510 | major | `ai-docs/doc-convention.md` § DOC-4 → *What the gate decides, and what review decides*, bullet 2 bans "a bare unqualified name used as a pointer — 'see such-and-such'", refused in review because no lexical rule can catch it. The diff introduces one: `// environment write (see the comment on TestRun_upDown_useTheSeamWithNoRuntime).` The exemption above it keeps only "a same-package symbol that **is the contract** (DOC-3) — a returned sentinel error, the guarantor of a precondition"; a sibling test function whose comment carries a rationale is not that, and it rots exactly as DOC-4 states — rename that test and the comment lies. The pre-change file carried no such pointer (`git show 8e18df9:cmd/testpg/run_test.go` → no `see ` pointer), and the same diff uses the conforming form eight times (`// Not parallel, same reason as above.`), so a conforming spelling was available and used everywhere else. `make comment-refs` is green, as expected — this is the half it delegates. | ✅ Fixed |
| 2 | cmd/testpg/run.go:118 | minor | `containerNameForDir` builds its error already prefixed (`fmt.Errorf("testpg: project directory %q …")`) while both call sites re-prefix it (`logf(stderr, "testpg: %v\n", err)` at run.go:270 and run.go:353), so the shipped binary doubles the program name. Measured against the built wrapper from a directory named `-badname`: `testpg: testpg: project directory "-badname" must start with a letter or digit and contain only letters, digits, underscores, dots or dashes to name a test-server container`, exit 1. The same doubling appears in the test output of `TestRun_down_staleLocator_invalidDir_stillExitsZero` under mutant M4. D6 is still satisfied (the message names the directory and states the rule), so this is presentation, not behaviour; the fix is to drop `testpg: ` from the `fmt.Errorf`, matching `code-style.md` § Errors — the prefix names the operation, and the call sites already supply the program name. | ✅ Fixed |

### What was checked

**ACs — every verification command in `## AC Status` re-run against the shipped tree:**

- **AC1 — PASS.** `go test ./cmd/testpg -count=1 -run TestContainerNameForDir` → `ok`. The table carries both worked examples verbatim (`first checkout` → `lab-game-test-postgres`, `sibling checkout` → `lab-game2-test-postgres`).
- **AC2 — PASS (unit half), probe record accepted.** `--down` provisions under the derived name (`run_test.go:382`); the differing-directory inequality case pins distinct names. The container-level half rests on the Step 9 probe; its teardown claim was re-verified independently rather than read — `podman ps -a` shows only the unrelated `pgshared`, no probe container survived.
- **AC3 — PASS.** `go test ./cmd/testpg -count=1 -run TestRunChild` → `ok`. Mutation M6 (adding a derivation to `runChild`) turns `TestRunChild_invalidWorkDir_stillRunsToCompletion` red — the case is discriminating, not decorative.
- **AC4 — PASS.** `go test ./cmd/testpg -count=1 -run 'TestContainerNameForDir_sameBaseName_differentParents_isEqual|TestRun_up_sameBaseName_differentParents_sameContainerName' -v` → both `--- PASS`.

**Mutation checks — every ordering and derivation claim pinned, each mutant confirmed to BUILD first:**

| Mutant | What it breaks | Result |
|---|---|---|
| M1 | `containerNameForDir` ignores its argument | RED — 7 tests incl. `TestRun_up_differentDirs_differentContainerNames` |
| M2 | `runUp` derives **below** its reaper `os.Setenv` | RED — `TestRun_up_invalidDir_leavesReaperSettingUnchanged`, and only that test |
| M3 | `runDown` derives **above** `sm.probe` | RED — `TestRun_down_staleLocator_invalidDir_stillExitsZero` |
| M4 | `runDown` derives **above** `sm.locate` | RED — `TestRun_downWithNoLocator_isANoOp` + the stale-locator case |
| M5 | `runDown` derives **below** its reaper `os.Setenv` | RED — `TestRun_down_invalidDir_leavesReaperSettingUnchanged` |
| M6 | `runChild` also derives a name | RED — `TestRunChild_invalidWorkDir_stillRunsToCompletion` |
| M7 | `--up` report line drops the container name | RED — `TestRun_upDown_useTheSeamWithNoRuntime` |

Every D5/D6 ordering half has an assertion that can only pass at the intended position. `cmd/testpg/run.go` was restored from a cp-backup after each mutant and `git diff` confirmed clean.

**Production seam exercised directly** (the one wiring the stub tests cannot reach): the built wrapper run from a directory named `-badname` exits 1 naming the directory, touching no container; run with `--down` from a locator-free directory it exits 0 with "nothing to remove". `podman ps -a` unchanged, `git status` clean outside `tmp/`.

**Gates re-run against the shipped tree:** `go build ./...` GREEN · `go vet ./...` GREEN · `golangci-lint run` GREEN (0 issues) · `go test ./cmd/testpg -count=1` GREEN · `go test -race ./cmd/testpg -count=1` GREEN · `make comment-refs` GREEN · `make file-limits` GREEN · `make import-guard` GREEN. Coverage ratchet rose 89.69 → 89.84.

**Design conformance:** D1 (base name + fixed suffix, parent contributes nothing) · D3 (`testdb.SharedContainerName` deleted; `git grep ContainerName -- '*.go'` shows `cmd/testpg/run.go` as the module's only container-naming site, confirming KD-20's new claim) · D4 (`workDir` returns dir+error, wired to `os.Getwd` in `productionSeam`, stubbed in `stubSeam`) · D5, D6, D7 (package-level `regexp.MustCompile`, matching the `internal/commentref/classify.go` precedent; `ai-docs/panic-index.md` carries no row for that class and stays empty) · D8 (report line) · D9 (no new file). No architectural decision taken outside the design.

**GO notes round-trip:** all four rows route `folded` at 8e18df9, which is `base_commit` — the design was updated before the implementation diff started, and the design is unchanged in this diff (`git diff --stat` lists only the progress file under `ai-docs/plans/`). G1's folding is realised as `TestRun_down_staleLocator_invalidDir_stillExitsZero`; G2's remedy shipped in the test-conventions bullet; G3 and G4 owed no code. No stale design section found.

**Prose claims re-derived rather than read** (`key-decisions.md` KD-20, `go-test-conventions.md`, `context-status.md`, `context.md`): "the wrapper is the only thing in this module that names a container" — confirmed by `git grep ContainerName -- '*.go'`; "`tmp/testpg-dsn`" — confirmed at `cmd/testpg/locator.go:12`; "every caller invokes the wrapper from the repository root" — confirmed against the `Makefile` targets; "fails before the reaper setting is written and before anything is provisioned" — confirmed by M2/M5 and by the live binary run. Propagation sweep: `git grep -l lab-game-test-postgres` now reaches only this task's own plan files, the derivation's table test, and the two amended pages where the string is a *worked example* of the derivation, not a fixed name; `SharedContainerName` survives only in `ai-docs/context-status.md`'s #67 entry, which the design ruled is append-only history. `README.md` and `docs/**` name no container or wrapper target, so neither is contradicted.

**Not defects (recorded in the register, not raised):** R1-3 the `_ = os.Setenv` cleanup pair · R1-4 one over-strong sentence in `context-status.md` · R1-5 `run_test.go`'s 711 lines · R1-6 the `--up` error-precedence shift. Re-entry fields: `current_step`, `last_passed_gate` and `entry_args` are present; `parent_skill` is correctly omitted per the canonical template, since `/task` is the parent flow here.

**Spawn prompt:** within the closed list — invocation line, `Spec:`, `Design:`, `Progress:`, commit range. No `PROMPT-CONTAMINATION`.
