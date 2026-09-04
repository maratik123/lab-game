# Progress: Bot API transport over telego — ACTIVE
_Updated: 2026-09-04 17:50_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-bot-api-transport
**base_commit:** c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**Last build:** PASS
**Issue:** #19
**Spec:** ai-docs/plans/2026-09-04-bot-api-transport.spec.md
**current_step:** Step 11 — review fixes complete (Round 6)
**last_passed_gate:** make verify | 2026-09-04T21:32:28Z | 2d93871
**entry_args:** 19

## Next action

**Do this immediately:** Step 10 — re-spawn `self-review` COLD on the fix-round diff, then Step 12.

## Subtasks

- [x] 1. `internal/config`: optional-with-default transport key class, `.env.example`, falsified doc comments — commit c79c12b
- [x] 2. `internal/tgtest`: in-process fake Bot API server — pipe-backed `net.Listener`/`http.Server`, no real socket, `.invalid` base URL, fake token, `Success`/`TooManyRequests`/`ServerError`/`Delayed`/`FailNextDial`/`CloseWithoutResponse` behaviours, 8 tests all green under `-race`
- [x] 3. `internal/tg` foundations + the telego dependency — `go get github.com/mymmrac/telego@v1.11.2` (direct requirement, go.mod/go.sum confirmed idempotent under a second `go mod tidy`); `Error`/`OptionError` (D8), `Observation`/`Observer` (D11), `MethodClass`+classifier (D4, tested against telego's own real method names incl. trap cases), `ChatTarget`/`ChatRef`/`Call`/`Gate` (D12), `Options`/`New`/`Client.API()` (D2) — `New` wires only `WithAPIServer` + `WithLogger`/`WithDiscardLogger` so far; `WithAPICaller`/`WithRequestConstructor` wiring is subtask 4's job per the design's own decomposition (client.go is edited again there)
- [x] 4. `internal/tg` limiter (D9) + minimal caller — `limit.go` (`schedule`/`Limiter`, decide-then-commit, ordered/unordered kinds, time-based `evict`), `constructor.go` (`encoding/json` `RequestConstructor`), `caller.go` (derives method+`ChatRef` from URL/body, gate, limiter acquire+wait, one HTTP attempt), `client.go` wires `WithAPICaller`/`WithRequestConstructor`. **All 7 of the binding red-first broken-variant table's rows implemented and run RED-then-GREEN** (`TestRedFirst_CommitAtCandidate`, `_CommitThenUndo`, `_BucketCap`, `_OrderedGlobal`, `_CountRetention`, `_PaceOnly`, `_QuotaOnly` — none stayed green as a broken variant). Plus `TestSchedule_*` (brute-force oracle over 200 randomized trials, invariant-after-every-commit, ordered non-decreasing, unbounded-contributes-no-window, time-based retention, real-now-vs-future-grant eviction), `TestLimiter_*` (AC10–AC15, AC30–AC33 functional scenarios), and `TestCaller_*` end-to-end through real telego + tgtest (success, `ChatKnown` derivation from a real `SendMessage`, gate refusal costs zero attempts, limiter delay honours a short context deadline).
- [x] 5. `internal/tg` caller: retry loop, backoff, `retry_after`, typed error, observation — `retry.go` (`defaultJitter` with the `//nolint:gosec // G404` directive, `backoffDelay` per D6's exact formula, `classifyAttempt` per D5's table, `sanitizedError`/`sanitizeErr` dropping the `*url.Error` layer plus a token-replacer defense-in-depth per D8, `Client.giveUpError`/`observe`); `caller.go`'s `Call` rewritten into the full per-attempt-charging loop (limiter `acquire` moved inside the loop, `httptrace.WroteRequest` sticky `atomic.Bool` wired into `doAttempt`). All of `retry_test.go`'s AC4/AC5/AC6/AC7/AC8/AC9/AC16/AC18/AC26/AC33 scenarios pass under `testing/synctest` (D14) — virtual-clock retry_after/backoff/deadline/cancellation waits complete in ~0.01s wall time despite simulating hours. `golangci-lint run` caught two real style defects (an `if`/`else if` chain gocritic wanted as a `switch`, and QF1008 promoted-field simplifications on `resp.Error.X`) — both fixed.
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

- **Step 11 (round 3)**: AC30's arrival-order property hung on one word (`orderedSchedule` in `chatScheduleLocked`) that no test exercised — the existing pair missed it for two different reasons, one configuring a case where both schedule kinds coincide and the other hand-building the schedule so the wiring is never chosen. The new test drives `Limiter` on a configuration where they diverge. Adding the max-below-base validation also surfaced a pre-existing test that set an hour-long base delay against the default 30s cap purely to force a cancellable wait.

- **Step 11 (round 4)**: the owner widened the charter from the named findings to the CLASS after four consecutive rounds produced the same shape. The per-test verdicts are recorded under `## Class sweep (round 4)` below — an earlier version of this bullet called the sweep "inspectable rather than a claim" while the enumeration existed only in a delegate's return message and nowhere in the tree, which is the same shape the sweep was hunting.
- **Step 11 (round 4)**: `TestNew_InstallsPackageJSONConstructor` reads telego's unexported `constructor` field through `reflect` + `unsafe` because telego exposes no accessor and the two codecs' output bytes are equivalent — only the installed seam distinguishes them. Read-only, test-only, and it fails with a diagnostic naming telego's layout change rather than silently.

- **Step 11 (round 6)**: two rows of my own `## Class sweep (round 4)` table attributed doc comments to tests that carry none — I transcribed a delegate's wording without opening the files, which is the same unverified-claim shape the sweep was hunting. Corrected in place and the sweep's own miss (`TestBackoffDelay_JitterBoundsExactly`) recorded in the table rather than left out of it.
- **Step 11 (round 6) — OBJECTED, not fixed**: the nit asking for an `**at:**` field on the second `ai-docs/learnings.md` entry cannot be actioned. `AGENTS.md` § Learning Log Boundary rule 1 makes the log append-only — no existing entry may be edited — and an entry *about* another entry "belongs in neither log", with the `Superseded by:` field reserved to the `self-improve` / `learnings-escalation-audit` subagents. Adding the field would violate the boundary rule; writing a new entry to carry it is the disallowed shape. The count in question ("two instances in one run") describes the entry's own narrative, not a measurement over a corpus the task was mutating, which is the case the `at:` requirement exists for.

- **Step 11 (round 6)**: the malformed-value table drove one key and generalised to thirteen, so two of `loadClass`'s three legs never saw a bad value; the consequence under mutation was a malformed per-chat rate silently starting the process on the compiled default instead of refusing — the fail-loud contract inverted on the flood-ban axis. Fixed by driving all thirteen keys, not the two legs the finding named.

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
| R3-1 | round 3 | major | fixed@5c94c78 | `sed -i 's/newSchedule(orderedSchedule, windows)/newSchedule(unorderedSchedule, windows)/' internal/tg/limit.go` (limit.go:260), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN). Not an equivalent mutant under the SHIPPED chat windows `[{1,1s},{20,1m}]`: with grants at 0s and 10s, `earliest(1s)` = **11s** ordered vs **1s** unordered |
| R3-2 | round 3 | minor | fixed@5c94c78 | `New(Options{BaseURL: …, Token: …, Transport: {RetryMaxAttempts: 3, RetryBaseDelay: 500ms, RetryMaxDelay: 100ms, AttemptTimeout: 30s}})` must return an `*OptionError` naming `Transport.RetryMaxDelay` (measured: nil error; `client.go:112` tests only `<= 0`) |
| R3-3 | round 3 | nit | fixed@499012f | `git show fffa38b:internal/tg/retry_test.go` \| `grep -c AttemptTimeoutAbandonsAttempt` → 0, and `git log --oneline -S 'fixed@fffa38b' -- ai-docs/plans/2026-09-04-bot-api-transport.progress.md` → `b1fdf70` — so `fixed@fffa38b` is false for rows R2-1…R2-8 |
| R3-4 | round 3 | nit | fixed@499012f | `grep -n '^## Status' ai-docs/context.md` → `## Status (2026-09-02)` while `git diff c1d03fd..HEAD -- ai-docs/context.md` rewrites that section's body |
| R3-5 | round 3 | major | accepted@3 — RETRACTED after verifying on the production path: telego's `constructAndCallRequest` defers `closer.Close()` on `data.BodyStream` around `b.api.Call` (telego@v1.11.2:bot.go:232), so every early return in our `Call` (gate refusal, limiter refusal, pre-attempt cancellation) still unblocks the multipart writer goroutine. The 20/20 and 9/9 permanent leaks measured first were an artefact of driving the unexported `caller.Call` directly, which no production path does | 20 `SendPhoto` calls through `Client.API()` under a refusing gate + 10 under a limiter deadline refusal, then `runtime.Stack(all=true)` grepped for `writeMultipartBody` (measured: before=0 after=0 in both) |
| R3-6 | round 3 | nit | accepted@3 — design D9 predicts this survivor ("ceil rather than truncation buys evenness only — the quota window is what forbids the N+1-th emission either way"), and the Test Design says AC10(iii) is explicitly **not** red-first against a merely-truncated pacing interval | `sed -i 's\|return (w + time.Duration(n) - 1) / time.Duration(n)\|return w / time.Duration(n)\|' internal/tg/limit.go`, then `go test -count=1 ./internal/tg/` (measured: GREEN, as designed) |
| R3-7 | round 3 | nit | accepted@3 — below severity floor: the Test Design's subtask-2 line "the server records the method path and body it received" is discharged consumer-side by handler closures (`TestCaller_RequestCarriesContentTypeHeader`, `TestCaller_DerivesChatKnownFromSendMessage`), not by a recorder inside `tgtest`; D13's own behaviour list requires none | `grep -n 'records the method path' ai-docs/plans/2026-09-04-bot-api-transport.design.md` + `grep -n 'Recorded\|record' internal/tgtest/tgtest.go` |
| R4-1 | round 4 | major | fixed@fc08810 | `sed -i 's/^\tglobal.evict(now)$/\t_ = now/' internal/tg/limit.go` (limit.go:318) **and** delete the `if chat != nil { chat.evict(now) }` block (limit.go:319-321), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN, both separately and together). Consequence measured with a scratch probe driving `Limiter.acquire` 5000 times at 10s real-time spacing under the shipped message defaults: `len(grants)` = **1** global / **7** chat with the wiring, **5000 / 5000** without |
| R4-2 | round 4 | minor | fixed@fc08810 | `sed -i 's/^\t\ttelego.WithRequestConstructor(jsonConstructor{}),$//' internal/tg/client.go` (client.go:145), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R4-3 | round 4 | minor | fixed@fc08810 | `sed -i 's/^\t\tjitter:        jitter,$/\t\tjitter:        defaultJitter,/' internal/tg/client.go` (client.go:131), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN); `grep -rn 'Jitter' internal/tg/*_test.go` returns only `TestBackoffDelay_JitterBoundsExactly`, never an `Options.Jitter` assignment |
| R4-4 | round 4 | nit | fixed@fc08810 | `grep -n '^\| R3-[1-4] \|' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` → all four read `open` while `## Self-Review (Round 3)`'s own table marks all four `✅ Fixed` |
| R4-5 | round 4 | minor | fixed@fc08810 | `git diff --name-only c1d03fd..HEAD \| grep -i learn` → no hit, while this file's `## Decisions log` (subtask 4 entry) labels its own finding `Kind: validation` — `learnings.md` vocabulary that never reached `learnings.md` |
| R4-6 | round 4 | nit | fixed@fc08810 | `sed -i 's/botOptions = append(botOptions, telego.WithDiscardLogger())/_ = botOptions/' internal/tg/client.go` (client.go:150), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN) |
| R4-7 | round 4 | nit | accepted@4 — equivalent mutant on every reachable path: `sanitizeErr`'s `*url.Error` unwrap (D8's primary defence) drops the whole URL before the replacer runs, so no live path leaves the replacer as the only barrier, and `TestSanitizeErr_UnwrapsURLErrorAndDropsURL` exercises the replacer directly with one it builds itself | `sed -i 's/strings.NewReplacer(opts.Token, "\[REDACTED_TOKEN\]")/strings.NewReplacer()/' internal/tg/client.go`, then `go test -count=1 ./internal/tg/` (measured: GREEN) |
| R5-1 | round 5 | major | fixed@41b39fd | `perl -0777 -i -pe 's/\t\tif d > maxDelay\/2 \{\n\t\t\td = maxDelay\n\t\t\tbreak\n\t\t\}\n//; s/\tif d > maxDelay \{\n\t\td = maxDelay\n\t\}\n//' internal/tg/retry.go` (deletes BOTH `RetryMaxDelay` cap branches), then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN). `go tool cover` confirms 0 hits on `retry.go:32.21,34.9` and `retry.go:38.18,40.3`. Consequence measured: `backoffDelay(500ms, 30s, 10, func() float64 {return 1})` = **30s** shipped / **8m32s** uncapped; at attempt 12, **30s** / **34m8s** |
| R5-2 | round 5 | minor | fixed@41b39fd | `perl -0777 -i -pe 's/\tcase ClassEdit:\n\t\treturn l\.limits\.Edit\n/\tcase ClassEdit:\n\t\treturn l.limits.Other\n/' internal/tg/limit.go`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN); `go tool cover` confirms 0 hits on `limit.go:229.17,230.23` |
| R5-3 | round 5 | nit | fixed@41b39fd | replace `telego.WithLogger(opts.Logger)` (client.go:148) with `telego.WithDiscardLogger()`, then `go test -count=1 ./internal/tg/` — must go RED (measured: GREEN); `go tool cover` confirms 0 hits on `client.go:147.24,149.3` |
| R5-4 | round 5 | minor | fixed@41b39fd | `git show fc08810 -- ai-docs/plans/2026-09-04-bot-api-transport.progress.md` shows the round-4 fix commit adding only two `## Decisions log` bullets; `git diff c1d03fd..HEAD \| grep -n verdict` returns exactly two hits, KD-25's prose and the claiming sentence itself — no per-test enumeration exists anywhere in the branch |
| R5-5 | round 5 | nit | fixed@41b39fd | `grep -n '^\| R4-[1-6] \|' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` → all six read `open` while round 4's table marks all six `✅ Fixed`; `git log --oneline -S 'Status (2026-09-04)' -- ai-docs/context.md` → `499012f` while rows R3-3/R3-4 read `fixed@5c94c78`; `**last_passed_gate:**` names `cca171f` while `git rev-parse --short HEAD` is `fc08810` |
| R5-6 | round 5 | nit | accepted@5 — below severity floor: every remaining uncovered production block in the diff is defensive or unreachable by construction — `caller.go:71-76` (fixed-point non-convergence, whose `acquireFixedPoint` half IS tested with a never-settling stub), `caller.go:109` / `:193` / `:198` / `:212` / `:217` / `:230` / `:238`, `limit.go:137` (documented unreachable), `class.go:33` and `gate.go:38` (closed-enum `String()` default arms), `constructor.go:24,70,83,86` (multipart writer error paths), `retry.go:113,120` (nil error, `*url.Error` with a nil `Unwrap`) | `go tool cover -func` / the per-block 0-hit list from `go test -coverprofile` over `./internal/tg/` |
| R6-1 | round 6 | major | fixed@2d93871 | `perl -0777 -i -pe 's/\t\tif r, ok, err := lookupRate\(lookup, chatRate\); err != nil \{\n\t\t\terrs = append\(errs, err\)\n\t\t\} else if ok \{/\t\tif r, ok, _ := lookupRate(lookup, chatRate); ok {/' internal/config/transport.go` (and the same edit on the `chatCap` leg), then `go test -count=1 ./internal/config/ ./internal/tg/` — must go RED (measured: GREEN for both legs, independently). `go tool cover` confirms 0 hits on `transport.go:182.61,184.4` and `transport.go:187.60,189.4`. Shipped behaviour confirmed correct by a scratch probe: `loadTransport({LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE: "garbage"})` → `config: LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE: invalid value: must be "off" or <count>/<duration>, got "garbage"` |
| R6-2 | round 6 | minor | fixed@2d93871 | `grep -rni 'wiring' internal/tg/*_test.go internal/tgtest/*_test.go` → **no hits at all**, while `## Class sweep (round 4)` says `TestRedFirst_*` is "documented as not claiming `Limiter`-wiring coverage" and `TestMultipartCloseErr` / `TestMultipartRequest_FieldOrderAndParts` are "self-documented as not reaching wiring" |
| R6-3 | round 6 | minor | fixed@2d93871 | `git grep -h '^func Test' fc08810 -- 'internal/tg/*_test.go' 'internal/tgtest/*_test.go' 'internal/config/transport_test.go' \| sed 's/^func \(Test[A-Za-z0-9_]*\).*/\1/' \| sort` = 87 tests; four match no row of `## Class sweep (round 4)` — `TestBackoffDelay_JitterBoundsExactly` and `TestChatRefFromData` (both present at the swept tree, `git grep -c 'func TestBackoffDelay_JitterBoundsExactly' 5c94c78 -- internal/tg/retry_test.go` → 1) plus the two `TestNew_Installs*` the fix commit itself added |
| R6-4 | round 6 | minor | fixed@2d93871 | `awk 'NR>=250 && NR<=263' internal/tg/caller_test.go` — `t.Fatal` / `t.Fatalf` at :253 and :258 run on the `http.Server` handler goroutine, not the test goroutine (`testing`: "FailNow must be called from the goroutine running the test"); `internal/tgtest/tgtest.go:173,178` is the same code shape written correctly as `s.tb.Errorf` + `return`, with a doc comment saying why |
| R6-5 | round 6 | nit | fixed@2d93871 | `grep -n 'retry.go:3[0-9]' <coverprofile>` → the shipped blocks are `retry.go:33.21,35.9` and `retry.go:39.18,41.3`, while `internal/tg/retry_test.go:715,717` cite `retry.go:32.21,34.9` and `retry.go:38.18,40.3` — off by one since `41b39fd` added the `fmt` import. **Re-resolved reviewer-side (locator drift, not an amendment trigger); the re-resolved coordinates are the two in this cell, measured at `41b39fd`** |
| R6-6 | round 6 | nit | fixed@2d93871 | `sed -n '/^\*\*at:\*\*/p'` over the two entries this diff appends to `ai-docs/learnings.md` → only the first carries one, while the second's "Two instances in one `/task` run" is a count and `ai-docs/templates/learnings-entry.md` marks `**at:**` REQUIRED "whenever the entry contains a numeric claim — a count, a ratio, a byte size, a percentage" |
| R6-7 | round 6 | nit | fixed@2d93871 | `grep -n '^\| R5-[1-5] \|' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` → all five read `open` while round 5's own table marks all five `✅ Fixed` (correct stamps: R5-1/R5-2/R5-3 → `921d76d`, R5-4/R5-5 → `5d7badf`); and `grep -n '← CURRENT' <this file>` still marks subtask 5 current although every subtask reads `[x]` |
| R6-8 | round 6 | nit | accepted@6 — closes R5-5(iii): the *content* of `**last_passed_gate:**` is outside this agent's scope. `.claude/agents/self-review.md` instruction 2 requires the extended re-entry fields to be verified PRESENT and says "do NOT review their content for correctness; their lifecycle is the calling skill's responsibility". The field is present; whether it names `HEAD` is the calling skill's to keep | `grep -n '^\*\*last_passed_gate:\*\*' ai-docs/plans/2026-09-04-bot-api-transport.progress.md` |
| R6-9 | round 6 | nit | accepted@6 — the third remaining zero-hit block, `limit.go:328`, is `Limiter.acquire`'s `!converged` arm: the other half of the fixed-point non-convergence pair `R5-6` accepted at `caller.go:71-76`, unreachable for the same reason and signalled as a defect rather than swallowed. `R5-6`'s enumeration does not name it; recorded here so the next round does not treat it as newly appeared | `go test -coverprofile` over `./internal/tg/`, then `grep -n 'limit.go:32[0-9]' <profile>` → `limit.go:328.16,330.3 1 0` |

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
| 1 | internal/tg/limit.go:260 · internal/tg/limit_test.go:184 | major | **AC30's arrival-order clause is discharged by an instrument that cannot fail it.** AC30 requires that calls "leave one interval apart, **in the order they arrived**". The property rests entirely on `chatScheduleLocked` building the per-chat schedule as `orderedSchedule` (limit.go:260) — `earliest`'s clamp to the key's own newest grant. Flipping that one word to `unorderedSchedule` leaves `go test -count=1 ./internal/tg/` **fully GREEN**. It is not an equivalent mutant: under the *shipped* chat windows (`ChatRate 1/1s` → `{1,1s}`, `ChatCap 20/1m` → `{20,1m}`), with grants already at 0s and 10s, `earliest(1s)` returns **11s** ordered and **1s** unordered — a later-arriving call jumps ahead of an already-granted one. `TestLimiter_SteadyOrderedEmission` cannot see it because it configures `ChatRate` alone, and with the single deduped `{1,1s}` window the two kinds coincide; `TestSchedule_OrderedEmissionIsNonDecreasing` tests the schedule type with a **hand-built** `orderedSchedule` and never exercises the wiring. Every other `orderedSchedule` in the test files is likewise hand-built. Fix: one limiter-level test with both shipped chat windows configured, whose acquires are separated in real "now" so a hole opens, asserting the grant sequence is non-decreasing. | ✅ Fixed |
| 2 | internal/tg/client.go:112 | minor | **`New` does not reject a max delay below the base, which the design names as one of its validation cases.** Design § Test Design, subtask 3: "`New` rejects a zero attempt count, a non-positive base delay, **a max delay below the base**, and an empty base URL, each naming the field". `client.go:106-117` tests `RetryMaxAttempts < 1`, `RetryBaseDelay <= 0`, `RetryMaxDelay <= 0` and `AttemptTimeout <= 0` — there is no cross-field check, and `TestNew_ValidatesEachField`'s nine cases contain none. Nothing in `loadTransport` catches it either, since it is a cross-key inconsistency rather than a per-key one. Consequence: `LAB_GAME_TG_RETRY_MAX_DELAY=100ms` with `LAB_GAME_TG_RETRY_BASE_DELAY=500ms` starts cleanly and silently yields every backoff in `[50ms, 100ms)` — shorter than the configured base, and flat rather than growing, which is the half of AC5 that says "successive delays … grow rather than repeat". The flood-ban-relevant half (strictly positive, no tight loop) still holds, which is why this is `minor` and not `major`. Fix: the cross-field check plus its `TestNew_ValidatesEachField` row. | ✅ Fixed |
| 3 | (this file) `## Review register` | nit | **Rows R2-1 … R2-8 carry `fixed@fffa38b`, but those fixes landed in `b1fdf70`.** `fffa38b` is round 1's fix commit and predates every round-2 fix: `git show fffa38b:internal/tg/retry_test.go` contains no `AttemptTimeoutAbandonsAttempt` and no `elapsed != 0`, and `git log -S 'fixed@fffa38b'` on this file returns `b1fdf70` — the commit that both applied the fixes and wrote the (wrong) sha. Instruction 7a scopes the next round on that sha, so the error is in the safe direction (a wider window), which is why it is a `nit` and not higher. The eight rows should read `fixed@b1fdf70`; the sixteen `R1-*` rows are correct as they stand. | ✅ Fixed |
| 4 | ai-docs/context.md:40 | nit | **`## Status (2026-09-02)` is stale by its own diff.** This change rewrites that section's body — the "no Telegram client" clause became the transport plus `internal/tgtest` — but leaves the date in the heading. A reader who trusts the heading dates the transport claim two days before the transport existed. Fix: `## Status (2026-09-04)`. | ✅ Fixed |

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

