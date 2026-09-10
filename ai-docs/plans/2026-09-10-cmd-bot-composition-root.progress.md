# Progress: cmd/bot composition root — ACTIVE
_Updated: 2026-09-10 14:26_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-10-cmd-bot-composition-root
**base_commit:** a56c6a93eba5db7b540b4a23f7bed1d2e59c26a1
**Last build:** not run
**Issue:** #24
**Spec:** ai-docs/plans/2026-09-10-cmd-bot-composition-root.spec.md
**current_step:** Step 8 — Implementation start
**last_passed_gate:** design-review GO (round 5) | 2026-09-10T14:26:34Z | a56c6a93eba5db7b540b4a23f7bed1d2e59c26a1
**entry_args:** 24

## Next action

**Do this immediately:** hand off Group A (subtasks 1–8) to `code-writer` through `/context-reset`, per the design's `## Handoff plan`.

## Subtasks

- [ ] 1. `internal/srcguard` — shared source-walk guard support  ← CURRENT
- [ ] 2. `internal/config` — the `LAB_GAME_PROCESS_` optional-with-default class
- [ ] 3. `internal/store` — liveness migration, `MigrateOption`/`WithAdvisoryLock`, `ProcessLockID`, pending query
- [ ] 4. `internal/scheduler` — `Liveness` (`AbsorbDowntime`, `Refresh`, `Run`, `Stop`)
- [ ] 5. `internal/scheduler` — `(*Worker).Stop()`
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
