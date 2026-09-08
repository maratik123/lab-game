#!/usr/bin/env bash
# git pre-commit hook body: the comment-reference gate over the staged set,
# then the coverage ratchet. Reached only through the tracked pre-commit
# entry, this file's own symlink target.
#
# PATHS ARE RESOLVED FROM THE WORKTREE ROOT, NOT FROM $PWD. githooks(5)
# promises git chdirs to the root of the working tree before running a hook,
# and measurement agrees — a bare invocation of the ratchet script resolves
# from the repository root, from a subdirectory, under `git -C` from outside the
# repo, and inside a linked worktree and its subdirectories. It is still
# resolved explicitly here: the promise is invisible at the call site, a
# reviewer has to go and look it up, and this hook is also the thing someone
# runs by hand to see what it does.
#
# Bypassing this hook is the owner's escape, not an agent's: a `PreToolUse`
# guard refuses `git commit --no-verify` from inside the harness.

set -uo pipefail

root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  printf 'pre-commit: not a git work tree; no gate run\n' >&2
  exit 0
}
cd "$root" || exit 1

# The comment-reference gate, run only when it has something to judge and a
# toolchain to run it with. Each skip is named and loud — never a silent
# pass — because a gate that could not run is not a clean tree either.
staged=$(git diff --cached --name-only --diff-filter=ACMR 2>/dev/null | \
  grep -E '\.(go|sh|sql|ya?ml)$|(^|/)(Makefile|\.gitignore|\.env\.example)$|^\.githooks/')

if [ -z "$staged" ]; then
  printf 'pre-commit: no staged path of the comment-reference gated set; gate skipped\n' >&2
elif [ ! -f "$root/go.mod" ]; then
  printf 'pre-commit: no go.mod at the worktree root; comment-reference gate skipped\n' >&2
elif ! command -v go >/dev/null 2>&1; then
  printf 'pre-commit: go not on PATH; comment-reference gate skipped\n' >&2
else
  go run ./cmd/commentrefs --staged
  status=$?
  if [ "$status" -ne 0 ]; then
    exit "$status"
  fi
fi

exec "$root/.githooks/coverage-ratchet.sh"
