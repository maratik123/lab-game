# Progress: health metrics and canaries — ACTIVE
_Updated: 2026-09-09 16:14_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-09-health-metrics-canaries
**base_commit:** 3616d527e04ea6abf7a1f72de041140b297e96a9
**Last build:** not run
**Issue:** #23
**Spec:** ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md
**current_step:** Step 8 — subtask 0 of 13 (Group A not yet spawned)
**last_passed_gate:** not run
**entry_args:** 23

## Next action

**Do this immediately:** spawn Group A (subtasks 1–6) through `/context-reset` with `subagent_type="code-writer"`, per the design's `## Handoff plan`.

## Subtasks

- [ ] 1. `internal/repotest` — the shared root helper, every existing declaration replaced  ← CURRENT
- [ ] 2. `go get github.com/prometheus/client_golang@v1.24.1`, `go mod tidy`
- [ ] 3. The `LAB_GAME_HEALTH_` configuration class
- [ ] 4. Package skeleton: registry, labels, buckets, the D16 register
- [ ] 5. Transport adapter (`tg.Observer`)
- [ ] 6. Scheduler adapter (`scheduler.Observer`)
- [ ] 7. Ingest adapter (`ingest.Observer`), `LagKnown` gate
- [ ] 8. pgx pool collector
- [ ] 9. `promhttp` endpoint server
- [ ] 10. The canaries: `Prober`/`ProberFactory`, both legs, the ticker
- [ ] 11. The structural guards (a)–(i)
- [ ] 12. The alert contract
- [ ] 13. Propagation sweep

## Decisions log

- **Step 7**: design-review never returned GO; the owner directed Step 8 after review round 6 regardless of verdict, and raised the design cap three times (3→4→5→6) along the way.
- **Step 7**: the test-helper consolidation is the design's own decision, not acceptance for #23. It entered the spec on the orchestrator's instruction after a note-severity finding and was removed again at d63c7a1 / 3616d52; AC29 keeps only what it protects.
- **Step 8**: group order follows the design — A = 1–6, B = 7–11 pinned as 10, 7, 8, 9, 11, C = 12–13.

## Key discoveries (don't re-investigate)

- Six file-location-ascent root resolvers exist, not four: the four `repoRootPath` copies plus `repoRoot` in `internal/commentref` and `internal/testdb`. Found by behaviour (`runtime.Caller`), not by identifier.
- `cmd/commentrefs` holds two git-based resolvers (`repoRoot`, `repoRootForTest`) that are OUT of the class by mechanism — they ask git, never their own file location.
- `client_golang@v1.24.1` is newest published. `newHistogram` panics on non-increasing buckets and on an `le` label; an EMPTY bucket slice silently becomes `DefBuckets` instead.
- `internal/ingest` and `internal/tg` each ban `client_golang`, but both guards walk only their own package — `internal/health` importing it trips neither.
- A pool built against an unreachable DSN still answers `Stat()`, so this package's tests need no Postgres.

## AC Status

| AC | Status |
|----|--------|
| AC1–AC34 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

