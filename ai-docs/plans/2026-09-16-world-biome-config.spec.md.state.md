# Interview state — world and biome config

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-16-world-biome-config.spec.md
issue_ref: "#28"
gh_issue:
  title: "World and biome config: generation inputs, lexicon, bestiary"
  state: open
  labels: ["mvp","area:world"]
  body: |
    ## What

    A world is an authored entity, not a parameter set: biome plus tone plus its gate-placement parameter. This issue defines the world config format and delivers the one world and one biome the MVP runs on. The prefab layer and the entrance prefab left the MVP with the world topology redesign (#117); in the MVP a chat's gate is a door cell at the centre of a fabric chunk (#118).

    ## Design refs

    - `docs/DESIGN.md` §2.2 — **a world is an authored whole**: biome (resources, monsters) plus tone (narrator vocabulary, location-naming style, bestiary) plus the entrance-placement algorithm. The generator's fabric is procedural; **lexicon and bestiary come from world config**. Tone across worlds is absurdist comedy.
    - `docs/DESIGN.md` §2.2 as amended by #118 — **every chat gets its own entrance into every world**, placed on a spiral from the world's centre when the chat activates in that world (its first raid); #120 owns the rule and #29 the allocation. From home, a door leads into each world through its entrance — every biome is one step away; choosing a biome is choosing a door, not mounting an expedition.
    - `docs/DESIGN.md` §2.2 as amended by #118 — **entrance placement is a parameter of the world**: k, the minimum gap in chunks between gates — k = 0 fills concentric rings, a larger k scatters. A chat still has different neighbours in different worlds, because the activation order differs per world.
    - `docs/DESIGN.md` §2.2.2 as amended by #118 — **the generation inputs a biome supplies**: the chunk radius R (at least 6), the algorithm weights and the growing-tree bias, and the island, extra-passage and portal shares; #119 reads them.
    - `docs/DESIGN.md` §2.2.3 — **two content layers**: fabric and embroidery (prefabs). **Prefabs, the entrance's decoration included, are out of the MVP** (§14 as amended by #118); the entrance-prefab design — five or six variants around the gate, drawn from the chunk seed, never touching the chunk border — stays for later, and nothing here may preclude it.
    - `docs/DESIGN.md` §16.2 as amended by #118 — entrance placement is closed by the spiral and k; prefab authoring stays open and post-MVP.
    - `docs/DESIGN.md` §14 — MVP is **one maze, one biome**.

    ## Depends on

    #18 #118 #119

    ## Scope

    - The world config format: seed; the generation inputs #119 reads — chunk radius R, algorithm weights, growing-tree bias, and the island, extra-passage and portal shares; the gate-placement parameter k #120 reads; biome parameters (resource profile); lexicon and naming style; bestiary. Tracked files, loaded through #18.
    - One complete world/biome config for the MVP, authored to the absurdist-comedy tone.
    - Tests: every generation input and k is validated at start-up, and a rejection names its key (R at least 6, shares in range, k non-negative); the MVP world file loads.

    ## Out of scope

    - The prefab layer, the entrance prefab, boss areas, ruins of old entrances, NPC outposts — not MVP (§14 as amended by #118). **The config format must not preclude them.**
    - Additional worlds beyond the MVP one — the format must support them; the content is later.
    - Gate placement — #120; chunk persistence and gate allocation — #29; the generator — #119.
    - Node content sampling from the bestiary — #34.
    - The narrator that consumes the lexicon — #35.

    ## Telemetry obligation

    None directly; the world identifier is a dimension on `node_entered`, `combat_resolved` and `raid_finished` (§13.4), which #29 and the raid issues emit.

    ## Open questions to close in the spec

    - **Biome resources: `ledger_kind` members, or world config?** Parked for exactly this task in `ai-docs/deferred/_inbox.jsonl:31` — "a biome's vocabulary is world configuration (`docs/DESIGN.md:53`) while a member is a migration; parked with the biome / world-config task". §2.2 (`docs/DESIGN.md:53`) makes a world's resources part of its authored whole, and `ai-docs/deferred/_inbox.jsonl:25` records the other side: **an enum member is permanent after merge** — each arrives by its own `ADD VALUE` migration and never leaves. That asymmetry is what makes a silent decision expensive: with resources as enum members, a world authored later cannot introduce one without a migration. **Owner's call; #34 implements whichever answer this returns.**

    ---

    Part of #47 (MVP roadmap). Reshaped by the world topology redesign (#117).


  comments: []
  linked_issues: ["#18", "#29", "#34", "#35", "#47", "#117", "#118", "#119", "#120"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
