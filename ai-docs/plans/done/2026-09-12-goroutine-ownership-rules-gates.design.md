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

  **Both carve-outs are scoped `linters: [forbidigo]`, and that scoping is load-bearing.** An
  exclusion rule with a `path` and no `linters` list switches *every* linter off for that path, so an
  unscoped `^cmd/` rule would silently take `containedctx`, `fatcontext`, `gocritic`'s `deferInLoop`
  and `govet`'s `nilness` off every `main` package — the criteria that carry those shapes (AC6–AC9)
  name no `main` carve-out, only AC3 and AC4 do. It would not go red: `cmd/` carries none of those
  shapes today, and D10's reach guard inspects `internal/` non-test paths only. For `^cmd/` that
  means a new rule whose `linters` list is `forbidigo` alone; for `_test\.go` it means **appending**
  `forbidigo` to the list the existing rule already carries rather than adding a second rule
  [measured 20cf1aa:.golangci.yml · `cat .golangci.yml` → `linters.exclusions.rules` carries
  `path: _test\.go` with `linters:` `goconst`, `gosec`, `unparam`, and no other rule].

  The settings key is `linters.settings.forbidigo.forbid`, each entry a `pattern`. The name matters
  more than a spelling usually does here, because a wrong one is not an error: see the
  silently-ignored-settings-key row in § Risks.

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

