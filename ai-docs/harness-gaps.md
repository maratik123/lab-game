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

### 2026-09-01 — a Checklist K measurement ran, produced its answer, and never became a finding
**target:** `.claude/skills/ai-audit/reference.md`
**Observed:** In the `/ai-audit` run that produced `e97768c`, Checklist K sub-checks 1 and 2 were issued in one command block. K1's output listed **seven** `SKILL.md` files over its 200-line threshold — `main-ci-failed` 394, `pr-ci-failed` 361, `pr-commented` 330, `bugfix` 303, `task` 293, `interview` 260, `dependabot-pr` 228. K2's heuristic in the same block was wrong (it matched bare basenames), so K2 was re-run alone and path-qualified. K1 was never revisited and emitted zero findings; the run's commit records fixes for A/B/C/G/H/O only. K1 is the harness's routine size relief, so the extraction it owed `task/SKILL.md` never happened, and the next `/task` — session `a47d904a` — collided with the byte cap that extraction would have relieved.
**Gap:** a sub-check's obligation to *emit* was carried by nothing but the agent's attention. Batching two sub-checks into one command makes the first one's completed measurement invisible the moment the second one needs repair, and repairing an instrument mid-checklist is exactly when that happens.
**Proposed edit:** one tool call per sub-check, plus a required verdict line per sub-check written before the next may start, with a closed set of two shapes and an explicit "absent verdict line counts as not run" (escalated in this same commit).
**at:** e97768c

### 2026-09-01 — the byte cap was enforced twice, and the second enforcer made every task plan around it
**target:** `AGENTS.md`, `.github/workflows/ci.yml`, `.claude/agents/spec-writer.md`
**Observed:** `AGENTS.md` § *Build & Test* said no task constrains itself by file size, while `ci.yml` § *Harness guards* turned a PR red at `≥ 40,000` bytes over the same corpus. A red PR is a byte budget by another name, so the prohibition could not bind. Session `a47d904a` demonstrates the cost: `.claude/skills/task/SKILL.md` stood at **39 917** bytes, 82 under the 39 999 pass threshold, and a rename adding 7 bytes per site into that file turned the run into arithmetic — the site count was re-derived three times (6, then 7, then 8; projection 39 959 → 39 973; headroom 40 → 26), burning design and review rounds, and the spec grew a `Technical constraint 1` byte table plus an `AC11` requiring every further edit to that file to be "net-neutral-or-shrinking". No implementation subtask ran before the owner stopped the session.
**Gap, first half:** the hysteresis had no actuator in the region where it mattered. Extraction was owed only *after* a file reached 40 000 — i.e. after CI was already red — so a file parked just under the gate could neither grow nor obtain relief.
**Gap, second half:** `spec-writer.md`'s guard forbade byte-ceiling ACs "below the hard cap". `AC11` pinned to the hard cap itself and passed the guard untouched. A semantic boundary, moved by a reader with a delivery bias.
**Proposed edit:** retire the CI size step so `/ai-audit` is the sole enforcer; restate the AXIOM as an ownership table naming every other flow as forbidden to measure; widen the `spec-writer` guard from "below the hard cap" to any file-size figure at any threshold (all escalated in this same commit).
**at:** 2b7144e