## Self-Review (Round 4)

**Verdict:** REJECT

**What was checked.** Diff `c1d03fd..HEAD` (32 files, 14 commits). Per instruction 7a the
register scoped this round: rows `R3-1` … `R3-4` still read `open`, so no narrowing applied
and all four were re-examined in full by re-running each row's own verifying command as a
mutation against the shipped tree. **All four confirmed genuinely fixed, none re-opened:**
R3-1 (`newSchedule(orderedSchedule, windows)` → `unorderedSchedule` at limit.go:260 now goes
RED, killed by the new `TestLimiter_LaterArrivalDoesNotJumpAnEarlierGrant`), R3-2 (deleting
the new `RetryMaxDelay < RetryBaseDelay` cross-field check at client.go:115-117 goes RED,
killed by `TestNew_ValidatesEachField`'s new row), R3-3 (register rows `R2-1` … `R2-8` now
read `fixed@b1fdf70`, the commit that actually carries those fixes), R3-4
(`ai-docs/context.md` now reads `## Status (2026-09-04)`).

The round-3 fix commits `5c94c78` and `499012f` were read line by line. Beyond them: every
AC1–AC33 row re-derived against the shipped artefact; D1–D15 read against the code; the
Test Design's per-subtask scenario list walked against the shipped test set;
`make verify` re-run (GATE-GREEN — fmt, build, vet, `golangci-lint run` 0 issues,
file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck) plus an
uncached `go test -count=1 -race ./internal/tg/ ./internal/tgtest/ ./internal/config/`
(RACE-GREEN); the panicking-call audit, the `_ = err` / `context.Background()` / `TODO` /
`//nolint` / `.md:<line>`-citation sweeps and the file-size measurement re-run over the whole
changed set (all clean; largest non-test `internal/tg/limit.go` 377 / hard 1000, largest test
`internal/tg/limit_test.go` 838 / hard 1500; three `//nolint` directives, each with a
specific linter and a stated reason).

