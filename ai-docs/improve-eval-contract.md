# `/improve` Step 6 — the eval contract: parent dispatch, the RED baseline, the verdict space

> Extracted from `.claude/agents/self-improve.md` § Step 6. That file carries a one-line summary of the contract and points here; **this page is the canonical statement** of why the parent dispatches, plus the provenance and the failure modes — and, since the RED-baseline change, of the pre-change baseline, the verdict space, and the deferred authorship gap (D1). Read when tempted to dispatch the reproducers from inside the Subagent, to substitute a cheaper path, or to count a PASS that was never preceded by a recorded FAIL.

## The contract is a MAY rule, not a CAN rule

`Agent` **is** present and callable from the `self-improve` Subagent class **in this project**. Probed 2026-07-17: a live `Agent(subagent_type: "general-purpose", …)` dispatch from inside a `self-improve` spawn launched and returned `PROBE_OK` intact.

So the prior claim — *"structurally unfulfillable; the runtime tool exposure genuinely lacks `Agent`"* — is **false here as of 2026-07-17**. **But it was true where it was written.** The sibling **quartzite** project recorded it with evidence in `maratik123/quartzite#364` and its matching 2026-05-15 tooling entry (*"the missing primitive is real … structurally unfulfillable by the subagent itself"*), after first falsifying the opposite hypothesis.

**The runtime changed between that finding and this one. The claim was not fabricated — it expired.** Re-probe rather than trusting either date.

Observed mechanism, for whoever probes next: the dispatch is **async**. It returns `Async agent launched successfully` with a task id, then delivers the result via a later notification — so a probe expecting a blocking call-and-return can misread a successful launch as a failure.

## Why the parent owns it anyway

Do not re-derive this from your tool list. **A capability grant is evidence about CAN and says nothing about MAY** (`.claude/agents/design-writer.md` § Quality checklist → Constraints).

The parent thread owns the eval because it owns the **user-facing report**: Step 6's verdict is addressed to the user, and this Subagent's contract is *analyse and propose*, not *adjudicate and report*. That reason is independent of what your tool list contains — which is exactly why it survived the capability claim turning out to be wrong.

If you believe the parent-dispatch contract is wrong, **say so in your report** and let the user decide. Do not resolve it by acting.

## The forbidden degraded paths — each on its own merits

You have `Agent`; do not use it for Step 6. And do **not** substitute any of these:

- a `Bash`-shelled invocation,
- `TaskCreate`-then-`TaskOutput` polling,
- an in-memory close-read.

None of them runs the reproducer in a **clean context**, which is the entire point of the eval. A same-context "close-read" grades the reproducer against the very transcript that authored the rule.

Authority: `maratik123/quartzite#362` Commit C (*"record eval-degradation pattern"*) and quartzite's 2026-05-15 process entry recording this Subagent silently degrading Step 6 from clean-context evals to same-context close-reads. Verify with `gh pr view 362 --repo maratik123/quartzite` — a bare `gh pr view 362` resolves against **this** repo and will falsely report *Could not resolve*. The rule stands on the clean-context requirement regardless of that citation.

## Leaking the grader defeats it just as surely

Emitting the reproducer as one block, or copying the GRADER block into the dispatch, destroys the same clean-context property — an eval that shows the agent its own expected answer measures nothing, and returns a near-guaranteed PASS that is statistically independent of rule strength. See [`ai-docs/templates/improve-eval-reproducer.md`](templates/improve-eval-reproducer.md) for the SUBJECT / GRADER split.

## The RED baseline — a PASS counts only after a recorded FAIL

**The defect this closes.** Every reproducer used to be dispatched *after* the proposal had been applied to the working tree, so a fresh agent that would have answered correctly **without** the new rule was indistinguishable from the rule working. The gate could only ever confirm. So: **a reproducer's PASS does not count until that same reproducer has been recorded FAILing against the pre-change state.** A reproducer that passes both ways proves nothing about the rule — it is **rewritten or dropped, never counted** toward the gate.

