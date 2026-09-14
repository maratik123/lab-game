#!/usr/bin/env bash
# Every open entry of the harness-gaps log routes its fix to files that exist.
#
# WHY. The target line is where a fix is routed, and it is written once, when
# the gap is diagnosed. A file renamed or moved afterwards leaves the pointer
# dead; the reader who follows it opens nothing and may conclude the surface is
# gone.
#
# WHAT THIS REFUSES (exit 1). On an entry with neither closing field and no
# supersession field: a backticked token on its target line that reads as a
# repository path, and matches nothing on disk. A token reads as a path when it
# has a slash, ends in a file extension this repository uses, or is a dotfile.
# It is read up to its first space, a trailing line locator is dropped, one
# level of braces is expanded, and a glob must match at least one path.
#
# NOT REFUSED. A closed entry, whose route is history; a superseded one, since a
# supersession field is how an append-only log corrects a dead pointer and the
# later entry carries the live one; a path outside the tree, home-relative or
# absolute; a placeholder; anything inside a fenced block.
#
# Exit 0 = every such token resolves. Exit 1 = at least one does not.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  check-harness-gaps-targets.sh    check that each open, unsuperseded entry of
                                   the harness-gaps log names existing files on
                                   its target line; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-harness-gaps-targets: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

log=ai-docs/harness-gaps.md
if [ ! -f "$log" ]; then
  printf 'check-harness-gaps-targets: %s absent; skipped\n' "$log" >&2
  exit 0
fi

# One line per open, unsuperseded entry that has a target line: its heading, a
# tab, the target line. A fenced block holds the entry skeleton; it is skipped.
entries=$(awk '
  function flush() {
    if (head != "" && !closed && target != "") print head "\t" target
    head = ""; target = ""; closed = 0
  }
  /^```/ { fence = !fence; next }
  fence { next }
  /^### 20/ { flush(); head = $0; next }
  head != "" && /^\*\*target:\*\*/ { target = $0; next }
  head != "" && /^\*\*(Forge|Closed by|Superseded by):\*\*/ { closed = 1; next }
  END { flush() }
' "$log")

# expand <token>: one level of braces, one path per line.
expand() {
  local pre body post item
  local -a items
  if [[ $1 =~ ^([^{]*)\{([^}]*)\}(.*)$ ]]; then
    pre=${BASH_REMATCH[1]} body=${BASH_REMATCH[2]} post=${BASH_REMATCH[3]}
    IFS=, read -r -a items <<< "$body"
    for item in "${items[@]}"; do printf '%s\n' "$pre$item$post"; done
  else
    printf '%s\n' "$1"
  fi
}

# resolves <path>: a glob must match something; a plain path must exist.
resolves() {
  case "$1" in
    *'*'*|*'?'*|*'['*) compgen -G "$1" >/dev/null ;;
    *) [ -e "$1" ] ;;
  esac
}

# The backtick, spelled so that no quoting of it reads as a command substitution.
bt=$(printf '\140')

findings=0
while IFS=$'\t' read -r head target; do
  [ -n "$head" ] || continue
  while IFS= read -r token; do
    word=${token%% *}
    word=${word%%:[0-9]*}
    case "$word" in
      ''|'~'*|/*|*'<'*|*'>'*|*'$'*) continue ;;
    esac
    case "$word" in
      */*|*.md|*.sh|*.json|*.yml|*.yaml|*.go|*.sql|.[A-Za-z]*) ;;
      *) continue ;;
    esac
    while IFS= read -r path; do
      resolves "$path" && continue
      if [ "$findings" -eq 0 ]; then
        printf 'check-harness-gaps-targets: an open entry routes its fix to a file that does not exist.\n\n' >&2
      fi
      printf '  %s\n    target: %s\n' "$head" "$path" >&2
      findings=$((findings + 1))
    done < <(expand "$word")
  done < <(printf '%s\n' "$target" | grep -oE "${bt}[^${bt}]+${bt}" | tr -d "$bt")
done <<< "$entries"

if [ "$findings" -gt 0 ]; then
  cat >&2 <<'MSG'

The log is append-only, so a target line is never edited. Correct a moved file
with a new entry that names the live path and a supersession field on the old
one; a forge that takes the entry closes it instead.
MSG
  exit 1
fi
exit 0
