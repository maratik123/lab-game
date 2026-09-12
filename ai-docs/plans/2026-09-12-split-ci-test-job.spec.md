# Split the CI Test job into four parallel jobs

**Source:** user description (free-text entry)
**Date:** 2026-09-12
**Tracked in:** none — free-text entry, no tracking issue yet.

## Scope
1. `make test`, `make test-race`, `make cover-ratchet` and `make test-fallback` run in CI as four separate jobs that execute at the same time, so a CI run reaching them costs the wall-clock of the slowest of the four rather than the sum of all four. [task: "ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback"]
2. Every live document whose claim this change falsifies states the new structure instead. [task: "job Test надо разбить на 4 отдельные джобы"]

## Out of scope
- A shorter CI wall-clock reached by any route other than running these four gates as concurrent jobs — no other gate is made faster by this task. [task: "ускоряем gh ci"]
- CI coverage for a gate the task does not name; `make test-contention` in particular gains no CI job here. [task: "make test, make test-race, make cover-ratchet и make test-fallback"]

## Deferred
- (none)

## Key decisions
- (none) — the task text names the four gates and requires them concurrent; nothing was left for the owner to settle.

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | CI invokes each of `make test`, `make test-race`, `make cover-ratchet` and `make test-fallback` from exactly one job, and no CI job invokes more than one of the four. [task: "разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback"] |
| AC2 | On a CI run that reaches them the four jobs are scheduled concurrently: none of the four is declared to start only after another of the four has finished. [task: "выполняющиеся параллельно"] |
| AC3 | Each of the four gates runs in CI on exactly the set of changes on which it runs before this split — no change starts a gate that it did not start before, and none stops starting one that it did. [task: "job Test надо разбить на 4 отдельные джобы"] |
| AC4 | Every live document whose claim this diff falsifies is corrected in the same pull request; the class is every site stating which CI job runs any of the four gates, per AGENTS.md § *Propagation Rule* step 4, and the sites known at drafting illustrate the class rather than bounding it. [task: "job Test надо разбить на 4 отдельные джобы"] |

## Open questions
- None.