**A thirteen-mutation sweep was run on the CONSTRUCTION AND WIRING seams the earlier rounds'
sweeps did not reach** — `New`'s five `telego.With*` options and its seven `Client` field
assignments, and `Limiter.acquire`'s two `evict(now)` call sites. Eight mutants were killed;
five survived. One survivor is a genuinely equivalent mutant (`R4-7`), and the other four are
findings 1, 2, 3 and 5 below. Every mutation was applied to a `cp` backup and restored
immediately; `git status --porcelain` was confirmed clean after each batch and at the end.

**The instrument was itself put to the test** (AGENTS.md § Patterns 2). Finding 1's claim
that the shipped code's retention really is bounded — and not merely untested — was measured
rather than read: a scratch probe drove `Limiter.acquire` 5000 times at 10s of real-time
spacing under the shipped message defaults (`Global 30/1s`, `ChatRate 1/1s`, `ChatCap 20/1m`)
and read `len(schedule.grants)` directly, with the `evict` wiring present and absent. The
probe was deleted and the tree re-confirmed clean.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/tg/limit.go:318,319-321 · internal/tg/schedule_test.go:235 | major | **D9's bounded-retention claim rests on two lines in `Limiter.acquire` that no test exercises, and the test named for them cannot fail on them.** `acquire` is the only production caller of `schedule.evict`; deleting `global.evict(now)` (:318) and the `if chat != nil { chat.evict(now) }` block (:319-321) leaves `go test -count=1 ./internal/tg/` **fully GREEN**. It is not an equivalent mutant: a scratch probe driving `Limiter.acquire` 5000 times at 10s real-time spacing under the shipped message defaults measured `len(grants)` = **1** (global) / **7** (chat) with the wiring and **5000 / 5000** without — i.e. one retained `time.Time` per outbound call, for the life of the process, on both the class-global schedule and every per-chat schedule. That is precisely the bound D9 asserts (`design.md` § D9 *Registry, lifetime and persistence*: *"A schedule's history is bounded by its own invariant for the grants already in the past … so retention is **O(calls in flight)**, bounded by concurrency rather than by the window set alone"*) — a claim about the **Limiter**, which is the only thing production constructs. `TestSchedule_EvictBoundsMemoryAcrossManyAcquires` (schedule_test.go:235) is named and documented for *"a long sequence of real-time-separated acquires"* but hand-rolls `s.evict / s.earliest / s.commit` on a bare `*schedule` and never calls `Limiter.acquire`, so it asserts coverage it does not provide; `TestSchedule_EvictThreshold` (:209) covers `evict`'s own threshold and boundary, not its call sites. This is the same shape as round 1's finding 3 and round 3's finding 1, both rated `major`. Fix: one `Limiter`-level test that acquires repeatedly with an advancing `now` and asserts `len(l.global[ClassMessage].grants)` and the per-chat schedule's stay bounded — mutation-verified against both deletions. (Distinct from `R1-10`, whose own two mutants — the weakened threshold and the flipped `Before` — are killed; this is the wiring one level up.) | ✅ Fixed |
| 2 | internal/tg/client.go:145 | minor | **The `encoding/json` half of D3's swap is unpinned.** Deleting `telego.WithRequestConstructor(jsonConstructor{})` leaves the suite GREEN. telego then falls back to `telegoapi.DefaultConstructor`, whose `JSONRequest` marshals through `github.com/mymmrac/telego/internal/json`, which with no build tag binds to `github.com/grbit/go-json` (`internal/json/lib.gojson.go`: `//go:build !sonic && !stdjson`, `import "github.com/grbit/go-json"`) — so every outbound request body would silently stop going through `encoding/json`, contradicting D3's *"Every byte this project marshals or unmarshals itself goes through `encoding/json`"* and KD-2. `TestGuard_NoFastHTTPOrGoJSONImport` cannot see it: D3 itself scopes that guard to **this project's own imports**, and telego's are not ours. `TestMultipartRequest_FieldOrderAndParts` calls `jsonConstructor{}.MultipartRequest` directly, and `TestCaller_MultipartRequestNeverRetried` hand-builds its `RequestData`, so neither reaches the wiring either. `minor` rather than `major` because the two codecs produce equivalent bytes — the loss is D3-conformance, not behaviour. Fix: assert the constructor installed on a `New`-built client is this package's (e.g. drive one real `SendMessage` through `Client.API()` against a `tgtest` handler that fails unless the body's field order matches `jsonConstructor`'s sorted output, or check the seam directly). | ✅ Fixed |
| 3 | internal/tg/client.go:131 · internal/tg/retry_test.go:81 | minor | **`Options.Jitter` is an exported option no test ever sets.** `grep -rn 'Jitter' internal/tg/*_test.go` returns only `TestBackoffDelay_JitterBoundsExactly`, which calls `backoffDelay` directly with its own function value; no test ever assigns `o.Jitter`. Replacing `jitter: jitter` with `jitter: defaultJitter` in the `Client` literal (:131) — severing the option from the backoff entirely — leaves the suite GREEN. The design's § Test Design says AC5's test is driven *"with a fixed jitter source"* asserting delays that *"double"*; the shipped `TestRetry_DelaysGrowAndStayPositive` (retry_test.go:81) injects no source and asserts only `delay > 0` and `delay >= lastDelay`, which D6 proves holds for **every** draw — so it passes under the real random source and cannot distinguish an injected one. AC5 itself is not unmet (the formula is pinned exactly at unit level), but the option and the end-to-end exactness the design asked for are both unasserted. Fix: set `o.Jitter` to a pinned source in that test and assert the exact delays D6's formula predicts. | ✅ Fixed |
| 4 | (this file) `## Review register` | nit | **Rows `R3-1` … `R3-4` still read `open` although round 3's own table marks all four `✅ Fixed`.** Same class as `R2-8`, which round 2 raised for the `R1-*` rows and round 2's fix closed; the fixer owns the `fixed@<sha>` marker per `ai-docs/templates/progress-format.md`, and instruction 7a scopes the next round on that column. The error is in the safe direction — a wider re-examination window — which is why it is a `nit`. All four are independently verified fixed above; they should read `fixed@5c94c78` (R3-1, R3-2) and `fixed@499012f` (R3-3, R3-4) — verified with `git log --oneline -S 'fixed@b1fdf70' -- <this file>` → `499012f` and `git log --oneline -S 'Status (2026-09-04)' -- ai-docs/context.md` → `499012f`. | ✅ Fixed |
| 5 | (branch) ai-docs/learnings.md | minor | **`ai-docs/learnings.md` is untouched by the entire branch diff**, while this file's `## Decisions log` records, in its own words, *"a `Kind: validation` finding for the red-first methodology itself"* (subtask 4) — `learnings.md` vocabulary (`AGENTS.md` § Learning Log: *"write `Kind: validation` for a working protocol to keep doing"*), and the surface `/improve` audits. `git diff --name-only c1d03fd..HEAD \| grep -i learn` returns nothing. `AGENTS.md` § Workflow: *"Before every `git commit` during a PR task, stage `ai-docs/learnings.md` with the related change — learnings are part of the deliverable and must be visible in the PR diff"*, and Boundary rule 2's carve-out names *"in-flow capture of an in-task insight during `/task` Steps 8–12"* as exactly the permitted route. The progress file becomes a history surface under `ai-docs/plans/done/` at Step 12, so the insight is otherwise lost to the learning loop. `minor`, not `major`: it cites a workspace rule rather than an AC, D-id or gate, and the orchestrator may legitimately discharge it at Step 12. | ✅ Fixed |
| 6 | internal/tg/client.go:150 | nit | **`Options.Logger`'s documented nil behaviour is unpinned.** `Options.Logger`'s doc comment (client.go:43-48) states *"Nil means `telego.WithDiscardLogger()`"* and gives the reason (telego's default stderr logger is a duplicate channel this package does not want to own). Replacing the `else` branch's `append(botOptions, telego.WithDiscardLogger())` with a no-op leaves the suite GREEN, so a regression restoring telego's default logger would ship silently. Low stakes — output noise, not correctness — hence `nit`. | ✅ Fixed |

**No Design/Spec Amendment trigger.** Findings 1, 2, 3 and 6 are test additions; finding 4 is
a progress-file field; finding 5 is a `learnings.md` append. In every case the spec and the
design say the right thing and the shipped artefact is what falls short of them — no
criterion has changed and no design decision is contradicted. Finding 3 notes a divergence
between § Test Design's stated AC5 shape and the shipped test; the design is the correct half
there, so the repair is the test, never an amendment.

**Gate results re-run against the shipped artefact (not quoted from drafting):**

- `make verify` → GATE-GREEN (fmt, build, vet, `golangci-lint run` 0 issues, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck). Zero `FAIL` lines in the saved log.
- `go test -count=1 -race ./internal/tg/ ./internal/tgtest/ ./internal/config/` → RACE-GREEN.
- AC2: no `fasthttp` / `go-json` / `goccy` / `grbit` / `fastjson` import in any non-test file under `cmd/` or `internal/`. PASS (see finding 2 for the clause this guard is scoped away from).
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2`, direct; tidy-check clean. PASS.
- AC5 / D6: `backoffDelay`'s loop re-derived against D6's `d_i = min(base·2^i, max)` and `delay_i = d_i/2 + u·d_i/2` — the `d > maxDelay/2 → d = maxDelay` early exit is exactly the cap, and the new `RetryMaxDelay >= RetryBaseDelay` validation is what keeps `d_0 = base` inside it. PASS.
- AC7 / D5: the 429 case is driven **both with and without** `parameters.retry_after` (`tgtest.TooManyRequests(2)` in `TestRetry_RetryAfterHonouredExactly`, `tgtest.TooManyRequests(0)` in `TestRetry_RetryableCasesAreRetried`'s `429` subtest), as D5 requires. PASS.
- AC16: `TestRetry_Observation` pins `Retries`, `RateLimited` and `StatusCode` exactly for all three shapes and asserts `len(got) == 3`. PASS.
- AC26: no `api_id` / `api_hash` and no real-token-shaped literal in any added line; `Idempotency-Key` appears nowhere in the module, so D5's corollary holds structurally. PASS.
- AC32: no `*.sql` and no migration file in the range; `internal/tg` writes no state outside the process. PASS.
- D2 seam: `telego.NewBot` / `telego.With*` call sites exist only in `internal/tg/client.go`. PASS.
- Panicking-call audit over all 15 changed non-test `.go` files → no `panic(` / `log.Fatal*` / `log.Panic*`. No `ai-docs/panic-index.md` row owed. PASS.
- Error handling / context discipline: no `_ = err`, no `context.Background()` in production, no `TODO`; `ctx context.Context` first on every outbound-path function; no struct field of type `context.Context`. PASS.
- Doc convention: no `<file>.md:<line>` citation in any Go file (DOC-4); every exported item added by the round-3 fixes carries a name-initial doc comment. PASS.
- File sizes: `internal/tg/limit.go` 377 / hard 1000; `internal/tg/limit_test.go` 838 / hard 1500. PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `entry_args`, `## Decisions log`) present; `parent_skill` correctly omitted (`/task` is the parent flow). PASS.

**Recorded, not raised** (entered in the register as `accepted@4` — `R4-7`): emptying the
`Client`'s token replacer (`strings.NewReplacer(opts.Token, "[REDACTED_TOKEN]")` →
`strings.NewReplacer()`) survives the suite, but it is an equivalent mutant on every
reachable path — `sanitizeErr`'s `*url.Error` unwrap is D8's **primary** defence and discards
the whole URL before the replacer is ever consulted, and `TestSanitizeErr_UnwrapsURLErrorAndDropsURL`
already exercises the replacer directly with one it constructs itself. The replacer is
documented as *"defense-in-depth … that does not depend on having predicted every path a
token could take"*, and a layer with no live path is untestable by construction rather than
untested by omission.

## Self-Review (Round 5)

**Verdict:** REJECT

**What was checked.** Diff `c1d03fd..HEAD` (33 files, 16 commits). Per instruction 7a the
register scoped this round: rows `R4-1` … `R4-6` still read `open`, so no narrowing applied
and all six were re-examined in full by re-running each row's own verifying command against
the shipped tree. **All six confirmed genuinely fixed, none re-opened:** R4-1 (both deletions
run separately — `global.evict(now)` → `_ = now` and the `if chat != nil { chat.evict(now) }`
block removed — each now RED, killed by the new
`TestLimiter_EvictRunsInsideAcquireBoundingMemory`), R4-2 (deleting
`telego.WithRequestConstructor(jsonConstructor{})` → RED, killed by
`TestNew_InstallsPackageJSONConstructor`), R4-3 (`jitter: jitter` → `jitter: defaultJitter`
→ RED, killed by `TestRetry_JitterOptionThreadedThroughToBackoff`), R4-6
(`telego.WithDiscardLogger()` → no-op → RED, killed by
`TestNew_InstallsDiscardLoggerByDefault`), R4-4 (register rows `R3-1` … `R3-4` now carry a
`fixed@` marker — see finding 5 for the sha half), R4-5 (`ai-docs/learnings.md` now carries
the 2026-09-04 `Kind: validation` entry, visible in `git diff c1d03fd..HEAD`).

The round-4 fix commits `cca171f` and `fc08810` were read line by line. Beyond them: every
AC1–AC33 row re-derived against the shipped artefact; the AC2 / AC3 / AC15 / AC17 / AC26 /
AC32 and D2-seam scans re-run as shell greps against the post-implementation tree, not quoted
from drafting; the panicking-call audit, the `_ = err` / `context.Background()` / `TODO` /
`//nolint` / `.md:<line>`-citation sweeps and the file-size measurement re-run over the whole
changed set (all clean; largest non-test `internal/tg/limit.go` 377 / hard 1000, largest test
`internal/tg/limit_test.go` 888 / hard 1500; three `//nolint` directives, each with a
specific linter and a stated reason). Progress-file required fields present.

**This round changed instrument: a coverage profile replaced guessing at where to mutate**
(AGENTS.md § Patterns 2 — a green suite is a claim about the suite). `go test -coverprofile`
over `./internal/tg/ ./internal/tgtest/ ./internal/config/` was read block by block, and every
production block with **zero hits** was then mutated to find out whether the shipped code's
correctness there was tested or merely true. Twenty-three zero-hit blocks in `internal/tg`;
five mutations run on the reachable ones plus two on `internal/config/transport.go`'s class
wiring. **Two config-side mutants were killed** (routing the `LAB_GAME_TG_LIMIT_EDIT_*` keys
into `t.Limits.Other`, and `cl.ChatCap = r` → `cl.ChatRate = r`, both RED via
`TestLoadTransport_LimitValuesParsed`); **three survived**, and they are findings 1, 2 and 3
below. Every mutation was applied over a `cp` backup and restored immediately;
`git status --porcelain` was confirmed clean after each batch and at the end.

**The proposed fix for finding 1 was itself run before being proposed** (AGENTS.md § Patterns
1 — a suggested fix is a claim that the fix works). A scratch assertion
`backoffDelay(500ms, 30s, 6, jitter=0) == 15s` and
`backoffDelay(500ms, 30s, 10, jitter=1) == 30s` was written, run GREEN on the shipped tree,
then run under the cap-deleted mutant where it failed with
`attempt=6 jitter=0: got 16s want 15s` and `attempt=10 jitter=1: got 8m32s want 30s`. The
probe was deleted and the tree re-confirmed clean.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/tg/retry.go:32-35,38-40 · internal/tg/retry_test.go:617 | major | **`RetryMaxDelay`'s entire cap is unexercised — deleting both branches leaves the suite fully GREEN.** `backoffDelay`'s only two enforcement lines (`if d > maxDelay/2 { d = maxDelay; break }` and the post-loop `if d > maxDelay { d = maxDelay }`) have **zero coverage hits** (`retry.go:32.21,34.9` and `retry.go:38.18,40.3`), and removing both together leaves `go test -count=1 ./internal/tg/` green. It is not an equivalent mutant: measured with a scratch probe at the shipped defaults (`RetryBaseDelay 500ms`, `RetryMaxDelay 30s`), `backoffDelay(…, attempt=10, jitter=1)` returns **30s** capped and **8m32s** uncapped; at attempt 12, **30s** vs **34m8s**. Design D6 states `d_i = min(base·2^i, max)`, and D10's own rationale states the property that is unverified in as many words: *"`30s` caps the scale so a higher configured attempt count cannot grow the wait without bound"* — `RetryMaxAttempts` being an operator key (AC21), that is a live configuration, not a hypothetical. No existing test reaches the capped region: `TestBackoffDelay_JitterBoundsExactly` calls only `attempt=0`, and `TestRetry_DelaysGrowAndStayPositive` and the new `TestRetry_JitterOptionThreadedThroughToBackoff` both run 4 attempts at `base 100ms / max 10s`, where `d` never exceeds 400ms. D6's *"the test configures `base`/`max` so the region under test is uncapped"* scopes **AC5's** test away from the cap — it does not discharge the cap itself, which is the whole purpose of the key. Fix: two rows on `TestBackoffDelay_JitterBoundsExactly` in the capped region — verified above to be GREEN on the shipped tree and RED against the deletion. | ⬜ Open |
| 2 | internal/tg/limit.go:229-230 · internal/tg/limit_test.go:121 | minor | **`Limiter.classLimits`'s `ClassEdit` arm is never executed by any test.** Zero coverage hits on `limit.go:229.17,230.23`; flipping `case ClassEdit: return l.limits.Edit` to `return l.limits.Other` leaves the suite GREEN. The six `LAB_GAME_TG_LIMIT_EDIT_*` keys are parsed and routed correctly into `config.TransportLimits.Edit` (that half IS mutation-covered — see above), but nothing verifies the last hop from `TransportLimits.Edit` into an edit-class call's schedule. AC13's shipped verifier `TestLimiter_UnboundedClassPassesThroughAndBoundCounterpartBinds` uses `ClassOther` for **both** halves, so `ClassEdit` is unbounded-by-default and untested-when-bounded at once. `minor`, not `major`: at the shipped defaults `Edit` and `Other` are both the zero `ClassLimits`, so the mutant is behaviourally equivalent until an operator configures the two classes differently — which is exactly what D10's key table exists for. Fix: give that test a third leg on `ClassEdit`, or table-drive it over all three classes. | ⬜ Open |
| 3 | internal/tg/client.go:147-149 | nit | **`Options.Logger`'s non-nil branch is unexercised — the other half of round 4's finding 6.** Zero coverage hits on `client.go:147.24,149.3`; replacing `telego.WithLogger(opts.Logger)` with `telego.WithDiscardLogger()` leaves the suite GREEN, so an option this package exports and documents is never once set by a test. Round 4's fix pinned the `else` arm (`TestNew_InstallsDiscardLoggerByDefault`) and stopped at the `if`. Low stakes — log routing, not correctness — hence `nit`, and the helper it needs (`installedLoggerFields`, client_test.go) already exists: construct with a recognisable `telego.Logger` and assert it is the installed one. | ⬜ Open |
| 4 | (this file) `## Decisions log`, Step 11 (round 4) | minor | **The round-4 fix's own inspectability claim is false.** The entry reads *"The sweep enumerated every test in `internal/tg`, `internal/tgtest` and `internal/config/transport_test.go` with a per-test verdict, so the sweep is inspectable rather than a claim"*. No such enumeration exists: `git show fc08810 -- <this file>` adds only two `## Decisions log` bullets plus the register and round-4 sections, and `git diff c1d03fd..HEAD \| grep -n verdict` returns exactly two hits — KD-25's prose and the claiming sentence itself. AGENTS.md § *Communication*: *"A recorded result is a claim, not a completion"*; and this file becomes a history surface under `ai-docs/plans/done/` at Step 12, so the sentence outlives the session that could substantiate it. Findings 1–3 above are three further instances of precisely the shape that sweep says it enumerated and cleared, which is what makes the wording load-bearing rather than stylistic. Fix: either record the per-test verdict table the sentence promises, or reword the entry to what the sweep actually left behind. | ⬜ Open |
| 5 | (this file) `## Review register` · `**last_passed_gate:**` | nit | **Three bookkeeping drifts, one of them in the unsafe direction.** (i) Rows `R4-1` … `R4-6` still read `open` although round 4's own table marks all six `✅ Fixed` — the third recurrence of the class round 2 raised as `R2-8` and round 4 as its own finding 4. (ii) Rows `R3-3`/`R3-4` read `fixed@5c94c78`, but both fixes landed in `499012f`, which is the commit **before** it (`git log --oneline -S 'Status (2026-09-04)' -- ai-docs/context.md` → `499012f`; `git log --oneline -S 'fixed@b1fdf70' -- <this file>` → `499012f`) — round 4's finding 4 named `fixed@499012f` for exactly these two rows and the fix wrote `5c94c78` for all four. Unlike R3-3's original, this error **narrows** instruction 7a's re-examination window rather than widening it. (iii) `**last_passed_gate:**` records `cca171f` while `HEAD` is `fc08810`, so the field a compaction-recovery re-entry trusts predates the final commit — `make verify` is GREEN on `fc08810` (re-run this round), so the property holds and only the record is stale. | ⬜ Open |

**No Design/Spec Amendment trigger.** Findings 1, 2 and 3 are test additions; findings 4 and
5 are progress-file fields. In every case the spec and the design say the right thing and the
shipped artefact is what falls short of them — no criterion has changed and no design
decision is contradicted. Finding 1 quotes D6's own sentence scoping AC5's test away from the
capped region; that sentence is correct as written and is not what the finding asks to
change, so the repair is a second test, never an amendment.

**Gate results re-run against the shipped artefact (not quoted from drafting):**

- `make verify` → GATE-GREEN (fmt, build, vet, `golangci-lint run` 0 issues, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck). Zero `FAIL` lines in the saved log.
- `go test -count=1 -race ./internal/tg/ ./internal/tgtest/ ./internal/config/` → RACE-GREEN (uncached).
- `go vet ./...` → exit 0. `golangci-lint run` → `0 issues`.
- AC2: `grep -rnE 'fasthttp|go-json|goccy|grbit|fastjson' --include=*.go cmd/ internal/` minus tests → one comment hit at `client.go:139`. PASS.
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2`, direct; tidy-check clean. PASS.
- AC15 source half: `TestGuard_BaseURLOnlyInConstructor` scans `internal/tg`'s non-test files and exempts `client.go`; `BaseURL` appears nowhere else in that package. (`internal/config` and `internal/tgtest` also carry the identifier, which is the *configuration* axis AC15 is about, not a branch in the limiter path.) PASS.
- AC17: no `prometheus` / `expvar` / metrics reference in any `internal/tg` non-test file. PASS.
- AC21 literal scan + AC27 gate refusal + D2 seam: `telego.NewBot` / `telego.With*` **call sites** exist only in `internal/tg/client.go` (the new `client_test.go` hits are comment text and failure-message strings, which the guard's call-shaped regex does not match — confirmed by the guard still passing). PASS.
- AC26: no `api_id` / `api_hash` and no real-token-shaped literal in any added line; `internal/tgtest`'s fake token carries a `//nolint:gosec` with a stated reason. PASS.
- AC32: no `*.sql` and no migration file in the range. PASS.
- Panicking-call audit over all changed non-test `.go` files → no `panic(` / `log.Fatal*` / `log.Panic*`. No `ai-docs/panic-index.md` row owed. PASS.
- Error handling / context discipline: no `_ = err`, no `context.Background()` in production, no `TODO`; `ctx context.Context` first on every outbound-path function; no struct field of type `context.Context`. PASS.
- Doc convention: no `<file>.md:<line>` citation in any Go file (DOC-4). PASS.
- File sizes: `internal/tg/limit.go` 377 / hard 1000; `internal/tg/limit_test.go` 888 / hard 1500. PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `entry_args`, `## Decisions log`) present; `parent_skill` correctly omitted (`/task` is the parent flow). PASS (with finding 5(iii) on `last_passed_gate`'s staleness).

**Recorded, not raised** (entered in the register as `accepted@5` — `R5-6`): the remaining
zero-hit production blocks are all defensive or unreachable by construction — `Call`'s
fixed-point non-convergence branch (whose `acquireFixedPoint` half **is** tested with a
never-settling stub, which is why the extraction exists), `schedule.earliest`'s documented
unreachable fallback, the closed-enum `String()` default arms in `class.go` and `gate.go`,
`writeMultipartBody`'s writer-error paths, `sanitizeErr`'s nil-error and nil-`Unwrap` guards,
and `caller.httpClient`'s `http.DefaultClient` fallback (no production site constructs a
`Client` yet — `cmd/bot` is deliberately untouched). None of these carries a design-asserted
property the way finding 1's cap does.