**Ordering — the pre-change state is obtained by not having applied anything yet.** The parent's sequence is `classify → baseline batch → apply to the working tree → verification batch → gate on the PAIR of outcomes`. `self-improve` already assembles both reproducer blocks at the END of its response and then yields, so the parent holds them before it applies anything and the baseline dispatch slots into that window. Nothing about who assembles, who dispatches, or when the blocks exist changes; no VCS manipulation and no isolation primitive is involved. The commit still happens only after both outcomes are in, so a proposal that fails its eval still never reaches history.

**The pre-change tree is the tree MINUS its own append-only history.** Both dispatches declare `ai-docs/learnings.md` and `ai-docs/harness-gaps.md` out of scope, in the SUBJECT block's own framing — naming the **files**, never the rule. Without that exclusion the baseline is void by construction on this corpus: a `Kind: correction` entry's content **is** the rule, and every proposal is escalated from entries already committed, so the state the baseline samples always contains the clause under test on a surface the dispatched agent can grep. Measured 2026-09-11: across 12 dispatches, agents located their own source entries — one quoting the near-verbatim original sentence together with that entry's `at:` commit — and **0** went RED. The exclusion is necessary and, on its own, **not sufficient**: three reproducers re-run under it still passed, through rules already shipped in `AGENTS.md`. What it buys is a baseline that samples the surface escalation actually changes — context loaded by default, not knowledge available on demand. What it does not buy is discriminating power over a rule whose failure mode needs load; that is what *out of instrument reach* below is for.

**Every run is a clean-context dispatch** carrying the SUBJECT block only — the default pair, and the load-bearing variant where one is needed. The forbidden degraded paths above bind the baseline dispatch exactly as they bind the verification dispatch — a same-context close-read of the pre-change state is not a baseline.

**Rejected mechanisms, recorded so they are not re-proposed:**

- **`git stash`** — a VCS manoeuvre across an async multi-dispatch window, on a tree holding user-approved uncommitted work. A stray write from an eval agent lands in the stashed-over tree and can conflict on `pop`. AGENTS.md § *Workflow* already flags whole-file-restoring git forms as hazardous precisely when a file mixes an edit you keep with one you drop — which is what a mid-`/improve` tree is.
- **`isolation: "worktree"`** — a live `Agent` parameter, **not adopted here, and the ground is scope rather than merit.** It is one of the three candidate containment mechanisms enumerated by maratik123/graphite-gp#162, so adopting it here as the *state* mechanism would pick #162's containment mechanism for one dispatch class. **#162 is a separate question, it remains open, and nothing on this page answers it: no sentence here forbids a read-only agent type, `isolation: "worktree"`, or a prompt-level prohibition** — all three stay available to whoever resolves it. (Independently, the tool schema does not state which ref the worktree checks out, so the property a baseline would rest on is unverified. That observation is not the reason for the rejection.)

**The added dispatch count is bounded, and the bound is a contract rather than an aspiration.** It is not 2×: a **baseline-exempt** proposal (neither coverage limb fires) adds **zero**, and on a FAIL loop-back to Step 3 the baseline is **re-used** whenever the reproducer is unchanged — a FAIL reverts before anything commits, so the pre-change state has not moved. Only a **changed** reproducer needs a fresh baseline.

**Which proposals need a baseline** is the two-limb coverage test, stated normatively in [`.claude/agents/self-improve.md`](../.claude/agents/self-improve.md) § Step 6: limb 1 — the proposal edits or strengthens rule text already present in an instruction file, evaluable from the diff; limb 2 — at least one of the pattern's source entries **records** the rule being cited or applied and the defect occurring anyway, evaluable from the source entries. Either limb fires the baseline, and the assembling agent has no per-proposal discretion.

### Attributability — an unattributable baseline FAIL is not a baseline

