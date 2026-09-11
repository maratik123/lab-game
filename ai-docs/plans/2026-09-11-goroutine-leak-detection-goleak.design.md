# Design: Goroutine-leak detection in the test suite — every package's tests end with a leak check

**Issue:** #74
**Spec:** `ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md`
**Branch:** `feat/2026-09-11-goroutine-leak-detection-goleak`
**Date:** 2026-09-11
**Round:** 2

**Tag forms used below.** A fact about something that already exists carries
`[measured <pin>:<path>[:<lines>] · <command> → <output>]`. `<pin>` is a commit of this
repository — `94e6de6` for round 1's measurements, `779b08b` for those round 2 added (the tree's
Go sources are identical at both: the commits between them touch `ai-docs/plans/` only
`[measured 779b08b · git diff --name-only 94e6de6 779b08b → the design, the spec and its state file, all under ai-docs/plans/]`); for a
dependency it is the module version (`goleak@v1.3.0`, paths relative to that
module's root in the module cache); for the standard library and the go command it is
`go1.26.5` (paths relative to `$(go env GOROOT)`). A behaviour of an existing tool measured by
running a scratch fixture against it names the tool as its pin and states the recipe. The probe
of today's tree (§ *What the tree leaves running today*) is written `94e6de6 + probe`: the tree at
that commit with only the probe `TestMain`s added, production code untouched. A claim about an
artefact this task creates carries `[derived → <AC or test>]` and no coordinate.

## Approach

### What the tree leaves running today

Measured rather than assumed, because the owner left the task's size open until the detector
has run everywhere
`[measured 94e6de6:ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md.state.md:74 · sed -n 74p → "The size stays open until the design has run the detector everywhere"]`.
The probe: a scratch clone of `94e6de6` in which every package with tests gets a probe `TestMain`
that wraps the package's existing entry point in `goleak.VerifyTestMain` with **no** ignore option
and records a verdict per package. It was run on the route `make test` takes, with `-count=1` so
that no cached result answers (`go run ./cmd/testpg -- go test -count=1 ./...`), and on the
per-binary fallback route exactly as `make test-fallback` runs it
(`LAB_GAME_TEST_DSN= go test -count=1 ./...`)
`[measured 94e6de6:Makefile:65,88 · sed -n → "go run ./cmd/testpg -- go test ./...", "LAB_GAME_TEST_DSN= go test -count=1 ./..."]`.

- **Shared route:** `internal/tg`, `internal/ingest` and `internal/health` report leaks; every
  other package is clean. Every reported goroutine is the same thing — a handler built by the fake
  Bot API server's `Delayed` helper, parked in `time.Sleep` under `tgtest.(*Server).serveHTTP`,
  created by `net/http.(*Server).Serve` — from `TestCaller_AttemptTimeoutAbandonsAttempt`,
  `TestPollOnce_StopDiscardsAParkedLongPoll`,
  `TestPollOnce_cancellationDuringLongPollReturnsPromptly` and `TestTelegramProber_Timeout`
  `[measured 94e6de6 + probe · go run ./cmd/testpg -- go test -count=1 ./... → "goleak: Errors on successful test run" for internal/health, internal/ingest and internal/tg only; each stack "in state sleep, with time.Sleep on top of the stack", through "tgtest.(*Server).serveHTTP", "created by net/http.(*Server).Serve"]`.
- **Fallback route:** only `internal/tg` reports it on that run
  `[measured 94e6de6 + probe · LAB_GAME_TEST_DSN= go test -count=1 ./... → goleak failure for internal/tg only]`.
  The class is timing-dependent: a handler whose client gave up before it started never parks,
  and one whose delay ran out during the container teardown that precedes the check has already
  exited — the tests set client deadlines far shorter than the handlers' delays, which run to
  seconds in `internal/ingest` and `internal/tg` and to an hour in `internal/health`
  `[measured 94e6de6:internal/health/probe_test.go:161-164, internal/ingest/loop_stop_test.go:143, internal/ingest/retry_test.go:513, internal/tg/retry_test.go:379 · sed -n → "tgtest.Delayed(time.Hour, …)" with "context.WithTimeout(context.Background(), 10*time.Millisecond)"; "tgtest.Delayed(5*time.Second, …)" twice; "tgtest.Delayed(2*time.Second, …)"]`.
  That is why it is fixed at its cause (D8) rather than observed.
- **Nothing else, on either route:** no database, container-runtime, metrics-client or HTTP
  keep-alive goroutine is reported — including in the database-backed packages on the fallback
  route, where the check ran after the provisioned container had been stopped (the same measured
  runs as above).

The cause is in the test helper, not in production code: `Delayed` sleeps its whole duration
without regard to the request
`[measured 94e6de6:internal/tgtest/tgtest.go:238-243 · sed -n '238,243p' → "time.Sleep(after)", "next(w, r)"]`,
and the server's cleanup closes the server without cancelling a handler in flight
`[measured 94e6de6:internal/tgtest/tgtest.go:84-87 · sed -n '84,87p' → "_ = s.httpSrv.Close()", "_ = s.listener.Close()"]`.

**Consequence for the ignore set: it lands empty.** The one leak class found is fixed, and
nothing else is reported on either route `[derived → T4's gate: make test, make test-race and make test-fallback green with no ignore entry declared anywhere]`.
The spec's examples of what the set might hold — goroutines of the pgx pool, the container
runtime, the Prometheus client and HTTP keep-alive connections (Scope 5) — are not present at the
moment the check runs (the measured runs above). The mechanism for entries (AC3, AC5, AC6) ships
all the same, held by discriminating tests instead of by a live entry.

### The shape

Every package with tests declares its `TestMain` in one form:

```go
func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, testdb.Main))      // a database-backed package
}

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, (*testing.M).Run)) // every other package
}
```

`leaktest.Main` hands `m` to the runner it is given — whatever owned the entry point before this
task — and, when the runner reports that the tests passed, checks that no goroutine outside the
package's ignore set is still running. A leak, a malformed ignore entry, or an ignore entry that is
not needed makes it return non-zero, so the test binary fails and `go test` reports the package as
`FAIL` on every route that runs tests `[derived → AC1, AC6]`. A guard in `internal/leaktest`'s own
tests walks the module and fails the suite when a package with tests declares no such `TestMain`
`[derived → AC7]`.

### Key decisions

**D1 — One shared package, `internal/leaktest`, over goleak.** Every package with tests is a call
site `[measured 94e6de6 · go list -f '{{if not (or .TestGoFiles .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./... → empty output: every package has test files]`,
far past the three-site threshold at which `design-writer.md` requires a shared `internal/`
package over per-site copies. The detector is `go.uber.org/goleak`, already a direct requirement
`[measured 94e6de6:go.mod:17 · grep -n goleak go.mod → "go.uber.org/goleak v1.3.0"]` whose reason
#24's design recorded (spec § Out of scope); hand-rolling a goroutine-stack scan instead is refused
by `AGENTS.md` § Dependency Versions
`[measured 94e6de6:AGENTS.md:202 · sed -n 202p → "AXIOM — Established Go packages and the standard library first. Hand-rolling is a decision that must be ARGUED"]`. The package is test support in the family `internal/testdb`,
`internal/tgtest` and `internal/repotest` belong to, and takes that family's `…test` suffix; its
importers are test files only `[derived → T4's file list: every file that gains the import is a _test.go file]`.

What `internal/leaktest` adds that goleak's public API does not have: goleak's `Option` carries
only an unexported method, so no option written outside goleak can observe which entry matched which
goroutine `[measured goleak@v1.3.0:options.go:31-33 · sed -n '31,33p' → "type Option interface {", "apply(*opts)"]`,
and its exported surface is `VerifyTestMain`, `VerifyNone`, `Find`, `IgnoreTopFunction`,
`IgnoreAnyFunction`, `IgnoreCurrent` and `Cleanup`
`[measured goleak@v1.3.0 · grep -n '^func [A-Z]' options.go leaks.go testmain.go → exactly those]`.
`VerifyTestMain` also calls `os.Exit` itself unless handed a `Cleanup`
`[measured goleak@v1.3.0:testmain.go:52-68 · sed -n '52,68p' → "exitCode := m.Run()" … "cleanup = _osExit"]`.
The per-entry reason (AC3) and the stale check (AC6) have to be written over that API in any case;
this package is where that code lives, once.

**D2 — Composition: the detector wraps whatever owned the entry point, and runs after it
returns.** `Main(m *testing.M, run func(*testing.M) int, ignore ...Ignore) int`. The runner is
named at every site, so the composition the spec requires to be made rather than discovered
(Scope 4) is written where a reader meets it. Both runners in use already have that type:
`testdb.Main` `[measured 94e6de6:internal/testdb/testdb.go:45 · sed -n 45p → "func Main(m *testing.M) int {"]`
and the method expression `(*testing.M).Run`. The order is the point: the check runs after the
runner has returned, i.e. after `testdb.Main` has stopped the container it started on the fallback
route `[measured 94e6de6:internal/testdb/testdb.go:63-67 · sed -n '63,67p' → "code := m.Run()", "if err := server.Stop(ctx); err != nil {"]`,
so it looks at the process after everything the entry point took has been given back.
`testdb.Main`'s behaviour does not change `[derived → T4 edits only its doc comment]`.

**D3 — When, and how long it waits.** Once per test binary, after all of its tests, and only when
the runner returned zero: a failing run already fails the package and its leftovers are noise —
goleak's own `VerifyTestMain` makes the same choice
`[measured goleak@v1.3.0:testmain.go:63 · sed -n 63p → "if exitCode == 0 {"]`. The only grace is
goleak's own bounded retry inside `Find`
`[measured goleak@v1.3.0:options.go:38,125,148-158 · sed -n → "const _defaultRetries = 20", "maxSleep:   100 * time.Millisecond", "d := time.Duration(int(time.Microsecond) << uint(i))"]`,
which v1.3.0 exposes no option to change (its `maxSleep` is unexported — same measurement as D1's
exported-surface list). `internal/leaktest` adds no patience window of its own. A wider window
lets a goroutine that outlives its test by seconds pass as clean, and the probe shows exactly that:
`internal/ingest`'s `Delayed` handlers, reported on the shared route, were not reported on the
fallback route, where the container teardown ran between them and the check
`[measured 94e6de6 + probe · the runs of § What the tree leaves running today → internal/ingest reported on the shared route, clean on the fallback route]`.
AC1 asks for the goroutine that is still running when the tests have finished; the answer to a slow
exit is an owner that ends its goroutine at cleanup, never a longer wait.

**D4 — What a detection reports.** goleak's `Find` error, which carries every reported
goroutine's state, full stack and creator line
`[measured goleak@v1.3.0:internal/stack/stacks.go:77-80 · sed -n '77,80p' → "Goroutine %v in state %v, with %v on top of the stack:\n%s"]`
— the probe's reports above each end in `created by …` — written to standard error beneath one
`leaktest:` line saying that goroutines were still running after the package's tests finished
`[derived → AC2; T2's leak case asserts the report names each leaking goroutine's function and its creator]`.
`go test` prints a failing package binary's output — the probe's goleak reports reached its gate
log that way (§ *What the tree leaves running today*) — so the stack is in the gate log with nothing
further to run.

**D5 — The ignore set is declared at the `TestMain` of the package it applies to.** An entry is
`leaktest.Ignore{Function, Anywhere, Reason}`: the fully qualified function goleak matches; whether
it must be the top frame or may be any frame — goleak's two matchers
`[measured goleak@v1.3.0:options.go:64-87 · sed -n '64,87p' → "func IgnoreTopFunction(f string) Option", "func IgnoreAnyFunction(f string) Option"]`;
and why the goroutine it matches is not a leak. Its scope is structural: the entries a `TestMain`
passes apply to that package's test binary, on every route, and are deleted with the package.

Before it calls the runner, `Main` refuses — non-zero, tests not run, every refusal named — an
entry whose reason is blank `[derived → AC3]`, whose function is blank, or whose function is this
module's own code: a name beginning with the module path followed by `/` or `.`
`[derived → AC5]`. That one prefix covers the commands too: inside a test binary a `package main`
function prints under its import path, not under `main`
`[measured go1.26.5 · scratch module, a package-main test leaving a goroutine in a main.go function → goleak reports "example.com/gl/cmdx.park", "created by example.com/gl/cmdx.TestMainPackageFrames"]`.
The module path comes from the test binary's build information, never from a literal
`[measured go1.26.5 · debug.ReadBuildInfo() inside a scratch test binary → Path "<import path>.test", Main.Path "<main module path>"]`,
through one small source function T2 asserts against the `module` line of `go.mod`
`[derived → T2's module-path case]`; a binary whose build information cannot name it is refused
the same way, because a check that cannot tell what this module's code is has not run. A nil
runner is refused likewise `[derived → T2's refusal cases]`.

`Main` itself is one statement: it hands the check the tests to run (its runner applied to `m`),
`os.Stderr`, that module-path source, D6's filter predicate bound to `flag.Lookup`, and the entries.
Everything it wires in is a function T2 drives directly; the wiring is what review reads and what
the Step-9 AC6 probe drives end to end `[derived → T2; the AC6 probe]`.

**Admission** — judged in review, and stated by `Ignore`'s doc comment: an entry is for a goroutine
that neither this module's code nor its test fixtures can stop, one a dependency starts for the life
of the process. A keep-alive connection a test opened, a pool a test did not close, a handler still
running — each has an owner that can end it, and ending it is the fix.

**D6 — Keeping the set honest: each entry is evaluated by leaving it out.** After a clean check
with the whole set, `Main` calls goleak's `Find` once more per entry, with that entry removed.

- **Nothing reported** → the entry matched no goroutine that another entry does not also match. It
  fails the suite, named, as not needed — stale after a dependency move, or redundant beside another
  entry `[derived → AC6]`.
- **Something reported** → the report lists exactly the goroutines only that entry was excusing. If
  any of their frames or creators is this module's code (D5's name predicate, applied to each frame
  and `created by` line of that report), the entry is excusing a leak: it fails the suite, with those
  stacks `[derived → AC5]`.

