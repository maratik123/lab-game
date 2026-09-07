# Progress: Update ingestion — long polling, dispatch, operation idempotency, chat allowlist — ACTIVE
_Updated: 2026-09-07 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-06-update-ingestion-dispatch-idempotency
**base_commit:** a09d26f56df4b72ab15c49e328b007b900668ca4
**Last build:** PASS
**Issue:** #22
**Spec:** ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md
**current_step:** Step 9 — Verify (ALL PASS)
**last_passed_gate:** make verify | 2026-09-07T00:24:28Z | ce6a11d11721d7e6026aca9ac346c9b45391ed04
**entry_args:** 22

## Next action

**Do this immediately:** Step 9.5 — append this task's entry to `ai-docs/context-status.md` with the literal `#TBD-at-Step-12` PR locator, then Step 10 (self-review).

**Environment blocker, live:** a RAID6 scrub on `md127` is running (`/proc/mdstat`, ~357 min remaining as of 2026-09-07T00:20Z). It saturates disk I/O, so `initdb` inside a testcontainer needs ~90s while testcontainers allows 60 — every database-backed package then fails with `"database system is ready to accept connections" matched 0 times`. That is the apparatus, not the tree. The whole Step-9 gate list was run green through the documented escape hatch instead: a hand-started Postgres plus `LAB_GAME_TEST_DSN=postgres://labgame:labgame@127.0.0.1:55432/labgame_test?sslmode=disable` (container `labgame-step9`). Any delegate that runs the suite without that variable will report a false red.

## Subtasks

