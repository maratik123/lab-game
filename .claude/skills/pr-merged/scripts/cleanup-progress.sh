#!/usr/bin/env bash
#
# Delete the local progress files belonging to the merged branch passed as the
# first argument.
#
# Run by the /pr-merged flow, after `git checkout main && git pull` and before
# `git branch -d <previous-branch>`.
#
# Derivation (pull-request linkage):
#   1. `gh pr list --state merged --head <branch>` -> merged number
#   2. Delete the /pr-commented fallback progress file for that number (the
#      path used for pull requests not produced by /task)
#   3. Delete the /pr-ci-failed fallback progress file for that number
#   4. `rmdir` both fallback directories -- opportunistic cleanup; non-fatal
#      if they still have unrelated files or do not exist.
#
# WHAT THIS SCRIPT NO LONGER DELETES, and why:
#   The /task progress file, the /interview state sibling and the
#   /main-ci-failed per-run progress file are each COMMITTED while their
#   flow runs and RETIRED by that flow into an ignored directory as the
#   last commit before its pull request. Retiring already achieves everything
#   the deletion here was for: the files are out of the diff, out of the
#   repo, and out of the directory listings that would otherwise mis-route the
#   next run. What is left is the only on-disk copy of a record that took
#   hours to produce, so this script leaves it alone. The spec-and-issue
#   derivation the deletion needed, and the secondary run-tracking probe,
#   both went with it.
#
# Failure modes (all exit 0 -- workflow step 4 proceeds regardless):
# - `PR_NUM` empty (branch merged outside `gh`, or PR is closed-not-merged):
#   prints a one-line note to stdout and exits 0 (nothing reliable to derive
#   paths from).
# - rm -f is silent on missing files (intentional -- files may not exist;
#   idempotent re-runs are safe).
# - rmdir failing on non-empty directory is expected and ignored.
#
# Deferred-task plan files are intentionally NOT touched -- deferral is its own
# workflow and a deferred task has no merged pull request to drive cleanup.

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
