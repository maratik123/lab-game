# Agent Writing Style

**Audience:** anyone editing instruction files (`AGENTS.md`,
`.claude/skills/**`, `.claude/agents/**`, `ai-docs/code-style.md`,
`ai-docs/doc-convention.md`).

**Goal:** write binary rules so that both Opus 4.7 and Sonnet 4.6 reading the
same paragraph land on the same interpretation.

> **AXIOM — Apply this style only to binary rules.**
> Visual fail-loud emphasis is a tool for unambiguous rules where misreads
> have caused real failures. Applying it to ordinary prose devalues every
> emphasis token in the file — readers tune out the YELLED RULES because
> they're everywhere.
>
> | If a paragraph is... | Style |
> |---|---|
> | A binary rule (do X / never do Y / exactly one of A or B) | Fail-loud (patterns below) |
> | A documented misread spot (`Escalated?` target in `learnings.md`) | Fail-loud |
> | A multi-axis decision tree | Fail-loud + action table |
> | Rationale, context, examples, links | Ordinary prose |

## Patterns

### 1. AXIOM blockquote (top-of-section)

For sections governed by one binary rule:

> **AXIOM — \<name\>.**
> \<single-sentence rule.\> \<optional why-it-matters sentence.\>
>
> | If you see... | Action |
> |---|---|
> | \<condition\> | \<action\> |

Place immediately after the `## heading`, before any nuanced rules. The
action table is what makes the rule mechanical.

Example: `ai-docs/go-api-naming.md § The …Unchecked AXIOM`.

### 2. Fail-loud verbs (bold uppercase)

| Use... | For |
|---|---|
| **NEVER** / **MUST NOT** / **FORBIDDEN** | Prohibitions |
| **ALWAYS** / **MUST** | Required actions |
| **STOP** | Halt-the-workflow signals |
| **REJECT** | Code-review verdicts |
| **REMOVE** / **REPLACE** / **DELETE** | Corrective actions |

At most one bold-uppercase verb per paragraph. If a paragraph carries three,
it is no longer a rule — it is noise.

### 3. Action tables ("if you see X → do Y")

For mechanical decisions:

| If you see... | Action |
|---|---|
| A function carrying the `Unchecked` suffix whose doc comment does not name the guard that guarantees its precondition | **REPLACE** the comment with one that names the guarantor, or **DELETE** the unchecked variant |

Left column lists conditions an editor or reviewer would recognise (visible
markers, file paths, error messages). Right column lists the action verbatim.
Do not bury the action in prose around the table.

### 4. Explicit file lists, never globs

In fail-loud blocks, enumerate each path:

> When writing to `ai-docs/learnings.md`, you **MUST NOT** also edit any of
> these files in the same turn:
>
> - `AGENTS.md`
> - `CLAUDE.md`
> - `.claude/skills/**` (any file under this directory)
> - `.claude/agents/**` (any file under this directory)
> - `.claude/settings.json`
> - `ai-docs/code-style.md`
> - `ai-docs/doc-convention.md`

A glob inside one bullet (with parenthetical "any file under this directory")
is fine. A glob *as the entire list* is not — readers expand globs
differently.

### 5. Numbered enumeration of triggers

For "this fires when..." or "this is authorised by...":

> Project-level escalation happens only when:
>
> 1. The user runs `/improve`, OR
> 2. The user explicitly asks ("escalate this", "update AGENTS.md", etc.).

State the OR/AND connector at the end of each item, consistently. Consistent
placement defeats nesting ambiguity.

### 6. Concrete do/not examples

For non-trivial rules, show both shapes:

> **Do this:**
> ```go
> if err := rows.Err(); err != nil {
>     return fmt.Errorf("scan discovered nodes: %w", err)
> }
> ```
>
> **NOT this:**
> ```go
> // rows.Err() never checked — a truncated result set reads as success
> return nodes, nil
> ```

Examples remove ambiguity faster than prose.

### 7. Compaction recovery callout

The callout exists because Sonnet-mode sessions in the Claude Code harness auto-compact when input approaches the 180k-token ceiling and risk losing intermediate reasoning mid-flow. Read the four numbers behind that rule as: **1M tokens** = Sonnet base-model context window (what the model can hold in principle); **200k tokens** = Claude Code harness session cap when Sonnet is the active model — the tighter, harness-imposed budget that actually fires the auto-compaction; **180k input + 20k output** = the harness's split of that 200k cap, with auto-compaction triggered as the input side approaches the 180k ceiling. The Opus-vs-Sonnet distinction is load-bearing: Opus is NOT auto-compacted in the same way and therefore does not need a callout. Every code-side orchestrator skill (the dual-model ones that run on Sonnet) carries this callout at the top of its `SKILL.md` body in one of three per-skill variants. Opus-mode skills (enumerated under `## Out of scope` below) do not need it and do not carry it.

