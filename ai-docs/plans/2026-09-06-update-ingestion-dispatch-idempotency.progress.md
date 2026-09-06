# Progress: Update ingestion — long polling, dispatch, operation idempotency, chat allowlist — ACTIVE
_Updated: 2026-09-06 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-06-update-ingestion-dispatch-idempotency
**base_commit:** a09d26f56df4b72ab15c49e328b007b900668ca4
**Last build:** PASS
**Issue:** #22
**Spec:** ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md
**current_step:** Step 8 — Group A complete (subtasks 1-6), gates re-verified by the orchestrator; Group B pending
**last_passed_gate:** go test -race ./... | 2026-09-06T23:30:02Z | 0b86c19cd135edbfa4cc83a7c08099f6f2ee10c6
**entry_args:** 22

## Next action

**Do this immediately:** start Group B at subtask 7.

**Design:** `ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.design.md` — Group B is subtasks 7-11, all `internal/ingest`. Subtask 7 carries the package's `TestMain` over `testdb.Main`, because `testdb.Schema` fatals any test that calls it before `Main` has provisioned a database; 9 and 10 depend on that landing first. Subtask 9 implements D19's poll-error policy (observe through `LoopObservation.Err` and continue at the poll interval; `Run` returns non-nil only on cancellation) and D3's `allowed_updates` sentinel; subtask 10 implements D10's positive-only, mutex-guarded cache and D18's refusal of a destination visible only inside an uncommitted transaction — that scenario is a PAIR (refused while uncommitted, allowed after commit, the commit the only variable), not a single case.

## Subtasks

