# Design: Split the CI Test job into four parallel jobs

**Issue:** #99
**Date:** 2026-09-12

## Approach

### What stands today

One CI job named `Test`, keyed `test`, gated `needs: changes` plus
`if: needs.changes.outputs.go == 'true'`, runs the four gates as four sequential
steps, so a run that reaches them costs their sum and a red step means the steps
below it never execute
`[measured 4a4c909:.github/workflows/ci.yml:115-137 · sed -n '115,137p' .github/workflows/ci.yml → job key `test`, `name: Test`, `needs: changes`, `if: needs.changes.outputs.go == 'true'`, `runs-on: ubuntu-latest`, preamble `actions/checkout@v7` + `actions/setup-go@v7` with `go-version-file: go.mod`, then `run: make test`, `run: make test-race`, `run: make cover-ratchet`, `run: make test-fallback`]`.

The four targets are mutually independent as commands. `test` and `test-race`
each route their own whole-module invocation through the provisioning wrapper;
`test-fallback` is the same invocation with the DSN variable explicitly cleared
and `-count=1`; `cover-ratchet` is the ratchet script in `--check` mode, which
routes its own measurement through the same wrapper
`[measured 4a4c909:Makefile:64-68,87-88,179-180 · sed -n '64,68p;87,88p;179,180p' Makefile → `test: go run ./cmd/testpg -- go test ./...`, `test-race: go run ./cmd/testpg -- go test -race ./...`, `test-fallback: LAB_GAME_TEST_DSN= go test -count=1 ./...`, `cover-ratchet: .githooks/coverage-ratchet.sh --check`]`.
Nothing any one of them produces is read by another: in `--check` mode the
ratchet script skips the staged/dirty branch entirely and goes straight to its
own measurement, so it depends on a git work tree and `go` and on nothing a
sibling step left behind
`[measured 4a4c909:.githooks/coverage-ratchet.sh:86-116 · sed -n '86,116p' .githooks/coverage-ratchet.sh → the `staged`/`dirty` block is inside `if [ "$mode" = raise ]`, and `--check` falls through to `go run ./cmd/testpg -- go test -covermode=atomic -coverprofile="$PROFILE" ./...`]`.
That independence is what makes the split a pure restructuring rather than a
behaviour change.

### The chosen shape — four sibling jobs, each gated exactly as the one it replaces

Replace the single `test` job with four jobs that are siblings of one another
and children of `changes` only. Each carries the preamble the replaced job
carried, and each runs exactly one of the four `make` targets:

| Job key | `name:` | Runs | Why it is a CI job at all |
|---|---|---|---|
| `test` | `Test` | `make test` | the suite |
| `test-race` | `Race` | `make test-race` | the race gate |
| `cover-ratchet` | `Coverage ratchet` | `make cover-ratchet` | catches a commit made where `core.hooksPath` is unset, and re-rolls the suite's timing-dependent statements on a runner |
| `test-fallback` | `Test fallback` | `make test-fallback` | the only CI route that still reaches the per-binary container path |

**The job key is the `make` target it runs; the `name:` is that gate in the
reviewer's words.** The key half means a reader of the workflow file never has
to map a job onto a command, and it is what any future `needs:` would reference.
The `name:` half is what the PR's checks list shows, which is why subtask 5 must
carry the new names into the failure-class taxonomy that `/pr-ci-failed` and
`/main-ci-failed` key off the CI job name.

Parallelism needs no keyword, and the primary ground for that is this
repository's own CI rather than a documentation sentence. Every job of this
workflow other than `changes` declares `needs: changes` and names no sibling,
and on a run of `main` they start together and overlap: the jobs that ran
started within a second of one another, right after `changes` finished, and the
longest-running one was still in progress long after the shortest had completed
`[measured 9d7b70c · gh run view 34664604281 --json jobs → `Detect changes` completedAt 01:22:14Z; `Build`, `Test`, `Comment references`, `Harness guards`, `Format` all startedAt 01:22:16Z and `Lint` startedAt 01:22:17Z, while `Format` completedAt 01:22:46Z and `Test` completedAt 01:27:13Z]`
`[measured 9d7b70c:.github/workflows/ci.yml · grep -n '^  [a-z-]*:$\|^    needs:\|^    name:' .github/workflows/ci.yml → every job key other than `changes` is followed by `name:` and `needs: changes`, and no `needs:` value names another job]`.
So the shape this design proposes for the four is the shape this workflow's
existing sibling jobs already run under, in this repository, on this runner
fleet. GitHub's workflow-syntax reference says the same thing in general terms —
"a workflow run is made up of one or more `jobs`, which run in parallel by
default", with `needs` the keyword that orders them
[<https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idneeds>,
fetched 2026-09-12] — and is carried as corroboration, not as the load-bearing
citation.

AC2 is therefore satisfied by each of the four naming `changes` in its `needs`
and naming no sibling — an absence, which is why § Test Design gives it a
positive control rather than trusting a clean grep, and why the PR's own run is
read at the end (§ Test Design, *Run-level verification*) as the observed form
of the same evidence.

