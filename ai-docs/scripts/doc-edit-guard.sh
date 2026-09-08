#!/usr/bin/env bash
# Snapshot-and-verify around a scripted edit of a durable markdown artefact
# (a spec or a design document), so that an edit which truncates the file is
# undone in the same command instead of being discovered rounds later.
#
# WHY. A heading-anchored slice edit (`s.index("## Open questions")`,
# `s.rindex('\n**Revision:**')` ...) finds the FIRST occurrence of its anchor,
# which is frequently an in-text mention of the heading rather than the heading
# itself; everything after the true heading is then dropped. It has happened to
# a design document (ai-docs/learnings.md 2026-09-02) and to a spec
# (ai-docs/learnings.md 2026-09-08), both untracked at the time, both rebuilt
# from the delegate's context. A rule that says "anchor with a newline" did not
# hold across those two runs; this guard does not depend on the rule holding.
#
# Usage:
#   bash ai-docs/scripts/doc-edit-guard.sh snapshot <file>
#   ... the scripted edit ...
#   bash ai-docs/scripts/doc-edit-guard.sh verify <file>
#
# snapshot: copies <file> to <file>.bak and records the shape counts.
# verify:   recounts. If any count SHRANK, restores <file> from <file>.bak,
#           prints what shrank, exits 2. Otherwise removes <file>.bak, exits 0.
#           A missing <file>.bak is a defect on its own (the snapshot step was
#           skipped): exits 2 without touching <file>.
#
# The shape counts are the structural markers a truncation removes and an edit
# never legitimately removes in bulk: `## ` section headings, `| AC` acceptance
# rows, `| D` / `| KD` decision rows, and the closing fence of the last code
# block. Growth is always fine; shrinkage of any one marker is the signal.
#
# Exit 0 = shape held. Exit 2 = shape shrank (file restored) or snapshot
# missing. Exit 1 = usage error.

set -uo pipefail

usage() { sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'; exit 1; }

[ $# -eq 2 ] || usage
mode=$1; file=$2
[ -f "$file" ] || { printf 'doc-edit-guard: no such file: %s\n' "$file" >&2; exit 1; }
bak="$file.bak"

shape() {
  # one line per marker: name<TAB>count
  printf 'sections\t%s\n' "$(grep -c '^## ' "$1")"
  printf 'ac-rows\t%s\n'  "$(grep -cE '^\| *AC[0-9]+' "$1")"
  printf 'decision-rows\t%s\n' "$(grep -cE '^\| *(KD|D)-?[0-9]+' "$1")"
  printf 'fences\t%s\n' "$(grep -c '^```' "$1")"
}

case "$mode" in
  snapshot)
    cp -- "$file" "$bak" || exit 1
    shape "$file" | sed 's/^/doc-edit-guard: snapshot /'
    exit 0
    ;;
  verify)
    if [ ! -f "$bak" ]; then
      printf 'doc-edit-guard: no snapshot %s -- run the snapshot step BEFORE the edit; nothing restored\n' "$bak" >&2
      exit 2
    fi
    shrank=0
    while IFS=$'\t' read -r name before; do
      after=$(shape "$file" | awk -F'\t' -v n="$name" '$1==n{print $2}')
      if [ "${after:-0}" -lt "$before" ]; then
        printf 'doc-edit-guard: %s shrank %s -> %s\n' "$name" "$before" "$after" >&2
        shrank=1
      fi
    done < <(shape "$bak")
    if [ "$shrank" -eq 1 ]; then
      mv -- "$bak" "$file"
      printf 'doc-edit-guard: RESTORED %s from snapshot; the edit truncated the document (anchor matched an in-text mention?). Re-anchor on "\\n## <heading>\\n" and retry.\n' "$file" >&2
      exit 2
    fi
    rm -f -- "$bak"
    echo "doc-edit-guard: shape held"
    exit 0
    ;;
  *) usage ;;
esac
