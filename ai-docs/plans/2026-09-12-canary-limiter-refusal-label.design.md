# Design: Canary limiter refusal label

**Issue:** #89
**Date:** 2026-09-12

## Approach

### What the code does today

`caller.Call` reads the context's deadline, asks the limiter for an emission
instant, and on a refusal builds the package's typed error with a fixed textual
cause that wraps nothing
`[measured 0b1feaa:internal/tg/caller.go:69-82 · grep -n "required wait ends after" internal/tg/caller.go → 79: errors.New("limiter: required wait ends after the context deadline")]`.

The limiter refuses on one predicate — the decided instant lands after the
deadline
`[measured 0b1feaa:internal/tg/limit.go:331-333 · grep -n "hasDeadline && t.After(deadline)" internal/tg/limit.go → 331]` —
and that predicate is true for two different worlds:

- a required wait that genuinely ends after a deadline still ahead, and
- a deadline that had **already passed** before the call reached the limiter. A
  class with no window configured answers `now` itself, and `now` is already
  past the deadline, so the refusal fires with no wait to name
  `[measured 0b1feaa:internal/tg/limit.go:108-116 · grep -n "if len(s.windows) == 0" internal/tg/limit.go → 114]`.

The canary's classifier renders `timeout` only for a chain carrying
`context.DeadlineExceeded`
`[measured 0b1feaa:internal/health/probe.go:132-144 · grep -n "case errors.Is(err, context.DeadlineExceeded)" internal/health/probe.go → 137]`,
so both worlds fall through to `network`. The owner's decision splits them.

### The chosen shape

Split the **cause**, at the refusal site, from the same
`(now, deadline, hasDeadline)` triple the limiter decided the refusal from.
`time.Now()` moves out of the `acquire` argument list into a `now` variable, so
the caller's classification and the limiter's decision read one instant and can
never disagree
`[measured 0b1feaa:internal/tg/caller.go:70 · grep -n "acquire(call, time.Now()" internal/tg/caller.go → 70: …acquire(call, time.Now(), deadline, hasDeadline)]`.

A small pure helper in `internal/tg/caller.go` answers "which cause does this
refusal carry": an already-passed deadline yields a cause wrapping
`context.DeadlineExceeded` with `%w`, and every other refusal yields today's
wait-does-not-fit cause unchanged. The refusal branch calls it; nothing else in
the loop moves `[derived → AC1, AC3]`. The chain reaches a caller intact because both wrappers between
the cause and the returned value unwrap
`[measured 0b1feaa:internal/tg/errors.go:49 + internal/tg/retry.go:78 · grep -n "func (e \*Error) Unwrap|func (e \*sanitizedError) Unwrap" → errors.go:49, retry.go:78]`,
and telego's own layer wraps its transport error with `%w` and has no context
short-circuit of its own above it
`[measured telego@v1.11.2 (the pinned version, go.mod) module cache bot.go:167-172 · grep -rn "ctx.Err()" *.go in that module root → webhook.go:74 only, in the webhook handler]`.

`internal/health` needs **no production change**: its classifier already maps a
`context.DeadlineExceeded` chain to `timeout` (cited above), so AC2 follows from
AC1 and is discharged there by a test, not by an edit `[derived → AC2]`.

`getMe` carries no send or edit prefix, so the canary's calls are the `Other`
class
`[measured 0b1feaa:internal/tg/class.go:20-21,38-46 · grep -n "ClassOther is every other" internal/tg/class.go → 20]`,
and it addresses no chat, so the class-global schedule is the only one consulted
for it
`[measured 0b1feaa:internal/tg/limit.go:238-241 · read → chatScheduleLocked returns nil for ChatNone, "such a call charges its class-global schedule only"]`.

### Key decisions

- **D1 — The split is decided in the caller's refusal branch, not inside
  `Limiter.acquire`.** The limiter is a pure scheduler that takes a deadline as
  a parameter; the context is the caller's, and "has my context's deadline
  already passed" is the caller's question. `acquire`'s decide-then-commit
  contract and its `(time.Time, bool, error)` signature — where a non-nil error
  means the fixed-point defect, not a refusal — stay exactly as KD-25 fixed them
  `[measured 0b1feaa:internal/tg/limit.go:298-340 · grep -n "func (l \*Limiter) acquire" internal/tg/limit.go → 311]`.
