#!/usr/bin/env bash
# The status checks a branch's rules require and the jobs the workflows define
# must be one set, compared by the job's display name.
#
# WHY. A required check is matched by name. A job renamed or removed without the
# same edit to the ruleset leaves that check pending for ever, so every pull
# request waits on it; a job added without one runs, reports and blocks nothing.
# Neither shows on the pull request that caused it.
#
# WHAT THIS REFUSES (exit 1): a required check that no workflow job produces,
# and a workflow job that no rule requires.
#
# FAIL DIRECTION. The rules are read from the GitHub API, and their absence is
# the repository owner's state, not a defect of the change under test. No rule
# that requires checks, an API call that fails, a reply that is not the rules
# list: each passes, with a notice saying which. So does a workflow whose check
# names the file cannot give, a matrix job or a name built from an expression,
# because those names carry values only a run knows.
#
# Environment: RULESET_BRANCH names the branch whose rules are read (default
# main); GITHUB_REPOSITORY, when set, names the repository.
#
# Exit 0 = the two sets agree, or could not be compared (a notice says why).
# Exit 1 = they disagree.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  check-ruleset-checks.sh    compare the status checks the branch rules require
                             with the display names of the workflow jobs; it
                             takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'check-ruleset-checks: not a git work tree; skipped\n' >&2; exit 0; }
cd "$root" || exit 1

branch=${RULESET_BRANCH:-main}

skip() {
  printf 'check-ruleset-checks: %s; the two sets were not compared\n' "$*" >&2
  exit 0
}

shopt -s nullglob
workflows=(.github/workflows/*.yml .github/workflows/*.yaml)
shopt -u nullglob
[ "${#workflows[@]}" -gt 0 ] || skip "no workflow file"

# A top-level job is a two-space key under `jobs:`, and the job's own keys sit
# four spaces in. A step's name is deeper, so the four-space name is the job's
# display name; a job with no name reports under its key.
jobs=$(awk -v q="'" '
  function emit() {
    if (id == "") return
    if (matrix || name ~ /\$\{\{/) print "UNREADABLE\t" id
    else print "JOB\t" (name != "" ? name : id)
    id = ""
  }
  FNR == 1 { emit(); injobs = 0 }
  /^jobs:[[:space:]]*$/ { injobs = 1; next }
  /^[^[:space:]#]/ { emit(); injobs = 0; next }
  !injobs { next }
  /^  [A-Za-z0-9_-]+:[[:space:]]*$/ {
    emit()
    id = $0; sub(/^  /, "", id); sub(/:[[:space:]]*$/, "", id)
    name = ""; matrix = 0
    next
  }
  id != "" && /^    name:/ {
    name = $0
    sub(/^    name:[[:space:]]*/, "", name); sub(/[[:space:]]+$/, "", name)
    gsub("^[\"" q "]|[\"" q "]$", "", name)
    next
  }
  id != "" && /^      matrix:/ { matrix = 1 }
  END { emit() }
' "${workflows[@]}")

unreadable=$(printf '%s\n' "$jobs" | awk -F'\t' '$1 == "UNREADABLE" { print $2 }' | paste -sd, -)
[ -z "$unreadable" ] || skip "the check names of job(s) $unreadable depend on values only a run knows"
names=$(printf '%s\n' "$jobs" | awk -F'\t' '$1 == "JOB" { print $2 }' | LC_ALL=C sort -u)
[ -n "$names" ] || skip "no job found in the workflow files"

repo=${GITHUB_REPOSITORY:-}
if [ -z "$repo" ]; then
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null) || repo=
fi
[ -n "$repo" ] || skip "the repository could not be named: GITHUB_REPOSITORY is unset and gh repo view failed"

endpoint="repos/$repo/rules/branches/$branch"
reply=$(gh api "$endpoint" 2>&1) || skip "gh api $endpoint failed: $(printf '%s\n' "$reply" | head -n 1)"
required=$(printf '%s' "$reply" | jq -r '.[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context' 2>/dev/null) ||
  skip "the reply from $endpoint is not the rules list this check reads"
required=$(printf '%s\n' "$required" | grep -v '^$' | LC_ALL=C sort -u)
[ -n "$required" ] || skip "no rule in force on $branch requires a status check"

missing=$(LC_ALL=C comm -23 <(printf '%s\n' "$required") <(printf '%s\n' "$names"))
advisory=$(LC_ALL=C comm -13 <(printf '%s\n' "$required") <(printf '%s\n' "$names"))

if [ -z "$missing" ] && [ -z "$advisory" ]; then
  printf 'check-ruleset-checks: the checks required on %s and the workflow jobs agree\n' "$branch"
  exit 0
fi

if [ -n "$missing" ]; then
  printf 'check-ruleset-checks: required on %s and produced by no workflow job, so every pull request waits on it:\n' "$branch" >&2
  printf '%s\n' "$missing" | sed 's/^/  /' >&2
fi
if [ -n "$advisory" ]; then
  printf 'check-ruleset-checks: a workflow job no rule on %s requires, so it reports and blocks nothing:\n' "$branch" >&2
  printf '%s\n' "$advisory" | sed 's/^/  /' >&2
fi
cat >&2 <<'MSG'

A required check is matched by the job's display name. Add, rename or remove a
job together with the same change to the ruleset. That change is the repository
owner's: name the check in the pull request and ask for it.
MSG
exit 1
