# Spiral gate placement and nearest-gate depth

**Source:** issue #120
**Date:** 2026-09-14
**Tracked in:** #120

## Scope
1. A spiral order over the chunk super-lattice: deterministic, starting from the centre chunk, with its starting direction and its ring traversal stated. [task: "over the chunk super-lattice from the centre, deterministic, with its starting direction and ring traversal stated"]
2. The next gate: given the set of created chunks, the set of gate chunks and k, the chunk the next chat's gate takes in that world. [task: "from the set of created chunks, the set of gate chunks and k, the chunk the next gate takes"]
3. The nearest-gate distance: the distance in cells from any cell to the nearest gate of any chat in the world — the depth every danger formula reads. [task: "the distance from a cell to the nearest gate that every danger formula reads"]
4. k is an input to the next-gate rule, and a negative k is refused. [task: "k as an input, validated non-negative"]
5. The properties below are exercised by property and table tests; a benchmark, where one exists, reports and gates nothing. [task: "Property and table tests; any benchmark reports and gates nothing."]

## Out of scope
- Persisting gates, the per-world lock and the activation transaction — #29 and #36.
- The chunk-created event carrying the spiral index, ring and k — #29.
- The door price formula (D13) — doors are not in the MVP.
- Boss areas and ruins in the spiral — post-MVP, open in `docs/DESIGN.md` §16.
- Correcting gates that explorers push outward (D9) — the design observes this, it does not correct it.

## Deferred
- None.

## Key decisions
| Question | Decision |
|---|---|
| Where does each ring of the spiral start: at a corner of the ring, or mid-side? | TBD |
| Which lattice direction does the spiral start in, and which way does it turn? | TBD |

## Acceptance Criteria
| # | Criterion |
|---|-----------|
| AC1 | The spiral order lists every chunk of the super-lattice exactly once, starts with the centre chunk, and lists every chunk at super-lattice hex distance n from the centre before any chunk at distance n+1. [task: "k = 0 fills concentric rings"] |
| AC2 | Within each ring, the spiral order starts and turns as recorded under Key decisions (TBD until the owner answers). [task: "with its starting direction and ring traversal stated"] |
| AC3 | With no gate chunks and the centre chunk not created, the next gate is the centre chunk. [task: "the first gate is the centre chunk"] |
| AC4 | The next gate is the first chunk in spiral order that is not a created chunk and lies at super-lattice hex distance of at least k+1 from every gate chunk. [task: "spiral order is respected; a created chunk is never chosen; every two gates are at least k+1 chunks apart"] |
| AC5 | For every finite set of created chunks, every finite set of gate chunks and every non-negative k, a next gate chunk is returned. [task: "the search always terminates, since the world is unbounded"] |
| AC6 | A negative k is refused, and no chunk is returned for it. [task: "k as an input, validated non-negative"] |
| AC7 | For a non-empty set of gates, a cell's nearest-gate distance equals the smallest hex-formula cell distance from that cell to the centre cell of any gate chunk; it is never a path length through the maze's passages. [task: "D22: gameplay distances are in cells by the hex formula, and depth is the distance to the nearest gate"] |
| AC8 | AC7 holds however far the nearest gate lies from the cell, including for a cell many chunks beyond every gate. [task: "finds the true nearest gate however sparse the gates are"] |
| AC9 | The work of computing a cell's nearest-gate distance does not grow with the number of gates far from the cell: gates beyond a stated search bound around the cell add no work. [task: "with cost that does not grow with the number of gates far from the cell"] |

## Open questions
