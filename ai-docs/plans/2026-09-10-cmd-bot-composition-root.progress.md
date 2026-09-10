# Progress: cmd/bot composition root — ACTIVE
_Updated: 2026-09-10 14:26_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-10-cmd-bot-composition-root
**base_commit:** a56c6a93eba5db7b540b4a23f7bed1d2e59c26a1
**Last build:** not run
**Issue:** #24
**Spec:** ai-docs/plans/2026-09-10-cmd-bot-composition-root.spec.md
**current_step:** Step 8 — subtask 4 of 15 complete
**last_passed_gate:** go build/test(-race)/vet/lint GREEN (internal/scheduler against real Postgres) | subtask 4 commit (this commit)
**entry_args:** 24

## Next action

**Do this immediately:** hand off Group A (subtasks 1–8) to `code-writer` through `/context-reset`, per the design's `## Handoff plan`.

## Subtasks

- [x] 1. `internal/srcguard` — shared source-walk guard support
- [x] 2. `internal/config` — the `LAB_GAME_PROCESS_` optional-with-default class
- [x] 3. `internal/store` — liveness migration, `MigrateOption`/`WithAdvisoryLock`, `ProcessLockID`, pending query
- [x] 4. `internal/scheduler` — `Liveness` (`AbsorbDowntime`, `Refresh`, `Run`, `Stop`)
- [ ] 5. `internal/scheduler` — `(*Worker).Stop()`  ← CURRENT
- [ ] 6. `internal/ingest` — `(*Loop).Stop()` + the `getUpdates`-scoped cancel
- [ ] 7. `internal/health` — `ReadyFunc`, `ServerOptions`, `/readyz`, the `Process` collector
- [ ] 8. `internal/testdb` — `SchemaDSN`
- [ ] 9. `cmd/bot` — settable `version`, logger, stderr identity, `readiness`, `assemble`
- [ ] 10. `cmd/bot` — `runner` set, `serve`, `drain`, migrate-only
- [ ] 11. `cmd/bot` — `run`'s final shape (argv dispatch)
- [ ] 12. `cmd/bot` — whole-root tests, guards, smoke, link-time version
- [ ] 13. `cmd/importguard` + gate wiring (`make import-guard`, CI)
- [ ] 14. The lifecycle document (AC2 + AC3)
- [ ] 15. Propagation sweep

## Decisions log

- **Step 8**: groups are A=1–8, B=9–13 (both `code-writer`, model pinned by frontmatter), C=14–15 (`general-purpose`, inherit) — taken from the design's `## Handoff plan`, not re-derived.
- **Subtask 1**: `internal/srcguard` — `PackageFiles`/`WalkSubtree`/`TestFilesOnly`/`ParseFile`/`ParseFiles`/`ImportPaths`/`WriteScratchFile`/`WriteScratchFileIn`. `internal/health` and `internal/ingest` guard plumbing (`walkGoFilesUnder`/`parseGoFile`/`importPaths`, `ingestNonTestFiles`/`walkIngestSource`/`parseIngestSource`) now delegate to it; every predicate unchanged. Commit 803e694. Comment-refs required removing bare-extension/example-path literals (`.go`, `_test.go`, `internal/ingest`, etc.) from doc comments — the gate is stricter than a first `make comment-refs` pass over already-committed content suggested, since the pre-commit hook runs against the newly staged content.
- **Subtask 2**: `internal/config/process.go` — `Process` struct (`MigrateOnStart`, `ShutdownTimeout`, `LivenessInterval`, `DowntimeThreshold`), `processEnvKeys()`, `defaultProcess()`, `loadProcess()`, all four `LAB_GAME_PROCESS_*` keys with the design's defaults (`true`, `30s`, `30s`, `5m`). Added `lookupBool` to `transport.go`'s lookup-helper family. Wired into `EnvKeys()`, `Config.Process` and `Load` in `config.go`/`env.go`; `.env.example` gained the four rows. Every existing disjointness/manifest test (`TestEnvExample_MatchesLoaderAndEnvKeys`, `TestLoad_ExampleEnvironmentSucceeds`) covers the new class automatically.
- **Subtask 3**: `internal/store/migrations/00005_process_liveness.sql` — the `process_liveness` guarded singleton (NULL-seeded `seen_at`), the `ingest_offset` shape. `migrate.go` gained `MigrateOption`/`WithAdvisoryLock(id)` (goose `lock.NewPostgresSessionLocker`/`WithLockID`), `ProcessLockID = lock.DefaultLockID`, and `HasPendingMigrations` (`Provider.HasPending`, which goose documents as ignoring a configured locker). Updated `migrate_test.go`'s exact base-table set (+`process_liveness`) and the re-apply `goose_db_version` count (5→6, one row per migration file plus goose's own version-0 bootstrap row, now 6 files). New `migrate_process_test.go`: NULL-seed assertion, singleton-CHECK refusal, `HasPendingMigrations` true→false across `Migrate`, `HasPendingMigrations` unblocked by a held advisory lock, and two concurrent `Migrate` calls under one shared **test-local** lock id applying the migration set exactly once (never `ProcessLockID`, which is deliberately database-wide). `go mod tidy` confirmed no `go.mod`/`go.sum` delta (`goose/v3/lock` is already part of the existing goose dependency). Full `internal/store` suite green against a real Postgres container.
- **Subtask 4**: `internal/scheduler/liveness.go` — `Liveness`/`LivenessOptions`/`Downtime`, `NewLiveness`, `AbsorbDowntime` (one transaction: `FOR UPDATE` locking SELECT computing `now() - seen_at` in SQL as a `pgtype.Interval`, conditional shift of `scheduled_task` reusing that same scanned interval value as the UPDATE parameter — never recomputed — then the `seen_at` write), `Refresh`, `Run` (ticker loop, `sync.Once`-guarded `stopCh`, `failureTolerance` = `ceil(DowntimeThreshold/Interval)`), `Stop`. Added `reasonMustNotBeNil` shared constant (worker.go) to dedupe the `goconst`-flagged `"must not be nil"` literal across `New`/`NewLiveness`. Fixed a real bug caught by the test suite: an unconditional deferred `tx.Rollback` after a successful `Commit` returned `pgx.ErrTxClosed` via `errors.Join`, turning every success into a reported error — replaced with explicit per-branch `_ = tx.Rollback(ctx)` matching `execute.go`'s existing convention (no deferred rollback-after-commit). `liveness_test.go` + `liveness_tracer_test.go` cover T4's full scenario list (seed/shift/threshold/dead-and-future/rerun/ledger-untouched/concurrent-lock/interval-write-count/stop-seam/failing-heartbeat-tolerance) plus the AC14 clock guard (via `srcguard`, proven discriminating against a scratch `time.Now` call). Full `internal/scheduler` suite green including `-race`.

## Key discoveries (don't re-investigate)

- Coverage ratchet headroom is ~12 uncovered statements at the recorded mark, so every subtask ships its tests with its code; this is why the decomposition pairs them.
- `cmd/bot` becomes the 5th `testdb.Main` caller in subtask 9 — `Binaries` and the ceiling arithmetic move in that same subtask so the manifest test goes red→green inside one commit.
- Design rounds ran to 5 under an owner-raised cap (`cap: 5 (was 3)`); GO at round 5, all GO notes folded in before Step 8.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC36 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
