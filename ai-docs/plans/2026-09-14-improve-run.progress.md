# /improve run — 2026-09-14 — progress

**base_commit:** 743f79b
**branch:** chore/2026-09-14-improve
**parent_skill:** /improve
**entry_args:** (none)
**current_step:** self-review
**last_passed_gate:** harness guards — pipe-status, size-measure, piped-gate, spawn-contract, gate-log-path, no-verify and script-shape suites; shellcheck over all 19 hook bodies and both new suites; check-script-shape; make comment-refs; actionlint on ci.yml; check-citations; check-harness-gaps-forge; relative-link check — all GREEN at 8f94ef9

## Decisions log

- **Step 5 (owner):** scope — "Сильные 7 (Recommended)": H1 pipe-status hook, H2 size-measure hook, C1 spawn-prompt blocks at /task Steps 7 and 10, C3 no `.go` suffix under `tmp/`, C6 code-writer doc-comment check, C9 `-count=1` for a new draw, C11 progress file never staged in the committing call.
- **Step 5 (owner):** all six auto-memory candidates answered `Surface`; each routes to Step 2b's "1 + names a workflow primitive" row and is held for second confirmation — no `Kind: validation` entry lands in this run, so no instruction-file edit derives from them.
- **Step 6 (eval):** H2 — baseline FAIL (size measured before reading), post-change PASS (hook refused, not worked around): applied and held. H1, C1, C3, C9, C11 — default baseline PASS, load-bearing baseline PASS, post-change PASS: out of instrument reach. C6 — baseline-exempt; post-change FAIL twice (property form: recalled and misapplied; procedure form: not recalled).
- **Step 6 (owner):** commit the out-of-reach proposals "H1 хук $? (Recommended), C1 шаблон спавна (Recommended), C3 .go под tmp/ (Recommended)"; C9 and C11 reverted from the tree. C6 reverted after its second FAIL.
- **Post-commit (owner):** "пункты 2-4 сделай, пункт 1 отдельным коммитом перед пушем и созданием пр, а пункт 5 - заведи issue" — items 2–4 recorded in `ai-docs/harness-gaps.md`; item 1 becomes a learnings entry in its own commit before push and PR; item 5 was checked against telego's `Bot.constructAndCallRequest` before filing, the suspected leak did not hold, and the owner chose "Не заводить (Recommended)".

## Self-Review (Round 1)

**Verdict:** APPROVE

Diff window `743f79b..fae7d09` (4 commits, 11 files). Spawn prompt: invocation line, `Progress:` path, commit range — within the closed list, no `PROMPT-CONTAMINATION`. No `Spec:` / `Design:` line: an `/improve` run, so the acceptance criteria are the owner decisions in `## Decisions log` above.

| # | File:line | Severity | Finding | Status |
|---|-----------|----------|---------|--------|

No `blocker` or `major` finding. **3 `minor`, below the severity floor** (register R1-1..R1-3): `.claude/skills/pr-ci-failed/SKILL.md`, `.claude/skills/main-ci-failed/SKILL.md`, `.claude/agents/code-writer.md` (R1-1); `AGENTS.md` (R1-2); `ai-docs/claude-tools-hierarchy.md` (R1-3).

**What was checked**

- **Scope against the owner decisions.** H1 (pipe-status hook), H2 (size-measure hook), C1 (literal spawn blocks at `/task` Steps 7 and 10), C3 (no `.go` suffix under `tmp/`) are present. C6 / C9 / C11 left nothing in the tree: `code-writer.md` is untouched, and the `-count=1` and doc-comment source entries still read `Escalated? no`. The five harness-gaps entries are appends to the designated parking log. The three from `fae7d09` are the owner's "пункты 2-4".
- **Hook MUST 1.** `shellcheck -s bash` over every body, read one JSON string per line: 19 bodies, 0 failures. A control body with a known defect fails shellcheck. A first attempt read 0 bodies through a broken delimiter and is discarded. Per-event counts from `jq`: SessionStart 2, PreToolUse 11, PostToolUse 5, Stop 1 — matches `claude-tools-hierarchy.md:33`.
- **Hook MUST 2, both new suites.** Both are green on the tree. Mutation, with suite copies pointed at mutated settings files that keep each body's message, left the repository untouched:
  - pipe-status never-block → 9 FAIL; always-block → 8 FAIL; line join removed → the multi-line fixture fails; escape hatch removed → 2 FAIL.
  - size-measure never-block → 12 FAIL; always-block → 13 FAIL; K1 exemption removed → 2 FAIL; assignment prefix removed → the `LC_ALL=` fixture fails; xargs branch removed → the xargs fixture fails.
  - Unmodified controls through the same harness are green.
