#!/usr/bin/env bash
# Regression suite for check-review-register.sh.
#
# The guard refuses a progress file whose `## Review register` still reads
# `open` for a finding the round's own table marks `✅ Fixed`. The fixtures are
# reduced from the real history that earned the gate: the transport run raised
# this same disagreement in five consecutive rounds.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-review-register.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard=ai-docs/scripts/check-review-register.sh
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

# --- no argument is a usage error, never a pass -----------------------------
"$guard" >/dev/null 2>&1
rc=$?
if [ "$rc" -ne 2 ]; then
  printf 'FAIL [no-argument]: expected exit 2, got %s\n' "$rc"
  failures=$((failures + 1))
fi

# expect_msg <name> <substring>: the guard's message on that fixture carries it.
expect_msg() {
  local msg
  msg=$("$guard" "$sandbox/$1.progress.md" 2>&1 >/dev/null)
  printf '%s' "$msg" | grep -qF -- "$2" && return 0
  printf 'FAIL [%s]: the message does not carry %s\n%s\n' "$1" "$2" "$msg"
  failures=$((failures + 1))
}

expect_msg stale-open 'R1-1 reads "open"'

# --- a re-opened register row still reads open, with its decoration ---------
name=reopened-decorated
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | major | open 🔁@2 | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ✅ Fixed |'
expect FAIL reopened-decorated
expect_msg reopened-decorated 'open 🔁@2'

# --- a re-opened finding leads its row with its original id ------------------
# Round 2's row 1 is round 1's finding 5, kept under its id as the reviewer's
# charter requires; joining by position would look for round 2's own row id.
name=named-id-fixed
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-5 | round 1 | major | fixed@abc1234 | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 5 | a.go:1 | major | one | ✅ Fixed |' '' \
  '## Self-Review (Round 2)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | R1-5 — re-opened: the fix missed a second caller | ✅ Fixed |'
expect PASS named-id-fixed

name=named-id-open
# shellcheck disable=SC2016  # the backticks are a markdown code span in the fixture row
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-5 | round 1 | major | open 🔁@2 | mutate x |' '' \
  '## Self-Review (Round 2)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | `R1-5` re-opened | ✅ Fixed |'
expect FAIL named-id-open
expect_msg named-id-open 'R1-5 reads "open 🔁@2"'

name=named-id-absent
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R2-1 | round 2 | major | fixed@abc1234 | mutate x |' '' \
  '## Self-Review (Round 2)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | R1-5 — re-opened | ✅ Fixed |'
expect FAIL named-id-absent
expect_msg named-id-absent 'names register id R1-5'

# --- a descriptive id: refused, and the message says why ---------------------
name=descriptive-id
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| SR1-CLUSTER-WHY | round 1 | major | fixed@abc1234 | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | major | one | ✅ Fixed |'
expect FAIL descriptive-id
expect_msg descriptive-id 'cannot parse: SR1-CLUSTER-WHY'

# --- a Finding cell that starts with digits names no id ----------------------
name=digits-not-an-id
emit \
  '# Progress' '' \
  '## Review register' '' \
  '| id | raised | severity | status | verifying command |' \
  '|----|--------|----------|--------|-------------------|' \
  '| R1-1 | round 1 | minor | fixed@abc1234 | mutate x |' '' \
  '## Self-Review (Round 1)' '' \
  '| # | File:line | Severity | Finding | Status |' \
  '|---|-----------|----------|---------|--------|' \
  '| 1 | a.go:1 | minor | 2-3 retries where one was meant | ✅ Fixed |'
expect PASS digits-not-an-id

# --- the hook: which files it hands the guard, and when ----------------------
# The live body runs inside a scratch repository, so the set of files it checks
# is measured rather than read off the body. The shapes are the ones that went
# through unchecked: staging and committing in one call, a retired progress
# file, and a bugfix trace that is never staged at all.
settings="$repo_root/.claude/settings.json"
hook=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("check-review-register.sh"))' "$settings")
if [ -z "$hook" ]; then
  echo "FAIL: review-register hook body not found in $settings"
  failures=$((failures + 1))
else
  repo="$sandbox/repo"
  git init -q -b feat/x "$repo"
  mkdir -p "$repo/ai-docs/scripts" "$repo/ai-docs/plans/ignored" "$repo/ai-docs/bugfix"
  cp "$guard" "$repo/ai-docs/scripts/"
  cp "$sandbox/stamped.progress.md" "$repo/ai-docs/plans/r.progress.md"
  git -C "$repo" add ai-docs
  git -C "$repo" -c user.name=t -c user.email=t@example.invalid commit -q -m base
  stale_file="$sandbox/stale-open.progress.md"

  # hook_expect <want> <label> <command>
  hook_expect() {
    local rc got
    (cd "$repo" && jq -n --arg c "$3" '{tool_input: {command: $c}}' | bash -c "$hook" >/dev/null 2>&1) && rc=0 || rc=$?
    if [ "$rc" -eq 2 ]; then got=BLOCK; else got=ALLOW; fi
    [ "$got" = "$1" ] && return 0
    printf 'FAIL [hook: %s]: expected %s, got %s\n' "$2" "$1" "$got"
    failures=$((failures + 1))
  }
  reset_repo() {
    git -C "$repo" reset -q
    git -C "$repo" checkout -q -- ai-docs
    rm -f "$repo"/ai-docs/plans/ignored/* "$repo"/ai-docs/bugfix/*
  }
  of_branch() { printf '**Branch:** %s\n\n' "$1"; cat "$stale_file"; }

  cp "$stale_file" "$repo/ai-docs/plans/r.progress.md"
  git -C "$repo" add ai-docs/plans/r.progress.md
  hook_expect BLOCK 'staged in an earlier call' 'git commit -m x'
  reset_repo

  cp "$stale_file" "$repo/ai-docs/plans/r.progress.md"
  hook_expect BLOCK 'staged and committed in one call' 'git add ai-docs/plans/r.progress.md && git commit -m x'
  reset_repo

  hook_expect ALLOW 'an agreeing progress file' 'git add ai-docs/plans/r.progress.md && git commit -m x'

  of_branch feat/x > "$repo/ai-docs/plans/ignored/old.progress.md"
  hook_expect BLOCK 'a retired progress file of this branch' 'git commit -m x'
  reset_repo

  of_branch feat/x-y > "$repo/ai-docs/plans/ignored/old.progress.md"
  hook_expect ALLOW 'a retired progress file of a branch whose name this one prefixes' 'git commit -m x'
  reset_repo

  cp "$stale_file" "$repo/ai-docs/bugfix/trace-2026-09-15-x.md"
  hook_expect BLOCK 'a bugfix trace, never staged' 'git commit -m x'
  hook_expect ALLOW 'a command that is not a commit' 'git status --porcelain'
  reset_repo
fi

if [ "$failures" -eq 0 ]; then
  echo "review-register guard: all fixtures behave as specified"
  exit 0
fi
printf 'review-register guard: %d check(s) failed\n' "$failures"
exit 1