- [x] 1. `internal/backoff`: `Exponential` + `EqualJitter`, table and monotonicity tests (Group A)
- [x] 2. Adopt `internal/backoff` in `internal/tg` and `internal/scheduler` — call-site gate first, then mutation probe, then re-point (Group A)
- [x] 3. `config.Ingest` + the `LAB_GAME_INGEST_` reader and both `Load` cross-checks (Group A)
- [x] 4. Migration `00004_ingest.sql`: `ingest_offset`, `ingest_dead_update`; extend the exact base-table assertion (Group A)
- [x] 5. `store.Queryer` + `store.PlayerExists`; delete and re-point `queryRower` (Group A)
- [x] 6. `internal/ingest` core types: `Kind`, `IDSpace`, the `operation_id` builder, `Handler`/`Update`, `Router` (Group A)
- [ ] 7. Offset and give-up storage; the package's `TestMain` lands here (Group B)
- [ ] 8. The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer` (Group B)
- [ ] 9. The loop: `Options`/`New`, requested kinds, `PollOnce`, `Run` with D19's poll-error policy (Group B)
- [ ] 10. The gate: `PlayerLookup`, pool-backed lookup, `Gate`/`NewGate`, the positive-only cache (Group B)
- [ ] 11. Guard tests and the structural source walks (Group B)
- [ ] 12. Propagation: `context.md`, `domain-invariants.md` (Group C)

## Decisions log

- **Step 7**: design-review reached GO on round 5; the owner raised the round cap to 5 (was 3) after round 3, and every round found new material rather than re-opening an earlier one.
- **Step 8 group A**: the orchestrator re-ran the subtask-2 mutation probe itself after the re-point (`Exponential(k-1)` → `Exponential(k)` at `settle.go:143,243` over a cp backup): RED at both sites with the predicted 200ms discrepancy, restored from backup, tree clean. The literal-ramp gate discriminates.
- **Step 7**: the four round-5 GO notes were folded into the design before Step 8, per Step 8's first-action rule; none was spec-amending, so no Spec Amendment recipe ran.
- **Subtask 1**: `internal/backoff.Exponential`/`EqualJitter` implemented per D2's full contract (in-domain, base>ceiling, and the out-of-domain rows). `go test ./internal/backoff/...` green, `golangci-lint run`/`fmt -d` clean, `go vet` clean.
- **Subtask 2**: ran the three-step spawn contract exactly as ordered. (a) Landed a literal one-based ramp (`map[int]time.Duration{1: 200*time.Millisecond, 2: 400*time.Millisecond}`) in `failure_test.go` and `deadline_test.go`, and ran `go test ./internal/scheduler/ -run 'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp|TestDeadline_successiveBreaches_growingDelay'` — GREEN against the still-shipped one-based `backoff`, confirming the literals equal what the shipped ramp computes at those attempts. (b) Wrote `backoff(k+1, cfg.RetryBaseDelay, cfg.RetryMaxDelay)` at both `settle.go` call sites over a `cp` backup, re-ran the same two tests — RED, both failing on the run_at bracket by the expected margin — then restored `settle.go` from the `cp` backup (verified `git diff` empty on it afterward). (c) Deleted `internal/tg`'s `backoffDelay` and `internal/scheduler`'s `backoff`, re-pointed every call site (`caller.go`, `retry_test.go`, `settle.go` with the `k-1` translation, `cadence_test.go`'s two ramp tests with the `tc.failures-1`/`f-1` translation in the argument only) — every pre-existing assertion and expected value in those four files stays byte-identical. `go test ./internal/tg/ ./internal/scheduler/` green as its own step; `go build ./...`, `go vet ./...`, `golangci-lint run ./...`, `golangci-lint fmt -d` (whole module) all clean.
- **Subtask 3**: `config.Ingest` added mirroring `Transport`/`Scheduler`'s optional-with-default shape exactly (`loadIngest`, `defaultIngest`, `ingestEnvKeys`, appended to `EnvKeys()`). `loadIngest` validates `BatchLimit` against the Bot API's 1-100 range locally; `checkIngestLongPollTimeout` (D16's pair — strictly below `Transport.AttemptTimeout`, and a whole number of seconds) runs from `Load` once both `Ingest` and `Transport` have loaded successfully, naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` either way. `.env.example` gained the six new lines with defaults matching `defaultIngest()` exactly (asserted by `TestLoadIngest_ExampleMatchesDefaults`). `go test ./internal/config/...` green (60+ subtests, including the D16 boundary cases: `25500ms`/`1500ms` rejected, `25s` accepted, and the equal/above/below-`AttemptTimeout` trio), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build ./...` clean.
- **Subtask 4**: migration `00004_ingest.sql` adds `ingest_offset` (guarded singleton — `PRIMARY KEY (id)` plus `CHECK (id = 1)`, seeded at `next_update_id = 0` by the migration itself) and `ingest_dead_update` (update_id, kind with a non-empty CHECK, nullable chat_id, consecutive_failures with a positive CHECK, last_error, created_at). No `-- +goose Down` section. `migrate_test.go`'s exact base-table set and `goose_db_version` row count (4 → 5) both extended; `schema_test.go` gained four new subtests (singleton-refused, seeded-exactly-once, empty-kind-refused) under `TestSchema_constraints`. `go test ./internal/store/...` green including `TestMigrate_hygiene/00004_ingest.sql`, `golangci-lint run`/`fmt -d` clean, `go vet` clean — `internal/store` is green on its own; the tables' consumer (`internal/ingest`) lands in Group B.
- **Subtask 5**: `store.Queryer` (the single `QueryRow` method) and `store.PlayerExists(ctx, q, telegramID)` added to `owner.go`, over the `(kind, telegram_id)` pair per D10/D11 — a chat sharing a player's telegram_id must report false, pinned by `TestPlayerExists_kindPairIsThePredicate`. `post_test.go`'s shipped `queryRower` (consumed only by `balanceOf`) deleted and `balanceOf` re-pointed at `store.Queryer` — compile-gated, no assertion moved. `go test ./internal/store/...` green including the four new `PlayerExists` tests (kind-pair, no-owner, closed-pool-surfaces-error, tx-and-pool-agree), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build ./...` clean.
- **Subtask 6**: new `internal/ingest` package. `Kind` is a 26-row explicit table (`kind.go`) — one row per Bot API update type, each an explicit `present` probe plus optional `date`/`chatID` extractors supplied per PAYLOAD TYPE per D4 (`*telego.Message`'s 7 kinds share `messageDate`/`messageChatID`; `KindMyChatMember`/`KindChatMember` share `chatMemberDate`/`chatMemberChatID`; every other kind — including `KindCallbackQuery`, which declares no date field at all — carries nil extractors). `Derive`/`Date`/`ChatID` are total: an update matching no row yields the zero `Kind` (unrouted). `IDSpace` + the unexported `operationID` builder implement D9's `"<space>:<id>"` grammar over `IDSpaceUpdate`/`IDSpaceCallbackQuery`, refusing an empty space (`ErrEmptyIDSpace`) or id (`ErrEmptyID`). `Update` holds the raw `telego.Update` in a NAMED field (never embedded, per D9's context-leak rationale); `NewUpdate` derives `Kind` and both operation ids. `Handler`'s doc comment states all three D18 obligations (ctx propagation, never read the update's own riding context, no outbound call on an uncommitted row). `Route`/`Router`/`NewRouter`/`Kinds` refuse an unknown kind (`ErrUnknownKind`) or a duplicate route (`ErrDuplicateRoute`), `Kinds()` sorted deterministic. `go test ./internal/ingest/...` green (`TestDerive_perRow` covers all 26 table rows plus the no-payload zero-kind case — 26/26 subtests pass), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build`/`vet`/`golangci-lint run` clean, `go mod tidy` a no-op (no new dependency). **Group A (subtasks 1-6) is now complete** — handoff to Group B (subtasks 7-11) is the orchestrator's next step per the design's Handoff plan.

## Key discoveries (don't re-investigate)

- The shipped scheduler suite is **invariant** to D2's one-based→zero-based call-site translation, not blind to it: `failure_test.go:154` and `deadline_test.go:252` both compute `want :=` through the same function `settle.go:142,242` calls. The literal-ramp gate plus the mutation probe are the only instruments that separate a correct translation from an incorrect one.
- The mutation probe must be written as `backoff(k+1, …)` in the shipped ramp's own units: while `func backoff` still stands, no file of `internal/scheduler` can import `internal/backoff` — the name collision is package-wide, not per-file.
- `encoding/json` erases an **empty** slice under `omitempty`, not only a nil one; the same tag sits on `GetUpdatesParams.Offset`, so a stored offset of `0` is legitimately absent from the request body.
- A handler must not issue an outbound call whose permission rests on a row its own uncommitted transaction created (D18) — a Telegram send is not rollback-able, so this holds even if the gate were made transaction-aware.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC38 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| (none yet — self-review runs at Step 10) | | | | |

## Files touched

- `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go` (new)
- `internal/tg/retry.go`, `internal/tg/caller.go`, `internal/tg/retry_test.go`
- `internal/scheduler/cadence.go`, `internal/scheduler/settle.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go`, `internal/scheduler/deadline_test.go`
- `internal/config/ingest.go`, `internal/config/ingest_test.go` (new), `internal/config/config.go`, `internal/config/env.go`, `.env.example`
- `internal/store/migrations/00004_ingest.sql` (new), `internal/store/migrate_test.go`, `internal/store/schema_test.go`
- `internal/store/owner.go`, `internal/store/owner_test.go`, `internal/store/post_test.go`
- `internal/ingest/doc.go`, `internal/ingest/kind.go`, `internal/ingest/kind_test.go`, `internal/ingest/operation.go`, `internal/ingest/operation_test.go`, `internal/ingest/router.go`, `internal/ingest/router_test.go`, `internal/ingest/errors.go` (all new)
