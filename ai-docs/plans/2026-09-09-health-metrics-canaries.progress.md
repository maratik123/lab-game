# Progress: health metrics and canaries — ACTIVE
_Updated: 2026-09-09 17:42_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-09-health-metrics-canaries
**base_commit:** 3616d527e04ea6abf7a1f72de041140b297e96a9
**Last build:** PASS
**Issue:** #23
**Spec:** ai-docs/plans/2026-09-09-health-metrics-canaries.spec.md
**current_step:** Step 8 — subtask 13 of 13 complete (Group C DONE; all subtasks complete)
**last_passed_gate:** golangci-lint run | 2026-09-09T18:52:32Z | c9b521add4a931bb99936dedf40f3c4806427fc6
**entry_args:** 23

## Next action

**Do this immediately:** Group C is complete (subtask 12 at 1af724a, subtask 13 at 311ca88), and
with it every subtask of Step 8. The orchestrator's next steps are Step 9 / 9.5 / 10 —
`ai-docs/context-status.md`'s per-issue entry was deliberately NOT written by this group, because it
is Step 9.5's. One finding for Step 9's attention is recorded in the subtask-12 Decisions-log entry:
the run's own measurement contradicts design decisions D13/D14 about what the shipped
`.env.example` placeholder does, and both the alert contract and `.env.example` now state the
measured behaviour rather than the predicted one. The design document itself is unedited.

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
- [x] 13. Propagation sweep

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
- **Step 8 subtask 13 (Group C, last)**: the propagation sweep, re-run against the finished diff
  rather than against the design's illustrative file list. Surfaces touched, four:
  `ai-docs/agent-docs-index.md` (the alert contract indexed, beside the key-decisions row);
  `ai-docs/context.md` (the layout paragraph gains `internal/health` and `internal/repotest`; the
  three clauses reading "the observation seam #23 reads" now name the package that reads it — the
  design predicted exactly this falsification; the status bullet records the health surface, its key
  class and the contract, its heading date moves, and the `cmd/bot` sentence is corrected, since
  nothing constructs a metrics server or a canary either and the reason for those two is the
  composition root rather than the missing queue); `ai-docs/key-decisions.md` (KD-27's member
  enumeration gains its fourth scope, plus an amendment clause naming the cloud canary token and the
  cloud canary base URL as exceptions to its required-with-no-default sentence, each carrying the
  reason, with the clause left standing for every other secret and base URL and the generalisation
  an earlier draft proposed recorded as refused); and `ai-docs/plans/INDEX.md` (the row's status off
  "spec only"). Deliberately untouched, each for a stated reason: `ai-docs/context-status.md`, whose
  per-issue entry belongs to Step 9.5 and whose three existing entries are history; `ai-docs/
  learnings.md`, `ai-docs/harness-gaps.md` and `ai-docs/plans/done/**` as history surfaces; and
  `docs/DESIGN.md` § 13, which prescribes this surface rather than describing it, so nothing there
  is falsified — its `/metrics` line is now satisfied, not contradicted. Two near-misses inspected
  and left alone rather than silently skipped: KD-28's "an idempotency-hit count that nothing
  exposes until #23" is a prediction now satisfied rather than a false claim, and
  `ai-docs/domain-invariants.md`'s "lag by task type belongs on the health dashboard" is still true
  and now exportable. The sweep command was
  `rg -l -i "LAB_GAME_HEALTH_|internal/health|internal/repotest|alert-contract|observation seam|observation point|/metrics" --glob '*.md'`
  over the tree with the plans, both learning logs and `tmp/` excluded; its only remaining matches
  are the history surfaces above, the unrelated `ai-docs/metrics/` task-telemetry file, and
  `docs/DESIGN.md`'s own prescription. Gates run and green: `make comment-refs`, the CI
  relative-markdown-link check run locally, the citation-namespace guard
  (`check-citations.sh`, PASS), and `realpath -e` on both new relative links. Committed at 311ca88.

- **Step 9**: all 34 criteria verified by the orchestrator's own commands; interface satisfaction proven by compiling an assertion program rather than by matching method names. Coverage ratchet 89.74%, unchanged.

- **Step 9.5**: `context.md` needed no orchestrator edit — the Group C sweep had already carried the layout, the status bullet and the contract link. Only the `context-status.md` entry remained, written with the `#TBD-at-Step-12` locator that Step 12 substitutes.

