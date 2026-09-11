#!/usr/bin/env bash
# Regression suite for the Stop hook gate.
#
# The gate blocks a turn that ends while the in-flight marker exists and the
# turn neither advanced the flow nor handed back. A hand-back token
# disarms it for ONE stop, and this hook spends the token itself.
#
# WHY THE HOOK SPENDS IT. The token used to be cleared by the orchestrator's
# next turn — a disposition, and it failed the way dispositions fail. Measured
# in the run that first hit it: the clearing `sed` travelled inside a compound
# Bash command whose other half was refused by the PreToolUse piped-gate guard,
# so the whole command was dropped, the agent re-sent only the half it had come
# for, and a seven-minute-old token was still the marker's last line when the
# next turn announced "moving to Step 12" and stopped. The gate exited 0 in
# silence and the session idled for two hours. Case `replay-pr54` below is that
# sequence; it must end in a BLOCK. Its name is the historical label the case
# has carried since it was written.
#
# Anti-drift: this suite runs the LIVE hook body, extracted with jq and executed
# as the program it is. No regex is copied here, so there is nothing to drift.
#
# Verdict convention: the body exits 2 to block the stop. Any other status lets
# the turn end.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-stop-gate.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select by a fixed substring of the message, never by index.
body=$(jq -r '.hooks.Stop[].hooks[].command
  | select(contains("BLOCKED: /task is in-flight"))' "$settings")
[ -n "$body" ] || { echo "FAIL: stop-gate body not found in $settings"; exit 1; }

failures=0
sandbox=$(mktemp -d)
trap 'chmod -R u+w "$sandbox" 2>/dev/null; rm -rf "$sandbox"' EXIT

# The body hard-codes a repo-relative marker path, so each case runs in its own
# sandbox that mimics the tree shape rather than touching the real marker.
marker_rel="ai-docs/plans/.task-inflight"

# run <case-dir> <stop_hook_active:true|false> -> prints BLOCK or ALLOW
run() {
  local dir=$1 active=$2 rc payload
  payload=$(jq -nc --argjson a "$active" '{stop_hook_active: $a}')
  ( cd "$dir" && printf '%s' "$payload" | bash -c "$body" ) >/dev/null 2>&1 && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

new_case() {
  local dir="$sandbox/$1"
  mkdir -p "$dir/ai-docs/plans"
  printf '%s\n' "$dir"
}

expect() {
  local what=$1 got=$2 case=$3
  [ "$what" = "$got" ] && return 0
  printf 'FAIL [%s]: expected %s, got %s\n' "$case" "$what" "$got"
  failures=$((failures + 1))
}

# --- 1. no marker: the gate is not armed at all -----------------------------
d=$(new_case no-marker)
expect ALLOW "$(run "$d" false)" no-marker

# --- 2. stop_hook_active: never re-block its own continuation ---------------
d=$(new_case active)
printf '2026-09-05T11:00:00Z\n' > "$d/$marker_rel"
expect ALLOW "$(run "$d" true)" active
grep -q '^blocked: ' "$d/$marker_rel" \
  && { echo "FAIL [active]: a re-entrant stop wrote a ledger line"; failures=$((failures + 1)); }

# --- 3. marker, no token: block, and record the block -----------------------
d=$(new_case bare-marker)
printf '2026-09-05T11:00:00Z\n' > "$d/$marker_rel"
expect BLOCK "$(run "$d" false)" bare-marker
grep -q '^blocked: [0-9]\{4\}-' "$d/$marker_rel" \
  || { echo "FAIL [bare-marker]: no dated ledger line was appended"; failures=$((failures + 1)); }

# --- 4. fresh token: allow ONE stop, and spend the token --------------------
d=$(new_case token-spent)
printf '2026-09-05T11:00:00Z\nhandback: 2026-09-05T12:55:03Z awaiting self-review round 2\n' > "$d/$marker_rel"
expect ALLOW "$(run "$d" false)" token-spent
grep -q '^handback-spent: 2026-09-05T12:55:03Z' "$d/$marker_rel" \
  || { echo "FAIL [token-spent]: the token was not spent"; failures=$((failures + 1)); }
grep -q '^handback: ' "$d/$marker_rel" \
  && { echo "FAIL [token-spent]: a live token survived the stop it permitted"; failures=$((failures + 1)); }

# --- 5. replay of the recorded failure: one token, two stops ----------------
# Stop 1 is the legitimate hand-back while self-review round 2 runs. The
# delegate returns, the turn announces Step 12 and ends without doing it.
# Stop 2 must BLOCK. Before this hook spent its own tokens it did not.
d=$(new_case replay-pr54)
printf '2026-09-05T11:07:14Z\nhandback: 2026-09-05T12:55:03Z awaiting self-review round 2 verdict\n' > "$d/$marker_rel"
expect ALLOW "$(run "$d" false)" replay-pr54/stop-1
expect BLOCK "$(run "$d" false)" replay-pr54/stop-2

# --- 6. a token outside the last three lines does not disarm ----------------
# The tail -n 3 window is part of the contract; a token buried under later
# activity is not a hand-back for the stop happening now.
d=$(new_case token-buried)
printf 'handback: 2026-09-05T12:00:00Z stale\nblocked: 2026-09-05T12:10:00Z\nblocked: 2026-09-05T12:20:00Z\nblocked: 2026-09-05T12:30:00Z\n' > "$d/$marker_rel"
expect BLOCK "$(run "$d" false)" token-buried

# --- 7. the token cannot be spent: fail OPEN, and say so --------------------
# Named fail direction. The hook cannot spend the token, so it cannot promise
# the next stop will be caught; blocking a legitimate hand-back over a file
# permission would be the expensive error. It warns on stderr instead.
#
# The unwritable thing is the DIRECTORY, not the file: `sed -i` unlinks and
# recreates, so a read-only marker in a writable directory is still rewritten.
# A fixture that chmod'ed the file would have asserted nothing.
d=$(new_case unspendable)
printf '2026-09-05T11:00:00Z\nhandback: 2026-09-05T12:00:00Z awaiting delegate return\n' > "$d/$marker_rel"
chmod 555 "$d/ai-docs/plans"
warn=$( (cd "$d" && printf '{"stop_hook_active": false}' | bash -c "$body") 2>&1 >/dev/null )
rc=$?
chmod 755 "$d/ai-docs/plans"
[ "$rc" -ne 2 ] || { echo "FAIL [unspendable]: blocked on a directory permission"; failures=$((failures + 1)); }
printf '%s' "$warn" | grep -q 'could not spend' \
  || { echo "FAIL [unspendable]: failed open in silence"; failures=$((failures + 1)); }

# --- 8. the ledger the closing report is required to cite --------------------
# Step 12 item 13 reads these two counts out of the marker before removing it.
d=$(new_case ledger)
printf '2026-09-05T11:00:00Z\n' > "$d/$marker_rel"
run "$d" false >/dev/null
printf 'handback: 2026-09-05T12:00:00Z awaiting delegate return\n' >> "$d/$marker_rel"
run "$d" false >/dev/null
run "$d" false >/dev/null
counts=$(awk '/^blocked: /{b++} /^handback-spent: /{h++} END{printf "%d/%d", b+0, h+0}' "$d/$marker_rel")
[ "$counts" = "2/1" ] \
  || { printf 'FAIL [ledger]: blocks/hand-backs = %s, want 2/1\n' "$counts"; failures=$((failures + 1)); }

# --- 9-13. the gate belongs to the session that owns the marker --------------
# A second session in the same working tree -- a conversation the owner runs
# beside a /task -- was blocked at its own stop and wrote a line into the
# running flow's ledger. The marker now names its owner, and a stop by any
# other session is not this gate's business: no block, no ledger line, and no
# hand-back token spent on its behalf. A marker with no owner line, or a stop
# whose input carries no session id, keeps the old behaviour, so every failure
# of the new half degrades to the gate as it was, never to no gate.
run_sid() {
  local dir=$1 sid=$2 rc payload
  payload=$(jq -nc --arg s "$sid" '{stop_hook_active: false, session_id: $s}')
  ( cd "$dir" && printf '%s' "$payload" | bash -c "$body" ) >/dev/null 2>&1 && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

d=$(new_case owner-stops)
printf '2026-09-11T17:00:00Z\nowner: session-a\n' > "$d/$marker_rel"
expect BLOCK "$(run_sid "$d" session-a)" owner-stops
grep -q '^blocked: ' "$d/$marker_rel" \
  || { echo "FAIL [owner-stops]: the owner's block was not recorded"; failures=$((failures + 1)); }

d=$(new_case foreign-stops)
printf '2026-09-11T17:00:00Z\nowner: session-a\nhandback: 2026-09-11T19:00:00Z awaiting delegate return\n' > "$d/$marker_rel"
expect ALLOW "$(run_sid "$d" session-b)" foreign-stops
grep -q '^blocked: ' "$d/$marker_rel" \
  && { echo "FAIL [foreign-stops]: a foreign stop wrote into the ledger"; failures=$((failures + 1)); }
grep -q '^handback: 2026-09-11T19:00:00Z' "$d/$marker_rel" \
  || { echo "FAIL [foreign-stops]: a foreign stop spent the owner's token"; failures=$((failures + 1)); }

d=$(new_case no-owner-line)
printf '2026-09-11T17:00:00Z\n' > "$d/$marker_rel"
expect BLOCK "$(run_sid "$d" session-b)" no-owner-line

d=$(new_case no-session-id)
printf '2026-09-11T17:00:00Z\nowner: session-a\n' > "$d/$marker_rel"
expect BLOCK "$(run "$d" false)" no-session-id

d=$(new_case reclaimed)
printf '2026-09-11T17:00:00Z\nowner: session-a\nclaim: 2026-09-11T18:00:00Z\nowner: session-c\n' > "$d/$marker_rel"
expect BLOCK "$(run_sid "$d" session-c)" reclaimed/new-owner
expect ALLOW "$(run_sid "$d" session-a)" reclaimed/old-owner

# --- 14-19. the stamp: which commands make a session the owner ----------------
# The owner is recorded by a PostToolUse hook, from the session id in its own
# input, when the main agent's command creates the marker or appends a claim
# line to it. A subagent shares the session id but is not the flow; a handback
# or a read is not a creation; and with no marker there is nothing to stamp.
stamp=$(jq -r '.hooks.PostToolUse[].hooks[].command
  | select(contains("owner: %s"))' "$settings")
[ -n "$stamp" ] || { echo "FAIL: marker-stamp body not found in $settings"; exit 1; }

# stamp_run <case-dir> <session> <agent-id or empty> <command>
stamp_run() {
  local dir=$1 sid=$2 agent=$3 cmd=$4 payload
  if [ -n "$agent" ]; then
    payload=$(jq -nc --arg s "$sid" --arg a "$agent" --arg c "$cmd" '{session_id: $s, agent_id: $a, tool_input: {command: $c}}')
  else
    payload=$(jq -nc --arg s "$sid" --arg c "$cmd" '{session_id: $s, tool_input: {command: $c}}')
  fi
  ( cd "$dir" && printf '%s' "$payload" | bash -c "$stamp" ) >/dev/null 2>&1
}
owners() { grep -c '^owner: ' "$1/$marker_rel" 2>/dev/null || true; }
last_owner() { sed -n 's/^owner: //p' "$1/$marker_rel" | tail -n 1; }

create='date -u +%FT%TZ > ai-docs/plans/.task-inflight'
claim="echo \"claim: \$(date -u +%FT%TZ)\" >> ai-docs/plans/.task-inflight"

d=$(new_case stamp-create)
printf '2026-09-11T17:00:00Z\n' > "$d/$marker_rel"
stamp_run "$d" session-a "" "$create"
[ "$(last_owner "$d")" = session-a ] \
  || { echo "FAIL [stamp-create]: the creating session was not recorded as owner"; failures=$((failures + 1)); }

d=$(new_case stamp-handback)
printf '2026-09-11T17:00:00Z\nowner: session-a\n' > "$d/$marker_rel"
stamp_run "$d" session-b "" 'echo "handback: 2026-09-11T19:00:00Z awaiting delegate return" >> ai-docs/plans/.task-inflight'
stamp_run "$d" session-b "" 'cat ai-docs/plans/.task-inflight'
if [ "$(owners "$d")" != 1 ] || [ "$(last_owner "$d")" != session-a ]; then
  echo "FAIL [stamp-handback]: a hand-back or a read changed the owner"; failures=$((failures + 1))
fi

d=$(new_case stamp-subagent)
printf '2026-09-11T17:00:00Z\n' > "$d/$marker_rel"
stamp_run "$d" session-a agent-7 "$create"
[ "$(owners "$d")" = 0 ] \
  || { echo "FAIL [stamp-subagent]: a subagent's command stamped the marker"; failures=$((failures + 1)); }

d=$(new_case stamp-claim)
printf '2026-09-11T17:00:00Z\nowner: session-a\n' > "$d/$marker_rel"
stamp_run "$d" session-c "" "$claim"
stamp_run "$d" session-c "" "$claim"
if [ "$(last_owner "$d")" != session-c ] || [ "$(owners "$d")" != 2 ]; then
  echo "FAIL [stamp-claim]: a claim did not move ownership exactly once"; failures=$((failures + 1))
fi

d=$(new_case stamp-no-marker)
stamp_run "$d" session-a "" "$create"
[ ! -e "$d/$marker_rel" ] \
  || { echo "FAIL [stamp-no-marker]: the stamp created a marker"; failures=$((failures + 1)); }

d=$(new_case stamp-end-to-end)
printf '2026-09-11T17:00:00Z\n' > "$d/$marker_rel"
stamp_run "$d" session-a "" "$create"
expect ALLOW "$(run_sid "$d" session-b)" stamp-end-to-end/foreign
expect BLOCK "$(run_sid "$d" session-a)" stamp-end-to-end/owner

if [ "$failures" -eq 0 ]; then
  echo "stop gate: all fixtures behave as specified"
  exit 0
fi
printf 'stop gate: %d check(s) failed\n' "$failures"
exit 1
