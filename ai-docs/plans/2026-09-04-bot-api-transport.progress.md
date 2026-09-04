# Progress: Bot API transport over telego — ACTIVE
_Updated: 2026-09-04 17:50_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-bot-api-transport
**base_commit:** c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**Last build:** PASS
**Issue:** #19
**Spec:** ai-docs/plans/2026-09-04-bot-api-transport.spec.md
**current_step:** Step 11 — review fixes complete (Round 1)
**last_passed_gate:** make verify | 2026-09-04T19:16:29Z | 28b1c09
**entry_args:** 19

## Next action

**Do this immediately:** Step 10 — re-spawn `self-review` COLD on the fix-round diff, then Step 12.

## Subtasks

- [x] 1. `internal/config`: optional-with-default transport key class, `.env.example`, falsified doc comments — commit c79c12b
- [x] 2. `internal/tgtest`: in-process fake Bot API server — pipe-backed `net.Listener`/`http.Server`, no real socket, `.invalid` base URL, fake token, `Success`/`TooManyRequests`/`ServerError`/`Delayed`/`FailNextDial`/`CloseWithoutResponse` behaviours, 8 tests all green under `-race`
- [x] 3. `internal/tg` foundations + the telego dependency — `go get github.com/mymmrac/telego@v1.11.2` (direct requirement, go.mod/go.sum confirmed idempotent under a second `go mod tidy`); `Error`/`OptionError` (D8), `Observation`/`Observer` (D11), `MethodClass`+classifier (D4, tested against telego's own real method names incl. trap cases), `ChatTarget`/`ChatRef`/`Call`/`Gate` (D12), `Options`/`New`/`Client.API()` (D2) — `New` wires only `WithAPIServer` + `WithLogger`/`WithDiscardLogger` so far; `WithAPICaller`/`WithRequestConstructor` wiring is subtask 4's job per the design's own decomposition (client.go is edited again there)
- [x] 4. `internal/tg` limiter (D9) + minimal caller — `limit.go` (`schedule`/`Limiter`, decide-then-commit, ordered/unordered kinds, time-based `evict`), `constructor.go` (`encoding/json` `RequestConstructor`), `caller.go` (derives method+`ChatRef` from URL/body, gate, limiter acquire+wait, one HTTP attempt), `client.go` wires `WithAPICaller`/`WithRequestConstructor`. **All 7 of the binding red-first broken-variant table's rows implemented and run RED-then-GREEN** (`TestRedFirst_CommitAtCandidate`, `_CommitThenUndo`, `_BucketCap`, `_OrderedGlobal`, `_CountRetention`, `_PaceOnly`, `_QuotaOnly` — none stayed green as a broken variant). Plus `TestSchedule_*` (brute-force oracle over 200 randomized trials, invariant-after-every-commit, ordered non-decreasing, unbounded-contributes-no-window, time-based retention, real-now-vs-future-grant eviction), `TestLimiter_*` (AC10–AC15, AC30–AC33 functional scenarios), and `TestCaller_*` end-to-end through real telego + tgtest (success, `ChatKnown` derivation from a real `SendMessage`, gate refusal costs zero attempts, limiter delay honours a short context deadline).
- [x] 5. `internal/tg` caller: retry loop, backoff, `retry_after`, typed error, observation — `retry.go` (`defaultJitter` with the `//nolint:gosec // G404` directive, `backoffDelay` per D6's exact formula, `classifyAttempt` per D5's table, `sanitizedError`/`sanitizeErr` dropping the `*url.Error` layer plus a token-replacer defense-in-depth per D8, `Client.giveUpError`/`observe`); `caller.go`'s `Call` rewritten into the full per-attempt-charging loop (limiter `acquire` moved inside the loop, `httptrace.WroteRequest` sticky `atomic.Bool` wired into `doAttempt`). All of `retry_test.go`'s AC4/AC5/AC6/AC7/AC8/AC9/AC16/AC18/AC26/AC33 scenarios pass under `testing/synctest` (D14) — virtual-clock retry_after/backoff/deadline/cancellation waits complete in ~0.01s wall time despite simulating hours. `golangci-lint run` caught two real style defects (an `if`/`else if` chain gocritic wanted as a `switch`, and QF1008 promoted-field simplifications on `resp.Error.X`) — both fixed.  ← CURRENT
- [x] 6. `internal/tg` package-level guard tests — `guards_test.go`: AC2 (no fasthttp/go-json import under `cmd/`+`internal/`), AC17 (no metrics-registry import in `internal/tg`), AC21's literal-scan (no bare `<digit> * time.Unit` in production files — confirmed the regex does NOT false-positive on retry.go's genuine runtime `retry_after`-seconds conversion), the D2 seam guard (`telego.NewBot`/`telego.With*` grep outside `internal/tg` — verified as a REAL red-then-green control: a planted `telego.NewBot(...)` call in `cmd/bot` was caught, a bare unqualified reference to the function value was correctly NOT caught, confirming the regex targets calls), AC15's source half (`BaseURL` referenced only in `client.go`), the D4 token-exposure-sites log (informational, `Token()`/`FileDownloadURL`), AC27 (a refusing gate blocks a call through `Client.API()`), and an end-to-end call through a real `config.Load`-produced `Config.BotAPIBaseURL` into `tgtest`. **Group A (subtasks 1-6) is now complete** — every gate green on the full tree.
- [x] 7. `ai-docs/key-decisions.md`: KD-2 rewrite + KD-25/KD-26/KD-27 — commit 453503c