| Variant | Probe shape | Distinguishing phrase (verbatim) | Skills using it |
|---|---|---|---|
| A | Preamble-glob — the skill's `⚡ First` (or equivalent) preamble runs a `ls <glob>` probe that doubles as path discovery and RESUME/fresh routing | `"Locate the durable-state file via this skill's active-state probe"` | `/task`, `/project-review`, `/pr-commented`, `/main-ci-failed`, `/pr-ci-failed`, `/dependabot-pr` |
| B | Fixed-glob single in-flight artefact — the durable state is a single named file under a known directory | `"If exactly one in-flight artefact exists"` | `/bugfix`, `/interview` |
| C | Parent-routing — the skill has no own durable surface; it inherits whichever parent skill is active | `"Identify the **parent workflow**"` | `/context-reset` (also the canonical cross-link target) |

Variants A and B share the invariant phrase `"re-enter this skill from the top of its body"`. Variant C uses the equivalent phrasing `"Run the parent skill's own compaction-recovery callout"`.

The canonical cross-link target — every callout body ends with a `See ... § Compaction recovery (re-entry)` link — is the singular h2 `## Compaction recovery (re-entry)` in `.claude/skills/context-reset/SKILL.md`. The locked full bodies for all three variants are the wording the code-side skill files carry verbatim; copy the live body from a sibling skill that already uses the variant rather than re-deriving it.

**Variant A — trimmed example** (see `.claude/skills/task/SKILL.md` for the full body):

> **⚡ Compaction recovery check — read FIRST on every invocation.**
> If you are re-entering this skill after auto-compaction […], STOP
> before any tool call and:
>
> 1. **Locate the durable-state file via this skill's active-state probe**
>    — run the preamble glob (`ls <skill-specific>.progress.md 2>/dev/null`)
>    and apply the validation it documents.
> 2. Read the matched file top-to-bottom in one pass — the recorded
>    `current_step` is a cross-check, never an instruction to skip the read.
> 3. Then re-enter this skill from the top of its body — let the
>    preamble's probe / validation / RESUME sequence route control.
>
> See `.claude/skills/context-reset/SKILL.md` § **Compaction recovery
> (re-entry)** for the canonical handoff rationale.

When adding the callout to a new code-side skill, pick the variant matching the skill's durable-state shape; copy the live full body from a sibling that already uses that variant; do not invent a 4th variant without first updating this section and the cross-link target.

### 8. 40k byte-cap on instruction files, with hysteresis

Source-of-truth AXIOM lives in `AGENTS.md § Build & Test`. Pattern 8 is the
style-guide-side restatement so the rule is discoverable from the writing
conventions reference and audit-able via `/ai-audit` Phase 2 Checklist M.

> **AXIOM — Every covered instruction file stays under 40,000 bytes, and the band below the cap is a working range, not a warning.**
> Measured by `wc -c` — **bytes, not characters**: a character count under-counts multibyte punctuation (every `—`, `§`, `≥` in these files is 2–3 bytes), so a char-based reading silently reports a file as smaller than the gate sees it. The harness applies a soft cap on per-invocation instruction-file load; crossing 40,000 bytes imposes measurable per-invocation cost on every Subagent spawn and Skill invocation.
>
> | If `wc -c <file>` reports... | Action |
> |---|---|
> | `≥ 40,000` bytes | **Gate.** The growing flow stops; the next `/ai-audit` run owns an extraction pass whose single postcondition is the file landing **below 35,000** — restoring the growth budget. CI is red on this. Extract verbose subsections into `ai-docs/<topic>.md` reference pages with anchored links from the source file. |
> | `35,000–39,999` bytes | **Normal working range.** No flow reports it, no task constrains itself by it, no AC names a byte ceiling below the hard cap. |
> | `< 35,000` bytes | Post-`/ai-audit` state; full growth budget. |

**Covered file set** (enumerate verbatim; no glob-as-the-entire-list per Pattern 4):

- `AGENTS.md`
- `CLAUDE.md`
- `.claude/skills/**/*.md` (every markdown file under this directory — `SKILL.md` + `reference.md` siblings)
- `.claude/agents/**.md` (every file under this directory)
- `.claude/rules/*.md` (flat — `.claude/rules/` has no subdirectories today)
- `ai-docs/code-style.md`
- `ai-docs/doc-convention.md`
- `ai-docs/context.md`
- `ai-docs/agent-writing-style.md`
- `ai-docs/corrections-log.md`

