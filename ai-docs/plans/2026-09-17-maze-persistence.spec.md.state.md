# Interview state — maze persistence

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-17-maze-persistence.spec.md
issue_ref: "#29"
gh_issue:
  title: "Maze persistence: mazes, lazy chunk creation, gates, discoveries, depth"
  state: open
  labels: ["mvp", "area:world"]
  body: |
    ## What

    The database side of the world: mazes, lazily created chunks with their stored maps, chat gates, cell state, personal discovery records and the chat-knowledge view, plus the depth metric every danger formula hangs off.

    ## Design refs

    - `docs/DESIGN.md` §2.2.4 as amended by #118 — `mazes`: id, biome, world_seed, season. **A chunk table replaces `nodes`**: the chunk coordinate, chunk type, creation cause, the gate's chat, spiral index, the stored map and its generation version. **Cells are not stored row by row; only cell state is.**
    - `docs/DESIGN.md` §2.2.2 as amended by #118 — **the chunk map is stored**, not regenerated; a new chunk takes its shared border with an existing neighbour from that neighbour's stored map.
    - `docs/DESIGN.md` §2.2 as amended by #118 — **a chat's gate is placed when the chat activates in that world** (its first raid), on #120's spiral rule; the gate is the centre cell of its chunk. Prefab placements are out of the MVP.
    - `docs/DESIGN.md` §2.2.4 — knowledge: `node_discoveries` (cell, who, when) is **personal**; **chat knowledge is the union across members** (a view), and the radar reads it.
    - `docs/DESIGN.md` §2.2.4 as amended by #118 — distances and gradients for the formulas (PvP, waves, experience, monster budget) are **cell distances by the hex formula**, never a BFS. **Danger distance is to the nearest gate of any chat; crafted doors do not count** — a door shortens the road but not the danger; gates do count, so a new neighbouring chat genuinely civilises the area, and that is a feature.
    - `docs/DESIGN.md` §2.2.2 as amended by #118 — **a chunk is created whole** when an explorer crosses into it or looks into one of its cells; look reaches one cell, so it creates at most two chunks at a three-chunk corner.
    - `docs/DESIGN.md` §2.2.2 as amended by #118 — **every chunk creation, by an explorer or at activation, runs under a per-world lock in a short transaction of its own**: check the chunk is absent, read the borders of existing neighbours, generate, insert. Uniqueness on (maze, chunk) and (maze, chat) is a second line, not the mechanism.
    - `docs/DESIGN.md` §2.2 — a maze is static within a season; rotation is a new `world_seed`. Monsters and traps respawn on a timer.
    - `AGENTS.md` § API Stability carve-out — this is a data contract. It changes by forward migration, never by redefinition.

    ## Depends on

    #28 #119 #120

    ## Scope

    - Forward migration for `maze`, the chunk table, cell state and `node_discovery` (singular table names, per the 2026-09-02 decision), plus the chat-knowledge view.
    - Chunk creation under the per-world lock in a short transaction of its own — absent check, the neighbours' stored borders, generation through #119, insert — with a test proving that two explorers creating neighbouring chunks concurrently agree on their shared border.
    - The depth metric: the cell distance to the nearest gate in that maze, through #120, with crafted doors excluded by construction rather than by a filter someone can forget.
    - Discovery recording and the union-by-chat view.
    - Gate allocation at activation: the next gate chunk from #120 under the same lock, unique per (maze, chat), called by #36; a test proving that two concurrent activations never break the gap k.
    - Tests against real Postgres, including both concurrency cases and the check that a stored map equals what generation yields for that chunk at its generation version.

    ## Out of scope

    - The radar / Mini App that reads the knowledge view (§2.4, §11) — not MVP.
    - Season rotation (§10) — not MVP, but the schema carries `season` and `world_seed` so rotation stays a data operation.
    - Monster and trap respawn timers — #34.
    - Doors (§2.3) — MVP has only the starting entrance (§14).
    - The generator — #119; the spiral rule and the nearest-gate distance — #120; the activation, `move` and look edges that call into this issue — #36.

    ## Telemetry obligation

    - Events: `node_entered` (player, chat, maze, depth) — emitted by the raid edge that moves, but this issue owns the depth value it carries.
    - Events: **the chunk-created event** — one event for every cause, with chunk type and creation cause as separate fields (§13.4 as amended by #118). Its other fields — for example the initiating player and chat, the generation version, and per cause the spiral index, ring and k or the exploration trigger — are the spec's to settle. Registered in the §13.4 dictionary this issue's PR extends.

    ## Open questions to close in the spec

    - Whether depth is stored or computed on read — with cell distances there is no per-chunk value to store, and a new gate nearby changes the danger of the area, which the design wants.
    - The lock's form — a row lock on `maze` or an advisory lock — and its ordering against the session row lock when #36 creates a chunk during a raid.
    - The chunk-created event's name.

    ---

    Part of #47 (MVP roadmap). Reshaped by the world topology redesign (#117).



  comments: []
  linked_issues: ["#28", "#34", "#36", "#47", "#117", "#118", "#119", "#120"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
