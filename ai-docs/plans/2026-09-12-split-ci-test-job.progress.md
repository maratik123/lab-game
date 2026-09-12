# Progress: Split the CI Test job into four parallel jobs — ACTIVE
_Updated: 2026-09-12 05:22_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** chore/2026-09-12-split-ci-test-job
**base_commit:** ad9414eaf672116d4f67b3989d3a53246d402af3
**Last build:** not run

**Issue:** #99
**Spec:** ai-docs/plans/2026-09-12-split-ci-test-job.spec.md

**current_step:** Step 8 — Group B subtask 6 of 6 complete
**last_passed_gate:** citation guard + relative-markdown-link check + CI's context-status PR-locator check, its pattern shown to match a constructed placeholder | 2026-09-12 | this commit
**entry_args:** ускоряем gh ci: job Test надо разбить на 4 отдельные джобы, выполняющиеся параллельно: make test, make test-race, make cover-ratchet и make test-fallback

## Next action

**Do this immediately:** Group B's last subtask, 7 — the AC4 falsified-claim sweep over the live tracked set, both passes, with the pre-change positive control read before either verdict.

## Subtasks

- [x] 1. `.github/workflows/ci.yml` — four sibling jobs, comment rewrites, cluster comment  (Group A, complete @ 606d7c4)
- [x] 2. `AGENTS.md` — ratchet AXIOM + § Build & Test gate enumeration (Group B, complete)
- [x] 3. `ai-docs/claude-tools-hierarchy.md` — CI job table (Group B, complete)
- [x] 4. `ai-docs/go-test-conventions.md` — the fallback gate's CI sentence (Group B, complete)
- [x] 5. `.claude/skills/pr-ci-failed/SKILL.md` + CI sync-group siblings (Group B, complete)
- [x] 6. `ai-docs/context-status.md` — the shared-test-server entry's falsified sentence (Group B, complete)
- [ ] 7. AC4 falsified-claim sweep with its positive control (Group B, terminal)

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
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- `.github/workflows/ci.yml` (subtask 1, @ 606d7c4)
- `AGENTS.md` (subtask 2, @ d8f8707)
- `ai-docs/claude-tools-hierarchy.md` (subtask 3, @ b194121)
- `ai-docs/go-test-conventions.md` (subtask 4, @ 27aaa1e)
- `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md` (subtask 5, @ bd75da5; `.claude/skills/dependabot-pr/reference.md` inspected, no falsified claim, unchanged)
- `ai-docs/context-status.md` (subtask 6)
