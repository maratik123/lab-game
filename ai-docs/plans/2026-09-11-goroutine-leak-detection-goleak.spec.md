# Goroutine-leak detection in the test suite: adopt goleak

**Source:** issue #74
**Date:** 2026-09-11
**Tracked in:** #74

## Scope

1. **A leaked goroutine is a checked failure of the test suite, not a finding review has to
   make.** A goroutine that this module's code starts or runs, and that a test leaves running,
   fails the suite on the routes where the suite is a gate
   [task: "condition in the test suite rather than a reviewed one"].
2. **The detection runs in every covered package, whatever owns that package's test entry point
   today** — the database-backed packages included, where `internal/testdb.Main` owns `TestMain`.
   How the detection composes with that entry point is the design's call, and the design makes it
   explicitly [task: "it has to be made rather than discovered"].
3. **The ignore set** — the goroutines the detection deliberately does not report, such as those
   the pgx pool, the container runtime, the Prometheus client and HTTP keep-alive connections hold
   across tests — **carries a reason per entry and stays honest as dependencies move**
   [task: "The ignore set, with a reason per entry."].
4. **A detection identifies what leaked**: the failure names the leaked goroutine's stack, not
   merely that a count is non-zero [task: "a failure that names the leaked goroutine's stack"].
5. **Every leak the detection surfaces in today's code when it lands is handled under one stated
   policy** (Key decisions) [task: "A pre-existing leak surfaced by adoption"].
6. **Propagation.** This task adds a check to the suite's gates; every live site whose claim the
   diff falsifies is updated in the same PR, per AGENTS.md § *Propagation Rule* step 4
   [task: "condition in the test suite rather than a reviewed one"].

## Out of scope

- **#24's shutdown behaviour, and the leak check its smoke condition already carries.** #24 shipped
  both (PR #75); this task generalises that check into a suite-wide capability rather than
  delivering it again.
- **Making `go.uber.org/goleak` a module requirement.** #24 already made it a direct requirement
  [source: 52f505a:go.mod § require · grep -n goleak go.mod], with its reason stated in that task's
  design [source: 52f505a:ai-docs/plans/done/2026-09-10-cmd-bot-composition-root.design.md § Dependencies · sed -n '/^### Dependencies/,/^### What this task does not touch/p' ai-docs/plans/done/2026-09-10-cmd-bot-composition-root.design.md].
- **Race detection.** `-race` is already a required gate and detects a different class.
- **Test provisioning and the container-runtime import invariant** — KD-20 and #67 own those
  surfaces; this task only avoids contradicting them.
- **Telemetry.** No metric and no event: the task adds no mechanic and moves no balance.

## Deferred

_None._

## Key decisions

| Question | Decision |
|---|---|
| A leak the detection surfaces in today's code when it lands — fixed inside this task, or ignored explicitly with its own tracking issue? | TBD |
| Which packages the detection covers — every package with tests, or only the packages that start goroutines today? | TBD |
| What keeps the ignore set honest as dependencies move? | TBD |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | In every covered package, a goroutine started by, or running, this module's code that is still running when that package's tests have finished fails that package's tests as `make verify` and CI run them. [task: "condition in the test suite rather than a reviewed one"] |
| AC2 | The failure a detection produces carries the stack of each leaked goroutine it reports, so the goroutine is identifiable from the failure output alone. [task: "a failure that names the leaked goroutine's stack"] |
| AC3 | Every entry of the ignore set states why the goroutine it matches is not a leak. [task: "each entry carries its justification"] |
| AC4 | TBD — the set of covered packages (Key decisions). |
| AC5 | TBD — the state of every leak the detection surfaces in today's code when it lands (Key decisions). |
| AC6 | TBD — the condition that keeps the ignore set honest as dependencies move (Key decisions). |

## Open questions