## Class sweep (round 4)

The per-test verdicts behind the round-4 Decisions-log bullet. Two forms were hunted:
**(a)** a test that hand-builds the object under test instead of driving the public entry
point that constructs it, so the wiring which installs the behaviour is never exercised;
**(b)** a test whose fixture is configured where the two branches coincide, so it cannot
distinguish them.

| Test | Form checked | Verdict |
|---|---|---|
| `TestSchedule_EvictBoundsMemoryAcrossManyAcquires` | (a) hand-built `*schedule` | **Fixed** — companion `TestLimiter_EvictRunsInsideAcquireBoundingMemory` drives `Limiter.acquire`; the original stays as a narrower unit test of `evict`'s own arithmetic |
| `TestSchedule_OrderedEmissionIsNonDecreasing` | (a) hand-built `orderedSchedule` | Cleared — covered, not repaired: `TestLimiter_LaterArrivalDoesNotJumpAnEarlierGrant` reaches the same property through `chatScheduleLocked`'s real kind selection |
| `TestLimiter_SteadyOrderedEmission` | (b) single window, where ordered and unordered coincide | Cleared — same companion is the distinguishing fixture, and names this test's limitation in its own doc comment |
| `TestRetry_DelaysGrowAndStayPositive` | (b) cannot distinguish an injected jitter | **Fixed** — companion `TestRetry_JitterOptionThreadedThroughToBackoff` pins exact delays with the option set |
| `TestCaller_MultipartRequestNeverRetried` | (a) hand-built `&caller{client: c}` | Cleared — its doc comment covers D5's retry-exit rule, not the constructor wiring; that gap is closed by `TestNew_InstallsPackageJSONConstructor`. |
| `TestSanitizeErr_UnwrapsURLErrorAndDropsURL` | (a) hand-built replacer | Cleared — adjudicated as `R4-7`: the `*url.Error` unwrap discards the URL before the replacer runs |
| `TestSchedule_*` (oracle, invariant, unbounded, retention, evict-vs-future-grant, threshold) | direct algorithm properties, no wiring claim | Cleared |
| `TestRedFirst_*` (7) | broken-vs-real comparative at schedule level | Cleared — they compare a hand-built broken mechanism against the real one at schedule level and make no claim about `Limiter` wiring. No test states this; it is the sweep's reading of them. |
| `TestPaceWindows_*`, `TestAcquireFixedPoint_*` | pure functions | Cleared |
| `TestLimiter_*` (all others) | drive `Limiter.acquire` | Cleared |
| `TestCaller_*`, `TestRetry_*` (all others) | drive `c.API()` through `New`/`newTestClient` | Cleared |
| `TestMultipartCloseErr`, `TestMultipartRequest_FieldOrderAndParts` | component tests | Cleared — they exercise the writer directly and do not reach the constructor wiring, which `TestNew_InstallsPackageJSONConstructor` covers instead. No test states this; it is the sweep's reading of them. |
| `TestGuard_*` | static source scans plus one real end-to-end | Cleared |
| `TestClassifyMethod`, `TestMethodClass_String`, `TestChatTarget_String` | pure-function tables | Cleared |
| `TestServer_*` (`internal/tgtest`) | tests of the fake server itself | Cleared |
| `TestLoadTransport_*` (`internal/config`) | call `loadTransport` directly | Cleared |
| `TestNew_HappyPath`, `TestNew_ValidatesEachField`, `TestOptionError_*`, `TestError_*` | call `New` / pure methods | Cleared |
| `TestBackoffDelay_JitterBoundsExactly` | (b) fixture at `attempt=0` only, where the cap cannot bind | **MISSED by this sweep** — round 5 found it by coverage profile instead, and it is the reason the criterion replaced the reading-based sweep |
| `TestNew_InstallsPackageJSONConstructor`, `TestNew_InstallsDiscardLoggerByDefault` | added by this round; drive `New` | Cleared — omitted from the sweep's first write-up |

