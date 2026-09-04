# Configuration layer: runtime settings and the balance/world constant files

**Source:** issue #18
**Date:** 2026-09-04
**Tracked in:** #18

Every balance number in this game is a configuration value, never a Go literal
(`docs/DESIGN.md` §16.5; `AGENTS.md` § Code Style; `ai-docs/domain-invariants.md` § 8).
Nothing else in the MVP can land correctly until the layer that holds those values
exists. This task builds that layer: typed access, start-up validation, and a clean
separation between secrets, runtime settings, and game constants.

## Scope

1. **A new package `internal/config`** — loads, validates and exposes typed
   configuration. No package-level mutable state, no `init()` magic, no global
   singleton: the loader returns a value and an error, and the caller holds it.
2. **The environment layer** — secrets and per-environment runtime settings:
   the bot token, the Postgres DSN, the Bot API base URL, `ALLOWED_CHAT_IDS`, and
   the paths at which the tracked config files are found. Variable names carry the
   `LAB_GAME_` prefix this repo already uses
   [source: 1646096:internal/testdb/testdb.go:31 · `grep -rn LAB_GAME internal/`].
3. **The balance layer** — **one** tracked YAML file holding a placeholder value for
   every balance axis the design names. A new mechanic adds its constants to that same
   file (owner decision, round 1).
4. **The world/biome file set — path resolution and layering only.** The loader learns
   from the environment where the world set lives and validates that the path resolves
   to an existing, readable target; it performs **no decode** of the world set and
   defines no world schema (owner decision, round 1). A tracked placeholder target
   exists at the default path so a fresh clone loads clean. #28 adds the typed world
   configuration, its schema, and its content.
5. **Precedence between the three sources: they are disjoint by domain** (owner
   decision, round 1) — the environment supplies secrets, runtime settings and the file
   paths and can never supply or override a game constant; the balance file is the sole
   source of balance values; the world file set is the sole source of world content.
   The resolved precedence is stated in the package doc comment, not left implied.
6. **Start-up validation, fail loud.** A missing, unparsable, unknown or structurally
   invalid key stops the process with a message naming the key path. ("Unknown key"
   reaches only the balance file — it is the one file source this task decodes.) No
   balance value has a compiled-in fallback: a silently defaulted balance number is the
   exact defect this layer exists to prevent.
7. **A tracked `.env.example`** with placeholder values only, so a fresh clone can be
   brought up without a real secret.
8. **The three-value Bot API base-URL axis** — own instance (production), cloud
   `api.telegram.org` (emergency fallback), fake server (evals), the last usable by a
   test [source: 1646096:docs/DESIGN.md:304 · `grep -n "Base URL" docs/DESIGN.md`].
9. **`cmd/bot/main.go` wired to load and validate at start-up**, exiting non-zero with
   the key-naming message on failure. Without this the "stops the process" clause is
   unexercised. The binary is still a scaffold otherwise; no Telegram client, no DB
   connection, no update loop.
10. **Reload policy stated explicitly** — see *Key decisions*.
11. **The permission narrowing that makes `.env.example` authorable at all** — already
    committed on this branch and therefore shipping in this task's PR: the
    `.claude/settings.json` deny list stopped matching `.env.example` (929b8e7), with
    the tool-contract propagation to `ai-docs/claude-tools-hierarchy.md` (db7999e).
    Rationale and the verified rule state: *Technical constraints*.
12. **Propagation** — every site whose claim this change falsifies is updated in the
    same PR (membership criterion: `AGENTS.md` § Propagation Rule step 4). Known
    members illustrating the class, not bounding it: `AGENTS.md` § Build & Test
    (`go run ./cmd/bot` now requires environment), `ai-docs/key-decisions.md`,
    `ai-docs/context.md`, `ai-docs/plans/INDEX.md`.

## Out of scope

- **Filling in the real numbers** — #46 does that once the mechanics exist. This task
  ships placeholders.
- **World/biome config content, schema, and decoding** — #28. This task resolves and
  validates the *path* to the world set and nothing inside it. Its internal layout
  (one file per world, a directory per world, lexicon and bestiary shapes) is #28's to
  decide; no wording here constrains that choice.
- **Environment overrides of game constants** — excluded by the disjoint-sources
  decision, not merely unimplemented. A balance change is an edit to the tracked
  balance file, taking effect at the next start-up.
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
- Upgrading how settings are stored so that balance can move without a deploy | seasons
  are not MVP, and the owner deferred the storage question until they arrive (round-2
  correction, verbatim: *«когда сезоны появятся, мы подумаем на апгрейдом хранения
  настроек (как это делать без деплоя), сейчас это не в мвп»*). This is the disposition
  of the closing clause of `ai-docs/domain-invariants.md` § 8 — "the season's balance is
  expected to move without a deploy" [source: 1646096:ai-docs/domain-invariants.md:68 ·
  `sed -n '68p' ai-docs/domain-invariants.md`]: deferred, so it binds no design decision
  in this task. The rest of § 8 — balance numbers are configuration, never a Go literal
  — is untouched and remains the reason this task exists | yes, with the seasons work
