#!/usr/bin/env bash
# Regression suite for the pre-commit hook — the dispatch and its path
# resolution, not the ratchet's arithmetic.
#
# THE QUESTION THIS ANSWERS. The hook used to `exec` a bare relative path and
# so depended on githooks(5)'s promise that git chdirs to the root of the
# working tree first. That promise holds — measured from the repository root,
# from a subdirectory, under `git -C` from outside the repo, and inside a
# linked worktree and its subdirectories — but it is invisible at the call
# site, and a reviewer asked exactly the right question about it. The hook now
# resolves the path itself; this suite is what keeps it that way.
#
# It builds a throwaway repository with no Go in it, so the ratchet takes its
# "nothing that can move coverage is staged" exit and the whole suite runs in
# well under a second. What is exercised is the dispatch: the hook is found,
# it finds its script, and it does not fail on a path.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-precommit-dispatch.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
[ -x .githooks/pre-commit ] || { echo "FAIL: .githooks/pre-commit not executable"; exit 1; }

failures=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT

# A repository that shares this project's hook shape and nothing else.
sut="$sandbox/repo"
mkdir -p "$sut/deep/nest"
cp -r "$repo_root/.githooks" "$sut/.githooks"
git -C "$sut" init -q
git -C "$sut" config user.email probe@example.invalid
git -C "$sut" config user.name probe
git -C "$sut" config core.hooksPath .githooks
printf 'seed\n' > "$sut/README.md"
git -C "$sut" add README.md
git -C "$sut" -c core.hooksPath= commit -q -m seed

# commit_from <cwd> <label> [outside]
# "outside" runs git with -C, the way a caller that is not inside the tree must.
commit_from() {
  local dir=$1 label=$2 outside=${3:-} out rc g
  printf '%s\n' "$label" >> "$sut/README.md"
  if [ -n "$outside" ]; then
    g="git -C $sut"
  else
    g="git"
  fi
  # `:/` is a repository-root-relative pathspec, so one spelling works from
  # every cwd the fixtures use.
  out=$( cd "$dir" && $g add -- :/README.md 2>&1 && $g commit -m "$label" 2>&1 ) && rc=0 || rc=$?
  if [ "$rc" -ne 0 ]; then
    printf 'FAIL [%s]: commit refused (exit %s)\n%s\n' "$label" "$rc" "$out"
    failures=$((failures + 1))
    return
  fi
  # The signature of the defect this suite exists for: the dispatcher could not
  # find the script it execs.
  if printf '%s' "$out" | grep -qiE 'no such file|not found|cannot execute'; then
    printf 'FAIL [%s]: the hook could not resolve its script\n%s\n' "$label" "$out"
    failures=$((failures + 1))
  fi
}

commit_from "$sut" root
commit_from "$sut/deep" subdir
commit_from "$sut/deep/nest" deeper-subdir
commit_from "$sandbox" outside-via-git-C outside

# A linked worktree: git chdirs to THAT worktree's root, and .githooks is
# tracked, so the tracked copy travels with it.
wt="$sandbox/worktree"
if git -C "$sut" worktree add -q --detach "$wt" HEAD 2>/dev/null; then
  printf 'wt\n' >> "$wt/README.md"
  if out=$( cd "$wt" && git add -- :/README.md && git commit -m worktree 2>&1 ); then
    printf '%s' "$out" | grep -qiE 'no such file|not found|cannot execute' && {
      printf 'FAIL [worktree]: the hook could not resolve its script\n%s\n' "$out"
      failures=$((failures + 1))
    }
  else
    printf 'FAIL [worktree]: commit refused\n%s\n' "$out"
    failures=$((failures + 1))
  fi
  git -C "$sut" worktree remove --force "$wt" >/dev/null 2>&1
else
  echo "NOTE: linked-worktree fixture skipped (git worktree unavailable here)"
fi

# THE INSTRUMENT CHECK. Every case above passes when the hook runs and skips,
# and it also passes when the hook never runs at all — the ratchet is silent
# either way with nothing coverage-moving staged. So drive the same repository
# with the ratchet script REMOVED: git must then refuse the commit, because the
# dispatcher cannot exec what is not there. If this case passes too, none of
# the cases above measured anything: a green instrument is a claim about the
# instrument.
control="$sandbox/control"
mkdir -p "$control/deep"
cp -r "$repo_root/.githooks" "$control/.githooks"
rm -f "$control/.githooks/coverage-ratchet.sh"
git -C "$control" init -q
git -C "$control" config user.email probe@example.invalid
git -C "$control" config user.name probe
git -C "$control" config core.hooksPath .githooks
printf 'seed\n' > "$control/README.md"
git -C "$control" add README.md
git -C "$control" -c core.hooksPath= commit -q -m seed
printf 'x\n' >> "$control/README.md"
if ( cd "$control/deep" && git add -- :/README.md && git commit -m "must be refused" ) >/dev/null 2>&1; then
  echo "FAIL [instrument]: a commit succeeded with the ratchet script removed — the hook is not running, so the cases above prove nothing"
  failures=$((failures + 1))
fi

# Anti-drift: the dispatcher must keep resolving from the worktree root. A
# later "simplification" back to a bare relative exec re-creates the invisible
# dependency this suite was written to retire.
grep -q 'git rev-parse --show-toplevel' .githooks/pre-commit || {
  echo "FAIL: .githooks/pre-commit no longer resolves its script from the worktree root"
  failures=$((failures + 1))
}

if [ "$failures" -eq 0 ]; then
  echo "pre-commit dispatch: all fixtures behave as specified"
  exit 0
fi
printf 'pre-commit dispatch: %d check(s) failed\n' "$failures"
exit 1
