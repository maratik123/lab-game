# Progress: Bot API transport over telego — ACTIVE
_Updated: 2026-09-04 17:50_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-bot-api-transport
**base_commit:** c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**Last build:** not run
**Issue:** #19
**Spec:** ai-docs/plans/2026-09-04-bot-api-transport.spec.md
**current_step:** Step 8 — subtask 4 of 7 complete (Group A, subtask 4 of 6)
**last_passed_gate:** go build ./... + go test -race ./... + go vet ./... + golangci-lint run (0 issues) + golangci-lint fmt -d (clean) + go mod tidy (no delta) | (subtask 4 commit, see below)
**entry_args:** 19

## Next action

**Do this immediately:** continue Group A with subtask 5 (`internal/tg` caller — extends subtask 4's `Call`: the attempt loop, `httptrace` write-evidence classifier, equal-jitter backoff with the `//nolint:gosec // G404` directive, exact `retry_after` honouring, the D8 typed error's token sanitisation, and the single observation).

## Subtasks

- [x] 1. `internal/config`: optional-with-default transport key class, `.env.example`, falsified doc comments — commit c79c12b
- [x] 2. `internal/tgtest`: in-process fake Bot API server — pipe-backed `net.Listener`/`http.Server`, no real socket, `.invalid` base URL, fake token, `Success`/`TooManyRequests`/`ServerError`/`Delayed`/`FailNextDial`/`CloseWithoutResponse` behaviours, 8 tests all green under `-race`
- [x] 3. `internal/tg` foundations + the telego dependency — `go get github.com/mymmrac/telego@v1.11.2` (direct requirement, go.mod/go.sum confirmed idempotent under a second `go mod tidy`); `Error`/`OptionError` (D8), `Observation`/`Observer` (D11), `MethodClass`+classifier (D4, tested against telego's own real method names incl. trap cases), `ChatTarget`/`ChatRef`/`Call`/`Gate` (D12), `Options`/`New`/`Client.API()` (D2) — `New` wires only `WithAPIServer` + `WithLogger`/`WithDiscardLogger` so far; `WithAPICaller`/`WithRequestConstructor` wiring is subtask 4's job per the design's own decomposition (client.go is edited again there)
- [x] 4. `internal/tg` limiter (D9) + minimal caller — `limit.go` (`schedule`/`Limiter`, decide-then-commit, ordered/unordered kinds, time-based `evict`), `constructor.go` (`encoding/json` `RequestConstructor`), `caller.go` (derives method+`ChatRef` from URL/body, gate, limiter acquire+wait, one HTTP attempt), `client.go` wires `WithAPICaller`/`WithRequestConstructor`. **All 7 of the binding red-first broken-variant table's rows implemented and run RED-then-GREEN** (`TestRedFirst_CommitAtCandidate`, `_CommitThenUndo`, `_BucketCap`, `_OrderedGlobal`, `_CountRetention`, `_PaceOnly`, `_QuotaOnly` — none stayed green as a broken variant). Plus `TestSchedule_*` (brute-force oracle over 200 randomized trials, invariant-after-every-commit, ordered non-decreasing, unbounded-contributes-no-window, time-based retention, real-now-vs-future-grant eviction), `TestLimiter_*` (AC10–AC15, AC30–AC33 functional scenarios), and `TestCaller_*` end-to-end through real telego + tgtest (success, `ChatKnown` derivation from a real `SendMessage`, gate refusal costs zero attempts, limiter delay honours a short context deadline).  ← CURRENT
- [ ] 3. `internal/tg` foundations + the telego dependency (`go get`)
- [ ] 4. `internal/tg` limiter: the window schedule + the minimal caller
- [ ] 5. `internal/tg` caller: retry loop, backoff, `retry_after`, typed error, observation
- [ ] 6. `internal/tg` package-level guard tests
- [ ] 7. `ai-docs/key-decisions.md`: KD-2 rewrite + the new decisions

## Decisions log

- **Step 6–7**: design reached GO at review round 7; the first mechanism (composed limiter legs) was abandoned at the owner's direction after three rounds produced four defects of one shape, and replaced by a single per-key schedule holding a set of `(count, per)` windows.
- **Step 7**: owner decisions recorded — keep `LAB_GAME_TG_ATTEMPT_TIMEOUT` (30s); defer `stdjson` to its own PR; stay on Go 1.26.5 with telego pinned at v1.11.2; leave `docs/DESIGN.md` §11 untouched; media stays out of MVP with no media mechanism; no "exactly one sending process" constraint recorded.
- **Step 7**: the telego v1.12.x ceiling is a configuration block, not an incompatibility — v1.12.1 builds clean under go1.26.5 once its own directive is lowered; raising the toolchain is deferred to its own task.
- **Subtask 1 (code-writer)**: implemented `loadTransport` as a dedicated reader (never added to `envKeys()`), 13 `LAB_GAME_TG_*` keys, `Rate`/`ClassLimits`/`TransportLimits`/`Transport` value types per D10's exact shape. `.env.example` and the reader landed in the same commit (disjointness test requires it). `env.go`/`config.go`'s falsified doc comments rewritten; `doc.go`'s enumeration extended. Verified via `rg -U` sweep that no other live site asserts the old "every variable is required" claim outside history surfaces and balance-scoped comments. All gates green: build, full `go test ./...`, `go vet`, `golangci-lint run` (0 issues), `golangci-lint fmt -d` (clean).
- **Subtask 4 (code-writer) — a genuine defect the red-first tests caught, not one the design predicted.** The first `schedule.commit(t)` implementation evicted aged-out grants using `t` (the just-granted, possibly far-future instant) as the "now" for the eviction threshold. Measured directly: `TestSchedule_InvariantHoldsAfterEveryCommit` and `TestSchedule_EarliestMatchesBruteForceOracle` both went red — a burst of several near-simultaneous calls (all sharing one real "now") could have an EARLY member of the burst wrongly evicted merely because a LATER member's own granted instant landed far enough in the future to make the earlier grant look "expired" relative to that future instant, even though real time had not actually advanced. Fixed by separating `commit` (insert only, no eviction) from a new `evict(now)` (time-based, using the caller's real "now"), with `Limiter.acquire` calling `evict(now)` once per acquisition before deciding — never per commit. Re-ran both tests plus the full suite green afterward. This is a `Kind: validation` finding for the red-first methodology itself (the spawn's own framing: "a row that stays green is itself a finding") — here the STRUCTURAL tests (brute-force oracle, invariant-after-commit), not the seven named broken-variant rows, are what caught it, confirming the design's own point that an instant-list assertion is not enough and a checkable postcondition is what matters.

## Key discoveries (don't re-investigate)

- telego's stock `net/http` caller discards the HTTP status code and reports 5xx as an untyped error, so the §13.2 health numbers can only be observed by replacing `telegoapi.Caller`.
- `MultipartRequest` returns `BodyStream` (an `io.Pipe`) with `BodyRaw` nil, so a file upload's `chat_id` is unreachable at the caller seam — hence the three-state `ChatNone`/`ChatUnknown`/`ChatKnown`.
- `url.Error.Error()` prints the full URL, and the URL carries the bot token; `stripPassword` redacts only userinfo. Store `url.Error.Unwrap()` and run rendered strings through a token replacer.
- `net/http` retries a POST internally only when an `Idempotency-Key` header is present — never set one, or "never retry on doubt" is silently defeated.
- `testing/synctest` with a `net.Pipe` dialer replaces an injectable clock entirely; production code needs no clock abstraction.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC33 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| — | — | — | — | — |
