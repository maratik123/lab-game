# Progress: Shared PostgreSQL test server — ACTIVE
_Updated: 2026-09-08 22:07_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** perf/2026-09-08-shared-postgres-test-server
**base_commit:** 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**Last build:** PASS

**Issue:** #67
**Spec:** ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md

**current_step:** Step 11 — review fixes complete (Round 1)
**last_passed_gate:** `make verify` | 2026-09-08T23:20Z | f4e6e759f08fedab25a57a119ad17980c2d3aa35
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
| R1-3 | round 1 | major | fixed@4088c69 | `make test-contention CONTENTION_PARALLEL=4; grep -inE 'ceiling\|clients=\|parallel=' tmp/test-contention.log` → prints both the granted ceiling and the client/parallel pair. The recipe now captures the TARGET's own two streams to a third scratch file and replays them, which is the exit the finding offered ("a file under tmp/"); the two child logs stay the children's own output, since the arithmetic is the target's, not theirs. Coverage half: with `cmd/testpg/run.go`'s ceiling `logf` deleted, `go test -count=1 ./cmd/testpg` goes RED — mutation-run, not assumed |
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
| R2-1 | round 2 | nit | fixed@4088c69 | `grep -n 'testdb: %v' internal/testdb/testdb.go` against `grep -n 'testdb: starting' internal/testdb/server.go` |
| R2-2 | round 2 | nit | fixed@4088c69 | `grep -n 'os.Setenv' cmd/testpg/run.go` against `grep -n 't.Parallel' cmd/testpg/run_test.go` at the --up/--down tests |
| R2-A1 | round 2 | — | accepted@2 — R1-1's correction re-measured independently on the shipped tree: the DSN is in no cache key, so all four corrected documents are true | `go run ./cmd/testpg -- go test -count=1 ./internal/store/` then `LAB_GAME_TEST_DSN='postgres://labgame:labgame@127.0.0.1:1/labgame_test?sslmode=disable' go test ./internal/store/` → `(cached)` |
| R2-A2 | round 2 | — | accepted@2 — `internal/ingest`'s untouched 2 s / 5 s budgets: subtask 8's mandate was to CLASSIFY every wall-clock constant, and these were classified and recorded; the probe is green with `internal/ingest` at 4.194s under load | `grep -rn 'time.Sleep\|WithTimeout\|time.After' internal/ingest/*_test.go` |
| R2-A3 | round 2 | — | accepted@2 — `schemaMaxConns`'s `max_connections = 100` comment is still true of the path it describes; the design reserves the image default for the fallback explicitly | `grep -n -B 3 'schemaMaxConns = 4' internal/testdb/testdb.go` |
| R2-A4 | round 2 | — | accepted@2 — `TestBinaries_matchesTree` compares counts and prints the found package names, so it does fail by name; a name-set manifest would need a second constant | `grep -n -A 4 'if len(got) != Binaries' internal/testdb/server_test.go` |
| R2-A5 | round 2 | — | accepted@2 — AC9 re-derived on the shipped file list: every changed path is `**/*.go`, `Makefile`, `.githooks/**`, `ai-docs/**`, `AGENTS.md` or `.github/workflows/**`, all named in the filter | `git diff --name-only dbff3d8..HEAD` against `.github/workflows/ci.yml:38-79` |
| R2-A6 | round 2 | — | accepted@2 — R1-6's objection upheld: a nit with a specific, technically accurate reason; the "thirteen"/"three entry points" counts are loose (14 functions, `runChild` + `run --up` + `run --down`) but the ground holds | `grep -c '^func Test' cmd/testpg/run_test.go` → 14 |
| R2-A7 | round 2 | — | accepted@2 — R1-2, R1-4, R1-5, R1-7, R1-8 each re-verified fixed on the shipped tree; no container and no locator file left behind by any probe | see the round-2 § What was checked |

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
