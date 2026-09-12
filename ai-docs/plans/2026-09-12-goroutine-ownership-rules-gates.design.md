# Design: Goroutine-leak prevention — ownership rules and the gates that hold them

**Issue:** #80
**Date:** 2026-09-12

## Approach

### What is there now

The lint gate enables a named set beyond the standard one, and none of `containedctx`, `fatcontext`
or `forbidigo` is in it; `gocritic` runs with no `enabled-checks` block, so its experimental checkers
stay off, and `govet` runs with no settings block, so it runs its own default analyzer suite
[measured e66beb5:.golangci.yml · `cat .golangci.yml` → `linters.enable` lists `bodyclose`,
`contextcheck`, `durationcheck`, `errorlint`, `exhaustive`, `makezero`, `nilerr`, `noctx`,
`rowserrcheck`, `sqlclosecheck`, `unconvert`, `wastedassign`, `gosec`, `asciicheck`, `goconst`,
`gocritic`, `misspell`, `nolintlint`, `predeclared`, `revive`, `unparam`, `whitespace`;
`linters.settings` holds `exhaustive`, `nolintlint` and `revive` only].

The written half is a bullet: `ai-docs/code-style.md` § Concurrency states that every goroutine has
an owner that can stop it and a documented exit condition, and nothing checks it
[measured e66beb5:ai-docs/code-style.md · `cat ai-docs/code-style.md` → § Concurrency's bullets, of
which one is "Every goroutine has an owner that can stop it and a documented exit condition"].

Goroutine launches in module source are the statements in `(*app).drain` (`cmd/bot/serve.go`),
`(*Loop).getUpdatesStoppable` (`internal/ingest/loop.go`), `New` (`internal/tgtest/tgtest.go`),
`(*Canary).Start` and `(*Canary).tick` (`internal/health/canary.go`), `(*Server).Start` and
`(*Server).Shutdown` (`internal/health/server.go`), `(*Worker).executeOne`
(`internal/scheduler/execute.go`), and `jsonConstructor.MultipartRequest`
(`internal/tg/constructor.go`)
[measured e66beb5 · `rg -n '^\s*go\s+' --type go --glob '!*_test.go'` filtered of
`internal/health/testdata`, cross-read against `grep -n '^func ' <file>` for each enclosing
declaration]. No Go source under `internal/health/testdata` carries a `go` statement
[measured e66beb5 · `rg -n '^\s*go\s+' internal/health/testdata/` → no output].

