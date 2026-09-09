# Progress: health metrics and canaries — ACTIVE
_Updated: 2026-09-09 17:38_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-09-health-metrics-canaries
**base_commit:** 3616d527e04ea6abf7a1f72de041140b297e96a9
**Last build:** not run
**Issue:** #23
**Spec:** ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md
**current_step:** Step 8 — subtask 12 of 13 complete (Group C in progress)
**last_passed_gate:** make comment-refs, the CI relative-markdown-link check, go test ./internal/config/... — green (2026-09-09T17:38:40Z, commit 1af724aeee98389704856b32a4c84f531bce0dc0)
**entry_args:** 23

## Next action

**Do this immediately:** Group C is under way — subtask 12 (the alert contract) is committed at
1af724a. Subtask 13, the propagation sweep, is next and is the last subtask of the task.
`ai-docs/context-status.md`'s per-issue entry is deliberately NOT written by this group: it is
Step 9.5's, and the orchestrator's.

## Subtasks

- [x] 1. `internal/repotest` — the shared root helper, every existing declaration replaced
- [x] 2. `go get github.com/prometheus/client_golang@v1.24.1`, `go mod tidy`
- [x] 3. The `LAB_GAME_HEALTH_` configuration class
- [x] 4. Package skeleton: registry, labels, buckets, the D16 register
- [x] 5. Transport adapter (`tg.Observer`)
- [x] 6. Scheduler adapter (`scheduler.Observer`)
- [x] 7. Ingest adapter (`ingest.Observer`), `LagKnown` gate
- [x] 8. pgx pool collector
- [x] 9. `promhttp` endpoint server
- [x] 10. The canaries: `Prober`/`ProberFactory`, both legs, the ticker
- [x] 11. The structural guards (a)–(i)
- [x] 12. The alert contract
- [ ] 13. Propagation sweep  ← CURRENT (Group C)

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
- **Step 8, subtask 4**: `make comment-refs` (run explicitly against the new untracked files, since
  the no-args form only scans tracked paths) caught package-qualified module symbols
  (`tg.Observation`, `scheduler.Observation`, etc.), bare repo-relative paths (`internal/tg`,
  `registry.go`), and inline `AC4`/`D3` citations in the first drafts of `doc.go` and
  `registry_test.go` — all rewritten to prose naming no symbol/path/id. `golangci-lint run` separately
  flagged `unused` on every declaration only later subtasks will call (buckets, label mappers, the
  allow-list, the observation register) — resolved by writing this subtask's own unit tests
  (bucket-validity table test, per-mapper table tests, a register shape sanity test) so every
  declaration has a real caller now rather than only in a future subtask, and flagged `goconst` on
  repeated field-name/family-name string literals inside `observationRegister` — resolved by naming
  family constants (`familyBotAPICallDuration` etc.) and field-name constants (`fieldBatchSize`,
  `fieldDuration`, `fieldErr`) once and referencing them, which also makes a future typo between the
  register and a real registration a compile-time-shared value rather than an independent literal.
  `go mod tidy` pulled `client_golang`'s full transitive closure (`beorn7/perks`,
  `prometheus/client_model`, `prometheus/common`, `prometheus/procfs`, `google.golang.org/protobuf`,
  `kylelemons/godebug`, `munnerz/goautoneg`) now that a package actually imports it, matching the
  design's predicted closure exactly. Gates run and green: `go build ./...`, `go test ./...` (whole
  module), `go vet ./...`, `golangci-lint fmt -d` (no target file present in its diff),
  `golangci-lint run` (0 issues), `make comment-refs`. Committed at 9407fd4.
- **Step 8, subtask 5**: `TransportObserver` touches the rate-limited and retry counters only when
  their condition holds (`RateLimited` true / `Retries > 0`), so a method that never rate-limits or
  retries carries no dead series for those two families — consistent with D8's no-pre-initialisation
  rule generalised beyond the canary leg it was stated for. `make comment-refs` again caught
  package-qualified symbols (`tg.Observer`, `tg.Observation`) in the first draft's doc comments;
  rewritten to prose naming neither. Gates run and green: `go build ./...`, `go test ./...` (whole
  module), `go vet ./...`, `golangci-lint fmt -d` (no target file present in its diff),
  `golangci-lint run` (0 issues), `make comment-refs`. Committed at f5b9b4e.
