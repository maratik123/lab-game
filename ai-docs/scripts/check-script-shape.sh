#!/usr/bin/env bash
# One help-flag shape for every script, and one place a shebang may live.
#
# WHAT THIS REFUSES, over the tracked tree:
#   1. A script that dispatches on the help flag in any shape but the fixed
#      one. Two spellings of one convention is how the convention becomes a
#      suggestion, and the drift is invisible until someone reads both files
#      side by side.
#   2. A help dispatch that something can run before. The whole property is
#      that nothing happens first: no measurement taken, no subprocess spawned,
#      no file written. A constant assignment and a function definition are not
#      side effects and may precede it; a command substitution is one and may
#      not.
#   3. A help dispatch that exits non-zero, or prints nothing. Both leave the
#      caller worse off than the comment the flag replaced.
#   4. A tracked file that begins with a shebang, is not named with the shell
#      extension, and is not a symbolic link to one that is. Such a file is
#      invisible to every tool here that finds scripts by name — the lint
#      target, the CI step, and the reference gate's own router.
#
# A script that answers no help flag is not this checker's business. Nothing
# here says a script must have one.
#
# Exit 0 = every tracked script conforms. Exit 1 = at least one does not.

set -uo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-script-shape: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

# The shape, as one string. Every dispatch line in the tree must equal it byte
# for byte, so it is compared, never matched. The failure message prints this
# same variable, so the advice cannot drift from the assertion.
marker='  -h|--help) usage; exit 0 ;;'

# A dispatch is a case arm on the flag, or a conditional naming the long form.
# A flag inside a fixture payload or inside a pattern is neither, which is why
# the arm is anchored at the start of its line. The short form alone is not
# enough for the conditional branch: a file test spells one the same way.
dispatch_rx='^[[:space:]]*(-h|--help)(\|(-h|--help))?\)|^[[:space:]]*(el)?if[[:space:]].*--help'

findings=0
report() { printf '  %s\n' "$*" >&2; findings=$((findings + 1)); }

# --- 1..3: the help dispatch -------------------------------------------------
while IFS= read -r f; do
  [ -f "$f" ] || continue
  [ -L "$f" ] && continue
  grep -qE "$dispatch_rx" "$f" || continue

  # (1) there is exactly one dispatch line and it is the shape.
  n_shape=$(grep -cFx -- "$marker" "$f")
  n_dispatch=$(grep -cE "$dispatch_rx" "$f")
  if [ "$n_shape" -ne 1 ] || [ "$n_dispatch" -ne 1 ]; then
    report "$f dispatches on the help flag in a shape that is not the fixed one"
    grep -nE "$dispatch_rx" "$f" | sed 's/^/      /' >&2
    continue
  fi

  # (2) nothing runs before it. Walked rather than grepped: the question is
  # what precedes the dispatch, and only a scan answers that.
  before=$(awk -v marker="$marker" '
    NR == 1 && /^#!/ { next }
    $0 == marker { exit }
    /^[[:space:]]*(#|$)/ { next }
    infunc {
      if ($0 ~ /^\}/) infunc = 0
      next
    }
    /^[A-Za-z_][A-Za-z0-9_-]*\(\)[[:space:]]*\{.*\}[[:space:]]*$/ { next }
    /^[A-Za-z_][A-Za-z0-9_-]*\(\)[[:space:]]*\{[[:space:]]*$/ { infunc = 1; next }
    /^set[[:space:]]/ { next }
    /^(case|esac)([[:space:]]|$)/ { next }
    /^[A-Za-z_][A-Za-z0-9_]*=/ && !/\$\(/ && !/`/ { next }
    { printf "%d: %s\n", NR, $0; exit }
  ' "$f")
  if [ -n "$before" ]; then
    report "$f runs something before it answers the help flag"
    printf '      %s\n' "$before" >&2
  fi

  # (3) it exits 0 and prints something. Safe to run precisely because (2) is
  # asserted for every script that reaches here.
  out=$(bash "$f" --help 2>/dev/null)
  rc=$?
  [ "$rc" -eq 0 ] || report "$f answers the help flag with exit $rc"
  [ -n "$out" ] || report "$f answers the help flag with an empty block"
done < <(git ls-files '*.sh')

# --- 4: where a shebang may live ---------------------------------------------
# A symbolic link is read through, so a link to a script reads as a script; the
# link's own name is what the extension rule is about.
while IFS= read -r f; do
  [ -e "$f" ] || continue
  [ "$(head -c2 -- "$f" 2>/dev/null)" = '#!' ] || continue
  case "$f" in *.sh) continue ;; esac
  if [ -L "$f" ]; then
    case "$(readlink -- "$f")" in *.sh) continue ;; esac
  fi
  report "$f begins with a shebang but is neither named with the shell extension nor a link to one that is"
done < <(git ls-files)

if [ "$findings" -gt 0 ]; then
  printf 'check-script-shape: %d finding(s).\n\n' "$findings" >&2
  printf 'The help flag is answered by one block, copied out of a script that\n' >&2
  printf 'already has it rather than written again:\n\n' >&2
  printf '  usage() {\n    cat <<%sUSAGE%s\n  Usage:\n' "'" "'" >&2
  printf '    <the invocation grammar, one line per form>\n  USAGE\n  }\n\n' >&2
  # shellcheck disable=SC2016  # the brace form is literal text in the advice
  printf '  case "${1:-}" in\n%s\n  esac\n\n' "$marker" >&2
  printf 'Nothing may precede it but comments, a set line, constant assignments\n' >&2
  printf 'and function definitions. A file beginning with a shebang is named with\n' >&2
  printf 'the shell extension, or is a symbolic link to one that is.\n' >&2
  exit 1
fi
echo "script shape: every tracked script conforms"
exit 0
