# Interview state — rename the `design` Subagent

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-03-rename-design-subagent.spec.md
issue_ref: "#10"
gh_issue:
  title: "Rename the `design` Subagent so no project Subagent shares a name with an embedded Skill"
  state: open
  labels: []
  body: |
    
    Split out of #8, where it was item 2. Issue #8's own recommendation was to leave the
    name alone and record a cross-axis exemption; that recommendation is withdrawn here,
    for a reason the original framing did not have.
    
    ## Why the "cross-axis, therefore harmless" argument does not hold
    
    `.claude/agents/design.md` declares `name: design`, a **Subagent**. The harness ships
    an embedded `design` **Skill** (Claude Design canvas). Checklist O's rewritten severity
    table rates a cross-axis clash `minor` and routes it to the owner, on the reasoning
    that the two dispatch through different tools — `Agent(subagent_type="design")` versus
    `Skill(skill="design")` — and each therefore resolves unambiguously.
    
    That reasoning is about the dispatcher. The observed failure is about the model.
    In sibling projects the owner has repeatedly seen agents confuse a Skill and a Subagent
    that share a name — picking the wrong tool, or blending the two definitions — even
    though dispatch itself was never ambiguous. Unambiguous-to-the-parser is not the same
    property as unambiguous-to-the-reader, and only the second one was ever load-bearing.
    
    **This makes Checklist O's severity rule itself a defect, not just its verdict on this
    name.** The rule's stated motive — cross-axis clashes are theoretical because dispatch
    resolves — is the exact claim the field evidence contradicts. Fixing only `design`
    leaves the rule that waved it through in place for the next collision. Whichever way
    the rename lands, the severity table's justification needs rewriting.
    
    ## Scope, measured
    
    Measured on the tree with harness/forge-4 applied (`wc`/`grep` at that tree):
    
    | Class | Count | Notes |
    |---|---|---|
    | `.claude/agents/design.md` path references | 15 | includes the `git mv` target itself |
    | `subagent_type="design"` | 2 | both in `.claude/skills/task/reference.md`, typed by forge-4 |
    | prose `` `design` `` naming the Subagent | ~20 | needs per-site judgement — many `design` tokens mean the *document*, not the agent |
    | **files touched** | **20** | listed below |
    
    ```
    .claude/agents/design.md              .claude/skills/interview/SKILL.md
    .claude/agents/design-review.md       .claude/skills/main-ci-failed/SKILL.md
    .claude/agents/self-reflect.md        .claude/skills/main-ci-failed/reference.md
    .claude/agents/self-review.md         .claude/skills/pr-ci-failed/SKILL.md
    .claude/agents/spec-writer.md         .claude/skills/pr-ci-failed/reference.md
    .claude/skills/context-reset/SKILL.md .claude/skills/pr-commented/SKILL.md
    .claude/skills/task/SKILL.md          .claude/skills/pr-commented/reference.md
    .claude/skills/task/reference.md      ai-docs/agent-writing-style.md
    ai-docs/claude-tools-hierarchy.md     ai-docs/improve-eval-contract.md
    ai-docs/propagation-groups.md         ai-docs/task-run-schema.md
    ```
    
    Several sit in declared sync groups, so the Propagation Rule applies. `ai-docs/learnings.md`
    is a history surface and is **not** renamed.
    
    ## The trap this issue must not repeat
    
    Session `a47d904a` attempted this rename inside `/task` and never reached implementation.
    Two things went wrong, and both are now avoidable:
    
    1. **The site count moved three times** — 6, then 7, then 8 for `.claude/skills/task/SKILL.md`
       alone — because each sweep pattern-matched a symptom rather than the concept. The
       count is genuinely hard: the token `design` names the Subagent, the design *document*,
       the `.design.md` extension, and `design-review`, and only the first renames. Sweep at
       concept level and enumerate per-site with a judgement column; do not trust a bare grep count.
    2. **The byte arithmetic ate the rounds.** That cause is gone: harness/forge-4 retired the
       CI size gate and made instruction-file size `/ai-audit`'s exclusive property. No spec
       constraint, AC, design risk row or review finding in this task may name a file size.
       If one appears, it is a spec defect under `.claude/agents/spec-writer.md`.
    
    ## Naming
    
    `design-writer` was chosen by the owner in `a47d904a` round 2: the repo's dominant pattern
    is `<artifact>-writer` (`spec-writer` produces the spec, `code-writer` produces the code),
    it pairs cleanly with the unchanged `design-review`, and it names the role rather than the
    artifact, as every other agent in the directory does. Re-confirm or re-pick at interview time.
    
    ## Route
    
    This is a pure harness edit — no Go source changes. Per the standing rule that harness
    edits do not travel through the harness they edit, it belongs in a forge patch applied
    with `git am`, not in a `/task` run. `a47d904a` was mis-routed at the entry point and
    that contributed to its fragility.
    
    ## Acceptance shape (not ACs — those are the spec's)
    
    - The Subagent file is renamed, `name:` matches the new basename, and every reference
      resolves — `check-citations.sh` and the CI link check both green.
    - No project Subagent, Skill or Hook-event name collides with an embedded name on either
      axis. Checklist O re-run reports a clean scan with a non-empty embedded list (an empty
      list is `inconclusive`, never `pass`).
    - Checklist O's severity table no longer justifies cross-axis leniency by dispatch
      resolvability.
    
  comments: []
  linked_issues: ["#8"]
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 1
agent_id: null
prior_qa: []
```
