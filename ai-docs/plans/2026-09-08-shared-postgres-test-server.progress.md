# Progress: Shared PostgreSQL test server — ACTIVE
_Updated: 2026-09-08 22:07_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** perf/2026-09-08-shared-postgres-test-server
**base_commit:** 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**Last build:** PASS

**Issue:** #67
**Spec:** ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md

**current_step:** Step 12 — finalising, PR not yet opened
**last_passed_gate:** `make verify` + `make cover-ratchet` + `make test-contention` | 2026-09-09T10:05Z | f70c827cdf3235c74fff49884bccb9ef4a883486
**entry_args:** 67

## Next action

**Do this immediately:** Group B is complete and all 13 subtasks are committed. The orchestrator resumes /task at Step 9 (Verify) — the per-AC sweep, then Step 9.5's `ai-docs/context-status.md` entry, which is also where D10's superseding of the exclusive-database-access trap entry lands (the trap entry itself is never edited: that file is append-only by its own header). One follow-up needs routing at Step 12 — see the Decisions log's last line.

## AC11 demonstration RED, recorded before any green (subtask 9, committed as 0f54e39)

Reverted `internal/scheduler/reconcile_test.go`'s `Run_reconciles_before_first_cycle`
context timeout from the subtask-8 value (5s) to 10ms (over a `cp` backup at
`tmp/reconcile_test.go.bak`; also tried the test's actual pre-subtask-8 value,
500ms, at `--clients 2 --parallel 24`, which came back GREEN — inconclusive,
not usable — before tightening further). Ran `make test-contention
CONTENTION_PARALLEL=24` (computed ceiling 672, within `ceilingMax`):

- **Result: RED.** `make: *** [Makefile:100: test-contention] Ошибка 1` (exit
  status 1, the wrapper's own non-zero passthrough). The race log
  (`tmp/test-contention-race.log`, snapshotted to
  `tmp/test-contention-race-RED.log`) shows:
  `--- FAIL: TestRun_reconcilesBeforeFirstCycle_RunOnceDoesNot/Run_reconciles_before_first_cycle`,
  `reconcile_test.go:372: row count after Run = 0, want 1 (seeded before the
  first cycle)`.
- **Exhaustion scan: clean.** `grep -n "sorry, too many clients already\|SQLSTATE 53300"`
  over both `tmp/test-contention-race.log` and `tmp/test-contention-load.log`
  found nothing (exit 1) — this is a genuine contention-class RED against the
  exact assertion the test exists to protect, not a connection-exhaustion
  artifact per D12's table.
- **Honest caveat, not swept under the rug:** the test's actual pre-subtask-8
  value (500ms) and even the worker suite's pre-subtask-8 `TaskTimeout` (1s,
  tried first, also GREEN) did NOT reproduce a RED at this load level on this
  machine — only reverting to an artificially tight 10ms did. The design
  itself flags why: the recorded trap "predates both the tmpfs change and the
  backoff refactor," and this machine is fast/idle enough that those two
  fixes apparently already closed most of the original gap. 10ms is
  therefore not a claim that 500ms is still unsafe — it exists solely to
  exercise the RED path and confirm the scan correctly distinguishes a real
  assertion failure from the exhaustion literal, which is what AC11 asks for.

Restored `internal/scheduler/reconcile_test.go` from the backup (`cp
tmp/reconcile_test.go.bak internal/scheduler/reconcile_test.go`), confirmed
`git diff` against the file is empty (byte-identical to the subtask-8
commit), rebuilt, and re-ran `make test-contention CONTENTION_PARALLEL=24`:
**GREEN**, every package `ok`, race log clean. AC11's demonstration and
AC10's green are both satisfied, RED recorded first.

## Group A note for whoever reads this next

