#!/usr/bin/env bash
# Regression suite for the harness-gaps closing-field gate (Forge, Closed by).
#
# Two halves, both in scratch repositories. The shape fixtures rewrite one log
# on main, where there is no branch diff, so only the shape check can speak.
# The provenance fixtures start from a main whose log carries one entry marked
# Forge, one marked Closed by and one open, make one edit per branch, commit
# it, and read the guard's exit status under that branch's name. Each half
# carries its instrument check: the fence skip is shown to be what spares the
# skeleton lines, and the forge branch is shown to be what spares a mark.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  test-harness-gaps-forge.sh    run the whole suite; it takes no arguments
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard="$repo_root/ai-docs/scripts/check-harness-gaps-forge.sh"
[ -f "$guard" ] || { echo "FAIL: $guard missing"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/gaps-forge.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
failures=0

# The log as the real one is laid out: header, a fenced skeleton carrying both
# fields' template lines, a separator, then an entry a forge took, an entry a
# pull request closed, and an open entry.
write_log() {
  cat > "$1" <<'LOG'
# Harness gaps

Header prose.

## Entry skeleton

```
### YYYY-MM-DD — [short description of the gap]
**target:** [file]
**Forge:** forge-<N>[, forge-<M>]    (forge-only)
**Closed by:** #<N>[, #<M>]    (forge-only)
```

---

### 2026-09-01 — first entry
**target:** `x`
**Observed:** one.
**at:** abc1234
**Forge:** forge-3

### 2026-09-02 — second entry
**target:** `y`
**Observed:** two.
**at:** abc1234
**Closed by:** #77

### 2026-09-03 — third entry
**target:** `z`
**Observed:** three.
**at:** abc1234
LOG
}

new_repo() {
  local repo=$1
  mkdir -p "$repo/ai-docs"
  git -C "$repo" init -q -b main .
  git -C "$repo" config user.email t@t
  git -C "$repo" config user.name t
  git -C "$repo" config commit.gpgsign false
  write_log "$repo/ai-docs/harness-gaps.md"
  git -C "$repo" add -A
  git -C "$repo" commit -q -m base
}

# expect <PASS|FAIL> <name> <repo>
expect() {
  local want=$1 name=$2 repo=$3 rc got
  (cd "$repo" && bash "$guard") >/dev/null 2>"$scratch/stderr"; rc=$?
  case $rc in
    0) got=PASS ;;
    1) got=FAIL ;;
    *) got="exit $rc" ;;
  esac
  [ "$got" = "$want" ] && return 0
  printf 'FAIL [%s]: expected %s, got %s\n' "$name" "$want" "$got"
  sed 's/^/    /' "$scratch/stderr"
  failures=$((failures + 1))
}

F='^\*\*Forge:\*\* forge-3$'
C='^\*\*Closed by:\*\* #77$'

# --- shape: one log on main, rewritten per fixture ---------------------------
shape="$scratch/shape"
new_repo "$shape"
log="$shape/ai-docs/harness-gaps.md"

# shape_case <PASS|FAIL> <name> <sed program>
shape_case() {
  write_log "$log"
  sed -i "$3" "$log"
  expect "$1" "$2" "$shape"
}

expect PASS clean-log "$shape"
shape_case PASS two-forges        "s/$F/**Forge:** forge-3, forge-13/"
shape_case PASS two-prs           "s/$C/**Closed by:** #51, #77/"
shape_case PASS both-fields       "s/$F/**Forge:** forge-3\n**Closed by:** #80/"
shape_case FAIL no-space          "s/$F/**Forge:** forge-3,forge-13/"
shape_case FAIL trailing-note     "s/$F/**Forge:** forge-3 — partial/"
shape_case FAIL bare-number       "s/$F/**Forge:** 3/"
shape_case FAIL pr-in-forge       "s/$F/**Forge:** #15/"
shape_case FAIL empty-forge       "s/$F/**Forge:**/"
shape_case FAIL forge-twice       "s/$F/**Forge:** forge-3\n**Forge:** forge-4/"
shape_case FAIL forge-in-header   's/^Header prose\.$/Header prose.\n**Forge:** forge-3/'
shape_case FAIL closed-bare       "s/$C/**Closed by:** 77/"
shape_case FAIL closed-prefixed   "s/$C/**Closed by:** PR #77/"
shape_case FAIL closed-note       "s/$C/**Closed by:** #77 — partly/"
shape_case FAIL forge-in-closed   "s/$C/**Closed by:** forge-15/"
shape_case FAIL closed-twice      "s/$C/**Closed by:** #77\n**Closed by:** #78/"
shape_case FAIL closed-in-header  's/^Header prose\.$/Header prose.\n**Closed by:** #77/'
# Instrument check: without its fence the skeleton's template lines are judged,
# so the clean pass above is the fence skip working, not a matcher that sees
# nothing.
shape_case FAIL unfenced-skeleton '/^```$/d'

