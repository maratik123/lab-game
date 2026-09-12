# Progress: Goroutine-leak prevention — ownership rules and the gates that hold them — ACTIVE
_Updated: 2026-09-12 07:49_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-goroutine-ownership-rules-gates
**base_commit:** 4ff90da26c5bad135fdeb46e7ff2b0d3d7ca3157
**Last build:** PASS
**Issue:** #80
**Spec:** ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md
**current_step:** Step 8 — subtask 7 of 10 complete
**last_passed_gate:** go test -race ./cmd/bot/... | 2026-09-12T00:00Z | 22ecdf6
**entry_args:** 80

## Next action

**Do this immediately:** spawn Group A (subtasks 1–8) through `/context-reset` with `subagent_type="code-writer"`, per the design's `## Handoff plan`.

## Subtasks

- [x] 1. `.golangci.yml`: enable `containedctx` + `fatcontext`; pin `run.relative-path-mode: gomod` (D2); lift the issue-truncation caps (D3)
- [x] 2. `.golangci.yml` + `internal/store/basis_test.go`: enable `gocritic`'s `deferInLoop` and `govet`'s `nilness`
- [x] 3. `internal/ingest`: replace the retry loop's unstoppable timer with a wait-or-cancel helper (D5)
- [x] 4. `internal/scheduler`: bound the detached connection close with its own named-constant timeout (D7)
- [x] 5. `.golangci.yml` + 7 files: enable `forbidigo` with patterns and carve-outs (D1, D2, D4); annotate every surviving fresh-root-context site (D6)
- [x] 6. `internal/srcguard`: move the compiled-directory predicate in, fold the leak-guard's private copy into it (D9)
- [x] 7. `cmd/bot`: own HTTP client threaded to the Telegram client and canary legs, plus the closer releasing idle connections (D12)
- [ ] 8. `internal/gateguard`: the launch allow list + checker (D8, D9), the lint-configuration guard (D10), discriminating twins (D11)
- [ ] 9. `ai-docs/code-style.md`: the ownership rules and the reviewer's checklist; § Linter posture brought in line
- [ ] 10. Sweep every live surface for a claim this diff falsifies and fix each

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review returned GO on round 1; five issues (3 minor, 2 note) and three recommendations folded by `design-writer` round 2, no re-review per the Step 7 table.
- **Step 7**: the owner settled all four of the design's open questions — widening confirmed, caps lifted, subtask 4 lands here, the panic-recovery rule lands now; none became a spec row.
- **Step 8**: `base_commit` recorded as the post-design commit, so the self-review diff covers implementation only; spec and design were already gated by design-review.

## GO notes

| # | round | note | kind | route | resolution |
|---|-------|------|------|-------|------------|
| G1 | 1 | D1 specifies an exclusion rule on `^cmd/` and one on `_test\.go` but never says they are scoped to `forbidigo` alone | design-internal | folded | design § D1 @ 4ff90da |
| G2 | 1 | D7 defines detached as "the caller does not wait" … Applied literally that is false for three launches | design-internal | folded | design § D7 @ 4ff90da |
| G3 | 1 | D6 lists "the health listener's bind and its graceful-stop goroutine" among the surviving fresh-root-context sites | design-internal | folded | design § D6 @ 4ff90da |
| G4 | 1 | A `go` statement whose outermost enclosing declaration is not a `FuncDecl` … has no key | design-internal | folded | design § D8 @ 4ff90da |
| G5 | 1 | D9 argues why `main` packages are not carved out of the launch gate, but states the `_test.go` exclusion without a reason | design-internal | folded | design § D9 @ 4ff90da |
| G6 | 1 | The design's four § Open questions … are correctly routed to the owner and should stay there | orchestrator-routing | owner asked, all four answered | design § Open questions "None open" @ 4ff90da |
| G7 | 1 | D10's guard already parses `.golangci.yml`. Asserting in the same guard that the enabled set still contains … | design-internal | folded | design § D10 @ 4ff90da |
| G8 | 1 | Round-trip required: before Step 8, update the design doc to incorporate each note/recommendation above | process | discharged | design round 2 committed @ 4ff90da |

## Key discoveries (don't re-investigate)

- **`forbidigo`'s settings key is `forbid`, not `patterns`.** `golangci-lint run` accepts `patterns` silently — no warning, zero findings, the gate looks enabled and is not. `golangci-lint config verify` rejects it (`additional properties 'patterns' not allowed`, exit 3) and accepts `forbid` (exit 0). Verified by the orchestrator independently of the design. Nothing in `make verify` or CI runs `config verify` today.
- **The lint gate truncates by default and hides sites.** Under the default caps only the `internal/health` sites appeared; with both caps at `0` the scheduler, `internal/testdb` (×3) and `internal/tgtest` sites appear too. AC10's "every site" is unmeasurable without D3.
- **`path: ^cmd/` is meaningless without `run.relative-path-mode: gomod`** — run from a config outside the module root the anchor misses and every `cmd/` site is reported.
- **An exclusion rule with a `path` and no `linters` list switches every linter off for that path.** `cmd/` is clean today, so such a mistake goes red nowhere; D10's guard is what makes it structural.
- **`nolintlint` runs with `allow-unused` unset**, so a `//nolint` directive with no finding under it fails the gate — annotate what a re-run reports, never what a design row lists.
- **`go-ruleguard` is not reachable from this module** (`go mod why -m` → "main module does not need"), so the precise-loop-gate alternative to D4 would add a dependency.

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
| AC11 | NOT_TESTED |
| AC12 | NOT_TESTED |
| AC13 | NOT_TESTED |
| AC14 | NOT_TESTED |
| AC15 | NOT_TESTED |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
