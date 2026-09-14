#!/usr/bin/env bash
# Regression suite for check-ruleset-checks.sh.
#
# Every fixture runs the real checker in a scratch repository whose workflow
# file is set per case, against a stand-in for the GitHub CLI that replays a
# recorded reply. A reply has the shape the rules endpoint returns for a
# branch: a list of rules, one of them carrying the required status checks.
#
# The fail-open cases are asserted as carefully as the refusals. A missing
# ruleset is the owner's state, and a checker that turned it red would stop
# every pull request for a reason none of them caused.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-ruleset-checks.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
checker="$repo_root/ai-docs/scripts/check-ruleset-checks.sh"
real_workflow="$repo_root/.github/workflows/ci.yml"
[ -x "$checker" ] || { echo "FAIL: $checker not executable"; exit 1; }
[ -f "$real_workflow" ] || { echo "FAIL: $real_workflow absent"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/ruleset-checks.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
repo="$scratch/repo"
mkdir -p "$scratch/bin" "$repo/.github/workflows"
git init -q "$repo"

# The stand-in prints the recorded reply and exits with the recorded status.
cat > "$scratch/bin/gh" <<'GH'
#!/usr/bin/env bash
cat "$FAKE_GH_REPLY"
exit "$FAKE_GH_RC"
GH
chmod +x "$scratch/bin/gh"

failures=0
gh_rc=0

reply() { printf '%s' "$1" > "$scratch/reply.json"; }

# rules <context>...: a rules reply whose required-checks rule names these.
rules() {
  local ctx="" c
  for c in "$@"; do ctx="$ctx${ctx:+,}{\"context\":\"$c\"}"; done
  reply "[{\"type\":\"deletion\"},{\"type\":\"non_fast_forward\"},{\"type\":\"required_status_checks\",\"parameters\":{\"strict_required_status_checks_policy\":false,\"required_status_checks\":[$ctx]}}]"
}

workflow() { cat > "$repo/.github/workflows/ci.yml"; }

# expect <exit> <label> [a substring stderr must carry] [one it must not]
expect() {
  local rc err
  err=$(cd "$repo" && PATH="$scratch/bin:$PATH" GITHUB_REPOSITORY=o/r \
    FAKE_GH_REPLY="$scratch/reply.json" FAKE_GH_RC="$gh_rc" "$checker" 2>&1 >/dev/null) && rc=0 || rc=$?
  if [ "$rc" != "$1" ]; then
    printf 'FAIL: expected exit %s, got %s, for: %s\n%s\n' "$1" "$rc" "$2" "$err"
    failures=$((failures + 1))
  elif [ -n "${3:-}" ] && ! printf '%s' "$err" | grep -qF -- "$3"; then
    printf 'FAIL: %s: stderr does not carry %s\n%s\n' "$2" "$3" "$err"
    failures=$((failures + 1))
  elif [ -n "${4:-}" ] && printf '%s' "$err" | grep -qF -- "$4"; then
    printf 'FAIL: %s: stderr carries %s\n%s\n' "$2" "$4" "$err"
    failures=$((failures + 1))
  fi
}

workflow <<'YML'
name: CI
on:
  pull_request:
jobs:
  changes:
    name: Detect changes
    runs-on: ubuntu-latest
    steps:
      - name: a step, not a job
        run: true
  build:
    name: "Build"
    needs: changes
    steps:
      - run: true
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: true
YML

# --- agree ---
rules 'Detect changes' Build lint
expect 0 'every job required, every required check a job'

# --- refuse: the two directions, and a rename that is both ---
rules 'Detect changes' Build lint Test
expect 1 'a required check no job produces' 'Test'
rules 'Detect changes' Build
expect 1 'a job no rule requires' 'lint'
rules 'Detect changes' 'Build and vet' lint
expect 1 'a job renamed without the ruleset' 'Build and vet'
expect 1 'a job renamed without the ruleset, the other half' '  Build'

# --- pass with a notice: nothing to compare against ---
reply '[]'
expect 0 'no rule on the branch' 'no rule in force'
reply '[{"type":"deletion"},{"type":"non_fast_forward"}]'
expect 0 'rules, none requiring checks' 'no rule in force'
reply 'gh: Not Found (HTTP 404)'
gh_rc=1
expect 0 'the API call fails' 'failed'
gh_rc=0
reply 'not json'
expect 0 'a reply that is not JSON' 'not the rules list'
reply '{"message":"Not Found"}'
expect 0 'a reply that is an error object' 'not the rules list'

# --- pass with a notice: names the file cannot give ---
rules 'Detect changes'
workflow <<'YML'
jobs:
  test:
    name: Test
    strategy:
      matrix:
        go: [stable, oldstable]
    steps:
      - run: true
YML
expect 0 'a matrix job' 'values only a run knows'
workflow <<'YML'
jobs:
  test:
    name: ${{ github.event_name }} test
    steps:
      - run: true
YML
expect 0 'a name built from an expression' 'values only a run knows'

# --- the parser across more than one workflow file ---
workflow <<'YML'
jobs:
  a:
    name: First
    steps:
      - run: true
YML
cat > "$repo/.github/workflows/other.yml" <<'YML'
jobs:
  b:
    name: Second
    steps:
      - run: true
YML
rules First Second
expect 0 'jobs from two workflow files'
rules First
expect 1 'a job in the second workflow file' 'Second'
rm "$repo/.github/workflows/other.yml"

# --- the parser on the real workflow: job names read, step names not ---
cp "$real_workflow" "$repo/.github/workflows/ci.yml"
rules 'Harness guards'
expect 1 'the real workflow, one check required' 'Detect changes' 'shellcheck every tracked script'

# --- no workflow at all ---
rm -f "$repo/.github/workflows/ci.yml"
expect 0 'no workflow file' 'no workflow file'

if [ "$failures" -eq 0 ]; then
  echo "ruleset checks: all fixtures behave as specified"
  exit 0
fi
printf 'ruleset checks: %d check(s) failed\n' "$failures"
exit 1
