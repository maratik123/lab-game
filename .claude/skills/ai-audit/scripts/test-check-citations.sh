#!/usr/bin/env bash
# Regression test for the citation-namespace guard.
#
# Locks ONE invariant: check (2)'s format-spec exclusion must identify the
# excluded text by its CONTENT, not by its line number.
#
# Why that invariant and not just "the gate is green": a fix that merely
# re-pins the hardcoded line number (47 -> 49) would make the gate green
# today and silently rot again on the next insertion above that row. Case 2
# below is what distinguishes a real fix from a re-pin — it shifts the row and
# requires the guard to still find it.
#
# Exit 0 = all cases pass. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-check-citations.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

guard=".claude/skills/ai-audit/scripts/check-citations.sh"
target="ai-docs/corrections-log.md"
backup=$(mktemp)
failures=0

# Restore the pristine file however we exit — including on interrupt. A stray
# edit to a tracked doc is worse than a failed test.
# The signal handlers EXIT rather than sharing the EXIT body: a handler that
# only cleans up lets the script resume with its backup already deleted, so
# every later restore silently fails and the target is left mutated.
# shellcheck disable=SC2064  # intentional: expand $backup/$target now, not at trap time
trap "cp '$backup' '$target'; rm -f '$backup'" EXIT
# shellcheck disable=SC2064  # same
trap "cp '$backup' '$target'; rm -f '$backup'; exit 130" INT
# shellcheck disable=SC2064  # same
trap "cp '$backup' '$target'; rm -f '$backup'; exit 143" TERM
cp "$target" "$backup"
mode_before=$(stat -c '%a' "$target")

guard_out=""
run_guard() {
  # Capture the guard's own output so a FAIL can say WHY -- a silent >/dev/null
  # once hid a transient gh outage behind "expected exit 0, got 1".
  guard_out=$(bash "$guard" 2>&1)
}

report() {
  # $1 = case name, $2 = observed exit, $3 = expected exit
  if [ "$2" -eq "$3" ]; then
    printf '  PASS  %s (exit %s)\n' "$1" "$2"
  else
    printf '  FAIL  %s — expected exit %s, got %s\n' "$1" "$3" "$2"
    printf '%s\n' "$guard_out" | sed 's/^/        | /'
    failures=$((failures + 1))
  fi
}

echo "== test-check-citations =="
echo

# --- Case 1: the guard is green on the pristine tree -------------------------
# Failed before the fix: the exclusion was pinned to a line number while the
# format-spec example it means to exclude had drifted two lines down.
run_guard
report "case 1: green on pristine tree" "$?" 0

# --- Case 2: the exclusion survives line drift --------------------------------
# Insert blank lines above the glossary so every row below shifts down. A
# content-based exclusion still finds its target; a line-pinned one does not.
# Blank lines are used deliberately — they add no text that could trip any
# other check, so a RED here isolates the drift behaviour.
# shellcheck disable=SC2016  # literal backticks in the markdown row are the pattern, not an expansion
row_before=$(grep -n '^> `Superseded by:`' "$target" | cut -d: -f1)
shifted=$(mktemp)
{ head -n 1 "$target"; printf '\n\n\n'; tail -n +2 "$target"; } > "$shifted"
# `cat >` into the existing path, never `mv` onto it: `mv` would replace the
# tracked file with mktemp's inode and permanently drop its mode 644 -> 600.
# Git does not track that bit, so `git status` stays clean and the damage is
# invisible to the obvious check.
cat "$shifted" > "$target"
rm -f "$shifted"
# shellcheck disable=SC2016  # same literal-backtick pattern as above
row_after=$(grep -n '^> `Superseded by:`' "$target" | cut -d: -f1)

if [ "$row_before" = "$row_after" ]; then
  printf '  FAIL  case 2 setup — row did not move (%s); the drift was never exercised\n' "$row_before"
  failures=$((failures + 1))
