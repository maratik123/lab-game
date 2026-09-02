# Design: Rename the `design` Subagent

**Issue:** #10
**Date:** 2026-09-03

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
ordinary English ("prose by design", "a design error"). Those referents are not hypothetical: the
whole-tree sweep behind § Decomposition found live occurrences of the word spread across the
harness corpus and the Go packages alike, the OUT-of-class ones dominating
`[measured b448c45 · for f in $(git ls-files); do grep -cwi design "$f"; done → non-zero in files
ranging from .claude/skills/task/reference.md and AGENTS.md through ai-docs/doc-convention.md,
.golangci.yml, cmd/bot/main.go and internal/store/*.go]`. A `sed`-style substitution corrupts them.
Rejected for that reason; the alternative kept is a per-file read with a recorded per-site verdict,
closed by a whole-tree re-sweep as the last subtask.

**Site inventory.** The files named in § Decomposition are the IN-class subset of that sweep,
narrowed by a shape-directed pass over the same corpus
`[measured b448c45 · grep -rnEi '(`design`|design Subagent|subagent_type="design"|agents/design\.md|name: design|spawn .?design)' over the live tree minus docs/, ai-docs/learnings.md,
ai-docs/harness-gaps.md, ai-docs/plans/done/ and ai-docs/metrics/ → hits in .claude/agents/{design.md,design-review.md,spec-writer.md,self-review.md,self-reflect.md,self-improve.md},
.claude/skills/{task/SKILL.md,task/reference.md,context-reset/SKILL.md,interview/SKILL.md,pr-commented/{SKILL.md,reference.md},pr-ci-failed/{SKILL.md,reference.md},main-ci-failed/{SKILL.md,reference.md}},
and ai-docs/{claude-tools-hierarchy.md,propagation-groups.md,improve-eval-contract.md}]`. Because
the class is open by the spec's own statement, that list is the **starting** set, not the boundary
— subtask 8 re-derives it against the finished tree (§ Risks R2). Neither `AGENTS.md` nor
`CLAUDE.md` appears in it: every `design` token in `AGENTS.md` resolves to `docs/DESIGN.md`, to the
design document, or to ordinary English
`[measured b448c45:AGENTS.md · grep -nwi design AGENTS.md → docs/DESIGN.md references, "Thin by
design", "a design risk row", "design-blocking STOP", "pure function by design"]`.

Two judgement calls made here so they are not re-litigated per site:

- **`### Step 6: Design Subagent`** in `.claude/skills/task/SKILL.md` names the agent, not the
  phase, so it is IN the class. It carries no inbound anchor link
  `[measured b448c45 · grep -rn '#step-6\|design-subagent\|Step 6: Design' --include='*.md' . →
  only .claude/skills/task/SKILL.md:108 itself]`, so renaming the heading breaks no cross-reference.
- **`Designer Subagent.`**, the lead sentence of the renamed file
  `[measured b448c45:.claude/agents/design.md:9 · sed -n '9p' → "Designer Subagent. Receives a
  task description…"]`, is a **role noun**, not the registered name — it stays as prose. The H1
  above it is the site that changes, because the directory's convention is the Title-Cased
  registered name plus "Subagent"
  `[measured b448c45:.claude/agents/*.md · awk 'NR<12 && /^# /' → "# Code-Writer Subagent",
  "# Design Review Subagent", "# Spec Writer Subagent", "# Design Subagent"]` — so the renamed
  file's H1 becomes `# Design-Writer Subagent`, matching its closest sibling `code-writer`.

### Why the Checklist O rewrite carries a worked example

