# Progress: configuration layer — runtime settings and the balance/world constant files — ACTIVE
_Updated: 2026-09-04 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-config-layer-balance-files
**base_commit:** 869cfb37a773cc24aa2a0f3f705ced93f847422a
**Last build:** PASS

**Issue:** #18
**Spec:** ai-docs/plans/2026-09-04-config-layer-balance-files.spec.md

**current_step:** Step 10 — self-review APPROVE (Round 2)
**last_passed_gate:** make verify + mutation re-test | 2026-09-04T10:04:06Z | 9899d9c60a7e0c57684d24140dc1963f74616fa2
**entry_args:** 18

## Next action

**Do this immediately:** every Step-8 subtask (1-9) is complete and committed on the branch. The orchestrator resumes at `/task` Step 9 (Verify). Note for Step 9.5: `ai-docs/context.md`'s § Status **Code** bullet still reads "`cmd/bot` is still the scaffold", which this diff makes false — subtask 9 deliberately did not touch it (see the Decisions log).

## Subtasks

- [x] 1. Balance schema + walker; provisional package comment in `doc.go`; adds the YAML parser
- [x] 2. Tracked balance set at the default path, placeholder per schema key + curve comments
- [x] 3. Environment layer: `Lookup`, `EnvKeys`, name constants, token/DSN/base-URL/chat-id validation
- [x] 4. World-set path resolution: required variable, readability probe, both errors handled
- [x] 5. `Load`: `Config`, `Secret`, joined error; rewrites `doc.go`'s comment body to AC13 wording
- [x] 6. `.env.example` + disjointness tests; adds `godotenv` as a test-only requirement
- [x] 7. `cmd/bot` wired to load and validate before any other work; `main` delegates to `run`
- [x] 8. CI paths-filter: tracked config artefacts added to the `go` filter; actionlint
- [x] 9. Propagation sweep over the prose sites the diff falsifies

## Decisions log

