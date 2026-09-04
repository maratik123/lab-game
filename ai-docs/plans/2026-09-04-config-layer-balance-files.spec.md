# Configuration layer: runtime settings and the balance/world constant files

**Source:** issue #18
**Date:** 2026-09-04
**Tracked in:** #18

Every balance number in this game is a configuration value, never a Go literal
(`docs/DESIGN.md` §16.5; `AGENTS.md` § Code Style; `ai-docs/domain-invariants.md` § 8).
Nothing else in the MVP can land correctly until the layer that holds those values
exists. This task builds that layer: typed access, start-up validation, and a clean
separation between secrets, runtime settings, and game constants.

> **Round-1 draft.** Three decisions are still open (Q1–Q3 in *Key decisions*); the
> sections they touch are marked `TBD (Q<n>)`.

## Scope

1. **A new package `internal/config`** — loads, validates and exposes typed
   configuration. No package-level mutable state, no `init()` magic, no global
   singleton: the loader returns a value and an error, and the caller holds it.
2. **The environment layer** — secrets and per-environment runtime settings:
   the bot token, the Postgres DSN, the Bot API base URL, `ALLOWED_CHAT_IDS`, and
   the paths at which the tracked config files are found. Variable names carry the
   `LAB_GAME_` prefix this repo already uses
   [source: a1415ac:internal/testdb/testdb.go:31 · `grep -rn LAB_GAME internal/`].
3. **The balance layer** — a tracked YAML file (or file set — TBD Q1) holding a
   placeholder value for every balance axis the design names.
4. **The world/biome file set** — its place in the layering, and however much of its
   loading this task owns — TBD (Q3). The world config's *content and schema* are
   issue #28's.
5. **Precedence between the three sources** — TBD (Q2). Whatever the answer, the
   resolved precedence is stated in the package doc comment, not left implied.
6. **Start-up validation, fail loud.** A missing, unparsable, unknown or structurally
   invalid key stops the process with a message naming the key path. No balance value
   has a compiled-in fallback — a silently defaulted balance number is the exact defect
   this layer exists to prevent.
7. **A tracked `.env.example`** with placeholder values only, so a fresh clone can be
   brought up without a real secret.
8. **The three-value Bot API base-URL axis** — own instance (production), cloud
   `api.telegram.org` (emergency fallback), fake server (evals), the last usable by a
   test [source: a1415ac:docs/DESIGN.md:304 · `grep -n "Base URL" docs/DESIGN.md`].
9. **`cmd/bot/main.go` wired to load and validate at start-up**, exiting non-zero with
   the key-naming message on failure. Without this the "stops the process" clause is
   unexercised. The binary is still a scaffold otherwise; no Telegram client, no DB
   connection, no update loop.
10. **Reload policy stated explicitly** — see *Key decisions*.
11. **Propagation** — every site whose claim this change falsifies is updated in the
    same PR (membership criterion: `AGENTS.md` § Propagation Rule step 4). Known
    members illustrating the class, not bounding it: `AGENTS.md` § Build & Test
    (`go run ./cmd/bot` now requires environment), `ai-docs/key-decisions.md`,
    `ai-docs/context.md`, `ai-docs/plans/INDEX.md`.

## Out of scope

- **Filling in the real numbers** — #46 does that once the mechanics exist. This task
  ships placeholders.
- **World/biome config content and schema** — #28.
- **Deploy-time configuration management** — compose files, secret stores, the second
  compose file for the testing environment (`docs/DESIGN.md` §12.5). Infrastructure pass.
- **Consuming the configuration** — the Telegram transport, the store wiring, the raid
  FSM. This task exposes values; connecting anything to them belongs to those issues.
- **In-process `.env` parsing** — see *Key decisions*.
- **`api_id` / `api_hash`** — credentials of the self-hosted `telegram-bot-api`
  container, not of the bot process (`docs/DESIGN.md` §12.2). They never reach this
  package.

## Deferred

- Hot reload / SIGHUP re-read of the balance set | MVP does not need it and it makes
  every consumer re-entrant | yes, if a season ever needs a mid-flight tuning change
- Per-key numeric range bounds (min/max, monotonicity of the curves) | the real numbers
  and their admissible ranges are #46's; this task validates shape and presence | no —
  folds into #46
- A schema/validation report command (`bot config check`) | operator convenience, not
  needed to land the layer | yes, small follow-up
- Environment-specific overlay files (`balance.testing.yaml`) | isolation is by env per
  KD-11, and no second environment exists yet | yes, with the infrastructure pass

## Key decisions