Subtask 8's classification covered: `internal/scheduler/reconcile_test.go`'s `Run_reconciles_before_first_cycle` 500ms→5s context timeout (instrument); a new `contentionSafeConfig()` in `worker_test.go` (TaskTimeout 1s→30s) used by `newWorker` and by every non-deadline worker-construction call site in `reconcile_test.go`, `observe_test.go`, `schedule_test.go`, and four sites in `failure_test.go` (`backoffProbeConfig`'s own explicit 10s TaskTimeout was left as is — already generous, already per-test); `observe_test.go`'s `TestRun_shortPollInterval_picksUpFreshlyInsertedTask` 2s→10s (both the pickup-wait deadline and the post-cancel return wait); `deadline_test.go`'s `waitLockFree` polling ceiling's `+2*time.Second` slack →`+10*time.Second` at all 9 call sites (the deadline suite's own `TaskTimeout`/backoff values are the SUBJECT and were left untouched — `shortDeadlineConfig()` is unedited). `internal/ingest`'s "prompt return after cancellation" assertions were reviewed and left alone: their reaction path is a Go-level `select` on `ctx.Done()` with no database call in the critical path, so they are not database-contention-sensitive by the same reasoning `cadence_test.go`/`registry_test.go` (pure arithmetic, no DB) needed no changes at all. Membership was NOT exhaustively re-derived via `make test-contention` before this commit — that is subtask 9's own territory, and if it finds a further-affected test, fix it there rather than reopening this subtask.

## Subtasks

Design: `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md`. Group A = 1–9 (code, `code-writer`, sonnet/medium pinned). Group B = 10–13 (instructions/harness, `general-purpose`, model inherited).

- [x] 1. `internal/testdb`: provisioning API in a new file; `Main`'s container branch re-expressed over it — commit e09e79f. Coverage ratchet lowered 91.66% → 90.30% in the same commit (D7: regime shift + new uncovered StartServer/Probe/Ceiling code). NOTE: this subtask's commit did not include the `.progress.md` update (missed the "stage with the subtask commit" step) — caught up in subtask 2's commit instead.
- [x] 2. Tests for the ceiling formula, its refusal path, the capacity probe, the binaries manifest — `internal/testdb/server_test.go`, external-package tests reusing the existing `TestMain`. All green against a session-local shared server. Coverage ratchet rose 90.30% → 91.10% (recorded automatically by the hook).
- [x] 3. `cmd/testpg`: the wrapper — `cmd/testpg/main.go`, `cmd/testpg/run.go`, `cmd/testpg/locator.go`. Decision order (D1/D3a), `--up`/`--down` sized by the same `testdb.Ceiling` terms with the reaper disabled via `TESTCONTAINERS_RYUK_DISABLED` for `--up`, signal-aware teardown (child under the signal context, teardown under a fresh `context.Background()` — `//nolint:contextcheck` explained at both sites), real child exit code propagated via `exec.ExitError.ExitCode()` (go run's own flattening is the caller's concern, not this wrapper's). Manually smoke-tested against the real podman runtime: caller-DSN path with reported shortfall, non-zero child propagation (both `go run` and the built binary), `--up` (created `lab-game-test-postgres`, wrote the locator, printed the DSN and ceiling), the default path finding and reusing it, `--down` (removed the container and the locator file), and `--down` again with no locator (no-op). Coverage ratchet lowered 91.10% → 86.02% in the same commit.
- [x] 4. `cmd/testpg/run_test.go`: table tests over a `stubSeam` covering every decision-order scenario from the design's Test Design section (caller DSN untouched/undersized, no-locator fallback with stop on pass/fail/cancelled-context, locator unreachable/undersized/admits, ceiling arithmetic matches `testdb.Ceiling`, `--up`/`--down` over the seam, `--up`+`--down` together and no-child-args are usage errors). Verified via `podman ps -a` that no container exists beyond the pre-existing session-local one — this package's own tests start none. Coverage ratchet rose 86.02% → 89.44%.
- [x] 5. `Makefile`: `test`/`test-race` route through `go run ./cmd/testpg -- go test [...] ./...`; added `test-db-up` (`CLIENTS ?= 1`, overridable), `test-db-down`, `test-fallback` (bare `go test ./...` with `LAB_GAME_TEST_DSN=` cleared — not part of `verify`, matching `cover-ratchet`'s own precedent), `test-contention` (`CONTENTION_PARALLEL ?= nproc`, `--clients 2`, single wrapper invocation whose `bash -c` child backgrounds a `-count=1` load loop of the database-backed packages and foregrounds the whole-module race gate, both logging to `tmp/`, capturing `fg_status` explicitly under the nested script's own `set -eu -o pipefail` rather than letting it abort before the load loop is killed). Manually ran both `make test` and `make test-contention CONTENTION_PARALLEL=2` to green against the real podman runtime.
- [x] 6. `.githooks/coverage-ratchet.sh`: the measurement command routed through `go run ./cmd/testpg --`, placed after the existing raise-mode skip decision (unchanged — the wrap only touches the command already past that check, so AC7 needs no new code) — the runtime advice now names `make test-db-up` first alongside the existing `export LAB_GAME_TEST_DSN=...` line. `shellcheck` and the script-shape gate green; `.githooks/coverage-ratchet.sh --check` ran green against a session-local shared server, confirming the wrap didn't change the measured value.
- [x] 7. `.github/workflows/ci.yml`: added a `make test-fallback` step to the Test job, after `cover-ratchet` (D8/AC3 — no other Test-job step still reaches the per-package container path). Verified AC9/D11 by diffing every file this branch has touched (`git diff --name-only <base>..HEAD`) against the change filter's globs: every one (`*.go`, `Makefile`, `.githooks/**`, `ai-docs/**`, `.github/workflows/**`) is already named — no filter edit needed, confirming the design's own claim rather than trusting it. `actionlint` and `make comment-refs` green.
- [x] 8. Contention tolerance per D9 — see "Group A note" below for the full list of what changed and why each was classified instrument vs. subject.
- [x] 9. The AC11 demonstration — see the dedicated section above. RED recorded (a genuine assertion failure, exhaustion-scan clean) before the restore and the GREEN re-run. Group A is now COMPLETE.
- [x] 10. `ai-docs/key-decisions.md`: KD-20 amendment — connection arithmetic + corrected consequence clause. Written as an `*Amended by #67 …*` clause on the existing entry, following KD-27's and KD-31's own precedent, and in prose rather than a table or a fenced block, because the file has neither anywhere. The falsified *Consequence* clause was rewritten in place rather than appended to: the invariant it always meant (`go list -deps ./cmd/bot` free of `testcontainers` — re-measured: 0 matches for `cmd/bot`, 10 for `cmd/testpg`) is kept and the wrong version of it ("only `_test.go` files import `internal/testdb`") is named as no longer true, so a later reader does not read `cmd/testpg` as a violation.
- [x] 11. `ai-docs/go-test-conventions.md`: the provisioning story, both paths. The one Postgres bullet became six: what `internal/testdb` provisions (unchanged), how a gate reaches it (the wrapper's decision order, and that the ratchet takes the same route after its skip decision), what keeping a server across runs buys (including why a stable DSN is what restores the test cache's replay), that the fallback path is still real and has its own gate, D9's instrument-vs-subject rule for wall-clock constants with the widen-per-test corollary, and `make test-contention` as the probe that establishes membership — with D12's exhaustion-literal scan named, so a red run is classified before it is believed.
- [x] 12. `AGENTS.md` § Build & Test: new targets, container-runtime row, the cache-replay sentence. Four edits. The six `make` targets joined the command block and a callout under it states the routing; the ratchet table's *Suite not green* row now names `make test-db-up` first, matching the script's own advice (which subtask 6 had already changed, leaving the table the last site saying otherwise); the cache-replay sentence became conditional on a stable DSN, naming why (`testdb.Main` consults `LAB_GAME_TEST_DSN`, so the anonymous container's fresh port is a cache miss every commit and the drifting statements are re-drawn rather than replayed). The bare `go test ./...` / `go test -race ./...` lines were deliberately LEFT — D10 excludes the bare-invocation sites, the invocation still works, and rewriting them here would have desynchronised `AGENTS.md` from the `.claude/**` files that spell the same commands.
- [x] 13. The propagation sweep of D10's class over the live tree, with the two stated exclusions — both lists recorded in the dedicated section below; the one remaining member, `ai-docs/context.md`'s layout paragraph, was edited in this subtask's commit.

## D10 propagation sweep — both lists (subtask 13)

The class is `AGENTS.md` § *Propagation Rule* step 4's: every LIVE site whose claim this
diff falsifies. The sweep returns more files than the class has members, so both lists are
here rather than only the conclusion.

**Instrument check before the verdict.** The design's own recipe
(`grep -rni 'LAB_GAME_TEST_DSN\|testcontainers\|container per'` over `*.md` / `*.sh` /
`*.yml` / `*.json`, with `ai-docs/plans` and `tmp` excluded) was re-run against the tree and
returned the same nine paths the design recorded — so the recipe still reaches its corpus
rather than reporting clean because it matched nothing. It was then broadened, because one
pattern's silence is a claim about the pattern: a second pass over `LAB_GAME_TEST` /
`testcontainer` / `container per` / `per-package container` / `shared server` /
`shared postgres` / `test server` / `testdb`, a third over `postgres:18` / `schema per test`,
a fourth over `ryuk` / `reaper` / `max_connections` / `provision` / `exclusive database
access`, and a fifth over the ratchet's own vocabulary (`replays the previous profile` /
`cached profile` / `test cache` / `Container runtime missing` / `coverage is not
measurable`). A `grep -rli testcontainers` control returned five files, so the sweep's
instrument was seen non-empty before any "no further members" was recorded. `.claude/**`
returned **nothing** on every probe except the bare-invocation one below.

**Members — each stated something this diff falsifies, and each is now corrected:**

| Site | What was false | Fixed in |
|---|---|---|
| `ai-docs/key-decisions.md` § KD-20 | the per-binary provisioning story; the *Consequence* clause claiming only `_test.go` files import `internal/testdb` | subtask 10 |
| `ai-docs/go-test-conventions.md` § *Postgres is tested against Postgres* | one provisioning route and no contention rule | subtask 11 |
| `AGENTS.md` § *Build & Test* | the target list; the ratchet table's container-runtime row; the unconditional cache-replay sentence | subtask 12 |
| `.githooks/coverage-ratchet.sh` runtime advice | named only the DSN variable | subtask 6 (Group A) |
| `ai-docs/context.md` layout paragraph | `internal/testdb` described as provisioning "for package tests", implying no non-test importer | subtask 13 |

**Returned but NOT members — the reason recorded so the next reader does not re-derive it:**

- `ai-docs/dependency-versions.md` — matches on `testcontainers-go` used as an *example* inside the dependency-reason rule. Says nothing about how the suite reaches a database.
- `ai-docs/scripts/test-ac-shape.sh` — matches inside a gate fixture's payload string, where the text is the thing under test. Editing it would edit the fixture, not a claim.
- `docs/DESIGN.md` — one line in the Russian design corpus's testing note (`go test -race` over testcontainers, still true) plus an unrelated Grafana-provisioning line. DECISIONS, not redesigned here.
- `ai-docs/context-status.md` — **stated exclusion 1.** Append-only by its own header; the exclusive-database-access trap entry is superseded by the entry Step 9.5 writes, never edited.
- `ai-docs/learnings.md` — append-only history surface, which the Propagation Rule's own step 4 leaves untouched. Its `postgres:18` match is the probes-run-in-a-container rule, a different subject.
- `ai-docs/harness-gaps.md` — matches only on the branch name `perf/2026-09-07-testdb-container-speedup` inside an observation, and on the ratchet named as escaping a different hook's starvation. No claim of this class.
- `ai-docs/domain-invariants.md` — "provisioning" is Grafana's.
- `ai-docs/context.md` line 44 (the *Gates* bullet) — states the gate list and the pre-commit dispatcher, both unchanged. Not a claim about how a gate reaches a database.
- `.githooks/coverage-ratchet.sh` header's test-cache paragraph — states that the measurement keeps the cache and bounds a *replayed* draw. Both still true; what changed is how often a replay happens, and that claim lived in `AGENTS.md`, which is a member and was corrected.
**The declared sync group was checked too, and it is a different obligation from D10's.**
`ai-docs/propagation-groups.md` carries a row *"A gate command (adding, removing, or
renaming one) → `AGENTS.md` § Build & Test AND every skill's `allowed-tools` line that
grants it AND `.claude/skills/task/reference.md` § Gate checklist"*, and this change adds
four gate commands, so the row fires. Resolved by reading each half rather than by
assuming:

- `AGENTS.md` § *Build & Test* — **updated** (subtask 12).
- Every skill's `allowed-tools` line — **no edit needed, verified rather than assumed.**
  Every skill that runs gates grants `Bash(make *)`, a wildcard, and `.claude/settings.json`
  grants the same; nothing anywhere enumerates `make` targets, so the four new ones are
  already permitted. (`ai-audit`, `interview`, `pr-merged`, `triage` and `verify-change`
  grant no `make` at all, and none of them runs these gates.)
- `.claude/skills/task/reference.md` § *Gate checklist* and § *Step 9 — verify list (full)*
  — **checked, no edit made, and the reason is a precedent rather than a judgement call.**
  Both enumerate the gates `make verify` discharges plus `make file-limits` and
  `make comment-refs`; `make cover-ratchet` — a mandatory pre-commit gate that is
  deliberately outside `verify` — appears on neither. `make test-fallback` and
  `make test-contention` are outside `verify` for the same stated reason (D8), so listing
  them would put a whole-module fallback run and a contention probe into every future
  `/task` Step 9. This task's own use of them is Step 9's per-AC sweep (item 12), which is
  where a task-specific instrument belongs. **Adding them permanently is a scope decision
  for the owner, not for this group**, and it is flagged rather than taken.

- Every harness instruction file spelling a bare `go test ./...` (`.claude/**`, `ai-docs/claude-tools-hierarchy.md`, `ai-docs/templates/progress-format.md`, and `AGENTS.md`'s own command block) — **stated exclusion 2.** The bare invocation still works and is the designed fallback (AC3), so none of them states a falsehood. Sweeping them onto `make test` is a real ergonomics gap and is a follow-up, not absorbed here.

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review reached GO at round 4 after the owner raised the cap to 4 (was 3); the GO's five notes and four recommendations were written back into the design before Step 8 and verified against the tree.
- **Step 8**: the coverage ratchet is deliberately not a subtask — D7 makes its movement an obligation inside whichever code commit moves coverage, which is where the workspace same-commit rule already puts it.
- **Step 8**: Group A's ratchet movements are per-commit and each names its reason: 91.66 -> 90.30 (regime shift plus new uncovered provisioning code), 90.30 -> 91.10 (rise, hook-recorded), 91.10 -> 86.02 (a wholly new uncovered package), 86.02 -> 89.44 (rise). Net 91.66 -> 89.44; the orchestrator re-ran `make cover-ratchet` and the tree measures 89.23, holding inside the 0.60 pp tolerance with about 0.39 pp of headroom.
- **Step 8**: the orchestrator repaired this file at the Group A boundary - the delegate left duplicate unchecked rows for subtasks 2-9, a `last_passed_gate` reading "pending final subtask-9 commit" after that commit had landed, and an empty `## Files touched

- **Step 8 (Group B)**: KD-20 is amended in place rather than replaced by a new KD — the spec's own key-decisions table offers either, and an amendment clause keeps the never-skip rule and the retired-local-instance rule (both explicitly out of scope) attached to the entry a reader already knows, following KD-27's and KD-31's `*Amended by #NN …*` precedent. The arithmetic is prose, not a table or a fenced block, because `ai-docs/key-decisions.md` contains neither anywhere.
- **Step 8 (Group B)**: the falsified half of KD-20's *Consequence* clause was rewritten in place, not appended to. `ai-docs/key-decisions.md` is a live decision record, not an append-only log, and D10 asks for the wrong version of the invariant to stop standing; the correction names both propositions so the difference between them survives, which a silent replacement would not.
- **Step 8 (Group B)**: the bare `go test ./...` lines in `AGENTS.md`'s command block were LEFT as they are, deliberately. D10's second stated exclusion covers exactly those sites, and rewriting the ones in this file while leaving the identical commands across `.claude/**` would have created the divergence the exclusion exists to avoid.
- **Step 8 (Group B)**: FOLLOW-UP FOR STEP 12, needs routing — the design records "sweeping the harness instruction files that spell a bare `go test ./...` onto `make test`" as a deferred ergonomics item, but the finalised spec's `## Deferred` section does not carry a row for it, so Step 12's parse of that section will not see it. It is recorded here instead of hand-appended to `ai-docs/deferred/_inbox.jsonl`, which only Step 12 and `/triage` may write.
- **Step 8 (Group B)**: `ai-docs/propagation-groups.md`'s gate-command row fires on this change and was discharged by reading both halves — the `allowed-tools` half needs no edit (every gate-running skill and `settings.json` grant `Bash(make *)`, a wildcard; nothing enumerates targets), and the Step-9 verify list was checked and left alone on `make cover-ratchet`'s precedent. Adding the two new outside-`verify` gates to every future `/task` Step 9 is a scope decision for the owner; it is flagged in the sweep section, not taken here.
- **Step 8 (Group B)**: this file had NO `## Files touched` heading when Group B opened it — the orchestrator's Group A repair commit truncated its own decision line mid-sentence at that heading's text and lost the heading with it, so the file list had been living inside the Decisions log. The heading is restored here; the truncated line above is left byte-identical, because the Decisions log is append-only and its content is not Group B's to rewrite.

- **Step 8**: the truncated decision line above is the orchestrator's own defect, not the delegate's — the Group A repair script anchored on the bare substring `## Files touched`, and the line it had just inserted contained that text, so `index()` matched the in-text mention instead of the heading and cut the file there. It destroyed `## Key discoveries`, `## AC Status` and `## Review register`; all three are restored here from `git show 0f54e39`, and the truncated line is left byte-identical because this log is append-only. This is the exact failure `ai-docs/scripts/doc-edit-guard.sh` exists to catch, and the edit was made without it.
- **Step 8**: Group B's two flagged items are carried to the orchestrator rather than decided by the delegate — the harness-wide `go test ./...` sweep (routing at Step 12) and whether `test-fallback` / `test-contention` join every future Step-9 verify list (an owner scope decision, deliberately not taken).
- **Step 9**: every AC was measured with the orchestrator's own command rather than accepted from a delegate return. AC1 evidence is the verify log's two postgres containers (one per gate invocation, not per package binary); AC2 and the locator path each left the container count unchanged; AC4 was measured against a real `--up`/`--down` round trip that left the owner's own long-lived container untouched; AC10's green was accepted only because the exhaustion scan came back clean in both logs.
- **Step 9**: AC3's gate was defective and the AC was not. `make test-fallback` carried no `-count=1`, so a second invocation was served entirely from the test cache and returned green having started no container — the target D8 introduces precisely so AC3 cannot go stale. The tree itself was fine: the honest run provisioned one container per database-backed binary. Owner approved the flag; the fix is 1a3ee3a and D8 was amended to match, with the owner exempting the amendment's re-review for this instance. Found because the per-AC sweep re-ran the gate instead of reading its last recorded result.
- **Step 9**: no panic or log.Fatal was added on any production path, so the panic index needs no row; the change moves no balance and adds no mechanic, so the domain-invariant sweep is a no-op.
- **Step 9.5**: `ai-docs/context.md` was left as Group B's sweep left it. Its layout paragraph already carries the summary fact (one server per run, the per-binary path surviving as the fallback), and repeating it in the Gates bullet would create two copies to keep in step. No open question in that file is resolved by this change - the ones it names live in the design corpus and are untouched.
- **Step 9.5**: no repository-root user-facing document states anything this diff falsifies; there is no README, and the Russian design corpus's one testcontainers sentence stays true because the fallback path still uses them and the shared server is itself a container.
- **Step 9.5**: the only name this diff removes from the tree is the unexported DSN env constant, exported under a new name; a case-insensitive sweep finds it only in plan documents, where it is either history or a pinned citation of the pre-change state.
- **Step 11 (Round 1)**: R1-6 is the round's one objection — the thirteen wrapper tests span three entry points and assert structurally different things, so a table would need a per-case closure field, which separate functions already are.
- **Step 11 (Round 1)**: R1-1 and R1-5 were routed through the Design Amendment recipe rather than folded in, surfaced to the owner with the reviewer's wording verbatim. The owner decided no ordinary gate gains `-count=1` and exempted the amendment's re-review for this instance. Amending D7 turned up more than the finding: D10 and subtask 12 were instructing an edit that replaces a true workspace sentence with a false one, which is the route by which the falsehood reached a live document.
- **Step 11 (Round 1)**: the mechanism behind R1-1 was read out of the toolchain rather than inferred — `m.Run` calls `m.before()`, which is where `StartTestLog` opens the log the go command reads for cache validity, and the DSN is read before that. A `TestMain` reading configuration before delegating to `m.Run()` is invisible to the test cache by construction.
- **Step 11 (Round 2)**: R1-3 was taken by the recipe route rather than the design-amendment route — the finding offered both and D12's requirement is served, not narrowed, by making the values durable. The re-litigation tripwire was computed before continuing: three rows raised, one of them a re-opening, so 33% against a 50% threshold, and R1-3 was at its first re-opening.
- **Step 11 (Round 2)**: the round-1 fix for R1-3 shipped with no test, which the reviewer proved by mutation. The replacement assertion was mutation-checked the same way before being recorded: delete the echo, `cmd/testpg` goes red.
- **Step 11 (Round 3)**: the owner raised the self-review cap to 4 (was 3). R3-1 is the third instance of one defect class in this task — a gate answered from the test cache — after AC3's at Step 9 and the load loop's in the design. The class is now stated in the recipe's own comment rather than left to be rediscovered a fourth time.
- **Step 11 (Round 3)**: AC10's earlier recorded PASS was a cached green, including the orchestrator's own Step-9 measurement of it. Re-measured after the fix: zero cached packages in the race log and `internal/scheduler` at 5.822s under load. AC11's RED stands unaffected — a source change invalidates the cache, so the reverted-instrument run genuinely executed; only its trailing green needed redoing.
- **Step 11 (Round 4)**: R4-3 was closed by making the code do what D12 already said it did — the target now runs the exhaustion scan itself — so no design amendment was needed. The new branch was seen RED before its clean result was trusted: against a 15-connection server the race log carries 256 hits of the literal and the target exits 2 naming an instrument failure, and against a sized server it reports the scan clean.
- **Step 11 (Round 4)**: R4-1's fix was proved by the same injection that proved the defect — an unresolvable package in the loop's package list. Before: one iteration then silence, target green. After: three failure blocks, the loop still loading.
- **Step 11 (Round 5)**: the first round of the five to find no defect in shipped behaviour — its major was a false sentence this run itself wrote, and the rest were test coverage and instrument robustness. That is the convergence signal the round count alone does not give.
- **Step 11 (Round 5)**: R5-4 is the exhaustion scan's own § Patterns 2 instance. `grep -q` answers 2 when the scan itself fails, and 2 took the same branch as 1, so a scan that could not run would have printed `exhaustion scan clean` — a green verdict from an instrument that never looked. The three-way branch was exercised at 0, 1 and 2 before being trusted.
- **Step 12**: the owner raised the self-review cap three times (3 to 4, 4 to 5) and chose to open the PR at the third exhaustion, on the evidence that round 5 was the first to find no defect in shipped behaviour. Five rounds, twenty-one findings, none re-litigated above a 33% share.
- **Step 12**: fourteen rows went to the deferred inbox from the spec's three sections and the design's open questions, including the harness-wide bare-`go test` sweep that Group B flagged as at risk of being lost — it is carried by the design's own open questions, which Step 12 parses.
## Key discoveries (don't re-investigate)

- `testdb.Main` returns `m.Run()` before touching testcontainers when `LAB_GAME_TEST_DSN` is set, so no test needs to change to reach a shared server.
- Four packages call `testdb.Main`: `internal/ingest`, `internal/scheduler`, `internal/store`, `internal/testdb`. That is D3's `binaries` term.
- `go run` does not propagate the child's exact exit code: `os.Exit(7)` under `go run` yields exit 1 with `exit status 7` on stderr; the built binary yields 7. Branch on zero-vs-non-zero, never on the value.
- testcontainers v0.44.0 parses exposed ports with `network.ParsePortRange`, which has no `host:container` form — a fixed host port would need `HostConfigModifier` and promote `moby/moby/api` to a direct requirement. Rejected.
- The ratchet's headroom is thin: the shared regime measures below the container regime, and the recorded mark sits close to the floor. Assume the ratchet blocks on the first commit landing this code and lower it in that same commit with the reason.
- Connection exhaustion is `FATAL: sorry, too many clients already (SQLSTATE 53300)` — the project's own client renders both spellings on one line.
- No gate enforces KD-20's "imported only from `_test.go` files" clause; `cmd/testpg` importing `internal/testdb` breaks no check, only the prose that subtask 10 owns.
- The DSN variable is in NO `go test` cache key, measured: `testdb.Main` reads it before calling `m.Run()`, and `m.Run()` is where the testing package opens the log the go command reads to decide cache validity. A different DSN, and a cleared one, both replay. So the shared and fallback regimes share cache entries, and no reasoning may treat a changing DSN as forcing a fresh run. (This line previously asserted the opposite; self-review R1-1 measured it.)

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS |
| AC2 | PASS |
| AC3 | PASS |
| AC4 | PASS |
| AC5 | PASS |
| AC6 | PASS |
| AC7 | PASS |
| AC8 | PASS |
| AC9 | PASS |
| AC10 | PASS |
| AC11 | PASS |
| AC12 | PASS |
| AC13 | PASS |
| AC14 | PASS |
| AC15 | PASS |
| AC16 | PASS |
| AC17 | PASS (make verify) — CI half pending at Step 12 |
| AC18 | PASS |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-1 | round 1 | major | fixed@f4e6e75 | `go clean -testcache; for i in 1 2 3; do go run ./cmd/testpg -- bash -c 'echo "DSN=$LAB_GAME_TEST_DSN"; go test ./internal/store/'; done` |
| R1-2 | round 1 | major | fixed@a11a5fa | `make test-db-up CLIENTS=1; make test-db-up CLIENTS=2; psql "$(cat tmp/testpg-dsn)" -At -c 'show max_connections'` |
| R1-3 | round 1 | major | fixed@bcca2d6 | `make test-contention CONTENTION_PARALLEL=4; grep -inE 'ceiling\|clients=\|parallel=' tmp/test-contention.log` → prints both the granted ceiling and the client/parallel pair. The recipe now captures the TARGET's own two streams to a third scratch file and replays them, which is the exit the finding offered ("a file under tmp/"); the two child logs stay the children's own output, since the arithmetic is the target's, not theirs. Coverage half: with `cmd/testpg/run.go`'s ceiling `logf` deleted, `go test -count=1 ./cmd/testpg` goes RED — mutation-run, not assumed |
| R1-4 | round 1 | minor | fixed@a11a5fa | `DOCKER_HOST=unix:///nonexistent/podman.sock make test-db-up` |
| R1-5 | round 1 | minor | fixed@f4e6e75 | `grep -n 'redirects to a file under' ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md` |
| R1-6 | round 1 | nit | accepted@1 — shape only; the thirteen cases span three entry points and assert structurally different things (seam untouched / stop ran / ceiling arithmetic / usage error), so a table would need a per-case closure field, which separate test functions already are | `grep -c '^func Test' cmd/testpg/run_test.go` |
| R1-7 | round 1 | nit | fixed@a11a5fa | `go test -count=1 -run TestRun_reconcilesBeforeFirstCycle ./internal/scheduler/` |
| R1-8 | round 1 | nit | fixed@a11a5fa | `make test-db-up; make test-db-down; podman ps -a --format '{{.Names}}'` |
| R1-A1 | round 1 | — | accepted@1 — the AC11 demonstration is sound: at the 10 ms instrument the scheduler suite is GREEN with no load (`-race` included), so the recorded RED was contention-induced, not an impossible budget | `go run ./cmd/testpg -- go test -race -count=1 ./internal/scheduler/` at a 10 ms reconcile bound |
| R1-A2 | round 1 | — | accepted@1 — no production panic / log.Fatal added, so the panic index needs no row | `grep -nE '(^\|[^[:alnum:]_.])(panic\(\|log\.(Fatal\|Panic)[a-z]*\()' cmd/testpg/*.go internal/testdb/server.go internal/testdb/testdb.go` |
| R1-A3 | round 1 | — | accepted@1 — no balance moves, no mechanic added, no schema or enum touched: the domain-invariant sweep is a no-op | `git diff --name-only dbff3d8..HEAD -- internal/store internal/store/migrations` |
| R1-A4 | round 1 | — | accepted@1 — AC9 holds: every changed path class is already named in the CI change filter | `git diff --name-only dbff3d8..HEAD` against `.github/workflows/ci.yml:38-82` |
| R1-A5 | round 1 | — | accepted@1 — KD-20's corrected consequence clause is true on the shipped tree | `go list -deps ./cmd/bot \| grep -c testcontainers` → 0 |
| R1-A6 | round 1 | — | accepted@1 — both new `at:` SHAs in `learnings.md` resolve; the third is the deliberately-preserved wrong value its successor entry corrects | `git cat-file -t 56857fb314b73e180f9c9132ccd4e0c488b8b994 b8951419d4848f807f1fe105cd152fdb537e4a55` |
| R1-A7 | round 1 | — | accepted@1 — the ratchet holds on a cold cache with headroom, so the recorded 89.44 is not a lucky replay | `go clean -testcache && make cover-ratchet` → 90.04% >= 89.44% |
| R2-1 | round 2 | nit | fixed@bcca2d6 | `grep -n 'testdb: %v' internal/testdb/testdb.go` against `grep -n 'testdb: starting' internal/testdb/server.go` |
| R2-2 | round 2 | nit | fixed@bcca2d6 | `grep -n 'os.Setenv' cmd/testpg/run.go` against `grep -n 't.Parallel' cmd/testpg/run_test.go` at the --up/--down tests |
| R2-A1 | round 2 | — | accepted@2 — R1-1's correction re-measured independently on the shipped tree: the DSN is in no cache key, so all four corrected documents are true | `go run ./cmd/testpg -- go test -count=1 ./internal/store/` then `LAB_GAME_TEST_DSN='postgres://labgame:labgame@127.0.0.1:1/labgame_test?sslmode=disable' go test ./internal/store/` → `(cached)` |
| R2-A2 | round 2 | — | accepted@2 — `internal/ingest`'s untouched 2 s / 5 s budgets: subtask 8's mandate was to CLASSIFY every wall-clock constant, and these were classified and recorded; the probe is green with `internal/ingest` at 4.194s under load | `grep -rn 'time.Sleep\|WithTimeout\|time.After' internal/ingest/*_test.go` |
| R2-A3 | round 2 | — | accepted@2 — `schemaMaxConns`'s `max_connections = 100` comment is still true of the path it describes; the design reserves the image default for the fallback explicitly | `grep -n -B 3 'schemaMaxConns = 4' internal/testdb/testdb.go` |
| R2-A4 | round 2 | — | accepted@2 — `TestBinaries_matchesTree` compares counts and prints the found package names, so it does fail by name; a name-set manifest would need a second constant | `grep -n -A 4 'if len(got) != Binaries' internal/testdb/server_test.go` |
| R2-A5 | round 2 | — | accepted@2 — AC9 re-derived on the shipped file list: every changed path is `**/*.go`, `Makefile`, `.githooks/**`, `ai-docs/**`, `AGENTS.md` or `.github/workflows/**`, all named in the filter | `git diff --name-only dbff3d8..HEAD` against `.github/workflows/ci.yml:38-79` |
| R2-A6 | round 2 | — | accepted@2 — R1-6's objection upheld: a nit with a specific, technically accurate reason; the "thirteen"/"three entry points" counts are loose (14 functions, `runChild` + `run --up` + `run --down`) but the ground holds | `grep -c '^func Test' cmd/testpg/run_test.go` → 14 |
| R2-A7 | round 2 | — | accepted@2 — R1-2, R1-4, R1-5, R1-7, R1-8 each re-verified fixed on the shipped tree; no container and no locator file left behind by any probe | see the round-2 § What was checked |
| R3-1 | round 3 | major | fixed@19baf8c | `make test-contention CONTENTION_PARALLEL=4 && grep -c '(cached)' tmp/test-contention-race.log` → must be **0**. Today it is 8–11 and `internal/scheduler` is among them, so the gate whose status is the probe's verdict reports green having executed none of the contended packages |
| R3-2 | round 3 | minor | fixed@19baf8c | `make test-contention CONTENTION_PARALLEL=4; grep -c 'SQLSTATE 57P0' tmp/test-contention-load.log` → must be **0**. Today the load log ends in a teardown-induced failure block, because `kill "$load_pid"` reaches the loop subshell and not the `go test` it is running |
| R3-A1 | round 3 | — | accepted@3 — R1-3 re-verified fixed on the shipped tree, both halves: the three values reach the scratch log, and the echo assertion is load-bearing (mutation-run, not assumed) | `grep -inE 'ceiling\|clients=\|parallel=' tmp/test-contention.log` → 2 hits; then `sed -i '150d' cmd/testpg/run.go && go test -count=1 ./cmd/testpg/` → RED at `run_test.go:149` |
| R3-A2 | round 3 | — | accepted@3 — R2-1 and R2-2 re-verified fixed | `grep -n 'testdb: %v' internal/testdb/testdb.go` → no hit; `grep -n 't.Parallel' cmd/testpg/run_test.go` → no hit at the `--up`/`--down` tests |
| R3-A3 | round 3 | — | accepted@3 — the shipped tree's own gate run: `make verify` GREEN and the ratchet holds with headroom | `make verify; make cover-ratchet` → 89.19% >= 89.44% (tolerance 0.60 pp) |
| R3-A4 | round 3 | — | accepted@3 — AC8, AC9, AC13, AC15, AC18 and the panic / domain-invariant sweeps re-derived on the shipped file list: no service block, every changed path class named in the change filter, no `done/` edit, no pipe in any new recipe, no production panic site, no committed host literal beyond stub DSNs and a deliberately unreachable one | `git diff --name-only dbff3d8..HEAD` against `.github/workflows/ci.yml:38-79` |
| R3-A5 | round 3 | — | accepted@3 — the `//nolint` reason at `cmd/testpg/run.go:316` naming `runChild`'s teardown is judged a same-package contract note, not a banned bare-name pointer: it states why the two teardowns are deliberately identical, and it is not the only thing carrying the reason | `make comment-refs` → 0, plus the review-judged read of the doc convention's two manual halves |
| R3-A6 | round 3 | — | accepted@3 — the contention retiming re-read whole: every widened value is an instrument (`contentionSafeConfig`, `waitLockFree`'s poll ceiling, the reconcile poll loop), the deadline suite's `shortDeadlineConfig` is untouched, and no asserted proposition changed | `git diff dbff3d8..HEAD -- internal/scheduler/` |
| R3-A7 | round 3 | — | accepted@3 — the three `at:` values in the new `learnings.md` entries re-resolved: two are real objects, the third is the deliberately-preserved wrong value its successor entry exists to correct | `git cat-file -t 56857fb314b73e180f9c9132ccd4e0c488b8b994 b8951419d4848f807f1fe105cd152fdb537e4a55` → commit, commit; `git cat-file -t f9834898…` → fatal |
| R4-1 | round 4 | major | fixed@33be21b | over a `cp` backup, prepend `./internal/nosuchpkg/...` to the load loop's package list at `Makefile:127`, then `make test-contention CONTENTION_PARALLEL=4; grep -c '^ok' tmp/test-contention-load.log` → must exceed 4, i.e. the loop survives its failing iteration and keeps loading. Today it is exactly 4 and the log ends `FAIL` |
| R4-2 | round 4 | minor | fixed@33be21b | `grep -n "test-contention's load loop carry it" ai-docs/go-test-conventions.md` → the bullet must also account for the foreground race gate's own `-count=1`, whose reason today lives only in `Makefile:113-119` |
| R4-3 | round 4 | minor | fixed@33be21b | `grep -rn '53300\|too many clients' --include='*.sh' --include='*.go' --include=Makefile --include='*.yml' .` → must find the scan the design says the target performs; today, outside `tmp/`, it finds nothing |
| R4-4 | round 4 | nit | fixed@33be21b | `grep -n 'nothing else can' AGENTS.md; grep -n 'the only thing that can' ai-docs/go-test-conventions.md` → both claims must be narrowed to "nothing removes it automatically" |
| R4-A1 | round 4 | — | accepted@4 — R3-1 and R3-2 re-verified fixed on the shipped tree: 0 `(cached)` in the race log, 0 `SQLSTATE 57P0` in the load log, the load log ends in a killed iteration rather than a teardown failure block, and no process or container survives the run | `make test-contention CONTENTION_PARALLEL=4; grep -c '(cached)' tmp/test-contention-race.log; grep -c 'SQLSTATE 57P0' tmp/test-contention-load.log` → 0 and 0 |
| R4-A2 | round 4 | — | accepted@4 — the new `set -m` comment's claim measured rather than read: the load-loop subshell is its own process-group leader and its `go test` sits in that group, so `kill -- -$load_pid` reaches it; and an interrupt of the target removes the container and leaves no stray process | `ps -eo pid,pgid,args` during a run → loop subshell pgid == its own pid, its `go test` in that pgid; then SIGINT to make's pgid → container terminated, no survivors |
| R4-A3 | round 4 | — | accepted@4 — the design was NOT amended for the foreground gate's `-count=1`, and that is not raisable here: it is R3-1's own subject, R3-1's verifying command passes, D7/D12 are silent rather than contradicted, and round 3 recorded the doc route as the owner's option. Recorded so round 5 does not mint a new id for it | `grep -n 'count=1' ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md` against `Makefile:130` |
| R4-A4 | round 4 | — | accepted@4 — AC1–AC5, AC7–AC9, AC13, AC16, AC17's local half and AC18 re-derived by command on the shipped tree, not read from this file | see the round-4 § What was checked |
| R4-A5 | round 4 | — | accepted@4 — R1-6's objection still upheld; the test file is unchanged since round 1 and the stated reason still holds | `git diff bcca2d6..HEAD -- cmd/testpg/run_test.go` → empty |
| R4-A6 | round 4 | — | accepted@4 — no Go file changed since round 3, so the panic-index sweep, the domain-invariant sweep and the D9 retiming read all stand as round 3 left them | `git diff --name-only bcca2d6..HEAD` → `Makefile` and this file only |
| R4-A7 | round 4 | — | accepted@4 — KD-20's ceiling arithmetic re-derived against the shipped binary's own echo, term by term: `clients×Binaries×parallel×(schemaMaxConns+1)+ceilingSlack` gives 2×4×4×5+32 = 192 and 1×4×16×5+32 = 352 | `make test-contention CONTENTION_PARALLEL=4` → `ceiling 192 (clients=2, parallel=4)`; `make test-db-up` → `capacity 352 admits the 352 needed (clients=1, parallel=16)` |
| R5-1 | round 5 | major | fixed@f70c827 | `grep -c 'go test .*-count=1' Makefile` → **3** (`:88`, `:138`, `:141`), against `ai-docs/context-status.md:208`'s "the two that do are the only ones that need it"; the row closes when `grep -c 'the two that do are the only ones that need it' ai-docs/context-status.md` → 0 and the replacement sentence names all three plus the run-conditions reason the two-case partition cannot express |
| R5-2 | round 5 | minor | fixed@f70c827 | `grep -n 'INSTRUMENT FAILURE\|exit 2' Makefile` finds the scan the target now performs, against `grep -n 'scan both logs' ai-docs/go-test-conventions.md` → the bullet must say the target does it and that exit 2 means the run is discarded, not that the race gate failed |
| R5-3 | round 5 | nit | fixed@f70c827 | `grep -n 'provisionErr\|stopErr' cmd/testpg/run_test.go` → today only the declaration (`:20`, `:24`) and the stub's read (`:38-39`, `:47`); no test assigns either, so `cmd/testpg/run.go:143-145` and `:152-155` are unexercised. Closes when a test sets one and the branch is asserted |
| R5-4 | round 5 | nit | fixed@f70c827 | `grep -qE 'zzz' tmp/test-contention-race.log /nonexistent; echo $?` → **2**, the same else-branch `Makefile:145-151` treats as "exhaustion scan clean". Closes when an error status is distinguished from a clean scan |
| R5-A1 | round 5 | — | accepted@5 — R4-1 re-proved by the same injection that proved the defect: with `./internal/nosuchpkg/...` prepended to the load list over a `cp` backup, the load log carries **5** `^ok` lines across two iterations plus three failure blocks, i.e. the loop survives its failing iteration. R4-2, R4-3 and R4-4 re-verified by their own commands | injection re-run, then `grep -c '^ok' tmp/test-contention-load.log` → 5 (> 4); `grep -n "test-contention's load loop carry it" ai-docs/go-test-conventions.md` → no hit, the bullet now names both children; `grep -rn '53300' --include=Makefile .` → `Makefile:145`; `grep -n 'nothing else can' AGENTS.md; grep -n 'the only thing that can' ai-docs/go-test-conventions.md` → both empty |
| R5-A2 | round 5 | — | accepted@5 — the shipped tree's own gates: `make verify` GREEN (24 `ok`, 0 `FAIL`), `make cover-ratchet` **89.19% holds against 89.44%**, `make test-contention CONTENTION_PARALLEL=4` GREEN with 0 `(cached)`, 0 `SQLSTATE 57P0`, the exhaustion scan clean, and no surviving container or process | `make verify; make cover-ratchet; make test-contention CONTENTION_PARALLEL=4` |
| R5-A3 | round 5 | — | accepted@5 — R4-A3 upheld and NOT re-raised: the design's Decomposition row 5 still names `-count=1` on the load run only, and that remains R3-1's own subject with D7/D12 silent rather than contradicted. R5-1 is a different artefact class — a live document with a counted claim — and takes the ordinary-edit route, not the Design Amendment recipe | `grep -n 'count=1 on the load run' ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md` against `Makefile:141` |
| R5-A4 | round 5 | — | accepted@5 — R1-6's objection still upheld; `cmd/testpg/run_test.go` is byte-identical to the round-1 fix | `git diff bcca2d6..HEAD -- cmd/testpg/run_test.go` → empty |
| R5-A5 | round 5 | — | accepted@5 — no Go file has changed since round 3, so the panic-index sweep, the domain-invariant sweep and the D9 retiming read stand as rounds 3 and 4 left them; the only code change since round 4 is none at all (the tree is byte-identical to 33be21b) | `git diff --name-only 33be21b..HEAD` → this file only |
| R5-A6 | round 5 | — | accepted@5 — `CONTENTION_PARALLEL`'s `nproc` default refuses above 24 cores (`2×4×p×5+32 > 1000`), and that is the design's own stated residue with its mitigation, not a defect: `Ceiling` names the flags that lower it. Measured here at `nproc` = 16 → 672, inside `ceilingMax` | `nproc` → 16; design § Risks and the `ceilingMax` row |
| R5-A7 | round 5 | — | accepted@5 — the wrapper's decision-order tests do reach the clauses they name rather than passing vacuously: every "seam untouched" case also asserts the DSN the child actually saw, so an early return would fail the second assertion | `grep -n 'child saw DSN' cmd/testpg/run_test.go` → the caller-DSN and locator-admits cases |

## Files touched

- `internal/testdb/server.go` (new) - exported provisioning API, `Ceiling` formula, capacity `Probe`, `Binaries` manifest constant
- `internal/testdb/server_test.go` (new) - ceiling table tests, refusal path, probe against a live server, binaries manifest
- `internal/testdb/testdb.go` - `Main`'s container branch re-expressed over `StartServer`; `dsnEnv` exported as `DSNEnv`
- `cmd/testpg/main.go`, `cmd/testpg/run.go`, `cmd/testpg/locator.go` (new) - the wrapper: decision order, `--up`/`--down`, signal-aware teardown, exit-status passthrough, injected seam
- `cmd/testpg/run_test.go` (new) - decision-order coverage over a stub seam; starts no container
- `Makefile` - `test`/`test-race` routed through the wrapper; `test-db-up`, `test-db-down`, `test-fallback`, `test-contention` added
- `.githooks/coverage-ratchet.sh` - measurement routed through the wrapper, after the skip decision; runtime advice updated
- `.github/workflows/ci.yml` - `test-fallback` step added to the Test job
- `ai-docs/coverage-ratchet.txt` - re-centred per commit, net 91.66 -> 89.44
- `internal/scheduler/*_test.go` (deadline, failure, observe, reconcile, schedule, worker) - D9 instrument/subject retiming for cross-package contention
- `ai-docs/context.md` - the layout paragraph's `internal/testdb` clause: provisioning for the whole suite, exported as an API `cmd/testpg` imports as well as the `_test.go` files
- `AGENTS.md` - § *Build & Test*: the six new targets plus the routing callout, the ratchet table's container-runtime row, and the cache-replay sentence made conditional on a stable DSN
- `ai-docs/go-test-conventions.md` - § *Postgres is tested against Postgres*: the shared path, the long-lived pair, the fallback's own gate, D9's contention rule and the D12 probe
- `ai-docs/key-decisions.md` - KD-20's amendment clause: the decision order, the bring-up/take-down pair, `Ceiling`'s formula term by term with each term's value, the floor/refusal asymmetry, and the two stated residues; the *Consequence* clause corrected to the import-graph invariant

## Self-Review (Round 1)

**Verdict:** REJECT

**What was checked.** `AGENTS.md`; the spec's `## Acceptance Criteria` (AC1–AC18); the design's
Approach, D1–D12, Decomposition, Handoff plan, Risks and Test Design. Diff `dbff3d8..HEAD`
(27 files). Re-run against the shipped tree, not accepted from the progress file:
`go vet ./...` (0), `golangci-lint run` (0 issues), `make comment-refs` (0),
`make file-limits` (0), `make shellcheck` (0), `make actionlint` (0),
`bash ai-docs/scripts/check-script-shape.sh` (0), `make test` (0),
`make test-fallback` (0 — the four database-backed binaries take 8.9–13.5 s each, the
per-binary container path observed rather than inferred: AC3 PASS),
`make cover-ratchet` on a **cold** test cache (90.04% >= 89.44%, AC-adjacent),
`make test-contention CONTENTION_PARALLEL=2` (0, exhaustion scan clean on both logs: AC10 PASS),
a real `make test-db-up` / `make test-db-down` round trip (AC4 clause), and the AC11
demonstration's missing control (the 10 ms instrument is GREEN with no load, so the recorded
RED was contention: AC11 PASS). AC8, AC9, AC13, AC15, AC18 read in the diff; AC7 read at
`.githooks/coverage-ratchet.sh:86-113` (the staged-file skip precedes the wrapper invocation).
Working tree restored and verified clean after every mutation probe.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R1-1 | `AGENTS.md:113-119`, `ai-docs/go-test-conventions.md:44`, `ai-docs/context-status.md:207`, design `:382` | major | **The test-cache claim this diff writes into three live documents is false, and in `AGENTS.md` it replaced a true sentence.** All four say the DSN is in the `go test` cache key, so the database-backed packages are "a cache miss on every run" under the wrapper's anonymous container. Measured: `go clean -testcache`, then three wrapper runs printing their own DSN — ports 36255, 35543, 42761, all distinct — gave `internal/store 1.039s`, then `(cached)`, then `(cached)`. Same result for `internal/scheduler`. So the wrapper's own path replays cached results exactly as before, `make test` / `make test-race` / the ratchet measurement can return green having run none of the database-backed packages, and D8's `-count=1` reasoning applies to them too — the design's ground for exempting them is this falsified premise. Violates AC14 (a live document must agree with shipped behaviour). The design half is a **Design Amendment trigger** — design doc `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md:379-386` (D7) contradicts the implementation; recipe at `.claude/skills/task/SKILL.md` Step 11 fail-loud table. Whether any target gains `-count=1` is a scope decision for the owner, not part of this fix. | ✅ Fixed (design amended) |
| R1-2 | `cmd/testpg/run.go:240-256` | major | **`--up` reports a ceiling the reused container does not have, and the documented remedy for D3a's stated residue therefore does not work.** `runUp` passes `ConnCeiling` alongside `ContainerName`, but `StartServer` reuses an existing container by name and the new ceiling is silently ignored. Measured: `make test-db-up CLIENTS=1` → "shared server up, ceiling 352", `show max_connections` = 352; then `make test-db-up CLIENTS=2` → "shared server up, ceiling 672" on the **same** container, `show max_connections` still **352**. Two `make test` runs each compute a need of 352, each is admitted by the 352-slot server, and the combined population is 704 — the exhaustion this task exists to remove. Contradicts design D4 (`:292`, "**It sizes with the same terms a run does**") and the remedy KD-20 now ships ("the answer is to size it (`make test-db-up CLIENTS=2`)"). | ✅ Fixed |
| R1-3 | `cmd/testpg/run.go:136-146` | major | **The granted ceiling is never echoed on the path that grants it.** Only `runUp` prints a ceiling (`:255`); `runChild`'s provisioning branch prints nothing, and `Makefile:105` echoes only `clients=N parallel=N`. Measured: after a full `make test-contention CONTENTION_PARALLEL=2`, `grep -in ceiling tmp/sr-contention.log tmp/test-contention-race.log` returns **no output**. Design D12 requires "The target echoes the granted ceiling, the client count and the pinned value into the log, so the arithmetic a run relied on is readable after the fact", and Decomposition row 3 (design `:581`) lists "the granted ceiling echoed to stderr" as a deliverable of subtask 3. Neither is met, and the design was not amended to say so. | ✅ Fixed |
| R1-4 | `AGENTS.md:100`, `.githooks/coverage-ratchet.sh:116-119` | minor | The ratchet's runtime-failure advice now leads with a command that cannot work in the case the same sentence names. `AGENTS.md:100` reads "Container runtime missing? `make test-db-up` brings up a long-lived one" — but that target reaches the runtime through the same socket. Measured: `DOCKER_HOST=unix:///nonexistent/podman.sock make test-db-up` → "could not start the shared server: … dial unix /nonexistent/podman.sock: connect: no such file or directory", exit 2. The correct answer (`LAB_GAME_TEST_DSN`) survives in the same cell but is now second. The script's wording ("container-runtime failure") is weaker but has the same ordering. | ✅ Fixed |
| R1-5 | design `:793-794` | minor | The AC15 read recipe asserts "every gate this change adds or edits redirects to a file under `tmp/` and is read from there". Only the ratchet measurement and `test-contention`'s two children do; `test`, `test-race`, `test-fallback`, `test-db-up` and `test-db-down` write to the terminal. AC15's substance holds — no load-bearing status crosses a pipe — so this is the recipe's claim, not the tree's behaviour. Same Design Amendment route as R1-1 if the sentence is to be corrected. | ✅ Fixed (design amended) |
| R1-6 | `cmd/testpg/run_test.go:81-336` | nit | Thirteen top-level `func Test…` over one `stubSeam`, where the case set is a decision-order matrix. The workspace default for more than two cases is a table-driven `t.Run` set; the design's own Test Design enumerates them as scenarios of one entry point. Behaviour-describing names are already there, so this is shape only. | ⚠️ Objected: the thirteen cases span three entry points and assert structurally different things (seam untouched / stop ran / ceiling arithmetic / usage error), so a table would need a per-case closure field, which separate test functions already are |
| R1-7 | `internal/scheduler/reconcile_test.go:361` | nit | The widened instrument is now waited out in full: `Run` is asserted to return `context.DeadlineExceeded`, so the subtest blocks for the whole 5 s instead of the previous 0.5 s. D9 names polling-with-a-generous-ceiling as the better of its two routes; the generous-constant route was taken, adding ~4.5 s of pure waiting to every scheduler run in a task whose purpose is suite wall clock. | ✅ Fixed |
| R1-8 | `cmd/testpg/run.go:263-296` | nit | `runDown` does not disable the reaper the way `runUp` does, so removing the long-lived server starts a Ryuk container to supervise nothing. Observed: `podman ps -a` immediately after `make test-db-down` lists `reaper_cd270f93…  Up 2 seconds`. It self-terminates, so this is cosmetic. | ✅ Fixed |

**Recorded, not raised** — each is a register row (`accepted@1`) rather than a finding, so
round 2 is scoped by it: the AC11 demonstration's own control (R1-A1), the panic-index
sweep (R1-A2), the domain-invariant sweep (R1-A3), AC9's change-filter comparison (R1-A4),
KD-20's corrected import-graph consequence (R1-A5), the two new `learnings.md` `at:` SHAs
(R1-A6), and the cold-cache ratchet draw (R1-A7).

## Self-Review (Round 2)

**Verdict:** REJECT

**What was checked.** `AGENTS.md`; the spec's `## Acceptance Criteria` (AC1–AC18); the design's
Approach, D1–D12, Decomposition, Handoff plan, Risks, Test Design and Open questions. Diff
`dbff3d8..HEAD` (27 files, 40 commits). Round-2 scope taken from the `## Review register` per
instruction 7a. The spawn prompt carried only the five permitted lines — no contamination.

Re-run against the shipped tree, never accepted from the progress file: `go vet ./...` (0),
`golangci-lint run` (0 issues), `golangci-lint fmt -d` (clean), `make comment-refs` (0),
`make file-limits` (0), `make shellcheck` (0), `make actionlint` (0), `make tidy-check` (0),
`bash ai-docs/scripts/check-script-shape.sh` (0), **`make verify` GREEN**,
`make cover-ratchet` → **89.88% >= 89.44%** with the recorded mark unchanged, and
`make test-contention CONTENTION_PARALLEL=2` → **GREEN**, every package `ok`, exhaustion scan
(`sorry, too many clients already` / `SQLSTATE 53300`) clean on both captured logs — AC10 PASS.

Per-row re-verification of the round-1 register:

- **R1-1 (fixed@f4e6e75) — confirmed, and independently re-measured rather than read.**
  Mechanism read out of the toolchain: `$GOROOT/src/testing/testing.go:2434` (`m.Run` calls
  `m.before()`) and `:2674` (`m.before()` calls `m.deps.StartTestLog`), against
  `internal/testdb/testdb.go:46` where `Main` reads the variable above its `m.Run()`.
  Behaviour: `go run ./cmd/testpg -- go test -count=1 ./internal/store/` → `1.122s` on an
  ephemeral-port server, then the same package under a **different, unreachable** DSN →
  `(cached)`. The corrected sentences in `AGENTS.md`, `ai-docs/go-test-conventions.md`,
  `ai-docs/context-status.md` and design D7 are true.
- **R1-2 (fixed@a11a5fa) — confirmed.** `make test-db-up CLIENTS=1` → `capacity 352 admits the
  352 needed`, `show max_connections` = 352; `make test-db-up CLIENTS=2` → exit 1,
  `the shared server's capacity is 352, below the 672 needed for clients=2 parallel=16 … take it
  down and bring it up again to resize`. `ai-docs/go-test-conventions.md:44` documents that
  remedy, so KD-20's shorter "size it" clause is not left standing alone.
- **R1-3 (fixed@a11a5fa) — RE-OPENED.** See the table below.
- **R1-4 (fixed@a11a5fa) — confirmed.** `AGENTS.md:100` and `.githooks/coverage-ratchet.sh:119-121`
  now lead with `LAB_GAME_TEST_DSN` and demote `make test-db-up` to the slow-suite answer.
- **R1-5 (fixed@f4e6e75) — confirmed.** The AC15 recipe now says redirecting to `tmp/` is what
  the two multi-child gates do, not a property of every target.
- **R1-6 — objection upheld** (nit, specific and technically accurate reason; register R2-A6).
- **R1-7 (fixed@a11a5fa) — confirmed.** `internal/scheduler/reconcile_test.go:356-388` now polls
  the row count on a 10 ms tick under a 5 s ceiling and cancels as soon as the row appears, so the
  widened instrument is no longer paid in wall clock.
- **R1-8 (fixed@a11a5fa) — confirmed by observation.** `podman ps -a` immediately after
  `make test-db-down` lists only the machine's pre-existing unrelated container: no reaper.

Also read/derived in this round: AC9's change-filter comparison on the shipped file list (R2-A5);
AC18 swept over the whole non-plan diff — the only host literals are `postgres://stub/…` fixtures
and a deliberately unreachable `127.0.0.1:1` (`internal/testdb/server_test.go:113`); the
panic-index sweep (`grep` over every changed non-test Go file → no hit, so no row is owed); the
domain-invariant sweep (no balance moves, no mechanic, no schema or enum touched); the progress
file's required re-entry fields (`current_step`, `last_passed_gate`, `entry_args` present;
`parent_skill` correctly omitted — conditional per the canonical template). Every mutation probe
was taken over a `cp` backup and the working tree confirmed clean (`git status --short` empty)
afterwards; no container and no locator file was left behind.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R1-3 | `Makefile:102-114`, design `:563-565` | major | **RE-OPENED. D12 requires the target to echo the granted ceiling, the client count and the pinned parallelism *into the log*; none of the three reaches either scratch log, and the round-1 fix that added the echo is untested.** Measured on the shipped tree: `make test-contention CONTENTION_PARALLEL=2` → GREEN, then `grep -inE 'ceiling|clients=|parallel=' tmp/test-contention-race.log tmp/test-contention-load.log` → **no output, exit 1**. The wrapper's new line (`cmd/testpg/run.go:150`) and the recipe's own `echo` (`Makefile:112`) both write to the terminal stream; the two files the same D12 paragraph names as "both logging to files under the scratch directory" carry none of it, so a red run's arithmetic is unreadable from the run's own artefacts — which is the sentence's stated purpose. Decomposition row 3's narrower "the granted ceiling echoed to stderr" **is** met: `testpg: started a server, ceiling 112 (clients=2, parallel=2)` appears in the make-output capture. Second half, mutation-confirmed: deleting `run.go:150` leaves `go test -count=1 ./cmd/testpg/` **fully green**, so the deliverable ships with zero coverage. Two exits, and the choice is the orchestrator's: change the recipe so the three values land in a file under `tmp/` (plus a test over the echo), or treat it as a **Design Amendment trigger** — design doc `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md:563-565` (D12) contradicts the implementation; recipe at `.claude/skills/task/SKILL.md` Step 11 fail-loud table. If the doc route is taken, spawn the `design-writer` Subagent to amend it — never an orchestrator `Edit`. | ✅ Fixed |
| R2-1 | `internal/testdb/testdb.go:48` | nit | `Main` prints `"testdb: %v"` over an error `StartServer` has already prefixed (`internal/testdb/server.go:132`, `"testdb: starting %s: %w"`), so a runtime failure now reads `testdb: testdb: starting docker.io/library/postgres:18: …`. The stutter is new in this diff — the pre-change line was `"testdb: starting %s: %v"` — and design D3a cites exactly this message as the one that names its cause. | ✅ Fixed |
| R2-2 | `cmd/testpg/run.go:239`, `:306` | nit | `runUp` and `runDown` mutate the process environment with `os.Setenv("TESTCONTAINERS_RYUK_DISABLED", …)`, and `cmd/testpg/run_test.go:269` and `:301` reach both from `t.Parallel()` tests. Harmless today — nothing in this package reads that variable and `-race` is clean — but it is precisely the mutation `t.Setenv` refuses in a parallel test, and it leaks into every later test in the binary. | ✅ Fixed |

**Recorded, not raised** — each is an `accepted@2` register row rather than a finding, so round 3
is scoped by it: R1-1's independent re-measurement (R2-A1), `internal/ingest`'s classified-but-
unchanged budgets (R2-A2), `schemaMaxConns`'s image-default comment (R2-A3),
`TestBinaries_matchesTree`'s count-plus-names shape (R2-A4), AC9's re-derivation (R2-A5), R1-6's
upheld objection (R2-A6), and the five round-1 fixes re-verified green (R2-A7).

## Self-Review (Round 3)

**Verdict:** REJECT

**What was checked.** `AGENTS.md`; the spec's `## Acceptance Criteria` (AC1–AC18); the design's
Approach, D1–D12, Decomposition, Handoff plan, Risks, Test Design and Open questions. Diff
`dbff3d8..HEAD` (27 files, 42 commits); the code tree is byte-identical to `bcca2d6`, the only
later commit being this file's own register close. Round-3 scope taken from the `## Review
register` per instruction 7a. The spawn prompt carried only the five permitted lines — no
contamination.

Re-run against the shipped tree, never accepted from the progress file: `go vet ./...` (0),
`golangci-lint run` (0 issues), `golangci-lint fmt -d` (clean), **`make verify` GREEN**,
`make cover-ratchet` → **89.19% >= 89.44%** (tolerance 0.60 pp), and
`make test-contention CONTENTION_PARALLEL=4` **twice** — which is where the round's finding is.

Per-row re-verification of the round-2 register:

- **R1-3 (fixed@bcca2d6) — confirmed, both halves.** `grep -inE 'ceiling|clients=|parallel='
  tmp/test-contention.log` returns `testpg: started a server, ceiling 192 (clients=2,
  parallel=4)` and `test-contention: clients=2 parallel=4`, so the arithmetic now outlives the
  scrollback. Coverage half mutation-run rather than read: deleting `cmd/testpg/run.go:150`
  turns `go test -count=1 ./cmd/testpg/` RED at `run_test.go:149` (`stderr = "", want the
  granted ceiling 100 echoed`). Working tree restored and confirmed clean.
- **R2-1 (fixed@bcca2d6) — confirmed.** `internal/testdb/testdb.go:56` prints `"%v\n"`; the
  provisioner's own prefix is no longer doubled.
- **R2-2 (fixed@bcca2d6) — confirmed.** Neither `--up`/`--down` test calls `t.Parallel()` any
  more, and the reason is written where the two of them sit.
- **R1-6 — objection still upheld** (register R2-A6); nothing has changed since.

Also re-derived in this round: AC8 (no `services:` block; the Test job's four run steps are
`make test` / `test-race` / `cover-ratchet` / `test-fallback`); AC9 on the shipped file list
against the filter block; AC13 (KD-20 carries the arithmetic term by term; no `ai-docs/plans/done/**`
file is touched); AC15 (no new recipe pipes a load-bearing status — the `|` characters in the
contention recipe are all `||`); AC18 (only stub DSNs and a deliberately unreachable
`127.0.0.1:1`); the panic-index sweep over every changed non-test Go file (no hit, so no row is
owed); the domain-invariant sweep (no balance moves, no mechanic, no schema or enum touched);
the D9 retiming read whole; the three `at:` values in the new `learnings.md` entries; and the
progress file's required re-entry fields (`current_step`, `last_passed_gate`, `entry_args`
present, `parent_skill` correctly omitted). Every mutation probe was taken over a `cp` backup
and `git status --short` confirmed empty afterwards; no container and no locator file was left
behind.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R3-1 | `Makefile:115` | major | **`make test-contention`'s foreground race gate carries no `-count=1`, so the gate whose exit status IS the probe's verdict is served from the test cache — and on this tree it is. AC10's recorded PASS is a cached green.** Measured twice on the shipped tree: `make test-contention CONTENTION_PARALLEL=4` → GREEN in 11.7 s, and `tmp/test-contention-race.log` reads `ok … internal/scheduler (cached)` and `ok … internal/ingest (cached)` — 8 of 12 packages cached in one run, 9 of 12 in the other, `internal/scheduler` (the package the recorded trap names and the whole contention subtask exists for) cached in both. It was not executed at all, under load or otherwise. Counterfactual over a `cp` backup of the Makefile: adding `-count=1` to that one command makes the same target at the same parallelism run `internal/scheduler 5.755s`, `internal/store 6.207s`, `internal/ingest 4.146s` against the same background load, GREEN — so the tree is sound and the instrument was not. **This does not re-litigate the owner's "no ordinary gate gains `-count=1`".** D7 records that decision with its ground — "`make test`, `make test-race` and the ratchet measurement keep replaying: *a cached pass is a real pass from an identical earlier run*" — and that ground holds for a gate asserting *the code passes* and fails for this one, which asserts *the code passes under induced cross-package contention*: a property of the run's conditions, which the cache key cannot see, by R1-1's own measurement that the provisioning regime enters no cache key at all. D12 already wrote this argument for the target's **other** child (design `:534-541`, "**The load run carries `-count=1`**, and that is not decoration … an instrument that cannot load anything") and stopped one child short; it is the same defect class as AC3's, found at Step 9 and fixed in 1a3ee3a. Consequence: AC10 is recorded PASS on a run that may have executed none of the contended packages, and AC11's green half is in the same position (its RED half is sound — the reverted instrument changed the file, so that run missed the cache by construction). Exit: add `-count=1` to `Makefile:115` — that contradicts nothing in D12, it applies the paragraph D12 already wrote for the sibling child — then re-run and re-record AC10 and AC11's green with `grep -c '(cached)' tmp/test-contention-race.log` = 0. If the owner would rather the design state the reasoning explicitly, that is a **Design Amendment trigger** — design doc `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md:532-541` (D12) is silent where it should be explicit; recipe at `.claude/skills/task/SKILL.md` Step 11 fail-loud table, and then spawn the `design-writer` Subagent to amend it, never an orchestrator `Edit`. | ✅ Fixed |
| R3-2 | `Makefile:116-117` | minor | **`kill "$load_pid"` reaches the loop subshell, not the `go test` the loop is running, so the load run is not killed — it is orphaned and runs on into the wrapper's container teardown.** D12 (design `:532`) says the recipe kills the load run afterwards; it kills the loop driver. Mechanism confirmed directly rather than inferred: `( while true; do sleep 30; done ) & kill $!; wait` leaves the `sleep` alive. Observed effect on every run: `tmp/test-contention-load.log` ends in a block of teardown-induced failures — 43 `FAIL` lines in the run just taken, `FATAL: the database system is shutting down (SQLSTATE 57P03)` and `terminating connection due to administrator command (SQLSTATE 57P01)` — so the log D12 requires to be scanned before any red is classified always ends in failures a reader has to learn to discount, and the target returns while a `go test` is still holding connections. Not an AC break: the foreground's status is the verdict and the exhaustion literals D12 scans for are absent. Exit: start the loop under job control and kill its process group, or have the loop trap the signal and forward it to the child. | ✅ Fixed |

**Recorded, not raised** — each is an `accepted@3` register row rather than a finding, so round 4
is scoped by it: R1-3's two-half re-verification (R3-A1), R2-1 and R2-2 re-verified (R3-A2), the
shipped tree's own `make verify` / ratchet run (R3-A3), the AC8 / AC9 / AC13 / AC15 / AC18 and
panic / domain sweeps (R3-A4), the `runChild`-teardown `//nolint` reason judged a same-package
contract note rather than a banned bare-name pointer (R3-A5), the D9 retiming read whole
(R3-A6), and the three `learnings.md` `at:` values (R3-A7).

## Self-Review (Round 4)

**Verdict:** REJECT

**What was checked.** `AGENTS.md`; the spec's `## Acceptance Criteria` (AC1–AC18); the design's
Approach, D1–D12, Decomposition, Handoff plan, Risks, Test Design and Open questions. Diff
`dbff3d8..HEAD` (27 files, 44 commits); the only code change since round 3 is `Makefile`'s
`test-contention` target. Round-4 scope taken from the `## Review register` per instruction 7a.
The spawn prompt carried only the five permitted lines — no contamination.

Re-run against the shipped tree, never accepted from this file: **`make verify` GREEN** (which
carries `golangci-lint fmt -d`, `go build`, `go vet`, `golangci-lint run`, `make file-limits`,
`make test`, `make test-race`, `make tidy-check`, `make actionlint`, `make shellcheck`,
`make comment-refs`), `bash ai-docs/scripts/check-script-shape.sh` (conforms),
`make cover-ratchet` → **89.19% holds against 89.44%** (tolerance 0.60 pp), and
`make test-contention CONTENTION_PARALLEL=4` → **GREEN**, exhaustion scan clean on both logs.

Per-row re-verification of the round-3 register:

- **R3-1 (fixed@19baf8c) — confirmed.** `grep -c '(cached)' tmp/test-contention-race.log` → **0**,
  and the race log carries real timings for every contended package
  (`internal/scheduler 6.144s`, `internal/store 5.813s`, `internal/ingest 4.130s`). The gate
  whose status is the probe's verdict now executes.
- **R3-2 (fixed@19baf8c) — confirmed, by observation as well as by grep.**
  `grep -c 'SQLSTATE 57P0' tmp/test-contention-load.log` → **0**; the load log is 5 lines and
  ends mid-iteration (`internal/ingest 0.862s`) instead of the previous 43-line teardown failure
  block, it did not grow after the target returned, and `ps` finds no surviving `go test`.
  The mechanism the new comment claims was measured, not read: during a run the load-loop
  subshell is its own process-group leader (pid 39963, pgid 39963) with its `go test` in that
  group (pid 39965, pgid 39963), so `kill -- -$load_pid` reaches the child.
- **R1-6 — objection still upheld** (register R4-A5); `cmd/testpg/run_test.go` is unchanged.

AC recipes re-run rather than read: **AC1** — `make verify`'s two gate invocations created
exactly two postgres containers, one per invocation, not one per binary; **AC2** — a
caller-supplied DSN reached the child verbatim with the container count unchanged and the
shortfall reported; **AC3** — `make test-fallback` GREEN, and the per-binary provisioning
*observed* rather than inferred: polling the runtime across a bare run showed **5** concurrent
postgres containers (4 per-binary plus the machine's pre-existing one), with each database-backed
binary at 8.9–11.6 s against ~1 s under the wrapper; **AC4** — a real `make test-db-up` /
`make test-db-down` round trip created and removed both the container and the locator file and
left the pre-existing container alone, and an interrupted `make test-contention` terminated its
container and left no stray process; **AC5** — the arithmetic re-derived term by term against the
binary's own echo (192 and 352, register R4-A7); **AC7** — read at
`.githooks/coverage-ratchet.sh:86-113`, where the raise-mode staged check `exit 0`s above the
`mkdir -p tmp` and the wrapper invocation; **AC8** — no `services:` block, the Test job's four run
steps are `make test` / `test-race` / `cover-ratchet` / `test-fallback`; **AC9** — the shipped
27-path file list against the filter block at `.github/workflows/ci.yml:38-82`, every class named;
**AC13** — KD-20 carries the arithmetic and no `ai-docs/plans/done/**` path is touched; **AC16** —
`make shellcheck`, the script-shape gate and `make comment-refs` all green; **AC18** — the
non-plan diff's only host literals are `postgres://stub/…` fixtures, a `postgres://…` ellipsis in
advice text and a deliberately unreachable `127.0.0.1:1`. The progress file's required re-entry
fields are present (`current_step`, `last_passed_gate`, `entry_args`; `parent_skill` correctly
omitted). Every mutation probe was taken over a `cp` backup and `git status --short` confirmed
empty afterwards; no container and no locator file was left behind.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R4-1 | `Makefile:125`, `:127` | major | **The load generator stops at its first FAILING iteration, silently, and the verdict does not notice.** The child runs under `set -eu -o pipefail` (`:125`) and the load loop is a subshell (`:127`), which inherits `-e`; a `go test` in a `while true` body sits in no `-e`-exempt context, so one failing iteration ends the loop for the rest of the run, and `wait "$load_pid" … \|\| true` (`:132`) discards the status while only the foreground's is the verdict. Measured twice. Shell semantics, minimal repro: `bash -c 'set -eu -o pipefail; set -m; ( n=0; while true; do n=$((n+1)); echo "iteration $n"; if [ $n -eq 2 ]; then false; fi; done ) >tmp/… 2>&1 & wait $!'` → log stops at iteration 2, `wait` returns 1. Real recipe, over a `cp` backup with one unresolvable package prepended to `:127`'s list: `make test-contention CONTENTION_PARALLEL=4` → **exit 0, GREEN**, `tmp/test-contention-load.log` holds exactly one iteration then `FAIL`, and the only log the target prints says nothing about it — so the foreground's last seconds ran with no load at all. **Not R3-1 re-opened:** that row's mechanism is the test cache and its verifying command passes (0 `(cached)`); this one is the shell's `-e`. It defeats the sentence D12 spends a paragraph on — `-count=1` exists so the loop keeps loading past its FIRST iteration (design `:534-541`, "an instrument that cannot load anything") — by ending it at the first failing one, and the trigger correlates with the condition probed: a load-run test failing under contention is the signal the probe exists to raise, and it silences the generator instead. Today's AC10 green is **not** falsified — the load log ends in a killed iteration, not a failure — so this is the instrument's soundness for the next run, not a false record now. Exit: let the loop survive a failing iteration (`\|\| true` on its `go test`, or `set +e` inside the subshell) so load runs for the whole foreground, and consider surfacing the load run's own failures rather than discarding them. Contradicts nothing in D12 — no design amendment needed. | ✅ Fixed |
| R4-2 | `ai-docs/go-test-conventions.md:45` | minor | The live rule that decides which gates defeat the test cache was not updated when the race gate gained `-count=1` in round 3. The bullet partitions gates in two — "a gate that exists to exercise a particular provisioning path must pass `-count=1` … which is exactly why `make test-fallback` and `test-contention`'s load loop carry it", and "the ordinary gates keep replaying as they always did" — and the shipped tree has a third case fitting neither: `test-contention`'s foreground race gate, which carries the flag because it asserts a property of the run's *conditions* that the cache key cannot see. That reason exists only in `Makefile:113-119`'s comment, so a reader classifying a NEW gate by this bullet reaches exactly the answer that produced R3-1 — the third instance of the class in this task. Nothing here is strictly false, which is why it is `minor`; the fix is one clause in the same bullet, and `:48`'s description of the target wants the same word. Cites AC14 and `AGENTS.md` § *Propagation Rule* step 4. | ✅ Fixed |
| R4-3 | design `:571` | minor | D12 states "the target greps both captured logs for `sorry, too many clients already` and for `SQLSTATE 53300`". The shipped target contains no grep: `grep -rn '53300\|too many clients'` over every `*.go` / `*.sh` / `*.yml` / `Makefile` outside `tmp/` returns nothing. The scan is an operator step on every run recorded in this file, and the design's own Decomposition row 5 (`:605`) lists the target's contents without it — the design contradicts itself and the implementation followed the row. AC10/AC11 are unaffected and the live successor is correct: `ai-docs/go-test-conventions.md:48` tells the *reader* to scan, which is what happens. **Design Amendment trigger** — design doc `ai-docs/plans/2026-09-08-shared-postgres-test-server.design.md:571` contradicts the implementation; recipe at `.claude/skills/task/SKILL.md` Step 11 fail-loud table, and if that route is taken, spawn the `design-writer` Subagent to amend it, never an orchestrator `Edit`. The other exit is to make the sentence true by adding the scan to the target. | ✅ Fixed |
| R4-4 | `AGENTS.md:49`, `ai-docs/go-test-conventions.md:44` | nit | "remove it — nothing else can" and "it is the only thing that can" are false as written: `podman rm -f lab-game-test-postgres` removes it, as does any runtime CLI. The reason both sentences give — the reaper is disabled at creation — establishes the narrower true claim: nothing removes it *automatically*, which is what lets it outlive the process that made it. Worth the word because the surrounding prose is otherwise measured. | ✅ Fixed |

**Recorded, not raised** — each is an `accepted@4` register row rather than a finding, so round 5
is scoped by it: R3-1 and R3-2 re-verified fixed (R4-A1), the `set -m` process-group claim
measured and the interrupt path swept (R4-A2), the un-amended design half of R3-1 ruled
not-raisable with its reason (R4-A3), the AC recipes re-derived by command (R4-A4), R1-6's
objection still upheld (R4-A5), the panic and domain-invariant sweeps standing as round 3 left
them because no Go file changed (R4-A6), and KD-20's ceiling arithmetic re-derived against the
binary's own echo (R4-A7).

## Self-Review (Round 5)

**Verdict:** REJECT

**What was checked.** `AGENTS.md`; the spec's `## Acceptance Criteria` (AC1–AC18); the design's
Approach, D1–D12, Decomposition, Handoff plan, Risks, Test Design and Open questions. Diff
`dbff3d8..HEAD` (27 files, 45 commits); `git diff --name-only 33be21b..HEAD` is this file alone,
so the code tree is byte-identical to the round-4 fix. Round-5 scope taken from the
`## Review register` per instruction 7a. The spawn prompt carried only the five permitted
lines — no contamination.

Re-run against the shipped tree, never accepted from this file: **`make verify` GREEN**
(24 `ok`, 0 `FAIL`; it carries `golangci-lint fmt -d`, `go build`, `go vet`,
`golangci-lint run`, `make file-limits`, `make test`, `make test-race`, `make tidy-check`,
`make actionlint`, `make shellcheck`, `make comment-refs`), `make cover-ratchet` →
**89.19% holds against 89.44%** (tolerance 0.60 pp), and `make test-contention
CONTENTION_PARALLEL=4` → **GREEN**, `grep -c '(cached)' tmp/test-contention-race.log` → **0**
with real timings for every contended package (`internal/scheduler 6.043s`,
`internal/store 6.698s`, `internal/ingest 4.262s`), `grep -c 'SQLSTATE 57P0'
tmp/test-contention-load.log` → **0**, the target's own `exhaustion scan clean` line present,
and neither a container nor a `go test` surviving the run — AC10 re-measured PASS.

Per-row re-verification of the round-4 register:

- **R4-1 (fixed@33be21b) — confirmed by the row's own injection, not by reading the `|| true`.**
  Over a `cp` backup with `./internal/nosuchpkg/...` prepended to the load loop's package list,
  `make test-contention CONTENTION_PARALLEL=4` left `tmp/test-contention-load.log` with **5**
  `^ok` lines (> 4) across two iterations, each preceded by its own
  `FAIL ./internal/nosuchpkg/... [setup failed]` block: the loop now survives a failing
  iteration and keeps loading. Makefile restored from the backup, `git status --short` empty.
- **R4-2 (fixed@33be21b) — confirmed.** `ai-docs/go-test-conventions.md:45` now names **both**
  of `test-contention`'s children and states the run-conditions reason the foreground gate
  carries the flag. (Its sibling site is R5-1 below.)
- **R4-3 (fixed@33be21b) — confirmed.** The scan is in the target at `Makefile:145`, and the
  target classifies rather than reports: `INSTRUMENT FAILURE … exit 2`.
- **R4-4 (fixed@33be21b) — confirmed.** `grep -n 'nothing else can' AGENTS.md` and
  `grep -n 'the only thing that can' ai-docs/go-test-conventions.md` are both empty; both
  sentences now say no reaper will remove it and that any container command reaches it.
- **R1-6 — objection still upheld** (register R5-A4); `cmd/testpg/run_test.go` is unchanged
  since `bcca2d6`.

Also derived in this round rather than read: the wrapper's decision-order tests checked for the
§ *Patterns* 2 shape — every "seam untouched" case also asserts the DSN the child actually
printed, so an early return could not pass them vacuously (R5-A7); `CONTENTION_PARALLEL`'s
`nproc` default against `ceilingMax` (R5-A6, `nproc` = 16 → 672, inside the bound, and the
refusal above 24 cores is the design's stated residue); the progress file's required re-entry
fields (`current_step`, `last_passed_gate`, `entry_args` present, `parent_skill` correctly
omitted per the canonical template); and the panic-index and domain-invariant sweeps left
standing on the ground that no Go file has changed since round 3 (R5-A5). Every mutation probe
was taken over a `cp` backup and `git status --short` confirmed empty afterwards.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R5-1 | `ai-docs/context-status.md:208` | major | **A live document this diff itself wrote states a counted claim the shipped tree falsifies — and the count it gets wrong is the one defect class this task has already repeated three times.** Under *Invariants this now relies on*: "A gate that exists to exercise one particular path therefore carries `-count=1`, and **the two that do are the only ones that need it**." Measured: `grep -c 'go test .*-count=1' Makefile` → **3** — `test-fallback` (`:88`), `test-contention`'s load loop (`:138`) and `test-contention`'s foreground race gate (`:141`). The third landed in round 3's fix (19baf8c), *after* this entry was authored (55682cc), and it is there for a reason the sentence's two-case partition cannot express: not because it exercises a provisioning path, but because it asserts a property of the run's *conditions* — which is precisely what R3-1 established. R4-2 found this drift and fixed it in `ai-docs/go-test-conventions.md:45`; the sweep did not reach this file, and it is the only live site left (`grep -rn 'count=1\\|test-contention' --include='*.md' .` outside `ai-docs/plans/**`, `tmp/` and the append-only learnings log → `AGENTS.md:51,71,122`, `context-status.md:198,207,208`, `go-test-conventions.md:45,48`; only `:208` carries a count). Violates **AC14**, which the `## AC Status` table records PASS. `major`, not R4-2's `minor`, on that row's own stated ground — it read "Nothing here is strictly false, which is why it is `minor`", and this sentence *is* strictly false — and on § *Patterns* 3: the bullet is filed as an **invariant** in the file whose stated purpose is capturing "traps … worth not rediscovering", and it mis-states the trap that has now recurred three times in this run (AC3 at Step 9 → 1a3ee3a; R3-1 → 19baf8c; R4-2 → 33be21b). A reader classifying a new gate by it reaches the fourth. **Not R4-A3 re-raised** — that row rules the *design* doc's silence not-raisable; this is a live document, the half R4-2 established as raisable. **Not R4-2 re-opened** — R4-2's verifying command passes on its own file. **No design amendment**: the sentence is this diff's own new prose, so the fix is an ordinary edit to the entry this run added, not an edit of a pre-existing append-only entry. | ✅ Fixed |
| R5-2 | `ai-docs/go-test-conventions.md:48` | minor | The live convention page still hands the reader the exhaustion scan that R4-3's fix moved into the target. It reads "A red run is read before it is believed: scan both logs for `sorry, too many clients already` first" — but `Makefile:144-151` now scans, prints `test-contention: INSTRUMENT FAILURE …` and **exits 2**, so an exhausted run never reaches the reader as a red the instruction applies to. Nothing here is false, which is why it is `minor`; the gap is that **no live document names exit 2 or says what it means** — that the run is discarded, not that the race gate failed. `ai-docs/context-status.md:207` carries the semantics ("an instrument failure and is thrown away") but not that the target enforces them. Cites AC14 and `AGENTS.md` § *Propagation Rule* step 4 — the same incomplete sweep as R5-1, one fix later. | ✅ Fixed |
| R5-3 | `cmd/testpg/run_test.go:20`, `:24` | nit | `stubSeam.provisionErr` and `stubSeam.stopErr` are read by the stub's closures (`:38-39`, `:47`) and set by **no** test — `grep -n 'provisionErr\\|stopErr' cmd/testpg/run_test.go` returns only the declarations and those reads. So the two branches they exist to reach are unexercised: `runChild`'s provisioning failure (`cmd/testpg/run.go:143-145`, "could not start a server" → `exitFailure`) and the deferred teardown's error report (`:152-155`). The design's Test Design enumerates neither scenario, so this is not a design gap — it is two dead fields advertising coverage the package does not have. | ✅ Fixed |
| R5-4 | `Makefile:145` | nit | The exhaustion scan cannot tell "no match" from "the scan could not run". `grep -qE … a.log b.log \|\| exhausted=$?` yields 1 for no-match and **2** for an error — measured: `grep -qE 'zzz' tmp/test-contention-race.log /nonexistent; echo $?` → `2` — and `:147`'s `[ "$exhausted" -eq 0 ]` sends both down the same branch, printing `test-contention: exhaustion scan clean`. Unreachable today, because both logs are created by redirections earlier in the same script, which is why it is a `nit` — but it is the § *Patterns* 2 shape sitting in the one instrument the target's exit-2 verdict rests on. Exit: branch on 0 / 1 / other, and make the third an instrument failure of its own. | ✅ Fixed |

**Recorded, not raised** — each is an `accepted@5` register row rather than a finding, so round 6
is scoped by it: R4-1's injection re-run and R4-2/R4-3/R4-4 re-verified (R5-A1), the shipped
tree's own three gate runs (R5-A2), R4-A3 upheld and the design half deliberately not re-raised
(R5-A3), R1-6's objection still upheld (R5-A4), the panic / domain / D9 sweeps standing because
no Go file changed (R5-A5), `CONTENTION_PARALLEL`'s default against `ceilingMax` ruled a stated
residue (R5-A6), and the decision-order tests checked against the vacuous-guard shape (R5-A7).
