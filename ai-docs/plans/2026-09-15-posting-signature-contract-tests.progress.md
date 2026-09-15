# Progress: posting-signature contract tests — ACTIVE
_Updated: 2026-09-15 12:21 UTC_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-15-posting-signature-contract-tests
**base_commit:** 47b42e978cafc9f0a991ce3267f5b6612de44ec4
**Last build:** not run

**Issue:** #26
**Spec:** ai-docs/plans/2026-09-15-posting-signature-contract-tests.spec.md

**current_step:** Step 8 — Implementation start
**last_passed_gate:** none yet
**entry_args:** 26

## Next action

**Do this immediately:** Group A (code-writer, Mode A) — subtasks 1–6 of ai-docs/plans/2026-09-15-posting-signature-contract-tests.design.md § Decomposition, in order, one commit per subtask.

## Subtasks

Group A — code, `sonnet`/`medium` via `code-writer`:
- [ ] 1. The shared pool fixture — `internal/storetest` (D13)  ← CURRENT
- [ ] 2. Move the scheduler, ingest and bot suites onto `storetest.Pool` (D14)
- [ ] 3. Package scaffold, vocabulary and registry — `internal/contract` (D1–D5, D12)
- [ ] 4. The pure matcher (D4, D7, D11)
- [ ] 5. The contract check against real transactions (D6, D11)
- [ ] 6. The declared set, then close the group with the whole-module gates (D8, D10)

Group B — instructions/harness, `inherit` via `general-purpose`:
- [ ] 7. Documentation (KD-42, domain-invariants, context.md layout line)

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review round 1 GO; its notes 1–4 folded (e6bced8); note 5 put to the owner, whose answers 3.1 and 3.2 narrowed the scope — spec amended in interview round 4 (7883ed1), issue #26 body updated.
- **Step 7**: design reconciled with the amended spec (b4b1240); design-review round 2 GO; its notes and recommendations folded (4fc6316).
- **Step 7**: GO note 3 (pool-helper duplication) raised an owner-scope question; owner answers 5.1 «Да, перевести» and 6.1 «Да, со всеми вызовами» — D14 and subtask 2 (3e5762b, 47b42e9). Pre-amendment round-1 GO notes are no longer authoritative (Spec Amendment recipe step 7).

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 2 | D6 does not define the mark when no journal entry exists yet. | design-internal | folded | design § D6 zero mark, `TestNewMark_emptyJournal` @ 4fc6316 |
| G2 | 2 | § Risks says "A cancelled-context test drives each read's query-error branch" | design-internal | folded | design § D11 reads-before-judging, `TestReads_returnEveryError`, § Risks @ 4fc6316 |
| G3 | 2 | The migrated schema-pool helper (`testdb.Schema` → `store.NewPool` → `store.Migrate`) is already copied per package in 4 places | design-internal | folded | design § D13 @ 4fc6316; owner-scope extension § D14 + subtask 2 on answers 5.1 and 6.1 @ 47b42e9 |
| G4 | 2 | Two out-of-remit sentences. | design-internal | folded | both sentences removed @ 4fc6316 |
| G5 | 2 | D8's rules for the literal (catalog codes, named sign and cardinality, no helper) do not require one leg per line. | design-internal | folded | design § D8 "each leg on its own line" @ 4fc6316 |
| G6 | 2 | In the Test Design fixture list, label the test registry's rows as made-up shapes that borrow existing type codes, not the §11 signatures. | design-internal | folded | § Test Design fixture label @ 4fc6316 |
| G7 | 2 | `internal/store/errors.go:10`, cited in D1, points at the comment above `ErrNoBasis`; the declaration is on line 11. | design-internal | folded | D1 citation re-pinned to line 11 @ 4fc6316 |

## Key discoveries (don't re-investigate)

- No production code writes under any basis document today: the framework is exercised by its own test fixtures only.
- `internal/store`'s own pool helper cannot move to `storetest` (import cycle, measured in D13).

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

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|

## Files touched

- (none yet)
