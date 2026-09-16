# `/ai-audit` reference

Static reference content extracted from `SKILL.md`. Loaded on demand when `/ai-audit` Phase 2 Step 2.3 hits a specific checklist letter, or when Step 2.6 sub-step 4 needs to re-verify cross-references anchor-aware.

The `SKILL.md` body keeps a collapsed table of every checklist letter + one-line purpose + the matching anchor below. Read the SKILL.md table first; jump here only for the detail body of a letter that surfaced a finding.

## Checklist A — Cross-reference integrity

- Every relative link (`../`, `./`, file references in prose) resolves to an existing file. Verify with `realpath` from the link's source directory or with `find`.
- Every `[text](file.md)` and bare `file.md` mentioned in instructions points to a file that exists.
- Every Skill or Subagent named in another Skill/Subagent (e.g., `project-review` references `review-findings`, `self-review`) actually has a matching file.

## Checklist B — Conflicting / duplicated rules

- The same topic must not have contradictory guidance in two places (e.g., commit policy in AGENTS.md vs. in a skill).
- Verbatim-duplicated rules across files → consolidate to one canonical home + reference.
- Rule says "see `<other-file>`" — confirm the target file actually contains that rule.

## Checklist C — Dead references

- Skills/agents named in `AGENTS.md` "Sync groups" must exist. (E.g., the AGENTS.md note about `task` ↔ `task-issue` collapse — verify no stale references remain.)
- Agent names referenced in skills must match a file under `.claude/agents/`.
- `ai-docs/plans/done/` references in Subagent checklists must still resolve.

## Checklist D — Frontmatter conformance (skills)

Per the official docs:

- Every `SKILL.md` has YAML frontmatter with at minimum `name` and `description`.
- `description` should make trigger conditions clear (when to invoke).
- `disable-model-invocation: true` ↔ skill is user-only — verify intent matches.
- `argument-hint` style is consistent across skills.
- `allowed-tools` syntax matches `ToolName(pattern)` form documented at `code.claude.com/docs/en/skills`.

## Checklist E — Frontmatter conformance (agents)

- Every `.claude/agents/*.md` starts with YAML frontmatter (`---`-delimited block at top of file). A file in `.claude/agents/` without frontmatter is not a Subagent — it's a stray document. Enumerate the directory at audit time rather than baking in a count.
- `name` field equals the file basename.
- `description` is one line and tells the orchestrator when to spawn this Subagent.

## Checklist F — Hooks (`.claude/settings.json`)

- Each hook event name is one of the documented set (`SessionStart`, `PreToolUse`, `PostToolUse`, etc. — confirm against `hooks-guide`).
- Matchers are valid tool name patterns.
- Hook commands fail closed (`exit 2` for blocking) where intended; non-blocking informational hooks use stderr without `exit 2`.
- Timeouts are reasonable (≤30s default, longer only when the work demands it).
- Commands quote `$CLAUDE_PROJECT_DIR` and other env vars correctly — no shell injection footguns.

## Checklist G — AGENTS.md "Propagation Rule" coherence

- Every "sync group" listed in AGENTS.md still has all listed members present and cross-referenced.
- Behaviors described in AGENTS.md and replicated in Subagent checklists agree (e.g., file-size hard/soft limits in `review-findings.md` match AGENTS.md).
- Exemptions in AGENTS.md appear in every enforcement file. **Resolve each named exemption against `AGENTS.md` before checking that it propagated** — this bullet shipped naming `examples/` / `benches/` no-test exemptions and a trait-impl doc-convention exemption, none of which has ever existed here (`grep -niE 'benches|examples/|trait.impl' AGENTS.md` → no match, in every commit); an auditor following it verified the propagation of an empty set and passed vacuously on every tree, the same always-green shape as the pre-rewrite Checklist O. The live Go-side exemptions are: the `file-limits` path prune in `Makefile` (the only escape from the 1000/1500 hard bands — `ai-docs/code-style.md` § *File size*), the `_test.go` 1500-vs-1000 band (`AGENTS.md` § *Code Style*), `main` may exit non-zero where libraries return errors (the panic rule), and `//nolint` carrying a named linter plus a stated reason (`nolintlint`).

## Checklist H — Documentation conformance pointers

