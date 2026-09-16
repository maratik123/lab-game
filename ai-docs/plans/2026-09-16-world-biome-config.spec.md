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
2. The world set is tracked in this repository and reaches the process through
   the configuration layer. [task: "Tracked files, loaded through #18."]
3. One complete world and biome for the MVP, authored to the absurdist-comedy
   tone. How complete its lexicon, naming style and bestiary must be in this
   task is the third question in *Key decisions*.
   [task: "One complete world/biome config for the MVP, authored to the absurdist-comedy tone."]
4. Validation at start-up of every generation input and of k, refusing a value
   outside its permitted range and naming the key it refused.
   [task: "every generation input and k is validated at start-up, and a rejection names its key (R at least 6, shares in range, k non-negative)"]
5. A format that holds worlds beyond the MVP one, and that precludes none of the
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
| Are a biome's resource kinds ledger enum members, world-configuration content, or items of the item machine? | TBD — round 1 question |
| Do the generation inputs and k live in the world configuration, or does the balance configuration keep the chunk radius it carries today? | TBD — round 1 question |
| How complete must the MVP world's lexicon, naming style and bestiary be in this task, and is the MVP world one of the starting-world sketches? | TBD — round 1 question |

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
  [source: 52fdb4a:config/balance.yaml § world.chunk.radius · `git show 52fdb4a:config/balance.yaml | sed -n '/^world:/,/^raid:/p'`]

**Resolution:** open — the second question in *Key decisions*, put to the owner
in round 1.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Each of these is a configured value the process reads at start-up for a world: the world's seed; the chunk radius R; the algorithm weights; the growing-tree bias; the island, extra-passage and portal shares; the gate-placement parameter k; the biome's parameters; the world's lexicon and naming style; and its bestiary. [task: "the generation inputs #119 reads — chunk radius R, algorithm weights, growing-tree bias, and the island, extra-passage and portal shares"] [task: "the gate-placement parameter k #120 reads; biome parameters (resource profile); lexicon and naming style; bestiary"] |
| AC2 | The MVP world's configuration is tracked in this repository, and the process obtains it through the configuration layer. [task: "Tracked files, loaded through #18."] |
| AC3 | Start-up refuses a world whose chunk radius is below six, whose island, extra-passage or portal share falls outside its permitted range, or whose k is negative; the refusal names the key it refused, and the process does not continue past it. [task: "every generation input and k is validated at start-up, and a rejection names its key (R at least 6, shares in range, k non-negative)"] |
| AC4 | Start-up accepts the MVP world this task authors. [task: "the MVP world file loads."] |
| AC5 | The MVP world is one world with one biome, and its lexicon, naming style and bestiary read as absurdist comedy. [task: "One complete world/biome config for the MVP, authored to the absurdist-comedy tone."] [task: "one maze, one biome"] |
| AC6 | A second world is expressible in the format, with its own values for every part of AC1, without changing what any part means for the MVP world. [task: "Additional worlds beyond the MVP one — the format must support them; the content is later."] |
| AC7 | A later task can add prefab placements, the entrance prefab, boss areas, ruins of old entrances and NPC outposts to a world without redefining anything this format already states. [task: "The config format must not preclude them."] [task: "nothing here may preclude it"] |
| AC8 | TBD — how a biome's resource kinds are represented (first question in *Key decisions*). |

## Open questions

- None beyond the three in *Key decisions*, all three put to the owner in round 1.