The pre-change FAIL must be attributable to the **absence of the rule under test**. The GRADER block's `FAIL criterion` is therefore required to be the **negation of the clause under test** — quoted verbatim in that block's `Rule clause under test:` field — and never a generic wrong-answer test.

**The consequence, stated so it cannot be waved through:** a baseline that fails for an unrelated reason — an ambiguous scenario, an exhausted budget, a misread prompt — is **indistinguishable from a genuine *not recalled*** when read from the returned answer alone. A FAIL-before / PASS-after pair resting on such a FAIL would therefore **license a rule the run never tested**. That is the confirm-only defect this whole mechanism exists to remove, reproduced one level down *inside its own instrument*. A reproducer whose pre-change FAIL cannot be attributed to the rule's absence is rewritten or dropped on exactly the same footing as one that passes both ways.

## Verdict space

Three outcomes, plus **two** terminal states and one modifier. All of it is read from the **post-change** run alone, so it costs no extra dispatch and is available for every proposal, baseline-exempt ones included. The three-outcome split extends **no** blocking power: the post-change eval is already the commit gate, so every outcome other than *applied and held* already blocked — the split subdivides an **already-blocking** FAIL so Step 3 can route on it. *Out of instrument reach* is the one row that moves in the other direction, and it is stated as a narrow, owner-gated exception precisely because an unblocking cell is the dangerous kind: on a corpus where the instrument cannot reach the class, a gate with no such cell blocks **everything**, which is the confirm-only defect inverted rather than cured.

| Outcome | Read from the returned answer | Commits? | Remedy — what Step 3 changes |
|---|---|---|---|
| **not recalled** | neither cites nor applies the clause under test; the `FAIL criterion` is met | **No** — reverted from the working tree before it lands | The rule's **placement or strength** — promote it, or move it closer to the point of execution (Step 2a's *rule exists but isn't working* row) |
| **recalled and misapplied** | cites or applies the clause, yet the outcome still violates what the clause requires | **No** — reverted from the working tree before it lands | The rule's **form** — restate a property as a **procedure** with a checkable output |
| **applied and held** | cites or applies the clause and satisfies it | **Yes** | — |
| **no valid reproducer** (terminal) | every attempted reproducer passed both ways, or its baseline FAIL was unattributable, or it failed derivability | **No** | Neither of the above: the run produced **no signal about the rule**, so there is nothing to route on. Work goes into the reproducer first, and the proposal is surfaced **with the failed attempts attached**, so a later reader can tell *"no valid reproducer"* from *"nobody tried hard"*. A proposal never commits on an "unevaluable" verdict — that would reinstate the confirm-only gate under a label. The single exception is the row below, which is not a label but a **demonstration**, and which costs an owner decision recorded in the diff |
| **out of instrument reach** (terminal) | every attempted reproducer passed both ways, **and** a **load-bearing** variant of at least one of them — the same clause reached as an *incidental step inside a larger primary task*, rather than as the whole question put to an idle agent — also passed | **Only** on explicit owner approval recorded in the PR body, with **both** runs attached | None to the RULE — the remedy is to the **instrument**. File the reach failure in `ai-docs/harness-gaps.md`, naming the class it cannot reach |
| **baseline-exempt** (a modifier, not an outcome) | neither coverage limb fires | only on *applied and held* | None of its own — it records its ground on the GRADER block's `Baseline (pre-change) required?:` line, and the three outcomes above still apply |

**The two terminal states are not synonyms, and the difference is an experiment with an INDEPENDENT VARIABLE.** *No valid reproducer* says nobody built one that discriminates — compatible with insufficient effort, which is why the failed attempts must be attached. *Out of instrument reach* says the class cannot be reached by clean-context dispatch **at all**, and it is earned only by varying the one thing every default dispatch holds fixed: **load**. The default regime puts the clause to an idle agent as the whole question; the load-bearing variant buries it as an incidental step inside a larger task the agent must also carry out. If the clause is honoured there too, the failure mode the corpus records is not reproducible by this instrument. Without that second, *different* run the verdict is *no valid reproducer* and the proposal does not commit.