- **D2 — The predicate is `hasDeadline && now.After(deadline)`, strict.** At
  `now == deadline` a zero wait is still granted (the refusal predicate is
  `t.After(deadline)`), so a refusal at that instant is necessarily a real-wait
  refusal and the deadline has not passed — which is the world AC3 governs. A
  non-strict `!now.Before(deadline)` would mislabel it.
- **D3 — The deadline form wraps `context.DeadlineExceeded` directly, never
  `ctx.Err()`.** Two independent reasons: a context that is *cancelled* while
  its deadline is still ahead would make `ctx.Err()` yield `context.Canceled`,
  which AC3 forbids in that world; and a deadline context publishes its error
  from a `time.AfterFunc` callback
  `[measured go1.26.5 GOROOT src/context/context.go:652 · go version + grep -n "time.AfterFunc" → 652, inside WithDeadlineCause]`,
  so it lags the wall clock by however long that callback is delayed — the caller
  would then refuse for one reason and label for another, which is the
  load-sensitivity class #84 documents.
- **D4 — The deadline form's message still names the limiter.** A deadline
  breached during the HTTP wait and a refusal decided past the deadline now
  render the same label, so the log line is what an operator tells them apart by
  — that, and `Attempts` being zero on this one `[derived → AC2]`.
- **D5 — `internal/health` carries no production edit.** AC2 is a property of
  the existing classifier once AC1 holds; adding a branch there would be a second
  encoding of the same rule.
- **D6 — The *retry-wait* pre-refusal branch is not touched.** It is a different
  branch, its cause is the last attempt's, and the test that pins that cause as
  *not* a context error belongs to the transport contract
  `[measured 0b1feaa:internal/tg/retry_test.go:324-365 · grep -n "TestRetry_DeadlineRefusalInsteadOfSleep" internal/tg/retry_test.go → 324]`.
  See `## Open questions`.

### Rejected alternatives

- **Widen `Limiter.acquire` to report the refusal reason** (an extra return, or
  a refusal error in the existing `error` slot). It merges the "this is a defect"
  signal with "this is normal traffic" at the type level, widens an unexported
  API that the limiter's own suite drives through a shared wrapper over its whole
  test set
  `[measured 0b1feaa:internal/tg/limit_test.go:19-26 · grep -n "func acquireOK" internal/tg/limit_test.go → 19, the helper every limiter test calls acquire through]`,
  and buys nothing: the caller already holds every input the distinction needs.
- **Short-circuit before `acquire`** — return the deadline error at the top of
  the loop when the deadline has passed, never consulting the limiter. Same
  observable outcome, but it skips the eviction pass that runs inside `acquire`
  and bounds the schedules' memory
  `[measured 0b1feaa:internal/tg/limit.go:311-320 · grep -n "global.evict(now)" internal/tg/limit.go → 317]`,
  so a workload arriving entirely past its deadline would stop ageing grants out;
  and it makes AC1's antecedent ("the limiter refuses") vacuous rather than true.
- **A new `reason` value for a limiter refusal** (the issue's option 2) and
  **documenting `network` as covering it** (option 3). The owner chose option 1
  in round 1; both others are settled against.
- **`ctx.Err()` as the predicate** — see D3.

### What this change does not touch

- The transport's own observation. The refusal branch's `observe` call and its
  arguments are untouched, so the series `internal/health` reads from this
  package move exactly as they do today — only the error's cause differs
  `[measured 0b1feaa:internal/tg/caller.go:77-82 · read → the !ok branch's observe call takes lastStatus, rateLimited and observedRetries(attempts), none of them an expression this change edits]`.
- The alert contract's series, conditions or label set: `timeout` is already a
  member, and the sweep of the live documentation found no page stating which
  failures fall under which value
  `[measured 0b1feaa:ai-docs/alert-contract.md:172-175 · grep -rniE "classif|refus" over ai-docs/*.md docs/ AGENTS.md .claude/ → the reason paragraph at alert-contract.md:172-175 enumerates the values and defines none of them; no other live page classifies a limiter refusal]`.
- The tests #84 owns (`TestCaller_LimiterDelaysAndHonoursDeadline`,
  `TestTelegramProber_Timeout`), both of which that issue's merged PR already
  retimed
  `[measured PR #90 (merged) · gh pr view 90 --json state,files → MERGED; internal/tg/caller_test.go and internal/health/probe_test.go among the changed paths]`.
  The spec's Out-of-scope row 2 gives their wall-clock shape to #84.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Hoist `now` out of the `acquire` argument list; add the pure refusal-cause helper (doc comment stating the precondition that a refusal without a deadline is unreachable, and carrying no path, section or identifier outside this package per DOC-4); call it from the `!ok` branch. Tests first, in the same subtask: the helper's table test, the AC1 end-to-end refusal, and the AC3 time-still-left refusal under `synctest`. | `internal/tg/caller.go`, `internal/tg/caller_test.go` | — |