- **Step 10**: self-review round 1 REJECT — 5 major, plus minors. My Step-9 AC16 PASS was false: I verified the token-transmission test and never exercised the success condition, so the check could not detect the defect it was recorded against.

- **Step 11**: ten of eleven rows fixed at c9b521a. PROC-5 objected, not fixed: `self-review.md` instruction 2 forbids reviewing the progress file's field content ("do NOT review their content for correctness; their lifecycle is the calling skill's responsibility"), and line 166 defines a Design Amendment trigger as a finding whose resolution REQUIRES editing the design — this one audits the order of an amendment already made. Surfaced to the owner verbatim; awaiting their ruling.

- **Step 11**: the D13/D14 correction at ed3e778 carried no amendment-record header while the eight before it did; the header was added through design-writer, since design writes are subagent-owned.

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
- `ai-docs/agent-docs-index.md`, `ai-docs/context.md`, `ai-docs/key-decisions.md`,
  `ai-docs/plans/INDEX.md` (subtask 13: the propagation sweep)

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS — `NewRegistry` returns a dedicated registry; every family takes a supplied `prometheus.Registerer` |
| AC2 | PASS — `rg 'promauto|DefaultRegisterer|MustRegister'` hits only `guards_test.go`, which names them to forbid them |
| AC3 | PASS — `promhttp.HandlerFor` on its own `http.Server`, addr from config, exported `Start`/`Shutdown` |
| AC4 | PASS — compiled `var _ tg.Observer = (*health.TransportObserver)(nil)` in a throwaway program — satisfaction proven, not eyeballed |
| AC5 | PASS — same method: `_ scheduler.Observer = (*health.SchedulerObserver)(nil)` compiles |
| AC6 | PASS — same method: `_ ingest.Observer = (*health.IngestObserver)(nil)` compiles |
| AC7 | PASS — `LagKnown` gate in `ingest.go`; package tests green |
| AC8 | PASS — scheduler lag histogram carries the task-type label; guard (c) asserts the value set |
| AC9 | PASS — transport families: latency by method, codes by method+status, 429 and retry counters |
| AC10 | PASS — `TestGuard_LabelNamesAndClosedSetValues` green — observed set equals the six `ingest.Outcome` members |
| AC11 | PASS — same guard — observed set equals the five `scheduler.FailureKind` members |
| AC12 | PASS — `PoolCollector.Collect` calls the `func() *pgxpool.Stat` accessor at collection time |
| AC13 | PASS — `collectors.NewGoCollector()` and `NewProcessCollector` registered on the same registry |
| AC14 | PASS — two legs, `leg` label with exactly `own`/`cloud`, outcome counter and latency histogram each |
| AC15 | PASS — `TestNewTelegramProber_TransmitsItsOwnToken` green — asserts the token on the transmitted request path |
| AC16 | FAIL — recorded PASS at Step 9 in error. `Probe` returns success on `err == nil` and never reads the recorded status; `tg.Caller` succeeds on `resp.Ok` with no status gate, so a 500 carrying `ok:true` counts as a healthy probe. Self-review round 1, R1-1. |
| AC17 | PASS — `TestNewLegs_EmptyCloudTokenDisablesCloudLeg` and `TestCanary_DisabledCloudLegExportsNoSeries` green |
| AC18 | PASS — `TestTelegramProber_NeverWritesToATransportRegistry` green; one attempt per tick per leg |
| AC19 | PASS — failures classified by status code or transport-error class; guard (c) rejects raw error text |
| AC20 | PASS — `defaultHealth()` sets `CanaryInterval: time.Minute` — one value driving both legs |
| AC21 | PASS — guard (i): `canary.go`/`probe.go` import no pgx, pgxpool, store or scheduler symbol |
| AC22 | PASS — `defaultHealthMetricsAddr = "127.0.0.1" + ":" + "9095"` — loopback only |
| AC23 | PASS — guard (c) plus `TestGuard_ScrapeCarriesNoSentinelSecret`, both green |
| AC24 | PASS — `CanaryCloudToken Secret` in the config struct |
| AC25 | PASS — four `LAB_GAME_HEALTH_` keys in `.env.example`; `healthEnvKeys()` in the exported enumeration; config suite green |
| AC26 | PASS — `ai-docs/alert-contract.md` exists, English, carries both canary alerts, the cross-leg expression, the off-leg meaning and the absence test |
| AC27 | PASS — listed in `ai-docs/agent-docs-index.md` |
| AC28 | PASS — `go.mod` requires `client_golang v1.24.1`; `go mod tidy` leaves no delta |
| AC29 | PASS — the only `cmd/` path in the diff is `cmd/bot/main_test.go` — a test file; no production file, no composition moved |
| AC30 | PASS — `revive` `exported` + `package-comments` enabled and `golangci-lint run` green |
| AC31 | PASS — `make comment-refs` green over the whole tracked gated set |
| AC32 | PASS — no `panic(`, `log.Fatal` or `os.Exit` in `internal/health` or `internal/repotest`; panic index still empty |
| AC33 | PASS — build, vet, fmt, lint, tidy, comment-refs, `make test` and `make test-race` all green (0 FAIL, 0 DATA RACE); coverage ratchet 89.74% holds against 89.74% |
| AC34 | PASS — propagation sweep touched `.env.example`, `agent-docs-index.md`, `context.md`, `key-decisions.md`, `INDEX.md`; Go-comment half landed in subtask 3 |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| HEALTH-1 | R1 | major | fixed@c9b521a | add a `tgtest` handler answering `500` with `{"ok":true,...}`, call `TelegramProber.Probe`; the probe must return a non-nil error |
| HEALTH-2 | R1 | major | fixed@c9b521a | `grep -nE 'OwnToken\|CloudToken\|Token ' internal/health/canary.go internal/health/probe.go` — each token field must read `config.Secret` |
| HEALTH-3 | R1 | major | fixed@c9b521a | `Start`, `Shutdown(ctx)`, then `Shutdown(ctx2)` with a 500ms deadline on `*health.Server`; the second call must return, not block |
| HEALTH-4 | R1 | major | fixed@c9b521a | `git diff 3616d52..HEAD -- '*.go' \| grep -nE '^\+.*//.*([Ss]ee \|pinned by) '` must print nothing |
| PROC-5 | R1 | major | accepted@1 — objected with the owner's approval. Out of the reviewer's own charter in two of three parts: `self-review.md` instruction 2 says the progress file's fields are verified PRESENT but "do NOT review their content for correctness; their lifecycle is the calling skill's responsibility", which covers the missing decisions-log entry; and line 166 defines a Design Amendment trigger as a finding whose resolution REQUIRES editing the design, while this audits the ordering of an amendment already made. The re-run half rests on an incomplete premise: the owner granted the exemption explicitly when authorising the correction. The third part was right and is fixed — the amendment header was added to the design. | `git log --format='%h %s' 3616d52..HEAD -- ai-docs/plans/*.design.md` and `grep -n '^\*\*Amended:' ai-docs/plans/2026-09-09-health-metrics-canaries.design.md` — the amendment must carry its own round header and a recorded design-review verdict |
| HEALTH-6 | R1 | minor | fixed@c9b521a | `grep -n 'sentinelChatID\|sentinelUpdateID\|sentinelTaskID\|sentinelOperationID' internal/health/guards_test.go` — each must appear in a fixture write, not only in the final assertion loop |
| HEALTH-7 | R1 | minor | fixed@c9b521a | write any `.go` file under `tmp/`, then `go test ./internal/health/ -run 'TestGuard_SingleRepoRootResolver_RealTree\|TestGuard_NoRepoRootPathIdentifierAnywhere' -count=1` must stay green |
| HEALTH-8 | R1 | minor | fixed@c9b521a | `grep -n 'unreachable in an observed series' internal/health/labels.go` — the scheduler-outcome mapper must not carry the claim D16 declines |
| HEALTH-9 | R1 | minor | fixed@c9b521a | `go doc github.com/maratik123/lab-game/internal/health TransportObserver` — the doc must state its concurrency contract |
| HEALTH-10 | R1 | nit | fixed@c9b521a | `grep -n 'defaultHealthMetricsAddr =' internal/config/health.go` — one plain literal, no concatenation |
| PROG-11 | R1 | nit | fixed@c9b521a | `grep -c '^## Files touched' ai-docs/plans/2026-09-09-health-metrics-canaries.progress.md` must print `1` |
| REC-A | R1 | — | accepted@1 — every return path in the transport caller calls `observe`, so `statusRecorder.take()` can never hand back a stale observation; the never-reset field is safe as written | `grep -c 'c.client.observe(' internal/tg/caller.go` |
| REC-B | R1 | — | accepted@1 — `.env.example` ships `changeme` for the cloud token; the value is a placeholder in a file whose every value must be non-empty, and the loader's present-but-empty clause is the documented way off. Not a tracked secret. | `grep -n 'CANARY_CLOUD_TOKEN' .env.example` |
| REC-C | R1 | — | accepted@1 — `promAutoOffenses` matches only the `prometheus` identifier, so an aliased import escapes guard (a). No such alias exists in the module and the `promauto` import path check is alias-proof. | `rg -n 'client_golang/prometheus"' --type go` |
| REC-D | R1 | — | accepted@1 — `PoolCollector.Describe` sends no descriptor (unchecked collector). Deliberate per D9 and documented at the method. | `grep -n 'func (c \*PoolCollector) Describe' -A 1 internal/health/pool.go` |

