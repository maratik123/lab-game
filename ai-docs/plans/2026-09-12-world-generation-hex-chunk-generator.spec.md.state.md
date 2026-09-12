# Interview state — world generation core: hex topology and the deterministic chunk generator

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-12-world-generation-hex-chunk-generator.spec.md
issue_ref: "#27"
gh_issue:
  title: "World generation core: hex topology and the deterministic chunk generator"
  state: open
  labels: ["mvp", "area:world"]
  body: |
    ## What

    The maze itself: hexagonal topology, the canonical face representation, and the deterministic chunked generator. Everything about the world hangs off this being a pure function of `(world_seed, coordinate)`.

    ## Design refs

    - `docs/DESIGN.md` §2.2.1 — hex lattice, 2D, planar connectivity (crossing edges are geometrically impossible on hexes). Between neighbours: a passage or a wall. **A face is shared by two cells; generation works from a canonical representation of the face, so both sides agree by construction.**
    - `docs/DESIGN.md` §2.2.1 — **full connectivity is guaranteed by construction, not by probability.** Unreachable islands are allowed (a single cell walled on six sides, or a closed group) and their share is a biome parameter.
    - `docs/DESIGN.md` §2.2.2 — **determinism**: cell content and its faces are `f(world_seed, coord)`. This buys three things: two groups opening the same cell simultaneously resolve by idempotent insert; the world is reproducible for tests; season rotation is a seed change.
    - `docs/DESIGN.md` §2.2.2 — **chunks** (size is config, reference 16x16) are the unit of the connectivity guarantee, two levels: inside a chunk, a spanning structure from the chunk seed (a classic maze algorithm over a finite set — connectivity by construction) plus **10-20% extra passages for cycles** (a pure tree means one path between any two points: boring, and it makes queues); between neighbouring chunks, **1-2 guaranteed portals per border**, with the faces chosen deterministically from both chunks' seeds. By induction the world is connected. Cells outside the spanning structure are the islands.
    - `docs/DESIGN.md` §2.2.2 — **post-hoc connectivity repair is rejected**: a repair depends on exploration order, and the world stops being a pure function of the seed.
    - `docs/DESIGN.md` §2.2.4 — **faces are not stored**, they are derived. Revisit only if non-local portal passages appear.
    - `docs/DESIGN.md` §16.2 — the open question this issue closes: **which maze algorithm runs inside a chunk**.
    - `AGENTS.md` § Code Style — determinism: no `time.Now()`, no map iteration order, no unseeded `math/rand` on this path.

    ## Depends on

    #18

    ## Scope

    - Hex primitives: axial coordinates, the six neighbours, the canonical face key, coordinate-to-chunk mapping, and the **chunk-grid distance** metric (§2.2.4 chooses chunk distance over hex BFS deliberately — cheap, stable, and BFS precision is not needed).
    - The generator: `f(world_seed, coord)` returning cell content and its six face states, computed from the chunk seed plus neighbour chunk seeds for borders.
    - The chosen intra-chunk maze algorithm, the cycle-adding pass, the border portal selection, and the island share as a parameter.
    - Property tests: determinism (same seed, same output, across processes); face agreement from both sides; connectivity of the non-island set across a multi-chunk region; island share within tolerance; no dependence on evaluation order.
    - Benchmarks: generating a cell must be cheap enough to call on every step without caching being mandatory.

    ## Out of scope

    - Prefabs and the world/biome config — #28. The generator must expose the hook prefabs plug into (a prefab check precedes fabric generation, §2.2.3).
    - Persistence and materialisation — #29.
    - Node content (monsters, resources) — #34.

    ## Telemetry obligation

    None — pure computation, no balance movement. Generation timing is a benchmark, not a metric.

    ## Open questions to close in the spec

    - Which maze algorithm inside a chunk (§16.2), and why — the choice shows up in how corridors feel.
    - Chunk seed derivation from world seed and chunk coordinate, and the hash it uses (it must be stable across Go versions and architectures).

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#18", "#28", "#29", "#34", "#47"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: ac11c6eef428a2106
prior_qa: []
```
