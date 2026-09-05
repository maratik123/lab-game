#!/usr/bin/env bash
# Regression suite for the PreToolUse gate-log-path guard in .claude/settings.json.
#
# The guard refuses a gate whose output is redirected to a BARE FILENAME, which
# lands in the repository root. `*.gate.log` is gitignored, so such a file is
# invisible to `git status` and to every tree-clean probe the flows run: 60 of
# them had accumulated in the root by 2026-09-05, under names each run invented
# (a.gate.log, m1.gate.log, f1verify.gate.log, s9actionlint.gate.log). AGENTS.md
# § Build & Test had named exactly one filename the whole time — the text held
# for none of them, which is why the path is now a gate.
#
# The rule is deliberately a SHAPE, not a path list: any target containing a
# slash passes (tmp/x.log, /dev/null, a scratchpad path, ../x). Only a bare filename
# is refused. That keeps the guard from having an opinion about where scratch
# lives outside the work tree.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq. No regex is
# copied here.
#
# Verdict convention: the body exits 2 to block a tool call.
#
# Usage: bash .claude/skills/ai-audit/scripts/test-gate-log-path-guard.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("BLOCKED: gate output redirected into the repository root"))' "$settings")
[ -n "$body" ] || { echo "FAIL: gate-log-path guard body not found in $settings"; exit 1; }

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
# --- must block: the shapes that produced the 60 files ---
BLOCK	go test ./... > gate.log 2>&1 && echo GATE-GREEN || echo GATE-RED
BLOCK	go build ./... > build.gate.log 2>&1
BLOCK	make verify > f1verify.gate.log 2>&1
BLOCK	golangci-lint run > lint.gate.log 2>&1
BLOCK	go vet ./... 2> vet.log
BLOCK	actionlint .github/workflows/*.yml > actionlint.gate.log 2>&1
BLOCK	shellcheck -s bash x.sh > s9lint.gate.log 2>&1
BLOCK	cd /home/syt/lab-game && go test -count=1 ./... >> m2.gate.log 2>&1
# --- must allow: any target with a slash, and any non-gate command ---
ALLOW	mkdir -p tmp && go test ./... > tmp/gate.log 2>&1 && echo GATE-GREEN || echo GATE-RED
ALLOW	grep -E "^(FAIL|ok|---)" tmp/gate.log
ALLOW	go test ./... > /dev/null 2>&1
ALLOW	go test ./... > "$SCRATCH/gate.log" 2>&1
ALLOW	go build ./... > ../out.log 2>&1
ALLOW	go test ./... 2>&1 | tee tmp/gate.log
ALLOW	git log --oneline > out.txt
ALLOW	jq -r '.x' settings.json > out.json
ALLOW	go test ./...
ALLOW	make verify
FIXTURES

# The escape shape is what makes the guard cheap; assert it survives verbatim.
grep -qF -- '[A-Za-z0-9._-]+([[:space:]]|$)' <<<"$body" || {
  echo "FAIL: the bare-filename shape is no longer matched verbatim"
  failures=$((failures + 1))
}

if [ "$failures" -eq 0 ]; then
  echo "gate-log-path guard: all fixtures behave as specified"
  exit 0
fi
printf 'gate-log-path guard: %d check(s) failed\n' "$failures"
exit 1
