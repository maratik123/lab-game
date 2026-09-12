# Design: World generation core — hex topology and the deterministic chunk generator

**Issue:** #27
**Date:** 2026-09-12

## Approach

### Shape of the deliverable

New packages, no call site, no configuration key, no migration, no telemetry.

- **`internal/hexgrid`** — the topology vocabulary: the axial cell coordinate, the six face
  directions and the opposite relation, the canonical face, the chunk coordinate, the chunk
  grid's dimensions, the coordinate→chunk mapping, and the chunk-grid distance. It knows nothing
  about seeds, mazes or content.
- **`internal/maze`** — generation: seed derivation, the pinned draw reductions, the generation
  inputs, island selection, the algorithm draw, the algorithm implementations, the extra-passage
  pass, border portals, the prefab hook, and the per-coordinate entry point.

**Why the topology is its own package.** The chunk-grid distance is what the danger formulas read —
`docs/DESIGN.md` §2.2.4 puts the PvP gradient, the wave multiplier, the experience boost and the
monster budget on the chunk grid rather than on a hex traversal, and #29 owns the depth metric built
on it — and none of those consumers should acquire the generator through their distance metric. A
coordinate is also the vocabulary doors, discoveries and trails speak; the generator is one consumer
of it, not its owner. The direction of the dependency is `maze → hexgrid`, never back
`[derived → go build ./..., which refuses an import cycle]`.

**Why the package is not named `hex`.** `encoding/hex` occupies that name, and a determinism-path
package whose import needs an alias wherever a digest is rendered is a name that invites the
mistake. `hexgrid` names the thing without the clash.