goleak stays the only matcher, so "matches" means what goleak means. Measured with two tests in a
scratch module: leaving out a used entry returns an error listing that goroutine's stack after
goleak's full retry, leaving out an unused one returns nil at once, and a stdlib-only goroutine and a
goroutine running module code are told apart by the frames in that error
`[measured goleak@v1.3.0 · scratch test (a): two test goroutines each excused by an IgnoreAnyFunction entry on its own function, plus an IgnoreTopFunction entry on a function on no stack; Find with all → nil; Find without either used entry → error carrying that goroutine's stack, after the full retry; Find without the unused entry → nil at once. Scratch test (b): IgnoreAnyFunction entries on "net/http.(*Server).Serve" (an httptest.Server's serve goroutine) and on "sync.(*WaitGroup).Wait" (a test goroutine blocked in Wait); Find with both → nil; without the Serve entry → a stack of stdlib frames only, "created by net/http/httptest.(*Server).goServe"; without the Wait entry → a stack carrying the test function's own frame and creator]`.

The two halves of "this module's code" — a frame, and the `created by` line — can each appear
without the other, and T2 isolates both:
`[measured go1.26.5 + goleak@v1.3.0 · scratch: a time.AfterFunc callback in a test file, blocked in sync.(*WaitGroup).Wait → frames "sync.(*WaitGroup).Wait", "example.com/gl/inner.TestFrameOnly.func1()", creator "created by time.goFunc"; "go srv.Serve(l)" in a named function of a test file → frames from internal/poll up to "net/http.(*Server).Serve" only, creator "created by example.com/gl/inner.startServeDirect"]`.

