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
