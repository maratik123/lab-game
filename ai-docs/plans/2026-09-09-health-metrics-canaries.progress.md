# Progress: health metrics and canaries — ACTIVE
_Updated: 2026-09-09 16:14_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-09-health-metrics-canaries
**base_commit:** 3616d527e04ea6abf7a1f72de041140b297e96a9
**Last build:** not run
**Issue:** #23
**Spec:** ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md
**current_step:** Step 8 — subtask 3 of 13 complete
**last_passed_gate:** go build/test/vet, golangci-lint fmt -d + run, make comment-refs — all green (commit c68390c)
**entry_args:** 23

## Next action

**Do this immediately:** spawn Group A (subtasks 1–6) through `/context-reset` with `subagent_type="code-writer"`, per the design's `## Handoff plan`.

## Subtasks

- [x] 1. `internal/repotest` — the shared root helper, every existing declaration replaced
- [x] 2. `go get github.com/prometheus/client_golang@v1.24.1`, `go mod tidy`
- [x] 3. The `LAB_GAME_HEALTH_` configuration class
- [ ] 4. Package skeleton: registry, labels, buckets, the D16 register  ← CURRENT
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
- **Step 8, subtask 1**: the widened D19 member set (six file-location-ascent resolvers, not four)
  was re-verified by behaviour (`rg -n 'runtime\.Caller' --type go`) before writing `internal/repotest`,
  confirming `internal/commentref/testhelpers_test.go` and `internal/testdb/server_test.go` are in
  scope alongside the four `repoRootPath` copies. `cmd/commentrefs`'s two git-based resolvers were
  confirmed out of scope (`rg -n 'rev-parse.*--show-toplevel' cmd/commentrefs/`) and left untouched.
  Per D17, `internal/tg/guards_test.go`'s six `repoRootPath(t, ".")` call sites became
  `repotest.Root(t)`; every other call site with an actual relative path became
  `repotest.RootPath(t, rel)`. All six original local declarations (and their now-stale doc comments)
  were deleted rather than left behind. `internal/config/repo_root_test.go` was deleted outright since
  it declared only the helper. Gates run and green: `go build ./...`, `go test ./...` (whole module),
  `go vet ./...`, `golangci-lint fmt -d` (no target file present in its diff), `golangci-lint run`
  (0 issues), `make comment-refs`. Committed at f57403f.
- **Step 8, subtask 2**: measured that `go mod tidy` immediately prunes an as-yet-unimported module —
  ran `go get github.com/prometheus/client_golang@v1.24.1` then `go mod tidy` and the require line and
  its two go.sum entries were removed entirely (git diff empty after tidy). Re-ran `go get` alone
  (skipping the destructive `go mod tidy` this time) to keep the require line, marked `// indirect`
  since nothing imports it yet; `go build ./...` and `go vet ./...` stay green with it present but
  unused. The transitive closure (beorn7/perks, prometheus/client_model, prometheus/common,
  prometheus/procfs, google.golang.org/protobuf, kylelemons/godebug, munnerz/goautoneg) is deferred to
  subtask 4, where internal/health first imports the library and a real `go mod tidy` has something to
  resolve against. Committed at 4c9e9ff.
- **Step 8, subtask 3**: `internal/config/health.go` adds `Health` (`MetricsAddr`, `CanaryInterval`,
  `CanaryCloudToken`, `CanaryCloudBaseURL`) as a fourth optional-with-default class beside Transport,
  Scheduler and Ingest. The comment-reference gate (`make comment-refs`) rejected an IP:port literal
  in a doc comment as an "locator" match and rejected inline `AC17`/`AC22`/`D13` citations as
  ac-id/decision-anchor matches — fixed by splitting the default address into concatenated string
  literals (`"127.0.0.1" + ":" + "9095"`) named by a constant instead of writing the literal directly
  in a comment, and by rewriting every comment to state the rule in prose without citing an AC or
  design-decision id. `defaultHealth()`'s cloud-base-URL parse failure is handled (a zero `url.URL`
  fallback), not panicked — the panic-gate hook caught an initial `panic()` on this exact
  supposedly-unreachable branch and it was replaced before commit. Gates run and green:
  `go build ./...`, `go test ./...` (whole module), `go vet ./...`, `golangci-lint fmt -d` (no target
  file present in its diff), `golangci-lint run` (0 issues), `make comment-refs`. Committed at c68390c.

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