Round 5 then found three further members of the same class that this sweep cleared or did
not reach — the `RetryMaxDelay` cap, `classLimits`'s `ClassEdit` arm, and `Options.Logger`'s
non-nil branch — by mutating every production block a coverage profile showed with zero hits
instead of choosing targets by reading. That instrument is stronger than this sweep's.

## Self-Review (Round 6)

**Verdict:** REJECT

**What was checked.** Diff `c1d03fd..HEAD` (33 files, 19 commits), `HEAD` = `41b39fd`. Per
instruction 7a the register applied no narrowing — rows `R5-1` … `R5-5` all read `open`, so
all five were re-examined in full by re-running each row's own verifying command as a
mutation against the shipped tree. **All five confirmed genuinely fixed, none re-opened:**

- **R5-1** — deleting BOTH `RetryMaxDelay` cap branches now goes RED, killed by three new rows
  on `TestBackoffDelay_JitterBoundsExactly`: `backoffDelay(attempt=6, jitter=0) = 16s, want
  15s`, `= 8m32s, want 30s` at attempt 10, and `= 20s, want 15s` for the post-loop clamp. The
  third row is what reaches `retry.go:39.18,41.3`, which the first two do not.
- **R5-2** — `case ClassEdit: return l.limits.Edit` → `l.limits.Other` now goes RED, killed by
  `TestLimiter_ClassLimitsRoutesEachClassToItsOwnConfig` (`second-first = 3s, want exactly
  2s`), which table-drives all three classes on distinguishable `ChatRate`s.
