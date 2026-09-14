# Posting-signature contract tests

**Source:** issue #26
**Date:** 2026-09-15
**Tracked in:** #26

## Scope

1. A basis-document type can declare its posting signature: the postings it expects — which account definitions, which kinds, which signs, which cardinality — together with the item movements it expects. [task: "per basis-document type, the expected postings (which account definitions, which kinds, which signs, which cardinality) and the expected item movements"]
2. A contract check that, given a document type and a real transaction written under a document of that type, confirms that the actual postings and item movements conform to the type's declared signature, and that nothing the signature does not declare was written. [task: "given a document type and a real transaction, asserts the actual postings and movements conform — including that nothing unexpected was written"]
3. A registry completeness check: a basis-document type with no declared signature fails the module's test suite, so it fails the build. [task: "a basis-document type with no declared signature fails the suite"]
4. A manual correction's signature admits any balanced set, and that permission loosens the check of no other document type. [task: "(manual correction) is expressed without weakening the check for everything else"]
5. A change to a document type's signature is visible to a reviewer in the pull request's diff. [task: "how a reviewer sees a signature change in a diff"]
6. Signatures are declared for every basis-document type that exists when this task lands. [task: "Application to every document type that exists when this lands"]

## Out of scope

- A signature for a mechanic that does not exist yet: each ships in the pull request of the mechanic that moves the balance.
- The event-dictionary half of the telemetry obligation: #21 owns the event-type registry; this task owns the postings half.

## Deferred

None.

## Key decisions

| Question | Decision |
|---|---|
| What is a basis-document type that carries a signature of its own: each basis table (event, player operation, deferred task, recurrent task, manual correction), or each type within one (an event type such as `shop_sale`, a task type)? See § Source conflicts. | TBD |
| What does a type that exists but has no balance-moving mechanic yet declare, and does the completeness check require a declaration from it? | TBD |
| Does a manual correction's "any balanced set" extend to the item movements written under it? | TBD |

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

Resolution: none yet. Asked of the owner in round 1; Key decisions row 1 records the answer.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | The contract check passes for a transaction whose document's actual postings and item movements are exactly those its type's signature declares. [task: "asserts the actual postings and movements conform"] |
| AC2 | The contract check fails for a transaction whose document's actual postings differ from its type's declared signature in any one of: an account definition, a kind, a sign, or the cardinality of a declared posting. [task: "which account definitions, which kinds, which signs, which cardinality"] |
| AC3 | The contract check fails for a transaction whose document's actual item movements differ from the movements its type's signature declares. [task: "item movements are part of the same contract"] |
| AC4 | The contract check fails for a transaction whose document wrote a posting or an item movement that its type's signature does not declare. [task: "including that nothing unexpected was written"] |
| AC5 | While any basis-document type has no declared signature, the module's test suite fails, and so does the build. [task: "and it fails the build"] |
| AC6 | A manual correction whose postings are any balanced set passes the contract check, while for every other document type a posting outside its declared signature still fails it. [task: "(manual correction) is expressed without weakening the check for everything else"] |
| AC7 | Every basis-document type that exists on the merged tree has a declared signature, and the completeness check passes there. [task: "Application to every document type that exists when this lands"] |
| AC8 | A change to a document type's declared signature shows in a pull request's diff as a change a reviewer can read as a change in the postings or item movements that type expects. [task: "how a reviewer sees a signature change in a diff"] |

## Open questions

None.
