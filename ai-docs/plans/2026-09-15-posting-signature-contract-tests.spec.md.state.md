# Interview state — posting-signature contract tests

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-15-posting-signature-contract-tests.spec.md
issue_ref: "#26"
gh_issue:
  title: "Posting-signature contract-test framework"
  state: open
  labels: ["mvp", "area:ledger"]
  body: |
    ## What

    The design promises that every basis-document type has an expected posting set, and that a contract test checks actual postings against it. That check does not exist. This issue builds the framework; each balance-moving mechanic applies it to its own document types in its own PR.

    ## Design refs

    - `docs/DESIGN.md` §11 — **posting signatures per document type**: `craft_succeeded` writes resource debits plus an item credit; `shop_sale` is resource against money; a manual correction is any balanced set. The structural check "actual postings match the signature" is the economy's contract test. The §13.4 event dictionary extends into signatures: a new mechanic declares its event **and** its signature, and the harness verifier checks it.
    - `docs/DESIGN.md` §13.4 + `AGENTS.md` § Domain Rules — **a mechanic that moves balances declares its posting signature in the same PR**, with the contract test that checks it. This is an axiom, not a nicety.
    - `ai-docs/deferred/_inbox.jsonl` — recorded as deferred from the ledger-core task with the note "needs a mechanic with a signature to check — with the first posting mechanic". This is that follow-up.
    - `docs/DESIGN.md` §11 — item movements are part of the same contract: one document produces postings in both machines, and **both** signatures are checked.

    ## Depends on

    #25

    ## Scope

    - A signature declaration form: per basis-document type, the expected postings (which account definitions, which kinds, which signs, which cardinality) and the expected item movements.
    - A test harness that, given a document type and a real transaction, asserts the actual postings and movements conform — including that nothing unexpected was written.
    - A write under a basis-document type that has no declared signature fails the check.
    - The manual correction's signature — any balanced set — declared as the design states it.

    ## Out of scope

    - Signatures for mechanics that do not exist yet — each ships with its mechanic, which is the point.
    - The event-dictionary half of the obligation — #21 owns the registry; this issue owns the postings half.
    - A completeness gate that requires a declared signature for every existing basis-document type. `DESIGN.md` §13.4 binds the signature to a mechanic that moves balances and gives the declaration check to the harness, whose review checklists already rate a balance-moving mechanic without a posting signature or a contract test as `major`. Narrowed by the owner during the spec interview, 2026-09-15.

    ## Telemetry obligation

    This issue is the check mechanism for the posting half of the telemetry axiom; each balance-moving mechanic declares its signature and exercises the check in its own PR.

    ## Open questions to close in the spec

    - Whether a signature is Go data or a declarative file, and how a reviewer sees a signature change in a diff.
    - How "any balanced set" (manual correction) is expressed without weakening the check for everything else.

    ---

    Part of #47 (MVP roadmap).
  comments: []
  linked_issues: ["#25", "#21", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 5
agent_id: ae858b7d19a9300e8
prior_qa:
  - round: 1
    question: "Что считать одним типом документа-основания со своей сигнатурой? §11 противоречит себе: «Типы оснований — отдельные таблицы», но примеры сигнатур — shop_sale (тип события внутри одной таблицы) и ручная коррекция (целая таблица), а «Типы тасок = документы-основания»."
    answer: "Тип в таблице"
  - round: 1
    question: "Тип уже существует, но механики, двигающей под ним балансы, ещё нет (сейчас так со всеми: 16 типов событий в реестре, ни один не пишет проводки в продовом коде). Что проверка полноты требует от такого типа?"
    answer: "Явно пустая"
  - round: 1
    question: "Ручная коррекция по §11 — «любая сбалансированная пачка». Распространяется ли это на движения предметов, записанные под коррекцией?"
    answer: "Любые"
  - round: 2
    question: "Выбран «тип в таблице», но у операции игрока нет типа внутри таблицы: колонки вида действия нет, и продовый код сейчас не создаёт ни одной операции игрока. Как операция игрока входит в проверку полноты? (Поправка к раунду 1: реестр типов тасок есть — scheduler.Registry; неверным было только это замечание в описании варианта.)"
    answer: "Целиком, пусто"
  - round: 3
    question: "Design-review дал GO, но одно замечание упирается в спеку. Дословно (#5, note): «AC11 requires \"every existing type that no balance-moving mechanic writes under\" to be explicitly empty. On the merged tree no production code writes under the manual correction either, yet D10 gives it `AnyBalanced`. That follows from Scope 6 and AC8, but the design never says so. A literal Step 9 check of AC11 against the `TestDeclared_forms` log could flag it.» Предложение ревьюера: «In D10, and in the `TestDeclared_forms` bullet, state that Scope 6 carves the manual correction out of AC11. This only records the design's reading. Scope 6 is the more specific row, so the spec itself doesn't contradict.» Как поступить?"
    answer: "Давай почитай issue, смежные issue и DESiGN.md, может, тут в спеке искусственно раздут скоуп, в том числе и мной?"
  - round: 3
    question: "Гейт полноты по всем типам — надстройка issue #26 над DESIGN.md (§13.4 ограничивает обязанность механиками, двигающими балансы, и отдаёт проверку харнессу), а ответы 1.2 и 2.1 его расширили. Как поступить со скоупом #26? Варианты: «Сузить (Recommended)» — → spec amendment via spec-writer + design-review re-run. Убрать гейт полноты по всем типам и всё, что на нём держится: явно пустые сигнатуры, строку операции игрока, ось тасок в cmd/bot. Остаётся: форма, проверка по транзакции, гранулярность по типу, коррекция AnyBalanced, дифф, провал при записи под типом без сигнатуры. Тело #26 обновляется под новый скоуп. Замечания ревью #1 и #5 уходят вместе с гейтом. / «Отложить #26» / «Оставить скоуп»."
    answer: "Сузить (Recommended)"
  - round: 5
    question: "Design-review (раунд 2, GO) заметил: хелпер «мигрированный пул схемы» уже скопирован в 4 пакета, #26 добавил бы пятую копию. Design-writer решил: новый тест-пакет internal/storetest с Pool(), contract берёт его оттуда; копию store перенести нельзя (import cycle, проверено). Вопрос: переводить ли в этом PR три остальные копии (newScheduler в internal/scheduler, newIngestPool в internal/ingest, newBotPool в cmd/bot) на storetest.Pool? Задача этого не просит. Варианты: «Нет (Recommended)» — дизайн как есть, три копии остаются / «Да, перевести» — → design amendment via design-writer: решение с вашими словами в дизайне, плюс одна code-подзадача в группе A (правки scheduler_test.go, ingest main_test.go, cmd/bot readiness_test.go). Спека не меняется."
    answer: "Да, перевести"
```
