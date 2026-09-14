# Checklist M — `agent-writing-style.md` conformance

> Split out of [`reference.md`](reference.md) — that page had reached 36,054 bytes and this section was a third of it; the split was made under the pre-hysteresis rule, which treated that band as a warning. Under the current AXIOM the band is a normal working range and would not by itself have forced the split. Linked from [`SKILL.md`](SKILL.md) checklist row M.


`ai-docs/agent-writing-style.md` is the canonical style reference for fail-loud rules in instruction files. Checklist M sweeps the audited corpus for drift against the 7 Patterns + Anti-patterns table. **Audited corpus** (named inline; do NOT defer to Step 2.2's inventory which omits some of these): `AGENTS.md` + every `.claude/skills/**/SKILL.md` + every `.claude/agents/**.md` + `ai-docs/code-style.md` + `ai-docs/doc-convention.md` + `ai-docs/agent-writing-style.md` + `ai-docs/corrections-log.md` + `.claude/rules/**/*.md`.

| # | Sub-check | Detection mechanism | Severity |
|---|---|---|---|
| 1 | **Pattern 1 (AXIOM blockquote)** — every `> **AXIOM —`-prefixed block must be followed by an action table within the same blockquote. | For each match of `rg -n '^> \*\*AXIOM —'`, read up to 30 lines following the AXIOM line (the blockquote body is usually `> `-prefixed but the window is a fixed 30-line lookahead — do NOT terminate early on a `>`-only separator line, which is a valid blockquote-internal line). If no `> \|` table row appears within the window, flag the AXIOM line. **Exemption — the extracted table.** Checklists I and K actively prescribe thin `SKILL.md` + `reference.md`; an AXIOM whose action table was moved out under that pattern carries a pointer instead. Before flagging, scan **the AXIOM's own blockquote** — every line from the AXIOM line up to the first line that is neither `> `-prefixed nor a bare `>` (a bare `>` is blockquote-internal, so do NOT terminate on it — the same early-termination this cell warns about above; and NOT the fixed 30-line window, which crosses heading boundaries and can reach an unrelated fence under the next `###`) — for a § pointer to the page that carries the extracted table (a skill's `reference.md` / `checklist-m.md` sibling — or, for `AGENTS.md`, whose sections extract to `ai-docs/` pages and whose self-review matrix lives in the Subagent that applies it, the `ai-docs/*.md` or `.claude/agents/*.md` page the blockquote itself names) or a `> `-prefixed fenced shape contract. If either is present the table is extracted, not missing, and the AXIOM does NOT flag; flagging it would penalise the extraction the audit itself demands. | `major` |
| 2 | **Pattern 2 (fail-loud verbs)** — at most one bold-uppercase verb per paragraph (`**NEVER**` / `**MUST**` / `**MUST NOT**` / `**FORBIDDEN**` / `**STOP**` / `**REJECT**` / `**REMOVE**` / `**REPLACE**` / `**DELETE**` / `**ALWAYS**`). | **Pre-filter table rows** (lines matching `^\|` or `^> \|` — markdown table rows or blockquoted table rows). Tables are governed by Pattern 3, not Pattern 2; each row is its own action-verb scope. After the pre-filter, `awk` splits the remaining content on blank lines into paragraph chunks. For each chunk, count matches of `\*\*(NEVER\|MUST\|MUST NOT\|FORBIDDEN\|STOP\|REJECT\|REMOVE\|REPLACE\|DELETE\|ALWAYS)\*\*`. If count > 1, flag the paragraph (file:start-line). | `minor` |
| 3 | **Pattern 3 (action tables)** — the right column of every `\| If you see... \| Action \|` (or analogous) table must start with an action verb (imperative form), NOT prose narrative. | For each `\| If you see` table row, extract the right-column cell. Heuristic: starts with one of `Run`, `Apply`, `Stop`, `Add`, `Remove`, `Edit`, `Confirm`, `Bail`, `STOP`, `**NEVER**`, `**MUST**`, etc. OR a backtick-quoted command. If the cell starts with prose narrative (e.g., `"This is..."`, `"Usually..."`), flag the row. | `minor` |
| 4 | **Pattern 4 (explicit file lists, never globs)** — fail-loud lists that enumerate files must spell out each path; no glob-as-the-entire-list. | For each fail-loud block (paragraph containing a Pattern 2 verb), scan immediate `- ` or `* ` bullet list. If the entire list reduces to one or two globs (`.claude/**`, `**/*.go`) with no specific paths, flag. (Per-bullet parenthetical globs like `.claude/skills/** (any file under this directory)` are acceptable.) | `major` |
| 5 | **Pattern 5 (numbered enumeration of triggers)** — OR/AND connector placement must be consistent across items. | For each numbered list (`^1\.`, `^2\.`) inside a fail-loud block, check that EITHER every non-last item ends in `, OR` (or `, AND`), OR no items carry the connector. Mixed placement (some items connector-suffixed, some not) → flag. | `nit` |
| 6 | **Pattern 6 (do/not examples for non-trivial rules)** — paragraphs that articulate a contrast between two shapes must demonstrate both shapes. | **Tightened heuristic** (per design-review note 4 on the `maratik123/quartzite#369` PR — `not`/`NOT` alone are too noisy, firing on every "do not" / "must not" / "is not" paragraph): trigger iff the paragraph contains **BOTH** (a) a Pattern 2 fail-loud verb AND (b) one of the stronger contrast markers `instead` / `wrong` / `correct` / `forbidden`. Then check if a fenced code block OR a two-column `\| Do this \| NOT this \|` table follows within 8 lines. If both triggers fire AND no example follows, flag the paragraph. (Words `not` / `NOT` / `right` / `bad` / `good` are NOT in the trigger list — they produce false positives at unacceptable scale.) | `nit` |
| 7 | **Pattern 7 (compaction recovery callout)** — every callout-carrying skill must carry exactly one of the three locked variant-distinguishing phrases. | **Drive off the live grep, NOT the style guide table.** For each `.claude/skills/*/SKILL.md` whose body contains the literal string `Compaction recovery check`, run `rg -F` against the three variant-distinguishing phrases (verbatim, as carried by the code-side skill files that use each variant): Variant A = `"Locate the durable-state file via this skill's active-state probe"`; Variant B = `"If exactly one in-flight artefact exists"`; Variant C = `"Identify the **parent workflow**"`. If a callout-carrying skill contains zero or > 1 of the phrases → flag (likely invented 4th variant OR Variant-A/B/C drift). Also flag any callout-carrying skill not enumerated in the style guide Pattern 7 table (`ai-docs/agent-writing-style.md` § *7. Compaction recovery callout*, the `| Variant | Probe shape |` table — locate it by heading, never by line number) (style guide drift; the table should grow when a new skill onboards the callout). | `major` |
| 8 | **Anti-patterns table audit** — no row of the Anti-patterns table (`ai-docs/agent-writing-style.md` § *Anti-patterns* — locate it by heading, never by line number) should appear verbatim as a positive rule anywhere in the audited corpus. | For each anti-pattern row's left-column text (e.g., `"Every paragraph in caps"`, `"AXIOM blockquote without action table"`), grep the audited corpus for matches NOT inside the style guide itself. Flag matches. | `major` |
| 9 | **File-size AXIOM conformance** — every covered instruction file must stay under the 40,000-**byte** hard cap; 35,000–39,999 is the normal working range and is **not** reported. Rule-of-truth: § *Sub-check 9* of this page, which is the only file in the harness that states a threshold. | Run the verbatim `wc -c` invocation below against the covered file set, apply the three-band severity table. See § *Sub-check 9 — file-size AXIOM conformance* below for the AXIOM itself, the recipe, the severity bands and the covered file set. | see body |
| 10 | **Style-guide audit coverage map** — every `## ` (level-2) heading in `ai-docs/agent-writing-style.md` must map to either an existing Checklist M sub-check or to the explicit exclusion list of non-rule-bearing meta-sections. Unmapped headings produce `nit` "audit coverage gap" findings. | Parse ATX `## ` headings from the **live** `ai-docs/agent-writing-style.md` (re-grep at audit time; do NOT use a baked-in snapshot). Apply the inline coverage map below. See § *Sub-check 10 — style-guide audit coverage map* below for the parser recipe + map + finding format. | `nit` |
| 11 | **Cross-shape verbs** — carrot-shaped rules (entries in a `## Patterns` section) MUST NOT use stick verbs; stick-shaped rules (AGENTS.md AXIOM blockquotes or fail-loud bodies) MUST NOT use carrot verbs. The verb asymmetry IS the asymmetric-promotion contract — a wrong-shape verb either underweights a real obligation or locks in a brittle default as a hard rule. | (a) **Carrot block with stick verb:** for each `### N. <Name>` entry under a `## Patterns` section in the audited corpus, grep the entry body for `**MUST**` / `**NEVER**` / `**MUST NOT**` / `**FORBIDDEN**` — any match flags the entry. **Named exempt source.** `ai-docs/agent-writing-style.md` is the template-source style guide and is exempt from the (a) sweep — Patterns inside it legitimately *describe* stick verbs (Pattern 2 verb-table self-description) and *quote* stick-rule examples (Pattern 4 quoting Boundary rule 2). This mirrors Checklist N's *Forward (named exempt source)* row. (b) **Stick block with carrot verb:** for each `> **AXIOM —` blockquote (and its action-table body) outside `## Patterns` sections, grep for `Default to` / `Prefer` — any match flags the blockquote. Both directions flagged at the same severity. The detection cross-checks the Kind shape (Patterns block ↔ Kind: validation entry; AXIOM block ↔ Kind: correction entry) against the verb pattern. | `major` |

