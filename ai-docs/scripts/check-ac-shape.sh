#!/usr/bin/env bash
# An acceptance criterion states a condition; it does not prescribe a test.
#
# WHAT THIS REFUSES. A `## Acceptance Criteria` row containing a test
# prescription — "A test asserts …", "A test covers …", "Tests drive …". The
# criterion stays; the sentence naming the test moves to the design's
# `## Test Design`, which both recent designs already carry and which
# `self-review` already reads.
#
# WHY IT IS A GATE AND NOT A SENTENCE. `spec-writer.md` Rule 9/PROC-3 has said
# "an AC is DECLARATIVE … it states a condition over the tree" since before any
# of this. Measured across the four merged specs in ai-docs/plans/done/ as of
# 8fae04a: 1 of 19 rows carried a test prescription (2026-09-02), then 0 of 17,
# then 10 of 33, then 12 of 36 — 5% to 33% in three runs.
#
# WHAT IT COSTS TO LEAVE IT. When the AC names the test, the test's EXISTENCE
# becomes the criterion, and existence is exactly what a cosmetic test
# satisfies. ai-docs/learnings.md 2026-09-04 records five such tests on one run
# — each passing on the shipped code AND on the mutant, each named as an AC's
# verifier, one of them written in an earlier fix round for that very property.
# That entry escalated nowhere; this is where it escalates.
#
# WHY A SUBSTRING GATE IS SOUND HERE, given spec-writer.md's own box rejecting
# lexical gates for semantic distinctions: the box's bar is a phrase that occurs
# essentially only in its forbidden sense. Measured over every spec in
# ai-docs/plans/done/ at 8fae04a — 142 AC rows: 24 hits, every one a
# prescription, zero false positives. The lower-case alternative was measured
# separately before it was added: it contributes exactly one row (transport
# AC31), and that row is a prescription too.
#
# Usage:
#   check-ac-shape.sh                 check the AC rows this branch added or
#                                     changed, against the merge base with main
#   check-ac-shape.sh <file>...       audit the named specs in full
#
# Exit 0 = no prescription in any checked AC table (or nothing to check).
# Exit 1 = at least one row prescribes a test.

set -uo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-ac-shape: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

# One shape, deliberately narrow: a sentence that starts by naming a test.
prescription='(^|[.;] )([Aa] test |[Tt]ests? (assert|cover|driv|exercis))'

# The matcher reads `<file><TAB><AC row>` on stdin and prints `<file><TAB><id>`
# for each offending row, so both modes below share one regex and one parser.
# It splits on the FIRST tab only: a markdown row may contain tabs of its own,
# and -F'\t' would silently drop such a row out of the check.
match_rows() {
  awk -v rx="$prescription" '
    {
      t = index($0, "\t")
      if (t == 0) next
      file = substr($0, 1, t - 1)
      row  = substr($0, t + 1)
      split(row, c, "|")
      id = c[2]; gsub(/[[:space:]]/, "", id)
      if (id !~ /^AC[0-9]+$/) next
      # Match the CRITERION CELL, not the line: a prescription that opens the
      # cell has "| " in front of it, which is neither a start-of-string nor a
      # sentence break, and a line-anchored pattern silently misses it.
      cell = c[3]; gsub(/^[[:space:]]+/, "", cell)
      if (cell ~ rx) printf "%s\t%s\n", file, id
    }'
}

if [ $# -gt 0 ]; then
  # Explicit files: audit their whole Acceptance Criteria table.
  hits=$(
    for f in "$@"; do
      [ -r "$f" ] || continue
      awk -v file="$f" '
        /^## Acceptance Criteria/ { in_ac = 1; next }
        /^## / { in_ac = 0 }
        in_ac && /^\|/ { printf "%s\t%s\n", file, $0 }
      ' "$f"
    done | match_rows
  )
else
  # Default: only the rows this branch ADDED or CHANGED.
  #
  # Scanning whole files would re-litigate every AC written before the gate
  # existed, and it would fire on a PR that edits an old done/ spec for an
  # unrelated propagation reason — a routine shape here, since AGENTS.md
  # § Propagation Rule makes such edits mandatory. A row nobody touched is not
  # this branch's to answer for.
  base=$(git merge-base origin/main HEAD 2>/dev/null || git merge-base main HEAD 2>/dev/null)
  if [ -z "$base" ]; then
    printf 'check-ac-shape: no merge base with main; skipped\n' >&2
    exit 0
  fi
  hits=$(git diff -U0 "$base"...HEAD -- '*.spec.md' 2>/dev/null | awk '
    /^\+\+\+ b\// { file = substr($0, 7); next }
    /^\+/ && file != "" { printf "%s\t%s\n", file, substr($0, 2) }
  ' | match_rows)
fi

failures=0
if [ -n "$hits" ]; then
  printf 'check-ac-shape: an acceptance criterion prescribes a test.\n\n' >&2
  printf '%s\n' "$hits" | while IFS=$'\t' read -r file id; do
    [ -n "$file" ] && printf '  %s  %s\n' "$file" "$id" >&2
  done
  failures=1
fi

if [ "$failures" -gt 0 ]; then
  cat >&2 <<'MSG'

Keep the condition in the AC; move the sentence naming the test to the design's
`## Test Design` section. The criterion is what must be true of the system, and
the verifier writes its own command into the progress file's `verifying command`
column — spec-writer.md Rule 9/PROC-3.

When the AC names the test, the test's EXISTENCE becomes the criterion, and
existence is what a cosmetic test satisfies (ai-docs/learnings.md 2026-09-04:
five AC-verifying tests on one run, each green on the shipped code and green on
the mutant).
MSG
  exit 1
fi
exit 0