- **R5-3** — `telego.WithLogger(opts.Logger)` → `telego.WithDiscardLogger()` now goes RED,
  killed by `TestNew_InstallsProvidedLoggerWhenNonNil`.
- **R5-4** — `## Class sweep (round 4)` now exists in the tree with a per-test verdict table,
  and the Decisions-log bullet no longer claims inspectability it cannot show. (The table's own
  accuracy is findings 2 and 3 below — the repair landed, two claims inside it did not.)
- **R5-5** — (i) rows `R4-1` … `R4-6` now carry `fixed@fc08810`; (ii) rows `R3-3`/`R3-4` now
  read `fixed@499012f`, the commit the fixes actually landed in. (iii) is adjudicated as
  `R6-8`, not re-opened: instruction 2 puts the *content* of `**last_passed_gate:**` outside
  this agent's scope.

The three commits since round 5 (`5d7badf`, `921d76d`, `41b39fd`) were read line by line.
`41b39fd` is the only production change in the window and is a real bug fix, not a test
addition: `sanitizeErr`'s `cause == nil` fallback called `uerr.Err.Error()` on the interface it
had just found nil, so a `*url.Error` with no wrapped cause panicked inside the error path —
against `AGENTS.md` § *Go Test Conventions*' no-`panic`-in-production rule, on the path D8
exists to make safe. Both halves of the fix were mutation-verified here: replacing
`fmt.Errorf("%s: no underlying error", uerr.Op)` with an anonymous error goes RED, and deleting
the `if err == nil { return nil }` guard goes RED.

