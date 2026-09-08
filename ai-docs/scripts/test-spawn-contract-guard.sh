#!/usr/bin/env bash
# Regression suite for the PreToolUse reviewer-spawn-contract hook guard.
#
# The guard blocks a `self-review` / `design-review` spawn whose prompt carries
# a line outside the closed list those two agent files declare. Reviewer-side
# enforcement — a `major` PROMPT-CONTAMINATION finding — costs a whole review
# round to discharge; the hook costs one re-spawn, before the round is spent.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq and executed
# as the program it is. There is no copied regex here, so there is nothing to
# drift.
#
# The corpora are artefacts, not imagination — a corpus you wrote is drawn from
# the same imagination that wrote the bug:
#   must-allow -- the four spawn templates this harness actually ships, with
#                 their placeholders realised.
#   must-block -- the round-2 prompt recorded verbatim in the 2026-09-03 run's
#                 review register, which the reviewer opened with a `major`
#                 PROMPT-CONTAMINATION row, plus the two pre-unification
#                 template shapes, so a revert to either fails here instead of
#                 in a review round.
#
# Verdict convention: the body exits 2 to block a spawn. Any other exit status
# means the spawn proceeds.
#
# Fail-open cases are asserted too. The guard keys on harness-supplied fields,
# so a missing `jq`, an unparseable payload, an absent `subagent_type` and an
# empty prompt must all ALLOW: the reviewer-side rule is the backstop, and a
# guard that blocks on its own instrument failure stops every review.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-spawn-contract-guard.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entry by a fixed substring of its message, never by index: an index
# silently re-points the moment another PreToolUse entry is added.
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("spawn prompt carries content beyond the closed list"))' "$settings")
[ -n "$body" ] || { echo "FAIL: spawn-contract guard body not found in $settings"; exit 1; }

failures=0

# Feed one raw hook payload to the real body and report BLOCK or ALLOW.
raw_verdict() {
  local rc
  printf '%s' "$1" | bash -c "$body" >/dev/null 2>&1 && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

verdict() {
  raw_verdict "$(jq -n --arg st "$1" --arg p "$2" '{tool_input: {subagent_type: $st, prompt: $p}}')"
}

# check <want> <label> <subagent_type> <prompt>
check() {
  local got
  got=$(verdict "$3" "$4")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, for: %s\n' "$1" "$got" "$2"
    failures=$((failures + 1))
  fi
}

# check_raw <want> <label> <payload>
check_raw() {
  local got
  got=$(raw_verdict "$3")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, for: %s\n' "$1" "$got" "$2"
    failures=$((failures + 1))
  fi
}

# --- must allow: the shipped templates, placeholders realised ---

check ALLOW '/task Step 10 self-review' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Spec: ai-docs/plans/2026-09-04-name.spec.md
  Design: ai-docs/plans/2026-09-04-name.design.md
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  7efdbb7..HEAD
P
)"

check ALLOW '/task design-review' design-review "$(cat <<'P'
  Read .claude/agents/design-review.md and follow it.
  Spec: ai-docs/plans/2026-09-04-name.spec.md
  Design: ai-docs/plans/2026-09-04-name.design.md
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  Round: 2
P
)"

check ALLOW '/bugfix self-review' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Spec-equivalent: ai-docs/bugfix/trace-2026-09-04-name.md
  Progress: ai-docs/bugfix/trace-2026-09-04-name.md
  Diff window: 9f8e7d6..HEAD
P
)"

check ALLOW '/project-review self-review' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-project-review.progress.md
  1111111..2222222
P
)"

check ALLOW 'bare paths, no labels' self-review "$(cat <<'P'
Read .claude/agents/self-review.md and follow it.
ai-docs/plans/2026-09-04-name.spec.md
ai-docs/plans/2026-09-04-name.design.md
ai-docs/plans/2026-09-04-name.progress.md
Commits: 7efdbb7..HEAD
P
)"

check ALLOW 'design-review, no progress file yet' design-review "$(cat <<'P'
  Read .claude/agents/design-review.md and follow it.
  Spec: ai-docs/plans/2026-09-04-name.spec.md
  Design: ai-docs/plans/2026-09-04-name.design.md
  Round: 1
