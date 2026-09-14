#!/usr/bin/env bash
# Regression suite for the PreToolUse instruction-file size-measurement guard.
#
# The guard blocks wc, du and stat over the instruction-file corpus, and cat
# or find piped into them, outside the two audit recipes that own that
# measurement. Every other flow is forbidden to measure the corpus whatever
# the purpose, and a flow that never opens the audit checklist cannot know it.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq and
# executed as the program it is. There is no copied regex here, so there is
# nothing to drift.
#
# Verdict convention: the body exits 2 to block a tool call. Any other exit
# status means the call proceeds.
#
# Known false positive, asserted deliberately: a search whose pattern text
# puts wc, du or stat right after one of ; & | ( { beside a covered path is
# BLOCKED, because the guard matches command text; the pattern can be put in
# a file and read with grep -f. A plain quoted 'stat -c' is not matched.
#
# Known misses, asserted so that closing one is a deliberate change: a line
# count through awk, through grep -c with an empty pattern, and through ls -l;
# wc behind time, if, command, backticks or bash -c; and find -exec wc are
# ALLOWED. The guard carries the prohibition to the common spellings; it is
# not a sandbox.
#
# The Sub-check 9 recipe passes without an exemption: its brace group ends
# the find with ;, which the xargs branch does not cross. Its one-line and
# multi-line forms are both fixtures, so a regex change that starts refusing
# the recipe fails here, and text appended after it is refused like any other.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-size-measure-guard.sh    run the whole suite; it takes no arguments
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
  | select(contains("BLOCKED: instruction-file size measured outside /ai-audit"))' "$settings")
[ -n "$body" ] || { echo "FAIL: size-measure guard body not found in $settings"; exit 1; }

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
BLOCK wc -c .claude/skills/task/SKILL.md .claude/skills/task/reference.md
BLOCK wc -c .claude/agents/self-improve.md
BLOCK wc -c AGENTS.md
BLOCK sed -n '24,76p' .claude/skills/ai-audit/checklist-m.md; wc -c AGENTS.md CLAUDE.md
BLOCK wc -l -c /work/lab-game/AGENTS.md /work/lab-game/.claude/agents/self-review.md
BLOCK LC_ALL=C.UTF-8 wc -m -c /work/lab-game/AGENTS.md /work/lab-game/.claude/agents/self-review.md
# --- must block: other spellings of the same measurement ---
BLOCK cat AGENTS.md | wc -c
BLOCK find .claude/agents -name '*.md' | xargs wc -c
BLOCK stat -c %s ai-docs/context.md
BLOCK du -b .claude/rules
BLOCK wc -l .claude/skills/*/SKILL.md .claude/skills/*/reference.md
BLOCK for f in x; do wc -c AGENTS.md; done
# --- must block: the accepted false positive (see the header) ---
BLOCK grep -rn -E '(^|[^A-Za-z])(wc -[clmL]|du -[bsh]|stat -c)' AGENTS.md
# --- must block: a measurement appended after the Sub-check 9 recipe ---
BLOCK { find AGENTS.md CLAUDE.md .claude/rules .claude/agents .claude/skills -name '*.md'; } | xargs wc -c; wc -c AGENTS.md
# --- must allow: the two audit recipes, verbatim ---
ALLOW wc -l .claude/skills/*/SKILL.md
ALLOW { find AGENTS.md CLAUDE.md .claude/rules .claude/agents .claude/skills -name '*.md'; printf '%s\n' ai-docs/code-style.md ai-docs/doc-convention.md ai-docs/context.md ai-docs/agent-writing-style.md ai-docs/corrections-log.md; } | xargs wc -c
# --- must allow: innocent commands containing the substrings ---
ALLOW grep -rn 'wc -c' AGENTS.md
ALLOW grep -rn 'stat -c' AGENTS.md
ALLOW wc -l ai-docs/learnings.md
ALLOW wc -l .claude/skills/task/scripts/test-append-task-run.sh
ALLOW grep -rn 'Propagation' .claude/skills | wc -l
ALLOW sed -n '1,40p' AGENTS.md
ALLOW jq -r '.hooks[][].hooks[].command' .claude/settings.json | wc -l
ALLOW git diff --stat -- AGENTS.md
ALLOW grep -c '^### ' ai-docs/learnings.md
ALLOW ls .claude/agents | wc -l
ALLOW LC_ALL=C sort -u tmp/names.txt
# --- must allow: the accepted misses (see the header) ---
ALLOW awk 'END { print NR }' AGENTS.md
ALLOW grep -c '' AGENTS.md
ALLOW ls -l AGENTS.md
ALLOW time wc -c AGENTS.md
ALLOW if wc -c AGENTS.md; then :; fi
ALLOW command wc -c AGENTS.md
ALLOW echo `wc -c AGENTS.md`
ALLOW bash -c "wc -c AGENTS.md"
ALLOW find . -name AGENTS.md -exec wc -c {} +
FIXTURES

# The Sub-check 9 recipe in its multi-line form, as the audit checklist prints it.
recipe=$(cat <<'RECIPE'
{ find AGENTS.md CLAUDE.md .claude/rules .claude/agents .claude/skills -name '*.md';
  printf '%s\n' ai-docs/code-style.md ai-docs/doc-convention.md \
                 ai-docs/context.md ai-docs/agent-writing-style.md \
                 ai-docs/corrections-log.md; } | xargs wc -c
RECIPE
)
check ALLOW "$recipe"

# K1's exemption must survive byte-for-byte.
grep -qF -- "'wc -l .claude/skills/*/SKILL.md') exit 0" <<<"$body" || {
  echo "FAIL: K1 exemption no longer present verbatim"
  failures=$((failures + 1))
}

if [ "$failures" -eq 0 ]; then
  echo "size-measure guard: all fixtures behave as specified"
  exit 0
fi
printf 'size-measure guard: %d check(s) failed\n' "$failures"
exit 1