## Decisions log

- **Step 6–7**: design reached GO at review round 7; the first mechanism (composed limiter legs) was abandoned at the owner's direction after three rounds produced four defects of one shape, and replaced by a single per-key schedule holding a set of `(count, per)` windows.
- **Step 7**: owner decisions recorded — keep `LAB_GAME_TG_ATTEMPT_TIMEOUT` (30s); defer `stdjson` to its own PR; stay on Go 1.26.5 with telego pinned at v1.11.2; leave `docs/DESIGN.md` §11 untouched; media stays out of MVP with no media mechanism; no "exactly one sending process" constraint recorded.
- **Step 7**: the telego v1.12.x ceiling is a configuration block, not an incompatibility — v1.12.1 builds clean under go1.26.5 once its own directive is lowered; raising the toolchain is deferred to its own task.
- **Subtask 1 (code-writer)**: implemented `loadTransport` as a dedicated reader (never added to `envKeys()`), 13 `LAB_GAME_TG_*` keys, `Rate`/`ClassLimits`/`TransportLimits`/`Transport` value types per D10's exact shape. `.env.example` and the reader landed in the same commit (disjointness test requires it). `env.go`/`config.go`'s falsified doc comments rewritten; `doc.go`'s enumeration extended. Verified via `rg -U` sweep that no other live site asserts the old "every variable is required" claim outside history surfaces and balance-scoped comments. All gates green: build, full `go test ./...`, `go vet`, `golangci-lint run` (0 issues), `golangci-lint fmt -d` (clean).
- **Subtask 4 (code-writer) — a genuine defect the red-first tests caught, not one the design predicted.** The first `schedule.commit(t)` implementation evicted aged-out grants using `t` (the just-granted, possibly far-future instant) as the "now" for the eviction threshold. Measured directly: `TestSchedule_InvariantHoldsAfterEveryCommit` and `TestSchedule_EarliestMatchesBruteForceOracle` both went red — a burst of several near-simultaneous calls (all sharing one real "now") could have an EARLY member of the burst wrongly evicted merely because a LATER member's own granted instant landed far enough in the future to make the earlier grant look "expired" relative to that future instant, even though real time had not actually advanced. Fixed by separating `commit` (insert only, no eviction) from a new `evict(now)` (time-based, using the caller's real "now"), with `Limiter.acquire` calling `evict(now)` once per acquisition before deciding — never per commit. Re-ran both tests plus the full suite green afterward. This is a `Kind: validation` finding for the red-first methodology itself (the spawn's own framing: "a row that stays green is itself a finding") — here the STRUCTURAL tests (brute-force oracle, invariant-after-commit), not the seven named broken-variant rows, are what caught it, confirming the design's own point that an instant-list assertion is not enough and a checkable postcondition is what matters.

- **Step 9**: all gates PASS on the full tree — build, vet, `golangci-lint run` (0 issues), `golangci-lint fmt -d`, `go test -count=1`, `go test -race -count=1`, `go mod tidy` (no delta), and `make verify`. No panic-index addition (no `panic`/`log.Fatal` in new production code); no posting signature and no event-dictionary entry owed (this task moves no balance and adds no migration).

- **Step 9.5**: `context.md`'s § Status "no Telegram client" clause replaced with the transport plus the reason `cmd/bot` still constructs nothing; `context-status.md` entry appended with the literal `#TBD-at-Step-12` locator. No open question in `context.md` was resolved by this task. Propagation sweep for the removed claim found only this run's own spec and design, which quote it to define AC29 and become history surfaces at Step 12.

- **Step 11 (round 1)**: all 15 code/test findings fixed, each mutation-verified — the assertion was pointed at the broken state and seen RED before being trusted GREEN. Row 16 (per-AC table) done orchestrator-side. No finding required a Design or Spec Amendment: in every case the design was right and the code had diverged.

## Key discoveries (don't re-investigate)

- telego's stock `net/http` caller discards the HTTP status code and reports 5xx as an untyped error, so the §13.2 health numbers can only be observed by replacing `telegoapi.Caller`.
- `MultipartRequest` returns `BodyStream` (an `io.Pipe`) with `BodyRaw` nil, so a file upload's `chat_id` is unreachable at the caller seam — hence the three-state `ChatNone`/`ChatUnknown`/`ChatKnown`.
- `url.Error.Error()` prints the full URL, and the URL carries the bot token; `stripPassword` redacts only userinfo. Store `url.Error.Unwrap()` and run rendered strings through a token replacer.
- `net/http` retries a POST internally only when an `Idempotency-Key` header is present — never set one, or "never retry on doubt" is silently defeated.
- `testing/synctest` with a `net.Pipe` dialer replaces an injectable clock entirely; production code needs no clock abstraction.

## AC Status

| AC | Status | Verifying command / test |
|----|--------|--------------------------|
| AC1 | PASS | `TestNew_HappyPath`, `TestNew_ValidatesEachField`, `go vet ./internal/tg` |
| AC2 | PASS | `TestGuard_NoFastHTTPOrGoJSONImport` |
| AC3 | PASS | `grep telego go.mod` + `make tidy-check` |
| AC4 | PASS | `TestRetry_RetryAfterHonouredExactly` |
| AC5 | PASS | `TestRetry_DelaysGrowAndStayPositive`, `TestBackoffDelay_JitterBoundsExactly` |
| AC6 | PASS | `TestRetry_AmbiguousMakesExactlyOneAttempt` |
| AC7 | PASS | `TestRetry_RetryableCasesAreRetried` |
| AC8 | PASS | `TestRetry_GiveUpFieldsByField` |
| AC9 | PASS | `TestRetry_DeadlineRefusalInsteadOfSleep` |
| AC10 | PASS | `TestLimiter_PerClassGlobalAdmission`, `TestRedFirst_PaceOnly` |
| AC11 | PASS | `TestLimiter_CrossChatNonBlockingWithGlobalBounded`, `TestRedFirst_OrderedGlobal` |
| AC12 | PASS | `TestLimiter_BurstThenCapShape`, `TestRedFirst_BucketCap` |
| AC13 | PASS | `TestLimiter_UnboundedClassPassesThroughAndBoundCounterpartBinds` |
| AC14 | PASS | `TestLimiter_PrivateChatIsChargedLikeAnyOther` |
| AC15 | PASS | `TestGuard_BaseURLOnlyInConstructor` (source half) + `TestLimiter_IdenticalBehaviourAcrossBaseURLs` (rewritten in fix round 1 to use two real clients) |
| AC16 | PASS | `TestRetry_Observation` (exact `Retries`/`StatusCode`/`Latency` after fix round 1) |
| AC17 | PASS | `TestGuard_NoMetricsRegistryImportInTG` |
| AC18 | PASS | `TestRetry_CancellationAtEveryWaitingSite` |
| AC19 | PASS | `TestServer_*` (8 behaviours in `internal/tgtest`) |
| AC20 | PASS | `TestGuard_EndToEndViaConfigLoadProducedBaseURL` |
| AC21 | PASS | `TestGuard_NoRetryOrRateLimitLiteralAtCallSite` + `TestLoadTransport_*` |
| AC22 | PASS | `TestLoadTransport_AllAbsentYieldsDefaults`, `TestLoadTransport_Malformed` |
| AC23 | PASS | `TestLoadTransport_QueriesEveryKeyUnconditionally` + the existing `disjoint_test.go` equality |
| AC24 | PASS | existing `TestLoadEnv_RequiredVariableUnset`/`_Empty`, unchanged by this task |
| AC25 | PASS | `rg -U` propagation sweep re-run at Step 9.5; no live falsified claim remains |
| AC26 | PASS | `TestRetry_TokenAbsentFromRenderedError`, `TestSanitizeErr_UnwrapsURLErrorAndDropsURL` |
| AC27 | PASS | `TestGuard_RefusingGateBlocksTheAccessor`, `TestGuard_NoTelegoBotConstructionOutsideTG` |
| AC28 | PASS | `make verify` GREEN |
| AC29 | PASS | Step 9.5 sweep + `context.md`, `context-status.md`, `key-decisions.md`, `env.go`, `config.go`, `doc.go`, `.env.example`; `INDEX.md` at Step 12 |
| AC30 | PASS | `TestLimiter_SteadyOrderedEmission`, `TestRedFirst_QuotaOnly` |
| AC31 | PASS | `TestLimiter_SaturationEndsInEmissionOrError`, `TestLimiter_RefusalLeavesScheduleUnchanged`, `TestRedFirst_CommitThenUndo` |
| AC32 | PASS | `rg` for persistence in `internal/tg` non-test: none; no migration in the range |
| AC33 | PASS | `TestRetry_AttemptCap` |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-1 | round 1 | major | open | probe: build a `BodyStream` `RequestData` via `jsonConstructor{}.MultipartRequest`, drive `caller.Call` against a permanent 500 `tgtest` server, assert the handler saw exactly 1 request (measured: 3) |
| R1-2 | round 1 | major | open | `chatRefFromData(&ta.RequestData{BodyRaw: []byte("[1,2,3]")}).Target` and `chatRefFromData(&ta.RequestData{}).Target` must both be `ChatUnknown` (measured: `ChatNone` for both) |
| R1-3 | round 1 | major | open | replace `return ChatRef{Target: ChatUnknown}` (caller.go:231) with `ChatNone`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-4 | round 1 | minor | open | one 500 then a limiter wait past the ctx deadline → the single `Observation` must carry `Retries: 0` (measured: 1) |
| R1-5 | round 1 | minor | open | change all four `attempts-1` observe args to `attempts`, then `go test -count=1 ./internal/tg/ -run TestRetry_Observation` — must go RED (measured: GREEN) |
| R1-6 | round 1 | minor | open | delete the `errors.As(err, &uerr)` block in `sanitizeErr`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-7 | round 1 | minor | open | make `backoffDelay` return `d` instead of `half + jitter`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-8 | round 1 | minor | open | `sed -n '38,42p' internal/tg/client.go` vs `sed -n '199,204p' internal/tg/caller.go` |
| R1-9 | round 1 | minor | open | `sed -n '208,212p;238,254p' internal/tg/limit.go` — the comment says "only for bounded classes"; the code has no boundedness test |
| R1-10 | round 1 | minor | open | `grep -rn "evict" internal/tg/*_test.go` — currently only `TestSchedule_EvictIsRelativeToRealNow*`; no bounded-memory assertion |
| R1-11 | round 1 | minor | open | `grep -n "BaseURL" internal/tg/limit_test.go` → no hit, yet the test is named `IdenticalBehaviourAcrossBaseURLs` |
| R1-12 | round 1 | minor | open | `sed -n '286,300p' internal/tg/limit.go` vs design.md:737 |
| R1-13 | round 1 | minor | open | `sed -n '38,44p' internal/tg/constructor.go` |
| R1-14 | round 1 | minor | open | `sed -n '73,80p' internal/tg/client.go` vs `sed -n '37p' ai-docs/doc-convention.md` |
| R1-15 | round 1 | nit | open | `sed -n '320,330p' internal/tg/limit.go` vs design.md:825 |
| R1-16 | round 1 | nit | open | `sed -n '/^## AC Status/,/^## /p' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` |
| R1-17 | round 1 | nit | accepted@1 — design declares `TestGuard_TokenExposureSitesAreTheAcceptedOnes` informational; `TestRetry_TokenAbsentFromRenderedError` is the real AC26 check | `grep -n "t.Logf" internal/tg/guards_test.go` |
| R1-18 | round 1 | nit | accepted@1 — D11 establishes no other metrics registry is reachable from this module | `go mod why -m github.com/prometheus/client_golang` |
| R1-19 | round 1 | nit | accepted@1 — recorded open question, fail-safe direction chosen deliberately | `grep -n "sendChatAction" ai-docs/plans/2026-09-04-bot-api-transport.design.md` |
| R1-20 | round 1 | nit | accepted@1 — AGENTS.md's balance-constant rule is scoped to game balance (`docs/DESIGN.md` §16.5); D10/KD-27 make these overridable env keys | `sed -n '/^func defaultTransport/,/^}/p' internal/config/transport.go` |