The spec leaves this open (§ Open questions, item 1). Decision: **include one**, naming the
`design` → `design-writer` case. Two grounds. First, the rewritten `:152` justification is
exactly the paragraph shape Checklist M sub-check 6 triggers on — a Pattern 2 fail-loud verb plus
one of the stronger contrast markers `instead` / `wrong` / `correct` / `forbidden`, which demands
a fenced block or a two-column table within eight lines or the paragraph flags
`[measured b448c45:.claude/skills/ai-audit/checklist-m.md:15 · sed -n '15p' → "trigger iff the
paragraph contains BOTH (a) a Pattern 2 fail-loud verb AND (b) one of the stronger contrast
markers `instead` / `wrong` / `correct` / `forbidden`… no example follows, flag the paragraph"]`.
Second, the example is the evidence a future auditor needs for *why* the carve-out went, which is
the failure the spec's own § Source conflicts identifies. Checklist M sub-check 2 caps bold-uppercase verbs at one
per non-table paragraph, so the rewrite is written to that cap
`[measured b448c45:.claude/skills/ai-audit/checklist-m.md:11 · sed -n '11p' → "Pattern 2
(fail-loud verbs) — at most one bold-uppercase verb per paragraph"]`.

The new rule is written as **prose plus the existing trigger table**, not as a new `> **AXIOM —`
blockquote: Checklist M sub-check 1 requires an action table inside every AXIOM blockquote, and
the verdict already has one — the trigger table that ends the section
`[measured b448c45:.claude/skills/ai-audit/reference.md:180-184 · sed -n '180,184p' → the
`| Trigger | Action |` table whose last row rates a non-empty intersection `major`]`.

### Where the rationale lives

The spec leaves this open too (§ Open questions, item 2). Decision: **both surfaces, different
jobs, no duplication.** AC10 already forces the reader/model-confusion *argument* into Checklist O
itself. `ai-docs/key-decisions.md` gains a KD row that records the *decision* — its stated purpose
is "so a later reader does not re-litigate a settled trade-off"
`[measured b448c45:ai-docs/key-decisions.md:3 · sed -n '3p' → "Decisions with the reasoning that
produced them, so a later reader does not re-litigate a settled trade-off"]` — and points at
Checklist O for the argument rather than restating it. It lands in a **new dated section** after
the last existing KD, because the KD numbering ascends in reading order and the trailing section
is `## Ledger core (2026-09-02)`
`[measured b448c45:ai-docs/key-decisions.md · grep -n '^## ' → "## Stack", "## Infrastructure",
"## Repository and harness", "## Ledger core (2026-09-02)"]`; appending under
`## Repository and harness` would put a higher number above a lower one.

### Rejected alternatives

