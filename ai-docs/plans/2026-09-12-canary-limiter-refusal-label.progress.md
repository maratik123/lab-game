# Progress: Canary limiter refusal label — ACTIVE
_Updated: 2026-09-12 00:06_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-canary-limiter-refusal-label
**base_commit:** c6b3ce89c401763dbcb102c51791094761ebb2d0
**Last build:** PASS

**Issue:** #89
**Spec:** ai-docs/plans/2026-09-12-canary-limiter-refusal-label.spec.md

**current_step:** Step 11 — review fixes complete (Round 1)
**last_passed_gate:** make verify (fmt-check, build, vet, lint, file-limits, test, test-race, tidy-check, actionlint, shellcheck, comment-refs, import-guard) | 2026-09-11T23:58Z | 8f934dc18971b714f52910f228e0f8b9e0270aa0
**entry_args:** 89

## Next action

**Do this immediately:** Round-1 findings are fixed and pushed; re-enter Step 10 with a warm self-review scoped to R1-1 and R1-2 only.

## Subtasks

- [x] 1. `internal/tg/caller.go` + `caller_test.go` — read `now` per loop pass beside the `ctx.Deadline()` read, add the pure refusal-cause helper (deadline form carries D4's message wrapping `context.DeadlineExceeded`), call it from the `!ok` branch; tests first — commit 5b08da6
- [x] 2. `internal/health/probe_test.go` — the canary-label test: an already-passed deadline is counted a failure with `reason` `timeout`, fake handler never reached — commit 9dbd45e

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review returned GO on round 1; all four `## Issues` rows and both recommendations were design-internal, so they were folded by `design-writer` and design-review did not run again.
- **Step 8 (subtask 1)**: the helper's doc comment and the two new end-to-end test doc comments initially named "AC1"/"AC3"/"D4"/"KD-26" as their rationale; `make comment-refs` flagged all four as forbidden ac-id/decision-anchor references (DOC-4), so the comments were reworded to describe the property directly instead of citing the spec/design anchor. Re-ran the gate clean afterward.
- **Step 8 (subtask 2)**: verified the new `TestTelegramProber_ExpiredDeadlineCountsAsTimeout` test's discriminating power directly — checked out `internal/tg/caller.go` at the pre-subtask-1 commit (HEAD~1) with the new test still in place, ran it, and it failed with `classifyFailure = "network"` as expected; restored the post-subtask-1 file (`git diff` confirmed byte-identical) before continuing.
- **Step 9**: panic-index needs no row — the only changed production file carries no `panic(` / `log.Fatal*` / `Must…` site, and both scan patterns were confirmed against a constructed control before the clean result was accepted.
- **Step 9**: domain-invariant sweep — sweeps 1, 2, 3 and 5 clean (each pattern confirmed against a constructed control first); sweep 4 hits `time.Now()` in the transport caller and its tests. Legitimate: the determinism rule binds world generation, combat and PvP-trail replay, and the Bot API transport is none of them — it is wall-clock by nature (rate-limit windows, retry backoff, latency observation). The one new read, `caller.go:69`, is the per-pass hoist the design mandates so the limiter's decision and the caller's classification read one instant.
- **Step 9**: the per-AC sweep ran three mutants rather than trusting the green suite — a non-strict predicate (`!now.Before`), the already-passed condition dropped entirely, and the call site reverted to the pre-change cause. Each was confirmed to BUILD first; each turned the expected test RED with a failure line naming the asserted property. `internal/tg/caller.go` was restored from a cp-backup and `git status` re-checked clean afterwards.
- **Step 9.5**: `context.md` left untouched — no block's high-level state moved (no new package, no new capability; the change refines which cause one existing branch carries) and its open-question line points at `docs/DESIGN.md` §16, none of whose entries this answers. `alert-contract.md` also left untouched: its `reason` sentence enumerates a status code else `timeout` / `canceled` / `network`, and option 1 adds no value to that set, so the sentence stays true. No repo-root user-facing doc exists to contradict.
- **Step 10**: self-review round 1 returned REJECT — 1 major, 1 nit open; four further rows accepted as non-defects. No re-litigation: round 1 has no earlier round to cite.
- **Step 11**: both open findings fixed as code (no `*.spec.md` / `*.design.md` in either fix diff, so neither is an amendment trigger). R1-1 was re-confirmed by reading the file before fixing, not taken on the reviewer's word.
- **Step 11**: the first post-fix `make verify` was RED at `build` — the round-1 reviewer left `caller_test_pre.go`, `probe_test_pre.go` and `fixed_test.go` in `tmp/`, and `go build ./...` walks that directory. Verified they were copies of the pre-change tree plus a constructed fixture, so nothing in the working tree was at risk, and removed them.
- **Step 11**: the second `make verify` was RED at `test-race` on `cmd/bot`'s `TestServe_SignalCleanStopExitZero`, a package this branch does not touch (`git diff --name-only 8b7cfb6..HEAD -- cmd/bot` is empty). Not treated as transient: reproduced under induced CPU load (2 failures in 1200 trials on this tree), and with `internal/tg/caller.go` restored to the base version 0 failures in 1200 — a difference that is not significant at that rate. The causal question was settled by reading instead: `serve`/`drain` walk `a.runners` / `a.closers`, which the test builds from fakes, so no `internal/tg` code is in the path; the failure is one runner's `run` returning between two sequential `stop` calls. A third `make verify` on the same tree was fully GREEN.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 1 | "D4 makes the log line the operator's discriminator between \"deadline breached during the HTTP wait\" and \"refusal decided past the deadline\", but the design never says what the deadline form's message *is*." | design-internal | folded | design § Key decisions D4 (message named) + § Decomposition subtask 1 @ c6b3ce8 |
| G2 | 1 | "\"`time.Now()` moves out of the `acquire` argument list into a `now` variable\" does not say where the variable lives." | design-internal | folded | design § The chosen shape (per-pass read, inside the loop) + § Decomposition subtask 1 @ c6b3ce8 |
| G3 | 1 | "§ Test Design opens \"All claims below are about tests that do not exist yet\" — a certification over the section's own tags" | design-internal | folded | design § Test Design — sentence deleted @ c6b3ce8 |
| G4 | 1 | "D2's strict `now.After(deadline)` deliberately disagrees with `context`'s own boundary" | design-internal | folded | design § Key decisions D2 — the `dur <= 0` clause @ c6b3ce8 |
| G5 | 1 | "Consider having the AC1 test also assert the cause's message differs from the wait-does-not-fit form" | design-internal | folded | design § Test Design items 1 and 2 @ c6b3ce8 |
| G6 | 1 | "`limit.go`'s `acquire` doc comment still describes the refusal solely as \"the required wait would end after deadline\"" | design-internal | folded | design § What this change does not touch @ c6b3ce8 |

## Key discoveries (don't re-investigate)

- The canary's prober builds its client with no `Gate`, so the gate-refusal path into `network` is unreachable for a canary leg — it is out of this task's reach.
- `internal/health` needs no production edit: `classifyFailure` already maps a `context.DeadlineExceeded` chain to `timeout`, so AC2 follows from AC1 and is discharged by a test.
- The only production sentinel consumers are the canary classifier and `internal/ingest/loop.go`, which tests `context.Canceled` only and cannot change behaviour.

## AC Status

| AC | Status | Verifying command (orchestrator's own, Step 9) | Mutant that turned it RED |
|----|--------|-----------------------------------------------|---------------------------|
| AC1 | PASS — `TestCaller_ExpiredDeadlineAtLimiterIsContextError`, `internal/tg/caller_test.go` | `go test ./internal/tg -run TestCaller_ExpiredDeadlineAtLimiterIsContextError -count=1` | call site reverted to the pre-change cause → `errors.Is(err, context.DeadlineExceeded) = false, want true` |
| AC2 | PASS — `TestTelegramProber_ExpiredDeadlineCountsAsTimeout`, `internal/health/probe_test.go` | `go test ./internal/health -run TestTelegramProber_ExpiredDeadlineCountsAsTimeout -count=1` | same mutant → `classifyFailure = "network", want "timeout"` |
| AC3 | PASS — `TestCaller_RealWaitPastDeadlineIsNotContextError`, `internal/tg/caller_test.go` | `go test ./internal/tg -run TestCaller_RealWaitPastDeadlineIsNotContextError -count=1` | already-passed condition dropped (`if hasDeadline {`) → `errors.Is(err, context.DeadlineExceeded) = true, want false` |
| — | boundary + message, `TestLimiterRefusalCause` | `go test ./internal/tg -run TestLimiterRefusalCause -count=1` | predicate relaxed to `!now.Before(deadline)` → the `now_equal_to_deadline` row fails on both the chain and the wording |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-1 | round 1 | major | fixed@8f934dc | `sed -n '/^func TestLimiterRefusalCause/q;p' internal/tg/caller_test.go \| tac \| awk '/^\/\//{print;next}{exit}' \| tac \| head -1` — must print `// TestLimiterRefusalCause …`; now prints `// TestLimiterRefusalCause is a table test over limiterRefusalCause's whole` |
| R1-2 | round 1 | nit | fixed@8f934dc | `sed -n '165,180p' internal/tg/caller.go` (coordinate re-resolved after the fix) — the helper's doc comment must name neither the limiter's predicate nor its own `if` line nor its test; it now states the contract, the strictness decision and the totality reason only |
| R1-3 | round 1 | nit | accepted@1 — pre-existing, outside the diff window (present at `8b7cfb6` too) | `python3 tmp/docblock-check.py internal/health/probe_test.go` → `probe_test.go:331: doc block above func TestClassifyFailure opens with: // classifyFailure_TableTest …` |
| R1-4 | round 1 | nit | accepted@1 — cohesive test file for one production file; code-style § File size says do not flag a cohesive medium file, and the hard band is 1500 | `wc -l internal/tg/caller_test.go` → 583 |
| R1-5 | round 1 | minor | accepted@1 — the determinism rule binds world generation, combat and PvP-trail replay; the Bot API transport is none of them and is wall-clock by nature | `grep -n 'time.Now()' internal/tg/caller.go` → 69, 137 (69 is the per-pass hoist the design mandates) |
| R1-6 | round 1 | nit | accepted@1 — the prompt's range is the merge-base, a superset of the recorded `base_commit`; reviewed over the superset, so coverage is strictly wider | `git merge-base main HEAD` → `8b7cfb62071cfaca697c176318aacb9227e08403`; header records `c6b3ce89c401763dbcb102c51791094761ebb2d0` |

## Self-Review (Round 1)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| R1-1 | internal/tg/caller_test.go:174 | major | The new tests were inserted between `TestCaller_HTTPClientFallsBackToDefaultClient`'s doc comment and its `func` line, with no blank line. The block at 174–179 ("…covers (\*caller).httpClient's http.DefaultClient fallback…") is now the OPENING of `TestLimiterRefusalCause`'s doc comment, so that comment's first sentence names a different function and describes a test it is not — a DOC-1 violation (a doc comment opens with the identifier's own name) and a false claim in a durable artefact. The same edit orphaned `TestCaller_HTTPClientFallsBackToDefaultClient` (now line 333), which lost its explanation of a non-obvious fixture with no decision recording it. Instrument verified in both directions — a per-file check that every `func Test…`'s contiguous doc block opens with that function's own name is GREEN on `git show 8b7cfb6:internal/tg/caller_test.go` and RED on the shipped file (`internal/tg/caller_test.go:174: doc block above func TestLimiterRefusalCause opens with: // TestCaller_HTTPClientFallsBackToDefaultClient covers`), and the register's one-liner flips to `// TestLimiterRefusalCause is a table test…` once a blank line is reinstated. Fix: move the 174–179 block back so it sits immediately above `func TestCaller_HTTPClientFallsBackToDefaultClient` (a blank line alone only leaves it floating, attached to nothing). | ✅ Fixed |
| R1-2 | internal/tg/caller.go:165-181 | nit | `limiterRefusalCause`'s doc comment crosses both review-judged halves of DOC-4. It reproduces another function's implementation verbatim ("acquire's own predicate (hasDeadline && t.After(deadline))") — which rots the moment that predicate changes; it restates its own `if` line ("The predicate is strict (now.After(deadline), not !now.Before(deadline))"); and it points outward by a bare unqualified name ("for the table test", "a canary probe's failure classifier"). The contract — hasDeadline is guaranteed true by the only caller, and an already-passed deadline yields the deadline form — survives all three cuts. | ✅ Fixed |

**What was checked.**

- **Diff window.** `8b7cfb62071cfaca697c176318aacb9227e08403..HEAD` (the merge-base with `main`, a superset of the header's `base_commit` `c6b3ce8`): `internal/tg/caller.go`, `internal/tg/caller_test.go`, `internal/health/probe_test.go`, `ai-docs/context-status.md`, `ai-docs/learnings.md`, plus the spec / design / state / progress plan files.
- **AC1 · AC2 · AC3 — each re-run and each mutated, not read.** All four verifying commands from `## AC Status` PASS. Discriminating power established against four mutants, every one confirmed to BUILD first, `internal/tg/caller.go` restored from a cp-backup and `git status` re-checked clean after each: (a) call site reverted to the pre-change `errors.New` cause → `TestCaller_ExpiredDeadlineAtLimiterIsContextError` and `TestTelegramProber_ExpiredDeadlineCountsAsTimeout` both RED, nothing else; (b) predicate relaxed to `!now.Before(deadline)` → only `TestLimiterRefusalCause/now_equal_to_deadline` RED, exactly the row D2 names; (c) the already-passed condition dropped to `if hasDeadline {` → `TestCaller_RealWaitPastDeadlineIsNotContextError` plus the `deadline_still_ahead` and `now_equal_to_deadline` rows RED; (d) the deadline form's message replaced by the wait form's wording, `%w` kept → `TestCaller_ExpiredDeadlineAtLimiterIsContextError` and `TestLimiterRefusalCause/deadline_already_passed` RED, which is D4 gated rather than merely described.
- **Design conformance.** `now` is read inside the loop, on the line beside `ctx.Deadline()` (caller.go:69) and is the same instant passed to `acquire` and to the helper — the D1/D2/D3/D4/D5/D6 shape as written. D6 confirmed by the diff: the only two hunks in `caller.go` are the hoist and the `!ok` cause; the retry-wait pre-refusal branch is untouched. All six `## GO notes` rows are routed `folded` and each is present in the design at `c6b3ce8`, which precedes the first implementation commit `5b08da6` — G1 (D4 names the message), G2 (§ The chosen shape fixes the per-pass read), G3 (the Test Design certification sentence is gone), G4 (D2 carries the `dur <= 0` clause), G5 (Test Design items 1 and 2 assert the wording), G6 (§ What this change does not touch records the `acquire` doc comment as deliberately left).
- **The helper's stated precondition, verified rather than trusted.** `acquire` returns `ok == false` only under `hasDeadline && t.After(deadline)` (`internal/tg/limit.go:331`), and the caller's `acquireErr != nil` branch returns before the `!ok` branch — so `hasDeadline` is indeed always true where the helper is called.
- **Blast radius of putting `context.DeadlineExceeded` into a refusal chain.** The non-test sentinel sites are `internal/health/probe.go:137,139` (the intended consumer) and `internal/ingest/loop.go:183`, which tests `context.Canceled` only and cannot move. `labelReason` is produced in `internal/health` alone, so no other surface renders a `reason`.
- **Prose claims in the diff, each re-derived.** `context-status.md`: the per-pass hoist, the zero-wait grant at `now == deadline` (`t.After(deadline)` is false there), `time.AfterFunc` at `context.go:652` under go1.26.5, and "an attempt count of zero identifies the pre-attempt refusal only on the first pass" (`attempts++` sits after `doAttempt`, so a later-pass refusal carries the attempts already made) all hold; the `#TBD-at-Step-12` locator is the literal Step 9.5 mandates, not a lapse, and the entry carries no counts. `learnings.md`: the design-review spawn contract does fix `Spec:` / `Design:` / `Progress:` / `Round: <N>` as line shapes and the `PreToolUse` hook does match `Task|Agent`.
- **Gates, run against the shipped tree.** `go vet ./...` exit 0 · `golangci-lint run` → `0 issues.` · `make comment-refs` green · `make file-limits` green · `go test -race ./internal/tg ./internal/health -count=1` green (the `-race` run the header records at `c8b4e38` re-confirmed here).
- **Safety and invariants.** Panicking-call grep over `internal/tg/caller.go` returns nothing, and the pattern was confirmed against a constructed control first — so no `panic-index.md` row is owed. No ledger write, no basis document, no posting, no schema change, no new mechanic and so no telemetry owed, no balance constant (the test's `rate(1, time.Hour)` is a fixture), no outbound send path, no secret. No `//nolint` added. Error construction: the deadline form wraps the stdlib sentinel with `%w` and its message names the operation.
- **Progress-file required fields present** (content not reviewed): `Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `entry_args`, `## Decisions log`. `parent_skill` is correctly absent — `/task` is the parent flow here.

## Files touched

- `internal/tg/caller.go` — `now` read per loop pass; new `limiterRefusalCause` helper
- `internal/tg/caller_test.go` — `TestLimiterRefusalCause`, `TestCaller_ExpiredDeadlineAtLimiterIsContextError`, `TestCaller_RealWaitPastDeadlineIsNotContextError`
- `internal/health/probe_test.go` — `TestTelegramProber_ExpiredDeadlineCountsAsTimeout`