- **D3 — Every default that stops the gate reporting a site it would otherwise report is switched
  off, because AC10 is a claim about *every* site.** Two independent mechanisms do that, and closing
  one leaves the other open: the issue-truncation caps, and the cross-linter line dedup.

  **The caps.** The defaults hid most of the fresh-context sites behind the same-issue cap: the
  first run of the
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
  is safe on today's tree, and that is pinned to a run with the *second* suppressor off as well: a
  run made under the line dedup could not have seen a finding another linter had claimed on the same
  line, which is the next paragraph's whole point, so it is not evidence this paragraph may rest on.
  With the caps off, the dedup off and every existing linter enabled, the only findings are the new
  gates' own [measured e66beb5 — the base tree, read in a detached worktree — under the gate
  configuration at 52ef68e plus `issues: {uniq-by-line: false}` ·
  `golangci-lint run -c <that file> ./...` → every reported line carries `(forbidigo)` or `(govet)`,
  and no other linter named].

  Put to the owner on 2026-09-12 as § Open questions' "D3's reach", and confirmed in their words:
  *"Set max-issues-per-linter and max-same-issues to 0. Verified independently: today's tree is 0
  issues under the CURRENT config with the caps lifted, so nothing goes red. Under the defaults the
  gate was truncating and hiding real sites."*

  **The line dedup, found at Step 9's per-AC sweep and closed by the same argument.**
  `issues.uniq-by-line` defaults to `true`, and it keeps one finding per source line across *all*
  linters, so a site is reported only when no other linter has claimed its line first
  [measured 3e72070 · `golangci-lint run --help` → `--uniq-by-line  Make issues output unique by
  line (default true)`]. The shape that collides is the canonical one for AC8's own checker: a
  `defer f.Close()` inside a loop draws `errcheck` on the same line, and under this repository's
  configuration the `gocritic` finding is the one dropped — the checker is enabled and correct, and
  only the dedup hides it [measured 3e72070:.golangci.yml · a scratch module outside the tree whose
  loop body opens a file and defers its close, built first, run under this repository's
  configuration → the report carries `errcheck`'s finding at that line and names no `gocritic`
  finding; the same module under `--enable-only=gocritic` → `deferInLoop: Possible resource leak,
  'defer' is called in the 'for' loop`; the same module under the configuration plus
  `issues: {uniq-by-line: false}` → both findings]. The suppression is also *transient*, which is
  what makes it a reporting hazard rather than a stable omission: silence the competing finding and
  the hidden one surfaces [measured 3e72070 · the same scratch module with a `//nolint:errcheck` on
  the colliding line, under the unmodified configuration → the `deferInLoop` finding reported].

  So which sites the gate reports depends on what some *other* linter happens to find on the same
  line, and subtask 5's method — annotate the sites the gate's own re-run reports — is therefore
  reading a set that is not the whole one. That is AC10's "every site it would otherwise report"
  failing through a second door, and the argument that lifted the caps closes it unchanged.
  `golangci-lint config verify` accepts the key, and the verifier is discriminating rather than
  merely permissive — it rejects a misspelling of it [measured 3e72070 · `golangci-lint config
  verify` on this repository's configuration plus `issues: {uniq-by-line: false}` → exit 0, no
  output; on the same file with the key spelled `uniq-by-lines` → exit 3, ``jsonschema: "issues" …
  additional properties 'uniq-by-lines' not allowed``]. The change costs nothing on today's tree
  [measured 3e72070 · `golangci-lint run` over `./...` under this repository's configuration, and
  again with `issues: {uniq-by-line: false}` → `0 issues.` and exit 0 both times].

  Unlike the caps, this key's safe value is a **boolean `false`, and its absence is the unsafe
  state.** A cap is pinned to the integer `0` meaning "no limit", and the guard's existing predicate
  for that already treats an absent key as a finding
  [measured 3e72070:internal/gateguard/guard_test.go · `grep -n -A12 'func isZeroInt'
  internal/gateguard/guard_test.go` → a nil map and a key of any non-numeric type both return
  `false`]. An absent `uniq-by-line`, by contrast, silently means `true`. D10's assertion is written
  for that asymmetry.

  Put to the owner on 2026-09-12 as a scope deviation, and answered in their words: *"Add +
  re-review. Add `uniq-by-line: false` beside the two caps. design-writer amends D3; design-review
  re-runs (round 2 of 3). The unconditional-re-review default — slowest, most checked."*

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
  `require-explanation`/`require-specific` → `0 issues.`].

  Put to the owner on 2026-09-12 as § Open questions' "D4's widening", and confirmed in their words:
  *"Forbid `time.After` outright in gated files; a site that needs it carries //nolint with a stated
  reason. Stricter than AC3 asked for. Zero new dependencies, one linter pattern."* The widening is
  therefore the design's shape, and the `ruleguard` alternative below is closed, not deferred.

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
  reason and a named owner (AC5). The list is exactly the set the gate reports, and it is that set
  because an annotation with nothing under it is itself a finding.** They are the
  composition-independent lifetimes, and each already has an owner to name: the canary's tick-loop
  root, whose `Shutdown` cancels it and waits on its done channel; the health listener's bind, owned
  by the same package's `Shutdown`; the readiness gauge's per-scrape context, which already carries
  its own timeout; the scheduler's deadline watchdog; and the test-helper packages' own roots —
  `testdb.Main`, which is a test binary's entry point and has no caller context above it,
  `testdb.Schema` and `Schema`'s cleanup dropper, whose signature is fixed by design, and
  `tgtest.New`, whose `tb.Cleanup` cancels it and closes the server
  [measured 20cf1aa:internal/tgtest/tgtest.go · `sed -n '92,98p' internal/tgtest/tgtest.go` →
  the `tb.Cleanup` calling `cancelBase()` then closing the server and the listener].

  The health server's *graceful-stop* goroutine is deliberately **not** on that list: it runs
  `httpSrv.Shutdown` on the caller's own `ctx`, so the gate never reports it, and
  `internal/health/server.go` yields the bind and nothing else [measured 20cf1aa · the candidate
  configuration over `./...` → `forbidigo` reports `internal/health/canary.go` at `Start`'s run
  context, `internal/health/process.go` at the readiness gauge's scrape context,
  `internal/health/server.go` at the listener bind and at no other position in that file,
  `internal/scheduler/execute.go` at the watchdog's close, `internal/testdb/testdb.go` at `Main`, at
  `Schema` and at `Schema`'s cleanup dropper, `internal/tgtest/tgtest.go` at `New`'s base context,
  and the `time.After` at `internal/ingest/attempt.go` that subtask 3 removes].
  Listing a site the gate does not report is not a harmless surplus: `nolintlint` runs with
  `allow-unused` unset, so a directive with no finding under it fails the gate [measured 20cf1aa · a
  scratch package under the candidate configuration carrying one `//nolint:forbidigo` over a real
  `context.TODO()` and one over a function with no forbidden identifier → the second reported,
  ``directive `//nolint:forbidigo …` is unused for linter "forbidigo" (nolintlint)``, the first
  silent]. Subtask 5 therefore annotates what its own re-run reports, not what this row names.

