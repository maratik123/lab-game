# Progress: configuration layer — runtime settings and the balance/world constant files — ACTIVE
_Updated: 2026-09-04 

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-04-config-layer-balance-files
**base_commit:** 869cfb37a773cc24aa2a0f3f705ced93f847422a
**Last build:** not run

**Issue:** #18
**Spec:** ai-docs/plans/2026-09-04-config-layer-balance-files.spec.md

**current_step:** Step 8 — Implementation start
**last_passed_gate:** go build ./... | 2026-09-04T04:26:20Z | 869cfb37a773cc24aa2a0f3f705ced93f847422a
**entry_args:** 18

## Next action

**Do this immediately:** hand off Group A (subtasks 1–8) to `code-writer` via `/context-reset`, starting with subtask 1 — `internal/config/doc.go` (provisional package comment), the `Balance` types, the schema entries, the duplicate-key pre-pass and the node walk, adding `go.yaml.in/yaml/v3`.

## Subtasks

- [ ] 1. Balance schema + walker; provisional package comment in `doc.go`; adds the YAML parser  ← CURRENT
- [ ] 2. Tracked balance set at the default path, placeholder per schema key + curve comments
- [ ] 3. Environment layer: `Lookup`, `EnvKeys`, name constants, token/DSN/base-URL/chat-id validation
- [ ] 4. World-set path resolution: required variable, readability probe, both errors handled
- [ ] 5. `Load`: `Config`, `Secret`, joined error; rewrites `doc.go`'s comment body to AC13 wording
- [ ] 6. `.env.example` + disjointness tests; adds `godotenv` as a test-only requirement
- [ ] 7. `cmd/bot` wired to load and validate before any other work; `main` delegates to `run`
- [ ] 8. CI paths-filter: tracked config artefacts added to the `go` filter; actionlint
- [ ] 9. Propagation sweep over the prose sites the diff falsifies

## Decisions log

- **Step 8**: entering implementation at design `869cfb3` (round-2 GO plus the mandated notes fold-in), spec `5285f4c`.

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

- (none yet — Step 8 has not produced code)