**No configuration in this task.** The generation inputs arrive as a plain `maze.Params` value the
caller fills. The spec puts the biome file that carries the weights, the per-algorithm parameters
and the island share in #28 and fixes no key name here, so `internal/config` is untouched — even
though the chunk grid's dimensions already have keys there
`[measured 46ee531:internal/config/balance_load.go:113-114 · grep -n "world.chunk" → entry("world.chunk.cols", bindInt(&b.World.Chunk.Cols, positiveInt, "positive")), entry("world.chunk.rows", …)]`.
Mapping those keys (and #28's new ones) onto `Params` belongs to the composition root, with #28.
Consequence worth stating: `internal/maze` imports `internal/hexgrid`, the standard library and
`shopspring/decimal`, and nothing else of this module
`[measured 46ee531:go.mod:15 · grep -n shopspring go.mod → github.com/shopspring/decimal v1.4.0]`.

### Determinism: what makes `f(world_seed, coord)` survive a toolchain

Each property below has a mechanism behind it rather than a promise.

1. **Every hashed input is encoded at a fixed width, big-endian, behind a domain tag.** The
   preimage is built by one helper per width, each reinterpreting a signed value's
   two's-complement bits. No `int`-width value and no reflection-driven encoder ever reaches a
   hash. Measured trap the implementor will hit: `gosec` rejects the reinterpretation itself, and
   a `//nolint:gosec` naming G115 and its reason clears the gate
   `[measured 46ee531:.golangci.yml:31 · golangci-lint run over a scratch package in tmp/ containing binary.BigEndian.PutUint64(buf[:], uint64(seed)) → "G115: integer overflow conversion int64 -> uint64 (gosec)", exit 1; the same file with the conversion wrapped in a helper carrying //nolint:gosec and a reason → "0 issues.", exit 0]`.
2. **Key derivation is a frozen standard.** `crypto/sha256` over that preimage. SHA-256 is a
   published specification with test vectors, so the digest for a preimage is not a property of
   the toolchain. The same run measured that `gosec` raises no weak-randomness finding for a
   seeded `math/rand/v2` source, so no suppression is needed for the stream itself
   `[measured 46ee531:.golangci.yml:31 · the golangci-lint run above over a file calling rand.NewChaCha8 and (*ChaCha8).Uint64 → the only finding was G115; no G404]`.
3. **The stream is a specified generator, and the reduction from it is ours.**
   `math/rand/v2.NewChaCha8(key [32]byte)` supplies the `uint64` stream. ChaCha8Rand has a
   published C2SP specification with test vectors, written so other implementations "share
   repeatability with the Go implementation for a given seed"
   (<https://go.dev/blog/chacha8rand>, <https://c2sp.org/chacha8rand>) — but **neither page
   promises stability across Go releases**, and the `math/rand/v2` documentation makes no
   stability statement in either direction
   `[measured go1.26.5 · go doc math/rand/v2 → package synopsis carries no compatibility or reproducibility clause]`.
   So the stream primitive is chosen for being specified, and **the golden in § Test Design is what
   actually gates it.** Every reduction from the stream — bounded draw, shuffle, weighted pick — is
   implemented and documented in this package, never taken from `rand.Rand`, so a change to the
   standard library's mapping from source to value cannot move a single face. That also removes a
   panic surface: the standard library's bounded draw panics on a non-positive bound
   `[measured go1.26.5 · go doc math/rand/v2.Rand.IntN → "IntN returns … in the half-open interval [0,n). It panics if n <= 0."]`,
   which the project's zero-production-panic posture refuses
   `[measured 46ee531:ai-docs/panic-index.md · cat → the table's only row is "| — | — | — |"]`.
   Ours is total: a bound of one or zero returns zero and consumes nothing from the stream.
4. **One stream per purpose, domain-separated.** The algorithm draw, island selection, the
   spanning structure, the extra-passage pass and each border's portals each key their own
   `ChaCha8` from their own tagged digest. A pass whose draw count changes therefore cannot shift
   another pass's values — which is what makes the five algorithms, each consuming a different
   number of draws, interchangeable without disturbing the island set or the portals.

The domain tag carries a version marker. **Changing it re-mints every world**, so it is part of a
world's identity and not a refactor; the golden pins it for exactly that reason.

### Locality: what one cell's generation consults

`Cell(coord)` needs the interior faces of `coord`'s own chunk, and — for each face that leaves that
chunk — the portal decision for the border it crosses. A border's portals are a function of the
world key and the **ordered pair of chunk coordinates** alone: they never consult either chunk's
interior maze. So the chunk keys one call consults are exactly
`{ChunkOf(coord)} ∪ {ChunkOf(coord.Neighbor(d)) : d over the six directions}`, whose size is bounded
by the lattice's face count and is the same beside the origin as a million cells away. That set is
computed by one function which `Cell` itself uses, so the assertion in § Test Design is about the
real path rather than about a restatement of it `[derived → AC12]`.

**This is what fixes the island rule.** An island whose face carried a portal would be reachable,
and deciding a portal around the neighbour's island set would make one cell's generation consult
the neighbour's neighbours' keys. Both are avoided by one constraint: **a cell with any face on a
chunk border is never an island.** Islands are drawn from the chunk's interior cells only, so every
border face has two non-island endpoints and the portal guarantee needs no knowledge of the other
side beyond its key. The visible consequence — islands never touch a chunk boundary — is accepted;
the alternative makes each neighbour's island set depend on that neighbour's own borders, so the key
set for one cell grows by a further neighbour layer and the work for one cell grows by a whole chunk
build per neighbour.

### The per-chunk construction, in order

Everything below is a pure function of `(world seed, chunk coordinate, Params)`.

1. **Island set.** Target count is `round(islandShare × cellsInChunk)`. Candidates are the chunk's
   non-border cells in index order, shuffled by the island stream. Each candidate is taken
   tentatively and accepted only if the chunk's remaining non-island cells are still connected
   **on the lattice** (a flood fill from the chunk's first border cell); otherwise it is skipped.
   The walk stops at the target or when candidates run out.
   *Why a guard rather than an argument:* removing cells from a triangular lattice can enclose a
   pocket, and a pocket unreachable from the rest breaks the intra-chunk connectivity criterion.
   The guard makes connectivity a property the construction *checks while building*, not one the
   result is hoped to have — and it is not the rejected post-hoc repair, which is rejected for
   depending on **exploration order**: this depends on the chunk key alone.
   *Rejected alternative:* islands as iteratively-pruned leaves of the finished spanning tree
   (connectivity then holds trivially, since a subtree stays a tree). Rejected twice over — the
   reachable island count becomes whatever that tree's prunable-leaf supply happens to be, so the
   share input stops being honoured, and the island set becomes a function of the drawn algorithm,
   which couples the island criterion to the algorithm criterion and makes both harder to test.
   *Cost, accepted:* the flood fill runs once per candidate. The local removability test from
   digital topology would make it constant-time per candidate; it is not taken, because nothing
   gates the cost here and a topological argument is a worse thing to rest on than a flood fill.
2. **Algorithm draw.** A weighted pick over the algorithm enum's **canonical order** — never over a
   map's iteration order — from the algorithm stream. Zero weight is unreachable by construction.
3. **Spanning structure** over the non-island cells, by the drawn algorithm, over interior faces
   only, from the structure stream.
4. **Extra passages.** `round(extraPassageShare × (nonIslandCells − 1))` — a share *of the spanning
   structure's own edge count*, which is what "additional passages" means — drawn from the interior
   faces that are still walls and have two non-island endpoints, in canonical order, shuffled by
   the cycle stream, capped by how many such faces exist.
   *Rejected alternative:* a share of the remaining closed faces. Same number, denser result,
   because the lattice's face count grows faster than the tree's edge count; it would make the
   design's reference share mean something other than what the design says.
5. **Border portals**, per border, from that border's stream: a count of one or two, then that many
   distinct faces from the border's candidate list, capped by the candidate count. The candidate
   list is always enumerated **from the lexicographically lesser of the two chunk coordinates**,
   over its border cells × directions, keeping the faces whose destination chunk is the other one —
   so both sides build the same ordered list. Enumeration is by destination chunk and not by
   direction, because more than one direction crosses the same border.
   The extra-passage pass never touches a border face, which is what keeps a border at no more than
   two passages.
   **The cap is not a degenerate-case afterthought: two of a chunk's six borders always have a
   single candidate face, at every dimension.** Reaching the chunk at delta `(+1,−1)` needs a
   neighbour with `q` past the chunk's last column *and* `r` before its first row, which only the
   corner cell at (last column, first row) achieves and only through the `(+1,−1)` direction;
   the `(−1,+1)` border is its mirror. Those two borders therefore carry exactly one portal always,
   while the four remaining borders have a candidate for most of their edge cells in two directions
   each. Verified by enumerating every face leaving a chunk at the reference dimensions
   `[measured probe of this design's own tiling rule, no tracked file involved · a scratch
   enumeration over cols=rows=16 of all six directions from every chunk cell, grouped by
   destination chunk → "border to chunk (1, -1) candidates: 1 [(15, 0, 1)]", "border to chunk
   (-1, 1) candidates: 1 [(0, 15, 4)]", and 31 for each of (1,0), (-1,0), (0,1), (0,-1)]`.

For a coordinate no prefab claims — the claimed case is § The prefab boundary below — a cell's six
faces then read: for each direction, the canonical face; if both its cells are in this chunk, the
interior face's state; otherwise the border's portal membership. An island's faces are all walls,
and no neighbour of an island ever carves into it, so agreement from both sides needs no special
case `[derived → AC3]`.

**Numeric types and rounding, because they fix world identity too.** The shares and the bias are
`decimal.Decimal` — arbitrary-precision, so exact and architecture-independent — and **no
floating-point type appears in either package**, which a guard holds structurally. `round(x)` in the
island-count and extra-passage expressions above means `floor(x + ½)`: multiply exactly, add one
half, take the floor, then convert to `int`. The bias becomes `floor(bias × 2^32)` as a `uint64`,
compared **strictly less than** against the stream's top thirty-two bits, so a bias of zero never
selects the newest entry and a bias of one always does, with no overflow branch. Naming the rounding
is not pedantry: an exact-half case resolves one way under half-away-from-zero and the other under
half-to-even, and either choice is a different world under the golden `[derived → AC1, AC2]`.

### The prefab boundary: what a claimed coordinate carries

`docs/DESIGN.md` §2.2.3 fixes two things about a prefab, and a naive reading of the first breaks the
second: the generation function checks prefab membership **before** it generates fabric, *and* a
prefab's border portals follow the **general** rules, so a prefab is stitched into the world
connectedly for free. A claimed coordinate that carried nothing at all would honour the first and
destroy the second — and would break face agreement outright, because the cell across a chunk border
reads that border's portal from the two chunk keys and knows nothing of any claim. So the split
follows §2.2.3's own line:

| A claimed coordinate's… | Who decides it |
|---|---|
| interior faces (both cells in one chunk) | the prefab layer — the generator reports the deferred state and builds no chunk fabric for the coordinate |
| border faces (the two cells lie in different chunks) | **the portal rule, unchanged** — the very value the unclaimed cell across that border reads |
| cell seed | nobody here: a claimed coordinate receives none |

`FaceState` therefore carries a third member, **`FaceDeferred`, as its zero value**. A claimed
cell's interior face is then honest rather than silently a wall: a wall is a claim about the world
that the prefab's authored interior may contradict, and it would be indistinguishable from a real
one. The prefab marker on the result is the discriminator, so a claimed coordinate's zero cell seed
needs no sentinel.

**Granularity is the hook's contract, not the generator's check.** §2.2.3 places a prefab over a
chunk or a group of chunks. The generator does not verify that, because verifying would mean asking
the hook about every cell of a chunk on every call; the interface's doc comment states the
precondition and names the prefab layer that implements it as the guarantor, per the workspace's
unchecked-precondition rule. **Within that contract, face agreement is total** — an interior face of
a claimed chunk has claimed cells on both sides and both report the deferred state; a border face of
a claimed chunk is the portal rule on both sides — which is why the agreement sweep is run with a
whole-chunk claim registered and not only against a nil hook. Break the contract by claiming part of
a chunk and the unclaimed cells of that chunk read interior faces carved as though the claimed cell
were fabric: that is the disagreement the contract exists to exclude, and it is stated here rather
than absorbed `[derived → AC3, AC13]`.

**#28 needs no accessor added here.** The portals a prefab must stitch to are exactly the border
faces `Cell` already returns for the claimed coordinates on its chunk's border ring.

### The algorithm set, and what each weight buys

The spec fixes the set and the owner's round-4 answer fixes the standard they are held to: five
genuinely different implementations, "даже если при кравевых условиях дают похожий результат".
Each is its own function with its own traversal discipline, sharing only the graph accessors and
the draw helpers; **no statistic is asserted to separate them** (see § Open questions for the
owner's round-3 question this answers).

| Algorithm | Discipline | Corridor signature |
|---|---|---|
| Backtracker | depth-first with an explicit stack (never recursion — an explicit stack has no depth to exhaust) | "low branching factor and … many long corridors" |
| Kruskal | shuffle the candidate faces, accept a face that joins two components | "tends to produce regular patterns which are fairly easy to solve"; many short dead ends |
| Prim, frontier-based | grow outward from one cell, picking a random frontier cell and joining it to a random in-tree neighbour | "tend to branch slightly more than the edge-based version" |
| Growing tree | one active list, the bias choosing between its newest entry and a random entry | at always-newest it *is* the backtracker, at always-random it *is* Prim's, and a mix "generate[s] mazes with attributes of both" |
| Wilson's walk | loop-erased random walks into the growing tree | "an unbiased sample from the uniform distribution over all mazes" |

Sources: <https://en.wikipedia.org/wiki/Maze_generation_algorithm> for the backtracker, Kruskal,
frontier-Prim and Wilson rows;
<https://weblog.jamisbuck.org/2011/1/27/maze-generation-growing-tree-algorithm> for the growing-tree
row. The growing-tree row is why the owner's "similar result at edge conditions" answer was the
right one to give: the family's extremes coincide with two of its siblings by construction, and
that is a property of the literature, not a defect in the implementation.

Component tracking for Kruskal is a component-label array over the chunk's cells with
relabel-on-merge, inside that one function — not an imported disjoint-set structure and not a
shared helper: this design gives it one call site, so the shared-package threshold the workspace
sets for replicated helpers is not reached. If a second algorithm or a later task needs it, it is
lifted then, not pre-emptively.

**Per-algorithm parameters.** The growing tree takes the bias; the other four take nothing beyond
their stream (Wilson's start cell is drawn, not configured). The bias reaches the draw as a pinned
threshold: `floor(bias × 2^32)` compared against the stream's top thirty-two bits, so a bias of one
selects the newest entry on every step with no overflow branch and no floating-point comparison
`[derived → AC15]`.

### Why not a library

| Candidate | Why it loses |
|---|---|
| `github.com/itchyny/maze` | Rectangular grids only; one algorithm, not a selectable set; no generation over a cell subset `[measured pkg.go.dev/github.com/itchyny/maze · WebFetch → "Hexagonal grids (rectangular only)"; "Named algorithm selection (single implementation)"; "Custom cell subset generation" — not supported]` |
| `gitlab.com/zaba505/maze` | Rectangular only; carries Kruskal and Prim, none of backtracker, growing tree or Wilson `[measured pkg.go.dev/gitlab.com/zaba505/maze · WebFetch → "Hexagonal grids: Not supported (rectangular mazes only)"; "Supported: Kruskal, Prim. Missing: backtracker, growing tree, Wilson"]` |
| `gonum.org/v1/gonum/graph/path` | Minimum spanning trees from edge weights only; the frontier Prim, the growing tree and Wilson's walk are not expressible as an MST over weights `[measured pkg.go.dev/gonum.org/v1/gonum/graph/path · WebFetch → "Kruskal generates a minimum spanning tree of g by greedy tree coalescence"; "No support exists for … Uniform spanning tree generation, Loop-erased random walk (Wilson's algorithm), Frontier-based Prim maze generation, Randomized or seedable variants"]` |

Each fails on the lattice and on the algorithm set, and every one of them would additionally have
to expose a stream this package can pin to satisfy stability across toolchains — which none
documents. That is the "API cannot express the requirement" argument, made against named packages
rather than against the idea of a dependency.

**The escape hatch, which an argued wheel owes** (the scheduler decision's model names River for
exactly this reason). Two exits, in order of cost. If an algorithm proves wrong in play — too
regular, too costly, too many dead ends — it is **weighted to zero in the biome file**: a
configuration change, no code, already the mechanism the owner's own example weight set used, and
the reason the weighted draw is worth more than a fixed choice. If the hand-rolled set proves wrong
as a *whole*, the lift target is the graph-library route already evaluated above: `gonum`'s graph
interfaces can host the Kruskal and Prim bodies the day it exposes a seedable spanning-tree variant,
behind the same `Algorithm` enum and the same `Params`, because the enum and the weights are the
public surface and the traversal bodies are not. Neither exit touches a caller.

### API sketch

```
hexgrid: Coord{Q,R int32} · Direction (six members) · Direction.Opposite · Coord.Neighbor
         Face{Cell Coord; Dir Direction}  (Dir is always the earlier of the two opposite
                                           directions, so the face from either side normalises
                                           to one identical value)
         Chunk{Q,R int32} · Dims{Cols,Rows int32} · Dims.ChunkOf · Dims.Origin · Dims.Contains
         ChunkDistance(a,b Chunk) int64 · Dims.Distance(a,b Coord) int64

maze:    Seed int64 · Algorithm (the five) · Params · Cell{Prefab bool; Faces; Seed uint64}
         FaceState (FaceDeferred as the zero value | Wall | Passage)
         PrefabClaimer interface{ Claims(hexgrid.Coord) bool }
         New(Seed, Params, PrefabClaimer) (*Generator, error)
         (*Generator).Cell(hexgrid.Coord) Cell
         unexported: stream interface{ Uint64() uint64 }  — see § Standing constraints
```

Decisions inside that sketch:

- **`int32` coordinates, `int64` distance.** Fixed width is what makes the hashed preimage
  toolchain-independent, and `integer` is what #29 will persist. Distance is returned wider because
  the difference of two extreme chunk coordinates does not fit the narrower type. Coordinate
  arithmetic at the domain's extremes wraps, which Go defines, so there is no panic path there.
- **A constructor that validates and a method that cannot fail.** `New` rejects a bad input naming
  it (non-positive dimension; a weight set with no positive weight; an unknown algorithm key; a
  share or bias outside its range; an island share above what the given dimensions can hold, which
  is derived from the dimensions and not a literal). `Cell` then returns a value with no error and
  no panic. A share above one is rejected as a units mistake rather than clamped.
- **`Generator` is immutable after `New` and holds no memo.** The caching question is deferred by
  the spec, and a memo would silently convert the order-independence criterion into a
  cache-correctness question. It is therefore safe for concurrent use, asserted under `-race`.
- **The prefab hook is a one-method consumer-declared interface returning a bare `bool`.** #28 owns
  prefab identity, so no identifier type is fixed here. A claimed coordinate yields a `Cell` marked
  as a prefab's, with its interior faces deferred, its **border faces carried from the portal rule**
  and no cell seed — the split § The prefab boundary sets out. A nil hook means every coordinate
  receives fabric, and nothing anywhere is deferred.
- **The cell seed is derived from the world seed and the coordinate alone** — not from the chunk
  key, not from the dimensions. So the same world seed under different chunk dimensions yields the
  same cell seed and different faces, which is the sharp form of the criterion
  `[derived → AC16]`.
- **Nothing is exported for a test's benefit.** The algorithm draw and the chunk build stay
  unexported and are asserted from in-package tests.

### Standing constraints this design was checked against

- `exhaustive` is enabled with `default-signifies-exhaustive: true`, so every switch over the
  direction and algorithm enums is total or carries a default
  `[measured 46ee531:.golangci.yml:20,44-45 · grep -n "exhaustive\|default-signifies" .golangci.yml → "- exhaustive # every FSM state / enum switch is total", "exhaustive:", "default-signifies-exhaustive: true"]`.
- `revive`'s `exported` and `package-comments` rules are on, so every exported item carries a
  name-first doc comment and each new package carries a `doc.go`
  `[measured 46ee531:.golangci.yml:65-68 · sed -n '65,68p' .golangci.yml → "revive:", "rules:", "- name: exported", "- name: package-comments"]`.
- `nolintlint` requires a specific linter and a reason, which the encoding helpers' G115
  suppressions carry
  `[measured 46ee531:.golangci.yml:62-64 · grep -n "nolintlint\|require-explanation\|require-specific" .golangci.yml → "nolintlint:", "require-explanation: true", "require-specific: true"]`.
- No comment in either package names a markdown path, a design-section sign, an acceptance-criterion
  id, an issue number outside `TODO(#…)`, or a URL — so the specification URLs above live in
  this document and in the pull-request body, and the doc comments state the property
  self-containedly `[derived → make comment-refs over the staged set]`.
- **The stream is held behind an unexported one-method interface (`Uint64() uint64`), not as a
  `*rand.ChaCha8` field.** This is what makes the guard below satisfiable: the guard permits exactly
  one identifier from `math/rand/v2` — the constructor — and a struct field typed with the concrete
  type would name a second one, at which point the implementor's cheapest exit is to loosen the
  guard rather than to fix the design. Declaring the interface in the consuming package is also what
  the naming rules ask for, and it makes a fake stream available to the reduction tests.
- This task moves no balance, writes no posting, declares no event and touches no persisted enum, so
  the ledger and telemetry obligations and the forward-migration rule are not engaged. Verified
  against the invariants page rather than assumed
  `[measured 46ee531:ai-docs/domain-invariants.md:40,57,80 · grep -n maze ai-docs/domain-invariants.md → the raid session's maze_id column, the §13.4 event dimensions, and the non-obligations paragraph; no world-generation obligation]`.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | `internal/hexgrid`: the axial coordinate, the six directions and `Opposite`, the canonical `Face`, `Chunk`, `Dims`, the **floor-division** `ChunkOf` with `Origin`/`Contains`, `ChunkDistance` and `Dims.Distance` — with its table tests, its leak-check `TestMain`, and its own structural guard (no clock, no unseeded randomness, no floating-point type, no map ranging), each half paired with a scratch-package red case | `internal/hexgrid/doc.go`, `internal/hexgrid/coord.go`, `internal/hexgrid/face.go`, `internal/hexgrid/chunk.go`, `internal/hexgrid/main_test.go`, `internal/hexgrid/coord_test.go`, `internal/hexgrid/chunk_test.go`, `internal/hexgrid/guards_test.go` | — |
| 2 | `internal/maze` derivation core: the per-width fixed-width preimage helpers with their G115 suppressions, the domain tag, the world key, the cell key and cell seed, the chunk key, the border key, the unexported one-method stream interface the ChaCha8 constructor is the only producer of, and the pinned reductions (total bounded draw, shuffle, weighted pick) — with unit tests over a fake stream and a golden over the derived keys | `internal/maze/doc.go`, `internal/maze/seed.go`, `internal/maze/draw.go`, `internal/maze/main_test.go`, `internal/maze/seed_test.go`, `internal/maze/draw_test.go`, `internal/maze/testdata/derive.golden` | 1 |
| 3 | `Params` with its validation (including the decimal shares and the bias, and the rounding the design pins for each), the `Algorithm` enum with its canonical order, the `FaceState` enum with the deferred state as its zero value, and the weighted per-chunk algorithm draw | `internal/maze/params.go`, `internal/maze/algorithm.go`, `internal/maze/params_test.go`, `internal/maze/algorithm_test.go` | 2 |
| 4 | The chunk cell graph (index mapping, six-neighbour adjacency, border-cell predicate, interior-face indexing) and island selection with its lattice-connectivity guard | `internal/maze/chunkgraph.go`, `internal/maze/island.go`, `internal/maze/chunkgraph_test.go`, `internal/maze/island_test.go` | 3 |
| 5 | The algorithm implementations over the non-island induced subgraph: backtracker, Kruskal, frontier Prim, growing tree with its bias threshold, Wilson's walk | `internal/maze/algorithms.go` (split by algorithm if it passes the soft size limit), `internal/maze/algorithms_test.go` | 4 |
| 6 | The extra-passage pass and border-portal selection, including the canonical-lesser-chunk candidate enumeration | `internal/maze/cycles.go`, `internal/maze/portal.go`, `internal/maze/cycles_test.go`, `internal/maze/portal_test.go` | 5 |
| 7 | `Generator`, `New`, `Cell`, the chunks-consulted function `Cell` itself uses, and the `PrefabClaimer` hook with the prefab boundary § Approach sets out — the claim checked before any chunk build, interior faces deferred, **border faces still carried from the portal rule**, no cell seed, and the whole-chunk granularity stated as the interface's precondition with its guarantor named | `internal/maze/generate.go`, `internal/maze/prefab.go`, `internal/maze/generate_test.go`, `internal/maze/prefab_test.go` | 6 |
| 8 | The property suite (the agreement sweep run both with a nil hook and with a whole-chunk claim), the cell golden with its mint flag, and the package's structural guards (determinism imports, no floating-point type, the single permitted `math/rand/v2` reference, no map ranging, the connectivity helper confined to island selection, the key derivations confined to the fabric and portal builders) | `internal/maze/property_test.go`, `internal/maze/golden_test.go`, `internal/maze/guards_test.go`, `internal/maze/testdata/cells.golden` | 7 |
| 9 | The benchmarks: one cell, and one per algorithm under a single-weight input | `internal/maze/bench_test.go` | 7 |
| 10 | Close the open question in the design corpus and record the engineering decisions: strike the intra-chunk maze-algorithm choice from the open-question list (`docs/DESIGN.md` §16.2, item 2) and record — in §2.2.2, **confined to recording the owner's interview decision and nothing more**: the per-chunk weighted draw over the decided algorithm set, with the weights biome-level — both edits **in Russian**, since `docs/**` is Russian by the workspace's own rule and is not to be translated. Anything beyond recording that decision would be redesigning the corpus and is out of scope. Then add the key decisions (the derivation chain and its domain tag, the topology/generator package split, the island-and-border rule, and what each share input is a share *of*); then sweep every live document, case-insensitively, for the same open-question claim | `docs/DESIGN.md`, `ai-docs/key-decisions.md`, plus whatever the sweep finds | — |

## Handoff plan

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The first group is entered through a handoff like every other.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–9 (code change-type: `*.go` and their `testdata`). Non-terminal; within the
  `≤ 10` size cap, homogeneous, and the whole code change-type clustered into one group rather than
  interleaved with the documentation subtask.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task` resumes
  in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent, 1M-token window —
  subtask 10 (instructions/harness change-type: `*.md`, `ai-docs/**`). Terminal group (1 subtask;
  within the `1..=10` range).

Two groups, within the default maximum of four; no user gate needed.

## Risks

- **Truncating division silently merges two chunks.** Go's `/` truncates toward zero, so a naive
  `q / cols` puts the coordinate just below the origin in the origin's own chunk and destroys the
  partition. Mitigation: floor division in `ChunkOf`, and a test whose coordinate table straddles
  zero in both axes and both signs — `[derived → AC5]`.
- **Distance overflows the coordinate width.** The difference of two chunk coordinates at opposite
  extremes does not fit `int32`. Mitigation: the distance computation widens before subtracting and
  returns the wider type — `[derived → AC6, with an extreme-coordinate case]`.
- **A bounded draw with a zero bound hangs instead of panicking.** A rejection loop masked to a
  zero-width window never terminates, which is worse than the panic the standard library would
  raise. Mitigation: the total form returns zero and consumes nothing for a bound of one or zero,
  with the precondition and that answer stated in its doc comment, and a unit test pinning both the
  value and the untouched stream — `[derived → the draw-helper test in § Test Design]`.
- **Map iteration order reaching a drawn value.** The weight set is naturally a map keyed by
  algorithm, and ranging it would make the draw order-dependent. Mitigation: the draw walks the
  enum's canonical order and only reads the map; a structural guard forbids ranging over a map in
  either package's non-test files, with no carve-out — `[derived → AC14 and the guards subtask]`.
- **The ChaCha8 stream is specified but not promised stable across Go releases.** Nothing in the
  toolchain's documentation commits to it
  `[measured go1.26.5 · go doc math/rand/v2 → no compatibility clause]`, so the design does not
  rest on the promise: the committed golden fails the suite on any toolchain, architecture or
  library change that moves a face — `[derived → AC2]`.
- **Wilson's walk has no worst-case bound.** A loop-erased walk's length is not bounded by the
  chunk's size, so the slowest chunk in a world is a Wilson chunk. Nothing gates the cost in this
  task; the benchmark reports it per algorithm so the deferred caching decision has the spread and
  not an average, and the operator's escape hatch already exists — a zero weight makes it
  unreachable, which is what the owner's own example weight set already did
  (`answer 1.1`: `weights:{… wilson_walk: 0}`) — `[derived → AC17's per-algorithm benchmark]`.
- **Degenerate chunk dimensions behave differently from each other, and conflating them prescribes
  a wrong expected outcome.** "Every cell is a border cell" does **not** imply "no interior face
  exists" — two border cells of one chunk share an interior face. Measured: a single-cell chunk has
  no interior face at all, while a single-row chunk at the reference column count has an interior
  face between each adjacent pair along the row
  `[measured probe of this design's own tiling rule, no tracked file involved · a scratch
  enumeration counting, for cols=16 rows=1, the directed face slots whose destination cell is in
  the same chunk → "directed interior face slots = 30 -> undirected interior faces = 15"; for
  cols=rows=1 → "directed interior face slots = 0"]`. So the expectations
  are per case: **1×1** — no interior face, no island, no spanning edge, no extra passage, all six
  faces decided by the portal rule. **Single row or single column** — interior faces one fewer than
  the cells, the spanning structure is the forced path that opens *all* of them, no island (every
  cell is a border cell), and therefore no closed interior face survives, so extra passages cap at
  none. **Reference dimensions** — the criteria as stated. Mitigation: a case per shape, with the
  right expectation in each — `[derived → AC5, AC7, AC10, AC11]`.
- **The island share can be unreachable even at healthy dimensions.** The connectivity guard skips
  a candidate whose removal would enclose a pocket, so the achieved count can fall below the
  target. Mitigation: the achieved share is what the tolerance is asserted against, and the
  failure message reports the achieved value rather than only the verdict —
  `[derived → AC11]`.
- **A partial-chunk prefab claim breaks face agreement on that chunk's interior faces.** The hook
  gates the per-coordinate result; it does not remove the coordinate from its chunk's spanning
  structure, so an unclaimed cell of a partly-claimed chunk reads an interior face carved as though
  its claimed neighbour were fabric while the claimed neighbour reports the face deferred. The
  whole-chunk granularity `docs/DESIGN.md` §2.2.3 fixes is therefore a **stated precondition of the
  hook**, carried in the interface's doc comment with its guarantor named, not a check the generator
  performs. Border faces are *not* affected in either direction, because the portal rule never
  consults a claim; that asymmetry is the whole reason the interior half and the border half need
  separate statements, and arguing only the interior half leaves the border half silently wrong.
  Mitigation: the agreement sweep runs with a whole-chunk claim registered, so the
  contract-honouring case is proven rather than assumed — `[derived → AC3, AC13]`.
- **The coverage ratchet refuses a commit that adds substantially uncovered code.** The recorded
  high-water mark is already high
  `[measured 46ee531:ai-docs/coverage-ratchet.txt:1 · cat → 90.48]`, and a new package landing
  ahead of its tests would drop the measurement past the tolerance and block the commit.
  Mitigation: each subtask's tests land in that subtask's own commit, tests before production code
  — `[derived → the per-subtask commit discipline in § Test Design]`.
- **`algorithms.go` can pass the file-size soft limit** `[derived → the file-limits gate on the
  landed tree]`. The whole traversal-discipline set in one file is the readable arrangement for
  comparing them, and it is the arrangement to keep until the soft limit is passed, at which point
  the split is per algorithm. The hard limit is gated
  `[measured 46ee531:Makefile:55-57 · sed -n '55,57p' Makefile → the file-limits target's awk step, comparing each file against the non-test and test line limits]`.

## Test Design

**Placement and shape.** `internal/hexgrid/*_test.go` and `internal/maze/*_test.go` beside the
code; no Postgres anywhere in this task, so both packages' `TestMain` is the single
`os.Exit(leaktest.Main(m, (*testing.M).Run))` statement — the form the conventions page pins for a
package that is not database-backed
`[measured 46ee531:ai-docs/go-test-conventions.md:99 · grep -n "leaktest.Main(m, (\*testing.M).Run)" → "os.Exit(leaktest.Main(m, (*testing.M).Run)) // every other package"]`. Table-driven subtests with `t.Parallel()`
are the default; `pgregory.net/rapid` carries the property cases and is already a direct dependency
`[measured 46ee531:go.mod:22 · grep -n rapid go.mod → pgregory.net/rapid v1.3.0]`.
Structural guards enumerate and parse through `internal/srcguard` and each one is paired with a
scratch package written by `srcguard.WriteScratchFile` that **does** contain the forbidden
construct, so the guard is watched going red in the same file
`[measured 46ee531:internal/srcguard/srcguard.go:26-28,140-152 · sed -n '1,60p' and sed -n '140,175p' internal/srcguard/srcguard.go → "PackageFiles returns the sorted, absolute paths of every non-test Go source file directly inside dir"; "WriteScratchFile creates rel … under a fresh t.TempDir()"]`.

**Topology — AC4, AC5, AC6.** Entry points `Coord.Neighbor`, `Direction.Opposite`, `FaceOf`,
`Dims.ChunkOf`, `Dims.Origin`, `Dims.Contains`, `ChunkDistance`, `Dims.Distance`.
Scenarios: a cell's neighbours are distinct and the relation is symmetric; `FaceOf` from either
side of a neighbouring pair returns one identical value, and distinct directions give distinct
faces (rapid over coordinates). Chunk mapping is a partition — over a region straddling zero in
both axes, every cell maps to exactly one chunk, that chunk's `Contains` accepts it, and the
chunks' cell sets recover the region exactly; the same coordinate under two different dimension
sets maps to the correspondingly different chunks. Distance is zero exactly when both coordinates
share a chunk, equals the chunk lattice's hex distance on a table of hand-computed pairs, is
symmetric, obeys the triangle inequality (rapid), and does not overflow at the coordinate domain's
extremes.

**Derivation — AC2, AC16.** Entry points the preimage helpers, the world/cell/chunk/border keys and
the cell seed. Scenarios: the preimage bytes for a table of signed inputs match
`testdata/derive.golden` — the encoding is the one thing a hash cannot reveal a change in, so it is
pinned directly; the cell seed is a function of the world seed and the coordinate alone, proven by
two generators differing only in chunk dimensions yielding **the same** cell seed and **different**
faces; cell seeds over a coordinate table are pairwise distinct; two different world seeds move
every key.

**Draw reductions — AC2, AC14.** Entry points the bounded draw, the shuffle and the weighted pick.
Scenarios: a bound of one or zero returns zero and leaves the stream's next value unchanged; the
bounded draw's outputs lie in range and, over a long run against a fixed key, match a pinned prefix
(so the reduction cannot be swapped for the standard library's); the shuffle is a permutation and is
reproducible from the same key; the weighted pick never returns a zero-weight index, and is
reproducible.

**Islands — AC7's precondition, AC11.** Entry point the island selector. Scenarios: at the
reference dimensions and the share below, the chunk's non-island cells remain connected on the
lattice for a sweep of chunk coordinates and world seeds; no island is a border cell; a share of
zero yields no island; the degenerate shapes yield no island (every cell is a border cell in each
of them); the set is unchanged by the algorithm weights.
**The share criterion's instrument is pinned here, not chosen after the first measurement:**
dimensions 16×16, island share `0.05`, the region the block of chunks spanning chunk coordinates
`(-2,-2)` through `(1,1)` inclusive — so both signs of both axes are in it — and an **absolute
tolerance of ±0.01** on the achieved share. The per-chunk target rounds to slightly above the
input, so the tolerance's slack is for guard rejections and nothing else; the failure message
reports the achieved share and the rejection count rather than only the verdict.

**Algorithms — AC7, AC19.** Entry point each algorithm, plus the draw. Scenarios: for each of the
five in turn, under a weight set giving weight to that one alone, the drawn algorithm is that one
for a large sweep of chunks, and the structure it builds over the non-island set is a spanning tree
— connected, and with an edge count one below the non-island cell count when the extra-passage
share is zero. The growing tree is additionally driven at both bias extremes and in between, and
two different bias values are asserted to produce different structures for the same chunk (AC15).
**No statistic separating the five is asserted**: the owner struck that requirement in round 4, and
the growing-tree family's extremes coincide with two siblings by construction.

**Algorithm draw — AC14.** Scenarios: one chunk draws the same algorithm on repeat evaluation; over
a chunk sweep with a spread of weights more than one algorithm appears; a zero-weight algorithm
appears for no chunk in the sweep; two weight sets differ on at least one chunk of the sweep.

**Extra passages — AC10.** Scenarios: the count of open interior faces exceeds the spanning
structure's own edge count by the rounded share, capped by availability; a share of zero yields
exactly a tree; with a positive share a cycle is found (a traversal meeting an already-visited cell
through an unused edge); no border face is opened by this pass.

**Portals — AC3, AC8.** Scenarios: every border of a chunk sweep carries at least one and at most
two passages; the portal set computed from the lesser chunk equals the set computed from the
greater; both endpoint cells of every portal are non-island. The two counts are asserted on the
borders that can show them: **over the four many-candidate borders both one and two occur** across
the sweep, so the count draw is not degenerate, while **the two single-candidate diagonal borders
carry exactly one portal on every chunk of the sweep at the reference dimensions** — a designed
case with its own assertion, not a degenerate-only footnote, since the cap binds there at every
dimension. A single-cell chunk pair is asserted separately.

**Face agreement — AC3.** Entry point `Cell`. Over a multi-chunk region, exhaustively: for every
cell and every direction, the state read from one side equals the state read from the other side's
opposite direction. Run **twice** — once with a nil hook, and once with a hook claiming one whole
chunk **inside** the region, so the claimed chunk's own interior faces, its border faces against
unclaimed neighbours, and the unclaimed neighbours' view of those same faces are all in the sweep.
The second run is the load-bearing one: a nil-hook sweep cannot see the defect class where a claimed
cell reports nothing for a border face its unclaimed neighbour reads as a passage, so a sweep run
only with a nil hook is an instrument that is blind to the whole prefab boundary. Plus rapid over
world seeds and coordinates, including coordinates far from the origin and negative in both axes.

**Connectivity — AC7, AC9.** Entry point `Cell`, **with a nil hook** — a prefab's interior structure
is authored by #28, so a region containing a claimed chunk has no fabric path across it for this
task to assert; the scoping is stated here rather than left to be discovered when the sweep is
written. Over a multi-chunk region and a sweep of world seeds, flood-fill the region's non-island
cells **through the passages** and assert a single component; separately, per chunk, assert every
non-island cell of that chunk reachable from every other. AC7's "no repair stage" clause is discharged structurally rather than behaviourally: a guard
asserts the lattice-connectivity helper is called from the island selector and from nowhere else in
the package's non-test files, so no connectivity check exists downstream of the structure build for
a repair to hide in.

**Determinism — AC1, AC2.** `testdata/cells.golden`, minted by a `-update` flag on its own test.
Pinned in its header: the domain tag, the world seed `20260912`, chunk dimensions 16×16, an island
share of `0.05`, an extra-passage share of `0.15`, a growing-tree bias of `0.5`, and a weight set
giving every one of the five equal weight, and **a nil prefab hook** — the prefab path derives
nothing of its own, so the golden stays about the derivation chain, and the constant prefab column
pins that a nil hook defers nothing anywhere. Covered coordinates: every cell of the origin chunk;
every cell of the chunk diagonally below-left of it (negative in both axes); and the border ring of
the chunk below the origin. Compared fields, per line: the six face states **in direction order**,
the cell seed, the prefab marker, and — per chunk — the algorithm the draw returned, so a change in
the draw is caught even where two algorithms happen to build the same structure. **What a diff
means:** the domain tag, the preimage encoding, the digest, the stream, a reduction, the island
rule, an algorithm's traversal order, the cycle pass or the portal rule changed — every world
already generated under that seed is now a different world, and the change is a season rotation
rather than a refactor. The golden also carries AC1's separate-process clause and AC2 outright: it
was minted by a different process and CI re-checks it on a different machine, which a re-exec test
inside this pure-computation package would add nothing to.
Alongside it: a repeat evaluation in one process yields identical values; evaluating the coordinate
table in an order shuffled by a test-local fixed seed yields the same values as evaluating it in
sorted order; and a `-race` case driving one `Generator` from several goroutines yields the same
values as the single-goroutine run, which is also what would catch a memo added later.

**Locality — AC12.** Entry point the chunks-consulted function `Cell` uses. Scenarios: for a
coordinate beside the origin and one far from it, the returned set has the same size; every member
is the cell's own chunk or the chunk of one of its face-neighbours; and for two coordinates at the
same position **within** their respective chunks, the sets agree **after subtracting each
coordinate's own chunk** — the set of *relative* chunk offsets is the invariant, since the absolute
chunk coordinates necessarily differ. Plus the structural guard: the chunk-key and border-key
derivations are called only from the chunk-fabric builder and the portal builder.

**Prefab hook — AC13.** Entry point `Cell` with a recording fake that claims **one whole chunk**,
honouring the hook's granularity contract. Scenarios: with no hook, no coordinate of a table is
marked as a prefab's, every coordinate carries fabric, and no face anywhere is deferred. With the
whole-chunk hook: a claimed coordinate in the claimed chunk's **interior** is marked as a prefab's,
defers all six faces and carries a zero cell seed; a claimed coordinate on the claimed chunk's
**border ring** is marked as a prefab's, defers its interior faces, and carries border-face states
**equal to what the unclaimed cell across each of those borders reads** — the portals #28 stitches
to; the same generator with no hook returns non-deferred fabric for the same interior coordinate,
which is what shows the claim suppresses fabric rather than merely labelling it; and the fake
records having been asked about every coordinate, claimed or not. The "before" half of the ordering
has no black-box observable beyond that suppression and is stated here as review-judged against the
early return in `Cell`, not claimed as mechanically covered.

**Params — AC5, AC15.** Entry point `New`. Scenarios: each rejection names its input (non-positive
dimension, an all-zero weight set, an unknown algorithm key, a share or bias out of range, an
island share above what the dimensions can hold); a valid input yields a generator; the chunk
dimensions are honoured, proven by the mapping test above.

**Benchmarks — AC17.** `internal/maze/bench_test.go`: one benchmark over a single cell, and one per
algorithm under a single-weight input. No threshold is asserted and no gate runs them — a benchmark
does not run without the flag that selects it
`[measured go1.26.5 · go help testflag → "-bench regexp … By default, no benchmarks are run."]` —
so the measurement is on demand (`go test -run=^$ -bench=. ./internal/maze/`), it compiles under
every route that builds the tests so it cannot rot silently, and the caching decision stays where
the spec left it. The measured numbers go into the pull-request body and the per-task implementation log, never
into a doc comment or a tracked claim that would rot.

**The closed open question — AC18.** No test. It is a documentation criterion, established by the
sweep in subtask 10 and checked in review against the diff: a case-insensitive sweep of every live
document for the intra-chunk maze-algorithm claim, with `docs/DESIGN.md` §16.2 as one site and not
the bound of the class. History surfaces (the learnings log, retired plans) are left untouched.

**Guards — AC2, AC12.** In both packages' non-test files: no `time` import and no clock call; no
`math/rand` (v1), `hash/maphash` or `crypto/rand` import; **no floating-point type**, so the
rounding decisions above cannot be quietly re-expressed in binary floating point; exactly one
identifier referenced from `math/rand/v2` — the ChaCha8 constructor, which the unexported stream
interface is what keeps satisfiable — so any other selector on it fails the guard; no ranging over
a map; and, in `internal/maze`, the call-site confinements above — the connectivity helper reached
only from island selection, and the chunk-key and border-key derivations reached only from the
chunk-fabric builder and the portal builder. Each paired with its scratch-package red case.

## Open questions

- **None blocking.** The spec carries no row that prescribes a mechanism, a file set or a placement,
  and no row restating a standing rule, so nothing is flagged `SPEC-REMIT`.
- **One owner question from the interview was never answered, and this design answers it rather
  than parking it.** In round 3 the owner asked whether the algorithms had been compared on
  dead-ends alone or on further properties — "количество прямых, количество ветвлений, длина от
  ветвления до тупика". The round-4 question moved to approval and left it open. § Approach → *The
  algorithm set, and what each weight buys* is the answer, with its sources named: the properties
  those sources put on record are branching factor and corridor length, the dead-end profile, and
  uniformity over spanning trees. Straight-run length and branch-to-dead-end distance are stated by
  neither source, and this task measures neither, because the owner's round-4 answer removed any
  requirement to separate the algorithms by statistic. If the owner wants that characterisation
  measured over the shipped generator, it is a separate measurement task and not a change to this
  one.
