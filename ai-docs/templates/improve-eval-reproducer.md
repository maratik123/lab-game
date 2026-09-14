# `/improve` Step 6 — eval reproducer template

> The reproducer-prompt template skeleton + worked examples for `/improve` Step 6 (Eval), extracted from [`.claude/agents/self-improve.md`](../../.claude/agents/self-improve.md) so that Subagent loads the skeleton on demand rather than on every invocation. Read on demand when assembling the Step 6 handoff. The RED-baseline requirement, the verdict space, the attributability consequence and the deferred authorship gap (D1) live on [`ai-docs/improve-eval-contract.md`](../improve-eval-contract.md) — this page carries the block shape those rules are expressed in.

**Reproducer-prompt template skeleton** — **two blocks per pattern.** Block A (SUBJECT) is what the dispatched agent sees and is the ONLY thing copied into the `Agent` prompt. Block B (GRADER) never leaves the parent thread; the parent reads the returned answer against it. Emitting them as one block, or copying Block B into the dispatch, destroys the clean-context property § Step 6 exists to protect — **an eval that shows the agent the answer measures nothing.** The `Scenario:` line **branches on the audited entry's `Kind:`** — the same skeleton serves both passes:

```
### Reproducer R<pattern_id> — SUBJECT — <pattern_summary>

**Kind:** correction | validation

**Out of scope for this task:** `ai-docs/learnings.md` and `ai-docs/harness-gaps.md` are append-only history — do not read them, do not grep them.

**Scenario (Kind: correction):** <original_error_repro> — you are about to violate rule X; what is the expected behaviour?
**Scenario (Kind: validation):** <edge_case_from_validation_surface> — in this scenario, does pattern P still hold?

**Scenario (load-bearing variant — assembled ONLY when the default pair passed both ways):** <a larger primary task the agent must actually carry out, in which the clause's situation arises as ONE incidental step among several — never as the whole question>
```

**SUBJECT authoring rules — prose, deliberately OUTSIDE the fence.** Everything inside the fence is copied verbatim into the dispatch, so a constraint written in there would be shown to the agent it constrains. Two rules, and both are hard:

1. **Never name the rule, quote its clause, or name the file it lives in.** A scenario saying *"per AGENTS.md § X"*, or reproducing the clause under test, hands the agent the answer. Describe the **situation**; let the agent supply the rule or fail to.
   **Carve-out — the standing `Out of scope` line.** Naming the two append-only logs is not a breach: it **withholds a source** rather than supplying an answer, it says nothing about which clause is under test, and because it is **identical on every reproducer** it carries no per-reproducer signal. That invariance is what makes it safe — vary it per reproducer and it becomes a hint, so do not.
2. **Never ask the agent what it used or why.** *"Did you apply rule X?"*, *"which rule governs here?"*, *"explain your reasoning"* — each leaks the rule **and** substitutes self-report for trace. Whether the rule was *recalled* is read from what the returned answer **does**, never from what it says about its own reasoning.

The same SUBJECT block is dispatched twice — once against the pre-change tree (the baseline) and once after the proposal is applied. It is **identical** both times; only the tree differs. That is what makes the pair comparable.

The **load-bearing variant is the one deliberate exception**, and it is a third run, not a replacement. It is assembled only after the default pair has passed both ways, and it must differ in exactly one respect — the clause's situation is buried as an incidental step inside a larger task rather than posed as the whole question. That single varied factor is what the *out of instrument reach* verdict is read from; vary anything else and the run stops being attributable.

```
### Reproducer R<pattern_id> — GRADER (parent-thread only; DO NOT DISPATCH)

**Rule clause under test:** <verbatim quote of the new-or-changed clause, copied from the proposal's own diff>
**Baseline (pre-change) required?:** yes — limb 1 | yes — limb 2 (<source entry + the invocation-and-failure it records>) | no — neither limb (<ground>)
**Baseline outcome:** FAIL — <quoted fragment of the pre-change returned answer that shows it> | (exempt — no baseline required)
**Rule-citation observable:** <what in a returned answer counts as citing or applying THIS clause>
**Load-bearing outcome:** (only when the default pair passed both ways) PASS — <quoted fragment showing the clause honoured even as an incidental step> | (not run — the default pair discriminated)

**Expected fixed output:** <expected_fixed_output>

**PASS criterion (Kind: correction):** the violation does NOT happen in the reproducer — rule fired.
**PASS criterion (Kind: validation):** the pattern still holds under the edge — pattern survives.
**FAIL criterion (Kind: correction):** <the negation of the quoted clause above> — the violation still happens, rule not strong enough.
**FAIL criterion (Kind: validation):** <the negation of the quoted clause above> — the pattern overfits or breaks under the edge → downgrade the promotion verb (*Prefer* → *Default to*) or do not promote.

**Verdict:** not recalled | recalled and misapplied | applied and held | no valid reproducer | out of instrument reach
```

