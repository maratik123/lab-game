# Progress: Comment reference ban — doc comments stay, outward references go — ACTIVE
_Updated: 2026-09-08 09:41_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-08-comment-reference-ban
**base_commit:** f1bc60fdc29f78133452a41ff7a8b1d0f070e577
**Last build:** not run
**Issue:** #68
**Spec:** ai-docs/plans/2026-09-08-comment-reference-ban.spec.md
**current_step:** Step 8 — Group A subtask 1 of 10 complete
**last_passed_gate:** golangci-lint run (internal/commentref)
**entry_args:** 68

## Next action

**Do this immediately:** start Group A subtask 2 — the banned-class classifier (D4) and the exemption pass (D5), over extracted comments.

## Subtasks

Groups per the design's `## Handoff plan`: **A** = 1–10 (code, `sonnet`/`code-writer`) · **B** = 11–14 (instructions/harness, `inherit`/`general-purpose`) · **C** = 15 (code, `sonnet`/`code-writer`, terminal).

- [x] 1. Comment extraction: file-class router + one extractor per grammar; add `mvdan.cc/sh/v3`
- [ ] 2. The banned-class classifier (D4) and the exemption pass (D5)
- [ ] 3. The command `cmd/commentrefs`: input modes, exit codes, report format
- [ ] 4. Sweep `cmd/bot`, `internal/config`, `internal/backoff`
- [ ] 5. Sweep `internal/store` + migrations, `internal/testdb`
- [ ] 6. Sweep `internal/tg`, `internal/tgtest`
- [ ] 7. Sweep `internal/ingest`, `internal/scheduler`
- [ ] 8. Sweep the build/runtime gated files; `coverage-ratchet.sh` gains the D9 `--help`
- [ ] 9. `.env.example` restated; `config/balance.yaml` gains English self-contained prose
- [ ] 10. Wiring landing one: `comment-refs` target; `pre-commit` symlink + dispatcher
- [ ] 11. Rewrite `ai-docs/doc-convention.md` to the new rule
- [ ] 12. Sweep the harness shell scripts; usage prose behind `--help`, block copied verbatim
- [ ] 13. The script-shape checker and its suite; extend the dispatch suite
- [ ] 14. Propagate the rule text across the instruction surface and the hook messages
- [ ] 15. Wiring landing two: `verify` prerequisite; the CI job; Harness-guards runs `make shellcheck`

## Decisions log

- **Step 7**: design-review reached GO on round 3 of a cap of 3; its five GO notes were folded into the design at f1bc60f before Step 8 opened.
- **Step 7**: three reviewer findings were spec-amending rather than design-internal; the owner settled all three and the spec was amended at e0e99a1 (KD-9 narrowed to this module, AC15's carve-out made explicit, Scope item 11 + KD-18 + AC24 added).
- **Subtask 1**: `go get mvdan.cc/sh/v3@v3.14.1 && go mod tidy` reproduced exactly the design's D3 prediction in this tree — `go.mod` gains only the direct requirement, the `go` directive normalises `1.26` → `1.26.0`, and `go.sum` grows with the dependency's own test-dep checksums.
- **Subtask 1**: `go.yaml.in/yaml/v3`'s reported `Text` field keeps the leading `#` marker (measured with a scratch probe over synthetic YAML), unlike `mvdan.cc/sh/v3/syntax`'s `Comment.Text`, which already has its `#` stripped — the two extractors' marker-stripping therefore differ in what they start from, and both are exercised by their own extraction tests.
- **Subtask 1**: the YAML extractor's exempt-empty-file path (`ExtractYAML` returns `nil, nil` for an all-whitespace source) was added because `yaml.Unmarshal` on an empty document does not populate a document node to walk; no fixture file in the gated set is empty today, so this is defensive rather than load-bearing yet.

## Key discoveries (don't re-investigate)

- `go.yaml.in/yaml/v3` never reports a comment's own line — it reports the attached node's line, and it attaches across a blank line contrary to its own field doc. D1a's nearest-match reconciliation, with `UNRECONCILED ⇒ exit 2`, is the answer; an offset rule is measurably wrong on `config/balance.yaml`.
- No tracked `*.sh` is free of a banned-class comment (`comm -23` complement empty, both operands non-empty). That is why the wiring lands in two parts: anything judging the whole tree before subtask 12 would be red.
- `pg_query_go/v6` can answer the SQL comment-span question but costs cgo; it is the named escape hatch, not a rejected-on-capability alternative.
- Adding `mvdan.cc/sh/v3` grows `go.sum` by its own test-dep checksums and normalises the `go` directive `1.26` → `1.26.0`, because that dependency's own `go.mod` declares `1.26.0`.
- No script in this corpus resolves its own location — every one that resolves anything resolves the repo root via `git rev-parse --show-toplevel`. That is why `--help` is copied per script rather than sourced from a library.

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
| AC18 | NOT_TESTED |
| AC19 | NOT_TESTED |
| AC20 | NOT_TESTED |
| AC21 | NOT_TESTED |
| AC22 | NOT_TESTED |
| AC23 | NOT_TESTED |
| AC24 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `go.mod`, `go.sum` — `mvdan.cc/sh/v3` added
- `internal/commentref/` — new package: `commentref.go` (the `Comment` type), `go_extract.go`, `shell_extract.go`, `yaml_extract.go`, `sql_extract.go`, `makefile_extract.go`, `gitignore_extract.go`, `envexample_extract.go`, `router.go`, plus a `_test.go` beside each and `testhelpers_test.go`
