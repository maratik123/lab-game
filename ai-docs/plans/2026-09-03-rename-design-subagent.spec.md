# Rename the `design` Subagent

**Source:** issue #10
**Date:** 2026-09-03
**Tracked in:** #10

The project Subagent declared as `name: design` in `.claude/agents/design.md` shares its
name with a Skill the harness ships embedded. The two dispatch through different tools, so
the *parser* never confuses them — but the owner's field experience across sibling projects
is that the *model* does: it picks the wrong tool, or blends the two definitions, precisely
because the name is the same. Unambiguous-to-the-parser and unambiguous-to-the-reader are
different properties, and only the second one was ever load-bearing.

Two things follow, and this task does both. The Subagent is renamed to **`design-writer`**.
And Checklist O — the `/ai-audit` check that inspected this exact clash and waved it through —
has its severity rule rewritten to **one rule, any axis is `major`**, because that rule's
stated motive ("dispatch resolves, therefore the clash is theoretical") is the claim the field
evidence contradicts. Renaming only `design` would leave the rule that permitted it standing
for the next collision.

## Scope

1. **Rename the Subagent definition** to **`design-writer`** (owner, round 2). `git mv
   .claude/agents/design.md .claude/agents/design-writer.md`, and set the file's YAML `name:`
   value to `design-writer`.
2. **Update every live site that refers to the Subagent** — see *the class* below.
3. **Rewrite Checklist O's severity rule** (`.claude/skills/ai-audit/reference.md`
   § *Checklist O — Embedded-name clash scan*) to the owner's round-2 verdict: **one rule,
   any axis is `major`.** An embedded-name clash is a `major` finding whichever axis it
   crosses; the finding names the axis, and naming it does not lower the severity. Three
   sites carry the change, enumerated with their verbatim text in § *Source conflicts* —
   `:186` is deleted outright, `:152`'s justification clause is rewritten, and `:174`'s two
   severity presuppositions are removed while its procedural content survives intact.
4. **Re-run Checklist O** against the renamed tree, with the AXIOM at
   `.claude/skills/ai-audit/reference.md` § Checklist O honoured: an empty embedded-name
   list is `inconclusive`, never `pass`.
5. **Propagate** per `AGENTS.md` § *Propagation Rule*, including the sync-group obligations
   in § *Technical constraints*.

### The class of sites to update (membership criterion, not an enumeration)

The rename touches **all live sites whose claim this diff falsifies**, per `AGENTS.md`
§ *Propagation Rule* step 4 — prose, tables, code fences, spawn examples and scripts alike.
The sites named anywhere in this spec or in issue #10 **illustrate** the class; they do not
bound it. A site found late is inside the already-open class, not an amendment.

A site is IN the class when the token denotes **the Subagent**:

| Shape | Example |
|---|---|
| Its definition path | `.claude/agents/design.md` |
| Its dispatch value | `subagent_type="design"` (`subagent_type="design-review"` is a different agent — untouched) |
| Its frontmatter | `name: design` |
| Prose naming it as an agent | "the `design` Subagent", "spawn `design`", "`design` / `design-review` / `self-review` gates inherit it" |

