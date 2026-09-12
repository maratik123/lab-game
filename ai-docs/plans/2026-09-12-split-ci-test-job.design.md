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

Parallelism needs no keyword. GitHub's workflow-syntax reference states that "a
workflow run is made up of one or more `jobs`, which **run in parallel by
default**", and `needs` is the only thing that orders them
(<https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idneeds>).
So AC2 is satisfied by each of the four naming `changes` in its `needs` and
naming no sibling — an absence, which is why § Test Design gives it a positive
control rather than trusting a clean grep.

AC3 is satisfied the same structural way: the `changes` job and its `filters:`
block are untouched, and each of the four repeats the replaced job's
`needs: changes` and `if: needs.changes.outputs.go == 'true'` verbatim. The set
of changes that starts each gate is therefore the same set that starts it now —
the `go` filter — and the verification is a before/after comparison against
`git show HEAD:.github/workflows/ci.yml`, not a reading of the new file alone.

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
  (<https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/run-job-variations>),
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
  names.** After the split a `--- FAIL:` line can arrive from `Test`,
  `Coverage ratchet` or `Test fallback`, and a `WARNING: DATA RACE` from `Race`.
  The *CI job* column of both CI skills' classification tables is therefore
  falsified and must be corrected. The *class* column is not: introducing a
  `cover-ratchet` class would be new scope the task does not ask for, and the
  gap it would fill is pre-existing (§ Open questions — *no class for a coverage-ratchet verdict*).
- **KD-D — `ai-docs/context-status.md` is not edited.** Its own header declares
  it "the detailed, append-only implementation log: one entry per completed
  task", written at `/task` Step 9.5
  `[measured 4a4c909:ai-docs/context-status.md:3 · sed -n '3p' ai-docs/context-status.md → "The detailed, append-only implementation log: one entry per completed task, capturing the design decisions, traps and invariants worth not rediscovering. Written by `/task` Step 9.5"]`.
  Its sentence "`make test-fallback` … CI's Test job runs it as a step" is a
  dated record of what landed at an earlier PR, in the same genre as
  `ai-docs/learnings.md` and `ai-docs/plans/done/**`, which the Propagation
  Rule's step 4 leaves untouched. The log stays current by *gaining* this task's
  own Step-9.5 entry, not by having an old one rewritten.

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
| 1 | Replace the `test` job with the four sibling jobs of the § Approach table: each gets the replaced job's `needs: changes`, `if: needs.changes.outputs.go == 'true'`, `runs-on`, and checkout + `setup-go` preamble, and exactly one `run: make <target>`; none gains a `needs:` on another. Rewrite the in-file comments that assert step ordering and add the cluster comment. `changes` and its `filters:` block are left byte-identical. Run `actionlint .github/workflows/ci.yml` and the before/after gating comparison of § Test Design **before** `git add`. | `.github/workflows/ci.yml` | — |
| 2 | Correct the falsified claims in `AGENTS.md`: the ratchet AXIOM's "`make cover-ratchet` and CI's Test job run the identical script with `--check`" must name the job that now runs it; the § *Build & Test* gate enumeration's `Test (incl. -race)` entry must name the four jobs the split creates. Nothing else in that enumeration is re-authored. | `AGENTS.md` | 1 |
| 3 | Correct the CI job table in `ai-docs/claude-tools-hierarchy.md`: the single `Test` row becomes one row per new job, each naming its own `make` target and keeping the unchanged `go` paths-changed condition. | `ai-docs/claude-tools-hierarchy.md` | 1 |
| 4 | Correct `ai-docs/go-test-conventions.md`'s "CI runs it as a Test-job step" so it names the job that now runs `make test-fallback`; the surrounding claim about why the fallback path needs its own gate is unchanged. | `ai-docs/go-test-conventions.md` | 1 |
| 5 | Correct `.claude/skills/pr-ci-failed/SKILL.md` — the `CI exists` job enumeration, and the *CI job* column of the classification table for the `test` and `race` classes — and apply the same corrections to its declared CI sync-group siblings `.claude/skills/main-ci-failed/SKILL.md` and `.claude/skills/dependabot-pr/reference.md`, editing `dependabot-pr/reference.md` only if it carries a falsified claim and recording the no-change outcome if it does not. | `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md`, `.claude/skills/dependabot-pr/reference.md` | 1 |
| 6 | Run the AC4 falsified-claim sweep of § Test Design over the tracked live set with its positive control, after the last edit of subtasks 2–5; record the control's output and the sweep's findings. Any site the sweep names that subtasks 2–5 did not reach is fixed here. | (whichever files the sweep names) | 2, 3, 4, 5 |

