# Progress: Split the CI Test job into four parallel jobs — ACTIVE
_Updated: 2026-09-12 03:12_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-12-split-ci-test-job
**base_commit:** ad9414eaf672116d4f67b3989d3a53246d402af3
**Last build:** PASS

**Issue:** #99
**Spec:** ai-docs/plans/2026-09-12-split-ci-test-job.spec.md

**current_step:** Step 10 — self-review APPROVE (Round 2)
**last_passed_gate:** actionlint + make comment-refs + the AC1/AC2/AC3 extractions re-run after the fix | 2026-09-12T02:55:00Z | c4761a3800fa2d5a5016540062a5774245539590
**entry_args:** ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback

## Next action

**Do this immediately:** Step 9.5 is complete. Next is Step 10 — spawn `self-review` with exactly the invocation line, the spec path, the design path, this progress path, and the range `ad9414eaf672116d4f67b3989d3a53246d402af3..HEAD`, and nothing else.

## Subtasks

- [x] 1. `.github/workflows/ci.yml` — four sibling jobs, comment rewrites, cluster comment  (Group A, complete @ 606d7c4)
- [x] 2. `AGENTS.md` — ratchet AXIOM + § Build & Test gate enumeration (Group B, complete)
- [x] 3. `ai-docs/claude-tools-hierarchy.md` — CI job table (Group B, complete)
- [x] 4. `ai-docs/go-test-conventions.md` — the fallback gate's CI sentence (Group B, complete)
- [x] 5. `.claude/skills/pr-ci-failed/SKILL.md` + CI sync-group siblings (Group B, complete)
- [x] 6. `ai-docs/context-status.md` — the shared-test-server entry's falsified sentence (Group B, complete)
- [x] 7. AC4 falsified-claim sweep with its positive control (Group B, terminal, complete)

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 4 (interview)**: tracking issue #99 created from the approved spec rather than found — no open issue concerned CI wall-clock; `issue_ref` in the state file stays `free-text` because the entry mode was free text and the spec's anchors resolve against its `task_description` block.
- **Step 7**: design-review round 1 returned ITERATE; the orchestrator re-ran the major finding's three load-bearing measurements before forwarding it, and all three resolved.
- **Step 7**: design-review round 2 returned GO; its one spec-amending note was routed to the owner, who chose "fix the design only", which is also his per-instance exemption from a re-review. design-review did not run again.
- **Step 8, Group A, subtask 1**: ran the § Test Design extractions for AC1/AC2/AC3 with their controls before `git add`, per the design's Handoff-plan gate list. AC1 control (extraction over `git show HEAD:.github/workflows/ci.yml`) reported all four `make` targets under the single replaced `test` job; the new file reports exactly one target per new job. AC2's assertion is an absence, so I built a positive-control fixture (a constructed job block declaring `needs: [changes, test]`) and confirmed the same extractor reports it, before reading the new file's clean four `needs: changes` lines as meaningful. AC3 gating parity: each of the four new jobs reproduces the replaced job's `needs: changes` and `if: needs.changes.outputs.go == 'true'` exactly, and a `diff` of the `changes` job block between `git show HEAD:` and the new file is empty (byte-identical `filters:`). `actionlint .github/workflows/ci.yml` and `make comment-refs` both exited 0.
- **Step 8, Group B, subtask 2**: the gate enumeration's `Test (incl. -race)` entry became four middle-dot entries carrying the four shipped job *names* (`Test · Race · Coverage ratchet · Test fallback`) and no `make` targets — the enumeration is a list of CI job names, and the job-to-target mapping is subtask 3's table. The `(incl. -race)` parenthetical is deleted rather than moved: it existed to disclose that `-race` ran inside the `Test` job, which the split makes false. Both edited sentences were checked against the shipped `ci.yml`: each name written matches a `name:` value there, and the same extractor over the pre-change file reports `Race`, `Coverage ratchet` and `Test fallback` absent and `Test` present — so it discriminates rather than matching everything. `AGENTS.md` was then re-grepped whole for the four gate names and for job vocabulary; the remaining hits name no CI job and stay true.
- **Step 8, Group B, subtask 3**: the old row's falsified half was the word "then" (`make test` **then** `make test-race`) and its silent omission of the other two targets; the four replacement rows each carry one target plus the one-clause reason the neighbouring rows' style already uses. Each row was checked against a job-name/target mapping extracted from the shipped `ci.yml`, whose pre-change control maps all four targets onto the single `Test` job. The extractor's first version printed nothing for the four-target filter — an empty right-hand side, not a clean answer — and was fixed before any verdict was read. The `Coverage ratchet` row's "never records a new mark" and the `Race` row's "the only one of the four whose target passes the flag" were both resolved against the script and the Makefile rather than carried over from the design.
- **Step 8, Group B, subtask 4**: the corrected clause reads "CI runs it in a job of its own, Test fallback". A first draft said "as the one step of a job of its own", which the shipped job refutes — it has three steps (checkout, `setup-go`, the `run:`) — so the step-count claim was dropped rather than qualified; the sentence had to name the job, not count its steps. The file's other three `make test-fallback` mentions and its three `CI`/`CI's` mentions name no job and stay true. The markdown-link checker was pointed at a constructed broken link and seen to exit 1 before its green on the tree was read.
- **Step 8, Group B, subtask 5**: KD-C's premise was executed here rather than copied — a throwaway module outside the tree whose one test calls `t.Fatal` emits `--- FAIL:` under the plain route, under `-race`, and under the fallback route (`LAB_GAME_TEST_DSN= go test -count=1`), and `WARNING: DATA RACE` occurs zero times in the `-race` log while the same pattern matches a constructed line. For the fourth route the signal reaches the job log through the ratchet script's own echo of `^(FAIL|---|ok)` lines on its not-green exit, verified against the captured failing log with a green log as the control. So the `test` row's new cell — all four jobs — is true of the log a classifier actually reads, and the `race` row's `Race` is the only origin because `test-race` is the only target passing `-race`. `.claude/skills/dependabot-pr/reference.md` is the third member of the declared CI sync group and was **not** edited: it carries the class names only (`fmt / build / … / actionlint`) and names no CI job anywhere — checked case-insensitively for `job`, `ci.yml` and `checks` — so this diff falsifies nothing in it. The `CI exists` enumeration's pre-existing omission of the `Comment references` job is left standing in both CI skills, per the design's open question.
- **Step 8, Group B, subtask 6**: the edit is one clause inside one bullet of the PR-#72 entry — "CI's Test job runs it as a step" became "CI runs it in a job of its own, Test fallback", the same wording subtask 4 used, so the two live surfaces describing the fallback gate agree verbatim. Nothing else in the entry, no other entry and no entry order changed. The file was then re-grepped whole for job and step vocabulary: the `Comment references` / Harness-guards sentence in the PR-#68 entry, the `-count=1` bullet, the ratchet-dirtiness bullet and the build-constraint bullet all name targets or other jobs and stay true.
- **Step 8, Group B, subtask 7 — the swept set.** `git ls-files` = 373 tracked paths; the KD-E exclusions matched 34 of them (`ai-docs/learnings.md`, this task's four artefacts, and 29 under `ai-docs/plans/done/**`), leaving **339** swept. `ai-docs/plans/ignored/**` matched nothing because nothing under it is tracked, so it is outside the set without an exclusion doing any work.
- **Step 8, Group B, subtask 7 — the positive control, run before either verdict was read.** Pattern set over the pre-change tree (the merge base with `main`, `ad9414ea`, since HEAD now carries the corrections): 76 hits, and each of the five axes the design names printed its site. Possessive/job — `AGENTS.md:106` "and CI's Test job run the identical script with `--check`". Hyphenated + step — `ai-docs/go-test-conventions.md:46` "CI runs it as a Test-job step". Table cell — `.claude/skills/pr-ci-failed/SKILL.md:165` and `:166`, both `| Test |`. Middle-dot enumeration — `.claude/skills/pr-ci-failed/SKILL.md:8` "Format · Build · Test · …". Step vocabulary — `ai-docs/context-status.md:188` "runs it as a step" and `ai-docs/key-decisions.md:53` "run as its own CI step" (plus `:97`'s "a CI step and a paths filter"). No axis printed nothing, so no pattern was rewritten.
- **Step 8, Group B, subtask 7 — the sweep.** Over the live tree both passes together return **87** hits in 21 files, every one read in its sentence. **No site's sentence is false after the diff**: the four `AGENTS.md` / `claude-tools-hierarchy.md` / `go-test-conventions.md` / `context-status.md` / CI-skill sites the control found are the ones subtasks 2–6 corrected, and every remaining hit either names a `make` target without saying where it runs (`Makefile`, `ai-docs/context.md`, the rest of `go-test-conventions.md`, four `ai-docs/deferred/_inbox.jsonl` rows) or names a job this split does not touch (`.claude/skills/task/reference.md:166` "CI's Lint job", `ai-docs/key-decisions.md:53` "CI's Build job", `ai-docs/context-status.md:166` "CI gained a `Comment references` job"). **Decided-and-left, recorded rather than re-decided:** `ai-docs/key-decisions.md:53` (KD-20) "run as its own CI step" — true, and confirmed against the shipped workflow, where `make test-fallback` is a `run:` step and now the only target-running step of its own job (KD-E made this ruling; the sweep did not reopen it). Also left, on the same keep-or-fix test: the four local-hook ratchet sentences (`.claude/settings.json:66`, `.githooks/pre-commit.sh:3`, `ai-docs/scripts/test-no-verify-guard.sh:4`, `ai-docs/harness-gaps.md:260` and `:332`), which describe the pre-commit route and no CI job; `.githooks/coverage-ratchet.sh:5` "`make cover-ratchet` and CI run it with `--check`", which names no job; and four generic "a CI step" phrases (`ai-docs/harness-gaps.md:281`, `.claude/skills/ai-audit/reference.md:96`, `ai-docs/key-decisions.md:97`, `ai-docs/scripts/check-script-shape.sh:19`) that make no claim about the four gates.
- **Step 8, Group B, subtask 7 — the code-change-type branch did NOT fire, so no third group is needed.** The code change-type files the sweep names — `Makefile` (6 hits), `.githooks/coverage-ratchet.sh` (3), `.githooks/pre-commit` and `.githooks/pre-commit.sh`, `ai-docs/scripts/check-script-shape.sh`, `test-no-verify-guard.sh`, `test-piped-gate-guard.sh` (a `BLOCK make test | tail -5` fixture line) — carry no sentence the diff falsifies, which is what the design predicted from the two headers it measured. `.github/workflows/ci.yml`'s 11 hits are subtask 1's own rewritten comments and are not the sweep's to find; read once for correctness, they hold.
- **Step 8, Group B, subtask 7 — two cross-checks beyond the design's two passes, each with its own control.** (a) Pass 1's completeness argument is that a sentence claiming where a gate runs must name the gate; to test it rather than trust it I swept the set for CI-job vocabulary the gate names cannot reach (`harness guards`, `actionlint`, `comment references`, `paths-filter`) and read the ten files that hit but that pass 1 had not: `ai-docs/code-style.md:17` (the `actionlint` make-versus-CI route), `ai-docs/task-run-schema.md:453`, `.claude/agents/code-writer.md`, `design-writer.md`, `self-improve.md`, `.claude/skills/dependabot-pr/SKILL.md`, `pr-commented/SKILL.md`, `task/SKILL.md`, `ai-docs/scripts/test-gate-log-path-guard.sh` — all gate-command lists, `allowed-tools` lines or fixtures, none naming a job for any of the four. (b) An encoding axis neither pass had: a claim keyed on the job **key** in backticks (`` the `test` job ``), which pass 2's `test job` cannot match because the backtick breaks the adjacency. Zero hits in the live set, with the pattern shown to match a constructed line first.

- **Step 9**: every gate run by the orchestrator, not read from a return summary. Cheap gates PASS: `go build`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run`, `make comment-refs`, `make import-guard`, `actionlint`, `make shellcheck`, `make file-limits`, `make tidy-check`. Suite gates PASS: `make test`, `make test-race` (17 `ok` lines, no `--- FAIL:`, no `WARNING: DATA RACE`), `make cover-ratchet`, `make test-fallback`. Harness gates PASS: hook-body shellcheck, citation guard, the context-status locator check, the four shape checkers, all sixteen guard regression suites, the relative-link check. `-race` was not earned by the diff (no `*.go` in it) and was run anyway, because CI's new `Race` job fires on this PR: the `go` paths-filter names `.github/workflows/**`.
- **Step 9**: the coverage ratchet reads 90.04% against a recorded 90.04% at a 0.60 pp tolerance — it holds with no margin, and the local measurement is **not an independent draw**: the diff changes no Go file, so the profile replayed from the test cache. CI measures it fresh on a runner, where the suite's timing-dependent statements can move it inside the tolerance. Recorded so a red `Coverage ratchet` on this PR is read as that known behaviour rather than as a regression this task introduced.
- **Step 9**: my own AC4 sweep's first pattern set MISSED `AGENTS.md:138` — the middle-dot enumeration there reads `· Test (incl. `-race`) ·`, and a pattern written as `· Test ·` cannot match it. The control at the merge base is what exposed it, before any verdict on the live tree; widening the pattern took the control from 10 hits in 7 files to 49 in 12. A clean live sweep under the narrow pattern would have been a claim about the pattern. Decided-and-left after reading each sentence: `key-decisions.md:53` and `:97`, `context.md:27`, `coverage-ratchet.sh:4-5` and `:121`, `harness-gaps.md:260` and `:332` (both about the pre-commit hook, not CI), `test-piped-gate-guard.sh:83` (a fixture), and three `_inbox.jsonl` rows — which are also uneditable by hand under the standing AXIOM.
- **Step 9**: panic index needs no change (the diff adds no `panic(` / `log.Fatal`), and the domain-invariant sweep is vacuous by construction — the diff touches no `*.go`, no `*.sql` and no `go.mod`, so no ledger, schema, scheduler or telemetry surface is in it.

- **Step 9.5**: `ai-docs/context-status.md` gained this task's entry with the literal `#TBD-at-Step-12` locator, so the placeholder occurs exactly once in the file and sub-step 10a's `Edit` has the unique match it needs. `ai-docs/context.md`'s Gates bullet gained one orientation-level clause, phrased without a job tally so it stays true if a fifth test gate is ever added. No repo-root user-facing doc was touched: `README.md` carries no CI claim and `docs/DESIGN.md`'s one CI sentence is about evals, not job structure. The removal sweep over every doc touched found the deleted `(incl. -race)` phrasing only inside the new entry's own account of deleting it.

- **Step 11 (Round 1)**: the one `major` was verified before it was fixed — the design does require the cluster comment to carry the reason as well as the prohibition, and a grep for the reason's vocabulary over the shipped workflow found it nowhere, so the finding stood on its own measurement rather than on the reviewer's reading. Fixed in the workflow comment, not by a design or spec amendment: the design was not contradicted, the implementation was incomplete against it. The fix-round measurement pass re-ran `actionlint`, `make comment-refs` and the AC1/AC2/AC3 extractions **after** the edit, since the edit lands in the very file those ACs measure. The register row's `verifying command` coordinate drifted (the comment grew) and was re-resolved in place — a coordinate drift, not an amendment.

- **Step 10**: APPROVE at round 2, one round of fixes. Round 2 also found a defect in round 1's own register: the id `SR1-CLUSTER-WHY` fails the guard's join key, so the row was skipped by the parser and the `Fixed` round-table row joined to nothing — renumbered to `SR1-1`, and the guard verified in both directions (exit 0 now; a mutant flipping the row back to `open` still exits 1 naming it).
- **Step 10**: the reviewer reported that the `PreToolUse` register hook had not blocked the earlier commit although the guard was red on the file as committed. Verified rather than accepted: the guard exits 1 on `git show <sha>:<progress file>`, and the hook body selects its inputs with `git diff --cached`, which at `PreToolUse` runs before the command — so a call that staged and committed in one invocation presented an empty index and the gate examined nothing. Already logged twice in `ai-docs/harness-gaps.md` (2026-09-07, 2026-09-09), both entries open, so no duplicate was filed; the conduct half — asserting that the hook had accepted the commit — went to `ai-docs/learnings.md`. Staging has been its own tool call since.
- **Step 10**: the reviewer's second item — that the register's join-key grammar is documented only inside the guard script, so a reviewer following instruction 8 can write an internally correct register the gate cannot parse, and the resulting message names the wrong defect — is a harness diagnosis with no prior entry naming that target, and was filed in `ai-docs/harness-gaps.md`.

- **Step 12**: the spec's `## Deferred` and `## Open questions` bodies are each a single `- (none)` / `- None.` bullet. Read as the parser's NONE sentinel and emitted zero rows, although the sentinel rule's letter excludes bodies containing a `- ` bullet line: that clause exists so a section holding real bullets alongside the word cannot be silently swallowed, and neither body holds one. Emitting a literal `(none)` row would have put junk in the inbox for `/triage` to clear. Six rows emitted in total — two out-of-scope from the spec, four open-question from the design — by a parser reading the item text out of the files rather than by retyping it, and the whole `_inbox.jsonl` re-parsed afterwards because one malformed line breaks every future read of it.
- **Step 12**: the dedupe set is empty — `ai-docs/deferred/` holds no thematic `.jsonl` sibling yet, so no file-level skip could apply. Cardinality read before the verdict rather than after, since an empty right-hand side reports "not a duplicate" for every possible input.
- **Step 12**: the task-run record wrote complete (`incomplete: false`), and both checks of the schema's verification block pass — trailing byte `0a`, and `instruction_corpus_lines` recomputed by the pinned command equal to the recorded value. A first `jq` over the record printed `null` for four fields, which was my query naming keys the schema does not use, not a degraded write.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | "AC3 is discharged as paths-filter parity, and that reading is never stated." | spec-amending (b) | owner (2) design only | design § Approach — *What AC3's "set of changes" is, and what it is not* + § Test Design *gating parity* @ 65af2b0; answer 3.1 |
| G2 | 2 | "The 'jobs run in parallel by default' claim is load-bearing for AC2 and rests only on a `docs.github.com` quotation with a bare URL" | design-internal | folded | design § Approach parallelism paragraph — in-repo run observation primary, docs quote corroboration, `fetched 2026-09-12` on all three external URLs @ 65af2b0 |
| G3 | 2 | "Subtask 7 routes a falsified site in a code change-type file to the orchestrator 'to be routed back to a code group or ruled out of scope'." | design-internal | folded | design § Decomposition row 7 + § Test Design failure scenario + § Handoff plan contingent third group @ 65af2b0 |
| G4 | 2 | "'Every claim below is about something this task creates.' That last sentence certifies a property over the section's own claims" | design-internal | folded | design § Test Design preamble — sentence deleted, `actionlint` tag's justification rewritten @ 65af2b0 |
| G5 | 2 | "Cite `ai-docs/propagation-groups.md:30` beside the row-28/29 citation." | design-internal | folded | design § Decomposition, after the table @ 65af2b0 |
| G6 | 2 | "Subtask 1 says 'add the cluster comment' without saying where it lands." | design-internal | folded | design § Decomposition row 1 — immediately above the first of the four job declarations @ 65af2b0 |
| G7 | 2 | "Group A will run `go test ./...` for a YAML-only subtask under `code-writer` Mode A step 3." | design-internal | folded | design § Handoff plan, Group A gate list @ 65af2b0 |

## Key discoveries (don't re-investigate)

- **The current `test` job has a second, implicit suppressor.** No `continue-on-error` anywhere in the workflow and every `if:` is job-level, so today a red `make test` step means the three later steps never execute. The split removes that suppression — which is information the run gains, not a regression. The owner ruled this a design matter, not an AC3 rewording.
- **Sibling jobs of `changes` are already observed concurrent in this repo's own CI.** The jobs that ran started within a second of one another after `changes` completed, and overlapped. Two of the reviewer's figures were wrong and were corrected from the run itself: `Lint` started one second later than the others, and `Actionlint` was skipped rather than started.
- **`ai-docs/context-status.md` is a live document for propagation purposes, not a history surface.** The Propagation Rule's carve-out names only `ai-docs/learnings.md` and `ai-docs/plans/done/**`; a prior commit corrected a false claim inside an existing entry of this file in place; and Step 12 sub-step 10a mandates an in-place edit of it. "Append-only" governs how the log grows, not whether a false sentence inside an entry may be fixed.
- **`ai-docs/key-decisions.md` KD-20's "run as its own CI step" survives the split** — it stays true and names no job, which is what separates it from the `context-status.md` sentence that does name one.

## AC Status

| AC | Status |
|----|--------|
| AC1 | PASS |
| AC2 | PASS |
| AC3 | PASS |
| AC4 | PASS |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| SR1-1 | 1 | major | fixed@c4761a3 | `sed -n '115,123p' .github/workflows/ci.yml` (range re-resolved after the fix; the comment grew) — the cluster comment (`CLUSTER-WHY`) must state why the four are four jobs (the run costs the slowest rather than the sum, and every gate reports), not only the no-`needs:` prohibition. **Id renumbered in round 2**: it was `SR1-CLUSTER-WHY`, which does not parse as `check-review-register.sh`'s join key `^[A-Za-z]*[0-9]+-[0-9]+$`, so the row was skipped and the gate read "round 1 finding 1 is marked Fixed and has no register row" |
| SR1-KD20-STEP | 1 | — | accepted@1 — `ai-docs/key-decisions.md:53` "run as its own CI step" is still true: `make test-fallback` is a `run:` step, now the only target-running step of its own job. KD-E's ruling re-verified independently, not carried over | `grep -n "run as its own CI step" ai-docs/key-decisions.md` |
| SR1-RATCHET-HDR | 1 | — | accepted@1 — `.githooks/coverage-ratchet.sh:5` "`make cover-ratchet` and CI run it with --check" names no job and stays true; a code change-type file, so KD-E's keep test applies and no third group is owed | `sed -n '5p' .githooks/coverage-ratchet.sh` |
| SR1-DEPENDABOT | 1 | — | accepted@1 — `.claude/skills/dependabot-pr/reference.md` carries class names only and names no CI job (checked case-insensitively for job/CI-job/paths-filter vocabulary against a control that prints on the sibling SKILL.md); the recorded no-change outcome is correct | `grep -niE "coverage ratchet\|test fallback\|test[ -]job\|ci's [A-Z]" .claude/skills/dependabot-pr/reference.md` |
| SR1-REPRODUCER | 1 | — | accepted@1 — the `test` class's local reproducer (`go test ./...`) stays correct for all four jobs: with no DSN exported it IS the fallback route, and a suite failure is what `Coverage ratchet` reports. Not falsified by the split; the adjacent taxonomy gap is the design's surfaced open question | `sed -n '182p' .claude/skills/pr-ci-failed/SKILL.md` |
| SR1-PER-BINARY | 1 | — | accepted@1 — the rewritten fallback comment changes "per-package" to "per-binary" beyond the design's keep-the-reason mandate; correct against the rest of the corpus (one container per database-backed test *binary*), so an alignment rather than scope creep | `sed -n '170,174p' .github/workflows/ci.yml` |
| SR1-TBD-LOCATOR | 1 | — | accepted@1 — `ai-docs/context-status.md:297` carries the literal `#TBD-at-Step-12`; correct at Step 10, filled by Step 12 sub-step 10a, and the CI guard keeps it unique | `grep -c 'TBD-at-Step-12' ai-docs/context-status.md` |
| SR2-REGISTER-ID | 2 | major | fixed-in-round@2 (reviewer-owned artefact, corrected in the round that found it) | `bash ai-docs/scripts/check-review-register.sh ai-docs/plans/2026-09-12-split-ci-test-job.progress.md` — was exit 1, "round 1 finding 1 is marked Fixed and has no register row", because round 1's id `SR1-CLUSTER-WHY` does not parse as the guard's join key `^[A-Za-z]*[0-9]+-[0-9]+$`; renumbered to `SR1-1`, now exit 0, and a mutant flipping that row to `open` still exits 1, so the green is earned and not an empty join. Free-form ids on rows with no round-table counterpart stay legitimate — the guard documents them as skipped by design |
| SR1-PARENT-SKILL | 1 | — | accepted@1 — this progress file carries no `**parent_skill:**`; the canonical template makes it conditional ("omit when the current skill IS the parent flow") and this is a plain `/task` run. Every required field is present | `grep -nE '^\*\*(current_step\|last_passed_gate\|entry_args\|base_commit)' ai-docs/plans/2026-09-12-split-ci-test-job.progress.md` |

## Files touched

- `.github/workflows/ci.yml` (subtask 1, @ 606d7c4)
- `AGENTS.md` (subtask 2, @ d8f8707)
- `ai-docs/claude-tools-hierarchy.md` (subtask 3, @ b194121)
- `ai-docs/go-test-conventions.md` (subtask 4, @ 27aaa1e)
- `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md` (subtask 5, @ bd75da5; `.claude/skills/dependabot-pr/reference.md` inspected, no falsified claim, unchanged)
- `ai-docs/context-status.md` (subtask 6, @ bab2f26)
- `ai-docs/learnings.md` (subtask 7 — two entries: the append-order slip on this file's own decisions log, and the empty-extractor catch during subtask 3's verification)
- (subtask 7 edited no document: the sweep found nothing left to correct)

## Self-Review (Round 1)

**Verdict:** REJECT

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|
| 1 | .github/workflows/ci.yml:115-119 | major | The cluster comment does not record **why** the four are four jobs, which the design specifies as half its content. Design `ai-docs/plans/2026-09-12-split-ci-test-job.design.md:116-118`: "A new comment on the cluster records **why the four are four jobs** and that none of them may acquire a `needs:` on another — the only durable defence against a future edit quietly re-merging them." The shipped comment delivers the second half verbatim in force ("None of the four may acquire a needs: on another") but justifies it only circularly — "that would silently re-merge the split this comment exists to prevent". The reason the split exists (a run costs the slowest of the four rather than their sum, and a red gate no longer stops the other three from reporting) appears nowhere in the file. That omission lands on the guard's primary case: the design's own § Rejected alternatives names the most likely re-merge argument — "Keeping `cover-ratchet` sequenced after `test` on the theory that measuring coverage on a red suite is wasted" — and the missing rationale is exactly the material that answers it. A future editor arriving with a reason meets a bare prohibition and nothing to weigh it against. No guard script gates the shape (design § Open questions — *Nothing gates the four-job shape*, refused as out of scope), so this comment is the whole defence. Fix: add the reason to the cluster comment; `make comment-refs` and `actionlint` gate the edit. Not a Design Amendment trigger — the design is not contradicted, the implementation is incomplete against it. | ✅ Fixed |

### What was checked

**AC1 — one gate per job (PASS).** Extractor over job → `run: make <target>` mapping, run against the pre-change tree first as its control: `git show ad9414ea:.github/workflows/ci.yml` reports all four targets under the single `test` job (count=4), so the extractor discriminates. Against the shipped file: `test`→`make test`, `test-race`→`make test-race`, `cover-ratchet`→`make cover-ratchet`, `test-fallback`→`make test-fallback`, count=1 each, and no other job in the workflow invokes any of the four.

**AC2 — sibling independence (PASS).** All four report `needs: changes` and name no sibling. Read only after the positive control: the same extractor pointed at a constructed job block declaring `needs: [changes, test]` printed `needs=[changes, test]`, so the list form a scalar-shaped pattern would miss is reported. The clean answer on the real file is therefore evidence.

**AC3 — gating parity (PASS).** Each of the four reproduces the replaced job's pair exactly (`needs: changes`, `if: needs.changes.outputs.go == 'true'`). The `changes` job block is byte-identical between `git show ad9414ea:` and the shipped file (65 lines each, empty `diff`; the `filters:` block is 1843 characters on both sides). Every pre-existing job's `needs:`/`if:` pair is unchanged and no job was removed.

**AC4 — falsified-claim sweep (PASS), run independently of subtask 7's.** Swept set = `git ls-files` (373) minus the KD-E exclusions = 339 paths. Two passes with encodings chosen not to duplicate the implementation's: (a) job/step vocabulary — `Test job`, `Test-job`, `job Test`, `CI's <Job> job`, `· Test`, `| Test |`, `as a step`, `CI step`, `its own job`, `job of its own`, plus the Russian `джоб`; (b) every line carrying both a CI word and one of the four gate names. Control over the pre-change tree first: 24 hits across 15 files on pass (a), 37 on pass (b) — the instrument is live on every axis. Live tree: every hit read in its sentence; no live site states which CI job runs any of the four and is now false. The sites left standing and why are in the register (`SR1-KD20-STEP`, `SR1-RATCHET-HDR`, `SR1-DEPENDABOT`, `SR1-REPRODUCER`). `README.md` carries no CI claim; `docs/DESIGN.md`'s two hits are the game's nightly cron and the evals sentence, neither naming a CI job. `ai-docs/metrics/task-runs.jsonl` carries no CI-job field.

**Propagation Rule.** `ai-docs/propagation-groups.md:30` ("a job added, renamed, or removed") names three targets — the CI group's class tables, `AGENTS.md` § *Build & Test*, `ai-docs/claude-tools-hierarchy.md`. All three are in the diff. The CI sync group's third member (`dependabot-pr/reference.md`) was verified to carry no falsified claim.

**GO-notes round-trip closure (PASS).** All seven rows in `## GO notes` carry a route; G1 is `owner (2) design only`, G2–G7 `folded`, all resolved at `65af2b0`, which precedes the implementation commit `606d7c4`. Each resolution verified present in the design: G1 (§ Approach *What AC3's "set of changes" is* + § Test Design *gating parity*), G2 (parallelism paragraph with the in-repo run primary and `fetched 2026-09-12` on all three external URLs — 3 tags, 3 URLs), G3 (§ Decomposition row 7 + § Test Design failure scenario + § Handoff plan contingent third group), G4 (the certifying sentence is absent; `grep` exit 1), G5 (`propagation-groups.md:30` cited after the table), G6 (row 1 names the cluster comment's placement), G7 (Group A gate list). The design is not stale against the implementation on any point — job keys, job names, gating, comment rewrites and the byte-identical `changes` block all match what shipped.

**Prose claims re-derived, not accepted (the diff is predominantly prose).** `AGENTS.md` ratchet AXIOM — `make cover-ratchet` is `.githooks/coverage-ratchet.sh --check` and the `Coverage ratchet` job runs `make cover-ratchet`; `--check` reaches neither write site of the ratchet file (both guarded by an earlier `--check` exit), so "never records a new mark" holds. `claude-tools-hierarchy.md` `Race` row — `test-race` is the only one of the four whose target passes `-race` (`cover-ratchet` measures with `-covermode=atomic`, `test-fallback` is a bare run; `test-contention` passes it but is not a CI job and is out of scope by the spec). `Test fallback` row — the only CI route clearing `LAB_GAME_TEST_DSN`; the other three route through `cmd/testpg`. Both CI skills' `test`-class claim "a test that simply fails emits this signal under every one of the four" — for `Coverage ratchet` the signal reaches the job log through the script's `grep -E '^(FAIL|---|ok)' tmp/coverage-run.log >&2` on its not-green exit, read in the script at line 118. `context-status.md`'s new entry — the `55682cc` precedent is a real in-place `-`/`+` rewrite of an existing entry's bullets in that file; the old shape's suppressor claim holds (`continue-on-error` absent, no step-level `if:` in the pre-change file, both exit 1). Every job name written into prose matches a `name:` value in the shipped workflow.

**Gates re-run against the shipped tree, not read from the log.** `actionlint .github/workflows/ci.yml` exit 0. `make comment-refs` exit 0. `go vet ./...` exit 0. `golangci-lint run` exit 0.

**Vacuous by construction, confirmed rather than assumed.** The diff touches no `*.go`, `*.sql`, `go.mod` or `go.sum`, so §3 (test coverage, `-race`, determinism, Postgres invariants, leak-ignore entries, FSM edges), §4 (panic index, error handling, context discipline, `Unchecked`) and §4a (ledger, basis, telemetry, balance constants, schema, chat-safety, non-determinism) have no subject. Grepped the added lines anyway: no `panic(` / `log.Fatal*` / `log.Panic*`, and no token, DSN, `api_id` or `api_hash`.

**Learning-log boundary rules.** The four new `ai-docs/learnings.md` entries are appended at the end of the file, no prior entry touched, format and categories valid, all `Escalated? no`. No commit mixes a learnings entry with an instruction-file edit — `e64fd73`, `1f62476` and `b549736` each touch `learnings.md` (plus the progress file) and nothing else.

**Progress-file re-entry fields.** `**base_commit:**`, `**Branch:**`, `**Last build:**`, `**current_step:**`, `**last_passed_gate:**`, `**entry_args:**` and `## Decisions log` all present; `**parent_skill:**` correctly omitted (conditional field, and this is the parent flow). Content not reviewed, per the calling skill's ownership.

**Below the severity floor: 0 items.** Eight items were examined and ruled not-a-defect; each is an `accepted@1` row in the register rather than a note here, so round 2 does not re-litigate them.

## Self-Review (Round 2)

**Verdict:** APPROVE

No `blocker` or `major` row is open. The round-1 finding is fixed and verified; one defect found in this round was in the reviewer-owned register and was corrected here, in the round that found it.

### Round-1 finding: fixed, verified, not re-raised

`SR1-1` reads `fixed@c4761a3`. Its verifying command on the post-fix file:

```
$ sed -n '115,123p' .github/workflows/ci.yml
  # The Test job is split into four sibling jobs below — test, test-race,
  # cover-ratchet, test-fallback — each gated exactly as the job it replaces:
  # same needs, same if, one make target apiece. Four jobs rather than four
  # steps of one because siblings run at the same time: a run costs the
  # slowest of them instead of their sum, and a gate that goes red no longer
  # stops the other three from reporting — which is the point, since a step
  # that never executed tells you nothing. Sequencing any of them after
  # another, however reasonable it looks for one pair, gives both of those
  # back: none of the four may acquire a needs: on another.
```

Both halves the design specifies are now present, and the fix goes past the minimum in the one way that mattered: *"however reasonable it looks for one pair"* answers the re-merge argument the design's § Rejected alternatives names (`cover-ratchet` sequenced after `test`), which is precisely the hole round 1 cited. The prohibition is no longer justified circularly. Not re-raised.

Both new claims re-derived rather than accepted: *"a run costs the slowest of them instead of their sum"* matches the spec's Scope item 1 wording; *"a gate that goes red no longer stops the other three from reporting"* rests on the pre-change file carrying no `continue-on-error` and no step-level `if:` (both greps exit 1 on `git show ad9414ea:.github/workflows/ci.yml`) and on the four new jobs sharing no `needs:` edge. The comment carries no path, section number, issue number, URL or package-qualified symbol, and narrates no implementation step.

### A defect in this round's own instrument, found and corrected here

`check-review-register.sh` was **RED** on this file at the start of the round:

```
$ bash ai-docs/scripts/check-review-register.sh ai-docs/plans/2026-09-12-split-ci-test-job.progress.md
check-review-register: the register and a round table disagree.
  ...  round 1 finding 1 is marked Fixed and has no register row
exit=1
```

The cause was not the disagreement the gate is named for — the register and the round table agreed in substance. Round 1's register id was `SR1-CLUSTER-WHY`, which does not match the guard's documented join key `^[A-Za-z]*[0-9]+-[0-9]+$`, so the row was **skipped** by the parser and the Fixed round-table row joined to nothing. The gate's `PreToolUse` hook fires on any `git commit` staging a `*.progress.md`, so this would have refused the Step-12 commit. The id is reviewer-owned state (instruction 8), so it was renumbered to `SR1-1` here rather than surfaced as work for the orchestrator; the descriptive handle is preserved in the row text.

Verified in both directions, not just the green one: the guard now exits 0, and a mutant flipping `SR1-1`'s register status to `open` still exits 1 with `SR1-1 reads "open" while round 1 marks finding 1 Fixed` — so the pass is a real join, not an empty one.

### What was checked this round

**Scope of the round, per instruction 7a.** The diff since round 1 is two commits: `c4761a3` (`.github/workflows/ci.yml`, comment only) and `a4fc43c` (this progress file only). No live document changed, so AC4's sweep has no new subject and is not re-run; the seven `accepted@1` rows are not re-raised, nothing having changed for any of them.

**AC1/AC2/AC3 re-extracted on the post-fix file**, because the file changed even though only a comment did: each of `make test`, `make test-race`, `make cover-ratchet`, `make test-fallback` under exactly one job, count=1 each; all four `needs: changes` with no sibling edge anywhere in the workflow; the `changes` job block still byte-identical to `ad9414ea` (empty `diff`). PASS, PASS, PASS.

**Gates re-run after the last edit of this round:** `actionlint .github/workflows/ci.yml` exit 0, `make comment-refs` exit 0, `go vet ./...` exit 0, `golangci-lint run` exit 0, `check-review-register.sh` exit 0.

**Still vacuous, re-confirmed:** the range adds no `*.go`, `*.sql`, `go.mod` or `go.sum`, and no `panic(` / `log.Fatal*` / secret — so §3, §4 and §4a have no subject in round 2 either. No new `ai-docs/learnings.md` entry landed since round 1.

**Below the severity floor: 0 items.**