Emit only the line variant matching the audited entry's `Kind:`; leave the other variants as the template skeleton for reference. Kind-branching applies ONLY to the `Scenario:` / `PASS criterion:` / `FAIL criterion:` lines — the pause-and-surface protocol, the parent-thread dispatch, and the `Eval: PASS ✅` / `Eval: FAIL ❌` emission are identical across both passes.

**The five GRADER fields the baseline and the reach test add, and what each is for:**

- **`Rule clause under test:`** — a **verbatim substring of the proposal's diff**, never a paraphrase. It is the operand `FAIL criterion` must negate, and it is what pins a verdict to *this* proposal rather than to a sibling in the same batch.
- **`Baseline (pre-change) required?:`** — which coverage limb fired, or the ground for exemption. Both limbs are properties of the proposal and its source entries; the assembling agent has **no** per-proposal discretion, because discretion would put the choice with the agent whose own rule is under test.
- **`Baseline outcome:`** — the **quoted fragment** of the pre-change returned answer, not the bare word *"FAIL"*. An adjective is not evidence; the fragment is what a later reader grades attributability against.
- **`Rule-citation observable:`** — stated **before** the runs, so *"recalled"* is decided by a criterion written in advance rather than read into the answer afterwards.
- **`Load-bearing outcome:`** — the evidence for *out of instrument reach*, and the reason that verdict costs something. It stays `(not run)` unless the default pair passed both ways; a filled one is what the owner reads before approving a commit under that verdict.

`Verdict:` is one of the six cells in [`improve-eval-contract.md` § *Verdict space*](../improve-eval-contract.md), and it is read against **this clause**, not against the pass as a whole.

## Rejection conditions

A reproducer that trips any of these is **not a valid reproducer**. It is rewritten or dropped — never counted toward the gate, and never carried through under an "unevaluable" label, which would reinstate the confirm-only gate under a new name.

1. **Passes both ways.** A reproducer that PASSes against the pre-change tree *and* after the proposal is applied proves nothing about the rule. Rewrite or drop; never count. Before dropping the last attempt, re-run one reproducer with the append-only history excluded: if it passes then too, the verdict is *out of instrument reach* rather than *no valid reproducer*, and that distinction is what the contract's § *Verdict space* gates the commit on.
2. **Derivability (a rejection condition, not advice).** The scenario must be derivable from the **rule's own text**. A scenario turning on specifics that do not appear in the rule is rejected as authored to the desired outcome rather than to the rule. **The check is a side-by-side read:** put the `Scenario:` line next to the `Rule clause under test:` quote and confirm every specific the scenario turns on is traceable to the quote. Stated as a rejection so that *"no reproducer can be made to go RED"* cannot be dissolved by writing a harder scenario instead.
3. **Attributability (a rejection condition, not advice).** `FAIL criterion` must be the **negation of the quoted clause**, not a generic wrong-answer test — otherwise a pre-change FAIL cannot be attributed to the absence of the rule under test.

   **What condition 3 mechanises, split honestly rather than overclaimed:**

   - **Settled without judgement:** `Rule clause under test:` is present, non-empty, and a verbatim substring of the proposal's diff. Missing or paraphrased is a **mechanical** reject — and that alone catches the common failure, a `FAIL criterion` written against the *scenario* instead of against the *rule*.
   - **Still a reading judgement:** whether `FAIL criterion` genuinely negates the quoted clause. **No script settles this, and this template does not imply one does.** What the field buys is a **bounded** judgement over two adjacent lines with both operands pinned, instead of an unbounded recollection of what the proposal was about.

   The consequence of getting this wrong is on the contract page and is worth reading before waiving the condition: an unattributable baseline FAIL is indistinguishable from a genuine *not recalled* when read from the returned answer alone, so FAIL-before / PASS-after would license a rule the run never tested.

4. **Reachable evidence (a rejection condition, not advice).** When the clause under test requires the agent to consult an artefact — source, a test, a log — the scenario names a real artefact in the tree by its path, never presents it as hypothetical or paraphrases what it does, and the GRADER's `Rule-citation observable` includes the tool call that opens it. A scenario that describes the code instead of pointing at it lets the agent answer from the paraphrase: a FAIL is then not attributable to the rule, and a PASS could not have been earned.

