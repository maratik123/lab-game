# Progress: Goroutine-leak prevention — ownership rules and the gates that hold them — ACTIVE
_Updated: 2026-09-12 08:44_

> Read THIS FIRST → ready to continue. No need to re-read the codebase.

**Branch:** feat/2026-09-12-goroutine-ownership-rules-gates
**base_commit:** 4ff90da26c5bad135fdeb46e7ff2b0d3d7ca3157
**Last build:** PASS
**Issue:** #80
**Spec:** ai-docs/plans/2026-09-12-goroutine-ownership-rules-gates.spec.md
**current_step:** Step 9.5 — docs updated
**last_passed_gate:** make verify | 2026-09-12T09:32Z | 80b5f22
**entry_args:** 80

## Next action

**Do this immediately:** run the Step 9 per-AC sweep now that subtask 1a/8a is implemented and gated.

## Subtasks

- [x] 1. `.golangci.yml`: enable `containedctx` + `fatcontext`; pin `run.relative-path-mode: gomod` (D2); lift the issue-truncation caps (D3)
- [x] 2. `.golangci.yml` + `internal/store/basis_test.go`: enable `gocritic`'s `deferInLoop` and `govet`'s `nilness`
- [x] 3. `internal/ingest`: replace the retry loop's unstoppable timer with a wait-or-cancel helper (D5)
- [x] 4. `internal/scheduler`: bound the detached connection close with its own named-constant timeout (D7)
- [x] 5. `.golangci.yml` + 7 files: enable `forbidigo` with patterns and carve-outs (D1, D2, D4); annotate every surviving fresh-root-context site (D6)
- [x] 6. `internal/srcguard`: move the compiled-directory predicate in, fold the leak-guard's private copy into it (D9)
- [x] 7. `cmd/bot`: own HTTP client threaded to the Telegram client and canary legs, plus the closer releasing idle connections (D12)
- [x] 8. `internal/gateguard`: the launch allow list + checker (D8, D9), the lint-configuration guard (D10), discriminating twins (D11)
- [x] 9. `ai-docs/code-style.md`: the ownership rules and the reviewer's checklist; § Linter posture brought in line
- [x] 1a/8a. **Amendment follow-on (design rounds 3–4):** `.golangci.yml` gains `issues.uniq-by-line: false` beside the two caps (subtask 1's amended half); `internal/gateguard`'s lint-configuration guard asserts the key is present and boolean `false` (subtask 8's amended half), with the § Test Design scenario that drives the ABSENT-key case; subtask 5's dedup control
- [x] 10. Sweep every live surface for a claim this diff falsifies and fix each

## Decisions log

Append-only, one line per non-trivial decision. Each line is prefixed with the step or phase that made it. Never edit or remove prior entries.

- **Step 7**: design-review returned GO on round 1; five issues (3 minor, 2 note) and three recommendations folded by `design-writer` round 2, no re-review per the Step 7 table.
- **Step 7**: the owner settled all four of the design's open questions — widening confirmed, caps lifted, subtask 4 lands here, the panic-recovery rule lands now; none became a spec row.
- **Step 8**: `base_commit` recorded as the post-design commit, so the self-review diff covers implementation only; spec and design were already gated by design-review.
- **Step 8**: Group A returned; the orchestrator re-ran the gates itself at 26d0d8d rather than accepting the return summary — `go build`, `go vet`, `golangci-lint fmt -d`, `golangci-lint run` (`0 issues.`) and `golangci-lint config verify` all green. `last_passed_gate` corrected from the delegate's `91904f8` / `00:00Z` placeholder to this measured run.
- **Step 8**: the detached close's bound is `5 * time.Second`, the value of the `pingTimeout` precedent the design named as its shape; the design fixed the shape, not the number, and no free choice outside the design survived.
- **Step 9**: the per-AC sweep found `issues.uniq-by-line` (default `true`) suppressing a second finding on a line another linter already claimed — `deferInLoop` vanished behind `errcheck` in a constructed probe (19 findings with it on, 22 with it off). AC8 itself holds (a defer-in-loop on a clean line fails the gate, exit 1); the gap is AC10's enumerability and D3's stated purpose. The real tree is `0 issues.` either way.
- **Step 9**: routed to the owner as a scope deviation rather than folded in, because their D3 confirmation named two specific keys. Their answer: "Add + re-review. Add `uniq-by-line: false` beside the two caps. design-writer amends D3; design-review re-runs (round 2 of 3). The unconditional-re-review default — slowest, most checked." Design Amendment triggered.
- **Step 8 (subtask 9)**: § Linter posture's enumeration was made **complete** against `.golangci.yml`'s `linters.enable`, not merely extended by this task's five additions. "Bring the enumeration in line with the enabled set" has no reading under which ten already-omitted linters stay omitted; the bullet became a table, one row per enabled linter plus the two analyzer sets switched on inside `gocritic` and `govet`.
- **Step 8 (subtask 9)**: the Propagation Rule's Review-checklist group fired. `ai-docs/propagation-groups.md` requires a rule added to `ai-docs/code-style.md` that leaves any half to review to gain a judging row in BOTH `.claude/agents/self-review.md` and `.claude/agents/review-findings.md`; the new ownership rules are largely review-judged (the lint gate and the launch allow list carry only part), so both files gained a goroutine-ownership row in subtask 9's own commit. The design's subtask-9 file list named `ai-docs/code-style.md` alone — the AXIOM in `AGENTS.md` § Propagation Rule outranks a design's file list, and subtask 10's charter is explicitly open-ended ("the class is every live site", not the list drafted there).
- **Step 8 (subtask 9)**: gates run on the markdown-only tree before the commit — the CI relative-link check (controlled against a constructed broken link, seen RED, then GREEN), the six `ai-docs/scripts/check-*.sh` harness guards, `make comment-refs`, and the Go side unchanged but re-run anyway: `make fmt-check build vet file-limits tidy-check import-guard` and `make lint` (`0 issues.`). `make test`/`test-race` were not re-run: `git diff --stat 26d0d8d..` shows no `.go`, `.sql`, `go.mod` or `go.sum` file in this subtask, so the suite's inputs are byte-identical to the tree the orchestrator already gated at 26d0d8d.
- **Step 8 (subtask 10)**: the sweep changed four files and eight claims — KD-16's linter enumeration and KD-32's closer-list decision (`ai-docs/key-decisions.md`); the orientation page's package layout, which had no `internal/gateguard`, and its gate list (`ai-docs/context.md`); the lifecycle page's step-11 row, the unwind columns of steps 12 and 13, and the drain's closer walk, each of which now carries the process HTTP client (`ai-docs/process-lifecycle.md`); and `ai-docs/go-test-conventions.md`'s enumeration of `internal/srcguard`'s mechanical half, which gained the compiled-directory predicate subtask 6 moved in.
- **Step 8 (subtask 10)**: the lifecycle page's **"No goroutine of this module survives the drain"** was the one claim the diff falsified outright rather than left incomplete. `internal/gateguard`'s allow list, added by subtask 8, records the scheduler's deadline watchdog as joined by nothing — the module's one deliberately detached launch — so the unqualified sentence could not stand beside it in the same pull request. It now states the exception and names #81 as where reclaiming the handler under it lives. The watchdog predates this diff; what the diff changed is that the detachment is now written down.
- **Step 8 (subtask 10)**: `ai-docs/context-status.md` was in the design's drafted file list for this subtask and got **no** change. Its entries are the per-task implementation log written by `/task` Step 9.5, and neither the composition-root entry nor the goroutine-leak entry carries a claim this diff falsifies (both read line by line, including the closer-list and leak-detection bullets). This task's own entry is Step 9.5's to write, not subtask 10's.
- **Step 8 (subtask 10)**: three surfaces were read and deliberately left unchanged, each with its reason. `.claude/skills/project-review/SKILL.md` is the Review group's third member, so subtask 9's edit obliged a check — it carries no per-topic finding checklist, delegating to `review-findings.md` and `self-review.md`, so the obligation is discharged by the check with no edit. `.claude/agents/design-writer.md` and `design-review.md` illustrate binding lint constraints with `exhaustive` / `revive` / `rowserrcheck` and already instruct the reader to open `.golangci.yml`; no propagation row binds them to a lint-config change and nothing there is falsified. `ai-docs/go-test-conventions.md`'s "only `internal/health` and `internal/ingest` moved onto it" is a statement about the scope of the task that introduced `internal/srcguard`, not about today's importer set (which was already seven files before this branch), so it is history, not drift.
- **Step 8 (subtask 10)**: gates re-run at 239c9f1 with a markdown-only working tree — the CI relative-link check, the six `ai-docs/scripts/check-*.sh` harness guards, `make comment-refs`, and `make lint` (`0 issues.`). No `.go`, `.yml`, `.sh`, `.sql`, `go.mod` or `go.sum` file is touched by subtask 10 (`git status --short` lists four `.md` paths), so the Go and harness-shellcheck gates read the same inputs they read at 239c9f1.
- **Step 9 (subtask 1a/8a)**: `.golangci.yml` gained `issues.uniq-by-line: false` beside the two caps; `internal/gateguard/guard_test.go` gained `isFalseBool` (present-and-boolean-`false`, distinct from `isZeroInt`'s numeric zero because the key's own default is `true`), the `checkLintConfig` assertion using it, the `configOpts.uniqByLine`/`omitUniqByLine` fields and their rendering, and `TestLintConfigGuard_UniqByLineFails` driving both the explicit-`true` and the absent-key case as its own scenario, per the design's Test Design table for subtask 8. `golangci-lint config verify` accepted the amended file (exit 0); `go build ./...`, `go test ./...` (all packages `ok`, `internal/gateguard` included), `go vet ./...` and `golangci-lint run` (`0 issues.`) all re-ran green at fd5b27c plus the working-tree edit.
- **Step 9 (subtask 1a/8a)**: subtask 5's dedup control (design § Test Design, "The line-dedup switch gets a control of its own") re-run against the shipped configuration rather than only the design's earlier investigation copy: a scratch module outside the tree with a `defer` inside a `for` loop, `.golangci.yml` copied in verbatim with `--enable-only=gocritic,errcheck` — `uniq-by-line: false` (as shipped) reports both `main.go:8:16: errcheck` and `main.go:8:3: deferInLoop (gocritic)`; the same file with `uniq-by-line: true` (the linter's own default) reports only `errcheck` on that line and drops `deferInLoop`. Run in the session scratchpad, never in the working tree.
- **Step 9**: ALL PASS at `80b5f22`. No panic-index row added — no `panic(` or `log.Fatal` in any non-test file this branch changed, and the index keeps its empty row. No posting signature and no event-dictionary row: the diff touches no migration, `.sql`, ledger, posting or event surface, which matches the spec's "Metrics: none. Events: none."
- **Step 9**: AC13's `README.md` clause has no target — this repository has no README. Recorded as such in the AC table rather than reported as a verified clean pass, because a sweep over a path that does not exist is an absent instrument, not evidence.
- **Step 9.5**: `context-status.md` gained this task's entry with the literal `#TBD-at-Step-12` locator (exactly one occurrence, which is what sub-step 10a's `Edit` needs). `context.md`'s Status date was still `2026-09-11` after the branch had edited that page — every one of the last five tasks touching it moved the date, so it was a missed propagation, not a convention; bumped to `2026-09-12`. No open question in `context.md` was resolved by this task.

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

| AC | Status | How it was verified (the orchestrator's own command) |
|----|--------|------|
| AC1 | PASS | `TestLaunchGuard_UnknownKeyFails` drives the real `checkLaunches` over a scratch file holding an actual `go func(){}()` against an empty table; `TestLaunchGuard_RealTreePasses` over the tree |
| AC2 | PASS | `launchAnswer`'s four fields plus `missingField()` make it structural; `TestLaunchGuard_EmptyAnswerFails`; the table's 11 answers equal the 11 `go` statements in compiled non-test source, counted independently |
| AC3 | PASS | constructed violating module under the shipped config: `time.Tick` and `time.After` both reported by `forbidigo` |
| AC4 | PASS | same module: `context.Background` and `context.TODO` both reported by `forbidigo` |
| AC5 | PASS | all 8 `//nolint:forbidigo` directives read: each states a reason and names an owner; the one detached site (the scheduler watchdog) carries `detachedCloseTimeout` |
| AC6 | PASS | same module: `containedctx` reports the struct-held context |
| AC7 | PASS | same module: `fatcontext` reports BOTH halves — `nested context in loop` and `nested context in function literal` |
| AC8 | PASS | same module: `deferInLoop` reported — it was SUPPRESSED before the amendment by the line dedup; a second module whose only reportable shape is the defer exits 1 on its own |
| AC9 | PASS | same module: `nilness` reports BOTH halves — `tautological condition: nil == nil` and `nil dereference in load` |
| AC10 | PASS | `golangci-lint run` over the tree → `0 issues.`; the amendment closed the line-dedup blind spot that made "every site it would otherwise report" unenumerable |
| AC11 | PASS | `ai-docs/code-style.md` § Concurrency → Ownership rules 1–12 carry the twelve rules of Scope item 6, one for one |
| AC12 | PASS | the same section's reviewer's checklist carries all five categories — per `go` statement, channel, context, resource, pull request — and each question Scope item 7 enumerates |
| AC13 | PASS (one clause has no target) | swept the live instruction files and `docs/`; the `docs/DESIGN.md` goroutine sentence is about session state, not the module's goroutine set, so the diff does not falsify it. **This repository has no `README.md`**, so that clause of the criterion has nothing to check — recorded rather than reported as a clean pass |
| AC14 | PASS | `forbidigo` reported in `internal/testdb` (×3) and `internal/tgtest` — non-test files of packages only tests import — and each required annotation on the same terms as production; the exclusion-reach guard keeps it so |
| AC15 | PASS | `TestServe_ReleasesTheProcessHTTPClientsIdleConnectionsOnDrain` goes through the real `assemble`+`serve`, counts exactly 1 `CloseIdleConnections` after the drain, then strips the closer and requires 0 — red when the mechanism is removed |

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
