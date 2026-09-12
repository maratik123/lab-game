# Progress: Split the CI Test job into four parallel jobs — ACTIVE
_Updated: 2026-09-12 02:46_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-12-split-ci-test-job
**base_commit:** ad9414eaf672116d4f67b3989d3a53246d402af3
**Last build:** PASS

**Issue:** #99
**Spec:** ai-docs/plans/2026-09-12-split-ci-test-job.spec.md

**current_step:** Step 9 — Verify (ALL PASS)
**last_passed_gate:** golangci-lint run | 2026-09-12T05:35:00Z | b5497363229f85f6271004ef02a56c64981026cd
**entry_args:** ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback

## Next action

**Do this immediately:** Step 9 is complete, every gate PASS and every AC verified by the orchestrator's own command. Next is Step 9.5 — append this task's entry to `ai-docs/context-status.md` with the literal `#TBD-at-Step-12` locator, and bump the affected summary bullet in `ai-docs/context.md`.

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

## Files touched

- `.github/workflows/ci.yml` (subtask 1, @ 606d7c4)
- `AGENTS.md` (subtask 2, @ d8f8707)
- `ai-docs/claude-tools-hierarchy.md` (subtask 3, @ b194121)
- `ai-docs/go-test-conventions.md` (subtask 4, @ 27aaa1e)
- `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md` (subtask 5, @ bd75da5; `.claude/skills/dependabot-pr/reference.md` inspected, no falsified claim, unchanged)
- `ai-docs/context-status.md` (subtask 6, @ bab2f26)
- `ai-docs/learnings.md` (subtask 7 — two entries: the append-order slip on this file's own decisions log, and the empty-extractor catch during subtask 3's verification)
- (subtask 7 edited no document: the sweep found nothing left to correct)
