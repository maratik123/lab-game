#!/usr/bin/env bash
# The coverage ratchet: statement coverage may rise freely and may not fall.
#
# ONE IMPLEMENTATION, THREE CALLERS. `.githooks/pre-commit` runs it in raise
# mode, `make cover-ratchet` and CI run it with --check. A second spelling of
# the measurement would let a local run and a CI run disagree about what the
# number is, which is the whole reason the Makefile exists (KD-16).
#
# WHAT IS MEASURED. `go test -coverprofile ./...` over every package, statement
# mode, each package covered by ITS OWN tests — the Go default. Cross-package
# coverage (`-coverpkg=./...`) is deliberately not used: it is slower, and it
# makes a package's number depend on which other packages' tests ran.
#
# ROUNDING. Half-up to hundredths of a percentage point, computed as
# int(p*100 + 0.5)/100 rather than printf "%.2f" — printf rounds half-to-even
# in most awks, and a ratchet should not depend on which awk is installed.
# Float arithmetic throughout; the tolerance absorbs the epsilon.
#
# THE TOLERANCE is not zero because the suite's coverage drifts between runs on
# its own: a few error paths in internal/scheduler and internal/ingest are
# reached only when a deadline or a cancellation lands inside a database
# round-trip. Today that is 6 statements, 0.34 pp of 1757.
#
# To re-derive the drifting set instead of trusting this sentence:
#   for i in 1 2 3; do
#     go test -count=1 -covermode=atomic -coverprofile="tmp/c$i.out" ./...
#   done
#   # blocks with a non-zero count in some profiles and zero in others = the set
#
# Change TOLERANCE_PP only from such a series, and only if it shows the drift has
# GROWN past the constant — never from a single blocked commit. `-count=1` is not
# optional there: without it the second run of a series replays the first from
# the test cache, and the series measures one run. Re-measure in BOTH a local and
# a CI environment. Why the number is 0.60 and what a mark recorded from a cached
# draw once cost: `git log -p` on this file, 2026-09-07.
#
# THE MEASUREMENT BELOW KEEPS THE TEST CACHE — owner's decision, 2026-09-07. A
# replayed draw sits at most one drift above the floor, which the tolerance
# covers with room. If the drift ever grows toward the tolerance, the answer is
# `-count=1` on that command, not a wider tolerance.
#
# WHAT THE TOLERANCE COSTS, bounded: the recorded value never decreases, so the
# total that can be lost silently is one tolerance below the all-time high —
# ONCE, not per commit.
#
# Usage:
#   coverage-ratchet.sh            raise mode: check, and record a new high
#   coverage-ratchet.sh --check    check only: never writes, never stages
#
# Exit 0 = coverage holds (or the ratchet was initialised, or the run was
#          skipped for a named reason printed to stderr).
# Exit 1 = coverage fell past the tolerance, or it could not be measured.

set -uo pipefail

TOLERANCE_PP=0.60
RATCHET_FILE=ai-docs/coverage-ratchet.txt
PROFILE=tmp/coverage.out

mode=${1:-raise}
case "$mode" in
  raise|--check) ;;
  *) printf 'coverage-ratchet: unknown argument %s\n' "$mode" >&2; exit 1 ;;
esac

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'coverage-ratchet: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

# --- Fail directions, each named ---------------------------------------------
# `go` absent: SKIP, loud. A machine without a Go toolchain cannot be asked to
# measure Go coverage, and refusing every commit there would be the expensive
# error.
command -v go >/dev/null 2>&1 || {
  printf 'coverage-ratchet: go not on PATH; ratchet skipped\n' >&2; exit 0; }

if [ "$mode" = raise ]; then
  # Nothing that can move coverage is staged: SKIP, silently. This is what
  # keeps the hook free on the docs, plan and harness commits that make up most
  # of a /task run.
  staged=$(git diff --cached --name-only --diff-filter=ACMR \
    -- '*.go' '*.sql' go.mod go.sum 2>/dev/null)
  [ -n "$staged" ] || exit 0

  # Unstaged edits to files that move coverage: BLOCK, loud. The measurement is
  # taken on the working tree, so with such edits present it describes neither
  # the commit nor the tree. Silently measuring the wrong thing is how a ratchet
  # becomes decoration.
  dirty=$(git diff --name-only -- '*.go' '*.sql' go.mod go.sum 2>/dev/null)
  if [ -n "$dirty" ]; then
    printf 'coverage-ratchet: BLOCKED — unstaged changes to files that move coverage:\n' >&2
    printf '%s\n' "$dirty" | sed 's/^/  /' >&2
    printf '\nThe ratchet measures the working tree, so with these present the number\n' >&2
    printf 'describes neither the commit nor the tree. Stage them or stash them:\n' >&2
    printf '  git add -- %s\n' "$(printf '%s' "$dirty" | tr '\n' ' ')" >&2
    printf '  git stash push --keep-index\n' >&2
    exit 1
  fi