After running Checklist M, surface findings using the same severity-driven apply-or-ask pattern as Checklists A–L (Step 2.5). Pattern 6 noise-management fallback: if AC5's demonstrator run shows > 50% false-positive rate on Pattern 6 findings, record the rate and tighten the heuristic in a follow-up `/improve` cycle (the heuristic itself is encoded here, not in a separate config file — design choice to keep the audit self-contained).

### Sub-check 9 — file-size AXIOM conformance

> **AXIOM — Every covered instruction file stays under 40,000 bytes, and `/ai-audit` is the only surface that knows the figure.**
> Measured by `wc -c` — **bytes, not characters**: a character count under-counts multibyte punctuation (every `—`, `§`, `≥` in these files is 2–3 bytes), so a char-based reading silently reports a file as smaller than the gate sees it. Crossing 40,000 bytes imposes measurable per-invocation cost on every Subagent spawn and Skill invocation.
>
> | Who | Obligation |
> |---|---|
> | This sub-check (M9) | Make extraction mandatory at `≥ 40,000` bytes; the postcondition is the file landing **below 35,000**, not merely back under the cap. |
> | Checklist K1 ([`reference.md`](reference.md)) | Propose extraction for every `SKILL.md` over 200 lines, every pass — the routine relief that keeps files away from the gate. |
> | Every other flow — `/task`, `/interview`, `/bugfix`, `/improve`, both reviewers, CI | **FORBIDDEN** to measure, report or plan around instruction-file size. No spec constraint, no AC, no design risk row, no review finding may name a file size or byte budget, at any threshold. A flow whose edits push a file past a threshold ships anyway and says nothing; the next `/ai-audit` extracts. |