## Self-Review (Round 1)

**Verdict:** REJECT

**What was checked.** The whole diff `3616d52..HEAD` (53 files, +4986/-149). Read in full:
every non-test file of `internal/health` (`doc.go`, `registry.go`, `labels.go`, `transport.go`,
`scheduler.go`, `ingest.go`, `pool.go`, `server.go`, `canary.go`, `probe.go`),
`internal/config/health.go`, `internal/repotest/repotest.go`, the whole of
`internal/health/guards_test.go`, `.env.example`'s new block, `ai-docs/alert-contract.md`, and the
`context.md` / `key-decisions.md` / `agent-docs-index.md` / `INDEX.md` propagation edits. Read as
upstream context: `internal/tg/caller.go`, `internal/tg/observe.go`,
`internal/scheduler/observe.go`, `internal/ingest/observe.go`, `internal/config/config.go`.
Design decisions re-read against the code: D1, D3, D5, D6, D7, D8, D9, D10, D11, D12, D13, D14,
D16, D17.

**Gates re-run against the shipped tree, not quoted from the log:** `go build ./...` GREEN ·
`go vet ./...` GREEN · `golangci-lint run` GREEN (0 issues) · `make comment-refs` GREEN ·
`make file-limits` GREEN · `go mod tidy` + `git diff --exit-code go.mod go.sum` CLEAN ·
`make test` GREEN · `make test-race` GREEN (0 DATA RACE) · `make cover-ratchet` GREEN
(89.74% holds against 89.74%).

