# World generation rework: hexagonal chunks on a super-lattice

**Source:** issue #119
**Date:** 2026-09-14
**Tracked in:** #119

The world-generation core shipped by #27 builds the maze in rhombic chunks. `docs/DESIGN.md` §2.2.2, as amended by #118, describes a different world. Chunks are hexagons of radius R tiling a hexagonal super-lattice, with six equal borders. A chunk's type is an input to its fabric, and a new chunk takes its shared border from a neighbour's stored map. This task reworks the core to match. The core stays a pure computation: storing chunks, the gate spiral and the world configuration format belong to other issues.

## Scope
1. Topology on the hexagonal super-lattice: every cell coordinate corresponds to exactly one pair of a chunk and a local coordinate inside that chunk, and the pair corresponds back to the cell; every chunk has six neighbouring chunks. [task: "a cell coordinate maps to exactly one (chunk, local coordinate) on the hexagonal super-lattice and back"] [task: "a chunk's six neighbours"]
2. The distance between two cells that gameplay formulas read is the hex distance counted in cells. [task: "the hex cell distance as the gameplay metric (D22)"]
3. A chunk is the hexagon of cells within radius R of its centre, and a radius below six is not accepted. [task: "over a hexagonal chunk of radius R, validated R ≥ 6"]
4. Borders and portals: each of a chunk's six borders is 2R+1 faces, enumerated identically from either chunk. The number of portals on a border lies in a range set by two portal shares, and it is drawn from that border's own key. No two portals of one border touch. The rule stops at a three-chunk corner, so a border depends on its two chunks alone. [task: "a border's 2R+1 faces enumerated identically from either side"] [task: "from the border's own key"] [task: "no two portals of one border share a vertex"] [task: "the rule does not extend across a three-chunk corner, so a border stays a function of its two chunks"]
5. Islands are drawn only from cells off the chunk's border, never from the centre of a gate chunk, and under the connectivity guard the shipped core already applies. [task: "candidates are non-border cells, minus the centre when the chunk is a gate chunk (D29, D30), with the existing connectivity guard"]
6. The chunk type is an input to generating a chunk, and a generated chunk map carries an identifier of the generation version that produced it. [task: "Chunk type as a generation input"] [task: "a generation version identifier the stored map can carry (D3)"]
7. A new chunk can be generated against a neighbour's stored map, taking their shared border from that map instead of deriving it again. [task: "the API through which a new chunk takes its shared border from an existing neighbour's stored map instead of recomputing it"]
8. Configuration: the chunk radius replaces the rectangular chunk dimensions in the balance configuration and its schema, and the two portal shares are generation inputs. [task: "the chunk radius key replaces the rectangular dimensions in the balance configuration and its schema"] [task: "the portal shares are generation inputs"]
9. The generation goldens are minted again from the reworked core. [task: "goldens re-minted"]
10. The key decisions recording the shipped core are revised wherever this rework changes what they state. [task: "each is revised where this rework changes it (KD-39's one-or-two portals per border, KD-40's island capacity)"]

## Out of scope
- Storing chunks, creating them lazily, and the per-world lock: #29 owns these. [task: "Storing chunks, lazy creation and the per-world lock — #29."]
- Spiral gate placement and the nearest-gate depth: #120 owns these. [task: "Spiral gate placement and nearest-gate depth — #120."]
- The world and biome configuration format beyond the chunk radius: #28 owns it. [task: "The world and biome configuration format beyond the radius — #28."]
- Prefab regions inside a chunk: post-MVP. [task: "Prefab regions inside a chunk — post-MVP (D11, D28)."]
- Telemetry: this task declares no event and no posting signature. The chunk-created event is #29's. [task: "None — pure computation, no balance movement. The chunk-created event belongs to #29."]

## Deferred
- None.