| 2 | Add the canary-label test: a probe whose context deadline has already passed is counted a failure with `reason` `timeout`, and the fake Bot API handler is never reached. | `internal/health/probe_test.go` | 1 |

## Handoff plan

`M = 2`. Both subtasks change `*.go` only, so they are one homogeneous **code**
group — the fewest groups the size cap (`≤ 10`), the dependency order (2 after
1) and change-type homogeneity allow, and within the default maximum of 4
groups.

- **Handoff into Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Every
  design-defined group is entered that way, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned in frontmatter) via the
  `code-writer` subagent, 1M-token window — subtasks 1–2 (code change-type:
  `*.go`). Terminal group (2 subtasks; within the `1..=10` range). No handoff
  between groups; this single group completes Step 8 in its own
  `/context-reset` subagent.

## Risks

- **Blast radius of putting `context.DeadlineExceeded` into a refusal chain.**
  The production sites that test those sentinels are the canary's failure
  classifier — the intended consumer — and the ingest poll loop, which tests
  `context.Canceled` only and therefore cannot change behaviour
  `[measured 0b1feaa:internal/health/probe.go:137,139 + internal/ingest/loop.go:183 · grep -rn "DeadlineExceeded|context.Canceled" --include=*.go internal/ cmd/ (non-test) → those sites, plus a doc comment in internal/tg/errors.go]`.
- **A test that pins the opposite half goes red.** The retry-wait pre-refusal's
  cause must stay non-context; that test runs under `synctest` with virtual time
  that does not advance, and a transport fixture with no limits, so its refusal
  is decided with the deadline still ahead and D2's predicate is false there
  `[measured 0b1feaa:internal/tg/client_test.go:140-148 · grep -n "func validTransport" -A 8 internal/tg/client_test.go → no Limits field set]`.
  Subtask 1's gate run is what confirms it
  `[derived → the internal/tg suite green at the end of subtask 1]`.
- **New tests inheriting the load sensitivity #84 fixed.** Every new assertion is
  either clock-free (a deadline that is in the past by construction stays in the
  past under any amount of descheduling) or inside a `synctest` bubble per KD-26
  — no sleep, no patience bound, no wall-clock threshold
  `[derived → the tests § Test Design lists]`.
- **A green test that never exercised the new branch.** Each AC test asserts
  `Attempts == 0` and the typed error's method name, so a failure produced by an
  HTTP-level deadline or by a short-circuit above this package cannot be mistaken
  for the refusal branch — `[derived → the AC1 and AC3 tests in § Test Design]`.
- **Coverage ratchet.** The helper's unreachable-in-production row (no deadline)
  is covered by its table test rather than left as an uncovered statement —
  `[derived → the helper table test in § Test Design]`.
- **Panic surface.** Nothing added panics: the helper returns an error on every
  path and takes no pointer, and the index it would otherwise need a row in
  carries no entry
  `[measured 0b1feaa:ai-docs/panic-index.md:7-9 · read → the table body is the em-dash placeholder]`.

## Test Design

All claims below are about tests that do not exist yet.

**1. The refusal-cause helper — table test, `internal/tg/caller_test.go`.**
- Entry point: the pure helper subtask 1 adds.
- Scenarios, one row each: deadline already passed → the chain carries
  `context.DeadlineExceeded`; `now` equal to the deadline → it does not; deadline
  still ahead → it does not; no deadline → it does not.