- **Step 8, subtask 6**: `Observation.Duration` and `Observation.BatchSize` (the loop-level ones) are
  observed into their histograms regardless of whether the cycle's `Err` is non-nil — read
  `internal/scheduler/worker.go`'s `RunOnce` directly (`sed -n '95,127p'`) and confirmed both fields
  carry a real, meaningful value on every `observeLoop` call site, including the two error paths;
  only the loop-error counter is conditional on `Err != nil`. Added `internal/health/gather_test.go`
  as a shared test helper (not itself a subtask deliverable named in the design, but needed by this
  subtask's exempt-field byte-comparison test and reusable by every later adapter's own exempt-field
  test) — `gatherFrom`, `observedLabelValues`, `gatherText`, the last using
  `github.com/prometheus/common/expfmt` for a canonical text-exposition comparison
  `testutil.CollectAndCompare` cannot give across two different collectors. Gates run and green:
  `go build ./...`, `go test ./...` (whole module), `go vet ./...`, `golangci-lint fmt -d` (no target
  file present in its diff), `golangci-lint run` (0 issues), `make comment-refs`. Committed at
  ca44911. **Group A (subtasks 1–6) is complete.**
- **Step 8, subtask 10 (Group B, first)**: `internal/health/probe.go` and `canary.go` implement the
  `Prober`/`ProberFactory` seam, `TelegramProber` with its own `statusRecorder` (a private
  transport-observer, never `TransportObserver`), `classifyFailure`, `LegsOptions`/`NewLegs` (the
  own/cloud credential-endpoint pairing, asserted by a recording `ProberFactory` in
  `TestNewLegs_PairingNeverSwapped`), and `Canary`/`NewCanary`/`Start`/`Shutdown` (a
  `context.WithCancel` run loop, per-tick `context.WithTimeout(ctx, interval)` so a blocked prober is
  cancelled rather than overlapping the next tick, and a `sync.WaitGroup` driving both legs
  concurrently per tick). Canary and probe metric family/label-value consts are declared locally in
  `canary.go`, not in `registry.go`'s existing block, since subtask 10's own file list is
  `canary.go`/`probe.go` only. Runner tests use `testing/synctest` with fake `Prober`s (no network);
  prober tests use `internal/tgtest`'s fake server. `make comment-refs` caught nine ac-id/decision-
  anchor citations and four package-qualified module-symbol mentions (`tg.Observation`, `tg.Client`,
  `tg.Observer`, `config.Transport`, `tg.New`) across `canary.go`/`probe.go`/`probe_test.go` on first
  pass — rewritten to prose naming neither. Gates run and green: `go build ./...`,
  `go test ./internal/health/...`, `go test -race ./internal/health/...`, `go vet ./...`,
  `golangci-lint fmt -d` (no target file present in its diff), `golangci-lint run` (0 issues),
  `make comment-refs`. Committed at 64a015a.
- **Step 8, subtask 7**: `internal/health/ingest.go`'s `IngestObserver` reuses `registry.go`'s
  existing ingest family consts (subtask 4 already declared them) and `labels.go`'s
  `ingestKindLabel`/`ingestOutcomeLabel`. `ObserveUpdate` samples the lag histogram only when
  `obs.LagKnown`, verified by a test asserting a zero histogram count alongside a non-zero counter
  count from the same observation. `make comment-refs` raised no findings on first pass — the file
  cites no AC/decision id and no package-qualified module symbol. One test fix during authoring:
  `testutil.CollectAndCount` on a plain (non-vec) `prometheus.Counter` always returns 1 once the
  collector exists, regardless of its value, so the "no loop error increments the counter" assertion
  had to switch to `testutil.ToFloat64` — `CollectAndCount` only discriminates a `*Vec`'s label
  cardinality, not a scalar counter's value. Gates run and green: `go build ./...`,
  `go test ./internal/health/...` and the whole module, `go test -race ./internal/health/...`,
  `go vet ./...`, `golangci-lint fmt -d` (no target file present in its diff), `golangci-lint run`
  (0 issues), `make comment-refs`. Committed at a4ba9a2.
