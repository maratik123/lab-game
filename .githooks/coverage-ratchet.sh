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
# THE TOLERANCE, AND THE MEASUREMENT UNDER IT. 24 runs of the suite on 8fae04a,
# 1329 statements throughout:
#
#   covered  percent   rounded  runs
#     1189   89.4658%   89.47    20   (83%)
#     1190   89.5410%   89.54     2   ( 8%)
#     1191   89.6163%   89.62     1   ( 4%)
#     1194   89.8420%   89.84     1   ( 4%)
#
# Observed spread 0.3762 pp; one statement is worth 0.0752 pp. Five blocks flip
# between runs, all of them timing-dependent, all in one package:
#   internal/scheduler/execute.go:167.87,170.3
#   internal/scheduler/execute.go:223.73,225.3
#   internal/scheduler/settle.go:202.101,204.4
#   internal/scheduler/worker.go:116.57,119.4
#   internal/scheduler/worker.go:150.21,151.20
#
# The spread is the binding constraint on the tolerance, and it binds whatever
# the raise rule is: a lucky run RECORDS its value, and every ordinary run
# afterwards reads as a fall of up to the spread. So TOLERANCE_PP must exceed
# 0.3762, or the ratchet blocks forever with no code change involved. 0.50 was
# that with headroom for a tail 24 runs had not seen — the 4-statement jump was
# observed once.
#
# RAISED TO 0.60 on 2026-09-07, and the reason is a term the series above could
# not see: it was measured on ONE machine, and the ratchet is checked on two.
# On PR #66, run 34144605615, CI measured 90.88% where this repository's
# development machine measured 91.28% twice in a row at the same commit — a gap
# of 0.40 pp, about 7 statements at today's 1744. The five blocks below are
# worth 0.2867 pp at that denominator, so the cross-environment term is the
# LARGER of the two and is not drift between runs at all: it is the same suite
# taking different branches under a different container runtime and a slower
# runner. A mark recorded by a local pre-commit therefore leaves CI only
# (tolerance - gap) of headroom, and at 0.50 that was 0.10 pp — two statements.
# 0.60 restores a usable margin without touching what the ratchet guards.
#
# What that costs is bounded the same way as before, one tolerance below the
# all-time high, now about 10.5 statements at 1744, ONCE.
#
# WHAT THE TOLERANCE COSTS, bounded: the recorded value never decreases, so the
# total coverage that can be lost silently is one tolerance below the all-time
# high — ONCE, not per commit.
#
# THE TOLERANCE IS A STANDING VALUE — there is no plan to tighten it, and
# chasing the five blocks is explicitly not one. Mocking a server-side clock through the
# database is not a cheap change, and buying tenths of a percentage point with
# it would be the ratchet setting the project's priorities instead of guarding
# them.
#
# The spread narrows on its own as the tree grows, because it is a COUNT of
# statements over a growing denominator. The same five blocks are worth
# 0.3762 pp at today's 1329 statements, 0.25 pp at 2000, and 0.167 pp at 3000.
# The tolerance does not have to follow it down: a fixed value simply becomes
# roomier, and what it can hide stays bounded at one tolerance below the
# all-time high, once. The cross-environment term added above narrows the same
# way, for the same reason.
#
# Revisit this number only on a re-measurement — if a series of runs shows the
# spread has GROWN past it, which would mean new flaky blocks arrived faster
# than the denominator grew. Re-run the series before touching the constant;
# do not adjust it from a single blocked commit.
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
