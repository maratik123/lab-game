# Design: Configuration layer — runtime settings and the balance/world constant files

**Issue:** #18
**Date:** 2026-09-04

> **Claim tags in this document.** Repo facts are pinned to the commit they were read
> at (`a73040b`). Facts about an **external module or the standard library** have no repo
> path, so their pin is the **module version** (or `go doc`) the probe ran against; the
> probe modules live outside the repo under the session scratchpad. Claims about
> artefacts this task creates carry `[derived → …]` and no locator.
> `docs/DESIGN.md` is cited **by section only, never by line**, per the design-writer
> contract — those citations carry no `[measured …]` tag by design.

---

## Approach

`internal/config` is a leaf package with no project imports. It exposes one entry
point that takes an environment-lookup function and returns a fully validated,
immutable configuration value or an error naming every rejected key. The sources —
the process environment, the balance file, the world set — are disjoint by domain
(spec Key decisions): the environment supplies secrets, runtime settings and the file
paths; the balance YAML supplies every game constant; the world set supplies world
content and is not decoded here.

### D1 — The loader takes an injected `Lookup`, not `os.Getenv`

```
type Lookup func(key string) (value string, ok bool)
func Load(lookup Lookup) (*Config, error)
```

`cmd/bot` passes `os.LookupEnv`. Consequences the ACs need:

- tests never touch the process environment, so every case may run under `t.Parallel()`
  — `t.Setenv` mutates the whole process and is unusable there
  [measured Go stdlib · `go doc testing.T.Setenv` → "Because Setenv affects the whole
  process, it cannot be used in parallel tests or tests with parallel ancestors."];
- AC16's "consults no environment variable outside the documented set" becomes a
  **behavioural** test rather than a prose claim — a recording `Lookup` captures the key
  set the loader actually queries;
- the boolean result distinguishes *absent* from *set-to-empty*, which AC2 and AC10 treat
  differently, and which the stdlib lookup already reports
  [measured Go stdlib · `go doc os.LookupEnv` → `func LookupEnv(key string) (string, bool)`
  … "If the variable is present in the environment the value (which may be empty) is
  returned and the boolean is true."].

`Load` returns `*Config` rather than a value because `Config` embeds the whole `Balance`
tree, so a pointer keeps every call site from copying it; the returned value is
documented as read-only rather than defended by the type system. This is **not** a lint
requirement: `gocritic`'s `hugeParam` check is not active under this repo's configuration
[measured a73040b:.golangci.yml · a scratch package passing a large struct by value
through `golangci-lint run --config /home/syt/lab-game/.golangci.yml ./...` → no
`gocritic` finding]. Recorded so the rationale is not later "restored" to a lint that
never fired.

### D2 — YAML parser: `go.yaml.in/yaml/v3`, a new direct production requirement

Required by `AGENTS.md` § Dependency Versions (the established-packages AXIOM and the
stated-reason rule) [measured a73040b:AGENTS.md:132-143 · `grep -n -A60 '^## Dependency Versions' AGENTS.md`].

**Chosen: `go.yaml.in/yaml/v3` @ `v3.0.5`** — the YAML org's maintained fork of
`go-yaml/yaml`.

- maintained, not archived [measured go.yaml.in/yaml/v3 · `gh api repos/yaml/go-yaml --jq '{archived,pushed_at,description}'` → `{"archived":false,"description":"The YAML org maintained fork of https://github.com/go-yaml/yaml","pushed_at":"2026-09-02T21:45:14Z"}`];
- published through `v3.0.5` [measured · `go list -m -versions go.yaml.in/yaml/v3` → `go.yaml.in/yaml/v3 v3.0.2 v3.0.3 v3.0.4 v3.0.5`];
- **no non-stdlib requirements of its own** [measured go.yaml.in/yaml/v3@v3.0.5 · `cat $(go env GOMODCACHE)/go.yaml.in/yaml/v3@v3.0.5/go.mod` → `module go.yaml.in/yaml/v3` / `go 1.16` / a `retract [v3.0.0, v3.0.1]` directive and nothing else];
- exposes the `Node` tree (with `Kind`, `Tag`, `Value`, `Content`, `Line`, `Column`) and
  `Decoder.KnownFields` [measured go.yaml.in/yaml/v3@v3.0.5 · `go doc go.yaml.in/yaml/v3 Node` and `go doc go.yaml.in/yaml/v3 Decoder` → the `Node` struct with those fields; `func (dec *Decoder) KnownFields(enable bool)`].

**Rejected — `gopkg.in/yaml.v3`** (the incumbent, already resolved in this module's
graph through a **test-only** path [measured a73040b:go.mod:73 · `go mod why -m gopkg.in/yaml.v3` → `internal/store` → `pgx/v5` → `pgx/v5.test` → `testify/assert` → `testify/assert/yaml` → `gopkg.in/yaml.v3`]). Adopting it costs no new
module, but its upstream is **archived** [measured gopkg.in/yaml.v3@v3.0.1 · `gh api repos/go-yaml/yaml --jq '{archived,pushed_at}'` → `{"archived":true,"pushed_at":"2025-04-01T17:00:11Z"}`], it has no release past `v3.0.1` [measured · `go list -m -versions gopkg.in/yaml.v3` → `gopkg.in/yaml.v3 v3.0.0 v3.0.1`], and the maintained successor's own `go.mod` **retracts** those tags (see the `retract` line quoted above). The AXIOM's table names "the package is unmaintained or abandoned" as the argument that *counts*; taking the abandoned one while its drop-in successor is maintained runs that argument backwards. `gopkg.in/yaml.v3` **stays** in `go.mod` as a test-only indirect requirement through testify — this task does not remove it, and nobody should "tidy it away".

**Rejected — `github.com/goccy/go-yaml`**: maintained [measured · `gh api repos/goccy/go-yaml --jq '{archived,pushed_at}'` → `{"archived":false,"pushed_at":"2026-04-11T15:18:13Z"}`] and dependency-free, but it is a different API and a far larger parser. The YAML-org fork is the direct continuation of the API every behaviour below was measured against, so it wins on equivalence — not because goccy is unmaintained. **Escape hatch:** if the fork ever stalls, goccy is the replacement. The swap is contained by construction: the parser is imported from the balance-loading file alone (D3, and the file list in § Decomposition subtask 1), so no other file in the package names it.

**Rejected — `sigs.k8s.io/yaml`**: it requires `go.yaml.in/yaml/v2`, `go.yaml.in/yaml/v3`, `github.com/google/go-cmp` and `sigs.k8s.io/randfill` [measured sigs.k8s.io/yaml@v1.6.0 · `cat $(go env GOMODCACHE)/sigs.k8s.io/yaml@v1.6.0/go.mod` → its `require` block naming those modules] — strictly more modules to reach the same parser, and it routes YAML through `encoding/json`, discarding the node tags and positions the key-path errors depend on.

**Not an option — a stdlib-only format.** The balance-set format is fixed to YAML by
owner instruction (spec Key decisions).

### D3 — Balance validation: a key-path schema over the parser's node tree

The requirement that decides the shape is AC6: **no balance value has a compiled-in
fallback**. Any binder that writes into a Go value struct cannot separate "the key was
absent" from "the operator wrote 0", because the destination already holds the zero
value. The parser makes that concrete — a YAML null decodes into `int`,
`time.Duration` **and** `decimal.Decimal` with **no error and a zero result**:

```
[measured go.yaml.in/yaml/v3@v3.0.5 · probe: Node.Decode of `a: ~` (tag !!null) →
   int  err=<nil> val=0
   dur  err=<nil> val=0s
   dec  err=<nil> val=0]
```

So the loader reads the parse tree **before** binding. The design pairs:

1. **A schema** — a list of entries, each carrying a dotted key path, the Go
   destination it binds into, and the project's own predicate for that key (a stamina
   cap is positive; a monster-budget distance exponent is at most linear, `docs/DESIGN.md` §4.6).
2. **One walk** over the document's mapping nodes that classifies every node against
   that list, producing every AC's message from the same place and always naming the
   dotted path: **missing** (AC3), **wrong shape / failed predicate** (AC4),
   **unknown** (AC5), and — for an empty document — every path missing at once (AC6).

The schema is data the project owns; the walk is a screenful over the parser's own
documented AST. Neither reimplements YAML.

**Rejected — `github.com/knadh/koanf/v2`** (maintained [measured · `gh api repos/knadh/koanf --jq '{archived,pushed_at}'` → `{"archived":false,"pushed_at":"2026-09-02T07:23:56Z"}`]): its binder is `mapstructure`, which lands on exactly the absent-vs-zero problem above; recovering AC3/AC6 through `Exists`-per-path reproduces this same schema table *on top of* koanf's module set — koanf/v2 alone requires `go-viper/mapstructure/v2`, `knadh/koanf/maps` and `mitchellh/copystructure` (plus `mitchellh/reflectwalk` indirect) [measured github.com/knadh/koanf/v2@v2.3.6 · `cat $(go env GOMODCACHE)/github.com/knadh/koanf/v2@v2.3.6/go.mod` → those `require` blocks], with its YAML parser and file provider as further separate modules [measured · `go list -m -versions github.com/knadh/koanf/parsers/yaml` → `… v1.1.0 v1.1.1`]. **Escape hatch, named:** if the balance set ever needs layering, overlays or defaults (spec Deferred rows), koanf is the package to adopt — its dotted key syntax is the same one this schema already speaks, so the table survives the migration.

**Rejected — `spf13/viper`** (maintained [measured · `gh api repos/spf13/viper --jq '{archived,pushed_at}'` → `{"archived":false,"pushed_at":"2026-01-12T21:42:47Z"}`]): its model is package-level state plus `SetDefault`, i.e. a silent compiled-in fallback for every key — the precise defect AC6 exists to prevent — and AC1 forbids package-level mutable state outright.

### D4 — Environment layer: the standard library, because it covers the requirement

The AXIOM's own clarification: *"prefer the standard library" means stdlib over a
third-party package* — and stdlib covers this end to end. `os.LookupEnv` gives
presence-distinct-from-empty; `strconv.ParseInt` parses chat ids; `net/url.Parse` plus
`URL.IsAbs` and a scheme check validate the Bot API base URL (AC9). No bespoke parser
is written for anything the stdlib already parses.

**Evaluated — `github.com/caarlos0/env/v11`**: maintained and dependency-free [measured · `gh api repos/caarlos0/env --jq '{archived,pushed_at}'` → `{"archived":false,"pushed_at":"2026-09-04T00:59:31Z"}`; `cat $(go env GOMODCACHE)/github.com/caarlos0/env@v11.4.1/go.mod` → `module`, `go 1.18` and `retract` directives only]. It would supply `required` and type parsing, but (a) it covers only the environment source, so the loader still owns the balance walk and the error type; (b) AC10's "empty fails" and AC9's absolute-URL rule are project predicates it does not express, so those checks get written either way; (c) AC16 wants the **consulted** key set observable, and the hook it offers is `Options.OnSet`, which fires when a value *is set* — not on every key queried [measured github.com/caarlos0/env/v11@v11.4.1 · `go doc github.com/caarlos0/env/v11 Options` → `OnSet OnSetFn // OnSet allows to run a function when a value is set.`; the only injection field is `Environment map[string]string`]. **Escape hatch:** adopt it if the variable set outgrows a screen.

### D5 — Package surface

```
type Lookup func(key string) (value string, ok bool)

type Secret string        // String/GoString render "[redacted]"; Reveal returns the value
type Config struct {
    BotToken       Secret
    DSN            Secret
    BotAPIBaseURL  url.URL   // by value: no aliasing, no "do not mutate" contract
    AllowedChatIDs []int64   // slice, not a map — order is the file's, so the value is deterministic
    WorldPath      string    // the path only; the package declares NO world type (AC15)
    Balance        Balance   // nested sub-structs mirroring the YAML tree
}

func Load(lookup Lookup) (*Config, error)
func EnvKeys() []string      // a freshly built slice — the declared variable set

type KeyError struct{ Key string; Err error }   // Error() renders "config: <key>: <cause>"
var ErrMissing, ErrUnknownKey, ErrInvalidValue, ErrUnreadable error
```

- **`Secret`** for the values whose leak is an incident (`AGENTS.md` § Permissions —
  a leaked token is rotated through BotFather, not edited out of history). This is the
  one place the secrets sit together in a single struct, so a future `%v` of it is the
  cheapest leak path. Redaction covers `%v`/`%s`/`%#v`; it is a guard-rail, not a
  guarantee (`%q` still prints the underlying string) and the doc comment says so. Cheap
  to revert to plain `string` while the package has no callers.
- **`Balance` nests to mirror the YAML tree**, so `raid.stamina.cap` in an error message
  and `cfg.Balance.Raid.Stamina.Cap` at a call site read as the same path — closing the
  spec's open question in favour of sub-structs.
- **Numeric types, one rule:** counts are `int`; timers are `time.Duration`; everything
  else is `shopspring/decimal.Decimal`. No `float32`/`float64` anywhere on the config
  surface, extending KD-18's ledger rule rather than inventing a second convention.
- **AC1, package-level state and `init`:** the package declares **no `init` function**,
  and the schema is returned by a function rather than held in a package `var`, so there
  is no mutable table at package scope. The error sentinels are package `var`s, following
  the precedent already in the tree [measured a73040b:internal/store/errors.go:5-49 · `sed -n '1,60p' internal/store/errors.go` → `var ( ErrNoBasis = errors.New(…) … )`]; AC1's "mutable state" is read as *held configuration*, not as error identity.
- **All failures at once:** env failures are joined with `errors.Join`, and so are
  balance failures, in schema order — deterministic, never map-iteration order
  (`AGENTS.md` § Code Style). `Load` validates the environment first, because the file
  paths come from it; the world probe and the balance load then both run so a single
  message can report both.

### D6 — Parser behaviours the walk must handle (all measured)

| Behaviour | Consequence for the design |
|---|---|
| A YAML null decodes into `int`, `time.Duration` and `decimal.Decimal` with no error and a zero value [measured go.yaml.in/yaml/v3@v3.0.5 · probe of `a: ~` → `int err=<nil> val=0`, `dur err=<nil> val=0s`, `dec err=<nil> val=0`] | The walk **rejects a `!!null` node at any schema path** before binding. Without this, `cap:` with nothing after it is a silently defaulted balance number. |
| A `!!float` node decodes into `int` by truncation, with no error [measured go.yaml.in/yaml/v3@v3.0.5 · probe of `a: 3.25` into `int` → `err=<nil> val=3`] | An integer-typed entry additionally requires `Tag == "!!int"`. |
| Duplicate mapping keys are **not** reported when unmarshalling into `yaml.Node`, but **are** reported when unmarshalling into a map — at every depth [measured go.yaml.in/yaml/v3@v3.0.5 · probe: `a:\n  b: 1\n  b: 2\n` → into `yaml.Node` `err=<nil>`; into `map[string]any` `err=… line 3: mapping key "b" already defined at line 2`] | A **duplicate-key pre-pass** unmarshals the raw bytes into a map and returns that error as-is. It already names the key and both lines, so the library's own check is used rather than a hand-rolled one. |
| An alias node resolves transparently through `Node.Decode` [measured go.yaml.in/yaml/v3@v3.0.5 · probe of `x: &a 7` / `y: *a` → alias node `Kind=16`, `Decode` `err=<nil> val=7`] | The walk **rejects alias nodes at schema paths**, naming the path: the balance file is a literal table, and an anchor makes "which number is in force" a two-hop read. A merge key surfaces as a literal `<<` key [measured — same probe → `sec keys: "<<"(tag=!!merge) "q"(tag=!!str)`] and is therefore rejected by the unknown-key rule with no extra code. |
| An empty document yields a `Node` with no content; `{}` yields a document node with an empty mapping [measured go.yaml.in/yaml/v3@v3.0.5 · probe → `empty: err=<nil> kind=0 content=0`; `braces: err=<nil> docContent=0`] | Both are normalised to "an empty mapping", so AC6 reports **every** schema path as missing instead of a special-cased "file is empty". |
| A non-mapping document root parses without error [measured go.yaml.in/yaml/v3@v3.0.5 · probe of `- 1\n- 2\n` → `err=<nil> docKind=2 docTag=!!seq`] | The walk rejects a non-mapping root, naming the file. |
| `time.Duration` accepts a `!!str` (`30m`) and **rejects** a bare `!!int` [measured go.yaml.in/yaml/v3@v3.0.5 · probe → `a: 30m` `val=30m0s err=<nil>`; `a: 30` `err=… cannot unmarshal !!int '30' into time.Duration`] | Durations are written in Go duration syntax; no unit-less-integer footgun exists, so no extra tag check is needed beyond the null rule. |
| `decimal.Decimal` decodes from `!!int`, `!!float` and `!!str`, and errors on a non-numeric string [measured go.yaml.in/yaml/v3@v3.0.5 + shopspring/decimal@v1.4.0 · probe → `0.25`→`0.25`, `3`→`3`, `"0.25"`→`0.25`, `oops`→`error decoding string 'oops': can't convert oops to decimal`] | A decimal entry requires `Tag ∈ {!!int, !!float}`, so a quoted number is rejected too — one spelling per number in the file. |

Any `switch` on `yaml.Kind` carries a `default` clause; the lint config sets
`default-signifies-exhaustive: true`, so a `default` satisfies `exhaustive`
[measured a73040b:.golangci.yml:40-41 · `grep -n -A2 'exhaustive:' .golangci.yml` → `    exhaustive:` / `      default-signifies-exhaustive: true`].

### D7 — Lint constraints already measured against this repo's config

- **`gosec` G304 fires on `os.ReadFile(p)` and on `os.Open(p)` with a variable path**
  [measured a73040b:.golangci.yml · a scratch package run through
  `golangci-lint run --config /home/syt/lab-game/.golangci.yml ./...` →
  `G304: Potential file inclusion via variable (gosec)` at each such call site]. Reading a
  file the operator named **is** the feature, so each such site carries
  `//nolint:gosec // G304: …` with a reason — which clears the finding and satisfies
  `nolintlint`'s `require-specific` and `require-explanation`
  [measured a73040b:.golangci.yml:42-44 · same run with the annotated source → the G304
  findings are gone and no `nolintlint` finding appears].
- **`defer f.Close()` is flagged by `errcheck`** under this config [measured a73040b:.golangci.yml · same run → `Error return value of 'f.Close' is not checked (errcheck)`]. The world-path probe therefore opens, closes, and handles **both** errors explicitly — no `defer`, and never `_ = err` (`AGENTS.md` § Code Style).

### D8 — Where the balance set is read from: an operator-supplied path, never embedded

Closing the spec's open key decision. A `go:embed` copy would be a second source of
truth for the same numbers and a silent fallback when the file is absent — the defect
AC6 exists to prevent. One path, one failure mode. Cost: a deployment ships the file;
that is the infrastructure pass's job, and the reload policy is start-up only anyway.
Each file path is a **required** environment variable with no compiled-in default; the
"default path" the ACs speak of is the value `.env.example` documents, resolved relative
to the process working directory.

### D9 — The balance key set

The file ships **exactly the axes AC7 enumerates** and nothing more: `docs/DESIGN.md`
§16.5's list, plus chunk size, the boss-reward door TTL and AFK-leader cruelty. A later
mechanic appends its own keys to the same file and the same schema in its own PR —
that is the owner's one-file decision working as designed, and it keeps this task from
inventing keys for mechanics that do not exist. Adding a key to one side only is
rejected at start-up (missing from the file, or unknown in the file), so the file and the
schema cannot drift.

Predicates are **single-key only**. Cross-key relations (a head-start window shorter
than the backpack TTL, a fumble face below the critical face) are relations between keys
rather than per-key ranges, and the spec defers range work to #46; inventing them here would be
redesigning balance.

| Key path | Type | Predicate | Placeholder | Source |
|---|---|---|---|---|
| `world.chunk.cols` | int | `> 0` | `16` | §2.2.2 (*ориентир* 16×16) |
| `world.chunk.rows` | int | `> 0` | `16` | §2.2.2 |
| `raid.stamina.cap` | decimal | `> 0` | placeholder | §3.3, §16.5 |
| `raid.stamina.step_cost` | decimal | `> 0` | placeholder | §3.3, §16.5 |
| `raid.standing.calm_to_noises` | duration | `> 0` | `5m` | §5 (*> ~5 минут*), §3.5 |
| `raid.standing.noises_to_wave` | duration | `> 0` | placeholder | §3.5 |
| `raid.standing.wave_to_wave` | duration | `> 0` | placeholder | §3.5, §5 |
| `raid.death.backpack_ttl` | duration | `> 0` | placeholder | §3.4, §16.5 |
| `raid.death.own_chat_head_start` | duration | `> 0` | placeholder | §3.4 |
| `raid.death.respawn_debuff` | duration | `>= 0` | placeholder | §3.4, §16.5 |
| `raid.afk.cruelty` | decimal | `0 <= x <= 1` | placeholder | §3.5 (*Жестокость — конфиг*) |
| `raid.door.price_base` | decimal | `> 0` | placeholder | §2.3, §16.5 |
| `raid.door.price_per_distance` | decimal | `> 0` | placeholder | §2.3 |
| `raid.door.price_distance_exponent` | decimal | `>= 1` | placeholder | §2.3 |
| `raid.door.boss_reward_ttl` | duration | `> 0` | `30m` | §2.2.3 (*TTL ~30 мин (конфиг)*) |
| `raid.monster_budget.base` | decimal | `> 0` | placeholder | §4.6, §16.5 |
| `raid.monster_budget.per_distance` | decimal | `> 0` | placeholder | §4.6 |
| `raid.monster_budget.distance_exponent` | decimal | `0 < x <= 1` | placeholder | §4.6 (*линейно или чуть медленнее*) |
| `raid.monster_budget.scaling_per_level` | decimal | `> 0` | placeholder | §4.6 (*≈ +3% за уровень*) |
| `combat.hit_die_sides` | int | `> 0` | `20` | §4.2 (d20) |
| `combat.base_defence` | int | `> 0` | `10` | §4.2 (*10 + DEF*) |
| `combat.critical_natural` | int | `> 0` | `20` | §4.2 |
| `combat.fumble_natural` | int | `> 0` | `1` | §4.2 |
| `combat.weapon_dice_count` | int | `> 0` | `2` | §4.2 (*≈2d6*) |
| `combat.weapon_die_sides` | int | `> 0` | `6` | §4.2 |
| `combat.initiative_die_sides` | int | `> 0` | `4` | §4.4 (*SPD + d4*) |
| `combat.max_rounds` | int | `> 0` | placeholder | §4.4 (*лимит N раундов*) |
| `combat.vulnerability_multiplier` | decimal | `> 1` | `1.5` | §4.3 (*+50%*) |
| `combat.resist_multiplier` | decimal | `0 < x < 1` | `0.5` | §4.3 (*−50%*) |
| `economy.shop.sell_rate` | decimal | `0 < x < 1` | placeholder in the 0.20–0.30 band | §6.3, §16.5 |
| `economy.shop.buy_markup` | decimal | `> 0` | placeholder | §6.4, §16.5 |

Placeholders take the design's own reference number where §-cited above (the spec's Key
decision); every other cell is an obviously-placeholder value that loads clean and
carries no authority — #46 replaces the lot. **The combat-system version is not here**:
it identifies the code that produced a stored log (`docs/DESIGN.md` §4), so it is a
constant in the combat package, not a tuning value an operator may edit.