## Self-Review (Round 1)

**Verdict:** REJECT

**What was checked.** All 33 ACs mapped to a named test or a re-run command; every design decision D1–D15 read against the shipped code; `make verify` re-run on the tree (GREEN, log kept) plus an uncached `go test -count=1 -race ./internal/tg/... ./internal/tgtest/... ./internal/config/...` (GREEN); the AC2 / AC15 / AC17 / AC21 / AC26 / AC32 guard scans re-run against the shipped artefact, not against drafting-time output; the AC25/AC29 propagation sweep re-run over the live tree; telego v1.11.2's real import paths and its whole exported `*Bot` surface audited for a caller bypass (none); 15 surgical mutations applied to the production sources and the suite re-run for each (10 killed, 5 survived); three targeted probes written, run and deleted (tree confirmed clean afterwards).

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/tg/caller.go:106 | major | **A multipart (`BodyStream`) request is retried.** Design D5 (`design.md:499,506`) states *"A multipart request is never retried … A `BodyStream` request therefore makes exactly one attempt"*. The loop's only exits are `!out.retryable` and the attempt cap; `data.BodyStream` is never consulted. Measured: a `BodyStream` request against a permanent 500 made **3 attempts**; attempts 2–3 re-read the already-drained `io.Pipe`, so a media send that meets one 429 silently degrades into a malformed request. Fix: treat a `BodyRaw == nil && BodyStream != nil` request as terminal after attempt 1, plus a test. | ✅ Fixed |
| 2 | internal/tg/caller.go:233,237 | major | **`chatRefFromData` fails open on both "no decodable body" branches.** D4's decision (`design.md:369`) and D12's table (`design.md:1149`) both classify *no decodable body* (`BodyRaw == nil`) as `ChatUnknown` — bounded by the reserved unknown-chat key, and **refused** by #22's future allowlist. The code returns `ChatNone` when `BodyRaw` fails to decode (:237) and when `BodyRaw == nil && BodyStream == nil` (:233). `ChatNone` means *no per-chat schedule at all* (`limit.go:239`) and *allow* at the gate — precisely the "a chat exempt from every per-chat window by construction" failure D4 was written to prevent. It also contradicts the function's own doc comment (`caller.go:224-227`, "`ChatNone` when the body decodes but carries no `chat_id`"). Measured: `chatRefFromData(&ta.RequestData{BodyRaw: []byte("[1,2,3]")}).Target` = `ChatNone`; `chatRefFromData(&ta.RequestData{}).Target` = `ChatNone`. | ✅ Fixed |
| 3 | internal/tg/caller.go:230 | major | **The `ChatUnknown` derivation has zero test coverage, and the suite cannot see its removal.** Collapsing `return ChatRef{Target: ChatUnknown}` into `ChatNone` leaves `go test ./internal/tg/` fully GREEN — i.e. every file-upload call could escape per-chat rate limiting and no test would notice. `TestLimiter_UndecodableBodyIsBoundedNotExempt` (`limit_test.go:89`) builds `ChatRef{Target: ChatUnknown}` by hand and never exercises the derivation; no test anywhere constructs a `BodyStream` request. The design's own decomposition names "the undecodable-body branch" as a subtask-4 scenario. Fix: a table test over `chatRefFromData` covering all three targets. | ✅ Fixed |
| 4 | internal/tg/caller.go:69,74 | minor | **`Observation.Retries` is overstated by one on the two limiter-side bail-outs.** Both pass `attempts`; the other four observation sites (`:82,:108,:126,:131`) pass `attempts-1`, and `observe.go:22-24` documents `Retries` as *"the number of attempts beyond the first"*. Measured: one 500, then a limiter wait outrunning the deadline → `{Method:getMe StatusCode:500 Retries:1}` after exactly **one** attempt. | ✅ Fixed |
| 5 | internal/tg/retry_test.go:453 | minor | **The loose assertion that let #4 ship, plus two unasserted AC16 fields.** `if got[2].Retries == 0 { … want Retries > 0 }` bounds the give-up retry count instead of pinning it; changing all four `attempts-1` sites to `attempts` leaves the suite GREEN. `TestRetry_Observation` also never asserts `StatusCode` or `Latency`, two of the five fields AC16 names. | ✅ Fixed |
| 6 | internal/tg/retry.go:117-123 | minor | **D8's primary token-leak defence is unprotected.** Deleting the `errors.As(err, &uerr)` unwrap entirely leaves the suite GREEN: `TestRetry_TokenAbsentFromRenderedError` asserts only `!strings.Contains(rendered, Token)`, which the `strings.Replacer` alone satisfies. Untested consequences: that the stored cause is the *unwrapped* value, and that the URL is dropped rather than merely scrubbed. Fix: a direct `sanitizeErr` unit test on `Unwrap()` identity and URL absence. | ✅ Fixed |
| 7 | internal/tg/retry.go:42 | minor | **D6's equal-jitter shape is unprotected.** Returning the full `d` (no jitter at all — removing D6's whole anti-thundering-herd purpose) leaves the suite GREEN; `TestRetry_DelaysGrowAndStayPositive` only bounds delays from below. Neither `backoffDelay` nor `jitter` appears in any `_test.go`. Fix: a direct test with `jitter()` pinned to 0 and to 1, asserting `d/2` and `~d`. | ✅ Fixed |
| 8 | internal/tg/client.go:38-42 | minor | **False doc comment.** *"Nil means `http.DefaultClient`'s transport settings are not assumed — a client is constructed with sane defaults."* `caller.httpClient()` (`caller.go:199-204`) returns `http.DefaultClient` verbatim; no client is constructed. | ✅ Fixed |
| 9 | internal/tg/limit.go:210 | minor | **False doc comment + an unbounded map.** *"per-chat schedules are … built lazily … only for bounded classes"*, but `chatScheduleLocked` (:238-254) has no boundedness test and inserts unconditionally, so with `EDIT`/`OTHER` at their shipped `off` defaults every distinct chat id still creates a permanent entry in a registry D9 says never evicts. (No grants accumulate — `commit` returns early — so the "no history" half holds.) | ✅ Fixed |
| 10 | internal/tg/limit.go:168 | minor | **`evict` has no test at all.** D9's time-based-retention and bounded-memory claim is unasserted; weakening the threshold to `now-2*maxPer`, or flipping `Before` to `!After`, both leave the suite GREEN. (Both are equivalent mutants for *scheduling*, so this is a coverage gap, not a latent scheduling bug — but a retention regression would be a memory leak the suite cannot see.) Fix: assert `len(schedule.grants)` stays bounded across many time-separated acquires. | ✅ Fixed |
| 11 | internal/tg/limit_test.go:246 | minor | **AC15's behavioural clause is discharged by an instrument that cannot fail it.** AC15 asks for *"a test asserts identical limiter behaviour for two different base URLs"*; `TestLimiter_IdenticalBehaviourAcrossBaseURLs` compares two limiters constructed from the same config with no base URL anywhere in it. The source guard `TestGuard_BaseURLOnlyInConstructor` is a complete cover for the structural half, so AC15 is not unmet — but this test proves determinism, not base-URL independence. | ✅ Fixed |
| 12 | internal/tg/limit.go:286-300 | minor | **Fixed-point exhaustion is a fallback, not a defect.** Design `:737`: *"An implementation bounds the passes and treats exhaustion as a defect, not as a fallback."* On exhaustion the loop falls through and commits at whatever `t` it reached — no error, no signal. Believed unreachable; the design asked for the opposite treatment. | ✅ Fixed |
| 13 | internal/tg/constructor.go:41 | minor | `_ = writer.Close()` drops the error that writes the closing multipart boundary, so a truncated body closes the pipe with `nil`. Fold `writer.Close()`'s error into the value passed to `pw.CloseWithError`. | ✅ Fixed |
| 14 | internal/tg/client.go:79 | minor | `Client` states no concurrency-safety contract, though it is the process-wide client whose whole one-mutex limiter exists to serialise concurrent calls. `ai-docs/doc-convention.md:37`: *"A function safe for concurrent use says so; the default assumption is that it is not."* A reader following that default would construct several Clients and defeat the global schedule. | ✅ Fixed |
| 15 | internal/tg/limit.go:325-328 | nit | `paceWindows` returns two identical windows when `N == 1`, so the shipped `_CHAT_RATE=1/1s` schedule holds `{(1,1s),(1,1s)}`. Design `:825`: *"When `N` is 1 the two windows coincide and the schedule holds one."* Behaviourally identical; the design sentence is false of the code. Cheapest fix is code-side (dedupe), not a design amendment. | ✅ Fixed |
| 16 | (this file) `## AC Status` | nit | 33 acceptance criteria collapsed into a single `AC1–AC33 \| PASS` row. Per-AC rows are what a later reader can re-check. | ✅ Fixed |