The per-entry evaluation runs only on an **unfiltered** run. Under `-run`, `-skip`, `-list` or
`-short` a package's tests may never start the goroutine an entry exists for, so the evaluation is
skipped with one line saying so; the leak check itself still runs. No gate filters: the test
invocations in `make test`, `make test-race`, `make test-fallback` and `make test-contention`
`[measured 94e6de6:Makefile:65,68,88,138,141 · grep -n 'go test' Makefile → "go run ./cmd/testpg -- go test ./...", "go run ./cmd/testpg -- go test -race ./...", "LAB_GAME_TEST_DSN= go test -count=1 ./...", "go test -count=1 -parallel $(CONTENTION_PARALLEL) ./internal/ingest/... …", "go test -race -count=1 -parallel $(CONTENTION_PARALLEL) ./..."]`
and the coverage ratchet's
`[measured 94e6de6:.githooks/coverage-ratchet.sh:115 · sed -n 115p → "go run ./cmd/testpg -- go test -covermode=atomic -coverprofile=\"$PROFILE\" ./..."]`
carry none of those flags. The flags are read after the runner returns, because `testing` parses
them inside `M.Run` `[measured go1.26.5:src/testing/testing.go:2352 · sed -n 2352p → "if !flag.Parsed() {"]`.
The predicate reads them through a lookup it is handed — `flag.Lookup` from `Main` — so T2 drives
the real predicate with a fake lookup. "Filtered" means `test.run`, `test.skip` or `test.list` is
non-empty, or `test.short` is true; a flag the lookup does not find counts as unset, so a lookup
that finds nothing leaves the evaluation on rather than silently off `[derived → T2's filter cases]`.

**D7 — Coverage of the module is enforced by a guard in `internal/leaktest`'s own tests.**
`TestGuard_EveryPackageWithTestsRunsTheDetection` walks the module through `internal/srcguard` and
collects every directory holding a `_test.go` file, under the go tool's own rule for what belongs to
a package `[measured go1.26.5 · go help packages → "Directory and file names that begin with \".\" or \"_\" are ignored by the go tool, as are directories named \"testdata\"."]`.
Of each it requires exactly one `TestMain` across its test files, whose body is the single
statement `os.Exit(leaktest.Main(<its own parameter>, <runner>, <entries…>))`, with `os` and
`leaktest` resolved through the file's own imports. A `TestMain` that calls the detector and
discards the result, calls something else, or takes a second statement fails it. Every offending
directory is named, with the declaration it needs `[derived → AC7]`. The predicate lives beside the
proposition it enforces, as `ai-docs/go-test-conventions.md` § Structural guards requires
`[measured 94e6de6:ai-docs/go-test-conventions.md:67-68 · sed -n → "Every predicate stays in the package that owns the proposition"]`.

**The walk is in-process, and that is load-bearing.** `go test` replays a cached pass unless an
input the test itself opened has changed; a directory the test listed counts as such an input, and
files an exec'd child reads do not. Measured with two scratch guards in a clone — one walking the
module with `filepath.WalkDir`, one exec'ing `go list ./...` — after a new package with a test was
added, the walking guard re-ran and failed naming it while the exec'ing guard answered `(cached)`
`[measured go1.26.5 · go test twice, add internal/zznewpkg/x_test.go, go test again → walk guard: "(cached)" on the repeat, then "--- FAIL … found ../../internal/zznewpkg"; exec guard: "ok … (cached)" after the new package, and FAIL only under -count=1]`.
A guard that enumerated packages through `go list` would let a package added after this task pass
on a cached run. `srcguard.WalkSubtree` walks with `filepath.WalkDir` in the test's own process
`[measured 94e6de6:internal/srcguard/srcguard.go:53-76 · sed -n '53,76p' → "err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {"]`.