- Splitting the balance file into per-area files | the owner chose one file for loader
  simplicity and one place to look; the accepted cost, in the option's own words, is
  that "every mechanic edits the same file, so concurrent branches collide there" —
  a cost that only bites once several mechanics land in parallel | yes, if the
  collisions become routine rather than occasional

## Key decisions

| Question | Decision |
|---|---|
| Balance-set file format | **YAML.** Owner instruction at `/task` entry, verbatim: *"формат balance-set yaml"*. |
| One balance file or one per area, and how a new mechanic adds constants without a standing merge conflict | **One file** (owner, round 1). A single tracked balance YAML: simplest loader, one place to look. A new mechanic adds its constants to that file. The accepted cost is branch collisions in it; recorded as a Deferred row rather than designed around now. |
| Precedence between environment, balance file and world file set | **Disjoint by domain** (owner, round 1) — there is no override chain to resolve. The environment owns secrets, runtime settings and the file paths and can never change a game constant; each file source owns its own domain exclusively. Consequence for the design: no generic key-path override machinery, and no environment-vs-file merge step. |
| What this task ships for the world/biome file set, given #28 owns its schema | **Paths and layering only** (owner, round 1). The loader resolves the world-set path from the environment and validates that it exists and is readable; it does not decode it and declares no world type. A tracked placeholder target ships at the default path so the load succeeds on a fresh clone — its *contents* are #28's, and this task neither reads nor constrains them. |
| Reload policy | **Start-up only.** Configuration is read and validated once, during start-up; nothing re-reads it while the process runs, so a balance change takes effect no earlier than the next start-up. |
| Where the balance set is read from — an operator-supplied path, a copy embedded at build time, or both | **Left to design**, exactly like the parser choice below. Nothing in the MVP constrains it: the "move balance without a deploy" question is deferred out of MVP (see *Deferred*), and no other requirement here depends on the answer. |
| Environment-variable naming | `LAB_GAME_` prefix, matching the existing `LAB_GAME_TEST_DSN` [source: 1646096:internal/testdb/testdb.go:31 · `grep -rn LAB_GAME internal/`]. |
| Where environment values come from | **The process environment only.** The Go process does not parse `.env`; the operator or compose exports it. `.env.example` is the tracked documentation of the required set, which — under the disjoint-sources decision — is exactly: secrets, runtime settings, and the path of each file source the design reads from disk. Keeps the loader standard-library-only on this axis. |
| `ALLOWED_CHAT_IDS` | **Required and non-empty in every environment**, including production. `docs/DESIGN.md` §12.5 makes it a code-level safety net that is *always on in testing*; the MVP ships to one friendly chat (§14), so requiring it everywhere costs nothing and removes the "unset means write anywhere" failure mode. Unset or empty is a start-up error naming the variable. |
| Unknown key in the balance file | **An error.** Strict decoding: a typo in a balance key must not silently leave the intended key at its (nonexistent) default. The rule has no world-file counterpart in this task, because the world set is not decoded here. |
| Bot API base URL representation | **One value, one axis** — an absolute `http`/`https` URL. The three design values are three values of that one variable; the evals value is an arbitrary fake-server URL, so an enum cannot express it. Rejecting a non-absolute or non-http(s) value is part of validation. |
| What `.env.example` lists | **Only the loader's own variables** (owner, round 4). Test-suite variables such as `LAB_GAME_TEST_DSN` are documented elsewhere and do not appear there — the `LAB_GAME_` prefix precedent in Scope 2 cites that variable for its *naming*, not for inclusion. This is what keeps AC8 and AC16 a set equality: every variable the loader requires is listed, and the loader consults nothing outside the list. Neither AC widens. |
| Placeholder semantics | A placeholder is a **valid value that loads**, not a sentinel that fails. The tracked balance file must load clean — that is what "the repo runs without a real secret" means. Placeholders carry no authority: #46 replaces them. |
| Where balance placeholders come from | The axes named by the design, with the design's own reference values where it states one: chunk size — DESIGN's word is *ориентир* (reference), 16×16 hexes [source: 1646096:docs/DESIGN.md:69 · `sed -n '69p' docs/DESIGN.md`]; boss-reward door TTL — stated as approximate, ~30 minutes [source: 1646096:docs/DESIGN.md:81 · `sed -n '81p' docs/DESIGN.md`]. Axes with no design number get an obviously-placeholder value. |
| YAML parser choice | **Left to design.** See *Technical constraints* — the choice carries a dependency obligation the design document must discharge. |

