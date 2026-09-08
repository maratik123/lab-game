#!/usr/bin/env bash
# Regression suite for the PreToolUse instruction-edit hook guard.
#
# The guard blocks a write into an instruction file -- the project rule file,
# its per-tool alias, the agent-and-skill directory, and the code-style and
# doc-convention pages, the list the learning-log boundary rule states verbatim
# -- while an interview is live: an interview state file exists and the
# in-flight marker does not. Steps 8-12 of /task carry the marker and are
# exempt; a branch with no state file is exempt; /improve therefore runs
# untouched on its own branch.
#
# Anti-drift: this suite runs the LIVE hook bodies, extracted with jq and
# executed as the programs they are, inside a scratch directory whose plans/
# folder is set to each of the three states the guard distinguishes. There is
# no copied regex here, so there is nothing to drift.
#
# The must-block fixtures are the real shapes from the 2026-09-08 run, where
# "the instruction should say X" was read as authorisation: a python heredoc
# that write_text()s an agent instruction file, and its Edit-tool equivalent.
# The must-allow fixtures include the rollback that undid
# it (git checkout -- <file>): a guard that blocks the recovery is worse than
# no guard.
#
# Verdict convention: a body exits 2 to block a tool call. Any other exit
# status means the call proceeds.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-instruction-edit-guard.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entries by a fixed substring of their message, never by index.
bash_body=$(jq -r '.hooks.PreToolUse[] | select(.matcher == "Bash") | .hooks[].command
  | select(contains("instruction file while an interview is live"))' "$settings")
edit_body=$(jq -r '.hooks.PreToolUse[] | select(.matcher | test("Edit")) | .hooks[].command
  | select(contains("instruction file while an interview is live"))' "$settings")
[ -n "$bash_body" ] || { echo "FAIL: Bash-side guard body not found in $settings"; exit 1; }
[ -n "$edit_body" ] || { echo "FAIL: Edit/Write-side guard body not found in $settings"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/instruction-edit-guard.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/ai-docs/plans"

failures=0

# Put the scratch plans/ folder into one of three states:
#   live      -- a state file exists, no in-flight marker  (guard armed)
#   inflight  -- state file AND marker                      (Steps 8-12, exempt)
#   none      -- no state file                              (no interview, exempt)
set_state() {
  rm -f "$scratch"/ai-docs/plans/*.spec.md.state.md "$scratch/ai-docs/plans/.task-inflight"
  case "$1" in
    live)     : > "$scratch/ai-docs/plans/2026-09-08-fixture.spec.md.state.md" ;;
    inflight) : > "$scratch/ai-docs/plans/2026-09-08-fixture.spec.md.state.md"
              : > "$scratch/ai-docs/plans/.task-inflight" ;;
    none)     ;;
    *) echo "bad state $1"; exit 1 ;;
  esac
}

# verdict <bash|edit> <payload-json>
verdict() {
  local body rc
  case "$1" in bash) body=$bash_body ;; edit) body=$edit_body ;; esac
  (cd "$scratch" && printf '%s' "$2" | bash -c "$body" >/dev/null 2>&1) && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

check_bash() {  # want state command
  local got payload
  set_state "$2"
  payload=$(jq -n --arg c "$3" '{tool_input: {command: $c}}')
  got=$(verdict bash "$payload")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, state=%s, for: %s\n' "$1" "$got" "$2" "$3"
    failures=$((failures + 1))
  fi
}

check_edit() {  # want state file_path
  local got payload
  set_state "$2"
  payload=$(jq -n --arg f "$3" '{tool_input: {file_path: $f}}')
  got=$(verdict edit "$payload")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, state=%s, for file_path: %s\n' "$1" "$got" "$2" "$3"
    failures=$((failures + 1))
  fi
}

# The real command from the 2026-09-08 run, trimmed to its load-bearing lines.
real_write=$(cat <<'REAL'
cd /home/syt/lab-game
python3 - <<'PY'
import pathlib
p = pathlib.Path('.claude/agents/spec-writer.md')
s = p.read_text(encoding='utf-8')
s = s.replace(old_head, new_head, 1)
p.write_text(s, encoding='utf-8')
print('spec-writer.md edited')
PY
sed -n '132,141p' .claude/agents/spec-writer.md
REAL
)

# --- Bash side, guard armed (state=live) ---
check_bash BLOCK live "$real_write"
check_bash BLOCK live "sed -i 's/foo/bar/' .claude/agents/spec-writer.md"
check_bash BLOCK live "cat >> AGENTS.md <<'EOF'
extra rule
EOF"
check_bash BLOCK live "echo x > .claude/settings.json"
check_bash BLOCK live "printf '%s\n' rule | tee -a ai-docs/doc-convention.md"
check_bash BLOCK live "cp tmp/draft.md .claude/skills/task/SKILL.md"
check_bash BLOCK live "git mv .claude/agents/design.md .claude/agents/design-writer.md"
check_bash BLOCK live "python3 -c \"open('ai-docs/code-style.md','a').write('x')\""
check_bash BLOCK live "cd /home/syt/lab-game && sed -i.orig 's/a/b/' CLAUDE.md"
# --- Bash side, guard armed: reads and recoveries stay legal ---
check_bash ALLOW live "sed -n '127,132p' .claude/agents/spec-writer.md"
check_bash ALLOW live "grep -n 'Rule 8' .claude/agents/spec-writer.md | head"
check_bash ALLOW live "cat .claude/settings.json | jq '.hooks'"
check_bash ALLOW live "git checkout -- .claude/agents/spec-writer.md"
check_bash ALLOW live "git restore .claude/agents/spec-writer.md"
check_bash ALLOW live "git diff .claude/agents/spec-writer.md > tmp/spec-writer.diff"
check_bash ALLOW live "cp .claude/agents/spec-writer.md tmp/spec-writer.md.bak"
check_bash ALLOW live "cat >> ai-docs/harness-gaps.md <<'EOF'
### 2026-09-08 -- target: .claude/agents/spec-writer.md
EOF"
check_bash ALLOW live "cat >> ai-docs/learnings.md <<'EOF'
**Rule:** never edit .claude/** on a diagnosis
EOF"
check_bash ALLOW live "rm -f tmp/gate.log; grep -c '' .claude/agents/spec-writer.md"
check_bash ALLOW live "sed -i 's/round: 1/round: 2/' ai-docs/plans/2026-09-08-fixture.spec.md.state.md"
# --- Bash side, guard disarmed by the in-flight marker (Steps 8-12) ---
check_bash ALLOW inflight "$real_write"
check_bash ALLOW inflight "sed -i 's/foo/bar/' .claude/agents/spec-writer.md"
# --- Bash side, no interview on this branch (/improve, owner edits) ---
check_bash ALLOW none "$real_write"
check_bash ALLOW none "echo x > AGENTS.md"

# --- Edit/Write side ---
check_edit BLOCK live "/home/syt/lab-game/.claude/agents/spec-writer.md"
check_edit BLOCK live ".claude/agents/spec-writer.md"
check_edit BLOCK live "/home/syt/lab-game/AGENTS.md"
check_edit BLOCK live "CLAUDE.md"
check_edit BLOCK live "ai-docs/code-style.md"
check_edit BLOCK live "/home/syt/lab-game/ai-docs/doc-convention.md"
check_edit ALLOW live "/home/syt/lab-game/ai-docs/plans/2026-09-08-fixture.spec.md"
check_edit ALLOW live "ai-docs/harness-gaps.md"
check_edit ALLOW live "ai-docs/learnings.md"
check_edit ALLOW live "internal/config/config.go"
check_edit ALLOW inflight "/home/syt/lab-game/.claude/agents/spec-writer.md"
check_edit ALLOW none "/home/syt/lab-game/.claude/agents/spec-writer.md"

if [ "$failures" -eq 0 ]; then
  echo "instruction-edit guard: all fixtures behave as specified"
  exit 0
fi
printf 'instruction-edit guard: %d check(s) failed\n' "$failures"
exit 1