**The coverage instrument round 5 introduced was re-run rather than inherited.**
`go test -count=1 -coverprofile` over `./internal/tg/ ./internal/tgtest/ ./internal/config/`
now reports `internal/tg` at **98.6 %** with **three** zero-hit blocks left, down from
twenty-three: `caller.go:71-76` (fixed-point non-convergence, whose `acquireFixedPoint` half
IS driven by a never-settling stub), `limit.go:137` (documented-unreachable `earliest`
fallback) and `limit.go:328` (`Limiter.acquire`'s `!converged` arm — the other half of the
pair `921d76d`'s own commit message names, and not separately listed in `R5-6`'s text). The
first two are `accepted@5` verbatim; the third is adjudicated here as the same mechanism and
is left with them. **Every one of the nineteen production blocks that moved from
zero hits to covered was then mutated to find out whether the new test asserts the behaviour
or merely executes it: all nineteen mutants were killed** (`caller.go`'s no-body / build-request
/ read-body / decode-json / generic-cause / `http.DefaultClient` / `methodFromURL`-parse
blocks; `constructor.go`'s four wrapping paths; `class.go`'s and `gate.go`'s `String()` default
arms; `retry.go`'s two `sanitizeErr` guards and its two `RetryMaxDelay` cap branches;
`limit.go`'s `ClassEdit` arm; `client.go`'s non-nil-Logger arm). None of the round's new tests
is cosmetic.

**The coverage read reached one package the round-5 sweep's mutation phase did not**, and that
is finding 1: `internal/config/transport.go`'s `loadClass` closure has three sibling error
branches and only the first has ever been executed by a test.

**Four guards were re-planted rather than trusted** (`AGENTS.md` § *Patterns* 2). A
`_ = 500 * time.Millisecond` literal planted in `retry.go` turns
`TestGuard_NoRetryOrRateLimitLiteralAtCallSite` RED; a `_ "github.com/valyala/fasthttp"` import
planted in `caller.go` turns `TestGuard_NoFastHTTPOrGoJSONImport` RED with the file named.
Both are load-bearing on the current tree, not tautologies.