## Technical constraints

- **Module and toolchain:** `github.com/maratik123/lab-game`, `go 1.26`
  [source: 1646096:go.mod:1-3 · `cat go.mod`].
- **YAML parsing is a dependency-graph question the design must answer.**
  `gopkg.in/yaml.v3 v3.0.1` is listed in `go.mod` as an **indirect** requirement
  [source: 1646096:go.mod:73 · `grep -n yaml go.mod`], and `go mod why` resolves it
  through a **test-only** path — `internal/store` → `github.com/jackc/pgx/v5` →
  `github.com/jackc/pgx/v5.test` → `testify/assert/yaml`
  [source: 1646096:go.mod:73 · `go mod why -m gopkg.in/yaml.v3`]. Importing a YAML
  parser from production code changes that standing. `AGENTS.md` § Dependency Versions
  requires a stated reason in the design document for such a change, and forbids
  hand-editing `go.mod`. The design names the parser and states the reason.
- **No `panic` / `log.Fatal` in production code** (`AGENTS.md` § Go Test Conventions).
  `main` may exit non-zero; the loader returns errors.
- **No balance literal in Go source.** A tuning value compiled into a `.go` file is a
  defect even when it is a named constant (`AGENTS.md` § Code Style).
- **Secrets never enter a tracked file.** `.gitignore` ignores `.env` and `.env.*` and
  carries an explicit `!.env.example` negation
  [source: 1646096:.gitignore:8-10 · `grep -n "" .gitignore`]; the negation wins, so a
  `.env.example` created by this task is not ignored and will be committed
  [source: 1646096:.gitignore:10 · `git check-ignore -q .env.example` → exit 1, against
  exit 0 for `.env`]. It must therefore carry placeholders only — never a real token,
  DSN, `api_id` or `api_hash` (`AGENTS.md` § Permissions).
- **Config axes the design marks as configuration, beyond the §16.5 balance list:**
  chunk size [source: 1646096:docs/DESIGN.md:69 · `sed -n '69p' docs/DESIGN.md`],
  boss-reward door TTL [source: 1646096:docs/DESIGN.md:81 · `sed -n '81p' docs/DESIGN.md`],
  AFK-leader cruelty [source: 1646096:docs/DESIGN.md:156 · `grep -n "Жестокость" docs/DESIGN.md`],
  the Bot API base URL [source: 1646096:docs/DESIGN.md:304 · `grep -n "Base URL" docs/DESIGN.md`],
  `ALLOWED_CHAT_IDS` [source: 1646096:docs/DESIGN.md:395 · `grep -n ALLOWED_CHAT_IDS docs/DESIGN.md`],
  and the world lexicon / bestiary [source: 1646096:docs/DESIGN.md:53 · `grep -n "конфига мира" docs/DESIGN.md`].
- **Determinism is untouched** by this task, but the config layer must not become a
  hidden source of non-determinism: no wall clock, no map-iteration order in anything
  the loader returns to a deterministic path (`AGENTS.md` § Code Style).
- **Documentation:** package comment plus a doc comment on every exported item
  (`ai-docs/doc-convention.md`).
- **Executability of the acceptance criteria — the command side *and* the file-tool
  side.** The command side: `Bash(go *)` and `Bash(make *)` are in `permissions.allow`,
  so `go run ./cmd/bot` and the `make` gate targets run unattended; running a *built*
  binary by bare path (`./bot`) is not granted, so an AC about start-up behaviour is
  checked through `go run` or a Go test, never by invoking a compiled artefact directly.
  The file-tool side is the one that blocks: AC8 requires this task to **create**
  `.env.example`, and a `deny` entry is evaluated before `allow`, is final, cannot be
  approved in-session, takes gitignore syntax **with no in-pattern negation**, and
  governs the `Write` tool through its `Edit(...)` entries — no separate `Write(...)`
  rule exists. A `**/.env.*` catch-all therefore matches `.env.example` too and would
  leave the required artefact unauthorable and unreadable. The deny list enumerates the
  real environment files instead — `**/.env` plus `.env.local`, `.env.development`,
  `.env.test`, `.env.staging`, `.env.production`, `.env.secret`, `.env.secrets`, each
  for both `Read` and `Edit` — and carries no `.env.*` catch-all
  [source: db7999e:.claude/settings.json · `jq -r '.permissions.deny[]?' .claude/settings.json`].
  Confirmed against the live matcher rather than by reading the rules alone: a `Read` of
  `.env.example` returns "File does not exist", i.e. it passes the permission layer.
