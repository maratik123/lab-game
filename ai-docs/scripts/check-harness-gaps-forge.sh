#!/usr/bin/env bash
# The two closing fields of the harness-gaps log are a forge's record and
# nobody else's: Forge names the forge that took an entry, Closed by names the
# merged pull request that fixed it outside any forge.
#
# WHAT THIS REFUSES.
#   1. Over the whole log, outside fenced code: a Forge value that is not a
#      comma-separated list of forge-<N> names, a Closed by value that is not a
#      comma-separated list of #<N> numbers, either field before the first entry
#      heading, and either field twice inside one entry.
#   2. Against the merge base with main: a line of either field this branch
#      added or removed, when the branch is not named harness/forge-<N>; and, on
#      such a branch, an added Forge value naming a forge above the branch's own
#      number.
#
# WHY A GATE AND NOT A SENTENCE. Any flow here may append to the log, and the
# two fields are how the owner reads which entries are still open. A flow that
# writes one onto its own entry takes that entry out of the open set, silently,
# and no later reader can tell such a mark from a forge's.
#
# FAIL DIRECTION. The branch name is the whole provenance signal. The CI job
# checks out the pull request's head branch, so that is the name read here. A
# local commit is not refused: the pull request is the first place a finding
# shows, and deleting the line is the whole fix. With no merge base, check 2
# has nothing to compare against and says so; check 1 still runs. A detached
# HEAD is not a forge. A Closed by number is not resolved here; the citation
# guard already refuses a number this repository never issued.
#
# Exit 0 = conforms. Exit 1 = at least one finding.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  check-harness-gaps-forge.sh    check the Forge and Closed by fields of the
                                 harness-gaps log: their shape over the whole
                                 file, and their provenance over the lines this
                                 branch added or removed against the merge base
                                 with main
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-harness-gaps-forge: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

log=ai-docs/harness-gaps.md
if [ ! -f "$log" ]; then
  printf 'check-harness-gaps-forge: %s absent; skipped\n' "$log" >&2
  exit 0
fi

findings=0

# --- 1: shape, over the whole log -------------------------------------------
# A field is found with index(), never a regex: its asterisks would need
# escapes, and awk re-processes escapes in a -v string. The value patterns
# carry no backslash for the same reason. The fenced skeleton carries both
# fields' template lines and is skipped.
shape=$(awk '
  function judge(name, rest, rx) {
    if (!entry) { printf "%d\t%s sits before the first entry heading\n", NR, name; return }
    if (seen[name]) { printf "%d\t%s appears twice in one entry\n", NR, name; return }
    seen[name] = 1
    if (rest !~ rx) printf "%d\t%s is not a comma-separated list of the right names\n", NR, name
  }
  /^```/ { fence = !fence; next }
  fence { next }
  /^## / { entry = 0; next }
  /^### / { entry = 1; split("", seen); next }
  index($0, "**Forge:**") == 1 {
    judge("Forge", substr($0, 11), "^ forge-[0-9]+(, forge-[0-9]+)*$"); next }
  index($0, "**Closed by:**") == 1 {
    judge("Closed by", substr($0, 15), "^ #[0-9]+(, #[0-9]+)*$"); next }
  ' "$log")

if [ -n "$shape" ]; then
  printf 'check-harness-gaps-forge: a closing field has the wrong shape or place.\n\n' >&2
  while IFS=$'\t' read -r n why; do
    printf '  %s:%s  %s\n' "$log" "$n" "$why" >&2
  done <<< "$shape"
  cat >&2 <<'MSG'

  Forge:     forge-<N>[, forge-<M>]   the forge that took the entry
  Closed by: #<N>[, #<M>]             the merged PR that fixed it outside a forge

MSG
  findings=1
fi

# --- 2: provenance, over the lines this branch added or removed -------------
base=$(git merge-base origin/main HEAD 2>/dev/null || git merge-base main HEAD 2>/dev/null)
if [ -z "$base" ]; then
  printf 'check-harness-gaps-forge: no merge base with main; provenance not checked\n' >&2
else
  changed=$(git diff -U0 "$base"...HEAD -- "$log" 2>/dev/null |
    grep -E '^[+-]\*\*(Forge|Closed by):\*\*')
  if [ -n "$changed" ]; then
    branch=$(git branch --show-current 2>/dev/null)
    own=
    case "$branch" in
      harness/forge-*) own=${branch#harness/forge-} ;;
    esac
    if ! [[ $own =~ ^[0-9]+$ ]]; then
      printf 'check-harness-gaps-forge: %s is not a forge branch, and it changes closing fields:\n\n' \
        "${branch:-a detached HEAD}" >&2
      printf '%s\n' "$changed" | sed 's/^/  /' >&2
      cat >&2 <<'MSG'

A Forge line records that a forge patch took the entry; a Closed by line
records that a forge verified a fix which landed through another pull request.
Only a harness/forge-<N> branch adds, changes or removes either. A flow that
parks a diagnosis, or fixes one, leaves both fields out, and the entry stays
open until a forge records it. Delete the line from this branch.
MSG
      findings=1
    else
      over=$(printf '%s\n' "$changed" | grep -E '^\+\*\*Forge:\*\*' | grep -oE 'forge-[0-9]+' |
        while read -r f; do
          [ "$((10#${f#forge-}))" -gt "$((10#$own))" ] && printf '%s\n' "$f"
        done)
      if [ -n "$over" ]; then
        printf 'check-harness-gaps-forge: %s adds a Forge value above its own number:\n\n' "$branch" >&2
        printf '%s\n' "$over" | sed 's/^/  /' >&2
        findings=1
      fi
    fi
  fi
fi

exit "$findings"