AC3 is satisfied the same structural way: the `changes` job and its `filters:`
block are untouched, and each of the four repeats the replaced job's
`needs: changes` and `if: needs.changes.outputs.go == 'true'` verbatim. The set
of changes that starts each gate is therefore the same set that starts it now —
the `go` filter — and the verification is a before/after comparison against
`git show HEAD:.github/workflows/ci.yml`, not a reading of the new file alone.

**What AC3's "set of changes" is, and what it is not.** It is the `changes`
job's own filter set — which paths-changed condition admits a gate — and the
reading is recorded here rather than left to a per-reader inference, because
the old shape carried a **second** suppressor that was never a paths-changed
condition and that this diff deliberately removes. `ci.yml` declares no
`continue-on-error` anywhere, and every `if:` in it sits at job level, none
inside a `steps:` block
`[measured 9d7b70c:.github/workflows/ci.yml · grep -n continue-on-error .github/workflows/ci.yml → no match, exit 1; grep -n '^    if:' .github/workflows/ci.yml → every hit is a job-level key at four-space indent, none inside a steps: block]`,
so a red step ended the job and the steps below it never ran. Observed in this
repository's own run history rather than reasoned from a documented default
`[measured 9d7b70c · gh run view 34395604132 --json jobs → the `Test` job's steps read `Run make test -> failure`, then `Run make test-race -> skipped`, `Run make cover-ratchet -> skipped`, `Run make test-fallback -> skipped`]`.
After the split, a change that reddens `make test` does let the other three
execute. **That is the information the split exists to buy, not an AC3
violation.** Preserving the suppression would require `needs:` chains among the
four, which AC2 forbids outright — so a reading of AC3 that demanded it would
make AC1, AC2 and AC3 mutually unsatisfiable, and the filter-set reading is the
only one under which all three hold together. Step 9 therefore verifies AC3
against each new job's paths-changed condition and the untouched `filters:`
block, and reads the loss of the red-step suppression as the intended
consequence recorded here. AC3's wording is left exactly as the spec has it;
this is a design-side reading, not a spec amendment, because that is the option
the owner chose when the reading was routed to him
`[measured 9d7b70c, working tree (the round-3 entry is uncommitted at drafting):ai-docs/plans/2026-09-12-split-ci-test-job.spec.md.state.md · grep -n "Только дизайн" … → `answer: "Только дизайн (рекомендую)"` inside prior_qa]`.

### The comments inside `ci.yml` that assert step ordering are part of this diff

The workflow's own prose asserts the step ordering that this change removes:
the ratchet comment calls it "the LAST gate" and "this step", and the fallback
comment opens "No other step above still reaches the per-package container
path"
`[measured 4a4c909:.github/workflows/ci.yml:127-137 · sed -n '127,137p' .github/workflows/ci.yml → the comment blocks above `run: make cover-ratchet` and above `run: make test-fallback`]`.
Both must be rewritten to speak of jobs rather than of steps above them, and
both must keep their *reason* — those sentences are the only record of why
those gates are in CI at all. The rewrite stays inside the comment-reference
rule, which gates `*.yml`: no path, no section number, no issue number
`[measured 4a4c909:Makefile:182-185 · sed -n '182,185p' Makefile → "The comment reference gate, over the whole tracked gated set: no comment in a gated file carries an outward reference." above `comment-refs: go run ./cmd/commentrefs`]`.
A new comment on the cluster records why the four are four jobs and that
none of them may acquire a `needs:` on another — the only durable defence
against a future edit quietly re-merging them (§ Open questions — *Nothing gates the four-job shape*).

### Key decisions

- **KD-A — four explicit job declarations, not a `strategy.matrix`.** A matrix
  over the four target names would also produce four concurrently-scheduled
  jobs and would be shorter. It is rejected on these grounds. First, the owner's
  words are "разбить на 4 отдельные джобы" — separate jobs — and `AGENTS.md`
  § *Communication* binds the literal, conservative reading. Second, a matrix
  entry cannot carry the rationale comments where they belong: those
  sentences are attached to the gate they justify today, and the project's
  comment rule pushes prose toward the thing it describes rather than toward a
  list of names. Third, a matrix brings `fail-fast` into play: GitHub documents
  that when it is on, "if any of the jobs with `continue-on-error: false` fail,
  all jobs that are in progress or queued will be cancelled"
  [<https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/run-job-variations>,
  fetched 2026-09-12],
  so a single red gate would destroy the very information the split exists to
  expose unless the flag is explicitly turned off. Four independent jobs carry
  no such coupling and nothing has to be remembered.
