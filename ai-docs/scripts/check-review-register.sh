#!/usr/bin/env bash
# The review register and the round tables must agree about what is fixed.
#
# WHAT THIS REFUSES. A `## Self-Review (Round N)` table row marked `✅ Fixed`
# whose matching `## Review register` row still reads `open` — bare, or with the
# re-opening decoration `open 🔁@<round>` — or has no register row at all. The
# two places record the same fact with no link between them, and the fixer
# updates the round table because that is the table it is reading.
#
# WHY IT IS A GATE. The same finding was raised in FIVE consecutive rounds of
# one run: "N rows still read open although round N-1's own table marks them
# ✅ Fixed". Each round fixed the instance by hand and the next round found the
# next one. Five recurrences of a rule that exists as text — one more than the
# four this project had previously measured as the point where a disposition is
# proven not to hold.
#
# THE JOIN KEY is the register id: letters, then `<round>-<n>` — `R<N>-<n>`,
# and `SR<N>-<n>` joins the same way. A round-table row joins the id its Finding
# cell leads with, when that cell starts with one: a re-opened finding keeps its
# original id, so a later round's row may lead with an earlier round's id.
# Otherwise the row joins `R<N>-<n>`, its own round and row number. A register
# id of any other shape is skipped; when a Fixed row then finds no register row,
# the message names the ids that could not be read, the likelier cause.
#
# Exit 0 = every ✅ Fixed row has a register row that agrees (or there is no
#          register to check).
# Exit 1 = at least one disagreement.
# Exit 2 = no progress file named. A call with nothing to check is a mistake,
#          never a pass.

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

[ $# -gt 0 ] || { printf 'check-review-register: no progress file named; usage: %s <progress-file>...\n' "$0" >&2; exit 2; }

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
      id = c[2]; gsub(/[[:space:]`]/, "", id)
      if (id !~ /^[A-Za-z]*[0-9]+-[0-9]+$/) {
        if (id != "" && id !~ /^-+$/ && tolower(id) != "id") unreadable[id] = 1
        next
      }
      st = c[5]; gsub(/^[[:space:]]+|[[:space:]]+$/, "", st)
      regstatus[id] = st
      next
    }

    # --- a round table row marked fixed --------------------------------------
    section == "round" && /^\|[[:space:]]*[0-9]+[[:space:]]*\|/ {
      nc = split($0, c, "|")
      n = c[2] + 0
      # A row trailing pipe is optional in GFM, so the Status cell is the last
      # non-blank cell, not a fixed index.
      status = c[nc]
      for (i = nc; i >= 1; i--) { if (c[i] ~ /[^[:space:]]/) { status = c[i]; break } }
      if (index(status, "Fixed") == 0) next
      fixed[round "-" n] = 1
      # An id leading the Finding cell names the register row this one joins.
      named = c[5]; sub(/^[[:space:]`*]+/, "", named)
      if (match(named, /^[A-Za-z]+[0-9]+-[0-9]+/) && substr(named, RLENGTH + 1, 1) !~ /[A-Za-z0-9-]/) {
        want[round "-" n] = substr(named, 1, RLENGTH)
      }
      next
    }

    END {
      list = ""
      for (u in unreadable) list = list (list == "" ? "" : ", ") u
      for (k in fixed) {
        split(k, p, "-"); r = p[1]; n = p[2]
        found = ""
        if (k in want) {
          if (!(want[k] in regstatus)) {
            printf "%s\tround %s finding %s names register id %s, and the register has no row with that id\n", file, r, n, want[k]
            continue
          }
          found = want[k]
        } else {
          for (id in regstatus) {
            if (id ~ ("^[A-Za-z]*" r "-" n "$")) { found = id; break }
          }
        }
        if (found == "") {
          msg = sprintf("%s\tround %s finding %s is marked Fixed and has no register row", file, r, n)
          if (list != "") msg = msg sprintf(" (the register carries ids this check cannot parse: %s; an id is letters, then <round>-<n>)", list)
          print msg
          continue
        }
        s = regstatus[found]
        if (s ~ /^open([^A-Za-z0-9_]|$)/) {
          printf "%s\t%s reads \"%s\" while round %s marks finding %s Fixed\n", file, found, s, r, n
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

A re-opened finding keeps its id: lead its round-table Finding cell with that id
(`R1-5 — ...`), and the row joins R1-5 instead of its own round and row number.
MSG
  exit 1
fi
exit 0
