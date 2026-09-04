# Interview state — configuration layer: runtime settings and the balance/world constant files

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-04-config-layer-balance-files.spec.md
issue_ref: "#18"
gh_issue:
  title: "Configuration layer: runtime settings and the balance/world constant files"
  state: open
  labels: ["mvp", "area:platform"]
  body: |
    ## What

    Every balance number in this game is a configuration value, never a Go literal (`docs/DESIGN.md` §16.5, `AGENTS.md` § Code Style). Nothing else can land correctly until the layer that holds those values exists. This issue builds the configuration layer: typed access, validation at start-up, and a clean separation between secrets, runtime settings, and game constants.

    ## Design refs

    - `docs/DESIGN.md` §16.5 — all balance constants live in config: stamina cap, step cost, backpack / respawn / standing timers, shop rates, door price curve, `budget(dist)`, combat dice and scales.
    - `docs/DESIGN.md` §11 — **Bot API base URL is a config axis with three values**: own instance (prod), `api.telegram.org` (emergency fallback), fake server (evals). Chunk size is config (reference 16x16). AFK-leader cruelty is config.
    - `docs/DESIGN.md` §2.2 — world lexicon and bestiary come from a world config (the world config *format* belongs to its own issue; this one owns the loader and the layering).
    - `AGENTS.md` § Permissions — a real token / DSN never enters a tracked file. Secrets come from the environment.

    ## Depends on

    _None — this one can start immediately._

    ## Scope

    - A `internal/config` package: load, validate, expose typed structs. No global mutable state, no `init()` magic.
    - Three sources with a fixed precedence: environment (secrets, DSN, base URL, allowlist), a tracked balance file, and a tracked world/biome file set.
    - **Validation at start-up, fail loud.** A missing or out-of-range constant stops the process with a message naming the key; it never silently defaults. A defaulted balance number is the exact defect this layer exists to prevent.
    - A tracked `.env.example` and a tracked balance file with placeholder values, so the repo runs without a real secret.
    - The three-value base-URL axis, with the fake-server value usable by tests.
    - Reload policy stated explicitly (start-up only is an acceptable answer for MVP; say so rather than leaving it implied).

    ## Out of scope

    - Filling in the real numbers — #46 does that after the mechanics exist. This issue ships placeholders.
    - The world/biome config *content* and its schema — #28.
    - Deploy-time configuration management (compose files, secret stores) — the infrastructure pass.

    ## Telemetry obligation

    None — this issue adds no mechanic and moves no balance.

    ## Open questions to close in the spec

    - File format for the balance set (TOML / YAML / JSON) and whether the world set shares it.
    - Whether the balance file is one file or one per area, and how a mechanic's constants get added without a merge conflict every time.

    ---

    Part of #47 (MVP roadmap).

  comments: []
  linked_issues: ["#28", "#46", "#47"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: a1ec125d6a209b32b
prior_qa:
  - round: 0
    question: "File format for the balance set (TOML / YAML / JSON)? — open question carried in the issue body"
    answer: |
      YAML. User instruction at /task entry, verbatim: "формат balance-set yaml".
      Supplied before round 1; not produced by a spec-writer round.
  - round: 1
    question: "The balance set: one tracked YAML file, or a directory of per-area files the loader merges? This also fixes how a new mechanic's constants get added."
    answer: |
      One file — "A single tracked balance YAML. Simplest loader, one place to look;
      every mechanic edits the same file, so concurrent branches collide there."
  - round: 1
    question: "Precedence between the three sources: may an environment variable change a value that lives in the balance or world files?"
    answer: |
      Disjoint — "Environment owns secrets, runtime settings and the file paths, and can
      never change a game constant. A balance change is always a tracked file edit plus a restart."
  - round: 1
    question: "The world/biome file set, given #28 owns its schema and content: how much does this task load?"
    answer: |
      Paths only — "The loader resolves and validates where the world set lives and how it
      layers with the other two sources. No world decode at all; #28 adds the typed schema."
```