**D8 — The one leak class found at adoption: the fake Bot API server's in-flight handlers.**
`tgtest.New` gives its `http.Server` a base context that the test's cleanup cancels, and `Delayed`
waits for its duration or for the request's context to end, whichever comes first, returning
without calling the wrapped handler in the second case `[derived → T1's cleanup case]`. The base
context is the lever, not the client's disconnect. For a request whose body is unread, `net/http`
starts watching the connection for its peer closing only once that body has been consumed; for a
body-less request it watches at once
`[measured go1.26.5:src/net/http/server.go:2059-2063 · sed -n '2059,2063p' → "if requestBodyRemains(req.Body) {", "registerOnHitEOF(req.Body, w.conn.r.startBackgroundRead)", "} else {", "w.conn.r.startBackgroundRead()"]`.
Every consumer of the fake server sends a body — a JSON body built for each call, `null` for a
call with no parameters, posted by the transport's caller
`[measured 779b08b:internal/tg/constructor.go:22-28, internal/tg/caller.go:189-197 · sed -n → "body, err := json.Marshal(parameters)", "BodyRaw: body"; "body = bytes.NewReader(data.BodyRaw)", "http.NewRequestWithContext(attemptCtx, http.MethodPost, rawURL, body)"]`
— and a `Delayed` handler never reads it, so a client giving up never ends the handler's request
context. Measured on a scratch fake server of the same shape: a handler waiting on its request
context was released within microseconds of the client's cancel when the POST had no body, and not
at all when it carried `null`, until the server's base context was cancelled
`[measured go1.26.5 · scratch net.Pipe server, handler selecting on its request context and an hour-long timer, client cancelling after the handler started → no body: released within microseconds of the cancel with or without the base-context cancel; body "null": never released by the cancel, released when the base context was cancelled]`.
`BaseContext` is the server's own seam for a context every request derives from
`[measured go1.26.5:src/net/http/server.go:3049-3055 · sed -n '3049,3055p' → "BaseContext optionally specifies a function that returns the base context for incoming requests on this server."]`.
Neither the base context nor its cancel function becomes a field of `Server`: the `BaseContext`
closure holds the one and the cleanup closure the other, because a context is never stored in a
struct `[measured 94e6de6:ai-docs/code-style.md:35 · sed -n 35p → "Never store a context in a struct."]`.
The cleanup does not join handler goroutines: a handler that ignores its request's context is then
reported by the leak check, with its stack, instead of hanging the binary. The wait stays on
whichever clock is active, so inside a `testing/synctest` bubble a `Delayed` handler still answers
after exactly its delay on the virtual clock, which KD-26 depends on
`[derived → T1's bubble case]`. The doc comments on `New` and `Delayed` state what the code now
does.

**D9 — The `Binaries` manifest recognises a reference to the provisioner's `Main`, not a
substring.** Its walk counts a directory when a test file contains the text `testdb.Main(`
`[measured 94e6de6:internal/testdb/server_test.go:156 · sed -n 156p → "if strings.Contains(string(contents), \"testdb.Main(\") {"]`.
The new `TestMain` form passes `testdb.Main` as a value, which that substring does not match; and
D7's discriminating fixtures carry `TestMain` source as string literals, which any looser substring
would count. `callersOfMain` becomes a parse: a directory counts when one of its test files holds a
selector on its import of `internal/testdb` naming `Main`, called or passed, and a string literal
counts for nothing. It enumerates through `srcguard.WalkSubtree` and parses through
`srcguard.ParseFile`, the standing rule for a walk that is being rewritten anyway
(`ai-docs/go-test-conventions.md` § Structural guards) `[derived → T3's scratch case]`. The
directories it finds are the same before and after the rollout, so `Binaries` does not move and
KD-20's proposition is untouched `[derived → TestBinaries_matchesTree green at T3 and at T4]`.

**D10 — No `Makefile` change and no workflow change.** The detection rides in each package's
`TestMain`, so every invocation that runs a package's tests runs it: CI's Test job
`[measured 94e6de6:.github/workflows/ci.yml:125-137 · sed -n '125,137p' → "run: make test", "run: make test-race", "run: make cover-ratchet", "run: make test-fallback"]`,
and `make verify`'s `test` and `test-race`
`[measured 94e6de6:Makefile:41 · sed -n 41p → "verify: fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck comment-refs import-guard"]`.
The Test job's paths filter already reaches every Go file
`[measured 94e6de6:.github/workflows/ci.yml:40 · sed -n 40p → "- '**/*.go'"]`.

### Rejected alternatives

- **A per-test `defer goleak.VerifyNone(t)`.** goleak documents it as incompatible with
  `t.Parallel`, since it cannot tell one test's goroutines from another's
  `[measured goleak@v1.3.0:leaks.go:86-90 · sed -n '86,90p' → "VerifyNone is currently incompatible with t.Parallel because it cannot associate specific goroutines with specific tests."]`,
  and parallel table tests are this suite's default shape (`ai-docs/go-test-conventions.md` §
  Table-driven subtests are the default). `cmd/bot`'s smoke test keeps its own per-test check — it
  is not parallel and takes a snapshot first
  `[measured 94e6de6:cmd/bot/smoke_test.go:86,181 · sed -n → "leakOpt := goleak.IgnoreCurrent()", "goleak.VerifyNone(t, leakOpt)"]`
  — and stays out of this task (spec § Out of scope).
- **`goleak.VerifyTestMain` written at every site.** It exits the process itself (D1), so it cannot
  sit around `testdb.Main` without an adapter at every site, and it has no per-entry reason and no
  stale check.
- **The check inside `testdb.Main`.** Couples provisioning to leak detection, leaves every other
  package needing a second entry point, and gives the guard two entry points to vouch for.
- **A check inside the runner** — wrapping `m` and handing the wrapper to `testdb.Main`. It would run
  before the provisioner's own teardown, while what the entry point started is still running.
- **A separate `make` target, or a second pass over the suite.** A second run whose conditions are
  not the gate's, plus a CI step and a paths filter to keep it executed.
- **A central ignore table keyed by package import path.** An entry naming a package later renamed
  or deleted is never evaluated, so it never goes stale — dishonest by construction. An entry declared
  at its package's `TestMain` goes with the package.
- **Scoping entries by provisioning route.** No goroutine differs between the routes today apart from
  the D8 class's timing (§ *What the tree leaves running today*). If one ever does, its entry fails as
  not needed on the other route, which is the signal to add a route condition then.
- **A patience window wider than goleak's.** D3.

### Propagation (`AGENTS.md` § Propagation Rule)

