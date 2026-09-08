#!/usr/bin/env bash
# Regression suite for the PreToolUse --no-verify guard in .claude/settings.json.
#
# `.githooks/pre-commit` runs the coverage ratchet, and `git commit --no-verify`
# switches it off. That escape belongs to the owner, not to an agent: a gate an
# agent can turn off is not a gate. The project has one recorded instance of an
# agent dodging a textual gate by rewording the match (ai-docs/learnings.md
# 2026-09-04), which is why the ratchet's bypass is closed mechanically on the
# same day the ratchet lands rather than after the first dodge.
#
# The short flag is covered too: `git commit -n` IS `--no-verify`, and a
# guard that matched only the long spelling would be the same gate with a
# one-character hole.
#
# TWO FALSE POSITIVES, BOTH DELIBERATE, both fixtures below so a later "fix"
# that opens the hole fails here instead of in production:
#   * `git push -n` is --dry-run, not --no-verify. It is blocked, because this
#     hook matches COMMAND TEXT and cannot know which subcommand a short flag
#     belongs to. A dry run executes nothing, so the cost is a loud refusal on
#     a no-op. Same accepted trade as the piped-gate guard's `make -n`.
#   * a commit MESSAGE containing a lone `-n` token is blocked. Cost: reword
#     the message. The alternative is a silently bypassed ratchet, and this
#     project already has one recorded agent dodging a textual gate.
#
# Anti-drift: runs the LIVE hook body, extracted with jq.
# Verdict convention: the body exits 2 to block a tool call.
#
# Usage: bash ai-docs/scripts/test-no-verify-guard.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("BLOCKED: --no-verify on git commit/push"))' "$settings")
[ -n "$body" ] || { echo "FAIL: --no-verify guard body not found in $settings"; exit 1; }

failures=0

verdict() {
  local payload rc
  payload=$(jq -n --arg c "$1" '{tool_input: {command: $c}}')
  printf '%s' "$payload" | bash -c "$body" >/dev/null 2>&1 && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

check() {
  local got
  got=$(verdict "$2")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, for: %s\n' "$1" "$got" "$2"
    failures=$((failures + 1))
  fi
}

while IFS=$'\t' read -r want cmd; do
  case "$want" in ''|'#'*) continue ;; esac
  check "$want" "$cmd"
done <<'FIXTURES'
# --- must block: every spelling of the bypass ---
BLOCK	git commit --no-verify -m "wip"
BLOCK	git commit -m "wip" --no-verify
BLOCK	git commit -n -m "wip"
BLOCK	git commit -nm "wip"
BLOCK	git push --no-verify
BLOCK	cd /home/syt/lab-game && git commit --no-verify -m "x"
# --- must block: the two accepted false positives (see the header) ---
BLOCK	git push -n
BLOCK	git commit -m "docs: explain the -n flag"
# --- must allow: ordinary commits and pushes ---
ALLOW	git commit -m "feat(scheduler): add Reconcile"
ALLOW	git commit -q -m "chore(plans): retire the run's state files"
ALLOW	git commit --amend --no-edit
ALLOW	git push
ALLOW	git push -u origin harness/forge-11
ALLOW	git add -- ai-docs/coverage-ratchet.txt
ALLOW	git log --oneline -n 5
ALLOW	git diff --name-only
FIXTURES

if [ "$failures" -eq 0 ]; then
  echo "--no-verify guard: all fixtures behave as specified"
  exit 0
fi
printf -- '--no-verify guard: %d check(s) failed\n' "$failures"
exit 1