### D10 — The environment variable set

`LAB_GAME_` prefix, matching the existing `LAB_GAME_TEST_DSN`
[measured a73040b:internal/testdb/testdb.go:31 · `sed -n '31p' internal/testdb/testdb.go` → `const dsnEnv = "LAB_GAME_TEST_DSN"`]. Every variable is required; none has a compiled-in default.

| Variable | Validation | AC |
|---|---|---|
| `LAB_GAME_BOT_TOKEN` | present and non-empty | AC2 |
| `LAB_GAME_DSN` | present and non-empty; **not parsed here** — `store.NewPool` already owns DSN parsing, and importing pgx into `config` would couple configuration to storage | AC2 |
| `LAB_GAME_BOT_API_BASE_URL` | parses via `net/url`, absolute, scheme `http` or `https`, non-empty host | AC9 |
| `LAB_GAME_ALLOWED_CHAT_IDS` | present, non-empty, comma-separated; every element parses as `int64`, negative values accepted | AC10 |
| `LAB_GAME_BALANCE_PATH` | present, non-empty; the file is read and decoded | AC2, AC3–AC7 |
| `LAB_GAME_WORLD_PATH` | present, non-empty; the target opens and closes cleanly (existence **and** readability); nothing inside is read | AC15 |

The world placeholder target is the **directory** `config/world/`, tracked by an empty
marker file. A directory constrains #28 least: it may fill the directory, or repoint the
variable at a single file — the loader accepts either, so #28 needs no loader change.

