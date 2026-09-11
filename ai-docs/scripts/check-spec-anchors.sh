#!/usr/bin/env bash
# Every row a spec binds the design with names its source in the owner's words.
#
# WHAT THIS REFUSES, in a spec file:
#   1. an item of the Scope list, a row of the Key decisions table or a row of
#      the Acceptance Criteria table that carries no anchor. The anchor forms
#      are a closed list of two, and both quote:
#        [task: "<fragment>"]              a verbatim fragment of the task text
#        [answer <round>.<n>: "<fragment>"] a verbatim fragment of the n-th
#                                          owner answer recorded in that round
#   2. with the interview state file on disk: a fragment the named source does
#      not contain, an answer id that names no recorded answer, and any task
#      anchor when the state file holds no task text at all -- an issue body
#      marked superseded with no task_description block beside it.
#
# An answer anchor quotes for the same reason a task anchor does: an id alone
# resolves for any row at all, and a quote is something a reader can hold the
# row against.
#
# WHY A GATE AND NOT A SENTENCE. A spec holds what the task needs in order to
# count as solved, and nothing about how the solution is built. The charter
# already said "apply the standing rules silently" and "leave architecture to
# the design", and the drafting agent still wrote both into the spec: the
# owner struck the restated rules and the prescribed mechanisms from one
# acceptance table, and the struck mechanism survived in the decisions table
# and in the scope list through two further rulings, because each ruling was
# applied to a section and not to the class. A standing rule, a design
# default and an orchestrator's instruction share one property: none of them
# is in the owner's words, so none of them can carry an anchor that resolves.
#
# WHAT IT CANNOT SEE, stated so nobody records a pass it never earned. An
# anchor proves that a source exists, not that the row belongs in the spec. A
# mechanism written under a real quote passes here, and so does a standing
# rule the task text happens to mention. Those remain the charter's to forbid
# and the design review's to catch.
#
# FAIL DIRECTION, per dependency. The state file sits beside the spec for the
# whole interview, or in the plans directory's ignored folder once the run has
# retired it, and it is never in the pull request -- so CI cannot resolve an
# anchor. Wherever the file is missing, presence is still checked and
# resolution is skipped with a notice on stderr. A row whose cell begins with
# TBD is a question still open and is not judged; fenced code is skipped.
#
# Exit 0 = conforms (or nothing to check). Exit 1 = at least one finding.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  check-spec-anchors.sh               check the Scope items, Key decisions rows
                                      and Acceptance Criteria rows this branch
                                      added or changed in any spec file,
                                      against the merge base with main
  check-spec-anchors.sh <file>...     audit the named specs in full
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-spec-anchors: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

# The interview state file of a spec: its sibling while the run is live, the
# retired copy once the run has moved it out of the plans directory.
state_for() {
  local spec=$1 retired
  retired="ai-docs/plans/ignored/$(basename "$spec").state.md"
  if [ -f "$spec.state.md" ]; then
    printf '%s\n' "$spec.state.md"
  elif [ -f "$retired" ]; then
    printf '%s\n' "$retired"
  fi
}