- **Hook MUST 3.** Both new hooks fired on this reviewer's own real Bash calls: the size guard on a `wc -l` over `.claude/skills/improve/*.md`, the pipe-status guard on a `$?` read after a pipeline. So `tool_input.command` is populated on this path.
- **Audit exemptions.** The Sub-check 9 recipe, extracted multi-line from `checklist-m.md`, passes the live size body. `wc -l .claude/skills/*/SKILL.md` passes.
- **C1 templates against the live spawn-contract body.** Blocks extracted from `.claude/skills/task/SKILL.md`, placeholders realised:
  - Step 7 passes, with and without its `Progress:` line; Step 10 passes.
  - `<base-sha>..HEAD` left unsubstituted is refused, and so are the unrealised Step 7 block, lowercase `spec:` / `design:` / `round:` and snake-case `spec_path:` — every refusal the new prose claims holds.
  - `test-spawn-contract-guard.sh` is green.
- **C3 claims, in a scratch module outside the repository.** A broken `.go` under `tmp/_x/` plus a `.go.bak` → `go build ./...` exit 0. The same file as `tmp/y/bad.go` → exit 1 with `undefined: undefinedSymbol`. `git check-ignore -v tmp/probe.go` → `.gitignore:18:/tmp/`.
- **Other gates on the tree.** Green: `test-piped-gate-guard.sh`, `test-gate-log-path-guard.sh`, `test-no-verify-guard.sh`, `test-instruction-edit-guard.sh`, `test-harness-gaps-forge.sh`, `check-harness-gaps-forge.sh`, `check-script-shape.sh`, `check-citations.sh`, `actionlint .github/workflows/ci.yml`, shellcheck on both new suites, `make comment-refs`. The CI `harness` paths-filter includes `ai-docs/**` and `.claude/**`, so the two new suite lines are reached.
- **Escalated? backfill (`8f94ef9`).** 18 lines, only `Escalated?` lines. Each target carries a rule addressing the recorded shape:
  - `hook`: every recorded command is a suite fixture.
  - `AGENTS.md`: lines 98 and 238.
  - `skill:task`: the Step 7 and Step 10 blocks.
  - `skill:ai-audit`: the Sub-check 9 FORBIDDEN row.
- **Facts in the new harness-gaps entries.**
  - `f9834898fcbd1c6e6b0f0d6e21ef4b0d1e5e08b0` → `git cat-file -e` exit 128; `f983489` → exit 0.
  - The cited learnings headings resolve (learnings.md:91, 398, 412, 1009).
  - `f9f24ca`'s trailer reads `62 new tests; all 120 tests green.`
  - Step 11's "re-measure them each round" is at SKILL.md:256; Step 12 item 8 is at SKILL.md:279; the FORBIDDEN row's "No spec constraint, no AC … may name a file size" is at checklist-m.md:33.
  - `Derivability` and `Rule-citation observable` exist in `ai-docs/templates/improve-eval-reproducer.md`.
  - `Set.Depth` exists: `internal/gate/depth.go:37` (R1-4).
  - `743f79b` and `8f94ef9` resolve.
- **Propagation.** The Spawn group row now names Steps 7 and 10, and the new checklist-m row is present. A sweep of live instruction files for `wc`/`du`/`stat` found sites outside the audit recipes (R1-1). The `/ai-audit` skill carries no other measuring command the hook refuses.

**Not in this window, still owed before the PR.** The out-of-instrument-reach commits for H1, C1 and C3 need the owner decision recorded in the PR body, with all three runs attached (`ai-docs/improve-eval-contract.md` outcome table). The owner's item 1 becomes a learnings entry in its own commit before push.