- **D7 — "Detached" means the work the fresh context bounds continues after the function that created
  that context has returned *and nothing in the process joins it*; by that test the scheduler's
  deadline watchdog is the only detached site.** AC5's timeout clause needs a definition the
  implementor can apply the same way twice, and "the caller does not wait" is not that definition —
  applied literally it catches the canary's tick loop, the health server's serve goroutine and
  `tgtest`'s server, each of which is owned perfectly well by something that is simply not the
  creating function's caller. The second half of the test is what separates them: a *join* is
  anything in the process
  that ends the work deterministically and is reachable from an owner — a channel a stopper waits on,
  or a cleanup registered before the owning scope ends. Under it:

  | Site | Joined by | Detached? |
  |---|---|---|
  | `(*Canary).Start`'s tick loop | `Shutdown` cancels, then waits on `done` | no |
  | `(*Server).Start`'s serve goroutine | `Shutdown` receives its terminal error on `serveErr` | no |
  | `tgtest.New`'s serve goroutine | the `tb.Cleanup` `New` registers, which cancels the base context and closes the server and the listener | no |
  | the scheduler's deadline watchdog | nothing — `executeOne` returns before the orphaned handler does, and no shutdown path reaches the goroutine | **yes** |

  [measured 20cf1aa:internal/health/canary.go,internal/health/server.go,internal/tgtest/tgtest.go,internal/scheduler/execute.go
  · `cat` of each → `Shutdown` selecting on `c.done`; `Shutdown`'s stopper reading `<-serveErr`;
  `New`'s `tb.Cleanup` calling `cancelBase()` then `Close()`; and the `go func()` that waits on
  `resultCh` and then calls `pconn.Close(context.Background())`, with `executeOne` returning nil
  immediately after]. The watchdog therefore gains its own bound, a named constant beside the code in
  the shape `pingTimeout` already sets
  [measured 20cf1aa:cmd/bot/assemble.go · `sed -n '24,29p' cmd/bot/assemble.go` → "A named constant,
  not a configuration key"]. This is not a balance value, so § *Magic numbers vs balance constants*
  puts it in Go rather than in configuration. No other surviving site is detached, so none takes a
  timeout — and the canary's root context and `tgtest`'s base context in particular do not, which is
  the outcome the joined column is there to make unambiguous.

  That the bound lands here rather than in #81, which is slated to rework the same site, was put to
  the owner on 2026-09-12 as § Open questions' "Subtask 4 against #81" and confirmed in their words:
  *"Land it here. Minimal change now: the detached close carries a named-constant timeout,
  satisfying AC5 at that site. #81 may rework or remove it later."*

- **D8 — The bare-`go` allow list is a table keyed by symbol, not by line, and it lives in the guard's
  own source.** One row per gated launch, keyed by the launch's package directory and the name of the
  outermost enclosing function declaration — a method written with its receiver type — carrying, per
  launch that function makes and in source order, its answers: how it stops, who waits for it, where
  its error goes, and where its panic goes. The checker reports a `go` statement whose key is absent from
  the table, a row whose answers do not cover every launch its function makes, and any answer left
  empty [derived → AC1 and AC2, established by the guard and its discriminating twin in § Test
  Design].

  **A `go` statement with no enclosing `FuncDecl` is a guard failure, not a skip.** Such a launch —
  a function literal in a package-level `var` initialiser is the reachable shape — has no key the
  table can hold, and a checker that silently walked past it would leave open exactly the hole AC1
  exists to close. The guard instead fails, naming the file and the statement's position, which
  forces whoever introduces the shape to decide how it is keyed rather than inheriting silence. This
  is forward-looking: no such launch exists in the module today
  [measured 20cf1aa · `rg -U 'var\s+\w+\s*=\s*func\(' --type go` → no output; `rg -n '^\s*go\s+'
  --type go --glob '!*_test.go'` → every hit's enclosing declaration is a `func`, cross-read against
  the file].

  Keying by symbol rather than by file position is the form this repository requires of a
  reference written down to be read later
  [measured e66beb5:ai-docs/doc-convention.md ·
  `grep -n 'Durable references' ai-docs/doc-convention.md` → the section "Durable references — a
  place named to be read later names a SYMBOL"]; a rename or a move therefore forces a table edit,
  which is a re-review of the launch and not an accident.

- **D9 — The gated set for that guard is the module's compiled non-test source.** Every `.go` file
  that is not a `_test.go` file and does not lie under a directory the go tool itself never compiles —
  `testdata`, or a name beginning with `.` or `_`. `main` packages are **not** carved out: the spec
  carves `main` out of the lint-gate criteria only, and the composition root's own join goroutine is a
  launch that deserves its answers as much as any other.

  Test source **is** carved out, and that asymmetry is a decision rather than an oversight. Scope
  item 1 states no test carve-out, unlike Scope item 2 — but the spec already assigns test launches a
  different gate: Scope item 6 has this task write down that a test stops everything it started
  through `t.Cleanup`, which is a reviewed written rule, and AC14 states its own reach as the
  "non-`_test.go` files" of a test-only package, which presupposes the boundary. A per-launch
  allow-list row for every launch in the module's test files would put the module's own fixtures and
  fakes into a table whose purpose is reviewing *production* ownership, and would answer "who waits
  for it" with the same `t.Cleanup` sentence over and over. The written rule is the gate for test
  source; the table is the gate for compiled source.

  That directory predicate exists today as an
  unexported helper inside the leak-detection guard
  [measured e66beb5:internal/leaktest/guard_test.go · `sed -n '44,56p' internal/leaktest/guard_test.go`
  → `excludedByDirName`, skipping `testdata` and any part beginning with `.` or `_`]; it is mechanical
  walking, which is `internal/srcguard`'s stated charter, so it moves there and the private copy is
  deleted rather than duplicated.

- **D10 — AC14 gets a guard of its own over the lint configuration, and that guard is also where
  subtasks 1 and 5 get their regression.** The proposition — every
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

  Since the guard has the parsed configuration in hand, it carries further assertions about it, and
  they are what turn "the gate this task added is still the gate" from a one-time scratch probe into
  a repository-resident regression — subtasks 1 and 5 have no other:

  - **Every exclusion rule names its `linters`.** A rule with a `path` and no `linters` list
    switches every linter off for that path; the guard fails on any such rule, which is D1's scoping
    made structural rather than remembered.
  - **The enabled set still holds what this task enabled** — `containedctx`, `fatcontext`,
    `forbidigo` and `gocritic` in `linters.enable`; `deferInLoop` in `gocritic`'s `enabled-checks`;
    `nilness` in `govet`'s `enable`; and a `forbid` entry for each of the four identifiers D1 names.
  - **The settings D2 and D3 pin are still pinned** — `run.relative-path-mode` is `gomod`, without
    which the `^cmd/` anchor stops meaning the module root; both issue-truncation caps are `0`; and
    `issues.uniq-by-line` is **present and boolean `false`**, without which a site is reported only
    when no other linter claims its line first. Without all three, AC10's "every site" stops being
    checkable. The `uniq-by-line` assertion cannot reuse the predicate the caps share, which accepts
    a numeric zero only
    [measured 3e72070:internal/gateguard/guard_test.go · `grep -n -A12 'func isZeroInt'
    internal/gateguard/guard_test.go` → an `int`/`int64`/`float64` compared against zero, every
    other type returning `false`]: `uniq-by-line`'s safe value is a boolean and its *absence* is the
    unsafe state, because the key defaults to `true` (D3). The guard must therefore demand the key
    be present and `false`, not merely not-`true`
    [derived → the pinned-settings scenarios in § Test Design, subtask 8].

  A guard over the file's *content* does not prove golangci-lint *accepts* that content — those are
  different questions, and the silently-ignored-settings-key row in § Risks is why the difference is
  not academic. `golangci-lint config verify` answers the second one, and § Test Design puts it in
  subtasks 1 and 5 rather than in this guard, which has no golangci-lint binary to call.

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

- **D13 — The panic-recovery rule lands in this task even though its one named site does not yet
  comply, and the allow list records that site's real answer rather than an aspirational one.** Scope
  item 6 has this task write down that a goroutine running an interface-supplied handler recovers a
  panic at that boundary; the scheduler's handler goroutine does not, and the spec puts that site in
  #81 and out of scope. Put to the owner on 2026-09-12 as § Open questions' "The rule that arrives
  ahead of its one exception", and confirmed in their words: *"Land now. The rule goes into
  code-style.md now; the allow list records the scheduler's launch honestly as today's non-compliant
  answer. Matches the issue body's own line: \"The scheduler's application of this rule is #81.\""*
  The consequence for subtask 8 is concrete: that launch's row answers "where its panic goes" with
  what is true today, so the gap is visible in the one file whose entire purpose is reviewing
  launches, rather than papered over by a row written as though #81 had already landed.

### Rejected alternatives

- **A `gocritic` `ruleguard` rule for "`time.After` inside a loop".** It would give AC3 its exact
  shape instead of D4's widening, at the price of a rule file in a directory the go tool skips, an
  experimental checker enabled by name, and a module that is not a dependency today
  [measured e66beb5:go.mod,go.sum · `grep -n "quasilyte\|go-ruleguard" go.mod go.sum` → no output].
  D4's widening plus a reasoned `//nolint` is the cheaper shape for a module whose only gated-file
  instance of the identifier is being removed anyway. Put to the owner on 2026-09-12 and rejected in
  their words — *"Zero new dependencies, one linter pattern"* (D4).

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
| 1 | Enable `containedctx` and `fatcontext`; pin `run.relative-path-mode: gomod` (D2); in a top-level `issues:` section, lift the issue-truncation caps **and** switch the cross-linter line dedup off with `uniq-by-line: false` — every default that suppresses a site the gate would otherwise report, closed together (D3). Both linters are silent on today's tree, so the gate stays green with no source change [measured e66beb5 · `golangci-lint run --enable-only=containedctx ./...` → `0 issues.`; the same for `fatcontext`]. `golangci-lint config verify` runs before the green run is believed (§ Risks, the silently-ignored-settings-key row). Serves AC6, AC7. | `.golangci.yml` | — |
| 2 | Enable `gocritic`'s `deferInLoop` and `govet`'s `nilness`. `deferInLoop` reports nothing on today's tree and `nilness` reports the deliberate typed-nil-in-interface assertion in `internal/store` and nothing else — pinned to a run with the line dedup off as well as the caps, because a run made under the dedup could not have seen a `deferInLoop` whose line another linter had already claimed (D3) [measured e66beb5 — the base tree, read in a detached worktree — under the gate configuration at 52ef68e with the caps lifted and `issues: {uniq-by-line: false}` · `golangci-lint run -c <that file> ./...` → `internal/store/basis_test.go:73:11: nilness: impossible condition: non-nil == nil (govet)`, the `forbidigo` sites D6 enumerates plus the `time.After` subtask 3 removes, and no `gocritic` line; the checker is live in that configuration rather than merely quiet — the same configuration over a constructed package whose loop body defers a file close reports `deferInLoop: Possible resource leak, 'defer' is called in the 'for' loop (gocritic)` beside `errcheck`'s finding on that line, and drops it again with `uniq-by-line` back at its default]; that assertion's existing directive gains `govet` beside `staticcheck` and states the added linter's reason. Serves AC8, AC9, AC10. | `.golangci.yml`, `internal/store/basis_test.go` | 1 |
| 3 | Replace the ingest retry loop's unstoppable timer with a wait-or-cancel helper holding a stopped timer (D5). Test first. Serves AC3, AC10. | `internal/ingest/attempt.go`, `internal/ingest/retry_test.go` | — |
| 4 | Bound the scheduler's detached connection close with its own named-constant timeout (D7). Serves AC5. | `internal/scheduler/execute.go` | — |
| 5 | Enable `forbidigo` with the patterns under `linters.settings.forbidigo.forbid`, and the two carve-outs each scoped `linters: [forbidigo]` — a new `^cmd/` rule, and `forbidigo` appended to the existing `_test\.go` rule's list (D1, D2, D4). Give every fresh-root-context site the gate reports its `//nolint:forbidigo` carrying a stated reason and a named owner (D6), annotating the re-run's own set rather than D6's prose. `golangci-lint config verify` runs before the green run is believed. The gate must be green at this subtask's commit, so the enabling and the annotations land together. Serves AC4, AC5, AC10, AC14. | `.golangci.yml`, `internal/health/canary.go`, `internal/health/process.go`, `internal/health/server.go`, `internal/scheduler/execute.go`, `internal/testdb/testdb.go`, `internal/tgtest/tgtest.go` | 2, 3, 4 |
| 6 | Move the compiled-directory predicate into `internal/srcguard` with its own table test, and fold the leak-detection guard's private copy into it (D9). Serves AC1 (as the guard's scope rule). | `internal/srcguard/srcguard.go`, `internal/srcguard/srcguard_test.go`, `internal/leaktest/guard_test.go` | — |
| 7 | Give the composition root its own HTTP client, thread it into the Telegram client and the canary legs, and append the closer that releases its idle connections (D12). Test first. Serves AC15. | `cmd/bot/assemble.go`, `cmd/bot/assemble_test.go`, `cmd/bot/serve_test.go` | — |
| 8 | Add `internal/gateguard`: the launch allow list and its checker (D8, D9), the lint-configuration guard — exclusion reach, exclusion scoping, the enabled set and the pinned settings (D10) — and a discriminating twin for each (D11). The allow list is written against the tree subtasks 1–7 leave, and the scheduler's launch row answers "where its panic goes" with what is true today (D13). Serves AC1, AC2, AC14. | `internal/gateguard/doc.go`, `internal/gateguard/guard_test.go`, `internal/gateguard/main_test.go` | 5, 6, 7 |
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

