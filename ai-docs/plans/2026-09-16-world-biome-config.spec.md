# World and biome config: generation inputs, lexicon, bestiary

**Source:** issue #28
**Date:** 2026-09-16
**Tracked in:** #28

A world is an authored whole: its biome, its tone, and the parameter that places
chats' gates in it. This task defines the configuration that holds one world and
authors the single world and biome the MVP runs on. The generator, the gate rule
and maze persistence each read parts of that configuration; each belongs to its
own issue.

## Scope

1. A world configuration format that states, for one world: its seed; the
   generation inputs — the chunk radius R, the algorithm weights, the
   growing-tree bias, and the island, extra-passage and portal shares; the
   gate-placement parameter k; the biome's parameters; its lexicon and naming
   style; and its bestiary.
   [task: "the generation inputs #119 reads — chunk radius R, algorithm weights, growing-tree bias, and the island, extra-passage and portal shares"]
   [task: "the gate-placement parameter k #120 reads; biome parameters (resource profile); lexicon and naming style; bestiary"]
   [answer 1.2: "В конфиг мира"]
2. Those generation inputs and k are the world's own rather than balance
   numbers: the balance configuration stops carrying the chunk radius it carries
   today. [answer 1.2: "В конфиг мира"]
3. Every live site whose claim that move falsifies agrees with it in the same
   change. Membership criterion: AGENTS.md § *Propagation Rule* step 4. Two
   examples, which illustrate the class and do not bound it — a statement that
   the balance configuration carries the chunk radius, and a statement that
   nothing inside the world set is read. [answer 1.2: "В конфиг мира"]
4. The world set is tracked in this repository and reaches the process through
   the configuration layer. [task: "Tracked files, loaded through #18."]
5. One complete world and biome for the MVP: the cotton-candy starting-world
   sketch, authored to the absurdist-comedy tone and in the language that sketch
   is written in. [answer 1.3: "из IDEAS сахарная вата"]
   [task: "One complete world/biome config for the MVP, authored to the absurdist-comedy tone."]
6. Validation at start-up of every generation input and of k, refusing a value
   outside its permitted range and naming the key it refused.
   [task: "every generation input and k is validated at start-up, and a rejection names its key (R at least 6, shares in range, k non-negative)"]
7. A format that holds worlds beyond the MVP one, and that precludes none of the
   post-MVP content layers.
   [task: "Additional worlds beyond the MVP one — the format must support them; the content is later."]
   [task: "The config format must not preclude them."]

## Out of scope

- Placing a gate — #120; persisting a chunk and allocating a gate — #29;
  generating the fabric — #119.
- Sampling node content from the bestiary — #34.
- The narrator that consumes the lexicon — #35.
- The prefab layer and the entrance prefab, boss areas, ruins of old entrances
  and NPC outposts: post-MVP content this task only leaves room for.
- The content of worlds beyond the MVP one.
- Any event or posting signature: this task moves no balance and declares no
  telemetry of its own. The world identifier reaches telemetry as a dimension of
  events #29 and the raid issues emit.

## Deferred

- None this round.

## Key decisions

| Question | Decision |
|---|---|
| Are a biome's resource kinds ledger enum members, world-configuration content, or items of the item machine? | Ledger enum members. Each arrives by its own `ADD VALUE` migration and is permanent after merge, so a world authored later introduces a resource only with a migration; #34 implements that representation. [answer 1.1: "Члены enum"] [task: "#34 implements whichever answer this returns."] [task: "an enum member is permanent after merge"] |
| Do the generation inputs and k live in the world configuration, or does the balance configuration keep the chunk radius it carries today? | In the world configuration, per world; the balance configuration stops carrying the chunk radius. This settles the conflict recorded under *Source conflicts*. [answer 1.2: "В конфиг мира"] |
| What content does the MVP world carry? | The cotton-candy starting-world sketch from the design corpus' idea backlog, which supplies tone, lexicon register and bestiary but no resources or stats. [answer 1.3: "из IDEAS сахарная вата"] |
| Does the design corpus record the promotion of that sketch, and how? | TBD — round 2 question |
| Does this task author the MVP biome's resource profile, or does it arrive with the enum members in #34? | TBD — round 2 question |

## Source conflicts

`~/lab-private/DESIGN.md` files the same values under two different owners, and
this repository's configuration sources are disjoint by domain, so the two
readings name two different files. All sites, verbatim:

