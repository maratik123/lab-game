# Test database container named per checkout

**Source:** user description (free-text entry)
**Date:** 2026-09-12
**Tracked in:** #91

## Scope

1. **The long-lived test-database server a checkout brings up is addressed under a name derived
   from that checkout's project directory name.** A checkout whose project directory is named
   `lab-game` addresses `lab-game-test-postgres`; one named `lab-game2` addresses
   `lab-game2-test-postgres`
   [task: "имя контейнера бд выводилось из имени каталога проекта"].
2. **Two checkouts of this project on one host each own their own long-lived test server.**
   Bringing one up, running gates against it and taking it down in one checkout does not reach
   the server of a checkout whose project directory carries a different name
   [task: "для параллелизации разработки"].

## Out of scope

- Per-checkout isolation of any shared-per-host resource other than the test-database server.
- A new way for two checkouts to deliberately share one test server.

## Deferred

- none

## Key decisions

| Question | Decision |
|---|---|
| Two checkouts whose project directories carry the same name, under different parents — one server between them, or one each? | One server between them. The name follows the project directory's name and nothing else. [answer 1.1: "Two checkouts with the same directory name under different parents share one server."] |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | The name under which a long-lived test server is created, found again and removed is derived from the checkout's project directory name: for a project directory named `lab-game` that name is `lab-game-test-postgres`, and for one named `lab-game2` it is `lab-game2-test-postgres`. [task: "lab-game-test-postgres для ~/lab-game и lab-game2-test-postgres для ~/lab-game2"] |
| AC2 | Taking the long-lived test server down in one checkout leaves a long-lived test server belonging to a checkout with a differently-named project directory running and reachable. [task: "для параллелизации разработки"] |
| AC3 | Gates run in one checkout address that checkout's own long-lived test server, and never one another checkout brought up. [task: "для параллелизации разработки"] |
| AC4 | Two checkouts whose project directories carry the same name address one and the same long-lived test server. [answer 1.1: "Two checkouts with the same directory name under different parents share one server."] |

## Open questions