The spec no longer carries a propagation row; the standing rule binds this PR on its own, and the
owner kept this section when the row was struck
`[measured 779b08b:ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md.state.md:92 · sed -n 92p → "AGENTS.md § Propagation Rule still binds this PR on its own, and the design keeps its § Propagation (with the retry_test.go site the reviewer found)"]`.

Every comment the diff makes false is rewritten in the step that falsifies it:

- `tgtest.New`'s "so no goroutine outlives the test", and `Delayed`'s contract (T1).
- The doc comment on `TestCaller_AttemptTimeoutAbandonsAttempt` in `internal/tg/retry_test.go`
  (T1). It justifies running in real time by a handler that outlives the bubble: *"a still-sleeping
  handler goroutine trips that "deadlock" check"*
  `[measured 779b08b:internal/tg/retry_test.go:367-376 · sed -n '367,376p' → "This test deliberately runs in real time rather than under synctest: the fake handler's delay outlives the aborted attempt (net/http never interrupts a handler mid-flight just because the client gave up), and synctest requires every bubble goroutine to finish or be durably blocked before the bubble's root returns — a still-sleeping handler goroutine trips that \"deadlock\" check"]`.
  True today, false after D8: a bubble's cleanups run inside it before its root goroutine exits,
  and time stops advancing only when the root exits
  `[measured go1.26.5:src/testing/synctest/synctest.go:43,283-284, src/testing/testing.go:2150-2151 · sed -n → "Time stops advancing when the root goroutine of the bubble exits."; "T.Cleanup functions run inside the bubble, immediately before Test returns."; "go tRunner(t2, f)", "if !<-t2.signal {"]`
  — so the fake server's cleanup, now cancelling its base context, releases the handler before
  the bubble waits for it. The comment keeps its first sentence, what the test covers, and loses the
  real-time rationale; the test itself stays as it is — nothing in this task needs it inside a
  bubble `[derived → T1]`.
- `callersOfMain`'s description (T3).
- `testdb.Main`'s "the exit code the caller's TestMain must pass to os.Exit", which the caller now
  hands to the detector instead (T4).

The suite-level rule is written once in each place agents read it — a section of
`ai-docs/go-test-conventions.md`, one bullet in `AGENTS.md` § Go Test Conventions beside the race
gate, and KD-36 in `ai-docs/key-decisions.md` (T6). The package layout in `ai-docs/context.md` and
the `ai-docs/context-status.md` entry are `/task` Step 9.5's.

Three case-insensitive sweeps — the leak check's own vocabulary and the behaviour D8 changes over the
live surfaces, and the rule's step 4 over the user-facing docs, in the Russian those are written in —
find nothing else this diff contradicts:

- `[measured 94e6de6 · rg -n -i 'goleak|goroutine leak|leak check|leaked goroutine|no goroutine' excluding ai-docs/plans/**, ai-docs/learnings.md, ai-docs/metrics/** → go.mod, go.sum, KD-8, KD-28, internal/ingest/loop.go, cmd/bot/smoke_test.go, a fixture line in ai-docs/scripts/test-spec-anchors.sh, the drain section of ai-docs/process-lifecycle.md ("No goroutine of this module survives the drain."), internal/tgtest/tgtest.go's New comment — each true after this task except the last, false today and rewritten by T1]`;
- `[measured 779b08b · rg -n -i 'Delayed|still-sleeping|still sleeping|mid-flight|outlives the|outlive the|handler goroutine|keeps sleeping|sleeping handler' excluding ai-docs/plans/**, ai-docs/learnings.md, ai-docs/metrics/**, ai-docs/deferred/**; control: the retry_test.go sentence matches → the Delayed call sites and TestServer_Delayed; tgtest.go's Handler, New and Delayed comments; internal/tg/retry_test.go:372-375; and, about other things, the Makefile's test-contention comment, two nolint reasons in cmd/bot/assemble.go, internal/tg/limit.go, cmd/testpg/run.go, internal/scheduler/execute.go's own handler goroutine, a limiter test comment, ai-docs/process-lifecycle.md's drain section and ai-docs/go-test-conventions.md's long-lived-server bullet — the only sites about Delayed's behaviour are tgtest.go's New and Delayed comments and the retry_test.go comment, all three in T1]`;
- `[measured 779b08b · rg -n -i 'горутин|утечк|goroutine|goleak' docs/ README.md → no README.md in the tree; two docs/DESIGN.md lines, one on maps leaking out to wikis and one on a raid session being a database row rather than a process — neither about the test suite; control: "утечка горутин" matches]`.

### What this task does not touch