### 2026-09-01 — `model: opus` on the quality gates was a floor that became a ceiling
**target:** `.claude/agents/{design,design-review,spec-writer,self-improve,self-reflect,self-review,review-findings}.md`, `.claude/skills/{interview,context-reset,task}/SKILL.md`, `ai-docs/agent-writing-style.md`, `ai-docs/claude-tools-hierarchy.md`
**Observed:** (1) Seven agents carry `model: opus`, imported as "Opus quality gates" (a717b24) when Opus was the top tier; the owner's session model is now a higher tier (`~/.claude/settings.json` `model`), so a top-tier orchestrator had its designs written and its design / spec gates judged one tier below itself. (2) `/interview` repeated the pin per call (`interview/SKILL.md:129`, `:154`); the documented resolution order is per-invocation `model` → frontmatter → `CLAUDE_CODE_SUBAGENT_MODEL` → session model, so the frontmatter was not the lever it claimed to be. (3) Prose disagreed with frontmatter: `task/SKILL.md:177` and `design.md:130` said `self-review` "stays Opus"; `self-review.md` never carried a `model:` line (inherits by omission) and `claude-tools-hierarchy.md:39` said "inherited". (4) `agent-writing-style.md:260` enumerated four of the seven pinned agents.
**Gap:** a fixed alias encodes "top tier" only while it IS the top; nothing re-derives it when a higher tier ships, and a per-call override layer duplicated the pin outside the file that owns it.
**Proposed edit:** the gates and the instructions/harness implementor spawn take the orchestrator's model — `model: inherit` in frontmatter (explicit on `self-review` / `review-findings` too; omission falls through the env var, `inherit` does not), no inline `model=` on any Step 8 or `/interview` spawn (closed rule); only cost-capped pins stay fixed (`code-writer` sonnet/medium; `learnings-escalation-audit`, `triage-runner` opus). Reader class replaces the enumeration in `agent-writing-style.md` (escalated in this same commit).
**at:** 0b8218b

### 2026-09-02 — the Spec Amendment recipe's design-review spawn template carries a `Context:` line the agent's closed list forbids
**target:** `.claude/skills/task/reference.md` (§ Spec Amendment recipe, step 6 template at `:62`); `.claude/skills/task/SKILL.md` Step 7
**Observed:** `.claude/agents/design-review.md:15-19` permits exactly five prompt items and raises `PROMPT-CONTAMINATION` (major) on anything else. `reference.md:62`'s template for the post-amendment design-review spawn reads "Context: spec was amended during a previous Step 7 GO-with-notes resolution — verify the design now matches the amended spec." — an orchestrator copying the template violates the contract. This session's Step 7 spawn (its own `Context:` line, same shape) was flagged; `SKILL.md:114` says only "per `design-review.md`" and restates none of the five items.
**Gap:** the template predates (or was never propagated from) the closed-list contract, and the skill's Step 7 gives no inline enumeration — so the one in-flow example an orchestrator meets is the contradicting one, while Step 10 does enumerate self-review's list inline.
**Proposed edit:** drop the `Context:` line from the `reference.md:62` template (the amended spec is visible on disk; the round number is the only permitted state carrier), and have `SKILL.md` Step 7 enumerate the five items inline exactly as Step 10 does for `self-review`; sweep `reference.md:17,27,52` for the `design` agent's spawn templates against `design.md`'s own contract, if it has one.
**at:** a11f637

### 2026-09-02 — `[measured:]` tags certify a statement without naming the input classes the probe sent
**target:** `.claude/agents/design.md` (§ Rules, the measured/derived tag contract); `.claude/agents/design-review.md` (the tag re-run check)
**Observed:** A round-1 design tag read `[measured: … upsert rows=1 both paths …]` for the balance upsert; the probe behind it had sent credits only. The upsert refuses every covered debit (`23514` on the proposed INSERT row). design-review round 1 re-ran the tagged probes, found them reproducible, and passed the tag — a reproducible probe of the wrong class certifies nothing about the class that matters. The defect surfaced two rounds later when the AC2 property was executed against a database.
**Gap:** the tag contract asks "was this measured?" and the reviewer's check asks "does the probe reproduce?"; neither asks "which input classes did the probe send, and are they the ones production will send?" A single-class probe therefore passes both gates.
**Proposed edit:** require a `[measured:]` tag on any data-modifying statement to list the input classes exercised (sign, nullness, row-present/absent, boundary sizes), and have design-review's probe re-run step compare that list against the classes the design's own algorithm will send — a missing class is a finding, not a pass. Cheapest form: one table row per statement in § Test Design, columns "statement · classes sent · classes production sends".
**at:** a11f637