## Key decisions
| Question | Decision |
|---|---|
| While prefabs are out of the MVP, does the reworked generator still offer a point where a prefab layer plugs in? | TBD |
| Which task defines the form a chunk's map takes in storage, given that the stored form is a persisted data contract? | TBD |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | Every cell coordinate belongs to exactly one chunk, at exactly one local coordinate within distance R of that chunk's centre, and that chunk and local coordinate lead back to the original cell. Every chunk holds exactly the cells within distance R of its centre. [task: "every cell belongs to exactly one chunk"] [task: "a cell coordinate maps to exactly one (chunk, local coordinate) on the hexagonal super-lattice and back"] |
| AC2 | Every chunk has exactly six neighbouring chunks, the neighbour relation is symmetric, and two chunks are neighbours exactly when some face joins a cell of one to a cell of the other. [task: "a chunk's six neighbours"] |
| AC3 | The gameplay distance between two cell coordinates is the number of steps between them on the hex lattice, walls disregarded; which chunks the two cells lie in does not change it. [task: "the hex cell distance as the gameplay metric (D22)"] |
| AC4 | A chunk radius below six is refused and no chunk is generated at it; a chunk radius of six or more is accepted. [task: "over a hexagonal chunk of radius R, validated R ≥ 6"] |
| AC5 | Every border between two neighbouring chunks consists of exactly 2R+1 faces, and both chunks of the border enumerate the same faces in the same order. [task: "a border's 2R+1 faces enumerated identically from either side"] [task: "six equal borders"] |
| AC6 | The number of portal faces on every border lies between the lower and the upper portal share of that border's 2R+1 faces, each product rounded up to a whole face. Both shares are generation inputs. Which count in that range a border carries is settled by the world seed and the border's two chunks. [task: "the portal count stays in range and no two portals of a border touch"] [task: "from the border's own key"] [task: "the portal shares are generation inputs"] |
| AC7 | No two portal faces of one border share a vertex, and this includes two faces of the same cell. How a portal of one border lies against a portal of another border at a three-chunk corner is unconstrained. [task: "no two portals of one border share a vertex"] [task: "the rule does not extend across a three-chunk corner, so a border stays a function of its two chunks"] |
| AC8 | A border's faces, portal or wall, are settled by the world seed, the generation inputs and the border's two chunks, and by nothing else. They do not change with either chunk's type, or with any third chunk. [task: "so a border stays a function of its two chunks"] [task: "a border does not depend on chunk type"] |
| AC9 | Every island cell of a chunk lies off all six of that chunk's borders. In a gate chunk the centre cell is never an island; in a chunk of any other type the centre may be one, as any other cell off the border may. [task: "candidates are non-border cells, minus the centre when the chunk is a gate chunk (D29, D30)"] [task: "the gate centre is never an island"] [task: "Chunk type as a generation input"] |
| AC10 | Within any chunk, every non-island cell is reachable from every other non-island cell of that chunk through that chunk's own passages, and the centre of a gate chunk is one such cell. [task: "with the existing connectivity guard"] [task: "the gate centre is never an island"] |
| AC11 | Over a region of several chunks, gate chunks among them, every non-island cell is reachable from every other non-island cell of the region through generated passages. No stage repairs connectivity after the passages are built. [task: "connectivity over a multi-chunk region"] [task: "post-hoc connectivity repair is rejected"] |
| AC12 | A chunk's generated map is settled by these inputs and by nothing else: the world seed, the chunk coordinate, the chunk type, the generation inputs, and any stored neighbour border it is generated against. The order in which chunks are generated does not change it, and generating it reads no database. [task: "The core stays a pure, database-free function"] [task: "fabric is a function of world seed, chunk coordinate and chunk type"] |
| AC13 | Every chunk map that generation yields names the generation version that produced it. [task: "a generation version identifier the stored map can carry (D3)"] |
| AC14 | When a chunk is generated against a neighbour's stored map, its border with that neighbour carries exactly the stored border's faces, including where those faces differ from what the chunk would derive from the world seed. [task: "the API through which a new chunk takes its shared border from an existing neighbour's stored map instead of recomputing it"] |
| AC15 | The balance configuration and its schema carry a chunk radius where they carried the rectangular chunk dimensions, and they no longer carry those dimensions. [task: "the chunk radius key replaces the rectangular dimensions in the balance configuration and its schema"] |
| AC16 | The generation goldens in the repository record what the reworked hexagonal-chunk generator yields. [task: "goldens re-minted"] |
| AC17 | KD-37 through KD-40 in `ai-docs/key-decisions.md` describe the generation core as this task leaves it. Wherever one of them described the rhombic chunk (its dimensions, its one or two portals per border, its island capacity), it now describes the hexagonal one. [task: "each is revised where this rework changes it (KD-39's one-or-two portals per border, KD-40's island capacity)"] |
| AC18 | No live document describes the world generator's chunks by columns and rows, or its borders as carrying one or two portals. The balance configuration's own comments are one such site, and they do not bound the class; the class is every site whose claim this change falsifies, per AGENTS.md § *Propagation Rule* step 4. [task: "the chunk radius key replaces the rectangular dimensions in the balance configuration and its schema"] [task: "each is revised where this rework changes it (KD-39's one-or-two portals per border, KD-40's island capacity)"] |

## Open questions
- None.
