# Interview state — split the CI Test job into four parallel jobs

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-split-ci-test-job.spec.md
issue_ref: "free-text"             # entry mode; the tracking issue created at Step 4 is in the spec's Tracked in field
task_description: |
  ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback
round_cap: 4
questions_per_round_cap: 3
round: 3
agent_id: a0752a1d5c1fe3b3d
prior_qa:
  - round: 1
    question: "Утверждаешь спеку в этом виде?"
    answer: "Утверждаю"
  - round: 1
    question: "Трекинг-issue: подходящего открытого не нашлось. Что делаем?"
    answer: "Создать новый issue"
  - round: 3
    question: "design-review (GO, раунд 2) выдал заметку, которую сам пометил как spec-amending и маршрутизовал владельцу: AC3 прочитано как паритет paths-filter, и это прочтение нигде не записано; при буквальном чтении AC1+AC2+AC3 несовместимы, потому что сегодня красный `make test` подавляет три последующих шага, а после разбиения — нет. Как решаем: править спеку / только дизайн / оставить как есть?"
    answer: "Только дизайн (рекомендую)"
```
