# Interview state — Spiral gate placement and nearest-gate depth

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-14-spiral-gate-placement-depth.spec.md
issue_ref: "#120"
gh_issue:
  title: "Spiral gate placement and nearest-gate depth"
  state: open
  labels: ["mvp", "area:world"]
  body: |
    ## What
    
    The pure rule that places a chat's gate in a world, and the distance from a cell to the nearest gate that every danger formula reads. No database: given what already exists in a world, where does the next gate go, and how deep is a cell.
    
    ## Design refs
    
    - `docs/DESIGN.md` §2.2 and §2.2.4 as amended by #118.
    - `docs/world-topology-redesign-plan.md` — D5: the first activated chat takes the centre chunk; every next gate takes the first not-yet-created chunk in spiral order from the centre that is at least k chunks from every gate, gate chunks themselves not counted; k = 0 fills concentric rings. D6: activation is a chat's first raid, separately in each world. D12: the world is unbounded. D22: gameplay distances are in cells by the hex formula, and depth is the distance to the nearest gate. D30: the gate is the chunk centre. D9: gates pushed outward by explorers are observed, not corrected.
    - D24 — the caller runs this under the per-world lock, so the function may assume a consistent snapshot of created chunks and gates.
    
    ## Depends on
    
    #118 #119
    
    ## Scope
    
    - **Spiral order** over the chunk super-lattice from the centre, deterministic, with its starting direction and ring traversal stated.
    - **Next gate**: from the set of created chunks, the set of gate chunks and k, the chunk the next gate takes. Properties: the first gate is the centre chunk; spiral order is respected; a created chunk is never chosen; every two gates are at least k+1 chunks apart; the search always terminates, since the world is unbounded.
    - **Nearest-gate distance** in cells from any cell to the nearest gate, with no BFS, and with cost that does not grow with the number of gates far from the cell.
    - k as an input, validated non-negative.
    - Property and table tests; any benchmark reports and gates nothing.
    
    ## Out of scope
    
    - Persisting gates, the per-world lock and the activation transaction — #29 and #36.
    - The door price formula (D13) — doors are not in the MVP.
    - Boss areas and ruins in the spiral — post-MVP, open in §16.
    
    ## Telemetry obligation
    
    None — pure computation. The chunk-created event, carrying the spiral index, ring and k, is #29's.
    
    ## Open questions to close in the spec
    
    - The spiral's starting direction and ring traversal order — once gates are stored, they fix world identity.
    - The nearest-gate search bound: how far from a cell the search looks, and why that bound finds the true nearest gate however sparse the gates are.
    
    ---
    
    Part of #47. Tracked in #117.
    
  comments: []
  linked_issues: ["#118", "#119", "#29", "#36", "#47", "#117"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: a7879fa54e60232cc
prior_qa:
  - round: 1
    question: "Where should each ring of the gate spiral start? Once k > 0 this decides how many gates fit near the centre and how deep the cells between them are, and it becomes part of the world's identity once gates are stored. Figures cover chunks within ring 30 of the centre, with no chunks created by explorers; depth is the distance to the nearest gate in chunks."
    answer: "Как ты понимаешь, что такое k?"
  - round: 1
    question: "Which lattice direction does the spiral start in, and which way does it turn? Both only rotate or mirror the same world; neither changes packing or depth."
    answer: "Design fixes (Recommended)"
```