`testdb.Main`'s provisioning and KD-20's import invariant; `cmd/bot`'s smoke test and its own leak
check; `go.mod` and `go.sum` — goleak is already required with both checksums recorded
`[measured 94e6de6:go.sum:203-204 · grep -n goleak go.sum → "go.uber.org/goleak v1.3.0 h1:…", "go.uber.org/goleak v1.3.0/go.mod h1:…"]`;
the `Makefile`; the workflows; and every package the bot links — `cmd/bot`'s non-test dependency
graph neither gains nor loses a package `[derived → make import-guard green; go list -deps ./cmd/bot unchanged]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | `internal/tgtest` — D8: the base context the cleanup cancels; `Delayed` waiting on its duration or on the request's context; the doc comments on `New` and `Delayed`; with their tests (T1's cleanup case, with both red demonstrations, and bubble case). In the same step, the doc comment on `TestCaller_AttemptTimeoutAbandonsAttempt` loses the real-time rationale D8 falsifies (§ Propagation); that test's body is untouched | `internal/tgtest/tgtest.go`, `internal/tgtest/tgtest_test.go`, `internal/tg/retry_test.go` | — |
| 2 | `internal/leaktest` — package comment, `Main`, `Ignore`, and the check behind them: D5's refusals, D3's detection, D4's report, D6's per-entry evaluation and its filtered-run skip; with the unit tests of § Test Design T2, and the package's own `TestMain` in the D2 form | `internal/leaktest/leaktest.go`, `internal/leaktest/leaktest_test.go`, `internal/leaktest/main_test.go` | — |
| 3 | `internal/testdb` — D9: `callersOfMain` as a parse through `internal/srcguard`, its doc comment, and T3's scratch case | `internal/testdb/server_test.go` | — |
| 4 | Rollout: every package with tests declares `TestMain` in the D2 form — edited in place where it already delegates to `testdb.Main` (runner `testdb.Main`), a new `main_test.go` everywhere else (runner `(*testing.M).Run`; in the external test package where the directory has only external test files); `testdb.Main`'s doc comment; then the whole suite on every route. A leak any route reports that is not the D8 class is a **STOP** back to the orchestrator, not a fix inside this step | `cmd/bot/assemble_test.go`, `internal/ingest/main_test.go`, `internal/scheduler/scheduler_test.go`, `internal/store/store_test.go`, `internal/testdb/testdb_test.go`, `internal/testdb/testdb.go`; new `main_test.go` in `cmd/commentrefs`, `cmd/importguard`, `cmd/testpg`, `internal/backoff`, `internal/commentref`, `internal/config`, `internal/health`, `internal/repotest`, `internal/srcguard`, `internal/tg`, `internal/tgtest` | 1, 2, 3 |
| 5 | The D7 guard: the real-tree case with its positive control, and the discriminating scratch cases of § Test Design T5 | `internal/leaktest/guard_test.go` | 2, 4 |
| 6 | Docs: a *Goroutine-leak detection* section in `ai-docs/go-test-conventions.md` (the `TestMain` form and its two runners, when the check runs and what it reports, the ignore set's shape, admission rule, refusals and per-entry evaluation, the filtered-run skip, the guard and why it walks in-process); one bullet in `AGENTS.md` § Go Test Conventions beside the race gate, pointing there; KD-36 in `ai-docs/key-decisions.md` (the decision, D2's composition, the rejected alternatives above, the consequences) | `ai-docs/go-test-conventions.md`, `AGENTS.md`, `ai-docs/key-decisions.md` | 1, 2, 3, 4, 5 |

**T4's package list is a scoping floor, and the guard is the authority.** It is what `go list`
reports with test files at the pin, split by whether a `TestMain` already exists
`[measured 94e6de6 · go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... and rg -n 'func TestMain' --type go → TestMain in exactly the files T4 edits in place; internal/repotest and internal/srcguard have only external test files (TestGoFiles empty)]`.
A package added between this design and T4 is caught by T5's guard, not by this table.

**T4's per-site preconditions, checked.** Each site's package clause follows where its tests
already live — external for the directories with only external test files, as above. No existing
guard objects to the `os.Exit` a `TestMain` carries: the packages that forbid `os.Exit` scan their
non-test files only
`[measured 94e6de6:internal/ingest/guards_test.go:27-36,47-56, internal/health/guards_test.go:511-520 · sed -n → the ingest guard's walk iterates "ingestNonTestFiles(t)"; the health scan skips a name when "strings.HasSuffix(e.Name(), \"_test.go\")"]`,
and a `TestMain` ending in `os.Exit` is already the form in the files T4 edits in place
`[measured 94e6de6:internal/ingest/main_test.go:15-17 · sed -n '15,17p' → "func TestMain(m *testing.M) {", "os.Exit(testdb.Main(m))"]`.
Every site is leak-free once T1 has landed `[derived → T4's route runs]`; a site that is not is
T4's STOP.

## Handoff plan

- **Entry into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry).
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1, 2, 3, 4, 5 (code change-type: `*.go`). Run in dependency order
  1 → 2 → 3 → 4 → 5, each subtask's commit green on its own: 1, 2 and 3 read nothing the others
  write; 4 needs 1 (the rollout is red on the D8 leak without it), 2 (the detector) and 3 (the
  manifest loses the database-backed packages without it); 5 needs 4 (the guard is red over a tree
  not yet rolled out).
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh), via the `general-purpose` subagent, 1M-token window — subtask 6
  (instructions/harness change-type: `*.md`, `AGENTS.md`, `ai-docs/**`). Terminal group (1 subtask;
  within the `1..=10` range). It follows Group A because the conventions section and KD-36 describe
  the shipped shape by symbol. `AGENTS.md` is one of the files the instruction-edit hook guards;
  Step 8's in-flight marker is what admits the edit
  `[measured 94e6de6:AGENTS.md:373 · sed -n 373p → "Steps 8–12 carry the marker and are exempt"]`.

Two groups — one per change-type, the fewest homogeneity allows, within the default maximum of four.

## Risks

- **A goroutine still exiting when goleak's retry runs out reads as a leak.** The likeliest place
  is induced load (`make test-contention`). Mitigation: a fixture ends what it starts at cleanup
  instead of relying on the window — D8 does that for the fake server — and no window is widened
  (D3). The window is goleak's own constant, with no option to change it in v1.3.0 (D1, D3)
  `[derived → T1's cleanup case; T4's runs of every route]`.
- **The test cache replays the guard's pass after a package is added.** Mitigation: the in-process
  walk D7 measured `[derived → the AC7 verification probe below, run against a cached result]`.
