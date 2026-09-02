#!/usr/bin/env bash
#
# Delete the local progress files belonging to the merged branch passed as $1.
#
# Run by `/pr-merged` skill (.claude/skills/pr-merged/SKILL.md, step 3) after
# `git checkout main && git pull`, before `git branch -d <previous-branch>`.
#
# Derivation (PR linkage):
#   1. `gh pr list --state merged --head <branch>` -> merged PR number
#   2. Delete ai-docs/pr-comments/pr-<PR_NUM>.progress.md (the fallback path
#      used by /pr-commented for PRs not produced by /task)
#   3. Delete ai-docs/ci-fixes/pr-<PR_NUM>.progress.md (the fallback path
#      used by /pr-ci-failed for PRs not produced by /task)
#   4. `rmdir ai-docs/pr-comments ai-docs/ci-fixes`
#      -- opportunistic cleanup; non-fatal if the directories still have
#      unrelated files or do not exist.
#
# WHAT THIS SCRIPT NO LONGER DELETES, and why:
#   The /task progress file, the /interview `.state.md` sibling and the
#   /main-ci-failed per-run progress file are each COMMITTED while their
#   flow runs and RETIRED by that flow into an ignored directory as the
#   last commit before its PR (/task Step 12 sub-step 9a ->
#   ai-docs/plans/ignored/; /main-ci-failed Step 7 sub-step 1a ->
#   ai-docs/main-ci/ignored/). Retiring already achieves everything the
#   deletion here was for: the files are out of the PR diff, out of the
#   repo, and out of the `ls ai-docs/plans/*.progress.md`,
#   `ls ai-docs/plans/*.spec.md.state.md` and `ls ai-docs/main-ci/*.progress.md`
#   probes that would otherwise mis-route the next run. What is left is the
#   only on-disk copy of a record that took hours to produce, so this script
#   leaves it alone. The spec/issue derivation the deletion needed
#   (Closes #N -> Tracked in: #N -> spec-base) and the `**Tracked in run:**`
#   secondary probe both went with it.
#
# Failure modes (all exit 0 -- workflow step 4 proceeds regardless):
# - `PR_NUM` empty (branch merged outside `gh`, or PR is closed-not-merged):
#   prints a one-line note to stdout and exits 0 (nothing reliable to derive
#   paths from).
# - rm -f is silent on missing files (intentional -- files may not exist;
#   idempotent re-runs are safe).
# - rmdir failing on non-empty directory is expected and ignored.
#
# Deferred-task plan files (ai-docs/plans/deferred/) are intentionally NOT
# touched -- deferral is its own workflow and a deferred task has no merged
# PR to drive cleanup.

set -uo pipefail

PREV_BRANCH="${1:-}"
if [ -z "${PREV_BRANCH}" ]; then
  printf 'cleanup-progress.sh: usage: cleanup-progress.sh <previous-branch>\n' >&2
  exit 2
fi

PR_NUM=$(gh pr list --state merged --head "${PREV_BRANCH}" --json number --jq '.[0].number // empty')

if [ -z "${PR_NUM}" ]; then
  printf 'pr-merged: no merged PR found for %s; skipping progress-file cleanup.\n' "${PREV_BRANCH}"
  exit 0
fi

# Fallback surfaces only. The /task, /interview and /main-ci-failed state
# files are retired by their own flow before its PR and are deliberately
# left alone -- see the header note.
rm -f "ai-docs/pr-comments/pr-${PR_NUM}.progress.md"
rm -f "ai-docs/ci-fixes/pr-${PR_NUM}.progress.md"

# Opportunistic cleanup -- fails non-fatally if directories have other files
# or do not exist. Exit code intentionally ignored.
rmdir ai-docs/pr-comments ai-docs/ci-fixes 2>/dev/null || true

exit 0
