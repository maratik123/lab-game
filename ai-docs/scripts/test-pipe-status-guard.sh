#!/usr/bin/env bash
# Regression suite for the PreToolUse pipe-status hook guard.
#
# The guard blocks a command that reads $? in the statement right after a
# pipeline, because bash reports the LAST pipeline stage's exit status: head,
# cut, sed and tee succeed on any input, so a no-match or an error from the
# stage that was meant to be measured records as a clean result.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq and
# executed as the program it is. There is no copied regex here, so there is
# nothing to drift.
#
# Verdict convention: the body exits 2 to block a tool call. Any other exit
# status means the call proceeds.
#
# Known false positive, asserted deliberately: a spaced pipe inside a quoted
# pattern, followed by a read of $?, is BLOCKED. The guard matches command
# text, not shell grammar, and a loud refusal costs one re-spelling.
# Known miss, asserted so that closing it is a deliberate change: a status
# read joined to the pipeline with && instead of ; is ALLOWED.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-pipe-status-guard.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entry by a fixed substring of its message, never by index: an
# index silently re-points the moment another PreToolUse entry is added.
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("BLOCKED: exit status read after a pipeline"))' "$settings")
[ -n "$body" ] || { echo "FAIL: pipe-status guard body not found in $settings"; exit 1; }

failures=0

# Feed one command string to the real body and report BLOCK or ALLOW.
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

# verdict, one space, command. Both directions are required: the block half
# alone is satisfied by a regex that matches everything.
while read -r want cmd; do
  case "${want:-}" in ''|'#'*) continue ;; esac
  check "$want" "$cmd"
done <<'FIXTURES'
# --- must block: the recorded shapes ---
BLOCK grep -rniE 'falsified' README.md docs/*.md | head -10; echo "exit=$?"
BLOCK grep -n 'resultCh' internal/tg/client.go | cut -c1-150; echo $?
BLOCK git diff origin/main...HEAD | grep -E '^-func [A-Z]' | head; echo "exit=$?"
BLOCK grep -niE 'struck' ai-docs/plans/x.spec.md | sed 's/^/hit: /'; echo "rc=$?"
# --- must block: other stages and other reads of $? ---
BLOCK go test ./... 2>&1 | tee tmp/t.log; echo "exit=$?"
BLOCK rg -n foo . < /dev/null | wc -l; rc=$?; echo "$rc"
BLOCK for f in a b; do grep -c x "$f" | cat; echo "$f=$?"; done
# --- must block: the accepted false positive (see the header) ---
BLOCK grep -c 'a | b' AGENTS.md; echo $?
# --- must allow ---
ALLOW grep -rn foo AGENTS.md > tmp/probe.log 2>&1; echo "exit=$?"; head -20 tmp/probe.log
ALLOW set -o pipefail; grep foo AGENTS.md | head; echo $?
ALLOW grep foo AGENTS.md | head; echo "${PIPESTATUS[0]}"
ALLOW grep -nw 'resultCh' internal/tg/client.go | cut -c1-150; echo "grep=${PIPESTATUS[0]} cut=${PIPESTATUS[1]}"
ALLOW grep -E 'foo|bar' AGENTS.md; echo "exit=$?"
ALLOW make test > tmp/gate.log 2>&1 || echo "failed: $?"
ALLOW git log --oneline | head -5; git status --porcelain; echo "status=$?"
ALLOW ls .claude/agents | wc -l
ALLOW grep -rn 'echo $?' AGENTS.md | head -3
ALLOW go build ./... > tmp/b.log 2>&1; echo "build=$?"
# --- must allow: the accepted miss (see the header) ---
ALLOW grep foo AGENTS.md | head && echo "ok $?"
FIXTURES

# A pipeline on one line and the status read on the next is the same defect:
# the body joins lines before matching, and this case keeps it doing so.
multiline=$(cat <<'ML'
grep -n foo AGENTS.md | head
echo "exit=$?"
ML
)
check BLOCK "$multiline"

# The escape hatch must survive byte-for-byte.
grep -qF -- "grep -qE 'pipefail|PIPESTATUS' && exit 0" <<<"$body" || {
  echo "FAIL: the pipefail/PIPESTATUS escape hatch is no longer present verbatim"
  failures=$((failures + 1))
}

if [ "$failures" -eq 0 ]; then
  echo "pipe-status guard: all fixtures behave as specified"
  exit 0
fi
printf 'pipe-status guard: %d check(s) failed\n' "$failures"
exit 1
