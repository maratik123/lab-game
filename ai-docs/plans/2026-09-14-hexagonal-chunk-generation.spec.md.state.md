# Interview state — hexagonal chunk generation

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-14-hexagonal-chunk-generation.spec.md
issue_ref: "#119"
gh_issue:
  title: "World generation rework: hexagonal chunks on a super-lattice"
  state: open
  labels: ["mvp", "area:world"]
  body: |
    ## What

    Rework the generation core shipped by #27 from rhombic chunks to hexagonal chunks of radius R on a hexagonal super-lattice, as the amended `docs/DESIGN.md` §2.2.2 describes. The core stays a pure, database-free function; what changes is the chunk shape, the border and portal rules, the island candidates, and the chunk type becoming a generation input.

    ## Design refs

    - `docs/DESIGN.md` §2.2.2 as amended by #118 — hexagonal chunks of radius R with six equal borders; fabric is a function of world seed, chunk coordinate and chunk type; a border does not depend on chunk type; portals are 10–20% of the 2R+1 border faces rounded up and never touch within one border; R ≥ 6; the door cell is never an island; post-hoc connectivity repair is rejected.
    - `docs/world-topology-redesign-plan.md` — D1, D25–D27, D29–D31 (geometry, portals, islands); D3 (the map is stored with its generation version — this issue exposes what persistence needs, storing it is #29); D22 (gameplay distances in cells by the hex formula); D28 (prefabs are out of the MVP, and D11's prefab region inside a chunk must not be precluded).
    - `ai-docs/key-decisions.md` KD-37…KD-40 — they describe the shipped core; each is revised where this rework changes it (KD-39's one-or-two portals per border, KD-40's island capacity).
    - `AGENTS.md` § Code Style — determinism on the generation path; § API Stability — a clean break, no compatibility shims for the rhombic-chunk vocabulary.
    - `AGENTS.md` § Dependency Versions — the super-lattice mapping is an established package or an argued hand-roll, never an unargued one.

    ## Depends on

    #118

    ## Scope

    - **Topology**: a cell coordinate maps to exactly one (chunk, local coordinate) on the hexagonal super-lattice and back; a chunk's six neighbours; the hex cell distance as the gameplay metric (D22).
    - **Chunk graph** over a hexagonal chunk of radius R, validated R ≥ 6.
    - **Borders and portals**: a border's 2R+1 faces enumerated identically from either side; the portal count drawn in `[⌈0.1·(2R+1)⌉, ⌈0.2·(2R+1)⌉]` from the border's own key; no two portals of one border share a vertex; the rule does not extend across a three-chunk corner, so a border stays a function of its two chunks.
    - **Islands**: candidates are non-border cells, minus the centre when the chunk is a gate chunk (D29, D30), with the existing connectivity guard.
    - **Chunk type as a generation input**, and a generation version identifier the stored map can carry (D3).
    - **Border agreement with a stored neighbour**: the API through which a new chunk takes its shared border from an existing neighbour's stored map instead of recomputing it; persistence itself is #29.
    - **Configuration**: the chunk radius key replaces the rectangular dimensions in the balance configuration and its schema; the portal shares are generation inputs.
    - **Tests**: every cell belongs to exactly one chunk; six equal borders; face agreement from both sides; connectivity over a multi-chunk region; the gate centre is never an island; the portal count stays in range and no two portals of a border touch; goldens re-minted.
    - **KD-37…KD-40** revised in `ai-docs/key-decisions.md`.

    ## Out of scope

    - Storing chunks, lazy creation and the per-world lock — #29.
    - Spiral gate placement and nearest-gate depth — #120.
    - The world and biome configuration format beyond the radius — #28.
    - Prefab regions inside a chunk — post-MVP (D11, D28).

    ## Telemetry obligation

    None — pure computation, no balance movement. The chunk-created event belongs to #29.

    ## Open questions to close in the spec

    - Whether the whole-chunk prefab hook is kept, removed or reshaped while prefabs are out of the MVP.
    - The form of the generation version: KD-37's domain tag, a separate constant, or a stored integer.
    - The super-lattice mapping: an established package or a hand-roll, with the argument the dependency rule requires.

    ---

    Part of #47. Tracked in #117.
  comments: []
  linked_issues: ["#27", "#118", "#29", "#120", "#28", "#47", "#117"]
  issue_body_status: current
  linked_prs: []
round_cap: 5
questions_per_round_cap: 3
round: 5
agent_id: ae266ab21629f092a
prior_qa:
  - round: 1
    question: "While prefabs are out of the MVP, does the reworked generator still offer a point where a prefab layer plugs in? DESIGN 2.2.3 only requires that MVP code not rule prefabs out."
    answer: "No hook"
  - round: 1
    question: "When #29 stores a chunk's map, which task defines the stored form? Once maps are written, that form changes only by forward migration."
    answer: "What is better?"
  - round: 2
    question: "Which task defines the form a chunk's map takes in storage? Once maps are written, that form changes only by forward migration. I recommend this task. The form mirrors the map this task builds and the version it names, and encoding it needs no database. The goldens would then lock the stored form, so any change to it shows up in the same diff as the generator change that caused it. #29 checks a stored map against what generation yields at its version, which is simplest when the generator produces that exact form. The cost: this task gets bigger, and #29 stores a layout it did not choose."
    answer: "This task (Recommended)"
  - round: 3
    question: "Approve and post cross-link comment?"
    answer: "Tweak first"
  - round: 3
    question: "Что поправить в спеке? (the orchestrator's note shown to the owner just before, verbatim: «Обратите внимание: AC20 — это расширение задачи, которое вы выбрали. Задача заметно вырастает: сериализация карты с проверкой «записали, прочитали, получили то же».»)"
    answer: "Давай откатим расширение скоупа?"
  - round: 4
    question: "Approve and post cross-link comment?"
    answer: "Approve"
  - round: 5
    question: "AC4 — верхняя граница радиуса → spec amendment via spec-writer + design-review re-run (при выборе правки спеки). Замечание design-writer, дословно: «AC4 says \"a chunk radius of six or more is accepted\". D12 refuses radii above `MaxRadius`, the largest R whose cell count fits `int32`. Above it, slice lengths and local indices stop being representable and generation would panic, which the zero-production-panic target forbids. The design reads AC4 as \"every radius from six up to that representability bound\", and its test accepts `MaxRadius`. If the owner wants a lower practical cap, that is a new policy number for the balance configuration, not this design's to pick.» Ревью (issue 2) добавляет: около MaxRadius один Generate выделяет граф ~2.1×10⁹ клеток — это нехватка памяти. Как поступить? (options: Amend the spec (Recommended) / Fix design only / Leave it)"
    answer: "Какой бред, это число буду задавать я, и я явно буду делать разумный выбор. Зачем я буду выбирать числа порядка 2^20? Чтобы что?"
  - round: 5
    question: "AC1–AC3 на шве int32 → spec amendment via spec-writer + design-review re-run (при выборе правки спеки). Замечание ревью (issue 3), дословно: «At the `int32` seam, D4 quietly narrows AC1, AC2 and AC3. N is always odd, so it never divides 2³². At the seam, AC2's \"neighbours exactly when some face joins\" fails, AC1's \"every chunk holds exactly the cells within distance R of its centre\" fails for chunks that straddle the seam, and AC3's step count and `Distance` disagree. D4 states a reading, but unlike the AC4 upper end it is not raised as an Open question.» (Шов лежит дальше 2³¹ клеток от центра мира.) Как поступить? (options: Amend the spec (Recommended) / Fix design only / Leave it)"
    answer: "Аналогичный вопрос. Может, оценивать реально? Зачем закладывать то, что никогда не будет достигнуто? Это телеграм игра, готовим мвп, ты реально считаешь, что к игре подключаться 100500 чатов на старте?"
  - round: 5
    question: "AC18 пересказывает общее правило → spec amendment via spec-writer + design-review re-run (при выборе правки спеки). Замечание design-writer, дословно: «`SPEC-REMIT: AC18` — the clause \"per AGENTS.md § *Propagation Rule* step 4\" restates a standing rule. The outcome it protects is that no live document carries the rhombic-chunk claim. It blocks nothing: the design implements that outcome in subtask 10's recipe.» Как поступить? (options: Amend the spec (Recommended) / Fix design only / Leave it)"
    answer: "Amend the spec (Recommended)"
  - round: 5
    question: "docs/world-topology-redesign-plan.md, строка 45: «у текущих чанков-параллелограммов тоже шесть соседей…» — после переделки фраза станет ложной. Дизайн считает файл историей (заголовок файла: план, решения живут в DESIGN.md) и не правит его. Считать документ живым? (options: История, не править (Recommended) / Живой, править)"
    answer: "Живой, править"
```