Non-test source recovers a panic at the ingest handler boundary and nowhere else
[measured e66beb5:internal/ingest/attempt.go · `rg -n 'recover\(\)' --type go --glob '!*_test.go'` →
`internal/ingest/attempt.go` alone]. The scheduler's handler goroutine does not recover; the spec
puts that squarely out of scope, in #81
[measured e66beb5:ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md ·
`cat ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md` → § Out of scope, "The
scheduler's panic recovery and its reclamation of a handler that breached its deadline — #81"].

On the HTTP side, the Telegram caller falls back to `http.DefaultClient` when the client carries none,
and the composition root threads its own `assembleOptions.HTTPClient` — nil on the production path —
into both `tg.New` and `health.NewLegs`
[measured e66beb5:internal/tg/caller.go,cmd/bot/assemble.go · `rg -n 'http\.DefaultClient|HTTPClient'
--type go --glob '!*_test.go'` → `caller.go`'s `httpClient()` returning `http.DefaultClient` when
`c.client.httpClient` is nil; `assemble.go` passing `opts.HTTPClient` to `tg.New` and to
`health.NewLegs`]. The lifecycle prose already calls that value "the process HTTP client"
[measured e66beb5:ai-docs/process-lifecycle.md · `sed -n '40,72p' ai-docs/process-lifecycle.md` →
step 13's row, "both canary legs over the process HTTP client"].

### Chosen solution

The mechanisms, each matched to the acceptance criterion that names it.

**1. The lint gate carries every criterion that says "the lint gate" (AC3, AC4, AC6–AC9, AC14).**
Each shape maps onto a linter of the installed golangci-lint, so nothing needs a custom-built binary:
`containedctx` for a context stored in a struct, `fatcontext` for a context nested in a loop or a
function literal, `gocritic`'s `deferInLoop` for a `defer` inside a loop, `govet`'s `nilness` for a
provable nil dereference or impossible nil comparison, and `forbidigo` for the fresh-root-context and
unstoppable-timer identifiers
[measured e66beb5 · `golangci-lint help linters` → "containedctx: … detects struct contained
context.Context field", "fatcontext: Detects nested contexts in loops and function literals";
`golangci-lint version` → 2.13.1]. Each was pointed at a constructed violating package and seen to
report
[measured e66beb5 · a scratch module outside the tree run under a config enabling exactly these →
`containedctx` on the struct field, `fatcontext` "nested context in loop" and "nested context in
function literal", `forbidigo` on `context.Background`, `time.After` and `time.Tick`, `gocritic`
`deferInLoop`, `govet` `nilness` "tautological condition: nil == nil" and "nil dereference in load"].

**2. A repository-wide allow list holds the bare-`go` criterion (AC1, AC2).** `forbidigo` cannot see a
`go` statement at all
[measured e66beb5:ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md.state.md ·
`cat ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md.state.md` → the issue body's
§ Verified facts, "**`forbidigo` cannot forbid a `go` statement.** It visits identifiers and selector
expressions only, and `go` is a statement keyword"], so the gate for it is a structural guard
test over the module's own source — the shape `internal/leaktest/guard_test.go` already uses for a
repository-wide proposition, walking and parsing through `internal/srcguard`
[measured e66beb5:internal/srcguard/srcguard.go · `cat internal/srcguard/srcguard.go` → `WalkSubtree`,
`NonTestFile`, `ParseFile`, `WriteScratchFile`, and the package comment's rule that "every predicate —
what a guard actually forbids — stays in the package that owns the proposition"]. Because that rule
puts the predicate in the package that owns it and no existing package owns "every launch has a named
owner", the guard gets a package of its own.

**3. The written rules and the reviewer's checklist go where the owner put them (AC11, AC12).**
`ai-docs/code-style.md` § Concurrency grows from that bullet into the enumerated rules, and gains the
reviewer's checklist beside them.

**4. The composition root takes ownership of its HTTP client (AC15).** `assemble` stops handing a nil
client down: it uses the caller's when one is supplied and otherwise builds the process's own from a
clone of the default transport, threads that one value into both the Bot API client and the canary
legs, and appends a closer that releases its idle connections — the same closer list the start-up
unwind and the drain already walk backwards. `CloseIdleConnections` is exactly the operation AC15
names, and it leaves the client usable
[measured e66beb5 · `go doc net/http.Client.CloseIdleConnections` → "closes any connections on its
Transport which were previously connected from previous requests but are now sitting idle in a
'keep-alive' state. It does not interrupt any connections currently in use."].

### Key decisions

- **D1 — The `forbidigo` patterns forbid `context.Background`, `context.TODO`, `time.Tick` and
  `time.After`, and the carve-outs are exactly the ones the spec names.** An exclusion rule on
  `^cmd/`, which is the spec's "outside `main` packages" — every `cmd/` directory is a `main` package
  and no file under `internal/` is [measured e66beb5 · `rg -n '^package main$' --type go internal/` →
  no output; `rg -l '^package main$' --type go cmd/` → `cmd/bot`, `cmd/commentrefs`,
  `cmd/contentionverdict`, `cmd/importguard`, `cmd/testpg`] — and the standing layout rule that keeps
  it exact [measured e66beb5:ai-docs/code-style.md · `cat ai-docs/code-style.md` → § Source files,
  "Go only, under `cmd/<binary>/` (process entry points) and `internal/<package>/` (everything
  else)"]; and an exclusion rule on `_test\.go`, which is the spec's "and tests". Nothing else is
  excluded, which is what makes the gate reach the non-test source of a package only tests import on
  the same terms as production code (AC14).

- **D2 — `run.relative-path-mode: gomod` is pinned, because the `^cmd/` anchor is meaningless without
  it.** The exclusion regex is matched against the path as rendered, and the rendering depends on
  where the configuration file sits: run from a configuration outside the module root, the same rule
  failed to exclude anything and every `cmd/` site was reported
  [measured e66beb5 · the candidate configuration copied to `tmp/` and run twice, once with
  `run.relative-path-mode: gomod` and once without → with it, the reported set is
  `internal/health/canary.go`, `internal/health/process.go`, `internal/health/server.go`,
  `internal/ingest/attempt.go`, `internal/scheduler/execute.go`, `internal/store/basis_test.go`,
  `internal/testdb/testdb.go` at `Main`, at `Schema` and at `Schema`'s cleanup dropper, and
  `internal/tgtest/tgtest.go`; without it, every path gained a `../`
  prefix and `cmd/bot/run.go`, `cmd/commentrefs/run.go`, `cmd/contentionverdict/run.go`,
  `cmd/importguard/run.go` and `cmd/testpg/run.go` joined the report]. Pinning the mode makes the
  anchor mean the module root wherever the gate is invoked from.

- **D3 — The issue-truncation caps are lifted, because AC10 is a claim about *every* site.** The
  defaults hid most of the fresh-context sites behind the same-issue cap: the first run of the
  candidate configuration reported the `internal/health` sites and nothing else of that class, and
  lifting the caps revealed the scheduler, `internal/testdb` and `internal/tgtest` sites
  [measured e66beb5 · the candidate configuration run twice, once with the defaults and once with
  `--max-same-issues 0 --max-issues-per-linter 0` → the first run reports `context.Background`
  forbidden at `internal/health/canary.go`, `internal/health/process.go` and
  `internal/health/server.go`; the second adds `internal/scheduler/execute.go`,
  `internal/testdb/testdb.go` at `Main`, at `Schema` and at `Schema`'s cleanup dropper, and
  `internal/tgtest/tgtest.go`]. AC10 says every site the gate
  *would otherwise report* is handled, and a gate that stops reporting after a few of a kind cannot
  establish that — the same hazard `design-writer`'s own truncating-gate rule names. The keys live in
  the top-level `issues:` section and the whole configuration loads with them present
  [measured e66beb5 · the candidate configuration plus `issues: {max-issues-per-linter: 0,
  max-same-issues: 0}` → the run loads and reports the same set the CLI flags produced]. Lifting them
  is safe on today's tree: with the caps off and every existing linter enabled, the only findings are
  the new gates' own [same measurement → every reported line carries `(forbidigo)` or `(govet)`, and no other linter
  named].

- **D4 — `time.After` is forbidden outright in gated files, not only inside a loop, and that is a
  deliberate widening.** No linter of the installed golangci-lint can express "inside a loop" for it:
  `staticcheck`'s own rule for the unstoppable-timer class is gated off by the module's Go directive
  [measured e66beb5:ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md.state.md ·
  `cat ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md.state.md` → the issue body's
  § Verified facts, "**staticcheck's SA1015 (`time.Tick` outside `main`) does not fire on this
  module.** It is gated off from Go 1.23 … the same one-file probe under this repository's
  `.golangci.yml` reports it with a `go 1.22` directive and stays silent with the module's 1.26.0"],
  and `forbidigo` matches identifiers and selectors with no statement context (§ Chosen solution,
  mechanism 2). The widening is coherent rather than arbitrary — the rule this task writes down says a
  timer or a ticker is stopped, and `time.After` is precisely the timer that cannot be — and the
  escape is the one the repository already uses everywhere else, a `//nolint:forbidigo` carrying a
  specific linter and a stated reason, which the gate honours
  [measured e66beb5 · a scratch package whose only `context.Background()` carries
  `//nolint:forbidigo // <reason>` run under `forbidigo` plus `nolintlint` with
  `require-explanation`/`require-specific` → `0 issues.`]. § Open questions carries this for the
  owner.

- **D5 — The gated-file `time.After` is fixed, not excused.** It sits in the ingest retry loop's
  `select` beside `ctx.Done()`
  [measured e66beb5:internal/ingest/attempt.go · `sed -n '1,45p' internal/ingest/attempt.go` →
  `runAttempts`'s `for attempt := range l.cfg.RetryMaxAttempts` containing
  `select { case <-ctx.Done(): … case <-time.After(delay): }`]. It becomes a small unexported
  wait-or-cancel helper in `internal/ingest` holding a `time.NewTimer` with its `Stop` deferred in the
  helper's own body — so the timer is stopped on both exits and no `defer` lands inside a loop, which
  AC8's gate would report. The near-twin `waitUntil` in `internal/tg` takes an absolute instant rather
  than a duration and is left alone
  [measured e66beb5:internal/tg/caller.go · `sed -n '295,325p' internal/tg/caller.go` →
  `func waitUntil(ctx context.Context, t time.Time) error` over `time.NewTimer(d)` with
  `defer timer.Stop()`]: two sites with different signatures is the borderline case of the
  shared-package rule, and lifting a shared helper would pull `internal/tg` into a task that does not
  ask for it. Recorded here so the trade-off is auditable rather than invisible.

- **D6 — Every fresh-root-context site that survives keeps its context, and each carries a stated
  reason and a named owner (AC5).** They are the composition-independent lifetimes, and each already
  has an owner to name: the canary's tick loop, whose `Shutdown` cancels it and waits on its done
  channel; the health listener's bind and its graceful-stop goroutine, owned by the same `Shutdown`;
  the readiness gauge's per-scrape context, which already carries its own timeout; the scheduler's
  deadline watchdog; and the test-helper packages' own roots — `testdb.Main`, which is a test
  binary's entry point and has no caller context above it, `testdb.Schema` and its cleanup dropper,
  whose signature is fixed by design, and `tgtest.New`, whose `tb.Cleanup` cancels it and closes the
  server [measured e66beb5:internal/tgtest/tgtest.go · `sed -n '92,98p' internal/tgtest/tgtest.go` →
  the `tb.Cleanup` calling `cancelBase()` then closing the server and the listener].

- **D7 — "Detached" means the caller does not wait, and by that test the scheduler's deadline
  watchdog is the only detached site.** AC5's timeout clause needs a definition the implementor can
  apply the same way twice; this is it. The watchdog is detached — it outlives `executeOne`, which
  returns before the orphaned handler does
  [measured e66beb5:internal/scheduler/execute.go · `sed -n '140,160p' internal/scheduler/execute.go`
  → the `go func()` that waits on `resultCh` and then calls `pconn.Close(context.Background())`, with
  `executeOne` returning nil immediately after] — so it gains its own bound, a named constant beside
  the code in the shape `pingTimeout` already sets
  [measured e66beb5:cmd/bot/assemble.go · `sed -n '24,29p' cmd/bot/assemble.go` → "A named constant,
  not a configuration key"]. This is not a balance value, so § *Magic numbers vs balance constants*
  puts it in Go rather than in configuration. Every other surviving site is awaited by its caller and
  takes no timeout.

- **D8 — The bare-`go` allow list is a table keyed by symbol, not by line, and it lives in the guard's
  own source.** One row per gated launch, keyed by the launch's package directory and the name of the
  outermost enclosing function declaration — a method written with its receiver type — carrying, per
  launch that function makes and in source order, its answers: how it stops, who waits for it, where
  its error goes, and where its panic goes. The checker reports a `go` statement whose key is absent from
  the table, a row whose answers do not cover every launch its function makes, and any answer left
  empty [derived → AC1 and AC2, established by the guard and its discriminating twin in § Test
  Design]. Keying by symbol rather than by file position is the form this repository requires of a
  reference written down to be read later
  [measured e66beb5:ai-docs/doc-convention.md ·
  `grep -n 'Durable references' ai-docs/doc-convention.md` → the section "Durable references — a
  place named to be read later names a SYMBOL"]; a rename or a move therefore forces a table edit,
  which is a re-review of the launch and not an accident.

- **D9 — The gated set for that guard is the module's compiled non-test source.** Every `.go` file
  that is not a `_test.go` file and does not lie under a directory the go tool itself never compiles —
  `testdata`, or a name beginning with `.` or `_`. `main` packages are **not** carved out: the spec
  carves `main` out of the lint-gate criteria only, and the composition root's own join goroutine is a
  launch that deserves its answers as much as any other. That directory predicate exists today as an
  unexported helper inside the leak-detection guard
  [measured e66beb5:internal/leaktest/guard_test.go · `sed -n '44,56p' internal/leaktest/guard_test.go`
  → `excludedByDirName`, skipping `testdata` and any part beginning with `.` or `_`]; it is mechanical
  walking, which is `internal/srcguard`'s stated charter, so it moves there and the private copy is
  deleted rather than duplicated.

- **D10 — AC14 gets a guard of its own, over the lint configuration's reach.** The proposition — every
  gate binds the non-test files of a test-only package on the same terms as production code — is a
  property of the exclusion rules, so the guard reads `.golangci.yml`, compiles every
  `linters.exclusions.rules[].path` pattern, and fails when any of them matches a non-test `.go` file
  under `internal/`. That keeps `internal/testdb` and `internal/tgtest` bound by construction and
  fails loudly the day someone adds a carve-out for one of them, which is the same posture the file
  limits already take — exempt in a reviewed diff to the gate itself, never with an inline escape
  [measured e66beb5:ai-docs/code-style.md · `cat ai-docs/code-style.md` → § File size, "There is no
  `//nolint` channel and no magic comment … exempted by adding a path prune to the `file-limits`
  recipe in `Makefile`, in a reviewed diff"]. The YAML parser it needs is already a direct requirement
  [measured e66beb5:go.mod · `cat go.mod` → `go.yaml.in/yaml/v3 v3.0.5` in the first require block].

- **D11 — Both guards live in one new package, `internal/gateguard`.** Its package comment states the
  shared proposition: the gates this module points at its own source — which files they reach, and
  what they demand of a goroutine launch. It holds a `doc.go`, the guard file, and the `main_test.go`
  every package with tests carries. No `make` target and no CI job is added: a guard test runs inside
  `go test ./...`, which every gate route this project owns already reaches
  [measured e66beb5:Makefile · `cat Makefile` → `test`, `test-race`, `test-fallback` and
  `test-contention` each invoking `go test` over `./...`, and `verify` naming `test` and `test-race`],
  and the CI path filter for Go work already covers new `.go` files and the lint configuration
  [measured e66beb5:.github/workflows/ci.yml · `sed -n '36,60p' .github/workflows/ci.yml` → the `go`
  filter listing `'**/*.go'` and `'.golangci.yml'`].

- **D12 — The process HTTP client is folded into the existing start-up step, not added as a new
  one.** The lifecycle prose, KD-32 and the orientation page each state the start-up order's step
  count [measured e66beb5:ai-docs/process-lifecycle.md,ai-docs/key-decisions.md,ai-docs/context.md ·
  `grep -rniE 'fourteen' --include='*.md' .` → the lifecycle's "`assemble` runs the fourteen steps
  below in this order", KD-32's "Fourteen steps in a fixed order", and the orientation page's
  "a fourteen-step, all-fatal, unwinding start-up order"]. Building the client inside the Telegram
  client step keeps that number true and still puts its closer before the canary's, so the drain
  releases the idle connections after the canary has stopped using them — the closer list is walked
  backwards [measured e66beb5:cmd/bot/assemble.go,cmd/bot/serve.go · `cat cmd/bot/assemble.go` →
  `unwind` walking `a.closers` from the last appended; `cat cmd/bot/serve.go` → `drain`'s identical
  backwards walk]. When the caller supplies a client the process uses that one and still releases its
  idle connections on the way out, so AC15 holds on every route rather than only the production one.

### Rejected alternatives

- **A `gocritic` `ruleguard` rule for "`time.After` inside a loop".** It would give AC3 its exact
  shape instead of D4's widening, at the price of a rule file in a directory the go tool skips, an
  experimental checker enabled by name, and a module that is not a dependency today
  [measured e66beb5:go.mod,go.sum · `grep -n "quasilyte\|go-ruleguard" go.mod go.sum` → no output].
  D4's widening plus a reasoned `//nolint` is the cheaper shape for a module whose only gated-file
  instance of the identifier is being removed anyway. § Open questions puts the trade to the owner.

- **A central allow list keyed by file and line.** Refused by the repository's own rule for a
  reference written down to be read later
  [measured e66beb5:ai-docs/doc-convention.md ·
  `grep -n 'Durable references' ai-docs/doc-convention.md` → the section "Durable references — a
  place named to be read later names a SYMBOL"], and for the reason behind it: a line coordinate goes
  stale on the next edit and turns into a false finding at review time. D8's symbol key moves with
  the code.

- **A marker comment on each `go` statement, parsed by the guard.** It keeps the answers beside the
  code, but it invents a comment grammar this module does not have, and it puts a block of ceremony
  at every launch in a repository whose owner's standing position is that comments are not themselves
  the value. The allow list makes the same answers reviewable in one place, and makes
  adding a launch a diff in the file whose entire purpose is reviewing launches.

- **A new `cmd/` binary plus a `make` target for the launch guard, in the shape of the comment and
  import gates.** It would need its own walker, because `internal/srcguard`'s API is `testing.TB`-
  bound, and its own `make` target and CI wiring. The guard-test shape reuses the walker and rides
  gate routes that already exist.

- **`http.DefaultClient.CloseIdleConnections()` on the shutdown path, leaving the fallback in
  place.** It would satisfy the letter of AC15 while leaving the process operating on a package-level
  global it shares with anything else in the binary, and it would leave the lifecycle prose's "the
  process HTTP client" still describing something that does not exist.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Enable `containedctx` and `fatcontext`; pin `run.relative-path-mode: gomod` (D2); lift the issue-truncation caps in a top-level `issues:` section (D3). Both linters are silent on today's tree, so the gate stays green with no source change [measured e66beb5 · `golangci-lint run --enable-only=containedctx ./...` → `0 issues.`; the same for `fatcontext`]. Serves AC6, AC7. | `.golangci.yml` | — |
| 2 | Enable `gocritic`'s `deferInLoop` and `govet`'s `nilness`. `deferInLoop` reports nothing on today's tree and `nilness` reports the deliberate typed-nil-in-interface assertion in `internal/store` and nothing else [measured e66beb5 · the candidate configuration on the tree with the truncation caps lifted → `internal/store/basis_test.go:73:11: nilness: impossible condition: non-nil == nil (govet)`, and no `gocritic` line]; that assertion's existing directive gains `govet` beside `staticcheck` and states the added linter's reason. Serves AC8, AC9, AC10. | `.golangci.yml`, `internal/store/basis_test.go` | 1 |
| 3 | Replace the ingest retry loop's unstoppable timer with a wait-or-cancel helper holding a stopped timer (D5). Test first. Serves AC3, AC10. | `internal/ingest/attempt.go`, `internal/ingest/retry_test.go` | — |
| 4 | Bound the scheduler's detached connection close with its own named-constant timeout (D7). Serves AC5. | `internal/scheduler/execute.go` | — |
| 5 | Enable `forbidigo` with the patterns and the carve-outs (D1, D2, D4); give every surviving fresh-root-context site its `//nolint:forbidigo` carrying a stated reason and a named owner (D6). The gate must be green at this subtask's commit, so the enabling and the annotations land together. Serves AC4, AC5, AC10, AC14. | `.golangci.yml`, `internal/health/canary.go`, `internal/health/process.go`, `internal/health/server.go`, `internal/scheduler/execute.go`, `internal/testdb/testdb.go`, `internal/tgtest/tgtest.go` | 2, 3, 4 |
| 6 | Move the compiled-directory predicate into `internal/srcguard` with its own table test, and fold the leak-detection guard's private copy into it (D9). Serves AC1 (as the guard's scope rule). | `internal/srcguard/srcguard.go`, `internal/srcguard/srcguard_test.go`, `internal/leaktest/guard_test.go` | — |
| 7 | Give the composition root its own HTTP client, thread it into the Telegram client and the canary legs, and append the closer that releases its idle connections (D12). Test first. Serves AC15. | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go`, `cmd/bot/serve_test.go` | — |
| 8 | Add `internal/gateguard`: the launch allow list and its checker (D8, D9), the lint-exclusion-reach guard (D10), and a discriminating twin for each (D11). The allow list is written against the tree subtasks 1–7 leave. Serves AC1, AC2, AC14. | `internal/gateguard/doc.go`, `internal/gateguard/guard_test.go`, `internal/gateguard/main_test.go` | 5, 6, 7 |
| 9 | Write the ownership rules and the reviewer's checklist into `ai-docs/code-style.md` § Concurrency, and bring § Linter posture's enumeration in line with the enabled set. Serves AC11, AC12, and part of AC13. | `ai-docs/code-style.md` | — |
| 10 | Sweep every live surface for a claim this diff falsifies and fix each: the decision log's linter enumeration and its closer-list decision, the orientation page's package layout and gate list, and the lifecycle page's start-up-step row and closer walk. The class is every live site whose claim the diff falsifies, not the list drafted here. Serves AC13. | `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/process-lifecycle.md`, `ai-docs/context-status.md` | 1–9 |

## Handoff plan

Grouping is required for every `M ≥ 1`, and this design's `M` is the Decomposition table's subtask
count; the contract's sub-points (a)–(h) are applied below. Every group is homogeneous by
change-type, marked with its implementor model and effort, and the group count is the minimum the
change-type split and the dependency order allow.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The first group gets a handoff exactly as every later one does.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–8 (code change-type: `*.go` plus the lint gate's own configuration, which is
  the gate over Go source and belongs with it). Every same-change-type subtask is clustered into this
  one group rather than interleaved with the prose work; the group is within the `≤ 10` size cap, and
  the dependency order inside it (2→1, 5→2,3,4, 8→5,6,7) is respected by the numbering.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`
  override, 1M-token window — subtasks 9–10 (instructions/harness change-type: `ai-docs/**`).
  Terminal group, sized 2, within the `1..=10` range.

Group-count check: the change-type switch between subtask 8 and subtask 9 forces the boundary, and
subtask 10 depends on the whole code group, so no reordering collapses the two groups into one. The
total is within the default maximum of 4 design-defined groups, so no user approval is needed.

## Risks

- **A truncating gate turns "every site" into "the first few of a kind", and it already did here.**
  The first candidate run reported only part of the fresh-context class and looked complete
  [measured e66beb5 · the candidate configuration run with golangci-lint's defaults → the
  `internal/health` sites and nothing of the scheduler or the test-helper packages; the same run
  with `--max-same-issues 0 --max-issues-per-linter 0` → those sites plus
  `internal/scheduler/execute.go`, `internal/testdb/testdb.go` at `Main`, at `Schema` and at
  `Schema`'s cleanup dropper, and `internal/tgtest/tgtest.go`].
  *Mitigation:* subtask 1 lifts the caps before any site work begins (D3), and subtask 5 re-runs the
  gate after its last annotation rather than trusting the list this design names.

- **The `^cmd/` carve-out is a path proxy for "main package", and a `main` package outside `cmd/`
  would slip past AC4's gate.** No such package exists today
  [measured e66beb5 · `rg -n '^package main$' --type go internal/` → no output]. *Mitigation:* the
  standing layout rule keeps main packages under `cmd/` and is stated in the same file this task
  edits; the residue is named here rather than closed, because a guard for it is scope the spec does
  not ask for.

- **Enabling an analyzer by name can silently narrow the set that was running.** It does not here:
  with `govet.enable: [nilness]`, the default analyzers still fire, and with
  `gocritic.enabled-checks: [deferInLoop]`, the default checkers still fire
  [measured e66beb5 · a scratch module with a `fmt.Sprintf("%d", "not an int")` and with `x = x + 1`
  plus a single-case type switch, run under those settings → `printf: fmt.Sprintf format %d has arg
  "not an int" of wrong type string (govet)`, `assignOp` and `singleCaseSwitch` (gocritic)].

- **The rule this task writes down — a goroutine running an interface-supplied handler recovers a
  panic at that boundary — lands while a named site does not comply.** The scheduler's handler goroutine
  has no recover [measured e66beb5 · `rg -n 'recover\(\)' --type go --glob '!*_test.go'` →
  `internal/ingest/attempt.go` only]. *Mitigation:* the spec puts that site in #81 and out of scope;
  the allow-list row records today's answer honestly rather than an aspirational one, so the gap is
  visible where a reviewer looks. Carried to § Open questions.

- **Subtask 4 edits a site #81 is slated to rework.** *Mitigation:* the change is one bounded context
  around one close, with no change to the watchdog's structure or to what it waits on; § Open
  questions puts it to the owner.

- **A caller-supplied HTTP client now has `CloseIdleConnections` called on it during the drain.** The
  client stays usable afterwards, so a test that reuses it is unaffected
  [measured e66beb5 · `go doc net/http.Client.CloseIdleConnections` → "It does not interrupt any
  connections currently in use."].

- **`http.DefaultTransport` is asserted to a concrete type to clone it, and a failed assertion must
  not panic — the module's panic index carries no row and this change adds none**
  [measured e66beb5:ai-docs/panic-index.md · `tail -12 ai-docs/panic-index.md` → "**The project
  targets zero production panics and currently holds it** — the table below is empty", above a table
  whose only row is `| — | — | — |`]. *Mitigation:* the comma-ok form returns a start-up error naming
  the step, which is the shape every other `assemble` failure already takes [derived → the
  assemble-failure case in § Test Design].

- **A guard is an instrument before it is evidence.** A green launch guard over the real tree proves
  nothing until the same checker has been seen to fail. *Mitigation:* each guard ships a
  `_ProvenDiscriminating` twin driving it over a constructed violating scratch tree, the convention
  the health guards already follow
  [measured e66beb5:internal/health/guards_test.go · `grep -n '^func ' internal/health/guards_test.go`
  → `TestGuard_NoPromautoOrDefaultRegisterer_ProvenDiscriminating`,
  `TestGuard_ObservationRegisterBindsStructsBothDirections_ProvenDiscriminating`,
  `TestGuard_NoPanicLogFatalOsExitOrLibraryMustCall_ProvenDiscriminating`].

- **The coverage ratchet.** Subtask 7 adds production statements to `cmd/bot`; subtask 8's package is
  test source and a `doc.go` holding only a package comment, so it adds none [derived → subtask 8's
  file set in § Decomposition]. *Mitigation:* subtask 7's tests drive the new branch in both
  directions (a supplied client and a constructed one), so the added statements are covered in the
  same commit [derived → subtask 7's cases in § Test Design].

## Test Design

### Subtasks 1 and 5 — the gate additions that carry no Go test

- Location: none. Each subtask's deliverable is the lint gate's own configuration plus, in subtask 5,
  the directives at the sites it reports; the verifier is the gate.
- Verification, and the discriminating half stated rather than assumed: for every pattern and checker
  enabled, a constructed violating package must be seen reported before the tree's own run is
  believed — the scratch-module controls in § Approach are that evidence, and subtask 5 repeats them
  for the `^cmd/` and `_test\.go` carve-outs by checking that the same shape inside a `main` package
  and inside a `_test.go` file is *not* reported while the shape in `internal/` non-test source is
  [derived → AC3, AC4, AC6, AC7, AC14].
- The `//nolint:forbidigo` escape must be seen both to suppress the finding and to be refused when it
  carries no specific linter or no explanation, which `nolintlint`'s existing settings already require
  [derived → AC5].

### Subtask 2 — the nil-comparison gate

- Location: no new test. The change is a directive on an existing assertion.
- Verification: the lint gate itself. Before the directive is extended the gate reports the site
  [measured e66beb5 · the candidate configuration on the tree → `internal/store/basis_test.go:73:11:
  nilness: impossible condition: non-nil == nil (govet)`]; after, it is silent, and `nolintlint`'s
  `require-specific` and `require-explanation` keep the directive honest.

### Subtask 3 — the ingest wait

- Location: `internal/ingest/retry_test.go`, beside the existing retry cases.
- Entry point: the unexported wait-or-cancel helper, and `(*Loop).runAttempts` through it.
- Scenarios: the wait returns nil once the duration elapses; it returns the context's error when the
  context is cancelled first, and returns promptly rather than at the end of the duration; a zero or
  negative duration returns without waiting; and `runAttempts` still returns the context's error when
  cancellation arrives between attempts, which the existing retry cases already assert and must keep
  asserting [derived → AC3 and AC10].
- Fixtures: none beyond a cancellable context; the helper takes a duration, so no clock seam is
  needed.

### Subtask 4 — the bounded detached close

- Location: no new test, stated with its reason rather than left as a gap. The value closed is a
  concrete `pgx` connection type with no seam to substitute, and the behaviour the bound changes —
  what happens when a socket never drains — is not reproducible in-process without one. The existing
  deadline-breach cases in `internal/scheduler` drive the path the watchdog runs on and must stay
  green [derived → AC5, with the gate's own report as the check that the site carries its reason].

### Subtask 6 — the directory predicate

- Location: `internal/srcguard/srcguard_test.go`.
- Entry point: the new exported predicate.
- Scenarios: a path directly under the root; a path under `testdata`; a path under a directory whose
  name begins with `.` and one beginning with `_`; a path whose *file* name begins with `_` but whose
  directories do not; a nested directory where only an inner segment is excluded
  [derived → the guard scope D9 states].
- Regression: the leak-detection guard must report the same real-tree result after its private copy
  is deleted as before [derived → subtask 6's own green run of `internal/leaktest`].

### Subtask 7 — the process HTTP client

- Location: `cmd/bot/assemble_test.go` for construction, `cmd/bot/serve_test.go` for the shutdown
  path.
- Entry point: `assemble`, and `(*app).serve` through its drain.
- Scenarios:
  - assembled with no client supplied, the process holds a non-nil client of its own, and it is not
    the package-level default [derived → AC15];
  - assembled with a client supplied, the process holds that one [derived → AC15];
  - driven through a full `serve` that ends on a signal, the client the process holds has had its
    idle connections released, observed through a transport stub that records the call — and the
    record is ordered after the canary's own closer, which is what "released after the users stopped
    using it" means on a list walked backwards [derived → AC15];
  - the same test with the closer removed must go red; this is the discriminating half and is
    exercised once, deliberately, before the case is trusted [derived → the green-instrument rule];
  - `assemble` returns its `stepError` naming the Telegram-client step, rather than panicking, when
    the transport cannot be cloned [derived → the panic-surface risk row].
- Fixtures: the existing fake Bot API server supplies the transport for the supplied-client cases; the
  recording stub is a round-tripper that delegates to it and counts the release call. No end-state
  socket assertion is specced: the fake server is deliberately socket-free, and standing up a second,
  real-listener Bot API stub to count connections would duplicate it for one assertion whose
  underlying contract is already documented behaviour of the standard library.

### Subtask 8 — the guards

- Location: `internal/gateguard/guard_test.go`, with `main_test.go` running the module's
  goroutine-leak check as every package with tests does.
- Entry points: the launch checker over a tree root, and the exclusion-reach checker over a lint
  configuration path.
- Scenarios, launch checker:
  - the real tree passes, every launch the allow list names accounted for [derived → AC1, AC2];
  - a scratch package with a `go` statement whose enclosing function is absent from the table fails,
    and the failure names the package and the function [derived → AC1];
  - a scratch package whose table row leaves one answer empty fails [derived → AC2];
  - a scratch package whose function makes more launches than its row has answers fails [derived →
    AC2];
  - a launch inside a nested function literal is attributed to the outermost enclosing declaration
    [derived → D8];
  - a `go` statement under `testdata` or under a `.`/`_`-prefixed directory is not gated, and one in a
    non-test file of a package only tests import **is** [derived → AC14];
  - the joined forms are not `go` statements and are not gated [derived → D8].
- Scenarios, exclusion-reach checker:
  - the repository's own lint configuration passes [derived → AC14];
  - a scratch configuration whose exclusion rule matches a non-test path under `internal/` fails, and
    the failure names the rule [derived → AC14];
  - a scratch configuration excluding only `_test.go` and `^cmd/` passes [derived → D1].
- Fixtures: `internal/srcguard`'s scratch-file writer for every constructed tree, so no case ever
  touches the working tree; `internal/repotest` for the repository root and the lint configuration's
  path.

### Subtasks 9 and 10 — prose

- No test. The checks are the harness gates the repository already runs over Markdown — the link
  check in CI's harness job, and a read of each amended claim against the tree it now describes
  [derived → AC11, AC12, AC13].

### Gates

`make verify` after every subtask, and `make test-race` for subtask 7, which changes what the
composition root holds and closes.

## Open questions

- **D4's widening.** The lint gate cannot express "`time.After` inside a loop", so this design
  forbids the identifier outright in gated files and leaves a reasoned `//nolint` as the escape.
  The alternative is a `gocritic` `ruleguard` rule that expresses the loop condition exactly, at the
  cost of a rule file, an experimental checker and a module that is not a dependency today. Confirm
  the widening, or ask for the precise gate.

- **D3's reach.** Lifting the issue-truncation caps makes the lint gate report every finding of every
  linter, not only the new ones. It is what makes AC10's "every site" checkable, and today's tree is
  clean either way (D3's second measurement), but it changes the gate's behaviour beyond the criteria
  that asked for it. Confirm or strike.

- **Subtask 4 against #81.** Bounding the scheduler's detached connection close is what AC5's timeout
  clause asks for at the one detached site, and that site is inside the code #81 is to rework.
  Confirm the minimal change lands here, or defer the bound to #81 and leave the site with its stated
  reason and named owner alone.

- **The rule that arrives ahead of its one exception.** Scope item 6 has this task write down that a
  goroutine running an interface-supplied handler recovers a panic at that boundary; the scheduler's
  handler goroutine does not, and #81 owns it. Confirm the rule lands now with the gap visible in the
  allow list, rather than waiting for #81.
