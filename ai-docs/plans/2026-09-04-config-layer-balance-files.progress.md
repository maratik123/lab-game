# Progress: configuration layer — runtime settings and the balance/world constant files — ACTIVE
_Updated: 2026-09-04 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-config-layer-balance-files
**base_commit:** 869cfb37a773cc24aa2a0f3f705ced93f847422a
**Last build:** not run

**Issue:** #18
**Spec:** ai-docs/plans/2026-09-04-config-layer-balance-files.spec.md

**current_step:** Step 8 — Group A complete (subtasks 1-8 of 8); handoff to Group B for subtask 9
**last_passed_gate:** golangci-lint run ./... | 2026-09-04T00:00:00Z | (pre-commit)
**entry_args:** 18

## Next action

**Do this immediately:** Group A (`code-writer`, subtasks 1-8) is complete. Hand off to Group B (subtask 9 — propagation sweep, instructions/harness change-type) per the design's Handoff plan and `.claude/skills/context-reset/SKILL.md`.

## Subtasks

- [x] 1. Balance schema + walker; provisional package comment in `doc.go`; adds the YAML parser
- [x] 2. Tracked balance set at the default path, placeholder per schema key + curve comments
- [x] 3. Environment layer: `Lookup`, `EnvKeys`, name constants, token/DSN/base-URL/chat-id validation
- [x] 4. World-set path resolution: required variable, readability probe, both errors handled
- [x] 5. `Load`: `Config`, `Secret`, joined error; rewrites `doc.go`'s comment body to AC13 wording
- [x] 6. `.env.example` + disjointness tests; adds `godotenv` as a test-only requirement
- [x] 7. `cmd/bot` wired to load and validate before any other work; `main` delegates to `run`
- [x] 8. CI paths-filter: tracked config artefacts added to the `go` filter; actionlint
- [ ] 9. Propagation sweep over the prose sites the diff falsifies  ← CURRENT (Group B)

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

- **Step 8**: owner's decision — the harness changes on this branch (AGENTS.md hand-rolling axiom, the settings.json deny narrowing, the claude-tools-hierarchy propagation) ship in the SAME PR as the config layer, not split out.

## Key discoveries (don't re-investigate)

- Permission deny rules reach `Bash` by command text, not only the file tools: `ls -la .env .env.example` is refused, `ls -la .env.example` and `git check-ignore -q .env` both run. Verification commands touching `.env.example` use one path per command.
- `.env.example` is authorable and readable since `929b8e7` narrowed the deny list; confirmed against the live matcher.
- `reflect.DeepEqual` on `decimal.Decimal` is TRUE for two parses of the same text and false only for a respelling (`1.50` vs `1.5`) — it passes today and breaks later. Use `decimal.Decimal.Equal` field-wise.
- A bare `!!int 30` is rejected for `time.Duration` rather than silently becoming 30ns — reproduced twice, independently.
- D6's parser-behaviour table is normative for subtask 1: re-probe, never re-derive from memory.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |
| AC11 | NOT_TESTED |
| AC12 | NOT_TESTED |
| AC13 | NOT_TESTED |
| AC14 | NOT_TESTED |
| AC15 | NOT_TESTED |
| AC16 | NOT_TESTED |
| AC17 | NOT_TESTED |

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
