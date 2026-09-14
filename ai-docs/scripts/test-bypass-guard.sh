#!/usr/bin/env bash
# Regression suite for the PreToolUse guard that refuses a command bypassing or
# changing the server-side protection of the repository.
#
# A session acts with the repository owner's credentials, so the server cannot
# tell an agent's admin merge or ruleset write from the owner's own. The guard
# refuses three shapes: a pull-request merge with the admin flag, a REST write
# to a ruleset or a branch protection, and a GraphQL mutation of either. It
# lets every read through, and an ordinary merge, because a guard that blocks
# reading the rules stops the one check a flow is told to make before trusting
# them.
#
# Anti-drift: the live hook body is extracted with jq and run as the program it
# is.
#
# Verdict convention: the body exits 2 to block. Any other status proceeds.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-bypass-guard.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entry by a fixed substring of its message, never by index.
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("Server-side protection of main belongs to the owner"))' "$settings")
[ -n "$body" ] || { echo "FAIL: bypass guard body not found in $settings"; exit 1; }

failures=0

verdict_raw() {
  local rc
  printf '%s' "$1" | bash -c "$body" >/dev/null 2>&1 && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

# check <want> <command>
check() {
  local got
  got=$(verdict_raw "$(jq -n --arg c "$2" '{tool_input: {command: $c}}')")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, for: %s\n' "$1" "$got" "$2"
    failures=$((failures + 1))
  fi
}

r=repos/maratik123/lab-game

# --- must block: an admin merge ---
check BLOCK 'gh pr merge 57 --admin --merge'
check BLOCK 'gh pr merge --admin 57'
check BLOCK 'gh pr merge --merge --admin'
check BLOCK 'cd /x && gh pr merge 57 --merge --admin'
check BLOCK "$(printf 'gh pr merge 57 --merge \\\n  --admin')"

# --- must block: a REST write to a ruleset or a branch protection ---
check BLOCK "gh api -X DELETE $r/rulesets/23358425"
check BLOCK "gh api -XDELETE $r/rulesets/23358425"
check BLOCK "gh api --method PUT $r/rulesets/23358425 --input ruleset.json"
check BLOCK "gh api --method=DELETE $r/branches/main/protection"
check BLOCK "gh api -X patch $r/rulesets/23358425 -f enforcement=disabled"
check BLOCK "gh api $r/rulesets --input ruleset.json"
check BLOCK "gh api $r/rulesets -f name=main -f enforcement=active"
check BLOCK "gh api $r/branches/main/protection -X PUT --input protection.json"

# --- must block: a GraphQL mutation of either ---
check BLOCK "gh api graphql -f query='mutation { deleteRepositoryRuleset(input: {repositoryRulesetId: \"x\"}) { clientMutationId } }'"
check BLOCK "gh api graphql -f query='mutation { updateBranchProtectionRule(input: {branchProtectionRuleId: \"x\"}) { clientMutationId } }'"

# --- must allow: reads, and an ordinary merge ---
check ALLOW 'gh pr merge --merge 57'
check ALLOW 'gh pr view 57 --json mergeable,mergeStateStatus'
check ALLOW "gh api $r/rules/branches/main"
check ALLOW "gh api $r/rulesets"
check ALLOW "gh api $r/rulesets/23358425 --jq .enforcement"
check ALLOW "gh api -X GET $r/rulesets -f includes_parents=true"
check ALLOW "gh api $r/branches/main/protection"
check ALLOW "gh api graphql -f query='query { repository(owner: \"o\", name: \"r\") { rulesets(first: 5) { nodes { name } } } }'"

# --- must allow: writes elsewhere, and mentions that run nothing ---
check ALLOW "gh api $r/issues -f title=x -f body=y"
check ALLOW 'git commit -m "never merge with --admin"'
check ALLOW 'gh pr comment 57 --body "merged without --admin"'

# --- fail open on the hook's own inputs ---
got=$(verdict_raw '{}')
[ "$got" = ALLOW ] || { echo "FAIL: a payload with no command must ALLOW, got $got"; failures=$((failures + 1)); }
got=$(verdict_raw 'not json')
[ "$got" = ALLOW ] || { echo "FAIL: an unparseable payload must ALLOW, got $got"; failures=$((failures + 1)); }

if [ "$failures" -eq 0 ]; then
  echo "bypass guard: all fixtures behave as specified"
  exit 0
fi
printf 'bypass guard: %d check(s) failed\n' "$failures"
exit 1
