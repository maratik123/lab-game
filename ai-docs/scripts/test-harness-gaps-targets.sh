#!/usr/bin/env bash
# Regression suite for check-harness-gaps-targets.sh.
#
# Each fixture is a log written into a scratch repository that holds a few real
# files, and the checker runs there. The refusals are the shapes the real log
# has held: a script that moved directories while an open entry still named its
# old path, and a brace list one member of which named a renamed file. The
# passes are what an append-only log legitimately keeps: closed and superseded
# entries whose paths are history, prose tokens in backticks, paths outside the
# tree, and the skeleton inside its fence.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-harness-gaps-targets.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
checker="$repo_root/ai-docs/scripts/check-harness-gaps-targets.sh"
[ -x "$checker" ] || { echo "FAIL: $checker not executable"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/harness-gaps-targets.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
repo="$scratch/repo"
git init -q "$repo"
mkdir -p "$repo/.claude/agents" "$repo/ai-docs/scripts"
: > "$repo/.claude/agents/self-review.md"
: > "$repo/.claude/agents/design-review.md"
: > "$repo/AGENTS.md"
: > "$repo/.gitignore"
: > "$repo/ai-docs/scripts/check-ac-shape.sh"

failures=0

# log: a header and the fenced skeleton, whose example path is dead on purpose,
# then the entries from stdin.
# shellcheck disable=SC2016  # the backticks are markdown code spans in the log
log() {
  {
    printf '# Harness gaps\n\n```\n### YYYY-MM-DD — [short description]\n'
    printf '**target:** [harness file — e.g. `.claude/agents/absent.md`]\n```\n\n---\n\n'
    cat
  } > "$repo/ai-docs/harness-gaps.md"
}

# expect <exit> <label> [a substring stderr must carry]
expect() {
  local rc err
  err=$(cd "$repo" && "$checker" 2>&1 >/dev/null) && rc=0 || rc=$?
  if [ "$rc" != "$1" ]; then
    printf 'FAIL: expected exit %s, got %s, for: %s\n%s\n' "$1" "$rc" "$2" "$err"
    failures=$((failures + 1))
  elif [ -n "${3:-}" ] && ! printf '%s' "$err" | grep -qF -- "$3"; then
    printf 'FAIL: %s: stderr does not carry %s\n%s\n' "$2" "$3" "$err"
    failures=$((failures + 1))
  fi
}

# --- every target resolves ---
log <<'LOG'
### 2026-09-01 — resolving targets
**target:** `.claude/agents/self-review.md` § *Rules*, `AGENTS.md`, `.gitignore`
**Observed:** x
LOG
expect 0 'plain paths that exist'

# --- the refusal: a script moved while an open entry named its old path ---
log <<'LOG'
### 2026-09-07 — the gate matches only a bare open
**target:** `.claude/skills/ai-audit/scripts/check-review-register.sh`; `AGENTS.md`
**Observed:** x
LOG
expect 1 'a moved script, open entry' '.claude/skills/ai-audit/scripts/check-review-register.sh'
expect 1 'the refusal names the entry' '### 2026-09-07 — the gate matches only a bare open'

# --- the same dead path on a closed or superseded entry is history ---
for field in '**Forge:** forge-19' '**Closed by:** #77' '**Superseded by:** 2026-09-15 — the script moved'; do
  log <<LOG
### 2026-09-07 — the gate matches only a bare open
**target:** \`.claude/skills/ai-audit/scripts/check-review-register.sh\`
**Observed:** x
$field
LOG
  expect 0 "a moved script, entry carrying ${field%%:*}"
done

# --- braces: one dead member is enough ---
log <<'LOG'
### 2026-09-02 — a brace list
**target:** `.claude/agents/{self-review,design,design-review}.md`
LOG
expect 1 'a brace member that names a renamed file' '.claude/agents/design.md'
log <<'LOG'
### 2026-09-02 — a brace list
**target:** `.claude/agents/{self-review,design-review}.md`
LOG
expect 0 'every brace member exists'

# --- globs ---
log <<'LOG'
### 2026-09-03 — a glob
**target:** `.claude/**` and `ai-docs/scripts/*.sh`
LOG
expect 0 'globs that match'
log <<'LOG'
### 2026-09-03 — a glob
**target:** `.claude/skills/ai-audit/**`
LOG
expect 1 'a glob that matches nothing' '.claude/skills/ai-audit/**'

# --- tokens that are not repository paths ---
log <<'LOG'
### 2026-09-04 — tokens that are not paths
**target:** `AGENTS.md § Build & Test`, `AGENTS.md:67`, `:62`, the `Explore` spawn, `open`, `~/lab-private/DESIGN.md`, `/etc/absent.md`, `<spec>.md`, `$HOME/x.md`
LOG
expect 0 'a section suffix, a line locator, prose tokens, out-of-tree paths, placeholders'
log <<'LOG'
### 2026-09-04 — a dotfile
**target:** `.gitignored`
LOG
expect 1 'a dotfile that does not exist' '.gitignored'

# --- structure: no target line; an entry glued to the one before it ---
log <<'LOG'
### 2026-09-05 — no target line
**Observed:** x
LOG
expect 0 'an entry with no target line'
log <<'LOG'
### 2026-09-05 — first
**target:** `AGENTS.md`
**at:** abc1234
### 2026-09-06 — glued to the one above
**target:** `ai-docs/scripts/absent.sh`
LOG
expect 1 'an entry heading with no blank line before it' 'ai-docs/scripts/absent.sh'

# --- the instrument: only the fenced skeleton carries a dead path ---
log < /dev/null
expect 0 'the skeleton inside its fence'

rm "$repo/ai-docs/harness-gaps.md"
expect 0 'no log' ''

if [ "$failures" -eq 0 ]; then
  echo "harness-gaps targets: all fixtures behave as specified"
  exit 0
fi
printf 'harness-gaps targets: %d check(s) failed\n' "$failures"
exit 1
