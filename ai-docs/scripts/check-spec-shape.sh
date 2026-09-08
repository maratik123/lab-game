#!/usr/bin/env bash
# A spec stores how a thing is located and how it is measured -- never a bare
# line number and never a tally.
#
# WHAT THIS REFUSES, in a `*.spec.md`:
#   1. a bare `path:line` reference -- `ai-docs/doc-convention.md:41`,
#      `internal/tg/limit.go:190` -- with no 7-hex commit in front of it. A line
#      number is invalidated by any edit above it, and a spec is read on a
#      different tree than it was written on. The pinned form
#      `[source: <commit>:<file> § <section-or-symbol> · <command>]` passes; so
#      does a `<commit>:<path>:<line>` coordinate or a module-version one
#      (`telego@v1.11.2/methods.go:28`), which at least name the tree they
#      were taken at.
#   2. a counting command inside a `[source: ... · <command>]` annotation --
#      `wc -l`, `grep -c`, `uniq -c`, `sort -n` -- because a command that counts
#      exists to justify a stored tally, and the tally is what must not be stored.
#   3. a `### Sizing` heading, the shape the stored-tally section took when it
#      was measured (ai-docs/learnings.md 2026-09-08).
#
# EXEMPTION, visible on the line: a spec whose subject is the reference ban has
# to show a banned form as an example. A line carrying the HTML comment
# `<!-- spec-shape: example -->` is skipped. It is reviewed like a `//nolint`
# reason -- the marker is in the diff, and an example row is the only shape
# that earns it.
#
# WHY IT IS A GATE AND NOT A SENTENCE. spec-writer.md Rule 8 and the
# `## Source conflicts` template contradicted each other for the whole life of
# the file (one required a commit pin, the other prescribed `file:line`), and
# the delegate obeyed the template. The project convention -- a durable
# reference names a symbol or carries its commit -- lived in ai-docs/learnings.md
# with `Escalated? no`, binding nothing. The owner rejected a spec on sight for
# exactly this form on 2026-09-08. A rule that says "cite properly" has been
# written three times; this file is the fourth time and the first that runs.
#
# WHAT A DIGIT GREP CANNOT SEE, stated so nobody records a pass it never
# earned: a tally spelled in words ("thirteen of the sixteen") passes here.
# That residue is the reviewer's, and Rule 8 says so.
#
# Usage:
#   check-spec-shape.sh                 check the lines this branch added or
#                                       changed in any *.spec.md, against the
#                                       merge base with main
#   check-spec-shape.sh <file>...       audit the named specs in full
#
# Exit 0 = clean (or nothing to check). Exit 1 = at least one hit.

set -uo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-spec-shape: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

# A path with a source-ish extension, a colon, digits -- NOT preceded by
# `<7-hex>:` (the pinned form) and not part of a URL or a time.
bare_ref='(^|[^A-Za-z0-9_/.:-])[A-Za-z0-9_.][A-Za-z0-9_./-]*\.(md|go|sh|sql|yml|yaml|json|toml|mod)(:[0-9]+)(-[0-9]+)?([^0-9A-Za-z]|$)'
# Two pins: a commit (`f5b236a:path:line`) and a module version
# (`telego@v1.11.2/methods.go:28-35`) -- both name the tree they were read on.
pinned='([0-9a-f]{7,40}:[A-Za-z0-9_.][A-Za-z0-9_./-]*|[A-Za-z0-9_./-]+@v?[0-9][A-Za-z0-9_.+-]*/[A-Za-z0-9_./-]*)\.(md|go|sh|sql|yml|yaml|json|toml|mod):[0-9]+'
# Deliberately `.*`, not `[^]]*`: an annotation's command routinely contains
# `]` (`[[:space:]]`, `AC[0-9]`), and a class that stops at the first one
# reported three placeholders where there were eight (2026-09-08 run).
counting='\[source:.*(wc -l|wc -c|grep -c|grep -[A-Za-z]*c[A-Za-z]* |uniq -c|sort -n)'
sizing='^#{2,4} *Sizing'

# Input: `<file><TAB><line>` rows. Output: `<file><TAB><kind><TAB><line>` hits.
match_lines() {
  # Patterns travel through the environment: `awk -v` re-processes backslash
  # escapes and would turn `\[source:` into a bracket expression.
  bare="$bare_ref" pin="$pinned" cnt="$counting" siz="$sizing" awk '
    BEGIN { bare = ENVIRON["bare"]; pin = ENVIRON["pin"]; cnt = ENVIRON["cnt"]; siz = ENVIRON["siz"] }
    {
      t = index($0, "\t")
      if (t == 0) next
      file = substr($0, 1, t - 1)
      line = substr($0, t + 1)
      if (line ~ /^[[:space:]]*```/) { fence = !fence }   # skip fenced code blocks
      if (fence) next
      # A spec ABOUT references must be allowed to show one. The exemption is a
      # visible marker on the line, reviewed like a //nolint reason.
      if (line ~ /spec-shape: example/) next
      if (line ~ siz) { printf "%s\tsizing\t%s\n", file, line; next }
      if (line ~ cnt) { printf "%s\tcounting\t%s\n", file, line; next }
      if (line ~ bare) {
        stripped = line
        gsub(pin, "", stripped)             # drop every pinned coordinate first
        if (stripped ~ bare) printf "%s\tbare-ref\t%s\n", file, line
      }
    }'
}

if [ $# -gt 0 ]; then
  hits=$(
    for f in "$@"; do
      [ -r "$f" ] || continue
      awk -v file="$f" '{ printf "%s\t%s\n", file, $0 }' "$f"
    done | match_lines
  )
else
  base=$(git merge-base origin/main HEAD 2>/dev/null || git merge-base main HEAD 2>/dev/null)
  if [ -z "$base" ]; then
    printf 'check-spec-shape: no merge base with main; skipped\n' >&2
    exit 0
  fi
  hits=$(git diff -U0 "$base"...HEAD -- '*.spec.md' 2>/dev/null | awk '
    /^\+\+\+ b\// { file = substr($0, 7); next }
    /^\+/ && file != "" { printf "%s\t%s\n", file, substr($0, 2) }
  ' | match_lines)
fi

if [ -z "$hits" ]; then
  exit 0
fi

printf 'check-spec-shape: a spec stores a bare line reference or a tally.\n\n' >&2
printf '%s\n' "$hits" | while IFS=$'\t' read -r file kind line; do
  [ -n "$file" ] && printf '  %s  [%s]  %s\n' "$file" "$kind" "${line:0:110}" >&2
done
cat >&2 <<'MSG'

bare-ref  -> locate by section heading or symbol, pinned to a commit:
             [source: <commit>:<file> § <section-or-symbol> · <command>]
counting  -> drop the tally and the command that produced it; state the
             measurement instruction declaratively (class, glob, exclusions)
sizing    -> delete the section; a spec carries no measured figures at all

spec-writer.md Rule 8 (owner's ruling of 2026-09-08). Numerals spelled in words
are not caught here; they are still a defect.
MSG
exit 1