- [x] 1. `internal/backoff`: `Exponential` + `EqualJitter`, table and monotonicity tests (Group A)
- [x] 2. Adopt `internal/backoff` in `internal/tg` and `internal/scheduler` — call-site gate first, then mutation probe, then re-point (Group A)
- [x] 3. `config.Ingest` + the `LAB_GAME_INGEST_` reader and both `Load` cross-checks (Group A)
- [x] 4. Migration `00004_ingest.sql`: `ingest_offset`, `ingest_dead_update`; extend the exact base-table assertion (Group A)
- [x] 5. `store.Queryer` + `store.PlayerExists`; delete and re-point `queryRower` (Group A)
- [x] 6. `internal/ingest` core types: `Kind`, `IDSpace`, the `operation_id` builder, `Handler`/`Update`, `Router` (Group A)
- [x] 7. Offset and give-up storage; the package's `TestMain` lands here (Group B)
- [x] 8. The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer` (Group B)
- [x] 9. The loop: `Options`/`New`, requested kinds, `PollOnce`, `Run` with D19's poll-error policy (Group B)
- [x] 10. The gate: `PlayerLookup`, pool-backed lookup, `Gate`/`NewGate`, the positive-only cache (Group B)
- [x] 11. Guard tests and the structural source walks (Group B)
- [x] 12. Propagation: `context.md`, `domain-invariants.md` (Group C)

## Decisions log

- **Step 7**: design-review reached GO on round 5; the owner raised the round cap to 5 (was 3) after round 3, and every round found new material rather than re-opening an earlier one.
- **Step 9**: the whole gate list ran green, but only through `LAB_GAME_TEST_DSN` against a hand-started Postgres — a RAID6 scrub makes container `initdb` exceed testcontainers' 60s wait, so a bare `go test ./...` reports four false FAILs. No panic-index row was added (the package has none), and no event-dictionary or posting-signature entry was needed because this task moves no balance.
- **Step 8 group A**: the orchestrator re-ran the subtask-2 mutation probe itself after the re-point (`Exponential(k-1)` → `Exponential(k)` at `settle.go:143,243` over a cp backup): RED at both sites with the predicted 200ms discrepancy, restored from backup, tree clean. The literal-ramp gate discriminates.
- **Step 7**: the four round-5 GO notes were folded into the design before Step 8, per Step 8's first-action rule; none was spec-amending, so no Spec Amendment recipe ran.
- **Subtask 1**: `internal/backoff.Exponential`/`EqualJitter` implemented per D2's full contract (in-domain, base>ceiling, and the out-of-domain rows). `go test ./internal/backoff/...` green, `golangci-lint run`/`fmt -d` clean, `go vet` clean.
- **Subtask 2**: ran the three-step spawn contract exactly as ordered. (a) Landed a literal one-based ramp (`map[int]time.Duration{1: 200*time.Millisecond, 2: 400*time.Millisecond}`) in `failure_test.go` and `deadline_test.go`, and ran `go test ./internal/scheduler/ -run 'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp|TestDeadline_successiveBreaches_growingDelay'` — GREEN against the still-shipped one-based `backoff`, confirming the literals equal what the shipped ramp computes at those attempts. (b) Wrote `backoff(k+1, cfg.RetryBaseDelay, cfg.RetryMaxDelay)` at both `settle.go` call sites over a `cp` backup, re-ran the same two tests — RED, both failing on the run_at bracket by the expected margin — then restored `settle.go` from the `cp` backup (verified `git diff` empty on it afterward). (c) Deleted `internal/tg`'s `backoffDelay` and `internal/scheduler`'s `backoff`, re-pointed every call site (`caller.go`, `retry_test.go`, `settle.go` with the `k-1` translation, `cadence_test.go`'s two ramp tests with the `tc.failures-1`/`f-1` translation in the argument only) — every pre-existing assertion and expected value in those four files stays byte-identical. `go test ./internal/tg/ ./internal/scheduler/` green as its own step; `go build ./...`, `go vet ./...`, `golangci-lint run ./...`, `golangci-lint fmt -d` (whole module) all clean.
- **Subtask 3**: `config.Ingest` added mirroring `Transport`/`Scheduler`'s optional-with-default shape exactly (`loadIngest`, `defaultIngest`, `ingestEnvKeys`, appended to `EnvKeys()`). `loadIngest` validates `BatchLimit` against the Bot API's 1-100 range locally; `checkIngestLongPollTimeout` (D16's pair — strictly below `Transport.AttemptTimeout`, and a whole number of seconds) runs from `Load` once both `Ingest` and `Transport` have loaded successfully, naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` either way. `.env.example` gained the six new lines with defaults matching `defaultIngest()` exactly (asserted by `TestLoadIngest_ExampleMatchesDefaults`). `go test ./internal/config/...` green (60+ subtests, including the D16 boundary cases: `25500ms`/`1500ms` rejected, `25s` accepted, and the equal/above/below-`AttemptTimeout` trio), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build ./...` clean.
- **Subtask 4**: migration `00004_ingest.sql` adds `ingest_offset` (guarded singleton — `PRIMARY KEY (id)` plus `CHECK (id = 1)`, seeded at `next_update_id = 0` by the migration itself) and `ingest_dead_update` (update_id, kind with a non-empty CHECK, nullable chat_id, consecutive_failures with a positive CHECK, last_error, created_at). No `-- +goose Down` section. `migrate_test.go`'s exact base-table set and `goose_db_version` row count (4 → 5) both extended; `schema_test.go` gained four new subtests (singleton-refused, seeded-exactly-once, empty-kind-refused) under `TestSchema_constraints`. `go test ./internal/store/...` green including `TestMigrate_hygiene/00004_ingest.sql`, `golangci-lint run`/`fmt -d` clean, `go vet` clean — `internal/store` is green on its own; the tables' consumer (`internal/ingest`) lands in Group B.
- **Subtask 5**: `store.Queryer` (the single `QueryRow` method) and `store.PlayerExists(ctx, q, telegramID)` added to `owner.go`, over the `(kind, telegram_id)` pair per D10/D11 — a chat sharing a player's telegram_id must report false, pinned by `TestPlayerExists_kindPairIsThePredicate`. `post_test.go`'s shipped `queryRower` (consumed only by `balanceOf`) deleted and `balanceOf` re-pointed at `store.Queryer` — compile-gated, no assertion moved. `go test ./internal/store/...` green including the four new `PlayerExists` tests (kind-pair, no-owner, closed-pool-surfaces-error, tx-and-pool-agree), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build ./...` clean.
- **Subtask 6**: new `internal/ingest` package. `Kind` is a 26-row explicit table (`kind.go`) — one row per Bot API update type, each an explicit `present` probe plus optional `date`/`chatID` extractors supplied per PAYLOAD TYPE per D4 (`*telego.Message`'s 7 kinds share `messageDate`/`messageChatID`; `KindMyChatMember`/`KindChatMember` share `chatMemberDate`/`chatMemberChatID`; every other kind — including `KindCallbackQuery`, which declares no date field at all — carries nil extractors). `Derive`/`Date`/`ChatID` are total: an update matching no row yields the zero `Kind` (unrouted). `IDSpace` + the unexported `operationID` builder implement D9's `"<space>:<id>"` grammar over `IDSpaceUpdate`/`IDSpaceCallbackQuery`, refusing an empty space (`ErrEmptyIDSpace`) or id (`ErrEmptyID`). `Update` holds the raw `telego.Update` in a NAMED field (never embedded, per D9's context-leak rationale); `NewUpdate` derives `Kind` and both operation ids. `Handler`'s doc comment states all three D18 obligations (ctx propagation, never read the update's own riding context, no outbound call on an uncommitted row). `Route`/`Router`/`NewRouter`/`Kinds` refuse an unknown kind (`ErrUnknownKind`) or a duplicate route (`ErrDuplicateRoute`), `Kinds()` sorted deterministic. `go test ./internal/ingest/...` green (`TestDerive_perRow` covers all 26 table rows plus the no-payload zero-kind case — 26/26 subtests pass), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build`/`vet`/`golangci-lint run` clean, `go mod tidy` a no-op (no new dependency). **Group A (subtasks 1-6) is now complete** — handoff to Group B (subtasks 7-11) is the orchestrator's next step per the design's Handoff plan.
- **Subtask 7**: `readOffset`/`advanceOffset` (`offset.go`) implement D5/D14's guarded-monotone `next_update_id` advance; `readOffset` takes a `store.Queryer` (satisfied by both `pgx.Tx` and `*pgxpool.Pool`) rather than a bare `pgx.Tx`, anticipating subtask 9's poll-time read off the pool with no transaction of its own. `writeDeadUpdate` and the exported `DeadUpdates(ctx, tx, limit)` (`dead.go`) give `ingest_dead_update` its insert and its deterministic (`created_at, id`) projection over a caller-owned transaction it neither commits nor rolls back. The package's `TestMain` over `testdb.Main` lands here (`main_test.go`), per the design's dependency note. Also closed subtask 6's pre-existing `NewUpdate` coverage gap (0% before this subtask) that the coverage ratchet's whole-module measurement surfaced once this subtask's own new statements were added — added to `router_test.go`. `go test ./internal/ingest/...` green (97.0%→96.7% package coverage across the two commits), `golangci-lint run`/`fmt -d` clean, `go vet` clean, whole-module `go build` clean. Ratchet: 90.27%→90.50%.
- **Subtask 8**: `observe.go` — `Outcome` (Handled/Duplicate/Unrouted/Failed/Panic/GivenUp, `String()` with a `default` fallback for `exhaustive`), `Observation` (kind, attempt, outcome, duration, and a `Lag`/`LagKnown` pair per D13 — a kind with no date field must never render as a healthy zero lag), `LoopObservation`, `Observer`, and the nil-checked `observeUpdate`/`observeLoop` report helpers (AC19). `recordingObserver` (mutex-guarded, race-clean) is written in `observe_test.go` for reuse by subtasks 9 and 10, mirroring `internal/scheduler`'s own shape. `go test ./internal/ingest/...` green (97.0% package coverage), gates clean. Ratchet: 90.50%→90.57%.
- **Subtask 9**: the loop, split per the design's named seams — `loop.go` (`Options`/`New`/`OptionError`, `PollOnce`, `Run`), `attempt.go` (`runAttempts`/`attemptOnce`/`safeHandle`: one transaction per attempt, `recover`, D8's `errors.Is(err, store.ErrAlreadyPosted)` sentinel classification), `settle.go` (the three attempt-less settlements — unrouted, duplicate-after-rollback, given-up — each advancing the offset in a transaction of its own per D5). D3's sentinel (`telego.ShippingQueryUpdates`) is transmitted only while `Router.Kinds()` is empty; `PollOnce` reads the offset off the pool directly (no transaction) and relies on `encoding/json`'s `omitempty` to erase a zero offset automatically — no special-casing needed at the call site. Test coverage: request-shape (sentinel/registered-kinds/timeout/offset-omitted-at-zero), happy path, transmitted-offset-after-settlement (the D14 live-lock scenario), unrouted, duplicate (via an offset rewind to redeliver the same `update_id`), option validation, failure-and-retry-to-give-up, panic recovery, poll-failure-does-not-stop-`Run` (D19, with a busy-loop floor assertion), cancellation-leaves-update-unsettled, and long-poll-cancellation-returns-promptly. `go test ./internal/ingest/...` and `go test -race ./internal/ingest/...` both green, gates clean, `go.mod`/`go.sum` unchanged. Ratchet: 90.57%→90.15% (held; within the 0.50pp tolerance against the 90.57% high-water mark).
- **Subtask 10**: `gate.go` — `PlayerLookup` (consumer-declared per D11), `poolPlayerLookup`, `Gate`/`NewGate`/`NewPoolGate` implementing `tg.Gate` per D10 (ChatNone allowed, ChatUnknown refused, ChatKnown checked against the allowlist then the mutex-guarded positive-only cache then `PlayerLookup`, with the lookup call outside the lock). `TestGate_uncommittedOwnerRowIsInvisible` pins D18 directly against a real schema: open a tx, `CreateOwner` a player row, `AllowCall` refused before commit, allowed after — same `Gate`, same pool, only the commit varies. `TestGate_concurrentAllowCall` drives many goroutines (including several racing on the same not-yet-cached id) under `go test -race`. An integration scenario wires the `Gate` into a real `tg.Client` against a `tgtest.Server` and shows a refused call never reaches the server. `go test ./internal/ingest/...` and `-race` both green, gates clean. Ratchet: 90.15%→90.32% (held).
- **Subtask 11**: `guards_test.go` — source-text scans (no `panic(`/`log.Fatal`/`os.Exit`, no metrics-registry import, no `telego.Bot`/`http.Client`/`telegoapi` construction of this package's own) plus `go/ast`/`reflect` walks (D4's drift check over `telego.Update`'s exported pointer fields against `kind.go`'s table; AC8's no-exported-func/method-returns-`pgx.Tx`/`pgx.Conn` and no-exported-field-of-either, with `DeadUpdates` — takes a caller-owned tx — and `Options.Pool` — a `*pgxpool.Pool`, not a transaction — both legal by construction since the walk checks results/fields, never parameters; D9's ctx-first walk with a commented exemption set for pure/no-I/O exported methods, no `context.Context` field on any declared type, no embedding of `telego.Update`, and no selector call of `.Context()`/`.WithContext()` — with the `context.Context` TYPE reference itself excluded from that last check, which the first run caught as a false positive and was fixed before commit). `go test ./internal/ingest/...` and `-race` both green (91.7% package coverage), gates clean, whole-module `go build`/`go vet`/`golangci-lint run` clean, `go.mod`/`go.sum` unchanged. Ratchet: 90.32%→90.27% (held). **Group B (subtasks 7-11) is now complete** — handoff to Group C (subtask 12, propagation) is the orchestrator's next step per the design's Handoff plan.

## Key discoveries (don't re-investigate)

- The shipped scheduler suite is **invariant** to D2's one-based→zero-based call-site translation, not blind to it: `failure_test.go:154` and `deadline_test.go:252` both compute `want :=` through the same function `settle.go:142,242` calls. The literal-ramp gate plus the mutation probe are the only instruments that separate a correct translation from an incorrect one.
- The mutation probe must be written as `backoff(k+1, …)` in the shipped ramp's own units: while `func backoff` still stands, no file of `internal/scheduler` can import `internal/backoff` — the name collision is package-wide, not per-file.
- `encoding/json` erases an **empty** slice under `omitempty`, not only a nil one; the same tag sits on `GetUpdatesParams.Offset`, so a stored offset of `0` is legitimately absent from the request body.
- A handler must not issue an outbound call whose permission rests on a row its own uncommitted transaction created (D18) — a Telegram send is not rollback-able, so this holds even if the gate were made transaction-aware.

## AC Status

Taken at `ce6a11d11721d7e6026aca9ac346c9b45391ed04`, with `make verify` green end to end. Every row was checked with the orchestrator's own command rather than with the AC's stated scope alone.

| AC | Status | Verification |
|----|--------|--------------|
| AC1-AC2 | PASS | `TestGuard_CtxFirstAndNoRidingContext`; `revive` `exported` / `package-comments` under `golangci-lint run` |
| AC3 | PASS | grep for `http.Client{` and `telego.NewBot` over non-test `internal/ingest` finds nothing; `TestGuard_NoOwnBotAPIPath` |
| AC4-AC5 | PASS | `TestPollOnce_requestShape` |
| AC6-AC7 | PASS | `ingest_offset` created in `00004_ingest.sql`; the file carries no `goose Down` section; `TestReadOffset_freshSchemaReadsSeededZero` |
| AC8 | PASS | `TestGuard_NoTransactionEscapesTheHandlerContract` |
| AC9-AC10 | PASS | `TestLoop_unrouted`, `TestLoop_panicIsRecoveredAndRetried` |
| AC11 | PASS | grep for `panic(`, `log.Fatal`, `os.Exit` over non-test `internal/ingest` and `internal/backoff` finds nothing; `ai-docs/panic-index.md` gains no row |
| AC12-AC14 | PASS | `TestLoop_duplicate`, `TestOperationID_grammar`, `TestOperationID_differentSpacesSameRawID` |
| AC15-AC16 | PASS | `TestGate_chatNoneAllowed`, `TestGate_chatUnknownRefused`, `TestGate_integrationRefusedCallNeverReachesTheServer` |
| AC17-AC19 | PASS | `TestObservation_lagAbsentIsDistinguishableFromZeroLag`, the four `TestObserve*` nil-observer rows, `TestGuard_NoMetricsLibraryImport` |
| AC20-AC21 | PASS | the diff against the base names none of `00003_event_log.sql`, `store/event.go`, `store/post.go`, `store/basis.go`, `store/errors.go` |
| AC22-AC26 | PASS | `TestLoop_failureAndRetryGivesUp`, `TestRun_cancellationLeavesTheUpdateUnsettled`, `TestPollOnce_cancellationDuringLongPollReturnsPromptly`, `TestRun_pollFailureDoesNotStopTheLoop` |
| AC27 | PASS | the six `LAB_GAME_INGEST_*` keys agree between `internal/config/ingest.go` and `.env.example`; `TestNew_optionValidation` |
| AC28, AC33-AC35 | PASS | `TestGate_playerOwnerAllowsAChatOutsideTheAllowlist`, `TestGate_noOwnerRefused`, `TestGate_nonIntegerTokenRefused`, `TestGate_lookupErrorRefuses`, `TestGate_refusedThenAllowedWithNoRestart`, `TestGate_uncommittedOwnerRowIsInvisible` |
| AC29 | PASS | the suite runs on `internal/tgtest` and `internal/testdb` with no mock of the package's own making |
| AC30 | PASS | `make verify` exits zero |
| AC31 | PASS | `coverage-ratchet.sh --check`: 90.27% holds against 90.57%, tolerance 0.50 pp |
| AC32 | PASS | removed-name sweep re-run by the orchestrator: `queryRower` and `backoffDelay` have no live-doc reference; KD-31's mention is deliberate past tense |
| AC36-AC37 | PASS | `ingest_dead_update` declares no payload column; `TestDeadUpdates_deterministicOrderAndLimit`, `TestDeadUpdates_neitherCommitsNorRollsBackTheCallersTx` |
| AC38 | PASS | `TestGuard_NoOwnBotAPIPath` plus `internal/tg/guards_test.go`'s `TestGuard_RefusingGateBlocksTheAccessor` |

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
- `internal/ingest/doc.go`, `internal/ingest/kind.go`, `internal/ingest/kind_test.go`, `internal/ingest/operation.go`, `internal/ingest/operation_test.go`, `internal/ingest/router.go`, `internal/ingest/router_test.go`, `internal/ingest/errors.go` (all new; `errors.go` and `router_test.go` also touched in Group B)
- `internal/ingest/offset.go`, `internal/ingest/offset_test.go`, `internal/ingest/dead.go`, `internal/ingest/dead_test.go`, `internal/ingest/main_test.go` (all new, subtask 7)
- `internal/ingest/observe.go`, `internal/ingest/observe_test.go` (all new, subtask 8)
- `internal/ingest/loop.go`, `internal/ingest/attempt.go`, `internal/ingest/settle.go`, `internal/ingest/loop_test.go`, `internal/ingest/retry_test.go` (all new, subtask 9)
- `internal/ingest/gate.go`, `internal/ingest/gate_test.go` (all new, subtask 10)
- `internal/ingest/guards_test.go` (new, subtask 11)
