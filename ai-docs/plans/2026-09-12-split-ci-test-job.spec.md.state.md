# Interview state — split the CI Test job into four parallel jobs

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-split-ci-test-job.spec.md
issue_ref: "free-text"
task_description: |
  ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