# Reads one spec from `input` (`-` is stdin). `ranges` is `all` or a comma-separated list of
# `<first>-<last>` line spans; a unit is judged when any of its lines falls in
# one. Prints `F<TAB>file<TAB>unit<TAB>kind<TAB>detail` per finding and
# `N<TAB>file` when an anchor could not be resolved for want of a state file.
# Patterns and inputs travel through the environment, never `awk -v`.
judge_spec() {
  file="$1" ranges="$2" state="$3" input="$4"
  file="$file" ranges="$ranges" state="$state" awk '
    function trim(s) { sub(/^[[:space:]]+/, "", s); sub(/[[:space:]]+$/, "", s); return s }
    function norm(s) { gsub(/[[:space:]]+/, " ", s); return trim(s) }
    function hot(a, b,   i) {
      if (all) return 1
      for (i = 1; i <= nr; i++) if (rs[i] <= b && re[i] >= a) return 1
      return 0
    }
    function report(unit, kind, detail) {
      printf "F\t%s\t%s\t%s\t%s\n", file, unit, kind, detail
    }
    function judge(unit, text, lo, hi,   t, frag, found, id, rn, a, key) {
      if (!hot(lo, hi)) return
      found = 0
      t = text
      while (match(t, /\[task: "[^"]*"\]/)) {
        found = 1
        frag = substr(t, RSTART + 8, RLENGTH - 10)
        gsub(/\\\|/, "|", frag)
        frag = norm(frag)
        if (!have_state) skipped = 1
        else if (task == "") report(unit, "no-task-text", frag)
        else if (frag == "" || index(task, frag) == 0) report(unit, "task-unresolved", frag)
        t = substr(t, RSTART + RLENGTH)
      }
      t = text
      while (match(t, /\[answer [0-9]+\.[0-9]+: "[^"]*"\]/)) {
        found = 1
        a = substr(t, RSTART + 8, RLENGTH - 9)
        id = a; sub(/:.*/, "", id)
        frag = a; sub(/^[^"]*"/, "", frag); sub(/"$/, "", frag)
        gsub(/\\\|/, "|", frag)
        frag = norm(frag)
        split(id, rn, ".")
        key = (rn[1] + 0) SUBSEP (rn[2] + 0)
        if (!have_state) skipped = 1
        else if (!(key in ans)) report(unit, "answer-unresolved", id " (no such answer)")
        else if (frag == "" || index(norm(ans[key]), frag) == 0) report(unit, "answer-unresolved", id ": " frag)
        t = substr(t, RSTART + RLENGTH)
      }
      if (!found) report(unit, "no-anchor", substr(norm(text), 1, 90))
    }
    # A table row is held back one line: the row right before a separator is
    # the header, whatever its titles are.
    function table_line(line, n,   c, k, first, i, cell) {
      k = split(line, c, "|")
      first = trim(c[2])
      if (first ~ /^:?-+:?$/) { held = ""; return }
      release()
      held = line; held_n = n; held_sec = sec
    }
    function release(   c, k, first, i, cell, unit) {
      if (held == "") return
      k = split(held, c, "|")
      first = trim(c[2])
      for (i = 3; i < k; i++) {
        cell = trim(c[i])
        if (cell ~ /^[*_]*TBD/) { held = ""; return }
      }
      if (held_sec == "ac") {
        if (first ~ /^AC[0-9]+$/) judge(first, held, held_n, held_n)
      } else {
        unit = first
        if (k > 4) unit = first " " trim(c[3])
        judge("Key decision \"" substr(unit, 1, 50) "\"", held, held_n, held_n)
      }
      held = ""
    }
    function flush() {
      release()
      if (u_open) judge("Scope item \"" substr(norm(u_text), 1, 50) "\"", u_text, u_start, u_end)
      u_open = 0; u_text = ""; blank = 0
    }
    BEGIN {
      file = ENVIRON["file"]
      all = (ENVIRON["ranges"] == "all")
      nr = 0
      if (!all) {
        n = split(ENVIRON["ranges"], parts, ",")
        for (i = 1; i <= n; i++) {
          if (parts[i] == "") continue
          split(parts[i], ab, "-")
          nr++; rs[nr] = ab[1] + 0; re[nr] = ab[2] + 0
        }
      }
      state = ENVIRON["state"]; have_state = (state != "")
      task = ""; tdesc = ""; ghi = ""; has_desc = 0; superseded = 0
      if (have_state) {
        inyaml = 0; region = ""
        while ((getline l < state) > 0) {
          if (l ~ /^```/) { inyaml = !inyaml; region = ""; continue }
          if (!inyaml) continue
          if (l ~ /^[A-Za-z_]+:/) {
            region = l; sub(/:.*/, "", region)
            if (region == "task_description") {
              has_desc = 1
              rest = l; sub(/^task_description:[[:space:]]*\|?/, "", rest)
              tdesc = tdesc " " rest
            }
            continue
          }
          if (region == "task_description") tdesc = tdesc " " l
          else if (region == "gh_issue") {
            # Only the words of the issue are task text: its title, body and
            # comments -- never the labels, the state or the linked numbers.
            if (l ~ /^  [A-Za-z_]+:/) {
              field = l; sub(/^  /, "", field); sub(/:.*/, "", field)
              rest = l; sub(/^  [A-Za-z_]+:[[:space:]]*/, "", rest)
              if (field == "issue_body_status" && rest ~ /^superseded/) superseded = 1
              if (field == "title" || field == "comments") ghi = ghi " " rest
            } else if (field == "title" || field == "body" || field == "comments") ghi = ghi " " l
          } else if (region == "prior_qa") {
            # One entry per answer; the answer field may wrap onto deeper
            # lines, and any other field of the entry ends it.
            if (l ~ /^[[:space:]]*-[[:space:]]*round:[[:space:]]*[0-9]+/) {
              r = l; sub(/^[[:space:]]*-[[:space:]]*round:[[:space:]]*/, "", r)
              r = r + 0; cnt[r]++; cur = r SUBSEP cnt[r]; ans[cur] = ""; inans = 0
            } else if (l ~ /^[[:space:]]+answer:/) {
              rest = l; sub(/^[[:space:]]+answer:[[:space:]]*/, "", rest)
              ans[cur] = ans[cur] " " rest; inans = 1
            } else if (l ~ /^[[:space:]]+[A-Za-z_]+:/) inans = 0
            else if (inans) ans[cur] = ans[cur] " " l
          }
        }
        close(state)
        if (has_desc) task = norm(tdesc)
        else if (!superseded) task = norm(ghi)
      }
    }
    {
      line = $0
      if (line ~ /^[[:space:]]*```/) { flush(); fence = !fence; next }
      if (fence) next
      if (line ~ /^#+[[:space:]]/) {
        flush()
        if (line ~ /^##[[:space:]]/) {
          h = tolower(line)
          if (h ~ /^##[[:space:]]+scope[[:space:]]*$/) sec = "scope"
          else if (h ~ /^##[[:space:]]+key decisions[[:space:]]*$/) sec = "kd"
          else if (h ~ /^##[[:space:]]+acceptance criteria[[:space:]]*$/) sec = "ac"
          else sec = ""
        }
        next
      }
      if (sec == "kd" || sec == "ac") {
        if (line ~ /^[[:space:]]*\|/) table_line(line, NR)
        else release()
        next
      }
      if (sec == "scope") {
        if (line ~ /^([0-9]+\.|[-*+])[[:space:]]/) {
          flush(); u_open = 1; u_text = line; u_start = NR; u_end = NR
          next
        }
        if (line ~ /^[[:space:]]*$/) { if (u_open) blank = 1; next }
        if (u_open && (line ~ /^[[:space:]]/ || !blank)) {
          u_text = u_text " " line; u_end = NR; blank = 0
          next
        }
        flush()
      }
    }
    END {
      flush()
      if (skipped) printf "N\t%s\n", file
    }' "$input"
}

out=""
if [ $# -gt 0 ]; then
  for f in "$@"; do
    [ -r "$f" ] || continue
    out+=$(judge_spec "$f" all "$(state_for "$f")" "$f")$'\n'
  done
else
  base=$(git merge-base origin/main HEAD 2>/dev/null || git merge-base main HEAD 2>/dev/null)
  if [ -z "$base" ]; then
    printf 'check-spec-anchors: no merge base with main; skipped\n' >&2
    exit 0
  fi
  # One record per spec file this branch touched: the path and the line spans,
  # at HEAD, of what the branch added or changed.
  while IFS=$'\t' read -r f spans; do
    [ -n "$f" ] || continue
    out+=$(git show "HEAD:$f" 2>/dev/null | judge_spec "$f" "$spans" "$(state_for "$f")" -)$'\n'
  done < <(git diff -U0 "$base"...HEAD -- '*.spec.md' 2>/dev/null | awk '
    /^\+\+\+ / {
      if (f != "") print f "\t" r
      f = ""; r = ""
      if ($0 ~ /^\+\+\+ b\//) f = substr($0, 7)
      next
    }
    /^@@ / && f != "" {
      if (match($0, /\+[0-9]+(,[0-9]+)?/)) {
        n = split(substr($0, RSTART + 1, RLENGTH - 1), p, ",")
        len = (n > 1) ? p[2] + 0 : 1
        if (len > 0) r = r (r == "" ? "" : ",") p[1] "-" (p[1] + len - 1)
      }
    }
    END { if (f != "") print f "\t" r }')
fi

while IFS=$'\t' read -r tag f _; do
  [ "$tag" = N ] && printf 'check-spec-anchors: %s: no interview state file on disk; anchors checked for presence only\n' "$f" >&2
done <<< "$out"

findings=$(printf '%s' "$out" | awk -F'\t' '$1 == "F"')
if [ -z "$findings" ]; then
  exit 0
fi

printf 'check-spec-anchors: a spec row names no source in the owner'"'"'s words.\n\n' >&2
printf '%s\n' "$findings" | while IFS=$'\t' read -r _ f unit kind detail; do
  printf '  %s  %s  [%s]  %s\n' "$f" "$unit" "$kind" "${detail:0:90}" >&2
done
cat >&2 <<'MSG'

Every Scope item, Key decisions row and Acceptance Criteria row ends with one of:
  [task: "<verbatim fragment of the task text>"]
  [answer <round>.<n>: "<verbatim fragment of that answer>"]
                     the n-th prior_qa entry recorded with that round

no-anchor          -> a row with no source in the owner's words is not the
                      spec's: a standing rule (Rule 1), a design choice (What to
                      leave to the design phase), or an instruction from
                      outside (Rule 10). Drop it, or ask the owner and anchor
                      the answer.
task-unresolved    -> quote the persisted task text verbatim; only whitespace
                      is normalised, and a table cell writes a pipe as \|
answer-unresolved  -> <round>.<n> counts the prior_qa entries of that round,
                      and the fragment is quoted from that entry's answer
no-task-text       -> the issue body is superseded and the state file carries
                      no task_description block; the interview writes TASK there

spec-writer.md Rule 11. An anchor proves a source, not a remit: a mechanism
under a real quote passes here and is still Rule 11's to forbid.
MSG
exit 1
