# Progress: Bot API transport over telego — ACTIVE
_Updated: 2026-09-04 17:50_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-bot-api-transport
**base_commit:** c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**Last build:** PASS
**Issue:** #19
**Spec:** ai-docs/plans/2026-09-04-bot-api-transport.spec.md
**current_step:** Step 11 — review fixes complete (Round 2)
**last_passed_gate:** make verify | 2026-09-04T19:38:24Z | fffa38b
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

- **Step 11 (round 2)**: both `major` rows were AC verifiers that could not fail — AC9's bounded a lower elapsed time instead of pinning the absence of a sleep, and AC22/AC23's compared `defaultTransport()` to itself, leaving the flood-ban-relevant defaults free to drift from both D10's table and `.env.example`. Every fix was mutation-verified. One finding (DOC-4 line-number citations) was introduced by fix round 1, so fix rounds get reviewed too.
- **Step 11 (round 2)**: `AttemptTimeout`'s test is real-time, not `testing/synctest` — a bubble deadlocks when `net/http`'s handler goroutine stays durably blocked in `time.Sleep`, which synctest treats as an unrecoverable deadlock rather than a leak.

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
| R1-1 | round 1 | major | fixed@fffa38b | probe: build a `BodyStream` `RequestData` via `jsonConstructor{}.MultipartRequest`, drive `caller.Call` against a permanent 500 `tgtest` server, assert the handler saw exactly 1 request (measured: 3) |
| R1-2 | round 1 | major | fixed@fffa38b | `chatRefFromData(&ta.RequestData{BodyRaw: []byte("[1,2,3]")}).Target` and `chatRefFromData(&ta.RequestData{}).Target` must both be `ChatUnknown` (measured: `ChatNone` for both) |
| R1-3 | round 1 | major | fixed@fffa38b | replace `return ChatRef{Target: ChatUnknown}` (caller.go:231) with `ChatNone`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-4 | round 1 | minor | fixed@fffa38b | one 500 then a limiter wait past the ctx deadline → the single `Observation` must carry `Retries: 0` (measured: 1) |
| R1-5 | round 1 | minor | fixed@fffa38b | change all four `attempts-1` observe args to `attempts`, then `go test -count=1 ./internal/tg/ -run TestRetry_Observation` — must go RED (measured: GREEN) |
| R1-6 | round 1 | minor | fixed@fffa38b | delete the `errors.As(err, &uerr)` block in `sanitizeErr`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-7 | round 1 | minor | fixed@fffa38b | make `backoffDelay` return `d` instead of `half + jitter`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R1-8 | round 1 | minor | fixed@fffa38b | `sed -n '38,42p' internal/tg/client.go` vs `sed -n '199,204p' internal/tg/caller.go` |
| R1-9 | round 1 | minor | fixed@fffa38b | `sed -n '208,212p;238,254p' internal/tg/limit.go` — the comment says "only for bounded classes"; the code has no boundedness test |
| R1-10 | round 1 | minor | fixed@fffa38b | `grep -rn "evict" internal/tg/*_test.go` — currently only `TestSchedule_EvictIsRelativeToRealNow*`; no bounded-memory assertion |
| R1-11 | round 1 | minor | fixed@fffa38b | `grep -n "BaseURL" internal/tg/limit_test.go` → no hit, yet the test is named `IdenticalBehaviourAcrossBaseURLs` |
| R1-12 | round 1 | minor | fixed@fffa38b | `sed -n '286,300p' internal/tg/limit.go` vs design.md:737 |
| R1-13 | round 1 | minor | fixed@fffa38b | `sed -n '38,44p' internal/tg/constructor.go` |
| R1-14 | round 1 | minor | fixed@fffa38b | `sed -n '73,80p' internal/tg/client.go` vs `sed -n '37p' ai-docs/doc-convention.md` |
| R1-15 | round 1 | nit | fixed@fffa38b | `sed -n '320,330p' internal/tg/limit.go` vs design.md:825 |
| R1-16 | round 1 | nit | fixed@fffa38b | `sed -n '/^## AC Status/,/^## /p' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` |
| R1-17 | round 1 | nit | accepted@1 — design declares `TestGuard_TokenExposureSitesAreTheAcceptedOnes` informational; `TestRetry_TokenAbsentFromRenderedError` is the real AC26 check | `grep -n "t.Logf" internal/tg/guards_test.go` |
| R1-18 | round 1 | nit | accepted@1 — D11 establishes no other metrics registry is reachable from this module | `go mod why -m github.com/prometheus/client_golang` |
| R1-19 | round 1 | nit | accepted@1 — recorded open question, fail-safe direction chosen deliberately | `grep -n "sendChatAction" ai-docs/plans/2026-09-04-bot-api-transport.design.md` |
| R1-20 | round 1 | nit | accepted@1 — AGENTS.md's balance-constant rule is scoped to game balance (`docs/DESIGN.md` §16.5); D10/KD-27 make these overridable env keys | `sed -n '/^func defaultTransport/,/^}/p' internal/config/transport.go` |
| R2-1 | round 2 | major | fixed@b1fdf70 | `perl -0777 -i -pe 's/hasDeadline && waitUntilTime\.After\(deadline\)/false && hasDeadline && waitUntilTime.After(deadline)/' internal/tg/caller.go`, then `go test -count=1 -run TestRetry_DeadlineRefusalInsteadOfSleep ./internal/tg/` — must go RED (measured: PASS; a scratch probe showed the call's elapsed going 0s → 5s and its cause going `retry after: 100` → `context deadline exceeded`) |
| R2-2 | round 2 | major | fixed@b1fdf70 | three independent edits, each must turn `go test -count=1 ./internal/config/ ./internal/tg/` RED (measured: GREEN for all three): `defaultTransport()` `RetryMaxAttempts: 3`→`5`; `defaultTransport()` `ChatRate: {1,1s}`→`{2,1s}`; `.env.example` `LAB_GAME_TG_RETRY_MAX_ATTEMPTS=3`→`99` and `..._MESSAGE_CHAT_RATE=1/1s`→`7/1s` |
| R2-3 | round 2 | minor | fixed@b1fdf70 | delete the `c.client.observe(method, time.Since(start), 0, false, 0)` line in the gate-refusal branch (caller.go:53), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R2-4 | round 2 | minor | fixed@b1fdf70 | disable the `AttemptTimeout` block (caller.go:172, `if to := …; false && to > 0`), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R2-5 | round 2 | minor | fixed@b1fdf70 | replace `req.Header.Set(ta.ContentTypeHeader, data.ContentType)` (caller.go:201) with `_ = data.ContentType`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R2-6 | round 2 | minor | fixed@b1fdf70 | `grep -rn '\.md:[0-9]' --include=*.go internal/ cmd/` — must return nothing (measured: 3 hits; `ai-docs/doc-convention.md` DOC-4 forbids line citations) |
| R2-7 | round 2 | minor | fixed@b1fdf70 | delete `sort.Strings(fieldNames)` from `writeMultipartBody` (constructor.go:68), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R2-8 | round 2 | nit | fixed@b1fdf70 | `grep -cn '^| R1-.* | fixed@fffa38b |' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` — must be 0 once the fixer marks them (measured: 16 rows still `open` although round 1's table marks all 16 `✅ Fixed`) |
| R2-9 | round 2 | minor | accepted@2 — the AC21 guard's regex is narrower than the design's Test Design sentence (it matches only `<digits> * time.Unit`, not a window count or an attempt count), but neither escape is live: a literal `attempts >= 3` and a literal `count: 30` are both killed by the functional suite | mutate caller.go's `attempts >= c.client.transport.RetryMaxAttempts` → `>= 3` and limit.go's `count: r.Count` → `count: 30`, then `go test -count=1 ./internal/tg/` |
| R2-10 | round 2 | nit | accepted@2 — `http.Server.Serve` returns `http.ErrServerClosed` unwrapped, so the `!=` comparison is sound; `errorlint` is enabled in `.golangci.yml` and `golangci-lint run` reports 0 issues on the shipped tree | `sed -n '81p' internal/tgtest/tgtest.go` + `golangci-lint run` |
| R2-11 | round 2 | nit | accepted@2 — a derived `ChatUnknown` always carries an empty `Key`, so splitting the reserved unknown-chat key per call is a semantically equivalent mutant, not a live escape from D4's "single reserved key" | mutate `key = chatKey{key: unknownChatKey + call.Chat.Key, …}`, then `go test -count=1 ./internal/tg/` |
| R2-12 | round 2 | minor | accepted@2 — an over-restriction defect IS visible: widening every window by 10% goes RED, so AC30's `>= 1s` spacing assertion is not the only instrument on that axis | mutate `at(k).Sub(at(j)) < w.per` → `< w.per+w.per/10` in `schedule.holds`, then `go test -count=1 ./internal/tg/` |
| R3-1 | round 3 | major | open | `sed -i 's/newSchedule(orderedSchedule, windows)/newSchedule(unorderedSchedule, windows)/' internal/tg/limit.go` (limit.go:260), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN). Not an equivalent mutant under the SHIPPED chat windows `[{1,1s},{20,1m}]`: with grants at 0s and 10s, `earliest(1s)` = **11s** ordered vs **1s** unordered |
| R3-2 | round 3 | minor | open | `New(Options{BaseURL: …, Token: …, Transport: {RetryMaxAttempts: 3, RetryBaseDelay: 500ms, RetryMaxDelay: 100ms, AttemptTimeout: 30s}})` must return an `*OptionError` naming `Transport.RetryMaxDelay` (measured: nil error; `client.go:112` tests only `<= 0`) |
| R3-3 | round 3 | nit | open | `git show fffa38b:internal/tg/retry_test.go` \| `grep -c AttemptTimeoutAbandonsAttempt` → 0, and `git log --oneline -S 'fixed@fffa38b' -- ai-docs/plans/2026-09-04-bot-api-transport.progress.md` → `b1fdf70` — so `fixed@fffa38b` is false for rows R2-1…R2-8 |
| R3-4 | round 3 | nit | open | `grep -n '^## Status' ai-docs/context.md` → `## Status (2026-09-02)` while `git diff c1d03fd..HEAD -- ai-docs/context.md` rewrites that section's body |
| R3-5 | round 3 | major | accepted@3 — RETRACTED after verifying on the production path: telego's `constructAndCallRequest` defers `closer.Close()` on `data.BodyStream` around `b.api.Call` (telego@v1.11.2:bot.go:232), so every early return in our `Call` (gate refusal, limiter refusal, pre-attempt cancellation) still unblocks the multipart writer goroutine. The 20/20 and 9/9 permanent leaks measured first were an artefact of driving the unexported `caller.Call` directly, which no production path does | 20 `SendPhoto` calls through `Client.API()` under a refusing gate + 10 under a limiter deadline refusal, then `runtime.Stack(all=true)` grepped for `writeMultipartBody` (measured: before=0 after=0 in both) |
| R3-6 | round 3 | nit | accepted@3 — design D9 predicts this survivor ("ceil rather than truncation buys evenness only — the quota window is what forbids the N+1-th emission either way"), and the Test Design says AC10(iii) is explicitly **not** red-first against a merely-truncated pacing interval | `sed -i 's\|return (w + time.Duration(n) - 1) / time.Duration(n)\|return w / time.Duration(n)\|' internal/tg/limit.go`, then `go test -count=1 ./internal/tg/` (measured: GREEN, as designed) |
| R3-7 | round 3 | nit | accepted@3 — below severity floor: the Test Design's subtask-2 line "the server records the method path and body it received" is discharged consumer-side by handler closures (`TestCaller_RequestCarriesContentTypeHeader`, `TestCaller_DerivesChatKnownFromSendMessage`), not by a recorder inside `tgtest`; D13's own behaviour list requires none | `grep -n 'records the method path' ai-docs/plans/2026-09-04-bot-api-transport.design.md` + `grep -n 'Recorded\|record' internal/tgtest/tgtest.go` |

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

## Self-Review (Round 2)

**Verdict:** REJECT

**What was checked.** Diff `c1d03fd..HEAD` (32 files, 11 commits), with the round-1 fix
commit `fffa38b` re-examined line by line per instruction 7a. All 16 `⬜ Open` round-1
register rows re-verified by re-running each row's own verifying command as a mutation
against the shipped tree — **all 16 confirmed genuinely fixed, none re-opened**: the eleven
that are mechanically checkable (R1-1 multipart-never-retried, R1-2/R1-3 both
`chatRefFromData` fail-open branches, R1-4/R1-5 `observedRetries`, R1-6 `sanitizeErr`'s
`*url.Error` unwrap, R1-7 `backoffDelay`'s jitter, R1-9 the unbounded-class per-chat
allocation, R1-10 `evict`'s threshold and its `Before` boundary, R1-12 fixed-point
exhaustion, R1-15 `paceWindows` dedupe) were each killed by a named test; R1-8, R1-11,
R1-13, R1-14 and R1-16 were verified by reading the shipped text.

Beyond the fix round: every AC1–AC33 row re-derived against the shipped artefact; D1–D15
read against the code; `make verify` re-run (GATE-GREEN, log kept) plus an uncached
`go test -count=1 -race ./internal/tg/... ./internal/tgtest/... ./internal/config/...`
(RACE-GREEN); the AC2 / AC3 / AC15 / AC17 / AC26 / AC32 / D2-seam scans re-run as shell
greps against the post-implementation tree, not quoted from drafting; the panicking-call
audit run over all 15 changed non-test `.go` files (no hits); context discipline, `_ = err`,
`context.Background()` and `//nolint` sweeps run over the same set; file sizes measured
(largest non-test `internal/tg/limit.go` 376, largest test `internal/tg/limit_test.go` 795 —
both inside their hard bands); progress-file required fields confirmed present with
`parent_skill` correctly omitted. **A 38-mutation sweep was run across `caller.go`,
`retry.go`, `limit.go`, `client.go`, `class.go`, `constructor.go`, `errors.go`,
`internal/config/transport.go` and `.env.example`; 29 mutants were killed and 9 survived.**
Of the 9 survivors, one is semantically equivalent (recorded as R2-11) and three are one
finding (the two `defaultTransport()` edits and the `.env.example` edit, finding 2 below);
the rest are findings 1, 3, 4, 5 and 7. Findings 6 and 8 come from source and progress-file
reads, not from the sweep. The tree was restored from a `cp` backup after every mutation and
`git status --porcelain` confirmed clean at the end of each batch.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/tg/caller.go:140 · internal/tg/retry_test.go:251 | major | **AC9's named verifier is cosmetic — the guard clause it is named for can be deleted with the suite green.** AC9 requires the call to return the typed error *"immediately instead of sleeping"*. Disabling the pre-attempt deadline check (`hasDeadline && waitUntilTime.After(deadline)` → `false && …`) leaves `TestRetry_DeadlineRefusalInsteadOfSleep` **PASS**, because its only timing assertion is the bound `elapsed >= 100*time.Second`. Measured with a scratch probe: correct code returns at `elapsed=0s` with the cause `429 … retry after: 100`; the mutant returns at `elapsed=5s` (it sleeps the whole remaining deadline) with the cause `context deadline exceeded`. Both distinguishing properties — the immediacy and the retained 429 cause — are unasserted. Fix: pin the elapsed time (`elapsed != 0` under `synctest` is a failure) and assert `tgErr.Err` is not a context error. | ✅ Fixed |
| 2 | internal/config/transport_test.go:14-17 | major | **AC22's and AC23's "documented default" clauses are discharged by a tautology.** `TestLoadTransport_AllAbsentYieldsDefaults` asserts `*tr == defaultTransport()` — an assertion that passes for every possible value of `defaultTransport()` (self-review checklist § 3: *"no assertion that passes for every plausible output"*). Nothing anywhere pins the values D10's Keys-and-defaults table states, and nothing ties `.env.example` to them. Measured, each independently leaving `go test -count=1 ./internal/config/ ./internal/tg/` **GREEN**: `RetryMaxAttempts: 3`→`5`; `ChatRate: {1,1s}`→`{2,1s}`; and `.env.example` rewritten to `LAB_GAME_TG_RETRY_MAX_ATTEMPTS=99` / `..._MESSAGE_CHAT_RATE=7/1s`. So the two flood-ban-relevant per-chat defaults (`1/1s`, `20/1m` — `ai-docs/domain-invariants.md` § 6) can drift from both the design table and the file operators copy, silently. Fix: parse `.env.example` with the existing `readEnvExampleKeys` helper, feed its `LAB_GAME_TG_*` values through `loadTransport`, and assert the result equals `defaultTransport()` — one test that pins AC22's "documented default for each" and AC23's "with its default" at once. | ✅ Fixed |
| 3 | internal/tg/caller.go:53 | minor | **D11's gate-refusal observation is unprotected.** D11: *"The point fires exactly once per outbound call — including a gate refusal, so #23 sees refusals too"*, and `retry.go:141-143` repeats it. Deleting the `observe` call in the gate-refusal branch leaves `go test -count=1 ./internal/tg/` GREEN — `TestGuard_RefusingGateBlocksTheAccessor` asserts only that the call errors. Fix: give that test an `Observer` and assert one `Observation{Retries: 0, StatusCode: 0}` arrives. | ✅ Fixed |
| 4 | internal/tg/caller.go:172 | minor | **`AttemptTimeout` has zero coverage.** Disabling the whole `context.WithTimeout` block leaves the suite GREEN. It is an owner-DECIDED addition (design § Open questions: *"keep it, default 30s, as designed"*), motivated by `docs/DESIGN.md` §12.2's wedged-instance incident — a safety valve whose entire purpose is one case that no test reaches. Fix: a `synctest` test with a short `AttemptTimeout` against `tgtest.Delayed`, asserting the attempt is abandoned at the timeout while the caller's own context is still live. | ✅ Fixed |
| 5 | internal/tg/caller.go:201 | minor | **The request's `Content-Type` header is unasserted.** Replacing `req.Header.Set(ta.ContentTypeHeader, data.ContentType)` with `_ = data.ContentType` leaves the suite GREEN: `tgtest`'s handlers never read the request's headers, so a JSON body sent with no content type would pass every test here and be rejected by a real Bot API server. Fix: have one `tgtest` handler assert `r.Header.Get("Content-Type")`. | ✅ Fixed |
| 6 | internal/tg/limit.go:310,350 · internal/tg/limit_test.go:740 | minor | **Line-number design citations, forbidden by `ai-docs/doc-convention.md` DOC-4** (*"Cite the section number, never a line number: the design document is edited, and `§2.2.4` survives what `:118` does not"*). The fix round introduced `design.md:737` and `design.md:825` into two production doc comments and one test comment. Both resolve correctly today, but they carry no path and no commit pin, and the design doc moves to `ai-docs/plans/done/` at Step 12. Fix: cite by decision id — D9's fixed-point paragraph and D9's pacing-windows paragraph. | ✅ Fixed |
| 7 | internal/tg/constructor.go:35,63 | minor | **`MultipartRequest` and `writeMultipartBody` are untested.** `constructor_test.go` covers only `multipartCloseErr`; no test ever calls `MultipartRequest`, so deleting `sort.Strings(fieldNames)` — the line whose own comment claims determinism, *"Field order is sorted for determinism"* — leaves the suite GREEN. The path also now carries D5's load-bearing property: it is what sets `BodyStream`, which is what makes a request non-retryable, and `TestCaller_MultipartRequestNeverRetried` builds its `RequestData` by hand instead of through this constructor. Fix: one test that calls `MultipartRequest`, drains the pipe, and asserts the field order and the parsed parts. | ✅ Fixed |
| 8 | (this file) `## Review register` | nit | Rows R1-1 … R1-16 still read `open` although round 1's own table marks all 16 `✅ Fixed`. Per `ai-docs/templates/progress-format.md` the fixer owns the `fixed@<sha>` marker, and instruction 7a scopes the next round on this column — leaving them `open` invites a re-litigation of 16 closed findings. All 16 are independently verified fixed above; the fixer should set `fixed@fffa38b`. | ✅ Fixed |

**No Design/Spec Amendment trigger.** Findings 1–5 and 7 are test additions; finding 6 is a
code-comment edit; finding 8 is a progress-file field. In every case the spec and the design
say the right thing and the shipped artefact is what falls short of them — no criterion has
changed and no design decision is contradicted. Finding 6 is a **locator drift in the
opposite direction** (a correct citation carried in the wrong form), which the
locator-drift carve-out covers explicitly: it is fixed in the code comment, never by
amending the design.

**Gate results re-run against the shipped artefact:**

- `make verify` → GATE-GREEN (fmt, build, vet, `golangci-lint run` 0 issues, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck).
- `go test -count=1 -race ./internal/tg/... ./internal/tgtest/... ./internal/config/...` → RACE-GREEN.
- AC2: `grep -rn "fasthttp|go-json|goccy|grbit|fastjson" --include=*.go cmd/ internal/ | grep -v _test.go` → one comment hit in `client.go:136` only. PASS.
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2`, direct; tidy-check clean. PASS.
- AC15 source half: `BaseURL` appears in exactly one non-test file, `internal/tg/client.go`. PASS.
- AC17: no metrics-registry reference in any `internal/tg` non-test file. PASS.
- AC26: no `api_id`/`api_hash` and no real-token-shaped literal in any added line; `Idempotency-Key` appears nowhere in the module (D5's corollary holds structurally). PASS.
- AC32: no `*.sql` and no migration file in the range. PASS.
- D2 seam: `telego.NewBot` / `telego.With*` call sites exist only in `internal/tg/client.go`. PASS.
- Panicking-call audit over all 15 changed non-test `.go` files → no `panic(` / `log.Fatal*` / `log.Panic*`. No panic-index row owed. PASS.
- Context discipline: `ctx context.Context` first on `Call`, `doAttempt`, `waitUntil`, `Gate.AllowCall`, `Server.DialContext`; no struct field of type `context.Context`; no `context.Background()` in production. PASS.
- File sizes: `internal/tg/limit.go` 376 / hard 1000; `internal/tg/limit_test.go` 795 / hard 1500. PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `## Decisions log`) present; `entry_args` present; `parent_skill` correctly omitted (`/task` is the parent flow). PASS.

**Recorded, not raised** (entered in the register as `accepted@2` — R2-9 … R2-12): the AC21
literal guard's regex is narrower than the design's Test Design sentence, but neither escape
it misses is live (both a literal attempt cap and a literal window count are killed by the
functional suite); `internal/tgtest/tgtest.go:81`'s `err != http.ErrServerClosed` compares an
unwrapped sentinel and `errorlint` is enabled and clean; the reserved `unknownChatKey`'s
per-call split is a semantically equivalent mutant because a derived `ChatUnknown` always
carries an empty `Key`; and AC30's `>= 1s` spacing bound is not the only instrument on the
over-restriction axis — widening every window by 10% goes RED.

## Self-Review (Round 3)

**Verdict:** REJECT

**What was checked.** Diff `c1d03fd..HEAD` (32 files, 12 commits). Per instruction 7a the
register scoped this round: every row carries `fixed@fffa38b`, so the re-examination window
is `fffa38b..HEAD` — the round-2 fix commit `b1fdf70`, read line by line — plus anything
newly introduced. **All eight `⬜ Open` round-2 rows re-verified by re-running each row's own
verifying command as a mutation against the shipped tree; all eight confirmed genuinely
fixed, none re-opened:** R2-1 (`false && hasDeadline && …` → RED, `elapsed = 5s` and
`Err = context deadline exceeded` both now asserted), R2-2 (all three drift edits —
`RetryMaxAttempts: 3→5`, `ChatRate: {1,1s}→{2,1s}`, and `.env.example` rewritten to
`…RETRY_MAX_ATTEMPTS=99` / `…MESSAGE_CHAT_RATE=7/1s` — each killed by
`TestLoadTransport_ExampleMatchesDefaults`), R2-3 (gate-refusal `observe` deleted → RED),
R2-4 (`AttemptTimeout` block disabled → RED), R2-5 (`Content-Type` unset → RED), R2-6
(`grep -rn '\.md:[0-9]' --include=*.go internal/ cmd/` → no hits), R2-7 (both
`sort.Strings` calls deleted → RED, one at a time), R2-8 (all sixteen R1 rows now carry a
`fixed@` marker).

Beyond the fix round: every AC1–AC33 row re-derived against the shipped artefact; D1–D15 read
against the code; `make verify` re-run (GATE-GREEN) plus `go test -race -count=5` over
`./internal/tg/ ./internal/tgtest/ ./internal/config/` (GREEN — the fix round's one real-time
test, `TestCaller_AttemptTimeoutAbandonsAttempt`, is stable across five repeats); the
AC2 / AC3 / AC15 / AC17 / AC25 / AC29 / AC32 and D2-seam scans re-run as shell greps against
the post-implementation tree; the panicking-call audit, `_ = err`, `context.Background()`,
`//nolint`, `TODO` and file-size sweeps run over the whole changed set (all clean; largest
non-test `internal/tg/limit.go` 377 / hard 1000, largest test `internal/tg/limit_test.go`
796 / hard 1500).

**Four guards were themselves put to the test** (AGENTS.md § Patterns 2 — a green instrument
is a claim about the instrument): a planted `"github.com/valyala/fasthttp` string, a planted
`prometheus/client_golang` string, a planted `500 * time.Millisecond` literal and a planted
`BaseURL` reference were each added to a production file in `internal/tg`, and each guard went
RED. All four are load-bearing, not tautologies.

**Two claims the fix round asserts in prose were re-derived rather than read.**
`TestCaller_AttemptTimeoutAbandonsAttempt`'s comment (and the Decisions-log entry beside it)
says a `synctest` bubble deadlock-panics because `tgtest`'s handler goroutine stays blocked in
`time.Sleep` — a scratch `synctest` version of that exact test was written and run: it
produced `panic: deadlock: main bubble goroutine has exited but blocked goroutines remain`
with `goroutine 18 [sleep (durable), synctest bubble 1]` inside `tgtest.Delayed`. The comment
is accurate and the real-time choice is justified. And telego v1.11.2's whole exported `*Bot`
surface was re-audited for an AC27 bypass — `Token`, `SecretToken`, `ID`, `Username`,
`FileDownloadURL`, `UpdatesViaLongPolling`, `UpdatesViaWebhook` are the only non-generated
methods, and the two update methods reach the network only through `b.api.Call`.

**A twelve-mutation sweep was run on paths the earlier rounds' sweeps did not reach**
(the per-chat/global schedule wiring, `quotaWindows`, `ceilDiv`, the per-chat `commit`, the
reserved unknown-chat key, `New`'s validation ladder). Nine mutants were killed; three
survived — one is finding 1 below, one is predicted by design D9 (`R3-6`), and one is the
already-`accepted@2` equivalent mutant `R2-11`.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/tg/limit.go:260 · internal/tg/limit_test.go:184 | major | **AC30's arrival-order clause is discharged by an instrument that cannot fail it.** AC30 requires that calls "leave one interval apart, **in the order they arrived**". The property rests entirely on `chatScheduleLocked` building the per-chat schedule as `orderedSchedule` (limit.go:260) — `earliest`'s clamp to the key's own newest grant. Flipping that one word to `unorderedSchedule` leaves `go test -count=1 ./internal/tg/` **fully GREEN**. It is not an equivalent mutant: under the *shipped* chat windows (`ChatRate 1/1s` → `{1,1s}`, `ChatCap 20/1m` → `{20,1m}`), with grants already at 0s and 10s, `earliest(1s)` returns **11s** ordered and **1s** unordered — a later-arriving call jumps ahead of an already-granted one. `TestLimiter_SteadyOrderedEmission` cannot see it because it configures `ChatRate` alone, and with the single deduped `{1,1s}` window the two kinds coincide; `TestSchedule_OrderedEmissionIsNonDecreasing` tests the schedule type with a **hand-built** `orderedSchedule` and never exercises the wiring. Every other `orderedSchedule` in the test files is likewise hand-built. Fix: one limiter-level test with both shipped chat windows configured, whose acquires are separated in real "now" so a hole opens, asserting the grant sequence is non-decreasing. | ⬜ Open |
| 2 | internal/tg/client.go:112 | minor | **`New` does not reject a max delay below the base, which the design names as one of its validation cases.** Design § Test Design, subtask 3: "`New` rejects a zero attempt count, a non-positive base delay, **a max delay below the base**, and an empty base URL, each naming the field". `client.go:106-117` tests `RetryMaxAttempts < 1`, `RetryBaseDelay <= 0`, `RetryMaxDelay <= 0` and `AttemptTimeout <= 0` — there is no cross-field check, and `TestNew_ValidatesEachField`'s nine cases contain none. Nothing in `loadTransport` catches it either, since it is a cross-key inconsistency rather than a per-key one. Consequence: `LAB_GAME_TG_RETRY_MAX_DELAY=100ms` with `LAB_GAME_TG_RETRY_BASE_DELAY=500ms` starts cleanly and silently yields every backoff in `[50ms, 100ms)` — shorter than the configured base, and flat rather than growing, which is the half of AC5 that says "successive delays … grow rather than repeat". The flood-ban-relevant half (strictly positive, no tight loop) still holds, which is why this is `minor` and not `major`. Fix: the cross-field check plus its `TestNew_ValidatesEachField` row. | ⬜ Open |
| 3 | (this file) `## Review register` | nit | **Rows R2-1 … R2-8 carry `fixed@fffa38b`, but those fixes landed in `b1fdf70`.** `fffa38b` is round 1's fix commit and predates every round-2 fix: `git show fffa38b:internal/tg/retry_test.go` contains no `AttemptTimeoutAbandonsAttempt` and no `elapsed != 0`, and `git log -S 'fixed@fffa38b'` on this file returns `b1fdf70` — the commit that both applied the fixes and wrote the (wrong) sha. Instruction 7a scopes the next round on that sha, so the error is in the safe direction (a wider window), which is why it is a `nit` and not higher. The eight rows should read `fixed@b1fdf70`; the sixteen `R1-*` rows are correct as they stand. | ⬜ Open |
| 4 | ai-docs/context.md:40 | nit | **`## Status (2026-09-02)` is stale by its own diff.** This change rewrites that section's body — the "no Telegram client" clause became the transport plus `internal/tgtest` — but leaves the date in the heading. A reader who trusts the heading dates the transport claim two days before the transport existed. Fix: `## Status (2026-09-04)`. | ⬜ Open |

**No Design/Spec Amendment trigger.** Findings 1 and 2 are a test addition and a validation
plus its test row: in both cases the design says the right thing and the shipped artefact
falls short of it — no criterion has changed and no design decision is contradicted. Findings
3 and 4 are a progress-file field and a documentation heading.

**Gate results re-run against the shipped artefact (not quoted from drafting):**

- `make verify` → GATE-GREEN (fmt, build, vet, `golangci-lint run` 0 issues, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck).
- `go test -race -count=5 ./internal/tg/ ./internal/tgtest/ ./internal/config/` → REPEAT-GREEN.
- AC2: `grep -rnE 'fasthttp|go-json|goccy|grbit|fastjson' --include=*.go cmd/ internal/` minus tests → one comment hit at `client.go:136`. PASS. Guard verified live against a planted import.
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2`, direct; tidy-check clean. PASS.
- AC15 source half: `BaseURL` appears in exactly one non-test file, `internal/tg/client.go`. PASS. Guard verified live against a planted reference.
- AC17: no metrics-registry reference in any `internal/tg` non-test file. PASS. Guard verified live.
- AC21 literal scan: guard verified live against a planted `500 * time.Millisecond`. PASS.
- AC25/AC29: the requiredness sweep over the live tree finds `internal/config/env.go:13` correctly narrowed ("Every variable declared here is required … the Bot API transport's tuning variables (transport.go) are a separate, optional-with-default class"); `context.md`, `context-status.md`, `key-decisions.md` KD-27, `doc.go`, `config.go`, `.env.example` all updated. `INDEX.md` remains Step 12's by the design's ownership table. PASS (with finding 4's heading nit).
- AC26: no `api_id`/`api_hash` and no real-token-shaped literal in any added line. PASS.
- AC27: telego's full exported `*Bot` surface re-audited — no non-generated method reaches the network outside `b.api.Call`. PASS.
- AC32: no `*.sql` and no migration file in the range. PASS.
- D2 seam: `telego.NewBot` / `telego.With*` call sites exist only in `internal/tg/client.go`. PASS.
- Panicking-call audit over all 15 changed non-test `.go` files → no `panic(` / `log.Fatal*` / `log.Panic*`. No panic-index row owed. PASS.
- Error handling / context discipline: no `_ = err`, no `context.Background()` in production, no `TODO`, three `//nolint` directives all with a specific linter and a stated reason. `ctx context.Context` first on every outbound-path function; no struct field of type `context.Context`. PASS.
- File sizes: `internal/tg/limit.go` 377 / hard 1000; `internal/tg/limit_test.go` 796 / hard 1500. PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `entry_args`, `## Decisions log`) present; `parent_skill` correctly omitted (`/task` is the parent flow). PASS.

**Recorded, not raised** (entered in the register as `accepted@3` — R3-5 … R3-7): a suspected
permanent goroutine leak on every early return of `caller.Call` for a multipart request —
**raised, probed, and retracted** once it was driven through the production path instead of
through the unexported `caller.Call` directly, because telego's own
`constructAndCallRequest` defers `closer.Close()` on `data.BodyStream` around `b.api.Call`;
`ceilDiv`'s truncation mutant surviving, which design D9 predicts in as many words; and
`tgtest` not itself recording the request path and body, which consumer-side handler closures
discharge and D13's behaviour list never asks for.