- **KD-B — `.github/workflows/**` is grouped with **code** for change-type
  purposes.** The harness's enumerated classes are `*.go`/migrations and
  `*.md`/`.claude/**`/`AGENTS.md`/`ai-docs/**`; a workflow file is in neither.
  The precedent that settled this in a merged design of this repository is
  followed rather than re-argued: files in neither class that are "executable
  build plumbing rather than instructions an agent reads" go with the code
  `[measured 4a4c909:ai-docs/plans/done/2026-09-08-shared-postgres-test-server.design.md:633-636 · sed -n '633,636p' ai-docs/plans/done/2026-09-08-shared-postgres-test-server.design.md → "The files of this change that fall in **neither** enumerated class — `Makefile`, `.githooks/**` and `.github/workflows/**` — are grouped with the code"]`.
  This is what forces two groups in § Handoff plan.
- **KD-C — the failure-class taxonomy gains no class, only corrected job
  names.** Today the `test` class's *CI job* column reads `Test` and the `race`
  class's reads `Test`
  `[measured e64fd73:.claude/skills/pr-ci-failed/SKILL.md:165-166 · sed -n '165,166p' .claude/skills/pr-ci-failed/SKILL.md → "| `test` | Test | `--- FAIL:` / `FAIL	github.com/...` |" and "| `race` | Test | `WARNING: DATA RACE` under `go test -race` |"]`.
  After the split a `--- FAIL:` line can arrive from `Test`, `Race`,
  `Coverage ratchet` **or** `Test fallback` — `Race` included, because a test
  that simply fails under `go test -race` emits the `test` class's signal and
  not the `race` class's. Executed rather than reasoned about, on a throwaway
  module outside the tree
  `[measured e64fd73 · go test -race ./... on a throwaway module whose only test calls t.Fatal → the log carries "--- FAIL: TestPlainFailure" and a "FAIL" summary line, and grep -c "WARNING: DATA RACE" over that log returns 0]`.
  So the `test` class's job column must list `Race` or the shipped table tells a
  `/pr-ci-failed` run that a `--- FAIL:` cannot come from `Race`. The `race`
  class's column stays `Race` alone: that signal has exactly one origin, because
  `Race` is the only one of the four jobs whose target passes `-race`
  — `[derived → AC1; § Approach's job table]`. The *CI job* column of both CI skills' classification tables is
  therefore falsified and must be corrected on both rows. The *class* column is
  not: introducing a `cover-ratchet` class would be new scope the task does not
  ask for, and the gap it would fill is pre-existing (§ Open questions — *no class for a coverage-ratchet verdict*).
- **KD-D — `ai-docs/context-status.md` IS edited; the exclusion argued in
  round 1 does not survive its own measurements.** The file's header calls it
  "the detailed, append-only implementation log", and round 1 read that as
  putting it in the genre of `ai-docs/learnings.md` and `ai-docs/plans/done/**`.
  Three measurements refute that reading. (a) The Propagation Rule's step 4
  enumerates its history surfaces and this file is not among them
  `[measured e64fd73:AGENTS.md:288 · sed -n '288p' AGENTS.md → "Completeness test: every LIVE doc must agree; history surfaces (`ai-docs/learnings.md`, `ai-docs/plans/done/**`) are left untouched."]`.
  (b) An in-place correction of an existing entry's body in this file is
  established practice, not an exception
  `[measured e64fd73:ai-docs/context-status.md · git show --stat 55682cc → subject "docs: correct the test-cache claim in three live documents", touching `ai-docs/context-status.md` alongside `AGENTS.md` and `Makefile`; `git show 55682cc -- ai-docs/context-status.md` is a `-`/`+` rewrite of an existing entry's bullet]`.
  (c) `/task`'s own Step 12 mandates an in-place `Edit` of an existing line of
  this file, so "append-only" is already not literal
  `[measured e64fd73:.claude/skills/task/SKILL.md:271 · sed -n '271p' .claude/skills/task/SKILL.md → "`Edit` `ai-docs/context-status.md`, replacing the literal `#TBD-at-Step-12` in this run's entry heading with `#<N>`"]`.
  «Append-only» there governs how the log *grows* — one entry per task, entries
  are never reordered or deleted — not whether a present-tense sentence inside
  an entry may be made true again. And the sentence at issue is present-tense
  and false after this diff
  `[measured e64fd73:ai-docs/context-status.md:188 · grep -n "CI's Test job runs it as a step" ai-docs/context-status.md → "`make test-fallback` keeps the per-binary container path executed and CI's Test job runs it as a step"]`
  — squarely AC4's class. It is corrected in subtask 6, and the file stays
  **inside** the AC4 sweep's set: an exclusion decided by the same key decision
  the sweep exists to bound would make the instrument circular.
- **KD-E — the AC4 sweep is scoped by AGENTS.md step 4 alone, and the
  keep-or-fix test is falsity, not vocabulary.** Excluded from the sweep are
  exactly the surfaces step 4 names (`ai-docs/learnings.md`,
  `ai-docs/plans/done/**`), the retired interview state under
  `ai-docs/plans/ignored/**`, and this task's own spec, state, design and
  progress files. Everything else tracked is swept — `ai-docs/context-status.md`
  (KD-D), `ai-docs/harness-gaps.md` and `ai-docs/key-decisions.md` included. For
  each site the sweep names the test is a single measurable question: **is the
  sentence false once the diff lands?** False → corrected. True → left, and the
  ruling recorded. One ruling is made here rather than left to the sweep,
  because it is the case the class's wording does not settle:
  `ai-docs/key-decisions.md` KD-20 says `make test-fallback` is "run as its own
  CI step"
  `[measured e64fd73:ai-docs/key-decisions.md:53 · grep -n "run as its own CI step" ai-docs/key-decisions.md → "and `make test-fallback` — a bare whole-module run with the variable explicitly cleared, run as its own CI step — is what keeps the per-binary path executed now that no default gate reaches it"]`.
  After the split that gate still runs as a `run:` step, now the only step of
  its own job, so the sentence is **true** and names no job; it is
  **decided-and-left**, and subtask 7 does not re-decide it. The
  job-versus-step vocabulary is nonetheless added to the sweep's pattern set
  (§ Test Design), because the axis is what round 1's pattern set could not
  reach at all — an unreachable axis and a reachable-but-true site are different
  defects, and only the second one is closed by a ruling.

### Rejected alternatives

- **A `strategy.matrix`** — KD-A.
- **A composite action for the shared `checkout` + `setup-go` preamble.** The
  preamble is already repeated across the workflow's Go jobs before this change
  and would simply be repeated in more of them after it. Lifting it is a
  refactor of pre-existing duplication that the task does not ask for and that
  would change every Go job's diff; `AGENTS.md` § *Communication* makes widening
  scope an ask, not a notification. Named here so the trade-off is on the
  record.
- **Keeping `cover-ratchet` sequenced after `test`** on the theory that
  measuring coverage on a red suite is wasted. It is not wasted: the ratchet
  script already detects a non-green suite itself and exits with a named
  "coverage is not measurable" failure
  `[measured 4a4c909:.githooks/coverage-ratchet.sh:115-122 · sed -n '115,122p' .githooks/coverage-ratchet.sh → "coverage-ratchet: BLOCKED — the test suite is not green, so coverage is not measurable." followed by `exit 1`]`,
  so chaining would buy nothing and would violate AC2 outright.
- **Adding a structural guard script that asserts the four-job shape.** New
  scope; surfaced instead (§ Open questions — *Nothing gates the four-job shape*).

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Replace the `test` job with the four sibling jobs of the § Approach table: each gets the replaced job's `needs: changes`, `if: needs.changes.outputs.go == 'true'`, `runs-on`, and checkout + `setup-go` preamble, and exactly one `run: make <target>`; none gains a `needs:` on another. Rewrite the in-file comments that assert step ordering, and add the cluster comment **immediately above the first of the four job declarations**, where a reader arriving at the cluster meets it before any of the four (the two rewritten rationale comments stay attached to the gate each justifies, now inside that gate's own job). `changes` and its `filters:` block are left byte-identical. Run `actionlint .github/workflows/ci.yml` and the before/after gating comparison of § Test Design **before** `git add`. | `.github/workflows/ci.yml` | — |
| 2 | Correct the falsified claims in `AGENTS.md`: the ratchet AXIOM's "`make cover-ratchet` and CI's Test job run the identical script with `--check`" must name the job that now runs it; the § *Build & Test* gate enumeration's `Test (incl. -race)` entry must name the four jobs the split creates. Nothing else in that enumeration is re-authored. | `AGENTS.md` | 1 |
| 3 | Correct the CI job table in `ai-docs/claude-tools-hierarchy.md`: the single `Test` row becomes one row per new job, each naming its own `make` target and keeping the unchanged `go` paths-changed condition. | `ai-docs/claude-tools-hierarchy.md` | 1 |
| 4 | Correct `ai-docs/go-test-conventions.md`'s "CI runs it as a Test-job step" so it names the job that now runs `make test-fallback`; the surrounding claim about why the fallback path needs its own gate is unchanged. | `ai-docs/go-test-conventions.md` | 1 |
| 5 | Correct `.claude/skills/pr-ci-failed/SKILL.md` — the `CI exists` job enumeration, and the *CI job* column of the classification table for the `test` and `race` classes, the `test` row gaining `Race` alongside `Test`, `Coverage ratchet` and `Test fallback` per KD-C — and apply the same corrections to its declared CI sync-group siblings `.claude/skills/main-ci-failed/SKILL.md` and `.claude/skills/dependabot-pr/reference.md`, editing `dependabot-pr/reference.md` only if it carries a falsified claim and recording the no-change outcome if it does not. | `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md`, `.claude/skills/dependabot-pr/reference.md` | 1 |
| 6 | Correct the shared-test-server entry's falsified sentence in `ai-docs/context-status.md` (KD-D) so it names the job that now runs `make test-fallback` rather than asserting CI's Test job runs it as a step. The rest of that entry, every other entry, and the file's entry order are untouched — this is a claim correction inside one existing bullet, not a rewrite of the log. | `ai-docs/context-status.md` | 1 |
| 7 | Run the AC4 falsified-claim sweep of § Test Design over the live set with its positive control, after the last edit of subtasks 2–6; record the control's output and the sweep's findings. Any site the sweep names whose sentence the diff makes false and that subtasks 2–6 did not reach is fixed here; a site whose sentence stays true is left and recorded (KD-E). **A site the sweep names in a code change-type file** (`*.go`, migrations, `.github/workflows/**` per KD-B, `Makefile`, `.githooks/**`) **is never edited inside this group** — Group B is instructions/harness-homogeneous, so such a site is surfaced to the orchestrator with its line. AC4 admits no out-of-scope disposition, so the surfacing has exactly two legitimate ends, and "ruled out of scope" is not one of them: either the site's sentence is **not false** after the diff, in which case it is outside AC4's class by the same keep-or-fix test as any other site and is recorded as decided-and-left (KD-E); or it **is** false, in which case AC4 obliges its correction in this same pull request and the orchestrator must open a **third, code-homogeneous group** for it (within the default maximum of four — § Handoff plan). | (whichever instructions/harness files the sweep names) | 2, 3, 4, 5, 6 |

The CI sync group is declared, so subtask 5's sibling obligation is not a
judgement call
`[measured 4a4c909:ai-docs/propagation-groups.md:28-29 · sed -n '28,29p' ai-docs/propagation-groups.md → "`.claude/skills/pr-ci-failed/SKILL.md` | `.claude/skills/main-ci-failed/SKILL.md` AND `.claude/skills/dependabot-pr/reference.md` (CI group — the failure-class taxonomy and the per-class reproducers must agree)"]`.
And subtasks 2, 3 and 5 are **obligatory rather than discretionary**, because
the same table declares a row keyed on exactly this diff's trigger — a job
added, renamed or removed in `ci.yml` — whose target list names the CI group's
class tables, `AGENTS.md` § *Build & Test* and
`ai-docs/claude-tools-hierarchy.md`, and whose stated reason is the failure this
task would otherwise cause
`[measured 9d7b70c:ai-docs/propagation-groups.md:30 · sed -n '30p' ai-docs/propagation-groups.md → "`.github/workflows/ci.yml` (a job added, renamed, or removed) | The CI group's class tables AND `AGENTS.md` § *Build & Test* AND `ai-docs/claude-tools-hierarchy.md` — a class with no job, or a job with no class, is how a red run becomes unclassifiable"]`.
That row's test — *a class with no job, or a job with no class* — is precisely
what KD-C answers, which makes KD-C compliance with a declared row rather than a
judgement of this design's own.

Subtask 7's code-change-type branch is unlikely to fire, measured rather than
assumed: the comments in the code-type files nearest this change speak of CI
without naming the `Test` job, so the diff does not falsify them. The ratchet
script's header says the gate runs with `--check` in CI and names no job
`[measured 9d7b70c:.githooks/coverage-ratchet.sh:4-8 · sed -n '4,8p' .githooks/coverage-ratchet.sh → "The pre-commit hook runs it in raise mode, `make cover-ratchet` and CI run it with --check."]`,
and the `Makefile`'s header says CI invokes the sub-targets from its
paths-filtered jobs, which stays true when there are more of them
`[measured 9d7b70c:Makefile:4-6 · sed -n '4,6p' Makefile → "CI never runs `verify` — it invokes the same sub-targets from its paths-filtered jobs, so a local run and a CI run cannot disagree about what any gate's command is."]`.
`ci.yml`'s own comments are subtask 1's and are not the sweep's to find.

## Handoff plan

Grouping is required for **every M ≥ 1** — this section is mandatory in every
design, a single-subtask one included, whose one group is also terminal and runs
in its own `/context-reset` subagent. A group holds **up to 10** consecutive
subtasks; ten is a **maximum**, not an exact count, and a group ends at
whichever comes first: the size cap, a change-type switch, or a
dependency-forced boundary. The terminal group's size is in `1..=10`. Each group
is homogeneous by change-type — **code** (`*.go`, migrations) or
**instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`, `ai-docs/**`) —
never both; `.github/workflows/ci.yml` falls in neither enumerated class and is
grouped with the code per KD-B. Same-change-type subtasks are clustered into the
**fewest groups possible**, bounded by the size cap, by dependency order and by
homogeneity; naive interleaving is the least-desirable fallback and is not used
here. The default maximum is **4** groups per task, and more than 4 is surfaced
to the user for approval; this design defines **2**. One contingent third group
is named rather than left to be improvised: if subtask 7's sweep finds a **code**
change-type file whose sentence the diff falsifies, AC4 obliges the correction
in this pull request and Group B cannot make it without losing its homogeneity,
so the orchestrator opens a third, code-homogeneous group for exactly those
sites. Three is still inside the default maximum of four, so no user gate is
crossed; the branch is measured as unlikely to fire (§ Decomposition, after the
table).

- **Entry into Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). The
  handoff binds at the start of **every** group, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer`
  subagent, 1M-token window — subtask 1 (code change-type per KD-B:
  `.github/workflows/ci.yml`). Non-terminal; every later subtask depends on it,
  so it cannot be clustered with Group B and cannot be reordered after it.
  **This group's gates are named here, so the return summary is readable and the
  implementor reads a list rather than deriving one.** Subtask 1 edits no `*.go`
  file, so the gates that carry information are
  `actionlint .github/workflows/ci.yml`, `make comment-refs`, and the § Test
  Design extractions — gating parity (AC3), sibling independence (AC2), one gate
  per job (AC1) — each run with its own control against the pre-change tree.
  This is the list `code-writer`'s own Mode A step 3 asks for rather than an
  override of it: its gate paragraph carves out a subtask with no `*.go`,
  directing the implementor to the design's Test Design checks and naming
  `actionlint` on a changed workflow among them
  `[measured 9d7b70c:.claude/agents/code-writer.md:65 · sed -n '65p' .claude/agents/code-writer.md → "(For an instructions/harness subtask with no `*.go`, the Go gates simply stay green — run the design's Test Design checks instead: grep / `wc -c` / `actionlint` on a changed workflow / `shellcheck` on a changed script.)"]`.
  A Go gate run anyway stays green on a YAML-only diff and discharges no AC
  here; the carve-out's class label reads "instructions/harness" while KD-B
  groups this subtask with the code, and that mismatch changes nothing, because
  the condition the carve-out actually turns on is the absence of a `*.go` edit
  and the substitute check it names is the one this subtask needs.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry).
  Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the
  orchestrator (typically xHigh), 1M-token window, via the `general-purpose`
  subagent with no inline `model=` — subtasks 2–7 (instructions/harness
  change-type: `AGENTS.md`, `ai-docs/**`, `.claude/**`). Terminal group
  (6 subtasks; within the `1..=10` range). All same-change-type subtasks are
  clustered into this ONE group rather than interleaved with Group A. Subtask 7
  is the group's homogeneity boundary in practice: a site its sweep names in a
  code change-type file leaves the group as a surfaced finding rather than as an
  edit, so the group cannot become non-homogeneous by way of the sweep's
  open-ended file column.

## Risks

- **A sibling job silently stops running, because its `if:` or `needs:` drifted
  from the replaced job's.** This is the whole of AC3 and it fails silently — a
  job that does not run reports no red. Mitigation: subtask 1's gate is a
  before/after comparison against `git show HEAD:.github/workflows/ci.yml`, not
  a reading of the new file, and the `changes` job's `filters:` block is
  required to come out byte-identical — `[derived → AC3; § Test Design, "Gating
  parity"]`.
- **The four are declared concurrent but one of them acquires an ordering
  dependency later.** Nothing in the toolchain knows the four must stay
  siblings; `actionlint` validates syntax, not this invariant. Mitigation
  in-scope: the cluster comment subtask 1 adds states the constraint where the
  next editor will read it. Mitigation refused as out of scope: a guard script
  (§ Open questions — *Nothing gates the four-job shape*) — `[derived → AC2; § Test Design, "Sibling
  independence"]`.
- **Runner-minutes rise even though wall-clock falls.** Work the replaced job
  did once inside a single workspace — the checkout, the Go set-up, and any
  compilation or image layer the later steps inherited from the earlier ones —
  is done by each of the four jobs separately. The load-bearing half is local
  and structural — each of the four declares its own checkout and `setup-go`
  steps, per § Approach's job table. GitHub's runner reference corroborates that
  nothing is inherited across them: "With the exception of single-CPU runners,
  each GitHub-hosted runner is a new virtual machine (VM) hosted by GitHub"
  [<https://docs.github.com/en/actions/concepts/runners/github-hosted-runners>,
  fetched 2026-09-12] — quoted with its exception clause intact, which is why
  the structural half rather than the quote is what the row rests on.
  Each gate was already provisioning its own Postgres before the split, so that
  part of the cost does not move
  `[measured 4a4c909:Makefile:59-68 · sed -n '59,68p' Makefile → "Both route through the provisioning wrapper: it reuses an already-set DSN or an already-running long-lived server (test-db-up) unchanged, and otherwise provisions and removes its own sized, anonymous container"]`.
  This is the cost the spec's Scope item 1 explicitly buys — sum-of-four
  wall-clock traded for slowest-of-four — and it is accepted, not mitigated
  — `[derived → AC1]`.
- **A per-job wall-clock rise hides inside the run-level win.** A consequence of
  the row above: a single job may now take longer than the same step took as
  part of the old job, while the run as a whole takes less. Reading one job's
  duration would therefore read the change as a regression. Mitigation: the
  verdict is read at the run level, on the PR's own CI run, which this diff
  triggers because the `go` filter names `.github/workflows/**`
  `[measured 4a4c909:.github/workflows/ci.yml:39-48 · sed -n '39,48p' .github/workflows/ci.yml → the `go:` filter lists `'.github/workflows/**'` among its paths]`.
- **The instruction-edit guard blocks subtasks 2 and 5 if they run outside
  `/task` Steps 8–12.** A `PreToolUse` hook refuses any write to `AGENTS.md`,
  `CLAUDE.md` or `.claude/**` while a `*.spec.md.state.md` exists and
  `ai-docs/plans/.task-inflight` does not — and this branch carries such a state
  file. Mitigation: nothing; the exemption is the marker itself, which Step 8
  creates before Group A runs
  `[measured 4a4c909:.claude/settings.json:78,98 · grep -n 'task-inflight' .claude/settings.json → both instruction-edit hooks exit 0 when `[ -f ai-docs/plans/.task-inflight ]`]`.
  Recorded so a blocked write is read as "the marker is missing", not as "this
  edit is forbidden".
- **The sweep that discharges AC4 reports clean because it cannot detect
  anything.** A grep over prose fails on the encoding it was not written for —
  `Test job` against `Test-job`, a table cell `| Test |` against an enumeration
  `· Test ·`, a capital against a lower-case, and — the axis round 1's pattern
  set missed entirely — a claim written in step vocabulary rather than job
  vocabulary (`run as its own CI step`, `CI runs it as a Test-job step`), which
  no amount of varying the spelling of `Test` reaches. Mitigation, structural
  rather than enumerative: the sweep's **primary pass is over the gate names
  themselves** — the four `make` targets and the four job names — because a
  sentence in AC4's class must name a gate to make a claim about it, so the
  primary pass cannot be escaped by vocabulary; the encoding variants are a
  secondary pass over the job names, now carrying the step-vocabulary axis. The
  positive control drawn from the **pre-change** tree runs before either
  verdict is readable — `[derived → AC4; § Test Design, "AC4 falsified-claim
  sweep"]`.
- **The sweep's own scope is decided by the design it is meant to check.**
  Round 1 excluded `ai-docs/context-status.md` from the sweep set on the same
  key decision that declined to edit it, so the instrument bounding AC4's class
  was configured blind to a member of that class — and a control drawn from
  already-known sites cannot detect that kind of blindness. Mitigation: the
  exclusion set is no longer a design judgement but a quotation of the
  Propagation Rule's own step-4 enumeration plus this task's own artefacts
  (KD-E), and every keep-or-fix call inside the swept set is the single
  measurable question *is the sentence false after the diff?* —
  `[derived → AC4; § Test Design, "AC4 falsified-claim sweep"]`.
- **A corrected sentence introduces an outward reference into a gated file.**
  `ci.yml` is inside the comment-reference gate's tracked set, so a rewritten
  comment that names a path or a section number turns the `Comment references`
  job red. Mitigation: `make comment-refs` is part of subtask 1's pre-`git add`
  gate — `[derived → subtask 1's gate list]`.

## Test Design

No subtask in § Decomposition touches a `*.go` file, so this change's verifiable
artefacts are the workflow's structure, the live CI run it produces, and the
prose sweep.

**Subtask 1 — `actionlint`.** Location: the required workflow gate.
Entry point: `actionlint .github/workflows/ci.yml`, run before `git add` per the
`AGENTS.md` § *Build & Test* AXIOM. Scenario: exit 0 on the restructured file.
The binary is present on this machine — an environment fact about a tool this
task does not create, not a claim about a new artefact, so the tag is a
`measured` one carrying the session's commit pin and no path
`[measured 9d7b70c · actionlint --version → v1.7.12, built from source]`.
Expected outcome of the restructured file passing it — `[derived → AC1]`.

**Subtask 1 — gating parity (AC3).** Entry point: a comparison, not a read.
Capture the replaced job's gating from the pre-change tree
(`git show HEAD:.github/workflows/ci.yml`) and require that each of the four new
jobs reproduces the same `needs:` value and the same `if:` expression, and that
the `changes` job's `filters:` block is unchanged between the two versions.
Fixture: the pre-change file itself — the control is that the extraction, run
against the pre-change tree, must yield the replaced job's gating pair; an
extractor that yields nothing there is broken and its clean answer on the new
file means nothing. Edge case the comparison must catch: a job that acquired
`if: always()`, or one whose `if:` lost the `needs.changes.outputs.go` term.
**What this comparison deliberately does not check** is whether a red gate still
suppresses the other three — that suppression was a step-ordering artefact, not
a paths-changed condition, and removing it is what the split is for, so a
verifier reading AC3 as requiring it would fail the criterion for delivering the
change the spec asked for (§ Approach — *What AC3's "set of changes" is, and what
it is not*) — `[derived → AC3]`.

**Subtask 1 — sibling independence (AC2).** Entry point: the `needs:` value of
each of the four. Scenario: each names `changes` and names no other of the four.
Because the assertion is an absence, it is reportable only after a positive
control — run the same extraction against a constructed job block that *does*
declare a sibling dependency and require it to be reported. Edge case: a
`needs:` written in list form rather than scalar form must be read the same way
— `[derived → AC2]`.

**Subtask 1 — one gate per job (AC1).** Entry point: the `run:` lines of the
four jobs. Scenario: each of `make test`, `make test-race`, `make cover-ratchet`
and `make test-fallback` appears under exactly one job, and no job carries more
than one of them. Control: the same extraction over the pre-change file must
report all four under a single job — if it does not, the extractor is wrong
before it is ever pointed at the new file — `[derived → AC1]`.

**Subtask 1 — the comment gate.** Entry point: `make comment-refs`. Scenario:
the rewritten and added `ci.yml` comments carry no path, section number, URL or
issue number. Run before `git add`, alongside `actionlint` —
`[derived → subtask 1's gate list]`.

**Subtask 7 — AC4 falsified-claim sweep.** Location: the tracked live set —
`git ls-files` minus exactly what KD-E excludes: the surfaces the Propagation
Rule's step 4 names (`ai-docs/learnings.md`, `ai-docs/plans/done/**`), the
retired interview state under `ai-docs/plans/ignored/**`, and this task's own
spec, state, design and progress files. `ai-docs/context-status.md`,
`ai-docs/harness-gaps.md` and `ai-docs/key-decisions.md` are **in** the set.

Entry point — **two passes, the first of which vocabulary cannot escape.**

*Pass 1, over the gates themselves.* Scan the set, case-insensitively, for the
four target names (`make test`, `make test-race`, `make cover-ratchet`,
`make test-fallback`, and the bare `test-race` / `cover-ratchet` /
`test-fallback` forms) and the four job names the split creates, then **read
each hit's sentence** and ask the single keep-or-fix question of KD-E: is this
sentence false once the diff lands? This pass is the one that bounds the class,
because a sentence claiming where in CI a gate runs must name that gate; it
reaches a claim written in step vocabulary, in job vocabulary, or in neither.

*Pass 2, over the job-name encodings.* The same scan restricted to the shapes the
class takes in this tree, kept as a cross-check on pass 1's sentence-reading:
the possessive prose form (`CI's Test job`), the hyphenated form (`Test-job`),
the bare form (`Test job` / `job Test`), the table-cell form (`| Test |`), the
middle-dot job enumeration (`Format · Build · Test · …`), and — the axis round 1
lacked — the **job-versus-step vocabulary** (`CI step`, `run as its own CI
step`, `runs it as a step`, `as a Test-job step`, `CI's <Job>`).

Fixtures: none needed beyond the tree. **Positive control, mandatory before
either pass's verdict is readable:** run the pattern set against the
**pre-change** tree via `git show HEAD:<path>`, requiring a printed line from at
least one site per encoding axis — the possessive/job axis from `AGENTS.md`, the
hyphenated + step axis from `ai-docs/go-test-conventions.md`, the table-cell
axis from `.claude/skills/pr-ci-failed/SKILL.md`, and the step-vocabulary axis
from `ai-docs/context-status.md` and `ai-docs/key-decisions.md`. An axis whose
pattern prints nothing on the pre-change tree is evidence about that pattern,
not about the tree, and is rewritten before the sweep is re-run. The control is
a liveness check on the instrument, **not** a completeness argument: completeness
rests on pass 1, whose input is the gate names rather than a guess at how a
sentence phrases its claim.

Scenarios: (happy) both passes name no live site whose sentence the diff
falsifies; (keep) a pass names a site whose sentence stays true — recorded with
its line and left, as KD-E rules for `ai-docs/key-decisions.md` KD-20; (failure)
a pass names a false site, which is fixed here if it is an instructions/harness
file and **surfaced to the orchestrator for a third, code-homogeneous group** if
it is a code change-type file — never ruled out of scope, because AC4 admits no
such disposition (§ Decomposition, subtask 7); (instrument failure) the control
prints nothing on an axis, in which case nothing about AC4 has been established
on that axis.

The sites known at drafting — the ratchet AXIOM and the gate enumeration in
`AGENTS.md`, the CI job table row in `ai-docs/claude-tools-hierarchy.md`, the
fallback paragraph in `ai-docs/go-test-conventions.md`, the `CI exists` line plus
the classification table in each of `.claude/skills/pr-ci-failed/SKILL.md` and
`.claude/skills/main-ci-failed/SKILL.md`, and the shared-test-server entry's
sentence in `ai-docs/context-status.md` — illustrate the class; the sweep bounds
it — `[derived → AC4]`.

**Run-level verification (AC2, observed rather than asserted).** The PR's own CI
run is the final instrument: this diff touches `.github/workflows/**`, which the
`go` filter names, so all four new jobs run on it. Read the run's job list and
confirm the four started without waiting on one another, and read the run's
duration against the sum of the four jobs' durations. This is evidence, not a
gate — CI is advisory at the merge button on this repository — and it is read at
the run level per the § Risks row *A per-job wall-clock rise hides inside the run-level win* — `[derived → AC2]`.

## Open questions

- **No `SPEC-REMIT` row.** Checked deliberately. Scope item 1 and AC1–AC3 state
  the outcome the owner named in his own words ("4 отдельные джобы,
  выполняющиеся параллельно") and prescribe no mechanism, no file set and no
  placement — the job names, the job keys, the comment rewrites and the
  rejection of a matrix are all decided here, not there. AC4 cites
  `AGENTS.md` § *Propagation Rule* step 4 to **define the class** of documents
  that must end up correct; it names no file and no edit, so it is a criterion
  rather than a restated standing rule, and it is obeyed as written. Recorded so
  the absence of a flag is visible as a decision rather than as an omission.
- **Pre-existing: both CI skills' `CI exists` enumeration omits the
  `Comment references` job**, which `AGENTS.md`'s own enumeration includes
  `[measured 4a4c909:.claude/skills/pr-ci-failed/SKILL.md:8 · sed -n '8p' .claude/skills/pr-ci-failed/SKILL.md → "Format · Build · Test · Lint · Harness guards · Actionlint, each `paths-filter`-gated"]`.
  This diff does not falsify that omission, so subtask 5 corrects only the
  `Test` entry and leaves the omission standing. Widening the edit would be
  scope drift, and `AGENTS.md` § *Communication* makes that an ask. Surfaced for
  the orchestrator to route.
- **Pre-existing: the failure-class taxonomy has no class for a coverage-ratchet
  verdict.** A "coverage fell past the tolerance" failure and a `test-fallback`
  provisioning failure both land in `test` or `other` today and still will after
  the split (KD-C). Naming them would be a genuine improvement and is not this
  task's. Surfaced.
- **Nothing gates the four-job shape.** `actionlint` checks syntax; no guard
  asserts that the four gates stay in four sibling jobs, so a future edit could
  re-merge them and only the cluster comment would object. A guard script under
  `ai-docs/scripts/` with its own regression suite — the shape every other
  structural rule in this repository has — is the obvious follow-up and is
  outside the task's scope. Surfaced.