**AC verification commands re-run against the shipped artefact.** AC2 PASS
(`rg 'promauto|DefaultRegisterer|MustRegister' --type go` hits only `guards_test.go`, which names
them to forbid them). AC28 PASS (`go.mod` line 11 requires `client_golang v1.24.1`; tidy leaves no
delta). AC29 PASS (the only `cmd/` path in the diff is `cmd/bot/main_test.go`). AC31 PASS
(`make comment-refs` over the whole tracked gated set). AC32 PASS
(`grep -rnE '(^|[^[:alnum:]_.])(panic\(|log\.(Fatal|Panic)[a-z]*\(|os\.Exit\()' internal/health
internal/repotest internal/config/health.go` hits only guard fixtures inside `guards_test.go`;
`ai-docs/panic-index.md` still carries its `| — | — | — |` body row). AC33 PASS (the gate list
above). AC34 PASS (the metric catalogue in `ai-docs/alert-contract.md` was diffed name-for-name
against the 30 `namePrefix + …` constants in the source: zero names in the code are missing from
the document, and the only document-only token is the derived `_bucket` suffix). **AC16 FAILS —
finding 1.**

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/health/probe.go:112-120 | major | **AC16 is not implemented, and the doc comment asserts the missing half as a fact.** AC16: the leg "counts a probe successful only on an HTTP 200 response carrying `ok:true`"; D11: "a probe reports success **only** when `GetMe` returned no error *and* the recorded status code is 200 — `ok:true` from the caller, `200` from the recorder". `Probe` never reads `obs.StatusCode` for the success decision — it returns `err` alone, and `internal/tg/caller.go:88` returns success on `attemptErr == nil && resp != nil && resp.Ok` with **no** status gate, while `doAttempt` decodes the envelope for any status. Falsified by running it: a `tgtest` handler answering `500` with `{"ok":true,…}` yields `Probe -> status=500 err=<nil>`, i.e. the canary counts a **success** with `outcome="success"` and a latency sample, on a leg that is returning 500. Fix: gate success on `obs.StatusCode == http.StatusOK` as well, and add the falsifying case as a test — no test in the suite covers it (`TestTelegramProber_Success` asserts 200 on the happy path; `_OkFalse` and `_ServerError` both vary the other axis). | ✅ Fixed |
| 2 | internal/health/canary.go:41,47 · internal/health/probe.go:41 | major | **D11's stated secret protection is absent — a `%v` of the options struct prints the production bot token in full.** The design says, at design line 613: "Both token fields are `config.Secret`, so a `%v` of the options struct redacts them." The shipped `LegsOptions.OwnToken`, `LegsOptions.CloudToken` and `TelegramProberOptions.Token` are all plain `string`; `grep -rn 'config.Secret' internal/health/*.go` returns nothing but one test name. `config.Secret` exists precisely for this (`String`/`GoString` render `[redacted]`), `internal/health` already imports `internal/config` for `config.Transport`, and the composition root this package hands `LegsOptions` to carries the **production** bot token in `OwnToken`. Deviating from a design decision without triggering the amendment recipe is forbidden by `.claude/skills/task/SKILL.md:131`. Fix: declare all three fields `config.Secret` and `Reveal()` at the `tg.New` call site. | ✅ Fixed |
| 3 | internal/health/server.go:92-107 | major | **`Server.Shutdown` deadlocks on a second call and ignores its own context while doing it.** `serveErr` is a one-shot buffered channel that is never closed and `started` is never cleared, so the second `Shutdown` blocks forever on the bare `err := <-serveErr` receive — a plain receive, so the `ctx` it was handed is not consulted. Demonstrated: `Start` → `Shutdown(ctx)` → `Shutdown(ctx2)` with a 500ms deadline on `ctx2` reports `DEADLOCK: second Shutdown never returned (blocked on the drained serve-error channel), despite its own context expiring after 500ms` after a 3s test guard. The asymmetry is the tell: `Start` guards its own second call and returns an error, and the sibling `Canary.Shutdown` is idempotent because it selects on a **closed** `done` channel. A composition root that pairs a `defer srv.Shutdown(ctx)` with an explicit shutdown hangs the process. Fix: latch the terminal serve error under `s.mu` on first read (or `select` on `ctx.Done()`), and return the latched value thereafter. | ✅ Fixed |
| 4 | internal/config/health.go:53,70,80 · internal/health/canary.go:51 | major | **Four bare-name pointers — the review-judged half of DOC-4** (`ai-docs/doc-convention.md` § DOC-4 → *What the gate decides, and what review decides*, second bullet: "A bare unqualified name used as a pointer — 'see such-and-such' … It is banned, and it is refused in review rather than by the gate"). The sites: `health.go:53` "See defaultHealthMetricsAddr for the compiled-in default **and the reasoning behind it**"; `health.go:70` "(see defaultHealthCanaryCloudBaseURL)"; `health.go:80` "pinned by TestDefaultHealth_CloudBaseURLParses" (a test is not a contract symbol, so no exemption reaches it); `canary.go:51` "(see NewTelegramProber)" — the weakest of the four, since that constructor is arguably the field's guarantor, but it is still written in the banned pointer form. DOC-4's own remedy applies: "Where a sentence exists only to carry the reference, the sentence goes with it" — state the default and the reason in place rather than sending the reader. `make comment-refs` cannot see any of these; that is what makes them review's. | ✅ Fixed |
| 5 | ai-docs/plans/2026-09-09-health-metrics-canaries.design.md (commit ed3e778) | major | **A design amendment landed after the deviation shipped, with no recorded design-review re-run and no amendment header.** Subtask 12 measured that D13/D14 were factually wrong about the shipped placeholder and committed the corrected behaviour into `.env.example` and `ai-docs/alert-contract.md` at `1af724a` — the progress file's own subtask-12 entry says so and adds "this is the one place the run contradicts its own design". The design was corrected only afterwards, at `ed3e778`. `.claude/skills/task/SKILL.md:129` requires the reverse order (stop the step, surface, spawn `design-writer`, **re-run Step 7 design-review**, resume on GO) and `:131` names the shipped order FORBIDDEN; `:136` makes the re-review unconditional, exempt only by the owner's explicit word per instance. Two checkable facts back this: the `## Decisions log` carries an entry for every other commit in the run and **none** for `ed3e778`, and `ed3e778` adds no `**Amended:** … round 9` line while all eight prior amendments carry one. **Design Amendment trigger** — surface to the owner with this wording quoted, per SKILL.md:216's two-exits rule; if the re-review did happen, record it in the Decisions log and add the round header via the `design-writer` Subagent (the orchestrator must not edit `*.design.md` directly). | ⚠️ Objected: out of the reviewer charter in two of three parts; the amendment header was the right part and is fixed |
| 6 | internal/health/guards_test.go:420-494 | minor | **Four of guard (d)'s seven sentinels are structurally unreachable, so those four assertions cannot go red** (AGENTS.md § *Patterns* 2). `sentinelChatID`, `sentinelUpdateID`, `sentinelTaskID` and `sentinelOperationID` are declared and then asserted absent, but nothing in the fixture ever writes them anywhere: the test drives only two canary legs and a pool collector. D12 describes this guard as asserting the body carries none of "the sentinel secrets **the fixture was built with**" — it was built with three of them. Confirmed structural: `tg.Observation` has no chat field, `ingest.Observation` no update id, `scheduler.Observation` no task id, so no observation can carry one. Either drive a value that *can* reach a label (`scheduler.Observation.Type` and `ingest.Kind` are the two caller-supplied strings that pass through verbatim) or delete the four constants and say in the comment that those categories are closed by the observation structs' own field sets, not by this scrape. | ✅ Fixed |
| 7 | internal/health/guards_test.go:654,680,789 | minor | **Guards (g)/(h) walk `tmp/`, the workspace's own mandated scratch directory, and red on a legitimate probe.** `walkGoFilesUnder` prunes only `.git`. AGENTS.md § *Build & Test* designates `tmp/` as "the one ignored scratch directory" for "a throwaway probe", and this very run used it (subtask 12's Decisions-log entry). Demonstrated: a `tmp/probe/main.go` declaring a `runtime.Caller`-based `repoRootPath` reds both guards — `file-location-ascent resolver outside internal/repotest: tmp/probe/main.go` and `repoRootPath still declared at: [tmp/probe/main.go]`. The module's own `file-limits` recipe already prunes `./tmp` for exactly this reason, and sibling guard (a) already filters to `cmd`/`internal`. Fix: prune `tmp` in `walkGoFilesUnder`, or apply guard (a)'s top-level filter to these two walks. | ✅ Fixed |
| 8 | internal/health/labels.go:42-46 · ai-docs/alert-contract.md:66-69 | minor | **Both artefacts make the scheduler-outcome unreachability claim D16 deliberately declines to make.** `schedulerOutcomeLabel`'s doc comment: "The default branch is unreachable in an observed series — the worker refuses an out-of-range outcome before ever building an observation"; the alert contract § 2: "For the scheduler and ingest outcome labels it is a declared-only branch that the shipped code has no path to produce". D16 states the opposite framing outright: "The claim is deliberately **not** made for `scheduler.Outcome`, which a consumer-declared `Handler` supplies and the worker passes through unnormalised (D6): there `unknown` is reachable in principle". The claim rests on another package's internal control flow, which is exactly why the design refused it — and a durable comment asserting it is the rot shape DOC-4 exists to prevent. Restrict the strong claim to `ingest.Outcome` and `scheduler.FailureKind` (the two the ACs bind and D16 does clear), and give the scheduler-outcome mapper the conservative wording. | ✅ Fixed |
| 9 | internal/health/transport.go:15 · internal/health/pool.go:53 | minor | **No concurrency contract on types whose upstream seam guarantees concurrent calls.** `tg.Observer`'s own declaration states "ObserveCall runs on the calling goroutine", so `TransportObserver` is called from every goroutine that makes a Bot API call; `PoolCollector.Collect` is called by the registry on scrape goroutines. Neither doc comment says whether the type is safe for concurrent use, and `ai-docs/doc-convention.md`'s concurrency rule makes silence mean "assume it is not safe" — which is wrong here (the client library's vecs are goroutine-safe) and will mislead the composition root into serialising or into doubting the seam. One sentence per type. | ✅ Fixed |
| 10 | internal/config/health.go:27 | nit | `defaultHealthMetricsAddr = "127.0.0.1" + ":" + "9095"` splits a value that has no reason to be split. The subtask-3 Decisions-log entry attributes the split to `make comment-refs`, but that gate reads `//` and `/* */` comments only, never a Go string literal — measured: substituting the plain `"127.0.0.1:9095"` literal and running `go run ./cmd/commentrefs internal/config/health.go` reports **CREFS-GREEN**. The split obscures the address in review and in `go doc` for nothing. | ✅ Fixed |
| 11 | ai-docs/plans/2026-09-09-health-metrics-canaries.progress.md:334,411 | nit | The progress file carries two `## Files touched` headings; the second (line 411) is empty. Delete the stray. | ✅ Fixed |

