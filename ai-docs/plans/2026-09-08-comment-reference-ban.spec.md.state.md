# Interview state — comment reference ban

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-08-comment-reference-ban.spec.md
issue_ref: "#68"
gh_issue:
  title: "Strip every comment from .go, .sh and .sql — doc comments included"
  state: open
  labels: []
  body: |
    ## Decision (owner, 2026-09-08)
    
    Every comment in `.go`, `.sh` and `.sql` goes, **including the doc comments the linter currently demands**. Keeping them would just relocate the graphomania: a rule that requires a comment on every exported item is a standing invitation to write one.
    
    Only machine-read comments survive. Everything else is deleted, and the rules that mandate comments are deleted with it, in the same PR.
    
    ## Survivors — the whole list, verified at `accc794`
    
    The list is **by class, not by today's inventory** — a directive added next month is a survivor because of what reads it, not because it appears below.
    
    | Class | In the tree at `accc794` | Why it stays |
    |---|---|---|
    | Go toolchain directives — the whole `//go:` family plus `// +build` | `//go:embed migrations/*.sql` (`internal/store/migrate.go:16`) — load-bearing: strip it and the migrations are not embedded | read by the compiler / `go` tool, not by a person |
    | goose annotations `-- +goose Up` / `-- +goose Down` | 4 files in `internal/store/migrations/` | goose parses them; without them migrations do not run |
    | `//nolint:<linter> // <reason>` | 15 sites | `nolintlint` requires both the specific linter and the reason |
    | `# shellcheck disable=…` | 7 sites | same shape, shellcheck-side |
    | `#!/usr/bin/env bash` | every script | interpreter selector |
    
    `//go:build` / `// +build` appear nowhere today, and `//go:generate` neither; both are survivors anyway, by the first row.
    
    ## Harness changes that must land in the same PR
    
    Deleting the comments without these leaves `make lint` red and the instruction files lying.
    
    **Lint config** — `.golangci.yml`: `revive` is enabled *only* for `exported` and `package-comments`; both rules go, which leaves `revive` contributing nothing, so drop the linter and its `settings.revive` block.
    
    **`AGENTS.md`** — four sites:
    - § *Code Style* → the **Documentation** bullet ("every exported item carries a doc comment…") and its pointer to `ai-docs/doc-convention.md`.
    - § *Go Test Conventions* → the panic rule says a surviving `panic` is "justified in a doc comment **and** recorded in `ai-docs/panic-index.md`". The justification moves into the index; the index becomes the only carrier.
    - § *API Naming* → the **`…Unchecked` AXIOM** is defined as *"its doc comment names both the precondition and the caller that guarantees it"*. That contract needs a new carrier or a new formulation — the suffix alone cannot state a guarantor.
    - § *Learning Log* Boundary rule 2 lists `ai-docs/doc-convention.md` among the instruction files; update when that file is retired.
    
    **Docs** — `ai-docs/doc-convention.md` is the convention being retired: delete it, and its row in `ai-docs/agent-docs-index.md`. `ai-docs/go-api-naming.md` has three rows that hinge on the doc comment. `ai-docs/code-style.md`, `ai-docs/go-test-conventions.md`, `ai-docs/key-decisions.md` each carry a reference.
    
    **Harness surfaces** — 26 files mention a doc comment, a package comment or the doc convention (counted at `accc794`; re-count before the sweep, the number is a snapshot), most of them checklist rows in agents and skills (`self-review` § 6, `code-writer`, `review-findings`, `design-writer`, `design-review`, `self-improve`, `learnings-escalation-audit`, `ai-audit` × 3, `project-review`, `pr-commented`, `pr-ci-failed`, `main-ci-failed`, `bugfix`, plus `instruction-file-validation`, `agent-writing-style`, `context-status`, `corrections-log`). Each is a delete-or-restate, and the Propagation Rule puts them in this PR.
    
    **Hook** — `.claude/settings.json`'s panic gate mentions the doc-comment justification in its message; re-point it at the index.
    
    ## Where a fact goes when its comment dies
    
    Nowhere, by default — the code says what it does. The exceptions are the three contracts above (a precondition and its guarantor, a panic justification, a `//nolint` reason): those live in `ai-docs/**`, in the design document, or in the directive itself — never back in a comment.
    
    ## Sweep order (comment lines / total, at `accc794`)
    
    ```
    299 /  877  34%  .claude/skills/task/scripts/test-append-task-run.sh
    181 /  933  19%  internal/tg/limit_test.go
    131 /  345  37%  .claude/skills/task/scripts/append-task-run.sh
    123 /  776  15%  internal/tg/retry_test.go
    121 /  755  16%  internal/scheduler/deadline_test.go
    119 /  377  31%  internal/tg/limit.go
    116 /  179  64%  .claude/skills/ai-audit/scripts/check-citations.sh
    ```
    
    Totals: `*.go` 3853 / 22920 (17 %), `*.sh` 1077 / 3087 (35 %), `*.sql` 107 / 458 (23 %). Shell and tests are the worst — a test comment usually restates the test name one line below it.
    
    ## Acceptance
    
    Conditions over the tree. How each one is checked is the implementor's business, decided against the tree that exists then — this issue prescribes no commands, and nothing in it has been run against a post-sweep tree, because no such tree exists yet.
    
    - No comment remains in any `.go`, `.sh` or `.sql` file outside the survivor classes above.
    - `make verify` is green, and the migrations still apply against a fresh database.
    - No instruction file still requires a doc comment, and no lint rule still enforces one. History surfaces (`ai-docs/learnings.md`, `ai-docs/harness-gaps.md`, `ai-docs/plans/**`) keep whatever they say — they record the past, not the rules.
    - Behaviour is unchanged: in the Go and SQL diff, every removed line is a comment and no statement moves.
    
    https://claude.ai/code/session_01QCSjJTZKZXvNZZYpCCj9oc
    
  comments: []
  linked_issues: []
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: a9c88cf858e49f630
prior_qa:
  - round: 0
    question: "Ссылки на символы в комментарии (`// see raid.Session`, `// реализует Poster`) — под запрет или разрешены? Формулировка называет строки, md-файлы и AC, про символы молчит."
    answer: "ссылки на символы удаляем - причина: символы могут поменяться -> коммент протухнет"
  - round: 0
    question: "`Makefile` в списке расширений не назван, а комментарии там есть (29 строк, из них 3 со ссылками). Включаем в скоуп?"
    answer: "Makefile включаем"
  - round: 0
    question: "Блоки `run:` в `.github/workflows/ci.yml` — это shell, попадают под правило как часть `.yml`. Тела хуков в `.claude/settings.json` — тоже shell, но `.json` в списке нет и `#`-комментариев там сейчас 0. Как с ними?"
    answer: "Shell внутри хуков и ci.yml - комменты чистим по тем же правилам, но не делаем гейт для них. Просто в док-стайле упоминаем, чтобы селф-ревьювер и тот, кто пишет код обращали на это внимания."
  - round: 0
    question: "Требование «все sh-файлы имеют расширение .sh» остаётся в этой же issue? Единственный нарушитель — `.githooks/pre-commit`, имя которого диктует git через `core.hooksPath`."
    answer: "В этом issue, pre-commit становится симлинком на sh-файл"
  - round: 0
    question: "Машинно проверяется только запрет ссылок. «Не пересказывать детали реализации» не формализуется и остаётся на ревью — то есть прекоммитный хук и CI-гейт ловят ссылки, а не многословие. Так?"
    answer: "все так, как ты написал"
  - round: 0
    question: "Куда девается факт, живший в удаляемой ссылке? Например `config/balance.yaml` без `docs/DESIGN.md §…` теряет обоснование каждого числа. Переносим в дизайн / `ai-docs/**`, или просто теряем?"
    answer: "просто теряем, не жалко"
  - round: 0
    question: "(unprompted constraint from the product owner, delivered mid-round-1; relayed verbatim to the spec-writer in the same turn)"
    answer: "Еще предложение: .env.example - чистить аналогично правилам. Документацию в balance.yaml сделать самодостаточной и такой же полной, как в почищенном .env.example (это правило пропагируется на всю директорию config и все вложенные в нее директории)"
  - round: 1
    question: "Символ в комментарии: «контракт вызова» (сохраняем) и «ссылка на символ» (удаляем) сталкиваются лоб в лоб. 65 строк комментариев называют идентификатор вида Err…, часть — собственный doc-коммент переменной, остальные — перекрёстные ссылки. Как?"
    answer: "Свой пакет — символ можно назвать, когда он и ЕСТЬ контракт: возвращаемая sentinel-ошибка, гарант предусловия, но только из своего пакета. Ссылки в чужие пакеты и «см. такой-то» под запрет. DOC-3 и AXIOM …Unchecked выживают как есть."
  - round: 1
    question: "Какие файлы попадают в машинный гейт? Про .sh и .sql отдельного слова не было."
    answer: "Включая .sh/.sql — *.go, *.sh, *.sql, *.yml, *.yaml, .gitignore, .env.example, Makefile, .githooks/**."
  - round: 1
    question: "Какие ещё классы ссылок под запретом? (design D<N>/KD-<N>, §-секция без файла, #N, голый путь, URL)"
    answer: "ОТКЛОНЕНО, спека возвращена на переделку. Владелец: «зачем спек-врайтер не пользуется устоявшейся терминологией отсылки через ревизию файла? В МЕМОРИ-ФАЙЛАХ ЧЕТКО ПРОПИСАНО, КАК ССЫЛАТЬСЯ НА СТРОКИ. ЧТО ЗА ЕБАНИНА ВИДА \"файл:строка\", КОТОРАЯ СРАЗУ ЖЕ ПРОТУХНЕТ, КАК ТОЛЬКО ФАЙЛ БУДЕТ МЕНЯТЬСЯ. Пусть переделывает спеку КАК СЛЕДУЕТ, и снова возвращается с этим же вопросом». Причина в самой инструкции делегата: шаблон секции ## Source conflicts требует голый file:line, Rule 8 того же файла требует закреплённую координату с коммитом. Заведено в ai-docs/harness-gaps.md."
```