- **Step 8, subtask 8**: `internal/health/pool.go`'s `PoolCollector` is an ad-hoc
  `prometheus.Collector` (not a struct of pre-built vecs like the other adapters) since its metric
  set depends on the accessor's return value at scrape time; `Describe` sends no descriptor,
  marking it unchecked, which the client library's own doc comment names as the deliberate way to
  build such a collector. The fixture is a real `*pgxpool.Pool` built via `pgxpool.NewWithConfig`
  against a DSN pointing at a port a `net.ListenConfig` bound then immediately closed — confirmed
  (subtask 8's own design note) that such a pool still answers `Stat()` with no server, so the test
  binary stays database-free. The "not cached" scenario needed two pools built with different
  `MaxConns` (a bare re-run of `realStat` twice would return two structurally-identical zero-value
  snapshots and the assertion would pass vacuously) so the two scrapes' text actually differs.
  `golangci-lint run`'s `noctx` flagged a bare `net.Listen` in the test fixture — fixed with
  `(*net.ListenConfig).Listen`. `make comment-refs` caught one `repo-path` match
  (`internal/testdb`, named only to say the fixture avoids it) — rewritten to prose naming no
  package path. Gates run and green: `go build ./...`, `go test ./internal/health/...` and the whole
  module, `go test -race ./internal/health/...`, `go vet ./...`, `golangci-lint fmt -d` (no target
  file present in its diff), `golangci-lint run` (0 issues), `make comment-refs`. Committed at
  6bb8320.
- **Step 8, subtask 9**: `Server.Start` binds via `(*net.ListenConfig).Listen` (the `noctx` linter
  rejects a bare `net.Listen`) synchronously, before spawning the serve goroutine, so a bind error
  is always a returned error rather than a background-goroutine surprise. `Shutdown` joins the
  graceful-stop error with the serve goroutine's own terminal error via `errors.Join`, treating
  `http.ErrServerClosed` as success. Tests run outside a `synctest` bubble (a real listener is not
  durably blockable inside one) and hit the bound `127.0.0.1:0` port over real HTTP. `golangci-lint
  run` flagged the same `noctx` issue in the test fixture's already-bound-address probe and an
  unused `nolint:gosec` half of a directive (`nolintlint`) — fixed by switching production code to
  `(*net.ListenConfig).Listen` and narrowing the test directive to `//nolint:noctx` alone. Gates run
  and green: `go build ./...`, `go test ./internal/health/...` and the whole module,
  `go test -race ./internal/health/...`, `go vet ./...`, `golangci-lint fmt -d` (no target file
  present in its diff), `golangci-lint run` (0 issues), `make comment-refs`. Committed at 63eb6b3.
- **Step 8, subtask 11 (Group B, last)**: implemented guards (a)–(i) in `internal/health/guards_test.go`.
  Deviations from the design's own mechanism, each recorded here rather than silently:
  (1) guard (b)'s "proved discriminating" requirement was met by factoring the check into a plain
  function (`registerOffenses`) and calling it with a mutated table/doc-text in a second test,
  rather than mutating real files — the design's own text allows this for (b) since it does no file
  walk. (2) guards (a)/(f)/(i)'s discriminating proofs use a fresh, unrelated `t.TempDir()` (not a
  copy of the real tree plus a scratch file) since the walks all take an explicit root/dir parameter
  — a fresh temp directory avoids any possible collision with a sibling guard just as the design's
  own copy-based protocol does, with less code. (3) guard (g)'s pre-change-tree proof resolves the
  six pre-move files' content via `git show f57403f~1:<path>` (a hardcoded revision — subtask 1's
  own commit, already merged history on this branch by construction) rather than "the subtask-1
  parent commit" resolved dynamically; re-verified directly that each of the six files still
  declares its old resolver at that revision and that `cmd/commentrefs`'s two git-based resolvers
  are unaffected by the move. (4) `internal/health/doc.go` needed a same-commit fix: its exempt-field
  prose named the batch-size, consecutive-failures and attempt fields only in kebab-case English
  ("batch-size field"), which contains no substring identical to the Go identifier guard (b) checks
  for — confirmed by `grep -n` finding zero literal occurrences before the fix — so guard (b) would
  have reported a false "not named in the package doc comment" for all three. Fixed by adding each
  literal identifier alongside the existing prose; this is subtask 4's file but the falsification is
  subtask 11's own guard, so the fix landed in this commit per the same reasoning design decision
  D18 states for a comment a later subtask's own change falsifies.
  `make comment-refs` caught the widest set yet in this task — repo-path matches on bare `.go`
  mentions in comment prose (any word ending in a gated extension is a repo-path finding regardless
  of context), on `cmd/`/`internal/`-prefixed phrases, and on the shared root-resolving package
  named directly in a comment, plus module-symbol matches and one decision-anchor citation — all
  rewritten to prose naming no path/symbol/id; the string literals inside `t.Errorf`/`os.WriteFile`
  calls are exempt since the gate scans only `//`/`/* */` comments, not runtime string literals.
  `golangci-lint run`'s `noctx` flagged a bare `os/exec.Command` in the guard's own git-show helper
  — fixed with `exec.CommandContext`. Sentinel canary tokens for guard (d) had to match telego's own
  token format (a digit run, a colon, then exactly 35 word/hyphen characters) rather than being
  freely-chosen strings, since the client validates format at construction; padded each sentinel to
  the exact length. Ran the whole `make verify` (fmt check, build, vet, lint, file-limits,
  `go test ./...` and `-race` both through the shared-server route, `go mod tidy` delta,
  `actionlint`, `shellcheck`, `make comment-refs`) plus the coverage ratchet, all green — Group B
  (subtasks 7–11) is complete. Committed at 4829153.
