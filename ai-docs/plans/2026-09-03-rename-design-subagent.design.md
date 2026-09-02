# Design: Rename the `design` Subagent

**Issue:** #10
**Date:** 2026-09-03
**Round:** 2 — revised against the amended spec (AC4 gained AC3's history-surface exclusion) and
design-review rounds 1–2.

## Approach

The task has two independent halves that share one PR: a **rename** (`design` → `design-writer`,
plus every live site whose claim the rename falsifies) and a **rule rewrite** (Checklist O's
severity rule collapses to *one rule, any axis is `major`*). Both are prose-only edits under
`.claude/**` and `ai-docs/**`; nothing under `cmd/**`, `internal/**` or `docs/**` is touched
`[derived → AC13]`.

### Why per-site judgement, not a blanket substitution

The spec's § Scope defines the change set by **membership criterion**, not enumeration, and the
token `design` carries every referent that section's second table places OUT of the class — the
design *document* (`*.design.md`, "the design"), the design *phase* ("`design` rounds", "the
design phase"), the sibling agent `design-review`, the game-design corpus (`docs/DESIGN.md`), and
ordinary English ("prose by design", "a design error"). Those referents are not hypothetical — the
word lives in the harness corpus and in the Go packages alike, and the occurrences outside the
harness cannot be in the class at all
`[measured 08271a3 · grep -lwi design .claude/skills/task/reference.md AGENTS.md
ai-docs/doc-convention.md .golangci.yml cmd/bot/main.go internal/store/store.go → every one of
those paths echoed back]`. A `sed`-style substitution corrupts them.
Rejected for that reason; the alternative kept is a per-file read with a recorded per-site verdict,
closed by a whole-tree re-sweep as the last subtask.

**Site inventory, and why it takes two sweeps rather than one.** A shape-directed pass narrows the
word sweep to the files whose `design` token is likely to denote the Subagent
`[measured 08271a3 · grep -lEi '(`design`|design Subagent|subagent_type="design"|agents/design\.md|name: design|spawn .?design)' over git ls-files minus docs/, ai-docs/learnings.md,
ai-docs/harness-gaps.md, ai-docs/plans/done/, ai-docs/plans/2026-09-03-rename* and ai-docs/metrics/
→ .claude/agents/design-review.md, .claude/agents/design.md, .claude/agents/self-reflect.md,
.claude/agents/self-review.md, .claude/agents/spec-writer.md, .claude/skills/context-reset/SKILL.md,
.claude/skills/interview/SKILL.md, .claude/skills/main-ci-failed/{SKILL.md,reference.md},
.claude/skills/pr-ci-failed/{SKILL.md,reference.md}, .claude/skills/pr-commented/{SKILL.md,reference.md},
.claude/skills/task/{SKILL.md,reference.md}, ai-docs/claude-tools-hierarchy.md,
ai-docs/improve-eval-contract.md, ai-docs/propagation-groups.md, ai-docs/task-run-schema.md]`.

That pass is **necessary but not sufficient in either direction**, which is why § Decomposition is
built from the word sweep with the shape pass as a filter, never from the shape pass alone:

- It **over-reaches**: `ai-docs/task-run-schema.md` is in its output and is OUT of the class — its
  token is the design *round*, which the spec's second table names explicitly
  `[measured 08271a3:ai-docs/task-run-schema.md:335 · sed -n '335p' → "rounds, `design` rounds and
  `design-review` rounds appear nowhere in the record"]`.
- It **under-reaches**: `.claude/agents/self-improve.md` is absent from its output yet carries a
  genuine IN-class site — a closed enumeration of agent file stems, which the shape regex cannot
  match because the stem appears bare in a fenced list
  `[measured 08271a3:.claude/agents/self-improve.md:69 · sed -n '69p' → "  self-improve, design,
  design-review, review-findings, self-review,"]`. Subtask 3 owns it.

**Deliberately not touched, so it is not re-raised each round.** `ai-docs/harness-gaps.md` carries
`target:` fields naming `.claude/agents/design.md`, a path this task deletes
`[measured 08271a3:ai-docs/harness-gaps.md:110,117 · grep -n -F '.claude/agents/design.md'
ai-docs/harness-gaps.md → both lines are `**target:**` fields of parked harness diagnoses]`. They
stay as they are: the spec's § Out of scope names the file as an append-only history surface, and
AC3's exclusion clause covers it. A `target:` field records where a *diagnosis was aimed when it was
written*; rewriting it would edit history, which is the same reasoning that keeps `learnings.md`,
`ai-docs/plans/done/**` and `ai-docs/metrics/task-runs.jsonl` untouched. The next `/improve` that
acts on either row resolves the path then.

Because the class is open by the spec's own statement, § Decomposition's file list is the
**starting** set, not the boundary — subtask 8 re-derives it against the finished tree
(§ Risks R2). Neither `AGENTS.md` nor `CLAUDE.md` is in it: every `design` token in `AGENTS.md`
resolves to `docs/DESIGN.md`, to the design document, or to ordinary English
`[measured 08271a3:AGENTS.md · grep -nwi design AGENTS.md → docs/DESIGN.md references, "Thin by
design", "a design risk row", "design-blocking STOP", "pure function by design"]`.

Two judgement calls made here so they are not re-litigated per site:

- **`### Step 6: Design Subagent`** in `.claude/skills/task/SKILL.md` names the agent, not the
  phase, so it is IN the class. Nothing links to its anchor — not from anywhere, and not into that
  file at all
  `[measured 08271a3 · grep -rnE '\]\([^)]*#step-6' --include='*.md' . → empty, and
  grep -rnE '\]\([^)]*task/SKILL\.md#' --include='*.md' . → empty]`, so renaming the heading breaks
  no cross-reference. (The probe is the markdown-link form on purpose: a bare-substring sweep also
  matches this design's own prose and the `spec_path:` YAML value in the run's `.state.md`, neither
  of which is a link.)
- **`Designer Subagent.`**, the lead sentence of the renamed file
  `[measured 08271a3:.claude/agents/design.md:9 · sed -n '9p' → "Designer Subagent. Receives a
  task description…"]`, is a **role noun**, not the registered name — it stays as prose. The H1
  above it is the site that changes, because the directory's convention is the Title-Cased
  registered name plus "Subagent"
  `[measured 08271a3:.claude/agents/*.md · awk 'FNR<12 && /^# /' .claude/agents/*.md →
  "# Code-Writer Subagent" / "# Design Subagent" / "# Design Review Subagent" / "# Learnings
  Escalation Audit" / "# Review Findings Subagent" / "# Self-Improve Subagent" / "# Self-Reflect
  Subagent" / "# Self-Review Agent" / "# Spec Writer Subagent" / "# Triage Runner Agent"]` — so the
  renamed file's H1 becomes `# Design-Writer Subagent`, matching its closest sibling `code-writer`.
  (`FNR`, not `NR`: the cumulative counter would stop the sweep inside the first file of the glob.)

### Why the Checklist O rewrite carries a worked example

The spec leaves this open (§ Open questions, item 1). Decision: **include one**, naming the
`design` → `design-writer` case. Two grounds. First, the rewritten `:152` justification is
exactly the paragraph shape Checklist M sub-check 6 triggers on — a Pattern 2 fail-loud verb plus
one of the stronger contrast markers `instead` / `wrong` / `correct` / `forbidden`, which demands
a fenced block or a two-column table within eight lines or the paragraph flags
`[measured 08271a3:.claude/skills/ai-audit/checklist-m.md:15 · sed -n '15p' → "trigger iff the
paragraph contains BOTH (a) a Pattern 2 fail-loud verb AND (b) one of the stronger contrast
markers `instead` / `wrong` / `correct` / `forbidden`… no example follows, flag the paragraph"]`.
Second, the example is the evidence a future auditor needs for *why* the carve-out went, which is
the failure the spec's own § Source conflicts identifies. Checklist M sub-check 2 caps bold-uppercase verbs at one
per non-table paragraph, so the rewrite is written to that cap
`[measured 08271a3:.claude/skills/ai-audit/checklist-m.md:11 · sed -n '11p' → "Pattern 2
(fail-loud verbs) — at most one bold-uppercase verb per paragraph"]`.

The new rule is written as **prose plus the existing trigger table**, not as a new `> **AXIOM —`
blockquote: Checklist M sub-check 1 requires an action table inside every AXIOM blockquote, and
the verdict already has one — the trigger table that ends the section
`[measured 08271a3:.claude/skills/ai-audit/reference.md:180-184 · sed -n '180,184p' → the
`| Trigger | Action |` table whose last row rates a non-empty intersection `major`]`.

### Where the rationale lives

The spec leaves this open too (§ Open questions, item 2). Decision: **both surfaces, different
jobs, no duplication.** AC10 already forces the reader/model-confusion *argument* into Checklist O
itself. `ai-docs/key-decisions.md` gains a KD row that records the *decision* — its stated purpose
is "so a later reader does not re-litigate a settled trade-off"
`[measured 08271a3:ai-docs/key-decisions.md:3 · sed -n '3p' → "Decisions with the reasoning that
produced them, so a later reader does not re-litigate a settled trade-off"]` — and points at
Checklist O for the argument rather than restating it. It lands in a **new dated section** after
the last existing KD, because the KD numbering ascends in reading order and the trailing section
is `## Ledger core (2026-09-02)`
`[measured 08271a3:ai-docs/key-decisions.md · grep -n '^## ' → "## Stack", "## Infrastructure",
"## Repository and harness", "## Ledger core (2026-09-02)"]`; appending under
`## Repository and harness` would put a higher number above a lower one.

### The Audit sync group — the obligation subtask 6 incurs, and how it is discharged

Subtask 6 edits `.claude/skills/ai-audit/reference.md`, which is a declared member of the **Audit
group**
`[measured 08271a3:ai-docs/propagation-groups.md:24 · sed -n '24p' → "| `.claude/skills/ai-audit/SKILL.md` | `.claude/skills/ai-audit/reference.md` AND `checklist-m.md` AND
`.claude/agents/learnings-escalation-audit.md` (Audit group) |"]`. `AGENTS.md` § *Propagation Rule*
makes the obligation membership-based, not direction-based, so editing the reference page obliges a
sweep of the other three. The spec's § Key decisions assigned this question to the design
("Whether Checklist O's index row in `.claude/skills/ai-audit/SKILL.md` changes … Design decides").

**Decision: discharged by inspection, no sibling edit** — and the sweep is recorded here because the
Propagation Rule requires a recorded one, not a silent one. All three siblings were read for any
statement of a clash's severity or of an axis distinction, and none makes one
`[measured 08271a3 · grep -niE 'checklist o|embedded[- ]name|clash|cross-axis|same-axis'
.claude/skills/ai-audit/SKILL.md .claude/skills/ai-audit/checklist-m.md
.claude/agents/learnings-escalation-audit.md → a single hit, SKILL.md:116; zero hits in
checklist-m.md and in learnings-escalation-audit.md]`. That one hit is Checklist O's index row, and
it states the invariant and the `inconclusive` AXIOM while naming **no** severity and **no** axis
`[measured 08271a3:.claude/skills/ai-audit/SKILL.md:116 · sed -n '116p' → "| O | Embedded-name
clash scan — project-defined Subagent / Skill / Hook-event names MUST NOT clash with the harness's
embedded names (read from the session's agent-type + skill listings, NOT from any file in this
repo; an empty embedded list is `inconclusive`, never `pass`) | …"]`. Nothing the Q2 verdict changes
is asserted there, so the row is left byte-identical; subtask 6 must **re-run this same grep after
its edit** and escalate to the orchestrator if the rewrite has made any sibling's text false.

`checklist-m.md` is in the group for a second reason: it is the audit that will judge the rewritten
prose (§ *Why the Checklist O rewrite carries a worked example*, § Risks R7). Being *governed by* a
sibling is not the same as *needing an edit to* it, and neither of its Pattern rules changes here.

**The other groups subtasks 3–5 trigger, inspected on the same terms.** The Audit group is not the
only membership this diff touches, and the recorded-sweep obligation is per group, not per design.
Each row below was resolved against `ai-docs/propagation-groups.md` and each named sibling read for
a `design` token denoting the Subagent; **every one is OUT of the class, so no sibling edit is
needed** — recorded here because Step-10 self-review applies the same rule and looks for the record,
and because an unrecorded OUT is indistinguishable from an unexamined one.

| Row / group | Siblings reached | Verdict |
|---|---|---|
| **Review** (`self-review.md` ↔ `review-findings.md` AND `project-review/SKILL.md`) | `review-findings.md`, `project-review/SKILL.md` | Tokens are `*.design.md`, "Design conformance", "API design", `docs/DESIGN.md`, "no spec/design" — OUT; no edit |
| **Reflect** (`reflect/SKILL.md` ↔ `self-reflect.md`) | `reflect/SKILL.md` | One token, "needs its own enforcement design" — OUT; no edit |
| **Improve** — the row that fires is `self-improve.md` → `improve/SKILL.md` AND `improve-eval-contract.md`, since subtask 3 edits `self-improve.md` | `improve/SKILL.md` | One token, "design's `AC<N> verified by:` lines" — the design *document* — OUT; no edit. (`improve-eval-contract.md` is IN class and is subtask 5's.) |
| **CI** (`pr-ci-failed/SKILL.md` → `main-ci-failed/SKILL.md` AND `dependabot-pr/reference.md`) | `dependabot-pr/reference.md` | One token, "Round 1 of the design considered" — OUT; no edit. (`main-ci-failed/SKILL.md` is IN class and is subtask 4's.) |
| **Domain-invariant row** (a domain rule → `domain-invariants.md` AND `self-review.md` § 4a AND `review-findings.md` § 1a AND the renamed agent's § Rules) | `domain-invariants.md` | The row's *trigger* is "a domain-invariant rule changed", and subtask 1 changes none — it touches frontmatter, the H1 and one agent list. Obligation vacuous; `domain-invariants.md`'s tokens are `docs/DESIGN.md` references — OUT; no edit. The row's own text names the agent by path, which is why it is a site in subtask 5. |

Measured for the whole table
`[measured 08271a3 · grep -nwi design .claude/agents/review-findings.md
.claude/skills/project-review/SKILL.md .claude/skills/reflect/SKILL.md
.claude/skills/improve/SKILL.md .claude/skills/dependabot-pr/reference.md
ai-docs/domain-invariants.md → every hit is a `docs/DESIGN.md` reference, a `*.design.md` /
"design document" reference, a phase or round name, or ordinary English]`
`[measured 08271a3:ai-docs/propagation-groups.md:7-9,15,17-20,27-28 ·
sed -n '7,9p;15p;17,20p;27,28p' → the Review, domain-invariant, Reflect, Improve and CI rows quoted
above]`.

### Rejected alternatives

- **Blanket `sed`/`rg -r` substitution over the tree.** Rejected: corrupts the OUT-of-class
  referents above, and a mutating flag that rewrites output while exiting 0 is precisely the
  silent-success shape `AGENTS.md` § *Build & Test* warns against
  `[measured 08271a3:AGENTS.md:75 · grep -n 'rg -r' AGENTS.md → "the same silent-success shape
  covers a `jq` filter printing `null` from an error body, and a mutating flag (`rg -r`) rewriting
  output while exiting 0"]`.
- **Renaming only `.claude/agents/design.md` and leaving Checklist O alone.** Rejected by the
  spec: the rule that waved this clash through would stand for the next collision.
- **Adding `Bash(bash *)` / `Bash(python3 *)` to `.claude/settings.json` `permissions.allow`** so
  the citation guard and the link check run locally. Rejected — see § Risks R5. The grants are
  broad (arbitrary script execution) and unrelated to a rename; the gates in question already run
  on this diff in CI.
- **Amending the spec so AC3's glob excludes the in-flight plan artefacts.** Rejected as
  unnecessary — see § Risks R4; the terminal tree satisfies AC3 as written.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | `git mv` the definition to `.claude/agents/design-writer.md`; set frontmatter `name: design-writer`; retitle the H1 to `# Design-Writer Subagent`; update the self-referential agent list in its § Rules sub-point (g). Keep the content delta small so the commit records a rename (§ Risks R1). | `.claude/agents/design.md` → `.claude/agents/design-writer.md` | — |
| 2 | Update the **Task/Design sync group**. Not one passage-kind but several: the dispatch examples and the Step-6 heading and body; the Design-Amendment prose and its anti-pattern table rows; the handoff-trigger prose; the **quality-gate enumerations** that list this agent beside `design-review` / `self-review` / `spec-writer` (`context-reset/SKILL.md`'s group-spawn rule, `task/SKILL.md`'s every-group-handoff item, `task/reference.md`'s per-group-implementor note); the coordinate-drift rows that name it as the owner of `*.design.md`; and `design-review`'s own frontmatter description plus its checklist cross-references. Sweep the file, do not stop at the recipe. | `.claude/skills/task/SKILL.md`, `.claude/skills/task/reference.md`, `.claude/agents/design-review.md`, `.claude/skills/context-reset/SKILL.md` | 1 |
| 3 | Update the remaining Subagent definitions that name it — `spec-writer`'s optimization-target and hand-off prose, `self-review`'s locator-drift and Subagent-ownership rows, `self-reflect`'s CAN-vs-MAY citation, and `self-improve`'s closed agent-stem enumeration. | `.claude/agents/spec-writer.md`, `.claude/agents/self-review.md`, `.claude/agents/self-reflect.md`, `.claude/agents/self-improve.md` | 1 |
| 4 | Update the remaining Skills that name it. The Spec-Amendment recipe on each `reference.md` is only part of the residue — most sites in the three `SKILL.md` files sit **outside** it: the never-inline-edit-a-design-doc rules, the design-doc-ownership bail, and the binding "a spec-touching round runs `design` → `design-review` before `self-review`" rule. Plus `/interview`'s run-before gate and its exit line. Sweep each file whole. | `.claude/skills/interview/SKILL.md`, `.claude/skills/pr-commented/SKILL.md`, `.claude/skills/pr-commented/reference.md`, `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/pr-ci-failed/reference.md`, `.claude/skills/main-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/reference.md` | 1 |
| 5 | Update the `ai-docs/` inventory pages — the Subagent-table row and its `design-review` neighbour, the Task/Design sync-group rows and the domain-invariant row (each keyed by file path), and the CAN-vs-MAY citation. | `ai-docs/claude-tools-hierarchy.md`, `ai-docs/propagation-groups.md`, `ai-docs/improve-eval-contract.md` | 1 |
| 6 | Rewrite Checklist O's severity rule to *one rule, any axis is `major`*: replace `:152`'s justification clause, strip `:174`'s severity presuppositions while leaving its procedural content intact, leave `:184` untouched, delete the `:186` paragraph, and add the worked example. | `.claude/skills/ai-audit/reference.md` | — |
| 7 | Record the decision as a new `KD` row in a new dated section: the rename, the severity flip, and a pointer to Checklist O for the reader-vs-parser argument. | `ai-docs/key-decisions.md` | 1, 6 |
| 8 | Closing concept-level re-sweep of the whole live tree against the § Scope membership criterion, with a per-site verdict recorded in the progress file's decisions log; fix any in-class site subtasks 1–7 missed, and report any OUT-of-class site deliberately left alone. **Re-derive the class from the spec's two § Scope tables — never from the edit log of subtasks 1–7.** The same delegate authored those edits, so a sweep driven by its own record would only re-confirm what it already believed; the sweep must start from the criterion and meet the tree cold. | whole live tree (no new file expected) | 1–7 |

**Where rows 2–5's file contents come from.** Every "update X's <named passage>" claim in the table
above is the IN-class residue of the two sweeps in § Approach → *Site inventory*, which carries the
measurement. Each row's named passage is re-resolved by its sentence text at edit time, never by a
line number — so a row names the *kinds* of passage to look for, and the implementor's sweep, not
the row, is what bounds the edit.

**Subtask 6's site coordinates, re-pinned.** The spec pins `:152` / `:174` / `:184` / `:186` at
`fe7c6c1`; they still land on the same sentences at this design's base
`[measured 08271a3:.claude/skills/ai-audit/reference.md:152,174,184,186 · sed -n
'152p;174p;184p;186p' → the intro sentence ending "the project side renames, never the embedded
name"; the step-2 callout opening "The subtraction blinds this list to the SAME-AXIS clash"; the
trigger-table row "`comm -12` output is non-empty | `major` finding per name"; and the closing
paragraph opening "Cross-axis clashes are reportable but not automatically defects"]`. They are
locators, not the contract — the contract is the spec's disposition table, and subtask 6 re-resolves
each site by its sentence text before editing.

## Handoff plan

Every subtask changes **instructions/harness** files only (`.claude/**`, `ai-docs/**`) — no `*.go`
and no migration anywhere in the table `[derived → AC13]`. The change-type is therefore uniform,
which makes one group both legal and minimal.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The first group takes a handoff like every other.
- **Group A** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh), 1M-token window, routed to `subagent_type="general-purpose"` with **no**
  inline `model=` — subtasks 1–8 (instructions/harness change-type). Terminal group (8 subtasks;
  within the `1..=10` range, at or below the size cap of 10). One group is the floor for a
  single-change-type decomposition of this size, so the count is minimized; 1 ≤ 4, so no user
  gate applies. No handoff between groups — there is no second group.

**Not in Group A, by binding constraint.** AC12's Checklist O re-run stays with the
**orchestrator** at Step 9. Checklist O's embedded inventory is read from the blocks "the harness
injects into the orchestrator's context"
`[measured 08271a3:.claude/skills/ai-audit/reference.md:155 · sed -n '155p' → "The embedded
inventory is session state: the `Available agent types for the Agent tool` block and the `The
following skills are available` block the harness injects into the orchestrator's context, plus
the hook-event table the Step 2.1 `claude-code-guide` spawn returns"]` — a delegate's listings are
its own, so a delegate-side run measures the wrong session. The same placement also keeps the
`claude-code-guide` spawn the checklist's step 2 prescribes on the orchestrator side.

## Risks

- **R1 — the commit records a delete+add instead of a rename, failing AC2.** Git detects renames
  by similarity at diff time rather than storing them, and the default `-M` threshold is a
  similarity index over the file's size
  `[measured 08271a3 · git diff --help | grep -A4 'find-renames' → "-M[<n>], --find-renames[=<n>]
  Detect renames. If <n> is specified, it is a threshold on the similarity index (i.e. amount of
  addition/deletions compared to the file's size)"]`. *Mitigation:* subtask 1 changes only the
  frontmatter `name:`, the H1 and the one self-referential list, and commits that alone; the
  larger prose edits live in subtasks 2–8 against other files. Verified by AC2's own command
  (§ Test Design), which must print a `rename` summary line, not a delete plus an add.
- **R2 — the class is open, so a site found late is not an amendment.** The spec states the sites
  it names illustrate the class rather than bound it. *Mitigation:* subtask 8 re-sweeps the whole
  live tree after 1–7 land, in the same spirit as the re-run-the-gate-after-cleanup rule for
  truncating gates; a newly-surfaced site is inside the already-open class and is fixed in place,
  while a **second embedded-name clash** is out of scope and routes to § Deferred.
  `[derived → AC5]`
- **R3 — a string-level sweep matches the referent's neighbours.** This is the failure the spec's
  § *The trap this issue must not repeat* records. *Mitigation:* per-site read with a recorded
  verdict (§ Approach); the OUT-of-class referents are enumerated in the spec's second table and
  restated in subtask 8's acceptance. No `sed -i` / `rg -r` over the tree.
  `[derived → AC5]`
- **R4 — the run's own plan artefacts sit inside AC3's and AC4's search space until Step 12 moves
  them.** Both ACs now exclude "the history surfaces named in § *Out of scope*", and
  `ai-docs/plans/done/**` is one of them — but the spec and the design only *arrive* there at Step
  12, so a Step-9 verifier still sees them under `ai-docs/plans/`. The literals are genuinely
  present in both files while the run is in flight: the old path in the spec and in this design
  `[measured 08271a3 · grep -rn -F '.claude/agents/design.md' . → occurrences in the spec, this
  design, .claude/agents/{design-review.md,spec-writer.md,self-reflect.md},
  .claude/skills/task/{SKILL.md,reference.md}, ai-docs/{propagation-groups.md,improve-eval-contract.md},
  the `.state.md`, ai-docs/harness-gaps.md and ai-docs/plans/done/2026-08-30-mechanical-code-style-gates.design.md]`,
  and the dispatch literal likewise
  `[measured 08271a3 · grep -rn -F 'subagent_type="design"' . → .claude/skills/task/reference.md:13
  and :49, plus the spec, this design and the `.state.md`]`.
  *Resolution, measured rather than asserted:* `/task` Step 12 `git mv`s the spec **and** the
  design into `ai-docs/plans/done/` and retires the progress file and the `.state.md` into
  `ai-docs/plans/ignored/`, which is gitignored
  `[measured 08271a3:.claude/skills/task/SKILL.md · awk '/^### Step 12/,/^## /' → sub-step 4
  "`git mv` the spec and design files to `ai-docs/plans/done/`" and sub-step 9a "mv
  ai-docs/plans/<spec-base>.progress.md ai-docs/plans/ignored/"]`
  `[measured 08271a3:.gitignore:22 · grep -n 'ignored' .gitignore → "/ai-docs/plans/ignored/"]`.
  On the terminal PR tree every one of those paths is either a named history surface or absent, so
  **both AC3 and AC4 hold as written**. *Consequence, binding on the verifier and identical for the
  two ACs:* run each command **after** Step 12's moves, or with the history surfaces — including
  the whole of `ai-docs/plans/` — pruned while the run is in flight. Never against an unpruned
  mid-run tree. The residual the spec-writer flagged and declined to act on is exactly this
  ordering, and it is owned here rather than by a further spec amendment: the AC wording is now
  correct for the terminal tree, and only the *when* was ever open.
  **The two surviving in-flight sites are not defects and are not to be "fixed":** the spec's
  § Scope row that defines the dispatch value, and this design's own R6 paragraph, both must spell
  the old token to be intelligible, and both land in `done/**` at Step 12.
- **R5 — the script-backed ACs are not locally reachable without a permission prompt.**
  `permissions.allow` grants no `Bash(bash *)`, no `Bash(python3 *)` and no `Bash(./**)`
  `[measured 08271a3:.claude/settings.json · jq -r '.permissions.allow[]' → Edit(./**),
  Edit(.claude/**), Bash(go *), Bash(gofmt *), Bash(golangci-lint *), Bash(make *), Bash(git *),
  Bash(gh *), Bash(ast-index *), Bash(psql *), Bash(actionlint *), Bash(shellcheck *),
  Bash(grep *), Bash(rg *), Bash(jq *), Bash(awk *), Bash(sort *), Bash(wc *)]`, and `/task`'s own
  `allowed-tools` grants neither the guard scripts nor `comm`
  `[measured 08271a3:.claude/skills/task/SKILL.md:6 · sed -n '6p' → the allowed-tools line, whose
  only script entry is `Bash(.claude/skills/task/scripts/append-task-run.sh *)`]`. *Decision:*
  **AC7, AC8 and AC12's script-dependent half are discharged by CI on the PR**, not locally. The
  *Harness guards* job runs the citation guard, the guard regression suites and the relative-link
  check
  `[measured 08271a3:.github/workflows/ci.yml:148-175 · sed -n '148,175p' → the "citation
  namespaces resolve", "guard regression suites" and "relative markdown links resolve" steps]`,
  and it is reached on this diff because its `paths-filter` includes `.claude/**` and `ai-docs/**`
  `[measured 08271a3:.github/workflows/ci.yml:46-50 · sed -n '46,50p' → "harness:" with
  '.claude/**', 'ai-docs/**', 'AGENTS.md', 'CLAUDE.md']`. A local run remains available at the
  cost of one approval prompt; the design does not assume it is free, and no permission grant is
  added. *Note for the verifier:* under `/ai-audit`'s own frontmatter — not under `/task`'s — the
  guard scripts and `comm` **are** granted
  `[measured 08271a3:.claude/skills/ai-audit/SKILL.md:6 · sed -n '6p' → allowed-tools including
  Bash(comm *), Bash(.claude/skills/ai-audit/scripts/check-citations.sh),
  Bash(.claude/skills/ai-audit/scripts/test-check-citations.sh)]`, so an `/ai-audit` invocation is
  the unattended local route if one is wanted later. The guard-suites pin § Test Design carries is
  re-resolved at this round's base
  `[measured 08271a3:.github/workflows/ci.yml:152 · sed -n '152p' → "- name: guard regression
  suites"]`.
- **R6 — the new name is not dispatchable in the session that creates it.** The set of
  `subagent_type` values is session state (spec § *Technical constraints*): after subtask 1 lands,
  the tree says `design-writer` while the running session still resolves `design`. *Mitigation:*
  no verification step dispatches the new name, and if `/task` re-enters Step 6 or the Design
  Amendment recipe **after** subtask 1, the orchestrator spawns the still-registered
  `subagent_type="design"` even though the edited files name `design-writer` — a temporary
  divergence between the tree and the live registry, not a defect and not a site to "fix".
  *The premise is the spec's, and it is not verifiable from inside this session* — the only way to
  test "the old name still resolves after its definition file moves" is to dispatch it, which would
  consume a real spawn to learn something no AC depends on. It is deliberately **non-load-bearing**:
  if a `subagent_type="design"` spawn does fail after subtask 1, the fallback is to hand the work to
  a `general-purpose` spawn pointed at `.claude/agents/design-writer.md` by path — reading the
  renamed file directly. Under no circumstance is the answer to re-add the old name to the tree.
- **R7 — the rewritten Checklist O prose trips the audit that owns it.** Checklist M sub-checks 2
  and 6 govern exactly the paragraph shape the rewrite produces (§ Approach). *Mitigation:* one
  bold-uppercase verb per non-table paragraph, and a demonstrator within eight lines of the
  contrast paragraph. `[derived → AC9, AC10, AC11]`
- **R8 — a `#N` citation added by the rewrite fails the citation guard.** Check (1) flags a bare
  `#N` above the repository's PR high-water mark. Citing this task's own issue is safe: the guard
  skips any `#N` at or below that mark
  `[measured 08271a3:.claude/skills/ai-audit/scripts/check-citations.sh:84 · sed -n '84p' →
  '[ "$n" -le "$LOCAL_MAX" ] 2>/dev/null && continue']`, and issue #10 is this repository's own and
  below it. Any citation to the sibling projects must still carry its namespace. `[derived → AC8]`
- **R9 — a relative markdown link to the renamed file breaks (AC7).** Measured negative: no
  markdown link anywhere in the tree targets the file — every reference to it is inline code
  `[measured 08271a3 · grep -rnE '\]\([^)]*design\.md' --include='*.md' . → no match anywhere in
  the tree]`. The renamed file also keeps its directory, so its own outbound `../../ai-docs/…`
  links resolve unchanged. The residual risk is a link introduced by subtasks 2–7, which AC7's
  command and the CI step both catch.

## Test Design

This task ships **no Go code**, so there is no `_test.go` to add and no fixture to build
`[derived → AC13]`. The verification surface is the AC command set below plus the CI *Harness
guards* job; the guard regression suites CI already runs —
`.claude/skills/ai-audit/scripts/test-check-citations.sh`,
`.claude/skills/task/scripts/test-append-task-run.sh` and
`.claude/skills/ai-audit/scripts/test-piped-gate-guard.sh`
`[measured 08271a3:.github/workflows/ci.yml:152-159 · sed -n '152,159p' → the "guard regression
suites" step invoking those scripts]` — are re-run unchanged, and this task adds no case to
any of them `[derived → AC8]`.

Each command below is the one the orchestrator runs at Step 9 for that AC. Every claim in this
section is about a state this task will create, so each carries `[derived → …]`.

**Three ACs share one search space; it is defined once here.** AC3, AC4 and AC5 each range over the
*live* tree, which is the tracked tree minus the history surfaces the spec's § *Out of scope* names.
Call that set **LIVE**: `git ls-files` minus `docs/**`, `ai-docs/learnings.md`,
`ai-docs/harness-gaps.md`, `ai-docs/plans/**`, `ai-docs/metrics/**` and `ai-docs/deferred/**`.
`ai-docs/plans/**` is pruned in full rather than only its `done/` subtree, because the run's own
spec, design and progress file sit outside `done/` until Step 12 moves them (§ Risks R4); after
Step 12 the two prunings coincide. Every command below spells LIVE out rather than assuming a
verifier reconstructs it. `git ls-files` is the category-matched probe for *tracked* status and is
granted; `ls` is granted in neither `.claude/settings.json` `permissions.allow` nor `/task`'s
`allowed-tools`, so no command here uses it
`[measured 08271a3 · jq -r '.permissions.allow[]' .claude/settings.json | grep -c '^Bash(ls' → 0,
and sed -n '6p' .claude/skills/task/SKILL.md | grep -c 'Bash(ls' → 0]`.

- **AC1 — the file and its frontmatter.** Existence half, with a granted and category-matched tool:
  `git ls-files .claude/agents/` → lists `.claude/agents/design-writer.md` and does **not** list
  `.claude/agents/design.md`. (`git ls-files` answers the tracked-status question AC1 actually asks;
  `ls` would answer an on-disk question and is granted nowhere — see the LIVE note above.) Then
  `awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/agents/design-writer.md`
  → `design-writer`, matching the basename. That awk program is Checklist O's own step-1 extractor
  `[measured 08271a3:.claude/skills/ai-audit/reference.md:168 · sed -n '168p' → the recipe's
  "Subagent names:" bullet, carrying that exact awk one-liner over .claude/agents/*.md]`, so the
  same command discharges the spec's name-equals-basename constraint. `[derived → AC1]`
- **AC2 — rename continuity.** `git diff -M --summary <base>...HEAD -- .claude/agents/` must print
  a `rename .claude/agents/{design.md => design-writer.md} (NN%)` line and no separate
  delete/create pair for those paths. `[derived → AC2]`
- **AC3 — no live site carries the old path.** `grep -n -F '.claude/agents/design.md' $LIVE` →
  empty, where `$LIVE` is the file set defined above. Run it **after** Step 12's `done/` move, or
  with `ai-docs/plans/**` pruned, per § Risks R4. `[derived → AC3]`
- **AC4 — no stale dispatch value.** Same shape and the **same pruning as AC3**, which is what the
  amended AC now requires: `grep -n -F 'subagent_type="design"' $LIVE` → empty. The closing quote
  already excludes `subagent_type="design-review"`, and the sibling's own dispatch sites must still
  resolve — `grep -n -F 'subagent_type="design-review"' .claude/skills/task/reference.md` non-empty
  — proving they were not collaterally rewritten. Ordering is identical to AC3's and is R4's, not a
  separate rule: run it after Step 12, or with `ai-docs/plans/**` pruned. `[derived → AC4]`
- **AC5 — the class, per-site.** `grep -niw 'design' $LIVE` — the same `$LIVE` set as AC3 and AC4,
  spelled out rather than left as a placeholder, since AC5 carries the whole per-site judgement and
  an unfilled search space would silently narrow it. Every hit is re-read against the IN- and
  OUT-of-class tables in the spec's § Scope and given a verdict recorded in the progress file's
  decisions log; the OUT verdicts are recorded too, because an unrecorded OUT is indistinguishable
  from a missed site. A tally is not the evidence here — the recorded per-site judgement is.
  `[derived → AC5]`
- **AC6 — the inventory pages.** `grep -n 'design-writer' ai-docs/claude-tools-hierarchy.md ai-docs/propagation-groups.md`
  resolves in the Subagent table and in the Task/Design group rows, and
  `grep -nw 'design' ai-docs/claude-tools-hierarchy.md ai-docs/propagation-groups.md` shows no
  remaining occurrence whose referent is this Subagent. `[derived → AC6]`
- **AC7 — relative links.** The CI *Harness guards* "relative markdown links resolve" step is the
  authority (§ Risks R5). Locally it is the same inline `python3` program, which costs an approval
  prompt. `[derived → AC7]`
- **AC8 — citation namespaces.** CI's "citation namespaces resolve" and "guard regression suites"
  steps. `[derived → AC8]`
- **AC9 / AC10 / AC11 — the Checklist O rewrite.** *Read AC9's "per-axis distinction" as
  severity-scoped, because AC11 says so.* AC11 requires `:174` to keep **every** procedural claim,
  and one of those claims — "The session listing stays authoritative for the cross-axis sweep only"
  `[measured 08271a3:.claude/skills/ai-audit/reference.md:174 · sed -n '174p' → that sentence
  closing the step-2 callout]` — *is* a per-axis distinction, about which **instrument covers which
  axis**. It must survive. The spec's disposition table settles the apparent conflict in AC11's
  favour: `:174`'s "Procedural content survives **unchanged**", with only the severity
  presuppositions removed
  `[measured 08271a3:ai-docs/plans/2026-09-03-rename-design-subagent.spec.md:177 · sed -n '177p' →
  "Procedural content survives **unchanged** … Only the two severity presuppositions go: it may no
  longer imply that same-axis is the axis the table rates `major`, nor that same-axis is 'the
  serious case'"]`. So the rule a verifier applies is: **an axis distinction about *severity* is a
  defect; an axis distinction about *instrument coverage* is required.** Failing a correct rewrite
  on a literal reading of AC9 is the error this paragraph exists to prevent. With that settled,
  read the whole `## Checklist O` section and
  check, per site: `:152`'s verdict survives with a justification that names reader/model
  confusion and **not** dispatch-time ambiguity; `:174` keeps every procedural claim (the
  subtraction blinds the listing; the `claude-code-guide` roster is the fix) while asserting no
  severity or "serious case" difference between axes; `:184` is byte-identical to its pre-change
  form; the `:186` paragraph is gone. Mechanical support:
  `grep -niE 'cross-axis|dispatch time|dispatches through different tools|not automatically|minor' .claude/skills/ai-audit/reference.md`
  restricted to the section — every surviving hit must be one the disposition table permits.
  `[derived → AC9, AC10, AC11]`
- **AC12 — the Checklist O re-run.** Orchestrator-side at Step 9 (§ Handoff plan). Build
  `project-names.txt` with the checklist's own `awk` extractor over `.claude/agents/*.md` and
  `.claude/skills/*/SKILL.md`; transcribe the embedded agent-type and skill listings from the
  orchestrator's context into `embedded-names.txt`; **assert the embedded list is non-empty
  before reading the intersection** — an empty list is `inconclusive`, never `pass`. Intersect
  with `grep -Fxf project-names.txt embedded-names.txt` rather than `comm -12`, because `comm` is
  not in `/task`'s reach (§ Risks R5) while `grep` is. Then run the same-axis half against the
  `claude-code-guide` built-in roster, per `:174`. A clash on any name **other** than
  `design-writer` is the spec's § Deferred item: file it as its own issue and report it in the
  task summary — do not fix it here. `[derived → AC12]`
- **AC13 — the diff's shape.** `git diff --name-only <base>...HEAD` must contain no path under
  `cmd/`, `internal/`, `docs/`, and no `*.go`, `*.sql`, `go.mod` or `go.sum`. `[derived → AC13]`

## Open questions

- **Closed this round: whether AC3 and AC4 hold mid-run or on the terminal tree.** The round-1
  design raised this against AC3; the spec's round-2 amendment gave AC4 the same history-surface
  exclusion, and § Risks R4 now owns the ordering for **both** — run each command after Step 12's
  moves, or with `ai-docs/plans/**` pruned. No further spec amendment is needed: the AC wording is
  correct for the terminal tree and only the *when* was ever open. Recorded here rather than
  deleted so the next reader sees it was answered, not dropped.
- **Does any project name other than `design` clash with an embedded name?** Unanswerable before
  AC12's run, since the embedded inventory is session state. Handled by the spec's § Deferred:
  report and file, do not fix.
