# Progress: Bot API transport over telego — ACTIVE
_Updated: 2026-09-04 17:50_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-bot-api-transport
**base_commit:** c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**Last build:** not run
**Issue:** #19
**Spec:** ai-docs/plans/2026-09-04-bot-api-transport.spec.md
**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | 2026-09-04T17:49:58Z | c1d03fde428a1b84d00f822e8e3f3c42a1b093cb
**entry_args:** 19

## Next action

**Do this immediately:** hand off Group A (subtasks 1–6) to `code-writer` via `/context-reset`, per the design's `## Handoff plan`; start with subtask 1 (`internal/config` transport key class + `.env.example`).

## Subtasks

- [ ] 1. `internal/config`: optional-with-default transport key class, `.env.example`, falsified doc comments  ← CURRENT
- [ ] 2. `internal/tgtest`: in-process fake Bot API server
- [ ] 3. `internal/tg` foundations + the telego dependency (`go get`)
- [ ] 4. `internal/tg` limiter: the window schedule + the minimal caller
- [ ] 5. `internal/tg` caller: retry loop, backoff, `retry_after`, typed error, observation
- [ ] 6. `internal/tg` package-level guard tests
- [ ] 7. `ai-docs/key-decisions.md`: KD-2 rewrite + the new decisions

## Decisions log

- **Step 6–7**: design reached GO at review round 7; the first mechanism (composed limiter legs) was abandoned at the owner's direction after three rounds produced four defects of one shape, and replaced by a single per-key schedule holding a set of `(count, per)` windows.
- **Step 7**: owner decisions recorded — keep `LAB_GAME_TG_ATTEMPT_TIMEOUT` (30s); defer `stdjson` to its own PR; stay on Go 1.26.5 with telego pinned at v1.11.2; leave `docs/DESIGN.md` §11 untouched; media stays out of MVP with no media mechanism; no "exactly one sending process" constraint recorded.
- **Step 7**: the telego v1.12.x ceiling is a configuration block, not an incompatibility — v1.12.1 builds clean under go1.26.5 once its own directive is lowered; raising the toolchain is deferred to its own task.

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
