#!/usr/bin/env bash
# Regression suite for ai-docs/scripts/doc-edit-guard.sh.
#
# The load-bearing fixture replays the real truncation of 2026-09-08
# (ai-docs/learnings.md, "left the spec untracked for three rounds"): a spec
# whose KD table mentions "## Open questions" inside a cell, and a Python edit
# that slices at `s.index("## Open questions")` -- which lands on the mention,
# not the heading, and drops Technical constraints, Source conflicts and the
# whole AC table. The guard must restore the file and exit 2. The same edit
# anchored on "\n## Open questions\n" must pass.
#
# Usage: bash ai-docs/scripts/test-doc-edit-guard.sh
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
guard=ai-docs/scripts/doc-edit-guard.sh
[ -f "$guard" ] || { echo "FAIL: $guard missing"; exit 1; }

scratch=$(mktemp -d "${TMPDIR:-/tmp}/doc-edit-guard.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
failures=0
fail() { printf 'FAIL: %s\n' "$1"; failures=$((failures + 1)); }

write_fixture() {
  cat > "$1" <<'SPEC'
# Fixture spec

**Source:** issue #0

## Scope

1. First item.

## Key decisions

| Decision | Resolution |
|---|---|
| KD-1 — shape | **Settled.** |
| KD-2 — open? | Deferred to ## Open questions below, see the list there. |

## Technical constraints

Constraint text.

## Source conflicts

None.

## Acceptance Criteria

| AC | Condition |
|---|---|
| AC1 | Holds. |
| AC2 | Holds too. |

## Open questions

- old question one
- old question two

```yaml
status: ready
```
SPEC
}

# --- 1. the real truncating edit: guard restores, exit 2 ---
f="$scratch/a.spec.md"; write_fixture "$f"
bash "$guard" snapshot "$f" >/dev/null || fail "snapshot exited non-zero"
[ -f "$f.bak" ] || fail "snapshot did not create $f.bak"
python3 - "$f" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
i = s.index("## Open questions")            # lands on the KD-2 cell mention
s = s[:i] + "## Open questions\n\n- the one live question\n"
p.write_text(s)
PY
grep -q '^| AC1' "$f" && fail "fixture did not reproduce the truncation (AC table still present)"
bash "$guard" verify "$f" >/dev/null 2>"$scratch/err"; rc=$?
[ "$rc" -eq 2 ] || fail "truncating edit: expected exit 2, got $rc"
grep -q '^| AC2 ' "$f" || fail "truncating edit: file was not restored"
grep -q 'RESTORED' "$scratch/err" || fail "truncating edit: no RESTORED message on stderr"
[ ! -f "$f.bak" ] || fail "truncating edit: .bak left behind after restore"

# --- 2. the same edit, anchored on the heading line: passes, .bak removed ---
f="$scratch/b.spec.md"; write_fixture "$f"
bash "$guard" snapshot "$f" >/dev/null
python3 - "$f" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); s = p.read_text()
i = s.index("\n## Open questions\n")
s = s[:i] + "\n## Open questions\n\n- the one live question\n\n```yaml\nstatus: ready\n```\n"
p.write_text(s)
PY
bash "$guard" verify "$f" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 0 ] || fail "well-anchored edit: expected exit 0, got $rc"
grep -q 'the one live question' "$f" || fail "well-anchored edit: edit was undone"
[ ! -f "$f.bak" ] || fail "well-anchored edit: .bak not removed on success"

# --- 3. growth is fine: adding a section and AC rows passes ---
f="$scratch/c.spec.md"; write_fixture "$f"
bash "$guard" snapshot "$f" >/dev/null
printf '\n## Deferred\n\n| AC3 | New. |\n' >> "$f"
bash "$guard" verify "$f" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 0 ] || fail "growth: expected exit 0, got $rc"

# --- 4. shrinking one AC row is caught even when sections are intact ---
f="$scratch/d.spec.md"; write_fixture "$f"
bash "$guard" snapshot "$f" >/dev/null
sed -i '/^| AC2 /d' "$f"
bash "$guard" verify "$f" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 2 ] || fail "dropped AC row: expected exit 2, got $rc"
grep -q '^| AC2 ' "$f" || fail "dropped AC row: not restored"

# --- 5. verify without snapshot is itself a defect: exit 2, file untouched ---
f="$scratch/e.spec.md"; write_fixture "$f"
cp "$f" "$scratch/e.orig"
bash "$guard" verify "$f" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 2 ] || fail "no snapshot: expected exit 2, got $rc"
cmp -s "$f" "$scratch/e.orig" || fail "no snapshot: file was modified"

# --- 6. usage errors exit 1 ---
bash "$guard" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 1 ] || fail "no args: expected exit 1, got $rc"
bash "$guard" frobnicate "$f" >/dev/null 2>&1; rc=$?
[ "$rc" -eq 1 ] || fail "bad mode: expected exit 1, got $rc"

if [ "$failures" -eq 0 ]; then
  echo "doc-edit guard: all fixtures behave as specified"
  exit 0
fi
printf 'doc-edit guard: %d check(s) failed\n' "$failures"
exit 1
