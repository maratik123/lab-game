# Canary limiter refusal label

**Source:** issue #89
**Date:** 2026-09-12
**Tracked in:** #89

## Scope
1. A Bot API call the limiter refuses at an instant when the call's own context deadline has already passed reports a deadline error, which any caller can identify as one. [task: "An already-expired deadline at the limiter decision is a context error"]
2. A canary probe refused that way is counted a failure with `reason` `timeout` rather than `network`. [answer 1.1: "An already-expired deadline at the limiter decision reports `timeout`"]

## Out of scope
- Any change to which series an alert reads, or to the condition shape of any alert. [task: "The `reason` label is not what any alert condition tests (`ai-docs/alert-contract.md`), so the stakes are the accuracy of an alert body and of routing."]
- Making the race-route tests with wall-clock assumptions hold under machine load — issue #84 owns that outcome. [task: "#84 fixes that test by widening its deadline and leaves the production behaviour as designed; this issue carries the design question."]

## Deferred
- (none)

## Key decisions
| Question | Decision |
|---|---|
| What `reason` does a canary probe report when the transport's limiter refuses it before any attempt? | Whether the deadline had already passed at the limiter's decision is what decides it: a deadline already passed reports `timeout`, and a refusal made for a wait that would end past a deadline still ahead goes on reporting `network`. [answer 1.1: "An already-expired deadline at the limiter decision reports `timeout`; a refusal with a real wait that ends past the deadline goes on reporting `network`"] |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | A Bot API call the limiter refuses at an instant when the call's own context deadline has already passed fails with an error whose chain carries `context.DeadlineExceeded`. [task: "An already-expired deadline at the limiter decision is a context error"] |
| AC2 | A canary probe that fails that way is counted a failure with `reason` `timeout`. [answer 1.1: "An already-expired deadline at the limiter decision reports `timeout`"] |
| AC3 | A limiter refusal made because the required wait would end after a context deadline that has not yet passed fails with an error whose chain carries neither `context.DeadlineExceeded` nor `context.Canceled`, and a canary probe that fails that way is counted a failure with `reason` `network`. [answer 1.1: "a refusal with a real wait that ends past the deadline goes on reporting `network`"] |

## Open questions
- None.