# --- provenance: one edit per branch, against main ---------------------------
prov="$scratch/prov"
new_repo "$prov"
plog="$prov/ai-docs/harness-gaps.md"

# branch_case <PASS|FAIL> <name> <branch> <sed program | append:<line>>
branch_case() {
  local want=$1 name=$2 branch=$3 edit=$4
  git -C "$prov" checkout -q -B "$branch" main
  case $edit in
    append:*) printf '%s\n' "${edit#append:}" >> "$plog" ;;
    *) sed -i "$edit" "$plog" ;;
  esac
  git -C "$prov" commit -q -am "$name"
  expect "$want" "$name" "$prov"
  git -C "$prov" checkout -q main
}

branch_case PASS park-a-gap        chore/park    'append:### 2026-09-04 — fourth entry'
branch_case PASS prose-edit        chore/prose   's/^\*\*Observed:\*\* one\.$/**Observed:** one, reworded./'
branch_case FAIL mark-own-entry    chore/mark    'append:**Forge:** forge-3'
branch_case FAIL close-own-entry   chore/close   'append:**Closed by:** #78'
branch_case FAIL unmark            chore/unmark  "/$F/d"
branch_case FAIL unclose           chore/unclose "/$C/d"
branch_case FAIL extend-mark       chore/extend  "s/$F/**Forge:** forge-3, forge-9/"
branch_case FAIL name-not-a-number harness/forge-x 'append:**Forge:** forge-3'
# Instrument check: the same edits pass on a forge branch, so each red above is
# the branch name speaking, not the edit being malformed.
branch_case PASS forge-marks       harness/forge-15 'append:**Forge:** forge-15'
branch_case PASS forge-closes      harness/forge-15 'append:**Closed by:** #78'
branch_case PASS forge-unmarks     harness/forge-15 "/$F/d"
branch_case PASS forge-uncloses    harness/forge-15 "/$C/d"
branch_case PASS forge-extends     harness/forge-15 "s/$F/**Forge:** forge-3, forge-15/"
branch_case FAIL forge-above-own   harness/forge-15 'append:**Forge:** forge-16'

# A detached HEAD carrying a mark is not a forge.
git -C "$prov" checkout -q -B chore/detach main
printf '**Closed by:** #78\n' >> "$plog"
git -C "$prov" commit -q -am detach
git -C "$prov" checkout -q --detach
expect FAIL detached-head "$prov"
git -C "$prov" checkout -q main

# After a forge branch merges, main itself has nothing to answer for.
git -C "$prov" checkout -q -B harness/forge-15 main
printf '**Forge:** forge-15\n' >> "$plog"
git -C "$prov" commit -q -am mark
git -C "$prov" checkout -q main
git -C "$prov" merge -q --no-ff -m merge harness/forge-15
expect PASS main-after-merge "$prov"

# No merge base: provenance is skipped and says so; shape still passes over a
# log whose entries all carry a closing field.
git -C "$prov" checkout -q --orphan lone
git -C "$prov" add -A
git -C "$prov" commit -q -m lone
expect PASS no-merge-base "$prov"
grep -q 'provenance not checked' "$scratch/stderr" ||
  { echo "FAIL [no-merge-base]: the skip was silent"; failures=$((failures + 1)); }

if [ "$failures" -eq 0 ]; then
  echo "harness-gaps closing-field guard: all fixtures behave as specified"
  exit 0
fi
printf 'harness-gaps closing-field guard: %d check(s) failed\n' "$failures"
exit 1
