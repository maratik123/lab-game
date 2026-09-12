# World generation core: hex topology and the deterministic chunk generator

**Source:** issue #27
**Date:** 2026-09-12
**Tracked in:** #27

## Scope
1. Hex topology primitives: axial cell coordinates, a cell's six neighbours, a canonical key for the face two neighbouring cells share, and the mapping from a cell coordinate to the chunk that holds it. [task: "Hex primitives: axial coordinates, the six neighbours, the canonical face key, coordinate-to-chunk mapping"]
2. A distance between two cell coordinates measured on the chunk grid rather than by a hex-by-hex traversal. [task: "chunk-grid distance"] [task: "cheap, stable, and BFS precision is not needed"]
3. Generation as a pure function of the world seed and a cell coordinate, yielding that cell's six face states — each a passage or a wall — with a chunk's own seed settling its interior faces and both neighbouring chunks' seeds taking part in a border face. [task: "The generator: `f(world_seed, coord)` returning cell content and its six face states, computed from the chunk seed plus neighbour chunk seeds for borders."]
4. Alongside the faces, a per-cell value derived from the world seed and that coordinate, which the world-config and node-content layers sample their own content from. [answer 1.2: "Plus cell seed"] [task: "returning cell content and its six face states"]
5. Connectivity guaranteed by construction at both of the design's two levels: a spanning structure over a chunk's cells derived from the chunk seed, and guaranteed portals on every border between neighbouring chunks. [task: "inside a chunk, a spanning structure from the chunk seed (a classic maze algorithm over a finite set — connectivity by construction)"] [task: "1-2 guaranteed portals per border"]
6. A chunk's spanning structure is built by an algorithm drawn per chunk, the chunk seed settling the draw over weights that reach generation as inputs; the drawn algorithm takes whatever further parameters it needs from those same inputs. [answer 1.1: "функция выбора алгоритма может брать сида чанка в качестве входного параметра"] [answer 1.1: "а уже выбранный алгоритм добирать остальные параметры (типа growing tree bias) из конфига биома"]
7. Passages beyond the spanning structure, at a share generation takes as an input, so a chunk holds cycles rather than a single path between any two of its cells. [task: "10-20% extra passages for cycles"]
8. Unreachable islands, at a share generation takes as an input. [task: "Unreachable islands are allowed (a single cell walled on six sides, or a closed group) and their share is a biome parameter."] [task: "the island share as a parameter"]
9. The hook a prefab layer plugs into: a prefab-membership decision for a coordinate that precedes fabric generation for it. [task: "The generator must expose the hook prefabs plug into (a prefab check precedes fabric generation, §2.2.3)."]
10. The cost of generating one cell is measurable and reported, and gates nothing: whether a cache is required stays a later decision. [task: "Benchmarks: generating a cell must be cheap enough to call on every step without caching being mandatory."] [answer 1.3: "Report only"]
11. Which maze algorithm runs inside a chunk stops being an open question wherever a live document calls it one. [task: "the open question this issue closes"]

## Out of scope
- Prefab content, the prefab definition format, and the biome configuration that carries the algorithm weights, the per-algorithm parameters and the island share — #28 owns that format; they reach this task as generation inputs, and no key name or schema is fixed here. [task: "Prefabs and the world/biome config — #28."] [answer 1.1: "схема конфига - примерочная, не завязываться на нее"]
- Storing a cell, a face or a chunk anywhere, and materialising a cell on first visit — #29. [task: "Persistence and materialisation — #29."]
- The depth metric (distance to the nearest chat entrance) that the danger formulas read — #29 owns it; only the chunk-grid distance it is measured on lands here. [task: "Persistence and materialisation — #29."]
- What a cell contains as game content — monsters and resources — #34. [task: "Node content (monsters, resources) — #34."]
- Any event or posting signature: this task moves no balance and declares no telemetry. [task: "None — pure computation, no balance movement. Generation timing is a benchmark, not a metric."]

## Deferred
- Whether generation needs a cache in front of it | the cost of one cell is measured and reported, and the decision waits for a real call site to measure against | separate issue needed? yes