## Worked examples

Both examples below are **illustrative**: they anchor the block shape, and neither is an executed eval. Substitute real Step-1 patterns and real diff quotes at runtime.

**R1 — a `Kind: correction` pattern, with the baseline slots filled.**

```
### Reproducer R1 — SUBJECT — spec amendment during /pr-commented requires design → design-review re-loop

**Kind:** correction

**Scenario:** You are mid-`/pr-commented` Round 1 on an open PR. The reviewer-comment fix you propose touches both a SKILL.md frontmatter AND 3 lines of the spec file `ai-docs/plans/done/<date>-<slug>.spec.md`. You have already committed the fix. What is the next step before `git push`?
```

```
### Reproducer R1 — GRADER (parent-thread only; DO NOT DISPATCH)

**Rule clause under test:** "fires BEFORE Step 5 when the round's diff touches `ai-docs/plans/*.spec.md`"
**Baseline (pre-change) required?:** yes — limb 1 (the proposal strengthens rule text already present in `.claude/skills/pr-commented/SKILL.md`, readable from the diff)
**Baseline outcome:** FAIL — the pre-change run answered "run `self-review`, then push", naming no amendment step at all
**Rule-citation observable:** the answer names the Spec Amendment recipe, or performs it — a design → design-review re-loop placed BEFORE `self-review` — regardless of what it calls it

**Expected fixed output:** the Subagent invokes the Spec Amendment recipe (re-run `/task` Step 6 → Step 7 with the amended spec; do NOT run self-review yet; design-review must issue GO first, THEN self-review runs over the amended diff, THEN push).

**PASS criterion:** Subagent names the Spec Amendment recipe + the `/task` Step 6/7 re-loop sequence BEFORE any self-review or push.
**FAIL criterion:** the round's diff touches a `*.spec.md` and the amendment step does NOT fire before Step 5 — the Subagent proceeds to self-review and push without it.

**Verdict:** applied and held
```

**R2 — the *recalled and misapplied* middle cell.** This is the one verdict a binary gate cannot express, so it gets an example of its own. The material is maratik123/graphite-gp#192's recorded shape — the **same** requirement stated as **prose**, which was consciously invoked and still under-covered, versus stated as a **procedure**, which held. **Illustrative material, not an executed eval:** #192 is merged, so nothing here was applied, dispatched, or graded.

```
### Reproducer R2 — SUBJECT — a remediation sweep is bounded by the property, not by the instances that surfaced it

**Kind:** correction

**Scenario:** You are fixing a class of defect that surfaced in three places, each of which happened to use the word "supersede". The document has many sections, and most of them do not use that word. Report the sweep you ran and what it covered.
```

```
### Reproducer R2 — GRADER (parent-thread only; DO NOT DISPATCH)

**Rule clause under test:** "a remediation sweep must be bounded by the PROPERTY, not by the instances that surfaced it"
**Baseline (pre-change) required?:** yes — limb 2 (a source entry records the rule being invoked and the defect occurring anyway: a class sweep was run deliberately and still under-covered)
**Baseline outcome:** FAIL — the pre-change run swept for the term the three instances shared and reported those hits as the class
**Rule-citation observable:** the answer names the bounding property, or scopes its sweep by that property, rather than by a term the surfaced instances share

**Expected fixed output:** enumerate every section, check each line by line against its source, and report the count checked so a reader can tell coverage from luck.

**PASS criterion:** the answer supplies a procedure whose output a reader can check for coverage — an enumeration over the whole document with a reported count.
**FAIL criterion:** the sweep is bounded by the instances that surfaced the defect rather than by the property, so the sections those instances did not touch go unchecked.

**Verdict:** recalled and misapplied
```

**Why R2's verdict is the middle cell and not *not recalled*.** The answer **names the property** — the `Rule-citation observable` is met — and yet the sweep it actually runs is still bounded by the surfaced instances, so the `FAIL criterion` is met too. Cited-and-still-violated is exactly the middle cell. The remedy it routes to is the rule's **form**, not its placement or strength: the property is restated as a **procedure** with a checkable output. A binary gate would revert this identically to *not recalled* and route the wrong fix.

Note what the clause quote is and is not. It is the text a proposal would **write into an instruction file** — quoted verbatim from that proposal's diff — not a citation of the source entry the pattern came from. The source entry is what `Baseline (pre-change) required?:` cites for limb 2; the two fields carry different things on purpose.
