#!/usr/bin/env bash
# Regression suite for the PreToolUse guard that refuses a module-wide Go gate
# while Go source files sit under the scratch directory.
#
# The scratch directory is ignored by git and lies inside the module, so a Go
# source file there is compiled by every ./... gate while git status stays
# empty. The must-block trees are the three shapes recorded in two days: a
# delegate's deliberately broken probe package, a design's cited probe package,
# and a reviewer's mutation backups that kept their extension. The must-allow
# trees are the remedies those runs used: backups renamed off the Go suffix, a
# probe moved under an underscore directory with a symlink left at the cited
# path, and a checkout copy that carries its own go.mod.
#
# Anti-drift: the live hook body is extracted with jq and run as the program it
# is, inside a scratch directory set to each tree in turn.
#
# Verdict convention: the body exits 2 to block. Any other status proceeds.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-tmp-go-guard.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1

settings=".claude/settings.json"
# Select the entry by a fixed substring of its message, never by index.
body=$(jq -r '.hooks.PreToolUse[].hooks[].command
  | select(contains("Go source files under tmp/"))' "$settings")
[ -n "$body" ] || { echo "FAIL: tmp Go guard body not found in $settings"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/tmp-go-guard.XXXXXX")
trap 'rm -rf "$scratch"' EXIT

failures=0

# tree <name>: empty the scratch directory's tmp/, then build one state in it.
tree() {
  rm -rf "${scratch:?}/tmp"
  local t="$scratch/tmp" f
  case "$1" in
    none) ;;
    logs)
      mkdir -p "$t"; : > "$t/gate.log" ;;
    broken-probe)
      mkdir -p "$t/dgprobe/impossible"; printf 'package impossible\n' > "$t/dgprobe/impossible/p.go" ;;
    cited-probe)
      mkdir -p "$t/spiralprobe"; printf 'package main\n' > "$t/spiralprobe/main.go" ;;
    mutation-backups)
      mkdir -p "$t/sr1-bak"
      for f in depth next spiral; do printf 'package gate\n' > "$t/sr1-bak/$f.go"; done ;;
    renamed-backups)
      mkdir -p "$t/sr1-bak"
      for f in depth next spiral; do printf 'package gate\n' > "$t/sr1-bak/$f.go.bak"; done ;;
    underscore-probe)
      mkdir -p "$t/_spiralprobe"; printf 'package main\n' > "$t/_spiralprobe/main.go"
      ln -s _spiralprobe "$t/spiralprobe" ;;
    deep-underscore)
      mkdir -p "$t/_probe/x/y"; printf 'package y\n' > "$t/_probe/x/y/y.go" ;;
    dot-dir)
      mkdir -p "$t/.cache/x"; printf 'package x\n' > "$t/.cache/x/x.go" ;;
    testdata)
      mkdir -p "$t/testdata"; printf 'package x\n' > "$t/testdata/x.go" ;;
    nested-module)
      mkdir -p "$t/base/internal/a"; printf 'module example.com/base\n' > "$t/base/go.mod"
      printf 'package a\n' > "$t/base/internal/a/a.go" ;;
    *) echo "FAIL: unknown tree $1"; exit 1 ;;
  esac
}

# verdict_raw <payload>: feed one raw payload to the body, report BLOCK or ALLOW.
verdict_raw() {
  local rc
  (cd "$scratch" && printf '%s' "$1" | bash -c "$body" >/dev/null 2>&1) && rc=0 || rc=$?
  if [ "$rc" -eq 2 ]; then echo BLOCK; else echo ALLOW; fi
}

verdict() {
  verdict_raw "$(jq -n --arg c "$1" '{tool_input: {command: $c}}')"
}

# check <want> <label> <tree> <command>
check() {
  local got
  tree "$3"
  got=$(verdict "$4")
  if [ "$got" != "$1" ]; then
    printf 'FAIL: expected %s, got %s, for: %s [tree %s]\n' "$1" "$got" "$2" "$3"
    failures=$((failures + 1))
  fi
}

# --- must block: the recorded trees, under the gates they broke ---
check BLOCK 'broken probe package, go build'           broken-probe     'go build ./...'
check BLOCK 'cited probe package, golangci-lint run'   cited-probe      'golangci-lint run'
check BLOCK 'mutation backups, go build'               mutation-backups 'go build ./...'
check BLOCK 'mutation backups, go test with flags'     mutation-backups 'go test -race -count=1 ./...'
check BLOCK 'mutation backups, the provisioning wrapper' mutation-backups 'go run ./cmd/testpg -- go test ./...'
check BLOCK 'mutation backups, go vet captured to a log' mutation-backups 'mkdir -p tmp && go vet ./... > tmp/vet.log 2>&1'
check BLOCK 'mutation backups, lint with a flag'       mutation-backups 'golangci-lint run --timeout=5m ./...'
check BLOCK 'mutation backups, go list'                mutation-backups 'go list ./...'
check BLOCK 'mutation backups, make test'              mutation-backups 'make test'
check BLOCK 'mutation backups, make verify'            mutation-backups 'make verify'
check BLOCK 'mutation backups, bare make'              mutation-backups 'make'
check BLOCK 'mutation backups, gate on a second line'  mutation-backups "$(printf 'cd .\ngo test ./...')"

# --- must allow: the remedies those runs used, and what the go tool skips ---
check ALLOW 'backups renamed away from .go'            renamed-backups  'go build ./...'
check ALLOW 'probe under an underscore directory, symlink at the cited path' underscore-probe 'go build ./...'
check ALLOW 'deep under an underscore directory'       deep-underscore  'golangci-lint run'
check ALLOW 'a dot directory'                          dot-dir          'go build ./...'
check ALLOW 'a testdata directory'                     testdata         'go build ./...'
check ALLOW 'a checkout copy with its own go.mod'      nested-module    'go build ./...'
check ALLOW 'gate logs only'                           logs             'go test ./...'
check ALLOW 'no scratch directory'                     none             'go test ./...'

# --- must allow: commands that do not walk the module ---
check ALLOW 'a package-scoped test'                    mutation-backups 'go test ./internal/gate/'
check ALLOW 'lint scoped to a package'                 mutation-backups 'golangci-lint run ./internal/gate/'
check ALLOW 'a make target that compiles nothing'      mutation-backups 'make shellcheck'
check ALLOW 'git status'                               mutation-backups 'git status --porcelain'
check ALLOW 'the remedy itself'                        mutation-backups "for f in tmp/sr1-bak/*.go; do mv \"\$f\" \"\$f.bak\"; done"

# --- fail open on the hook's own inputs ---
tree mutation-backups
got=$(verdict_raw '{}')
[ "$got" = ALLOW ] || { echo "FAIL: a payload with no command must ALLOW, got $got"; failures=$((failures + 1)); }
got=$(verdict_raw 'not json')
[ "$got" = ALLOW ] || { echo "FAIL: an unparseable payload must ALLOW, got $got"; failures=$((failures + 1)); }

# --- the refusal names the file, so the reader can act on it ---
tree broken-probe
msg=$(cd "$scratch" && jq -n '{tool_input: {command: "go build ./..."}}' | bash -c "$body" 2>&1 >/dev/null)
printf '%s' "$msg" | grep -qF 'tmp/dgprobe/impossible/p.go' || {
  echo "FAIL: the refusal does not name the offending file"; failures=$((failures + 1)); }

if [ "$failures" -eq 0 ]; then
  echo "tmp Go guard: all fixtures behave as specified"
  exit 0
fi
printf 'tmp Go guard: %d check(s) failed\n' "$failures"
exit 1