- **A gate that suppresses findings by default turns "every site" into "the ones nothing else hid",
  and two separate defaults did that here.** *Truncation* was the first: the first candidate run
  reported only part of the fresh-context class and looked complete
  [measured e66beb5 · the candidate configuration run with golangci-lint's defaults → the
  `internal/health` sites and nothing of the scheduler or the test-helper packages; the same run
  with `--max-same-issues 0 --max-issues-per-linter 0` → those sites plus
  `internal/scheduler/execute.go`, `internal/testdb/testdb.go` at `Main`, at `Schema` and at
  `Schema`'s cleanup dropper, and `internal/tgtest/tgtest.go`].
  *Cross-linter line dedup* was the second, and it survived the cap lift because it is a different
  mechanism: `issues.uniq-by-line` defaults to `true` and drops a site's finding whenever another
  linter has already claimed that line — for AC8's checker the colliding shape is the canonical one
  [measured 3e72070 · a scratch module whose loop body defers a file close, run under this
  repository's configuration → `errcheck`'s finding at that line and no `gocritic` finding; under
  `--enable-only=gocritic` → the `deferInLoop` finding (D3)].
  *Mitigation:* subtask 1 switches off both — the caps and the dedup — before any site work begins
  (D3); subtask 5 re-runs the gate after its last annotation rather than trusting the list this
  design names; and D10's guard asserts each of those keys from the parsed configuration, so a later
  edit that restores any default has a repository-resident regression to break. That the class kept
  a second member after the first was closed is itself the lesson: the mitigation is written against
  *"a default that suppresses a report"*, not against the two keys that have been found.

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
  visible where a reviewer looks. Settled by the owner on 2026-09-12 — D13.

- **Subtask 4 edits a site #81 is slated to rework.** *Mitigation:* the change is one bounded context
  around one close, with no change to the watchdog's structure or to what it waits on. Settled by the
  owner on 2026-09-12 — D7's closing paragraph.

- **An unrecognised key under `linters.settings` is accepted silently by `golangci-lint run`, so a
  mistyped settings block reads as a clean gate.** Measured on this toolchain: the candidate
  configuration written with `linters.settings.forbidigo.patterns` instead of `forbid` runs to
  completion, reports not one `forbidigo` finding over the whole module, prints no warning, and exits
  on the unrelated `govet` finding alone; `golangci-lint config verify` on the identical file rejects
  it, naming the key [measured 20cf1aa · the candidate configuration under both spellings →
  with `patterns`, `1 issues: * govet: 1` and no diagnostic about the configuration; `config verify`
  → ``jsonschema: "linters.settings.forbidigo" … additional properties 'patterns' not allowed``;
  with `forbid`, the set D6 enumerates]. This is the green-instrument shape
  exactly: a gate that cannot fire and a gate with nothing to find are indistinguishable from the
  exit status. *Mitigation, two halves because the two questions differ:* subtasks 1 and 5 run
  `golangci-lint config verify` before believing any green run and see a constructed violation
  reported for every pattern and checker enabled (§ Test Design), and D10's guard asserts the
  enabled set and the pinned settings from the parsed file so the next edit to `.golangci.yml` has a
  repository-resident regression to break.

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
- The carve-outs are checked for **scope**, not only for reach: with the `^cmd/` rule in place, a
  constructed `containedctx` / `fatcontext` / `deferInLoop` / `nilness` shape inside a `main` package
  must still be reported, because those criteria carry no `main` carve-out (D1)
  [derived → AC6, AC7, AC8, AC9].
- `golangci-lint config verify` runs on `.golangci.yml` before any green run of the gate is believed,
  in both subtasks. A green `golangci-lint run` is evidence about the *tree* only once the
  configuration is known to have loaded as written; an unrecognised settings key is accepted in
  silence (§ Risks, the silently-ignored-settings-key row) [derived → AC10].
- The line-dedup switch gets a control of its own, because a key that changes *which* findings
  survive cannot be checked against a green tree: a constructed package whose `defer` inside a loop
  shares its line with another linter's finding must be seen reported for `deferInLoop` under the
  configuration as shipped, and seen *not* reported when `uniq-by-line` is put back to its default.
  The repository's own tree discriminates nothing here — it is `0 issues.` either way (D3)
  [derived → AC8, AC10]. **That control runs in subtask 5, not in subtask 1.** Subtask 1 sets the
  key, but `gocritic`'s `enabled-checks` does not carry `deferInLoop` until subtask 2, so at subtask
  1's commit the control has no checker to discriminate with and would come back green whatever the
  key said — a green instrument rather than evidence. Subtask 5 is where both halves are present and
  where the gate is re-run anyway [derived → the dependency order in § Decomposition, 5 → 2 → 1].
- The `//nolint:forbidigo` escape must be seen both to suppress the finding and to be refused when it
  carries no specific linter or no explanation, which `nolintlint`'s existing settings already require
  [derived → AC5].
- Subtask 5 annotates the sites its **own** re-run reports rather than the list D6 names: `nolintlint`
  runs with `allow-unused` unset, so a directive over a site the gate does not report fails the gate
  in its own right (D6) [derived → AC5, AC10].

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
- Entry points: the launch checker over a tree root, and the lint-configuration checker over a lint
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
  - a scratch package whose `go` statement has **no** enclosing `FuncDecl` — a function literal in a
    package-level `var` initialiser — fails, and the failure names the file and the statement's
    position rather than passing over it [derived → D8's no-enclosing-declaration clause];
  - a `go` statement under `testdata` or under a `.`/`_`-prefixed directory is not gated, and one in a
    non-test file of a package only tests import **is** [derived → AC14];
  - a `go` statement in a `_test.go` file is not gated [derived → D9's test carve-out];
  - the joined forms are not `go` statements and are not gated [derived → D8].
- Scenarios, lint-configuration checker:
  - the repository's own lint configuration passes every assertion [derived → AC14, D10];
  - a scratch configuration whose exclusion rule matches a non-test path under `internal/` fails, and
    the failure names the rule [derived → AC14];
  - a scratch configuration excluding only `_test.go` and `^cmd/`, each with its `linters` list,
    passes [derived → D1];
  - a scratch configuration whose exclusion rule carries a `path` and **no** `linters` list fails,
    and the failure names the rule [derived → D1's scoping clause];
  - a scratch configuration missing one enabled linter, one `gocritic` check, one `govet` analyzer or
    one `forbidigo` pattern fails, and the failure names what is missing — driven once per assertion
    so no single omission is the only one exercised [derived → D10's enabled-set assertion];
  - a scratch configuration whose `run.relative-path-mode` is absent or not `gomod`, and one whose
    issue-truncation caps are absent or non-zero, each fail [derived → D10's pinned-settings
    assertion];
  - a scratch configuration whose `issues.uniq-by-line` is `true` fails, **and so does one where the
    key is absent entirely** — the absent case is the one that matters, since the key defaults to
    `true`, and a guard that only rejected an explicit `true` would pass the very configuration the
    amendment exists to prevent (D3, D10). Driven as its own case rather than folded into the caps'
    table, because the predicate is a different one: present-and-boolean-`false`, not zero-valued
    [derived → D10's pinned-settings assertion].
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

**None open.** The four this design raised were put to the owner on 2026-09-12 and all four confirmed
the design as drafted. Each answer is recorded in the owner's own words at the decision it settles,
so nothing below asks what has been answered:

| Question, as it was asked | Owner's answer, 2026-09-12 | Recorded at |
|---|---|---|
| D4's widening — forbid `time.After` outright, or build the precise loop gate? | "Confirm widening" | D4's closing paragraph, and the `ruleguard` entry in § Rejected alternatives |
| D3's reach — lift the issue-truncation caps for every linter, or strike? | "Confirm lift" | D3's closing paragraph |
| Subtask 4 against #81 — bound the detached close here, or defer it? | "Land it here" | D7's closing paragraph |
| The rule that arrives ahead of its one exception — land it now, or wait for #81? | "Land now" | D13 |

None of these four became a spec row: the spec states what counts as solved and these settle how, so
they live here and in the Key decisions they bind, per `design-writer`'s § Rules → *The spec states
what*.

A fifth ruling binds this design without appearing in that table, because the design did not raise
it: the orchestrator's Step 9 per-AC sweep found the cross-linter line dedup, put the fix to the
owner as a scope deviation, and the owner ruled *"Add + re-review"* on 2026-09-12. It is recorded in
their words at D3, alongside the cap lift it extends, and it likewise became no spec row. Listed
here so a reader auditing which owner rulings this design rests on finds every one of them from this
section.
