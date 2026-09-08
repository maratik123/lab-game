# Progress: Comment reference ban — doc comments stay, outward references go — ACTIVE
_Updated: 2026-09-08 09:41_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-08-comment-reference-ban
**base_commit:** f1bc60fdc29f78133452a41ff7a8b1d0f070e577
**Last build:** PASS
**Issue:** #68
**Spec:** ai-docs/plans/2026-09-08-comment-reference-ban.spec.md
**current_step:** Step 8 — Group B subtask 13 of 14 complete
**last_passed_gate:** `make comment-refs` silent; `make shellcheck`; all fourteen guard regression suites (the new one included); the four standalone checkers; two dispatcher mutants each caught and reverted
**entry_args:** 68

## Next action

**Do this immediately:** start Group B subtask 14 — propagate the rule text across the instruction surface and the hook messages, per the design's D15 and the `grep -rni` sweep; the AC22 grammar sites; the tool-hierarchy and propagation-group rows for the new gate, job, checker and suite; rewrite the #68 body.

## Subtasks

Groups per the design's `## Handoff plan`: **A** = 1–10 (code, `sonnet`/`code-writer`) · **B** = 11–14 (instructions/harness, `inherit`/`general-purpose`) · **C** = 15 (code, `sonnet`/`code-writer`, terminal).

