# Interview state — test container name per checkout

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md
issue_ref: "free-text"
task_description: |
  сделать так, чтобы имя контейнера бд выводилось из имени каталога проекта, например lab-game-test-postgres для ~/lab-game и lab-game2-test-postgres для ~/lab-game2 (для параллелизации разработки)
round_cap: 4
questions_per_round_cap: 3
round: 3
agent_id: ad89014dfe5794564
prior_qa:
  - round: 1
    question: "Two checkouts whose project directories carry the same name (say ~/work/lab-game and ~/backup/lab-game) — one server between them, or one each?"
    answer: |
      Dir name. The owner selected the option labelled "Имя каталога" — the question was put in Russian, which is the owner's conversation surface — carrying the description: "The name comes from the project directory's name exactly, as in the examples. Two checkouts with the same directory name under different parents share one server."
  - round: 3
    question: "design-review round 1 flagged AC4 as SPEC-REMIT: the row cites the workspace propagation rule as its own authority, while its anchor [task: \"имя контейнера бд выводилось из имени каталога проекта\"] says nothing about document propagation, and that rule holds on every branch regardless of this task. Restate the row as the outcome it protects, strike it, or leave it?"
    answer: |
      Strike it. The owner selected the option labelled "Вычеркнуть" — the question was put in Russian, which is the owner's conversation surface — carrying the description: "Spec amendment: AC4 is removed outright. The propagation of the KD-20 and test-conventions edits is delivered by the design's subtask 3 and by the standing workspace rule regardless."
```
