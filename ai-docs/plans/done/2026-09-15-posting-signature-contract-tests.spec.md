# Posting-signature contract tests

**Source:** issue #26
**Date:** 2026-09-15
**Tracked in:** #26

## Scope

1. A basis-document type can declare its posting signature: the postings it expects — which account definitions, which kinds, which signs, which cardinality — together with the item movements it expects. [task: "per basis-document type, the expected postings (which account definitions, which kinds, which signs, which cardinality) and the expected item movements"]
2. A contract check that, given a document type and a real transaction written under a document of that type, confirms that the actual postings and item movements conform to the type's declared signature, and that nothing the signature does not declare was written. [task: "given a document type and a real transaction, asserts the actual postings and movements conform — including that nothing unexpected was written"]
3. A write under a basis-document type that has no declared signature fails the contract check. [task: "A write under a basis-document type that has no declared signature fails the check."]
4. A type that carries a signature of its own is a type within a basis table: each event type, each task type, and the manual correction. [answer 1.1: "Тип в таблице"]
5. The manual correction's signature is declared, and it is any balanced set of postings. [task: "The manual correction's signature — any balanced set — declared as the design states it."]
6. The manual correction's permission to write any balanced set loosens the check of no other document type. [task: "(manual correction) is expressed without weakening the check for everything else"]
7. The manual correction's signature admits any item movements as well. [answer 1.3: "Любые"]
8. A change to a document type's signature is visible to a reviewer in the pull request's diff. [task: "how a reviewer sees a signature change in a diff"]

## Out of scope

- A signature for a mechanic that does not exist yet: each ships in the pull request of the mechanic that moves the balance, which declares it and exercises the contract check there.
- The event-dictionary half of the telemetry obligation: #21 owns the event-type registry; this task owns the postings half.
- A completeness gate requiring a declared signature for every existing basis-document type, and every declaration that would exist only to satisfy one — an explicitly empty signature for a type no mechanic writes under, and a whole-table signature for the player operation. The owner narrowed the task to exclude it (Key decisions row 2).

## Deferred

None.

## Key decisions

| Question | Decision |
|---|---|
| What is a basis-document type that carries a signature of its own: each basis table, or each type within one? See § Source conflicts. | A type within a basis table: each event type (such as `shop_sale`), each task type, and the manual correction carries its own signature. [answer 1.1: "Тип в таблице"] |
| Does this task require a declared signature for every existing basis-document type? | No. The completeness gate over every type is removed, and with it everything that rested on it: the explicitly empty signatures and the player operation's whole-table signature. Answers 1.2 and 2.1, given to questions that presupposed that gate, are superseded. What remains: the declaration form, the check of a real transaction, the per-type granularity, the manual correction's any-balanced-set signature, the diff, and a failed check for a write under a type with no declared signature. [answer 3.2: "Сузить (Recommended)"] |
| Does the manual correction's "any balanced set" extend to the item movements written under it? | Yes: the manual correction admits any item movements. [answer 1.3: "Любые"] |

## Source conflicts

### What a "basis-document type" is — `~/lab-private/DESIGN.md` § 11

Site A — types are the basis tables:

> «Типы оснований — **отдельные таблицы** со своей схемой и жизненным циклом: игровое событие (из лога 13.1), ручная коррекция, cron-задача (закрытие дня), отложенная one-time, рекуррентная таска, закрытие сезона, операция игрока (`player_operation`, источник — enum; 2026-09-02)…»

[source: 48f7c2e:~/lab-private/DESIGN.md § 11. Технические решения · grep -n 'Типы оснований — ' ~/lab-private/DESIGN.md]

Site B — the signature examples are keyed by a type inside a table (`shop_sale` is an event type in § 13.4's starter dictionary) beside a whole table (manual correction):

> «**Проводочные сигнатуры по типу документа**: у каждого типа основания — ожидаемый набор проводок (`craft_succeeded`: списания ресурсов + зачисление предмета; `shop_sale`: ресурс↔деньги; ручная коррекция: любая сбалансированная пачка).»

[source: 48f7c2e:~/lab-private/DESIGN.md § 11. Технические решения · grep -n 'Проводочные сигнатуры по типу документа' ~/lab-private/DESIGN.md]

Site C — task types are named as basis documents:

> «Типы тасок = документы-основания леджера — своя модель, без маппинга на чужую.»

[source: 48f7c2e:~/lab-private/DESIGN.md § 11. Технические решения · grep -n 'Типы тасок = документы-основания' ~/lab-private/DESIGN.md]

Resolution: the owner chose the type within a table (round 1, answer 1.1): each event type, each task type and the manual correction carries its own signature.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | The contract check passes for a transaction whose document's actual postings and item movements are exactly those its type's signature declares. [task: "asserts the actual postings and movements conform"] |
| AC2 | The contract check fails for a transaction whose document's actual postings differ from its type's declared signature in any one of: an account definition, a kind, a sign, or the cardinality of a declared posting. [task: "which account definitions, which kinds, which signs, which cardinality"] |
| AC3 | The contract check fails for a transaction whose document's actual item movements differ from the movements its type's signature declares. [task: "item movements are part of the same contract"] |
| AC4 | The contract check fails for a transaction whose document wrote a posting or an item movement that its type's signature does not declare. [task: "including that nothing unexpected was written"] |
| AC5 | The contract check judges a document by the signature of its own event type or task type: a transaction that conforms to one event type's signature fails the check when its document carries a different event type whose signature differs. [answer 1.1: "Тип в таблице"] |
| AC6 | The contract check fails for a transaction written under a document whose basis-document type has no declared signature. [task: "A write under a basis-document type that has no declared signature fails the check."] |
| AC7 | A manual correction whose postings are any balanced set passes the contract check, while for every other document type a posting outside its declared signature still fails it. [task: "(manual correction) is expressed without weakening the check for everything else"] |
| AC8 | A manual correction under which item movements were written beside a balanced set of postings passes the contract check, whatever those movements are. [answer 1.3: "Любые"] |
| AC9 | A change to a document type's declared signature shows in a pull request's diff as a change a reviewer can read as a change in the postings or item movements that type expects. [task: "how a reviewer sees a signature change in a diff"] |

## Open questions

None.