- `ai-docs/doc-convention.md` is referenced by `review-findings.md` and `self-review.md`. Confirm the relative paths resolve.
- The canonical godoc contract-section order is `ai-docs/doc-convention.md § DOC-3 — Contract sections` (its sole authoritative home — AGENTS.md carries no duplicate order list). Confirm any file that restates the order agrees with DOC-3 exactly. The Review sync group (`self-review.md` § *Doc convention conformance*, `review-findings.md` § 6, `project-review/SKILL.md` step 6) is where restatements live; a Rust-flavoured restatement (`pub` items, `///` comments, `# Parameters` / `# Panics` / `# Safety` headings, `impl Trait for Type` exemptions) is import residue, not a convention — this project is Go and DOC-1…DOC-6 are its only sections.

## Checklist I — File-size & structure (instruction files)

- No `SKILL.md` or Subagent file exceeds ~500 lines without clear sectioning. Long files should split into a thin `SKILL.md` + reference file (the `improve` / `project-review` / `task` Skills use this pattern).
- Each skill directory contains exactly one `SKILL.md` (no extra markdown unless intentional reference material).

## Checklist J — Allow-list / permission consistency

- Tools used in skills' `allowed-tools` should be present (or coverable by) the `permissions.allow` list in `settings.json` — otherwise the user gets a prompt every time.
- Conversely: any allow-listed pattern that no skill actually uses is dead and should be reviewed.

## Checklist K — Skill-directory layout (SKILL.md + supporting files + scripts/)

