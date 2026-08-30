#!/usr/bin/env bash
# Regression suite for the PreToolUse piped-gate guard in .claude/settings.json.
#
# The guard blocks a gate piped into tail/head without pipefail, because bash
# reports the LAST pipeline stage's exit status and a RED gate then records as
# green. AGENTS.md § Build & Test advertises that hook as the enforcement for
# the whole class, so a silent narrowing of its alternation is a live
# instruction-file claim going false.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq and
# executed as the program it is. There is no copied regex here, so there is
# nothing to drift. An edit that un-blocks a must-block case, or that starts
# blocking a must-allow case, fails here instead of being discovered by a
# blocked agent weeks later.
#
# Verdict convention: the body exits 2 to block a tool call. Any other exit
# status means the call proceeds.
#
# Known false positive, asserted deliberately: `make -n verify` piped into
# head is BLOCKED. `make` is matched as a class, not by target enumeration,
# because any flag between `make` and the target defeats an anchored
# enumeration and every target in this project's Makefile is a gate (KD-16). A
# dry run executes nothing, so the cost is a loud refusal rather than a green
# record of a red gate. It is a fixture below so that a later "fix" which
# quietly un-blocks it fails this suite.
#
# Usage: bash .claude/skills/ai-audit/scripts/test-piped-gate-guard.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entry by a fixed substring of its message, never by index: an
# index silently re-points the moment another PreToolUse entry is added.
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("BLOCKED: Go gate piped through tail/head"))' "$settings")
[ -n "$body" ] || { echo "FAIL: piped-gate guard body not found in $settings"; exit 1; }

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

# verdict<TAB>command. BLOCK rows are AC21 plus the AC23 known false positive;
# ALLOW rows are AC22. Both directions are required: AC21 alone is satisfied by
# a regex that matches everything.
while IFS=$'\t' read -r want cmd; do
  case "${want:-}" in ''|'#'*) continue ;; esac
  check "$want" "$cmd"
done <<'FIXTURES'
# --- must block: the gates this change introduces ---
BLOCK	golangci-lint fmt -d | tail -5
BLOCK	make verify | tail -5
BLOCK	make lint | head -30
BLOCK	make test | tail -5
BLOCK	make | tail -5
BLOCK	make -s verify | tail -5
BLOCK	make -B lint | tail -5
BLOCK	make -C . verify | tail -5
BLOCK	make -j4 test | tail -5
BLOCK	make -f Makefile verify | tail -5
# --- must block: regression cover for what the guard already blocked ---
BLOCK	go test ./... | tail -5
BLOCK	gofmt -l . | tail -5
BLOCK	golangci-lint run | tail -5
BLOCK	go build ./... | head -20
BLOCK	go vet ./... | tail -3
# --- must block: the accepted false positive (see the header) ---
BLOCK	make -n verify | head
# --- must allow ---
ALLOW	set -o pipefail; make verify | tail -5
ALLOW	make --help | head
ALLOW	go list ./... | head -20
ALLOW	golangci-lint linters | head -20
ALLOW	git log --oneline | head -20
ALLOW	go test ./... > gate.log 2>&1 && echo GATE-RED
ALLOW	grep -E "^(FAIL|ok)" gate.log | head -5
ALLOW	cmake --build . | tail -5
ALLOW	echo makezero | tail -1
ALLOW	make verify
FIXTURES

# Both carve-outs must survive byte-for-byte (AC23).
for carveout in '(^|[[:space:]])(--help|-h)([[:space:]]|$)' \
                '(^|[;&|[:space:]])set[[:space:]]+-o[[:space:]]+pipefail'; do
  grep -qF -- "$carveout" <<<"$body" && continue
  printf 'FAIL: carve-out no longer present verbatim: %s\n' "$carveout"
  failures=$((failures + 1))
done

# ...and no third carve-out has been added. A `-n` carve-out in particular is
# rejected on the record: the carve-outs are whole-command greps, so it would
# exempt `tail -n 5`, the canonical spelling of `tail -5`.
negations=$(grep -o -- '! printf' <<<"$body" | wc -l)
if [ "$negations" -ne 2 ]; then
  printf 'FAIL: expected exactly 2 carve-outs, found %s\n' "$negations"
  failures=$((failures + 1))
fi

if [ "$failures" -eq 0 ]; then
  echo "piped-gate guard: all fixtures behave as specified"
  exit 0
fi
printf 'piped-gate guard: %d check(s) failed\n' "$failures"
exit 1
