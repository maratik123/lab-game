# Canary limiter refusal label

**Source:** issue #89
**Date:** 2026-09-12
**Tracked in:** #89

## Scope
1. A canary probe that the transport's limiter refuses before any attempt reports a `reason` the health surface's live label-set contract declares for that case, and that contract agrees with what the code emits. [task: "the Bot API client's limiter refuses before any attempt — no request written, no network touched"]

## Out of scope
- Any change to which series an alert reads, or to the condition shape of any alert. [task: "The `reason` label is not what any alert condition tests (`ai-docs/alert-contract.md`), so the stakes are the accuracy of an alert body and of routing."]
- Making the race-route tests with wall-clock assumptions hold under machine load — issue #84 owns that outcome. [task: "#84 fixes that test by widening its deadline and leaves the production behaviour as designed; this issue carries the design question."]

## Deferred
- (none recorded yet)

## Key decisions
| Question | Decision |
|---|---|
| What `reason` does a canary probe report when the transport's limiter refuses it before any attempt, and does an already-expired deadline differ there from a refusal with a real wait ahead of it? | TBD — asked in round 1. |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | TBD — follows the round-1 decision. |

## Open questions
- None beyond the round-1 question above.