The CI sync group is declared, so subtask 5's sibling obligation is not a
judgement call
`[measured 4a4c909:ai-docs/propagation-groups.md:28-29 · sed -n '28,29p' ai-docs/propagation-groups.md → "`.claude/skills/pr-ci-failed/SKILL.md` | `.claude/skills/main-ci-failed/SKILL.md` AND `.claude/skills/dependabot-pr/reference.md` (CI group — the failure-class taxonomy and the per-class reproducers must agree)"]`.

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
to the user for approval; this design defines **2**.

- **Entry into Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). The
  handoff binds at the start of **every** group, the first included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer`
  subagent, 1M-token window — subtask 1 (code change-type per KD-B:
  `.github/workflows/ci.yml`). Non-terminal; every later subtask depends on it,
  so it cannot be clustered with Group B and cannot be reordered after it.
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry).
  Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the
  orchestrator (typically xHigh), 1M-token window, via the `general-purpose`
  subagent with no inline `model=` — subtasks 2–6 (instructions/harness
  change-type: `AGENTS.md`, `ai-docs/**`, `.claude/**`). Terminal group
  (5 subtasks; within the `1..=10` range). All same-change-type subtasks are
  clustered into this ONE group rather than interleaved with Group A.

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
  is done by each of the four jobs separately, because each declares its own
  checkout and set-up steps and GitHub's runner reference states that "each
  GitHub-hosted runner is a new virtual machine (VM) hosted by GitHub"
  (<https://docs.github.com/en/actions/concepts/runners/github-hosted-runners>).
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
  `· Test ·`, a capital against a lower-case. Mitigation: the sweep runs a
  positive control drawn from the **pre-change** tree before its verdict is
  readable, and varies the encoding — `[derived → AC4; § Test Design, "AC4
  falsified-claim sweep"]`.
- **A corrected sentence introduces an outward reference into a gated file.**
  `ci.yml` is inside the comment-reference gate's tracked set, so a rewritten
  comment that names a path or a section number turns the `Comment references`
  job red. Mitigation: `make comment-refs` is part of subtask 1's pre-`git add`
  gate — `[derived → subtask 1's gate list]`.

## Test Design

No subtask in § Decomposition touches a `*.go` file, so this change's verifiable
artefacts are the workflow's structure, the live CI run it produces, and the
prose sweep. Every claim below is about something this task creates.

**Subtask 1 — `actionlint`.** Location: the required workflow gate.
Entry point: `actionlint .github/workflows/ci.yml`, run before `git add` per the
`AGENTS.md` § *Build & Test* AXIOM. Scenario: exit 0 on the restructured file.
The binary is present on this machine — an environment fact, so the tag carries
the session's commit pin and no path
`[measured 4a4c909 · actionlint --version → v1.7.12]`.
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
`if: always()`, or one whose `if:` lost the `needs.changes.outputs.go` term —
`[derived → AC3]`.

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

**Subtask 6 — AC4 falsified-claim sweep.** Location: the tracked live set —
`git ls-files` minus the history surfaces (`ai-docs/learnings.md`,
`ai-docs/harness-gaps.md`, `ai-docs/context-status.md` per KD-D,
`ai-docs/plans/done/**`, `ai-docs/plans/ignored/**`) and minus this task's own
spec, state and design files. Entry point: a case-insensitive scan for the
claim class — *a site stating which CI job runs any of the four gates* — run in
each of the encodings the class actually takes in this tree: the possessive
prose form (`CI's Test job`), the hyphenated form (`Test-job`), the bare form
(`Test job` / `job Test`), the table-cell form (`| Test |`), and the
middle-dot job enumeration (`Format · Build · Test · …`). Fixtures: none needed
beyond the tree. **Positive control, mandatory before the verdict is readable:**
run the same pattern set against the **pre-change** tree via
`git show HEAD:<path>` for `AGENTS.md`, `.claude/skills/pr-ci-failed/SKILL.md`
and `ai-docs/go-test-conventions.md`, and require each to print its falsified
line; a sweep whose control prints nothing is evidence about the pattern, not
about the tree. Scenarios: (happy) the post-edit sweep names no live site;
(failure) it names one, which is fixed in subtask 6; (instrument failure) the
control prints nothing, in which case nothing about AC4 has been established and
the pattern set is rewritten before the sweep is re-run. The sites known at
drafting — the ratchet AXIOM and the gate enumeration in `AGENTS.md`, the CI job
table row in `ai-docs/claude-tools-hierarchy.md`, the fallback paragraph in
`ai-docs/go-test-conventions.md`, and the `CI exists` line plus the
classification table in each of `.claude/skills/pr-ci-failed/SKILL.md` and
`.claude/skills/main-ci-failed/SKILL.md` — illustrate the class; the sweep
bounds it — `[derived → AC4]`.

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
