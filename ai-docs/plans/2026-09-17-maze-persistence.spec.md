# Maze persistence: mazes, lazy chunk creation, gates, discoveries, depth

**Source:** issue #29
**Date:** 2026-09-17
**Tracked in:** #29

The persistent world: a maze, the chunks that are created as players reach them and
whose maps are then stored, the gate each chat is given when it first raids there,
what each player has discovered, and the depth of a cell that every danger formula
reads. The pure generator is #119's and the spiral and distance rules are #120's;
this task is the part that lives in the database and calls into them.

## Scope

1. A maze is a persisted entity carrying its biome, its world seed and its season. [task: "`mazes`: id, biome, world_seed, season"]
2. A chunk of a maze is created whole, on demand, and its map is stored with the generation version it was made at; every later read of that chunk returns the stored map rather than a fresh generation. [task: "**the chunk map is stored**, not regenerated"]
3. A chunk created beside an already-created neighbour shares that neighbour's stored border, and concurrent creations of two neighbours cannot leave the shared border disagreeing. [task: "a new chunk takes its shared border with an existing neighbour from that neighbour's stored map"]
4. A chat activating in a maze — its first raid there — is given exactly one gate in that maze, at the chunk the spiral rule yields for the world as it stands at that moment. [task: "Gate allocation at activation: the next gate chunk from #120 under the same lock, unique per (maze, chat), called by #36"]
5. The depth of a cell is available from the persisted world as the cell distance to the nearest gate of any chat in that maze, and a crafted door can never be one of its inputs. [task: "with crafted doors excluded by construction rather than by a filter someone can forget"]
6. A player's discovery of a cell is recorded, and the record is personal. [task: "`node_discoveries` (cell, who, when) is **personal**"]
7. Chunk creation is observable in telemetry: the event dictionary gains the chunk-creation event, and every created chunk produces one, whatever its cause. [task: "**the chunk-created event** — one event for every cause, with chunk type and creation cause as separate fields"]

## Out of scope

- The radar and the Mini App that read chat knowledge — not MVP.
- Season rotation — not MVP; what this task delivers is the stored season and world seed that let a later rotation be a change of data.
- Monster and trap respawn timers, and node content — #34.
- Doors as a mechanic — the MVP has only the starting entrance.
- Generation itself — #119. The spiral rule and the nearest-gate distance function — #120. The activation, `move` and look edges that call into this task — #36.
- Player-to-chat membership — #30.

## Deferred

_(what | why | separate issue needed? — nothing yet.)_

## Key decisions

| Question | Decision |
|---|---|
| What cell state this task delivers, given that every mechanic that stores any is another issue | TBD |
| What the chat-knowledge view unions over, given that membership is #30's and #30 waits on this issue | TBD |
| What the chunk-creation event carries beyond chunk type and creation cause | TBD |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | A stored maze carries its biome, its world seed and its season, so that changing the season a maze runs under is a change of stored values and not of shape. [task: "the schema carries `season` and `world_seed` so rotation stays a data operation"] |
| AC2 | Asking for the chunk that contains a given cell of a maze yields that chunk, creating it whole first when it does not exist yet. [task: "**a chunk is created whole** when an explorer crosses into it or looks into one of its cells"] |
| AC3 | A chunk's stored map equals what generation yields for that chunk at the generation version the chunk carries. [task: "the check that a stored map equals what generation yields for that chunk at its generation version"] |
| AC4 | Where a newly created chunk meets an already-created neighbour, the two stored maps agree face for face along the shared border. [task: "a new chunk takes its shared border with an existing neighbour from that neighbour's stored map"] |
| AC5 | Two creations of neighbouring chunks that run concurrently leave the two stored maps agreeing face for face along their shared border. [task: "two explorers creating neighbouring chunks concurrently agree on their shared border"] |
| AC6 | Asking for a chunk that already exists yields the stored chunk and leaves its stored map, its chunk type and its creation cause unchanged, however many such requests arrive at once. [task: "check the chunk is absent"] |
| AC7 | A chunk created while serving a caller remains present after that caller's own work fails and is rolled back. [task: "in a short transaction of its own"] |
| AC8 | Activating a chat in a maze yields the gate chunk that the spiral rule yields for the chunks and the gates that exist in that maze at that moment. [task: "the next gate chunk from #120 under the same lock"] |
| AC9 | Activating a chat that already has a gate in that maze yields the gate it already has, and no chat ever holds two gates in one maze. [task: "unique per (maze, chat)"] |
| AC10 | Two activations in one maze that run concurrently yield two distinct gate chunks, each keeping the world's gap k from every other gate in that maze. [task: "two concurrent activations never break the gap k"] |
| AC11 | The depth of a cell in a maze is the cell distance to the nearest gate of any chat in that maze. [task: "**Danger distance is to the nearest gate of any chat; crafted doors do not count**"] |
| AC12 | The gates of the maze are the only stored positions the depth metric reads, so no crafted door can reach it once doors exist. [task: "with crafted doors excluded by construction rather than by a filter someone can forget"] |
| AC13 | A gate allocated after a cell's depth was last read lowers that depth when it is nearer to the cell than every gate that existed before. [task: "a new neighbouring chat genuinely civilises the area, and that is a feature"] |
| AC14 | Recording a player's discovery of a cell yields a record naming the cell, the player and the time, and that record is the player's own rather than the chat's. [task: "`node_discoveries` (cell, who, when) is **personal**"] |
| AC15 | The event dictionary carries one chunk-creation event, and a chunk created by an explorer and a chunk created at a chat's activation each produce one of it, with the chunk type and the creation cause in separate fields. [task: "one event for every cause, with chunk type and creation cause as separate fields"] |

## Open questions

_None._