- **The deny rules reach `Bash` as well as the file tools.** A Bash command whose text
  names a denied path is refused outright, while the same command naming only
  `.env.example` runs — measured this round: `ls -la .env .env.example` was refused,
  `ls -la .env.example` executed. Consequence for whoever verifies AC8: use a command
  that names `.env.example` alone, and reach `.env` (if it must be reached at all)
  through neither tool.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | Package `internal/config` exists, carries a package comment, and exposes a loader that returns a typed configuration value and an error. The package declares no package-level mutable state and no `init` function. |
| AC2 | Loading with a required environment variable unset fails, and the error message names that variable. |
| AC3 | Loading with a balance key absent from the balance set fails, and the error message names the missing key path. |
| AC4 | Loading with a balance key present but structurally invalid (wrong YAML type, or non-positive where the design requires a positive quantity) fails, and the error message names the key path. |
| AC5 | Loading a balance file containing a key the schema does not define fails, and the error message names the unknown key path. |
| AC6 | No balance value has a compiled-in fallback: loading a syntactically valid but empty balance set fails rather than yielding a usable configuration. |
| AC7 | A tracked YAML balance set exists at the repository's default path and carries a placeholder value for every axis listed in `docs/DESIGN.md` §16.5 (stamina cap, step cost, backpack / respawn / standing timers, shop rates, door price curve, `budget(dist)` curve, combat dice and scales) plus chunk size, boss-reward door TTL and AFK-leader cruelty. Loading it with the example environment succeeds. |
| AC8 | A tracked `.env.example` exists, is not matched by any `.gitignore` rule, and lists every environment variable the loader requires — each with a placeholder value. No file in the change contains a real bot token, DSN, `api_id` or `api_hash`. |
| AC9 | The Bot API base URL is a single configuration value: the loader accepts a self-hosted instance URL, `https://api.telegram.org`, and an arbitrary fake-server URL a test can point at; it rejects a value that is not an absolute `http`/`https` URL, naming the variable. |
| AC10 | `ALLOWED_CHAT_IDS` is required: unset, empty, or containing a non-integer element fails, naming the variable. A well-formed value parses to the corresponding list of chat ids. |
| AC11 | `cmd/bot` loads and validates configuration before any other work; on a validation failure it writes the key-naming message to standard error and exits non-zero. No `panic` and no `log.Fatal` appears in any non-test Go file of the change. |
| AC12 | Table-driven tests cover each failure mode in AC2–AC6, AC9, AC10 and AC15, and each success path in AC7, AC11, AC15 and AC16, without requiring a database or network. |
| AC13 | The `internal/config` package doc comment states the reload policy (start-up only) and names each source's exclusive domain: environment — secrets, runtime settings, file paths; balance file — balance values; world set — world content, not decoded here. |
| AC14 | Every gate `make verify` runs is green on the resulting tree. |
| AC15 | The world-set path is a required environment variable resolved by the loader: a value naming a target that does not exist or is unreadable fails, naming the variable; a value naming an existing readable target succeeds. The loaded configuration exposes that path and no decoded world content, and no Go file in the change declares a type describing the world set's interior. A tracked placeholder target exists at the default path named in `.env.example`. |
| AC16 | The sources are disjoint in fact, not only in prose: the loader consults no environment variable outside the set documented in `.env.example`, and no balance value the loader returns differs from the value in the balance file for any environment. |
| AC17 | Propagation is complete for this change: every site whose claim the diff falsifies is updated in the same PR, membership decided by `AGENTS.md` § Propagation Rule step 4. Sites known at spec time — illustrative, not exhaustive: `AGENTS.md` § Build & Test (the `go run ./cmd/bot` line, which now requires environment), `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/plans/INDEX.md`. Two members have already landed on the branch and are part of this class, not exceptions to it: `.claude/settings.json` (929b8e7) and `ai-docs/claude-tools-hierarchy.md` (db7999e), the sibling required by the Propagation Rule for a change to a tool contract. |

## Open questions

- **Per-key admissible ranges.** Validation in this task is presence + shape +
  design-stated invariants (a duration is positive, a chunk dimension is positive).
  Real bounds arrive with the real numbers in #46.
- **Whether the loader should expose the balance set as one flat struct or as per-area
  sub-structs.** Consumers do not exist yet; the design may choose either, and the
  choice is cheap to revise while `internal/config` has no callers.
- **Whether a future second environment wants overlay files** rather than a wholly
  separate tracked file. KD-11 (isolation by environment, same image) suggests overlays
  eventually; nothing forces the question now. Note that an overlay is a *file* source,
  so it does not reopen the disjoint-sources decision.
- **Whether the world-set path should name a file or a directory.** This task validates
  whichever the environment names, so the choice stays open for #28; the placeholder
  target shipped here is only what makes a fresh clone load, not a commitment.
