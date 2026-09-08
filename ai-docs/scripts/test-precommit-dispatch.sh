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
# It builds throwaway repositories with nothing tracked that can move coverage,
# so the ratchet takes its "nothing staged" exit and the path fixtures run in
# well under a second. What those exercise is the dispatch: the hook is found,
# it finds its script, and it does not fail on a path.
#
# THE GATE HALF runs against a STUB, not against the real reference gate. The
# sandbox writes an untracked module and an untracked command at the path the
# dispatcher invokes; the stub records its argument vector in a marker file and
# takes its exit status from a fixture file, so both directions are drivable
# and the never-invoked case is observable. Untracked is load-bearing twice:
# nothing about the stub is ever staged, so the ratchet keeps taking its cheap
# exit, and the fixtures stage a shell probe, which the gate covers and the
# ratchet does not.
#
# What that proves: the dispatcher's real invocation form, its named skip
# conditions, and its exit-code propagation in both directions. What it does
# NOT prove: the gate's own verdict. That belongs to the gate command's own
# tests, and saying so is the point — a suite that quietly proved neither is
# how a green instrument reads as a green subject.
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

# --- The tracked entry is a link, not a second copy of the script ------------
# Two copies of one hook body drift, and the reference gate would report the
# same comment twice, once under each name.
entry_mode=$(git ls-files -s -- .githooks/pre-commit | awk '{print $1}')
if [ "$entry_mode" != "120000" ]; then
  printf 'FAIL [symlink]: the tracked pre-commit entry is recorded as mode %s, not as a symbolic link\n' "${entry_mode:-<absent>}"
  failures=$((failures + 1))
fi
entry_target=$(readlink .githooks/pre-commit 2>/dev/null)
case "$entry_target" in
  */*)     printf 'FAIL [symlink]: the entry points outside its own directory: %s\n' "$entry_target"; failures=$((failures + 1)) ;;
  *.sh)    [ -f ".githooks/$entry_target" ] || { printf 'FAIL [symlink]: the entry points at a missing file: %s\n' "$entry_target"; failures=$((failures + 1)); } ;;
  *)       printf 'FAIL [symlink]: the entry points at %s, which does not carry the shell extension\n' "${entry_target:-<not a link>}"; failures=$((failures + 1)) ;;
esac

# --- The gate half, on a stub the fixtures drive ------------------------------
if ! command -v go >/dev/null 2>&1; then
  echo "NOTE: gate-dispatch fixtures SKIPPED — no Go toolchain here, so the stub cannot be built. This is a case that did not run, not a case that passed."
else
  gate="$sandbox/gate"
  mkdir -p "$gate/cmd/commentrefs"
  cp -r "$repo_root/.githooks" "$gate/.githooks"
  git -C "$gate" init -q
  git -C "$gate" config user.email probe@example.invalid
  git -C "$gate" config user.name probe
  git -C "$gate" config core.hooksPath .githooks
  printf 'seed\n' > "$gate/README.md"
  git -C "$gate" add README.md
  git -C "$gate" -c core.hooksPath= commit -q -m seed

  # Untracked, both of them: the module the dispatcher looks for and the command
  # it runs. The directive is read from this project's own module so the stub
  # builds under whatever toolchain is installed here.
  printf 'module gateprobe\n\ngo %s\n' "$(awk '/^go /{print $2; exit}' "$repo_root/go.mod")" > "$gate/go.mod"
  cat > "$gate/cmd/commentrefs/main.go" <<'GO'
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	_ = os.WriteFile("gate-marker.txt", []byte(strings.Join(os.Args[1:], " ")+"\n"), 0o644)
	verdict, _ := os.ReadFile("gate-verdict.txt")
	if strings.TrimSpace(string(verdict)) == "refuse" {
		fmt.Fprintln(os.Stderr, "STUB-GATE-REFUSED")
		os.Exit(1)
	}
}
GO

  gate_commit() {   # gate_commit <label> <path-to-stage>
    ( cd "$gate" && git add -- "$2" >/dev/null 2>&1 && git commit -m "$1" 2>&1 )
  }

  # (a) the gate refuses: the commit is refused, and the stub's stderr reaches
  #     the caller rather than being swallowed by the dispatcher.
  printf 'refuse\n' > "$gate/gate-verdict.txt"
  rm -f "$gate/gate-marker.txt"
  printf '#!/usr/bin/env bash\necho probe\n' > "$gate/probe.sh"
  if out=$(gate_commit "gate refuses" probe.sh); then
    printf 'FAIL [gate-refuses]: the commit succeeded although the gate exited non-zero\n%s\n' "$out"
    failures=$((failures + 1))
  elif ! printf '%s' "$out" | grep -q 'STUB-GATE-REFUSED'; then
    printf 'FAIL [gate-refuses]: the gate ran and refused, but its stderr never reached the caller\n%s\n' "$out"
    failures=$((failures + 1))
  fi
  # The invocation form itself, recorded by the stub rather than asserted from
  # the dispatcher's source.
  if [ "$(cat "$gate/gate-marker.txt" 2>/dev/null)" != "--staged" ]; then
    printf 'FAIL [gate-argv]: the gate was invoked as %s, not over the staged set\n' "$(cat "$gate/gate-marker.txt" 2>/dev/null || echo '<never invoked>')"
    failures=$((failures + 1))
  fi

  # (b) the same staged content, the gate accepting: the commit goes through.
  printf 'accept\n' > "$gate/gate-verdict.txt"
  if ! out=$(gate_commit "gate accepts" probe.sh); then
    printf 'FAIL [gate-accepts]: the commit was refused although the gate exited 0\n%s\n' "$out"
    failures=$((failures + 1))
  fi

  # (c) nothing gated staged: the gate must not be invoked at all. Without this
  #     case, deleting the staged-path condition would pass (a) and (b) both.
  printf 'refuse\n' > "$gate/gate-verdict.txt"
  rm -f "$gate/gate-marker.txt"
  printf 'more\n' >> "$gate/README.md"
  if ! out=$(gate_commit "nothing gated staged" README.md); then
    printf 'FAIL [gate-skip-nothing-gated]: the commit was refused although no gated path was staged\n%s\n' "$out"
    failures=$((failures + 1))
  elif [ -e "$gate/gate-marker.txt" ]; then
    printf 'FAIL [gate-skip-nothing-gated]: the gate ran on a commit that staged no gated path\n'
    failures=$((failures + 1))
  elif ! printf '%s' "$out" | grep -q 'no staged path'; then
    printf 'FAIL [gate-skip-nothing-gated]: the skip was silent; it must name its reason\n%s\n' "$out"
    failures=$((failures + 1))
  fi

  # (d) the module is gone: a named, loud skip, and the commit proceeds.
  rm -f "$gate/go.mod"
  rm -f "$gate/gate-marker.txt"
  printf 'echo again\n' >> "$gate/probe.sh"
  if ! out=$(gate_commit "no module" probe.sh); then
    printf 'FAIL [gate-skip-no-module]: the commit was refused although the skip is a pass\n%s\n' "$out"
    failures=$((failures + 1))
  elif ! printf '%s' "$out" | grep -q 'no go.mod'; then
    printf 'FAIL [gate-skip-no-module]: the skip was silent; it must name its reason\n%s\n' "$out"
    failures=$((failures + 1))
  elif [ -e "$gate/gate-marker.txt" ]; then
    printf 'FAIL [gate-skip-no-module]: the gate ran with no module at the worktree root\n'
    failures=$((failures + 1))
  fi
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
