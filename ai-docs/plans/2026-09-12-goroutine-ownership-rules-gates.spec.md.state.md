# Interview state — goroutine ownership rules and the gates that hold them

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md
issue_ref: "#80"
gh_issue:
  title: "Goroutine-leak prevention: ownership rules and the gates that hold them"
  state: open
  labels: ["enhancement", "area:platform"]
  body: |
    ## What

    Make goroutine discipline a set of **checked** conditions and **written** rules rather than knowledge carried by whoever reviews. #74 detects a leaked goroutine after the fact, in the test suite; this issue prevents the shapes that produce one, at lint time and in review.

    The principle every item below serves: **a goroutine has an owner.** Whoever writes `go` answers three questions — how it stops, who waits for it to finish, and where its error and its panic go. A goroutine without all three answers is a defect in review even when it works today. It is rule one of both the Uber Go Style Guide ("Don't fire-and-forget goroutines") and Google's Go style.

    Static checks close the periphery, not the class itself: none of them can prove that a `<-ch` will ever receive, because the question reduces to whether something stops, and static analysis only has heuristics for that. The gates below are that periphery; the written rules and the reviewer's checklist carry the rest.

    ## Why now

    Nothing in the module stops a goroutine from being started without an owner: `ai-docs/code-style.md` § Concurrency states the ownership rule in one line, and no gate checks it. Most of the module is still unwritten — every gameplay issue in #47 is open — so the rules cost least before that code exists.

    ## Decided by the owner (2026-09-11)

    Settled before the task, against the decisions already in the repository:

    - **The shutdown pattern is KD-33's, not a new one.** The recipe this issue first carried (`signal.NotifyContext` + `errgroup.WithContext`) is withdrawn: KD-33 deliberately joins the runners with a plain `errgroup.Group` and stops them through a stop seam beside the context. What this issue adds is the rule that a new long-lived component joins the composition root as a **runner** (`Run(ctx)` plus `Stop`) or as a **closer** — never as a free goroutine.
    - **A goroutine that runs a handler supplied through an interface recovers a panic** at that boundary, records its stack, and routes it into the same failure path a returned error takes — the policy `internal/ingest` already applies to its handlers. A package's own loops are not wrapped. The scheduler's application of this rule is #81.
    - **A scheduler handler blocked outside the database after its deadline** is an explicit exception to the ownership rule: no mechanism reclaims that goroutine, and the `Handler` contract — honour your `ctx` — is what covers it. The database-blocked case is closed by #81.
    - **The production half** — an alert on goroutine growth, the goroutine profile on the service listener, the runbook step — belongs to the infrastructure pass, in #82. `docs/DESIGN.md` §13.2's alert set is unchanged by this issue.
    - **A load test with a goroutine baseline** is post-MVP, in #83.

    ## Scope

    - **A bare goroutine launch outside an allow-listed set fails a gate.** The joined forms — `errgroup.Group.Go`, `sync.WaitGroup.Go` — are the default; a bare `go` belongs only in `main` or in a wrapper that joins what it starts with an explicit wait, and each allow-listed launch states how it stops, who waits for it, and where its error and panic go.
    - **`time.After` inside a loop, `time.Tick`, and `context.Background()` / `context.TODO()` fail the lint gate outside `main` packages and tests.** A site that genuinely needs a fresh context keeps one only with a stated reason and a named owner; a detached operation also carries its own timeout.
    - **A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate.**
    - **A nil dereference or an impossible nil comparison that static analysis can prove within a function fails the lint gate** — `govet`'s `nilness` analyzer, chosen on 2026-09-11 in place of `nilaway`: a panic in a goroutine takes the process down, and this closes the provable share of nil panics at no new dependency.
    - **Every current site that breaks a new gate is fixed or carries its stated reason**; none lands silently excused.
    - **The written rules** an implementor and a reviewer read:
      - no goroutine is started from `init()` or as a constructor's side effect; a component that starts goroutines exposes an explicit lifecycle — `Run(ctx) error` that blocks until it is stopped or cancelled (preferred: it joins an `errgroup` as it is), or `Start` plus `Stop` / `Close`;
      - the runner-or-closer rule above;
      - **a goroutine does not outlive its context or its stop seam**: every blocking operation inside it — a channel send or receive, a lock, a network call — can wake on cancellation, and a goroutine that outlives its context is a leak by definition even if it ends eventually (the scheduler exception above is the one named case);
      - a `CancelFunc` is deferred where its context is derived, on that line or the next; `context.WithCancelCause` / `context.AfterFunc` carry an explicit reason for a stop;
      - a timer or a ticker is stopped — `time.NewTimer` / `time.NewTicker` with `defer Stop()`: since Go 1.23 an unreferenced timer is collected, but a running ticker keeps waking its goroutine;
      - the sender closes a channel, and nobody else does; a receiver's early return never strands a sender — the channel is buffered to the number of senders, or every sender selects on cancellation;
      - no goroutine per inbound update or message without a bound — `errgroup.Group.SetLimit`, a semaphore or a fixed-size pool; unbounded growth is not a leak, but it looks exactly like one;
      - the handler-boundary panic rule above;
      - a library that holds background goroutines — an HTTP transport, a pool, a client, a logger — is closed on the shutdown path, or goleak and the goroutine profile show it for ever and people stop reading them. Known instance: the production path passes no HTTP client, so the Bot API client and both canary legs fall back to `http.DefaultClient`, and nothing on the shutdown path closes its idle connections;
      - a test stops everything it started, through `t.Cleanup`.
    - **A reviewer's checklist** lives on the surfaces review actually reads, and asks at least:
      - per `go` statement: who waits for it to finish (`Wait`, an `errgroup`, a blocking `Run`) — and if nobody does, why that is acceptable; how it stops — by context, by a closed channel, by `Stop()` — where no answer is a blocker; whether every blocking operation inside it (`<-ch`, `ch <-`, a lock, the network) can wake on cancellation; where its error and its panic go; whether the number of such goroutines is bounded;
      - per channel: who closes it, and whether that is the only closer; what happens to a sender when the receiver leaves early (an error return, a timeout) — an unbuffered channel in a fan-out with an early return is the classic stranded sender, so ask it directly;
      - per context: why a `Background` / `TODO` sits below `main`; whether `cancel` is called on every path; whether a detached operation has its own timeout and an owner;
      - per resource: `Close` / `Shutdown` / `Stop` in a `defer` or on the service's explicit shutdown path;
      - per pull request: one that adds a goroutine says in its description how that goroutine dies — the leak check itself is #74's and runs everywhere.
    - **A runner has a test that stops it and one that cancels it**, each asserting that `Run` returns within a bound. Every runner has such tests today; the rule is written down so a new runner inherits it.
    - **Propagation.** Every live site whose claim the diff falsifies is updated in the same PR, per `AGENTS.md` § *Propagation Rule* step 4.

    ## Out of scope

    - **Detecting a leak in the test suite** — #74.
    - **The shutdown pattern itself** — #24 and KD-33, unchanged.
    - **The scheduler's panic recovery and deadline-breach reclamation** — #81.
    - **Production observability** (goroutine-growth alert, goroutine profile, runbook) — #82.
    - **A load or soak test** — #83.
    - **Race detection** — `-race` is already a required gate.
    - **Declined on 2026-09-11, not deferred:**
      - replacing `context.Background()` with `t.Context()` across the tests — the leaks it would prevent are #74's to catch;
      - `paralleltest` / `tparallel` — `goleak.VerifyNone` is documented as incompatible with `t.Parallel`, so the premise that they keep goleak stable is inverted, and this repository parallelises tests by default;
      - `goleak.VerifyNone(t)` inside individual tests, to name the test that leaked — the same incompatibility with `t.Parallel` rules it out, and #74 requires its package-level check to report each leaked goroutine's stack, which is what identifies the culprit;
      - `errcheck`'s `check-blank` — it collides with the deliberate `_ = tx.Rollback(ctx)` shape and is an error-handling question, not a goroutine one;
      - `goroutinectx` — not an established package (a single author, pre-1.0, retracted releases, no golangci-lint integration and none pursued), and the bare-`go` gate above is the stricter check: it asks every launch how it stops, not only whether it mentions a context. Its reports on today's tree are both deliberate code — the drain's join goroutine, which KD-33 stops by joining rather than by context, and the scheduler's deadline watchdog, which #81 reworks;
      - `nilaway` — replaced by `nilness` (Scope). It has no tagged release, states that false positives and breaking changes can happen, and inside golangci-lint runs only as a module plugin, which needs a custom-built binary locally and in CI; the reports examined on today's tree were inferences it cannot close — errors collected and guarded together — rather than defects. Reconsidered only if it becomes a built-in golangci-lint linter.

    ## Already in force — not to be specified again

    `ai-docs/code-style.md` § Context and § Concurrency (ctx first, never stored in a struct, no `context.Background()` inside a request path, every goroutine has an owner); `-race` as a required gate; and in the lint gate `govet` (`lostcancel`, `copylocks`, `unusedresult`, `waitgroup`), `staticcheck`, `errcheck`, `bodyclose`, `sqlclosecheck`, `rowserrcheck`, `noctx`, `contextcheck`, and `gocritic`, whose default check set includes `exitAfterDefer`.

    ## Verified facts

    - **`forbidigo` cannot forbid a `go` statement.** It visits identifiers and selector expressions only, and `go` is a statement keyword — the node switch in `forbidigo/forbidigo.go` of `github.com/ashanbrown/forbidigo/v2@v2.3.1`. A bare-`go` gate needs a different mechanism; which one is the design's choice.
    - **`goroutinectx` and `nilaway` are not linters of golangci-lint 2.13.1** (`golangci-lint help linters`), so neither can be switched on in `.golangci.yml`.
    - **staticcheck's SA1015 (`time.Tick` outside `main`) does not fire on this module.** It is gated off from Go 1.23, where tickers are collected: the same one-file probe under this repository's `.golangci.yml` reports it with a `go 1.22` directive and stays silent with the module's 1.26.0. `time.Tick` therefore needs the gate in Scope. SA2000 (`wg.Add` inside the goroutine) and SA4006 (a value never read) stay silent under this configuration too, but `govet`'s `waitgroup` and `ineffassign` report the same shapes; SA2001 and SA4017 fire.
    - **`nilness` is outside `go vet`'s default analyzer suite** (`go tool vet help` does not list it), so the lint gate's `govet` runs without it until it is enabled by name.
    - **`gocritic`'s `deferInLoop` is tagged experimental**, so it stays off until enabled by name (`checkers/deferInLoop_checker.go` of `github.com/go-critic/go-critic@v0.14.4`).
    - **A bounded-concurrency primitive costs no new module.** `golang.org/x/sync` is already a direct requirement (`grep 'golang.org/x/sync' go.mod`) and ships `semaphore` beside `errgroup`; `sync.WaitGroup.Go` is available, since the module's `go` directive is 1.26.0 and the method arrived in Go 1.25 (`go doc sync.WaitGroup.Go`).

    ## Depends on

    #74 — it touches every package's `TestMain` and the shared test helpers, and whatever it fixes or ignores is input here.

    ## Telemetry obligation

    - Metrics: none.
    - Events: none.

    This adds no mechanic and moves no balance.

    ## Open questions to close in the spec

    - **Test helpers.** Whether the bare-`go` gate and the context rules bind the non-`_test.go` files that only tests import (`internal/tgtest`, `internal/testdb`).
    - **Which review surfaces carry the checklist** — `ai-docs/code-style.md` alone, or also the `self-review` agent's checklist, which is an instruction file with propagation obligations of its own.



  comments: []
  linked_issues: ["#24", "#47", "#74", "#81", "#82", "#83"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
