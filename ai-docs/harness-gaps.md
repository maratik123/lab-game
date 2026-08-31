# Harness gaps

Append-only log of **harness diagnoses**: gaps, ambiguities, and defects in instruction files, queued for `/improve`. This is the designated parking surface `AGENTS.md` § *Learning Log* names — a harness diagnosis written here is NOT an instruction-file edit under Boundary rule 2, and does NOT belong in `ai-docs/learnings.md` (that log holds conduct corrections only).

Boundary rule 1 (append-only) applies here verbatim: never edit, reorder, summarise, or delete an existing entry; supersession is a new entry plus the old entry's `Superseded by:` field.

## Entry skeleton

```
### YYYY-MM-DD — [short description of the gap]
**target:** [harness file the fix belongs in — e.g. `.claude/agents/self-review.md`]
**Observed:** [what happened that the harness permitted or failed to prevent]
**Gap:** [the property of the harness that allowed it]
**Proposed edit:** [minimal protocol-shaped change — a gate, a required field, a closed list; not a disposition]
**at:** [commit SHA | `main`]    (required for any numeric claim)
**Superseded by:** [ref] — [reason]    (optional)
```

Entries are appended at the END, newest last.

---

### 2026-08-31 — amendment duty was bound to phases, not to the question's subject
**target:** `.claude/skills/task/SKILL.md`
**Observed:** (1) Session `ec78f817`: a reviewer spec-amendment trigger closed in-thread — patched by forge-1's two-exits rule, Step 11 only. (2) Task-A design phase: two genuine scope questions (propagation breadth; a `settings.json` grant) surfaced to the user bare — no route attached; the routing had to be supplied by hand. (3) By mid-session the pattern self-corrected in-context: the guard, AC8 and §I questions all arrived route-attached — precedent holds within a session; the gate is for the next session, which has no precedent loaded.
**Gap:** the obligation lived at named points (Step 11 fix-diff table; reviewer trigger wording); a scope question born elsewhere arrived bare.
**Proposed edit:** phase-independent AXIOM keyed to the question's SUBJECT (escalated in this same commit).
**at:** d2db0c7

### 2026-08-31 — reviewer prompts accumulated orchestrator state; design-review had no closed-list charter
**target:** `.claude/agents/design-review.md`, `.claude/agents/self-review.md`, `.claude/skills/task/SKILL.md`
**Observed:** Task-A design-review spawns carried, escalating round over round: cap-exhaustion state, "hit hardest at Issue 1" priorities with a hinted fix trap, then a routing-judgement request ("say whether it blocks or can ride to Step-11, and what you'd choose"). Verdict quality stayed high (the reviewer twice rejected its own proposals with measurements), but a contaminated GO and an earned GO are indistinguishable from outside — the contamination costs verifiability, not necessarily correctness. Root: forge-1's E12 covered self-review.md only. Owner's carrier argument: the reader has a delivery-bias system prompt, so ANY cap/cost information weighs toward "don't find more" — the boundary must be protocol (closed list, no exception categories), not semantics ("routing info OK, priorities not").
**Gap:** one review charter of two carried the closed list; nothing anywhere banned caps/routing-questions as a class.
**Proposed edit:** closed list in BOTH charters incl. the invocation line (task-A reviewer flagged it as the one persistent extra — SR2-A0/SR3-A6); caps, history, priorities and routing requests named as PROMPT-CONTAMINATION; orchestrator-side line at Step 10 (escalated in this same commit).
**at:** d2db0c7

### 2026-08-31 — spec-phase misses that force amendment round-trips: enumeration, permissions, byte-ceilings
**target:** `.claude/agents/spec-writer.md`
**Observed:** Task A took 5 spec amendments; the classifiable ones: (1) propagation listed 5 known sites, design found 25 → +1 more (`AGENTS.md:77`) on a second sweep — enumerations, not classes; (2) `Bash(make *)` grant missing for AC13's unattended run, then again for task/SKILL.md's own allowed-tools (leg 2, seven-of-eight); (3) AC9 pinned AGENTS.md to a byte ceiling below the hard cap, manufacturing "no-growth vs must-propagate" twice. Probe-class finds (guard regex narrowing) are NOT in this class — those legitimately belong to design.
**Gap:** spec rules verified sources (PROC-1/2) but not closure of propagation, executability, or ceiling sanity.
**Proposed edit:** PROC-3 with the three sub-rules (escalated in this same commit).
**at:** d2db0c7

### 2026-08-31 — auto-memory is an unpropagated fact channel
**target:** none (user-local; harness may not edit it — recording the channel)
**Observed:** `~/.claude/.../memory/harness-editing-gotchas.md` carried the alarm-band reading of the 35k threshold after the owner declared hysteresis semantics; every future session loads memory before any instruction file, with self-trust and no gate. The 30-site propagation sweep could not reach it by design (privacy boundary).
**Gap:** semantic changes to axioms have no sweep over auto-memory; re-infection is silent.
**Proposed edit:** none mechanical (privacy boundary). Hygiene habit: after changing an axiom's semantics, the OWNER greps the memory dir for the stale reading. Owner performed the first such pass 2026-08-31.
**at:** d2db0c7