- **Leaving an entry out has a price and a precise meaning.** Each entry in use costs one full
  goleak retry in that binary's check, and two entries matching the same goroutine are both
  reported as not needed (D6's measurement). No entry exists at landing
  `[derived → T4's gate]`.
- **D6's module-code scan reads goleak's formatted error text**
  `[measured goleak@v1.3.0:internal/stack/stacks.go:77-80 · the format string above]`.
  Mitigation: the version is pinned (D1); T2's two module-excuse cases — one per half, frame and
  creator — go red if an upgrade changes the format so that the scan no longer finds either
  `[derived → T2's "running" and "started by" cases]`.
- **An entry needed on one provisioning route only** fails as not needed on the other. None exists
  today (§ *What the tree leaves running today*); the route condition is added when one does
  (§ Rejected alternatives).
- **The `Binaries` manifest stops seeing the database-backed packages under the new form**
  `[measured 94e6de6:internal/testdb/server_test.go:156 · the substring predicate above]`.
  Mitigation: D9, landed before the rollout `[derived → TestBinaries_matchesTree green at T3 and T4]`.
- **The repository-root-resolver guard rejects any function, test files included, that pairs
  `runtime.Caller` with a path ascent**
  `[measured 94e6de6 + probe:internal/health/guards_test.go:578-614,651 · a probe helper naming its package via runtime.Caller and filepath.Dir → TestGuard_SingleRepoRootResolver_RealTree: "file-location-ascent resolver outside internal/repotest"]`.
  Mitigation: the detector needs no path — the module path comes from build information (D5) — and
  the guard gets the root from `repotest.Root` `[derived → T5 green beside the resolver guard]`.
- **Coverage of the new package.** Each package is covered by its own tests only
  `[measured 94e6de6:.githooks/coverage-ratchet.sh:10-13 · sed -n '10,13p' → "each package covered by ITS OWN tests — the Go default"]`,
  and `testing` writes the profile when `M.Run` returns
  `[measured go1.26.5:src/testing/testing.go:2435,2685,2751 · sed -n → "defer m.after()", "m.writeProfiles()", "coverReport()"]`,
  so nothing `Main` does after its runner returns is counted by its own `TestMain`. Mitigation:
  `Main` is a single delegating statement, and every branch of the check behind it is driven by T2's
  unit tests directly `[derived → the pre-commit coverage ratchet passes at T2]`.
- **The check's own tests share a process with the check.** T2's cases start goroutines the check
  must see; beside a parallel neighbour they would also see the neighbour's. Mitigation: the cases
  that exercise the check call no `t.Parallel`, and each releases and waits for every goroutine it
  started before returning — the package's own `TestMain` is the backstop that says so
  `[derived → T2]`.
- **goleak panics on a traceback it cannot parse**
  `[measured goleak@v1.3.0:internal/stack/stacks.go:84-91 · sed -n '84,91p' → "panic(fmt.Sprintf(\"Failed to parse stack trace: %v\n%s\", err, trace))"]`.
  Dependency code, reached only from test binaries through `internal/leaktest`; the panic index
  lists this module's own panicking calls
  `[measured 94e6de6:ai-docs/panic-index.md:3 · sed -n 3p → "Every intentional panicking call (…) in **production** code (outside `_test.go`)"]`,
  and `internal/leaktest` makes none `[derived → the panic-gate hook and review at T2]`.
- **A test file excluded by a build constraint.** The guard parses every `_test.go`, while a gate
  compiles the default configuration only; a `TestMain` in a constrained file would satisfy the guard
  and not run. A constraint can be written two ways, and the tree carries neither: no Go file has a
  `//go:build` line
  `[measured 94e6de6 · rg -n '^//go:build' --type go → exit 1, no output; the same pattern against the constructed line "//go:build integration" → match]`,
  and no tracked Go file's name ends in a GOOS or GOARCH suffix
  `[measured 779b08b · git ls-files '*.go' | grep -E "_(<GOOS>|<GOARCH>)(_(<GOARCH>))?(_test)?\.go$", the name lists taken from go tool dist list → exit 1, no output; the same pattern against the constructed names "y_linux_test.go", "z_amd64.go", "w_windows_arm64_test.go" → all three match]`.
  Stated, not engineered.
- **A stale entry survives a filtered run** (D6) and fails the next unfiltered one — every gate is
  unfiltered `[derived → T2's filtered pair]`.
- **The whole-tree guards already bind the new package's non-test file.** It may declare no
  `func init()` and no package-level variable built by a `New…` constructor of this module — both
  walked over every non-test file under `cmd/` and `internal/`
  `[measured 94e6de6:cmd/bot/guards_test.go:20-35 · sed -n '20,35p' → "every non-test Go source file under this module's two source trees"; TestGuard_NoInitFuncAnywhere, TestGuard_NoPackageLevelSubsystemVarAnywhere]`
  — and it may not import `internal/repotest`
  `[measured 94e6de6:internal/health/guards_test.go:799-820 · TestGuard_NoNonTestFileImportsRepotest → "non-test file(s) import the shared root-resolving package"]`.
  D1–D6 need none of the three; only T5's guard, a test file, reaches `repotest`
  `[derived → the whole suite green at T2 and T5]`.
- **No comment this task writes may name another package of this module by qualified symbol**
  `[measured 94e6de6:ai-docs/doc-convention.md:79 · sed -n 79p → "A package-qualified symbol of **this** module named outside the comment's own package"]`
  — `internal/leaktest`'s comments describe the provisioner's `Main` rather than write
  `testdb.Main`, and the comments T1, T3 and T4 rewrite in `internal/tgtest` and `internal/testdb`
  describe the detector rather than write `leaktest.Main`
  `[derived → make comment-refs green at T1, T2, T3 and T4]`.

## Test Design

Test names are the implementor's; the propositions are not.

### T1 — `internal/tgtest/tgtest_test.go`

- **Entry point:** `New`, `Delayed`, through `(*Server).Client()` requests.
- **Cleanup case:** a handler that closes `started` on entry and `returned` on exit wraps
  `Delayed(<a delay far beyond the case's patience>, Success(nil))`. Inside a subtest: `New` with it,
  and a `POST` carrying a JSON body the handler never reads — the shape every consumer sends (D8) —
  whose context the case cancels only after `started` has closed, so the case cannot pass because
  the handler never ran. After the subtest returns (its cleanup has run), `returned` must close
  within a generous patience budget. Proposition: a `Delayed` handler in flight when its server's
  cleanup runs returns without waiting out its delay, and it is the cleanup that releases it
  `[derived → D8; AC5 — the leak fixed, not ignored]`. A body-less request would not discriminate:
  its handler is released by the client's cancel alone (D8's measurement).
  **Two red demonstrations**, each from a cp-backup before the fix lands and each required red:
  today's `Delayed`, whose handler sleeps on; and the half-fix — `Delayed` waiting on its request's
  context while the cleanup cancels nothing — whose handler is never released either, because a
  client giving up does not end a request whose body is unread. In both, `returned` stays open.
- **Bubble case:** inside `synctest.Test`, `New` with `Delayed(d, Success(…))` and a request whose
  context outlives `d`: the call succeeds, and the virtual time elapsed across it equals `d` exactly.
  Proposition: the new wait is one the bubble sees as durably blocked (KD-26) `[derived → D8]`.
- `TestServer_Delayed` stays as it is.
- `internal/tg/retry_test.go`: the comment rewrite § Propagation specifies; the test's body and
  assertions do not change `[derived → T1's diff]`.

### T2 — `internal/leaktest/leaktest_test.go` (internal) and `main_test.go` (external)

- **Entry points:** the unexported check behind `Main` — the tests to run, the report writer, the
  module path, a predicate for "this run was filtered", and the entries — returning the exit code;
  and the two sources `Main` wires into it, the module-path source and the filter predicate (D5, D6).
- **Shape:** the cases that exercise the check run sequentially; a helper starts a goroutine inside a
  named function of the test file and returns a release that also waits for it to exit.