**The figures live here and nowhere else, and that placement is the enforcement.** They also stood in `AGENTS.md` § *Build & Test* and among the Patterns of `ai-docs/agent-writing-style.md` until forge-9 removed both. `CLAUDE.md` is `@AGENTS.md`, so the thresholds and the `wc -c` command sat in the context window of every session — which is the audience the third row forbids to use them — and a `/task` orchestrator duly assembled a size grep out of those very figures and reported a verdict about this AXIOM from a file the AXIOM does not cover (`ai-docs/learnings.md` 2026-09-03; `ai-docs/harness-gaps.md` 2026-09-03). A prohibition does not need to carry the number it prohibits knowing; enforcement does, and enforcement is here.

Detection mechanism. Run this verbatim invocation:

```bash
{ find AGENTS.md CLAUDE.md .claude/rules .claude/agents .claude/skills -name '*.md';
  printf '%s\n' ai-docs/code-style.md ai-docs/doc-convention.md \
                 ai-docs/context.md ai-docs/agent-writing-style.md \
                 ai-docs/corrections-log.md; } | xargs wc -c
```

Apply the three-band severity table to every reported size. `wc -c` reports
**bytes**; do not restate the numbers as "chars" — multibyte punctuation makes
a character count read lower than the gate does.

| Reported size (bytes) | Finding | Severity |
|---|---|---|
| `< 35,000` | none | — |
| `35,000–39,999` | none — **normal working range**; do NOT report it. No flow outside this audit may name a file size at all, so there is no AC or spec constraint here to police | — |
| `≥ 40,000` | `<path>: <count> bytes — AXIOM violation (≥ 40,000)`; the extraction pass this run owns must land the file **below 35,000** | `major` |