**Per-commit invariant.** The cap binds at every commit boundary on a
feature branch — not just at merge time. A commit that introduces a
transient violation (e.g., adds 4,000 bytes to a 38,000-byte file, then a
later commit on the same branch extracts the content back out) is still a
violation. Stage the extraction in the same commit as the addition, or
sequence the extraction commit first.

**Extraction model.** The canonical extraction pattern for `AGENTS.md`:
verbose subsections moved into `ai-docs/<topic>.md` reference pages with
anchored links from the source file. Apply the same model when a covered
file reaches the 40,000-byte gate — the extraction's postcondition is
landing it below 35,000, not merely back under the cap.

## Writing checklist

Before submitting a rule paragraph, check:

- [ ] Single declarative sentence? (Three nested clauses → split.)
- [ ] Verb at the start? ("Append to..." beats "It is the case that...")
- [ ] Every condition explicit? (No "usually" / "the obvious case".)
- [ ] File paths spelled out? (Not "the usual file".)
- [ ] Says what to DO, not only what NOT to do?
- [ ] Action verb present? (Not just "this is forbidden".)

## Anti-patterns

| Anti-pattern | Why bad | Fix |
|---|---|---|
| Every paragraph in caps | Emphasis devalued | Reserve fail-loud for binary rules |
| AXIOM blockquote without action table | States rule but not application | Always include "if X → do Y" table |
| Implicit conditions ("usually") | Models infer differently | State explicitly |
| Globs as the entire fail-loud list | Reader expansion varies | Enumerate files |
| Rule + rationale + example + edge case in one paragraph | Rule lost in prose | Split: rule (fail-loud), rationale (prose), example (do/not), edge case (sub-bullet) |
| Negative-only rules ("don't X") | Reader doesn't know correct action | Pair with "do Y instead" |

## Citation in PRs

When a PR adds or modifies a fail-loud section, cite this doc in the body:

> Per `ai-docs/agent-writing-style.md` § Pattern 1, this PR adds an AXIOM
> blockquote to `<file> § <section>`.

Citing this doc grounds the stylistic choice in the agreed convention rather
than per-PR taste.

## Enforcement

PR-side citation (above) relies on the author remembering. The
**reverse-direction audit** — sweeping downstream files for drift against the
7 Patterns + Anti-patterns table — lives in `/ai-audit`'s Phase 2 Checklist M.
That checklist lives at `.claude/skills/ai-audit/checklist-m.md` and lands with the
audit skill — it carries the 11 sub-checks (Patterns 1–7 + Anti-patterns + Sub-checks 9/10 + Cross-shape verbs) and
the severity assignments. Run `/ai-audit` after any PR that touches a fail-loud
section if you want a mechanical conformance check on the corpus.

The forward direction (style-guide edits fan out to downstream consumers) is
already covered by the next section, `## Propagation rule for new patterns`,
which instructs editors to grep `.claude/agents/` + `.claude/skills/` for
files affected by a new or amended Pattern entry.

## Propagation rule for new patterns

When a new fail-loud pattern entry is added under `## Patterns` (or an
existing entry is amended), the change must fan out to every downstream
consumer that carries — or should carry — the pattern:

- Every `.claude/skills/**/SKILL.md`
- Every `.claude/agents/**.md`

The style guide names the *shape*; the downstream consumers carry the
*body*.

**Procedure.** After adding or amending a pattern entry, run:

```
grep -rni "<pattern-keyword>" .claude/agents/ .claude/skills/
```

…to find any file already half-using the pattern and reconcile. Pattern 7
*Compaction recovery callout* is the introducing case — see § Pattern 7
for the variant taxonomy.

This section is the `ai-docs/agent-writing-style.md` sub-rule of
`AGENTS.md` § *Propagation Rule*; AGENTS.md keeps a one-line stub row
pointing here.

## Out of scope

This file does not govern:

- Files for Opus-only readers: agents with `model: opus` frontmatter
  (`design`, `design-review`, `learnings-escalation-audit`, `self-improve`)
  and Opus-mode skills (`/ai-audit`, `/improve`)
- Go source documentation (covered by `ai-docs/doc-convention.md`)
- Project context (covered by `ai-docs/context.md`)

For those readers, AXIOM/fail-loud styling is optional — the reader has the
context to disambiguate without it.
