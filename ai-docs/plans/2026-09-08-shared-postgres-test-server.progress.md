# Progress: Shared PostgreSQL test server — ACTIVE
_Updated: 2026-09-08 21:41_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** perf/2026-09-08-shared-postgres-test-server
**base_commit:** 943efed1e0c8c7fa1ee406b8c23b1e2632e8f500
**Last build:** PASS

**Issue:** #67
**Spec:** ai-docs/plans/2026-09-08-shared-postgres-test-server.spec.md

**current_step:** Step 8 — Group B COMPLETE (subtasks 10-13 of 13 committed)
**last_passed_gate:** `make comment-refs && bash ai-docs/scripts/check-ac-shape.sh && bash ai-docs/scripts/check-spec-shape.sh && bash ai-docs/scripts/check-script-shape.sh && bash .claude/skills/ai-audit/scripts/check-citations.sh && CI's relative-markdown-link check re-run locally` | 2026-09-08T21:41Z | 47f526d
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
- **Step 8 (Group B)**: this file had NO `## Files touched` heading when Group B opened it — the orchestrator's Group A repair commit truncated its own decision line mid-sentence at that heading's text and lost the heading with it, so the file list had been living inside the Decisions log. The heading is restored here; the truncated line above is left byte-identical, because the Decisions log is append-only and its content is not Group B's to rewrite.

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