| Question | Decision |
|---|---|
| Balance-set file format | **YAML.** Owner instruction at `/task` entry, verbatim: *"формат balance-set yaml"*. |
| One balance file or one per area, and how a new mechanic adds constants without a standing merge conflict | **TBD (Q1).** |
| Precedence between environment, balance file and world file set | **TBD (Q2).** |
| What this task ships for the world/biome file set, given #28 owns its schema | **TBD (Q3).** |
| Reload policy | **Start-up only.** A balance change takes effect on process restart; no rebuild, no redeploy of code. Consequence the design must honour: the balance set is read from the filesystem at a path the operator controls — a `go:embed`-only build would make a balance change require a rebuild, contradicting `ai-docs/domain-invariants.md` § 8 ("the season's balance is expected to move without a deploy"). An embedded copy as a *fallback* is a design option only if the external path still wins. |
| Environment-variable naming | `LAB_GAME_` prefix, matching the existing `LAB_GAME_TEST_DSN` [source: a1415ac:internal/testdb/testdb.go:31 · `grep -rn LAB_GAME internal/`]. |
| Where environment values come from | **The process environment only.** The Go process does not parse `.env`; the operator or compose exports it. `.env.example` is the tracked documentation of the required set. Keeps the loader standard-library-only on this axis. |
| `ALLOWED_CHAT_IDS` | **Required and non-empty in every environment**, including production. `docs/DESIGN.md` §12.5 makes it a code-level safety net that is *always on in testing*; the MVP ships to one friendly chat (§14), so requiring it everywhere costs nothing and removes the "unset means write anywhere" failure mode. Unset or empty is a start-up error naming the variable. |
| Unknown key in a config file | **An error.** Strict decoding: a typo in a balance key must not silently leave the intended key at its (nonexistent) default. |
| Bot API base URL representation | **One value, one axis** — an absolute `http`/`https` URL. The three design values are three values of that one variable; the evals value is an arbitrary fake-server URL, so an enum cannot express it. Rejecting a non-absolute or non-http(s) value is part of validation. |
| Placeholder semantics | A placeholder is a **valid value that loads**, not a sentinel that fails. The tracked balance file must load clean — that is what "the repo runs without a real secret" means. Placeholders carry no authority: #46 replaces them. |
| Where balance placeholders come from | The axes named by the design, with the design's own reference values where it states one: chunk size — DESIGN's word is *ориентир* (reference), 16×16 hexes [source: a1415ac:docs/DESIGN.md:69 · `sed -n '69p' docs/DESIGN.md`]; boss-reward door TTL — stated as approximate, ~30 minutes [source: a1415ac:docs/DESIGN.md:81 · `sed -n '81p' docs/DESIGN.md`]. Axes with no design number get an obviously-placeholder value. |
| YAML parser choice | **Left to design.** See *Technical constraints* — the choice carries a dependency obligation the design document must discharge. |

## Technical constraints

- **Module and toolchain:** `github.com/maratik123/lab-game`, `go 1.26`
  [source: a1415ac:go.mod:1-3 · `cat go.mod`].
- **YAML parsing is a dependency-graph question the design must answer.**
  `gopkg.in/yaml.v3 v3.0.1` is listed in `go.mod` as an **indirect** requirement
  [source: a1415ac:go.mod:73 · `grep -n yaml go.mod`], and `go mod why` resolves it
  through a **test-only** path — `internal/store` → `github.com/jackc/pgx/v5` →
  `github.com/jackc/pgx/v5.test` → `testify/assert/yaml`
  [source: a1415ac:go.mod:73 · `go mod why -m gopkg.in/yaml.v3`]. Importing a YAML
  parser from production code changes that standing. `AGENTS.md` § Dependency Versions
  requires a stated reason in the design document for such a change, and forbids
  hand-editing `go.mod`. The design names the parser and states the reason.
- **No `panic` / `log.Fatal` in production code** (`AGENTS.md` § Go Test Conventions).
  `main` may exit non-zero; the loader returns errors.
- **No balance literal in Go source.** A tuning value compiled into a `.go` file is a
  defect even when it is a named constant (`AGENTS.md` § Code Style).
- **Secrets never enter a tracked file.** `.gitignore` ignores `.env` and `.env.*` and
  carries an explicit `!.env.example` negation
  [source: a1415ac:.gitignore:8-10 · `grep -n "" .gitignore`]; the negation wins, so a
  `.env.example` created by this task is not ignored and will be committed
  [source: a1415ac:.gitignore:10 · `git check-ignore -q .env.example` → exit 1, against
  exit 0 for `.env`]. It must therefore carry placeholders only — never a real token,
  DSN, `api_id` or `api_hash` (`AGENTS.md` § Permissions).