fi

mkdir -p tmp

if ! go test -covermode=atomic -coverprofile="$PROFILE" ./... > tmp/coverage-run.log 2>&1; then
  printf 'coverage-ratchet: BLOCKED — the test suite is not green, so coverage is not measurable.\n' >&2
  printf 'Log: tmp/coverage-run.log\n' >&2
  grep -E '^(FAIL|---|ok)' tmp/coverage-run.log >&2
  printf '\nIf this is a container-runtime failure rather than a test failure, point the\n' >&2
  printf 'suite at a running server: export LAB_GAME_TEST_DSN=postgres://...\n' >&2
  exit 1
fi

read -r current rounded <<EOF
$(awk '
  !/^mode:/ { total += $2; if ($3 > 0) covered += $2 }
  END {
    if (total == 0) { print "NaN NaN"; exit }
    p = 100 * covered / total
    printf "%.6f %.2f\n", p, int(p * 100 + 0.5) / 100
  }' "$PROFILE")
EOF

if [ "$current" = NaN ]; then
  printf 'coverage-ratchet: BLOCKED — the profile has no statements; nothing was measured.\n' >&2
  exit 1
fi

# Ratchet file absent or unparseable: INITIALISE and let the commit through.
# The owner's step 1 ("record the current state") is this branch, so the
# ratchet installs itself rather than needing a separate ceremony.
recorded=""
[ -r "$RATCHET_FILE" ] && recorded=$(tr -d '[:space:]' < "$RATCHET_FILE")
case "$recorded" in
  ''|*[!0-9.]*) recorded="" ;;
esac

if [ -z "$recorded" ]; then
  if [ "$mode" = --check ]; then
    printf 'coverage-ratchet: BLOCKED — %s is missing or unparseable.\n' "$RATCHET_FILE" >&2
    printf 'Measured now: %s%%. Commit that value to initialise the ratchet.\n' "$rounded" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$RATCHET_FILE")"
  printf '%s\n' "$rounded" > "$RATCHET_FILE"
  git add -- "$RATCHET_FILE"
  printf 'coverage-ratchet: initialised at %s%% (%s)\n' "$rounded" "$RATCHET_FILE" >&2
  exit 0
fi

verdict=$(awk -v cur="$current" -v rec="$recorded" -v rnd="$rounded" -v tol="$TOLERANCE_PP" '
  BEGIN {
    if (cur + tol < rec) { print "FELL"; exit }
    if (rnd > rec)       { print "ROSE"; exit }
    print "HELD"
  }')

case "$verdict" in
  FELL)
    printf 'coverage-ratchet: BLOCKED — statement coverage fell past the tolerance.\n\n' >&2
    printf '  recorded: %s%%   (%s)\n' "$recorded" "$RATCHET_FILE" >&2
    printf '  measured: %s%%\n' "$rounded" >&2
    printf '  tolerance: %s pp\n\n' "$TOLERANCE_PP" >&2
    printf 'Cover what this change added, or say in the commit why the drop is correct\n' >&2
    printf 'and lower the recorded value in the same commit — never with --no-verify.\n' >&2
    printf 'Uncovered lines, worst files first:\n' >&2
    go tool cover -func="$PROFILE" 2>/dev/null | awk '$3 != "100.0%" && $1 != "total:"' | head -20 >&2
    exit 1
    ;;
  ROSE)
    if [ "$mode" = --check ]; then
      printf 'coverage-ratchet: %s%% >= %s%% (a rise the pre-commit hook would have recorded)\n' \
        "$rounded" "$recorded" >&2
      exit 0
    fi
    printf '%s\n' "$rounded" > "$RATCHET_FILE"
    git add -- "$RATCHET_FILE"
    printf 'coverage-ratchet: raised %s%% -> %s%%\n' "$recorded" "$rounded" >&2
    exit 0
    ;;
  *)
    printf 'coverage-ratchet: %s%% holds against %s%% (tolerance %s pp)\n' \
      "$rounded" "$recorded" "$TOLERANCE_PP" >&2
    exit 0
    ;;
esac