## Review register

| id | raised | severity | status | verifying command |
|----|--------|----------|--------|-------------------|
| R1-1 | round 1 | minor | accepted@1 — below severity floor: `pr-ci-failed/SKILL.md:184` and `main-ci-failed/SKILL.md:177` name `wc -c` as a `harness`-class reproducer (the CI size gate it reproduced is retired), and `code-writer.md:65` names `wc -c` among Test Design checks; the new size hook refuses those over a covered file, while the Step 11 harness-gaps entry and the new propagation row scope this class to Step 11 and the audit recipes only. Fails closed, and a size check over the corpus is already FORBIDDEN outside `/ai-audit` | `rg -n 'wc -c' .claude/skills/pr-ci-failed/SKILL.md .claude/skills/main-ci-failed/SKILL.md .claude/agents/code-writer.md`; `bash ai-docs/scripts/test-size-measure-guard.sh` (fixture `BLOCK wc -c AGENTS.md`) |
| R1-2 | round 1 | minor | accepted@1 — below severity floor: `AGENTS.md:98` says the second hook "blocks reading `$?` right after any pipeline"; an unspaced pipe, `\|&`, a pipe ending a line and a backslash-continued pipeline all pass the live body (only the `&&` join and the unspaced form are listed as known misses, in `claude-tools-hierarchy.md:22`) | write `{"tool_input":{"command":"grep foo AGENTS.md \|& head; echo $?"}}` to `tmp/p.json` with the Write tool; `jq -r '.hooks.PreToolUse[].hooks[].command \| select(contains("exit status read after a pipeline"))' .claude/settings.json > tmp/pipe-body.sh`; `bash tmp/pipe-body.sh < tmp/p.json; echo "rc=$?"` — rc 0 = not blocked |
| R1-3 | round 1 | minor | accepted@1 — below severity floor: `claude-tools-hierarchy.md:23` calls both exemptions "verbatim", but the Sub-check 9 exemption is a substring match (a command carrying the recipe's `find …` clause followed by `; wc -c AGENTS.md` passes); `time wc`, `if wc`, `command wc`, backticks, `bash -c "wc …"` and `find … -exec wc` also pass and are unlisted among the known misses | write the payload to `tmp/s.json` with the Write tool (the size hook matches the command text); extract the body selected on `instruction-file size measured outside /ai-audit` to `tmp/size-body.sh`; `bash tmp/size-body.sh < tmp/s.json; echo "rc=$?"` — rc 0 = not blocked |
| R1-4 | round 1 | none | accepted@1 — not a defect: the reproducer harness-gaps entry quotes the scenario's `func (s *Set) Depth(c Cell) int`, while the tree has `func (s Set) Depth(cell hexgrid.Coord) (int64, bool)`; the entry's "the function exists in the tree" holds, and the paraphrase is the scenario's, which is the entry's point | `ast-index symbol "Depth"` |
| R1-5 | round 1 | none | accepted@1 — not a defect: `test-spawn-contract-guard.sh`'s header still says "the four spawn templates" after Steps 7 and 10 gained literal blocks; both block shapes are already fixtures, and the live body passes both blocks extracted from `SKILL.md` | `bash ai-docs/scripts/test-spawn-contract-guard.sh` |
| R1-6 | round 1 | none | accepted@1 — owner-routed: `/task` Step 11's "re-measure them each round" contradicts the new size hook; the owner sent it to `ai-docs/harness-gaps.md` (Decisions log, post-commit), where it stands as the 2026-09-14 Step 11 entry | `rg -n 're-measure' .claude/skills/task/SKILL.md` |
| R1-7 | round 1 | none | accepted@1 — not a defect: the `8f94ef9` backfill touches 18 `Escalated?` lines and nothing else, each target carries a rule addressing its entry's recorded shape, and the reverted proposals' source entries stay `no`; who authored the edit (subagent or parent) is not derivable from the tree, and the commit matches `self-improve.md` Commit B's shape | `git diff --stat 743f79b..fae7d09 -- ai-docs/learnings.md` |