**No Design/Spec Amendment trigger.** Every finding above is a code or test fix: in each case the design document is correct and the implementation diverged from it. Findings 12 and 15 are the only two where a document edit is even conceivable, and both are cheaper to fix code-side.

**Gate results re-run against the shipped artefact (not quoted from drafting):**

- `make verify` → GATE-GREEN (fmt, build, vet, `golangci-lint run` 0 issues, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck).
- `go test -count=1 -race ./internal/tg/... ./internal/tgtest/... ./internal/config/...` → RACE-GREEN.
- AC2: `grep -rn "fasthttp\|go-json\|goccy\|grbit" --include=*.go cmd/ internal/ | grep -v _test.go` → one comment hit only. PASS. Guard strings verified against telego's real import paths (`github.com/valyala/fasthttp`, `github.com/grbit/go-json`, `github.com/valyala/fastjson`) — the guard is not a tautology.
- AC15 source half: `BaseURL` appears only in `internal/tg/client.go`. PASS.
- AC17: no metrics-registry import in `internal/tg`. PASS.
- D2 seam: `telego.NewBot` / `telego.With*` call sites exist only in `internal/tg/client.go`. PASS. telego's full exported `*Bot` surface audited — `Token`, `SecretToken`, `Logger`, `ID`, `Username`, `FileDownloadURL` are the only non-generated methods and none performs HTTP, so AC27's "no exported API lets a caller bypass" holds.
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2` direct; tidy-check clean. PASS.
- AC32: no `*.sql` and no migration in the range. PASS.
- AC25/AC29: propagation sweep over the live tree finds no surviving falsified claim; `env.go`, `config.go`, `doc.go`, `.env.example`, `context.md`, `context-status.md`, `key-decisions.md` KD-2/KD-27 all updated. `INDEX.md` is Step 12's by the design's own ownership table. PASS.
- Panic index: no `panic(` / `log.Fatal*` in any new production file — no row owed. PASS.
- File sizes: largest new file is `limit_test.go` at 693 lines (limit 1500); largest non-test is `limit.go` at 343 (limit 1000). PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `## Decisions log`) all present; `parent_skill` correctly omitted (`/task` is the parent flow); `entry_args` present. PASS.

**Recorded, not raised** (entered in the register as `accepted@1`): `TestGuard_TokenExposureSitesAreTheAcceptedOnes` carries no assertion — the design declares it informational and `TestRetry_TokenAbsentFromRenderedError` is the real AC26 check; `TestGuard_NoMetricsRegistryImportInTG` matches only `prometheus/client_golang` — D11 establishes no other registry is reachable from this module; `sendChatAction` staying in `ClassMessage` — a recorded open question with the fail-safe direction chosen; the transport defaults living as Go literals in `internal/config/defaultTransport()` — AGENTS.md's balance-constant rule is scoped to game balance (`docs/DESIGN.md` §16.5), and D10/KD-27 make these overridable environment keys.