- [x] 1. Comment extraction: file-class router + one extractor per grammar; add `mvdan.cc/sh/v3`
- [x] 2. The banned-class classifier (D4) and the exemption pass (D5)
- [x] 3. The command `cmd/commentrefs`: input modes, exit codes, report format
- [x] 4. Sweep `cmd/bot`, `internal/config`, `internal/backoff`
- [x] 5. Sweep `internal/store` + migrations, `internal/testdb`
- [x] 6. Sweep `internal/tg`, `internal/tgtest`
- [x] 7. Sweep `internal/ingest`, `internal/scheduler`
- [x] 8. Sweep the build/runtime gated files; `coverage-ratchet.sh` gains the D9 `--help`
- [x] 9. `.env.example` restated; `config/balance.yaml` gains English self-contained prose
- [x] 10. Wiring landing one: `comment-refs` target; `pre-commit` symlink + dispatcher
- [x] 11. Rewrite `ai-docs/doc-convention.md` to the new rule
- [x] 12. Sweep the harness shell scripts; usage prose behind `--help`, block copied verbatim
- [x] 13. The script-shape checker and its suite; extend the dispatch suite
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
- **Subtask 5**: the sweep of `internal/store`, its embedded migrations and `internal/testdb` removed roughly 150 report lines across 24 files, the densest single subtask so far — the `00003_event_log.sql` migration alone carried ~34, mostly `§13.x`/`§11`/`§5` design-section citations woven into view-doc prose ("answers §13.3's headline MVP number") that needed rephrasing into standalone sentences ("answers the headline MVP activation number"), not just deletion.
- **Subtask 5**: two SQL comments used "D1"/"D7" as the domain's own retention-metric shorthand (Day-1/Day-7 retention), which collided lexically with the `decision-anchor` pattern (`D[0-9]+`) though the sentence was never citing a design decision. Resolved by spelling them "Day-1"/"Day-7" in prose — clearer for a reader anyway, and it sidesteps the classifier without touching the classifier itself, consistent with the design's own risk note that the direction is conservative and the fix is to write the sentence without the colliding token.
- **Subtask 5**: `internal/store/migrations/00004_ingest.sql`'s header comment named `scheduler.DeadTask` — a genuine cross-package module-symbol under KD-9's this-module-only narrowing, banned even from a comment that is, in substance, describing the shared contract — rephrased to describe the shape ("the same shape the scheduler's own dead-task row already has") without the qualified symbol.
- **Subtask 6**: `internal/tg` + `internal/tgtest` was the densest single subtask — roughly 380 report lines across 20 files, `limit.go` and `limit_test.go` alone carrying ~55 `decision-anchor: D9` citations from a design section that spelled out the limiter's whole mechanism inline. Handled with the same per-file read-then-python-batch-replace-then-reverify loop as subtasks 4–5, always re-running `go run ./cmd/commentrefs <file>` after each batch before moving to the next file — every batch matched on the first attempt except two (`internal/store/basis_test.go`, `internal/tg/caller.go`) where a copy-paste typo in the search string was caught immediately by the tool's own "string not found" error, never landing a wrong edit.
- **Subtask 6**: many comments narrated a **historical defect** ("self-review round 5's coverage sweep", "finding 6", "R1-8") rather than citing a design decision — these aren't `decision-anchor` matches (no `D<N>`/`KD-<N>` token) but were still rewritten to drop the "round N finding N" framing, since a future round-number is exactly the kind of thing that stops meaning anything once the review that produced it is history. Kept: the substance of what regressed and why the test exists.
- **Subtask 6**: `-race` was run in addition to the standard gate set for this subtask specifically (`go test -race ./internal/tg/... ./internal/tgtest/...`) since the package's own suite uses `testing/synctest` and a shared `net.Pipe`-backed fake server across goroutines — a comment-only sweep should not be able to introduce a race, but the design's own AGENTS.md rule makes `-race` a required gate for any change touching goroutines or shared state, and this package is exactly that shape.
- **Subtask 7**: `internal/ingest` + `internal/scheduler` was the largest subtask by file count (43 files, ~370 initial report lines) but individually thinner per file than subtask 6 — mostly straightforward "(design DN)" parenthetical citations, handled with a first-pass regex strip (`\s*\(design D[0-9]+(?:, ?D[0-9]+)*\)`) across every file before the per-file manual read-and-patch loop, which cut the remaining hand-edited residue by roughly two-thirds.
- **Subtask 7**: the regex-first strip left five stray `//.`/`// .` orphan comment lines (a citation that was the whole remainder of its sentence, same shape as subtask 4's `config/balance.go` finding) — caught by `golangci-lint run`'s `gocritic` `commentFormatting` check, not by the comment-reference gate itself (an orphaned marker with no banned token is not a finding). Fixed by re-flowing each sentence to end on the preceding line rather than leaving the bare marker.
- **Subtask 7**: `go test -race ./internal/ingest/... ./internal/scheduler/...` was run for the same reason as subtask 6 — both packages run a worker loop with goroutines, a shared pending-settlement map, and (in ingest's case) a mutex-guarded cache — required by AGENTS.md for any change touching goroutines or shared state, even a comment-only one.
- **Subtask 8**: `.githooks/coverage-ratchet.sh` is the one script this subtask gives the D9 `--help` shape to (the design's own assignment — every other script gets it in Group B's subtask 12, copying this shape verbatim). It already parsed a mode at `mode=${1:-raise}`, so per D9 the `-h|--help` case was inserted at that existing site, after the file's constant assignments (TOLERANCE_PP, RATCHET_FILE, PROFILE) and before the mode-validation case — no side effect precedes it. The removed `# Usage:` header block became the `usage()` function's heredoc body verbatim; the fixed marker line `-h|--help) usage; exit 0 ;;` is exactly what subtask 12 must copy byte-identical later. Verified: `bash .githooks/coverage-ratchet.sh --help` prints the usage and exits 0 with nothing else run; `shellcheck -s bash` clean; the existing `ai-docs/scripts/test-precommit-dispatch.sh` suite (which drives this script through the real hook) still passes unchanged; a real `--check` run against the working tree still reports the correct percentage.
- **Subtask 8**: `.golangci.yml`, `.github/workflows/ci.yml`, `.github/dependabot.yml`, `Makefile`, `.gitignore` needed only the reference sweep (no `--help`, no functional wiring change — `Makefile`'s `comment-refs` target and the pre-commit dispatcher are subtask 10; `verify`'s prerequisite and the CI job are subtask 15, per the design's two-landing split).
- **Subtask 9**: for each of `config/balance.yaml`'s leaf keys, the source `docs/DESIGN.md` section named by its own pointer was read (§2.2.2 for the chunk grid, §3.3 for stamina, §5/§3.5 for the noise-standing timers, §3.4 for death, §3.5 for the AFK cruelty dial, §2.3/§2.2.3 for doors, §4.6 for the monster budget, §4.2/§4.3/§4.4 for combat, §6.3/§6.4 for the shop) **before** the pointer was removed, per D13's ordering requirement — this is translation of substance into self-contained English prose, not transcription of the Russian wording, consistent with KD-12 (English for `config/**`) and the project's Russian-for-`docs/**`-only convention.
- **Subtask 9**: no numeric value in either `.env.example` or `config/balance.yaml` changed — verified by grepping the diff for `key: value` / `LAB_GAME_*=` lines and confirming zero matches, not merely by intent. `go test ./internal/config/...` staying green (including the disjointness test between `.env.example`'s key set, the loader's actually-consulted set, and the loader's own declared set, and the balance-schema round-trip test) is independent confirmation that the schema and the example environment still agree.
- **Subtask 10 — the group's own completion condition**: since no permanent regression suite exists yet for the dispatcher (that lands in Group B's subtask 13), the design requires the dispatcher be exercised directly in a scratch repository and the probe recorded here. Built five scratch repos (`git init`, `core.hooksPath .githooks`, a real `git commit`) and drove each of D16's cases through a real commit: (1) nothing gated staged → `pre-commit: no staged path of the comment-reference gated set; gate skipped`, commit succeeds, falls through to the ratchet; (2) a staged `.go` file carrying `AC6` in a comment → the gate's own finding line printed, commit refused (exit 1); (3) a staged clean gated `.go` file → no gate output, falls through to a normal ratchet run, commit succeeds; (4) `go.mod` removed from the scratch worktree → `pre-commit: no go.mod at the worktree root; comment-reference gate skipped` printed correctly (the ratchet's own subsequent failure in that same broken worktree is expected collateral of removing `go.mod`, not a dispatcher defect); (5) `PATH` narrowed to a minimal binary set excluding `go` → `pre-commit: go not on PATH; comment-reference gate skipped` printed, and the ratchet's own identical skip fired right after it, commit succeeds. All five match the design's named behaviour exactly. Also re-ran the existing `ai-docs/scripts/test-precommit-dispatch.sh` suite unchanged against the new symlink shape — still green, confirming its `[ -x .githooks/pre-commit ]` and `grep -q 'git rev-parse --show-toplevel' .githooks/pre-commit` assertions survive a symlink target exactly as the design predicted.
- **Subtask 10**: `make comment-refs` (the new target) was run once by hand against the whole tracked tree to confirm it invokes correctly — it exits non-zero today (found ~170 report lines, all in not-yet-swept harness scripts under `ai-docs/scripts/` and `.claude/skills/`), which is expected and does not block anything: the target is deliberately NOT added to `verify`'s prerequisite list yet, per the design's two-landing split — that wiring is subtask 15, after Group B's harness sweep.

- **Subtask 11**: `ai-docs/doc-convention.md` keeps DOC-1, DOC-2, DOC-3, DOC-5 and DOC-6 as they stood; DOC-4 inverts wholesale from "cite the design section" into the ban, and the section carries the banned-class table, the exemptions, the record of which two halves are review-judged and why, the record that workflow `run:` blocks and `.claude/settings.json` hook bodies obey the rule ungated, the `config/**` positive documentation requirement, and the usage-prose-behind-`--help` rule. The § Scope section was widened, because the file's opening sentence claimed it covered Go doc comments only while DOC-4 now governs every comment of the gated set; leaving that unwidened would have been the exact stale-claim shape the ban exists to stop.
- **Subtask 11**: DOC-3's two symbol-naming bullets were left byte-identical (the spec keeps them under KD-9) and a closing sentence was added tying them to DOC-4's same-package carve-out, so a reader arriving at DOC-3 first does not read it as contradicting the ban.
- **Subtask 11 gates**: the file is markdown and therefore outside the gated set, so `make comment-refs` is not the applicable gate; the CI relative-markdown-link check was run over the whole tree instead (clean), which is what the new `go-api-naming.md` link needed.
- **Progress-file hygiene**: the `## Subtasks` list carried a stale duplicate block of unchecked rows 3–10 left over from Group A's edits, directly contradicting the checked rows above them. Removed — a progress file whose own checklist asserts both states of the same subtask is worse than no checklist.

- **Subtask 12 — the membership set, derived and argued**: the set owing `--help` was derived at the task's `base_commit` (f1bc60f, before any sweep touched a comment) with a **case-insensitive** scan for a usage-introducing comment, and the case-insensitivity is load-bearing: `append-task-run.sh` heads its block `# USAGE`, and a case-sensitive sweep reports it as owing nothing. Result: 19 scripts owe the flag (18 here plus `coverage-ratchet.sh` from subtask 8); three do not — `check-citations.sh` (the spec resolves it explicitly to no), `cleanup-progress.sh` (no usage comment at all; it already prints its grammar at runtime on a wrong call, which is the spec's own complement criterion), and the new `pre-commit.sh`.
- **Subtask 12 — how the ambiguous case was settled**: a one-line `# Usage: bash <path>` on a suite that takes no arguments could be read either way — as usage prose (owes a flag) or as "only a path, no invocation grammar" (the reasoning the spec uses to exempt `check-citations.sh`). Read as **owes**, on the owner's own framing (*"if usage is absent, `--help` isn't needed either"* — usage is not absent here, it is spelled `Usage:`) and on the substitution's stated rationale (*"a renamed script's `--help` still works; its `# Usage:` line silently lies"*), which is exactly about a line naming the script's own path. `check-citations.sh` stays exempt because its line is not a usage line — it is a "runnable standalone" note inside a sentence about the CI surface, which is the single case the spec names.
- **Subtask 12 — the block is copied, not authored**: a helper read the `usage()` template and the dispatch `case` out of `.githooks/coverage-ratchet.sh` by regex at insertion time and substituted only the grammar lines, so no script's shape was retyped. Verified after the fact rather than assumed: `git ls-files '*.sh' | xargs grep -c '^  -h|--help) usage; exit 0 ;;$'` reports exactly 19 carriers, and `sort -u | cat -A` over every `-h|--help` line in the tracked tree collapses to the single byte sequence `  -h|--help) usage; exit 0 ;;$`.
- **Subtask 12 — the one script that needed more than an insertion**: `doc-edit-guard.sh`'s `usage()` read its own comment block back out of the file (`sed -n '2,32p' "$0"`), which the sweep destroys, and it exited 1 unconditionally. It now carries the same heredoc shape as every other script, prints to stdout and exits 0 for `--help`, and its two wrong-call sites became `usage >&2; exit 1` so the refusal direction is unchanged. Its suite's "usage errors exit 1" case still passes.
- **Subtask 12 — what the sweep deleted, deliberately**: the `tmp/`-directory rationale in the gate-log guard's header, the coordinate-drift narrative in the citation guard (kept as behaviour, lost as a coordinate), the pull-request and issue numbers behind several guards' "why it is a gate" paragraphs, and every design-section pointer. Where a whole clause existed only to carry a pointer it went with it. Nothing was relocated to a markdown page and nothing was reworded into a still-outward pointer — the owner's decided cost, recorded in the design's risk row.
- **Subtask 12 — a script's printed failure message is not a comment and was not touched**: `check-ac-shape.sh`'s `MSG` heredoc still names the spec-writing rule and the corrections-log date, which is where a good deal of the rationale the header lost still lives. The gate agrees: it reported no finding inside a heredoc in any script, which is also independent evidence that the shell extractor's heredoc handling behaves as the design predicted.
- **Subtask 12 — behaviour preservation was measured, not assumed**: every one of the thirteen guard regression suites CI runs was executed after its own script's edit and after the whole sweep, plus the three standalone checkers over the live tree, plus `--help` on each of the 19 carriers and `make shellcheck`. The two pure-sweep scripts (`check-citations.sh`, `cleanup-progress.sh`) were additionally diffed for non-comment lines — there are none, which is what AC15 asks of a file whose only reason to appear in the diff is the sweep.

- **Subtask 13 — the checker's four rules**: (1) a script's help dispatch must be exactly one line and byte-identical to the marker, (2) nothing may run before it — walked with `awk` rather than grepped, since the question is what *precedes* the dispatch and only a scan answers that; comments, a `set` line, function definitions and assignments without a command substitution are allowed, anything else is a finding, (3) the flag must exit 0 and print a non-empty block, run dynamically and safe to run precisely because rule 2 held first, and (4) a tracked file beginning with a shebang is named with the shell extension or is a symbolic link to one that is.
- **Subtask 13 — what counts as a "help dispatch"**, so the checker does not report a script that merely mentions the flag: a case arm anchored at the start of its line, or a conditional naming the **long** form. The short form alone was deliberately dropped from the conditional branch — `if [ -h "$f" ]` is a file test and spells it the same way, and a checker that false-positived on symlink tests would be turned off rather than fixed. Verified against the live corpus: the two `--help` occurrences in the piped-gate suite (a fixture payload and the hook's own carve-out pattern) are correctly not dispatches.
- **Subtask 13 — the checker was seen to go RED on the real corpus, not only on its fixtures**: replayed against the task's base tree (`git archive f1bc60f` into a scratch repository), it reports `.githooks/pre-commit begins with a shebang but is neither named with the shell extension nor a link to one that is` and exits 1 — the one file this task converted to a symlink. That is the pre-change control the harness's own pattern asks for, and it is stronger evidence than the sandbox fixtures that rule 4 is load-bearing.
- **Subtask 13 — the dispatch suite's new cases were mutation-tested, both directions**: with the dispatcher's whole gate block deleted, the suite reports five failures including `gate-refuses` and `gate-argv`; with the staged-path filter widened to every path, it reports exactly one — `gate-skip-nothing-gated`, the never-invoked case. Each mutant was applied to a `cp` backup and reverted, with `git diff --name-only` confirming the restore. Without those two runs the new cases would be a green instrument.
- **Subtask 13 — the stub records its own argument vector** rather than the suite asserting the invocation from the dispatcher's source: `gate-marker.txt` must read `--staged` after a refusing run and must not exist after a skipping one, so the "gate ran over the staged set" claim comes from the run and not from a grep of the file that makes it.
- **Subtask 13 — the two new scripts answer no `--help`, deliberately.** AC17 and AC18 are derived from the pre-sweep tree, and a file that did not exist there carried no usage prose; adding a flag would be the permissive reading of AC18's "no script that carried no usage prose answers `--help`". The restrictive reading is taken, which is also what subtask 10 did for the new dispatcher. Neither new script takes an argument, so nothing is left undocumented.
- **Subtask 13 — one `shellcheck` suppression**: the checker's failure message prints the dispatch block as advice, and the block contains the parameter-expansion form literally, which trips SC2016. Suppressed with the specific code and a stated reason. The advice interpolates the same `marker` variable the assertion compares against, so the printed shape cannot drift from the enforced one.

- **Step 8 Group A**: the orchestrator re-ran build, vet, test, lint and the new gate against the returned tree rather than accepting the group's summary. All green; `make comment-refs` reports only harness `*.sh` findings, which subtask 12 owns, and `SKIP .githooks/pre-commit` confirms the GO-note-1 symlink branch works.

## Key discoveries (don't re-investigate)

- `go.yaml.in/yaml/v3` never reports a comment's own line — it reports the attached node's line, and it attaches across a blank line contrary to its own field doc. D1a's nearest-match reconciliation, with `UNRECONCILED ⇒ exit 2`, is the answer; an offset rule is measurably wrong on `config/balance.yaml`.
- No tracked `*.sh` is free of a banned-class comment (`comm -23` complement empty, both operands non-empty). That is why the wiring lands in two parts: anything judging the whole tree before subtask 12 would be red.
- `pg_query_go/v6` can answer the SQL comment-span question but costs cgo; it is the named escape hatch, not a rejected-on-capability alternative.
- Adding `mvdan.cc/sh/v3` grows `go.sum` by its own test-dep checksums and normalises the `go` directive `1.26` → `1.26.0`, because that dependency's own `go.mod` declares `1.26.0`.
- No script in this corpus resolves its own location — every one that resolves anything resolves the repo root via `git rev-parse --show-toplevel`. That is why `--help` is copied per script rather than sourced from a library.

- The `--help` membership criterion must be applied **case-insensitively**: one script heads its block `# USAGE`, not `# Usage:`, and a case-sensitive derivation silently puts it in the complement.
- Three tracked `*.sh` legitimately answer nothing to `--help` and must stay that way: `check-citations.sh`, `cleanup-progress.sh`, `.githooks/pre-commit.sh`. Passing `--help` to the first two simply runs them, which is the pre-existing behaviour the spec measured and is what AC18 asks for.

- The gate's report names the matched fragment, not the whole reference: `scripts/test-*.sh` is reported as `repo-path: .sh`, because the `*` ends the path match. The file, line and class are all correct, so AC4 holds; the fragment is a readability question for self-review, not a classification defect.

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
- `internal/store/*.go` (all non-test and test files), `internal/store/migrations/*.sql` — comment sweep
- `internal/testdb/testdb.go`, `internal/testdb/testdb_test.go` — comment sweep
- `internal/tg/*.go` (all 17 files) — comment sweep
- `internal/tgtest/tgtest.go`, `internal/tgtest/tgtest_test.go` — comment sweep
- `internal/ingest/*.go` (all 23 files) — comment sweep
- `internal/scheduler/*.go` (all 22 files) — comment sweep
- `.golangci.yml`, `.github/workflows/ci.yml`, `.github/dependabot.yml`, `Makefile`, `.gitignore` — comment sweep
- `.githooks/coverage-ratchet.sh` — comment sweep + the D9 `--help` shape (`usage()`, the `-h|--help` case)
- `.env.example` — header + per-key comment sweep, no key/value change
- `config/balance.yaml` — every design-section pointer replaced with self-contained English prose read from that section, no value change
- `Makefile` — new `comment-refs` target (not a `verify` prerequisite yet)
- `.githooks/pre-commit` — now a symlink to `.githooks/pre-commit.sh`
- `ai-docs/scripts/*.sh` (all 15) — comment sweep; 15 of them gain the D9 `--help`
- `ai-docs/scripts/check-script-shape.sh` — new: the four shape rules over the tracked tree
- `ai-docs/scripts/test-script-shape.sh` — new: its regression suite, one sandbox per defect plus the all-conforming instrument check
- `ai-docs/scripts/test-precommit-dispatch.sh` — extended: the symlink case and the four gate-dispatch cases on the stub seam
- `.claude/skills/ai-audit/scripts/check-citations.sh` — comment sweep only (owes no `--help`)
- `.claude/skills/ai-audit/scripts/test-check-citations.sh`, `.claude/skills/task/scripts/append-task-run.sh`, `.claude/skills/task/scripts/test-append-task-run.sh` — comment sweep + the D9 `--help`
- `.claude/skills/pr-merged/scripts/cleanup-progress.sh` — comment sweep only (owes no `--help`)
- `ai-docs/doc-convention.md` — DOC-4 inverted to the ban; § Scope widened to the gated set; DOC-1/2/3/5/6 substantively unchanged
- `.githooks/pre-commit.sh` — new file: the dispatcher (comment-reference gate over `--staged`, then the coverage ratchet), with D16's three named skip conditions