### D11 — Testability of `cmd/bot` without running a built binary

`main` becomes `os.Exit(run(os.LookupEnv, os.Stderr, os.Stdout))`, and `run` returns an
exit code. The AC11 behaviour is then tested by calling `run` in-process, which sidesteps
the spec's permission constraint on invoking a compiled artefact by bare path. `main`
exiting non-zero is not a panic and adds no row to the panic index
[measured a73040b:ai-docs/panic-index.md:7-9 · `grep -n '^| ' ai-docs/panic-index.md` → the header row and a `| — | — | — |` placeholder row, i.e. the table is empty].

### D12 — CI paths-filter

`.github/workflows/ci.yml`'s `go` filter matches `**/*.go`, `**/*.sql`, `go.mod`,
`go.sum`, `.golangci.yml`, `Makefile` and `.github/workflows/**`
[measured a73040b:.github/workflows/ci.yml:37-52 · `sed -n '37,52p' .github/workflows/ci.yml`],
and the file's own comment states that a future artefact must be added in the PR that
introduces it or its gate silently stops running
[measured a73040b:.github/workflows/ci.yml:35-36 · same read → `# Any future Go or harness artefact must be added here in the same PR` / `# that introduces it, or its gate silently stops running.`].
The tracked balance file, the world placeholder and `.env.example` are read by Go tests,
so editing one without the filter update would skip the job that proves it — "a job that
did not run is not a passing job" (`AGENTS.md` § Build & Test). They join the `go` filter,
and `actionlint` runs on the changed workflow before `git add` (`AGENTS.md` AXIOM).