- **Step 8**: entering implementation at design `869cfb3` (round-2 GO plus the mandated notes fold-in), spec `5285f4c`.
- **Step 8, subtask 1**: `go get go.yaml.in/yaml/v3@v3.0.5` + `go mod tidy` produced the single-line `go.mod`/`go.sum` diff D2 predicted; `gopkg.in/yaml.v3 v3.0.1 // indirect` untouched, confirmed after tidy.
- **Step 8, subtask 1**: the walker unifies "root absent" and "root is an empty mapping" (`{}`) through one code path — `loadBalance` resolves `root *yaml.Node` to `doc.Content[0]` (nil when the parsed document is zero or has no content) and calls `walkNode(root, tree, nil)` unconditionally; `walkInterior`'s `doc == nil` branch recurses `walkNode(nil, child, …)` into every schema child, so both empty/whitespace input and `{}` report every path missing via the same "value node absent for this child" mechanism, and a non-mapping root (a YAML sequence) is rejected by the same function's Kind check — no separate resolveRootMapping function was needed, simplifying design D6's stated approach without changing its observable behaviour (verified: all three `TestLoadBalance_EmptyDocumentReportsEveryPathMissing` subtests plus `TestLoadBalance_NonMappingRoot` pass).
- **Step 8, subtask 1**: found and fixed two bugs while running the fixture tests (not carried in from the design): `raid.monster_budget.distance_exponent`'s predicate was miswired to the open interval (0,1) instead of D9's stated (0,1], caught by `TestLoadBalance_HappyPath` using the design's own placeholder value 1; and the AC6 whitespace-only fixture originally used a tab character, which YAML rejects for indentation and turned the case into a parse-error path rather than a missing-path path — replaced with space-only whitespace.
- **Step 8, subtask 1**: `bindInt`'s `want` parameter trips `unparam` today because every int-typed key in D9 happens to use "positive" — kept (not simplified away) for symmetry with `bindDuration`/`bindDecimal` and because a future int key need not share that predicate; annotated `//nolint:unparam` with that reason rather than dropping the parameter.
- **Step 8, subtask 2**: `config/balance.yaml`'s placeholder values are byte-identical to `balance_load_test.go`'s `validBalanceYAML` fixture (both are "an obviously-placeholder value that loads clean", D9) — not a coincidence to preserve, just the simplest set of numbers that satisfies every predicate; no code shares the two.
- **Step 8, subtask 2**: `repo_root_test.go` derives the repository root from `runtime.Caller(0)` of the test file itself rather than `os.Getwd()`, so `repoRootPath` is correct regardless of which directory `go test` is invoked from; subtask 6 reuses it for the example-environment test (design § Risks).
- **Step 8, subtask 3**: `EnvKeys()` and `envValues` cover all six `LAB_GAME_` variables (including `BALANCE_PATH`/`WORLD_PATH`), not only the four D10 lists as this subtask's own validation scope — `loadEnv` checks presence+non-empty uniformly for all six (the shared, uniform half of D10's rule for the path variables) and leaves the richer checks (file open/close, YAML decode) to subtask 4's world prober and subtask 5's `Load`/balance loader, which is what the decomposition table's per-subtask file list implies. This keeps the declared variable set in one place ahead of AC8/AC16's disjointness test (subtask 6).
- **Step 8, subtask 3**: `gosec` flagged `envBotToken`'s constant declaration as G101 "potential hardcoded credentials" — a false positive, since the string is the environment-variable *name*, never a token value; annotated `//nolint:gosec` with that reason. Not one of D7's pre-measured lint findings, so recorded here for the next reader.
- **Step 8, subtask 4 (correction to subtask 3's scope)**: re-reading the decomposition table's subtask 3 row ("validation of token, DSN, base URL and ALLOWED_CHAT_IDS" — BALANCE_PATH/WORLD_PATH not listed) against subtask 4's Test-Design entry point ("the unexported world-path probe … variable unset → missing, named") showed `loadEnv` originally over-scoped: it also validated `LAB_GAME_BALANCE_PATH`/`LAB_GAME_WORLD_PATH` presence, which would have duplicated the presence check subtask 4's own `resolveWorldPath` needs to do to be independently testable via a `Lookup`, and duplicated (as two separate error rows for the same variable) whatever subtask 5 does for `LAB_GAME_BALANCE_PATH`. Corrected in this commit: `envValues`/`loadEnv` now cover only the four env-only variables (`envKeys()`, unexported); `EnvKeys()` (exported, AC8/AC16's full set) appends `envBalancePath`/`envWorldPath` on top. Each path variable's presence check now lives exactly once, next to the richer validation that consumes it — `resolveWorldPath` here, `Load`'s balance-path handling in subtask 5. `envKeys()` is a function, not a package `var`, to keep AC1's no-package-level-mutable-state rule literal.
- **Step 8, subtask 4**: the chmod-0 permission case is deliberately not written (design § Risks) — `TestResolveWorldPath_ParentIsRegularFile` (a path whose parent is a regular file) is the uid-independent negative case that exercises `ErrUnreadable` instead.
- **Step 8, subtask 5**: `Load` runs `loadEnv`, `resolveWorldPath` and the `LAB_GAME_BALANCE_PATH` presence check unconditionally against the same `lookup`, joining every failure — none of the three needs another to succeed first, since each takes `lookup` directly rather than another's result; `loadBalance` is the only step gated (on `LAB_GAME_BALANCE_PATH` itself resolving), matching the design's "no file is read" case.
- **Step 8, subtask 5**: verified `doc.go` is still the only file in the package whose first line is a package comment (`head -3` swept across every file) before staging — D7's honour-system gate for a second, unnoticed package comment.
- **Step 8, subtask 5**: `Secret`'s `%s` redaction test triggers `staticcheck` S1025 ("should use String() instead of fmt.Sprintf") — a correct suggestion for production code, but the test's whole point is exercising the `%s` verb through `fmt`, not calling `.String()` directly; annotated `//nolint:staticcheck` with that reason.
- **Step 8, subtask 6**: verified `.env.example` is committable before writing it: `git check-ignore -q .env.example; echo $?` → `1` (one path per command, D13(c)) — same result the design measured, re-confirmed live rather than trusted from the design text.
- **Step 8, subtask 6**: `go get github.com/joho/godotenv@v1.5.1` + `go mod tidy` produced the single-line diff design D2/§ Risks predicted; no other module moved.
- **Step 8, subtask 6**: all six disjointness/agreement tests passed on the first run with no fixture bugs — unlike subtask 1's fixture, `.env.example`'s values and `config/balance.yaml` were both already exercised by earlier subtasks' own tests, so this subtask mainly wired existing, already-verified fixtures together.
- **Step 8, subtask 7 — found a bug in design § Test Design's own AC11 verification command.** `rg -n --glob '!*_test.go' --glob '*.go' -e '\bpanic\(' -e '\blog\.Fatal' cmd internal` (as written, negation glob first) does **not** exclude `_test.go` files in this ripgrep build (15.2.0): ripgrep applies globs in argument order and the *last* matching glob wins, so the trailing `--glob '*.go'` re-includes every `_test.go` file the leading negation excluded. The design's own measured "no output, exit 1" was taken before any `_test.go` file under `cmd`/`internal` contained `panic(` or `log.Fatal`, so the bug had nothing to expose at measurement time; `internal/config/balance_load_test.go`'s `dec` helper (subtask 1, `panic(err)` on a hard-coded-valid literal) is what surfaced it. Verified by re-running with the glob order flipped (`--glob '*.go' --glob '!*_test.go'`), which correctly drops `_test.go` files — re-confirmed against a positive-control scratch file in both orders. Fixed on the production side too: `cmd/bot/main.go`'s doc comment originally read "…Never panics and never calls log.Fatal (AC11)…", which the *broken* order's own false-negative test would have matched as a real hit had the doc comment run through the correct-order command — reworded to avoid the literal substring "log.Fatal". The corrected-order command now returns exit 1 with no output over `cmd internal`, matching the design's stated result. This is a harness-gap finding (a design's grep recipe, not project code) — recorded here per `Kind: correction`; `ai-docs/harness-gaps.md` is out of scope for a `code-writer` Mode A subtask to write, so it is left for `/task` Step 9's propagation/self-review pass to route.
- **Step 8, subtask 7**: `run` takes `config.Lookup` directly (not `os.LookupEnv` baked in), so `main` is the only place that ever touches the process environment — `main_test.go` never calls `os.Exit` or spawns a binary, matching the design's "no test invokes a compiled artefact by bare path" constraint.
- **Step 8, subtask 8**: `.github/workflows/ci.yml`'s `go` paths-filter gains `config/**` and `.env.example`; `actionlint .github/workflows/ci.yml` passed clean before staging (AGENTS.md AXIOM). Group A (subtasks 1-8) is now complete; handing off to Group B for subtask 9 per the design's Handoff plan.

- **Step 8, subtask 9 — boundary call on `ai-docs/context.md`.** The design's § Propagation-targets row cites `5285f4c:ai-docs/context.md:43`, which is the § Status **Code** bullet ("`cmd/bot` is still the scaffold"), while the same section's preamble reserves `context.md`'s status bullet for `/task` Step 9.5. Resolved in favour of the explicit carve-out (also this group's spawn instruction): subtask 9 edited the § Architecture "**Layout so far:**" sentence — line 27, prose, not a status bullet — to add `internal/config`, and left line 43 alone. **Outstanding for Step 9.5:** the "still the scaffold" clause and the package list in that Code bullet are now false.
- **Step 8, subtask 9 — the sweep, and the textual hits deliberately left standing.** Swept case-insensitively (`grep -rni` over `.claude/`, `AGENTS.md`, `ai-docs/`, `docs/`, plus `.github/` and `Makefile` where relevant) for `scaffold`, `cmd/bot`, `internal/config`, `env.example`, `LAB_GAME_`, `configuration`, `reload` / `without a deploy`, `balance.yaml` / `config/world`, `run the bot`, and for any directory-layout listing (none exists outside `context.md` and `INDEX.md`). Two `scaffold` hits — `.claude/skills/pr-commented/reference.md:9` and `.claude/skills/ai-audit/reference.md:134` — are the unrelated "template scaffolding" sense, inspected and left verbatim, not reworded. Hits in `ai-docs/plans/done/**`, `ai-docs/learnings.md`, `ai-docs/harness-gaps.md` and `ai-docs/context-status.md`'s past entries are history surfaces: left untouched per the Propagation Rule step-4 completeness test ("every LIVE doc must agree").
- **Step 8, subtask 9**: the new `AGENTS.md` § Build & Test comment was written from a live run, not from the design — `env -u` on all six `LAB_GAME_` variables then `go run ./cmd/bot` exits 1 and names every missing key on stderr, so the comment reads "exits non-zero unless `.env.example`'s variables are exported".
- **Step 8, subtask 9**: KD-22's two load-bearing repo claims were re-measured rather than copied from the design — `grep -rn 'go.yaml.in/yaml' --include='*.go' .` → the single import site `internal/config/balance_load.go:11` (so "the swap stays contained to one file" is true today), and `go mod why -m gopkg.in/yaml.v3` → reached only through `internal/store` → pgx test → testify, i.e. still test-only indirect. D2's explicit warning was honoured: the fork's `retract [v3.0.0, v3.0.1]` is written into KD-22 **as a non-argument**, not as a third reason.
- **Step 8, subtask 9**: verified, not re-done, the two harness commits the design's last propagation row names — `.claude/settings.json`'s deny list holds `.idea/**`, `**/secrets*`, `**/.secrets*`, `**/.env` and the six named variants, with `.env.example` absent, which is what `ai-docs/claude-tools-hierarchy.md:97` describes. One pre-existing under-statement observed and deliberately left: that sentence names `**/secrets*` but not `**/.secrets*`. The omission predates this branch (it survives the `**/.env*` wording `db7999e` replaced) and is not a claim this diff falsifies, so widening it would be unapproved scope.
- **Step 8, subtask 9 — gates for a prose-only diff.** Nothing non-prose moved, so no `actionlint` and no `shellcheck` applied. Run: `go build ./...` → green (sanity check); the CI step *relative markdown links resolve* executed verbatim over the whole tree → `LINKS-GREEN`, and the two new relative links (`INDEX.md` → the spec, `domain-invariants.md` → `key-decisions.md`) additionally traced with `realpath`; `.claude/skills/ai-audit/scripts/check-citations.sh` → `PASS` (local high-water mark PR #49), covering the new `#18` citations.

- **Step 8**: owner's decision — the harness changes on this branch (AGENTS.md hand-rolling axiom, the settings.json deny narrowing, the claude-tools-hierarchy propagation) ship in the SAME PR as the config layer, not split out.

- **Step 9**: no panic-index row owed — the index's own wider scope (panic, log.Fatal*, log.Panic*, must… helpers) run over the go-list non-test file set returns exit 1; the table stays the empty placeholder. No domain-invariant surface: the diff carries no store.Post, posting or item_movements reference, and the issue declares no telemetry obligation.
- **Step 9**: AC11 verified behaviourally, not only by its tests — cmd/bot with all six variables unset exits 1 naming every missing key on stderr with stdout silent; with a valid environment it exits 0. Observed while doing so: the message doubles its prefix ("configuration: config: LAB_GAME_BOT_TOKEN: config: missing: required"), which no AC forbids but reads as an oversight — left for self-review to weigh rather than churned now.
- **Step 9**: AC1's "no package-level mutable state" holds — the only package-level var block is the four error sentinels in errors.go, which is the form AGENTS.md § Code Style prescribes, not configuration state.

- **Step 9.5**: context.md § Status Code claimed "cmd/bot is still the scaffold", which Group A falsified; Group B left it deliberately because this step owns that bullet. Now reworded, and AC17 closes with it.
- **Step 9.5**: also corrected claude-tools-hierarchy.md:97, a line this PR authored — it named **/secrets* but not **/.secrets*, which settings.json also denies. In scope because the PR rewrote that sentence; not a widening.

- **Step 11**: SR1-1 was found by mutation testing and is closed the same way — deleting the bindDuration tag guard now turns TestLoadBalance_NullValue_ZeroAdmittingKeys/duration_non_negative RED, and deleting the bindDecimal one turns .../decimal_unit_fraction RED. Both re-run after the last edit; balance_load.go restored byte-identical from a cp backup, never git checkout.
- **Step 11**: the doubled prefix is fixed on the sentinel side, not the KeyError side — the sentinels lose their "config: " and KeyError keeps rendering it once, so a message reads "config: <key>: missing: required". No test asserted on sentinel text (grep confirmed before the change).

- **Step 10**: APPROVE at round 2. The three below-floor reservations it named were fixed anyway before the PR — a citation that resolves but does not support its claim is the failure shape AGENTS.md § Communication names, and shipping one knowingly is worse than the round it would have cost. Mutation coverage re-verified after the edits, since they touched the very test that kills the mutant.

## Key discoveries (don't re-investigate)

- Permission deny rules reach `Bash` by command text, not only the file tools: `ls -la .env .env.example` is refused, `ls -la .env.example` and `git check-ignore -q .env` both run. Verification commands touching `.env.example` use one path per command.
- `.env.example` is authorable and readable since `929b8e7` narrowed the deny list; confirmed against the live matcher.
- `reflect.DeepEqual` on `decimal.Decimal` is TRUE for two parses of the same text and false only for a respelling (`1.50` vs `1.5`) — it passes today and breaks later. Use `decimal.Decimal.Equal` field-wise.
- A bare `!!int 30` is rejected for `time.Duration` rather than silently becoming 30ns — reproduced twice, independently.
- D6's parser-behaviour table is normative for subtask 1: re-probe, never re-derive from memory.

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS |
| AC2 | PASS |
| AC3 | PASS |
| AC4 | PASS |
| AC5 | PASS |
| AC6 | PASS |
| AC7 | PASS |
| AC8 | PASS |
| AC9 | PASS |
| AC10 | PASS |
| AC11 | PASS |
| AC12 | PASS |
| AC13 | PASS |
| AC14 | PASS |
| AC15 | PASS |
| AC16 | PASS |
| AC17 | PASS |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| D1-1 | design round 1 | major | fixed@929b8e7 | `git check-ignore -q .env.example` (exit 1) |
| D1-2 | design round 1 | minor | fixed@869cfb3 | `golangci-lint run` on `internal/config` after subtask 1 |
| D1-3 | design round 1 | note | fixed@869cfb3 | — |
| D1-4 | design round 1 | note | fixed@869cfb3 — owner: loader variables only | — |
| D2-1 | design round 2 | minor | fixed@869cfb3 | `grep -n price_distance_exponent <design>` |
| D2-2 | design round 2 | minor | fixed@869cfb3 | — |
| D2-3 | design round 2 | minor | fixed@869cfb3 | — |
| D2-4 | design round 2 | minor | fixed@869cfb3 | — |
| D2-5 | design round 2 | minor | fixed@869cfb3 | `sed -n '49p' ai-docs/key-decisions.md` |
| SR1-1 | self-review round 1 | major | ✅ fixed@3306197 | delete `bindDuration`'s `if n.Tag != "!!str"` block (`internal/config/balance_load.go:60-62`), then `go test -count=1 ./internal/config/` — must go RED; today it stays green |
| SR1-2 | self-review round 1 | major | ✅ fixed@3306197 | `grep -n 'chunk size in cells' ai-docs/code-style.md` — must return nothing once the row is amended |
| SR1-3 | self-review round 1 | minor | ✅ fixed@9899d9c |
| SR1-4 | self-review round 1 | minor | ✅ fixed@3306197 | `env -u LAB_GAME_BOT_TOKEN … go run ./cmd/bot 2>&1 \| grep -c 'config: .*: config: '` — must be 0 |
| SR1-5 | self-review round 1 | minor | ✅ fixed@3306197 | `sed -n '63,74p' internal/config/config.go \| grep -c 'ErrMissing'` — must be >= 1 |
| SR1-6 | self-review round 1 | nit | ✅ fixed@3306197 | `sed -n '59,61p' internal/config/config_test.go` — the `&&` conjunction replaced by a direct assertion |
| SR1-a1 | self-review round 1 | — | accepted@1 — the leaf-alias, leaf-shape and interior-alias guards are each unprotected by the suite (deleting any one leaves it green), but none of the three is a silent accept: the value still lands in `bind`/`default` and is still rejected as `ErrInvalidValue` at the same key. Redundant defence, not a coverage hole. | for each guard: delete it, run `go test -count=1 ./internal/config/` (green) and confirm the value is still rejected |
| SR1-a2 | self-review round 1 | — | accepted@1 — `WorldBalance`/`ChunkBalance` (`internal/config/balance.go:23-31`) do not violate AC15: they type the chunk-grid *balance* axis AC7 enumerates, not the world set's interior (lexicon/bestiary), which `Config.WorldPath` exposes as a bare string. | `grep -n 'WorldPath' internal/config/config.go` → the path string only |
| SR1-a3 | self-review round 1 | — | accepted@1 — the `AGENTS.md` hand-rolling AXIOM (a73040b) and `ai-docs/dependency-versions.md` ride in this PR outside the spec's Scope list; authorised by the owner's recorded decision (this file, § Decisions log, "**Step 8**: owner's decision"). Not scope creep. | `git log --oneline -1 a73040b` |
| SR1-a4 | self-review round 1 | — | accepted@1 — `cmd/bot/main.go`'s `run` doc comment avoids the literal `log.Fatal`/`panic(` substrings; the reworded text was never committed with them (first commit 250739e already carries the current wording), the comment is accurate, and the discharge is recorded in `ai-docs/learnings.md` + `ai-docs/harness-gaps.md`. Not a live instance of the dodge. | `git show 250739e:cmd/bot/main.go` |
| SR2-1 | self-review round 2 | minor | fixed@2a3ce41 — raised as below-floor; below severity floor | `grep -n 'DESIGN.md.:69' ai-docs/code-style.md` — must return nothing (cite `§2.2.2`), and the row must quote «размер — конфиг», not *ориентир* |
| SR2-2 | self-review round 2 | nit | fixed@2a3ce41 — raised as below-floor; below severity floor | `sed -n '/ZeroAdmittingKeys/,/^}/p' internal/config/balance_load_test.go` — the field holding the replacement fixture text is not called `want` |

## Self-Review (Round 1)

**Verdict:** REJECT

**Diff window reviewed:** `3ff8c98..HEAD` (the whole branch; `3ff8c98` is `git merge-base main HEAD`), 37 files, +3283/-13.

**What was checked.** Spec ACs 1-17 against the shipped tree; design D1-D13, § Decomposition (subtasks 1-9), § Handoff plan, § Risks and § Test Design (subtasks 1-7 + gate-level checks); every `.go` file in `cmd/bot` and `internal/config`; `config/balance.yaml`, `config/world/.gitkeep`, `.env.example`, `.github/workflows/ci.yml`, `.claude/settings.json`, `go.mod`/`go.sum`; the propagation edits to `AGENTS.md`, `ai-docs/context.md`, `ai-docs/context-status.md`, `ai-docs/domain-invariants.md`, `ai-docs/key-decisions.md`, `ai-docs/claude-tools-hierarchy.md`, `ai-docs/dependency-versions.md`, `ai-docs/plans/INDEX.md`, plus the `learnings.md` / `harness-gaps.md` entries.

**AC-verification commands re-run against the shipped tree (design § Test Design → Gate-level checks):**

| Check | Result |
|---|---|
| `make verify` | **PASS** — `VERIFY-GREEN`; `golangci-lint run` → `0 issues.`; file-limits, tidy-delta, `actionlint .github/workflows/*.yml`, shellcheck sweep all clean |
| `go test -race -count=1 ./...` (uncached) | **PASS** — `ok` for `cmd/bot`, `internal/config`, `internal/store`, `internal/testdb` |
| AC11: `rg -n -e '\bpanic\(' -e '\blog\.Fatal' $(go list -f '{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}' ./cmd/... ./internal/...)` | **PASS** — no output, exit 1 (the corrected `go list` file set, per `2ddc228`) |
| Panic-index wider run (adds `\blog\.(Fatal\|Panic)`, `\bmust[A-Z]`) | **PASS** — no output, exit 1; no index row owed |
| `git check-ignore -q .env.example; echo $?` | **PASS** — `1` (committable); tracked, confirmed by `git ls-files .env.example` |
| `git check-ignore -q .env; echo $?` (own command, D13(c)) | **PASS** — `0` (still ignored) |
| `rg -n --glob '*.go' 'internal/testdb' internal/config` + `go list -f '{{.Imports}} {{.TestImports}} {{.XTestImports}}' ./internal/config` | **PASS** — one hit, `env.go:12`, inside a comment; neither `internal/testdb` nor `testcontainers` in any of the three import lists (AC12: no database, no network) |
| D7 honour-system: `doc.go` is the only file whose first line is a package comment | **PASS** — swept `head -1` over all 14 files in the package |
| AC8 secret sweep over the added lines of the diff | **PASS** — every `api_id`/`api_hash`/token match is prose in the spec/design; `.env.example` carries placeholders only |
| `go mod why -m gopkg.in/yaml.v3` | **PASS** — still `internal/store` → pgx → pgx.test → testify → `assert/yaml`, i.e. test-only indirect; `go.mod`/`go.sum` moved by exactly the two predicted direct lines |

**Mutation tests run (checklist §3 / Pattern 2 — "would this test still pass if I broke the thing it names?"):** the missing-key report, the `!!int` and decimal tag guards, the unknown-key report, the non-mapping-root guard, the duplicate-key pre-pass and the `assertBalanceEqual` comparison were each deleted or perturbed in turn; every one turned the suite RED. Three guards (leaf alias, leaf shape, interior alias) leave it green but are not silent accepts — recorded as `SR1-a1`. One guard leaves it green **and** silently defaults a balance number — finding 1.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | internal/config/balance_load.go:60-62 | major | **`bindDuration`'s `!!str` tag guard is untested, and deleting it silently defaults a real balance key.** Design D6 row 1 is normative: "The walk **rejects a `!!null` node at any schema path** before binding. Without this, `cap:` with nothing after it is a silently defaulted balance number." Removing only the `if n.Tag != "!!str" { … }` block leaves the committed suite **fully green** (`ok github.com/maratik123/lab-game/internal/config 0.005s`) while a probe on `raid.death.respawn_debuff: null` reports `ACCEPTED SILENTLY: respawn_debuff = 0s` (unmutated tree, same probe: `REJECTED: config: raid.death.respawn_debuff: config: invalid value: must be a duration string, got !!null`). Neither test that looks like it covers the guard reaches it: `balance_load_test.go:182` `TestLoadBalance_NullValue` mutates `raid.stamina.cap`, whose `> 0` predicate rejects the decoded zero on its own, and `balance_load_test.go:206` `TestLoadBalance_DurationGivenBareInt` is rejected by `time.Duration`'s own decoder (D6's own measured row), so the guard is never the rejecting clause in either. `raid.death.respawn_debuff` is the one duration key whose predicate admits zero (`nonNegativeDuration`, `balance_load.go:126`), which is exactly why it is the case the table is missing. **Fix:** add a null case at `raid.death.respawn_debuff` to the balance table (and, for the same reason on the decimal side, one at `raid.afk.cruelty`, whose `0 <= x <= 1` predicate also admits zero). | ⬜ Open |
| 2 | ai-docs/code-style.md:50 | major | **AC17 propagation miss — a live instruction file still routes a shipped balance axis into a Go constant.** The row reads "\| A structural constant (**chunk size in cells**, the number of edges of a hex, a protocol limit) \| A named Go constant next to the code that owns it \|". This diff ships chunk size **as configuration**: `config/balance.yaml:10-14` (`world.chunk.cols` / `world.chunk.rows`), `internal/config/balance_load.go:114-115`, and AC7 names "chunk size" explicitly; `docs/DESIGN.md:69` says «**Чанки** (размер — конфиг, ориентир 16×16 гексов)». The schema rejects unknown keys, so chunk size is configuration by construction now — an implementor following `code-style.md:50` would compile in the very kind of value this PR exists to move out of Go. AC17's membership criterion is `AGENTS.md` § Propagation Rule step 4 ("every LIVE doc must agree"), and the design's § Propagation targets states its own list is "illustrative of the class, not a bound on it", so this is an implementation gap, **not** a Design/Spec Amendment trigger — the fix is a prose edit to `ai-docs/code-style.md`, an instruction doc, with no `.spec.md`/`.design.md` change. | ⬜ Open |
| 3 | internal/config/repo_root_test.go:245 | minor | **False citation offered as authority.** The comment reads "(AGENTS.md §16 style: no reliance on ambient state that a test runner may not control)". `AGENTS.md` has no numbered sections at all (`grep -n '^## ' AGENTS.md` → 15 named ones), and its only `§16` occurrences cite `docs/DESIGN.md` §16, the open-question list, which says nothing about working directories. `AGENTS.md` § Communication: "A citation offered as authority is itself a claim — open it." Drop the citation or name the real rule. | ⬜ Open |
| 4 | internal/config/errors.go:45 | minor | **Every rendered error doubles its package prefix.** `KeyError.Error()` formats `"config: %s: %s"` while each sentinel's own text already opens `config: ` (`errors.go:14,18,22,26`), so `go run ./cmd/bot` with an empty environment prints `config: LAB_GAME_BOT_TOKEN: config: missing: required` (measured). `ai-docs/code-style.md:21` — "The prefix names the operation, not the error." No AC forbids it and Step 9 already flagged it in the Decisions log; drop `config: ` from the sentinel texts **or** from `Error()`, not both. | ⬜ Open |
| 5 | internal/config/config.go:63-74 | minor | **DOC-3 — sentinel errors not named.** `Load` is the only exported function that can return `ErrMissing`, `ErrUnknownKey`, `ErrInvalidValue` and `ErrUnreadable`, and its doc comment names none of them. `ai-docs/doc-convention.md` DOC-3: "A function returning a sentinel error names it." (`errors.go`'s var-block comment carries the information, which is why this is `minor`, not `major`.) | ⬜ Open |
| 6 | internal/config/config_test.go:59 | nit | **Near-vacuous secondary assertion.** `if strings.Count(err.Error(), "\n") > 0 && strings.Contains(err.Error(), "no such file")` fires only when *both* halves hold, so the "no file was read" guarantee the comment claims is not what is asserted. Assert the joined error has exactly one leaf, or that it contains no `*KeyError` other than `LAB_GAME_BALANCE_PATH`. | ⬜ Open |

**Not raised, and why** (durable rows in the register above): `SR1-a1` three redundant-but-unprotected walker guards; `SR1-a2` `WorldBalance`/`ChunkBalance` vs AC15; `SR1-a3` the `AGENTS.md` hand-rolling AXIOM riding in this PR; `SR1-a4` `cmd/bot/main.go`'s gate-shaped doc comment.

**AC status after this round:** AC1-AC16 hold as recorded. **AC17 does not** — finding 2. Findings 1 and 3-6 are quality defects against the design and the convention files rather than AC failures.

## Self-Review (Round 2)

**Verdict:** APPROVE

**Diff window reviewed:** `3ff8c98..HEAD` as given; the round-1 register scoped the work to the diff since `3306197` (`fix(config): cover the null guard the suite could not kill`) plus `0f62955` (progress file only). Six files moved: `ai-docs/code-style.md`, `internal/config/{balance_load_test.go,config.go,config_test.go,errors.go,repo_root_test.go}`. `go.mod`/`go.sum` untouched (`git diff --stat 0ebf112..HEAD -- go.mod go.sum` → empty).

**What was checked.** Every round-1 register row re-verified by its own recorded command; the fix diff read in full for new defects; every gate re-run because production code moved (`errors.go` sentinel texts, `config.go` doc comment).

**Round-1 rows, re-verified by their register commands:**

| id | Register command | Result |
|---|---|---|
| SR1-1 | delete `bindDuration`'s `if n.Tag != "!!str"` block, then `go test -count=1 ./internal/config/` — must go RED | **✅ FIXED.** Now RED: `--- FAIL: TestLoadBalance_NullValue_ZeroAdmittingKeys/duration_non_negative`. The symmetric decimal mutation is also killed: `--- FAIL: …/decimal_unit_fraction` **and** `--- FAIL: TestLoadBalance_DecimalGivenQuotedNumber`. The new table covers both zero-admitting keys (`raid.death.respawn_debuff`, `raid.afk.cruelty`) and guards its own fixture with a `t.Fatalf` when the replacement line is not found — the round-1 hole is closed, not papered over. |
| SR1-2 | `grep -n 'chunk size in cells' ai-docs/code-style.md` — must return nothing | **✅ FIXED** (exit 1). Chunk size moved to the configuration row; AC17's contradiction is gone. The replacement row's *citation* is a new, separate defect — `SR2-1`, below. |
| SR1-3 | `grep -n 'AGENTS.md §16' internal/config/repo_root_test.go` — must return nothing | **🔁 RE-OPENED.** The literal grep passes (exit 1), but the fix is incorrect in the same way the finding named. See below. |
| SR1-4 | `go run ./cmd/bot` with all six variables unset, count of `config: .*: config: ` | **✅ FIXED** — count `0`. Measured: `lab-game bot: configuration: config: LAB_GAME_BOT_TOKEN: missing: required` (one prefix). Sentinel texts lost their `config: ` prefix; swept the tree for the old strings (`config: missing` / `unknown key` / `invalid value` / `unreadable path`) — no live reference outside this file's own round-1 record. |
| SR1-5 | `Load`'s doc comment names `ErrMissing` | **✅ FIXED** — all four sentinels named, with what each classifies and `errors.Is` (DOC-3 satisfied). |
| SR1-6 | `config_test.go:59` — the `&&` conjunction replaced | **✅ FIXED** — now `if strings.Contains(err.Error(), "no such file")`, a single unconditional assertion. |

**Gates re-run against the shipped tree (production code moved, so none of round 1's results were carried over):**

| Check | Result |
|---|---|
| `make verify` | **PASS** — `VERIFY-GREEN`, `golangci-lint run` → `0 issues.` |
| `go test -race -count=1 ./internal/config/ ./cmd/bot/` (uncached) | **PASS** — `ok` both |
| AC11 `rg` over the `go list` `GoFiles` set | **PASS** — no output, exit 1 |
| Panic-index wider run (`panic(`, `log.(Fatal\|Panic)`, `must[A-Z]`) | **PASS** — no output, exit 1; still no index row owed |
| `git check-ignore -q .env.example` / `.env` (separate commands, D13(c)) | **PASS** — `1` / `0` |
| `git diff --stat 0ebf112..HEAD -- go.mod go.sum` | **PASS** — empty; the dependency graph did not move |

**No `blocker` or `major` is open**, so the verdict is APPROVE. Three reservations ride along in the register and must not be read as blocking:

- **`SR1-3` — re-opened, `minor`, `internal/config/repo_root_test.go:13-14`.** The comment now reads "(AGENTS.md § Code Style — Determinism: no reliance on ambient state that a test runner may not control)". That section resolves, but the bullet it names reads in full: *"world generation, combat, and any PvP-trail replay are pure functions of `(seed, input)`. No `time.Now()`, no map-iteration order, and no un-seeded `math/rand` on those paths"* — it says nothing about a test's working directory, and `repoRootPath` is a helper on none of those three paths. A non-resolving citation was replaced by a resolving-but-unsupporting one, which is `AGENTS.md` § Communication's named shape: *a thematically adjacent entry makes the misattribution feel checked*. The reason in the comment stands on its own; the cheapest correct fix is to drop the citation.
- **`SR2-1` — new, `minor`, `ai-docs/code-style.md:51`.** Two defects in the citation that now carries the corrected row. (a) It cites `docs/DESIGN.md`**:69**, a line number, while the same row cites `§16.5` by section — `ai-docs/doc-convention.md` DOC-4: *"Cite the section number, never a line number… `§2.2.4` survives what `:118` does not."* The section is **§2.2.2**. (b) It says chunk size is what DESIGN "calls an *ориентир*". Verbatim, `docs/DESIGN.md:69` is «**Чанки** (размер — конфиг, ориентир 16×16 гексов)»: *конфиг* attaches to the **size**, *ориентир* to the **number 16×16**. The row drops the one word that carries its own rule and quotes the one that does not — a wrong premise attached to a correct rule.
- **`SR2-2` — new, `nit`, `internal/config/balance_load_test.go:198-201`.** The case struct's `want` field holds the *replacement fixture text* (an input) while `wantKey` holds the expected error key; `want` reads as an expectation. Rename to `new`/`replacement`.

**Count and file list for the below-floor items:** 3 items (2 `minor`, 1 `nit`) across 2 files — `ai-docs/code-style.md`, `internal/config/repo_root_test.go`, `internal/config/balance_load_test.go`. None changes behaviour; none is the difference between verdicts.

**AC status after this round:** AC1-AC17 all hold. AC17's round-1 failure is closed by the `ai-docs/code-style.md` amendment.

## Files touched

- `internal/config/doc.go` — provisional package comment (D7; subtask 5 rewrites the body)
- `internal/config/balance.go` — `Balance` and its nested sub-structs, mirroring the YAML tree
- `internal/config/errors.go` — `ErrMissing`/`ErrUnknownKey`/`ErrInvalidValue`/`ErrUnreadable` sentinels, `*KeyError`
- `internal/config/balance_load.go` — the schema (`balanceSchema`), the schema tree, the duplicate-key pre-pass, and the node walk (`loadBalance`)
- `internal/config/balance_load_test.go` — subtask 1's table tests plus `assertBalanceEqual`/`compareBalanceValue` (the field-wise `decimal.Decimal` comparison helper subtask 6 reuses)
- `go.mod`, `go.sum` — `go.yaml.in/yaml/v3 v3.0.5` added as a direct requirement
- `config/balance.yaml` — the tracked balance set, one placeholder per schema key, curve comments for the door-price and monster-budget triples
- `internal/config/balance_file_test.go` — the both-directions agreement test for `config/balance.yaml`
- `internal/config/repo_root_test.go` — `repoRootPath`, resolving a repo-root-relative path from the test file's own location
- `internal/config/env.go` — `Lookup`, `EnvKeys`, `envKeys` (the loadEnv-validated subset), the six `LAB_GAME_` variable-name constants, `loadEnv`, the base-URL and chat-id parsers
- `internal/config/env_test.go` — subtask 3's table tests plus `mapLookup`/`validEnv`, reused by subtask 5
- `internal/config/world.go` — `resolveWorldPath`: `LAB_GAME_WORLD_PATH`'s presence check and the open/close readability probe
- `internal/config/world_test.go` — subtask 4's table tests
- `config/world/.gitkeep` — the tracked, empty-by-design placeholder target for `LAB_GAME_WORLD_PATH`'s default value
- `internal/config/doc.go` — package comment rewritten to AC13's wording (reload policy + each source's exclusive domain), replacing subtask 1's provisional text
- `internal/config/config.go` — `Secret`, `Config`, `Load`, `requiredBalancePath`
- `internal/config/config_test.go` — subtask 5's scenarios plus `validConfigEnv`
- `.env.example` — a placeholder for every `EnvKeys()` variable, opened by the header comment D13(b) specifies
- `internal/config/disjoint_test.go` — the two-direction key-set agreement test, the non-empty-values test, the example-environment load, and the balance-independent-of-other-variables test
- `go.mod`, `go.sum` — `github.com/joho/godotenv v1.5.1` added (test-only import, `internal/config/disjoint_test.go`)
- `cmd/bot/main.go` — `main` now delegates to `run(lookup, stderr, stdout) int`, loading and validating configuration before any other work (AC11)
- `cmd/bot/main_test.go` — subtask 7's scenarios, plus a local `repoRootPath`/`mapLookup`/`validEnv` (cmd/bot cannot import internal/config's unexported test helpers)
- `.github/workflows/ci.yml` — `config/**` and `.env.example` added to the `go` paths-filter
- `AGENTS.md` — § Build & Test: the `go run ./cmd/bot` line now states that it exits non-zero without the documented environment
- `ai-docs/context.md` — § Architecture "Layout so far": `internal/config` added (the § Status bullets are Step 9.5's)
- `ai-docs/domain-invariants.md` — § 8: the reload policy named (start-up only, no hot reload), pointing at KD-24
- `ai-docs/key-decisions.md` — new § *Configuration layer (2026-09-04)*: KD-22 (YAML parser + rejected alternatives + escape hatches), KD-23 (disjoint sources, no override chain), KD-24 (start-up-only reload, operator-supplied path, no `go:embed`)
- `ai-docs/plans/INDEX.md` — the row for this plan pair (🟢 in progress, #18)
