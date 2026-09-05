#!/usr/bin/env bash
# Regression suite for check-review-register.sh.
#
# The guard refuses a progress file whose `## Review register` still reads
# `open` for a finding the round's own table marks `✅ Fixed`. The fixtures are
# reduced from the real history that earned the gate: the transport run
# (PR #53) raised this same disagreement in five consecutive rounds.
#
# Usage: bash .claude/skills/ai-audit/scripts/test-review-register.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard=.claude/skills/ai-audit/scripts/check-review-register.sh
[ -x "$guard" ] || { echo "FAIL: $guard not executable"; exit 1; }

failures=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT

emit() { printf '%s\n' "$@" > "$sandbox/$name.progress.md"; }

verdict() {
  "$guard" "$sandbox/$1.progress.md" >/dev/null 2>&1 && echo PASS || echo FAIL
}

expect() {
  local want=$1 name=$2 got
  got=$(verdict "$name")
  [ "$got" = "$want" ] && return 0
  printf 'FAIL [%s]: expected %s, got %s\n' "$name" "$want" "$got"
  failures=$((failures + 1))
}

# --- the disagreement that recurred five times ------------------------------
name=stale-open
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | major | open | mutate x, go test — must go RED |' \
  '| R1-2 | round 1 | minor | fixed@abc1234 | mutate y, go test — must go RED |' '' \
  '## Self-Review (Round 1)' '' \
  '**Verdict:** REJECT' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | internal/tg/caller.go:106 | major | A multipart request is retried | ✅ Fixed |' \
  '| 2 | internal/tg/limit.go:210 | minor | False doc comment | ✅ Fixed |'
expect FAIL stale-open

# --- both stamped: the state each round was supposed to leave behind --------
name=stamped
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | major | fixed@abc1234 | mutate x, go test — must go RED |' \
  '| R1-2 | round 1 | minor | fixed@abc1234 | mutate y, go test — must go RED |' '' \
  '## Self-Review (Round 1)' '' \
  '**Verdict:** REJECT' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | internal/tg/caller.go:106 | major | A multipart request is retried | ✅ Fixed |' \
  '| 2 | internal/tg/limit.go:210 | minor | False doc comment | ✅ Fixed |'
expect PASS stamped

# --- a Fixed row with no register row at all --------------------------------
name=missing-row
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | major | fixed@abc1234 | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ✅ Fixed |' \
  '| 2 | b.go:2 | minor | two | ✅ Fixed |'
expect FAIL missing-row

# --- an Open row is not this guard's business -------------------------------
# A finding still open in the round table and open in the register agrees.
name=both-open
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | major | open | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ⬜ Open |'
expect PASS both-open

# --- a re-opened row: the additive marker must not read as Fixed ------------
name=reopened
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R2-1 | round 2 | major | open | mutate x |' '' \
  '## Self-Review (Round 2)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ⬜ Open 🔁 Re-opened |'
expect PASS reopened

# --- an SR-prefixed id joins the same way -----------------------------------
name=sr-prefix
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| SR1-1 | self-review round 1 | major | open | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ✅ Fixed |'
expect FAIL sr-prefix

# --- no register at all: not every flow keeps one; fail open ----------------
name=no-register
emit \
  '# Progress' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ✅ Fixed |'
expect PASS no-register

if [ "$failures" -eq 0 ]; then
  echo "review-register guard: all fixtures behave as specified"
  exit 0
fi
printf 'review-register guard: %d check(s) failed\n' "$failures"
exit 1