- **Blanket `sed`/`rg -r` substitution over the tree.** Rejected: corrupts the OUT-of-class
  referents above, and a mutating flag that rewrites output while exiting 0 is precisely the
  silent-success shape `AGENTS.md` § *Build & Test* warns against
  `[measured b448c45:AGENTS.md:75 · grep -n 'rg -r' AGENTS.md → "the same silent-success shape
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
| 2 | Update the **Task/Design sync group**: the dispatch examples, the Step-6 heading and body, the Design-Amendment prose, the handoff-trigger prose, the anti-pattern table rows, and `design-review`'s own frontmatter description and checklist cross-references. | `.claude/skills/task/SKILL.md`, `.claude/skills/task/reference.md`, `.claude/agents/design-review.md`, `.claude/skills/context-reset/SKILL.md` | 1 |
| 3 | Update the remaining Subagent definitions that name it — `spec-writer`'s optimization-target and hand-off prose, `self-review`'s locator-drift and Subagent-ownership rows, `self-reflect`'s CAN-vs-MAY citation, and `self-improve`'s closed agent-stem enumeration. | `.claude/agents/spec-writer.md`, `.claude/agents/self-review.md`, `.claude/agents/self-reflect.md`, `.claude/agents/self-improve.md` | 1 |
| 4 | Update the remaining Skills that name it — `/interview`'s run-before gate and exit line, and the Spec-Amendment recipe carried by the CI/comment skills and their reference pages. | `.claude/skills/interview/SKILL.md`, `.claude/skills/pr-commented/SKILL.md`, `.claude/skills/pr-commented/reference.md`, `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/pr-ci-failed/reference.md`, `.claude/skills/main-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/reference.md` | 1 |
| 5 | Update the `ai-docs/` inventory pages — the Subagent-table row and its `design-review` neighbour, the Task/Design sync-group rows and the domain-invariant row (each keyed by file path), and the CAN-vs-MAY citation. | `ai-docs/claude-tools-hierarchy.md`, `ai-docs/propagation-groups.md`, `ai-docs/improve-eval-contract.md` | 1 |
| 6 | Rewrite Checklist O's severity rule to *one rule, any axis is `major`*: replace `:152`'s justification clause, strip `:174`'s severity presuppositions while leaving its procedural content intact, leave `:184` untouched, delete the `:186` paragraph, and add the worked example. | `.claude/skills/ai-audit/reference.md` | — |
| 7 | Record the decision as a new `KD` row in a new dated section: the rename, the severity flip, and a pointer to Checklist O for the reader-vs-parser argument. | `ai-docs/key-decisions.md` | 1, 6 |
| 8 | Closing concept-level re-sweep of the whole live tree against the § Scope membership criterion, with a per-site verdict recorded in the progress file's decisions log; fix any in-class site subtasks 1–7 missed, and report any OUT-of-class site deliberately left alone. | whole live tree (no new file expected) | 1–7 |

**Subtask 6's site coordinates, re-pinned.** The spec pins `:152` / `:174` / `:184` / `:186` at
`fe7c6c1`; they still land on the same sentences at this design's base
`[measured b448c45:.claude/skills/ai-audit/reference.md:152,174,184,186 · sed -n
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
`[measured b448c45:.claude/skills/ai-audit/reference.md:155 · sed -n '155p' → "The embedded
inventory is session state: the `Available agent types for the Agent tool` block and the `The
following skills are available` block the harness injects into the orchestrator's context, plus
the hook-event table the Step 2.1 `claude-code-guide` spawn returns"]` — a delegate's listings are
its own, so a delegate-side run measures the wrong session. The same placement also keeps the
`claude-code-guide` spawn the checklist's step 2 prescribes on the orchestrator side.

## Risks

- **R1 — the commit records a delete+add instead of a rename, failing AC2.** Git detects renames
  by similarity at diff time rather than storing them, and the default `-M` threshold is a
  similarity index over the file's size
  `[measured b448c45 · git diff --help | grep -A4 'find-renames' → "-M[<n>], --find-renames[=<n>]
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
- **R4 — AC3's glob names `ai-docs/**` and the run's own plan artefacts live there.** This spec,
  this design and the progress file all carry the literal `.claude/agents/design.md` while the run
  is in flight, and § Out of scope names only the `.state.md` sibling and `plans/done/**`.
  *Resolution, measured rather than asserted:* `/task` Step 12 `git mv`s the spec **and** the
  design into `ai-docs/plans/done/` and retires the progress file and the `.state.md` into
  `ai-docs/plans/ignored/`, which is gitignored
  `[measured b448c45:.claude/skills/task/SKILL.md · awk '/^### Step 12/,/^## /' → sub-step 4
  "`git mv` the spec and design files to `ai-docs/plans/done/`" and sub-step 9a "mv
  ai-docs/plans/<spec-base>.progress.md ai-docs/plans/ignored/"]`
  `[measured b448c45:.gitignore:22 · grep -n 'ignored' .gitignore → "/ai-docs/plans/ignored/"]`.
  On the terminal PR tree every one of those paths is either a named history surface or absent, so
  AC3 holds as written. *Consequence:* AC3's verification command is run **after** Step 12's moves,
  or with `ai-docs/plans/` pruned while the run is in flight — never against the mid-run tree.
- **R5 — the script-backed ACs are not locally reachable without a permission prompt.**
  `permissions.allow` grants no `Bash(bash *)`, no `Bash(python3 *)` and no `Bash(./**)`
  `[measured b448c45:.claude/settings.json · jq -r '.permissions.allow[]' → Edit(./**),
  Edit(.claude/**), Bash(go *), Bash(gofmt *), Bash(golangci-lint *), Bash(make *), Bash(git *),
  Bash(gh *), Bash(ast-index *), Bash(psql *), Bash(actionlint *), Bash(shellcheck *),
  Bash(grep *), Bash(rg *), Bash(jq *), Bash(awk *), Bash(sort *), Bash(wc *)]`, and `/task`'s own
  `allowed-tools` grants neither the guard scripts nor `comm`
  `[measured b448c45:.claude/skills/task/SKILL.md:6 · sed -n '6p' → the allowed-tools line, whose
  only script entry is `Bash(.claude/skills/task/scripts/append-task-run.sh *)`]`. *Decision:*
  **AC7, AC8 and AC12's script-dependent half are discharged by CI on the PR**, not locally. The
  *Harness guards* job runs the citation guard, the guard regression suites and the relative-link
  check
  `[measured b448c45:.github/workflows/ci.yml:148-175 · sed -n '148,175p' → the "citation
  namespaces resolve", "guard regression suites" and "relative markdown links resolve" steps]`,
  and it is reached on this diff because its `paths-filter` includes `.claude/**` and `ai-docs/**`
  `[measured b448c45:.github/workflows/ci.yml:46-50 · sed -n '46,50p' → "harness:" with
  '.claude/**', 'ai-docs/**', 'AGENTS.md', 'CLAUDE.md']`. A local run remains available at the
  cost of one approval prompt; the design does not assume it is free, and no permission grant is
  added. *Note for the verifier:* under `/ai-audit`'s own frontmatter — not under `/task`'s — the
  guard scripts and `comm` **are** granted
  `[measured b448c45:.claude/skills/ai-audit/SKILL.md:6 · sed -n '6p' → allowed-tools including
  Bash(comm *), Bash(.claude/skills/ai-audit/scripts/check-citations.sh),
  Bash(.claude/skills/ai-audit/scripts/test-check-citations.sh)]`, so an `/ai-audit` invocation is
  the unattended local route if one is wanted later.
- **R6 — the new name is not dispatchable in the session that creates it.** The set of
  `subagent_type` values is session state (spec § *Technical constraints*): after subtask 1 lands,
  the tree says `design-writer` while the running session still resolves `design`. *Mitigation:*
  no verification step dispatches the new name, and if `/task` re-enters Step 6 or the Design
  Amendment recipe **after** subtask 1, the orchestrator spawns the still-registered
  `subagent_type="design"` even though the edited files name `design-writer` — a temporary
  divergence between the tree and the live registry, not a defect and not a site to "fix".
- **R7 — the rewritten Checklist O prose trips the audit that owns it.** Checklist M sub-checks 2
  and 6 govern exactly the paragraph shape the rewrite produces (§ Approach). *Mitigation:* one
  bold-uppercase verb per non-table paragraph, and a demonstrator within eight lines of the
  contrast paragraph. `[derived → AC9, AC10, AC11]`
- **R8 — a `#N` citation added by the rewrite fails the citation guard.** Check (1) flags a bare
  `#N` above the repository's PR high-water mark. Citing this task's own issue is safe: the mark
  is at or above 16 `[measured b448c45 · gh pr list --state all --limit 1 --json number --jq
  '.[0].number // 0' → 16]` and the guard skips any `#N` at or below it
  `[measured b448c45:.claude/skills/ai-audit/scripts/check-citations.sh:84 · sed -n '84p' →
  '[ "$n" -le "$LOCAL_MAX" ] 2>/dev/null && continue']`. Any citation to the sibling projects must
  still carry its namespace. `[derived → AC8]`
- **R9 — a relative markdown link to the renamed file breaks (AC7).** Measured negative: no
  markdown link anywhere in the tree targets the file — every reference to it is inline code
  `[measured b448c45 · grep -rnE '\]\([^)]*design\.md' --include='*.md' . → no match anywhere in
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
`[measured b448c45:.github/workflows/ci.yml:152-159 · sed -n '152,159p' → the "guard regression
suites" step invoking those scripts]` — are re-run unchanged, and this task adds no case to
any of them `[derived → AC8]`.

Each command below is the one the orchestrator runs at Step 9 for that AC. Every claim in this
section is about a state this task will create, so each carries `[derived → …]`.

- **AC1 — the file and its frontmatter.** `ls .claude/agents/design-writer.md .claude/agents/design.md`
  (the first resolves, the second does not) and
  `awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/agents/design-writer.md`
  → `design-writer`, matching the basename. That awk program is Checklist O's own step-1 extractor
  `[measured b448c45:.claude/skills/ai-audit/reference.md:168 · sed -n '168p' → the recipe's
  "Subagent names:" bullet, carrying that exact awk one-liner over .claude/agents/*.md]`, so the
  same command discharges the spec's name-equals-basename constraint. `[derived → AC1]`
- **AC2 — rename continuity.** `git diff -M --summary <base>...HEAD -- .claude/agents/` must print
  a `rename .claude/agents/{design.md => design-writer.md} (NN%)` line and no separate
  delete/create pair for those paths. `[derived → AC2]`
- **AC3 — no live site carries the old path.** `grep -rn -F '.claude/agents/design.md' .claude/ ai-docs/ AGENTS.md CLAUDE.md .github/ Makefile`
  with the history surfaces pruned (`ai-docs/learnings.md`, `ai-docs/harness-gaps.md`,
  `ai-docs/plans/**`, `ai-docs/metrics/`, `ai-docs/deferred/`) → empty. Run it **after** Step 12's
  `done/` move per § Risks R4. `[derived → AC3]`
- **AC4 — no stale dispatch value.** `grep -rn -F 'subagent_type="design"' .` → empty (the closing
  quote already excludes `subagent_type="design-review"`), and
  `grep -rn -F 'subagent_type="design-review"' .claude/skills/task/reference.md` still resolves,
  proving the sibling's dispatch sites were not collaterally rewritten. `[derived → AC4]`
- **AC5 — the class, per-site.** `grep -rniw 'design' <live set>` re-read site by site against the
  IN- and OUT-of-class tables in the spec's § Scope, with the verdict for each recorded in the
  progress file's decisions log.
  A tally is not the evidence here — the recorded per-site judgement is. `[derived → AC5]`
- **AC6 — the inventory pages.** `grep -n 'design-writer' ai-docs/claude-tools-hierarchy.md ai-docs/propagation-groups.md`
  resolves in the Subagent table and in the Task/Design group rows, and
  `grep -nw 'design' ai-docs/claude-tools-hierarchy.md ai-docs/propagation-groups.md` shows no
  remaining occurrence whose referent is this Subagent. `[derived → AC6]`
- **AC7 — relative links.** The CI *Harness guards* "relative markdown links resolve" step is the
  authority (§ Risks R5). Locally it is the same inline `python3` program, which costs an approval
  prompt. `[derived → AC7]`
- **AC8 — citation namespaces.** CI's "citation namespaces resolve" and "guard regression suites"
  steps. `[derived → AC8]`
- **AC9 / AC10 / AC11 — the Checklist O rewrite.** Read the whole `## Checklist O` section and
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

- **Is AC3 meant to hold mid-run, or on the terminal tree?** This design resolves it as *terminal
  tree* and shows the resolution is mechanical (§ Risks R4). If `design-review` or the orchestrator
  reads AC3 as a mid-run invariant, that is a **Spec Amendment** — the spec's § Out of scope would
  have to name this run's own spec/design/progress artefacts alongside the `.state.md` sibling —
  and it routes through `spec-writer`, not through an edit here.
- **Does any project name other than `design` clash with an embedded name?** Unanswerable before
  AC12's run, since the embedded inventory is session state. Handled by the spec's § Deferred:
  report and file, do not fix.