- Fixtures: a fixed base instant and offsets from it; no client, no server, no
  clock read. `[derived → AC1, AC3]`
- Discriminating power: under a mutant relaxing D2's predicate to
  `!now.Before(deadline)`, the `now == deadline` row must go RED and no other row
  may move. `[derived → this test]`

**2. AC1 — an expired deadline at the limiter decision, `internal/tg/caller_test.go`.**
- Entry point: `Client.API().GetMe` on a client built with the suite's existing
  fake Bot API server and default transport fixture.
- Scenario: a context whose deadline is already in the past. Assert the returned
  error is this package's typed error, that its method is `getMe` and its
  `Attempts` is zero (so the failure is the pre-attempt refusal, not an HTTP
  timeout and not a short-circuit above this package), and that the chain carries
  `context.DeadlineExceeded`. `[derived → AC1]`
- Fixtures: none beyond the existing test-client helper. No sleep: the deadline
  is in the past by construction.
- Discriminating power: it must be seen RED against the pre-change caller, where
  the chain carries the wait-does-not-fit cause instead. `[derived → this test]`

**3. AC3 — a real wait ending past a deadline still ahead, `internal/tg/caller_test.go`.**
- Entry point: the same, inside `synctest.Test` per KD-26.
- Scenario: transport limits set so the `Other` class admits one call per hour;
  the first `GetMe` succeeds; the second runs under a deadline of seconds. Assert
  elapsed virtual time is zero (it refused rather than sitting out the hour),
  `Attempts` is zero, and the chain carries **neither** `context.DeadlineExceeded`
  **nor** `context.Canceled`. `[derived → AC3]`
- Fixtures: the existing fake server and test-client helpers; the limits value is
  a test fixture, not a production tuning number, so it stays in the test.
- Discriminating power: under a mutant widening D2's predicate to `hasDeadline`
  alone, this test must go RED while test 2 stays green. `[derived → this test]`

**4. AC2 — the canary's label, `internal/health/probe_test.go`.**
- Entry point: `TelegramProber.Probe`, then the package's failure classifier on
  its result — the same pairing the existing probe tests use.
- Scenario: the prober's transport fixture configures no limits
  `[measured 0b1feaa:internal/health/probe_test.go:23-31 · grep -n "AttemptTimeout:   30" internal/health/probe_test.go → 29, in a struct literal with no Limits field]`,
  and the probe runs under a context whose deadline is already in the past.
  Assert the probe errored, the recorded status code is zero, the classifier
  renders `timeout`, and the fake server's handler was never invoked — the
  issue's own claim that no request is written and no network is touched.
  `[derived → AC2]`
- Fixtures: the existing prober helper plus a request counter of the shape the
  server-error probe test already uses.
- Discriminating power: it must be seen RED against the pre-change transport,
  where the same probe renders `network`. `[derived → this test]`

**Gate expectations for both subtasks:** `go build ./...`, `golangci-lint run`,
`golangci-lint fmt -d`, and the project's test route (`make test` /
`make test-race`, which provision one shared Postgres for the whole run). While
iterating, `go test -race ./internal/tg/ ./internal/health/` is the fast loop —
neither package is database-backed, so it starts no container
`[measured 0b1feaa:internal/tg/main_test.go:10-12 + internal/health/main_test.go:10-12 · read → each TestMain is leaktest.Main(m, (*testing.M).Run), not the database provisioner's Main]`.
`[derived → the subtask gate runs]`

## Open questions

- **The retry-wait pre-refusal's analogous case — not blocking.** When the retry
  loop refuses because the *next* wait would end after the deadline, the same
  "the deadline had already passed" world exists and this design leaves it
  reporting the last attempt's cause (D6). Nothing in the spec's Scope, Key
  decisions or ACs reaches it — each names the limiter refusal — and the issue
  body's Mechanism section names only the `!ok` branch after `limiter.acquire`.
  #84, which the interview state file records as the linked issue, is about test
  timing, and its merged PR changed no production file in `internal/tg`
  `[measured PR #90 (merged) · gh pr view 90 --json files → test files, ai-docs pages and internal/scheduler/worker.go among the changed paths; no internal/tg production file]`.
  AC1–AC3 are fully satisfied without it. If the owner wants the two deadline
  branches to agree, that is a follow-up.
