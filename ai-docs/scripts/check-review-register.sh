#!/usr/bin/env bash
# The review register and the round tables must agree about what is fixed.
#
# WHAT THIS REFUSES. A `## Self-Review (Round N)` table row marked `✅ Fixed`
# whose matching `## Review register` row still reads `open` — or has no
# register row at all. The two places record the same fact with no link between
# them, and the fixer updates the round table because that is the table it is
# reading.
#
# WHY IT IS A GATE. The same finding was raised in FIVE consecutive rounds of
# one run: "N rows still read open although round N-1's own table marks them
# ✅ Fixed". Each round fixed the instance by hand and the next round found the
# next one. Five recurrences of a rule that exists as text — one more than the
# four this project had previously measured as the point where a disposition is
# proven not to hold.
#
# THE JOIN KEY is the register id: a self-review row for finding `n` of round
# `N` is `R<N>-<n>` (`SR<N>-<n>` is also accepted; the leading letters are
# free). Rows whose id does not parse are SKIPPED, not failed — design-review
# ids and free-form ids are not this check's business.
#
# Exit 0 = every ✅ Fixed row has a register row that agrees (or there is no
#          register to check).
# Exit 1 = at least one disagreement.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  check-review-register.sh <progress-file>...
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

[ $# -gt 0 ] || { printf 'check-review-register: usage: %s <progress-file>...\n' "$0" >&2; exit 0; }

failures=0

for pf in "$@"; do
  [ -r "$pf" ] || continue
  grep -q '^## Review register' "$pf" || continue

  out=$(awk -v file="$pf" '
    # --- the register: id -> status -----------------------------------------
    /^## Review register/ { section = "reg"; next }
    /^## Self-Review \(Round [0-9]+\)/ {
      section = "round"
      match($0, /Round [0-9]+/); round = substr($0, RSTART + 6, RLENGTH - 6) + 0
      next
    }
    /^## / { section = "" ; next }

    section == "reg" && /^\|/ {
      split($0, c, "|")
      id = c[2]; gsub(/[[:space:]]/, "", id)
      if (id !~ /^[A-Za-z]*[0-9]+-[0-9]+$/) next
      st = c[5]; gsub(/^[[:space:]]+|[[:space:]]+$/, "", st)
      regstatus[id] = st
      next
    }

    # --- a round table row marked fixed --------------------------------------
    section == "round" && /^\|[[:space:]]*[0-9]+[[:space:]]*\|/ {
      split($0, c, "|")
      n = c[2] + 0
      status = c[6]
      # A row trailing pipe is optional in GFM, so the Status cell is the last
      # non-blank cell, not a fixed index.
      last = c[length(c)]
      for (i = length(c); i >= 1; i--) { if (c[i] ~ /[^[:space:]]/) { last = c[i]; break } }
      status = last
      if (index(status, "Fixed") == 0) next
      fixed[round "-" n] = 1
      next
    }

    END {
      for (k in fixed) {
        split(k, p, "-"); r = p[1]; n = p[2]
        found = ""
        for (id in regstatus) {
          if (id ~ ("^[A-Za-z]*" r "-" n "$")) { found = id; break }
        }
        if (found == "") {
          printf "%s\tround %s finding %s is marked Fixed and has no register row\n", file, r, n
          continue
        }
        s = regstatus[found]
        if (s ~ /^open$/ || s ~ /^[[:space:]]*open[[:space:]]*$/) {
          printf "%s\t%s reads \"open\" while round %s marks finding %s Fixed\n", file, found, r, n
        }
      }
    }
  ' "$pf")

  if [ -n "$out" ]; then
    if [ "$failures" -eq 0 ]; then
      printf 'check-review-register: the register and a round table disagree.\n\n' >&2
    fi
    printf '%s\n' "$out" | sed 's/\t/  /' | sed 's/^/  /' >&2
    failures=$((failures + 1))
  fi
done

if [ "$failures" -gt 0 ]; then
  cat >&2 <<'MSG'

The register is the loop's durable cross-round state — the next round reads it,
not the round tables (.claude/agents/self-review.md instruction 7a). A row left
at `open` is invisible to that round, which then re-raises it as a new finding.

Stamp each fixed row in the register with the commit that fixed it:
  | <id> | round N | <sev> | fixed@<sha> | <verifying command> |
MSG
  exit 1
fi
exit 0