- **Scenarios:**
  - clean run → `0`, nothing written;
  - runner returns non-zero, with a goroutine still running → that code unchanged, nothing written
    (the check does not run);
  - two goroutines left running in distinct named functions → non-zero; the report names both
    functions and carries a `created by` line for each `[derived → AC1, AC2]`;
  - an entry in use — a stdlib-only goroutine (an `httptest.Server`'s serve goroutine) excused by an
    `Anywhere` entry on `net/http.(*Server).Serve` → `0` `[derived → AC6: a needed entry does not fail]`;
  - that entry plus one naming a function on no stack → non-zero, the second named as not needed
    `[derived → AC6]`;
  - two entries matching the same goroutine → non-zero, both named as not needed `[derived → D6]`;
  - **the "running" half alone** — a goroutine the standard library started to run this package's
    test code: a `time.AfterFunc` callback blocked in `sync.(*WaitGroup).Wait`, excused by an
    `Anywhere` entry on `sync.(*WaitGroup).Wait` → non-zero, the report carrying the module frame.
    Its creator is `time.goFunc` (D6's measurement), so a scan that reads only `created by` lines
    lets it through `[derived → AC5, "running"]`;
  - **the "started by" half alone** — a goroutine whose frames are all standard library but which a
    named function of the test file started: `go srv.Serve(l)` on an `http.Server` and a listener,
    excused by an `Anywhere` entry on `net/http.(*Server).Serve` → non-zero, the report carrying the
    module's `created by` line. It pairs with the `httptest.Server` case above — the same frames
    under a standard-library creator, and `0` — so a scan that reads only frames lets it through
    `[derived → AC5, "started by"]`;
  - refusals, each with a runner that records whether it was called — blank reason, blank function,
    a function under the module path, a nil runner, an empty module path → non-zero, runner not
    called, the refusal named `[derived → AC3, AC5]`;
  - a stale entry on a filtered run → `0` with the skip line; the same entry on an unfiltered run →
    non-zero `[derived → D6]`;
  - **the real filter predicate**, driven with a fake flag lookup: nothing set → unfiltered; each of
    `test.run`, `test.skip` and `test.list` non-empty, and `test.short` true → filtered; a lookup
    that finds no flag at all → unfiltered. A predicate that always answers "filtered" — which would
    switch D6 off on every gate — is red here `[derived → D6; AC6]`;
  - **the real module-path source** returns the path the `module` line of `go.mod` declares, read
    through `repotest.RootPath` `[derived → D5]`.
- **`main_test.go`:** the package's own `TestMain` in the D2 form, `(*testing.M).Run` as its runner
  `[derived → AC4]`.

### T3 — `internal/testdb/server_test.go`

- **Entry point:** `callersOfMain` over a scratch tree built with `srcguard.WriteScratchFile` /
  `WriteScratchFileIn`.
- **Scenarios:** directories whose test file calls the provisioner's `Main`; passes it as a value;
  reaches it through an aliased import; names it only inside a string literal; does not reference it.
  Exactly the calling, value-passing and aliased directories are returned `[derived → D9]`. Red
  against today's predicate: the value reference and the alias are missed and the string literal is
  counted.
- `TestBinaries_matchesTree` keeps its assertion `[derived → KD-20's manifest, green at T3 and T4]`.

### T4 — no new test

- **Gates:** `make test`, `make test-race` and `make test-fallback`, every package `ok`; and
  `rg -n 'leaktest\.Ignore' --type go` finds no entry outside `internal/leaktest`'s own tests
  `[derived → AC4, AC5]`.

### T5 — `internal/leaktest/guard_test.go` (external)

- **Real tree:** from `repotest.Root`, no offending directory; positive control — the packages the
  walk found include `internal/leaktest` itself, so an empty or truncated walk fails as an instrument
  failure instead of passing `[derived → AC4, AC7]`.
- **Discriminating scratch cases**, each its own scratch tree `[derived → AC7]`:
  - reported: a package with a test and no `TestMain`; a `TestMain` calling only the provisioner's
    `Main`, today's form; one calling `leaktest.Main` and then `os.Exit(0)`; one passing something
    other than its own parameter; one with a second statement; two `TestMain` declarations across a
    directory's internal and external test files;
  - not reported: a compliant package with either runner, with entries, and under import aliases; a
    compliant `TestMain` in the external test package of a mixed directory; `_test.go` files under
    `testdata/`, `_x/` and `.x/`, and a `_x_test.go` file.

### Acceptance criteria → where they are established

| AC | Established by |
|----|----------------|
| AC1 | T2's leak case; T4's rollout; the AC1/AC2 probe |
| AC2 | T2's leak case (report content); the AC1/AC2 probe |
| AC3 | T2's blank-reason refusal |
| AC4 | T4's rollout; T5's real-tree case |
| AC5 | T1 (the leak fixed, not ignored, and both red demonstrations); T2's module-name refusals and its "running" and "started by" cases; T4's empty set; the AC5 probe |
| AC6 | T2's stale, redundant and filter-predicate cases; the AC6 probe |
| AC7 | T5; the AC7 probe, run against a cached result |

### Verification probes (Step 9; not committed; every edit restored from a cp-backup)

- **AC1/AC2:** a test in a package with no database starts a goroutine blocked forever; `make test`
  → that package `FAIL`, and the gate log carries the goroutine's stack, its function and its
  `created by` line.
- **AC5:** the pre-D8 `Delayed` restored from `git show` into `internal/tgtest/tgtest.go`, and
  `internal/tg`'s `TestMain` given an entry on `time.Sleep` with a reason; `make test` →
  `internal/tg` `FAIL`, the entry named as excusing this module's code, with the handler's stack.
- **AC6:** one package's `TestMain` given an entry naming a function on no stack; `make test` → that
  package `FAIL`, the entry named as not needed.
- **AC7, through the cache:** `go test ./internal/leaktest/` twice, the second answering `(cached)`;
  then a scratch package with a test and no `TestMain` added under `internal/`; the same command →
  not `(cached)`, `FAIL`, the directory named; the scratch package deleted.

## Open questions

- None open. Round 1's `SPEC-REMIT` against the spec's propagation row is resolved: the owner struck
  the row, and § Propagation stands under `AGENTS.md` § Propagation Rule (the owner's ruling is quoted
  there).
- **Observation outside this task, for the orchestrator.** `make test-contention`, run once on a
  scratch clone, ended red for `cmd/bot` and `internal/store` because the shared server stopped
  accepting connections mid-run, while its exhaustion scan was clean
  `[measured 94e6de6 + probe + a D8 candidate · make test-contention → "test-contention: exhaustion scan clean"; tmp/test-contention-race.log: "failed to execute pg_try_advisory_lock: unexpected EOF", then "dial tcp 127.0.0.1:…: connect: connection refused", FAIL for cmd/bot and internal/store only]`.
  It is scratch-tree evidence, and nothing in this design relies on it. The lost server has nothing
  to do with leak detection and was not investigated; it may deserve its own look.