- **Step 8 subtask 10 (post-commit gap fix)**: closed a test-coverage gap the orchestrator found in
  subtask 10 before Group C started. `TestNewLegs_PairingNeverSwapped` only asserts the
  credential-endpoint pairing at the `NewLegs` constructor boundary (a recording `ProberFactory`
  checking the options struct it received); nothing in the suite reached the wire to confirm
  `NewTelegramProber` actually transmits the token it was handed rather than a swapped or
  hard-coded one, which left AC15/AC16 ("issues `getMe` with the production bot token") unverified
  at the strength the design calls for. Added
  `TestNewTelegramProber_TransmitsItsOwnToken` in `internal/health/probe_test.go`: two
  `TelegramProber`s, each built with its own format-valid-but-distinguishable fake token against
  its own `tgtest.Server`, each asserting its own recorded `r.URL.Path` (telego's own path shape is
  `/bot<token>/<method>`, read directly from `github.com/mymmrac/telego`'s `bot.go`) carries exactly
  its own token, plus a same-path check that would catch a same-value hard-coding masking as two
  correct answers. Recording uses a mutex-guarded `pathRecorder`, not a bare variable, following the
  existing `atomic` counter in `TestTelegramProber_ServerError` -- a plain field written on the fake
  server's handler goroutine and read from the test goroutine after `Probe` returns is not
  established to be race-free by the HTTP round trip alone.
  Proved discriminating (this design's own guard-section requirement): temporarily replaced
  `NewTelegramProber`'s `tg.Options.Token` argument with a fixed, format-valid, wrong token
  (`"9:ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-"`) in `internal/health/probe.go`, re-ran the new test --
  it FAILED, reporting both legs' recorded paths carrying the wrong constant token and the
  same-path check firing -- then reverted `probe.go` via a `cp`-backup taken before the edit and
  confirmed (`diff`) the reverted file is byte-identical to the pre-corruption version, and that the
  test passes again.
  Base-URL observability finding: `tgtest.Server.DialContext`'s signature is
  `func(ctx context.Context, _, _ string) (net.Conn, error)` -- it discards the network/address
  arguments entirely and always hands back the same in-process pipe connection, so a prober's
  `BaseURL` is NOT observable through `srv.Client()`; any `BaseURL` value routes to the same fake
  server. Only the request path (which embeds the token) is observable at the wire through this
  fixture, so the new test asserts the token half of the pairing at the wire and leaves the
  endpoint half resting on the existing `NewLegs`-level constructor check plus this file's own
  `tgtest.BaseURL`-only usage; this is reported here rather than asserted as something it is not.
  Gates run and green: `go build ./...`, `go test ./internal/health/...`,
  `go test -race ./internal/health/...`, `go vet ./...`, `golangci-lint fmt -d` (clean),
  `golangci-lint run` (0 issues), `make comment-refs`, `go test ./...` (whole module, all packages
  ok). Committed at e957baf.
- **Step 8 subtask 12 (Group C, first)**: `ai-docs/alert-contract.md` written in English under
  `ai-docs/`, carrying the whole metric catalogue (every family, its type, its labels and each
  label's value set) plus one section per alert with the metrics it reads, the expression shape,
  the condition shape and the severity class. Non-trivial choices, each recorded rather than left
  implicit. (1) **The consecutive-failure alert is not a literal in-a-row detector, and the page
  says why**: a counter pair records how many probes succeeded and how many failed in a window and
  does not record their order, so the expressible form is "N failures and no success in a window
  covering N intervals"; it is written with `unless` rather than `and … == 0` because a leg that has
  failed since process start has no `success` series at all and a zero-comparison against an absent
  series yields nothing. (2) **A silence rule is wired beside the update-lag alert**, because a
  stalled poller feeds the lag histogram no samples at all, so `histogram_quantile` over a `rate`
  window goes EMPTY rather than high — the level alert cannot fire on the worst case it exists to
  catch. The spec's "at minimum" wording is what admits it; it is presented as the lag alert's
  companion condition, not as a freestanding new alert. (3) **Severity classes assigned**: own-leg
  canary and update lag page, cloud-leg canary tickets, the absence test is a recording/inhibition
  rule and not a page, since a disabled leg is a supported operating mode. (4) **The placeholder
  trap is carried in the form the code has, not the form D13 predicted**, and this is the one place
  the run contradicts its own design. Measured, not reasoned: a throwaway program under `tmp/`
  (deleted immediately; `git status` re-checked clean) called `health.NewLegs` with the shipped
  `.env.example` value and printed
  `token="changeme" legs!=nil=false cloudNil=true err=health: build cloud-reference canary leg:
  health: build canary client: tg: options: Token: telego: invalid token format`, against
  `token="123456789:AAAA…"` → both legs built, no error, and `token=""` → own leg built, cloud nil,
  no error. So the shipped placeholder does NOT make the cloud leg "fail every probe from the first
  tick" as D13/D14 and `.env.example`'s own comment both state: telego's token regexp
  (`^\d+:[\w-]{35}$`, read directly from its `bot.go`) rejects it, `internal/tg`'s constructor wraps
  that as an option error naming `Token`, and `NewLegs` fails wholesale — the OWN leg is not built
  either and no canary series exists at all. The "fails every probe" case is real but belongs to a
  well-formed-but-wrong credential. Both cases are named in the contract with the same single fix
  (set the key empty). (5) **`.env.example`'s comment for that key was corrected in the same
  commit**, because it asserted the second behaviour for the first case; it is a live surface
  asserting what an environment variable does, which is squarely inside subtask 13's declared sweep
  class, and leaving it to contradict the contract for a commit was the worse option. No `.go` file
  was touched. Re-checked rather than redone: subtask 3's Go-comment half of the propagation
  obligation did land (`git diff 3616d52..HEAD -- internal/config/config.go internal/config/env.go`
  shows `Config`'s doc comment and the environment-variable block both rewritten to name the four
  optional classes). Gates run and green: `make comment-refs` (whole tracked gated set),
  the CI relative-markdown-link check run locally as its own Python snippet, and
  `go test ./internal/config/...` (the suite that reads `.env.example`). Committed at 1af724a.

## Key discoveries (don't re-investigate)

- Six file-location-ascent root resolvers exist, not four: the four `repoRootPath` copies plus `repoRoot` in `internal/commentref` and `internal/testdb`. Found by behaviour (`runtime.Caller`), not by identifier.
- `cmd/commentrefs` holds two git-based resolvers (`repoRoot`, `repoRootForTest`) that are OUT of the class by mechanism — they ask git, never their own file location.
- `client_golang@v1.24.1` is newest published. `newHistogram` panics on non-increasing buckets and on an `le` label; an EMPTY bucket slice silently becomes `DefBuckets` instead.
- `internal/ingest` and `internal/tg` each ban `client_golang`, but both guards walk only their own package — `internal/health` importing it trips neither.
- A pool built against an unreachable DSN still answers `Stat()`, so this package's tests need no Postgres.
- `make comment-refs` (no args) scans only the tracked set — an untracked new file needs an explicit
  path argument (`go run ./cmd/commentrefs <path>...`) to be checked before it is staged.
- `go mod tidy` prunes an as-yet-unimported module entirely (require line and go.sum entries both) —
  confirmed by direct measurement in subtask 2. The transitive closure only appears once a package
  actually imports the module (subtask 4), at which point `go mod tidy` matches the design's
  predicted closure exactly.

## Files touched

- `internal/repotest/repotest.go`, `internal/repotest/repotest_test.go` (new)
- `cmd/bot/main_test.go`, `internal/ingest/guards_test.go`, `internal/tg/guards_test.go`,
  `internal/commentref/testhelpers_test.go`, `internal/commentref/envexample_extract_test.go`,
  `internal/commentref/go_extract_test.go`, `internal/commentref/shell_extract_test.go`,
  `internal/commentref/sql_extract_test.go`, `internal/commentref/yaml_extract_test.go`,
  `internal/testdb/server_test.go` (repoRootPath/repoRoot call sites converted)
- `internal/config/repo_root_test.go` (deleted)
- `internal/config/balance_file_test.go`, `internal/config/disjoint_test.go` (repoRootPath call sites
  converted)
- `go.mod`, `go.sum` (client_golang v1.24.1 + transitive closure)
- `internal/config/health.go`, `internal/config/health_test.go` (new)
- `internal/config/config.go`, `internal/config/env.go` (Health wiring, falsified doc comments fixed)
- `.env.example` (LAB_GAME_HEALTH_* keys)
- `internal/health/doc.go`, `internal/health/registry.go`, `internal/health/registry_test.go`,
  `internal/health/labels.go`, `internal/health/labels_test.go` (new)
- `internal/health/transport.go`, `internal/health/transport_test.go` (new)
- `internal/health/scheduler.go`, `internal/health/scheduler_test.go`, `internal/health/gather_test.go` (new)
- `internal/health/canary.go`, `internal/health/probe.go`, `internal/health/canary_test.go`,
  `internal/health/probe_test.go` (new)
- `internal/health/ingest.go`, `internal/health/ingest_test.go` (new)
- `internal/health/pool.go`, `internal/health/pool_test.go` (new)
- `internal/health/server.go`, `internal/health/server_test.go` (new)
- `internal/health/guards_test.go` (new)
- `internal/health/doc.go` (exempt fields also named by their literal Go identifier, for guard (b))
- `ai-docs/alert-contract.md` (new — subtask 12)
- `.env.example` (subtask 12: the cloud-token comment's placeholder-trap claim corrected to the
  measured behaviour)
- `ai-docs/learnings.md` (subtask 12: one validation entry)

## AC Status

| AC | Status |
|----|--------|
| AC1–AC34 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

