# Interview state — canary limiter refusal label

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-canary-limiter-refusal-label.spec.md
issue_ref: "#89"
gh_issue:
  title: "Canary: a limiter refusal that never reached the network is labelled `network`"
  state: open
  labels: ["question", "area:observability"]
  body: |
    ## What

    A canary probe that the Bot API client's limiter refuses before any attempt — no request written, no network touched — is counted a failure with `reason="network"`.

    ## Mechanism

    - `internal/tg/caller.go` (`caller.Call`, the `!ok` branch after `limiter.acquire`) returns the typed error with `errors.New("limiter: required wait ends after the context deadline")` as its cause. The chain carries neither `context.DeadlineExceeded` nor `context.Canceled`, and `Attempts` is 0.
    - `internal/tg/limit.go` (`Limiter.acquire`) refuses when the earliest permitted instant is after the context deadline. That includes the case where the deadline has **already passed** and the required wait is zero — a class with no windows configured still refuses, and the error text then names a wait that does not exist.
    - `internal/health/probe.go` (`classifyFailure`) maps a zero status code with neither context error in the chain to `network`.

    ## Where the transport contract stands

    This is designed behaviour on the transport side, not a slip: the Bot API transport spec (AC8, AC9, AC31) makes "a call that cannot be emitted before its context deadline" return the typed error, distinguishable from a context error, and `TestRetry_DeadlineRefusalInsteadOfSleep` pins the retry-after pre-refusal's cause as *not* `context.DeadlineExceeded`. AC18 makes a deadline that passes *while waiting* a context error. An already-expired deadline at the moment of the limiter decision sits on the boundary between the two, and the canary's label set (`ai-docs/alert-contract.md`: a status code, else `timeout`, `canceled` or `network`) has no label for "refused by the limiter".

    ## Reach

    - The canary's legs are built with the process's transport configuration, limits included (`internal/health/canary.go`, `NewLegs`). `getMe` is the `Other` class with no chat, so `LAB_GAME_TG_LIMIT_OTHER_GLOBAL` is the only window that applies. It is `off` by default (`.env.example`).
    - Each tick runs under a context bounded by `CanaryInterval`, one minute by default (`internal/health/canary.go`, `tick`).
    - With the defaults, a refusal needs the probe to reach the limiter more than a minute after its tick began, which is practically unreachable. With `LAB_GAME_TG_LIMIT_OTHER_GLOBAL` set to a window whose required wait exceeds the tick interval, refused probes would be labelled `network` routinely — inferred from the code, not measured.
    - The `reason` label is not what any alert condition tests (`ai-docs/alert-contract.md`), so the stakes are the accuracy of an alert body and of routing.

    ## Open question — a design decision, not decided here

    Which of these should hold?

    1. An already-expired deadline at the limiter decision is a context error (as `golang.org/x/time/rate`'s `Wait` treats an already-done context), so the probe is labelled `timeout`, while a refusal with time still left keeps the AC9 typed cause.
    2. A limiter refusal gets its own `reason` value in the canary's label set.
    3. The current behaviour stands and the label set documents that `network` covers a refusal that never reached the network.

    ## Origin

    Found while fixing #84: `TestTelegramProber_Timeout` failed under load because its 10 ms deadline passed before the call reached the limiter, and the resulting refusal was classified `network` rather than `timeout`. #84 fixes that test by widening its deadline and leaves the production behaviour as designed; this issue carries the design question.
  comments: []
  linked_issues: ["#84"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