else
  printf '  ....  case 2 setup: Superseded-by row moved %s -> %s\n' "$row_before" "$row_after"
  run_guard
  report "case 2: green after the excluded row drifts" "$?" 0
fi

cp "$backup" "$target"

# --- Case 3: a REAL unresolvable citation is still caught ---------------------
# Guards against over-correcting case 1 into a blanket skip of the whole file.
#
# The fixture date is ASSEMBLED AT RUNTIME, never written as a literal. This
# file sits inside the instruction surface the guard scans — a literal
# out-of-range date here would make the guard flag its own test as a bad
# citation. Excluding this path in the guard was the alternative and was
# rejected: a path is another pinned identifier, and a rename would break it
# silently, which is the exact bug class this test exists to prevent.
bad_year="2026"
bad_date="${bad_year}-03-14"
printf '\n> See the %s learnings entry for context.\n' "$bad_date" >> "$target"
run_guard
report "case 3: genuine bad citation still RED" "$?" 1

cp "$backup" "$target"

# --- Cases 5-7: the ceiling is the newer of the newest issue and pull request -
# A fake gh on PATH answers the two high-water reads with chosen numbers, so
# the ordering that produced the false red -- an issue filed after the last
# pull request, then cited -- is reproducible on any tree. The cited numbers
# are ASSEMBLED AT RUNTIME for the same reason as case 3's date: a literal
# above the real ceiling in this file would be flagged by the real guard.
fakebin=$(mktemp -d)
cat > "$fakebin/gh" <<'GH'
#!/usr/bin/env bash
case "$1 $2" in
  "pr list") printf '%s\n' "${FAKE_PR_MAX-}" ;;
  "issue list") printf '%s\n' "${FAKE_ISSUE_MAX-}" ;;
  *) exit 1 ;;
esac
GH
chmod +x "$fakebin/gh"
fake_pr=1000
fake_issue=$((fake_pr + 5))
run_fake_guard() {
  guard_out=$(PATH="$fakebin:$PATH" FAKE_PR_MAX="$1" FAKE_ISSUE_MAX="$2" bash "$guard" 2>&1)
}

cited=$((fake_pr + 3))
printf '\n> Filed as #%s during the run.\n' "$cited" >> "$target"
run_fake_guard "$fake_pr" "$fake_issue"
report "case 5: a ref above the newest pull request but not above the newest issue is local" "$?" 0
cp "$backup" "$target"

cited=$((fake_issue + 2))
printf '\n> Filed as #%s during the run.\n' "$cited" >> "$target"
run_fake_guard "$fake_pr" "$fake_issue"
report "case 6: a ref above both is still RED" "$?" 1
cp "$backup" "$target"

run_fake_guard "$fake_pr" ""
rc=$?
if [ "$rc" -eq 1 ] && printf '%s' "$guard_out" | grep -q 'Instrument failure'; then
  printf '  PASS  case 7: an unreadable issue mark is an instrument error, not a pass\n'
else
  printf '  FAIL  case 7: an unreadable issue mark gave exit %s without the instrument error\n' "$rc"
  printf '%s\n' "$guard_out" | sed 's/^/        | /'
  failures=$((failures + 1))
fi
rm -rf "$fakebin"

# --- Case 4: this test must not mutate the tracked file's MODE ---------------
# `git status` cannot see a permission change, so a test that quietly drops
# 644 -> 600 (as `mv` from mktemp does) leaves damage no obvious check reports.
mode_after=$(stat -c '%a' "$target")
if [ "$mode_before" = "$mode_after" ]; then
  printf '  PASS  case 4: target mode preserved (%s)\n' "$mode_after"
else
  printf '  FAIL  case 4: target mode changed %s -> %s\n' "$mode_before" "$mode_after"
  failures=$((failures + 1))
fi

echo
if [ "$failures" -gt 0 ]; then
  echo "FAIL: ${failures} case(s) regressed."
  exit 1
fi
echo "PASS: exclusion is content-addressed and drift-proof."