P
)"

check ALLOW 'non-reviewer spawn is out of scope' code-writer "$(cat <<'P'
  Read .claude/agents/code-writer.md and follow it.
  Group 2 of 3, subtasks 4-6. The migration lands first; run go test ./... after each.
  Round 2 of the fix loop; golangci-lint run was GREEN at 7efdbb7.
P
)"

# --- must block: the recorded contamination, category by category ---

check BLOCK 'recorded round-2 prompt (2026-09-03)' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Spec: ai-docs/plans/2026-09-03-rename-design-subagent.spec.md
  Design: ai-docs/plans/2026-09-03-rename-design-subagent.design.md
  Progress: ai-docs/plans/2026-09-03-rename-design-subagent.progress.md
  7efdbb7..HEAD
  Round 2. Both Round-1 findings are addressed at 7efdbb7; re-verify them and re-run whatever they could have moved.
  Gates re-run after the last edit of the batch: go build GREEN, golangci-lint run 0 issues, check-citations.sh PASS.
  If you judge a scheduled action an insufficient close for a durable placeholder, say so and I will reword the heading instead.
P
)"

check BLOCK 'round history alone' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  7efdbb7..HEAD
  Round 2. Both Round-1 findings are addressed.
P
)"

check BLOCK 'gate results alone' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  7efdbb7..HEAD
  go build GREEN, golangci-lint run 0 issues.
P
)"

check BLOCK 'routing request alone' design-review "$(cat <<'P'
  Read .claude/agents/design-review.md and follow it.
  Design: ai-docs/plans/2026-09-04-name.design.md
  Round: 3
  Would you block on the handoff grouping, or can it wait for the amendment?
P
)"

check BLOCK 'pre-unification /project-review prose' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-project-review.progress.md
  base_commit is recorded in the progress file.
  There is no spec or design doc — this is a review-driven task.
  Treat the findings table in ## AC Status as the acceptance criteria.
P
)"

check BLOCK 'pre-unification /bugfix framing' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  This is a /bugfix self-review (no /task spec; no design doc).
  Spec-equivalent: ai-docs/bugfix/trace-2026-09-04-name.md
  Out-of-scope reminder: this self-review is scoped to fitness-against-the-bug.
  Diff window: 9f8e7d6..HEAD
P
)"

check BLOCK 'unsubstituted placeholder range' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  <base_commit>..HEAD
P
)"

check BLOCK 'a Context: line' design-review "$(cat <<'P'
  Read .claude/agents/design-review.md and follow it.
  Design: ai-docs/plans/2026-09-04-name.design.md
  Round: 2
  Context: the design was amended during implementation.
P
)"

check BLOCK 'a priority steer' self-review "$(cat <<'P'
  Read .claude/agents/self-review.md and follow it.
  Progress: ai-docs/plans/2026-09-04-name.progress.md
  7efdbb7..HEAD
  Focus on the migration test; the rest is mechanical.
P
)"

# --- must allow: instrument failure never blocks a review ---

check_raw ALLOW 'unparseable payload' 'not json at all'
check_raw ALLOW 'no subagent_type field' '{"tool_input": {"prompt": "Round 2. Everything is fixed."}}'
check ALLOW 'reviewer with empty prompt' self-review ''

# --- anti-drift: the contract this guard enforces still says five ---
# If a contract grows a sixth permitted item, the guard's alternation is stale
# and must be edited in the same change. That is what this assertion forces.
for f in .claude/agents/self-review.md .claude/agents/design-review.md; do
  grep -qF 'exactly five things' "$f" && continue
  printf 'FAIL: %s no longer declares "exactly five things" — the guard alternation may be stale\n' "$f"
  failures=$((failures + 1))
done

# ...and both gated agent types are still named in the body's own filter.
for st in self-review design-review; do
  grep -qF "$st" <<<"$body" && continue
  printf 'FAIL: guard body no longer names %s\n' "$st"
  failures=$((failures + 1))
done

if [ "$failures" -eq 0 ]; then
  echo "spawn-contract guard: all fixtures behave as specified"
  exit 0
fi
printf 'spawn-contract guard: %d check(s) failed\n' "$failures"
exit 1
