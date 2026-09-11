# Goroutine-leak detection in the test suite: adopt goleak

**Source:** issue #74
**Date:** 2026-09-11
**Tracked in:** #74

## Scope

1. **A leaked goroutine is a checked failure of the test suite, not a finding review has to
   make.** A goroutine that this module's code starts or runs, and that a test leaves running,
   fails the suite on the routes where the suite is a gate
   [task: "condition in the test suite rather than a reviewed one"].
2. **Every package of this module that has tests runs the detection**, whether or not its code
   starts a goroutine today [answer 1.2: "Every package with tests runs it."].
3. **That coverage is enforced, not remembered**: a package with tests that does not run the
   detection fails the suite, so a package added after this task is covered from its first test
   [answer 2.1: "A check fails the suite when a package with tests does not run the detector"].
4. **The detection composes with whatever owns a package's test entry point today** — the
   database-backed packages included, where `internal/testdb.Main` owns `TestMain`. How it
   composes is the design's call, and the design makes it explicitly
   [task: "it has to be made rather than discovered"].
5. **The ignore set** — the goroutines the detection deliberately does not report, such as those
   the pgx pool, the container runtime, the Prometheus client and HTTP keep-alive connections hold
   across tests — **carries a reason per entry and stays honest as dependencies move**
   [task: "The ignore set, with a reason per entry."].
6. **A detection identifies what leaked**: the failure names the leaked goroutine's stack, not
   merely that a count is non-zero [task: "a failure that names the leaked goroutine's stack"].
7. **Every leak the detection surfaces in today's code when it lands is fixed inside this task**;
   none lands excused by an ignore entry
   [answer 1.1: "Every leak found at adoption is fixed here, so none lands excused."].

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
| A leak the detection surfaces in today's code when it lands — fixed inside this task, or ignored explicitly with its own tracking issue? | Fixed inside this task. Every leak found at adoption is fixed here, and none lands excused by an ignore entry. [answer 1.1: "Every leak found at adoption is fixed here, so none lands excused."] |
| Which packages the detection covers — every package with tests, or only the packages that start goroutines today? | Module-wide: every package of this module that has tests runs the detection, whether or not its code starts a goroutine today. [answer 1.2: "Every package with tests runs it."] |
| What keeps the ignore set honest as dependencies move? | A stale entry fails the suite. An entry that matches no goroutine in the runs it applies to fails the suite, so a dependency move that drops or renames a goroutine forces the entry to be updated or deleted; every entry still carries its reason. [answer 1.4: "An entry that matches no goroutine in the runs it applies to fails the suite"] |
| Does module-wide coverage bind a package added after this task — held by a check — or is it the state this task leaves, kept by review? | Enforced. A package with tests that does not run the detection fails the suite, so a package added later is covered from its first test without anyone having to remember. [answer 2.1: "A check fails the suite when a package with tests does not run the detector"] |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | In every package that runs the detection (AC4), a goroutine started by, or running, this module's code that is still running when that package's tests have finished fails that package's tests as `make verify` and CI run them. [task: "condition in the test suite rather than a reviewed one"] |
| AC2 | The failure a detection produces carries the stack of each leaked goroutine it reports, so the goroutine is identifiable from the failure output alone. [task: "a failure that names the leaked goroutine's stack"] |
| AC3 | Every entry of the ignore set states why the goroutine it matches is not a leak. [task: "each entry carries its justification"] |
| AC4 | Every package of this module that has tests runs the detection. [answer 1.2: "Every package with tests runs it."] |
| AC5 | No ignore-set entry excuses a goroutine started by, or running, this module's code: every leak the detection surfaced in today's code is fixed, not ignored. [answer 1.1: "Every leak found at adoption is fixed here, so none lands excused."] |
| AC6 | An ignore-set entry that matches no goroutine in the runs it applies to fails the test suite as `make verify` and CI run it. [answer 1.4: "An entry that matches no goroutine in the runs it applies to fails the suite"] |
| AC7 | The test suite, as `make verify` and CI run it, fails while any package of this module that has tests does not run the detection — a package added after this task included. [answer 2.1: "A check fails the suite when a package with tests does not run the detector"] |

## Open questions

_None._
