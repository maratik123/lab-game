# Delegation — the five-phase lifecycle

> Extracted from `AGENTS.md` § *Workflow*. That file keeps the binding rules; this page carries the mechanics and the incidents that produced them. **Read before any Subagent spawn that commits, edits protected files, or runs long.**

Delegation fails in the middle, not at the ends. Most instruction text covers phase 1 (decide to delegate) and phase 4 (read the result); phases 2 and 3 are where the observed failures actually land.

## Phase 0 — Outbound: every load-bearing claim in the prompt is executed first

Phases 1–4 govern what comes **back**. This one governs what goes **out**, and it is the phase with
no prior coverage: `AGENTS.md` § *Patterns* 1 makes an **intermediary's** claim a claim and says
nothing about one you originate.

**MUST — any load-bearing numeric, limit, property or invisibility claim entering a delegate prompt,
a design amendment, or a ruling relayed to a delegate is executed against the tree first, whoever
authored it.** A claim you originate feels like reasoning rather than a citation, so it never trips
the inbound check.

| Claim shape in a draft prompt | Run before sending |
|---|---|
| An invisibility premise — *"no suite sees it"*, *"this is unreachable"*, *"nothing catches this"* | Execute it against the named artefact. If you cannot, forward the remedy **WITHOUT** the premise |
| A numeric / overflow / boundary claim | Evaluate it **and its neighbouring values** — the argument's own value confirms the argument; its neighbours are where an unrelated defect shows itself |
| A strengthened guarantee — *merely* X → *strictly* X, "for most" → "for every" | Enumerate the inputs the validation admits and evaluate at the **edges** of that set, never at the defaults, where every existing test already lives |
| A premise computed under an exception the ruling itself removes | Re-derive it against the tree **after** the ruling is stated. Tell: a sentence of the form "only X can happen", written while arguing to abolish the rule that made the other cases not count |
| A constraint justified by what the user did **NOT** say | **NEVER send it.** Silence on an axis is not a position on it; removing a constraint is the whole of the correction, and what it used to decide reverts to its ordinary owner |

**A delegate answering *"confirmed by measurement"* is not cover** — check WHICH half it measured,
because the half it can verify cheaply is the half it will verify. A premise travels further than
the rule it is attached to: the rule gets reviewed, the premise gets copied.

## Phase 1 — Fit: can this delegate really do this work?

Check the delegate's **charter** AND its **environment**. A step that says "delegate" is evidence about CAN, never about fit, and neither is a `tools:` grant.

- **Charter fit.** `code-writer` is a *code* implementor. A predominantly-prose diff (`.claude/**`, `ai-docs/**`, `*.md`) has no code to delegate — author it in-thread. Its Go gates are also meaningless on a diff with no `.go`.
- **Environment fit.** A *background* Subagent cannot answer an interactive or self-modification permission prompt. A protected-file edit therefore **fails closed** regardless of what `Edit(...)` allow-lists say. Apply those in-thread.

## Phase 2 — Hand-off state: leave the index CLEAN

**`git commit` captures the whole index, not just what the delegate's own `git add` added.** Anything you pre-staged — a `git mv`, a half-finished edit — lands in the delegate's first commit, attributed to its work and mixed into its diff.

Before delegating to a subagent that commits: either commit your own staged work first, or `git restore --staged <path>` to keep it working-tree-only.

## Phase 3 — While it runs: waiting is not stuck

A delegate that ends its turn to wait on a long job is **waiting**, not hung.

Do **not** start a parallel investigation of the same question. It duplicates the work and steals CPU from the very job you are impatient about — the two racing processes make each other slower, which reads as further confirmation that it is stuck.

Verify what it is actually doing (`ps`, its committed output) before judging. If it lacks information you already have, send it that information plus an explicit decision rule. Take over only after stopping it — and say so.

## Phase 4 — Take-over and return: the summary is a claim

**A subagent's RETURN SUMMARY is a claim, not a record.** After any group or subagent returns, verify every gate / PASS / "I did X" assertion against the **durable** record — the `.progress.md`, the commit body, `git log` / `git diff`, the file itself. When they disagree, trust the durable record, and close the gap in-thread — re-run the gate yourself rather than accepting the summary's word for it.

This applies with extra force to a delegate that also writes shared files. Both of these have proven false against the durable record in the **graphite-gp** project, whose harness this one inherits: a `code-writer`'s *"I appended a learning"*, and its *"golden artefact verified by a checking subagent → PASS"*.

Treat a delegate's stated **reason** for skipping a gate as suspect too — especially when its other observed behaviour contradicts it.

**If you revert and re-author its work by hand**, treat its touched-file list as a **checklist**. The sub-edits — propagations, sync-group siblings — are exactly what a manual redo drops. Run the `AGENTS.md` § *Propagation Rule* grep before commit, unconditionally.