A site is OUT of the class when the token denotes something else. This distinction is the
part that failed before (issue #10 § *The trap this issue must not repeat*): the count moved
three times because sweeps matched the string instead of the referent. Sweep at concept
level, and record a per-site judgement rather than trusting a grep tally.

| Referent | Examples that stay `design` |
|---|---|
| The design **document / artefact** | `*.design.md`, "the design", "Design Document", "the design's § Cases table" |
| The design **phase or round** | "the design phase", "`design` rounds", "the Design Amendment recipe", "interview → spec → design → design-review" |
| A **different agent** | `design-review` (unchanged, and it pairs with the new name) |
| The **game-design corpus** | `docs/DESIGN.md`, `docs/IDEAS.md` |
| Ordinary English | "prose by design", "a design obligation", "destroyed by design" |

## Out of scope

- **The embedded `design` Skill.** Not this repository's to rename; Checklist O's own rule
  is that the project side renames, never the embedded side.
- **`design-review`.** Its name, file and contract are unchanged.
- **The `.design.md` extension, `## Design` headings, and every phase/round name.** They
  name the artefact and the phase, which this task does not rename.
- **`docs/**`.** The game-design corpus is untouched.
- **History surfaces**, left alone per `AGENTS.md` § *Propagation Rule* step 4 and
  § *Learning Log* Boundary rule 1: `ai-docs/learnings.md`, `ai-docs/harness-gaps.md`
  (append-only by its own header), `ai-docs/plans/done/**`, `ai-docs/metrics/task-runs.jsonl`.
- **`ai-docs/deferred/_inbox.jsonl`.** `AGENTS.md` makes it writable only by `/task` Step 12
  and `/triage`; its `design` tokens denote the design document in any case.
- **This spec's `.state.md` sibling.** It quotes issue #10 verbatim; a verbatim quotation is
  not a live claim about the Subagent's name.
- **Go source.** No `cmd/**` or `internal/**` change; no schema, migration or ledger change.
- **Any second embedded-name clash** the Checklist O re-run surfaces — see § Deferred.

## Deferred

- A clash between some *other* project Subagent / Skill / hook-event name and an embedded
  name, if the Checklist O re-run surfaces one | this task's subject is the `design` name and
  the severity rule; a second rename carries its own propagation sweep and its own judgement
  problem | yes — file it as a separate issue and report it in the task's summary.
- Re-wording the design **phase** vocabulary ("design rounds", "the design gate") | the phase
  is still the design phase whichever agent runs it; nothing in the clash argument reaches it | no.

## Key decisions

| Question | Decision |
|---|---|
| Q1 — the new Subagent name | **`design-writer`** — owner, round 2, re-confirming the pick issue #10 records from session `a47d904a`. It matches the directory's `<artifact>-writer` pattern (`spec-writer`, `code-writer`), pairs with the unchanged `design-review`, and names the role rather than the artifact. |
| Q2 — what Checklist O's severity rule becomes | **One rule, any axis is `major`** — owner, round 2. The cross-axis carve-out is deleted, not re-justified: any embedded-name clash is `major`, the finding names the axis, and the axis does not lower the severity. |
| Q2a — which Checklist O sites the Q2 verdict touches | **Three, not one.** The round-2 answer's option text said the rewrite "only removes `:186`". Checked against the file at `fe7c6c1`, that under-counts: `:152` and `:184` do already carry the *verdict*, but `:152` also carries the *justification* ("A clash makes it ambiguous which definition a name resolves to at dispatch time"), which is false for precisely the cross-axis case the rule must now cover — delete `:186` alone and the only surviving statement of *why* argues a future auditor straight back into the carve-out. `:174` is a fourth site the round-1 sweep missed, and two of its clauses presuppose the carve-out. `:184` is genuinely unchanged. Widening from one site to three is not a scope change the owner has to re-approve: issue #10's acceptance item 3 already requires the justification to stop resting on dispatch resolvability, and `:152` and `:174` are where it rests once `:186` is gone. |
| Scope of the sweep: issue #10's 20-file table vs. a concept-level sweep | Concept-level sweep, per issue #10 § *The trap*. Verified at `fe7c6c1`: the standalone token `design` occurs in live files beyond that table — among them `.claude/skills/task/scripts/test-append-task-run.sh`, `.claude/skills/task/preambles.md`, `.claude/skills/task/inbox-propagation.md`, `ai-docs/context.md`, `ai-docs/key-decisions.md` — and most of those occurrences are OUT of the class. Neither the table nor a grep tally is the boundary; the membership criterion is. |
| Whether the rename is mechanical (`sed`-style) or per-site | Per-site with a recorded judgement. A blanket substitution corrupts every OUT-of-class site in the table above. |
| Whether Checklist O's index row in `.claude/skills/ai-audit/SKILL.md` changes | Read at `fe7c6c1`: the row states the invariant and the `inconclusive` AXIOM and names **no severity and no axis**, so the Q2 verdict does not falsify it. It changes only if the rewrite alters something it asserts. Design decides. |
| Where the rename's rationale is recorded so it is not re-litigated | Design's call between `ai-docs/key-decisions.md` and the rewritten Checklist O prose itself; the reader-vs-parser argument must survive somewhere live, since it is the reason the rule changed. |

## Technical constraints

- **Sync groups (`AGENTS.md` § *Propagation Rule*, table in `ai-docs/propagation-groups.md`).**
  The renamed file is the anchor of the **Task/Design group** — `.claude/skills/task/SKILL.md`
  (Steps 6–8) AND `.claude/agents/design-review.md` AND `.claude/skills/context-reset/SKILL.md`.
  The **domain-invariant** row and the **Interview** group also reach files this diff touches.
  Group membership is stated by file *path*, so `ai-docs/propagation-groups.md` is itself a
  site in the class.
- **`ai-docs/claude-tools-hierarchy.md` is mandatory.** `AGENTS.md` § *Propagation Rule*
  requires it to be updated in the same PR for any edit that changes a Subagent contract; its
  Subagent table names this agent in a row of its own.
- **A Subagent's `name:` must equal its file basename.** Checklist O's own step 1 reads names
  with `awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/agents/*.md`;
  a mismatch between `name:` and basename is a latent dispatch defect.
- **Registration is session state.** The set of dispatchable `subagent_type` values is loaded
  at session start, so the new name becomes dispatchable only in a session started after the
  rename lands, while the previous name keeps resolving for the remainder of the current one.
  Neither fact is a defect; the design must not build a verification step that depends on
  dispatching the new name inside the session that creates it.
- **Checklist O's embedded inventory is not in this repository.** It is read from the
  session's agent-type and skill listings plus the `claude-code-guide` spawn — the checklist
  says so explicitly and forbids substituting `ai-docs/claude-tools-hierarchy.md` for it.
- **Gate reachability.** The citation guard and its regression suite live at
  `.claude/skills/ai-audit/scripts/check-citations.sh` and `…/test-check-citations.sh`, and
  the relative-link check is an inline `python3` step in `.github/workflows/ci.yml`
  (job *Harness guards*). `.claude/settings.json` `permissions.allow` grants no `Bash(bash *)`,
  `Bash(python3 *)` or `Bash(./**)` entry, so none of the three runs unattended today. The
  design chooses one of: add the needed grant as its own subtask, or state that these three
  ACs are verified by CI on the PR rather than locally — it may not assume a local run is free.
- **`AGENTS.md` § *Build & Test* applies unchanged and needs no restatement here.** Its
  instruction-file AXIOM already forbids every flow other than `/ai-audit` to measure, report
  or plan around instruction-file size; this spec adds no clause on the subject, and design
  and review add none either. That arithmetic is what consumed session `a47d904a`, and the
  rule that ended it is already in force.

## Source conflicts

`.claude/skills/ai-audit/reference.md` § *Checklist O* disagrees with itself about the
severity of an embedded-name clash. Four sites, all read at
`[source: fe7c6c1:.claude/skills/ai-audit/reference.md:152,174,184,186 · sed -n '152p;174p;184p;186p']`:

- **:152 (intro)** — "A clash makes it ambiguous which definition a name resolves to at
  dispatch time. Any match → `major` finding with a rename recommendation; the project side
  renames, never the embedded name."
- **:174 (step-2 callout)** — "**The subtraction blinds this list to the SAME-AXIS clash — the
  one the table rates `major`.** A project skill named `X` and an embedded skill named `X`
  collapse to a single listing row, and step 2 then subtracts it, so the intersection is empty
  precisely when the serious case is present. … The session listing stays authoritative for
  the cross-axis sweep only."
- **:184 (trigger table)** — "| `comm -12` output is non-empty | `major` finding per name:
  *\"Project-defined `<name>` clashes with embedded `<name>`. Rename the project side.\"* |"
- **:186 (closing paragraph)** — "**Cross-axis clashes are reportable but not automatically
  defects.** A project *Subagent* sharing a name with an embedded *Skill* dispatches through
  different tools (`Agent(subagent_type=…)` vs `Skill(skill=…)`) and resolves unambiguously
  today. Report it at `minor` with the axis named, and let the owner decide; reserve `major`
  for a same-axis collision, where dispatch is genuinely ambiguous."

**The shape of the disagreement.** `:152` and `:184` admit no axis distinction and rate **any**
match `major`. `:186` carves cross-axis down to `minor`. `:174` presupposes the carve-out twice
over — "the one the table rates `major`" implies only same-axis is rated so, and "the serious
case" implies cross-axis is not one. Issue #10 attributes the `minor` rating to "the severity
table", but the table (`:184`) does not say it; only the prose at `:186` does.

**Resolution — one rule, any axis is `major`. Chosen by the owner, round 2 (prior_qa answer to
the round-1 Q2), not by this spec.** What each site becomes:

| Site | Disposition |
|---|---|
| `:152` | Verdict already correct and stays. Its **justification** clause is rewritten: dispatch-time ambiguity is not why a clash matters, because for a cross-axis clash dispatch is *not* ambiguous and the clash still bites. The replacement states the reader/model-confusion ground — the owner's repeated field observation that a model picks the wrong tool or blends the two definitions when a Skill and a Subagent share a name. |
| `:174` | Procedural content survives **unchanged** — the subtraction genuinely does blind the session listing to a same-axis clash, and the `claude-code-guide` roster genuinely is the fix. Only the two severity presuppositions go: it may no longer imply that same-axis is the axis the table rates `major`, nor that same-axis is "the serious case". |
| `:184` | Unchanged. It states the verdict with no axis and no rationale. |
| `:186` | Deleted outright. It is the carve-out. |

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | `.claude/agents/design-writer.md` exists, `.claude/agents/design.md` does not, and the new file's YAML frontmatter declares `name: design-writer` — matching its basename. |
| AC2 | The renamed file's git history is continuous with the former `.claude/agents/design.md` — the change is recorded as a rename, not as a delete plus an add. |
| AC3 | No file matching glob `.claude/**`, `ai-docs/**` (excluding the history surfaces named in § *Out of scope*), `AGENTS.md`, `CLAUDE.md`, `.github/**` or `Makefile` contains the literal string `.claude/agents/design.md`. |
| AC4 | No file in the tree contains the literal string `subagent_type="design"` (with the closing quote, which already excludes `subagent_type="design-review"`). Every dispatch of this Subagent reads `subagent_type="design-writer"`, and the `design-review` dispatch sites are unchanged. |
| AC5 | Every live site in the class defined in § *Scope* names the Subagent `design-writer`, and no live site names the Subagent `design`. Sites whose referent is OUT of the class per that section's second table are unchanged. |
| AC6 | `design-writer` occurs in `ai-docs/claude-tools-hierarchy.md`'s Subagent table and in `ai-docs/propagation-groups.md`'s Task/Design group rows; neither file names this Subagent `design` any longer. |
| AC7 | Every relative markdown link in every `*.md` file in the repository resolves to an existing path (the condition the CI *Harness guards* job's relative-link step enforces). |
| AC8 | The citation-namespace invariant stated in `.claude/skills/ai-audit/scripts/check-citations.sh`'s header holds over the post-rename tree, and its regression suite `.claude/skills/ai-audit/scripts/test-check-citations.sh` passes every case it defines. |
| AC9 | `.claude/skills/ai-audit/reference.md` § *Checklist O* states exactly one severity rule for an embedded-name clash — `major`, on any axis — and no text in that section states or implies a lower severity, a per-axis distinction, or an "not automatically a defect" disposition for any clash. |
| AC10 | No text under `.claude/skills/ai-audit/` § *Checklist O* gives dispatch-time ambiguity as the reason a clash matters, and none asserts that a cross-axis clash is acceptable, theoretical, or lower-severity because the two names dispatch through different tools. The section instead states the reader/model-confusion ground recorded in § *Source conflicts*. |
| AC11 | Each of the four Checklist O sites listed in § *Source conflicts* matches its row in that section's disposition table: `:152` keeps its verdict with a rewritten justification, `:174` keeps every procedural claim while asserting no severity or "serious case" distinction between axes, `:184` is unchanged, and the `:186` paragraph is absent from the file. |
| AC12 | A Checklist O run over the post-rename tree, performed with an embedded-name list of at least one entry, reports no clash for `design-writer`. A run whose embedded list is empty satisfies nothing — it is `inconclusive` by the checklist's own AXIOM. |
| AC13 | No file changed by this task is under `cmd/**`, `internal/**`, `docs/**`, or is a `.go`, `.sql`, `go.mod` or `go.sum` file. |

## Open questions

- Whether the rewritten Checklist O should also carry a worked example of the `design` →
  `design-writer` clash (so a future auditor sees why the rule changed) or stay abstract.
  Design's call; either satisfies AC9, AC10 and AC11.
- Whether `ai-docs/key-decisions.md` gains a `KD-n` row for the reader-vs-parser argument.
  Recorded as a Key decision above; the rationale must live somewhere live, and the design
  picks where.
- Whether any project name other than `design` clashes with an embedded name. Unanswerable
  before the Checklist O re-run, since the embedded inventory is session state. Handled by
  § Deferred: report and file, do not fix here.