- **Config axes the design marks as configuration, beyond the §16.5 balance list:**
  chunk size [source: a1415ac:docs/DESIGN.md:69 · `sed -n '69p' docs/DESIGN.md`],
  boss-reward door TTL [source: a1415ac:docs/DESIGN.md:81 · `sed -n '81p' docs/DESIGN.md`],
  AFK-leader cruelty [source: a1415ac:docs/DESIGN.md:156 · `grep -n "Жестокость" docs/DESIGN.md`],
  the Bot API base URL [source: a1415ac:docs/DESIGN.md:304 · `grep -n "Base URL" docs/DESIGN.md`],
  `ALLOWED_CHAT_IDS` [source: a1415ac:docs/DESIGN.md:395 · `grep -n ALLOWED_CHAT_IDS docs/DESIGN.md`],
  and the world lexicon / bestiary [source: a1415ac:docs/DESIGN.md:53 · `grep -n "конфига мира" docs/DESIGN.md`].
- **Determinism is untouched** by this task, but the config layer must not become a
  hidden source of non-determinism: no wall clock, no map-iteration order in anything
  the loader returns to a deterministic path (`AGENTS.md` § Code Style).
- **Documentation:** package comment plus a doc comment on every exported item
  (`ai-docs/doc-convention.md`).
- **Executability of the acceptance criteria.** Every gate an AC below relies on is
  already permitted: `Bash(go *)` and `Bash(make *)` are in `permissions.allow` in
  `.claude/settings.json`, so `go run ./cmd/bot` and the `make` gate targets run
  unattended. Running a *built* binary by bare path (`./bot`) is **not** granted — an
  AC about start-up behaviour is checked through `go run` or a Go test, not by invoking
  a compiled artefact directly. No new permission grant is needed for this task.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Package `internal/config` exists, carries a package comment, and exposes a loader that returns a typed configuration value and an error. The package declares no package-level mutable state and no `init` function. |
| AC2 | Loading with a required environment variable unset fails, and the error message names that variable. |
| AC3 | Loading with a balance key absent from the balance set fails, and the error message names the missing key path. |
| AC4 | Loading with a balance key present but structurally invalid (wrong YAML type, or non-positive where the design requires a positive quantity) fails, and the error message names the key path. |
| AC5 | Loading a config file containing a key the schema does not define fails, and the error message names the unknown key path. |
| AC6 | No balance value has a compiled-in fallback: loading a syntactically valid but empty balance set fails rather than yielding a usable configuration. |
| AC7 | A tracked YAML balance set exists at the repository's default path and carries a placeholder value for every axis listed in `docs/DESIGN.md` §16.5 (stamina cap, step cost, backpack / respawn / standing timers, shop rates, door price curve, `budget(dist)` curve, combat dice and scales) plus chunk size, boss-reward door TTL and AFK-leader cruelty. Loading it with the example environment succeeds. |
| AC8 | A tracked `.env.example` exists, is not matched by any `.gitignore` rule, and lists every environment variable the loader requires — each with a placeholder value. No file in the change contains a real bot token, DSN, `api_id` or `api_hash`. |
| AC9 | The Bot API base URL is a single configuration value: the loader accepts a self-hosted instance URL, `https://api.telegram.org`, and an arbitrary fake-server URL a test can point at; it rejects a value that is not an absolute `http`/`https` URL, naming the variable. |
| AC10 | `ALLOWED_CHAT_IDS` is required: unset, empty, or containing a non-integer element fails, naming the variable. A well-formed value parses to the corresponding list of chat ids. |
| AC11 | `cmd/bot` loads and validates configuration before any other work; on a validation failure it writes the key-naming message to standard error and exits non-zero. No `panic` and no `log.Fatal` appears in any non-test Go file of the change. |
| AC12 | Table-driven tests cover each failure mode in AC2–AC6 and AC9–AC10, and each success path in AC7 and AC11, without requiring a database or network. |
| AC13 | The `internal/config` package doc comment states the reload policy (start-up only) and the resolved precedence between the configuration sources. |
| AC14 | Every gate `make verify` runs is green on the resulting tree. |
| AC15 | Propagation is complete for this change: every site whose claim the diff falsifies is updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4. Sites known at spec time — illustrative, not exhaustive: `AGENTS.md` § Build & Test (the `go run ./cmd/bot` line, which now requires environment), `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/plans/INDEX.md`. |

## Open questions

- **Per-key admissible ranges.** Validation in this task is presence + shape +
  design-stated invariants (a duration is positive, a chunk dimension is positive).
  Real bounds arrive with the real numbers in #46.
- **Whether the loader should expose the balance set as one flat struct or as per-area
  sub-structs.** Consumers do not exist yet; the design may choose either, and the
  choice is cheap to revise while `internal/config` has no callers.
- **Whether a future second environment wants overlay files** rather than a wholly
  separate tracked file. KD-11 (isolation by environment, same image) suggests overlays
  eventually; nothing forces the question now.
- **Whether the world file set is one file per world or a directory per world.** Bound
  up with #28's schema; this task fixes only as much as Q3 decides.