**Every factual claim in the round's prose was re-derived, not read** (§ *Patterns* 1 — the
window's largest single artefact, `5d7badf`, is 148 lines of prose and 0 lines of Go). All 39
test names cited in `## AC Status` resolve to a `func Test…` in the tree; the six named
companions in `## Class sweep (round 4)` all exist; `TestLimiter_LaterArrivalDoesNotJumpAn`
`EarlierGrant` does drive `chatScheduleLocked`'s real kind selection on a configuration where
the kinds diverge, and its doc comment does name `TestLimiter_SteadyOrderedEmission`'s
limitation. Two claims in that same table did not survive the check — findings 2 and 3.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/config/transport.go:182-190 · internal/config/transport_test.go:107 | major | **AC22's malformed clause is unverified for six of the thirteen keys, and the mutant is not equivalent.** AC22: *"Loading with one present and malformed fails with an error naming that variable."* `TestLoadTransport_Malformed`'s seven rate-shaped rows all use `envTGLimitMessageGlobal`, so only `loadClass`'s **first** leg is ever driven with a bad value; the `chatRate` and `chatCap` legs are separate blocks and both sit at **zero coverage hits** (`transport.go:182.61,184.4` and `187.60,189.4`). Replacing either `err != nil { errs = append(errs, err) }` with a discard leaves `go test -count=1 ./internal/config/ ./internal/tg/` **GREEN** — measured separately for each leg. That is a live escape, not a rename: under the mutant a malformed `LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE` is silently dropped and the process starts on the compiled-in `1/1s` default instead of refusing to start, and the six keys it covers (`MESSAGE`/`EDIT`/`OTHER` × `CHAT_RATE`/`CHAT_CAP`) include exactly the two per-chat figures `ai-docs/domain-invariants.md` § 6 puts on the flood-ban axis — the same axis round 2's finding 2 was raised on. The shipped code is right: a scratch probe returns `config: LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE: invalid value: must be "off" or <count>/<duration>, got "garbage"` for all four keys tried. It is AC22's verifier that cannot fail. Fix: extend `TestLoadTransport_Malformed`'s table with a `CHAT_RATE` row and a `CHAT_CAP` row (one class suffices — the three classes share the closure; the three *legs* do not). | ⬜ Open |
| 2 | (this file) `## Class sweep (round 4)` | minor | **Two rows of the round-4 sweep table assert documentation that does not exist anywhere in the tree.** The `TestRedFirst_*` row reads *"Cleared — documented as not claiming `Limiter`-wiring coverage"*, and the `TestMultipartCloseErr`, `TestMultipartRequest_FieldOrderAndParts` row reads *"component tests, self-documented as not reaching wiring"*. `grep -rni 'wiring' internal/tg/*_test.go internal/tgtest/*_test.go` returns **no hits at all**; the red-first block's header comment describes the RED-then-GREEN protocol and says nothing about wiring coverage, and `TestMultipartCloseErr`'s doc comment is about finding 13's `writer.Close()` error, while `TestMultipartRequest_FieldOrderAndParts`'s documents the *opposite* direction (that it *does* drive `MultipartRequest`, unlike `TestCaller_MultipartRequestNeverRetried`). This section is the artefact written to close `R5-4`, a finding about a claim with nothing behind it; `AGENTS.md` § *Communication*: *"A citation offered as authority is itself a claim — open it."* Fix: either write the exempting sentence into those tests' doc comments, or restate the two verdicts as the sweep's own judgement rather than as a citation of the source. | ⬜ Open |
| 3 | (this file) `## Class sweep (round 4)` | minor | **The table presented as "the per-test verdicts" omits two tests that existed at the tree it swept, and the omission was consequential.** At `fc08810` the three packages hold 87 `func Test…` functions; four match no row of the table by name or by glob. Two of those four (`TestNew_InstallsPackageJSONConstructor`, `TestNew_InstallsDiscardLoggerByDefault`) the round-4 fix commit added itself, so they are legitimately out of scope — but `TestBackoffDelay_JitterBoundsExactly` and `TestChatRefFromData` were both present at the swept tree (`git grep -c 'func TestBackoffDelay_JitterBoundsExactly' 5c94c78 -- internal/tg/retry_test.go` → 1). `TestBackoffDelay_JitterBoundsExactly` is precisely a form-**(b)** instance — its every row ran `attempt=0`, where `RetryMaxDelay`'s cap branch cannot fire — which is what round 5's finding 1 then had to find by a different instrument. A sweep whose stated purpose is to enumerate a class must say which population it enumerated. Fix: add the two rows with their round-4 verdicts, or state the table's scope as the tests the sweep reached. | ⬜ Open |
| 4 | internal/tg/caller_test.go:253,258 | minor | **`t.Fatal` / `t.Fatalf` called from the HTTP handler goroutine.** `TestDoAttempt_TruncatedBodyReturnsReadError`'s handler closure calls `t.Fatal("ResponseWriter does not support Hijack")` and `t.Fatalf("hijack: %v", err)`; that closure is invoked by `http.Server` on the connection-serving goroutine, not by the test goroutine. `testing`'s own contract: *"FailNow must be called from the goroutine running the test … Calling FailNow does not stop those other goroutines."* The consequence is that the guard aborts the connection mid-response through `runtime.Goexit` while the test goroutine keeps running against a half-dead server, which is the failure mode the guard was written to report cleanly. `go vet`'s `testinggoroutine` analyser cannot see it because the closure is passed as a value to `tgtest.New`. This package already has the correct shape written down: `internal/tgtest/tgtest.go:169-182` handles the identical `Hijacker` assertion with `s.tb.Errorf(…)` + `return`, and its doc comment states why. Fix: `t.Errorf` + `return`, matching the precedent. | ⬜ Open |
| 5 | internal/tg/retry_test.go:715,717 | nit | **Stale coverage-block coordinates, introduced by the following commit.** The comment cites `retry.go:32.21,34.9` and `retry.go:38.18,40.3`; `41b39fd` then added `"fmt"` to `retry.go`'s import block, so the shipped blocks are `retry.go:33.21,35.9` and `retry.go:39.18,41.3` (read from this round's own coverprofile). A **locator drift**, re-resolved reviewer-side and recorded in `R6-5` — not a Design/Spec Amendment trigger, and it carries no commit pin, which is why it drifted within one commit of being written. Fix: cite the branch by its code (*"the in-loop early break"* / *"the post-loop clamp"*, both already spelled out in the same comment) and drop the numbers. | ⬜ Open |
| 6 | ai-docs/learnings.md (second entry added by this diff) | nit | **A required `**at:**` field is missing.** `ai-docs/templates/learnings-entry.md` marks `**at:**` REQUIRED *"whenever the entry contains a numeric claim — a count, a ratio, a byte size, a percentage"*; the `2026-09-04 — process — writing a claim about my own work…` entry opens with *"Two instances in one `/task` run"* and carries no `**at:**`, while the `2026-09-04 — testing` entry beside it correctly carries `**at:** 5c94c78`. The count is measured mid-task over the very artefacts the task is still mutating, which is the case the field's own rationale names. Fix: add `**at:** 5d7badf`. Note for the fixer: Boundary rule 1's append-only prohibition protects *merged* history — this entry is introduced by the branch under review and is not yet history, so adding the line is completing the entry, not editing the log. | ⬜ Open |
| 7 | (this file) `## Review register` · `## Subtasks` | nit | **Two stale bookkeeping markers.** (i) Rows `R5-1` … `R5-5` still read `open` although round 5's own table marks all five `✅ Fixed`, and this round has independently confirmed all five — the **fourth** recurrence of the class (`R2-8`, round 4's finding 4, `R5-5`(i)). Per `ai-docs/templates/progress-format.md` the fixer owns the `fixed@<sha>` cell; the correct stamps are `921d76d` for `R5-1`/`R5-2`/`R5-3` and `5d7badf` for `R5-4`/`R5-5`. Instruction 7a scopes the next round on this column, so leaving them `open` buys another full re-verification of five closed findings. (ii) `## Subtasks` item 5 still ends `← CURRENT` although all seven items read `[x]` and Group A is recorded complete in item 6. | ⬜ Open |

**No Design/Spec Amendment trigger.** Finding 1 is a test addition; findings 2, 3, 6 and 7 are
edits to this progress file and to `learnings.md`; finding 4 is a two-line test edit; finding 5
is a code-comment edit and is explicitly the **locator-drift** case the carve-out covers
(re-resolved above, no `design-writer` / `spec-writer` involvement). In every case the spec and
the design say the right thing and the shipped artefact is what falls short of them — no
criterion has changed and no design decision is contradicted.

**Gate results re-run against the shipped artefact (not quoted from drafting):**

- `make verify` → GATE-GREEN, exit 0 (fmt, build, vet, `golangci-lint run` `0 issues.`, file-limits, `go test`, `go test -race`, tidy-check, actionlint, shellcheck). Zero `FAIL` lines in the saved log.
- `go test -count=1 -race ./internal/tg/ ./internal/tgtest/ ./internal/config/` → RACE-GREEN, uncached (`ok` ×3, 1.5 s / 1.0 s / 1.0 s).
- `go vet ./...` → exit 0. `golangci-lint run` → `0 issues.`
- Coverage: `internal/tg` 98.6 %, `internal/tgtest` 80.3 %, `internal/config` 94.4 %. Three zero-hit blocks remain in `internal/tg`, all `accepted@5` (`R5-6`).
- AC2: `grep -rnE 'fasthttp|go-json|goccy|grbit|fastjson' --include=*.go cmd/ internal/` → three hits, all prose (`client.go:139` comment, `guards_test.go:62,69` the guard's own forbidden list). PASS, and re-planted RED.
- AC3: `go.mod:9 github.com/mymmrac/telego v1.11.2`, direct; tidy-check clean. PASS.
- AC15 source half: `BaseURL` appears in exactly one non-test file, `internal/tg/client.go` (`:18,:22,:100,:101,:143`). PASS.
- AC17: no `prometheus` / `expvar` / metrics identifier in any `internal/tg` non-test file. PASS.
- AC21: the literal guard is load-bearing — a planted `500 * time.Millisecond` in `retry.go` turns `TestGuard_NoRetryOrRateLimitLiteralAtCallSite` RED. PASS.
- AC22/AC23: `.env.example` ↔ `defaultTransport()` equality holds (`TestLoadTransport_ExampleMatchesDefaults`); AC22's *malformed* half is finding 1.
- AC26: no `api_id` / `api_hash` and no real-token-shaped literal in any added line; the only token literal is `internal/tgtest/tgtest.go:42`'s fake, carrying `//nolint:gosec` with a stated reason. PASS.
- AC32: no `*.sql` and no migration file in `git diff --name-only c1d03fd..HEAD`. PASS.
- D2 seam: `telego.NewBot` / `telego.With*` **call sites** exist only in `internal/tg/client.go` (`:143,:144,:145,:148,:150,:153`); every other hit is comment or failure-message text. PASS.
- Panicking-call audit over every changed non-test `.go` file → no `panic(` / `log.Fatal*` / `log.Panic*`. No `ai-docs/panic-index.md` row owed — and `41b39fd` **removed** the one latent production panic this package had. PASS.
- Error handling / context discipline over the same set: no `_ = err`, no `context.Background()`, no `TODO`. Three `//nolint` directives, each naming a specific linter with a stated reason. PASS.
- Doc convention: no `<file>.md:<line>` citation in any Go file (DOC-4). No new exported item in the window (the round's additions are tests plus one unexported production branch). PASS.
- File sizes: largest non-test `internal/tg/limit.go` 377 / hard 1000; largest test `internal/tg/limit_test.go` 933 / hard 1500. PASS.
- Progress-file required fields (`Branch`, `base_commit`, `Last build`, `current_step`, `last_passed_gate`, `## Decisions log`) present, plus the conditional `entry_args`; `parent_skill` correctly omitted per `ai-docs/templates/progress-format.md` (`/task` is the parent flow). Content not reviewed, per instruction 2. PASS.
- Working tree confirmed clean (`git status --porcelain` empty) after every mutation batch and at the end of the round; every mutation was applied over a `cp` backup and restored, and the two scratch probes were deleted.

**Recorded, not raised** (entered in the register as `accepted@6` — `R6-8`): the *content* of
`**last_passed_gate:**`, which round 5 raised as its finding 5(iii). Instruction 2 requires the
extended re-entry fields to be verified **present** and states that their content is the
calling skill's lifecycle, not this agent's to review. The field is present; whether its SHA
names `HEAD` is out of scope here, and re-raising it each round is the loop re-litigating a
question it is not the right instrument for.
