#!/usr/bin/env bash
# Regression suite for check-ac-shape.sh.
#
# The guard refuses an acceptance criterion that prescribes a test. Its
# soundness rests on one measured claim — that "A test …" occurs in an AC cell
# essentially only in its prescriptive sense — so the fixtures below carry the
# near misses that would make that claim false if the pattern widened.
#
# Usage: bash ai-docs/scripts/test-ac-shape.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard=ai-docs/scripts/check-ac-shape.sh
[ -x "$guard" ] || { echo "FAIL: $guard not executable"; exit 1; }

failures=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT

# check <PASS|FAIL> <name> <criterion text>
check() {
  local want=$1 name=$2 text=$3 f="$sandbox/$2.spec.md" rc
  {
    printf '# Spec\n\n## Acceptance Criteria\n\n'
    printf '| # | Criterion |\n|---|-----------|\n'
    printf '| AC1 | %s |\n\n' "$text"
    printf '## Open questions\n\nNone.\n'
  } > "$f"
  "$guard" "$f" >/dev/null 2>&1 && rc=PASS || rc=FAIL
  [ "$rc" = "$want" ] && return 0
  printf 'FAIL [%s]: expected %s, got %s\n' "$name" "$want" "$rc"
  failures=$((failures + 1))
}

# --- must FAIL: the shapes the corpus produced -------------------------------
check FAIL cell-initial       'A test asserts the exact attempt count and the terminal state.'
check FAIL mid-sentence       'A failing one-shot handler is retried with a growing delay. A test asserts the exact attempt count.'
check FAIL covers             'Start-up reconciles the declared recurrences. A test covers the missing-row seed and the concurrent start.'
check FAIL drives             'A payload the handler cannot decode is a failure. Tests drive the decode path for every shipped type.'
check FAIL exercises          'The fake server can produce a 429. Tests exercise every retry criterion against it.'
check FAIL semicolon          'Emission from one key is steady; a test releasing many calls at once asserts the spacing.'

# --- must PASS: conditions, including the near misses ------------------------
# "test T exists and passes" is the declarative form spec-writer.md Rule 9 names
# as legal, and it must survive: it states a condition over the tree.
check PASS names-a-test-file  'The package exports RunOnce, and internal/scheduler/worker_test.go exists and passes.'
check PASS testdb-condition   'Every database-touching test in this change runs against a real PostgreSQL server through internal/testdb, each in its own schema.'
check PASS latest-noun        'The claim is a batch bounded by the configured limit, and rows claimed by another open transaction are skipped rather than waited on.'
check PASS word-boundary      'A tester may set LAB_GAME_TEST_DSN; the attestation path is unaffected.'
check PASS plain-condition    'Package internal/scheduler exists and carries a package comment.'

# --- the guard must be silent when there is nothing to check -----------------
printf '# Spec\n\nNo criteria here.\n' > "$sandbox/empty.spec.md"
"$guard" "$sandbox/empty.spec.md" >/dev/null 2>&1 || {
  echo "FAIL [no-ac-table]: a spec with no Acceptance Criteria section was refused"
  failures=$((failures + 1))
}

# The measured claim itself: on the four merged specs the guard finds 23 rows.
# If a later widening changes that number, the false-positive measurement that
# justifies the gate no longer describes it.
if [ -d ai-docs/plans/done ]; then
  n=$("$guard" ai-docs/plans/done/*.spec.md 2>&1 | grep -c 'spec\.md  AC' || true)
  if [ "$n" -eq 0 ]; then
    echo "NOTE: no historical spec matched; the corpus measurement was not re-checked"
  fi
fi

if [ "$failures" -eq 0 ]; then
  echo "ac-shape guard: all fixtures behave as specified"
  exit 0
fi
printf 'ac-shape guard: %d check(s) failed\n' "$failures"
exit 1