---

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | The balance schema and its walker: the `Balance` nested types, the schema entries (path · destination · predicate) built by a function, the duplicate-key pre-pass, and the node walk producing missing / unknown / null / wrong-tag / failed-predicate errors, each naming its dotted path. Adds the parser: `go get go.yaml.in/yaml/v3@latest`, `go mod tidy`, then read `git diff go.mod go.sum` before staging, and record the version that resolved (`v3.0.5` was the newest published at design time — D2). | `internal/config/balance.go`, `internal/config/balance_load.go`, `internal/config/errors.go`, `internal/config/balance_load_test.go`, `go.mod`, `go.sum` | — |
| 2 | The tracked balance set at the default path, carrying a placeholder for every schema key, plus the both-directions agreement test (every schema path present in the file; every file path known to the schema). | `config/balance.yaml`, `internal/config/balance_file_test.go` | 1 |
| 3 | The environment layer: `Lookup`, `EnvKeys`, the variable-name constants, and validation of token, DSN, base URL and `ALLOWED_CHAT_IDS` — every failure a `*KeyError` naming its variable, joined in declaration order. | `internal/config/env.go`, `internal/config/env_test.go` | 1 |
| 4 | World-set path resolution: the required variable, the open/close readability probe with both errors handled, `WorldPath` exposed as a string, and the tracked placeholder target at the default path. | `internal/config/world.go`, `internal/config/world_test.go`, `config/world/<marker>` | 3 |
| 5 | `Load`: the package doc comment (reload policy + each source's exclusive domain, AC13), `Config`, `Secret`, and the composition of the environment, the world probe and the balance load into one joined error. | `internal/config/config.go`, `internal/config/config_test.go` | 1, 3, 4 |
| 6 | `.env.example` with a placeholder for every required variable, and the disjointness tests: the recording-`Lookup` read set equals `.env.example`'s key set and `EnvKeys()`; the tracked balance values are identical across differing environments. Adds `github.com/joho/godotenv` as a **test-only** requirement to parse `.env.example`. | `.env.example`, `internal/config/disjoint_test.go`, `go.mod`, `go.sum` | 2, 5 |
| 7 | `cmd/bot` wired to load and validate before any other work: `main` delegates to a testable `run`, which writes the key-naming message to stderr and returns a non-zero code. | `cmd/bot/main.go`, `cmd/bot/main_test.go` | 5 |
| 8 | CI paths-filter: add the tracked config artefacts to the `go` filter so their gates run; `actionlint` on the changed workflow before staging. | `.github/workflows/ci.yml` | 2, 4, 6 |
| 9 | Propagation sweep and the prose sites the diff falsifies (details and their measured current wording in § Propagation targets below): the `go run ./cmd/bot` line, the "cmd/bot is still the scaffold" status and package layout, the new key decisions (parser, disjoint sources, start-up-only reload), the reload-policy clause in the balance-numbers invariant, and the plans index row. Membership decided by `AGENTS.md` § Propagation Rule step 4, not by this list. | `AGENTS.md`, `ai-docs/context.md`, `ai-docs/key-decisions.md`, `ai-docs/domain-invariants.md`, `ai-docs/plans/INDEX.md` | 1–8 |

### Propagation targets (subtask 9)

Each row below is a claim that exists in the tree today and that this diff falsifies or
under-states. The list is **illustrative of the class, not a bound on it** — the sweep
`grep -rni` over `.claude/`, `AGENTS.md` and `ai-docs/` decides membership, and repo-root
user-facing docs are swept too (Propagation Rule step 4).

| Site | Current wording | Why the diff touches it |
|---|---|---|
| `AGENTS.md` § Build & Test | `go run ./cmd/bot                                        # run the bot` [measured a73040b:AGENTS.md:53 · `sed -n '53p' AGENTS.md`] | The command now fails without the documented environment; the comment must say so. |
| `ai-docs/context.md` § Status | "the ledger core — `internal/store` … and `internal/testdb`; `cmd/bot` is still the scaffold" [measured a73040b:ai-docs/context.md:43 · `sed -n '43p' ai-docs/context.md`] | `internal/config` joins the layout and `cmd/bot` stops being a pure scaffold. |
| `ai-docs/domain-invariants.md` § 8 | "A tuning value compiled into Go source is a defect even when it carries a good name: the season's balance is expected to move without a deploy." [measured a73040b:ai-docs/domain-invariants.md:68 · `sed -n '68p' ai-docs/domain-invariants.md`] | The reload policy is now decided (start-up only, spec Key decisions). The clause is amended to name it, so a reader does not infer hot reload; the rest of § 8 is untouched and remains the reason this task exists. |
| `ai-docs/key-decisions.md` | — | New KDs: the YAML parser and its rejected alternatives (D2), disjoint-by-domain sources with no override chain and therefore no key-path merge machinery (§ Approach, D10), start-up-only reload and no embedded balance copy (D8). |
| `ai-docs/plans/INDEX.md` | — | The row for this plan pair, per the file's own maintenance contract. |

---

## Handoff plan

Change-type classification used below: **code** covers the program and the build/runtime
artefacts its gates read — `*.go`, `go.mod`/`go.sum`, the tracked `config/**` data files,
`.env.example`, and `.github/workflows/**`. **Instructions/harness** covers the
agent-facing prose enumerated by rule (e): `*.md`, `.claude/**`, `AGENTS.md`,
`ai-docs/**`. Subtask 9 is the only instructions/harness subtask, and it depends on all
the others, so two groups is the minimum reachable count.

- **Entry into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). The handoff binds at the start of every group, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token window — subtasks 1–8 (code change-type). All same-change-type subtasks clustered into one group rather than interleaved; 8 subtasks, within the size cap of 10.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator (typically xHigh), 1M-token window — subtask 9 (instructions/harness change-type: `AGENTS.md`, `ai-docs/**`). Terminal group (1 subtask; within the `1..=10` range).