**Covered file set** — enumerated, never a glob-as-the-entire-list, so a static reader sees the set the shell recipe above consumes:

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

This list and the recipe are one surface: a change to either updates the other in the same PR per the Propagation Rule. There is no upstream copy to defer to. The size-measurement `PreToolUse` hook in `.claude/settings.json` carries the covered set as a path pattern, and this recipe and K1's command as its two verbatim exemptions — a change to any of the three updates the hook and `ai-docs/scripts/test-size-measure-guard.sh` in the same PR.

**Extraction model.** The canonical pattern for `AGENTS.md`: verbose subsections move into `ai-docs/<topic>.md` reference pages with anchored links from the source file. `/ai-audit` applies the same model in both its extraction passes — K1's routine proposal and M9's mandatory one.

The recipe is `find`-based, not glob-based, and that is load-bearing: a `**` glob resolves only one level deep without `globstar`, so a future `.claude/skills/<skill>/<sub>/*.md` would silently leave the corpus. Pattern 4's explicit-path requirement applies to the *covered-file list* above (so static readers see the set), not to the shell command that consumes it.

Sub-check 9 is the **only** enforcement surface for the byte cap. `.github/workflows/ci.yml` carried a mechanical `≥ 40,000` gate over this corpus until forge-4 retired it: a red PR is a byte budget by another name, and it forced every `/task` to plan around sizes the AXIOM forbids it to know. Nothing else measures now — if this sub-check does not run, or runs and does not record a verdict, the cap is unenforced for that pass. A `PreToolUse` hook refuses the common measuring shapes — `wc`, `du` and `stat` over the covered set — outside this sub-check's recipe and K1's command; it enforces the FORBIDDEN row above, not the cap, and carries no figure.

### Sub-check 10 — style-guide audit coverage map

Detection mechanism. The audit reads `ai-docs/agent-writing-style.md` at audit time and parses every ATX `## ` heading.

Parser strictness rules:

1. Match **ATX-style level-2 headings only** — exactly two `#` characters followed by exactly one space, then heading text.
2. **Skip lines inside fenced code blocks.** Track ` ``` ` and `~~~` fence state; a `## ` line inside an open fence is NOT a heading.
3. **Case-sensitive match.** `## Patterns` ≠ `## patterns`.
4. **Trim** leading/trailing whitespace from heading text before lookup.

Inline coverage map (live as of this commit; re-validate at audit time by re-running `grep -n '^## ' ai-docs/agent-writing-style.md` and reconciling against this map):

| `## ` heading | Maps to | Outcome |
|---|---|---|
| `## Patterns` | sub-checks 1–7 (audits the shape of every entry under this heading) | no finding |
| `## Anti-patterns` | sub-check 8 | no finding |
| `## Writing checklist` | excluded — meta-section (reader checklist, not a rule shape) | no finding |
| `## Citation in PRs` | excluded — meta-section (PR-author convention, not a rule shape) | no finding |
| `## Enforcement` | excluded — meta-section (cross-references the audit itself) | no finding |
| `## Propagation rule for new patterns` | excluded — meta-section (fan-out procedure, not a rule shape) | no finding |
| `## Out of scope` | excluded — meta-section (negative-space scoping, not a rule shape) | no finding |

Unmatched-heading rule. For every parsed `## ` heading NOT in the coverage map above, emit:

- **Finding text:** `audit coverage gap: § <heading>`
- **Proposed action:** `add sub-check N+1 to /ai-audit Checklist M` (where N is the current max sub-check number)
- **Severity:** `nit`

When a future PR adds a new `## ` heading to `agent-writing-style.md`, Sub-check 10 fires at the next `/ai-audit` run with the gap; the operator either adds a corresponding sub-check or extends the exclusion list in the same follow-up.

### Checklist M — audited corpus

The audited corpus enumeration is identical to the Checklist M intro paragraph above:

- `AGENTS.md`
- every `.claude/skills/**/SKILL.md`
- every `.claude/agents/**.md`
- `ai-docs/code-style.md`
- `ai-docs/doc-convention.md`
- `ai-docs/agent-writing-style.md`
- `ai-docs/corrections-log.md`
- every `.claude/rules/**/*.md`