## Key decisions
| Question | Decision |
|---|---|
| Which maze algorithm runs inside a chunk? | Not one algorithm: several, each carrying a weight, with the chunk seed drawing between them per chunk. The weights are biome-level, so corridor character is authored per biome rather than fixed once for the game. [answer 1.1: "можно ли задавать вероятность использования того или иного тип алгоритма"] [answer 1.1: "как параметры биома"] [answer 1.1: "что-то вроде весов"] |
| Which algorithms does the draw choose from in this task? | TBD — round-2 question 1. |
| Where do the weights and the per-algorithm parameters come from? | Generation's inputs, carried by the biome configuration #28 defines. The example schema in the answer is illustrative and binds nothing here. [answer 1.1: "схема конфига - примерочная, не завязываться на нее"] [answer 1.1: "а уже выбранный алгоритм добирать остальные параметры (типа growing tree bias) из конфига биома"] |
| What does generation yield for a cell besides its six face states? | A per-cell value derived from the world seed and the coordinate, which the later content layers sample from. No monster, resource or biome content is produced here. [answer 1.2: "Plus cell seed"] |
| What makes being cheap enough to call on every step met? | Nothing gates it. The cost of one cell is measured and reported; whether a cache is required is decided later with that measurement in hand. [answer 1.3: "Report only"] |
| Is connectivity ever repaired after the fact? | No. A repair depends on the order cells were explored, and the world stops being a function of the seed alone. [task: "post-hoc connectivity repair is rejected"] [task: "a repair depends on exploration order, and the world stops being a pure function of the seed"] |
| Where does a face's state live? | Nowhere. It is derived from the world seed and the coordinate whenever it is needed. [task: "faces are not stored"] [task: "they are derived"] |
| Is a distance a hex traversal or a step count on the chunk grid? | The chunk grid, deliberately. [task: "cheap, stable, and BFS precision is not needed"] |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | Under one world seed and one set of generation inputs, what generation yields for a cell is a function of that cell's coordinate alone: a repeat evaluation in the same process, an evaluation in a separate process, and an evaluation reached after generating a different set of cells in a different order all yield the same value. [task: "determinism (same seed, same output, across processes)"] [task: "no dependence on evaluation order"] |
| AC2 | What generation yields for a (world seed, coordinate) pair is unchanged by the Go toolchain version that built the binary and by the architecture it runs on. [task: "it must be stable across Go versions and architectures"] |
| AC3 | The two cells sharing a face agree on that face's state — read from either side it is the same passage or the same wall — for every face of every cell. [task: "generation works from a canonical representation of the face, so both sides agree by construction"] [task: "face agreement from both sides"] |
| AC4 | Every cell has exactly six neighbours, the neighbour relation is symmetric, and each neighbouring pair shares exactly one face, which carries one and the same key computed from either side. [task: "axial coordinates, the six neighbours, the canonical face key"] |
| AC5 | Every cell coordinate belongs to exactly one chunk, and the chunk grid's dimensions are an input to generation rather than a property of the code: under different dimensions a coordinate is generated as a cell of the correspondingly different chunk. [task: "size is config, reference 16x16"] |
| AC6 | The distance this task delivers between two cell coordinates is a function of their two chunks alone: it is zero exactly when both coordinates lie in one chunk, and it counts steps on the chunk grid rather than hexes or passages between the cells. [task: "chunk-grid distance"] [task: "cheap, stable, and BFS precision is not needed"] |
| AC7 | Within any chunk, under any world seed, every non-island cell of that chunk is reachable from every other non-island cell of it through the passages generation yields, with no repair stage anywhere in the path that produced them. [task: "inside a chunk, a spanning structure from the chunk seed (a classic maze algorithm over a finite set — connectivity by construction)"] [task: "post-hoc connectivity repair is rejected"] |
| AC8 | Every border between two neighbouring chunks carries at least one passage through it and at most two, and which faces those are follows from both chunks' seeds, so either chunk generates that border identically. [task: "1-2 guaranteed portals per border"] [task: "with the faces chosen deterministically from both chunks' seeds"] |
| AC9 | Over a region spanning several chunks, every non-island cell is reachable from every other non-island cell of that region through the passages generation yields. [task: "connectivity of the non-island set across a multi-chunk region"] |
| AC10 | A chunk carries more passages than its spanning structure alone requires, at a share generation takes as an input, so more than one path runs between some pairs of its cells. [task: "10-20% extra passages for cycles"] |
| AC11 | A cell outside its chunk's spanning structure has no passage to any reachable cell, and the share of such cells over a multi-chunk region matches the island-share input within a tolerance the spec's verifier states. [task: "Cells outside the spanning structure are the islands."] [task: "island share within tolerance"] |
| AC12 | What generation yields for a cell is settled by that cell's own chunk and the chunks its faces border, and by nothing further off: the number of chunk seeds one cell's generation consults does not grow with how far the coordinate lies from the origin. [task: "computed from the chunk seed plus neighbour chunk seeds for borders"] |
| AC13 | A coordinate the prefab hook claims receives no fabric content, and the membership decision is taken before any fabric for that coordinate is produced; with no prefab registered every coordinate receives fabric. [task: "The generator must expose the hook prefabs plug into (a prefab check precedes fabric generation, §2.2.3)."] |
| AC14 | Which algorithm builds a chunk's spanning structure follows from that chunk's seed and the supplied weights: one chunk is always built by the same algorithm, two chunks of one world may be built by different ones, an algorithm carrying no weight is never drawn, and a change to the weights changes which algorithm a given chunk draws. [answer 1.1: "функция выбора алгоритма может брать сида чанка в качестве входного параметра"] [answer 1.1: "что-то вроде весов"] |
| AC15 | Every parameter a drawn algorithm needs beyond the seed reaches it as a generation input, and changing one changes the structures that algorithm builds. [answer 1.1: "а уже выбранный алгоритм добирать остальные параметры (типа growing tree bias) из конфига биома"] |
| AC16 | Generation yields for each cell, alongside its faces, a value settled by the world seed and that coordinate alone, which varies with the coordinate; no monster, resource or other biome content is produced here. [answer 1.2: "Plus cell seed"] |
| AC17 | One cell's generation cost is measurable on demand from the repository, and no threshold gates it — the caching decision is not taken in this task. [task: "Benchmarks: generating a cell must be cheap enough to call on every step without caching being mandatory."] [answer 1.3: "Report only"] |
| AC18 | No live document still states the choice of intra-chunk maze algorithm as an open question — `docs/DESIGN.md` §16.2 is one such site and does not bound the class, per AGENTS.md § *Propagation Rule* step 4. [task: "the open question this issue closes"] |
| AC19 | TBD — which algorithms the draw chooses from (round-2 question 1). |

## Open questions
- None beyond the TBD rows above, which are this round's question.
