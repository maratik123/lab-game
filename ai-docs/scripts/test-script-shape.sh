#!/usr/bin/env bash
# Regression suite for the script-shape checker.
#
# Each fixture below is a sandbox repository holding the conforming set plus at
# most one defect, and each defect must be REPORTED BY NAME. A checker that
# printed nothing whatever it was given would satisfy the last case alone, so
# the clean sandbox is asserted too: it is the instrument check, and it is the
# only case here that must come back green.
#
# The sandboxes are real git repositories because the checker walks the tracked
# tree, which is the property that makes it hold for a file added later without
# anyone editing a list.
#
# THE DISPATCH ARM IS ASSEMBLED AT RUNTIME, never written out here. This file is
# itself a tracked script the checker scans, and a fixture spelled literally
# would be read as this file's own dispatch — five of them, in three shapes.
# The citation guard's suite assembles its fixture date for the same reason.
#
# Exit 0 = every fixture behaves as specified. Exit 1 = regression.

set -uo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root" || exit 1
checker="$repo_root/ai-docs/scripts/check-script-shape.sh"
[ -x "$checker" ] || { echo "FAIL: $checker not executable"; exit 1; }

failures=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT

conforming_arm='-h|--help'
divergent_arm='--help'

# dispatch <arm> — the case block a fixture ends its preamble with.
# shellcheck disable=SC2016  # the fixture's own parameter form, not this shell's
dispatch() { printf 'case "${1:-}" in\n  %s) usage; exit 0 ;;\nesac\n' "$1"; }

# A sandbox holding only conforming files. Every case starts from this.
seed() {
  local d="$sandbox/$1"
  mkdir -p "$d"
  git -C "$d" init -q
  git -C "$d" config user.email probe@example.invalid
  git -C "$d" config user.name probe
  cat > "$d/good.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  good.sh    conforms
USAGE
}

SH
  dispatch "$conforming_arm" >> "$d/good.sh"
  printf '\necho body\n' >> "$d/good.sh"

  cat > "$d/silent.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail
echo "answers no help flag, and owes none"
SH
  chmod +x "$d/good.sh" "$d/silent.sh"
  # A link whose target carries the extension: allowed, and the reason the
  # shebang rule cannot be a plain name check.
  ln -s good.sh "$d/entry"
  git -C "$d" add good.sh silent.sh entry
  printf '%s' "$d"
}

# verdict <dir> -> PASS | FAIL
verdict() { ( cd "$1" && bash "$checker" ) >/dev/null 2>&1 && echo PASS || echo FAIL; }

# expect_flagged <case-name> <dir> <substring the report must name>
expect_flagged() {
  local name=$1 dir=$2 needle=$3 got out
  got=$(verdict "$dir")
  out=$( cd "$dir" && bash "$checker" 2>&1 )
  if [ "$got" != "FAIL" ]; then
    printf 'FAIL [%s]: the checker passed a tree it must refuse\n' "$name"
    failures=$((failures + 1))
    return
  fi
  case "$out" in
    *"$needle"*) ;;
    *)
      printf 'FAIL [%s]: refused, but the report never names %s\n%s\n' "$name" "$needle" "$out"
      failures=$((failures + 1))
      ;;
  esac
}

# --- The instrument check: a wholly conforming tree must come back green -----
clean=$(seed clean)
if [ "$(verdict "$clean")" != "PASS" ]; then
  printf 'FAIL [instrument]: the checker refuses a conforming tree, so every case below would pass for the wrong reason\n'
  ( cd "$clean" && bash "$checker" 2>&1 ) | sed 's/^/  /'
  failures=$((failures + 1))
fi

# --- A different dispatch shape ----------------------------------------------
d=$(seed shape)
cat > "$d/odd.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail

usage() { echo "Usage: odd.sh"; }

SH
dispatch "$divergent_arm" >> "$d/odd.sh"
printf '\necho body\n' >> "$d/odd.sh"
chmod +x "$d/odd.sh"
git -C "$d" add odd.sh
expect_flagged "different-shape" "$d" "odd.sh"

# --- The exact shape, but something runs first -------------------------------
d=$(seed late)
cat > "$d/late.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  late.sh    answers too late
USAGE
}

root=$(git rev-parse --show-toplevel)

SH
dispatch "$conforming_arm" >> "$d/late.sh"
# shellcheck disable=SC2016  # the fixture's own variable, not this shell's
printf '\necho "$root"\n' >> "$d/late.sh"
chmod +x "$d/late.sh"
git -C "$d" add late.sh
expect_flagged "runs-before-help" "$d" "late.sh"

# --- The exact shape, answered with a non-zero status -------------------------
d=$(seed status)
cat > "$d/rc.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail

usage() {
  cat <<'USAGE'
Usage:
  rc.sh    prints, then refuses
USAGE
  exit 3
}

SH
dispatch "$conforming_arm" >> "$d/rc.sh"
printf '\necho body\n' >> "$d/rc.sh"
chmod +x "$d/rc.sh"
git -C "$d" add rc.sh
expect_flagged "non-zero-help" "$d" "rc.sh"

# --- The exact shape, answered with nothing ----------------------------------
d=$(seed empty)
cat > "$d/mute.sh" <<'SH'
#!/usr/bin/env bash
set -uo pipefail

usage() { :; }

SH
dispatch "$conforming_arm" >> "$d/mute.sh"
printf '\necho body\n' >> "$d/mute.sh"
chmod +x "$d/mute.sh"
git -C "$d" add mute.sh
expect_flagged "empty-help" "$d" "mute.sh"

# --- A shebang with neither the extension nor a link -------------------------
d=$(seed shebang)
printf '#!/usr/bin/env bash\necho stray\n' > "$d/stray"
chmod +x "$d/stray"
git -C "$d" add stray
expect_flagged "bare-shebang" "$d" "stray"

# --- A link to a file that does not carry the extension ----------------------
d=$(seed badlink)
printf '#!/usr/bin/env bash\necho target\n' > "$d/target"
chmod +x "$d/target"
ln -s target "$d/alias"
git -C "$d" add target alias
expect_flagged "link-to-extensionless" "$d" "alias"

if [ "$failures" -eq 0 ]; then
  echo "script shape guard: all fixtures behave as specified"
  exit 0
fi
printf 'script shape guard: %d check(s) failed\n' "$failures"
exit 1
