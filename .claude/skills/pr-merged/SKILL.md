---
name: pr-merged
description: "After a PR merge: switch to main, pull, delete the fallback progress files of PRs no flow produced, and delete the local PR branch. Flow-owned state files are retired by their own flow before its PR and are left alone."
disable-model-invocation: true
allowed-tools: Bash(git checkout main) Bash(git pull) Bash(git pull *) Bash(git branch -d *) Bash(git branch --show-current) Bash(git status) Bash(git status --porcelain) Bash(.claude/skills/pr-merged/scripts/cleanup-progress.sh *)
---

> Near-stateless: no `.progress.md` discipline applies; re-entry consists of re-invoking the skill.

Current branch: !`git branch --show-current`

Working tree:
```!
git status --porcelain
```

If the current branch is `main`, stop and tell the user this skill must be run while standing on the merged PR branch.

If `git status --porcelain` above is non-empty (any modified, staged, or untracked entries), stop and ask the user how to proceed (commit, stash, discard, ignore). Do not run any further commands until the user answers.

Otherwise, run in order. **Capture `<previous-branch>` from the `Current branch:` value above before step 1** — once `git checkout main` runs, `git branch --show-current` no longer returns it.

1. `git checkout main`
2. `git pull`
3. **Delete the merged branch's fallback progress files only.** The `/task` and `/main-ci-failed` state files are **not** deleted any more: each flow retires its own into an ignored directory before opening its PR (`/task` Step 12 sub-step 9a, `/main-ci-failed` Step 7 sub-step 1a), they are out of every probe, and they are the run's only working copy of a record that took hours to produce. What still gets removed is the fallback surfaces created for PRs no flow produced — `ai-docs/pr-comments/`, `ai-docs/ci-fixes/` — which were never tracked and have no retire step. Run the cleanup script:

   ```bash
   ${CLAUDE_SKILL_DIR}/scripts/cleanup-progress.sh <previous-branch>
   ```

   The script encapsulates the PR-linkage derivation (PR number → spec lookup → progress-file paths) and handles the failure modes:
   - **`PR_NUM` empty** (branch merged outside `gh`, or PR is closed-not-merged): prints `pr-merged: no merged PR found for <previous-branch>; skipping progress-file cleanup.` and exits 0.
   - **No fallback file on disk** (the usual case for a `/task`-produced PR, which retired its own state): `rm -f` is silent on missing files and the script is idempotent.
   - **`rmdir` on `ai-docs/pr-comments`** is opportunistic — non-fatal if the directory has unrelated files or doesn't exist; exit code ignored.

   Deferred-task plan files (`ai-docs/plans/deferred/`) and every retired state file under `ai-docs/plans/ignored/` and `ai-docs/main-ci/ignored/` are intentionally NOT touched. Source: [`scripts/cleanup-progress.sh`](scripts/cleanup-progress.sh).

4. `git branch -d <previous-branch>` — always `-d`, never `-D`. If `-d` refuses because the branch is not fully merged, stop and report the message; do not force-delete.

## Patterns

### 1. Auto-delete the merged local branch without a confirmation pause

*Default to* running `git branch -d <previous-branch>` immediately at step 4 of the workflow without pausing for user confirmation. *Prefer* the silent auto-delete over an `AskUserQuestion` prompt — `git branch -d` is the safe form (refuses to delete unmerged branches), and the user has already invoked this skill specifically to perform cleanup; pausing for a yes/no on a guaranteed-safe operation is friction without protection. If `-d` refuses (branch not fully merged), stop and report; do NOT escalate to `-D` without an explicit user instruction.

Validated by user feedback recorded in the sibling **quartzite** project's memory namespace — `~/.claude/projects/-home-syt-RustroverProjects-quartzite/memory/feedback_remove_merged_branches.md` (`type: feedback`): "Remove merged local branches without asking — `git branch -d` merged branches after `git pull`; do not pause to confirm". That file also carries the `-d`-not-`-D` rationale above. Surfaced by a **quartzite** `/improve` run (2026-05-22, Step 1c) — not a run in this repo, whose log begins 2026-08-29. The memory is user-local to that project: this skill's namespace resolves from `pwd`, so a lab-game sweep will never see it, by design.
