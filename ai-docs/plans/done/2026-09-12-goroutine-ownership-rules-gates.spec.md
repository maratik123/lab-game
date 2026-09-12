# Goroutine-leak prevention: ownership rules and the gates that hold them

**Source:** issue #80
**Date:** 2026-09-12
**Tracked in:** #80

## Scope

1. A bare goroutine launch outside an allow-listed set fails a gate, and each launch on that allow list states how it stops, who waits for it, and where its error and its panic go. [task: "A bare goroutine launch outside an allow-listed set fails a gate."]
2. A `time.After` inside a loop, a `time.Tick`, and a `context.Background()` or `context.TODO()` outside `main` packages and tests fail the lint gate; a site that genuinely needs a fresh context keeps one only with a stated reason and a named owner, and a detached operation also carries its own timeout. [task: "`time.After` inside a loop, `time.Tick`, and `context.Background()` / `context.TODO()` fail the lint gate outside `main` packages and tests."]
3. A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate. [task: "A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate."]
4. A nil dereference or an impossible nil comparison that static analysis can prove within a function fails the lint gate. [task: "A nil dereference or an impossible nil comparison that static analysis can prove within a function fails the lint gate"]
5. Every current site that breaks a new gate is fixed or carries its stated reason; none lands silently excused. [task: "Every current site that breaks a new gate is fixed or carries its stated reason"]
6. Written rules an implementor and a reviewer read, carried by `ai-docs/code-style.md`, stating at least: [task: "an implementor and a reviewer read"] [answer 1.2: "ai-docs/code-style.md carries both"]
   - no goroutine is started from `init()` or as a constructor's side effect; a component that starts goroutines exposes an explicit lifecycle — a `Run(ctx) error` that blocks until it is stopped or cancelled, or `Start` plus `Stop` / `Close`;
   - a new long-lived component joins the composition root as a runner or as a closer, and not as a free goroutine [task: "never as a free goroutine"];
   - a goroutine does not outlive its context or its stop seam: every blocking operation inside it — a channel send or receive, a lock, a network call — can wake on cancellation, and a goroutine that outlives its context is a leak even if it ends eventually;
   - the one named exception to that rule is a scheduler handler blocked outside the database after its deadline, which the `Handler` contract covers [task: "is an explicit exception to the ownership rule: no mechanism reclaims that goroutine"];
   - a `CancelFunc` is deferred where its context is derived, on that line or the next; `context.WithCancelCause` / `context.AfterFunc` carry an explicit reason for a stop;
   - a timer or a ticker is stopped;
   - the sender closes a channel, and nobody else does; a receiver's early return never strands a sender — the channel is buffered to the number of senders, or every sender selects on cancellation;
   - no goroutine per inbound update or message without a bound;
   - a goroutine that runs a handler supplied through an interface recovers a panic at that boundary, records its stack, and routes it into the same failure path a returned error takes, while a package's own loops are not wrapped [task: "at that boundary, records its stack, and routes it into the same failure path a returned error takes"];
   - a library that holds background goroutines — an HTTP transport, a pool, a client, a logger — is closed on the shutdown path;
   - a test stops everything it started, through `t.Cleanup`;
   - a runner is covered both when it is stopped and when it is cancelled, each asserting that `Run` returns within a bound [task: "each asserting that `Run` returns within a bound"].
7. A reviewer's checklist on the surfaces review actually reads — `ai-docs/code-style.md` carries it — asking at least: [task: "lives on the surfaces review actually reads, and asks at least"] [answer 1.2: "ai-docs/code-style.md carries both"]
   - per `go` statement: who waits for it to finish, and if nobody does, why that is acceptable; how it stops, where no answer is a blocker; whether every blocking operation inside it can wake on cancellation; where its error and its panic go; whether the number of such goroutines is bounded;
   - per channel: who closes it, and whether that is the only closer; what happens to a sender when the receiver leaves early;
   - per context: why a `Background` / `TODO` sits below `main`; whether `cancel` is called on every path; whether a detached operation has its own timeout and an owner;
   - per resource: `Close` / `Shutdown` / `Stop` in a `defer` or on the service's explicit shutdown path;
   - per pull request: one that adds a goroutine says in its description how that goroutine dies.
8. Every live site whose claim this change falsifies is updated in the same pull request, per `AGENTS.md` § *Propagation Rule* step 4 — the sites found while drafting illustrate that class and do not bound it. [task: "Every live site whose claim the diff falsifies is updated in the same PR"]
9. Every gate this task adds binds the non-`_test.go` files of a package that only tests import on the same terms as it binds production code. [answer 1.1: "Gated exactly like production code: every current site in those packages is fixed or carries its stated reason."]
10. The process's shutdown path releases the idle connections of the HTTP client the Bot API client and the canary legs use. [answer 1.3: "the shutdown path releases those idle connections"]