**The trap this row was drafted into once, recorded so it is not re-entered.** The first draft earned the cell on "a re-run with the append-only history excluded" — after the same edit had made that exclusion the **default for both dispatches**. The extra dispatch therefore varied nothing, and every *no valid reproducer* reached the unblocking cell by re-running an unchanged reproducer. **A verdict whose extra dispatch has no independent variable is the confirm-only gate wearing a new label** — the precise defect this section exists to prevent. Before adding or amending any earning condition here, name the variable it varies and check it is not already fixed by the default regime.

**Why three rather than binary, since two of the rows revert.** *not recalled* and *recalled and misapplied* share the revert but **not** the remedy, and that difference is the whole reason the space is three-wide: a rule that is recalled and misapplied needs its **form** restated, while one that is never recalled needs **promoting or relocating**. A binary gate maps both onto one cell, reverts them together, and — because a reverted proposal otherwise leaves no record — destroys the very evidence that would justify telling them apart later.

**"Recalled" is an artifact property, never a self-report.** It is determined from a **citation of, or an application of, the clause** in the dispatched agent's returned answer — never from what the agent says about its own reasoning. A SUBJECT scenario that *asks* the agent whether it used rule X is rejected twice over: it leaks the rule to the agent being graded, and it substitutes self-report for trace. Without this, the middle cell becomes the drawer everything inconvenient goes into.

**The verdict is read against the clause under test, not against the pass as a whole.** With every baseline run before a batched apply, a reproducer's post-change PASS could be produced by a *sibling* proposal in the same pass. The GRADER block's `Rule clause under test:` pins the clause and its `Rule-citation observable` requires the returned answer to cite or apply **that** clause — so a PASS produced by a sibling reads as *not recalled* on the clause under test, and reverts.

## D1 — reproducer/rule authorship separation (deferred, recorded)

**The gap.** A rule's reproducer is assembled by the **same `self-improve` invocation that proposes the rule**. Closing it is a structural change to the assemble-vs-dispatch split whose cost has never been weighed, so it is deferred — recorded here, not solved here.

**The concrete failure mode, named rather than gestured at:** *a reproducer authored by the rule's own proposer returns FAIL-before / PASS-after by construction, so a same-author baseline cannot distinguish a rule that works from one that was written to fit its own test.* The RED baseline raises the floor — it removes the reproducers that pass both ways — but it does not close this, because both runs are still graded against a criterion the proposer wrote.

**What would show the gap is biting** — stated so a later reader can check it **off the PR-body verdict tables** rather than re-derive it. Either signature:

- the *recalled and misapplied* and *applied and held* cells **never separating** — pass after pass landing in one cell only; or
- **FAIL-before / PASS-after being near-universal** — a baseline that goes RED for essentially every proposal put to it.

Neither is what a gate discriminating on its own merits looks like; that produces a **mixed** table. Both are what an unclosed authorship gap looks like.

**Precedent, cited by slug:** **graphite-gp**'s `ai-docs/learnings.md`, 2026-07-31 — *"the reviewer builds its OWN representative input; a shared fixture buys correlated blindness, not economy"* (`Kind: validation`). That entry extends the clean-context principle from *context* to *fixture*: an author cannot validate their own work with their own test input any more than in their own context. D1 is the same shape one level up — the author of the **rule** authoring the input that tests it.

## Propagation-rule asymmetry

The Learning-Log sync-group sister file `.claude/agents/learnings-escalation-audit.md` has **no** Step 6 eval-phase equivalent — its workflow is a passive auditor and its `Step 6 — Report` is structured output, not a primitive-dispatch step. This contract therefore requires no mirrored edit there.