### 2026-08-31 — forge patches themselves lack DELTA discipline
**target:** (process note for the forge channel, not a harness file)
**Observed:** forge-1's E14 (draft PR at first group return) conflicted with the standing rule "self-review before every push" (AGENTS.md § Workflow AXIOM); the conflict was neither surfaced at patch time nor resolved by the owner — the task-A orchestrator resolved it silently in favour of the more specific instruction, the exact silent-resolution shape the forge exists to kill. Second instance, same commit: the Stop hook's handback vocabulary omitted the awaiting-delegate shape, discovered by a live false-positive.
**Gap:** a forge patch is a hand-off with no `## DELTA` section: constraints it adds or contradicts are not enumerated against the standing corpus.
**Proposed edit:** each forge patch's instructions end with a DELTA table — every standing rule the patch narrows, widens or contradicts, one line each — reviewed by the owner before `git am`. Adopted from forge-3 onward (this patch's DELTA is in its instructions file).
**at:** d2db0c7

### 2026-08-31 — `/improve` Step 6 states an eval ordering its own canonical contract forbids
**target:** `.claude/skills/improve/SKILL.md`
**Observed:** Item 6 reads "The **parent thread** … owns the ordering: **apply the approved proposals to the working tree first**, then **dispatch the clean-context reproducers**", and closes "This working-tree-apply → clean-context-eval → commit-gated-on-PASS ordering is what keeps a failed proposal out of history." The canonical statement it defers to says the opposite: `ai-docs/improve-eval-contract.md` § *The RED baseline* fixes the sequence as `classify → baseline batch → apply to the working tree → verification batch → gate on the PAIR of outcomes`, and `.claude/agents/self-improve.md` § Step 6 makes the pre-change FAIL mandatory. This run followed the skill text, applied three proposals before any baseline, and had to unwind to the pre-change tree via cp-backup mid-flow to recover the baseline window.
**Gap:** the skill restates an ordering instead of deferring to the contract for it, while claiming in the same sentence that it does not restate ("this item references them and does **not** restate them"). The RED-baseline change updated the contract page and the Subagent charter; the Skill's own ordering sentence was not swept.
**Proposed edit:** replace item 6's ordering clause with the contract's four-stage sequence, or delete the ordering sentence entirely and let the existing pointer carry it. Either way the Skill must stop asserting a sequence it also disclaims restating.
**at:** f4d39e8

### 2026-08-31 — the RED baseline runs against a tree that still contains the rule's source correction
**target:** `ai-docs/improve-eval-contract.md`, `.claude/agents/self-improve.md`
**Observed:** All four baseline reproducers dispatched this run came back GREEN — R1 in four separate scenario shapes, R2, R3 — so no proposal could be committed. The cause is structural, not luck: `AGENTS.md` § *Agent Docs* heads its table "Read on nearly every task:" and lists `ai-docs/learnings.md`. The pre-change tree therefore instructs every baseline agent to read the corrections log, which holds the very entry the proposed rule is derived from. Direct evidence rather than inference: the R2 baseline wrote "this host carried a 2022 selection". `eselect iptables list` prints no dates and currently shows `xtables-nft-multi *`; that 2022 fact exists only in the source correction at `ai-docs/learnings.md:31`. The agent read the rule's source and reproduced it.
**Gap:** "pre-change tree" is treated as equivalent to "rule-absent context". Removing a rule from an instruction file does not remove the lesson from the repository, because the append-only log that motivated the rule stays, and the harness routes agents to it by default. Every escalation whose source entry is textually specific is affected; the more concrete the entry, the more completely it teaches the rule it was supposed to test the absence of.
**Gap, second half:** the contract names two signatures of a broken instrument under D1, and both are *near-universal RED* baselines. A uniformly GREEN baseline is not named anywhere, so this run's outcome reads as "three unnecessary rules" when it is actually "no signal".
**Proposed edit:** name the contamination channel on the contract page and give the baseline dispatch a defined exclusion — the pre-change context must exclude the source entries, not merely the proposed diff. Add uniformly-GREEN to the D1 signature list as the third shape, so a later reader can tell a redundant rule from a blind instrument.
**at:** f4d39e8

### 2026-08-31 — two harness mechanics reachable only from user-local memory
**target:** `.claude/skills/ai-audit/**` (citation gate), `ai-docs/code-style.md` or `ai-docs/agent-writing-style.md` (scripted markdown edits)
**Observed:** Surfaced through this run's Step 1c auto-memory sweep and released by the owner's `Surface` consent. Two facts live only in `harness-editing-gotchas.md` in the user-local memory layer, each recorded there as having cost a red gate: (1) `check-citations.sh` reads **line by line**, so a namespace qualifier must sit on the *same line* as the date or issue number it qualifies — a reference split across two lines reads correctly to a human and goes RED; (2) a scripted markdown edit that anchors on a bare heading string finds the first occurrence, which is frequently an in-text *reference* to that heading rather than the heading itself, so the excised region can swallow half the file — anchor on the heading wrapped in newlines instead. Neither is stated anywhere in the repository. The remaining points of that memory (byte-measured size caps with hysteresis; never piping a load-bearing gate) are already escalated in `AGENTS.md`.
**Gap:** the 2026-08-31 entry above on auto-memory recorded the *channel* as unpropagated and proposed no mechanism, correctly, on privacy grounds. What it did not do is drain the channel's existing contents once. These two are the residue.
**Proposed edit:** document the citation gate's line-based reading beside the gate itself, and the heading-anchor hazard wherever scripted instruction-file edits are described. Held for second confirmation under the Step 2b rule — no matching `Kind: validation` entry exists in `ai-docs/learnings.md`, so neither is written into an instruction file on this pass.
**at:** f4d39e8