- §16, open question 5 — R, the three shares and k are balance numbers:
  «радиус чанка R (в MVP ≥ 6), доли островков, дополнительных проходов и
  порталов, параметр размещения ворот k … Все балансовые константы — в конфиг,
  не в код.»
  [source: 48f7c2e:DESIGN.md § 16 item 5 · `git -C ~/lab-private show 48f7c2e:DESIGN.md | sed -n '/^5\. \*\*Числа\*\*/p'`]
- §2.2.2 — the algorithm weights are a biome parameter:
  «Веса алгоритмов — **параметр биома**: характер коридоров авторится
  побиомно.»
  [source: 48f7c2e:DESIGN.md § 2.2.2 · `git -C ~/lab-private show 48f7c2e:DESIGN.md | sed -n '/Веса алгоритмов/p'`]
- §2.2 — the world is an authored whole, k among its parts, and the lexicon and
  bestiary come from the world's own configuration:
  «Мир — **авторская сущность целиком**: биом (ресурсы, монстры) + тон (словарь
  нарративизатора, стиль названий локаций, бестиарий) + параметр размещения
  входов (k, см. ниже). «Ткань» генератора процедурна, лексика и бестиарий — из
  конфига мира.»
  [source: 48f7c2e:DESIGN.md § 2.2 · `git -C ~/lab-private show 48f7c2e:DESIGN.md | sed -n '/лексика и бестиарий/p'`]
- The shipped tree reads with the first site: the balance configuration carries
  the chunk radius today, under `world.chunk.radius`.
  [source: cfc6fa3:config/balance.yaml § world.chunk.radius · `git show cfc6fa3:config/balance.yaml | sed -n '/^world:/,/^raid:/p'`]

**Resolution:** the world configuration owns them, and the balance configuration
stops carrying the chunk radius. Chosen by the owner in round 1
[answer 1.2: "В конфиг мира"].

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Each of these is part of a world's own configuration, read for that world at start-up: its seed; the chunk radius R; the algorithm weights; the growing-tree bias; the island, extra-passage and portal shares; the gate-placement parameter k; the biome's parameters; the world's lexicon and naming style; and its bestiary. [task: "the generation inputs #119 reads — chunk radius R, algorithm weights, growing-tree bias, and the island, extra-passage and portal shares"] [task: "the gate-placement parameter k #120 reads; biome parameters (resource profile); lexicon and naming style; bestiary"] [answer 1.2: "В конфиг мира"] |
| AC2 | The balance configuration carries no chunk radius, and no world's generation input or k is read from it. [answer 1.2: "В конфиг мира"] |
| AC3 | No live document or file states that the balance configuration carries the chunk radius, or that nothing inside the world set is read. The class is every site whose claim this change falsifies, per AGENTS.md § *Propagation Rule* step 4; those two are examples and do not bound it. [answer 1.2: "В конфиг мира"] |
| AC4 | The MVP world's configuration is tracked in this repository, and the process obtains it through the configuration layer. [task: "Tracked files, loaded through #18."] |
| AC5 | Start-up refuses a world whose chunk radius is below six, whose island, extra-passage or portal share falls outside its permitted range, or whose k is negative; the refusal names the key it refused, and the process does not continue past it. [task: "every generation input and k is validated at start-up, and a rejection names its key (R at least 6, shares in range, k non-negative)"] |
| AC6 | Start-up accepts the MVP world this task authors. [task: "the MVP world file loads."] |
| AC7 | The MVP world is one world with one biome, and its lexicon, naming style and bestiary are the cotton-candy sketch's — its imagery, its creatures and its inversion of cuteness and threat — in absurdist-comedy tone. [answer 1.3: "из IDEAS сахарная вата"] [task: "One complete world/biome config for the MVP, authored to the absurdist-comedy tone."] [task: "one maze, one biome"] |
| AC8 | A second world is expressible in the format, with its own values for every part of AC1, without changing what any part means for the MVP world. [task: "Additional worlds beyond the MVP one — the format must support them; the content is later."] |
| AC9 | A later task can add prefab placements, the entrance prefab, boss areas, ruins of old entrances and NPC outposts to a world without redefining anything this format already states. [task: "The config format must not preclude them."] [task: "nothing here may preclude it"] |
| AC10 | TBD — whether the design corpus records the sketch's promotion (fourth question in *Key decisions*). |
| AC11 | TBD — whether the MVP biome's resource profile is authored here (fifth question in *Key decisions*). |

## Open questions

- None beyond the two in *Key decisions*, both put to the owner in round 2.
