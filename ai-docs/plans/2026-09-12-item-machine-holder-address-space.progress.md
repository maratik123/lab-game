# Progress: Item machine — `item`, `item_movement`, and the shared holder address space — ACTIVE
_Updated: 2026-09-12 14:42_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-item-machine-holder-address-space
**base_commit:** 4d7d9b91561d3676367e593c144b32f7513c7d70
**Last build:** not run
**Issue:** #25
**Spec:** ai-docs/plans/2026-09-12-item-machine-holder-address-space.spec.md
**current_step:** Step 8 — Group A handoff, subtask 1 of 6
**last_passed_gate:** check-spec-anchors.sh + check-spec-shape.sh + check-ac-shape.sh | 2026-09-12T13:05Z | 155bbc8
**entry_args:** 25

## Next action

**Do this immediately:** spawn the Group A handoff (`code-writer`, subtasks 1–5) per the design's `## Handoff plan`; the first subtask is the forward migration `00006_capacity_kinds.sql` / `00007_item_machine.sql` plus the Go mirrors and the existing assertions the schema change moves.

## Subtasks

- [ ] 1. Forward migration + Go mirrors + the assertions the schema change moves  ← CURRENT
- [ ] 2. The schema's own refusals, by SQLSTATE and constraint name
- [ ] 3. The reconciliation views' tests, each anomaly class planted and seen red
- [ ] 4. `Move`: extract `post`, add `Movement` / `Move` / the sentinels
- [ ] 5. The `rapid` property test and the `-race` concurrency test
- [ ] 6. Propagation sweep over every live surface the diff falsifies

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 ITERATE (2 major), round 2 GO; the two majors were verified against the tree by the orchestrator before the ITERATE was relayed.
- **Step 7**: owner ruled KD-17's «No composite FKs» **scoped**, so D3's composite chain FK stands; recorded in the design with the owner's words, not in the spec.
- **Step 7**: owner ruled `docs/DESIGN.md` §11's plural table names **are** corrected in this PR, so subtask 6 edits `docs/**`.
- **Step 7**: the `[measured probe · …]` fourth claim-tag form is kept in the design and the underlying gap parked as a harness diagnosis; editing `.claude/agents/design-writer.md` is out of this task's scope and hook-blocked for the interview window.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | "D4's phase a says \"no instance named twice in one batch\", but `const NewItem ItemID = 0` is a single sentinel value" | design-internal | folded | design § D4 @ 4d7d9b9 — `NewItem` exempt from the duplicate check, multi-mint legal, return slice parallel to `movements` |
| G2 | 2 | "`[measured probe · …]` is a **fourth** tag form" | design-internal | folded | design § Open questions @ 4d7d9b9 — form kept; gap parked in `ai-docs/harness-gaps.md` @ d2a1f6b |
| G3 | 2 | "The Decomposition's `Serves` annotations name Scope 1, 2, 5, 6 but **not Scope 3** … or **Scope 4**" | design-internal | folded | design § Decomposition @ 4d7d9b9 — Scope 4 added to subtasks 1 and 3, Scope 3 to subtask 4; all six Scope items and all ten ACs now appear |
| G4 | 2 | "Two of the design's § Open questions must be **answered before the subtask they gate**" | design-internal | folded | both answered before Group A; design § D3 and § D9 @ 4d7d9b9 carry the owner's words |
| G5 | 2 | "the two highest-subtlety artefacts in the task … both land in the sonnet group" | design-internal | folded | design § Handoff plan @ 4d7d9b9 — the two artefacts named so Step 9 reads them rather than sampling |
| G6 | 2 | "D6's `item_chain_break` sketch elides the join … The `reached_movement <> recorded_movement` predicate is NULL-blind" | design-internal | folded | design § D6 @ 4d7d9b9 — join written out from `item`, every count `COALESCE`d, subtask 3's off-World-genesis scenario named as the discriminator |
| G7 | 2 | "§ A note on evidence certifies a property over the document's own tags" | design-internal | folded | design § A note on evidence @ 4d7d9b9 — the self-audit clause dropped, both pins kept |
| G8 | 2 | "D5's \"which one a holder enforces is configuration\" lists two levers … without saying which applies to which kind" | design-internal | folded | design § D5 @ 4d7d9b9 — `slots` by the budget lever alone, `weight` by the cost lever |
| G9 | 2 | "**whether two conditions share a sentinel** is [review's business]" | design-internal | folded | design § D4 @ 4d7d9b9 — one sentinel per refusal condition in a table, `ErrUnknownItem` and `ErrNotCurrentHolder` stated as never collapsing; § Test Design asserts `errors.Is` against the other is false |

## Key discoveries (don't re-investigate)

- `Move` must carry the mechanic's own postings, not only the derived capacity legs: with a `*PlayerOperation` basis a second call returns `ErrAlreadyPosted`, and with the other basis types it silently writes a second document. Verified against `internal/store/basis.go` before the design was changed.
- `appendOnlyPattern` names its tables one by one and stops at `event`; a new append-only table is invisible to it until listed. `item` deliberately stays out — #32 will backfill a definition column, which the scan would refuse.
- The `00006` / `00007` migration split is **forced** by the hygiene gate, not chosen: `addValueRe` fires on `ADD VALUE` only, so `CREATE TYPE … AS ENUM` may share a file with its use while a `ledger_kind` member may not.
- `CreateOwner` has no non-test caller today, but the design does not lean on it: `00007` backfills `scope` / `account` / `account_balance` for pre-existing owners under `ON CONFLICT DO NOTHING`.

## AC Status

| AC | Status |
|----|--------|
| AC1 | NOT_TESTED |
| AC2 | NOT_TESTED |
| AC3 | NOT_TESTED |
| AC4 | NOT_TESTED |
| AC5 | NOT_TESTED |
| AC6 | NOT_TESTED |
| AC7 | NOT_TESTED |
| AC8 | NOT_TESTED |
| AC9 | NOT_TESTED |
| AC10 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- _(none yet — Group A has not returned)_
