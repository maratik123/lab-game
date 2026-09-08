# Progress: Comment reference ban — doc comments stay, outward references go — ACTIVE
_Updated: 2026-09-08 09:41_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-08-comment-reference-ban
**base_commit:** f1bc60fdc29f78133452a41ff7a8b1d0f070e577
**Last build:** not run
**Issue:** #68
**Spec:** ai-docs/plans/2026-09-08-comment-reference-ban.spec.md
**current_step:** Step 8 — Group A subtask 4 of 10 complete
**last_passed_gate:** golangci-lint run (whole module)
**entry_args:** 68

## Next action

**Do this immediately:** start Group A subtask 5 — sweep `internal/store` + migrations, `internal/testdb`.

## Subtasks

Groups per the design's `## Handoff plan`: **A** = 1–10 (code, `sonnet`/`code-writer`) · **B** = 11–14 (instructions/harness, `inherit`/`general-purpose`) · **C** = 15 (code, `sonnet`/`code-writer`, terminal).

- [x] 1. Comment extraction: file-class router + one extractor per grammar; add `mvdan.cc/sh/v3`
- [x] 2. The banned-class classifier (D4) and the exemption pass (D5)
- [x] 3. The command `cmd/commentrefs`: input modes, exit codes, report format
- [x] 4. Sweep `cmd/bot`, `internal/config`, `internal/backoff`
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
- **Subtask 2**: `Classify` strips a trailing `_test` from `ownPackage` itself, rather than requiring the caller to do it, so the contract is exercised directly by this subtask's own tests rather than deferred to subtask 3's wiring.
- **Subtask 2**: classification runs sequentially over one working copy of the comment text, blanking each matched span (with same-length spaces) before the next, narrower-precedence class runs — this stops a URL's own `://` and digits from also being reported as a `locator`, without disturbing the byte positions later classes scan.
- **Subtask 3**: running `cmd/commentrefs` over the whole tracked tree (`go run ./cmd/commentrefs`, no arguments) at this point in the branch produced 1303 report lines and exit 1, with zero instrument-failure lines — every extractor from subtask 1 parsed every real tracked file of its class without error, which is stronger evidence than the fixture tests alone that the extractors hold up on this corpus. That output is expected and not itself a defect: subtasks 4–9 and 12 are the sweep.
- **Subtask 3**: a `go list ./internal/...` failure (no reachable `go.mod`, or no Go toolchain) is treated as non-fatal — `run` warns on stderr and continues with an empty module-package set, narrowing only `module-symbol` classification rather than returning exit 2, because every other banned class is still fully decidable without it. This also lets the command's own tests exercise a scratch git repository with no `go.mod` at all.
- **Subtask 3**: `AC6` — the gate's own source was run against itself (`go run ./cmd/commentrefs internal/commentref/*.go cmd/commentrefs/*.go`) and initially reported nine real findings, all in doc comments that named a gated-set grammar by its literal repository-root file name (`Makefile`, `.gitignore`, `.env.example`) or by a bare source extension token (`.sh`), plus two stray `KD-5`/`D4` design citations left over from drafting. Every doc comment was reworded to describe the grammar or behaviour without the literal banned token; the gate is clean over its own package now, verified by re-running it, not merely reasoned about.
- **Subtask 4**: the sweep of `cmd/bot`, `internal/config`, `internal/backoff` removed roughly 120 report lines across 16 files — overwhelmingly `decision-anchor` (`D6`, `D9`, `D10`, `D13`, `D15`, `D16`, `D20`, …) and `ac-id` citations in doc comments, plus a smaller set of `repo-path` (sibling-file and package-path mentions like `internal/tg`, `transport.go`, `config/balance.yaml`) and `module-symbol` findings (`backoff.DefaultFactor`, `backoff.Exponential`, `backoff.ValidFactor` — cross-package under KD-9's "this module only" narrowing, so banned even though they name the real default/contract). Every sentence's substantive claim survived; only the outward pointer was cut, per D12 ("where a sentence exists only to carry the reference, the sentence goes with it" — no sentence here existed only for that). Verified per-file with `go run ./cmd/commentrefs <file>` after each edit, not only at the end.
- **Subtask 4**: `config/balance.go`'s citation-stripping regex left two literal `//.` orphan comment lines (`gocritic`'s `commentFormatting` caught both); fixed by re-flowing the sentence rather than leaving the stray period, confirmed by re-running `golangci-lint run`.

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
- `internal/commentref/` — new package: `commentref.go` (the `Comment` type), `go_extract.go`, `shell_extract.go`, `yaml_extract.go`, `sql_extract.go`, `makefile_extract.go`, `gitignore_extract.go`, `envexample_extract.go`, `router.go`, `classify.go` (the `Class`/`Finding` types and `Classify`), plus a `_test.go` beside each and `testhelpers_test.go`
- `cmd/commentrefs/` — new command: `main.go`, `run.go` (the testable `run`, the three input modes, exit codes), `git.go` (git subprocess helpers + module package name discovery), `run_test.go`, `git_test.go`
- `cmd/bot/main.go`, `cmd/bot/main_test.go` — comment sweep
- `internal/config/*.go` (all 19 files) — comment sweep
- `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go` — comment sweep