Per the [Claude Code skill-directory pattern](https://code.claude.com/docs/en/skills#add-supporting-files), a skill directory may contain SKILL.md plus supporting files (reference docs loaded on demand, scripts the skill executes, examples Claude can read). Audit checks:

**Recording protocol (binding).** Run each of the three sub-checks below in its
**own** tool call — never two in one command block — and write its verdict line
into the run's findings record **before** starting the next one. The verdict
line is one of exactly two shapes:

- `K<n>: clear` — the check ran and nothing crossed its threshold.
- `K<n>: <count> over threshold → <count> findings` — followed immediately by the findings themselves.

Checklist K is not complete until all three lines exist. A sub-check whose
measurement ran but whose verdict line is absent counts as **not run**: re-run
it. Rationale, measured — in the `/ai-audit` run at `e97768c`, K1 and K2 shared
one command block; K2's heuristic was wrong and was re-run alone, and K1's
result (seven `SKILL.md` files over the 200-line threshold, `.claude/skills/task/SKILL.md`
among them at 293) was left in the discarded output and never became a finding.
The relief K1 owes never arrived, and the next `/task` collided with the byte
cap that extraction would have relieved. Fixing an instrument mid-checklist is
exactly when a completed measurement gets dropped.

1. **Oversized SKILL.md check.** Severity `minor`. Run `wc -l .claude/skills/*/SKILL.md` against the live tree, alone. This is the harness's routine size relief — since CI carries no size gate, K1 and Sub-check M9 are the only surfaces that ever measure an instruction file, and K1 is the one that fires before anything is over the cap. For each file > 200 lines, emit a `minor` finding: scan for *reference content* sections (format specs, parser rules, lookup tables, long checklists, embedded templates) — material that is referenced once or twice in the workflow but loaded into context on every invocation — and propose extraction to a supporting file (a `reference.md` sibling loaded on demand, as `improve` / `project-review` / `task` already do). lab-game keeps no skill-size exemption index; if an oversized SKILL.md is intentional, note the rationale inline in the finding rather than tracking it in a separate file.

2. **Multi-consumer supporting files belong in `ai-docs/templates/`.** When a supporting file is referenced from **>1 Skill or Subagent**, propose moving it from the owning Skill's directory to `ai-docs/templates/<file>.md` (per AGENTS.md *Agent Docs*). Single-consumer supporting files stay inside the owning Skill directory. Cross-references then point at `ai-docs/templates/` directly instead of routing through another Skill's body. Severity `minor`.

3. **Inline-script extraction candidates.** Identify `SKILL.md` sections containing **self-contained `bash` blocks** — a complete, executable recipe with at most one or two `<placeholder>` substitutions, NOT orchestration guidance that Claude reconstructs dynamically per call. For each, propose extraction to `.claude/skills/<skill>/scripts/<descriptive-name>.sh` **only when that skill is the script's sole caller**, invoked via the canonical `${CLAUDE_SKILL_DIR}/scripts/<name>.sh <args>` pattern; a script any other skill, agent, hook or CI step will call goes to `ai-docs/scripts/<name>.sh` instead (placement rule: `ai-docs/claude-tools-hierarchy.md` § Shell guards — eleven of thirteen files once sat under `ai-audit/scripts/` because it was the first directory that existed, not because `/ai-audit` ran them). **Sub-check, `minor`:** a file under `.claude/skills/<skill>/scripts/` that is referenced from outside that skill's directory by a live surface (`grep -rl` over `.claude/`, `AGENTS.md`, `ai-docs/*.md`, `.github/`; history surfaces excluded) belongs in `ai-docs/scripts/`. After extraction, narrow the skill's `allowed-tools` from per-command patterns to a single `Bash(.claude/skills/<skill>/scripts/<name>.sh *)` entry. Severity `minor`.

   **Counter-rule.** Bash snippets that are *orchestration guidance* — every call has different placeholder values that the Subagent constructs — are NOT script-extraction candidates. Forcing them into scripts requires the Subagent to call a helper for guidance it can express inline. Skip those.

## Checklist L — Learning-Log field coherence

When AGENTS.md § Learning Log's *Entry format* block lists a field maintained by `/improve` and `/ai-audit` (currently `Escalated?` and `Superseded by:`), verify each field is covered in **all four** mandatory locations:

| Location | Required content |
|---|---|
| AGENTS.md *Boundary rule 1 Exception* | Explicit authorization to edit the field in-place |
| AGENTS.md *Boundary rule 2 Exception* | Explicit authorization for the field's edits to coexist with instruction-file edits in the same `/improve` / `/ai-audit` turn |
| `.claude/agents/self-improve.md` Step 5 (Commit B backfill) | Workflow describing when and how `/improve` writes the field |
| `.claude/agents/learnings-escalation-audit.md` Steps 2/3/4 | Verification recipe + Category-1 drift fixes (including typo fixes within the field's value) |

A field added to the entry format without parallel coverage in all four targets → `major` finding (rules diverge across the surface; one of the two gates fails silently). The historical proof point: F1 in this PR — Boundary rule 2 Exception text lagged behind Boundary rule 1 Exception after the `Superseded by:` field landed, leaving Boundary rule 2 under-describing the contract.

**Declared-schema fields (no `/improve`-time mutation).** Some Entry-format fields are *declared* by AGENTS.md and *parsed* by the two agents but are **never** mutated by `/improve` or `/ai-audit` after the entry is written (currently: `Kind:`). For each such field, verify coverage in this analogous 4-location list:

| Location | Required content |
|---|---|
| AGENTS.md `## Learning Log` *Entry format* block | Declaration of the field name, allowed values, and default-when-omitted semantics |
| `.claude/agents/self-improve.md` Step 5 (Commit B backfill site) | Mention of the field where the workflow branches on its value (e.g., Step 5 / Step 6 routing on `Kind:`) |
| `.claude/agents/learnings-escalation-audit.md` Steps 2/3/4 | Parse-site usage of the field (verdict routing, sweep predicates) |
| `ai-docs/corrections-log.md` field glossary | Declaration mirror — same allowed-values list, same default-when-omitted note |

The Exception-body locations used by the `Escalated?` / `Superseded by:` rows do not literally apply since declared-schema fields have no `/improve`-time mutation. A declared-schema field added to the entry format without parallel coverage in all four locations above → `major` finding (the field's allowed values or default semantics drift across the surface; the two agents disagree on how to interpret it).

## Checklist M — `agent-writing-style.md` conformance

**Split out to [`checklist-m.md`](checklist-m.md)** — 11 sub-checks (Patterns 1–7 + Anti-patterns + Sub-checks 9/10 + Cross-shape verbs) over the audited corpus, plus the corpus enumeration. Moved because this page had grown past 35,000 bytes and Checklist M was a third of it; the checks themselves are unchanged. (The split predates the hysteresis AXIOM, under which that band is a normal working range, not a trigger.)

## Checklist N — Bidirectional `## Patterns` ↔ `Kind: validation` coherence

The carrot-side analog of Checklist C (dead references). Every promoted-from-validation carrot must round-trip in both directions:

**Forward direction.** Every `### N. <Name>` entry under a `## Patterns` section in the audited corpus (`.claude/skills/**/SKILL.md`, `.claude/agents/**.md`, `AGENTS.md`) whose **body uses carrot verbs** (`Default to` / `Prefer`) MUST back-link to at least one `Kind: validation` entry in `ai-docs/learnings.md`. Detection recipe: for each `### N. <Name>` block, grep its body for `Default to` or `Prefer`; if found, also grep for a `learnings.md` back-link (path + date-slug citation) within the same block. Carrot-verb present AND no back-link → flag.

**Forward-sweep carrier-vs-template exemption.** Entries WITHOUT carrot verbs (template scaffolding, non-promoted prose, structural placeholders) are out of scope for the forward sweep — the audit greps for carrot-verb presence within each `### N. <Name>` block **before** requiring a back-link. The named exempt source is `ai-docs/agent-writing-style.md § Patterns` (template source, not a promoted-from-validation carrier). Other `## Patterns` sections that grow in future PRs are subject to the same carrot-verb-presence filter — no further per-file exemptions.

**Reverse direction.** Every `Kind: validation` entry in `ai-docs/learnings.md` whose `Escalated?` ≠ `no` MUST have a corresponding `## Patterns` block in each named target file (Skill / Subagent / AGENTS.md). Detection recipe: parse each `Kind: validation` entry's `Escalated?` line; for each comma-separated target value, confirm the named file contains a `## Patterns` block AND that block contains an entry back-linking to this validation entry. Predicate gate: entries with `Escalated? no` are NOT subject to reverse-direction enforcement — only entries the operator has promoted (`Escalated? ≠ no`) require a paired `## Patterns` block.

Multi-target reverse direction. The `Escalated?` field may name multiple comma-separated targets (e.g., `skill:context-reset, AGENTS.md`). The reverse sweep iterates each value independently — a validation entry escalated to two targets must have a `## Patterns` block in BOTH files; missing in either flags.

| Direction | Trigger | Action |
|---|---|---|
| Forward | `### N. <Name>` block in `## Patterns` uses `Default to` / `Prefer` AND no `learnings.md` back-link in the same block | flag (severity `major`) |
| Forward (exemption) | `### N. <Name>` block has no carrot verb in its body | no flag (carrier-vs-template exemption) |
| Forward (named exempt source) | The audited file is `ai-docs/agent-writing-style.md` | no flag (template source) |
| Reverse | `Kind: validation` entry with `Escalated? ≠ no` AND named target file lacks a `## Patterns` block OR lacks a back-linking entry | flag (severity `major`) |
| Reverse (predicate gate) | `Kind: validation` entry with `Escalated? no` | no flag (not promoted; pattern block not required) |

Severity `major` — dead-reference class. The bidirectional shape mirrors Checklist C: every reference resolves AND every target has a back-reference.

## Checklist O — Embedded-name clash scan

Project-defined Subagent / Skill / Hook-event names MUST NOT clash with the **embedded** names the harness ships (Anthropic built-in agent types and skills, marketplace-plugin skills, harness hook events). What a clash costs is paid by the reader, not by the parser: one name now carries two definitions, so the model picks the wrong one or blends them, and it does that whether or not the two tokens ever reach the same dispatcher. One rule, therefore, and no axis exemption — any match → `major` finding with a rename recommendation; the project side renames, never the embedded name. A finding names the axis it crosses, and naming it does not lower the severity.

**Worked example — the clash that rewrote this rule (issue #10).** This project's design Subagent declared `name: design` while the harness ships an embedded `design` Skill. Nothing ever mis-resolved. The damage landed before any tool call was made:

| What the clash did not do | What it did |
|---|---|
| Send a call to the wrong destination — `Agent(subagent_type=…)` and `Skill(skill=…)` are separate tools with separate registries | Leave one name meaning two things, so the model reached for the Skill where the workflow wanted the Subagent, and blended the two contracts when it reached for the Subagent |

The project side was renamed to `design-writer`. The example is kept here because the rule above cannot be re-derived from how dispatch behaves — dispatch behaved correctly for as long as the clash stood, and a future auditor reasoning from resolvability alone would talk itself back out of the rule.

> **The embedded inventory is NOT in this repository — do not look for it here.**
> [`ai-docs/claude-tools-hierarchy.md`](../../../ai-docs/claude-tools-hierarchy.md) is the **project** inventory (this repo's own hooks, rules, Subagents, Skills, shell guards). Intersecting project names against it would match *everything*, not nothing. The embedded inventory is **session state**: the `Available agent types for the Agent tool` block and the `The following skills are available` block the harness injects into the orchestrator's context, plus the hook-event table the Step 2.1 `claude-code-guide` spawn returns.

> **AXIOM — an empty embedded list is `inconclusive`, NEVER `pass`.**
> `comm -12` against an empty right-hand side is empty for every possible left-hand side, so the check reports "baseline holds" on any tree whatsoever. Count the embedded list before trusting the intersection.
>
> | If the embedded list has... | Action |
> |---|---|
> | 0 entries | **STOP** — report Checklist O as `inconclusive` and say the listings were unavailable. Never record a pass. |
> | ≥ 1 entry | Proceed to the intersection |

**Recipe.**

1. **Project names** — every name the project DEFINES. Hook **matcher** values are NOT project-defined (they reference embedded Tool names) and are excluded:
   - Subagent names: `awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/agents/*.md` — the `FNR==1{f=0}` reset is load-bearing: it scopes the in-frontmatter flag to each file. Without it, a file with an odd number of `---` delimiters leaks state into the next file in the glob, silently dropping `name:` matches.
   - Skill names: `awk 'FNR==1{f=0} /^---$/{f=!f; next} f && /^name:/{print $2}' .claude/skills/*/SKILL.md` — same `FNR==1` per-file reset rationale.
   - Sort + dedupe → `project-names.txt`.
   - **Hook event keys are NOT project names.** `jq -r '.hooks | keys[]' .claude/settings.json` returns `SessionStart` / `PreToolUse` / … — harness event names the project *references*, and matching them is correct, not a clash. Feeding them into `project-names.txt` manufactures a false positive per configured event. They get the inverse check instead (step 4).
2. **Embedded names** — transcribe the agent-type and skill listings from session context, **minus** the project's own entries (registered project Subagents and Skills appear in those same listings, so they must be subtracted or every project name self-matches). Namespaced names like `ast-index:initialize-rust` count as ONE token; do NOT split on `:`. Sort + dedupe → `embedded-names.txt`.

   > **The subtraction blinds this list to the SAME-AXIS clash.** A project skill named `X` and an embedded skill named `X` collapse to a single listing row, and step 2 then subtracts it, so the intersection is empty precisely when that clash is present. The listing alone cannot distinguish *shadowed* from *absent*. For the same-axis sweep, get the built-in roster from the Step 2.1 `claude-code-guide` spawn (ask it for the built-in skill and agent-type names from `code.claude.com`) and intersect the project names against **that** roster, which no project registration can suppress. The session listing stays authoritative for the cross-axis sweep only.

   Separately, from the Step 2.1 `hooks-guide` table, write the hook event names to their own file → `embedded-events.txt` (step 4 consumes it; do NOT merge them into `embedded-names.txt`, or every configured hook event self-matches in step 3).
3. **Intersection** — `comm -12 <(sort -u project-names.txt) <(sort -u embedded-names.txt)` MUST return empty.
4. **Hook-event validity (the inverse check).** Every `hooks.<Event>` key MUST appear in the embedded hook-event set from the Step 2.1 `hooks-guide` table. A key that does not is a hook that never fires — silent, since the harness ignores an unknown event rather than erroring. Severity `major`: `comm -23 <(jq -r '.hooks|keys[]' .claude/settings.json | sort -u) <(sort -u embedded-events.txt)` MUST return empty.

| Trigger | Action |
|---|---|
| `embedded-names.txt` is empty | `inconclusive` — the instrument measured nothing; do not record a pass |
| `comm -12` output is empty (and the embedded list is non-empty) | no flag — clash-scan baseline holds |
| `comm -12` output is non-empty | `major` finding per name: *"Project-defined `<name>` clashes with embedded `<name>`. Rename the project side."* |

## Step 2.6 sub-step 4 — Cross-reference re-verification (anchor-aware)

For every relative link the audit touched in any `.claude/agents/*.md` or `.claude/skills/**/SKILL.md`, confirm the target file exists AND the anchor (if present) matches a heading slug. Use this anchor-aware check rather than naive `realpath -m` (which mistakes `#anchor` for part of the path).

> **Strip fenced blocks AND inline-code spans before matching, or the page audits its own examples.** A recipe that documents link syntax contains link syntax — in fences *and* in backticked prose. Without both strips, this very section reports its own illustrative `file.md` and `reference.md#anchor` as broken. That is why CI's link check carries a hard-coded `file.md` exclusion; stripping code spans is the general form of the same fix.

> **The pattern must match SAME-DIRECTORY links, not just `../` ones.** A `../`-anchored matcher (`\(\.\./…\)`) is the shape this recipe shipped with, and it is blind to `](reference.md#anchor)` — the dominant form inside a skill directory, where the thin-`SKILL.md` + `reference.md` layout puts almost every link. Measured on `.claude/skills/ai-audit/SKILL.md`: the `../` form matches **2 of 21** links. Match the markdown link syntax and filter by scheme instead.

```bash
for f in <changed-files>; do
  # strip fenced code blocks first, then filter schemes and <placeholder> targets
  awk '/^```/{fence=!fence; next} !fence' "$f" \
    | sed 's/`[^`]*`//g' \
    | grep -oE '\]\([^)]+\)' | sed 's/^](//; s/)$//' \
    | grep -vE '^(https?|mailto):' | grep -vF '<' | sort -u | while read -r path_with_anchor; do
    path=${path_with_anchor%%#*}
    anchor=${path_with_anchor#*#}; [ "$anchor" = "$path_with_anchor" ] && anchor=""
    src_dir=$(dirname "$f")
    # empty path = same-file anchor, e.g. ](#some-heading)
    if [ -z "$path" ]; then abs=$f; else abs=$(realpath -m "$src_dir/$path"); fi
    [ -e "$abs" ] || { echo "FILE MISSING: $f -> $path"; continue; }
    [ -z "$anchor" ] && continue
    # heading-slug match (GitHub algorithm): lowercase, strip to alnum/underscore/hyphen/space, spaces->hyphens; keep '_' and consecutive hyphens (GitHub does NOT drop underscores or collapse '--', so an em-dash heading like `A — B` slugs to `a--b`)
    awk '/^#{1,6}\s/{line=$0; gsub(/^#+\s+/,"",line); line=tolower(line); gsub(/[^a-z0-9 _-]/,"",line); gsub(/ /,"-",line); sub(/^-+/,"",line); sub(/-+$/,"",line); print line}' "$abs" | grep -Fx "$anchor" >/dev/null || echo "ANCHOR MISSING: $f -> $path#$anchor"
  done
done
```

See [`ai-docs/corrections-log.md`](../../../ai-docs/corrections-log.md) for the FORBIDDEN-reasoning anchor target referenced from the broader audit narrative.

## Checklist P — Cross-repo citation resolvability

Enforces one invariant: **every citation resolves for its reader.** A citation whose referent a reader cannot reach is not neutral decoration — but the failure is almost never a *fabricated* reference. It is an **unqualified** one.

**Why.** This harness was adapted from the sibling **quartzite** project (`maratik123/quartzite`) in commit `9077bfb`. `#N` is repo-relative by GitHub convention; `learnings.md <date>` / `feedback_*.md` are project-relative. The imported text never changed, but each referent silently rebound from a real quartzite object to a nonexistent local one — and `ai-docs/learnings.md` was created empty in that same commit, so its date citations pointed into nothing from the start.

> **AXIOM — QUALIFY, never drop.** A repo-scoped probe cannot falsify a cross-repo reference: a bare `gh pr view` resolves against **this** repo and reports *Could not resolve* for a real, MERGED `maratik123/quartzite` PR. That false negative reads as proof of fabrication and has already destroyed accurate provenance once. **Name the namespace, then re-probe.** Drop a citation only after `--repo`-qualified verification shows the referent genuinely does not exist.

**Recipe.** Run the script; it encodes all three probes. Then run its regression
test — the guard carries targeted exclusions, and the test is what keeps them
**content-addressed** instead of pinned to a line number that drifts.

```bash
.claude/skills/ai-audit/scripts/check-citations.sh
.claude/skills/ai-audit/scripts/test-check-citations.sh   # must stay green — every case it defines passes
```

> **Why the test is part of this checklist, not optional.** Exclusion by
> `file:line` has already rotted once: the check-(2) exclusion was pinned to
> `corrections-log.md:47`, an unrelated commit inserted rows above it, and the
> pin silently re-pointed at a dateless neighbour — turning the guard RED on a
> clean tree while looking like it still worked. Case 2 of the test shifts the
> excluded row and fails any fix that merely re-pins the number; case 3 fails
> any fix that over-corrects into skipping the whole file; case 4 catches a
> test run that mutates the target file's mode, which `git status` cannot see.
> **The test covers check (2) only.** Check (1)'s prose-specimen exclusion is
> content-addressed as well — a phrase match on the `task/reference.md`
> `entry_args` format demo. Of the `file:line` pair it replaced, the
> `spec-writer.md` pin had already drifted onto an empty line (and its specimen
> is covered by the anchored template-field rule regardless), while the
> `task/reference.md` pin was still accurate — the phrase match has been in
> place since the learning-loop import, and the commit that rewrote this
> paragraph moved that line by 57 rows, which would have broken a pin had the
> code still carried one. But
> **no test case exercises the phrase match**: if you touch it, add a case that
> shifts the demo row before trusting a green run; do not re-pin.

| Probe | Fires on |
|---|---|
| 1 | An unqualified `#N` — **bare or `PR #N`** — whose `N` exceeds this repo's live high-water mark (`gh pr list --state all --limit 1`). Bare `#N` is the same defect; a `PR #N`-only matcher misses it (that blind spot shipped once, in self-review Round 1). |
| 2 | A `learnings.md` date outside this log's range, on a line carrying a `see`/`entry`/`validated`/`recurrence`/`added` cue. Five cues — `added` catches the `(added <date>)` parenthetical shape. |
| 3 | A `feedback_*.md` cited without the namespace that owns it. |

Any hit → **`major`**. Fix by qualification: `PR #295` → `maratik123/quartzite#295`; `` `ai-docs/learnings.md` 2026-05-19 `` → quartzite's `` `ai-docs/learnings.md` `` 2026-05-19; a bare memory filename → its full `~/.claude/projects/<encoded>/memory/…` path.

**Do NOT replace the script with a substring hook.** It must tell a **citation** from an **example** (an illustrative `YYYY-MM-DD-slug.spec.md` filename asserts nothing) and a **use** from a **mention** (prose naming a quartzite PR in order to retire its claim). A blanket matcher over-fires on both; the script's cue-word, field-name, and filename-shape filters exist for that reason, and it excludes `ai-docs/bugfix/**` (traces discuss citations by nature).

**Self-test before trusting a PASS.** A green run proves nothing until it has failed for the right reason. Append a **bare** ref — no `PR` prefix — numbered far above the high-water mark, confirm RED, then restore. The bare form is deliberate: a `PR`-prefixed self-test would still pass if someone narrowed probe 1 back to `PR #N` — a self-test that inherits the aim of its phrasing cannot detect the aim being wrong.

**A fifth shape exists, currently clean: bare commit SHAs** (`9077bfb` is as namespace-relative as a bare issue ref). No probe is wired — every SHA in scope resolves locally (`git cat-file -t` → `commit`). If one ever fails, add it: `git cat-file -t <sha>` is exact — no substrings, cue words, or skips — and cheaper than probes 1–3.

**This page is the hardest to keep green.** Any text teaching *"do not write unqualified refs"* is under constant pressure to show one, so it is the highest-density false-positive source for its own script — inherent, not carelessness (the guard flagged this page five times during the fix that wrote it, twice while closing the prior finding). The discipline: **describe the bad shape; spell it only alongside its fix.** Exactly one specimen above is spelled — the `Fix by qualification` row — beside its qualified form on one line, which keeps it green. Re-run the script after editing this page; do not assume prose is safe. (Validated as a keep-doing pattern in **graphite-gp**'s `learnings.md`, 2026-07-17, *a pattern-matching guard's own documentation is the highest-density false-positive source for that guard*; generalises to any lint/guard whose reference page must exhibit the shape the guard forbids.)
