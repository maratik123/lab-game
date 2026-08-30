# Harness restart metrics

Baseline: orchestrator session `ec78f817` (2026-08-29/30, force-stopped at ~18 h, 0 PR),
measured during its forensics (`FORENSICS.md` on branch `forensics/ec78f817`; transcript
line coords `L<n>`). This page exists so the first `/task` run after the harness-forge
commit is judged against numbers, not impressions. It is a one-off measurement doc, not
an instruction file; delete or archive it after the comparison is made.

## How to measure a run

`T` = the session transcript `~/.claude/projects/-home-syt-lab-game/<session>.jsonl`;
`P` = the task's `*.progress.md`; `B` = the task branch.

| # | Metric | Baseline (ec78f817) | Target | Command / where to look |
|---|---|---|---|---|
| 1 | Self-review rounds to APPROVE | 10 rounds, 0 APPROVE (force-stopped) | APPROVE ≤ 3 rounds, or a tripwire STOP surfaced to user | `grep -c '## Self-Review' P`; verdicts: `grep '\*\*Verdict:\*\*' P` |
| 2 | Re-litigation share per round | 52% overall (48 of 92 findings from round 2 on cited an earlier round); round 3 hit 90% | < 50% every round (tripwire fires otherwise) | register rows `open 🔁@` ÷ rows raised, per round |
| 3 | Findings pinned to the round cap | rounds 1–8: exactly 10 findings each (cap = quota) | count varies round to round; no clustering "to stay inside the cap" | per-round row counts in P |
| 4 | Threshold/label distortion in flight | 1 (hard 500), lived 7 h 26 m, shipped to AGENTS.md + 2 reviewer charters | 0 rows without `[source:` in the spec; 0 user corrections of a constant | `grep -nE '[0-9]{3,}' spec.md \| grep -v 'source:'` should return nothing threshold-shaped |
| 5 | Hand-off shape | 15,623-char ориентировка injected as the task; scope directives inside it | TASK byte-identical to the user's text; DELTA line present; RECON carries NOT READ: | diff the `## TASK` block in T against the user's `/task` args |
| 6 | Pushes / PR visibility | 142 commits, 0 pushes, 0 PR | draft PR exists after the first Step-8 group return; every fix round pushed | `gh pr view --json isDraft,commits`; `git log origin/B` |
| 7 | Cap arithmetic | caps compounded ×2 then ×3 → 36/18 | any raise echoes `cap: N (was M)`; N never exceeds charter × the last user multiplier | grep `cap:` in T |
| 8 | learnings.md growth & genre | +27 entries in one session; 7 `HARNESS*`-prefixed; 6 entries about entries | conduct corrections only in learnings.md; harness diagnoses in harness-gaps.md; 0 entries about entries | `grep -c '^### ' ai-docs/learnings.md ai-docs/harness-gaps.md` |
| 9 | Corrective user interventions | 24 corrective messages; chains B (announce-not-act) and C (over-tighten) recurred 4× each | no single correction class recurs after its first statement | count corrective turns in T; Stop-hook blocks visible in T for chain B |
| 10 | Declined amendment triggers | 1 of 3 reviewer spec-triggers closed in-thread | 0 closed in-thread (each executed or surfaced verbatim) | grep `Amendment trigger` in P; each must have an exit recorded in the register |
| 11 | Instruction-file size drift | AGENTS.md pushed into the 35k band mid-task by AC self-inflation; project-gates.md grew 22,193 → 36,103 B on review prose alone | fix rounds re-measure size ACs; no AC fails between rounds unnoticed | register verifying-command outputs per round |

## Reading the result

- Metrics 1–3 and 10 test the E5/E6/E7/E8 cluster (register + termination + verdict duty).
- Metrics 4–5 test E1/E2 (derivation + hand-off schema). Metric 4's zero is the direct
  falsifier of the §1 failure class.
- Metric 6 tests E14; metric 7 tests E6's arithmetic; metrics 8 tests E9/E10; metric 9
  tests E11 (and is the honest one: dispositions don't transfer as text, so what is
  measured is whether the GATES fired, not whether conduct improved).
- A run that stops early via the tripwire with a clean register is a SUCCESS for this
  comparison even without an APPROVE: the loop surfacing non-convergence beats the loop
  consuming it.