## Out of scope

- Detecting a leaked goroutine in the test suite — #74, merged.
- The shutdown pattern itself — #24 and KD-33, unchanged.
- The scheduler's panic recovery and its reclamation of a handler that breached its deadline — #81.
- Production observability: the goroutine-growth alert, the goroutine profile, the runbook step — #82.
- A load or soak test with a goroutine baseline — #83.
- Race detection.
- Declined by the owner on 2026-09-11, not deferred — this task delivers none of them, and none is a follow-up:
  - replacing `context.Background()` with `t.Context()` across the tests;
  - the `paralleltest` / `tparallel` linters;
  - a per-test `goleak.VerifyNone(t)`;
  - `errcheck`'s `check-blank`;
  - the `goroutinectx` linter;
  - the `nilaway` analyzer, reconsidered only if it becomes a built-in golangci-lint linter.

## Deferred

- (none)

## Key decisions

| Question | Decision |
|---|---|
| Do the new gates bind the non-`_test.go` files of packages that only tests import? | They bind them on the same terms as production code, and every current site there is fixed or carries its stated reason. [answer 1.1: "Gated exactly like production code: every current site in those packages is fixed or carries its stated reason."] |
| Which review surfaces carry the written rules and the reviewer's checklist? | `ai-docs/code-style.md` carries both. [answer 1.2: "ai-docs/code-style.md carries both"] |
| Does this task close the idle connections of the HTTP client the production path uses? | It does: the shutdown path releases them. [answer 1.3: "the shutdown path releases those idle connections"] |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | A bare goroutine launch in a gated file outside the allow-listed set fails a gate. [task: "A bare goroutine launch outside an allow-listed set fails a gate."] |
| AC2 | Every launch on that allow list states how it stops, who waits for it, and where its error and its panic go. [task: "each allow-listed launch states how it stops, who waits for it, and where its error and panic go"] |
| AC3 | A `time.After` inside a loop, and a `time.Tick`, outside `main` packages and tests fail the lint gate. [task: "`time.After` inside a loop, `time.Tick`, and `context.Background()` / `context.TODO()` fail the lint gate outside `main` packages and tests."] |
| AC4 | A `context.Background()` or `context.TODO()` outside `main` packages and tests fails the lint gate. [task: "`time.After` inside a loop, `time.Tick`, and `context.Background()` / `context.TODO()` fail the lint gate outside `main` packages and tests."] |
| AC5 | Every site that keeps a fresh context past the AC4 gate carries a stated reason and a named owner, and where its operation is detached it also carries its own timeout. [task: "A site that genuinely needs a fresh context keeps one only with a stated reason and a named owner; a detached operation also carries its own timeout."] |
| AC6 | A context stored in a struct fails the lint gate. [task: "A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate."] |
| AC7 | A context nested inside a loop or a function literal fails the lint gate. [task: "A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate."] |
| AC8 | A `defer` inside a loop fails the lint gate. [task: "A context stored in a struct, a context nested inside a loop or a function literal, and a `defer` inside a loop fail the lint gate."] |
| AC9 | A nil dereference or an impossible nil comparison that static analysis can prove within a function fails the lint gate. [task: "A nil dereference or an impossible nil comparison that static analysis can prove within a function fails the lint gate"] |
| AC10 | Every gate this task adds is green on the tree, and every site it would otherwise report is either changed so the shape is gone or carries its stated reason where the gate reports it. [task: "Every current site that breaks a new gate is fixed or carries its stated reason"] |
| AC11 | `ai-docs/code-style.md` states every rule enumerated in Scope item 6 (the ownership rules). [task: "an implementor and a reviewer read"] [answer 1.2: "ai-docs/code-style.md carries both"] |
| AC12 | `ai-docs/code-style.md` carries a reviewer's checklist that asks, per `go` statement, per channel, per context, per resource and per pull request, at least the questions enumerated in Scope item 7 (the reviewer's checklist). [task: "lives on the surfaces review actually reads, and asks at least"] [answer 1.2: "ai-docs/code-style.md carries both"] |
| AC13 | No live instruction file, `README.md` or `docs/**` page carries a claim about the gate set or about goroutine ownership that this change falsifies; the class is every live site whose claim the diff falsifies, per `AGENTS.md` § *Propagation Rule* step 4. [task: "Every live site whose claim the diff falsifies is updated in the same PR"] |
| AC14 | Every gate this task adds reports a violating shape in a non-`_test.go` file of a package that only tests import, on the same terms as it reports one in production code. [answer 1.1: "Gated exactly like production code: every current site in those packages is fixed or carries its stated reason."] |
| AC15 | Once the process's shutdown path has run, no idle connection of the HTTP client the Bot API client and the canary legs use is still held open. [answer 1.3: "the shutdown path releases those idle connections"] |

## Open questions