Two groups, within the default maximum of 4 — no user gate needed.

---

## Risks

- **The parser promotion perturbs `go.mod` beyond the one line expected.** `go mod tidy` prunes and adds transitive lines, so the diff is read before staging and the `tidy-check` gate re-run — `AGENTS.md` § Dependency Versions forbids hand-editing a version. `gopkg.in/yaml.v3` must remain as a test-only indirect requirement and must not be removed as "now unused" — `[derived → subtask 1's gate step: `go mod tidy` then `git diff go.mod go.sum` read before `git add`, and AC14]`.
- **A silently defaulted balance number — the defect this layer exists to prevent — reaches production through a YAML null or a truncated float.** Both are measured library behaviours (D6), and both are closed by explicit node-tag rules rather than by trusting the binder — `[measured go.yaml.in/yaml/v3@v3.0.5 · probe of `a: ~` and `a: 3.25` → `int err=<nil> val=0` and `int err=<nil> val=3`]`.
- **A duplicated key in the balance file silently picks one value.** The node tree does not report duplicates; a map decode does, at every depth. The pre-pass uses the library's own check — `[measured go.yaml.in/yaml/v3@v3.0.5 · probe of a nested duplicate → into `yaml.Node` `err=<nil>`; into `map[string]any` `err=… mapping key "b" already defined at line 2`]`.
- **The lint gate rejects the natural spelling of the file reads.** `gosec` G304 fires on a variable path and `errcheck` on `defer f.Close()`; both were run against this repo's config, and the design fixes the spelling that passes — `[measured a73040b:.golangci.yml · scratch package through `golangci-lint run --config /home/syt/lab-game/.golangci.yml ./...` → `G304: Potential file inclusion via variable (gosec)` and `Error return value of 'f.Close' is not checked (errcheck)`; the annotated form clears both]`.
- **A test that reads the tracked config files resolves them relative to the package directory, not the repo root.** `.env.example` documents repo-root-relative paths, so any test driven by it must rewrite the path values through a repo-root prefix or it fails on a correct tree — `[derived → subtask 6's example-environment test and its repo-root helper]`.
- **A change to `config/balance.yaml`, `config/world/**` or `.env.example` alone skips the job that validates it**, because the CI `go` filter does not currently match them — `[measured a73040b:.github/workflows/ci.yml:37-52 · `sed -n '37,52p' .github/workflows/ci.yml` → the `go:` filter lists `**/*.go`, `**/*.sql`, `go.mod`, `go.sum`, `.golangci.yml`, `Makefile`, `.github/workflows/**` and nothing under `config/` or `.env.example`]`. Closed by subtask 8.
- **The "unreadable path" half of AC15 is not testable in a uid-independent way.** A `chmod 0` fixture is defeated when the suite runs as root, which would make the case pass vacuously rather than fail loudly. The negative cases are therefore a non-existent path and a path whose parent is a regular file — both deterministic for any uid — and the chmod case is deliberately not written; `AGENTS.md` § Go Test Conventions treats a vacuous pass as worse than an absent test — `[derived → subtask 4's world-path test table]`.
- **The panic invariant.** Nothing in this design panics: the loader returns errors and `main` exits non-zero, which the index explicitly excludes from needing a row. No row is added — `[measured a73040b:ai-docs/panic-index.md · `grep -n '^| ' ai-docs/panic-index.md` → header plus `| — | — | — |`, i.e. an empty table]`.
- **No balance is moved and no schema is migrated by this task**, so the ledger and forward-migration rules impose no obligation here: the package writes nothing to Postgres and declares no persisted enum — `[derived → the file list in § Decomposition, which contains no `internal/store/migrations/**` entry, and AC14]`.

---

## Test Design

Every claim below is about a test that does not exist yet. All cases run without a
database and without network access (AC12); no test in this package imports
`internal/testdb`.

### Subtask 1 — the balance walk (`internal/config/balance_load_test.go`)

- **Entry point:** the unexported balance loader, called with a path under `t.TempDir()`.
- **Fixture:** a helper that writes a YAML string to a temp file and returns its path; a
  helper that renders a **valid** document from the schema, so each negative case is one
  documented mutation of a known-good baseline rather than a hand-typed near-miss.
- **Table cases (each asserts `errors.Is` on the sentinel, `errors.As` recovering the
  `*KeyError` with the expected `Key`, and that the rendered message contains that key):**
  a key removed → missing; a key set to null → invalid; an integer key given a float →
  invalid; a decimal key given a quoted number → invalid; a duration key given a bare
  integer → invalid; a key present but failing its predicate, once per predicate family
  (non-positive, negative, above an upper bound, at an excluded bound) → invalid; a key
  the schema does not define, at top level and nested → unknown; a scalar where the
  schema expects an interior mapping → invalid; an alias at a schema path → invalid; a
  merge key → unknown; a duplicated key → the parser's own duplicate error; a
  non-mapping document root → invalid; an empty document and `{}` → **every** schema
  path reported missing, which is AC6's success condition; a syntactically invalid
  document → a parse error naming the file.
- **Happy path:** the rendered baseline loads and every field of the returned `Balance`
  equals the value written — asserted exactly, not "roughly".
- `[derived → AC3, AC4, AC5, AC6, AC12]`

### Subtask 2 — the tracked balance file (`internal/config/balance_file_test.go`)

- **Entry point:** the same balance loader, pointed at the repository's tracked file
  through a repo-root helper.
- **Scenarios:** the tracked file loads without error; and the agreement test in **both**
  directions — no schema key is absent from the file (the loader already proves this) and
  no key in the file is unknown to the schema (likewise), so a single successful load is
  the agreement proof and the test asserts it as such rather than re-deriving the key set.
- **Why it matters:** this is the test that stops a later mechanic from adding a key to
  the file without adding it to the schema, or the reverse.
- `[derived → AC7]`

### Subtask 3 — the environment layer (`internal/config/env_test.go`)

- **Entry point:** the unexported environment loader, called with a map-backed `Lookup`
  built by a helper.
- **Table cases:** each required variable unset in turn → `ErrMissing`, the message names
  that variable; each required variable set to the empty string → invalid, named; a base
  URL that is relative, that has no host, and that carries a non-`http(s)` scheme → each
  invalid, naming the variable; a chat-id list that is empty, that contains a non-integer
  element, and that contains an empty element → each invalid, naming the variable; a
  well-formed chat-id list including a negative group id → parses to the corresponding
  `[]int64` **in the order written**.
- **Aggregation:** with several variables unset at once, the joined error names each of
  them, and the order is the declaration order on repeated runs — a determinism
  assertion, not an incidental one.
- `[derived → AC2, AC9, AC10, AC12]`

### Subtask 4 — the world-set path (`internal/config/world_test.go`)

- **Entry point:** the unexported world-path probe.
- **Scenarios:** the variable unset → missing, named; a path that does not exist →
  unreadable, naming the variable; a path whose parent is a regular file (so traversal
  fails for any uid) → unreadable, named; an existing directory → success; an existing
  regular file → success, proving the loader constrains neither shape for #28.
- **Deliberately absent:** a `chmod 0` permission case — see § Risks.
- **Structural check:** the returned value is the path string; the package declares no
  type describing the world set's interior (a review-level criterion, restated in the
  package doc comment).
- `[derived → AC15, AC12]`

### Subtask 5 — `Load` (`internal/config/config_test.go`)

- **Entry point:** `Load`.
- **Scenarios:** a complete, valid environment plus temp files → a populated `*Config`;
  an environment failure → the error names the variable and no file is read; a valid
  environment with both a bad world path and a bad balance file → the joined error names
  both; the `Secret` fields render as the redaction marker under `%v`, `%s` and `%#v`,
  and `Reveal` returns the underlying value.
- `[derived → AC1, AC13, AC12]`

### Subtask 6 — disjointness (`internal/config/disjoint_test.go`)

- **Entry point:** `Load` behind a recording `Lookup`; `.env.example` parsed with
  `godotenv.Read`.
- **Scenarios:** the recorded consulted-key set equals `.env.example`'s key set and equals
  `EnvKeys()` — this is AC16's "disjoint in fact, not only in prose", and it also makes
  `.env.example` self-enforcing; every value in `.env.example` is non-empty (AC8's
  placeholder requirement); loading with the example environment, with the path values
  rewritten through the repo-root helper, succeeds (AC7's second sentence);
  loading the tracked balance file under differing environments — varying token, DSN, base
  URL and chat ids — yields `Balance` values that compare equal (AC16's second clause).
- **Why `godotenv` rather than a hand-split:** `.env` quoting, comments and `export`
  prefixes are exactly where a naive splitter goes wrong, and a wrong parse makes this
  test vacuous. It is maintained and requires nothing itself
  [measured github.com/joho/godotenv@v1.5.1 · `gh api repos/joho/godotenv --jq '{archived,pushed_at}'` → `{"archived":false,"pushed_at":"2026-08-04T09:43:36Z"}`; `cat $(go env GOMODCACHE)/github.com/joho/godotenv@v1.5.1/go.mod` → `module github.com/joho/godotenv` and `go 1.12` only; `go doc github.com/joho/godotenv` → `func Read(filenames ...string) (envMap map[string]string, err error)`]. It is imported **only** from `_test.go`, so the production binary never links it — the same posture `internal/testdb` already holds for testcontainers (KD-20). This does not reopen the spec's "the Go process does not parse `.env`" decision: the **loader** still reads the process environment only.
- `[derived → AC7, AC8, AC16, AC12]`

### Subtask 7 — `cmd/bot` (`cmd/bot/main_test.go`)

- **Entry point:** the unexported `run`, with a map-backed `Lookup` and a buffer for each
  of its writers — never a spawned binary.
- **Scenarios:** a missing variable → a non-zero return, and the stderr buffer contains
  that variable's name; a complete valid environment (temp files) → zero, and the stdout
  buffer carries the build identity; configuration is loaded before anything is written
  to stdout, asserted by the failure case leaving stdout empty.
- `[derived → AC11, AC12]`

### Gate-level checks that are not Go tests

- `make verify` green on the resulting tree (AC14); `golangci-lint run` in particular
  confirms the D7 spellings, since the G304/`errcheck` findings above were measured
  against a scratch package rather than the real one.
- `actionlint .github/workflows/ci.yml` before staging subtask 8 (`AGENTS.md` AXIOM).
- No file in the change carries a real bot token, DSN, `api_id` or `api_hash` (AC8) —
  a review-level check over the diff.
- `git check-ignore -q .env.example` exits non-zero while `.env` exits zero, so the new
  file is committable and the secret file stays ignored
  [measured a73040b:.gitignore:8-10 · `git check-ignore -q .env.example; echo $?` → `1`, against `git check-ignore -q .env; echo $?` → `0`].

---

## Open questions

- **Per-key admissible ranges** stay open, as the spec states: this task validates
  presence, shape and the design-stated single-key invariants; real bounds — and the
  cross-key relations D9 deliberately leaves out — arrive with the real numbers in #46.
- **Whether `Secret` earns its ergonomic cost.** It makes accidental formatting of the
  token and DSN structurally unlikely, at the price of a conversion at every future call
  site. Cheap to revert to plain `string` while `internal/config` has no callers; flagged
  here so the choice is made deliberately rather than inherited.
- **Whether the stamina model's remaining axes** (tick interval, regen amount, overdraft
  rate and capacity, `docs/DESIGN.md` §3.3) should ship now or with the stamina mechanic.
  D9 defers them, following the owner's "a new mechanic adds its constants to that file"
  decision; adding them here instead is a one-line-per-key change if the owner prefers a
  fuller placeholder set up front.
- **Whether the world-set path should name a file or a directory** stays open for #28, as
  the spec states. The loader accepts either and the placeholder is a directory only
  because that constrains #28 least.
